// Package database provides database connection management and migrations for tvarr.
// It supports SQLite, PostgreSQL, and MySQL through GORM.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
	sqlite3 "github.com/mattn/go-sqlite3"
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
	// For SQLite, use a SINGLE connection. While WAL mode allows concurrent readers,
	// Go's connection pool blocks when all connections are busy. With multiple connections,
	// if all are tied up in writes (ingestion, job workers), READ requests block waiting
	// for a free connection - causing 30+ second waits and timeouts.
	// A single connection forces serialization but ensures predictable operation ordering.
	maxOpen := cfg.MaxOpenConns
	maxIdle := cfg.MaxIdleConns
	if cfg.Driver == "sqlite" {
		maxOpen = 1 // Single connection prevents Go-level blocking
		maxIdle = 1
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Note: SQLite PRAGMAs are applied via ConnectHook in registerSQLiteDriver()
	// which ensures they are set on EVERY connection from the pool, not just the first.

	dbWrapper := &DB{
		DB:     db,
		cfg:    cfg,
		logger: log,
	}

	// Log connection pool configuration and SQLite PRAGMAs for debugging
	if cfg.Driver == "sqlite" {
		dbWrapper.logSQLiteConfig()
	} else {
		log.Info("database connection pool configured",
			slog.Int("max_open_conns", maxOpen),
			slog.Int("max_idle_conns", maxIdle),
		)
	}

	return dbWrapper, nil
}

// sqliteDriverName is the custom driver name for SQLite with connection hooks.
const sqliteDriverName = "sqlite3_tvarr"

// sqliteDriverRegistered tracks whether our custom driver has been registered.
var sqliteDriverRegistered bool

// registerSQLiteDriver registers a custom SQLite driver with a ConnectHook
// that applies PRAGMAs to every new connection. This is more reliable than
// DSN parameters because it works with all connection pool implementations.
func registerSQLiteDriver() {
	if sqliteDriverRegistered {
		return
	}

	sql.Register(sqliteDriverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			// These PRAGMAs are applied to EVERY connection from the pool.
			// This ensures consistent behavior regardless of which connection
			// handles a particular request.
			//
			// Note: busy_timeout is set to 15s as a balance between giving time for
			// locks to be released and failing fast to allow retries.
			pragmas := []string{
				"PRAGMA busy_timeout = 15000",      // Wait 15s when database is locked
				"PRAGMA journal_mode = WAL",        // Better read/write concurrency
				"PRAGMA synchronous = NORMAL",      // Better performance with WAL
				"PRAGMA foreign_keys = ON",         // Enable foreign key constraints
				"PRAGMA cache_size = -64000",       // 64MB cache (negative = KB)
				"PRAGMA wal_autocheckpoint = 1000", // Checkpoint every 1000 pages to prevent WAL growth
			}

			for _, pragma := range pragmas {
				if _, err := conn.Exec(pragma, nil); err != nil {
					// Log but don't fail - some pragmas may not be supported
					// in all SQLite versions or configurations
					return nil
				}
			}
			return nil
		},
	})

	sqliteDriverRegistered = true
}

// getDialector returns the appropriate GORM dialector for the configured driver.
func getDialector(cfg config.DatabaseConfig) (gorm.Dialector, error) {
	switch cfg.Driver {
	case "sqlite":
		// Register our custom SQLite driver with connection hooks
		registerSQLiteDriver()

		// Use the custom driver that applies PRAGMAs on every connection
		return sqlite.Dialector{
			DriverName: sqliteDriverName,
			DSN:        cfg.DSN,
		}, nil
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
	sqlStr, rows := fc()

	// Categorize errors for better debugging
	errStr := ""
	errType := ""
	if err != nil {
		errStr = err.Error()
		switch {
		case strings.Contains(errStr, "database is locked"):
			errType = "SQLITE_BUSY"
		case strings.Contains(errStr, "context canceled"):
			errType = "CONTEXT_CANCELED"
		case strings.Contains(errStr, "context deadline exceeded"):
			errType = "TIMEOUT"
		case strings.Contains(errStr, "record not found"):
			errType = "NOT_FOUND"
		default:
			errType = "OTHER"
		}
	}

	switch {
	case err != nil && l.level >= logger.Error:
		l.logger.ErrorContext(ctx, "database error",
			slog.String("error_type", errType),
			slog.String("sql", sqlStr),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
			slog.String("error", errStr),
		)
	case elapsed > 200*time.Millisecond && l.level >= logger.Warn:
		l.logger.WarnContext(ctx, "slow query",
			slog.String("sql", sqlStr),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
		)
	case l.level >= logger.Info:
		l.logger.DebugContext(ctx, "database query",
			slog.String("sql", sqlStr),
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

// logSQLiteConfig queries and logs the actual SQLite PRAGMA values.
// This helps verify that our configuration is being applied correctly.
func (db *DB) logSQLiteConfig() {
	sqlDB, err := db.DB.DB()
	if err != nil {
		db.logger.Warn("failed to get sql.DB for SQLite config logging", slog.String("error", err.Error()))
		return
	}

	stats := sqlDB.Stats()

	// Query actual PRAGMA values
	var journalMode, synchronous string
	var busyTimeout, cacheSize, walAutocheckpoint int

	_ = db.DB.Raw("PRAGMA journal_mode").Scan(&journalMode)
	_ = db.DB.Raw("PRAGMA synchronous").Scan(&synchronous)
	_ = db.DB.Raw("PRAGMA busy_timeout").Scan(&busyTimeout)
	_ = db.DB.Raw("PRAGMA cache_size").Scan(&cacheSize)
	_ = db.DB.Raw("PRAGMA wal_autocheckpoint").Scan(&walAutocheckpoint)

	db.logger.Info("SQLite configuration",
		slog.String("journal_mode", journalMode),
		slog.String("synchronous", synchronous),
		slog.Int("busy_timeout_ms", busyTimeout),
		slog.Int("cache_size", cacheSize),
		slog.Int("wal_autocheckpoint", walAutocheckpoint),
		slog.Int("max_open_conns", stats.MaxOpenConnections),
		slog.Int("max_idle_conns", stats.MaxOpenConnections), // MaxIdleConns not directly available
		slog.Int("open_conns", stats.OpenConnections),
		slog.Int("in_use", stats.InUse),
		slog.Int("idle", stats.Idle),
	)
}
