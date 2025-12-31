package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/codec"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/version"
)

// Variant cleanup configuration
const (
	// VariantCleanupInterval is how often to check for unused variants
	VariantCleanupInterval = 30 * time.Second

	// VariantIdleTimeout is how long a variant can be unused before cleanup
	VariantIdleTimeout = 60 * time.Second
)

// Resource history configuration
const (
	// ResourceHistorySize is the number of samples to keep for sparklines
	ResourceHistorySize = 30

	// ResourceSampleInterval is how often to sample CPU/memory stats
	ResourceSampleInterval = 1 * time.Second
)

// ResourceHistory tracks historical CPU and memory usage for sparklines.
type ResourceHistory struct {
	mu           sync.RWMutex
	cpuHistory   []float64 // Ring buffer of CPU percentages
	memHistory   []float64 // Ring buffer of memory MB values
	writeIndex   int       // Current write position in ring buffer
	sampleCount  int       // Total samples written (for partial buffer)
	lastSampleAt time.Time // Last sample time
}

// NewResourceHistory creates a new resource history tracker.
func NewResourceHistory() *ResourceHistory {
	return &ResourceHistory{
		cpuHistory: make([]float64, ResourceHistorySize),
		memHistory: make([]float64, ResourceHistorySize),
	}
}

// AddSample adds a CPU/memory sample to the history.
func (h *ResourceHistory) AddSample(cpuPercent, memoryMB float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cpuHistory[h.writeIndex] = cpuPercent
	h.memHistory[h.writeIndex] = memoryMB
	h.writeIndex = (h.writeIndex + 1) % ResourceHistorySize
	if h.sampleCount < ResourceHistorySize {
		h.sampleCount++
	}
	h.lastSampleAt = time.Now()
}

// GetHistory returns the CPU and memory history in chronological order.
func (h *ResourceHistory) GetHistory() (cpuHistory, memHistory []float64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.sampleCount == 0 {
		return nil, nil
	}

	// Calculate the actual number of samples to return
	count := h.sampleCount
	if count > ResourceHistorySize {
		count = ResourceHistorySize
	}

	cpuHistory = make([]float64, count)
	memHistory = make([]float64, count)

	// Read from oldest to newest
	if h.sampleCount >= ResourceHistorySize {
		// Buffer is full, read from writeIndex (oldest) forward
		for i := 0; i < count; i++ {
			idx := (h.writeIndex + i) % ResourceHistorySize
			cpuHistory[i] = h.cpuHistory[idx]
			memHistory[i] = h.memHistory[idx]
		}
	} else {
		// Buffer is not full, read from start
		for i := 0; i < count; i++ {
			cpuHistory[i] = h.cpuHistory[i]
			memHistory[i] = h.memHistory[i]
		}
	}

	return cpuHistory, memHistory
}

// ShouldSample returns true if enough time has passed to take a new sample.
func (h *ResourceHistory) ShouldSample() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return time.Since(h.lastSampleAt) >= ResourceSampleInterval
}

// ProcessorConfig holds configuration for on-demand processor creation.
type ProcessorConfig struct {
	TargetVariant         CodecVariant
	TargetSegmentDuration float64
	MaxSegments           int
	PlaylistSegments      int // Number of segments in playlist for new clients (near live edge)
}

// RelaySession represents an active relay session.
type RelaySession struct {
	ID                         models.ULID
	ChannelID                  models.ULID
	ChannelName                string      // Display name of the channel
	SourceID                   models.ULID // ID of the stream source (for connection tracking)
	StreamSourceName           string      // Name of the stream source (e.g., "s8k")
	StreamURL                  string
	SourceMaxConcurrentStreams int // Max concurrent streams for the source (0 = unlimited)
	EncodingProfile            *models.EncodingProfile
	Classification             ClassificationResult
	CachedCodecInfo            *models.LastKnownCodec // Pre-probed codec info for faster startup
	StartedAt                  time.Time

	// Use atomic values for frequently updated fields to avoid mutex contention
	// These are updated by the ingest loop on every read, which would block stats collection
	lastActivity atomic.Value // time.Time - last activity timestamp
	idleSince    atomic.Value // time.Time - when session became idle (zero if not idle)

	// Session state - atomic to avoid lock contention with stats collection
	closed          atomic.Bool           // Whether session is closed
	inFallback      atomic.Bool           // Whether session is in fallback mode
	ingestCompleted atomic.Bool           // Whether origin ingest has finished (EOF received)
	lastErr         atomic.Pointer[error] // Last error (if any)

	// Resource history for sparklines (CPU/memory over time)
	resourceHistory *ResourceHistory

	// Per-edge bandwidth tracking for flow visualization
	edgeBandwidth *EdgeBandwidthTrackers

	manager            *Manager
	ctx                context.Context
	cancel             context.CancelFunc
	fallbackController *FallbackController
	fallbackGenerator  *FallbackGenerator

	// Multi-format streaming support
	formatRouter    *FormatRouter          // Routes requests to appropriate output handler
	containerFormat models.ContainerFormat // Current container format

	// Elementary stream based processing (multi-variant codec support)
	esBuffer        *SharedESBuffer  // Shared ES buffer for multi-variant codec processing
	tsDemuxer       *TSDemuxer       // MPEG-TS demuxer for ES extraction
	processorConfig *ProcessorConfig // Config for on-demand processor creation

	// Per-variant processors (keyed by CodecVariant for multi-client codec support)
	// This allows different clients to receive different codec variants from the same session
	// Using sync.Map-backed types for lock-free concurrent access (avoids mutex contention)
	hlsTSProcessors   HLSTSProcessorMap   // HLS-TS processors per variant
	hlsFMP4Processors HLSfMP4ProcessorMap // HLS-fMP4 processors per variant
	dashProcessors    DASHProcessorMap    // DASH processors per variant
	mpegtsProcessors  MPEGTSProcessorMap  // MPEG-TS processors per variant
	esTranscoders     []Transcoder        // Active transcoders for codec variants (interface)
	esTranscodersMu   sync.RWMutex
	transcoderFactory *TranscoderFactory // Factory for creating transcoders

	// Legacy fields - set once during pipeline init, read-only afterward
	// Protected by readyCh synchronization (readers wait for ready before accessing)
	ffmpegCmd     *ffmpeg.Command // Running FFmpeg command for stats access
	hlsCollapser  *HLSCollapser
	hlsRepackager *HLSRepackager // HLS-to-HLS repackaging (container format change)
	inputReader   io.ReadCloser

	// Pipeline readiness signaling
	readyCh   chan struct{} // Closed when pipeline is ready for clients
	readyOnce sync.Once     // Ensures readyCh is closed only once
}

// start begins the relay session.
func (s *RelaySession) start(ctx context.Context) error {
	// Acquire connection slot
	release, err := s.manager.connectionPool.Acquire(ctx, s.StreamURL)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}

	// Start appropriate pipeline based on classification and profile
	go func() {
		defer release()
		s.runPipeline()
	}()

	return nil
}

// runPipeline runs the relay pipeline with fallback support.
func (s *RelaySession) runPipeline() {
	var err error
	defer func() {
		if err != nil {
			s.lastErr.Store(&err)
			// Only mark closed on error - successful completion should let cleanup
			// handle session lifecycle based on client count and idle timeout
			s.closed.Store(true)
		}
		// NOTE: Don't set closed=true on success! The ingest may have completed
		// (e.g., fixed-length source stream), but clients may still be connected
		// waiting for transcoded data. Let the cleanup logic handle session closure
		// based on client count and idle timeout.
	}()

	for {
		// Check if we're in fallback mode
		if s.inFallback.Load() {
			err = s.runFallbackStream()
			if err != nil && !errors.Is(err, context.Canceled) {
				// Fallback stream failed, exit pipeline
				return
			}
			// If we exit fallback cleanly, it means we recovered - continue loop
			continue
		}

		// Run the normal pipeline
		err = s.runNormalPipeline()

		// Check for cancellation
		if errors.Is(err, context.Canceled) {
			// Context cancellation means session was explicitly closed
			s.closed.Store(true)
			return
		}

		// If error occurred and fallback is configured, switch to fallback
		if err != nil && s.fallbackController != nil {
			if s.fallbackController.CheckError(err.Error()) {
				s.inFallback.Store(true)
				continue // Start fallback stream
			}
		}

		// No error or no fallback configured - exit
		if err != nil {
			cb := s.manager.circuitBreakers.Get(s.StreamURL)
			cb.RecordFailure()
		}
		return
	}
}

// runNormalPipeline runs the normal (non-fallback) pipeline based on profile/classification.
// All pipelines use the ES pipeline architecture:
// 1. Ingest source stream to SharedESBuffer (source variant)
// 2. Processors request the target codec variant from the profile
// 3. If target differs from source, on-demand FFmpeg transcoder is spawned
// 4. Transcoder reads from source variant, writes to target variant
// 5. Processors read from appropriate variant
func (s *RelaySession) runNormalPipeline() error {
	// Log the pipeline decision with all relevant context
	s.logPipelineDecision()

	// Handle special source format cases
	switch s.Classification.Mode {
	case StreamModeCollapsedHLS:
		return s.runHLSCollapsePipeline()
	case StreamModePassthroughHLS, StreamModeTransparentHLS, StreamModePassthroughDASH:
		// All HLS/DASH streams go through ES pipeline for connection sharing
		// ES pipeline handles both passthrough (copy) and transcoding modes
		return s.runESPipeline()
	default:
		// Raw MPEG-TS or unknown - use ES pipeline for all processing
		// The ES pipeline handles both passthrough and on-demand transcoding
		return s.runESPipeline()
	}
}

// logPipelineDecision logs an INFO message summarizing the pipeline decision and context.
func (s *RelaySession) logPipelineDecision() {
	// Determine pipeline type and reason based on classification
	pipelineType := "es"
	reason := "default"

	switch s.Classification.Mode {
	case StreamModeCollapsedHLS:
		pipelineType = "hls-collapse"
		reason = "collapsing HLS to MPEG-TS"
	case StreamModePassthroughHLS, StreamModeTransparentHLS:
		if s.EncodingProfile != nil && s.EncodingProfile.NeedsTranscode() {
			reason = "HLS source requires transcoding"
		} else {
			reason = "HLS source with copy mode"
		}
	case StreamModePassthroughDASH:
		if s.EncodingProfile != nil && s.EncodingProfile.NeedsTranscode() {
			reason = "DASH source requires transcoding"
		} else {
			reason = "DASH source with copy mode"
		}
	default:
		reason = "raw MPEG-TS source"
	}

	// Build log fields
	logFields := []any{
		slog.String("session_id", s.ID.String()),
		slog.String("channel", s.ChannelName),
		slog.String("pipeline", pipelineType),
		slog.String("reason", reason),
		slog.String("source_format", string(s.Classification.SourceFormat)),
		slog.String("stream_mode", s.Classification.Mode.String()),
	}

	// Add codec info if available
	if s.CachedCodecInfo != nil {
		logFields = append(logFields,
			slog.String("source_video", s.CachedCodecInfo.VideoCodec),
			slog.String("source_audio", s.CachedCodecInfo.AudioCodec),
			slog.String("resolution", fmt.Sprintf("%dx%d", s.CachedCodecInfo.VideoWidth, s.CachedCodecInfo.VideoHeight)),
		)
	}

	// Add profile info if available
	if s.EncodingProfile != nil {
		logFields = append(logFields,
			slog.String("profile", s.EncodingProfile.Name),
			slog.Bool("needs_transcode", s.EncodingProfile.NeedsTranscode()),
		)
		if s.EncodingProfile.NeedsTranscode() {
			logFields = append(logFields,
				slog.String("target_video", string(s.EncodingProfile.TargetVideoCodec)),
				slog.String("target_audio", string(s.EncodingProfile.TargetAudioCodec)),
			)
		}
	}

	slog.Info("Starting relay pipeline", logFields...)
}

// runFallbackStream runs the fallback stream until recovery or cancellation.
// Note: Fallback now streams directly through the ES pipeline's MPEG-TS processor
func (s *RelaySession) runFallbackStream() error {
	if s.fallbackGenerator == nil || !s.fallbackGenerator.IsReady() {
		return ErrFallbackNotReady
	}

	// Start a recovery check goroutine
	recoveryDone := make(chan bool, 1)
	go s.runRecoveryLoop(recoveryDone)

	// For fallback, we wait until recovery or context cancellation
	// The ES pipeline processors handle serving the fallback content
	<-s.ctx.Done()

	// Signal recovery goroutine to stop
	close(recoveryDone)

	return s.ctx.Err()
}

// runRecoveryLoop periodically checks if upstream has recovered.
func (s *RelaySession) runRecoveryLoop(done <-chan bool) {
	if s.fallbackController == nil {
		return
	}

	for {
		select {
		case <-done:
			return
		case <-s.ctx.Done():
			return
		default:
		}

		// Check if it's time to attempt recovery
		if !s.fallbackController.ShouldAttemptRecovery() {
			// Sleep before next check
			select {
			case <-done:
				return
			case <-s.ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}

		// Attempt recovery
		s.fallbackController.StartRecoveryAttempt()

		if s.testUpstreamRecovery() {
			// Upstream recovered - exit fallback mode
			s.fallbackController.RecoverySucceeded()
			s.inFallback.Store(false)

			// Cancel current context to stop the fallback stream
			// The runFallbackStream will return and runPipeline will restart normal pipeline
			s.cancel()

			// Create a new context for the resumed pipeline
			s.ctx, s.cancel = context.WithCancel(s.manager.ctx)
			return
		}

		s.fallbackController.RecoveryFailed()
	}
}

// testUpstreamRecovery tests if the upstream is available again.
func (s *RelaySession) testUpstreamRecovery() bool {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.StreamURL, nil)
	if err != nil {
		return false
	}

	resp, err := s.manager.config.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// NOTE: runFFmpegPipeline has been removed - FFmpeg transcoding now happens on-demand
// via handleVariantRequest when processors request codec variants that differ from source.
// This is the correct ES pipeline pattern:
// 1. Origin -> TSDemuxer -> SharedESBuffer (source variant)
// 2. Processors request target variant from profile
// 3. If target != source, a transcoder is spawned to read from source and write to target
// 4. Processors read from appropriate variant

// runHLSCollapsePipeline runs the HLS collapsing pipeline.
// This uses the ES pipeline to demux collapsed HLS content and remux for output.
func (s *RelaySession) runHLSCollapsePipeline() error {
	playlistURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		playlistURL = s.Classification.SelectedMediaPlaylist
	}

	collapser := NewHLSCollapser(s.manager.config.HTTPClient, playlistURL)
	s.hlsCollapser = collapser

	if err := collapser.Start(s.ctx); err != nil {
		return fmt.Errorf("starting HLS collapser: %w", err)
	}

	// Initialize the ES pipeline to process collapsed HLS content
	// Use buffer config from the manager if available, otherwise defaults
	esConfig := s.manager.config.BufferConfig
	esConfig.Logger = slog.Default()
	// Pass codec hints from probing to pre-create source variant with both codecs
	if s.CachedCodecInfo != nil {
		esConfig.ExpectedVideoCodec = s.CachedCodecInfo.VideoCodec
		esConfig.ExpectedVideoProfile = s.CachedCodecInfo.VideoProfile
		esConfig.ExpectedVideoWidth = s.CachedCodecInfo.VideoWidth
		esConfig.ExpectedVideoHeight = s.CachedCodecInfo.VideoHeight
		esConfig.ExpectedFramerate = s.CachedCodecInfo.VideoFramerate
		esConfig.ExpectedVideoBitrate = s.CachedCodecInfo.VideoBitrate
		esConfig.ExpectedAudioCodec = s.CachedCodecInfo.AudioCodec
		esConfig.ExpectedAudioChannels = s.CachedCodecInfo.AudioChannels
		esConfig.ExpectedAudioSampleRate = s.CachedCodecInfo.AudioSampleRate
		esConfig.ExpectedAudioBitrate = s.CachedCodecInfo.AudioBitrate
		esConfig.ExpectedContainer = s.CachedCodecInfo.ContainerFormat
		esConfig.ExpectedIsLive = s.CachedCodecInfo.IsLiveStream
	}
	s.esBuffer = NewSharedESBuffer(s.ChannelID.String(), s.ID.String(), esConfig)

	// Set up the transcoding callback
	s.esBuffer.SetVariantRequestCallback(s.handleVariantRequest)

	// Create TS demuxer to parse incoming MPEG-TS from collapser
	demuxerConfig := TSDemuxerConfig{
		Logger: slog.Default(),
	}
	// Use probed audio codec as fallback for codecs not natively supported by demuxer (e.g., E-AC3)
	if s.CachedCodecInfo != nil && s.CachedCodecInfo.AudioCodec != "" {
		demuxerConfig.ProbeOverrideAudioCodec = s.CachedCodecInfo.AudioCodec
	}
	s.tsDemuxer = NewTSDemuxer(s.esBuffer, demuxerConfig)

	// Use HLS config from manager for segment settings
	targetSegmentDuration := s.manager.config.HLSConfig.TargetSegmentDuration
	maxSegments := s.manager.config.HLSConfig.MaxSegments
	playlistSegments := s.manager.config.HLSConfig.PlaylistSegments

	// No need to initialize processor maps - sync.Map is zero-value safe

	// Start processors for the default variant (VariantCopy for passthrough)
	hlsTSConfig := DefaultHLSTSProcessorConfig()
	hlsTSConfig.Logger = slog.Default()
	hlsTSConfig.TargetSegmentDuration = targetSegmentDuration
	hlsTSConfig.MaxSegments = maxSegments
	hlsTSConfig.PlaylistSegments = playlistSegments

	hlsTSProcessor := NewHLSTSProcessor(
		fmt.Sprintf("hls-ts-%s-%s", s.ID.String(), VariantCopy.String()),
		s.esBuffer,
		VariantCopy,
		hlsTSConfig,
	)
	if err := hlsTSProcessor.Start(s.ctx); err != nil {
		return fmt.Errorf("starting HLS-TS processor: %w", err)
	}
	s.configureProcessorStreamContext(hlsTSProcessor.BaseProcessor)
	hlsTSProcessor.SetBandwidthTracker(s.edgeBandwidth.GetOrCreateProcessorTracker("hls"))
	s.hlsTSProcessors.Store(VariantCopy, hlsTSProcessor)

	// Start MPEG-TS processor for raw streaming
	mpegtsConfig := DefaultMPEGTSProcessorConfig()
	mpegtsConfig.Logger = slog.Default()
	mpegtsProcessor := NewMPEGTSProcessor(
		fmt.Sprintf("mpegts-%s-%s", s.ID.String(), VariantCopy.String()),
		s.esBuffer,
		VariantCopy,
		mpegtsConfig,
	)
	if err := mpegtsProcessor.Start(s.ctx); err != nil {
		slog.Warn("Failed to start MPEG-TS processor", slog.String("error", err.Error()))
	} else {
		s.configureProcessorStreamContext(mpegtsProcessor.BaseProcessor)
		mpegtsProcessor.SetBandwidthTracker(s.edgeBandwidth.GetOrCreateProcessorTracker("mpegts"))
		s.mpegtsProcessors.Store(VariantCopy, mpegtsProcessor)
	}

	// Set up format router
	s.formatRouter = NewFormatRouter(models.ContainerFormatMPEGTS)
	s.formatRouter.RegisterHandler(FormatValueHLS, NewHLSHandler(hlsTSProcessor))
	if mpegtsProcessor != nil {
		s.formatRouter.RegisterHandler(FormatValueMPEGTS, NewMPEGTSHandler(mpegtsProcessor))
	}

	// Signal that the pipeline is ready for clients
	s.markReady()

	slog.Debug("Started HLS collapse pipeline with ES processing",
		slog.String("session_id", s.ID.String()),
		slog.String("playlist_url", playlistURL))

	// Read from collapser and feed to demuxer
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-s.ctx.Done():
			collapser.Stop()
			s.tsDemuxer.Flush()
			return s.ctx.Err()
		default:
		}

		n, err := collapser.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, ErrCollapserAborted) {
				s.tsDemuxer.Flush()
				return nil
			}
			return err
		}

		if n > 0 {
			// Track bytes ingested from origin (HLS collapse pipeline)
			s.edgeBandwidth.OriginToBuffer.Add(uint64(n))

			if err := s.tsDemuxer.Write(buf[:n]); err != nil {
				slog.Warn("Demuxer error", slog.String("error", err.Error()))
			}
			// Use atomic store to avoid blocking stats collection with mutex
			s.lastActivity.Store(time.Now())
		}
	}
}

// getTargetVariant returns the target codec variant based on the profile settings.
// If the profile specifies transcoding, this returns the target variant (e.g., "h264/aac").
// If no transcoding is needed, this returns VariantCopy which means use source codecs.
// When processors request a variant that differs from the source, the on-demand
// transcoding callback (handleVariantRequest) will spawn a transcoder.
//
// When a codec is set to "copy", empty, or "none", this function resolves it to the
// actual source codec from CachedCodecInfo, so the variant name reflects the real
// codec being used (e.g., "h265/aac" instead of "h265/copy").
func (s *RelaySession) getTargetVariant() CodecVariant {
	if s.EncodingProfile == nil || !s.EncodingProfile.NeedsTranscode() {
		return VariantCopy
	}

	// Build target variant from profile codecs
	videoCodec := string(s.EncodingProfile.TargetVideoCodec)
	audioCodec := string(s.EncodingProfile.TargetAudioCodec)

	// EncodingProfile doesn't have "auto" codecs - it always has concrete target codecs
	// If empty, treat as copy
	if videoCodec == "" {
		videoCodec = "copy"
	}
	if audioCodec == "" {
		audioCodec = "copy"
	}

	// Normalize: "copy", empty, and "none" all mean keep source codec
	if videoCodec == "" || videoCodec == "copy" || videoCodec == "none" {
		videoCodec = "copy"
	}
	if audioCodec == "" || audioCodec == "copy" || audioCodec == "none" {
		audioCodec = "copy"
	}

	// If both are copy, return VariantCopy to avoid triggering transcoder
	if videoCodec == "copy" && audioCodec == "copy" {
		return VariantCopy
	}

	// Resolve "copy" to actual source codec from CachedCodecInfo
	// This ensures the variant name reflects the real codec (e.g., "h265/aac" not "h265/copy")
	if s.CachedCodecInfo != nil {
		if videoCodec == "copy" && s.CachedCodecInfo.VideoCodec != "" {
			videoCodec = s.CachedCodecInfo.VideoCodec
		}
		if audioCodec == "copy" && s.CachedCodecInfo.AudioCodec != "" {
			audioCodec = s.CachedCodecInfo.AudioCodec
		}
	}

	variant := NewCodecVariant(videoCodec, audioCodec)
	slog.Debug("getTargetVariant result",
		slog.String("session_id", s.ID.String()),
		slog.String("resolved_variant", variant.String()))
	return variant
}

// runESPipeline runs the elementary stream based pipeline.
// This uses SharedESBuffer for multi-variant codec support, enabling:
// - Single upstream connection with multiple output format/codec variants
// - On-demand transcoding when clients request different codecs
// - Efficient ES-level buffering with MPEG-TS/fMP4 muxing at output
//
// Pipeline flow:
// 1. Source stream → TSDemuxer → SharedESBuffer (source variant, "copy/copy")
// 2. Processors request target variant from profile (e.g., "h264/aac")
// 3. If target != source, handleVariantRequest spawns a transcoder
// 4. Transcoder reads from source variant, writes to target variant
// 5. Processors read from target variant
func (s *RelaySession) runESPipeline() error {
	// Initialize the shared ES buffer
	// Use buffer config from the manager if available, otherwise defaults
	esConfig := s.manager.config.BufferConfig
	esConfig.Logger = slog.Default()
	// Pass codec hints from probing to pre-create source variant with both codecs
	if s.CachedCodecInfo != nil {
		esConfig.ExpectedVideoCodec = s.CachedCodecInfo.VideoCodec
		esConfig.ExpectedVideoProfile = s.CachedCodecInfo.VideoProfile
		esConfig.ExpectedVideoWidth = s.CachedCodecInfo.VideoWidth
		esConfig.ExpectedVideoHeight = s.CachedCodecInfo.VideoHeight
		esConfig.ExpectedFramerate = s.CachedCodecInfo.VideoFramerate
		esConfig.ExpectedVideoBitrate = s.CachedCodecInfo.VideoBitrate
		esConfig.ExpectedAudioCodec = s.CachedCodecInfo.AudioCodec
		esConfig.ExpectedAudioChannels = s.CachedCodecInfo.AudioChannels
		esConfig.ExpectedAudioSampleRate = s.CachedCodecInfo.AudioSampleRate
		esConfig.ExpectedAudioBitrate = s.CachedCodecInfo.AudioBitrate
		esConfig.ExpectedContainer = s.CachedCodecInfo.ContainerFormat
		esConfig.ExpectedIsLive = s.CachedCodecInfo.IsLiveStream
	}
	s.esBuffer = NewSharedESBuffer(s.ChannelID.String(), s.ID.String(), esConfig)

	// Set up the transcoding callback - spawns FFmpeg transcoder when new codec variant is requested
	s.esBuffer.SetVariantRequestCallback(s.handleVariantRequest)

	// Create TS demuxer to parse incoming MPEG-TS and populate ES buffer
	// The demuxer writes to the source variant (VariantCopy)
	demuxerConfig := TSDemuxerConfig{
		Logger: slog.Default(),
		// No TargetVariant - writes to source variant ("copy/copy")
	}
	// Use probed audio codec as fallback for codecs not natively supported by demuxer (e.g., E-AC3)
	if s.CachedCodecInfo != nil && s.CachedCodecInfo.AudioCodec != "" {
		demuxerConfig.ProbeOverrideAudioCodec = s.CachedCodecInfo.AudioCodec
	}
	s.tsDemuxer = NewTSDemuxer(s.esBuffer, demuxerConfig)

	// Determine the source URL
	inputURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		inputURL = s.Classification.SelectedMediaPlaylist
	}

	// Use HLS config from manager for segment settings
	targetSegmentDuration := s.manager.config.HLSConfig.TargetSegmentDuration
	maxSegments := s.manager.config.HLSConfig.MaxSegments
	playlistSegments := s.manager.config.HLSConfig.PlaylistSegments

	slog.Debug("Starting ES pipeline",
		slog.String("session_id", s.ID.String()),
		slog.Bool("needs_transcode", s.EncodingProfile != nil && s.EncodingProfile.NeedsTranscode()))

	// Start ingest in a goroutine FIRST - this feeds data to the demuxer
	// which will detect codecs and create the source variant
	ingestErrCh := make(chan error, 1)
	go func() {
		ingestErrCh <- s.runIngestLoop(inputURL, s.tsDemuxer)
	}()

	// Wait for EITHER the source variant to be ready OR ingest to fail
	// This avoids waiting the full timeout if upstream fails quickly (e.g., 404)
	sourceReadyCtx, sourceReadyCancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer sourceReadyCancel()

	slog.Debug("Waiting for source variant to be ready",
		slog.String("session_id", s.ID.String()))

	// Use a goroutine to convert WaitSourceVariant into a channel
	sourceReadyCh := make(chan error, 1)
	go func() {
		sourceReadyCh <- s.esBuffer.WaitSourceVariant(sourceReadyCtx)
	}()

	// Helper to drain channels in background to prevent goroutine leaks
	// The writing goroutines will eventually complete and need somewhere to send
	drainChannels := func() {
		go func() { <-ingestErrCh }()
		go func() { <-sourceReadyCh }()
	}

	// Wait for either source ready or ingest error
	select {
	case err := <-sourceReadyCh:
		if err != nil {
			// Source wait failed - check if ingest also failed for better error message
			select {
			case ingestErr := <-ingestErrCh:
				if ingestErr != nil {
					return fmt.Errorf("ingest failed before source detection: %w", ingestErr)
				}
			default:
				// Drain ingestErrCh in background since we're returning
				go func() { <-ingestErrCh }()
			}
			return fmt.Errorf("waiting for source variant: %w", err)
		}
		// Source is ready, continue (ingestErrCh will be read at end of pipeline)
	case ingestErr := <-ingestErrCh:
		// Ingest failed before source was detected
		// sourceReadyCh will complete soon due to deferred sourceReadyCancel()
		go func() { <-sourceReadyCh }()
		if ingestErr != nil {
			return fmt.Errorf("ingest failed: %w", ingestErr)
		}
		// Ingest completed without error but source not ready - shouldn't happen
		return fmt.Errorf("ingest completed without detecting source codecs")
	case <-s.ctx.Done():
		drainChannels()
		return s.ctx.Err()
	}

	slog.Debug("Source variant ready, pipeline initialized",
		slog.String("session_id", s.ID.String()),
		slog.String("source_variant", s.esBuffer.SourceVariantKey().String()))

	// Get the target variant from profile - NOW that source codecs are known
	// "copy" will be resolved to actual source codec (e.g., "av1/aac" not "av1/copy")
	targetVariant := s.getTargetVariant()

	slog.Debug("Target variant resolved",
		slog.String("session_id", s.ID.String()),
		slog.String("target_variant", targetVariant.String()))

	// Store processor config for on-demand creation
	// Processors are NOT created here - they are created on-demand when clients connect
	s.processorConfig = &ProcessorConfig{
		TargetVariant:         targetVariant,
		TargetSegmentDuration: targetSegmentDuration,
		MaxSegments:           maxSegments,
		PlaylistSegments:      playlistSegments,
	}

	// Set up format router WITHOUT pre-created processors
	// Processors will be created on-demand via GetOrCreateProcessor()
	s.formatRouter = NewFormatRouter(models.ContainerFormatMPEGTS)

	// Signal that the pipeline is ready for clients
	// Clients will trigger on-demand processor creation when they connect
	s.markReady()

	slog.Debug("Started ES-based pipeline (processors created on-demand)",
		slog.String("session_id", s.ID.String()),
		slog.String("source_url", inputURL),
		slog.String("target_variant", targetVariant.String()))

	// Start variant cleanup loop to remove unused transcoded variants
	go s.runVariantCleanupLoop()

	// Wait for ingest to complete (or fail)
	// Note: ingestErrCh may have already been consumed if ingest failed before source detection
	select {
	case err := <-ingestErrCh:
		return err
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// runIngestLoop fetches upstream MPEG-TS and feeds it to the demuxer.
// This runs in a goroutine and populates the SharedESBuffer with elementary streams.
func (s *RelaySession) runIngestLoop(inputURL string, demuxer *TSDemuxer) error {
	slog.Debug("Ingest loop starting",
		slog.String("session_id", s.ID.String()),
		slog.String("url", inputURL))

	// Fetch upstream MPEG-TS and feed to demuxer
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, inputURL, nil)
	if err != nil {
		slog.Error("Ingest loop: failed to create request",
			slog.String("session_id", s.ID.String()),
			slog.String("error", err.Error()))
		return err
	}

	resp, err := s.manager.config.HTTPClient.Do(req)
	if err != nil {
		slog.Error("Ingest loop: HTTP request failed",
			slog.String("session_id", s.ID.String()),
			slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Ingest loop: upstream returned non-200 status",
			slog.String("session_id", s.ID.String()),
			slog.Int("status", resp.StatusCode))
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	// Log upstream response headers for debugging stream termination issues
	contentLength := resp.Header.Get("Content-Length")
	contentType := resp.Header.Get("Content-Type")
	transferEncoding := resp.Header.Get("Transfer-Encoding")
	connection := resp.Header.Get("Connection")
	slog.Debug("Upstream response received",
		slog.String("session_id", s.ID.String()),
		slog.String("url", inputURL),
		slog.Int("status", resp.StatusCode),
		slog.String("content_length", contentLength),
		slog.String("content_type", contentType),
		slog.String("transfer_encoding", transferEncoding),
		slog.String("connection", connection),
		slog.Int64("content_length_parsed", resp.ContentLength))

	// Warn if Content-Length is set - this indicates a finite stream, not live
	if resp.ContentLength > 0 {
		slog.Warn("Upstream sent Content-Length header - stream may be finite, not live",
			slog.String("session_id", s.ID.String()),
			slog.Int64("content_length", resp.ContentLength))
	}

	s.inputReader = resp.Body

	// Update last activity using atomic to avoid blocking stats collection
	s.lastActivity.Store(time.Now())

	// Read from upstream and feed to demuxer
	buf := make([]byte, 64*1024)
	var totalBytesRead uint64
	var lastLogBytes uint64
	startTime := time.Now()
	lastLogTime := startTime

	// Throughput logging ticker
	throughputTicker := time.NewTicker(5 * time.Second)
	defer throughputTicker.Stop()

	slog.Debug("Ingest loop: starting read loop",
		slog.String("session_id", s.ID.String()))

	for {
		select {
		case <-s.ctx.Done():
			slog.Debug("Ingest loop: context cancelled",
				slog.String("session_id", s.ID.String()),
				slog.Uint64("total_bytes_read", totalBytesRead),
				slog.Duration("duration", time.Since(startTime)),
				slog.String("context_err", s.ctx.Err().Error()))
			demuxer.Flush()
			return s.ctx.Err()
		case <-throughputTicker.C:
			now := time.Now()
			elapsed := now.Sub(lastLogTime).Seconds()
			bytesDelta := totalBytesRead - lastLogBytes
			throughputKBps := float64(bytesDelta) / elapsed / 1024
			slog.Debug("Ingest throughput",
				slog.String("session_id", s.ID.String()),
				slog.Float64("kbps", throughputKBps),
				slog.Uint64("total_bytes", totalBytesRead))
			lastLogBytes = totalBytesRead
			lastLogTime = now
		default:
		}

		n, err := resp.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.Info("Ingest loop: upstream stream ended (EOF), origin disconnected",
					slog.String("session_id", s.ID.String()),
					slog.String("url", inputURL),
					slog.Uint64("total_bytes_read", totalBytesRead),
					slog.Duration("duration", time.Since(startTime)))
				s.ingestCompleted.Store(true)
				demuxer.Flush()
				// Signal ES buffer that source stream is complete
				// This triggers graceful shutdown of transcoders after they finish processing
				if s.esBuffer != nil {
					s.esBuffer.MarkSourceCompleted()
				}
				return nil
			}
			// Check if this is a context cancellation error
			if errors.Is(err, context.Canceled) {
				slog.Debug("Ingest loop: read cancelled by context",
					slog.String("session_id", s.ID.String()),
					slog.Uint64("total_bytes_read", totalBytesRead),
					slog.Duration("duration", time.Since(startTime)))
				demuxer.Flush()
				return err
			}
			slog.Warn("Ingest loop: upstream read error",
				slog.String("session_id", s.ID.String()),
				slog.String("error", err.Error()),
				slog.String("error_type", fmt.Sprintf("%T", err)),
				slog.Uint64("total_bytes_read", totalBytesRead),
				slog.Duration("duration", time.Since(startTime)))
			return err
		}

		totalBytesRead += uint64(n)

		if n > 0 {
			// Track bytes ingested from origin (direct MPEG-TS ingest loop)
			s.edgeBandwidth.OriginToBuffer.Add(uint64(n))

			if err := demuxer.Write(buf[:n]); err != nil {
				slog.Warn("Demuxer error",
					slog.String("error", err.Error()))
			}
			// Use atomic store to avoid blocking stats collection with mutex
			s.lastActivity.Store(time.Now())
		}
	}
}

// runVariantCleanupLoop periodically cleans up unused transcoded variants and their transcoders.
// This prevents memory leaks when clients stop requesting certain codec variants.
// It also checks HLS/DASH processors for playlist idle timeout (no playlist polls = client left).
func (s *RelaySession) runVariantCleanupLoop() {
	ticker := time.NewTicker(VariantCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupUnusedVariants()
			s.cleanupInactiveStreamingClients()
		}
	}
}

// DefaultClientIdleTimeout is the default timeout for client inactivity cleanup.
const DefaultClientIdleTimeout = 60 * time.Second

// cleanupInactiveStreamingClients cleans up inactive clients and stops idle processors.
// Each processor type implements IsIdle() according to its own semantics:
// - HLS-fMP4: playlist request timeout (clients poll for manifests)
// - HLS-TS/DASH/MPEG-TS: no connected clients
func (s *RelaySession) cleanupInactiveStreamingClients() {
	sessionID := s.ID.String()

	// Check HLS-fMP4 processors
	s.hlsFMP4Processors.Range(func(variant CodecVariant, processor *HLSfMP4Processor) bool {
		// Use playlist idle timeout for client cleanup since that's the polling interval
		clientTimeout := processor.PlaylistIdleTimeout()
		if removed := processor.CleanupInactiveClients(clientTimeout); removed > 0 {
			slog.Debug("Cleaned up inactive clients",
				slog.String("session_id", sessionID),
				slog.String("format", "hls-fmp4"),
				slog.String("variant", variant.String()),
				slog.Int("removed_clients", removed))
		}

		if processor.IsIdle() {
			slog.Debug("Processor is idle, scheduling cleanup",
				slog.String("session_id", sessionID),
				slog.String("format", "hls-fmp4"),
				slog.String("variant", variant.String()))
			go s.StopProcessorIfIdle("hls-fmp4")
		}
		return true
	})

	// Check HLS-TS processors
	s.hlsTSProcessors.Range(func(variant CodecVariant, processor *HLSTSProcessor) bool {
		if removed := processor.CleanupInactiveClients(DefaultClientIdleTimeout); removed > 0 {
			slog.Debug("Cleaned up inactive clients",
				slog.String("session_id", sessionID),
				slog.String("format", "hls-ts"),
				slog.String("variant", variant.String()),
				slog.Int("removed_clients", removed))
		}

		if processor.IsIdle() {
			slog.Debug("Processor is idle, scheduling cleanup",
				slog.String("session_id", sessionID),
				slog.String("format", "hls-ts"),
				slog.String("variant", variant.String()))
			go s.StopProcessorIfIdle("hls-ts")
		}
		return true
	})

	// Check DASH processors
	s.dashProcessors.Range(func(variant CodecVariant, processor *DASHProcessor) bool {
		if removed := processor.CleanupInactiveClients(DefaultClientIdleTimeout); removed > 0 {
			slog.Debug("Cleaned up inactive clients",
				slog.String("session_id", sessionID),
				slog.String("format", "dash"),
				slog.String("variant", variant.String()),
				slog.Int("removed_clients", removed))
		}

		if processor.IsIdle() {
			slog.Debug("Processor is idle, scheduling cleanup",
				slog.String("session_id", sessionID),
				slog.String("format", "dash"),
				slog.String("variant", variant.String()))
			go s.StopProcessorIfIdle("dash")
		}
		return true
	})

	// Update session idle state based on whether any processors remain active
	s.UpdateIdleState()
}

// cleanupUnusedVariants removes variants that haven't been accessed recently
// and stops their associated transcoders.
func (s *RelaySession) cleanupUnusedVariants() {
	if s.esBuffer == nil {
		return
	}

	// Get list of variants before cleanup
	variantsBefore := s.esBuffer.ListVariants()

	// Clean up unused variants from the buffer
	removed := s.esBuffer.CleanupUnusedVariants(VariantIdleTimeout)

	if removed > 0 {
		// Get list of variants after cleanup to determine which were removed
		variantsAfter := s.esBuffer.ListVariants()
		removedVariants := make(map[CodecVariant]bool)

		for _, v := range variantsBefore {
			found := false
			for _, va := range variantsAfter {
				if v == va {
					found = true
					break
				}
			}
			if !found {
				removedVariants[v] = true
			}
		}

		// Stop transcoders for removed variants
		s.stopTranscodersForVariants(removedVariants)

		slog.Debug("Cleaned up unused variants",
			slog.String("session_id", s.ID.String()),
			slog.Int("removed_count", removed))
	}
}

// stopTranscodersForVariants stops all transcoders producing the specified variants.
func (s *RelaySession) stopTranscodersForVariants(variants map[CodecVariant]bool) {
	s.esTranscodersMu.Lock()
	defer s.esTranscodersMu.Unlock()

	// Find and stop transcoders for removed variants
	var remaining []Transcoder
	for _, transcoder := range s.esTranscoders {
		stats := transcoder.Stats()
		if variants[stats.TargetVariant] {
			slog.Debug("Stopping transcoder for removed variant",
				slog.String("session_id", s.ID.String()),
				slog.String("target_variant", stats.TargetVariant.String()))
			transcoder.Stop()
		} else {
			remaining = append(remaining, transcoder)
		}
	}
	s.esTranscoders = remaining
}

// handleVariantRequest is called when a processor requests a codec variant that doesn't exist.
// It spawns a transcoder via ffmpegd (either remote daemon or local subprocess) to produce the requested variant.
func (s *RelaySession) handleVariantRequest(source, target CodecVariant) error {
	slog.Default().Log(context.Background(), observability.LevelTrace, "handleVariantRequest called",
		slog.String("session_id", s.ID.String()),
		slog.String("source_variant", source.String()),
		slog.String("target_variant", target.String()))

	// Check if a transcoder for this source+target already exists and is still running
	// This prevents duplicate transcoders for the same variant pair
	s.esTranscodersMu.Lock()
	for _, existing := range s.esTranscoders {
		stats := existing.Stats()
		if stats.SourceVariant == source && stats.TargetVariant == target {
			if !existing.IsClosed() {
				s.esTranscodersMu.Unlock()
				slog.Debug("Transcoder already exists for variant pair",
					slog.String("session_id", s.ID.String()),
					slog.String("source_variant", source.String()),
					slog.String("target_variant", target.String()),
					slog.String("transcoder_id", stats.ID))
				return nil // Transcoder already running, nothing to do
			}
			// Transcoder exists but is closed - will be cleaned up later
		}
	}
	s.esTranscodersMu.Unlock()

	// Initialize transcoder factory if needed
	if s.transcoderFactory == nil {
		// Create factory with daemon registry and stream/job managers for distributed transcoding
		// All transcoding is done via ffmpegd - either remote daemon or local subprocess
		s.transcoderFactory = NewTranscoderFactory(TranscoderFactoryConfig{
			Spawner:                  s.manager.FFmpegDSpawner(),
			DaemonRegistry:           s.manager.DaemonRegistry(),
			DaemonStreamManager:      s.manager.DaemonStreamManager(),
			ActiveJobManager:         s.manager.ActiveJobManager(),
			PreferRemote:             s.manager.PreferRemote(),
			EncoderOverridesProvider: s.manager.EncoderOverridesProvider(),
			Logger:                   slog.Default(),
		})
	}

	// Check if source audio/video codec can be demuxed by mediacommon
	// If not, use direct URL input mode (FFmpeg reads directly from source URL)
	sourceAudioCodec := source.AudioCodec()
	sourceVideoCodec := source.VideoCodec()
	useDirectInput := false
	sourceURL := s.StreamURL

	// Use selected media playlist if available (for HLS sources)
	if s.Classification.SelectedMediaPlaylist != "" {
		sourceURL = s.Classification.SelectedMediaPlaylist
	}

	// Also check CachedCodecInfo - the source variant may have been created before
	// audio was detected, so source.AudioCodec() may be empty even though we know the codec
	if sourceAudioCodec == "" && s.CachedCodecInfo != nil && s.CachedCodecInfo.AudioCodec != "" {
		sourceAudioCodec = s.CachedCodecInfo.AudioCodec
	}
	if sourceVideoCodec == "" && s.CachedCodecInfo != nil && s.CachedCodecInfo.VideoCodec != "" {
		sourceVideoCodec = s.CachedCodecInfo.VideoCodec
	}

	// Note: We keep the original source variant name (e.g., "h265/") even if we know the audio codec
	// The ESBuffer stores variants by their created name, and the source variant was created before
	// audio was detected. The transcoder will get the actual codec from the ESTrack, not the variant name.

	// Check audio codec demuxability
	if sourceAudioCodec != "" && !codec.IsAudioDemuxable(sourceAudioCodec) {
		slog.Info("Source audio codec not demuxable, using direct URL input",
			slog.String("session_id", s.ID.String()),
			slog.String("audio_codec", sourceAudioCodec),
			slog.String("source_url", sourceURL))
		useDirectInput = true
	}

	// Check video codec demuxability
	if sourceVideoCodec != "" && !codec.IsVideoDemuxable(sourceVideoCodec) {
		slog.Info("Source video codec not demuxable, using direct URL input",
			slog.String("session_id", s.ID.String()),
			slog.String("video_codec", sourceVideoCodec),
			slog.String("source_url", sourceURL))
		useDirectInput = true
	}

	// If useDirectInput is needed, check if opening another connection would breach
	// the source's max_concurrent_streams limit. Each useDirectInput transcoder needs
	// its own connection to the source URL.
	if useDirectInput && s.SourceMaxConcurrentStreams > 0 {
		// Count existing active sessions for this source
		currentConnections := s.manager.CountActiveSessionsForSource(s.SourceID)
		// +1 for the new FFmpeg connection (useDirectInput mode)
		if currentConnections+1 > s.SourceMaxConcurrentStreams {
			unsupportedCodec := ""
			if sourceAudioCodec != "" && !codec.IsAudioDemuxable(sourceAudioCodec) {
				unsupportedCodec = sourceAudioCodec
			} else if sourceVideoCodec != "" && !codec.IsVideoDemuxable(sourceVideoCodec) {
				unsupportedCodec = sourceVideoCodec
			}
			return fmt.Errorf("UseDirectInput would connect to the origin and breach max concurrent streams (current: %d, limit: %d). Codec %s is unsupported and cannot be demuxed",
				currentConnections, s.SourceMaxConcurrentStreams, unsupportedCodec)
		}
	}

	opts := CreateTranscoderOptions{
		SourceURL:      sourceURL,
		UseDirectInput: useDirectInput,
		ChannelName:    s.ChannelName,
	}

	var transcoder Transcoder
	var err error

	// Try to create transcoder from profile if available, otherwise use variant directly
	// Transcoder ID includes both source and target variants for clarity:
	// transcoder-{session-id}-{source-variant}-{target-variant}
	transcoderID := fmt.Sprintf("transcoder-%s-%s-%s", s.ID.String(), source.String(), target.String())

	if s.EncodingProfile != nil {
		// Use profile for full configuration (bitrate, preset, hwaccel, etc.)
		// Pass target variant which may override profile's target codecs (e.g., from client detection)
		transcoder, err = s.transcoderFactory.CreateTranscoderFromProfile(
			transcoderID,
			s.esBuffer,
			source,
			target, // Target variant (may override profile targets via client detection)
			s.EncodingProfile,
			opts,
		)
		if err != nil {
			return fmt.Errorf("creating transcoder from profile: %w", err)
		}
	} else {
		// No profile - create transcoder directly from variant with default settings
		slog.Info("Creating transcoder from variant (no profile available)",
			slog.String("session_id", s.ID.String()),
			slog.String("source", source.String()),
			slog.String("target", target.String()))

		transcoder, err = s.transcoderFactory.CreateTranscoderFromVariant(
			transcoderID,
			s.esBuffer,
			source,
			target,
			opts,
		)
		if err != nil {
			return fmt.Errorf("creating transcoder from variant: %w", err)
		}
	}

	// Start the transcoder
	if err := transcoder.Start(s.ctx); err != nil {
		return fmt.Errorf("starting transcoder: %w", err)
	}

	// Track the transcoder for cleanup
	s.esTranscodersMu.Lock()
	s.esTranscoders = append(s.esTranscoders, transcoder)
	s.esTranscodersMu.Unlock()

	slog.Debug("Started transcoder for variant request",
		slog.String("session_id", s.ID.String()),
		slog.String("source", source.String()),
		slog.String("target", target.String()))

	return nil
}

// GetESBuffer returns the shared ES buffer for this session.
// This is set during pipeline init and read-only afterward.
func (s *RelaySession) GetESBuffer() *SharedESBuffer {
	return s.esBuffer
}

// configureProcessorStreamContext sets the X-Stream headers context on a processor.
// This enables processors to include mode, decision, and version headers in responses.
func (s *RelaySession) configureProcessorStreamContext(p *BaseProcessor) {
	ctx := StreamContext{
		ProxyMode: "smart", // Sessions are only used in smart mode
		Version:   version.Version,
	}
	p.SetStreamContext(ctx)
}

// GetHLSTSProcessor returns the HLS-TS processor for the default variant if it exists.
// Returns nil if the processor hasn't been created yet.
func (s *RelaySession) GetHLSTSProcessor() *HLSTSProcessor {
	// Return the processor for the session's default variant
	variant := VariantCopy
	if s.processorConfig != nil {
		variant = s.processorConfig.TargetVariant
	}
	processor, _ := s.hlsTSProcessors.Load(variant)
	return processor
}

// GetOrCreateHLSTSProcessor returns the HLS-TS processor for the session's default variant.
// For per-client codec variants, use GetOrCreateHLSTSProcessorForVariant instead.
func (s *RelaySession) GetOrCreateHLSTSProcessor() (*HLSTSProcessor, error) {
	variant := VariantCopy
	if s.processorConfig != nil {
		variant = s.processorConfig.TargetVariant
	}
	return s.GetOrCreateHLSTSProcessorForVariant(variant)
}

// GetOrCreateHLSTSProcessorForVariant returns the HLS-TS processor for a specific codec variant,
// creating it on-demand if needed. This enables per-client codec variants - different clients
// can receive different codecs from the same session by requesting different variants.
// If the variant doesn't exist in the ES buffer, transcoding will be triggered automatically.
func (s *RelaySession) GetOrCreateHLSTSProcessorForVariant(variant CodecVariant) (*HLSTSProcessor, error) {
	// Fast path: check if processor already exists (lock-free read)
	if processor, exists := s.hlsTSProcessors.Load(variant); exists {
		return processor, nil
	}

	// Verify session is ready for processor creation
	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

	// Create config for new processor
	config := DefaultHLSTSProcessorConfig()
	config.Logger = slog.Default()
	config.TargetSegmentDuration = s.processorConfig.TargetSegmentDuration
	config.MaxSegments = s.processorConfig.MaxSegments
	if s.processorConfig.PlaylistSegments > 0 {
		config.PlaylistSegments = s.processorConfig.PlaylistSegments
	}

	processor := NewHLSTSProcessor(
		fmt.Sprintf("hls-ts-%s-%s", s.ID.String(), variant.String()),
		s.esBuffer,
		variant,
		config,
	)

	if err := processor.Start(s.ctx); err != nil {
		return nil, fmt.Errorf("starting HLS-TS processor for variant %s: %w", variant.String(), err)
	}
	s.configureProcessorStreamContext(processor.BaseProcessor)
	processor.SetBandwidthTracker(s.edgeBandwidth.GetOrCreateProcessorTracker("hls"))

	// Atomically store or get existing - if another goroutine won the race, stop our duplicate
	existing, loaded := s.hlsTSProcessors.LoadOrStore(variant, processor)
	if loaded {
		// Another goroutine created the processor first, stop ours
		processor.Stop()
		return existing, nil
	}

	slog.Debug("Created HLS-TS processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_hls_ts_processors", s.hlsTSProcessors.Len()))

	return processor, nil
}

// GetOrCreateHLSfMP4Processor returns the HLS-fMP4 processor for the session's default variant.
// For per-client codec variants, use GetOrCreateHLSfMP4ProcessorForVariant instead.
func (s *RelaySession) GetOrCreateHLSfMP4Processor() (*HLSfMP4Processor, error) {
	variant := VariantCopy
	if s.processorConfig != nil {
		variant = s.processorConfig.TargetVariant
	}
	return s.GetOrCreateHLSfMP4ProcessorForVariant(variant)
}

// GetOrCreateHLSfMP4ProcessorForVariant returns the HLS-fMP4 processor for a specific codec variant,
// creating it on-demand if needed.
func (s *RelaySession) GetOrCreateHLSfMP4ProcessorForVariant(variant CodecVariant) (*HLSfMP4Processor, error) {
	// Fast path: check if processor already exists (lock-free read)
	if processor, exists := s.hlsFMP4Processors.Load(variant); exists {
		return processor, nil
	}

	// Verify session is ready for processor creation
	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

	// Create config for new processor
	config := DefaultHLSfMP4ProcessorConfig()
	config.Logger = slog.Default()
	config.TargetSegmentDuration = s.processorConfig.TargetSegmentDuration
	config.MaxSegments = s.processorConfig.MaxSegments
	if s.processorConfig.PlaylistSegments > 0 {
		config.PlaylistSegments = s.processorConfig.PlaylistSegments
	}

	processor := NewHLSfMP4Processor(
		fmt.Sprintf("hls-fmp4-%s-%s", s.ID.String(), variant.String()),
		s.esBuffer,
		variant,
		config,
	)

	if err := processor.Start(s.ctx); err != nil {
		return nil, fmt.Errorf("starting HLS-fMP4 processor for variant %s: %w", variant.String(), err)
	}
	s.configureProcessorStreamContext(processor.BaseProcessor)
	processor.SetBandwidthTracker(s.edgeBandwidth.GetOrCreateProcessorTracker("hls"))

	// Atomically store or get existing - if another goroutine won the race, stop our duplicate
	existing, loaded := s.hlsFMP4Processors.LoadOrStore(variant, processor)
	if loaded {
		// Another goroutine created the processor first, stop ours
		processor.Stop()
		return existing, nil
	}

	slog.Debug("Created HLS-fMP4 processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_hls_fmp4_processors", s.hlsFMP4Processors.Len()))

	return processor, nil
}

// GetOrCreateDASHProcessor returns the DASH processor for the session's default variant.
// For per-client codec variants, use GetOrCreateDASHProcessorForVariant instead.
func (s *RelaySession) GetOrCreateDASHProcessor() (*DASHProcessor, error) {
	variant := VariantCopy
	if s.processorConfig != nil {
		variant = s.processorConfig.TargetVariant
	}
	return s.GetOrCreateDASHProcessorForVariant(variant)
}

// GetOrCreateDASHProcessorForVariant returns the DASH processor for a specific codec variant,
// creating it on-demand if needed.
func (s *RelaySession) GetOrCreateDASHProcessorForVariant(variant CodecVariant) (*DASHProcessor, error) {
	// Fast path: check if processor already exists (lock-free read)
	if processor, exists := s.dashProcessors.Load(variant); exists {
		return processor, nil
	}

	// Verify session is ready for processor creation
	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

	// Create config for new processor
	config := DefaultDASHProcessorConfig()
	config.Logger = slog.Default()
	config.TargetSegmentDuration = s.processorConfig.TargetSegmentDuration
	config.MaxSegments = s.processorConfig.MaxSegments
	if s.processorConfig.PlaylistSegments > 0 {
		config.PlaylistSegments = s.processorConfig.PlaylistSegments
	}

	processor := NewDASHProcessor(
		fmt.Sprintf("dash-%s-%s", s.ID.String(), variant.String()),
		s.esBuffer,
		variant,
		config,
	)

	if err := processor.Start(s.ctx); err != nil {
		return nil, fmt.Errorf("starting DASH processor for variant %s: %w", variant.String(), err)
	}
	s.configureProcessorStreamContext(processor.BaseProcessor)
	processor.SetBandwidthTracker(s.edgeBandwidth.GetOrCreateProcessorTracker("dash"))

	// Atomically store or get existing - if another goroutine won the race, stop our duplicate
	existing, loaded := s.dashProcessors.LoadOrStore(variant, processor)
	if loaded {
		// Another goroutine created the processor first, stop ours
		processor.Stop()
		return existing, nil
	}

	slog.Debug("Created DASH processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_dash_processors", s.dashProcessors.Len()))

	return processor, nil
}

// GetOrCreateMPEGTSProcessor returns the MPEG-TS processor for the session's default variant.
// For per-client codec variants, use GetOrCreateMPEGTSProcessorForVariant instead.
func (s *RelaySession) GetOrCreateMPEGTSProcessor() (*MPEGTSProcessor, error) {
	variant := VariantCopy
	if s.processorConfig != nil {
		variant = s.processorConfig.TargetVariant
	}
	return s.GetOrCreateMPEGTSProcessorForVariant(variant)
}

// GetOrCreateMPEGTSProcessorForVariant returns the MPEG-TS processor for a specific codec variant,
// creating it on-demand if needed.
func (s *RelaySession) GetOrCreateMPEGTSProcessorForVariant(variant CodecVariant) (*MPEGTSProcessor, error) {
	// Fast path: check if processor already exists (lock-free read)
	if processor, exists := s.mpegtsProcessors.Load(variant); exists {
		return processor, nil
	}

	// Verify session is ready for processor creation
	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

	// Create config for new processor
	config := DefaultMPEGTSProcessorConfig()
	config.Logger = slog.Default()

	// Set callback to track session idle state when clients connect/disconnect
	config.OnClientChange = func(clientCount int) {
		s.UpdateIdleState()
		// If no clients remain, schedule processor cleanup
		if clientCount == 0 {
			// Use a goroutine to avoid blocking the callback
			go s.StopProcessorIfIdle("mpegts")
		}
	}

	processor := NewMPEGTSProcessor(
		fmt.Sprintf("mpegts-%s-%s", s.ID.String(), variant.String()),
		s.esBuffer,
		variant,
		config,
	)

	if err := processor.Start(s.ctx); err != nil {
		return nil, fmt.Errorf("starting MPEG-TS processor for variant %s: %w", variant.String(), err)
	}
	s.configureProcessorStreamContext(processor.BaseProcessor)
	processor.SetBandwidthTracker(s.edgeBandwidth.GetOrCreateProcessorTracker("mpegts"))

	// Atomically store or get existing - if another goroutine won the race, stop our duplicate
	existing, loaded := s.mpegtsProcessors.LoadOrStore(variant, processor)
	if loaded {
		// Another goroutine created the processor first, stop ours
		processor.Stop()
		return existing, nil
	}

	slog.Debug("Created MPEG-TS processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_mpegts_processors", s.mpegtsProcessors.Len()))

	return processor, nil
}

// ClientCount returns the number of connected clients.
// With the ES pipeline architecture, clients are tracked per-processor across all variants.
func (s *RelaySession) ClientCount() int {
	count := 0

	// Count clients across all HLS-TS processors (lock-free iteration)
	s.hlsTSProcessors.Range(func(_ CodecVariant, p *HLSTSProcessor) bool {
		count += p.ClientCount()
		return true
	})

	// Count clients across all HLS-fMP4 processors
	s.hlsFMP4Processors.Range(func(_ CodecVariant, p *HLSfMP4Processor) bool {
		count += p.ClientCount()
		return true
	})

	// Count clients across all DASH processors
	s.dashProcessors.Range(func(_ CodecVariant, p *DASHProcessor) bool {
		count += p.ClientCount()
		return true
	})

	// Count clients across all MPEG-TS processors
	s.mpegtsProcessors.Range(func(_ CodecVariant, p *MPEGTSProcessor) bool {
		count += p.ClientCount()
		return true
	})

	return count
}

// UpdateIdleState updates the session's idle tracking based on current client count.
// This should be called after clients connect or disconnect.
// When client count drops to 0, IdleSince is set to the current time.
// When clients reconnect, IdleSince is cleared.
func (s *RelaySession) UpdateIdleState() {
	clientCount := s.ClientCount()

	if clientCount == 0 {
		// No clients - mark as idle if not already
		idleSince, _ := s.idleSince.Load().(time.Time)
		if idleSince.IsZero() {
			now := time.Now()
			s.idleSince.Store(now)
			slog.Debug("Session became idle",
				slog.String("session_id", s.ID.String()),
				slog.Time("idle_since", now))
		}
	} else {
		// Has clients - clear idle state
		idleSince, _ := s.idleSince.Load().(time.Time)
		if !idleSince.IsZero() {
			slog.Debug("Session no longer idle",
				slog.String("session_id", s.ID.String()),
				slog.Int("client_count", clientCount))
			s.idleSince.Store(time.Time{})
		}
	}

	s.lastActivity.Store(time.Now())
}

// ClearIdleState clears the session's idle state when a client connects.
// This should be called after registering a client to ensure the session
// is not cleaned up while active clients are connected.
func (s *RelaySession) ClearIdleState() {
	idleSince, _ := s.idleSince.Load().(time.Time)
	if !idleSince.IsZero() {
		slog.Debug("Clearing session idle state (client connected)",
			slog.String("session_id", s.ID.String()))
		s.idleSince.Store(time.Time{})
	}
	s.lastActivity.Store(time.Now())
}

// StopProcessorIfIdle stops idle processors of a given type across all variants.
// This is called when a client disconnects to immediately clean up unused processors.
func (s *RelaySession) StopProcessorIfIdle(processorType string) {
	// Collect and remove idle processors atomically, then stop them outside the map
	type processorToStop struct {
		variant   CodecVariant
		processor interface{ Stop() }
	}
	var toStop []processorToStop
	sessionID := s.ID.String()

	switch processorType {
	case "mpegts":
		s.mpegtsProcessors.Range(func(variant CodecVariant, processor *MPEGTSProcessor) bool {
			if processor.ClientCount() == 0 {
				toStop = append(toStop, processorToStop{variant, processor})
				s.mpegtsProcessors.Delete(variant)
			}
			return true
		})
	case "hls-ts":
		s.hlsTSProcessors.Range(func(variant CodecVariant, processor *HLSTSProcessor) bool {
			if processor.ClientCount() == 0 {
				toStop = append(toStop, processorToStop{variant, processor})
				s.hlsTSProcessors.Delete(variant)
			}
			return true
		})
	case "hls-fmp4":
		s.hlsFMP4Processors.Range(func(variant CodecVariant, processor *HLSfMP4Processor) bool {
			if processor.ClientCount() == 0 {
				toStop = append(toStop, processorToStop{variant, processor})
				s.hlsFMP4Processors.Delete(variant)
			}
			return true
		})
	case "dash":
		s.dashProcessors.Range(func(variant CodecVariant, processor *DASHProcessor) bool {
			if processor.ClientCount() == 0 {
				toStop = append(toStop, processorToStop{variant, processor})
				s.dashProcessors.Delete(variant)
			}
			return true
		})
	}

	// Stop processors after removing from map - Stop() can block waiting for goroutines
	for _, p := range toStop {
		slog.Debug("Stopping idle processor",
			slog.String("session_id", sessionID),
			slog.String("type", processorType),
			slog.String("variant", p.variant.String()))
		p.processor.Stop()
	}

	// Trigger immediate variant cleanup to free resources sooner
	// This avoids waiting up to 30s for the next cleanup cycle
	if len(toStop) > 0 {
		s.cleanupUnusedVariants()
	}

	// Update session idle state after processor cleanup
	clientCount := s.ClientCount()
	idleSince, _ := s.idleSince.Load().(time.Time)
	if clientCount == 0 && idleSince.IsZero() {
		s.idleSince.Store(time.Now())
		slog.Debug("Session became idle after processor cleanup",
			slog.String("session_id", sessionID))
	}
}

// Close closes the session and releases resources.
func (s *RelaySession) Close() {
	// Use CompareAndSwap to ensure Close() is only executed once
	if !s.closed.CompareAndSwap(false, true) {
		return
	}

	s.cancel()

	if s.hlsCollapser != nil {
		s.hlsCollapser.Stop()
	}

	if s.hlsRepackager != nil {
		s.hlsRepackager.Close()
	}

	if s.inputReader != nil {
		s.inputReader.Close()
	}

	// Collect transcoders to stop WITHOUT holding the lock during Stop calls
	// This prevents blocking other goroutines that need the lock
	s.esTranscodersMu.Lock()
	transcodersToStop := make([]Transcoder, len(s.esTranscoders))
	copy(transcodersToStop, s.esTranscoders)
	s.esTranscoders = nil
	s.esTranscodersMu.Unlock()

	// Stop transcoders outside the lock - Stop() has its own timeout handling
	for _, transcoder := range transcodersToStop {
		transcoder.Stop()
	}

	// Clear and stop all processors using the Clear() method which returns them for cleanup
	hlsTSToStop := s.hlsTSProcessors.Clear()
	hlsFMP4ToStop := s.hlsFMP4Processors.Clear()
	dashToStop := s.dashProcessors.Clear()
	mpegtsToStop := s.mpegtsProcessors.Clear()

	// Stop all processors (these are already removed from the maps)
	for _, processor := range hlsTSToStop {
		processor.Stop()
	}
	for _, processor := range hlsFMP4ToStop {
		processor.Stop()
	}
	for _, processor := range dashToStop {
		processor.Stop()
	}
	for _, processor := range mpegtsToStop {
		processor.Stop()
	}

	// Close the TS demuxer to stop its reader goroutine
	if s.tsDemuxer != nil {
		s.tsDemuxer.Close()
	}

	if s.esBuffer != nil {
		s.esBuffer.Close()
	}
}

// Stats returns session statistics.
// This method minimizes lock contention by:
// 1. Reading immutable fields without locks (set at creation time)
// 2. Using atomic values for frequently updated fields
// 3. Using separate lock scopes for different data sources
func (s *RelaySession) Stats() SessionStats {
	// Use helper methods for atomic values (safe to call without any locks)
	lastActivity := s.LastActivity()
	idleSince := s.IdleSince()

	// Collect session state with minimal lock scope
	var errStr string
	var profileName string
	var closed, inFallback bool
	var esBuffer *SharedESBuffer
	var ffmpegCmd *ffmpeg.Command
	var fallbackController *FallbackController
	var cachedCodecInfo *models.LastKnownCodec
	var classification ClassificationResult

	// Read atomic state values
	if errPtr := s.lastErr.Load(); errPtr != nil {
		errStr = (*errPtr).Error()
	}
	profileName = "passthrough"
	if s.EncodingProfile != nil {
		profileName = s.EncodingProfile.Name
	}
	closed = s.closed.Load()
	inFallback = s.inFallback.Load()

	// These fields are set once during init and read-only afterward
	esBuffer = s.esBuffer
	ffmpegCmd = s.ffmpegCmd
	fallbackController = s.fallbackController
	cachedCodecInfo = s.CachedCodecInfo
	classification = s.Classification

	// Get stats from ES buffer if available (has its own lock)
	var bytesWritten uint64
	if esBuffer != nil {
		esStats := esBuffer.Stats()
		bytesWritten = esStats.TotalBytes
	}

	// Collect clients from all processors across all variants (lock-free iteration)
	var clientCount int
	var clients []ClientStats
	activeFormats := make([]string, 0, 4)

	// Track which format types have active processors
	hasMpegts := s.mpegtsProcessors.Len() > 0
	hasHlsTS := s.hlsTSProcessors.Len() > 0
	hasHlsFMP4 := s.hlsFMP4Processors.Len() > 0
	hasDash := s.dashProcessors.Len() > 0

	// MPEG-TS processor clients (all variants)
	s.mpegtsProcessors.Range(func(variant CodecVariant, processor *MPEGTSProcessor) bool {
		clientCount += processor.ClientCount()
		processorClients := processor.GetClients()
		slog.Debug("Stats: MPEG-TS processor variant",
			slog.String("session_id", s.ID.String()),
			slog.String("variant", string(variant)),
			slog.Int("client_count", len(processorClients)))
		for _, c := range processorClients {
			clients = append(clients, ClientStats{
				ID:            c.ID,
				BytesRead:     c.BytesRead.Load(),
				ConnectedAt:   c.ConnectedAt,
				UserAgent:     c.UserAgent,
				RemoteAddr:    c.RemoteAddr,
				ClientFormat:  "mpegts",
				ClientVariant: string(variant),
			})
		}
		return true
	})

	// HLS TS processor clients (all variants)
	s.hlsTSProcessors.Range(func(variant CodecVariant, processor *HLSTSProcessor) bool {
		clientCount += processor.ClientCount()
		for _, c := range processor.GetClients() {
			clients = append(clients, ClientStats{
				ID:            c.ID,
				BytesRead:     c.BytesRead.Load(),
				ConnectedAt:   c.ConnectedAt,
				UserAgent:     c.UserAgent,
				RemoteAddr:    c.RemoteAddr,
				ClientFormat:  "hls",
				ClientVariant: string(variant),
			})
		}
		return true
	})

	// HLS fMP4 processor clients (all variants)
	s.hlsFMP4Processors.Range(func(variant CodecVariant, processor *HLSfMP4Processor) bool {
		clientCount += processor.ClientCount()
		for _, c := range processor.GetClients() {
			clients = append(clients, ClientStats{
				ID:            c.ID,
				BytesRead:     c.BytesRead.Load(),
				ConnectedAt:   c.ConnectedAt,
				UserAgent:     c.UserAgent,
				RemoteAddr:    c.RemoteAddr,
				ClientFormat:  "hls",
				ClientVariant: string(variant),
			})
		}
		return true
	})

	// DASH processor clients (all variants)
	s.dashProcessors.Range(func(variant CodecVariant, processor *DASHProcessor) bool {
		clientCount += processor.ClientCount()
		for _, c := range processor.GetClients() {
			clients = append(clients, ClientStats{
				ID:            c.ID,
				BytesRead:     c.BytesRead.Load(),
				ConnectedAt:   c.ConnectedAt,
				UserAgent:     c.UserAgent,
				RemoteAddr:    c.RemoteAddr,
				ClientFormat:  "dash",
				ClientVariant: string(variant),
			})
		}
		return true
	})

	// Build active formats list (outside lock since we captured the bools)
	if hasMpegts {
		activeFormats = append(activeFormats, "mpegts")
	}
	if hasHlsTS {
		activeFormats = append(activeFormats, "hls")
	}
	if hasHlsFMP4 && !hasHlsTS {
		// Only add hls-fmp4 if hlsTS isn't already there (avoid duplicate HLS entries)
		activeFormats = append(activeFormats, "hls")
	}
	if hasDash {
		activeFormats = append(activeFormats, "dash")
	}

	// Build stats from immutable fields (set at creation, no lock needed)
	// Check ingest completion state (atomic, safe to call)
	ingestCompleted := s.IngestCompleted()
	originConnected := !ingestCompleted && !closed

	stats := SessionStats{
		ID:                s.ID.String(),
		ChannelID:         s.ChannelID.String(),
		ChannelName:       s.ChannelName,
		StreamSourceName:  s.StreamSourceName,
		ProfileName:       profileName,
		StreamURL:         s.StreamURL,
		Classification:    classification.Mode.String(),
		StartedAt:         s.StartedAt,
		LastActivity:      lastActivity,
		IdleSince:         idleSince,
		ClientCount:       clientCount,
		BytesWritten:      bytesWritten,
		BytesFromUpstream: bytesWritten,
		Closed:            closed,
		IngestCompleted:   ingestCompleted,
		OriginConnected:   originConnected,
		Error:             errStr,
		InFallback:        inFallback,
		Clients:           clients,
	}

	// Set source format from classification
	if classification.SourceFormat != "" {
		stats.SourceFormat = string(classification.SourceFormat)
	}

	// Active processor formats already collected above (lock-free iteration)
	stats.ActiveProcessorFormats = activeFormats

	// Determine output format from active processors if ClientFormat not set
	// This ensures the flow visualization shows the correct processor type
	if stats.ClientFormat == "" {
		if len(activeFormats) > 0 {
			// Use the first active processor format as the primary
			stats.ClientFormat = activeFormats[0]
		} else if stats.SourceFormat != "" {
			// Fallback to source format if no processors active
			stats.ClientFormat = stats.SourceFormat
		}
	}

	// Include codec info from cached codec data if available
	if cachedCodecInfo != nil {
		stats.VideoCodec = cachedCodecInfo.VideoCodec
		stats.AudioCodec = cachedCodecInfo.AudioCodec
		stats.Framerate = cachedCodecInfo.VideoFramerate
		stats.VideoWidth = cachedCodecInfo.VideoWidth
		stats.VideoHeight = cachedCodecInfo.VideoHeight
	}

	// Include fallback controller stats if available
	if fallbackController != nil {
		ctrlStats := fallbackController.Stats()
		stats.FallbackEnabled = true
		stats.FallbackErrorCount = ctrlStats.ErrorCount
		stats.FallbackRecoveryAttempts = ctrlStats.RecoveryAttempts
	}

	// Include FFmpeg process stats if running (legacy pipeline)
	if ffmpegCmd != nil {
		if procStats := ffmpegCmd.ProcessStats(); procStats != nil {
			// Sample resource history if enough time has passed
			if s.resourceHistory != nil && s.resourceHistory.ShouldSample() {
				s.resourceHistory.AddSample(procStats.CPUPercent, procStats.MemoryRSSMB)
			}

			stats.FFmpegStats = &FFmpegProcessStats{
				PID:           procStats.PID,
				CPUPercent:    procStats.CPUPercent,
				MemoryRSSMB:   procStats.MemoryRSSMB,
				MemoryPercent: procStats.MemoryPercent,
				BytesWritten:  procStats.BytesWritten,
				WriteRateMbps: procStats.WriteRateMbps,
				DurationSecs:  procStats.Duration.Seconds(),
			}

			// Include resource history
			if s.resourceHistory != nil {
				cpuHistory, memHistory := s.resourceHistory.GetHistory()
				stats.FFmpegStats.CPUHistory = cpuHistory
				stats.FFmpegStats.MemoryHistory = memHistory
			}
		}
	}

	// Include ES transcoder stats if any are running (ES pipeline)
	s.esTranscodersMu.RLock()
	if len(s.esTranscoders) > 0 {
		// Aggregate stats from all ES transcoders
		var totalBytesIn, totalBytesOut uint64
		var totalSamplesIn, totalSamplesOut uint64
		var totalCPUPercent, totalMemoryRSSMB, totalMemoryPercent float64
		var activePID int
		var transcoderList []ESTranscoderStats
		var activeTranscoderCount int

		for _, transcoder := range s.esTranscoders {
			// Skip closed transcoders - they've finished processing and stopped
			if transcoder.IsClosed() {
				continue
			}
			activeTranscoderCount++
			tStats := transcoder.Stats()
			totalBytesIn += tStats.BytesIn
			totalBytesOut += tStats.BytesOut
			totalSamplesIn += tStats.SamplesIn
			totalSamplesOut += tStats.SamplesOut

			// Get process stats (CPU/memory) from the transcoder
			var cpuPercent, memoryRSSMB, memoryPercent float64
			var pid int
			if procStats := transcoder.ProcessStats(); procStats != nil {
				pid = procStats.PID
				cpuPercent = procStats.CPUPercent
				memoryRSSMB = procStats.MemoryRSSMB
				memoryPercent = procStats.MemoryPercent
				// Aggregate process stats
				totalCPUPercent += cpuPercent
				totalMemoryRSSMB += memoryRSSMB
				totalMemoryPercent += memoryPercent
				if activePID == 0 {
					activePID = pid
				}
			}

			// Get resource history for sparkline graphs
			cpuHistory, memHistory := transcoder.GetResourceHistory()

			transcoderList = append(transcoderList, ESTranscoderStats{
				ID:            tStats.ID,
				SourceVariant: tStats.SourceVariant.String(),
				TargetVariant: tStats.TargetVariant.String(),
				StartedAt:     tStats.StartedAt,
				LastActivity:  tStats.LastActivity,
				SamplesIn:     tStats.SamplesIn,
				SamplesOut:    tStats.SamplesOut,
				BytesIn:       tStats.BytesIn,
				BytesOut:      tStats.BytesOut,
				Errors:        tStats.Errors,
				// Codec names from target variant (e.g., "h265", "aac")
				VideoCodec: tStats.VideoCodec,
				AudioCodec: tStats.AudioCodec,
				// Encoder names (e.g., "libx265", "h264_nvenc")
				VideoEncoder:  tStats.VideoEncoder,
				AudioEncoder:  tStats.AudioEncoder,
				HWAccel:       tStats.HWAccel,
				HWAccelDevice: tStats.HWAccelDevice,
				PID:           pid,
				CPUPercent:    cpuPercent,
				MemoryRSSMB:   memoryRSSMB,
				MemoryPercent: memoryPercent,
				EncodingSpeed: tStats.EncodingSpeed,
				CPUHistory:    cpuHistory,
				MemoryHistory: memHistory,
				FFmpegCommand: tStats.FFmpegCommand,
			})
		}

		stats.ESTranscoderStats = &ESTranscodersStats{
			Count:          activeTranscoderCount,
			TotalBytesIn:   totalBytesIn,
			TotalBytesOut:  totalBytesOut,
			TotalSamplesIn: totalSamplesIn,
			Transcoders:    transcoderList,
		}

		// If no legacy FFmpeg stats but we have active ES transcoders, populate FFmpegStats
		// with actual process stats so the flow visualization shows transcoding
		if stats.FFmpegStats == nil && activeTranscoderCount > 0 && len(transcoderList) > 0 {
			// Sample resource history if enough time has passed
			if s.resourceHistory != nil && s.resourceHistory.ShouldSample() {
				s.resourceHistory.AddSample(totalCPUPercent, totalMemoryRSSMB)
			}

			// Get encoding config from first active transcoder for display
			firstStats := transcoderList[0]
			stats.FFmpegStats = &FFmpegProcessStats{
				PID:           activePID,
				CPUPercent:    totalCPUPercent,
				MemoryRSSMB:   totalMemoryRSSMB,
				MemoryPercent: totalMemoryPercent,
				BytesWritten:  totalBytesOut,
				DurationSecs:  time.Since(firstStats.StartedAt).Seconds(),
				// Codec names (e.g., "h265", "aac") - what the stream IS
				VideoCodec: firstStats.VideoCodec,
				AudioCodec: firstStats.AudioCodec,
				// Encoder names (e.g., "libx265", "h264_nvenc") - what FFmpeg uses
				VideoEncoder:  firstStats.VideoEncoder,
				AudioEncoder:  firstStats.AudioEncoder,
				HWAccel:       firstStats.HWAccel,
				HWAccelDevice: firstStats.HWAccelDevice,
				// Encoding speed from first transcoder
				EncodingSpeed: firstStats.EncodingSpeed,
			}

			// Include resource history
			if s.resourceHistory != nil {
				cpuHistory, memHistory := s.resourceHistory.GetHistory()
				stats.FFmpegStats.CPUHistory = cpuHistory
				stats.FFmpegStats.MemoryHistory = memHistory
			}
		}
	}
	s.esTranscodersMu.RUnlock()

	// Include ES buffer stats for HLS/DASH sessions
	if esBuffer != nil {
		esStats := esBuffer.Stats()
		stats.SegmentBufferStats = &SessionSegmentBufferStats{
			TotalBytesWritten: esStats.TotalBytes,
			// Other stats can be populated from processors if needed
		}
		// Include full ES buffer stats with variant information
		stats.ESBufferStats = &esStats
	}

	// Include edge bandwidth stats if available
	if s.edgeBandwidth != nil {
		// Sample all trackers before collecting stats
		s.edgeBandwidth.SampleAll()
		stats.EdgeBandwidthStats = s.edgeBandwidth.Stats()
	}

	return stats
}

// SessionStats holds session statistics.
type SessionStats struct {
	ID                string    `json:"id"`
	ChannelID         string    `json:"channel_id"`
	ChannelName       string    `json:"channel_name,omitempty"`
	StreamSourceName  string    `json:"stream_source_name,omitempty"` // Name of the stream source (e.g., "s8k")
	ProfileName       string    `json:"profile_name,omitempty"`
	StreamURL         string    `json:"stream_url"`
	Classification    string    `json:"classification"`
	StartedAt         time.Time `json:"started_at"`
	LastActivity      time.Time `json:"last_activity"`
	IdleSince         time.Time `json:"idle_since,omitempty"`
	ClientCount       int       `json:"client_count"`
	BytesWritten      uint64    `json:"bytes_written"`
	BytesFromUpstream uint64    `json:"bytes_from_upstream"`
	Closed            bool      `json:"closed"`
	IngestCompleted   bool      `json:"ingest_completed"` // True if origin ingest finished (EOF received)
	OriginConnected   bool      `json:"origin_connected"` // True if origin is still streaming data
	Error             string    `json:"error,omitempty"`
	// Smart delivery information (only present when using smart mode)
	DeliveryDecision       string   `json:"delivery_decision,omitempty"`        // passthrough, repackage, or transcode
	ClientFormat           string   `json:"client_format,omitempty"`            // requested output format
	SourceFormat           string   `json:"source_format,omitempty"`            // detected source format
	ActiveProcessorFormats []string `json:"active_processor_formats,omitempty"` // Actually running processors (mpegts, hls, dash)
	VideoCodec             string   `json:"video_codec,omitempty"`              // video codec (h264, hevc, etc.)
	AudioCodec             string   `json:"audio_codec,omitempty"`              // audio codec (aac, ac3, etc.)
	Framerate              float64  `json:"framerate,omitempty"`                // video framerate (fps)
	VideoWidth             int      `json:"video_width,omitempty"`              // video width in pixels
	VideoHeight            int      `json:"video_height,omitempty"`             // video height in pixels
	// Fallback information
	InFallback               bool `json:"in_fallback"`
	FallbackEnabled          bool `json:"fallback_enabled"`
	FallbackErrorCount       int  `json:"fallback_error_count,omitempty"`
	FallbackRecoveryAttempts int  `json:"fallback_recovery_attempts,omitempty"`
	// FFmpeg process stats (only present when FFmpeg is running)
	FFmpegStats *FFmpegProcessStats `json:"ffmpeg_stats,omitempty"`
	// ES transcoder stats (present when ES-based transcoders are running)
	ESTranscoderStats *ESTranscodersStats `json:"es_transcoder_stats,omitempty"`
	// Connected client details
	Clients []ClientStats `json:"clients,omitempty"`
	// Segment buffer stats (only present for HLS/DASH sessions)
	SegmentBufferStats *SessionSegmentBufferStats `json:"segment_buffer_stats,omitempty"`
	// ES buffer stats (variant information from shared buffer)
	ESBufferStats *ESBufferStats `json:"es_buffer_stats,omitempty"`
	// Edge bandwidth stats (per-edge bandwidth tracking for flow visualization)
	EdgeBandwidthStats *EdgeBandwidthStats `json:"edge_bandwidth_stats,omitempty"`
}

// ClientStats holds statistics for a single connected client.
type ClientStats struct {
	ID            string    `json:"id"`
	BytesRead     uint64    `json:"bytes_read"`
	LastSequence  uint64    `json:"last_sequence"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastRead      time.Time `json:"last_read,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
	RemoteAddr    string    `json:"remote_addr,omitempty"`
	ClientFormat  string    `json:"client_format,omitempty"`  // Format this client is using (hls, mpegts, dash)
	ClientVariant string    `json:"client_variant,omitempty"` // Codec variant this client receives (e.g., "h265/aac", "av1/eac3")
}

// SessionSegmentBufferStats contains segment buffer statistics for a session.
type SessionSegmentBufferStats struct {
	ChunkCount        int    `json:"chunk_count" doc:"Number of chunks in buffer"`
	SegmentCount      int    `json:"segment_count" doc:"Number of segments available"`
	BufferSizeBytes   int64  `json:"buffer_size_bytes" doc:"Current buffer size in bytes"`
	TotalBytesWritten uint64 `json:"total_bytes_written" doc:"Total bytes written to buffer"`
	FirstSegment      uint64 `json:"first_segment" doc:"First available segment sequence"`
	LastSegment       uint64 `json:"last_segment" doc:"Last available segment sequence"`

	// Memory usage metrics
	MaxBufferSizeBytes int64   `json:"max_buffer_size_bytes" doc:"Configured maximum buffer size"`
	BufferUtilization  float64 `json:"buffer_utilization" doc:"Buffer usage percentage (0-100)"`
	AverageChunkSize   int64   `json:"average_chunk_size" doc:"Average bytes per chunk"`
	AverageSegmentSize int64   `json:"average_segment_size" doc:"Average bytes per segment"`
	TotalChunksWritten uint64  `json:"total_chunks_written" doc:"Total chunks written since creation"`
}

// FFmpegProcessStats contains resource usage for the FFmpeg process.
type FFmpegProcessStats struct {
	PID           int     `json:"pid"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryRSSMB   float64 `json:"memory_rss_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	BytesWritten  uint64  `json:"bytes_written"`
	WriteRateMbps float64 `json:"write_rate_mbps"`
	DurationSecs  float64 `json:"duration_secs"`
	// Codec names (e.g., "h264", "h265", "aac") - what the stream IS
	VideoCodec string `json:"video_codec,omitempty"`
	AudioCodec string `json:"audio_codec,omitempty"`
	// Encoder names (e.g., "libx264", "h264_nvenc", "libopus") - what FFmpeg uses
	VideoEncoder  string `json:"video_encoder,omitempty"`
	AudioEncoder  string `json:"audio_encoder,omitempty"`
	HWAccel       string `json:"hwaccel,omitempty"`
	HWAccelDevice string `json:"hwaccel_device,omitempty"`
	// Encoding speed (1.0 = realtime, 2.0 = 2x realtime, 0.5 = half realtime)
	EncodingSpeed float64 `json:"encoding_speed,omitempty"`
	// Resource history for sparkline graphs (last 30 samples, ~1 sample/sec)
	CPUHistory    []float64 `json:"cpu_history,omitempty"`
	MemoryHistory []float64 `json:"memory_history,omitempty"`
}

// ESTranscodersStats contains aggregate stats for all ES-based transcoders.
type ESTranscodersStats struct {
	Count          int                 `json:"count"`
	TotalBytesIn   uint64              `json:"total_bytes_in"`
	TotalBytesOut  uint64              `json:"total_bytes_out"`
	TotalSamplesIn uint64              `json:"total_samples_in"`
	Transcoders    []ESTranscoderStats `json:"transcoders,omitempty"`
}

// ESTranscoderStats contains stats for a single ES-based transcoder.
type ESTranscoderStats struct {
	ID            string    `json:"id"`
	SourceVariant string    `json:"source_variant"`
	TargetVariant string    `json:"target_variant"`
	StartedAt     time.Time `json:"started_at"`
	LastActivity  time.Time `json:"last_activity"`
	SamplesIn     uint64    `json:"samples_in"`
	SamplesOut    uint64    `json:"samples_out"`
	BytesIn       uint64    `json:"bytes_in"`
	BytesOut      uint64    `json:"bytes_out"`
	Errors        uint64    `json:"errors"`
	// Codec names (e.g., "h264", "h265", "aac") - what the stream IS
	VideoCodec string `json:"video_codec,omitempty"`
	AudioCodec string `json:"audio_codec,omitempty"`
	// Encoder names (e.g., "libx264", "h264_nvenc", "libopus") - what FFmpeg uses
	VideoEncoder  string `json:"video_encoder,omitempty"`
	AudioEncoder  string `json:"audio_encoder,omitempty"`
	HWAccel       string `json:"hwaccel,omitempty"`
	HWAccelDevice string `json:"hwaccel_device,omitempty"`
	// Process stats
	PID           int     `json:"pid,omitempty"`
	CPUPercent    float64 `json:"cpu_percent,omitempty"`
	MemoryRSSMB   float64 `json:"memory_rss_mb,omitempty"`
	MemoryPercent float64 `json:"memory_percent,omitempty"`
	// Encoding speed (1.0 = realtime, 2.0 = 2x realtime, 0.5 = half realtime)
	EncodingSpeed float64 `json:"encoding_speed,omitempty"`
	// Resource history for sparkline graphs (last 30 samples, ~1 sample/sec)
	CPUHistory    []float64 `json:"cpu_history,omitempty"`
	MemoryHistory []float64 `json:"memory_history,omitempty"`
	// FFmpeg command for debugging
	FFmpegCommand string `json:"ffmpeg_command,omitempty"`
}

// GetFormatRouter returns the format router for handling output format requests.
// GetFormatRouter returns the format router for this session.
// This is set during pipeline init and read-only afterward.
func (s *RelaySession) GetFormatRouter() *FormatRouter {
	return s.formatRouter
}

// LastActivity returns the last activity timestamp for this session.
// This is safe to call concurrently without holding any locks.
func (s *RelaySession) LastActivity() time.Time {
	t, _ := s.lastActivity.Load().(time.Time)
	return t
}

// IdleSince returns when the session became idle (zero clients).
// Returns zero time if the session is not idle.
// This is safe to call concurrently without holding any locks.
func (s *RelaySession) IdleSince() time.Time {
	t, _ := s.idleSince.Load().(time.Time)
	return t
}

// IngestCompleted returns true if the origin ingest has finished (EOF received).
// This indicates the source stream has ended (finite content) but clients may still
// be connected and consuming buffered data.
// This is safe to call concurrently without holding any locks.
func (s *RelaySession) IngestCompleted() bool {
	return s.ingestCompleted.Load()
}

// HasActiveContent returns true if the session has active transcoders that are still working.
// This is used to determine if a session with completed ingest can still serve new clients.
// For finite streams (like IPTV error videos), we want to reuse the session if transcoders
// are still producing output, rather than creating a new session that re-fetches the source.
//
// Note: We only consider active transcoders as "active content", not buffer data alone.
// If ingest is complete and all transcoders are stopped, buffer data is stale and the
// session should be cleaned up even if the buffer still has bytes.
func (s *RelaySession) HasActiveContent() bool {
	// Check for active (non-closed) transcoders
	s.esTranscodersMu.RLock()
	transcoderCount := len(s.esTranscoders)
	activeTranscoders := 0
	for _, transcoder := range s.esTranscoders {
		if !transcoder.IsClosed() {
			activeTranscoders++
		}
	}
	s.esTranscodersMu.RUnlock()

	// Check if ES buffer has content (for logging purposes)
	var bufferBytes uint64
	if s.esBuffer != nil {
		stats := s.esBuffer.Stats()
		bufferBytes = stats.TotalBytes
	}

	// Only active transcoders count as active content.
	// Buffer data alone (without active transcoders) is considered stale
	// and the session can be cleaned up.
	hasActive := activeTranscoders > 0

	slog.Debug("HasActiveContent check",
		slog.String("session_id", s.ID.String()),
		slog.Int("total_transcoders", transcoderCount),
		slog.Int("active_transcoders", activeTranscoders),
		slog.Uint64("buffer_bytes", bufferBytes),
		slog.Bool("has_active_content", hasActive))

	return hasActive
}

// markReady signals that the session pipeline is ready for clients.
// This is called after the format router and processors are initialized.
func (s *RelaySession) markReady() {
	s.readyOnce.Do(func() {
		close(s.readyCh)
	})
}

// WaitReady blocks until the session pipeline is ready or the context is canceled.
// Returns nil if ready, context error if canceled, or ErrSessionClosed if session closed.
func (s *RelaySession) WaitReady(ctx context.Context) error {
	select {
	case <-s.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		// Session was closed before becoming ready
		if errPtr := s.lastErr.Load(); errPtr != nil {
			return *errPtr
		}
		return ErrSessionClosed
	}
}

// IsReady returns true if the session pipeline is ready for clients.
func (s *RelaySession) IsReady() bool {
	select {
	case <-s.readyCh:
		return true
	default:
		return false
	}
}

// GetContainerFormat returns the current container format.
// GetContainerFormat returns the current container format.
// This is set during pipeline init and read-only afterward.
func (s *RelaySession) GetContainerFormat() models.ContainerFormat {
	return s.containerFormat
}

// SupportsFormat returns true if the session can serve content in the requested format.
// SupportsFormat checks if the session supports the given output format.
// This is safe to call without locks because formatRouter is set once during
// pipeline init and only read afterward.
func (s *RelaySession) SupportsFormat(format string) bool {
	if s.formatRouter == nil {
		// Only MPEG-TS is supported without format router
		return format == FormatValueMPEGTS || format == ""
	}

	return s.formatRouter.HasHandler(format)
}

// GetOutputHandler returns the appropriate output handler for the requested format.
// GetOutputHandler returns the output handler for the given request.
// This is safe to call without locks because formatRouter is set once during
// pipeline init and only read afterward.
func (s *RelaySession) GetOutputHandler(req OutputRequest) (OutputHandler, error) {
	if s.formatRouter == nil {
		return nil, ErrNoHandlerAvailable
	}
	return s.formatRouter.GetHandler(req)
}
