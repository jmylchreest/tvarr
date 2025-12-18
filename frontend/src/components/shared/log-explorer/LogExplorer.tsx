'use client';

import React, { ReactNode, useState, useMemo, useCallback } from 'react';
import { cn } from '@/lib/utils';
import { LogExplorerProvider, useLogExplorer } from './LogExplorerContext';
import { LogTable } from './LogTable';
import { FieldsSidebar } from './FieldsSidebar';
import { LogDetailPanel } from './LogDetailPanel';
import { FilterBadges } from './FilterBadges';
import { TimeRangeSelector } from './TimeRangeSelector';
import { ColumnSelector } from './ColumnSelector';
import { useLogSearch } from './hooks/useLogSearch';
import { LogColumn, LogExplorerProps } from './types';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { TooltipProvider } from '@/components/ui/tooltip';
import { Search, RefreshCw, Loader2 } from 'lucide-react';

/**
 * Inner component that uses the LogExplorer context.
 */
function LogExplorerInner<T extends { id: string; timestamp: string }>({
  isLoading,
  onRefresh,
  showFieldsSidebar = true,
  showDetailPanel = true,
  title,
  emptyState,
  controls,
  statsCards,
}: Pick<
  LogExplorerProps<T>,
  | 'isLoading'
  | 'onRefresh'
  | 'showFieldsSidebar'
  | 'showDetailPanel'
  | 'title'
  | 'emptyState'
  | 'controls'
  | 'statsCards'
>) {
  const {
    data,
    filteredData,
    availableFields,
    searchQuery,
    setSearchQuery,
    sidebarCollapsed,
    detailCollapsed,
  } = useLogExplorer<T>();

  // Initialize search hook
  const { search } = useLogSearch(
    filteredData as unknown as Record<string, unknown>[],
    availableFields
  );

  // Search state
  const [localSearchQuery, setLocalSearchQuery] = useState('');

  // Apply search
  const displayData = useMemo(() => {
    if (!localSearchQuery.trim()) {
      return filteredData;
    }
    const result = search(localSearchQuery);
    return result.items as unknown as T[];
  }, [filteredData, localSearchQuery, search]);

  // Handle search input
  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setLocalSearchQuery(e.target.value);
    },
    []
  );

  // Handle search submit
  const handleSearchSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      setSearchQuery(localSearchQuery);
    },
    [localSearchQuery, setSearchQuery]
  );

  return (
    <div className="flex flex-col h-full">
      {/* Stats cards section */}
      {statsCards && <div className="flex-shrink-0">{statsCards}</div>}

      {/* Header toolbar */}
      <div className="flex-shrink-0 flex flex-col gap-2 p-3 border-b">
        {/* Title and controls row */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            {title && (
              <h2 className="text-lg font-semibold">{title}</h2>
            )}
            {controls}
          </div>
          <div className="flex items-center gap-2">
            {/* Refresh button */}
            {onRefresh && (
              <Button
                variant="outline"
                size="sm"
                onClick={onRefresh}
                disabled={isLoading}
                className="h-8"
              >
                {isLoading ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="h-3.5 w-3.5" />
                )}
              </Button>
            )}
          </div>
        </div>

        {/* Search and filters row */}
        <div className="flex items-center gap-2">
          {/* Search input */}
          <form onSubmit={handleSearchSubmit} className="flex-1 max-w-md">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search logs..."
                value={localSearchQuery}
                onChange={handleSearchChange}
                className="pl-9 h-8"
              />
            </div>
          </form>

          {/* Time range selector */}
          <TimeRangeSelector />

          {/* Column selector */}
          <ColumnSelector />

          {/* Record count */}
          <div className="text-sm text-muted-foreground whitespace-nowrap">
            {displayData.length === data.length ? (
              <span>{data.length} records</span>
            ) : (
              <span>
                {displayData.length} of {data.length}
              </span>
            )}
          </div>
        </div>

        {/* Filter badges */}
        <FilterBadges />
      </div>

      {/* Main content area */}
      <div className="flex-1 flex min-h-0">
        {/* Fields sidebar */}
        {showFieldsSidebar && <FieldsSidebar />}

        {/* Table */}
        <div className="flex-1 min-w-0 overflow-hidden">
          {displayData.length === 0 && !isLoading ? (
            <div className="h-full flex items-center justify-center">
              <div className="text-center">
                <h3 className="text-lg font-medium">
                  {emptyState?.title || 'No records found'}
                </h3>
                {emptyState?.description && (
                  <p className="text-sm text-muted-foreground mt-1">
                    {emptyState.description}
                  </p>
                )}
              </div>
            </div>
          ) : (
            <LogTable className="h-full" />
          )}
        </div>

        {/* Detail panel */}
        {showDetailPanel && <LogDetailPanel />}
      </div>
    </div>
  );
}

/**
 * LogExplorer component - A Kibana-style log viewer with fuzzy search,
 * configurable columns, and a detail panel.
 *
 * @example
 * ```tsx
 * <LogExplorer
 *   data={logs}
 *   storageKey="app-logs"
 *   defaultColumns={[
 *     { id: 'timestamp', header: 'Time', fieldPath: 'timestamp', width: 120, minWidth: 80, visible: true },
 *     { id: 'level', header: 'Level', fieldPath: 'level', width: 80, minWidth: 60, visible: true },
 *     { id: 'message', header: 'Message', fieldPath: 'message', width: 400, minWidth: 200, visible: true },
 *   ]}
 *   onRefresh={handleRefresh}
 * />
 * ```
 */
export function LogExplorer<T extends { id: string; timestamp: string }>({
  data,
  storageKey,
  isLoading = false,
  defaultColumns,
  excludeFields = [],
  cellRenderers = {},
  onRefresh,
  showFieldsSidebar = true,
  showDetailPanel = true,
  title,
  emptyState,
  controls,
  statsCards,
}: LogExplorerProps<T>) {
  return (
    <TooltipProvider>
      <LogExplorerProvider
        data={data}
        storageKey={storageKey}
        defaultColumns={defaultColumns}
        excludeFields={excludeFields}
        cellRenderers={cellRenderers}
      >
        <LogExplorerInner
          isLoading={isLoading}
          onRefresh={onRefresh}
          showFieldsSidebar={showFieldsSidebar}
          showDetailPanel={showDetailPanel}
          title={title}
          emptyState={emptyState}
          controls={controls}
          statsCards={statsCards}
        />
      </LogExplorerProvider>
    </TooltipProvider>
  );
}
