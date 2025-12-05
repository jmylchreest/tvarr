package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
)

// JobHandler handles job API endpoints.
type JobHandler struct {
	jobService *service.JobService
}

// NewJobHandler creates a new job handler.
func NewJobHandler(jobService *service.JobService) *JobHandler {
	return &JobHandler{
		jobService: jobService,
	}
}

// Register registers the job routes with the API.
func (h *JobHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listJobs",
		Method:      "GET",
		Path:        "/api/v1/jobs",
		Summary:     "List jobs",
		Description: "Returns all jobs",
		Tags:        []string{"Jobs"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getJob",
		Method:      "GET",
		Path:        "/api/v1/jobs/{id}",
		Summary:     "Get job",
		Description: "Returns a job by ID",
		Tags:        []string{"Jobs"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "listJobsByType",
		Method:      "GET",
		Path:        "/api/v1/jobs/type/{type}",
		Summary:     "List jobs by type",
		Description: "Returns all jobs of a specific type",
		Tags:        []string{"Jobs"},
	}, h.ListByType)

	huma.Register(api, huma.Operation{
		OperationID: "listPendingJobs",
		Method:      "GET",
		Path:        "/api/v1/jobs/pending",
		Summary:     "List pending jobs",
		Description: "Returns all pending jobs",
		Tags:        []string{"Jobs"},
	}, h.ListPending)

	huma.Register(api, huma.Operation{
		OperationID: "listRunningJobs",
		Method:      "GET",
		Path:        "/api/v1/jobs/running",
		Summary:     "List running jobs",
		Description: "Returns all running jobs",
		Tags:        []string{"Jobs"},
	}, h.ListRunning)

	huma.Register(api, huma.Operation{
		OperationID: "getJobHistory",
		Method:      "GET",
		Path:        "/api/v1/jobs/history",
		Summary:     "Get job history",
		Description: "Returns job execution history with pagination",
		Tags:        []string{"Jobs"},
	}, h.GetHistory)

	huma.Register(api, huma.Operation{
		OperationID: "getJobStats",
		Method:      "GET",
		Path:        "/api/v1/jobs/stats",
		Summary:     "Get job statistics",
		Description: "Returns job statistics",
		Tags:        []string{"Jobs"},
	}, h.GetStats)

	huma.Register(api, huma.Operation{
		OperationID: "getRunnerStatus",
		Method:      "GET",
		Path:        "/api/v1/jobs/runner",
		Summary:     "Get runner status",
		Description: "Returns the job runner status",
		Tags:        []string{"Jobs"},
	}, h.GetRunnerStatus)

	huma.Register(api, huma.Operation{
		OperationID: "cancelJob",
		Method:      "POST",
		Path:        "/api/v1/jobs/{id}/cancel",
		Summary:     "Cancel job",
		Description: "Cancels a pending or running job",
		Tags:        []string{"Jobs"},
	}, h.Cancel)

	huma.Register(api, huma.Operation{
		OperationID: "deleteJob",
		Method:      "DELETE",
		Path:        "/api/v1/jobs/{id}",
		Summary:     "Delete job",
		Description: "Deletes a completed job",
		Tags:        []string{"Jobs"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "triggerStreamIngestion",
		Method:      "POST",
		Path:        "/api/v1/jobs/trigger/stream/{id}",
		Summary:     "Trigger stream ingestion",
		Description: "Triggers immediate ingestion for a stream source",
		Tags:        []string{"Jobs"},
	}, h.TriggerStreamIngestion)

	huma.Register(api, huma.Operation{
		OperationID: "triggerEpgIngestion",
		Method:      "POST",
		Path:        "/api/v1/jobs/trigger/epg/{id}",
		Summary:     "Trigger EPG ingestion",
		Description: "Triggers immediate ingestion for an EPG source",
		Tags:        []string{"Jobs"},
	}, h.TriggerEpgIngestion)

	huma.Register(api, huma.Operation{
		OperationID: "triggerProxyGeneration",
		Method:      "POST",
		Path:        "/api/v1/jobs/trigger/proxy/{id}",
		Summary:     "Trigger proxy generation",
		Description: "Triggers immediate generation for a stream proxy",
		Tags:        []string{"Jobs"},
	}, h.TriggerProxyGeneration)

	huma.Register(api, huma.Operation{
		OperationID: "validateCron",
		Method:      "POST",
		Path:        "/api/v1/jobs/cron/validate",
		Summary:     "Validate cron expression",
		Description: "Validates a cron expression and returns the next run time",
		Tags:        []string{"Jobs"},
	}, h.ValidateCron)
}

// ListJobsInput is the input for listing jobs.
type ListJobsInput struct{}

// ListJobsOutput is the output for listing jobs.
type ListJobsOutput struct {
	Body struct {
		Jobs []JobResponse `json:"jobs"`
	}
}

// List returns all jobs.
func (h *JobHandler) List(ctx context.Context, input *ListJobsInput) (*ListJobsOutput, error) {
	jobs, err := h.jobService.GetAll(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs", err)
	}

	resp := &ListJobsOutput{}
	resp.Body.Jobs = make([]JobResponse, 0, len(jobs))
	for _, j := range jobs {
		resp.Body.Jobs = append(resp.Body.Jobs, JobFromModel(j))
	}

	return resp, nil
}

// GetJobInput is the input for getting a job.
type GetJobInput struct {
	ID string `path:"id" doc:"Job ID (ULID)"`
}

// GetJobOutput is the output for getting a job.
type GetJobOutput struct {
	Body JobResponse
}

// GetByID returns a job by ID.
func (h *JobHandler) GetByID(ctx context.Context, input *GetJobInput) (*GetJobOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	job, err := h.jobService.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job", err)
	}
	if job == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("job %s not found", input.ID))
	}

	return &GetJobOutput{
		Body: JobFromModel(job),
	}, nil
}

// ListJobsByTypeInput is the input for listing jobs by type.
type ListJobsByTypeInput struct {
	Type string `path:"type" doc:"Job type (stream_ingestion, epg_ingestion, proxy_generation, logo_cleanup)" enum:"stream_ingestion,epg_ingestion,proxy_generation,logo_cleanup"`
}

// ListJobsByTypeOutput is the output for listing jobs by type.
type ListJobsByTypeOutput struct {
	Body struct {
		Jobs []JobResponse `json:"jobs"`
	}
}

// ListByType returns all jobs of a specific type.
func (h *JobHandler) ListByType(ctx context.Context, input *ListJobsByTypeInput) (*ListJobsByTypeOutput, error) {
	jobType := models.JobType(input.Type)

	jobs, err := h.jobService.GetByType(ctx, jobType)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list jobs", err)
	}

	resp := &ListJobsByTypeOutput{}
	resp.Body.Jobs = make([]JobResponse, 0, len(jobs))
	for _, j := range jobs {
		resp.Body.Jobs = append(resp.Body.Jobs, JobFromModel(j))
	}

	return resp, nil
}

// ListPendingJobsInput is the input for listing pending jobs.
type ListPendingJobsInput struct{}

// ListPendingJobsOutput is the output for listing pending jobs.
type ListPendingJobsOutput struct {
	Body struct {
		Jobs []JobResponse `json:"jobs"`
	}
}

// ListPending returns all pending jobs.
func (h *JobHandler) ListPending(ctx context.Context, input *ListPendingJobsInput) (*ListPendingJobsOutput, error) {
	jobs, err := h.jobService.GetPending(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list pending jobs", err)
	}

	resp := &ListPendingJobsOutput{}
	resp.Body.Jobs = make([]JobResponse, 0, len(jobs))
	for _, j := range jobs {
		resp.Body.Jobs = append(resp.Body.Jobs, JobFromModel(j))
	}

	return resp, nil
}

// ListRunningJobsInput is the input for listing running jobs.
type ListRunningJobsInput struct{}

// ListRunningJobsOutput is the output for listing running jobs.
type ListRunningJobsOutput struct {
	Body struct {
		Jobs []JobResponse `json:"jobs"`
	}
}

// ListRunning returns all running jobs.
func (h *JobHandler) ListRunning(ctx context.Context, input *ListRunningJobsInput) (*ListRunningJobsOutput, error) {
	jobs, err := h.jobService.GetRunning(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list running jobs", err)
	}

	resp := &ListRunningJobsOutput{}
	resp.Body.Jobs = make([]JobResponse, 0, len(jobs))
	for _, j := range jobs {
		resp.Body.Jobs = append(resp.Body.Jobs, JobFromModel(j))
	}

	return resp, nil
}

// GetJobHistoryInput is the input for getting job history.
type GetJobHistoryInput struct {
	Type   string `query:"type" doc:"Filter by job type (optional)" enum:"stream_ingestion,epg_ingestion,proxy_generation,logo_cleanup,"`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"1000" doc:"Limit for pagination"`
}

// GetJobHistoryOutput is the output for getting job history.
type GetJobHistoryOutput struct {
	Body struct {
		History    []JobHistoryResponse `json:"history"`
		Pagination PaginationMeta       `json:"pagination"`
	}
}

// GetHistory returns job execution history.
func (h *JobHandler) GetHistory(ctx context.Context, input *GetJobHistoryInput) (*GetJobHistoryOutput, error) {
	var jobType *models.JobType
	if input.Type != "" {
		jt := models.JobType(input.Type)
		jobType = &jt
	}

	history, total, err := h.jobService.GetHistory(ctx, jobType, input.Offset, input.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job history", err)
	}

	resp := &GetJobHistoryOutput{}
	resp.Body.History = make([]JobHistoryResponse, 0, len(history))
	for _, h := range history {
		resp.Body.History = append(resp.Body.History, JobHistoryFromModel(h))
	}

	totalPages := total / int64(input.Limit)
	if total%int64(input.Limit) > 0 {
		totalPages++
	}

	resp.Body.Pagination = PaginationMeta{
		CurrentPage: (input.Offset / input.Limit) + 1,
		PageSize:    input.Limit,
		TotalItems:  total,
		TotalPages:  totalPages,
	}

	return resp, nil
}

// GetJobStatsInput is the input for getting job statistics.
type GetJobStatsInput struct{}

// GetJobStatsOutput is the output for getting job statistics.
type GetJobStatsOutput struct {
	Body JobStatsResponse
}

// GetStats returns job statistics.
func (h *JobHandler) GetStats(ctx context.Context, input *GetJobStatsInput) (*GetJobStatsOutput, error) {
	stats, err := h.jobService.GetStats(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get job stats", err)
	}

	return &GetJobStatsOutput{
		Body: JobStatsResponse{
			PendingCount:   stats.PendingCount,
			RunningCount:   stats.RunningCount,
			CompletedCount: stats.CompletedCount,
			FailedCount:    stats.FailedCount,
			ByType:         stats.ByType,
		},
	}, nil
}

// GetRunnerStatusInput is the input for getting runner status.
type GetRunnerStatusInput struct{}

// GetRunnerStatusOutput is the output for getting runner status.
type GetRunnerStatusOutput struct {
	Body RunnerStatusResponse
}

// GetRunnerStatus returns the job runner status.
func (h *JobHandler) GetRunnerStatus(ctx context.Context, input *GetRunnerStatusInput) (*GetRunnerStatusOutput, error) {
	status, err := h.jobService.GetRunnerStatus()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get runner status", err)
	}

	return &GetRunnerStatusOutput{
		Body: RunnerStatusResponse{
			Running:      status.Running,
			WorkerCount:  status.WorkerCount,
			WorkerID:     status.WorkerID,
			PendingJobs:  status.PendingJobs,
			RunningJobs:  status.RunningJobs,
			PollInterval: status.PollInterval.String(),
		},
	}, nil
}

// CancelJobInput is the input for canceling a job.
type CancelJobInput struct {
	ID string `path:"id" doc:"Job ID (ULID)"`
}

// CancelJobOutput is the output for canceling a job.
type CancelJobOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Cancel cancels a pending or running job.
func (h *JobHandler) Cancel(ctx context.Context, input *CancelJobInput) (*CancelJobOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.jobService.CancelJob(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "cannot cancel") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to cancel job", err)
	}

	return &CancelJobOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("job %s cancelled", input.ID),
		},
	}, nil
}

// DeleteJobInput is the input for deleting a job.
type DeleteJobInput struct {
	ID string `path:"id" doc:"Job ID (ULID)"`
}

// DeleteJobOutput is the output for deleting a job.
type DeleteJobOutput struct{}

// Delete deletes a completed job.
func (h *JobHandler) Delete(ctx context.Context, input *DeleteJobInput) (*DeleteJobOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.jobService.DeleteJob(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "cannot delete") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to delete job", err)
	}

	return &DeleteJobOutput{}, nil
}

// TriggerStreamIngestionInput is the input for triggering stream ingestion.
type TriggerStreamIngestionInput struct {
	ID string `path:"id" doc:"Stream source ID (ULID)"`
}

// TriggerStreamIngestionOutput is the output for triggering stream ingestion.
type TriggerStreamIngestionOutput struct {
	Body JobResponse
}

// TriggerStreamIngestion triggers immediate ingestion for a stream source.
func (h *JobHandler) TriggerStreamIngestion(ctx context.Context, input *TriggerStreamIngestionInput) (*TriggerStreamIngestionOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	job, err := h.jobService.TriggerStreamIngestion(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "not configured") {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to trigger stream ingestion", err)
	}

	return &TriggerStreamIngestionOutput{
		Body: JobFromModel(job),
	}, nil
}

// TriggerEpgIngestionInput is the input for triggering EPG ingestion.
type TriggerEpgIngestionInput struct {
	ID string `path:"id" doc:"EPG source ID (ULID)"`
}

// TriggerEpgIngestionOutput is the output for triggering EPG ingestion.
type TriggerEpgIngestionOutput struct {
	Body JobResponse
}

// TriggerEpgIngestion triggers immediate ingestion for an EPG source.
func (h *JobHandler) TriggerEpgIngestion(ctx context.Context, input *TriggerEpgIngestionInput) (*TriggerEpgIngestionOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	job, err := h.jobService.TriggerEpgIngestion(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "not configured") {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to trigger EPG ingestion", err)
	}

	return &TriggerEpgIngestionOutput{
		Body: JobFromModel(job),
	}, nil
}

// TriggerProxyGenerationInput is the input for triggering proxy generation.
type TriggerProxyGenerationInput struct {
	ID string `path:"id" doc:"Stream proxy ID (ULID)"`
}

// TriggerProxyGenerationOutput is the output for triggering proxy generation.
type TriggerProxyGenerationOutput struct {
	Body JobResponse
}

// TriggerProxyGeneration triggers immediate generation for a stream proxy.
func (h *JobHandler) TriggerProxyGeneration(ctx context.Context, input *TriggerProxyGenerationInput) (*TriggerProxyGenerationOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	job, err := h.jobService.TriggerProxyGeneration(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "not configured") {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to trigger proxy generation", err)
	}

	return &TriggerProxyGenerationOutput{
		Body: JobFromModel(job),
	}, nil
}

// ValidateCronInput is the input for validating a cron expression.
type ValidateCronInput struct {
	Body ValidateCronRequest
}

// ValidateCronOutput is the output for validating a cron expression.
type ValidateCronOutput struct {
	Body ValidateCronResponse
}

// ValidateCron validates a cron expression.
func (h *JobHandler) ValidateCron(ctx context.Context, input *ValidateCronInput) (*ValidateCronOutput, error) {
	err := h.jobService.ValidateCron(input.Body.Expression)
	if err != nil {
		if strings.Contains(err.Error(), "not configured") {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		return &ValidateCronOutput{
			Body: ValidateCronResponse{
				Valid: false,
				Error: err.Error(),
			},
		}, nil
	}

	nextRun, err := h.jobService.GetNextRun(input.Body.Expression)
	if err != nil {
		return &ValidateCronOutput{
			Body: ValidateCronResponse{
				Valid: true,
				Error: fmt.Sprintf("valid but cannot calculate next run: %s", err.Error()),
			},
		}, nil
	}

	return &ValidateCronOutput{
		Body: ValidateCronResponse{
			Valid:   true,
			NextRun: &nextRun,
		},
	}, nil
}
