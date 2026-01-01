// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// SwappableWriter is an io.Writer that can be redirected to different underlying buffers.
// This allows a single TSMuxer to write to different segment buffers while maintaining
// continuity counters across segments.
type SwappableWriter struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

// NewSwappableWriter creates a new SwappableWriter with an initial buffer.
func NewSwappableWriter(buf *bytes.Buffer) *SwappableWriter {
	return &SwappableWriter{buf: buf}
}

// Write implements io.Writer.
func (w *SwappableWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf == nil {
		return 0, io.ErrClosedPipe
	}
	return w.buf.Write(p)
}

// SetBuffer switches the underlying buffer.
func (w *SwappableWriter) SetBuffer(buf *bytes.Buffer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = buf
}

// MPEG-TS constants.
const (
	// TSPacketSize is the standard MPEG-TS packet size.
	TSPacketSize = 188
	// TSSyncByte is the MPEG-TS sync byte.
	TSSyncByte = 0x47

	TSPATStreamID  = 0x00
	TSPMTProgramID = 0x1000
	TSVideoPID     = 0x0100
	TSAudioPID     = 0x0101
	TSPCRPid       = TSVideoPID

	// Stream types - use codec package constants
	StreamTypeH264 = codec.StreamTypeH264
	StreamTypeH265 = codec.StreamTypeH265
	StreamTypeAAC  = codec.StreamTypeAAC
	StreamTypeAC3  = codec.StreamTypeAC3
	StreamTypeEAC3 = codec.StreamTypeEAC3
	StreamTypeMP3  = codec.StreamTypeMP3
)

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

// TSMuxerConfig configures the TS muxer.
type TSMuxerConfig struct {
	VideoPID uint16
	AudioPID uint16
	Logger   *slog.Logger

	// Codec configuration (optional, will be auto-detected if not set)
	VideoCodec string // "h264", "h265"
	AudioCodec string // "aac", "ac3", "mp3", "opus"

	// AAC configuration (required for AAC audio)
	AACConfig *mpeg4audio.AudioSpecificConfig

	// VideoParams is an optional shared VideoParamHelper for persistent SPS/PPS across segments.
	// If nil, a new one will be created.
	VideoParams *VideoParamHelper
}

// TSMuxer muxes elementary streams into MPEG-TS format using mediacommon.
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

// NewTSMuxer creates a new MPEG-TS muxer backed by mediacommon.
func NewTSMuxer(w io.Writer, config TSMuxerConfig) *TSMuxer {
	if config.VideoPID == 0 {
		config.VideoPID = TSVideoPID
	}
	if config.AudioPID == 0 {
		config.AudioPID = TSAudioPID
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.VideoCodec == "" {
		config.VideoCodec = "h264" // Default to H.264
	}
	// Note: AudioCodec is intentionally NOT defaulted here.
	// Empty audio codec means video-only stream.
	// The caller should explicitly pass the audio codec.
	// Previous behavior was to default to "aac" which caused issues
	// when the actual audio codec was different (e.g., E-AC3).

	// Use provided VideoParams or create a new one
	videoParams := config.VideoParams
	if videoParams == nil {
		videoParams = NewVideoParamHelper()
	}

	m := &TSMuxer{
		writer:      w,
		config:      config,
		videoCodec:  config.VideoCodec,
		audioCodec:  config.AudioCodec,
		videoParams: videoParams,
	}

	return m
}

// initialize creates the mediacommon writer with configured tracks.
func (m *TSMuxer) initialize() error {
	if m.initialized {
		return nil
	}

	// Create video track using helper
	m.videoTrack = &mpegts.Track{
		PID:   m.config.VideoPID,
		Codec: createVideoCodec(m.videoCodec),
	}
	m.tracks = append(m.tracks, m.videoTrack)

	// Create audio track using helper
	audioCodec, normalizedName := createAudioCodec(m.audioCodec, m.config.AACConfig)
	m.audioCodec = normalizedName // Normalize the codec name
	m.audioTrack = &mpegts.Track{
		PID:   m.config.AudioPID,
		Codec: audioCodec,
	}
	m.tracks = append(m.tracks, m.audioTrack)

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

// SetVideoStreamType sets the video stream type (for compatibility).
// This should be called before any Write operations.
func (m *TSMuxer) SetVideoStreamType(streamType uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch streamType {
	case StreamTypeH264:
		m.videoCodec = "h264"
	case StreamTypeH265:
		m.videoCodec = "h265"
	}
}

// SetAudioStreamType sets the audio stream type (for compatibility).
// This should be called before any Write operations.
func (m *TSMuxer) SetAudioStreamType(streamType uint8) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch streamType {
	case StreamTypeAAC:
		m.audioCodec = "aac"
	case StreamTypeAC3:
		m.audioCodec = "ac3"
	case StreamTypeEAC3:
		m.audioCodec = "eac3"
	case StreamTypeMP3:
		m.audioCodec = "mp3"
	}
}

// SetVideoCodec sets the video codec by name.
func (m *TSMuxer) SetVideoCodec(codec string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.videoCodec = codec
}

// SetAudioCodec sets the audio codec by name.
func (m *TSMuxer) SetAudioCodec(codec string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audioCodec = codec
}

// SetAACConfig sets the AAC configuration for audio.
func (m *TSMuxer) SetAACConfig(config *mpeg4audio.AudioSpecificConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.AACConfig = config
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

	if len(data) == 0 {
		return nil
	}

	// Write based on track's codec type - this is DRY as adding a new codec
	// only requires updating createAudioCodec() and adding the write case here
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

// Flush writes any pending PAT/PMT (no-op for mediacommon, kept for compatibility).
func (m *TSMuxer) Flush() error {
	// mediacommon handles PAT/PMT automatically
	return nil
}

// InitializeAndGetHeader forces initialization and returns the PAT/PMT header bytes.
// This is useful for sending PAT/PMT to clients that connect after the muxer is started.
// MPEG-TS demuxers require PAT/PMT tables to understand the stream structure.
//
// IMPORTANT: VLC's demuxer requires at least 3 consecutive TS packets to validate
// the sync pattern (0x47 every 188 bytes). PAT+PMT is only 2 packets (376 bytes),
// so we add null packets (PID 0x1FFF) to ensure clients can probe successfully.
func (m *TSMuxer) InitializeAndGetHeader() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		if err := m.initialize(); err != nil {
			return nil, err
		}
	}

	// Create a temporary buffer to capture PAT/PMT output
	var buf bytes.Buffer

	// Create a temporary muxer with the same track configuration to get PAT/PMT
	tempMuxer := &mpegts.Writer{
		W:      &buf,
		Tracks: m.tracks,
	}

	if err := tempMuxer.Initialize(); err != nil {
		return nil, fmt.Errorf("initializing temp muxer for PAT/PMT: %w", err)
	}

	// Explicitly write PAT/PMT tables using the mediacommon WriteTables() method
	if _, err := tempMuxer.WriteTables(); err != nil {
		return nil, fmt.Errorf("writing PAT/PMT tables: %w", err)
	}

	// Add null packets (PID 0x1FFF) to ensure demuxers can validate sync byte pattern.
	// Most demuxers need 3+ consecutive sync bytes (0x47) at 188-byte intervals.
	// PAT+PMT is only 2 packets (376 bytes), so we add 2 null packets for 4 total.
	patPmt := buf.Bytes()
	minPackets := 4 // PAT + PMT + 2 null = 752 bytes minimum
	currentPackets := len(patPmt) / TSPacketSize
	packetsNeeded := minPackets - currentPackets

	if packetsNeeded > 0 {
		// Create null packets for padding
		// Null packet: sync(0x47) + PID 0x1FFF + no adaptation + CC=0 + stuffing(0xFF)
		nullPacket := make([]byte, TSPacketSize)
		nullPacket[0] = TSSyncByte // 0x47
		nullPacket[1] = 0x1F       // PID high bits (0x1FFF = null)
		nullPacket[2] = 0xFF       // PID low bits
		nullPacket[3] = 0x10       // No adaptation field, payload only, CC=0
		for i := 4; i < TSPacketSize; i++ {
			nullPacket[i] = 0xFF // Stuffing bytes
		}

		// Append null packets
		result := make([]byte, len(patPmt)+packetsNeeded*TSPacketSize)
		copy(result, patPmt)
		for i := 0; i < packetsNeeded; i++ {
			copy(result[len(patPmt)+i*TSPacketSize:], nullPacket)
		}
		return result, nil
	}

	return patPmt, nil
}

// Reset resets the muxer state for reuse.
func (m *TSMuxer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initialized = false
	m.muxer = nil
	m.videoTrack = nil
	m.audioTrack = nil
	m.tracks = nil
	m.videoParams = NewVideoParamHelper() // Reset parameter sets
}

// dataToAccessUnit converts raw video data to access unit format using mediacommon.
// It handles Annex B format (with start codes), AVCC format (length-prefixed), and raw NAL units.
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

	// Try AVCC format (length-prefixed) using mediacommon
	// fMP4 outputs use AVCC format, so we need to handle this for HLS-TS muxing
	if len(data) >= 4 {
		var au h264.AVCC
		if err := au.Unmarshal(data); err == nil && len(au) > 0 {
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


// VideoTrack returns the video track.
func (m *TSMuxer) VideoTrack() *mpegts.Track {
	return m.videoTrack
}

// AudioTrack returns the audio track.
func (m *TSMuxer) AudioTrack() *mpegts.Track {
	return m.audioTrack
}

// Writer returns the underlying mediacommon writer.
func (m *TSMuxer) Writer() *mpegts.Writer {
	return m.muxer
}
