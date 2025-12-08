package httpclient

import (
	"log/slog"
	"time"
)

// ClientFactory creates HTTP clients with appropriate circuit breaker protection
// based on service names. This decouples services from circuit breaker management.
type ClientFactory struct {
	manager       *CircuitBreakerManager
	defaultConfig Config
	logger        *slog.Logger
}

// NewClientFactory creates a new client factory.
// If manager is nil, uses the DefaultManager.
func NewClientFactory(manager *CircuitBreakerManager) *ClientFactory {
	if manager == nil {
		manager = DefaultManager
	}

	return &ClientFactory{
		manager:       manager,
		defaultConfig: DefaultConfig(),
		logger:        slog.Default(),
	}
}

// WithDefaultConfig sets the default client config used when creating clients.
func (f *ClientFactory) WithDefaultConfig(cfg Config) *ClientFactory {
	f.defaultConfig = cfg
	return f
}

// WithLogger sets the logger for the factory.
func (f *ClientFactory) WithLogger(logger *slog.Logger) *ClientFactory {
	f.logger = logger
	f.defaultConfig.Logger = logger
	return f
}

// CreateClientForService creates an HTTP client for a specific service.
// The client uses a circuit breaker from the manager, allowing shared state
// and runtime configuration updates.
//
// Common service names:
//   - "source_m3u" - M3U source fetching
//   - "source_xmltv" - XMLTV EPG source fetching
//   - "source_xc_stream" - Xtream codes stream fetching
//   - "source_xc_xmltv" - Xtream codes XMLTV fetching
//   - "logo_fetch" - Logo downloading
//   - "relay" - Stream relay/proxy
func (f *ClientFactory) CreateClientForService(serviceName string) *Client {
	// Get or create circuit breaker for this service
	breaker := f.manager.GetOrCreate(serviceName)

	// Get the service's effective config for acceptable status codes
	cbConfig := f.manager.GetServiceConfig(serviceName)

	// Create client config
	cfg := f.defaultConfig
	cfg.AcceptableStatusCodes = cbConfig.AcceptableStatusCodes

	// Create client with the shared breaker
	client := NewWithBreaker(cfg, breaker)

	f.logger.Debug("created HTTP client for service",
		slog.String("service", serviceName),
		slog.String("circuit_state", breaker.State().String()),
	)

	return client
}

// CreateBasicClient creates an HTTP client without circuit breaker protection.
// Use this for internal services or when circuit breaker protection isn't needed.
func (f *ClientFactory) CreateBasicClient() *Client {
	return New(f.defaultConfig)
}

// CreateClientWithConfig creates an HTTP client with custom config and circuit breaker
// from the manager for the given service name.
func (f *ClientFactory) CreateClientWithConfig(serviceName string, cfg Config) *Client {
	breaker := f.manager.GetOrCreate(serviceName)

	// Override acceptable status codes from circuit breaker config if not set
	if cfg.AcceptableStatusCodes == nil {
		cbConfig := f.manager.GetServiceConfig(serviceName)
		cfg.AcceptableStatusCodes = cbConfig.AcceptableStatusCodes
	}

	return NewWithBreaker(cfg, breaker)
}

// Manager returns the underlying circuit breaker manager.
func (f *ClientFactory) Manager() *CircuitBreakerManager {
	return f.manager
}

// ClientConfig holds configuration for creating a Client with options.
type ClientConfig struct {
	// ServiceName is used to get the appropriate circuit breaker.
	ServiceName string

	// Timeout overrides the default timeout.
	Timeout time.Duration

	// RetryAttempts overrides the default retry attempts.
	RetryAttempts int

	// MaxResponseSize sets a limit on response body size.
	MaxResponseSize int64

	// EnableDecompression enables automatic response decompression.
	EnableDecompression *bool

	// UserAgent sets a custom user agent.
	UserAgent string
}

// CreateClient creates an HTTP client with the given options.
func (f *ClientFactory) CreateClient(opts ClientConfig) *Client {
	cfg := f.defaultConfig

	if opts.Timeout > 0 {
		cfg.Timeout = opts.Timeout
	}
	if opts.RetryAttempts > 0 {
		cfg.RetryAttempts = opts.RetryAttempts
	}
	if opts.MaxResponseSize > 0 {
		cfg.MaxResponseSize = opts.MaxResponseSize
	}
	if opts.EnableDecompression != nil {
		cfg.EnableDecompression = *opts.EnableDecompression
	}
	if opts.UserAgent != "" {
		cfg.UserAgent = opts.UserAgent
	}

	if opts.ServiceName != "" {
		breaker := f.manager.GetOrCreate(opts.ServiceName)
		cbConfig := f.manager.GetServiceConfig(opts.ServiceName)
		if cfg.AcceptableStatusCodes == nil {
			cfg.AcceptableStatusCodes = cbConfig.AcceptableStatusCodes
		}
		return NewWithBreaker(cfg, breaker)
	}

	return New(cfg)
}

// DefaultFactory is a convenience factory using the default manager.
var DefaultFactory = NewClientFactory(nil)
