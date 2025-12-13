package migrations

import (
	"gorm.io/gorm"
)

// migration011ProxyFilterIsActive adds the is_active column to proxy_filters table.
// This allows users to disable a filter assignment without removing it from the proxy.
func migration011ProxyFilterIsActive() Migration {
	return Migration{
		Version:     "011",
		Description: "Add is_active column to proxy_filters",
		Up: func(tx *gorm.DB) error {
			// Add is_active column with default true
			// All existing filter assignments will be active by default
			if !tx.Migrator().HasColumn("proxy_filters", "is_active") {
				if err := tx.Exec("ALTER TABLE proxy_filters ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE").Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// SQLite doesn't support DROP COLUMN directly, but for testing we use
			// a workaround. In production, rollback isn't typically needed.
			// We use raw SQL to recreate the table without the column.
			if tx.Migrator().HasColumn("proxy_filters", "is_active") {
				// For SQLite, we need to recreate the table
				// Note: The ProxyFilter model uses "priority" column, not "priority_order"
				queries := []string{
					"CREATE TABLE proxy_filters_backup AS SELECT id, created_at, updated_at, deleted_at, proxy_id, filter_id, priority FROM proxy_filters",
					"DROP TABLE proxy_filters",
					"CREATE TABLE proxy_filters (id VARCHAR(26) PRIMARY KEY, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, proxy_id VARCHAR(26) NOT NULL, filter_id VARCHAR(26) NOT NULL, priority INTEGER DEFAULT 0)",
					"INSERT INTO proxy_filters SELECT * FROM proxy_filters_backup",
					"DROP TABLE proxy_filters_backup",
					"CREATE INDEX idx_proxy_filter ON proxy_filters (proxy_id, filter_id)",
				}
				for _, q := range queries {
					if err := tx.Exec(q).Error; err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}
