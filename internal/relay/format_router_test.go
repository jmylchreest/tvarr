package relay

import (
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
)

func TestFormatRouter_ResolveFormat(t *testing.T) {
	router := NewFormatRouter(models.OutputFormatMPEGTS)

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "explicit HLS",
			format:   "hls",
			expected: FormatValueHLS,
		},
		{
			name:     "explicit DASH",
			format:   "dash",
			expected: FormatValueDASH,
		},
		{
			name:     "explicit MPEG-TS",
			format:   "mpegts",
			expected: FormatValueMPEGTS,
		},
		{
			name:     "uppercase HLS",
			format:   "HLS",
			expected: FormatValueHLS,
		},
		{
			name:     "mixed case DASH",
			format:   "DaSh",
			expected: FormatValueDASH,
		},
		{
			name:     "empty format uses default",
			format:   "",
			expected: FormatValueMPEGTS, // Default format
		},
		{
			name:     "auto uses detection",
			format:   "auto",
			expected: FormatValueMPEGTS, // Falls back to default without UA/Accept
		},
		{
			name:     "unknown format uses default",
			format:   "unknown",
			expected: FormatValueMPEGTS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := OutputRequest{Format: tt.format}
			result := router.ResolveFormat(req)
			if result != tt.expected {
				t.Errorf("ResolveFormat(%q) = %q, want %q", tt.format, result, tt.expected)
			}
		})
	}
}

func TestFormatRouter_DetectOptimalFormat(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		accept    string
		expected  string
	}{
		// Apple devices -> HLS
		{
			name:      "Safari on macOS",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Safari/605.1.15",
			accept:    "*/*",
			expected:  FormatValueHLS,
		},
		{
			name:      "iPhone Safari",
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
			accept:    "*/*",
			expected:  FormatValueHLS,
		},
		{
			name:      "iPad Safari",
			userAgent: "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
			accept:    "*/*",
			expected:  FormatValueHLS,
		},
		{
			name:      "Apple TV",
			userAgent: "AppleCoreMedia/1.0.0.19A346 (Apple TV; U; CPU OS 15_0 like Mac OS X; en_us)",
			accept:    "*/*",
			expected:  FormatValueHLS,
		},
		{
			name:      "AppleCoreMedia",
			userAgent: "AppleCoreMedia/1.0.0.19A346 (iPhone; U; CPU OS 15_0 like Mac OS X)",
			accept:    "*/*",
			expected:  FormatValueHLS,
		},
		// Chrome on Mac should NOT be forced to HLS
		{
			name:      "Chrome on macOS",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
			accept:    "*/*",
			expected:  FormatValueMPEGTS, // Default, not forced HLS
		},
		{
			name:      "Firefox on macOS",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:89.0) Gecko/20100101 Firefox/89.0",
			accept:    "*/*",
			expected:  FormatValueMPEGTS, // Default, not forced HLS
		},
		// Accept header preferences
		{
			name:      "Accept DASH",
			userAgent: "Mozilla/5.0",
			accept:    "application/dash+xml",
			expected:  FormatValueDASH,
		},
		{
			name:      "Accept HLS",
			userAgent: "Mozilla/5.0",
			accept:    "application/vnd.apple.mpegurl",
			expected:  FormatValueHLS,
		},
		{
			name:      "Accept x-mpegurl",
			userAgent: "Mozilla/5.0",
			accept:    "application/x-mpegurl",
			expected:  FormatValueHLS,
		},
		// DASH player identifiers
		{
			name:      "Shaka Player",
			userAgent: "Mozilla/5.0 Shaka-Player/3.0",
			accept:    "*/*",
			expected:  FormatValueDASH,
		},
		{
			name:      "DASH.js",
			userAgent: "dash.js/4.0",
			accept:    "*/*",
			expected:  FormatValueDASH,
		},
		// Generic/unknown -> default
		{
			name:      "Generic browser",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124",
			accept:    "*/*",
			expected:  FormatValueMPEGTS,
		},
		{
			name:      "VLC",
			userAgent: "VLC/3.0.16 LibVLC/3.0.16",
			accept:    "*/*",
			expected:  FormatValueMPEGTS,
		},
		{
			name:      "FFmpeg",
			userAgent: "Lavf/58.76.100",
			accept:    "*/*",
			expected:  FormatValueMPEGTS,
		},
		{
			name:      "Empty User-Agent",
			userAgent: "",
			accept:    "",
			expected:  FormatValueMPEGTS,
		},
	}

	router := NewFormatRouter(models.OutputFormatMPEGTS)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.DetectOptimalFormat(tt.userAgent, tt.accept)
			if result != tt.expected {
				t.Errorf("DetectOptimalFormat(ua=%q, accept=%q) = %q, want %q",
					tt.userAgent, tt.accept, result, tt.expected)
			}
		})
	}
}

func TestFormatRouter_RegisterHandler(t *testing.T) {
	router := NewFormatRouter(models.OutputFormatMPEGTS)

	// Create a mock handler using an anonymous struct
	mockProvider := &mockSegmentProvider{}
	handler := NewHLSHandler(mockProvider)

	// Register handler
	router.RegisterHandler(FormatValueHLS, handler)

	// Verify handler is registered
	if !router.HasHandler(FormatValueHLS) {
		t.Error("expected HLS handler to be registered")
	}

	// Verify other formats are not registered
	if router.HasHandler(FormatValueDASH) {
		t.Error("DASH handler should not be registered")
	}

	// Get handler
	req := OutputRequest{Format: FormatValueHLS}
	got, err := router.GetHandler(req)
	if err != nil {
		t.Errorf("GetHandler failed: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil handler")
	}
}

func TestFormatRouter_GetHandler_NotFound(t *testing.T) {
	router := NewFormatRouter(models.OutputFormatMPEGTS)

	req := OutputRequest{Format: FormatValueHLS}
	_, err := router.GetHandler(req)
	if err != ErrNoHandlerAvailable {
		t.Errorf("expected ErrNoHandlerAvailable, got %v", err)
	}
}

func TestFormatRouter_SupportedFormats(t *testing.T) {
	router := NewFormatRouter(models.OutputFormatMPEGTS)

	// Initially empty
	formats := router.SupportedFormats()
	if len(formats) != 0 {
		t.Errorf("expected empty formats, got %v", formats)
	}

	// Register handlers
	mockProvider := &mockSegmentProvider{}
	router.RegisterHandler(FormatValueHLS, NewHLSHandler(mockProvider))
	router.RegisterHandler(FormatValueDASH, NewDASHHandler(mockProvider))

	// Should have both formats
	formats = router.SupportedFormats()
	if len(formats) != 2 {
		t.Errorf("expected 2 formats, got %d", len(formats))
	}

	// Check formats are present (order may vary)
	hasHLS := false
	hasDASH := false
	for _, f := range formats {
		if f == FormatValueHLS {
			hasHLS = true
		}
		if f == FormatValueDASH {
			hasDASH = true
		}
	}
	if !hasHLS {
		t.Error("expected HLS in supported formats")
	}
	if !hasDASH {
		t.Error("expected DASH in supported formats")
	}
}

func TestFormatRouter_DefaultFormat(t *testing.T) {
	// Test with HLS default
	router := NewFormatRouter(models.OutputFormatHLS)
	if router.DefaultFormat() != models.OutputFormatHLS {
		t.Errorf("expected default format HLS, got %s", router.DefaultFormat())
	}

	// Test changing default
	router.SetDefaultFormat(models.OutputFormatDASH)
	if router.DefaultFormat() != models.OutputFormatDASH {
		t.Errorf("expected default format DASH, got %s", router.DefaultFormat())
	}
}

func TestOutputRequest_IsPlaylistRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      OutputRequest
		expected bool
	}{
		{
			name:     "no segment or init",
			req:      OutputRequest{Format: FormatValueHLS},
			expected: true,
		},
		{
			name: "has segment",
			req: OutputRequest{
				Format:  FormatValueHLS,
				Segment: ptrUint64(1),
			},
			expected: false,
		},
		{
			name: "has init type",
			req: OutputRequest{
				Format:   FormatValueDASH,
				InitType: "v",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.IsPlaylistRequest(); got != tt.expected {
				t.Errorf("IsPlaylistRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestOutputRequest_IsSegmentRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      OutputRequest
		expected bool
	}{
		{
			name:     "no segment",
			req:      OutputRequest{Format: FormatValueHLS},
			expected: false,
		},
		{
			name: "has segment",
			req: OutputRequest{
				Format:  FormatValueHLS,
				Segment: ptrUint64(1),
			},
			expected: true,
		},
		{
			name: "has segment 0",
			req: OutputRequest{
				Format:  FormatValueHLS,
				Segment: ptrUint64(0),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.IsSegmentRequest(); got != tt.expected {
				t.Errorf("IsSegmentRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestOutputRequest_IsInitRequest(t *testing.T) {
	tests := []struct {
		name     string
		req      OutputRequest
		expected bool
	}{
		{
			name:     "no init type",
			req:      OutputRequest{Format: FormatValueDASH},
			expected: false,
		},
		{
			name: "has init type v",
			req: OutputRequest{
				Format:   FormatValueDASH,
				InitType: "v",
			},
			expected: true,
		},
		{
			name: "has init type a",
			req: OutputRequest{
				Format:   FormatValueDASH,
				InitType: "a",
			},
			expected: true,
		},
		{
			name: "empty init type",
			req: OutputRequest{
				Format:   FormatValueDASH,
				InitType: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.IsInitRequest(); got != tt.expected {
				t.Errorf("IsInitRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsAppleDevice(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  bool
	}{
		{"iPhone", "mozilla/5.0 (iphone; cpu iphone os 15_0 like mac os x)", true},
		{"iPad", "mozilla/5.0 (ipad; cpu os 15_0 like mac os x)", true},
		{"iPod", "mozilla/5.0 (ipod touch; cpu iphone os 14_0 like mac os x)", true},
		{"Safari macOS", "mozilla/5.0 (macintosh; intel mac os x 10_15_7) applewebkit/605.1.15 (khtml, like gecko) version/16.0 safari/605.1.15", true},
		{"AppleCoreMedia", "applecoremedia/1.0.0.19a346", true},
		{"Apple TV", "apple tv/15.0", true},
		{"tvOS", "mozilla/5.0 (tvos)", true},
		{"Chrome on Mac", "mozilla/5.0 (macintosh; intel mac os x 10_15_7) applewebkit/537.36 (khtml, like gecko) chrome/91.0.4472.124 safari/537.36", false},
		{"Firefox on Mac", "mozilla/5.0 (macintosh; intel mac os x 10.15; rv:89.0) gecko/20100101 firefox/89.0", false},
		{"Windows", "mozilla/5.0 (windows nt 10.0; win64; x64)", false},
		{"Android", "mozilla/5.0 (linux; android 11)", false},
		{"VLC", "vlc/3.0.16", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAppleDevice(tt.userAgent); got != tt.expected {
				t.Errorf("isAppleDevice(%q) = %v, want %v", tt.userAgent, got, tt.expected)
			}
		})
	}
}

// Helper to create uint64 pointer
func ptrUint64(v uint64) *uint64 {
	return &v
}

// Mock segment provider for testing
type mockSegmentProvider struct{}

func (m *mockSegmentProvider) GetSegmentInfos() []SegmentInfo {
	return nil
}

func (m *mockSegmentProvider) GetSegment(sequence uint64) (*Segment, error) {
	return nil, ErrSegmentNotFound
}

func (m *mockSegmentProvider) TargetDuration() int {
	return 6
}
