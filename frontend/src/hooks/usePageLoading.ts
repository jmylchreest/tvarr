'use client';

import { useEffect } from 'react';
import { usePageLoading as usePageLoadingContext } from '@/providers/PageLoadingProvider';

/**
 * Hook for managing page loading state
 *
 * @param isLoading - Current loading state of the page
 * @param delay - Delay before showing/hiding spinner (default: 100ms)
 */
export function usePageLoading(isLoading: boolean = false, delay: number = 100) {
  const { setIsLoading } = usePageLoadingContext();

  useEffect(() => {
    let timeoutId: NodeJS.Timeout;

    if (isLoading) {
      // Show spinner immediately or with small delay
      timeoutId = setTimeout(() => {
        setIsLoading(true);
      }, delay);
    } else {
      // Hide spinner immediately
      setIsLoading(false);
    }

    return () => {
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, [isLoading, delay, setIsLoading]);
}

/**
 * Hook for async operations with loading state
 *
 * @param asyncFn - Async function to execute
 * @param deps - Dependencies for the async function
 */
export function useAsyncLoading<T>(asyncFn: () => Promise<T>, deps: React.DependencyList = []) {
  const { setIsLoading } = usePageLoadingContext();

  useEffect(() => {
    let isCancelled = false;

    const executeAsync = async () => {
      setIsLoading(true);
      try {
        await asyncFn();
      } catch (error) {
        console.error('Async operation failed:', error);
      } finally {
        if (!isCancelled) {
          setIsLoading(false);
        }
      }
    };

    executeAsync();

    return () => {
      isCancelled = true;
      setIsLoading(false);
    };
  }, deps);
}

/**
 * Manual loading control hook
 */
export function useManualLoading() {
  const context = usePageLoadingContext();
  return context;
}
