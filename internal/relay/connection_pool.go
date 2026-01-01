package relay

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"
)

// ErrPoolExhausted is returned when a connection pool limit is reached.
var ErrPoolExhausted = errors.New("connection pool exhausted")

// ErrPoolClosed is returned when trying to use a closed pool.
var ErrPoolClosed = errors.New("connection pool closed")

// ConnectionPoolConfig holds configuration for connection pooling.
type ConnectionPoolConfig struct {
	// MaxConnsPerHost is the maximum concurrent connections per host.
	MaxConnsPerHost int
	// GlobalMaxConns is the total maximum concurrent connections.
	GlobalMaxConns int
	// AcquireTimeout is how long to wait for a connection slot.
	AcquireTimeout time.Duration
	// OnLimitReached is called when a limit is reached.
	OnLimitReached func(host string, current, max int)
}

// DefaultConnectionPoolConfig returns sensible defaults.
func DefaultConnectionPoolConfig() ConnectionPoolConfig {
	return ConnectionPoolConfig{
		MaxConnsPerHost: 3,
		GlobalMaxConns:  50,
		AcquireTimeout:  30 * time.Second,
	}
}

// ConnectionPool manages concurrent connections per host.
type ConnectionPool struct {
	config ConnectionPoolConfig

	mu         sync.Mutex
	closed     bool
	hostConns  map[string]int
	globalConn int
	waiters    map[string][]chan struct{}
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(config ConnectionPoolConfig) *ConnectionPool {
	return &ConnectionPool{
		config:    config,
		hostConns: make(map[string]int),
		waiters:   make(map[string][]chan struct{}),
	}
}

// Acquire acquires a connection slot for the given URL.
// It returns a release function that must be called when done.
func (p *ConnectionPool) Acquire(ctx context.Context, rawURL string) (func(), error) {
	host, err := extractHost(rawURL)
	if err != nil {
		return nil, err
	}

	return p.AcquireForHost(ctx, host)
}

// AcquireForHost acquires a connection slot for the given host.
func (p *ConnectionPool) AcquireForHost(ctx context.Context, host string) (func(), error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Check if we can acquire immediately
	if p.canAcquire(host) {
		p.hostConns[host]++
		p.globalConn++
		p.mu.Unlock()
		return p.releaseFunc(host), nil
	}

	// Create waiter channel
	waiter := make(chan struct{}, 1)
	p.waiters[host] = append(p.waiters[host], waiter)
	p.mu.Unlock()

	// Call limit callback
	if p.config.OnLimitReached != nil {
		p.config.OnLimitReached(host, p.hostConns[host], p.config.MaxConnsPerHost)
	}

	// Wait for slot with timeout
	var timeoutCtx context.Context
	var cancel context.CancelFunc

	if p.config.AcquireTimeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, p.config.AcquireTimeout)
	} else {
		timeoutCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	select {
	case <-waiter:
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrPoolClosed
		}
		p.hostConns[host]++
		p.globalConn++
		p.mu.Unlock()
		return p.releaseFunc(host), nil

	case <-timeoutCtx.Done():
		p.mu.Lock()
		p.removeWaiter(host, waiter)
		p.mu.Unlock()
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return nil, ErrPoolExhausted
		}
		return nil, timeoutCtx.Err()
	}
}

// canAcquire checks if a connection can be acquired (must hold lock).
func (p *ConnectionPool) canAcquire(host string) bool {
	if p.config.GlobalMaxConns > 0 && p.globalConn >= p.config.GlobalMaxConns {
		return false
	}
	if p.config.MaxConnsPerHost > 0 && p.hostConns[host] >= p.config.MaxConnsPerHost {
		return false
	}
	return true
}

// releaseFunc returns a function that releases a connection slot.
func (p *ConnectionPool) releaseFunc(host string) func() {
	return func() {
		p.Release(host)
	}
}

// Release releases a connection slot for the given host.
func (p *ConnectionPool) Release(host string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.hostConns[host] > 0 {
		p.hostConns[host]--
		if p.hostConns[host] == 0 {
			delete(p.hostConns, host)
		}
	}

	if p.globalConn > 0 {
		p.globalConn--
	}

	// Notify any waiters for this host
	if len(p.waiters[host]) > 0 {
		waiter := p.waiters[host][0]
		p.waiters[host] = p.waiters[host][1:]
		if len(p.waiters[host]) == 0 {
			delete(p.waiters, host)
		}
		select {
		case waiter <- struct{}{}:
		default:
		}
		return
	}

	// Or notify any other host waiter if global slot freed
	for h, ws := range p.waiters {
		if len(ws) > 0 && p.canAcquire(h) {
			waiter := ws[0]
			p.waiters[h] = ws[1:]
			if len(p.waiters[h]) == 0 {
				delete(p.waiters, h)
			}
			select {
			case waiter <- struct{}{}:
			default:
			}
			return
		}
	}
}

// removeWaiter removes a waiter channel (must hold lock).
func (p *ConnectionPool) removeWaiter(host string, waiter chan struct{}) {
	waiters := p.waiters[host]
	for i, w := range waiters {
		if w == waiter {
			p.waiters[host] = append(waiters[:i], waiters[i+1:]...)
			if len(p.waiters[host]) == 0 {
				delete(p.waiters, host)
			}
			break
		}
	}
}

// Close closes the connection pool and releases all waiters.
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	// Notify all waiters
	for _, waiters := range p.waiters {
		for _, w := range waiters {
			close(w)
		}
	}
	p.waiters = nil
}

// Stats returns connection pool statistics.
func (p *ConnectionPool) Stats() ConnectionPoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	hostStats := make(map[string]int, len(p.hostConns))
	for host, count := range p.hostConns {
		hostStats[host] = count
	}

	waitingCount := 0
	for _, waiters := range p.waiters {
		waitingCount += len(waiters)
	}

	return ConnectionPoolStats{
		GlobalConnections: p.globalConn,
		MaxGlobal:         p.config.GlobalMaxConns,
		HostConnections:   hostStats,
		MaxPerHost:        p.config.MaxConnsPerHost,
		WaitingCount:      waitingCount,
	}
}

// ConnectionPoolStats holds connection pool statistics.
type ConnectionPoolStats struct {
	GlobalConnections int            `json:"global_connections"`
	MaxGlobal         int            `json:"max_global"`
	HostConnections   map[string]int `json:"host_connections"`
	MaxPerHost        int            `json:"max_per_host"`
	WaitingCount      int            `json:"waiting_count"`
}

// extractHost extracts the host from a URL.
func extractHost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

