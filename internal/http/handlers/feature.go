package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// FeatureHandler handles feature flags API endpoints.
// Feature state is held in memory and reset on restart.
type FeatureHandler struct {
	mu     sync.RWMutex
	flags  map[string]bool
	config map[string]map[string]interface{}
}

// NewFeatureHandler creates a new feature handler with default flags.
func NewFeatureHandler() *FeatureHandler {
	return &FeatureHandler{
		flags: map[string]bool{
			"debug-frontend": false, // Controls frontend debug logging
			"feature-cache":  false, // Controls whether feature flags are cached by frontend
		},
		config: map[string]map[string]interface{}{
			// Cache configuration (only used when feature-cache is true)
			"feature-cache": {
				"cache-duration": 300000, // 5 minutes in milliseconds
			},
		},
	}
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
		Description: "Updates feature flags configuration at runtime (not persisted across restarts)",
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
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Copy current state to avoid data races
	flags := make(map[string]bool, len(h.flags))
	for k, v := range h.flags {
		flags[k] = v
	}

	config := make(map[string]map[string]interface{}, len(h.config))
	for k, v := range h.config {
		configCopy := make(map[string]interface{}, len(v))
		for ck, cv := range v {
			configCopy[ck] = cv
		}
		config[k] = configCopy
	}

	resp := &GetFeaturesOutput{}
	resp.Body.Success = true
	resp.Body.Data = FeaturesData{
		Flags:     flags,
		Config:    config,
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

// UpdateFeatures updates feature flags at runtime.
// Changes are held in memory and reset on restart.
func (h *FeatureHandler) UpdateFeatures(ctx context.Context, input *UpdateFeaturesInput) (*UpdateFeaturesOutput, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	flagsUpdated := 0
	configsUpdated := 0

	// Update flags
	for key, value := range input.Body.Flags {
		h.flags[key] = value
		flagsUpdated++
	}

	// Update config
	for key, value := range input.Body.Config {
		if h.config[key] == nil {
			h.config[key] = make(map[string]interface{})
		}
		for ck, cv := range value {
			h.config[key][ck] = cv
		}
		configsUpdated++
	}

	resp := &UpdateFeaturesOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Feature flags updated (changes are held in memory, reset on restart)"
	resp.Body.FlagsUpdated = flagsUpdated
	resp.Body.ConfigsUpdated = configsUpdated
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}
