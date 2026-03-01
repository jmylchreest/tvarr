package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/internal/database"
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// epgProgramRepo implements EpgProgramRepository using GORM.
type epgProgramRepo struct {
	db *gorm.DB
}

// NewEpgProgramRepository creates a new EpgProgramRepository.
func NewEpgProgramRepository(db *gorm.DB) *epgProgramRepo {
	return &epgProgramRepo{db: db}
}

// Create creates a new EPG program.
func (r *epgProgramRepo) Create(ctx context.Context, program *models.EpgProgram) error {
	if err := r.db.WithContext(ctx).Create(program).Error; err != nil {
		return fmt.Errorf("creating EPG program: %w", err)
	}
	return nil
}

// CreateBatch creates multiple programs in a single batch.
// Uses UPSERT to handle duplicates: when a program with the same (source_id, channel_id, start)
// already exists, it will be updated with the new data instead of causing a constraint error.
// Wraps the upsert in an explicit transaction so SQLite performs a single fsync per batch
// instead of one per row (SkipDefaultTransaction=true means no implicit transaction).
func (r *epgProgramRepo) CreateBatch(ctx context.Context, programs []*models.EpgProgram) error {
	if len(programs) == 0 {
		return nil
	}

	// Use ON CONFLICT DO UPDATE to handle duplicates in XMLTV files.
	// The unique constraint is on (source_id, channel_id, start).
	// When a duplicate is found, update all non-key fields with the new values.
	// Retries on transient SQLite BUSY/LOCKED errors with exponential backoff.
	return database.WithRetry(ctx, database.DefaultRetryConfig, nil, "CreateBatch", func() error {
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "source_id"}, {Name: "channel_id"}, {Name: "start"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"stop", "title", "sub_title", "description", "category",
					"icon", "episode_num", "rating", "language", "credits",
					"is_new", "is_premiere", "updated_at",
				}),
			}).Create(programs).Error; err != nil {
				return fmt.Errorf("creating EPG program batch: %w", err)
			}
			return nil
		})
	})
}

// CreateInBatches creates multiple programs in smaller batches for memory efficiency.
// Uses UPSERT to handle duplicates: when a program with the same (source_id, channel_id, start)
// already exists, it will be updated with the new data instead of causing a constraint error.
func (r *epgProgramRepo) CreateInBatches(ctx context.Context, programs []*models.EpgProgram, batchSize int) error {
	if len(programs) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1000
	}

	// Use ON CONFLICT DO UPDATE to handle duplicates in XMLTV files.
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_id"}, {Name: "channel_id"}, {Name: "start"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"stop", "title", "sub_title", "description", "category",
			"icon", "episode_num", "rating", "language", "credits",
			"is_new", "is_premiere", "updated_at",
		}),
	}).CreateInBatches(programs, batchSize).Error; err != nil {
		return fmt.Errorf("creating EPG programs in batches: %w", err)
	}
	return nil
}

// GetByID retrieves an EPG program by ID.
func (r *epgProgramRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgProgram, error) {
	var program models.EpgProgram
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&program).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting EPG program by ID: %w", err)
	}
	return &program, nil
}

// GetBySourceID retrieves all programs for a source using a callback for streaming.
// Uses GORM's Rows() iterator for reliable row-by-row processing without batch issues.
func (r *epgProgramRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.EpgProgram) error) error {
	rows, err := r.db.WithContext(ctx).
		Model(&models.EpgProgram{}).
		Where("source_id = ?", sourceID).
		Order("id ASC").
		Rows()
	if err != nil {
		return fmt.Errorf("querying programs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var program models.EpgProgram
		if err := r.db.ScanRows(rows, &program); err != nil {
			return fmt.Errorf("scanning program row: %w", err)
		}
		if err := callback(&program); err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating programs: %w", err)
	}

	return nil
}

// GetByChannelID retrieves programs for a channel within a time range.
func (r *epgProgramRepo) GetByChannelID(ctx context.Context, channelID string, start, end time.Time) ([]*models.EpgProgram, error) {
	var programs []*models.EpgProgram

	// Get programs that overlap with the time range
	// A program overlaps if it starts before the end AND stops after the start
	if err := r.db.WithContext(ctx).
		Where("channel_id = ? AND start < ? AND stop > ?", channelID, end, start).
		Order("start ASC").
		Find(&programs).Error; err != nil {
		return nil, fmt.Errorf("getting EPG programs by channel: %w", err)
	}

	return programs, nil
}

// GetCurrentByChannelID retrieves the currently airing program for a channel.
func (r *epgProgramRepo) GetCurrentByChannelID(ctx context.Context, channelID string) (*models.EpgProgram, error) {
	now := time.Now()
	var program models.EpgProgram

	if err := r.db.WithContext(ctx).
		Where("channel_id = ? AND start <= ? AND stop > ?", channelID, now, now).
		First(&program).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting current EPG program: %w", err)
	}

	return &program, nil
}

// GetByChannelIDWithLimit retrieves upcoming programs for a channel with a limit.
func (r *epgProgramRepo) GetByChannelIDWithLimit(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error) {
	now := time.Now()
	var programs []*models.EpgProgram

	if err := r.db.WithContext(ctx).
		Where("channel_id = ? AND stop > ?", channelID, now).
		Order("start ASC").
		Limit(limit).
		Find(&programs).Error; err != nil {
		return nil, fmt.Errorf("getting EPG programs by channel: %w", err)
	}

	return programs, nil
}

// Delete hard-deletes an EPG program by ID.
// Uses Unscoped() for permanent deletion for consistency with DeleteBySourceID.
func (r *epgProgramRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&models.EpgProgram{}).Error; err != nil {
		return fmt.Errorf("deleting EPG program: %w", err)
	}
	return nil
}

// DeleteBySourceID hard-deletes all programs for a source.
// Uses Unscoped() for permanent deletion since EPG programs are fully replaced on each ingestion.
// Deletes in batches by channel to reduce lock duration and prevent SQLite BUSY errors.
func (r *epgProgramRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	// Get distinct channel IDs for this source to batch delete by channel
	var channelIDs []string
	if err := r.db.WithContext(ctx).
		Model(&models.EpgProgram{}).
		Where("source_id = ?", sourceID).
		Distinct("channel_id").
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return fmt.Errorf("fetching channel IDs for deletion: %w", err)
	}

	if len(channelIDs) == 0 {
		// No programs to delete
		return nil
	}

	// Delete in batches of channels (e.g., 50 channels at a time)
	// This balances transaction size with number of transactions
	const channelBatchSize = 50
	totalDeleted := int64(0)

	for i := 0; i < len(channelIDs); i += channelBatchSize {
		end := i + channelBatchSize
		if end > len(channelIDs) {
			end = len(channelIDs)
		}
		channelBatch := channelIDs[i:end]

		// Delete all programs for this batch of channels
		result := r.db.WithContext(ctx).Unscoped().
			Where("source_id = ? AND channel_id IN ?", sourceID, channelBatch).
			Delete(&models.EpgProgram{})

		if result.Error != nil {
			return fmt.Errorf("deleting EPG programs (batch %d-%d of %d channels): %w",
				i, end, len(channelIDs), result.Error)
		}
		totalDeleted += result.RowsAffected

		// Check for context cancellation between batches
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return nil
}

// DeleteStaleBySourceID deletes programs for a source that haven't been updated since the given time.
// This implements "mark and sweep" cleanup: upsert updates the updated_at timestamp, so programs
// not present in the new ingestion data will have an older updated_at and will be deleted.
// Returns the number of programs deleted.
// Retries on transient SQLite BUSY/LOCKED errors with exponential backoff.
func (r *epgProgramRepo) DeleteStaleBySourceID(ctx context.Context, sourceID models.ULID, olderThan time.Time) (int64, error) {
	var rowsAffected int64
	err := database.WithRetry(ctx, database.DefaultRetryConfig, nil, "DeleteStaleBySourceID", func() error {
		result := r.db.WithContext(ctx).Unscoped().
			Where("source_id = ? AND updated_at < ?", sourceID, olderThan).
			Delete(&models.EpgProgram{})
		if result.Error != nil {
			return fmt.Errorf("deleting stale EPG programs: %w", result.Error)
		}
		rowsAffected = result.RowsAffected
		return nil
	})
	return rowsAffected, err
}

// DeleteExpired hard-deletes programs that ended before the given time.
// Uses Unscoped() for permanent deletion since expired programs have no value.
func (r *epgProgramRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Unscoped().Where("stop < ?", before).Delete(&models.EpgProgram{})
	if result.Error != nil {
		return 0, fmt.Errorf("deleting expired EPG programs: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// DeleteOld deletes programs older than 24 hours (default retention period).
func (r *epgProgramRepo) DeleteOld(ctx context.Context) (int64, error) {
	// Delete programs that ended more than 24 hours ago
	before := time.Now().Add(-24 * time.Hour)
	return r.DeleteExpired(ctx, before)
}

// CountBySourceID returns the number of programs for a source.
func (r *epgProgramRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.EpgProgram{}).Where("source_id = ?", sourceID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting EPG programs: %w", err)
	}
	return count, nil
}

// GetDistinctChannels returns all unique channel IDs.
func (r *epgProgramRepo) GetDistinctChannels(ctx context.Context) ([]string, error) {
	var channels []string
	if err := r.db.WithContext(ctx).
		Model(&models.EpgProgram{}).
		Distinct("channel_id").
		Order("channel_id ASC").
		Pluck("channel_id", &channels).Error; err != nil {
		return nil, fmt.Errorf("getting distinct channels: %w", err)
	}
	return channels, nil
}

// Ensure epgProgramRepo implements EpgProgramRepository at compile time.
var _ EpgProgramRepository = (*epgProgramRepo)(nil)
