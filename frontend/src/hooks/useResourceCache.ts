import { useCallback, useRef } from 'react';
import { Debug } from '@/utils/debug';

const debug = Debug.createLogger('ResourceCache');

interface CacheEntry {
  data: any;
  timestamp: number;
  loading: Promise<any> | null;
}

export function useResourceCache() {
  const cacheRef = useRef<Map<string, CacheEntry>>(new Map());
  const CACHE_DURATION = 60000; // 1 minute

  const getCachedOrFetch = useCallback(
    async <T>(key: string, fetchFn: () => Promise<T>): Promise<T> => {
      const now = Date.now();
      const cached = cacheRef.current.get(key);

      // Return cached if still valid
      if (cached && now - cached.timestamp < CACHE_DURATION) {
        debug.log(`Cache hit for: ${key}`);
        return cached.data as T;
      }

      // Return in-flight request if exists
      if (cached?.loading) {
        debug.log(`Cache loading for: ${key}`);
        return cached.loading as Promise<T>;
      }

      // Fetch new data
      debug.log(`Cache miss, fetching: ${key}`);
      const loadingPromise = fetchFn();

      // Store loading promise
      cacheRef.current.set(key, {
        data: null,
        timestamp: now,
        loading: loadingPromise,
      });

      try {
        const result = await loadingPromise;

        // Update cache with result
        cacheRef.current.set(key, {
          data: result,
          timestamp: now,
          loading: null,
        });

        return result;
      } catch (error) {
        // Remove failed entry
        cacheRef.current.delete(key);
        throw error;
      }
    },
    []
  );

  const invalidateCache = useCallback((key?: string) => {
    if (key) {
      cacheRef.current.delete(key);
      debug.log(`Cache invalidated for: ${key}`);
    } else {
      cacheRef.current.clear();
      debug.log('Cache cleared');
    }
  }, []);

  return {
    getCachedOrFetch,
    invalidateCache,
  };
}
