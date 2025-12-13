package codec

import (
	"testing"
)

func TestMediacommonCodecDetection(t *testing.T) {
	// Test that EAC3 is now detected as supported (with our fork)
	tests := []struct {
		name     string
		codec    string
		expected bool
	}{
		// Video codecs - should be supported
		{"H264", "h264", true},
		{"H265", "h265", true},
		{"MPEG1", "mpeg1", true},
		{"MPEG4", "mpeg4", true},

		// Audio codecs - should be supported
		{"AAC", "aac", true},
		{"AC3", "ac3", true},
		{"EAC3", "eac3", true}, // Our fork adds this!
		{"MP3", "mp3", true},
		{"Opus", "opus", true},

		// Unsupported codecs
		{"DTS", "dts", false},
		{"TrueHD", "truehd", false},
		{"Vorbis", "vorbis", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMediacommonCodecSupported(tt.codec)
			if got != tt.expected {
				t.Errorf("IsMediacommonCodecSupported(%q) = %v, want %v", tt.codec, got, tt.expected)
			}
		})
	}
}

func TestMediacommonSupportedCodecsStruct(t *testing.T) {
	// Verify the detection struct is populated correctly
	t.Logf("Detected codec support:")
	t.Logf("  H264:  %v", mediacommonSupportedCodecs.H264)
	t.Logf("  H265:  %v", mediacommonSupportedCodecs.H265)
	t.Logf("  MPEG1: %v", mediacommonSupportedCodecs.MPEG1)
	t.Logf("  MPEG4: %v", mediacommonSupportedCodecs.MPEG4)
	t.Logf("  AAC:   %v", mediacommonSupportedCodecs.AAC)
	t.Logf("  AC3:   %v", mediacommonSupportedCodecs.AC3)
	t.Logf("  EAC3:  %v", mediacommonSupportedCodecs.EAC3)
	t.Logf("  MP3:   %v", mediacommonSupportedCodecs.MP3)
	t.Logf("  Opus:  %v", mediacommonSupportedCodecs.Opus)

	// EAC3 should be true with our fork
	if !mediacommonSupportedCodecs.EAC3 {
		t.Error("EAC3 should be supported with mediacommon fork")
	}
}

func TestRegistryUpdatedWithDetection(t *testing.T) {
	// Verify that the audioRegistry was updated by the init function
	eac3Info, ok := audioRegistry[AudioEAC3]
	if !ok {
		t.Fatal("AudioEAC3 not found in registry")
	}

	if !eac3Info.Demuxable {
		t.Error("AudioEAC3.Demuxable should be true after detection (with our fork)")
	}

	t.Logf("AudioEAC3 registry entry: Demuxable=%v, MPEGTSStreamType=0x%02X",
		eac3Info.Demuxable, eac3Info.MPEGTSStreamType)
}

func TestIsDemuxableUsesDetection(t *testing.T) {
	// Test that the IsDemuxable methods now use the dynamically detected values
	eac3 := AudioEAC3
	if !eac3.IsDemuxable() {
		t.Error("AudioEAC3.IsDemuxable() should return true with our fork")
	}

	// Also test via the convenience function
	if !IsAudioDemuxable("eac3") {
		t.Error("IsAudioDemuxable(\"eac3\") should return true with our fork")
	}

	// Test alias
	if !IsAudioDemuxable("ec-3") {
		t.Error("IsAudioDemuxable(\"ec-3\") should return true with our fork")
	}
}
