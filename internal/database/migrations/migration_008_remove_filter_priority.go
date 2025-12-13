package migrations

import (
	"gorm.io/gorm"
)

// migration008RemoveFilterPriority removes the redundant priority column from the filters table.
// Priority is only relevant in the proxy_filters join table, not on filters themselves.
// The filters table had a priority field that was never actually used for ordering -
// the pipeline loads all enabled filters globally via FilterRepo.GetEnabled().
func migration008RemoveFilterPriority() Migration {
	return Migration{
		Version:     "008",
		Description: "Remove redundant priority column from filters table",
		Up: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Drop the priority column from filters table if it exists
			// SQLite 3.35.0+ (2021) supports DROP COLUMN
			if migrator.HasColumn("filters", "priority") {
				// Try to drop the column - may fail on older SQLite versions
				// In that case, the column will remain but be unused (GORM ignores it)
				_ = tx.Exec("ALTER TABLE filters DROP COLUMN priority").Error
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Re-add the priority column if it doesn't exist
			if !migrator.HasColumn("filters", "priority") {
				if err := tx.Exec("ALTER TABLE filters ADD COLUMN priority INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
