package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/spf13/viper"
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

	huma.Register(api, huma.Operation{
		OperationID: "getStartupConfig",
		Method:      "GET",
		Path:        "/api/v1/settings/startup",
		Summary:     "Get startup configuration",
		Description: "Returns read-only startup configuration (requires restart to change)",
		Tags:        []string{"Settings"},
	}, h.GetStartupConfig)
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
	Default     any             `json:"default"`
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

// GetStartupConfigInput is the input for getting startup config.
type GetStartupConfigInput struct{}

// StartupConfigSection represents a section of startup configuration.
type StartupConfigSection struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Settings    []StartupConfigSetting `json:"settings"`
}

// StartupConfigSetting represents a single startup configuration setting.
type StartupConfigSetting struct {
	Key         string `json:"key"`
	Value       any    `json:"value"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// GetStartupConfigOutput is the output for getting startup config.
type GetStartupConfigOutput struct {
	Body struct {
		Success   bool                   `json:"success"`
		Message   string                 `json:"message"`
		Sections  []StartupConfigSection `json:"sections"`
		Timestamp string                 `json:"timestamp"`
	}
}

// GetStartupConfig returns read-only startup configuration.
// These settings require a restart to change.
func (h *SettingsHandler) GetStartupConfig(ctx context.Context, input *GetStartupConfigInput) (*GetStartupConfigOutput, error) {
	resp := &GetStartupConfigOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Startup configuration retrieved (read-only, requires restart to change)"
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)

	// Pipeline/Logo settings
	pipelineSection := StartupConfigSection{
		Name:        "Pipeline",
		Description: "Logo caching and pipeline processing configuration",
		Settings: []StartupConfigSetting{
			{
				Key:         "pipeline.logo_concurrency",
				Value:       viper.GetInt("pipeline.logo_concurrency"),
				Type:        "integer",
				Description: "Number of concurrent logo downloads",
			},
			{
				Key:         "pipeline.logo_timeout",
				Value:       viper.GetDuration("pipeline.logo_timeout").String(),
				Type:        "duration",
				Description: "Timeout for individual logo downloads",
			},
			{
				Key:         "pipeline.logo_retry_attempts",
				Value:       viper.GetInt("pipeline.logo_retry_attempts"),
				Type:        "integer",
				Description: "Number of retry attempts for failed logo downloads",
			},
			{
				Key:         "pipeline.logo_circuit_breaker",
				Value:       viper.GetString("pipeline.logo_circuit_breaker"),
				Type:        "string",
				Description: "Circuit breaker namespace for logo downloads",
			},
			{
				Key:         "pipeline.logo_batch_size",
				Value:       viper.GetInt("pipeline.logo_batch_size"),
				Type:        "integer",
				Description: "Number of logos to process per batch",
			},
			{
				Key:         "pipeline.stream_batch_size",
				Value:       viper.GetInt("pipeline.stream_batch_size"),
				Type:        "integer",
				Description: "Number of streams to process per batch",
			},
		},
	}

	// Relay settings
	relaySection := StartupConfigSection{
		Name:        "Relay",
		Description: "Stream relay configuration",
		Settings: []StartupConfigSetting{
			{
				Key:         "relay.enabled",
				Value:       viper.GetBool("relay.enabled"),
				Type:        "boolean",
				Description: "Enable stream relay functionality",
			},
			{
				Key:         "relay.max_concurrent_streams",
				Value:       viper.GetInt("relay.max_concurrent_streams"),
				Type:        "integer",
				Description: "Maximum number of concurrent relay streams",
			},
			{
				Key:         "relay.circuit_breaker_threshold",
				Value:       viper.GetInt("relay.circuit_breaker_threshold"),
				Type:        "integer",
				Description: "Failures before circuit breaker opens",
			},
			{
				Key:         "relay.circuit_breaker_timeout",
				Value:       viper.GetDuration("relay.circuit_breaker_timeout").String(),
				Type:        "duration",
				Description: "Circuit breaker reset timeout",
			},
			{
				Key:         "relay.stream_timeout",
				Value:       viper.GetDuration("relay.stream_timeout").String(),
				Type:        "duration",
				Description: "Timeout for individual stream connections",
			},
		},
	}

	// Ingestion settings
	ingestionSection := StartupConfigSection{
		Name:        "Ingestion",
		Description: "Source ingestion configuration",
		Settings: []StartupConfigSetting{
			{
				Key:         "ingestion.http_timeout",
				Value:       viper.GetDuration("ingestion.http_timeout").String(),
				Type:        "duration",
				Description: "HTTP timeout for source fetching",
			},
			{
				Key:         "ingestion.max_concurrent",
				Value:       viper.GetInt("ingestion.max_concurrent"),
				Type:        "integer",
				Description: "Maximum concurrent ingestion operations",
			},
			{
				Key:         "ingestion.retry_attempts",
				Value:       viper.GetInt("ingestion.retry_attempts"),
				Type:        "integer",
				Description: "Number of retry attempts for failed ingestion",
			},
			{
				Key:         "ingestion.retry_delay",
				Value:       viper.GetDuration("ingestion.retry_delay").String(),
				Type:        "duration",
				Description: "Delay between retry attempts",
			},
		},
	}

	// Server settings
	serverSection := StartupConfigSection{
		Name:        "Server",
		Description: "HTTP server configuration",
		Settings: []StartupConfigSetting{
			{
				Key:         "server.host",
				Value:       viper.GetString("server.host"),
				Type:        "string",
				Description: "Server bind address",
			},
			{
				Key:         "server.port",
				Value:       viper.GetInt("server.port"),
				Type:        "integer",
				Description: "Server listen port",
			},
			{
				Key:         "server.read_timeout",
				Value:       viper.GetDuration("server.read_timeout").String(),
				Type:        "duration",
				Description: "HTTP read timeout",
			},
			{
				Key:         "server.write_timeout",
				Value:       viper.GetDuration("server.write_timeout").String(),
				Type:        "duration",
				Description: "HTTP write timeout",
			},
		},
	}

	// Storage settings
	storageSection := StartupConfigSection{
		Name:        "Storage",
		Description: "File storage configuration",
		Settings: []StartupConfigSetting{
			{
				Key:         "storage.base_dir",
				Value:       viper.GetString("storage.base_dir"),
				Type:        "string",
				Description: "Base directory for file storage",
			},
			{
				Key:         "storage.logo_retention",
				Value:       viper.GetDuration("storage.logo_retention").String(),
				Type:        "duration",
				Description: "Logo file retention period",
			},
			{
				Key:         "storage.max_logo_size",
				Value:       viper.GetInt64("storage.max_logo_size"),
				Type:        "integer",
				Description: "Maximum logo file size in bytes",
			},
		},
	}

	resp.Body.Sections = []StartupConfigSection{
		pipelineSection,
		relaySection,
		ingestionSection,
		serverSection,
		storageSection,
	}

	return resp, nil
}
