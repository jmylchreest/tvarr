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

func TestDefaultRoutingDecider_DecideWithExplicitDetectionMode(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name           string
		sourceFormat   SourceFormat
		sourceCodecs   []string
		profile        *models.RelayProfile
		expectedDecision RoutingDecision
		expectedFormat string
	}{
		{
			name:         "detection_mode=hls with HLS source - passthrough",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1.64001f", "mp4a.40.2"},
			profile: &models.RelayProfile{
				Name:          "HLS Profile",
				DetectionMode: models.DetectionModeHLS,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RoutePassthrough,
			expectedFormat:   FormatValueHLS,
		},
		{
			name:         "detection_mode=mpegts with MPEGTS source - transcode (not segmented)",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"h264", "aac"},
			profile: &models.RelayProfile{
				Name:          "MPEGTS Profile",
				DetectionMode: models.DetectionModeMPEGTS,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RouteTranscode,
			expectedFormat:   FormatValueMPEGTS,
		},
		{
			name:         "detection_mode=dash with DASH source - passthrough",
			sourceFormat: SourceFormatDASH,
			sourceCodecs: []string{"avc1", "mp4a"},
			profile: &models.RelayProfile{
				Name:          "DASH Profile",
				DetectionMode: models.DetectionModeDASH,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RoutePassthrough,
			expectedFormat:   FormatValueDASH,
		},
		{
			name:         "detection_mode=hls with DASH source - repackage",
			sourceFormat: SourceFormatDASH,
			sourceCodecs: []string{"avc1", "mp4a"},
			profile: &models.RelayProfile{
				Name:          "HLS from DASH Profile",
				DetectionMode: models.DetectionModeHLS,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RouteRepackage,
			expectedFormat:   FormatValueHLS,
		},
		{
			name:         "detection_mode=hls with MPEGTS source - transcode (not segmented)",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"h264", "aac"},
			profile: &models.RelayProfile{
				Name:          "HLS from MPEGTS Profile",
				DetectionMode: models.DetectionModeHLS,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RouteTranscode,
			expectedFormat:   FormatValueHLS,
		},
		{
			name:         "detection_mode=hls with transcode profile - transcode",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			profile: &models.RelayProfile{
				Name:          "Transcode to H264 Profile",
				DetectionMode: models.DetectionModeHLS,
				VideoCodec:    models.VideoCodecH264,
				AudioCodec:    models.AudioCodecAAC,
			},
			expectedDecision: RouteTranscode,
			expectedFormat:   FormatValueHLS,
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
			assert.Equal(t, tt.expectedFormat, result.ClientFormat, "unexpected client format")
			assert.NotEmpty(t, result.Reasons, "reasons should not be empty")
		})
	}
}

func TestDefaultRoutingDecider_DecideWithAutoDetectionMode(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name             string
		sourceFormat     SourceFormat
		sourceCodecs     []string
		client           ClientCapabilities
		profile          *models.RelayProfile
		expectedDecision RoutingDecision
	}{
		{
			name:         "auto mode with client preferring HLS, HLS source - passthrough",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			client: ClientCapabilities{
				PlayerName:      "hls.js",
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			profile: &models.RelayProfile{
				Name:          "Auto Profile",
				DetectionMode: models.DetectionModeAuto,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RoutePassthrough,
		},
		{
			name:         "auto mode with client preferring DASH, HLS source - repackage",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			client: ClientCapabilities{
				PlayerName:      "dash.js",
				PreferredFormat: FormatValueDASH,
				SupportsFMP4:    true,
			},
			profile: &models.RelayProfile{
				Name:          "Auto Profile",
				DetectionMode: models.DetectionModeAuto,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RouteRepackage,
		},
		{
			name:         "auto mode with profile requiring transcode - transcode",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			client: ClientCapabilities{
				PlayerName:      "hls.js",
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			profile: &models.RelayProfile{
				Name:          "Transcode Profile",
				DetectionMode: models.DetectionModeAuto,
				VideoCodec:    models.VideoCodecH264,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RouteTranscode,
		},
		{
			name:         "auto mode with raw TS source - transcode (needs segmentation)",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"h264", "aac"},
			client: ClientCapabilities{
				PlayerName:      "hls.js",
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			profile: &models.RelayProfile{
				Name:          "Auto Profile",
				DetectionMode: models.DetectionModeAuto,
				VideoCodec:    models.VideoCodecCopy,
				AudioCodec:    models.AudioCodecCopy,
			},
			expectedDecision: RouteTranscode,
		},
		{
			name:         "auto mode with no client preference, fMP4 support - uses fMP4",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1", "mp4a"},
			client: ClientCapabilities{
				PlayerName:      "generic",
				PreferredFormat: "",
				SupportsFMP4:    true,
			},
			profile: &models.RelayProfile{
				Name:            "Auto Profile",
				DetectionMode:   models.DetectionModeAuto,
				VideoCodec:      models.VideoCodecCopy,
				AudioCodec:      models.AudioCodecCopy,
				ContainerFormat: models.ContainerFormatAuto,
			},
			expectedDecision: RoutePassthrough,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.Decide(tt.sourceFormat, tt.sourceCodecs, tt.client, tt.profile)

			assert.Equal(t, tt.expectedDecision, result.Decision, "unexpected routing decision")
			assert.Contains(t, result.Reasons, "detection_mode is auto - applying client detection")
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
		{"H264 + AAC - compatible", []string{"h264", "aac"}, true},
		{"AVC1 + MP4A - compatible", []string{"avc1.64001f", "mp4a.40.2"}, true},
		{"HEVC + AAC - compatible", []string{"hevc", "aac"}, true},
		{"H265 + AC3 - compatible", []string{"h265", "ac3"}, true},
		{"H264 + MP3 - compatible", []string{"h264", "mp3"}, true},
		{"H264 + EAC3 - compatible", []string{"h264", "eac3"}, true},
		{"VP9 + AAC - incompatible", []string{"vp9", "aac"}, false},
		{"AV1 + AAC - incompatible", []string{"av1", "aac"}, false},
		{"H264 + OPUS - incompatible", []string{"h264", "opus"}, false},
		{"Empty codecs - compatible", []string{}, true},
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
		name     string
		client   ClientCapabilities
		profile  *models.RelayProfile
		expected string
	}{
		{
			name: "client prefers HLS",
			client: ClientCapabilities{
				PreferredFormat: FormatValueHLS,
				SupportsFMP4:    true,
			},
			profile:  &models.RelayProfile{ContainerFormat: models.ContainerFormatAuto},
			expected: FormatValueHLS,
		},
		{
			name: "client prefers DASH",
			client: ClientCapabilities{
				PreferredFormat: FormatValueDASH,
				SupportsFMP4:    true,
			},
			profile:  &models.RelayProfile{ContainerFormat: models.ContainerFormatAuto},
			expected: FormatValueDASH,
		},
		{
			name: "no client preference, profile specifies MPEGTS",
			client: ClientCapabilities{
				PreferredFormat: "",
				SupportsFMP4:    true,
			},
			profile:  &models.RelayProfile{ContainerFormat: models.ContainerFormatMPEGTS},
			expected: string(models.ContainerFormatMPEGTS),
		},
		{
			name: "no client preference, auto profile, supports fMP4",
			client: ClientCapabilities{
				PreferredFormat: "",
				SupportsFMP4:    true,
			},
			profile:  &models.RelayProfile{ContainerFormat: models.ContainerFormatAuto},
			expected: FormatValueHLSFMP4,
		},
		{
			name: "no client preference, auto profile, no fMP4 support",
			client: ClientCapabilities{
				PreferredFormat: "",
				SupportsFMP4:    false,
			},
			profile:  &models.RelayProfile{ContainerFormat: models.ContainerFormatAuto},
			expected: FormatValueMPEGTS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.determineOutputFormat(tt.client, tt.profile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRoutingDecider_IsPassthroughCompatible(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name         string
		sourceFormat SourceFormat
		clientFormat string
		codecs       []string
		expected     bool
	}{
		{"HLS to HLS - compatible", SourceFormatHLS, FormatValueHLS, []string{"avc1"}, true},
		{"HLS to HLS-fMP4 - compatible", SourceFormatHLS, FormatValueHLSFMP4, []string{"avc1"}, true},
		{"HLS to HLS-TS - compatible", SourceFormatHLS, FormatValueHLSTS, []string{"avc1"}, true},
		{"DASH to DASH - compatible", SourceFormatDASH, FormatValueDASH, []string{"avc1"}, true},
		{"MPEGTS to MPEGTS - compatible", SourceFormatMPEGTS, FormatValueMPEGTS, []string{"h264"}, true},
		{"HLS to DASH - incompatible", SourceFormatHLS, FormatValueDASH, []string{"avc1"}, false},
		{"DASH to HLS - incompatible", SourceFormatDASH, FormatValueHLS, []string{"avc1"}, false},
		{"MPEGTS to HLS - incompatible", SourceFormatMPEGTS, FormatValueHLS, []string{"h264"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.isPassthroughCompatible(tt.sourceFormat, tt.clientFormat, tt.codecs)
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
		{"HLS to DASH - can repackage", SourceFormatHLS, FormatValueDASH, []string{"avc1", "mp4a"}, true},
		{"DASH to HLS - can repackage", SourceFormatDASH, FormatValueHLS, []string{"avc1", "mp4a"}, true},
		{"HLS to MPEGTS with compatible codecs - can repackage", SourceFormatHLS, FormatValueMPEGTS, []string{"h264", "aac"}, true},
		{"HLS to MPEGTS with VP9 - cannot repackage", SourceFormatHLS, FormatValueMPEGTS, []string{"vp9", "opus"}, false},
		{"MPEGTS to HLS - cannot repackage (not segmented)", SourceFormatMPEGTS, FormatValueHLS, []string{"h264"}, false},
		{"Unknown to HLS - cannot repackage", SourceFormatUnknown, FormatValueHLS, []string{"h264"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.isRepackageCompatible(tt.sourceFormat, tt.clientFormat, tt.codecs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoutingResult_ReasonsArePopulated(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	profile := &models.RelayProfile{
		Name:          "Test Profile",
		DetectionMode: models.DetectionModeAuto,
		VideoCodec:    models.VideoCodecCopy,
		AudioCodec:    models.AudioCodecCopy,
	}

	client := ClientCapabilities{
		PlayerName:      "test-player",
		PreferredFormat: FormatValueHLS,
		SupportsFMP4:    true,
	}

	result := decider.Decide(SourceFormatHLS, []string{"avc1", "mp4a"}, client, profile)

	// Should have reasons explaining the decision
	assert.NotEmpty(t, result.Reasons)
	assert.Contains(t, result.Reasons, "detection_mode is auto - applying client detection")
}
