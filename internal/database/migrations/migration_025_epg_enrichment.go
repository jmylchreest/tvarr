package migrations

import (
	"gorm.io/gorm"
)

// migration025EpgEnrichment adds enrichment fields to epg_programs table
// for improved Jellyfin/Plex compatibility:
// - previously_shown: BOOLEAN for repeat/rerun detection
// - date: VARCHAR(20) for production year or air date (e.g., "2025", "20250315")
// - star_rating: VARCHAR(20) for community/critic rating (e.g., "8/10", "4/5")
// - season_number: INTEGER for parsed season number (1-based, 0 means unknown)
// - episode_number: INTEGER for parsed episode number (1-based, 0 means unknown)
// - program_id: VARCHAR(100) for unique program identifier (e.g., dd_progid) for recording dedup
// - credits: TEXT for cast/crew stored as JSON
func migration025EpgEnrichment() Migration {
	return Migration{
		Version:     "025",
		Description: "Add EPG enrichment fields to epg_programs table",
		Up: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			if !migrator.HasColumn("epg_programs", "previously_shown") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN previously_shown BOOLEAN DEFAULT FALSE").Error; err != nil {
					return err
				}
			}

			if !migrator.HasColumn("epg_programs", "date") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN date VARCHAR(20)").Error; err != nil {
					return err
				}
			}

			if !migrator.HasColumn("epg_programs", "star_rating") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN star_rating VARCHAR(20)").Error; err != nil {
					return err
				}
			}

			if !migrator.HasColumn("epg_programs", "season_number") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN season_number INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}

			if !migrator.HasColumn("epg_programs", "episode_number") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN episode_number INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}

			if !migrator.HasColumn("epg_programs", "program_id") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN program_id VARCHAR(100)").Error; err != nil {
					return err
				}
			}

			if !migrator.HasColumn("epg_programs", "credits") {
				if err := tx.Exec("ALTER TABLE epg_programs ADD COLUMN credits TEXT").Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			migrator := tx.Migrator()

			if migrator.HasColumn("epg_programs", "previously_shown") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN previously_shown").Error
			}

			if migrator.HasColumn("epg_programs", "date") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN date").Error
			}

			if migrator.HasColumn("epg_programs", "star_rating") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN star_rating").Error
			}

			if migrator.HasColumn("epg_programs", "season_number") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN season_number").Error
			}

			if migrator.HasColumn("epg_programs", "episode_number") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN episode_number").Error
			}

			if migrator.HasColumn("epg_programs", "program_id") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN program_id").Error
			}

			if migrator.HasColumn("epg_programs", "credits") {
				_ = tx.Exec("ALTER TABLE epg_programs DROP COLUMN credits").Error
			}

			return nil
		},
	}
}
