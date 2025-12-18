package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNew_SQLite(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    1, // SQLite in-memory requires single connection
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		LogLevel:        "warn",
	}

	db, err := New(cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

	// Verify we can ping
	err = db.Ping(context.Background())
	assert.NoError(t, err)

	// Verify driver name
	assert.Equal(t, "sqlite", db.Driver())
}

func TestNew_InvalidDriver(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver: "invalid",
		DSN:    ":memory:",
	}

	db, err := New(cfg, nil, nil)
	assert.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "unsupported database driver")
}

func TestDB_Ping(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	err := db.Ping(ctx)
	assert.NoError(t, err)
}

func TestDB_Ping_WithTimeout(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := db.Ping(ctx)
	assert.NoError(t, err)
}

func TestDB_Close(t *testing.T) {
	db := setupTestDB(t)

	err := db.Close()
	assert.NoError(t, err)

	// Ping should fail after close
	err = db.Ping(context.Background())
	assert.Error(t, err)
}

func TestDB_Stats(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	stats, err := db.Stats()
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Verify expected keys exist
	assert.Contains(t, stats, "max_open_connections")
	assert.Contains(t, stats, "open_connections")
	assert.Contains(t, stats, "in_use")
	assert.Contains(t, stats, "idle")
}

func TestDB_WithContext(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	ctxDB := db.WithContext(ctx)

	assert.NotNil(t, ctxDB)
	assert.Equal(t, db.Driver(), ctxDB.Driver())
}

func TestDB_Transaction(t *testing.T) {
	cfg := config.DatabaseConfig{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    1, // Single connection for SQLite in-memory
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		LogLevel:        "silent",
	}

	db, err := New(cfg, nil, &Options{PrepareStmt: false})
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Create a test model for transaction testing
	type TxTestItem struct {
		ID    uint   `gorm:"primarykey"`
		Value string `gorm:"not null"`
	}

	// Auto-migrate the test table
	err = db.DB.AutoMigrate(&TxTestItem{})
	require.NoError(t, err)

	// Test successful transaction
	err = db.Transaction(ctx, func(tx *gorm.DB) error {
		return tx.Create(&TxTestItem{Value: "test1"}).Error
	})
	assert.NoError(t, err)

	// Verify the insert
	var count int64
	err = db.DB.Model(&TxTestItem{}).Where("value = ?", "test1").Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Test failed transaction (should rollback)
	testErr := fmt.Errorf("forced rollback error")
	err = db.Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&TxTestItem{Value: "test2"}).Error; err != nil {
			return err
		}
		return testErr // Force rollback
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)

	// Verify test2 was not inserted (rolled back)
	err = db.DB.Model(&TxTestItem{}).Where("value = ?", "test2").Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestDB_SQLitePragmas(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Note: In-memory SQLite uses "memory" journal mode, not "wal"
	// WAL mode is only applicable to file-based databases
	var journalMode string
	err := db.DB.Raw("PRAGMA journal_mode").Scan(&journalMode).Error
	require.NoError(t, err)
	assert.Equal(t, "memory", journalMode) // In-memory DB uses memory journal

	// Verify foreign keys are enabled
	var foreignKeys int
	err = db.DB.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error
	require.NoError(t, err)
	assert.Equal(t, 1, foreignKeys)
}

func TestGormLogLevel(t *testing.T) {
	tests := []struct {
		level    string
		expected logger.LogLevel
	}{
		{"silent", logger.Silent},
		{"error", logger.Error},
		{"warn", logger.Warn},
		{"info", logger.Info},
		{"unknown", logger.Warn},
		{"", logger.Warn},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			result := gormLogLevel(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildSQLiteDSN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "empty DSN unchanged",
			input:    "",
			contains: []string{},
			excludes: []string{"_busy_timeout"},
		},
		{
			name:     "memory DSN unchanged",
			input:    ":memory:",
			contains: []string{},
			excludes: []string{"_busy_timeout"},
		},
		{
			name:     "simple file path gets defaults",
			input:    "tvarr.db",
			contains: []string{"_busy_timeout=30000", "_journal_mode=WAL", "_synchronous=NORMAL", "_foreign_keys=ON", "_txlock=immediate"},
			excludes: []string{},
		},
		{
			name:     "path with existing params preserves them",
			input:    "/data/tvarr.db?_busy_timeout=5000",
			contains: []string{"_busy_timeout=5000", "_journal_mode=WAL"},
			excludes: []string{"_busy_timeout=30000"},
		},
		{
			name:     "user can override all defaults",
			input:    "test.db?_busy_timeout=1000&_journal_mode=DELETE&_synchronous=FULL&_foreign_keys=OFF&_txlock=deferred",
			contains: []string{"_busy_timeout=1000", "_journal_mode=DELETE", "_synchronous=FULL", "_foreign_keys=OFF", "_txlock=deferred"},
			excludes: []string{"_busy_timeout=30000", "_journal_mode=WAL"},
		},
		{
			name:     "case insensitive parameter detection",
			input:    "test.db?_BUSY_TIMEOUT=5000",
			contains: []string{"_BUSY_TIMEOUT=5000", "_journal_mode=WAL"},
			excludes: []string{"_busy_timeout=30000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSQLiteDSN(tt.input)

			for _, s := range tt.contains {
				assert.Contains(t, result, s, "expected DSN to contain %q", s)
			}
			for _, s := range tt.excludes {
				assert.NotContains(t, result, s, "expected DSN to NOT contain %q", s)
			}
		})
	}
}

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	cfg := config.DatabaseConfig{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    1, // SQLite in-memory requires single connection
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		LogLevel:        "silent",
	}

	db, err := New(cfg, nil, nil)
	require.NoError(t, err)

	return db
}
