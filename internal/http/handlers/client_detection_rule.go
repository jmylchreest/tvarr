package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/expression"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service"
)

// ClientDetectionRuleHandler handles client detection rule API endpoints.
type ClientDetectionRuleHandler struct {
	svc *service.ClientDetectionService
}

// NewClientDetectionRuleHandler creates a new client detection rule handler.
func NewClientDetectionRuleHandler(svc *service.ClientDetectionService) *ClientDetectionRuleHandler {
	return &ClientDetectionRuleHandler{svc: svc}
}

// Register registers the client detection rule routes with the API.
func (h *ClientDetectionRuleHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listClientDetectionRules",
		Method:      "GET",
		Path:        "/api/v1/client-detection-rules",
		Summary:     "List client detection rules",
		Description: "Returns all client detection rules, ordered by priority",
		Tags:        []string{"Client Detection"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getClientDetectionRule",
		Method:      "GET",
		Path:        "/api/v1/client-detection-rules/{id}",
		Summary:     "Get client detection rule",
		Description: "Returns a client detection rule by ID",
		Tags:        []string{"Client Detection"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createClientDetectionRule",
		Method:      "POST",
		Path:        "/api/v1/client-detection-rules",
		Summary:     "Create client detection rule",
		Description: "Creates a new client detection rule",
		Tags:        []string{"Client Detection"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateClientDetectionRule",
		Method:      "PUT",
		Path:        "/api/v1/client-detection-rules/{id}",
		Summary:     "Update client detection rule",
		Description: "Updates an existing client detection rule. System rules can only have is_enabled toggled.",
		Tags:        []string{"Client Detection"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteClientDetectionRule",
		Method:      "DELETE",
		Path:        "/api/v1/client-detection-rules/{id}",
		Summary:     "Delete client detection rule",
		Description: "Deletes a client detection rule. System rules cannot be deleted.",
		Tags:        []string{"Client Detection"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "toggleClientDetectionRule",
		Method:      "PUT",
		Path:        "/api/v1/client-detection-rules/{id}/toggle",
		Summary:     "Toggle client detection rule",
		Description: "Toggles the enabled state of a client detection rule",
		Tags:        []string{"Client Detection"},
	}, h.Toggle)

	huma.Register(api, huma.Operation{
		OperationID: "reorderClientDetectionRules",
		Method:      "POST",
		Path:        "/api/v1/client-detection-rules/reorder",
		Summary:     "Reorder client detection rules",
		Description: "Updates the priority of multiple client detection rules",
		Tags:        []string{"Client Detection"},
	}, h.Reorder)

	huma.Register(api, huma.Operation{
		OperationID: "testClientDetectionExpression",
		Method:      "POST",
		Path:        "/api/v1/client-detection-rules/test",
		Summary:     "Test client detection expression",
		Description: "Tests an expression against a sample User-Agent string",
		Tags:        []string{"Client Detection"},
	}, h.Test)
}

// ClientDetectionRuleResponse represents a client detection rule in API responses.
type ClientDetectionRuleResponse struct {
	ID                  string   `json:"id" doc:"Rule ID (ULID)"`
	Name                string   `json:"name" doc:"Rule name"`
	Description         string   `json:"description,omitempty" doc:"Rule description"`
	Expression          string   `json:"expression" doc:"Expression to match against requests"`
	Priority            int      `json:"priority" doc:"Priority (lower = higher priority)"`
	IsEnabled           bool     `json:"is_enabled" doc:"Whether the rule is enabled"`
	IsSystem            bool     `json:"is_system" doc:"Whether this is a system-provided rule"`
	AcceptedVideoCodecs []string `json:"accepted_video_codecs" doc:"Video codecs this client accepts"`
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs" doc:"Audio codecs this client accepts"`
	PreferredVideoCodec string   `json:"preferred_video_codec" doc:"Video codec to transcode to if needed"`
	PreferredAudioCodec string   `json:"preferred_audio_codec" doc:"Audio codec to transcode to if needed"`
	SupportsFMP4        bool     `json:"supports_fmp4" doc:"Client supports fMP4 segments"`
	SupportsMPEGTS      bool     `json:"supports_mpegts" doc:"Client supports MPEG-TS segments"`
	PreferredFormat     string   `json:"preferred_format,omitempty" doc:"Preferred output format"`
	EncodingProfileID   *string  `json:"encoding_profile_id,omitempty" doc:"Override encoding profile ID"`
	CreatedAt           string   `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt           string   `json:"updated_at" doc:"Last update timestamp"`
}

// ClientDetectionRuleFromModel converts a models.ClientDetectionRule to response.
func ClientDetectionRuleFromModel(r *models.ClientDetectionRule) ClientDetectionRuleResponse {
	resp := ClientDetectionRuleResponse{
		ID:                  r.ID.String(),
		Name:                r.Name,
		Description:         r.Description,
		Expression:          r.Expression,
		Priority:            r.Priority,
		IsEnabled:           models.BoolVal(r.IsEnabled),
		IsSystem:            r.IsSystem,
		AcceptedVideoCodecs: r.GetAcceptedVideoCodecs(),
		AcceptedAudioCodecs: r.GetAcceptedAudioCodecs(),
		PreferredVideoCodec: string(r.PreferredVideoCodec),
		PreferredAudioCodec: string(r.PreferredAudioCodec),
		SupportsFMP4:        models.BoolVal(r.SupportsFMP4),
		SupportsMPEGTS:      models.BoolVal(r.SupportsMPEGTS),
		PreferredFormat:     r.PreferredFormat,
		CreatedAt:           r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if resp.AcceptedVideoCodecs == nil {
		resp.AcceptedVideoCodecs = []string{}
	}
	if resp.AcceptedAudioCodecs == nil {
		resp.AcceptedAudioCodecs = []string{}
	}
	if r.EncodingProfileID != nil {
		s := r.EncodingProfileID.String()
		resp.EncodingProfileID = &s
	}
	return resp
}

// ListClientDetectionRulesInput is the input for listing rules.
type ListClientDetectionRulesInput struct {
	Enabled    string `query:"enabled" doc:"Filter by enabled status" required:"false" enum:"true,false,"`
	SystemOnly string `query:"system_only" doc:"Only return system rules" required:"false" enum:"true,false,"`
}

// ListClientDetectionRulesOutput is the output for listing rules.
type ListClientDetectionRulesOutput struct {
	Body struct {
		Rules []ClientDetectionRuleResponse `json:"rules"`
		Count int                           `json:"count"`
	}
}

// List returns all client detection rules.
func (h *ClientDetectionRuleHandler) List(ctx context.Context, input *ListClientDetectionRulesInput) (*ListClientDetectionRulesOutput, error) {
	var rules []*models.ClientDetectionRule
	var err error

	if input.SystemOnly == "true" {
		rules, err = h.svc.GetSystem(ctx)
	} else if input.Enabled == "true" {
		rules, err = h.svc.GetEnabled(ctx)
	} else {
		rules, err = h.svc.GetAll(ctx)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list client detection rules", err)
	}

	resp := &ListClientDetectionRulesOutput{}
	resp.Body.Rules = make([]ClientDetectionRuleResponse, 0, len(rules))
	for _, r := range rules {
		resp.Body.Rules = append(resp.Body.Rules, ClientDetectionRuleFromModel(r))
	}
	resp.Body.Count = len(rules)

	return resp, nil
}

// GetClientDetectionRuleInput is the input for getting a rule.
type GetClientDetectionRuleInput struct {
	ID string `path:"id" doc:"Rule ID (ULID)"`
}

// GetClientDetectionRuleOutput is the output for getting a rule.
type GetClientDetectionRuleOutput struct {
	Body ClientDetectionRuleResponse
}

// GetByID returns a client detection rule by ID.
func (h *ClientDetectionRuleHandler) GetByID(ctx context.Context, input *GetClientDetectionRuleInput) (*GetClientDetectionRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	rule, err := h.svc.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrClientDetectionRuleNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("client detection rule %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get client detection rule", err)
	}

	return &GetClientDetectionRuleOutput{
		Body: ClientDetectionRuleFromModel(rule),
	}, nil
}

// CreateClientDetectionRuleRequest is the request body for creating a rule.
type CreateClientDetectionRuleRequest struct {
	Name                string   `json:"name" doc:"Rule name" minLength:"1" maxLength:"255"`
	Description         string   `json:"description,omitempty" doc:"Rule description" maxLength:"1024"`
	Expression          string   `json:"expression" doc:"Expression to match against requests" minLength:"1"`
	Priority            int      `json:"priority" doc:"Priority (lower = higher priority)"`
	IsEnabled           *bool    `json:"is_enabled,omitempty" doc:"Whether the rule is enabled (default: true)"`
	AcceptedVideoCodecs []string `json:"accepted_video_codecs" doc:"Video codecs this client accepts"`
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs" doc:"Audio codecs this client accepts"`
	PreferredVideoCodec string   `json:"preferred_video_codec" doc:"Video codec to transcode to if needed"`
	PreferredAudioCodec string   `json:"preferred_audio_codec" doc:"Audio codec to transcode to if needed"`
	SupportsFMP4        *bool    `json:"supports_fmp4,omitempty" doc:"Client supports fMP4 segments (default: true)"`
	SupportsMPEGTS      *bool    `json:"supports_mpegts,omitempty" doc:"Client supports MPEG-TS segments (default: true)"`
	PreferredFormat     string   `json:"preferred_format,omitempty" doc:"Preferred output format"`
	EncodingProfileID   *string  `json:"encoding_profile_id,omitempty" doc:"Override encoding profile ID"`
}

// CreateClientDetectionRuleInput is the input for creating a rule.
type CreateClientDetectionRuleInput struct {
	Body CreateClientDetectionRuleRequest
}

// CreateClientDetectionRuleOutput is the output for creating a rule.
type CreateClientDetectionRuleOutput struct {
	Body ClientDetectionRuleResponse
}

// Create creates a new client detection rule.
func (h *ClientDetectionRuleHandler) Create(ctx context.Context, input *CreateClientDetectionRuleInput) (*CreateClientDetectionRuleOutput, error) {
	rule := &models.ClientDetectionRule{
		Name:                input.Body.Name,
		Description:         input.Body.Description,
		Expression:          input.Body.Expression,
		Priority:            input.Body.Priority,
		IsEnabled:           models.BoolPtr(true),
		PreferredVideoCodec: models.VideoCodec(input.Body.PreferredVideoCodec),
		PreferredAudioCodec: models.AudioCodec(input.Body.PreferredAudioCodec),
		SupportsFMP4:        models.BoolPtr(true),
		SupportsMPEGTS:      models.BoolPtr(true),
		PreferredFormat:     input.Body.PreferredFormat,
	}

	if input.Body.IsEnabled != nil {
		rule.IsEnabled = input.Body.IsEnabled
	}
	if input.Body.SupportsFMP4 != nil {
		rule.SupportsFMP4 = input.Body.SupportsFMP4
	}
	if input.Body.SupportsMPEGTS != nil {
		rule.SupportsMPEGTS = input.Body.SupportsMPEGTS
	}

	if err := rule.SetAcceptedVideoCodecs(input.Body.AcceptedVideoCodecs); err != nil {
		return nil, huma.Error400BadRequest("invalid accepted_video_codecs", err)
	}
	if err := rule.SetAcceptedAudioCodecs(input.Body.AcceptedAudioCodecs); err != nil {
		return nil, huma.Error400BadRequest("invalid accepted_audio_codecs", err)
	}

	if input.Body.EncodingProfileID != nil && *input.Body.EncodingProfileID != "" {
		id, err := models.ParseULID(*input.Body.EncodingProfileID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid encoding_profile_id format", err)
		}
		rule.EncodingProfileID = &id
	}

	if err := h.svc.Create(ctx, rule); err != nil {
		if errors.Is(err, service.ErrClientDetectionRuleInvalidExpression) {
			return nil, huma.Error400BadRequest("invalid expression", err)
		}
		return nil, huma.Error500InternalServerError("failed to create client detection rule", err)
	}

	return &CreateClientDetectionRuleOutput{
		Body: ClientDetectionRuleFromModel(rule),
	}, nil
}

// UpdateClientDetectionRuleRequest is the request body for updating a rule.
type UpdateClientDetectionRuleRequest struct {
	Name                *string  `json:"name,omitempty" doc:"Rule name" maxLength:"255"`
	Description         *string  `json:"description,omitempty" doc:"Rule description" maxLength:"1024"`
	Expression          *string  `json:"expression,omitempty" doc:"Expression to match against requests"`
	Priority            *int     `json:"priority,omitempty" doc:"Priority (lower = higher priority)"`
	IsEnabled           *bool    `json:"is_enabled,omitempty" doc:"Whether the rule is enabled"`
	AcceptedVideoCodecs []string `json:"accepted_video_codecs,omitempty" doc:"Video codecs this client accepts"`
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs,omitempty" doc:"Audio codecs this client accepts"`
	PreferredVideoCodec *string  `json:"preferred_video_codec,omitempty" doc:"Video codec to transcode to if needed"`
	PreferredAudioCodec *string  `json:"preferred_audio_codec,omitempty" doc:"Audio codec to transcode to if needed"`
	SupportsFMP4        *bool    `json:"supports_fmp4,omitempty" doc:"Client supports fMP4 segments"`
	SupportsMPEGTS      *bool    `json:"supports_mpegts,omitempty" doc:"Client supports MPEG-TS segments"`
	PreferredFormat     *string  `json:"preferred_format,omitempty" doc:"Preferred output format"`
	EncodingProfileID   *string  `json:"encoding_profile_id,omitempty" doc:"Override encoding profile ID"`
}

// UpdateClientDetectionRuleInput is the input for updating a rule.
type UpdateClientDetectionRuleInput struct {
	ID   string `path:"id" doc:"Rule ID (ULID)"`
	Body UpdateClientDetectionRuleRequest
}

// UpdateClientDetectionRuleOutput is the output for updating a rule.
type UpdateClientDetectionRuleOutput struct {
	Body ClientDetectionRuleResponse
}

// Update updates an existing client detection rule.
func (h *ClientDetectionRuleHandler) Update(ctx context.Context, input *UpdateClientDetectionRuleInput) (*UpdateClientDetectionRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	rule, err := h.svc.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrClientDetectionRuleNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("client detection rule %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get client detection rule", err)
	}

	// System rules can only have is_enabled toggled
	if rule.IsSystem {
		if input.Body.Name != nil || input.Body.Description != nil ||
			input.Body.Expression != nil || input.Body.Priority != nil ||
			input.Body.AcceptedVideoCodecs != nil || input.Body.AcceptedAudioCodecs != nil ||
			input.Body.PreferredVideoCodec != nil || input.Body.PreferredAudioCodec != nil ||
			input.Body.SupportsFMP4 != nil || input.Body.SupportsMPEGTS != nil ||
			input.Body.PreferredFormat != nil || input.Body.EncodingProfileID != nil {
			return nil, huma.Error403Forbidden("system rules can only have is_enabled toggled")
		}
		// Only allow is_enabled update
		if input.Body.IsEnabled != nil {
			rule.IsEnabled = input.Body.IsEnabled
		}
	} else {
		// Apply updates for non-system rules
		if input.Body.Name != nil {
			rule.Name = *input.Body.Name
		}
		if input.Body.Description != nil {
			rule.Description = *input.Body.Description
		}
		if input.Body.Expression != nil {
			rule.Expression = *input.Body.Expression
		}
		if input.Body.Priority != nil {
			rule.Priority = *input.Body.Priority
		}
		if input.Body.IsEnabled != nil {
			rule.IsEnabled = input.Body.IsEnabled
		}
		if input.Body.AcceptedVideoCodecs != nil {
			if err := rule.SetAcceptedVideoCodecs(input.Body.AcceptedVideoCodecs); err != nil {
				return nil, huma.Error400BadRequest("invalid accepted_video_codecs", err)
			}
		}
		if input.Body.AcceptedAudioCodecs != nil {
			if err := rule.SetAcceptedAudioCodecs(input.Body.AcceptedAudioCodecs); err != nil {
				return nil, huma.Error400BadRequest("invalid accepted_audio_codecs", err)
			}
		}
		if input.Body.PreferredVideoCodec != nil {
			rule.PreferredVideoCodec = models.VideoCodec(*input.Body.PreferredVideoCodec)
		}
		if input.Body.PreferredAudioCodec != nil {
			rule.PreferredAudioCodec = models.AudioCodec(*input.Body.PreferredAudioCodec)
		}
		if input.Body.SupportsFMP4 != nil {
			rule.SupportsFMP4 = input.Body.SupportsFMP4
		}
		if input.Body.SupportsMPEGTS != nil {
			rule.SupportsMPEGTS = input.Body.SupportsMPEGTS
		}
		if input.Body.PreferredFormat != nil {
			rule.PreferredFormat = *input.Body.PreferredFormat
		}
		if input.Body.EncodingProfileID != nil {
			if *input.Body.EncodingProfileID == "" {
				rule.EncodingProfileID = nil
			} else {
				profileID, err := models.ParseULID(*input.Body.EncodingProfileID)
				if err != nil {
					return nil, huma.Error400BadRequest("invalid encoding_profile_id format", err)
				}
				rule.EncodingProfileID = &profileID
			}
		}
	}

	if err := h.svc.Update(ctx, rule); err != nil {
		if errors.Is(err, service.ErrClientDetectionRuleInvalidExpression) {
			return nil, huma.Error400BadRequest("invalid expression", err)
		}
		if errors.Is(err, service.ErrClientDetectionRuleCannotEditSystem) {
			return nil, huma.Error403Forbidden("cannot edit system rule")
		}
		return nil, huma.Error500InternalServerError("failed to update client detection rule", err)
	}

	return &UpdateClientDetectionRuleOutput{
		Body: ClientDetectionRuleFromModel(rule),
	}, nil
}

// DeleteClientDetectionRuleInput is the input for deleting a rule.
type DeleteClientDetectionRuleInput struct {
	ID string `path:"id" doc:"Rule ID (ULID)"`
}

// DeleteClientDetectionRuleOutput is the output for deleting a rule.
type DeleteClientDetectionRuleOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// Delete deletes a client detection rule.
func (h *ClientDetectionRuleHandler) Delete(ctx context.Context, input *DeleteClientDetectionRuleInput) (*DeleteClientDetectionRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.svc.Delete(ctx, id); err != nil {
		if errors.Is(err, service.ErrClientDetectionRuleNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("client detection rule %s not found", input.ID))
		}
		if errors.Is(err, service.ErrClientDetectionRuleCannotDeleteSystem) {
			return nil, huma.Error403Forbidden("system rules cannot be deleted")
		}
		return nil, huma.Error500InternalServerError("failed to delete client detection rule", err)
	}

	return &DeleteClientDetectionRuleOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("client detection rule %s deleted", input.ID),
		},
	}, nil
}

// ToggleClientDetectionRuleInput is the input for toggling a rule.
type ToggleClientDetectionRuleInput struct {
	ID string `path:"id" doc:"Rule ID (ULID)"`
}

// ToggleClientDetectionRuleOutput is the output for toggling a rule.
type ToggleClientDetectionRuleOutput struct {
	Body ClientDetectionRuleResponse
}

// Toggle toggles the enabled state of a client detection rule.
func (h *ClientDetectionRuleHandler) Toggle(ctx context.Context, input *ToggleClientDetectionRuleInput) (*ToggleClientDetectionRuleOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	rule, err := h.svc.ToggleEnabled(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrClientDetectionRuleNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("client detection rule %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to toggle client detection rule", err)
	}

	return &ToggleClientDetectionRuleOutput{
		Body: ClientDetectionRuleFromModel(rule),
	}, nil
}

// ReorderClientDetectionRulesRequest is the request body for reordering rules.
type ReorderClientDetectionRulesRequest struct {
	Reorders []ReorderItem `json:"reorders" doc:"List of rule ID and priority pairs"`
}

// ReorderItem represents a single reorder request.
type ReorderItem struct {
	ID       string `json:"id" doc:"Rule ID (ULID)"`
	Priority int    `json:"priority" doc:"New priority value"`
}

// ReorderClientDetectionRulesInput is the input for reordering rules.
type ReorderClientDetectionRulesInput struct {
	Body ReorderClientDetectionRulesRequest
}

// ReorderClientDetectionRulesOutput is the output for reordering rules.
type ReorderClientDetectionRulesOutput struct {
	Body struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
}

// Reorder updates the priority of multiple client detection rules.
func (h *ClientDetectionRuleHandler) Reorder(ctx context.Context, input *ReorderClientDetectionRulesInput) (*ReorderClientDetectionRulesOutput, error) {
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
		return nil, huma.Error500InternalServerError("failed to reorder client detection rules", err)
	}

	return &ReorderClientDetectionRulesOutput{
		Body: struct {
			Message string `json:"message"`
			Count   int    `json:"count"`
		}{
			Message: "client detection rules reordered",
			Count:   len(reorders),
		},
	}, nil
}

// TestClientDetectionExpressionRequest is the request body for testing an expression.
type TestClientDetectionExpressionRequest struct {
	Expression string            `json:"expression" doc:"Expression to test" minLength:"1"`
	UserAgent  string            `json:"user_agent" doc:"User-Agent string to test against"`
	Headers    map[string]string `json:"headers,omitempty" doc:"Additional HTTP headers to test against (e.g., X-Video-Codec, X-Audio-Codec)"`
}

// TestClientDetectionExpressionInput is the input for testing an expression.
type TestClientDetectionExpressionInput struct {
	Body TestClientDetectionExpressionRequest
}

// TestClientDetectionExpressionOutput is the output for testing an expression.
type TestClientDetectionExpressionOutput struct {
	Body struct {
		Matches bool   `json:"matches" doc:"Whether the expression matched"`
		Error   string `json:"error,omitempty" doc:"Error message if expression is invalid"`
	}
}

// Test tests an expression against a sample User-Agent string and optional headers.
func (h *ClientDetectionRuleHandler) Test(ctx context.Context, input *TestClientDetectionExpressionInput) (*TestClientDetectionExpressionOutput, error) {
	// Create a mock request with the provided User-Agent and headers
	// We use the expression package directly for parsing validation
	// and the service for full evaluation

	result := &TestClientDetectionExpressionOutput{}

	// Test the expression with user agent and optional headers
	matches, err := h.testExpressionWithHeaders(input.Body.Expression, input.Body.UserAgent, input.Body.Headers)
	if err != nil {
		result.Body.Matches = false
		result.Body.Error = err.Error()
		return result, nil
	}

	result.Body.Matches = matches
	return result, nil
}

// testExpressionWithHeaders tests an expression against a User-Agent string and optional headers.
// This supports:
// - Static fields (client_ip, path, url, method, host)
// - @dynamic(request.headers):header-name syntax (preferred)
// - Legacy @header_req:Header-Name syntax (backward compatible)
func (h *ClientDetectionRuleHandler) testExpressionWithHeaders(expr, userAgent string, headers map[string]string) (bool, error) {
	// Parse the expression
	parsed, err := expression.Parse(expr)
	if err != nil {
		return false, err
	}

	// Create a base accessor with standard fields (no user_agent - that's now dynamic only)
	baseAccessor := &mockRequestAccessor{
		clientIP: "127.0.0.1", // Default for testing
	}

	// Build http.Header from the provided headers map
	httpHeaders := make(http.Header)
	if userAgent != "" {
		httpHeaders.Set("User-Agent", userAgent)
	}
	for key, value := range headers {
		httpHeaders.Set(key, value)
	}

	// Set up DynamicContext for @dynamic(path):key resolution
	dynCtx := expression.NewDynamicContext()
	dynCtx.SetRequestHeaders(httpHeaders)

	// Create registry with unified context for @dynamic() syntax
	registry := expression.NewDynamicFieldRegistryWithContext(dynCtx)

	// Evaluate using the evaluator with dynamic fields support
	evaluator := expression.NewEvaluator()
	result, err := evaluator.EvaluateWithDynamicFields(parsed, baseAccessor, registry)
	if err != nil {
		return false, err
	}

	return result.Matches, nil
}

// mockRequestAccessor implements expression.FieldValueAccessor for testing.
// Provides only static/computed fields - header-based fields use @dynamic(request.headers):
type mockRequestAccessor struct {
	clientIP string
}

func (m *mockRequestAccessor) GetFieldValue(name string) (string, bool) {
	switch name {
	case "client_ip", "ip", "remote_addr":
		return m.clientIP, true
	default:
		return "", false
	}
}
