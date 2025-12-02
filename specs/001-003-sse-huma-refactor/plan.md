# Implementation Plan: SSE Huma Refactor

**Branch**: `001-003-sse-huma-refactor` | **Date**: 2025-12-02 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-003-sse-huma-refactor/spec.md`

## Summary

Refactor SSE endpoints (`/api/v1/progress/events`, `/api/v1/logs/stream`) to use Huma's native `sse.Register` function for automatic OpenAPI documentation, while maintaining backward compatibility with existing frontend SSE consumers and custom heartbeat implementation.

## Technical Context

**Language/Version**: Go 1.25.x
**Primary Dependencies**: Huma v2.34+, Chi router
**Storage**: N/A (no database changes)
**Testing**: go test, testify
**Target Platform**: Linux server
**Project Type**: Web application (backend Go, frontend Next.js)
**Performance Goals**: SSE events delivered within 100ms of generation
**Constraints**: Must maintain exact event format for frontend compatibility
**Scale/Scope**: 2 SSE endpoints, 5 event types

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| Memory-First Design | PASS | SSE uses streaming, no buffering changes |
| Modular Pipeline Architecture | PASS | Handler-level changes only |
| Test-First Development | PASS | Tests will be written before implementation |
| Clean Architecture (SOLID) | PASS | Maintaining existing patterns |
| Idiomatic Go | PASS | Using standard library patterns |
| Observable and Debuggable | PASS | Logging preserved |
| Security by Default | PASS | No security changes |
| No Magic Strings | PASS | Event types are constants |
| Resilient HTTP Clients | N/A | No HTTP client changes |
| Human-Readable Duration | PASS | Heartbeat interval unchanged |
| Human-Readable Byte Size | N/A | No byte size config |
| Production-Grade CI/CD | PASS | Existing CI applies |

## Project Structure

### Documentation (this feature)

```text
specs/001-003-sse-huma-refactor/
├── plan.md              # This file
├── research.md          # Phase 0 output - Huma SSE API analysis
├── data-model.md        # Phase 1 output - SSE event schemas
├── quickstart.md        # Phase 1 output - Verification guide
├── contracts/           # Phase 1 output
│   └── sse-endpoints.yaml  # OpenAPI contract for SSE endpoints
└── tasks.md             # Phase 2 output (to be generated)
```

### Source Code (repository root)

```text
internal/http/handlers/
├── progress.go          # MODIFY - Add SSE event type wrappers, update registration
├── progress_test.go     # MODIFY - Add tests for new SSE registration
├── logs.go              # MODIFY - Add SSE event type wrappers, update registration
└── logs_test.go         # MODIFY - Add tests for new SSE registration
```

**Structure Decision**: Modifications only to existing handler files. No new files needed.

## Complexity Tracking

> No constitution violations. This is a focused refactor of 2 files.

## Implementation Approach

### Key Finding from Research

Huma's `sse.Register` does **not** support SSE comments (`:connected`, `:heartbeat`). The `Sender` interface only sends data events via `Message{}`. However, we need custom heartbeats for frontend compatibility.

### Chosen Strategy: Hybrid Registration

1. **Use `sse.Register`** for OpenAPI documentation - registers the operation, event types, and schemas
2. **Keep raw HTTP handler** on chi router - provides full control over comments and heartbeats
3. **Ensure handler precedence** - chi router registration takes priority over Huma's auto-generated handler

This approach:
- Achieves OpenAPI documentation goal
- Maintains exact SSE behavior including comments
- Requires no frontend changes
- Uses Huma's schema generation for event types

### Implementation Phases

#### Phase 1: Create Event Type Wrappers

Add distinct types required by Huma for OpenAPI schema generation:

```go
// progress.go - after ProgressResponse
type ProgressProgressEvent ProgressResponse
type ProgressCompletedEvent ProgressResponse
type ProgressErrorEvent ProgressResponse
type ProgressCancelledEvent ProgressResponse
```

```go
// logs.go - after LogEntryResponse
type LogLogEvent LogEntryResponse
```

#### Phase 2: Add Huma SSE Registration

Register SSE operations with Huma for OpenAPI documentation:

```go
// progress.go - in Register() method
sse.Register(api, huma.Operation{
    OperationID: "progressEvents",
    Method:      "GET",
    Path:        "/api/v1/progress/events",
    Summary:     "Subscribe to progress events",
    Description: "Server-Sent Events stream for real-time progress updates...",
    Tags:        []string{"Progress"},
}, map[string]any{
    "progress":  ProgressProgressEvent{},
    "completed": ProgressCompletedEvent{},
    "error":     ProgressErrorEvent{},
    "cancelled": ProgressCancelledEvent{},
}, func(ctx context.Context, input *SSEEventsInput, send sse.Sender) {
    // This handler will be overridden by chi router registration
    // But Huma needs it for schema generation
    <-ctx.Done()
})
```

#### Phase 3: Maintain Chi Router Override

Ensure the raw handler is registered AFTER Huma to take precedence:

```go
// serve.go - registration order matters
progressHandler.Register(server.API())  // Registers Huma SSE (for OpenAPI)
progressHandler.RegisterSSE(server.Router())  // Registers chi handler (takes precedence)
```

#### Phase 4: Update Tests

Add tests to verify:
1. SSE endpoints appear in OpenAPI spec
2. Event schemas are correctly documented
3. Existing SSE behavior unchanged

## File Changes Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/http/handlers/progress.go` | MODIFY | Add event type wrappers, add `sse.Register` call |
| `internal/http/handlers/progress_test.go` | MODIFY | Add OpenAPI verification tests |
| `internal/http/handlers/logs.go` | MODIFY | Add event type wrapper, add `sse.Register` call |
| `internal/http/handlers/logs_test.go` | MODIFY | Add OpenAPI verification tests |

## Success Criteria

From spec.md:

- [ ] **SC-001**: Both SSE endpoints appear in the OpenAPI spec at `/openapi.yaml`
- [ ] **SC-002**: The `/docs` Swagger UI shows SSE endpoints with event schemas
- [ ] **SC-003**: Existing frontend SSE consumers work without modification
- [ ] **SC-004**: All existing SSE tests continue to pass
- [ ] **SC-005**: Event schemas in OpenAPI match the actual JSON structure sent by the server

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Huma handler overrides chi | Register chi handler AFTER Huma registration |
| Event type mismatch | Use type aliases that serialize identically to base types |
| Test flakiness | Use deterministic test data with fictional names |
| Frontend breakage | Full compatibility testing before merge |

## Dependencies

- `github.com/danielgtaylor/huma/v2/sse` package (already available, v2.34+)

## Testing Strategy

1. **Unit Tests**: Verify event type wrappers serialize correctly
2. **Integration Tests**:
   - OpenAPI spec contains SSE endpoints
   - Swagger UI renders SSE documentation
   - SSE connections work with filters
3. **Contract Tests**: OpenAPI schema matches actual event JSON
4. **Regression Tests**: All existing SSE tests pass unchanged

## Rollback Plan

If issues arise:
1. Remove `sse.Register` calls
2. Remove event type wrappers (unused)
3. Existing behavior fully preserved in raw handlers
