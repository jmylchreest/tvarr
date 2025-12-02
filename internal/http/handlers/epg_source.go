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

// EpgSourceHandler handles EPG source API endpoints.
type EpgSourceHandler struct {
	epgService *service.EpgService
}

// NewEpgSourceHandler creates a new EPG source handler.
func NewEpgSourceHandler(epgService *service.EpgService) *EpgSourceHandler {
	return &EpgSourceHandler{
		epgService: epgService,
	}
}

// Register registers the EPG source routes with the API.
func (h *EpgSourceHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listEpgSources",
		Method:      "GET",
		Path:        "/api/v1/sources/epg",
		Summary:     "List EPG sources",
		Description: "Returns all EPG sources",
		Tags:        []string{"EPG Sources"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgSource",
		Method:      "GET",
		Path:        "/api/v1/sources/epg/{id}",
		Summary:     "Get EPG source",
		Description: "Returns an EPG source by ID",
		Tags:        []string{"EPG Sources"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createEpgSource",
		Method:      "POST",
		Path:        "/api/v1/sources/epg",
		Summary:     "Create EPG source",
		Description: "Creates a new EPG source",
		Tags:        []string{"EPG Sources"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateEpgSource",
		Method:      "PUT",
		Path:        "/api/v1/sources/epg/{id}",
		Summary:     "Update EPG source",
		Description: "Updates an existing EPG source",
		Tags:        []string{"EPG Sources"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteEpgSource",
		Method:      "DELETE",
		Path:        "/api/v1/sources/epg/{id}",
		Summary:     "Delete EPG source",
		Description: "Deletes an EPG source and all its programs",
		Tags:        []string{"EPG Sources"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "ingestEpgSource",
		Method:      "POST",
		Path:        "/api/v1/sources/epg/{id}/ingest",
		Summary:     "Trigger EPG ingestion",
		Description: "Triggers ingestion for an EPG source",
		Tags:        []string{"EPG Sources"},
	}, h.Ingest)
}

// ListEpgSourcesInput is the input for listing EPG sources.
type ListEpgSourcesInput struct{}

// ListEpgSourcesOutput is the output for listing EPG sources.
type ListEpgSourcesOutput struct {
	Body struct {
		Sources []EpgSourceResponse `json:"sources"`
	}
}

// List returns all EPG sources.
func (h *EpgSourceHandler) List(ctx context.Context, input *ListEpgSourcesInput) (*ListEpgSourcesOutput, error) {
	sources, err := h.epgService.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list EPG sources", err)
	}

	resp := &ListEpgSourcesOutput{}
	resp.Body.Sources = make([]EpgSourceResponse, 0, len(sources))
	for _, s := range sources {
		resp.Body.Sources = append(resp.Body.Sources, EpgSourceFromModel(s))
	}

	return resp, nil
}

// GetEpgSourceInput is the input for getting an EPG source.
type GetEpgSourceInput struct {
	ID string `path:"id" doc:"EPG source ID (ULID)"`
}

// GetEpgSourceOutput is the output for getting an EPG source.
type GetEpgSourceOutput struct {
	Body EpgSourceResponse
}

// GetByID returns an EPG source by ID.
func (h *EpgSourceHandler) GetByID(ctx context.Context, input *GetEpgSourceInput) (*GetEpgSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	source, err := h.epgService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("EPG source %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get EPG source", err)
	}

	return &GetEpgSourceOutput{
		Body: EpgSourceFromModel(source),
	}, nil
}

// CreateEpgSourceInput is the input for creating an EPG source.
type CreateEpgSourceInput struct {
	Body CreateEpgSourceRequest
}

// CreateEpgSourceOutput is the output for creating an EPG source.
type CreateEpgSourceOutput struct {
	Body EpgSourceResponse
}

// Create creates a new EPG source.
func (h *EpgSourceHandler) Create(ctx context.Context, input *CreateEpgSourceInput) (*CreateEpgSourceOutput, error) {
	source := input.Body.ToModel()

	if err := h.epgService.Create(ctx, source); err != nil {
		// Check for validation errors
		if errors.Is(err, models.ErrNameRequired) ||
			errors.Is(err, models.ErrURLRequired) ||
			errors.Is(err, models.ErrInvalidURL) ||
			errors.Is(err, models.ErrInvalidEpgSourceType) ||
			errors.Is(err, models.ErrXtreamCredentialsRequired) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		// Check for unique constraint violation (duplicate name)
		errStr := err.Error()
		if strings.Contains(errStr, "UNIQUE constraint failed") || strings.Contains(errStr, "duplicate key") {
			return nil, huma.Error409Conflict("an EPG source with this name already exists")
		}
		return nil, huma.Error500InternalServerError("failed to create EPG source", err)
	}

	return &CreateEpgSourceOutput{
		Body: EpgSourceFromModel(source),
	}, nil
}

// UpdateEpgSourceInput is the input for updating an EPG source.
type UpdateEpgSourceInput struct {
	ID   string `path:"id" doc:"EPG source ID (ULID)"`
	Body UpdateEpgSourceRequest
}

// UpdateEpgSourceOutput is the output for updating an EPG source.
type UpdateEpgSourceOutput struct {
	Body EpgSourceResponse
}

// Update updates an existing EPG source.
func (h *EpgSourceHandler) Update(ctx context.Context, input *UpdateEpgSourceInput) (*UpdateEpgSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	source, err := h.epgService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("EPG source %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get EPG source", err)
	}

	input.Body.ApplyToModel(source)

	if err := h.epgService.Update(ctx, source); err != nil {
		return nil, huma.Error500InternalServerError("failed to update EPG source", err)
	}

	return &UpdateEpgSourceOutput{
		Body: EpgSourceFromModel(source),
	}, nil
}

// DeleteEpgSourceInput is the input for deleting an EPG source.
type DeleteEpgSourceInput struct {
	ID string `path:"id" doc:"EPG source ID (ULID)"`
}

// DeleteEpgSourceOutput is the output for deleting an EPG source.
type DeleteEpgSourceOutput struct{}

// Delete deletes an EPG source.
func (h *EpgSourceHandler) Delete(ctx context.Context, input *DeleteEpgSourceInput) (*DeleteEpgSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.epgService.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("EPG source %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to delete EPG source", err)
	}

	return &DeleteEpgSourceOutput{}, nil
}

// IngestEpgSourceInput is the input for triggering EPG ingestion.
type IngestEpgSourceInput struct {
	ID string `path:"id" doc:"EPG source ID (ULID)"`
}

// IngestEpgSourceOutput is the output for triggering EPG ingestion.
type IngestEpgSourceOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Ingest triggers ingestion for an EPG source.
func (h *EpgSourceHandler) Ingest(ctx context.Context, input *IngestEpgSourceInput) (*IngestEpgSourceOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.epgService.IngestAsync(ctx, id); err != nil {
		return nil, huma.Error500InternalServerError("failed to start EPG ingestion", err)
	}

	return &IngestEpgSourceOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("EPG ingestion started for source %s", input.ID),
		},
	}, nil
}
