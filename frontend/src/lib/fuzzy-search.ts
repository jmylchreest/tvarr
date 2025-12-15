/**
 * Fuzzy search utility using Fuse.js for typo-tolerant search
 * in channel and EPG browsers.
 */

import Fuse, { IFuseOptions, FuseResult } from 'fuse.js';

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
