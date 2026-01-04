# Research: Configuration Settings & Debug UI Consolidation

**Feature**: 006-config-settings-ui | **Date**: 2025-12-06

## Executive Summary

Research findings confirm the spec requirements are achievable with existing patterns. The codebase has well-established handler/service/repository architecture that we'll extend for the unified config endpoint. Key gaps identified: no `/livez` endpoint, no circuit breaker error categorization, no state transition history.

---

## 1. Existing Endpoint Inventory

### Current Config-Related Endpoints (10 total)

| Endpoint | Method | Handler | Purpose |
|----------|--------|---------|---------|
| `/api/v1/settings` | GET | SettingsHandler | Runtime settings (log_level, request_logging) |
| `/api/v1/settings` | PUT | SettingsHandler | Update runtime settings |
| `/api/v1/settings/info` | GET | SettingsHandler | Settings metadata (types, options) |
| `/api/v1/settings/startup` | GET | SettingsHandler | Read-only startup config |
| `/api/v1/features` | GET | FeatureHandler | Feature flags |
| `/api/v1/features` | PUT | FeatureHandler | Update feature flags |
| `/api/v1/circuit-breakers/config` | GET | CircuitBreakerHandler | CB config + status |
| `/api/v1/circuit-breakers/config` | PUT | CircuitBreakerHandler | Update CB config |
| `/api/v1/circuit-breakers/{name}/reset` | POST | CircuitBreakerHandler | Reset single breaker |
| `/api/v1/circuit-breakers/reset` | POST | CircuitBreakerHandler | Reset all breakers |
| `/health` | GET | HealthHandler | Health metrics |

### Health Endpoints Status

| Endpoint | Status | Notes |
|----------|--------|-------|
| `/health` | EXISTS | Full health data (CPU, memory, DB, circuit breakers) |
| `/live` | MISSING | Frontend calls this but it doesn't exist |
| `/livez` | MISSING | K8s liveness probe needed |
| `/readyz` | MISSING | K8s readiness probe needed |

---

## 2. Circuit Breaker Implementation Analysis

### Current Stats Structure

```go
type CircuitBreakerStats struct {
    State                 CircuitState  // Closed/Open/HalfOpen
    Failures              int           // Consecutive failures
    Successes             int           // Consecutive successes (half-open)
    TotalRequests         int64         // Total request count
    TotalSuccesses        int64         // Total successful
    TotalFailures         int64         // Total failed
    LastFailure           time.Time     // Last failure timestamp
    Config                CircuitBreakerProfileConfig
}
```

### Gap Analysis: What's Missing for Spec

| Requirement | Current State | Action Needed |
|-------------|---------------|---------------|
| Error breakdown (2xx/4xx/5xx/timeout/network) | Only total failures | Add error categorization by HTTP status |
| State duration ("Closed for 2h 15m") | Not tracked | Add `StateEnteredAt time.Time` |
| State transition history | Not tracked | Add circular buffer of transitions |
| Time spent in each state | Not tracked | Add cumulative duration per state |
| Recovery countdown (time to Half-Open) | Not exposed | Calculate from `LastFailure + ResetTimeout` |

### Proposed Enhanced Stats Structure

```go
type CircuitBreakerStats struct {
    // Existing fields
    State           CircuitState
    Failures        int
    Successes       int
    TotalRequests   int64
    TotalSuccesses  int64
    TotalFailures   int64
    LastFailure     time.Time
    Config          CircuitBreakerProfileConfig

    // NEW: Error categorization
    ErrorCounts     ErrorCategoryCount  // By HTTP status category

    // NEW: State tracking
    StateEnteredAt  time.Time           // When current state began
    StateDurations  map[CircuitState]time.Duration  // Cumulative time in each state

    // NEW: Transition history
    Transitions     []StateTransition   // Circular buffer (last 50)
}

type ErrorCategoryCount struct {
    Success2xx    int64
    ClientError4xx int64
    ServerError5xx int64
    Timeout       int64
    NetworkError  int64
}

type StateTransition struct {
    Timestamp       time.Time
    FromState       CircuitState
    ToState         CircuitState
    Reason          string  // "threshold_exceeded", "timeout_recovery", "manual_reset"
    ConsecutiveCount int    // Failures or successes at transition
}
```

---

## 3. API Consolidation Decision

### Decision: Create Unified `/api/v1/config` Endpoint

**Rationale:**
- Frontend currently makes 4+ API calls to load settings page
- Spec requires reducing to 2 calls maximum (SC-008)
- Single response structure simplifies frontend state management

### Unified Response Structure

```go
type UnifiedConfigResponse struct {
    Runtime struct {
        Settings     RuntimeSettings           // log_level, request_logging
        Features     map[string]bool           // Feature flags
        CircuitBreakers CircuitBreakerConfig   // Global + profiles
    }
    Startup StartupConfig  // Read-only startup settings
    Meta struct {
        ConfigPath   string    // Path to config file (if known)
        CanPersist   bool      // Whether config file is writable
        LastModified time.Time // Config file modification time
    }
}
```

### Migration Strategy

1. **Phase 1**: Add new `/api/v1/config` endpoint alongside existing endpoints
2. **Phase 2**: Update frontend to use new endpoint
3. **Phase 3**: Mark old endpoints as deprecated (return warning header)
4. **Future**: Remove deprecated endpoints (out of scope)

---

## 4. Config Persistence Research

### Current State

- Settings and feature flags are **in-memory only** (reset on restart)
- Viper loads config from file at startup but doesn't write back
- No API endpoint for config persistence

### Decision: YAML Persistence via Viper

**Rationale:**
- Viper already used for config loading
- Viper supports `WriteConfig()` for persistence
- YAML format matches existing config files

### Implementation Approach

```go
// service/config/persistence.go
type ConfigPersister struct {
    configPath string
    viper      *viper.Viper
}

func (p *ConfigPersister) CanPersist() (bool, error) {
    // Check file permissions
    return os.Access(p.configPath, os.O_WRONLY) == nil, nil
}

func (p *ConfigPersister) Persist(ctx context.Context, config UnifiedConfigUpdate) error {
    // 1. Validate write permissions
    // 2. Update viper values
    // 3. Write config file
    // 4. Return success/failure
}
```

### Considerations

- **Preserve comments**: Viper loses comments on write (acceptable trade-off)
- **Atomic writes**: Write to temp file, then rename
- **Validation**: Validate config before writing

---

## 5. Frontend Patterns

### Current API Client

```typescript
// frontend/src/lib/api-client.ts
export const apiClient = {
  get: async <T>(endpoint: string) => { ... },
  put: async <T>(endpoint: string, data: any) => { ... },
  post: async <T>(endpoint: string, data: any) => { ... },
};
```

### Decision: Extend Existing Client

No changes to api-client.ts needed. Frontend will call new endpoint using existing patterns.

### Component Structure for Circuit Breaker Visualization

```
frontend/src/components/circuit-breaker/
├── index.ts              # Exports
├── CircuitBreakerCard.tsx   # Main card component
├── SegmentedProgressBar.tsx # Success/error breakdown
├── StateIndicator.tsx       # Closed/Open/Half-Open badge
├── StateTimeline.tsx        # Transition history
└── types.ts                 # TypeScript interfaces
```

---

## 6. Health Endpoint Design

### Decision: Add `/livez` and `/readyz`

**`/livez` (Liveness)**
- Purpose: "Is the process running?"
- Response: `200 OK` with `{"status":"ok"}` or `503` if hung
- Checks: None (always returns OK if reachable)
- Use: UI polling, K8s liveness probe

**`/readyz` (Readiness)**
- Purpose: "Can we serve traffic?"
- Response: `200 OK` or `503 Service Unavailable`
- Checks: Database connection, scheduler running
- Use: K8s readiness probe

**`/health` (Detailed)**
- Keep as-is with enhanced circuit breaker stats
- Not for K8s probes (too heavy)

### Implementation

```go
// handlers/health.go

func (h *HealthHandler) GetLivez(ctx context.Context, input *LivezInput) (*LivezOutput, error) {
    return &LivezOutput{Body: LivezResponse{Status: "ok"}}, nil
}

func (h *HealthHandler) GetReadyz(ctx context.Context, input *ReadyzInput) (*ReadyzOutput, error) {
    // Check database
    if err := h.db.PingContext(ctx); err != nil {
        return nil, huma.Error503ServiceUnavailable("database not ready")
    }
    // Check scheduler (if reference available)
    return &ReadyzOutput{Body: ReadyzResponse{Status: "ok"}}, nil
}
```

---

## 7. Alternatives Considered

### API Design Alternatives

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| Keep separate endpoints | No changes needed | Too many API calls, spec violation | REJECTED |
| GraphQL | Flexible queries | Major architectural change | REJECTED |
| Unified REST endpoint | Simple, follows spec | Larger response | SELECTED |

### Circuit Breaker Stats Storage

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| In-memory only | Simple, fast | Lost on restart | SELECTED |
| Database persistence | Survives restart | Complexity, performance | REJECTED |
| Redis cache | Fast, survives restart | External dependency | REJECTED |

### Config Persistence

| Option | Pros | Cons | Decision |
|--------|------|------|----------|
| Viper WriteConfig | Built-in, YAML support | Loses comments | SELECTED |
| Direct file write | Preserve formatting | Manual YAML handling | REJECTED |
| Database storage | Survives restarts | Config file not updated | REJECTED |

---

## 8. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Config file permissions | Medium | Medium | Check permissions before save, clear error message |
| Circuit breaker memory growth | Low | Medium | Circular buffer limits (50 transitions) |
| API backward compatibility | Medium | High | Keep deprecated endpoints during transition |
| Frontend migration complexity | Low | Medium | Feature flag to switch API endpoints |

---

## 9. Key Files to Modify

### Backend (Go)

| File | Action | Changes |
|------|--------|---------|
| `internal/http/handlers/config.go` | NEW | Unified config handler |
| `internal/http/handlers/health.go` | MODIFY | Add /livez, /readyz |
| `internal/service/config/persistence.go` | NEW | Config file writer |
| `pkg/httpclient/circuit_breaker.go` | MODIFY | Add error categorization |
| `pkg/httpclient/stats.go` | NEW | Enhanced stats with history |
| `cmd/tvarr/cmd/serve.go` | MODIFY | Register new handlers |

### Frontend (Next.js)

| File | Action | Changes |
|------|--------|---------|
| `frontend/src/components/settings.tsx` | MODIFY | Use unified config API |
| `frontend/src/components/debug.tsx` | MODIFY | Error resilience, CB viz |
| `frontend/src/components/circuit-breaker/` | NEW | Visualization components |
| `frontend/src/providers/backend-connectivity-provider.tsx` | MODIFY | /live → /livez |
| `frontend/src/types/api.ts` | MODIFY | Add unified config types |

---

## 10. Conclusion

All NEEDS CLARIFICATION items resolved. The implementation follows existing patterns and requires no architectural changes. Key decisions:

1. **Unified API**: Single `/api/v1/config` endpoint with structured response
2. **K8s Health**: Add `/livez` (liveness) and `/readyz` (readiness)
3. **Circuit Breaker Stats**: Extend in-memory tracking with error categories and state history
4. **Config Persistence**: Use Viper's WriteConfig with permission checking
5. **Migration**: Keep deprecated endpoints during transition period
