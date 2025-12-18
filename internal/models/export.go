package models

import "time"

// ExportFormatVersion is the current version of the export file format.
// This is used to ensure compatibility when importing configuration files.
const ExportFormatVersion = "1.0.0"

// ExportType represents the type of configuration being exported.
type ExportType string

const (
	// ExportTypeFilters indicates a filters export.
	ExportTypeFilters ExportType = "filters"
	// ExportTypeDataMappingRules indicates a data mapping rules export.
	ExportTypeDataMappingRules ExportType = "data_mapping_rules"
	// ExportTypeClientDetectionRules indicates a client detection rules export.
	ExportTypeClientDetectionRules ExportType = "client_detection_rules"
	// ExportTypeEncodingProfiles indicates an encoding profiles export.
	ExportTypeEncodingProfiles ExportType = "encoding_profiles"
)

// ConflictResolution represents how to handle import conflicts.
type ConflictResolution string

const (
	// ConflictResolutionSkip skips importing the conflicting item.
	ConflictResolutionSkip ConflictResolution = "skip"
	// ConflictResolutionRename renames the imported item with a suffix.
	ConflictResolutionRename ConflictResolution = "rename"
	// ConflictResolutionOverwrite overwrites the existing item with imported values.
	ConflictResolutionOverwrite ConflictResolution = "overwrite"
)

// ConfigExport is the generic wrapper for all config exports.
type ConfigExport[T any] struct {
	Metadata ExportMetadata `json:"metadata"`
	Items    []T            `json:"items"`
}

// ExportMetadata contains export file metadata.
type ExportMetadata struct {
	Version      string     `json:"version"`       // Export format version (e.g., "1.0.0")
	TvarrVersion string     `json:"tvarr_version"` // tvarr application version
	ExportType   ExportType `json:"export_type"`   // "filters", "data_mapping_rules", etc.
	ExportedAt   time.Time  `json:"exported_at"`   // UTC timestamp
	ItemCount    int        `json:"item_count"`    // Number of items in export
}

// FilterExportItem represents a filter for export/import.
// Note: Filters do not have an enabled/disabled state. The enabled state is
// controlled at the proxy-filter relationship level (ProxyFilter.IsActive).
type FilterExportItem struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Expression  string  `json:"expression"`
	SourceType  string  `json:"source_type"` // "stream" or "epg"
	Action      string  `json:"action"`      // "include" or "exclude"
	SourceID    *string `json:"source_id,omitempty"` // Optional source restriction
}

// DataMappingRuleExportItem represents a data mapping rule for export/import.
type DataMappingRuleExportItem struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Expression  string  `json:"expression"`
	SourceType  string  `json:"source_type"` // "stream" or "epg"
	Priority    int     `json:"priority"`
	StopOnMatch bool    `json:"stop_on_match"`
	IsEnabled   bool    `json:"is_enabled"`
	SourceID    *string `json:"source_id,omitempty"`
}

// ClientDetectionRuleExportItem represents a client detection rule for export/import.
// AcceptedVideoCodecs and AcceptedAudioCodecs are stored as JSON arrays in the model
// but exported as slice for human readability.
type ClientDetectionRuleExportItem struct {
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Expression          string   `json:"expression"`
	Priority            int      `json:"priority"`
	IsEnabled           bool     `json:"is_enabled"`
	AcceptedVideoCodecs []string `json:"accepted_video_codecs,omitempty"` // Decoded from JSON array
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs,omitempty"` // Decoded from JSON array
	PreferredVideoCodec string   `json:"preferred_video_codec,omitempty"`
	PreferredAudioCodec string   `json:"preferred_audio_codec,omitempty"`
	SupportsFMP4        bool     `json:"supports_fmp4"`
	SupportsMPEGTS      bool     `json:"supports_mpegts"`
	PreferredFormat     string   `json:"preferred_format,omitempty"`
	EncodingProfileName *string  `json:"encoding_profile_name,omitempty"` // Reference by name, not ID
}

// EncodingProfileExportItem represents an encoding profile for export/import.
type EncodingProfileExportItem struct {
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	TargetVideoCodec string `json:"target_video_codec"`
	TargetAudioCodec string `json:"target_audio_codec"`
	QualityPreset    string `json:"quality_preset"` // low, medium, high, ultra
	HWAccel          string `json:"hw_accel"`       // auto, none, cuda, vaapi, qsv, videotoolbox
	GlobalFlags      string `json:"global_flags,omitempty"`
	InputFlags       string `json:"input_flags,omitempty"`
	OutputFlags      string `json:"output_flags,omitempty"`
	IsDefault        bool   `json:"is_default"`
	Enabled          bool   `json:"enabled"`
}

// ImportPreview shows what will happen if import proceeds.
type ImportPreview struct {
	TotalItems     int            `json:"total_items"`
	NewItems       []PreviewItem  `json:"new_items"`
	Conflicts      []ConflictItem `json:"conflicts"`
	Errors         []ImportError  `json:"errors"`
	VersionWarning string         `json:"version_warning,omitempty"` // Warning if exported from different tvarr version
}

// PreviewItem represents an item that will be imported without conflict.
type PreviewItem struct {
	Name string `json:"name"`
}

// ConflictItem represents an item with a name conflict.
type ConflictItem struct {
	ImportName   string             `json:"import_name"`
	ExistingID   string             `json:"existing_id"`
	ExistingName string             `json:"existing_name"`
	Resolution   ConflictResolution `json:"resolution"` // "skip", "rename", "overwrite" - default "skip"
}

// ImportResult summarizes an import operation.
type ImportResult struct {
	TotalItems    int            `json:"total_items"`
	Imported      int            `json:"imported"`
	Skipped       int            `json:"skipped"`
	Overwritten   int            `json:"overwritten"`
	Renamed       int            `json:"renamed"`
	Errors        int            `json:"errors"`
	ErrorDetails  []ImportError  `json:"error_details,omitempty"`
	ImportedItems []ImportedItem `json:"imported_items"`
}

// ImportError describes a single import error.
type ImportError struct {
	ItemName string `json:"item_name"`
	Error    string `json:"error"`
}

// ImportedItem describes a successfully imported item.
type ImportedItem struct {
	OriginalName string `json:"original_name"`
	FinalName    string `json:"final_name"` // May differ if renamed
	ID           string `json:"id"`         // New or existing ID
	Action       string `json:"action"`     // "created", "overwritten", "renamed"
}
