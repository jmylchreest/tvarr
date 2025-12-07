package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRelayProfile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		profile *RelayProfile
		wantErr error
	}{
		{
			name:    "valid profile",
			profile: &RelayProfile{Name: "default", VideoCodec: VideoCodecCopy, AudioCodec: AudioCodecCopy},
			wantErr: nil,
		},
		{
			name:    "missing name",
			profile: &RelayProfile{VideoCodec: VideoCodecH264},
			wantErr: ErrRelayProfileNameRequired,
		},
		{
			name:    "negative video bitrate",
			profile: &RelayProfile{Name: "test", VideoBitrate: -100},
			wantErr: ErrRelayProfileInvalidBitrate,
		},
		{
			name:    "negative audio bitrate",
			profile: &RelayProfile{Name: "test", AudioBitrate: -100},
			wantErr: ErrRelayProfileInvalidBitrate,
		},
		{
			name:    "zero bitrate is valid",
			profile: &RelayProfile{Name: "test", VideoBitrate: 0, AudioBitrate: 0},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRelayProfile_IsPassthrough(t *testing.T) {
	tests := []struct {
		name     string
		video    VideoCodec
		audio    AudioCodec
		expected bool
	}{
		{"both copy", VideoCodecCopy, AudioCodecCopy, true},
		{"video transcode", VideoCodecH264, AudioCodecCopy, false},
		{"audio transcode", VideoCodecCopy, AudioCodecAAC, false},
		{"both transcode", VideoCodecH264, AudioCodecAAC, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RelayProfile{VideoCodec: tt.video, AudioCodec: tt.audio}
			assert.Equal(t, tt.expected, p.IsPassthrough())
		})
	}
}

func TestRelayProfile_UsesHardwareAccel(t *testing.T) {
	tests := []struct {
		name     string
		hwAccel  HWAccelType
		expected bool
	}{
		{"none", HWAccelNone, false},
		{"empty", "", false},
		{"cuda", HWAccelNVDEC, true},
		{"qsv", HWAccelQSV, true},
		{"vaapi", HWAccelVAAPI, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RelayProfile{HWAccel: tt.hwAccel}
			assert.Equal(t, tt.expected, p.UsesHardwareAccel())
		})
	}
}

func TestRelayProfile_NeedsTranscode(t *testing.T) {
	tests := []struct {
		name     string
		video    VideoCodec
		audio    AudioCodec
		expected bool
	}{
		{"passthrough", VideoCodecCopy, AudioCodecCopy, false},
		{"video only", VideoCodecH264, AudioCodecCopy, true},
		{"audio only", VideoCodecCopy, AudioCodecAAC, true},
		{"both", VideoCodecH264, AudioCodecAAC, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RelayProfile{VideoCodec: tt.video, AudioCodec: tt.audio}
			assert.Equal(t, tt.expected, p.NeedsTranscode())
		})
	}
}

func TestRelayProfile_Clone(t *testing.T) {
	original := &RelayProfile{
		Name:         "original",
		Description:  "Test profile",
		VideoCodec:   VideoCodecH264,
		AudioCodec:   AudioCodecAAC,
		VideoBitrate: 5000,
		IsDefault:    true,
	}
	original.ID = NewULID()

	clone := original.Clone()
	clone.Name = "cloned"
	clone.Description = "Cloned profile"

	assert.NotEqual(t, original.ID, clone.ID)
	assert.Equal(t, "cloned", clone.Name)
	assert.Equal(t, "Cloned profile", clone.Description) // Clone clears description, must be set by caller
	assert.Equal(t, original.VideoCodec, clone.VideoCodec)
	assert.Equal(t, original.AudioCodec, clone.AudioCodec)
	assert.Equal(t, original.VideoBitrate, clone.VideoBitrate)
	assert.False(t, clone.IsDefault) // IsDefault should be false on clone
}

func TestVideoCodec_Constants(t *testing.T) {
	// Verify codec strings match FFmpeg codec names
	assert.Equal(t, "copy", string(VideoCodecCopy))
	assert.Equal(t, "libx264", string(VideoCodecH264))
	assert.Equal(t, "libx265", string(VideoCodecH265))
	assert.Equal(t, "h264_nvenc", string(VideoCodecNVENC))
	assert.Equal(t, "h264_qsv", string(VideoCodecQSV))
	assert.Equal(t, "h264_vaapi", string(VideoCodecVAAPI))
}

func TestAudioCodec_Constants(t *testing.T) {
	// Verify codec strings match FFmpeg codec names
	assert.Equal(t, "copy", string(AudioCodecCopy))
	assert.Equal(t, "aac", string(AudioCodecAAC))
	assert.Equal(t, "libmp3lame", string(AudioCodecMP3))
	assert.Equal(t, "libopus", string(AudioCodecOpus))
}

func TestOutputFormat_Constants(t *testing.T) {
	assert.Equal(t, "mpegts", string(OutputFormatMPEGTS))
	assert.Equal(t, "hls", string(OutputFormatHLS))
	assert.Equal(t, "flv", string(OutputFormatFLV))
}

func TestContainerFormat_Constants(t *testing.T) {
	assert.Equal(t, "auto", string(ContainerFormatAuto))
	assert.Equal(t, "fmp4", string(ContainerFormatFMP4))
	assert.Equal(t, "mpegts", string(ContainerFormatMPEGTS))
}

func TestIsFMP4OnlyVideoCodec(t *testing.T) {
	tests := []struct {
		codec    VideoCodec
		expected bool
	}{
		{VideoCodecCopy, false},
		{VideoCodecH264, false},
		{VideoCodecH265, false},
		{VideoCodecNVENC, false},
		{VideoCodecNVENCH5, false},
		{VideoCodecQSV, false},
		{VideoCodecQSVH5, false},
		{VideoCodecVAAPI, false},
		{VideoCodecVAAPIH5, false},
		// fMP4-only codecs
		{VideoCodecVP9, true},
		{VideoCodecAV1, true},
		{VideoCodecAV1NVENC, true},
		{VideoCodecAV1QSV, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.codec), func(t *testing.T) {
			assert.Equal(t, tt.expected, IsFMP4OnlyVideoCodec(tt.codec))
		})
	}
}

func TestIsFMP4OnlyAudioCodec(t *testing.T) {
	tests := []struct {
		codec    AudioCodec
		expected bool
	}{
		{AudioCodecCopy, false},
		{AudioCodecAAC, false},
		{AudioCodecMP3, false},
		{AudioCodecAC3, false},
		{AudioCodecEAC3, false},
		// fMP4-only codecs
		{AudioCodecOpus, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.codec), func(t *testing.T) {
			assert.Equal(t, tt.expected, IsFMP4OnlyAudioCodec(tt.codec))
		})
	}
}

func TestRelayProfile_RequiresFMP4(t *testing.T) {
	tests := []struct {
		name     string
		video    VideoCodec
		audio    AudioCodec
		expected bool
	}{
		{"copy/copy - no requirement", VideoCodecCopy, AudioCodecCopy, false},
		{"h264/aac - no requirement", VideoCodecH264, AudioCodecAAC, false},
		{"h265/aac - no requirement", VideoCodecH265, AudioCodecAAC, false},
		{"vp9/aac - requires fMP4 (video)", VideoCodecVP9, AudioCodecAAC, true},
		{"av1/aac - requires fMP4 (video)", VideoCodecAV1, AudioCodecAAC, true},
		{"av1_nvenc/aac - requires fMP4 (video)", VideoCodecAV1NVENC, AudioCodecAAC, true},
		{"av1_qsv/aac - requires fMP4 (video)", VideoCodecAV1QSV, AudioCodecAAC, true},
		{"h264/opus - requires fMP4 (audio)", VideoCodecH264, AudioCodecOpus, true},
		{"vp9/opus - requires fMP4 (both)", VideoCodecVP9, AudioCodecOpus, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RelayProfile{VideoCodec: tt.video, AudioCodec: tt.audio}
			assert.Equal(t, tt.expected, p.RequiresFMP4())
		})
	}
}

func TestRelayProfile_ValidateCodecFormat(t *testing.T) {
	tests := []struct {
		name            string
		containerFormat ContainerFormat
		video           VideoCodec
		audio           AudioCodec
		wantErr         error
	}{
		// Valid combinations
		{"auto with any codec", ContainerFormatAuto, VideoCodecVP9, AudioCodecOpus, nil},
		{"fmp4 with VP9", ContainerFormatFMP4, VideoCodecVP9, AudioCodecAAC, nil},
		{"fmp4 with AV1", ContainerFormatFMP4, VideoCodecAV1, AudioCodecAAC, nil},
		{"fmp4 with Opus", ContainerFormatFMP4, VideoCodecH264, AudioCodecOpus, nil},
		{"mpegts with h264/aac", ContainerFormatMPEGTS, VideoCodecH264, AudioCodecAAC, nil},
		{"mpegts with copy/copy", ContainerFormatMPEGTS, VideoCodecCopy, AudioCodecCopy, nil},

		// Invalid combinations
		{"mpegts with VP9", ContainerFormatMPEGTS, VideoCodecVP9, AudioCodecAAC, ErrRelayProfileCodecRequiresFMP4},
		{"mpegts with AV1", ContainerFormatMPEGTS, VideoCodecAV1, AudioCodecAAC, ErrRelayProfileCodecRequiresFMP4},
		{"mpegts with AV1 NVENC", ContainerFormatMPEGTS, VideoCodecAV1NVENC, AudioCodecAAC, ErrRelayProfileCodecRequiresFMP4},
		{"mpegts with AV1 QSV", ContainerFormatMPEGTS, VideoCodecAV1QSV, AudioCodecAAC, ErrRelayProfileCodecRequiresFMP4},
		{"mpegts with Opus", ContainerFormatMPEGTS, VideoCodecH264, AudioCodecOpus, ErrRelayProfileCodecRequiresFMP4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RelayProfile{
				Name:            "test",
				ContainerFormat: tt.containerFormat,
				VideoCodec:      tt.video,
				AudioCodec:      tt.audio,
			}
			err := p.ValidateCodecFormat()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRelayProfile_DetermineContainer(t *testing.T) {
	tests := []struct {
		name            string
		containerFormat ContainerFormat
		video           VideoCodec
		audio           AudioCodec
		expected        ContainerFormat
	}{
		// Explicit container format
		{"explicit fMP4", ContainerFormatFMP4, VideoCodecH264, AudioCodecAAC, ContainerFormatFMP4},
		{"explicit MPEG-TS", ContainerFormatMPEGTS, VideoCodecH264, AudioCodecAAC, ContainerFormatMPEGTS},

		// Explicit MPEG-TS overridden by codec requirements
		{"explicit MPEG-TS with VP9 - forced fMP4", ContainerFormatMPEGTS, VideoCodecVP9, AudioCodecAAC, ContainerFormatFMP4},
		{"explicit MPEG-TS with AV1 - forced fMP4", ContainerFormatMPEGTS, VideoCodecAV1, AudioCodecAAC, ContainerFormatFMP4},
		{"explicit MPEG-TS with Opus - forced fMP4", ContainerFormatMPEGTS, VideoCodecH264, AudioCodecOpus, ContainerFormatFMP4},

		// Auto mode with fMP4-requiring codecs
		{"auto with VP9 - fMP4", ContainerFormatAuto, VideoCodecVP9, AudioCodecAAC, ContainerFormatFMP4},
		{"auto with AV1 - fMP4", ContainerFormatAuto, VideoCodecAV1, AudioCodecAAC, ContainerFormatFMP4},
		{"auto with Opus - fMP4", ContainerFormatAuto, VideoCodecH264, AudioCodecOpus, ContainerFormatFMP4},

		// Auto mode with passthrough - MPEG-TS for compatibility
		{"auto with copy/copy - MPEG-TS", ContainerFormatAuto, VideoCodecCopy, AudioCodecCopy, ContainerFormatMPEGTS},

		// Auto mode with standard codecs - fMP4 (modern default)
		{"auto with h264/aac - fMP4", ContainerFormatAuto, VideoCodecH264, AudioCodecAAC, ContainerFormatFMP4},
		{"auto with h265/aac - fMP4", ContainerFormatAuto, VideoCodecH265, AudioCodecAAC, ContainerFormatFMP4},

		// Empty container format treated as auto
		{"empty with h264/aac - fMP4", "", VideoCodecH264, AudioCodecAAC, ContainerFormatFMP4},
		{"empty with copy/copy - MPEG-TS", "", VideoCodecCopy, AudioCodecCopy, ContainerFormatMPEGTS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RelayProfile{
				ContainerFormat: tt.containerFormat,
				VideoCodec:      tt.video,
				AudioCodec:      tt.audio,
			}
			assert.Equal(t, tt.expected, p.DetermineContainer())
		})
	}
}
