package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// FeatureHandler handles feature flags API endpoints.
type FeatureHandler struct{}

// NewFeatureHandler creates a new feature handler.
func NewFeatureHandler() *FeatureHandler {
	return &FeatureHandler{}
}

// Register registers the feature routes with the API.
func (h *FeatureHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getFeatures",
		Method:      "GET",
		Path:        "/api/v1/features",
		Summary:     "Get feature flags",
		Description: "Returns current feature flags and configuration",
		Tags:        []string{"Features"},
	}, h.GetFeatures)

	huma.Register(api, huma.Operation{
		OperationID: "updateFeatures",
		Method:      "PUT",
		Path:        "/api/v1/features",
		Summary:     "Update feature flags",
		Description: "Updates feature flags configuration at runtime",
		Tags:        []string{"Features"},
	}, h.UpdateFeatures)
}

// FeaturesData represents the feature flags data.
type FeaturesData struct {
	Flags     map[string]bool                   `json:"flags"`
	Config    map[string]map[string]interface{} `json:"config"`
	Timestamp string                            `json:"timestamp"`
}

// GetFeaturesInput is the input for getting feature flags.
type GetFeaturesInput struct{}

// GetFeaturesOutput is the output for getting feature flags.
type GetFeaturesOutput struct {
	Body struct {
		Success bool         `json:"success"`
		Data    FeaturesData `json:"data"`
	}
}

// GetFeatures returns current feature flags.
func (h *FeatureHandler) GetFeatures(ctx context.Context, input *GetFeaturesInput) (*GetFeaturesOutput, error) {
	// Return default feature flags matching m3u-proxy defaults
	// These are the known features that the frontend expects
	resp := &GetFeaturesOutput{}
	resp.Body.Success = true
	resp.Body.Data = FeaturesData{
		Flags: map[string]bool{
			"debug-frontend": false, // Controls frontend debug logging
			"feature-cache":  false, // Controls whether feature flags are cached by frontend
		},
		Config: map[string]map[string]interface{}{
			// Cache configuration (only used when feature-cache is true)
			"feature-cache": {
				"cache-duration": 300000, // 5 minutes in milliseconds
			},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	return resp, nil
}

// UpdateFeaturesInput is the input for updating feature flags.
type UpdateFeaturesInput struct {
	Body struct {
		Flags  map[string]bool                   `json:"flags"`
		Config map[string]map[string]interface{} `json:"config"`
	}
}

// UpdateFeaturesOutput is the output for updating feature flags.
type UpdateFeaturesOutput struct {
	Body struct {
		Success        bool   `json:"success"`
		Message        string `json:"message"`
		FlagsUpdated   int    `json:"flags_updated"`
		ConfigsUpdated int    `json:"configs_updated"`
		Timestamp      string `json:"timestamp"`
	}
}

// UpdateFeatures updates feature flags (stub implementation).
func (h *FeatureHandler) UpdateFeatures(ctx context.Context, input *UpdateFeaturesInput) (*UpdateFeaturesOutput, error) {
	// Stub implementation - in a full implementation, these would be persisted
	resp := &UpdateFeaturesOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Feature flags updated (note: changes are not persisted in this implementation)"
	resp.Body.FlagsUpdated = len(input.Body.Flags)
	resp.Body.ConfigsUpdated = len(input.Body.Config)
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}
