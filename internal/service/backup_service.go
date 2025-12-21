// Package service provides business logic layer for tvarr operations.
package service

import (
	"archive/tar"
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

// Backup archive internal filenames
const (
	backupDatabaseFile = "database.db"
	backupMetadataFile = "metadata.json"
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

// GetScheduleInfo returns the effective backup schedule configuration.
// Database settings take precedence over config file defaults.
func (s *BackupService) GetScheduleInfo(ctx context.Context) models.BackupScheduleInfo {
	// Get database settings (may be nil or have nil fields)
	var dbSettings models.BackupSettings
	s.db.WithContext(ctx).First(&dbSettings)

	// Merge with config defaults
	return dbSettings.ToScheduleInfo(
		s.cfg.Schedule.Enabled,
		s.cfg.Schedule.Cron,
		s.cfg.Schedule.Retention,
	)
}

// GetEffectiveSchedule returns the effective schedule settings for the scheduler.
// This is used by the scheduler to determine if and when to run backups.
func (s *BackupService) GetEffectiveSchedule(ctx context.Context) (enabled bool, cron string, retention int) {
	info := s.GetScheduleInfo(ctx)
	return info.Enabled, info.Cron, info.Retention
}

// UpdateScheduleSettings updates the backup schedule settings in the database.
// Only non-nil values in the input will be updated.
func (s *BackupService) UpdateScheduleSettings(ctx context.Context, enabled *bool, cron *string, retention *int) (*models.BackupScheduleInfo, error) {
	// Validate cron if provided
	if cron != nil && *cron != "" {
		// Basic validation - ensure it has 6 fields
		fields := strings.Fields(*cron)
		if len(fields) != 6 {
			return nil, fmt.Errorf("invalid cron expression: must have 6 fields (sec min hour day month weekday)")
		}
	}

	// Validate retention if provided
	if retention != nil && *retention < 0 {
		return nil, fmt.Errorf("invalid retention: must be non-negative")
	}

	// Get or create the settings record (singleton, ID=1)
	var settings models.BackupSettings
	result := s.db.WithContext(ctx).First(&settings)
	if result.Error != nil {
		// Create new record
		settings = models.BackupSettings{ID: 1}
	}

	// Update fields if provided
	if enabled != nil {
		settings.Enabled = enabled
	}
	if cron != nil {
		settings.Cron = *cron
	}
	if retention != nil {
		settings.Retention = retention
	}

	// Save to database
	if err := s.db.WithContext(ctx).Save(&settings).Error; err != nil {
		return nil, fmt.Errorf("saving backup settings: %w", err)
	}

	s.logger.Info("backup settings updated",
		slog.Any("enabled", settings.Enabled),
		slog.String("cron", settings.Cron),
		slog.Any("retention", settings.Retention),
	)

	// Return the effective settings
	info := s.GetScheduleInfo(ctx)
	return &info, nil
}

// GetBackupDirectory returns the backup storage directory path.
func (s *BackupService) GetBackupDirectory() string {
	return s.storageDir
}

// CreateBackup creates a full database backup as a tar.gz archive containing
// both the database and metadata.
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
	tarGzPath := filepath.Join(s.storageDir, baseName+".tar.gz")

	// Check if backup with same name already exists (extremely rare with ms precision)
	if _, err := os.Stat(tarGzPath); err == nil {
		return nil, fmt.Errorf("backup already exists: %s", filepath.Base(tarGzPath))
	}

	// Use VACUUM INTO for consistent SQLite backup
	s.logger.Debug("creating backup using VACUUM INTO", slog.String("path", dbPath))
	if err := s.db.Exec("VACUUM INTO ?", dbPath).Error; err != nil {
		return nil, fmt.Errorf("vacuum into backup: %w", err)
	}
	defer os.Remove(dbPath) // Clean up temp database file

	// Get uncompressed size
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("stat backup db: %w", err)
	}
	uncompressedSize := dbInfo.Size()

	// Get table counts
	tableCounts, err := s.getTableCounts(ctx)
	if err != nil {
		s.logger.Warn("failed to get table counts", slog.String("error", err.Error()))
		tableCounts = make(map[string]int)
	}

	// Create metadata struct (checksum will be added after we know the archive size)
	metaFile := &models.BackupMetadataFile{
		TvarrVersion: version.Version,
		DatabaseSize: uncompressedSize,
		CreatedAt:    timestamp,
		TableCounts:  tableCounts,
	}

	// Create the tar.gz archive
	if err := s.createTarGzArchive(tarGzPath, dbPath, metaFile); err != nil {
		os.Remove(tarGzPath)
		return nil, fmt.Errorf("creating archive: %w", err)
	}

	// Get archive size and calculate checksum
	archiveInfo, err := os.Stat(tarGzPath)
	if err != nil {
		return nil, fmt.Errorf("stat archive: %w", err)
	}

	checksum, err := s.calculateChecksum(tarGzPath)
	if err != nil {
		return nil, fmt.Errorf("calculating checksum: %w", err)
	}

	// Update the archive with the checksum in metadata
	metaFile.CompressedSize = archiveInfo.Size()
	metaFile.Checksum = checksum
	if err := s.createTarGzArchive(tarGzPath, dbPath, metaFile); err != nil {
		os.Remove(tarGzPath)
		return nil, fmt.Errorf("updating archive with checksum: %w", err)
	}

	// Get final archive size (should be nearly identical)
	archiveInfo, _ = os.Stat(tarGzPath)

	// Build return metadata
	meta := &models.BackupMetadata{
		Filename:       filepath.Base(tarGzPath),
		FilePath:       tarGzPath,
		CreatedAt:      timestamp,
		FileSize:       archiveInfo.Size(),
		Checksum:       checksum,
		TvarrVersion:   version.Version,
		DatabaseSize:   uncompressedSize,
		CompressedSize: archiveInfo.Size(),
		TableCounts:    metaFile.ToTableCounts(),
	}

	s.logger.Info("backup created",
		slog.String("filename", meta.Filename),
		slog.Int64("size", meta.FileSize),
		slog.String("checksum", truncateChecksum(meta.Checksum)),
	)

	return meta, nil
}

// createTarGzArchive creates a tar.gz archive containing the database and metadata.
func (s *BackupService) createTarGzArchive(archivePath, dbPath string, meta *models.BackupMetadataFile) error {
	// Create the archive file
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(archiveFile)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Add database file to archive
	dbFile, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer dbFile.Close()

	dbInfo, err := dbFile.Stat()
	if err != nil {
		return fmt.Errorf("stat database: %w", err)
	}

	dbHeader := &tar.Header{
		Name:    backupDatabaseFile,
		Size:    dbInfo.Size(),
		Mode:    0644,
		ModTime: meta.CreatedAt,
	}
	if err := tarWriter.WriteHeader(dbHeader); err != nil {
		return fmt.Errorf("writing database header: %w", err)
	}
	if _, err := io.Copy(tarWriter, dbFile); err != nil {
		return fmt.Errorf("writing database content: %w", err)
	}

	// Add metadata file to archive
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	metaHeader := &tar.Header{
		Name:    backupMetadataFile,
		Size:    int64(len(metaJSON)),
		Mode:    0644,
		ModTime: meta.CreatedAt,
	}
	if err := tarWriter.WriteHeader(metaHeader); err != nil {
		return fmt.Errorf("writing metadata header: %w", err)
	}
	if _, err := tarWriter.Write(metaJSON); err != nil {
		return fmt.Errorf("writing metadata content: %w", err)
	}

	return nil
}


// ListBackups returns all available backups sorted by creation time (newest first).
// Supports both new .tar.gz format and legacy .db.gz format.
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
		// Support both new .tar.gz and legacy .db.gz formats
		if !strings.HasSuffix(entry.Name(), ".tar.gz") && !strings.HasSuffix(entry.Name(), ".db.gz") {
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
// Handles both new .tar.gz format and legacy .db.gz format with companion .meta.json.
func (s *BackupService) DeleteBackup(ctx context.Context, filename string) error {
	// Validate filename (prevent path traversal)
	if filepath.Base(filename) != filename {
		return fmt.Errorf("invalid filename")
	}

	backupPath := filepath.Join(s.storageDir, filename)

	// Remove backup file
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing backup file: %w", err)
	}

	// For legacy .db.gz files, also remove companion .meta.json if it exists
	if strings.HasSuffix(filename, ".db.gz") {
		metaPath := strings.TrimSuffix(backupPath, ".db.gz") + ".meta.json"
		if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to remove metadata file",
				slog.String("path", metaPath),
				slog.String("error", err.Error()),
			)
		}
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
// Supports both new .tar.gz format and legacy .db.gz format.
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

	// Verify checksum for legacy .db.gz format only
	// For .tar.gz format, skip checksum verification since the checksum is embedded
	// inside the archive - after embedding, the archive's actual checksum changes,
	// making verification impossible.
	if meta.Checksum != "" && strings.HasSuffix(filename, ".db.gz") {
		checksum, err := s.calculateChecksum(backupPath)
		if err != nil {
			return fmt.Errorf("calculating checksum: %w", err)
		}
		if checksum != meta.Checksum {
			return fmt.Errorf("checksum mismatch: backup may be corrupted")
		}
	}

	// Create pre-restore backup for rollback capability
	preRestoreBackup, err := s.CreateBackup(ctx)
	if err != nil {
		return fmt.Errorf("creating pre-restore backup: %w", err)
	}
	s.logger.Info("created pre-restore backup", slog.String("filename", preRestoreBackup.Filename))

	// Extract/decompress database to temp file
	tempDB, err := os.CreateTemp(s.storageDir, "restore-*.db")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tempPath := tempDB.Name()
	tempDB.Close()

	// Handle both formats
	if strings.HasSuffix(backupPath, ".tar.gz") {
		if err := s.extractDatabaseFromArchive(backupPath, tempPath); err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("extracting database from archive: %w", err)
		}
	} else {
		// Legacy .db.gz format
		if err := s.decompressFile(backupPath, tempPath); err != nil {
			os.Remove(tempPath)
			return fmt.Errorf("decompressing backup: %w", err)
		}
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

// CleanupOldBackups removes unprotected backups exceeding the retention limit.
// Protected backups are excluded from retention counting and are never deleted.
// Uses effective retention from database settings, falling back to config defaults.
func (s *BackupService) CleanupOldBackups(ctx context.Context) (int, error) {
	_, _, retention := s.GetEffectiveSchedule(ctx)
	if retention <= 0 {
		return 0, nil // No cleanup configured
	}

	backups, err := s.ListBackups(ctx)
	if err != nil {
		return 0, err
	}

	// Filter to only unprotected backups for retention calculation
	var unprotected []*models.BackupMetadata
	for _, b := range backups {
		if !b.Protected {
			unprotected = append(unprotected, b)
		}
	}

	if len(unprotected) <= retention {
		return 0, nil
	}

	// Delete oldest unprotected backups beyond retention limit
	deleted := 0
	for i := retention; i < len(unprotected); i++ {
		backup := unprotected[i]
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

// SetBackupProtection sets the protected status for a backup.
// Protected backups are excluded from retention cleanup.
// Supports both new .tar.gz format and legacy .db.gz format.
func (s *BackupService) SetBackupProtection(ctx context.Context, filename string, protected bool) error {
	// Validate filename (prevent path traversal)
	if filepath.Base(filename) != filename {
		return fmt.Errorf("invalid filename")
	}

	backupPath := filepath.Join(s.storageDir, filename)

	if strings.HasSuffix(filename, ".tar.gz") {
		// New format: update metadata inside the tar.gz archive
		return s.setProtectionInArchive(backupPath, protected)
	}

	// Legacy .db.gz format: update companion .meta.json file
	metaPath := strings.TrimSuffix(backupPath, ".db.gz") + ".meta.json"

	// Load existing metadata
	var metaFile models.BackupMetadataFile
	metaData, err := os.ReadFile(metaPath)
	if err == nil {
		if err := json.Unmarshal(metaData, &metaFile); err != nil {
			return fmt.Errorf("parsing metadata: %w", err)
		}
	} else if os.IsNotExist(err) {
		// Create minimal metadata if it doesn't exist
		metaFile = models.BackupMetadataFile{
			CreatedAt: parseTimestampFromFilename(filename),
		}
	} else {
		return fmt.Errorf("reading metadata: %w", err)
	}

	// Update protected status
	metaFile.Protected = protected

	// Write back
	metaJSON, err := json.MarshalIndent(metaFile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	s.logger.Info("backup protection updated",
		slog.String("filename", filename),
		slog.Bool("protected", protected),
	)

	return nil
}

// setProtectionInArchive updates the protected status in a tar.gz backup archive.
// This requires extracting the database, updating metadata, and recreating the archive.
func (s *BackupService) setProtectionInArchive(archivePath string, protected bool) error {
	// Read current metadata
	metaFile, err := s.readMetadataFromArchive(archivePath)
	if err != nil {
		return fmt.Errorf("reading metadata: %w", err)
	}

	// If protection status is already as requested, no need to update
	if metaFile.Protected == protected {
		return nil
	}

	// Extract database to temp file
	tempDB, err := os.CreateTemp(s.storageDir, "protection-update-*.db")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tempDBPath := tempDB.Name()
	tempDB.Close()
	defer os.Remove(tempDBPath)

	if err := s.extractDatabaseFromArchive(archivePath, tempDBPath); err != nil {
		return fmt.Errorf("extracting database: %w", err)
	}

	// Update metadata
	metaFile.Protected = protected

	// Create temp archive
	tempArchive, err := os.CreateTemp(s.storageDir, "protection-update-*.tar.gz")
	if err != nil {
		return fmt.Errorf("creating temp archive: %w", err)
	}
	tempArchivePath := tempArchive.Name()
	tempArchive.Close()
	defer os.Remove(tempArchivePath)

	// Create new archive with updated metadata
	if err := s.createTarGzArchive(tempArchivePath, tempDBPath, &metaFile); err != nil {
		return fmt.Errorf("creating archive: %w", err)
	}

	// Atomic replace: rename temp to original
	if err := os.Rename(tempArchivePath, archivePath); err != nil {
		return fmt.Errorf("replacing archive: %w", err)
	}

	s.logger.Info("backup protection updated",
		slog.String("filename", filepath.Base(archivePath)),
		slog.Bool("protected", protected),
	)

	return nil
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

	var metaFile models.BackupMetadataFile
	filename := filepath.Base(backupPath)

	// Determine format and load metadata accordingly
	if strings.HasSuffix(backupPath, ".tar.gz") {
		// New format: read metadata from inside tar.gz archive
		metaFile, err = s.readMetadataFromArchive(backupPath)
		if err != nil {
			s.logger.Warn("failed to read metadata from archive",
				slog.String("path", backupPath),
				slog.String("error", err.Error()),
			)
		}
	} else if strings.HasSuffix(backupPath, ".db.gz") {
		// Legacy format: read from companion .meta.json file
		metaPath := strings.TrimSuffix(backupPath, ".db.gz") + ".meta.json"
		metaData, err := os.ReadFile(metaPath)
		if err == nil {
			if err := json.Unmarshal(metaData, &metaFile); err != nil {
				s.logger.Warn("failed to parse metadata file",
					slog.String("path", metaPath),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	// Parse timestamp from filename if metadata doesn't have it
	createdAt := metaFile.CreatedAt
	if createdAt.IsZero() {
		createdAt = parseTimestampFromFilename(filename)
		if createdAt.IsZero() {
			createdAt = info.ModTime()
		}
	}

	// For legacy files without metadata, version is unknown
	tvarrVersion := metaFile.TvarrVersion
	if tvarrVersion == "" && strings.HasSuffix(backupPath, ".db.gz") {
		tvarrVersion = "unknown"
	}

	return &models.BackupMetadata{
		Filename:       filename,
		FilePath:       backupPath,
		CreatedAt:      createdAt,
		FileSize:       info.Size(),
		Checksum:       metaFile.Checksum,
		TvarrVersion:   tvarrVersion,
		DatabaseSize:   metaFile.DatabaseSize,
		CompressedSize: metaFile.CompressedSize,
		TableCounts:    metaFile.ToTableCounts(),
		Protected:      metaFile.Protected,
		Imported:       metaFile.Imported,
	}, nil
}

// readMetadataFromArchive reads the metadata.json file from inside a tar.gz archive.
func (s *BackupService) readMetadataFromArchive(archivePath string) (models.BackupMetadataFile, error) {
	var metaFile models.BackupMetadataFile

	file, err := os.Open(archivePath)
	if err != nil {
		return metaFile, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return metaFile, fmt.Errorf("opening gzip: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return metaFile, fmt.Errorf("reading tar: %w", err)
		}

		if header.Name == backupMetadataFile {
			metaData, err := io.ReadAll(tarReader)
			if err != nil {
				return metaFile, fmt.Errorf("reading metadata: %w", err)
			}
			if err := json.Unmarshal(metaData, &metaFile); err != nil {
				return metaFile, fmt.Errorf("parsing metadata: %w", err)
			}
			return metaFile, nil
		}
	}

	return metaFile, fmt.Errorf("metadata.json not found in archive")
}

// extractDatabaseFromArchive extracts the database.db file from a tar.gz archive to the specified path.
func (s *BackupService) extractDatabaseFromArchive(archivePath, destPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("opening gzip: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		if header.Name == backupDatabaseFile {
			destFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("creating destination file: %w", err)
			}
			defer destFile.Close()

			if _, err := io.Copy(destFile, tarReader); err != nil {
				return fmt.Errorf("extracting database: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("database.db not found in archive")
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
// Expected formats:
// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.tar.gz (new format with ms)
// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.tar.gz (new format)
// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.db.gz (legacy with ms)
// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz (legacy)
func parseTimestampFromFilename(filename string) time.Time {
	// Try new .tar.gz format with milliseconds first
	reTarMs := regexp.MustCompile(`tvarr-backup-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3})\.tar\.gz`)
	if matches := reTarMs.FindStringSubmatch(filename); len(matches) == 2 {
		if t, err := time.Parse("2006-01-02T15-04-05.000", matches[1]); err == nil {
			return t.UTC()
		}
	}

	// Try new .tar.gz format without milliseconds
	reTar := regexp.MustCompile(`tvarr-backup-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})\.tar\.gz`)
	if matches := reTar.FindStringSubmatch(filename); len(matches) == 2 {
		if t, err := time.Parse("2006-01-02T15-04-05", matches[1]); err == nil {
			return t.UTC()
		}
	}

	// Try legacy .db.gz format with milliseconds
	reDbMs := regexp.MustCompile(`tvarr-backup-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.\d{3})\.db\.gz`)
	if matches := reDbMs.FindStringSubmatch(filename); len(matches) == 2 {
		if t, err := time.Parse("2006-01-02T15-04-05.000", matches[1]); err == nil {
			return t.UTC()
		}
	}

	// Try legacy .db.gz format without milliseconds
	reDb := regexp.MustCompile(`tvarr-backup-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})\.db\.gz`)
	if matches := reDb.FindStringSubmatch(filename); len(matches) == 2 {
		if t, err := time.Parse("2006-01-02T15-04-05", matches[1]); err == nil {
			return t.UTC()
		}
	}

	return time.Time{}
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
// Supports both new .tar.gz format (with embedded metadata) and legacy .db.gz format.
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
		return nil, fmt.Errorf("invalid filename format: expected tvarr-backup-YYYY-MM-DDTHH-MM-SS.tar.gz or .db.gz")
	}

	// Check if backup with same name already exists
	destPath := filepath.Join(s.storageDir, originalFilename)
	if _, err := os.Stat(destPath); err == nil {
		return nil, fmt.Errorf("backup with filename %s already exists", originalFilename)
	}

	// Write uploaded file to temp location first
	tempFile, err := os.CreateTemp(s.storageDir, "upload-*")
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

	// Handle based on file format
	if strings.HasSuffix(originalFilename, ".tar.gz") {
		return s.importTarGzBackup(ctx, tempPath, destPath, originalFilename)
	}

	// Legacy .db.gz format
	return s.importLegacyBackup(ctx, tempPath, destPath, originalFilename)
}

// importTarGzBackup handles importing a new format .tar.gz backup.
func (s *BackupService) importTarGzBackup(ctx context.Context, tempPath, destPath, originalFilename string) (*models.BackupMetadata, error) {
	defer os.Remove(tempPath)

	// Validate the archive by trying to read its metadata
	metaFile, err := s.readMetadataFromArchive(tempPath)
	if err != nil {
		return nil, fmt.Errorf("invalid backup archive: %w", err)
	}

	// Extract and validate the database
	tempDBPath := tempPath + ".db"
	defer os.Remove(tempDBPath)

	if err := s.extractDatabaseFromArchive(tempPath, tempDBPath); err != nil {
		return nil, fmt.Errorf("extracting database: %w", err)
	}

	if err := s.validateDatabase(tempDBPath); err != nil {
		return nil, fmt.Errorf("validating database: %w", err)
	}

	// Move archive to final location
	if err := os.Rename(tempPath, destPath); err != nil {
		return nil, fmt.Errorf("moving backup to final location: %w", err)
	}

	// Get file info
	fileInfo, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("getting file info: %w", err)
	}

	// Mark as imported and protected if not already
	if !metaFile.Imported {
		metaFile.Imported = true
		metaFile.Protected = true
		// Update the archive with the imported flag
		if err := s.setProtectionInArchive(destPath, true); err != nil {
			s.logger.Warn("failed to update imported flag", slog.String("error", err.Error()))
		}
	}

	meta := &models.BackupMetadata{
		Filename:       originalFilename,
		FilePath:       destPath,
		CreatedAt:      metaFile.CreatedAt,
		FileSize:       fileInfo.Size(),
		Checksum:       metaFile.Checksum,
		TvarrVersion:   metaFile.TvarrVersion,
		DatabaseSize:   metaFile.DatabaseSize,
		CompressedSize: metaFile.CompressedSize,
		TableCounts:    metaFile.ToTableCounts(),
		Protected:      metaFile.Protected,
		Imported:       metaFile.Imported,
	}

	s.logger.Info("backup imported",
		slog.String("filename", meta.Filename),
		slog.Int64("size", meta.FileSize),
		slog.String("version", meta.TvarrVersion),
		slog.Bool("protected", meta.Protected),
	)

	return meta, nil
}

// importLegacyBackup handles importing a legacy .db.gz backup.
func (s *BackupService) importLegacyBackup(ctx context.Context, tempPath, destPath, originalFilename string) (*models.BackupMetadata, error) {
	defer os.Remove(tempPath)

	// Verify the uploaded file is a valid gzipped SQLite database
	if err := s.validateGzippedBackup(tempPath); err != nil {
		return nil, fmt.Errorf("validating backup: %w", err)
	}

	// Move temp file to final location
	if err := os.Rename(tempPath, destPath); err != nil {
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

	// Create metadata file - imported backups are protected by default
	metaPath := strings.TrimSuffix(destPath, ".db.gz") + ".meta.json"
	metaFile := &models.BackupMetadataFile{
		TvarrVersion:   "unknown", // Legacy imports don't have version info
		CompressedSize: fileInfo.Size(),
		Checksum:       checksum,
		CreatedAt:      createdAt,
		TableCounts:    make(map[string]int),
		Protected:      true,
		Imported:       true,
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
		Protected:      metaFile.Protected,
		Imported:       metaFile.Imported,
	}

	s.logger.Info("legacy backup imported",
		slog.String("filename", meta.Filename),
		slog.Int64("size", meta.FileSize),
		slog.Bool("protected", meta.Protected),
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
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.tar.gz (35 chars, new format)
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.tar.gz (39 chars, new format with ms)
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz (34 chars, legacy)
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.db.gz (38 chars, legacy with ms)
	if len(filename) < 34 { // Minimum length for valid filename
		return false
	}

	// Must start with tvarr-backup-
	prefix := "tvarr-backup-"
	if !strings.HasPrefix(filename, prefix) {
		return false
	}

	// Must end with .tar.gz or .db.gz
	if !strings.HasSuffix(filename, ".tar.gz") && !strings.HasSuffix(filename, ".db.gz") {
		return false
	}

	// Validate timestamp portion can be parsed
	ts := parseTimestampFromFilename(filename)
	return !ts.IsZero()
}
