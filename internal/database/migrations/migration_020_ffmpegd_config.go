package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration020FFmpegdConfig creates the ffmpegd_config table for storing
// coordinator configuration settings for the distributed transcoding system.
func migration020FFmpegdConfig() Migration {
	return Migration{
		Version:     "020",
		Description: "Add ffmpegd_config table for distributed transcoding configuration",
		Up: func(tx *gorm.DB) error {
			// Create the ffmpegd_config table
			if err := tx.AutoMigrate(&models.FFmpegdConfig{}); err != nil {
				return err
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Drop the ffmpegd_config table
			return tx.Migrator().DropTable("ffmpegd_config")
		},
	}
}
