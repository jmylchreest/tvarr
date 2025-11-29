package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Load without config file should use defaults
	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Server defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)

	// Database defaults
	assert.Equal(t, "sqlite", cfg.Database.Driver)
	assert.Equal(t, "tvarr.db", cfg.Database.DSN)
	assert.Equal(t, 10, cfg.Database.MaxOpenConns)

	// Storage defaults
	assert.Equal(t, "./data", cfg.Storage.BaseDir)
	assert.Equal(t, "logos", cfg.Storage.LogoDir)
	assert.Equal(t, "output", cfg.Storage.OutputDir)

	// Logging defaults
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)

	// Ingestion defaults
	assert.Equal(t, 1000, cfg.Ingestion.ChannelBatchSize)
	assert.Equal(t, 5000, cfg.Ingestion.EPGBatchSize)

	// Pipeline defaults
	assert.Equal(t, 1000, cfg.Pipeline.StreamBatchSize)
	assert.True(t, cfg.Pipeline.EnableGCHints)

	// Relay defaults
	assert.False(t, cfg.Relay.Enabled)
	assert.Equal(t, 10, cfg.Relay.MaxConcurrentStreams)

	// FFmpeg defaults
	assert.False(t, cfg.FFmpeg.UseEmbedded)
}

func TestLoad_FromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  host: "127.0.0.1"
  port: 9090
  read_timeout: 60s

database:
  driver: "postgres"
  dsn: "postgres://user:pass@localhost/tvarr"
  max_open_conns: 20

storage:
  base_dir: "/var/lib/tvarr"

logging:
  level: "debug"
  format: "text"

ingestion:
  channel_batch_size: 2000
  epg_batch_size: 10000
`
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Check file values were loaded
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, 60*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "postgres", cfg.Database.Driver)
	assert.Equal(t, "postgres://user:pass@localhost/tvarr", cfg.Database.DSN)
	assert.Equal(t, 20, cfg.Database.MaxOpenConns)
	assert.Equal(t, "/var/lib/tvarr", cfg.Storage.BaseDir)
	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.Equal(t, "text", cfg.Logging.Format)
	assert.Equal(t, 2000, cfg.Ingestion.ChannelBatchSize)
	assert.Equal(t, 10000, cfg.Ingestion.EPGBatchSize)
}

func TestLoad_EnvOverride(t *testing.T) {
	// Set environment variables
	t.Setenv("TVARR_SERVER_PORT", "3000")
	t.Setenv("TVARR_DATABASE_DRIVER", "mysql")
	t.Setenv("TVARR_DATABASE_DSN", "mysql://localhost/test")
	t.Setenv("TVARR_LOGGING_LEVEL", "warn")
	t.Setenv("TVARR_INGESTION_CHANNEL_BATCH_SIZE", "500")

	cfg, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Check env overrides
	assert.Equal(t, 3000, cfg.Server.Port)
	assert.Equal(t, "mysql", cfg.Database.Driver)
	assert.Equal(t, "mysql://localhost/test", cfg.Database.DSN)
	assert.Equal(t, "warn", cfg.Logging.Level)
	assert.Equal(t, 500, cfg.Ingestion.ChannelBatchSize)
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
database:
  driver: "sqlite"
  dsn: "test.db"
`
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Set env var to override file
	t.Setenv("TVARR_SERVER_PORT", "9000")

	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Env should override file
	assert.Equal(t, 9000, cfg.Server.Port)
	// File value should be preserved
	assert.Equal(t, "sqlite", cfg.Database.Driver)
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "test.db",
		},
		Storage: StorageConfig{
			BaseDir: "./data",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Ingestion: IngestionConfig{
			ChannelBatchSize: 1000,
			EPGBatchSize:     5000,
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"port too high", 70000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{Port: tt.port},
				Database: DatabaseConfig{
					Driver: "sqlite",
					DSN:    "test.db",
				},
				Storage: StorageConfig{BaseDir: "./data"},
				Logging: LoggingConfig{Level: "info", Format: "json"},
				Ingestion: IngestionConfig{
					ChannelBatchSize: 1000,
					EPGBatchSize:     5000,
				},
			}
			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "server.port")
		})
	}
}

func TestValidate_InvalidDriver(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Port: 8080},
		Database: DatabaseConfig{
			Driver: "invalid",
			DSN:    "test.db",
		},
		Storage:   StorageConfig{BaseDir: "./data"},
		Logging:   LoggingConfig{Level: "info", Format: "json"},
		Ingestion: IngestionConfig{ChannelBatchSize: 1000, EPGBatchSize: 5000},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database.driver")
}

func TestValidate_EmptyDSN(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Port: 8080},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "",
		},
		Storage:   StorageConfig{BaseDir: "./data"},
		Logging:   LoggingConfig{Level: "info", Format: "json"},
		Ingestion: IngestionConfig{ChannelBatchSize: 1000, EPGBatchSize: 5000},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database.dsn")
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Driver: "sqlite", DSN: "test.db"},
		Storage:  StorageConfig{BaseDir: "./data"},
		Logging: LoggingConfig{
			Level:  "invalid",
			Format: "json",
		},
		Ingestion: IngestionConfig{ChannelBatchSize: 1000, EPGBatchSize: 5000},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logging.level")
}

func TestValidate_InvalidLogFormat(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Driver: "sqlite", DSN: "test.db"},
		Storage:  StorageConfig{BaseDir: "./data"},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "xml",
		},
		Ingestion: IngestionConfig{ChannelBatchSize: 1000, EPGBatchSize: 5000},
	}

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logging.format")
}

func TestValidate_InvalidBatchSize(t *testing.T) {
	tests := []struct {
		name        string
		channelBS   int
		epgBS       int
		errContains string
	}{
		{"zero channel batch", 0, 5000, "channel_batch_size"},
		{"negative channel batch", -1, 5000, "channel_batch_size"},
		{"zero epg batch", 1000, 0, "epg_batch_size"},
		{"negative epg batch", 1000, -1, "epg_batch_size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Server:   ServerConfig{Port: 8080},
				Database: DatabaseConfig{Driver: "sqlite", DSN: "test.db"},
				Storage:  StorageConfig{BaseDir: "./data"},
				Logging:  LoggingConfig{Level: "info", Format: "json"},
				Ingestion: IngestionConfig{
					ChannelBatchSize: tt.channelBS,
					EPGBatchSize:     tt.epgBS,
				},
			}
			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestServerConfig_Address(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{"localhost", "127.0.0.1", 8080, "127.0.0.1:8080"},
		{"all interfaces", "0.0.0.0", 3000, "0.0.0.0:3000"},
		{"hostname", "example.com", 443, "example.com:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ServerConfig{Host: tt.host, Port: tt.port}
			assert.Equal(t, tt.expected, cfg.Address())
		})
	}
}

func TestStorageConfig_Paths(t *testing.T) {
	cfg := &StorageConfig{
		BaseDir:   "/var/lib/tvarr",
		LogoDir:   "logos",
		OutputDir: "output",
		TempDir:   "temp",
	}

	assert.Equal(t, "/var/lib/tvarr/logos", cfg.LogoPath())
	assert.Equal(t, "/var/lib/tvarr/output", cfg.OutputPath())
	assert.Equal(t, "/var/lib/tvarr/temp", cfg.TempPath())
}

func TestLoad_InvalidConfigFile(t *testing.T) {
	// Create an invalid YAML file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidContent := `
server:
  port: "not a number"
  invalid yaml structure
`
	err := os.WriteFile(configPath, []byte(invalidContent), 0o600)
	require.NoError(t, err)

	_, err = Load(configPath)
	assert.Error(t, err)
}

func TestLoad_NonExistentFile(t *testing.T) {
	// Specifying a non-existent file should fail
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
}

func TestConfig_AllDrivers(t *testing.T) {
	drivers := []string{"sqlite", "postgres", "mysql"}

	for _, driver := range drivers {
		t.Run(driver, func(t *testing.T) {
			cfg := &Config{
				Server: ServerConfig{Port: 8080},
				Database: DatabaseConfig{
					Driver: driver,
					DSN:    "test-dsn",
				},
				Storage:   StorageConfig{BaseDir: "./data"},
				Logging:   LoggingConfig{Level: "info", Format: "json"},
				Ingestion: IngestionConfig{ChannelBatchSize: 1000, EPGBatchSize: 5000},
			}
			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}
}
