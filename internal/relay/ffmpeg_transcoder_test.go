package relay

import (
	"testing"

	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T066: Integration test for quality preset affecting transcoded output.
// This tests that CreateTranscoderFromProfile correctly uses the quality preset
// parameters from an EncodingProfile to configure the FFmpeg transcoder.
func TestCreateTranscoderFromProfile_QualityPresets(t *testing.T) {
	// Create a shared buffer for testing (we don't need actual samples)
	buffer := NewSharedESBuffer("test-channel", "test-proxy", SharedESBufferConfig{})

	// Mock FFmpeg binary info
	ffmpegBin := &ffmpeg.BinaryInfo{
		FFmpegPath: "/usr/bin/ffmpeg",
	}

	sourceVariant := CodecVariant("h264/aac")

	t.Run("low quality preset applies correct parameters", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "Low Quality",
			QualityPreset:    models.QualityPresetLow,
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			HWAccel:          models.HWAccelNone,
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-low",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil, // Logger
		)

		require.NoError(t, err)
		require.NotNil(t, transcoder)

		// Verify config matches low preset expectations
		assert.Equal(t, 2000, transcoder.config.VideoBitrate, "Low preset should have 2000kbps video bitrate")
		assert.Equal(t, 128, transcoder.config.AudioBitrate, "Low preset should have 128kbps audio bitrate")
		assert.Equal(t, "fast", transcoder.config.VideoPreset, "Low preset should use 'fast' video preset")
	})

	t.Run("medium quality preset applies correct parameters", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "Medium Quality",
			QualityPreset:    models.QualityPresetMedium,
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			HWAccel:          models.HWAccelNone,
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-medium",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil,
		)

		require.NoError(t, err)
		require.NotNil(t, transcoder)

		// Verify config matches medium preset expectations
		assert.Equal(t, 5000, transcoder.config.VideoBitrate, "Medium preset should have 5000kbps video bitrate")
		assert.Equal(t, 192, transcoder.config.AudioBitrate, "Medium preset should have 192kbps audio bitrate")
		assert.Equal(t, "medium", transcoder.config.VideoPreset, "Medium preset should use 'medium' video preset")
	})

	t.Run("high quality preset applies correct parameters", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "High Quality",
			QualityPreset:    models.QualityPresetHigh,
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecOpus,
			HWAccel:          models.HWAccelNone,
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-high",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil,
		)

		require.NoError(t, err)
		require.NotNil(t, transcoder)

		// Verify config matches high preset expectations
		assert.Equal(t, 10000, transcoder.config.VideoBitrate, "High preset should have 10000kbps video bitrate")
		assert.Equal(t, 256, transcoder.config.AudioBitrate, "High preset should have 256kbps audio bitrate")
		assert.Equal(t, "medium", transcoder.config.VideoPreset, "High preset should use 'medium' video preset")

		// Also verify codec encoders are set correctly
		assert.Equal(t, "libx265", transcoder.config.VideoCodec, "H.265 profile should use libx265 encoder")
		assert.Equal(t, "libopus", transcoder.config.AudioCodec, "Opus profile should use libopus encoder")
	})

	t.Run("ultra quality preset applies correct parameters", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "Ultra Quality",
			QualityPreset:    models.QualityPresetUltra,
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			HWAccel:          models.HWAccelNone,
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-ultra",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil,
		)

		require.NoError(t, err)
		require.NotNil(t, transcoder)

		// Verify config matches ultra preset expectations
		assert.Equal(t, 0, transcoder.config.VideoBitrate, "Ultra preset should have no bitrate cap (0)")
		assert.Equal(t, 320, transcoder.config.AudioBitrate, "Ultra preset should have 320kbps audio bitrate")
		assert.Equal(t, "slow", transcoder.config.VideoPreset, "Ultra preset should use 'slow' video preset")
	})

	t.Run("different codecs use correct FFmpeg encoders", func(t *testing.T) {
		testCases := []struct {
			name         string
			videoCodec   models.VideoCodec
			audioCodec   models.AudioCodec
			wantVideoEnc string
			wantAudioEnc string
		}{
			{
				name:         "H.264 + AAC",
				videoCodec:   models.VideoCodecH264,
				audioCodec:   models.AudioCodecAAC,
				wantVideoEnc: "libx264",
				wantAudioEnc: "aac",
			},
			{
				name:         "H.265 + AAC",
				videoCodec:   models.VideoCodecH265,
				audioCodec:   models.AudioCodecAAC,
				wantVideoEnc: "libx265",
				wantAudioEnc: "aac",
			},
			{
				name:         "VP9 + Opus",
				videoCodec:   models.VideoCodecVP9,
				audioCodec:   models.AudioCodecOpus,
				wantVideoEnc: "libvpx-vp9",
				wantAudioEnc: "libopus",
			},
			{
				name:         "AV1 + Opus",
				videoCodec:   models.VideoCodecAV1,
				audioCodec:   models.AudioCodecOpus,
				wantVideoEnc: "libaom-av1",
				wantAudioEnc: "libopus",
			},
			{
				name:         "H.264 + AC3",
				videoCodec:   models.VideoCodecH264,
				audioCodec:   models.AudioCodecAC3,
				wantVideoEnc: "libx264",
				wantAudioEnc: "ac3",
			},
			{
				name:         "H.264 + EAC3",
				videoCodec:   models.VideoCodecH264,
				audioCodec:   models.AudioCodecEAC3,
				wantVideoEnc: "libx264",
				wantAudioEnc: "eac3",
			},
			{
				name:         "H.264 + MP3",
				videoCodec:   models.VideoCodecH264,
				audioCodec:   models.AudioCodecMP3,
				wantVideoEnc: "libx264",
				wantAudioEnc: "libmp3lame",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				profile := &models.EncodingProfile{
					Name:             tc.name,
					QualityPreset:    models.QualityPresetMedium,
					TargetVideoCodec: tc.videoCodec,
					TargetAudioCodec: tc.audioCodec,
					HWAccel:          models.HWAccelNone,
				}

				transcoder, err := CreateTranscoderFromProfile(
					"test-codec",
					buffer,
					sourceVariant,
					profile,
					ffmpegBin,
					nil,
				)

				require.NoError(t, err)
				require.NotNil(t, transcoder)

				assert.Equal(t, tc.wantVideoEnc, transcoder.config.VideoCodec, "Video encoder for %s", tc.name)
				assert.Equal(t, tc.wantAudioEnc, transcoder.config.AudioCodec, "Audio encoder for %s", tc.name)
			})
		}
	})

	t.Run("target variant is created from profile codecs", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "Test Variant",
			QualityPreset:    models.QualityPresetMedium,
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecOpus,
			HWAccel:          models.HWAccelNone,
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-variant",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil,
		)

		require.NoError(t, err)
		require.NotNil(t, transcoder)

		// Verify target variant is created correctly
		assert.Equal(t, CodecVariant("h265/opus"), transcoder.config.TargetVariant)
		assert.Equal(t, "h265", transcoder.config.TargetVariant.VideoCodec())
		assert.Equal(t, "opus", transcoder.config.TargetVariant.AudioCodec())
	})

	t.Run("nil profile returns error", func(t *testing.T) {
		transcoder, err := CreateTranscoderFromProfile(
			"test-nil",
			buffer,
			sourceVariant,
			nil, // nil profile
			ffmpegBin,
			nil,
		)

		require.Error(t, err)
		assert.Nil(t, transcoder)
		assert.Contains(t, err.Error(), "profile is required")
	})

	t.Run("hardware acceleration is passed through", func(t *testing.T) {
		hwAccelTypes := []models.HWAccelType{
			models.HWAccelNone,
			models.HWAccelVAAPI,
			models.HWAccelNVDEC,
			models.HWAccelQSV,
		}

		for _, hwAccel := range hwAccelTypes {
			t.Run(string(hwAccel), func(t *testing.T) {
				profile := &models.EncodingProfile{
					Name:             "HW Accel Test",
					QualityPreset:    models.QualityPresetMedium,
					TargetVideoCodec: models.VideoCodecH264,
					TargetAudioCodec: models.AudioCodecAAC,
					HWAccel:          hwAccel,
				}

				transcoder, err := CreateTranscoderFromProfile(
					"test-hwaccel",
					buffer,
					sourceVariant,
					profile,
					ffmpegBin,
					nil,
				)

				require.NoError(t, err)
				require.NotNil(t, transcoder)

				assert.Equal(t, string(hwAccel), transcoder.config.HWAccel, "HW accel should be passed through")
			})
		}
	})
}

// TestTranscoderStats verifies the Stats() method returns correct values.
func TestTranscoderStats(t *testing.T) {
	buffer := NewSharedESBuffer("test-channel", "test-proxy", SharedESBufferConfig{})
	ffmpegBin := &ffmpeg.BinaryInfo{FFmpegPath: "/usr/bin/ffmpeg"}
	sourceVariant := CodecVariant("h264/aac")

	t.Run("stats with software encoding", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "Test Software",
			QualityPreset:    models.QualityPresetMedium,
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecAAC,
			HWAccel:          models.HWAccelNone, // Software encoding
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-stats-sw",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil,
		)

		require.NoError(t, err)

		stats := transcoder.Stats()

		// Verify stats reflect the configuration
		assert.Equal(t, "test-stats-sw", stats.ID)
		assert.Equal(t, sourceVariant, stats.SourceVariant)
		assert.Equal(t, CodecVariant("h265/aac"), stats.TargetVariant)
		assert.Equal(t, "h265", stats.VideoCodec)
		assert.Equal(t, "aac", stats.AudioCodec)
		assert.Equal(t, "libx265", stats.VideoEncoder, "Software encoding should use libx265")
		assert.Equal(t, "aac", stats.AudioEncoder)
		assert.Equal(t, "none", stats.HWAccel)
	})

	t.Run("stats with VAAPI hardware encoding", func(t *testing.T) {
		profile := &models.EncodingProfile{
			Name:             "Test VAAPI",
			QualityPreset:    models.QualityPresetMedium,
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecAAC,
			HWAccel:          models.HWAccelVAAPI, // Hardware encoding
		}

		transcoder, err := CreateTranscoderFromProfile(
			"test-stats-hw",
			buffer,
			sourceVariant,
			profile,
			ffmpegBin,
			nil,
		)

		require.NoError(t, err)

		stats := transcoder.Stats()

		// Verify stats reflect the configuration
		assert.Equal(t, "test-stats-hw", stats.ID)
		assert.Equal(t, CodecVariant("h265/aac"), stats.TargetVariant)
		assert.Equal(t, "h265", stats.VideoCodec)
		assert.Equal(t, "aac", stats.AudioCodec)
		assert.Equal(t, "hevc_vaapi", stats.VideoEncoder, "VAAPI encoding should use hevc_vaapi")
		assert.Equal(t, "aac", stats.AudioEncoder)
		assert.Equal(t, "vaapi", stats.HWAccel)
	})
}

// TestNewCodecVariant tests the codec variant creation function.
func TestNewCodecVariant(t *testing.T) {
	t.Run("creates variant from codec names", func(t *testing.T) {
		variant := NewCodecVariant("h264", "aac")
		assert.Equal(t, CodecVariant("h264/aac"), variant)
		assert.Equal(t, "h264", variant.VideoCodec())
		assert.Equal(t, "aac", variant.AudioCodec())
	})

	t.Run("handles empty codec names", func(t *testing.T) {
		variant := NewCodecVariant("", "")
		assert.Equal(t, CodecVariant("copy/copy"), variant)
	})

	t.Run("handles copy values", func(t *testing.T) {
		variant := NewCodecVariant("copy", "copy")
		assert.Equal(t, CodecVariant("copy/copy"), variant)
	})
}
