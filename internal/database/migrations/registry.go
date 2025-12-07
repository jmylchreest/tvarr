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

		// Rename order column to priority for consistency
		migration019RenameProxyFilterOrderToPriority(),

		// Add API method field for Xtream EPG sources
		migration020EpgSourceApiMethod(),

		// Job and job history tables for scheduler
		migration021JobsTable(),

		// Add hls_collapse column to stream_proxies table
		migration022StreamProxyHLSCollapse(),

		// FFmpeg profile configuration extensions (feature 007)
		migration023RelayProfileExtensions(),

		// Update system profiles with hwaccel and fallback enabled
		migration024SystemProfileHWAccel(),

		// Codec caching for ffprobe pre-detection
		migration025LastKnownCodecs(),

		// Smart codec matching flags for relay profiles
		migration026ForceTranscodeFlags(),

		// Multi-format streaming support (feature 008)
		migration027MultiFormatStreaming(),

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
				// Note: AV1 profile removed - AV1 is not supported in MPEG-TS containers
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

// migration019RenameProxyFilterOrderToPriority renames the 'order' column to 'priority'
// in proxy_filters and proxy_mapping_rules tables for consistency with other proxy tables.
func migration019RenameProxyFilterOrderToPriority() Migration {
	return Migration{
		Version:     "019",
		Description: "Rename order column to priority in proxy_filters and proxy_mapping_rules",
		Up: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Rename 'order' to 'priority' in proxy_filters
			if migrator.HasColumn(&models.ProxyFilter{}, "order") {
				if err := migrator.RenameColumn(&models.ProxyFilter{}, "order", "priority"); err != nil {
					return err
				}
			}

			// Rename 'order' to 'priority' in proxy_mapping_rules
			if migrator.HasColumn(&models.ProxyMappingRule{}, "order") {
				if err := migrator.RenameColumn(&models.ProxyMappingRule{}, "order", "priority"); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Rename 'priority' back to 'order' in proxy_filters
			if migrator.HasColumn(&models.ProxyFilter{}, "priority") {
				if err := migrator.RenameColumn(&models.ProxyFilter{}, "priority", "order"); err != nil {
					return err
				}
			}

			// Rename 'priority' back to 'order' in proxy_mapping_rules
			if migrator.HasColumn(&models.ProxyMappingRule{}, "priority") {
				if err := migrator.RenameColumn(&models.ProxyMappingRule{}, "priority", "order"); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

// migration020EpgSourceApiMethod adds api_method column to epg_sources table
// for selecting between Xtream API methods (stream_id JSON vs bulk XMLTV).
func migration020EpgSourceApiMethod() Migration {
	return Migration{
		Version:     "020",
		Description: "Add api_method column to epg_sources for Xtream API method selection",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate will add the new column to the existing table
			return tx.AutoMigrate(&models.EpgSource{})
		},
		Down: func(tx *gorm.DB) error {
			// Drop the column
			migrator := tx.Migrator()
			if migrator.HasColumn(&models.EpgSource{}, "api_method") {
				if err := migrator.DropColumn(&models.EpgSource{}, "api_method"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// migration021JobsTable creates the jobs and job_history tables for the scheduler.
func migration021JobsTable() Migration {
	return Migration{
		Version:     "021",
		Description: "Create jobs and job_history tables for scheduler",
		Up: func(tx *gorm.DB) error {
			// Create jobs table
			if err := tx.AutoMigrate(&models.Job{}); err != nil {
				return err
			}
			// Create job_history table
			return tx.AutoMigrate(&models.JobHistory{})
		},
		Down: func(tx *gorm.DB) error {
			// Drop job_history first (depends on jobs)
			if err := tx.Migrator().DropTable("job_history"); err != nil {
				return err
			}
			return tx.Migrator().DropTable("jobs")
		},
	}
}

// migration022StreamProxyHLSCollapse adds the hls_collapse column to stream_proxies table.
func migration022StreamProxyHLSCollapse() Migration {
	return Migration{
		Version:     "022",
		Description: "Add hls_collapse column to stream_proxies",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate will add the new column to the existing table
			return tx.AutoMigrate(&models.StreamProxy{})
		},
		Down: func(tx *gorm.DB) error {
			// Drop the column
			migrator := tx.Migrator()
			if migrator.HasColumn(&models.StreamProxy{}, "hls_collapse") {
				if err := migrator.DropColumn(&models.StreamProxy{}, "hls_collapse"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// migration023RelayProfileExtensions adds new columns to relay_profiles for
// FFmpeg profile configuration feature (hardware accel extensions, custom flags validation,
// and profile usage statistics).
func migration023RelayProfileExtensions() Migration {
	return Migration{
		Version:     "023",
		Description: "Add FFmpeg profile configuration extensions to relay_profiles",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate will add the new columns to the existing table:
			// - hw_accel_output_format
			// - hw_accel_decoder_codec
			// - hw_accel_extra_options
			// - gpu_index
			// - custom_flags_validated
			// - custom_flags_warnings
			// - success_count
			// - failure_count
			// - last_used_at
			// - last_error_at
			// - last_error_msg
			return tx.AutoMigrate(&models.RelayProfile{})
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()
			columns := []string{
				"hw_accel_output_format",
				"hw_accel_decoder_codec",
				"hw_accel_extra_options",
				"gpu_index",
				"custom_flags_validated",
				"custom_flags_warnings",
				"success_count",
				"failure_count",
				"last_used_at",
				"last_error_at",
				"last_error_msg",
			}
			for _, col := range columns {
				if migrator.HasColumn(&models.RelayProfile{}, col) {
					if err := migrator.DropColumn(&models.RelayProfile{}, col); err != nil {
						// Log but continue - SQLite doesn't support DROP COLUMN well
						tx.Logger.Warn(tx.Statement.Context, "failed to drop column %s: %v", col, err)
					}
				}
			}
			return nil
		},
	}
}

// migration024SystemProfileHWAccel updates system profiles with hardware acceleration
// enabled (auto mode) and fallback enabled for better out-of-box experience.
func migration024SystemProfileHWAccel() Migration {
	return Migration{
		Version:     "024",
		Description: "Enable hwaccel (auto) and fallback for system relay profiles",
		Up: func(tx *gorm.DB) error {
			// Update all system profiles to have hwaccel=auto and fallback_enabled=true
			// Use UpdateColumns to skip hooks since we're doing a partial update
			return tx.Model(&models.RelayProfile{}).
				Where("is_system = ?", true).
				UpdateColumns(map[string]interface{}{
					"hw_accel":         string(models.HWAccelAuto),
					"fallback_enabled": true,
				}).Error
		},
		Down: func(tx *gorm.DB) error {
			// Revert to no hwaccel and fallback disabled
			// Use UpdateColumns to skip hooks since we're doing a partial update
			return tx.Model(&models.RelayProfile{}).
				Where("is_system = ?", true).
				UpdateColumns(map[string]interface{}{
					"hw_accel":         "",
					"fallback_enabled": false,
				}).Error
		},
	}
}

// migration025LastKnownCodecs creates the last_known_codecs table for caching
// stream codec information discovered via ffprobe.
func migration025LastKnownCodecs() Migration {
	return Migration{
		Version:     "025",
		Description: "Create last_known_codecs table for codec caching",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.LastKnownCodec{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("last_known_codecs")
		},
	}
}

// migration026ForceTranscodeFlags adds force_video_transcode and force_audio_transcode
// columns to relay_profiles for smart codec matching (copy when source matches target).
func migration026ForceTranscodeFlags() Migration {
	return Migration{
		Version:     "026",
		Description: "Add force_video_transcode and force_audio_transcode to relay_profiles",
		Up: func(tx *gorm.DB) error {
			// AutoMigrate will add the new columns (default: false)
			return tx.AutoMigrate(&models.RelayProfile{})
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()
			columns := []string{"force_video_transcode", "force_audio_transcode"}
			for _, col := range columns {
				if migrator.HasColumn(&models.RelayProfile{}, col) {
					if err := migrator.DropColumn(&models.RelayProfile{}, col); err != nil {
						tx.Logger.Warn(tx.Statement.Context, "failed to drop column %s: %v", col, err)
					}
				}
			}
			return nil
		},
	}
}

// migration027MultiFormatStreaming renames segment_time to segment_duration and
// updates defaults for HLS/DASH multi-format streaming support.
func migration027MultiFormatStreaming() Migration {
	return Migration{
		Version:     "027",
		Description: "Rename segment_time to segment_duration for multi-format streaming",
		Up: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Rename segment_time to segment_duration if it exists
			if migrator.HasColumn(&models.RelayProfile{}, "segment_time") {
				if err := migrator.RenameColumn(&models.RelayProfile{}, "segment_time", "segment_duration"); err != nil {
					// Log but continue - column might already be renamed
					tx.Logger.Warn(tx.Statement.Context, "failed to rename segment_time: %v", err)
				}
			}

			// AutoMigrate will ensure segment_duration column exists with correct defaults
			if err := tx.AutoMigrate(&models.RelayProfile{}); err != nil {
				return err
			}

			// Update existing profiles with 0 segment_duration to use default of 6
			if err := tx.Model(&models.RelayProfile{}).
				Where("segment_duration = ? OR segment_duration IS NULL", 0).
				UpdateColumn("segment_duration", 6).Error; err != nil {
				return err
			}

			// Update existing profiles with 0 playlist_size to use default of 5
			if err := tx.Model(&models.RelayProfile{}).
				Where("playlist_size = ? OR playlist_size IS NULL", 0).
				UpdateColumn("playlist_size", 5).Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Rename segment_duration back to segment_time
			if migrator.HasColumn(&models.RelayProfile{}, "segment_duration") {
				if err := migrator.RenameColumn(&models.RelayProfile{}, "segment_duration", "segment_time"); err != nil {
					tx.Logger.Warn(tx.Statement.Context, "failed to rename segment_duration: %v", err)
				}
			}

			return nil
		},
	}
}

