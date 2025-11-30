'use client';

import { useMemo } from 'react';
import { useProgressEvents } from './useProgressEvents';
import { ProgressEvent } from '@/providers/ProgressProvider';

export interface ProgressState {
  // Basic state
  isActive: boolean;
  isProcessing: boolean;
  isCompleted: boolean;
  isFailed: boolean;
  hasError: boolean;

  // Progress information
  progress: {
    overall_percentage: number;
    current_stage: string;
    stages: Array<{
      id: string;
      name: string;
      percentage: number;
      state:
        | 'idle'
        | 'processing'
        | 'completed'
        | 'error'
        | 'preparing'
        | 'connecting'
        | 'downloading'
        | 'saving'
        | 'cleanup'
        | 'cancelled';
      stage_step: string | null;
    }>;
  } | null;

  // Stage information (for backward compatibility)
  stages: {
    currentStage: string | null;
    currentStageName: string | null;
    stageProgressPercentage: number | null;
    totalStages: number | null;
    completedStages: number | null;
    stageMetadata: Record<string, any>;
  } | null;

  // Timing information
  duration: string;
  durationMs: number;
  startedAt: Date | null;

  // Current state
  currentStep: string | null;
  operationName: string | null;
  error: string | null;

  // Raw event
  event: ProgressEvent | null;
}

/**
 * Derive useful state information from progress events
 * @param resourceId - The resource ID to get state for
 * @returns Processed state information
 */
export function useProgressState(resourceId: string): ProgressState {
  const event = useProgressEvents(resourceId);

  return useMemo(() => {
    if (!event) {
      return {
        isActive: false,
        isProcessing: false,
        isCompleted: false,
        isFailed: false,
        hasError: false,
        progress: null,
        stages: null,
        duration: '0s',
        durationMs: 0,
        startedAt: null,
        currentStep: null,
        operationName: null,
        error: null,
        event: null,
      };
    }

    // Consider all non-terminal states as active (idle, processing, connecting, downloading, saving, cleanup)
    // Terminal states are: completed, error, cancelled, failed
    const isActive = !['completed', 'error', 'cancelled', 'failed'].includes(event.state);
    const isProcessing = event.state === 'processing';
    const isCompleted = event.state === 'completed';
    const isFailed = event.state === 'error';
    const hasError = !!event.error || isFailed;

    // Format progress information using new structure
    const progress = {
      overall_percentage: event.overall_percentage,
      current_stage: event.current_stage,
      stages: event.stages,
    };

    // Format stage information for backward compatibility
    const currentStageData = event.stages.find((s) => s.id === event.current_stage);
    const completedStages = event.stages.filter((s) => s.state === 'completed').length;

    const stages = {
      currentStage: event.current_stage,
      currentStageName: currentStageData?.name || null,
      stageProgressPercentage: currentStageData?.percentage || null,
      totalStages: event.stages.length,
      completedStages,
      stageMetadata: {},
    };

    // Calculate duration from timestamps
    const startTime = new Date(event.started_at).getTime();
    const updateTime = new Date(event.last_update).getTime();
    const durationMs = updateTime - startTime;
    const duration = formatDuration(durationMs);

    // Parse start time
    const startedAt = new Date(event.started_at);

    return {
      isActive,
      isProcessing,
      isCompleted,
      isFailed,
      hasError,
      progress,
      stages,
      duration,
      durationMs,
      startedAt,
      currentStep: currentStageData?.stage_step || null,
      operationName: event.operation_name || null,
      error: event.error,
      event,
    };
  }, [event]);
}

/**
 * Format duration from milliseconds to human readable string
 */
function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }

  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    const remainingMinutes = minutes % 60;
    const remainingSeconds = seconds % 60;
    return `${hours}h ${remainingMinutes}m ${remainingSeconds}s`;
  } else if (minutes > 0) {
    const remainingSeconds = seconds % 60;
    return `${minutes}m ${remainingSeconds}s`;
  } else {
    return `${seconds}s`;
  }
}

/**
 * Format progress as a human-readable string
 */
export function formatProgress(progress: ProgressState['progress']): string {
  if (!progress) return '';

  const parts: string[] = [];

  // Overall percentage
  parts.push(`${progress.overall_percentage.toFixed(1)}%`);

  // Current stage name and step
  const currentStageData = progress.stages.find((s) => s.id === progress.current_stage);
  if (currentStageData) {
    parts.push(`${currentStageData.name}: ${currentStageData.stage_step}`);
  }

  return parts.join(' - ');
}
