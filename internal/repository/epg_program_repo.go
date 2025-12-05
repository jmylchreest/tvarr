package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
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
func (r *epgProgramRepo) CreateBatch(ctx context.Context, programs []*models.EpgProgram) error {
	if len(programs) == 0 {
		return nil
	}

	if err := r.db.WithContext(ctx).Create(programs).Error; err != nil {
		return fmt.Errorf("creating EPG program batch: %w", err)
	}
	return nil
}

// CreateInBatches creates multiple programs in smaller batches for memory efficiency.
func (r *epgProgramRepo) CreateInBatches(ctx context.Context, programs []*models.EpgProgram, batchSize int) error {
	if len(programs) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1000
	}

	if err := r.db.WithContext(ctx).CreateInBatches(programs, batchSize).Error; err != nil {
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
func (r *epgProgramRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("source_id = ?", sourceID).Delete(&models.EpgProgram{}).Error; err != nil {
		return fmt.Errorf("deleting EPG programs by source ID: %w", err)
	}
	return nil
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
