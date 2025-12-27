package repository

import (
	"context"
	"fmt"
	"time"

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

// UpsertBatch creates or updates multiple channels, handling duplicates gracefully.
// Uses ON CONFLICT to update existing channels based on (source_id, ext_id).
func (r *channelRepo) UpsertBatch(ctx context.Context, channels []*models.Channel) error {
	if len(channels) == 0 {
		return nil
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_id"}, {Name: "ext_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"tvg_id", "tvg_name", "tvg_logo", "group_title", "channel_name",
			"channel_number", "stream_url", "stream_type", "language",
			"country", "is_adult", "extra", "updated_at",
		}),
	}).Create(channels).Error; err != nil {
		return fmt.Errorf("upserting channel batch: %w", err)
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

// GetByIDWithSource retrieves a channel by ID with its Source relationship preloaded.
func (r *channelRepo) GetByIDWithSource(ctx context.Context, id models.ULID) (*models.Channel, error) {
	var channel models.Channel
	if err := r.db.WithContext(ctx).Preload("Source").Where("id = ?", id).First(&channel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting channel by ID with source: %w", err)
	}
	return &channel, nil
}

// GetBySourceID retrieves all channels for a source using a callback for streaming.
// Uses GORM's Rows() iterator for reliable row-by-row processing without batch issues.
func (r *channelRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.Channel) error) error {
	rows, err := r.db.WithContext(ctx).
		Model(&models.Channel{}).
		Where("source_id = ?", sourceID).
		Order("id ASC").
		Rows()
	if err != nil {
		return fmt.Errorf("querying channels: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var channel models.Channel
		if err := r.db.ScanRows(rows, &channel); err != nil {
			return fmt.Errorf("scanning channel row: %w", err)
		}
		if err := callback(&channel); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating channels: %w", err)
	}

	return nil
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

// Delete hard-deletes a channel by ID.
// Uses Unscoped() for permanent deletion for consistency with DeleteBySourceID.
func (r *channelRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&models.Channel{}).Error; err != nil {
		return fmt.Errorf("deleting channel: %w", err)
	}
	return nil
}

// DeleteBySourceID hard-deletes all channels for a source.
// Uses Unscoped() for permanent deletion since channels are fully replaced on each ingestion.
func (r *channelRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("source_id = ?", sourceID).Delete(&models.Channel{}).Error; err != nil {
		return fmt.Errorf("deleting channels by source ID: %w", err)
	}
	return nil
}

// DeleteStaleBySourceID deletes channels for a source that haven't been updated since the given time.
// This is used for "mark and sweep" cleanup: upsert updates the updated_at timestamp, so channels
// not present in the new data will have an older updated_at and will be deleted.
// Returns the number of channels deleted.
func (r *channelRepo) DeleteStaleBySourceID(ctx context.Context, sourceID models.ULID, olderThan time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Unscoped().
		Where("source_id = ? AND updated_at < ?", sourceID, olderThan).
		Delete(&models.Channel{})
	if result.Error != nil {
		return 0, fmt.Errorf("deleting stale channels: %w", result.Error)
	}
	return result.RowsAffected, nil
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

// allowedAutocompleteFields defines which fields can be queried for autocomplete.
// This prevents SQL injection by whitelisting allowed column names.
var allowedAutocompleteFields = map[string]string{
	"group_title":  "group_title",
	"channel_name": "channel_name",
	"tvg_id":       "tvg_id",
	"tvg_name":     "tvg_name",
	"country":      "country",
	"language":     "language",
}

// GetDistinctFieldValues returns distinct values for a channel field with occurrence counts.
// The field parameter must be one of the allowed fields (group_title, channel_name, tvg_id, country, language).
// Results are filtered by the query parameter (case-insensitive contains) and limited.
func (r *channelRepo) GetDistinctFieldValues(ctx context.Context, field string, query string, limit int) ([]FieldValueResult, error) {
	// Validate field name against whitelist to prevent SQL injection
	columnName, ok := allowedAutocompleteFields[field]
	if !ok {
		return nil, fmt.Errorf("invalid field name: %s", field)
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Build query for distinct values with counts
	var results []FieldValueResult
	db := r.db.WithContext(ctx).
		Model(&models.Channel{}).
		Select(columnName + " AS value, COUNT(*) AS count").
		Where(columnName + " IS NOT NULL AND " + columnName + " != ''").
		Group(columnName).
		Order("count DESC").
		Limit(limit)

	// Add search filter if query provided
	if query != "" {
		db = db.Where("LOWER("+columnName+") LIKE LOWER(?)", "%"+query+"%")
	}

	if err := db.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("getting distinct field values: %w", err)
	}

	return results, nil
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
