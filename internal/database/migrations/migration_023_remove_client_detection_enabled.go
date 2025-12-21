package migrations

import (
	"gorm.io/gorm"
)

// migration023RemoveClientDetectionEnabled removes the deprecated client_detection_enabled column
// from stream_proxies table. Client detection is now always enabled, and the column is unused.
func migration023RemoveClientDetectionEnabled() Migration {
	return Migration{
		Version:     "023",
		Description: "Remove deprecated client_detection_enabled column from stream_proxies",
		Up: func(tx *gorm.DB) error {
			// Check if the column exists before attempting to drop it
			if tx.Migrator().HasColumn("stream_proxies", "client_detection_enabled") {
				if err := tx.Exec("ALTER TABLE stream_proxies DROP COLUMN client_detection_enabled").Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Re-add the column if rolling back (default to enabled)
			if !tx.Migrator().HasColumn("stream_proxies", "client_detection_enabled") {
				if err := tx.Exec("ALTER TABLE stream_proxies ADD COLUMN client_detection_enabled INTEGER DEFAULT 1").Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
