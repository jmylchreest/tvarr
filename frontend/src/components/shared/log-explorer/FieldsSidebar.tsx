'use client';

import React, { useState, useMemo } from 'react';
import { cn } from '@/lib/utils';
import { useLogExplorer } from './LogExplorerContext';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Plus,
  Minus,
  Search,
  ChevronLeft,
  ChevronRight,
  Filter,
  Hash,
  Type,
  ToggleLeft,
  Calendar,
  Braces,
  List,
} from 'lucide-react';
import { LogField, LogFilter } from './types';

interface FieldsSidebarProps {
  className?: string;
}

/**
 * Icon component for field types.
 */
function FieldTypeIcon({ type }: { type: LogField['type'] }) {
  const iconProps = { className: 'h-3 w-3 text-muted-foreground' };

  switch (type) {
    case 'number':
      return <Hash {...iconProps} />;
    case 'boolean':
      return <ToggleLeft {...iconProps} />;
    case 'datetime':
      return <Calendar {...iconProps} />;
    case 'object':
      return <Braces {...iconProps} />;
    case 'array':
      return <List {...iconProps} />;
    case 'string':
    default:
      return <Type {...iconProps} />;
  }
}

/**
 * Sidebar component showing available fields for the log explorer.
 * Allows users to add fields as columns or create filters.
 */
export function FieldsSidebar({ className }: FieldsSidebarProps) {
  const {
    availableFields,
    columns,
    addColumn,
    removeColumn,
    addFilter,
    sidebarCollapsed,
    toggleSidebar,
  } = useLogExplorer();

  const [searchQuery, setSearchQuery] = useState('');

  // Check if a field is currently visible as a column
  const isFieldVisible = (fieldPath: string) => {
    return columns.some((col) => col.id === fieldPath && col.visible);
  };

  // Filter fields based on search query
  const filteredFields = useMemo(() => {
    if (!searchQuery) return availableFields;

    const query = searchQuery.toLowerCase();
    return availableFields.filter(
      (field) =>
        field.path.toLowerCase().includes(query) ||
        field.displayName.toLowerCase().includes(query)
    );
  }, [availableFields, searchQuery]);

  // Handle adding a field as a column
  const handleAddColumn = (fieldPath: string) => {
    addColumn(fieldPath);
  };

  // Handle removing a field column
  const handleRemoveColumn = (fieldPath: string) => {
    removeColumn(fieldPath);
  };

  // Handle adding a filter for a field
  const handleAddFilter = (field: LogField) => {
    const newFilter: Omit<LogFilter, 'id'> = {
      field: field.path,
      fieldDisplayName: field.displayName,
      operator: 'exists',
    };
    addFilter(newFilter);
  };

  // Collapsed view
  if (sidebarCollapsed) {
    return (
      <div
        className={cn(
          'flex flex-col items-center py-2 border-r bg-muted/30',
          className
        )}
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={toggleSidebar}
              className="h-8 w-8"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Expand Fields</TooltipContent>
        </Tooltip>
      </div>
    );
  }

  return (
    <div
      className={cn(
        'flex flex-col border-r bg-muted/30 w-64 min-w-64',
        className
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between p-2 border-b">
        <span className="text-sm font-medium">Fields</span>
        <Button
          variant="ghost"
          size="icon"
          onClick={toggleSidebar}
          className="h-6 w-6"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
      </div>

      {/* Search */}
      <div className="p-2 border-b">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Filter fields..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-8 h-8 text-sm"
          />
        </div>
      </div>

      {/* Field list */}
      <ScrollArea className="flex-1">
        <div className="p-2 space-y-1">
          {filteredFields.length === 0 ? (
            <div className="text-sm text-muted-foreground text-center py-4">
              No fields found
            </div>
          ) : (
            filteredFields.map((field) => {
              const isVisible = isFieldVisible(field.path);

              return (
                <div
                  key={field.path}
                  className={cn(
                    'group flex items-center gap-2 p-1.5 rounded-md hover:bg-muted/50 transition-colors',
                    isVisible && 'bg-muted/30'
                  )}
                >
                  {/* Toggle column visibility */}
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() =>
                      isVisible
                        ? handleRemoveColumn(field.path)
                        : handleAddColumn(field.path)
                    }
                    className={cn(
                      'h-5 w-5 flex-shrink-0',
                      isVisible
                        ? 'text-primary'
                        : 'text-muted-foreground opacity-0 group-hover:opacity-100'
                    )}
                  >
                    {isVisible ? (
                      <Minus className="h-3 w-3" />
                    ) : (
                      <Plus className="h-3 w-3" />
                    )}
                  </Button>

                  {/* Field type icon */}
                  <FieldTypeIcon type={field.type} />

                  {/* Field name */}
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="flex-1 text-sm truncate cursor-default">
                        {field.displayName}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent side="right" className="max-w-xs">
                      <div className="space-y-1">
                        <div className="font-mono text-xs">{field.path}</div>
                        <div className="text-xs text-muted-foreground">
                          Type: {field.type}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          In {field.count} records
                        </div>
                        {field.sampleValues && field.sampleValues.length > 0 && (
                          <div className="text-xs">
                            <span className="text-muted-foreground">
                              Sample:{' '}
                            </span>
                            {field.sampleValues.slice(0, 3).join(', ')}
                          </div>
                        )}
                      </div>
                    </TooltipContent>
                  </Tooltip>

                  {/* Add filter button */}
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleAddFilter(field)}
                        className="h-5 w-5 flex-shrink-0 opacity-0 group-hover:opacity-100 text-muted-foreground"
                      >
                        <Filter className="h-3 w-3" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="right">Add filter</TooltipContent>
                  </Tooltip>
                </div>
              );
            })
          )}
        </div>
      </ScrollArea>

      {/* Footer with field count */}
      <div className="p-2 border-t text-xs text-muted-foreground">
        {filteredFields.length} of {availableFields.length} fields
      </div>
    </div>
  );
}
