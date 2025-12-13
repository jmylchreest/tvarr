package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// FilterHandler handles filter API endpoints.
type FilterHandler struct {
	repo repository.FilterRepository
}

// NewFilterHandler creates a new filter handler.
func NewFilterHandler(repo repository.FilterRepository) *FilterHandler {
	return &FilterHandler{repo: repo}
}

// Register registers the filter routes with the API.
func (h *FilterHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listFilters",
		Method:      "GET",
		Path:        "/api/v1/filters",
		Summary:     "List filters",
		Description: "Returns all filters, ordered by priority",
		Tags:        []string{"Filters"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getFilter",
		Method:      "GET",
		Path:        "/api/v1/filters/{id}",
		Summary:     "Get filter",
		Description: "Returns a filter by ID",
		Tags:        []string{"Filters"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createFilter",
		Method:      "POST",
		Path:        "/api/v1/filters",
		Summary:     "Create filter",
		Description: "Creates a new filter",
		Tags:        []string{"Filters"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateFilter",
		Method:      "PUT",
		Path:        "/api/v1/filters/{id}",
		Summary:     "Update filter",
		Description: "Updates an existing filter",
		Tags:        []string{"Filters"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteFilter",
		Method:      "DELETE",
		Path:        "/api/v1/filters/{id}",
		Summary:     "Delete filter",
		Description: "Deletes a filter",
		Tags:        []string{"Filters"},
	}, h.Delete)
}

// FilterResponse represents a filter in API responses.
type FilterResponse struct {
	ID          string  `json:"id" doc:"Filter ID (ULID)"`
	Name        string  `json:"name" doc:"Filter name"`
	Description string  `json:"description,omitempty" doc:"Filter description"`
	SourceType  string  `json:"source_type" doc:"Source type (stream or epg)"`
	Action      string  `json:"action" doc:"Filter action (include or exclude)"`
	Expression  string  `json:"expression" doc:"Filter expression"`
	IsEnabled   bool    `json:"is_enabled" doc:"Whether the filter is enabled"`
	IsSystem    bool    `json:"is_system" doc:"Whether this is a system-provided filter (cannot be edited/deleted)"`
	SourceID    *string `json:"source_id,omitempty" doc:"Source ID to restrict filter to (optional)"`
	CreatedAt   string  `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt   string  `json:"updated_at" doc:"Last update timestamp"`
}

// FilterFromModel converts a models.Filter to FilterResponse.
func FilterFromModel(f *models.Filter) FilterResponse {
	resp := FilterResponse{
		ID:          f.ID.String(),
		Name:        f.Name,
		Description: f.Description,
		SourceType:  string(f.SourceType),
		Action:      string(f.Action),
		Expression:  f.Expression,
		IsEnabled:   f.IsEnabled,
		IsSystem:    f.IsSystem,
		CreatedAt:   f.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   f.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if f.SourceID != nil {
		s := f.SourceID.String()
		resp.SourceID = &s
	}
	return resp
}

// ListFiltersInput is the input for listing filters.
type ListFiltersInput struct {
	SourceType string `query:"source_type" doc:"Filter by source type (stream or epg)" required:"false"`
	Enabled    string `query:"enabled" doc:"Filter by enabled status (true or false)" required:"false" enum:"true,false,"`
}

// ListFiltersOutput is the output for listing filters.
type ListFiltersOutput struct {
	Body struct {
		Filters []FilterResponse `json:"filters"`
		Count   int              `json:"count"`
	}
}

// List returns all filters.
func (h *FilterHandler) List(ctx context.Context, input *ListFiltersInput) (*ListFiltersOutput, error) {
	var filters []*models.Filter
	var err error

	// Parse enabled filter (string to bool)
	enabledFilter := input.Enabled == "true"
	hasEnabledFilter := input.Enabled != ""

	if hasEnabledFilter && enabledFilter {
		if input.SourceType != "" {
			filters, err = h.repo.GetEnabledForSourceType(ctx, models.FilterSourceType(input.SourceType), nil)
		} else {
			filters, err = h.repo.GetEnabled(ctx)
		}
	} else if input.SourceType != "" {
		filters, err = h.repo.GetBySourceType(ctx, models.FilterSourceType(input.SourceType))
	} else {
		filters, err = h.repo.GetAll(ctx)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list filters", err)
	}

	resp := &ListFiltersOutput{}
	resp.Body.Filters = make([]FilterResponse, 0, len(filters))
	for _, f := range filters {
		resp.Body.Filters = append(resp.Body.Filters, FilterFromModel(f))
	}
	resp.Body.Count = len(filters)

	return resp, nil
}

// GetFilterInput is the input for getting a filter.
type GetFilterInput struct {
	ID string `path:"id" doc:"Filter ID (ULID)"`
}

// GetFilterOutput is the output for getting a filter.
type GetFilterOutput struct {
	Body FilterResponse
}

// GetByID returns a filter by ID.
func (h *FilterHandler) GetByID(ctx context.Context, input *GetFilterInput) (*GetFilterOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	filter, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get filter", err)
	}
	if filter == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("filter %s not found", input.ID))
	}

	return &GetFilterOutput{
		Body: FilterFromModel(filter),
	}, nil
}

// CreateFilterRequest is the request body for creating a filter.
type CreateFilterRequest struct {
	Name        string  `json:"name" doc:"Filter name" minLength:"1" maxLength:"255"`
	Description string  `json:"description,omitempty" doc:"Filter description" maxLength:"1024"`
	SourceType  string  `json:"source_type" doc:"Source type (stream or epg)" enum:"stream,epg"`
	Action      string  `json:"action" doc:"Filter action (include or exclude)" enum:"include,exclude"`
	Expression  string  `json:"expression" doc:"Filter expression" minLength:"1"`
	IsEnabled   *bool   `json:"is_enabled,omitempty" doc:"Whether the filter is enabled (default: true)"`
	SourceID    *string `json:"source_id,omitempty" doc:"Source ID to restrict filter to (optional)"`
}

// CreateFilterInput is the input for creating a filter.
type CreateFilterInput struct {
	Body CreateFilterRequest
}

// CreateFilterOutput is the output for creating a filter.
type CreateFilterOutput struct {
	Body FilterResponse
}

// Create creates a new filter.
func (h *FilterHandler) Create(ctx context.Context, input *CreateFilterInput) (*CreateFilterOutput, error) {
	filter := &models.Filter{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		SourceType:  models.FilterSourceType(input.Body.SourceType),
		Action:      models.FilterAction(input.Body.Action),
		Expression:  input.Body.Expression,
		IsEnabled:   true,
	}

	if input.Body.IsEnabled != nil {
		filter.IsEnabled = *input.Body.IsEnabled
	}

	if input.Body.SourceID != nil && *input.Body.SourceID != "" {
		id, err := models.ParseULID(*input.Body.SourceID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid source_id format", err)
		}
		filter.SourceID = &id
	}

	if err := h.repo.Create(ctx, filter); err != nil {
		return nil, huma.Error500InternalServerError("failed to create filter", err)
	}

	return &CreateFilterOutput{
		Body: FilterFromModel(filter),
	}, nil
}

// UpdateFilterRequest is the request body for updating a filter.
type UpdateFilterRequest struct {
	Name        *string `json:"name,omitempty" doc:"Filter name" maxLength:"255"`
	Description *string `json:"description,omitempty" doc:"Filter description" maxLength:"1024"`
	SourceType  *string `json:"source_type,omitempty" doc:"Source type (stream or epg)" enum:"stream,epg"`
	Action      *string `json:"action,omitempty" doc:"Filter action (include or exclude)" enum:"include,exclude"`
	Expression  *string `json:"expression,omitempty" doc:"Filter expression"`
	IsEnabled   *bool   `json:"is_enabled,omitempty" doc:"Whether the filter is enabled"`
	SourceID    *string `json:"source_id,omitempty" doc:"Source ID to restrict filter to (null to make global)"`
}

// UpdateFilterInput is the input for updating a filter.
type UpdateFilterInput struct {
	ID   string `path:"id" doc:"Filter ID (ULID)"`
	Body UpdateFilterRequest
}

// UpdateFilterOutput is the output for updating a filter.
type UpdateFilterOutput struct {
	Body FilterResponse
}

// Update updates an existing filter.
func (h *FilterHandler) Update(ctx context.Context, input *UpdateFilterInput) (*UpdateFilterOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	filter, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get filter", err)
	}
	if filter == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("filter %s not found", input.ID))
	}

	// System defaults can only have is_enabled toggled
	if filter.IsSystem {
		if input.Body.Name != nil || input.Body.Description != nil ||
			input.Body.SourceType != nil || input.Body.Action != nil ||
			input.Body.Expression != nil || input.Body.SourceID != nil {
			return nil, huma.Error403Forbidden("system filters can only have is_enabled toggled")
		}
		// Only allow is_enabled update
		if input.Body.IsEnabled != nil {
			filter.IsEnabled = *input.Body.IsEnabled
		}
	} else {
		// Apply updates for non-system filters
		if input.Body.Name != nil {
			filter.Name = *input.Body.Name
		}
		if input.Body.Description != nil {
			filter.Description = *input.Body.Description
		}
		if input.Body.SourceType != nil {
			filter.SourceType = models.FilterSourceType(*input.Body.SourceType)
		}
		if input.Body.Action != nil {
			filter.Action = models.FilterAction(*input.Body.Action)
		}
		if input.Body.Expression != nil {
			filter.Expression = *input.Body.Expression
		}
		if input.Body.IsEnabled != nil {
			filter.IsEnabled = *input.Body.IsEnabled
		}
		if input.Body.SourceID != nil {
			if *input.Body.SourceID == "" {
				filter.SourceID = nil
			} else {
				sourceID, err := models.ParseULID(*input.Body.SourceID)
				if err != nil {
					return nil, huma.Error400BadRequest("invalid source_id format", err)
				}
				filter.SourceID = &sourceID
			}
		}
	}

	if err := h.repo.Update(ctx, filter); err != nil {
		return nil, huma.Error500InternalServerError("failed to update filter", err)
	}

	return &UpdateFilterOutput{
		Body: FilterFromModel(filter),
	}, nil
}

// DeleteFilterInput is the input for deleting a filter.
type DeleteFilterInput struct {
	ID string `path:"id" doc:"Filter ID (ULID)"`
}

// DeleteFilterOutput is the output for deleting a filter.
type DeleteFilterOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Delete deletes a filter.
func (h *FilterHandler) Delete(ctx context.Context, input *DeleteFilterInput) (*DeleteFilterOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	filter, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get filter", err)
	}
	if filter == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("filter %s not found", input.ID))
	}

	// Prevent deletion of system filters
	if filter.IsSystem {
		return nil, huma.Error403Forbidden("system filters cannot be deleted")
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete filter", err)
	}

	return &DeleteFilterOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("filter %s deleted", input.ID),
		},
	}, nil
}
