// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// UnifiedBufferConfig configures the unified buffer.
type UnifiedBufferConfig struct {
	// MaxBufferSize is the maximum buffer size in bytes.
	MaxBufferSize int64

	// MaxChunks is the maximum number of chunks to keep.
	MaxChunks int

	// ChunkTimeout is how long to keep chunks before expiry.
	ChunkTimeout time.Duration

	// TargetSegmentDuration is the target segment duration in seconds for HLS/DASH.
	TargetSegmentDuration int

	// MaxSegments is the maximum number of segments to keep for playlists.
	MaxSegments int

	// CleanupInterval is how often to run cleanup.
	CleanupInterval time.Duration

	// ContainerFormat specifies the container format for segments.
	// When set to "fmp4", the buffer uses the CMAF muxer for segment detection.
	ContainerFormat string
}

// DefaultUnifiedBufferConfig returns sensible defaults.
func DefaultUnifiedBufferConfig() UnifiedBufferConfig {
	return UnifiedBufferConfig{
		MaxBufferSize:         100 * 1024 * 1024, // 100MB (SC-004)
		MaxChunks:             2000,
		ChunkTimeout:          120 * time.Second,
		TargetSegmentDuration: DefaultSegmentDuration, // 6 seconds
		MaxSegments:           DefaultPlaylistSize,    // 5 segments
		CleanupInterval:       5 * time.Second,
	}
}

// Chunk represents a piece of stream data in the buffer.
type Chunk struct {
	Sequence  uint64
	Data      []byte
	Timestamp time.Time
}

// SegmentMarker marks the boundaries of a segment within the chunk buffer.
// A segment is a contiguous range of chunks that form a playable unit.
type SegmentMarker struct {
	Sequence      uint64    // Segment sequence number
	StartChunk    uint64    // First chunk sequence in this segment
	EndChunk      uint64    // Last chunk sequence in this segment (inclusive)
	Duration      float64   // Segment duration in seconds
	Timestamp     time.Time // When segment was created
	IsKeyframe    bool      // Whether segment starts with a keyframe
	ByteSize      int       // Total bytes in segment
	Discontinuity bool      // Whether this segment marks a discontinuity
}

// UnifiedBuffer provides a single buffer that serves both continuous
// MPEG-TS streaming and segment-based HLS/DASH delivery.
type UnifiedBuffer struct {
	config UnifiedBufferConfig

	// Chunk storage
	chunkMu       sync.RWMutex
	chunks        []Chunk
	chunkSequence atomic.Uint64

	// Segment index (marks boundaries within chunks)
	segmentMu       sync.RWMutex
	segments        []SegmentMarker
	segmentSequence atomic.Uint64

	// Segment accumulation state
	accumMu            sync.Mutex
	accumStartChunk    uint64
	accumBytes         int
	accumStartTime     time.Time
	accumKeyframeFound bool
	nextDiscontinuity  bool // Mark next segment as discontinuity

	// Client tracking
	clientsMu sync.RWMutex
	clients   map[uuid.UUID]*UnifiedClient

	// Statistics
	totalBytesWritten  atomic.Uint64
	totalChunksWritten atomic.Uint64

	// Lifecycle
	closed bool
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Logging
	logger *slog.Logger

	// fMP4/CMAF support
	cmafMuxer   *CMAFMuxer   // Muxer for fMP4 parsing
	initSegment *InitSegment // Cached init segment for fMP4
}

// NewUnifiedBuffer creates a new unified buffer.
func NewUnifiedBuffer(config UnifiedBufferConfig) *UnifiedBuffer {
	return NewUnifiedBufferWithLogger(config, nil)
}

// NewUnifiedBufferWithLogger creates a new unified buffer with a custom logger.
func NewUnifiedBufferWithLogger(config UnifiedBufferConfig, logger *slog.Logger) *UnifiedBuffer {
	if config.MaxBufferSize <= 0 {
		config.MaxBufferSize = DefaultUnifiedBufferConfig().MaxBufferSize
	}
	if config.MaxChunks <= 0 {
		config.MaxChunks = DefaultUnifiedBufferConfig().MaxChunks
	}
	if config.TargetSegmentDuration <= 0 {
		config.TargetSegmentDuration = DefaultSegmentDuration
	}
	if config.MaxSegments <= 0 {
		config.MaxSegments = DefaultPlaylistSize
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	ub := &UnifiedBuffer{
		config:   config,
		chunks:   make([]Chunk, 0, config.MaxChunks),
		segments: make([]SegmentMarker, 0, config.MaxSegments),
		clients:  make(map[uuid.UUID]*UnifiedClient),
		stopCh:   make(chan struct{}),
		logger:   logger.With(slog.String("component", "unified_buffer")),
	}

	// Initialize CMAF muxer for fMP4 mode
	if config.ContainerFormat == "fmp4" {
		cmafConfig := DefaultCMAFMuxerConfig()
		cmafConfig.MaxFragments = config.MaxSegments
		ub.cmafMuxer = NewCMAFMuxer(cmafConfig)
		ub.logger.Info("initialized CMAF muxer for fMP4 mode",
			slog.Int("max_fragments", cmafConfig.MaxFragments))
	}

	// Start cleanup goroutine
	ub.wg.Add(1)
	go ub.cleanupLoop()

	return ub
}

// WriteChunk writes a chunk of data to the buffer.
// This is the primary write path - all stream data comes through here.
// For fMP4 mode, data is passed to the CMAF muxer for parsing.
func (ub *UnifiedBuffer) WriteChunk(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// For fMP4 mode, route through CMAF muxer
	if ub.cmafMuxer != nil {
		return ub.writeFMP4Chunk(data)
	}

	ub.chunkMu.Lock()
	if ub.closed {
		ub.chunkMu.Unlock()
		return ErrBufferClosed
	}

	// Create chunk with copied data
	seq := ub.chunkSequence.Add(1)
	chunk := Chunk{
		Sequence:  seq,
		Data:      make([]byte, len(data)),
		Timestamp: time.Now(),
	}
	copy(chunk.Data, data)

	// Add to buffer
	ub.chunks = append(ub.chunks, chunk)
	ub.totalBytesWritten.Add(uint64(len(data)))
	ub.totalChunksWritten.Add(1)

	// Enforce chunk limits
	ub.enforceChunkLimits()
	ub.chunkMu.Unlock()

	// Update segment accumulation
	ub.updateSegmentAccumulation(seq, len(data), data)

	// Notify waiting clients
	ub.notifyClients()

	return nil
}

// writeFMP4Chunk writes data to the CMAF muxer and creates segments from fragments.
func (ub *UnifiedBuffer) writeFMP4Chunk(data []byte) error {
	ub.chunkMu.Lock()
	if ub.closed {
		ub.chunkMu.Unlock()
		return ErrBufferClosed
	}
	ub.chunkMu.Unlock()

	// Track fragment count before write
	fragCountBefore := ub.cmafMuxer.FragmentCount()

	// Write to CMAF muxer
	n, err := ub.cmafMuxer.Write(data)
	if err != nil {
		return err
	}

	ub.totalBytesWritten.Add(uint64(n))

	// Check if init segment is now available
	if ub.initSegment == nil && ub.cmafMuxer.HasInitSegment() {
		cmafInit := ub.cmafMuxer.GetInitSegment()
		if cmafInit != nil {
			ub.initSegment = &InitSegment{
				Data:      cmafInit.Data,
				Timestamp: time.Now(),
				HasVideo:  cmafInit.HasVideo,
				HasAudio:  cmafInit.HasAudio,
				Timescale: cmafInit.Timescale,
			}
			ub.logger.Info("fMP4 init segment captured",
				slog.Int("size", len(cmafInit.Data)),
				slog.Bool("has_video", cmafInit.HasVideo),
				slog.Bool("has_audio", cmafInit.HasAudio),
				slog.Uint64("timescale", uint64(cmafInit.Timescale)))
		}
	}

	// Check for new fragments and create segments
	fragCountAfter := ub.cmafMuxer.FragmentCount()
	if fragCountAfter > fragCountBefore {
		// New fragment(s) available - create segment markers
		fragments := ub.cmafMuxer.GetFragments()
		for i := fragCountBefore; i < fragCountAfter && i < len(fragments); i++ {
			frag := fragments[i]
			ub.createFMP4Segment(frag)
		}
	}

	// Notify waiting clients
	ub.notifyClients()

	return nil
}

// createFMP4Segment creates a segment marker from an fMP4 fragment.
func (ub *UnifiedBuffer) createFMP4Segment(frag *FMP4Fragment) {
	// Store fragment data as a chunk
	ub.chunkMu.Lock()
	seq := ub.chunkSequence.Add(1)
	chunk := Chunk{
		Sequence:  seq,
		Data:      make([]byte, len(frag.Data)),
		Timestamp: time.Now(),
	}
	copy(chunk.Data, frag.Data)
	ub.chunks = append(ub.chunks, chunk)
	ub.totalChunksWritten.Add(1)
	ub.enforceChunkLimits()
	ub.chunkMu.Unlock()

	// Calculate duration from fragment
	var duration float64
	if ub.initSegment != nil && ub.initSegment.Timescale > 0 {
		duration = float64(frag.Duration) / float64(ub.initSegment.Timescale)
	} else {
		duration = float64(ub.config.TargetSegmentDuration)
	}

	// Create segment marker
	marker := SegmentMarker{
		Sequence:      ub.segmentSequence.Add(1),
		StartChunk:    seq,
		EndChunk:      seq, // fMP4 segment is a single chunk
		Duration:      duration,
		Timestamp:     time.Now(),
		IsKeyframe:    frag.IsKeyframe,
		ByteSize:      len(frag.Data),
		Discontinuity: ub.nextDiscontinuity,
	}

	ub.segmentMu.Lock()
	ub.segments = append(ub.segments, marker)

	// Enforce segment limit
	for len(ub.segments) > ub.config.MaxSegments {
		ub.segments = ub.segments[1:]
	}
	ub.segmentMu.Unlock()

	ub.accumMu.Lock()
	ub.nextDiscontinuity = false
	ub.accumMu.Unlock()

	ub.logger.Debug("fMP4 segment created",
		slog.Uint64("sequence", marker.Sequence),
		slog.Uint64("frag_sequence", uint64(frag.SequenceNumber)),
		slog.Float64("duration", duration),
		slog.Int("bytes", len(frag.Data)),
		slog.Bool("keyframe", frag.IsKeyframe))
}

// WriteChunkWithKeyframe writes a chunk and marks it as containing a keyframe.
// This triggers segment boundary detection.
func (ub *UnifiedBuffer) WriteChunkWithKeyframe(data []byte, isKeyframe bool) error {
	if len(data) == 0 {
		return nil
	}

	ub.chunkMu.Lock()
	if ub.closed {
		ub.chunkMu.Unlock()
		return ErrBufferClosed
	}

	seq := ub.chunkSequence.Add(1)
	chunk := Chunk{
		Sequence:  seq,
		Data:      make([]byte, len(data)),
		Timestamp: time.Now(),
	}
	copy(chunk.Data, data)

	ub.chunks = append(ub.chunks, chunk)
	ub.totalBytesWritten.Add(uint64(len(data)))
	ub.totalChunksWritten.Add(1)

	ub.enforceChunkLimits()
	ub.chunkMu.Unlock()

	// Update segment accumulation with keyframe info
	ub.updateSegmentAccumulationWithKeyframe(seq, len(data), isKeyframe)

	ub.notifyClients()

	return nil
}

// updateSegmentAccumulation tracks chunks for segment creation.
func (ub *UnifiedBuffer) updateSegmentAccumulation(chunkSeq uint64, size int, data []byte) {
	ub.accumMu.Lock()
	defer ub.accumMu.Unlock()

	// Initialize if needed
	if ub.accumStartChunk == 0 {
		ub.accumStartChunk = chunkSeq
		ub.accumStartTime = time.Now()
	}

	ub.accumBytes += size

	// Detect keyframe in data (simple heuristic based on MPEG-TS)
	isKeyframe := detectKeyframeInChunk(data)
	if isKeyframe {
		ub.accumKeyframeFound = true
	}

	// Check if we should emit a segment
	elapsed := time.Since(ub.accumStartTime).Seconds()
	shouldEmit := false

	// Emit on keyframe after target duration
	if isKeyframe && ub.accumKeyframeFound && elapsed >= float64(ub.config.TargetSegmentDuration) {
		shouldEmit = true
	}
	// Force emit at 2x target duration
	if elapsed >= float64(ub.config.TargetSegmentDuration)*2 {
		shouldEmit = true
	}

	if shouldEmit && ub.accumBytes > 0 {
		ub.emitSegment(chunkSeq, elapsed, ub.accumKeyframeFound)
	}
}

// updateSegmentAccumulationWithKeyframe tracks chunks with explicit keyframe marking.
func (ub *UnifiedBuffer) updateSegmentAccumulationWithKeyframe(chunkSeq uint64, size int, isKeyframe bool) {
	ub.accumMu.Lock()
	defer ub.accumMu.Unlock()

	if ub.accumStartChunk == 0 {
		ub.accumStartChunk = chunkSeq
		ub.accumStartTime = time.Now()
	}

	ub.accumBytes += size

	if isKeyframe {
		ub.accumKeyframeFound = true
	}

	elapsed := time.Since(ub.accumStartTime).Seconds()
	shouldEmit := false

	if isKeyframe && elapsed >= float64(ub.config.TargetSegmentDuration)*0.8 {
		shouldEmit = true
	}
	if elapsed >= float64(ub.config.TargetSegmentDuration)*2 {
		shouldEmit = true
	}

	if shouldEmit && ub.accumBytes > 0 {
		ub.emitSegment(chunkSeq, elapsed, ub.accumKeyframeFound)
	}
}

// emitSegment creates a segment marker for the accumulated chunks.
func (ub *UnifiedBuffer) emitSegment(endChunk uint64, duration float64, isKeyframe bool) {
	seg := SegmentMarker{
		Sequence:      ub.segmentSequence.Add(1),
		StartChunk:    ub.accumStartChunk,
		EndChunk:      endChunk,
		Duration:      duration,
		Timestamp:     time.Now(),
		IsKeyframe:    isKeyframe,
		ByteSize:      ub.accumBytes,
		Discontinuity: ub.nextDiscontinuity,
	}

	ub.segmentMu.Lock()
	ub.segments = append(ub.segments, seg)
	segmentCount := len(ub.segments)

	// Enforce segment limit
	evictedCount := 0
	for len(ub.segments) > ub.config.MaxSegments {
		ub.segments = ub.segments[1:]
		evictedCount++
	}
	ub.segmentMu.Unlock()

	// Log segment emission
	ub.logger.Debug("segment emitted",
		slog.Uint64("sequence", seg.Sequence),
		slog.Float64("duration", seg.Duration),
		slog.Int("bytes", seg.ByteSize),
		slog.Bool("keyframe", seg.IsKeyframe),
		slog.Bool("discontinuity", seg.Discontinuity),
		slog.Int("segment_count", segmentCount),
		slog.Int("evicted", evictedCount),
	)

	// Reset accumulation for next segment
	ub.accumStartChunk = endChunk + 1
	ub.accumBytes = 0
	ub.accumStartTime = time.Now()
	ub.accumKeyframeFound = false
	ub.nextDiscontinuity = false // Reset discontinuity flag after use
}

// detectKeyframeInChunk attempts to detect keyframes in MPEG-TS data.
func detectKeyframeInChunk(data []byte) bool {
	// Look for MPEG-TS sync byte and random access indicator
	for i := 0; i+188 <= len(data); i += 188 {
		if data[i] == 0x47 { // TS sync byte
			if len(data) > i+5 {
				// Check adaptation field for random access indicator
				hasAdaptation := (data[i+3] & 0x20) != 0
				if hasAdaptation && len(data) > i+5 {
					adaptFlags := data[i+5]
					if (adaptFlags & 0x40) != 0 { // Random access indicator
						return true
					}
				}
			}
		}
	}
	return false
}

// enforceChunkLimits removes old chunks (must hold chunkMu write lock).
func (ub *UnifiedBuffer) enforceChunkLimits() {
	// Remove by count
	for len(ub.chunks) > ub.config.MaxChunks {
		ub.chunks = ub.chunks[1:]
	}

	// Remove by size
	var totalSize int64
	for _, c := range ub.chunks {
		totalSize += int64(len(c.Data))
	}
	for totalSize > ub.config.MaxBufferSize && len(ub.chunks) > 0 {
		totalSize -= int64(len(ub.chunks[0].Data))
		ub.chunks = ub.chunks[1:]
	}
}

// ReadChunksFrom reads chunks starting from a sequence number (for MPEG-TS streaming).
func (ub *UnifiedBuffer) ReadChunksFrom(fromSeq uint64) []Chunk {
	ub.chunkMu.RLock()
	defer ub.chunkMu.RUnlock()

	var result []Chunk
	for _, chunk := range ub.chunks {
		if chunk.Sequence > fromSeq {
			result = append(result, chunk)
		}
	}
	return result
}

// ReadChunksForClient reads available chunks for a client (continuous streaming).
func (ub *UnifiedBuffer) ReadChunksForClient(client *UnifiedClient) []Chunk {
	chunks := ub.ReadChunksFrom(client.GetLastChunkSequence())
	if len(chunks) > 0 {
		client.SetLastChunkSequence(chunks[len(chunks)-1].Sequence)
		var bytes uint64
		for _, c := range chunks {
			bytes += uint64(len(c.Data))
		}
		client.AddBytesRead(bytes)
		client.UpdateLastRead()
	}
	return chunks
}

// ReadChunksWithWait reads chunks or waits for new data.
func (ub *UnifiedBuffer) ReadChunksWithWait(ctx context.Context, client *UnifiedClient) ([]Chunk, error) {
	for {
		chunks := ub.ReadChunksForClient(client)
		if len(chunks) > 0 {
			return chunks, nil
		}

		if err := client.Wait(ctx); err != nil {
			return nil, err
		}

		ub.chunkMu.RLock()
		closed := ub.closed
		ub.chunkMu.RUnlock()
		if closed {
			return nil, ErrBufferClosed
		}
	}
}

// GetSegment returns a segment by sequence number (for HLS/DASH).
func (ub *UnifiedBuffer) GetSegment(segmentSeq uint64) (*Segment, error) {
	ub.segmentMu.RLock()
	var marker *SegmentMarker
	for i := range ub.segments {
		if ub.segments[i].Sequence == segmentSeq {
			marker = &ub.segments[i]
			break
		}
	}
	ub.segmentMu.RUnlock()

	if marker == nil {
		return nil, ErrSegmentNotFound
	}

	// Collect chunk data for this segment
	ub.chunkMu.RLock()
	defer ub.chunkMu.RUnlock()

	var data []byte
	for _, chunk := range ub.chunks {
		if chunk.Sequence >= marker.StartChunk && chunk.Sequence <= marker.EndChunk {
			data = append(data, chunk.Data...)
		}
	}

	if len(data) == 0 {
		return nil, ErrSegmentNotFound // Chunks may have been evicted
	}

	return &Segment{
		Sequence:   marker.Sequence,
		Duration:   marker.Duration,
		Data:       data,
		Timestamp:  marker.Timestamp,
		IsKeyframe: marker.IsKeyframe,
	}, nil
}

// GetSegments returns all available segment markers (for playlist generation).
func (ub *UnifiedBuffer) GetSegments() []SegmentMarker {
	ub.segmentMu.RLock()
	defer ub.segmentMu.RUnlock()

	result := make([]SegmentMarker, len(ub.segments))
	copy(result, ub.segments)
	return result
}

// GetSegmentInfos returns segment metadata for playlist generation.
// Implements the SegmentProvider interface.
func (ub *UnifiedBuffer) GetSegmentInfos() []SegmentInfo {
	ub.segmentMu.RLock()
	defer ub.segmentMu.RUnlock()

	if len(ub.segments) == 0 {
		return nil
	}

	// Check if we're in fMP4 mode
	isFMP4 := ub.IsFMP4Mode()

	result := make([]SegmentInfo, len(ub.segments))
	for i, marker := range ub.segments {
		result[i] = SegmentInfo{
			Sequence:      marker.Sequence,
			Duration:      marker.Duration,
			IsKeyframe:    marker.IsKeyframe,
			Timestamp:     marker.Timestamp,
			Discontinuity: marker.Discontinuity,
			IsFMP4:        isFMP4,
		}
	}
	return result
}

// GetSegmentData returns the actual data for a segment.
func (ub *UnifiedBuffer) GetSegmentData(marker SegmentMarker) ([]byte, error) {
	ub.chunkMu.RLock()
	defer ub.chunkMu.RUnlock()

	var data []byte
	for _, chunk := range ub.chunks {
		if chunk.Sequence >= marker.StartChunk && chunk.Sequence <= marker.EndChunk {
			data = append(data, chunk.Data...)
		}
	}

	if len(data) == 0 {
		return nil, ErrSegmentNotFound
	}
	return data, nil
}

// FirstSegmentSequence returns the first available segment sequence.
func (ub *UnifiedBuffer) FirstSegmentSequence() (uint64, bool) {
	ub.segmentMu.RLock()
	defer ub.segmentMu.RUnlock()

	if len(ub.segments) == 0 {
		return 0, false
	}
	return ub.segments[0].Sequence, true
}

// LastSegmentSequence returns the last available segment sequence.
func (ub *UnifiedBuffer) LastSegmentSequence() (uint64, bool) {
	ub.segmentMu.RLock()
	defer ub.segmentMu.RUnlock()

	if len(ub.segments) == 0 {
		return 0, false
	}
	return ub.segments[len(ub.segments)-1].Sequence, true
}

// TargetDuration returns the target segment duration.
func (ub *UnifiedBuffer) TargetDuration() int {
	return ub.config.TargetSegmentDuration
}

// Client management

// AddClient adds a client to the buffer.
func (ub *UnifiedBuffer) AddClient(userAgent, remoteAddr string) (*UnifiedClient, error) {
	ub.chunkMu.RLock()
	if ub.closed {
		ub.chunkMu.RUnlock()
		return nil, ErrBufferClosed
	}
	currentSeq := ub.chunkSequence.Load()
	ub.chunkMu.RUnlock()

	client := NewUnifiedClient(userAgent, remoteAddr)
	client.SetLastChunkSequence(currentSeq)

	ub.clientsMu.Lock()
	ub.clients[client.ID] = client
	ub.clientsMu.Unlock()

	return client, nil
}

// RemoveClient removes a client.
func (ub *UnifiedBuffer) RemoveClient(clientID uuid.UUID) bool {
	ub.clientsMu.Lock()
	defer ub.clientsMu.Unlock()

	if _, ok := ub.clients[clientID]; ok {
		delete(ub.clients, clientID)
		return true
	}
	return false
}

// ClientCount returns the number of connected clients.
func (ub *UnifiedBuffer) ClientCount() int {
	ub.clientsMu.RLock()
	defer ub.clientsMu.RUnlock()
	return len(ub.clients)
}

// notifyClients signals all clients that new data is available.
func (ub *UnifiedBuffer) notifyClients() {
	ub.clientsMu.RLock()
	defer ub.clientsMu.RUnlock()

	for _, client := range ub.clients {
		client.Notify()
	}
}

// Cleanup

func (ub *UnifiedBuffer) cleanupLoop() {
	defer ub.wg.Done()

	ticker := time.NewTicker(ub.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ub.stopCh:
			return
		case <-ticker.C:
			ub.cleanupOldChunks()
			ub.cleanupOrphanedSegments()
		}
	}
}

func (ub *UnifiedBuffer) cleanupOldChunks() {
	ub.chunkMu.Lock()
	defer ub.chunkMu.Unlock()

	now := time.Now()
	for len(ub.chunks) > 0 {
		if now.Sub(ub.chunks[0].Timestamp) > ub.config.ChunkTimeout {
			ub.chunks = ub.chunks[1:]
		} else {
			break
		}
	}
}

func (ub *UnifiedBuffer) cleanupOrphanedSegments() {
	ub.chunkMu.RLock()
	var minChunkSeq uint64
	if len(ub.chunks) > 0 {
		minChunkSeq = ub.chunks[0].Sequence
	}
	ub.chunkMu.RUnlock()

	if minChunkSeq == 0 {
		return
	}

	ub.segmentMu.Lock()
	defer ub.segmentMu.Unlock()

	// Remove segments whose chunks have been evicted
	orphanedCount := 0
	for len(ub.segments) > 0 {
		if ub.segments[0].StartChunk < minChunkSeq {
			ub.segments = ub.segments[1:]
			orphanedCount++
		} else {
			break
		}
	}

	if orphanedCount > 0 {
		ub.logger.Debug("cleaned up orphaned segments",
			slog.Int("orphaned_count", orphanedCount),
			slog.Uint64("min_chunk_seq", minChunkSeq),
			slog.Int("remaining_segments", len(ub.segments)),
		)
	}
}

// Close closes the buffer.
func (ub *UnifiedBuffer) Close() {
	ub.chunkMu.Lock()
	if ub.closed {
		ub.chunkMu.Unlock()
		return
	}
	ub.closed = true
	ub.chunkMu.Unlock()

	close(ub.stopCh)

	// Wake all waiting clients
	ub.clientsMu.RLock()
	for _, client := range ub.clients {
		client.Notify()
	}
	ub.clientsMu.RUnlock()

	ub.wg.Wait()
}

// IsClosed returns true if the buffer is closed.
func (ub *UnifiedBuffer) IsClosed() bool {
	ub.chunkMu.RLock()
	defer ub.chunkMu.RUnlock()
	return ub.closed
}

// MarkDiscontinuity marks the next segment as a discontinuity.
// This should be called when the stream restarts, source changes,
// or encoding parameters change.
func (ub *UnifiedBuffer) MarkDiscontinuity() {
	ub.accumMu.Lock()
	ub.nextDiscontinuity = true
	ub.accumMu.Unlock()

	ub.logger.Info("discontinuity marked for next segment")
}

// Stats returns buffer statistics.
func (ub *UnifiedBuffer) Stats() UnifiedBufferStats {
	ub.chunkMu.RLock()
	chunkCount := len(ub.chunks)
	var bufferSize int64
	for _, c := range ub.chunks {
		bufferSize += int64(len(c.Data))
	}
	ub.chunkMu.RUnlock()

	ub.segmentMu.RLock()
	segmentCount := len(ub.segments)
	var firstSeg, lastSeg uint64
	var totalSegmentBytes int64
	if segmentCount > 0 {
		firstSeg = ub.segments[0].Sequence
		lastSeg = ub.segments[segmentCount-1].Sequence
		for _, seg := range ub.segments {
			totalSegmentBytes += int64(seg.ByteSize)
		}
	}
	ub.segmentMu.RUnlock()

	ub.clientsMu.RLock()
	clientCount := len(ub.clients)
	ub.clientsMu.RUnlock()

	// Calculate memory metrics
	maxBufferSize := ub.config.MaxBufferSize
	var bufferUtilization float64
	if maxBufferSize > 0 {
		bufferUtilization = float64(bufferSize) / float64(maxBufferSize) * 100
	}

	var avgChunkSize int64
	if chunkCount > 0 {
		avgChunkSize = bufferSize / int64(chunkCount)
	}

	var avgSegmentSize int64
	if segmentCount > 0 {
		avgSegmentSize = totalSegmentBytes / int64(segmentCount)
	}

	return UnifiedBufferStats{
		ChunkCount:         chunkCount,
		SegmentCount:       segmentCount,
		BufferSize:         bufferSize,
		TotalBytesWritten:  ub.totalBytesWritten.Load(),
		FirstSegment:       firstSeg,
		LastSegment:        lastSeg,
		ClientCount:        clientCount,
		MaxBufferSize:      maxBufferSize,
		BufferUtilization:  bufferUtilization,
		AverageChunkSize:   avgChunkSize,
		AverageSegmentSize: avgSegmentSize,
		TotalChunksWritten: ub.totalChunksWritten.Load(),
	}
}

// UnifiedBufferStats holds buffer statistics.
type UnifiedBufferStats struct {
	ChunkCount        int    `json:"chunk_count"`
	SegmentCount      int    `json:"segment_count"`
	BufferSize        int64  `json:"buffer_size"`
	TotalBytesWritten uint64 `json:"total_bytes_written"`
	FirstSegment      uint64 `json:"first_segment"`
	LastSegment       uint64 `json:"last_segment"`
	ClientCount       int    `json:"client_count"`

	// Memory usage metrics
	MaxBufferSize      int64   `json:"max_buffer_size"`       // Configured maximum buffer size
	BufferUtilization  float64 `json:"buffer_utilization"`    // Percentage of max buffer used (0-100)
	AverageChunkSize   int64   `json:"average_chunk_size"`    // Average bytes per chunk
	AverageSegmentSize int64   `json:"average_segment_size"`  // Average bytes per segment
	TotalChunksWritten uint64  `json:"total_chunks_written"`  // Total chunks written since creation
}

// UnifiedClient represents a client connected to the unified buffer.
type UnifiedClient struct {
	ID          uuid.UUID
	UserAgent   string
	RemoteAddr  string
	ConnectedAt time.Time

	mu               sync.RWMutex
	lastChunkSeq     uint64
	lastRead         time.Time
	bytesRead        uint64
	notifyCh         chan struct{}
}

// NewUnifiedClient creates a new unified client.
func NewUnifiedClient(userAgent, remoteAddr string) *UnifiedClient {
	return &UnifiedClient{
		ID:          uuid.New(),
		UserAgent:   userAgent,
		RemoteAddr:  remoteAddr,
		ConnectedAt: time.Now(),
		lastRead:    time.Now(),
		notifyCh:    make(chan struct{}, 1),
	}
}

// GetLastChunkSequence returns the last chunk sequence read.
func (c *UnifiedClient) GetLastChunkSequence() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastChunkSeq
}

// SetLastChunkSequence sets the last chunk sequence.
func (c *UnifiedClient) SetLastChunkSequence(seq uint64) {
	c.mu.Lock()
	c.lastChunkSeq = seq
	c.mu.Unlock()
}

// AddBytesRead adds to bytes read counter.
func (c *UnifiedClient) AddBytesRead(n uint64) {
	c.mu.Lock()
	c.bytesRead += n
	c.mu.Unlock()
}

// GetBytesRead returns total bytes read.
func (c *UnifiedClient) GetBytesRead() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bytesRead
}

// UpdateLastRead updates the last read time.
func (c *UnifiedClient) UpdateLastRead() {
	c.mu.Lock()
	c.lastRead = time.Now()
	c.mu.Unlock()
}

// Notify signals the client that data is available.
func (c *UnifiedClient) Notify() {
	select {
	case c.notifyCh <- struct{}{}:
	default:
	}
}

// Wait waits for notification or context cancellation.
func (c *UnifiedClient) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.notifyCh:
		return nil
	}
}

// IsStale returns true if the client hasn't read recently.
func (c *UnifiedClient) IsStale(timeout time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastRead) > timeout
}

// fMP4/CMAF support methods for UnifiedBuffer

// IsFMP4Mode returns true if the buffer is in fMP4/CMAF mode.
func (ub *UnifiedBuffer) IsFMP4Mode() bool {
	return ub.cmafMuxer != nil
}

// GetInitSegment returns the initialization segment for fMP4 streams.
// Returns nil if the init segment is not yet available or buffer is in MPEG-TS mode.
func (ub *UnifiedBuffer) GetInitSegment() *InitSegment {
	if ub.initSegment == nil {
		return nil
	}
	return ub.initSegment.Clone()
}

// HasInitSegment returns true if the initialization segment is available.
func (ub *UnifiedBuffer) HasInitSegment() bool {
	return ub.initSegment != nil && len(ub.initSegment.Data) > 0
}

// GetFMP4Segment returns a segment for fMP4 streams including container info.
func (ub *UnifiedBuffer) GetFMP4Segment(segmentSeq uint64) (*Segment, error) {
	seg, err := ub.GetSegment(segmentSeq)
	if err != nil {
		return nil, err
	}

	// Add fMP4-specific fields
	if ub.IsFMP4Mode() {
		seg.IsFragmented = true
		seg.ContainerFormat = "fmp4"
	}

	return seg, nil
}

// ContainerFormat returns the container format of the buffer.
func (ub *UnifiedBuffer) ContainerFormat() string {
	return ub.config.ContainerFormat
}
