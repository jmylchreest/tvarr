# Feature Specification: SSE Huma Refactor

**Feature Branch**: `001-003-sse-huma-refactor`
**Created**: 2025-12-02
**Status**: Draft
**Input**: Refactor SSE endpoints to use Huma's native `sse.Register` for automatic OpenAPI documentation

## User Scenarios & Testing *(mandatory)*

### User Story 1 - API Documentation Discovery (Priority: P1)

As a developer integrating with the tvarr API, I want to see the SSE endpoints documented in the OpenAPI spec at `/docs` so I can understand how to subscribe to real-time events for progress tracking and log streaming.

**Why this priority**: Without API documentation, developers cannot discover SSE endpoints exist or understand how to use them. This is the primary driver for this refactor.

**Independent Test**: Can be fully tested by visiting `/docs` and verifying SSE endpoints appear with their event schemas documented.

**Acceptance Scenarios**:

1. **Given** the tvarr server is running, **When** I visit `/docs`, **Then** I see the `/api/v1/progress/events` SSE endpoint documented with its query parameters and event schemas
2. **Given** the tvarr server is running, **When** I visit `/docs`, **Then** I see the `/api/v1/logs/stream` SSE endpoint documented with its query parameters and event schemas
3. **Given** I download `/openapi.yaml`, **When** I examine the spec, **Then** the SSE endpoints include proper content-type (`text/event-stream`) and event type definitions

---

### User Story 2 - Progress Events Streaming (Priority: P1)

As a frontend developer, I want to receive real-time progress events via SSE so I can display operation progress (ingestion, proxy generation) to users without polling.

**Why this priority**: This is existing functionality that must continue working after the refactor. Progress tracking is core to the user experience.

**Independent Test**: Can be tested by triggering an ingestion operation and verifying SSE events are received with correct event types (`progress`, `completed`, `error`).

**Acceptance Scenarios**:

1. **Given** I'm connected to `/api/v1/progress/events`, **When** an ingestion starts, **Then** I receive `progress` events with operation details including stages and percentages
2. **Given** I'm connected to `/api/v1/progress/events`, **When** an operation completes, **Then** I receive a `completed` event with final state
3. **Given** I'm connected to `/api/v1/progress/events`, **When** an operation fails, **Then** I receive an `error` event with error details
4. **Given** I'm connected to `/api/v1/progress/events` with `?owner_id=X`, **When** operations occur, **Then** I only receive events for that owner

---

### User Story 3 - Log Events Streaming (Priority: P1)

As a frontend developer, I want to receive real-time log events via SSE so I can display application logs to users in the admin UI.

**Why this priority**: This is existing functionality that must continue working after the refactor. Log streaming enables real-time debugging.

**Independent Test**: Can be tested by connecting to the logs stream and verifying log events are received as they're generated.

**Acceptance Scenarios**:

1. **Given** I'm connected to `/api/v1/logs/stream`, **When** the server logs a message, **Then** I receive a `log` event with the log entry
2. **Given** I'm connected to `/api/v1/logs/stream?initial=50`, **When** I connect, **Then** I first receive up to 50 recent log entries before live streaming
3. **Given** I'm connected to `/api/v1/logs/stream?level=error`, **When** logs are generated, **Then** I only receive error-level logs
4. **Given** I'm connected to `/api/v1/logs/stream?module=ingestor`, **When** logs are generated, **Then** I only receive logs from the ingestor module

---

### User Story 4 - Connection Health (Priority: P2)

As a frontend developer, I want the SSE connection to send heartbeats so I can detect disconnections and implement reconnection logic.

**Why this priority**: Heartbeats ensure clients can detect stale connections, but this is secondary to core event delivery.

**Independent Test**: Can be tested by connecting and waiting for heartbeat comments to be received.

**Acceptance Scenarios**:

1. **Given** I'm connected to an SSE endpoint, **When** 30 seconds pass without events, **Then** I receive a heartbeat comment
2. **Given** I'm connected to an SSE endpoint, **When** the connection is first established, **Then** I receive a `:connected` comment immediately

---

### Edge Cases

- What happens when a client connects with invalid filter parameters? (Should return 400 Bad Request)
- How does the system handle client disconnection mid-stream? (Should clean up subscriber resources)
- What happens when the event buffer is full and client is slow? (Should skip events for slow clients to prevent backpressure)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST register SSE endpoints using Huma's `sse.Register` function
- **FR-002**: System MUST document all SSE event types in the OpenAPI specification
- **FR-003**: System MUST support the same query parameters currently available (`operation_type`, `owner_id`, `resource_id` for progress; `level`, `module`, `initial` for logs)
- **FR-004**: System MUST send a `:connected` comment immediately on connection
- **FR-005**: System MUST send heartbeat comments every 30 seconds
- **FR-006**: System MUST properly close subscriber channels on client disconnect
- **FR-007**: System MUST maintain backward compatibility with existing frontend SSE consumers

### SSE Event Types

#### Progress Events (`/api/v1/progress/events`)

| Event Type | Description | Schema |
|------------|-------------|--------|
| `progress` | Operation progress update | `ProgressResponse` |
| `completed` | Operation completed successfully | `ProgressResponse` |
| `error` | Operation failed | `ProgressResponse` with error field |
| `cancelled` | Operation was cancelled | `ProgressResponse` |

#### Log Events (`/api/v1/logs/stream`)

| Event Type | Description | Schema |
|------------|-------------|--------|
| `log` | A log entry | `LogEntryResponse` |

### Key Entities

- **ProgressResponse**: Existing response type for progress operations (id, operation_name, operation_type, owner_id, state, stages, etc.)
- **LogEntryResponse**: Existing response type for log entries (id, timestamp, level, message, module, fields, context)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Both SSE endpoints appear in the OpenAPI spec at `/openapi.yaml`
- **SC-002**: The `/docs` Swagger UI shows SSE endpoints with event schemas
- **SC-003**: Existing frontend SSE consumers work without modification
- **SC-004**: All existing SSE tests continue to pass
- **SC-005**: Event schemas in OpenAPI match the actual JSON structure sent by the server

## Technical Notes

### Current Implementation

The current SSE implementation uses raw `http.HandlerFunc` handlers registered directly on the chi router:

```go
// progress.go
func (h *ProgressHandler) RegisterSSE(router interface {
    Get(pattern string, handlerFn http.HandlerFunc)
}) {
    router.Get("/api/v1/progress/events", h.handleSSEEvents)
}

// logs.go
func (h *LogsHandler) RegisterSSE(router interface {
    Get(pattern string, handlerFn http.HandlerFunc)
}) {
    router.Get("/api/v1/logs/stream", h.handleSSEStream)
}
```

This bypasses Huma's registration, so endpoints don't appear in OpenAPI.

### Target Implementation

Use Huma's native SSE support via `sse.Register`:

```go
import "github.com/danielgtaylor/huma/v2/sse"

// Register with event type to schema mapping
sse.Register(api, huma.Operation{
    OperationID: "progressEvents",
    Method:      "GET",
    Path:        "/api/v1/progress/events",
    Summary:     "Subscribe to progress events",
    Tags:        []string{"Progress"},
}, map[string]any{
    "progress":  ProgressResponse{},
    "completed": ProgressResponse{},
    "error":     ProgressResponse{},
    "cancelled": ProgressResponse{},
}, func(ctx context.Context, input *SSEEventsInput, send sse.Sender) {
    // Subscribe to events and send via send.Data()
})
```

### Migration Considerations

1. The `sse.Sender` interface replaces direct `http.ResponseWriter` usage
2. Heartbeat implementation may need adjustment for Huma's SSE model
3. Connection establishment comment (`:connected`) behavior should be verified
4. Query parameter parsing will use Huma's input struct binding instead of manual `r.URL.Query()`
