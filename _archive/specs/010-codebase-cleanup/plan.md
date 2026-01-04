# Implementation Plan: Codebase Cleanup & Migration Compaction

**Branch**: `010-codebase-cleanup` | **Date**: 2025-12-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/010-codebase-cleanup/spec.md`

## Summary

Thorough codebase review to remove dead code, eliminate unnecessary method wrappers, and compact 34 database migrations into 2-3 migrations. This is a refactoring effort with no new features - focused on reducing technical debt while preserving all existing functionality and API contracts.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary)
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: Go testing with testify, gomock; E2E tests with e2e-runner
**Target Platform**: Linux server (primary), macOS, Windows (cross-platform)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: No regression from current performance; DB init < 1 second
**Constraints**: API backward compatibility required; no functionality changes
**Scale/Scope**: ~291 Go files, 34 migrations (1766 lines), 422 ESLint warnings, ~100 golangci-lint warnings

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Design Check (Phase 0)

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | N/A | Refactoring only, no new data patterns |
| II. Modular Pipeline Architecture | PASS | Preserving existing pipeline patterns |
| III. Test-First Development | PASS | Existing tests must continue to pass |
| IV. Clean Architecture with SOLID | PASS | Cleanup improves SOLID compliance |
| V. Idiomatic Go | PASS | Removing dead code improves idiomaticity |
| VI. Observable and Debuggable | N/A | No changes to observability |
| VII. Security by Default | N/A | No security-relevant changes |
| VIII. No Magic Strings | N/A | Not adding new strings |
| IX. Resilient HTTP Clients | N/A | No HTTP client changes |
| X. Human-Readable Duration Config | N/A | No duration config changes |
| XI. Human-Readable Byte Size Config | N/A | No byte size config changes |
| XII. Production-Grade CI/CD | PASS | Build/test must pass after changes |
| XIII. Test Data Standards | N/A | Not adding test data |

**PRE-DESIGN GATE STATUS**: PASS

### Post-Design Check (Phase 1)

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Adding pre-allocation and cleanup patterns |
| II. Modular Pipeline Architecture | PASS | No changes to pipeline structure |
| III. Test-First Development | PASS | All existing tests must pass |
| IV. Clean Architecture with SOLID | PASS | Removing dead code/wrappers improves SRP |
| V. Idiomatic Go | PASS | Adding proper error handling, using clear(), errors.Is |
| VI. Observable and Debuggable | PASS | No logging changes |
| VII. Security by Default | N/A | No security changes |
| VIII. No Magic Strings | N/A | Not adding strings |
| IX. Resilient HTTP Clients | N/A | No client changes |
| X. Human-Readable Duration Config | N/A | No config changes |
| XI. Human-Readable Byte Size Config | N/A | No config changes |
| XII. Production-Grade CI/CD | PASS | Zero lint warnings target |
| XIII. Test Data Standards | N/A | Not adding test data |

**POST-DESIGN GATE STATUS**: PASS - All principles maintained or improved.

## Project Structure

### Documentation (this feature)

```text
specs/010-codebase-cleanup/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (minimal - no new entities)
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (no changes - API preserved)
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
# Backend (Go) - Primary focus of cleanup
cmd/
├── tvarr/               # Main application
│   └── cmd/             # Cobra commands
└── e2e-runner/          # E2E test runner

internal/
├── assets/              # Embedded assets
├── config/              # Configuration
├── database/
│   └── migrations/      # PRIMARY TARGET: 34 migrations → 2-3
├── expression/          # Expression engine
├── ffmpeg/              # FFmpeg integration
├── http/
│   └── handlers/        # HTTP handlers
├── ingestion/           # Data ingestion
├── models/              # GORM models
├── pipeline/            # Processing pipeline
├── relay/               # Relay streaming
├── repository/          # Data access layer
├── scheduler/           # Job scheduler
└── xtream/              # Xtream API

pkg/
├── bytesize/            # Byte size parsing
├── duration/            # Duration parsing
├── httpclient/          # HTTP client wrapper
└── paths/               # Path utilities

# Frontend (Out of scope for this cleanup)
frontend/
└── [Next.js application - not modified]
```

**Structure Decision**: Existing Go project structure preserved. Primary changes in:
1. `internal/database/migrations/registry.go` - Compact 34 migrations to 2-3
2. Various packages - Remove dead code and unnecessary wrappers

## Complexity Tracking

> No violations requiring justification - this is a simplification effort.

## Cleanup Categories

### Category 1: Migration Compaction (High Impact)

**Current State**: 34 migrations in `registry.go` (1766 lines)

**Target State**: 2-3 migrations:
- Migration 001: Schema creation (all tables, indexes, constraints)
- Migration 002: System data (default profiles, filters, rules, mappings)

**Approach**:
1. Capture current database schema from running all 34 migrations
2. Create single schema migration using AutoMigrate for all models
3. Create data seeding migration with all default records
4. Remove individual migration functions
5. Verify E2E tests pass with new migrations

### Category 2: Dead Code Removal (Medium Impact)

**Tools**:
- `go build` - Identifies unused imports
- `golangci-lint` with `deadcode`, `unused` linters
- Manual review for interface implementations

**Target Areas**:
1. Repository layer - Check for unused methods
2. Handler layer - Check for unused handlers
3. Expression engine - Check for unused operators
4. Models - Check for unused fields/enums

### Category 3: Wrapper Method Elimination (Medium Impact)

**Pattern to Identify**:
```go
// BAD: Wrapper that just calls another method
func (r *Repo) FindByID(id string) (*Model, error) {
    return r.FindByIDWithOptions(id, DefaultOptions())
}

// GOOD: Only keep the method with options if callers need it
func (r *Repo) FindByID(id string, opts ...Option) (*Model, error) {
    options := applyOptions(opts...)
    // actual implementation
}
```

**Review Method**:
1. Identify methods that are single-line calls to other methods
2. Analyze if the wrapper provides abstraction value
3. If not, inline or remove the wrapper
4. Update all call sites

### Category 4: Code Consistency (Low Impact)

**Patterns to Standardize**:
1. Error handling in repositories
2. Request validation in handlers
3. Field naming in models (if inconsistent)

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Break existing functionality | Medium | High | Run full test suite after each change |
| Break API contracts | Low | High | No handler signature changes |
| Data loss in migrations | Low | Critical | Test on fresh DB first; no production |
| Missing interface impl | Low | Medium | Compiler catches missing methods |

## Validation Strategy

1. **After each change**:
   - `go build ./...`
   - `task test`
   - `task lint`

2. **After migration compaction**:
   - Fresh DB test: Delete DB, run migrations, verify schema
   - E2E test: `task e2e:full`
   - Compare schema before/after

3. **Final validation**:
   - Full CI pipeline pass
   - All existing tests pass
   - No new lint warnings
   - Frontend still works with backend
