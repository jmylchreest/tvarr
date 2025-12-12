package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service"
)

// validMappingVideoCodecs are the allowed values for preferred video codec in mappings.
// These are codec names, NOT encoder names (e.g., "h265" not "libx265").
var validMappingVideoCodecs = map[string]bool{
	"":     true, // Empty means default
	"copy": true,
	"h264": true,
	"h265": true,
	"hevc": true, // Alias for h265
	"vp9":  true,
	"av1":  true,
}

// validMappingAudioCodecs are the allowed values for preferred audio codec in mappings.
// These are codec names, NOT encoder names (e.g., "aac" not "libfdk_aac").
var validMappingAudioCodecs = map[string]bool{
	"":     true, // Empty means default
	"copy": true,
	"aac":  true,
	"mp3":  true,
	"ac3":  true,
	"eac3": true,
	"opus": true,
	"flac": true,
}

// validateMappingVideoCodec checks if a video codec value is valid for mappings.
// Returns an error if the value looks like an encoder name instead of a codec name.
func validateMappingVideoCodec(codec string) error {
	if validMappingVideoCodecs[codec] {
		return nil
	}
	// Check for common encoder names that users might accidentally use
	switch codec {
	case "libx264", "libx265", "h264_nvenc", "hevc_nvenc", "h264_vaapi", "hevc_vaapi",
		"h264_qsv", "hevc_qsv", "libvpx", "libvpx-vp9", "libaom-av1", "av1_nvenc", "av1_qsv":
		return fmt.Errorf("invalid preferred_video_codec '%s': use codec name (h264, h265, vp9, av1) not encoder name", codec)
	}
	return fmt.Errorf("invalid preferred_video_codec '%s': must be one of: copy, h264, h265, vp9, av1", codec)
}

// validateMappingAudioCodec checks if an audio codec value is valid for mappings.
// Returns an error if the value looks like an encoder name instead of a codec name.
func validateMappingAudioCodec(codec string) error {
	if validMappingAudioCodecs[codec] {
		return nil
	}
	// Check for common encoder names that users might accidentally use
	switch codec {
	case "libfdk_aac", "libmp3lame", "libopus", "libvorbis", "libflac":
		return fmt.Errorf("invalid preferred_audio_codec '%s': use codec name (aac, mp3, opus, ac3) not encoder name", codec)
	}
	return fmt.Errorf("invalid preferred_audio_codec '%s': must be one of: copy, aac, mp3, ac3, eac3, opus, flac", codec)
}

// RelayProfileMappingHandler handles relay profile mapping API endpoints.
type RelayProfileMappingHandler struct {
	service *service.RelayProfileMappingService
}

// NewRelayProfileMappingHandler creates a new relay profile mapping handler.
func NewRelayProfileMappingHandler(svc *service.RelayProfileMappingService) *RelayProfileMappingHandler {
	return &RelayProfileMappingHandler{service: svc}
}

// Register registers the relay profile mapping routes with the API.
func (h *RelayProfileMappingHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listRelayProfileMappings",
		Method:      "GET",
		Path:        "/api/v1/relay-profile-mappings",
		Summary:     "List relay profile mappings",
		Description: "Returns all relay profile mappings, ordered by priority",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getRelayProfileMapping",
		Method:      "GET",
		Path:        "/api/v1/relay-profile-mappings/{id}",
		Summary:     "Get relay profile mapping",
		Description: "Returns a relay profile mapping by ID",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createRelayProfileMapping",
		Method:      "POST",
		Path:        "/api/v1/relay-profile-mappings",
		Summary:     "Create relay profile mapping",
		Description: "Creates a new relay profile mapping",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateRelayProfileMapping",
		Method:      "PUT",
		Path:        "/api/v1/relay-profile-mappings/{id}",
		Summary:     "Update relay profile mapping",
		Description: "Updates an existing relay profile mapping",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteRelayProfileMapping",
		Method:      "DELETE",
		Path:        "/api/v1/relay-profile-mappings/{id}",
		Summary:     "Delete relay profile mapping",
		Description: "Deletes a relay profile mapping",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "reorderRelayProfileMappings",
		Method:      "POST",
		Path:        "/api/v1/relay-profile-mappings/reorder",
		Summary:     "Reorder relay profile mappings",
		Description: "Updates the priority of multiple mappings in a batch",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.Reorder)

	huma.Register(api, huma.Operation{
		OperationID: "testRelayProfileMappingExpression",
		Method:      "POST",
		Path:        "/api/v1/relay-profile-mappings/test",
		Summary:     "Test relay profile mapping expression",
		Description: "Tests an expression against sample request data",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.TestExpression)

	huma.Register(api, huma.Operation{
		OperationID: "getRelayProfileMappingStats",
		Method:      "GET",
		Path:        "/api/v1/relay-profile-mappings/stats",
		Summary:     "Get relay profile mapping statistics",
		Description: "Returns statistics about relay profile mappings",
		Tags:        []string{"Relay Profile Mappings"},
	}, h.GetStats)
}

// RelayProfileMappingResponse represents a relay profile mapping in API responses.
type RelayProfileMappingResponse struct {
	ID                  string   `json:"id" doc:"Mapping ID (ULID)"`
	Name                string   `json:"name" doc:"Mapping name"`
	Description         string   `json:"description,omitempty" doc:"Mapping description"`
	Priority            int      `json:"priority" doc:"Priority (lower = higher priority)"`
	Expression          string   `json:"expression" doc:"Expression for matching request context"`
	IsEnabled           bool     `json:"is_enabled" doc:"Whether the mapping is enabled"`
	IsSystem            bool     `json:"is_system" doc:"Whether this is a system-provided mapping"`
	AcceptedVideoCodecs []string `json:"accepted_video_codecs" doc:"Source video codecs that can be copied"`
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs" doc:"Source audio codecs that can be copied"`
	AcceptedContainers  []string `json:"accepted_containers" doc:"Source containers that can pass through"`
	PreferredVideoCodec string   `json:"preferred_video_codec" doc:"Target video codec if source not accepted"`
	PreferredAudioCodec string   `json:"preferred_audio_codec" doc:"Target audio codec if source not accepted"`
	PreferredContainer  string   `json:"preferred_container" doc:"Target container if source not accepted"`
	CreatedAt           string   `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt           string   `json:"updated_at" doc:"Last update timestamp"`
}

// RelayProfileMappingFromModel converts a models.RelayProfileMapping to RelayProfileMappingResponse.
func RelayProfileMappingFromModel(m *models.RelayProfileMapping) RelayProfileMappingResponse {
	return RelayProfileMappingResponse{
		ID:                  m.ID.String(),
		Name:                m.Name,
		Description:         m.Description,
		Priority:            m.Priority,
		Expression:          m.Expression,
		IsEnabled:           m.IsEnabled,
		IsSystem:            m.IsSystem,
		AcceptedVideoCodecs: m.AcceptedVideoCodecs,
		AcceptedAudioCodecs: m.AcceptedAudioCodecs,
		AcceptedContainers:  m.AcceptedContainers,
		PreferredVideoCodec: string(m.PreferredVideoCodec),
		PreferredAudioCodec: string(m.PreferredAudioCodec),
		PreferredContainer:  string(m.PreferredContainer),
		CreatedAt:           m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           m.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListRelayProfileMappingsInput is the input for listing mappings.
type ListRelayProfileMappingsInput struct {
	Enabled string `query:"enabled" doc:"Filter by enabled status (true or false)" required:"false" enum:"true,false,"`
}

// ListRelayProfileMappingsOutput is the output for listing mappings.
type ListRelayProfileMappingsOutput struct {
	Body struct {
		Mappings []RelayProfileMappingResponse `json:"mappings"`
		Count    int                           `json:"count"`
	}
}

// List returns all relay profile mappings.
func (h *RelayProfileMappingHandler) List(ctx context.Context, input *ListRelayProfileMappingsInput) (*ListRelayProfileMappingsOutput, error) {
	var mappings []*models.RelayProfileMapping
	var err error

	if input.Enabled == "true" {
		mappings, err = h.service.GetEnabled(ctx)
	} else {
		mappings, err = h.service.GetAll(ctx)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list relay profile mappings", err)
	}

	resp := &ListRelayProfileMappingsOutput{}
	resp.Body.Mappings = make([]RelayProfileMappingResponse, 0, len(mappings))
	for _, m := range mappings {
		resp.Body.Mappings = append(resp.Body.Mappings, RelayProfileMappingFromModel(m))
	}
	resp.Body.Count = len(mappings)
	return resp, nil
}

// GetRelayProfileMappingInput is the input for getting a mapping by ID.
type GetRelayProfileMappingInput struct {
	ID string `path:"id" doc:"Mapping ID"`
}

// GetRelayProfileMappingOutput is the output for getting a mapping by ID.
type GetRelayProfileMappingOutput struct {
	Body RelayProfileMappingResponse
}

// GetByID returns a relay profile mapping by ID.
func (h *RelayProfileMappingHandler) GetByID(ctx context.Context, input *GetRelayProfileMappingInput) (*GetRelayProfileMappingOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid mapping ID", err)
	}

	mapping, err := h.service.GetByID(ctx, id)
	if err != nil {
		if err == service.ErrRelayProfileMappingNotFound {
			return nil, huma.Error404NotFound("relay profile mapping not found")
		}
		return nil, huma.Error500InternalServerError("failed to get relay profile mapping", err)
	}

	return &GetRelayProfileMappingOutput{
		Body: RelayProfileMappingFromModel(mapping),
	}, nil
}

// CreateRelayProfileMappingInput is the input for creating a mapping.
type CreateRelayProfileMappingInput struct {
	Body struct {
		Name                string   `json:"name" doc:"Mapping name" minLength:"1" maxLength:"100"`
		Description         string   `json:"description,omitempty" doc:"Mapping description" maxLength:"500"`
		Priority            int      `json:"priority" doc:"Priority (lower = higher priority)" default:"0"`
		Expression          string   `json:"expression" doc:"Expression for matching request context" minLength:"1"`
		IsEnabled           bool     `json:"is_enabled" doc:"Whether the mapping is enabled" default:"true"`
		AcceptedVideoCodecs []string `json:"accepted_video_codecs" doc:"Source video codecs that can be copied"`
		AcceptedAudioCodecs []string `json:"accepted_audio_codecs" doc:"Source audio codecs that can be copied"`
		AcceptedContainers  []string `json:"accepted_containers" doc:"Source containers that can pass through"`
		PreferredVideoCodec string   `json:"preferred_video_codec" doc:"Target video codec if source not accepted" default:"h265"`
		PreferredAudioCodec string   `json:"preferred_audio_codec" doc:"Target audio codec if source not accepted" default:"aac"`
		PreferredContainer  string   `json:"preferred_container" doc:"Target container if source not accepted" default:"fmp4"`
	}
}

// CreateRelayProfileMappingOutput is the output for creating a mapping.
type CreateRelayProfileMappingOutput struct {
	Body RelayProfileMappingResponse
}

// Create creates a new relay profile mapping.
func (h *RelayProfileMappingHandler) Create(ctx context.Context, input *CreateRelayProfileMappingInput) (*CreateRelayProfileMappingOutput, error) {
	// Validate preferred codec values - these must be codec names (h264, h265), not encoder names (libx264, libx265)
	if err := validateMappingVideoCodec(input.Body.PreferredVideoCodec); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateMappingAudioCodec(input.Body.PreferredAudioCodec); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	mapping := &models.RelayProfileMapping{
		Name:                input.Body.Name,
		Description:         input.Body.Description,
		Priority:            input.Body.Priority,
		Expression:          input.Body.Expression,
		IsEnabled:           input.Body.IsEnabled,
		AcceptedVideoCodecs: input.Body.AcceptedVideoCodecs,
		AcceptedAudioCodecs: input.Body.AcceptedAudioCodecs,
		AcceptedContainers:  input.Body.AcceptedContainers,
		PreferredVideoCodec: models.VideoCodec(input.Body.PreferredVideoCodec),
		PreferredAudioCodec: models.AudioCodec(input.Body.PreferredAudioCodec),
		PreferredContainer:  models.ContainerFormat(input.Body.PreferredContainer),
	}

	if err := h.service.Create(ctx, mapping); err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("failed to create relay profile mapping: %v", err))
	}

	return &CreateRelayProfileMappingOutput{
		Body: RelayProfileMappingFromModel(mapping),
	}, nil
}

// UpdateRelayProfileMappingInput is the input for updating a mapping.
type UpdateRelayProfileMappingInput struct {
	ID   string `path:"id" doc:"Mapping ID"`
	Body struct {
		Name                string   `json:"name" doc:"Mapping name" minLength:"1" maxLength:"100"`
		Description         string   `json:"description,omitempty" doc:"Mapping description" maxLength:"500"`
		Priority            int      `json:"priority" doc:"Priority (lower = higher priority)"`
		Expression          string   `json:"expression" doc:"Expression for matching request context" minLength:"1"`
		IsEnabled           bool     `json:"is_enabled" doc:"Whether the mapping is enabled"`
		AcceptedVideoCodecs []string `json:"accepted_video_codecs" doc:"Source video codecs that can be copied"`
		AcceptedAudioCodecs []string `json:"accepted_audio_codecs" doc:"Source audio codecs that can be copied"`
		AcceptedContainers  []string `json:"accepted_containers" doc:"Source containers that can pass through"`
		PreferredVideoCodec string   `json:"preferred_video_codec" doc:"Target video codec if source not accepted"`
		PreferredAudioCodec string   `json:"preferred_audio_codec" doc:"Target audio codec if source not accepted"`
		PreferredContainer  string   `json:"preferred_container" doc:"Target container if source not accepted"`
	}
}

// UpdateRelayProfileMappingOutput is the output for updating a mapping.
type UpdateRelayProfileMappingOutput struct {
	Body RelayProfileMappingResponse
}

// Update updates an existing relay profile mapping.
func (h *RelayProfileMappingHandler) Update(ctx context.Context, input *UpdateRelayProfileMappingInput) (*UpdateRelayProfileMappingOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid mapping ID", err)
	}

	// Validate preferred codec values - these must be codec names (h264, h265), not encoder names (libx264, libx265)
	if err := validateMappingVideoCodec(input.Body.PreferredVideoCodec); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if err := validateMappingAudioCodec(input.Body.PreferredAudioCodec); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Get existing mapping to preserve system flag
	existing, err := h.service.GetByID(ctx, id)
	if err != nil {
		if err == service.ErrRelayProfileMappingNotFound {
			return nil, huma.Error404NotFound("relay profile mapping not found")
		}
		return nil, huma.Error500InternalServerError("failed to get relay profile mapping", err)
	}

	mapping := &models.RelayProfileMapping{
		BaseModel:           existing.BaseModel,
		Name:                input.Body.Name,
		Description:         input.Body.Description,
		Priority:            input.Body.Priority,
		Expression:          input.Body.Expression,
		IsEnabled:           input.Body.IsEnabled,
		IsSystem:            existing.IsSystem, // Preserve system flag
		AcceptedVideoCodecs: input.Body.AcceptedVideoCodecs,
		AcceptedAudioCodecs: input.Body.AcceptedAudioCodecs,
		AcceptedContainers:  input.Body.AcceptedContainers,
		PreferredVideoCodec: models.VideoCodec(input.Body.PreferredVideoCodec),
		PreferredAudioCodec: models.AudioCodec(input.Body.PreferredAudioCodec),
		PreferredContainer:  models.ContainerFormat(input.Body.PreferredContainer),
	}

	if err := h.service.Update(ctx, mapping); err != nil {
		if err == service.ErrRelayProfileMappingCannotEditSystem {
			return nil, huma.Error403Forbidden("cannot edit system mapping (only enable/disable allowed)")
		}
		return nil, huma.Error400BadRequest(fmt.Sprintf("failed to update relay profile mapping: %v", err))
	}

	return &UpdateRelayProfileMappingOutput{
		Body: RelayProfileMappingFromModel(mapping),
	}, nil
}

// DeleteRelayProfileMappingInput is the input for deleting a mapping.
type DeleteRelayProfileMappingInput struct {
	ID string `path:"id" doc:"Mapping ID"`
}

// DeleteRelayProfileMappingOutput is the output for deleting a mapping.
type DeleteRelayProfileMappingOutput struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// Delete deletes a relay profile mapping.
func (h *RelayProfileMappingHandler) Delete(ctx context.Context, input *DeleteRelayProfileMappingInput) (*DeleteRelayProfileMappingOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid mapping ID", err)
	}

	if err := h.service.Delete(ctx, id); err != nil {
		if err == service.ErrRelayProfileMappingNotFound {
			return nil, huma.Error404NotFound("relay profile mapping not found")
		}
		if err == service.ErrRelayProfileMappingCannotDeleteSystem {
			return nil, huma.Error403Forbidden("cannot delete system mapping")
		}
		return nil, huma.Error500InternalServerError("failed to delete relay profile mapping", err)
	}

	return &DeleteRelayProfileMappingOutput{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "relay profile mapping deleted",
		},
	}, nil
}

// ReorderRelayProfileMappingsInput is the input for reordering mappings.
type ReorderRelayProfileMappingsInput struct {
	Body struct {
		Mappings []struct {
			ID       string `json:"id" doc:"Mapping ID"`
			Priority int    `json:"priority" doc:"New priority"`
		} `json:"mappings" doc:"List of mappings with new priorities"`
	}
}

// ReorderRelayProfileMappingsOutput is the output for reordering mappings.
type ReorderRelayProfileMappingsOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// Reorder updates the priority of multiple mappings.
func (h *RelayProfileMappingHandler) Reorder(ctx context.Context, input *ReorderRelayProfileMappingsInput) (*ReorderRelayProfileMappingsOutput, error) {
	requests := make([]repository.ReorderRequest, 0, len(input.Body.Mappings))
	for _, m := range input.Body.Mappings {
		id, err := models.ParseULID(m.ID)
		if err != nil {
			return nil, huma.Error400BadRequest(fmt.Sprintf("invalid mapping ID: %s", m.ID), err)
		}
		requests = append(requests, repository.ReorderRequest{
			ID:       id,
			Priority: m.Priority,
		})
	}

	if err := h.service.Reorder(ctx, requests); err != nil {
		return nil, huma.Error500InternalServerError("failed to reorder mappings", err)
	}

	return &ReorderRelayProfileMappingsOutput{
		Body: struct {
			Success bool `json:"success"`
		}{
			Success: true,
		},
	}, nil
}

// TestExpressionInput is the input for testing an expression.
type TestExpressionInput struct {
	Body struct {
		Expression string            `json:"expression" doc:"Expression to test" minLength:"1"`
		TestData   map[string]string `json:"test_data" doc:"Sample request context data"`
	}
}

// TestExpressionOutput is the output for testing an expression.
type TestExpressionOutput struct {
	Body struct {
		Matches bool   `json:"matches" doc:"Whether the expression matched"`
		Error   string `json:"error,omitempty" doc:"Error message if parsing failed"`
	}
}

// TestExpression tests an expression against sample request data.
func (h *RelayProfileMappingHandler) TestExpression(ctx context.Context, input *TestExpressionInput) (*TestExpressionOutput, error) {
	matches, err := h.service.TestExpression(input.Body.Expression, input.Body.TestData)
	if err != nil {
		return &TestExpressionOutput{
			Body: struct {
				Matches bool   `json:"matches" doc:"Whether the expression matched"`
				Error   string `json:"error,omitempty" doc:"Error message if parsing failed"`
			}{
				Matches: false,
				Error:   err.Error(),
			},
		}, nil
	}

	return &TestExpressionOutput{
		Body: struct {
			Matches bool   `json:"matches" doc:"Whether the expression matched"`
			Error   string `json:"error,omitempty" doc:"Error message if parsing failed"`
		}{
			Matches: matches,
		},
	}, nil
}

// GetMappingStatsInput is the input for getting statistics.
type GetMappingStatsInput struct{}

// GetMappingStatsOutput is the output for getting statistics.
type GetMappingStatsOutput struct {
	Body service.MappingStats
}

// GetStats returns statistics about relay profile mappings.
func (h *RelayProfileMappingHandler) GetStats(ctx context.Context, input *GetMappingStatsInput) (*GetMappingStatsOutput, error) {
	stats, err := h.service.GetStats(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get stats", err)
	}

	return &GetMappingStatsOutput{
		Body: *stats,
	}, nil
}
