// Package migrations provides database migration management for tvarr.
package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// AllMigrations returns all registered migrations in order.
// This is a compacted migration set for new installations.
// Previous 34 migrations have been consolidated into 2 migrations:
// - 001: Schema creation using GORM AutoMigrate
// - 002: System data (default filters, rules, profiles, mappings)
func AllMigrations() []Migration {
	return []Migration{
		migration001Schema(),
		migration002SystemData(),
		migration003CleanupRelayProfiles(),
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

				// Relay system
				&models.RelayProfile{},
				&models.RelayProfileMapping{},

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
				"relay_profile_mappings",
				"relay_profiles",
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
		Description: "Insert default filters, rules, profiles, and mappings",
		Up: func(tx *gorm.DB) error {
			// Create default filters
			if err := createDefaultFilters(tx); err != nil {
				return err
			}

			// Create default data mapping rules
			if err := createDefaultDataMappingRules(tx); err != nil {
				return err
			}

			// Create default relay profiles
			if err := createDefaultRelayProfiles(tx); err != nil {
				return err
			}

			// Create default relay profile mappings (client detection rules)
			if err := createDefaultRelayProfileMappings(tx); err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Delete system data in reverse order
			if err := tx.Where("is_system = ?", true).Delete(&models.RelayProfileMapping{}).Error; err != nil {
				return err
			}
			if err := tx.Where("is_system = ?", true).Delete(&models.RelayProfile{}).Error; err != nil {
				return err
			}
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

// migration003CleanupRelayProfiles purges soft-deleted relay profiles and fixes encoder names.
func migration003CleanupRelayProfiles() Migration {
	return Migration{
		Version:     "003",
		Description: "Purge soft-deleted relay profiles and fix encoder names to codec names",
		Up: func(tx *gorm.DB) error {
			// 1. Hard-delete all soft-deleted relay profiles
			if err := tx.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.RelayProfile{}).Error; err != nil {
				return err
			}

			// 2. Hard-delete all soft-deleted relay profile mappings
			if err := tx.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.RelayProfileMapping{}).Error; err != nil {
				return err
			}

			// 3. Fix encoder names in video_codec column -> codec names
			// Use UpdateColumn to bypass model hooks/validation
			// H.264 encoders -> h264
			if err := tx.Model(&models.RelayProfile{}).
				Where("video_codec IN ?", []string{"libx264", "h264_nvenc", "h264_qsv", "h264_vaapi", "h264_videotoolbox", "h264_amf", "h264_v4l2m2m"}).
				UpdateColumn("video_codec", "h264").Error; err != nil {
				return err
			}

			// H.265/HEVC encoders -> h265
			if err := tx.Model(&models.RelayProfile{}).
				Where("video_codec IN ?", []string{"libx265", "hevc_nvenc", "hevc_qsv", "hevc_vaapi", "hevc_videotoolbox", "hevc_amf", "hevc_v4l2m2m"}).
				UpdateColumn("video_codec", "h265").Error; err != nil {
				return err
			}

			// VP9 encoders -> vp9
			if err := tx.Model(&models.RelayProfile{}).
				Where("video_codec IN ?", []string{"libvpx-vp9", "vp9_qsv", "vp9_vaapi"}).
				UpdateColumn("video_codec", "vp9").Error; err != nil {
				return err
			}

			// AV1 encoders -> av1
			if err := tx.Model(&models.RelayProfile{}).
				Where("video_codec IN ?", []string{"libaom-av1", "av1_nvenc", "av1_qsv", "av1_vaapi"}).
				UpdateColumn("video_codec", "av1").Error; err != nil {
				return err
			}

			// 4. Fix encoder names in audio_codec column -> codec names
			// AAC encoders -> aac
			if err := tx.Model(&models.RelayProfile{}).
				Where("audio_codec IN ?", []string{"libfdk_aac", "aac_at"}).
				UpdateColumn("audio_codec", "aac").Error; err != nil {
				return err
			}

			// Opus encoders -> opus
			if err := tx.Model(&models.RelayProfile{}).
				Where("audio_codec = ?", "libopus").
				UpdateColumn("audio_codec", "opus").Error; err != nil {
				return err
			}

			// MP3 encoders -> mp3
			if err := tx.Model(&models.RelayProfile{}).
				Where("audio_codec IN ?", []string{"libmp3lame", "libshine"}).
				UpdateColumn("audio_codec", "mp3").Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// This migration cannot be reversed (data cleanup is permanent)
			// The encoder->codec name normalization is the correct format anyway
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

	for _, rule := range rules {
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

// createDefaultRelayProfiles creates the default system relay profiles.
func createDefaultRelayProfiles(tx *gorm.DB) error {
	profiles := []models.RelayProfile{
		{
			Name:            "Automatic",
			Description:     "Automatically selects optimal codecs based on client detection rules. Configure rules in Admin > Relay Profile Mappings.",
			IsDefault:       true,
			Enabled:         true,
			IsSystem:        true,
			VideoCodec:      models.VideoCodecAuto,
			AudioCodec:      models.AudioCodecAuto,
			ContainerFormat: models.ContainerFormatAuto,
			HWAccel:         models.HWAccelAuto,
			FallbackEnabled: true,
			Timeout:         30,
		},
		{
			Name:            "Passthrough",
			Description:     "Pass-through profile that copies streams without transcoding",
			IsDefault:       false,
			Enabled:         true,
			IsSystem:        true,
			VideoCodec:      models.VideoCodecCopy,
			AudioCodec:      models.AudioCodecCopy,
			ContainerFormat: models.ContainerFormatMPEGTS,
			Timeout:         30,
		},
		{
			Name:            "h264/AAC",
			Description:     "H.264 video with AAC audio - maximum device compatibility",
			IsDefault:       false,
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
			ContainerFormat: models.ContainerFormatMPEGTS,
			HWAccel:         models.HWAccelAuto,
			FallbackEnabled: true,
			Timeout:         30,
		},
		{
			Name:            "h265/AAC",
			Description:     "H.265/HEVC video with AAC audio - better compression, modern devices",
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
			ContainerFormat: models.ContainerFormatMPEGTS,
			HWAccel:         models.HWAccelAuto,
			FallbackEnabled: true,
			Timeout:         30,
		},
		{
			Name:            "VP9/Opus",
			Description:     "VP9 video with Opus audio - open/royalty-free codecs, good for web",
			IsDefault:       false,
			Enabled:         true,
			IsSystem:        true,
			VideoCodec:      models.VideoCodecVP9,
			AudioCodec:      models.AudioCodecOpus,
			ContainerFormat: models.ContainerFormatFMP4,
			HWAccel:         models.HWAccelAuto,
			FallbackEnabled: true,
			Timeout:         30,
		},
		{
			Name:            "AV1/Opus",
			Description:     "AV1 video with Opus audio - best compression, next-gen codecs",
			IsDefault:       false,
			Enabled:         true,
			IsSystem:        true,
			VideoCodec:      models.VideoCodecAV1,
			AudioCodec:      models.AudioCodecOpus,
			ContainerFormat: models.ContainerFormatFMP4,
			HWAccel:         models.HWAccelAuto,
			FallbackEnabled: true,
			Timeout:         30,
		},
	}

	for _, profile := range profiles {
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}
	}
	return nil
}

// createDefaultRelayProfileMappings creates the default client detection rules.
func createDefaultRelayProfileMappings(tx *gorm.DB) error {
	// Helper functions for codec arrays
	videoCodecs := func(codecs ...string) models.PqStringArray {
		return codecs
	}
	audioCodecs := func(codecs ...string) models.PqStringArray {
		return codecs
	}
	containers := func(formats ...string) models.PqStringArray {
		return formats
	}

	mappings := []models.RelayProfileMapping{
		// Browsers (10-19)
		{
			Name:                "Safari",
			Description:         "Apple Safari browser - No VP9/Opus support",
			Priority:            10,
			Expression:          `user_agent contains "Safari" AND user_agent not_contains "Chrome"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Chrome/Chromium",
			Description:         "Google Chrome/Chromium browser - Full modern codec support",
			Priority:            11,
			Expression:          `user_agent matches ".*(Chrome|Chromium).*" AND user_agent not_contains "Edg"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Edge",
			Description:         "Microsoft Edge browser - Chromium-based",
			Priority:            12,
			Expression:          `user_agent contains "Edg/"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Firefox",
			Description:         "Mozilla Firefox - No H.265 without system codec",
			Priority:            13,
			Expression:          `user_agent contains "Firefox"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		// Media Players (20-29)
		{
			Name:                "VLC",
			Description:         "VLC Media Player - Excellent codec support",
			Priority:            20,
			Expression:          `user_agent contains "VLC"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "MPV",
			Description:         "MPV Media Player - Excellent codec support",
			Priority:            21,
			Expression:          `user_agent contains "mpv"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "ffmpeg/ffplay",
			Description:         "FFmpeg/FFplay - Full codec support",
			Priority:            22,
			Expression:          `user_agent matches ".*(ffmpeg|ffplay|Lavf).*"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		// Media Servers/Apps (30-39)
		{
			Name:                "Jellyfin",
			Description:         "Jellyfin Media Server - Server transcodes further if needed",
			Priority:            30,
			Expression:          `user_agent contains "Jellyfin"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "Plex",
			Description:         "Plex Media Server - Server handles client compatibility",
			Priority:            31,
			Expression:          `user_agent matches ".*(Plex|PMS).*"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "Emby",
			Description:         "Emby Media Server - Similar to Jellyfin",
			Priority:            32,
			Expression:          `user_agent contains "Emby"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "Kodi",
			Description:         "Kodi Media Center - Codec support depends on device",
			Priority:            33,
			Expression:          `user_agent contains "Kodi"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		// Streaming Devices (40-49)
		{
			Name:                "Android TV / Google TV",
			Description:         "Android TV / Google TV / Chromecast - Modern codec support",
			Priority:            40,
			Expression:          `user_agent matches ".*(Android TV|GoogleTV|Chromecast).*"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Roku",
			Description:         "Roku devices - Limited VP9/AV1 support",
			Priority:            41,
			Expression:          `user_agent contains "Roku"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "Apple TV",
			Description:         "Apple TV - No VP9/Opus support",
			Priority:            42,
			Expression:          `user_agent contains "AppleTV"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Fire TV",
			Description:         "Amazon Fire TV - Good modern codec support",
			Priority:            43,
			Expression:          `user_agent matches ".*(Fire TV|AFTM|AFT).*"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Tizen (Samsung TV)",
			Description:         "Samsung Smart TVs - Limited AV1/VP9 support",
			Priority:            44,
			Expression:          `user_agent contains "Tizen"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "webOS (LG TV)",
			Description:         "LG Smart TVs - Limited AV1/VP9 support",
			Priority:            45,
			Expression:          `user_agent contains "Web0S"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3", "eac3"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		// Mobile (50-59)
		{
			Name:                "iOS",
			Description:         "iOS devices (native Safari) - No VP9/Opus",
			Priority:            50,
			Expression:          `user_agent matches ".*(iPhone|iPad).*" AND user_agent not_contains "CriOS"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		{
			Name:                "Android",
			Description:         "Android devices - Codec support varies by device",
			Priority:            51,
			Expression:          `user_agent contains "Android" AND user_agent not_contains "Android TV"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264", "vp9", "av1"),
			AcceptedAudioCodecs: audioCodecs("aac", "opus"),
			AcceptedContainers:  containers("fmp4", "mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatFMP4,
		},
		// IPTV Apps (60-69)
		{
			Name:                "TiviMate",
			Description:         "TiviMate IPTV Player - Popular Android IPTV app",
			Priority:            60,
			Expression:          `user_agent contains "TiviMate"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3"),
			AcceptedContainers:  containers("mpegts"),
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "IPTV Smarters",
			Description:         "IPTV Smarters - Conservative codec support",
			Priority:            61,
			Expression:          `user_agent matches ".*(Smarters|IPTV).*"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac", "ac3"),
			AcceptedContainers:  containers("mpegts"),
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		{
			Name:                "GSE Smart IPTV",
			Description:         "GSE Smart IPTV - iOS/Android IPTV app",
			Priority:            62,
			Expression:          `user_agent contains "GSE"`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h265", "h264"),
			AcceptedAudioCodecs: audioCodecs("aac"),
			AcceptedContainers:  containers("mpegts"),
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
		// Fallback (999)
		{
			Name:                "Default (Universal)",
			Description:         "Fallback rule - Maximum compatibility (H.264/AAC/MPEG-TS)",
			Priority:            999,
			Expression:          `true`,
			IsEnabled:           true,
			IsSystem:            true,
			AcceptedVideoCodecs: videoCodecs("h264"),
			AcceptedAudioCodecs: audioCodecs("aac"),
			AcceptedContainers:  containers("mpegts"),
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			PreferredContainer:  models.ContainerFormatMPEGTS,
		},
	}

	// Insert default mappings with skip hooks
	for i := range mappings {
		mappings[i].ID = models.NewULID()
		if err := tx.Session(&gorm.Session{SkipHooks: true}).Create(&mappings[i]).Error; err != nil {
			return err
		}
	}

	return nil
}
