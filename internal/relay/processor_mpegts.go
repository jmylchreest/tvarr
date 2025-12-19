// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
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
	*BaseProcessor

	config   MPEGTSProcessorConfig
	esBuffer *SharedESBuffer
	variant  CodecVariant

	// TS muxer for generating output
	muxer    *TSMuxer
	muxerBuf bytes.Buffer

	// ES reading state
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Reference to the ES variant for consumer tracking
	esVariant *ESVariant

	// Streaming clients
	streamClients   map[string]*mpegtsStreamClient
	streamClientsMu sync.RWMutex

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
}

// mpegtsStreamClient represents a connected streaming client.
type mpegtsStreamClient struct {
	id              string
	writer          http.ResponseWriter
	flusher         http.Flusher
	done            chan struct{}
	mu              sync.Mutex    // Protects waitForKeyframe only
	bytesWritten    atomic.Uint64 // Atomic for lock-free updates
	startedAt       time.Time
	waitForKeyframe bool // True if client is waiting for next keyframe before receiving data
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

	base := NewBaseProcessor(id, OutputFormatMPEGTS, nil)

	p := &MPEGTSProcessor{
		BaseProcessor: base,
		config:        config,
		esBuffer:      esBuffer,
		variant:       variant,
		streamClients: make(map[string]*mpegtsStreamClient),
	}

	return p
}

// Start begins processing data from the shared buffer.
func (p *MPEGTSProcessor) Start(ctx context.Context) error {
	if !p.started.CompareAndSwap(false, true) {
		return errors.New("processor already started")
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.BaseProcessor.startedAt = time.Now()

	// Get or create the variant we'll read from
	// This will wait for the source variant to be ready if requesting VariantCopy
	esVariant, err := p.esBuffer.GetOrCreateVariantWithContext(p.ctx, p.variant)
	if err != nil {
		return fmt.Errorf("getting variant: %w", err)
	}

	// Register with buffer
	p.esBuffer.RegisterProcessor(p.id)

	// Store variant reference and register as a consumer to prevent eviction of unread samples
	p.esVariant = esVariant
	esVariant.RegisterConsumer(p.id)

	// Resolve actual codecs from the ES variant's tracks
	// The variant key (e.g., "h265/") may not include audio if it wasn't detected
	// when the source was created, but the track codec gets updated when audio arrives.
	// For accurate codec info, we read directly from the tracks.
	videoCodec := esVariant.VideoTrack().Codec()
	audioCodec := esVariant.AudioTrack().Codec()

	// If audio codec is empty, wait briefly for audio detection
	// Audio often arrives shortly after video in the stream
	if audioCodec == "" {
		p.config.Logger.Debug("Waiting for audio codec detection")
		waitCtx, waitCancel := context.WithTimeout(p.ctx, 2*time.Second)
		ticker := time.NewTicker(50 * time.Millisecond)
		for audioCodec == "" {
			select {
			case <-waitCtx.Done():
				p.config.Logger.Debug("Audio codec detection timeout, proceeding without audio")
				ticker.Stop()
				waitCancel()
				goto initMuxer
			case <-ticker.C:
				audioCodec = esVariant.AudioTrack().Codec()
			}
		}
		ticker.Stop()
		waitCancel()
		p.config.Logger.Debug("Audio codec detected", slog.String("audio_codec", audioCodec))
	}

initMuxer:
	// Get AAC config from initData if available
	// For AAC, we need the initData to get correct sample rate/channels
	// Wait briefly for it since the demuxer may not have parsed the first ADTS packet yet
	var aacConfig *mpeg4audio.AudioSpecificConfig
	if audioCodec == "aac" {
		initData := esVariant.AudioTrack().GetInitData()
		if initData == nil {
			p.config.Logger.Debug("Waiting for AAC initData from demuxer")
			waitCtx, waitCancel := context.WithTimeout(p.ctx, 2*time.Second)
			ticker := time.NewTicker(50 * time.Millisecond)
		waitLoop:
			for {
				select {
				case <-waitCtx.Done():
					p.config.Logger.Debug("AAC initData timeout, using defaults")
					break waitLoop
				case <-ticker.C:
					initData = esVariant.AudioTrack().GetInitData()
					if initData != nil {
						break waitLoop
					}
				}
			}
			ticker.Stop()
			waitCancel()
		}

		if initData != nil {
			aacConfig = &mpeg4audio.AudioSpecificConfig{}
			if err := aacConfig.Unmarshal(initData); err != nil {
				p.config.Logger.Debug("Failed to unmarshal AAC config from initData, using defaults",
					slog.String("error", err.Error()))
				aacConfig = nil
			} else {
				p.config.Logger.Debug("AAC config from initData",
					slog.Int("type", int(aacConfig.Type)),
					slog.Int("sample_rate", aacConfig.SampleRate),
					slog.Int("channel_count", aacConfig.ChannelCount))
			}
		} else {
			p.config.Logger.Debug("No AAC initData available, using defaults")
		}
	}

	// Initialize TS muxer with the correct codec types from the tracks
	p.muxer = NewTSMuxer(&p.muxerBuf, TSMuxerConfig{
		Logger:     p.config.Logger,
		VideoCodec: videoCodec,
		AudioCodec: audioCodec,
		AACConfig:  aacConfig,
	})

	p.config.Logger.Debug("MPEG-TS muxer initialized",
		slog.String("requested_variant", p.variant.String()),
		slog.String("resolved_variant", esVariant.Variant().String()),
		slog.String("video_codec", videoCodec),
		slog.String("audio_codec", audioCodec),
		slog.Bool("has_aac_config", aacConfig != nil))

	p.config.Logger.Debug("Starting MPEG-TS processor",
		slog.String("id", p.id),
		slog.String("variant", p.variant.String()))

	// Start processing loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *MPEGTSProcessor) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()

	// Close all stream clients
	p.streamClientsMu.Lock()
	for _, client := range p.streamClients {
		close(client.done)
	}
	p.streamClients = make(map[string]*mpegtsStreamClient)
	p.streamClientsMu.Unlock()

	// Unregister as a consumer to allow eviction of our unread samples
	if p.esVariant != nil {
		p.esVariant.UnregisterConsumer(p.id)
	}

	p.esBuffer.UnregisterProcessor(p.id)
	p.BaseProcessor.Close()

	p.config.Logger.Debug("MPEG-TS processor stopped",
		slog.String("id", p.id))
}

// RegisterClient adds a streaming client.
func (p *MPEGTSProcessor) RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("response writer does not support flushing")
	}

	client := &mpegtsStreamClient{
		id:              clientID,
		writer:          w,
		flusher:         flusher,
		done:            make(chan struct{}),
		startedAt:       time.Now(),
		waitForKeyframe: true, // New clients wait for next keyframe before receiving data
	}

	p.streamClientsMu.Lock()
	p.streamClients[clientID] = client
	clientCount := len(p.streamClients)
	p.streamClientsMu.Unlock()

	// Also register with base processor for stats
	p.RegisterClientBase(clientID, w, r)

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
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")

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
		return nil
	case <-r.Context().Done():
		return r.Context().Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

// runProcessingLoop is the main processing loop.
func (p *MPEGTSProcessor) runProcessingLoop(esVariant *ESVariant) {
	videoTrack := esVariant.VideoTrack()
	audioTrack := esVariant.AudioTrack()

	// Wait for initial video keyframe
	p.config.Logger.Debug("Waiting for initial keyframe")
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-videoTrack.NotifyChan():
		}

		samples := videoTrack.ReadFromKeyframe(p.lastVideoSeq, 1)
		if len(samples) > 0 {
			p.lastVideoSeq = samples[0].Sequence - 1
			break
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
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
	videoSamples := videoTrack.ReadFrom(p.lastVideoSeq, 100)
	for _, sample := range videoSamples {
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
		p.lastVideoSeq = sample.Sequence
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(p.lastAudioSeq, 200)
	for _, sample := range audioSamples {
		// Log muxer errors for debugging but continue streaming
		if err := p.muxer.WriteAudio(sample.PTS, sample.Data); err != nil {
			p.config.Logger.Debug("WriteAudio error",
				slog.String("error", err.Error()),
				slog.Int64("pts", sample.PTS),
				slog.Int("data_len", len(sample.Data)))
		}
		p.lastAudioSeq = sample.Sequence
	}

	// Update consumer position to allow eviction of samples we've processed
	if p.esVariant != nil && (len(videoSamples) > 0 || len(audioSamples) > 0) {
		p.esVariant.UpdateConsumerPosition(p.id, p.lastVideoSeq, p.lastAudioSeq)
	}

	return hasKeyframe
}

// broadcastToExistingClients sends data only to clients that are already receiving
// (not waiting for a keyframe). Used to send pre-keyframe data.
// IMPORTANT: This method does NOT hold locks during HTTP I/O to prevent blocking.
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

		// Check waitForKeyframe under lock, get I/O references
		client.mu.Lock()
		shouldSkip := client.waitForKeyframe
		writer := client.writer
		flusher := client.flusher
		clientID := client.id
		client.mu.Unlock()

		if shouldSkip {
			continue
		}

		// Perform HTTP I/O WITHOUT holding any locks
		// Use a channel-based timeout to prevent blocking the processing loop
		writeDone := make(chan error, 1)
		go func() {
			_, err := writer.Write(data)
			writeDone <- err
		}()

		// Wait for write to complete with timeout
		select {
		case err := <-writeDone:
			if err != nil {
				p.config.Logger.Debug("Client write failed",
					slog.String("client_id", clientID),
					slog.String("error", err.Error()))
				p.UnregisterClient(clientID)
				continue
			}
		case <-time.After(5 * time.Second):
			// Write timed out - client is too slow, disconnect them
			p.config.Logger.Warn("Client write timeout (pre-keyframe), disconnecting slow client",
				slog.String("client_id", clientID),
				slog.String("processor_id", p.id),
				slog.Uint64("bytes_written", client.bytesWritten.Load()),
				slog.Duration("connected_duration", time.Since(client.startedAt)),
				slog.Int("pending_write_bytes", len(data)))
			p.UnregisterClient(clientID)
			continue
		}
		if flusher != nil {
			flusher.Flush()
		}
		client.bytesWritten.Add(uint64(len(data)))
		p.UpdateClientBytes(clientID, uint64(len(data)))
	}

	p.RecordBytesWritten(uint64(len(data)))
}

// broadcastToClients sends data to all connected streaming clients.
// New clients wait for the next keyframe before receiving data.
// This ensures they can start decoding immediately.
// IMPORTANT: This method does NOT hold locks during HTTP I/O to prevent blocking.
func (p *MPEGTSProcessor) broadcastToClients(data []byte, hasKeyframe bool) {
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
			// Client is done, skip
			continue
		default:
		}

		// Check and update waitForKeyframe state under lock
		client.mu.Lock()
		shouldSkip := false
		if client.waitForKeyframe {
			if hasKeyframe {
				// Keyframe found - start sending data to this client
				client.waitForKeyframe = false
				p.config.Logger.Debug("Client starting at keyframe",
					slog.String("client_id", client.id))
			} else {
				// No keyframe yet - skip this client
				shouldSkip = true
			}
		}
		// Get references we need for I/O
		writer := client.writer
		flusher := client.flusher
		clientID := client.id
		client.mu.Unlock()

		if shouldSkip {
			continue
		}

		// Perform HTTP I/O WITHOUT holding any locks
		// Use a channel-based timeout to prevent blocking the processing loop
		writeDone := make(chan error, 1)
		go func() {
			_, err := writer.Write(data)
			writeDone <- err
		}()

		// Wait for write to complete with timeout
		select {
		case err := <-writeDone:
			if err != nil {
				// Client write failed, unregister
				p.config.Logger.Debug("Client write failed",
					slog.String("client_id", clientID),
					slog.String("error", err.Error()))
				p.UnregisterClient(clientID)
				continue
			}
		case <-time.After(5 * time.Second):
			// Write timed out - client is too slow, disconnect them
			p.config.Logger.Warn("Client write timeout, disconnecting slow client",
				slog.String("client_id", clientID),
				slog.String("processor_id", p.id),
				slog.Uint64("bytes_written", client.bytesWritten.Load()),
				slog.Duration("connected_duration", time.Since(client.startedAt)),
				slog.Int("pending_write_bytes", len(data)))
			p.UnregisterClient(clientID)
			continue
		}
		if flusher != nil {
			flusher.Flush()
		}

		// Update stats atomically (no lock needed)
		client.bytesWritten.Add(uint64(len(data)))

		// Sync bytes to base processor client for stats reporting
		p.UpdateClientBytes(clientID, uint64(len(data)))
	}

	// Update stats
	p.RecordBytesWritten(uint64(len(data)))
}

// GetStreamStats returns statistics for all connected stream clients.
func (p *MPEGTSProcessor) GetStreamStats() []StreamClientStats {
	p.streamClientsMu.RLock()
	defer p.streamClientsMu.RUnlock()

	stats := make([]StreamClientStats, 0, len(p.streamClients))
	for _, client := range p.streamClients {
		// bytesWritten is atomic, no lock needed for reading it
		stats = append(stats, StreamClientStats{
			ID:           client.id,
			BytesWritten: client.bytesWritten.Load(),
			StartedAt:    client.startedAt,
			Duration:     time.Since(client.startedAt),
		})
	}
	return stats
}

// StreamClientStats contains statistics for a stream client.
type StreamClientStats struct {
	ID           string
	BytesWritten uint64
	StartedAt    time.Time
	Duration     time.Duration
}

// SupportsStreaming returns true since this is a streaming processor.
func (p *MPEGTSProcessor) SupportsStreaming() bool {
	return true
}
