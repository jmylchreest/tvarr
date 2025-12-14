// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// clientDetectionRuleRepository implements ClientDetectionRuleRepository using GORM.
type clientDetectionRuleRepository struct {
	db *gorm.DB
}

// NewClientDetectionRuleRepository creates a new ClientDetectionRuleRepository.
func NewClientDetectionRuleRepository(db *gorm.DB) ClientDetectionRuleRepository {
	return &clientDetectionRuleRepository{db: db}
}

// Create creates a new client detection rule.
func (r *clientDetectionRuleRepository) Create(ctx context.Context, rule *models.ClientDetectionRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validating rule: %w", err)
	}
	return r.db.WithContext(ctx).Create(rule).Error
}

// GetByID retrieves a client detection rule by ID.
func (r *clientDetectionRuleRepository) GetByID(ctx context.Context, id models.ULID) (*models.ClientDetectionRule, error) {
	var rule models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Preload("EncodingProfile").
		First(&rule, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// GetByIDs retrieves client detection rules by multiple IDs.
func (r *clientDetectionRuleRepository) GetByIDs(ctx context.Context, ids []models.ULID) ([]*models.ClientDetectionRule, error) {
	if len(ids) == 0 {
		return []*models.ClientDetectionRule{}, nil
	}
	var rules []*models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Preload("EncodingProfile").
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetAll retrieves all client detection rules ordered by priority.
func (r *clientDetectionRuleRepository) GetAll(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	var rules []*models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Preload("EncodingProfile").
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetEnabled retrieves all enabled rules ordered by priority.
func (r *clientDetectionRuleRepository) GetEnabled(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	var rules []*models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Where("is_enabled = ?", true).
		Preload("EncodingProfile").
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetUserCreated retrieves all user-created client detection rules (IsSystem=false).
func (r *clientDetectionRuleRepository) GetUserCreated(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	var rules []*models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Where("is_system = ?", false).
		Preload("EncodingProfile").
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// GetByName retrieves a rule by name.
func (r *clientDetectionRuleRepository) GetByName(ctx context.Context, name string) (*models.ClientDetectionRule, error) {
	var rule models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Preload("EncodingProfile").
		Where("name = ?", name).
		First(&rule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// GetSystem retrieves all system rules.
func (r *clientDetectionRuleRepository) GetSystem(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	var rules []*models.ClientDetectionRule
	if err := r.db.WithContext(ctx).
		Where("is_system = ?", true).
		Preload("EncodingProfile").
		Order("priority ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// Update updates an existing rule.
func (r *clientDetectionRuleRepository) Update(ctx context.Context, rule *models.ClientDetectionRule) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("validating rule: %w", err)
	}
	return r.db.WithContext(ctx).Save(rule).Error
}

// Delete deletes a rule by ID.
func (r *clientDetectionRuleRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Delete(&models.ClientDetectionRule{}, "id = ?", id).Error
}

// Count returns the total number of rules.
func (r *clientDetectionRuleRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.ClientDetectionRule{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountEnabled returns the number of enabled rules.
func (r *clientDetectionRuleRepository) CountEnabled(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.ClientDetectionRule{}).
		Where("is_enabled = ?", true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Reorder updates priorities for multiple rules in a single transaction.
func (r *clientDetectionRuleRepository) Reorder(ctx context.Context, reorders []ReorderRequest) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, req := range reorders {
			if err := tx.Model(&models.ClientDetectionRule{}).
				Where("id = ?", req.ID).
				Update("priority", req.Priority).Error; err != nil {
				return fmt.Errorf("updating priority for %s: %w", req.ID, err)
			}
		}
		return nil
	})
}

// Ensure clientDetectionRuleRepository implements ClientDetectionRuleRepository.
var _ ClientDetectionRuleRepository = (*clientDetectionRuleRepository)(nil)
