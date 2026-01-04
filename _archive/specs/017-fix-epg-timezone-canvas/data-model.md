# Data Model: Fix EPG Timezone Normalization and Canvas Layout

**Date**: 2025-12-14
**Feature**: 017-fix-epg-timezone-canvas
**Spec**: [spec.md](./spec.md)

## Overview

This document defines the data structures and modifications for the EPG timezone normalization and canvas layout fix. Key areas:
1. Existing entity modifications (EpgSource timezone fields)
2. Frontend state models for canvas metrics
3. API request/response structures for lazy loading

## Existing Entity Modifications

### EpgSource (Modified)

The `EpgSource` model already has the required fields for timezone handling. No schema changes needed - only the ingestion logic changes.

```go
// internal/models/epg_source.go (existing fields, no changes)
type EpgSource struct {
    BaseModel

    // ... existing fields ...

    // DetectedTimezone is the timezone detected from the EPG data (e.g., "+00:00", "+01:00").
    // This is auto-detected during ingestion and updated on each ingest. Read-only for users.
    DetectedTimezone string `gorm:"size:50" json:"detected_timezone,omitempty"`

    // EpgShift is a manual time shift in hours to adjust EPG times (e.g., -2, +1).
    // Use this when the detected timezone is incorrect or you want to shift all times.
    // Positive values shift times forward, negative values shift times back.
    EpgShift int `gorm:"default:0" json:"epg_shift"`
}
```

**Behavior Change**: During ingestion, `DetectedTimezone` is used to normalize program times to UTC:
- Detected offset `+01:00` → subtract 1 hour from all program times
- Detected offset `-05:00` → add 5 hours to all program times
- After UTC normalization, apply `EpgShift` (add hours)

### EpgProgram (No Changes)

The `EpgProgram` model remains unchanged. Times are already stored as `time.Time` in UTC.

```go
// internal/models/epg_program.go (existing, no changes)
type EpgProgram struct {
    BaseModel

    SourceID  ULID      `gorm:"type:varchar(26);not null;..." json:"source_id"`
    ChannelID string    `gorm:"not null;size:255;..." json:"channel_id"`
    Start     time.Time `gorm:"not null;..." json:"start"`  // Stored in UTC
    Stop      time.Time `gorm:"not null;index" json:"stop"` // Stored in UTC
    Title     string    `gorm:"not null;size:512" json:"title"`
    // ... other fields unchanged
}
```

### Channel (No Changes)

The `Channel` model remains unchanged. Used for search functionality.

```go
// internal/models/channel.go (existing, no changes)
type Channel struct {
    BaseModel

    TvgID       string `gorm:"size:255;index" json:"tvg_id,omitempty"`
    ChannelName string `gorm:"not null;size:512" json:"channel_name"`
    // ... other fields unchanged
}
```

## Frontend State Models

### CanvasMetrics (New)

Runtime state for dynamic time-to-pixel calculations.

```typescript
// frontend/src/hooks/useCanvasMetrics.ts

interface CanvasMetrics {
  // Container dimensions
  containerWidth: number;
  containerHeight: number;

  // Calculated values
  pixelsPerHour: number;        // Dynamic: (containerWidth - sidebarWidth) / hoursToDisplay
  sidebarWidth: number;         // Fixed or observed from sidebar element
  hoursToDisplay: number;       // Number of hours visible at once

  // Guide time boundaries
  guideStartTime: number;       // Unix timestamp (ms) of leftmost visible time
  guideEndTime: number;         // Unix timestamp (ms) of rightmost loaded data

  // Scroll state (stored as time, not pixels)
  scrollTimeMs: number;         // Unix timestamp (ms) at current scroll position

  // Minimum thresholds
  minPixelsPerHour: number;     // 50px minimum for readability
  minProgramWidth: number;      // Minimum clickable width (25px)
}

// Default values
const DEFAULT_METRICS: CanvasMetrics = {
  containerWidth: 0,
  containerHeight: 0,
  pixelsPerHour: 200,           // Initial default, recalculated on mount
  sidebarWidth: 200,            // Will be observed
  hoursToDisplay: 6,            // Default visible hours
  guideStartTime: Date.now(),
  guideEndTime: Date.now() + 12 * 60 * 60 * 1000,
  scrollTimeMs: Date.now(),
  minPixelsPerHour: 50,
  minProgramWidth: 25,
};
```

### LazyLoadState (New)

State for managing lazy loading of EPG data.

```typescript
// frontend/src/components/epg/CanvasEPG.tsx

interface LazyLoadState {
  // Loaded data boundaries
  loadedStartTime: number;      // Unix timestamp (ms) of earliest loaded data
  loadedEndTime: number;        // Unix timestamp (ms) of latest loaded data

  // Loading state
  isLoadingMore: boolean;       // Currently fetching more data
  hasReachedEnd: boolean;       // No more data available beyond loadedEndTime

  // Configuration
  fetchThresholdHours: number;  // Trigger fetch when within N hours of boundary (default: 2)
  fetchChunkHours: number;      // Fetch N hours of data per request (default: 12)
}
```

### SearchState (Modified)

Enhanced search state for comprehensive field search.

```typescript
// frontend/src/app/epg/page.tsx

interface SearchState {
  query: string;                // User's search input
  isSearching: boolean;         // Search in progress

  // Fields to search (all enabled by default)
  searchFields: {
    channelName: boolean;
    channelTvgId: boolean;
    programTitle: boolean;
    programDescription: boolean;
  };
}
```

## API Request/Response Structures

### GetGuide (Modified)

The existing `/api/v1/epg/guide` endpoint already supports time-range queries. Minor enhancement for lazy loading.

**Existing Input** (no changes needed):
```go
// internal/http/handlers/epg.go
type GetGuideInput struct {
    StartTime string `query:"start_time"` // RFC3339 time
    EndTime   string `query:"end_time"`   // RFC3339 time
    SourceID  string `query:"source_id"`  // Optional filter
}
```

**Existing Output** (no changes needed):
```go
type GuideData struct {
    Channels  map[string]GuideChannelInfo   `json:"channels"`
    Programs  map[string][]GuideProgramInfo `json:"programs"`
    TimeSlots []string                      `json:"time_slots"`
    StartTime string                        `json:"start_time"`
    EndTime   string                        `json:"end_time"`
}
```

**Usage for Lazy Loading**:
```
# Initial load (12 hours from now)
GET /api/v1/epg/guide?start_time=2025-12-14T12:00:00Z&end_time=2025-12-15T00:00:00Z

# Subsequent load (next 12 hours)
GET /api/v1/epg/guide?start_time=2025-12-15T00:00:00Z&end_time=2025-12-15T12:00:00Z
```

### EpgSource Response (No Changes)

The `DetectedTimezone` field is already included in API responses.

```json
{
  "id": "01HGW2QKT...",
  "name": "My EPG Source",
  "url": "http://example.com/epg.xml",
  "detected_timezone": "+01:00",
  "epg_shift": 0,
  "status": "success",
  "program_count": 5000
}
```

## Timezone Normalization Logic

### Offset Parsing

Parse timezone offset from XMLTV format (e.g., `20251214140000 +0100`).

```go
// internal/ingestion/xmltv_handler.go (implementation detail)

// parseTimezoneOffset parses offset strings like "+0100", "-0530", "+00:00"
// Returns the offset as a time.Duration
func parseTimezoneOffset(offset string) (time.Duration, error) {
    // "+0100" -> +1 hour
    // "-0530" -> -5 hours 30 minutes
    // "+00:00" -> 0
}
```

### Normalization Formula

```go
// Pseudocode for normalization during ingestion

// Step 1: Parse program time from XMLTV (includes timezone)
rawTime := parseXMLTVTime("20251214140000 +0100") // 14:00 local time

// Step 2: Detect timezone offset (auto-detected per source)
detectedOffset := time.Hour // +01:00

// Step 3: Normalize to UTC by applying INVERSE of detected offset
utcTime := rawTime.Add(-detectedOffset) // 14:00 - 1hr = 13:00 UTC

// Step 4: Apply user-configured timeshift (hours)
finalTime := utcTime.Add(time.Duration(source.EpgShift) * time.Hour)

// Store finalTime in database
program.Start = finalTime
```

## Canvas Calculation Formulas

### Pixels Per Hour

```typescript
// Calculate pixels per hour dynamically
const calculatePixelsPerHour = (
  containerWidth: number,
  sidebarWidth: number,
  hoursToDisplay: number,
  minPixelsPerHour: number
): number => {
  const availableWidth = containerWidth - sidebarWidth;
  const calculated = availableWidth / hoursToDisplay;
  return Math.max(minPixelsPerHour, calculated);
};
```

### Program Position & Width

```typescript
const MS_PER_HOUR = 60 * 60 * 1000;

// Calculate program left position (pixels from guide start)
const programLeft = ((programStart - guideStartTime) / MS_PER_HOUR) * pixelsPerHour;

// Calculate program width (pixels based on duration)
const programWidth = ((programEnd - programStart) / MS_PER_HOUR) * pixelsPerHour;

// Enforce minimum width
const actualWidth = Math.max(programWidth, minProgramWidth);
```

### Scroll Position Preservation

```typescript
// Before resize: convert pixel scroll to time
const currentScrollTime = guideStartTime + (scrollX / pixelsPerHour) * MS_PER_HOUR;

// After resize: convert time back to pixels with new ratio
const newScrollX = ((currentScrollTime - guideStartTime) / MS_PER_HOUR) * newPixelsPerHour;
```

## State Transitions

### Lazy Loading State Machine

```
[Initial] → [Loading First Chunk] → [Idle]
     ↓              ↓                 ↓
  [Error]       [Error]       [Near Boundary]
                                    ↓
                            [Loading More] → [Idle]
                                    ↓          ↓
                                [Error]   [Reached End]
```

### Resize State Machine

```
[Stable] → [Resize Event] → [Debouncing]
              ↑                  ↓
              └──── [Timeout] ───┘
                       ↓
               [Recalculating]
                       ↓
                   [Stable]
```

## Validation Rules

### Timezone Offset Validation

| Offset Format | Valid | Example |
|--------------|-------|---------|
| `+HHMM` | Yes | `+0100`, `+1200` |
| `-HHMM` | Yes | `-0500`, `-1100` |
| `+HH:MM` | Yes | `+01:00`, `+12:00` |
| `-HH:MM` | Yes | `-05:00`, `-11:00` |
| `Z` | Yes | UTC equivalent |
| Empty/None | Yes | Treated as UTC |
| Invalid | No | `+25:00`, `abc` |

### Canvas Constraints

| Parameter | Min | Max | Default |
|-----------|-----|-----|---------|
| `pixelsPerHour` | 50 | 400 | 200 |
| `hoursToDisplay` | 1 | 24 | 6 |
| `minProgramWidth` | 10 | 50 | 25 |
| `fetchThresholdHours` | 1 | 6 | 2 |
| `fetchChunkHours` | 6 | 24 | 12 |

### Search Input Validation

| Field | Constraint |
|-------|------------|
| `query` | Min 1 character, max 100 characters |
| Search sanitization | Escape special regex characters |
| Case sensitivity | Case-insensitive (toLowerCase comparison) |

## No New Database Tables

This feature modifies behavior only - no schema migrations required:
- `EpgSource.DetectedTimezone` - already exists
- `EpgSource.EpgShift` - already exists
- `EpgProgram.Start/Stop` - already stored in UTC format
- `Channel` - no changes
