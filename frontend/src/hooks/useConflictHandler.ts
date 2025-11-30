import { useState, useCallback } from 'react';
import { ApiError } from '@/lib/api-client';

interface ConflictState {
  show: boolean;
  message: string;
  resourceId?: string;
}

export function useConflictHandler() {
  const [conflicts, setConflicts] = useState<Map<string, ConflictState>>(new Map());

  const handleApiError = useCallback((error: unknown, resourceId?: string, operation?: string) => {
    if (error instanceof ApiError && error.status === 409) {
      const key = resourceId || 'global';
      const message = operation
        ? `${operation} already in progress. Please wait for it to complete.`
        : 'Operation already in progress. Please wait for it to complete.';

      setConflicts(
        (prev) =>
          new Map(
            prev.set(key, {
              show: true,
              message,
              resourceId,
            })
          )
      );

      return true; // Indicates conflict was handled
    }
    return false; // Let other error handling proceed
  }, []);

  const dismissConflict = useCallback((resourceId?: string) => {
    const key = resourceId || 'global';
    setConflicts((prev) => {
      const newMap = new Map(prev);
      const existing = newMap.get(key);
      if (existing) {
        newMap.set(key, { ...existing, show: false });
        // Clean up after animation completes
        setTimeout(() => {
          setConflicts((current) => {
            const updated = new Map(current);
            updated.delete(key);
            return updated;
          });
        }, 200);
      }
      return newMap;
    });
  }, []);

  const getConflictState = useCallback(
    (resourceId?: string): ConflictState => {
      const key = resourceId || 'global';
      return conflicts.get(key) || { show: false, message: '' };
    },
    [conflicts]
  );

  const hasActiveConflicts = conflicts.size > 0;

  return {
    handleApiError,
    dismissConflict,
    getConflictState,
    hasActiveConflicts,
  };
}
