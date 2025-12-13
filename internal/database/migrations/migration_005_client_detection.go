package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration005ClientDetection adds the ClientDetectionRule model and related fields.
// This migration:
// 1. Creates the client_detection_rules table
// 2. Seeds system client detection rules
// 3. Adds client_detection_enabled to stream_proxies
func migration005ClientDetection() Migration {
	return Migration{
		Version:     "005",
		Description: "Add ClientDetectionRule model",
		Up: func(tx *gorm.DB) error {
			// Step 1: Create client_detection_rules table
			if err := tx.AutoMigrate(&models.ClientDetectionRule{}); err != nil {
				return err
			}

			// Step 2: Seed system client detection rules
			if err := createSystemClientDetectionRules(tx); err != nil {
				return err
			}

			// Step 3: Add client_detection_enabled to stream_proxies
			if !tx.Migrator().HasColumn(&models.StreamProxy{}, "client_detection_enabled") {
				if err := tx.Exec("ALTER TABLE stream_proxies ADD COLUMN client_detection_enabled INTEGER DEFAULT 1").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Remove client_detection_enabled from stream_proxies
			if tx.Migrator().HasColumn(&models.StreamProxy{}, "client_detection_enabled") {
				if err := tx.Migrator().DropColumn(&models.StreamProxy{}, "client_detection_enabled"); err != nil {
					return err
				}
			}

			// Drop client_detection_rules table
			if tx.Migrator().HasTable("client_detection_rules") {
				if err := tx.Migrator().DropTable("client_detection_rules"); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

// createSystemClientDetectionRules creates the default system client detection rules.
func createSystemClientDetectionRules(tx *gorm.DB) error {
	rules := []models.ClientDetectionRule{
		{
			Name:                "Android TV",
			Description:         "Android TV devices including Google TV, NVIDIA Shield, etc.",
			Expression:          `@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"`,
			Priority:            100,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3","eac3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Samsung Smart TV",
			Description:         "Samsung Tizen-based Smart TVs",
			Expression:          `@dynamic(request.headers):user-agent contains "Tizen" OR @dynamic(request.headers):user-agent contains "SMART-TV"`,
			Priority:            110,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "LG WebOS",
			Description:         "LG WebOS-based Smart TVs",
			Expression:          `@dynamic(request.headers):user-agent contains "webOS"`,
			Priority:            120,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3","eac3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Roku",
			Description:         "Roku streaming devices",
			Expression:          `@dynamic(request.headers):user-agent contains "Roku"`,
			Priority:            130,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265","vp9"]`,
			AcceptedAudioCodecs: `["aac","ac3","eac3","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Apple TV",
			Description:         "Apple TV devices",
			Expression:          `@dynamic(request.headers):user-agent contains "AppleTV" OR @dynamic(request.headers):user-agent contains "tvOS"`,
			Priority:            140,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","ac3","eac3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(false),
			PreferredFormat:     "hls-fmp4",
		},
		{
			Name:                "iOS Safari",
			Description:         "iPhone and iPad Safari browser",
			Expression:          `@dynamic(request.headers):user-agent contains "iPhone" OR @dynamic(request.headers):user-agent contains "iPad"`,
			Priority:            150,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(false),
			PreferredFormat:     "hls-fmp4",
		},
		{
			Name:                "Chrome Browser",
			Description:         "Google Chrome browser (excluding Edge)",
			Expression:          `@dynamic(request.headers):user-agent contains "Chrome" AND NOT @dynamic(request.headers):user-agent contains "Edge"`,
			Priority:            160,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","vp8","vp9","av1"]`,
			AcceptedAudioCodecs: `["aac","mp3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Firefox Browser",
			Description:         "Mozilla Firefox browser",
			Expression:          `@dynamic(request.headers):user-agent contains "Firefox"`,
			Priority:            170,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","vp8","vp9","av1"]`,
			AcceptedAudioCodecs: `["aac","mp3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Safari Desktop",
			Description:         "Safari browser on macOS",
			Expression:          `@dynamic(request.headers):user-agent contains "Safari" AND @dynamic(request.headers):user-agent contains "Macintosh"`,
			Priority:            180,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(false),
			PreferredFormat:     "hls-fmp4",
		},
		{
			Name:                "Edge Browser",
			Description:         "Microsoft Edge browser",
			Expression:          `@dynamic(request.headers):user-agent contains "Edge"`,
			Priority:            190,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","vp9","av1"]`,
			AcceptedAudioCodecs: `["aac","mp3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Android Mobile",
			Description:         "Android phones and tablets (non-TV)",
			Expression:          `@dynamic(request.headers):user-agent contains "Android" AND NOT @dynamic(request.headers):user-agent contains "TV"`,
			Priority:            200,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Generic Smart TV",
			Description:         "Fallback for unidentified Smart TVs",
			Expression:          `@dynamic(request.headers):user-agent contains "SmartTV" OR @dynamic(request.headers):user-agent contains "smart-tv"`,
			Priority:            900,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264"]`,
			AcceptedAudioCodecs: `["aac","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
	}

	for i := range rules {
		rules[i].ID = models.NewULID()
		if err := tx.Create(&rules[i]).Error; err != nil {
			return err
		}
	}

	return nil
}
