package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	StreamURL      string
	Profile        *models.RelayProfile
	Classification ClassificationResult
	StartedAt      time.Time
	LastActivity   time.Time

	manager            *Manager
	buffer             *CyclicBuffer
	ctx                context.Context
	cancel             context.CancelFunc
	fallbackController *FallbackController
	fallbackGenerator  *FallbackGenerator

	mu           sync.RWMutex
	ffmpegCmd    *ffmpeg.CommandBuilder
	hlsCollapser *HLSCollapser
	inputReader  io.ReadCloser
	closed       bool
	err          error
	inFallback   bool
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

	builder := ffmpeg.NewCommandBuilder(binInfo.FFmpegPath).
		Input(inputURL).
		InputArgs("-re") // Read at native frame rate

	// Apply profile settings
	if s.Profile != nil {
		if s.Profile.VideoCodec == models.VideoCodecCopy {
			builder.VideoCodec("copy")
		} else {
			builder.VideoCodec(string(s.Profile.VideoCodec))
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

		// Apply hardware acceleration
		if s.Profile.HWAccel != models.HWAccelNone {
			builder.HWAccel(string(s.Profile.HWAccel))
		}
	}

	// Output to pipe
	builder.OutputFormat(string(s.Profile.OutputFormat)).
		Output("pipe:1").
		OutputArgs("-fflags", "+genpts")

	// Build and run FFmpeg, write to buffer
	cmd := builder.Build()
	writer := NewStreamWriter(s.buffer)
	return cmd.StreamToWriter(s.ctx, writer)
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
	s.mu.Unlock()

	return client, reader, nil
}

// RemoveClient removes a client from the session.
func (s *RelaySession) RemoveClient(clientID uuid.UUID) bool {
	return s.buffer.RemoveClient(clientID)
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

	stats := SessionStats{
		ID:             s.ID.String(),
		ChannelID:      s.ChannelID.String(),
		StreamURL:      s.StreamURL,
		Classification: s.Classification.Mode.String(),
		StartedAt:      s.StartedAt,
		LastActivity:   s.LastActivity,
		ClientCount:    bufferStats.ClientCount,
		BytesWritten:   bufferStats.TotalBytesWritten,
		Closed:         s.closed,
		Error:          errStr,
		InFallback:     s.inFallback,
	}

	// Include fallback controller stats if available
	if s.fallbackController != nil {
		ctrlStats := s.fallbackController.Stats()
		stats.FallbackEnabled = true
		stats.FallbackErrorCount = ctrlStats.ErrorCount
		stats.FallbackRecoveryAttempts = ctrlStats.RecoveryAttempts
	}

	return stats
}

// SessionStats holds session statistics.
type SessionStats struct {
	ID             string    `json:"id"`
	ChannelID      string    `json:"channel_id"`
	StreamURL      string    `json:"stream_url"`
	Classification string    `json:"classification"`
	StartedAt      time.Time `json:"started_at"`
	LastActivity   time.Time `json:"last_activity"`
	ClientCount    int       `json:"client_count"`
	BytesWritten   uint64    `json:"bytes_written"`
	Closed         bool      `json:"closed"`
	Error          string    `json:"error,omitempty"`
	// Fallback information
	InFallback              bool `json:"in_fallback"`
	FallbackEnabled         bool `json:"fallback_enabled"`
	FallbackErrorCount      int  `json:"fallback_error_count,omitempty"`
	FallbackRecoveryAttempts int `json:"fallback_recovery_attempts,omitempty"`
}
