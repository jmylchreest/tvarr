package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration018BackupSettings creates the backup_settings table for storing
// user-configurable backup schedule settings.
func migration018BackupSettings() Migration {
	return Migration{
		Version:     "018",
		Description: "Add backup_settings table for user-configurable backup schedule",
		Up: func(tx *gorm.DB) error {
			// Create the backup_settings table
			if err := tx.AutoMigrate(&models.BackupSettings{}); err != nil {
				return err
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Drop the backup_settings table
			return tx.Migrator().DropTable("backup_settings")
		},
	}
}
