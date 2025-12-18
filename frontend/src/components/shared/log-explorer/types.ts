import { ReactNode } from 'react';

/**
 * Represents a flattened field extracted from log entries.
 * Fields like `fields.channel_name` are flattened to a single path.
 */
export interface LogField {
  /** Full path to the field (e.g., "fields.channel_name") */
  path: string;
  /** Display name for UI (e.g., "Channel Name") */
  displayName: string;
  /** Field type for formatting */
  type: 'string' | 'number' | 'boolean' | 'datetime' | 'object' | 'array';
  /** Count of records containing this field */
  count: number;
  /** Sample values for preview */
  sampleValues?: (string | number | boolean)[];
}

/**
 * Column configuration for the log table.
 */
export interface LogColumn {
  /** Unique identifier - the field path */
  id: string;
  /** Display header text */
  header: string;
  /** Field path to access data */
  fieldPath: string;
  /** Column width in pixels */
  width: number;
  /** Minimum width in pixels */
  minWidth: number;
  /** Whether column is visible */
  visible: boolean;
  /** Display order (lower = left) */
  order: number;
  /** Whether column is pinned to left */
  pinned?: boolean;
}

/**
 * Filter operators for log filtering.
 */
export type LogFilterOperator =
  | 'equals'
  | 'not_equals'
  | 'contains'
  | 'not_contains'
  | 'exists'
  | 'not_exists'
  | 'gt'
  | 'lt'
  | 'gte'
  | 'lte';

/**
 * Filter for log entries.
 */
export interface LogFilter {
  /** Unique filter ID */
  id: string;
  /** Field path being filtered */
  field: string;
  /** Field display name */
  fieldDisplayName: string;
  /** Filter operator */
  operator: LogFilterOperator;
  /** Filter value */
  value?: string | number | boolean;
}

/**
 * Quick time range preset.
 */
export interface TimeRangePreset {
  label: string;
  value: string;
  getRange: () => { from: Date; to: Date };
}

/**
 * Time range for log filtering.
 */
export interface TimeRange {
  /** Quick range preset value (e.g., "15m", "1h", "24h") */
  preset?: string;
  /** Start time for custom range */
  from?: Date;
  /** End time for custom range */
  to?: Date;
}

/**
 * Sort configuration.
 */
export interface LogSort {
  field: string;
  direction: 'asc' | 'desc';
}

/**
 * LogExplorer configuration persisted to localStorage.
 */
export interface LogExplorerStoredConfig {
  /** Config version for migrations */
  version: number;
  /** Column configurations (subset of LogColumn for storage) */
  columns: Array<{
    id: string;
    width: number;
    visible: boolean;
    order: number;
    pinned?: boolean;
  }>;
  /** Active filters */
  filters: LogFilter[];
  /** Time range */
  timeRange: TimeRange;
  /** Fields sidebar collapsed state */
  sidebarCollapsed: boolean;
  /** Detail panel collapsed state */
  detailCollapsed: boolean;
  /** Sort configuration */
  sort?: LogSort;
}

/**
 * Props for custom cell renderers.
 */
export interface CellRendererProps<T> {
  value: unknown;
  record: T;
  field: string;
}

/**
 * Props for the LogExplorer component.
 */
export interface LogExplorerProps<T extends { id: string; timestamp: string }> {
  /** Array of log entries to display */
  data: T[];
  /** Unique storage key for localStorage persistence */
  storageKey: string;
  /** Loading state */
  isLoading?: boolean;
  /** Default columns to show when no config exists */
  defaultColumns: Array<Omit<LogColumn, 'order'>>;
  /** Fields to exclude from available fields list */
  excludeFields?: string[];
  /** Custom cell renderer for specific fields */
  cellRenderers?: Record<string, (value: unknown, record: T) => ReactNode>;
  /** Callback when data should be refreshed */
  onRefresh?: () => void;
  /** Whether to show the fields sidebar */
  showFieldsSidebar?: boolean;
  /** Whether to show the detail panel */
  showDetailPanel?: boolean;
  /** Custom title for the component */
  title?: string;
  /** Custom empty state */
  emptyState?: {
    title: string;
    description?: string;
  };
  /** Controls section (pause/resume, connect/disconnect buttons) */
  controls?: ReactNode;
  /** Statistics cards to display above the explorer */
  statsCards?: ReactNode;
}

/**
 * Context value for LogExplorer state management.
 */
export interface LogExplorerContextValue<T extends { id: string; timestamp: string }> {
  // Data
  data: T[];
  filteredData: T[];
  availableFields: LogField[];

  // Columns
  columns: LogColumn[];
  addColumn: (fieldPath: string) => void;
  removeColumn: (fieldPath: string) => void;
  reorderColumns: (columnIds: string[]) => void;
  resizeColumn: (columnId: string, width: number) => void;
  toggleColumnVisibility: (columnId: string) => void;
  resetColumns: () => void;

  // Filters
  filters: LogFilter[];
  addFilter: (filter: Omit<LogFilter, 'id'>) => void;
  removeFilter: (filterId: string) => void;
  toggleFilter: (filterId: string) => void;
  clearFilters: () => void;

  // Time range
  timeRange: TimeRange;
  setTimeRange: (range: TimeRange) => void;

  // Search
  searchQuery: string;
  setSearchQuery: (query: string) => void;

  // Selection
  selectedRecordId: string | null;
  selectedRecord: T | null;
  selectRecord: (recordId: string | null) => void;

  // Sort
  sort: LogSort;
  setSort: (sort: LogSort) => void;

  // Panel state
  sidebarCollapsed: boolean;
  detailCollapsed: boolean;
  toggleSidebar: () => void;
  toggleDetailPanel: () => void;

  // Cell renderers
  cellRenderers: Record<string, (value: unknown, record: T) => ReactNode>;
}

/**
 * Default time range presets.
 */
export const TIME_RANGE_PRESETS: TimeRangePreset[] = [
  {
    label: 'Last 5 minutes',
    value: '5m',
    getRange: () => ({
      from: new Date(Date.now() - 5 * 60 * 1000),
      to: new Date(),
    }),
  },
  {
    label: 'Last 15 minutes',
    value: '15m',
    getRange: () => ({
      from: new Date(Date.now() - 15 * 60 * 1000),
      to: new Date(),
    }),
  },
  {
    label: 'Last 1 hour',
    value: '1h',
    getRange: () => ({
      from: new Date(Date.now() - 60 * 60 * 1000),
      to: new Date(),
    }),
  },
  {
    label: 'Last 4 hours',
    value: '4h',
    getRange: () => ({
      from: new Date(Date.now() - 4 * 60 * 60 * 1000),
      to: new Date(),
    }),
  },
  {
    label: 'Last 24 hours',
    value: '24h',
    getRange: () => ({
      from: new Date(Date.now() - 24 * 60 * 60 * 1000),
      to: new Date(),
    }),
  },
  {
    label: 'Last 7 days',
    value: '7d',
    getRange: () => ({
      from: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
      to: new Date(),
    }),
  },
];

/**
 * Current config version for localStorage migrations.
 */
export const CONFIG_VERSION = 1;
