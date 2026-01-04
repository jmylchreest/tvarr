# Data Model: Pipeline Logging, Error Feedback, and M3U/XMLTV Generation

**Date**: 2025-12-02
**Feature Branch**: `002-pipeline-logging-fixes`

## Overview

This feature primarily modifies behavior rather than data models. The only new data structure is `ErrorDetail` for enhanced error reporting. All other changes are to logging output and transient state.

## New Types

### ErrorDetail (Progress Enhancement)

**Location**: `internal/service/progress/types.go`

```go
// ErrorDetail provides structured error information for UI display.
type ErrorDetail struct {
    Stage      string `json:"stage"`       // Stage that failed (e.g., "generate_m3u")
    Message    string `json:"message"`     // User-friendly error message
    Technical  string `json:"technical"`   // Technical details for debugging
    Suggestion string `json:"suggestion"`  // Actionable suggestion for resolution
}
```

**Validation Rules**:
- Stage: Required, must be valid stage ID
- Message: Required, non-empty
- Technical: Optional, may contain error details
- Suggestion: Optional, human-readable guidance

**Relationship to UniversalProgress**:
- Replaces simple `Error string` with structured `ErrorDetail`
- Backward compatible: Error field retained, ErrorDetail added

### Stage Logging Context

**Location**: Used in each stage, not persisted

```go
// StageLogContext provides consistent logging context for stages.
type StageLogContext struct {
    StageID    string        // Stage identifier
    StageName  string        // Human-readable name
    ProxyID    string        // Proxy being generated
    SourceID   string        // Current source (if applicable)
    SourceName string        // Current source name
    BatchNum   int           // Current batch number
    TotalBatch int           // Total batch count
    ItemStart  int           // First item index in batch
    ItemEnd    int           // Last item index in batch
}
```

## Modified Types

### UniversalProgress (Existing)

**Location**: `internal/service/progress/types.go`

**Additions**:
```go
type UniversalProgress struct {
    // ... existing fields ...

    // NEW: Structured error details (in addition to Error string)
    ErrorDetail *ErrorDetail `json:"error_detail,omitempty"`

    // NEW: Warning count for non-fatal issues
    WarningCount int `json:"warning_count,omitempty"`

    // NEW: Warnings slice for detailed warning messages
    Warnings []string `json:"warnings,omitempty"`
}
```

### StageResult (Existing)

**Location**: `internal/pipeline/core/interfaces.go`

**No schema changes** - existing `Message` and `Errors` fields sufficient.

## No Database Changes

This feature does not modify database schema. All changes are:
1. In-memory logging output
2. Transient progress events (SSE)
3. Temporary files (already handled)

## Entity Relationship Context

```
StreamProxy (existing)
    |
    +-- StreamProxySource (existing, priority-ordered)
    |       |
    |       +-- StreamSource (existing)
    |               |
    |               +-- Channel (existing)
    |
    +-- StreamProxyEpgSource (existing, priority-ordered)
            |
            +-- EpgSource (existing)
                    |
                    +-- EpgProgram (existing)

Pipeline Execution (transient)
    |
    +-- State (in-memory)
    |       |
    |       +-- Channels []*Channel
    |       +-- Programs []*EpgProgram
    |       +-- Errors []error
    |       +-- TempDir string
    |
    +-- Progress Events (SSE)
            |
            +-- UniversalProgress
                    |
                    +-- ErrorDetail (NEW)
                    +-- Warnings (NEW)
```

## State Transitions

### Progress State Machine

```
idle -> preparing -> processing -> completed
                 \               /
                  `-> error ----'
                 \               /
                  `-> cancelled -'
```

### Stage Processing States

```
starting -> processing -> completed
                      \
                       `-> failed (with ErrorDetail)
```

## File Artifacts

### M3U Output Format

```m3u
#EXTM3U
#EXTINF:-1 tvg-id="channel1" tvg-name="Channel One" tvg-logo="http://..." tvg-chno="1" group-title="News",Channel One HD
http://proxy/stream/{proxy_id}/{channel_id}
```

### XMLTV Output Format

```xml
<?xml version="1.0" encoding="UTF-8"?>
<tv generator-info-name="tvarr">
  <channel id="channel1">
    <display-name>Channel One HD</display-name>
    <icon src="http://..."/>
  </channel>
  <programme start="20251202120000 +0000" stop="20251202130000 +0000" channel="channel1">
    <title>Program Title</title>
    <desc>Program description</desc>
  </programme>
</tv>
```

## Logging Schema

### Standard Log Fields

| Field | Type | Description |
|-------|------|-------------|
| stage_id | string | Stage identifier |
| stage_name | string | Human-readable name |
| stage_num | int | Position in sequence (1-based) |
| total_stages | int | Total stage count |
| proxy_id | string | ULID of proxy |
| source_id | string | ULID of source (if applicable) |
| source_name | string | Source name (if applicable) |
| duration | duration | Elapsed time |
| records_processed | int | Items processed |
| records_modified | int | Items changed (for mapping stages) |
| batch_num | int | Current batch (DEBUG) |
| total_batches | int | Total batches (DEBUG) |
| error | string | Error message |

### Log Levels by Action

| Action | Level | Required Fields |
|--------|-------|-----------------|
| Stage start | INFO | stage_id, stage_name, stage_num, total_stages |
| Source processing | INFO | source_id, source_name |
| Batch progress | DEBUG | batch_num, total_batches |
| Stage completion | INFO | stage_id, duration, records_processed |
| Stage warning | WARN | stage_id, warning message |
| Stage error | ERROR | stage_id, error, partial state |
