package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
	"gorm.io/gorm"
)

// ScheduleSyncer is called when cron schedules are changed via API.
// This allows the scheduler to immediately pick up changes without waiting for the sync interval.
type ScheduleSyncer interface {
	// ForceSync triggers an immediate sync of schedules from the database.
	ForceSync(ctx context.Context) error
}

// ProxyUsageChecker checks if entities are in use by proxies.
type ProxyUsageChecker interface {
	// GetProxyNamesByStreamSourceID returns names of proxies using a stream source.
	GetProxyNamesByStreamSourceID(ctx context.Context, sourceID models.ULID) ([]string, error)
	// GetProxyNamesByEpgSourceID returns names of proxies using an EPG source.
	GetProxyNamesByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]string, error)
	// GetProxyNamesByFilterID returns names of proxies using a filter.
	GetProxyNamesByFilterID(ctx context.Context, filterID models.ULID) ([]string, error)
	// GetProxyNamesByEncodingProfileID returns names of proxies using an encoding profile.
	GetProxyNamesByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]string, error)
}

// StreamSourceHandler handles stream source API endpoints.
type StreamSourceHandler struct {
	sourceService     *service.SourceService
	scheduleSyncer    ScheduleSyncer
	proxyUsageChecker ProxyUsageChecker
}

// NewStreamSourceHandler creates a new stream source handler.
func NewStreamSourceHandler(sourceService *service.SourceService) *StreamSourceHandler {
	return &StreamSourceHandler{
		sourceService: sourceService,
	}
}

// WithScheduleSyncer sets the schedule syncer for immediate schedule updates.
func (h *StreamSourceHandler) WithScheduleSyncer(syncer ScheduleSyncer) *StreamSourceHandler {
	h.scheduleSyncer = syncer
	return h
}

// WithProxyUsageChecker sets the proxy usage checker for delete validation.
func (h *StreamSourceHandler) WithProxyUsageChecker(checker ProxyUsageChecker) *StreamSourceHandler {
	h.proxyUsageChecker = checker
	return h
}

// syncSchedules triggers an immediate sync if a syncer is configured.
func (h *StreamSourceHandler) syncSchedules(ctx context.Context) {
	if h.scheduleSyncer != nil {
		// Fire and forget - don't block on sync errors
		go func() {
			_ = h.scheduleSyncer.ForceSync(ctx)
		}()
	}
}

// Register registers the stream source routes with the API.
func (h *StreamSourceHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listStreamSources",
		Method:      "GET",
		Path:        "/api/v1/sources/stream",
		Summary:     "List stream sources",
		Description: "Returns all stream sources",
		Tags:        []string{"Stream Sources"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getStreamSource",
		Method:      "GET",
		Path:        "/api/v1/sources/stream/{id}",
		Summary:     "Get stream source",
		Description: "Returns a stream source by ID",
		Tags:        []string{"Stream Sources"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createStreamSource",
		Method:      "POST",
		Path:        "/api/v1/sources/stream",
		Summary:     "Create stream source",
		Description: "Creates a new stream source",
		Tags:        []string{"Stream Sources"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateStreamSource",
		Method:      "PUT",
		Path:        "/api/v1/sources/stream/{id}",
		Summary:     "Update stream source",
		Description: "Updates an existing stream source",
		Tags:        []string{"Stream Sources"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteStreamSource",
		Method:      "DELETE",
		Path:        "/api/v1/sources/stream/{id}",
		Summary:     "Delete stream source",
		Description: "Deletes a stream source and all its channels",
		Tags:        []string{"Stream Sources"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "ingestStreamSource",
		Method:      "POST",
		Path:        "/api/v1/sources/stream/{id}/ingest",
		Summary:     "Trigger ingestion",
		Description: "Triggers ingestion for a stream source",
		Tags:        []string{"Stream Sources"},
	}, h.Ingest)
}

// ListStreamSourcesInput is the input for listing stream sources.
type ListStreamSourcesInput struct{}

// ListStreamSourcesOutput is the output for listing stream sources.
type ListStreamSourcesOutput struct {
	Body struct {
		Sources []StreamSourceResponse `json:"sources"`
	}
}

// List returns all stream sources.
func (h *StreamSourceHandler) List(ctx context.Context, input *ListStreamSourcesInput) (*ListStreamSourcesOutput, error) {
	sources, err := h.sourceService.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list sources", err)
	}

	resp := &ListStreamSourcesOutput{}
	resp.Body.Sources = make([]StreamSourceResponse, 0, len(sources))
	for _, s := range sources {
		resp.Body.Sources = append(resp.Body.Sources, StreamSourceFromModel(s))
	}

	return resp, nil
}

// GetStreamSourceInput is the input for getting a stream source.
type GetStreamSourceInput struct {
	ID string `path:"id" doc:"Stream source ID (ULID)"`
}

// GetStreamSourceOutput is the output for getting a stream source.
type GetStreamSourceOutput struct {
	Body StreamSourceResponse
}

// GetByID returns a stream source by ID.
func (h *StreamSourceHandler) GetByID(ctx context.Context, input *GetStreamSourceInput) (*GetStreamSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	source, err := h.sourceService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream source %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get source", err)
	}

	return &GetStreamSourceOutput{
		Body: StreamSourceFromModel(source),
	}, nil
}

// CreateStreamSourceInput is the input for creating a stream source.
type CreateStreamSourceInput struct {
	Body CreateStreamSourceRequest
}

// CreateStreamSourceOutput is the output for creating a stream source.
type CreateStreamSourceOutput struct {
	Body StreamSourceResponse
}

// Create creates a new stream source.
func (h *StreamSourceHandler) Create(ctx context.Context, input *CreateStreamSourceInput) (*CreateStreamSourceOutput, error) {
	source := input.Body.ToModel()

	if err := h.sourceService.Create(ctx, source); err != nil {
		// Check for validation errors
		if errors.Is(err, models.ErrNameRequired) ||
			errors.Is(err, models.ErrURLRequired) ||
			errors.Is(err, models.ErrInvalidURL) ||
			errors.Is(err, models.ErrInvalidSourceType) ||
			errors.Is(err, models.ErrXtreamCredentialsRequired) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		// Check for unique constraint violation (duplicate name)
		errStr := err.Error()
		if strings.Contains(errStr, "UNIQUE constraint failed") || strings.Contains(errStr, "duplicate key") {
			return nil, huma.Error409Conflict("a stream source with this name already exists")
		}
		return nil, huma.Error500InternalServerError("failed to create source", err)
	}

	// Trigger immediate schedule sync if source has a cron schedule
	if source.CronSchedule != "" {
		h.syncSchedules(ctx)
	}

	return &CreateStreamSourceOutput{
		Body: StreamSourceFromModel(source),
	}, nil
}

// UpdateStreamSourceInput is the input for updating a stream source.
type UpdateStreamSourceInput struct {
	ID   string `path:"id" doc:"Stream source ID (ULID)"`
	Body UpdateStreamSourceRequest
}

// UpdateStreamSourceOutput is the output for updating a stream source.
type UpdateStreamSourceOutput struct {
	Body StreamSourceResponse
}

// Update updates an existing stream source.
func (h *StreamSourceHandler) Update(ctx context.Context, input *UpdateStreamSourceInput) (*UpdateStreamSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	source, err := h.sourceService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream source %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get source", err)
	}

	input.Body.ApplyToModel(source)

	if err := h.sourceService.Update(ctx, source); err != nil {
		return nil, huma.Error500InternalServerError("failed to update source", err)
	}

	// Trigger immediate schedule sync (schedule may have changed)
	h.syncSchedules(ctx)

	return &UpdateStreamSourceOutput{
		Body: StreamSourceFromModel(source),
	}, nil
}

// DeleteStreamSourceInput is the input for deleting a stream source.
type DeleteStreamSourceInput struct {
	ID string `path:"id" doc:"Stream source ID (ULID)"`
}

// DeleteStreamSourceOutput is the output for deleting a stream source.
type DeleteStreamSourceOutput struct{}

// Delete deletes a stream source.
func (h *StreamSourceHandler) Delete(ctx context.Context, input *DeleteStreamSourceInput) (*DeleteStreamSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	// Check if stream source is in use by any proxies
	if h.proxyUsageChecker != nil {
		proxyNames, err := h.proxyUsageChecker.GetProxyNamesByStreamSourceID(ctx, id)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to check proxy usage", err)
		}
		if len(proxyNames) > 0 {
			return nil, huma.Error409Conflict(fmt.Sprintf(
				"cannot delete stream source: in use by %d proxy(s): %s. Remove it from these proxies first.",
				len(proxyNames), strings.Join(proxyNames, ", ")))
		}
	}

	if err := h.sourceService.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream source %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to delete source", err)
	}

	// Trigger immediate schedule sync (removed source's schedule needs cleanup)
	h.syncSchedules(ctx)

	return &DeleteStreamSourceOutput{}, nil
}

// IngestStreamSourceInput is the input for triggering ingestion.
type IngestStreamSourceInput struct {
	ID string `path:"id" doc:"Stream source ID (ULID)"`
}

// IngestStreamSourceOutput is the output for triggering ingestion.
type IngestStreamSourceOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Ingest triggers ingestion for a stream source.
func (h *StreamSourceHandler) Ingest(ctx context.Context, input *IngestStreamSourceInput) (*IngestStreamSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.sourceService.IngestAsync(ctx, id); err != nil {
		return nil, huma.Error500InternalServerError("failed to start ingestion", err)
	}

	return &IngestStreamSourceOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("ingestion started for source %s", input.ID),
		},
	}, nil
}
