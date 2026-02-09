package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// streamSourceRepo implements StreamSourceRepository using GORM.
type streamSourceRepo struct {
	db *gorm.DB
}

// NewStreamSourceRepository creates a new StreamSourceRepository.
func NewStreamSourceRepository(db *gorm.DB) *streamSourceRepo {
	return &streamSourceRepo{db: db}
}

// Create creates a new stream source.
func (r *streamSourceRepo) Create(ctx context.Context, source *models.StreamSource) error {
	if err := r.db.WithContext(ctx).Create(source).Error; err != nil {
		return fmt.Errorf("creating stream source: %w", err)
	}
	return nil
}

// GetByID retrieves a stream source by ID.
func (r *streamSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	var source models.StreamSource
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&source).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting stream source by ID: %w", err)
	}
	return &source, nil
}

// GetAll retrieves all stream sources.
func (r *streamSourceRepo) GetAll(ctx context.Context) ([]*models.StreamSource, error) {
	var sources []*models.StreamSource
	if err := r.db.WithContext(ctx).Order("priority DESC, name ASC").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("getting all stream sources: %w", err)
	}
	return sources, nil
}

// GetEnabled retrieves all enabled stream sources.
func (r *streamSourceRepo) GetEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	var sources []*models.StreamSource
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("priority DESC, name ASC").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("getting enabled stream sources: %w", err)
	}
	return sources, nil
}

// Update updates an existing stream source.
func (r *streamSourceRepo) Update(ctx context.Context, source *models.StreamSource) error {
	if err := r.db.WithContext(ctx).Save(source).Error; err != nil {
		return fmt.Errorf("updating stream source: %w", err)
	}
	return nil
}

// Delete hard-deletes a stream source by ID.
// Uses Unscoped to permanently remove the record so the unique name
// constraint doesn't conflict when re-creating a source with the same name.
func (r *streamSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&models.StreamSource{}).Error; err != nil {
		return fmt.Errorf("deleting stream source: %w", err)
	}
	return nil
}

// GetByName retrieves a stream source by name.
func (r *streamSourceRepo) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	var source models.StreamSource
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&source).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting stream source by name: %w", err)
	}
	return &source, nil
}

// UpdateLastIngestion updates the last ingestion timestamp and status.
func (r *streamSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error {
	now := models.Now()
	updates := map[string]any{
		"status":            status,
		"channel_count":     channelCount,
		"last_ingestion_at": now,
		"last_error":        "",
	}

	if err := r.db.WithContext(ctx).Model(&models.StreamSource{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("updating last ingestion: %w", err)
	}
	return nil
}

// UpdateStatus updates only the status and optionally the error.
func (r *streamSourceRepo) UpdateStatus(ctx context.Context, id models.ULID, status string, lastError string) error {
	updates := map[string]any{
		"status":     status,
		"last_error": lastError,
	}

	if err := r.db.WithContext(ctx).Model(&models.StreamSource{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("updating status: %w", err)
	}
	return nil
}

// Ensure streamSourceRepo implements StreamSourceRepository at compile time.
var _ StreamSourceRepository = (*streamSourceRepo)(nil)
