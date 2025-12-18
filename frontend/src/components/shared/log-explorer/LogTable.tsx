'use client';

import React, { useCallback, useRef, useState, useEffect } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { cn } from '@/lib/utils';
import { useLogExplorer } from './LogExplorerContext';
import { formatValue, getNestedValue } from './hooks/useFieldExtractor';
import { ArrowUp, ArrowDown, GripVertical } from 'lucide-react';
import { LogColumn } from './types';

interface LogTableProps<T extends { id: string; timestamp: string }> {
  className?: string;
}

/**
 * Formats a timestamp for display.
 */
function formatTimestamp(timestamp: string): string {
  try {
    const date = new Date(timestamp);
    const time = date.toLocaleTimeString('en-US', {
      hour12: false,
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    // Add milliseconds manually
    const ms = date.getMilliseconds().toString().padStart(3, '0');
    return `${time}.${ms}`;
  } catch {
    return timestamp;
  }
}

/**
 * Log table component with sortable columns and row selection.
 */
export function LogTable<T extends { id: string; timestamp: string }>({
  className,
}: LogTableProps<T>) {
  const {
    filteredData,
    columns,
    selectedRecordId,
    selectRecord,
    sort,
    setSort,
    resizeColumn,
    cellRenderers,
  } = useLogExplorer<T>();

  // Get visible columns sorted by order
  const visibleColumns = columns
    .filter((col) => col.visible)
    .sort((a, b) => a.order - b.order);

  // Handle column click for sorting
  const handleColumnClick = useCallback(
    (column: LogColumn) => {
      if (sort.field === column.fieldPath) {
        // Toggle direction
        setSort({
          field: column.fieldPath,
          direction: sort.direction === 'asc' ? 'desc' : 'asc',
        });
      } else {
        // New column, default to descending for timestamps, ascending otherwise
        setSort({
          field: column.fieldPath,
          direction: column.fieldPath === 'timestamp' ? 'desc' : 'asc',
        });
      }
    },
    [sort, setSort]
  );

  // Handle row click for selection
  const handleRowClick = useCallback(
    (recordId: string) => {
      selectRecord(recordId === selectedRecordId ? null : recordId);
    },
    [selectRecord, selectedRecordId]
  );

  // Column resizing
  const [resizing, setResizing] = useState<{
    columnId: string;
    startX: number;
    startWidth: number;
  } | null>(null);
  const tableRef = useRef<HTMLTableElement>(null);

  const handleResizeStart = useCallback(
    (e: React.MouseEvent, column: LogColumn) => {
      e.preventDefault();
      e.stopPropagation();
      setResizing({
        columnId: column.id,
        startX: e.clientX,
        startWidth: column.width,
      });
    },
    []
  );

  useEffect(() => {
    if (!resizing) return;

    const handleMouseMove = (e: MouseEvent) => {
      const delta = e.clientX - resizing.startX;
      const newWidth = Math.max(60, resizing.startWidth + delta);
      resizeColumn(resizing.columnId, newWidth);
    };

    const handleMouseUp = () => {
      setResizing(null);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [resizing, resizeColumn]);

  // Render cell value
  const renderCellValue = useCallback(
    (record: T, column: LogColumn) => {
      const value = getNestedValue(
        record as unknown as Record<string, unknown>,
        column.fieldPath
      );

      // Check for custom renderer
      if (cellRenderers[column.fieldPath]) {
        return cellRenderers[column.fieldPath](value, record);
      }

      // Special formatting for timestamp
      if (column.fieldPath === 'timestamp') {
        return formatTimestamp(value as string);
      }

      // Default formatting
      return formatValue(value);
    },
    [cellRenderers]
  );

  // Calculate total width for table
  const totalWidth = visibleColumns.reduce((sum, col) => sum + col.width, 0);

  return (
    <div className={cn('relative overflow-auto', className)}>
      <Table
        ref={tableRef}
        className="w-max min-w-full"
        style={{ minWidth: totalWidth }}
      >
        <TableHeader className="sticky top-0 z-10 bg-background">
          <TableRow>
            {visibleColumns.map((column) => (
              <TableHead
                key={column.id}
                className={cn(
                  'relative select-none cursor-pointer hover:bg-muted/50 transition-colors',
                  column.pinned && 'sticky left-0 bg-background z-20'
                )}
                style={{ width: column.width, minWidth: column.minWidth }}
                onClick={() => handleColumnClick(column)}
              >
                <div className="flex items-center gap-1 pr-4">
                  <span className="truncate">{column.header}</span>
                  {sort.field === column.fieldPath && (
                    <span className="flex-shrink-0">
                      {sort.direction === 'asc' ? (
                        <ArrowUp className="h-3 w-3" />
                      ) : (
                        <ArrowDown className="h-3 w-3" />
                      )}
                    </span>
                  )}
                </div>
                {/* Resize handle */}
                <div
                  className={cn(
                    'absolute right-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-primary/50 transition-colors',
                    resizing?.columnId === column.id && 'bg-primary'
                  )}
                  onMouseDown={(e) => handleResizeStart(e, column)}
                  onClick={(e) => e.stopPropagation()}
                />
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {filteredData.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={visibleColumns.length}
                className="h-32 text-center text-muted-foreground"
              >
                No records to display
              </TableCell>
            </TableRow>
          ) : (
            filteredData.map((record) => (
              <TableRow
                key={record.id}
                className={cn(
                  'cursor-pointer transition-colors',
                  selectedRecordId === record.id && 'bg-muted'
                )}
                onClick={() => handleRowClick(record.id)}
              >
                {visibleColumns.map((column) => (
                  <TableCell
                    key={column.id}
                    className={cn(
                      'truncate',
                      column.pinned && 'sticky left-0 bg-background'
                    )}
                    style={{ width: column.width, maxWidth: column.width }}
                  >
                    {renderCellValue(record, column)}
                  </TableCell>
                ))}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
