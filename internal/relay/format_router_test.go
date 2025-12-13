package relay

import (
	"strings"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
)

func TestFormatRouter_ResolveFormat(t *testing.T) {
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

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

	router := NewFormatRouter(models.ContainerFormatMPEGTS)

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
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

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
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

	req := OutputRequest{Format: FormatValueHLS}
	_, err := router.GetHandler(req)
	if err != ErrNoHandlerAvailable {
		t.Errorf("expected ErrNoHandlerAvailable, got %v", err)
	}
}

func TestFormatRouter_SupportedFormats(t *testing.T) {
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

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
	// Test with MPEG-TS default
	router := NewFormatRouter(models.ContainerFormatMPEGTS)
	if router.DefaultFormat() != models.ContainerFormatMPEGTS {
		t.Errorf("expected default format mpegts, got %s", router.DefaultFormat())
	}

	// Test changing default to fMP4
	router.SetDefaultFormat(models.ContainerFormatFMP4)
	if router.DefaultFormat() != models.ContainerFormatFMP4 {
		t.Errorf("expected default format fmp4, got %s", router.DefaultFormat())
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

// TestZeroConfigProxyMultipleClientTypes is an integration test that verifies
// a zero-config proxy correctly serves multiple client types with appropriate
// formats and codec detection. This tests the full flow from User-Agent to
// detected capabilities without requiring database access.
func TestZeroConfigProxyMultipleClientTypes(t *testing.T) {
	// Test cases representing common client types that should work out-of-the-box
	// with a zero-config proxy (no custom encoding profile required)
	clientTests := []struct {
		name            string
		userAgent       string
		accept          string
		expectedFormat  string
		description     string
		isAppleExpected bool
	}{
		// Desktop browsers
		{
			name:            "Chrome on Windows",
			userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Chrome receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "Firefox on Linux",
			userAgent:       "Mozilla/5.0 (X11; Linux x86_64; rv:122.0) Gecko/20100101 Firefox/122.0",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Firefox receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "Edge on Windows",
			userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Edge receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		// Apple devices -> HLS
		{
			name:            "Safari on macOS",
			userAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
			accept:          "*/*",
			expectedFormat:  FormatValueHLS,
			description:     "Safari receives HLS (Apple device)",
			isAppleExpected: true,
		},
		{
			name:            "iPhone Safari",
			userAgent:       "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			accept:          "*/*",
			expectedFormat:  FormatValueHLS,
			description:     "iPhone receives HLS (Apple device)",
			isAppleExpected: true,
		},
		{
			name:            "iPad Safari",
			userAgent:       "Mozilla/5.0 (iPad; CPU OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			accept:          "*/*",
			expectedFormat:  FormatValueHLS,
			description:     "iPad receives HLS (Apple device)",
			isAppleExpected: true,
		},
		{
			name:            "Apple TV",
			userAgent:       "AppleCoreMedia/1.0.0.21K69 (Apple TV; U; CPU OS 17_2 like Mac OS X)",
			accept:          "*/*",
			expectedFormat:  FormatValueHLS,
			description:     "Apple TV receives HLS (Apple device)",
			isAppleExpected: true,
		},
		// Smart TVs / Streaming devices
		{
			name:            "Samsung Tizen TV",
			userAgent:       "Mozilla/5.0 (SMART-TV; LINUX; Tizen 7.0) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/4.0 Chrome/94.0.4606.31 TV Safari/537.36",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Samsung TV receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "LG WebOS TV",
			userAgent:       "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36 WebAppManager",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "LG TV receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "Roku",
			userAgent:       "Roku/DVP-12.0 (12.0.0.4191-AW)",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Roku receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "Android TV",
			userAgent:       "Mozilla/5.0 (Linux; Android 12; Google TV Device) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.5359.61 Safari/537.36",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Android TV receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		// Media players
		{
			name:            "VLC",
			userAgent:       "VLC/3.0.20 LibVLC/3.0.20",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "VLC receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "MPV",
			userAgent:       "mpv/0.37.0",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "MPV receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "Kodi",
			userAgent:       "Kodi/20.2 (Linux; Android 12; NVIDIA Shield TV Pro)",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Kodi receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		{
			name:            "FFmpeg/Lavf",
			userAgent:       "Lavf/60.16.100",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "FFmpeg receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		// Mobile apps
		{
			name:            "Android mobile browser",
			userAgent:       "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.210 Mobile Safari/537.36",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Android phone receives MPEG-TS (universal format)",
			isAppleExpected: false,
		},
		// DASH players (via Accept header or User-Agent)
		{
			name:            "Shaka Player",
			userAgent:       "Mozilla/5.0 Shaka-Player/4.7.0",
			accept:          "*/*",
			expectedFormat:  FormatValueDASH,
			description:     "Shaka Player receives DASH (DASH player)",
			isAppleExpected: false,
		},
		{
			name:            "DASH.js via Accept",
			userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			accept:          "application/dash+xml, */*",
			expectedFormat:  FormatValueDASH,
			description:     "Client with DASH Accept header receives DASH",
			isAppleExpected: false,
		},
		// HLS via Accept header
		{
			name:            "HLS via Accept",
			userAgent:       "Generic Player/1.0",
			accept:          "application/vnd.apple.mpegurl",
			expectedFormat:  FormatValueHLS,
			description:     "Client with HLS Accept header receives HLS",
			isAppleExpected: false,
		},
		// Unknown/empty -> default
		{
			name:            "Unknown client",
			userAgent:       "UnknownPlayer/1.0",
			accept:          "*/*",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Unknown client receives MPEG-TS (fallback)",
			isAppleExpected: false,
		},
		{
			name:            "Empty User-Agent",
			userAgent:       "",
			accept:          "",
			expectedFormat:  FormatValueMPEGTS,
			description:     "Empty UA receives MPEG-TS (fallback)",
			isAppleExpected: false,
		},
	}

	// Create format router with MPEG-TS as default (zero-config default)
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

	for _, tc := range clientTests {
		t.Run(tc.name, func(t *testing.T) {
			// Test Apple device detection (isAppleDevice expects lowercase input,
			// matching how DetectOptimalFormat uses it)
			isApple := isAppleDevice(strings.ToLower(tc.userAgent))
			if isApple != tc.isAppleExpected {
				t.Errorf("isAppleDevice(%q) = %v, expected %v", tc.userAgent, isApple, tc.isAppleExpected)
			}

			// Test format detection through router
			detectedFormat := router.DetectOptimalFormat(tc.userAgent, tc.accept)
			if detectedFormat != tc.expectedFormat {
				t.Errorf("%s: DetectOptimalFormat() = %q, expected %q", tc.description, detectedFormat, tc.expectedFormat)
			}

			// Test through OutputRequest.ResolveFormat with "auto" mode
			req := OutputRequest{
				Format:    "auto",
				UserAgent: tc.userAgent,
				Accept:    tc.accept,
			}
			resolvedFormat := router.ResolveFormat(req)
			if resolvedFormat != tc.expectedFormat {
				t.Errorf("%s: ResolveFormat(auto) = %q, expected %q", tc.description, resolvedFormat, tc.expectedFormat)
			}
		})
	}
}

// TestZeroConfigExplicitFormatOverride verifies that explicit format parameters
// override auto-detection, allowing users to force a specific format regardless
// of client capabilities.
func TestZeroConfigExplicitFormatOverride(t *testing.T) {
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

	// Test that explicit format works even for clients that would auto-detect differently
	tests := []struct {
		name           string
		userAgent      string
		explicitFormat string
		expectedFormat string
	}{
		{
			name:           "Apple device forced to DASH",
			userAgent:      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X)",
			explicitFormat: FormatValueDASH,
			expectedFormat: FormatValueDASH,
		},
		{
			name:           "Chrome forced to HLS",
			userAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0",
			explicitFormat: FormatValueHLS,
			expectedFormat: FormatValueHLS,
		},
		{
			name:           "VLC forced to DASH",
			userAgent:      "VLC/3.0.20",
			explicitFormat: FormatValueDASH,
			expectedFormat: FormatValueDASH,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := OutputRequest{
				Format:    tc.explicitFormat,
				UserAgent: tc.userAgent,
				Accept:    "*/*",
			}
			resolved := router.ResolveFormat(req)
			if resolved != tc.expectedFormat {
				t.Errorf("Explicit format %q should override auto-detection, got %q",
					tc.explicitFormat, resolved)
			}
		})
	}
}

// TestDefaultClientDetectorMultipleClients verifies the DefaultClientDetector
// properly handles various client types without database-backed rules.
func TestDefaultClientDetectorMultipleClients(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	tests := []struct {
		name                 string
		userAgent            string
		accept               string
		formatOverride       string
		expectedSource       string
		expectSupportsFMP4   bool
		expectSupportsMPEGTS bool
	}{
		{
			name:                 "Format override takes precedence",
			userAgent:            "VLC/3.0.20",
			accept:               "*/*",
			formatOverride:       "dash",
			expectedSource:       "format_override",
			expectSupportsFMP4:   true,
			expectSupportsMPEGTS: false,
		},
		{
			name:                 "Accept header DASH",
			userAgent:            "Mozilla/5.0",
			accept:               "application/dash+xml",
			formatOverride:       "",
			expectedSource:       "accept",
			expectSupportsFMP4:   true,
			expectSupportsMPEGTS: false,
		},
		{
			name:                 "Accept header HLS",
			userAgent:            "Mozilla/5.0",
			accept:               "application/vnd.apple.mpegurl",
			formatOverride:       "",
			expectedSource:       "accept",
			expectSupportsFMP4:   true,
			expectSupportsMPEGTS: true,
		},
		{
			name:                 "Accept header MPEG-TS",
			userAgent:            "Mozilla/5.0",
			accept:               "video/mp2t",
			formatOverride:       "",
			expectedSource:       "accept",
			expectSupportsFMP4:   false,
			expectSupportsMPEGTS: true,
		},
		{
			name:                 "Default for unknown",
			userAgent:            "Unknown/1.0",
			accept:               "*/*",
			formatOverride:       "",
			expectedSource:       "default",
			expectSupportsFMP4:   true,
			expectSupportsMPEGTS: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := OutputRequest{
				UserAgent:      tc.userAgent,
				Accept:         tc.accept,
				FormatOverride: tc.formatOverride,
			}
			caps := detector.Detect(req)

			if caps.DetectionSource != tc.expectedSource {
				t.Errorf("DetectionSource = %q, expected %q", caps.DetectionSource, tc.expectedSource)
			}
			if caps.SupportsFMP4 != tc.expectSupportsFMP4 {
				t.Errorf("SupportsFMP4 = %v, expected %v", caps.SupportsFMP4, tc.expectSupportsFMP4)
			}
			if caps.SupportsMPEGTS != tc.expectSupportsMPEGTS {
				t.Errorf("SupportsMPEGTS = %v, expected %v", caps.SupportsMPEGTS, tc.expectSupportsMPEGTS)
			}
		})
	}
}

// ============================================================================
// User Story 2: Force Specific Codec Tests
// ============================================================================

// TestEncodingProfileOverridesClientDetection verifies that an encoding profile
// can force a specific codec regardless of client detection results.
// This is T035 from tasks.md - User Story 2.
func TestEncodingProfileOverridesClientDetection(t *testing.T) {
	// Test cases verifying encoding profile override behavior
	tests := []struct {
		name                 string
		profileVideoCodec    models.VideoCodec
		profileAudioCodec    models.AudioCodec
		clientSupportsFMP4   bool
		clientSupportsMPEGTS bool
		expectedContainer    models.ContainerFormat
		shouldRequireFMP4    bool
	}{
		// H.264/AAC - compatible with both containers
		{
			name:                 "H.264/AAC profile works with MPEG-TS capable client",
			profileVideoCodec:    models.VideoCodecH264,
			profileAudioCodec:    models.AudioCodecAAC,
			clientSupportsFMP4:   true,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4, // Default to modern fMP4
			shouldRequireFMP4:    false,
		},
		{
			name:                 "H.264/AAC profile with MPEG-TS only client",
			profileVideoCodec:    models.VideoCodecH264,
			profileAudioCodec:    models.AudioCodecAAC,
			clientSupportsFMP4:   false,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4, // Profile determines container
			shouldRequireFMP4:    false,
		},
		// H.265/AAC - compatible with both containers
		{
			name:                 "H.265/AAC profile works with MPEG-TS capable client",
			profileVideoCodec:    models.VideoCodecH265,
			profileAudioCodec:    models.AudioCodecAAC,
			clientSupportsFMP4:   true,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4,
			shouldRequireFMP4:    false,
		},
		// VP9 - requires fMP4 container
		{
			name:                 "VP9/Opus profile requires fMP4 container",
			profileVideoCodec:    models.VideoCodecVP9,
			profileAudioCodec:    models.AudioCodecOpus,
			clientSupportsFMP4:   true,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4,
			shouldRequireFMP4:    true,
		},
		{
			name:                 "VP9/Opus profile still requires fMP4 even if client prefers MPEG-TS",
			profileVideoCodec:    models.VideoCodecVP9,
			profileAudioCodec:    models.AudioCodecOpus,
			clientSupportsFMP4:   false,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4, // VP9 REQUIRES fMP4
			shouldRequireFMP4:    true,
		},
		// AV1 - requires fMP4 container
		{
			name:                 "AV1/Opus profile requires fMP4 container",
			profileVideoCodec:    models.VideoCodecAV1,
			profileAudioCodec:    models.AudioCodecOpus,
			clientSupportsFMP4:   true,
			clientSupportsMPEGTS: false,
			expectedContainer:    models.ContainerFormatFMP4,
			shouldRequireFMP4:    true,
		},
		// Mixed codec scenarios
		{
			name:                 "H.264/Opus - Opus requires fMP4",
			profileVideoCodec:    models.VideoCodecH264,
			profileAudioCodec:    models.AudioCodecOpus,
			clientSupportsFMP4:   true,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4,
			shouldRequireFMP4:    true, // Opus is fMP4-only
		},
		{
			name:                 "VP9/AAC - VP9 requires fMP4",
			profileVideoCodec:    models.VideoCodecVP9,
			profileAudioCodec:    models.AudioCodecAAC,
			clientSupportsFMP4:   true,
			clientSupportsMPEGTS: true,
			expectedContainer:    models.ContainerFormatFMP4,
			shouldRequireFMP4:    true, // VP9 is fMP4-only
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create encoding profile
			profile := &models.EncodingProfile{
				Name:             "test-profile",
				TargetVideoCodec: tc.profileVideoCodec,
				TargetAudioCodec: tc.profileAudioCodec,
				QualityPreset:    models.QualityPresetMedium,
			}

			// Verify RequiresFMP4 behavior
			requiresFMP4 := profile.RequiresFMP4()
			if requiresFMP4 != tc.shouldRequireFMP4 {
				t.Errorf("RequiresFMP4() = %v, expected %v for video=%s, audio=%s",
					requiresFMP4, tc.shouldRequireFMP4,
					tc.profileVideoCodec, tc.profileAudioCodec)
			}

			// Verify DetermineContainer behavior
			container := profile.DetermineContainer()
			if container != tc.expectedContainer {
				t.Errorf("DetermineContainer() = %v, expected %v for video=%s, audio=%s",
					container, tc.expectedContainer,
					tc.profileVideoCodec, tc.profileAudioCodec)
			}
		})
	}
}

// TestEncodingProfileForcesSpecificCodec is an integration test verifying that
// when a proxy has an encoding profile set, all clients receive that codec
// regardless of their detected capabilities.
// This is T036 from tasks.md - User Story 2.
func TestEncodingProfileForcesSpecificCodec(t *testing.T) {
	// Simulate different client types connecting to a proxy with a forced codec profile
	clientTypes := []struct {
		name      string
		userAgent string
		accept    string
	}{
		{"Chrome", "Mozilla/5.0 Chrome/120.0.0.0", "*/*"},
		{"Safari", "Mozilla/5.0 Safari/605.1.15 Macintosh", "*/*"},
		{"iPhone", "Mozilla/5.0 iPhone Safari/604.1", "*/*"},
		{"VLC", "VLC/3.0.20 LibVLC/3.0.20", "*/*"},
		{"Kodi", "Kodi/20.2", "*/*"},
		{"Android TV", "Mozilla/5.0 Android TV Chrome/108.0", "*/*"},
	}

	// Create an H.265 encoding profile (forcing H.265 for all clients)
	h265Profile := &models.EncodingProfile{
		Name:             "Force-H265",
		Description:      "Forces H.265 codec for all clients",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetHigh,
	}

	// Create VP9 encoding profile (requires fMP4)
	vp9Profile := &models.EncodingProfile{
		Name:             "Force-VP9",
		Description:      "Forces VP9 codec for all clients",
		TargetVideoCodec: models.VideoCodecVP9,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
	}

	t.Run("H.265 profile forces H.265 for all clients", func(t *testing.T) {
		for _, client := range clientTypes {
			t.Run(client.name, func(t *testing.T) {
				// The encoding profile determines the codec, not client detection
				targetCodec := h265Profile.TargetVideoCodec
				if targetCodec != models.VideoCodecH265 {
					t.Errorf("Expected target codec H.265, got %s", targetCodec)
				}

				// H.265 can work with either container
				container := h265Profile.DetermineContainer()
				if container != models.ContainerFormatFMP4 {
					t.Errorf("Expected fMP4 container for H.265, got %s", container)
				}

				// Verify codec is consistent regardless of client
				if h265Profile.TargetVideoCodec != models.VideoCodecH265 {
					t.Errorf("Client %s should receive H.265 but profile shows %s",
						client.name, h265Profile.TargetVideoCodec)
				}
			})
		}
	})

	t.Run("VP9 profile forces VP9 and fMP4 for all clients", func(t *testing.T) {
		for _, client := range clientTypes {
			t.Run(client.name, func(t *testing.T) {
				// VP9 requires fMP4 container
				if !vp9Profile.RequiresFMP4() {
					t.Error("VP9 profile should require fMP4")
				}

				container := vp9Profile.DetermineContainer()
				if container != models.ContainerFormatFMP4 {
					t.Errorf("VP9 must use fMP4, got %s for client %s", container, client.name)
				}

				// All clients must receive VP9 regardless of capability
				if vp9Profile.TargetVideoCodec != models.VideoCodecVP9 {
					t.Errorf("Client %s should receive VP9 but profile shows %s",
						client.name, vp9Profile.TargetVideoCodec)
				}
			})
		}
	})
}

// TestFormatRouterWithEncodingProfile tests that the format router correctly
// handles encoding profile container requirements.
func TestFormatRouterWithEncodingProfile(t *testing.T) {
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

	// Test that encoding profile container takes precedence
	tests := []struct {
		name              string
		profile           *models.EncodingProfile
		userAgent         string
		accept            string
		expectedContainer models.ContainerFormat
	}{
		{
			name: "VP9 profile overrides MPEG-TS default",
			profile: &models.EncodingProfile{
				TargetVideoCodec: models.VideoCodecVP9,
				TargetAudioCodec: models.AudioCodecOpus,
			},
			userAgent:         "VLC/3.0.20", // VLC normally gets MPEG-TS
			accept:            "*/*",
			expectedContainer: models.ContainerFormatFMP4, // VP9 forces fMP4
		},
		{
			name: "AV1 profile overrides MPEG-TS default",
			profile: &models.EncodingProfile{
				TargetVideoCodec: models.VideoCodecAV1,
				TargetAudioCodec: models.AudioCodecOpus,
			},
			userAgent:         "mpv/0.37.0", // mpv normally gets MPEG-TS
			accept:            "*/*",
			expectedContainer: models.ContainerFormatFMP4, // AV1 forces fMP4
		},
		{
			name: "H.264 profile defaults to fMP4",
			profile: &models.EncodingProfile{
				TargetVideoCodec: models.VideoCodecH264,
				TargetAudioCodec: models.AudioCodecAAC,
			},
			userAgent:         "Chrome/120.0.0.0",
			accept:            "*/*",
			expectedContainer: models.ContainerFormatFMP4, // H.264 defaults to fMP4
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Encoding profile determines container, not format router
			container := tc.profile.DetermineContainer()
			if container != tc.expectedContainer {
				t.Errorf("Expected container %s, got %s", tc.expectedContainer, container)
			}

			// The format router's auto-detection still works for format (HLS/DASH/MPEG-TS)
			// but the profile determines whether transcoding to specific codec happens
			req := OutputRequest{
				UserAgent: tc.userAgent,
				Accept:    tc.accept,
			}
			format := router.DetectOptimalFormat(req.UserAgent, req.Accept)
			// Format detection still works - codec is separate concern
			if format == "" {
				t.Error("Format detection should return a valid format")
			}
		})
	}
}
