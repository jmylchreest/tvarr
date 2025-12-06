package relay

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

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
