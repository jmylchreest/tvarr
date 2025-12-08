package httpclient

import (
	"log/slog"
	"sync"
)

// CircuitBreakerManager manages circuit breakers with support for runtime configuration updates.
// It provides:
//   - Shared circuit breakers by name (same name = same breaker instance)
//   - Global config with per-service profile overrides
//   - Runtime config updates that preserve circuit breaker state
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker              // Shared breakers by service name
	configs  map[string]*CircuitBreakerProfileConfig // Per-service config pointers
	config   *CircuitBreakerConfig                   // Full config with global + profiles
	logger   *slog.Logger
}

// NewCircuitBreakerManager creates a new manager with the given initial configuration.
func NewCircuitBreakerManager(cfg *CircuitBreakerConfig) *CircuitBreakerManager {
	if cfg == nil {
		defaultCfg := DefaultCircuitBreakerConfig()
		cfg = &defaultCfg
	}

	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		configs:  make(map[string]*CircuitBreakerProfileConfig),
		config:   cfg,
		logger:   slog.Default(),
	}
}

// WithLogger sets the logger for the manager.
func (m *CircuitBreakerManager) WithLogger(logger *slog.Logger) *CircuitBreakerManager {
	m.logger = logger
	return m
}

// GetOrCreate returns an existing circuit breaker for the service name,
// or creates a new one with the appropriate config (merged from global + service profile).
// Multiple calls with the same name return the same breaker instance.
func (m *CircuitBreakerManager) GetOrCreate(name string) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return existing breaker if found
	if breaker, ok := m.breakers[name]; ok {
		return breaker
	}

	// Get or create the config for this service
	cfg := m.getOrCreateConfigLocked(name)

	// Create new breaker with the config
	breaker := NewCircuitBreakerWithConfig(cfg)
	m.breakers[name] = breaker

	m.logger.Debug("created circuit breaker",
		slog.String("service", name),
		slog.Int("failure_threshold", cfg.FailureThreshold),
		slog.Duration("reset_timeout", cfg.ResetTimeout),
	)

	return breaker
}

// getOrCreateConfigLocked returns the config for a service, creating it if needed.
// Caller must hold m.mu lock.
func (m *CircuitBreakerManager) getOrCreateConfigLocked(name string) *CircuitBreakerProfileConfig {
	// Return existing config if found
	if cfg, ok := m.configs[name]; ok {
		return cfg
	}

	// Create merged config from global + service profile
	cfg := m.config.GetProfileFor(name)
	m.configs[name] = cfg
	return cfg
}

// Get returns an existing circuit breaker by name, or nil if not found.
func (m *CircuitBreakerManager) Get(name string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.breakers[name]
}

// UpdateConfig updates the full configuration and propagates changes to all active breakers.
// Circuit breaker state (failures, successes) is preserved.
func (m *CircuitBreakerManager) UpdateConfig(cfg *CircuitBreakerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg == nil {
		return
	}

	// Store new config
	m.config = cfg

	// Update all existing breakers with new merged configs
	for name, breaker := range m.breakers {
		newCfg := cfg.GetProfileFor(name)
		m.configs[name] = newCfg
		breaker.UpdateConfig(newCfg)

		m.logger.Debug("updated circuit breaker config",
			slog.String("service", name),
			slog.Int("failure_threshold", newCfg.FailureThreshold),
			slog.Duration("reset_timeout", newCfg.ResetTimeout),
		)
	}

	m.logger.Info("circuit breaker configuration updated",
		slog.Int("active_breakers", len(m.breakers)),
	)
}

// UpdateGlobalConfig updates only the global config and propagates to all breakers.
func (m *CircuitBreakerManager) UpdateGlobalConfig(cfg CircuitBreakerProfileConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config.Global = cfg

	// Update all existing breakers with new merged configs
	for name, breaker := range m.breakers {
		newCfg := m.config.GetProfileFor(name)
		m.configs[name] = newCfg
		breaker.UpdateConfig(newCfg)
	}

	m.logger.Info("global circuit breaker configuration updated")
}

// UpdateServiceConfig sets or updates a service-specific profile.
// If the service has an active breaker, it's updated immediately.
func (m *CircuitBreakerManager) UpdateServiceConfig(name string, cfg CircuitBreakerProfileConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the profile
	m.config.Profiles[name] = cfg

	// Create merged config
	mergedCfg := m.config.GetProfileFor(name)
	m.configs[name] = mergedCfg

	// Update existing breaker if present
	if breaker, ok := m.breakers[name]; ok {
		breaker.UpdateConfig(mergedCfg)
		m.logger.Debug("updated circuit breaker config",
			slog.String("service", name),
			slog.Int("failure_threshold", mergedCfg.FailureThreshold),
			slog.Duration("reset_timeout", mergedCfg.ResetTimeout),
		)
	}
}

// RemoveServiceConfig removes a service-specific profile.
// The service's breaker will revert to global config.
func (m *CircuitBreakerManager) RemoveServiceConfig(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.config.Profiles, name)

	// Update to global config
	globalCfg := m.config.Global.Clone()
	m.configs[name] = globalCfg

	// Update existing breaker if present
	if breaker, ok := m.breakers[name]; ok {
		breaker.UpdateConfig(globalCfg)
	}
}

// GetConfig returns a copy of the current full configuration.
// This includes both statically configured profiles and dynamically created service configs.
func (m *CircuitBreakerManager) GetConfig() CircuitBreakerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := *m.config.Clone()

	// Ensure Profiles map is initialized
	if result.Profiles == nil {
		result.Profiles = make(map[string]CircuitBreakerProfileConfig)
	}

	// Include dynamically created service configs (from active breakers)
	for name, cfg := range m.configs {
		if cfg != nil {
			// Only add if not already in the static profiles
			if _, exists := result.Profiles[name]; !exists {
				result.Profiles[name] = *cfg
			}
		}
	}

	return result
}

// GetServiceConfig returns the effective config for a service (merged global + profile).
func (m *CircuitBreakerManager) GetServiceConfig(name string) CircuitBreakerProfileConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if cfg, ok := m.configs[name]; ok && cfg != nil {
		return *cfg
	}
	return *m.config.GetProfileFor(name)
}

// Names returns the names of all active circuit breakers.
func (m *CircuitBreakerManager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.breakers))
	for name := range m.breakers {
		names = append(names, name)
	}
	return names
}

// GetAllStats returns statistics for all active circuit breakers.
func (m *CircuitBreakerManager) GetAllStats() map[string]CircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]CircuitBreakerStats, len(m.breakers))
	for name, breaker := range m.breakers {
		stats[name] = breaker.Stats()
	}
	return stats
}

// GetAllEnhancedStats returns enhanced statistics for all active circuit breakers.
func (m *CircuitBreakerManager) GetAllEnhancedStats() map[string]EnhancedCircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]EnhancedCircuitBreakerStats, len(m.breakers))
	for name, breaker := range m.breakers {
		stats[name] = breaker.EnhancedStats(name)
	}
	return stats
}

// GetEnhancedStats returns enhanced statistics for a specific circuit breaker.
func (m *CircuitBreakerManager) GetEnhancedStats(name string) (EnhancedCircuitBreakerStats, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	breaker, ok := m.breakers[name]
	if !ok {
		return EnhancedCircuitBreakerStats{}, false
	}
	return breaker.EnhancedStats(name), true
}

// ResetBreaker resets a specific circuit breaker to closed state.
func (m *CircuitBreakerManager) ResetBreaker(name string) bool {
	m.mu.RLock()
	breaker, ok := m.breakers[name]
	m.mu.RUnlock()

	if !ok {
		return false
	}

	breaker.Reset()
	m.logger.Info("circuit breaker reset", slog.String("service", name))
	return true
}

// ResetAll resets all circuit breakers to closed state.
func (m *CircuitBreakerManager) ResetAll() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for name, breaker := range m.breakers {
		breaker.Reset()
		m.logger.Debug("circuit breaker reset", slog.String("service", name))
		count++
	}

	m.logger.Info("all circuit breakers reset", slog.Int("count", count))
	return count
}

// Remove removes a circuit breaker from the manager.
// The breaker itself continues to work but won't be managed anymore.
func (m *CircuitBreakerManager) Remove(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.breakers[name]; !ok {
		return false
	}

	delete(m.breakers, name)
	delete(m.configs, name)
	return true
}

// DefaultManager is the global default circuit breaker manager.
var DefaultManager = NewCircuitBreakerManager(nil)
