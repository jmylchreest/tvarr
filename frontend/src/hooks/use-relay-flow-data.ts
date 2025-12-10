'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type { RelayFlowGraph } from '@/types/relay-flow';

interface UseRelayFlowDataOptions {
  pollingInterval?: number; // milliseconds, default 2000
  enabled?: boolean;
}

interface UseRelayFlowDataResult {
  data: RelayFlowGraph | null;
  isLoading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

/**
 * Hook for fetching relay session flow graph data.
 * Polls the /api/v1/relay/sessions endpoint which returns a pre-built flow graph
 * ready for React Flow visualization.
 */
export function useRelayFlowData(options: UseRelayFlowDataOptions = {}): UseRelayFlowDataResult {
  const { pollingInterval = 2000, enabled = true } = options;

  const [data, setData] = useState<RelayFlowGraph | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/relay/sessions');
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }
      // Backend returns RelayFlowGraph directly (pre-built with nodes, edges, metadata)
      const flowGraph: RelayFlowGraph = await response.json();
      setData(flowGraph);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err : new Error('Failed to fetch relay sessions'));
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Initial fetch and polling setup
  useEffect(() => {
    if (!enabled) {
      return;
    }

    // Initial fetch
    fetchData();

    // Set up polling
    if (pollingInterval > 0) {
      intervalRef.current = setInterval(fetchData, pollingInterval);
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [enabled, pollingInterval, fetchData]);

  return {
    data,
    isLoading,
    error,
    refetch: fetchData,
  };
}

export default useRelayFlowData;
