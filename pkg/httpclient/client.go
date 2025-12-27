// Package httpclient provides a resilient HTTP client with circuit breaker,
// automatic retries, transparent decompression, and structured logging.
//
// The client wraps the standard http.Client and adds production-grade features:
//   - Circuit breaker to prevent cascading failures
//   - Automatic retries with exponential backoff
//   - Transparent decompression (gzip, deflate, brotli)
//   - Structured logging (credential redaction handled by observability package)
//   - Configurable timeouts at connect and request levels
package httpclient

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

// Common errors returned by the client.
var (
	ErrCircuitOpen      = errors.New("circuit breaker is open")
	ErrMaxRetries       = errors.New("max retries exceeded")
	ErrRequestTimeout   = errors.New("request timeout")
	ErrResponseTooLarge = errors.New("response body exceeds maximum size limit")
)

// Default configuration values.
const (
	DefaultTimeout              = 30 * time.Second
	DefaultRetryAttempts        = 3
	DefaultRetryDelay           = 1 * time.Second
	DefaultRetryMaxDelay        = 30 * time.Second
	DefaultCircuitThreshold     = 5
	DefaultCircuitTimeout       = 30 * time.Second
	DefaultCircuitHalfOpenMax   = 1
	DefaultBackoffMultiplier    = 2.0
	DefaultMaxResponseBodyLog   = 1024
	DefaultMaxResponseSize      = 0 // 0 means no limit
	DefaultAcceptEncodingHeader = "gzip, deflate, br"
	DefaultUserAgentHeader      = "tvarr-httpclient/1.0"
)

// HTTP header constants.
const (
	HeaderAcceptEncoding  = "Accept-Encoding"
	HeaderContentEncoding = "Content-Encoding"
	HeaderUserAgent       = "User-Agent"

	EncodingGzip    = "gzip"
	EncodingDeflate = "deflate"
	EncodingBrotli  = "br"
)

// Config holds the configuration for the HTTP client.
type Config struct {
	// Timeout is the overall request timeout.
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts for failed requests.
	RetryAttempts int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration

	// RetryMaxDelay is the maximum delay between retries.
	RetryMaxDelay time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64

	// CircuitThreshold is the number of failures before the circuit opens.
	CircuitThreshold int

	// CircuitTimeout is how long the circuit stays open before trying again.
	CircuitTimeout time.Duration

	// CircuitHalfOpenMax is the max requests allowed in half-open state.
	CircuitHalfOpenMax int

	// UserAgent is the User-Agent header sent with requests.
	UserAgent string

	// Logger is the structured logger for request/response logging.
	Logger *slog.Logger

	// EnableDecompression enables automatic response decompression.
	EnableDecompression bool

	// MaxResponseSize is the maximum allowed response body size in bytes.
	// This limit is applied AFTER decompression to protect against zip bombs.
	// Set to 0 to disable the limit (default).
	MaxResponseSize int64

	// AcceptableStatusCodes specifies which HTTP status codes should be considered
	// "successful" for circuit breaker purposes.
	//
	// If set (non-nil/non-empty), ONLY these codes are acceptable - this gives full
	// control over what constitutes success. Supports both individual codes and ranges.
	//
	// Examples:
	//   AcceptableStatusCodes: MustParseStatusCodes("200-299,404")  // 2xx range + 404
	//   AcceptableStatusCodes: StatusCodesFromSlice([]int{200, 404}) // Individual codes
	//
	// If nil/empty (default), all 2xx status codes are considered acceptable.
	//
	// Note: Retryable status codes (429, 502, 503, 504) are always retried first,
	// regardless of this setting. This only affects circuit breaker failure tracking.
	AcceptableStatusCodes *StatusCodeSet

	// BaseClient is the underlying http.Client to use.
	// If nil, a default client is created.
	BaseClient *http.Client
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:             DefaultTimeout,
		RetryAttempts:       DefaultRetryAttempts,
		RetryDelay:          DefaultRetryDelay,
		RetryMaxDelay:       DefaultRetryMaxDelay,
		BackoffMultiplier:   DefaultBackoffMultiplier,
		CircuitThreshold:    DefaultCircuitThreshold,
		CircuitTimeout:      DefaultCircuitTimeout,
		CircuitHalfOpenMax:  DefaultCircuitHalfOpenMax,
		UserAgent:           DefaultUserAgentHeader,
		Logger:              slog.Default(),
		EnableDecompression: true,
		MaxResponseSize:     DefaultMaxResponseSize,
	}
}

// Client is a resilient HTTP client with circuit breaker and retry support.
type Client struct {
	config  Config
	client  *http.Client
	breaker *CircuitBreaker
	logger  *slog.Logger
}

// New creates a new resilient HTTP client with the given configuration.
func New(cfg Config) *Client {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	baseClient := cfg.BaseClient
	if baseClient == nil {
		baseClient = &http.Client{
			Timeout: cfg.Timeout,
		}
	}

	return &Client{
		config:  cfg,
		client:  baseClient,
		breaker: NewCircuitBreaker(cfg.CircuitThreshold, cfg.CircuitTimeout, cfg.CircuitHalfOpenMax),
		logger:  cfg.Logger,
	}
}

// NewWithDefaults creates a new client with default configuration.
func NewWithDefaults() *Client {
	return New(DefaultConfig())
}

// NewWithBreaker creates a new client with the given config and external circuit breaker.
// This allows sharing circuit breakers between clients (managed by CircuitBreakerManager).
// If breaker is nil, a new one is created based on the config.
func NewWithBreaker(cfg Config, breaker *CircuitBreaker) *Client {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	baseClient := cfg.BaseClient
	if baseClient == nil {
		baseClient = &http.Client{
			Timeout: cfg.Timeout,
		}
	}

	// Use provided breaker or create new one
	if breaker == nil {
		breaker = NewCircuitBreaker(cfg.CircuitThreshold, cfg.CircuitTimeout, cfg.CircuitHalfOpenMax)
	}

	return &Client{
		config:  cfg,
		client:  baseClient,
		breaker: breaker,
		logger:  cfg.Logger,
	}
}

// Do executes an HTTP request with circuit breaker protection and automatic retries.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.DoWithContext(req.Context(), req)
}

// DoWithContext executes an HTTP request with the given context.
func (c *Client) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Set default headers
	if req.Header.Get(HeaderUserAgent) == "" && c.config.UserAgent != "" {
		req.Header.Set(HeaderUserAgent, c.config.UserAgent)
	}
	if c.config.EnableDecompression && req.Header.Get(HeaderAcceptEncoding) == "" {
		req.Header.Set(HeaderAcceptEncoding, DefaultAcceptEncodingHeader)
	}

	var lastErr error
	delay := c.config.RetryDelay

	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			c.logger.Debug("retrying request",
				slog.Int("attempt", attempt),
				slog.Duration("delay", delay),
				slog.String("url", req.URL.String()),
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}

			// Exponential backoff
			delay = time.Duration(float64(delay) * c.config.BackoffMultiplier)
			if delay > c.config.RetryMaxDelay {
				delay = c.config.RetryMaxDelay
			}
		}

		// Check circuit breaker
		if !c.breaker.Allow() {
			lastErr = ErrCircuitOpen
			c.logger.Warn("circuit breaker open, skipping request",
				slog.String("url", req.URL.String()),
				slog.String("state", c.breaker.State().String()),
			)
			continue
		}

		// Execute request
		start := time.Now()
		resp, err := c.client.Do(req.WithContext(ctx))
		duration := time.Since(start)

		if err != nil {
			c.breaker.RecordFailure()
			lastErr = err
			c.logger.Warn("request failed",
				slog.String("url", req.URL.String()),
				slog.String("method", req.Method),
				slog.Duration("duration", duration),
				slog.String("error", err.Error()),
				slog.Int("attempt", attempt),
			)

			// Don't retry on context errors
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			continue
		}

		// Check for retryable status codes
		if isRetryableStatus(resp.StatusCode) {
			c.breaker.RecordFailure()
			lastErr = fmt.Errorf("retryable status code: %d", resp.StatusCode)
			c.logger.Warn("retryable status code",
				slog.String("url", req.URL.String()),
				slog.String("method", req.Method),
				slog.Int("status", resp.StatusCode),
				slog.Duration("duration", duration),
				slog.Int("attempt", attempt),
			)
			resp.Body.Close()
			continue
		}

		// Check if status code is acceptable for circuit breaker purposes
		if c.isAcceptableStatus(resp.StatusCode) {
			c.breaker.RecordSuccess()
		} else {
			// Non-acceptable status codes (e.g., 5xx errors) count as failures
			// but we don't retry them - just record the failure
			c.breaker.RecordFailure()
			c.logger.Debug("non-acceptable status code recorded as failure",
				slog.String("url", req.URL.String()),
				slog.Int("status", resp.StatusCode),
			)
		}
		c.logger.Debug("request completed",
			slog.String("url", req.URL.String()),
			slog.String("method", req.Method),
			slog.Int("status", resp.StatusCode),
			slog.Duration("duration", duration),
			slog.Int64("content_length", resp.ContentLength),
		)

		// Wrap response body with decompression if needed
		if c.config.EnableDecompression {
			resp.Body = c.wrapDecompression(resp)
		}

		// Apply max response size limit AFTER decompression
		// This protects against zip bombs where a small compressed payload
		// expands to a massive uncompressed size
		if c.config.MaxResponseSize > 0 {
			resp.Body = newLimitedReader(resp.Body, c.config.MaxResponseSize)
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrMaxRetries, lastErr)
	}
	return nil, ErrMaxRetries
}

// Get performs a GET request to the specified URL.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	return c.Do(req)
}

// CircuitState returns the current state of the circuit breaker.
func (c *Client) CircuitState() CircuitState {
	return c.breaker.State()
}

// ResetCircuit resets the circuit breaker to closed state.
func (c *Client) ResetCircuit() {
	c.breaker.Reset()
}

// StandardClient returns a standard *http.Client that uses this resilient client
// as its transport. This allows the resilient client to be used with any code
// that accepts a standard *http.Client.
func (c *Client) StandardClient() *http.Client {
	return &http.Client{
		Transport: &resilientTransport{client: c},
		Timeout:   c.config.Timeout,
	}
}

// resilientTransport implements http.RoundTripper using the resilient client.
type resilientTransport struct {
	client *Client
}

// RoundTrip implements http.RoundTripper.
func (t *resilientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.client.Do(req)
}

// Ensure resilientTransport implements http.RoundTripper.
var _ http.RoundTripper = (*resilientTransport)(nil)

// wrapDecompression wraps the response body with appropriate decompression.
func (c *Client) wrapDecompression(resp *http.Response) io.ReadCloser {
	encoding := resp.Header.Get(HeaderContentEncoding)
	if encoding == "" {
		return resp.Body
	}

	switch strings.ToLower(encoding) {
	case EncodingGzip:
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			c.logger.Warn("failed to create gzip reader, returning raw body",
				slog.String("error", err.Error()),
			)
			return resp.Body
		}
		return &decompressReader{reader: reader, closer: resp.Body}

	case EncodingDeflate:
		reader := flate.NewReader(resp.Body)
		return &decompressReader{reader: reader, closer: resp.Body}

	case EncodingBrotli:
		reader := brotli.NewReader(resp.Body)
		return &decompressReader{reader: reader, closer: resp.Body}

	default:
		c.logger.Debug("unknown content encoding, returning raw body",
			slog.String("encoding", encoding),
		)
		return resp.Body
	}
}

// decompressReader wraps a decompression reader with the original body closer.
type decompressReader struct {
	reader io.Reader
	closer io.Closer
}

func (d *decompressReader) Read(p []byte) (int, error) {
	return d.reader.Read(p)
}

func (d *decompressReader) Close() error {
	// Close the decompression reader if it implements io.Closer
	if closer, ok := d.reader.(io.Closer); ok {
		closer.Close()
	}
	return d.closer.Close()
}

// limitedReader wraps a reader with a maximum size limit.
// It returns ErrResponseTooLarge when the limit is exceeded.
type limitedReader struct {
	reader    io.Reader
	closer    io.Closer
	remaining int64
	exceeded  bool
}

func newLimitedReader(r io.ReadCloser, limit int64) *limitedReader {
	return &limitedReader{
		reader:    r,
		closer:    r,
		remaining: limit,
	}
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.exceeded {
		return 0, ErrResponseTooLarge
	}

	n, err := l.reader.Read(p)
	l.remaining -= int64(n)

	if l.remaining < 0 {
		l.exceeded = true
		return n, ErrResponseTooLarge
	}

	return n, err
}

func (l *limitedReader) Close() error {
	return l.closer.Close()
}

// isRetryableStatus returns true if the HTTP status code is retryable.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// isAcceptableStatus returns true if the HTTP status code should be considered
// "successful" for circuit breaker purposes.
//
// If AcceptableStatusCodes is configured (non-nil/non-empty), ONLY those codes are acceptable.
// This allows full control, including making 2xx codes unacceptable if needed.
//
// If AcceptableStatusCodes is nil/empty, defaults to accepting all 2xx status codes.
func (c *Client) isAcceptableStatus(code int) bool {
	// If explicitly configured, use only the configured codes
	if !c.config.AcceptableStatusCodes.IsEmpty() {
		return c.config.AcceptableStatusCodes.Contains(code)
	}

	// Default behavior: 2xx status codes are acceptable
	return code >= 200 && code < 300
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
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

// CircuitBreaker implements the circuit breaker pattern.
// It uses an atomic config pointer to allow runtime configuration updates
// without losing state (failure counts, etc.).
type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failures        int // consecutive failures
	successes       int // consecutive successes in half-open
	halfOpenCount   int
	lastFailureTime time.Time
	lastSuccessTime time.Time

	// Total counters (never reset, for stats/monitoring)
	totalRequests  int64
	totalSuccesses int64
	totalFailures  int64

	// Enhanced stats tracking
	errorCounts  ErrorCategoryCount
	stateTracker *StateTracker

	// config holds the circuit breaker configuration.
	// Use atomic operations via getConfig/setConfig for thread safety.
	configMu sync.RWMutex
	config   *CircuitBreakerProfileConfig
}

// NewCircuitBreaker creates a new circuit breaker with the given parameters.
// For runtime-configurable breakers, prefer NewCircuitBreakerWithConfig.
func NewCircuitBreaker(threshold int, timeout time.Duration, halfOpenMax int) *CircuitBreaker {
	cfg := &CircuitBreakerProfileConfig{
		FailureThreshold: threshold,
		ResetTimeout:     timeout,
		HalfOpenMax:      halfOpenMax,
	}
	return &CircuitBreaker{
		state:        CircuitClosed,
		config:       cfg,
		stateTracker: NewStateTracker(),
	}
}

// NewCircuitBreakerWithConfig creates a new circuit breaker with the given config.
// The config pointer can be updated at runtime via UpdateConfig.
func NewCircuitBreakerWithConfig(cfg *CircuitBreakerProfileConfig) *CircuitBreaker {
	if cfg == nil {
		defaultCfg := DefaultProfileConfig()
		cfg = &defaultCfg
	}
	return &CircuitBreaker{
		state:        CircuitClosed,
		config:       cfg,
		stateTracker: NewStateTracker(),
	}
}

// getConfig returns the current config safely.
func (cb *CircuitBreaker) getConfig() *CircuitBreakerProfileConfig {
	cb.configMu.RLock()
	defer cb.configMu.RUnlock()
	return cb.config
}

// UpdateConfig atomically updates the circuit breaker's configuration.
// The circuit breaker state (failures, successes, etc.) is preserved.
func (cb *CircuitBreaker) UpdateConfig(cfg *CircuitBreakerProfileConfig) {
	cb.configMu.Lock()
	defer cb.configMu.Unlock()
	cb.config = cfg
}

// Config returns a copy of the current configuration.
func (cb *CircuitBreaker) Config() CircuitBreakerProfileConfig {
	cfg := cb.getConfig()
	if cfg == nil {
		return DefaultProfileConfig()
	}
	return *cfg
}

// Allow returns true if the request should be allowed to proceed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cfg := cb.getConfig()
	if cfg == nil {
		return true // No config, allow all
	}

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailureTime) >= cfg.ResetTimeout {
			oldState := cb.state
			cb.state = CircuitHalfOpen
			cb.halfOpenCount = 1 // Count this first request
			// Record transition
			if cb.stateTracker != nil {
				cb.stateTracker.RecordTransition(oldState, cb.state, TransitionReasonTimeoutRecovery, cb.failures)
			}
			return true
		}
		return false

	case CircuitHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenCount < cfg.HalfOpenMax {
			cb.halfOpenCount++
			return true
		}
		return false

	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++
	cb.totalRequests++
	cb.totalSuccesses++
	cb.lastSuccessTime = time.Now()
	cb.errorCounts.Increment(ErrorCategorySuccess2xx)

	if cb.state == CircuitHalfOpen {
		// Reset to closed after success in half-open
		oldState := cb.state
		cb.state = CircuitClosed
		cb.failures = 0
		cb.successes = 0
		// Record transition
		if cb.stateTracker != nil {
			cb.stateTracker.RecordTransition(oldState, cb.state, TransitionReasonProbeSuccess, cb.successes)
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.RecordFailureWithCategory(ErrorCategoryServerError5xx)
}

// RecordFailureWithCategory records a failed request with a specific error category.
func (cb *CircuitBreaker) RecordFailureWithCategory(category ErrorCategory) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()
	cb.totalRequests++
	cb.totalFailures++
	cb.errorCounts.Increment(category)

	cfg := cb.getConfig()
	threshold := DefaultCircuitThreshold
	if cfg != nil && cfg.FailureThreshold > 0 {
		threshold = cfg.FailureThreshold
	}

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= threshold {
			oldState := cb.state
			cb.state = CircuitOpen
			// Record transition
			if cb.stateTracker != nil {
				cb.stateTracker.RecordTransition(oldState, cb.state, TransitionReasonThresholdExceeded, cb.failures)
			}
		}

	case CircuitHalfOpen:
		// Any failure in half-open returns to open
		oldState := cb.state
		cb.state = CircuitOpen
		// Record transition
		if cb.stateTracker != nil {
			cb.stateTracker.RecordTransition(oldState, cb.state, TransitionReasonProbeFailure, cb.failures)
		}
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCount = 0

	// Record transition if state changed
	if oldState != CircuitClosed && cb.stateTracker != nil {
		cb.stateTracker.RecordTransition(oldState, cb.state, TransitionReasonManualReset, 0)
	}
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Successes returns the current success count.
func (cb *CircuitBreaker) Successes() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.successes
}

// CircuitBreakerStats holds statistics about a circuit breaker.
type CircuitBreakerStats struct {
	State               CircuitState                `json:"state"`
	Failures            int                         `json:"failures"`             // consecutive failures
	Successes           int                         `json:"successes"`            // consecutive successes in half-open
	ConsecutiveFailures int                         `json:"consecutive_failures"` // same as Failures (for clarity)
	TotalRequests       int64                       `json:"total_requests"`
	TotalSuccesses      int64                       `json:"total_successes"`
	TotalFailures       int64                       `json:"total_failures"`
	LastFailure         time.Time                   `json:"last_failure,omitempty"`
	Config              CircuitBreakerProfileConfig `json:"config"`
}

// Stats returns current statistics for this circuit breaker.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:               cb.state,
		Failures:            cb.failures,
		Successes:           cb.successes,
		ConsecutiveFailures: cb.failures,
		TotalRequests:       cb.totalRequests,
		TotalSuccesses:      cb.totalSuccesses,
		TotalFailures:       cb.totalFailures,
		LastFailure:         cb.lastFailureTime,
		Config:              cb.Config(),
	}
}

// EnhancedStats returns enhanced statistics for this circuit breaker.
func (cb *CircuitBreaker) EnhancedStats(name string) EnhancedCircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	cfg := cb.Config()

	stats := EnhancedCircuitBreakerStats{
		Name:                 name,
		State:                cb.state,
		ConsecutiveFailures:  cb.failures,
		ConsecutiveSuccesses: cb.successes,
		TotalRequests:        cb.totalRequests,
		TotalSuccesses:       cb.totalSuccesses,
		TotalFailures:        cb.totalFailures,
		LastFailure:          cb.lastFailureTime,
		LastSuccess:          cb.lastSuccessTime,
		Config:               cfg,
		ErrorCounts:          cb.errorCounts.Clone(),
	}

	// Calculate failure rate
	if stats.TotalRequests > 0 {
		stats.FailureRate = float64(stats.TotalFailures) / float64(stats.TotalRequests) * 100
	}

	// Get state tracking info
	if cb.stateTracker != nil {
		stats.StateEnteredAt = cb.stateTracker.GetStateEnteredAt()
		stats.StateDurationMs = cb.stateTracker.GetStateDurationMs()
		stats.StateDurations = cb.stateTracker.GetDurationSummary()
		stats.Transitions = cb.stateTracker.GetTransitions()
	}

	// Calculate next half-open time when circuit is open
	if cb.state == CircuitOpen && !cb.lastFailureTime.IsZero() {
		stats.NextHalfOpenAt = cb.lastFailureTime.Add(cfg.ResetTimeout)
	}

	return stats
}
