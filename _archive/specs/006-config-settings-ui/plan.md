# Implementation Plan: Configuration Settings & Debug UI Consolidation

**Branch**: `006-config-settings-ui` | **Date**: 2025-12-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/006-config-settings-ui/spec.md`

## Summary

Consolidate configuration management into a unified API and improve debug UI with circuit breaker visualization. Key deliverables:
- Unified `/api/v1/config` endpoint (replaces 10+ fragmented endpoints)
- Rich circuit breaker visualization with error breakdown and state history
- K8s-aligned health endpoints (`/livez`, `/readyz`)
- Config persistence to YAML file
- Debug page error resilience (fix undefined access crashes)

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, Viper (config)
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM), YAML config files
**Testing**: testify + gomock, table-driven tests
**Target Platform**: Linux server (also darwin/amd64, darwin/arm64)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: API Response < 200ms p95, `/livez` < 100ms
**Constraints**: Memory < 500MB, config save < 5s
**Scale/Scope**: Settings page, debug page, 3 new API endpoints, ~15 files modified

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | ✅ PASS | No large datasets; config/health data is small |
| II. Modular Pipeline | ✅ PASS | Not applicable (no pipeline stages added) |
| III. Test-First (NON-NEGOTIABLE) | ⚠️ REQUIRED | Tests must be written before implementation |
| IV. Clean Architecture + SOLID | ✅ PASS | Handler → Service → Repository pattern maintained |
| V. Idiomatic Go | ✅ PASS | Standard patterns, context propagation, slog |
| VI. Observable and Debuggable | ✅ PASS | Health endpoints, circuit breaker metrics exposed |
| VII. Security by Default | ✅ PASS | Config file permissions checked before write |
| VIII. No Magic Strings | ✅ PASS | Use constants for endpoint paths, config keys |
| IX. Resilient HTTP Clients | ✅ PASS | Circuit breaker manager already exists |
| X. Human-Readable Duration | ✅ PASS | Use pkg/duration for any duration configs |
| XI. Human-Readable Byte Size | N/A | No byte size configs in this feature |
| XII. Production-Grade CI/CD | ✅ PASS | No changes to CI/CD pipeline |
| XIII. Test Data Standards | ✅ PASS | Fictional data for any test circuit breakers |

**Gate Status**: ✅ PASS - No violations requiring justification

## Project Structure

### Documentation (this feature)

```text
specs/006-config-settings-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (OpenAPI specs)
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── http/handlers/
│   ├── config.go           # NEW: Unified config handler
│   ├── health.go           # MODIFY: Add /livez, /readyz
│   └── settings.go         # DEPRECATE: Migrate to config.go
├── service/
│   └── config.go           # NEW: Config persistence service
└── config/
    └── persistence.go      # NEW: YAML file read/write

pkg/httpclient/
├── circuit_breaker.go      # MODIFY: Add error categorization
├── manager.go              # MODIFY: Add state transition tracking
└── stats.go                # NEW: Enhanced stats with history

# Frontend (Next.js)
frontend/src/
├── components/
│   ├── settings.tsx        # MODIFY: Use unified config API
│   ├── debug.tsx           # MODIFY: Error resilience, CB visualization
│   └── circuit-breaker/    # NEW: Rich CB visualization components
│       ├── SegmentedBar.tsx
│       ├── StateTimeline.tsx
│       └── CircuitBreakerCard.tsx
├── providers/
│   └── backend-connectivity-provider.tsx  # MODIFY: /live → /livez
└── lib/
    └── api.ts              # MODIFY: Use unified config endpoint
```

**Structure Decision**: Web application with Go backend and Next.js frontend. Follows existing handler → service → repository pattern. New config handler consolidates existing settings, features, and circuit-breaker config handlers.

## Complexity Tracking

> No violations requiring justification. Constitution check passed.
