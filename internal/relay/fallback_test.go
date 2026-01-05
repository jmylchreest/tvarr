package relay

import (
	"context"
	"log/slog"
	"os/exec"
	"slices"
	"testing"
	"time"
)

func TestDefaultFallbackConfig(t *testing.T) {
	config := DefaultFallbackConfig()

	if config.Width != 1280 {
		t.Errorf("expected width 1280, got %d", config.Width)
	}
	if config.Height != 720 {
		t.Errorf("expected height 720, got %d", config.Height)
	}
	if config.SegmentDuration != 2.0 {
		t.Errorf("expected segment duration 2.0, got %f", config.SegmentDuration)
	}
	if config.Message != "Stream Unavailable" {
		t.Errorf("expected message 'Stream Unavailable', got %s", config.Message)
	}
	if config.BackgroundColor != "black" {
		t.Errorf("expected background color 'black', got %s", config.BackgroundColor)
	}
	if config.TextColor != "white" {
		t.Errorf("expected text color 'white', got %s", config.TextColor)
	}
	if config.FontSize != 48 {
		t.Errorf("expected font size 48, got %d", config.FontSize)
	}
	if config.VideoBitrate != 1000 {
		t.Errorf("expected video bitrate 1000, got %d", config.VideoBitrate)
	}
	if !config.AudioEnabled {
		t.Error("expected audio enabled to be true")
	}
	if config.FFmpegPath != "ffmpeg" {
		t.Errorf("expected ffmpeg path 'ffmpeg', got %s", config.FFmpegPath)
	}
}

func TestNewFallbackGenerator(t *testing.T) {
	config := DefaultFallbackConfig()
	logger := slog.Default()

	gen := NewFallbackGenerator(config, logger)
	if gen == nil {
		t.Fatal("NewFallbackGenerator returned nil")
	}

	if gen.IsReady() {
		t.Error("generator should not be ready before Initialize")
	}
}

func TestFallbackGenerator_NotReady(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	_, err := gen.GetSegment()
	if err != ErrFallbackNotReady {
		t.Errorf("expected ErrFallbackNotReady, got %v", err)
	}
}

func TestFallbackGenerator_Stats(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	stats := gen.Stats()

	if stats.Initialized {
		t.Error("stats should show not initialized")
	}
	if stats.SegmentSize != 0 {
		t.Errorf("expected segment size 0, got %d", stats.SegmentSize)
	}
	if stats.Width != config.Width {
		t.Errorf("expected width %d, got %d", config.Width, stats.Width)
	}
	if stats.Height != config.Height {
		t.Errorf("expected height %d, got %d", config.Height, stats.Height)
	}
	if stats.SegmentDuration != config.SegmentDuration {
		t.Errorf("expected segment duration %f, got %f", config.SegmentDuration, stats.SegmentDuration)
	}
}

// TestFallbackGenerator_Initialize tests actual TS generation.
// This test is skipped if ffmpeg is not available.
func TestFallbackGenerator_Initialize(t *testing.T) {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping integration test")
	}

	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := gen.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !gen.IsReady() {
		t.Error("generator should be ready after Initialize")
	}

	// Get the segment
	segment, err := gen.GetSegment()
	if err != nil {
		t.Fatalf("GetSegment failed: %v", err)
	}

	// Verify TS data
	if len(segment) < 188 {
		t.Errorf("segment too small to be valid TS: %d bytes", len(segment))
	}

	// Check for TS sync byte (0x47)
	if segment[0] != 0x47 {
		t.Errorf("expected TS sync byte 0x47, got 0x%02x", segment[0])
	}

	// Verify stats after initialization
	stats := gen.Stats()
	if !stats.Initialized {
		t.Error("stats should show initialized")
	}
	if stats.SegmentSize == 0 {
		t.Error("stats should show non-zero segment size")
	}
	if stats.LastGenerated.IsZero() {
		t.Error("stats should show non-zero last generated time")
	}
}

// TestFallbackGenerator_InitializeTwice verifies idempotent Initialize.
func TestFallbackGenerator_InitializeTwice(t *testing.T) {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping integration test")
	}

	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize first time
	if err := gen.Initialize(ctx); err != nil {
		t.Fatalf("first Initialize failed: %v", err)
	}

	firstStats := gen.Stats()

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Initialize second time (should be no-op)
	if err := gen.Initialize(ctx); err != nil {
		t.Fatalf("second Initialize failed: %v", err)
	}

	secondStats := gen.Stats()

	// Should have the same generation time
	if firstStats.LastGenerated != secondStats.LastGenerated {
		t.Error("second Initialize should not regenerate")
	}
}

// Test helper functions

func TestEscapeFFmpegText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "Hello World"},
		{"It's working", "It\\'s working"},
		{"Time: 12:30", "Time\\: 12\\:30"},
		{"Path\\to\\file", "Path\\\\to\\\\file"},
	}

	for _, tt := range tests {
		result := escapeFFmpegText(tt.input)
		if result != tt.expected {
			t.Errorf("escapeFFmpegText(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"Hello World", "world", true}, // case-insensitive
		{"Hello World", "WORLD", true}, // case-insensitive
		{"Hello World", "foo", false},
		{"Connection refused", "refused", true},
		{"", "test", false},
		{"test", "", true},
	}

	for _, tt := range tests {
		got := containsString(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsString(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"Hello", 10, "Hello"},
		{"Hello World", 5, "Hello..."},
		{"", 5, ""},
		{"Test", 4, "Test"},
	}

	for _, tt := range tests {
		got := truncateString(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

// ErrorPatternDetector tests

func TestDefaultErrorPatterns(t *testing.T) {
	patterns := DefaultErrorPatterns()

	if len(patterns) == 0 {
		t.Error("expected non-empty error patterns")
	}

	// Check for some known patterns
	expectedPatterns := []string{
		"Connection refused",
		"Connection timed out",
		"I/O error",
	}

	for _, expected := range expectedPatterns {
		found := slices.Contains(patterns, expected)
		if !found {
			t.Errorf("expected pattern %q not found", expected)
		}
	}
}

func TestNewErrorPatternDetector(t *testing.T) {
	detector := NewErrorPatternDetector(nil)

	if detector == nil {
		t.Fatal("NewErrorPatternDetector returned nil")
	}

	if detector.ErrorCount() != 0 {
		t.Errorf("new detector should have 0 errors, got %d", detector.ErrorCount())
	}
}

func TestErrorPatternDetector_CheckLine(t *testing.T) {
	detector := NewErrorPatternDetector(nil)

	tests := []struct {
		line     string
		detected bool
	}{
		{"Normal output line", false},
		{"Connection refused by server", true},
		{"Connection timed out", true},
		{"Error: I/O error occurred", true},
		{"SERVER RETURNED 500", true},
		{"Everything is fine", false},
	}

	for _, tt := range tests {
		got := detector.CheckLine(tt.line)
		if got != tt.detected {
			t.Errorf("CheckLine(%q) = %v, want %v", tt.line, got, tt.detected)
		}
	}

	// Should have detected errors
	if detector.ErrorCount() == 0 {
		t.Error("expected some errors to be detected")
	}
}

func TestErrorPatternDetector_Reset(t *testing.T) {
	detector := NewErrorPatternDetector(nil)

	// Generate some errors
	detector.CheckLine("Connection refused")
	detector.CheckLine("I/O error")

	if detector.ErrorCount() == 0 {
		t.Fatal("expected errors before reset")
	}

	detector.Reset()

	if detector.ErrorCount() != 0 {
		t.Errorf("expected 0 errors after reset, got %d", detector.ErrorCount())
	}
}

func TestErrorPatternDetector_SetPatterns(t *testing.T) {
	detector := NewErrorPatternDetector(nil)

	// Set custom patterns
	detector.SetPatterns([]string{"CUSTOM_ERROR"})

	// Old pattern should not match
	if detector.CheckLine("Connection refused") {
		t.Error("old pattern should not match after SetPatterns")
	}

	// New pattern should match
	if !detector.CheckLine("CUSTOM_ERROR detected") {
		t.Error("new pattern should match")
	}
}

func TestErrorPatternDetector_LastErrorTime(t *testing.T) {
	detector := NewErrorPatternDetector(nil)

	// Initially zero
	if !detector.LastErrorTime().IsZero() {
		t.Error("expected zero time initially")
	}

	// After error
	before := time.Now()
	detector.CheckLine("Connection refused")
	after := time.Now()

	lastError := detector.LastErrorTime()
	if lastError.Before(before) || lastError.After(after) {
		t.Error("last error time should be between before and after")
	}
}

// FallbackController tests

func TestNewFallbackController(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	controller := NewFallbackController(gen, 3, 30*time.Second, nil)

	if controller == nil {
		t.Fatal("NewFallbackController returned nil")
	}

	if controller.InFallback() {
		t.Error("new controller should not be in fallback")
	}
}

func TestFallbackController_DefaultValues(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	// Test with invalid threshold
	controller := NewFallbackController(gen, 0, 0, nil)

	stats := controller.Stats()
	if stats.ErrorThreshold != 3 {
		t.Errorf("expected default threshold 3, got %d", stats.ErrorThreshold)
	}
	if stats.RecoveryInterval < 5*time.Second {
		t.Errorf("expected recovery interval >= 5s, got %v", stats.RecoveryInterval)
	}
}

func TestFallbackController_CheckError(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	controller := NewFallbackController(gen, 3, 30*time.Second, nil)

	// First two errors shouldn't trigger fallback
	for i := range 2 {
		triggered := controller.CheckError("Connection refused")
		if triggered {
			t.Errorf("error %d shouldn't trigger fallback", i+1)
		}
	}

	// Third error should trigger fallback
	if !controller.CheckError("Connection refused") {
		t.Error("third error should trigger fallback")
	}

	if !controller.InFallback() {
		t.Error("controller should be in fallback after threshold")
	}

	// Additional errors shouldn't return true again (already in fallback)
	if controller.CheckError("Connection refused") {
		t.Error("already in fallback, shouldn't return true again")
	}
}

func TestFallbackController_ShouldAttemptRecovery(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	// Note: NewFallbackController has a minimum recovery interval of 5s,
	// so passing a small value gets overridden to 30s.
	// Use 100ms which will be overridden to 30s by the constructor.
	controller := NewFallbackController(gen, 1, 100*time.Millisecond, nil)

	// Not in fallback, shouldn't attempt recovery
	if controller.ShouldAttemptRecovery() {
		t.Error("shouldn't attempt recovery when not in fallback")
	}

	// Enter fallback
	controller.CheckError("Connection refused")
	controller.CheckError("Connection refused") // redundant but harmless

	// Immediately after entering fallback, should be able to attempt
	// (lastRecovery is zero, so time.Since() is large)
	if !controller.ShouldAttemptRecovery() {
		t.Error("should attempt recovery after entering fallback")
	}

	// Start a recovery attempt
	controller.StartRecoveryAttempt()

	// Immediately after, shouldn't attempt again
	// (lastRecovery is now set, and we're within the 30s interval)
	if controller.ShouldAttemptRecovery() {
		t.Error("shouldn't attempt recovery immediately after starting one")
	}

	// Verify the recovery interval constraint is respected
	// We don't wait 30+ seconds in a test, but we verify the immediate behavior
	stats := controller.Stats()
	if stats.RecoveryInterval < 5*time.Second {
		t.Errorf("expected recovery interval >= 5s (minimum), got %v", stats.RecoveryInterval)
	}
}

func TestFallbackController_RecoverySucceeded(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	controller := NewFallbackController(gen, 1, 100*time.Millisecond, nil)

	// Enter fallback
	controller.CheckError("Connection refused")
	controller.CheckError("Connection refused")

	if !controller.InFallback() {
		t.Fatal("should be in fallback")
	}

	// Recovery succeeded
	controller.RecoverySucceeded()

	if controller.InFallback() {
		t.Error("should not be in fallback after recovery succeeded")
	}

	stats := controller.Stats()
	if stats.RecoveryAttempts != 0 {
		t.Errorf("recovery attempts should be reset, got %d", stats.RecoveryAttempts)
	}
}

func TestFallbackController_RecoveryFailed(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	controller := NewFallbackController(gen, 1, 100*time.Millisecond, nil)

	// Enter fallback
	controller.CheckError("Connection refused")
	controller.CheckError("Connection refused")

	controller.StartRecoveryAttempt()
	controller.RecoveryFailed()

	// Should still be in fallback
	if !controller.InFallback() {
		t.Error("should still be in fallback after recovery failed")
	}

	stats := controller.Stats()
	if stats.RecoveryAttempts != 1 {
		t.Errorf("expected 1 recovery attempt, got %d", stats.RecoveryAttempts)
	}
}

func TestFallbackController_Stats(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	controller := NewFallbackController(gen, 3, 30*time.Second, nil)

	stats := controller.Stats()

	if stats.InFallback {
		t.Error("should not be in fallback initially")
	}
	if stats.ErrorThreshold != 3 {
		t.Errorf("expected threshold 3, got %d", stats.ErrorThreshold)
	}
	if stats.RecoveryInterval != 30*time.Second {
		t.Errorf("expected recovery interval 30s, got %v", stats.RecoveryInterval)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", stats.ErrorCount)
	}
	if stats.RecoveryAttempts != 0 {
		t.Errorf("expected 0 recovery attempts, got %d", stats.RecoveryAttempts)
	}
}

func TestFallbackController_Stats_InFallback(t *testing.T) {
	config := DefaultFallbackConfig()
	gen := NewFallbackGenerator(config, nil)

	controller := NewFallbackController(gen, 1, 30*time.Second, nil)

	// Enter fallback
	controller.CheckError("Connection refused")
	controller.CheckError("Connection refused")

	time.Sleep(10 * time.Millisecond)

	stats := controller.Stats()

	if !stats.InFallback {
		t.Error("should be in fallback")
	}
	if stats.FallbackStart.IsZero() {
		t.Error("fallback start time should be set")
	}
	if stats.FallbackDuration == 0 {
		t.Error("fallback duration should be non-zero")
	}
}
