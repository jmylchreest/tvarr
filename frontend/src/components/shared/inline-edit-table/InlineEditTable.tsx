'use client';

import React, { useState, useCallback, useMemo, useRef, useEffect } from 'react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuCheckboxItem,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Plus,
  Trash2,
  MoreHorizontal,
  Settings2,
  ChevronUp,
  ChevronDown,
  AlertCircle,
  ImageIcon,
} from 'lucide-react';
import { EmptyState } from '../feedback/EmptyState';

// Column definition
export interface ColumnDef<T> {
  /** Unique column identifier */
  id: string;
  /** Column header label */
  header: string;
  /** Accessor key on the data object */
  accessorKey: keyof T;
  /** Cell width (CSS value) */
  width?: string;
  /** Minimum width */
  minWidth?: string;
  /** Whether the column is required (cannot be hidden) */
  required?: boolean;
  /** Whether the column is visible by default */
  defaultVisible?: boolean;
  /** Input type for editing */
  type?: 'text' | 'number' | 'url' | 'checkbox' | 'image';
  /** Placeholder text for input */
  placeholder?: string;
  /** Image URL resolver (for 'image' type) - converts value to displayable URL */
  resolveImageUrl?: (value: string) => string | undefined;
  /** Validation function - returns error message or undefined */
  validate?: (value: any, row: T) => string | undefined;
  /** Custom cell renderer */
  cell?: (value: any, row: T, onChange: (value: any) => void) => React.ReactNode;
}

// Row with internal metadata
interface InternalRow<T> {
  id: string;
  data: T;
  errors: Record<string, string>;
}

// Cell edit state for tracking original values (for Escape to revert)
interface CellEditState {
  rowId: string;
  columnId: string;
  originalValue: any;
}

export interface InlineEditTableProps<T> {
  /** Column definitions */
  columns: ColumnDef<T>[];
  /** Data rows */
  data: T[];
  /** Callback when data changes */
  onChange: (data: T[]) => void;
  /** Function to create a new empty row */
  createEmpty: () => T;
  /** Function to get row ID */
  getRowId: (row: T) => string;
  /** Callback when validation state changes */
  onValidityChange?: (isValid: boolean) => void;
  /** Loading state */
  isLoading?: boolean;
  /** Whether to allow adding rows */
  canAdd?: boolean;
  /** Whether to allow removing rows */
  canRemove?: boolean;
  /** Whether to allow reordering rows */
  canReorder?: boolean;
  /** Minimum number of rows required */
  minRows?: number;
  /** Maximum number of rows allowed */
  maxRows?: number;
  /** Additional className */
  className?: string;
  /** Empty state configuration */
  emptyState?: {
    title: string;
    description?: string;
  };
  /** Toolbar actions (rendered before column visibility) */
  toolbarActions?: React.ReactNode;
}

/**
 * InlineEditTable - An editable data table with inline validation
 *
 * Features:
 * - Inline cell editing with immediate validation
 * - Column visibility toggle
 * - Row add/remove/reorder
 * - Keyboard navigation (Tab, Enter, Escape)
 * - Validation error display with tooltips
 *
 * Usage:
 * ```tsx
 * <InlineEditTable
 *   columns={[
 *     { id: 'name', header: 'Name', accessorKey: 'channel_name', required: true },
 *     { id: 'url', header: 'URL', accessorKey: 'stream_url', type: 'url' },
 *   ]}
 *   data={channels}
 *   onChange={setChannels}
 *   createEmpty={() => ({ channel_name: '', stream_url: '' })}
 *   getRowId={(row) => row.id}
 * />
 * ```
 */
export function InlineEditTable<T extends Record<string, any>>({
  columns,
  data,
  onChange,
  createEmpty,
  getRowId,
  onValidityChange,
  isLoading = false,
  canAdd = true,
  canRemove = true,
  canReorder = false,
  minRows = 0,
  maxRows = Infinity,
  className,
  emptyState,
  toolbarActions,
}: InlineEditTableProps<T>) {
  // Column visibility state
  const [visibleColumns, setVisibleColumns] = useState<Set<string>>(() => {
    const initial = new Set<string>();
    columns.forEach((col) => {
      if (col.required || col.defaultVisible !== false) {
        initial.add(col.id);
      }
    });
    return initial;
  });

  // Track active cell edit for Escape to revert
  const [activeCellEdit, setActiveCellEdit] = useState<CellEditState | null>(null);

  // Ref for the table container to manage focus
  const tableRef = useRef<HTMLTableElement>(null);

  // Internal rows with validation state
  const [internalRows, setInternalRows] = useState<InternalRow<T>[]>(() =>
    data.map((row) => ({
      id: getRowId(row),
      data: row,
      errors: {},
    }))
  );

  // Sync internal state when data prop changes externally
  useEffect(() => {
    const currentIds = new Set(internalRows.map((r) => r.id));
    const newIds = new Set(data.map(getRowId));

    // Only sync if IDs have changed (external update)
    if (
      currentIds.size !== newIds.size ||
      ![...currentIds].every((id) => newIds.has(id))
    ) {
      setInternalRows(
        data.map((row) => ({
          id: getRowId(row),
          data: row,
          errors: {},
        }))
      );
    }
  }, [data, getRowId]); // eslint-disable-line react-hooks/exhaustive-deps

  // Validate all rows
  const validateAll = useCallback(
    (rows: InternalRow<T>[]): InternalRow<T>[] => {
      return rows.map((row) => {
        const errors: Record<string, string> = {};
        columns.forEach((col) => {
          if (col.validate) {
            const error = col.validate(row.data[col.accessorKey], row.data);
            if (error) {
              errors[col.id] = error;
            }
          }
        });
        return { ...row, errors };
      });
    },
    [columns]
  );

  // Check overall validity
  const isValid = useMemo(() => {
    const hasErrors = internalRows.some(
      (row) => Object.keys(row.errors).length > 0
    );
    const meetsMinRows = internalRows.length >= minRows;
    return !hasErrors && meetsMinRows;
  }, [internalRows, minRows]);

  // Report validity changes
  useEffect(() => {
    onValidityChange?.(isValid);
  }, [isValid, onValidityChange]);

  // Update a cell value
  const updateCell = useCallback(
    (rowId: string, columnId: string, value: any) => {
      setInternalRows((prev) => {
        const updated = prev.map((row) => {
          if (row.id !== rowId) return row;

          const column = columns.find((c) => c.id === columnId);
          if (!column) return row;

          const newData = { ...row.data, [column.accessorKey]: value };
          const errors = { ...row.errors };

          // Validate this cell
          if (column.validate) {
            const error = column.validate(value, newData);
            if (error) {
              errors[columnId] = error;
            } else {
              delete errors[columnId];
            }
          }

          return { ...row, data: newData, errors };
        });

        // Emit change to parent
        onChange(updated.map((r) => r.data));

        return updated;
      });
    },
    [columns, onChange]
  );

  // Add a new row
  const addRow = useCallback(() => {
    if (internalRows.length >= maxRows) return;

    const newRow = createEmpty();
    const newInternalRow: InternalRow<T> = {
      id: getRowId(newRow),
      data: newRow,
      errors: {},
    };

    setInternalRows((prev) => {
      const updated = validateAll([...prev, newInternalRow]);
      onChange(updated.map((r) => r.data));
      return updated;
    });
  }, [createEmpty, getRowId, internalRows.length, maxRows, onChange, validateAll]);

  // Remove a row
  const removeRow = useCallback(
    (rowId: string) => {
      if (internalRows.length <= minRows) return;

      setInternalRows((prev) => {
        const updated = prev.filter((r) => r.id !== rowId);
        onChange(updated.map((r) => r.data));
        return updated;
      });
    },
    [internalRows.length, minRows, onChange]
  );

  // Move row up/down
  const moveRow = useCallback(
    (rowId: string, direction: 'up' | 'down') => {
      setInternalRows((prev) => {
        const index = prev.findIndex((r) => r.id === rowId);
        if (index === -1) return prev;

        const newIndex = direction === 'up' ? index - 1 : index + 1;
        if (newIndex < 0 || newIndex >= prev.length) return prev;

        const updated = [...prev];
        [updated[index], updated[newIndex]] = [updated[newIndex], updated[index]];
        onChange(updated.map((r) => r.data));
        return updated;
      });
    },
    [onChange]
  );

  // Toggle column visibility
  const toggleColumn = useCallback((columnId: string) => {
    setVisibleColumns((prev) => {
      const next = new Set(prev);
      if (next.has(columnId)) {
        next.delete(columnId);
      } else {
        next.add(columnId);
      }
      return next;
    });
  }, []);

  // Visible columns
  const displayColumns = useMemo(
    () => columns.filter((col) => visibleColumns.has(col.id)),
    [columns, visibleColumns]
  );

  // Handle focus on a cell - store original value for Escape to revert
  const handleCellFocus = useCallback(
    (rowId: string, columnId: string, value: any) => {
      setActiveCellEdit({ rowId, columnId, originalValue: value });
    },
    []
  );

  // Handle blur on a cell - clear active edit state
  const handleCellBlur = useCallback(() => {
    setActiveCellEdit(null);
  }, []);

  // Handle keyboard events on cells
  const handleCellKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>, rowId: string, columnId: string) => {
      if (e.key === 'Escape') {
        // Revert to original value
        if (activeCellEdit && activeCellEdit.rowId === rowId && activeCellEdit.columnId === columnId) {
          updateCell(rowId, columnId, activeCellEdit.originalValue);
          // Blur the input to indicate cancellation
          e.currentTarget.blur();
        }
        e.preventDefault();
      } else if (e.key === 'Enter' && !e.shiftKey) {
        // Move to next row, same column (or confirm edit)
        e.currentTarget.blur();
        e.preventDefault();
      }
    },
    [activeCellEdit, updateCell]
  );

  // Render cell input
  const renderCell = (
    row: InternalRow<T>,
    column: ColumnDef<T>,
    tabIndex: number
  ) => {
    const value = row.data[column.accessorKey];
    const error = row.errors[column.id];
    const hasError = !!error;

    // Custom cell renderer
    if (column.cell) {
      return column.cell(value, row.data, (newValue) =>
        updateCell(row.id, column.id, newValue)
      );
    }

    // Checkbox
    if (column.type === 'checkbox') {
      return (
        <div className="flex items-center justify-center">
          <Checkbox
            checked={!!value}
            onCheckedChange={(checked) =>
              updateCell(row.id, column.id, !!checked)
            }
            tabIndex={tabIndex}
          />
        </div>
      );
    }

    // Image type - input with preview thumbnail
    if (column.type === 'image') {
      const imageUrl = column.resolveImageUrl?.(value) ?? (value && /^https?:\/\//.test(value) ? value : undefined);

      const imageInput = (
        <div className="flex items-center gap-2">
          <div className="flex-shrink-0 w-8 h-8 rounded border bg-muted/50 flex items-center justify-center overflow-hidden">
            {imageUrl ? (
              <img
                src={imageUrl}
                alt=""
                className="w-full h-full object-contain"
                onError={(e) => {
                  // Replace with placeholder on error
                  e.currentTarget.style.display = 'none';
                  e.currentTarget.nextElementSibling?.classList.remove('hidden');
                }}
              />
            ) : null}
            <ImageIcon className={cn('h-4 w-4 text-muted-foreground', imageUrl && 'hidden')} />
          </div>
          <Input
            type="text"
            value={value ?? ''}
            onChange={(e) => updateCell(row.id, column.id, e.target.value)}
            onFocus={() => handleCellFocus(row.id, column.id, value)}
            onBlur={handleCellBlur}
            onKeyDown={(e) => handleCellKeyDown(e, row.id, column.id)}
            placeholder={column.placeholder}
            className={cn(
              'h-8 text-sm flex-1',
              hasError && 'border-destructive focus-visible:ring-destructive'
            )}
            tabIndex={tabIndex}
          />
        </div>
      );

      if (hasError) {
        return (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="relative">
                  {imageInput}
                  <AlertCircle className="absolute right-2 top-1/2 -translate-y-1/2 h-4 w-4 text-destructive" />
                </div>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="text-destructive">
                {error}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        );
      }

      return imageInput;
    }

    // Text/Number/URL input
    const inputElement = (
      <Input
        type={column.type === 'number' ? 'number' : 'text'}
        value={value ?? ''}
        onChange={(e) => {
          const newValue =
            column.type === 'number'
              ? e.target.value
                ? Number(e.target.value)
                : undefined
              : e.target.value;
          updateCell(row.id, column.id, newValue);
        }}
        onFocus={() => handleCellFocus(row.id, column.id, value)}
        onBlur={handleCellBlur}
        onKeyDown={(e) => handleCellKeyDown(e, row.id, column.id)}
        placeholder={column.placeholder}
        className={cn(
          'h-8 text-sm',
          hasError && 'border-destructive focus-visible:ring-destructive'
        )}
        tabIndex={tabIndex}
      />
    );

    // Wrap with tooltip if error
    if (hasError) {
      return (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="relative">
                {inputElement}
                <AlertCircle className="absolute right-2 top-1/2 -translate-y-1/2 h-4 w-4 text-destructive" />
              </div>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="text-destructive">
              {error}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      );
    }

    return inputElement;
  };

  return (
    <div className={cn('flex flex-col', className)}>
      {/* Toolbar */}
      <div className="flex items-center justify-between gap-2 px-2 py-2 border-b">
        <div className="flex items-center gap-2">
          {canAdd && (
            <Button
              size="sm"
              variant="outline"
              onClick={addRow}
              disabled={internalRows.length >= maxRows}
            >
              <Plus className="h-4 w-4 mr-1" />
              Add Row
            </Button>
          )}
          {toolbarActions}
        </div>

        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {internalRows.length} row{internalRows.length !== 1 ? 's' : ''}
            {!isValid && (
              <span className="text-destructive ml-2">
                (has errors)
              </span>
            )}
          </span>

          {/* Column visibility */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="ghost">
                <Settings2 className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {columns.map((col) => (
                <DropdownMenuCheckboxItem
                  key={col.id}
                  checked={visibleColumns.has(col.id)}
                  onCheckedChange={() => toggleColumn(col.id)}
                  disabled={col.required}
                >
                  {col.header}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Table */}
      <ScrollArea className="flex-1">
        {isLoading ? (
          <div className="p-4">Loading...</div>
        ) : internalRows.length === 0 ? (
          <EmptyState
            title={emptyState?.title ?? 'No data'}
            description={emptyState?.description}
            action={
              canAdd
                ? {
                    label: 'Add Row',
                    onClick: addRow,
                  }
                : undefined
            }
            size="sm"
            className="py-12"
          />
        ) : (
          <table className="w-full">
            <thead className="bg-muted/50 sticky top-0">
              <tr>
                {canReorder && (
                  <th className="w-10 px-2 py-2 text-left text-xs font-medium text-muted-foreground" />
                )}
                {displayColumns.map((col) => (
                  <th
                    key={col.id}
                    className="px-2 py-2 text-left text-xs font-medium text-muted-foreground"
                    style={{ width: col.width, minWidth: col.minWidth }}
                  >
                    {col.header}
                  </th>
                ))}
                {canRemove && (
                  <th className="w-10 px-2 py-2 text-left text-xs font-medium text-muted-foreground" />
                )}
              </tr>
            </thead>
            <tbody>
              {internalRows.map((row, rowIndex) => {
                const hasRowError = Object.keys(row.errors).length > 0;
                return (
                  <tr
                    key={row.id}
                    className={cn(
                      'border-b hover:bg-muted/30 transition-colors',
                      hasRowError && 'bg-destructive/5'
                    )}
                  >
                    {canReorder && (
                      <td className="px-1 py-1">
                        <div className="flex flex-col">
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-5 w-5"
                            disabled={rowIndex === 0}
                            onClick={() => moveRow(row.id, 'up')}
                          >
                            <ChevronUp className="h-3 w-3" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-5 w-5"
                            disabled={rowIndex === internalRows.length - 1}
                            onClick={() => moveRow(row.id, 'down')}
                          >
                            <ChevronDown className="h-3 w-3" />
                          </Button>
                        </div>
                      </td>
                    )}
                    {displayColumns.map((col, colIndex) => (
                      <td key={col.id} className="px-2 py-1">
                        {renderCell(
                          row,
                          col,
                          rowIndex * displayColumns.length + colIndex
                        )}
                      </td>
                    ))}
                    {canRemove && (
                      <td className="px-1 py-1">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7 text-muted-foreground hover:text-destructive"
                          onClick={() => removeRow(row.id)}
                          disabled={internalRows.length <= minRows}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </ScrollArea>
    </div>
  );
}

export default InlineEditTable;
