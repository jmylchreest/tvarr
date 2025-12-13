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
		name           string
		formatOverride string
		expectedFormat string
		expectedSource string
		expectedFMP4   bool
		expectedMPEGTS bool
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

func TestDefaultClientDetector_DetectDefault(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	// No matching criteria - should return defaults
	// Note: User-Agent based detection is now handled by database-backed
	// ClientDetectionRules via ClientDetectionService.
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

	t.Run("format override takes priority over Accept header", func(t *testing.T) {
		req := mockOutputRequest("", "video/mp2t", "", "dash")
		caps := detector.Detect(req)

		assert.Equal(t, FormatValueDASH, caps.PreferredFormat)
		assert.Equal(t, "format_override", caps.DetectionSource)
	})

	t.Run("Accept header is used when no format override", func(t *testing.T) {
		req := mockOutputRequest("", "application/dash+xml", "", "")
		caps := detector.Detect(req)

		assert.Equal(t, FormatValueDASH, caps.PreferredFormat)
		assert.Equal(t, "accept", caps.DetectionSource)
	})
}

func TestDefaultClientDetector_CaseInsensitivity(t *testing.T) {
	detector := NewDefaultClientDetector(nil)

	t.Run("Accept header is case insensitive", func(t *testing.T) {
		tests := []string{"Application/Dash+xml", "APPLICATION/DASH+XML", "application/dash+xml"}
		for _, accept := range tests {
			req := mockOutputRequest("", accept, "", "")
			caps := detector.Detect(req)
			assert.Equal(t, FormatValueDASH, caps.PreferredFormat, "Accept %s should match DASH", accept)
		}
	})

	t.Run("Format override is case insensitive", func(t *testing.T) {
		tests := []string{"DASH", "Dash", "dash"}
		for _, format := range tests {
			req := mockOutputRequest("", "", "", format)
			caps := detector.Detect(req)
			assert.Equal(t, FormatValueDASH, caps.PreferredFormat, "Format %s should match DASH", format)
		}
	})
}
