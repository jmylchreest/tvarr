# Quickstart: Configuration Settings & Debug UI Consolidation

**Feature**: 006-config-settings-ui | **Date**: 2025-12-06

## Overview

This guide provides quick reference for implementing the configuration settings and debug UI consolidation feature.

---

## Key Endpoints

### New Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/config` | GET | Get all config (replaces 4+ calls) |
| `/api/v1/config` | PUT | Update runtime config |
| `/api/v1/config/persist` | POST | Save config to file |
| `/livez` | GET | Liveness probe (UI + K8s) |
| `/readyz` | GET | Readiness probe (K8s) |

### Deprecated Endpoints

These continue to work but return deprecation warnings:

- `GET /api/v1/settings` → Use `GET /api/v1/config`
- `PUT /api/v1/settings` → Use `PUT /api/v1/config`
- `GET /api/v1/features` → Use `GET /api/v1/config`
- `PUT /api/v1/features` → Use `PUT /api/v1/config`
- `GET /api/v1/circuit-breakers/config` → Use `GET /api/v1/config`
- `PUT /api/v1/circuit-breakers/config` → Use `PUT /api/v1/config`

---

## Quick Examples

### Get All Configuration

```bash
curl http://localhost:8080/api/v1/config | jq
```

Response structure:
```json
{
  "success": true,
  "runtime": {
    "settings": { "log_level": "info", "enable_request_logging": false },
    "features": { "debug-frontend": false },
    "circuit_breakers": { "global": {...}, "profiles": {...} }
  },
  "startup": {
    "server": {...},
    "database": {...},
    "storage": {...}
  },
  "meta": {
    "config_path": "/etc/tvarr/config.yaml",
    "can_persist": true,
    "source": "file"
  }
}
```

### Update Log Level

```bash
curl -X PUT http://localhost:8080/api/v1/config \
  -H "Content-Type: application/json" \
  -d '{"settings": {"log_level": "debug"}}'
```

### Save Config to File

```bash
curl -X POST http://localhost:8080/api/v1/config/persist
```

### Liveness Check

```bash
curl http://localhost:8080/livez
# {"status":"ok"}
```

### Readiness Check

```bash
curl http://localhost:8080/readyz
# {"status":"ok","components":{"database":"ok","scheduler":"ok"}}
```

---

## Implementation Checklist

### Backend Tasks

1. **Create unified config handler** (`internal/http/handlers/config.go`)
   - [ ] Implement `GET /api/v1/config`
   - [ ] Implement `PUT /api/v1/config`
   - [ ] Implement `POST /api/v1/config/persist`

2. **Add health endpoints** (`internal/http/handlers/health.go`)
   - [ ] Add `/livez` endpoint
   - [ ] Add `/readyz` endpoint

3. **Enhance circuit breaker stats** (`pkg/httpclient/`)
   - [ ] Add error categorization (2xx/4xx/5xx/timeout/network)
   - [ ] Add state transition tracking
   - [ ] Add state duration tracking

4. **Config persistence** (`internal/service/config/`)
   - [ ] Check file permissions
   - [ ] Write config via Viper

5. **Register handlers** (`cmd/tvarr/cmd/serve.go`)
   - [ ] Register new config handler
   - [ ] Register livez/readyz routes

### Frontend Tasks

1. **Migrate API calls** (`frontend/src/components/settings.tsx`)
   - [ ] Use unified `/api/v1/config` endpoint
   - [ ] Update types

2. **Fix connectivity provider** (`frontend/src/providers/backend-connectivity-provider.tsx`)
   - [ ] Change `/live` to `/livez`

3. **Debug page resilience** (`frontend/src/components/debug.tsx`)
   - [ ] Add optional chaining for undefined data
   - [ ] Add "Not available" placeholders

4. **Circuit breaker visualization** (`frontend/src/components/circuit-breaker/`)
   - [ ] Create `SegmentedProgressBar.tsx`
   - [ ] Create `StateIndicator.tsx`
   - [ ] Create `StateTimeline.tsx`
   - [ ] Create `CircuitBreakerCard.tsx`

---

## File Locations

### Backend (Go)

```
internal/http/handlers/
├── config.go           # NEW: Unified config handler
├── health.go           # MODIFY: Add livez/readyz

internal/service/config/
└── persistence.go      # NEW: Config file writer

pkg/httpclient/
├── circuit_breaker.go  # MODIFY: Error categorization
├── manager.go          # MODIFY: State tracking
└── stats.go            # NEW: Enhanced stats
```

### Frontend (Next.js)

```
frontend/src/components/
├── settings.tsx        # MODIFY: Use unified API
├── debug.tsx           # MODIFY: Error resilience
└── circuit-breaker/    # NEW: CB visualization
    ├── SegmentedProgressBar.tsx
    ├── StateIndicator.tsx
    ├── StateTimeline.tsx
    └── CircuitBreakerCard.tsx

frontend/src/providers/
└── backend-connectivity-provider.tsx  # MODIFY: /live → /livez
```

---

## Testing

### Backend Unit Tests

```bash
# Test unified config handler
go test ./internal/http/handlers/... -run TestConfig

# Test circuit breaker stats
go test ./pkg/httpclient/... -run TestCircuitBreaker

# Test config persistence
go test ./internal/service/config/... -run TestPersist
```

### Frontend Tests

```bash
cd frontend
pnpm test -- --grep "settings"
pnpm test -- --grep "circuit-breaker"
```

### Manual Testing

1. Start server: `task run`
2. Open debug page: `http://localhost:8080/debug`
3. Open settings page: `http://localhost:8080/settings`
4. Test liveness: `curl http://localhost:8080/livez`
5. Test config persistence: `curl -X POST http://localhost:8080/api/v1/config/persist`

---

## Common Patterns

### Handler Pattern (Go)

```go
type ConfigHandler struct {
    configService *service.ConfigService
    cbManager     *httpclient.CircuitBreakerManager
}

func NewConfigHandler(cs *service.ConfigService, cbm *httpclient.CircuitBreakerManager) *ConfigHandler {
    return &ConfigHandler{configService: cs, cbManager: cbm}
}

func (h *ConfigHandler) Register(api huma.API) {
    huma.Register(api, huma.Operation{
        OperationID: "getConfig",
        Method:      "GET",
        Path:        "/api/v1/config",
        Summary:     "Get unified configuration",
        Tags:        []string{"Configuration"},
    }, h.GetConfig)
    // ... more registrations
}
```

### Error Resilient Rendering (React)

```tsx
// Good: Optional chaining prevents crashes
<span>{healthData?.cpu_info?.load_1min?.toFixed(2) ?? 'N/A'}</span>

// Good: Conditional rendering
{healthData?.components?.circuit_breakers && (
  <CircuitBreakerList breakers={healthData.components.circuit_breakers} />
)}

// Bad: Will crash if undefined
<span>{healthData.cpu_info.load_1min.toFixed(2)}</span>
```

### Circuit Breaker State Colors

```tsx
const stateColors = {
  closed: 'bg-green-500',    // Healthy
  open: 'bg-red-500',        // Failing
  half_open: 'bg-amber-500', // Recovering
};
```

---

## Performance Targets

| Metric | Target |
|--------|--------|
| `/livez` response | < 100ms |
| `/api/v1/config` response | < 200ms |
| Config persist | < 5s |
| Settings page load | 2 API calls max |

---

## Troubleshooting

### Config persist fails with 403

Check file permissions:
```bash
ls -la /etc/tvarr/config.yaml
# Ensure write permission for tvarr user
```

### Circuit breaker not showing in debug page

1. Check if any requests have been made through the breaker
2. Verify breaker is registered in manager
3. Check health endpoint includes circuit_breakers

### Frontend shows "backend unreachable"

1. Verify `/livez` endpoint exists
2. Check CORS configuration
3. Verify backend is running on expected port
