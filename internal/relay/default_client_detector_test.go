package relay

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockOutputRequest creates an OutputRequest for testing
func mockOutputRequest(userAgent, accept, xTvarrPlayer, formatOverride string) OutputRequest {
	headers := http.Header{}
	if xTvarrPlayer != "" {
		headers.Set(XTvarrPlayerHeader, xTvarrPlayer)
	}
	return OutputRequest{
		UserAgent:      userAgent,
		Accept:         accept,
		FormatOverride: formatOverride,
		Headers:        headers,
	}
}

func TestDefaultClientDetector_DetectFormatOverride(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	tests := []struct {
		name            string
		formatOverride  string
		expectedFormat  string
		expectedSource  string
		expectedFMP4    bool
		expectedMPEGTS  bool
	}{
		{
			name:           "fmp4 override",
			formatOverride: "fmp4",
			expectedFormat: FormatValueHLSFMP4,
			expectedSource: "format_override",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "hls-fmp4 override",
			formatOverride: "hls-fmp4",
			expectedFormat: FormatValueHLSFMP4,
			expectedSource: "format_override",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "mpegts override",
			formatOverride: "mpegts",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "format_override",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
		{
			name:           "ts override",
			formatOverride: "ts",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "format_override",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
		{
			name:           "hls override",
			formatOverride: "hls",
			expectedFormat: FormatValueHLS,
			expectedSource: "format_override",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "dash override",
			formatOverride: "dash",
			expectedFormat: FormatValueDASH,
			expectedSource: "format_override",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "hls-ts override",
			formatOverride: "hls-ts",
			expectedFormat: FormatValueHLSTS,
			expectedSource: "format_override",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
		{
			name:           "unknown format defaults to hls",
			formatOverride: "unknown",
			expectedFormat: FormatValueHLS,
			expectedSource: "format_override",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockOutputRequest("", "", "", tt.formatOverride)
			caps := detector.Detect(req)

			assert.Equal(t, tt.expectedFormat, caps.PreferredFormat)
			assert.Equal(t, tt.expectedSource, caps.DetectionSource)
			assert.Equal(t, tt.expectedFMP4, caps.SupportsFMP4)
			assert.Equal(t, tt.expectedMPEGTS, caps.SupportsMPEGTS)
		})
	}
}

func TestDefaultClientDetector_DetectXTvarrPlayer(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	tests := []struct {
		name            string
		xTvarrPlayer    string
		expectedPlayer  string
		expectedFormat  string
		expectedSource  string
		expectedFMP4    bool
		expectedMPEGTS  bool
	}{
		{
			name:           "hls.js player",
			xTvarrPlayer:   "hls.js",
			expectedPlayer: "hls.js",
			expectedFormat: FormatValueHLSFMP4,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "mpegts.js player",
			xTvarrPlayer:   "mpegts.js",
			expectedPlayer: "mpegts.js",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
		{
			name:           "exoplayer",
			xTvarrPlayer:   "exoplayer",
			expectedPlayer: "exoplayer",
			expectedFormat: FormatValueHLSFMP4,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "shaka player prefers DASH",
			xTvarrPlayer:   "shaka",
			expectedPlayer: "shaka",
			expectedFormat: FormatValueDASH,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "dash.js player prefers DASH",
			xTvarrPlayer:   "dash.js",
			expectedPlayer: "dash.js",
			expectedFormat: FormatValueDASH,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "vlc player no preference",
			xTvarrPlayer:   "vlc",
			expectedPlayer: "vlc",
			expectedFormat: "",
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "player with format override",
			xTvarrPlayer:   "hls.js:mpegts",
			expectedPlayer: "hls.js",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "unknown player",
			xTvarrPlayer:   "custom-player",
			expectedPlayer: "custom-player",
			expectedFormat: "",
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "unknown player with format",
			xTvarrPlayer:   "custom-player:dash",
			expectedPlayer: "custom-player",
			expectedFormat: FormatValueDASH,
			expectedSource: "x-tvarr-player",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockOutputRequest("", "", tt.xTvarrPlayer, "")
			caps := detector.Detect(req)

			assert.Equal(t, tt.expectedPlayer, caps.PlayerName)
			assert.Equal(t, tt.expectedFormat, caps.PreferredFormat)
			assert.Equal(t, tt.expectedSource, caps.DetectionSource)
			assert.Equal(t, tt.expectedFMP4, caps.SupportsFMP4)
			assert.Equal(t, tt.expectedMPEGTS, caps.SupportsMPEGTS)
		})
	}
}

func TestDefaultClientDetector_DetectAcceptHeader(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	tests := []struct {
		name           string
		accept         string
		expectedFormat string
		expectedSource string
		expectedFMP4   bool
		expectedMPEGTS bool
	}{
		{
			name:           "DASH accept",
			accept:         "application/dash+xml",
			expectedFormat: FormatValueDASH,
			expectedSource: "accept",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "Apple HLS accept",
			accept:         "application/vnd.apple.mpegurl",
			expectedFormat: FormatValueHLS,
			expectedSource: "accept",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "x-mpegurl accept",
			accept:         "application/x-mpegurl",
			expectedFormat: FormatValueHLS,
			expectedSource: "accept",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "MPEG-TS accept",
			accept:         "video/mp2t",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "accept",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockOutputRequest("", tt.accept, "", "")
			caps := detector.Detect(req)

			assert.Equal(t, tt.expectedFormat, caps.PreferredFormat)
			assert.Equal(t, tt.expectedSource, caps.DetectionSource)
			assert.Equal(t, tt.expectedFMP4, caps.SupportsFMP4)
			assert.Equal(t, tt.expectedMPEGTS, caps.SupportsMPEGTS)
		})
	}
}

func TestDefaultClientDetector_DetectUserAgent(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	tests := []struct {
		name           string
		userAgent      string
		expectedPlayer string
		expectedFormat string
		expectedSource string
		expectedFMP4   bool
		expectedMPEGTS bool
	}{
		{
			name:           "hls.js user agent",
			userAgent:      "Mozilla/5.0 hls.js/1.5.0",
			expectedPlayer: "hls.js",
			expectedFormat: FormatValueHLSFMP4,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "mpegts.js user agent",
			userAgent:      "Mozilla/5.0 mpegts.js",
			expectedPlayer: "mpegts.js",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "user-agent",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
		{
			name:           "ExoPlayer user agent",
			userAgent:      "ExoPlayer/2.18.0",
			expectedPlayer: "exoplayer",
			expectedFormat: FormatValueHLSFMP4,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "AVPlayer user agent",
			userAgent:      "AppleCoreMedia/1.0.0",
			expectedPlayer: "avplayer",
			expectedFormat: FormatValueHLS,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "VLC user agent",
			userAgent:      "VLC/3.0.18",
			expectedPlayer: "vlc",
			expectedFormat: "",
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "Kodi user agent",
			userAgent:      "Kodi/20.0",
			expectedPlayer: "kodi",
			expectedFormat: "",
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: true,
		},
		{
			name:           "IPTV client",
			userAgent:      "TiviMate/4.0.0",
			expectedPlayer: "iptv-client",
			expectedFormat: FormatValueMPEGTS,
			expectedSource: "user-agent",
			expectedFMP4:   false,
			expectedMPEGTS: true,
		},
		{
			name:           "Safari on iPad",
			userAgent:      "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 Safari/605.1.15",
			expectedPlayer: "safari",
			expectedFormat: FormatValueHLS,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "Safari on iPhone",
			userAgent:      "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X)",
			expectedPlayer: "safari",
			expectedFormat: FormatValueHLS,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "Shaka player user agent",
			userAgent:      "Shaka/3.0.0",
			expectedPlayer: "shaka",
			expectedFormat: FormatValueDASH,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
		{
			name:           "dash.js user agent",
			userAgent:      "dash.js/4.0.0",
			expectedPlayer: "dash.js",
			expectedFormat: FormatValueDASH,
			expectedSource: "user-agent",
			expectedFMP4:   true,
			expectedMPEGTS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockOutputRequest(tt.userAgent, "", "", "")
			caps := detector.Detect(req)

			assert.Equal(t, tt.expectedPlayer, caps.PlayerName)
			assert.Equal(t, tt.expectedFormat, caps.PreferredFormat)
			assert.Equal(t, tt.expectedSource, caps.DetectionSource)
			assert.Equal(t, tt.expectedFMP4, caps.SupportsFMP4)
			assert.Equal(t, tt.expectedMPEGTS, caps.SupportsMPEGTS)
		})
	}
}

func TestDefaultClientDetector_DetectDefault(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	// No matching criteria - should return defaults
	req := mockOutputRequest("Mozilla/5.0 (Unknown Browser)", "", "", "")
	caps := detector.Detect(req)

	assert.Equal(t, "", caps.PlayerName)
	assert.Equal(t, "", caps.PreferredFormat)
	assert.Equal(t, "default", caps.DetectionSource)
	assert.True(t, caps.SupportsFMP4)
	assert.True(t, caps.SupportsMPEGTS)
}

func TestDefaultClientDetector_DetectionPriority(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	t.Run("format override takes priority over everything", func(t *testing.T) {
		req := mockOutputRequest("hls.js/1.0", "video/mp2t", "mpegts.js", "dash")
		caps := detector.Detect(req)

		assert.Equal(t, FormatValueDASH, caps.PreferredFormat)
		assert.Equal(t, "format_override", caps.DetectionSource)
	})

	t.Run("X-Tvarr-Player takes priority over Accept and User-Agent", func(t *testing.T) {
		req := mockOutputRequest("hls.js/1.0", "video/mp2t", "mpegts.js", "")
		caps := detector.Detect(req)

		assert.Equal(t, "mpegts.js", caps.PlayerName)
		assert.Equal(t, FormatValueMPEGTS, caps.PreferredFormat)
		assert.Equal(t, "x-tvarr-player", caps.DetectionSource)
	})

	t.Run("Accept header takes priority over User-Agent", func(t *testing.T) {
		req := mockOutputRequest("hls.js/1.0", "application/dash+xml", "", "")
		caps := detector.Detect(req)

		assert.Equal(t, FormatValueDASH, caps.PreferredFormat)
		assert.Equal(t, "accept", caps.DetectionSource)
	})

	t.Run("User-Agent is used when no other hints available", func(t *testing.T) {
		req := mockOutputRequest("ExoPlayer/2.18.0", "", "", "")
		caps := detector.Detect(req)

		assert.Equal(t, "exoplayer", caps.PlayerName)
		assert.Equal(t, FormatValueHLSFMP4, caps.PreferredFormat)
		assert.Equal(t, "user-agent", caps.DetectionSource)
	})
}

func TestDefaultClientDetector_NormalizeFormat(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"fmp4", FormatValueHLSFMP4},
		{"FMP4", FormatValueHLSFMP4},
		{"hls-fmp4", FormatValueHLSFMP4},
		{"ts", FormatValueMPEGTS},
		{"mpegts", FormatValueMPEGTS},
		{"mpeg-ts", FormatValueMPEGTS},
		{"hls", FormatValueHLS},
		{"HLS", FormatValueHLS},
		{"hls-ts", FormatValueHLSTS},
		{"dash", FormatValueDASH},
		{"DASH", FormatValueDASH},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := detector.normalizeFormat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultClientDetector_CaseInsensitivity(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	t.Run("X-Tvarr-Player is case insensitive", func(t *testing.T) {
		tests := []string{"HLS.JS", "Hls.Js", "hls.js"}
		for _, xPlayer := range tests {
			req := mockOutputRequest("", "", xPlayer, "")
			caps := detector.Detect(req)
			assert.Equal(t, "hls.js", caps.PlayerName, "Player %s should match hls.js", xPlayer)
		}
	})

	t.Run("Accept header is case insensitive", func(t *testing.T) {
		tests := []string{"Application/Dash+xml", "APPLICATION/DASH+XML", "application/dash+xml"}
		for _, accept := range tests {
			req := mockOutputRequest("", accept, "", "")
			caps := detector.Detect(req)
			assert.Equal(t, FormatValueDASH, caps.PreferredFormat, "Accept %s should match DASH", accept)
		}
	})

	t.Run("User-Agent patterns are case insensitive", func(t *testing.T) {
		tests := []string{"HLS.JS/1.0", "Hls.Js/1.0", "hls.js/1.0"}
		for _, ua := range tests {
			req := mockOutputRequest(ua, "", "", "")
			caps := detector.Detect(req)
			assert.Equal(t, "hls.js", caps.PlayerName, "User-Agent %s should match hls.js", ua)
		}
	})
}
