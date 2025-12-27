// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

// TSMuxerConfig configures the TS muxer for the daemon.
type TSMuxerConfig struct {
	Logger        *slog.Logger
	VideoCodec    string // "h264", "h265"
	AudioCodec    string // "aac", "ac3", "eac3", "mp3", "opus"
	AudioInitData []byte // AudioSpecificConfig for AAC (used to set correct ADTS parameters)
}

// TSMuxer muxes elementary streams into MPEG-TS format for FFmpeg input.
type TSMuxer struct {
	writer io.Writer
	config TSMuxerConfig

	// mediacommon writer
	muxer *mpegts.Writer

	// Track references
	videoTrack *mpegts.Track
	audioTrack *mpegts.Track

	// Track codec types
	videoCodec string
	audioCodec string

	// Audio configuration parsed from init data
	audioInitData []byte

	// Video parameter set helper for ensuring VPS/SPS/PPS are present on keyframes
	videoParams *VideoParamHelper

	// Initialization state
	mu          sync.Mutex
	initialized bool
	tracks      []*mpegts.Track
}

// PID constants for MPEG-TS.
const (
	tsVideoPID = 0x0100
	tsAudioPID = 0x0101
)

// NewTSMuxer creates a new MPEG-TS muxer for daemon use.
func NewTSMuxer(w io.Writer, config TSMuxerConfig) *TSMuxer {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.VideoCodec == "" {
		config.VideoCodec = "h264" // Default to H.264
	}

	return &TSMuxer{
		writer:        w,
		config:        config,
		videoCodec:    config.VideoCodec,
		audioCodec:    config.AudioCodec,
		audioInitData: config.AudioInitData,
		videoParams:   NewVideoParamHelper(),
	}
}

// initialize creates the mediacommon writer with configured tracks.
func (m *TSMuxer) initialize() error {
	if m.initialized {
		return nil
	}

	// Create video track
	m.videoTrack = &mpegts.Track{
		PID:   tsVideoPID,
		Codec: createVideoCodec(m.videoCodec),
	}
	m.tracks = append(m.tracks, m.videoTrack)

	// Create audio track if configured
	if m.audioCodec != "" {
		// Parse AudioSpecificConfig from init data if available
		var aacConfig *mpeg4audio.AudioSpecificConfig
		if len(m.audioInitData) > 0 && (m.audioCodec == "aac" || m.audioCodec == "") {
			var cfg mpeg4audio.AudioSpecificConfig
			if err := cfg.Unmarshal(m.audioInitData); err == nil {
				aacConfig = &cfg
				m.config.Logger.Info("Parsed AAC config from init data",
					slog.Int("sample_rate", cfg.SampleRate),
					slog.Int("channels", cfg.ChannelCount),
					slog.Int("object_type", int(cfg.Type)))
			} else {
				m.config.Logger.Warn("Failed to parse AAC config from init data, using defaults",
					slog.String("error", err.Error()))
			}
		} else if m.audioCodec == "aac" {
			m.config.Logger.Warn("No AAC init data provided, using hardcoded defaults (48kHz, stereo)")
		}

		audioCodec, normalizedName := createAudioCodec(m.audioCodec, aacConfig)
		m.audioCodec = normalizedName
		m.audioTrack = &mpegts.Track{
			PID:   tsAudioPID,
			Codec: audioCodec,
		}
		m.tracks = append(m.tracks, m.audioTrack)
	}

	// Create the mediacommon writer
	m.muxer = &mpegts.Writer{
		W:      m.writer,
		Tracks: m.tracks,
	}

	if err := m.muxer.Initialize(); err != nil {
		return fmt.Errorf("initializing mpegts writer: %w", err)
	}

	// Write PAT/PMT tables immediately after initialization
	// This ensures FFmpeg can detect codec parameters at stream start
	if _, err := m.muxer.WriteTables(); err != nil {
		return fmt.Errorf("writing PAT/PMT tables: %w", err)
	}

	m.initialized = true
	m.config.Logger.Debug("MPEG-TS muxer initialized",
		slog.String("video_codec", m.videoCodec),
		slog.String("audio_codec", m.audioCodec))

	return nil
}

// createVideoCodec creates a mediacommon video codec from a codec name.
func createVideoCodec(codecName string) mpegts.Codec {
	switch codecName {
	case "h265", "hevc":
		return &mpegts.CodecH265{}
	default:
		return &mpegts.CodecH264{}
	}
}

// createAudioCodec creates a mediacommon audio codec from a codec name.
// Returns the codec and the normalized codec name.
func createAudioCodec(codecName string, aacConfig *mpeg4audio.AudioSpecificConfig) (mpegts.Codec, string) {
	switch codecName {
	case "ac3":
		return &mpegts.CodecAC3{SampleRate: 48000, ChannelCount: 2}, "ac3"
	case "eac3", "ec-3", "ec3":
		return &mpegts.CodecEAC3{SampleRate: 48000, ChannelCount: 6}, "eac3"
	case "mp3":
		return &mpegts.CodecMPEG1Audio{}, "mp3"
	case "opus":
		return &mpegts.CodecOpus{ChannelCount: 2}, "opus"
	default:
		// Default to AAC
		if aacConfig == nil {
			aacConfig = &mpeg4audio.AudioSpecificConfig{
				Type:         mpeg4audio.ObjectTypeAACLC,
				SampleRate:   48000,
				ChannelCount: 2,
			}
		}
		return &mpegts.CodecMPEG4Audio{Config: *aacConfig}, "aac"
	}
}

// WriteVideo writes a video access unit (NAL unit with or without start codes).
func (m *TSMuxer) WriteVideo(pts, dts int64, data []byte, isKeyframe bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize on first write
	if !m.initialized {
		if err := m.initialize(); err != nil {
			return err
		}
	}

	// Convert data to access unit format (slice of NAL units)
	au := dataToAccessUnit(data)
	if len(au) == 0 {
		return nil
	}

	// Determine if H.265 based on track's codec type
	_, isH265 := m.videoTrack.Codec.(*mpegts.CodecH265)
	m.videoParams.ExtractFromNALUs(au, isH265)

	// For keyframes, prepend VPS/SPS/PPS (H.265) or SPS/PPS (H.264) if not already present
	// Use ForceKeyframePrependNALUs since the caller (gRPC message) already indicates
	// this is a keyframe - we don't need to verify by checking NAL types
	if isKeyframe {
		au = m.videoParams.ForceKeyframePrependNALUs(au, isH265)
	}

	// Reorder NAL units to ensure SPS/PPS come before SEI and other NALs.
	// Many IPTV sources have NAL order like: SEI, SEI, SPS, PPS, IDR
	// But FFmpeg's decoder expects SPS/PPS before SEI (since SEI may reference SPS).
	// The correct order for decoders is: SPS, PPS, SEI, IDR (for H.264)
	// or: VPS, SPS, PPS, SEI, IDR (for H.265)
	au = reorderNALUnits(au, isH265)

	// Write based on track's codec type
	return m.writeVideoByCodecType(pts, dts, au)
}

// writeVideoByCodecType dispatches video writes based on the track's codec type.
func (m *TSMuxer) writeVideoByCodecType(pts, dts int64, au [][]byte) error {
	switch m.videoTrack.Codec.(type) {
	case *mpegts.CodecH265:
		return m.muxer.WriteH265(m.videoTrack, pts, dts, au)
	case *mpegts.CodecH264:
		return m.muxer.WriteH264(m.videoTrack, pts, dts, au)
	default:
		// Fallback to H.264
		return m.muxer.WriteH264(m.videoTrack, pts, dts, au)
	}
}

// WriteAudio writes an audio frame.
func (m *TSMuxer) WriteAudio(pts int64, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize on first write
	if !m.initialized {
		if err := m.initialize(); err != nil {
			return err
		}
	}

	if len(data) == 0 || m.audioTrack == nil {
		return nil
	}

	return m.writeAudioByCodecType(pts, data)
}

// writeAudioByCodecType dispatches audio writes based on the track's codec type.
func (m *TSMuxer) writeAudioByCodecType(pts int64, data []byte) error {
	switch m.audioTrack.Codec.(type) {
	case *mpegts.CodecMPEG4Audio:
		// For AAC, data may be ADTS framed or raw - mediacommon expects raw AUs
		aus := extractAACFrames(data)
		if len(aus) == 0 {
			return nil
		}
		return m.muxer.WriteMPEG4Audio(m.audioTrack, pts, aus)
	case *mpegts.CodecAC3:
		return m.muxer.WriteAC3(m.audioTrack, pts, data)
	case *mpegts.CodecEAC3:
		return m.muxer.WriteEAC3(m.audioTrack, pts, data)
	case *mpegts.CodecMPEG1Audio:
		frames := [][]byte{data}
		return m.muxer.WriteMPEG1Audio(m.audioTrack, pts, frames)
	case *mpegts.CodecOpus:
		packets := [][]byte{data}
		return m.muxer.WriteOpus(m.audioTrack, pts, packets)
	default:
		// Fallback to AAC for unknown codecs
		aus := extractAACFrames(data)
		if len(aus) == 0 {
			return nil
		}
		return m.muxer.WriteMPEG4Audio(m.audioTrack, pts, aus)
	}
}

// Flush writes any pending data (no-op for mediacommon, kept for compatibility).
func (m *TSMuxer) Flush() error {
	// mediacommon handles PAT/PMT automatically
	return nil
}

// InitializeAndGetHeader initializes the muxer and returns the PAT/PMT header bytes.
// This can be used to send the header to FFmpeg before any media data.
func (m *TSMuxer) InitializeAndGetHeader() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return nil, nil // Already initialized
	}

	if err := m.initialize(); err != nil {
		return nil, err
	}

	// Return a copy of any bytes that were written during initialization
	// (PAT/PMT tables written by WriteTables in initialize())
	return nil, nil // The bytes are already in the writer (m.writer)
}

// Format returns the FFmpeg input format string for MPEG-TS.
func (m *TSMuxer) Format() string {
	return "mpegts"
}

// Ensure TSMuxer implements InputMuxer interface.
var _ InputMuxer = (*TSMuxer)(nil)

// reorderNALUnits reorders NAL units to ensure parameter sets come first.
// This fixes issues where IPTV sources send NALs in wrong order (e.g., SEI before SPS/PPS).
// FFmpeg's decoder expects: VPS, SPS, PPS, AUD, SEI, slices (for H.265)
// or: SPS, PPS, AUD, SEI, slices (for H.264)
func reorderNALUnits(nalus [][]byte, isH265 bool) [][]byte {
	if len(nalus) <= 1 {
		return nalus
	}

	// Categorize NAL units
	var paramSets [][]byte // VPS, SPS, PPS
	var audNALs [][]byte   // Access Unit Delimiter
	var seiNALs [][]byte   // Supplemental Enhancement Information
	var sliceNALs [][]byte // Picture data (IDR, non-IDR slices)
	var otherNALs [][]byte // Everything else

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
	result = append(result, audNALs...)   // AUD first (if present)
	result = append(result, paramSets...) // VPS/SPS/PPS
	result = append(result, seiNALs...)   // SEI (now after SPS/PPS so references work)
	result = append(result, sliceNALs...) // IDR/slice data
	result = append(result, otherNALs...) // Anything else

	return result
}

// dataToAccessUnit converts raw video data to access unit format using mediacommon.
// It handles both Annex B format (with start codes) and raw NAL units.
func dataToAccessUnit(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}

	// Check if data is in Annex B format (starts with start code)
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 {
		if (data[2] == 0x01) || (data[2] == 0x00 && len(data) >= 4 && data[3] == 0x01) {
			// Annex B format - use mediacommon to extract NAL units
			var au h264.AnnexB
			if err := au.Unmarshal(data); err != nil {
				// Fallback: treat as single NAL unit
				return [][]byte{data}
			}
			return au
		}
	}

	// Raw NAL unit (no start code) - wrap in slice
	return [][]byte{data}
}

// extractAACFrames extracts AAC frames from potentially ADTS-framed data.
func extractAACFrames(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}

	// Check for ADTS sync word (0xFFF)
	if len(data) >= 7 && data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
		return extractADTSFrames(data)
	}

	// Raw AAC frame
	return [][]byte{data}
}

// extractADTSFrames extracts raw AAC frames from ADTS container.
func extractADTSFrames(data []byte) [][]byte {
	var frames [][]byte
	offset := 0

	for offset+7 <= len(data) {
		// Check ADTS sync word
		if data[offset] != 0xFF || (data[offset+1]&0xF0) != 0xF0 {
			offset++
			continue
		}

		// Parse ADTS header
		protectionAbsent := (data[offset+1] & 0x01) != 0
		headerSize := 7
		if !protectionAbsent {
			headerSize = 9 // CRC present
		}

		// Frame length (13 bits)
		frameLen := int(data[offset+3]&0x03)<<11 |
			int(data[offset+4])<<3 |
			int(data[offset+5]>>5)

		if frameLen < headerSize || offset+frameLen > len(data) {
			break
		}

		// Extract raw AAC frame (without ADTS header)
		rawFrame := data[offset+headerSize : offset+frameLen]
		if len(rawFrame) > 0 {
			frames = append(frames, rawFrame)
		}

		offset += frameLen
	}

	return frames
}
