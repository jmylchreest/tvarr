// Package scheduler provides job scheduling and execution for tvarr.
// It supports cron-based recurring jobs and one-off immediate jobs.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// Scheduler manages job scheduling using cron expressions.
// It periodically syncs schedules from sources/proxies and ensures
// jobs are created at the appropriate times.
type Scheduler struct {
	mu sync.RWMutex

	jobRepo          repository.JobRepository
	streamSourceRepo repository.StreamSourceRepository
	epgSourceRepo    repository.EpgSourceRepository
	proxyRepo        repository.StreamProxyRepository

	logger *slog.Logger

	// cron parser for validating/parsing cron expressions
	parser cron.Parser

	// Running state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Sync interval for checking schedules
	syncInterval time.Duration

	// Job deduplication grace period
	dedupeGracePeriod time.Duration
}

// SchedulerConfig holds configuration for the scheduler.
type SchedulerConfig struct {
	// SyncInterval is how often to check for jobs that need scheduling.
	// Default: 1 minute
	SyncInterval time.Duration

	// DedupeGracePeriod is the time window for job deduplication.
	// If a job for the same target was scheduled within this period, skip creating a new one.
	// Default: 5 minutes
	DedupeGracePeriod time.Duration
}

// DefaultSchedulerConfig returns the default scheduler configuration.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		SyncInterval:      time.Minute,
		DedupeGracePeriod: 5 * time.Minute,
	}
}

// NewScheduler creates a new scheduler.
func NewScheduler(
	jobRepo repository.JobRepository,
	streamSourceRepo repository.StreamSourceRepository,
	epgSourceRepo repository.EpgSourceRepository,
	proxyRepo repository.StreamProxyRepository,
) *Scheduler {
	config := DefaultSchedulerConfig()
	return &Scheduler{
		jobRepo:           jobRepo,
		streamSourceRepo:  streamSourceRepo,
		epgSourceRepo:     epgSourceRepo,
		proxyRepo:         proxyRepo,
		logger:            slog.Default(),
		parser:            cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		syncInterval:      config.SyncInterval,
		dedupeGracePeriod: config.DedupeGracePeriod,
	}
}

// WithLogger sets a custom logger.
func (s *Scheduler) WithLogger(logger *slog.Logger) *Scheduler {
	s.logger = logger
	return s
}

// WithConfig applies configuration to the scheduler.
func (s *Scheduler) WithConfig(config SchedulerConfig) *Scheduler {
	if config.SyncInterval > 0 {
		s.syncInterval = config.SyncInterval
	}
	if config.DedupeGracePeriod > 0 {
		s.dedupeGracePeriod = config.DedupeGracePeriod
	}
	return s
}

// Start begins the scheduler's background sync loop.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ctx != nil {
		return fmt.Errorf("scheduler already started")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go s.syncLoop()

	s.logger.Info("scheduler started",
		slog.Duration("sync_interval", s.syncInterval),
		slog.Duration("dedupe_grace_period", s.dedupeGracePeriod))

	return nil
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	s.wg.Wait()

	s.mu.Lock()
	s.ctx = nil
	s.cancel = nil
	s.mu.Unlock()

	s.logger.Info("scheduler stopped")
}

// syncLoop periodically syncs schedules and creates due jobs.
func (s *Scheduler) syncLoop() {
	defer s.wg.Done()

	// Run immediately on start
	s.syncSchedules(s.ctx)

	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.syncSchedules(s.ctx)
		}
	}
}

// syncSchedules checks all sources and proxies for due schedules.
func (s *Scheduler) syncSchedules(ctx context.Context) {
	s.syncStreamSources(ctx)
	s.syncEpgSources(ctx)
	s.syncProxies(ctx)
}

// syncStreamSources checks stream sources for due schedules.
func (s *Scheduler) syncStreamSources(ctx context.Context) {
	sources, err := s.streamSourceRepo.GetEnabled(ctx)
	if err != nil {
		s.logger.Error("failed to get stream sources for scheduling", slog.Any("error", err))
		return
	}

	for _, source := range sources {
		if source.CronSchedule == "" {
			continue
		}

		if s.isDue(source.CronSchedule) {
			if err := s.createJobIfNotDuplicate(ctx, models.JobTypeStreamIngestion, source.ID, source.Name, source.CronSchedule); err != nil {
				s.logger.Error("failed to create stream ingestion job",
					slog.String("source", source.Name),
					slog.Any("error", err))
			}
		}
	}
}

// syncEpgSources checks EPG sources for due schedules.
func (s *Scheduler) syncEpgSources(ctx context.Context) {
	sources, err := s.epgSourceRepo.GetEnabled(ctx)
	if err != nil {
		s.logger.Error("failed to get EPG sources for scheduling", slog.Any("error", err))
		return
	}

	for _, source := range sources {
		if source.CronSchedule == "" {
			continue
		}

		if s.isDue(source.CronSchedule) {
			if err := s.createJobIfNotDuplicate(ctx, models.JobTypeEpgIngestion, source.ID, source.Name, source.CronSchedule); err != nil {
				s.logger.Error("failed to create EPG ingestion job",
					slog.String("source", source.Name),
					slog.Any("error", err))
			}
		}
	}
}

// syncProxies checks proxies for due schedules.
func (s *Scheduler) syncProxies(ctx context.Context) {
	proxies, err := s.proxyRepo.GetActive(ctx)
	if err != nil {
		s.logger.Error("failed to get proxies for scheduling", slog.Any("error", err))
		return
	}

	for _, proxy := range proxies {
		if proxy.CronSchedule == "" {
			continue
		}

		if s.isDue(proxy.CronSchedule) {
			if err := s.createJobIfNotDuplicate(ctx, models.JobTypeProxyGeneration, proxy.ID, proxy.Name, proxy.CronSchedule); err != nil {
				s.logger.Error("failed to create proxy generation job",
					slog.String("proxy", proxy.Name),
					slog.Any("error", err))
			}
		}
	}
}

// isDue checks if a cron schedule is due for execution.
// A schedule is due if the next run time is within the sync interval.
func (s *Scheduler) isDue(cronExpr string) bool {
	schedule, err := s.parser.Parse(cronExpr)
	if err != nil {
		s.logger.Warn("invalid cron expression", slog.String("cron", cronExpr), slog.Any("error", err))
		return false
	}

	now := time.Now()
	next := schedule.Next(now.Add(-s.syncInterval))

	// Check if next run is within the current sync window
	return next.Before(now) || next.Equal(now) || next.Before(now.Add(s.syncInterval))
}

// createJobIfNotDuplicate creates a job if no duplicate pending job exists.
func (s *Scheduler) createJobIfNotDuplicate(ctx context.Context, jobType models.JobType, targetID models.ULID, targetName, cronSchedule string) error {
	// Check for existing pending/running job
	existing, err := s.jobRepo.FindDuplicatePending(ctx, jobType, targetID)
	if err != nil {
		return fmt.Errorf("checking for duplicate job: %w", err)
	}

	if existing != nil {
		s.logger.Debug("skipping duplicate job",
			slog.String("type", string(jobType)),
			slog.String("target", targetName))
		return nil
	}

	// Create new job
	job := &models.Job{
		Type:         jobType,
		TargetID:     targetID,
		TargetName:   targetName,
		Status:       models.JobStatusPending,
		CronSchedule: cronSchedule,
	}

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return fmt.Errorf("creating job: %w", err)
	}

	s.logger.Info("created scheduled job",
		slog.String("type", string(jobType)),
		slog.String("target", targetName),
		slog.String("job_id", job.ID.String()))

	return nil
}

// ScheduleImmediate creates an immediate (one-off) job for the given target.
// Returns the existing job if a duplicate is pending.
func (s *Scheduler) ScheduleImmediate(ctx context.Context, jobType models.JobType, targetID models.ULID, targetName string) (*models.Job, error) {
	// Check for existing pending/running job
	existing, err := s.jobRepo.FindDuplicatePending(ctx, jobType, targetID)
	if err != nil {
		return nil, fmt.Errorf("checking for duplicate job: %w", err)
	}

	if existing != nil {
		s.logger.Debug("returning existing pending job",
			slog.String("type", string(jobType)),
			slog.String("target", targetName),
			slog.String("job_id", existing.ID.String()))
		return existing, nil
	}

	// Create new immediate job
	job := &models.Job{
		Type:       jobType,
		TargetID:   targetID,
		TargetName: targetName,
		Status:     models.JobStatusPending,
	}

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("creating immediate job: %w", err)
	}

	s.logger.Info("created immediate job",
		slog.String("type", string(jobType)),
		slog.String("target", targetName),
		slog.String("job_id", job.ID.String()))

	return job, nil
}

// ParseCron validates a cron expression and returns the next run time.
func (s *Scheduler) ParseCron(expr string) (time.Time, error) {
	schedule, err := s.parser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return schedule.Next(time.Now()), nil
}

// ValidateCron validates a cron expression.
func (s *Scheduler) ValidateCron(expr string) error {
	_, err := s.parser.Parse(expr)
	return err
}
