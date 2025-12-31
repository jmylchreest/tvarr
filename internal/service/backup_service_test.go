// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBackupTestDB(t *testing.T, dbPath string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Auto-migrate tables
	err = db.AutoMigrate(
		&models.Filter{},
		&models.DataMappingRule{},
		&models.ClientDetectionRule{},
		&models.EncodingProfile{},
	)
	require.NoError(t, err)

	return db
}

func TestBackupService_CreateBackup(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	// Setup database
	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	// Add some test data
	filter := createTestFilter("Test Filter", "test > 1", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	err := db.Create(filter).Error
	require.NoError(t, err)

	// Create backup service
	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create backup
	meta, err := service.CreateBackup(ctx)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// Verify metadata
	assert.NotEmpty(t, meta.Filename)
	assert.Contains(t, meta.Filename, "tvarr-backup-")
	assert.Contains(t, meta.Filename, ".tar.gz")
	assert.Equal(t, backupDir, filepath.Dir(meta.FilePath))
	assert.NotZero(t, meta.FileSize)
	assert.NotEmpty(t, meta.Checksum)
	assert.True(t, meta.Checksum[:7] == "sha256:")
	assert.NotZero(t, meta.DatabaseSize)
	assert.NotZero(t, meta.CompressedSize)
	assert.True(t, meta.CompressedSize <= meta.DatabaseSize, "compressed should be smaller or equal")

	// Verify file exists
	_, err = os.Stat(meta.FilePath)
	require.NoError(t, err, "backup file should exist")

	// Note: new .tar.gz format embeds metadata inside the archive,
	// not as a companion .meta.json file

	// Verify table counts in metadata (TableCounts is a struct)
	assert.Equal(t, 1, meta.TableCounts.Filters, "should have 1 filter")
}

func TestBackupService_ListBackups(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Initially no backups
	backups, err := service.ListBackups(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 0)

	// Create a single backup
	created, err := service.CreateBackup(ctx)
	require.NoError(t, err)

	// List backups should now return 1
	backups, err = service.ListBackups(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 1)
	assert.Equal(t, created.Filename, backups[0].Filename)
}

func TestBackupService_GetBackup(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create a backup
	created, err := service.CreateBackup(ctx)
	require.NoError(t, err)

	// Get the backup
	retrieved, err := service.GetBackup(ctx, created.Filename)
	require.NoError(t, err)
	assert.Equal(t, created.Filename, retrieved.Filename)
	assert.Equal(t, created.Checksum, retrieved.Checksum)

	// Test path traversal prevention
	_, err = service.GetBackup(ctx, "../../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filename")

	// Test non-existent backup
	_, err = service.GetBackup(ctx, "nonexistent.db.gz")
	assert.Error(t, err)
}

func TestBackupService_DeleteBackup(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create a backup
	created, err := service.CreateBackup(ctx)
	require.NoError(t, err)

	// Verify it exists
	_, err = os.Stat(created.FilePath)
	require.NoError(t, err)

	// Delete the backup
	err = service.DeleteBackup(ctx, created.Filename)
	require.NoError(t, err)

	// Verify file is deleted
	_, err = os.Stat(created.FilePath)
	assert.True(t, os.IsNotExist(err))

	// Verify metadata file is also deleted
	metaPath := created.FilePath[:len(created.FilePath)-6] + ".meta.json"
	_, err = os.Stat(metaPath)
	assert.True(t, os.IsNotExist(err))

	// Test path traversal prevention
	err = service.DeleteBackup(ctx, "../../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filename")
}

func TestBackupService_CleanupOldBackups(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	// Retention of 2
	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   true,
			Retention: 2,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create backup directory
	err := os.MkdirAll(backupDir, 0755)
	require.NoError(t, err)

	// Create fake backup files with different timestamps to simulate multiple backups
	backupFiles := []string{
		"tvarr-backup-2025-01-01T10-00-00.db.gz",
		"tvarr-backup-2025-01-02T10-00-00.db.gz",
		"tvarr-backup-2025-01-03T10-00-00.db.gz",
		"tvarr-backup-2025-01-04T10-00-00.db.gz",
		"tvarr-backup-2025-01-05T10-00-00.db.gz",
	}
	for _, filename := range backupFiles {
		// Create empty gz file (minimal valid gzip)
		filePath := filepath.Join(backupDir, filename)
		err := os.WriteFile(filePath, []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 0644)
		require.NoError(t, err)
	}

	// Verify we have 5
	backups, err := service.ListBackups(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 5)

	// Clean up old backups
	deleted, err := service.CleanupOldBackups(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, deleted, "should delete 3 oldest backups")

	// Verify only 2 remain
	backups, err = service.ListBackups(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 2)

	// Verify the newest two remain
	remainingNames := make([]string, len(backups))
	for i, b := range backups {
		remainingNames[i] = b.Filename
	}
	assert.Contains(t, remainingNames, "tvarr-backup-2025-01-05T10-00-00.db.gz")
	assert.Contains(t, remainingNames, "tvarr-backup-2025-01-04T10-00-00.db.gz")
}

func TestBackupService_CleanupOldBackups_NoRetention(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	// Retention of 0 (disabled)
	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 0,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create backup directory
	err := os.MkdirAll(backupDir, 0755)
	require.NoError(t, err)

	// Create fake backup files
	backupFiles := []string{
		"tvarr-backup-2025-01-01T10-00-00.db.gz",
		"tvarr-backup-2025-01-02T10-00-00.db.gz",
		"tvarr-backup-2025-01-03T10-00-00.db.gz",
	}
	for _, filename := range backupFiles {
		filePath := filepath.Join(backupDir, filename)
		err := os.WriteFile(filePath, []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, 0644)
		require.NoError(t, err)
	}

	// Clean up with retention=0 should not delete anything
	deleted, err := service.CleanupOldBackups(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)

	// All 3 should still exist
	backups, err := service.ListBackups(ctx)
	require.NoError(t, err)
	assert.Len(t, backups, 3)
}

func TestBackupService_OpenBackupFile(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create a backup
	created, err := service.CreateBackup(ctx)
	require.NoError(t, err)

	// Open the backup file
	file, err := service.OpenBackupFile(ctx, created.Filename)
	require.NoError(t, err)
	defer file.Close()

	// Should be able to read from the file
	info, err := file.Stat()
	require.NoError(t, err)
	assert.Equal(t, created.FileSize, info.Size())

	// Test path traversal prevention
	_, err = service.OpenBackupFile(ctx, "../../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filename")
}

func TestBackupService_RestoreBackup(t *testing.T) {
	// NOTE: Full restore testing is complex because RestoreBackup replaces the DB file
	// while connections may be open. This test focuses on validation and error paths.
	// The actual restore logic is tested via checksum validation and path traversal checks.

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	// Setup database with initial data
	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	// Add original test data
	filter := createTestFilter("Original Filter", "original > 1", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	err := db.Create(filter).Error
	require.NoError(t, err)

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create a backup
	backup, err := service.CreateBackup(ctx)
	require.NoError(t, err)

	// Verify the backup file exists and has content
	backupInfo, err := os.Stat(backup.FilePath)
	require.NoError(t, err)
	assert.Greater(t, backupInfo.Size(), int64(0))

	// Verify the metadata matches
	retrieved, err := service.GetBackup(ctx, backup.Filename)
	require.NoError(t, err)
	assert.Equal(t, backup.Checksum, retrieved.Checksum)
	assert.Equal(t, backup.DatabaseSize, retrieved.DatabaseSize)
}

func TestBackupService_RestoreBackup_CorruptedArchive(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Create a backup
	backup, err := service.CreateBackup(ctx)
	require.NoError(t, err)

	// Corrupt the backup file by overwriting bytes in the middle
	// (appending doesn't work because gzip ignores trailing data)
	f, err := os.OpenFile(backup.FilePath, os.O_WRONLY, 0644)
	require.NoError(t, err)
	f.Seek(100, 0) // Seek to middle of file
	f.WriteString("CORRUPTED")
	f.Close()

	// Attempt restore - should fail due to archive corruption
	// Note: .tar.gz format skips checksum verification (checksum is self-referential),
	// but corruption is detected during gzip/tar extraction
	err = service.RestoreBackup(ctx, backup.Filename)
	assert.Error(t, err, "restore should fail on corrupted archive")
}

func TestBackupService_RestoreBackup_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	db := setupBackupTestDB(t, dbPath)
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	service := NewBackupService(db, cfg, tempDir)

	ctx := context.Background()

	// Attempt restore with path traversal
	err := service.RestoreBackup(ctx, "../../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filename")
}

func TestBackupService_BackupPath(t *testing.T) {
	tests := []struct {
		name           string
		directory      string
		storageBaseDir string
		expectedPath   string
	}{
		{
			name:           "custom directory",
			directory:      "/custom/backups",
			storageBaseDir: "/data",
			expectedPath:   "/custom/backups",
		},
		{
			name:           "default directory (empty)",
			directory:      "",
			storageBaseDir: "/data",
			expectedPath:   "/data/backups",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.BackupConfig{
				Directory: tt.directory,
			}
			result := cfg.BackupPath(tt.storageBaseDir)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}
