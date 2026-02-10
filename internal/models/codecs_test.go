package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVideoCodec_GetFFmpegEncoder(t *testing.T) {
	tests := []struct {
		name     string
		codec    VideoCodec
		hwaccel  HWAccelType
		expected string
	}{
		{"h264 software", VideoCodecH264, HWAccelNone, "libx264"},
		{"h264 auto", VideoCodecH264, HWAccelAuto, "libx264"},
		{"h264 nvidia", VideoCodecH264, HWAccelNVDEC, "h264_nvenc"},
		{"h264 qsv", VideoCodecH264, HWAccelQSV, "h264_qsv"},
		{"h264 vaapi", VideoCodecH264, HWAccelVAAPI, "h264_vaapi"},
		{"h264 videotoolbox", VideoCodecH264, HWAccelVT, "h264_videotoolbox"},
		{"h265 software", VideoCodecH265, HWAccelNone, "libx265"},
		{"h265 nvidia", VideoCodecH265, HWAccelNVDEC, "hevc_nvenc"},
		{"h265 qsv", VideoCodecH265, HWAccelQSV, "hevc_qsv"},
		{"h265 vaapi", VideoCodecH265, HWAccelVAAPI, "hevc_vaapi"},
		{"vp9 software", VideoCodecVP9, HWAccelNone, "libvpx-vp9"},
		{"vp9 vaapi", VideoCodecVP9, HWAccelVAAPI, "vp9_vaapi"},
		{"av1 software", VideoCodecAV1, HWAccelNone, "libaom-av1"},
		{"av1 nvidia", VideoCodecAV1, HWAccelNVDEC, "av1_nvenc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.codec.GetFFmpegEncoder(tt.hwaccel)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVideoCodec_GetFFmpegEncoder_UnknownCodec(t *testing.T) {
	c := VideoCodec("unknown_codec")
	// Unknown codecs should return the codec string as-is
	assert.Equal(t, "unknown_codec", c.GetFFmpegEncoder(HWAccelNone))
}

func TestVideoCodec_IsFMP4Only(t *testing.T) {
	tests := []struct {
		name     string
		codec    VideoCodec
		expected bool
	}{
		{"h264 not fMP4 only", VideoCodecH264, false},
		{"h265 not fMP4 only", VideoCodecH265, false},
		{"vp9 is fMP4 only", VideoCodecVP9, true},
		{"av1 is fMP4 only", VideoCodecAV1, true},
		{"unknown is not fMP4 only", VideoCodec("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.codec.IsFMP4Only())
		})
	}
}

func TestAudioCodec_GetFFmpegEncoder(t *testing.T) {
	tests := []struct {
		name     string
		codec    AudioCodec
		expected string
	}{
		{"aac", AudioCodecAAC, "aac"},
		{"mp3", AudioCodecMP3, "libmp3lame"},
		{"ac3", AudioCodecAC3, "ac3"},
		{"eac3", AudioCodecEAC3, "eac3"},
		{"opus", AudioCodecOpus, "libopus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.codec.GetFFmpegEncoder())
		})
	}
}

func TestAudioCodec_GetFFmpegEncoder_UnknownCodec(t *testing.T) {
	c := AudioCodec("unknown_codec")
	assert.Equal(t, "unknown_codec", c.GetFFmpegEncoder())
}

func TestAudioCodec_IsFMP4Only(t *testing.T) {
	tests := []struct {
		name     string
		codec    AudioCodec
		expected bool
	}{
		{"aac not fMP4 only", AudioCodecAAC, false},
		{"mp3 not fMP4 only", AudioCodecMP3, false},
		{"ac3 not fMP4 only", AudioCodecAC3, false},
		{"eac3 not fMP4 only", AudioCodecEAC3, false},
		{"opus is fMP4 only", AudioCodecOpus, true},
		{"unknown is not fMP4 only", AudioCodec("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.codec.IsFMP4Only())
		})
	}
}

func TestParseVideoCodec(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected VideoCodec
		expectOK bool
	}{
		{"h264", "h264", VideoCodecH264, true},
		{"h265", "h265", VideoCodecH265, true},
		{"hevc alias", "hevc", VideoCodecH265, true},
		{"vp9", "vp9", VideoCodecVP9, true},
		{"av1", "av1", VideoCodecAV1, true},
		{"case insensitive", "H264", VideoCodecH264, true},
		{"avc alias", "avc", VideoCodecH264, true},
		{"invalid", "invalid_codec", VideoCodec(""), false},
		{"empty string", "", VideoCodec(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec, ok := ParseVideoCodec(tt.input)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expected, codec)
			}
		})
	}
}

func TestParseAudioCodec(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected AudioCodec
		expectOK bool
	}{
		{"aac", "aac", AudioCodecAAC, true},
		{"mp3", "mp3", AudioCodecMP3, true},
		{"ac3", "ac3", AudioCodecAC3, true},
		{"eac3", "eac3", AudioCodecEAC3, true},
		{"opus", "opus", AudioCodecOpus, true},
		{"case insensitive", "AAC", AudioCodecAAC, true},
		{"invalid", "invalid_codec", AudioCodec(""), false},
		{"empty string", "", AudioCodec(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec, ok := ParseAudioCodec(tt.input)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expected, codec)
			}
		})
	}
}

func TestParsePreferredFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		expectOK bool
	}{
		{"hls-fmp4", "hls-fmp4", "hls-fmp4", true},
		{"hls-ts", "hls-ts", "hls-ts", true},
		{"dash", "dash", "dash", true},
		{"mpegts", "mpegts", "mpegts", true},
		{"fmp4 alias", "fmp4", "hls-fmp4", true},
		{"ts alias", "ts", "hls-ts", true},
		{"case insensitive", "HLS-FMP4", "hls-fmp4", true},
		{"with whitespace", "  dash  ", "dash", true},
		{"invalid", "invalid_format", "", false},
		{"empty string", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, ok := ParsePreferredFormat(tt.input)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expected, format)
			}
		})
	}
}

func TestValidVideoCodecs_Map(t *testing.T) {
	// Ensure the map contains expected entries
	assert.Equal(t, VideoCodecH264, ValidVideoCodecs["h264"])
	assert.Equal(t, VideoCodecH265, ValidVideoCodecs["h265"])
	assert.Equal(t, VideoCodecH265, ValidVideoCodecs["hevc"], "hevc should alias to h265")
	assert.Equal(t, VideoCodecVP9, ValidVideoCodecs["vp9"])
	assert.Equal(t, VideoCodecAV1, ValidVideoCodecs["av1"])
}

func TestValidAudioCodecs_Map(t *testing.T) {
	assert.Equal(t, AudioCodecAAC, ValidAudioCodecs["aac"])
	assert.Equal(t, AudioCodecMP3, ValidAudioCodecs["mp3"])
	assert.Equal(t, AudioCodecAC3, ValidAudioCodecs["ac3"])
	assert.Equal(t, AudioCodecEAC3, ValidAudioCodecs["eac3"])
	assert.Equal(t, AudioCodecOpus, ValidAudioCodecs["opus"])
}
