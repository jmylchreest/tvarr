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

// epgProgramDeleteBatch bounds how many programs a single DELETE removes.
//
// Each statement takes SQLite's write lock for its duration, so the batch size
// trades total throughput against how long anything else -- an ingestion, a
// stream lookup -- is blocked behind it.
const epgProgramDeleteBatch = 5000

// DeleteBySourceID hard-deletes all programs for a source.
// Uses Unscoped() for permanent deletion since EPG programs are fully replaced
// on each ingestion.
//
// Deletes in bounded batches keyed on the primary key. The previous
// implementation first ran SELECT DISTINCT channel_id across every program row
// for the source, purely to group the deletes by channel: on a source with over
// a million programs that scan alone outlasted the HTTP request that triggered
// it, and the caller saw a timeout before a single row had been removed.
// Batching on the primary key needs no such pre-pass, and each statement holds
// the write lock only briefly.
func (r *epgProgramRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	var total int64

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		ids := r.db.Model(&models.EpgProgram{}).
			Select("id").
			Where("source_id = ?", sourceID).
			Limit(epgProgramDeleteBatch)

		result := r.db.WithContext(ctx).Unscoped().
			Where("id IN (?)", ids).
			Delete(&models.EpgProgram{})
		if result.Error != nil {
			return fmt.Errorf("deleting EPG programs for source %s after %d rows: %w",
				sourceID, total, result.Error)
		}

		total += result.RowsAffected
		if result.RowsAffected == 0 {
			return nil
		}
	}
}

// DeleteOrphaned removes programs whose source no longer exists, returning how
// many rows were deleted.
//
// Source deletion removes the source row first so the UI responds immediately,
// then sweeps the programs in the background. If the process stops mid-sweep the
// remaining programs have no source, are unreachable through any query, and
// would otherwise sit in the database forever. This reclaims them.
func (r *epgProgramRepo) DeleteOrphaned(ctx context.Context) (int64, error) {
	var total int64

	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		liveSources := r.db.Model(&models.EpgSource{}).Select("id")

		ids := r.db.Model(&models.EpgProgram{}).
			Select("id").
			Where("source_id NOT IN (?)", liveSources).
			Limit(epgProgramDeleteBatch)

		result := r.db.WithContext(ctx).Unscoped().
			Where("id IN (?)", ids).
			Delete(&models.EpgProgram{})
		if result.Error != nil {
			return total, fmt.Errorf("deleting orphaned EPG programs after %d rows: %w", total, result.Error)
		}

		total += result.RowsAffected
		if result.RowsAffected == 0 {
			return total, nil
		}
	}
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
