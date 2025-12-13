package relay

import (
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestRoutingDecision_String(t *testing.T) {
	tests := []struct {
		decision RoutingDecision
		expected string
	}{
		{RoutePassthrough, "passthrough"},
		{RouteRepackage, "repackage"},
		{RouteTranscode, "transcode"},
		{RoutingDecision(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.decision.String())
		})
	}
}

func TestDefaultRoutingDecider_DecideWithTranscodingProfile(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name             string
		sourceFormat     SourceFormat
		sourceCodecs     []string
		profile          *models.EncodingProfile
		expectedDecision RoutingDecision
	}{
		{
			name:         "profile requires transcoding - HLS source",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1.64001f", "mp4a.40.2"},
			profile: &models.EncodingProfile{
				Name:             "Transcode Profile",
				TargetVideoCodec: models.VideoCodecH264,
				TargetAudioCodec: models.AudioCodecAAC,
				QualityPreset:    models.QualityPresetMedium,
			},
			expectedDecision: RouteTranscode,
		},
		{
			name:         "profile requires transcoding - MPEGTS source",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"h264", "aac"},
			profile: &models.EncodingProfile{
				Name:             "Transcode Profile",
				TargetVideoCodec: models.VideoCodecH265,
				TargetAudioCodec: models.AudioCodecOpus,
				QualityPreset:    models.QualityPresetHigh,
			},
			expectedDecision: RouteTranscode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := ClientCapabilities{
				PlayerName:      "test-player",
				PreferredFormat: "",
				SupportsFMP4:    true,
			}

			result := decider.Decide(tt.sourceFormat, tt.sourceCodecs, client, tt.profile)

			assert.Equal(t, tt.expectedDecision, result.Decision, "unexpected routing decision")
			assert.NotEmpty(t, result.Reasons, "reasons should not be empty")
		})
	}
}

func TestDefaultRoutingDecider_DecideWithNilProfile(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name             string
		sourceFormat     SourceFormat
		sourceCodecs     []string
		client           ClientCapabilities
		expectedDecision RoutingDecision
	}{
		{
			name:         "nil profile with client preferring HLS, HLS source - passthrough",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			client: ClientCapabilities{
				PlayerName:      "hls-player",
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			expectedDecision: RoutePassthrough,
		},
		{
			name:         "nil profile with client preferring DASH, HLS source - repackage",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			client: ClientCapabilities{
				PlayerName:      "dash-player",
				PreferredFormat: FormatValueDASH,
				SupportsFMP4:    true,
			},
			expectedDecision: RouteRepackage,
		},
		{
			name:         "nil profile with MPEGTS source - transcode (not segmented)",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"h264", "aac"},
			client: ClientCapabilities{
				PlayerName:      "hls-player",
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			expectedDecision: RouteTranscode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.Decide(tt.sourceFormat, tt.sourceCodecs, tt.client, nil)

			assert.Equal(t, tt.expectedDecision, result.Decision, "unexpected routing decision")
			assert.NotEmpty(t, result.Reasons, "reasons should not be empty")
		})
	}
}

func TestDefaultRoutingDecider_CodecsCompatibleWithMPEGTS(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name     string
		codecs   []string
		expected bool
	}{
		{"H.264 + AAC compatible", []string{"avc1.64001f", "mp4a.40.2"}, true},
		{"H.265 + AAC compatible", []string{"hvc1.1.6.L93.B0", "mp4a.40.2"}, true},
		{"H.264 + AC3 compatible", []string{"avc1", "ac3"}, true},
		{"VP9 + Opus not compatible", []string{"vp9", "opus"}, false},
		{"AV1 + AAC not compatible", []string{"av1", "mp4a"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.codecsCompatibleWithMPEGTS(tt.codecs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRoutingDecider_DetermineOutputFormat(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name           string
		client         ClientCapabilities
		profile        *models.EncodingProfile
		expectedFormat string
	}{
		{
			name: "client prefers HLS",
			client: ClientCapabilities{
				PlayerName:      "hls-player",
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			profile:        nil,
			expectedFormat: FormatValueHLS,
		},
		{
			name: "client prefers DASH",
			client: ClientCapabilities{
				PlayerName:      "dash-player",
				PreferredFormat: FormatValueDASH,
				SupportsFMP4:    true,
			},
			profile:        nil,
			expectedFormat: FormatValueDASH,
		},
		{
			name: "no client preference with fMP4 profile",
			client: ClientCapabilities{
				PlayerName:      "generic-player",
				PreferredFormat: "",
				SupportsFMP4:    true,
			},
			profile: &models.EncodingProfile{
				TargetVideoCodec: models.VideoCodecH264,
				TargetAudioCodec: models.AudioCodecAAC,
			},
			expectedFormat: FormatValueFMP4,
		},
		{
			name: "no client preference, no fMP4 support",
			client: ClientCapabilities{
				PlayerName:      "legacy-player",
				PreferredFormat: "",
				SupportsFMP4:    false,
			},
			profile:        nil,
			expectedFormat: FormatValueMPEGTS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.determineOutputFormat(tt.client, tt.profile)
			assert.Equal(t, tt.expectedFormat, result)
		})
	}
}

func TestDefaultRoutingDecider_IsPassthroughCompatible(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name         string
		sourceFormat SourceFormat
		clientFormat string
		expected     bool
	}{
		{"HLS to HLS compatible", SourceFormatHLS, FormatValueHLS, true},
		{"DASH to DASH compatible", SourceFormatDASH, FormatValueDASH, true},
		{"MPEGTS to MPEGTS compatible", SourceFormatMPEGTS, FormatValueMPEGTS, true},
		{"HLS to DASH not compatible", SourceFormatHLS, FormatValueDASH, false},
		{"DASH to HLS not compatible", SourceFormatDASH, FormatValueHLS, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.isPassthroughCompatible(tt.sourceFormat, tt.clientFormat, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRoutingDecider_IsRepackageCompatible(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name         string
		sourceFormat SourceFormat
		clientFormat string
		codecs       []string
		expected     bool
	}{
		{"HLS to DASH repackage possible", SourceFormatHLS, FormatValueDASH, []string{"avc1", "mp4a"}, true},
		{"DASH to HLS repackage possible", SourceFormatDASH, FormatValueHLS, []string{"avc1", "mp4a"}, true},
		{"MPEGTS to HLS repackage not possible", SourceFormatMPEGTS, FormatValueHLS, []string{"h264", "aac"}, false},
		{"HLS to MPEGTS with incompatible codecs", SourceFormatHLS, FormatValueMPEGTS, []string{"vp9", "opus"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.isRepackageCompatible(tt.sourceFormat, tt.clientFormat, tt.codecs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRoutingDecider_FullDecisionFlow(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	// Test a complete flow: MPEGTS source with HLS client and transcoding profile
	profile := &models.EncodingProfile{
		Name:             "Full Test Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}

	client := ClientCapabilities{
		PlayerName:      "hls-player",
		PreferredFormat: FormatValueHLS,
		SupportsFMP4:    true,
	}

	result := decider.Decide(SourceFormatMPEGTS, []string{"h264", "aac"}, client, profile)

	// Should transcode because profile specifies target codecs
	assert.Equal(t, RouteTranscode, result.Decision)
	assert.Equal(t, FormatValueHLS, result.ClientFormat)
	assert.NotEmpty(t, result.Reasons)
}
