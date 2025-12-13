// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// PassthroughConnection represents an active passthrough/direct proxy connection.
type PassthroughConnection struct {
	ID           string    `json:"id"`
	ChannelID    uuid.UUID `json:"channelId"`
	ChannelName  string    `json:"channelName"`
	StreamURL    string    `json:"streamUrl"`
	VideoCodec   string    `json:"videoCodec"`
	AudioCodec   string    `json:"audioCodec"`
	SourceFormat string    `json:"sourceFormat"` // mpegts, hls, dash
	RemoteAddr   string    `json:"remoteAddr"`
	UserAgent    string    `json:"userAgent"`
	StartedAt    time.Time `json:"startedAt"`

	// Atomic counters for thread-safe updates
	bytesIn  atomic.Uint64
	bytesOut atomic.Uint64

	// Bandwidth tracking
	lastBytesIn    uint64
	lastBytesOut   uint64
	lastUpdateTime time.Time
	ingressBps     atomic.Uint64
	egressBps      atomic.Uint64

	// History for sparklines (protected by historyMu)
	historyMu      sync.RWMutex
	ingressHistory []uint64
	egressHistory  []uint64
}

// NewPassthroughConnection creates a new passthrough connection tracker.
func NewPassthroughConnection(
	channelID uuid.UUID,
	channelName string,
	streamURL string,
	videoCodec string,
	audioCodec string,
	sourceFormat string,
	remoteAddr string,
	userAgent string,
) *PassthroughConnection {
	return &PassthroughConnection{
		ID:             uuid.New().String(),
		ChannelID:      channelID,
		ChannelName:    channelName,
		StreamURL:      streamURL,
		VideoCodec:     videoCodec,
		AudioCodec:     audioCodec,
		SourceFormat:   sourceFormat,
		RemoteAddr:     remoteAddr,
		UserAgent:      userAgent,
		StartedAt:      time.Now(),
		lastUpdateTime: time.Now(),
		ingressHistory: make([]uint64, 0, 30),
		egressHistory:  make([]uint64, 0, 30),
	}
}

// AddBytesIn records bytes received from upstream.
func (c *PassthroughConnection) AddBytesIn(n uint64) {
	c.bytesIn.Add(n)
}

// AddBytesOut records bytes sent to client.
func (c *PassthroughConnection) AddBytesOut(n uint64) {
	c.bytesOut.Add(n)
}

// BytesIn returns total bytes received from upstream.
func (c *PassthroughConnection) BytesIn() uint64 {
	return c.bytesIn.Load()
}

// BytesOut returns total bytes sent to client.
func (c *PassthroughConnection) BytesOut() uint64 {
	return c.bytesOut.Load()
}

// IngressBps returns current ingress bandwidth in bytes per second.
func (c *PassthroughConnection) IngressBps() uint64 {
	return c.ingressBps.Load()
}

// EgressBps returns current egress bandwidth in bytes per second.
func (c *PassthroughConnection) EgressBps() uint64 {
	return c.egressBps.Load()
}

// DurationSecs returns how long the connection has been active.
func (c *PassthroughConnection) DurationSecs() float64 {
	return time.Since(c.StartedAt).Seconds()
}

// UpdateBandwidth calculates and stores current bandwidth rates.
// Should be called periodically (e.g., every second).
func (c *PassthroughConnection) UpdateBandwidth() {
	now := time.Now()
	elapsed := now.Sub(c.lastUpdateTime).Seconds()
	if elapsed < 0.1 {
		return // Too soon
	}

	currentBytesIn := c.bytesIn.Load()
	currentBytesOut := c.bytesOut.Load()

	// Calculate rates
	ingressBps := uint64(float64(currentBytesIn-c.lastBytesIn) / elapsed)
	egressBps := uint64(float64(currentBytesOut-c.lastBytesOut) / elapsed)

	c.ingressBps.Store(ingressBps)
	c.egressBps.Store(egressBps)

	// Update history
	c.historyMu.Lock()
	c.ingressHistory = append(c.ingressHistory, ingressBps)
	c.egressHistory = append(c.egressHistory, egressBps)
	// Keep last 30 samples
	if len(c.ingressHistory) > 30 {
		c.ingressHistory = c.ingressHistory[1:]
	}
	if len(c.egressHistory) > 30 {
		c.egressHistory = c.egressHistory[1:]
	}
	c.historyMu.Unlock()

	c.lastBytesIn = currentBytesIn
	c.lastBytesOut = currentBytesOut
	c.lastUpdateTime = now
}

// IngressHistory returns a copy of the ingress bandwidth history.
func (c *PassthroughConnection) IngressHistory() []uint64 {
	c.historyMu.RLock()
	defer c.historyMu.RUnlock()
	result := make([]uint64, len(c.ingressHistory))
	copy(result, c.ingressHistory)
	return result
}

// EgressHistory returns a copy of the egress bandwidth history.
func (c *PassthroughConnection) EgressHistory() []uint64 {
	c.historyMu.RLock()
	defer c.historyMu.RUnlock()
	result := make([]uint64, len(c.egressHistory))
	copy(result, c.egressHistory)
	return result
}

// PassthroughTracker manages active passthrough connections.
type PassthroughTracker struct {
	mu          sync.RWMutex
	connections map[string]*PassthroughConnection
}

// NewPassthroughTracker creates a new passthrough tracker.
func NewPassthroughTracker() *PassthroughTracker {
	return &PassthroughTracker{
		connections: make(map[string]*PassthroughConnection),
	}
}

// Register adds a new passthrough connection and returns it.
func (t *PassthroughTracker) Register(conn *PassthroughConnection) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connections[conn.ID] = conn
}

// Unregister removes a passthrough connection.
func (t *PassthroughTracker) Unregister(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.connections, id)
}

// Get returns a passthrough connection by ID.
func (t *PassthroughTracker) Get(id string) *PassthroughConnection {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connections[id]
}

// List returns all active passthrough connections.
func (t *PassthroughTracker) List() []*PassthroughConnection {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]*PassthroughConnection, 0, len(t.connections))
	for _, conn := range t.connections {
		result = append(result, conn)
	}
	return result
}

// Count returns the number of active passthrough connections.
func (t *PassthroughTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.connections)
}

// UpdateAllBandwidth updates bandwidth calculations for all connections.
func (t *PassthroughTracker) UpdateAllBandwidth() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, conn := range t.connections {
		conn.UpdateBandwidth()
	}
}

// TotalIngressBps returns the combined ingress bandwidth of all connections.
func (t *PassthroughTracker) TotalIngressBps() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var total uint64
	for _, conn := range t.connections {
		total += conn.IngressBps()
	}
	return total
}

// TotalEgressBps returns the combined egress bandwidth of all connections.
func (t *PassthroughTracker) TotalEgressBps() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var total uint64
	for _, conn := range t.connections {
		total += conn.EgressBps()
	}
	return total
}
