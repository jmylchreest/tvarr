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
	Logger     *slog.Logger
	VideoCodec string // "h264", "h265"
	AudioCodec string // "aac", "ac3", "eac3", "mp3", "opus"
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
		writer:      w,
		config:      config,
		videoCodec:  config.VideoCodec,
		audioCodec:  config.AudioCodec,
		videoParams: NewVideoParamHelper(),
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
		audioCodec, normalizedName := createAudioCodec(m.audioCodec, nil)
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
	// This ensures decoders can always decode keyframes even after buffer eviction
	if isKeyframe {
		au = m.videoParams.PrependParamsToKeyframeNALUs(au, isH265)
	}

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
