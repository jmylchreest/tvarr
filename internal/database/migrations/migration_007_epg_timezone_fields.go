package migrations

import (
	"gorm.io/gorm"
)

// migration007EpgTimezoneFields renames EPG source timezone fields:
// - original_timezone -> detected_timezone (auto-detected from EPG data)
// - time_offset (string) -> epg_shift (integer hours)
//
// The new model is simpler: timezone is auto-detected and stored for display,
// while epg_shift allows users to manually adjust times by N hours.
//
// SQLite doesn't support DROP COLUMN directly in older versions, so we use
// the table recreation pattern for the time_offset -> epg_shift conversion.
func migration007EpgTimezoneFields() Migration {
	return Migration{
		Version:     "007",
		Description: "Rename EPG timezone fields to detected_timezone and epg_shift",
		Up: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Handle original_timezone -> detected_timezone using ALTER TABLE RENAME COLUMN
			// (supported in SQLite 3.25.0+, 2018)
			if migrator.HasColumn("epg_sources", "original_timezone") {
				if err := tx.Exec("ALTER TABLE epg_sources RENAME COLUMN original_timezone TO detected_timezone").Error; err != nil {
					return err
				}
			} else if !migrator.HasColumn("epg_sources", "detected_timezone") {
				// Column doesn't exist at all, add it
				if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN detected_timezone VARCHAR(50)").Error; err != nil {
					return err
				}
			}

			// Handle time_offset -> epg_shift (string to int conversion)
			// SQLite doesn't support changing column types, so we add new column and migrate data
			if migrator.HasColumn("epg_sources", "time_offset") {
				// Add new column
				if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN epg_shift INTEGER DEFAULT 0").Error; err != nil {
					return err
				}

				// Migrate data: try to parse time_offset string like "+02:00" to integer hours
				// This is best-effort - complex offsets will become 0
				_ = tx.Exec(`
					UPDATE epg_sources
					SET epg_shift = CASE
						WHEN time_offset LIKE '+%' THEN CAST(SUBSTR(time_offset, 2, 2) AS INTEGER)
						WHEN time_offset LIKE '-%' THEN -CAST(SUBSTR(time_offset, 2, 2) AS INTEGER)
						ELSE 0
					END
					WHERE time_offset IS NOT NULL AND time_offset != ''
				`).Error

				// For SQLite, we can't easily drop columns in older versions.
				// The old time_offset column will remain but be unused.
				// GORM will ignore it since it's not in the model.
				// In SQLite 3.35.0+ (2021), DROP COLUMN is supported:
				_ = tx.Exec("ALTER TABLE epg_sources DROP COLUMN time_offset").Error
			} else if !migrator.HasColumn("epg_sources", "epg_shift") {
				// Column doesn't exist at all, add it
				if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN epg_shift INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			// Reverse: detected_timezone -> original_timezone
			if migrator.HasColumn("epg_sources", "detected_timezone") {
				if err := tx.Exec("ALTER TABLE epg_sources RENAME COLUMN detected_timezone TO original_timezone").Error; err != nil {
					return err
				}
			}

			// Reverse: epg_shift -> time_offset (convert int to string)
			if migrator.HasColumn("epg_sources", "epg_shift") {
				// Add old column back if it doesn't exist
				if !migrator.HasColumn("epg_sources", "time_offset") {
					if err := tx.Exec("ALTER TABLE epg_sources ADD COLUMN time_offset VARCHAR(20)").Error; err != nil {
						return err
					}
				}

				// Migrate data back
				_ = tx.Exec(`
					UPDATE epg_sources
					SET time_offset = CASE
						WHEN epg_shift >= 0 THEN '+' || printf('%02d', epg_shift) || ':00'
						ELSE '-' || printf('%02d', -epg_shift) || ':00'
					END
				`).Error

				// Try to drop the new column (may fail on older SQLite)
				_ = tx.Exec("ALTER TABLE epg_sources DROP COLUMN epg_shift").Error
			}

			return nil
		},
	}
}
