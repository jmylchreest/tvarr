# Research: SSE Huma Refactor

**Date**: 2025-12-02
**Feature**: 001-003-sse-huma-refactor

## Executive Summary

This research documents the migration of tvarr's SSE endpoints from raw `http.HandlerFunc` handlers to Huma's native `sse.Register` function. The primary goal is to achieve automatic OpenAPI documentation for SSE endpoints while maintaining backward compatibility with existing frontend consumers.

## Current Implementation Analysis

### Progress Handler (`internal/http/handlers/progress.go`)

**Current Pattern:**
```go
func (h *ProgressHandler) RegisterSSE(router interface {
    Get(pattern string, handlerFn http.HandlerFunc)
}) {
    router.Get("/api/v1/progress/events", h.handleSSEEvents)
}
```

**Key Features:**
- Raw HTTP handler directly registered on chi router (bypasses Huma)
- Manual SSE header setting (Content-Type, Cache-Control, Connection, X-Accel-Buffering)
- Manual CORS headers for cross-origin requests
- Custom heartbeat implementation using `fmt.Fprintf(w, ":heartbeat %d\n\n", time.Now().Unix())`
- Uses `http.NewResponseController(w)` for reliable flushing
- Query parameter parsing via `r.URL.Query()`
- Event types: `progress`, `completed`, `error`, `cancelled`

**Handler Logic Flow:**
1. Set SSE/CORS headers
2. Parse filter parameters from query string
3. Subscribe to progress service events
4. Send `:connected\n\n` comment immediately
5. Event loop with heartbeat ticker (30s default)
6. Write SSE events as `event: <type>\ndata: <json>\n\n`

### Logs Handler (`internal/http/handlers/logs.go`)

**Current Pattern:**
```go
func (h *LogsHandler) RegisterSSE(router interface {
    Get(pattern string, handlerFn http.HandlerFunc)
}) {
    router.Get("/api/v1/logs/stream", h.handleSSEStream)
}
```

**Key Features:**
- Similar raw HTTP pattern as progress handler
- Supports initial batch of recent logs via `?initial=N` parameter
- Level and module filtering via query params
- Single event type: `log`
- Same heartbeat implementation as progress handler

## Huma SSE Package Analysis

### API Reference

**Register Function Signature:**
```go
func Register[I any](
    api huma.API,
    op huma.Operation,
    eventTypeMap map[string]any,
    f func(ctx context.Context, input *I, send Sender),
)
```

**Sender Type:**
```go
type Sender func(Message) error

func (s Sender) Data(data any) error {
    return s(Message{Data: data})
}
```

**Message Structure:**
```go
type Message struct {
    ID    int    // Optional SSE message ID
    Data  any    // Payload (auto-marshaled to JSON)
    Retry int    // Client retry interval (ms)
}
```

### Event Type Requirements

Per Huma documentation: "Each event model **must** be a unique Go type." This means our existing `ProgressResponse` struct must be wrapped into distinct types for each event type:

```go
// Good - distinct types for OpenAPI schema generation
type ProgressEvent ProgressResponse
type CompletedEvent ProgressResponse
type ErrorEvent ProgressResponse
type CancelledEvent ProgressResponse
```

### Huma SSE Capabilities

**What Huma Provides:**
- Automatic OpenAPI schema generation for SSE endpoints
- Content-Type: `text/event-stream` auto-set
- Event type mapping to schema types
- Input struct binding for query parameters
- Automatic JSON marshaling via `send.Data()`
- Flushing via `http.Flusher` when available

**What Huma Does NOT Provide:**
- SSE comments (`:comment\n\n` format)
- Heartbeat/keepalive mechanism
- Connection establishment comments
- Direct response writer access (abstracted away)

### Critical Gap: Heartbeats and Comments

Huma's `sse.Sender` only supports sending data events via `Message{}`. There is **no built-in mechanism** for:
- Sending SSE comments (`:heartbeat <unix_epoch>\n\n`)
- Sending connection establishment comments (`:connected\n\n`)

**Options for Heartbeat Implementation:**

1. **Access Underlying ResponseWriter** - The callback receives `context.Context` which may contain the underlying `http.ResponseWriter` via Huma's context. We need to verify if this is accessible.

2. **Wrapper Pattern** - Create a custom writer wrapper that intercepts Huma's SSE writes and adds heartbeat functionality.

3. **Timer-Based Data Events** - Send heartbeat as a named event type (e.g., `event: heartbeat\ndata: {"ts": 1234567890}\n\n`) instead of a comment. This would change the current behavior but would document the heartbeat in OpenAPI.

4. **Hybrid Approach** - Use Huma for OpenAPI documentation but implement the actual handler using raw HTTP with the documented schema types.

**Recommended Approach:** Option 4 (Hybrid) - Register the operation with Huma for OpenAPI documentation but use a custom middleware that intercepts the request before Huma's handler runs, implementing the full SSE logic including comments. This preserves backward compatibility while gaining OpenAPI documentation.

## Frontend Contract Analysis

### Progress Events Frontend Expectations

```typescript
// frontend/src/types/api.ts
interface ProgressEvent {
  id: string;
  operation_name: string;
  operation_type: string;
  owner_id: string;
  owner_type: string;
  state: string;
  overall_percentage: number;
  error?: string;
  stages?: ProgressStage[];
  current_stage: string;
  started_at: string;
  last_update: string;
  completed_at?: string;
  metadata?: Record<string, any>;
}
```

**Event Types Expected:**
- `progress` - Progress update during operation
- `completed` - Operation finished successfully
- `error` - Operation failed
- `cancelled` - Operation was cancelled

**SSE Comments Expected:**
- `:connected` - On connection establishment
- `:heartbeat <unix_epoch>` - Every 30 seconds

### Logs Events Frontend Expectations

```typescript
interface LogEntry {
  id: string;
  timestamp: string;
  level: string;
  message: string;
  module?: string;
  target?: string;
  file?: string;
  line?: number;
  fields?: Record<string, any>;
  context?: Record<string, any>;
}
```

**Event Type Expected:**
- `log` - Single log entry

## Migration Strategy

### Phase 1: Create Event Type Wrappers

Create distinct types for OpenAPI schema generation while maintaining JSON compatibility:

```go
// Progress events - each must be a unique type for Huma
type ProgressProgressEvent ProgressResponse
type ProgressCompletedEvent ProgressResponse
type ProgressErrorEvent ProgressResponse
type ProgressCancelledEvent ProgressResponse

// Logs event
type LogLogEvent LogEntryResponse
```

### Phase 2: Hybrid Registration

Register SSE endpoints using Huma's operation definition (for OpenAPI) while keeping the raw HTTP handler:

```go
func (h *ProgressHandler) Register(api huma.API) {
    // ... existing REST endpoints ...

    // SSE endpoint - register operation for OpenAPI but use raw handler
    h.registerSSEOperation(api)
}

func (h *ProgressHandler) registerSSEOperation(api huma.API) {
    // This registers the OpenAPI documentation only
    // The actual handler is registered separately on chi router
    api.OpenAPI().AddOperation(&huma.Operation{
        OperationID: "progressEvents",
        Method:      "GET",
        Path:        "/api/v1/progress/events",
        Summary:     "Subscribe to progress events",
        Description: "Server-Sent Events stream for real-time progress updates",
        Tags:        []string{"Progress"},
        // ... response schemas for event types ...
    })
}
```

### Phase 3: Update Handler to Use Documented Types

Ensure the raw handler uses the same types documented in OpenAPI.

### Phase 4: Maintain RegisterSSE Method

Keep the `RegisterSSE` method for raw HTTP registration:

```go
func (h *ProgressHandler) RegisterSSE(router interface {
    Get(pattern string, handlerFn http.HandlerFunc)
}) {
    router.Get("/api/v1/progress/events", h.handleSSEEvents)
}
```

## OpenAPI Schema Design

### Progress Events Endpoint

```yaml
/api/v1/progress/events:
  get:
    operationId: progressEvents
    summary: Subscribe to progress events
    description: |
      Server-Sent Events stream for real-time progress updates.

      ## Connection Protocol
      - On connect: receives `:connected` comment
      - Every 30s without events: receives `:heartbeat <unix_epoch>` comment

      ## Event Types
      - `progress`: Operation in progress
      - `completed`: Operation finished successfully
      - `error`: Operation failed
      - `cancelled`: Operation was cancelled
    tags:
      - Progress
    parameters:
      - name: operation_type
        in: query
        schema:
          type: string
      - name: owner_id
        in: query
        schema:
          type: string
      - name: resource_id
        in: query
        schema:
          type: string
    responses:
      '200':
        description: SSE event stream
        content:
          text/event-stream:
            schema:
              oneOf:
                - $ref: '#/components/schemas/ProgressEvent'
                - $ref: '#/components/schemas/CompletedEvent'
                - $ref: '#/components/schemas/ErrorEvent'
                - $ref: '#/components/schemas/CancelledEvent'
```

### Logs Stream Endpoint

```yaml
/api/v1/logs/stream:
  get:
    operationId: logsStream
    summary: Subscribe to log events
    description: |
      Server-Sent Events stream for real-time log entries.

      ## Connection Protocol
      - On connect: receives `:connected` comment
      - On connect with `initial=N`: receives up to N recent log entries
      - Every 30s without events: receives `:heartbeat <unix_epoch>` comment
    tags:
      - Logs
    parameters:
      - name: level
        in: query
        schema:
          type: string
          enum: [trace, debug, info, warn, error]
      - name: module
        in: query
        schema:
          type: string
      - name: initial
        in: query
        schema:
          type: integer
          minimum: 0
          maximum: 500
          default: 50
    responses:
      '200':
        description: SSE event stream
        content:
          text/event-stream:
            schema:
              $ref: '#/components/schemas/LogEntryResponse'
```

## Risk Assessment

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Frontend SSE parsing breaks | High | Low | Maintain exact event format, extensive testing |
| Heartbeat comments not supported by Huma | Medium | Confirmed | Use hybrid approach with raw handler |
| OpenAPI schema doesn't reflect actual events | Medium | Low | Test schema generation, manual verification |
| Connection establishment comment missing | Low | Low | Frontend gracefully handles missing `:connected` |

## Verification Plan

1. **Unit Tests**: Verify event serialization matches current format
2. **Integration Tests**: SSE connection lifecycle with filters
3. **Contract Tests**: Validate OpenAPI schema matches actual events
4. **Frontend Compatibility**: Manual testing with existing frontend
5. **Documentation Check**: Verify `/docs` shows SSE endpoints

## References

- [Huma SSE Documentation](https://huma.rocks/features/server-sent-events-sse/)
- [Huma SSE Package (pkg.go.dev)](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/sse)
- Current implementation: `internal/http/handlers/progress.go`, `internal/http/handlers/logs.go`
