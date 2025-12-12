// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// VideoCodecParams holds extracted video codec parameters.
type VideoCodecParams struct {
	Codec string // "h264" or "h265"

	// H.264 parameters
	H264SPS []byte
	H264PPS []byte

	// H.265 parameters
	H265VPS []byte
	H265SPS []byte
	H265PPS []byte
}

// AudioCodecParams holds extracted audio codec parameters.
type AudioCodecParams struct {
	Codec     string // "aac", "ac3", etc.
	AACConfig *mpeg4audio.Config
}

// ExtractVideoCodecParams extracts video codec parameters from ES samples.
// It looks for SPS/PPS NAL units in the first keyframe sample.
func ExtractVideoCodecParams(samples []ESSample) *VideoCodecParams {
	for _, sample := range samples {
		if !sample.IsKeyframe {
			continue
		}

		// Parse NAL units from sample data
		nalUnits := extractNALUnitsFromData(sample.Data)
		if len(nalUnits) == 0 {
			continue
		}

		params := &VideoCodecParams{}

		for _, nal := range nalUnits {
			if len(nal) == 0 {
				continue
			}

			// Try to detect codec from NAL unit type using mediacommon constants
			h264NalType := h264.NALUType(nal[0] & 0x1F)

			// Check for H.264 NAL types
			switch h264NalType {
			case h264.NALUTypeSPS:
				params.Codec = "h264"
				params.H264SPS = nal
			case h264.NALUTypePPS:
				params.Codec = "h264"
				params.H264PPS = nal
			}

			// Check for H.265 NAL types (different format: (nal[0] >> 1) & 0x3F)
			// Only check if not already detected as H.264
			if (params.Codec == "" || params.Codec == "h265") && len(nal) >= 2 {
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
			AACConfig: &mpeg4audio.Config{
				Type:         mpeg4audio.ObjectTypeAACLC,
				SampleRate:   48000,
				ChannelCount: 2,
			},
		}
	}

	// Default AAC config
	return &AudioCodecParams{
		Codec: "aac",
		AACConfig: &mpeg4audio.Config{
			Type:         mpeg4audio.ObjectTypeAACLC,
			SampleRate:   48000,
			ChannelCount: 2,
		},
	}
}

// parseADTSHeader extracts MPEG-4 Audio config from an ADTS header.
func parseADTSHeader(data []byte) *mpeg4audio.Config {
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

	return &mpeg4audio.Config{
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

// extractNALUnitsLengthPrefixed extracts NAL units from length-prefixed format using mediacommon.
// This is a fallback for formats not handled by AVCC.Unmarshal.
func extractNALUnitsLengthPrefixed(data []byte, lengthSize int) [][]byte {
	// For 4-byte length prefix, use mediacommon's AVCC
	if lengthSize == 4 {
		var au h264.AVCC
		if err := au.Unmarshal(data); err == nil {
			return au
		}
	}

	// Manual fallback for non-standard length sizes
	var units [][]byte
	offset := 0

	for offset+lengthSize <= len(data) {
		var nalLen int
		switch lengthSize {
		case 4:
			nalLen = int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		case 2:
			nalLen = int(data[offset])<<8 | int(data[offset+1])
		case 1:
			nalLen = int(data[offset])
		default:
			return units
		}

		offset += lengthSize

		if nalLen <= 0 || offset+nalLen > len(data) {
			break
		}

		units = append(units, data[offset:offset+nalLen])
		offset += nalLen
	}

	return units
}

// ConvertESSamplesToFMP4Video converts ES video samples to fmp4.Sample format.
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

		// Convert NAL units to Annex B format for fmp4
		payload := convertToAnnexB(sample.Data)

		fmp4Samples = append(fmp4Samples, &fmp4.Sample{
			Duration:        duration,
			PTSOffset:       ptsOffset,
			IsNonSyncSample: !sample.IsKeyframe,
			Payload:         payload,
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

// convertToAnnexB ensures video data is in Annex B format using mediacommon.
func convertToAnnexB(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	// Check if already Annex B format
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 {
		if data[2] == 0x01 || (data[2] == 0x00 && data[3] == 0x01) {
			return data
		}
	}

	// Try to parse as AVCC and convert to Annex B using mediacommon
	if len(data) >= 4 {
		var avcc h264.AVCC
		if err := avcc.Unmarshal(data); err == nil && len(avcc) > 0 {
			// Convert to Annex B using mediacommon
			annexB, err := h264.AnnexB(avcc).Marshal()
			if err == nil {
				return annexB
			}
		}
	}

	// Assume raw NAL unit, convert to Annex B using mediacommon
	annexB, err := h264.AnnexB([][]byte{data}).Marshal()
	if err != nil {
		// Fallback: manually add start code
		result := make([]byte, len(data)+4)
		result[0] = 0x00
		result[1] = 0x00
		result[2] = 0x00
		result[3] = 0x01
		copy(result[4:], data)
		return result
	}
	return annexB
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
// Also extracts and stores VPS/SPS/PPS for prepending to keyframes.
func (a *ESSampleAdapter) UpdateVideoParams(samples []ESSample) bool {
	// Always extract parameter sets for the helper (even if main params are locked)
	// This ensures we have the latest VPS/SPS/PPS for prepending to keyframes
	for _, sample := range samples {
		isH265 := a.videoParams != nil && a.videoParams.Codec == "h265"
		a.videoParamHelper.ExtractFromAnnexB(sample.Data, isH265)
	}

	if a.paramsLocked && a.videoParams != nil {
		return false
	}

	params := ExtractVideoCodecParams(samples)
	if params != nil && (params.H264SPS != nil || params.H265SPS != nil) {
		a.videoParams = params

		// Also set params in the helper for use when prepending
		if params.Codec == "h265" {
			a.videoParamHelper.SetH265Params(params.H265VPS, params.H265SPS, params.H265PPS)
		} else {
			a.videoParamHelper.SetH264Params(params.H264SPS, params.H264PPS)
		}
		return true
	}
	return false
}

// UpdateAudioParams updates audio codec params from samples.
// Only updates if params haven't been locked.
func (a *ESSampleAdapter) UpdateAudioParams(samples []ESSample) bool {
	if a.paramsLocked && a.audioParams != nil {
		return false
	}

	params := ExtractAudioCodecParams(samples)
	if params != nil {
		a.audioParams = params
		return true
	}
	return false
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
// For keyframes, it prepends VPS/SPS/PPS if not already present.
func (a *ESSampleAdapter) ConvertVideoSamples(samples []ESSample) ([]*fmp4.Sample, uint64) {
	// Prepend VPS/SPS/PPS to keyframes before conversion
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
		}
	}

	if a.audioParams != nil && a.audioParams.AACConfig != nil {
		writer.SetAACConfig(a.audioParams.AACConfig)
	}

	return nil
}

// GetSPSInfo parses H.264 SPS to extract video dimensions.
func GetSPSInfo(sps []byte) (width, height int, err error) {
	if len(sps) == 0 {
		return 0, 0, nil
	}

	var spsp h264.SPS
	if err := spsp.Unmarshal(sps); err != nil {
		return 0, 0, err
	}

	return spsp.Width(), spsp.Height(), nil
}

// GetH265SPSInfo parses H.265 SPS to extract video dimensions.
func GetH265SPSInfo(sps []byte) (width, height int, err error) {
	if len(sps) == 0 {
		return 0, 0, nil
	}

	var spsp h265.SPS
	if err := spsp.Unmarshal(sps); err != nil {
		return 0, 0, err
	}

	return spsp.Width(), spsp.Height(), nil
}
