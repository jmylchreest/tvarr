// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// SegmentBuffer errors.
// Note: ErrBufferClosed and ErrClientNotFound are defined in cyclic_buffer.go
var (
	ErrSegmentNotFound = errors.New("segment not found")
	ErrSegmentEmpty    = errors.New("segment is empty")
	ErrBufferFull      = errors.New("buffer is full")
	ErrInvalidSequence = errors.New("invalid segment sequence")
)

// SegmentBufferConfig configures the segment buffer.
type SegmentBufferConfig struct {
	// MaxSegments is the maximum number of segments to keep.
	MaxSegments int

	// TargetDuration is the target segment duration (seconds).
	TargetDuration int

	// MaxBufferSize is the maximum total buffer size in bytes.
	MaxBufferSize int64
}

// DefaultSegmentBufferConfig returns defaults per spec requirements.
func DefaultSegmentBufferConfig() SegmentBufferConfig {
	return SegmentBufferConfig{
		MaxSegments:    DefaultPlaylistSize,    // FR-012: 5 segments
		TargetDuration: DefaultSegmentDuration, // FR-011: 6 seconds
		MaxBufferSize:  DefaultMaxBufferSize,   // SC-004: 100MB per stream
	}
}

// SegmentBufferStats holds buffer statistics.
type SegmentBufferStats struct {
	// SegmentCount is the current number of segments in the buffer.
	SegmentCount int

	// TotalBytes is the total bytes added to the buffer.
	TotalBytes uint64

	// CurrentSize is the current buffer size in bytes.
	CurrentSize int64

	// FirstSequence is the oldest available segment sequence.
	FirstSequence uint64

	// LastSequence is the newest available segment sequence.
	LastSequence uint64

	// ClientCount is the number of connected clients.
	ClientCount int

	// CreatedAt is when the buffer was created.
	CreatedAt time.Time
}

// SegmentBuffer manages segments for HLS/DASH delivery.
type SegmentBuffer struct {
	config   SegmentBufferConfig
	mu       sync.RWMutex
	segments []Segment
	sequence atomic.Uint64
	closed   bool

	// Clients tracking
	clientsMu sync.RWMutex
	clients   map[uuid.UUID]*SegmentClient

	// Metrics
	totalBytes  atomic.Uint64
	currentSize atomic.Int64

	// Timestamps
	createdAt time.Time
}

// NewSegmentBuffer creates a new segment buffer.
func NewSegmentBuffer(config SegmentBufferConfig) *SegmentBuffer {
	if config.MaxSegments <= 0 {
		config.MaxSegments = DefaultPlaylistSize
	}
	if config.TargetDuration <= 0 {
		config.TargetDuration = DefaultSegmentDuration
	}
	if config.MaxBufferSize <= 0 {
		config.MaxBufferSize = DefaultMaxBufferSize
	}

	return &SegmentBuffer{
		config:    config,
		segments:  make([]Segment, 0, config.MaxSegments),
		clients:   make(map[uuid.UUID]*SegmentClient),
		createdAt: time.Now(),
	}
}

// AddSegment adds a segment to the buffer.
// If the buffer is full, the oldest segment is evicted.
func (sb *SegmentBuffer) AddSegment(seg Segment) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.closed {
		return ErrBufferClosed
	}

	if seg.IsEmpty() {
		return ErrSegmentEmpty
	}

	// Assign sequence number
	seg.Sequence = sb.sequence.Add(1)
	seg.Timestamp = time.Now()

	// Track size
	segSize := int64(seg.Size())
	sb.totalBytes.Add(uint64(segSize))

	// Evict oldest if at capacity
	for len(sb.segments) >= sb.config.MaxSegments {
		evicted := sb.segments[0]
		sb.segments = sb.segments[1:]
		sb.currentSize.Add(-int64(evicted.Size()))
	}

	// Check total buffer size limit
	for sb.currentSize.Load()+segSize > sb.config.MaxBufferSize && len(sb.segments) > 0 {
		evicted := sb.segments[0]
		sb.segments = sb.segments[1:]
		sb.currentSize.Add(-int64(evicted.Size()))
	}

	// Add new segment
	sb.segments = append(sb.segments, seg)
	sb.currentSize.Add(segSize)

	return nil
}

// GetSegment retrieves a segment by sequence number.
func (sb *SegmentBuffer) GetSegment(sequence uint64) (*Segment, error) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if sb.closed {
		return nil, ErrBufferClosed
	}

	if len(sb.segments) == 0 {
		return nil, ErrSegmentNotFound
	}

	// Binary search since segments are in order
	firstSeq := sb.segments[0].Sequence
	lastSeq := sb.segments[len(sb.segments)-1].Sequence

	if sequence < firstSeq || sequence > lastSeq {
		return nil, ErrSegmentNotFound
	}

	// Calculate index (sequences are contiguous)
	idx := int(sequence - firstSeq)
	if idx < 0 || idx >= len(sb.segments) {
		return nil, ErrSegmentNotFound
	}

	// Verify sequence matches (defensive)
	if sb.segments[idx].Sequence != sequence {
		// Fall back to linear search
		for i := range sb.segments {
			if sb.segments[i].Sequence == sequence {
				return &sb.segments[i], nil
			}
		}
		return nil, ErrSegmentNotFound
	}

	return &sb.segments[idx], nil
}

// GetSegments returns all available segments.
func (sb *SegmentBuffer) GetSegments() []Segment {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if sb.closed || len(sb.segments) == 0 {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]Segment, len(sb.segments))
	copy(result, sb.segments)
	return result
}

// GetLatestSegments returns the most recent n segments.
func (sb *SegmentBuffer) GetLatestSegments(n int) []Segment {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if sb.closed || len(sb.segments) == 0 {
		return nil
	}

	if n > len(sb.segments) {
		n = len(sb.segments)
	}

	start := len(sb.segments) - n
	result := make([]Segment, n)
	copy(result, sb.segments[start:])
	return result
}

// FirstSequence returns the sequence number of the oldest segment.
func (sb *SegmentBuffer) FirstSequence() (uint64, bool) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if len(sb.segments) == 0 {
		return 0, false
	}
	return sb.segments[0].Sequence, true
}

// LastSequence returns the sequence number of the newest segment.
func (sb *SegmentBuffer) LastSequence() (uint64, bool) {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if len(sb.segments) == 0 {
		return 0, false
	}
	return sb.segments[len(sb.segments)-1].Sequence, true
}

// TargetDuration returns the target segment duration.
func (sb *SegmentBuffer) TargetDuration() int {
	return sb.config.TargetDuration
}

// GetSegmentInfos returns segment metadata for playlist generation.
// Implements the SegmentProvider interface.
func (sb *SegmentBuffer) GetSegmentInfos() []SegmentInfo {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if sb.closed || len(sb.segments) == 0 {
		return nil
	}

	result := make([]SegmentInfo, len(sb.segments))
	for i, seg := range sb.segments {
		result[i] = SegmentInfo{
			Sequence:      seg.Sequence,
			Duration:      seg.Duration,
			IsKeyframe:    seg.IsKeyframe,
			Timestamp:     seg.Timestamp,
			Discontinuity: seg.Discontinuity,
		}
	}
	return result
}

// Close closes the buffer and releases resources.
func (sb *SegmentBuffer) Close() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.closed = true
	sb.segments = nil
	sb.currentSize.Store(0)

	// Clear clients
	sb.clientsMu.Lock()
	sb.clients = make(map[uuid.UUID]*SegmentClient)
	sb.clientsMu.Unlock()
}

// IsClosed returns true if the buffer is closed.
func (sb *SegmentBuffer) IsClosed() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.closed
}

// Stats returns buffer statistics.
func (sb *SegmentBuffer) Stats() SegmentBufferStats {
	sb.mu.RLock()
	segCount := len(sb.segments)
	var firstSeq, lastSeq uint64
	if segCount > 0 {
		firstSeq = sb.segments[0].Sequence
		lastSeq = sb.segments[segCount-1].Sequence
	}
	sb.mu.RUnlock()

	sb.clientsMu.RLock()
	clientCount := len(sb.clients)
	sb.clientsMu.RUnlock()

	return SegmentBufferStats{
		SegmentCount:  segCount,
		TotalBytes:    sb.totalBytes.Load(),
		CurrentSize:   sb.currentSize.Load(),
		FirstSequence: firstSeq,
		LastSequence:  lastSeq,
		ClientCount:   clientCount,
		CreatedAt:     sb.createdAt,
	}
}

// SegmentClient tracks a client's position in the segment buffer.
type SegmentClient struct {
	ID          uuid.UUID
	UserAgent   string
	RemoteAddr  string
	ConnectedAt time.Time
	LastRequest time.Time

	lastSegment atomic.Uint64 // Last segment sequence requested
	bytesServed atomic.Uint64
}

// LastSegment returns the last segment sequence requested by this client.
func (c *SegmentClient) LastSegment() uint64 {
	return c.lastSegment.Load()
}

// SetLastSegment sets the last segment sequence requested.
func (c *SegmentClient) SetLastSegment(seq uint64) {
	c.lastSegment.Store(seq)
	c.LastRequest = time.Now()
}

// BytesServed returns the total bytes served to this client.
func (c *SegmentClient) BytesServed() uint64 {
	return c.bytesServed.Load()
}

// AddBytesServed adds to the bytes served counter.
func (c *SegmentClient) AddBytesServed(n uint64) {
	c.bytesServed.Add(n)
}

// AddClient adds a client to track.
func (sb *SegmentBuffer) AddClient(userAgent, remoteAddr string) (*SegmentClient, error) {
	sb.mu.RLock()
	closed := sb.closed
	sb.mu.RUnlock()

	if closed {
		return nil, ErrBufferClosed
	}

	client := &SegmentClient{
		ID:          uuid.New(),
		UserAgent:   userAgent,
		RemoteAddr:  remoteAddr,
		ConnectedAt: time.Now(),
		LastRequest: time.Now(),
	}

	sb.clientsMu.Lock()
	sb.clients[client.ID] = client
	sb.clientsMu.Unlock()

	return client, nil
}

// GetClient retrieves a client by ID.
func (sb *SegmentBuffer) GetClient(clientID uuid.UUID) (*SegmentClient, error) {
	sb.clientsMu.RLock()
	defer sb.clientsMu.RUnlock()

	client, ok := sb.clients[clientID]
	if !ok {
		return nil, ErrClientNotFound
	}
	return client, nil
}

// RemoveClient removes a client.
func (sb *SegmentBuffer) RemoveClient(clientID uuid.UUID) bool {
	sb.clientsMu.Lock()
	defer sb.clientsMu.Unlock()

	_, ok := sb.clients[clientID]
	if ok {
		delete(sb.clients, clientID)
	}
	return ok
}

// ClientCount returns the number of connected clients.
func (sb *SegmentBuffer) ClientCount() int {
	sb.clientsMu.RLock()
	defer sb.clientsMu.RUnlock()
	return len(sb.clients)
}
