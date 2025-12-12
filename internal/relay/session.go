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

	"github.com/google/uuid"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
)

// Variant cleanup configuration
const (
	// VariantCleanupInterval is how often to check for unused variants
	VariantCleanupInterval = 30 * time.Second

	// VariantIdleTimeout is how long a variant can be unused before cleanup
	VariantIdleTimeout = 60 * time.Second
)

// ProcessorConfig holds configuration for on-demand processor creation.
type ProcessorConfig struct {
	TargetVariant         CodecVariant
	TargetSegmentDuration float64
	MaxSegments           int
	PlaylistSegments      int // Number of segments in playlist for new clients (near live edge)
}

// RelaySession represents an active relay session.
type RelaySession struct {
	ID               uuid.UUID
	ChannelID        uuid.UUID
	ChannelName      string // Display name of the channel
	StreamSourceName string // Name of the stream source (e.g., "s8k")
	StreamURL        string
	Profile          *models.RelayProfile
	Classification   ClassificationResult
	CachedCodecInfo  *models.LastKnownCodec // Pre-probed codec info for faster startup
	StartedAt        time.Time

	// Use atomic values for frequently updated fields to avoid mutex contention
	// These are updated by the ingest loop on every read, which would block stats collection
	lastActivity atomic.Value // time.Time - last activity timestamp
	idleSince    atomic.Value // time.Time - when session became idle (zero if not idle)

	// Smart delivery context (set when using smart mode)
	DeliveryContext *DeliveryContext

	manager            *Manager
	ctx                context.Context
	cancel             context.CancelFunc
	fallbackController *FallbackController
	fallbackGenerator  *FallbackGenerator

	// Multi-format streaming support
	formatRouter    *FormatRouter          // Routes requests to appropriate output handler
	containerFormat models.ContainerFormat // Current container format

	// Passthrough handlers for HLS/DASH sources
	hlsPassthrough  *HLSPassthroughHandler
	dashPassthrough *DASHPassthroughHandler

	// Elementary stream based processing (multi-variant codec support)
	esBuffer        *SharedESBuffer  // Shared ES buffer for multi-variant codec processing
	tsDemuxer       *TSDemuxer       // MPEG-TS demuxer for ES extraction
	processorConfig *ProcessorConfig // Config for on-demand processor creation

	// Per-variant processors (keyed by CodecVariant for multi-client codec support)
	// This allows different clients to receive different codec variants from the same session
	hlsTSProcessors   map[CodecVariant]*HLSTSProcessor   // HLS-TS processors per variant
	hlsFMP4Processors map[CodecVariant]*HLSfMP4Processor // HLS-fMP4 processors per variant
	dashProcessors    map[CodecVariant]*DASHProcessor    // DASH processors per variant
	mpegtsProcessors  map[CodecVariant]*MPEGTSProcessor  // MPEG-TS processors per variant
	processorsMu      sync.RWMutex                       // Protects processor creation/access
	esTranscoders     []*FFmpegTranscoder                // Active transcoders for codec variants
	esTranscodersMu   sync.RWMutex

	mu            sync.RWMutex
	ffmpegCmd     *ffmpeg.Command // Running FFmpeg command for stats access
	hlsCollapser  *HLSCollapser
	hlsRepackager *HLSRepackager // HLS-to-HLS repackaging (container format change)
	inputReader   io.ReadCloser
	closed        bool
	err           error
	inFallback    bool

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
		s.mu.Lock()
		s.err = err
		s.closed = true
		s.mu.Unlock()
	}()

	for {
		// Check if we're in fallback mode
		s.mu.RLock()
		inFallback := s.inFallback
		s.mu.RUnlock()

		if inFallback {
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
			return
		}

		// If error occurred and fallback is configured, switch to fallback
		if err != nil && s.fallbackController != nil {
			if s.fallbackController.CheckError(err.Error()) {
				s.mu.Lock()
				s.inFallback = true
				s.mu.Unlock()
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
	// If smart delivery context is set, use its decision for special cases
	if s.DeliveryContext != nil {
		switch s.DeliveryContext.Decision {
		case DeliveryRepackage:
			// Repackage means changing container format without re-encoding
			// Use HLSRepackager for HLS-to-HLS with different segment types
			if s.Classification.SourceFormat == SourceFormatHLS {
				slog.Info("Smart delivery: HLS repackage mode",
					slog.String("session_id", s.ID.String()),
					slog.String("source_format", string(s.DeliveryContext.Source.SourceFormat)),
					slog.String("client_format", string(s.DeliveryContext.ClientFormat)))
				return s.runHLSRepackagePipeline()
			}
			// For non-HLS sources, fall through to ES pipeline
		case DeliveryPassthrough:
			// Check if we can use native passthrough handlers for HLS/DASH sources
			switch s.Classification.Mode {
			case StreamModePassthroughHLS, StreamModeTransparentHLS:
				return s.runHLSPassthroughPipeline()
			case StreamModePassthroughDASH:
				return s.runDASHPassthroughPipeline()
			}
			// Otherwise fall through to ES pipeline
		}
	}

	// Handle special source format cases
	switch s.Classification.Mode {
	case StreamModeCollapsedHLS:
		return s.runHLSCollapsePipeline()
	case StreamModePassthroughHLS, StreamModeTransparentHLS:
		// Check if profile requires transcoding - if so, use ES pipeline
		if s.Profile != nil && s.Profile.NeedsTranscode() {
			slog.Info("HLS source requires transcoding, using ES pipeline",
				slog.String("session_id", s.ID.String()),
				slog.String("video_codec", string(s.Profile.VideoCodec)),
				slog.String("audio_codec", string(s.Profile.AudioCodec)))
			return s.runESPipeline()
		}
		return s.runHLSPassthroughPipeline()
	case StreamModePassthroughDASH:
		// Check if profile requires transcoding - if so, use ES pipeline
		if s.Profile != nil && s.Profile.NeedsTranscode() {
			slog.Info("DASH source requires transcoding, using ES pipeline",
				slog.String("session_id", s.ID.String()),
				slog.String("video_codec", string(s.Profile.VideoCodec)),
				slog.String("audio_codec", string(s.Profile.AudioCodec)))
			return s.runESPipeline()
		}
		return s.runDASHPassthroughPipeline()
	default:
		// Raw MPEG-TS or unknown - use ES pipeline for all processing
		// The ES pipeline handles both passthrough and on-demand transcoding
		return s.runESPipeline()
	}
}

// runPassthroughBySourceFormat runs the appropriate passthrough pipeline based on source format.
func (s *RelaySession) runPassthroughBySourceFormat() error {
	// Handle passthrough based on source format classification
	switch s.Classification.Mode {
	case StreamModeCollapsedHLS:
		return s.runHLSCollapsePipeline()
	case StreamModePassthroughHLS, StreamModeTransparentHLS:
		// Use HLS passthrough handler for HLS sources
		return s.runHLSPassthroughPipeline()
	case StreamModePassthroughDASH:
		// Use DASH passthrough handler for DASH sources
		return s.runDASHPassthroughPipeline()
	default:
		// Raw MPEG-TS or unknown - use ES-based pipeline for proper HLS segment generation
		// The ES pipeline demuxes MPEG-TS to elementary streams, then muxes back to HLS segments
		// with proper timing, enabling multi-variant codec support
		slog.Info("Using ES-based pipeline for raw MPEG-TS source",
			slog.String("session_id", s.ID.String()),
			slog.String("stream_url", s.StreamURL))
		return s.runESPipeline()
	}
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
			s.mu.Lock()
			s.inFallback = false
			s.mu.Unlock()

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
// 3. If target != source, FFmpegTranscoder is spawned to read from source and write to target
// 4. Processors read from appropriate variant

// runHLSCollapsePipeline runs the HLS collapsing pipeline.
// This uses the ES pipeline to demux collapsed HLS content and remux for output.
func (s *RelaySession) runHLSCollapsePipeline() error {
	playlistURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		playlistURL = s.Classification.SelectedMediaPlaylist
	}

	collapser := NewHLSCollapser(s.manager.config.HTTPClient, playlistURL)

	s.mu.Lock()
	s.hlsCollapser = collapser
	s.mu.Unlock()

	if err := collapser.Start(s.ctx); err != nil {
		return fmt.Errorf("starting HLS collapser: %w", err)
	}

	// Initialize the ES pipeline to process collapsed HLS content
	esConfig := DefaultSharedESBufferConfig()
	esConfig.Logger = slog.Default()
	s.esBuffer = NewSharedESBuffer(s.ChannelID.String(), s.ID.String(), esConfig)

	// Set up the transcoding callback
	s.esBuffer.SetVariantRequestCallback(s.handleVariantRequest)

	// Create TS demuxer to parse incoming MPEG-TS from collapser
	demuxerConfig := TSDemuxerConfig{
		Logger: slog.Default(),
	}
	s.tsDemuxer = NewTSDemuxer(s.esBuffer, demuxerConfig)

	// Get processor settings from profile
	var targetSegmentDuration float64 = 6.0
	var maxSegments = 7
	if s.Profile != nil {
		if s.Profile.SegmentDuration > 0 {
			targetSegmentDuration = float64(s.Profile.SegmentDuration)
		}
		if s.Profile.PlaylistSize > 0 {
			maxSegments = s.Profile.PlaylistSize
		}
	}

	// Initialize processor maps
	s.hlsTSProcessors = make(map[CodecVariant]*HLSTSProcessor)
	s.mpegtsProcessors = make(map[CodecVariant]*MPEGTSProcessor)

	// Start processors for the default variant (VariantCopy for passthrough)
	hlsTSConfig := DefaultHLSTSProcessorConfig()
	hlsTSConfig.Logger = slog.Default()
	hlsTSConfig.TargetSegmentDuration = targetSegmentDuration
	hlsTSConfig.MaxSegments = maxSegments

	hlsTSProcessor := NewHLSTSProcessor(
		fmt.Sprintf("hls-ts-%s-%s", s.ID.String(), VariantCopy.String()),
		s.esBuffer,
		VariantCopy,
		hlsTSConfig,
	)
	if err := hlsTSProcessor.Start(s.ctx); err != nil {
		return fmt.Errorf("starting HLS-TS processor: %w", err)
	}
	s.hlsTSProcessors[VariantCopy] = hlsTSProcessor

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
		s.mpegtsProcessors[VariantCopy] = mpegtsProcessor
	}

	// Set up format router
	s.formatRouter = NewFormatRouter(models.ContainerFormatMPEGTS)
	s.formatRouter.RegisterHandler(FormatValueHLS, NewHLSHandler(hlsTSProcessor))
	if mpegtsProcessor != nil {
		s.formatRouter.RegisterHandler(FormatValueMPEGTS, NewMPEGTSHandler(mpegtsProcessor))
	}

	// Signal that the pipeline is ready for clients
	s.markReady()

	slog.Info("Started HLS collapse pipeline with ES processing",
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
			if err := s.tsDemuxer.Write(buf[:n]); err != nil {
				slog.Warn("Demuxer error", slog.String("error", err.Error()))
			}
			// Use atomic store to avoid blocking stats collection with mutex
			s.lastActivity.Store(time.Now())
		}
	}
}

// runHLSRepackagePipeline runs the HLS repackaging pipeline.
// This uses HLSRepackager to convert HLS sources to HLS with potentially different
// segment container formats (e.g., MPEG-TS to fMP4 or vice versa) without transcoding.
func (s *RelaySession) runHLSRepackagePipeline() error {
	// Determine the playlist URL (use selected media playlist if available)
	playlistURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		playlistURL = s.Classification.SelectedMediaPlaylist
	}

	// Determine the output variant based on profile container format preference
	// Default to MPEG-TS for maximum compatibility with legacy HLS clients
	outputVariant := HLSMuxerVariantMPEGTS
	if s.Profile != nil {
		// Use the profile's container format setting to determine variant
		container := s.Profile.DetermineContainer()
		switch container {
		case models.ContainerFormatFMP4:
			outputVariant = HLSMuxerVariantFMP4
		case models.ContainerFormatMPEGTS:
			outputVariant = HLSMuxerVariantMPEGTS
		}
	}

	// Configure segment settings from profile
	segmentCount := 7
	segmentDuration := 1 * time.Second
	if s.Profile != nil {
		if s.Profile.PlaylistSize > 0 {
			segmentCount = s.Profile.PlaylistSize
		}
		if s.Profile.SegmentDuration > 0 {
			segmentDuration = time.Duration(s.Profile.SegmentDuration) * time.Second
		}
	}

	// Create the HLS repackager
	repackager := NewHLSRepackager(HLSRepackagerConfig{
		SourceURL:          playlistURL,
		OutputVariant:      outputVariant,
		SegmentCount:       segmentCount,
		SegmentMinDuration: segmentDuration,
		HTTPClient:         s.manager.config.HTTPClient,
		Logger:             slog.Default(),
	})

	s.mu.Lock()
	s.hlsRepackager = repackager
	s.mu.Unlock()

	// Start the repackager
	if err := repackager.Start(); err != nil {
		return fmt.Errorf("starting HLS repackager: %w", err)
	}

	// Signal that the pipeline is ready for clients
	s.markReady()

	slog.Info("Started HLS repackage pipeline",
		slog.String("session_id", s.ID.String()),
		slog.String("source_url", playlistURL),
		slog.String("output_variant", outputVariant.String()))

	// Wait for context cancellation or repackager error
	for {
		select {
		case <-s.ctx.Done():
			repackager.Close()
			return s.ctx.Err()
		default:
		}

		// Check for repackager error
		if err := repackager.Error(); err != nil {
			return fmt.Errorf("HLS repackager error: %w", err)
		}

		// Check if repackager closed unexpectedly
		if repackager.IsClosed() {
			if err := repackager.Error(); err != nil {
				return fmt.Errorf("HLS repackager closed with error: %w", err)
			}
			return nil // Clean shutdown
		}

		// Update activity timestamp using atomic to avoid blocking stats collection
		s.lastActivity.Store(time.Now())

		// Small sleep to avoid busy loop
		time.Sleep(100 * time.Millisecond)
	}
}

// runHLSPassthroughPipeline runs the HLS passthrough proxy pipeline.
// This initializes the HLS passthrough handler and waits for cancellation.
// The handler serves requests directly via the format router.
func (s *RelaySession) runHLSPassthroughPipeline() error {
	// Get the upstream playlist URL
	playlistURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		playlistURL = s.Classification.SelectedMediaPlaylist
	}

	// Build the base URL for proxy URLs
	// This will be used by the handler to rewrite segment URLs
	baseURL := s.buildProxyBaseURL()

	// Initialize HLS passthrough handler
	config := DefaultHLSPassthroughConfig()
	config.HTTPClient = s.manager.config.HTTPClient
	s.hlsPassthrough = NewHLSPassthroughHandler(playlistURL, baseURL, config)

	// Register with format router if available
	s.mu.Lock()
	if s.formatRouter != nil {
		s.formatRouter.RegisterPassthroughHandler(FormatValueHLS, s.hlsPassthrough)
	}
	s.mu.Unlock()

	// Signal that the pipeline is ready for clients
	s.markReady()

	slog.Info("Started HLS passthrough pipeline",
		slog.String("session_id", s.ID.String()),
		slog.String("upstream_url", playlistURL),
		slog.String("base_url", baseURL))

	// Wait for context cancellation - passthrough handlers serve on demand
	<-s.ctx.Done()
	return s.ctx.Err()
}

// runDASHPassthroughPipeline runs the DASH passthrough proxy pipeline.
// This initializes the DASH passthrough handler and waits for cancellation.
// The handler serves requests directly via the format router.
func (s *RelaySession) runDASHPassthroughPipeline() error {
	// Build the base URL for proxy URLs
	baseURL := s.buildProxyBaseURL()

	// Initialize DASH passthrough handler
	config := DefaultDASHPassthroughConfig()
	config.HTTPClient = s.manager.config.HTTPClient
	s.dashPassthrough = NewDASHPassthroughHandler(s.StreamURL, baseURL, config)

	// Register with format router if available
	s.mu.Lock()
	if s.formatRouter != nil {
		s.formatRouter.RegisterPassthroughHandler(FormatValueDASH, s.dashPassthrough)
	}
	s.mu.Unlock()

	// Signal that the pipeline is ready for clients
	s.markReady()

	slog.Info("Started DASH passthrough pipeline",
		slog.String("session_id", s.ID.String()),
		slog.String("upstream_url", s.StreamURL),
		slog.String("base_url", baseURL))

	// Wait for context cancellation - passthrough handlers serve on demand
	<-s.ctx.Done()
	return s.ctx.Err()
}

// buildProxyBaseURL constructs the base URL for passthrough proxy URLs.
func (s *RelaySession) buildProxyBaseURL() string {
	// This should be constructed based on the server's configuration
	// For now, use a placeholder that will be replaced by the handler
	// The actual base URL would come from server config or request context
	return fmt.Sprintf("/api/v1/relay/stream/%s", s.ID.String())
}

// getTargetVariant returns the target codec variant based on the profile settings.
// If the profile specifies transcoding, this returns the target variant (e.g., "h264/aac").
// If no transcoding is needed, this returns VariantCopy which means use source codecs.
// When processors request a variant that differs from the source, the on-demand
// transcoding callback (handleVariantRequest) will spawn an FFmpegTranscoder.
func (s *RelaySession) getTargetVariant() CodecVariant {
	if s.Profile == nil || !s.Profile.NeedsTranscode() {
		return VariantCopy
	}

	// Build target variant from profile codecs
	videoCodec := string(s.Profile.VideoCodec)
	audioCodec := string(s.Profile.AudioCodec)

	// Check for unresolved "auto" codecs - this indicates a bug in the call chain
	// Auto codecs should be resolved via client detection BEFORE the session is created
	if videoCodec == "auto" || audioCodec == "auto" {
		slog.Warn("Profile has unresolved 'auto' codecs at session level - falling back to copy",
			slog.String("session_id", s.ID.String()),
			slog.String("profile_name", s.Profile.Name),
			slog.String("video_codec", videoCodec),
			slog.String("audio_codec", audioCodec))
		return VariantCopy
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

	return MakeCodecVariant(videoCodec, audioCodec)
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
// 3. If target != source, handleVariantRequest spawns FFmpegTranscoder
// 4. FFmpegTranscoder reads from source variant, writes to target variant
// 5. Processors read from target variant
func (s *RelaySession) runESPipeline() error {
	// Initialize the shared ES buffer
	esConfig := DefaultSharedESBufferConfig()
	esConfig.Logger = slog.Default()
	s.esBuffer = NewSharedESBuffer(s.ChannelID.String(), s.ID.String(), esConfig)

	// Set up the transcoding callback - spawns FFmpeg transcoder when new codec variant is requested
	s.esBuffer.SetVariantRequestCallback(s.handleVariantRequest)

	// Create TS demuxer to parse incoming MPEG-TS and populate ES buffer
	// The demuxer writes to the source variant (VariantCopy)
	demuxerConfig := TSDemuxerConfig{
		Logger: slog.Default(),
		// No TargetVariant - writes to source variant ("copy/copy")
	}
	s.tsDemuxer = NewTSDemuxer(s.esBuffer, demuxerConfig)

	// Determine the source URL
	inputURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		inputURL = s.Classification.SelectedMediaPlaylist
	}

	// Get the target variant from profile - this is what processors will request
	// If target differs from source, on-demand transcoding will be triggered
	targetVariant := s.getTargetVariant()

	// Get processor settings from profile
	var targetSegmentDuration float64 = 6.0
	var maxSegments = 7
	if s.Profile != nil {
		if s.Profile.SegmentDuration > 0 {
			targetSegmentDuration = float64(s.Profile.SegmentDuration)
		}
		if s.Profile.PlaylistSize > 0 {
			maxSegments = s.Profile.PlaylistSize
		}
	}

	slog.Info("Starting ES pipeline",
		slog.String("session_id", s.ID.String()),
		slog.String("target_variant", targetVariant.String()),
		slog.Bool("needs_transcode", s.Profile != nil && s.Profile.NeedsTranscode()))

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
			}
			return fmt.Errorf("waiting for source variant: %w", err)
		}
		// Source is ready, continue
	case ingestErr := <-ingestErrCh:
		// Ingest failed before source was detected
		if ingestErr != nil {
			return fmt.Errorf("ingest failed: %w", ingestErr)
		}
		// Ingest completed without error but source not ready - shouldn't happen
		return fmt.Errorf("ingest completed without detecting source codecs")
	case <-s.ctx.Done():
		return s.ctx.Err()
	}

	slog.Info("Source variant ready, pipeline initialized",
		slog.String("session_id", s.ID.String()),
		slog.String("source_variant", s.esBuffer.SourceVariantKey().String()))

	// Store processor config for on-demand creation
	// Processors are NOT created here - they are created on-demand when clients connect
	s.processorConfig = &ProcessorConfig{
		TargetVariant:         targetVariant,
		TargetSegmentDuration: targetSegmentDuration,
		MaxSegments:           maxSegments,
	}

	// Set up format router WITHOUT pre-created processors
	// Processors will be created on-demand via GetOrCreateProcessor()
	s.formatRouter = NewFormatRouter(models.ContainerFormatMPEGTS)

	// Signal that the pipeline is ready for clients
	// Clients will trigger on-demand processor creation when they connect
	s.markReady()

	slog.Info("Started ES-based pipeline (processors created on-demand)",
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
	// Fetch upstream MPEG-TS and feed to demuxer
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, inputURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.manager.config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	s.mu.Lock()
	s.inputReader = resp.Body
	s.mu.Unlock()

	// Update last activity using atomic to avoid blocking stats collection
	s.lastActivity.Store(time.Now())

	// Read from upstream and feed to demuxer
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-s.ctx.Done():
			demuxer.Flush()
			return s.ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				demuxer.Flush()
				return nil
			}
			return err
		}

		if n > 0 {
			if err := demuxer.Write(buf[:n]); err != nil {
				slog.Warn("Demuxer error",
					slog.String("error", err.Error()))
			}
			// Use atomic store to avoid blocking stats collection with mutex
			s.lastActivity.Store(time.Now())
		}
	}
}

// HLSClientIdleTimeout is how long an HLS/DASH client can be inactive before being removed.
// HLS clients make periodic requests for playlists/segments, so 30s covers ~5 segment requests.
const HLSClientIdleTimeout = 30 * time.Second

// runVariantCleanupLoop periodically cleans up unused transcoded variants and their transcoders.
// This prevents memory leaks when clients stop requesting certain codec variants.
// It also cleans up inactive HLS/DASH clients that haven't made requests recently.
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

// cleanupInactiveStreamingClients removes HLS/DASH clients that haven't made requests recently.
// This is necessary because HLS/DASH are request-based protocols without persistent connections,
// unlike MPEG-TS which maintains a persistent streaming connection.
func (s *RelaySession) cleanupInactiveStreamingClients() {
	s.processorsMu.RLock()
	defer s.processorsMu.RUnlock()

	totalRemoved := 0

	// Clean up inactive clients from all HLS-TS processors
	for variant, processor := range s.hlsTSProcessors {
		if processor != nil {
			removed := processor.CleanupInactiveClients(HLSClientIdleTimeout)
			if removed > 0 {
				totalRemoved += removed
				slog.Debug("Cleaned up inactive HLS-TS clients",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()),
					slog.Int("removed", removed))
			}
		}
	}

	// Clean up inactive clients from all HLS-fMP4 processors
	for variant, processor := range s.hlsFMP4Processors {
		if processor != nil {
			removed := processor.CleanupInactiveClients(HLSClientIdleTimeout)
			if removed > 0 {
				totalRemoved += removed
				slog.Debug("Cleaned up inactive HLS-fMP4 clients",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()),
					slog.Int("removed", removed))
			}
		}
	}

	// Clean up inactive clients from all DASH processors
	for variant, processor := range s.dashProcessors {
		if processor != nil {
			removed := processor.CleanupInactiveClients(HLSClientIdleTimeout)
			if removed > 0 {
				totalRemoved += removed
				slog.Debug("Cleaned up inactive DASH clients",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()),
					slog.Int("removed", removed))
			}
		}
	}

	// If we removed clients, update session idle state
	if totalRemoved > 0 {
		s.UpdateIdleState()
	}
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

		slog.Info("Cleaned up unused variants",
			slog.String("session_id", s.ID.String()),
			slog.Int("removed_count", removed))
	}
}

// stopTranscodersForVariants stops all transcoders producing the specified variants.
func (s *RelaySession) stopTranscodersForVariants(variants map[CodecVariant]bool) {
	s.esTranscodersMu.Lock()
	defer s.esTranscodersMu.Unlock()

	// Find and stop transcoders for removed variants
	var remaining []*FFmpegTranscoder
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

// cleanupIdleTranscoders stops transcoders that have been idle too long.
// This is called in addition to variant cleanup to handle transcoders that
// may have stopped producing output.
func (s *RelaySession) cleanupIdleTranscoders() {
	s.esTranscodersMu.Lock()
	defer s.esTranscodersMu.Unlock()

	cutoff := time.Now().Add(-VariantIdleTimeout)
	var remaining []*FFmpegTranscoder

	for _, transcoder := range s.esTranscoders {
		stats := transcoder.Stats()
		if stats.LastActivity.Before(cutoff) && !stats.LastActivity.IsZero() {
			slog.Debug("Stopping idle transcoder",
				slog.String("session_id", s.ID.String()),
				slog.String("target_variant", stats.TargetVariant.String()),
				slog.Duration("idle_time", time.Since(stats.LastActivity)))
			transcoder.Stop()
		} else {
			remaining = append(remaining, transcoder)
		}
	}
	s.esTranscoders = remaining
}

// handleVariantRequest is called when a processor requests a codec variant that doesn't exist.
// It spawns an FFmpeg transcoder to produce the requested variant from the source.
func (s *RelaySession) handleVariantRequest(source, target CodecVariant) error {
	// Detect FFmpeg
	binInfo, err := s.manager.ffmpegBin.Detect(s.ctx)
	if err != nil {
		return fmt.Errorf("detecting ffmpeg: %w", err)
	}

	var transcoder *FFmpegTranscoder

	// Try to create transcoder from profile if available, otherwise use variant directly
	if s.Profile != nil {
		// Use profile for full configuration (bitrate, preset, hwaccel, etc.)
		transcoder, err = CreateTranscoderFromProfile(
			fmt.Sprintf("transcoder-%s-%s", s.ID.String(), target.String()),
			s.esBuffer,
			source,
			s.Profile,
			binInfo,
			slog.Default(),
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

		transcoder, err = CreateTranscoderFromVariant(
			fmt.Sprintf("transcoder-%s-%s", s.ID.String(), target.String()),
			s.esBuffer,
			source,
			target,
			binInfo,
			slog.Default(),
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

	slog.Info("Started transcoder for variant request",
		slog.String("session_id", s.ID.String()),
		slog.String("source", source.String()),
		slog.String("target", target.String()))

	return nil
}

// GetESBuffer returns the shared ES buffer for this session.
func (s *RelaySession) GetESBuffer() *SharedESBuffer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.esBuffer
}

// GetHLSTSProcessor returns the HLS-TS processor for the default variant if it exists.
// Returns nil if the processor hasn't been created yet.
func (s *RelaySession) GetHLSTSProcessor() *HLSTSProcessor {
	s.processorsMu.RLock()
	defer s.processorsMu.RUnlock()
	if s.hlsTSProcessors == nil {
		return nil
	}
	// Return the processor for the session's default variant
	if s.processorConfig != nil {
		return s.hlsTSProcessors[s.processorConfig.TargetVariant]
	}
	// Fall back to VariantCopy
	return s.hlsTSProcessors[VariantCopy]
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
	s.processorsMu.Lock()
	defer s.processorsMu.Unlock()

	// Initialize map if needed
	if s.hlsTSProcessors == nil {
		s.hlsTSProcessors = make(map[CodecVariant]*HLSTSProcessor)
	}

	// Check if processor already exists for this variant
	if processor, exists := s.hlsTSProcessors[variant]; exists {
		return processor, nil
	}

	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

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

	s.hlsTSProcessors[variant] = processor

	slog.Info("Created HLS-TS processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_hls_ts_processors", len(s.hlsTSProcessors)))

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
	s.processorsMu.Lock()
	defer s.processorsMu.Unlock()

	// Initialize map if needed
	if s.hlsFMP4Processors == nil {
		s.hlsFMP4Processors = make(map[CodecVariant]*HLSfMP4Processor)
	}

	// Check if processor already exists for this variant
	if processor, exists := s.hlsFMP4Processors[variant]; exists {
		return processor, nil
	}

	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

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

	s.hlsFMP4Processors[variant] = processor

	slog.Info("Created HLS-fMP4 processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_hls_fmp4_processors", len(s.hlsFMP4Processors)))

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
	s.processorsMu.Lock()
	defer s.processorsMu.Unlock()

	// Initialize map if needed
	if s.dashProcessors == nil {
		s.dashProcessors = make(map[CodecVariant]*DASHProcessor)
	}

	// Check if processor already exists for this variant
	if processor, exists := s.dashProcessors[variant]; exists {
		return processor, nil
	}

	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

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

	s.dashProcessors[variant] = processor

	slog.Info("Created DASH processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_dash_processors", len(s.dashProcessors)))

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
	s.processorsMu.Lock()
	defer s.processorsMu.Unlock()

	// Initialize map if needed
	if s.mpegtsProcessors == nil {
		s.mpegtsProcessors = make(map[CodecVariant]*MPEGTSProcessor)
	}

	// Check if processor already exists for this variant
	if processor, exists := s.mpegtsProcessors[variant]; exists {
		return processor, nil
	}

	if s.processorConfig == nil || s.esBuffer == nil {
		return nil, errors.New("session not ready for processor creation")
	}

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

	s.mpegtsProcessors[variant] = processor

	slog.Info("Created MPEG-TS processor on-demand",
		slog.String("session_id", s.ID.String()),
		slog.String("variant", variant.String()),
		slog.Int("total_mpegts_processors", len(s.mpegtsProcessors)))

	return processor, nil
}

// ClientCount returns the number of connected clients.
// With the ES pipeline architecture, clients are tracked per-processor across all variants.
func (s *RelaySession) ClientCount() int {
	s.processorsMu.RLock()
	defer s.processorsMu.RUnlock()

	count := 0

	// Count clients across all HLS-TS processors
	for _, p := range s.hlsTSProcessors {
		if p != nil {
			count += p.ClientCount()
		}
	}

	// Count clients across all HLS-fMP4 processors
	for _, p := range s.hlsFMP4Processors {
		if p != nil {
			count += p.ClientCount()
		}
	}

	// Count clients across all DASH processors
	for _, p := range s.dashProcessors {
		if p != nil {
			count += p.ClientCount()
		}
	}

	// Count clients across all MPEG-TS processors
	for _, p := range s.mpegtsProcessors {
		if p != nil {
			count += p.ClientCount()
		}
	}

	return count
}

// UpdateIdleState updates the session's idle tracking based on current client count.
// This should be called after clients connect or disconnect.
// When client count drops to 0, IdleSince is set to the current time.
// When clients reconnect, IdleSince is cleared.
func (s *RelaySession) UpdateIdleState() {
	clientCount := s.ClientCount()

	s.mu.Lock()
	defer s.mu.Unlock()

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

// StopProcessorIfIdle stops idle processors of a given type across all variants.
// This is called when a client disconnects to immediately clean up unused processors.
func (s *RelaySession) StopProcessorIfIdle(processorType string) {
	s.processorsMu.Lock()
	defer s.processorsMu.Unlock()

	switch processorType {
	case "mpegts":
		for variant, processor := range s.mpegtsProcessors {
			if processor != nil && processor.ClientCount() == 0 {
				slog.Info("Stopping idle MPEG-TS processor",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()))
				processor.Stop()
				delete(s.mpegtsProcessors, variant)
			}
		}
	case "hls-ts":
		for variant, processor := range s.hlsTSProcessors {
			if processor != nil && processor.ClientCount() == 0 {
				slog.Info("Stopping idle HLS-TS processor",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()))
				processor.Stop()
				delete(s.hlsTSProcessors, variant)
			}
		}
	case "hls-fmp4":
		for variant, processor := range s.hlsFMP4Processors {
			if processor != nil && processor.ClientCount() == 0 {
				slog.Info("Stopping idle HLS-fMP4 processor",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()))
				processor.Stop()
				delete(s.hlsFMP4Processors, variant)
			}
		}
	case "dash":
		for variant, processor := range s.dashProcessors {
			if processor != nil && processor.ClientCount() == 0 {
				slog.Info("Stopping idle DASH processor",
					slog.String("session_id", s.ID.String()),
					slog.String("variant", variant.String()))
				processor.Stop()
				delete(s.dashProcessors, variant)
			}
		}
	}

	// Update session idle state after processor cleanup
	s.mu.Lock()
	clientCount := s.ClientCount()
	idleSince, _ := s.idleSince.Load().(time.Time)
	if clientCount == 0 && idleSince.IsZero() {
		s.idleSince.Store(time.Now())
		slog.Debug("Session became idle after processor cleanup",
			slog.String("session_id", s.ID.String()))
	}
	s.mu.Unlock()
}

// Close closes the session and releases resources.
func (s *RelaySession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

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

	// Clean up ES-based processing resources
	s.esTranscodersMu.Lock()
	for _, transcoder := range s.esTranscoders {
		transcoder.Stop()
	}
	s.esTranscoders = nil
	s.esTranscodersMu.Unlock()

	// Stop all processors across all variants
	s.processorsMu.Lock()
	for _, processor := range s.hlsTSProcessors {
		if processor != nil {
			processor.Stop()
		}
	}
	s.hlsTSProcessors = nil

	for _, processor := range s.hlsFMP4Processors {
		if processor != nil {
			processor.Stop()
		}
	}
	s.hlsFMP4Processors = nil

	for _, processor := range s.dashProcessors {
		if processor != nil {
			processor.Stop()
		}
	}
	s.dashProcessors = nil

	for _, processor := range s.mpegtsProcessors {
		if processor != nil {
			processor.Stop()
		}
	}
	s.mpegtsProcessors = nil
	s.processorsMu.Unlock()

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
	var deliveryContext *DeliveryContext
	var classification ClassificationResult

	s.mu.RLock()
	if s.err != nil {
		errStr = s.err.Error()
	}
	profileName = "passthrough"
	if s.Profile != nil {
		profileName = s.Profile.Name
	}
	closed = s.closed
	inFallback = s.inFallback
	esBuffer = s.esBuffer
	ffmpegCmd = s.ffmpegCmd
	fallbackController = s.fallbackController
	cachedCodecInfo = s.CachedCodecInfo
	deliveryContext = s.DeliveryContext
	classification = s.Classification
	s.mu.RUnlock()

	// Get stats from ES buffer if available (has its own lock)
	var bytesWritten uint64
	if esBuffer != nil {
		esStats := esBuffer.Stats()
		bytesWritten = esStats.TotalBytes
	}

	// Collect clients from all processors across all variants (separate lock scope)
	var clientCount int
	var clients []ClientStats

	s.processorsMu.RLock()

	// MPEG-TS processor clients (all variants)
	for _, processor := range s.mpegtsProcessors {
		if processor != nil {
			clientCount += processor.ClientCount()
			for _, c := range processor.GetClients() {
				clients = append(clients, ClientStats{
					ID:           c.ID,
					BytesRead:    c.BytesRead.Load(),
					ConnectedAt:  c.ConnectedAt,
					UserAgent:    c.UserAgent,
					RemoteAddr:   c.RemoteAddr,
					ClientFormat: "mpegts",
				})
			}
		}
	}

	// HLS TS processor clients (all variants)
	for _, processor := range s.hlsTSProcessors {
		if processor != nil {
			clientCount += processor.ClientCount()
			for _, c := range processor.GetClients() {
				clients = append(clients, ClientStats{
					ID:           c.ID,
					BytesRead:    c.BytesRead.Load(),
					ConnectedAt:  c.ConnectedAt,
					UserAgent:    c.UserAgent,
					RemoteAddr:   c.RemoteAddr,
					ClientFormat: "hls",
				})
			}
		}
	}

	// HLS fMP4 processor clients (all variants)
	for _, processor := range s.hlsFMP4Processors {
		if processor != nil {
			clientCount += processor.ClientCount()
			for _, c := range processor.GetClients() {
				clients = append(clients, ClientStats{
					ID:           c.ID,
					BytesRead:    c.BytesRead.Load(),
					ConnectedAt:  c.ConnectedAt,
					UserAgent:    c.UserAgent,
					RemoteAddr:   c.RemoteAddr,
					ClientFormat: "hls",
				})
			}
		}
	}

	// DASH processor clients (all variants)
	for _, processor := range s.dashProcessors {
		if processor != nil {
			clientCount += processor.ClientCount()
			for _, c := range processor.GetClients() {
				clients = append(clients, ClientStats{
					ID:           c.ID,
					BytesRead:    c.BytesRead.Load(),
					ConnectedAt:  c.ConnectedAt,
					UserAgent:    c.UserAgent,
					RemoteAddr:   c.RemoteAddr,
					ClientFormat: "dash",
				})
			}
		}
	}

	s.processorsMu.RUnlock()

	// Build stats from immutable fields (set at creation, no lock needed)
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
		Error:             errStr,
		InFallback:        inFallback,
		Clients:           clients,
	}

	// Include smart delivery context if available
	if deliveryContext != nil {
		stats.DeliveryDecision = deliveryContext.Decision.String()
		stats.ClientFormat = string(deliveryContext.ClientFormat)
		stats.SourceFormat = string(deliveryContext.Source.SourceFormat)
	}

	// Fallback to Classification source format if DeliveryContext not available
	if stats.SourceFormat == "" && classification.SourceFormat != "" {
		stats.SourceFormat = string(classification.SourceFormat)
	}

	// Collect active processor formats - already collected above with processorsMu
	// Rebuild from the processor maps we already iterated
	s.processorsMu.RLock()
	activeFormats := make([]string, 0, 4)
	if len(s.mpegtsProcessors) > 0 {
		activeFormats = append(activeFormats, "mpegts")
	}
	if len(s.hlsTSProcessors) > 0 {
		activeFormats = append(activeFormats, "hls")
	}
	if len(s.hlsFMP4Processors) > 0 {
		// Only add hls-fmp4 if hlsTS isn't already there (avoid duplicate HLS entries)
		hasHLS := false
		for _, f := range activeFormats {
			if f == "hls" {
				hasHLS = true
				break
			}
		}
		if !hasHLS {
			activeFormats = append(activeFormats, "hls")
		}
	}
	if len(s.dashProcessors) > 0 {
		activeFormats = append(activeFormats, "dash")
	}
	s.processorsMu.RUnlock()
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
			stats.FFmpegStats = &FFmpegProcessStats{
				PID:           procStats.PID,
				CPUPercent:    procStats.CPUPercent,
				MemoryRSSMB:   procStats.MemoryRSSMB,
				MemoryPercent: procStats.MemoryPercent,
				BytesWritten:  procStats.BytesWritten,
				WriteRateMbps: procStats.WriteRateMbps,
				DurationSecs:  procStats.Duration.Seconds(),
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

		for _, transcoder := range s.esTranscoders {
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
			})
		}

		stats.ESTranscoderStats = &ESTranscodersStats{
			Count:          len(s.esTranscoders),
			TotalBytesIn:   totalBytesIn,
			TotalBytesOut:  totalBytesOut,
			TotalSamplesIn: totalSamplesIn,
			Transcoders:    transcoderList,
		}

		// If no legacy FFmpeg stats but we have ES transcoders, populate FFmpegStats
		// with actual process stats so the flow visualization shows transcoding
		if stats.FFmpegStats == nil && len(s.esTranscoders) > 0 {
			// Get encoding config from first transcoder for display
			firstStats := s.esTranscoders[0].Stats()
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
}

// ClientStats holds statistics for a single connected client.
type ClientStats struct {
	ID           string    `json:"id"`
	BytesRead    uint64    `json:"bytes_read"`
	LastSequence uint64    `json:"last_sequence"`
	ConnectedAt  time.Time `json:"connected_at"`
	LastRead     time.Time `json:"last_read,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	RemoteAddr   string    `json:"remote_addr,omitempty"`
	ClientFormat string    `json:"client_format,omitempty"` // Format this client is using (hls, mpegts, dash)
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
}

// GetFormatRouter returns the format router for handling output format requests.
func (s *RelaySession) GetFormatRouter() *FormatRouter {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
		s.mu.RLock()
		err := s.err
		s.mu.RUnlock()
		if err != nil {
			return err
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
func (s *RelaySession) GetContainerFormat() models.ContainerFormat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.containerFormat
}

// SupportsFormat returns true if the session can serve content in the requested format.
func (s *RelaySession) SupportsFormat(format string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.formatRouter == nil {
		// Only MPEG-TS is supported without format router
		return format == FormatValueMPEGTS || format == ""
	}

	// Check for passthrough mode first
	if s.formatRouter.IsPassthroughMode(format) {
		return true
	}

	return s.formatRouter.HasHandler(format)
}

// GetOutputHandler returns the appropriate output handler for the requested format.
func (s *RelaySession) GetOutputHandler(req OutputRequest) (OutputHandler, error) {
	s.mu.RLock()
	router := s.formatRouter
	s.mu.RUnlock()

	if router == nil {
		return nil, ErrNoHandlerAvailable
	}
	return router.GetHandler(req)
}

// IsPassthroughMode returns true if the session is using passthrough handlers.
func (s *RelaySession) IsPassthroughMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hlsPassthrough != nil || s.dashPassthrough != nil
}

// GetHLSPassthrough returns the HLS passthrough handler if available.
func (s *RelaySession) GetHLSPassthrough() *HLSPassthroughHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hlsPassthrough
}

// GetDASHPassthrough returns the DASH passthrough handler if available.
func (s *RelaySession) GetDASHPassthrough() *DASHPassthroughHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dashPassthrough
}

// demuxerWriter wraps TSDemuxer to implement io.Writer for FFmpeg output.
// It feeds FFmpeg's MPEG-TS output to the demuxer which populates the ES buffer.
type demuxerWriter struct {
	demuxer *TSDemuxer
	session *RelaySession
}

// Write implements io.Writer, feeding data to the demuxer.
func (dw *demuxerWriter) Write(p []byte) (int, error) {
	if err := dw.demuxer.Write(p); err != nil {
		slog.Warn("Demuxer write error", slog.String("error", err.Error()))
		// Don't fail the write - demuxer errors shouldn't stop FFmpeg
	}

	// Update activity timestamp using atomic to avoid blocking stats collection
	dw.session.lastActivity.Store(time.Now())

	return len(p), nil
}
