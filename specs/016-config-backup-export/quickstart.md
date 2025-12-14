# Quickstart: Config Export/Import & Backup System

**Date**: 2025-12-14
**Feature**: 016-config-backup-export
**Spec**: [spec.md](./spec.md)

## Overview

This guide provides implementation patterns and examples for the config export/import and backup system.

## Directory Structure

```
internal/
├── config/
│   └── config.go              # Add BackupConfig struct
├── service/
│   ├── export_service.go      # NEW: Config export logic
│   ├── import_service.go      # NEW: Config import logic
│   └── backup_service.go      # NEW: Database backup/restore
├── http/handlers/
│   ├── export.go              # NEW: Export endpoints
│   ├── import.go              # NEW: Import endpoints
│   └── backup.go              # NEW: Backup endpoints
├── scheduler/
│   └── executor.go            # Add BackupHandler
└── models/
    └── export.go              # NEW: Export/import data structures
```

## Implementation Patterns

### 1. Export Service

```go
// internal/service/export_service.go

package service

import (
    "context"
    "encoding/json"
    "time"

    "github.com/jmylchreest/tvarr/internal/models"
    "github.com/jmylchreest/tvarr/internal/repository"
    "github.com/jmylchreest/tvarr/internal/version"
)

type ExportService struct {
    filterRepo          repository.FilterRepository
    dataMappingRuleRepo repository.DataMappingRuleRepository
    clientDetectionRepo repository.ClientDetectionRuleRepository
    encodingProfileRepo repository.EncodingProfileRepository
}

func NewExportService(
    filterRepo repository.FilterRepository,
    dataMappingRuleRepo repository.DataMappingRuleRepository,
    clientDetectionRepo repository.ClientDetectionRuleRepository,
    encodingProfileRepo repository.EncodingProfileRepository,
) *ExportService {
    return &ExportService{
        filterRepo:          filterRepo,
        dataMappingRuleRepo: dataMappingRuleRepo,
        clientDetectionRepo: clientDetectionRepo,
        encodingProfileRepo: encodingProfileRepo,
    }
}

// ExportFilters exports selected filters or all user-created filters.
func (s *ExportService) ExportFilters(ctx context.Context, ids []models.ULID, exportAll bool) (*models.ConfigExport, error) {
    var filters []*models.Filter
    var err error

    if exportAll || len(ids) == 0 {
        // Get all user-created filters (IsSystem = false)
        filters, err = s.filterRepo.GetUserCreated(ctx)
    } else {
        // Get specific filters by ID
        filters, err = s.filterRepo.GetByIDs(ctx, ids)
    }
    if err != nil {
        return nil, err
    }

    // Filter out system filters if specific IDs were provided
    userFilters := make([]*models.Filter, 0, len(filters))
    for _, f := range filters {
        if !f.IsSystem {
            userFilters = append(userFilters, f)
        }
    }

    // Convert to export items
    items := make([]models.FilterExportItem, len(userFilters))
    for i, f := range userFilters {
        items[i] = models.FilterExportItem{
            Name:        f.Name,
            Description: f.Description,
            Expression:  f.Expression,
            SourceType:  string(f.SourceType),
            Action:      string(f.Action),
            IsEnabled:   models.BoolVal(f.IsEnabled),
            SourceID:    ulidPtrToStringPtr(f.SourceID),
        }
    }

    return &models.ConfigExport{
        Metadata: models.ExportMetadata{
            Version:      "1.0.0",
            TvarrVersion: version.Version,
            ExportType:   "filters",
            ExportedAt:   time.Now().UTC(),
            ItemCount:    len(items),
        },
        Items: items,
    }, nil
}

// Helper function
func ulidPtrToStringPtr(id *models.ULID) *string {
    if id == nil {
        return nil
    }
    s := id.String()
    return &s
}
```

### 2. Import Service with Conflict Resolution

```go
// internal/service/import_service.go

package service

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/jmylchreest/tvarr/internal/models"
    "github.com/jmylchreest/tvarr/internal/repository"
    "gorm.io/gorm"
)

type ImportService struct {
    db                  *gorm.DB
    filterRepo          repository.FilterRepository
    // ... other repos
}

// ConflictResolution defines how to handle a name conflict.
type ConflictResolution string

const (
    ResolutionSkip      ConflictResolution = "skip"
    ResolutionRename    ConflictResolution = "rename"
    ResolutionOverwrite ConflictResolution = "overwrite"
)

// ImportFiltersPreview returns a preview of what will happen on import.
func (s *ImportService) ImportFiltersPreview(ctx context.Context, export *models.ConfigExport) (*models.ImportPreview, error) {
    items, ok := export.Items.([]models.FilterExportItem)
    if !ok {
        return nil, fmt.Errorf("invalid export items type")
    }

    preview := &models.ImportPreview{
        TotalItems: len(items),
        NewItems:   make([]models.PreviewItem, 0),
        Conflicts:  make([]models.ConflictItem, 0),
        Errors:     make([]models.ImportError, 0),
    }

    // Check each item for conflicts
    for _, item := range items {
        // Validate expression syntax
        if err := validateExpression(item.Expression); err != nil {
            preview.Errors = append(preview.Errors, models.ImportError{
                ItemName: item.Name,
                Error:    err.Error(),
            })
            continue
        }

        // Check for name conflict
        existing, err := s.filterRepo.GetByName(ctx, item.Name)
        if err != nil && err != gorm.ErrRecordNotFound {
            preview.Errors = append(preview.Errors, models.ImportError{
                ItemName: item.Name,
                Error:    err.Error(),
            })
            continue
        }

        if existing != nil {
            preview.Conflicts = append(preview.Conflicts, models.ConflictItem{
                ImportName:   item.Name,
                ExistingID:   existing.ID.String(),
                ExistingName: existing.Name,
                Resolution:   string(ResolutionSkip), // Default
            })
        } else {
            preview.NewItems = append(preview.NewItems, models.PreviewItem{
                Name: item.Name,
            })
        }
    }

    return preview, nil
}

// ImportFilters imports filters with specified conflict resolutions.
func (s *ImportService) ImportFilters(
    ctx context.Context,
    export *models.ConfigExport,
    resolutions map[string]ConflictResolution,
) (*models.ImportResult, error) {
    items, ok := export.Items.([]models.FilterExportItem)
    if !ok {
        return nil, fmt.Errorf("invalid export items type")
    }

    result := &models.ImportResult{
        TotalItems:    len(items),
        ImportedItems: make([]models.ImportedItem, 0),
    }

    // Use transaction for atomicity
    err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        for _, item := range items {
            imported, err := s.importSingleFilter(ctx, tx, item, resolutions)
            if err != nil {
                result.Errors++
                result.ErrorDetails = append(result.ErrorDetails, models.ImportError{
                    ItemName: item.Name,
                    Error:    err.Error(),
                })
                continue
            }

            if imported != nil {
                result.ImportedItems = append(result.ImportedItems, *imported)
                switch imported.Action {
                case "created":
                    result.Imported++
                case "overwritten":
                    result.Overwritten++
                case "renamed":
                    result.Renamed++
                }
            } else {
                result.Skipped++
            }
        }

        // Rollback if any errors occurred (atomic import)
        if result.Errors > 0 {
            return fmt.Errorf("import failed with %d errors", result.Errors)
        }

        return nil
    })

    if err != nil {
        return result, err
    }

    return result, nil
}

func (s *ImportService) importSingleFilter(
    ctx context.Context,
    tx *gorm.DB,
    item models.FilterExportItem,
    resolutions map[string]ConflictResolution,
) (*models.ImportedItem, error) {
    // Check for existing filter with same name
    var existing models.Filter
    err := tx.Where("name = ?", item.Name).First(&existing).Error

    if err == nil {
        // Conflict exists - check resolution
        resolution := resolutions[item.Name]
        if resolution == "" {
            resolution = ResolutionSkip
        }

        switch resolution {
        case ResolutionSkip:
            return nil, nil // Skip this item

        case ResolutionRename:
            // Find unique name
            newName := findUniqueName(tx, item.Name)
            filter := filterFromExportItem(item)
            filter.Name = newName
            if err := tx.Create(&filter).Error; err != nil {
                return nil, err
            }
            return &models.ImportedItem{
                OriginalName: item.Name,
                FinalName:    newName,
                ID:           filter.ID.String(),
                Action:       "renamed",
            }, nil

        case ResolutionOverwrite:
            // Update existing record, preserving ID
            updates := filterFromExportItem(item)
            if err := tx.Model(&existing).Updates(updates).Error; err != nil {
                return nil, err
            }
            return &models.ImportedItem{
                OriginalName: item.Name,
                FinalName:    item.Name,
                ID:           existing.ID.String(),
                Action:       "overwritten",
            }, nil
        }
    } else if err == gorm.ErrRecordNotFound {
        // No conflict - create new
        filter := filterFromExportItem(item)
        filter.ID = models.NewULID()
        if err := tx.Create(&filter).Error; err != nil {
            return nil, err
        }
        return &models.ImportedItem{
            OriginalName: item.Name,
            FinalName:    item.Name,
            ID:           filter.ID.String(),
            Action:       "created",
        }, nil
    }

    return nil, err
}

func findUniqueName(tx *gorm.DB, baseName string) string {
    suffix := 1
    for {
        newName := fmt.Sprintf("%s (%d)", baseName, suffix)
        var count int64
        tx.Model(&models.Filter{}).Where("name = ?", newName).Count(&count)
        if count == 0 {
            return newName
        }
        suffix++
    }
}

func filterFromExportItem(item models.FilterExportItem) models.Filter {
    enabled := item.IsEnabled
    return models.Filter{
        Name:        item.Name,
        Description: item.Description,
        Expression:  item.Expression,
        SourceType:  models.FilterSourceType(item.SourceType),
        Action:      models.FilterAction(item.Action),
        IsEnabled:   &enabled,
        IsSystem:    false, // Always user-created on import
    }
}
```

### 3. Backup Service with SQLite VACUUM INTO

```go
// internal/service/backup_service.go

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
    "sort"
    "strings"
    "time"

    "github.com/jmylchreest/tvarr/internal/config"
    "github.com/jmylchreest/tvarr/internal/database"
    "github.com/jmylchreest/tvarr/internal/version"
)

type BackupService struct {
    db        *database.DB
    cfg       config.BackupConfig
    storageDir string
    logger    *slog.Logger
}

func NewBackupService(db *database.DB, cfg config.BackupConfig, storageBaseDir string, logger *slog.Logger) *BackupService {
    backupDir := cfg.Directory
    if backupDir == "" {
        backupDir = filepath.Join(storageBaseDir, "backups")
    }

    return &BackupService{
        db:         db,
        cfg:        cfg,
        storageDir: backupDir,
        logger:     logger,
    }
}

// CreateBackup creates a full database backup.
func (s *BackupService) CreateBackup(ctx context.Context) (*BackupMetadata, error) {
    // Ensure backup directory exists
    if err := os.MkdirAll(s.storageDir, 0755); err != nil {
        return nil, fmt.Errorf("creating backup directory: %w", err)
    }

    // Generate timestamp-based filename
    timestamp := time.Now().UTC()
    baseName := fmt.Sprintf("tvarr-backup-%s", timestamp.Format("2006-01-02T15-04-05"))
    dbPath := filepath.Join(s.storageDir, baseName+".db")
    gzPath := filepath.Join(s.storageDir, baseName+".db.gz")
    metaPath := filepath.Join(s.storageDir, baseName+".meta.json")

    // Use VACUUM INTO for consistent SQLite backup
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

    // Create metadata
    meta := &BackupMetadata{
        Filename:       filepath.Base(gzPath),
        FilePath:       gzPath,
        CreatedAt:      timestamp,
        FileSize:       gzInfo.Size(),
        DatabaseSize:   uncompressedSize,
        Checksum:       checksum,
        TvarrVersion:   version.Version,
        TableCounts:    tableCounts,
    }

    // Write metadata file
    metaJSON, err := json.MarshalIndent(meta, "", "  ")
    if err != nil {
        return nil, fmt.Errorf("marshaling metadata: %w", err)
    }
    if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
        return nil, fmt.Errorf("writing metadata: %w", err)
    }

    s.logger.Info("backup created",
        slog.String("filename", meta.Filename),
        slog.Int64("size", meta.FileSize),
        slog.String("checksum", meta.Checksum[:16]+"..."),
    )

    return meta, nil
}

// ListBackups returns all available backups sorted by creation time (newest first).
func (s *BackupService) ListBackups(ctx context.Context) ([]*BackupMetadata, error) {
    entries, err := os.ReadDir(s.storageDir)
    if err != nil {
        if os.IsNotExist(err) {
            return []*BackupMetadata{}, nil
        }
        return nil, err
    }

    var backups []*BackupMetadata
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

// RestoreBackup restores the database from a backup file.
func (s *BackupService) RestoreBackup(ctx context.Context, filename string) error {
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

    // Create pre-restore backup
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

    // Get current database path from config
    currentDBPath := s.db.DSN()
    if !filepath.IsAbs(currentDBPath) {
        currentDBPath, _ = filepath.Abs(currentDBPath)
    }

    // Close current database connections
    // Note: This requires coordination with the application
    // Implementation depends on how the app manages DB lifecycle

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

    return deleted, nil
}

// Helper methods

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
    tables := []string{"channels", "epg_programs", "filters", "data_mapping_rules",
        "client_detection_rules", "encoding_profiles", "sources", "stream_proxies"}

    for _, table := range tables {
        var count int64
        if err := s.db.WithContext(ctx).Table(table).Count(&count).Error; err != nil {
            continue // Skip tables that don't exist
        }
        counts[table] = int(count)
    }

    return counts, nil
}
```

### 4. HTTP Handler Pattern

```go
// internal/http/handlers/backup.go

package handlers

import (
    "context"
    "io"
    "net/http"
    "path/filepath"

    "github.com/danielgtaylor/huma/v2"
    "github.com/go-chi/chi/v5"
    "github.com/jmylchreest/tvarr/internal/service"
)

type BackupHandler struct {
    backupService *service.BackupService
}

func NewBackupHandler(backupService *service.BackupService) *BackupHandler {
    return &BackupHandler{backupService: backupService}
}

func (h *BackupHandler) Register(api huma.API) {
    huma.Register(api, huma.Operation{
        OperationID: "listBackups",
        Method:      "GET",
        Path:        "/api/v1/backups",
        Summary:     "List available backups",
        Tags:        []string{"Backup"},
    }, h.ListBackups)

    huma.Register(api, huma.Operation{
        OperationID: "createBackup",
        Method:      "POST",
        Path:        "/api/v1/backups",
        Summary:     "Create a new backup",
        Tags:        []string{"Backup"},
    }, h.CreateBackup)

    // Note: Download uses Chi directly for streaming response
}

// RegisterFileServer registers direct Chi routes for file operations.
func (h *BackupHandler) RegisterFileServer(router *chi.Mux) {
    router.Get("/api/v1/backups/{filename}", h.DownloadBackup)
    router.Delete("/api/v1/backups/{filename}", h.DeleteBackupDirect)
    router.Post("/api/v1/backups/{filename}/restore", h.RestoreBackupDirect)
}

// DownloadBackup streams the backup file to the client.
func (h *BackupHandler) DownloadBackup(w http.ResponseWriter, r *http.Request) {
    filename := chi.URLParam(r, "filename")

    // Validate filename (prevent path traversal)
    if filepath.Base(filename) != filename {
        http.Error(w, "invalid filename", http.StatusBadRequest)
        return
    }

    file, err := h.backupService.OpenBackupFile(r.Context(), filename)
    if err != nil {
        http.Error(w, "backup not found", http.StatusNotFound)
        return
    }
    defer file.Close()

    w.Header().Set("Content-Type", "application/gzip")
    w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

    io.Copy(w, file)
}

// CreateBackup handler implementation
type CreateBackupInput struct{}
type CreateBackupOutput struct {
    Body BackupResponse
}

type BackupResponse struct {
    Filename     string         `json:"filename"`
    CreatedAt    string         `json:"created_at"`
    FileSize     int64          `json:"file_size"`
    DatabaseSize int64          `json:"database_size"`
    Checksum     string         `json:"checksum"`
    TvarrVersion string         `json:"tvarr_version"`
    TableCounts  map[string]int `json:"table_counts"`
}

func (h *BackupHandler) CreateBackup(ctx context.Context, input *CreateBackupInput) (*CreateBackupOutput, error) {
    meta, err := h.backupService.CreateBackup(ctx)
    if err != nil {
        return nil, huma.Error500InternalServerError("Failed to create backup: " + err.Error())
    }

    return &CreateBackupOutput{
        Body: BackupResponse{
            Filename:     meta.Filename,
            CreatedAt:    meta.CreatedAt.Format(time.RFC3339),
            FileSize:     meta.FileSize,
            DatabaseSize: meta.DatabaseSize,
            Checksum:     meta.Checksum,
            TvarrVersion: meta.TvarrVersion,
            TableCounts:  meta.TableCounts,
        },
    }, nil
}
```

### 5. Scheduled Backup Job

```go
// internal/scheduler/executor.go (add to existing file)

// BackupHandler handles scheduled backup jobs.
type BackupHandler struct {
    backupService *service.BackupService
    logger        *slog.Logger
}

func NewBackupHandler(backupService *service.BackupService, logger *slog.Logger) *BackupHandler {
    return &BackupHandler{
        backupService: backupService,
        logger:        logger,
    }
}

func (h *BackupHandler) Execute(ctx context.Context, job *models.Job) (string, error) {
    h.logger.Info("starting scheduled backup")

    // Create backup
    meta, err := h.backupService.CreateBackup(ctx)
    if err != nil {
        return "", fmt.Errorf("backup failed: %w", err)
    }

    // Cleanup old backups based on retention
    deleted, err := h.backupService.CleanupOldBackups(ctx)
    if err != nil {
        h.logger.Warn("failed to cleanup old backups", slog.String("error", err.Error()))
    }

    result := fmt.Sprintf("Backup created: %s (size: %d bytes)", meta.Filename, meta.FileSize)
    if deleted > 0 {
        result += fmt.Sprintf(", deleted %d old backups", deleted)
    }

    h.logger.Info("scheduled backup completed",
        slog.String("filename", meta.Filename),
        slog.Int("deleted", deleted),
    )

    return result, nil
}
```

## Testing Patterns

### Export Service Test

```go
func TestExportFilters_ExcludesSystemFilters(t *testing.T) {
    // Setup
    mockRepo := mocks.NewMockFilterRepository(t)
    service := NewExportService(mockRepo, nil, nil, nil)

    filters := []*models.Filter{
        {Name: "User Filter", IsSystem: false},
        {Name: "System Filter", IsSystem: true},
    }

    mockRepo.EXPECT().GetUserCreated(mock.Anything).Return(filters, nil)

    // Execute
    export, err := service.ExportFilters(context.Background(), nil, true)

    // Assert
    require.NoError(t, err)
    items := export.Items.([]FilterExportItem)
    assert.Len(t, items, 1)
    assert.Equal(t, "User Filter", items[0].Name)
}
```

### Import Service Test with Conflicts

```go
func TestImportFilters_OverwriteConflict(t *testing.T) {
    // Setup
    db := setupTestDB(t)
    service := NewImportService(db, ...)

    // Create existing filter
    existing := &models.Filter{
        ID:   models.NewULID(),
        Name: "Existing Filter",
        Expression: "old expression",
    }
    db.Create(existing)

    // Import with overwrite resolution
    export := &models.ConfigExport{
        Items: []FilterExportItem{{
            Name:       "Existing Filter",
            Expression: "new expression",
        }},
    }
    resolutions := map[string]ConflictResolution{
        "Existing Filter": ResolutionOverwrite,
    }

    // Execute
    result, err := service.ImportFilters(context.Background(), export, resolutions)

    // Assert
    require.NoError(t, err)
    assert.Equal(t, 1, result.Overwritten)

    // Verify update
    var updated models.Filter
    db.First(&updated, "id = ?", existing.ID)
    assert.Equal(t, "new expression", updated.Expression)
}
```

## Configuration Example

```yaml
# config.yaml
backup:
  directory: "/data/backups"  # Optional, defaults to {storage.base_dir}/backups
  schedule:
    enabled: true
    cron: "0 0 2 * * *"       # Daily at 2 AM
    retention: 7              # Keep last 7 backups
```

## Key Implementation Notes

1. **SQLite Backup**: Use `VACUUM INTO` for consistent, atomic backups without CGO
2. **Atomicity**: All imports wrapped in transactions for all-or-nothing behavior
3. **Conflict Resolution**: Name-based matching with Skip/Rename/Overwrite options
4. **File Handling**: Follow existing patterns from `OutputHandler` and `LogoHandler`
5. **Scheduled Jobs**: Extend existing scheduler with new `backup` job type
6. **Retention**: Automatic cleanup of old backups after scheduled backup completes
