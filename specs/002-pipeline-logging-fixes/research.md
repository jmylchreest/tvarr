# Research: Pipeline Logging, Error Feedback, and M3U/XMLTV Generation

**Date**: 2025-12-02
**Feature Branch**: `002-pipeline-logging-fixes`

## Research Tasks

### 1. m3u-proxy Reference Codebase Logging Patterns

**Decision**: Adopt structured logging with execution ID correlation and stage-level metrics

**Rationale**: The m3u-proxy Rust codebase demonstrates comprehensive logging patterns:
- Each pipeline execution has a unique prefix (e.g., `exec=20241201-123456`) for log correlation
- Stage start logs include input counts and configuration
- Stage progress logs include batch progress and intermediate counts
- Stage completion logs include timing, output counts, and statistics
- Error logs include full context (source, item being processed, error details)

**Key Patterns to Adopt**:
```go
// Stage start - always INFO
logger.InfoContext(ctx, "stage starting",
    slog.String("stage_id", StageID),
    slog.Int("input_count", len(items)),
)

// Batch progress - DEBUG level
logger.DebugContext(ctx, "processing batch",
    slog.Int("batch_num", batchNum),
    slog.Int("total_batches", totalBatches),
    slog.Int("items_start", startIdx),
    slog.Int("items_end", endIdx),
)

// Source processing - INFO level
logger.InfoContext(ctx, "processing source",
    slog.String("source_id", source.ID.String()),
    slog.String("source_name", source.Name),
    slog.Int("channel_count", channelCount),
)

// Stage completion - INFO level
logger.InfoContext(ctx, "stage completed",
    slog.String("stage_id", StageID),
    slog.Duration("duration", elapsed),
    slog.Int("records_processed", count),
    slog.Int("records_modified", modified),
)

// Error with context - ERROR level
logger.ErrorContext(ctx, "failed to process item",
    slog.String("stage_id", StageID),
    slog.String("item_id", item.ID),
    slog.String("error", err.Error()),
)
```

**Alternatives Considered**:
- Minimal logging (only errors): Rejected - insufficient for debugging
- Trace-level verbose logging: Rejected - too noisy, use DEBUG selectively

---

### 2. Existing Tvarr Pipeline Logging Gaps

**Decision**: Enhance each stage with standardized logging, keeping orchestrator logging as-is

**Rationale**: Analysis of current tvarr pipeline:

| Component | Current State | Gap |
|-----------|--------------|-----|
| `orchestrator.go` | Good - logs stage start/end/error | Add records_processed to start log |
| `loadchannels/stage.go` | None inside Execute | Add per-source logging, counts |
| `loadprograms/stage.go` | None inside Execute | Add per-source logging, counts |
| `filtering/stage.go` | None inside Execute | Add filter stats, kept/removed counts |
| `datamapping/stage.go` | None inside Execute | Add rule application stats |
| `numbering/stage.go` | None inside Execute | Add numbering stats |
| `logocaching/stage.go` | None inside Execute | Add cache hit/miss stats |
| `generatem3u/stage.go` | None inside Execute | Add file size, validation status |
| `generatexmltv/stage.go` | None inside Execute | Add program count, file size |
| `publish/stage.go` | None inside Execute | Add file copy success/failure |

**Implementation Approach**:
- Add logger field to BaseStage or inject via deps
- Each stage logs at INFO level for start/completion
- Each stage logs at DEBUG level for batch/item progress
- Use slog.With() to add stage context to all logs

**Alternatives Considered**:
- Central logging wrapper: Rejected - stages know their own context
- Event-based logging: Rejected - adds complexity without benefit

---

### 3. Temp Directory Cleanup Strategy

**Decision**: Use deferred cleanup in orchestrator + startup sweep for orphans

**Rationale**: Current orchestrator already handles cleanup via `defer os.RemoveAll(tempDir)`. Gaps:
1. Crash during execution leaves orphan directories
2. No startup cleanup of orphans
3. No cleanup on error before defer executes (not a real gap - defer handles it)

**Implementation**:
```go
// internal/startup/cleanup.go
package startup

func CleanupOrphanedTempDirs(logger *slog.Logger) error {
    pattern := "tvarr-proxy-*"
    matches, err := filepath.Glob(filepath.Join(os.TempDir(), pattern))
    if err != nil {
        return err
    }

    for _, dir := range matches {
        // Check age - only remove if > 1 hour old (stale)
        info, err := os.Stat(dir)
        if err != nil {
            continue
        }
        if time.Since(info.ModTime()) > time.Hour {
            logger.Info("removing orphaned temp directory",
                slog.String("path", dir),
                slog.Duration("age", time.Since(info.ModTime())),
            )
            os.RemoveAll(dir)
        }
    }
    return nil
}
```

**Alternatives Considered**:
- Scheduled cleanup task: Rejected - startup is simpler and sufficient
- OS tmp cleaner reliance: Rejected - not all systems clean /tmp

---

### 4. Progress Event Error Structure

**Decision**: Extend existing `UniversalProgress.Error` field with structured error details

**Rationale**: Current progress types (`internal/service/progress/types.go`) already have:
- `Error string` field on `UniversalProgress`
- `StateError` state for terminal failure

**Enhancement**:
```go
// Add to progress/types.go

// ErrorDetail provides structured error information for UI display.
type ErrorDetail struct {
    Stage      string `json:"stage"`       // Stage that failed
    Message    string `json:"message"`     // User-friendly message
    Technical  string `json:"technical"`   // Technical details (for logs)
    Suggestion string `json:"suggestion"`  // Actionable suggestion
}

// Example usage in proxy_service.go:
progressMgr.FailWithDetail(ErrorDetail{
    Stage:      "generate_m3u",
    Message:    "Failed to create M3U playlist",
    Technical:  err.Error(),
    Suggestion: "Check disk space and output directory permissions",
})
```

**Alternatives Considered**:
- Separate error event type: Rejected - adds complexity
- Error codes: Rejected - strings are more flexible for new error types

---

### 5. UI Error Display Patterns

**Decision**: Use existing toast system with enhanced error state styling

**Rationale**: Frontend already has:
- `ProgressProvider.tsx` with SSE handling
- `events.tsx` with progress display
- Backend connectivity error display pattern

**Implementation**:
- Error state shows in progress indicator with red styling
- Error details accessible via click/hover
- Toast notification on new errors
- Source/proxy cards show error indicator badge

**Alternatives Considered**:
- Modal error dialog: Rejected - interrupts workflow
- Separate error log page: Rejected - disconnects from context

---

### 6. M3U/XMLTV Output Validation

**Decision**: Add validation checks during generation, log warnings for issues

**Rationale**: Valid M3U requires:
- `#EXTM3U` header
- `#EXTINF` line before each URL
- Non-empty stream URLs

Valid XMLTV requires:
- XML declaration
- `<tv>` root element
- `<channel>` elements with id attribute
- `<programme>` elements with channel, start, stop attributes

**Implementation**:
```go
// In generatem3u/stage.go - validate during write
if ch.StreamURL == "" {
    state.AddError(fmt.Errorf("channel %s has empty stream URL", ch.ChannelName))
    continue // Skip but don't fail
}

// In generatexmltv/stage.go - validate during write
if prog.ChannelID == "" || prog.Start.IsZero() {
    state.AddError(fmt.Errorf("program missing required fields"))
    continue
}

// Log summary warnings at stage end
if len(state.Errors) > 0 {
    logger.WarnContext(ctx, "stage completed with warnings",
        slog.Int("warning_count", len(state.Errors)),
    )
}
```

**Alternatives Considered**:
- Post-generation validation pass: Rejected - doubles file I/O
- Fail on any invalid item: Rejected - one bad channel shouldn't break whole playlist

---

## Summary of Decisions

| Area | Decision | Key Change |
|------|----------|------------|
| Logging Pattern | Structured slog with context | Add logger to stages, use consistent format |
| Stage Logging | Per-stage INFO/DEBUG logs | Add logging to each stage Execute() |
| Temp Cleanup | Orchestrator defer + startup sweep | Add startup/cleanup.go |
| Error Structure | ErrorDetail struct | Extend progress types |
| UI Errors | Toast + indicator badges | Enhance existing components |
| Output Validation | Inline validation with warnings | Skip invalid, log warnings |

## Dependencies Resolved

All technologies are already in use:
- slog: Already configured in `internal/observability/logger.go`
- Progress service: Already implemented in `internal/service/progress/`
- SSE: Already implemented in handlers/progress.go
- Toast/notifications: Already in frontend providers

**No new external dependencies required.**
