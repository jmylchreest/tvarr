# Implementation Plan: Config Export/Import & Backup System

**Branch**: `016-config-backup-export` | **Date**: 2025-12-14 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/016-config-backup-export/spec.md`

## Summary

Implement a configuration export/import system allowing users to share filters, data mapping rules, client detection rules, and encoding profiles as portable JSON files. Additionally, provide full SQLite database backup/restore with scheduled automatic backups and retention policies.

**Technical Approach**:
- JSON-based export format with metadata envelope for config items
- SQLite `VACUUM INTO` for atomic, consistent database backups (no CGO required)
- gzip compression for backup storage efficiency
- Extend existing job scheduler for automated backups
- GORM transactions for atomic import operations
- Name-based conflict detection with Skip/Rename/Overwrite resolution

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, robfig/cron/v3, compress/gzip
**Storage**: SQLite (primary), PostgreSQL/MySQL (GORM-compatible, backup via API for SQLite only)
**Testing**: testify + gomock (table-driven tests)
**Target Platform**: Linux server, Docker container, cross-platform
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**:
- Export/import < 30s for up to 100 items (SC-001)
- Full backup < 2 min for databases up to 1GB (SC-002)
- Restore < 5 min for databases up to 1GB (SC-003)
**Constraints**:
- SQLite backup only (PostgreSQL/MySQL out of scope)
- Local filesystem backup storage only
- No encryption of backups
**Scale/Scope**:
- Up to 100k channels, 1M EPG entries in database
- Typical config: ~50 filters, ~20 mapping rules, ~10 profiles

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Export/import uses streaming JSON, backups use file-based operations |
| II. Modular Pipeline Architecture | PASS | Services separated (ExportService, ImportService, BackupService) |
| III. Test-First Development | REQUIRED | Tests must be written before implementation |
| IV. Clean Architecture with SOLID | PASS | Repository pattern for DB, service layer for business logic |
| V. Idiomatic Go | REQUIRED | Follow standard Go conventions |
| VI. Observable and Debuggable | PASS | slog logging throughout, no emojis |
| VII. Security by Default | PASS | Sandbox file operations, path traversal prevention |
| VIII. No Magic Strings | REQUIRED | Use constants for export versions, job types |
| IX. Resilient HTTP Clients | N/A | No external HTTP calls for this feature |
| X. Human-Readable Duration | PASS | Use pkg/duration for backup retention config |
| XI. Human-Readable Byte Size | N/A | Not applicable |
| XII. Production-Grade CI/CD | N/A | Uses existing CI/CD |
| XIII. Test Data Standards | REQUIRED | Use fictional names in tests |

**Post-Design Re-Check**: All gates pass. No complexity violations.

## Project Structure

### Documentation (this feature)

```text
specs/016-config-backup-export/
├── plan.md              # This file
├── research.md          # Phase 0: Technical decisions
├── data-model.md        # Phase 1: Data structures
├── quickstart.md        # Phase 1: Implementation patterns
├── contracts/           # Phase 1: API contracts
│   └── openapi.yaml     # OpenAPI 3.1 specification
└── tasks.md             # Phase 2: Implementation tasks (TBD)
```

### Source Code (repository root)

```text
internal/
├── config/
│   └── config.go                    # MODIFY: Add BackupConfig struct
├── models/
│   └── export.go                    # NEW: Export/import data structures
├── service/
│   ├── export_service.go            # NEW: Config export logic
│   ├── import_service.go            # NEW: Config import logic
│   └── backup_service.go            # NEW: Database backup/restore
├── http/handlers/
│   ├── export.go                    # NEW: Export endpoints
│   ├── import.go                    # NEW: Import endpoints
│   └── backup.go                    # NEW: Backup endpoints
├── repository/
│   ├── interfaces.go                # MODIFY: Add GetUserCreated, GetByName methods
│   ├── filter_repo.go               # MODIFY: Implement new methods
│   ├── data_mapping_rule_repo.go    # MODIFY: Implement new methods
│   ├── client_detection_rule_repo.go # MODIFY: Implement new methods
│   └── encoding_profile_repo.go     # MODIFY: Implement new methods
└── scheduler/
    └── executor.go                  # MODIFY: Add BackupHandler

frontend/
└── src/
    ├── components/
    │   ├── config-export/           # NEW: Export UI components
    │   │   ├── export-dialog.tsx
    │   │   └── export-button.tsx
    │   ├── config-import/           # NEW: Import UI components
    │   │   ├── import-dialog.tsx
    │   │   ├── import-preview.tsx
    │   │   └── conflict-resolver.tsx
    │   └── backup/                  # NEW: Backup UI components
    │       ├── backup-list.tsx
    │       ├── backup-create.tsx
    │       └── restore-dialog.tsx
    └── pages/
        └── settings/
            └── backup.tsx           # NEW: Settings > Backup page
```

**Structure Decision**: Web application structure (Option 2). Backend in `internal/`, frontend in `frontend/src/`. Follows existing tvarr patterns.

## Key Implementation Decisions

### 1. SQLite Backup Method

**Decision**: Use `VACUUM INTO` command

**Rationale**:
- Atomic, consistent backup without requiring CGO
- Works correctly with WAL mode
- Available in SQLite 3.27.0+ (widely available)

**Implementation**:
```go
db.Exec("VACUUM INTO ?", backupPath)
```

### 2. Export Format

**Decision**: JSON with metadata envelope

**Structure**:
```json
{
  "metadata": {
    "version": "1.0.0",
    "tvarr_version": "1.2.3",
    "export_type": "filters",
    "exported_at": "2025-12-14T10:30:00Z",
    "item_count": 5
  },
  "items": [...]
}
```

### 3. Conflict Resolution

**Decision**: Name-based matching with three options

- **Skip**: Don't import conflicting item
- **Rename**: Add suffix "(1)", "(2)", etc.
- **Overwrite**: Replace existing, preserving original ID

### 4. Backup Scheduling

**Decision**: Extend existing job scheduler

- New job type: `JobTypeBackup = "backup"`
- Cron-based scheduling via config
- Automatic retention cleanup after backup

### 5. Backup Storage

**Decision**: Local filesystem with configurable directory

- Default: `{storage.base_dir}/backups/`
- File naming: `tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz`
- Companion metadata: `.meta.json` files

## Dependencies

### New Dependencies
None required - uses standard library and existing dependencies.

### Existing Dependencies Used
- `compress/gzip` - Backup compression
- `crypto/sha256` - Checksum verification
- `encoding/json` - Export/import format
- `robfig/cron/v3` - Scheduled backups (already in use)
- `gorm.io/gorm` - Database operations (already in use)

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Backup during active writes | VACUUM INTO handles WAL correctly |
| Restore interrupts streams | Warn user, require explicit confirmation |
| Disk space exhaustion | Check space before backup, log warning |
| Import version mismatch | Include version in export, warn if different |
| Restore failure | Pre-restore backup enables rollback |

## Next Steps

1. **Generate Tasks**: Run `/speckit.tasks` to create implementation tasks
2. **Implement Backend**: Start with services, then handlers
3. **Implement Frontend**: Build UI components after API is stable
4. **Testing**: Write integration tests for backup/restore scenarios

## Related Documents

- [research.md](./research.md) - Technical research and decisions
- [data-model.md](./data-model.md) - Data structure definitions
- [quickstart.md](./quickstart.md) - Implementation patterns and examples
- [contracts/openapi.yaml](./contracts/openapi.yaml) - API specification
