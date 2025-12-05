package cmd

import (
	"fmt"
	"reflect"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/pkg/bytesize"
	"github.com/jmylchreest/tvarr/pkg/duration"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
	Long:  `Commands for managing tvarr configuration.`,
}

var configDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump the default configuration",
	Long: `Dump the default configuration values in YAML format.

This shows all available configuration options with their default values.
You can redirect this output to a file to create a configuration template:

  tvarr config dump > config.yaml

Configuration can be set via:
  - Config file (config.yaml, .tvarr.yaml, /etc/tvarr/config.yaml)
  - Environment variables (TVARR_SERVER_PORT, TVARR_DATABASE_DSN, etc.)
  - Command-line flags (for some options)

Environment variables use the TVARR_ prefix and underscores for nesting.
Example: server.port -> TVARR_SERVER_PORT`,
	RunE: runConfigDump,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configDumpCmd)
}

// toMap converts a struct to a map, formatting durations and sizes for human readability.
func toMap(v any) map[string]any {
	result := make(map[string]any)
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Get yaml tag or use lowercase field name
		key := fieldType.Tag.Get("mapstructure")
		if key == "" {
			key = fieldType.Tag.Get("yaml")
		}
		if key == "" {
			key = fieldType.Name
		}

		// Handle different types
		switch v := field.Interface().(type) {
		case time.Duration:
			result[key] = duration.Format(v)
		case int64:
			// Check if this looks like a byte size (field name contains "size")
			if contains(key, "size", "bytes") {
				result[key] = bytesize.Format(bytesize.Size(v))
			} else {
				result[key] = v
			}
		default:
			if field.Kind() == reflect.Struct {
				result[key] = toMap(field.Interface())
			} else {
				result[key] = field.Interface()
			}
		}
	}
	return result
}

func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

func runConfigDump(cmd *cobra.Command, args []string) error {
	// Load config with defaults (no file, just defaults)
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Convert to map with human-readable values
	cfgMap := toMap(cfg)

	// Marshal to YAML
	yamlData, err := yaml.Marshal(cfgMap)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Print header with documentation
	fmt.Println("# tvarr Configuration File")
	fmt.Println("# =========================")
	fmt.Println("#")
	fmt.Println("# All values shown below are defaults.")
	fmt.Println("# Duration format: 30s, 5m, 1h, 30d")
	fmt.Println("# Size format: 5MB, 1GB")
	fmt.Println("#")
	fmt.Println("# Environment variable overrides:")
	fmt.Println("#   TVARR_SERVER_HOST, TVARR_SERVER_PORT")
	fmt.Println("#   TVARR_DATABASE_DRIVER, TVARR_DATABASE_DSN")
	fmt.Println("#   TVARR_STORAGE_BASE_DIR, TVARR_STORAGE_LOGO_DIR")
	fmt.Println("#   TVARR_LOGGING_LEVEL, TVARR_LOGGING_FORMAT")
	fmt.Println("#   etc.")
	fmt.Println("#")
	fmt.Println("")
	fmt.Print(string(yamlData))

	return nil
}
