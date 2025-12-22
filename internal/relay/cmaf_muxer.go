// Package relay provides stream relay functionality with CMAF support.
package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
	// Note: vp9 codec package is imported in fmp4_adapter.go for header parsing
)

// CMAF/fMP4 box types (ISO Base Media File Format)
const (
	BoxTypeFTYP = "ftyp" // File type
	BoxTypeMOOV = "moov" // Movie (metadata)
	BoxTypeMOOF = "moof" // Movie fragment
	BoxTypeMDAT = "mdat" // Media data
	BoxTypeSIDX = "sidx" // Segment index
	BoxTypeSYNC = "sync" // Sync sample
	BoxTypePRFT = "prft" // Producer reference time
	BoxTypeMFRA = "mfra" // Movie fragment random access
)

// Errors for CMAF parsing
var (
	ErrInvalidBoxHeader   = errors.New("invalid MP4 box header")
	ErrUnexpectedEOF      = errors.New("unexpected end of data")
	ErrNoInitSegment      = errors.New("no initialization segment found")
	ErrNoMediaSegments    = errors.New("no media segments found")
	ErrInvalidFragmentBox = errors.New("invalid fragment: expected moof+mdat")
	ErrCodecNotConfigured = errors.New("codec not configured")
)

// FMP4Fragment represents a single fMP4 fragment (moof+mdat pair).
type FMP4Fragment struct {
	SequenceNumber uint32 // mfhd sequence number
	DecodeTime     uint64 // Base decode time from tfdt
	Duration       uint64 // Total duration of samples (in track timescale units)
	Data           []byte // Complete fragment data (moof+mdat)
	IsKeyframe     bool   // True if contains sync sample (keyframe)
	TrackID        uint32 // Track ID from tfhd (for timescale lookup)
}

// FMP4InitSegment represents the initialization segment (ftyp+moov).
type FMP4InitSegment struct {
	Data            []byte            // Complete init segment data
	HasVideo        bool              // Contains video track
	HasAudio        bool              // Contains audio track
	Timescale       uint32            // Movie timescale (from mvhd) - used as fallback
	TrackTimescales map[uint32]uint32 // Per-track timescales (track_id -> mdhd timescale)
	VideoTrackID    uint32            // Track ID for video track (if present)
	AudioTrackID    uint32            // Track ID for audio track (if present)
}

// GetTimescale returns the timescale for a given track ID.
// Falls back to the movie timescale if track-specific timescale is not found.
func (is *FMP4InitSegment) GetTimescale(trackID uint32) uint32 {
	if is.TrackTimescales != nil {
		if ts, ok := is.TrackTimescales[trackID]; ok {
			return ts
		}
	}
	// Fallback to movie timescale
	return is.Timescale
}

// CMAFMuxer parses and segments fMP4/CMAF streams using mediacommon.
// It extracts initialization segments and fragments from a continuous fMP4 stream.
type CMAFMuxer struct {
	mu sync.RWMutex

	// Parsed segments
	initSegment *FMP4InitSegment
	fragments   []*FMP4Fragment

	// Buffer for incomplete data
	buffer bytes.Buffer

	// Configuration
	maxFragments int // Maximum fragments to keep (sliding window)

	// State
	expectingInit     bool   // True if we haven't seen init segment yet
	currentSeqNum     uint32 // Current sequence number
	fragmentStarted   bool   // True if we're in the middle of a fragment
	pendingMoof       []byte // Pending moof box waiting for mdat
	pendingMoofSeqNum uint32 // Sequence number from pending moof

	// mediacommon init for generating segments
	fmp4Init *fmp4.Init
}

// CMAFMuxerConfig holds configuration for the CMAF muxer.
type CMAFMuxerConfig struct {
	MaxFragments int // Maximum fragments to keep (0 = unlimited)
}

// DefaultCMAFMuxerConfig returns default configuration.
func DefaultCMAFMuxerConfig() CMAFMuxerConfig {
	return CMAFMuxerConfig{
		MaxFragments: 10, // Keep last 10 fragments (typical HLS/DASH window)
	}
}

// NewCMAFMuxer creates a new CMAF muxer.
func NewCMAFMuxer(config CMAFMuxerConfig) *CMAFMuxer {
	return &CMAFMuxer{
		maxFragments:  config.MaxFragments,
		expectingInit: true,
		fragments:     make([]*FMP4Fragment, 0),
	}
}

// Write processes incoming fMP4 data.
// It buffers data and extracts complete boxes as they arrive.
func (m *CMAFMuxer) Write(data []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	n, err := m.buffer.Write(data)
	if err != nil {
		return n, err
	}

	// Process all complete boxes
	if err := m.processBuffer(); err != nil {
		return n, err
	}

	return n, nil
}

// processBuffer processes complete boxes from the buffer.
func (m *CMAFMuxer) processBuffer() error {
	for {
		// Need at least 8 bytes for a box header
		if m.buffer.Len() < 8 {
			return nil
		}

		// Peek at the header
		header, err := peekBoxHeader(m.buffer.Bytes())
		if err != nil {
			return err
		}

		// Check if we have the complete box
		if uint64(m.buffer.Len()) < header.Size {
			return nil // Wait for more data
		}

		// Extract the complete box
		boxData := make([]byte, header.Size)
		if _, err := io.ReadFull(&m.buffer, boxData); err != nil {
			return err
		}

		// Process the box based on type
		if err := m.processBox(header, boxData); err != nil {
			return err
		}
	}
}

// processBox handles a single complete box.
func (m *CMAFMuxer) processBox(header BoxHeader, data []byte) error {
	switch header.Type {
	case BoxTypeFTYP:
		// Start of init segment - buffer it
		if m.initSegment == nil {
			m.initSegment = &FMP4InitSegment{
				Data:            make([]byte, 0),
				TrackTimescales: make(map[uint32]uint32),
			}
		}
		m.initSegment.Data = append(m.initSegment.Data, data...)

	case BoxTypeMOOV:
		// Complete init segment
		if m.initSegment == nil {
			m.initSegment = &FMP4InitSegment{
				Data:            make([]byte, 0),
				TrackTimescales: make(map[uint32]uint32),
			}
		}
		m.initSegment.Data = append(m.initSegment.Data, data...)

		// Parse moov for metadata using mediacommon
		if err := m.parseInitSegment(); err != nil {
			return fmt.Errorf("parsing init segment: %w", err)
		}
		m.expectingInit = false

	case BoxTypeMOOF:
		// Start of a new fragment - save moof and wait for mdat
		m.pendingMoof = data
		m.fragmentStarted = true

		// Extract sequence number from moof
		if seqNum, err := extractSequenceNumber(data); err == nil {
			m.pendingMoofSeqNum = seqNum
		}

	case BoxTypeMDAT:
		// Media data - should follow moof
		if !m.fragmentStarted || m.pendingMoof == nil {
			// Stray mdat without moof - could be part of init
			if m.expectingInit && m.initSegment != nil {
				m.initSegment.Data = append(m.initSegment.Data, data...)
			}
			return nil
		}

		// Create complete fragment (moof + mdat)
		fragment := &FMP4Fragment{
			SequenceNumber: m.pendingMoofSeqNum,
			Data:           append(m.pendingMoof, data...),
			IsKeyframe:     m.hasKeyframe(m.pendingMoof),
		}

		// Extract timing and track ID from moof
		if dt, dur, tid, err := m.extractTiming(m.pendingMoof); err == nil {
			fragment.DecodeTime = dt
			fragment.Duration = dur
			fragment.TrackID = tid
		}

		m.fragments = append(m.fragments, fragment)
		m.currentSeqNum = m.pendingMoofSeqNum

		// Limit fragment count
		if m.maxFragments > 0 && len(m.fragments) > m.maxFragments {
			m.fragments = m.fragments[1:]
		}

		// Reset fragment state
		m.pendingMoof = nil
		m.fragmentStarted = false

	case BoxTypeSIDX:
		// Segment index - can be part of fragment or standalone
		// For now, ignore as we track fragments via moof/mdat

	default:
		// Unknown box - if we're expecting init, append to init segment
		if m.expectingInit && m.initSegment != nil {
			m.initSegment.Data = append(m.initSegment.Data, data...)
		}
	}

	return nil
}

// parseInitSegment parses the init segment using mediacommon.
func (m *CMAFMuxer) parseInitSegment() error {
	if m.initSegment == nil || len(m.initSegment.Data) == 0 {
		return ErrNoInitSegment
	}

	// Parse using mediacommon
	var init fmp4.Init
	reader := bytes.NewReader(m.initSegment.Data)
	if err := init.Unmarshal(reader); err != nil {
		// Fall back to manual parsing if mediacommon fails
		return m.parseInitManual()
	}

	m.fmp4Init = &init

	// Extract track info
	for _, track := range init.Tracks {
		m.initSegment.TrackTimescales[uint32(track.ID)] = track.TimeScale

		switch track.Codec.(type) {
		case *mp4.CodecH264, *mp4.CodecH265, *mp4.CodecAV1, *mp4.CodecVP9:
			m.initSegment.HasVideo = true
			m.initSegment.VideoTrackID = uint32(track.ID)
			if m.initSegment.Timescale == 0 {
				m.initSegment.Timescale = track.TimeScale
			}
		case *mp4.CodecMPEG4Audio, *mp4.CodecAC3, *mp4.CodecEAC3, *mp4.CodecOpus, *mp4.CodecMPEG1Audio:
			m.initSegment.HasAudio = true
			m.initSegment.AudioTrackID = uint32(track.ID)
		}
	}

	return nil
}

// parseInitManual parses the init segment manually (fallback).
func (m *CMAFMuxer) parseInitManual() error {
	data := m.initSegment.Data
	offset := 0

	// Find and parse moov box
	for offset < len(data) {
		if offset+8 > len(data) {
			break
		}

		header, err := peekBoxHeader(data[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(data) {
			break
		}

		if header.Type == BoxTypeMOOV {
			m.parseMoovManual(data[offset:boxEnd])
		}

		offset = boxEnd
	}

	return nil
}

// parseMoovManual extracts metadata from the moov box manually.
func (m *CMAFMuxer) parseMoovManual(data []byte) {
	if len(data) < 8 {
		return
	}

	offset := 8
	for offset < len(data) {
		if offset+8 > len(data) {
			break
		}

		header, err := peekBoxHeader(data[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(data) {
			break
		}

		switch header.Type {
		case "mvhd":
			if ts := extractTimescale(data[offset:boxEnd]); ts > 0 {
				m.initSegment.Timescale = ts
			}
		case "trak":
			m.parseTrakManual(data[offset:boxEnd])
		}

		offset = boxEnd
	}
}

// parseTrakManual parses a trak box manually.
func (m *CMAFMuxer) parseTrakManual(data []byte) {
	if len(data) < 8 {
		return
	}

	trackID := extractTrackID(data)
	trackTimescale := extractTrackTimescale(data)

	if trackID > 0 && trackTimescale > 0 {
		m.initSegment.TrackTimescales[trackID] = trackTimescale
	}

	if isVideoTrack(data) {
		m.initSegment.HasVideo = true
		m.initSegment.VideoTrackID = trackID
	}
	if isAudioTrack(data) {
		m.initSegment.HasAudio = true
		m.initSegment.AudioTrackID = trackID
	}
}

// GetInitSegment returns the initialization segment if available.
func (m *CMAFMuxer) GetInitSegment() *FMP4InitSegment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.initSegment
}

// GetFragments returns all available fragments.
func (m *CMAFMuxer) GetFragments() []*FMP4Fragment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*FMP4Fragment, len(m.fragments))
	copy(result, m.fragments)
	return result
}

// GetFragment returns a specific fragment by sequence number.
func (m *CMAFMuxer) GetFragment(seqNum uint32) *FMP4Fragment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, f := range m.fragments {
		if f.SequenceNumber == seqNum {
			return f
		}
	}
	return nil
}

// GetLatestFragment returns the most recent fragment.
func (m *CMAFMuxer) GetLatestFragment() *FMP4Fragment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.fragments) == 0 {
		return nil
	}
	return m.fragments[len(m.fragments)-1]
}

// FragmentCount returns the number of available fragments.
func (m *CMAFMuxer) FragmentCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.fragments)
}

// HasInitSegment returns true if the initialization segment has been parsed.
func (m *CMAFMuxer) HasInitSegment() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.initSegment != nil && !m.expectingInit
}

// Reset clears all parsed data.
func (m *CMAFMuxer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initSegment = nil
	m.fragments = make([]*FMP4Fragment, 0)
	m.buffer.Reset()
	m.expectingInit = true
	m.currentSeqNum = 0
	m.fragmentStarted = false
	m.pendingMoof = nil
	m.fmp4Init = nil
}

// BoxHeader represents an MP4 box header.
type BoxHeader struct {
	Size     uint64 // Total size including header
	Type     string // 4-character box type
	Extended bool   // True if using 64-bit size
}

// peekBoxHeader reads a box header without consuming data.
func peekBoxHeader(data []byte) (BoxHeader, error) {
	if len(data) < 8 {
		return BoxHeader{}, ErrUnexpectedEOF
	}

	size := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	boxType := string(data[4:8])

	header := BoxHeader{
		Size: uint64(size),
		Type: boxType,
	}

	// Handle extended size
	if size == 1 {
		if len(data) < 16 {
			return BoxHeader{}, ErrUnexpectedEOF
		}
		header.Size = uint64(data[8])<<56 | uint64(data[9])<<48 | uint64(data[10])<<40 | uint64(data[11])<<32 |
			uint64(data[12])<<24 | uint64(data[13])<<16 | uint64(data[14])<<8 | uint64(data[15])
		header.Extended = true
	} else if size == 0 {
		// Box extends to end of file - can't determine size without more context
		return BoxHeader{}, ErrInvalidBoxHeader
	}

	return header, nil
}

// extractSequenceNumber extracts the sequence number from a moof box.
func extractSequenceNumber(moof []byte) (uint32, error) {
	if len(moof) < 16 {
		return 0, ErrInvalidBoxHeader
	}

	// Skip moof header and search for mfhd
	offset := 8
	for offset+8 < len(moof) {
		header, err := peekBoxHeader(moof[offset:])
		if err != nil {
			break
		}

		if header.Type == "mfhd" && offset+16 <= len(moof) {
			// mfhd: version(1) + flags(3) + sequence_number(4)
			return uint32(moof[offset+12])<<24 | uint32(moof[offset+13])<<16 |
				uint32(moof[offset+14])<<8 | uint32(moof[offset+15]), nil
		}

		offset += int(header.Size)
		if header.Size == 0 {
			break
		}
	}

	return 0, errors.New("mfhd not found in moof")
}

// extractTiming extracts decode time, duration, and track ID from a moof box.
func (m *CMAFMuxer) extractTiming(moof []byte) (decodeTime uint64, duration uint64, trackID uint32, err error) {
	if len(moof) < 8 {
		return 0, 0, 0, ErrInvalidBoxHeader
	}

	// Search for traf (track fragment) containing tfdt (track fragment decode time)
	offset := 8
	for offset+8 < len(moof) {
		header, e := peekBoxHeader(moof[offset:])
		if e != nil {
			break
		}

		if header.Type == "traf" && offset+int(header.Size) <= len(moof) {
			dt, dur, tid := m.parseTraf(moof[offset : offset+int(header.Size)])
			if dt > 0 || tid > 0 {
				return dt, dur, tid, nil
			}
		}

		offset += int(header.Size)
		if header.Size == 0 {
			break
		}
	}

	return 0, 0, 0, errors.New("timing info not found")
}

// parseTraf parses a traf box for decode time, duration, and track ID.
func (m *CMAFMuxer) parseTraf(traf []byte) (decodeTime uint64, duration uint64, trackID uint32) {
	if len(traf) < 8 {
		return 0, 0, 0
	}

	var defaultSampleDuration uint32

	offset := 8
	for offset+8 < len(traf) {
		header, err := peekBoxHeader(traf[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(traf) {
			break
		}

		switch header.Type {
		case "tfhd":
			if boxEnd >= offset+16 {
				flags := uint32(traf[offset+9])<<16 | uint32(traf[offset+10])<<8 | uint32(traf[offset+11])
				trackID = uint32(traf[offset+12])<<24 | uint32(traf[offset+13])<<16 |
					uint32(traf[offset+14])<<8 | uint32(traf[offset+15])
				tfhdOffset := offset + 16

				if flags&0x000001 != 0 { // base_data_offset
					tfhdOffset += 8
				}
				if flags&0x000002 != 0 { // sample_description_index
					tfhdOffset += 4
				}
				if flags&0x000008 != 0 { // default_sample_duration
					if tfhdOffset+4 <= boxEnd {
						defaultSampleDuration = uint32(traf[tfhdOffset])<<24 | uint32(traf[tfhdOffset+1])<<16 |
							uint32(traf[tfhdOffset+2])<<8 | uint32(traf[tfhdOffset+3])
					}
				}
			}

		case "tfdt":
			if boxEnd >= offset+16 {
				version := traf[offset+8]
				if version == 1 && boxEnd >= offset+20 {
					decodeTime = uint64(traf[offset+12])<<56 | uint64(traf[offset+13])<<48 |
						uint64(traf[offset+14])<<40 | uint64(traf[offset+15])<<32 |
						uint64(traf[offset+16])<<24 | uint64(traf[offset+17])<<16 |
						uint64(traf[offset+18])<<8 | uint64(traf[offset+19])
				} else {
					decodeTime = uint64(traf[offset+12])<<24 | uint64(traf[offset+13])<<16 |
						uint64(traf[offset+14])<<8 | uint64(traf[offset+15])
				}
			}

		case "trun":
			if boxEnd >= offset+16 {
				flags := uint32(traf[offset+9])<<16 | uint32(traf[offset+10])<<8 | uint32(traf[offset+11])
				sampleCount := uint32(traf[offset+12])<<24 | uint32(traf[offset+13])<<16 |
					uint32(traf[offset+14])<<8 | uint32(traf[offset+15])

				trunOffset := offset + 16
				if flags&0x000001 != 0 { // data_offset
					trunOffset += 4
				}
				if flags&0x000004 != 0 { // first_sample_flags
					trunOffset += 4
				}

				// Calculate sample entry size
				sampleEntrySize := 0
				if flags&0x000100 != 0 { // sample_duration
					sampleEntrySize += 4
				}
				if flags&0x000200 != 0 { // sample_size
					sampleEntrySize += 4
				}
				if flags&0x000400 != 0 { // sample_flags
					sampleEntrySize += 4
				}
				if flags&0x000800 != 0 { // sample_composition_time_offset
					sampleEntrySize += 4
				}

				// Sum sample durations
				if flags&0x000100 != 0 && sampleEntrySize > 0 {
					for i := uint32(0); i < sampleCount; i++ {
						sampleOffset := trunOffset + int(i)*sampleEntrySize
						if sampleOffset+4 <= boxEnd {
							sampleDuration := uint32(traf[sampleOffset])<<24 | uint32(traf[sampleOffset+1])<<16 |
								uint32(traf[sampleOffset+2])<<8 | uint32(traf[sampleOffset+3])
							duration += uint64(sampleDuration)
						}
					}
				} else if defaultSampleDuration > 0 {
					duration = uint64(defaultSampleDuration) * uint64(sampleCount)
				} else {
					// Estimate based on 30fps
					duration = uint64(sampleCount) * 1001 / 30
				}
			}
		}

		offset = boxEnd
	}

	return decodeTime, duration, trackID
}

// hasKeyframe checks if a moof contains a sync sample (keyframe).
func (m *CMAFMuxer) hasKeyframe(moof []byte) bool {
	if len(moof) < 8 {
		return false
	}

	offset := 8
	for offset+8 < len(moof) {
		header, err := peekBoxHeader(moof[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		if header.Type == "traf" && offset+int(header.Size) <= len(moof) {
			if m.trafHasKeyframe(moof[offset : offset+int(header.Size)]) {
				return true
			}
		}

		offset += int(header.Size)
	}

	return true // Default to true for first fragment
}

// trafHasKeyframe checks if a traf box contains a sync sample.
func (m *CMAFMuxer) trafHasKeyframe(traf []byte) bool {
	if len(traf) < 8 {
		return false
	}

	offset := 8
	for offset+8 < len(traf) {
		header, err := peekBoxHeader(traf[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		if header.Type == "trun" && offset+12 < len(traf) {
			flags := uint32(traf[offset+9])<<16 | uint32(traf[offset+10])<<8 | uint32(traf[offset+11])
			if flags&0x404 != 0 {
				return true
			}
		}

		offset += int(header.Size)
	}

	return true
}

// extractTimescale extracts the timescale from a mvhd box.
func extractTimescale(mvhd []byte) uint32 {
	if len(mvhd) < 20 {
		return 0
	}

	version := mvhd[8]
	if version == 1 {
		if len(mvhd) < 32 {
			return 0
		}
		return uint32(mvhd[28])<<24 | uint32(mvhd[29])<<16 | uint32(mvhd[30])<<8 | uint32(mvhd[31])
	}

	if len(mvhd) < 24 {
		return 0
	}
	return uint32(mvhd[20])<<24 | uint32(mvhd[21])<<16 | uint32(mvhd[22])<<8 | uint32(mvhd[23])
}

// isVideoTrack checks if a trak box contains a video track.
func isVideoTrack(trak []byte) bool {
	return findHandler(trak) == "vide"
}

// isAudioTrack checks if a trak box contains an audio track.
func isAudioTrack(trak []byte) bool {
	return findHandler(trak) == "soun"
}

// findHandler finds the handler type in a trak box.
func findHandler(trak []byte) string {
	if len(trak) < 8 {
		return ""
	}

	offset := 8
	for offset+8 < len(trak) {
		header, err := peekBoxHeader(trak[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(trak) {
			break
		}

		if header.Type == "mdia" {
			return findHandlerInMdia(trak[offset:boxEnd])
		}

		offset = boxEnd
	}

	return ""
}

// findHandlerInMdia finds hdlr in mdia box.
func findHandlerInMdia(mdia []byte) string {
	if len(mdia) < 8 {
		return ""
	}

	offset := 8
	for offset+8 < len(mdia) {
		header, err := peekBoxHeader(mdia[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(mdia) {
			break
		}

		if header.Type == "hdlr" && boxEnd >= offset+20 {
			return string(mdia[offset+16 : offset+20])
		}

		offset = boxEnd
	}

	return ""
}

// extractTrackID extracts the track ID from a trak box.
func extractTrackID(trak []byte) uint32 {
	if len(trak) < 8 {
		return 0
	}

	offset := 8
	for offset+8 < len(trak) {
		header, err := peekBoxHeader(trak[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(trak) {
			break
		}

		if header.Type == "tkhd" && boxEnd > offset+20 {
			version := trak[offset+8]
			if version == 1 {
				if boxEnd >= offset+28 {
					return uint32(trak[offset+24])<<24 | uint32(trak[offset+25])<<16 |
						uint32(trak[offset+26])<<8 | uint32(trak[offset+27])
				}
			} else {
				if boxEnd >= offset+20 {
					return uint32(trak[offset+16])<<24 | uint32(trak[offset+17])<<16 |
						uint32(trak[offset+18])<<8 | uint32(trak[offset+19])
				}
			}
		}

		offset = boxEnd
	}

	return 0
}

// extractTrackTimescale extracts the timescale from a trak box (from mdhd).
func extractTrackTimescale(trak []byte) uint32 {
	if len(trak) < 8 {
		return 0
	}

	offset := 8
	for offset+8 < len(trak) {
		header, err := peekBoxHeader(trak[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(trak) {
			break
		}

		if header.Type == "mdia" {
			return extractTimescaleFromMdia(trak[offset:boxEnd])
		}

		offset = boxEnd
	}

	return 0
}

// extractTimescaleFromMdia extracts timescale from mdia box.
func extractTimescaleFromMdia(mdia []byte) uint32 {
	if len(mdia) < 8 {
		return 0
	}

	offset := 8
	for offset+8 < len(mdia) {
		header, err := peekBoxHeader(mdia[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		boxEnd := offset + int(header.Size)
		if boxEnd > len(mdia) {
			break
		}

		if header.Type == "mdhd" {
			return extractTimescaleFromMdhd(mdia[offset:boxEnd])
		}

		offset = boxEnd
	}

	return 0
}

// extractTimescaleFromMdhd extracts timescale from mdhd box.
func extractTimescaleFromMdhd(mdhd []byte) uint32 {
	if len(mdhd) < 20 {
		return 0
	}

	version := mdhd[8]
	if version == 1 {
		if len(mdhd) < 32 {
			return 0
		}
		return uint32(mdhd[28])<<24 | uint32(mdhd[29])<<16 | uint32(mdhd[30])<<8 | uint32(mdhd[31])
	}

	if len(mdhd) < 24 {
		return 0
	}
	return uint32(mdhd[20])<<24 | uint32(mdhd[21])<<16 | uint32(mdhd[22])<<8 | uint32(mdhd[23])
}

// FMP4Writer wraps mediacommon's fmp4 for writing init segments and parts.
type FMP4Writer struct {
	mu sync.Mutex

	// Track configuration
	videoTrack *fmp4.InitTrack
	audioTrack *fmp4.InitTrack

	// Codec params
	h264SPS       []byte
	h264PPS       []byte
	h265VPS       []byte
	h265SPS       []byte
	h265PPS       []byte
	av1SeqHeader  []byte
	// VP9 params
	vp9Width             int
	vp9Height            int
	vp9Profile           uint8
	vp9BitDepth          uint8
	vp9ChromaSubsampling uint8
	vp9ColorRange        bool
	vp9Configured        bool
	aacConf              *mpeg4audio.AudioSpecificConfig

	// Opus params
	opusChannelCount int
	opusConfigured   bool

	// AC3 params
	ac3SampleRate   int
	ac3ChannelCount int
	ac3Configured   bool

	// EAC3 params
	eac3SampleRate   int
	eac3ChannelCount int
	eac3Configured   bool

	// MP3 params
	mp3SampleRate   int
	mp3ChannelCount int
	mp3Configured   bool

	// State
	seqNum      uint32
	initialized bool
}

// NewFMP4Writer creates a new fMP4 writer using mediacommon.
func NewFMP4Writer() *FMP4Writer {
	return &FMP4Writer{
		seqNum: 1,
	}
}

// SetH264Params sets H.264 codec parameters.
func (w *FMP4Writer) SetH264Params(sps, pps []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.h264SPS = sps
	w.h264PPS = pps
}

// SetH265Params sets H.265 codec parameters.
func (w *FMP4Writer) SetH265Params(vps, sps, pps []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.h265VPS = vps
	w.h265SPS = sps
	w.h265PPS = pps
}

// SetAV1Params sets AV1 codec parameters.
func (w *FMP4Writer) SetAV1Params(seqHeader []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.av1SeqHeader = seqHeader
}

// SetVP9Params sets VP9 codec parameters.
func (w *FMP4Writer) SetVP9Params(width, height int, profile, bitDepth, chromaSubsampling uint8, colorRange bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.vp9Width = width
	w.vp9Height = height
	w.vp9Profile = profile
	w.vp9BitDepth = bitDepth
	w.vp9ChromaSubsampling = chromaSubsampling
	w.vp9ColorRange = colorRange
	w.vp9Configured = true
}

// SetAACConfig sets AAC codec configuration.
func (w *FMP4Writer) SetAACConfig(config *mpeg4audio.AudioSpecificConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.aacConf = config
}

// SetOpusConfig sets Opus codec configuration.
func (w *FMP4Writer) SetOpusConfig(channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if channelCount <= 0 {
		channelCount = 2 // Default to stereo
	}
	w.opusChannelCount = channelCount
	w.opusConfigured = true
}

// SetAC3Config sets AC3 codec configuration.
func (w *FMP4Writer) SetAC3Config(sampleRate, channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channelCount <= 0 {
		channelCount = 2
	}
	w.ac3SampleRate = sampleRate
	w.ac3ChannelCount = channelCount
	w.ac3Configured = true
}

// SetEAC3Config sets EAC3 (Enhanced AC3 / Dolby Digital Plus) codec configuration.
func (w *FMP4Writer) SetEAC3Config(sampleRate, channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channelCount <= 0 {
		channelCount = 2
	}
	w.eac3SampleRate = sampleRate
	w.eac3ChannelCount = channelCount
	w.eac3Configured = true
}

// SetMP3Config sets MP3 codec configuration.
func (w *FMP4Writer) SetMP3Config(sampleRate, channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channelCount <= 0 {
		channelCount = 2
	}
	w.mp3SampleRate = sampleRate
	w.mp3ChannelCount = channelCount
	w.mp3Configured = true
}

// GenerateInit generates an fMP4 initialization segment.
func (w *FMP4Writer) GenerateInit(hasVideo, hasAudio bool, videoTimescale, audioTimescale uint32) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	init := fmp4.Init{
		Tracks: make([]*fmp4.InitTrack, 0),
	}

	trackID := 1

	if hasVideo {
		var codec mp4.Codec

		if w.h264SPS != nil && w.h264PPS != nil {
			var spsp h264.SPS
			if err := spsp.Unmarshal(w.h264SPS); err == nil {
				codec = &mp4.CodecH264{
					SPS: w.h264SPS,
					PPS: w.h264PPS,
				}
			}
		} else if w.h265SPS != nil && w.h265PPS != nil {
			var spsp h265.SPS
			if err := spsp.Unmarshal(w.h265SPS); err == nil {
				codec = &mp4.CodecH265{
					VPS: w.h265VPS,
					SPS: w.h265SPS,
					PPS: w.h265PPS,
				}
			}
		} else if w.av1SeqHeader != nil {
			// AV1 codec - verify sequence header is valid
			var seqHdr av1.SequenceHeader
			if err := seqHdr.Unmarshal(w.av1SeqHeader); err == nil {
				codec = &mp4.CodecAV1{
					SequenceHeader: w.av1SeqHeader,
				}
			}
		} else if w.vp9Configured {
			// VP9 codec
			codec = &mp4.CodecVP9{
				Width:             w.vp9Width,
				Height:            w.vp9Height,
				Profile:           w.vp9Profile,
				BitDepth:          w.vp9BitDepth,
				ChromaSubsampling: w.vp9ChromaSubsampling,
				ColorRange:        w.vp9ColorRange,
			}
		}

		if codec != nil {
			w.videoTrack = &fmp4.InitTrack{
				ID:        trackID,
				TimeScale: videoTimescale,
				Codec:     codec,
			}
			init.Tracks = append(init.Tracks, w.videoTrack)
			trackID++
		}
	}

	if hasAudio {
		var audioCodec mp4.Codec

		// Check audio codecs in order of priority
		if w.opusConfigured {
			audioCodec = &mp4.CodecOpus{
				ChannelCount: w.opusChannelCount,
			}
		} else if w.ac3Configured {
			audioCodec = &mp4.CodecAC3{
				SampleRate:   w.ac3SampleRate,
				ChannelCount: w.ac3ChannelCount,
			}
		} else if w.eac3Configured {
			audioCodec = &mp4.CodecEAC3{
				SampleRate:   w.eac3SampleRate,
				ChannelCount: w.eac3ChannelCount,
			}
		} else if w.mp3Configured {
			audioCodec = &mp4.CodecMPEG1Audio{
				SampleRate:   w.mp3SampleRate,
				ChannelCount: w.mp3ChannelCount,
			}
		} else if w.aacConf != nil {
			audioCodec = &mp4.CodecMPEG4Audio{
				Config: *w.aacConf,
			}
		}

		if audioCodec != nil {
			w.audioTrack = &fmp4.InitTrack{
				ID:        trackID,
				TimeScale: audioTimescale,
				Codec:     audioCodec,
			}
			init.Tracks = append(init.Tracks, w.audioTrack)
		}
	}

	if len(init.Tracks) == 0 {
		return nil, ErrCodecNotConfigured
	}

	// Marshal to bytes
	var buf seekablebuffer.Buffer
	if err := init.Marshal(&buf); err != nil {
		return nil, fmt.Errorf("marshaling init: %w", err)
	}

	w.initialized = true
	return buf.Bytes(), nil
}

// GeneratePart generates an fMP4 media part.
func (w *FMP4Writer) GeneratePart(videoSamples, audioSamples []*fmp4.Sample, videoBaseTime, audioBaseTime uint64) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.initialized {
		return nil, errors.New("not initialized")
	}

	part := fmp4.Part{
		SequenceNumber: w.seqNum,
		Tracks:         make([]*fmp4.PartTrack, 0),
	}

	if w.videoTrack != nil && len(videoSamples) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       w.videoTrack.ID,
			BaseTime: videoBaseTime,
			Samples:  videoSamples,
		})
	}

	if w.audioTrack != nil && len(audioSamples) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       w.audioTrack.ID,
			BaseTime: audioBaseTime,
			Samples:  audioSamples,
		})
	}

	if len(part.Tracks) == 0 {
		return nil, errors.New("no samples to write")
	}

	var buf seekablebuffer.Buffer
	if err := part.Marshal(&buf); err != nil {
		return nil, fmt.Errorf("marshaling part: %w", err)
	}

	w.seqNum++
	return buf.Bytes(), nil
}

// VideoTrackID returns the video track ID.
func (w *FMP4Writer) VideoTrackID() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.videoTrack != nil {
		return w.videoTrack.ID
	}
	return 0
}

// AudioTrackID returns the audio track ID.
func (w *FMP4Writer) AudioTrackID() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.audioTrack != nil {
		return w.audioTrack.ID
	}
	return 0
}

// Reset resets the writer state.
func (w *FMP4Writer) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.videoTrack = nil
	w.audioTrack = nil
	w.seqNum = 1
	w.initialized = false
}

// SequenceNumber returns the current sequence number.
func (w *FMP4Writer) SequenceNumber() uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seqNum
}
