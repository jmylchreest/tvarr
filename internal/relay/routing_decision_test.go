package relay

import (
	"encoding/json"
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

func TestRoutingDecision_JSONMarshal(t *testing.T) {
	tests := []struct {
		decision     RoutingDecision
		expectedJSON string
	}{
		{RoutePassthrough, `"passthrough"`},
		{RouteRepackage, `"repackage"`},
		{RouteTranscode, `"transcode"`},
	}

	for _, tt := range tests {
		t.Run(tt.decision.String(), func(t *testing.T) {
			data, err := json.Marshal(tt.decision)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedJSON, string(data))
		})
	}
}

func TestRoutingDecision_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		jsonData string
		expected RoutingDecision
	}{
		{`"passthrough"`, RoutePassthrough},
		{`"repackage"`, RouteRepackage},
		{`"transcode"`, RouteTranscode},
		{`"unknown"`, RoutePassthrough}, // Unknown defaults to passthrough
	}

	for _, tt := range tests {
		t.Run(tt.jsonData, func(t *testing.T) {
			var decision RoutingDecision
			err := json.Unmarshal([]byte(tt.jsonData), &decision)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, decision)
		})
	}
}

func TestRoutingDecision_JSONRoundTrip(t *testing.T) {
	// Test that a struct with RoutingDecision serializes correctly
	type TestStruct struct {
		RouteType RoutingDecision `json:"route_type"`
		Name      string          `json:"name"`
	}

	original := TestStruct{
		RouteType: RouteTranscode,
		Name:      "test",
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"route_type":"transcode"`)

	var decoded TestStruct
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, original.RouteType, decoded.RouteType)
	assert.Equal(t, original.Name, decoded.Name)
}

func TestDefaultRoutingDecider_DecideWithTranscodingProfile(t *testing.T) {
	decider := NewDefaultRoutingDecider(nil)

	tests := []struct {
		name             string
		sourceFormat     SourceFormat
		sourceCodecs     []string
		client           ClientCapabilities
		profile          *models.EncodingProfile
		expectedDecision RoutingDecision
	}{
		{
			name:         "client accepts source codecs - skip transcoding, repackage HLS",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"avc1.64001f", "mp4a.40.2"}, // h264/aac
			client: ClientCapabilities{
				PlayerName:   "test-player",
				SupportsFMP4: true,
				// No codec restrictions = accepts all
			},
			profile: &models.EncodingProfile{
				Name:             "Transcode Profile",
				TargetVideoCodec: models.VideoCodecH264,
				TargetAudioCodec: models.AudioCodecAAC,
				QualityPreset:    models.QualityPresetMedium,
			},
			// Client accepts source (h264/aac), so no transcoding needed
			expectedDecision: RouteRepackage,
		},
		{
			name:         "client does not accept source audio - transcode required",
			sourceFormat: SourceFormatHLS,
			sourceCodecs: []string{"h264", "eac3"},
			client: ClientCapabilities{
				PlayerName:          "test-player",
				SupportsFMP4:        true,
				AcceptedVideoCodecs: []string{"h264", "h265"},
				AcceptedAudioCodecs: []string{"aac", "mp3"}, // Does NOT accept eac3
			},
			profile: &models.EncodingProfile{
				Name:             "Transcode Profile",
				TargetVideoCodec: models.VideoCodecH264,
				TargetAudioCodec: models.AudioCodecAAC,
				QualityPreset:    models.QualityPresetMedium,
			},
			// Client does NOT accept eac3, so must transcode
			expectedDecision: RouteTranscode,
		},
		{
			name:         "MPEGTS source always needs transcoding for segmentation",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"h264", "aac"},
			client: ClientCapabilities{
				PlayerName:   "test-player",
				SupportsFMP4: true,
				// No codec restrictions = accepts all
			},
			profile: &models.EncodingProfile{
				Name:             "Transcode Profile",
				TargetVideoCodec: models.VideoCodecH265,
				TargetAudioCodec: models.AudioCodecOpus,
				QualityPreset:    models.QualityPresetHigh,
			},
			// Raw MPEGTS is not segmented, needs FFmpeg for segmentation
			expectedDecision: RouteTranscode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decider.Decide(tt.sourceFormat, tt.sourceCodecs, tt.client, tt.profile)

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
		{
			name:         "raw MPEGTS passthrough - client accepts source format and codecs",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"hevc", "eac3"},
			client: ClientCapabilities{
				PlayerName:          "jellyfin",
				SupportsFMP4:        true,
				SupportsMPEGTS:      true, // Client accepts MPEG-TS format
				AcceptedVideoCodecs: []string{"h264", "h265", "hevc"},
				AcceptedAudioCodecs: []string{"aac", "ac3", "eac3"},
			},
			// Client accepts both the source format (MPEG-TS) and codecs (hevc/eac3)
			// so we can passthrough directly without any FFmpeg processing
			expectedDecision: RoutePassthrough,
		},
		{
			name:         "raw MPEGTS with client not accepting source codecs - transcode required",
			sourceFormat: SourceFormatMPEGTS,
			sourceCodecs: []string{"hevc", "eac3"},
			client: ClientCapabilities{
				PlayerName:          "limited-player",
				SupportsFMP4:        true,
				SupportsMPEGTS:      true,
				AcceptedVideoCodecs: []string{"h264"}, // Does NOT accept hevc
				AcceptedAudioCodecs: []string{"aac"},  // Does NOT accept eac3
			},
			// Client accepts MPEG-TS format but NOT the source codecs
			// so we need to transcode
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
