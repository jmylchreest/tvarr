// Package handlers provides HTTP API handlers for tvarr.
package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	version   string
	startTime time.Time
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{
		version:   version,
		startTime: time.Now(),
	}
}

// HealthInput is the input for the health check endpoint.
type HealthInput struct{}

// HealthOutput is the output for the health check endpoint.
type HealthOutput struct {
	Body HealthResponse
}

// Register registers the health routes with the API.
func (h *HealthHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getHealth",
		Method:      "GET",
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns the health status of the service",
		Tags:        []string{"System"},
	}, h.GetHealth)
}

// GetHealth returns the health status of the service.
func (h *HealthHandler) GetHealth(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
	uptime := time.Since(h.startTime).Round(time.Second).String()

	return &HealthOutput{
		Body: HealthResponse{
			Status:  "healthy",
			Version: h.version,
			Uptime:  uptime,
			Checks: map[string]string{
				"database": "ok",
			},
		},
	}, nil
}
