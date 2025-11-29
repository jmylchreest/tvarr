// Package observability provides logging, tracing, and metrics for tvarr.
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// RequestIDKey is the context key for request IDs.
	RequestIDKey contextKey = "request_id"
	// CorrelationIDKey is the context key for correlation IDs.
	CorrelationIDKey contextKey = "correlation_id"
)

// NewLogger creates a new slog.Logger based on the provided configuration.
// The logger supports JSON and text formats with configurable log levels.
func NewLogger(cfg config.LoggingConfig) *slog.Logger {
	return NewLoggerWithWriter(cfg, os.Stdout)
}

// NewLoggerWithWriter creates a new slog.Logger that writes to the provided writer.
// This is useful for testing or custom output destinations.
func NewLoggerWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// Customize time format if specified
			if a.Key == slog.TimeKey && cfg.TimeFormat != "" {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String(slog.TimeKey, t.Format(cfg.TimeFormat))
				}
			}
			return a
		},
	}

	var handler slog.Handler
	switch cfg.Format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		// Default to JSON if format is unknown
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}

// parseLevel converts a string log level to slog.Level.
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithRequestID adds a request ID to the logger.
func WithRequestID(logger *slog.Logger, requestID string) *slog.Logger {
	return logger.With(slog.String("request_id", requestID))
}

// WithCorrelationID adds a correlation ID to the logger.
func WithCorrelationID(logger *slog.Logger, correlationID string) *slog.Logger {
	return logger.With(slog.String("correlation_id", correlationID))
}

// WithComponent adds a component name to the logger for identifying the source.
func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	return logger.With(slog.String("component", component))
}

// WithOperation adds an operation name to the logger for tracking specific operations.
func WithOperation(logger *slog.Logger, operation string) *slog.Logger {
	return logger.With(slog.String("operation", operation))
}

// WithError adds an error to the logger attributes.
func WithError(logger *slog.Logger, err error) *slog.Logger {
	if err == nil {
		return logger
	}
	return logger.With(slog.String("error", err.Error()))
}

// LoggerFromContext extracts a logger from the context.
// If no logger is found, returns the default logger.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// ContextWithLogger adds a logger to the context.
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// loggerKey is the context key for the logger.
const loggerKey contextKey = "logger"

// RequestIDFromContext extracts a request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// ContextWithRequestID adds a request ID to the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// CorrelationIDFromContext extracts a correlation ID from the context.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// ContextWithCorrelationID adds a correlation ID to the context.
func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// SetDefault sets the provided logger as the default slog logger.
// This affects all code using slog.Info(), slog.Error(), etc. without a specific logger.
func SetDefault(logger *slog.Logger) {
	slog.SetDefault(logger)
}

// LogAttrs is a convenience function for logging with attributes at different levels.
type LogAttrs struct {
	logger *slog.Logger
}

// NewLogAttrs creates a new LogAttrs helper.
func NewLogAttrs(logger *slog.Logger) *LogAttrs {
	return &LogAttrs{logger: logger}
}

// Info logs an info message with the given attributes.
func (l *LogAttrs) Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// Debug logs a debug message with the given attributes.
func (l *LogAttrs) Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Warn logs a warning message with the given attributes.
func (l *LogAttrs) Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Error logs an error message with the given attributes.
func (l *LogAttrs) Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

// TimedOperation logs the start and end of an operation with duration.
// Returns a function that should be deferred to log the completion.
//
// Usage:
//
//	done := logger.TimedOperation(ctx, "process_channels")
//	defer done()
func TimedOperation(ctx context.Context, logger *slog.Logger, operation string) func() {
	start := time.Now()
	logger.InfoContext(ctx, "operation started", slog.String("operation", operation))

	return func() {
		duration := time.Since(start)
		logger.InfoContext(ctx, "operation completed",
			slog.String("operation", operation),
			slog.Duration("duration", duration),
		)
	}
}

// TimedOperationWithError is like TimedOperation but accepts an error pointer
// to determine success/failure status. The error pointer is required because
// the error value may be set after calling this function but before the
// returned done function is called.
//
// Usage:
//
//	var err error
//	done := logger.TimedOperationWithError(ctx, "process_channels", &err)
//	defer done()
//	err = doSomething()
//
//nolint:gocritic // errPtr must be a pointer to capture errors set after this call
func TimedOperationWithError(ctx context.Context, logger *slog.Logger, operation string, errPtr *error) func() {
	start := time.Now()
	logger.InfoContext(ctx, "operation started", slog.String("operation", operation))

	return func() {
		duration := time.Since(start)
		if errPtr != nil && *errPtr != nil {
			logger.ErrorContext(ctx, "operation failed",
				slog.String("operation", operation),
				slog.Duration("duration", duration),
				slog.String("error", (*errPtr).Error()),
			)
		} else {
			logger.InfoContext(ctx, "operation completed",
				slog.String("operation", operation),
				slog.Duration("duration", duration),
			)
		}
	}
}
