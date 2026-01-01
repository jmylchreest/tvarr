// Package database provides database connection management and migrations for tvarr.
// It supports SQLite, PostgreSQL, and MySQL through GORM.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jmylchreest/tvarr/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
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

	// Enable stats logging on SQLITE_BUSY errors
	gormLogger.SetSQLDB(sqlDB)

	// Configure connection pool
	// For SQLite in WAL mode, concurrent readers are allowed but only one writer at a time.
	// We use 6 connections (in the recommended 5-8 range) to balance:
	// - Enough connections for concurrent reads during writes
	// - Not so many that we increase lock contention
	// - Job workers (2), ingestion writes, and UI reads need their own slots
	// Monitor wait_count/wait_duration in logs to detect connection starvation.
	maxOpen := cfg.MaxOpenConns
	maxIdle := cfg.MaxIdleConns
	if cfg.Driver == "sqlite" {
		maxOpen = 6 // 5-8 recommended, avoid over-provisioning
		maxIdle = 3
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

// getDialector returns the appropriate GORM dialector for the configured driver.
func getDialector(cfg config.DatabaseConfig) (gorm.Dialector, error) {
	switch cfg.Driver {
	case "sqlite":
		// Use pure Go SQLite driver (github.com/glebarez/sqlite -> modernc.org/sqlite)
		// This eliminates CGO overhead which was 40%+ of CPU time in profiles.
		// PRAGMAs are applied via DSN parameters using _pragma syntax.
		dsn := cfg.DSN
		if !strings.Contains(dsn, "?") {
			dsn += "?"
		} else {
			dsn += "&"
		}
		// Apply SQLite PRAGMAs via DSN for the pure Go driver
		// These are applied to every connection from the pool.
		dsn += "_pragma=busy_timeout(30000)" + // Wait 30s when database is locked
			"&_pragma=journal_mode(WAL)" + // Better read/write concurrency
			"&_pragma=synchronous(NORMAL)" + // Better performance with WAL
			"&_pragma=foreign_keys(ON)" + // Enable foreign key constraints
			"&_pragma=cache_size(-64000)" + // 64MB cache (negative = KB)
			"&_pragma=mmap_size(268435456)" + // 256MB memory-mapped I/O for faster reads
			"&_pragma=temp_store(MEMORY)" + // Store temp tables/indices in RAM
			"&_pragma=wal_autocheckpoint(1000)" // Checkpoint every 1000 pages

		return sqlite.Open(dsn), nil
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
func newGormLogger(level string, log *slog.Logger) *slogGormLogger {
	return &slogGormLogger{
		logger: log,
		level:  gormLogLevel(level),
	}
}

// SetSQLDB sets the sql.DB reference for stats logging on errors.
// Call this after opening the connection.
func (l *slogGormLogger) SetSQLDB(db *sql.DB) {
	l.sqlDB = db
}

// slogGormLogger implements GORM's logger.Interface using slog.
type slogGormLogger struct {
	logger        *slog.Logger
	level         logger.LogLevel
	sqlDB         *sql.DB    // Optional: for stats logging on errors
	lastStatsLog  time.Time  // Rate limit stats logging
	statsLogMutex sync.Mutex // Protect lastStatsLog
}

func (l *slogGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &slogGormLogger{
		logger:       l.logger,
		level:        level,
		sqlDB:        l.sqlDB,
		lastStatsLog: l.lastStatsLog,
	}
}

// logStatsOnError logs connection pool stats when we see lock contention.
// Rate limited to once per minute to avoid log spam.
func (l *slogGormLogger) logStatsOnError() {
	if l.sqlDB == nil {
		return
	}

	l.statsLogMutex.Lock()
	defer l.statsLogMutex.Unlock()

	// Rate limit to once per minute
	if time.Since(l.lastStatsLog) < time.Minute {
		return
	}
	l.lastStatsLog = time.Now()

	stats := l.sqlDB.Stats()
	l.logger.Warn("SQLite connection pool stats (on lock contention)",
		slog.Int("max_open_conns", stats.MaxOpenConnections),
		slog.Int("open_conns", stats.OpenConnections),
		slog.Int("in_use", stats.InUse),
		slog.Int("idle", stats.Idle),
		slog.Int64("wait_count", stats.WaitCount),
		slog.String("wait_duration", stats.WaitDuration.String()),
	)
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

// slowQueryThreshold defines when a query is considered slow.
// Set to 1 second to avoid excessive logging during batch operations.
const slowQueryThreshold = 1 * time.Second

// maxSQLLogLength limits SQL string length in logs to reduce overhead.
// Full SQL with interpolated values can be megabytes for batch inserts.
const maxSQLLogLength = 200

// truncateSQL truncates a SQL string for logging, preserving the query type.
func truncateSQL(sql string) string {
	if len(sql) <= maxSQLLogLength {
		return sql
	}
	return sql[:maxSQLLogLength] + "... (truncated)"
}

func (l *slogGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)

	// Determine if we need to log BEFORE calling fc() to avoid expensive SQL string generation.
	// fc() calls GORM's ExplainSQL which builds the full SQL string with interpolated parameters.
	// This was causing 26% of all allocations in profiles.
	isError := err != nil
	isSlow := elapsed > slowQueryThreshold

	// Fast path: determine what we would log and at what level
	// Only generate SQL string if slog will actually output it
	var willLog bool
	if isError && l.level >= logger.Error {
		// Errors are logged at ERROR level - always visible
		willLog = true
	} else if isSlow && l.level >= logger.Warn {
		// Slow queries are logged at WARN level
		willLog = l.logger.Enabled(ctx, slog.LevelWarn)
	} else if l.level >= logger.Info {
		// Normal queries are logged at DEBUG level - check if DEBUG is enabled
		willLog = l.logger.Enabled(ctx, slog.LevelDebug)
	}

	if !willLog {
		return
	}

	// Only now do we call fc() to get the SQL string
	sqlStr, rows := fc()

	// Categorize errors for better debugging
	errStr := ""
	errType := ""
	if err != nil {
		errStr = err.Error()
		switch {
		case strings.Contains(errStr, "database is locked"):
			errType = "SQLITE_BUSY"
			// Log connection pool stats on lock contention (rate limited)
			l.logStatsOnError()
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
	case isError:
		// For errors, truncate SQL to avoid log spam but keep enough for debugging
		l.logger.ErrorContext(ctx, "database error",
			slog.String("error_type", errType),
			slog.String("sql", truncateSQL(sqlStr)),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
			slog.String("error", errStr),
		)
	case isSlow:
		// For slow queries, truncate SQL - the pattern is more important than the data
		l.logger.WarnContext(ctx, "slow query",
			slog.String("sql", truncateSQL(sqlStr)),
			slog.Int64("rows", rows),
			slog.Duration("elapsed", elapsed),
		)
	default:
		l.logger.DebugContext(ctx, "database query",
			slog.String("sql", truncateSQL(sqlStr)),
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

// StartStatsMonitor starts a background goroutine that logs connection pool
// stats every 30 minutes. Only active for SQLite. Cancel ctx to stop.
func (db *DB) StartStatsMonitor(ctx context.Context) {
	if db.cfg.Driver != "sqlite" {
		return
	}

	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				db.LogStats()
			}
		}
	}()

	db.logger.Debug("SQLite stats monitor started (logs every 30m)")
}

// LogStats logs current connection pool statistics.
func (db *DB) LogStats() {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return
	}

	stats := sqlDB.Stats()
	db.logger.Info("SQLite connection pool stats (periodic)",
		slog.Int("max_open_conns", stats.MaxOpenConnections),
		slog.Int("open_conns", stats.OpenConnections),
		slog.Int("in_use", stats.InUse),
		slog.Int("idle", stats.Idle),
		slog.Int64("wait_count", stats.WaitCount),
		slog.String("wait_duration", stats.WaitDuration.String()),
		slog.Int64("max_idle_closed", stats.MaxIdleClosed),
		slog.Int64("max_lifetime_closed", stats.MaxLifetimeClosed),
	)
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
	var journalMode, synchronous, tempStore string
	var busyTimeout, cacheSize, walAutocheckpoint, mmapSize int64

	_ = db.DB.Raw("PRAGMA journal_mode").Scan(&journalMode)
	_ = db.DB.Raw("PRAGMA synchronous").Scan(&synchronous)
	_ = db.DB.Raw("PRAGMA busy_timeout").Scan(&busyTimeout)
	_ = db.DB.Raw("PRAGMA cache_size").Scan(&cacheSize)
	_ = db.DB.Raw("PRAGMA wal_autocheckpoint").Scan(&walAutocheckpoint)
	_ = db.DB.Raw("PRAGMA mmap_size").Scan(&mmapSize)
	_ = db.DB.Raw("PRAGMA temp_store").Scan(&tempStore)

	db.logger.Info("SQLite configuration",
		slog.String("journal_mode", journalMode),
		slog.String("synchronous", synchronous),
		slog.Int64("busy_timeout_ms", busyTimeout),
		slog.Int64("cache_size", cacheSize),
		slog.Int64("mmap_size_mb", mmapSize/(1024*1024)),
		slog.String("temp_store", tempStore),
		slog.Int64("wal_autocheckpoint", walAutocheckpoint),
	)

	// Log Go connection pool statistics - these show connection starvation
	db.logger.Info("SQLite connection pool",
		slog.Int("max_open_conns", stats.MaxOpenConnections),
		slog.Int("open_conns", stats.OpenConnections),
		slog.Int("in_use", stats.InUse),
		slog.Int("idle", stats.Idle),
		slog.Int64("wait_count", stats.WaitCount),                 // Total blocked connections
		slog.String("wait_duration", stats.WaitDuration.String()), // Total time blocked
	)
}
