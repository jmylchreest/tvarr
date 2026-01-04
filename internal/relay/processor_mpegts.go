// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
)

// MPEG-TS Processor errors.
var (
	ErrMPEGTSProcessorClosed = errors.New("MPEG-TS processor closed")
)

// MPEGTSProcessorConfig configures the MPEG-TS processor.
type MPEGTSProcessorConfig struct {
	// ChunkSize is the size of each data chunk to buffer before broadcasting.
	ChunkSize int

	// MaxBufferSize is the maximum size of the output buffer per client.
	MaxBufferSize int

	// Logger for structured logging.
	Logger *slog.Logger

	// OnClientChange is called when clients connect or disconnect.
	// The callback receives the new client count.
	OnClientChange func(clientCount int)
}

// DefaultMPEGTSProcessorConfig returns sensible defaults.
func DefaultMPEGTSProcessorConfig() MPEGTSProcessorConfig {
	return MPEGTSProcessorConfig{
		ChunkSize:     188 * 7 * 10, // 70 TS packets (~13KB)
		MaxBufferSize: 188 * 1000,   // ~188KB per client
		Logger:        slog.Default(),
	}
}

// MPEGTSProcessor reads from a SharedESBuffer variant and produces a continuous MPEG-TS stream.
// Unlike HLS/DASH processors, this outputs a continuous stream without segmentation.
// It implements the Processor interface.
type MPEGTSProcessor struct {
	*ESProcessorBase

	config MPEGTSProcessorConfig

	// TS muxer for generating output
	muxer    *TSMuxer
	muxerBuf bytes.Buffer

	// PAT/PMT header bytes to send to new clients
	// MPEG-TS requires PAT/PMT tables for demuxing - clients joining late need these
	patPmtHeader   []byte
	patPmtHeaderMu sync.RWMutex

	// Streaming clients
	streamClients   map[string]*mpegtsStreamClient
	streamClientsMu sync.RWMutex
}

// mpegtsStreamClient represents a connected streaming client.
type mpegtsStreamClient struct {
	id              string
	writer          http.ResponseWriter
	flusher         http.Flusher
	reqCtx          context.Context // Request context - signals when client disconnects
	done            chan struct{}
	writeCh         chan []byte   // Buffered channel for data to write
	mu              sync.Mutex    // Protects waitForKeyframe only
	bytesWritten    atomic.Uint64 // Atomic for lock-free updates
	startedAt       time.Time
	waitForKeyframe bool        // True if client is waiting for next keyframe before receiving data
	slowClient      atomic.Bool // True if client is falling behind
}

// NewMPEGTSProcessor creates a new MPEG-TS processor.
func NewMPEGTSProcessor(
	id string,
	esBuffer *SharedESBuffer,
	variant CodecVariant,
	config MPEGTSProcessorConfig,
) *MPEGTSProcessor {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	esBase := NewESProcessorBase(id, OutputFormatMPEGTS, esBuffer, variant, ESProcessorConfig{
		Logger: config.Logger,
	})

	p := &MPEGTSProcessor{
		ESProcessorBase: esBase,
		config:          config,
		streamClients:   make(map[string]*mpegtsStreamClient),
	}

	return p
}

// Start begins processing data from the shared buffer.
func (p *MPEGTSProcessor) Start(ctx context.Context) error {
	p.config.Logger.Debug("MPEG-TS processor requesting variant from buffer",
		slog.String("processor_id", p.id),
		slog.String("requested_variant", p.Variant().String()))

	esVariant, err := p.InitES(ctx)
	if err != nil {
		return err
	}

	// Wait for audio codec and AAC init data using base class helpers
	audioCodec := p.WaitForAudioCodec()
	aacConfig := p.WaitForAACInitData()

	// Initialize TS muxer with the correct codec types from the tracks
	p.muxer = NewTSMuxer(&p.muxerBuf, TSMuxerConfig{
		Logger:     p.config.Logger,
		VideoCodec: p.ResolvedVideoCodec(),
		AudioCodec: audioCodec,
		AACConfig:  aacConfig,
	})

	// Capture PAT/PMT header bytes for new clients
	// MPEG-TS demuxers need these tables to understand the stream structure
	patPmt, err := p.muxer.InitializeAndGetHeader()
	if err != nil {
		p.config.Logger.Warn("Failed to capture PAT/PMT header",
			slog.String("error", err.Error()))
	} else {
		p.patPmtHeaderMu.Lock()
		p.patPmtHeader = patPmt
		p.patPmtHeaderMu.Unlock()
		p.config.Logger.Log(p.Context(), observability.LevelTrace, "Captured PAT/PMT header for late-joining clients",
			slog.Int("size_bytes", len(patPmt)))
	}

	p.config.Logger.Debug("MPEG-TS muxer initialized",
		slog.String("requested_variant", p.Variant().String()),
		slog.String("resolved_variant", esVariant.Variant().String()),
		slog.String("video_codec", p.ResolvedVideoCodec()),
		slog.String("audio_codec", audioCodec),
		slog.Bool("has_aac_config", aacConfig != nil))

	p.config.Logger.Debug("Starting MPEG-TS processor",
		slog.String("id", p.id),
		slog.String("variant", p.Variant().String()))

	// Start processing loop
	p.WaitGroup().Add(1)
	go func() {
		defer p.WaitGroup().Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *MPEGTSProcessor) Stop() {
	// Close all stream clients before stopping processing
	p.streamClientsMu.Lock()
	for _, client := range p.streamClients {
		close(client.done)
	}
	p.streamClients = make(map[string]*mpegtsStreamClient)
	p.streamClientsMu.Unlock()

	// Use base class cleanup
	p.StopES()
}

// RegisterClient adds a streaming client.
// Returns ErrProcessorStopping if the processor is being shut down.
func (p *MPEGTSProcessor) RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error {
	// Check if processor is stopping before doing any registration
	// This prevents the TOCTOU race where a client registers while the processor is being cleaned up
	if p.IsStopping() {
		p.config.Logger.Debug("Rejecting MPEG-TS client - processor is stopping",
			slog.String("client_id", clientID))
		return ErrProcessorStopping
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("response writer does not support flushing")
	}

	// Register with base processor first (this checks stopping flag under lock)
	_, err := p.RegisterClientBase(clientID, w, r)
	if err != nil {
		return err
	}

	client := &mpegtsStreamClient{
		id:              clientID,
		writer:          w,
		flusher:         flusher,
		reqCtx:          r.Context(), // Store request context to detect client disconnect
		done:            make(chan struct{}),
		writeCh:         make(chan []byte, 64), // Buffer up to 64 chunks (~1MB at 13KB/chunk)
		startedAt:       time.Now(),
		waitForKeyframe: true, // New clients wait for next keyframe before receiving data
	}

	p.streamClientsMu.Lock()
	p.streamClients[clientID] = client
	clientCount := len(p.streamClients)
	p.streamClientsMu.Unlock()

	// Start a dedicated writer goroutine for this client
	go p.clientWriteLoop(client)

	p.config.Logger.Debug("Registered MPEG-TS stream client",
		slog.String("client_id", clientID),
		slog.Bool("waiting_for_keyframe", true))

	// Notify callback of client change
	if p.config.OnClientChange != nil {
		p.config.OnClientChange(clientCount)
	}

	return nil
}

// UnregisterClient removes a streaming client.
func (p *MPEGTSProcessor) UnregisterClient(clientID string) {
	p.streamClientsMu.Lock()
	clientExists := false
	if client, exists := p.streamClients[clientID]; exists {
		clientExists = true
		close(client.done)
		delete(p.streamClients, clientID)
	}
	newCount := len(p.streamClients)
	p.streamClientsMu.Unlock()

	p.UnregisterClientBase(clientID)

	p.config.Logger.Debug("Unregistered MPEG-TS stream client",
		slog.String("client_id", clientID),
		slog.Int("remaining_clients", newCount))

	// Notify callback of client change
	if clientExists && p.config.OnClientChange != nil {
		p.config.OnClientChange(newCount)
	}
}

// ClientCount returns the number of connected streaming clients.
// This overrides BaseProcessor.ClientCount because MPEG-TS uses persistent connections
// tracked in streamClients rather than request-based tracking in the base clients map.
func (p *MPEGTSProcessor) ClientCount() int {
	p.streamClientsMu.RLock()
	defer p.streamClientsMu.RUnlock()
	return len(p.streamClients)
}

// IsIdle returns true if no clients are connected.
func (p *MPEGTSProcessor) IsIdle() bool {
	return p.ClientCount() == 0
}

// ServeManifest is not applicable for raw MPEG-TS streams.
func (p *MPEGTSProcessor) ServeManifest(w http.ResponseWriter, r *http.Request) error {
	http.Error(w, "MPEG-TS streams do not have manifests", http.StatusNotFound)
	return errors.New("MPEG-TS streams do not have manifests")
}

// ServeSegment is not applicable for raw MPEG-TS streams.
func (p *MPEGTSProcessor) ServeSegment(w http.ResponseWriter, r *http.Request, segmentName string) error {
	http.Error(w, "MPEG-TS streams do not have segments", http.StatusNotFound)
	return errors.New("MPEG-TS streams do not have segments")
}

// ServeStream serves the continuous MPEG-TS stream to a client.
// This is the main entry point for MPEG-TS streaming.
func (p *MPEGTSProcessor) ServeStream(w http.ResponseWriter, r *http.Request, clientID string) error {
	// Register the client
	if err := p.RegisterClient(clientID, w, r); err != nil {
		return err
	}
	defer p.UnregisterClient(clientID)

	// Set headers for streaming
	p.SetStreamHeaders(w)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")

	// Wait for PAT/PMT to be available
	ctx := p.Context()
	var patPmtHeader []byte
	waitStart := time.Now()
	for time.Since(waitStart) < 5*time.Second {
		p.patPmtHeaderMu.RLock()
		patPmtHeader = p.patPmtHeader
		p.patPmtHeaderMu.RUnlock()
		if len(patPmtHeader) > 0 {
			break
		}
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
			// Continue waiting
		}
	}

	if len(patPmtHeader) == 0 {
		p.config.Logger.Warn("PAT/PMT not available for client, demux probe may fail",
			slog.String("client_id", clientID))
	}

	// Send PAT/PMT header for late-joining clients
	if len(patPmtHeader) > 0 {
		if _, err := w.Write(patPmtHeader); err != nil {
			p.config.Logger.Debug("Failed to send initial PAT/PMT to client",
				slog.String("client_id", clientID),
				slog.String("error", err.Error()))
			return err
		}
		p.config.Logger.Log(ctx, observability.LevelTrace, "Sent initial PAT/PMT to client",
			slog.String("client_id", clientID),
			slog.Int("bytes", len(patPmtHeader)))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	// Get the client
	p.streamClientsMu.RLock()
	client, exists := p.streamClients[clientID]
	p.streamClientsMu.RUnlock()

	if !exists {
		return errors.New("client not found")
	}

	// Wait for client to disconnect or context to cancel
	select {
	case <-client.done:
		p.config.Logger.Info("MPEG-TS client done channel closed",
			slog.String("client_id", clientID),
			slog.Uint64("bytes_written", client.bytesWritten.Load()),
			slog.Duration("connected_duration", time.Since(client.startedAt)))
		return nil
	case <-r.Context().Done():
		p.config.Logger.Info("MPEG-TS client HTTP context cancelled",
			slog.String("client_id", clientID),
			slog.Uint64("bytes_written", client.bytesWritten.Load()),
			slog.Duration("connected_duration", time.Since(client.startedAt)),
			slog.String("error", r.Context().Err().Error()))
		return r.Context().Err()
	case <-ctx.Done():
		p.config.Logger.Info("MPEG-TS processor context cancelled",
			slog.String("client_id", clientID),
			slog.Uint64("bytes_written", client.bytesWritten.Load()),
			slog.Duration("connected_duration", time.Since(client.startedAt)),
			slog.String("error", ctx.Err().Error()))
		return ctx.Err()
	}
}

// runProcessingLoop is the main processing loop.
func (p *MPEGTSProcessor) runProcessingLoop(esVariant *ESVariant) {
	ctx := p.Context()
	videoTrack := esVariant.VideoTrack()
	audioTrack := esVariant.AudioTrack()

	// Wait for initial video keyframe using base class helper
	if _, ok := p.WaitForKeyframe(videoTrack); !ok {
		return
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// Read and process samples
			hasKeyframe := p.processAvailableSamples(videoTrack, audioTrack)

			// Flush muxer and broadcast to clients
			p.muxer.Flush()
			if p.muxerBuf.Len() > 0 {
				data := p.muxerBuf.Bytes()
				p.broadcastToClients(data, hasKeyframe)
				p.muxerBuf.Reset()
			}
		}
	}
}

// processAvailableSamples reads and muxes available ES samples.
// When a keyframe is encountered, it flushes pre-keyframe data to existing clients only,
// then signals that new clients can start receiving from the keyframe.
// Returns true if a keyframe was processed (to trigger keyframe buffer update).
func (p *MPEGTSProcessor) processAvailableSamples(videoTrack, audioTrack *ESTrack) bool {
	hasKeyframe := false

	// Read video samples
	videoSamples := videoTrack.ReadFrom(p.LastVideoSeq(), 100)
	var bytesRead uint64
	for _, sample := range videoSamples {
		bytesRead += uint64(len(sample.Data))
		if sample.IsKeyframe {
			// Before writing the keyframe, flush any buffered data to EXISTING clients only.
			// This ensures new clients start exactly at the keyframe boundary.
			p.muxer.Flush()
			if p.muxerBuf.Len() > 0 {
				preKeyframeData := make([]byte, p.muxerBuf.Len())
				copy(preKeyframeData, p.muxerBuf.Bytes())
				p.muxerBuf.Reset()
				// Send pre-keyframe data only to clients already receiving (not waiting)
				p.broadcastToExistingClients(preKeyframeData)
			}
			hasKeyframe = true
		}
		// Log muxer errors for debugging but continue streaming
		if err := p.muxer.WriteVideo(sample.PTS, sample.DTS, sample.Data, sample.IsKeyframe); err != nil {
			p.config.Logger.Debug("WriteVideo error",
				slog.String("error", err.Error()),
				slog.Int64("pts", sample.PTS),
				slog.Int("data_len", len(sample.Data)),
				slog.Bool("keyframe", sample.IsKeyframe))
		}
		p.SetLastVideoSeq(sample.Sequence)
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(p.LastAudioSeq(), 200)
	for _, sample := range audioSamples {
		bytesRead += uint64(len(sample.Data))
		// Log muxer errors for debugging but continue streaming
		if err := p.muxer.WriteAudio(sample.PTS, sample.Data); err != nil {
			p.config.Logger.Debug("WriteAudio error",
				slog.String("error", err.Error()),
				slog.Int64("pts", sample.PTS),
				slog.Int("data_len", len(sample.Data)))
		}
		p.SetLastAudioSeq(sample.Sequence)
	}

	// Track bytes read from buffer for bandwidth stats
	if bytesRead > 0 {
		p.TrackBytesFromBuffer(bytesRead)
	}

	// Update consumer position to allow eviction of samples we've processed
	if len(videoSamples) > 0 || len(audioSamples) > 0 {
		p.UpdateConsumerPosition()
	}

	return hasKeyframe
}

// broadcastToExistingClients sends data only to clients that are already receiving
// (not waiting for a keyframe). Used to send pre-keyframe data.
// Uses per-client write channels for efficient non-blocking writes.
func (p *MPEGTSProcessor) broadcastToExistingClients(data []byte) {
	if len(data) == 0 {
		return
	}

	p.streamClientsMu.RLock()
	clients := make([]*mpegtsStreamClient, 0, len(p.streamClients))
	for _, c := range p.streamClients {
		clients = append(clients, c)
	}
	p.streamClientsMu.RUnlock()

	for _, client := range clients {
		select {
		case <-client.done:
			continue
		default:
		}

		// Check waitForKeyframe under lock
		client.mu.Lock()
		shouldSkip := client.waitForKeyframe
		client.mu.Unlock()

		if shouldSkip {
			continue
		}

		// Copy data for this client (channels share memory, need copy for safety)
		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)

		// Non-blocking send to client's write channel
		select {
		case client.writeCh <- dataCopy:
			// Data queued successfully
		default:
			// Channel full - client is too slow
			if !client.slowClient.Swap(true) {
				// First time detecting slow client, log warning
				p.config.Logger.Warn("Client write buffer full (pre-keyframe), disconnecting slow client",
					slog.String("client_id", client.id),
					slog.String("processor_id", p.id),
					slog.Uint64("bytes_written", client.bytesWritten.Load()),
					slog.Duration("connected_duration", time.Since(client.startedAt)))
			}
			p.UnregisterClient(client.id)
		}
	}

	p.RecordBytesWritten(uint64(len(data)))
}

// broadcastToClients sends data to all connected streaming clients.
// New clients wait for the next keyframe before receiving data.
// Uses per-client write channels for efficient non-blocking writes.
func (p *MPEGTSProcessor) broadcastToClients(data []byte, hasKeyframe bool) {
	if len(data) == 0 {
		return
	}

	ctx := p.Context()

	p.streamClientsMu.RLock()
	clients := make([]*mpegtsStreamClient, 0, len(p.streamClients))
	for _, c := range p.streamClients {
		clients = append(clients, c)
	}
	p.streamClientsMu.RUnlock()

	// Get PAT/PMT header for new clients (read once, share with all)
	p.patPmtHeaderMu.RLock()
	patPmtHeader := p.patPmtHeader
	p.patPmtHeaderMu.RUnlock()

	for _, client := range clients {
		select {
		case <-client.done:
			// Client is done, skip
			continue
		default:
		}

		// Check and update waitForKeyframe state under lock
		client.mu.Lock()
		shouldSkip := false
		needsPatPmt := false
		if client.waitForKeyframe {
			if hasKeyframe {
				// Keyframe found - start sending data to this client
				client.waitForKeyframe = false
				needsPatPmt = true // New client needs PAT/PMT tables first
				p.config.Logger.Log(ctx, observability.LevelTrace, "Client starting at keyframe",
					slog.String("client_id", client.id))
			} else {
				// No keyframe yet - skip this client
				shouldSkip = true
			}
		}
		clientID := client.id
		client.mu.Unlock()

		if shouldSkip {
			continue
		}

		// The muxer automatically writes PAT/PMT when outputting keyframes
		// (RandomAccessIndicator=true triggers WriteTables internally).
		// We only need to prepend our captured PAT/PMT if the data doesn't
		// already start with PAT (which would cause continuity counter errors).
		dataToWrite := data

		// Check if data already starts with PAT (PID 0x0000)
		// PAT header: sync(0x47) + flags/PID where PID 0 = 0x40 0x00 or 0x00 0x00
		dataAlreadyHasPAT := len(data) >= 3 && data[0] == 0x47 && (data[1]&0x1F) == 0x00 && data[2] == 0x00

		if needsPatPmt && len(patPmtHeader) > 0 && !dataAlreadyHasPAT {
			// Data doesn't have PAT - prepend our captured PAT/PMT
			combined := make([]byte, len(patPmtHeader)+len(data))
			copy(combined, patPmtHeader)
			copy(combined[len(patPmtHeader):], data)
			dataToWrite = combined
			p.config.Logger.Log(ctx, observability.LevelTrace, "Prepending PAT/PMT to new client data",
				slog.String("client_id", clientID),
				slog.Int("total_bytes", len(combined)))
		}

		// Copy data for this client (channels share memory, need copy for safety)
		dataCopy := make([]byte, len(dataToWrite))
		copy(dataCopy, dataToWrite)

		// Non-blocking send to client's write channel
		select {
		case client.writeCh <- dataCopy:
			// Data queued successfully
		default:
			// Channel full - client is too slow
			if !client.slowClient.Swap(true) {
				// First time detecting slow client, log warning
				p.config.Logger.Warn("Client write buffer full, disconnecting slow client",
					slog.String("client_id", clientID),
					slog.String("processor_id", p.id),
					slog.Uint64("bytes_written", client.bytesWritten.Load()),
					slog.Duration("connected_duration", time.Since(client.startedAt)),
					slog.Int("pending_write_bytes", len(dataCopy)))
			}
			p.UnregisterClient(clientID)
		}
	}

	// Update stats
	p.RecordBytesWritten(uint64(len(data)))
}

// clientWriteLoop is a dedicated goroutine for writing data to a client.
// This eliminates the need to spawn a goroutine for each write.
// Also monitors the request context to detect client disconnects even when no data is flowing.
func (p *MPEGTSProcessor) clientWriteLoop(client *mpegtsStreamClient) {
	for {
		select {
		case <-client.done:
			return

		case <-client.reqCtx.Done():
			// Client disconnected (request context cancelled)
			// This catches disconnects even when no data is flowing (e.g., after origin EOF)
			p.config.Logger.Info("MPEG-TS client disconnected (context cancelled)",
				slog.String("client_id", client.id),
				slog.String("processor_id", p.id),
				slog.Uint64("bytes_written", client.bytesWritten.Load()),
				slog.Duration("connected_duration", time.Since(client.startedAt)))
			p.UnregisterClient(client.id)
			return

		case data := <-client.writeCh:
			// Check if client was closed while we were waiting for data
			// This prevents writing to a closed connection (which can panic)
			select {
			case <-client.done:
				return
			default:
			}

			// Write with timeout using deadline
			_, err := client.writer.Write(data)
			if err != nil {
				p.config.Logger.Info("MPEG-TS client write failed in write loop, disconnecting",
					slog.String("client_id", client.id),
					slog.String("processor_id", p.id),
					slog.Uint64("bytes_written", client.bytesWritten.Load()),
					slog.Duration("connected_duration", time.Since(client.startedAt)),
					slog.String("error", err.Error()))
				p.UnregisterClient(client.id)
				return
			}
			if client.flusher != nil {
				client.flusher.Flush()
			}
			client.bytesWritten.Add(uint64(len(data)))
			p.UpdateClientBytes(client.id, uint64(len(data)))
		}
	}
}

// SupportsStreaming returns true since this is a streaming processor.
func (p *MPEGTSProcessor) SupportsStreaming() bool {
	return true
}
