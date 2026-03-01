package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSQLiteError simulates a modernc.org/sqlite error with a code.
type mockSQLiteError struct {
	code int
	msg  string
}

func (e *mockSQLiteError) Error() string { return e.msg }
func (e *mockSQLiteError) Code() int     { return e.code }

func TestIsTransientSQLiteError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-sqlite error", errors.New("some error"), false},
		{"SQLITE_BUSY (5)", &mockSQLiteError{code: sqliteBusy, msg: "database is locked"}, true},
		{"SQLITE_LOCKED (6)", &mockSQLiteError{code: sqliteLocked, msg: "table is locked"}, true},
		{"SQLITE_INTERRUPT (9)", &mockSQLiteError{code: sqliteInterrupt, msg: "interrupted"}, true},
		{"other sqlite code (1)", &mockSQLiteError{code: 1, msg: "SQLITE_ERROR"}, false},
		{"wrapped SQLITE_BUSY", fmt.Errorf("repo: %w", &mockSQLiteError{code: sqliteBusy, msg: "busy"}), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTransientSQLiteError(tt.err))
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	calls := 0
	err := WithRetry(context.Background(), DefaultRetryConfig, nil, "test", func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestWithRetry_NonTransientNoRetry(t *testing.T) {
	calls := 0
	sentinel := errors.New("permanent error")
	err := WithRetry(context.Background(), DefaultRetryConfig, nil, "test", func() error {
		calls++
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls, "should not retry non-transient errors")
}

func TestWithRetry_TransientEventualSuccess(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	}
	calls := 0
	busyErr := &mockSQLiteError{code: sqliteBusy, msg: "busy"}

	err := WithRetry(context.Background(), cfg, nil, "test", func() error {
		calls++
		if calls < 3 {
			return busyErr
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls, "should succeed on third attempt")
}

func TestWithRetry_TransientExhausted(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	}
	calls := 0
	busyErr := &mockSQLiteError{code: sqliteBusy, msg: "busy"}

	err := WithRetry(context.Background(), cfg, nil, "test", func() error {
		calls++
		return busyErr
	})
	require.Error(t, err)
	assert.Equal(t, 3, calls, "should attempt exactly MaxAttempts times")
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: 100 * time.Millisecond, // long enough to be cancelled
		MaxBackoff:     time.Second,
		Multiplier:     2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	busyErr := &mockSQLiteError{code: sqliteBusy, msg: "busy"}

	// Cancel after first attempt
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := WithRetry(ctx, cfg, nil, "test", func() error {
		calls++
		return busyErr
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "should stop retrying when context is cancelled")
}
