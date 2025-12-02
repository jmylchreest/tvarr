package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// channelRepo implements ChannelRepository using GORM.
type channelRepo struct {
	db *gorm.DB
}

// NewChannelRepository creates a new ChannelRepository.
func NewChannelRepository(db *gorm.DB) *channelRepo {
	return &channelRepo{db: db}
}

// Create creates a new channel.
func (r *channelRepo) Create(ctx context.Context, channel *models.Channel) error {
	if err := r.db.WithContext(ctx).Create(channel).Error; err != nil {
		return fmt.Errorf("creating channel: %w", err)
	}
	return nil
}

// CreateBatch creates multiple channels in a single batch.
func (r *channelRepo) CreateBatch(ctx context.Context, channels []*models.Channel) error {
	if len(channels) == 0 {
		return nil
	}

	if err := r.db.WithContext(ctx).Create(channels).Error; err != nil {
		return fmt.Errorf("creating channel batch: %w", err)
	}
	return nil
}

// CreateInBatches creates multiple channels in batches.
// This is optimized for bulk inserts to minimize memory usage.
func (r *channelRepo) CreateInBatches(ctx context.Context, channels []*models.Channel, batchSize int) error {
	if len(channels) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1000
	}

	if err := r.db.WithContext(ctx).CreateInBatches(channels, batchSize).Error; err != nil {
		return fmt.Errorf("creating channels in batches: %w", err)
	}
	return nil
}

// GetByID retrieves a channel by ID.
func (r *channelRepo) GetByID(ctx context.Context, id models.ULID) (*models.Channel, error) {
	var channel models.Channel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&channel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting channel by ID: %w", err)
	}
	return &channel, nil
}

// GetBySourceID retrieves all channels for a source using a callback for streaming.
// This is memory-efficient for large datasets as it processes channels in batches.
func (r *channelRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.Channel) error) error {
	const batchSize = 1000

	return r.db.WithContext(ctx).
		Where("source_id = ?", sourceID).
		Order("id ASC").
		FindInBatches(&[]models.Channel{}, batchSize, func(tx *gorm.DB, batch int) error {
			channels := tx.Statement.Dest.(*[]models.Channel)
			for i := range *channels {
				if err := callback(&(*channels)[i]); err != nil {
					return err
				}
			}
			return nil
		}).Error
}

// GetBySourceIDPaginated retrieves channels for a source with pagination.
func (r *channelRepo) GetBySourceIDPaginated(ctx context.Context, sourceID models.ULID, offset, limit int) ([]*models.Channel, int64, error) {
	var channels []*models.Channel
	var total int64

	// Get total count
	if err := r.db.WithContext(ctx).Model(&models.Channel{}).Where("source_id = ?", sourceID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("counting channels: %w", err)
	}

	// Get paginated results
	if err := r.db.WithContext(ctx).
		Where("source_id = ?", sourceID).
		Order("channel_name ASC").
		Offset(offset).
		Limit(limit).
		Find(&channels).Error; err != nil {
		return nil, 0, fmt.Errorf("getting paginated channels: %w", err)
	}

	return channels, total, nil
}

// Update updates an existing channel.
func (r *channelRepo) Update(ctx context.Context, channel *models.Channel) error {
	if err := r.db.WithContext(ctx).Save(channel).Error; err != nil {
		return fmt.Errorf("updating channel: %w", err)
	}
	return nil
}

// Delete deletes a channel by ID.
func (r *channelRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Channel{}).Error; err != nil {
		return fmt.Errorf("deleting channel: %w", err)
	}
	return nil
}

// DeleteBySourceID deletes all channels for a source.
func (r *channelRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	if err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).Delete(&models.Channel{}).Error; err != nil {
		return fmt.Errorf("deleting channels by source ID: %w", err)
	}
	return nil
}

// CountBySourceID returns the number of channels for a source.
func (r *channelRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.Channel{}).Where("source_id = ?", sourceID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting channels: %w", err)
	}
	return count, nil
}

// UpsertBySourceAndExtID updates or creates a channel based on source ID and external ID.
// This uses GORM's upsert functionality to handle duplicates efficiently.
func (r *channelRepo) UpsertBySourceAndExtID(ctx context.Context, channel *models.Channel) error {
	// Use ON CONFLICT for upsert
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_id"}, {Name: "ext_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tvg_id", "tvg_name", "tvg_logo", "group_title", "channel_name",
			"channel_number", "stream_url", "stream_type", "language",
			"country", "is_adult", "extra", "updated_at",
		}),
	}).Create(channel).Error; err != nil {
		return fmt.Errorf("upserting channel: %w", err)
	}
	return nil
}

// GetByExtID retrieves a channel by source ID and external ID.
func (r *channelRepo) GetByExtID(ctx context.Context, sourceID models.ULID, extID string) (*models.Channel, error) {
	var channel models.Channel
	if err := r.db.WithContext(ctx).Where("source_id = ? AND ext_id = ?", sourceID, extID).First(&channel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting channel by ext_id: %w", err)
	}
	return &channel, nil
}

// GetByTvgID retrieves channels by EPG ID (for matching with programs).
func (r *channelRepo) GetByTvgID(ctx context.Context, tvgID string) ([]*models.Channel, error) {
	var channels []*models.Channel
	if err := r.db.WithContext(ctx).Where("tvg_id = ?", tvgID).Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("getting channels by tvg_id: %w", err)
	}
	return channels, nil
}

// GetByGroupTitle retrieves channels by group/category.
func (r *channelRepo) GetByGroupTitle(ctx context.Context, groupTitle string) ([]*models.Channel, error) {
	var channels []*models.Channel
	if err := r.db.WithContext(ctx).Where("group_title = ?", groupTitle).Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("getting channels by group_title: %w", err)
	}
	return channels, nil
}

// GetDistinctGroups returns all unique group titles.
func (r *channelRepo) GetDistinctGroups(ctx context.Context) ([]string, error) {
	var groups []string
	if err := r.db.WithContext(ctx).
		Model(&models.Channel{}).
		Distinct("group_title").
		Where("group_title != ''").
		Order("group_title ASC").
		Pluck("group_title", &groups).Error; err != nil {
		return nil, fmt.Errorf("getting distinct groups: %w", err)
	}
	return groups, nil
}

// Transaction executes the given function within a database transaction.
// The provided function receives a transactional repository.
// If the function returns an error, the transaction is rolled back.
func (r *channelRepo) Transaction(ctx context.Context, fn func(ChannelRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &channelRepo{db: tx}
		return fn(txRepo)
	})
}

// Ensure channelRepo implements ChannelRepository at compile time.
var _ ChannelRepository = (*channelRepo)(nil)
