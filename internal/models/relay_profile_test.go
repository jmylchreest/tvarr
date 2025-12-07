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
