'use client';

import React, { useState } from 'react';
import { cn } from '@/lib/utils';
import { useLogExplorer } from './LogExplorerContext';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Checkbox } from '@/components/ui/checkbox';
import { Columns, Search, RotateCcw } from 'lucide-react';

interface ColumnSelectorProps {
  className?: string;
}

/**
 * Popover component for managing column visibility.
 */
export function ColumnSelector({ className }: ColumnSelectorProps) {
  const { columns, toggleColumnVisibility, resetColumns } = useLogExplorer();
  const [searchQuery, setSearchQuery] = useState('');
  const [open, setOpen] = useState(false);

  // Sort columns by order
  const sortedColumns = [...columns].sort((a, b) => a.order - b.order);

  // Filter columns based on search
  const filteredColumns = searchQuery
    ? sortedColumns.filter((col) =>
        col.header.toLowerCase().includes(searchQuery.toLowerCase())
      )
    : sortedColumns;

  // Count visible columns
  const visibleCount = columns.filter((col) => col.visible).length;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn('h-8 gap-1.5', className)}
        >
          <Columns className="h-3.5 w-3.5" />
          <span className="text-sm">Columns</span>
          <span className="text-xs text-muted-foreground">({visibleCount})</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 p-0">
        {/* Search */}
        <div className="p-2 border-b">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search columns..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-8 h-8 text-sm"
            />
          </div>
        </div>

        {/* Column list */}
        <ScrollArea className="max-h-64">
          <div className="p-2 space-y-1">
            {filteredColumns.length === 0 ? (
              <div className="text-sm text-muted-foreground text-center py-2">
                No columns found
              </div>
            ) : (
              filteredColumns.map((column) => (
                <label
                  key={column.id}
                  className="flex items-center gap-2 p-1.5 rounded-md hover:bg-muted/50 cursor-pointer transition-colors"
                >
                  <Checkbox
                    checked={column.visible}
                    onCheckedChange={() => toggleColumnVisibility(column.id)}
                  />
                  <span className="text-sm truncate flex-1">
                    {column.header}
                  </span>
                </label>
              ))
            )}
          </div>
        </ScrollArea>

        {/* Footer */}
        <div className="p-2 border-t">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              resetColumns();
              setOpen(false);
            }}
            className="w-full h-8 text-xs"
          >
            <RotateCcw className="h-3 w-3 mr-1.5" />
            Reset to defaults
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
