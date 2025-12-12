package relay

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Circuit Breaker Tests

func TestCircuitBreaker_InitialState(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	assert.Equal(t, CircuitClosed, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Record failures
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	assert.Equal(t, CircuitOpen, cb.State())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	assert.Equal(t, CircuitHalfOpen, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_ClosesAfterSuccesses(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// Should be half-open
	assert.Equal(t, CircuitHalfOpen, cb.State())

	// Record successes
	cb.RecordSuccess()
	cb.RecordSuccess()

	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_ReopensOnHalfOpenFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// Should be half-open
	assert.Equal(t, CircuitHalfOpen, cb.State())

	// Fail in half-open state
	cb.RecordFailure()

	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_Execute(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Successful execution
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	assert.NoError(t, err)

	// Failed execution
	err = cb.Execute(ctx, func(ctx context.Context) error {
		return assert.AnError
	})
	assert.Error(t, err)
}

func TestCircuitBreaker_ExecuteRejectsWhenOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          1 * time.Hour, // Long timeout
	}
	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Open the circuit
	cb.RecordFailure()

	// Should be rejected
	err := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.State())

	// Reset
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var fromState, toState CircuitState
	var callbackCalled bool
	var mu sync.Mutex

	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		OnStateChange: func(from, to CircuitState) {
			mu.Lock()
			fromState = from
			toState = to
			callbackCalled = true
			mu.Unlock()
		},
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure()

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	assert.True(t, callbackCalled)
	assert.Equal(t, CircuitClosed, fromState)
	assert.Equal(t, CircuitOpen, toState)
	mu.Unlock()
}

func TestCircuitBreakerRegistry_Get(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	registry := NewCircuitBreakerRegistry(config)

	cb1 := registry.Get("host1")
	cb2 := registry.Get("host1")
	cb3 := registry.Get("host2")

	assert.Same(t, cb1, cb2)
	assert.NotSame(t, cb1, cb3)
	assert.Equal(t, 2, registry.Count())
}

func TestCircuitBreakerRegistry_Remove(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	registry := NewCircuitBreakerRegistry(config)

	registry.Get("host1")
	registry.Get("host2")
	assert.Equal(t, 2, registry.Count())

	registry.Remove("host1")
	assert.Equal(t, 1, registry.Count())
}

func TestCircuitBreakerRegistry_OpenCircuits(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          1 * time.Hour,
	}
	registry := NewCircuitBreakerRegistry(config)

	registry.Get("host1").RecordFailure()
	registry.Get("host2") // Stays closed
	registry.Get("host3").RecordFailure()

	open := registry.OpenCircuits()
	assert.Len(t, open, 2)
	assert.Contains(t, open, "host1")
	assert.Contains(t, open, "host3")
}

// Connection Pool Tests

func TestConnectionPool_Acquire(t *testing.T) {
	config := DefaultConnectionPoolConfig()
	pool := NewConnectionPool(config)
	defer pool.Close()
	ctx := context.Background()

	release, err := pool.Acquire(ctx, "http://example.com/stream")
	require.NoError(t, err)
	require.NotNil(t, release)

	stats := pool.Stats()
	assert.Equal(t, 1, stats.GlobalConnections)
	assert.Equal(t, 1, stats.HostConnections["example.com"])

	release()

	stats = pool.Stats()
	assert.Equal(t, 0, stats.GlobalConnections)
}

func TestConnectionPool_MaxPerHost(t *testing.T) {
	config := ConnectionPoolConfig{
		MaxConnsPerHost: 2,
		GlobalMaxConns:  100,
		AcquireTimeout:  50 * time.Millisecond,
	}
	pool := NewConnectionPool(config)
	defer pool.Close()
	ctx := context.Background()

	// Acquire max connections
	release1, err := pool.AcquireForHost(ctx, "host1")
	require.NoError(t, err)
	release2, err := pool.AcquireForHost(ctx, "host1")
	require.NoError(t, err)

	// Third should fail/timeout
	_, err = pool.AcquireForHost(ctx, "host1")
	assert.ErrorIs(t, err, ErrPoolExhausted)

	// Different host should work
	release3, err := pool.AcquireForHost(ctx, "host2")
	require.NoError(t, err)

	release1()
	release2()
	release3()
}

func TestConnectionPool_GlobalMax(t *testing.T) {
	config := ConnectionPoolConfig{
		MaxConnsPerHost: 10,
		GlobalMaxConns:  3,
		AcquireTimeout:  50 * time.Millisecond,
	}
	pool := NewConnectionPool(config)
	defer pool.Close()
	ctx := context.Background()

	releases := make([]func(), 3)
	for i := 0; i < 3; i++ {
		var err error
		releases[i], err = pool.AcquireForHost(ctx, "host"+string(rune('1'+i)))
		require.NoError(t, err)
	}

	// Should hit global limit
	_, err := pool.AcquireForHost(ctx, "host4")
	assert.ErrorIs(t, err, ErrPoolExhausted)

	for _, r := range releases {
		r()
	}
}

func TestConnectionPool_WaitForSlot(t *testing.T) {
	config := ConnectionPoolConfig{
		MaxConnsPerHost: 1,
		GlobalMaxConns:  10,
		AcquireTimeout:  1 * time.Second,
	}
	pool := NewConnectionPool(config)
	defer pool.Close()
	ctx := context.Background()

	release1, err := pool.AcquireForHost(ctx, "host1")
	require.NoError(t, err)

	// Start waiting for slot in goroutine
	acquireDone := make(chan error, 1)
	go func() {
		_, err := pool.AcquireForHost(ctx, "host1")
		acquireDone <- err
	}()

	// Release after short delay
	time.Sleep(50 * time.Millisecond)
	release1()

	// Should succeed
	select {
	case err := <-acquireDone:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("acquire should have succeeded")
	}
}

func TestConnectionPool_Close(t *testing.T) {
	config := DefaultConnectionPoolConfig()
	pool := NewConnectionPool(config)
	ctx := context.Background()

	release, _ := pool.Acquire(ctx, "http://example.com/stream")
	release()

	pool.Close()

	_, err := pool.Acquire(ctx, "http://example.com/stream")
	assert.ErrorIs(t, err, ErrPoolClosed)
}

// Host Limiter Tests

func TestHostLimiter_Basic(t *testing.T) {
	limiter := NewHostLimiter(2)
	ctx := context.Background()

	// Should allow 2 concurrent
	err := limiter.Acquire(ctx, "host1")
	require.NoError(t, err)
	err = limiter.Acquire(ctx, "host1")
	require.NoError(t, err)

	assert.Equal(t, 2, limiter.ActiveForHost("host1"))

	// Third should block (with timeout)
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err = limiter.Acquire(ctx, "host1")
	assert.Error(t, err)

	// Release one
	limiter.Release("host1")
	assert.Equal(t, 1, limiter.ActiveForHost("host1"))
}

func TestStreamMode_String(t *testing.T) {
	tests := []struct {
		mode     StreamMode
		expected string
	}{
		{StreamModePassthroughRawTS, "passthrough-raw-ts"},
		{StreamModeCollapsedHLS, "collapsed-hls"},
		{StreamModeTransparentHLS, "transparent-hls"},
		{StreamModePassthroughHLS, "passthrough-hls"},
		{StreamModePassthroughDASH, "passthrough-dash"},
		{StreamModeUnknown, "unknown"},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, tc.mode.String())
	}
}

// Relay Manager Tests

func TestManager_CreateAndClose(t *testing.T) {
	config := DefaultManagerConfig()
	config.MaxSessions = 10
	config.SessionTimeout = 1 * time.Second
	config.CleanupInterval = 100 * time.Millisecond

	manager := NewManager(config)
	defer manager.Close()

	stats := manager.Stats()
	assert.Equal(t, 0, stats.ActiveSessions)
	assert.Equal(t, 10, stats.MaxSessions)
}

func TestManager_Close(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewManager(config)

	// Should not panic on double close
	manager.Close()
	manager.Close()
}
