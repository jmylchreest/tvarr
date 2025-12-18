import { useMemo, useCallback } from 'react';
import Fuse, { IFuseOptions, FuseResult } from 'fuse.js';
import { LogField } from '../types';

/**
 * Configuration for the Fuse.js search.
 */
interface FuseSearchConfig {
  /** Base keys to always include in search */
  baseKeys: Array<{ name: string; weight: number }>;
  /** Threshold for fuzzy matching (0 = exact, 1 = match anything) */
  threshold?: number;
  /** Minimum number of characters before search starts */
  minMatchCharLength?: number;
  /** Include match information in results */
  includeMatches?: boolean;
  /** Include score in results */
  includeScore?: boolean;
}

/**
 * Default search configuration.
 */
const DEFAULT_CONFIG: FuseSearchConfig = {
  baseKeys: [
    { name: 'message', weight: 0.4 },
    { name: 'level', weight: 0.1 },
    { name: 'target', weight: 0.15 },
    { name: 'source', weight: 0.15 },
  ],
  threshold: 0.3,
  minMatchCharLength: 2,
  includeMatches: true,
  includeScore: true,
};

/**
 * Result of a search operation.
 */
interface SearchResult<T> {
  /** Matching items */
  items: T[];
  /** Total number of matches */
  total: number;
  /** Fuse.js results with match details */
  fuseResults: FuseResult<T>[];
}

/**
 * Hook return type.
 */
interface UseLogSearchReturn<T> {
  /** Perform a search */
  search: (query: string) => SearchResult<T>;
  /** Fuse instance for advanced usage */
  fuse: Fuse<T> | null;
}

/**
 * Builds Fuse.js keys from available fields.
 * Adds dynamically discovered fields to the search configuration.
 */
function buildSearchKeys(
  availableFields: LogField[],
  baseKeys: Array<{ name: string; weight: number }>
): Array<{ name: string; weight: number }> {
  // Start with base keys
  const keys = [...baseKeys];
  const existingPaths = new Set(baseKeys.map((k) => k.name));

  // Add available fields that aren't already in base keys
  for (const field of availableFields) {
    if (!existingPaths.has(field.path)) {
      // Weight based on field type - prioritize strings
      const weight =
        field.type === 'string' ? 0.1 : field.type === 'number' ? 0.05 : 0.02;
      keys.push({ name: field.path, weight });
      existingPaths.add(field.path);
    }
  }

  return keys;
}

/**
 * Hook for fuzzy searching log data using Fuse.js.
 *
 * @param data - Array of records to search
 * @param availableFields - Fields extracted from the data for dynamic key generation
 * @param config - Optional search configuration
 * @returns Search function and Fuse instance
 *
 * @example
 * const { search } = useLogSearch(logs, availableFields);
 * const results = search('error connection');
 */
export function useLogSearch<T extends Record<string, unknown>>(
  data: T[],
  availableFields: LogField[],
  config: Partial<FuseSearchConfig> = {}
): UseLogSearchReturn<T> {
  const mergedConfig = { ...DEFAULT_CONFIG, ...config };

  // Build dynamic search keys
  const searchKeys = useMemo(
    () => buildSearchKeys(availableFields, mergedConfig.baseKeys),
    [availableFields, mergedConfig.baseKeys]
  );

  // Create Fuse instance
  const fuse = useMemo(() => {
    if (data.length === 0) return null;

    const fuseOptions: IFuseOptions<T> = {
      keys: searchKeys,
      threshold: mergedConfig.threshold,
      minMatchCharLength: mergedConfig.minMatchCharLength,
      includeMatches: mergedConfig.includeMatches,
      includeScore: mergedConfig.includeScore,
      // Enable extended search for operators like ^prefix, =exact
      useExtendedSearch: true,
      // Ignore location - match anywhere in the field
      ignoreLocation: true,
      // Sort by score
      shouldSort: true,
    };

    return new Fuse(data, fuseOptions);
  }, [data, searchKeys, mergedConfig]);

  // Search function
  const search = useCallback(
    (query: string): SearchResult<T> => {
      if (!fuse || !query.trim()) {
        return {
          items: data,
          total: data.length,
          fuseResults: [],
        };
      }

      const fuseResults = fuse.search(query);

      return {
        items: fuseResults.map((result) => result.item),
        total: fuseResults.length,
        fuseResults,
      };
    },
    [fuse, data]
  );

  return { search, fuse };
}

/**
 * Highlights matching portions of a string based on Fuse.js match data.
 *
 * @param text - Original text
 * @param indices - Array of [start, end] tuples indicating match positions
 * @returns Array of { text, highlighted } segments
 */
export function highlightMatches(
  text: string,
  indices: readonly [number, number][] | undefined
): Array<{ text: string; highlighted: boolean }> {
  if (!indices || indices.length === 0) {
    return [{ text, highlighted: false }];
  }

  const result: Array<{ text: string; highlighted: boolean }> = [];
  let lastEnd = 0;

  for (const [start, end] of indices) {
    // Add non-highlighted text before match
    if (start > lastEnd) {
      result.push({
        text: text.slice(lastEnd, start),
        highlighted: false,
      });
    }

    // Add highlighted match
    result.push({
      text: text.slice(start, end + 1),
      highlighted: true,
    });

    lastEnd = end + 1;
  }

  // Add remaining text
  if (lastEnd < text.length) {
    result.push({
      text: text.slice(lastEnd),
      highlighted: false,
    });
  }

  return result;
}
