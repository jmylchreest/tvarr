package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T065: Unit test for QualityPreset.GetEncodingParams() returning correct values.
// This tests that each quality preset returns the expected FFmpeg encoding parameters.
func TestQualityPreset_GetEncodingParams(t *testing.T) {
	t.Run("low preset returns bandwidth-optimized values", func(t *testing.T) {
		params := QualityPresetLow.GetEncodingParams()

		assert.Equal(t, 28, params.CRF, "Low preset should have CRF 28")
		assert.Equal(t, "2M", params.Maxrate, "Low preset should have 2M max bitrate")
		assert.Equal(t, "4M", params.Bufsize, "Low preset should have 4M buffer size")
		assert.Equal(t, "128k", params.AudioBitrate, "Low preset should have 128k audio bitrate")
		assert.Equal(t, "fast", params.VideoPreset, "Low preset should use fast video preset")
	})

	t.Run("medium preset returns balanced values", func(t *testing.T) {
		params := QualityPresetMedium.GetEncodingParams()

		assert.Equal(t, 23, params.CRF, "Medium preset should have CRF 23")
		assert.Equal(t, "5M", params.Maxrate, "Medium preset should have 5M max bitrate")
		assert.Equal(t, "10M", params.Bufsize, "Medium preset should have 10M buffer size")
		assert.Equal(t, "192k", params.AudioBitrate, "Medium preset should have 192k audio bitrate")
		assert.Equal(t, "medium", params.VideoPreset, "Medium preset should use medium video preset")
	})

	t.Run("high preset returns high quality values", func(t *testing.T) {
		params := QualityPresetHigh.GetEncodingParams()

		assert.Equal(t, 20, params.CRF, "High preset should have CRF 20")
		assert.Equal(t, "10M", params.Maxrate, "High preset should have 10M max bitrate")
		assert.Equal(t, "20M", params.Bufsize, "High preset should have 20M buffer size")
		assert.Equal(t, "256k", params.AudioBitrate, "High preset should have 256k audio bitrate")
		assert.Equal(t, "medium", params.VideoPreset, "High preset should use medium video preset")
	})

	t.Run("ultra preset returns maximum quality values", func(t *testing.T) {
		params := QualityPresetUltra.GetEncodingParams()

		assert.Equal(t, 16, params.CRF, "Ultra preset should have CRF 16")
		assert.Equal(t, "", params.Maxrate, "Ultra preset should have no bitrate cap")
		assert.Equal(t, "", params.Bufsize, "Ultra preset should have no buffer size")
		assert.Equal(t, "320k", params.AudioBitrate, "Ultra preset should have 320k audio bitrate")
		assert.Equal(t, "slow", params.VideoPreset, "Ultra preset should use slow video preset")
	})

	t.Run("unknown preset defaults to medium values", func(t *testing.T) {
		unknownPreset := QualityPreset("invalid")
		params := unknownPreset.GetEncodingParams()

		// Should fall back to medium preset values
		assert.Equal(t, 23, params.CRF, "Unknown preset should default to CRF 23 (medium)")
		assert.Equal(t, "5M", params.Maxrate, "Unknown preset should default to 5M max bitrate (medium)")
		assert.Equal(t, "10M", params.Bufsize, "Unknown preset should default to 10M buffer size (medium)")
		assert.Equal(t, "192k", params.AudioBitrate, "Unknown preset should default to 192k audio bitrate (medium)")
		assert.Equal(t, "medium", params.VideoPreset, "Unknown preset should default to medium video preset")
	})
}

// TestQualityPreset_IsValid tests the IsValid method for quality presets.
func TestQualityPreset_IsValid(t *testing.T) {
	t.Run("valid presets return true", func(t *testing.T) {
		validPresets := []QualityPreset{QualityPresetLow, QualityPresetMedium, QualityPresetHigh, QualityPresetUltra}
		for _, preset := range validPresets {
			assert.True(t, preset.IsValid(), "Preset %q should be valid", preset)
		}
	})

	t.Run("invalid presets return false", func(t *testing.T) {
		invalidPresets := []QualityPreset{"", "invalid", "LOW", "MEDIUM", "super", "max"}
		for _, preset := range invalidPresets {
			assert.False(t, preset.IsValid(), "Preset %q should be invalid", preset)
		}
	})
}

// TestValidQualityPresets tests the ValidQualityPresets function.
func TestValidQualityPresets(t *testing.T) {
	presets := ValidQualityPresets()

	assert.Len(t, presets, 4, "Should have 4 valid quality presets")
	assert.Contains(t, presets, QualityPresetLow)
	assert.Contains(t, presets, QualityPresetMedium)
	assert.Contains(t, presets, QualityPresetHigh)
	assert.Contains(t, presets, QualityPresetUltra)
}

// TestEncodingProfile_GetEncodingParams tests that EncodingProfile delegates to QualityPreset.
func TestEncodingProfile_GetEncodingParams(t *testing.T) {
	t.Run("profile returns encoding params from its quality preset", func(t *testing.T) {
		profile := &EncodingProfile{
			Name:             "Test Profile",
			QualityPreset:    QualityPresetHigh,
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
		}

		params := profile.GetEncodingParams()

		// Should match QualityPresetHigh values
		assert.Equal(t, 20, params.CRF)
		assert.Equal(t, "10M", params.Maxrate)
		assert.Equal(t, "20M", params.Bufsize)
		assert.Equal(t, "256k", params.AudioBitrate)
		assert.Equal(t, "medium", params.VideoPreset)
	})

	t.Run("profile with different presets returns different params", func(t *testing.T) {
		lowProfile := &EncodingProfile{
			Name:             "Low Quality",
			QualityPreset:    QualityPresetLow,
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
		}

		ultraProfile := &EncodingProfile{
			Name:             "Ultra Quality",
			QualityPreset:    QualityPresetUltra,
			TargetVideoCodec: VideoCodecH265,
			TargetAudioCodec: AudioCodecOpus,
		}

		lowParams := lowProfile.GetEncodingParams()
		ultraParams := ultraProfile.GetEncodingParams()

		// CRF should differ significantly between low and ultra
		assert.Greater(t, lowParams.CRF, ultraParams.CRF, "Low preset should have higher CRF than ultra")
		assert.NotEqual(t, lowParams.AudioBitrate, ultraParams.AudioBitrate, "Audio bitrates should differ")
	})
}

// TestEncodingProfile_GetVideoBitrate tests video bitrate calculation from quality preset.
func TestEncodingProfile_GetVideoBitrate(t *testing.T) {
	t.Run("returns bitrate in kbps from maxrate", func(t *testing.T) {
		tests := []struct {
			preset          QualityPreset
			expectedBitrate int
		}{
			{QualityPresetLow, 2000},    // 2M -> 2000 kbps
			{QualityPresetMedium, 5000}, // 5M -> 5000 kbps
			{QualityPresetHigh, 10000},  // 10M -> 10000 kbps
			{QualityPresetUltra, 0},     // No limit -> 0
		}

		for _, tt := range tests {
			t.Run(string(tt.preset), func(t *testing.T) {
				profile := &EncodingProfile{
					Name:             "Test",
					QualityPreset:    tt.preset,
					TargetVideoCodec: VideoCodecH264,
					TargetAudioCodec: AudioCodecAAC,
				}

				bitrate := profile.GetVideoBitrate()
				assert.Equal(t, tt.expectedBitrate, bitrate, "Preset %s should have bitrate %d", tt.preset, tt.expectedBitrate)
			})
		}
	})

	t.Run("returns 0 when custom output flags are set", func(t *testing.T) {
		profile := &EncodingProfile{
			Name:             "Custom",
			QualityPreset:    QualityPresetHigh,
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
			OutputFlags:      "-c:v libx264 -preset fast -crf 18", // Custom flags
		}

		bitrate := profile.GetVideoBitrate()
		assert.Equal(t, 0, bitrate, "Custom output flags should return 0 bitrate (user manages)")
	})
}

// TestEncodingProfile_GetAudioBitrate tests audio bitrate calculation from quality preset.
func TestEncodingProfile_GetAudioBitrate(t *testing.T) {
	t.Run("returns bitrate in kbps", func(t *testing.T) {
		tests := []struct {
			preset          QualityPreset
			expectedBitrate int
		}{
			{QualityPresetLow, 128},    // 128k -> 128 kbps
			{QualityPresetMedium, 192}, // 192k -> 192 kbps
			{QualityPresetHigh, 256},   // 256k -> 256 kbps
			{QualityPresetUltra, 320},  // 320k -> 320 kbps
		}

		for _, tt := range tests {
			t.Run(string(tt.preset), func(t *testing.T) {
				profile := &EncodingProfile{
					Name:             "Test",
					QualityPreset:    tt.preset,
					TargetVideoCodec: VideoCodecH264,
					TargetAudioCodec: AudioCodecAAC,
				}

				bitrate := profile.GetAudioBitrate()
				assert.Equal(t, tt.expectedBitrate, bitrate, "Preset %s should have audio bitrate %d", tt.preset, tt.expectedBitrate)
			})
		}
	})

	t.Run("returns 0 when custom output flags are set", func(t *testing.T) {
		profile := &EncodingProfile{
			Name:             "Custom",
			QualityPreset:    QualityPresetHigh,
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
			OutputFlags:      "-c:a aac -b:a 256k", // Custom flags
		}

		bitrate := profile.GetAudioBitrate()
		assert.Equal(t, 0, bitrate, "Custom output flags should return 0 bitrate (user manages)")
	})
}

// TestEncodingProfile_GetVideoPreset tests video preset retrieval from quality preset.
func TestEncodingProfile_GetVideoPreset(t *testing.T) {
	t.Run("returns preset from quality setting", func(t *testing.T) {
		tests := []struct {
			quality        QualityPreset
			expectedPreset string
		}{
			{QualityPresetLow, "fast"},
			{QualityPresetMedium, "medium"},
			{QualityPresetHigh, "medium"},
			{QualityPresetUltra, "slow"},
		}

		for _, tt := range tests {
			t.Run(string(tt.quality), func(t *testing.T) {
				profile := &EncodingProfile{
					Name:             "Test",
					QualityPreset:    tt.quality,
					TargetVideoCodec: VideoCodecH264,
					TargetAudioCodec: AudioCodecAAC,
				}

				preset := profile.GetVideoPreset()
				assert.Equal(t, tt.expectedPreset, preset, "Quality %s should have video preset %s", tt.quality, tt.expectedPreset)
			})
		}
	})

	t.Run("returns empty when custom output flags are set", func(t *testing.T) {
		profile := &EncodingProfile{
			Name:             "Custom",
			QualityPreset:    QualityPresetHigh,
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
			OutputFlags:      "-c:v libx264 -preset veryfast", // Custom flags
		}

		preset := profile.GetVideoPreset()
		assert.Equal(t, "", preset, "Custom output flags should return empty preset (user manages)")
	})
}

// TestEncodingProfile_Validate tests profile validation with quality presets.
func TestEncodingProfile_Validate(t *testing.T) {
	t.Run("valid profile with valid quality preset passes validation", func(t *testing.T) {
		profile := &EncodingProfile{
			Name:             "Valid Profile",
			QualityPreset:    QualityPresetMedium,
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
		}

		err := profile.Validate()
		require.NoError(t, err)
	})

	t.Run("profile with invalid quality preset fails validation", func(t *testing.T) {
		profile := &EncodingProfile{
			Name:             "Invalid Profile",
			QualityPreset:    QualityPreset("invalid"),
			TargetVideoCodec: VideoCodecH264,
			TargetAudioCodec: AudioCodecAAC,
		}

		err := profile.Validate()
		require.Error(t, err)
		assert.Equal(t, ErrEncodingProfileInvalidQualityPreset, err)
	})

	t.Run("profile with custom output flags skips quality preset validation", func(t *testing.T) {
		// When custom output flags are provided, quality preset isn't validated
		// because the user is managing encoding parameters directly
		profile := &EncodingProfile{
			Name:             "Custom Flags Profile",
			QualityPreset:    QualityPreset("anything"), // Would normally be invalid
			TargetVideoCodec: VideoCodecH264,            // Still validated
			TargetAudioCodec: AudioCodecAAC,             // Still validated
			OutputFlags:      "-c:v libx264 -preset fast",
		}

		err := profile.Validate()
		// Should pass because custom output flags bypass preset validation
		require.NoError(t, err)
	})
}

// TestEncodingProfile_CRFProgression tests that CRF values progress correctly.
// Lower CRF means higher quality, so ultra < high < medium < low in CRF values.
func TestEncodingProfile_CRFProgression(t *testing.T) {
	presets := []QualityPreset{QualityPresetLow, QualityPresetMedium, QualityPresetHigh, QualityPresetUltra}
	var lastCRF int = 100

	for _, preset := range presets {
		params := preset.GetEncodingParams()
		assert.Less(t, params.CRF, lastCRF, "CRF should decrease as quality increases (preset: %s)", preset)
		lastCRF = params.CRF
	}
}

// TestEncodingProfile_BitrateProgression tests that bitrate values progress correctly.
// Higher quality presets should have higher bitrates (or no limit).
func TestEncodingProfile_BitrateProgression(t *testing.T) {
	// Test that bitrates increase from low to high (ultra has no limit)
	lowProfile := &EncodingProfile{QualityPreset: QualityPresetLow}
	mediumProfile := &EncodingProfile{QualityPreset: QualityPresetMedium}
	highProfile := &EncodingProfile{QualityPreset: QualityPresetHigh}

	lowBitrate := lowProfile.GetVideoBitrate()
	mediumBitrate := mediumProfile.GetVideoBitrate()
	highBitrate := highProfile.GetVideoBitrate()

	assert.Less(t, lowBitrate, mediumBitrate, "Low bitrate should be less than medium")
	assert.Less(t, mediumBitrate, highBitrate, "Medium bitrate should be less than high")
}
