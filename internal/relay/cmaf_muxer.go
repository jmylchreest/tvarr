// Package relay provides stream relay functionality with CMAF support.
package relay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
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
)

// BoxHeader represents an MP4 box header.
type BoxHeader struct {
	Size     uint64 // Total size including header
	Type     string // 4-character box type
	Extended bool   // True if using 64-bit size
}

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

// CMAFMuxer parses and segments fMP4/CMAF streams.
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
				Data: make([]byte, 0),
			}
		}
		m.initSegment.Data = append(m.initSegment.Data, data...)

	case BoxTypeMOOV:
		// Complete init segment
		if m.initSegment == nil {
			m.initSegment = &FMP4InitSegment{
				Data: make([]byte, 0),
			}
		}
		m.initSegment.Data = append(m.initSegment.Data, data...)

		// Parse moov for metadata
		if err := m.parseMoov(data); err != nil {
			return fmt.Errorf("parsing moov: %w", err)
		}
		m.expectingInit = false

	case BoxTypeMOOF:
		// Start of a new fragment - save moof and wait for mdat
		m.pendingMoof = data
		m.fragmentStarted = true

		// Extract sequence number from moof
		seqNum, err := extractSequenceNumber(data)
		if err == nil {
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
			IsKeyframe:     hasKeyframe(m.pendingMoof),
		}

		// Extract timing and track ID from moof
		if dt, dur, tid, err := extractTiming(m.pendingMoof); err == nil {
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

// parseMoov extracts metadata from the moov box.
func (m *CMAFMuxer) parseMoov(data []byte) error {
	if len(data) < 8 {
		return ErrInvalidBoxHeader
	}

	// Initialize the per-track timescales map
	m.initSegment.TrackTimescales = make(map[uint32]uint32)

	// Skip moov header and search for tracks
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
			// Movie header - contains movie timescale (fallback)
			if ts, err := extractTimescale(data[offset:boxEnd]); err == nil {
				m.initSegment.Timescale = ts
			}

		case "trak":
			// Track - extract track ID and per-track timescale
			trackData := data[offset:boxEnd]
			trackID := extractTrackID(trackData)
			trackTimescale := extractTrackTimescale(trackData)

			if trackID > 0 && trackTimescale > 0 {
				m.initSegment.TrackTimescales[trackID] = trackTimescale
			}

			// Check if video or audio and store track ID
			if isVideoTrack(trackData) {
				m.initSegment.HasVideo = true
				m.initSegment.VideoTrackID = trackID
			}
			if isAudioTrack(trackData) {
				m.initSegment.HasAudio = true
				m.initSegment.AudioTrackID = trackID
			}
		}

		offset = boxEnd
	}

	return nil
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
}

// Helper functions for box parsing

// peekBoxHeader reads a box header without consuming data.
func peekBoxHeader(data []byte) (BoxHeader, error) {
	if len(data) < 8 {
		return BoxHeader{}, ErrUnexpectedEOF
	}

	size := binary.BigEndian.Uint32(data[0:4])
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
		header.Size = binary.BigEndian.Uint64(data[8:16])
		header.Extended = true
	} else if size == 0 {
		// Box extends to end of file - can't determine size without more context
		return BoxHeader{}, ErrInvalidBoxHeader
	}

	return header, nil
}

// extractSequenceNumber extracts the sequence number from a moof box.
func extractSequenceNumber(moof []byte) (uint32, error) {
	// moof contains mfhd (movie fragment header) with sequence number
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
			return binary.BigEndian.Uint32(moof[offset+12 : offset+16]), nil
		}

		offset += int(header.Size)
		if header.Size == 0 {
			break
		}
	}

	return 0, errors.New("mfhd not found in moof")
}

// extractTiming extracts decode time, duration, and track ID from a moof box.
func extractTiming(moof []byte) (decodeTime uint64, duration uint64, trackID uint32, err error) {
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
			dt, dur, tid := parseTraf(moof[offset : offset+int(header.Size)])
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
func parseTraf(traf []byte) (decodeTime uint64, duration uint64, trackID uint32) {
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
			// Track fragment header - contains track_id and may contain default sample duration
			if boxEnd >= offset+16 {
				flags := binary.BigEndian.Uint32(traf[offset+8:offset+12]) & 0x00FFFFFF
				// track_id is at offset 12 (after version/flags)
				trackID = binary.BigEndian.Uint32(traf[offset+12 : offset+16])
				tfhdOffset := offset + 16 // After version/flags/track_id

				// Check for optional fields based on flags
				if flags&0x000001 != 0 { // base_data_offset present
					tfhdOffset += 8
				}
				if flags&0x000002 != 0 { // sample_description_index present
					tfhdOffset += 4
				}
				if flags&0x000008 != 0 { // default_sample_duration present
					if tfhdOffset+4 <= boxEnd {
						defaultSampleDuration = binary.BigEndian.Uint32(traf[tfhdOffset : tfhdOffset+4])
					}
				}
			}

		case "tfdt":
			// Track fragment decode time
			if boxEnd >= offset+16 {
				version := traf[offset+8]
				if version == 1 && boxEnd >= offset+20 {
					decodeTime = binary.BigEndian.Uint64(traf[offset+12 : offset+20])
				} else if boxEnd >= offset+16 {
					decodeTime = uint64(binary.BigEndian.Uint32(traf[offset+12 : offset+16]))
				}
			}

		case "trun":
			// Track run - contains sample count and per-sample durations
			if boxEnd > offset+12 {
				flags := binary.BigEndian.Uint32(traf[offset+8:offset+12]) & 0x00FFFFFF
				sampleCount := binary.BigEndian.Uint32(traf[offset+12 : offset+16])

				trunOffset := offset + 16 // After version/flags and sample_count

				// Check for optional fields before sample table
				if flags&0x000001 != 0 { // data_offset present
					trunOffset += 4
				}
				if flags&0x000004 != 0 { // first_sample_flags present
					trunOffset += 4
				}

				// Calculate sample entry size based on flags
				sampleEntrySize := 0
				hasSampleDuration := flags&0x000100 != 0
				hasSampleSize := flags&0x000200 != 0
				hasSampleFlags := flags&0x000400 != 0
				hasSampleCTO := flags&0x000800 != 0

				if hasSampleDuration {
					sampleEntrySize += 4
				}
				if hasSampleSize {
					sampleEntrySize += 4
				}
				if hasSampleFlags {
					sampleEntrySize += 4
				}
				if hasSampleCTO {
					sampleEntrySize += 4
				}

				// Sum up all sample durations
				if hasSampleDuration && sampleEntrySize > 0 {
					for i := uint32(0); i < sampleCount; i++ {
						sampleOffset := trunOffset + int(i)*sampleEntrySize
						if sampleOffset+4 <= boxEnd {
							sampleDuration := binary.BigEndian.Uint32(traf[sampleOffset : sampleOffset+4])
							duration += uint64(sampleDuration)
						}
					}
				} else if defaultSampleDuration > 0 {
					// Use default sample duration from tfhd
					duration = uint64(defaultSampleDuration) * uint64(sampleCount)
				} else {
					// No duration info available - estimate based on typical 30fps video
					// Assume 1001 ticks per frame at 30000 timescale (29.97fps NTSC)
					// or 1000 ticks at 30000 timescale (30fps)
					duration = uint64(sampleCount) * 1001 / 30 // ~33ms per frame
				}
			}
		}

		offset = boxEnd
	}

	return decodeTime, duration, trackID
}

// hasKeyframe checks if a moof contains a sync sample (keyframe).
func hasKeyframe(moof []byte) bool {
	if len(moof) < 8 {
		return false
	}

	// Search for traf with trun that has sync samples
	offset := 8
	for offset+8 < len(moof) {
		header, err := peekBoxHeader(moof[offset:])
		if err != nil || header.Size == 0 {
			break
		}

		if header.Type == "traf" && offset+int(header.Size) <= len(moof) {
			if trafHasKeyframe(moof[offset : offset+int(header.Size)]) {
				return true
			}
		}

		offset += int(header.Size)
	}

	// If no specific indication, assume first fragment has keyframe
	return true
}

// trafHasKeyframe checks if a traf box contains a sync sample.
func trafHasKeyframe(traf []byte) bool {
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
			// Check tr_flags for first_sample_flags or sample_flags
			flags := binary.BigEndian.Uint32(traf[offset+8 : offset+12])

			// Sample flags present (0x400) or first sample flags present (0x4)
			if flags&0x404 != 0 {
				// Need to parse sample flags to determine if sync sample
				// For simplicity, assume if flags are present, check first sample
				return true // Simplified - assume keyframe if sample flags present
			}
		}

		offset += int(header.Size)
	}

	return true // Default to true for first fragment
}

// extractTimescale extracts the timescale from a mvhd box.
func extractTimescale(mvhd []byte) (uint32, error) {
	if len(mvhd) < 20 {
		return 0, ErrInvalidBoxHeader
	}

	// mvhd: version(1) + flags(3) + create_time + mod_time + timescale
	version := mvhd[8]
	if version == 1 {
		// 64-bit times
		if len(mvhd) < 32 {
			return 0, ErrInvalidBoxHeader
		}
		return binary.BigEndian.Uint32(mvhd[28:32]), nil
	}

	// 32-bit times
	if len(mvhd) < 24 {
		return 0, ErrInvalidBoxHeader
	}
	return binary.BigEndian.Uint32(mvhd[20:24]), nil
}

// isVideoTrack checks if a trak box contains a video track.
func isVideoTrack(trak []byte) bool {
	return findHandler(trak) == "vide"
}

// isAudioTrack checks if a trak box contains an audio track.
func isAudioTrack(trak []byte) bool {
	return findHandler(trak) == "soun"
}

// extractTrackID extracts the track ID from a trak box (from tkhd sub-box).
func extractTrackID(trak []byte) uint32 {
	if len(trak) < 8 {
		return 0
	}

	// Search for tkhd inside trak
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

		if header.Type == "tkhd" {
			// tkhd: version(1) + flags(3) + ...
			// version 0: creation_time(4) + mod_time(4) + track_id(4)
			// version 1: creation_time(8) + mod_time(8) + track_id(4)
			if boxEnd < offset+20 {
				break
			}
			version := trak[offset+8]
			if version == 0 {
				// track_id at offset 8+1+3+4+4 = 20
				if boxEnd >= offset+24 {
					return binary.BigEndian.Uint32(trak[offset+20 : offset+24])
				}
			} else {
				// version 1: track_id at offset 8+1+3+8+8 = 28
				if boxEnd >= offset+32 {
					return binary.BigEndian.Uint32(trak[offset+28 : offset+32])
				}
			}
		}

		offset = boxEnd
	}

	return 0
}

// extractTrackTimescale extracts the timescale from a trak box (from mdia/mdhd sub-box).
func extractTrackTimescale(trak []byte) uint32 {
	if len(trak) < 8 {
		return 0
	}

	// Search for mdia inside trak
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
			// Search inside mdia for mdhd
			mdiaOffset := offset + 8
			for mdiaOffset+8 < boxEnd {
				innerHeader, err := peekBoxHeader(trak[mdiaOffset:])
				if err != nil || innerHeader.Size == 0 {
					break
				}

				innerEnd := mdiaOffset + int(innerHeader.Size)
				if innerEnd > boxEnd {
					break
				}

				if innerHeader.Type == "mdhd" {
					// mdhd: version(1) + flags(3) + ...
					// version 0: creation_time(4) + mod_time(4) + timescale(4)
					// version 1: creation_time(8) + mod_time(8) + timescale(4)
					if innerEnd < mdiaOffset+20 {
						break
					}
					version := trak[mdiaOffset+8]
					if version == 0 {
						// timescale at offset 8+1+3+4+4 = 20
						if innerEnd >= mdiaOffset+24 {
							return binary.BigEndian.Uint32(trak[mdiaOffset+20 : mdiaOffset+24])
						}
					} else {
						// version 1: timescale at offset 8+1+3+8+8 = 28
						if innerEnd >= mdiaOffset+32 {
							return binary.BigEndian.Uint32(trak[mdiaOffset+28 : mdiaOffset+32])
						}
					}
				}

				mdiaOffset = innerEnd
			}
		}

		offset = boxEnd
	}

	return 0
}

// findHandler finds the handler type in a trak box.
func findHandler(trak []byte) string {
	if len(trak) < 8 {
		return ""
	}

	// Search for mdia -> hdlr
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
			// Search inside mdia for hdlr
			mdiaOffset := offset + 8
			for mdiaOffset+8 < boxEnd {
				innerHeader, err := peekBoxHeader(trak[mdiaOffset:])
				if err != nil || innerHeader.Size == 0 {
					break
				}

				innerEnd := mdiaOffset + int(innerHeader.Size)
				if innerEnd > boxEnd {
					break
				}

				if innerHeader.Type == "hdlr" && innerEnd >= mdiaOffset+20 {
					// hdlr: version(1) + flags(3) + pre_defined(4) + handler_type(4)
					return string(trak[mdiaOffset+16 : mdiaOffset+20])
				}

				mdiaOffset = innerEnd
			}
		}

		offset = boxEnd
	}

	return ""
}
