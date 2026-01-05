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
	var parsed map[string]any
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
	// Source adds "logpos" field with relative file path and line number
	assert.Contains(t, output, "logpos")
	assert.Contains(t, output, "internal/observability/logger_test.go:")
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
		{"trace", LevelTrace},
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

func TestTraceLevelDisplay(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "trace", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	// Log at trace level
	logger.Log(context.Background(), LevelTrace, "trace message")

	output := buf.String()
	// Should contain the message
	assert.Contains(t, output, "trace message")
	// Should display level as "TRACE" not "DEBUG-4"
	assert.Contains(t, output, `"level":"TRACE"`)
	assert.NotContains(t, output, "DEBUG-4")
}

func TestTraceLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		configLevel string
		shouldLog   bool
	}{
		{"trace logs at trace level", "trace", true},
		{"trace logs at debug level", "debug", false},
		{"trace logs at info level", "info", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := config.LoggingConfig{Level: tt.configLevel, Format: "json"}
			logger := NewLoggerWithWriter(cfg, &buf)

			logger.Log(context.Background(), LevelTrace, "trace test")

			if tt.shouldLog {
				assert.NotEmpty(t, buf.String())
				assert.Contains(t, buf.String(), "trace test")
			} else {
				assert.Empty(t, buf.String())
			}
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

func TestSensitiveDataRedaction(t *testing.T) {
	tests := []struct {
		name          string
		fieldName     string
		sensitiveData string
	}{
		{"password lowercase", "password", "secret123"},
		{"password capitalized", "Password", "MyP@ssw0rd"},
		{"secret lowercase", "secret", "topsecret"},
		{"secret capitalized", "Secret", "TopSecret"},
		{"token lowercase", "token", "jwt-token-abc"},
		{"token capitalized", "Token", "Bearer xyz"},
		{"apikey lowercase", "apikey", "ak_12345"},
		{"apikey capitalized", "ApiKey", "AK_67890"},
		{"api_key snake case", "api_key", "api-key-value"},
		{"credential lowercase", "credential", "cred-abc"},
		{"credential capitalized", "Credential", "CRED-XYZ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := config.LoggingConfig{Level: "info", Format: "json"}
			logger := NewLoggerWithWriter(cfg, &buf)

			// Log with sensitive field
			logger.Info("test message", slog.String(tt.fieldName, tt.sensitiveData))

			output := buf.String()
			// Should NOT contain the actual sensitive data
			assert.NotContains(t, output, tt.sensitiveData,
				"sensitive data should be redacted for field %s", tt.fieldName)
			// Should contain a redaction marker
			assert.Contains(t, output, "[REDACTED]",
				"should contain redaction marker for field %s", tt.fieldName)
		})
	}
}

func TestSensitiveDataRedaction_NestedStruct(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	// Test with slog.Group containing sensitive data
	logger.Info("test with group",
		slog.Group("credentials",
			slog.String("username", "admin"),
			slog.String("password", "secret123"),
		),
	)

	output := buf.String()
	// Username should be visible
	assert.Contains(t, output, "admin")
	// Password should be redacted
	assert.NotContains(t, output, "secret123")
	assert.Contains(t, output, "[REDACTED]")
}

func TestNonSensitiveDataNotRedacted(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	// Log with non-sensitive fields
	logger.Info("test message",
		slog.String("username", "john"),
		slog.String("url", "http://example.com"),
		slog.Int("count", 42),
	)

	output := buf.String()
	// Should contain all non-sensitive data
	assert.Contains(t, output, "john")
	assert.Contains(t, output, "http://example.com")
	assert.Contains(t, output, "42")
}

func TestURLParameterRedaction(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		sensitiveValue string
		paramName      string
	}{
		{
			name:           "password in URL query",
			url:            "http://example.com/api?username=user&password=secret123&action=login",
			sensitiveValue: "secret123",
			paramName:      "password",
		},
		{
			name:           "password URL encoded",
			url:            "http://example.com/api?password=%2A%2A%2A&username=foo",
			sensitiveValue: "%2A%2A%2A",
			paramName:      "password",
		},
		{
			name:           "token in URL query",
			url:            "http://api.example.com/v1?token=abc123xyz&user=admin",
			sensitiveValue: "abc123xyz",
			paramName:      "token",
		},
		{
			name:           "apikey in URL query",
			url:            "http://api.example.com/data?apikey=sk_live_12345&format=json",
			sensitiveValue: "sk_live_12345",
			paramName:      "apikey",
		},
		{
			name:           "api_key snake case",
			url:            "http://example.com?api_key=my-secret-key&v=1",
			sensitiveValue: "my-secret-key",
			paramName:      "api_key",
		},
		{
			name:           "secret in URL query",
			url:            "http://example.com/webhook?secret=webhook_secret_value",
			sensitiveValue: "webhook_secret_value",
			paramName:      "secret",
		},
		{
			name:           "credential in URL query",
			url:            "http://example.com/auth?credential=cred_abc123",
			sensitiveValue: "cred_abc123",
			paramName:      "credential",
		},
		{
			name:           "case insensitive PASSWORD",
			url:            "http://example.com/api?PASSWORD=MySecret&user=test",
			sensitiveValue: "MySecret",
			paramName:      "PASSWORD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := config.LoggingConfig{Level: "info", Format: "json"}
			logger := NewLoggerWithWriter(cfg, &buf)

			// Log with URL containing sensitive parameter
			logger.Info("request completed", slog.String("url", tt.url))

			output := buf.String()
			// Should NOT contain the actual sensitive value
			assert.NotContains(t, output, tt.sensitiveValue,
				"URL should have %s value redacted", tt.paramName)
			// Should contain [REDACTED] marker
			assert.Contains(t, output, "[REDACTED]",
				"should contain redaction marker for %s parameter", tt.paramName)
			// Should still contain the parameter name
			assert.Contains(t, output, tt.paramName+"=[REDACTED]",
				"should show parameter name with redacted value")
		})
	}
}

func TestURLParameterRedaction_MultipleParams(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	// URL with multiple sensitive parameters
	url := "http://example.com/api?username=admin&password=secret123&token=bearer_xyz&apikey=ak_test"
	logger.Info("request", slog.String("url", url))

	output := buf.String()
	// None of the sensitive values should appear
	assert.NotContains(t, output, "secret123")
	assert.NotContains(t, output, "bearer_xyz")
	assert.NotContains(t, output, "ak_test")
	// Username should be preserved (not sensitive)
	assert.Contains(t, output, "admin")
}

func TestURLParameterRedaction_PreservesNonSensitiveURL(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	logger := NewLoggerWithWriter(cfg, &buf)

	// URL without sensitive parameters
	url := "http://example.com/api?username=john&action=get_data&format=json&page=1"
	logger.Info("request", slog.String("url", url))

	output := buf.String()
	// All non-sensitive params should be preserved
	assert.Contains(t, output, "username=john")
	assert.Contains(t, output, "action=get_data")
	assert.Contains(t, output, "format=json")
	assert.Contains(t, output, "page=1")
	// No redaction should occur
	assert.NotContains(t, output, "[REDACTED]")
}
