# Data Model: Configuration Settings & Debug UI Consolidation

**Feature**: 006-config-settings-ui | **Date**: 2025-12-06

## Overview

This feature extends existing in-memory configuration structures. No database schema changes required. All new entities are held in memory and optionally persisted to YAML config file.

---

## 1. Core Entities

### UnifiedConfig

The root entity for the consolidated configuration API.

```go
// UnifiedConfig represents all configuration data in a single structure
type UnifiedConfig struct {
    Runtime  RuntimeConfig  `json:"runtime"`   // Modifiable at runtime
    Startup  StartupConfig  `json:"startup"`   // Read-only (requires restart)
    Meta     ConfigMeta     `json:"meta"`      // Metadata about config file
}
```

**Relationships:**
- Contains `RuntimeConfig` (1:1)
- Contains `StartupConfig` (1:1)
- Contains `ConfigMeta` (1:1)

---

### RuntimeConfig

Runtime-modifiable settings.

```go
// RuntimeConfig contains all settings that can be changed without restart
type RuntimeConfig struct {
    Settings        RuntimeSettings              `json:"settings"`
    Features        map[string]bool              `json:"features"`
    FeatureConfig   map[string]map[string]any    `json:"feature_config,omitempty"`
    CircuitBreakers CircuitBreakerConfig         `json:"circuit_breakers"`
}
```

**Fields:**

| Field | Type | Description | Validation |
|-------|------|-------------|------------|
| settings | RuntimeSettings | Core runtime settings | Required |
| features | map[string]bool | Feature flag toggles | Optional |
| feature_config | map[string]map[string]any | Feature-specific config | Optional |
| circuit_breakers | CircuitBreakerConfig | CB global + profiles | Required |

---

### RuntimeSettings

Core runtime settings (existing, unchanged).

```go
type RuntimeSettings struct {
    LogLevel             string `json:"log_level"`
    EnableRequestLogging bool   `json:"enable_request_logging"`
}
```

**Validation:**
- `log_level`: Must be one of: `trace`, `debug`, `info`, `warn`, `error`

---

### CircuitBreakerConfig

Circuit breaker configuration (existing, unchanged).

```go
type CircuitBreakerConfig struct {
    Global   CircuitBreakerProfileConfig            `json:"global"`
    Profiles map[string]CircuitBreakerProfileConfig `json:"profiles,omitempty"`
}

type CircuitBreakerProfileConfig struct {
    FailureThreshold      int           `json:"failure_threshold"`
    ResetTimeout          time.Duration `json:"reset_timeout"`
    HalfOpenMax           int           `json:"half_open_max"`
    AcceptableStatusCodes string        `json:"acceptable_status_codes,omitempty"`
}
```

---

### StartupConfig

Read-only startup configuration (existing, enhanced structure).

```go
type StartupConfig struct {
    Server    ServerConfig    `json:"server"`
    Database  DatabaseConfig  `json:"database"`
    Storage   StorageConfig   `json:"storage"`
    Pipeline  PipelineConfig  `json:"pipeline"`
    Scheduler SchedulerConfig `json:"scheduler"`
    Relay     RelayConfig     `json:"relay"`
    Ingestion IngestionConfig `json:"ingestion"`
}
```

**Note:** All fields read from Viper at startup. No setters - require restart to change.

---

### ConfigMeta

Metadata about the configuration file.

```go
type ConfigMeta struct {
    ConfigPath   string    `json:"config_path,omitempty"` // Path to config file
    CanPersist   bool      `json:"can_persist"`           // Write permission exists
    LastModified time.Time `json:"last_modified,omitempty"` // File modification time
    Source       string    `json:"source"`                // "file", "env", "defaults"
}
```

---

## 2. Circuit Breaker Stats Entities (Enhanced)

### CircuitBreakerStats

Enhanced stats structure with error categorization and history.

```go
type CircuitBreakerStats struct {
    // Identity
    Name string `json:"name"`

    // Current state
    State           CircuitState `json:"state"`            // "closed", "open", "half_open"
    StateEnteredAt  time.Time    `json:"state_entered_at"` // NEW
    StateDurationMs int64        `json:"state_duration_ms"` // NEW: ms in current state

    // Counters
    ConsecutiveFailures int   `json:"consecutive_failures"`
    ConsecutiveSuccesses int  `json:"consecutive_successes"`
    TotalRequests       int64 `json:"total_requests"`
    TotalSuccesses      int64 `json:"total_successes"`
    TotalFailures       int64 `json:"total_failures"`
    FailureRate         float64 `json:"failure_rate"` // Percentage

    // NEW: Error categorization
    ErrorCounts ErrorCategoryCount `json:"error_counts"`

    // NEW: State duration tracking
    StateDurations StateDurationSummary `json:"state_durations"`

    // NEW: Transition history
    Transitions []StateTransition `json:"transitions,omitempty"`

    // Timestamps
    LastFailure   time.Time `json:"last_failure,omitempty"`
    LastSuccess   time.Time `json:"last_success,omitempty"`

    // Recovery info (when open)
    NextHalfOpenAt time.Time `json:"next_half_open_at,omitempty"` // NEW

    // Config reference
    Config CircuitBreakerProfileConfig `json:"config"`
}
```

---

### ErrorCategoryCount

Breakdown of failures by HTTP status category.

```go
type ErrorCategoryCount struct {
    Success2xx     int64 `json:"success_2xx"`
    ClientError4xx int64 `json:"client_error_4xx"`
    ServerError5xx int64 `json:"server_error_5xx"`
    Timeout        int64 `json:"timeout"`
    NetworkError   int64 `json:"network_error"`
}
```

**State Transitions:**
- Incremented on each request completion
- Reset when breaker is manually reset

---

### StateDurationSummary

Cumulative time spent in each state.

```go
type StateDurationSummary struct {
    ClosedMs   int64   `json:"closed_ms"`
    OpenMs     int64   `json:"open_ms"`
    HalfOpenMs int64   `json:"half_open_ms"`
    TotalMs    int64   `json:"total_ms"`
    ClosedPct  float64 `json:"closed_pct"`  // Percentage
    OpenPct    float64 `json:"open_pct"`
    HalfOpenPct float64 `json:"half_open_pct"`
}
```

---

### StateTransition

Record of a state change.

```go
type StateTransition struct {
    Timestamp        time.Time    `json:"timestamp"`
    FromState        CircuitState `json:"from_state"`
    ToState          CircuitState `json:"to_state"`
    Reason           string       `json:"reason"`           // "threshold_exceeded", "timeout_recovery", "probe_success", "probe_failure", "manual_reset"
    ConsecutiveCount int          `json:"consecutive_count"` // Failures or successes at transition
}
```

**Storage:**
- Circular buffer with configurable max size (default: 50)
- Oldest transitions discarded when buffer full

---

## 3. Health Response Entities (Enhanced)

### LivezResponse

Lightweight liveness check response.

```go
type LivezResponse struct {
    Status string `json:"status"` // "ok"
}
```

---

### ReadyzResponse

Readiness check response with component status.

```go
type ReadyzResponse struct {
    Status     string            `json:"status"` // "ok" or "not_ready"
    Components map[string]string `json:"components,omitempty"` // Individual check results
}
```

**Components checked:**
- `database`: Ping succeeds
- `scheduler`: Running state

---

### HealthResponse (Enhanced)

Existing structure with enhanced circuit breaker data.

```go
type HealthResponse struct {
    // ... existing fields ...

    Components HealthComponents `json:"components"`
}

type HealthComponents struct {
    Database        DatabaseHealth                    `json:"database"`
    Scheduler       SchedulerHealth                   `json:"scheduler"`
    CircuitBreakers map[string]CircuitBreakerStats    `json:"circuit_breakers"` // ENHANCED
}
```

---

## 4. Request/Response Types

### UnifiedConfigUpdate

Request body for updating configuration.

```go
type UnifiedConfigUpdate struct {
    Settings        *RuntimeSettings              `json:"settings,omitempty"`
    Features        map[string]bool               `json:"features,omitempty"`
    CircuitBreakers *CircuitBreakerConfigUpdate   `json:"circuit_breakers,omitempty"`
}

type CircuitBreakerConfigUpdate struct {
    Global   *CircuitBreakerProfileConfig            `json:"global,omitempty"`
    Profiles map[string]CircuitBreakerProfileConfig  `json:"profiles,omitempty"`
}
```

**Behavior:**
- Omitted fields are not modified (partial update)
- Empty map clears all profiles
- `null` value removes specific profile

---

### ConfigPersistRequest

Request body for persisting config to file.

```go
type ConfigPersistRequest struct {
    // Empty - persists current runtime config
}
```

---

### ConfigPersistResponse

Response for persist operation.

```go
type ConfigPersistResponse struct {
    Success  bool     `json:"success"`
    Message  string   `json:"message"`
    Path     string   `json:"path,omitempty"`      // Path written
    Sections []string `json:"sections,omitempty"`  // Sections persisted
}
```

---

## 5. Entity Relationships

```
UnifiedConfig
├── RuntimeConfig
│   ├── RuntimeSettings
│   ├── Features (map)
│   ├── FeatureConfig (map)
│   └── CircuitBreakerConfig
│       ├── Global (CircuitBreakerProfileConfig)
│       └── Profiles (map[string]CircuitBreakerProfileConfig)
├── StartupConfig
│   ├── ServerConfig
│   ├── DatabaseConfig
│   ├── StorageConfig
│   ├── PipelineConfig
│   ├── SchedulerConfig
│   ├── RelayConfig
│   └── IngestionConfig
└── ConfigMeta

CircuitBreakerStats
├── ErrorCategoryCount
├── StateDurationSummary
├── StateTransition[] (circular buffer)
└── CircuitBreakerProfileConfig
```

---

## 6. State Machine: Circuit Breaker

```
                    ┌─────────────────────────────────────┐
                    │                                     │
                    ▼                                     │
┌──────────┐  threshold   ┌──────────┐  timeout   ┌───────────┐
│  CLOSED  │ ──────────── │   OPEN   │ ────────── │ HALF_OPEN │
└──────────┘   exceeded   └──────────┘  expired   └───────────┘
     ▲                                                  │
     │                                                  │
     │         probe succeeds (half_open_max reached)   │
     └──────────────────────────────────────────────────┘
                                │
                                │ probe fails
                                ▼
                           ┌──────────┐
                           │   OPEN   │
                           └──────────┘
```

**Transition Reasons:**
- `threshold_exceeded`: Consecutive failures >= threshold
- `timeout_recovery`: Reset timeout expired, attempting recovery
- `probe_success`: Successful request in half-open state
- `probe_failure`: Failed request in half-open state
- `manual_reset`: User triggered reset via API

---

## 7. Validation Rules

### RuntimeSettings

| Field | Rule |
|-------|------|
| log_level | Enum: trace, debug, info, warn, error |
| enable_request_logging | Boolean |

### CircuitBreakerProfileConfig

| Field | Rule |
|-------|------|
| failure_threshold | Integer >= 1 |
| reset_timeout | Duration string (e.g., "30s") |
| half_open_max | Integer >= 1 |
| acceptable_status_codes | Optional, comma-separated integers |

### Features

| Field | Rule |
|-------|------|
| * | Boolean values only |

---

## 8. No Database Changes

This feature operates entirely with in-memory structures. No migrations required.

**Rationale:**
- Config changes are rare (not high-volume data)
- In-memory is faster for health checks
- Circuit breaker stats reset on restart is acceptable
- Config persistence uses YAML file, not database
