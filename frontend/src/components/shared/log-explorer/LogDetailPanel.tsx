'use client';

import React, { useMemo } from 'react';
import { cn } from '@/lib/utils';
import { useLogExplorer } from './LogExplorerContext';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { X, ChevronLeft, ChevronRight, Copy, Plus, Filter } from 'lucide-react';
import { formatValue, formatFieldName } from './hooks/useFieldExtractor';
import { LogFilter } from './types';

interface LogDetailPanelProps {
  className?: string;
}

/**
 * Recursively flattens an object into key-value pairs.
 */
function flattenObject(
  obj: Record<string, unknown>,
  prefix = ''
): Array<{ path: string; displayName: string; value: unknown }> {
  const result: Array<{ path: string; displayName: string; value: unknown }> =
    [];

  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    const displayName = formatFieldName(path);

    if (
      value !== null &&
      typeof value === 'object' &&
      !Array.isArray(value) &&
      !(value instanceof Date)
    ) {
      // Recurse into nested objects
      result.push(...flattenObject(value as Record<string, unknown>, path));
    } else {
      result.push({ path, displayName, value });
    }
  }

  return result;
}

/**
 * Formats a timestamp for detailed display.
 */
function formatDetailedTimestamp(timestamp: string): string {
  try {
    const date = new Date(timestamp);
    const formatted = date.toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
    // Add milliseconds manually
    const ms = date.getMilliseconds().toString().padStart(3, '0');
    return `${formatted}.${ms}`;
  } catch {
    return timestamp;
  }
}

/**
 * Detail panel component showing the full content of a selected log record.
 */
export function LogDetailPanel({ className }: LogDetailPanelProps) {
  const {
    selectedRecord,
    selectRecord,
    detailCollapsed,
    toggleDetailPanel,
    addColumn,
    addFilter,
    columns,
  } = useLogExplorer();

  // Flatten the selected record into key-value pairs
  const flattenedFields = useMemo(() => {
    if (!selectedRecord) return [];
    return flattenObject(selectedRecord as unknown as Record<string, unknown>);
  }, [selectedRecord]);

  // Check if a field is visible as a column
  const isFieldVisible = (fieldPath: string) => {
    return columns.some((col) => col.id === fieldPath && col.visible);
  };

  // Copy value to clipboard
  const handleCopyValue = async (value: unknown) => {
    const text = typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value);
    await navigator.clipboard.writeText(text);
  };

  // Add field as column
  const handleAddColumn = (fieldPath: string) => {
    addColumn(fieldPath);
  };

  // Add filter for field value
  const handleAddFilter = (path: string, displayName: string, value: unknown) => {
    const newFilter: Omit<LogFilter, 'id'> = {
      field: path,
      fieldDisplayName: displayName,
      operator: 'equals',
      value: value === null || value === undefined ? undefined : typeof value === 'object' ? JSON.stringify(value) : value as string | number | boolean,
    };
    addFilter(newFilter);
  };

  // Collapsed view
  if (detailCollapsed) {
    return (
      <div
        className={cn(
          'flex flex-col items-center py-2 border-l bg-muted/30',
          className
        )}
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={toggleDetailPanel}
              className="h-8 w-8"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="left">Expand Details</TooltipContent>
        </Tooltip>
      </div>
    );
  }

  return (
    <div
      className={cn(
        'flex flex-col border-l bg-muted/30 w-96 min-w-96',
        className
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between p-2 border-b">
        <span className="text-sm font-medium">Details</span>
        <div className="flex items-center gap-1">
          {selectedRecord && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => selectRecord(null)}
                  className="h-6 w-6"
                >
                  <X className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Clear selection</TooltipContent>
            </Tooltip>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={toggleDetailPanel}
            className="h-6 w-6"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Content */}
      {selectedRecord ? (
        <ScrollArea className="flex-1">
          <div className="p-3 space-y-3">
            {flattenedFields.map(({ path, displayName, value }) => (
              <div key={path} className="group">
                {/* Field name */}
                <div className="flex items-center justify-between mb-1">
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-xs font-medium text-muted-foreground truncate cursor-default">
                        {displayName}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent side="left">
                      <span className="font-mono text-xs">{path}</span>
                    </TooltipContent>
                  </Tooltip>

                  {/* Action buttons */}
                  <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                    {/* Copy value */}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleCopyValue(value)}
                          className="h-5 w-5"
                        >
                          <Copy className="h-3 w-3" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Copy value</TooltipContent>
                    </Tooltip>

                    {/* Add as column */}
                    {!isFieldVisible(path) && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleAddColumn(path)}
                            className="h-5 w-5"
                          >
                            <Plus className="h-3 w-3" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Add as column</TooltipContent>
                      </Tooltip>
                    )}

                    {/* Filter by this value */}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() =>
                            handleAddFilter(path, displayName, value)
                          }
                          className="h-5 w-5"
                        >
                          <Filter className="h-3 w-3" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Filter by this value</TooltipContent>
                    </Tooltip>
                  </div>
                </div>

                {/* Field value */}
                <div
                  className={cn(
                    'text-sm break-words',
                    path === 'timestamp' && 'font-mono'
                  )}
                >
                  {path === 'timestamp'
                    ? formatDetailedTimestamp(value as string)
                    : typeof value === 'object'
                      ? (
                          <pre className="text-xs bg-muted p-2 rounded overflow-auto max-h-32 font-mono">
                            {JSON.stringify(value, null, 2)}
                          </pre>
                        )
                      : formatValue(value)}
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      ) : (
        <div className="flex-1 flex items-center justify-center text-muted-foreground text-sm">
          Select a record to view details
        </div>
      )}
    </div>
  );
}
