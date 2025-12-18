'use client';

import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
  ReactNode,
} from 'react';
import {
  LogColumn,
  LogField,
  LogFilter,
  LogSort,
  TimeRange,
  LogExplorerContextValue,
  TIME_RANGE_PRESETS,
} from './types';
import { useLogColumns } from './hooks/useLogColumns';
import { useFieldExtractor, getNestedValue } from './hooks/useFieldExtractor';

/**
 * Props for the LogExplorerProvider.
 */
interface LogExplorerProviderProps<T extends { id: string; timestamp: string }> {
  children: ReactNode;
  data: T[];
  storageKey: string;
  defaultColumns: Array<Omit<LogColumn, 'order'>>;
  excludeFields?: string[];
  cellRenderers?: Record<string, (value: unknown, record: T) => ReactNode>;
}

// Create the context with undefined default value
const LogExplorerContext = createContext<LogExplorerContextValue<any> | undefined>(undefined);

/**
 * Applies filters to a single record.
 */
function matchesFilter<T extends Record<string, unknown>>(
  record: T,
  filter: LogFilter
): boolean {
  const value = getNestedValue(record, filter.field);
  const stringValue = value === null || value === undefined ? '' : String(value);
  const filterValue = filter.value === undefined ? '' : String(filter.value);

  switch (filter.operator) {
    case 'equals':
      return stringValue === filterValue;
    case 'not_equals':
      return stringValue !== filterValue;
    case 'contains':
      return stringValue.toLowerCase().includes(filterValue.toLowerCase());
    case 'not_contains':
      return !stringValue.toLowerCase().includes(filterValue.toLowerCase());
    case 'exists':
      return value !== null && value !== undefined;
    case 'not_exists':
      return value === null || value === undefined;
    case 'gt':
      return Number(value) > Number(filter.value);
    case 'lt':
      return Number(value) < Number(filter.value);
    case 'gte':
      return Number(value) >= Number(filter.value);
    case 'lte':
      return Number(value) <= Number(filter.value);
    default:
      return true;
  }
}

/**
 * Checks if a record falls within the specified time range.
 */
function matchesTimeRange<T extends { timestamp: string }>(
  record: T,
  timeRange: TimeRange
): boolean {
  // If no time range specified, match all
  if (!timeRange.preset && !timeRange.from && !timeRange.to) {
    return true;
  }

  const recordTime = new Date(record.timestamp).getTime();

  // Handle preset time ranges
  if (timeRange.preset) {
    const preset = TIME_RANGE_PRESETS.find((p) => p.value === timeRange.preset);
    if (preset) {
      const { from, to } = preset.getRange();
      return recordTime >= from.getTime() && recordTime <= to.getTime();
    }
  }

  // Handle custom time ranges
  if (timeRange.from && recordTime < new Date(timeRange.from).getTime()) {
    return false;
  }
  if (timeRange.to && recordTime > new Date(timeRange.to).getTime()) {
    return false;
  }

  return true;
}

/**
 * Provider component for LogExplorer state management.
 */
export function LogExplorerProvider<T extends { id: string; timestamp: string }>({
  children,
  data,
  storageKey,
  defaultColumns,
  excludeFields = [],
  cellRenderers = {},
}: LogExplorerProviderProps<T>) {
  // Column management with localStorage persistence
  const {
    columns,
    filters,
    timeRange,
    sort,
    sidebarCollapsed,
    detailCollapsed,
    addColumn,
    removeColumn,
    reorderColumns,
    resizeColumn,
    toggleColumnVisibility,
    resetColumns,
    setFilters,
    setTimeRange,
    setSort,
    setSidebarCollapsed,
    setDetailCollapsed,
  } = useLogColumns({ storageKey, defaultColumns });

  // Extract available fields from data
  const availableFields = useFieldExtractor(data, excludeFields);

  // Search state
  const [searchQuery, setSearchQuery] = useState('');

  // Selection state
  const [selectedRecordId, setSelectedRecordId] = useState<string | null>(null);

  // Generate unique filter ID
  const generateFilterId = useCallback(() => {
    return `filter-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
  }, []);

  // Filter management
  const addFilter = useCallback(
    (filter: Omit<LogFilter, 'id'>) => {
      const newFilter: LogFilter = {
        ...filter,
        id: generateFilterId(),
      };
      setFilters([...filters, newFilter]);
    },
    [filters, setFilters, generateFilterId]
  );

  const removeFilter = useCallback(
    (filterId: string) => {
      setFilters(filters.filter((f) => f.id !== filterId));
    },
    [filters, setFilters]
  );

  const toggleFilter = useCallback(
    (filterId: string) => {
      // For now, toggling removes the filter. Could add enabled state later.
      removeFilter(filterId);
    },
    [removeFilter]
  );

  const clearFilters = useCallback(() => {
    setFilters([]);
  }, [setFilters]);

  // Apply filters and time range to get filtered data
  const filteredData = useMemo(() => {
    let result = data;

    // Apply time range filter
    result = result.filter((record) => matchesTimeRange(record, timeRange));

    // Apply field filters
    for (const filter of filters) {
      result = result.filter((record) =>
        matchesFilter(record as unknown as Record<string, unknown>, filter)
      );
    }

    // Apply sorting
    if (sort.field) {
      result = [...result].sort((a, b) => {
        const aValue = getNestedValue(
          a as unknown as Record<string, unknown>,
          sort.field
        );
        const bValue = getNestedValue(
          b as unknown as Record<string, unknown>,
          sort.field
        );

        // Handle undefined/null values
        if (aValue === undefined || aValue === null) return sort.direction === 'asc' ? -1 : 1;
        if (bValue === undefined || bValue === null) return sort.direction === 'asc' ? 1 : -1;

        // Compare values
        let comparison = 0;
        if (typeof aValue === 'number' && typeof bValue === 'number') {
          comparison = aValue - bValue;
        } else if (typeof aValue === 'string' && typeof bValue === 'string') {
          comparison = aValue.localeCompare(bValue);
        } else {
          comparison = String(aValue).localeCompare(String(bValue));
        }

        return sort.direction === 'asc' ? comparison : -comparison;
      });
    }

    return result;
  }, [data, filters, timeRange, sort]);

  // Selection helpers
  const selectRecord = useCallback((recordId: string | null) => {
    setSelectedRecordId(recordId);
    // Auto-expand detail panel when selecting a record
    if (recordId) {
      setDetailCollapsed(false);
    }
  }, [setDetailCollapsed]);

  const selectedRecord = useMemo(() => {
    if (!selectedRecordId) return null;
    return data.find((record) => record.id === selectedRecordId) || null;
  }, [data, selectedRecordId]);

  // Panel toggles
  const toggleSidebar = useCallback(() => {
    setSidebarCollapsed(!sidebarCollapsed);
  }, [sidebarCollapsed, setSidebarCollapsed]);

  const toggleDetailPanel = useCallback(() => {
    setDetailCollapsed(!detailCollapsed);
  }, [detailCollapsed, setDetailCollapsed]);

  // Build context value
  const contextValue: LogExplorerContextValue<T> = useMemo(
    () => ({
      // Data
      data,
      filteredData,
      availableFields,

      // Columns
      columns,
      addColumn,
      removeColumn,
      reorderColumns,
      resizeColumn,
      toggleColumnVisibility,
      resetColumns,

      // Filters
      filters,
      addFilter,
      removeFilter,
      toggleFilter,
      clearFilters,

      // Time range
      timeRange,
      setTimeRange,

      // Search
      searchQuery,
      setSearchQuery,

      // Selection
      selectedRecordId,
      selectedRecord,
      selectRecord,

      // Sort
      sort,
      setSort,

      // Panel state
      sidebarCollapsed,
      detailCollapsed,
      toggleSidebar,
      toggleDetailPanel,

      // Cell renderers
      cellRenderers,
    }),
    [
      data,
      filteredData,
      availableFields,
      columns,
      addColumn,
      removeColumn,
      reorderColumns,
      resizeColumn,
      toggleColumnVisibility,
      resetColumns,
      filters,
      addFilter,
      removeFilter,
      toggleFilter,
      clearFilters,
      timeRange,
      setTimeRange,
      searchQuery,
      setSearchQuery,
      selectedRecordId,
      selectedRecord,
      selectRecord,
      sort,
      setSort,
      sidebarCollapsed,
      detailCollapsed,
      toggleSidebar,
      toggleDetailPanel,
      cellRenderers,
    ]
  );

  return (
    <LogExplorerContext.Provider value={contextValue}>
      {children}
    </LogExplorerContext.Provider>
  );
}

/**
 * Hook to access LogExplorer context.
 * Must be used within a LogExplorerProvider.
 */
export function useLogExplorer<T extends { id: string; timestamp: string }>(): LogExplorerContextValue<T> {
  const context = useContext(LogExplorerContext);
  if (context === undefined) {
    throw new Error('useLogExplorer must be used within a LogExplorerProvider');
  }
  return context as LogExplorerContextValue<T>;
}
