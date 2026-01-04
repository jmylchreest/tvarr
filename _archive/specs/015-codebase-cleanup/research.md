# Research: Codebase Cleanup and Refactoring

**Feature**: 015-codebase-cleanup
**Date**: 2025-12-14

## R1: slog Redaction Middleware Selection

### Decision
Use **m-mizutani/masq** for sensitive data redaction in slog logs.

### Rationale
- Native slog integration via `ReplaceAttr` handler option
- Field-name based matching without struct tag modifications
- Well-maintained with active development
- Simple API with multiple configuration options

### Alternatives Considered

| Library | Pros | Cons | Decision |
|---------|------|------|----------|
| m-mizutani/masq | Native slog, field-name matching | None significant | **Selected** |
| showa-93/go-mask | Customizable | Requires struct tags, invasive | Rejected |
| alesr/redact | Pipeline approach | Less mature | Rejected |
| Custom handler | Full control | Maintenance burden | Rejected |

### Implementation Pattern

```go
import "github.com/m-mizutani/masq"

redactor := masq.New(
    masq.WithFieldName("password"),
    masq.WithFieldName("secret"),
    // ... more fields
)

handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    ReplaceAttr: redactor,
})
```

## R2: Sensitive Fields Inventory

### Decision
Redact the following field names across all log output:

| Field Pattern | Reason | Found In |
|---------------|--------|----------|
| `password`, `Password` | Xtream API credentials | StreamSource, EpgSource |
| `secret`, `Secret` | Generic secret values | Future-proofing |
| `token`, `Token` | API tokens | Generic security |
| `apikey`, `ApiKey`, `api_key` | API keys | Generic security |
| `credential`, `Credential` | Generic credentials | Generic security |

### Rationale
- Case variations needed because slog field names are user-defined
- Covers all current sensitive fields in models
- Future-proofs against common secret naming patterns

### Source Analysis
Files containing password references (19 files identified):
- `internal/models/stream_source.go` - Password field
- `internal/models/epg_source.go` - Password field
- `pkg/xtream/client.go` - Password in API calls
- `internal/ingestor/xtream_handler.go` - Password in logs
- `internal/ingestor/xtream_epg_handler.go` - Password in logs
- Various test files

## R3: mediacommon Dependency Status

### Decision
Migrate from local path to GitHub fork reference.

### Current State
```go
// go.mod
replace github.com/bluenviron/mediacommon/v2 => ../mediacommon
```

### Target State
```go
// go.mod
replace github.com/bluenviron/mediacommon/v2 => github.com/jmylchreest/mediacommon/v2 v2.5.3-eac3
```

### Prerequisites
1. Verify `feat/eac3-support` branch exists on `github.com/jmylchreest/mediacommon`
2. Determine correct pseudo-version or tag
3. Update go.mod with remote reference
4. Run `go mod tidy` to verify resolution

### Risk Mitigation
- If branch not pushed: Push fork to GitHub first
- If pseudo-version fails: Create a release tag on the fork

## R4: Dead Code Analysis

### Decision
Remove all unused code identified by staticcheck U1000 warnings.

### Methodology
```bash
staticcheck ./... 2>&1 | grep U1000
```

### Findings Summary

#### Unused Functions (must remove)
| File | Function/Type | Lines |
|------|---------------|-------|
| `cmd/e2e-runner/main.go` | `waitForSSECompletion` | ~50 |
| `cmd/e2e-runner/main.go` | `runTest` | ~100 |
| `internal/http/handlers/relay_stream.go` | `contains` | ~5 |
| `internal/http/handlers/relay_stream.go` | `containsLower` | ~5 |
| `internal/http/handlers/relay_stream.go` | `matchLower` | ~10 |
| `internal/relay/fmp4_adapter.go` | `extractNALUnitsLengthPrefixed` | ~30 |
| `internal/relay/session.go` | `runPassthroughBySourceFormat` | ~100 |
| `internal/relay/session.go` | `cleanupIdleTranscoders` | ~50 |
| `internal/relay/session.go` | `demuxerWriter` type | ~10 |
| `internal/relay/ts_muxer.go` | `streamTypeFromCodec` | ~30 |
| `internal/service/client_detection_service.go` | `ruleToResult` | ~15 |
| `internal/service/client_detection_service.go` | `defaultResult` | ~10 |

#### Unused Test Mocks (must remove)
| File | Type | Lines |
|------|------|-------|
| `internal/service/epg_service_test.go` | `mockEpgHandler` | ~25 |
| `internal/service/job_service_test.go` | `mockScheduler` | ~10 |
| `internal/service/proxy_service_test.go` | `mockPipelineFactory` | ~10 |

#### Deprecated API Usage (must fix)
| File | Deprecated | Replacement |
|------|------------|-------------|
| `internal/relay/cmaf_muxer.go` | `mpeg4audio.Config` | `AudioSpecificConfig` |
| `internal/relay/fmp4_adapter.go` | `mpeg4audio.Config` | `AudioSpecificConfig` |
| `internal/relay/ts_muxer.go` | `mpeg4audio.Config` | `AudioSpecificConfig` |
| `internal/service/theme_service.go` | `strings.Title` | `cases.Title` |

#### Style Issues (should fix)
| File | Issue | Fix |
|------|-------|-----|
| `cmd/e2e-runner/main.go:2030` | Capitalized error string | Lowercase |
| `cmd/e2e-runner/main.go:2033` | Capitalized error string | Lowercase |
| `cmd/e2e-runner/main.go:1619` | Redundant nil check | Remove |
| `cmd/e2e-runner/testdata.go:119` | Unused variable | Remove or use |
| `internal/http/handlers/relay_stream.go:344,479` | Verbose struct conversion | Use type conversion |

### Estimated Line Reduction
- Unused functions: ~450 lines
- Unused test mocks: ~45 lines
- **Total**: ~495 lines removed

## R5: File Size Analysis

### Decision
Accept most large files as cohesive units; review two files for potential splits.

### Analysis

| File | Lines | Verdict | Reason |
|------|-------|---------|--------|
| `cmd/e2e-runner/main.go` | 3360 | Accept | Standalone test tool, not part of main app |
| `internal/relay/session.go` | 2360 | Review | May benefit from extracting session types |
| `internal/http/handlers/relay_stream.go` | 1555 | Review | Multiple handler functions may warrant split |
| `internal/relay/shared_buffer.go` | 1472 | Accept | Cohesive buffer implementation |
| `internal/relay/cmaf_muxer.go` | 1117 | Accept | Cohesive CMAF muxer |
| `internal/ffmpeg/wrapper.go` | 1111 | Accept | Cohesive FFmpeg wrapper |
| `internal/relay/ffmpeg_transcoder.go` | 1045 | Accept | Cohesive transcoder |
| `internal/http/handlers/types.go` | 1007 | Review | May contain unrelated request/response types |

### Recommendation
Focus on dead code removal first. File splits are lower priority and should only be done if clear thematic boundaries exist. Splitting for the sake of line count is counter-productive.

## R6: Package Naming Review

### Decision
Current package names are idiomatic Go. No changes required.

### Analysis
All packages follow Go conventions:
- Lowercase names ✓
- No underscores ✓
- Concise and descriptive ✓
- No stutter (e.g., `service.ServiceFoo`) ✓

## References

- [m-mizutani/masq GitHub](https://github.com/m-mizutani/masq)
- [staticcheck documentation](https://staticcheck.io/docs/)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
