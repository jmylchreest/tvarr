import { useState, useCallback, useEffect } from 'react';
import {
  LogColumn,
  LogExplorerStoredConfig,
  LogFilter,
  TimeRange,
  LogSort,
  CONFIG_VERSION,
} from '../types';
import { formatFieldName } from './useFieldExtractor';

/**
 * Default sort configuration.
 */
const DEFAULT_SORT: LogSort = {
  field: 'timestamp',
  direction: 'desc',
};

/**
 * Default time range (show all).
 */
const DEFAULT_TIME_RANGE: TimeRange = {};

/**
 * Generates a localStorage key for a given storage key.
 */
function getStorageKey(storageKey: string): string {
  return `log-explorer-${storageKey}`;
}

/**
 * Loads configuration from localStorage.
 */
function loadConfig(storageKey: string): LogExplorerStoredConfig | null {
  if (typeof window === 'undefined') return null;

  try {
    const stored = localStorage.getItem(getStorageKey(storageKey));
    if (!stored) return null;

    const config = JSON.parse(stored) as LogExplorerStoredConfig;

    // Check version for potential migrations
    if (config.version !== CONFIG_VERSION) {
      // Future: Handle migrations here
      return null;
    }

    return config;
  } catch {
    return null;
  }
}

/**
 * Saves configuration to localStorage.
 */
function saveConfig(storageKey: string, config: LogExplorerStoredConfig): void {
  if (typeof window === 'undefined') return;

  try {
    localStorage.setItem(getStorageKey(storageKey), JSON.stringify(config));
  } catch (error) {
    console.warn('Failed to save log explorer config:', error);
  }
}

/**
 * Creates initial columns from defaults.
 */
function createInitialColumns(
  defaults: Array<Omit<LogColumn, 'order'>>
): LogColumn[] {
  return defaults.map((col, index) => ({
    ...col,
    order: index,
  }));
}

/**
 * Merges stored column config with default columns.
 * Ensures new default columns are added and removed columns are respected.
 */
function mergeColumns(
  stored: LogExplorerStoredConfig['columns'],
  defaults: Array<Omit<LogColumn, 'order'>>
): LogColumn[] {
  const storedMap = new Map(stored.map((col) => [col.id, col]));
  const columns: LogColumn[] = [];

  // First, add columns that exist in both stored and defaults (in stored order)
  const orderedStored = [...stored].sort((a, b) => a.order - b.order);
  for (const storedCol of orderedStored) {
    const defaultCol = defaults.find((d) => d.id === storedCol.id);
    if (defaultCol) {
      columns.push({
        ...defaultCol,
        width: storedCol.width,
        visible: storedCol.visible,
        order: storedCol.order,
        pinned: storedCol.pinned,
      });
    } else {
      // Column was added by user (not in defaults)
      columns.push({
        id: storedCol.id,
        header: formatFieldName(storedCol.id),
        fieldPath: storedCol.id,
        width: storedCol.width,
        minWidth: 60,
        visible: storedCol.visible,
        order: storedCol.order,
        pinned: storedCol.pinned,
      });
    }
  }

  // Add any new default columns that weren't in stored config
  let maxOrder = Math.max(...columns.map((c) => c.order), -1);
  for (const defaultCol of defaults) {
    if (!storedMap.has(defaultCol.id)) {
      maxOrder++;
      columns.push({
        ...defaultCol,
        order: maxOrder,
      });
    }
  }

  return columns.sort((a, b) => a.order - b.order);
}

interface UseLogColumnsOptions {
  storageKey: string;
  defaultColumns: Array<Omit<LogColumn, 'order'>>;
}

interface UseLogColumnsReturn {
  columns: LogColumn[];
  filters: LogFilter[];
  timeRange: TimeRange;
  sort: LogSort;
  sidebarCollapsed: boolean;
  detailCollapsed: boolean;
  addColumn: (fieldPath: string) => void;
  removeColumn: (fieldPath: string) => void;
  reorderColumns: (columnIds: string[]) => void;
  resizeColumn: (columnId: string, width: number) => void;
  toggleColumnVisibility: (columnId: string) => void;
  resetColumns: () => void;
  setFilters: (filters: LogFilter[]) => void;
  setTimeRange: (timeRange: TimeRange) => void;
  setSort: (sort: LogSort) => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  setDetailCollapsed: (collapsed: boolean) => void;
}

/**
 * Hook for managing log explorer columns with localStorage persistence.
 */
export function useLogColumns({
  storageKey,
  defaultColumns,
}: UseLogColumnsOptions): UseLogColumnsReturn {
  // Load initial state from localStorage or defaults
  const [columns, setColumns] = useState<LogColumn[]>(() => {
    const stored = loadConfig(storageKey);
    if (stored?.columns) {
      return mergeColumns(stored.columns, defaultColumns);
    }
    return createInitialColumns(defaultColumns);
  });

  const [filters, setFilters] = useState<LogFilter[]>(() => {
    const stored = loadConfig(storageKey);
    return stored?.filters || [];
  });

  const [timeRange, setTimeRange] = useState<TimeRange>(() => {
    const stored = loadConfig(storageKey);
    return stored?.timeRange || DEFAULT_TIME_RANGE;
  });

  const [sort, setSort] = useState<LogSort>(() => {
    const stored = loadConfig(storageKey);
    return stored?.sort || DEFAULT_SORT;
  });

  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() => {
    const stored = loadConfig(storageKey);
    return stored?.sidebarCollapsed ?? false;
  });

  const [detailCollapsed, setDetailCollapsed] = useState<boolean>(() => {
    const stored = loadConfig(storageKey);
    return stored?.detailCollapsed ?? true;
  });

  // Save to localStorage when state changes
  useEffect(() => {
    const config: LogExplorerStoredConfig = {
      version: CONFIG_VERSION,
      columns: columns.map((col) => ({
        id: col.id,
        width: col.width,
        visible: col.visible,
        order: col.order,
        pinned: col.pinned,
      })),
      filters,
      timeRange,
      sidebarCollapsed,
      detailCollapsed,
      sort,
    };
    saveConfig(storageKey, config);
  }, [columns, filters, timeRange, sort, sidebarCollapsed, detailCollapsed, storageKey]);

  const addColumn = useCallback((fieldPath: string) => {
    setColumns((prev) => {
      // Check if column already exists
      if (prev.some((col) => col.id === fieldPath)) {
        // Make it visible if it exists but is hidden
        return prev.map((col) =>
          col.id === fieldPath ? { ...col, visible: true } : col
        );
      }

      // Add new column at the end
      const maxOrder = Math.max(...prev.map((c) => c.order), -1);
      return [
        ...prev,
        {
          id: fieldPath,
          header: formatFieldName(fieldPath),
          fieldPath,
          width: 150,
          minWidth: 60,
          visible: true,
          order: maxOrder + 1,
        },
      ];
    });
  }, []);

  const removeColumn = useCallback((fieldPath: string) => {
    setColumns((prev) =>
      prev.map((col) =>
        col.id === fieldPath ? { ...col, visible: false } : col
      )
    );
  }, []);

  const reorderColumns = useCallback((columnIds: string[]) => {
    setColumns((prev) => {
      const columnMap = new Map(prev.map((col) => [col.id, col]));
      return columnIds.map((id, index) => {
        const col = columnMap.get(id);
        if (!col) {
          throw new Error(`Column ${id} not found`);
        }
        return { ...col, order: index };
      });
    });
  }, []);

  const resizeColumn = useCallback((columnId: string, width: number) => {
    setColumns((prev) =>
      prev.map((col) =>
        col.id === columnId
          ? { ...col, width: Math.max(width, col.minWidth) }
          : col
      )
    );
  }, []);

  const toggleColumnVisibility = useCallback((columnId: string) => {
    setColumns((prev) =>
      prev.map((col) =>
        col.id === columnId ? { ...col, visible: !col.visible } : col
      )
    );
  }, []);

  const resetColumns = useCallback(() => {
    setColumns(createInitialColumns(defaultColumns));
  }, [defaultColumns]);

  return {
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
  };
}
