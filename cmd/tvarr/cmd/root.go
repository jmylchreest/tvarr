// Package cmd implements the CLI commands for tvarr.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// cfgFile holds the config file path from CLI flag.
var cfgFile string

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     "tvarr",
	Short:   "IPTV stream management and proxy service",
	Version: version.Short(),
	Long: `tvarr is a service for managing IPTV streams, EPG data, and generating
proxy playlists for media servers like Plex, Jellyfin, and Emby.

It supports multiple stream sources (M3U, Xtream Codes) and EPG formats
(XMLTV, Xtream EPG), with features for channel filtering, merging, and
automatic updates.`,
	// PersistentPreRunE is set in init() to avoid initialization cycle
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

	// Set PersistentPreRunE here to avoid initialization cycle
	// (initLogging references rootCmd.PersistentFlags)
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return initLogging()
	}

	// Global flags
	// Note: These flags are NOT bound to viper. Instead, we check if they were
	// explicitly set using Changed() and only then override the config/env values.
	// This preserves the correct priority: CLI flag > env var > config > default
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tvarr.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (trace, debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "json", "log format (text, json)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Set default configuration values before reading config file
	config.SetDefaults(viper.GetViper())

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".tvarr" (without extension).
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/tvarr")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".tvarr")
	}

	// Environment variables
	viper.SetEnvPrefix("TVARR")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// initLogging configures the slog logger based on configuration.
// Uses the observability package to ensure sensitive data redaction is applied.
// Note: log levels can be changed at runtime via the API (see observability.SetLogLevel).
//
// Priority order (highest to lowest):
//  1. CLI flags (--log-level, --log-format) - only if explicitly provided
//  2. Environment variables (TVARR_LOGGING_LEVEL, TVARR_LOGGING_FORMAT)
//  3. Config file values
//  4. Built-in defaults (info, json)
func initLogging() error {
	// Start with config/env values (viper handles precedence of env > config > default)
	level := viper.GetString("logging.level")
	format := viper.GetString("logging.format")

	// Override with CLI flags only if explicitly set by user.
	// We don't bind flags to viper because viper's flag layer would always
	// override env/config, even when using the flag's default value.
	if rootCmd.PersistentFlags().Changed("log-level") {
		level, _ = rootCmd.PersistentFlags().GetString("log-level")
	}
	if rootCmd.PersistentFlags().Changed("log-format") {
		format, _ = rootCmd.PersistentFlags().GetString("log-format")
	}

	// Apply defaults if still empty (shouldn't happen with proper config defaults)
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
	// Add "app" field to distinguish tvarr from tvarr-ffmpegd logs
	logger := observability.NewLoggerWithWriter(logCfg, os.Stderr)
	logger = observability.WithApp(logger, "tvarr")
	observability.SetDefault(logger)

	return nil
}

// mustBindPFlag binds a viper key to a cobra flag and panics if binding fails.
// This helper ensures lint-compliant error handling for viper.BindPFlag.
func mustBindPFlag(key string, flag *pflag.Flag) {
	if err := viper.BindPFlag(key, flag); err != nil {
		panic(fmt.Sprintf("failed to bind flag %q to key %q: %v", flag.Name, key, err))
	}
}
