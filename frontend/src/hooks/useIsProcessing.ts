'use client';

import { useMemo } from 'react';
import { useProgressContext } from '@/providers/ProgressProvider';

/**
 * Hook to check if a resource is currently processing.
 * Returns true when the resource has an active operation in progress.
 * Useful for triggering animations or showing loading states.
 *
 * @param resourceId The ID of the resource to check
 * @returns boolean indicating if the resource is processing
 */
export function useIsProcessing(resourceId: string): boolean {
  const progressContext = useProgressContext();

  return useMemo(() => {
    const state = progressContext.getResourceState(resourceId);
    if (!state) return false;

    const activeStates = ['processing', 'preparing', 'connecting', 'downloading', 'saving', 'cleanup'];
    return activeStates.includes(state.state);
  }, [progressContext, resourceId]);
}
