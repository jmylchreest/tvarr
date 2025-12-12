package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// relayProfileMappingRepository implements RelayProfileMappingRepository using GORM.
type relayProfileMappingRepository struct {
	db *gorm.DB
}

// NewRelayProfileMappingRepository creates a new RelayProfileMappingRepository.
func NewRelayProfileMappingRepository(db *gorm.DB) RelayProfileMappingRepository {
	return &relayProfileMappingRepository{db: db}
}

// Create creates a new relay profile mapping.
func (r *relayProfileMappingRepository) Create(ctx context.Context, mapping *models.RelayProfileMapping) error {
	if mapping.Name == "" {
		return fmt.Errorf("mapping name is required")
	}
	if mapping.Expression == "" {
		return fmt.Errorf("mapping expression is required")
	}
	return r.db.WithContext(ctx).Create(mapping).Error
}

// GetByID retrieves a relay profile mapping by ID.
func (r *relayProfileMappingRepository) GetByID(ctx context.Context, id models.ULID) (*models.RelayProfileMapping, error) {
	var mapping models.RelayProfileMapping
	if err := r.db.WithContext(ctx).First(&mapping, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &mapping, nil
}

// GetAll retrieves all relay profile mappings.
func (r *relayProfileMappingRepository) GetAll(ctx context.Context) ([]*models.RelayProfileMapping, error) {
	var mappings []*models.RelayProfileMapping
	if err := r.db.WithContext(ctx).Order("priority ASC, created_at ASC").Find(&mappings).Error; err != nil {
		return nil, err
	}
	return mappings, nil
}

// GetEnabled retrieves all enabled relay profile mappings ordered by priority.
func (r *relayProfileMappingRepository) GetEnabled(ctx context.Context) ([]*models.RelayProfileMapping, error) {
	var mappings []*models.RelayProfileMapping
	if err := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Order("priority ASC, created_at ASC").
		Find(&mappings).Error; err != nil {
		return nil, err
	}
	return mappings, nil
}

// Update updates an existing relay profile mapping.
func (r *relayProfileMappingRepository) Update(ctx context.Context, mapping *models.RelayProfileMapping) error {
	if mapping.Name == "" {
		return fmt.Errorf("mapping name is required")
	}
	if mapping.Expression == "" {
		return fmt.Errorf("mapping expression is required")
	}
	return r.db.WithContext(ctx).Save(mapping).Error
}

// Delete hard-deletes a relay profile mapping by ID.
// Uses Unscoped() for permanent deletion to avoid soft-deleted records being accidentally used.
func (r *relayProfileMappingRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&models.RelayProfileMapping{}, "id = ?", id).Error
}

// Count returns the total number of relay profile mappings.
func (r *relayProfileMappingRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.RelayProfileMapping{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountEnabled returns the number of enabled mappings.
func (r *relayProfileMappingRepository) CountEnabled(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.RelayProfileMapping{}).
		Where("is_enabled = ?", true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountSystem returns the number of system mappings.
func (r *relayProfileMappingRepository) CountSystem(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.RelayProfileMapping{}).
		Where("is_system = ?", true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Reorder updates the priority of multiple mappings in a batch.
func (r *relayProfileMappingRepository) Reorder(ctx context.Context, requests []ReorderRequest) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, req := range requests {
			if err := tx.Model(&models.RelayProfileMapping{}).
				Where("id = ?", req.ID).
				UpdateColumn("priority", req.Priority).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Ensure relayProfileMappingRepository implements RelayProfileMappingRepository.
var _ RelayProfileMappingRepository = (*relayProfileMappingRepository)(nil)
