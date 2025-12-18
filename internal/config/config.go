// Package config provides configuration management for tvarr using Viper.
// It supports configuration from files, environment variables, and defaults.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Default configuration values.
const (
	defaultServerPort            = 8080
	defaultServerTimeout         = 30 * time.Second
	defaultShutdownTimeout       = 10 * time.Second
	defaultMaxOpenConns          = 25
	defaultMaxIdleConns          = 10
	defaultConnMaxIdleTime       = 30 * time.Minute
	defaultLogoRetentionDays     = 30
	defaultMaxLogoSizeBytes      = 5 * 1024 * 1024 // 5MB
	defaultChannelBatchSize      = 1000
	defaultEPGBatchSize          = 5000
	defaultHTTPTimeout           = 60 * time.Second
	defaultMaxConcurrent         = 3
	defaultRetryAttempts         = 3
	defaultRetryDelay            = 5 * time.Second
	defaultLogoBatchSize         = 50
	defaultLogoConcurrency       = 10
	defaultLogoTimeout           = 30 * time.Second
	defaultLogoRetryAttempts     = 3
	defaultLogoCircuitBreaker    = "logos"
	defaultMaxConcurrentStreams  = 10
	defaultCircuitBreakerThresh  = 3
	defaultCircuitBreakerTimeout = 30 * time.Second
	defaultConnectionPoolSize    = 100
	defaultStreamTimeout         = 5 * time.Minute
)

// Config holds all configuration for the application.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Ingestion IngestionConfig `mapstructure:"ingestion"`
	Pipeline  PipelineConfig  `mapstructure:"pipeline"`
	Relay     RelayConfig     `mapstructure:"relay"`
	FFmpeg    FFmpegConfig    `mapstructure:"ffmpeg"`
	Backup    BackupConfig    `mapstructure:"backup"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	CORSOrigins     []string      `mapstructure:"cors_origins"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"` // sqlite, postgres, mysql
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	LogLevel        string        `mapstructure:"log_level"` // silent, error, warn, info
}

// StorageConfig holds file storage configuration.
type StorageConfig struct {
	BaseDir       string        `mapstructure:"base_dir"`
	LogoDir       string        `mapstructure:"logo_dir"`
	OutputDir     string        `mapstructure:"output_dir"`
	TempDir       string        `mapstructure:"temp_dir"`
	LogoRetention time.Duration `mapstructure:"logo_retention"`
	// MaxLogoSize is the maximum allowed size for logo files.
	// Supports human-readable values like "5MB", "1GB", or raw byte counts.
	MaxLogoSize ByteSize `mapstructure:"max_logo_size"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`  // debug, info, warn, error
	Format     string `mapstructure:"format"` // json, text
	AddSource  bool   `mapstructure:"add_source"`
	TimeFormat string `mapstructure:"time_format"`
}

// IngestionConfig holds source ingestion configuration.
type IngestionConfig struct {
	ChannelBatchSize int           `mapstructure:"channel_batch_size"`
	EPGBatchSize     int           `mapstructure:"epg_batch_size"`
	HTTPTimeout      time.Duration `mapstructure:"http_timeout"`
	MaxConcurrent    int           `mapstructure:"max_concurrent"`
	RetryAttempts    int           `mapstructure:"retry_attempts"`
	RetryDelay       time.Duration `mapstructure:"retry_delay"`
}

// PipelineConfig holds proxy generation pipeline configuration.
type PipelineConfig struct {
	StreamBatchSize    int           `mapstructure:"stream_batch_size"`
	EnableGCHints      bool          `mapstructure:"enable_gc_hints"`
	LogoBatchSize      int           `mapstructure:"logo_batch_size"`
	LogoConcurrency    int           `mapstructure:"logo_concurrency"`     // Number of concurrent logo downloads (default 10)
	LogoTimeout        time.Duration `mapstructure:"logo_timeout"`         // Timeout for individual logo downloads (default 30s)
	LogoRetryAttempts  int           `mapstructure:"logo_retry_attempts"`  // Number of retry attempts for failed logo downloads (default 3)
	LogoCircuitBreaker string        `mapstructure:"logo_circuit_breaker"` // Circuit breaker namespace for logos (default "logos")
}

// RelayConfig holds stream relay configuration.
type RelayConfig struct {
	Enabled                 bool          `mapstructure:"enabled"`
	MaxConcurrentStreams    int           `mapstructure:"max_concurrent_streams"`
	CircuitBreakerThreshold int           `mapstructure:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   time.Duration `mapstructure:"circuit_breaker_timeout"`
	ConnectionPoolSize      int           `mapstructure:"connection_pool_size"`
	StreamTimeout           time.Duration `mapstructure:"stream_timeout"`
	Buffer                  BufferConfig  `mapstructure:"buffer"`
}

// BufferConfig holds elementary stream buffer configuration.
type BufferConfig struct {
	// MaxVariantBytes is the maximum bytes per codec variant (0 = unlimited, uses consumer position for eviction only).
	// Supports human-readable values like "100MB", "1GB", or raw byte counts.
	MaxVariantBytes *ByteSize `mapstructure:"max_variant_bytes"`
}

// FFmpegConfig holds FFmpeg binary configuration.
type FFmpegConfig struct {
	BinaryPath      string   `mapstructure:"binary_path"`      // Path to ffmpeg binary (empty = auto-detect)
	ProbePath       string   `mapstructure:"probe_path"`       // Path to ffprobe binary (empty = auto-detect)
	UseEmbedded     bool     `mapstructure:"use_embedded"`     // Use embedded binary if available
	HWAccelPriority []string `mapstructure:"hwaccel_priority"` // Priority order: vaapi, nvenc, qsv, amf
}

// BackupConfig holds backup configuration.
type BackupConfig struct {
	Directory string               `mapstructure:"directory"` // Backup storage location (empty = {storage.base_dir}/backups)
	Schedule  BackupScheduleConfig `mapstructure:"schedule"`
}

// BackupScheduleConfig holds scheduled backup configuration.
type BackupScheduleConfig struct {
	Enabled   bool   `mapstructure:"enabled"`   // Enable scheduled backups
	Cron      string `mapstructure:"cron"`      // 6-field cron expression (default: "0 0 2 * * *" daily at 2 AM)
	Retention int    `mapstructure:"retention"` // Number of backups to keep
}

// Load reads configuration from file and environment variables.
// Environment variables take precedence over file configuration.
// Environment variables are prefixed with TVARR_ and use underscores for nesting.
// Example: TVARR_SERVER_PORT=8080.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	SetDefaults(v)

	// Config file settings
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/tvarr")
		v.AddConfigPath("$HOME/.tvarr")
	}

	// Environment variable settings
	v.SetEnvPrefix("TVARR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found is OK - we'll use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// SetDefaults configures default values for all configuration options.
// This should be called before reading the config file to ensure defaults are in place.
func SetDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", defaultServerPort)
	v.SetDefault("server.read_timeout", defaultServerTimeout)
	v.SetDefault("server.write_timeout", defaultServerTimeout)
	v.SetDefault("server.shutdown_timeout", defaultShutdownTimeout)
	v.SetDefault("server.cors_origins", []string{"*"})

	// Database defaults
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "tvarr.db")
	v.SetDefault("database.max_open_conns", defaultMaxOpenConns)
	v.SetDefault("database.max_idle_conns", defaultMaxIdleConns)
	v.SetDefault("database.conn_max_lifetime", time.Hour)
	v.SetDefault("database.conn_max_idle_time", defaultConnMaxIdleTime)
	v.SetDefault("database.log_level", "warn")

	// Storage defaults
	v.SetDefault("storage.base_dir", "./data")
	v.SetDefault("storage.logo_dir", "logos")
	v.SetDefault("storage.output_dir", "output")
	v.SetDefault("storage.temp_dir", "temp")
	v.SetDefault("storage.logo_retention", defaultLogoRetentionDays*24*time.Hour)
	v.SetDefault("storage.max_logo_size", defaultMaxLogoSizeBytes)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.add_source", false)
	v.SetDefault("logging.time_format", time.RFC3339)

	// Ingestion defaults
	v.SetDefault("ingestion.channel_batch_size", defaultChannelBatchSize)
	v.SetDefault("ingestion.epg_batch_size", defaultEPGBatchSize)
	v.SetDefault("ingestion.http_timeout", defaultHTTPTimeout)
	v.SetDefault("ingestion.max_concurrent", defaultMaxConcurrent)
	v.SetDefault("ingestion.retry_attempts", defaultRetryAttempts)
	v.SetDefault("ingestion.retry_delay", defaultRetryDelay)

	// Scheduler defaults
	v.SetDefault("scheduler.catchup_missed_runs", true)

	// Pipeline defaults
	v.SetDefault("pipeline.stream_batch_size", defaultChannelBatchSize)
	v.SetDefault("pipeline.enable_gc_hints", true)
	v.SetDefault("pipeline.logo_batch_size", defaultLogoBatchSize)
	v.SetDefault("pipeline.logo_concurrency", defaultLogoConcurrency)
	v.SetDefault("pipeline.logo_timeout", defaultLogoTimeout)
	v.SetDefault("pipeline.logo_retry_attempts", defaultLogoRetryAttempts)
	v.SetDefault("pipeline.logo_circuit_breaker", defaultLogoCircuitBreaker)

	// Relay defaults
	v.SetDefault("relay.enabled", false)
	v.SetDefault("relay.max_concurrent_streams", defaultMaxConcurrentStreams)
	v.SetDefault("relay.circuit_breaker_threshold", defaultCircuitBreakerThresh)
	v.SetDefault("relay.circuit_breaker_timeout", defaultCircuitBreakerTimeout)
	v.SetDefault("relay.connection_pool_size", defaultConnectionPoolSize)
	v.SetDefault("relay.stream_timeout", defaultStreamTimeout)

	// FFmpeg defaults
	v.SetDefault("ffmpeg.binary_path", "")
	v.SetDefault("ffmpeg.probe_path", "")
	v.SetDefault("ffmpeg.use_embedded", false)
	v.SetDefault("ffmpeg.hwaccel_priority", []string{"vaapi", "nvenc", "qsv", "amf"})

	// Backup defaults
	v.SetDefault("backup.directory", "")                // Empty = {storage.base_dir}/backups
	v.SetDefault("backup.schedule.enabled", true)       // Enabled by default
	v.SetDefault("backup.schedule.cron", "0 0 2 * * *") // Daily at 2 AM (6-field cron)
	v.SetDefault("backup.schedule.retention", 7)        // Keep last 7 backups
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Server validation
	const maxPort = 65535
	if c.Server.Port < 1 || c.Server.Port > maxPort {
		return fmt.Errorf("server.port must be between 1 and %d", maxPort)
	}

	// Database validation
	validDrivers := map[string]bool{"sqlite": true, "postgres": true, "mysql": true}
	if !validDrivers[c.Database.Driver] {
		return fmt.Errorf("database.driver must be one of: sqlite, postgres, mysql")
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}

	// Storage validation
	if c.Storage.BaseDir == "" {
		return fmt.Errorf("storage.base_dir is required")
	}

	// Logging validation
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("logging.format must be one of: json, text")
	}

	// Ingestion validation
	if c.Ingestion.ChannelBatchSize < 1 {
		return fmt.Errorf("ingestion.channel_batch_size must be at least 1")
	}
	if c.Ingestion.EPGBatchSize < 1 {
		return fmt.Errorf("ingestion.epg_batch_size must be at least 1")
	}

	return nil
}

// Address returns the server address in host:port format.
func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// LogoPath returns the full path to the logo directory.
func (c *StorageConfig) LogoPath() string {
	return fmt.Sprintf("%s/%s", c.BaseDir, c.LogoDir)
}

// OutputPath returns the full path to the output directory.
func (c *StorageConfig) OutputPath() string {
	return fmt.Sprintf("%s/%s", c.BaseDir, c.OutputDir)
}

// TempPath returns the full path to the temp directory.
func (c *StorageConfig) TempPath() string {
	return fmt.Sprintf("%s/%s", c.BaseDir, c.TempDir)
}

// BackupPath returns the backup directory path.
// If Directory is set, returns it directly; otherwise returns {BaseDir}/backups.
func (c *BackupConfig) BackupPath(storageBaseDir string) string {
	if c.Directory != "" {
		return c.Directory
	}
	return fmt.Sprintf("%s/backups", storageBaseDir)
}
