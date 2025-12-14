package models

import "time"

// BackupMetadata represents a backup file's metadata.
// This is derived from filesystem scanning and companion metadata files,
// not stored in the database.
type BackupMetadata struct {
	Filename       string    `json:"filename"`        // e.g., "tvarr-backup-2025-12-14T10-30-00.db.gz"
	FilePath       string    `json:"file_path"`       // Full path to backup file
	CreatedAt      time.Time `json:"created_at"`      // Extracted from filename
	FileSize       int64     `json:"file_size"`       // Size in bytes
	Checksum       string    `json:"checksum"`        // SHA256 hash for integrity verification
	TvarrVersion   string    `json:"tvarr_version"`   // Version that created the backup (from metadata file)
	DatabaseSize   int64     `json:"database_size"`   // Uncompressed size
	CompressedSize int64     `json:"compressed_size"` // Gzip compressed size
	TableCounts    TableCounts `json:"table_counts"`  // Row counts per table
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
	TvarrVersion   string            `json:"tvarr_version"`
	DatabaseSize   int64             `json:"database_size"`   // Uncompressed size
	CompressedSize int64             `json:"compressed_size"` // Gzip compressed size
	Checksum       string            `json:"checksum"`        // SHA256 of .db.gz file
	CreatedAt      time.Time         `json:"created_at"`
	TableCounts    map[string]int    `json:"table_counts"` // Row counts per table
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
	Enabled   bool   `json:"enabled"`
	Cron      string `json:"cron"`
	Retention int    `json:"retention"`
}
