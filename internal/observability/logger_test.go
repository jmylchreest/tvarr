package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
	}

	logger := NewLoggerWithWriter(cfg, &buf)
	logger.Info("test message", slog.String("key", "value"))

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, `"key":"value"`)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err)
}

func TestNewLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "text",
	}

	logger := NewLoggerWithWriter(cfg, &buf)
	logger.Info("test message", slog.String("key", "value"))

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "key=value")
}

func TestNewLogger_Levels(t *testing.T) {
	tests := []struct {
		name        string
		configLevel string
		logLevel    slog.Level
		shouldLog   bool
	}{
		{"debug logs at debug level", "debug", slog.LevelDebug, true},
		{"debug logs at info level", "debug", slog.LevelInfo, true},
		{"info does not log debug", "info", slog.LevelDebug, false},
		{"info logs at info level", "info", slog.LevelInfo, true},
		{"warn does not log info", "warn", slog.LevelInfo, false},
		{"warn logs at warn level", "warn", slog.LevelWarn, true},
		{"error does not log warn", "error", slog.LevelWarn, false},
		{"error logs at error level", "error", slog.LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := config.LoggingConfig{
				Level:  tt.configLevel,
				Format: "json",
			}

			logger := NewLoggerWithWriter(cfg, &buf)
			logger.Log(context.Background(), tt.logLevel, "test")

			if tt.shouldLog {
				assert.NotEmpty(t, buf.String())
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}
}

func TestNewLogger_AddSource(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{
		Level:     "info",
		Format:    "json",
		AddSource: true,
	}

	logger := NewLoggerWithWriter(cfg, &buf)
	logger.Info("test message")

	output := buf.String()
	// Source adds "source" field with file and line info
	assert.Contains(t, output, "source")
}

func TestNewLogger_CustomTimeFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{
		Level:      "info",
		Format:     "json",
		TimeFormat: "2006-01-02",
	}

	logger := NewLoggerWithWriter(cfg, &buf)
	logger.Info("test message")

	output := buf.String()
	// Should contain date in YYYY-MM-DD format
	today := time.Now().Format("2006-01-02")
	assert.Contains(t, output, today)
}

func TestWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	loggerWithID := WithRequestID(logger, "req-123")
	loggerWithID.Info("test")

	assert.Contains(t, buf.String(), `"request_id":"req-123"`)
}

func TestWithCorrelationID(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	loggerWithID := WithCorrelationID(logger, "corr-456")
	loggerWithID.Info("test")

	assert.Contains(t, buf.String(), `"correlation_id":"corr-456"`)
}

func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	loggerWithComp := WithComponent(logger, "ingestor")
	loggerWithComp.Info("test")

	assert.Contains(t, buf.String(), `"component":"ingestor"`)
}

func TestWithOperation(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	loggerWithOp := WithOperation(logger, "fetch_channels")
	loggerWithOp.Info("test")

	assert.Contains(t, buf.String(), `"operation":"fetch_channels"`)
}

func TestWithError(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	loggerWithErr := WithError(logger, errors.New("something went wrong"))
	loggerWithErr.Info("test")

	assert.Contains(t, buf.String(), `"error":"something went wrong"`)
}

func TestWithError_Nil(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	loggerWithErr := WithError(logger, nil)
	loggerWithErr.Info("test")

	// Should not contain error field when error is nil
	assert.NotContains(t, buf.String(), `"error"`)
}

func TestContextWithLogger(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	ctx := ContextWithLogger(context.Background(), logger)
	extractedLogger := LoggerFromContext(ctx)

	extractedLogger.Info("from context")
	assert.Contains(t, buf.String(), "from context")
}

func TestLoggerFromContext_Default(t *testing.T) {
	// When no logger in context, should return default
	ctx := context.Background()
	logger := LoggerFromContext(ctx)
	assert.NotNil(t, logger)
}

func TestContextWithRequestID(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-789")
	id := RequestIDFromContext(ctx)
	assert.Equal(t, "req-789", id)
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	id := RequestIDFromContext(ctx)
	assert.Empty(t, id)
}

func TestContextWithCorrelationID(t *testing.T) {
	ctx := ContextWithCorrelationID(context.Background(), "corr-abc")
	id := CorrelationIDFromContext(ctx)
	assert.Equal(t, "corr-abc", id)
}

func TestCorrelationIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	id := CorrelationIDFromContext(ctx)
	assert.Empty(t, id)
}

func TestLogAttrs(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "debug", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	la := NewLogAttrs(logger)
	ctx := context.Background()

	// Test Info
	la.Info(ctx, "info message", slog.Int("count", 42))
	assert.Contains(t, buf.String(), "info message")
	assert.Contains(t, buf.String(), `"count":42`)

	buf.Reset()

	// Test Debug
	la.Debug(ctx, "debug message")
	assert.Contains(t, buf.String(), "debug message")

	buf.Reset()

	// Test Warn
	la.Warn(ctx, "warn message")
	assert.Contains(t, buf.String(), "warn message")

	buf.Reset()

	// Test Error
	la.Error(ctx, "error message")
	assert.Contains(t, buf.String(), "error message")
}

func TestTimedOperation(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	ctx := context.Background()
	done := TimedOperation(ctx, logger, "test_operation")

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	done()

	output := buf.String()
	// Should have start log
	assert.True(t, strings.Contains(output, "operation started"))
	// Should have completion log
	assert.True(t, strings.Contains(output, "operation completed"))
	// Should have operation name
	assert.Contains(t, output, "test_operation")
	// Should have duration
	assert.Contains(t, output, "duration")
}

func TestTimedOperationWithError_Success(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	ctx := context.Background()
	var err error
	done := TimedOperationWithError(ctx, logger, "success_op", &err)

	// No error
	done()

	output := buf.String()
	assert.Contains(t, output, "operation completed")
	assert.NotContains(t, output, "operation failed")
}

func TestTimedOperationWithError_Failure(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	ctx := context.Background()
	var err error
	done := TimedOperationWithError(ctx, logger, "failure_op", &err)

	// Set error before done
	err = errors.New("operation failed")
	done()

	output := buf.String()
	assert.Contains(t, output, "operation failed")
	assert.Contains(t, output, "operation failed") // error message
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
		{"", slog.LevelInfo},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLevel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChainedWith(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	// Chain multiple With functions
	enrichedLogger := WithComponent(
		WithRequestID(
			WithOperation(logger, "process"),
			"req-chain",
		),
		"service",
	)

	enrichedLogger.Info("chained test")

	output := buf.String()
	assert.Contains(t, output, `"operation":"process"`)
	assert.Contains(t, output, `"request_id":"req-chain"`)
	assert.Contains(t, output, `"component":"service"`)
}
