// Package relay provides stream relay functionality including transcoding,
// connection pooling, and failure handling.
package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// FallbackConfig holds configuration for fallback stream generation.
type FallbackConfig struct {
	// Width of the fallback video frame.
	Width int
	// Height of the fallback video frame.
	Height int
	// Duration of each fallback segment in seconds.
	SegmentDuration float64
	// Message to display on the fallback slate.
	Message string
	// BackgroundColor in FFmpeg format (e.g., "black", "0x1a1a1a").
	BackgroundColor string
	// TextColor in FFmpeg format (e.g., "white", "0xffffff").
	TextColor string
	// FontSize for the message text.
	FontSize int
	// VideoBitrate in kbps for fallback stream.
	VideoBitrate int
	// AudioEnabled adds silent audio track if true.
	AudioEnabled bool
	// FFmpegPath is the path to ffmpeg binary.
	FFmpegPath string
}

// DefaultFallbackConfig returns sensible defaults for fallback generation.
func DefaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		Width:           1280,
		Height:          720,
		SegmentDuration: 2.0,
		Message:         "Stream Unavailable",
		BackgroundColor: "black",
		TextColor:       "white",
		FontSize:        48,
		VideoBitrate:    1000,
		AudioEnabled:    true,
		FFmpegPath:      "ffmpeg",
	}
}

// ErrFallbackGenerationFailed is returned when fallback stream generation fails.
var ErrFallbackGenerationFailed = errors.New("fallback stream generation failed")

// ErrFallbackNotReady is returned when fallback data hasn't been generated yet.
var ErrFallbackNotReady = errors.New("fallback stream not ready")

// FallbackGenerator generates and caches fallback MPEG-TS segments.
type FallbackGenerator struct {
	config FallbackConfig
	logger *slog.Logger

	mu          sync.RWMutex
	initialized bool
	tsData      []byte
	lastGenTime time.Time
}

// NewFallbackGenerator creates a new fallback generator.
func NewFallbackGenerator(config FallbackConfig, logger *slog.Logger) *FallbackGenerator {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackGenerator{
		config: config,
		logger: logger,
	}
}

// Initialize generates the fallback TS segment.
// This should be called at startup to pre-generate the fallback slate.
func (f *FallbackGenerator) Initialize(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.initialized {
		return nil
	}

	f.logger.Info("generating fallback stream slate",
		slog.Int("width", f.config.Width),
		slog.Int("height", f.config.Height),
		slog.String("message", f.config.Message),
	)

	data, err := f.generateTS(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFallbackGenerationFailed, err)
	}

	f.tsData = data
	f.initialized = true
	f.lastGenTime = time.Now()

	f.logger.Info("fallback stream slate generated",
		slog.Int("bytes", len(data)),
	)

	return nil
}

// generateTS creates an MPEG-TS segment using FFmpeg.
func (f *FallbackGenerator) generateTS(ctx context.Context) ([]byte, error) {
	// Build FFmpeg command to generate a slate with text overlay
	// Using lavfi to generate video and audio
	duration := fmt.Sprintf("%.1f", f.config.SegmentDuration)

	// Video filter: black background with centered text
	videoFilter := fmt.Sprintf(
		"color=c=%s:s=%dx%d:d=%s,drawtext=text='%s':fontcolor=%s:fontsize=%d:x=(w-text_w)/2:y=(h-text_h)/2",
		f.config.BackgroundColor,
		f.config.Width,
		f.config.Height,
		duration,
		escapeFFmpegText(f.config.Message),
		f.config.TextColor,
		f.config.FontSize,
	)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-f", "lavfi",
		"-i", videoFilter,
	}

	// Add silent audio if enabled
	if f.config.AudioEnabled {
		audioFilter := fmt.Sprintf("anullsrc=r=48000:cl=stereo:d=%s", duration)
		args = append(args, "-f", "lavfi", "-i", audioFilter)
	}

	// Video encoding settings
	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "stillimage",
		"-b:v", fmt.Sprintf("%dk", f.config.VideoBitrate),
		"-pix_fmt", "yuv420p",
	)

	// Audio encoding settings (if enabled)
	if f.config.AudioEnabled {
		args = append(args,
			"-c:a", "aac",
			"-b:a", "128k",
		)
	}

	// Output settings
	args = append(args,
		"-f", "mpegts",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, f.config.FFmpegPath, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %v, stderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// GetSegment returns the pre-generated fallback TS segment.
func (f *FallbackGenerator) GetSegment() ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if !f.initialized {
		return nil, ErrFallbackNotReady
	}

	// Return a copy to prevent mutation
	data := make([]byte, len(f.tsData))
	copy(data, f.tsData)
	return data, nil
}

// IsReady returns true if the fallback slate has been generated.
func (f *FallbackGenerator) IsReady() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.initialized
}

// Stats returns statistics about the fallback generator.
func (f *FallbackGenerator) Stats() FallbackStats {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return FallbackStats{
		Initialized:     f.initialized,
		SegmentSize:     len(f.tsData),
		LastGenerated:   f.lastGenTime,
		Width:           f.config.Width,
		Height:          f.config.Height,
		SegmentDuration: f.config.SegmentDuration,
	}
}

// FallbackStats holds statistics about fallback generation.
type FallbackStats struct {
	Initialized     bool      `json:"initialized"`
	SegmentSize     int       `json:"segment_size"`
	LastGenerated   time.Time `json:"last_generated,omitempty"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	SegmentDuration float64   `json:"segment_duration"`
}

// escapeFFmpegText escapes special characters for FFmpeg drawtext filter.
func escapeFFmpegText(text string) string {
	// Escape characters that have special meaning in drawtext
	// IMPORTANT: Backslashes must be escaped FIRST to avoid infinite loops
	// when the replacement itself contains the search character
	replacements := []struct{ old, new string }{
		{"\\", "\\\\"},
		{"'", "\\'"},
		{":", "\\:"},
	}
	for _, r := range replacements {
		text = replaceAllSafe(text, r.old, r.new)
	}
	return text
}

// replaceAllSafe is a string replacement helper that only replaces each occurrence once.
// It processes the string from left to right, appending to a result buffer.
func replaceAllSafe(s, old, new string) string {
	if old == "" {
		return s
	}
	var result []byte
	for {
		idx := indexString(s, old)
		if idx < 0 {
			result = append(result, s...)
			break
		}
		result = append(result, s[:idx]...)
		result = append(result, new...)
		s = s[idx+len(old):]
	}
	return string(result)
}

// indexString returns the index of substr in s, or -1 if not found.
func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// FallbackStreamer provides a continuous stream of fallback segments.
type FallbackStreamer struct {
	generator *FallbackGenerator
	logger    *slog.Logger

	mu       sync.RWMutex
	stopped  bool
	interval time.Duration
}

// NewFallbackStreamer creates a new fallback streamer.
func NewFallbackStreamer(generator *FallbackGenerator, logger *slog.Logger) *FallbackStreamer {
	if logger == nil {
		logger = slog.Default()
	}
	return &FallbackStreamer{
		generator: generator,
		logger:    logger,
		interval:  time.Duration(generator.config.SegmentDuration * float64(time.Second)),
	}
}

// Stream writes fallback segments continuously until context is cancelled.
func (s *FallbackStreamer) Stream(ctx context.Context, w io.Writer) error {
	if !s.generator.IsReady() {
		return ErrFallbackNotReady
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Write first segment immediately
	segment, err := s.generator.GetSegment()
	if err != nil {
		return err
	}
	if _, err := w.Write(segment); err != nil {
		return fmt.Errorf("writing fallback segment: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.mu.RLock()
			stopped := s.stopped
			s.mu.RUnlock()

			if stopped {
				return nil
			}

			segment, err := s.generator.GetSegment()
			if err != nil {
				return err
			}
			if _, err := w.Write(segment); err != nil {
				return fmt.Errorf("writing fallback segment: %w", err)
			}
		}
	}
}

// Stop stops the fallback streamer.
func (s *FallbackStreamer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
}

// ErrorPatternDetector monitors FFmpeg stderr for error patterns.
type ErrorPatternDetector struct {
	logger *slog.Logger

	mu           sync.Mutex
	errorCount   int
	lastError    time.Time
	errorPatterns []string
}

// DefaultErrorPatterns returns common FFmpeg error patterns.
func DefaultErrorPatterns() []string {
	return []string{
		"Connection refused",
		"Connection reset by peer",
		"Connection timed out",
		"No route to host",
		"Network is unreachable",
		"Server returned 4",   // HTTP 4xx errors
		"Server returned 5",   // HTTP 5xx errors
		"Invalid data found",
		"Stream not found",
		"End of file",
		"I/O error",
		"Broken pipe",
	}
}

// NewErrorPatternDetector creates a new error pattern detector.
func NewErrorPatternDetector(logger *slog.Logger) *ErrorPatternDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &ErrorPatternDetector{
		logger:        logger,
		errorPatterns: DefaultErrorPatterns(),
	}
}

// SetPatterns sets custom error patterns to detect.
func (d *ErrorPatternDetector) SetPatterns(patterns []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.errorPatterns = patterns
}

// CheckLine checks a line of FFmpeg output for error patterns.
// Returns true if an error pattern was detected.
func (d *ErrorPatternDetector) CheckLine(line string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, pattern := range d.errorPatterns {
		if containsString(line, pattern) {
			d.errorCount++
			d.lastError = time.Now()
			d.logger.Warn("ffmpeg error pattern detected",
				slog.String("pattern", pattern),
				slog.String("line", truncateString(line, 200)),
				slog.Int("error_count", d.errorCount),
			)
			return true
		}
	}
	return false
}

// ErrorCount returns the number of errors detected.
func (d *ErrorPatternDetector) ErrorCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.errorCount
}

// LastErrorTime returns when the last error was detected.
func (d *ErrorPatternDetector) LastErrorTime() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastError
}

// Reset resets the error count.
func (d *ErrorPatternDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.errorCount = 0
}

// containsString checks if s contains substr (case-insensitive).
func containsString(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	return indexString(sLower, substrLower) >= 0
}

// toLower converts a string to lowercase (simple ASCII version).
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FallbackController manages fallback state and recovery for a session.
type FallbackController struct {
	generator        *FallbackGenerator
	errorDetector    *ErrorPatternDetector
	logger           *slog.Logger

	// Configuration
	errorThreshold   int
	recoveryInterval time.Duration

	mu             sync.RWMutex
	inFallback     bool
	fallbackStart  time.Time
	recoveryChecks int
	lastRecovery   time.Time
}

// NewFallbackController creates a new fallback controller.
func NewFallbackController(
	generator *FallbackGenerator,
	errorThreshold int,
	recoveryInterval time.Duration,
	logger *slog.Logger,
) *FallbackController {
	if logger == nil {
		logger = slog.Default()
	}
	if errorThreshold < 1 {
		errorThreshold = 3
	}
	if recoveryInterval < 5*time.Second {
		recoveryInterval = 30 * time.Second
	}

	return &FallbackController{
		generator:        generator,
		errorDetector:    NewErrorPatternDetector(logger),
		logger:           logger,
		errorThreshold:   errorThreshold,
		recoveryInterval: recoveryInterval,
	}
}

// CheckError processes an error and determines if fallback should be activated.
// Returns true if fallback mode should be entered.
func (c *FallbackController) CheckError(line string) bool {
	if !c.errorDetector.CheckLine(line) {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Already in fallback
	if c.inFallback {
		return false
	}

	// Check if threshold exceeded
	if c.errorDetector.ErrorCount() >= c.errorThreshold {
		c.inFallback = true
		c.fallbackStart = time.Now()
		c.logger.Warn("entering fallback mode",
			slog.Int("error_count", c.errorDetector.ErrorCount()),
			slog.Int("threshold", c.errorThreshold),
		)
		return true
	}

	return false
}

// ShouldAttemptRecovery returns true if it's time to attempt recovery.
func (c *FallbackController) ShouldAttemptRecovery() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.inFallback {
		return false
	}

	return time.Since(c.lastRecovery) >= c.recoveryInterval
}

// StartRecoveryAttempt marks the beginning of a recovery attempt.
func (c *FallbackController) StartRecoveryAttempt() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.recoveryChecks++
	c.lastRecovery = time.Now()
	c.logger.Info("attempting upstream recovery",
		slog.Int("attempt", c.recoveryChecks),
	)
}

// RecoverySucceeded marks a successful recovery from fallback.
func (c *FallbackController) RecoverySucceeded() {
	c.mu.Lock()
	defer c.mu.Unlock()

	duration := time.Since(c.fallbackStart)
	c.logger.Info("upstream recovered, exiting fallback mode",
		slog.Duration("fallback_duration", duration),
		slog.Int("recovery_attempts", c.recoveryChecks),
	)

	c.inFallback = false
	c.fallbackStart = time.Time{}
	c.recoveryChecks = 0
	c.errorDetector.Reset()
}

// RecoveryFailed marks a failed recovery attempt.
func (c *FallbackController) RecoveryFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("recovery attempt failed, continuing fallback",
		slog.Int("attempt", c.recoveryChecks),
	)
}

// InFallback returns true if currently in fallback mode.
func (c *FallbackController) InFallback() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.inFallback
}

// Stats returns fallback controller statistics.
func (c *FallbackController) Stats() FallbackControllerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var fallbackDuration time.Duration
	if c.inFallback {
		fallbackDuration = time.Since(c.fallbackStart)
	}

	return FallbackControllerStats{
		InFallback:        c.inFallback,
		FallbackStart:     c.fallbackStart,
		FallbackDuration:  fallbackDuration,
		ErrorCount:        c.errorDetector.ErrorCount(),
		ErrorThreshold:    c.errorThreshold,
		RecoveryInterval:  c.recoveryInterval,
		RecoveryAttempts:  c.recoveryChecks,
		LastRecoveryCheck: c.lastRecovery,
	}
}

// FallbackControllerStats holds statistics for the fallback controller.
type FallbackControllerStats struct {
	InFallback        bool          `json:"in_fallback"`
	FallbackStart     time.Time     `json:"fallback_start,omitempty"`
	FallbackDuration  time.Duration `json:"fallback_duration_ns,omitempty"`
	ErrorCount        int           `json:"error_count"`
	ErrorThreshold    int           `json:"error_threshold"`
	RecoveryInterval  time.Duration `json:"recovery_interval_ns"`
	RecoveryAttempts  int           `json:"recovery_attempts"`
	LastRecoveryCheck time.Time     `json:"last_recovery_check,omitempty"`
}
