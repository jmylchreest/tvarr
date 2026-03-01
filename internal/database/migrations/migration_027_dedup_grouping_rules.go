package migrations

import (
	"gorm.io/gorm"
)

// migration027DedupGroupingRules hard-deletes the duplicate stream data mapping
// rules left over from migration 015. Migration 016 intended to replace the 015
// rules but its delete list used the old "Normalize *" names — by the time 016
// ran, 015 had already created rules with "Group *" names, so those were never
// deleted. Every "Group *" rule (Sports, Movies, News, etc.) ended up with two
// live rows of the same name, same priority, same expression.
//
// This migration hard-deletes the older copy of each duplicate (the one with the
// earlier created_at), leaving only the 016-era canonical row. It operates via
// raw SQL to bypass GORM's soft-delete and actually remove the rows.
func migration027DedupGroupingRules() Migration {
	return Migration{
		Version:     "027",
		Description: "Hard-delete duplicate stream grouping rules left by migration 015",
		Up: func(tx *gorm.DB) error {
			// Hard-delete rows where the same (name, source_type, is_system) combination
			// has more than one non-deleted row, keeping only the most recently created one.
			// Uses a correlated subquery: delete rows whose created_at is NOT the MAX
			// created_at for that name group.
			sql := `
				DELETE FROM data_mapping_rules
				WHERE deleted_at IS NULL
				  AND is_system = 1
				  AND source_type = 'stream'
				  AND id NOT IN (
				      SELECT id FROM (
				          SELECT id
				          FROM data_mapping_rules
				          WHERE deleted_at IS NULL
				            AND is_system = 1
				            AND source_type = 'stream'
				            AND name NOT IN ('Timeshift Detection')
				          GROUP BY name
				          HAVING created_at = MAX(created_at)
				      )
				  )
				  AND name NOT IN ('Timeshift Detection')
			`
			return tx.Exec(sql).Error
		},
		Down: func(tx *gorm.DB) error {
			// Cannot restore hard-deleted rows; this migration is irreversible.
			// The duplicates would be re-created by re-running migration 015 Up.
			return nil
		},
	}
}
