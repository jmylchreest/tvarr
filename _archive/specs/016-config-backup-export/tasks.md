# Tasks: Config Export/Import & Backup System

**Input**: Design documents from `/specs/016-config-backup-export/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/openapi.yaml

**Tests**: Required per constitution (Test-First Development is NON-NEGOTIABLE)

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` (Go)
- **Frontend**: `frontend/src/` (Next.js/React)
- **Tests**: `internal/*_test.go` (Go table-driven tests)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization, data structures, and configuration

- [X] T001 Add BackupConfig struct to internal/config/config.go with Directory, Schedule (Enabled, Cron, Retention) fields
- [X] T002 Add backup config defaults in SetDefaults() in internal/config/config.go (directory empty, schedule.enabled false, cron "0 0 2 * * *", retention 7)
- [X] T003 [P] Create export/import data structures in internal/models/export.go (ConfigExport, ExportMetadata, FilterExportItem, DataMappingRuleExportItem, ClientDetectionRuleExportItem, EncodingProfileExportItem, ImportPreview, ImportResult, ConflictItem)
- [X] T004 [P] Create backup metadata structures in internal/models/backup.go (BackupMetadata, BackupMetadataFile)
- [X] T005 [P] Add ExportFormatVersion constant ("1.0.0") and JobTypeBackup constant to internal/models/constants.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Repository extensions and shared service infrastructure that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Repository Extensions

- [X] T006 [P] Add GetUserCreated(ctx) method to FilterRepository interface in internal/repository/interfaces.go
- [X] T007 [P] Add GetByName(ctx, name) method to FilterRepository interface in internal/repository/interfaces.go
- [X] T008 [P] Add GetByIDs(ctx, ids) method to FilterRepository interface in internal/repository/interfaces.go
- [X] T009 [P] Add GetUserCreated(ctx) method to DataMappingRuleRepository interface in internal/repository/interfaces.go
- [X] T010 [P] Add GetByName(ctx, name) method to DataMappingRuleRepository interface in internal/repository/interfaces.go
- [X] T011 [P] Add GetByIDs(ctx, ids) method to DataMappingRuleRepository interface in internal/repository/interfaces.go
- [X] T012 [P] Add GetUserCreated(ctx) method to ClientDetectionRuleRepository interface in internal/repository/interfaces.go
- [X] T013 [P] Add GetByName(ctx, name) method to ClientDetectionRuleRepository interface in internal/repository/interfaces.go
- [X] T014 [P] Add GetByIDs(ctx, ids) method to ClientDetectionRuleRepository interface in internal/repository/interfaces.go
- [X] T015 [P] Add GetUserCreated(ctx) method to EncodingProfileRepository interface in internal/repository/interfaces.go
- [X] T016 [P] Add GetByIDs(ctx, ids) method to EncodingProfileRepository interface in internal/repository/interfaces.go

### Repository Implementations

- [X] T017 [P] Implement GetUserCreated, GetByName, GetByIDs for FilterRepository in internal/repository/filter_repo.go
- [X] T018 [P] Implement GetUserCreated, GetByName, GetByIDs for DataMappingRuleRepository in internal/repository/data_mapping_rule_repo.go
- [X] T019 [P] Implement GetUserCreated, GetByName, GetByIDs for ClientDetectionRuleRepository in internal/repository/client_detection_rule_repo.go
- [X] T020 [P] Implement GetUserCreated, GetByIDs for EncodingProfileRepository in internal/repository/encoding_profile_repo.go (GetByName already exists)

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Export Individual Configuration Items (Priority: P1) 🎯 MVP

**Goal**: Users can export selected filters, data mapping rules, client detection rules, and encoding profiles as portable JSON files, and import them with conflict resolution (Skip, Rename, Overwrite)

**Independent Test**: Create a filter rule, export it, delete the rule, import it back, verify rule is restored correctly

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T021 [P] [US1] Unit test for ExportService.ExportFilters in internal/service/export_service_test.go (test excludes IsSystem=true, generates correct metadata)
- [X] T022 [P] [US1] Unit test for ExportService.ExportDataMappingRules in internal/service/export_service_test.go
- [X] T023 [P] [US1] Unit test for ExportService.ExportClientDetectionRules in internal/service/export_service_test.go
- [X] T024 [P] [US1] Unit test for ExportService.ExportEncodingProfiles in internal/service/export_service_test.go
- [X] T025 [P] [US1] Unit test for ImportService.ImportFiltersPreview in internal/service/import_service_test.go (test conflict detection, validation errors)
- [X] T026 [P] [US1] Unit test for ImportService.ImportFilters with Skip resolution in internal/service/import_service_test.go
- [X] T027 [P] [US1] Unit test for ImportService.ImportFilters with Rename resolution in internal/service/import_service_test.go
- [X] T028 [P] [US1] Unit test for ImportService.ImportFilters with Overwrite resolution in internal/service/import_service_test.go
- [X] T029 [P] [US1] Integration test for export/import round-trip in internal/service/export_import_integration_test.go

### Implementation for User Story 1

#### Export Service

- [X] T030 [P] [US1] Create ExportService struct with repository dependencies in internal/service/export_service.go
- [X] T031 [US1] Implement ExportFilters method in internal/service/export_service.go (query user-created filters, convert to FilterExportItem, wrap in ConfigExport with metadata)
- [X] T032 [US1] Implement ExportDataMappingRules method in internal/service/export_service.go
- [X] T033 [US1] Implement ExportClientDetectionRules method in internal/service/export_service.go (export EncodingProfileID as EncodingProfileName)
- [X] T034 [US1] Implement ExportEncodingProfiles method in internal/service/export_service.go

#### Import Service

- [X] T035 [P] [US1] Create ImportService struct with DB and repository dependencies in internal/service/import_service.go
- [X] T036 [US1] Implement parseAndValidateExport helper in internal/service/import_service.go (validate JSON, check version, export_type)
- [X] T037 [US1] Implement ImportFiltersPreview method in internal/service/import_service.go (detect name conflicts, validate expressions)
- [X] T038 [US1] Implement ImportFilters method in internal/service/import_service.go (atomic transaction, handle Skip/Rename/Overwrite, generate new IDs)
- [X] T039 [US1] Implement ImportDataMappingRulesPreview and ImportDataMappingRules in internal/service/import_service.go
- [X] T040 [US1] Implement ImportClientDetectionRulesPreview and ImportClientDetectionRules in internal/service/import_service.go (resolve EncodingProfileName to ID)
- [X] T041 [US1] Implement ImportEncodingProfilesPreview and ImportEncodingProfiles in internal/service/import_service.go

#### HTTP Handlers

- [X] T042 [P] [US1] Create ExportHandler struct in internal/http/handlers/export.go
- [X] T043 [US1] Implement exportFilters endpoint (POST /api/v1/filters/export) in internal/http/handlers/export.go
- [X] T044 [US1] Implement exportDataMappingRules endpoint (POST /api/v1/data-mapping-rules/export) in internal/http/handlers/export.go
- [X] T045 [US1] Implement exportClientDetectionRules endpoint (POST /api/v1/client-detection-rules/export) in internal/http/handlers/export.go
- [X] T046 [US1] Implement exportEncodingProfiles endpoint (POST /api/v1/encoding-profiles/export) in internal/http/handlers/export.go
- [X] T047 [P] [US1] Create ImportHandler struct in internal/http/handlers/import.go
- [X] T048 [US1] Implement importFilters endpoint (POST /api/v1/filters/import) with preview query param in internal/http/handlers/import.go
- [X] T049 [US1] Implement importDataMappingRules endpoint in internal/http/handlers/import.go
- [X] T050 [US1] Implement importClientDetectionRules endpoint in internal/http/handlers/import.go
- [X] T051 [US1] Implement importEncodingProfiles endpoint in internal/http/handlers/import.go
- [X] T052 [US1] Register export and import handlers in internal/http/server.go

#### Frontend Components

- [X] T053 [P] [US1] Create ExportButton component in frontend/src/components/config-export/export-button.tsx
- [X] T054 [P] [US1] Create ExportDialog component in frontend/src/components/config-export/export-dialog.tsx (checkbox selection, export all option)
- [X] T055 [P] [US1] Create ImportDialog component in frontend/src/components/config-import/import-dialog.tsx (file upload)
- [X] T056 [P] [US1] Create ImportPreview component in frontend/src/components/config-import/import-preview.tsx (show new items, conflicts)
- [X] T057 [US1] Create ConflictResolver component in frontend/src/components/config-import/conflict-resolver.tsx (per-item Skip/Rename/Overwrite)
- [X] T058 [US1] Add export/import buttons to Filters page in frontend/src/pages/filters.tsx (or appropriate settings page)
- [X] T059 [US1] Add export/import buttons to Data Mapping Rules page
- [X] T060 [US1] Add export/import buttons to Client Detection Rules page
- [X] T061 [US1] Add export/import buttons to Encoding Profiles page

**Checkpoint**: User Story 1 complete - users can export and import individual config items with conflict resolution

---

## Phase 4: User Story 2 - Full System Backup and Restore (Priority: P2)

**Goal**: Administrators can create full database backups on demand, list available backups, download them, and restore from any backup

**Independent Test**: Run a full backup, wipe the database, restore from backup, verify all data is restored

### Tests for User Story 2

- [X] T062 [P] [US2] Unit test for BackupService.CreateBackup in internal/service/backup_service_test.go (test VACUUM INTO, gzip compression, metadata creation)
- [X] T063 [P] [US2] Unit test for BackupService.ListBackups in internal/service/backup_service_test.go
- [X] T064 [P] [US2] Unit test for BackupService.RestoreBackup in internal/service/backup_service_test.go (test pre-restore backup, checksum verification)
- [X] T065 [P] [US2] Unit test for BackupService.DeleteBackup in internal/service/backup_service_test.go
- [X] T066 [P] [US2] Integration test for backup/restore round-trip in internal/service/backup_integration_test.go (verify entity relationships preserved: ClientDetectionRule→EncodingProfile FK, Filter→Source FK)

### Implementation for User Story 2

#### Backup Service

- [X] T067 [P] [US2] Create BackupService struct with DB and config dependencies in internal/service/backup_service.go
- [X] T068 [US2] Implement CreateBackup method in internal/service/backup_service.go (VACUUM INTO, gzip compress, calculate checksum, write metadata JSON)
- [X] T069 [US2] Implement ListBackups method in internal/service/backup_service.go (scan directory, parse metadata files, sort by date)
- [X] T070 [US2] Implement loadBackupMetadata helper in internal/service/backup_service.go (read .meta.json, extract from filename if missing)
- [X] T071 [US2] Implement RestoreBackup method in internal/service/backup_service.go (verify checksum, create pre-restore backup, decompress, atomic swap; NOTE: caller must handle DB reconnection or app restart after restore completes)
- [X] T072 [US2] Implement DeleteBackup method in internal/service/backup_service.go (delete .db.gz and .meta.json files)
- [X] T073 [US2] Implement OpenBackupFile method in internal/service/backup_service.go (for download streaming)
- [X] T074 [US2] Implement getTableCounts helper in internal/service/backup_service.go (count rows in main tables for metadata)

#### HTTP Handlers

- [X] T075 [P] [US2] Create BackupHandler struct in internal/http/handlers/backup.go
- [X] T076 [US2] Implement listBackups endpoint (GET /api/v1/backups) in internal/http/handlers/backup.go
- [X] T077 [US2] Implement createBackup endpoint (POST /api/v1/backups) in internal/http/handlers/backup.go
- [X] T078 [US2] Implement downloadBackup endpoint (GET /api/v1/backups/{filename}) in internal/http/handlers/backup.go (stream file with Content-Disposition)
- [X] T079 [US2] Implement deleteBackup endpoint (DELETE /api/v1/backups/{filename}) in internal/http/handlers/backup.go
- [X] T080 [US2] Implement restoreBackup endpoint (POST /api/v1/backups/{filename}/restore) in internal/http/handlers/backup.go (require confirm=true query param)
- [X] T081 [US2] Register backup handler routes in internal/http/server.go

#### Frontend Components

- [X] T082 [P] [US2] Create BackupList component in frontend/src/components/backup/backup-list.tsx (table of backups with date, size, actions)
- [X] T083 [P] [US2] Create BackupCreate component in frontend/src/components/backup/backup-create.tsx (create backup button with loading state)
- [X] T084 [US2] Create RestoreDialog component in frontend/src/components/backup/restore-dialog.tsx (warning, confirmation, progress)
- [X] T085 [US2] Create Settings > Backup page in frontend/src/pages/settings/backup.tsx

**Checkpoint**: User Story 2 complete - administrators can backup and restore the full database

---

## Phase 5: User Story 3 - Scheduled Automatic Backups (Priority: P3)

**Goal**: System runs scheduled backups automatically based on cron configuration and enforces retention policies

**Independent Test**: Set backup schedule to every minute, wait, verify backups are created and old backups are pruned per retention

### Tests for User Story 3

- [X] T086 [P] [US3] Unit test for BackupService.CleanupOldBackups in internal/service/backup_service_test.go
- [X] T087 [P] [US3] Unit test for BackupHandler.Execute (scheduled job) in internal/scheduler/backup_handler_test.go

### Implementation for User Story 3

#### Backup Retention

- [X] T088 [US3] Implement CleanupOldBackups method in internal/service/backup_service.go (delete oldest backups exceeding retention limit)

#### Scheduled Job Handler

- [X] T089 [P] [US3] Create BackupHandler job handler struct in internal/scheduler/backup_handler.go
- [X] T090 [US3] Implement Execute method for BackupHandler in internal/scheduler/backup_handler.go (create backup, cleanup old backups, log results)
- [X] T091 [US3] Register BackupHandler with job type "backup" in internal/scheduler/executor.go
- [X] T092 [US3] Add backup job scheduling in scheduler startup when config backup.schedule.enabled is true in internal/scheduler/scheduler.go

#### Frontend Schedule Display

- [X] T093 [US3] Add schedule configuration display to Backup page in frontend/src/pages/settings/backup.tsx (show enabled, cron, retention, next run)

**Checkpoint**: User Story 3 complete - scheduled backups run automatically with retention enforcement

---

## Phase 6: User Story 4 - Bulk Conflict Resolution Actions (Priority: P4)

**Goal**: When importing many items, users can apply "Skip All", "Rename All", or "Overwrite All" to resolve all conflicts at once

**Independent Test**: Import a file with 10+ conflicts, use "Overwrite All", verify all conflicts are resolved

### Tests for User Story 4

- [X] T094 [P] [US4] Unit test for bulk resolution (all skip, all rename, all overwrite) in internal/service/import_service_test.go

### Implementation for User Story 4

- [X] T095 [US4] Update ImportFilters to accept bulk resolution parameter in internal/service/import_service.go
- [X] T096 [US4] Update ImportDataMappingRules, ImportClientDetectionRules, ImportEncodingProfiles for bulk resolution
- [X] T097 [US4] Update import endpoints to accept bulk_resolution form field in internal/http/handlers/import.go
- [X] T098 [US4] Add bulk action buttons to ConflictResolver component in frontend/src/components/config-import/conflict-resolver.tsx

**Checkpoint**: User Story 4 complete - users can resolve all import conflicts with one action

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup and quality improvements

- [X] T099 [P] Add slog logging for all export operations in internal/service/export_service.go
- [X] T100 [P] Add slog logging for all import operations in internal/service/import_service.go
- [X] T101 [P] Add slog logging for all backup operations in internal/service/backup_service.go
- [X] T102 Add path traversal prevention for backup filename parameter in internal/http/handlers/backup.go
- [X] T103 Add disk space check before backup creation in internal/service/backup_service.go (with unit test for failure path)
- [X] T104 Add version compatibility warning when importing from different tvarr version
- [X] T105 Run go test ./... and fix any failing tests
- [X] T106 Run task lint and fix any linting issues
- [ ] T107 Verify all acceptance scenarios from spec.md pass manual testing

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-6)**: All depend on Foundational phase completion
  - User stories can proceed in parallel (if staffed)
  - Or sequentially in priority order (P1 → P2 → P3 → P4)
- **Polish (Phase 7)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) - No dependencies on US1
- **User Story 3 (P3)**: Depends on US2 (BackupService.CreateBackup must exist)
- **User Story 4 (P4)**: Depends on US1 (import functionality must exist)

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Models/structs before services
- Services before handlers
- Backend before frontend
- Core implementation before polish
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks (T001-T005) marked [P] can run in parallel
- All Foundational repository interface extensions (T006-T016) can run in parallel
- All Foundational repository implementations (T017-T020) can run in parallel
- Within US1: All export service tests (T021-T024) can run in parallel
- Within US1: All import service tests (T025-T029) can run in parallel
- Within US1: Export and import handler creation (T042, T047) can run in parallel
- Within US1: All frontend components (T053-T057) can run in parallel initially
- Within US2: All backup service tests (T062-T066) can run in parallel
- Within US2: Backend and frontend work can be parallelized (T067-T081 backend, T082-T085 frontend)

---

## Parallel Example: Foundational Phase

```bash
# Launch all repository interface extensions together:
Task: "Add GetUserCreated to FilterRepository interface"
Task: "Add GetByName to FilterRepository interface"
Task: "Add GetByIDs to FilterRepository interface"
Task: "Add GetUserCreated to DataMappingRuleRepository interface"
# ... etc (T006-T016)

# Then launch all implementations together:
Task: "Implement GetUserCreated, GetByName, GetByIDs for FilterRepository"
Task: "Implement GetUserCreated, GetByName, GetByIDs for DataMappingRuleRepository"
# ... etc (T017-T020)
```

## Parallel Example: User Story 1 Tests

```bash
# Launch all US1 tests together (they should all fail initially):
Task: "Unit test for ExportService.ExportFilters"
Task: "Unit test for ExportService.ExportDataMappingRules"
Task: "Unit test for ImportService.ImportFiltersPreview"
Task: "Unit test for ImportService.ImportFilters with Skip resolution"
# ... etc (T021-T029)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (export/import with conflict resolution)
4. **STOP and VALIDATE**: Test export/import round-trip independently
5. Deploy/demo if ready - users can share configurations!

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test export/import → Deploy/Demo (MVP!)
3. Add User Story 2 → Test backup/restore → Deploy/Demo
4. Add User Story 3 → Test scheduled backups → Deploy/Demo
5. Add User Story 4 → Test bulk conflict resolution → Deploy/Demo
6. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (export/import)
   - Developer B: User Story 2 (backup/restore)
3. After US1 + US2 complete:
   - Developer A: User Story 4 (bulk resolution - depends on US1)
   - Developer B: User Story 3 (scheduled backups - depends on US2)

---

## Summary

| Phase | Task Count | Parallel Opportunities |
|-------|------------|------------------------|
| Phase 1: Setup | 5 | 3 |
| Phase 2: Foundational | 15 | 15 |
| Phase 3: US1 - Export/Import | 41 | 18 |
| Phase 4: US2 - Backup/Restore | 24 | 9 |
| Phase 5: US3 - Scheduled Backups | 8 | 2 |
| Phase 6: US4 - Bulk Conflict | 5 | 1 |
| Phase 7: Polish | 9 | 3 |
| **Total** | **107** | **51** |

### Independent Test Criteria

| User Story | Independent Test |
|------------|-----------------|
| US1 | Create filter → export → delete → import → verify restored |
| US2 | Create backup → wipe DB → restore → verify all data restored |
| US3 | Set schedule to 1 min → wait → verify backups created and pruned |
| US4 | Import file with 10+ conflicts → "Overwrite All" → verify all resolved |

### Suggested MVP Scope

**User Story 1 only** - Enables users to export and share configurations immediately. Backup features (US2-US3) and bulk resolution (US4) can be added incrementally.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story is independently completable and testable
- Verify tests fail before implementing (TDD per constitution)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Constitution requires: slog logging (no emojis), fictional test data, constants for magic strings
