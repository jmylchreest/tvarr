package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration022EncoderOverrides adds the encoder_overrides table and seeds it with
// default rules to work around known hardware encoder issues.
//
// This migration adds the initial override for:
// - AMD HEVC VAAPI: hevc_vaapi is broken with Mesa 21.1+ on AMD GPUs
func migration022EncoderOverrides() Migration {
	return Migration{
		Version:     "022",
		Description: "Add encoder_overrides table for hardware encoder workarounds",
		Up: func(tx *gorm.DB) error {
			// Create the encoder_overrides table
			if err := tx.AutoMigrate(&models.EncoderOverride{}); err != nil {
				return err
			}
			// Seed default encoder override rules
			return createDefaultEncoderOverrides(tx)
		},
		Down: func(tx *gorm.DB) error {
			// Delete the system encoder overrides by name
			names := []string{
				"AMD HEVC VAAPI Workaround",
			}
			for _, name := range names {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.EncoderOverride{}).Error; err != nil {
					return err
				}
			}
			// Drop the table
			return tx.Migrator().DropTable("encoder_overrides")
		},
	}
}

// createDefaultEncoderOverrides creates system encoder override rules for known hardware issues.
func createDefaultEncoderOverrides(tx *gorm.DB) error {
	overrides := []models.EncoderOverride{
		{
			Name:          "AMD HEVC VAAPI Workaround",
			Description:   "hevc_vaapi is broken with Mesa 21.1+ on AMD GPUs - encode at 0.02x speed. Forces software encoder (libx265) which runs at ~5x speed.",
			CodecType:     models.EncoderOverrideCodecTypeVideo,
			SourceCodec:   "h265",
			TargetEncoder: "libx265",
			HWAccelMatch:  "vaapi",
			CPUMatch:      "",
			Priority:      100,
			IsEnabled:     models.BoolPtr(false),
			IsSystem:      true,
		},
	}

	for i := range overrides {
		overrides[i].ID = models.NewULID()
		if err := tx.Create(&overrides[i]).Error; err != nil {
			return err
		}
	}

	return nil
}
