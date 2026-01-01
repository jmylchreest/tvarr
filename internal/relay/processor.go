// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
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

	// CleanupInactiveClients removes clients that haven't been active within the timeout.
	// Returns the number of clients removed.
	CleanupInactiveClients(timeout time.Duration) int

	// IsIdle returns true if the processor should be stopped due to inactivity.
	// Each processor type defines its own idle semantics:
	// - HLS-fMP4: no playlist requests for playlist_segments * segment_duration * 2
	// - HLS-TS/DASH/MPEG-TS: no connected clients
	IsIdle() bool

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

// ErrProcessorStopping is returned when trying to register a client on a processor
// that is being stopped. The caller should create or obtain a new processor.
var ErrProcessorStopping = errors.New("processor is stopping")

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
	stopping      atomic.Bool // Set when processor is being stopped - rejects new clients
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
//
// Returns nil and ErrProcessorStopping if the processor is being stopped.
// The caller should obtain or create a new processor instance.
func (p *BaseProcessor) RegisterClientBase(clientID string, w http.ResponseWriter, r *http.Request) (*ProcessorClient, error) {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	// Reject new registrations if processor is being stopped
	// This prevents the TOCTOU race where a client registers between
	// ClientCount() check and Stop() call in StopProcessorIfIdle
	if p.stopping.Load() {
		slog.Debug("Rejecting client registration - processor is stopping",
			slog.String("processor_id", p.id),
			slog.String("client_id", clientID))
		return nil, ErrProcessorStopping
	}

	now := time.Now()

	// Check if client already exists (for request-based protocols like HLS)
	if existing, exists := p.clients[clientID]; exists {
		// Update the existing client's activity - keep accumulated BytesRead
		existing.mu.Lock()
		oldActivity := existing.LastActivity
		existing.writer = w
		existing.flusher, _ = w.(http.Flusher)
		existing.LastActivity = now
		existing.mu.Unlock()
		p.lastActivity.Store(now)

		slog.Log(context.Background(), observability.LevelTrace, "Client activity updated",
			slog.String("processor_id", p.id),
			slog.String("client_id", clientID),
			slog.Duration("since_last_activity", now.Sub(oldActivity)))
		return existing, nil
	}

	// Create new client
	client := NewProcessorClient(clientID, w, r)
	p.clients[clientID] = client

	p.lastActivity.Store(now)

	slog.Log(context.Background(), observability.LevelTrace, "New client registered",
		slog.String("processor_id", p.id),
		slog.String("client_id", clientID))
	return client, nil
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

		idleDuration := now.Sub(lastActivity)
		if idleDuration > timeout {
			slog.Log(context.Background(), observability.LevelTrace, "Removing inactive client",
				slog.String("processor_id", p.id),
				slog.String("client_id", id),
				slog.Duration("idle_duration", idleDuration),
				slog.Duration("timeout", timeout))
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

// IsIdle returns true if the processor has no connected clients.
// This is the default implementation suitable for most processor types.
// Processors with different idle semantics (e.g., manifest-based) should override this.
func (p *BaseProcessor) IsIdle() bool {
	return p.ClientCount() == 0
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

// IsStopping returns true if the processor is being stopped.
func (p *BaseProcessor) IsStopping() bool {
	return p.stopping.Load()
}

// TryMarkForStopping atomically marks the processor for stopping if it has no clients.
// Returns true if the processor was successfully marked for stopping (and should be stopped).
// Returns false if there are active clients, meaning the processor should remain active.
//
// This method is used to prevent the TOCTOU race where:
// 1. StopProcessorIfIdle checks ClientCount() == 0
// 2. A new client registers before Stop() is called
// 3. The processor is stopped with an active client
//
// By holding the clientsMu lock while both checking client count AND setting the stopping flag,
// we ensure that RegisterClientBase will see the stopping flag and reject the registration.
func (p *BaseProcessor) TryMarkForStopping() bool {
	p.clientsMu.Lock()
	defer p.clientsMu.Unlock()

	// If there are clients, don't mark for stopping
	if len(p.clients) > 0 {
		return false
	}

	// No clients - mark as stopping to prevent new registrations
	p.stopping.Store(true)
	return true
}

// ClearStopping clears the stopping flag. This is used if the processor
// is not actually being removed (e.g., it was re-added to the session map).
func (p *BaseProcessor) ClearStopping() {
	p.stopping.Store(false)
}

// ClosedChan returns a channel that is closed when the processor is closed.
func (p *BaseProcessor) ClosedChan() <-chan struct{} {
	return p.closedCh
}
