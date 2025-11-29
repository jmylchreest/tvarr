package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// epgSourceRepo implements EpgSourceRepository using GORM.
type epgSourceRepo struct {
	db *gorm.DB
}

// NewEpgSourceRepository creates a new EpgSourceRepository.
func NewEpgSourceRepository(db *gorm.DB) *epgSourceRepo {
	return &epgSourceRepo{db: db}
}

// Create creates a new EPG source.
func (r *epgSourceRepo) Create(ctx context.Context, source *models.EpgSource) error {
	if err := r.db.WithContext(ctx).Create(source).Error; err != nil {
		return fmt.Errorf("creating EPG source: %w", err)
	}
	return nil
}

// GetByID retrieves an EPG source by ID.
func (r *epgSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error) {
	var source models.EpgSource
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&source).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting EPG source by ID: %w", err)
	}
	return &source, nil
}

// GetAll retrieves all EPG sources.
func (r *epgSourceRepo) GetAll(ctx context.Context) ([]*models.EpgSource, error) {
	var sources []*models.EpgSource
	if err := r.db.WithContext(ctx).Order("priority DESC, name ASC").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("getting all EPG sources: %w", err)
	}
	return sources, nil
}

// GetEnabled retrieves all enabled EPG sources.
func (r *epgSourceRepo) GetEnabled(ctx context.Context) ([]*models.EpgSource, error) {
	var sources []*models.EpgSource
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("priority DESC, name ASC").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("getting enabled EPG sources: %w", err)
	}
	return sources, nil
}

// Update updates an existing EPG source.
func (r *epgSourceRepo) Update(ctx context.Context, source *models.EpgSource) error {
	if err := r.db.WithContext(ctx).Save(source).Error; err != nil {
		return fmt.Errorf("updating EPG source: %w", err)
	}
	return nil
}

// Delete deletes an EPG source by ID.
func (r *epgSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.EpgSource{}).Error; err != nil {
		return fmt.Errorf("deleting EPG source: %w", err)
	}
	return nil
}

// GetByName retrieves an EPG source by name.
func (r *epgSourceRepo) GetByName(ctx context.Context, name string) (*models.EpgSource, error) {
	var source models.EpgSource
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&source).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting EPG source by name: %w", err)
	}
	return &source, nil
}

// UpdateLastIngestion updates the last ingestion timestamp and status.
func (r *epgSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, programCount int) error {
	now := models.Now()
	updates := map[string]interface{}{
		"status":            status,
		"program_count":     programCount,
		"last_ingestion_at": now,
		"last_error":        "",
	}

	if err := r.db.WithContext(ctx).Model(&models.EpgSource{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("updating last ingestion: %w", err)
	}
	return nil
}

// Ensure epgSourceRepo implements EpgSourceRepository at compile time.
var _ EpgSourceRepository = (*epgSourceRepo)(nil)
