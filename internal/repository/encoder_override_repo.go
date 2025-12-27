// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// encoderOverrideRepository implements EncoderOverrideRepository using GORM.
type encoderOverrideRepository struct {
	db *gorm.DB
}

// NewEncoderOverrideRepository creates a new EncoderOverrideRepository.
func NewEncoderOverrideRepository(db *gorm.DB) EncoderOverrideRepository {
	return &encoderOverrideRepository{db: db}
}

// Create creates a new encoder override.
func (r *encoderOverrideRepository) Create(ctx context.Context, override *models.EncoderOverride) error {
	if err := override.Validate(); err != nil {
		return fmt.Errorf("validating override: %w", err)
	}
	return r.db.WithContext(ctx).Create(override).Error
}

// GetByID retrieves an encoder override by ID.
func (r *encoderOverrideRepository) GetByID(ctx context.Context, id models.ULID) (*models.EncoderOverride, error) {
	var override models.EncoderOverride
	if err := r.db.WithContext(ctx).
		First(&override, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &override, nil
}

// GetAll retrieves all encoder overrides ordered by priority (highest first).
func (r *encoderOverrideRepository) GetAll(ctx context.Context) ([]*models.EncoderOverride, error) {
	var overrides []*models.EncoderOverride
	if err := r.db.WithContext(ctx).
		Order("priority DESC, created_at ASC").
		Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// GetEnabled retrieves all enabled overrides ordered by priority (highest first).
func (r *encoderOverrideRepository) GetEnabled(ctx context.Context) ([]*models.EncoderOverride, error) {
	var overrides []*models.EncoderOverride
	if err := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Order("priority DESC, created_at ASC").
		Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// GetByCodecType retrieves overrides for a specific codec type (video/audio).
func (r *encoderOverrideRepository) GetByCodecType(ctx context.Context, codecType models.EncoderOverrideCodecType) ([]*models.EncoderOverride, error) {
	var overrides []*models.EncoderOverride
	if err := r.db.WithContext(ctx).
		Where("codec_type = ?", codecType).
		Order("priority DESC, created_at ASC").
		Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// GetUserCreated retrieves all user-created encoder overrides (IsSystem=false).
func (r *encoderOverrideRepository) GetUserCreated(ctx context.Context) ([]*models.EncoderOverride, error) {
	var overrides []*models.EncoderOverride
	if err := r.db.WithContext(ctx).
		Where("is_system = ?", false).
		Order("priority DESC, created_at ASC").
		Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// GetByName retrieves an override by name.
func (r *encoderOverrideRepository) GetByName(ctx context.Context, name string) (*models.EncoderOverride, error) {
	var override models.EncoderOverride
	if err := r.db.WithContext(ctx).
		Where("name = ?", name).
		First(&override).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &override, nil
}

// GetSystem retrieves all system overrides.
func (r *encoderOverrideRepository) GetSystem(ctx context.Context) ([]*models.EncoderOverride, error) {
	var overrides []*models.EncoderOverride
	if err := r.db.WithContext(ctx).
		Where("is_system = ?", true).
		Order("priority DESC, created_at ASC").
		Find(&overrides).Error; err != nil {
		return nil, err
	}
	return overrides, nil
}

// Update updates an existing override.
func (r *encoderOverrideRepository) Update(ctx context.Context, override *models.EncoderOverride) error {
	if err := override.Validate(); err != nil {
		return fmt.Errorf("validating override: %w", err)
	}
	return r.db.WithContext(ctx).Save(override).Error
}

// Delete deletes an override by ID.
func (r *encoderOverrideRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Delete(&models.EncoderOverride{}, "id = ?", id).Error
}

// Count returns the total number of overrides.
func (r *encoderOverrideRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.EncoderOverride{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountEnabled returns the number of enabled overrides.
func (r *encoderOverrideRepository) CountEnabled(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.EncoderOverride{}).
		Where("is_enabled = ?", true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Reorder updates priorities for multiple overrides in a single transaction.
func (r *encoderOverrideRepository) Reorder(ctx context.Context, reorders []ReorderRequest) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, req := range reorders {
			if err := tx.Model(&models.EncoderOverride{}).
				Where("id = ?", req.ID).
				Update("priority", req.Priority).Error; err != nil {
				return fmt.Errorf("updating priority for %s: %w", req.ID, err)
			}
		}
		return nil
	})
}

// Ensure encoderOverrideRepository implements EncoderOverrideRepository.
var _ EncoderOverrideRepository = (*encoderOverrideRepository)(nil)
