// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// OutputFormat represents the output container format.
type OutputFormat string

const (
	OutputFormatHLSTS   OutputFormat = "hls-ts"   // HLS with MPEG-TS segments
	OutputFormatHLSFMP4 OutputFormat = "hls-fmp4" // HLS with fMP4/CMAF segments
	OutputFormatDASH    OutputFormat = "dash"     // MPEG-DASH with fMP4 segments
	OutputFormatMPEGTS  OutputFormat = "mpegts"   // Raw MPEG-TS stream
)

// ProcessorStats contains statistics for a processor.
type ProcessorStats struct {
	ID           string
	Format       OutputFormat
	ClientCount  int
	BytesWritten uint64
	StartedAt    time.Time
	LastActivity time.Time
}

// Processor reads from a SharedESBuffer and produces output for clients.
type Processor interface {
	// ID returns the unique identifier for this processor.
	ID() string

	// Format returns the output format this processor produces.
	Format() OutputFormat

	// Start begins processing data from the shared buffer.
	Start(ctx context.Context) error

	// Stop stops the processor and cleans up resources.
	Stop()

	// RegisterClient adds a client to receive output from this processor.
	RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error

	// UnregisterClient removes a client.
	UnregisterClient(clientID string)

	// ClientCount returns the number of connected clients.
	ClientCount() int

	// Stats returns current processor statistics.
	Stats() ProcessorStats

	// ServeManifest serves the manifest/playlist for this processor.
	ServeManifest(w http.ResponseWriter, r *http.Request) error

	// ServeSegment serves a specific segment by name.
	ServeSegment(w http.ResponseWriter, r *http.Request, segmentName string) error
}

// ProcessorClient represents a connected client.
// Note: The mutex protects mutable fields like writer and LastActivity.
// BytesRead is atomic for lock-free updates.
// IMPORTANT: The mutex should NOT be held during HTTP I/O operations
// as they can block indefinitely.
type ProcessorClient struct {
	ID           string
	UserAgent    string
	RemoteAddr   string
	ConnectedAt  time.Time
	LastActivity time.Time // For request-based protocols like HLS
	BytesRead    atomic.Uint64
	writer       io.Writer
	flusher      http.Flusher
	done         chan struct{}
	mu           sync.Mutex // Protects writer, flusher, LastActivity - NOT for I/O
}

// NewProcessorClient creates a new processor client.
func NewProcessorClient(id string, w http.ResponseWriter, r *http.Request) *ProcessorClient {
	flusher, _ := w.(http.Flusher)
	now := time.Now()
	return &ProcessorClient{
		ID:           id,
		UserAgent:    r.UserAgent(),
		RemoteAddr:   r.RemoteAddr,
		ConnectedAt:  now,
		LastActivity: now,
		writer:       w,
		flusher:      flusher,
		done:         make(chan struct{}),
	}
}

// Write writes data to the client.
// Note: This method does NOT hold locks during I/O to prevent blocking.
// BytesRead is atomic, so no lock is needed for the counter update.
func (c *ProcessorClient) Write(data []byte) (int, error) {
	// Perform I/O without holding any locks - HTTP writes can block indefinitely
	n, err := c.writer.Write(data)
	if n > 0 {
		c.BytesRead.Add(uint64(n))
	}
	if c.flusher != nil {
		c.flusher.Flush()
	}
	return n, err
}

// Close closes the client connection.
func (c *ProcessorClient) Close() {
	close(c.done)
}

// Done returns a channel that is closed when the client is done.
func (c *ProcessorClient) Done() <-chan struct{} {
	return c.done
}

// StreamContext holds metadata for X-Stream headers.
type StreamContext struct {
	ProxyMode        string // "direct" or "smart"
	DeliveryDecision string // "redirect", "proxy", "passthrough", "repackage", "transcode"
	Version          string // tvarr version
}

// BaseProcessor provides common functionality for all processor types.
type BaseProcessor struct {
	id            string
	format        OutputFormat
	clients       map[string]*ProcessorClient
	clientsMu     sync.RWMutex
	startedAt     time.Time
	lastActivity  atomic.Value // time.Time
	bytesWritten  atomic.Uint64
	closed        atomic.Bool
	closedCh      chan struct{}
	streamContext StreamContext

	// Bandwidth tracking for buffer-to-processor edge
	bandwidthTracker *BandwidthTracker
}

// NewBaseProcessor creates a new base processor.
func NewBaseProcessor(id string, format OutputFormat) *BaseProcessor {
	p := &BaseProcessor{
		id:       id,
		format:   format,
		clients:  make(map[string]*ProcessorClient),
		closedCh: make(chan struct{}),
	}
	p.lastActivity.Store(time.Now())
	return p
}

// ID returns the processor ID.
func (p *BaseProcessor) ID() string {
	return p.id
}

// Format returns the output format.
func (p *BaseProcessor) Format() OutputFormat {
	return p.format
}

// SetStreamContext sets the stream context for header generation.
func (p *BaseProcessor) SetStreamContext(ctx StreamContext) {
	p.streamContext = ctx
}

// SetBandwidthTracker sets the bandwidth tracker for buffer-to-processor edge tracking.
func (p *BaseProcessor) SetBandwidthTracker(tracker *BandwidthTracker) {
	p.bandwidthTracker = tracker
}

// TrackBytesFromBuffer records bytes read from the ES buffer.
// This should be called after reading samples from the buffer.
func (p *BaseProcessor) TrackBytesFromBuffer(bytes uint64) {
	if p.bandwidthTracker != nil {
		p.bandwidthTracker.Add(bytes)
	}
}

// SetStreamHeaders sets the X-Stream-* and X-Tvarr-Version headers on the response.
func (p *BaseProcessor) SetStreamHeaders(w http.ResponseWriter) {
	if p.streamContext.ProxyMode != "" {
		w.Header().Set("X-Stream-Mode", p.streamContext.ProxyMode)
	}
	if p.streamContext.DeliveryDecision != "" {
		w.Header().Set("X-Stream-Decision", p.streamContext.DeliveryDecision)
	}
	w.Header().Set("X-Stream-Format", string(p.format))
	if p.streamContext.Version != "" {
		w.Header().Set("X-Tvarr-Version", p.streamContext.Version)
	}
}

// RegisterClientBase adds a client (used by specific processor implementations).
// For request-based protocols like HLS/DASH, this updates the existing client's
// activity timestamp rather than creating a duplicate entry.
func (p *BaseProcessor) RegisterClientBase(clientID string, w http.ResponseWriter, r *http.Request) *ProcessorClient {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	// Check if client already exists (for request-based protocols like HLS)
	if existing, exists := p.clients[clientID]; exists {
		// Update the existing client's activity - keep accumulated BytesRead
		existing.mu.Lock()
		existing.writer = w
		existing.flusher, _ = w.(http.Flusher)
		existing.LastActivity = time.Now()
		existing.mu.Unlock()
		p.lastActivity.Store(time.Now())
		return existing
	}

	// Create new client
	client := NewProcessorClient(clientID, w, r)
	p.clients[clientID] = client

	p.lastActivity.Store(time.Now())
	return client
}

// UnregisterClientBase removes a client.
func (p *BaseProcessor) UnregisterClientBase(clientID string) {
	p.clientsMu.Lock()
	if client, exists := p.clients[clientID]; exists {
		client.Close()
		delete(p.clients, clientID)
	}
	p.clientsMu.Unlock()

	p.lastActivity.Store(time.Now())
}

// CleanupInactiveClients removes clients that haven't had activity within the timeout duration.
// This is used for request-based protocols like HLS/DASH where clients make periodic requests
// but don't maintain a persistent connection.
func (p *BaseProcessor) CleanupInactiveClients(timeout time.Duration) int {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	now := time.Now()
	removed := 0

	for id, client := range p.clients {
		client.mu.Lock()
		lastActivity := client.LastActivity
		client.mu.Unlock()

		if now.Sub(lastActivity) > timeout {
			client.Close()
			delete(p.clients, id)
			removed++
		}
	}

	return removed
}

// ClientCount returns the number of connected clients.
func (p *BaseProcessor) ClientCount() int {
	p.clientsMu.RLock()
	defer p.clientsMu.RUnlock()
	return len(p.clients)
}

// GetClients returns a copy of all clients.
func (p *BaseProcessor) GetClients() []*ProcessorClient {
	p.clientsMu.RLock()
	defer p.clientsMu.RUnlock()

	clients := make([]*ProcessorClient, 0, len(p.clients))
	for _, c := range p.clients {
		clients = append(clients, c)
	}
	return clients
}

// UpdateClientBytes adds bytes to a specific client's counter.
// This is used by processors that bypass ProcessorClient.Write() for performance.
func (p *BaseProcessor) UpdateClientBytes(clientID string, bytes uint64) {
	p.clientsMu.RLock()
	client, exists := p.clients[clientID]
	p.clientsMu.RUnlock()

	if exists && client != nil {
		client.BytesRead.Add(bytes)
	}
}

// Stats returns current processor statistics.
func (p *BaseProcessor) Stats() ProcessorStats {
	lastActivity, _ := p.lastActivity.Load().(time.Time)
	return ProcessorStats{
		ID:           p.id,
		Format:       p.format,
		ClientCount:  p.ClientCount(),
		BytesWritten: p.bytesWritten.Load(),
		StartedAt:    p.startedAt,
		LastActivity: lastActivity,
	}
}

// RecordBytesWritten adds to the bytes written counter.
func (p *BaseProcessor) RecordBytesWritten(n uint64) {
	p.bytesWritten.Add(n)
	p.lastActivity.Store(time.Now())
}

// Close marks the processor as closed.
func (p *BaseProcessor) Close() {
	if p.closed.CompareAndSwap(false, true) {
		close(p.closedCh)

		// Close all clients
		p.clientsMu.Lock()
		for _, client := range p.clients {
			client.Close()
		}
		p.clients = make(map[string]*ProcessorClient)
		p.clientsMu.Unlock()
	}
}

// IsClosed returns true if the processor is closed.
func (p *BaseProcessor) IsClosed() bool {
	return p.closed.Load()
}

// ClosedChan returns a channel that is closed when the processor is closed.
func (p *BaseProcessor) ClosedChan() <-chan struct{} {
	return p.closedCh
}
