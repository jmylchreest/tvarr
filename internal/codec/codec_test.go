package codec

import (
	"testing"
)

func TestParseVideo(t *testing.T) {
	tests := []struct {
		input    string
		expected Video
		ok       bool
	}{
		// Canonical names
		{"h264", VideoH264, true},
		{"h265", VideoH265, true},
		{"vp9", VideoVP9, true},
		{"av1", VideoAV1, true},
		// Aliases
		{"hevc", VideoH265, true},
		{"avc", VideoH264, true},
		{"avc1", VideoH264, true},
		{"hev1", VideoH265, true},
		{"hvc1", VideoH265, true},
		// Encoder names
		{"libx264", VideoH264, true},
		{"h264_nvenc", VideoH264, true},
		{"h264_qsv", VideoH264, true},
		{"h264_vaapi", VideoH264, true},
		{"libx265", VideoH265, true},
		{"hevc_nvenc", VideoH265, true},
		{"hevc_qsv", VideoH265, true},
		{"libvpx-vp9", VideoVP9, true},
		{"vp9_vaapi", VideoVP9, true},
		{"libaom-av1", VideoAV1, true},
		{"av1_nvenc", VideoAV1, true},
		// Case insensitive
		{"H264", VideoH264, true},
		{"HEVC", VideoH265, true},
		{"H264_NVENC", VideoH264, true},
		// Invalid
		{"", "", false},
		{"invalid", "", false},
		{"xyz123", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseVideo(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseVideo(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.expected {
				t.Errorf("ParseVideo(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseAudio(t *testing.T) {
	tests := []struct {
		input    string
		expected Audio
		ok       bool
	}{
		// Canonical names
		{"aac", AudioAAC, true},
		{"mp3", AudioMP3, true},
		{"ac3", AudioAC3, true},
		{"eac3", AudioEAC3, true},
		{"opus", AudioOpus, true},
		// Aliases
		{"mp4a", AudioAAC, true},
		{"mp3float", AudioMP3, true},
		{"ac-3", AudioAC3, true},
		{"a52", AudioAC3, true},
		{"ec-3", AudioEAC3, true},
		// Encoder names
		{"libfdk_aac", AudioAAC, true},
		{"libmp3lame", AudioMP3, true},
		{"libopus", AudioOpus, true},
		{"libvorbis", AudioVorbis, true},
		// Case insensitive
		{"AAC", AudioAAC, true},
		{"MP3", AudioMP3, true},
		// Invalid
		{"", "", false},
		{"invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseAudio(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseAudio(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.expected {
				t.Errorf("ParseAudio(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Video codecs
		{"libx264", "h264"},
		{"h264_nvenc", "h264"},
		{"hevc", "h265"},
		{"libx265", "h265"},
		{"hevc_nvenc", "h265"},
		{"libvpx-vp9", "vp9"},
		{"libaom-av1", "av1"},
		// Audio codecs
		{"libfdk_aac", "aac"},
		{"libmp3lame", "mp3"},
		{"ac-3", "ac3"},
		{"ec-3", "eac3"},
		{"libopus", "opus"},
		// Already canonical
		{"h264", "h264"},
		{"h265", "h265"},
		{"aac", "aac"},
		// Unknown - return as-is
		{"unknown", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsEncoder(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Software encoders (lib* prefix)
		{"libx264", true},
		{"libx265", true},
		{"libvpx-vp9", true},
		{"libaom-av1", true},
		{"libfdk_aac", true},
		{"libmp3lame", true},
		{"libopus", true},
		// Hardware encoders
		{"h264_nvenc", true},
		{"hevc_nvenc", true},
		{"h264_qsv", true},
		{"hevc_qsv", true},
		{"h264_vaapi", true},
		{"hevc_vaapi", true},
		{"h264_videotoolbox", true},
		{"hevc_videotoolbox", true},
		{"h264_amf", true},
		{"h264_v4l2m2m", true},
		// Codec names (not encoders)
		{"h264", false},
		{"h265", false},
		{"hevc", false},
		{"aac", false},
		{"mp3", false},
		{"vp9", false},
		{"av1", false},
		// Edge cases
		{"", false},
		{"copy", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsEncoder(tt.input)
			if got != tt.expected {
				t.Errorf("IsEncoder(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetVideoEncoder(t *testing.T) {
	tests := []struct {
		codec    Video
		hwaccel  HWAccel
		expected string
	}{
		// H.264
		{VideoH264, HWAccelNone, "libx264"},
		{VideoH264, HWAccelAuto, "libx264"},
		{VideoH264, HWAccelCUDA, "h264_nvenc"},
		{VideoH264, HWAccelQSV, "h264_qsv"},
		{VideoH264, HWAccelVAAPI, "h264_vaapi"},
		{VideoH264, HWAccelVT, "h264_videotoolbox"},
		// H.265
		{VideoH265, HWAccelNone, "libx265"},
		{VideoH265, HWAccelCUDA, "hevc_nvenc"},
		{VideoH265, HWAccelQSV, "hevc_qsv"},
		{VideoH265, HWAccelVAAPI, "hevc_vaapi"},
		// VP9
		{VideoVP9, HWAccelNone, "libvpx-vp9"},
		{VideoVP9, HWAccelQSV, "vp9_qsv"},
		{VideoVP9, HWAccelVAAPI, "vp9_vaapi"},
		{VideoVP9, HWAccelCUDA, "libvpx-vp9"}, // Fallback to software
		// AV1
		{VideoAV1, HWAccelNone, "libaom-av1"},
		{VideoAV1, HWAccelCUDA, "av1_nvenc"},
		{VideoAV1, HWAccelQSV, "av1_qsv"},
	}

	for _, tt := range tests {
		t.Run(string(tt.codec)+"_"+string(tt.hwaccel), func(t *testing.T) {
			got := GetVideoEncoder(tt.codec, tt.hwaccel)
			if got != tt.expected {
				t.Errorf("GetVideoEncoder(%v, %v) = %q, want %q", tt.codec, tt.hwaccel, got, tt.expected)
			}
		})
	}
}

func TestGetAudioEncoder(t *testing.T) {
	tests := []struct {
		codec    Audio
		expected string
	}{
		{AudioAAC, "aac"},
		{AudioMP3, "libmp3lame"},
		{AudioAC3, "ac3"},
		{AudioEAC3, "eac3"},
		{AudioOpus, "libopus"},
		{AudioVorbis, "libvorbis"},
		{AudioFLAC, "flac"},
	}

	for _, tt := range tests {
		t.Run(string(tt.codec), func(t *testing.T) {
			got := GetAudioEncoder(tt.codec)
			if got != tt.expected {
				t.Errorf("GetAudioEncoder(%v) = %q, want %q", tt.codec, got, tt.expected)
			}
		})
	}
}

func TestIsFMP4Only(t *testing.T) {
	videoTests := []struct {
		codec    Video
		expected bool
	}{
		{VideoH264, false},
		{VideoH265, false},
		{VideoVP8, true},
		{VideoVP9, true},
		{VideoAV1, true},
		{VideoMPEG2, false},
	}

	for _, tt := range videoTests {
		t.Run("video_"+string(tt.codec), func(t *testing.T) {
			got := tt.codec.IsFMP4Only()
			if got != tt.expected {
				t.Errorf("Video(%v).IsFMP4Only() = %v, want %v", tt.codec, got, tt.expected)
			}
		})
	}

	audioTests := []struct {
		codec    Audio
		expected bool
	}{
		{AudioAAC, false},
		{AudioMP3, false},
		{AudioAC3, false},
		{AudioEAC3, false},
		{AudioOpus, true},
		{AudioVorbis, true},
		{AudioFLAC, true},
	}

	for _, tt := range audioTests {
		t.Run("audio_"+string(tt.codec), func(t *testing.T) {
			got := tt.codec.IsFMP4Only()
			if got != tt.expected {
				t.Errorf("Audio(%v).IsFMP4Only() = %v, want %v", tt.codec, got, tt.expected)
			}
		})
	}
}

func TestIsDemuxable(t *testing.T) {
	videoTests := []struct {
		codec    Video
		expected bool
	}{
		{VideoH264, true},
		{VideoH265, true},
		{VideoMPEG1, true},
		{VideoMPEG2, true},
		{VideoMPEG4, true},
		{VideoVP8, false},
		{VideoVP9, false},
		{VideoAV1, false},
		{VideoVC1, false},
		{VideoProRes, false},
	}

	for _, tt := range videoTests {
		t.Run("video_"+string(tt.codec), func(t *testing.T) {
			got := tt.codec.IsDemuxable()
			if got != tt.expected {
				t.Errorf("Video(%v).IsDemuxable() = %v, want %v", tt.codec, got, tt.expected)
			}
		})
	}

	audioTests := []struct {
		codec    Audio
		expected bool
	}{
		{AudioAAC, true},
		{AudioMP3, true},
		{AudioAC3, true},
		{AudioOpus, true},
		{AudioEAC3, true}, // Now demuxable with mediacommon fork
		{AudioDTS, false},
		{AudioTrueHD, false},
		{AudioFLAC, false},
		{AudioVorbis, false},
		{AudioPCM, false},
	}

	for _, tt := range audioTests {
		t.Run("audio_"+string(tt.codec), func(t *testing.T) {
			got := tt.codec.IsDemuxable()
			if got != tt.expected {
				t.Errorf("Audio(%v).IsDemuxable() = %v, want %v", tt.codec, got, tt.expected)
			}
		})
	}
}

func TestIsVideoDemuxable(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Demuxable
		{"h264", true},
		{"h265", true},
		{"hevc", true},
		{"libx264", true}, // Encoder maps to h264
		{"h264_nvenc", true},
		{"hevc_nvenc", true},
		{"mpeg2", true},
		// Not demuxable
		{"vp9", false},
		{"av1", false},
		{"vc1", false},
		{"prores", false},
		{"libvpx-vp9", false},
		{"libaom-av1", false},
		// Unknown - defaults to true
		{"unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsVideoDemuxable(tt.input)
			if got != tt.expected {
				t.Errorf("IsVideoDemuxable(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsAudioDemuxable(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Demuxable
		{"aac", true},
		{"mp3", true},
		{"ac3", true},
		{"opus", true},
		{"libfdk_aac", true},
		{"libmp3lame", true},
		// Now demuxable with mediacommon fork
		{"eac3", true},
		{"ec-3", true}, // E-AC3 alias
		// Not demuxable
		{"dts", false},
		{"truehd", false},
		{"flac", false},
		{"vorbis", false},
		{"pcm", false},
		// Unknown - defaults to false (safer)
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsAudioDemuxable(tt.input)
			if got != tt.expected {
				t.Errorf("IsAudioDemuxable(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		// Same codec
		{"h264", "h264", true},
		{"h265", "h265", true},
		{"aac", "aac", true},
		// Aliases
		{"h265", "hevc", true},
		{"hevc", "h265", true},
		{"h264", "avc", true},
		{"ac3", "ac-3", true},
		// Encoder to codec
		{"libx264", "h264", true},
		{"h264_nvenc", "h264", true},
		{"libx265", "h265", true},
		{"hevc_nvenc", "hevc", true},
		// Different codecs
		{"h264", "h265", false},
		{"aac", "mp3", false},
		{"vp9", "av1", false},
		// Empty
		{"", "h264", false},
		{"h264", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := Match(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestMPEGTSStreamType(t *testing.T) {
	videoTests := []struct {
		codec    Video
		expected uint8
	}{
		{VideoH264, 0x1B},
		{VideoH265, 0x24},
		{VideoMPEG1, 0x01},
		{VideoMPEG2, 0x02},
		{VideoMPEG4, 0x10},
		{VideoVP9, 0}, // Not supported in MPEG-TS
		{VideoAV1, 0}, // Not supported in MPEG-TS
		{VideoVC1, 0}, // Not supported
	}

	for _, tt := range videoTests {
		t.Run("video_"+string(tt.codec), func(t *testing.T) {
			got := tt.codec.MPEGTSStreamType()
			if got != tt.expected {
				t.Errorf("Video(%v).MPEGTSStreamType() = 0x%02X, want 0x%02X", tt.codec, got, tt.expected)
			}
		})
	}

	audioTests := []struct {
		codec    Audio
		expected uint8
	}{
		{AudioAAC, 0x0F},
		{AudioMP3, 0x03},
		{AudioAC3, 0x81},
		{AudioEAC3, 0x87},
		{AudioDTS, 0x82},
		{AudioOpus, 0}, // Not supported in standard MPEG-TS
	}

	for _, tt := range audioTests {
		t.Run("audio_"+string(tt.codec), func(t *testing.T) {
			got := tt.codec.MPEGTSStreamType()
			if got != tt.expected {
				t.Errorf("Audio(%v).MPEGTSStreamType() = 0x%02X, want 0x%02X", tt.codec, got, tt.expected)
			}
		})
	}
}

func TestParseHWAccel(t *testing.T) {
	tests := []struct {
		input    string
		expected HWAccel
		ok       bool
	}{
		{"auto", HWAccelAuto, true},
		{"none", HWAccelNone, true},
		{"cuda", HWAccelCUDA, true},
		{"qsv", HWAccelQSV, true},
		{"vaapi", HWAccelVAAPI, true},
		{"videotoolbox", HWAccelVT, true},
		{"AUTO", HWAccelAuto, true}, // Case insensitive
		{"CUDA", HWAccelCUDA, true},
		{"invalid", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseHWAccel(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseHWAccel(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.expected {
				t.Errorf("ParseHWAccel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected OutputFormat
	}{
		{"mpegts", FormatMPEGTS},
		{"ts", FormatMPEGTS},
		{"hls", FormatHLS},
		{"m3u8", FormatHLS},
		{"flv", FormatFLV},
		{"mp4", FormatMP4},
		{"fmp4", FormatFMP4},
		{"cmaf", FormatFMP4},
		{"matroska", FormatMKV},
		{"mkv", FormatMKV},
		{"webm", FormatWebM},
		{"unknown", FormatUnknown},
		{"", FormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseOutputFormat(tt.input)
			if got != tt.expected {
				t.Errorf("ParseOutputFormat(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestOutputFormatRequiresAnnexB(t *testing.T) {
	tests := []struct {
		format   OutputFormat
		expected bool
	}{
		{FormatMPEGTS, true},
		{FormatHLS, true},
		{FormatFLV, false},
		{FormatMP4, false},
		{FormatFMP4, false},
		{FormatMKV, false},
		{FormatWebM, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got := tt.format.RequiresAnnexB()
			if got != tt.expected {
				t.Errorf("OutputFormat(%v).RequiresAnnexB() = %v, want %v", tt.format, got, tt.expected)
			}
		})
	}
}
