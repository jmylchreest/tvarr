// Package database provides database connection management and migrations for tvarr.
// It supports SQLite, PostgreSQL, and MySQL through GORM.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB wraps a GORM database connection with additional functionality.
type DB struct {
	*gorm.DB
	cfg    config.DatabaseConfig
	logger *slog.Logger
}

// Options contains optional configuration for database connections.
type Options struct {
	// PrepareStmt enables prepared statement caching. Default is true.
	// Set to false for SQLite when using transactions in tests.
	PrepareStmt bool
}

// New creates a new database connection based on the provided configuration.
// Use opts to customize behavior; pass nil for defaults (PrepareStmt: true).
func New(cfg config.DatabaseConfig, log *slog.Logger, opts *Options) (*DB, error) {
	if opts == nil {
		opts = &Options{PrepareStmt: true}
	}
	if log == nil {
		log = slog.Default()
	}

	dialector, err := getDialector(cfg)
	if err != nil {
		return nil, fmt.Errorf("getting dialector: %w", err)
	}

	gormLogger := newGormLogger(cfg.LogLevel, log)

	gormCfg := &gorm.Config{
		Logger:                                   gormLogger,
		SkipDefaultTransaction:                   true, // Performance: skip transactions for single operations
		DisableForeignKeyConstraintWhenMigrating: false,
		PrepareStmt:                              opts.PrepareStmt,
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting underlying sql.DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Enable WAL mode for SQLite (better concurrency)
	if cfg.Driver == "sqlite" {
		if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
			log.Warn("failed to enable WAL mode", slog.String("error", err.Error()))
		}
		// Set busy_timeout to 30 seconds to handle long-running transactions during
		// ingestion/generation without failing API requests
		if err := db.Exec("PRAGMA busy_timeout=30000").Error; err != nil {
			log.Warn("failed to set busy timeout", slog.String("error", err.Error()))
		}
		if err := db.Exec("PRAGMA synchronous=NORMAL").Error; err != nil {
			log.Warn("failed to set synchronous mode", slog.String("error", err.Error()))
		}
		if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
			log.Warn("failed to enable foreign keys", slog.String("error", err.Error()))
		}
	}

	return &DB{
		DB:     db,
		cfg:    cfg,
		logger: log,
	}, nil
}

// getDialector returns the appropriate GORM dialector for the configured driver.
func getDialector(cfg config.DatabaseConfig) (gorm.Dialector, error) {
	switch cfg.Driver {
	case "sqlite":
		return sqlite.Open(cfg.DSN), nil
	case "postgres":
		return postgres.Open(cfg.DSN), nil
	case "mysql":
		return mysql.Open(cfg.DSN), nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
}

// gormLogLevel maps string log levels to GORM logger levels.
func gormLogLevel(level string) logger.LogLevel {
	switch level {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "warn":
		return logger.Warn
	case "info":
		return logger.Info
	default:
		return logger.Warn
	}
}

// newGormLogger creates a GORM logger that uses slog.
func newGormLogger(level string, log *slog.Logger) logger.Interface {
	return &slogGormLogger{
		logger: log,
		level:  gormLogLevel(level),
	}
}

// slogGormLogger implements GORM's logger.Interface using slog.
type slogGormLogger struct {
	logger *slog.Logger
	level  logger.LogLevel
}

func (l *slogGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &slogGormLogger{
		logger: l.logger,
		level:  level,
	}
}

func (l *slogGormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Info {
		l.logger.InfoContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (l *slogGormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Warn {
		l.logger.WarnContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (l *slogGormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	if l.level >= logger.Error {
		l.logger.ErrorContext(ctx, fmt.Sprintf(msg, args...))
	}
}

func (l *slogGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && l.level >= logger.Error:
		l.logger.ErrorContext(ctx, "database error",
			slog.String("sql", sql),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
			slog.String("error", err.Error()),
		)
	case elapsed > 200*time.Millisecond && l.level >= logger.Warn:
		l.logger.WarnContext(ctx, "slow query",
			slog.String("sql", sql),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
		)
	case l.level >= logger.Info:
		l.logger.DebugContext(ctx, "database query",
			slog.String("sql", sql),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
		)
	}
}

// Close closes the database connection.
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("getting underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// Ping verifies the database connection is alive.
func (db *DB) Ping(ctx context.Context) error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("getting underlying sql.DB: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// WithContext returns a new DB with the given context.
func (db *DB) WithContext(ctx context.Context) *DB {
	return &DB{
		DB:     db.DB.WithContext(ctx),
		cfg:    db.cfg,
		logger: db.logger,
	}
}

// Transaction executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
func (db *DB) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return db.DB.WithContext(ctx).Transaction(fn)
}

// Driver returns the database driver name.
func (db *DB) Driver() string {
	return db.cfg.Driver
}

// Stats returns database connection pool statistics.
func (db *DB) Stats() (map[string]interface{}, error) {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("getting underlying sql.DB: %w", err)
	}

	stats := sqlDB.Stats()
	return map[string]interface{}{
		"max_open_connections": stats.MaxOpenConnections,
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration.String(),
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_idle_time_closed": stats.MaxIdleTimeClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}, nil
}
