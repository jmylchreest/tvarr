'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { useLogExplorer } from './LogExplorerContext';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { X } from 'lucide-react';
import { LogFilter, LogFilterOperator } from './types';

interface FilterBadgesProps {
  className?: string;
}

/**
 * Returns a human-readable label for a filter operator.
 */
function getOperatorLabel(operator: LogFilterOperator): string {
  switch (operator) {
    case 'equals':
      return '=';
    case 'not_equals':
      return '!=';
    case 'contains':
      return 'contains';
    case 'not_contains':
      return '!contains';
    case 'exists':
      return 'exists';
    case 'not_exists':
      return '!exists';
    case 'gt':
      return '>';
    case 'lt':
      return '<';
    case 'gte':
      return '>=';
    case 'lte':
      return '<=';
    default:
      return operator;
  }
}

/**
 * Formats a filter value for display.
 */
function formatFilterValue(value: LogFilter['value']): string {
  if (value === undefined || value === null) return '';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'string' && value.length > 20) {
    return `"${value.slice(0, 20)}..."`;
  }
  if (typeof value === 'string') return `"${value}"`;
  return String(value);
}

/**
 * Component that displays active filters as removable badges.
 */
export function FilterBadges({ className }: FilterBadgesProps) {
  const { filters, removeFilter, clearFilters } = useLogExplorer();

  if (filters.length === 0) {
    return null;
  }

  return (
    <div className={cn('flex flex-wrap items-center gap-2', className)}>
      {filters.map((filter) => (
        <Badge
          key={filter.id}
          variant="secondary"
          className="flex items-center gap-1 pr-1 font-normal"
        >
          <span className="font-medium">{filter.fieldDisplayName}</span>
          <span className="text-muted-foreground">
            {getOperatorLabel(filter.operator)}
          </span>
          {filter.value !== undefined && (
            <span className="font-mono text-xs">
              {formatFilterValue(filter.value)}
            </span>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => removeFilter(filter.id)}
            className="h-4 w-4 ml-1 hover:bg-destructive/20 hover:text-destructive rounded-full"
          >
            <X className="h-3 w-3" />
          </Button>
        </Badge>
      ))}

      {filters.length > 1 && (
        <Button
          variant="ghost"
          size="sm"
          onClick={clearFilters}
          className="h-6 px-2 text-xs text-muted-foreground hover:text-destructive"
        >
          Clear all
        </Button>
      )}
    </div>
  );
}
