# Decision: Progress Broadcast Throttling (ADR-001)

**Date**: 2025-12-02
**Status**: Active
**Context**: SSE progress event frequency management

## Decision

Progress broadcasts use a throttling system:
1. **Regular throttling**: 2 seconds minimum between progress percentage/message updates
2. **Stage transitions**: Broadcast immediately without throttling
3. **Terminal events**: Always broadcast immediately without any throttling

## Implementation

### Throttle Tracking

Throttle state is tracked at the Service level in `internal/service/progress/service.go`:

```go
type Service struct {
    // ... other fields
    lastBroadcast map[string]time.Time  // Per-operation throttle tracking
}
```

Key methods:
- `updateOperationThrottled()` - applies 2-second interval before broadcasting
- `updateOperationImmediate()` - broadcasts immediately for state/stage changes

Key constants:
- `DefaultProgressBroadcastInterval = 2 * time.Second` - for throttled updates

### SSE Flushing

The SSE handler uses `http.ResponseController` (Go 1.20+) for reliable flushing:
- Each event write is followed by `rc.Flush()` with error handling
- Flush errors indicate client disconnect and terminate the handler
- Short write detection logs partial writes for debugging

## Rules

1. **Per-Operation Tracking**: Throttle state is tracked by operation ID, not globally. Each concurrent operation has independent throttle tracking.

2. **Update Types**:
   - **Throttled updates** (2 seconds): Progress percentage, message, metadata, warnings
   - **Immediate updates** (no delay): Stage transitions, state changes, terminal events

3. **Terminal Events Always Broadcast**: These bypass ALL throttling:
   - Operation completion (`completed`)
   - Operation error (`error`)
   - Operation cancellation (`cancelled`)

4. **Cleanup on Termination**: When an operation reaches a terminal state, its throttle tracking entry is deleted from the map to prevent memory leaks.

5. **No Per-Channel Progress Updates**: Ingestion operations do not broadcast progress for each downloaded channel. Progress updates are limited to stage transitions and throttled periodic updates.

## Rationale

Without throttling, high-frequency progress updates would:
- Overwhelm SSE connections with redundant events
- Cause excessive frontend re-renders
- Increase network bandwidth usage
- Reduce overall system responsiveness

The throttling system provides:
- 2-second interval for progress updates (efficient, still perceivable)
- Immediate delivery for stage transitions (responsive user experience)
- Immediate delivery for terminal events (critical for UI state)

## Alternative Considered

Frontend-side throttling was considered but rejected because:
- Still transmits all events over the network
- Inconsistent behavior across different clients
- Server-side throttling is more efficient and consistent

## References

- Implementation: `internal/service/progress/service.go`
- SSE Handler: `internal/http/handlers/progress.go`
- Constants: `DefaultProgressBroadcastInterval`
