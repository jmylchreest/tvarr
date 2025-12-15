package migrations

import (
	"gorm.io/gorm"
)

// migration012EpgAutoShiftTimezone adds the auto_shift_timezone column to epg_sources table.
// This tracks which timezone the epg_shift was auto-configured for, allowing user overrides
// to persist until the detected timezone actually changes.
func migration012EpgAutoShiftTimezone() Migration {
	return Migration{
		Version:     "012",
		Description: "Add auto_shift_timezone column to epg_sources",
		Up: func(tx *gorm.DB) error {
			// Add auto_shift_timezone column with empty default
			// This tracks the timezone we last auto-configured epg_shift for
			if !tx.Migrator().HasColumn("epg_sources", "auto_shift_timezone") {
				if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN auto_shift_timezone TEXT DEFAULT ''").Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// SQLite doesn't support DROP COLUMN directly in older versions
			// For newer SQLite (3.35+), we can use DROP COLUMN
			if tx.Migrator().HasColumn("epg_sources", "auto_shift_timezone") {
				if err := tx.Exec("ALTER TABLE epg_sources DROP COLUMN auto_shift_timezone").Error; err != nil {
					// Fallback: For older SQLite, this is a no-op on rollback
					// The column will just be ignored if it exists
					return nil
				}
			}
			return nil
		},
	}
}
