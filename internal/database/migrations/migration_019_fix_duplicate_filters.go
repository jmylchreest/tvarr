package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration019FixDuplicateFilters cleans up duplicate "Exclude Adult Content" filters
// that may have been created by running both migration 002 and migration 015.
// Also upgrades the filter expression to use is_adult field in addition to keyword matching.
func migration019FixDuplicateFilters() Migration {
	return Migration{
		Version:     "019",
		Description: "Fix duplicate Exclude Adult Content filters and upgrade expression",
		Up: func(tx *gorm.DB) error {
			// Find all "Exclude Adult Content" system filters
			var filters []models.Filter
			if err := tx.Where("name = ? AND is_system = ?", "Exclude Adult Content", true).Find(&filters).Error; err != nil {
				return err
			}

			if len(filters) <= 1 {
				// No duplicates, but still update the expression if it exists
				if len(filters) == 1 {
					// Upgrade to improved expression that uses is_adult field
					improvedExpression := `is_adult == true OR group_title matches "(?i)\\b(adult|xxx|18\\+)" OR channel_name matches "(?i)\\b(adult|xxx|porn)"`
					if err := tx.Model(&filters[0]).Update("expression", improvedExpression).Error; err != nil {
						return err
					}
				}
				return nil
			}

			// Keep the first one (oldest), delete the rest
			keepID := filters[0].ID
			for i := 1; i < len(filters); i++ {
				if err := tx.Delete(&filters[i]).Error; err != nil {
					return err
				}
			}

			// Update the kept filter with the improved expression
			improvedExpression := `is_adult == true OR group_title matches "(?i)\\b(adult|xxx|18\\+)" OR channel_name matches "(?i)\\b(adult|xxx|porn)"`
			if err := tx.Model(&models.Filter{}).Where("id = ?", keepID).Update("expression", improvedExpression).Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Cannot restore duplicates - this is a cleanup migration
			// Revert expression to original simple form
			originalExpression := `group_title contains "adult" OR group_title contains "xxx" OR group_title contains "porn" OR channel_name contains "adult" OR channel_name contains "xxx" OR channel_name contains "porn"`
			return tx.Model(&models.Filter{}).
				Where("name = ? AND is_system = ?", "Exclude Adult Content", true).
				Update("expression", originalExpression).Error
		},
	}
}
