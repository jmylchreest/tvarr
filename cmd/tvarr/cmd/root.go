// Package cmd implements the CLI commands for tvarr.
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	cfgFile   string
	logLevel  string
	logFormat string
)

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
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		return initLogging()
	},
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

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tvarr.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format (text, json)")

	// Bind flags to viper
	mustBindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	mustBindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
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
func initLogging() error {
	level := slog.LevelInfo
	switch strings.ToLower(viper.GetString("log.level")) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch strings.ToLower(viper.GetString("log.format")) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
	return nil
}

// mustBindPFlag binds a viper key to a cobra flag and panics if binding fails.
// This helper ensures lint-compliant error handling for viper.BindPFlag.
func mustBindPFlag(key string, flag *pflag.Flag) {
	if err := viper.BindPFlag(key, flag); err != nil {
		panic(fmt.Sprintf("failed to bind flag %q to key %q: %v", flag.Name, key, err))
	}
}
