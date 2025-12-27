// Package migrations provides database migration management for tvarr.
package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// AllMigrations returns all registered migrations in order.
// This is a compacted migration set for new installations.
// Previous migrations have been consolidated into:
// - 001: Schema creation using GORM AutoMigrate
// - 002: System data (default filters, rules)
// - 003: Legacy cleanup (no-op)
// - 004: Add EncodingProfile
// - 005: Add ClientDetectionRule
// - 006: Add explicit codec header detection rules
// - 007: Rename EPG timezone fields (original_timezone->detected_timezone, time_offset->epg_shift)
// - 008: Remove redundant priority column from filters table
// - 009: Add dynamic codec header fields to client detection rules
// - 010: Update client detection rules to use @dynamic() syntax for user-agent
// - 011: Add is_active column to proxy_filters
// - 012: Add auto_shift_timezone column to epg_sources for timezone auto-shift tracking
// - 013: Add system client detection rules for popular media players (VLC, MPV, Kodi, Plex, Jellyfin, Emby)
// - 014: Rename system timeshift detection rule to shorter name
// - 015: Add default channel grouping data mapping rules and filters
// - 016: Fix grouping rules: enable country/adult, reorder priorities, rename to Group
// - 017: Remove is_enabled column from filters (filters are enabled/disabled at proxy level)
// - 018: Add backup_settings table for user-configurable backup schedule
// - 019: Fix duplicate "Exclude Adult Content" filters and upgrade expression
// - 020: Add ffmpegd_config table for distributed transcoding configuration
// - 021: Add max_concurrent_streams column to stream_sources table
// - 022: Add encoder_overrides table for hardware encoder workarounds
// - 023: Remove deprecated client_detection_enabled column from stream_proxies
func AllMigrations() []Migration {
	return []Migration{
		migration001Schema(),
		migration002SystemData(),
		migration003LegacyCleanup(),
		migration004EncodingProfiles(),
		migration005ClientDetection(),
		migration006ExplicitCodecHeaders(),
		migration007EpgTimezoneFields(),
		migration008RemoveFilterPriority(),
		migration009DynamicCodecHeaders(),
		migration010DynamicUserAgent(),
		migration011ProxyFilterIsActive(),
		migration012EpgAutoShiftTimezone(),
		migration013SystemClientDetectionRules(),
		migration014RenameTimeshiftRule(),
		migration015DefaultGroupingRules(),
		migration016FixGroupingRules(),
		migration017RemoveFilterIsEnabled(),
		migration018BackupSettings(),
		migration019FixDuplicateFilters(),
		migration020FFmpegdConfig(),
		migration021StreamSourceMaxConcurrent(),
		migration022EncoderOverrides(),
		migration023RemoveClientDetectionEnabled(),
		migration024FixDynamicCodecRules(),
	}
}

// migration001Schema creates all database tables using GORM AutoMigrate.
func migration001Schema() Migration {
	return Migration{
		Version:     "001",
		Description: "Create all database tables",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate all models in dependency order
			return tx.AutoMigrate(
				// Core tables
				&models.StreamSource{},
				&models.Channel{},
				&models.ManualStreamChannel{},
				&models.EpgSource{},
				&models.EpgProgram{},

				// Proxy configuration
				&models.StreamProxy{},

				// Proxy join tables
				&models.ProxySource{},
				&models.ProxyEpgSource{},
				&models.ProxyFilter{},
				&models.ProxyMappingRule{},

				// Expression engine
				&models.Filter{},
				&models.DataMappingRule{},

				// Scheduler
				&models.Job{},
				&models.JobHistory{},

				// Codec caching
				&models.LastKnownCodec{},
			)
		},
		Down: func(tx *gorm.DB) error {
			// Drop tables in reverse dependency order
			tables := []string{
				"last_known_codecs",
				"job_histories",
				"jobs",
				"data_mapping_rules",
				"filters",
				"proxy_mapping_rules",
				"proxy_filters",
				"proxy_epg_sources",
				"proxy_sources",
				"stream_proxies",
				"epg_programs",
				"epg_sources",
				"manual_stream_channels",
				"channels",
				"stream_sources",
			}
			for _, table := range tables {
				if tx.Migrator().HasTable(table) {
					if err := tx.Migrator().DropTable(table); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

// migration002SystemData inserts default system data.
func migration002SystemData() Migration {
	return Migration{
		Version:     "002",
		Description: "Insert default filters and rules",
		Up: func(tx *gorm.DB) error {
			// Create default filters
			if err := createDefaultFilters(tx); err != nil {
				return err
			}

			// Create default data mapping rules
			if err := createDefaultDataMappingRules(tx); err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Delete system data in reverse order
			if err := tx.Where("is_system = ?", true).Delete(&models.DataMappingRule{}).Error; err != nil {
				return err
			}
			if err := tx.Where("is_system = ?", true).Delete(&models.Filter{}).Error; err != nil {
				return err
			}
			return nil
		},
	}
}

// migration003LegacyCleanup is a no-op migration for legacy cleanup.
// Previously cleaned up RelayProfile data but that system has been removed.
func migration003LegacyCleanup() Migration {
	return Migration{
		Version:     "003",
		Description: "Legacy cleanup (no-op)",
		Up: func(tx *gorm.DB) error {
			// No-op - RelayProfile system has been removed
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// No-op
			return nil
		},
	}
}

// createDefaultFilters creates the default system filters.
func createDefaultFilters(tx *gorm.DB) error {
	filters := []models.Filter{
		{
			Name:       "Include All Valid Stream URLs",
			SourceType: models.FilterSourceTypeStream,
			Action:     models.FilterActionInclude,
			Expression: `stream_url starts_with "http"`,
			IsSystem:   true,
		},
		{
			Name:        "Exclude Adult Content",
			Description: "Excludes channels with adult content keywords in name or group",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionExclude,
			Expression:  `group_title contains "adult" OR group_title contains "xxx" OR group_title contains "porn" OR channel_name contains "adult" OR channel_name contains "xxx" OR channel_name contains "porn"`,
			IsSystem:    true,
		},
	}

	for _, filter := range filters {
		if err := tx.Create(&filter).Error; err != nil {
			return err
		}
	}
	return nil
}

// createDefaultDataMappingRules creates the default system data mapping rules.
func createDefaultDataMappingRules(tx *gorm.DB) error {
	rules := []models.DataMappingRule{
		{
			Name:        "Timeshift Detection",
			Description: "Automatically detects timeshift channels (+1, +24, etc.) and sets tvg-shift field using regex capture groups.",
			SourceType:  models.DataMappingRuleSourceTypeStream,
			Expression:  `channel_name matches ".*[ ](?:\\+([0-9]{1,2})|(-[0-9]{1,2}))([hH]?)(?:$|[ ]).*" AND channel_name not matches ".*(?:start:|stop:|24[-/]7).*" AND tvg_id matches "^.+$" SET tvg_shift = "$1$2"`,
			Priority:    1,
			StopOnMatch: false,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
		},
	}

	for _, rule := range rules {
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}
