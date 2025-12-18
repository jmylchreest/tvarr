package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration014RenameTimeshiftRule renames the system data mapping rule from
// "Default Timeshift Detection (Regex)" to "Timeshift Detection" for brevity.
func migration014RenameTimeshiftRule() Migration {
	return Migration{
		Version:     "014",
		Description: "Rename system timeshift detection rule to shorter name",
		Up: func(tx *gorm.DB) error {
			return tx.Model(&models.DataMappingRule{}).
				Where("name = ? AND is_system = ?", "Default Timeshift Detection (Regex)", true).
				Update("name", "Timeshift Detection").Error
		},
		Down: func(tx *gorm.DB) error {
			return tx.Model(&models.DataMappingRule{}).
				Where("name = ? AND is_system = ?", "Timeshift Detection", true).
				Update("name", "Default Timeshift Detection (Regex)").Error
		},
	}
}
