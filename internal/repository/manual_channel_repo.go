package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// manualChannelRepo implements ManualStreamChannelRepository using GORM.
type manualChannelRepo struct {
	db *gorm.DB
}

// NewManualChannelRepository creates a new ManualStreamChannelRepository.
func NewManualChannelRepository(db *gorm.DB) *manualChannelRepo {
	return &manualChannelRepo{db: db}
}

// Create creates a new manual channel.
func (r *manualChannelRepo) Create(ctx context.Context, channel *models.ManualStreamChannel) error {
	if err := r.db.WithContext(ctx).Create(channel).Error; err != nil {
		return fmt.Errorf("creating manual channel: %w", err)
	}
	return nil
}

// GetByID retrieves a manual channel by ID.
func (r *manualChannelRepo) GetByID(ctx context.Context, id models.ULID) (*models.ManualStreamChannel, error) {
	var channel models.ManualStreamChannel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&channel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting manual channel by ID: %w", err)
	}
	return &channel, nil
}

// GetAll retrieves all manual channels.
func (r *manualChannelRepo) GetAll(ctx context.Context) ([]*models.ManualStreamChannel, error) {
	var channels []*models.ManualStreamChannel
	if err := r.db.WithContext(ctx).Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("getting all manual channels: %w", err)
	}
	return channels, nil
}

// GetBySourceID retrieves all manual channels for a source.
func (r *manualChannelRepo) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	var channels []*models.ManualStreamChannel
	if err := r.db.WithContext(ctx).
		Where("source_id = ?", sourceID).
		Order("priority DESC, channel_name ASC").
		Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("getting manual channels by source ID: %w", err)
	}
	return channels, nil
}

// GetEnabledBySourceID retrieves enabled manual channels for a source, ordered by priority.
func (r *manualChannelRepo) GetEnabledBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	var channels []*models.ManualStreamChannel
	if err := r.db.WithContext(ctx).
		Where("source_id = ? AND enabled = ?", sourceID, true).
		Order("priority DESC, channel_name ASC").
		Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("getting enabled manual channels by source ID: %w", err)
	}
	return channels, nil
}

// Update updates an existing manual channel.
func (r *manualChannelRepo) Update(ctx context.Context, channel *models.ManualStreamChannel) error {
	if err := r.db.WithContext(ctx).Save(channel).Error; err != nil {
		return fmt.Errorf("updating manual channel: %w", err)
	}
	return nil
}

// Delete deletes a manual channel by ID.
func (r *manualChannelRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Delete(&models.ManualStreamChannel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("deleting manual channel: %w", err)
	}
	return nil
}

// DeleteBySourceID deletes all manual channels for a source.
func (r *manualChannelRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	if err := r.db.WithContext(ctx).Delete(&models.ManualStreamChannel{}, "source_id = ?", sourceID).Error; err != nil {
		return fmt.Errorf("deleting manual channels by source ID: %w", err)
	}
	return nil
}

// CountBySourceID returns the number of manual channels for a source.
func (r *manualChannelRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.ManualStreamChannel{}).
		Where("source_id = ?", sourceID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting manual channels by source ID: %w", err)
	}
	return count, nil
}
