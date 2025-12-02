package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
	"gorm.io/gorm"
)

// StreamProxyHandler handles stream proxy API endpoints.
type StreamProxyHandler struct {
	proxyService *service.ProxyService
}

// NewStreamProxyHandler creates a new stream proxy handler.
func NewStreamProxyHandler(proxyService *service.ProxyService) *StreamProxyHandler {
	return &StreamProxyHandler{
		proxyService: proxyService,
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
		OperationID: "generateProxy",
		Method:      "POST",
		Path:        "/api/v1/proxies/{id}/generate",
		Summary:     "Generate proxy output",
		Description: "Triggers generation for a stream proxy",
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
		resp.Body.Proxies = append(resp.Body.Proxies, StreamProxyFromModel(p))
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
		Body: StreamProxyDetailFromModel(proxy),
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

	// Set sources if provided
	if len(input.Body.SourceIDs) > 0 {
		if err := h.proxyService.SetSources(ctx, proxy.ID, input.Body.SourceIDs, nil); err != nil {
			return nil, huma.Error500InternalServerError("failed to set sources", err)
		}
	}

	// Set EPG sources if provided
	if len(input.Body.EpgSourceIDs) > 0 {
		if err := h.proxyService.SetEpgSources(ctx, proxy.ID, input.Body.EpgSourceIDs, nil); err != nil {
			return nil, huma.Error500InternalServerError("failed to set EPG sources", err)
		}
	}

	return &CreateStreamProxyOutput{
		Body: StreamProxyFromModel(proxy),
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

	return &UpdateStreamProxyOutput{
		Body: StreamProxyFromModel(proxy),
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
func (h *StreamProxyHandler) Generate(ctx context.Context, input *GenerateProxyInput) (*GenerateProxyOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	result, err := h.proxyService.Generate(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate proxy output", err)
	}

	return &GenerateProxyOutput{
		Body: struct {
			Message      string `json:"message"`
			ChannelCount int    `json:"channel_count"`
			ProgramCount int    `json:"program_count"`
			Duration     string `json:"duration"`
		}{
			Message:      fmt.Sprintf("generation completed for proxy %s", input.ID),
			ChannelCount: result.ChannelCount,
			ProgramCount: result.ProgramCount,
			Duration:     result.Duration.String(),
		},
	}, nil
}
