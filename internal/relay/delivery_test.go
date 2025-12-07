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
		profile      *models.RelayProfile
		expected     DeliveryDecision
	}{
		// Profile requires transcoding - always transcode
		{
			name:         "profile requires video transcode",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatHLS,
			profile:      &models.RelayProfile{VideoCodec: models.VideoCodecH264, AudioCodec: models.AudioCodecCopy},
			expected:     DeliveryTranscode,
		},
		{
			name:         "profile requires audio transcode",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatHLS,
			profile:      &models.RelayProfile{VideoCodec: models.VideoCodecCopy, AudioCodec: models.AudioCodecAAC},
			expected:     DeliveryTranscode,
		},

		// No profile or copy profile - format matching
		{
			name:         "HLS to HLS - passthrough",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatHLS,
			profile:      nil,
			expected:     DeliveryPassthrough,
		},
		{
			name:         "DASH to DASH - passthrough",
			sourceFormat: SourceFormatDASH,
			clientFormat: ClientFormatDASH,
			profile:      nil,
			expected:     DeliveryPassthrough,
		},
		{
			name:         "MPEGTS to MPEGTS - passthrough",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatMPEGTS,
			profile:      nil,
			expected:     DeliveryPassthrough,
		},
		{
			name:         "HLS to HLS with copy profile - passthrough",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatHLS,
			profile:      &models.RelayProfile{VideoCodec: models.VideoCodecCopy, AudioCodec: models.AudioCodecCopy},
			expected:     DeliveryPassthrough,
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

		// Auto format - passthrough
		{
			name:         "HLS to Auto - passthrough",
			sourceFormat: SourceFormatHLS,
			clientFormat: ClientFormatAuto,
			profile:      nil,
			expected:     DeliveryPassthrough,
		},
		{
			name:         "MPEGTS to Auto - passthrough",
			sourceFormat: SourceFormatMPEGTS,
			clientFormat: ClientFormatAuto,
			profile:      nil,
			expected:     DeliveryPassthrough,
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

func TestNewDeliveryContext(t *testing.T) {
	source := ClassificationResult{
		SourceFormat: SourceFormatHLS,
		Mode:         StreamModePassthroughHLS,
	}
	clientFormat := ClientFormatDASH
	profile := &models.RelayProfile{
		VideoCodec: models.VideoCodecCopy,
		AudioCodec: models.AudioCodecCopy,
	}

	ctx := NewDeliveryContext(source, clientFormat, profile)

	assert.Equal(t, source, ctx.Source)
	assert.Equal(t, clientFormat, ctx.ClientFormat)
	assert.Equal(t, profile, ctx.Profile)
	assert.Equal(t, DeliveryRepackage, ctx.Decision) // HLS to DASH with copy profile = repackage
}
