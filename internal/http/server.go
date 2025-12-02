// Package http provides the HTTP server and API handlers for tvarr.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jmylchreest/tvarr/internal/http/middleware"
)

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	// Host is the address to bind to (default: "0.0.0.0").
	Host string
	// Port is the port to listen on (default: 8080).
	Port int
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration
	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout time.Duration
	// ShutdownTimeout is the maximum duration to wait for active connections to close.
	ShutdownTimeout time.Duration
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Host:            "0.0.0.0",
		Port:            8080,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		IdleTimeout:     120 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
}

// Server represents the HTTP server.
type Server struct {
	config     ServerConfig
	router     *chi.Mux
	api        huma.API
	httpServer *http.Server
	logger     *slog.Logger
}

// NewServer creates a new HTTP server with the given configuration.
// The version parameter is used in the OpenAPI spec and should match the build version.
func NewServer(config ServerConfig, logger *slog.Logger, version string) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if version == "" {
		version = "dev"
	}

	router := chi.NewRouter()

	// Apply middleware
	router.Use(chimiddleware.RealIP)
	router.Use(middleware.RequestID)
	router.Use(middleware.NewLoggingMiddleware(logger))
	router.Use(middleware.Recovery(logger))
	router.Use(middleware.CORS())

	// Configure compression middleware with SSE exclusion.
	// SSE (text/event-stream) requires unbuffered streaming; compression interferes with flushing.
	router.Use(middleware.SkipCompressionForSSE(chimiddleware.Compress(5)))

	// Create Huma API with custom config
	// Note: DocsPath is left empty - we use our own docs handler with dark theme support
	humaConfig := huma.DefaultConfig("tvarr API", version)
	humaConfig.Info.Description = "M3U/XMLTV proxy and EPG management API"
	humaConfig.DocsPath = "" // Disabled - using custom DocsHandler

	api := humachi.New(router, humaConfig)

	return &Server{
		config: config,
		router: router,
		api:    api,
		logger: logger,
	}
}

// API returns the Huma API instance for registering operations.
func (s *Server) API() huma.API {
	return s.api
}

// Router returns the Chi router for registering additional routes.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.logger.Info("starting HTTP server",
		slog.String("address", addr),
	)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("starting server: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	s.logger.Info("shutting down HTTP server",
		slog.Duration("timeout", s.config.ShutdownTimeout),
	)

	// Create a context with timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down server: %w", err)
	}

	s.logger.Info("HTTP server stopped")
	return nil
}

// ListenAndServe starts the server and handles graceful shutdown.
// It blocks until the server is shut down.
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Start()
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errChan:
		return err
	}
}
