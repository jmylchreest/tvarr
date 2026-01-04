# Implementation Plan: Codebase Cleanup and Refactoring

**Branch**: `015-codebase-cleanup` | **Date**: 2025-12-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/015-codebase-cleanup/spec.md`

## Summary

This feature focuses on four areas: (1) sensitive data redaction in logs using m-mizutani/masq slog middleware, (2) migrating the mediacommon dependency from local filesystem to upstream GitHub fork, (3) eliminating dead code identified by staticcheck, and (4) ensuring code structure follows idiomatic Go 1.25 patterns with appropriate file sizes.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), m-mizutani/masq (new)
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: testify + gomock, go test
**Target Platform**: Linux server (primary), Darwin (development)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: Per constitution - API Response < 200ms, Relay Startup < 3s
**Constraints**: Memory < 1GB during ingestion, no unbounded collections
**Scale/Scope**: 100k+ channels, millions of EPG entries

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | N/A | No new data structures |
| II. Modular Pipeline Architecture | N/A | No pipeline changes |
| III. Test-First Development | PASS | Will add tests for masq integration |
| IV. Clean Architecture with SOLID | PASS | Removing dead code improves SRP |
| V. Idiomatic Go | PASS | Core goal of this feature |
| VI. Observable and Debuggable | PASS | Masq enhances log safety |
| VII. Security by Default | PASS | Redaction prevents credential leaks |
| VIII. No Magic Strings or Literals | PASS | Will verify no new literals |
| IX. Resilient HTTP Clients | N/A | No HTTP client changes |
| X. Human-Readable Duration Configuration | N/A | No duration config changes |
| XI. Human-Readable Byte Size Configuration | N/A | No byte size changes |
| XII. Production-Grade CI/CD | PASS | Will ensure staticcheck passes |
| XIII. Test Data Standards | N/A | No test data changes |

**Gate Status**: PASS - No violations requiring justification.

## Project Structure

### Documentation (this feature)

```text
specs/015-codebase-cleanup/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (minimal - no new entities)
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (empty - no new APIs)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/
├── tvarr/cmd/           # Main application CLI
└── e2e-runner/          # E2E test runner (cleanup target)

internal/
├── observability/       # Logger setup - masq integration point
├── relay/               # Multiple unused functions identified
├── service/             # Unused functions identified
├── http/handlers/       # Unused functions identified
└── [other packages]     # Review for dead code

pkg/
└── [packages]           # No changes expected

go.mod                   # Replace directive removal
```

**Structure Decision**: Existing structure is sound. This feature modifies existing files, no new packages required.

## Complexity Tracking

> No constitution violations to justify.

## Phase 0: Research Summary

### R1: masq Integration Pattern

**Decision**: Use m-mizutani/masq with `WithFieldName()` for sensitive field redaction
**Rationale**:
- Native slog integration via `ReplaceAttr`
- Simple field-name based matching for known sensitive fields
- Supports multiple field patterns in single configuration
**Alternatives Considered**:
- showa-93/go-mask: Requires struct tags, more invasive
- Custom handler: More maintenance burden
- alesr/redact: Less mature, fewer features

### R2: Sensitive Fields to Redact

**Decision**: Redact the following field names (case-insensitive where possible):
- `password`, `Password`
- `secret`, `Secret`
- `token`, `Token`
- `apikey`, `ApiKey`, `api_key`
- `credential`, `Credential`

**Rationale**: These cover all sensitive fields in StreamSource, EpgSource, and XtreamClient
**Implementation**: Configure masq with `WithFieldName()` for each pattern

### R3: mediacommon Dependency Migration

**Decision**: Replace local filesystem path with GitHub fork reference
**Current**: `replace github.com/bluenviron/mediacommon/v2 => ../mediacommon`
**Target**: `replace github.com/bluenviron/mediacommon/v2 => github.com/jmylchreest/mediacommon/v2 feat/eac3-support`

**Rationale**: Enables CI/CD builds, removes local path dependency
**Risk**: Branch must be pushed and accessible on GitHub

### R4: Dead Code Identified by staticcheck

**Decision**: Remove all unused code unless justified
**Identified Issues** (44 total from staticcheck output):

| File | Issue | Action |
|------|-------|--------|
| `cmd/e2e-runner/main.go:945` | `waitForSSECompletion` unused | Remove |
| `cmd/e2e-runner/main.go:1840` | `runTest` unused | Remove |
| `internal/http/handlers/relay_stream.go:674-687` | `contains`, `containsLower`, `matchLower` unused | Remove |
| `internal/relay/fmp4_adapter.go:252` | `extractNALUnitsLengthPrefixed` unused | Remove |
| `internal/relay/session.go:320` | `runPassthroughBySourceFormat` unused | Remove |
| `internal/relay/session.go:1150` | `cleanupIdleTranscoders` unused | Remove |
| `internal/relay/session.go:2344-2350` | `demuxerWriter` type unused | Remove |
| `internal/relay/ts_muxer.go:140` | `streamTypeFromCodec` unused | Remove |
| `internal/service/client_detection_service.go:561-577` | `ruleToResult`, `defaultResult` unused | Remove |
| `internal/service/epg_service_test.go:259-281` | `mockEpgHandler` unused | Remove |
| `internal/service/job_service_test.go:440` | `mockScheduler` unused | Remove |
| `internal/service/proxy_service_test.go:212-218` | `mockPipelineFactory` unused | Remove |

**Deprecated API Usage** (to fix):
- `mpeg4audio.Config` → `AudioSpecificConfig` (multiple files in relay/)
- `strings.Title` → `cases.Title` from golang.org/x/text/cases (theme_service.go)

**Style Issues**:
- Error strings should not be capitalized (e2e-runner)
- Nil check before len() is redundant (e2e-runner)
- Struct conversion could use type conversion (relay_stream.go)

### R5: File Size Analysis

**Decision**: Files exceeding 1000 lines need review
**Files Over Threshold**:

| File | Lines | Action |
|------|-------|--------|
| `cmd/e2e-runner/main.go` | 3360 | Accept - standalone test tool |
| `internal/relay/session.go` | 2360 | Review for split opportunities |
| `internal/http/handlers/relay_stream.go` | 1555 | Review for split opportunities |
| `internal/relay/shared_buffer.go` | 1472 | Accept - cohesive buffer implementation |
| `internal/relay/cmaf_muxer.go` | 1117 | Accept - cohesive muxer |
| `internal/ffmpeg/wrapper.go` | 1111 | Accept - cohesive wrapper |
| `internal/relay/ffmpeg_transcoder.go` | 1045 | Accept - cohesive transcoder |
| `internal/http/handlers/types.go` | 1007 | Review - may contain unrelated types |

**Rationale**: Most large files are cohesive units. Focus cleanup on dead code removal first; file splits only if thematically warranted.

## Phase 1: Design

### Data Model Changes

No new entities. Existing entities unchanged.

### API Changes

No new API endpoints. Existing endpoints unchanged.

### Logger Integration Design

```go
// internal/observability/logger.go modifications

import "github.com/m-mizutani/masq"

func NewLogger(cfg LogConfig) *slog.Logger {
    redactor := masq.New(
        masq.WithFieldName("password"),
        masq.WithFieldName("Password"),
        masq.WithFieldName("secret"),
        masq.WithFieldName("Secret"),
        masq.WithFieldName("token"),
        masq.WithFieldName("Token"),
        masq.WithFieldName("apikey"),
        masq.WithFieldName("ApiKey"),
        masq.WithFieldName("api_key"),
        masq.WithFieldName("credential"),
        masq.WithFieldName("Credential"),
    )

    opts := &slog.HandlerOptions{
        Level:       cfg.Level,
        AddSource:   cfg.AddSource,
        ReplaceAttr: redactor,
    }

    // ... rest of handler setup
}
```

### go.mod Changes

```go
// Before
replace github.com/bluenviron/mediacommon/v2 => ../mediacommon

// After
replace github.com/bluenviron/mediacommon/v2 => github.com/jmylchreest/mediacommon/v2 v2.5.3-0.20241214000000-feat-eac3-support
```

Note: The pseudo-version will be determined by the actual commit SHA on the feat/eac3-support branch.

## Implementation Order

1. **P1 - Sensitive Data Redaction** (highest priority - security)
   - Add masq dependency
   - Integrate with logger
   - Add tests for redaction
   - Verify no passwords in logs

2. **P2 - Dependency Migration** (enables CI/CD)
   - Push mediacommon fork if not already
   - Update go.mod replace directive
   - Run go mod tidy
   - Verify build succeeds

3. **P3 - Dead Code Elimination** (code quality)
   - Remove unused functions (sorted by file)
   - Fix deprecated API usage
   - Fix staticcheck style issues
   - Run staticcheck to verify zero issues

4. **P4 - Code Structure Review** (maintenance)
   - Review large files for split opportunities
   - Ensure package names are idiomatic
   - Verify no duplicate logic patterns

## Success Verification

```bash
# SC-001: Zero passwords in logs
task run:dev  # Trigger ingestion, grep logs for passwords

# SC-002: Build from fresh clone
rm -rf vendor go.sum
go mod download
go build ./...

# SC-003: staticcheck passes
staticcheck ./...  # Should return zero issues

# SC-004: Dead code eliminated
# Run staticcheck - no U1000 warnings

# SC-005: File sizes
find . -name "*.go" -not -name "*_test.go" -exec wc -l {} + | awk '$1 > 1000'

# SC-007: No local replace directives
grep -E "replace.*=>.*(\.\.|\./)" go.mod  # Should return nothing
```
