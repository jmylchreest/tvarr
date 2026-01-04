# Research: Config Export/Import & Backup System

**Date**: 2025-12-14
**Feature**: 016-config-backup-export
**Spec**: [spec.md](./spec.md)

## Executive Summary

This research document covers the technical decisions for implementing config export/import and full database backup/restore functionality for tvarr. The system will support:
1. Selective export/import of user configuration items (filters, data mapping rules, client detection rules, encoding profiles)
2. Full SQLite database backup with gzip compression
3. Scheduled automatic backups with retention policies

## Research Findings

### 1. SQLite Backup Strategy

**Decision**: Use SQLite's VACUUM INTO command for consistent, atomic backups

**Rationale**:
- `VACUUM INTO 'backup.db'` creates a complete, defragmented copy of the database in a single atomic operation
- Works correctly with WAL mode (tvarr uses WAL by default)
- No need for external dependencies or CGO SQLite bindings
- Available in SQLite 3.27.0+ (widely available)
- Alternative `backup` API requires CGO bindings which tvarr avoids

**Alternatives Considered**:
1. **SQLite Online Backup API** - Rejected: requires `database/sql` level access or CGO, complex for GORM setup
2. **File copy with PRAGMA wal_checkpoint** - Rejected: requires coordination, risk of corruption during WAL replay
3. **Raw database file copy** - Rejected: unsafe during active writes, WAL files must be handled
4. **gorm-sqlite backup extension** - Rejected: no stable GORM v2 compatible library found

**Implementation Approach**:
```go
// Execute VACUUM INTO to create consistent backup
db.Exec("VACUUM INTO ?", backupPath)

// Then gzip compress the backup file
```

### 2. Export File Format

**Decision**: Use JSON format with metadata envelope

**Rationale**:
- Human-readable and editable (per FR-005, SC-005)
- Standard format widely supported
- Easy to validate and parse
- Supports nested structures for complex entities

**Export Structure**:
```json
{
  "metadata": {
    "version": "1.0.0",
    "tvarr_version": "1.2.3",
    "export_type": "filters",
    "exported_at": "2025-12-14T10:30:00Z",
    "item_count": 5
  },
  "items": [
    {
      "name": "Sports Filter",
      "expression": "group CONTAINS 'Sports'",
      "source_type": "stream",
      "action": "include",
      "is_enabled": true
      // ... other fields
    }
  ]
}
```

**Alternatives Considered**:
1. **YAML** - Rejected: less portable, more complex parsing
2. **Protocol Buffers** - Rejected: not human-readable
3. **SQLite dump** - Rejected: only for full backups, not selective export

### 3. Conflict Resolution Strategy

**Decision**: Three-option conflict resolution (Skip, Rename, Overwrite) with name-based matching

**Rationale**:
- Name conflicts are most common (users share configs with similar naming)
- ID conflicts cannot happen (new IDs generated on import per FR-006)
- Provides user control over import behavior (per FR-007, FR-008)
- Overwrite preserves original ID for reference integrity

**Implementation**:
1. Parse import file and validate
2. For each item, check if name exists in database
3. If conflict, apply user-selected resolution:
   - **Skip**: Don't import the item
   - **Rename**: Add suffix (e.g., "Filter Name (1)")
   - **Overwrite**: Update existing record with imported values, preserving original ID

### 4. Backup Scheduling Mechanism

**Decision**: Extend existing job scheduler with new job type `backup`

**Rationale**:
- tvarr already has a robust job scheduler (`internal/scheduler/`)
- Uses `robfig/cron/v3` for cron expressions
- Job model already supports cron scheduling, retries, history tracking
- Consistent with existing patterns (stream_ingestion, epg_ingestion, etc.)

**New Job Type**:
```go
const JobTypeBackup JobType = "backup"
```

**Backup Job Configuration**:
- Stored in config under `backup.schedule` section
- Cron expression for frequency
- Retention count for cleanup policy

### 5. Backup File Storage

**Decision**: Store in configurable directory `backup.directory` with timestamped naming

**Rationale**:
- Timestamp naming enables natural sorting (per spec assumptions)
- Consistent with storage patterns (`storage.base_dir/...`)
- Sandboxed access prevents path traversal
- gzip compression reduces storage footprint

**File Naming**: `tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz`

**Default Location**: `{storage.base_dir}/backups/` (e.g., `./data/backups/`)

### 6. Export/Import Atomicity

**Decision**: Use database transactions for import atomicity

**Rationale**:
- All-or-nothing import (per spec assumptions)
- GORM transaction support is well-established in codebase
- Rollback on any error prevents partial imports

**Implementation**:
```go
db.Transaction(func(tx *gorm.DB) error {
    for _, item := range items {
        if err := tx.Create(&item).Error; err != nil {
            return err // Auto-rollback
        }
    }
    return nil // Auto-commit
})
```

### 7. Restore Safety

**Decision**: Pre-restore database backup with atomic swap

**Rationale**:
- FR-020 requires rollback capability
- Creating backup before restore enables recovery
- Atomic file operations minimize window for corruption

**Restore Process**:
1. Verify backup file integrity (checksum)
2. Create pre-restore backup of current database
3. Decompress backup to temp location
4. Validate restored database can be opened
5. Close active connections (with warning if streams active)
6. Atomic rename: current DB → old, restored → current
7. Reconnect and verify
8. If any step fails, rollback using pre-restore backup

### 8. Exportable Entity Identification

**Decision**: Use `IsSystem` boolean field to identify system vs user items

**Rationale**:
- All target entities (Filter, DataMappingRule, ClientDetectionRule, EncodingProfile) have `IsSystem bool` field
- FR-018 requires excluding system-provided defaults
- Simple query filter: `WHERE is_system = false`

**Entities with IsSystem**:
- `filters.is_system` - marks system filters
- `data_mapping_rules.is_system` - marks system rules
- `client_detection_rules.is_system` - marks system rules (priority 100-999)
- `encoding_profiles.is_system` - marks system profiles (H.264/AAC Universal, etc.)

### 9. Frontend File Download Pattern

**Decision**: Use standard HTTP response with Content-Disposition header

**Rationale**:
- Existing pattern in `OutputHandler` (output.go:167-168)
- Works with browser download mechanisms
- Supports both JSON exports and binary backups

**Implementation Pattern**:
```go
w.Header().Set("Content-Type", "application/json")
w.Header().Set("Content-Disposition", `attachment; filename="filters-export.json"`)
w.Write(data)
```

For binary backups:
```go
w.Header().Set("Content-Type", "application/gzip")
w.Header().Set("Content-Disposition", `attachment; filename="tvarr-backup-2025-12-14T10-30-00.db.gz"`)
io.Copy(w, file)
```

### 10. Frontend File Upload Pattern

**Decision**: Use multipart/form-data with Huma file handling

**Rationale**:
- Existing pattern in `LogoHandler.UploadLogo` (logo.go:667-723)
- Huma supports `multipart.Form` in request body
- Validates file before processing

**Implementation Pattern**:
```go
type ImportInput struct {
    RawBody multipart.Form
}

func (h *Handler) Import(ctx context.Context, input *ImportInput) (*ImportOutput, error) {
    files := input.RawBody.File["file"]
    // ... validate and process
}
```

## Technology Decisions

| Component | Technology | Justification |
|-----------|------------|---------------|
| Backup Method | VACUUM INTO + gzip | Atomic, consistent, no CGO required |
| Export Format | JSON | Human-readable, widely supported |
| Scheduling | robfig/cron/v3 | Already in use, proven |
| File Storage | Local filesystem | Simple, sandboxed, configurable |
| Transaction | GORM transactions | Existing pattern, atomic operations |

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Backup during active writes | Medium | Low | VACUUM INTO handles WAL correctly |
| Restore interrupts streams | High | Medium | Warn user, require confirmation |
| Disk space exhaustion | Low | High | Check space before backup, log warning |
| Import version mismatch | Medium | Medium | Include version in export, warn if different |
| Large export file handling | Low | Low | Stream writes, pagination for large datasets |

## Unknowns Resolved

1. **SQLite backup method**: VACUUM INTO (no CGO required)
2. **Export format**: JSON with metadata envelope
3. **Conflict detection**: Name-based matching
4. **Scheduling**: Extend existing job scheduler
5. **Storage location**: Configurable `backup.directory`
6. **Restore safety**: Pre-restore backup with atomic swap

## Next Steps

1. Generate data model for backup/export entities
2. Define API contracts for export/import/backup endpoints
3. Create implementation tasks
