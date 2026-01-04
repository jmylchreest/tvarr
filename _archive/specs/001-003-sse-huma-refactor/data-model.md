# Data Model: SSE Huma Refactor

**Date**: 2025-12-02
**Feature**: 001-003-sse-huma-refactor

## Overview

This feature refactors SSE endpoints to use Huma's native `sse.Register` for OpenAPI documentation. The data model is primarily about **SSE event types and their schemas** rather than database entities.

## Entity Definitions

### No New Database Entities

This refactor does not introduce any new database entities. All entities involved are existing response types that are already serialized to JSON for SSE events.

## SSE Event Type Schemas

### Progress Event Types

Per Huma's requirement that each event type must be a unique Go type, we need wrapper types:

```go
// ProgressResponse is the base schema for all progress events.
// Already exists in internal/http/handlers/progress.go
type ProgressResponse struct {
    ID                string            `json:"id"`
    OperationName     string            `json:"operation_name"`
    OperationType     string            `json:"operation_type"`
    OwnerID           string            `json:"owner_id"`
    OwnerType         string            `json:"owner_type"`
    State             string            `json:"state"`
    OverallPercentage float64           `json:"overall_percentage"`
    Error             string            `json:"error,omitempty"`
    Stages            []StageResponse   `json:"stages,omitempty"`
    CurrentStage      string            `json:"current_stage"`
    StartedAt         time.Time         `json:"started_at"`
    LastUpdate        time.Time         `json:"last_update"`
    CompletedAt       *time.Time        `json:"completed_at,omitempty"`
    Metadata          map[string]any    `json:"metadata,omitempty"`
}

// SSE Event Type Wrappers (NEW - required by Huma)
type ProgressProgressEvent ProgressResponse  // event: progress
type ProgressCompletedEvent ProgressResponse // event: completed
type ProgressErrorEvent ProgressResponse     // event: error
type ProgressCancelledEvent ProgressResponse // event: cancelled
```

### Stage Response Schema

```go
// StageResponse is nested within ProgressResponse.
// Already exists in internal/http/handlers/progress.go
type StageResponse struct {
    ID         string  `json:"id"`
    Name       string  `json:"name"`
    State      string  `json:"state"`
    Percentage float64 `json:"percentage"`
    StageStep  string  `json:"stage_step,omitempty"`
}
```

### Log Event Types

```go
// LogEntryResponse is the schema for log events.
// Already exists in internal/http/handlers/logs.go
type LogEntryResponse struct {
    ID        string                 `json:"id"`
    Timestamp time.Time              `json:"timestamp"`
    Level     string                 `json:"level"`
    Message   string                 `json:"message"`
    Module    string                 `json:"module,omitempty"`
    Target    string                 `json:"target,omitempty"`
    File      string                 `json:"file,omitempty"`
    Line      int                    `json:"line,omitempty"`
    Fields    map[string]interface{} `json:"fields,omitempty"`
    Context   map[string]interface{} `json:"context,omitempty"`
}

// SSE Event Type Wrapper (NEW - required by Huma)
type LogLogEvent LogEntryResponse // event: log
```

### SSE Input Schemas

For Huma's `sse.Register`, input structs define query parameters:

```go
// SSEProgressEventsInput defines query parameters for progress events SSE.
// Will be bound by Huma from URL query string.
type SSEProgressEventsInput struct {
    OperationType string `query:"operation_type" doc:"Filter events by operation type"`
    OwnerID       string `query:"owner_id" doc:"Filter events by owner ID"`
    ResourceID    string `query:"resource_id" doc:"Filter events by resource ID"`
}

// SSELogsStreamInput defines query parameters for logs SSE.
// Will be bound by Huma from URL query string.
type SSELogsStreamInput struct {
    Level   string `query:"level" doc:"Filter by log level (trace, debug, info, warn, error)"`
    Module  string `query:"module" doc:"Filter by module name"`
    Initial int    `query:"initial" default:"50" minimum:"0" maximum:"500" doc:"Number of recent logs to send on connect"`
}
```

## Type Mapping for OpenAPI

### Progress Events Endpoint

```go
eventTypeMap := map[string]any{
    "progress":  ProgressProgressEvent{},
    "completed": ProgressCompletedEvent{},
    "error":     ProgressErrorEvent{},
    "cancelled": ProgressCancelledEvent{},
}
```

### Logs Stream Endpoint

```go
eventTypeMap := map[string]any{
    "log": LogLogEvent{},
}
```

## OpenAPI Schema Output

The event type mapping generates the following OpenAPI components:

```yaml
components:
  schemas:
    ProgressProgressEvent:
      type: object
      properties:
        id: { type: string }
        operation_name: { type: string }
        operation_type: { type: string }
        owner_id: { type: string }
        owner_type: { type: string }
        state: { type: string }
        overall_percentage: { type: number }
        error: { type: string }
        stages:
          type: array
          items:
            $ref: '#/components/schemas/StageResponse'
        current_stage: { type: string }
        started_at: { type: string, format: date-time }
        last_update: { type: string, format: date-time }
        completed_at: { type: string, format: date-time }
        metadata: { type: object, additionalProperties: true }
      required:
        - id
        - operation_name
        - operation_type
        - owner_id
        - owner_type
        - state
        - overall_percentage
        - current_stage
        - started_at
        - last_update

    StageResponse:
      type: object
      properties:
        id: { type: string }
        name: { type: string }
        state: { type: string }
        percentage: { type: number }
        stage_step: { type: string }
      required:
        - id
        - name
        - state
        - percentage

    LogLogEvent:
      type: object
      properties:
        id: { type: string }
        timestamp: { type: string, format: date-time }
        level: { type: string, enum: [trace, debug, info, warn, error] }
        message: { type: string }
        module: { type: string }
        target: { type: string }
        file: { type: string }
        line: { type: integer }
        fields: { type: object, additionalProperties: true }
        context: { type: object, additionalProperties: true }
      required:
        - id
        - timestamp
        - level
        - message
```

## State Enumeration

### Progress States

```go
const (
    StateStarting  = "starting"
    StateRunning   = "running"
    StateCompleted = "completed"
    StateError     = "error"
    StateCancelled = "cancelled"
)
```

### Stage States

```go
const (
    StagePending    = "pending"
    StageInProgress = "in_progress"
    StageCompleted  = "completed"
    StageSkipped    = "skipped"
    StageError      = "error"
)
```

### Log Levels

```go
const (
    LevelTrace = "trace"
    LevelDebug = "debug"
    LevelInfo  = "info"
    LevelWarn  = "warn"
    LevelError = "error"
)
```

## Relationships

```
ProgressResponse
    └── 1:N StageResponse (stages array)

LogEntryResponse (standalone)
```

## Backward Compatibility

All existing types (`ProgressResponse`, `StageResponse`, `LogEntryResponse`) remain unchanged. The new wrapper types (`ProgressProgressEvent`, etc.) are type aliases that serialize to identical JSON, ensuring frontend compatibility.

## File Locations

| Type | File |
|------|------|
| `ProgressResponse` | `internal/http/handlers/progress.go` |
| `StageResponse` | `internal/http/handlers/progress.go` |
| `LogEntryResponse` | `internal/http/handlers/logs.go` |
| SSE Event Wrappers | `internal/http/handlers/progress.go`, `internal/http/handlers/logs.go` (to be added) |
| SSE Input Structs | `internal/http/handlers/progress.go`, `internal/http/handlers/logs.go` (existing, may need adjustment) |
