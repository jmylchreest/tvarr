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
