package database

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// SQLite extended error codes for transient lock conditions.
// These are returned by modernc.org/sqlite (the pure-Go SQLite driver).
const (
	// sqliteBusy is SQLITE_BUSY (5): the database file is locked by another connection.
	// This occurs when a writer holds a lock and busy_timeout has expired.
	sqliteBusy = 5

	// sqliteLocked is SQLITE_LOCKED (6): a table in the database is locked.
	// This occurs with intra-process table-level locks.
	sqliteLocked = 6

	// sqliteInterrupt is SQLITE_INTERRUPT (9): operation interrupted.
	// Can happen under high contention when SQLite internal operations are cancelled.
	sqliteInterrupt = 9
)

// sqliteErrCode is the interface implemented by modernc.org/sqlite errors.
// We use an interface to avoid a direct import dependency on the driver.
type sqliteErrCode interface {
	Code() int
}

// isTransientSQLiteError returns true if the error is a transient SQLite
// concurrency error that is safe to retry.
func isTransientSQLiteError(err error) bool {
	if err == nil {
		return false
	}

	// Walk the error chain looking for a SQLite error code.
	var coded sqliteErrCode
	if errors.As(err, &coded) {
		switch coded.Code() {
		case sqliteBusy, sqliteLocked, sqliteInterrupt:
			return true
		}
	}

	return false
}

// RetryConfig controls retry behaviour for transient SQLite errors.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (including the first).
	MaxAttempts int
	// InitialBackoff is the wait time before the second attempt.
	InitialBackoff time.Duration
	// MaxBackoff caps the wait time between attempts.
	MaxBackoff time.Duration
	// Multiplier scales the backoff after each attempt.
	Multiplier float64
}

// DefaultRetryConfig is a sensible default for repository operations:
// up to 4 attempts with 100 ms → 500 ms → 2 s backoff.
var DefaultRetryConfig = RetryConfig{
	MaxAttempts:    4,
	InitialBackoff: 100 * time.Millisecond,
	MaxBackoff:     2 * time.Second,
	Multiplier:     4.0,
}

// WithRetry executes fn, retrying on transient SQLite errors according to cfg.
// If ctx is cancelled between retries the context error is returned immediately.
// The last error is returned if all attempts are exhausted.
func WithRetry(ctx context.Context, cfg RetryConfig, logger *slog.Logger, op string, fn func() error) error {
	backoff := cfg.InitialBackoff

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if !isTransientSQLiteError(err) {
			// Non-transient – don't retry.
			return err
		}

		if attempt == cfg.MaxAttempts {
			// Final attempt exhausted.
			if logger != nil {
				logger.Warn("sqlite retry exhausted",
					slog.String("op", op),
					slog.Int("attempts", attempt),
					slog.Any("error", err),
				)
			}
			return err
		}

		if logger != nil {
			logger.Debug("sqlite transient error, retrying",
				slog.String("op", op),
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", cfg.MaxAttempts),
				slog.Duration("backoff", backoff),
				slog.Any("error", err),
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(float64(backoff) * cfg.Multiplier)
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}

	// Unreachable, but satisfies the compiler.
	return nil
}
