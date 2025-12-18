package migrations

import (
	"gorm.io/gorm"
)

// migration017RemoveFilterIsEnabled removes the is_enabled column from filters table.
// Filters should only be active/inactive through the proxy-filter relationship's is_active field.
// This allows the same filter to be enabled on one proxy and disabled on another.
func migration017RemoveFilterIsEnabled() Migration {
	return Migration{
		Version:     "017",
		Description: "Remove is_enabled column from filters table",
		Up: func(tx *gorm.DB) error {
			// Drop the is_enabled column from filters table
			// SQLite 3.35.0+ (2021) supports DROP COLUMN via raw SQL
			// On older versions, this will fail gracefully and the column remains but is unused
			if tx.Migrator().HasColumn("filters", "is_enabled") {
				_ = tx.Exec("ALTER TABLE filters DROP COLUMN is_enabled").Error
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Re-add the is_enabled column with default true using raw SQL
			if !tx.Migrator().HasColumn("filters", "is_enabled") {
				if err := tx.Exec("ALTER TABLE filters ADD COLUMN is_enabled BOOLEAN DEFAULT 1").Error; err != nil {
					return err
				}
				// Set all existing filters to enabled
				if err := tx.Exec("UPDATE filters SET is_enabled = 1").Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
