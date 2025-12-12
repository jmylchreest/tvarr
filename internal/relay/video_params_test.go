package relay

import (
	"bytes"
	"testing"
)

// Test H.264 NAL unit data
var (
	// Minimal SPS NAL unit (NAL type 7)
	h264SPS = []byte{0x67, 0x42, 0x00, 0x1f, 0x96, 0x54, 0x05, 0x01}
	// Minimal PPS NAL unit (NAL type 8)
	h264PPS = []byte{0x68, 0xce, 0x3c, 0x80}
	// IDR slice NAL unit (NAL type 5)
	h264IDR = []byte{0x65, 0x88, 0x84, 0x00, 0x00, 0x03}
	// Non-IDR slice NAL unit (NAL type 1)
	h264NonIDR = []byte{0x41, 0x9a, 0x00, 0x00}
)

// Test H.265 NAL unit data
var (
	// VPS NAL unit (NAL type 32)
	h265VPS = []byte{0x40, 0x01, 0x0c, 0x01, 0xff, 0xff}
	// SPS NAL unit (NAL type 33)
	h265SPS = []byte{0x42, 0x01, 0x01, 0x01, 0x60, 0x00}
	// PPS NAL unit (NAL type 34)
	h265PPS = []byte{0x44, 0x01, 0xc1, 0x72, 0xb4, 0x62}
	// IDR_W_RADL NAL unit (NAL type 19)
	h265IDR = []byte{0x26, 0x01, 0xaf, 0x00, 0x00}
	// Non-IDR slice NAL unit (NAL type 1)
	h265NonIDR = []byte{0x02, 0x01, 0xd0, 0x00, 0x00}
)

func buildAnnexB(nalus ...[]byte) []byte {
	var buf bytes.Buffer
	for _, nalu := range nalus {
		buf.Write([]byte{0x00, 0x00, 0x00, 0x01})
		buf.Write(nalu)
	}
	return buf.Bytes()
}

func TestVideoParamHelper_ExtractH264Params(t *testing.T) {
	h := NewVideoParamHelper()

	// Initially should have no params
	if h.HasH264Params() {
		t.Error("Expected no H264 params initially")
	}

	// Extract from data with SPS/PPS
	data := buildAnnexB(h264SPS, h264PPS, h264IDR)
	extracted := h.ExtractFromAnnexB(data, false)

	if !extracted {
		t.Error("Expected params to be extracted")
	}

	if !h.HasH264Params() {
		t.Error("Expected H264 params to be available")
	}

	sps, pps := h.GetH264Params()
	if !bytes.Equal(sps, h264SPS) {
		t.Errorf("SPS mismatch: got %x, want %x", sps, h264SPS)
	}
	if !bytes.Equal(pps, h264PPS) {
		t.Errorf("PPS mismatch: got %x, want %x", pps, h264PPS)
	}
}

func TestVideoParamHelper_ExtractH265Params(t *testing.T) {
	h := NewVideoParamHelper()

	// Initially should have no params
	if h.HasH265Params() {
		t.Error("Expected no H265 params initially")
	}

	// Extract from data with VPS/SPS/PPS
	data := buildAnnexB(h265VPS, h265SPS, h265PPS, h265IDR)
	extracted := h.ExtractFromAnnexB(data, true)

	if !extracted {
		t.Error("Expected params to be extracted")
	}

	if !h.HasH265Params() {
		t.Error("Expected H265 params to be available")
	}

	vps, sps, pps := h.GetH265Params()
	if !bytes.Equal(vps, h265VPS) {
		t.Errorf("VPS mismatch: got %x, want %x", vps, h265VPS)
	}
	if !bytes.Equal(sps, h265SPS) {
		t.Errorf("SPS mismatch: got %x, want %x", sps, h265SPS)
	}
	if !bytes.Equal(pps, h265PPS) {
		t.Errorf("PPS mismatch: got %x, want %x", pps, h265PPS)
	}
}

func TestVideoParamHelper_IsH264IDR(t *testing.T) {
	h := NewVideoParamHelper()

	// IDR frame
	data := buildAnnexB(h264IDR)
	if !h.IsH264IDR(data) {
		t.Error("Expected IDR to be detected")
	}

	// Non-IDR frame
	data = buildAnnexB(h264NonIDR)
	if h.IsH264IDR(data) {
		t.Error("Expected non-IDR not to be detected as IDR")
	}

	// SPS/PPS are not IDR
	data = buildAnnexB(h264SPS, h264PPS)
	if h.IsH264IDR(data) {
		t.Error("Expected SPS/PPS not to be detected as IDR")
	}
}

func TestVideoParamHelper_IsH265IDR(t *testing.T) {
	h := NewVideoParamHelper()

	// IDR frame
	data := buildAnnexB(h265IDR)
	if !h.IsH265IDR(data) {
		t.Error("Expected IDR to be detected")
	}

	// Non-IDR frame
	data = buildAnnexB(h265NonIDR)
	if h.IsH265IDR(data) {
		t.Error("Expected non-IDR not to be detected as IDR")
	}

	// VPS/SPS/PPS are not IDR
	data = buildAnnexB(h265VPS, h265SPS, h265PPS)
	if h.IsH265IDR(data) {
		t.Error("Expected VPS/SPS/PPS not to be detected as IDR")
	}
}

func TestVideoParamHelper_PrependH264Params(t *testing.T) {
	h := NewVideoParamHelper()

	// Set up params
	h.SetH264Params(h264SPS, h264PPS)

	// Test prepending to IDR frame without params
	idrOnly := buildAnnexB(h264IDR)
	result := h.PrependParamsToKeyframeAnnexB(idrOnly, false)

	// Should now contain SPS, PPS, IDR
	nalus := ParseAnnexBNALUs(result)
	if len(nalus) != 3 {
		t.Fatalf("Expected 3 NAL units, got %d", len(nalus))
	}

	// Verify order: SPS, PPS, IDR
	if nalus[0][0]&0x1F != H264NALTypeSPS {
		t.Errorf("First NAL should be SPS, got type %d", nalus[0][0]&0x1F)
	}
	if nalus[1][0]&0x1F != H264NALTypePPS {
		t.Errorf("Second NAL should be PPS, got type %d", nalus[1][0]&0x1F)
	}
	if nalus[2][0]&0x1F != H264NALTypeIDR {
		t.Errorf("Third NAL should be IDR, got type %d", nalus[2][0]&0x1F)
	}
}

func TestVideoParamHelper_PrependH265Params(t *testing.T) {
	h := NewVideoParamHelper()

	// Set up params
	h.SetH265Params(h265VPS, h265SPS, h265PPS)

	// Test prepending to IDR frame without params
	idrOnly := buildAnnexB(h265IDR)
	result := h.PrependParamsToKeyframeAnnexB(idrOnly, true)

	// Should now contain VPS, SPS, PPS, IDR
	nalus := ParseAnnexBNALUs(result)
	if len(nalus) != 4 {
		t.Fatalf("Expected 4 NAL units, got %d", len(nalus))
	}

	// Verify order: VPS, SPS, PPS, IDR
	if (nalus[0][0]>>1)&0x3F != H265NALTypeVPS {
		t.Errorf("First NAL should be VPS, got type %d", (nalus[0][0]>>1)&0x3F)
	}
	if (nalus[1][0]>>1)&0x3F != H265NALTypeSPS {
		t.Errorf("Second NAL should be SPS, got type %d", (nalus[1][0]>>1)&0x3F)
	}
	if (nalus[2][0]>>1)&0x3F != H265NALTypePPS {
		t.Errorf("Third NAL should be PPS, got type %d", (nalus[2][0]>>1)&0x3F)
	}
	if (nalus[3][0]>>1)&0x3F != H265NALTypeIDRWRADL {
		t.Errorf("Fourth NAL should be IDR, got type %d", (nalus[3][0]>>1)&0x3F)
	}
}

func TestVideoParamHelper_DoesNotPrependToNonIDR(t *testing.T) {
	h := NewVideoParamHelper()
	h.SetH264Params(h264SPS, h264PPS)

	// Non-IDR frame
	nonIDR := buildAnnexB(h264NonIDR)
	result := h.PrependParamsToKeyframeAnnexB(nonIDR, false)

	// Should be unchanged
	if !bytes.Equal(result, nonIDR) {
		t.Error("Non-IDR frame should not have params prepended")
	}
}

func TestVideoParamHelper_DoesNotDuplicateParams(t *testing.T) {
	h := NewVideoParamHelper()
	h.SetH264Params(h264SPS, h264PPS)

	// IDR frame that already has SPS/PPS
	withParams := buildAnnexB(h264SPS, h264PPS, h264IDR)
	result := h.PrependParamsToKeyframeAnnexB(withParams, false)

	// Should be unchanged since params are already present
	if !bytes.Equal(result, withParams) {
		t.Error("Should not duplicate existing params")
	}
}

func TestVideoParamHelper_PrependNALUs(t *testing.T) {
	h := NewVideoParamHelper()
	h.SetH265Params(h265VPS, h265SPS, h265PPS)

	// Test prepending to NALU slice
	nalus := [][]byte{h265IDR}
	result := h.PrependParamsToKeyframeNALUs(nalus, true)

	if len(result) != 4 {
		t.Fatalf("Expected 4 NAL units, got %d", len(result))
	}

	// Verify VPS, SPS, PPS, IDR
	if (result[0][0]>>1)&0x3F != H265NALTypeVPS {
		t.Error("First NAL should be VPS")
	}
	if (result[1][0]>>1)&0x3F != H265NALTypeSPS {
		t.Error("Second NAL should be SPS")
	}
	if (result[2][0]>>1)&0x3F != H265NALTypePPS {
		t.Error("Third NAL should be PPS")
	}
}

func TestParseAnnexBNALUs(t *testing.T) {
	// Test 4-byte start codes
	data := buildAnnexB(h264SPS, h264PPS, h264IDR)
	nalus := ParseAnnexBNALUs(data)

	if len(nalus) != 3 {
		t.Fatalf("Expected 3 NAL units, got %d", len(nalus))
	}

	if !bytes.Equal(nalus[0], h264SPS) {
		t.Errorf("First NAL mismatch")
	}
	if !bytes.Equal(nalus[1], h264PPS) {
		t.Errorf("Second NAL mismatch")
	}
	if !bytes.Equal(nalus[2], h264IDR) {
		t.Errorf("Third NAL mismatch")
	}
}

func TestParseAnnexBNALUs_3ByteStartCode(t *testing.T) {
	// Build with 3-byte start codes
	var buf bytes.Buffer
	for _, nalu := range [][]byte{h264SPS, h264PPS} {
		buf.Write([]byte{0x00, 0x00, 0x01})
		buf.Write(nalu)
	}

	nalus := ParseAnnexBNALUs(buf.Bytes())

	if len(nalus) != 2 {
		t.Fatalf("Expected 2 NAL units, got %d", len(nalus))
	}
}

func TestBuildAnnexB(t *testing.T) {
	nalus := [][]byte{h264SPS, h264PPS, h264IDR}
	result := BuildAnnexB(nalus)

	// Parse back and verify
	parsed := ParseAnnexBNALUs(result)

	if len(parsed) != 3 {
		t.Fatalf("Expected 3 NAL units after round trip, got %d", len(parsed))
	}

	for i, nalu := range nalus {
		if !bytes.Equal(parsed[i], nalu) {
			t.Errorf("NAL %d mismatch after round trip", i)
		}
	}
}
