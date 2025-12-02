# Quickstart: SSE Huma Refactor

**Date**: 2025-12-02
**Feature**: 001-003-sse-huma-refactor

## Overview

This guide shows how to verify the SSE endpoints are properly documented in OpenAPI after the refactor.

## Verification Steps

### 1. Check OpenAPI Documentation

Start the server and verify SSE endpoints appear in the API docs:

```bash
# Start the server
task run:dev

# Check if SSE endpoints are in OpenAPI spec
curl -s http://localhost:8080/openapi.yaml | grep -A5 "progressEvents"
curl -s http://localhost:8080/openapi.yaml | grep -A5 "logsStream"
```

### 2. Visit Swagger UI

Open http://localhost:8080/docs in a browser and verify:

- [ ] `/api/v1/progress/events` appears under "Progress" tag
- [ ] `/api/v1/logs/stream` appears under "Logs" tag
- [ ] Query parameters are documented
- [ ] Event schemas are visible

### 3. Test SSE Connection

```bash
# Test progress events SSE (will receive :connected and heartbeats)
curl -N -H "Accept: text/event-stream" "http://localhost:8080/api/v1/progress/events"

# Test logs SSE with initial batch
curl -N -H "Accept: text/event-stream" "http://localhost:8080/api/v1/logs/stream?initial=10"
```

### 4. Verify Event Format

Expected SSE output format:

```
:connected

event: progress
data: {"id":"01ABC...","operation_name":"Ingesting...","state":"running",...}

:heartbeat 1764708705

event: completed
data: {"id":"01ABC...","operation_name":"Ingesting...","state":"completed",...}
```

### 5. Frontend Compatibility

The existing frontend SSE consumers should work without modification:

```typescript
// This should still work after the refactor
const eventSource = new EventSource('/api/v1/progress/events');
eventSource.addEventListener('progress', (e) => {
  const data = JSON.parse(e.data);
  console.log('Progress:', data.overall_percentage);
});
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/http/handlers/progress.go` | Progress SSE handler |
| `internal/http/handlers/logs.go` | Logs SSE handler |
| `/openapi.yaml` | Generated OpenAPI spec |
| `/docs` | Swagger UI |

## Success Criteria Checklist

- [ ] SSE endpoints appear in `/openapi.yaml`
- [ ] SSE endpoints visible in Swagger UI at `/docs`
- [ ] Event schemas documented in OpenAPI
- [ ] `:connected` comment sent on connection
- [ ] `:heartbeat <unix_epoch>` comment sent every 30 seconds
- [ ] All existing event types work (`progress`, `completed`, `error`, `cancelled`, `log`)
- [ ] Query parameter filtering works as before
- [ ] Frontend SSE consumers work without modification

## Troubleshooting

### SSE Endpoints Not in OpenAPI

If SSE endpoints don't appear in the OpenAPI spec:

1. Check that `sse.Register` is being called correctly
2. Verify event type map includes all event types
3. Check for registration errors in server logs

### Events Not Received

If events aren't received by clients:

1. Check that event type names match (case-sensitive)
2. Verify `Content-Type: text/event-stream` header is set
3. Check for proxy buffering issues (X-Accel-Buffering: no)

### Heartbeats Not Working

If heartbeats aren't being sent:

1. Verify heartbeat ticker is running (30s interval)
2. Check response writer flush is working
3. Look for flush errors in logs
