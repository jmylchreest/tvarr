package relay

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ErrBufferClosed is returned when the buffer is closed.
var ErrBufferClosed = errors.New("cyclic buffer closed")

// ErrClientNotFound is returned when a client is not found.
var ErrClientNotFound = errors.New("client not found")

// CyclicBufferConfig configures the cyclic buffer.
type CyclicBufferConfig struct {
	// MaxBufferSize is the maximum buffer size in bytes.
	MaxBufferSize int
	// MaxChunks is the maximum number of chunks to keep.
	MaxChunks int
	// ChunkTimeout is how long to keep chunks.
	ChunkTimeout time.Duration
	// ClientTimeout is how long to wait for slow clients.
	ClientTimeout time.Duration
	// CleanupInterval is how often to cleanup old chunks.
	CleanupInterval time.Duration
}

// DefaultCyclicBufferConfig returns sensible defaults.
func DefaultCyclicBufferConfig() CyclicBufferConfig {
	return CyclicBufferConfig{
		MaxBufferSize:   50 * 1024 * 1024, // 50MB
		MaxChunks:       1000,
		ChunkTimeout:    60 * time.Second,
		ClientTimeout:   30 * time.Second,
		CleanupInterval: 5 * time.Second,
	}
}

// BufferChunk represents a chunk of data in the buffer.
type BufferChunk struct {
	Sequence  uint64
	Data      []byte
	Timestamp time.Time
}

// BufferClient represents a client reading from the buffer.
type BufferClient struct {
	ID           uuid.UUID
	lastSequence atomic.Uint64
	lastRead     time.Time
	lastReadMu   sync.RWMutex
	bytesRead    atomic.Uint64
	ConnectedAt  time.Time
	UserAgent    string
	RemoteAddr   string
	waitCh       chan struct{}
}

// NewBufferClient creates a new buffer client.
func NewBufferClient(userAgent, remoteAddr string) *BufferClient {
	return &BufferClient{
		ID:          uuid.New(),
		lastRead:    time.Now(),
		ConnectedAt: time.Now(),
		UserAgent:   userAgent,
		RemoteAddr:  remoteAddr,
		waitCh:      make(chan struct{}, 1),
	}
}

// GetLastSequence returns the last read sequence number.
func (c *BufferClient) GetLastSequence() uint64 {
	return c.lastSequence.Load()
}

// SetLastSequence sets the last read sequence number.
func (c *BufferClient) SetLastSequence(seq uint64) {
	c.lastSequence.Store(seq)
}

// GetBytesRead returns total bytes read by this client.
func (c *BufferClient) GetBytesRead() uint64 {
	return c.bytesRead.Load()
}

// AddBytesRead adds to the bytes read counter.
func (c *BufferClient) AddBytesRead(bytes uint64) {
	c.bytesRead.Add(bytes)
}

// UpdateLastRead updates the last read timestamp.
func (c *BufferClient) UpdateLastRead() {
	c.lastReadMu.Lock()
	c.lastRead = time.Now()
	c.lastReadMu.Unlock()
}

// IsStale returns true if the client hasn't read recently.
func (c *BufferClient) IsStale(timeout time.Duration) bool {
	c.lastReadMu.RLock()
	defer c.lastReadMu.RUnlock()
	return time.Since(c.lastRead) > timeout
}

// GetLastReadTime returns the last read time.
func (c *BufferClient) GetLastReadTime() time.Time {
	c.lastReadMu.RLock()
	defer c.lastReadMu.RUnlock()
	return c.lastRead
}

// Notify signals the client that new data is available.
func (c *BufferClient) Notify() {
	select {
	case c.waitCh <- struct{}{}:
	default:
		// Channel already has notification pending
	}
}

// Wait waits for new data or context cancellation.
func (c *BufferClient) Wait(ctx context.Context) error {
	select {
	case <-c.waitCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CyclicBuffer implements a circular buffer for multi-client streaming.
type CyclicBuffer struct {
	config CyclicBufferConfig

	mu       sync.RWMutex
	chunks   []BufferChunk
	sequence atomic.Uint64
	closed   bool

	clientsMu sync.RWMutex
	clients   map[uuid.UUID]*BufferClient

	totalBytes     atomic.Uint64
	upstreamBytes  atomic.Uint64
	currentSize    atomic.Int64

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewCyclicBuffer creates a new cyclic buffer.
func NewCyclicBuffer(config CyclicBufferConfig) *CyclicBuffer {
	cb := &CyclicBuffer{
		config:  config,
		chunks:  make([]BufferChunk, 0, config.MaxChunks),
		clients: make(map[uuid.UUID]*BufferClient),
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	cb.wg.Add(1)
	go cb.cleanupLoop()

	return cb
}

// AddClient adds a new client to the buffer.
func (cb *CyclicBuffer) AddClient(userAgent, remoteAddr string) (*BufferClient, error) {
	cb.mu.RLock()
	if cb.closed {
		cb.mu.RUnlock()
		return nil, ErrBufferClosed
	}
	currentSeq := cb.sequence.Load()
	cb.mu.RUnlock()

	client := NewBufferClient(userAgent, remoteAddr)
	// Set starting sequence to current, so they get new chunks only
	client.SetLastSequence(currentSeq)

	cb.clientsMu.Lock()
	cb.clients[client.ID] = client
	cb.clientsMu.Unlock()

	return client, nil
}

// RemoveClient removes a client from the buffer.
func (cb *CyclicBuffer) RemoveClient(clientID uuid.UUID) bool {
	cb.clientsMu.Lock()
	defer cb.clientsMu.Unlock()

	if _, ok := cb.clients[clientID]; ok {
		delete(cb.clients, clientID)
		return true
	}
	return false
}

// GetClient returns a client by ID.
func (cb *CyclicBuffer) GetClient(clientID uuid.UUID) (*BufferClient, bool) {
	cb.clientsMu.RLock()
	defer cb.clientsMu.RUnlock()
	client, ok := cb.clients[clientID]
	return client, ok
}

// ClientCount returns the number of connected clients.
func (cb *CyclicBuffer) ClientCount() int {
	cb.clientsMu.RLock()
	defer cb.clientsMu.RUnlock()
	return len(cb.clients)
}

// WriteChunk writes a chunk of data to the buffer.
func (cb *CyclicBuffer) WriteChunk(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	cb.mu.Lock()
	if cb.closed {
		cb.mu.Unlock()
		return ErrBufferClosed
	}

	// Track upstream bytes
	cb.upstreamBytes.Add(uint64(len(data)))

	// Create new chunk
	seq := cb.sequence.Add(1)
	chunk := BufferChunk{
		Sequence:  seq,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Add to buffer
	cb.chunks = append(cb.chunks, chunk)
	cb.currentSize.Add(int64(len(data)))

	// Enforce limits
	cb.enforceLimits()

	cb.totalBytes.Add(uint64(len(data)))
	cb.mu.Unlock()

	// Notify all clients
	cb.notifyClients()

	return nil
}

// enforceLimits removes old chunks to stay within limits (must hold write lock).
func (cb *CyclicBuffer) enforceLimits() {
	// Remove excess chunks
	for len(cb.chunks) > cb.config.MaxChunks {
		removed := cb.chunks[0]
		cb.chunks = cb.chunks[1:]
		cb.currentSize.Add(-int64(len(removed.Data)))
	}

	// Remove chunks that exceed size limit
	for cb.currentSize.Load() > int64(cb.config.MaxBufferSize) && len(cb.chunks) > 0 {
		removed := cb.chunks[0]
		cb.chunks = cb.chunks[1:]
		cb.currentSize.Add(-int64(len(removed.Data)))
	}
}

// notifyClients signals all clients that new data is available.
func (cb *CyclicBuffer) notifyClients() {
	cb.clientsMu.RLock()
	defer cb.clientsMu.RUnlock()

	for _, client := range cb.clients {
		client.Notify()
	}
}

// ReadChunksForClient reads available chunks for a specific client.
func (cb *CyclicBuffer) ReadChunksForClient(client *BufferClient) []BufferChunk {
	lastSeq := client.GetLastSequence()

	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var result []BufferChunk
	for _, chunk := range cb.chunks {
		if chunk.Sequence > lastSeq {
			result = append(result, chunk)
			client.SetLastSequence(chunk.Sequence)
			client.AddBytesRead(uint64(len(chunk.Data)))
		}
	}

	if len(result) > 0 {
		client.UpdateLastRead()
	}

	return result
}

// ReadWithWait reads chunks or waits for new data.
func (cb *CyclicBuffer) ReadWithWait(ctx context.Context, client *BufferClient) ([]BufferChunk, error) {
	for {
		chunks := cb.ReadChunksForClient(client)
		if len(chunks) > 0 {
			return chunks, nil
		}

		// Wait for new data
		if err := client.Wait(ctx); err != nil {
			return nil, err
		}

		// Check if buffer is closed
		cb.mu.RLock()
		closed := cb.closed
		cb.mu.RUnlock()
		if closed {
			return nil, ErrBufferClosed
		}
	}
}

// cleanupLoop periodically cleans up old chunks and stale clients.
func (cb *CyclicBuffer) cleanupLoop() {
	defer cb.wg.Done()

	ticker := time.NewTicker(cb.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cb.stopCh:
			return
		case <-ticker.C:
			cb.cleanupOldChunks()
			cb.cleanupStaleClients()
		}
	}
}

// cleanupOldChunks removes chunks that are too old.
func (cb *CyclicBuffer) cleanupOldChunks() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	removed := 0

	for len(cb.chunks) > 0 {
		if now.Sub(cb.chunks[0].Timestamp) > cb.config.ChunkTimeout {
			old := cb.chunks[0]
			cb.chunks = cb.chunks[1:]
			cb.currentSize.Add(-int64(len(old.Data)))
			removed++
		} else {
			break
		}
	}

	if removed > 0 {
		// Log cleanup (could add logger)
	}
}

// cleanupStaleClients removes clients that haven't read recently.
func (cb *CyclicBuffer) cleanupStaleClients() {
	cb.clientsMu.Lock()
	defer cb.clientsMu.Unlock()

	for id, client := range cb.clients {
		if client.IsStale(cb.config.ClientTimeout) {
			delete(cb.clients, id)
		}
	}
}

// Close closes the buffer and releases resources.
func (cb *CyclicBuffer) Close() {
	cb.mu.Lock()
	if cb.closed {
		cb.mu.Unlock()
		return
	}
	cb.closed = true
	cb.mu.Unlock()

	// Signal cleanup to stop
	close(cb.stopCh)

	// Wake up all waiting clients
	cb.clientsMu.RLock()
	for _, client := range cb.clients {
		client.Notify()
	}
	cb.clientsMu.RUnlock()

	// Wait for cleanup goroutine
	cb.wg.Wait()
}

// IsClosed returns true if the buffer is closed.
func (cb *CyclicBuffer) IsClosed() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.closed
}

// Stats returns buffer statistics.
func (cb *CyclicBuffer) Stats() CyclicBufferStats {
	cb.mu.RLock()
	chunkCount := len(cb.chunks)
	cb.mu.RUnlock()

	cb.clientsMu.RLock()
	clientCount := len(cb.clients)
	clients := make([]ClientStats, 0, clientCount)
	for _, c := range cb.clients {
		clients = append(clients, ClientStats{
			ID:           c.ID.String(),
			BytesRead:    c.GetBytesRead(),
			LastSequence: c.GetLastSequence(),
			ConnectedAt:  c.ConnectedAt,
			LastRead:     c.GetLastReadTime(),
			UserAgent:    c.UserAgent,
			RemoteAddr:   c.RemoteAddr,
		})
	}
	cb.clientsMu.RUnlock()

	return CyclicBufferStats{
		TotalChunks:     chunkCount,
		TotalBufferSize: cb.currentSize.Load(),
		TotalBytesWritten:     cb.totalBytes.Load(),
		BytesFromUpstream:     cb.upstreamBytes.Load(),
		CurrentSequence:       cb.sequence.Load(),
		ClientCount:           clientCount,
		Clients:               clients,
	}
}

// CyclicBufferStats holds buffer statistics.
type CyclicBufferStats struct {
	TotalChunks       int           `json:"total_chunks"`
	TotalBufferSize   int64         `json:"total_buffer_size"`
	TotalBytesWritten uint64        `json:"total_bytes_written"`
	BytesFromUpstream uint64        `json:"bytes_from_upstream"`
	CurrentSequence   uint64        `json:"current_sequence"`
	ClientCount       int           `json:"client_count"`
	Clients           []ClientStats `json:"clients,omitempty"`
}

// ClientStats holds statistics for a single client.
type ClientStats struct {
	ID           string    `json:"id"`
	BytesRead    uint64    `json:"bytes_read"`
	LastSequence uint64    `json:"last_sequence"`
	ConnectedAt  time.Time `json:"connected_at"`
	LastRead     time.Time `json:"last_read"`
	UserAgent    string    `json:"user_agent,omitempty"`
	RemoteAddr   string    `json:"remote_addr,omitempty"`
}

// StreamWriter wraps a CyclicBuffer for writing.
type StreamWriter struct {
	buffer *CyclicBuffer
}

// NewStreamWriter creates a writer for a cyclic buffer.
func NewStreamWriter(buffer *CyclicBuffer) *StreamWriter {
	return &StreamWriter{buffer: buffer}
}

// Write implements io.Writer.
func (sw *StreamWriter) Write(p []byte) (int, error) {
	if err := sw.buffer.WriteChunk(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// StreamReader wraps a CyclicBuffer for reading by a specific client.
type StreamReader struct {
	buffer *CyclicBuffer
	client *BufferClient
	pending []byte
}

// NewStreamReader creates a reader for a cyclic buffer.
func NewStreamReader(buffer *CyclicBuffer, client *BufferClient) *StreamReader {
	return &StreamReader{
		buffer: buffer,
		client: client,
	}
}

// Read implements io.Reader with context support.
func (sr *StreamReader) Read(p []byte) (int, error) {
	return sr.ReadContext(context.Background(), p)
}

// ReadContext reads with context support.
func (sr *StreamReader) ReadContext(ctx context.Context, p []byte) (int, error) {
	// Return pending data first
	if len(sr.pending) > 0 {
		n := copy(p, sr.pending)
		sr.pending = sr.pending[n:]
		return n, nil
	}

	// Get new chunks
	chunks, err := sr.buffer.ReadWithWait(ctx, sr.client)
	if err != nil {
		return 0, err
	}

	// Combine chunks into pending
	for _, chunk := range chunks {
		sr.pending = append(sr.pending, chunk.Data...)
	}

	if len(sr.pending) == 0 {
		return 0, nil
	}

	n := copy(p, sr.pending)
	sr.pending = sr.pending[n:]
	return n, nil
}
