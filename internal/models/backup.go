package models

import "time"

// BackupMetadata represents a backup file's metadata.
// This is derived from filesystem scanning and companion metadata files,
// not stored in the database.
type BackupMetadata struct {
	Filename       string      `json:"filename"`        // e.g., "tvarr-backup-2025-12-14T10-30-00.db.gz"
	FilePath       string      `json:"file_path"`       // Full path to backup file
	CreatedAt      time.Time   `json:"created_at"`      // Extracted from filename
	FileSize       int64       `json:"file_size"`       // Size in bytes
	Checksum       string      `json:"checksum"`        // SHA256 hash for integrity verification
	TvarrVersion   string      `json:"tvarr_version"`   // Version that created the backup (from metadata file)
	DatabaseSize   int64       `json:"database_size"`   // Uncompressed size
	CompressedSize int64       `json:"compressed_size"` // Gzip compressed size
	TableCounts    TableCounts `json:"table_counts"`    // Row counts per table
	Protected      bool        `json:"protected"`       // If true, excluded from retention cleanup
	Imported       bool        `json:"imported"`        // If true, backup was uploaded/imported
}

// TableCounts holds row counts for key tables in a backup.
type TableCounts struct {
	Channels             int `json:"channels"`
	EPGPrograms          int `json:"epg_programs"`
	Filters              int `json:"filters"`
	DataMappingRules     int `json:"data_mapping_rules"`
	ClientDetectionRules int `json:"client_detection_rules"`
	EncodingProfiles     int `json:"encoding_profiles"`
	StreamSources        int `json:"stream_sources"`
	EPGSources           int `json:"epg_sources"`
	StreamProxies        int `json:"stream_proxies"`
}

// BackupMetadataFile is the structure stored in companion .meta.json files.
type BackupMetadataFile struct {
	TvarrVersion   string         `json:"tvarr_version"`
	DatabaseSize   int64          `json:"database_size"`   // Uncompressed size
	CompressedSize int64          `json:"compressed_size"` // Gzip compressed size
	Checksum       string         `json:"checksum"`        // SHA256 of .db.gz file
	CreatedAt      time.Time      `json:"created_at"`
	TableCounts    map[string]int `json:"table_counts"` // Row counts per table
	Protected      bool           `json:"protected"`    // If true, excluded from retention cleanup
	Imported       bool           `json:"imported"`     // If true, backup was uploaded/imported
}

// ToTableCounts converts the map-based table counts to the structured TableCounts type.
func (m *BackupMetadataFile) ToTableCounts() TableCounts {
	return TableCounts{
		Channels:             m.TableCounts["channels"],
		EPGPrograms:          m.TableCounts["epg_programs"],
		Filters:              m.TableCounts["filters"],
		DataMappingRules:     m.TableCounts["data_mapping_rules"],
		ClientDetectionRules: m.TableCounts["client_detection_rules"],
		EncodingProfiles:     m.TableCounts["encoding_profiles"],
		StreamSources:        m.TableCounts["stream_sources"],
		EPGSources:           m.TableCounts["epg_sources"],
		StreamProxies:        m.TableCounts["stream_proxies"],
	}
}

// BackupScheduleInfo represents the backup schedule configuration for API responses.
type BackupScheduleInfo struct {
	Enabled   bool    `json:"enabled"`
	Cron      string  `json:"cron"`
	Retention int     `json:"retention"`
	NextRun   *string `json:"next_run,omitempty"`
}

// BackupSettings stores user-configurable backup settings in the database.
// This is a singleton table (only one row with ID=1).
type BackupSettings struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	Enabled   *bool  `gorm:"default:null" json:"enabled"`
	Cron      string `gorm:"size:100" json:"cron,omitempty"`
	Retention *int   `gorm:"default:null" json:"retention,omitempty"`
}

// TableName returns the table name for BackupSettings.
func (BackupSettings) TableName() string {
	return "backup_settings"
}

// ToScheduleInfo converts BackupSettings to BackupScheduleInfo, using defaults for nil values.
func (s *BackupSettings) ToScheduleInfo(defaultEnabled bool, defaultCron string, defaultRetention int) BackupScheduleInfo {
	enabled := defaultEnabled
	if s != nil && s.Enabled != nil {
		enabled = *s.Enabled
	}

	cron := defaultCron
	if s != nil && s.Cron != "" {
		cron = s.Cron
	}

	retention := defaultRetention
	if s != nil && s.Retention != nil {
		retention = *s.Retention
	}

	return BackupScheduleInfo{
		Enabled:   enabled,
		Cron:      cron,
		Retention: retention,
	}
}
