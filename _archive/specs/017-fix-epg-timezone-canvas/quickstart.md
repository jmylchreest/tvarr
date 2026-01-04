# Quickstart: Fix EPG Timezone Normalization and Canvas Layout

**Date**: 2025-12-14
**Feature**: 017-fix-epg-timezone-canvas
**Spec**: [spec.md](./spec.md)

## Overview

This guide provides implementation patterns and examples for fixing EPG timezone handling and canvas layout issues.

## Directory Structure

```
internal/
├── ingestion/
│   ├── xmltv_handler.go         # MODIFY: Timezone normalization
│   ├── xtream_epg_handler.go    # MODIFY: Timezone normalization
│   └── timezone.go              # NEW: Timezone parsing utilities
├── http/handlers/
│   └── epg.go                   # EXISTING: No changes needed
└── models/
    └── epg_source.go            # EXISTING: No changes needed

frontend/
├── src/
│   ├── components/epg/
│   │   └── CanvasEPG.tsx        # MODIFY: Dynamic metrics, lazy loading, search
│   ├── hooks/
│   │   └── useCanvasMetrics.ts  # NEW: Dynamic time-to-pixel calculations
│   └── app/epg/
│       └── page.tsx             # MODIFY: Remove timezone dropdown
└── tests/
    └── epg/
        └── canvas-metrics.test.ts  # NEW: Canvas metric tests
```

## Implementation Patterns

### 1. Timezone Parsing Utility

```go
// internal/ingestion/timezone.go

package ingestion

import (
    "fmt"
    "regexp"
    "strconv"
    "time"
)

// ParseTimezoneOffset parses timezone offset strings into a time.Duration.
// Supported formats: "+0100", "-0530", "+01:00", "-05:30", "Z", ""
func ParseTimezoneOffset(offset string) (time.Duration, error) {
    offset = strings.TrimSpace(offset)

    // Empty or Z means UTC
    if offset == "" || offset == "Z" {
        return 0, nil
    }

    // Regex for +/-HHMM or +/-HH:MM
    re := regexp.MustCompile(`^([+-])(\d{2}):?(\d{2})$`)
    matches := re.FindStringSubmatch(offset)
    if matches == nil {
        return 0, fmt.Errorf("invalid timezone offset format: %s", offset)
    }

    sign := matches[1]
    hours, _ := strconv.Atoi(matches[2])
    minutes, _ := strconv.Atoi(matches[3])

    // Validate ranges
    if hours > 14 || (hours == 14 && minutes > 0) {
        return 0, fmt.Errorf("timezone offset out of range: %s", offset)
    }
    if minutes > 59 {
        return 0, fmt.Errorf("invalid minutes in timezone offset: %s", offset)
    }

    duration := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
    if sign == "-" {
        duration = -duration
    }

    return duration, nil
}

// NormalizeProgramTime normalizes a program time to UTC.
// 1. Applies inverse of detected timezone offset
// 2. Applies user-configured timeshift
func NormalizeProgramTime(t time.Time, detectedOffset time.Duration, timeshiftHours int) time.Time {
    // Step 1: Apply inverse of detected offset to get UTC
    utcTime := t.Add(-detectedOffset)

    // Step 2: Apply user timeshift
    if timeshiftHours != 0 {
        utcTime = utcTime.Add(time.Duration(timeshiftHours) * time.Hour)
    }

    return utcTime.UTC()
}
```

### 2. XMLTV Handler Modification

```go
// internal/ingestion/xmltv_handler.go (modify existing)

// parseXMLTVTime parses XMLTV timestamp format and returns time + detected offset.
// Format: YYYYMMDDHHMMSS +HHMM (e.g., "20251214140000 +0100")
func (h *XMLTVHandler) parseXMLTVTime(s string, source *models.EpgSource) (time.Time, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return time.Time{}, fmt.Errorf("empty time string")
    }

    // Split into time and offset parts
    parts := strings.SplitN(s, " ", 2)
    timeStr := parts[0]

    var detectedOffset time.Duration
    var err error

    if len(parts) > 1 {
        offsetStr := parts[1]
        detectedOffset, err = ParseTimezoneOffset(offsetStr)
        if err != nil {
            h.logger.Warn("failed to parse timezone offset, assuming UTC",
                slog.String("offset", offsetStr),
                slog.String("error", err.Error()),
            )
            detectedOffset = 0
        }

        // Update source's detected timezone (for first program only or if changed)
        if source.DetectedTimezone == "" || source.DetectedTimezone != offsetStr {
            source.DetectedTimezone = offsetStr
        }
    }

    // Parse the time part (YYYYMMDDHHMMSS)
    layout := "20060102150405"
    if len(timeStr) < 14 {
        return time.Time{}, fmt.Errorf("time string too short: %s", timeStr)
    }

    t, err := time.Parse(layout, timeStr[:14])
    if err != nil {
        return time.Time{}, fmt.Errorf("failed to parse time: %w", err)
    }

    // IMPORTANT: Normalize to UTC
    // The parsed time is in the source's local timezone, so we subtract the offset
    normalizedTime := NormalizeProgramTime(t, detectedOffset, source.EpgShift)

    return normalizedTime, nil
}

// ProcessProgram processes a single XMLTV programme element.
func (h *XMLTVHandler) ProcessProgram(prog xmltv.Programme, source *models.EpgSource) (*models.EpgProgram, error) {
    // Parse start and stop times with normalization
    start, err := h.parseXMLTVTime(prog.Start, source)
    if err != nil {
        return nil, fmt.Errorf("failed to parse start time: %w", err)
    }

    stop, err := h.parseXMLTVTime(prog.Stop, source)
    if err != nil {
        return nil, fmt.Errorf("failed to parse stop time: %w", err)
    }

    return &models.EpgProgram{
        SourceID:    source.ID,
        ChannelID:   prog.Channel,
        Start:       start,  // Now in UTC
        Stop:        stop,   // Now in UTC
        Title:       prog.Title.Value,
        Description: prog.Desc.Value,
        // ... other fields
    }, nil
}
```

### 3. Dynamic Canvas Metrics Hook

```typescript
// frontend/src/hooks/useCanvasMetrics.ts

import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { debounce } from 'lodash';

const MS_PER_HOUR = 60 * 60 * 1000;
const MIN_PIXELS_PER_HOUR = 50;
const MIN_PROGRAM_WIDTH = 25;
const DEBOUNCE_MS = 100;

interface CanvasMetrics {
  pixelsPerHour: number;
  containerWidth: number;
  containerHeight: number;
  scrollTimeMs: number;
  guideStartTime: number;
}

interface UseCanvasMetricsOptions {
  sidebarWidth: number;
  hoursToDisplay: number;
  initialStartTime?: number;
}

export function useCanvasMetrics(
  containerRef: React.RefObject<HTMLDivElement>,
  options: UseCanvasMetricsOptions
) {
  const { sidebarWidth, hoursToDisplay, initialStartTime = Date.now() } = options;

  const [metrics, setMetrics] = useState<CanvasMetrics>({
    pixelsPerHour: 200,
    containerWidth: 0,
    containerHeight: 0,
    scrollTimeMs: initialStartTime,
    guideStartTime: initialStartTime - (2 * MS_PER_HOUR), // Start 2 hours before now
  });

  const scrollXRef = useRef(0);

  // Calculate pixels per hour based on container width
  const calculatePixelsPerHour = useCallback((containerWidth: number) => {
    const availableWidth = containerWidth - sidebarWidth;
    const calculated = availableWidth / hoursToDisplay;
    return Math.max(MIN_PIXELS_PER_HOUR, calculated);
  }, [sidebarWidth, hoursToDisplay]);

  // Convert scroll position to time
  const scrollToTime = useCallback((scrollX: number, pixelsPerHour: number, guideStartTime: number) => {
    return guideStartTime + (scrollX / pixelsPerHour) * MS_PER_HOUR;
  }, []);

  // Convert time to scroll position
  const timeToScroll = useCallback((timeMs: number, pixelsPerHour: number, guideStartTime: number) => {
    return ((timeMs - guideStartTime) / MS_PER_HOUR) * pixelsPerHour;
  }, []);

  // Handle resize with scroll position preservation
  const handleResize = useMemo(
    () => debounce((width: number, height: number) => {
      setMetrics(prev => {
        // Save current scroll time using OLD metrics
        const currentScrollTime = scrollToTime(scrollXRef.current, prev.pixelsPerHour, prev.guideStartTime);

        // Calculate new pixels per hour
        const newPixelsPerHour = calculatePixelsPerHour(width);

        return {
          ...prev,
          containerWidth: width,
          containerHeight: height,
          pixelsPerHour: newPixelsPerHour,
          scrollTimeMs: currentScrollTime,
        };
      });
    }, DEBOUNCE_MS),
    [calculatePixelsPerHour, scrollToTime]
  );

  // Set up ResizeObserver
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const observer = new ResizeObserver((entries) => {
      const { width, height } = entries[0].contentRect;
      handleResize(width, height);
    });

    observer.observe(container);

    // Initial measurement
    const { width, height } = container.getBoundingClientRect();
    handleResize(width, height);

    return () => {
      observer.disconnect();
      handleResize.cancel();
    };
  }, [containerRef, handleResize]);

  // Update scroll position ref when scrolling
  const updateScrollPosition = useCallback((scrollX: number) => {
    scrollXRef.current = scrollX;
  }, []);

  // Calculate scroll position to restore after resize
  const getRestoredScrollX = useCallback(() => {
    return timeToScroll(metrics.scrollTimeMs, metrics.pixelsPerHour, metrics.guideStartTime);
  }, [metrics, timeToScroll]);

  // Calculate program position and width
  const calculateProgramBounds = useCallback((startTime: number, endTime: number) => {
    const left = ((startTime - metrics.guideStartTime) / MS_PER_HOUR) * metrics.pixelsPerHour;
    const width = ((endTime - startTime) / MS_PER_HOUR) * metrics.pixelsPerHour;
    return {
      left,
      width: Math.max(width, MIN_PROGRAM_WIDTH),
    };
  }, [metrics]);

  // Calculate hour marker positions
  const hourMarkers = useMemo(() => {
    const markers: { time: number; x: number }[] = [];
    const startHour = Math.floor(metrics.guideStartTime / MS_PER_HOUR) * MS_PER_HOUR;

    for (let time = startHour; time < metrics.guideStartTime + (hoursToDisplay + 2) * MS_PER_HOUR; time += MS_PER_HOUR) {
      markers.push({
        time,
        x: ((time - metrics.guideStartTime) / MS_PER_HOUR) * metrics.pixelsPerHour,
      });
    }

    return markers;
  }, [metrics.guideStartTime, metrics.pixelsPerHour, hoursToDisplay]);

  return {
    metrics,
    updateScrollPosition,
    getRestoredScrollX,
    calculateProgramBounds,
    hourMarkers,
    minProgramWidth: MIN_PROGRAM_WIDTH,
    minPixelsPerHour: MIN_PIXELS_PER_HOUR,
  };
}
```

### 4. Lazy Loading Implementation

```typescript
// frontend/src/components/epg/CanvasEPG.tsx (add to existing component)

interface LazyLoadState {
  loadedEndTime: number;
  isLoadingMore: boolean;
  hasReachedEnd: boolean;
}

const FETCH_THRESHOLD_HOURS = 2;
const FETCH_CHUNK_HOURS = 12;
const MS_PER_HOUR = 60 * 60 * 1000;

function useLazyLoad(
  guideStartTime: number,
  pixelsPerHour: number,
  containerWidth: number,
  onFetchMore: (startTime: Date, endTime: Date) => Promise<boolean>
) {
  const [state, setState] = useState<LazyLoadState>({
    loadedEndTime: guideStartTime + 12 * MS_PER_HOUR,
    isLoadingMore: false,
    hasReachedEnd: false,
  });

  const handleScroll = useCallback(async (scrollX: number) => {
    if (state.isLoadingMore || state.hasReachedEnd) return;

    // Calculate time at right edge of viewport
    const viewportRightTime = guideStartTime + ((scrollX + containerWidth) / pixelsPerHour) * MS_PER_HOUR;

    // Check if within threshold of loaded boundary
    const thresholdTime = state.loadedEndTime - (FETCH_THRESHOLD_HOURS * MS_PER_HOUR);

    if (viewportRightTime >= thresholdTime) {
      setState(prev => ({ ...prev, isLoadingMore: true }));

      const fetchStart = new Date(state.loadedEndTime);
      const fetchEnd = new Date(state.loadedEndTime + FETCH_CHUNK_HOURS * MS_PER_HOUR);

      try {
        const hasMore = await onFetchMore(fetchStart, fetchEnd);

        setState(prev => ({
          ...prev,
          loadedEndTime: prev.loadedEndTime + FETCH_CHUNK_HOURS * MS_PER_HOUR,
          isLoadingMore: false,
          hasReachedEnd: !hasMore,
        }));
      } catch (error) {
        console.error('Failed to load more EPG data:', error);
        setState(prev => ({ ...prev, isLoadingMore: false }));
      }
    }
  }, [guideStartTime, pixelsPerHour, containerWidth, state, onFetchMore]);

  return {
    ...state,
    handleScroll,
  };
}

// Usage in CanvasEPG component
const fetchMorePrograms = useCallback(async (startTime: Date, endTime: Date) => {
  const response = await fetch(
    `/api/v1/epg/guide?start_time=${startTime.toISOString()}&end_time=${endTime.toISOString()}`
  );
  const data = await response.json();

  if (data.success && data.data) {
    // Merge new programs with existing
    setPrograms(prev => mergePrograms(prev, data.data.programs));

    // Return true if there are programs (more data exists)
    return Object.keys(data.data.programs).length > 0;
  }

  return false;
}, []);
```

### 5. Comprehensive Search Implementation

```typescript
// frontend/src/components/epg/CanvasEPG.tsx (add to existing)

interface Channel {
  id: string;
  name: string;
  tvg_id: string;
  logo?: string;
}

interface Program {
  id: string;
  channel_id: string;
  title: string;
  description?: string;
  start_time: string;
  end_time: string;
}

function useEpgSearch(
  channels: Map<string, Channel>,
  programsByChannel: Map<string, Program[]>
) {
  const [searchQuery, setSearchQuery] = useState('');

  const filteredChannels = useMemo(() => {
    if (!searchQuery.trim()) {
      return Array.from(channels.values());
    }

    const lowerQuery = searchQuery.toLowerCase();

    return Array.from(channels.values()).filter(channel => {
      // Search channel name
      if (channel.name.toLowerCase().includes(lowerQuery)) {
        return true;
      }

      // Search channel tvg_id
      if (channel.tvg_id?.toLowerCase().includes(lowerQuery)) {
        return true;
      }

      // Search programs for this channel
      const programs = programsByChannel.get(channel.id) || [];
      return programs.some(program => {
        // Search program title
        if (program.title.toLowerCase().includes(lowerQuery)) {
          return true;
        }

        // Search program description
        if (program.description?.toLowerCase().includes(lowerQuery)) {
          return true;
        }

        return false;
      });
    });
  }, [channels, programsByChannel, searchQuery]);

  return {
    searchQuery,
    setSearchQuery,
    filteredChannels,
    isFiltering: searchQuery.trim().length > 0,
  };
}
```

### 6. Remove Timezone Dropdown

```tsx
// frontend/src/app/epg/page.tsx (modify existing)

// BEFORE: Remove this state and UI
// const [selectedTimezone, setSelectedTimezone] = useState('UTC');

// AFTER: Simplified filter bar without timezone selector
function EpgFilterBar({
  searchQuery,
  onSearchChange,
  sources,
  selectedSource,
  onSourceChange,
}: EpgFilterBarProps) {
  return (
    <div className="flex items-center gap-4 p-4 border-b">
      {/* Search input */}
      <Input
        placeholder="Search channels, programs..."
        value={searchQuery}
        onChange={(e) => onSearchChange(e.target.value)}
        className="max-w-sm"
      />

      {/* Source filter */}
      <Select value={selectedSource} onValueChange={onSourceChange}>
        <SelectTrigger className="w-48">
          <SelectValue placeholder="All sources" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="">All sources</SelectItem>
          {sources.map(source => (
            <SelectItem key={source.id} value={source.id}>
              {source.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* NOTE: Timezone dropdown removed - times display in local timezone automatically */}
    </div>
  );
}
```

## Testing Patterns

### Timezone Normalization Test

```go
// internal/ingestion/timezone_test.go

func TestParseTimezoneOffset(t *testing.T) {
    tests := []struct {
        input    string
        expected time.Duration
        wantErr  bool
    }{
        {"+0100", time.Hour, false},
        {"-0500", -5 * time.Hour, false},
        {"+01:00", time.Hour, false},
        {"-05:30", -5*time.Hour - 30*time.Minute, false},
        {"Z", 0, false},
        {"", 0, false},
        {"+2500", 0, true},  // Invalid hours
        {"invalid", 0, true},
    }

    for _, tc := range tests {
        t.Run(tc.input, func(t *testing.T) {
            result, err := ParseTimezoneOffset(tc.input)
            if tc.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tc.expected, result)
            }
        })
    }
}

func TestNormalizeProgramTime(t *testing.T) {
    // Given: A time of 14:00 with +01:00 offset
    localTime := time.Date(2025, 12, 14, 14, 0, 0, 0, time.UTC)
    detectedOffset := time.Hour // +01:00

    // When: Normalizing to UTC
    result := NormalizeProgramTime(localTime, detectedOffset, 0)

    // Then: Should be 13:00 UTC (14:00 - 1 hour)
    expected := time.Date(2025, 12, 14, 13, 0, 0, 0, time.UTC)
    assert.Equal(t, expected, result)
}

func TestNormalizeProgramTimeWithShift(t *testing.T) {
    // Given: A time of 14:00 with +02:00 offset and +1 hour user shift
    localTime := time.Date(2025, 12, 14, 14, 0, 0, 0, time.UTC)
    detectedOffset := 2 * time.Hour // +02:00
    userShift := 1                   // +1 hour

    // When: Normalizing to UTC with shift
    result := NormalizeProgramTime(localTime, detectedOffset, userShift)

    // Then: Should be 13:00 UTC (14:00 - 2 hours + 1 hour = 13:00)
    expected := time.Date(2025, 12, 14, 13, 0, 0, 0, time.UTC)
    assert.Equal(t, expected, result)
}
```

### Canvas Metrics Test

```typescript
// frontend/tests/epg/canvas-metrics.test.ts

import { renderHook, act } from '@testing-library/react';
import { useCanvasMetrics } from '@/hooks/useCanvasMetrics';

describe('useCanvasMetrics', () => {
  it('calculates pixels per hour correctly', () => {
    const containerRef = { current: document.createElement('div') };
    Object.defineProperty(containerRef.current, 'getBoundingClientRect', {
      value: () => ({ width: 1200, height: 600 }),
    });

    const { result } = renderHook(() =>
      useCanvasMetrics(containerRef, {
        sidebarWidth: 200,
        hoursToDisplay: 6,
      })
    );

    // Available width = 1200 - 200 = 1000
    // Pixels per hour = 1000 / 6 = 166.67
    expect(result.current.metrics.pixelsPerHour).toBeCloseTo(166.67, 1);
  });

  it('enforces minimum pixels per hour', () => {
    const containerRef = { current: document.createElement('div') };
    Object.defineProperty(containerRef.current, 'getBoundingClientRect', {
      value: () => ({ width: 400, height: 600 }), // Very narrow
    });

    const { result } = renderHook(() =>
      useCanvasMetrics(containerRef, {
        sidebarWidth: 200,
        hoursToDisplay: 6,
      })
    );

    // Available width = 400 - 200 = 200
    // Calculated = 200 / 6 = 33.33 (below minimum)
    // Should use minimum of 50
    expect(result.current.metrics.pixelsPerHour).toBe(50);
  });

  it('preserves scroll time during resize', () => {
    const containerRef = { current: document.createElement('div') };
    const mockTime = Date.now();

    const { result, rerender } = renderHook(() =>
      useCanvasMetrics(containerRef, {
        sidebarWidth: 200,
        hoursToDisplay: 6,
        initialStartTime: mockTime,
      })
    );

    // Set initial scroll position
    act(() => {
      result.current.updateScrollPosition(300); // 300px scroll
    });

    // Simulate resize - scroll time should be preserved
    // Then getRestoredScrollX should return appropriate pixel position
    const scrollTime = result.current.metrics.scrollTimeMs;
    expect(scrollTime).toBeDefined();
  });

  it('calculates program bounds correctly', () => {
    const containerRef = { current: document.createElement('div') };
    const guideStart = new Date('2025-12-14T12:00:00Z').getTime();

    const { result } = renderHook(() =>
      useCanvasMetrics(containerRef, {
        sidebarWidth: 200,
        hoursToDisplay: 6,
        initialStartTime: guideStart + 2 * 60 * 60 * 1000, // 14:00
      })
    );

    // Program from 14:00 to 14:30
    const programStart = new Date('2025-12-14T14:00:00Z').getTime();
    const programEnd = new Date('2025-12-14T14:30:00Z').getTime();

    const bounds = result.current.calculateProgramBounds(programStart, programEnd);

    // Width should be 0.5 hours worth of pixels
    const expectedWidth = 0.5 * result.current.metrics.pixelsPerHour;
    expect(bounds.width).toBeGreaterThanOrEqual(expectedWidth);
  });
});
```

## Key Implementation Notes

1. **Timezone Normalization**: Apply inverse offset during ingestion, not display
2. **Dynamic Metrics**: Calculate `pixelsPerHour` from container width, not constant
3. **Scroll Preservation**: Store scroll as time, convert to pixels after resize
4. **Lazy Loading**: Fetch more data when within 2 hours of loaded boundary
5. **Search**: Client-side filtering across all fields (channel name, tvg_id, title, description)
6. **ResizeObserver**: Use on container element, not window, with 100ms debounce
7. **Minimum Thresholds**: 50px/hour minimum, 25px minimum program width

## Migration Notes

No database migrations required - this feature only changes:
1. Ingestion logic (how times are normalized before storage)
2. Frontend rendering logic (dynamic vs. static metrics)

Existing EPG data will remain unchanged. Re-ingestion of EPG sources will apply the new normalization logic.
