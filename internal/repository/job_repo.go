package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// jobRepo implements JobRepository using GORM.
type jobRepo struct {
	db     *gorm.DB
	driver string // "sqlite", "postgres", or "mysql"
}

// NewJobRepository creates a new JobRepository.
func NewJobRepository(db *gorm.DB) *jobRepo {
	driver := ""
	if db.Dialector != nil {
		driver = db.Dialector.Name()
	}
	return &jobRepo{db: db, driver: driver}
}

// Create creates a new job.
func (r *jobRepo) Create(ctx context.Context, job *models.Job) error {
	if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
		return fmt.Errorf("creating job: %w", err)
	}
	return nil
}

// GetByID retrieves a job by ID.
func (r *jobRepo) GetByID(ctx context.Context, id models.ULID) (*models.Job, error) {
	var job models.Job
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting job by ID: %w", err)
	}
	return &job, nil
}

// GetAll retrieves all jobs.
func (r *jobRepo) GetAll(ctx context.Context) ([]*models.Job, error) {
	var jobs []*models.Job
	if err := r.db.WithContext(ctx).Order("priority DESC, next_run_at ASC, created_at ASC").Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("getting all jobs: %w", err)
	}
	return jobs, nil
}

// GetPending retrieves all pending/scheduled jobs ready for execution.
// Returns jobs that are pending or scheduled with next_run_at <= now.
func (r *jobRepo) GetPending(ctx context.Context) ([]*models.Job, error) {
	var jobs []*models.Job
	now := time.Now()

	query := r.db.WithContext(ctx).
		Where("(status = ? OR (status = ? AND next_run_at <= ?))", models.JobStatusPending, models.JobStatusScheduled, now).
		Where("locked_by IS NULL OR locked_by = ''").
		Order("priority DESC, next_run_at ASC, created_at ASC")

	if err := query.Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("getting pending jobs: %w", err)
	}
	return jobs, nil
}

// GetByStatus retrieves jobs by status.
func (r *jobRepo) GetByStatus(ctx context.Context, status models.JobStatus) ([]*models.Job, error) {
	var jobs []*models.Job
	if err := r.db.WithContext(ctx).Where("status = ?", status).Order("priority DESC, created_at ASC").Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("getting jobs by status: %w", err)
	}
	return jobs, nil
}

// GetByType retrieves jobs by type.
func (r *jobRepo) GetByType(ctx context.Context, jobType models.JobType) ([]*models.Job, error) {
	var jobs []*models.Job
	if err := r.db.WithContext(ctx).Where("type = ?", jobType).Order("priority DESC, created_at ASC").Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("getting jobs by type: %w", err)
	}
	return jobs, nil
}

// GetByTargetID retrieves jobs for a specific target.
func (r *jobRepo) GetByTargetID(ctx context.Context, targetID models.ULID) ([]*models.Job, error) {
	var jobs []*models.Job
	if err := r.db.WithContext(ctx).Where("target_id = ?", targetID).Order("created_at DESC").Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("getting jobs by target ID: %w", err)
	}
	return jobs, nil
}

// GetRunning retrieves all currently running jobs.
func (r *jobRepo) GetRunning(ctx context.Context) ([]*models.Job, error) {
	var jobs []*models.Job
	if err := r.db.WithContext(ctx).Where("status = ?", models.JobStatusRunning).Order("started_at ASC").Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("getting running jobs: %w", err)
	}
	return jobs, nil
}

// Update updates an existing job.
func (r *jobRepo) Update(ctx context.Context, job *models.Job) error {
	if err := r.db.WithContext(ctx).Save(job).Error; err != nil {
		return fmt.Errorf("updating job: %w", err)
	}
	return nil
}

// Delete deletes a job by ID.
func (r *jobRepo) Delete(ctx context.Context, id models.ULID) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Job{}).Error; err != nil {
		return fmt.Errorf("deleting job: %w", err)
	}
	return nil
}

// DeleteCompleted deletes completed jobs older than the specified time.
func (r *jobRepo) DeleteCompleted(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("status IN (?, ?, ?) AND completed_at < ?",
			models.JobStatusCompleted, models.JobStatusFailed, models.JobStatusCancelled, before).
		Delete(&models.Job{})

	if result.Error != nil {
		return 0, fmt.Errorf("deleting completed jobs: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// AcquireJob atomically acquires a pending job for execution.
// Uses SELECT FOR UPDATE with SKIP LOCKED for PostgreSQL/MySQL.
// Uses optimistic locking with atomic UPDATE for SQLite (which doesn't support row locking).
func (r *jobRepo) AcquireJob(ctx context.Context, workerID string) (*models.Job, error) {
	if r.driver == "sqlite" {
		return r.acquireJobSQLite(ctx, workerID)
	}
	return r.acquireJobWithRowLocking(ctx, workerID)
}

// acquireJobWithRowLocking uses SELECT FOR UPDATE SKIP LOCKED (PostgreSQL/MySQL).
func (r *jobRepo) acquireJobWithRowLocking(ctx context.Context, workerID string) (*models.Job, error) {
	var job models.Job
	now := time.Now()

	// Use a transaction for atomic acquire
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find and lock a pending job
		query := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("(status = ? OR (status = ? AND next_run_at <= ?))", models.JobStatusPending, models.JobStatusScheduled, now).
			Where("locked_by IS NULL OR locked_by = ''").
			Order("priority DESC, next_run_at ASC, created_at ASC").
			Limit(1)

		if err := query.First(&job).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return err // Will cause nil return
			}
			return fmt.Errorf("finding pending job: %w", err)
		}

		// Mark as running with lock
		nowTime := models.Now()
		job.Status = models.JobStatusRunning
		job.StartedAt = &nowTime
		job.LockedBy = workerID
		job.LockedAt = &nowTime
		job.AttemptCount++

		if err := tx.Save(&job).Error; err != nil {
			return fmt.Errorf("acquiring job: %w", err)
		}

		return nil
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // No jobs available
		}
		return nil, err
	}

	return &job, nil
}

// acquireJobSQLite uses a single atomic UPDATE with subquery to claim a job.
// This avoids the SELECT-then-UPDATE race condition by finding and claiming
// in one statement. The first UPDATE to execute claims the job; subsequent
// concurrent attempts will find no matching rows.
func (r *jobRepo) acquireJobSQLite(ctx context.Context, workerID string) (*models.Job, error) {
	now := time.Now()
	nowTime := models.Now()

	// Build subquery to find the best candidate job using GORM's query builder.
	// This maintains schema awareness and lets GORM handle table/column names.
	subQuery := r.db.Model(&models.Job{}).
		Select("id").
		Where("(status = ? OR (status = ? AND next_run_at <= ?))",
			models.JobStatusPending, models.JobStatusScheduled, now).
		Where("locked_by IS NULL OR locked_by = ''").
		Order("priority DESC, next_run_at ASC, created_at ASC").
		Limit(1)

	// Single atomic UPDATE with subquery - finds and claims job in one statement.
	// This is the SQLite-idiomatic way to handle job claiming without row locking.
	// If two workers execute simultaneously, SQLite's write serialization ensures
	// only one succeeds in updating the row.
	// Use UpdateColumns to bypass model hooks (BeforeUpdate validation).
	result := r.db.WithContext(ctx).
		Model(&models.Job{}).
		Where("id = (?)", subQuery).
		UpdateColumns(map[string]any{
			"status":        models.JobStatusRunning,
			"started_at":    nowTime,
			"locked_by":     workerID,
			"locked_at":     nowTime,
			"attempt_count": gorm.Expr("attempt_count + 1"),
		})

	if result.Error != nil {
		return nil, fmt.Errorf("acquiring job: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// No jobs available
		return nil, nil
	}

	// Fetch the job we just claimed by looking for our worker's most recent lock
	var job models.Job
	err := r.db.WithContext(ctx).
		Where("locked_by = ? AND status = ?", workerID, models.JobStatusRunning).
		Order("locked_at DESC").
		First(&job).Error
	if err != nil {
		return nil, fmt.Errorf("fetching acquired job: %w", err)
	}

	return &job, nil
}

// ReleaseJob releases a job lock.
func (r *jobRepo) ReleaseJob(ctx context.Context, id models.ULID) error {
	// Use UpdateColumns to avoid triggering hooks (BeforeUpdate validation)
	result := r.db.WithContext(ctx).Model(&models.Job{}).Where("id = ?", id).
		UpdateColumns(map[string]any{
			"locked_by": nil,
			"locked_at": nil,
			"status":    models.JobStatusPending,
		})

	if result.Error != nil {
		return fmt.Errorf("releasing job: %w", result.Error)
	}
	return nil
}

// FindDuplicatePending finds an existing pending/scheduled job for the same type and target.
func (r *jobRepo) FindDuplicatePending(ctx context.Context, jobType models.JobType, targetID models.ULID) (*models.Job, error) {
	var job models.Job
	if err := r.db.WithContext(ctx).
		Where("type = ? AND target_id = ? AND status IN (?, ?, ?)",
			jobType, targetID, models.JobStatusPending, models.JobStatusScheduled, models.JobStatusRunning).
		First(&job).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("finding duplicate pending job: %w", err)
	}
	return &job, nil
}

// CreateHistory creates a job history record.
func (r *jobRepo) CreateHistory(ctx context.Context, history *models.JobHistory) error {
	if err := r.db.WithContext(ctx).Create(history).Error; err != nil {
		return fmt.Errorf("creating job history: %w", err)
	}
	return nil
}

// GetHistory retrieves job history with pagination.
func (r *jobRepo) GetHistory(ctx context.Context, jobType *models.JobType, offset, limit int) ([]*models.JobHistory, int64, error) {
	var history []*models.JobHistory
	var total int64

	query := r.db.WithContext(ctx).Model(&models.JobHistory{})
	if jobType != nil {
		query = query.Where("type = ?", *jobType)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("counting job history: %w", err)
	}

	// Get paginated results
	if err := query.Order("completed_at DESC").Offset(offset).Limit(limit).Find(&history).Error; err != nil {
		return nil, 0, fmt.Errorf("getting job history: %w", err)
	}

	return history, total, nil
}

// DeleteHistory deletes history records older than the specified time.
func (r *jobRepo) DeleteHistory(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("completed_at < ?", before).
		Delete(&models.JobHistory{})

	if result.Error != nil {
		return 0, fmt.Errorf("deleting job history: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// Ensure jobRepo implements JobRepository at compile time.
var _ JobRepository = (*jobRepo)(nil)
