package models

import (
	"time"

	"gorm.io/gorm"
)

// JobType represents the type of job to execute.
type JobType string

const (
	// JobTypeStreamIngestion represents a stream source ingestion job.
	JobTypeStreamIngestion JobType = "stream_ingestion"
	// JobTypeEpgIngestion represents an EPG source ingestion job.
	JobTypeEpgIngestion JobType = "epg_ingestion"
	// JobTypeProxyGeneration represents a proxy generation job.
	JobTypeProxyGeneration JobType = "proxy_generation"
	// JobTypeLogoCleanup represents a logo cache cleanup job.
	JobTypeLogoCleanup JobType = "logo_cleanup"
)

// JobStatus represents the current status of a job.
type JobStatus string

const (
	// JobStatusPending indicates the job is waiting to be executed.
	JobStatusPending JobStatus = "pending"
	// JobStatusScheduled indicates the job is scheduled for future execution.
	JobStatusScheduled JobStatus = "scheduled"
	// JobStatusRunning indicates the job is currently executing.
	JobStatusRunning JobStatus = "running"
	// JobStatusCompleted indicates the job completed successfully.
	JobStatusCompleted JobStatus = "completed"
	// JobStatusFailed indicates the job failed.
	JobStatusFailed JobStatus = "failed"
	// JobStatusCancelled indicates the job was cancelled.
	JobStatusCancelled JobStatus = "cancelled"
)

// Job represents a scheduled or immediate task execution record.
type Job struct {
	BaseModel

	// Type indicates what kind of job this is.
	Type JobType `gorm:"not null;size:50;index" json:"type"`

	// TargetID is the ID of the entity this job operates on (source ID, proxy ID, etc.).
	// This field is used to deduplicate concurrent job requests for the same target.
	TargetID ULID `gorm:"type:varchar(26);index" json:"target_id,omitempty"`

	// TargetName is a human-readable name for the target (for display purposes).
	TargetName string `gorm:"size:255" json:"target_name,omitempty"`

	// Status indicates the current status of the job.
	Status JobStatus `gorm:"not null;default:'pending';size:20;index" json:"status"`

	// CronSchedule for recurring jobs (optional).
	// Uses standard cron format: "0 */6 * * *" for every 6 hours.
	// Empty string indicates a one-off job.
	CronSchedule string `gorm:"size:100" json:"cron_schedule,omitempty"`

	// NextRunAt is the timestamp when the job should next execute.
	// For one-off jobs, this is the scheduled execution time.
	// For recurring jobs, this is recalculated after each execution.
	NextRunAt *Time `gorm:"index" json:"next_run_at,omitempty"`

	// StartedAt is the timestamp when the job started executing.
	StartedAt *Time `json:"started_at,omitempty"`

	// CompletedAt is the timestamp when the job completed (successfully or with error).
	CompletedAt *Time `json:"completed_at,omitempty"`

	// DurationMs is the execution duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`

	// AttemptCount is the number of times this job has been attempted.
	AttemptCount int `gorm:"default:0" json:"attempt_count"`

	// MaxAttempts is the maximum number of retry attempts (0 = no retries).
	MaxAttempts int `gorm:"default:3" json:"max_attempts"`

	// BackoffSeconds is the initial backoff duration in seconds for retries.
	// Each retry doubles the backoff up to a maximum.
	BackoffSeconds int `gorm:"default:60" json:"backoff_seconds"`

	// LastError contains the error message from the last failed attempt.
	LastError string `gorm:"size:4096" json:"last_error,omitempty"`

	// Result contains optional result data (e.g., counts, metrics).
	Result string `gorm:"size:4096" json:"result,omitempty"`

	// Priority determines execution order (higher = more important).
	// Jobs with higher priority are executed first.
	Priority int `gorm:"default:0;index" json:"priority"`

	// LockedBy is the worker ID that has locked this job for execution.
	// Used to prevent concurrent execution of the same job.
	LockedBy string `gorm:"size:100;index" json:"locked_by,omitempty"`

	// LockedAt is the timestamp when the job was locked.
	LockedAt *Time `json:"locked_at,omitempty"`
}

// TableName returns the table name for Job.
func (Job) TableName() string {
	return "jobs"
}

// IsRecurring returns true if this is a recurring scheduled job.
func (j *Job) IsRecurring() bool {
	return j.CronSchedule != ""
}

// IsOneOff returns true if this is a one-off immediate job.
func (j *Job) IsOneOff() bool {
	return j.CronSchedule == ""
}

// IsPending returns true if the job is pending execution.
func (j *Job) IsPending() bool {
	return j.Status == JobStatusPending || j.Status == JobStatusScheduled
}

// IsRunning returns true if the job is currently executing.
func (j *Job) IsRunning() bool {
	return j.Status == JobStatusRunning
}

// IsFinished returns true if the job has completed (successfully or not).
func (j *Job) IsFinished() bool {
	return j.Status == JobStatusCompleted || j.Status == JobStatusFailed || j.Status == JobStatusCancelled
}

// CanRetry returns true if the job can be retried.
func (j *Job) CanRetry() bool {
	return j.Status == JobStatusFailed && j.AttemptCount < j.MaxAttempts
}

// MarkRunning marks the job as running.
func (j *Job) MarkRunning(workerID string) {
	j.Status = JobStatusRunning
	now := Now()
	j.StartedAt = &now
	j.LockedBy = workerID
	j.LockedAt = &now
	j.AttemptCount++
	j.LastError = ""
}

// MarkCompleted marks the job as completed successfully.
func (j *Job) MarkCompleted(result string) {
	j.Status = JobStatusCompleted
	now := Now()
	j.CompletedAt = &now
	j.Result = result
	j.LastError = ""

	if j.StartedAt != nil {
		j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
	}

	j.LockedBy = ""
	j.LockedAt = nil
}

// MarkFailed marks the job as failed with an error message.
func (j *Job) MarkFailed(err error) {
	j.Status = JobStatusFailed
	now := Now()
	j.CompletedAt = &now

	if err != nil {
		j.LastError = err.Error()
	}

	if j.StartedAt != nil {
		j.DurationMs = now.Sub(*j.StartedAt).Milliseconds()
	}

	j.LockedBy = ""
	j.LockedAt = nil
}

// MarkCancelled marks the job as cancelled.
func (j *Job) MarkCancelled() {
	j.Status = JobStatusCancelled
	now := Now()
	j.CompletedAt = &now
	j.LockedBy = ""
	j.LockedAt = nil
}

// CalculateNextBackoff returns the backoff duration for the next retry.
// Uses exponential backoff: base * 2^(attemptCount-1), capped at 1 hour.
func (j *Job) CalculateNextBackoff() time.Duration {
	if j.BackoffSeconds <= 0 {
		j.BackoffSeconds = 60 // Default 1 minute
	}

	// Calculate exponential backoff
	// Ensure attemptCount is at least 1 to avoid negative shift
	attempts := j.AttemptCount
	if attempts < 1 {
		attempts = 1
	}

	multiplier := 1 << (attempts - 1) // 2^(attempts-1)
	if multiplier < 1 {
		multiplier = 1
	}

	backoffSecs := j.BackoffSeconds * multiplier

	// Cap at 1 hour
	maxBackoff := 3600
	if backoffSecs > maxBackoff {
		backoffSecs = maxBackoff
	}

	return time.Duration(backoffSecs) * time.Second
}

// ScheduleRetry schedules the job for retry with exponential backoff.
func (j *Job) ScheduleRetry() {
	if !j.CanRetry() {
		return
	}

	backoff := j.CalculateNextBackoff()
	nextRun := Now().Add(backoff)
	j.NextRunAt = &nextRun
	j.Status = JobStatusScheduled
	j.LockedBy = ""
	j.LockedAt = nil
}

// Validate performs basic validation on the job.
func (j *Job) Validate() error {
	if j.Type == "" {
		return ErrJobTypeRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the job and generates ULID.
func (j *Job) BeforeCreate(tx *gorm.DB) error {
	if err := j.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return j.Validate()
}

// BeforeUpdate is a GORM hook that validates the job before update.
func (j *Job) BeforeUpdate(tx *gorm.DB) error {
	return j.Validate()
}

// JobHistory stores historical execution records for completed jobs.
// This is separate from the main Job table to keep it lean.
type JobHistory struct {
	BaseModel

	// JobID is the ID of the original job.
	JobID ULID `gorm:"not null;index" json:"job_id"`

	// Type indicates what kind of job this was.
	Type JobType `gorm:"not null;size:50;index" json:"type"`

	// TargetID is the ID of the entity this job operated on.
	TargetID ULID `gorm:"type:varchar(26);index" json:"target_id,omitempty"`

	// TargetName is a human-readable name for the target.
	TargetName string `gorm:"size:255" json:"target_name,omitempty"`

	// Status indicates the final status of the job execution.
	Status JobStatus `gorm:"not null;size:20" json:"status"`

	// StartedAt is the timestamp when the job started executing.
	StartedAt *Time `gorm:"index" json:"started_at,omitempty"`

	// CompletedAt is the timestamp when the job completed.
	CompletedAt *Time `gorm:"index" json:"completed_at,omitempty"`

	// DurationMs is the execution duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`

	// AttemptNumber is which attempt this was (1 = first attempt).
	AttemptNumber int `json:"attempt_number"`

	// Error contains the error message if the job failed.
	Error string `gorm:"size:4096" json:"error,omitempty"`

	// Result contains optional result data.
	Result string `gorm:"size:4096" json:"result,omitempty"`
}

// TableName returns the table name for JobHistory.
func (JobHistory) TableName() string {
	return "job_history"
}

// NewJobFromSource creates a new job for source ingestion.
func NewJobFromSource(source *StreamSource, cronSchedule string) *Job {
	job := &Job{
		Type:         JobTypeStreamIngestion,
		TargetID:     source.ID,
		TargetName:   source.Name,
		CronSchedule: cronSchedule,
	}
	return job
}

// NewJobFromEpgSource creates a new job for EPG source ingestion.
func NewJobFromEpgSource(source *EpgSource, cronSchedule string) *Job {
	job := &Job{
		Type:         JobTypeEpgIngestion,
		TargetID:     source.ID,
		TargetName:   source.Name,
		CronSchedule: cronSchedule,
	}
	return job
}

// NewJobFromProxy creates a new job for proxy generation.
func NewJobFromProxy(proxy *StreamProxy, cronSchedule string) *Job {
	job := &Job{
		Type:         JobTypeProxyGeneration,
		TargetID:     proxy.ID,
		TargetName:   proxy.Name,
		CronSchedule: cronSchedule,
	}
	return job
}
