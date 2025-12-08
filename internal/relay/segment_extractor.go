// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// MPEG-TS constants.
const (
	// TSPacketSize is the standard MPEG-TS packet size.
	TSPacketSize = 188

	// TSSyncByte is the sync byte that starts every TS packet.
	TSSyncByte = 0x47

	// TSPacketHeaderSize is the minimum TS packet header size.
	TSPacketHeaderSize = 4

	// PIDPATTable is the Program Association Table PID.
	PIDPATTable = 0x0000

	// DefaultSegmentTargetDuration is the default segment duration in seconds.
	DefaultSegmentTargetDuration = 6.0
)

// SegmentExtractorConfig configures the segment extractor.
type SegmentExtractorConfig struct {
	// TargetDuration is the target segment duration in seconds.
	TargetDuration float64

	// Buffer is the segment buffer to write extracted segments to.
	Buffer *SegmentBuffer

	// MinSegmentDuration is the minimum segment duration (to avoid tiny segments).
	MinSegmentDuration float64

	// MaxSegmentDuration is the maximum segment duration before forced split.
	MaxSegmentDuration float64
}

// DefaultSegmentExtractorConfig returns default configuration.
func DefaultSegmentExtractorConfig(buffer *SegmentBuffer) SegmentExtractorConfig {
	return SegmentExtractorConfig{
		TargetDuration:     DefaultSegmentTargetDuration,
		Buffer:             buffer,
		MinSegmentDuration: 1.0,  // At least 1 second
		MaxSegmentDuration: 12.0, // At most 12 seconds
	}
}

// SegmentExtractor extracts segments from a continuous MPEG-TS stream.
// It detects keyframes (IDR frames) and splits the stream into segments
// that start on keyframe boundaries for clean playback.
type SegmentExtractor struct {
	config SegmentExtractorConfig

	mu            sync.Mutex
	currentBuffer bytes.Buffer
	startTime     time.Time
	packetCount   int
	lastPTS       int64
	keyframeFound bool

	// Stats
	totalPackets   int64
	totalSegments  int64
	totalBytes     int64
	lastSegmentAt  time.Time
	avgSegDuration float64
}

// NewSegmentExtractor creates a new segment extractor.
func NewSegmentExtractor(config SegmentExtractorConfig) *SegmentExtractor {
	if config.TargetDuration <= 0 {
		config.TargetDuration = DefaultSegmentTargetDuration
	}
	if config.MinSegmentDuration <= 0 {
		config.MinSegmentDuration = 1.0
	}
	if config.MaxSegmentDuration <= 0 {
		config.MaxSegmentDuration = config.TargetDuration * 2
	}

	return &SegmentExtractor{
		config:    config,
		startTime: time.Now(),
	}
}

// Write implements io.Writer to receive MPEG-TS data.
// It processes the incoming stream and extracts segments.
func (e *SegmentExtractor) Write(data []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.config.Buffer == nil {
		return 0, io.ErrClosedPipe
	}

	n := len(data)
	e.currentBuffer.Write(data)
	e.totalBytes += int64(n)

	// Process complete TS packets
	e.processPackets()

	return n, nil
}

// processPackets processes complete TS packets from the buffer.
func (e *SegmentExtractor) processPackets() {
	buf := e.currentBuffer.Bytes()

	// Find and process complete TS packets
	offset := 0
	for offset+TSPacketSize <= len(buf) {
		// Find sync byte
		if buf[offset] != TSSyncByte {
			// Scan for sync byte
			syncFound := false
			for i := offset + 1; i < len(buf); i++ {
				if buf[i] == TSSyncByte {
					offset = i
					syncFound = true
					break
				}
			}
			if !syncFound {
				break
			}
		}

		// Process packet
		packet := buf[offset : offset+TSPacketSize]
		e.processPacket(packet)
		offset += TSPacketSize
	}

	// Keep remaining incomplete data
	if offset > 0 {
		remaining := buf[offset:]
		e.currentBuffer.Reset()
		e.currentBuffer.Write(remaining)
	}
}

// processPacket processes a single TS packet.
func (e *SegmentExtractor) processPacket(packet []byte) {
	e.packetCount++
	e.totalPackets++

	// Extract PID
	pid := (uint16(packet[1]&0x1F) << 8) | uint16(packet[2])

	// Check for adaptation field with PCR or keyframe indicator
	hasAdaptationField := (packet[3] & 0x20) != 0
	hasPayload := (packet[3] & 0x10) != 0

	isKeyframe := false
	if hasAdaptationField && len(packet) > 4 {
		adaptLen := int(packet[4])
		if adaptLen > 0 && len(packet) > 5 {
			adaptFlags := packet[5]
			// Random access indicator (keyframe)
			if (adaptFlags & 0x40) != 0 {
				isKeyframe = true
			}
		}
	}

	// Check for video payload that might contain keyframe
	if hasPayload && !isKeyframe {
		// Video streams typically have PID in range 0x100-0x1FFF
		// We detect keyframes by looking for NAL unit type 5 (IDR) in H.264
		// or NAL unit type 19/20 (IDR/CRA) in H.265
		if pid >= 0x100 && pid <= 0x1FFF {
			isKeyframe = e.detectKeyframeInPayload(packet)
		}
	}

	// Calculate elapsed time based on packets (estimate ~1316 bytes/packet at typical bitrate)
	// This is a rough estimate; actual duration should come from PTS when available
	packetsPerSecond := 1000.0 // Rough estimate for 1.5 Mbps stream
	estimatedDuration := float64(e.packetCount) / packetsPerSecond

	// Check if we should emit a segment
	shouldEmit := false
	if e.packetCount > 0 {
		// Emit on keyframe if we've accumulated enough duration
		if isKeyframe && estimatedDuration >= e.config.MinSegmentDuration {
			shouldEmit = true
			e.keyframeFound = true
		}
		// Force emit if we've exceeded max duration
		if estimatedDuration >= e.config.MaxSegmentDuration {
			shouldEmit = true
		}
		// Emit at target duration if we've seen at least one keyframe
		if e.keyframeFound && estimatedDuration >= e.config.TargetDuration && isKeyframe {
			shouldEmit = true
		}
	}

	if shouldEmit {
		e.emitSegment(estimatedDuration, isKeyframe)
	}
}

// detectKeyframeInPayload attempts to detect keyframes in the payload.
// This is a simplified detection that looks for H.264/H.265 IDR NAL units.
func (e *SegmentExtractor) detectKeyframeInPayload(packet []byte) bool {
	// Skip TS header
	headerLen := 4
	if (packet[3] & 0x20) != 0 { // Adaptation field present
		if len(packet) > 4 {
			headerLen += 1 + int(packet[4]) // Skip adaptation field
		}
	}

	if headerLen >= len(packet) {
		return false
	}

	payload := packet[headerLen:]

	// Look for start codes (0x00 0x00 0x01 or 0x00 0x00 0x00 0x01)
	for i := 0; i < len(payload)-4; i++ {
		if payload[i] == 0x00 && payload[i+1] == 0x00 {
			nalStart := -1
			if payload[i+2] == 0x01 {
				nalStart = i + 3
			} else if payload[i+2] == 0x00 && i+3 < len(payload) && payload[i+3] == 0x01 {
				nalStart = i + 4
			}

			if nalStart > 0 && nalStart < len(payload) {
				nalType := payload[nalStart] & 0x1F
				// H.264 NAL type 5 is IDR
				if nalType == 5 {
					return true
				}
				// H.265 NAL types 19-21 are IDR/CRA
				h265Type := (payload[nalStart] >> 1) & 0x3F
				if h265Type >= 19 && h265Type <= 21 {
					return true
				}
			}
		}
	}

	return false
}

// emitSegment creates a segment from accumulated data.
func (e *SegmentExtractor) emitSegment(duration float64, isKeyframe bool) {
	// Get all accumulated data up to current point
	// Note: In a real implementation, we'd buffer data since last emit
	// For now, we're using a simplified approach

	// Create a copy of the buffered data
	data := make([]byte, e.currentBuffer.Len())
	copy(data, e.currentBuffer.Bytes())

	if len(data) == 0 {
		return
	}

	seg := Segment{
		Duration:   duration,
		Data:       data,
		Timestamp:  time.Now(),
		IsKeyframe: isKeyframe,
		PTS:        e.lastPTS,
		DTS:        e.lastPTS,
	}

	// Add to buffer
	if err := e.config.Buffer.AddSegment(seg); err == nil {
		e.totalSegments++
		e.lastSegmentAt = time.Now()

		// Update average duration
		if e.totalSegments > 1 {
			e.avgSegDuration = (e.avgSegDuration*float64(e.totalSegments-1) + duration) / float64(e.totalSegments)
		} else {
			e.avgSegDuration = duration
		}
	}

	// Reset for next segment
	e.currentBuffer.Reset()
	e.packetCount = 0
	e.keyframeFound = false
}

// Flush forces emission of any remaining buffered data as a segment.
func (e *SegmentExtractor) Flush() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.currentBuffer.Len() > 0 {
		packetsPerSecond := 1000.0
		estimatedDuration := float64(e.packetCount) / packetsPerSecond
		e.emitSegment(estimatedDuration, false)
	}
}

// Stats returns extraction statistics.
func (e *SegmentExtractor) Stats() SegmentExtractorStats {
	e.mu.Lock()
	defer e.mu.Unlock()

	return SegmentExtractorStats{
		TotalPackets:       e.totalPackets,
		TotalSegments:      e.totalSegments,
		TotalBytes:         e.totalBytes,
		AverageSegDuration: e.avgSegDuration,
		LastSegmentAt:      e.lastSegmentAt,
		BufferedBytes:      e.currentBuffer.Len(),
		BufferedPackets:    e.packetCount,
		RunningTime:        time.Since(e.startTime),
		TargetDuration:     e.config.TargetDuration,
	}
}

// SegmentExtractorStats holds extractor statistics.
type SegmentExtractorStats struct {
	TotalPackets       int64
	TotalSegments      int64
	TotalBytes         int64
	AverageSegDuration float64
	LastSegmentAt      time.Time
	BufferedBytes      int
	BufferedPackets    int
	RunningTime        time.Duration
	TargetDuration     float64
}
