# Research: EPG Timezone & Canvas Layout

**Feature**: 017-fix-epg-timezone-canvas
**Date**: 2025-12-14

## Executive Summary

Research consolidated from spec and codebase analysis. All critical decisions resolved.

---

## 1. Timezone Normalization Strategy

### Decision
Apply the **inverse of the detected timezone offset** during ingestion to normalize all program times to UTC. Timeshift is applied after normalization.

### Rationale
- XMLTV timestamps include timezone offsets (e.g., `20251214140000 +0100`)
- Current code calls `.UTC()` which only strips timezone info, doesn't apply offset
- Proper normalization: `+0100` detected → subtract 1 hour to get UTC
- This ensures the "now" indicator aligns with actual current time

### Current Implementation Issue
```go
// Current (broken) - xmltv_handler.go
t = t.UTC()  // Just strips timezone, doesn't apply offset

// Required (correct)
// If detected timezone is +01:00, subtract 1 hour
t = t.Add(-detectedOffset).UTC()
// Then apply user timeshift
t = t.Add(time.Duration(source.EpgShift) * time.Hour)
```

### Alternatives Considered
| Alternative | Why Rejected |
|-------------|--------------|
| Store in source timezone, convert on display | Complex, error-prone, requires frontend to know all offsets |
| Detect timezone per-program | Overcomplicated, XMLTV uses consistent timezone per feed |
| Ignore timezone entirely | Breaks for non-UTC feeds (majority of real-world EPG sources) |

---

## 2. Canvas Time-to-Pixel Mapping

### Decision
Replace hardcoded `PIXELS_PER_HOUR = 200` with dynamically calculated value based on container width and hours to display.

### Rationale
- Current constant doesn't adapt to viewport changes
- Professional EPG implementations (Planby, Kodi, TiVo) all use adaptive ratios
- Canvas already performs well; only the metric calculation needs fixing

### Implementation Pattern
```typescript
// Dynamic calculation
const calculatePixelsPerHour = (containerWidth: number, sidebarWidth: number, hoursToDisplay: number) => {
  const availableWidth = containerWidth - sidebarWidth;
  const calculated = availableWidth / hoursToDisplay;
  return Math.max(MIN_PIXELS_PER_HOUR, calculated);  // MIN = 50
};

// Position formula (unchanged logic, dynamic value)
const programLeft = ((startTime - guideStart) / MS_PER_HOUR) * pixelsPerHour;
const programWidth = (duration / MS_PER_HOUR) * pixelsPerHour;
```

### Alternatives Considered
| Alternative | Why Rejected |
|-------------|--------------|
| Replace with Planby | DOM-based, different performance profile, major refactor |
| Replace with react-tv-epg | Less maintained, sparse documentation |
| Fixed viewport with zoom controls | More complex UX, doesn't solve resize issue |

---

## 3. Scroll Position Preservation During Resize

### Decision
Store scroll position as **time** (Date/timestamp), not pixels. Convert to pixels on each render.

### Rationale
- Pixel positions become invalid when `pixelsPerHour` changes
- Time-based position remains valid regardless of scale
- Pattern: `scrollTime → pixelsPerHour × timeOffset = scrollPixels`

### Implementation Pattern
```typescript
// Before resize
const scrollTimeMs = guideStartTime + (scrollX / pixelsPerHour) * MS_PER_HOUR;

// After resize (new pixelsPerHour calculated)
const newScrollX = ((scrollTimeMs - guideStartTime) / MS_PER_HOUR) * newPixelsPerHour;
```

### Alternatives Considered
| Alternative | Why Rejected |
|-------------|--------------|
| Reset scroll to "now" on resize | Poor UX, loses user's browsing position |
| Proportional pixel scaling | Breaks if hours-to-display also changes |

---

## 4. Lazy Loading Strategy

### Decision
Trigger fetch when user scrolls within **2 hours** of loaded data boundary. Fetch **12-hour chunks**. Retain all previously loaded data.

### Rationale
- 2-hour threshold provides time for async fetch to complete
- 12-hour chunks balance between request count and data freshness
- Retaining loaded data prevents redundant fetches when scrolling back

### Implementation Pattern
```typescript
// Detect scroll position relative to loaded boundary
const currentTimeAtRightEdge = guideStartTime + ((scrollX + viewportWidth) / pixelsPerHour) * MS_PER_HOUR;
const loadedBoundary = loadedDataEndTime;
const threshold = 2 * 60 * 60 * 1000;  // 2 hours in ms

if (currentTimeAtRightEdge + threshold > loadedBoundary && !loading && !reachedEnd) {
  fetchNextChunk(loadedBoundary, loadedBoundary + 12 * 60 * 60 * 1000);
}
```

### API Contract
```
GET /api/epg/programs?start_time={ISO8601}&end_time={ISO8601}&channel_ids={comma-separated}
```

---

## 5. Comprehensive Search Strategy

### Decision
Client-side filtering across: `channel.name`, `channel.tvg_id`, `program.title`, `program.description`. Case-insensitive substring match.

### Rationale
- EPG data is already loaded in memory for canvas rendering
- Simple substring matching is sufficient per spec
- Avoids additional API calls for search

### Implementation Pattern
```typescript
const matchesSearch = (term: string, channel: Channel, programs: Program[]) => {
  const lowerTerm = term.toLowerCase();

  // Match channel metadata
  if (channel.name.toLowerCase().includes(lowerTerm)) return true;
  if (channel.tvg_id?.toLowerCase().includes(lowerTerm)) return true;

  // Match any program
  return programs.some(p =>
    p.title.toLowerCase().includes(lowerTerm) ||
    p.description?.toLowerCase().includes(lowerTerm)
  );
};
```

### Alternatives Considered
| Alternative | Why Rejected |
|-------------|--------------|
| Server-side full-text search | Overcomplicated for current scale, adds latency |
| Fuzzy matching | Not required per spec; substring sufficient |
| Regex patterns | Security risk, UX complexity |

---

## 6. Sidebar Width Handling

### Decision
Use CSS layout constraints (flexbox/grid) to ensure canvas container never overlaps sidebar. Canvas reads container width, not viewport width.

### Rationale
- Current issue: Canvas calculates width from viewport, ignores sidebar
- Fix: Container already accounts for sidebar; Canvas should use container bounds

### Implementation Pattern
```typescript
// Use ResizeObserver on container, not window
const containerRef = useRef<HTMLDivElement>(null);

useEffect(() => {
  const observer = new ResizeObserver(entries => {
    const { width, height } = entries[0].contentRect;
    setContainerDimensions({ width, height });
    // Recalculate pixelsPerHour with new width
  });
  if (containerRef.current) observer.observe(containerRef.current);
  return () => observer.disconnect();
}, []);
```

---

## 7. Resize Debouncing

### Decision
Debounce resize events with **100ms** delay to prevent excessive re-renders.

### Rationale
- Continuous resize events can fire 60+ times per second
- Canvas re-render is expensive
- 100ms is imperceptible delay but reduces render count by ~90%

### Implementation
```typescript
const debouncedResize = useMemo(
  () => debounce((width: number, height: number) => {
    recalculateMetrics(width, height);
  }, 100),
  []
);
```

---

## 8. Minimum Readable Width

### Decision
Minimum `pixelsPerHour = 50`. When calculated value falls below, enable horizontal scrolling rather than compressing further.

### Rationale
- Below 50px/hour, program titles become unreadable
- Scrolling is better UX than illegible text
- Matches industry standards for EPG displays

### Calculation
```
50px/hour = 1 hour shows as 50px
→ 30-min program = 25px (minimum clickable)
→ 6 hours = 300px minimum grid width
```

---

## Dependencies & Integration Points

### Backend Changes
- `internal/ingestion/xmltv_handler.go` - Timezone normalization
- `internal/ingestion/xtream_epg_handler.go` - Timezone normalization
- `internal/http/handlers/epg_handler.go` - Time-range query support

### Frontend Changes
- `frontend/src/components/epg/CanvasEPG.tsx` - Dynamic metrics, search
- `frontend/src/app/epg/page.tsx` - Remove timezone dropdown, lazy loading
- `frontend/src/lib/api-client.ts` - Time-range EPG fetching

### No New Dependencies Required
- All changes use existing libraries (React, HTML5 Canvas, GORM)
- Planby evaluated but not needed; current Canvas approach is sufficient

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Performance regression from frequent metric recalculation | Debounce resize, memoize calculations |
| Timezone normalization breaks existing data | Add migration to re-normalize all programs on first run post-deploy |
| Lazy loading causes memory bloat | Implement LRU cache to evict oldest data after 48h loaded |
| Search slows with large datasets | Current scale (thousands of programs) is within O(n) client-side limits |
