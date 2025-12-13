# Implementation Plan: Relay Profile Simplification

**Branch**: `014-relay-profile-simplify` | **Date**: 2025-12-12 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/014-relay-profile-simplify/spec.md`

## Summary

Simplify the relay profile system by replacing the complex 40+ field RelayProfile model with a streamlined EncodingProfile (~6 fields), immediately removing RelayProfileMapping, and leveraging built-in client detection rules. The two-tier system (Proxy Mode: direct/smart + Encoding Mode: auto/profile) reduces configuration complexity while maintaining automatic device optimization through existing expression engine patterns.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), gohlslib v2
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: testify + gomock, E2E runner (cmd/e2e-runner)
**Target Platform**: Linux server (primary), macOS, Windows
**Project Type**: Web application (Go backend + Next.js 16.x frontend)
**Performance Goals**: Client detection and routing decisions in <10ms per request
**Constraints**: Maintain backward compatibility for existing StreamProxy configurations during migration
**Scale/Scope**: Reduce encoding profile fields by ≥75% (from 40+ to <10)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | No changes to streaming architecture; same memory patterns |
| II. Modular Pipeline Architecture | PASS | Simplifying models doesn't affect pipeline stages |
| III. Test-First Development | PASS | TR-001 to TR-006, ER-001 to ER-006 define test requirements |
| IV. Clean Architecture with SOLID | PASS | EncodingProfile is simpler, follows same repository pattern |
| V. Idiomatic Go | PASS | Same conventions apply |
| VI. Observable and Debuggable | PASS | Structured logging for detection rule matching |
| VII. Security by Default | PASS | No security changes |
| VIII. No Magic Strings or Literals | PASS | Codec values already constants in models |
| IX. Resilient HTTP Clients | N/A | No external HTTP changes |
| X. Human-Readable Duration Config | PASS | No new durations |
| XI. Human-Readable Byte Size Config | PASS | No new byte sizes |
| XII. Production-Grade CI/CD | PASS | Migration tested via existing pipeline |
| XIII. Test Data Standards | PASS | E2E tests use fictional data |

## Project Structure

### Documentation (this feature)

```text
specs/014-relay-profile-simplify/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── models/
│   ├── encoding_profile.go     # NEW: Simplified EncodingProfile model
│   ├── relay_profile.go        # MODIFY: Deprecate, migrate to EncodingProfile
│   └── relay_profile_mapping.go # REMOVE: Delete after migration
├── repository/
│   ├── encoding_profile_repo.go # NEW: Repository for EncodingProfile
│   └── relay_profile_mapping_repo.go # REMOVE
├── service/
│   ├── encoding_profile_service.go # NEW: Service layer
│   └── relay_profile_mapping_service.go # REMOVE
├── http/handlers/
│   ├── encoding_profile.go     # NEW: API handlers
│   └── relay_profile_mapping.go # REMOVE
├── relay/
│   └── format_router.go        # MODIFY: Update client detection logic
├── database/migrations/
│   └── migrations.go           # MODIFY: Add migration for schema changes
└── expression/
    └── dynamic_field.go        # VERIFY: @header_req works for X-Video-Codec

cmd/
└── e2e-runner/
    └── main.go                 # MODIFY: Add client detection test mode

# Frontend (Next.js/React)
frontend/src/
├── components/
│   ├── encoding-profile-form.tsx  # NEW: Simplified profile editor
│   ├── relay-profile-form.tsx     # REMOVE
│   ├── proxies.tsx                # MODIFY: Pre-select sources/filters
│   └── CreateProxyModal.tsx       # MODIFY: Smart defaults
├── app/admin/
│   ├── encoding-profiles/page.tsx # NEW: Admin page
│   └── relays/page.tsx            # MODIFY: Redirect to encoding-profiles
└── lib/api/
    └── encoding-profiles.ts       # NEW: API client

tests/
├── integration/
│   └── encoding_profile_test.go   # NEW: Integration tests
└── unit/
    └── client_detection_test.go   # NEW: Unit tests for detection rules
```

**Structure Decision**: Web application structure with Go backend and Next.js frontend. Changes primarily affect the models layer with cascading updates to repositories, services, handlers, and frontend components.

## Complexity Tracking

> No constitution violations requiring justification.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | N/A | N/A |
