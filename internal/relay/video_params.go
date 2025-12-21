// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"fmt"
	"strings"
	"sync"
)

// H264 NAL unit types
const (
	H264NALTypeSlice    = 1  // Non-IDR slice
	H264NALTypeIDR      = 5  // IDR slice (keyframe)
	H264NALTypeSEI      = 6  // Supplemental Enhancement Information
	H264NALTypeSPS      = 7  // Sequence Parameter Set
	H264NALTypePPS      = 8  // Picture Parameter Set
	H264NALTypeAUD      = 9  // Access Unit Delimiter
	H264NALTypeEOSeq    = 10 // End of Sequence
	H264NALTypeEOStream = 11 // End of Stream
	H264NALTypeFiller   = 12 // Filler Data
)

// H265 NAL unit types
const (
	H265NALTypeBLAWLP    = 16 // BLA_W_LP (keyframe)
	H265NALTypeBLAWRADL  = 17 // BLA_W_RADL (keyframe)
	H265NALTypeBLANLP    = 18 // BLA_N_LP (keyframe)
	H265NALTypeIDRWRADL  = 19 // IDR_W_RADL (keyframe)
	H265NALTypeIDRNLP    = 20 // IDR_N_LP (keyframe)
	H265NALTypeCRANUT    = 21 // CRA_NUT (keyframe)
	H265NALTypeVPS       = 32 // Video Parameter Set
	H265NALTypeSPS       = 33 // Sequence Parameter Set
	H265NALTypePPS       = 34 // Picture Parameter Set
	H265NALTypeAUD       = 35 // Access Unit Delimiter
	H265NALTypeEOS       = 36 // End of Sequence
	H265NALTypeEOB       = 37 // End of Bitstream
	H265NALTypeFD        = 38 // Filler Data
	H265NALTypePrefixSEI = 39 // Prefix SEI
	H265NALTypeSuffixSEI = 40 // Suffix SEI
)

// VideoParamHelper extracts and stores video parameter sets (SPS/PPS for H.264, VPS/SPS/PPS for H.265)
// and provides methods to prepend them to keyframes. This ensures that parameter sets are always
// available to decoders, even after buffer eviction.
type VideoParamHelper struct {
	mu sync.RWMutex

	// H.264 parameter sets
	h264SPS []byte
	h264PPS []byte

	// H.265 parameter sets
	h265VPS []byte
	h265SPS []byte
	h265PPS []byte
}

// NewVideoParamHelper creates a new video parameter set helper.
func NewVideoParamHelper() *VideoParamHelper {
	return &VideoParamHelper{}
}

// ExtractFromAnnexB extracts parameter sets from Annex B formatted data.
// It parses the NAL units and stores any VPS/SPS/PPS found.
// Returns true if any new parameter sets were extracted.
func (h *VideoParamHelper) ExtractFromAnnexB(data []byte, isH265 bool) bool {
	nalus := ParseAnnexBNALUs(data)
	return h.ExtractFromNALUs(nalus, isH265)
}

// ExtractFromNALUs extracts parameter sets from a slice of NAL units.
// Returns true if any new parameter sets were extracted.
func (h *VideoParamHelper) ExtractFromNALUs(nalus [][]byte, isH265 bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	extracted := false

	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}

		if isH265 {
			// H.265 NAL unit type is in bits 1-6 of first byte
			naluType := (nalu[0] >> 1) & 0x3F

			switch naluType {
			case H265NALTypeVPS:
				if !bytesEqual(h.h265VPS, nalu) {
					h.h265VPS = copyBytes(nalu)
					extracted = true
				}
			case H265NALTypeSPS:
				if !bytesEqual(h.h265SPS, nalu) {
					h.h265SPS = copyBytes(nalu)
					extracted = true
				}
			case H265NALTypePPS:
				if !bytesEqual(h.h265PPS, nalu) {
					h.h265PPS = copyBytes(nalu)
					extracted = true
				}
			}
		} else {
			// H.264 NAL unit type is in bits 0-4 of first byte
			naluType := nalu[0] & 0x1F

			switch naluType {
			case H264NALTypeSPS:
				if !bytesEqual(h.h264SPS, nalu) {
					h.h264SPS = copyBytes(nalu)
					extracted = true
				}
			case H264NALTypePPS:
				if !bytesEqual(h.h264PPS, nalu) {
					h.h264PPS = copyBytes(nalu)
					extracted = true
				}
			}
		}
	}

	return extracted
}

// IsH264IDR checks if Annex B data contains an H.264 IDR frame.
func (h *VideoParamHelper) IsH264IDR(data []byte) bool {
	nalus := ParseAnnexBNALUs(data)
	for _, nalu := range nalus {
		if len(nalu) > 0 {
			naluType := nalu[0] & 0x1F
			if naluType == H264NALTypeIDR {
				return true
			}
		}
	}
	return false
}

// IsH265IDR checks if Annex B data contains an H.265 IDR frame.
func (h *VideoParamHelper) IsH265IDR(data []byte) bool {
	nalus := ParseAnnexBNALUs(data)
	for _, nalu := range nalus {
		if len(nalu) > 0 {
			naluType := (nalu[0] >> 1) & 0x3F
			// Check for IDR_W_RADL, IDR_N_LP, BLA, or CRA frames
			if naluType >= H265NALTypeBLAWLP && naluType <= H265NALTypeCRANUT {
				return true
			}
		}
	}
	return false
}

// IsKeyframe checks if the data contains a keyframe (works for both H.264 and H.265).
func (h *VideoParamHelper) IsKeyframe(data []byte, isH265 bool) bool {
	if isH265 {
		return h.IsH265IDR(data)
	}
	return h.IsH264IDR(data)
}

// HasH264Params returns true if H.264 SPS and PPS are available.
func (h *VideoParamHelper) HasH264Params() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.h264SPS != nil && h.h264PPS != nil
}

// HasH265Params returns true if H.265 VPS, SPS, and PPS are available.
func (h *VideoParamHelper) HasH265Params() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.h265VPS != nil && h.h265SPS != nil && h.h265PPS != nil
}

// GetH264Params returns copies of the H.264 parameter sets.
func (h *VideoParamHelper) GetH264Params() (sps, pps []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return copyBytes(h.h264SPS), copyBytes(h.h264PPS)
}

// GetH265Params returns copies of the H.265 parameter sets.
func (h *VideoParamHelper) GetH265Params() (vps, sps, pps []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return copyBytes(h.h265VPS), copyBytes(h.h265SPS), copyBytes(h.h265PPS)
}

// SetH264Params sets the H.264 parameter sets.
func (h *VideoParamHelper) SetH264Params(sps, pps []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.h264SPS = copyBytes(sps)
	h.h264PPS = copyBytes(pps)
}

// SetH265Params sets the H.265 parameter sets.
func (h *VideoParamHelper) SetH265Params(vps, sps, pps []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.h265VPS = copyBytes(vps)
	h.h265SPS = copyBytes(sps)
	h.h265PPS = copyBytes(pps)
}

// PrependParamsToKeyframeAnnexB prepends parameter sets to a keyframe in Annex B format.
// If the data is not a keyframe or parameters are not available, returns the original data.
// For H.265: prepends VPS, SPS, PPS
// For H.264: prepends SPS, PPS
func (h *VideoParamHelper) PrependParamsToKeyframeAnnexB(data []byte, isH265 bool) []byte {
	// Check if this is a keyframe
	if !h.IsKeyframe(data, isH265) {
		return data
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if isH265 {
		if h.h265VPS == nil || h.h265SPS == nil || h.h265PPS == nil {
			return data
		}

		// Check if data already has VPS/SPS/PPS
		if h.dataHasH265Params(data) {
			return data
		}

		// Prepend VPS, SPS, PPS with Annex B start codes
		return h.buildAnnexBWithParams([][]byte{h.h265VPS, h.h265SPS, h.h265PPS}, data)
	}

	// H.264
	if h.h264SPS == nil || h.h264PPS == nil {
		return data
	}

	// Check if data already has SPS/PPS
	if h.dataHasH264Params(data) {
		return data
	}

	// Prepend SPS, PPS with Annex B start codes
	return h.buildAnnexBWithParams([][]byte{h.h264SPS, h.h264PPS}, data)
}

// PrependParamsToKeyframeNALUs prepends parameter sets to a keyframe given as NAL unit slices.
// Returns the NAL units with parameter sets prepended if this is a keyframe.
func (h *VideoParamHelper) PrependParamsToKeyframeNALUs(nalus [][]byte, isH265 bool) [][]byte {
	// Check if this contains a keyframe
	hasKeyframe := false
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		if isH265 {
			naluType := (nalu[0] >> 1) & 0x3F
			if naluType >= H265NALTypeBLAWLP && naluType <= H265NALTypeCRANUT {
				hasKeyframe = true
				break
			}
		} else {
			naluType := nalu[0] & 0x1F
			if naluType == H264NALTypeIDR {
				hasKeyframe = true
				break
			}
		}
	}

	if !hasKeyframe {
		return nalus
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if isH265 {
		if h.h265VPS == nil || h.h265SPS == nil || h.h265PPS == nil {
			return nalus
		}

		// Check if NALUs already have VPS/SPS/PPS
		if h.nalusHaveH265Params(nalus) {
			return nalus
		}

		// Prepend VPS, SPS, PPS
		result := make([][]byte, 0, len(nalus)+3)
		result = append(result, copyBytes(h.h265VPS))
		result = append(result, copyBytes(h.h265SPS))
		result = append(result, copyBytes(h.h265PPS))
		result = append(result, nalus...)
		return result
	}

	// H.264
	if h.h264SPS == nil || h.h264PPS == nil {
		return nalus
	}

	// Check if NALUs already have SPS/PPS
	if h.nalusHaveH264Params(nalus) {
		return nalus
	}

	// Prepend SPS, PPS
	result := make([][]byte, 0, len(nalus)+2)
	result = append(result, copyBytes(h.h264SPS))
	result = append(result, copyBytes(h.h264PPS))
	result = append(result, nalus...)
	return result
}

// dataHasH265Params checks if Annex B data already contains VPS, SPS, and PPS.
func (h *VideoParamHelper) dataHasH265Params(data []byte) bool {
	nalus := ParseAnnexBNALUs(data)
	return h.nalusHaveH265Params(nalus)
}

// nalusHaveH265Params checks if NAL units contain VPS, SPS, and PPS.
func (h *VideoParamHelper) nalusHaveH265Params(nalus [][]byte) bool {
	hasVPS, hasSPS, hasPPS := false, false, false
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		naluType := (nalu[0] >> 1) & 0x3F
		switch naluType {
		case H265NALTypeVPS:
			hasVPS = true
		case H265NALTypeSPS:
			hasSPS = true
		case H265NALTypePPS:
			hasPPS = true
		}
	}
	return hasVPS && hasSPS && hasPPS
}

// dataHasH264Params checks if Annex B data already contains SPS and PPS.
func (h *VideoParamHelper) dataHasH264Params(data []byte) bool {
	nalus := ParseAnnexBNALUs(data)
	return h.nalusHaveH264Params(nalus)
}

// nalusHaveH264Params checks if NAL units contain SPS and PPS.
func (h *VideoParamHelper) nalusHaveH264Params(nalus [][]byte) bool {
	hasSPS, hasPPS := false, false
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		naluType := nalu[0] & 0x1F
		switch naluType {
		case H264NALTypeSPS:
			hasSPS = true
		case H264NALTypePPS:
			hasPPS = true
		}
	}
	return hasSPS && hasPPS
}

// buildAnnexBWithParams builds Annex B data with parameter NALUs prepended.
func (h *VideoParamHelper) buildAnnexBWithParams(params [][]byte, originalData []byte) []byte {
	// Calculate total size needed
	size := len(originalData)
	for _, param := range params {
		size += 4 + len(param) // 4-byte start code + NAL unit
	}

	result := make([]byte, 0, size)

	// Add parameter sets with 4-byte start codes
	for _, param := range params {
		result = append(result, 0x00, 0x00, 0x00, 0x01)
		result = append(result, param...)
	}

	// Add original data
	result = append(result, originalData...)

	return result
}

// ParseAnnexBNALUs parses Annex B formatted data into individual NAL units.
// This handles both 3-byte (0x000001) and 4-byte (0x00000001) start codes.
func ParseAnnexBNALUs(data []byte) [][]byte {
	if len(data) < 4 {
		return nil
	}

	var nalus [][]byte
	start := -1

	for i := 0; i < len(data)-2; i++ {
		// Check for start code
		if data[i] == 0x00 && data[i+1] == 0x00 {
			isStartCode := false
			startCodeLen := 0

			if data[i+2] == 0x01 {
				// 3-byte start code
				isStartCode = true
				startCodeLen = 3
			} else if i+3 < len(data) && data[i+2] == 0x00 && data[i+3] == 0x01 {
				// 4-byte start code
				isStartCode = true
				startCodeLen = 4
			}

			if isStartCode {
				if start >= 0 {
					// End previous NAL unit
					nalus = append(nalus, data[start:i])
				}
				start = i + startCodeLen
				i += startCodeLen - 1 // -1 because loop will increment
			}
		}
	}

	// Don't forget the last NAL unit
	if start >= 0 && start < len(data) {
		nalus = append(nalus, data[start:])
	}

	return nalus
}

// BuildAnnexB builds Annex B formatted data from NAL units.
func BuildAnnexB(nalus [][]byte) []byte {
	// Calculate total size
	size := 0
	for _, nalu := range nalus {
		size += 4 + len(nalu) // 4-byte start code + NAL unit
	}

	data := make([]byte, 0, size)
	for _, nalu := range nalus {
		data = append(data, 0x00, 0x00, 0x00, 0x01)
		data = append(data, nalu...)
	}

	return data
}

// copyBytes creates a copy of a byte slice.
func copyBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	result := make([]byte, len(data))
	copy(result, data)
	return result
}

// bytesEqual checks if two byte slices are equal.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ForceKeyframePrepend prepends parameter sets to a keyframe without checking
// if the data contains an IDR/keyframe NAL. This is used when the caller
// already knows this is a keyframe (e.g., from demuxer metadata).
func (h *VideoParamHelper) ForceKeyframePrepend(data []byte, isH265 bool) []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if isH265 {
		if h.h265VPS == nil || h.h265SPS == nil || h.h265PPS == nil {
			return data
		}

		// Check if data already has VPS/SPS/PPS
		if h.dataHasH265Params(data) {
			return data
		}

		// Prepend VPS, SPS, PPS with Annex B start codes
		return h.buildAnnexBWithParams([][]byte{h.h265VPS, h.h265SPS, h.h265PPS}, data)
	}

	// H.264
	if h.h264SPS == nil || h.h264PPS == nil {
		return data
	}

	// Check if data already has SPS/PPS
	if h.dataHasH264Params(data) {
		return data
	}

	// Prepend SPS, PPS with Annex B start codes
	return h.buildAnnexBWithParams([][]byte{h.h264SPS, h.h264PPS}, data)
}

// ReorderNALUnits reorders NAL units to ensure parameter sets come first.
// This fixes issues where IPTV sources send NALs in wrong order (e.g., SEI before SPS/PPS).
// FFmpeg's decoder expects: VPS, SPS, PPS, AUD, SEI, slices (for H.265)
// or: SPS, PPS, AUD, SEI, slices (for H.264)
// The SEI messages often reference parameter sets, so they must come after.
func ReorderNALUnits(nalus [][]byte, isH265 bool) [][]byte {
	if len(nalus) <= 1 {
		return nalus
	}

	// Categorize NAL units
	var paramSets [][]byte  // VPS, SPS, PPS
	var audNALs [][]byte    // Access Unit Delimiter
	var seiNALs [][]byte    // Supplemental Enhancement Information
	var sliceNALs [][]byte  // Picture data (IDR, non-IDR slices)
	var otherNALs [][]byte  // Everything else

	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}

		if isH265 {
			// H.265: NAL type is in bits 1-6 of first byte
			naluType := (nalu[0] >> 1) & 0x3F
			switch naluType {
			case H265NALTypeVPS, H265NALTypeSPS, H265NALTypePPS:
				paramSets = append(paramSets, nalu)
			case H265NALTypeAUD:
				audNALs = append(audNALs, nalu)
			case H265NALTypePrefixSEI, H265NALTypeSuffixSEI:
				seiNALs = append(seiNALs, nalu)
			case H265NALTypeBLAWLP, H265NALTypeBLAWRADL, H265NALTypeBLANLP,
				H265NALTypeIDRWRADL, H265NALTypeIDRNLP, H265NALTypeCRANUT:
				// IDR/CRA/BLA keyframe NALs
				sliceNALs = append(sliceNALs, nalu)
			default:
				if naluType <= 31 {
					// VCL NAL units (video coding layer - slice data)
					sliceNALs = append(sliceNALs, nalu)
				} else {
					otherNALs = append(otherNALs, nalu)
				}
			}
		} else {
			// H.264: NAL type is in bits 0-4 of first byte
			naluType := nalu[0] & 0x1F
			switch naluType {
			case H264NALTypeSPS, H264NALTypePPS:
				paramSets = append(paramSets, nalu)
			case H264NALTypeAUD:
				audNALs = append(audNALs, nalu)
			case H264NALTypeSEI:
				seiNALs = append(seiNALs, nalu)
			case H264NALTypeIDR, H264NALTypeSlice:
				sliceNALs = append(sliceNALs, nalu)
			default:
				otherNALs = append(otherNALs, nalu)
			}
		}
	}

	// Rebuild in correct order: AUD, param sets, SEI, slices, other
	// Note: AUD should technically come first if present, but it's optional
	result := make([][]byte, 0, len(nalus))
	result = append(result, audNALs...)     // AUD first (if present)
	result = append(result, paramSets...)   // VPS/SPS/PPS
	result = append(result, seiNALs...)     // SEI (now after SPS/PPS so references work)
	result = append(result, sliceNALs...)   // IDR/slice data
	result = append(result, otherNALs...)   // Anything else

	return result
}

// analyzeNALTypes returns a string describing the NAL unit types in the data.
// Used for debugging keyframe issues.
func analyzeNALTypes(data []byte, isH265 bool) string {
	nalus := ParseAnnexBNALUs(data)
	if len(nalus) == 0 {
		return "no NALUs found"
	}

	types := make([]string, 0, len(nalus))
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}

		var typeName string
		if isH265 {
			naluType := (nalu[0] >> 1) & 0x3F
			switch naluType {
			case H265NALTypeVPS:
				typeName = "VPS(32)"
			case H265NALTypeSPS:
				typeName = "SPS(33)"
			case H265NALTypePPS:
				typeName = "PPS(34)"
			case H265NALTypeIDRWRADL:
				typeName = "IDR_W_RADL(19)"
			case H265NALTypeIDRNLP:
				typeName = "IDR_N_LP(20)"
			case H265NALTypeCRANUT:
				typeName = "CRA(21)"
			case H265NALTypeAUD:
				typeName = "AUD(35)"
			case H265NALTypePrefixSEI:
				typeName = "SEI(39)"
			default:
				typeName = fmt.Sprintf("type(%d)", naluType)
			}
		} else {
			naluType := nalu[0] & 0x1F
			switch naluType {
			case H264NALTypeSPS:
				typeName = "SPS(7)"
			case H264NALTypePPS:
				typeName = "PPS(8)"
			case H264NALTypeIDR:
				typeName = "IDR(5)"
			case H264NALTypeSlice:
				typeName = "Slice(1)"
			case H264NALTypeSEI:
				typeName = "SEI(6)"
			case H264NALTypeAUD:
				typeName = "AUD(9)"
			default:
				typeName = fmt.Sprintf("type(%d)", naluType)
			}
		}
		types = append(types, typeName)
	}

	return strings.Join(types, ",")
}
