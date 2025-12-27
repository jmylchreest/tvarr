package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service"
)

// EncoderOverrideHandler handles encoder override API endpoints.
type EncoderOverrideHandler struct {
	svc *service.EncoderOverrideService
}

// NewEncoderOverrideHandler creates a new encoder override handler.
func NewEncoderOverrideHandler(svc *service.EncoderOverrideService) *EncoderOverrideHandler {
	return &EncoderOverrideHandler{svc: svc}
}

// Register registers the encoder override routes with the API.
func (h *EncoderOverrideHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listEncoderOverrides",
		Method:      "GET",
		Path:        "/api/v1/encoder-overrides",
		Summary:     "List encoder overrides",
		Description: "Returns all encoder overrides, ordered by priority (highest first)",
		Tags:        []string{"Encoder Overrides"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getEncoderOverride",
		Method:      "GET",
		Path:        "/api/v1/encoder-overrides/{id}",
		Summary:     "Get encoder override",
		Description: "Returns an encoder override by ID",
		Tags:        []string{"Encoder Overrides"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createEncoderOverride",
		Method:      "POST",
		Path:        "/api/v1/encoder-overrides",
		Summary:     "Create encoder override",
		Description: "Creates a new encoder override",
		Tags:        []string{"Encoder Overrides"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateEncoderOverride",
		Method:      "PUT",
		Path:        "/api/v1/encoder-overrides/{id}",
		Summary:     "Update encoder override",
		Description: "Updates an existing encoder override. System overrides can only have is_enabled toggled.",
		Tags:        []string{"Encoder Overrides"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteEncoderOverride",
		Method:      "DELETE",
		Path:        "/api/v1/encoder-overrides/{id}",
		Summary:     "Delete encoder override",
		Description: "Deletes an encoder override. System overrides cannot be deleted.",
		Tags:        []string{"Encoder Overrides"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "toggleEncoderOverride",
		Method:      "PATCH",
		Path:        "/api/v1/encoder-overrides/{id}/toggle",
		Summary:     "Toggle encoder override",
		Description: "Toggles the enabled state of an encoder override",
		Tags:        []string{"Encoder Overrides"},
	}, h.Toggle)

	huma.Register(api, huma.Operation{
		OperationID: "reorderEncoderOverrides",
		Method:      "POST",
		Path:        "/api/v1/encoder-overrides/reorder",
		Summary:     "Reorder encoder overrides",
		Description: "Updates the priority of multiple encoder overrides",
		Tags:        []string{"Encoder Overrides"},
	}, h.Reorder)
}

// EncoderOverrideResponse represents an encoder override in API responses.
type EncoderOverrideResponse struct {
	ID            string `json:"id" doc:"Override ID (ULID)"`
	Name          string `json:"name" doc:"Override name"`
	Description   string `json:"description,omitempty" doc:"Override description"`
	CodecType     string `json:"codec_type" doc:"Codec type (video or audio)"`
	SourceCodec   string `json:"source_codec" doc:"Source codec to match (h264, h265, aac, etc.)"`
	TargetEncoder string `json:"target_encoder" doc:"Encoder to use when override matches (libx265, h264_nvenc, etc.)"`
	HWAccelMatch  string `json:"hw_accel_match,omitempty" doc:"Hardware accelerator to match (vaapi, cuda, etc.) - empty matches all"`
	CPUMatch      string `json:"cpu_match,omitempty" doc:"CPU regex pattern to match - empty matches all"`
	Priority      int    `json:"priority" doc:"Priority (higher = evaluated first)"`
	IsEnabled     bool   `json:"is_enabled" doc:"Whether the override is enabled"`
	IsSystem      bool   `json:"is_system" doc:"Whether this is a system-provided override"`
	CreatedAt     string `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt     string `json:"updated_at" doc:"Last update timestamp"`
}

// EncoderOverrideFromModel converts a models.EncoderOverride to response.
func EncoderOverrideFromModel(o *models.EncoderOverride) EncoderOverrideResponse {
	return EncoderOverrideResponse{
		ID:            o.ID.String(),
		Name:          o.Name,
		Description:   o.Description,
		CodecType:     string(o.CodecType),
		SourceCodec:   o.SourceCodec,
		TargetEncoder: o.TargetEncoder,
		HWAccelMatch:  o.HWAccelMatch,
		CPUMatch:      o.CPUMatch,
		Priority:      o.Priority,
		IsEnabled:     models.BoolVal(o.IsEnabled),
		IsSystem:      o.IsSystem,
		CreatedAt:     o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     o.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListEncoderOverridesInput is the input for listing overrides.
type ListEncoderOverridesInput struct {
	Enabled    string `query:"enabled" doc:"Filter by enabled status" required:"false" enum:"true,false,"`
	CodecType  string `query:"codec_type" doc:"Filter by codec type" required:"false" enum:"video,audio,"`
	SystemOnly string `query:"system_only" doc:"Only return system overrides" required:"false" enum:"true,false,"`
}

// ListEncoderOverridesOutput is the output for listing overrides.
type ListEncoderOverridesOutput struct {
	Body struct {
		Overrides []EncoderOverrideResponse `json:"overrides"`
		Count     int                       `json:"count"`
	}
}

// List returns all encoder overrides.
func (h *EncoderOverrideHandler) List(ctx context.Context, input *ListEncoderOverridesInput) (*ListEncoderOverridesOutput, error) {
	var overrides []*models.EncoderOverride
	var err error

	if input.SystemOnly == "true" {
		overrides, err = h.svc.GetSystem(ctx)
	} else if input.CodecType != "" {
		overrides, err = h.svc.GetByCodecType(ctx, models.EncoderOverrideCodecType(input.CodecType))
	} else if input.Enabled == "true" {
		overrides, err = h.svc.GetEnabled(ctx)
	} else {
		overrides, err = h.svc.GetAll(ctx)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list encoder overrides", err)
	}

	resp := &ListEncoderOverridesOutput{}
	resp.Body.Overrides = make([]EncoderOverrideResponse, 0, len(overrides))
	for _, o := range overrides {
		resp.Body.Overrides = append(resp.Body.Overrides, EncoderOverrideFromModel(o))
	}
	resp.Body.Count = len(overrides)

	return resp, nil
}

// GetEncoderOverrideInput is the input for getting an override.
type GetEncoderOverrideInput struct {
	ID string `path:"id" doc:"Override ID (ULID)"`
}

// GetEncoderOverrideOutput is the output for getting an override.
type GetEncoderOverrideOutput struct {
	Body EncoderOverrideResponse
}

// GetByID returns an encoder override by ID.
func (h *EncoderOverrideHandler) GetByID(ctx context.Context, input *GetEncoderOverrideInput) (*GetEncoderOverrideOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	override, err := h.svc.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrEncoderOverrideNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("encoder override %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get encoder override", err)
	}

	return &GetEncoderOverrideOutput{
		Body: EncoderOverrideFromModel(override),
	}, nil
}

// CreateEncoderOverrideRequest is the request body for creating an override.
type CreateEncoderOverrideRequest struct {
	Name          string `json:"name" doc:"Override name" minLength:"1" maxLength:"255"`
	Description   string `json:"description,omitempty" doc:"Override description"`
	CodecType     string `json:"codec_type" doc:"Codec type (video or audio)" enum:"video,audio"`
	SourceCodec   string `json:"source_codec" doc:"Source codec to match (h264, h265, aac, etc.)" minLength:"1" maxLength:"50"`
	TargetEncoder string `json:"target_encoder" doc:"Encoder to use when override matches" minLength:"1" maxLength:"100"`
	HWAccelMatch  string `json:"hw_accel_match,omitempty" doc:"Hardware accelerator to match (empty matches all)" maxLength:"50"`
	CPUMatch      string `json:"cpu_match,omitempty" doc:"CPU regex pattern to match (empty matches all)" maxLength:"255"`
	Priority      int    `json:"priority" doc:"Priority (higher = evaluated first)" default:"100"`
	IsEnabled     *bool  `json:"is_enabled,omitempty" doc:"Whether the override is enabled (default: true)"`
}

// CreateEncoderOverrideInput is the input for creating an override.
type CreateEncoderOverrideInput struct {
	Body CreateEncoderOverrideRequest
}

// CreateEncoderOverrideOutput is the output for creating an override.
type CreateEncoderOverrideOutput struct {
	Body EncoderOverrideResponse
}

// Create creates a new encoder override.
func (h *EncoderOverrideHandler) Create(ctx context.Context, input *CreateEncoderOverrideInput) (*CreateEncoderOverrideOutput, error) {
	override := &models.EncoderOverride{
		Name:          input.Body.Name,
		Description:   input.Body.Description,
		CodecType:     models.EncoderOverrideCodecType(input.Body.CodecType),
		SourceCodec:   input.Body.SourceCodec,
		TargetEncoder: input.Body.TargetEncoder,
		HWAccelMatch:  input.Body.HWAccelMatch,
		CPUMatch:      input.Body.CPUMatch,
		Priority:      input.Body.Priority,
		IsEnabled:     models.BoolPtr(true),
	}

	if input.Body.IsEnabled != nil {
		override.IsEnabled = input.Body.IsEnabled
	}

	if err := h.svc.Create(ctx, override); err != nil {
		// Check for validation errors
		if ve, ok := err.(models.ValidationError); ok {
			return nil, huma.Error400BadRequest(ve.Error())
		}
		return nil, huma.Error500InternalServerError("failed to create encoder override", err)
	}

	return &CreateEncoderOverrideOutput{
		Body: EncoderOverrideFromModel(override),
	}, nil
}

// UpdateEncoderOverrideRequest is the request body for updating an override.
type UpdateEncoderOverrideRequest struct {
	Name          *string `json:"name,omitempty" doc:"Override name" maxLength:"255"`
	Description   *string `json:"description,omitempty" doc:"Override description"`
	CodecType     *string `json:"codec_type,omitempty" doc:"Codec type (video or audio)" enum:"video,audio,"`
	SourceCodec   *string `json:"source_codec,omitempty" doc:"Source codec to match" maxLength:"50"`
	TargetEncoder *string `json:"target_encoder,omitempty" doc:"Encoder to use when override matches" maxLength:"100"`
	HWAccelMatch  *string `json:"hw_accel_match,omitempty" doc:"Hardware accelerator to match" maxLength:"50"`
	CPUMatch      *string `json:"cpu_match,omitempty" doc:"CPU regex pattern to match" maxLength:"255"`
	Priority      *int    `json:"priority,omitempty" doc:"Priority (higher = evaluated first)"`
	IsEnabled     *bool   `json:"is_enabled,omitempty" doc:"Whether the override is enabled"`
}

// UpdateEncoderOverrideInput is the input for updating an override.
type UpdateEncoderOverrideInput struct {
	ID   string `path:"id" doc:"Override ID (ULID)"`
	Body UpdateEncoderOverrideRequest
}

// UpdateEncoderOverrideOutput is the output for updating an override.
type UpdateEncoderOverrideOutput struct {
	Body EncoderOverrideResponse
}

// Update updates an existing encoder override.
func (h *EncoderOverrideHandler) Update(ctx context.Context, input *UpdateEncoderOverrideInput) (*UpdateEncoderOverrideOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	override, err := h.svc.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrEncoderOverrideNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("encoder override %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get encoder override", err)
	}

	// System overrides can only have is_enabled toggled
	if override.IsSystem {
		if input.Body.Name != nil || input.Body.Description != nil ||
			input.Body.CodecType != nil || input.Body.SourceCodec != nil ||
			input.Body.TargetEncoder != nil || input.Body.HWAccelMatch != nil ||
			input.Body.CPUMatch != nil || input.Body.Priority != nil {
			return nil, huma.Error403Forbidden("system overrides can only have is_enabled toggled")
		}
		// Only allow is_enabled update
		if input.Body.IsEnabled != nil {
			override.IsEnabled = input.Body.IsEnabled
		}
	} else {
		// Apply updates for non-system overrides
		if input.Body.Name != nil {
			override.Name = *input.Body.Name
		}
		if input.Body.Description != nil {
			override.Description = *input.Body.Description
		}
		if input.Body.CodecType != nil {
			override.CodecType = models.EncoderOverrideCodecType(*input.Body.CodecType)
		}
		if input.Body.SourceCodec != nil {
			override.SourceCodec = *input.Body.SourceCodec
		}
		if input.Body.TargetEncoder != nil {
			override.TargetEncoder = *input.Body.TargetEncoder
		}
		if input.Body.HWAccelMatch != nil {
			override.HWAccelMatch = *input.Body.HWAccelMatch
		}
		if input.Body.CPUMatch != nil {
			override.CPUMatch = *input.Body.CPUMatch
		}
		if input.Body.Priority != nil {
			override.Priority = *input.Body.Priority
		}
		if input.Body.IsEnabled != nil {
			override.IsEnabled = input.Body.IsEnabled
		}
	}

	if err := h.svc.Update(ctx, override); err != nil {
		if errors.Is(err, service.ErrEncoderOverrideCannotEditSystem) {
			return nil, huma.Error403Forbidden("cannot edit system override")
		}
		// Check for validation errors
		if ve, ok := err.(models.ValidationError); ok {
			return nil, huma.Error400BadRequest(ve.Error())
		}
		return nil, huma.Error500InternalServerError("failed to update encoder override", err)
	}

	return &UpdateEncoderOverrideOutput{
		Body: EncoderOverrideFromModel(override),
	}, nil
}

// DeleteEncoderOverrideInput is the input for deleting an override.
type DeleteEncoderOverrideInput struct {
	ID string `path:"id" doc:"Override ID (ULID)"`
}

// DeleteEncoderOverrideOutput is the output for deleting an override.
type DeleteEncoderOverrideOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Delete deletes an encoder override.
func (h *EncoderOverrideHandler) Delete(ctx context.Context, input *DeleteEncoderOverrideInput) (*DeleteEncoderOverrideOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.svc.Delete(ctx, id); err != nil {
		if errors.Is(err, service.ErrEncoderOverrideNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("encoder override %s not found", input.ID))
		}
		if errors.Is(err, service.ErrEncoderOverrideCannotDeleteSystem) {
			return nil, huma.Error403Forbidden("system overrides cannot be deleted")
		}
		return nil, huma.Error500InternalServerError("failed to delete encoder override", err)
	}

	return &DeleteEncoderOverrideOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("encoder override %s deleted", input.ID),
		},
	}, nil
}

// ToggleEncoderOverrideInput is the input for toggling an override.
type ToggleEncoderOverrideInput struct {
	ID string `path:"id" doc:"Override ID (ULID)"`
}

// ToggleEncoderOverrideOutput is the output for toggling an override.
type ToggleEncoderOverrideOutput struct {
	Body EncoderOverrideResponse
}

// Toggle toggles the enabled state of an encoder override.
func (h *EncoderOverrideHandler) Toggle(ctx context.Context, input *ToggleEncoderOverrideInput) (*ToggleEncoderOverrideOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	override, err := h.svc.ToggleEnabled(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrEncoderOverrideNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("encoder override %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to toggle encoder override", err)
	}

	return &ToggleEncoderOverrideOutput{
		Body: EncoderOverrideFromModel(override),
	}, nil
}

// ReorderEncoderOverridesRequest is the request body for reordering overrides.
type ReorderEncoderOverridesRequest struct {
	Reorders []ReorderItem `json:"reorders" doc:"List of override ID and priority pairs"`
}

// ReorderEncoderOverridesInput is the input for reordering overrides.
type ReorderEncoderOverridesInput struct {
	Body ReorderEncoderOverridesRequest
}

// ReorderEncoderOverridesOutput is the output for reordering overrides.
type ReorderEncoderOverridesOutput struct {
	Body struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
}

// Reorder updates the priority of multiple encoder overrides.
func (h *EncoderOverrideHandler) Reorder(ctx context.Context, input *ReorderEncoderOverridesInput) (*ReorderEncoderOverridesOutput, error) {
	reorders := make([]repository.ReorderRequest, 0, len(input.Body.Reorders))
	for _, r := range input.Body.Reorders {
		id, err := models.ParseULID(r.ID)
		if err != nil {
			return nil, huma.Error400BadRequest(fmt.Sprintf("invalid ID format: %s", r.ID), err)
		}
		reorders = append(reorders, repository.ReorderRequest{
			ID:       id,
			Priority: r.Priority,
		})
	}

	if err := h.svc.Reorder(ctx, reorders); err != nil {
		return nil, huma.Error500InternalServerError("failed to reorder encoder overrides", err)
	}

	return &ReorderEncoderOverridesOutput{
		Body: struct {
			Message string `json:"message"`
			Count   int    `json:"count"`
		}{
			Message: "encoder overrides reordered",
			Count:   len(reorders),
		},
	}, nil
}
