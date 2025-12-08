// Package httpclient provides HTTP client utilities with circuit breaker support.
package httpclient

import (
	"sync"
	"time"
)

// MaxTransitionHistory is the maximum number of state transitions to retain.
const MaxTransitionHistory = 50

// ErrorCategory represents the category of an error for tracking purposes.
type ErrorCategory int

const (
	// ErrorCategorySuccess2xx represents 2xx HTTP responses.
	ErrorCategorySuccess2xx ErrorCategory = iota
	// ErrorCategoryClientError4xx represents 4xx HTTP responses.
	ErrorCategoryClientError4xx
	// ErrorCategoryServerError5xx represents 5xx HTTP responses.
	ErrorCategoryServerError5xx
	// ErrorCategoryTimeout represents timeout errors.
	ErrorCategoryTimeout
	// ErrorCategoryNetworkError represents network-level errors.
	ErrorCategoryNetworkError
)

// String returns the string representation of the error category.
func (c ErrorCategory) String() string {
	switch c {
	case ErrorCategorySuccess2xx:
		return "success_2xx"
	case ErrorCategoryClientError4xx:
		return "client_error_4xx"
	case ErrorCategoryServerError5xx:
		return "server_error_5xx"
	case ErrorCategoryTimeout:
		return "timeout"
	case ErrorCategoryNetworkError:
		return "network_error"
	default:
		return "unknown"
	}
}

// ErrorCategoryCount tracks counts by error category.
type ErrorCategoryCount struct {
	Success2xx     int64 `json:"success_2xx"`
	ClientError4xx int64 `json:"client_error_4xx"`
	ServerError5xx int64 `json:"server_error_5xx"`
	Timeout        int64 `json:"timeout"`
	NetworkError   int64 `json:"network_error"`
}

// Total returns the total count across all categories.
func (e *ErrorCategoryCount) Total() int64 {
	return e.Success2xx + e.ClientError4xx + e.ServerError5xx + e.Timeout + e.NetworkError
}

// Increment increments the count for the given category.
func (e *ErrorCategoryCount) Increment(category ErrorCategory) {
	switch category {
	case ErrorCategorySuccess2xx:
		e.Success2xx++
	case ErrorCategoryClientError4xx:
		e.ClientError4xx++
	case ErrorCategoryServerError5xx:
		e.ServerError5xx++
	case ErrorCategoryTimeout:
		e.Timeout++
	case ErrorCategoryNetworkError:
		e.NetworkError++
	}
}

// Reset resets all counts to zero.
func (e *ErrorCategoryCount) Reset() {
	e.Success2xx = 0
	e.ClientError4xx = 0
	e.ServerError5xx = 0
	e.Timeout = 0
	e.NetworkError = 0
}

// Clone returns a copy of the error counts.
func (e *ErrorCategoryCount) Clone() ErrorCategoryCount {
	return ErrorCategoryCount{
		Success2xx:     e.Success2xx,
		ClientError4xx: e.ClientError4xx,
		ServerError5xx: e.ServerError5xx,
		Timeout:        e.Timeout,
		NetworkError:   e.NetworkError,
	}
}

// TransitionReason represents the reason for a circuit breaker state transition.
type TransitionReason string

const (
	// TransitionReasonThresholdExceeded indicates consecutive failures exceeded the threshold.
	TransitionReasonThresholdExceeded TransitionReason = "threshold_exceeded"
	// TransitionReasonTimeoutRecovery indicates the reset timeout expired.
	TransitionReasonTimeoutRecovery TransitionReason = "timeout_recovery"
	// TransitionReasonProbeSuccess indicates a successful request in half-open state.
	TransitionReasonProbeSuccess TransitionReason = "probe_success"
	// TransitionReasonProbeFailure indicates a failed request in half-open state.
	TransitionReasonProbeFailure TransitionReason = "probe_failure"
	// TransitionReasonManualReset indicates a manual reset via API.
	TransitionReasonManualReset TransitionReason = "manual_reset"
)

// StateTransition records a circuit breaker state transition.
type StateTransition struct {
	Timestamp        time.Time        `json:"timestamp"`
	FromState        CircuitState     `json:"from_state"`
	ToState          CircuitState     `json:"to_state"`
	Reason           TransitionReason `json:"reason"`
	ConsecutiveCount int              `json:"consecutive_count"`
}

// StateDurationSummary tracks cumulative time spent in each state.
type StateDurationSummary struct {
	ClosedMs    int64   `json:"closed_ms"`
	OpenMs      int64   `json:"open_ms"`
	HalfOpenMs  int64   `json:"half_open_ms"`
	TotalMs     int64   `json:"total_ms"`
	ClosedPct   float64 `json:"closed_pct"`
	OpenPct     float64 `json:"open_pct"`
	HalfOpenPct float64 `json:"half_open_pct"`
}

// StateTracker tracks circuit breaker state timing and transitions.
type StateTracker struct {
	mu sync.RWMutex

	// Current state entry time
	stateEnteredAt time.Time
	currentState   CircuitState

	// Cumulative durations per state
	closedDuration   time.Duration
	openDuration     time.Duration
	halfOpenDuration time.Duration

	// Transition history (circular buffer)
	transitions []StateTransition
	startIndex  int
	count       int
}

// NewStateTracker creates a new state tracker starting in the closed state.
func NewStateTracker() *StateTracker {
	return &StateTracker{
		stateEnteredAt: time.Now(),
		currentState:   CircuitClosed,
		transitions:    make([]StateTransition, MaxTransitionHistory),
	}
}

// RecordTransition records a state transition.
func (t *StateTracker) RecordTransition(fromState, toState CircuitState, reason TransitionReason, consecutiveCount int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	// Update cumulative duration for the previous state
	if !t.stateEnteredAt.IsZero() {
		duration := now.Sub(t.stateEnteredAt)
		t.addDuration(fromState, duration)
	}

	// Record the transition
	transition := StateTransition{
		Timestamp:        now,
		FromState:        fromState,
		ToState:          toState,
		Reason:           reason,
		ConsecutiveCount: consecutiveCount,
	}

	// Add to circular buffer
	idx := (t.startIndex + t.count) % MaxTransitionHistory
	if t.count < MaxTransitionHistory {
		t.count++
	} else {
		t.startIndex = (t.startIndex + 1) % MaxTransitionHistory
	}
	t.transitions[idx] = transition

	// Update current state
	t.currentState = toState
	t.stateEnteredAt = now
}

// addDuration adds duration to the appropriate state counter.
func (t *StateTracker) addDuration(state CircuitState, duration time.Duration) {
	switch state {
	case CircuitClosed:
		t.closedDuration += duration
	case CircuitOpen:
		t.openDuration += duration
	case CircuitHalfOpen:
		t.halfOpenDuration += duration
	}
}

// GetStateEnteredAt returns when the current state was entered.
func (t *StateTracker) GetStateEnteredAt() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stateEnteredAt
}

// GetStateDurationMs returns the duration in the current state in milliseconds.
func (t *StateTracker) GetStateDurationMs() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.stateEnteredAt.IsZero() {
		return 0
	}
	return time.Since(t.stateEnteredAt).Milliseconds()
}

// GetDurationSummary returns a summary of time spent in each state.
func (t *StateTracker) GetDurationSummary() StateDurationSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Include current state duration up to now
	closedMs := t.closedDuration.Milliseconds()
	openMs := t.openDuration.Milliseconds()
	halfOpenMs := t.halfOpenDuration.Milliseconds()

	if !t.stateEnteredAt.IsZero() {
		currentDuration := time.Since(t.stateEnteredAt).Milliseconds()
		switch t.currentState {
		case CircuitClosed:
			closedMs += currentDuration
		case CircuitOpen:
			openMs += currentDuration
		case CircuitHalfOpen:
			halfOpenMs += currentDuration
		}
	}

	totalMs := closedMs + openMs + halfOpenMs

	summary := StateDurationSummary{
		ClosedMs:   closedMs,
		OpenMs:     openMs,
		HalfOpenMs: halfOpenMs,
		TotalMs:    totalMs,
	}

	// Calculate percentages
	if totalMs > 0 {
		summary.ClosedPct = float64(closedMs) / float64(totalMs) * 100
		summary.OpenPct = float64(openMs) / float64(totalMs) * 100
		summary.HalfOpenPct = float64(halfOpenMs) / float64(totalMs) * 100
	}

	return summary
}

// GetTransitions returns the transition history in chronological order.
func (t *StateTracker) GetTransitions() []StateTransition {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return nil
	}

	result := make([]StateTransition, t.count)
	for i := 0; i < t.count; i++ {
		idx := (t.startIndex + i) % MaxTransitionHistory
		result[i] = t.transitions[idx]
	}
	return result
}

// Reset resets the state tracker to initial state.
func (t *StateTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stateEnteredAt = time.Now()
	t.currentState = CircuitClosed
	t.closedDuration = 0
	t.openDuration = 0
	t.halfOpenDuration = 0
	t.transitions = make([]StateTransition, MaxTransitionHistory)
	t.startIndex = 0
	t.count = 0
}

// EnhancedCircuitBreakerStats holds enhanced statistics about a circuit breaker.
type EnhancedCircuitBreakerStats struct {
	// Identity
	Name string `json:"name"`

	// Current state
	State           CircuitState `json:"state"`
	StateEnteredAt  time.Time    `json:"state_entered_at"`
	StateDurationMs int64        `json:"state_duration_ms"`

	// Counters
	ConsecutiveFailures  int     `json:"consecutive_failures"`
	ConsecutiveSuccesses int     `json:"consecutive_successes"`
	TotalRequests        int64   `json:"total_requests"`
	TotalSuccesses       int64   `json:"total_successes"`
	TotalFailures        int64   `json:"total_failures"`
	FailureRate          float64 `json:"failure_rate"`

	// Error categorization
	ErrorCounts ErrorCategoryCount `json:"error_counts"`

	// State duration tracking
	StateDurations StateDurationSummary `json:"state_durations"`

	// Transition history
	Transitions []StateTransition `json:"transitions,omitempty"`

	// Timestamps
	LastFailure time.Time `json:"last_failure,omitempty"`
	LastSuccess time.Time `json:"last_success,omitempty"`

	// Recovery info (when open)
	NextHalfOpenAt time.Time `json:"next_half_open_at,omitempty"`

	// Config reference
	Config CircuitBreakerProfileConfig `json:"config"`
}

// CategorizeHTTPStatus returns the error category for an HTTP status code.
func CategorizeHTTPStatus(statusCode int) ErrorCategory {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return ErrorCategorySuccess2xx
	case statusCode >= 400 && statusCode < 500:
		return ErrorCategoryClientError4xx
	case statusCode >= 500:
		return ErrorCategoryServerError5xx
	default:
		return ErrorCategoryNetworkError
	}
}
