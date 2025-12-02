package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLastKnownCodec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		codec   *LastKnownCodec
		wantErr error
	}{
		{
			name:    "valid codec",
			codec:   &LastKnownCodec{StreamURL: "http://example.com/stream.m3u8"},
			wantErr: nil,
		},
		{
			name:    "missing stream URL",
			codec:   &LastKnownCodec{VideoCodec: "h264"},
			wantErr: ErrStreamURLRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.codec.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLastKnownCodec_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt *Time
		expected  bool
	}{
		{
			name:      "no expiry set",
			expiresAt: nil,
			expected:  false,
		},
		{
			name: "expired",
			expiresAt: func() *Time {
				t := Time(time.Now().Add(-time.Hour))
				return &t
			}(),
			expected: true,
		},
		{
			name: "not expired",
			expiresAt: func() *Time {
				t := Time(time.Now().Add(time.Hour))
				return &t
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &LastKnownCodec{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.expected, c.IsExpired())
		})
	}
}

func TestLastKnownCodec_IsValid(t *testing.T) {
	tests := []struct {
		name       string
		videoCodec string
		audioCodec string
		probeError string
		expected   bool
	}{
		{"valid video", "h264", "", "", true},
		{"valid audio", "", "aac", "", true},
		{"valid both", "h264", "aac", "", true},
		{"no codecs", "", "", "", false},
		{"has error", "h264", "aac", "connection failed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &LastKnownCodec{
				VideoCodec: tt.videoCodec,
				AudioCodec: tt.audioCodec,
				ProbeError: tt.probeError,
			}
			assert.Equal(t, tt.expected, c.IsValid())
		})
	}
}

func TestLastKnownCodec_SetExpiry(t *testing.T) {
	c := &LastKnownCodec{StreamURL: "http://example.com/stream"}
	c.SetExpiry(24 * time.Hour)

	assert.NotNil(t, c.ExpiresAt)
	assert.True(t, time.Time(*c.ExpiresAt).After(time.Now()))
	assert.True(t, time.Time(*c.ExpiresAt).Before(time.Now().Add(25*time.Hour)))
}

func TestLastKnownCodec_Touch(t *testing.T) {
	c := &LastKnownCodec{StreamURL: "http://example.com/stream", HitCount: 5}
	originalUpdate := c.UpdatedAt

	c.Touch()

	assert.Equal(t, int64(6), c.HitCount)
	assert.True(t, c.UpdatedAt.After(originalUpdate) || c.UpdatedAt.Equal(originalUpdate))
}

func TestLastKnownCodec_GetVideoInfo(t *testing.T) {
	t.Run("with video", func(t *testing.T) {
		c := &LastKnownCodec{
			VideoCodec:     "h264",
			VideoProfile:   "High",
			VideoLevel:     "4.1",
			VideoWidth:     1920,
			VideoHeight:    1080,
			VideoFramerate: 29.97,
			VideoBitrate:   5000000,
			VideoPixFmt:    "yuv420p",
		}

		info := c.GetVideoInfo()
		assert.NotNil(t, info)
		assert.Equal(t, "h264", info.Codec)
		assert.Equal(t, "High", info.Profile)
		assert.Equal(t, "4.1", info.Level)
		assert.Equal(t, 1920, info.Width)
		assert.Equal(t, 1080, info.Height)
		assert.Equal(t, 29.97, info.Framerate)
		assert.Equal(t, 5000000, info.Bitrate)
		assert.Equal(t, "yuv420p", info.PixFmt)
	})

	t.Run("without video", func(t *testing.T) {
		c := &LastKnownCodec{AudioCodec: "aac"}
		info := c.GetVideoInfo()
		assert.Nil(t, info)
	})
}

func TestLastKnownCodec_GetAudioInfo(t *testing.T) {
	t.Run("with audio", func(t *testing.T) {
		c := &LastKnownCodec{
			AudioCodec:      "aac",
			AudioSampleRate: 48000,
			AudioChannels:   2,
			AudioBitrate:    128000,
		}

		info := c.GetAudioInfo()
		assert.NotNil(t, info)
		assert.Equal(t, "aac", info.Codec)
		assert.Equal(t, 48000, info.SampleRate)
		assert.Equal(t, 2, info.Channels)
		assert.Equal(t, 128000, info.Bitrate)
	})

	t.Run("without audio", func(t *testing.T) {
		c := &LastKnownCodec{VideoCodec: "h264"}
		info := c.GetAudioInfo()
		assert.Nil(t, info)
	})
}

func TestLastKnownCodec_NeedsVideoTranscode(t *testing.T) {
	c := &LastKnownCodec{
		VideoCodec: "h264",
		VideoWidth: 1920,
		VideoHeight: 1080,
	}

	tests := []struct {
		name         string
		targetCodec  string
		targetWidth  int
		targetHeight int
		expected     bool
	}{
		{"no transcode needed - copy", "copy", 0, 0, false},
		{"no transcode needed - same codec", "h264", 0, 0, false},
		{"different codec", "hevc", 0, 0, true},
		{"downscale width", "", 1280, 0, true},
		{"downscale height", "", 0, 720, true},
		{"larger resolution no transcode", "", 3840, 2160, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.NeedsVideoTranscode(tt.targetCodec, tt.targetWidth, tt.targetHeight)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLastKnownCodec_NeedsAudioTranscode(t *testing.T) {
	c := &LastKnownCodec{
		AudioCodec:    "aac",
		AudioChannels: 6, // 5.1 surround
	}

	tests := []struct {
		name           string
		targetCodec    string
		targetChannels int
		expected       bool
	}{
		{"no transcode needed - copy", "copy", 0, false},
		{"no transcode needed - same codec", "aac", 0, false},
		{"different codec", "mp3", 0, true},
		{"downmix channels", "", 2, true},
		{"more channels no transcode", "", 8, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.NeedsAudioTranscode(tt.targetCodec, tt.targetChannels)
			assert.Equal(t, tt.expected, result)
		})
	}
}
