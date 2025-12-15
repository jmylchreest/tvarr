import { useState, useEffect, useCallback, useRef, useMemo } from 'react';

/**
 * Canvas metrics for EPG time-to-pixel calculations.
 * These values are dynamically calculated based on container dimensions.
 */
export interface CanvasMetrics {
  /** Container width in pixels */
  containerWidth: number;
  /** Container height in pixels */
  containerHeight: number;

  /** Dynamically calculated pixels per hour */
  pixelsPerHour: number;
  /** Width of the channel sidebar in pixels */
  sidebarWidth: number;
  /** Number of hours visible in the guide */
  hoursToDisplay: number;

  /** Unix timestamp (ms) of the guide start time */
  guideStartTime: number;
  /** Unix timestamp (ms) of the guide end time (loaded data boundary) */
  guideEndTime: number;

  /** Unix timestamp (ms) at current scroll position (time-based scroll preservation) */
  scrollTimeMs: number;

  /** Minimum pixels per hour for readability (below this, horizontal scroll is enabled) */
  minPixelsPerHour: number;
  /** Minimum program width in pixels (for very short programs) */
  minProgramWidth: number;

  /** Whether horizontal scrolling is needed (pixelsPerHour hit minimum threshold) */
  needsHorizontalScroll: boolean;
}

/** Program bounds calculated for hit testing and rendering */
export interface ProgramBounds {
  /** Left position in pixels from guide start */
  left: number;
  /** Width in pixels */
  width: number;
  /** Whether the program is visible in the current viewport */
  isVisible: boolean;
}

/** Default values for canvas metrics */
const DEFAULT_METRICS: CanvasMetrics = {
  containerWidth: 0,
  containerHeight: 0,
  pixelsPerHour: 200, // Initial default, recalculated on mount
  sidebarWidth: 200, // Will be observed
  hoursToDisplay: 6, // Default visible hours
  guideStartTime: Date.now(),
  guideEndTime: Date.now() + 12 * 60 * 60 * 1000,
  scrollTimeMs: Date.now(),
  minPixelsPerHour: 50,
  minProgramWidth: 25,
  needsHorizontalScroll: false,
};

const MS_PER_HOUR = 60 * 60 * 1000;

interface UseCanvasMetricsOptions {
  /** Initial guide start time (timestamp ms) */
  guideStartTime?: number;
  /** Initial guide end time (timestamp ms) */
  guideEndTime?: number;
  /** Number of hours to display */
  hoursToDisplay?: number;
  /** Width of the sidebar */
  sidebarWidth?: number;
  /** Minimum pixels per hour threshold */
  minPixelsPerHour?: number;
  /** Minimum program width */
  minProgramWidth?: number;
  /** Debounce delay for resize events in ms */
  resizeDebounceMs?: number;
}

/**
 * Hook for managing EPG canvas metrics with dynamic time-to-pixel calculations.
 *
 * Features:
 * - Dynamic pixels-per-hour calculation based on container width
 * - Scroll position preservation during resize (stored as time, not pixels)
 * - Minimum readability thresholds with horizontal scrolling fallback
 * - ResizeObserver-based container dimension tracking
 * - Debounced resize handling to prevent excessive re-renders
 *
 * @param containerRef - Ref to the container element to observe
 * @param options - Configuration options
 */
export function useCanvasMetrics(
  containerRef: React.RefObject<HTMLElement>,
  options: UseCanvasMetricsOptions = {}
) {
  const {
    guideStartTime: initialGuideStartTime = Date.now(),
    guideEndTime: initialGuideEndTime = Date.now() + 12 * MS_PER_HOUR,
    hoursToDisplay = 6,
    sidebarWidth = 200,
    minPixelsPerHour = 50,
    minProgramWidth = 25,
    resizeDebounceMs = 100,
  } = options;

  const [metrics, setMetrics] = useState<CanvasMetrics>({
    ...DEFAULT_METRICS,
    guideStartTime: initialGuideStartTime,
    guideEndTime: initialGuideEndTime,
    hoursToDisplay,
    sidebarWidth,
    minPixelsPerHour,
    minProgramWidth,
  });

  const resizeTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const previousScrollTimeRef = useRef<number>(initialGuideStartTime);

  /**
   * Calculate pixels per hour based on available width.
   * Returns the calculated value or minPixelsPerHour if below threshold.
   */
  const calculatePixelsPerHour = useCallback(
    (containerWidth: number): { pixelsPerHour: number; needsHorizontalScroll: boolean } => {
      const availableWidth = containerWidth - sidebarWidth;

      if (availableWidth <= 0) {
        return { pixelsPerHour: minPixelsPerHour, needsHorizontalScroll: true };
      }

      const calculated = availableWidth / hoursToDisplay;

      if (calculated < minPixelsPerHour) {
        return { pixelsPerHour: minPixelsPerHour, needsHorizontalScroll: true };
      }

      return { pixelsPerHour: calculated, needsHorizontalScroll: false };
    },
    [sidebarWidth, hoursToDisplay, minPixelsPerHour]
  );

  /**
   * Calculate program position and width based on current metrics.
   */
  const calculateProgramBounds = useCallback(
    (
      programStartMs: number,
      programEndMs: number,
      currentMetrics?: CanvasMetrics
    ): ProgramBounds => {
      const m = currentMetrics || metrics;

      // Program duration in hours
      const durationMs = programEndMs - programStartMs;
      const durationHours = durationMs / MS_PER_HOUR;

      // Program start offset from guide start in hours
      const startOffsetMs = programStartMs - m.guideStartTime;
      const startOffsetHours = startOffsetMs / MS_PER_HOUR;

      // Calculate pixel positions
      const left = startOffsetHours * m.pixelsPerHour;
      let width = durationHours * m.pixelsPerHour;

      // Enforce minimum width for very short programs
      if (width < m.minProgramWidth) {
        width = m.minProgramWidth;
      }

      // Check if program is visible in guide range
      const isVisible =
        programEndMs > m.guideStartTime && programStartMs < m.guideEndTime;

      return { left, width, isVisible };
    },
    [metrics]
  );

  /**
   * Convert pixel scroll position to time.
   */
  const pixelsToTime = useCallback(
    (scrollX: number, currentMetrics?: CanvasMetrics): number => {
      const m = currentMetrics || metrics;
      const hoursScrolled = scrollX / m.pixelsPerHour;
      return m.guideStartTime + hoursScrolled * MS_PER_HOUR;
    },
    [metrics]
  );

  /**
   * Convert time to pixel scroll position.
   */
  const timeToPixels = useCallback(
    (timeMs: number, currentMetrics?: CanvasMetrics): number => {
      const m = currentMetrics || metrics;
      const msFromStart = timeMs - m.guideStartTime;
      const hoursFromStart = msFromStart / MS_PER_HOUR;
      return hoursFromStart * m.pixelsPerHour;
    },
    [metrics]
  );

  /**
   * Handle container resize with debouncing.
   * Preserves scroll position by storing it as time before resize
   * and converting back to pixels after.
   */
  const handleResize = useCallback(
    (entries: ResizeObserverEntry[]) => {
      // Clear any pending debounce
      if (resizeTimeoutRef.current) {
        clearTimeout(resizeTimeoutRef.current);
      }

      // Debounce the actual metrics update
      resizeTimeoutRef.current = setTimeout(() => {
        const entry = entries[0];
        if (!entry) return;

        const { width, height } = entry.contentRect;

        // Calculate new pixels per hour
        const { pixelsPerHour, needsHorizontalScroll } = calculatePixelsPerHour(width);

        setMetrics((prev) => {
          // Preserve scroll position as time
          const scrollTime = previousScrollTimeRef.current;

          const newMetrics: CanvasMetrics = {
            ...prev,
            containerWidth: width,
            containerHeight: height,
            pixelsPerHour,
            needsHorizontalScroll,
            scrollTimeMs: scrollTime,
          };

          return newMetrics;
        });
      }, resizeDebounceMs);
    },
    [calculatePixelsPerHour, resizeDebounceMs]
  );

  /**
   * Update the stored scroll time (call this from scroll handler).
   */
  const updateScrollTime = useCallback(
    (scrollX: number) => {
      const time = pixelsToTime(scrollX);
      previousScrollTimeRef.current = time;
      setMetrics((prev) => ({
        ...prev,
        scrollTimeMs: time,
      }));
    },
    [pixelsToTime]
  );

  /**
   * Update guide time boundaries (e.g., when lazy loading more data).
   */
  const updateGuideBoundaries = useCallback(
    (startTime: number, endTime: number) => {
      setMetrics((prev) => ({
        ...prev,
        guideStartTime: startTime,
        guideEndTime: endTime,
      }));
    },
    []
  );

  /**
   * Get scroll position in pixels for the current scroll time.
   * Use this after resize to restore scroll position.
   */
  const getScrollPositionForTime = useCallback(
    (timeMs?: number): number => {
      const targetTime = timeMs ?? metrics.scrollTimeMs;
      return timeToPixels(targetTime);
    },
    [metrics.scrollTimeMs, timeToPixels]
  );

  /**
   * Calculate the "now" indicator position in pixels.
   */
  const getNowIndicatorPosition = useCallback((): number => {
    const now = Date.now();
    return timeToPixels(now);
  }, [timeToPixels]);

  // Set up ResizeObserver
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const resizeObserver = new ResizeObserver(handleResize);
    resizeObserver.observe(container);

    // Initial measurement
    const rect = container.getBoundingClientRect();
    const { pixelsPerHour, needsHorizontalScroll } = calculatePixelsPerHour(rect.width);

    setMetrics((prev) => ({
      ...prev,
      containerWidth: rect.width,
      containerHeight: rect.height,
      pixelsPerHour,
      needsHorizontalScroll,
    }));

    return () => {
      resizeObserver.disconnect();
      if (resizeTimeoutRef.current) {
        clearTimeout(resizeTimeoutRef.current);
      }
    };
  }, [containerRef, handleResize, calculatePixelsPerHour]);

  // Memoize the return value to prevent unnecessary re-renders
  return useMemo(
    () => ({
      metrics,
      calculateProgramBounds,
      pixelsToTime,
      timeToPixels,
      updateScrollTime,
      updateGuideBoundaries,
      getScrollPositionForTime,
      getNowIndicatorPosition,
    }),
    [
      metrics,
      calculateProgramBounds,
      pixelsToTime,
      timeToPixels,
      updateScrollTime,
      updateGuideBoundaries,
      getScrollPositionForTime,
      getNowIndicatorPosition,
    ]
  );
}

export default useCanvasMetrics;
