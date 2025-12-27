/**
 * Fuzzy search utility using Fuse.js for typo-tolerant search
 * in channel and EPG browsers.
 */

import Fuse, { IFuseOptions, FuseResult, FuseOptionKey } from 'fuse.js';

/**
 * Result of a fuzzy search with match information.
 */
export interface FuzzySearchResult<T> {
  /** The matched item */
  item: T;
  /** Fuzzy match score (0 = perfect match, 1 = no match) */
  score: number;
  /** Which fields matched and their match indices */
  matches: FuzzyMatch[];
}

/**
 * Information about which field matched and where.
 */
export interface FuzzyMatch {
  /** The field name that matched */
  key: string;
  /** The matched value */
  value: string;
  /** Start and end indices of matched characters */
  indices: [number, number][];
}

/**
 * Options for fuzzy search configuration.
 */
export interface FuzzySearchOptions {
  /** Minimum search query length before fuzzy search activates (default: 2) */
  minQueryLength?: number;
  /** Fuse.js threshold - 0 is exact match, 1 matches anything (default: 0.4) */
  threshold?: number;
  /** Distance parameter for fuzzy matching (default: 100) */
  distance?: number;
  /** Maximum results to return (default: all) */
  maxResults?: number;
}

/**
 * Base channel interface for fuzzy search.
 * Any object with at least these fields can be searched.
 */
export interface FuzzySearchChannel {
  id: string;
  name: string;
  tvg_name?: string;
  tvg_id?: string;
  group?: string;
  tvg_chno?: string;
}

/**
 * Base EPG program interface for fuzzy search.
 * Any object with at least these fields can be searched.
 */
export interface FuzzySearchEpgProgram {
  id: string;
  title: string;
  sub_title?: string;
  description?: string;
  channel_id?: string;
  category?: string;
}

/**
 * Default Fuse.js options for channel search.
 */
const DEFAULT_CHANNEL_FUSE_OPTIONS: IFuseOptions<FuzzySearchChannel> = {
  keys: [
    { name: 'name', weight: 0.4 },
    { name: 'tvg_name', weight: 0.2 },
    { name: 'tvg_id', weight: 0.15 },
    { name: 'group', weight: 0.15 },
    { name: 'tvg_chno', weight: 0.05 },
  ],
  threshold: 0.4,
  distance: 100,
  includeScore: true,
  includeMatches: true,
  ignoreLocation: true,
  useExtendedSearch: false,
  minMatchCharLength: 2,
};

/**
 * Default Fuse.js options for EPG search.
 */
const DEFAULT_EPG_FUSE_OPTIONS: IFuseOptions<FuzzySearchEpgProgram> = {
  keys: [
    { name: 'title', weight: 0.4 },
    { name: 'sub_title', weight: 0.2 },
    { name: 'description', weight: 0.2 },
    { name: 'channel_id', weight: 0.1 },
    { name: 'category', weight: 0.1 },
  ],
  threshold: 0.4,
  distance: 100,
  includeScore: true,
  includeMatches: true,
  ignoreLocation: true,
  useExtendedSearch: false,
  minMatchCharLength: 2,
};

/**
 * Creates a Fuse.js instance configured for channel search.
 *
 * @param channels - Array of channels to search through
 * @param options - Optional configuration overrides
 * @returns Configured Fuse instance
 */
export function createChannelFuse<T extends FuzzySearchChannel>(
  channels: T[],
  options?: Partial<IFuseOptions<T>>
): Fuse<T> {
  return new Fuse(channels, {
    ...DEFAULT_CHANNEL_FUSE_OPTIONS,
    ...options,
  } as IFuseOptions<T>);
}

/**
 * Creates a Fuse.js instance configured for EPG search.
 *
 * @param programs - Array of EPG programs to search through
 * @param options - Optional configuration overrides
 * @returns Configured Fuse instance
 */
export function createEpgFuse<T extends FuzzySearchEpgProgram>(
  programs: T[],
  options?: Partial<IFuseOptions<T>>
): Fuse<T> {
  return new Fuse(programs, {
    ...DEFAULT_EPG_FUSE_OPTIONS,
    ...options,
  } as IFuseOptions<T>);
}

/**
 * Performs fuzzy search on channels.
 *
 * @param fuse - Fuse instance to search with
 * @param query - Search query
 * @param options - Optional search options
 * @returns Array of search results with match information
 */
export function fuzzySearchChannels<T extends FuzzySearchChannel>(
  fuse: Fuse<T>,
  query: string,
  options?: FuzzySearchOptions
): FuzzySearchResult<T>[] {
  const minLength = options?.minQueryLength ?? 2;
  const maxResults = options?.maxResults;

  // Don't fuzzy search for very short queries
  if (query.length < minLength) {
    return [];
  }

  const results = fuse.search(query);

  const mapped = results.map((result): FuzzySearchResult<T> => ({
    item: result.item,
    score: result.score ?? 1,
    matches: (result.matches ?? []).map((match) => ({
      key: match.key ?? '',
      value: match.value ?? '',
      indices: (match.indices ?? []) as [number, number][],
    })),
  }));

  return maxResults ? mapped.slice(0, maxResults) : mapped;
}

/**
 * Performs fuzzy search on EPG programs.
 *
 * @param fuse - Fuse instance to search with
 * @param query - Search query
 * @param options - Optional search options
 * @returns Array of search results with match information
 */
export function fuzzySearchEpg<T extends FuzzySearchEpgProgram>(
  fuse: Fuse<T>,
  query: string,
  options?: FuzzySearchOptions
): FuzzySearchResult<T>[] {
  const minLength = options?.minQueryLength ?? 2;
  const maxResults = options?.maxResults;

  // Don't fuzzy search for very short queries
  if (query.length < minLength) {
    return [];
  }

  const results = fuse.search(query);

  const mapped = results.map((result): FuzzySearchResult<T> => ({
    item: result.item,
    score: result.score ?? 1,
    matches: (result.matches ?? []).map((match) => ({
      key: match.key ?? '',
      value: match.value ?? '',
      indices: (match.indices ?? []) as [number, number][],
    })),
  }));

  return maxResults ? mapped.slice(0, maxResults) : mapped;
}

/**
 * Highlights matched portions of text.
 * Returns an array of text segments with isMatch flag.
 *
 * @param text - Original text
 * @param indices - Array of [start, end] match indices
 * @returns Array of text segments
 */
export function highlightMatches(
  text: string,
  indices: [number, number][]
): { text: string; isMatch: boolean }[] {
  if (!indices || indices.length === 0) {
    return [{ text, isMatch: false }];
  }

  const result: { text: string; isMatch: boolean }[] = [];
  let lastIndex = 0;

  // Sort indices by start position
  const sortedIndices = [...indices].sort((a, b) => a[0] - b[0]);

  for (const [start, end] of sortedIndices) {
    // Add non-matched text before this match
    if (start > lastIndex) {
      result.push({
        text: text.slice(lastIndex, start),
        isMatch: false,
      });
    }

    // Add matched text
    result.push({
      text: text.slice(start, end + 1),
      isMatch: true,
    });

    lastIndex = end + 1;
  }

  // Add remaining non-matched text
  if (lastIndex < text.length) {
    result.push({
      text: text.slice(lastIndex),
      isMatch: false,
    });
  }

  return result;
}

/**
 * Gets the primary matched field from a fuzzy search result.
 * Returns the field with the best (lowest) score or most matches.
 *
 * @param matches - Array of matches from fuzzy search
 * @returns The key of the best matching field, or null if no matches
 */
export function getPrimaryMatchField(matches: FuzzyMatch[]): string | null {
  if (!matches || matches.length === 0) {
    return null;
  }

  // Return the field with the most match indices (most characters matched)
  let bestMatch = matches[0];
  let maxIndices = matches[0].indices.length;

  for (const match of matches) {
    if (match.indices.length > maxIndices) {
      maxIndices = match.indices.length;
      bestMatch = match;
    }
  }

  return bestMatch.key;
}

/**
 * Formats match field name for display.
 * Converts snake_case to Title Case.
 *
 * @param field - Field name
 * @returns Formatted field name
 */
export function formatMatchFieldName(field: string): string {
  const fieldNames: Record<string, string> = {
    name: 'Name',
    tvg_name: 'TVG Name',
    tvg_id: 'TVG ID',
    group: 'Group',
    tvg_chno: 'Channel #',
    title: 'Title',
    sub_title: 'Subtitle',
    description: 'Description',
    channel_id: 'Channel',
    category: 'Category',
  };

  return fieldNames[field] || field.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

/**
 * Configuration for creating a fuzzy filter function.
 */
export interface FuzzyFilterConfig<T> {
  /**
   * Fields to search with optional weights.
   * Can be simple field names or weighted key configs.
   * @example ['name', 'description'] or [{ name: 'name', weight: 0.6 }, { name: 'description', weight: 0.4 }]
   */
  keys: (string | { name: string; weight: number })[];
  /**
   * Fuzzy matching threshold (0 = exact, 1 = match anything).
   * @default 0.4
   */
  threshold?: number;
  /**
   * Function to extract the searchable object from an item.
   * Use this when your items are wrapped (e.g., { source: {...} }).
   * @example (item) => item.source
   */
  accessor?: (item: T) => Record<string, unknown>;
}

/**
 * Creates a fuzzy filter function compatible with MasterDetailLayout's filterFn prop.
 * Uses Fuse.js for typo-tolerant searching across multiple fields.
 *
 * @param config - Configuration for the fuzzy filter
 * @returns A filter function that can be passed to MasterDetailLayout's filterFn
 *
 * @example
 * // Simple usage with field names
 * const filterSource = createFuzzyFilter<SourceMasterItem>({
 *   keys: ['name', 'url', 'source_type'],
 *   accessor: (item) => item.source,
 * });
 *
 * @example
 * // With weighted fields
 * const filterProfile = createFuzzyFilter<ProfileItem>({
 *   keys: [
 *     { name: 'name', weight: 0.5 },
 *     { name: 'description', weight: 0.3 },
 *     { name: 'codec', weight: 0.2 },
 *   ],
 *   threshold: 0.3,
 * });
 */
export function createFuzzyFilter<T>(
  config: FuzzyFilterConfig<T>
): (item: T, searchTerm: string) => boolean {
  const { keys, threshold = 0.4, accessor } = config;

  // We'll create the Fuse instance lazily and cache it
  let fuseInstance: Fuse<Record<string, unknown>> | null = null;
  let cachedItems: T[] = [];

  return (item: T, searchTerm: string): boolean => {
    // For very short queries, fall back to simple includes matching
    if (searchTerm.length < 2) {
      const searchable = accessor ? accessor(item) : (item as Record<string, unknown>);
      const lower = searchTerm.toLowerCase();
      return keys.some((key) => {
        const fieldName = typeof key === 'string' ? key : key.name;
        const value = searchable[fieldName];
        return value != null && String(value).toLowerCase().includes(lower);
      });
    }

    // Get the searchable object
    const searchable = accessor ? accessor(item) : (item as Record<string, unknown>);

    // Create a temporary Fuse instance for single-item search
    // This is efficient for small lists used in MasterDetailLayout
    const tempFuse = new Fuse([searchable], {
      keys: keys as FuseOptionKey<Record<string, unknown>>[],
      threshold,
      distance: 100,
      ignoreLocation: true,
      minMatchCharLength: 2,
    });

    const results = tempFuse.search(searchTerm);
    return results.length > 0;
  };
}

/**
 * Creates a memoized fuzzy filter that pre-indexes all items for better performance.
 * Use this when you have the full list of items available upfront.
 *
 * @param items - Array of items to index
 * @param config - Configuration for the fuzzy filter
 * @returns Object with search function and item ID set for quick lookups
 *
 * @example
 * const { matchingIds, search } = createMemoizedFuzzyFilter(allSources, {
 *   keys: ['name', 'url'],
 *   accessor: (item) => item.source,
 *   getId: (item) => item.id,
 * });
 *
 * // Use in filterFn
 * filterFn={(item, term) => matchingIds(term).has(item.id)}
 */
export function createMemoizedFuzzyFilter<T>(
  items: T[],
  config: FuzzyFilterConfig<T> & { getId: (item: T) => string }
): {
  search: (term: string) => T[];
  matchingIds: (term: string) => Set<string>;
} {
  const { keys, threshold = 0.4, accessor, getId } = config;

  // Build searchable array
  const searchableItems = items.map((item) => ({
    _original: item,
    _id: getId(item),
    ...(accessor ? accessor(item) : (item as Record<string, unknown>)),
  }));

  const fuse = new Fuse(searchableItems, {
    keys: keys as FuseOptionKey<typeof searchableItems[0]>[],
    threshold,
    distance: 100,
    ignoreLocation: true,
    minMatchCharLength: 2,
  });

  // Cache for search results
  const cache = new Map<string, Set<string>>();

  return {
    search: (term: string): T[] => {
      if (term.length < 2) {
        // Fall back to simple matching for short queries
        const lower = term.toLowerCase();
        return items.filter((item) => {
          const searchable = accessor ? accessor(item) : (item as Record<string, unknown>);
          return keys.some((key) => {
            const fieldName = typeof key === 'string' ? key : key.name;
            const value = searchable[fieldName];
            return value != null && String(value).toLowerCase().includes(lower);
          });
        });
      }

      const results = fuse.search(term);
      return results.map((r) => r.item._original);
    },
    matchingIds: (term: string): Set<string> => {
      if (cache.has(term)) {
        return cache.get(term)!;
      }

      let ids: Set<string>;
      if (term.length < 2) {
        const lower = term.toLowerCase();
        ids = new Set(
          items
            .filter((item) => {
              const searchable = accessor ? accessor(item) : (item as Record<string, unknown>);
              return keys.some((key) => {
                const fieldName = typeof key === 'string' ? key : key.name;
                const value = searchable[fieldName];
                return value != null && String(value).toLowerCase().includes(lower);
              });
            })
            .map(getId)
        );
      } else {
        const results = fuse.search(term);
        ids = new Set(results.map((r) => r.item._id));
      }

      cache.set(term, ids);
      return ids;
    },
  };
}

/**
 * Creates a fuzzy matcher function that filters an array of items.
 * Returns a function that takes items and a search term, returning filtered results.
 *
 * @param config - Configuration for the fuzzy matcher
 * @returns A function (items, searchTerm) => filteredItems
 *
 * @example
 * const matcher = createFuzzyMatcher<Daemon>({
 *   keys: [{ name: 'name', weight: 0.5 }, { name: 'state', weight: 0.3 }],
 *   accessor: (item) => ({ name: item.name, state: item.state }),
 * });
 * const filtered = matcher(daemons, 'active');
 */
export function createFuzzyMatcher<T>(
  config: FuzzyFilterConfig<T>
): (items: T[], searchTerm: string) => T[] {
  const { keys, threshold = 0.4, accessor } = config;

  return (items: T[], searchTerm: string): T[] => {
    if (!searchTerm || searchTerm.length < 1) {
      return items;
    }

    // For very short queries, use simple substring matching
    if (searchTerm.length < 2) {
      const lower = searchTerm.toLowerCase();
      return items.filter((item) => {
        const searchable = accessor ? accessor(item) : (item as Record<string, unknown>);
        return keys.some((key) => {
          const fieldName = typeof key === 'string' ? key : key.name;
          const value = searchable[fieldName];
          return value != null && String(value).toLowerCase().includes(lower);
        });
      });
    }

    // Build searchable array with original reference
    const searchableItems = items.map((item) => ({
      _original: item,
      ...(accessor ? accessor(item) : (item as Record<string, unknown>)),
    }));

    const fuse = new Fuse(searchableItems, {
      keys: keys as FuseOptionKey<typeof searchableItems[0]>[],
      threshold,
      distance: 100,
      ignoreLocation: true,
      minMatchCharLength: 2,
    });

    const results = fuse.search(searchTerm);
    return results.map((r) => r.item._original);
  };
}
