# Data Model: Config Export/Import & Backup System

**Date**: 2025-12-14
**Feature**: 016-config-backup-export
**Spec**: [spec.md](./spec.md)

## Overview

This document defines the data structures for the config export/import and backup system. It covers:
1. Export file format for configuration items
2. Backup metadata tracking
3. Backup schedule configuration

## New Entities

### BackupMetadata

Tracks backup files in the backup directory. This is **not** stored in the database but derived from filesystem scanning.

```go
// BackupMetadata represents a backup file's metadata.
type BackupMetadata struct {
    Filename     string    // e.g., "tvarr-backup-2025-12-14T10-30-00.db.gz"
    FilePath     string    // Full path to backup file
    CreatedAt    time.Time // Extracted from filename
    FileSize     int64     // Size in bytes
    Checksum     string    // SHA256 hash for integrity verification
    TvarrVersion string    // Version that created the backup (from metadata file)
}
```

**Notes**:
- Metadata derived from filename and companion `.meta.json` file
- No database table - filesystem is source of truth for backups
- Companion metadata file: `tvarr-backup-YYYY-MM-DDTHH-MM-SS.meta.json`

### BackupScheduleConfig

Configuration for automatic backups, stored in Viper config (not database).

```yaml
# config.yaml
backup:
  directory: "./data/backups"       # Backup storage location
  schedule:
    enabled: false                  # Enable scheduled backups
    cron: "0 0 2 * * *"            # Daily at 2 AM (6-field cron)
    retention: 7                    # Keep last N backups
```

```go
// BackupConfig holds backup configuration.
type BackupConfig struct {
    Directory string         `mapstructure:"directory"`
    Schedule  ScheduleConfig `mapstructure:"schedule"`
}

type ScheduleConfig struct {
    Enabled   bool   `mapstructure:"enabled"`
    Cron      string `mapstructure:"cron"`      // 6-field cron expression
    Retention int    `mapstructure:"retention"` // Number of backups to keep
}
```

## Export File Formats

### ConfigExport (Generic Wrapper)

All configuration exports use this envelope format.

```go
// ConfigExport is the generic wrapper for all config exports.
type ConfigExport struct {
    Metadata ExportMetadata `json:"metadata"`
    Items    interface{}    `json:"items"` // Type-specific items array
}

// ExportMetadata contains export file metadata.
type ExportMetadata struct {
    Version      string    `json:"version"`       // Export format version (e.g., "1.0.0")
    TvarrVersion string    `json:"tvarr_version"` // tvarr application version
    ExportType   string    `json:"export_type"`   // "filters", "data_mapping_rules", "client_detection_rules", "encoding_profiles"
    ExportedAt   time.Time `json:"exported_at"`   // UTC timestamp
    ItemCount    int       `json:"item_count"`    // Number of items in export
}
```

### FilterExport

Export format for filters.

```go
// FilterExportItem represents a filter for export/import.
type FilterExportItem struct {
    Name        string  `json:"name"`
    Description string  `json:"description,omitempty"`
    Expression  string  `json:"expression"`
    SourceType  string  `json:"source_type"`  // "stream" or "epg"
    Action      string  `json:"action"`       // "include" or "exclude"
    IsEnabled   bool    `json:"is_enabled"`
    SourceID    *string `json:"source_id,omitempty"` // Optional source restriction
}
```

**Example Export**:
```json
{
  "metadata": {
    "version": "1.0.0",
    "tvarr_version": "1.2.3",
    "export_type": "filters",
    "exported_at": "2025-12-14T10:30:00Z",
    "item_count": 2
  },
  "items": [
    {
      "name": "Sports Channels",
      "description": "Include all sports channels",
      "expression": "group CONTAINS 'Sports'",
      "source_type": "stream",
      "action": "include",
      "is_enabled": true
    },
    {
      "name": "Adult Filter",
      "expression": "name MATCHES '(?i)adult|xxx'",
      "source_type": "stream",
      "action": "exclude",
      "is_enabled": true
    }
  ]
}
```

### DataMappingRuleExport

Export format for data mapping rules.

```go
// DataMappingRuleExportItem represents a data mapping rule for export/import.
type DataMappingRuleExportItem struct {
    Name        string  `json:"name"`
    Description string  `json:"description,omitempty"`
    Expression  string  `json:"expression"`
    SourceType  string  `json:"source_type"`   // "stream" or "epg"
    Priority    int     `json:"priority"`
    StopOnMatch bool    `json:"stop_on_match"`
    IsEnabled   bool    `json:"is_enabled"`
    SourceID    *string `json:"source_id,omitempty"`
}
```

### ClientDetectionRuleExport

Export format for client detection rules.

```go
// ClientDetectionRuleExportItem represents a client detection rule for export/import.
type ClientDetectionRuleExportItem struct {
    Name                  string   `json:"name"`
    Description           string   `json:"description,omitempty"`
    Expression            string   `json:"expression"`
    Priority              int      `json:"priority"`
    IsEnabled             bool     `json:"is_enabled"`
    AcceptedVideoCodecs   []string `json:"accepted_video_codecs,omitempty"`
    AcceptedAudioCodecs   []string `json:"accepted_audio_codecs,omitempty"`
    PreferredVideoCodec   string   `json:"preferred_video_codec,omitempty"`
    PreferredAudioCodec   string   `json:"preferred_audio_codec,omitempty"`
    SupportsFMP4          bool     `json:"supports_fmp4"`
    SupportsMPEGTS        bool     `json:"supports_mpegts"`
    PreferredFormat       string   `json:"preferred_format,omitempty"`
    EncodingProfileName   *string  `json:"encoding_profile_name,omitempty"` // Reference by name, not ID
}
```

**Note**: `EncodingProfileID` is exported as `EncodingProfileName` for portability. On import, the name is resolved to an ID.

### EncodingProfileExport

Export format for encoding profiles.

```go
// EncodingProfileExportItem represents an encoding profile for export/import.
type EncodingProfileExportItem struct {
    Name             string  `json:"name"`
    Description      string  `json:"description,omitempty"`
    TargetVideoCodec string  `json:"target_video_codec"`
    TargetAudioCodec string  `json:"target_audio_codec"`
    QualityPreset    string  `json:"quality_preset"` // low, medium, high, ultra
    HWAccel          string  `json:"hw_accel"`       // auto, none, cuda, vaapi, qsv, videotoolbox
    GlobalFlags      *string `json:"global_flags,omitempty"`
    InputFlags       *string `json:"input_flags,omitempty"`
    OutputFlags      *string `json:"output_flags,omitempty"`
    IsDefault        bool    `json:"is_default"`
    Enabled          bool    `json:"enabled"`
}
```

## Import Result Types

### ImportResult

Returned after an import operation completes.

```go
// ImportResult summarizes an import operation.
type ImportResult struct {
    TotalItems     int              `json:"total_items"`
    Imported       int              `json:"imported"`
    Skipped        int              `json:"skipped"`
    Overwritten    int              `json:"overwritten"`
    Renamed        int              `json:"renamed"`
    Errors         int              `json:"errors"`
    ErrorDetails   []ImportError    `json:"error_details,omitempty"`
    ImportedItems  []ImportedItem   `json:"imported_items"`
}

// ImportError describes a single import error.
type ImportError struct {
    ItemName string `json:"item_name"`
    Error    string `json:"error"`
}

// ImportedItem describes a successfully imported item.
type ImportedItem struct {
    OriginalName string `json:"original_name"`
    FinalName    string `json:"final_name"`    // May differ if renamed
    ID           string `json:"id"`            // New or existing ID
    Action       string `json:"action"`        // "created", "overwritten", "renamed"
}
```

### ImportPreview

Preview of what will happen before confirming import.

```go
// ImportPreview shows what will happen if import proceeds.
type ImportPreview struct {
    TotalItems int               `json:"total_items"`
    NewItems   []PreviewItem     `json:"new_items"`
    Conflicts  []ConflictItem    `json:"conflicts"`
    Errors     []ImportError     `json:"errors"`
}

// PreviewItem represents an item that will be imported without conflict.
type PreviewItem struct {
    Name string `json:"name"`
}

// ConflictItem represents an item with a name conflict.
type ConflictItem struct {
    ImportName   string `json:"import_name"`
    ExistingID   string `json:"existing_id"`
    ExistingName string `json:"existing_name"`
    Resolution   string `json:"resolution"` // "skip", "rename", "overwrite" - default "skip"
}
```

## Backup Companion Metadata File

Each backup has a companion `.meta.json` file with additional information.

```go
// BackupMetadataFile is stored alongside each backup.
type BackupMetadataFile struct {
    TvarrVersion string    `json:"tvarr_version"`
    DatabaseSize int64     `json:"database_size"`     // Uncompressed size
    CompressedSize int64   `json:"compressed_size"`   // Gzip compressed size
    Checksum     string    `json:"checksum"`          // SHA256 of .db.gz file
    CreatedAt    time.Time `json:"created_at"`
    TableCounts  map[string]int `json:"table_counts"` // Row counts per table
}
```

**Example**: `tvarr-backup-2025-12-14T10-30-00.meta.json`
```json
{
  "tvarr_version": "1.2.3",
  "database_size": 52428800,
  "compressed_size": 10485760,
  "checksum": "sha256:abc123...",
  "created_at": "2025-12-14T10:30:00Z",
  "table_counts": {
    "channels": 50000,
    "epg_programs": 1000000,
    "filters": 25,
    "data_mapping_rules": 10,
    "encoding_profiles": 8
  }
}
```

## Entity Relationships

### Existing Entities (Not Modified)

The following entities are exported but not modified by this feature:

| Entity | Table | IsSystem Field | Export Exclusion |
|--------|-------|----------------|------------------|
| Filter | `filters` | `is_system` | `WHERE is_system = false` |
| DataMappingRule | `data_mapping_rules` | `is_system` | `WHERE is_system = false` |
| ClientDetectionRule | `client_detection_rules` | `is_system` | `WHERE is_system = false` |
| EncodingProfile | `encoding_profiles` | `is_system` | `WHERE is_system = false` |

### Reference Resolution on Import

When importing ClientDetectionRules that reference EncodingProfiles:
1. If `encoding_profile_name` is provided in import
2. Look up EncodingProfile by name in database
3. If found, set `encoding_profile_id` to the found ID
4. If not found, set `encoding_profile_id` to null and warn user

## State Transitions

### Backup Lifecycle

```
[Not Exists] → [Creating] → [Available] → [Deleted]
                   ↓
               [Failed]
```

### Import Operation States

```
[Upload] → [Validating] → [Previewing] → [Importing] → [Complete]
              ↓               ↓              ↓
           [Invalid]     [Cancelled]     [Failed]
```

## Validation Rules

### Export Validation

- Only export items where `is_system = false`
- All referenced entities must exist (e.g., SourceID)
- Export at least one item (empty exports not allowed)

### Import Validation

| Field | Rule |
|-------|------|
| `metadata.version` | Must be "1.0.0" (current supported version) |
| `metadata.export_type` | Must match expected type for endpoint |
| `items[].name` | Required, non-empty, max 255 characters |
| `items[].expression` | Must be valid expression syntax (validated at import) |
| `items[].source_type` | Must be "stream" or "epg" (where applicable) |
| `items[].action` | Must be "include" or "exclude" (for filters) |

### Backup Validation

- Backup file must exist and be readable
- Checksum must match if present in metadata
- Database must be valid SQLite format
- Schema version must be compatible (same or older)

## Configuration Changes

### New Config Section

Add to `internal/config/config.go`:

```go
type Config struct {
    // ... existing fields ...
    Backup BackupConfig `mapstructure:"backup"`
}

type BackupConfig struct {
    Directory string         `mapstructure:"directory"` // Default: "{storage.base_dir}/backups"
    Schedule  ScheduleConfig `mapstructure:"schedule"`
}

type ScheduleConfig struct {
    Enabled   bool   `mapstructure:"enabled"`   // Default: false
    Cron      string `mapstructure:"cron"`      // Default: "0 0 2 * * *" (daily 2 AM)
    Retention int    `mapstructure:"retention"` // Default: 7
}
```

### Default Values

```go
// In SetDefaults()
v.SetDefault("backup.directory", "") // Empty = {storage.base_dir}/backups
v.SetDefault("backup.schedule.enabled", false)
v.SetDefault("backup.schedule.cron", "0 0 2 * * *")
v.SetDefault("backup.schedule.retention", 7)
```
