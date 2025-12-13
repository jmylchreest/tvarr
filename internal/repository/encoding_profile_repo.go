// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// encodingProfileRepository implements EncodingProfileRepository using GORM.
type encodingProfileRepository struct {
	db *gorm.DB
}

// NewEncodingProfileRepository creates a new EncodingProfileRepository.
func NewEncodingProfileRepository(db *gorm.DB) EncodingProfileRepository {
	return &encodingProfileRepository{db: db}
}

// Create creates a new encoding profile.
func (r *encodingProfileRepository) Create(ctx context.Context, profile *models.EncodingProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validating encoding profile: %w", err)
	}
	return r.db.WithContext(ctx).Create(profile).Error
}

// GetByID retrieves an encoding profile by ID.
func (r *encodingProfileRepository) GetByID(ctx context.Context, id models.ULID) (*models.EncodingProfile, error) {
	var profile models.EncodingProfile
	if err := r.db.WithContext(ctx).First(&profile, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// GetAll retrieves all encoding profiles.
func (r *encodingProfileRepository) GetAll(ctx context.Context) ([]*models.EncodingProfile, error) {
	var profiles []*models.EncodingProfile
	if err := r.db.WithContext(ctx).Order("is_default DESC, is_system DESC, name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetEnabled retrieves all enabled encoding profiles.
func (r *encodingProfileRepository) GetEnabled(ctx context.Context) ([]*models.EncodingProfile, error) {
	var profiles []*models.EncodingProfile
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("is_default DESC, is_system DESC, name ASC").
		Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetByName retrieves an encoding profile by name.
func (r *encodingProfileRepository) GetByName(ctx context.Context, name string) (*models.EncodingProfile, error) {
	var profile models.EncodingProfile
	if err := r.db.WithContext(ctx).First(&profile, "name = ?", name).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// GetDefault retrieves the default encoding profile.
func (r *encodingProfileRepository) GetDefault(ctx context.Context) (*models.EncodingProfile, error) {
	var profile models.EncodingProfile
	if err := r.db.WithContext(ctx).First(&profile, "is_default = ?", true).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// GetSystem retrieves all system encoding profiles.
func (r *encodingProfileRepository) GetSystem(ctx context.Context) ([]*models.EncodingProfile, error) {
	var profiles []*models.EncodingProfile
	if err := r.db.WithContext(ctx).
		Where("is_system = ?", true).
		Order("is_default DESC, name ASC").
		Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// Update updates an existing encoding profile.
func (r *encodingProfileRepository) Update(ctx context.Context, profile *models.EncodingProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validating encoding profile: %w", err)
	}
	return r.db.WithContext(ctx).Save(profile).Error
}

// Delete hard-deletes an encoding profile by ID.
// Uses Unscoped() for permanent deletion to avoid accumulating soft-deleted records.
func (r *encodingProfileRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&models.EncodingProfile{}, "id = ?", id).Error
}

// Count returns the total number of encoding profiles.
func (r *encodingProfileRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.EncodingProfile{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountEnabled returns the number of enabled encoding profiles.
func (r *encodingProfileRepository) CountEnabled(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.EncodingProfile{}).
		Where("enabled = ?", true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// SetDefault sets a profile as the default (unsets previous default).
func (r *encodingProfileRepository) SetDefault(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Unset current default - use UpdateColumn to skip hooks
		if err := tx.Model(&models.EncodingProfile{}).
			Where("is_default = ?", true).
			UpdateColumn("is_default", false).Error; err != nil {
			return err
		}
		// Set new default - use UpdateColumn to skip hooks
		result := tx.Model(&models.EncodingProfile{}).
			Where("id = ?", id).
			UpdateColumn("is_default", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// Ensure encodingProfileRepository implements EncodingProfileRepository.
var _ EncodingProfileRepository = (*encodingProfileRepository)(nil)
