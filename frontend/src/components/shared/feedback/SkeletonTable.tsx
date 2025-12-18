'use client';

import React from 'react';
import { cn } from '@/lib/utils';

export interface SkeletonTableProps {
  /** Number of rows to display */
  rows?: number;
  /** Number of columns to display */
  columns?: number;
  /** Whether to show a header row */
  showHeader?: boolean;
  /** Additional className for container */
  className?: string;
  /** Row height variant */
  rowHeight?: 'sm' | 'md' | 'lg';
}

/**
 * SkeletonTable - Loading placeholder for table content
 *
 * Usage:
 * ```tsx
 * {isLoading ? <SkeletonTable rows={5} columns={4} /> : <DataTable ... />}
 * ```
 */
export const SkeletonTable: React.FC<SkeletonTableProps> = ({
  rows = 5,
  columns = 4,
  showHeader = true,
  className,
  rowHeight = 'md',
}) => {
  const heightConfig = {
    sm: 'h-8',
    md: 'h-10',
    lg: 'h-12',
  };

  const rowClass = heightConfig[rowHeight];

  // Generate column widths that vary realistically
  const getColumnWidth = (colIndex: number, isHeader: boolean) => {
    const widths = ['w-1/6', 'w-1/4', 'w-1/3', 'w-1/5', 'w-2/5'];
    if (isHeader) {
      // Headers tend to be shorter
      return widths[colIndex % widths.length];
    }
    // Data cells vary more
    return widths[(colIndex + 2) % widths.length];
  };

  return (
    <div className={cn('w-full space-y-2', className)}>
      {/* Header Row */}
      {showHeader && (
        <div className="flex items-center gap-4 px-4 py-2 border-b">
          {Array.from({ length: columns }).map((_, colIndex) => (
            <div
              key={`header-${colIndex}`}
              className={cn(
                'h-4 bg-muted rounded animate-pulse',
                getColumnWidth(colIndex, true)
              )}
            />
          ))}
        </div>
      )}

      {/* Data Rows */}
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div
          key={`row-${rowIndex}`}
          className={cn(
            'flex items-center gap-4 px-4 border-b border-muted/50',
            rowClass
          )}
        >
          {Array.from({ length: columns }).map((_, colIndex) => (
            <div
              key={`cell-${rowIndex}-${colIndex}`}
              className={cn(
                'h-4 bg-muted/60 rounded animate-pulse',
                getColumnWidth(colIndex, false)
              )}
              style={{
                // Add slight delay variation for visual interest
                animationDelay: `${(rowIndex * columns + colIndex) * 50}ms`,
              }}
            />
          ))}
        </div>
      ))}
    </div>
  );
};

/**
 * SkeletonCard - Loading placeholder for card content
 */
export const SkeletonCard: React.FC<{
  className?: string;
  showImage?: boolean;
}> = ({ className, showImage = false }) => {
  return (
    <div className={cn('rounded-lg border bg-card p-4 space-y-3', className)}>
      {showImage && (
        <div className="w-full h-32 bg-muted rounded animate-pulse" />
      )}
      <div className="space-y-2">
        <div className="h-4 w-2/3 bg-muted rounded animate-pulse" />
        <div className="h-3 w-full bg-muted/60 rounded animate-pulse" />
        <div className="h-3 w-4/5 bg-muted/60 rounded animate-pulse" />
      </div>
    </div>
  );
};

/**
 * SkeletonList - Loading placeholder for list items
 */
export const SkeletonList: React.FC<{
  items?: number;
  className?: string;
}> = ({ items = 5, className }) => {
  return (
    <div className={cn('space-y-2', className)}>
      {Array.from({ length: items }).map((_, i) => (
        <div
          key={i}
          className="flex items-center gap-3 p-3 rounded-md border"
          style={{ animationDelay: `${i * 100}ms` }}
        >
          <div className="h-8 w-8 rounded-full bg-muted animate-pulse" />
          <div className="flex-1 space-y-1">
            <div className="h-4 w-1/3 bg-muted rounded animate-pulse" />
            <div className="h-3 w-1/2 bg-muted/60 rounded animate-pulse" />
          </div>
          <div className="h-6 w-16 bg-muted/40 rounded animate-pulse" />
        </div>
      ))}
    </div>
  );
};

export default SkeletonTable;
