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

	manager            *Manager
	buffer             *CyclicBuffer
	ctx                context.Context
	cancel             context.CancelFunc
	fallbackController *FallbackController
	fallbackGenerator  *FallbackGenerator

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
func (s *RelaySession) runNormalPipeline() error {
	// Determine pipeline based on profile and classification
	needsTranscoding := s.Profile != nil &&
		(s.Profile.VideoCodec != models.VideoCodecCopy ||
			s.Profile.AudioCodec != models.AudioCodecCopy)

	if needsTranscoding {
		return s.runFFmpegPipeline()
	} else if s.Classification.Mode == StreamModeCollapsedHLS {
		return s.runHLSCollapsePipeline()
	}
	return s.runPassthroughPipeline()
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

		// Apply hardware acceleration if determined
		if hwAccelType != "" {
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
		}
	}

	// INPUT FLAGS - Must come before -i
	// When we have cached codec info, we can use reduced probesize/analyzeduration
	// for faster startup since we already know the stream format
	if s.CachedCodecInfo != nil && s.CachedCodecInfo.IsValid() {
		// Minimal probing - we already know the codec from ffprobe cache
		builder.InputArgs("-analyzeduration", "1000000"). // 1 second
			InputArgs("-probesize", "1000000")            // 1MB
		slog.Debug("Using reduced probe settings with cached codec info",
			slog.String("video_codec", s.CachedCodecInfo.VideoCodec))
	} else {
		// Full probing - no cached info available
		builder.InputArgs("-analyzeduration", "10000000"). // 10 seconds
			InputArgs("-probesize", "10000000")            // 10MB
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

	// Determine video codec and family for BSF selection
	var videoCodecFamily ffmpeg.CodecFamily
	var videoCodec string
	var isVideoCopy bool

	if s.Profile != nil {
		videoCodec = string(s.Profile.VideoCodec)
		if s.Profile.VideoCodec == models.VideoCodecCopy {
			builder.VideoCodec("copy")
			isVideoCopy = true
			// For copy mode, use cached codec info if available for accurate BSF selection
			if s.CachedCodecInfo != nil && s.CachedCodecInfo.VideoCodec != "" {
				videoCodecFamily = ffmpeg.GetCodecFamily(s.CachedCodecInfo.VideoCodec)
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

		if s.Profile.AudioCodec == models.AudioCodecCopy {
			builder.AudioCodec("copy")
		} else {
			builder.AudioCodec(string(s.Profile.AudioCodec))
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

	// Determine output format
	outputFormat := ffmpeg.FormatMPEGTS
	if s.Profile != nil {
		outputFormat = ffmpeg.ParseOutputFormat(string(s.Profile.OutputFormat))
	}

	// CRITICAL: Apply appropriate bitstream filter based on codec, output format, and mode
	// BSF is ONLY needed when COPYING video (not transcoding) to convert container formats
	// When transcoding, the encoder outputs correct format and BSF would corrupt the stream
	bsfInfo := ffmpeg.GetVideoBitstreamFilter(videoCodecFamily, outputFormat, isVideoCopy)
	if bsfInfo.VideoBSF != "" {
		builder.VideoBitstreamFilter(bsfInfo.VideoBSF)
		slog.Debug("Applied video bitstream filter",
			slog.String("bsf", bsfInfo.VideoBSF),
			slog.String("codec_family", string(videoCodecFamily)),
			slog.String("output_format", string(outputFormat)),
			slog.Bool("is_copy", isVideoCopy),
			slog.String("reason", bsfInfo.Reason))
	} else {
		slog.Debug("No bitstream filter needed",
			slog.String("codec_family", string(videoCodecFamily)),
			slog.String("output_format", string(outputFormat)),
			slog.Bool("is_copy", isVideoCopy),
			slog.String("reason", bsfInfo.Reason))
	}

	// MPEG-TS output settings - use proven m3u-proxy configuration
	// This sets proper timestamp handling, PID allocation, and format flags
	if outputFormat == ffmpeg.FormatMPEGTS || outputFormat == ffmpeg.FormatHLS {
		builder.MpegtsArgs().     // Proper MPEG-TS flags (copyts, PIDs, etc.)
			FlushPackets().       // -flush_packets 1 - immediate output
			MuxDelay("0").        // -muxdelay 0 - zero muxing delay
			PatPeriod("0.1")      // -pat_period 0.1 - frequent PAT/PMT for mid-stream joins
	} else {
		// For non-MPEG-TS formats, just set the output format
		builder.OutputFormat(string(s.Profile.OutputFormat))
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
	// Fallback information
	InFallback               bool `json:"in_fallback"`
	FallbackEnabled          bool `json:"fallback_enabled"`
	FallbackErrorCount       int  `json:"fallback_error_count,omitempty"`
	FallbackRecoveryAttempts int  `json:"fallback_recovery_attempts,omitempty"`
	// FFmpeg process stats (only present when FFmpeg is running)
	FFmpegStats *FFmpegProcessStats `json:"ffmpeg_stats,omitempty"`
	// Connected client details
	Clients []ClientStats `json:"clients,omitempty"`
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
