package handlers

import (
	"context"
	"maps"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/pkg/bytesize"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/spf13/viper"
)

// ConfigHandler handles unified configuration API endpoints.
type ConfigHandler struct {
	cbManager      *httpclient.CircuitBreakerManager
	featureHandler *FeatureHandler
}

// NewConfigHandler creates a new unified config handler.
func NewConfigHandler(cbManager *httpclient.CircuitBreakerManager, featureHandler *FeatureHandler) *ConfigHandler {
	if cbManager == nil {
		cbManager = httpclient.DefaultManager
	}
	return &ConfigHandler{
		cbManager:      cbManager,
		featureHandler: featureHandler,
	}
}

// Register registers the config routes with the API.
func (h *ConfigHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getConfig",
		Method:      "GET",
		Path:        "/api/v1/config",
		Summary:     "Get unified configuration",
		Description: "Returns all configuration data including runtime settings, feature flags, circuit breaker config, and startup config",
		Tags:        []string{"Configuration"},
	}, h.GetConfig)

	huma.Register(api, huma.Operation{
		OperationID: "updateConfig",
		Method:      "PUT",
		Path:        "/api/v1/config",
		Summary:     "Update runtime configuration",
		Description: "Updates runtime-modifiable configuration. Omitted fields are not modified.",
		Tags:        []string{"Configuration"},
	}, h.UpdateConfig)

	huma.Register(api, huma.Operation{
		OperationID: "persistConfig",
		Method:      "POST",
		Path:        "/api/v1/config/persist",
		Summary:     "Save configuration to file",
		Description: "Persists current runtime configuration to the config file",
		Tags:        []string{"Configuration"},
	}, h.PersistConfig)
}

// UnifiedConfigInput is the input for getting unified config.
type UnifiedConfigInput struct{}

// UnifiedConfigOutput is the output for getting unified config.
type UnifiedConfigOutput struct {
	Body UnifiedConfigResponse
}

// GetConfig returns unified configuration.
func (h *ConfigHandler) GetConfig(ctx context.Context, input *UnifiedConfigInput) (*UnifiedConfigOutput, error) {
	// Build runtime config
	runtime := RuntimeConfig{
		Settings: ConfigRuntimeSettings{
			LogLevel:             observability.GetLogLevel(),
			EnableRequestLogging: observability.IsRequestLoggingEnabled(),
		},
		Features:        h.getFeatures(),
		FeatureConfig:   h.getFeatureConfig(),
		CircuitBreakers: h.getCircuitBreakerConfig(),
	}

	// Build startup config
	startup := h.getStartupConfig()

	// Build meta
	meta := h.getConfigMeta()

	return &UnifiedConfigOutput{
		Body: UnifiedConfigResponse{
			Success: true,
			Runtime: runtime,
			Startup: startup,
			Meta:    meta,
		},
	}, nil
}

// UnifiedConfigUpdateInput is the input for updating config.
type UnifiedConfigUpdateInput struct {
	Body UnifiedConfigUpdate
}

// UnifiedConfigUpdateOutput is the output for updating config.
type UnifiedConfigUpdateOutput struct {
	Body ConfigUpdateResponse
}

// UpdateConfig updates runtime configuration.
func (h *ConfigHandler) UpdateConfig(ctx context.Context, input *UnifiedConfigUpdateInput) (*UnifiedConfigUpdateOutput, error) {
	appliedChanges := []string{}

	// Update settings if provided
	if input.Body.Settings != nil {
		if input.Body.Settings.LogLevel != "" {
			oldLevel := observability.GetLogLevel()
			observability.SetLogLevel(input.Body.Settings.LogLevel)
			appliedChanges = append(appliedChanges, "log_level: "+oldLevel+" -> "+input.Body.Settings.LogLevel)
		}

		// Always apply enable_request_logging if settings block is provided
		oldLogging := observability.IsRequestLoggingEnabled()
		observability.SetRequestLogging(input.Body.Settings.EnableRequestLogging)
		if oldLogging != input.Body.Settings.EnableRequestLogging {
			appliedChanges = append(appliedChanges, "enable_request_logging: changed")
		}
	}

	// Update features if provided
	if input.Body.Features != nil {
		h.updateFeatures(input.Body.Features)
		for key, value := range input.Body.Features {
			if value {
				appliedChanges = append(appliedChanges, "features."+key+": true")
			} else {
				appliedChanges = append(appliedChanges, "features."+key+": false")
			}
		}
	}

	// Update circuit breaker config if provided
	if input.Body.CircuitBreakers != nil {
		if input.Body.CircuitBreakers.Global != nil {
			globalCfg, err := configFromProfile(*input.Body.CircuitBreakers.Global)
			if err != nil {
				return nil, err
			}
			h.cbManager.UpdateGlobalConfig(globalCfg)
			appliedChanges = append(appliedChanges, "circuit_breakers.global: updated")
		}

		for name, profile := range input.Body.CircuitBreakers.Profiles {
			cfg, err := configFromProfile(profile)
			if err != nil {
				return nil, err
			}
			h.cbManager.UpdateServiceConfig(name, cfg)
			appliedChanges = append(appliedChanges, "circuit_breakers.profiles."+name+": updated")
		}
	}

	return &UnifiedConfigUpdateOutput{
		Body: ConfigUpdateResponse{
			Success:        true,
			Message:        "Configuration updated successfully",
			AppliedChanges: appliedChanges,
		},
	}, nil
}

// PersistConfigInput is the input for persisting config.
type PersistConfigInput struct{}

// PersistConfigOutput is the output for persisting config.
type PersistConfigOutput struct {
	Body ConfigPersistResponse
}

// PersistConfig saves configuration to file.
func (h *ConfigHandler) PersistConfig(ctx context.Context, input *PersistConfigInput) (*PersistConfigOutput, error) {
	configPath := viper.ConfigFileUsed()

	// Check if we can write to the config file
	if configPath == "" {
		return nil, huma.Error403Forbidden("No config file path configured")
	}

	// Check write permissions
	if _, err := os.Stat(configPath); err == nil {
		// File exists, check if writable
		file, err := os.OpenFile(configPath, os.O_WRONLY, 0)
		if err != nil {
			return nil, huma.Error403Forbidden("Config file is not writable: " + err.Error())
		}
		file.Close()
	}

	// Update viper values with current runtime config
	viper.Set("logging.level", observability.GetLogLevel())
	viper.Set("logging.request_logging", observability.IsRequestLoggingEnabled())

	// Write config file
	if err := viper.WriteConfig(); err != nil {
		return nil, huma.Error500InternalServerError("Failed to write config file: " + err.Error())
	}

	return &PersistConfigOutput{
		Body: ConfigPersistResponse{
			Success:  true,
			Message:  "Configuration saved to " + configPath,
			Path:     configPath,
			Sections: []string{"logging"},
		},
	}, nil
}

// Helper methods

func (h *ConfigHandler) getFeatures() map[string]bool {
	if h.featureHandler == nil {
		return make(map[string]bool)
	}

	h.featureHandler.mu.RLock()
	defer h.featureHandler.mu.RUnlock()

	flags := make(map[string]bool, len(h.featureHandler.flags))
	maps.Copy(flags, h.featureHandler.flags)
	return flags
}

func (h *ConfigHandler) getFeatureConfig() map[string]map[string]any {
	if h.featureHandler == nil {
		return nil
	}

	h.featureHandler.mu.RLock()
	defer h.featureHandler.mu.RUnlock()

	if len(h.featureHandler.config) == 0 {
		return nil
	}

	config := make(map[string]map[string]any, len(h.featureHandler.config))
	for k, v := range h.featureHandler.config {
		configCopy := make(map[string]any, len(v))
		maps.Copy(configCopy, v)
		config[k] = configCopy
	}
	return config
}

func (h *ConfigHandler) updateFeatures(features map[string]bool) {
	if h.featureHandler == nil {
		return
	}

	h.featureHandler.mu.Lock()
	defer h.featureHandler.mu.Unlock()

	maps.Copy(h.featureHandler.flags, features)
}

func (h *ConfigHandler) getCircuitBreakerConfig() CircuitBreakerConfigData {
	cfg := h.cbManager.GetConfig()

	configData := CircuitBreakerConfigData{
		Global:   profileFromConfig(cfg.Global),
		Profiles: make(map[string]CircuitBreakerProfile),
	}

	for name, profile := range cfg.Profiles {
		configData.Profiles[name] = profileFromConfig(profile)
	}

	return configData
}

func (h *ConfigHandler) getStartupConfig() StartupConfig {
	return StartupConfig{
		Server: ServerConfigData{
			Host:         viper.GetString("server.host"),
			Port:         viper.GetInt("server.port"),
			ReadTimeout:  viper.GetDuration("server.read_timeout").String(),
			WriteTimeout: viper.GetDuration("server.write_timeout").String(),
		},
		Database: DatabaseConfigData{
			DSN:          "[redacted]", // Don't expose credentials
			MaxOpenConns: viper.GetInt("database.max_open_conns"),
			MaxIdleConns: viper.GetInt("database.max_idle_conns"),
		},
		Storage: StorageConfigData{
			BaseDir:       viper.GetString("storage.base_dir"),
			LogoRetention: viper.GetDuration("storage.logo_retention").String(),
			MaxLogoSize:   bytesize.Format(bytesize.Size(viper.GetInt64("storage.max_logo_size"))),
		},
		Pipeline: PipelineConfigData{
			LogoConcurrency:    viper.GetInt("pipeline.logo_concurrency"),
			LogoTimeout:        viper.GetDuration("pipeline.logo_timeout").String(),
			LogoRetryAttempts:  viper.GetInt("pipeline.logo_retry_attempts"),
			LogoCircuitBreaker: viper.GetString("pipeline.logo_circuit_breaker"),
			LogoBatchSize:      viper.GetInt("pipeline.logo_batch_size"),
			StreamBatchSize:    viper.GetInt("pipeline.stream_batch_size"),
		},
		Scheduler: SchedulerConfigData{
			WorkerCount:         viper.GetInt("scheduler.worker_count"),
			PollInterval:        viper.GetDuration("scheduler.poll_interval").String(),
			SyncInterval:        viper.GetDuration("scheduler.sync_interval").String(),
			JobHistoryRetention: viper.GetDuration("scheduler.job_history_retention").String(),
		},
		Relay: RelayConfigData{
			Enabled:                 viper.GetBool("relay.enabled"),
			MaxConcurrentStreams:    viper.GetInt("relay.max_concurrent_streams"),
			CircuitBreakerThreshold: viper.GetInt("relay.circuit_breaker_threshold"),
			CircuitBreakerTimeout:   viper.GetDuration("relay.circuit_breaker_timeout").String(),
			StreamTimeout:           viper.GetDuration("relay.stream_timeout").String(),
		},
		Ingestion: IngestionConfigData{
			HTTPTimeout:   viper.GetDuration("ingestion.http_timeout").String(),
			MaxConcurrent: viper.GetInt("ingestion.max_concurrent"),
			RetryAttempts: viper.GetInt("ingestion.retry_attempts"),
			RetryDelay:    viper.GetDuration("ingestion.retry_delay").String(),
		},
	}
}

func (h *ConfigHandler) getConfigMeta() ConfigMeta {
	configPath := viper.ConfigFileUsed()
	canPersist := false
	var lastModified time.Time
	source := "defaults"

	if configPath != "" {
		source = "file"
		// Check if file is writable
		if info, err := os.Stat(configPath); err == nil {
			lastModified = info.ModTime()
			// Try to open for writing to check permissions
			if file, err := os.OpenFile(configPath, os.O_WRONLY, 0); err == nil {
				canPersist = true
				file.Close()
			}
		}
	}

	// Check if any env vars are set (simplified check)
	if os.Getenv("TVARR_SERVER_PORT") != "" || os.Getenv("TVARR_DATABASE_DSN") != "" {
		source = "env"
	}

	return ConfigMeta{
		ConfigPath:   configPath,
		CanPersist:   canPersist,
		LastModified: lastModified,
		Source:       source,
	}
}
