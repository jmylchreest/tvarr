// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// dataMappingRuleRepository implements DataMappingRuleRepository using GORM.
type dataMappingRuleRepository struct {
	db *gorm.DB
}

// NewDataMappingRuleRepository creates a new DataMappingRuleRepository.
func NewDataMappingRuleRepository(db *gorm.DB) DataMappingRuleRepository {
	return &dataMappingRuleRepository{db: db}
}

// Create creates a new data mapping rule.
func (r *dataMappingRuleRepository) Create(ctx context.Context, rule *models.DataMappingRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validating rule: %w", err)
	}
	return r.db.WithContext(ctx).Create(rule).Error
}

// GetByID retrieves a data mapping rule by ID.
func (r *dataMappingRuleRepository) GetByID(ctx context.Context, id models.ULID) (*models.DataMappingRule, error) {
	var rule models.DataMappingRule
	if err := r.db.WithContext(ctx).First(&rule, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// GetByIDs retrieves data mapping rules by multiple IDs.
func (r *dataMappingRuleRepository) GetByIDs(ctx context.Context, ids []models.ULID) ([]*models.DataMappingRule, error) {
	if len(ids) == 0 {
		return []*models.DataMappingRule{}, nil
	}
	var rules []*models.DataMappingRule
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetByName retrieves a data mapping rule by name.
func (r *dataMappingRuleRepository) GetByName(ctx context.Context, name string) (*models.DataMappingRule, error) {
	var rule models.DataMappingRule
	if err := r.db.WithContext(ctx).First(&rule, "name = ?", name).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// GetAll retrieves all data mapping rules.
func (r *dataMappingRuleRepository) GetAll(ctx context.Context) ([]*models.DataMappingRule, error) {
	var rules []*models.DataMappingRule
	if err := r.db.WithContext(ctx).Order("priority ASC, created_at ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetEnabled retrieves all enabled data mapping rules.
func (r *dataMappingRuleRepository) GetEnabled(ctx context.Context) ([]*models.DataMappingRule, error) {
	var rules []*models.DataMappingRule
	if err := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetUserCreated retrieves all user-created data mapping rules (IsSystem=false).
func (r *dataMappingRuleRepository) GetUserCreated(ctx context.Context) ([]*models.DataMappingRule, error) {
	var rules []*models.DataMappingRule
	if err := r.db.WithContext(ctx).
		Where("is_system = ?", false).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetBySourceType retrieves rules by source type.
func (r *dataMappingRuleRepository) GetBySourceType(ctx context.Context, sourceType models.DataMappingRuleSourceType) ([]*models.DataMappingRule, error) {
	var rules []*models.DataMappingRule
	if err := r.db.WithContext(ctx).
		Where("source_type = ?", sourceType).
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetBySourceID retrieves rules for a specific source.
func (r *dataMappingRuleRepository) GetBySourceID(ctx context.Context, sourceID *models.ULID) ([]*models.DataMappingRule, error) {
	var rules []*models.DataMappingRule
	query := r.db.WithContext(ctx)
	if sourceID != nil {
		query = query.Where("source_id = ? OR source_id IS NULL", sourceID)
	} else {
		query = query.Where("source_id IS NULL")
	}
	if err := query.Order("priority ASC, created_at ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetEnabledForSourceType retrieves enabled rules for a source type, ordered by priority.
func (r *dataMappingRuleRepository) GetEnabledForSourceType(ctx context.Context, sourceType models.DataMappingRuleSourceType, sourceID *models.ULID) ([]*models.DataMappingRule, error) {
	var rules []*models.DataMappingRule
	query := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Where("source_type = ?", sourceType)

	if sourceID != nil {
		// Get global rules (source_id IS NULL) and source-specific rules
		query = query.Where("source_id = ? OR source_id IS NULL", sourceID)
	} else {
		// Only global rules
		query = query.Where("source_id IS NULL")
	}

	if err := query.Order("priority ASC, created_at ASC").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// Update updates an existing data mapping rule.
func (r *dataMappingRuleRepository) Update(ctx context.Context, rule *models.DataMappingRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validating rule: %w", err)
	}
	return r.db.WithContext(ctx).Save(rule).Error
}

// Delete deletes a data mapping rule by ID.
func (r *dataMappingRuleRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Delete(&models.DataMappingRule{}, "id = ?", id).Error
}

// Count returns the total number of rules.
func (r *dataMappingRuleRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.DataMappingRule{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Ensure dataMappingRuleRepository implements DataMappingRuleRepository.
var _ DataMappingRuleRepository = (*dataMappingRuleRepository)(nil)
