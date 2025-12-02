// Package repository provides data access implementations.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lastKnownCodecRepository implements LastKnownCodecRepository using GORM.
type lastKnownCodecRepository struct {
	db *gorm.DB
}

// NewLastKnownCodecRepository creates a new LastKnownCodecRepository.
func NewLastKnownCodecRepository(db *gorm.DB) LastKnownCodecRepository {
	return &lastKnownCodecRepository{db: db}
}

// Create creates a new codec cache entry.
func (r *lastKnownCodecRepository) Create(ctx context.Context, codec *models.LastKnownCodec) error {
	if err := codec.Validate(); err != nil {
		return fmt.Errorf("validating codec entry: %w", err)
	}
	return r.db.WithContext(ctx).Create(codec).Error
}

// GetByID retrieves a codec entry by ID.
func (r *lastKnownCodecRepository) GetByID(ctx context.Context, id models.ULID) (*models.LastKnownCodec, error) {
	var codec models.LastKnownCodec
	if err := r.db.WithContext(ctx).First(&codec, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &codec, nil
}

// GetByStreamURL retrieves codec info by stream URL.
func (r *lastKnownCodecRepository) GetByStreamURL(ctx context.Context, streamURL string) (*models.LastKnownCodec, error) {
	var codec models.LastKnownCodec
	if err := r.db.WithContext(ctx).First(&codec, "stream_url = ?", streamURL).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &codec, nil
}

// GetBySourceID retrieves all codec entries for a source.
func (r *lastKnownCodecRepository) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.LastKnownCodec, error) {
	var codecs []*models.LastKnownCodec
	if err := r.db.WithContext(ctx).
		Where("source_id = ?", sourceID).
		Order("probed_at DESC").
		Find(&codecs).Error; err != nil {
		return nil, err
	}
	return codecs, nil
}

// Upsert creates or updates a codec entry based on stream URL.
func (r *lastKnownCodecRepository) Upsert(ctx context.Context, codec *models.LastKnownCodec) error {
	if err := codec.Validate(); err != nil {
		return fmt.Errorf("validating codec entry: %w", err)
	}

	// Use GORM's upsert functionality with ON CONFLICT
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "stream_url"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"source_id",
			"video_codec", "video_profile", "video_level",
			"video_width", "video_height", "video_framerate",
			"video_bitrate", "video_pix_fmt",
			"audio_codec", "audio_sample_rate", "audio_channels", "audio_bitrate",
			"container_format", "duration",
			"is_live_stream", "has_subtitles", "stream_count", "title",
			"probed_at", "probe_error", "probe_ms",
			"expires_at", "hit_count",
			"updated_at",
		}),
	}).Create(codec).Error
}

// Update updates an existing codec entry.
func (r *lastKnownCodecRepository) Update(ctx context.Context, codec *models.LastKnownCodec) error {
	if err := codec.Validate(); err != nil {
		return fmt.Errorf("validating codec entry: %w", err)
	}
	return r.db.WithContext(ctx).Save(codec).Error
}

// Delete deletes a codec entry by ID.
func (r *lastKnownCodecRepository) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Delete(&models.LastKnownCodec{}, "id = ?", id).Error
}

// DeleteByStreamURL deletes a codec entry by stream URL.
func (r *lastKnownCodecRepository) DeleteByStreamURL(ctx context.Context, streamURL string) error {
	return r.db.WithContext(ctx).Delete(&models.LastKnownCodec{}, "stream_url = ?", streamURL).Error
}

// DeleteBySourceID deletes all codec entries for a source.
func (r *lastKnownCodecRepository) DeleteBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	result := r.db.WithContext(ctx).Delete(&models.LastKnownCodec{}, "source_id = ?", sourceID)
	return result.RowsAffected, result.Error
}

// DeleteExpired deletes expired codec entries.
func (r *lastKnownCodecRepository) DeleteExpired(ctx context.Context) (int64, error) {
	now := time.Now()
	result := r.db.WithContext(ctx).Delete(&models.LastKnownCodec{}, "expires_at IS NOT NULL AND expires_at < ?", now)
	return result.RowsAffected, result.Error
}

// Touch updates the access time and increments hit count for a stream URL.
func (r *lastKnownCodecRepository) Touch(ctx context.Context, streamURL string) error {
	now := models.Now()
	// Use UpdateColumns to skip hooks since we're doing a partial update
	result := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).
		Where("stream_url = ?", streamURL).
		UpdateColumns(map[string]interface{}{
			"hit_count":  gorm.Expr("hit_count + 1"),
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return models.ErrStreamURLNotFound
	}
	return nil
}

// GetValidCount returns the count of valid (non-expired, no error) entries.
func (r *lastKnownCodecRepository) GetValidCount(ctx context.Context) (int64, error) {
	var count int64
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).
		Where("probe_error = '' OR probe_error IS NULL").
		Where("video_codec != '' OR audio_codec != ''").
		Where("expires_at IS NULL OR expires_at > ?", now).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetStats returns cache statistics.
func (r *lastKnownCodecRepository) GetStats(ctx context.Context) (*CodecCacheStats, error) {
	stats := &CodecCacheStats{}
	now := time.Now()

	// Total entries
	if err := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).Count(&stats.TotalEntries).Error; err != nil {
		return nil, err
	}

	// Valid entries (has codec, no error, not expired)
	if err := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).
		Where("(probe_error = '' OR probe_error IS NULL)").
		Where("(video_codec != '' OR audio_codec != '')").
		Where("(expires_at IS NULL OR expires_at > ?)", now).
		Count(&stats.ValidEntries).Error; err != nil {
		return nil, err
	}

	// Expired entries
	if err := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).
		Where("expires_at IS NOT NULL AND expires_at < ?", now).
		Count(&stats.ExpiredEntries).Error; err != nil {
		return nil, err
	}

	// Error entries
	if err := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).
		Where("probe_error != '' AND probe_error IS NOT NULL").
		Count(&stats.ErrorEntries).Error; err != nil {
		return nil, err
	}

	// Total hits
	var totalHits struct {
		Sum int64
	}
	if err := r.db.WithContext(ctx).Model(&models.LastKnownCodec{}).
		Select("COALESCE(SUM(hit_count), 0) as sum").
		Scan(&totalHits).Error; err != nil {
		return nil, err
	}
	stats.TotalHits = totalHits.Sum

	return stats, nil
}

// Ensure lastKnownCodecRepository implements LastKnownCodecRepository.
var _ LastKnownCodecRepository = (*lastKnownCodecRepository)(nil)
