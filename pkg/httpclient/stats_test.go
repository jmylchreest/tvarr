package httpclient

import (
	"testing"
	"time"
)

func TestErrorCategoryCount(t *testing.T) {
	t.Run("increment and total", func(t *testing.T) {
		var counts ErrorCategoryCount

		counts.Increment(ErrorCategorySuccess2xx)
		counts.Increment(ErrorCategorySuccess2xx)
		counts.Increment(ErrorCategoryClientError4xx)
		counts.Increment(ErrorCategoryServerError5xx)
		counts.Increment(ErrorCategoryTimeout)
		counts.Increment(ErrorCategoryNetworkError)

		if counts.Success2xx != 2 {
			t.Errorf("expected Success2xx=2, got %d", counts.Success2xx)
		}
		if counts.ClientError4xx != 1 {
			t.Errorf("expected ClientError4xx=1, got %d", counts.ClientError4xx)
		}
		if counts.ServerError5xx != 1 {
			t.Errorf("expected ServerError5xx=1, got %d", counts.ServerError5xx)
		}
		if counts.Timeout != 1 {
			t.Errorf("expected Timeout=1, got %d", counts.Timeout)
		}
		if counts.NetworkError != 1 {
			t.Errorf("expected NetworkError=1, got %d", counts.NetworkError)
		}
		if counts.Total() != 6 {
			t.Errorf("expected total=6, got %d", counts.Total())
		}
	})

	t.Run("reset", func(t *testing.T) {
		counts := ErrorCategoryCount{
			Success2xx:     10,
			ClientError4xx: 5,
			ServerError5xx: 3,
			Timeout:        2,
			NetworkError:   1,
		}

		counts.Reset()

		if counts.Total() != 0 {
			t.Errorf("expected total=0 after reset, got %d", counts.Total())
		}
	})

	t.Run("clone", func(t *testing.T) {
		original := ErrorCategoryCount{
			Success2xx:     10,
			ClientError4xx: 5,
		}

		clone := original.Clone()
		original.Success2xx = 20

		if clone.Success2xx != 10 {
			t.Errorf("clone should be independent, expected 10, got %d", clone.Success2xx)
		}
	})
}

func TestCategorizeHTTPStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected ErrorCategory
	}{
		{200, ErrorCategorySuccess2xx},
		{201, ErrorCategorySuccess2xx},
		{204, ErrorCategorySuccess2xx},
		{299, ErrorCategorySuccess2xx},
		{400, ErrorCategoryClientError4xx},
		{401, ErrorCategoryClientError4xx},
		{404, ErrorCategoryClientError4xx},
		{499, ErrorCategoryClientError4xx},
		{500, ErrorCategoryServerError5xx},
		{502, ErrorCategoryServerError5xx},
		{503, ErrorCategoryServerError5xx},
		{599, ErrorCategoryServerError5xx},
		{100, ErrorCategoryNetworkError}, // Non-standard
		{0, ErrorCategoryNetworkError},   // Invalid
	}

	for _, tc := range tests {
		got := categorizeHTTPStatus(tc.status)
		if got != tc.expected {
			t.Errorf("categorizeHTTPStatus(%d) = %v, want %v", tc.status, got, tc.expected)
		}
	}
}

func TestStateTracker(t *testing.T) {
	t.Run("initial state", func(t *testing.T) {
		tracker := NewStateTracker()

		transitions := tracker.GetTransitions()
		if len(transitions) != 0 {
			t.Errorf("expected no transitions initially, got %d", len(transitions))
		}

		enteredAt := tracker.GetStateEnteredAt()
		if enteredAt.IsZero() {
			t.Error("expected state entered time to be set")
		}
	})

	t.Run("record transitions", func(t *testing.T) {
		tracker := NewStateTracker()

		// Record some transitions
		tracker.RecordTransition(CircuitClosed, CircuitOpen, TransitionReasonThresholdExceeded, 5)
		tracker.RecordTransition(CircuitOpen, CircuitHalfOpen, TransitionReasonTimeoutRecovery, 0)
		tracker.RecordTransition(CircuitHalfOpen, CircuitClosed, TransitionReasonProbeSuccess, 1)

		transitions := tracker.GetTransitions()
		if len(transitions) != 3 {
			t.Errorf("expected 3 transitions, got %d", len(transitions))
		}

		// Verify transitions are in chronological order
		if transitions[0].Reason != TransitionReasonThresholdExceeded {
			t.Errorf("first transition reason should be threshold_exceeded, got %s", transitions[0].Reason)
		}
		if transitions[1].Reason != TransitionReasonTimeoutRecovery {
			t.Errorf("second transition reason should be timeout_recovery, got %s", transitions[1].Reason)
		}
		if transitions[2].Reason != TransitionReasonProbeSuccess {
			t.Errorf("third transition reason should be probe_success, got %s", transitions[2].Reason)
		}
	})

	t.Run("circular buffer overflow", func(t *testing.T) {
		tracker := NewStateTracker()

		// Record more than maxTransitionHistory transitions
		for i := range maxTransitionHistory + 10 {
			tracker.RecordTransition(CircuitClosed, CircuitOpen, TransitionReasonThresholdExceeded, i)
		}

		transitions := tracker.GetTransitions()
		if len(transitions) != maxTransitionHistory {
			t.Errorf("expected %d transitions (max), got %d", maxTransitionHistory, len(transitions))
		}

		// Verify oldest transitions were discarded
		// The first transition should have ConsecutiveCount = 10 (not 0)
		if transitions[0].ConsecutiveCount != 10 {
			t.Errorf("expected first transition to have count=10, got %d", transitions[0].ConsecutiveCount)
		}
	})

	t.Run("duration summary", func(t *testing.T) {
		tracker := NewStateTracker()

		// Wait a bit to accumulate some duration
		time.Sleep(10 * time.Millisecond)

		summary := tracker.GetDurationSummary()

		if summary.TotalMs == 0 {
			t.Error("expected non-zero total duration")
		}
		if summary.ClosedMs == 0 {
			t.Error("expected non-zero closed duration")
		}
		if summary.ClosedPct < 99 {
			t.Errorf("expected closed percentage to be near 100, got %.1f", summary.ClosedPct)
		}
	})

	t.Run("reset", func(t *testing.T) {
		tracker := NewStateTracker()

		tracker.RecordTransition(CircuitClosed, CircuitOpen, TransitionReasonThresholdExceeded, 5)
		tracker.Reset()

		transitions := tracker.GetTransitions()
		if len(transitions) != 0 {
			t.Errorf("expected no transitions after reset, got %d", len(transitions))
		}
	})
}

func TestErrorCategoryString(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		expected string
	}{
		{ErrorCategorySuccess2xx, "success_2xx"},
		{ErrorCategoryClientError4xx, "client_error_4xx"},
		{ErrorCategoryServerError5xx, "server_error_5xx"},
		{ErrorCategoryTimeout, "timeout"},
		{ErrorCategoryNetworkError, "network_error"},
		{ErrorCategory(99), "unknown"},
	}

	for _, tc := range tests {
		got := tc.category.String()
		if got != tc.expected {
			t.Errorf("%d.String() = %q, want %q", tc.category, got, tc.expected)
		}
	}
}

func TestCircuitBreakerEnhancedStats(t *testing.T) {
	t.Run("tracks error categories", func(t *testing.T) {
		cb := NewCircuitBreaker(10, 100*time.Millisecond, 1)

		// Record various types of results
		cb.RecordSuccess()
		cb.RecordSuccess()
		cb.RecordFailureWithCategory(ErrorCategoryClientError4xx)
		cb.RecordFailureWithCategory(ErrorCategoryServerError5xx)
		cb.RecordFailureWithCategory(ErrorCategoryTimeout)

		stats := cb.EnhancedStats("test")

		if stats.ErrorCounts.Success2xx != 2 {
			t.Errorf("expected Success2xx=2, got %d", stats.ErrorCounts.Success2xx)
		}
		if stats.ErrorCounts.ClientError4xx != 1 {
			t.Errorf("expected ClientError4xx=1, got %d", stats.ErrorCounts.ClientError4xx)
		}
		if stats.ErrorCounts.ServerError5xx != 1 {
			t.Errorf("expected ServerError5xx=1, got %d", stats.ErrorCounts.ServerError5xx)
		}
		if stats.ErrorCounts.Timeout != 1 {
			t.Errorf("expected Timeout=1, got %d", stats.ErrorCounts.Timeout)
		}
		if stats.TotalRequests != 5 {
			t.Errorf("expected TotalRequests=5, got %d", stats.TotalRequests)
		}
	})

	t.Run("tracks state transitions", func(t *testing.T) {
		cb := NewCircuitBreaker(2, 10*time.Millisecond, 1)

		// Trigger open
		cb.RecordFailure()
		cb.RecordFailure()

		// Wait for half-open
		time.Sleep(15 * time.Millisecond)
		cb.Allow()

		// Success to close
		cb.RecordSuccess()

		stats := cb.EnhancedStats("test")

		if len(stats.Transitions) < 2 {
			t.Errorf("expected at least 2 transitions, got %d", len(stats.Transitions))
		}

		// First transition should be threshold exceeded
		if stats.Transitions[0].Reason != TransitionReasonThresholdExceeded {
			t.Errorf("first transition reason should be threshold_exceeded, got %s", stats.Transitions[0].Reason)
		}
	})

	t.Run("tracks state duration", func(t *testing.T) {
		cb := NewCircuitBreaker(10, 100*time.Millisecond, 1)

		time.Sleep(10 * time.Millisecond)

		stats := cb.EnhancedStats("test")

		if stats.StateDurationMs < 5 {
			t.Errorf("expected state duration >= 5ms, got %d", stats.StateDurationMs)
		}
		if stats.StateDurations.ClosedMs < 5 {
			t.Errorf("expected closed duration >= 5ms, got %d", stats.StateDurations.ClosedMs)
		}
	})

	t.Run("calculates next half-open time when open", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 100*time.Millisecond, 1)

		cb.RecordFailure()

		stats := cb.EnhancedStats("test")

		if stats.State != CircuitOpen {
			t.Errorf("expected state to be open, got %s", stats.State)
		}
		if stats.NextHalfOpenAt.IsZero() {
			t.Error("expected NextHalfOpenAt to be set when open")
		}
	})

	t.Run("records manual reset transition", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 100*time.Millisecond, 1)

		cb.RecordFailure()
		cb.Reset()

		stats := cb.EnhancedStats("test")

		// Should have two transitions: threshold_exceeded and manual_reset
		if len(stats.Transitions) != 2 {
			t.Errorf("expected 2 transitions, got %d", len(stats.Transitions))
		}

		lastTransition := stats.Transitions[len(stats.Transitions)-1]
		if lastTransition.Reason != TransitionReasonManualReset {
			t.Errorf("last transition should be manual_reset, got %s", lastTransition.Reason)
		}
	})
}

func TestCircuitBreakerManagerEnhancedStats(t *testing.T) {
	t.Run("returns enhanced stats for all breakers", func(t *testing.T) {
		manager := NewCircuitBreakerManager(nil)

		// Create some breakers
		cb1 := manager.GetOrCreate("service1")
		cb2 := manager.GetOrCreate("service2")

		cb1.RecordSuccess()
		cb2.RecordFailure()

		stats := manager.GetAllEnhancedStats()

		if len(stats) != 2 {
			t.Errorf("expected 2 breakers in stats, got %d", len(stats))
		}

		if stats["service1"].Name != "service1" {
			t.Errorf("expected name 'service1', got '%s'", stats["service1"].Name)
		}
		if stats["service2"].Name != "service2" {
			t.Errorf("expected name 'service2', got '%s'", stats["service2"].Name)
		}
	})

	t.Run("returns individual enhanced stats", func(t *testing.T) {
		manager := NewCircuitBreakerManager(nil)

		cb := manager.GetOrCreate("myservice")
		cb.RecordSuccess()

		stats, ok := manager.GetEnhancedStats("myservice")
		if !ok {
			t.Fatal("expected to find myservice stats")
		}

		if stats.Name != "myservice" {
			t.Errorf("expected name 'myservice', got '%s'", stats.Name)
		}
		if stats.TotalSuccesses != 1 {
			t.Errorf("expected TotalSuccesses=1, got %d", stats.TotalSuccesses)
		}
	})

	t.Run("returns false for non-existent breaker", func(t *testing.T) {
		manager := NewCircuitBreakerManager(nil)

		_, ok := manager.GetEnhancedStats("nonexistent")
		if ok {
			t.Error("expected ok=false for non-existent breaker")
		}
	})
}
