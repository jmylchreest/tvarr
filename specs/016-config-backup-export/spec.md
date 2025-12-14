# Feature Specification: Config Export/Import & Backup System

**Feature Branch**: `016-config-backup-export`
**Created**: 2025-12-14
**Status**: Draft
**Input**: User description: "I would like a feature that allows me to export and import custom config and share it easily, such as user defined filters, data mapping rules, client detection rules, etc. I would also like an ability to do a full backup/restore with a scheduled backup solution."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Export Individual Configuration Items (Priority: P1)

A user wants to share their carefully crafted filter rules with the community or back up specific configurations. They navigate to the relevant configuration section (Filters, Data Mapping Rules, Client Detection Rules, or Encoding Profiles), select items to export, and download them as a portable file that can be shared or stored.

**Why this priority**: This is the core functionality that enables configuration sharing and partial backups. Users frequently ask to share specific rules without exposing their entire setup.

**Independent Test**: Can be fully tested by creating a filter rule, exporting it, deleting the rule, then importing it back and verifying the rule is restored correctly.

**Acceptance Scenarios**:

1. **Given** a user has created custom filters, **When** they select filters and click "Export Selected", **Then** a JSON file is downloaded containing only the selected filter definitions
2. **Given** a user receives an exported filters file from another user, **When** they import the file, **Then** the filters are added to their system with new unique identifiers
3. **Given** a user exports data mapping rules, **When** they open the exported file, **Then** it contains human-readable JSON with all rule properties preserved
4. **Given** a user imports configuration with conflicting names, **When** the import dialog appears, **Then** the user is presented with options: Skip, Rename (add suffix), or Overwrite for each conflict
5. **Given** a user selects "Overwrite" for a conflicting item, **When** import completes, **Then** the existing item is replaced with the imported values while preserving the original ID

---

### User Story 2 - Full System Backup and Restore (Priority: P2)

An administrator wants to protect against data loss by creating complete backups of the tvarr database. They can trigger a manual backup or rely on scheduled automatic backups. When disaster strikes, they can restore from any backup to recover the entire system state.

**Why this priority**: Essential for production deployments where data loss is unacceptable. Depends on understanding the export format (P1) but provides comprehensive protection.

**Independent Test**: Can be tested by running a full backup, wiping the database, restoring from backup, and verifying all sources, channels, proxies, filters, and configuration are restored.

**Acceptance Scenarios**:

1. **Given** a user is on the Settings page, **When** they click "Create Backup", **Then** a compressed backup file is created containing the entire database and saved to the backup directory
2. **Given** backup files exist, **When** a user views the backup list, **Then** they see backup date, file size, and can download or restore each backup
3. **Given** a user selects a backup to restore, **When** they confirm the restore action, **Then** the system warns about data replacement, then replaces all data with the backup contents
4. **Given** a restore fails partway through, **When** the error occurs, **Then** the system rolls back to the pre-restore state and displays a clear error message

---

### User Story 3 - Scheduled Automatic Backups (Priority: P3)

An administrator wants peace of mind knowing backups happen automatically without manual intervention. They configure a backup schedule (daily, weekly) and retention policy (keep last N backups). The system creates backups on schedule and cleans up old backups automatically.

**Why this priority**: Enhances the backup feature (P2) with automation. Not strictly required for MVP but critical for production use where manual backups may be forgotten.

**Independent Test**: Can be tested by setting a backup schedule to every minute (for testing), waiting, and verifying backups are created and old backups are pruned according to retention policy.

**Acceptance Scenarios**:

1. **Given** a user enables scheduled backups, **When** they set frequency to "daily" and retention to 7, **Then** backups run daily and only the 7 most recent backups are kept
2. **Given** scheduled backups are enabled, **When** the scheduled time arrives, **Then** a backup is created without user intervention and appears in the backup list
3. **Given** more backups exist than the retention limit, **When** a new backup completes, **Then** the oldest backups beyond the retention limit are automatically deleted
4. **Given** a scheduled backup fails, **When** the failure occurs, **Then** the system logs the error with details and retries at the next scheduled time

---

### User Story 4 - Bulk Conflict Resolution Actions (Priority: P4)

When importing many items, a user wants to quickly apply the same conflict resolution action to all conflicts rather than handling each one individually.

**Why this priority**: Quality-of-life improvement for large imports. Individual conflict handling (P1) covers core functionality.

**Independent Test**: Can be tested by importing a file with 10+ conflicts and using "Overwrite All" to resolve them in one action.

**Acceptance Scenarios**:

1. **Given** a user imports a file with multiple name conflicts, **When** the import preview loads, **Then** bulk action buttons are available: "Skip All", "Rename All", "Overwrite All"
2. **Given** a user clicks "Overwrite All", **When** import completes, **Then** all conflicting items are replaced with imported values
3. **Given** a user has applied a bulk action, **When** they review the preview, **Then** they can still change individual items before confirming import

---

### Edge Cases

- What happens when importing a file with invalid JSON format?
  - System displays a validation error identifying the format issue
- What happens when importing items referencing non-existent related entities (e.g., filter referencing missing source)?
  - System creates the item with null reference and warns user
- What happens when the backup directory runs out of disk space?
  - Backup fails with clear error message; scheduled backups log warning and skip
- How does system handle importing configuration from a different tvarr version?
  - Include version in export format; warn if version differs; attempt migration for known schema changes
- What happens if user tries to restore while streams are active?
  - Display warning that active streams will be interrupted; require explicit confirmation

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow exporting selected filters as a portable JSON file
- **FR-002**: System MUST allow exporting selected data mapping rules as a portable JSON file
- **FR-003**: System MUST allow exporting selected client detection rules as a portable JSON file
- **FR-004**: System MUST allow exporting selected encoding profiles as a portable JSON file
- **FR-005**: System MUST allow importing configuration from exported JSON files
- **FR-006**: System MUST generate unique identifiers for imported items to prevent ID conflicts
- **FR-007**: System MUST present conflict resolution options (Skip, Rename, Overwrite) when importing items with matching names
- **FR-008**: System MUST replace existing items with imported values when user selects Overwrite, preserving the original database ID
- **FR-009**: System MUST allow creating full database backups on demand
- **FR-010**: System MUST store backups in a configurable backup directory
- **FR-011**: System MUST allow listing available backups with metadata (date, size)
- **FR-012**: System MUST allow restoring from a selected backup
- **FR-013**: System MUST support scheduled automatic backups with configurable frequency
- **FR-014**: System MUST support backup retention policies (keep last N backups)
- **FR-015**: System MUST automatically delete backups exceeding retention limit
- **FR-016**: System MUST include version information in all exports for compatibility checking
- **FR-017**: System MUST validate imported files before applying changes
- **FR-018**: System MUST exclude system-provided default rules (IsSystem=true) from configuration exports
- **FR-019**: System MUST preserve all entity relationships during backup/restore
- **FR-020**: System MUST provide restore rollback capability if restore fails partway through

### Key Entities

- **ConfigExport**: A portable file containing configuration items of a specific type (filters, mappings, rules, profiles) with metadata including version, export date, and item count
- **Backup**: A compressed archive (`tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz`) containing the complete database state including all tables, with metadata about creation time, tvarr version, and file integrity checksum
- **BackupSchedule**: Configuration for automated backups including frequency (hourly, daily, weekly), retention count, and next scheduled run time
- **ImportResult**: Summary of an import operation including items imported, items skipped, conflicts resolved, and any warnings or errors

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can export and import individual configuration items in under 30 seconds for typical configurations (up to 100 items)
- **SC-002**: Full backup creation completes in under 2 minutes for databases up to 1GB
- **SC-003**: Restore operation completes in under 5 minutes for databases up to 1GB
- **SC-004**: Scheduled backups run within 5 minutes of their configured time (cron has 1-minute granularity; 5-minute window accounts for job queue processing delays)
- **SC-005**: Exported configuration files are readable and editable by users with standard text editors
- **SC-006**: 100% of user-created configuration is preserved through a backup/restore cycle
- **SC-007**: Users can successfully share exported configuration files across different tvarr installations
- **SC-008**: System maintains zero data loss during backup/restore operations when adequate disk space is available

## Assumptions

- SQLite is the primary database (most common deployment). The backup solution will use SQLite's native backup API for consistency. PostgreSQL/MySQL support for backup can be added later but is out of scope for initial implementation.
- Backup storage is on local filesystem. Cloud storage (S3, etc.) is out of scope for initial implementation.
- Export format uses JSON for human readability and easy sharing.
- System-provided default rules (IsSystem=true) should not be exported since they exist on all installations.
- The backup directory is configurable via `backup.directory` setting, defaulting to `{storage.base_dir}/backups` (e.g., `~/.data/backups/` or `/data/backups/` in Docker).
- Backup files use timestamp-based naming: `tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz` for natural sorting and clear identification.
- Scheduled backup times use server local time.
- Import operations are atomic - either all items in the file are imported or none are.

## Clarifications

### Session 2025-12-14

- Q: How should the system notify administrators when scheduled backups fail? → A: Log only (no active notification); webhook/push notifications documented in wishlist for future consideration
- Q: What naming convention should backup files use? → A: Timestamp-based: `tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz`, stored in configurable `backup.directory` setting (defaults to `{storage.base_dir}/backups`)

## Out of Scope

- Cloud backup storage (S3, Google Drive, etc.)
- Backup encryption
- Backup to remote servers via SSH/FTP
- Differential or incremental backups (always full backups)
- Cross-database migration (SQLite to PostgreSQL)
- Configuration sync between multiple tvarr instances
- Selective restore (restoring only certain tables from a backup)
