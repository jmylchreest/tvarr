package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/service"
)

// UnifiedSourcesHandler handles the unified sources endpoint.
type UnifiedSourcesHandler struct {
	sourceService *service.SourceService
	epgService    *service.EpgService
}

// NewUnifiedSourcesHandler creates a new unified sources handler.
func NewUnifiedSourcesHandler(sourceService *service.SourceService, epgService *service.EpgService) *UnifiedSourcesHandler {
	return &UnifiedSourcesHandler{
		sourceService: sourceService,
		epgService:    epgService,
	}
}

// Register registers the unified sources route with the API.
func (h *UnifiedSourcesHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listAllSources",
		Method:      "GET",
		Path:        "/api/v1/sources",
		Summary:     "List all sources",
		Description: "Returns all stream and EPG sources combined",
		Tags:        []string{"Sources"},
	}, h.List)
}

// UnifiedSourceResponse represents a source in the unified response.
type UnifiedSourceResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SourceKind      string `json:"source_kind"` // "stream" or "epg"
	SourceType      string `json:"source_type"` // "m3u", "xtream", "xmltv", etc.
	URL             string `json:"url,omitempty"`
	Enabled         bool   `json:"enabled"`
	Status          string `json:"status"`
	ChannelCount    int    `json:"channel_count"`
	ProgramCount    int    `json:"program_count,omitempty"` // Only for EPG sources
	LastIngestionAt string `json:"last_ingestion_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// UnifiedSourcesOutput is the output for listing all sources.
type UnifiedSourcesOutput struct {
	Body []UnifiedSourceResponse
}

// List returns all stream and EPG sources combined.
func (h *UnifiedSourcesHandler) List(ctx context.Context, input *struct{}) (*UnifiedSourcesOutput, error) {
	resp := &UnifiedSourcesOutput{
		Body: make([]UnifiedSourceResponse, 0),
	}

	// Get stream sources
	streamSources, err := h.sourceService.List(ctx)
	if err == nil {
		for _, s := range streamSources {
			lastIngestion := ""
			if s.LastIngestionAt != nil {
				lastIngestion = s.LastIngestionAt.Format("2006-01-02T15:04:05Z")
			}

			resp.Body = append(resp.Body, UnifiedSourceResponse{
				ID:              s.ID.String(),
				Name:            s.Name,
				SourceKind:      "stream",
				SourceType:      string(s.Type),
				URL:             s.URL,
				Enabled:         s.Enabled,
				Status:          string(s.Status),
				ChannelCount:    s.ChannelCount,
				LastIngestionAt: lastIngestion,
				CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:       s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}
	}

	// Get EPG sources
	epgSources, err := h.epgService.List(ctx)
	if err == nil {
		for _, s := range epgSources {
			lastIngestion := ""
			if s.LastIngestionAt != nil {
				lastIngestion = s.LastIngestionAt.Format("2006-01-02T15:04:05Z")
			}

			resp.Body = append(resp.Body, UnifiedSourceResponse{
				ID:              s.ID.String(),
				Name:            s.Name,
				SourceKind:      "epg",
				SourceType:      string(s.Type),
				URL:             s.URL,
				Enabled:         s.Enabled,
				Status:          string(s.Status),
				ChannelCount:    0, // EPG sources don't have channel count
				ProgramCount:    s.ProgramCount,
				LastIngestionAt: lastIngestion,
				CreatedAt:       s.CreatedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:       s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}
	}

	return resp, nil
}
