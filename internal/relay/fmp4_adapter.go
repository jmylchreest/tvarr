// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"log/slog"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/vp9"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// VideoCodecParams holds extracted video codec parameters.
type VideoCodecParams struct {
	Codec string // "h264", "h265", "av1", or "vp9"

	// H.264 parameters
	H264SPS []byte
	H264PPS []byte

	// H.265 parameters
	H265VPS []byte
	H265SPS []byte
	H265PPS []byte

	// AV1 parameters
	AV1SequenceHeader []byte

	// VP9 parameters (extracted from frame header)
	VP9Width             int
	VP9Height            int
	VP9Profile           uint8
	VP9BitDepth          uint8
	VP9ChromaSubsampling uint8
	VP9ColorRange        bool
}

// AudioCodecParams holds extracted audio codec parameters.
type AudioCodecParams struct {
	Codec     string // "aac", "ac3", "opus", "mp3"
	AACConfig *mpeg4audio.AudioSpecificConfig

	// Opus parameters
	OpusChannelCount int

	// AC3/EAC3 parameters
	AC3SampleRate   int
	AC3ChannelCount int

	// MP3 parameters
	MP3SampleRate   int
	MP3ChannelCount int
}

// NewAudioCodecParamsFromCodec creates AudioCodecParams from a codec name with sensible defaults.
// This is used when we know the codec from the variant but don't have detailed stream info.
func NewAudioCodecParamsFromCodec(codecName string) *AudioCodecParams {
	params := &AudioCodecParams{
		Codec: codecName,
	}

	switch codecName {
	case "opus":
		params.OpusChannelCount = 2 // Default stereo
	case "aac":
		params.AACConfig = &mpeg4audio.AudioSpecificConfig{
			Type:         mpeg4audio.ObjectTypeAACLC,
			SampleRate:   48000,
			ChannelCount: 2,
		}
	case "ac3", "eac3":
		params.AC3SampleRate = 48000
		params.AC3ChannelCount = 2
	case "mp3":
		params.MP3SampleRate = 48000
		params.MP3ChannelCount = 2
	}

	return params
}

// ExtractVideoCodecParams extracts video codec parameters from ES samples.
// It looks for SPS/PPS NAL units (H.264/H.265), sequence header OBU (AV1), or VP9 frame header.
func ExtractVideoCodecParams(samples []ESSample) *VideoCodecParams {
	for _, sample := range samples {
		if !sample.IsKeyframe {
			continue
		}

		// First, try to detect and extract AV1 parameters
		if params := tryExtractAV1Params(sample.Data); params != nil {
			return params
		}

		// Try to detect and extract VP9 parameters
		if params := tryExtractVP9Params(sample.Data); params != nil {
			return params
		}

		// Parse NAL units from sample data (H.264/H.265)
		nalUnits := extractNALUnitsFromData(sample.Data)
		if len(nalUnits) == 0 {
			continue
		}

		params := &VideoCodecParams{}

		for _, nal := range nalUnits {
			if len(nal) == 0 {
				continue
			}

			// Check for H.265 NAL types FIRST (different format: (nal[0] >> 1) & 0x3F)
			// H.265 NAL types 32-34 are VPS/SPS/PPS which don't conflict with H.264 types
			if len(nal) >= 2 {
				h265NalType := h265.NALUType((nal[0] >> 1) & 0x3F)

				switch h265NalType {
				case h265.NALUType_VPS_NUT:
					params.Codec = "h265"
					params.H265VPS = nal
				case h265.NALUType_SPS_NUT:
					params.Codec = "h265"
					params.H265SPS = nal
				case h265.NALUType_PPS_NUT:
					params.Codec = "h265"
					params.H265PPS = nal
				}
			}

			// Only check for H.264 NAL types if not already detected as H.265
			if params.Codec != "h265" {
				h264NalType := h264.NALUType(nal[0] & 0x1F)

				switch h264NalType {
				case h264.NALUTypeSPS:
					params.Codec = "h264"
					params.H264SPS = nal
				case h264.NALUTypePPS:
					params.Codec = "h264"
					params.H264PPS = nal
				}
			}
		}

		// If we found codec params, return them
		if params.Codec == "h264" && params.H264SPS != nil && params.H264PPS != nil {
			return params
		}
		if params.Codec == "h265" && params.H265SPS != nil && params.H265PPS != nil {
			return params
		}
	}

	// Try to detect codec from any video sample even without finding params
	for _, sample := range samples {
		// Try AV1 detection first
		if params := tryExtractAV1Params(sample.Data); params != nil {
			return params
		}

		// Try VP9 detection
		if params := tryExtractVP9Params(sample.Data); params != nil {
			return params
		}

		nalUnits := extractNALUnitsFromData(sample.Data)
		for _, nal := range nalUnits {
			if len(nal) == 0 {
				continue
			}

			// Check if it looks like H.264 IDR or slice using mediacommon constants
			h264NalType := h264.NALUType(nal[0] & 0x1F)
			if h264NalType == h264.NALUTypeIDR || h264NalType == h264.NALUTypeNonIDR {
				return &VideoCodecParams{Codec: "h264"}
			}

			// Check if it looks like H.265 VCL NAL units
			if len(nal) >= 2 {
				h265NalType := h265.NALUType((nal[0] >> 1) & 0x3F)
				// H.265 VCL NAL units are types 0-23 (up to RSV_IRAP_VCL23)
				if h265NalType <= h265.NALUType_RSV_IRAP_VCL23 {
					return &VideoCodecParams{Codec: "h265"}
				}
			}
		}
	}

	// Default to H.264 if we can't detect
	return &VideoCodecParams{Codec: "h264"}
}

// tryExtractVP9Params attempts to extract VP9 codec parameters from data.
// VP9 codec info is extracted from the frame header.
// Returns nil if data is not VP9.
func tryExtractVP9Params(data []byte) *VideoCodecParams {
	if len(data) < 3 {
		return nil
	}

	// Try to parse as VP9 frame header
	var hdr vp9.Header
	if err := hdr.Unmarshal(data); err != nil {
		return nil
	}

	// VP9 keyframes have NonKeyFrame = false
	// We need to extract codec parameters from the frame header
	params := &VideoCodecParams{
		Codec:                "vp9",
		VP9Width:             hdr.Width(),
		VP9Height:            hdr.Height(),
		VP9Profile:           hdr.Profile,
		VP9ChromaSubsampling: hdr.ChromaSubsampling(),
	}

	// Extract bit depth and color range from color config if available
	if hdr.ColorConfig != nil {
		params.VP9BitDepth = hdr.ColorConfig.BitDepth
		params.VP9ColorRange = hdr.ColorConfig.ColorRange
	} else {
		// Default to 8-bit
		params.VP9BitDepth = 8
	}

	return params
}

// tryExtractAV1Params attempts to extract AV1 codec parameters from data.
// AV1 uses OBU (Open Bitstream Unit) format instead of NAL units.
// Returns nil if data is not AV1.
func tryExtractAV1Params(data []byte) *VideoCodecParams {
	if len(data) < 2 {
		return nil
	}

	// Extract OBUs from the data
	obus := extractOBUsFromData(data)
	if len(obus) == 0 {
		return nil
	}

	params := &VideoCodecParams{}

	for _, obu := range obus {
		if len(obu) < 1 {
			continue
		}

		// Parse OBU header
		// First byte: obu_forbidden_bit(1) | obu_type(4) | obu_extension_flag(1) | obu_has_size_field(1) | reserved(1)
		obuType := av1.OBUType((obu[0] >> 3) & 0x0F)

		if obuType == av1.OBUTypeSequenceHeader {
			// Found sequence header - verify it's valid AV1
			var seqHdr av1.SequenceHeader
			if err := seqHdr.Unmarshal(obu); err == nil {
				params.Codec = "av1"
				params.AV1SequenceHeader = make([]byte, len(obu))
				copy(params.AV1SequenceHeader, obu)
				return params
			}
		}

		// Check for other AV1 OBU types to detect AV1 codec
		// OBU types: 0=Reserved, 1=SequenceHeader, 2=TemporalDelimiter, 3=FrameHeader,
		// 4=TileGroup, 5=Metadata, 6=Frame, 7=RedundantFrameHeader, 8=TileList
		// Types 1-8 are valid AV1 content types
		switch obuType {
		case av1.OBUTypeTemporalDelimiter:
			// This is a valid AV1 OBU type - mark as AV1 even without sequence header
			params.Codec = "av1"
		default:
			// Check other OBU types by value (3-8 are frame/tile types)
			if obuType >= 3 && obuType <= 8 {
				params.Codec = "av1"
			}
		}
	}

	if params.Codec == "av1" {
		return params
	}

	return nil
}

// extractOBUsFromData extracts OBUs from AV1 data.
// Handles both low overhead bitstream format and length-prefixed format.
func extractOBUsFromData(data []byte) [][]byte {
	if len(data) < 2 {
		return nil
	}

	var obus [][]byte
	offset := 0

	for offset < len(data) {
		if offset+1 > len(data) {
			break
		}

		// Check for valid OBU header
		firstByte := data[offset]
		// obu_forbidden_bit must be 0
		if (firstByte & 0x80) != 0 {
			// Not a valid OBU, this might not be AV1 data
			return nil
		}

		obuType := (firstByte >> 3) & 0x0F
		extensionFlag := (firstByte >> 2) & 0x01
		hasSizeField := (firstByte >> 1) & 0x01

		// Validate OBU type (must be 0-8, 15 or reserved)
		if obuType > 15 {
			return nil
		}

		headerSize := 1
		if extensionFlag != 0 {
			headerSize = 2
		}

		if offset+headerSize > len(data) {
			break
		}

		if hasSizeField != 0 {
			// Length-prefixed OBU - read LEB128 size
			sizeStart := offset + headerSize
			obuSize, bytesRead := readLEB128(data[sizeStart:])
			if bytesRead == 0 {
				break
			}

			totalSize := headerSize + bytesRead + int(obuSize)
			if offset+totalSize > len(data) {
				break
			}

			obus = append(obus, data[offset:offset+totalSize])
			offset += totalSize
		} else {
			// Low overhead format - OBU extends to end of data
			// This is typically only used for the last OBU in a temporal unit
			obus = append(obus, data[offset:])
			break
		}
	}

	return obus
}

// readLEB128 reads an unsigned LEB128 encoded integer.
// Returns the value and number of bytes read.
func readLEB128(data []byte) (uint64, int) {
	var value uint64
	for i := 0; i < len(data) && i < 8; i++ {
		b := data[i]
		value |= uint64(b&0x7F) << (7 * i)
		if (b & 0x80) == 0 {
			return value, i + 1
		}
	}
	return 0, 0 // Invalid LEB128
}

// ExtractAudioCodecParams extracts audio codec parameters from ES samples.
func ExtractAudioCodecParams(samples []ESSample) *AudioCodecParams {
	for _, sample := range samples {
		if len(sample.Data) == 0 {
			continue
		}

		// Check for ADTS header (AAC)
		if len(sample.Data) >= 7 && sample.Data[0] == 0xFF && (sample.Data[1]&0xF0) == 0xF0 {
			// Parse ADTS header to extract AAC config
			config := parseADTSHeader(sample.Data)
			if config != nil {
				return &AudioCodecParams{
					Codec:     "aac",
					AACConfig: config,
				}
			}
		}

		// Check for AC-3 sync word
		if len(sample.Data) >= 2 && sample.Data[0] == 0x0B && sample.Data[1] == 0x77 {
			return &AudioCodecParams{Codec: "ac3"}
		}

		// Default to AAC if we have audio data
		return &AudioCodecParams{
			Codec: "aac",
			AACConfig: &mpeg4audio.AudioSpecificConfig{
				Type:         mpeg4audio.ObjectTypeAACLC,
				SampleRate:   48000,
				ChannelCount: 2,
			},
		}
	}

	// Default AAC config
	return &AudioCodecParams{
		Codec: "aac",
		AACConfig: &mpeg4audio.AudioSpecificConfig{
			Type:         mpeg4audio.ObjectTypeAACLC,
			SampleRate:   48000,
			ChannelCount: 2,
		},
	}
}

// parseADTSHeader extracts MPEG-4 Audio config from an ADTS header.
func parseADTSHeader(data []byte) *mpeg4audio.AudioSpecificConfig {
	if len(data) < 7 {
		return nil
	}

	// ADTS header format:
	// syncword: 12 bits (0xFFF)
	// id: 1 bit (0 = MPEG-4, 1 = MPEG-2)
	// layer: 2 bits (always 00)
	// protection_absent: 1 bit
	// profile: 2 bits (profile - 1, so AAC-LC = 1 means profile = 2)
	// sampling_frequency_index: 4 bits
	// private_bit: 1 bit
	// channel_configuration: 3 bits
	// ...

	profile := ((data[2] >> 6) & 0x03) + 1 // profile is stored as profile - 1
	sampleRateIndex := (data[2] >> 2) & 0x0F
	channelConfig := ((data[2] & 0x01) << 2) | ((data[3] >> 6) & 0x03)

	// Sample rate lookup table
	sampleRates := []int{
		96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
		16000, 12000, 11025, 8000, 7350, 0, 0, 0,
	}

	if int(sampleRateIndex) >= len(sampleRates) || sampleRates[sampleRateIndex] == 0 {
		return nil
	}

	// Map ADTS profile to MPEG-4 Audio Object Type
	// Note: mediacommon only exports ObjectTypeAACLC, so we use it as default
	// Profile 2 in ADTS corresponds to AAC-LC (ObjectType 2)
	var objectType mpeg4audio.ObjectType
	switch profile {
	case 2:
		objectType = mpeg4audio.ObjectTypeAACLC
	default:
		// Use AAC-LC for other profiles as it's the most common and compatible
		objectType = mpeg4audio.ObjectTypeAACLC
	}

	return &mpeg4audio.AudioSpecificConfig{
		Type:         objectType,
		SampleRate:   sampleRates[sampleRateIndex],
		ChannelCount: int(channelConfig),
	}
}

// extractNALUnitsFromData extracts NAL units from video data using mediacommon.
// Handles both Annex B format (with start codes) and AVCC/length-prefixed formats.
func extractNALUnitsFromData(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}

	// Check if data is in Annex B format (starts with start code)
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 {
		if (data[2] == 0x01) || (data[2] == 0x00 && len(data) >= 4 && data[3] == 0x01) {
			// Use mediacommon to parse Annex B format
			var au h264.AnnexB
			if err := au.Unmarshal(data); err != nil {
				// Fallback: treat as single NAL unit
				return [][]byte{data}
			}
			return au
		}
	}

	// Try AVCC format (length-prefixed) using mediacommon
	if len(data) >= 4 {
		var au h264.AVCC
		if err := au.Unmarshal(data); err == nil && len(au) > 0 {
			return au
		}
	}

	// Raw NAL unit
	return [][]byte{data}
}

// ConvertESSamplesToFMP4Video converts ES video samples to fmp4.Sample format.
// For H.264/H.265, this converts from Annex B format (start codes) to AVCC/HVCC format
// (length-prefixed NALs) as required by fMP4/MP4 containers.
func ConvertESSamplesToFMP4Video(samples []ESSample, timescale uint32) ([]*fmp4.Sample, uint64) {
	if len(samples) == 0 {
		return nil, 0
	}

	fmp4Samples := make([]*fmp4.Sample, 0, len(samples))
	baseTime := uint64(samples[0].DTS)

	for i, sample := range samples {
		// Calculate duration (estimate from next sample or default)
		var duration uint32
		if i+1 < len(samples) {
			duration = uint32(samples[i+1].DTS - sample.DTS)
		} else if i > 0 {
			// Use average of previous samples
			duration = uint32(sample.DTS-samples[0].DTS) / uint32(i) //nolint:gosec // G602: samples[0] access is safe — guarded by i > 0 condition above
		} else {
			// Default to 1/30 second at 90kHz
			duration = timescale / 30
		}

		if duration == 0 {
			duration = timescale / 30
		}

		// Calculate PTS offset (composition time offset)
		ptsOffset := int32(sample.PTS - sample.DTS)

		// Convert NAL units from Annex B to AVCC/HVCC format for fMP4
		// fMP4/MP4 containers require length-prefixed NALs, not start codes
		payload := convertAnnexBToAVCC(sample.Data)

		// Log first sample conversion for debugging format issues
		if i == 0 && len(samples) > 0 {
			inputFormat := "unknown"
			outputFormat := "unknown"
			if len(sample.Data) >= 4 {
				d := sample.Data
				if d[0] == 0 && d[1] == 0 && (d[2] == 1 || (d[2] == 0 && d[3] == 1)) {
					inputFormat = "annexb"
				} else {
					inputFormat = "avcc"
				}
			}
			if len(payload) >= 4 {
				d := payload
				if d[0] == 0 && d[1] == 0 && (d[2] == 1 || (d[2] == 0 && d[3] == 1)) {
					outputFormat = "annexb"
				} else {
					// Check if first 4 bytes look like valid AVCC length
					length := uint32(d[0])<<24 | uint32(d[1])<<16 | uint32(d[2])<<8 | uint32(d[3])
					if length > 0 && int(length)+4 <= len(payload) {
						outputFormat = "avcc"
					} else {
						outputFormat = "invalid_avcc"
					}
				}
			}
			slog.Debug("Video sample format conversion",
				slog.String("input_format", inputFormat),
				slog.String("output_format", outputFormat),
				slog.Int("input_len", len(sample.Data)),
				slog.Int("output_len", len(payload)),
				slog.Bool("is_keyframe", sample.IsKeyframe))
		}

		fmp4Samples = append(fmp4Samples, &fmp4.Sample{
			Duration:        duration,
			PTSOffset:       ptsOffset,
			IsNonSyncSample: !sample.IsKeyframe,
			Payload:         payload,
		})
	}

	return fmp4Samples, baseTime
}

// ConvertESSamplesToFMP4VideoAV1 converts AV1 ES video samples to fmp4.Sample format.
// AV1 uses OBU (Open Bitstream Unit) format, not NAL units, so no conversion is needed.
func ConvertESSamplesToFMP4VideoAV1(samples []ESSample, timescale uint32) ([]*fmp4.Sample, uint64) {
	if len(samples) == 0 {
		return nil, 0
	}

	fmp4Samples := make([]*fmp4.Sample, 0, len(samples))
	baseTime := uint64(samples[0].DTS)

	for i, sample := range samples {
		// Calculate duration (estimate from next sample or default)
		var duration uint32
		if i+1 < len(samples) {
			duration = uint32(samples[i+1].DTS - sample.DTS)
		} else if i > 0 {
			// Use average of previous samples
			duration = uint32(sample.DTS-samples[0].DTS) / uint32(i)
		} else {
			// Default to 1/30 second at 90kHz
			duration = timescale / 30
		}

		if duration == 0 {
			duration = timescale / 30
		}

		// Calculate PTS offset (composition time offset)
		ptsOffset := int32(sample.PTS - sample.DTS)

		// AV1 data is already in OBU format - pass through directly
		// No Annex B conversion needed
		fmp4Samples = append(fmp4Samples, &fmp4.Sample{
			Duration:        duration,
			PTSOffset:       ptsOffset,
			IsNonSyncSample: !sample.IsKeyframe,
			Payload:         sample.Data,
		})
	}

	return fmp4Samples, baseTime
}

// ConvertESSamplesToFMP4VideoVP9 converts VP9 ES video samples to fmp4.Sample format.
// VP9 frames are passed through directly without any conversion.
func ConvertESSamplesToFMP4VideoVP9(samples []ESSample, timescale uint32) ([]*fmp4.Sample, uint64) {
	if len(samples) == 0 {
		return nil, 0
	}

	fmp4Samples := make([]*fmp4.Sample, 0, len(samples))
	baseTime := uint64(samples[0].DTS)

	for i, sample := range samples {
		// Calculate duration (estimate from next sample or default)
		var duration uint32
		if i+1 < len(samples) {
			duration = uint32(samples[i+1].DTS - sample.DTS)
		} else if i > 0 {
			// Use average of previous samples
			duration = uint32(sample.DTS-samples[0].DTS) / uint32(i)
		} else {
			// Default to 1/30 second at 90kHz
			duration = timescale / 30
		}

		if duration == 0 {
			duration = timescale / 30
		}

		// Calculate PTS offset (composition time offset)
		ptsOffset := int32(sample.PTS - sample.DTS)

		// VP9 data is passed through directly - no conversion needed
		fmp4Samples = append(fmp4Samples, &fmp4.Sample{
			Duration:        duration,
			PTSOffset:       ptsOffset,
			IsNonSyncSample: !sample.IsKeyframe,
			Payload:         sample.Data,
		})
	}

	return fmp4Samples, baseTime
}

// ConvertESSamplesToFMP4Audio converts ES audio samples to fmp4.Sample format.
func ConvertESSamplesToFMP4Audio(samples []ESSample, timescale uint32, sampleRate int) ([]*fmp4.Sample, uint64) {
	if len(samples) == 0 {
		return nil, 0
	}

	fmp4Samples := make([]*fmp4.Sample, 0, len(samples))
	baseTime := uint64(samples[0].PTS)

	// Default duration for AAC (1024 samples per frame)
	defaultDuration := uint32(1024 * timescale / uint32(sampleRate))
	if defaultDuration == 0 {
		defaultDuration = 1024 // fallback
	}

	for i, sample := range samples {
		// Calculate duration
		var duration uint32
		if i+1 < len(samples) {
			duration = uint32(samples[i+1].PTS - sample.PTS)
		} else {
			duration = defaultDuration
		}

		if duration == 0 {
			duration = defaultDuration
		}

		// Strip ADTS header if present
		payload := stripADTSHeader(sample.Data)

		fmp4Samples = append(fmp4Samples, &fmp4.Sample{
			Duration: duration,
			Payload:  payload,
		})
	}

	return fmp4Samples, baseTime
}

// stripADTSHeader removes ADTS header from AAC frame if present.
func stripADTSHeader(data []byte) []byte {
	if len(data) < 7 {
		return data
	}

	// Check for ADTS sync word
	if data[0] != 0xFF || (data[1]&0xF0) != 0xF0 {
		return data
	}

	// Parse ADTS header
	protectionAbsent := (data[1] & 0x01) != 0
	headerSize := 7
	if !protectionAbsent {
		headerSize = 9 // CRC present
	}

	if len(data) <= headerSize {
		return data
	}

	return data[headerSize:]
}

// convertAnnexBToAVCC converts video data from Annex B format (start codes) to AVCC format (length-prefixed).
// This is required for fMP4/MP4 containers which use AVCC/HVCC format.
// The function works for both H.264 and H.265 as they use the same NAL unit structure.
func convertAnnexBToAVCC(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// Check if already in AVCC format (no start codes)
	if len(data) >= 4 && !(data[0] == 0x00 && data[1] == 0x00 && (data[2] == 0x01 || (data[2] == 0x00 && data[3] == 0x01))) {
		// Doesn't start with a start code - check if it looks like length-prefixed
		// Read first 4 bytes as big-endian length
		length := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		if length > 0 && int(length)+4 <= len(data) {
			// Looks like valid AVCC format already
			return data
		}
	}

	// Parse Annex B to extract NAL units
	var annexB h264.AnnexB
	if err := annexB.Unmarshal(data); err != nil {
		// Fallback: return as-is if we can't parse
		// Log the error for debugging
		slog.Debug("convertAnnexBToAVCC: failed to parse Annex B",
			slog.String("error", err.Error()),
			slog.Int("data_len", len(data)))
		return data
	}

	if len(annexB) == 0 {
		slog.Debug("convertAnnexBToAVCC: empty NAL units after parsing",
			slog.Int("data_len", len(data)))
		return data
	}

	// Convert to AVCC format (4-byte length prefix per NAL)
	avcc, err := h264.AVCC(annexB).Marshal()
	if err != nil {
		// Fallback: return as-is
		slog.Debug("convertAnnexBToAVCC: failed to marshal AVCC",
			slog.String("error", err.Error()),
			slog.Int("nal_count", len(annexB)))
		return data
	}

	return avcc
}

// ESSampleAdapterConfig configures the ES to fMP4 adapter.
type ESSampleAdapterConfig struct {
	VideoTimescale uint32
	AudioTimescale uint32
}

// DefaultESSampleAdapterConfig returns default configuration.
func DefaultESSampleAdapterConfig() ESSampleAdapterConfig {
	return ESSampleAdapterConfig{
		VideoTimescale: 90000, // Match PTS timescale
		AudioTimescale: 90000, // Match PTS timescale
	}
}

// ESSampleAdapter adapts ESSamples for use with FMP4Writer.
type ESSampleAdapter struct {
	config       ESSampleAdapterConfig
	videoParams  *VideoCodecParams
	audioParams  *AudioCodecParams
	paramsLocked bool

	// Video parameter set helper for ensuring VPS/SPS/PPS are present on keyframes
	videoParamHelper *VideoParamHelper
}

// NewESSampleAdapter creates a new adapter.
func NewESSampleAdapter(config ESSampleAdapterConfig) *ESSampleAdapter {
	return &ESSampleAdapter{
		config:           config,
		videoParamHelper: NewVideoParamHelper(),
	}
}

// UpdateVideoParams updates video codec params from samples.
// Only updates if params haven't been locked.
// Also extracts and stores VPS/SPS/PPS for prepending to keyframes (H.264/H.265).
func (a *ESSampleAdapter) UpdateVideoParams(samples []ESSample) bool {
	// Always extract parameter sets for the helper (even if main params are locked)
	// This ensures we have the latest VPS/SPS/PPS for prepending to keyframes
	// Note: This only applies to H.264/H.265, not AV1/VP9
	for _, sample := range samples {
		isH265 := a.videoParams != nil && a.videoParams.Codec == "h265"
		// Skip NAL extraction for AV1/VP9 codecs
		if a.videoParams == nil || (a.videoParams.Codec != "av1" && a.videoParams.Codec != "vp9") {
			a.videoParamHelper.ExtractFromAnnexB(sample.Data, isH265)
		}
	}

	if a.paramsLocked && a.videoParams != nil {
		return false
	}

	params := ExtractVideoCodecParams(samples)
	if params != nil && (params.H264SPS != nil || params.H265SPS != nil || params.AV1SequenceHeader != nil || params.VP9Width > 0) {
		prevCodec := ""
		if a.videoParams != nil {
			prevCodec = a.videoParams.Codec
		}
		a.videoParams = params

		// Also set params in the helper for use when prepending (H.264/H.265 only)
		if params.Codec == "h265" {
			a.videoParamHelper.SetH265Params(params.H265VPS, params.H265SPS, params.H265PPS)
		} else if params.Codec == "h264" {
			a.videoParamHelper.SetH264Params(params.H264SPS, params.H264PPS)
		}
		// AV1/VP9 don't need parameter prepending - codec config is in the bitstream

		// Log codec detection for debugging H.265 issues
		if params.Codec != prevCodec || params.Codec == "h265" {
			// Can't use slog here directly, but we could add a debug flag
			// For now, just silently track the codec change
		}
		return true
	}
	return false
}

// UpdateAudioParams updates audio codec params from samples.
// Only updates if params haven't been locked.
// For AAC, extracts detailed config from ADTS headers.
// For other codecs (Opus, AC3, MP3), uses the preset codec info.
func (a *ESSampleAdapter) UpdateAudioParams(samples []ESSample) bool {
	if a.paramsLocked && a.audioParams != nil {
		return false
	}

	// If we already have audio params set (e.g., from SetAudioCodecFromVariant),
	// and it's not AAC, don't try to extract from samples since we can't detect
	// Opus/AC3/MP3 from raw ES samples.
	if a.audioParams != nil && a.audioParams.Codec != "" && a.audioParams.Codec != "aac" {
		return false
	}

	params := ExtractAudioCodecParams(samples)
	if params != nil {
		// If we already have a codec set from variant, preserve it
		if a.audioParams != nil && a.audioParams.Codec != "" {
			params.Codec = a.audioParams.Codec
		}
		a.audioParams = params
		return true
	}
	return false
}

// SetAudioCodecFromVariant sets the audio codec params based on the variant.
// This should be called before processing samples to ensure correct codec detection.
// For non-AAC codecs (Opus, AC3, MP3), this is essential since we can't detect
// them from ES samples like we can with AAC's ADTS headers.
func (a *ESSampleAdapter) SetAudioCodecFromVariant(audioCodec string) {
	if a.paramsLocked && a.audioParams != nil {
		return
	}
	a.audioParams = NewAudioCodecParamsFromCodec(audioCodec)
}

// LockParams prevents further parameter updates.
func (a *ESSampleAdapter) LockParams() {
	a.paramsLocked = true
}

// VideoParams returns the current video params.
func (a *ESSampleAdapter) VideoParams() *VideoCodecParams {
	return a.videoParams
}

// AudioParams returns the current audio params.
func (a *ESSampleAdapter) AudioParams() *AudioCodecParams {
	return a.audioParams
}

// ConvertVideoSamples converts ES video samples to fmp4 format.
// For H.264/H.265 keyframes, it prepends VPS/SPS/PPS if not already present.
// For AV1/VP9, samples are passed through directly (OBU/raw format).
func (a *ESSampleAdapter) ConvertVideoSamples(samples []ESSample) ([]*fmp4.Sample, uint64) {
	// AV1 uses OBU format, not NAL units - no conversion needed
	if a.videoParams != nil && a.videoParams.Codec == "av1" {
		return ConvertESSamplesToFMP4VideoAV1(samples, a.config.VideoTimescale)
	}

	// VP9 frames are passed through directly - no conversion needed
	if a.videoParams != nil && a.videoParams.Codec == "vp9" {
		return ConvertESSamplesToFMP4VideoVP9(samples, a.config.VideoTimescale)
	}

	// H.264/H.265: Prepend VPS/SPS/PPS to keyframes before conversion
	isH265 := a.videoParams != nil && a.videoParams.Codec == "h265"
	processedSamples := make([]ESSample, len(samples))

	for i, sample := range samples {
		processedSamples[i] = sample
		if sample.IsKeyframe {
			// Prepend parameter sets to keyframe data if not already present
			processedSamples[i].Data = a.videoParamHelper.PrependParamsToKeyframeAnnexB(sample.Data, isH265)
		}
	}

	return ConvertESSamplesToFMP4Video(processedSamples, a.config.VideoTimescale)
}

// ConvertAudioSamples converts ES audio samples to fmp4 format.
func (a *ESSampleAdapter) ConvertAudioSamples(samples []ESSample) ([]*fmp4.Sample, uint64) {
	sampleRate := 48000
	if a.audioParams != nil && a.audioParams.AACConfig != nil {
		sampleRate = a.audioParams.AACConfig.SampleRate
	}
	return ConvertESSamplesToFMP4Audio(samples, a.config.AudioTimescale, sampleRate)
}

// ConfigureWriter configures an FMP4Writer with the extracted codec params.
func (a *ESSampleAdapter) ConfigureWriter(writer *FMP4Writer) error {
	if a.videoParams != nil {
		switch a.videoParams.Codec {
		case "h264":
			if a.videoParams.H264SPS != nil && a.videoParams.H264PPS != nil {
				writer.SetH264Params(a.videoParams.H264SPS, a.videoParams.H264PPS)
			}
		case "h265":
			if a.videoParams.H265SPS != nil && a.videoParams.H265PPS != nil {
				writer.SetH265Params(a.videoParams.H265VPS, a.videoParams.H265SPS, a.videoParams.H265PPS)
			}
		case "av1":
			if a.videoParams.AV1SequenceHeader != nil {
				writer.SetAV1Params(a.videoParams.AV1SequenceHeader)
			}
		case "vp9":
			if a.videoParams.VP9Width > 0 && a.videoParams.VP9Height > 0 {
				writer.SetVP9Params(
					a.videoParams.VP9Width,
					a.videoParams.VP9Height,
					a.videoParams.VP9Profile,
					a.videoParams.VP9BitDepth,
					a.videoParams.VP9ChromaSubsampling,
					a.videoParams.VP9ColorRange,
				)
			}
		}
	}

	if a.audioParams != nil {
		switch a.audioParams.Codec {
		case "aac":
			if a.audioParams.AACConfig != nil {
				writer.SetAACConfig(a.audioParams.AACConfig)
			}
		case "opus":
			writer.SetOpusConfig(a.audioParams.OpusChannelCount)
		case "ac3":
			writer.SetAC3Config(a.audioParams.AC3SampleRate, a.audioParams.AC3ChannelCount)
		case "eac3":
			writer.SetEAC3Config(a.audioParams.AC3SampleRate, a.audioParams.AC3ChannelCount)
		case "mp3":
			writer.SetMP3Config(a.audioParams.MP3SampleRate, a.audioParams.MP3ChannelCount)
		}
	}

	return nil
}
