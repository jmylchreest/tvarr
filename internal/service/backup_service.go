// Package service provides business logic layer for tvarr operations.
package service

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/version"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BackupService provides business logic for database backup and restore.
type BackupService struct {
	db         *gorm.DB
	cfg        config.BackupConfig
	storageDir string
	logger     *slog.Logger
}

// NewBackupService creates a new backup service.
func NewBackupService(db *gorm.DB, cfg config.BackupConfig, storageBaseDir string) *BackupService {
	backupDir := cfg.BackupPath(storageBaseDir)

	return &BackupService{
		db:         db,
		cfg:        cfg,
		storageDir: backupDir,
		logger:     slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *BackupService) WithLogger(logger *slog.Logger) *BackupService {
	s.logger = logger
	return s
}

// GetScheduleInfo returns the backup schedule configuration.
func (s *BackupService) GetScheduleInfo() models.BackupScheduleInfo {
	return models.BackupScheduleInfo{
		Enabled:   s.cfg.Schedule.Enabled,
		Cron:      s.cfg.Schedule.Cron,
		Retention: s.cfg.Schedule.Retention,
	}
}

// GetBackupDirectory returns the backup storage directory path.
func (s *BackupService) GetBackupDirectory() string {
	return s.storageDir
}

// CreateBackup creates a full database backup.
func (s *BackupService) CreateBackup(ctx context.Context) (*models.BackupMetadata, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(s.storageDir, 0755); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	// Check available disk space before proceeding
	if err := s.checkDiskSpace(); err != nil {
		return nil, err
	}

	// Generate timestamp-based filename with milliseconds for uniqueness
	timestamp := time.Now().UTC()
	baseName := fmt.Sprintf("tvarr-backup-%s", timestamp.Format("2006-01-02T15-04-05.000"))
	dbPath := filepath.Join(s.storageDir, baseName+".db")
	gzPath := filepath.Join(s.storageDir, baseName+".db.gz")
	metaPath := filepath.Join(s.storageDir, baseName+".meta.json")

	// Check if backup with same name already exists (extremely rare with ms precision)
	if _, err := os.Stat(gzPath); err == nil {
		return nil, fmt.Errorf("backup already exists: %s", filepath.Base(gzPath))
	}

	// Use VACUUM INTO for consistent SQLite backup
	s.logger.Debug("creating backup using VACUUM INTO", slog.String("path", dbPath))
	if err := s.db.Exec("VACUUM INTO ?", dbPath).Error; err != nil {
		return nil, fmt.Errorf("vacuum into backup: %w", err)
	}

	// Get uncompressed size
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("stat backup db: %w", err)
	}
	uncompressedSize := dbInfo.Size()

	// Compress with gzip
	if err := s.compressFile(dbPath, gzPath); err != nil {
		os.Remove(dbPath)
		return nil, fmt.Errorf("compressing backup: %w", err)
	}

	// Remove uncompressed file
	os.Remove(dbPath)

	// Get compressed size and calculate checksum
	gzInfo, err := os.Stat(gzPath)
	if err != nil {
		return nil, fmt.Errorf("stat compressed backup: %w", err)
	}

	checksum, err := s.calculateChecksum(gzPath)
	if err != nil {
		return nil, fmt.Errorf("calculating checksum: %w", err)
	}

	// Get table counts
	tableCounts, err := s.getTableCounts(ctx)
	if err != nil {
		s.logger.Warn("failed to get table counts", slog.String("error", err.Error()))
		tableCounts = make(map[string]int)
	}

	// Create metadata file struct
	metaFile := &models.BackupMetadataFile{
		TvarrVersion:   version.Version,
		DatabaseSize:   uncompressedSize,
		CompressedSize: gzInfo.Size(),
		Checksum:       checksum,
		CreatedAt:      timestamp,
		TableCounts:    tableCounts,
	}

	// Write metadata file
	metaJSON, err := json.MarshalIndent(metaFile, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		return nil, fmt.Errorf("writing metadata: %w", err)
	}

	// Build return metadata
	meta := &models.BackupMetadata{
		Filename:       filepath.Base(gzPath),
		FilePath:       gzPath,
		CreatedAt:      timestamp,
		FileSize:       gzInfo.Size(),
		Checksum:       checksum,
		TvarrVersion:   version.Version,
		DatabaseSize:   uncompressedSize,
		CompressedSize: gzInfo.Size(),
		TableCounts:    metaFile.ToTableCounts(),
	}

	s.logger.Info("backup created",
		slog.String("filename", meta.Filename),
		slog.Int64("size", meta.FileSize),
		slog.String("checksum", truncateChecksum(meta.Checksum)),
	)

	return meta, nil
}


// ListBackups returns all available backups sorted by creation time (newest first).
func (s *BackupService) ListBackups(ctx context.Context) ([]*models.BackupMetadata, error) {
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.BackupMetadata{}, nil
		}
		return nil, err
	}

	var backups []*models.BackupMetadata
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".db.gz") {
			continue
		}

		meta, err := s.loadBackupMetadata(filepath.Join(s.storageDir, entry.Name()))
		if err != nil {
			s.logger.Warn("failed to load backup metadata",
				slog.String("filename", entry.Name()),
				slog.String("error", err.Error()),
			)
			continue
		}
		backups = append(backups, meta)
	}

	// Sort by creation time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// GetBackup retrieves metadata for a specific backup.
func (s *BackupService) GetBackup(ctx context.Context, filename string) (*models.BackupMetadata, error) {
	// Validate filename (prevent path traversal)
	if filepath.Base(filename) != filename {
		return nil, fmt.Errorf("invalid filename")
	}

	backupPath := filepath.Join(s.storageDir, filename)
	return s.loadBackupMetadata(backupPath)
}

// DeleteBackup deletes a backup file and its metadata.
func (s *BackupService) DeleteBackup(ctx context.Context, filename string) error {
	// Validate filename (prevent path traversal)
	if filepath.Base(filename) != filename {
		return fmt.Errorf("invalid filename")
	}

	backupPath := filepath.Join(s.storageDir, filename)
	metaPath := strings.TrimSuffix(backupPath, ".db.gz") + ".meta.json"

	// Remove backup file
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing backup file: %w", err)
	}

	// Remove metadata file
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to remove metadata file",
			slog.String("path", metaPath),
			slog.String("error", err.Error()),
		)
	}

	s.logger.Info("backup deleted", slog.String("filename", filename))
	return nil
}

// OpenBackupFile opens a backup file for reading.
func (s *BackupService) OpenBackupFile(ctx context.Context, filename string) (*os.File, error) {
	// Validate filename (prevent path traversal)
	if filepath.Base(filename) != filename {
		return nil, fmt.Errorf("invalid filename")
	}

	backupPath := filepath.Join(s.storageDir, filename)
	return os.Open(backupPath)
}

// RestoreBackup restores the database from a backup file.
// Note: The caller must handle database reconnection after restore.
func (s *BackupService) RestoreBackup(ctx context.Context, filename string) error {
	// Validate filename
	if filepath.Base(filename) != filename {
		return fmt.Errorf("invalid filename")
	}

	backupPath := filepath.Join(s.storageDir, filename)

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	// Load and verify metadata
	meta, err := s.loadBackupMetadata(backupPath)
	if err != nil {
		return fmt.Errorf("loading backup metadata: %w", err)
	}

	// Verify checksum
	checksum, err := s.calculateChecksum(backupPath)
	if err != nil {
		return fmt.Errorf("calculating checksum: %w", err)
	}
	if checksum != meta.Checksum {
		return fmt.Errorf("checksum mismatch: backup may be corrupted")
	}

	// Create pre-restore backup for rollback capability
	preRestoreBackup, err := s.CreateBackup(ctx)
	if err != nil {
		return fmt.Errorf("creating pre-restore backup: %w", err)
	}
	s.logger.Info("created pre-restore backup", slog.String("filename", preRestoreBackup.Filename))

	// Decompress to temp file
	tempDB, err := os.CreateTemp(s.storageDir, "restore-*.db")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tempPath := tempDB.Name()
	tempDB.Close()

	if err := s.decompressFile(backupPath, tempPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("decompressing backup: %w", err)
	}

	// Validate the restored database can be opened
	if err := s.validateDatabase(tempPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("validating restored database: %w", err)
	}

	// Get current database path
	// Note: This relies on GORM's underlying *sql.DB having connection info
	// For SQLite, we need to get the DSN from the dialector
	currentDBPath := s.getDatabasePath()
	if currentDBPath == "" {
		os.Remove(tempPath)
		return fmt.Errorf("could not determine current database path")
	}

	// Atomic swap: rename current to .old, rename temp to current
	oldPath := currentDBPath + ".old"
	os.Remove(oldPath) // Remove any existing .old file

	if err := os.Rename(currentDBPath, oldPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("backing up current database: %w", err)
	}

	if err := os.Rename(tempPath, currentDBPath); err != nil {
		// Try to restore the old database
		os.Rename(oldPath, currentDBPath)
		return fmt.Errorf("installing restored database: %w", err)
	}

	// Remove old database file
	os.Remove(oldPath)

	s.logger.Info("database restored",
		slog.String("from_backup", filename),
		slog.String("pre_restore_backup", preRestoreBackup.Filename),
	)

	return nil
}

// CleanupOldBackups removes backups exceeding the retention limit.
func (s *BackupService) CleanupOldBackups(ctx context.Context) (int, error) {
	retention := s.cfg.Schedule.Retention
	if retention <= 0 {
		return 0, nil // No cleanup configured
	}

	backups, err := s.ListBackups(ctx)
	if err != nil {
		return 0, err
	}

	if len(backups) <= retention {
		return 0, nil
	}

	// Delete oldest backups beyond retention limit
	deleted := 0
	for i := retention; i < len(backups); i++ {
		backup := backups[i]
		if err := s.DeleteBackup(ctx, backup.Filename); err != nil {
			s.logger.Warn("failed to delete old backup",
				slog.String("filename", backup.Filename),
				slog.String("error", err.Error()),
			)
			continue
		}
		deleted++
	}

	if deleted > 0 {
		s.logger.Info("cleaned up old backups", slog.Int("deleted", deleted))
	}

	return deleted, nil
}

// Helper methods

// Minimum free disk space required for backup (100MB)
const minBackupDiskSpace = 100 * 1024 * 1024

// checkDiskSpace verifies sufficient disk space is available for backup.
func (s *BackupService) checkDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.storageDir, &stat); err != nil {
		// Log warning but don't fail - disk space check is best-effort
		s.logger.Warn("unable to check disk space", slog.String("error", err.Error()))
		return nil
	}

	// Calculate available space (available blocks * block size)
	availableBytes := stat.Bavail * uint64(stat.Bsize)

	if availableBytes < minBackupDiskSpace {
		return fmt.Errorf("insufficient disk space for backup: %d bytes available, %d bytes required",
			availableBytes, minBackupDiskSpace)
	}

	s.logger.Debug("disk space check passed",
		slog.Uint64("available_bytes", availableBytes),
		slog.Int64("required_bytes", minBackupDiskSpace),
	)

	return nil
}

func (s *BackupService) compressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	_, err = io.Copy(gzWriter, srcFile)
	return err
}

func (s *BackupService) decompressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	gzReader, err := gzip.NewReader(srcFile)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, gzReader)
	return err
}

func (s *BackupService) calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func (s *BackupService) getTableCounts(ctx context.Context) (map[string]int, error) {
	counts := make(map[string]int)
	tables := []string{
		"channels",
		"epg_programs",
		"filters",
		"data_mapping_rules",
		"client_detection_rules",
		"encoding_profiles",
		"stream_sources",
		"epg_sources",
		"stream_proxies",
	}

	for _, table := range tables {
		var count int64
		if err := s.db.WithContext(ctx).Table(table).Count(&count).Error; err != nil {
			continue // Skip tables that don't exist
		}
		counts[table] = int(count)
	}

	return counts, nil
}

func (s *BackupService) loadBackupMetadata(backupPath string) (*models.BackupMetadata, error) {
	// Get file info
	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, err
	}

	// Try to load companion metadata file
	metaPath := strings.TrimSuffix(backupPath, ".db.gz") + ".meta.json"
	var metaFile models.BackupMetadataFile

	metaData, err := os.ReadFile(metaPath)
	if err == nil {
		if err := json.Unmarshal(metaData, &metaFile); err != nil {
			s.logger.Warn("failed to parse metadata file",
				slog.String("path", metaPath),
				slog.String("error", err.Error()),
			)
		}
	}

	// Parse timestamp from filename if metadata doesn't have it
	createdAt := metaFile.CreatedAt
	if createdAt.IsZero() {
		createdAt = parseTimestampFromFilename(filepath.Base(backupPath))
		if createdAt.IsZero() {
			createdAt = info.ModTime()
		}
	}

	return &models.BackupMetadata{
		Filename:       filepath.Base(backupPath),
		FilePath:       backupPath,
		CreatedAt:      createdAt,
		FileSize:       info.Size(),
		Checksum:       metaFile.Checksum,
		TvarrVersion:   metaFile.TvarrVersion,
		DatabaseSize:   metaFile.DatabaseSize,
		CompressedSize: metaFile.CompressedSize,
		TableCounts:    metaFile.ToTableCounts(),
	}, nil
}

func (s *BackupService) validateDatabase(dbPath string) error {
	// Try to open the database file and run a simple query
	db, err := gorm.Open(s.getDialector(dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	defer sqlDB.Close()

	// Run integrity check
	var result string
	if err := db.Raw("PRAGMA integrity_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("database integrity check failed: %s", result)
	}

	return nil
}

func (s *BackupService) getDialector(dbPath string) gorm.Dialector {
	// Use SQLite dialector for backup validation
	return sqlite.Open(dbPath)
}

func (s *BackupService) getDatabasePath() string {
	// Get the DSN from GORM's dialector
	// For SQLite, this will be the file path
	sqlDB, err := s.db.DB()
	if err != nil {
		return ""
	}

	// For SQLite, we can try to get the database path from the connection
	// This is a bit of a hack, but SQLite stores this info
	var dbPath string
	row := sqlDB.QueryRow("PRAGMA database_list")
	var seq int
	var name string
	if err := row.Scan(&seq, &name, &dbPath); err != nil {
		return ""
	}

	return dbPath
}

// parseTimestampFromFilename extracts timestamp from backup filename.
// Expected formats: tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz or tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.db.gz
func parseTimestampFromFilename(filename string) time.Time {
	// Pattern: tvarr-backup-2006-01-02T15-04-05[.000].db.gz
	// Try with milliseconds first
	reMs := regexp.MustCompile(`tvarr-backup-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3})\.db\.gz`)
	matches := reMs.FindStringSubmatch(filename)
	if len(matches) == 2 {
		t, err := time.Parse("2006-01-02T15-04-05.000", matches[1])
		if err == nil {
			return t.UTC()
		}
	}

	// Fallback to format without milliseconds (for older backups)
	re := regexp.MustCompile(`tvarr-backup-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})\.db\.gz`)
	matches = re.FindStringSubmatch(filename)
	if len(matches) != 2 {
		return time.Time{}
	}

	t, err := time.Parse("2006-01-02T15-04-05", matches[1])
	if err != nil {
		return time.Time{}
	}

	return t.UTC()
}

// truncateChecksum returns a truncated checksum for display.
func truncateChecksum(checksum string) string {
	if len(checksum) > 23 { // "sha256:" + 16 chars
		return checksum[:23] + "..."
	}
	return checksum
}

// ImportBackup imports a backup file from an io.Reader and stores it in the backup directory.
// This allows uploading previously downloaded backups for restore on a new installation.
func (s *BackupService) ImportBackup(ctx context.Context, reader io.Reader, originalFilename string) (*models.BackupMetadata, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(s.storageDir, 0755); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	// Validate filename format (prevent path traversal and ensure valid format)
	if filepath.Base(originalFilename) != originalFilename {
		return nil, fmt.Errorf("invalid filename: must not contain path separators")
	}

	// Check if filename matches expected pattern
	if !isValidBackupFilename(originalFilename) {
		return nil, fmt.Errorf("invalid filename format: expected tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz")
	}

	// Check if backup with same name already exists
	destPath := filepath.Join(s.storageDir, originalFilename)
	if _, err := os.Stat(destPath); err == nil {
		return nil, fmt.Errorf("backup with filename %s already exists", originalFilename)
	}

	// Write uploaded file to temp location first
	tempFile, err := os.CreateTemp(s.storageDir, "upload-*.db.gz")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Copy uploaded content to temp file
	if _, err := io.Copy(tempFile, reader); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("writing uploaded file: %w", err)
	}
	tempFile.Close()

	// Verify the uploaded file is a valid gzipped SQLite database
	if err := s.validateGzippedBackup(tempPath); err != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("validating backup: %w", err)
	}

	// Move temp file to final location
	if err := os.Rename(tempPath, destPath); err != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("moving backup to final location: %w", err)
	}

	// Calculate checksum for the imported file
	checksum, err := s.calculateChecksum(destPath)
	if err != nil {
		return nil, fmt.Errorf("calculating checksum: %w", err)
	}

	// Get file info
	fileInfo, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("getting file info: %w", err)
	}

	// Parse timestamp from filename
	createdAt := parseTimestampFromFilename(originalFilename)
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	// Create metadata file
	metaPath := strings.TrimSuffix(destPath, ".db.gz") + ".meta.json"
	metaFile := &models.BackupMetadataFile{
		TvarrVersion:   "imported", // Mark as imported since we don't know the original version
		CompressedSize: fileInfo.Size(),
		Checksum:       checksum,
		CreatedAt:      createdAt,
		TableCounts:    make(map[string]int), // Will be populated when we validate
	}

	// Try to get database size and table counts by temporarily decompressing
	if dbSize, tableCounts, err := s.inspectGzippedDatabase(destPath); err == nil {
		metaFile.DatabaseSize = dbSize
		metaFile.TableCounts = tableCounts
	}

	metaJSON, err := json.MarshalIndent(metaFile, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		s.logger.Warn("failed to write metadata file", slog.String("error", err.Error()))
	}

	meta := &models.BackupMetadata{
		Filename:       originalFilename,
		FilePath:       destPath,
		CreatedAt:      createdAt,
		FileSize:       fileInfo.Size(),
		Checksum:       checksum,
		TvarrVersion:   metaFile.TvarrVersion,
		DatabaseSize:   metaFile.DatabaseSize,
		CompressedSize: metaFile.CompressedSize,
		TableCounts:    metaFile.ToTableCounts(),
	}

	s.logger.Info("backup imported",
		slog.String("filename", meta.Filename),
		slog.Int64("size", meta.FileSize),
	)

	return meta, nil
}

// validateGzippedBackup verifies that the file is a valid gzipped SQLite database.
func (s *BackupService) validateGzippedBackup(gzPath string) error {
	// Decompress to temp file
	tempFile, err := os.CreateTemp(s.storageDir, "validate-*.db")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if err := s.decompressFile(gzPath, tempPath); err != nil {
		return fmt.Errorf("decompressing: %w", err)
	}

	return s.validateDatabase(tempPath)
}

// inspectGzippedDatabase decompresses and inspects a backup to get database size and table counts.
func (s *BackupService) inspectGzippedDatabase(gzPath string) (int64, map[string]int, error) {
	// Decompress to temp file
	tempFile, err := os.CreateTemp(s.storageDir, "inspect-*.db")
	if err != nil {
		return 0, nil, err
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if err := s.decompressFile(gzPath, tempPath); err != nil {
		return 0, nil, err
	}

	// Get uncompressed size
	info, err := os.Stat(tempPath)
	if err != nil {
		return 0, nil, err
	}

	// Get table counts from the decompressed database
	db, err := gorm.Open(s.getDialector(tempPath), &gorm.Config{})
	if err != nil {
		return info.Size(), nil, nil // Return size even if we can't get counts
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	counts := make(map[string]int)
	tables := []string{
		"channels",
		"epg_programs",
		"filters",
		"data_mapping_rules",
		"client_detection_rules",
		"encoding_profiles",
		"stream_sources",
		"epg_sources",
		"stream_proxies",
	}

	for _, table := range tables {
		var count int64
		if err := db.Table(table).Count(&count).Error; err == nil {
			counts[table] = int(count)
		}
	}

	return info.Size(), counts, nil
}

// isValidBackupFilename checks if a filename matches the expected backup format.
func isValidBackupFilename(filename string) bool {
	// Expected formats:
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz (34 chars)
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.db.gz (38 chars)
	if len(filename) < 34 { // Minimum length for valid filename
		return false
	}

	// Must start with tvarr-backup-
	prefix := "tvarr-backup-"
	if len(filename) < len(prefix) || filename[:len(prefix)] != prefix {
		return false
	}

	// Must end with .db.gz
	suffix := ".db.gz"
	if len(filename) < len(suffix) || filename[len(filename)-len(suffix):] != suffix {
		return false
	}

	// Validate timestamp portion can be parsed
	ts := parseTimestampFromFilename(filename)
	return !ts.IsZero()
}
