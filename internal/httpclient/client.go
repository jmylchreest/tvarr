// Package httpclient provides a resilient HTTP client with circuit breaker,
// automatic retries, transparent decompression, and structured logging.
//
// The client wraps the standard http.Client and adds production-grade features:
//   - Circuit breaker to prevent cascading failures
//   - Automatic retries with exponential backoff
//   - Transparent decompression (gzip, deflate, brotli)
//   - Structured logging with credential obfuscation
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
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

// Common errors returned by the client.
var (
	ErrCircuitOpen    = errors.New("circuit breaker is open")
	ErrMaxRetries     = errors.New("max retries exceeded")
	ErrRequestTimeout = errors.New("request timeout")
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
				slog.String("url", obfuscateURL(req.URL)),
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
				slog.String("url", obfuscateURL(req.URL)),
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
				slog.String("url", obfuscateURL(req.URL)),
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
				slog.String("url", obfuscateURL(req.URL)),
				slog.String("method", req.Method),
				slog.Int("status", resp.StatusCode),
				slog.Duration("duration", duration),
				slog.Int("attempt", attempt),
			)
			resp.Body.Close()
			continue
		}

		// Success
		c.breaker.RecordSuccess()
		c.logger.Debug("request completed",
			slog.String("url", obfuscateURL(req.URL)),
			slog.String("method", req.Method),
			slog.Int("status", resp.StatusCode),
			slog.Duration("duration", duration),
			slog.Int64("content_length", resp.ContentLength),
		)

		// Wrap response body with decompression if needed
		if c.config.EnableDecompression {
			resp.Body = c.wrapDecompression(resp)
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

// obfuscateURL returns a URL string with sensitive query parameters obfuscated.
func obfuscateURL(u *url.URL) string {
	if u == nil {
		return ""
	}

	// Make a copy to avoid modifying the original
	sanitized := *u
	query := sanitized.Query()

	// List of sensitive parameter names to obfuscate
	sensitiveParams := []string{
		"password", "passwd", "pass", "pwd",
		"token", "api_key", "apikey", "key",
		"secret", "auth", "authorization",
		"credential", "credentials",
	}

	for _, param := range sensitiveParams {
		if query.Has(param) {
			query.Set(param, "***")
		}
	}

	sanitized.RawQuery = query.Encode()
	return sanitized.String()
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
type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failures        int
	successes       int
	threshold       int
	timeout         time.Duration
	halfOpenMax     int
	halfOpenCount   int
	lastFailureTime time.Time
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(threshold int, timeout time.Duration, halfOpenMax int) *CircuitBreaker {
	return &CircuitBreaker{
		state:       CircuitClosed,
		threshold:   threshold,
		timeout:     timeout,
		halfOpenMax: halfOpenMax,
	}
}

// Allow returns true if the request should be allowed to proceed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailureTime) >= cb.timeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenCount = 1 // Count this first request
			return true
		}
		return false

	case CircuitHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenCount < cb.halfOpenMax {
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

	if cb.state == CircuitHalfOpen {
		// Reset to closed after success in half-open
		cb.state = CircuitClosed
		cb.failures = 0
		cb.successes = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.threshold {
			cb.state = CircuitOpen
		}

	case CircuitHalfOpen:
		// Any failure in half-open returns to open
		cb.state = CircuitOpen
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

	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCount = 0
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}
