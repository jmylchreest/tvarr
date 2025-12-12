package handlers

import (
	"time"
)

// UnifiedConfigResponse is the full configuration response.
type UnifiedConfigResponse struct {
	Success bool          `json:"success"`
	Runtime RuntimeConfig `json:"runtime"`
	Startup StartupConfig `json:"startup"`
	Meta    ConfigMeta    `json:"meta"`
}

// RuntimeConfig contains all runtime-modifiable settings.
type RuntimeConfig struct {
	Settings        ConfigRuntimeSettings     `json:"settings"`
	Features        map[string]bool           `json:"features"`
	FeatureConfig   map[string]map[string]any `json:"feature_config,omitempty"`
	CircuitBreakers CircuitBreakerConfigData  `json:"circuit_breakers"`
}

// ConfigRuntimeSettings are the core runtime settings.
type ConfigRuntimeSettings struct {
	LogLevel             string `json:"log_level"`
	EnableRequestLogging bool   `json:"enable_request_logging"`
}

// StartupConfig contains read-only startup configuration.
type StartupConfig struct {
	Server    ServerConfigData    `json:"server"`
	Database  DatabaseConfigData  `json:"database"`
	Storage   StorageConfigData   `json:"storage"`
	Pipeline  PipelineConfigData  `json:"pipeline"`
	Scheduler SchedulerConfigData `json:"scheduler"`
	Relay     RelayConfigData     `json:"relay"`
	Ingestion IngestionConfigData `json:"ingestion"`
}

// ServerConfigData represents server configuration.
type ServerConfigData struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ReadTimeout  string `json:"read_timeout"`
	WriteTimeout string `json:"write_timeout"`
}

// DatabaseConfigData represents database configuration.
type DatabaseConfigData struct {
	DSN          string `json:"dsn"`
	MaxOpenConns int    `json:"max_open_conns"`
	MaxIdleConns int    `json:"max_idle_conns"`
}

// StorageConfigData represents storage configuration.
type StorageConfigData struct {
	BaseDir       string `json:"base_dir"`
	LogoRetention string `json:"logo_retention"`
	MaxLogoSize   string `json:"max_logo_size"` // Human-readable format, e.g., "5 MB"
}

// PipelineConfigData represents pipeline configuration.
type PipelineConfigData struct {
	LogoConcurrency    int    `json:"logo_concurrency"`
	LogoTimeout        string `json:"logo_timeout"`
	LogoRetryAttempts  int    `json:"logo_retry_attempts"`
	LogoCircuitBreaker string `json:"logo_circuit_breaker"`
	LogoBatchSize      int    `json:"logo_batch_size"`
	StreamBatchSize    int    `json:"stream_batch_size"`
}

// SchedulerConfigData represents scheduler configuration.
type SchedulerConfigData struct {
	WorkerCount         int    `json:"worker_count"`
	PollInterval        string `json:"poll_interval"`
	SyncInterval        string `json:"sync_interval"`
	JobHistoryRetention string `json:"job_history_retention"`
}

// RelayConfigData represents relay configuration.
type RelayConfigData struct {
	Enabled                 bool   `json:"enabled"`
	MaxConcurrentStreams    int    `json:"max_concurrent_streams"`
	CircuitBreakerThreshold int    `json:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   string `json:"circuit_breaker_timeout"`
	StreamTimeout           string `json:"stream_timeout"`
}

// IngestionConfigData represents ingestion configuration.
type IngestionConfigData struct {
	HTTPTimeout   string `json:"http_timeout"`
	MaxConcurrent int    `json:"max_concurrent"`
	RetryAttempts int    `json:"retry_attempts"`
	RetryDelay    string `json:"retry_delay"`
}

// ConfigMeta contains metadata about the configuration.
type ConfigMeta struct {
	ConfigPath   string    `json:"config_path,omitempty"`
	CanPersist   bool      `json:"can_persist"`
	LastModified time.Time `json:"last_modified,omitempty"`
	Source       string    `json:"source"` // "file", "env", "defaults"
}

// UnifiedConfigUpdate is the request body for updating configuration.
type UnifiedConfigUpdate struct {
	Settings        *ConfigRuntimeSettings          `json:"settings,omitempty"`
	Features        map[string]bool                 `json:"features,omitempty"`
	CircuitBreakers *CircuitBreakerConfigUpdateData `json:"circuit_breakers,omitempty"`
}

// CircuitBreakerConfigUpdateData is for updating circuit breaker configuration.
type CircuitBreakerConfigUpdateData struct {
	Global   *CircuitBreakerProfile           `json:"global,omitempty"`
	Profiles map[string]CircuitBreakerProfile `json:"profiles,omitempty"`
}

// ConfigUpdateResponse is the response for a config update.
type ConfigUpdateResponse struct {
	Success        bool     `json:"success"`
	Message        string   `json:"message"`
	AppliedChanges []string `json:"applied_changes,omitempty"`
}

// ConfigPersistResponse is the response for persisting config to file.
type ConfigPersistResponse struct {
	Success  bool     `json:"success"`
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
	Sections []string `json:"sections,omitempty"`
}
