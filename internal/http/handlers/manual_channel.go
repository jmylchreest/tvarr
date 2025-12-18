// Package handlers provides HTTP API handlers for tvarr.
package handlers

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
)

// ManualChannelHandler handles manual channel API endpoints.
type ManualChannelHandler struct {
	channelService service.ManualChannelServiceInterface
}

// NewManualChannelHandler creates a new manual channel handler.
func NewManualChannelHandler(channelService service.ManualChannelServiceInterface) *ManualChannelHandler {
	return &ManualChannelHandler{
		channelService: channelService,
	}
}

// Register registers the manual channel routes with the API.
func (h *ManualChannelHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listManualChannels",
		Method:      "GET",
		Path:        "/api/v1/sources/stream/{source_id}/manual-channels",
		Summary:     "List manual channels",
		Description: "Returns all manual channels for a manual stream source",
		Tags:        []string{"Manual Channels"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "replaceManualChannels",
		Method:      "PUT",
		Path:        "/api/v1/sources/stream/{source_id}/manual-channels",
		Summary:     "Replace manual channels",
		Description: "Atomically replaces all manual channels for a manual stream source",
		Tags:        []string{"Manual Channels"},
	}, h.Replace)

	huma.Register(api, huma.Operation{
		OperationID: "importM3U",
		Method:      "POST",
		Path:        "/api/v1/sources/stream/{source_id}/manual-channels/import-m3u",
		Summary:     "Import channels from M3U",
		Description: "Parse M3U content and optionally apply to the source. Use apply=false for preview, apply=true to persist.",
		Tags:        []string{"Manual Channels"},
	}, h.ImportM3U)

	huma.Register(api, huma.Operation{
		OperationID: "exportM3U",
		Method:      "GET",
		Path:        "/api/v1/sources/stream/{source_id}/manual-channels/export.m3u",
		Summary:     "Export channels as M3U",
		Description: "Generate M3U playlist from manual channel definitions",
		Tags:        []string{"Manual Channels"},
	}, h.ExportM3U)
}

// ListManualChannelsInput is the input for listing manual channels.
type ListManualChannelsInput struct {
	SourceID string `path:"source_id" doc:"Stream source ID (ULID) - must be a manual source"`
}

// ListManualChannelsOutput is the output for listing manual channels.
type ListManualChannelsOutput struct {
	Body struct {
		Items []ManualChannelResponse `json:"items"`
		Total int                     `json:"total"`
	}
}

// List returns all manual channels for a source.
func (h *ManualChannelHandler) List(ctx context.Context, input *ListManualChannelsInput) (*ListManualChannelsOutput, error) {
	sourceID, err := models.ParseULID(input.SourceID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid source ID format", err)
	}

	channels, err := h.channelService.ListBySourceID(ctx, sourceID)
	if err != nil {
		// Check for specific error types
		errMsg := err.Error()
		if strings.Contains(errMsg, "source not found") {
			return nil, huma.Error404NotFound(fmt.Sprintf("source %s not found", input.SourceID))
		}
		if strings.Contains(errMsg, "only valid for manual sources") {
			return nil, huma.Error400BadRequest("operation only valid for manual sources")
		}
		return nil, huma.Error500InternalServerError("failed to list channels", err)
	}

	resp := &ListManualChannelsOutput{}
	resp.Body.Items = make([]ManualChannelResponse, 0, len(channels))
	for _, ch := range channels {
		resp.Body.Items = append(resp.Body.Items, ManualChannelFromModel(ch))
	}
	resp.Body.Total = len(channels)

	return resp, nil
}

// ReplaceManualChannelsInput is the input for replacing manual channels.
type ReplaceManualChannelsInput struct {
	SourceID string                       `path:"source_id" doc:"Stream source ID (ULID) - must be a manual source"`
	Body     ReplaceManualChannelsRequest `doc:"List of channels to replace existing channels"`
}

// ReplaceManualChannelsOutput is the output for replacing manual channels.
type ReplaceManualChannelsOutput struct {
	Body struct {
		Items []ManualChannelResponse `json:"items"`
		Total int                     `json:"total"`
	}
}

// Replace atomically replaces all manual channels for a source.
func (h *ManualChannelHandler) Replace(ctx context.Context, input *ReplaceManualChannelsInput) (*ReplaceManualChannelsOutput, error) {
	sourceID, err := models.ParseULID(input.SourceID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid source ID format", err)
	}

	// Convert input channels to models
	channels := make([]*models.ManualStreamChannel, 0, len(input.Body.Channels))
	for _, ch := range input.Body.Channels {
		channels = append(channels, ch.ToModel(sourceID))
	}

	// Call service to replace channels
	result, err := h.channelService.ReplaceChannels(ctx, sourceID, channels)
	if err != nil {
		// Check for specific error types
		errMsg := err.Error()
		if strings.Contains(errMsg, "source not found") {
			return nil, huma.Error404NotFound(fmt.Sprintf("source %s not found", input.SourceID))
		}
		if strings.Contains(errMsg, "only valid for manual sources") {
			return nil, huma.Error400BadRequest("operation only valid for manual sources")
		}
		if strings.Contains(errMsg, "at least one channel is required") {
			return nil, huma.Error400BadRequest("at least one channel is required")
		}
		// Validation errors (channel name, stream URL, logo format)
		if strings.Contains(errMsg, "channel") && (strings.Contains(errMsg, "required") || strings.Contains(errMsg, "must be") || strings.Contains(errMsg, "invalid")) {
			return nil, huma.Error400BadRequest(errMsg)
		}
		return nil, huma.Error500InternalServerError("failed to replace channels", err)
	}

	resp := &ReplaceManualChannelsOutput{}
	resp.Body.Items = make([]ManualChannelResponse, 0, len(result))
	for _, ch := range result {
		resp.Body.Items = append(resp.Body.Items, ManualChannelFromModel(ch))
	}
	resp.Body.Total = len(result)

	return resp, nil
}

// ImportM3UInput is the input for importing M3U content.
type ImportM3UInput struct {
	SourceID string `path:"source_id" doc:"Stream source ID (ULID) - must be a manual source"`
	Apply    bool   `query:"apply" default:"false" doc:"Whether to persist the imported channels"`
	RawBody  []byte
}

// ImportM3UOutput is the output for M3U import.
type ImportM3UOutput struct {
	Body M3UImportResultResponse
}

// M3UImportResultResponse is the response format for M3U import.
type M3UImportResultResponse struct {
	ParsedCount  int                     `json:"parsed_count"`
	SkippedCount int                     `json:"skipped_count"`
	Applied      bool                    `json:"applied"`
	Channels     []ManualChannelResponse `json:"channels"`
	Errors       []string                `json:"errors,omitempty"`
}

// ImportM3U imports M3U content to a manual source.
func (h *ManualChannelHandler) ImportM3U(ctx context.Context, input *ImportM3UInput) (*ImportM3UOutput, error) {
	sourceID, err := models.ParseULID(input.SourceID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid source ID format", err)
	}

	m3uContent := string(input.RawBody)
	if strings.TrimSpace(m3uContent) == "" {
		return nil, huma.Error400BadRequest("M3U content is required")
	}

	result, err := h.channelService.ImportM3U(ctx, sourceID, m3uContent, input.Apply)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "source not found") {
			return nil, huma.Error404NotFound(fmt.Sprintf("source %s not found", input.SourceID))
		}
		if strings.Contains(errMsg, "only valid for manual sources") {
			return nil, huma.Error400BadRequest("operation only valid for manual sources")
		}
		if strings.Contains(errMsg, "no valid channels") {
			return nil, huma.Error400BadRequest("no valid channels to import")
		}
		return nil, huma.Error500InternalServerError("failed to import M3U", err)
	}

	resp := &ImportM3UOutput{}
	resp.Body.ParsedCount = result.ParsedCount
	resp.Body.SkippedCount = result.SkippedCount
	resp.Body.Applied = result.Applied
	resp.Body.Errors = result.Errors
	resp.Body.Channels = make([]ManualChannelResponse, 0, len(result.Channels))
	for _, ch := range result.Channels {
		resp.Body.Channels = append(resp.Body.Channels, ManualChannelFromModel(ch))
	}

	return resp, nil
}

// ExportM3UInput is the input for exporting M3U content.
type ExportM3UInput struct {
	SourceID string `path:"source_id" doc:"Stream source ID (ULID) - must be a manual source"`
}

// ExportM3UOutput is the output for M3U export.
type ExportM3UOutput struct {
	Body        io.Reader
	ContentType string `header:"Content-Type"`
}

// ExportM3U exports manual channels as M3U playlist.
func (h *ManualChannelHandler) ExportM3U(ctx context.Context, input *ExportM3UInput) (*ExportM3UOutput, error) {
	sourceID, err := models.ParseULID(input.SourceID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid source ID format", err)
	}

	m3uContent, err := h.channelService.ExportM3U(ctx, sourceID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "source not found") {
			return nil, huma.Error404NotFound(fmt.Sprintf("source %s not found", input.SourceID))
		}
		if strings.Contains(errMsg, "only valid for manual sources") {
			return nil, huma.Error400BadRequest("operation only valid for manual sources")
		}
		return nil, huma.Error500InternalServerError("failed to export M3U", err)
	}

	return &ExportM3UOutput{
		Body:        strings.NewReader(m3uContent),
		ContentType: "audio/x-mpegurl",
	}, nil
}
