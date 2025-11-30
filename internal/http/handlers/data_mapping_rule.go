package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// DataMappingRuleHandler handles data mapping rule API endpoints.
type DataMappingRuleHandler struct {
	repo repository.DataMappingRuleRepository
}

// NewDataMappingRuleHandler creates a new data mapping rule handler.
func NewDataMappingRuleHandler(repo repository.DataMappingRuleRepository) *DataMappingRuleHandler {
	return &DataMappingRuleHandler{repo: repo}
}

// Register registers the data mapping rule routes with the API.
func (h *DataMappingRuleHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listDataMappingRules",
		Method:      "GET",
		Path:        "/api/v1/data-mapping-rules",
		Summary:     "List data mapping rules",
		Description: "Returns all data mapping rules, ordered by priority",
		Tags:        []string{"DataMappingRules"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getDataMappingRule",
		Method:      "GET",
		Path:        "/api/v1/data-mapping-rules/{id}",
		Summary:     "Get data mapping rule",
		Description: "Returns a data mapping rule by ID",
		Tags:        []string{"DataMappingRules"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createDataMappingRule",
		Method:      "POST",
		Path:        "/api/v1/data-mapping-rules",
		Summary:     "Create data mapping rule",
		Description: "Creates a new data mapping rule",
		Tags:        []string{"DataMappingRules"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateDataMappingRule",
		Method:      "PUT",
		Path:        "/api/v1/data-mapping-rules/{id}",
		Summary:     "Update data mapping rule",
		Description: "Updates an existing data mapping rule",
		Tags:        []string{"DataMappingRules"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteDataMappingRule",
		Method:      "DELETE",
		Path:        "/api/v1/data-mapping-rules/{id}",
		Summary:     "Delete data mapping rule",
		Description: "Deletes a data mapping rule",
		Tags:        []string{"DataMappingRules"},
	}, h.Delete)
}

// DataMappingRuleResponse represents a data mapping rule in API responses.
type DataMappingRuleResponse struct {
	ID          string  `json:"id" doc:"Rule ID (ULID)"`
	Name        string  `json:"name" doc:"Rule name"`
	Description string  `json:"description,omitempty" doc:"Rule description"`
	SourceType  string  `json:"source_type" doc:"Source type (stream or epg)"`
	Expression  string  `json:"expression" doc:"Data mapping expression"`
	Priority    int     `json:"priority" doc:"Priority (lower = higher priority)"`
	StopOnMatch bool    `json:"stop_on_match" doc:"Stop processing subsequent rules when this rule matches"`
	IsEnabled   bool    `json:"is_enabled" doc:"Whether the rule is enabled"`
	SourceID    *string `json:"source_id,omitempty" doc:"Source ID to restrict rule to (optional)"`
	CreatedAt   string  `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt   string  `json:"updated_at" doc:"Last update timestamp"`
}

// DataMappingRuleFromModel converts a models.DataMappingRule to DataMappingRuleResponse.
func DataMappingRuleFromModel(r *models.DataMappingRule) DataMappingRuleResponse {
	resp := DataMappingRuleResponse{
		ID:          r.ID.String(),
		Name:        r.Name,
		Description: r.Description,
		SourceType:  string(r.SourceType),
		Expression:  r.Expression,
		Priority:    r.Priority,
		StopOnMatch: r.StopOnMatch,
		IsEnabled:   r.IsEnabled,
		CreatedAt:   r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if r.SourceID != nil {
		s := r.SourceID.String()
		resp.SourceID = &s
	}
	return resp
}

// ListDataMappingRulesInput is the input for listing data mapping rules.
type ListDataMappingRulesInput struct {
	SourceType string `query:"source_type" doc:"Filter by source type (stream or epg)" required:"false"`
	Enabled    string `query:"enabled" doc:"Filter by enabled status (true or false)" required:"false" enum:"true,false,"`
}

// ListDataMappingRulesOutput is the output for listing data mapping rules.
type ListDataMappingRulesOutput struct {
	Body struct {
		Rules []DataMappingRuleResponse `json:"rules"`
		Count int                       `json:"count"`
	}
}

// List returns all data mapping rules.
func (h *DataMappingRuleHandler) List(ctx context.Context, input *ListDataMappingRulesInput) (*ListDataMappingRulesOutput, error) {
	var rules []*models.DataMappingRule
	var err error

	// Parse enabled filter (string to bool)
	enabledFilter := input.Enabled == "true"
	hasEnabledFilter := input.Enabled != ""

	if hasEnabledFilter && enabledFilter {
		if input.SourceType != "" {
			rules, err = h.repo.GetEnabledForSourceType(ctx, models.DataMappingRuleSourceType(input.SourceType), nil)
		} else {
			rules, err = h.repo.GetEnabled(ctx)
		}
	} else if input.SourceType != "" {
		rules, err = h.repo.GetBySourceType(ctx, models.DataMappingRuleSourceType(input.SourceType))
	} else {
		rules, err = h.repo.GetAll(ctx)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list data mapping rules", err)
	}

	resp := &ListDataMappingRulesOutput{}
	resp.Body.Rules = make([]DataMappingRuleResponse, 0, len(rules))
	for _, r := range rules {
		resp.Body.Rules = append(resp.Body.Rules, DataMappingRuleFromModel(r))
	}
	resp.Body.Count = len(rules)

	return resp, nil
}

// GetDataMappingRuleInput is the input for getting a data mapping rule.
type GetDataMappingRuleInput struct {
	ID string `path:"id" doc:"Rule ID (ULID)"`
}

// GetDataMappingRuleOutput is the output for getting a data mapping rule.
type GetDataMappingRuleOutput struct {
	Body DataMappingRuleResponse
}

// GetByID returns a data mapping rule by ID.
func (h *DataMappingRuleHandler) GetByID(ctx context.Context, input *GetDataMappingRuleInput) (*GetDataMappingRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	rule, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get data mapping rule", err)
	}
	if rule == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("data mapping rule %s not found", input.ID))
	}

	return &GetDataMappingRuleOutput{
		Body: DataMappingRuleFromModel(rule),
	}, nil
}

// CreateDataMappingRuleRequest is the request body for creating a data mapping rule.
type CreateDataMappingRuleRequest struct {
	Name        string  `json:"name" doc:"Rule name" minLength:"1" maxLength:"255"`
	Description string  `json:"description,omitempty" doc:"Rule description" maxLength:"1024"`
	SourceType  string  `json:"source_type" doc:"Source type (stream or epg)" enum:"stream,epg"`
	Expression  string  `json:"expression" doc:"Data mapping expression" minLength:"1"`
	Priority    int     `json:"priority" doc:"Priority (lower = higher priority)"`
	StopOnMatch *bool   `json:"stop_on_match,omitempty" doc:"Stop processing subsequent rules when this rule matches (default: false)"`
	IsEnabled   *bool   `json:"is_enabled,omitempty" doc:"Whether the rule is enabled (default: true)"`
	SourceID    *string `json:"source_id,omitempty" doc:"Source ID to restrict rule to (optional)"`
}

// CreateDataMappingRuleInput is the input for creating a data mapping rule.
type CreateDataMappingRuleInput struct {
	Body CreateDataMappingRuleRequest
}

// CreateDataMappingRuleOutput is the output for creating a data mapping rule.
type CreateDataMappingRuleOutput struct {
	Body DataMappingRuleResponse
}

// Create creates a new data mapping rule.
func (h *DataMappingRuleHandler) Create(ctx context.Context, input *CreateDataMappingRuleInput) (*CreateDataMappingRuleOutput, error) {
	rule := &models.DataMappingRule{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		SourceType:  models.DataMappingRuleSourceType(input.Body.SourceType),
		Expression:  input.Body.Expression,
		Priority:    input.Body.Priority,
		StopOnMatch: false,
		IsEnabled:   true,
	}

	if input.Body.StopOnMatch != nil {
		rule.StopOnMatch = *input.Body.StopOnMatch
	}

	if input.Body.IsEnabled != nil {
		rule.IsEnabled = *input.Body.IsEnabled
	}

	if input.Body.SourceID != nil && *input.Body.SourceID != "" {
		id, err := models.ParseULID(*input.Body.SourceID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid source_id format", err)
		}
		rule.SourceID = &id
	}

	if err := h.repo.Create(ctx, rule); err != nil {
		return nil, huma.Error500InternalServerError("failed to create data mapping rule", err)
	}

	return &CreateDataMappingRuleOutput{
		Body: DataMappingRuleFromModel(rule),
	}, nil
}

// UpdateDataMappingRuleRequest is the request body for updating a data mapping rule.
type UpdateDataMappingRuleRequest struct {
	Name        *string `json:"name,omitempty" doc:"Rule name" maxLength:"255"`
	Description *string `json:"description,omitempty" doc:"Rule description" maxLength:"1024"`
	SourceType  *string `json:"source_type,omitempty" doc:"Source type (stream or epg)" enum:"stream,epg"`
	Expression  *string `json:"expression,omitempty" doc:"Data mapping expression"`
	Priority    *int    `json:"priority,omitempty" doc:"Priority (lower = higher priority)"`
	StopOnMatch *bool   `json:"stop_on_match,omitempty" doc:"Stop processing subsequent rules when this rule matches"`
	IsEnabled   *bool   `json:"is_enabled,omitempty" doc:"Whether the rule is enabled"`
	SourceID    *string `json:"source_id,omitempty" doc:"Source ID to restrict rule to (null to make global)"`
}

// UpdateDataMappingRuleInput is the input for updating a data mapping rule.
type UpdateDataMappingRuleInput struct {
	ID   string `path:"id" doc:"Rule ID (ULID)"`
	Body UpdateDataMappingRuleRequest
}

// UpdateDataMappingRuleOutput is the output for updating a data mapping rule.
type UpdateDataMappingRuleOutput struct {
	Body DataMappingRuleResponse
}

// Update updates an existing data mapping rule.
func (h *DataMappingRuleHandler) Update(ctx context.Context, input *UpdateDataMappingRuleInput) (*UpdateDataMappingRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	rule, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get data mapping rule", err)
	}
	if rule == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("data mapping rule %s not found", input.ID))
	}

	// Apply updates
	if input.Body.Name != nil {
		rule.Name = *input.Body.Name
	}
	if input.Body.Description != nil {
		rule.Description = *input.Body.Description
	}
	if input.Body.SourceType != nil {
		rule.SourceType = models.DataMappingRuleSourceType(*input.Body.SourceType)
	}
	if input.Body.Expression != nil {
		rule.Expression = *input.Body.Expression
	}
	if input.Body.Priority != nil {
		rule.Priority = *input.Body.Priority
	}
	if input.Body.StopOnMatch != nil {
		rule.StopOnMatch = *input.Body.StopOnMatch
	}
	if input.Body.IsEnabled != nil {
		rule.IsEnabled = *input.Body.IsEnabled
	}
	if input.Body.SourceID != nil {
		if *input.Body.SourceID == "" {
			rule.SourceID = nil
		} else {
			sourceID, err := models.ParseULID(*input.Body.SourceID)
			if err != nil {
				return nil, huma.Error400BadRequest("invalid source_id format", err)
			}
			rule.SourceID = &sourceID
		}
	}

	if err := h.repo.Update(ctx, rule); err != nil {
		return nil, huma.Error500InternalServerError("failed to update data mapping rule", err)
	}

	return &UpdateDataMappingRuleOutput{
		Body: DataMappingRuleFromModel(rule),
	}, nil
}

// DeleteDataMappingRuleInput is the input for deleting a data mapping rule.
type DeleteDataMappingRuleInput struct {
	ID string `path:"id" doc:"Rule ID (ULID)"`
}

// DeleteDataMappingRuleOutput is the output for deleting a data mapping rule.
type DeleteDataMappingRuleOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Delete deletes a data mapping rule.
func (h *DataMappingRuleHandler) Delete(ctx context.Context, input *DeleteDataMappingRuleInput) (*DeleteDataMappingRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	rule, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get data mapping rule", err)
	}
	if rule == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("data mapping rule %s not found", input.ID))
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete data mapping rule", err)
	}

	return &DeleteDataMappingRuleOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("data mapping rule %s deleted", input.ID),
		},
	}, nil
}
