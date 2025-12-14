// Package observability provides logging, tracing, and metrics for tvarr.
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/m-mizutani/masq"
)

// urlSensitiveParamPattern matches sensitive query parameters in URLs.
// Matches: password=value, secret=value, token=value, apikey=value, api_key=value, credential=value
// Case-insensitive, captures until next & or end of query string.
var urlSensitiveParamPattern = regexp.MustCompile(`(?i)(password|secret|token|apikey|api_key|credential)=([^&\s"']+)`)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// RequestIDKey is the context key for request IDs.
	RequestIDKey contextKey = "request_id"
	// CorrelationIDKey is the context key for correlation IDs.
	CorrelationIDKey contextKey = "correlation_id"
)

// GlobalLogLevel is the shared log level that can be changed at runtime.
// Use SetLogLevel and GetLogLevel to modify/read this value.
var GlobalLogLevel = &slog.LevelVar{}

// enableRequestLogging controls whether HTTP requests are logged.
var enableRequestLogging atomic.Bool

// NewLogger creates a new slog.Logger based on the provided configuration.
// The logger supports JSON and text formats with configurable log levels.
func NewLogger(cfg config.LoggingConfig) *slog.Logger {
	return NewLoggerWithWriter(cfg, os.Stdout)
}

// sensitiveFieldRedactor creates a masq redactor for sensitive field names.
// This redacts passwords, secrets, tokens, API keys, and credentials from logs.
func sensitiveFieldRedactor() func(groups []string, a slog.Attr) slog.Attr {
	return masq.New(
		masq.WithFieldName("password"),
		masq.WithFieldName("Password"),
		masq.WithFieldName("secret"),
		masq.WithFieldName("Secret"),
		masq.WithFieldName("token"),
		masq.WithFieldName("Token"),
		masq.WithFieldName("apikey"),
		masq.WithFieldName("ApiKey"),
		masq.WithFieldName("api_key"),
		masq.WithFieldName("credential"),
		masq.WithFieldName("Credential"),
	)
}

// redactURLParams redacts sensitive query parameters from URL strings.
// This handles cases where passwords appear in URL query strings like:
// http://example.com/api?username=foo&password=secret123
func redactURLParams(s string) string {
	return urlSensitiveParamPattern.ReplaceAllString(s, "$1=[REDACTED]")
}

// NewLoggerWithWriter creates a new slog.Logger that writes to the provided writer.
// This is useful for testing or custom output destinations.
// The logger uses GlobalLogLevel for dynamic log level changes at runtime.
// Sensitive fields (password, secret, token, apikey, credential) are automatically redacted.
func NewLoggerWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	// Set the initial level from config
	level := parseLevel(cfg.Level)
	GlobalLogLevel.Set(level)

	// Create the sensitive data redactor
	redactor := sensitiveFieldRedactor()

	opts := &slog.HandlerOptions{
		Level:     GlobalLogLevel, // Use the global LevelVar for dynamic changes
		AddSource: cfg.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// First apply sensitive data redaction (field-name based)
			a = redactor(groups, a)

			// Then redact sensitive URL query parameters in string values
			if a.Value.Kind() == slog.KindString {
				str := a.Value.String()
				redacted := redactURLParams(str)
				if redacted != str {
					a = slog.String(a.Key, redacted)
				}
			}

			// Finally customize time format if specified
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
	case "trace":
		return slog.LevelDebug - 4 // slog doesn't have trace, use lower than debug
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

// SetLogLevel changes the global log level at runtime.
// Valid levels: "trace", "debug", "info", "warn", "error"
func SetLogLevel(level string) {
	GlobalLogLevel.Set(parseLevel(level))
}

// GetLogLevel returns the current log level as a string.
func GetLogLevel() string {
	level := GlobalLogLevel.Level()
	switch {
	case level < slog.LevelDebug:
		return "trace"
	case level == slog.LevelDebug:
		return "debug"
	case level == slog.LevelInfo:
		return "info"
	case level == slog.LevelWarn:
		return "warn"
	case level >= slog.LevelError:
		return "error"
	default:
		return "info"
	}
}

// SetRequestLogging enables or disables HTTP request logging.
func SetRequestLogging(enabled bool) {
	enableRequestLogging.Store(enabled)
}

// IsRequestLoggingEnabled returns whether HTTP request logging is enabled.
func IsRequestLoggingEnabled() bool {
	return enableRequestLogging.Load()
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
