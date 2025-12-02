package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/repository"
)

// Runner manages a pool of workers that execute jobs.
type Runner struct {
	mu sync.RWMutex

	jobRepo  repository.JobRepository
	executor *Executor
	logger   *slog.Logger

	// Configuration
	workerCount   int
	pollInterval  time.Duration
	lockTimeout   time.Duration
	workerID      string
	jobTimeout    time.Duration
	cleanupAge    time.Duration
	cleanupEnable bool

	// Running state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// RunnerConfig holds configuration for the runner.
type RunnerConfig struct {
	// WorkerCount is the number of concurrent workers.
	// Default: 2
	WorkerCount int

	// PollInterval is how often workers poll for jobs.
	// Default: 5 seconds
	PollInterval time.Duration

	// LockTimeout is the duration after which a locked job is considered stale.
	// Jobs locked longer than this may be reclaimed by other workers.
	// Default: 30 minutes
	LockTimeout time.Duration

	// WorkerID is a unique identifier for this runner instance.
	// Used for job locking.
	// Default: randomly generated
	WorkerID string

	// JobTimeout is the maximum duration for a single job execution.
	// Default: 1 hour
	JobTimeout time.Duration

	// CleanupAge is the age after which completed jobs are deleted.
	// Default: 7 days
	CleanupAge time.Duration

	// CleanupEnable enables automatic cleanup of old jobs.
	// Default: true
	CleanupEnable bool
}

// DefaultRunnerConfig returns the default runner configuration.
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		WorkerCount:   2,
		PollInterval:  5 * time.Second,
		LockTimeout:   30 * time.Minute,
		WorkerID:      fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		JobTimeout:    time.Hour,
		CleanupAge:    7 * 24 * time.Hour,
		CleanupEnable: true,
	}
}

// NewRunner creates a new job runner.
func NewRunner(jobRepo repository.JobRepository, executor *Executor) *Runner {
	config := DefaultRunnerConfig()
	return &Runner{
		jobRepo:       jobRepo,
		executor:      executor,
		logger:        slog.Default(),
		workerCount:   config.WorkerCount,
		pollInterval:  config.PollInterval,
		lockTimeout:   config.LockTimeout,
		workerID:      config.WorkerID,
		jobTimeout:    config.JobTimeout,
		cleanupAge:    config.CleanupAge,
		cleanupEnable: config.CleanupEnable,
	}
}

// WithLogger sets a custom logger.
func (r *Runner) WithLogger(logger *slog.Logger) *Runner {
	r.logger = logger
	return r
}

// WithConfig applies configuration to the runner.
func (r *Runner) WithConfig(config RunnerConfig) *Runner {
	if config.WorkerCount > 0 {
		r.workerCount = config.WorkerCount
	}
	if config.PollInterval > 0 {
		r.pollInterval = config.PollInterval
	}
	if config.LockTimeout > 0 {
		r.lockTimeout = config.LockTimeout
	}
	if config.WorkerID != "" {
		r.workerID = config.WorkerID
	}
	if config.JobTimeout > 0 {
		r.jobTimeout = config.JobTimeout
	}
	if config.CleanupAge > 0 {
		r.cleanupAge = config.CleanupAge
	}
	r.cleanupEnable = config.CleanupEnable
	return r
}

// Start begins the runner with the configured number of workers.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ctx != nil {
		return fmt.Errorf("runner already started")
	}

	r.ctx, r.cancel = context.WithCancel(ctx)

	// Start workers
	for i := 0; i < r.workerCount; i++ {
		workerID := fmt.Sprintf("%s-%d", r.workerID, i)
		r.wg.Add(1)
		go r.worker(workerID)
	}

	// Start cleanup routine
	if r.cleanupEnable {
		r.wg.Add(1)
		go r.cleanup()
	}

	// Start stale job recovery routine
	r.wg.Add(1)
	go r.recoverStaleJobs()

	r.logger.Info("runner started",
		slog.Int("workers", r.workerCount),
		slog.Duration("poll_interval", r.pollInterval),
		slog.String("worker_id", r.workerID))

	return nil
}

// Stop stops the runner and waits for workers to finish.
func (r *Runner) Stop() {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()

	r.wg.Wait()

	r.mu.Lock()
	r.ctx = nil
	r.cancel = nil
	r.mu.Unlock()

	r.logger.Info("runner stopped")
}

// worker is the main worker loop.
func (r *Runner) worker(workerID string) {
	defer r.wg.Done()

	r.logger.Debug("worker started", slog.String("worker_id", workerID))

	for {
		select {
		case <-r.ctx.Done():
			r.logger.Debug("worker stopping", slog.String("worker_id", workerID))
			return
		default:
			// Try to acquire and execute a job
			if err := r.processJob(workerID); err != nil {
				// Only log unexpected errors, not "no jobs available"
				if err != errNoJobs {
					r.logger.Error("error processing job",
						slog.String("worker_id", workerID),
						slog.Any("error", err))
				}

				// Wait before polling again
				select {
				case <-r.ctx.Done():
					return
				case <-time.After(r.pollInterval):
				}
			}
		}
	}
}

var errNoJobs = fmt.Errorf("no jobs available")

// processJob acquires and executes a single job.
func (r *Runner) processJob(workerID string) error {
	// Try to acquire a job
	job, err := r.jobRepo.AcquireJob(r.ctx, workerID)
	if err != nil {
		return fmt.Errorf("acquiring job: %w", err)
	}

	if job == nil {
		return errNoJobs
	}

	r.logger.Debug("acquired job",
		slog.String("worker_id", workerID),
		slog.String("job_id", job.ID.String()),
		slog.String("type", string(job.Type)))

	// Create timeout context for job execution
	jobCtx, cancel := context.WithTimeout(r.ctx, r.jobTimeout)
	defer cancel()

	// Execute the job
	if err := r.executor.Execute(jobCtx, job); err != nil {
		return fmt.Errorf("executing job: %w", err)
	}

	return nil
}

// cleanup periodically removes old completed jobs and history.
func (r *Runner) cleanup() {
	defer r.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.performCleanup()
		}
	}
}

// performCleanup deletes old jobs and history.
func (r *Runner) performCleanup() {
	cutoff := time.Now().Add(-r.cleanupAge)

	// Clean up old completed jobs
	jobsDeleted, err := r.jobRepo.DeleteCompleted(r.ctx, cutoff)
	if err != nil {
		r.logger.Error("failed to clean up old jobs", slog.Any("error", err))
	} else if jobsDeleted > 0 {
		r.logger.Info("cleaned up old jobs", slog.Int64("deleted", jobsDeleted))
	}

	// Clean up old history
	historyDeleted, err := r.jobRepo.DeleteHistory(r.ctx, cutoff)
	if err != nil {
		r.logger.Error("failed to clean up old history", slog.Any("error", err))
	} else if historyDeleted > 0 {
		r.logger.Info("cleaned up old history", slog.Int64("deleted", historyDeleted))
	}
}

// recoverStaleJobs periodically checks for jobs that were locked but never completed.
// This can happen if a worker crashes.
func (r *Runner) recoverStaleJobs() {
	defer r.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.performStaleRecovery()
		}
	}
}

// performStaleRecovery releases jobs that have been locked too long.
func (r *Runner) performStaleRecovery() {
	running, err := r.jobRepo.GetRunning(r.ctx)
	if err != nil {
		r.logger.Error("failed to get running jobs for stale recovery", slog.Any("error", err))
		return
	}

	cutoff := time.Now().Add(-r.lockTimeout)

	for _, job := range running {
		if job.LockedAt != nil && job.LockedAt.Before(cutoff) {
			r.logger.Warn("recovering stale job",
				slog.String("job_id", job.ID.String()),
				slog.String("locked_by", job.LockedBy),
				slog.Time("locked_at", *job.LockedAt))

			// Mark as failed and possibly schedule retry
			job.MarkFailed(fmt.Errorf("job stale: locked since %s", job.LockedAt.Format(time.RFC3339)))
			if job.CanRetry() {
				job.ScheduleRetry()
			}

			if err := r.jobRepo.Update(r.ctx, job); err != nil {
				r.logger.Error("failed to recover stale job",
					slog.String("job_id", job.ID.String()),
					slog.Any("error", err))
			}
		}
	}
}

// GetStatus returns the current runner status.
func (r *Runner) GetStatus() RunnerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	running := r.ctx != nil && r.ctx.Err() == nil

	// Get job counts
	var pendingCount, runningCount int64
	if running {
		pending, _ := r.jobRepo.GetPending(r.ctx)
		pendingCount = int64(len(pending))
		runningJobs, _ := r.jobRepo.GetRunning(r.ctx)
		runningCount = int64(len(runningJobs))
	}

	return RunnerStatus{
		Running:      running,
		WorkerCount:  r.workerCount,
		WorkerID:     r.workerID,
		PendingJobs:  pendingCount,
		RunningJobs:  runningCount,
		PollInterval: r.pollInterval,
	}
}

// RunnerStatus represents the current state of the runner.
type RunnerStatus struct {
	Running      bool          `json:"running"`
	WorkerCount  int           `json:"worker_count"`
	WorkerID     string        `json:"worker_id"`
	PendingJobs  int64         `json:"pending_jobs"`
	RunningJobs  int64         `json:"running_jobs"`
	PollInterval time.Duration `json:"poll_interval"`
}
