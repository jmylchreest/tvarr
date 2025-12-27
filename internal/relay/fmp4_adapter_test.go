// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

func TestExtractVideoCodecParams_H264(t *testing.T) {
	// H.264 SPS NAL unit type 7
	sps := []byte{0x67, 0x42, 0xC0, 0x1E, 0x8C, 0x8D, 0x40}
	// H.264 PPS NAL unit type 8
	pps := []byte{0x68, 0xCE, 0x3C, 0x80}
	// H.264 IDR NAL unit type 5
	idr := []byte{0x65, 0x88, 0x84, 0x00}

	// Build Annex B formatted data with SPS, PPS, and IDR
	data := make([]byte, 0)
	data = append(data, 0x00, 0x00, 0x00, 0x01) // start code
	data = append(data, sps...)
	data = append(data, 0x00, 0x00, 0x00, 0x01) // start code
	data = append(data, pps...)
	data = append(data, 0x00, 0x00, 0x00, 0x01) // start code
	data = append(data, idr...)

	samples := []ESSample{
		{
			PTS:        90000,
			DTS:        90000,
			Data:       data,
			IsKeyframe: true,
		},
	}

	params := ExtractVideoCodecParams(samples)

	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Codec != "h264" {
		t.Errorf("Expected codec h264, got %s", params.Codec)
	}

	if params.H264SPS == nil {
		t.Error("Expected H264SPS to be set")
	}

	if params.H264PPS == nil {
		t.Error("Expected H264PPS to be set")
	}
}

func TestExtractVideoCodecParams_NoKeyframe(t *testing.T) {
	// Non-keyframe sample (P/B frame)
	samples := []ESSample{
		{
			PTS:        90000,
			DTS:        90000,
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x41, 0x9A, 0x00}, // non-IDR slice
			IsKeyframe: false,
		},
	}

	params := ExtractVideoCodecParams(samples)

	// Should still detect codec from sample
	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Codec != "h264" {
		t.Errorf("Expected codec h264, got %s", params.Codec)
	}
}

func TestExtractAudioCodecParams_AAC(t *testing.T) {
	// ADTS header for AAC-LC, 48kHz, stereo
	// Sync: 0xFFF, Profile: 1 (AAC-LC), Sample rate index: 3 (48kHz), Channels: 2
	adtsHeader := []byte{0xFF, 0xF1, 0x4C, 0x80, 0x01, 0x1F, 0xFC}
	aacFrame := append(adtsHeader, 0x00, 0x00, 0x00) // minimal frame data

	samples := []ESSample{
		{
			PTS:  90000,
			Data: aacFrame,
		},
	}

	params := ExtractAudioCodecParams(samples)

	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Codec != "aac" {
		t.Errorf("Expected codec aac, got %s", params.Codec)
	}

	if params.AACConfig == nil {
		t.Error("Expected AACConfig to be set")
	}
}

func TestExtractAudioCodecParams_DefaultAAC(t *testing.T) {
	// Raw audio data without ADTS header
	samples := []ESSample{
		{
			PTS:  90000,
			Data: []byte{0x21, 0x00, 0x49, 0x00}, // not ADTS
		},
	}

	params := ExtractAudioCodecParams(samples)

	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Codec != "aac" {
		t.Errorf("Expected default codec aac, got %s", params.Codec)
	}
}

func TestParseADTSHeader(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		expectedRate int
		expectNil    bool
	}{
		{
			name:         "48kHz stereo",
			data:         []byte{0xFF, 0xF1, 0x4C, 0x80, 0x01, 0x1F, 0xFC},
			expectedRate: 48000,
		},
		{
			name:         "44.1kHz stereo",
			data:         []byte{0xFF, 0xF1, 0x50, 0x80, 0x01, 0x1F, 0xFC},
			expectedRate: 44100,
		},
		{
			name:      "too short",
			data:      []byte{0xFF, 0xF1},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := parseADTSHeader(tt.data)

			if tt.expectNil {
				if config != nil {
					t.Error("Expected nil config")
				}
				return
			}

			if config == nil {
				t.Fatal("Expected non-nil config")
			}

			if config.SampleRate != tt.expectedRate {
				t.Errorf("Expected sample rate %d, got %d", tt.expectedRate, config.SampleRate)
			}
		})
	}
}

func TestExtractNALUnitsFromData(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		expectedCount int
		expectedFirst []byte
	}{
		{
			name:          "Annex B with 4-byte start code",
			data:          []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42},
			expectedCount: 1,
			expectedFirst: []byte{0x67, 0x42},
		},
		{
			name:          "Annex B with 3-byte start code",
			data:          []byte{0x00, 0x00, 0x01, 0x67, 0x42},
			expectedCount: 1,
			expectedFirst: []byte{0x67, 0x42},
		},
		{
			name:          "Multiple NAL units",
			data:          []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x00, 0x01, 0x68, 0xCE},
			expectedCount: 2,
			expectedFirst: []byte{0x67, 0x42},
		},
		{
			name:          "Raw NAL unit",
			data:          []byte{0x67, 0x42, 0xC0},
			expectedCount: 1,
			expectedFirst: []byte{0x67, 0x42, 0xC0},
		},
		{
			name:          "Empty",
			data:          []byte{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			units := extractNALUnitsFromData(tt.data)

			if len(units) != tt.expectedCount {
				t.Errorf("Expected %d units, got %d", tt.expectedCount, len(units))
				return
			}

			if tt.expectedCount > 0 && tt.expectedFirst != nil {
				if len(units[0]) != len(tt.expectedFirst) {
					t.Errorf("First unit length mismatch: expected %d, got %d", len(tt.expectedFirst), len(units[0]))
				}
			}
		})
	}
}

func TestConvertESSamplesToFMP4Video(t *testing.T) {
	samples := []ESSample{
		{
			PTS:        90000,
			DTS:        90000,
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84},
			IsKeyframe: true,
			Sequence:   1,
		},
		{
			PTS:        93000,
			DTS:        93000,
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x41, 0x9A},
			IsKeyframe: false,
			Sequence:   2,
		},
	}

	fmp4Samples, baseTime := ConvertESSamplesToFMP4Video(samples, 90000)

	if len(fmp4Samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(fmp4Samples))
	}

	if baseTime != 90000 {
		t.Errorf("Expected base time 90000, got %d", baseTime)
	}

	// First sample should be sync sample (keyframe)
	if fmp4Samples[0].IsNonSyncSample {
		t.Error("First sample should be sync sample")
	}

	// Second sample should be non-sync
	if !fmp4Samples[1].IsNonSyncSample {
		t.Error("Second sample should be non-sync sample")
	}

	// Duration should be calculated
	if fmp4Samples[0].Duration != 3000 {
		t.Errorf("Expected duration 3000, got %d", fmp4Samples[0].Duration)
	}
}

func TestConvertESSamplesToFMP4Audio(t *testing.T) {
	// ADTS header for 48kHz
	adtsHeader := []byte{0xFF, 0xF1, 0x4C, 0x80, 0x00, 0x0F, 0xFC}
	rawAudio := []byte{0x21, 0x00}

	samples := []ESSample{
		{
			PTS:      90000,
			Data:     append(adtsHeader, rawAudio...),
			Sequence: 1,
		},
		{
			PTS:      91920, // 1024 samples at 48kHz * 90kHz/48kHz
			Data:     append(adtsHeader, rawAudio...),
			Sequence: 2,
		},
	}

	fmp4Samples, baseTime := ConvertESSamplesToFMP4Audio(samples, 90000, 48000)

	if len(fmp4Samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(fmp4Samples))
	}

	if baseTime != 90000 {
		t.Errorf("Expected base time 90000, got %d", baseTime)
	}

	// Payload should have ADTS stripped
	if len(fmp4Samples[0].Payload) >= len(adtsHeader) {
		// Check if ADTS was stripped (payload should be shorter or different)
		if fmp4Samples[0].Payload[0] == 0xFF && (fmp4Samples[0].Payload[1]&0xF0) == 0xF0 {
			t.Error("ADTS header should be stripped")
		}
	}
}

func TestStripADTSHeader(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		expectedLength int
	}{
		{
			name:           "With ADTS header (7 bytes)",
			data:           []byte{0xFF, 0xF1, 0x4C, 0x80, 0x00, 0x0F, 0xFC, 0x21, 0x00},
			expectedLength: 2, // raw audio only
		},
		{
			name:           "No ADTS header",
			data:           []byte{0x21, 0x00, 0x49},
			expectedLength: 3, // unchanged
		},
		{
			name:           "Too short for ADTS",
			data:           []byte{0xFF, 0xF1},
			expectedLength: 2, // unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripADTSHeader(tt.data)
			if len(result) != tt.expectedLength {
				t.Errorf("Expected length %d, got %d", tt.expectedLength, len(result))
			}
		})
	}
}

func TestConvertToAnnexB(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectStart bool // should have start code at beginning
	}{
		{
			name:        "Already Annex B (4-byte)",
			data:        []byte{0x00, 0x00, 0x00, 0x01, 0x67},
			expectStart: true,
		},
		{
			name:        "Already Annex B (3-byte)",
			data:        []byte{0x00, 0x00, 0x01, 0x67},
			expectStart: true,
		},
		{
			name:        "Raw NAL",
			data:        []byte{0x67, 0x42, 0xC0},
			expectStart: true,
		},
		{
			name:        "Empty",
			data:        []byte{},
			expectStart: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToAnnexB(tt.data)

			if tt.expectStart && len(result) >= 4 {
				// Should have start code
				if result[0] != 0x00 || result[1] != 0x00 {
					t.Error("Expected start code prefix")
				}
			}
		})
	}
}

func TestConvertAnnexBToAVCC(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		expectLength   bool // should have length prefix at beginning
		expectedPrefix []byte
	}{
		{
			name:           "Single NAL unit Annex B (4-byte start code)",
			data:           []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xC0},
			expectLength:   true,
			expectedPrefix: []byte{0x00, 0x00, 0x00, 0x03}, // 3 bytes NAL
		},
		{
			name:           "Single NAL unit Annex B (3-byte start code)",
			data:           []byte{0x00, 0x00, 0x01, 0x67, 0x42, 0xC0},
			expectLength:   true,
			expectedPrefix: []byte{0x00, 0x00, 0x00, 0x03}, // 3 bytes NAL
		},
		{
			name:         "Empty",
			data:         []byte{},
			expectLength: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAnnexBToAVCC(tt.data)

			if tt.expectLength && len(result) >= 4 {
				// Should have length prefix (NOT start code)
				// Start code would be 0x00, 0x00, 0x00, 0x01 or 0x00, 0x00, 0x01
				// AVCC has the NAL length in the first 4 bytes
				if result[0] == 0x00 && result[1] == 0x00 && (result[2] == 0x00 && result[3] == 0x01) {
					t.Error("Result should NOT have start code, should have length prefix")
				}
				// Check length prefix matches expected NAL length
				nalLen := uint32(result[0])<<24 | uint32(result[1])<<16 | uint32(result[2])<<8 | uint32(result[3])
				expectedLen := uint32(len(tt.data) - 4) // Original minus start code
				if tt.data[2] == 0x01 {
					expectedLen = uint32(len(tt.data) - 3) // 3-byte start code
				}
				if nalLen != expectedLen {
					t.Errorf("Expected NAL length %d, got %d", expectedLen, nalLen)
				}
			}
		})
	}
}

func TestESSampleAdapter(t *testing.T) {
	adapter := NewESSampleAdapter(DefaultESSampleAdapterConfig())

	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}

	// Test video params update
	sps := []byte{0x67, 0x42, 0xC0, 0x1E}
	pps := []byte{0x68, 0xCE, 0x3C, 0x80}

	videoData := make([]byte, 0)
	videoData = append(videoData, 0x00, 0x00, 0x00, 0x01)
	videoData = append(videoData, sps...)
	videoData = append(videoData, 0x00, 0x00, 0x00, 0x01)
	videoData = append(videoData, pps...)

	videoSamples := []ESSample{
		{
			PTS:        90000,
			DTS:        90000,
			Data:       videoData,
			IsKeyframe: true,
		},
	}

	updated := adapter.UpdateVideoParams(videoSamples)
	if !updated {
		t.Error("Expected video params to be updated")
	}

	params := adapter.VideoParams()
	if params == nil {
		t.Error("Expected non-nil video params")
	}

	// Test audio params update
	adtsHeader := []byte{0xFF, 0xF1, 0x4C, 0x80, 0x01, 0x1F, 0xFC}
	audioSamples := []ESSample{
		{
			PTS:  90000,
			Data: adtsHeader,
		},
	}

	updated = adapter.UpdateAudioParams(audioSamples)
	if !updated {
		t.Error("Expected audio params to be updated")
	}

	audioParams := adapter.AudioParams()
	if audioParams == nil {
		t.Error("Expected non-nil audio params")
	}

	// Test lock
	adapter.LockParams()

	// After lock, updates should return false
	updated = adapter.UpdateVideoParams(videoSamples)
	if updated {
		t.Error("Expected update to fail after lock")
	}
}

func TestESSampleAdapter_ConfigureWriter(t *testing.T) {
	adapter := NewESSampleAdapter(DefaultESSampleAdapterConfig())
	writer := NewFMP4Writer()

	// Set up params manually with a valid minimal H.264 SPS/PPS
	// SPS: baseline profile, level 3.0, 640x480
	adapter.videoParams = &VideoCodecParams{
		Codec:   "h264",
		H264SPS: []byte{0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0x50, 0x1e, 0xd8, 0x08, 0x00, 0x00, 0x03, 0x00, 0x08, 0x00, 0x00, 0x03, 0x00, 0x3c, 0x8f, 0x16, 0x2d, 0x96},
		H264PPS: []byte{0x68, 0xce, 0x06, 0xe2},
	}
	adapter.audioParams = &AudioCodecParams{
		Codec: "aac",
		AACConfig: &mpeg4audio.AudioSpecificConfig{
			Type:         mpeg4audio.ObjectTypeAACLC,
			SampleRate:   48000,
			ChannelCount: 2,
		},
	}

	err := adapter.ConfigureWriter(writer)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Writer should now be configured - try generating init
	initData, err := writer.GenerateInit(true, true, 90000, 90000)
	if err != nil {
		t.Errorf("Failed to generate init: %v", err)
	}

	if len(initData) == 0 {
		t.Error("Expected non-empty init data")
	}
}

func TestConvertESSamplesToFMP4Video_SingleSample(t *testing.T) {
	samples := []ESSample{
		{
			PTS:        90000,
			DTS:        90000,
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88},
			IsKeyframe: true,
		},
	}

	fmp4Samples, _ := ConvertESSamplesToFMP4Video(samples, 90000)

	if len(fmp4Samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(fmp4Samples))
	}

	// Duration should use default for single sample
	if fmp4Samples[0].Duration == 0 {
		t.Error("Expected non-zero duration")
	}
}

func TestConvertESSamplesToFMP4Video_EmptySamples(t *testing.T) {
	samples := []ESSample{}

	fmp4Samples, baseTime := ConvertESSamplesToFMP4Video(samples, 90000)

	if len(fmp4Samples) != 0 {
		t.Errorf("Expected 0 samples, got %d", len(fmp4Samples))
	}

	if baseTime != 0 {
		t.Errorf("Expected base time 0, got %d", baseTime)
	}
}

func TestConvertESSamplesToFMP4Audio_EmptySamples(t *testing.T) {
	samples := []ESSample{}

	fmp4Samples, baseTime := ConvertESSamplesToFMP4Audio(samples, 90000, 48000)

	if len(fmp4Samples) != 0 {
		t.Errorf("Expected 0 samples, got %d", len(fmp4Samples))
	}

	if baseTime != 0 {
		t.Errorf("Expected base time 0, got %d", baseTime)
	}
}

func TestVideoCodecParams_H265Detection(t *testing.T) {
	// H.265 NAL unit types use different encoding: (nal[0] >> 1) & 0x3F
	// VPS = 32, SPS = 33, PPS = 34
	// For type 33: byte = (33 << 1) | 0 = 0x42 (with nuh_layer_id=0, nuh_temporal_id_plus1 in second byte)
	h265VPS := []byte{0x40, 0x01, 0x0C, 0x01} // H.265 VPS (type 32 = 0x40)
	h265SPS := []byte{0x42, 0x01, 0x01}       // H.265 SPS (type 33 = 0x42)
	h265PPS := []byte{0x44, 0x01}             // H.265 PPS (type 34 = 0x44)

	data := make([]byte, 0)
	data = append(data, 0x00, 0x00, 0x00, 0x01)
	data = append(data, h265VPS...)
	data = append(data, 0x00, 0x00, 0x00, 0x01)
	data = append(data, h265SPS...)
	data = append(data, 0x00, 0x00, 0x00, 0x01)
	data = append(data, h265PPS...)

	samples := []ESSample{
		{
			PTS:        90000,
			DTS:        90000,
			Data:       data,
			IsKeyframe: true,
		},
	}

	params := ExtractVideoCodecParams(samples)

	if params == nil {
		t.Fatal("Expected non-nil params")
	}

	if params.Codec != "h265" {
		t.Errorf("Expected codec h265, got %s", params.Codec)
	}

	if params.H265SPS == nil {
		t.Error("Expected H265SPS to be set")
	}

	if params.H265PPS == nil {
		t.Error("Expected H265PPS to be set")
	}
}

func TestDefaultESSampleAdapterConfig(t *testing.T) {
	config := DefaultESSampleAdapterConfig()

	if config.VideoTimescale != 90000 {
		t.Errorf("Expected video timescale 90000, got %d", config.VideoTimescale)
	}

	if config.AudioTimescale != 90000 {
		t.Errorf("Expected audio timescale 90000, got %d", config.AudioTimescale)
	}
}

// TestFMP4SampleFormat verifies the fmp4.Sample structure is correctly populated
func TestFMP4SampleFormat(t *testing.T) {
	samples := []ESSample{
		{
			PTS:        93000, // 1 second + 1/30 second
			DTS:        90000, // 1 second
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88},
			IsKeyframe: true,
		},
	}

	fmp4Samples, _ := ConvertESSamplesToFMP4Video(samples, 90000)

	if len(fmp4Samples) != 1 {
		t.Fatal("Expected 1 sample")
	}

	sample := fmp4Samples[0]

	// PTS offset should be PTS - DTS
	expectedPTSOffset := int32(93000 - 90000)
	if sample.PTSOffset != expectedPTSOffset {
		t.Errorf("Expected PTS offset %d, got %d", expectedPTSOffset, sample.PTSOffset)
	}

	// Should be sync sample
	if sample.IsNonSyncSample {
		t.Error("Expected sync sample")
	}

	// Payload should not be empty
	if len(sample.Payload) == 0 {
		t.Error("Expected non-empty payload")
	}
}

// Helper function to create a valid fmp4.Sample for testing
func createTestFMP4Sample(duration uint32, isKeyframe bool) *fmp4.Sample {
	return &fmp4.Sample{
		Duration:        duration,
		PTSOffset:       0,
		IsNonSyncSample: !isKeyframe,
		Payload:         []byte{0x00, 0x00, 0x00, 0x01, 0x65},
	}
}

func TestCreateTestFMP4Sample(t *testing.T) {
	sample := createTestFMP4Sample(3000, true)

	if sample.Duration != 3000 {
		t.Errorf("Expected duration 3000, got %d", sample.Duration)
	}

	if sample.IsNonSyncSample {
		t.Error("Expected sync sample")
	}
}
