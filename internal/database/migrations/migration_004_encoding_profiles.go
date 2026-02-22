package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration004EncodingProfiles adds the EncodingProfile model.
// This migration:
// 1. Creates the encoding_profiles table
// 2. Seeds system encoding profiles
// 3. Updates stream_proxies to use encoding_profile_id
// 4. Adds timezone fields to epg_sources
func migration004EncodingProfiles() Migration {
	return Migration{
		Version:     "004",
		Description: "Add EncodingProfile model",
		Up: func(tx *gorm.DB) error {
			// Step 1: Create encoding_profiles table
			if err := tx.AutoMigrate(&models.EncodingProfile{}); err != nil {
				return err
			}

			// Step 2: Seed system encoding profiles
			if err := createSystemEncodingProfiles(tx); err != nil {
				return err
			}

			// Step 3: Add encoding_profile_id to stream_proxies
			if err := migrateStreamProxyProfileReferences(tx); err != nil {
				return err
			}

			// Step 4: Add timezone fields to epg_sources
			if err := addTimezoneFieldsToEpgSources(tx); err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Remove encoding_profile_id from stream_proxies
			if tx.Migrator().HasColumn(&models.StreamProxy{}, "encoding_profile_id") {
				if err := tx.Migrator().DropColumn(&models.StreamProxy{}, "encoding_profile_id"); err != nil {
					return err
				}
			}

			// Remove timezone fields from epg_sources
			if tx.Migrator().HasColumn(&models.EpgSource{}, "source_timezone") {
				if err := tx.Migrator().DropColumn(&models.EpgSource{}, "source_timezone"); err != nil {
					return err
				}
			}
			if tx.Migrator().HasColumn(&models.EpgSource{}, "timezone_offset_minutes") {
				if err := tx.Migrator().DropColumn(&models.EpgSource{}, "timezone_offset_minutes"); err != nil {
					return err
				}
			}

			// Drop encoding_profiles table
			if tx.Migrator().HasTable("encoding_profiles") {
				if err := tx.Migrator().DropTable("encoding_profiles"); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

// createSystemEncodingProfiles creates the default system encoding profiles.
func createSystemEncodingProfiles(tx *gorm.DB) error {
	profiles := []models.EncodingProfile{
		{
			Name:             "H.264/AAC (Universal)",
			Description:      "Maximum device compatibility - works with all players and browsers",
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
			HWAccel:          models.HWAccelAuto,
			IsDefault:        true,
			IsSystem:         true,
			Enabled:          new(true),
		},
		{
			Name:             "H.265/AAC (Modern)",
			Description:      "Better compression for modern devices - 40-50% smaller than H.264",
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
			HWAccel:          models.HWAccelAuto,
			IsDefault:        false,
			IsSystem:         true,
			Enabled:          new(true),
		},
		{
			Name:             "VP9/Opus (Web)",
			Description:      "Open/royalty-free codecs - optimized for web browsers",
			TargetVideoCodec: models.VideoCodecVP9,
			TargetAudioCodec: models.AudioCodecOpus,
			QualityPreset:    models.QualityPresetMedium,
			HWAccel:          models.HWAccelAuto,
			IsDefault:        false,
			IsSystem:         true,
			Enabled:          new(true),
		},
		{
			Name:             "AV1/Opus (Next-Gen)",
			Description:      "Best compression - 30% smaller than H.265, requires modern hardware",
			TargetVideoCodec: models.VideoCodecAV1,
			TargetAudioCodec: models.AudioCodecOpus,
			QualityPreset:    models.QualityPresetMedium,
			HWAccel:          models.HWAccelAuto,
			IsDefault:        false,
			IsSystem:         true,
			Enabled:          new(true),
		},
		{
			Name:             "H.264 Low Bandwidth",
			Description:      "Optimized for slow connections - mobile/cellular streaming",
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetLow,
			HWAccel:          models.HWAccelAuto,
			IsDefault:        false,
			IsSystem:         true,
			Enabled:          new(true),
		},
		{
			Name:             "H.265 High Quality",
			Description:      "High quality H.265 encoding for home theater setups",
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetHigh,
			HWAccel:          models.HWAccelAuto,
			IsDefault:        false,
			IsSystem:         true,
			Enabled:          new(true),
		},
	}

	for i := range profiles {
		profiles[i].ID = models.NewULID()
		if err := tx.Create(&profiles[i]).Error; err != nil {
			return err
		}
	}

	return nil
}

// migrateStreamProxyProfileReferences adds encoding_profile_id to stream_proxies.
func migrateStreamProxyProfileReferences(tx *gorm.DB) error {
	// Add encoding_profile_id column if it doesn't exist
	if !tx.Migrator().HasColumn(&models.StreamProxy{}, "encoding_profile_id") {
		if err := tx.Exec("ALTER TABLE stream_proxies ADD COLUMN encoding_profile_id VARCHAR(26)").Error; err != nil {
			return err
		}
	}

	return nil
}

// addTimezoneFieldsToEpgSources adds timezone detection fields to the epg_sources table.
func addTimezoneFieldsToEpgSources(tx *gorm.DB) error {
	// Add source_timezone column
	if !tx.Migrator().HasColumn(&models.EpgSource{}, "source_timezone") {
		if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN source_timezone VARCHAR(50)").Error; err != nil {
			return err
		}
	}

	// Add timezone_offset_minutes column
	if !tx.Migrator().HasColumn(&models.EpgSource{}, "timezone_offset_minutes") {
		if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN timezone_offset_minutes INTEGER DEFAULT 0").Error; err != nil {
			return err
		}
	}

	return nil
}
