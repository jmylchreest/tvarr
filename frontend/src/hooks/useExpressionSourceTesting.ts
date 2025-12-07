/**
 * Shared hook for testing expressions against sources.
 * Used by both filter and data-mapping expression editors.
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';
import type { StreamSourceResponse, EpgSourceResponse, ExpressionValidationResponse } from '@/types/api';

export type SourceType = 'stream' | 'epg';

/**
 * Result of testing an expression against a source.
 */
export interface SourceTestResult {
  valid: boolean;
  loading: boolean;
  error?: string;
  // For filter testing
  matchCount?: number;
  totalCount?: number;
  // For data mapping preview
  preview?: {
    success?: boolean;
    message?: string;
    affected_channels?: number;
    total_channels?: number;
  };
}

/**
 * Configuration for the source testing hook.
 */
export interface UseExpressionSourceTestingConfig {
  /** The expression to test */
  expression: string;
  /** Source type (stream or epg) */
  sourceType: SourceType;
  /** Whether to show test results */
  enabled: boolean;
  /** Whether to auto-test when expression becomes valid */
  autoTest: boolean;
  /** The test endpoint to use */
  testEndpoint: 'filter' | 'data-mapping';
  /** Current validation result (for determining when to auto-test) */
  validation: ExpressionValidationResponse | null;
}

/**
 * Hook for testing expressions against sources.
 */
export function useExpressionSourceTesting(config: UseExpressionSourceTestingConfig) {
  const { expression, sourceType, enabled, autoTest, testEndpoint, validation } = config;

  const [allSources, setAllSources] = useState<(StreamSourceResponse | EpgSourceResponse)[]>([]);
  const [sourceResults, setSourceResults] = useState<Map<string, SourceTestResult>>(new Map());

  // Refs to manage debounce and current state
  const testTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const currentValidationRef = useRef<ExpressionValidationResponse | null>(null);
  const currentExpressionRef = useRef<string>('');
  const currentSourcesRef = useRef<(StreamSourceResponse | EpgSourceResponse)[]>([]);

  // Get filtered sources based on current source type
  const sources = allSources.filter((source) => {
    if ('source_kind' in source) {
      return source.source_kind === sourceType;
    }
    // Fallback for sources without source_kind property
    return sourceType === 'stream' ? 'channel_count' in source : 'program_count' in source;
  });

  // Update refs when state changes
  useEffect(() => {
    currentValidationRef.current = validation;
  }, [validation]);

  useEffect(() => {
    currentExpressionRef.current = expression;
  }, [expression]);

  useEffect(() => {
    currentSourcesRef.current = sources;
  }, [sources]);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (testTimeoutRef.current) {
        clearTimeout(testTimeoutRef.current);
      }
    };
  }, []);

  // Fetch all sources once on mount
  useEffect(() => {
    const fetchAllSources = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/sources`);

        if (response.ok) {
          const sourcesData = await response.json();
          if (Array.isArray(sourcesData)) {
            setAllSources(sourcesData.filter((source) => source.enabled));
          }
        }
      } catch (error) {
        console.warn('Failed to fetch sources:', error);
      }
    };

    if (enabled) {
      fetchAllSources();
    }
  }, [enabled]);

  // Test expression against a single source (filter test)
  const testFilterSource = useCallback(
    async (sourceId: string) => {
      if (!expression.trim()) return;

      setSourceResults((prev) =>
        new Map(prev.set(sourceId, { valid: false, loading: true }))
      );

      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/filters/test`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            source_id: sourceId,
            source_type: sourceType,
            filter_expression: expression,
            is_inverse: false,
          }),
        });

        if (response.ok) {
          const data = await response.json();
          setSourceResults((prev) =>
            new Map(prev.set(sourceId, {
              valid: true,
              loading: false,
              matchCount: data.matched_count || 0,
              totalCount: data.total_channels || 0,
            }))
          );
        } else {
          setSourceResults((prev) =>
            new Map(prev.set(sourceId, {
              valid: false,
              loading: false,
              error: 'Test failed',
            }))
          );
        }
      } catch (error) {
        console.warn(`Failed to test source ${sourceId}:`, error);
        setSourceResults((prev) =>
          new Map(prev.set(sourceId, {
            valid: false,
            loading: false,
            error: 'Network error',
          }))
        );
      }
    },
    [expression, sourceType]
  );

  // Preview expression against a single source (data mapping preview)
  const previewDataMappingSource = useCallback(
    async (sourceId: string) => {
      if (!expression.trim()) return;

      setSourceResults((prev) =>
        new Map(prev.set(sourceId, { valid: false, loading: true }))
      );

      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/data-mapping/test`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            source_ids: [sourceId],
            source_type: sourceType,
            expression: expression,
          }),
        });

        if (response.ok) {
          const data = await response.json();
          setSourceResults((prev) =>
            new Map(prev.set(sourceId, {
              valid: true,
              loading: false,
              preview: data,
            }))
          );
        } else {
          setSourceResults((prev) =>
            new Map(prev.set(sourceId, {
              valid: false,
              loading: false,
              error: 'Preview failed',
            }))
          );
        }
      } catch (error) {
        console.warn(`Failed to preview source ${sourceId}:`, error);
        setSourceResults((prev) =>
          new Map(prev.set(sourceId, {
            valid: false,
            loading: false,
            error: 'Network error',
          }))
        );
      }
    },
    [expression, sourceType]
  );

  // Test single source based on endpoint type
  const testSingleSource = useCallback(
    async (sourceId: string) => {
      if (testEndpoint === 'filter') {
        await testFilterSource(sourceId);
      } else {
        await previewDataMappingSource(sourceId);
      }
    },
    [testEndpoint, testFilterSource, previewDataMappingSource]
  );

  // Test all sources
  const testAllSources = useCallback(async () => {
    if (!expression.trim() || sources.length === 0) {
      setSourceResults(new Map());
      return;
    }

    await Promise.all(sources.map((source) => testSingleSource(source.id)));
  }, [sources, testSingleSource, expression]);

  // Handle validation completion - trigger source testing if validation is valid
  const handleValidationComplete = useCallback(() => {
    setTimeout(() => {
      Debug.log('SourceTesting: handleValidationComplete called', {
        autoTest,
        isValid: currentValidationRef.current?.is_valid,
        expression: currentExpressionRef.current?.trim(),
        sourcesLength: currentSourcesRef.current?.length,
      });

      // Clear any existing timeout
      if (testTimeoutRef.current) {
        clearTimeout(testTimeoutRef.current);
        testTimeoutRef.current = null;
      }

      // Always clear source results first to reset badges
      setSourceResults(new Map());

      // Only trigger source testing if conditions are met
      if (
        !autoTest ||
        !currentValidationRef.current?.is_valid ||
        !currentExpressionRef.current.trim() ||
        currentSourcesRef.current.length === 0
      ) {
        return;
      }

      Debug.log('SourceTesting: Scheduling source tests');

      // Debounce source testing
      testTimeoutRef.current = setTimeout(async () => {
        const currentSources = currentSourcesRef.current;
        const currentExpression = currentExpressionRef.current;

        if (!currentExpression.trim() || currentSources.length === 0) {
          setSourceResults(new Map());
          testTimeoutRef.current = null;
          return;
        }

        await Promise.all(currentSources.map((source) => testSingleSource(source.id)));
        testTimeoutRef.current = null;
      }, 200);
    }, 10);
  }, [autoTest, testSingleSource]);

  return {
    sources,
    sourceResults,
    testSingleSource,
    testAllSources,
    handleValidationComplete,
  };
}
