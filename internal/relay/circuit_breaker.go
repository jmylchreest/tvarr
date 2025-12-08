// Package relay provides stream relay functionality including transcoding,
// connection pooling, and failure handling.
package relay

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed allows requests through normally.
	CircuitClosed CircuitState = iota
	// CircuitOpen rejects requests immediately.
	CircuitOpen
	// CircuitHalfOpen allows a limited number of test requests.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of successes in half-open state to close the circuit.
	SuccessThreshold int
	// Timeout is how long the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// OnStateChange is called when the circuit state changes.
	OnStateChange func(from, to CircuitState)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config CircuitBreakerConfig

	mu              sync.RWMutex
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	lastStateChange time.Time
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:          config,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Check if we should transition from open to half-open
	if cb.state == CircuitOpen && time.Since(cb.lastFailureTime) >= cb.config.Timeout {
		return CircuitHalfOpen
	}

	return cb.state
}

// Allow checks if a request is allowed through.
func (cb *CircuitBreaker) Allow() bool {
	state := cb.State()
	return state == CircuitClosed || state == CircuitHalfOpen
}

// Execute runs a function through the circuit breaker.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	err := fn(ctx)
	if err != nil {
		cb.RecordFailure()
	} else {
		cb.RecordSuccess()
	}

	return err
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failures = 0 // Reset failures on success

	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.transitionTo(CircuitClosed)
		}

	case CircuitOpen:
		// Check if we should be half-open
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			cb.state = CircuitHalfOpen
			cb.successes = 1
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen)
		}

	case CircuitHalfOpen:
		// Any failure in half-open immediately opens the circuit
		cb.transitionTo(CircuitOpen)

	case CircuitOpen:
		// Already open, just update failure time
	}
}

// transitionTo changes the circuit state (must be called with lock held).
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Reset counters
	cb.failures = 0
	cb.successes = 0

	// Notify callback
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(oldState, newState)
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state != CircuitClosed {
		cb.transitionTo(CircuitClosed)
	} else {
		cb.failures = 0
		cb.successes = 0
	}
}

// Stats returns current circuit breaker statistics.
func (cb *CircuitBreaker) Stats() CircuitStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitStats{
		State:           cb.State().String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: cb.lastFailureTime,
		LastStateChange: cb.lastStateChange,
	}
}

// CircuitStats holds circuit breaker statistics.
type CircuitStats struct {
	State           string    `json:"state"`
	Failures        int       `json:"failures"`
	Successes       int       `json:"successes"`
	LastFailureTime time.Time `json:"last_failure_time,omitempty"`
	LastStateChange time.Time `json:"last_state_change"`
}

// CircuitBreakerRegistry manages circuit breakers for multiple endpoints.
type CircuitBreakerRegistry struct {
	config CircuitBreakerConfig
	mu     sync.RWMutex
	cbs    map[string]*CircuitBreaker
}

// NewCircuitBreakerRegistry creates a new registry.
func NewCircuitBreakerRegistry(config CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		config: config,
		cbs:    make(map[string]*CircuitBreaker),
	}
}

// Get returns or creates a circuit breaker for the given key.
func (r *CircuitBreakerRegistry) Get(key string) *CircuitBreaker {
	r.mu.RLock()
	cb, ok := r.cbs[key]
	r.mu.RUnlock()

	if ok {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok := r.cbs[key]; ok {
		return cb
	}

	cb = NewCircuitBreaker(r.config)
	r.cbs[key] = cb
	return cb
}

// Remove removes a circuit breaker from the registry.
func (r *CircuitBreakerRegistry) Remove(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cbs, key)
}

// AllStats returns statistics for all circuit breakers.
func (r *CircuitBreakerRegistry) AllStats() map[string]CircuitStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]CircuitStats, len(r.cbs))
	for key, cb := range r.cbs {
		stats[key] = cb.Stats()
	}
	return stats
}

// OpenCircuits returns keys of all open circuit breakers.
func (r *CircuitBreakerRegistry) OpenCircuits() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var open []string
	for key, cb := range r.cbs {
		if cb.State() == CircuitOpen {
			open = append(open, key)
		}
	}
	return open
}

// ResetAll resets all circuit breakers in the registry.
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.cbs {
		cb.Reset()
	}
}

// Count returns the number of circuit breakers in the registry.
func (r *CircuitBreakerRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cbs)
}

// Cleanup removes circuit breakers that have been closed for a long time.
func (r *CircuitBreakerRegistry) Cleanup(maxAge time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0
	now := time.Now()

	for key, cb := range r.cbs {
		stats := cb.Stats()
		if stats.State == "closed" && now.Sub(stats.LastStateChange) > maxAge {
			delete(r.cbs, key)
			removed++
		}
	}

	return removed
}
