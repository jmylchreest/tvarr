// Package migrations provides database migration management for tvarr.
package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// AllMigrations returns all registered migrations in order.
func AllMigrations() []Migration {
	return []Migration{
		// Phase 3: Stream Source Management
		migration001StreamSources(),
		migration002Channels(),
		migration003ManualStreamChannels(),

		// Phase 4: EPG Source Management
		migration004EpgSources(),
		migration005EpgPrograms(),

		// Phase 5: Proxy Configuration
		migration006StreamProxies(),
		migration007ProxySources(),
		migration008ProxyEpgSources(),
		migration009ProxyFilters(),
		migration010ProxyMappingRules(),

		// Phase 6.5: Expression Engine - Filters and Data Mapping Rules
		migration011Filters(),
		migration012DataMappingRules(),
		migration013DefaultFiltersAndRules(),

		// Phase 12: Relay System
		migration014RelayProfiles(),
		migration015DefaultRelayProfiles(),

		// Schema updates for system defaults protection
		migration016AddIsSystemColumn(),

		// Schema updates for EpgSource timezone fields
		migration017EpgSourceTimezoneFields(),

		// Fix channel unique index to be per-source
		migration018ChannelCompositeUniqueIndex(),

		// Note: Logo caching (Phase 10) uses file-based storage with
		// in-memory indexing, no database tables required.
	}
}

// migration001StreamSources creates the stream_sources table.
func migration001StreamSources() Migration {
	return Migration{
		Version:     "001",
		Description: "Create stream_sources table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.StreamSource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("stream_sources")
		},
	}
}

// migration002Channels creates the channels table.
func migration002Channels() Migration {
	return Migration{
		Version:     "002",
		Description: "Create channels table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.Channel{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("channels")
		},
	}
}

// migration003ManualStreamChannels creates the manual_stream_channels table.
func migration003ManualStreamChannels() Migration {
	return Migration{
		Version:     "003",
		Description: "Create manual_stream_channels table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ManualStreamChannel{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("manual_stream_channels")
		},
	}
}

// migration004EpgSources creates the epg_sources table.
func migration004EpgSources() Migration {
	return Migration{
		Version:     "004",
		Description: "Create epg_sources table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.EpgSource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("epg_sources")
		},
	}
}

// migration005EpgPrograms creates the epg_programs table.
func migration005EpgPrograms() Migration {
	return Migration{
		Version:     "005",
		Description: "Create epg_programs table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.EpgProgram{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("epg_programs")
		},
	}
}

// migration006StreamProxies creates the stream_proxies table.
func migration006StreamProxies() Migration {
	return Migration{
		Version:     "006",
		Description: "Create stream_proxies table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.StreamProxy{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("stream_proxies")
		},
	}
}

// migration007ProxySources creates the proxy_sources join table.
func migration007ProxySources() Migration {
	return Migration{
		Version:     "007",
		Description: "Create proxy_sources join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxySource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_sources")
		},
	}
}

// migration008ProxyEpgSources creates the proxy_epg_sources join table.
func migration008ProxyEpgSources() Migration {
	return Migration{
		Version:     "008",
		Description: "Create proxy_epg_sources join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxyEpgSource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_epg_sources")
		},
	}
}

// migration009ProxyFilters creates the proxy_filters join table.
func migration009ProxyFilters() Migration {
	return Migration{
		Version:     "009",
		Description: "Create proxy_filters join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxyFilter{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_filters")
		},
	}
}

// migration010ProxyMappingRules creates the proxy_mapping_rules join table.
func migration010ProxyMappingRules() Migration {
	return Migration{
		Version:     "010",
		Description: "Create proxy_mapping_rules join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxyMappingRule{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_mapping_rules")
		},
	}
}

// migration011Filters creates the filters table for expression-based filtering.
func migration011Filters() Migration {
	return Migration{
		Version:     "011",
		Description: "Create filters table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.Filter{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("filters")
		},
	}
}

// migration012DataMappingRules creates the data_mapping_rules table.
func migration012DataMappingRules() Migration {
	return Migration{
		Version:     "012",
		Description: "Create data_mapping_rules table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.DataMappingRule{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("data_mapping_rules")
		},
	}
}

// migration013DefaultFiltersAndRules inserts default filters and data mapping rules.
func migration013DefaultFiltersAndRules() Migration {
	return Migration{
		Version:     "013",
		Description: "Insert default filters and data mapping rules",
		Up: func(tx *gorm.DB) error {
			// Create default filters matching m3u-proxy
			defaultFilters := []models.Filter{
				{
					Name:       "Include All Valid Stream URLs",
					SourceType: models.FilterSourceTypeStream,
					Action:     models.FilterActionInclude,
					Expression: `stream_url starts_with "http"`,
					Priority:   0,
					IsEnabled:  true,
					IsSystem:   true,
				},
				{
					Name:        "Exclude Adult Content",
					Description: "Excludes channels with adult content keywords in name or group",
					SourceType:  models.FilterSourceTypeStream,
					Action:      models.FilterActionExclude,
					Expression:  `group_title contains "adult" OR group_title contains "xxx" OR group_title contains "porn" OR channel_name contains "adult" OR channel_name contains "xxx" OR channel_name contains "porn"`,
					Priority:    1,
					IsEnabled:   true,
					IsSystem:    true,
				},
			}

			for _, filter := range defaultFilters {
				if err := tx.Create(&filter).Error; err != nil {
					return err
				}
			}

			// Create default data mapping rules matching m3u-proxy
			defaultRules := []models.DataMappingRule{
				{
					Name:        "Default Timeshift Detection (Regex)",
					Description: "Automatically detects timeshift channels (+1, +24, etc.) and sets tvg-shift field using regex capture groups.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `channel_name matches ".*[ ](?:\\+([0-9]{1,2})|(-[0-9]{1,2}))([hH]?)(?:$|[ ]).*" AND channel_name not matches ".*(?:start:|stop:|24[-/]7).*" AND tvg_id matches "^.+$" SET tvg_shift = "$1$2"`,
					Priority:    1,
					StopOnMatch: false,
					IsEnabled:   true,
					IsSystem:    true,
				},
			}

			for _, rule := range defaultRules {
				if err := tx.Create(&rule).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Delete default filters
			if err := tx.Where("name IN ?", []string{
				"Include All Valid Stream URLs",
				"Exclude Adult Content",
			}).Delete(&models.Filter{}).Error; err != nil {
				return err
			}

			// Delete default data mapping rules
			if err := tx.Where("name = ?", "Default Timeshift Detection (Regex)").Delete(&models.DataMappingRule{}).Error; err != nil {
				return err
			}

			return nil
		},
	}
}

// migration014RelayProfiles creates the relay_profiles table.
func migration014RelayProfiles() Migration {
	return Migration{
		Version:     "014",
		Description: "Create relay_profiles table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.RelayProfile{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("relay_profiles")
		},
	}
}

// migration015DefaultRelayProfiles inserts default relay profiles matching m3u-proxy.
func migration015DefaultRelayProfiles() Migration {
	return Migration{
		Version:     "015",
		Description: "Insert default relay profiles",
		Up: func(tx *gorm.DB) error {
			// Create default relay profiles matching m3u-proxy
			defaultProfiles := []models.RelayProfile{
				{
					Name:            "H.264 + AAC (Standard)",
					Description:     "Maximum compatibility profile with H.264 video and AAC audio",
					IsDefault:       true,
					Enabled:         true,
					IsSystem:        true,
					VideoCodec:      models.VideoCodecH264,
					VideoBitrate:    2000,
					VideoPreset:     "fast",
					VideoProfile:    "main",
					AudioCodec:      models.AudioCodecAAC,
					AudioBitrate:    128,
					AudioSampleRate: 48000,
					AudioChannels:   2,
					OutputFormat:    models.OutputFormatMPEGTS,
					Timeout:         30,
				},
				{
					Name:            "H.265 + AAC (Standard)",
					Description:     "Better compression with H.265 video and AAC audio",
					IsDefault:       false,
					Enabled:         true,
					IsSystem:        true,
					VideoCodec:      models.VideoCodecH265,
					VideoBitrate:    1500,
					VideoPreset:     "fast",
					VideoProfile:    "main",
					AudioCodec:      models.AudioCodecAAC,
					AudioBitrate:    128,
					AudioSampleRate: 48000,
					AudioChannels:   2,
					OutputFormat:    models.OutputFormatMPEGTS,
					Timeout:         30,
				},
				{
					Name:            "H.264 + AAC (High Quality)",
					Description:     "High quality H.264 profile for better video quality",
					IsDefault:       false,
					Enabled:         true,
					IsSystem:        true,
					VideoCodec:      models.VideoCodecH264,
					VideoBitrate:    4000,
					VideoPreset:     "slower",
					VideoProfile:    "high",
					AudioCodec:      models.AudioCodecAAC,
					AudioBitrate:    192,
					AudioSampleRate: 48000,
					AudioChannels:   2,
					OutputFormat:    models.OutputFormatMPEGTS,
					Timeout:         30,
				},
				{
					Name:            "H.264 + AAC (Low Bitrate)",
					Description:     "Low bitrate H.264 profile for mobile devices or limited bandwidth",
					IsDefault:       false,
					Enabled:         true,
					IsSystem:        true,
					VideoCodec:      models.VideoCodecH264,
					VideoBitrate:    800,
					VideoPreset:     "veryfast",
					VideoProfile:    "baseline",
					AudioCodec:      models.AudioCodecAAC,
					AudioBitrate:    96,
					AudioSampleRate: 48000,
					AudioChannels:   2,
					OutputFormat:    models.OutputFormatMPEGTS,
					Timeout:         30,
				},
				{
					Name:            "H.265 + AAC (High Quality)",
					Description:     "High quality H.265/HEVC profile with better compression",
					IsDefault:       false,
					Enabled:         true,
					IsSystem:        true,
					VideoCodec:      models.VideoCodecH265,
					VideoBitrate:    3000,
					VideoPreset:     "slow",
					VideoProfile:    "main",
					AudioCodec:      models.AudioCodecAAC,
					AudioBitrate:    192,
					AudioSampleRate: 48000,
					AudioChannels:   2,
					OutputFormat:    models.OutputFormatMPEGTS,
					Timeout:         30,
				},
				{
					Name:            "AV1 + AAC (Next-gen)",
					Description:     "Next-generation AV1 codec for best compression efficiency",
					IsDefault:       false,
					Enabled:         true,
					IsSystem:        true,
					VideoCodec:      models.VideoCodecAV1,
					VideoBitrate:    2500,
					VideoPreset:     "medium",
					AudioCodec:      models.AudioCodecAAC,
					AudioBitrate:    128,
					AudioSampleRate: 48000,
					AudioChannels:   2,
					OutputFormat:    models.OutputFormatMPEGTS,
					Timeout:         30,
				},
				{
					Name:         "Copy Streams (No Transcoding)",
					Description:  "Pass-through profile that copies streams without transcoding",
					IsDefault:    false,
					Enabled:      true,
					IsSystem:     true,
					VideoCodec:   models.VideoCodecCopy,
					AudioCodec:   models.AudioCodecCopy,
					OutputFormat: models.OutputFormatMPEGTS,
					Timeout:      30,
				},
			}

			for _, profile := range defaultProfiles {
				if err := tx.Create(&profile).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Delete default relay profiles
			return tx.Where("name IN ?", []string{
				"H.264 + AAC (Standard)",
				"H.265 + AAC (Standard)",
				"H.264 + AAC (High Quality)",
				"H.264 + AAC (Low Bitrate)",
				"H.265 + AAC (High Quality)",
				"AV1 + AAC (Next-gen)",
				"Copy Streams (No Transcoding)",
			}).Delete(&models.RelayProfile{}).Error
		},
	}
}

// migration016AddIsSystemColumn adds is_system column and marks existing defaults.
func migration016AddIsSystemColumn() Migration {
	return Migration{
		Version:     "016",
		Description: "Add is_system column to filters, data_mapping_rules, and relay_profiles",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate will add the new column (is_system) to existing tables
			if err := tx.AutoMigrate(&models.Filter{}); err != nil {
				return err
			}
			if err := tx.AutoMigrate(&models.DataMappingRule{}); err != nil {
				return err
			}
			if err := tx.AutoMigrate(&models.RelayProfile{}); err != nil {
				return err
			}

			// Mark existing default filters as system (use UpdateColumn to bypass hooks)
			defaultFilterNames := []string{
				"Include All Valid Stream URLs",
				"Exclude Adult Content",
			}
			if err := tx.Model(&models.Filter{}).
				Where("name IN ?", defaultFilterNames).
				UpdateColumn("is_system", true).Error; err != nil {
				return err
			}

			// Mark existing default data mapping rules as system (use UpdateColumn to bypass hooks)
			defaultRuleNames := []string{
				"Default Timeshift Detection (Regex)",
			}
			if err := tx.Model(&models.DataMappingRule{}).
				Where("name IN ?", defaultRuleNames).
				UpdateColumn("is_system", true).Error; err != nil {
				return err
			}

			// Mark existing default relay profiles as system (use UpdateColumn to bypass hooks)
			defaultProfileNames := []string{
				"H.264 + AAC (Standard)",
				"H.265 + AAC (Standard)",
				"H.264 + AAC (High Quality)",
				"H.264 + AAC (Low Bitrate)",
				"H.265 + AAC (High Quality)",
				"AV1 + AAC (Next-gen)",
				"Copy Streams (No Transcoding)",
			}
			if err := tx.Model(&models.RelayProfile{}).
				Where("name IN ?", defaultProfileNames).
				UpdateColumn("is_system", true).Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Reset is_system to false (use UpdateColumn to bypass hooks)
			if err := tx.Model(&models.Filter{}).Where("is_system = ?", true).UpdateColumn("is_system", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.DataMappingRule{}).Where("is_system = ?", true).UpdateColumn("is_system", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.RelayProfile{}).Where("is_system = ?", true).UpdateColumn("is_system", false).Error; err != nil {
				return err
			}
			return nil
		},
	}
}

// migration017EpgSourceTimezoneFields adds timezone fields to epg_sources table.
func migration017EpgSourceTimezoneFields() Migration {
	return Migration{
		Version:     "017",
		Description: "Add original_timezone and time_offset columns to epg_sources",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate will add the new columns to the existing table
			return tx.AutoMigrate(&models.EpgSource{})
		},
		Down: func(tx *gorm.DB) error {
			// Drop the columns (SQLite doesn't support DROP COLUMN directly,
			// but GORM handles this appropriately for each database)
			migrator := tx.Migrator()
			if migrator.HasColumn(&models.EpgSource{}, "original_timezone") {
				if err := migrator.DropColumn(&models.EpgSource{}, "original_timezone"); err != nil {
					return err
				}
			}
			if migrator.HasColumn(&models.EpgSource{}, "time_offset") {
				if err := migrator.DropColumn(&models.EpgSource{}, "time_offset"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// migration018ChannelCompositeUniqueIndex fixes the channel ext_id unique index
// to be a composite unique index on (source_id, ext_id) instead of just ext_id.
// This allows the same ext_id to exist across different sources.
func migration018ChannelCompositeUniqueIndex() Migration {
	return Migration{
		Version:     "018",
		Description: "Fix channel unique index to be per-source composite (source_id, ext_id)",
		Up: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Drop the old unique index on ext_id if it exists
			if migrator.HasIndex(&models.Channel{}, "idx_source_ext_id") {
				if err := migrator.DropIndex(&models.Channel{}, "idx_source_ext_id"); err != nil {
					// Ignore errors - the index might not exist or be named differently
					tx.Logger.Warn(tx.Statement.Context, "failed to drop old index: %v", err)
				}
			}

			// AutoMigrate will create the new composite unique index
			return tx.AutoMigrate(&models.Channel{})
		},
		Down: func(tx *gorm.DB) error {
			// This migration cannot be safely reverted without data loss
			// as it would require a global unique constraint that might fail
			// if there are duplicate ext_ids across sources.
			// Just do nothing on down - the index will remain as composite.
			return nil
		},
	}
}


