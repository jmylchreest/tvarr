// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// filterRepository implements FilterRepository using GORM.
type filterRepository struct {
	db *gorm.DB
}

// NewFilterRepository creates a new FilterRepository.
func NewFilterRepository(db *gorm.DB) FilterRepository {
	return &filterRepository{db: db}
}

// Create creates a new filter.
func (r *filterRepository) Create(ctx context.Context, filter *models.Filter) error {
	if err := filter.Validate(); err != nil {
		return fmt.Errorf("validating filter: %w", err)
	}
	return r.db.WithContext(ctx).Create(filter).Error
}

// GetByID retrieves a filter by ID.
func (r *filterRepository) GetByID(ctx context.Context, id models.ULID) (*models.Filter, error) {
	var filter models.Filter
	if err := r.db.WithContext(ctx).First(&filter, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &filter, nil
}

// GetAll retrieves all filters.
func (r *filterRepository) GetAll(ctx context.Context) ([]*models.Filter, error) {
	var filters []*models.Filter
	if err := r.db.WithContext(ctx).Order("priority ASC, created_at ASC").Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetEnabled retrieves all enabled filters.
func (r *filterRepository) GetEnabled(ctx context.Context) ([]*models.Filter, error) {
	var filters []*models.Filter
	if err := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Order("priority ASC, created_at ASC").
		Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetBySourceType retrieves filters by source type.
func (r *filterRepository) GetBySourceType(ctx context.Context, sourceType models.FilterSourceType) ([]*models.Filter, error) {
	var filters []*models.Filter
	if err := r.db.WithContext(ctx).
		Where("source_type = ?", sourceType).
		Order("priority ASC, created_at ASC").
		Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetBySourceID retrieves filters for a specific source.
func (r *filterRepository) GetBySourceID(ctx context.Context, sourceID *models.ULID) ([]*models.Filter, error) {
	var filters []*models.Filter
	query := r.db.WithContext(ctx)
	if sourceID != nil {
		query = query.Where("source_id = ? OR source_id IS NULL", sourceID)
	} else {
		query = query.Where("source_id IS NULL")
	}
	if err := query.Order("priority ASC, created_at ASC").Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetEnabledForSourceType retrieves enabled filters for a source type, ordered by priority.
func (r *filterRepository) GetEnabledForSourceType(ctx context.Context, sourceType models.FilterSourceType, sourceID *models.ULID) ([]*models.Filter, error) {
	var filters []*models.Filter
	query := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Where("source_type = ?", sourceType)

	if sourceID != nil {
		// Get global filters (source_id IS NULL) and source-specific filters
		query = query.Where("source_id = ? OR source_id IS NULL", sourceID)
	} else {
		// Only global filters
		query = query.Where("source_id IS NULL")
	}

	if err := query.Order("priority ASC, created_at ASC").Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// Update updates an existing filter.
func (r *filterRepository) Update(ctx context.Context, filter *models.Filter) error {
	if err := filter.Validate(); err != nil {
		return fmt.Errorf("validating filter: %w", err)
	}
	return r.db.WithContext(ctx).Save(filter).Error
}

// Delete deletes a filter by ID.
func (r *filterRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Delete(&models.Filter{}, "id = ?", id).Error
}

// Count returns the total number of filters.
func (r *filterRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.Filter{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Ensure filterRepository implements FilterRepository.
var _ FilterRepository = (*filterRepository)(nil)
