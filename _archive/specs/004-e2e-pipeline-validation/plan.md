# Implementation Plan: End-to-End Pipeline Validation

**Branch**: `004-e2e-pipeline-validation` | **Date**: 2025-12-03 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-e2e-pipeline-validation/spec.md`

## Summary

End-to-end validation of the tvarr pipeline from source ingestion through output generation. This is a **testing/validation effort** to verify all pipeline stages work correctly, identify and fix bugs, and ensure the system meets its functional requirements for serving valid M3U and XMLTV outputs.

**Primary Focus Areas:**
1. Stream source ingestion (M3U and Xtream)
2. EPG source ingestion (XMLTV)
3. Stream proxy configuration with associations
4. Pipeline generation through all stages
5. M3U and XMLTV output serving

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma + Chi (web), GORM (ORM), slog (logging)
**Storage**: SQLite (development), PostgreSQL/MySQL (production)
**Testing**: testify + gomock, integration tests
**Target Platform**: Linux server (also darwin/amd64, darwin/arm64)
**Project Type**: Web application (Go backend + React frontend)
**Performance Goals**: Per constitution - 100k channels < 5min, 50k proxy generation < 2min
**Constraints**: <500MB memory during ingestion/generation
**Scale/Scope**: Testing with 1,000-10,000 channels, 10,000-100,000 EPG programs

## Constitution Check

*GATE: All constitutional principles verified*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Pipeline uses streaming/batching |
| II. Modular Pipeline Architecture | PASS | Stages implement common interface |
| III. Test-First Development | PARTIAL | Integration tests needed for E2E |
| IV. Clean Architecture | PASS | Repository/Service/Handler layers |
| V. Idiomatic Go | PASS | slog, context, explicit errors |
| VI. Observable and Debuggable | PASS | SSE progress, structured logging |
| VII. Security by Default | N/A | No new security surface |
| VIII. No Magic Strings | PASS | Constants for strings |
| IX. Resilient HTTP Clients | PASS | Circuit breaker, retries |
| X-XI. Human-Readable Config | N/A | No new config |
| XII. Production CI/CD | PASS | GitHub Actions pipeline |
| XIII. Test Data Standards | PASS | Fictional channel names |

**Violations:** None - this is a testing/validation effort

## Project Structure

### Documentation (this feature)

```text
specs/004-e2e-pipeline-validation/
├── plan.md              # This file
├── research.md          # Pipeline status investigation
├── test-scenarios.md    # E2E test scenarios
└── tasks.md             # Testing and fix tasks
```

### Source Code (existing structure)

```text
# Backend (Go)
internal/
├── ingestion/           # Source ingestion (M3U, Xtream, XMLTV)
├── pipeline/
│   ├── core/            # Pipeline orchestrator
│   └── stages/          # Pipeline stages (filtering, mapping, etc.)
├── service/             # Business logic (proxy generation)
├── http/handlers/       # API endpoints
├── repository/          # Database access
└── models/              # Data models

# Frontend (React)
frontend/src/
├── components/          # UI components
├── pages/               # Page components
└── lib/                 # API client, utilities

# Tests
internal/*/..._test.go   # Unit tests
tests/                   # Integration tests (if present)
```

**Structure Decision**: Existing web application structure with Go backend and React frontend. No structural changes needed - this is validation work.

## Existing Pipeline Stages

The pipeline executes these stages in order during proxy generation:

1. **LoadPrograms** - Load channels from database for selected sources
2. **Filtering** - Apply filter rules to include/exclude channels
3. **DataMapping** - Transform channel metadata via mapping rules
4. **Numbering** - Assign channel numbers (sequential/preserve/group)
5. **LogoCaching** - Cache channel/program logos if enabled
6. **GenerateM3U** - Render M3U playlist output
7. **GenerateXMLTV** - Render XMLTV guide output
8. **Publish** - Write outputs to configured paths

## Known Issues (from spec)

- **BUG-001**: Stream Proxy creation 422 validation error - **FIXED** (committed)
- **BUG-002**: Proxy edit doesn't load existing associations - **FIXED** (committed)

## Testing Approach

### Phase 1: Manual E2E Validation

1. Start clean database
2. Add stream source (M3U)
3. Trigger ingestion, verify channels
4. Add EPG source (XMLTV)
5. Trigger ingestion, verify programs
6. Create stream proxy with sources/EPG
7. Generate proxy output
8. Verify M3U endpoint serves valid playlist
9. Verify XMLTV endpoint serves valid guide

### Phase 2: Edge Case Testing

1. Large source handling (5k+ channels)
2. EPG matching to channels
3. Filter/mapping rule application
4. Logo caching behavior
5. Error handling and reporting

### Phase 3: Integration Tests

Add automated E2E tests covering the critical paths.

## Complexity Tracking

> No violations - this is a testing/validation effort with no new complexity.

| Item | Notes |
|------|-------|
| Bug fixes | Already committed (BUG-001, BUG-002) |
| Pipeline stages | All existing, well-tested |
| Output serving | Endpoints exist, need validation |

## Next Steps

1. Execute manual E2E validation (Phase 1)
2. Document any discovered issues
3. Fix issues as found
4. Add integration tests for critical paths
