package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/internal/urlutil"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// StreamProxyHandler handles stream proxy API endpoints.
type StreamProxyHandler struct {
	proxyService *service.ProxyService
	baseURL      string
	logger       *slog.Logger
}

// buildOrderMapFromIDs creates an order map from array indices.
// The order is derived from the position in the array (index 0 = order 0, etc.).
func buildOrderMapFromIDs(ids []models.ULID) map[models.ULID]int {
	if len(ids) == 0 {
		return nil
	}
	orders := make(map[models.ULID]int, len(ids))
	for i, id := range ids {
		orders[id] = i
	}
	return orders
}

// buildFilterMaps converts ProxyFilterAssignmentRequest slice to the maps needed by SetFilters.
// Returns: filterIDs slice, orders map (by priority_order), isActive map.
func buildFilterMaps(filters []ProxyFilterAssignmentRequest) ([]models.ULID, map[models.ULID]int, map[models.ULID]bool) {
	if len(filters) == 0 {
		return nil, nil, nil
	}
	filterIDs := make([]models.ULID, len(filters))
	orders := make(map[models.ULID]int, len(filters))
	isActive := make(map[models.ULID]bool, len(filters))

	for i, f := range filters {
		filterIDs[i] = f.FilterID
		orders[f.FilterID] = f.PriorityOrder
		isActive[f.FilterID] = f.IsActive
	}
	return filterIDs, orders, isActive
}

// NewStreamProxyHandler creates a new stream proxy handler.
func NewStreamProxyHandler(proxyService *service.ProxyService) *StreamProxyHandler {
	// Compute base URL from viper config (same logic as serve.go)
	baseURL := urlutil.NormalizeBaseURL(viper.GetString("server.base_url"))
	if baseURL == "" {
		serverHost := viper.GetString("server.host")
		serverPort := viper.GetInt("server.port")
		if serverHost == "0.0.0.0" || serverHost == "" {
			baseURL = fmt.Sprintf("http://localhost:%d", serverPort)
		} else {
			baseURL = fmt.Sprintf("http://%s:%d", serverHost, serverPort)
		}
	}

	return &StreamProxyHandler{
		proxyService: proxyService,
		baseURL:      baseURL,
		logger:       slog.Default(),
	}
}

// Register registers the stream proxy routes with the API.
func (h *StreamProxyHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listStreamProxies",
		Method:      "GET",
		Path:        "/api/v1/proxies",
		Summary:     "List stream proxies",
		Description: "Returns all stream proxies",
		Tags:        []string{"Stream Proxies"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getStreamProxy",
		Method:      "GET",
		Path:        "/api/v1/proxies/{id}",
		Summary:     "Get stream proxy",
		Description: "Returns a stream proxy by ID with its sources",
		Tags:        []string{"Stream Proxies"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createStreamProxy",
		Method:      "POST",
		Path:        "/api/v1/proxies",
		Summary:     "Create stream proxy",
		Description: "Creates a new stream proxy",
		Tags:        []string{"Stream Proxies"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateStreamProxy",
		Method:      "PUT",
		Path:        "/api/v1/proxies/{id}",
		Summary:     "Update stream proxy",
		Description: "Updates an existing stream proxy",
		Tags:        []string{"Stream Proxies"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteStreamProxy",
		Method:      "DELETE",
		Path:        "/api/v1/proxies/{id}",
		Summary:     "Delete stream proxy",
		Description: "Deletes a stream proxy",
		Tags:        []string{"Stream Proxies"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "setProxySources",
		Method:      "PUT",
		Path:        "/api/v1/proxies/{id}/sources",
		Summary:     "Set proxy sources",
		Description: "Sets the stream sources for a proxy",
		Tags:        []string{"Stream Proxies"},
	}, h.SetSources)

	huma.Register(api, huma.Operation{
		OperationID: "setProxyEpgSources",
		Method:      "PUT",
		Path:        "/api/v1/proxies/{id}/epg-sources",
		Summary:     "Set proxy EPG sources",
		Description: "Sets the EPG sources for a proxy",
		Tags:        []string{"Stream Proxies"},
	}, h.SetEpgSources)

	huma.Register(api, huma.Operation{
		OperationID: "regenerateProxy",
		Method:      "POST",
		Path:        "/api/v1/proxies/{id}/regenerate",
		Summary:     "Regenerate proxy output",
		Description: "Triggers regeneration for a stream proxy",
		Tags:        []string{"Stream Proxies"},
	}, h.Generate)
}

// ListStreamProxiesInput is the input for listing stream proxies.
type ListStreamProxiesInput struct{}

// ListStreamProxiesOutput is the output for listing stream proxies.
type ListStreamProxiesOutput struct {
	Body struct {
		Proxies []StreamProxyResponse `json:"proxies"`
	}
}

// List returns all stream proxies.
func (h *StreamProxyHandler) List(ctx context.Context, input *ListStreamProxiesInput) (*ListStreamProxiesOutput, error) {
	proxies, err := h.proxyService.GetAll(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list proxies", err)
	}

	resp := &ListStreamProxiesOutput{}
	resp.Body.Proxies = make([]StreamProxyResponse, 0, len(proxies))
	for _, p := range proxies {
		resp.Body.Proxies = append(resp.Body.Proxies, StreamProxyFromModel(p, h.baseURL))
	}

	return resp, nil
}

// GetStreamProxyInput is the input for getting a stream proxy.
type GetStreamProxyInput struct {
	ID string `path:"id" doc:"Stream proxy ID (ULID)"`
}

// GetStreamProxyOutput is the output for getting a stream proxy.
type GetStreamProxyOutput struct {
	Body StreamProxyDetailResponse
}

// GetByID returns a stream proxy by ID with its sources.
func (h *StreamProxyHandler) GetByID(ctx context.Context, input *GetStreamProxyInput) (*GetStreamProxyOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	proxy, err := h.proxyService.GetByIDWithRelations(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream proxy %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get proxy", err)
	}

	return &GetStreamProxyOutput{
		Body: StreamProxyDetailFromModel(proxy, h.baseURL),
	}, nil
}

// CreateStreamProxyInput is the input for creating a stream proxy.
type CreateStreamProxyInput struct {
	Body CreateStreamProxyRequest
}

// CreateStreamProxyOutput is the output for creating a stream proxy.
type CreateStreamProxyOutput struct {
	Body StreamProxyResponse
}

// Create creates a new stream proxy.
func (h *StreamProxyHandler) Create(ctx context.Context, input *CreateStreamProxyInput) (*CreateStreamProxyOutput, error) {
	proxy := input.Body.ToModel()

	if err := h.proxyService.Create(ctx, proxy); err != nil {
		return nil, huma.Error500InternalServerError("failed to create proxy", err)
	}

	// Set sources if provided (order derived from array index)
	if len(input.Body.SourceIDs) > 0 {
		priorities := buildOrderMapFromIDs(input.Body.SourceIDs)
		if err := h.proxyService.SetSources(ctx, proxy.ID, input.Body.SourceIDs, priorities); err != nil {
			return nil, huma.Error500InternalServerError("failed to set sources", err)
		}
	}

	// Set EPG sources if provided (order derived from array index)
	if len(input.Body.EpgSourceIDs) > 0 {
		priorities := buildOrderMapFromIDs(input.Body.EpgSourceIDs)
		if err := h.proxyService.SetEpgSources(ctx, proxy.ID, input.Body.EpgSourceIDs, priorities); err != nil {
			return nil, huma.Error500InternalServerError("failed to set EPG sources", err)
		}
	}

	// Set filters if provided
	if len(input.Body.Filters) > 0 {
		filterIDs, orders, isActive := buildFilterMaps(input.Body.Filters)
		if err := h.proxyService.SetFilters(ctx, proxy.ID, filterIDs, orders, isActive); err != nil {
			return nil, huma.Error500InternalServerError("failed to set filters", err)
		}
	} else if len(input.Body.FilterIDs) > 0 {
		// Backward compatibility: support legacy FilterIDs field (all active by default)
		orders := buildOrderMapFromIDs(input.Body.FilterIDs)
		if err := h.proxyService.SetFilters(ctx, proxy.ID, input.Body.FilterIDs, orders, nil); err != nil {
			return nil, huma.Error500InternalServerError("failed to set filters", err)
		}
	}

	return &CreateStreamProxyOutput{
		Body: StreamProxyFromModel(proxy, h.baseURL),
	}, nil
}

// UpdateStreamProxyInput is the input for updating a stream proxy.
type UpdateStreamProxyInput struct {
	ID   string `path:"id" doc:"Stream proxy ID (ULID)"`
	Body UpdateStreamProxyRequest
}

// UpdateStreamProxyOutput is the output for updating a stream proxy.
type UpdateStreamProxyOutput struct {
	Body StreamProxyResponse
}

// Update updates an existing stream proxy.
func (h *StreamProxyHandler) Update(ctx context.Context, input *UpdateStreamProxyInput) (*UpdateStreamProxyOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	proxy, err := h.proxyService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream proxy %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get proxy", err)
	}

	input.Body.ApplyToModel(proxy)

	if err := h.proxyService.Update(ctx, proxy); err != nil {
		return nil, huma.Error500InternalServerError("failed to update proxy", err)
	}

	// Set sources if provided (order derived from array index)
	if input.Body.SourceIDs != nil {
		priorities := buildOrderMapFromIDs(input.Body.SourceIDs)
		if err := h.proxyService.SetSources(ctx, id, input.Body.SourceIDs, priorities); err != nil {
			return nil, huma.Error500InternalServerError("failed to set sources", err)
		}
	}

	// Set EPG sources if provided (order derived from array index)
	if input.Body.EpgSourceIDs != nil {
		priorities := buildOrderMapFromIDs(input.Body.EpgSourceIDs)
		if err := h.proxyService.SetEpgSources(ctx, id, input.Body.EpgSourceIDs, priorities); err != nil {
			return nil, huma.Error500InternalServerError("failed to set EPG sources", err)
		}
	}

	// Set filters if provided
	if len(input.Body.Filters) > 0 {
		filterIDs, orders, isActive := buildFilterMaps(input.Body.Filters)
		if err := h.proxyService.SetFilters(ctx, id, filterIDs, orders, isActive); err != nil {
			return nil, huma.Error500InternalServerError("failed to set filters", err)
		}
	} else if input.Body.FilterIDs != nil {
		// Backward compatibility: support legacy FilterIDs field (all active by default)
		orders := buildOrderMapFromIDs(input.Body.FilterIDs)
		if err := h.proxyService.SetFilters(ctx, id, input.Body.FilterIDs, orders, nil); err != nil {
			return nil, huma.Error500InternalServerError("failed to set filters", err)
		}
	}

	return &UpdateStreamProxyOutput{
		Body: StreamProxyFromModel(proxy, h.baseURL),
	}, nil
}

// DeleteStreamProxyInput is the input for deleting a stream proxy.
type DeleteStreamProxyInput struct {
	ID string `path:"id" doc:"Stream proxy ID (ULID)"`
}

// DeleteStreamProxyOutput is the output for deleting a stream proxy.
type DeleteStreamProxyOutput struct{}

// Delete deletes a stream proxy.
func (h *StreamProxyHandler) Delete(ctx context.Context, input *DeleteStreamProxyInput) (*DeleteStreamProxyOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.proxyService.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream proxy %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to delete proxy", err)
	}

	return &DeleteStreamProxyOutput{}, nil
}

// SetProxySourcesInput is the input for setting proxy sources.
type SetProxySourcesInput struct {
	ID   string `path:"id" doc:"Stream proxy ID (ULID)"`
	Body SetProxySourcesRequest
}

// SetProxySourcesOutput is the output for setting proxy sources.
type SetProxySourcesOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// SetSources sets the stream sources for a proxy.
func (h *StreamProxyHandler) SetSources(ctx context.Context, input *SetProxySourcesInput) (*SetProxySourcesOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.proxyService.SetSources(ctx, id, input.Body.SourceIDs, input.Body.Priorities); err != nil {
		return nil, huma.Error500InternalServerError("failed to set sources", err)
	}

	return &SetProxySourcesOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("sources updated for proxy %s", input.ID),
		},
	}, nil
}

// SetProxyEpgSourcesInput is the input for setting proxy EPG sources.
type SetProxyEpgSourcesInput struct {
	ID   string `path:"id" doc:"Stream proxy ID (ULID)"`
	Body SetProxyEpgSourcesRequest
}

// SetProxyEpgSourcesOutput is the output for setting proxy EPG sources.
type SetProxyEpgSourcesOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// SetEpgSources sets the EPG sources for a proxy.
func (h *StreamProxyHandler) SetEpgSources(ctx context.Context, input *SetProxyEpgSourcesInput) (*SetProxyEpgSourcesOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.proxyService.SetEpgSources(ctx, id, input.Body.EpgSourceIDs, input.Body.Priorities); err != nil {
		return nil, huma.Error500InternalServerError("failed to set EPG sources", err)
	}

	return &SetProxyEpgSourcesOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("EPG sources updated for proxy %s", input.ID),
		},
	}, nil
}

// GenerateProxyInput is the input for triggering proxy generation.
type GenerateProxyInput struct {
	ID string `path:"id" doc:"Stream proxy ID (ULID)"`
}

// GenerateProxyOutput is the output for triggering proxy generation.
type GenerateProxyOutput struct {
	Body struct {
		Message      string `json:"message"`
		ChannelCount int    `json:"channel_count"`
		ProgramCount int    `json:"program_count"`
		Duration     string `json:"duration"`
	}
}

// Generate triggers generation for a stream proxy.
// This is an async operation - it starts generation in the background and returns immediately.
// Progress is tracked via the SSE /api/v1/progress endpoint.
func (h *StreamProxyHandler) Generate(ctx context.Context, input *GenerateProxyInput) (*GenerateProxyOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	// Check if proxy exists first
	proxy, err := h.proxyService.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("stream proxy %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get proxy", err)
	}
	if proxy == nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("stream proxy %s not found", input.ID))
	}

	// Capture proxy name for goroutine (avoid closure issues)
	proxyName := proxy.Name

	// Start generation in a goroutine - this is async.
	// Progress is tracked via the progress service SSE endpoint, not this request.
	go func() {
		// Use background context to avoid HTTP request cancellation
		_, err := h.proxyService.Generate(context.Background(), id)
		if err != nil {
			// Error is logged by the service layer and tracked in progress
			h.logger.Error("proxy generation failed",
				"proxy_id", id.String(),
				"proxy_name", proxyName,
				"error", err,
			)
		}
	}()

	return &GenerateProxyOutput{
		Body: struct {
			Message      string `json:"message"`
			ChannelCount int    `json:"channel_count"`
			ProgramCount int    `json:"program_count"`
			Duration     string `json:"duration"`
		}{
			Message:      fmt.Sprintf("generation started for proxy %s", input.ID),
			ChannelCount: 0, // Will be updated via SSE progress
			ProgramCount: 0,
			Duration:     "in progress",
		},
	}, nil
}
