# Tasks: SSE Huma Refactor

**Branch**: `001-003-sse-huma-refactor` | **Date**: 2025-12-02 | **Plan**: [plan.md](plan.md)

## Task Overview

This task list implements the hybrid Huma SSE registration strategy to achieve OpenAPI documentation for SSE endpoints while maintaining backward compatibility with existing frontend consumers and custom heartbeat implementation.

## Phase 1: Setup & Dependencies

- [x] **T-001**: Verify Huma SSE package import is available
  - File: `internal/http/handlers/progress.go`
  - Check: Import `github.com/danielgtaylor/huma/v2/sse`
  - Dependencies: None

## Phase 2: Event Type Wrappers

- [x] **T-002**: Add SSE event type wrappers to progress.go
  - File: `internal/http/handlers/progress.go`
  - Add after line 63 (StageResponse struct):
    ```go
    // SSE Event Type Wrappers - required by Huma for OpenAPI schema generation.
    // Each event type must be a unique Go type for Huma to generate distinct schemas.
    type ProgressProgressEvent ProgressResponse  // event: progress
    type ProgressCompletedEvent ProgressResponse // event: completed
    type ProgressErrorEvent ProgressResponse     // event: error
    type ProgressCancelledEvent ProgressResponse // event: cancelled
    ```
  - Dependencies: None

- [x] **T-003**: Add SSE event type wrapper to logs.go
  - File: `internal/http/handlers/logs.go`
  - Add after line 61 (LogStatsResponse struct):
    ```go
    // SSE Event Type Wrapper - required by Huma for OpenAPI schema generation.
    type LogLogEvent LogEntryResponse // event: log
    ```
  - Dependencies: None

## Phase 3: Huma SSE Registration

- [x] **T-004**: Add Huma SSE registration to progress.go
  - File: `internal/http/handlers/progress.go`
  - Modify Register() method to include sse.Register call
  - Add import for `github.com/danielgtaylor/huma/v2/sse`
  - Dependencies: T-002

- [x] **T-005**: Add Huma SSE registration to logs.go
  - File: `internal/http/handlers/logs.go`
  - Modify Register() method to include sse.Register call
  - Add import for `github.com/danielgtaylor/huma/v2/sse`
  - Dependencies: T-003

## Phase 4: Input Type Adjustments

- [x] **T-006**: Add SSE input type for logs handler
  - File: `internal/http/handlers/logs.go`
  - Add SSELogsStreamInput struct for Huma parameter binding
  - Dependencies: T-003

## Phase 5: Verification & Testing

- [x] **T-007**: Build and verify no compilation errors
  - Command: `task build`
  - Dependencies: T-004, T-005, T-006

- [x] **T-008**: Verify SSE endpoints appear in OpenAPI spec
  - Command: Start server and check `/openapi.yaml`
  - Dependencies: T-007

- [x] **T-009**: Verify existing SSE behavior is unchanged
  - Test: Connect to SSE endpoints and verify events
  - Dependencies: T-008

## Success Criteria Verification

- [x] **SC-001**: Both SSE endpoints appear in the OpenAPI spec at `/openapi.yaml`
- [x] **SC-002**: The `/docs` Swagger UI shows SSE endpoints with event schemas
- [x] **SC-003**: Existing frontend SSE consumers work without modification
- [x] **SC-004**: All existing SSE tests continue to pass
- [x] **SC-005**: Event schemas in OpenAPI match the actual JSON structure sent by the server

## Execution Order

```
T-001 (setup)
   ↓
T-002, T-003 [P] (event type wrappers - parallel)
   ↓
T-004, T-005, T-006 [P] (Huma registration - parallel after wrappers)
   ↓
T-007 (build verification)
   ↓
T-008 (OpenAPI verification)
   ↓
T-009 (behavior verification)
```

## Rollback Plan

If issues arise:
1. Remove `sse.Register` calls from Register() methods
2. Remove event type wrapper definitions
3. Existing SSE behavior is fully preserved in raw handlers
