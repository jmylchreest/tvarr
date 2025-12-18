// Main component
export { LogExplorer } from './LogExplorer';

// Context and hooks
export { LogExplorerProvider, useLogExplorer } from './LogExplorerContext';
export { useLogColumns } from './hooks/useLogColumns';
export { useLogSearch, highlightMatches } from './hooks/useLogSearch';
export {
  useFieldExtractor,
  extractFields,
  getNestedValue,
  formatFieldName,
  formatValue,
} from './hooks/useFieldExtractor';

// Sub-components (for advanced usage)
export { LogTable } from './LogTable';
export { FieldsSidebar } from './FieldsSidebar';
export { LogDetailPanel } from './LogDetailPanel';
export { FilterBadges } from './FilterBadges';
export { TimeRangeSelector } from './TimeRangeSelector';
export { ColumnSelector } from './ColumnSelector';

// Types
export type {
  LogField,
  LogColumn,
  LogFilter,
  LogFilterOperator,
  LogSort,
  TimeRange,
  TimeRangePreset,
  LogExplorerProps,
  LogExplorerContextValue,
  LogExplorerStoredConfig,
  CellRendererProps,
} from './types';

// Constants
export { TIME_RANGE_PRESETS, CONFIG_VERSION } from './types';
