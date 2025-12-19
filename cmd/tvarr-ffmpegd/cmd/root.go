// Package cmd implements the CLI commands for tvarr-ffmpegd.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// daemonViper is a separate viper instance for ffmpegd configuration
// to avoid conflicts with main tvarr configuration.
var daemonViper = viper.New()

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     "tvarr-ffmpegd",
	Short:   "Distributed transcoding daemon for tvarr",
	Version: version.Short(),
	Long: `tvarr-ffmpegd is a distributed transcoding daemon that connects to a
tvarr coordinator to provide transcoding capacity.

It reports hardware capabilities (GPU encoders, session limits) and accepts
bidirectional gRPC streaming of ES samples for transcoding operations.

Configuration is primarily via environment variables:
  TVARR_COORDINATOR_URL  - Coordinator gRPC address (required for remote mode)
  TVARR_AUTH_TOKEN       - Authentication token
  TVARR_DAEMON_NAME      - Human-readable daemon name
  TVARR_MAX_JOBS         - Maximum concurrent transcoding jobs

Example:
  # Connect to coordinator at 192.168.1.100:9090
  TVARR_COORDINATOR_URL=192.168.1.100:9090 \
  TVARR_AUTH_TOKEN=mytoken \
  tvarr-ffmpegd serve`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return fmt.Errorf("executing root command: %w", err)
	}
	return nil
}

func init() {
	cobra.OnInitialize(initConfig)

	// Set PersistentPreRunE for logging initialization
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return initLogging()
	}

	// Global flags
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "json", "log format (text, json)")
}

// initConfig reads environment variables for daemon configuration.
func initConfig() {
	// Environment variables with TVARR_ prefix
	daemonViper.SetEnvPrefix("TVARR")
	daemonViper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	daemonViper.AutomaticEnv()

	// Set daemon-specific defaults
	setDaemonDefaults()
}

// setDaemonDefaults sets default values for daemon configuration.
func setDaemonDefaults() {
	// Daemon identification
	hostname, _ := os.Hostname()
	daemonViper.SetDefault("daemon.name", hostname)
	daemonViper.SetDefault("daemon.id", "")                     // Auto-generated if empty
	daemonViper.SetDefault("daemon.max_jobs", 4)

	// Coordinator connection
	daemonViper.SetDefault("coordinator.url", "")               // Required for remote mode
	daemonViper.SetDefault("coordinator.auth_token", "")        // Required for auth
	daemonViper.SetDefault("coordinator.heartbeat_interval", "5s")
	daemonViper.SetDefault("coordinator.reconnect_delay", "5s")
	daemonViper.SetDefault("coordinator.reconnect_max_delay", "60s")

	// Hardware detection
	daemonViper.SetDefault("hw.accel", "auto")                  // auto, cuda, vaapi, qsv, none
	daemonViper.SetDefault("hw.device", "")                     // e.g., /dev/dri/renderD128
	daemonViper.SetDefault("hw.gpu_max_sessions", 0)            // 0 = auto-detect

	// Logging defaults (shared with main tvarr)
	daemonViper.SetDefault("logging.level", "info")
	daemonViper.SetDefault("logging.format", "json")
}

// initLogging configures the slog logger for the daemon.
func initLogging() error {
	// Start with config/env values
	level := daemonViper.GetString("logging.level")
	format := daemonViper.GetString("logging.format")

	// Override with CLI flags only if explicitly set
	if rootCmd.PersistentFlags().Changed("log-level") {
		level, _ = rootCmd.PersistentFlags().GetString("log-level")
	}
	if rootCmd.PersistentFlags().Changed("log-format") {
		format, _ = rootCmd.PersistentFlags().GetString("log-format")
	}

	// Apply defaults if still empty
	if level == "" {
		level = "info"
	}
	if format == "" {
		format = "json"
	}

	logCfg := config.LoggingConfig{
		Level:  strings.ToLower(level),
		Format: strings.ToLower(format),
	}

	// Handle "warning" as an alias for "warn"
	if logCfg.Level == "warning" {
		logCfg.Level = "warn"
	}

	// Use observability package to create logger with sensitive data redaction
	logger := observability.NewLoggerWithWriter(logCfg, os.Stderr)
	observability.SetDefault(logger)

	return nil
}

// GetDaemonViper returns the daemon-specific viper instance.
// This is used by subcommands to access configuration.
func GetDaemonViper() *viper.Viper {
	return daemonViper
}
