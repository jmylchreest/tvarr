package relay

import (
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestDeliveryDecision_String(t *testing.T) {
	tests := []struct {
		decision DeliveryDecision
		expected string
	}{
		{DeliveryPassthrough, "passthrough"},
		{DeliveryRepackage, "repackage"},
		{DeliveryTranscode, "transcode"},
		{DeliveryDecision(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.decision.String())
		})
	}
}

func TestSourceMatchesClient(t *testing.T) {
	tests := []struct {
		name         string
		sourceFormat SourceFormat
		clientFormat ClientFormat
		expected     bool
	}{
		// Exact matches
		{"HLS to HLS", SourceFormatHLS, ClientFormatHLS, true},
		{"DASH to DASH", SourceFormatDASH, ClientFormatDASH, true},
		{"MPEGTS to MPEGTS", SourceFormatMPEGTS, ClientFormatMPEGTS, true},

		// Mismatches
		{"HLS to DASH", SourceFormatHLS, ClientFormatDASH, false},
		{"DASH to HLS", SourceFormatDASH, ClientFormatHLS, false},
		{"MPEGTS to HLS", SourceFormatMPEGTS, ClientFormatHLS, false},
		{"HLS to MPEGTS", SourceFormatHLS, ClientFormatMPEGTS, false},

		// Auto always matches
		{"HLS to Auto", SourceFormatHLS, ClientFormatAuto, true},
		{"DASH to Auto", SourceFormatDASH, ClientFormatAuto, true},
		{"MPEGTS to Auto", SourceFormatMPEGTS, ClientFormatAuto, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := ClassificationResult{SourceFormat: tt.sourceFormat}
			result := sourceMatchesClient(source, tt.clientFormat)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanRepackage(t *testing.T) {
	tests := []struct {
		name         string
		sourceFormat SourceFormat
		clientFormat ClientFormat
		expected     bool
	}{
		// HLS source repackaging
		{"HLS to DASH - can repackage", SourceFormatHLS, ClientFormatDASH, true},
		{"HLS to HLS - no repackage needed", SourceFormatHLS, ClientFormatHLS, false},
		{"HLS to MPEGTS - cannot repackage", SourceFormatHLS, ClientFormatMPEGTS, false},

		// DASH source repackaging
		{"DASH to HLS - can repackage", SourceFormatDASH, ClientFormatHLS, true},
		{"DASH to DASH - no repackage needed", SourceFormatDASH, ClientFormatDASH, false},
		{"DASH to MPEGTS - cannot repackage", SourceFormatDASH, ClientFormatMPEGTS, false},

		// MPEGTS source - cannot repackage (no segments)
		{"MPEGTS to HLS - cannot repackage", SourceFormatMPEGTS, ClientFormatHLS, false},
		{"MPEGTS to DASH - cannot repackage", SourceFormatMPEGTS, ClientFormatDASH, false},
		{"MPEGTS to MPEGTS - no repackage needed", SourceFormatMPEGTS, ClientFormatMPEGTS, false},

		// Unknown source
		{"Unknown to HLS - cannot repackage", SourceFormatUnknown, ClientFormatHLS, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := ClassificationResult{SourceFormat: tt.sourceFormat}
			result := canRepackage(source, tt.clientFormat)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSelectDelivery(t *testing.T) {
	tests := []struct {
		name         string
		sourceFormat SourceFormat
		clientFormat ClientFormat
		profile      *models.EncodingProfile
		expected     DeliveryDecision
	}{
		// Profile requires transcoding - always transcode
		{
			name:         "profile requires video transcode",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatHLS,
			profile:      &models.EncodingProfile{TargetVideoCodec: models.VideoCodecH264, TargetAudioCodec: models.AudioCodecAAC},
			expected:     DeliveryTranscode,
		},

		// No profile - format matching uses repackage (buffer pipeline for connection sharing)
		// NOTE: We use DeliveryRepackage instead of DeliveryPassthrough to enable
		// multiple clients sharing a single upstream connection via the ES buffer
		{
			name:         "HLS to HLS - repackage for buffer sharing",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatHLS,
			profile:      nil,
			expected:     DeliveryRepackage,
		},
		{
			name:         "DASH to DASH - repackage for buffer sharing",
			sourceFormat: SourceFormatDASH,
			clientFormat: ClientFormatDASH,
			profile:      nil,
			expected:     DeliveryRepackage,
		},
		{
			name:         "MPEGTS to MPEGTS - repackage for buffer sharing",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      nil,
			expected:     DeliveryRepackage,
		},

		// Repackaging scenarios
		{
			name:         "HLS to DASH - repackage",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatDASH,
			profile:      nil,
			expected:     DeliveryRepackage,
		},
		{
			name:         "DASH to HLS - repackage",
			sourceFormat: SourceFormatDASH,
			clientFormat: ClientFormatHLS,
			profile:      nil,
			expected:     DeliveryRepackage,
		},

		// Transcode required scenarios (TS to HLS/DASH)
		{
			name:         "MPEGTS to HLS - must transcode",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatHLS,
			profile:      nil,
			expected:     DeliveryTranscode,
		},
		{
			name:         "MPEGTS to DASH - must transcode",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatDASH,
			profile:      nil,
			expected:     DeliveryTranscode,
		},

		// Auto format - repackage for buffer sharing
		{
			name:         "HLS to Auto - repackage",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatAuto,
			profile:      nil,
			expected:     DeliveryRepackage,
		},
		{
			name:         "MPEGTS to Auto - repackage",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatAuto,
			profile:      nil,
			expected:     DeliveryRepackage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := ClassificationResult{SourceFormat: tt.sourceFormat}
			result := SelectDelivery(source, tt.clientFormat, tt.profile)
			assert.Equal(t, tt.expected, result, "expected %s, got %s", tt.expected, result)
		})
	}
}

func TestSelectDeliveryWithCodecCompatibility(t *testing.T) {
	// Profile that would require transcoding (has target codecs set)
	transcodeProfile := &models.EncodingProfile{
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
	}

	tests := []struct {
		name             string
		sourceFormat     SourceFormat
		clientFormat     ClientFormat
		profile          *models.EncodingProfile
		clientCaps       *ClientCapabilities
		sourceVideoCodec string
		sourceAudioCodec string
		expected         DeliveryDecision
	}{
		// Client accepts source codecs with matching format - repackage (uses ES buffer for connection sharing)
		// NOTE: We use DeliveryRepackage instead of DeliveryPassthrough to enable
		// multiple clients sharing a single upstream connection via the ES buffer
		{
			name:         "client accepts h265/eac3 source - repackage for buffer sharing",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264", "h265"},
				AcceptedAudioCodecs: []string{"aac", "eac3"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         DeliveryRepackage,
		},
		// Client accepts source codecs with auto format - repackage for buffer sharing
		{
			name:         "client accepts h265/eac3 with auto format - repackage",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatAuto,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264", "h265"},
				AcceptedAudioCodecs: []string{"aac", "eac3"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         DeliveryRepackage,
		},
		// hevc alias normalized to h265 - repackage
		{
			name:         "hevc normalized to h265 - repackage",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h265"},
				AcceptedAudioCodecs: []string{"aac"},
			},
			sourceVideoCodec: "hevc", // Should be normalized to h265
			sourceAudioCodec: "aac",
			expected:         DeliveryRepackage,
		},
		// ec-3 alias normalized to eac3 - repackage
		{
			name:         "ec-3 normalized to eac3 - repackage",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"},
				AcceptedAudioCodecs: []string{"eac3"},
			},
			sourceVideoCodec: "h264",
			sourceAudioCodec: "ec-3", // Should be normalized to eac3
			expected:         DeliveryRepackage,
		},
		// Client does not accept source video codec - transcode
		{
			name:         "client rejects h265 video - transcode",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"}, // Does not include h265
				AcceptedAudioCodecs: []string{"aac", "eac3"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         DeliveryTranscode,
		},
		// Client does not accept source audio codec - transcode
		{
			name:         "client rejects eac3 audio - transcode",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264", "h265"},
				AcceptedAudioCodecs: []string{"aac"}, // Does not include eac3
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         DeliveryTranscode,
		},
		// Client accepts codecs but format doesn't match - transcode (TS to HLS)
		{
			name:         "client accepts codecs but wants HLS from TS - transcode",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatHLS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h265"},
				AcceptedAudioCodecs: []string{"eac3"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         DeliveryTranscode,
		},
		// Video-only stream (no audio) - repackage when video matches (buffer sharing)
		{
			name:         "video-only stream client accepts - repackage",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"},
				AcceptedAudioCodecs: []string{"aac"},
			},
			sourceVideoCodec: "h264",
			sourceAudioCodec: "", // Video-only
			expected:         DeliveryRepackage,
		},
		// No client capabilities provided - falls back to profile check (transcode)
		{
			name:             "no client caps with profile - transcode",
			sourceFormat:     SourceFormatMPEGTS,
			clientFormat:     ClientFormatMPEGTS,
			profile:          transcodeProfile,
			clientCaps:       nil,
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         DeliveryTranscode,
		},
		// No source video codec known - falls back to profile check (transcode)
		{
			name:         "no source codec known with profile - transcode",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      transcodeProfile,
			clientCaps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h265"},
				AcceptedAudioCodecs: []string{"eac3"},
			},
			sourceVideoCodec: "", // Unknown
			sourceAudioCodec: "eac3",
			expected:         DeliveryTranscode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := ClassificationResult{SourceFormat: tt.sourceFormat}
			opts := SelectDeliveryOptions{
				ClientCapabilities: tt.clientCaps,
				SourceVideoCodec:   tt.sourceVideoCodec,
				SourceAudioCodec:   tt.sourceAudioCodec,
			}
			result := SelectDelivery(source, tt.clientFormat, tt.profile, opts)
			assert.Equal(t, tt.expected, result, "expected %s, got %s", tt.expected, result)
		})
	}
}

func TestClientAcceptsSourceCodecs(t *testing.T) {
	tests := []struct {
		name             string
		caps             *ClientCapabilities
		sourceVideoCodec string
		sourceAudioCodec string
		expected         bool
	}{
		// Basic acceptance
		{
			name: "accepts both codecs",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264", "h265"},
				AcceptedAudioCodecs: []string{"aac", "eac3"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         true,
		},
		// Codec normalization
		{
			name: "hevc normalized to h265",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h265"},
				AcceptedAudioCodecs: []string{"aac"},
			},
			sourceVideoCodec: "hevc",
			sourceAudioCodec: "aac",
			expected:         true,
		},
		{
			name: "ec-3 normalized to eac3",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"},
				AcceptedAudioCodecs: []string{"eac3"},
			},
			sourceVideoCodec: "h264",
			sourceAudioCodec: "ec-3",
			expected:         true,
		},
		{
			name: "avc normalized to h264",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"},
				AcceptedAudioCodecs: []string{"aac"},
			},
			sourceVideoCodec: "avc",
			sourceAudioCodec: "aac",
			expected:         true,
		},
		// Rejection cases
		{
			name: "video not accepted",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"},
				AcceptedAudioCodecs: []string{"aac", "eac3"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         false,
		},
		{
			name: "audio not accepted",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264", "h265"},
				AcceptedAudioCodecs: []string{"aac"},
			},
			sourceVideoCodec: "h265",
			sourceAudioCodec: "eac3",
			expected:         false,
		},
		// Video-only stream
		{
			name: "video-only accepted",
			caps: &ClientCapabilities{
				AcceptedVideoCodecs: []string{"h264"},
				AcceptedAudioCodecs: []string{"aac"},
			},
			sourceVideoCodec: "h264",
			sourceAudioCodec: "",
			expected:         true,
		},
		// Nil capabilities
		{
			name:             "nil caps",
			caps:             nil,
			sourceVideoCodec: "h264",
			sourceAudioCodec: "aac",
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clientAcceptsSourceCodecs(tt.caps, tt.sourceVideoCodec, tt.sourceAudioCodec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDeliveryContext(t *testing.T) {
	source := ClassificationResult{
		SourceFormat: SourceFormatHLS,
		Mode:         StreamModePassthroughHLS,
	}
	clientFormat := ClientFormatDASH
	// EncodingProfile with no target codecs means no transcoding needed
	profile := &models.EncodingProfile{
		Name: "test-profile",
	}

	ctx := NewDeliveryContext(source, clientFormat, profile)

	assert.Equal(t, source, ctx.Source)
	assert.Equal(t, clientFormat, ctx.ClientFormat)
	assert.Equal(t, profile, ctx.EncodingProfile)
	assert.Equal(t, DeliveryRepackage, ctx.Decision) // HLS to DASH with no transcoding = repackage
}
