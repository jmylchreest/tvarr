package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
)

// RelaySession represents an active relay session.
type RelaySession struct {
	ID             uuid.UUID
	ChannelID      uuid.UUID
	ChannelName    string // Display name of the channel
	StreamURL      string
	Profile        *models.RelayProfile
	Classification ClassificationResult
	CachedCodecInfo *models.LastKnownCodec // Pre-probed codec info for faster startup
	StartedAt      time.Time
	LastActivity   time.Time
	IdleSince      time.Time // When the session became idle (0 clients), zero if not idle

	// Smart delivery context (set when using smart mode)
	DeliveryContext *DeliveryContext

	manager            *Manager
	buffer             *CyclicBuffer    // Legacy buffer for MPEG-TS streaming (deprecated, use unifiedBuffer)
	unifiedBuffer      *UnifiedBuffer   // Unified buffer for all streaming formats
	ctx                context.Context
	cancel             context.CancelFunc
	fallbackController *FallbackController
	fallbackGenerator  *FallbackGenerator

	// Multi-format streaming support
	formatRouter     *FormatRouter           // Routes requests to appropriate output handler
	containerFormat  models.ContainerFormat  // Current container format

	// Passthrough handlers for HLS/DASH sources
	hlsPassthrough  *HLSPassthroughHandler
	dashPassthrough *DASHPassthroughHandler

	mu             sync.RWMutex
	ffmpegCmd      *ffmpeg.Command // Running FFmpeg command for stats access
	hlsCollapser   *HLSCollapser
	inputReader    io.ReadCloser
	closed         bool
	err            error
	inFallback     bool
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
		s.buffer.Close()
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
// If a DeliveryContext is set (smart mode), it uses the pre-computed decision.
// Otherwise, it falls back to the legacy logic for backward compatibility.
func (s *RelaySession) runNormalPipeline() error {
	// If smart delivery context is set, use its decision
	if s.DeliveryContext != nil {
		switch s.DeliveryContext.Decision {
		case DeliveryTranscode:
			return s.runFFmpegPipeline()
		case DeliveryRepackage:
			// Repackage means changing manifest format without re-encoding
			// This is handled at the handler level, but if we're in a relay session,
			// we need to serve the source with appropriate handlers
			// For now, treat as passthrough and let handlers do the manifest conversion
			slog.Info("Smart delivery: repackage mode (serving source with manifest conversion)",
				slog.String("session_id", s.ID.String()),
				slog.String("source_format", string(s.DeliveryContext.Source.SourceFormat)),
				slog.String("client_format", string(s.DeliveryContext.ClientFormat)))
			return s.runPassthroughBySourceFormat()
		case DeliveryPassthrough:
			return s.runPassthroughBySourceFormat()
		}
	}

	// Legacy logic: Determine pipeline based on profile and classification
	needsTranscoding := s.Profile != nil &&
		(s.Profile.VideoCodec != models.VideoCodecCopy ||
			s.Profile.AudioCodec != models.AudioCodecCopy)

	if needsTranscoding {
		return s.runFFmpegPipeline()
	}

	return s.runPassthroughBySourceFormat()
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
		// Raw MPEG-TS or unknown - use direct passthrough
		return s.runPassthroughPipeline()
	}
}

// runFallbackStream runs the fallback stream until recovery or cancellation.
func (s *RelaySession) runFallbackStream() error {
	if s.fallbackGenerator == nil || !s.fallbackGenerator.IsReady() {
		return ErrFallbackNotReady
	}

	streamer := NewFallbackStreamer(s.fallbackGenerator, nil)
	writer := NewStreamWriter(s.buffer)

	// Start a recovery check goroutine
	recoveryDone := make(chan bool, 1)
	go s.runRecoveryLoop(recoveryDone)

	// Stream fallback content until stopped
	err := streamer.Stream(s.ctx, writer)

	// Signal recovery goroutine to stop
	close(recoveryDone)

	return err
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

// runFFmpegPipeline runs the FFmpeg transcoding pipeline.
func (s *RelaySession) runFFmpegPipeline() error {
	// Detect FFmpeg
	binInfo, err := s.manager.ffmpegBin.Detect(s.ctx)
	if err != nil {
		return fmt.Errorf("detecting ffmpeg: %w", err)
	}

	// Build command
	inputURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		inputURL = s.Classification.SelectedMediaPlaylist
	}

	// Build FFmpeg command with proper settings for live streaming
	// Following m3u-proxy's proven approach for flag order:
	// 1. Global flags (banner, loglevel)
	// 2. Hardware acceleration args (init_hw_device, hwaccel) - BEFORE input
	// 3. Input analysis args (analyzeduration, probesize)
	// 4. Custom input options
	// 5. Input (-i)
	// 6. Stream mapping
	// 7. Video codec and hwaccel filters
	// 8. Video settings
	// 9. Audio codec and settings
	// 10. Transport stream settings
	// 11. Custom output options
	// 12. Output

	builder := ffmpeg.NewCommandBuilder(binInfo.FFmpegPath)

	// GLOBAL FLAGS - Hide banner, set log level
	builder.HideBanner().LogLevel("info") // Use 'info' to capture useful output

	// HARDWARE ACCELERATION - Must be initialized BEFORE input
	// Based on m3u-proxy's proven approach: init device, then hwaccel
	hwAccelType := ""
	if s.Profile != nil {
		// Determine the effective hwaccel type
		configuredHWAccel := s.Profile.HWAccel

		if configuredHWAccel == models.HWAccelAuto {
			// Auto-select best available hardware acceleration
			// Priority order matches m3u-proxy: vaapi → nvenc/cuda → qsv
			selectedAccel := ffmpeg.SelectBestHWAccel(binInfo.HWAccels)
			if selectedAccel != "" {
				hwAccelType = selectedAccel
				slog.Info("Auto-selected hardware acceleration",
					slog.String("type", hwAccelType),
					slog.String("profile", s.Profile.Name))
			} else {
				slog.Debug("No hardware acceleration available, using software encoding",
					slog.String("profile", s.Profile.Name))
			}
		} else if configuredHWAccel != models.HWAccelNone && configuredHWAccel != "" {
			// Use explicitly configured hwaccel
			hwAccelType = string(configuredHWAccel)

			// Verify the configured hwaccel is still available
			if !binInfo.HasHWAccel(ffmpeg.HWAccelType(hwAccelType)) {
				slog.Warn("Configured hardware acceleration not available, falling back to software",
					slog.String("configured", hwAccelType),
					slog.String("profile", s.Profile.Name))
				hwAccelType = ""
			}
		}

		// EARLY DECISION: Determine if video copy will be used BEFORE applying hwaccel
		// hwaccel is only useful for decoding/encoding, not for copy mode
		videoCodec := string(s.Profile.VideoCodec)
		willUseVideoCopy := s.Profile.VideoCodec == models.VideoCodecCopy
		if !willUseVideoCopy && !s.Profile.ForceVideoTranscode && s.CachedCodecInfo != nil && s.CachedCodecInfo.VideoCodec != "" {
			if ffmpeg.SourceMatchesTargetCodec(s.CachedCodecInfo.VideoCodec, videoCodec) {
				slog.Info("Smart codec match: source video matches target, using copy instead of transcode",
					slog.String("source_codec", s.CachedCodecInfo.VideoCodec),
					slog.String("target_encoder", videoCodec),
					slog.String("profile", s.Profile.Name))
				willUseVideoCopy = true
			}
		}

		// Apply hardware acceleration ONLY if we're actually going to transcode video
		// hwaccel is for decoding - if we're copying, there's no decoding/encoding happening
		if hwAccelType != "" && !willUseVideoCopy {
			// Initialize hardware device first (critical for proper HW acceleration)
			devicePath := s.Profile.HWAccelDevice
			if devicePath == "" && s.Profile.GpuIndex >= 0 {
				devicePath = fmt.Sprintf("%d", s.Profile.GpuIndex)
			}
			builder.InitHWDevice(hwAccelType, devicePath)

			// Set hwaccel mode
			builder.HWAccel(hwAccelType)
			if devicePath != "" {
				builder.HWAccelDevice(devicePath)
			}
			if s.Profile.HWAccelOutputFormat != "" {
				builder.HWAccelOutputFormat(s.Profile.HWAccelOutputFormat)
			}

			// Apply decoder codec for hardware decoding (e.g., h264_cuvid, hevc_qsv)
			if s.Profile.HWAccelDecoderCodec != "" {
				builder.InputArgs("-c:v", s.Profile.HWAccelDecoderCodec)
			}

			// Apply any extra hardware acceleration options
			if s.Profile.HWAccelExtraOptions != "" {
				builder.ApplyCustomInputOptions(s.Profile.HWAccelExtraOptions)
			}
		} else if willUseVideoCopy {
			slog.Debug("Skipping hwaccel for video copy mode",
				slog.String("hwaccel", hwAccelType),
				slog.String("profile", s.Profile.Name))
		}
	}

	// INPUT FLAGS - Must come before -i
	// When we have cached codec info, we can use reduced probesize/analyzeduration
	// for faster startup since we already know the stream format
	if s.CachedCodecInfo != nil && s.CachedCodecInfo.IsValid() {
		// Minimal probing - we already know the codec from ffprobe cache
		builder.InputArgs("-analyzeduration", "500000"). // 0.5 seconds
			InputArgs("-probesize", "500000")            // 500KB
		slog.Debug("Using reduced probe settings with cached codec info",
			slog.String("video_codec", s.CachedCodecInfo.VideoCodec))
	} else {
		// Live stream probing - 3 seconds is sufficient for MPEG-TS/HLS detection
		// Previous 10 second values caused significant startup delays
		builder.InputArgs("-analyzeduration", "3000000"). // 3 seconds
			InputArgs("-probesize", "3000000")            // 3MB
	}
	builder.Reconnect() // Enable auto-reconnect for network streams

	// Apply custom input options (allows user overrides) - AFTER standard input flags
	if s.Profile != nil && s.Profile.InputOptions != "" {
		builder.ApplyCustomInputOptions(s.Profile.InputOptions)
	}

	// Set input
	builder.Input(inputURL)

	// Explicit stream mapping for first video and audio streams
	builder.OutputArgs("-map", "0:v:0").
		OutputArgs("-map", "0:a:0?") // ? makes audio optional

	// Apply video codec and family for BSF selection
	// The copy decision was already made above, now apply it
	var videoCodecFamily ffmpeg.CodecFamily
	var isVideoCopy bool

	if s.Profile != nil {
		// Convert abstract codec type to actual FFmpeg encoder name
		videoCodec := s.Profile.VideoCodec.GetFFmpegEncoder(s.Profile.HWAccel)
		willUseVideoCopy := s.Profile.VideoCodec == models.VideoCodecCopy || s.Profile.VideoCodec == models.VideoCodecNone
		if !willUseVideoCopy && !s.Profile.ForceVideoTranscode && s.CachedCodecInfo != nil && s.CachedCodecInfo.VideoCodec != "" {
			willUseVideoCopy = ffmpeg.SourceMatchesTargetCodec(s.CachedCodecInfo.VideoCodec, videoCodec)
		}

		if willUseVideoCopy {
			builder.VideoCodec("copy")
			isVideoCopy = true
			// For copy mode, use cached codec info if available for accurate BSF selection
			if s.CachedCodecInfo != nil && s.CachedCodecInfo.VideoCodec != "" {
				videoCodecFamily = ffmpeg.MapFFprobeCodecToFamily(s.CachedCodecInfo.VideoCodec)
				slog.Debug("Using cached codec for BSF selection",
					slog.String("source_codec", s.CachedCodecInfo.VideoCodec),
					slog.String("codec_family", string(videoCodecFamily)))
			} else {
				// Default to H.264 family since it's most common
				videoCodecFamily = ffmpeg.CodecFamilyH264
				slog.Debug("No cached codec info, defaulting to H.264 for BSF selection")
			}
		} else {
			builder.VideoCodec(videoCodec)
			isVideoCopy = false
			videoCodecFamily = ffmpeg.GetCodecFamily(videoCodec)

			// Add hardware upload filter ONLY when using a hardware encoder
			// hwupload uploads frames to GPU memory which is only useful for HW encoding
			// If using software encoder (libx264, etc.) with HW decoding, FFmpeg auto-handles it
			if hwAccelType != "" && ffmpeg.IsHardwareEncoder(videoCodec) {
				builder.HWUploadFilter(hwAccelType)
			}

			if s.Profile.VideoBitrate > 0 {
				builder.VideoBitrate(fmt.Sprintf("%dk", s.Profile.VideoBitrate))
			}
			if s.Profile.VideoWidth > 0 || s.Profile.VideoHeight > 0 {
				builder.VideoScale(s.Profile.VideoWidth, s.Profile.VideoHeight)
			}
			if s.Profile.VideoPreset != "" {
				builder.VideoPreset(s.Profile.VideoPreset)
			}
		}

		// Smart codec matching for audio: if source matches target and ForceAudioTranscode is false, use copy
		useAudioCopy := s.Profile.AudioCodec == models.AudioCodecCopy
		if !useAudioCopy && !s.Profile.ForceAudioTranscode && s.CachedCodecInfo != nil && s.CachedCodecInfo.AudioCodec != "" {
			if ffmpeg.SourceMatchesTargetCodec(s.CachedCodecInfo.AudioCodec, string(s.Profile.AudioCodec)) {
				slog.Info("Smart codec match: source audio matches target, using copy instead of transcode",
					slog.String("source_codec", s.CachedCodecInfo.AudioCodec),
					slog.String("target_encoder", string(s.Profile.AudioCodec)),
					slog.String("profile", s.Profile.Name))
				useAudioCopy = true
			}
		}

		if useAudioCopy {
			builder.AudioCodec("copy")
		} else {
			audioEncoder := s.Profile.AudioCodec.GetFFmpegEncoder()
			if audioEncoder != "" {
				builder.AudioCodec(audioEncoder)
			}
			if s.Profile.AudioBitrate > 0 {
				builder.AudioBitrate(fmt.Sprintf("%dk", s.Profile.AudioBitrate))
			}
			if s.Profile.AudioSampleRate > 0 {
				builder.AudioSampleRate(s.Profile.AudioSampleRate)
			}
			if s.Profile.AudioChannels > 0 {
				builder.AudioChannels(s.Profile.AudioChannels)
			}
		}
	}

	// Apply custom filter complex
	if s.Profile != nil && s.Profile.FilterComplex != "" {
		builder.ApplyFilterComplex(s.Profile.FilterComplex)
	}

	// Determine container format (new) vs output format (legacy)
	// ContainerFormat is preferred as it separates container from manifest format
	containerFormat := ffmpeg.FormatMPEGTS
	if s.Profile != nil {
		// Use DetermineContainer() which handles auto-selection based on codecs
		container := s.Profile.DetermineContainer()
		switch container {
		case models.ContainerFormatFMP4:
			containerFormat = ffmpeg.FormatFMP4
		case models.ContainerFormatMPEGTS:
			containerFormat = ffmpeg.FormatMPEGTS
		default:
			// Default to MPEG-TS for maximum compatibility
			containerFormat = ffmpeg.FormatMPEGTS
		}
	}

	// CRITICAL: Apply appropriate bitstream filter based on codec, output format, and mode
	// BSF is ONLY needed when COPYING video (not transcoding) to convert container formats
	// When transcoding, the encoder outputs correct format and BSF would corrupt the stream
	bsfInfo := ffmpeg.GetVideoBitstreamFilter(videoCodecFamily, containerFormat, isVideoCopy)
	if bsfInfo.VideoBSF != "" {
		builder.VideoBitstreamFilter(bsfInfo.VideoBSF)
		slog.Debug("Applied video bitstream filter",
			slog.String("bsf", bsfInfo.VideoBSF),
			slog.String("codec_family", string(videoCodecFamily)),
			slog.String("container_format", string(containerFormat)),
			slog.Bool("is_copy", isVideoCopy),
			slog.String("reason", bsfInfo.Reason))
	} else {
		slog.Debug("No bitstream filter needed",
			slog.String("codec_family", string(videoCodecFamily)),
			slog.String("container_format", string(containerFormat)),
			slog.Bool("is_copy", isVideoCopy),
			slog.String("reason", bsfInfo.Reason))
	}

	// Configure output format based on container
	switch containerFormat {
	case ffmpeg.FormatFMP4:
		// fMP4/CMAF output for HLS v7+ and DASH compatibility
		// Use segment duration from profile, default to 6 seconds
		fragDuration := float64(DefaultSegmentDuration)
		if s.Profile != nil && s.Profile.SegmentDuration > 0 {
			fragDuration = float64(s.Profile.SegmentDuration)
		}
		builder.FMP4Args(fragDuration).
			FlushPackets() // Immediate output for live streaming
		slog.Info("Configured fMP4/CMAF output",
			slog.Float64("frag_duration", fragDuration),
			slog.String("profile", s.Profile.Name))
	case ffmpeg.FormatMPEGTS, ffmpeg.FormatHLS:
		// MPEG-TS output settings - use proven m3u-proxy configuration
		// This sets proper timestamp handling, PID allocation, and format flags
		builder.MpegtsArgs().     // Proper MPEG-TS flags (copyts, PIDs, etc.)
			FlushPackets().       // -flush_packets 1 - immediate output
			MuxDelay("0").        // -muxdelay 0 - zero muxing delay
			PatPeriod("0.1")      // -pat_period 0.1 - frequent PAT/PMT for mid-stream joins
	default:
		// Default case - use MPEG-TS args for compatibility
		builder.MpegtsArgs().
			FlushPackets().
			MuxDelay("0").
			PatPeriod("0.1")
	}

	// Apply custom output options (allows user overrides)
	if s.Profile != nil && s.Profile.OutputOptions != "" {
		builder.ApplyCustomOutputOptions(s.Profile.OutputOptions)
	}

	// Set output destination
	builder.Output("pipe:1")

	// Build command and log it for debugging
	cmd := builder.Build()
	slog.Info("Starting FFmpeg relay",
		slog.String("profile", s.Profile.Name),
		slog.String("stream_url", inputURL),
		slog.String("video_codec", string(s.Profile.VideoCodec)),
		slog.String("audio_codec", string(s.Profile.AudioCodec)))
	slog.Debug("FFmpeg command details",
		slog.String("command", cmd.String()),
		slog.Any("args", cmd.Args))

	// Store command for stats access
	s.mu.Lock()
	s.ffmpegCmd = cmd
	s.mu.Unlock()

	// Run FFmpeg with retry logic for startup failures
	writer := NewStreamWriter(s.buffer)
	retryCfg := ffmpeg.DefaultRetryConfig()
	err = cmd.StreamToWriterWithRetry(s.ctx, writer, retryCfg)

	// Clear command reference when done
	s.mu.Lock()
	s.ffmpegCmd = nil
	s.mu.Unlock()

	return err
}

// runHLSCollapsePipeline runs the HLS collapsing pipeline.
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

	// Read from collapser and write to buffer
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-s.ctx.Done():
			collapser.Stop()
			return s.ctx.Err()
		default:
		}

		n, err := collapser.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, ErrCollapserAborted) {
				return nil
			}
			return err
		}

		if n > 0 {
			if err := s.buffer.WriteChunk(buf[:n]); err != nil {
				return err
			}
			s.mu.Lock()
			s.LastActivity = time.Now()
			s.mu.Unlock()
		}
	}
}

// runPassthroughPipeline runs the direct passthrough pipeline.
func (s *RelaySession) runPassthroughPipeline() error {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.StreamURL, nil)
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

	// Read from upstream and write to buffer
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if n > 0 {
			if err := s.buffer.WriteChunk(buf[:n]); err != nil {
				return err
			}
			s.mu.Lock()
			s.LastActivity = time.Now()
			s.mu.Unlock()
		}
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

// AddClient adds a client to the session and returns a reader.
func (s *RelaySession) AddClient(userAgent, remoteAddr string) (*BufferClient, *StreamReader, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, nil, ErrSessionClosed
	}
	s.mu.RUnlock()

	client, err := s.buffer.AddClient(userAgent, remoteAddr)
	if err != nil {
		return nil, nil, err
	}

	reader := NewStreamReader(s.buffer, client)

	s.mu.Lock()
	s.LastActivity = time.Now()
	s.IdleSince = time.Time{} // Clear idle state when a client connects
	s.mu.Unlock()

	return client, reader, nil
}

// RemoveClient removes a client from the session.
func (s *RelaySession) RemoveClient(clientID uuid.UUID) bool {
	removed := s.buffer.RemoveClient(clientID)
	if removed {
		// Check if session is now idle (no clients)
		if s.buffer.ClientCount() == 0 {
			s.mu.Lock()
			s.IdleSince = time.Now()
			s.mu.Unlock()
		}
	}
	return removed
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

	if s.inputReader != nil {
		s.inputReader.Close()
	}

	s.buffer.Close()
}

// Stats returns session statistics.
func (s *RelaySession) Stats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bufferStats := s.buffer.Stats()

	var errStr string
	if s.err != nil {
		errStr = s.err.Error()
	}

	// Get profile name if available
	profileName := "passthrough"
	if s.Profile != nil {
		profileName = s.Profile.Name
	}

	stats := SessionStats{
		ID:                s.ID.String(),
		ChannelID:         s.ChannelID.String(),
		ChannelName:       s.ChannelName,
		ProfileName:       profileName,
		StreamURL:         s.StreamURL,
		Classification:    s.Classification.Mode.String(),
		StartedAt:         s.StartedAt,
		LastActivity:      s.LastActivity,
		IdleSince:         s.IdleSince,
		ClientCount:       bufferStats.ClientCount,
		BytesWritten:      bufferStats.TotalBytesWritten,
		BytesFromUpstream: bufferStats.BytesFromUpstream,
		Closed:            s.closed,
		Error:             errStr,
		InFallback:        s.inFallback,
		Clients:           bufferStats.Clients,
	}

	// Include smart delivery context if available
	if s.DeliveryContext != nil {
		stats.DeliveryDecision = s.DeliveryContext.Decision.String()
		stats.ClientFormat = string(s.DeliveryContext.ClientFormat)
		stats.SourceFormat = string(s.DeliveryContext.Source.SourceFormat)
	}

	// Include fallback controller stats if available
	if s.fallbackController != nil {
		ctrlStats := s.fallbackController.Stats()
		stats.FallbackEnabled = true
		stats.FallbackErrorCount = ctrlStats.ErrorCount
		stats.FallbackRecoveryAttempts = ctrlStats.RecoveryAttempts
	}

	// Include FFmpeg process stats if running
	if s.ffmpegCmd != nil {
		if procStats := s.ffmpegCmd.ProcessStats(); procStats != nil {
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

	// Include segment buffer stats for HLS/DASH sessions
	if s.unifiedBuffer != nil {
		ubStats := s.unifiedBuffer.Stats()
		stats.SegmentBufferStats = &SessionSegmentBufferStats{
			ChunkCount:         ubStats.ChunkCount,
			SegmentCount:       ubStats.SegmentCount,
			BufferSizeBytes:    ubStats.BufferSize,
			TotalBytesWritten:  ubStats.TotalBytesWritten,
			FirstSegment:       ubStats.FirstSegment,
			LastSegment:        ubStats.LastSegment,
			MaxBufferSizeBytes: ubStats.MaxBufferSize,
			BufferUtilization:  ubStats.BufferUtilization,
			AverageChunkSize:   ubStats.AverageChunkSize,
			AverageSegmentSize: ubStats.AverageSegmentSize,
			TotalChunksWritten: ubStats.TotalChunksWritten,
		}
	}

	return stats
}

// SessionStats holds session statistics.
type SessionStats struct {
	ID             string    `json:"id"`
	ChannelID      string    `json:"channel_id"`
	ChannelName    string    `json:"channel_name,omitempty"`
	ProfileName    string    `json:"profile_name,omitempty"`
	StreamURL      string    `json:"stream_url"`
	Classification string    `json:"classification"`
	StartedAt      time.Time `json:"started_at"`
	LastActivity   time.Time `json:"last_activity"`
	IdleSince      time.Time `json:"idle_since,omitempty"`
	ClientCount    int       `json:"client_count"`
	BytesWritten   uint64    `json:"bytes_written"`
	BytesFromUpstream uint64 `json:"bytes_from_upstream"`
	Closed         bool      `json:"closed"`
	Error          string    `json:"error,omitempty"`
	// Smart delivery information (only present when using smart mode)
	DeliveryDecision string `json:"delivery_decision,omitempty"` // passthrough, repackage, or transcode
	ClientFormat     string `json:"client_format,omitempty"`     // requested output format
	SourceFormat     string `json:"source_format,omitempty"`     // detected source format
	// Fallback information
	InFallback               bool `json:"in_fallback"`
	FallbackEnabled          bool `json:"fallback_enabled"`
	FallbackErrorCount       int  `json:"fallback_error_count,omitempty"`
	FallbackRecoveryAttempts int  `json:"fallback_recovery_attempts,omitempty"`
	// FFmpeg process stats (only present when FFmpeg is running)
	FFmpegStats *FFmpegProcessStats `json:"ffmpeg_stats,omitempty"`
	// Connected client details
	Clients []ClientStats `json:"clients,omitempty"`
	// Segment buffer stats (only present for HLS/DASH sessions)
	SegmentBufferStats *SessionSegmentBufferStats `json:"segment_buffer_stats,omitempty"`
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
	PID            int     `json:"pid"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryRSSMB    float64 `json:"memory_rss_mb"`
	MemoryPercent  float64 `json:"memory_percent"`
	BytesWritten   uint64  `json:"bytes_written"`
	WriteRateMbps  float64 `json:"write_rate_mbps"`
	DurationSecs   float64 `json:"duration_secs"`
}

// InitMultiFormatOutput initializes the unified buffer and format router for multi-format output.
// This should be called when the session's output format is HLS or DASH.
func (s *RelaySession) InitMultiFormatOutput() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Determine unified buffer config from profile
	config := DefaultUnifiedBufferConfig()
	if s.Profile != nil {
		if s.Profile.SegmentDuration > 0 {
			config.TargetSegmentDuration = s.Profile.SegmentDuration
		}
		if s.Profile.PlaylistSize > 0 {
			config.MaxSegments = s.Profile.PlaylistSize
		}

		// Set container format for fMP4 mode
		// Use DetermineContainer() for smart auto-selection
		container := s.Profile.DetermineContainer()
		config.ContainerFormat = string(container)
		s.containerFormat = container
	}

	// Create unified buffer (replaces both CyclicBuffer and SegmentBuffer)
	s.unifiedBuffer = NewUnifiedBuffer(config)

	// Create format router with default format from profile
	defaultFormat := models.ContainerFormatMPEGTS
	if s.Profile != nil {
		defaultFormat = s.Profile.DetermineContainer()
	}
	s.formatRouter = NewFormatRouter(defaultFormat)

	// Register format handlers using the unified buffer as SegmentProvider
	// The unified buffer implements SegmentProvider interface
	s.formatRouter.RegisterHandler(FormatValueHLS, NewHLSHandler(s.unifiedBuffer))
	s.formatRouter.RegisterHandler(FormatValueDASH, NewDASHHandler(s.unifiedBuffer))
}

// GetUnifiedBuffer returns the unified buffer for all streaming formats.
func (s *RelaySession) GetUnifiedBuffer() *UnifiedBuffer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unifiedBuffer
}

// GetSegmentProvider returns the segment provider (implements SegmentProvider interface).
// This can be used by handlers that need to access segments.
func (s *RelaySession) GetSegmentProvider() SegmentProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unifiedBuffer
}

// GetFormatRouter returns the format router for handling output format requests.
func (s *RelaySession) GetFormatRouter() *FormatRouter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.formatRouter
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
