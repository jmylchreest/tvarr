package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// JobHandler defines the interface for handling specific job types.
type JobHandler interface {
	// Execute runs the job and returns a result string or error.
	Execute(ctx context.Context, job *models.Job) (string, error)
}

// StreamIngestionHandler handles stream source ingestion jobs.
type StreamIngestionHandler struct {
	sourceService SourceIngestService
}

// SourceIngestService defines the service interface for stream ingestion.
type SourceIngestService interface {
	Ingest(ctx context.Context, sourceID models.ULID) error
}

// NewStreamIngestionHandler creates a new handler for stream ingestion jobs.
func NewStreamIngestionHandler(service SourceIngestService) *StreamIngestionHandler {
	return &StreamIngestionHandler{sourceService: service}
}

// Execute runs a stream ingestion job.
func (h *StreamIngestionHandler) Execute(ctx context.Context, job *models.Job) (string, error) {
	if err := h.sourceService.Ingest(ctx, job.TargetID); err != nil {
		return "", err
	}
	return fmt.Sprintf("ingested source %s", job.TargetName), nil
}

// EpgIngestionHandler handles EPG source ingestion jobs.
type EpgIngestionHandler struct {
	epgService EpgIngestService
}

// EpgIngestService defines the service interface for EPG ingestion.
type EpgIngestService interface {
	Ingest(ctx context.Context, sourceID models.ULID) error
}

// NewEpgIngestionHandler creates a new handler for EPG ingestion jobs.
func NewEpgIngestionHandler(service EpgIngestService) *EpgIngestionHandler {
	return &EpgIngestionHandler{epgService: service}
}

// Execute runs an EPG ingestion job.
func (h *EpgIngestionHandler) Execute(ctx context.Context, job *models.Job) (string, error) {
	if err := h.epgService.Ingest(ctx, job.TargetID); err != nil {
		return "", err
	}
	return fmt.Sprintf("ingested EPG source %s", job.TargetName), nil
}

// ProxyGenerationHandler handles proxy generation jobs.
type ProxyGenerationHandler struct {
	proxyService ProxyGenerateService
}

// ProxyGenerateService defines the service interface for proxy generation.
type ProxyGenerateService interface {
	Generate(ctx context.Context, proxyID models.ULID) error
}

// NewProxyGenerationHandler creates a new handler for proxy generation jobs.
func NewProxyGenerationHandler(service ProxyGenerateService) *ProxyGenerationHandler {
	return &ProxyGenerationHandler{proxyService: service}
}

// Execute runs a proxy generation job.
func (h *ProxyGenerationHandler) Execute(ctx context.Context, job *models.Job) (string, error) {
	if err := h.proxyService.Generate(ctx, job.TargetID); err != nil {
		return "", err
	}
	return fmt.Sprintf("generated proxy %s", job.TargetName), nil
}

// Executor dispatches jobs to the appropriate handlers.
type Executor struct {
	handlers map[models.JobType]JobHandler
	jobRepo  repository.JobRepository
	logger   *slog.Logger
}

// NewExecutor creates a new job executor.
func NewExecutor(jobRepo repository.JobRepository) *Executor {
	return &Executor{
		handlers: make(map[models.JobType]JobHandler),
		jobRepo:  jobRepo,
		logger:   slog.Default(),
	}
}

// WithLogger sets a custom logger.
func (e *Executor) WithLogger(logger *slog.Logger) *Executor {
	e.logger = logger
	return e
}

// RegisterHandler registers a handler for a job type.
func (e *Executor) RegisterHandler(jobType models.JobType, handler JobHandler) {
	e.handlers[jobType] = handler
}

// Execute runs a job and updates its status.
func (e *Executor) Execute(ctx context.Context, job *models.Job) error {
	handler, ok := e.handlers[job.Type]
	if !ok {
		return fmt.Errorf("no handler registered for job type: %s", job.Type)
	}

	e.logger.Info("executing job",
		slog.String("job_id", job.ID.String()),
		slog.String("type", string(job.Type)),
		slog.String("target", job.TargetName))

	// Execute the job
	result, err := handler.Execute(ctx, job)

	if err != nil {
		e.logger.Error("job failed",
			slog.String("job_id", job.ID.String()),
			slog.String("type", string(job.Type)),
			slog.Any("error", err))

		job.MarkFailed(err)

		// Schedule retry if possible
		if job.CanRetry() {
			job.ScheduleRetry()
			e.logger.Info("job scheduled for retry",
				slog.String("job_id", job.ID.String()),
				slog.Int("attempt", job.AttemptCount),
				slog.Time("next_run", job.NextRunAt.UTC()))
		}
	} else {
		e.logger.Info("job completed",
			slog.String("job_id", job.ID.String()),
			slog.String("type", string(job.Type)),
			slog.String("result", result))

		job.MarkCompleted(result)
	}

	// Save job status
	if err := e.jobRepo.Update(ctx, job); err != nil {
		e.logger.Error("failed to update job status",
			slog.String("job_id", job.ID.String()),
			slog.Any("error", err))
		return fmt.Errorf("updating job status: %w", err)
	}

	// Create history record for completed/failed jobs
	if job.IsFinished() {
		e.createHistoryRecord(ctx, job)
	}

	return nil
}

// createHistoryRecord creates a job history record.
func (e *Executor) createHistoryRecord(ctx context.Context, job *models.Job) {
	history := &models.JobHistory{
		JobID:         job.ID,
		Type:          job.Type,
		TargetID:      job.TargetID,
		TargetName:    job.TargetName,
		Status:        job.Status,
		StartedAt:     job.StartedAt,
		CompletedAt:   job.CompletedAt,
		DurationMs:    job.DurationMs,
		AttemptNumber: job.AttemptCount,
		Error:         job.LastError,
		Result:        job.Result,
	}

	if err := e.jobRepo.CreateHistory(ctx, history); err != nil {
		e.logger.Error("failed to create job history",
			slog.String("job_id", job.ID.String()),
			slog.Any("error", err))
	}
}
