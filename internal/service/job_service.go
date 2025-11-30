package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/scheduler"
)

// JobService provides high-level job management operations.
type JobService struct {
	jobRepo          repository.JobRepository
	streamSourceRepo repository.StreamSourceRepository
	epgSourceRepo    repository.EpgSourceRepository
	proxyRepo        repository.StreamProxyRepository
	scheduler        *scheduler.Scheduler
	runner           *scheduler.Runner
	logger           *slog.Logger
}

// NewJobService creates a new JobService.
func NewJobService(
	jobRepo repository.JobRepository,
	streamSourceRepo repository.StreamSourceRepository,
	epgSourceRepo repository.EpgSourceRepository,
	proxyRepo repository.StreamProxyRepository,
) *JobService {
	return &JobService{
		jobRepo:          jobRepo,
		streamSourceRepo: streamSourceRepo,
		epgSourceRepo:    epgSourceRepo,
		proxyRepo:        proxyRepo,
		logger:           slog.Default(),
	}
}

// WithLogger sets a custom logger.
func (s *JobService) WithLogger(logger *slog.Logger) *JobService {
	s.logger = logger
	return s
}

// WithScheduler sets the scheduler instance.
func (s *JobService) WithScheduler(scheduler *scheduler.Scheduler) *JobService {
	s.scheduler = scheduler
	return s
}

// WithRunner sets the runner instance.
func (s *JobService) WithRunner(runner *scheduler.Runner) *JobService {
	s.runner = runner
	return s
}

// GetByID retrieves a job by ID.
func (s *JobService) GetByID(ctx context.Context, id models.ULID) (*models.Job, error) {
	return s.jobRepo.GetByID(ctx, id)
}

// GetAll retrieves all jobs.
func (s *JobService) GetAll(ctx context.Context) ([]*models.Job, error) {
	return s.jobRepo.GetAll(ctx)
}

// GetPending retrieves all pending jobs.
func (s *JobService) GetPending(ctx context.Context) ([]*models.Job, error) {
	return s.jobRepo.GetPending(ctx)
}

// GetRunning retrieves all running jobs.
func (s *JobService) GetRunning(ctx context.Context) ([]*models.Job, error) {
	return s.jobRepo.GetRunning(ctx)
}

// GetByType retrieves jobs by type.
func (s *JobService) GetByType(ctx context.Context, jobType models.JobType) ([]*models.Job, error) {
	return s.jobRepo.GetByType(ctx, jobType)
}

// GetByTargetID retrieves jobs for a specific target.
func (s *JobService) GetByTargetID(ctx context.Context, targetID models.ULID) ([]*models.Job, error) {
	return s.jobRepo.GetByTargetID(ctx, targetID)
}

// GetHistory retrieves job history with pagination.
func (s *JobService) GetHistory(ctx context.Context, jobType *models.JobType, offset, limit int) ([]*models.JobHistory, int64, error) {
	return s.jobRepo.GetHistory(ctx, jobType, offset, limit)
}

// TriggerStreamIngestion triggers an immediate stream source ingestion.
func (s *JobService) TriggerStreamIngestion(ctx context.Context, sourceID models.ULID) (*models.Job, error) {
	source, err := s.streamSourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("source not found: %s", sourceID)
	}

	if s.scheduler == nil {
		return nil, fmt.Errorf("scheduler not configured")
	}

	job, err := s.scheduler.ScheduleImmediate(ctx, models.JobTypeStreamIngestion, sourceID, source.Name)
	if err != nil {
		return nil, fmt.Errorf("scheduling job: %w", err)
	}

	s.logger.Info("triggered stream ingestion",
		slog.String("source_id", sourceID.String()),
		slog.String("source_name", source.Name),
		slog.String("job_id", job.ID.String()))

	return job, nil
}

// TriggerEpgIngestion triggers an immediate EPG source ingestion.
func (s *JobService) TriggerEpgIngestion(ctx context.Context, sourceID models.ULID) (*models.Job, error) {
	source, err := s.epgSourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting EPG source: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("EPG source not found: %s", sourceID)
	}

	if s.scheduler == nil {
		return nil, fmt.Errorf("scheduler not configured")
	}

	job, err := s.scheduler.ScheduleImmediate(ctx, models.JobTypeEpgIngestion, sourceID, source.Name)
	if err != nil {
		return nil, fmt.Errorf("scheduling job: %w", err)
	}

	s.logger.Info("triggered EPG ingestion",
		slog.String("source_id", sourceID.String()),
		slog.String("source_name", source.Name),
		slog.String("job_id", job.ID.String()))

	return job, nil
}

// TriggerProxyGeneration triggers an immediate proxy generation.
func (s *JobService) TriggerProxyGeneration(ctx context.Context, proxyID models.ULID) (*models.Job, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil {
		return nil, fmt.Errorf("getting proxy: %w", err)
	}
	if proxy == nil {
		return nil, fmt.Errorf("proxy not found: %s", proxyID)
	}

	if s.scheduler == nil {
		return nil, fmt.Errorf("scheduler not configured")
	}

	job, err := s.scheduler.ScheduleImmediate(ctx, models.JobTypeProxyGeneration, proxyID, proxy.Name)
	if err != nil {
		return nil, fmt.Errorf("scheduling job: %w", err)
	}

	s.logger.Info("triggered proxy generation",
		slog.String("proxy_id", proxyID.String()),
		slog.String("proxy_name", proxy.Name),
		slog.String("job_id", job.ID.String()))

	return job, nil
}

// CancelJob cancels a pending or running job.
func (s *JobService) CancelJob(ctx context.Context, id models.ULID) error {
	job, err := s.jobRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting job: %w", err)
	}
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	if job.IsFinished() {
		return fmt.Errorf("cannot cancel finished job")
	}

	job.MarkCancelled()
	if err := s.jobRepo.Update(ctx, job); err != nil {
		return fmt.Errorf("updating job: %w", err)
	}

	s.logger.Info("cancelled job",
		slog.String("job_id", id.String()),
		slog.String("type", string(job.Type)))

	return nil
}

// DeleteJob deletes a completed job.
func (s *JobService) DeleteJob(ctx context.Context, id models.ULID) error {
	job, err := s.jobRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting job: %w", err)
	}
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	if !job.IsFinished() {
		return fmt.Errorf("cannot delete unfinished job")
	}

	if err := s.jobRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting job: %w", err)
	}

	s.logger.Info("deleted job", slog.String("job_id", id.String()))
	return nil
}

// Cleanup deletes old completed jobs and history.
func (s *JobService) Cleanup(ctx context.Context, olderThan time.Duration) (jobsDeleted, historyDeleted int64, err error) {
	cutoff := time.Now().Add(-olderThan)

	jobsDeleted, err = s.jobRepo.DeleteCompleted(ctx, cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("deleting completed jobs: %w", err)
	}

	historyDeleted, err = s.jobRepo.DeleteHistory(ctx, cutoff)
	if err != nil {
		return jobsDeleted, 0, fmt.Errorf("deleting history: %w", err)
	}

	if jobsDeleted > 0 || historyDeleted > 0 {
		s.logger.Info("cleaned up old jobs",
			slog.Int64("jobs_deleted", jobsDeleted),
			slog.Int64("history_deleted", historyDeleted),
			slog.Duration("older_than", olderThan))
	}

	return jobsDeleted, historyDeleted, nil
}

// ValidateCron validates a cron expression.
func (s *JobService) ValidateCron(expr string) error {
	if s.scheduler == nil {
		return fmt.Errorf("scheduler not configured")
	}
	return s.scheduler.ValidateCron(expr)
}

// GetNextRun returns the next run time for a cron expression.
func (s *JobService) GetNextRun(expr string) (time.Time, error) {
	if s.scheduler == nil {
		return time.Time{}, fmt.Errorf("scheduler not configured")
	}
	return s.scheduler.ParseCron(expr)
}

// GetRunnerStatus returns the current runner status.
func (s *JobService) GetRunnerStatus() (*scheduler.RunnerStatus, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("runner not configured")
	}
	status := s.runner.GetStatus()
	return &status, nil
}

// JobStats represents job statistics.
type JobStats struct {
	PendingCount   int64            `json:"pending_count"`
	RunningCount   int64            `json:"running_count"`
	CompletedCount int64            `json:"completed_count"`
	FailedCount    int64            `json:"failed_count"`
	ByType         map[string]int64 `json:"by_type"`
}

// GetStats returns job statistics.
func (s *JobService) GetStats(ctx context.Context) (*JobStats, error) {
	stats := &JobStats{
		ByType: make(map[string]int64),
	}

	// Get counts by status
	pending, err := s.jobRepo.GetByStatus(ctx, models.JobStatusPending)
	if err != nil {
		return nil, fmt.Errorf("getting pending jobs: %w", err)
	}
	stats.PendingCount = int64(len(pending))

	scheduled, err := s.jobRepo.GetByStatus(ctx, models.JobStatusScheduled)
	if err != nil {
		return nil, fmt.Errorf("getting scheduled jobs: %w", err)
	}
	stats.PendingCount += int64(len(scheduled))

	running, err := s.jobRepo.GetByStatus(ctx, models.JobStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("getting running jobs: %w", err)
	}
	stats.RunningCount = int64(len(running))

	completed, err := s.jobRepo.GetByStatus(ctx, models.JobStatusCompleted)
	if err != nil {
		return nil, fmt.Errorf("getting completed jobs: %w", err)
	}
	stats.CompletedCount = int64(len(completed))

	failed, err := s.jobRepo.GetByStatus(ctx, models.JobStatusFailed)
	if err != nil {
		return nil, fmt.Errorf("getting failed jobs: %w", err)
	}
	stats.FailedCount = int64(len(failed))

	// Get counts by type
	allJobs, err := s.jobRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting all jobs: %w", err)
	}

	for _, job := range allJobs {
		stats.ByType[string(job.Type)]++
	}

	return stats, nil
}
