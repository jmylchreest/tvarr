// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
)

// FMP4MuxerConfig configures the fMP4 muxer for the daemon.
type FMP4MuxerConfig struct {
	Logger        *slog.Logger
	VideoCodec    string // "vp9", "av1", "h264", "h265"
	AudioCodec    string // "aac", "opus", "ac3"
	AudioInitData []byte // AudioSpecificConfig for AAC
}

// FMP4Muxer muxes elementary streams into fragmented MP4 format for FFmpeg input.
// This is used for VP9/AV1 sources which are not compatible with MPEG-TS.
type FMP4Muxer struct {
	writer io.Writer
	config FMP4MuxerConfig

	// Track configuration
	videoCodec string
	audioCodec string

	// Codec-specific initialization data
	videoInitData []byte // SPS/PPS for H.264/H.265, sequence header for AV1
	audioInitData []byte // AudioSpecificConfig for AAC

	// fMP4 state
	mu              sync.Mutex
	initialized     bool
	initWritten     bool
	videoTrackID    int
	audioTrackID    int
	videoTimeScale  uint32
	audioTimeScale  uint32
	sequenceNumber  uint32
	videoBaseTime   uint64
	audioBaseTime   uint64
	lastVideoPTS    int64
	lastAudioPTS    int64

	// Buffered samples for current fragment
	videoSamples []*fmp4.Sample
	audioSamples []*fmp4.Sample

	// Video parameter extraction
	vp9Header    []byte // VP9 superframe header
	av1SeqHeader []byte // AV1 sequence header OBU
	h264SPS      []byte
	h264PPS      []byte
	h265VPS      []byte
	h265SPS      []byte
	h265PPS      []byte
}

// NewFMP4Muxer creates a new fMP4 muxer for daemon use.
func NewFMP4Muxer(w io.Writer, config FMP4MuxerConfig) *FMP4Muxer {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &FMP4Muxer{
		writer:         w,
		config:         config,
		videoCodec:     normalizeCodec(config.VideoCodec),
		audioCodec:     normalizeCodec(config.AudioCodec),
		audioInitData:  config.AudioInitData,
		videoTrackID:   1,
		audioTrackID:   2,
		videoTimeScale: 90000, // 90kHz for video
		audioTimeScale: 48000, // 48kHz for audio (will be updated from init data)
		sequenceNumber: 1,
	}
}

// WriteVideo writes a video sample to the muxer.
func (m *FMP4Muxer) WriteVideo(pts, dts int64, data []byte, isKeyframe bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(data) == 0 {
		return nil
	}

	// Extract codec parameters from keyframes
	if isKeyframe {
		if err := m.extractVideoParams(data); err != nil {
			m.config.Logger.Warn("Failed to extract video params",
				slog.String("error", err.Error()))
		}
	}

	// Initialize if we have enough info and haven't yet
	if !m.initialized && m.canInitialize() {
		if err := m.initialize(); err != nil {
			return fmt.Errorf("failed to initialize fMP4 muxer: %w", err)
		}
	}

	// If not initialized, we need to wait for keyframe with params
	if !m.initialized {
		if isKeyframe {
			m.config.Logger.Debug("Waiting for video params before initializing")
		}
		return nil
	}

	// Create sample based on codec
	sample, err := m.createVideoSample(pts, dts, data, isKeyframe)
	if err != nil {
		return err
	}

	m.videoSamples = append(m.videoSamples, sample)
	m.lastVideoPTS = pts

	return nil
}

// WriteAudio writes an audio sample to the muxer.
func (m *FMP4Muxer) WriteAudio(pts int64, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(data) == 0 || !m.initialized {
		return nil
	}

	// Create audio sample
	sample := &fmp4.Sample{
		Duration:        1024, // AAC frame duration at 48kHz
		PTSOffset:       0,
		IsNonSyncSample: false, // Audio is always sync
		Payload:         m.extractRawAudio(data),
	}

	m.audioSamples = append(m.audioSamples, sample)
	m.lastAudioPTS = pts

	return nil
}

// Flush writes any buffered samples as a fragment.
func (m *FMP4Muxer) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return nil
	}

	// Write init segment if not done yet
	if !m.initWritten {
		if err := m.writeInit(); err != nil {
			return err
		}
		m.initWritten = true
	}

	// Write fragment if we have samples
	if len(m.videoSamples) > 0 || len(m.audioSamples) > 0 {
		if err := m.writeFragment(); err != nil {
			return err
		}
	}

	return nil
}

// InitializeAndGetHeader initializes the muxer and returns the init segment.
func (m *FMP4Muxer) InitializeAndGetHeader() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Can't write header until we have video params
	// The header will be written on first Flush() after initialization
	return nil, nil
}

// Format returns the FFmpeg input format string for fMP4.
func (m *FMP4Muxer) Format() string {
	return "mp4"
}

// canInitialize checks if we have enough info to initialize.
func (m *FMP4Muxer) canInitialize() bool {
	switch m.videoCodec {
	case "av1":
		return len(m.av1SeqHeader) > 0
	case "vp9":
		// VP9 doesn't require init data
		return true
	case "h265":
		return len(m.h265VPS) > 0 && len(m.h265SPS) > 0 && len(m.h265PPS) > 0
	case "h264":
		return len(m.h264SPS) > 0 && len(m.h264PPS) > 0
	default:
		return false
	}
}

// initialize sets up the muxer once we have codec params.
func (m *FMP4Muxer) initialize() error {
	m.initialized = true
	m.config.Logger.Info("fMP4 muxer initialized",
		slog.String("video_codec", m.videoCodec),
		slog.String("audio_codec", m.audioCodec))
	return nil
}

// extractVideoParams extracts codec parameters from video data.
func (m *FMP4Muxer) extractVideoParams(data []byte) error {
	switch m.videoCodec {
	case "av1":
		return m.extractAV1Params(data)
	case "vp9":
		return m.extractVP9Params(data)
	case "h265":
		return m.extractH265Params(data)
	case "h264":
		return m.extractH264Params(data)
	default:
		return fmt.Errorf("unsupported video codec: %s", m.videoCodec)
	}
}

func (m *FMP4Muxer) extractAV1Params(data []byte) error {
	var bs av1.Bitstream
	if err := bs.Unmarshal(data); err != nil {
		return err
	}

	for _, obu := range bs {
		if len(obu) == 0 {
			continue
		}
		obuType := av1.OBUType((obu[0] >> 3) & 0x0F)
		if obuType == av1.OBUTypeSequenceHeader {
			m.av1SeqHeader = make([]byte, len(obu))
			copy(m.av1SeqHeader, obu)
			return nil
		}
	}
	return nil
}

func (m *FMP4Muxer) extractVP9Params(data []byte) error {
	// VP9 superframe header is in the data itself, no separate extraction needed
	return nil
}

func (m *FMP4Muxer) extractH265Params(data []byte) error {
	// Parse NAL units
	nalus := dataToAccessUnit(data)
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		naluType := h265.NALUType((nalu[0] >> 1) & 0x3F)
		switch naluType {
		case h265.NALUType_VPS_NUT:
			m.h265VPS = make([]byte, len(nalu))
			copy(m.h265VPS, nalu)
		case h265.NALUType_SPS_NUT:
			m.h265SPS = make([]byte, len(nalu))
			copy(m.h265SPS, nalu)
		case h265.NALUType_PPS_NUT:
			m.h265PPS = make([]byte, len(nalu))
			copy(m.h265PPS, nalu)
		}
	}
	return nil
}

func (m *FMP4Muxer) extractH264Params(data []byte) error {
	// Parse NAL units
	nalus := dataToAccessUnit(data)
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		naluType := h264.NALUType(nalu[0] & 0x1F)
		switch naluType {
		case h264.NALUTypeSPS:
			m.h264SPS = make([]byte, len(nalu))
			copy(m.h264SPS, nalu)
		case h264.NALUTypePPS:
			m.h264PPS = make([]byte, len(nalu))
			copy(m.h264PPS, nalu)
		}
	}
	return nil
}

// createVideoSample creates an fMP4 sample from video data.
func (m *FMP4Muxer) createVideoSample(pts, dts int64, data []byte, isKeyframe bool) (*fmp4.Sample, error) {
	sample := &fmp4.Sample{
		Duration:        3000, // Default ~33ms at 90kHz
		PTSOffset:       int32(pts - dts),
		IsNonSyncSample: !isKeyframe,
	}

	// Calculate duration from previous sample
	if m.lastVideoPTS > 0 && pts > m.lastVideoPTS {
		sample.Duration = uint32(pts - m.lastVideoPTS)
	}

	switch m.videoCodec {
	case "av1":
		if err := sample.FillAV1(dataToOBUs(data)); err != nil {
			return nil, err
		}
	case "h265":
		if err := sample.FillH265(sample.PTSOffset, dataToAccessUnit(data)); err != nil {
			return nil, err
		}
	case "h264":
		if err := sample.FillH264(sample.PTSOffset, dataToAccessUnit(data)); err != nil {
			return nil, err
		}
	case "vp9":
		// VP9 data goes directly
		sample.Payload = data
		sample.IsNonSyncSample = !isVP9Keyframe(data)
	default:
		sample.Payload = data
	}

	return sample, nil
}

// extractRawAudio extracts raw audio from potentially ADTS-wrapped data.
func (m *FMP4Muxer) extractRawAudio(data []byte) []byte {
	// Check for ADTS header
	if len(data) >= 7 && data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
		frames := extractADTSFrames(data)
		if len(frames) > 0 {
			return frames[0]
		}
	}
	return data
}

// writeInit writes the fMP4 initialization segment.
func (m *FMP4Muxer) writeInit() error {
	init := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{},
	}

	// Add video track
	videoCodec, err := m.createVideoCodec()
	if err != nil {
		return fmt.Errorf("failed to create video codec: %w", err)
	}

	init.Tracks = append(init.Tracks, &fmp4.InitTrack{
		ID:        m.videoTrackID,
		TimeScale: m.videoTimeScale,
		Codec:     videoCodec,
	})

	// Add audio track if configured
	if m.audioCodec != "" {
		audioCodec, err := m.createAudioCodec()
		if err != nil {
			m.config.Logger.Warn("Failed to create audio codec, skipping audio track",
				slog.String("error", err.Error()))
		} else {
			init.Tracks = append(init.Tracks, &fmp4.InitTrack{
				ID:        m.audioTrackID,
				TimeScale: m.audioTimeScale,
				Codec:     audioCodec,
			})
		}
	}

	// Marshal to buffer then write
	var buf bytes.Buffer
	bufWriter := &seekableBuffer{Buffer: &buf}
	if err := init.Marshal(bufWriter); err != nil {
		return fmt.Errorf("failed to marshal init segment: %w", err)
	}

	_, err = m.writer.Write(buf.Bytes())
	return err
}

// writeFragment writes a fragment with current samples.
func (m *FMP4Muxer) writeFragment() error {
	if len(m.videoSamples) == 0 && len(m.audioSamples) == 0 {
		return nil
	}

	part := &fmp4.Part{
		SequenceNumber: m.sequenceNumber,
		Tracks:         []*fmp4.PartTrack{},
	}

	// Add video track
	if len(m.videoSamples) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       m.videoTrackID,
			BaseTime: m.videoBaseTime,
			Samples:  m.videoSamples,
		})

		// Update base time for next fragment
		for _, s := range m.videoSamples {
			m.videoBaseTime += uint64(s.Duration)
		}
		m.videoSamples = nil
	}

	// Add audio track
	if len(m.audioSamples) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       m.audioTrackID,
			BaseTime: m.audioBaseTime,
			Samples:  m.audioSamples,
		})

		// Update base time for next fragment
		for _, s := range m.audioSamples {
			m.audioBaseTime += uint64(s.Duration)
		}
		m.audioSamples = nil
	}

	// Marshal to buffer then write
	var buf bytes.Buffer
	bufWriter := &seekableBuffer{Buffer: &buf}
	if err := part.Marshal(bufWriter); err != nil {
		return fmt.Errorf("failed to marshal fragment: %w", err)
	}

	_, err := m.writer.Write(buf.Bytes())
	m.sequenceNumber++
	return err
}

// createVideoCodec creates the mp4.Codec for the video track.
func (m *FMP4Muxer) createVideoCodec() (mp4.Codec, error) {
	switch m.videoCodec {
	case "av1":
		if len(m.av1SeqHeader) == 0 {
			return nil, fmt.Errorf("AV1 sequence header not available")
		}
		return &mp4.CodecAV1{
			SequenceHeader: m.av1SeqHeader,
		}, nil

	case "vp9":
		// VP9 needs width/height from first frame, use defaults if not available
		return &mp4.CodecVP9{
			Width:   1920,
			Height:  1080,
			Profile: 0,
		}, nil

	case "h265":
		if len(m.h265VPS) == 0 || len(m.h265SPS) == 0 || len(m.h265PPS) == 0 {
			return nil, fmt.Errorf("H.265 VPS/SPS/PPS not available")
		}
		return &mp4.CodecH265{
			VPS: m.h265VPS,
			SPS: m.h265SPS,
			PPS: m.h265PPS,
		}, nil

	case "h264":
		if len(m.h264SPS) == 0 || len(m.h264PPS) == 0 {
			return nil, fmt.Errorf("H.264 SPS/PPS not available")
		}
		return &mp4.CodecH264{
			SPS: m.h264SPS,
			PPS: m.h264PPS,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported video codec: %s", m.videoCodec)
	}
}

// createAudioCodec creates the mp4.Codec for the audio track.
func (m *FMP4Muxer) createAudioCodec() (mp4.Codec, error) {
	switch m.audioCodec {
	case "aac":
		var config mpeg4audio.AudioSpecificConfig
		if len(m.audioInitData) > 0 {
			if err := config.Unmarshal(m.audioInitData); err != nil {
				// Use defaults
				config = mpeg4audio.AudioSpecificConfig{
					Type:         mpeg4audio.ObjectTypeAACLC,
					SampleRate:   48000,
					ChannelCount: 2,
				}
			}
		} else {
			config = mpeg4audio.AudioSpecificConfig{
				Type:         mpeg4audio.ObjectTypeAACLC,
				SampleRate:   48000,
				ChannelCount: 2,
			}
		}
		m.audioTimeScale = uint32(config.SampleRate)
		return &mp4.CodecMPEG4Audio{Config: config}, nil

	case "opus":
		return &mp4.CodecOpus{ChannelCount: 2}, nil

	case "ac3":
		return &mp4.CodecAC3{
			SampleRate:   48000,
			ChannelCount: 2,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported audio codec: %s", m.audioCodec)
	}
}

// dataToOBUs converts AV1 data to OBU slices.
func dataToOBUs(data []byte) [][]byte {
	var bs av1.Bitstream
	if err := bs.Unmarshal(data); err != nil {
		return [][]byte{data}
	}
	return bs
}

// isVP9Keyframe checks if VP9 data is a keyframe.
func isVP9Keyframe(data []byte) bool {
	if len(data) < 1 {
		return false
	}
	// VP9 frame marker
	frameMarker := (data[0] >> 6) & 0x03
	if frameMarker != 0x02 {
		return false
	}
	// Check profile and show_existing_frame
	profile := (data[0] >> 4) & 0x03
	if profile == 3 {
		// Profile 3 has different bit layout
		return (data[0] & 0x08) == 0
	}
	return (data[0] & 0x04) == 0
}

// seekableBuffer wraps bytes.Buffer to provide io.WriteSeeker.
type seekableBuffer struct {
	*bytes.Buffer
	pos int64
}

func (s *seekableBuffer) Write(p []byte) (n int, err error) {
	// Ensure buffer is large enough
	if int(s.pos) > s.Buffer.Len() {
		s.Buffer.Write(make([]byte, int(s.pos)-s.Buffer.Len()))
	}

	// Write at current position
	if int(s.pos) == s.Buffer.Len() {
		n, err = s.Buffer.Write(p)
	} else {
		// Overwrite existing data
		b := s.Buffer.Bytes()
		n = copy(b[s.pos:], p)
		if n < len(p) {
			m, err := s.Buffer.Write(p[n:])
			if err != nil {
				return n, err
			}
			n += m
		}
	}
	s.pos += int64(n)
	return n, err
}

func (s *seekableBuffer) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = s.pos + offset
	case io.SeekEnd:
		newPos = int64(s.Buffer.Len()) + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}
	if newPos < 0 {
		return 0, fmt.Errorf("negative position")
	}
	s.pos = newPos
	return newPos, nil
}

// Ensure FMP4Muxer implements InputMuxer interface.
var _ InputMuxer = (*FMP4Muxer)(nil)
