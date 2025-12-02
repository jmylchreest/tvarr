// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// relayProfileRepository implements RelayProfileRepository using GORM.
type relayProfileRepository struct {
	db *gorm.DB
}

// NewRelayProfileRepository creates a new RelayProfileRepository.
func NewRelayProfileRepository(db *gorm.DB) RelayProfileRepository {
	return &relayProfileRepository{db: db}
}

// Create creates a new relay profile.
func (r *relayProfileRepository) Create(ctx context.Context, profile *models.RelayProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validating relay profile: %w", err)
	}
	return r.db.WithContext(ctx).Create(profile).Error
}

// GetByID retrieves a relay profile by ID.
func (r *relayProfileRepository) GetByID(ctx context.Context, id models.ULID) (*models.RelayProfile, error) {
	var profile models.RelayProfile
	if err := r.db.WithContext(ctx).First(&profile, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// GetAll retrieves all relay profiles.
func (r *relayProfileRepository) GetAll(ctx context.Context) ([]*models.RelayProfile, error) {
	var profiles []*models.RelayProfile
	if err := r.db.WithContext(ctx).Order("is_default DESC, name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetByName retrieves a relay profile by name.
func (r *relayProfileRepository) GetByName(ctx context.Context, name string) (*models.RelayProfile, error) {
	var profile models.RelayProfile
	if err := r.db.WithContext(ctx).First(&profile, "name = ?", name).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// GetDefault retrieves the default relay profile.
func (r *relayProfileRepository) GetDefault(ctx context.Context) (*models.RelayProfile, error) {
	var profile models.RelayProfile
	if err := r.db.WithContext(ctx).First(&profile, "is_default = ?", true).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// Update updates an existing relay profile.
func (r *relayProfileRepository) Update(ctx context.Context, profile *models.RelayProfile) error {
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("validating relay profile: %w", err)
	}
	return r.db.WithContext(ctx).Save(profile).Error
}

// Delete deletes a relay profile by ID.
func (r *relayProfileRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Delete(&models.RelayProfile{}, "id = ?", id).Error
}

// Count returns the total number of relay profiles.
func (r *relayProfileRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.RelayProfile{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// SetDefault sets a profile as the default (unsets previous default).
func (r *relayProfileRepository) SetDefault(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Unset current default - use UpdateColumn to skip hooks
		if err := tx.Model(&models.RelayProfile{}).
			Where("is_default = ?", true).
			UpdateColumn("is_default", false).Error; err != nil {
			return err
		}
		// Set new default - use UpdateColumn to skip hooks
		result := tx.Model(&models.RelayProfile{}).
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

// Ensure relayProfileRepository implements RelayProfileRepository.
var _ RelayProfileRepository = (*relayProfileRepository)(nil)
