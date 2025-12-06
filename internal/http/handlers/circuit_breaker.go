package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
)

// CircuitBreakerHandler handles circuit breaker API endpoints.
type CircuitBreakerHandler struct {
	manager *httpclient.CircuitBreakerManager
}

// NewCircuitBreakerHandler creates a new circuit breaker handler.
func NewCircuitBreakerHandler(manager *httpclient.CircuitBreakerManager) *CircuitBreakerHandler {
	if manager == nil {
		manager = httpclient.DefaultManager
	}
	return &CircuitBreakerHandler{
		manager: manager,
	}
}

// Register registers the circuit breaker routes with the API.
func (h *CircuitBreakerHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getCircuitBreakerConfig",
		Method:      "GET",
		Path:        "/api/v1/circuit-breakers/config",
		Summary:     "Get circuit breaker configuration",
		Description: "Returns circuit breaker configuration and current status",
		Tags:        []string{"Circuit Breakers"},
	}, h.GetConfig)

	huma.Register(api, huma.Operation{
		OperationID: "getCircuitBreakerStats",
		Method:      "GET",
		Path:        "/api/v1/circuit-breakers/stats",
		Summary:     "Get enhanced circuit breaker statistics",
		Description: "Returns detailed statistics including error categorization, state durations, and transition history",
		Tags:        []string{"Circuit Breakers"},
	}, h.GetEnhancedStats)

	huma.Register(api, huma.Operation{
		OperationID: "updateCircuitBreakerConfig",
		Method:      "PUT",
		Path:        "/api/v1/circuit-breakers/config",
		Summary:     "Update circuit breaker configuration",
		Description: "Updates circuit breaker configuration at runtime",
		Tags:        []string{"Circuit Breakers"},
	}, h.UpdateConfig)

	huma.Register(api, huma.Operation{
		OperationID: "resetCircuitBreaker",
		Method:      "POST",
		Path:        "/api/v1/circuit-breakers/{name}/reset",
		Summary:     "Reset a circuit breaker",
		Description: "Resets a specific circuit breaker to closed state",
		Tags:        []string{"Circuit Breakers"},
	}, h.ResetCircuitBreaker)

	huma.Register(api, huma.Operation{
		OperationID: "resetAllCircuitBreakers",
		Method:      "POST",
		Path:        "/api/v1/circuit-breakers/reset",
		Summary:     "Reset all circuit breakers",
		Description: "Resets all circuit breakers to closed state",
		Tags:        []string{"Circuit Breakers"},
	}, h.ResetAllCircuitBreakers)
}

// CircuitBreakerProfile represents a circuit breaker configuration profile.
type CircuitBreakerProfile struct {
	FailureThreshold      int    `json:"failure_threshold"`
	ResetTimeout          string `json:"reset_timeout"`
	HalfOpenMax           int    `json:"half_open_max"`
	AcceptableStatusCodes string `json:"acceptable_status_codes,omitempty"`
}

// CircuitBreakerConfigData represents the circuit breaker configuration.
type CircuitBreakerConfigData struct {
	Global   CircuitBreakerProfile            `json:"global"`
	Profiles map[string]CircuitBreakerProfile `json:"profiles"`
}

// CircuitBreakerStatusData represents the current status of a circuit breaker.
type CircuitBreakerStatusData struct {
	Name             string    `json:"name"`
	State            string    `json:"state"`
	Failures         int       `json:"failures"`
	Successes        int       `json:"successes"`
	ConsecutiveFails int       `json:"consecutive_fails"`
	TotalRequests    int64     `json:"total_requests"`
	TotalSuccesses   int64     `json:"total_successes"`
	TotalFailures    int64     `json:"total_failures"`
	LastFailure      time.Time `json:"last_failure,omitempty"`
}

// Enhanced stats types - mirrors pkg/httpclient/stats.go for API response

// EnhancedErrorCounts categorizes errors for visualization.
type EnhancedErrorCounts struct {
	Success2xx     int64 `json:"success_2xx"`
	ClientError4xx int64 `json:"client_error_4xx"`
	ServerError5xx int64 `json:"server_error_5xx"`
	Timeout        int64 `json:"timeout"`
	NetworkError   int64 `json:"network_error"`
}

// EnhancedStateDurations tracks time spent in each state.
type EnhancedStateDurations struct {
	ClosedDurationMs   int64   `json:"closed_duration_ms"`
	OpenDurationMs     int64   `json:"open_duration_ms"`
	HalfOpenDurationMs int64   `json:"half_open_duration_ms"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	ClosedPercentage   float64 `json:"closed_percentage"`
	OpenPercentage     float64 `json:"open_percentage"`
	HalfOpenPercentage float64 `json:"half_open_percentage"`
}

// EnhancedStateTransition records a state change.
type EnhancedStateTransition struct {
	Timestamp        time.Time `json:"timestamp"`
	FromState        string    `json:"from_state"`
	ToState          string    `json:"to_state"`
	Reason           string    `json:"reason"`
	ConsecutiveCount int       `json:"consecutive_count"`
}

// EnhancedConfig contains the circuit breaker configuration.
type EnhancedConfig struct {
	FailureThreshold      int    `json:"failure_threshold"`
	ResetTimeout          string `json:"reset_timeout"`
	HalfOpenMax           int    `json:"half_open_max"`
	AcceptableStatusCodes string `json:"acceptable_status_codes,omitempty"`
}

// EnhancedCircuitBreakerStats contains detailed CB statistics for visualization.
type EnhancedCircuitBreakerStats struct {
	Name                 string                    `json:"name"`
	State                string                    `json:"state"`
	StateEnteredAt       time.Time                 `json:"state_entered_at"`
	StateDurationMs      int64                     `json:"state_duration_ms"`
	ConsecutiveFailures  int                       `json:"consecutive_failures"`
	ConsecutiveSuccesses int                       `json:"consecutive_successes"`
	TotalRequests        int64                     `json:"total_requests"`
	TotalSuccesses       int64                     `json:"total_successes"`
	TotalFailures        int64                     `json:"total_failures"`
	FailureRate          float64                   `json:"failure_rate"`
	ErrorCounts          EnhancedErrorCounts       `json:"error_counts"`
	StateDurations       EnhancedStateDurations    `json:"state_durations"`
	RecentTransitions    []EnhancedStateTransition `json:"recent_transitions,omitempty"`
	LastFailure          *time.Time                `json:"last_failure,omitempty"`
	LastSuccess          *time.Time                `json:"last_success,omitempty"`
	NextHalfOpenAt       *time.Time                `json:"next_half_open_at,omitempty"`
	Config               EnhancedConfig            `json:"config"`
}

// GetEnhancedStatsInput is the input for getting enhanced stats.
type GetEnhancedStatsInput struct{}

// GetEnhancedStatsOutput is the output for getting enhanced stats.
type GetEnhancedStatsOutput struct {
	Body struct {
		Success bool                          `json:"success"`
		Data    []EnhancedCircuitBreakerStats `json:"data"`
	}
}

// GetEnhancedStats returns enhanced circuit breaker statistics.
func (h *CircuitBreakerHandler) GetEnhancedStats(ctx context.Context, input *GetEnhancedStatsInput) (*GetEnhancedStatsOutput, error) {
	allStats := h.manager.GetAllEnhancedStats()
	result := make([]EnhancedCircuitBreakerStats, 0, len(allStats))

	for name, stats := range allStats {
		// Convert transitions
		transitions := make([]EnhancedStateTransition, 0, len(stats.Transitions))
		for _, t := range stats.Transitions {
			transitions = append(transitions, EnhancedStateTransition{
				Timestamp:        t.Timestamp,
				FromState:        t.FromState.String(),
				ToState:          t.ToState.String(),
				Reason:           string(t.Reason),
				ConsecutiveCount: t.ConsecutiveCount,
			})
		}

		// Build config
		acceptableCodes := ""
		if stats.Config.AcceptableStatusCodes != nil {
			acceptableCodes = stats.Config.AcceptableStatusCodes.String()
		}

		enhanced := EnhancedCircuitBreakerStats{
			Name:                 name,
			State:                stats.State.String(),
			StateEnteredAt:       stats.StateEnteredAt,
			StateDurationMs:      stats.StateDurationMs,
			ConsecutiveFailures:  stats.ConsecutiveFailures,
			ConsecutiveSuccesses: stats.ConsecutiveSuccesses,
			TotalRequests:        stats.TotalRequests,
			TotalSuccesses:       stats.TotalSuccesses,
			TotalFailures:        stats.TotalFailures,
			FailureRate:          stats.FailureRate,
			ErrorCounts: EnhancedErrorCounts{
				Success2xx:     stats.ErrorCounts.Success2xx,
				ClientError4xx: stats.ErrorCounts.ClientError4xx,
				ServerError5xx: stats.ErrorCounts.ServerError5xx,
				Timeout:        stats.ErrorCounts.Timeout,
				NetworkError:   stats.ErrorCounts.NetworkError,
			},
			StateDurations: EnhancedStateDurations{
				ClosedDurationMs:   stats.StateDurations.ClosedMs,
				OpenDurationMs:     stats.StateDurations.OpenMs,
				HalfOpenDurationMs: stats.StateDurations.HalfOpenMs,
				TotalDurationMs:    stats.StateDurations.TotalMs,
				ClosedPercentage:   stats.StateDurations.ClosedPct,
				OpenPercentage:     stats.StateDurations.OpenPct,
				HalfOpenPercentage: stats.StateDurations.HalfOpenPct,
			},
			RecentTransitions: transitions,
			Config: EnhancedConfig{
				FailureThreshold:      stats.Config.FailureThreshold,
				ResetTimeout:          stats.Config.ResetTimeout.String(),
				HalfOpenMax:           stats.Config.HalfOpenMax,
				AcceptableStatusCodes: acceptableCodes,
			},
		}

		// Set optional time fields only if not zero
		if !stats.LastFailure.IsZero() {
			enhanced.LastFailure = &stats.LastFailure
		}
		if !stats.LastSuccess.IsZero() {
			enhanced.LastSuccess = &stats.LastSuccess
		}
		if !stats.NextHalfOpenAt.IsZero() {
			enhanced.NextHalfOpenAt = &stats.NextHalfOpenAt
		}

		result = append(result, enhanced)
	}

	resp := &GetEnhancedStatsOutput{}
	resp.Body.Success = true
	resp.Body.Data = result
	return resp, nil
}

// GetConfigInput is the input for getting circuit breaker config.
type GetConfigInput struct{}

// GetConfigOutput is the output for getting circuit breaker config.
type GetConfigOutput struct {
	Body struct {
		Success bool `json:"success"`
		Data    struct {
			Config   CircuitBreakerConfigData   `json:"config"`
			Statuses []CircuitBreakerStatusData `json:"statuses"`
		} `json:"data"`
	}
}

// profileFromConfig converts internal config to API profile.
func profileFromConfig(cfg httpclient.CircuitBreakerProfileConfig) CircuitBreakerProfile {
	acceptableCodes := ""
	if cfg.AcceptableStatusCodes != nil {
		acceptableCodes = cfg.AcceptableStatusCodes.String()
	}
	return CircuitBreakerProfile{
		FailureThreshold:      cfg.FailureThreshold,
		ResetTimeout:          cfg.ResetTimeout.String(),
		HalfOpenMax:           cfg.HalfOpenMax,
		AcceptableStatusCodes: acceptableCodes,
	}
}

// configFromProfile converts API profile to internal config.
func configFromProfile(p CircuitBreakerProfile) (httpclient.CircuitBreakerProfileConfig, error) {
	cfg := httpclient.CircuitBreakerProfileConfig{
		FailureThreshold: p.FailureThreshold,
		HalfOpenMax:      p.HalfOpenMax,
	}

	// Parse reset timeout
	if p.ResetTimeout != "" {
		d, err := time.ParseDuration(p.ResetTimeout)
		if err != nil {
			return cfg, huma.Error400BadRequest("invalid reset_timeout format: " + err.Error())
		}
		cfg.ResetTimeout = d
	}

	// Parse acceptable status codes
	if p.AcceptableStatusCodes != "" {
		codes, err := httpclient.ParseStatusCodes(p.AcceptableStatusCodes)
		if err != nil {
			return cfg, huma.Error400BadRequest("invalid acceptable_status_codes: " + err.Error())
		}
		cfg.AcceptableStatusCodes = codes
	}

	return cfg, nil
}

// GetConfig returns circuit breaker configuration and status.
func (h *CircuitBreakerHandler) GetConfig(ctx context.Context, input *GetConfigInput) (*GetConfigOutput, error) {
	// Get current configuration from manager
	cfg := h.manager.GetConfig()

	// Build config response
	configData := CircuitBreakerConfigData{
		Global:   profileFromConfig(cfg.Global),
		Profiles: make(map[string]CircuitBreakerProfile),
	}

	for name, profile := range cfg.Profiles {
		configData.Profiles[name] = profileFromConfig(profile)
	}

	// Get current statuses from manager
	allStats := h.manager.GetAllStats()
	statuses := make([]CircuitBreakerStatusData, 0, len(allStats))

	for name, stats := range allStats {
		statuses = append(statuses, CircuitBreakerStatusData{
			Name:             name,
			State:            stats.State.String(),
			Failures:         stats.Failures,
			Successes:        stats.Successes,
			ConsecutiveFails: stats.ConsecutiveFailures,
			TotalRequests:    stats.TotalRequests,
			TotalSuccesses:   stats.TotalSuccesses,
			TotalFailures:    stats.TotalFailures,
			LastFailure:      stats.LastFailure,
		})
	}

	resp := &GetConfigOutput{}
	resp.Body.Success = true
	resp.Body.Data.Config = configData
	resp.Body.Data.Statuses = statuses

	return resp, nil
}

// UpdateConfigInput is the input for updating circuit breaker config.
type UpdateConfigInput struct {
	Body struct {
		Global   *CircuitBreakerProfile           `json:"global,omitempty"`
		Profiles map[string]CircuitBreakerProfile `json:"profiles,omitempty"`
	}
}

// UpdateConfigOutput is the output for updating circuit breaker config.
type UpdateConfigOutput struct {
	Body struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}
}

// UpdateConfig updates circuit breaker configuration at runtime.
func (h *CircuitBreakerHandler) UpdateConfig(ctx context.Context, input *UpdateConfigInput) (*UpdateConfigOutput, error) {
	// Update global config if provided
	if input.Body.Global != nil {
		globalCfg, err := configFromProfile(*input.Body.Global)
		if err != nil {
			return nil, err
		}
		h.manager.UpdateGlobalConfig(globalCfg)
	}

	// Update service-specific profiles
	for name, profile := range input.Body.Profiles {
		cfg, err := configFromProfile(profile)
		if err != nil {
			return nil, err
		}
		h.manager.UpdateServiceConfig(name, cfg)
	}

	resp := &UpdateConfigOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Circuit breaker configuration updated successfully"
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}

// ResetCircuitBreakerInput is the input for resetting a circuit breaker.
type ResetCircuitBreakerInput struct {
	Name string `path:"name" required:"true"`
}

// ResetCircuitBreakerOutput is the output for resetting a circuit breaker.
type ResetCircuitBreakerOutput struct {
	Body struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		Name      string `json:"name"`
		NewState  string `json:"new_state"`
		Timestamp string `json:"timestamp"`
	}
}

// ResetCircuitBreaker resets a specific circuit breaker.
func (h *CircuitBreakerHandler) ResetCircuitBreaker(ctx context.Context, input *ResetCircuitBreakerInput) (*ResetCircuitBreakerOutput, error) {
	if !h.manager.ResetBreaker(input.Name) {
		return nil, huma.Error404NotFound("Circuit breaker not found: " + input.Name)
	}

	// Get updated state
	breaker := h.manager.Get(input.Name)
	newState := "closed"
	if breaker != nil {
		newState = breaker.State().String()
	}

	resp := &ResetCircuitBreakerOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Circuit breaker reset successfully"
	resp.Body.Name = input.Name
	resp.Body.NewState = newState
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}

// ResetAllCircuitBreakersInput is the input for resetting all circuit breakers.
type ResetAllCircuitBreakersInput struct{}

// ResetAllCircuitBreakersOutput is the output for resetting all circuit breakers.
type ResetAllCircuitBreakersOutput struct {
	Body struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		Count     int    `json:"count"`
		Timestamp string `json:"timestamp"`
	}
}

// ResetAllCircuitBreakers resets all circuit breakers.
func (h *CircuitBreakerHandler) ResetAllCircuitBreakers(ctx context.Context, input *ResetAllCircuitBreakersInput) (*ResetAllCircuitBreakersOutput, error) {
	count := h.manager.ResetAll()

	resp := &ResetAllCircuitBreakersOutput{}
	resp.Body.Success = true
	resp.Body.Message = "All circuit breakers reset successfully"
	resp.Body.Count = count
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}
