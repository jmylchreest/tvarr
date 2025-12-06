package httpclient

import (
	"time"
)

// CircuitBreakerProfileConfig holds settings for a circuit breaker profile.
// These settings can be updated at runtime and are shared via pointer.
type CircuitBreakerProfileConfig struct {
	// FailureThreshold is the number of failures before the circuit opens.
	FailureThreshold int `json:"failure_threshold" yaml:"failure_threshold"`

	// ResetTimeout is how long the circuit stays open before transitioning to half-open.
	ResetTimeout time.Duration `json:"reset_timeout" yaml:"reset_timeout"`

	// HalfOpenMax is the max requests allowed in half-open state before deciding
	// whether to close or re-open the circuit.
	HalfOpenMax int `json:"half_open_max" yaml:"half_open_max"`

	// AcceptableStatusCodes specifies which HTTP status codes should be considered
	// "successful" for circuit breaker purposes. If nil, defaults to 2xx.
	AcceptableStatusCodes *StatusCodeSet `json:"acceptable_status_codes,omitempty" yaml:"acceptable_status_codes,omitempty"`
}

// DefaultProfileConfig returns a CircuitBreakerProfileConfig with sensible defaults.
func DefaultProfileConfig() CircuitBreakerProfileConfig {
	return CircuitBreakerProfileConfig{
		FailureThreshold:      DefaultCircuitThreshold,
		ResetTimeout:          DefaultCircuitTimeout,
		HalfOpenMax:           DefaultCircuitHalfOpenMax,
		AcceptableStatusCodes: nil, // nil means default to 2xx
	}
}

// Clone returns a deep copy of the profile config.
func (c *CircuitBreakerProfileConfig) Clone() *CircuitBreakerProfileConfig {
	if c == nil {
		return nil
	}
	clone := *c
	if c.AcceptableStatusCodes != nil {
		clone.AcceptableStatusCodes = c.AcceptableStatusCodes.Clone()
	}
	return &clone
}

// MergeWith returns a new config with values from other overriding zero values in c.
// This allows sparse profile configs to inherit from global.
func (c *CircuitBreakerProfileConfig) MergeWith(other *CircuitBreakerProfileConfig) *CircuitBreakerProfileConfig {
	if other == nil {
		return c.Clone()
	}
	if c == nil {
		return other.Clone()
	}

	result := c.Clone()

	// Override with non-zero values from other
	if other.FailureThreshold > 0 {
		result.FailureThreshold = other.FailureThreshold
	}
	if other.ResetTimeout > 0 {
		result.ResetTimeout = other.ResetTimeout
	}
	if other.HalfOpenMax > 0 {
		result.HalfOpenMax = other.HalfOpenMax
	}
	if other.AcceptableStatusCodes != nil {
		result.AcceptableStatusCodes = other.AcceptableStatusCodes.Clone()
	}

	return result
}

// CircuitBreakerConfig holds global and per-service circuit breaker configurations.
// This is the top-level config that can be loaded from YAML or updated via API.
type CircuitBreakerConfig struct {
	// Global is the default profile applied to all circuit breakers.
	Global CircuitBreakerProfileConfig `json:"global" yaml:"global"`

	// Profiles contains service-specific overrides keyed by service name.
	// Values are merged with Global - only non-zero fields override.
	Profiles map[string]CircuitBreakerProfileConfig `json:"profiles,omitempty" yaml:"profiles,omitempty"`
}

// DefaultCircuitBreakerConfig returns a config with sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Global: DefaultProfileConfig(),
		Profiles: map[string]CircuitBreakerProfileConfig{
			// Logo service profile: 404s are acceptable (missing logos are not failures)
			// and higher threshold since logo fetches can have many transient failures
			"logos": {
				FailureThreshold:      100,
				ResetTimeout:          DefaultCircuitTimeout,
				HalfOpenMax:           DefaultCircuitHalfOpenMax,
				AcceptableStatusCodes: MustParseStatusCodes("200-299,404"),
			},
		},
	}
}

// GetProfileFor returns the merged config for a service.
// If a service-specific profile exists, it's merged with global.
// Otherwise, returns the global config.
func (c *CircuitBreakerConfig) GetProfileFor(serviceName string) *CircuitBreakerProfileConfig {
	if serviceProfile, ok := c.Profiles[serviceName]; ok {
		return c.Global.MergeWith(&serviceProfile)
	}
	return c.Global.Clone()
}

// Clone returns a deep copy of the config.
func (c *CircuitBreakerConfig) Clone() *CircuitBreakerConfig {
	if c == nil {
		return nil
	}
	clone := &CircuitBreakerConfig{
		Global:   *c.Global.Clone(),
		Profiles: make(map[string]CircuitBreakerProfileConfig, len(c.Profiles)),
	}
	for name, profile := range c.Profiles {
		clone.Profiles[name] = *profile.Clone()
	}
	return clone
}
