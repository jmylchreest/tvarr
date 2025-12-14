// Package scheduler provides job scheduling and execution for tvarr.
// It supports cron-based recurring jobs and one-off immediate jobs.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// InternalJobConfig defines configuration for internal recurring jobs.
type InternalJobConfig struct {
	// JobType is the type of job to create.
	JobType models.JobType
	// TargetName is a human-readable name for the job.
	TargetName string
	// CronSchedule is the cron expression for scheduling. Supports both 6-field
	// (sec min hour dom month dow) and 7-field (sec min hour dom month dow year)
	// formats. The year field is validated but ignored since robfig/cron doesn't
	// support it. Empty string disables the job.
	CronSchedule string
}

// DefaultLogoScanSchedule is the default cron schedule for logo maintenance.
// Runs every 2 hours at minute 0 and second 0.
// Format: sec min hour dom month dow (6-field)
const DefaultLogoScanSchedule = "0 0 */2 * * *"

// NormalizeCronExpression normalizes a cron expression to 6-field format.
// It accepts both 6-field (default) and 7-field (legacy with year) formats.
//
// Supported formats:
//   - 6 fields: sec min hour dom month dow (passed through as-is)
//   - 7 fields: sec min hour dom month dow year (year stripped after validation)
//
// The year field (if present) must be "*" or a valid year/range (e.g., "2024", "2024-2030", "*").
func NormalizeCronExpression(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("empty cron expression")
	}

	// Handle special descriptors like @every, @hourly, etc.
	if strings.HasPrefix(expr, "@") {
		return expr, nil
	}

	fields := strings.Fields(expr)
	switch len(fields) {
	case 6:
		// Already 6 fields, pass through
		return expr, nil
	case 7:
		// Strip the year field (last field)
		yearField := fields[6]
		if !isValidYearField(yearField) {
			return "", fmt.Errorf("invalid year field %q: must be * or a valid year/range", yearField)
		}
		return strings.Join(fields[:6], " "), nil
	default:
		return "", fmt.Errorf("invalid cron expression: expected 6 or 7 fields, got %d", len(fields))
	}
}

// isValidYearField validates a cron year field.
// Accepts: *, specific years (2024), ranges (2024-2030), lists (2024,2025), step values (*/2, 2024/1).
func isValidYearField(field string) bool {
	if field == "*" {
		return true
	}
	// Basic validation - allow digits, commas, hyphens, slashes, and asterisks
	for _, r := range field {
		if !((r >= '0' && r <= '9') || r == ',' || r == '-' || r == '/' || r == '*') {
			return false
		}
	}
	return len(field) > 0
}

// Scheduler manages job scheduling using cron expressions.
// It uses robfig/cron as the timing engine for efficient execution
// and periodically syncs schedules from the database to pick up changes.
type Scheduler struct {
	mu sync.RWMutex

	jobRepo          repository.JobRepository
	streamSourceRepo repository.StreamSourceRepository
	epgSourceRepo    repository.EpgSourceRepository
	proxyRepo        repository.StreamProxyRepository

	logger *slog.Logger

	// cron parser for validating/parsing cron expressions
	// Default: 6 fields (second minute hour dom month dow)
	// Legacy: 7 fields with year are normalized to 6 fields
	parser cron.Parser

	// cronScheduler is the robfig/cron instance that handles timing
	cronScheduler *cron.Cron

	// entryMap tracks cron entry IDs by target key (type:targetID)
	// This allows us to update/remove entries when schedules change
	entryMap map[string]cron.EntryID

	// Running state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Sync interval for checking schedule changes in the database
	syncInterval time.Duration

	// Job deduplication grace period
	dedupeGracePeriod time.Duration

	// Internal jobs configuration
	internalJobs []InternalJobConfig
}

// SchedulerConfig holds configuration for the scheduler.
type SchedulerConfig struct {
	// SyncInterval is how often to sync schedules from the database.
	// Default: 1 minute
	SyncInterval time.Duration

	// DedupeGracePeriod is the time window for job deduplication.
	// If a job for the same target was scheduled within this period, skip creating a new one.
	// Default: 5 minutes
	DedupeGracePeriod time.Duration

	// InternalJobs defines internal recurring jobs to schedule.
	// These are not stored in the database but run on a fixed interval.
	InternalJobs []InternalJobConfig
}

// DefaultSchedulerConfig returns the default scheduler configuration.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		SyncInterval:      time.Minute,
		DedupeGracePeriod: 5 * time.Minute,
		InternalJobs:      []InternalJobConfig{},
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

	// Create a parser that supports 6-field cron expressions (with seconds)
	// Fields: second minute hour day-of-month month day-of-week
	// Note: 7-field expressions (with year) are normalized to 6-field before parsing
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

	// Create the cron scheduler with seconds support
	cronScheduler := cron.New(cron.WithParser(parser), cron.WithChain(
		cron.Recover(cron.DefaultLogger), // Recover from panics in jobs
	))

	return &Scheduler{
		jobRepo:           jobRepo,
		streamSourceRepo:  streamSourceRepo,
		epgSourceRepo:     epgSourceRepo,
		proxyRepo:         proxyRepo,
		logger:            slog.Default(),
		parser:            parser,
		cronScheduler:     cronScheduler,
		entryMap:          make(map[string]cron.EntryID),
		syncInterval:      config.SyncInterval,
		dedupeGracePeriod: config.DedupeGracePeriod,
		internalJobs:      config.InternalJobs,
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
	if len(config.InternalJobs) > 0 {
		s.internalJobs = config.InternalJobs
	}
	return s
}

// Start begins the scheduler's background operations.
// It loads initial schedules from the database and starts the cron scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.ctx != nil {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already started")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// Load initial schedules from database (outside of lock since loadSchedules acquires it)
	if err := s.loadSchedules(s.ctx); err != nil {
		s.logger.Error("failed to load initial schedules", slog.Any("error", err))
		// Continue anyway - we'll retry on next sync
	}

	// Register internal recurring jobs (acquires lock internally)
	s.registerInternalJobs()

	// Start the cron scheduler
	s.cronScheduler.Start()

	// Start the sync loop to pick up schedule changes
	s.wg.Add(1)
	go s.syncLoop()

	s.mu.RLock()
	entryCount := len(s.entryMap)
	s.mu.RUnlock()

	s.logger.Info("scheduler started",
		slog.Duration("sync_interval", s.syncInterval),
		slog.Duration("dedupe_grace_period", s.dedupeGracePeriod),
		slog.Int("initial_entries", entryCount))

	return nil
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}

	// Stop the cron scheduler (waits for running jobs to complete)
	stopCtx := s.cronScheduler.Stop()
	<-stopCtx.Done()

	s.mu.Unlock()

	s.wg.Wait()

	s.mu.Lock()
	s.ctx = nil
	s.cancel = nil
	s.mu.Unlock()

	s.logger.Info("scheduler stopped")
}

// syncLoop periodically syncs schedules from the database.
func (s *Scheduler) syncLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.loadSchedules(s.ctx); err != nil {
				s.logger.Error("failed to sync schedules", slog.Any("error", err))
			}
		}
	}
}

// loadSchedules loads all schedules from the database and updates the cron scheduler.
func (s *Scheduler) loadSchedules(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Track which entries we've seen (to detect removed schedules)
	seenEntries := make(map[string]bool)

	// Load stream source schedules
	if err := s.loadStreamSourceSchedules(ctx, seenEntries); err != nil {
		s.logger.Error("failed to load stream source schedules", slog.Any("error", err))
	}

	// Load EPG source schedules
	if err := s.loadEpgSourceSchedules(ctx, seenEntries); err != nil {
		s.logger.Error("failed to load EPG source schedules", slog.Any("error", err))
	}

	// Load proxy schedules
	if err := s.loadProxySchedules(ctx, seenEntries); err != nil {
		s.logger.Error("failed to load proxy schedules", slog.Any("error", err))
	}

	// Remove entries that no longer exist in the database
	for key, entryID := range s.entryMap {
		if !seenEntries[key] {
			s.cronScheduler.Remove(entryID)
			delete(s.entryMap, key)
			s.logger.Debug("removed schedule", slog.String("key", key))
		}
	}

	return nil
}

// loadStreamSourceSchedules loads stream source schedules.
func (s *Scheduler) loadStreamSourceSchedules(ctx context.Context, seenEntries map[string]bool) error {
	sources, err := s.streamSourceRepo.GetEnabled(ctx)
	if err != nil {
		return fmt.Errorf("getting stream sources: %w", err)
	}

	for _, source := range sources {
		if source.CronSchedule == "" {
			continue
		}

		key := fmt.Sprintf("stream:%s", source.ID.String())
		seenEntries[key] = true

		if err := s.upsertScheduleEntry(key, source.CronSchedule, models.JobTypeStreamIngestion, source.ID, source.Name); err != nil {
			s.logger.Error("failed to upsert stream source schedule",
				slog.String("source", source.Name),
				slog.String("cron", source.CronSchedule),
				slog.Any("error", err))
		}
	}

	return nil
}

// loadEpgSourceSchedules loads EPG source schedules.
func (s *Scheduler) loadEpgSourceSchedules(ctx context.Context, seenEntries map[string]bool) error {
	sources, err := s.epgSourceRepo.GetEnabled(ctx)
	if err != nil {
		return fmt.Errorf("getting EPG sources: %w", err)
	}

	for _, source := range sources {
		if source.CronSchedule == "" {
			continue
		}

		key := fmt.Sprintf("epg:%s", source.ID.String())
		seenEntries[key] = true

		if err := s.upsertScheduleEntry(key, source.CronSchedule, models.JobTypeEpgIngestion, source.ID, source.Name); err != nil {
			s.logger.Error("failed to upsert EPG source schedule",
				slog.String("source", source.Name),
				slog.String("cron", source.CronSchedule),
				slog.Any("error", err))
		}
	}

	return nil
}

// loadProxySchedules loads proxy schedules.
func (s *Scheduler) loadProxySchedules(ctx context.Context, seenEntries map[string]bool) error {
	proxies, err := s.proxyRepo.GetActive(ctx)
	if err != nil {
		return fmt.Errorf("getting proxies: %w", err)
	}

	for _, proxy := range proxies {
		if proxy.CronSchedule == "" {
			continue
		}

		key := fmt.Sprintf("proxy:%s", proxy.ID.String())
		seenEntries[key] = true

		if err := s.upsertScheduleEntry(key, proxy.CronSchedule, models.JobTypeProxyGeneration, proxy.ID, proxy.Name); err != nil {
			s.logger.Error("failed to upsert proxy schedule",
				slog.String("proxy", proxy.Name),
				slog.String("cron", proxy.CronSchedule),
				slog.Any("error", err))
		}
	}

	return nil
}

// upsertScheduleEntry adds or updates a cron entry.
// If an entry with the same key exists but has a different schedule, it's replaced.
// Accepts both 6-field (default) and 7-field (legacy with year) cron expressions.
func (s *Scheduler) upsertScheduleEntry(key, cronExpr string, jobType models.JobType, targetID models.ULID, targetName string) error {
	// Normalize cron expression (handles both 6-field and legacy 7-field)
	normalizedExpr, err := NormalizeCronExpression(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Validate the normalized cron expression
	schedule, err := s.parser.Parse(normalizedExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Check if entry already exists
	if existingID, exists := s.entryMap[key]; exists {
		entry := s.cronScheduler.Entry(existingID)
		if entry.Valid() {
			// Check if schedule changed by comparing next run times
			// This is a heuristic - same next time likely means same schedule
			existingNext := entry.Schedule.Next(time.Now())
			newNext := schedule.Next(time.Now())
			if existingNext.Equal(newNext) {
				// Schedule hasn't changed, nothing to do
				return nil
			}
		}
		// Remove old entry
		s.cronScheduler.Remove(existingID)
		delete(s.entryMap, key)
	}

	// Create the job function that enqueues work
	jobFunc := s.createJobFunc(jobType, targetID, targetName, normalizedExpr)

	// Add to cron scheduler
	entryID, err := s.cronScheduler.AddFunc(normalizedExpr, jobFunc)
	if err != nil {
		return fmt.Errorf("adding cron entry: %w", err)
	}

	s.entryMap[key] = entryID
	s.logger.Debug("added schedule",
		slog.String("key", key),
		slog.String("cron", cronExpr),
		slog.String("normalized", normalizedExpr),
		slog.Time("next_run", schedule.Next(time.Now())))

	return nil
}

// createJobFunc creates a function that enqueues a job when the cron fires.
func (s *Scheduler) createJobFunc(jobType models.JobType, targetID models.ULID, targetName, cronSchedule string) func() {
	return func() {
		ctx := context.Background()

		s.logger.Debug("cron triggered",
			slog.String("type", string(jobType)),
			slog.String("target", targetName))

		if _, err := s.createJobIfNotDuplicate(ctx, jobType, targetID, targetName, cronSchedule); err != nil {
			s.logger.Error("failed to create scheduled job",
				slog.String("type", string(jobType)),
				slog.String("target", targetName),
				slog.Any("error", err))
		}
	}
}

// createJobIfNotDuplicate creates a job if no duplicate pending job exists.
// Returns the job (existing or new) and any error.
func (s *Scheduler) createJobIfNotDuplicate(ctx context.Context, jobType models.JobType, targetID models.ULID, targetName, cronSchedule string) (*models.Job, error) {
	// Check for existing pending/running job
	existing, err := s.jobRepo.FindDuplicatePending(ctx, jobType, targetID)
	if err != nil {
		return nil, fmt.Errorf("checking for duplicate job: %w", err)
	}

	if existing != nil {
		s.logger.Debug("skipping duplicate job",
			slog.String("type", string(jobType)),
			slog.String("target", targetName),
			slog.String("existing_job_id", existing.ID.String()))
		return existing, nil
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
		return nil, fmt.Errorf("creating job: %w", err)
	}

	s.logger.Info("created scheduled job",
		slog.String("type", string(jobType)),
		slog.String("target", targetName),
		slog.String("job_id", job.ID.String()))

	return job, nil
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

	// Create new immediate job (no cron schedule)
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
// Accepts both 6-field (default) and 7-field (legacy with year) formats.
func (s *Scheduler) ParseCron(expr string) (time.Time, error) {
	// Normalize to 6-field format (strip year if present)
	normalized, err := NormalizeCronExpression(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	schedule, err := s.parser.Parse(normalized)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return schedule.Next(time.Now()), nil
}

// ValidateCron validates a cron expression.
// Accepts both 6-field (default) and 7-field (legacy with year) formats.
func (s *Scheduler) ValidateCron(expr string) error {
	// Normalize to 6-field format (strip year if present)
	normalized, err := NormalizeCronExpression(expr)
	if err != nil {
		return err
	}
	_, err = s.parser.Parse(normalized)
	return err
}

// GetEntryCount returns the number of scheduled entries.
func (s *Scheduler) GetEntryCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entryMap)
}

// CalculateNextRun calculates the next run time for a cron expression.
// This is a standalone helper function that doesn't require a Scheduler instance.
// Returns nil if the expression is empty or invalid.
func CalculateNextRun(cronExpr string) *time.Time {
	if cronExpr == "" {
		return nil
	}

	normalized, err := NormalizeCronExpression(cronExpr)
	if err != nil {
		return nil
	}

	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(normalized)
	if err != nil {
		return nil
	}

	nextRun := schedule.Next(time.Now())
	return &nextRun
}

// GetNextRunTimes returns the next run times for all scheduled entries.
func (s *Scheduler) GetNextRunTimes() map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]time.Time, len(s.entryMap))
	for key, entryID := range s.entryMap {
		entry := s.cronScheduler.Entry(entryID)
		if entry.Valid() {
			result[key] = entry.Next
		}
	}
	return result
}

// ForceSync forces an immediate sync of schedules from the database.
func (s *Scheduler) ForceSync(ctx context.Context) error {
	return s.loadSchedules(ctx)
}

// registerInternalJobs registers internal recurring jobs based on configuration.
func (s *Scheduler) registerInternalJobs() {
	for _, job := range s.internalJobs {
		if job.CronSchedule == "" {
			continue
		}

		key := fmt.Sprintf("internal:%s", job.JobType)

		// Normalize cron expression (handles both 6-field and legacy 7-field)
		cronExpr, err := NormalizeCronExpression(job.CronSchedule)
		if err != nil {
			s.logger.Error("failed to parse internal job cron schedule",
				slog.String("job_type", string(job.JobType)),
				slog.String("schedule", job.CronSchedule),
				slog.Any("error", err))
			continue
		}

		jobFunc := s.createJobFunc(job.JobType, models.ULID{}, job.TargetName, cronExpr)

		entryID, err := s.cronScheduler.AddFunc(cronExpr, jobFunc)
		if err != nil {
			s.logger.Error("failed to register internal job",
				slog.String("job_type", string(job.JobType)),
				slog.String("schedule", job.CronSchedule),
				slog.Any("error", err))
			continue
		}

		s.mu.Lock()
		s.entryMap[key] = entryID
		s.mu.Unlock()

		s.logger.Info("registered internal job",
			slog.String("job_type", string(job.JobType)),
			slog.String("target", job.TargetName),
			slog.String("schedule", job.CronSchedule),
			slog.String("normalized", cronExpr))
	}
}

// AddInternalJob adds an internal recurring job at runtime.
// This is useful for adding jobs after the scheduler has started.
// The cronSchedule supports both 6-field and 7-field (with year) formats.
func (s *Scheduler) AddInternalJob(jobType models.JobType, targetName string, cronSchedule string) error {
	if cronSchedule == "" {
		return fmt.Errorf("cron schedule cannot be empty")
	}

	// Normalize cron expression (handles both 6-field and legacy 7-field)
	cronExpr, err := NormalizeCronExpression(cronSchedule)
	if err != nil {
		return fmt.Errorf("parsing cron schedule: %w", err)
	}

	key := fmt.Sprintf("internal:%s", jobType)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if present
	if existingID, exists := s.entryMap[key]; exists {
		s.cronScheduler.Remove(existingID)
		delete(s.entryMap, key)
	}

	jobFunc := s.createJobFunc(jobType, models.ULID{}, targetName, cronExpr)

	entryID, err := s.cronScheduler.AddFunc(cronExpr, jobFunc)
	if err != nil {
		return fmt.Errorf("adding internal job: %w", err)
	}

	s.entryMap[key] = entryID

	s.logger.Info("added internal job",
		slog.String("job_type", string(jobType)),
		slog.String("target", targetName),
		slog.String("schedule", cronSchedule))

	return nil
}

// CatchupMissedRuns checks all sources with cron schedules and schedules immediate
// jobs for any that missed their scheduled run while the service was down.
// This should be called after the scheduler starts to catch up on missed work.
func (s *Scheduler) CatchupMissedRuns(ctx context.Context) (streamsCaught, epgCaught int, err error) {
	s.logger.Info("checking for missed scheduled runs")

	// Check stream sources
	streamsCaught, err = s.catchupStreamSources(ctx)
	if err != nil {
		s.logger.Error("failed to catch up stream sources", slog.Any("error", err))
	}

	// Check EPG sources
	epgCaught, err = s.catchupEpgSources(ctx)
	if err != nil {
		s.logger.Error("failed to catch up EPG sources", slog.Any("error", err))
	}

	if streamsCaught > 0 || epgCaught > 0 {
		s.logger.Info("scheduled catch-up jobs for missed runs",
			slog.Int("stream_sources", streamsCaught),
			slog.Int("epg_sources", epgCaught))
	} else {
		s.logger.Info("no missed scheduled runs detected")
	}

	return streamsCaught, epgCaught, nil
}

// catchupStreamSources checks stream sources and schedules jobs for any that missed runs.
func (s *Scheduler) catchupStreamSources(ctx context.Context) (int, error) {
	sources, err := s.streamSourceRepo.GetEnabled(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting stream sources: %w", err)
	}

	caught := 0
	now := time.Now()

	for _, source := range sources {
		if source.CronSchedule == "" {
			continue
		}

		if s.shouldCatchup(source.CronSchedule, source.LastIngestionAt, now) {
			s.logger.Debug("stream source missed scheduled run",
				slog.String("source", source.Name),
				slog.String("cron", source.CronSchedule),
				slog.Any("last_ingestion", source.LastIngestionAt))

			_, err := s.ScheduleImmediate(ctx, models.JobTypeStreamIngestion, source.ID, source.Name)
			if err != nil {
				s.logger.Error("failed to schedule catch-up job",
					slog.String("source", source.Name),
					slog.Any("error", err))
				continue
			}
			caught++
		}
	}

	return caught, nil
}

// catchupEpgSources checks EPG sources and schedules jobs for any that missed runs.
func (s *Scheduler) catchupEpgSources(ctx context.Context) (int, error) {
	sources, err := s.epgSourceRepo.GetEnabled(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting EPG sources: %w", err)
	}

	caught := 0
	now := time.Now()

	for _, source := range sources {
		if source.CronSchedule == "" {
			continue
		}

		if s.shouldCatchup(source.CronSchedule, source.LastIngestionAt, now) {
			s.logger.Debug("EPG source missed scheduled run",
				slog.String("source", source.Name),
				slog.String("cron", source.CronSchedule),
				slog.Any("last_ingestion", source.LastIngestionAt))

			_, err := s.ScheduleImmediate(ctx, models.JobTypeEpgIngestion, source.ID, source.Name)
			if err != nil {
				s.logger.Error("failed to schedule catch-up job",
					slog.String("source", source.Name),
					slog.Any("error", err))
				continue
			}
			caught++
		}
	}

	return caught, nil
}

// shouldCatchup determines if a source should have a catch-up job scheduled.
// Returns true if:
// - The source has never been ingested (lastIngestion is nil), OR
// - The next scheduled run after the last ingestion is before now (meaning we missed it)
func (s *Scheduler) shouldCatchup(cronExpr string, lastIngestion *models.Time, now time.Time) bool {
	// If never ingested, definitely need to catch up
	if lastIngestion == nil {
		return true
	}

	// Parse the cron expression
	normalized, err := NormalizeCronExpression(cronExpr)
	if err != nil {
		return false
	}

	schedule, err := s.parser.Parse(normalized)
	if err != nil {
		return false
	}

	// Calculate when the next run should have been after the last ingestion
	nextScheduledAfterLast := schedule.Next(*lastIngestion)

	// If that time is before now, we missed a run
	return nextScheduledAfterLast.Before(now)
}
