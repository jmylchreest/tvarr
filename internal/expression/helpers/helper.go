// Package helpers provides helper functions for expression processing.
package helpers

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Helper defines the interface for expression helpers.
// Helpers are invoked using @name:args syntax in action values.
type Helper interface {
	// Name returns the helper name (e.g., "time", "logo").
	Name() string

	// Process processes the helper with the given value and arguments.
	// For example, @time:now would call Process("now", "").
	Process(value, args string) (string, error)
}

// HelperRegistry manages registered helpers.
type HelperRegistry struct {
	mu      sync.RWMutex
	helpers map[string]Helper
}

// NewHelperRegistry creates a new helper registry.
func NewHelperRegistry() *HelperRegistry {
	return &HelperRegistry{
		helpers: make(map[string]Helper),
	}
}

// Register adds a helper to the registry.
func (r *HelperRegistry) Register(h Helper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.helpers[h.Name()] = h
}

// Get retrieves a helper by name.
func (r *HelperRegistry) Get(name string) (Helper, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.helpers[name]
	return h, ok
}

// Process processes a value, resolving any helper syntax.
// If the value starts with @, it's treated as a helper invocation.
func (r *HelperRegistry) Process(value string) (string, error) {
	isHelper, name, args := ParseHelperSyntax(value)
	if !isHelper {
		return value, nil
	}

	helper, ok := r.Get(name)
	if !ok {
		// Unknown helper - return original value
		return value, nil
	}

	return helper.Process(args, "")
}

// ProcessWithArgs processes a value with additional arguments.
func (r *HelperRegistry) ProcessWithArgs(value, extraArgs string) (string, error) {
	isHelper, name, args := ParseHelperSyntax(value)
	if !isHelper {
		return value, nil
	}

	helper, ok := r.Get(name)
	if !ok {
		return value, nil
	}

	return helper.Process(args, extraArgs)
}

// ParseHelperSyntax parses a helper syntax string (@name:args).
// Returns whether it's a helper, the helper name, and the arguments.
func ParseHelperSyntax(value string) (isHelper bool, name, args string) {
	if !strings.HasPrefix(value, "@") {
		return false, "", ""
	}

	rest := value[1:]
	if rest == "" {
		return false, "", ""
	}

	colonIdx := strings.Index(rest, ":")
	if colonIdx <= 0 {
		return false, "", ""
	}

	name = rest[:colonIdx]
	args = rest[colonIdx+1:]
	return true, name, args
}

// TimeHelper provides time-related helper functions.
type TimeHelper struct{}

// NewTimeHelper creates a new time helper.
func NewTimeHelper() *TimeHelper {
	return &TimeHelper{}
}

// Name returns the helper name.
func (h *TimeHelper) Name() string {
	return "time"
}

// Process processes time helper operations.
// Operations:
//   - now: returns current time in RFC3339 format (or custom format if args provided)
//   - parse: parses a time string
//   - format: formats a time (value|format)
//   - add: adds duration to time (value|duration)
func (h *TimeHelper) Process(value, args string) (string, error) {
	switch value {
	case "now":
		return h.now(args)
	case "parse":
		return h.parse(args)
	case "format":
		return h.format(args)
	case "add":
		return h.add(args)
	default:
		return "", fmt.Errorf("unknown time operation: %s", value)
	}
}

func (h *TimeHelper) now(format string) (string, error) {
	t := time.Now().UTC()
	if format == "" {
		return t.Format(time.RFC3339), nil
	}
	return t.Format(format), nil
}

func (h *TimeHelper) parse(input string) (string, error) {
	// Try common formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, input); err == nil {
			return t.Format(time.RFC3339), nil
		}
	}

	return "", fmt.Errorf("cannot parse time: %s", input)
}

func (h *TimeHelper) format(args string) (string, error) {
	// Format: input|output_format
	parts := strings.SplitN(args, "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("format requires input|format")
	}

	input := parts[0]
	outputFormat := parts[1]

	// Parse the input time
	t, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return "", fmt.Errorf("cannot parse time %s: %w", input, err)
	}

	return t.Format(outputFormat), nil
}

func (h *TimeHelper) add(args string) (string, error) {
	// Format: base_time|duration
	parts := strings.SplitN(args, "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("add requires base_time|duration")
	}

	baseTime := parts[0]
	durationStr := parts[1]

	t, err := time.Parse(time.RFC3339, baseTime)
	if err != nil {
		return "", fmt.Errorf("cannot parse base time %s: %w", baseTime, err)
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return "", fmt.Errorf("cannot parse duration %s: %w", durationStr, err)
	}

	return t.Add(duration).Format(time.RFC3339), nil
}

// LogoResolver resolves logo IDs to URLs.
// Implementations should check the database for logo existence.
type LogoResolver interface {
	// ResolveLogoURL checks if a logo ULID exists and returns its URL.
	// Returns the URL if found, empty string if not found, or error on database issues.
	ResolveLogoURL(id string) (string, error)
	// LogoExists checks if a logo ULID exists in the database.
	LogoExists(id string) (bool, error)
}

// LogoHelperConfig holds configuration for the logo helper.
type LogoHelperConfig struct {
	BaseURL  string // Base URL for constructing logo URLs (e.g., "https://example.com")
	Resolver LogoResolver
}

// LogoHelper provides logo resolution helper functions.
// Syntax: @logo:ULID
// If the ULID is valid and exists in the database, returns the full URL.
// If the ULID doesn't exist or is invalid, returns empty string (removing the field).
type LogoHelper struct {
	config LogoHelperConfig
}

// NewLogoHelper creates a new logo helper.
func NewLogoHelper(resolver LogoResolver) *LogoHelper {
	return &LogoHelper{config: LogoHelperConfig{Resolver: resolver}}
}

// NewLogoHelperWithConfig creates a new logo helper with configuration.
func NewLogoHelperWithConfig(config LogoHelperConfig) *LogoHelper {
	return &LogoHelper{config: config}
}

// Name returns the helper name.
func (h *LogoHelper) Name() string {
	return "logo"
}

// Process resolves a logo ULID to its URL.
// value contains the ULID (e.g., "01ARZ3NDEKTSV4RRFFQ69G5FAV").
// Returns the full URL if valid and exists, empty string otherwise.
func (h *LogoHelper) Process(value, args string) (string, error) {
	// Validate ULID format
	if !isValidULID(value) {
		// Invalid ULID - return empty to remove the field
		return "", nil
	}

	// If no resolver, we can't validate existence
	// Return the constructed URL but log a warning
	if h.config.Resolver == nil {
		// Construct URL even without resolver (for validation-only use)
		if h.config.BaseURL != "" {
			return h.constructLogoURL(value), nil
		}
		// No resolver and no base URL - return empty
		return "", nil
	}

	// Check if logo exists in database
	exists, err := h.config.Resolver.LogoExists(value)
	if err != nil {
		// Database error - propagate it
		return "", fmt.Errorf("checking logo existence: %w", err)
	}

	if !exists {
		// ULID doesn't exist in database - return empty to remove the field
		return "", nil
	}

	// Logo exists, construct and return URL
	return h.constructLogoURL(value), nil
}

// constructLogoURL builds the full logo URL from the ULID.
func (h *LogoHelper) constructLogoURL(ulid string) string {
	baseURL := strings.TrimSuffix(h.config.BaseURL, "/")
	if baseURL == "" {
		// Default URL pattern
		return fmt.Sprintf("/api/v1/logos/%s", ulid)
	}
	return fmt.Sprintf("%s/api/v1/logos/%s", baseURL, ulid)
}

// isValidULID validates a ULID string format.
// ULIDs are 26 characters long, using Crockford's base32 encoding.
// Format: 01ARZ3NDEKTSV4RRFFQ69G5FAV (26 chars, uppercase alphanumeric excluding I, L, O, U)
func isValidULID(s string) bool {
	if len(s) != 26 {
		return false
	}

	// Crockford's base32 alphabet (excludes I, L, O, U)
	for _, c := range s {
		if !isCrockfordBase32(byte(c)) {
			return false
		}
	}
	return true
}

// isCrockfordBase32 checks if a byte is a valid Crockford's base32 character.
// Valid chars: 0-9, A-H, J-K, M-N, P-T, V-Z (case insensitive)
func isCrockfordBase32(c byte) bool {
	// Convert to uppercase for consistent checking
	if c >= 'a' && c <= 'z' {
		c = c - 32 // Convert to uppercase
	}

	// 0-9 are valid
	if c >= '0' && c <= '9' {
		return true
	}

	// A-H, J-K, M-N, P-T, V-Z are valid (I, L, O, U excluded)
	if c >= 'A' && c <= 'Z' {
		// Exclude I, L, O, U
		return c != 'I' && c != 'L' && c != 'O' && c != 'U'
	}

	return false
}

// default registry singleton
var (
	defaultRegistry     *HelperRegistry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry returns the default helper registry with standard helpers.
func DefaultRegistry() *HelperRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewHelperRegistry()
		defaultRegistry.Register(NewTimeHelper())
		// Logo helper requires a resolver, so it's not registered by default
	})
	return defaultRegistry
}
