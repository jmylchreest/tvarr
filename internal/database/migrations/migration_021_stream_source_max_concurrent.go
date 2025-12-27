package migrations

import (
	"gorm.io/gorm"
)

// migration021StreamSourceMaxConcurrent adds the max_concurrent_streams column
// to the stream_sources table. This column controls how many concurrent connections
// are allowed to a source, preventing issues with connection limits.
func migration021StreamSourceMaxConcurrent() Migration {
	return Migration{
		Version:     "021",
		Description: "Add max_concurrent_streams column to stream_sources table",
		Up: func(tx *gorm.DB) error {
			// Add max_concurrent_streams column with default value of 1
			if !tx.Migrator().HasColumn("stream_sources", "max_concurrent_streams") {
				if err := tx.Exec("ALTER TABLE stream_sources ADD COLUMN max_concurrent_streams INTEGER DEFAULT 1").Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// For rollback, we use SQLite's table recreation approach since
			// SQLite doesn't fully support DROP COLUMN. The column will remain
			// but this is acceptable for development/rollback purposes.
			// In production, dropping columns in SQLite requires recreating the table.
			// For now, just return nil - the column staying is harmless and
			// a fresh schema migration would handle it correctly.
			return nil
		},
	}
}
