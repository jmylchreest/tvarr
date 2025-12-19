'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Debug } from '@/utils/debug';

export interface UseAutoRefreshOptions {
  /** Callback triggered on each refresh interval */
  onRefresh: () => void | Promise<void>;
  /** Available interval values in seconds. First value (typically 0) disables auto-refresh */
  stepValues?: number[];
  /** Initial interval index (default: 3, typically 10s with default stepValues) */
  defaultStepIndex?: number;
  /** Whether to start auto-refresh immediately (default: true) */
  startEnabled?: boolean;
  /** Unique identifier for debug logging */
  debugLabel?: string;
  /** Optional localStorage key to persist the refresh interval. If provided, interval is saved/restored */
  storageKey?: string;
}

export interface UseAutoRefreshReturn {
  /** Current refresh interval in seconds (0 = disabled) */
  refreshInterval: number;
  /** Whether auto-refresh is currently active */
  isAutoRefresh: boolean;
  /** Available step values */
  stepValues: number[];
  /** Current step index */
  stepIndex: number;
  /** Toggle auto-refresh on/off */
  toggleAutoRefresh: () => void;
  /** Start auto-refresh */
  startAutoRefresh: () => void;
  /** Stop auto-refresh */
  stopAutoRefresh: () => void;
  /** Trigger a manual refresh */
  manualRefresh: () => void;
  /** Handle slider value change */
  handleIntervalChange: (value: number[]) => void;
  /** Get display label for current interval */
  getIntervalLabel: () => string;
}

const DEFAULT_STEP_VALUES = [0, 1, 5, 10, 15, 30, 60];

function getStoredInterval(storageKey: string | undefined, stepValues: number[], defaultStepIndex: number): number {
  if (!storageKey || typeof window === 'undefined') {
    return stepValues[defaultStepIndex];
  }

  try {
    const stored = localStorage.getItem(`refresh-interval-${storageKey}`);
    if (stored) {
      const parsed = parseInt(stored, 10);
      // Validate that the stored value is in our step values
      if (stepValues.includes(parsed)) {
        return parsed;
      }
    }
  } catch {
    // localStorage might not be available
  }

  return stepValues[defaultStepIndex];
}

export function useAutoRefresh({
  onRefresh,
  stepValues = DEFAULT_STEP_VALUES,
  defaultStepIndex = 3,
  startEnabled = true,
  debugLabel = 'auto-refresh',
  storageKey,
}: UseAutoRefreshOptions): UseAutoRefreshReturn {
  const [refreshInterval, setRefreshInterval] = useState(() =>
    getStoredInterval(storageKey, stepValues, defaultStepIndex)
  );
  const [isAutoRefresh, setIsAutoRefresh] = useState(() => {
    const initialInterval = getStoredInterval(storageKey, stepValues, defaultStepIndex);
    return startEnabled && initialInterval !== 0;
  });
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const onRefreshRef = useRef(onRefresh);
  const prevIntervalRef = useRef(refreshInterval);

  // Keep onRefresh ref up to date without causing re-renders
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  }, [onRefresh]);

  // Persist to localStorage when interval changes
  useEffect(() => {
    if (storageKey && typeof window !== 'undefined') {
      try {
        localStorage.setItem(`refresh-interval-${storageKey}`, String(refreshInterval));
        Debug.log(`[${debugLabel}] Saved refresh interval to localStorage:`, refreshInterval);
      } catch {
        // localStorage might not be available
      }
    }
  }, [refreshInterval, storageKey, debugLabel]);

  const clearTimer = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  const startAutoRefresh = useCallback(() => {
    clearTimer();

    if (refreshInterval === 0) {
      setIsAutoRefresh(false);
      return;
    }

    Debug.log(`[${debugLabel}] Starting auto-refresh with interval:`, refreshInterval, 'seconds');
    intervalRef.current = setInterval(() => {
      onRefreshRef.current();
    }, refreshInterval * 1000);

    setIsAutoRefresh(true);
  }, [refreshInterval, clearTimer, debugLabel]);

  const stopAutoRefresh = useCallback(() => {
    clearTimer();
    setIsAutoRefresh(false);
    Debug.log(`[${debugLabel}] Stopped auto-refresh`);
  }, [clearTimer, debugLabel]);

  const toggleAutoRefresh = useCallback(() => {
    if (isAutoRefresh) {
      stopAutoRefresh();
    } else {
      startAutoRefresh();
    }
  }, [isAutoRefresh, startAutoRefresh, stopAutoRefresh]);

  const manualRefresh = useCallback(() => {
    onRefreshRef.current();
  }, []);

  const handleIntervalChange = useCallback((value: number[]) => {
    const newInterval = stepValues[value[0]];
    setRefreshInterval(newInterval);
  }, [stepValues]);

  const getIntervalLabel = useCallback(() => {
    if (refreshInterval === 0) return 'Off';
    return `${refreshInterval}s`;
  }, [refreshInterval]);

  // Handle interval changes - only restart if interval actually changed
  useEffect(() => {
    const intervalChanged = prevIntervalRef.current !== refreshInterval;
    prevIntervalRef.current = refreshInterval;

    if (!intervalChanged) return;

    if (refreshInterval === 0) {
      stopAutoRefresh();
    } else if (isAutoRefresh) {
      // Only restart if already running
      startAutoRefresh();
    }
  }, [refreshInterval, isAutoRefresh, startAutoRefresh, stopAutoRefresh]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      clearTimer();
    };
  }, [clearTimer]);

  // Start auto-refresh on mount if enabled
  useEffect(() => {
    if (startEnabled && refreshInterval > 0) {
      startAutoRefresh();
    }
    // Only run on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    refreshInterval,
    isAutoRefresh,
    stepValues,
    stepIndex: stepValues.indexOf(refreshInterval),
    toggleAutoRefresh,
    startAutoRefresh,
    stopAutoRefresh,
    manualRefresh,
    handleIntervalChange,
    getIntervalLabel,
  };
}
