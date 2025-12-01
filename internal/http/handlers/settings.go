package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/observability"
)

// SettingsHandler handles settings API endpoints.
type SettingsHandler struct{}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
}

// Register registers the settings routes with the API.
func (h *SettingsHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getSettings",
		Method:      "GET",
		Path:        "/api/v1/settings",
		Summary:     "Get runtime settings",
		Description: "Returns current runtime settings",
		Tags:        []string{"Settings"},
	}, h.GetSettings)

	huma.Register(api, huma.Operation{
		OperationID: "updateSettings",
		Method:      "PUT",
		Path:        "/api/v1/settings",
		Summary:     "Update runtime settings",
		Description: "Updates runtime settings configuration",
		Tags:        []string{"Settings"},
	}, h.UpdateSettings)

	huma.Register(api, huma.Operation{
		OperationID: "getSettingsInfo",
		Method:      "GET",
		Path:        "/api/v1/settings/info",
		Summary:     "Get settings metadata",
		Description: "Returns metadata about available settings",
		Tags:        []string{"Settings"},
	}, h.GetSettingsInfo)
}

// RuntimeSettings represents the runtime settings data.
type RuntimeSettings struct {
	LogLevel             string `json:"log_level"`
	EnableRequestLogging bool   `json:"enable_request_logging"`
}

// GetSettingsInput is the input for getting settings.
type GetSettingsInput struct{}

// GetSettingsOutput is the output for getting settings.
type GetSettingsOutput struct {
	Body struct {
		Success        bool            `json:"success"`
		Message        string          `json:"message"`
		Settings       RuntimeSettings `json:"settings"`
		AppliedChanges []string        `json:"applied_changes"`
	}
}

// GetSettings returns current runtime settings.
func (h *SettingsHandler) GetSettings(ctx context.Context, input *GetSettingsInput) (*GetSettingsOutput, error) {
	resp := &GetSettingsOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Settings retrieved"
	resp.Body.Settings = RuntimeSettings{
		LogLevel:             observability.GetLogLevel(),
		EnableRequestLogging: observability.IsRequestLoggingEnabled(),
	}
	resp.Body.AppliedChanges = []string{}
	return resp, nil
}

// UpdateSettingsInput is the input for updating settings.
type UpdateSettingsInput struct {
	Body struct {
		LogLevel             *string `json:"log_level,omitempty"`
		EnableRequestLogging *bool   `json:"enable_request_logging,omitempty"`
	}
}

// UpdateSettingsOutput is the output for updating settings.
type UpdateSettingsOutput struct {
	Body struct {
		Success        bool            `json:"success"`
		Message        string          `json:"message"`
		Settings       RuntimeSettings `json:"settings"`
		AppliedChanges []string        `json:"applied_changes"`
	}
}

// UpdateSettings updates runtime settings.
// Log level changes take effect immediately for all loggers using GlobalLogLevel.
func (h *SettingsHandler) UpdateSettings(ctx context.Context, input *UpdateSettingsInput) (*UpdateSettingsOutput, error) {
	appliedChanges := []string{}

	// Apply log level change if provided
	if input.Body.LogLevel != nil {
		observability.SetLogLevel(*input.Body.LogLevel)
		appliedChanges = append(appliedChanges, "log_level")
	}

	// Apply request logging change if provided
	if input.Body.EnableRequestLogging != nil {
		observability.SetRequestLogging(*input.Body.EnableRequestLogging)
		appliedChanges = append(appliedChanges, "enable_request_logging")
	}

	resp := &UpdateSettingsOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Settings updated successfully"
	resp.Body.Settings = RuntimeSettings{
		LogLevel:             observability.GetLogLevel(),
		EnableRequestLogging: observability.IsRequestLoggingEnabled(),
	}
	resp.Body.AppliedChanges = appliedChanges
	return resp, nil
}

// GetSettingsInfoInput is the input for getting settings info.
type GetSettingsInfoInput struct{}

// SettingOption represents an option for a setting field.
type SettingOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// SettingField represents metadata about a setting field.
type SettingField struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Default     interface{}     `json:"default"`
	Options     []SettingOption `json:"options,omitempty"`
}

// GetSettingsInfoOutput is the output for getting settings info.
type GetSettingsInfoOutput struct {
	Body struct {
		Fields    []SettingField `json:"fields"`
		Version   string         `json:"version"`
		Timestamp string         `json:"timestamp"`
	}
}

// GetSettingsInfo returns metadata about available settings.
func (h *SettingsHandler) GetSettingsInfo(ctx context.Context, input *GetSettingsInfoInput) (*GetSettingsInfoOutput, error) {
	resp := &GetSettingsInfoOutput{}
	resp.Body.Fields = []SettingField{
		{
			Name:        "log_level",
			Type:        "select",
			Description: "Logging verbosity level",
			Default:     "info",
			Options: []SettingOption{
				{Value: "trace", Label: "Trace", Description: "Most verbose logging"},
				{Value: "debug", Label: "Debug", Description: "Debug level logging"},
				{Value: "info", Label: "Info", Description: "Standard logging"},
				{Value: "warn", Label: "Warning", Description: "Warnings and errors only"},
				{Value: "error", Label: "Error", Description: "Errors only"},
			},
		},
		{
			Name:        "enable_request_logging",
			Type:        "boolean",
			Description: "Enable logging of HTTP requests",
			Default:     false,
		},
	}
	resp.Body.Version = "1.0.0"
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}
