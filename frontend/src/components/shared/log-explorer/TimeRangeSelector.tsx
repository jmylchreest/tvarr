'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { useLogExplorer } from './LogExplorerContext';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Clock, ChevronDown, X } from 'lucide-react';
import { TIME_RANGE_PRESETS } from './types';

interface TimeRangeSelectorProps {
  className?: string;
}

/**
 * Gets a display label for the current time range.
 */
function getTimeRangeLabel(timeRange: { preset?: string; from?: Date; to?: Date }): string {
  if (timeRange.preset) {
    const preset = TIME_RANGE_PRESETS.find((p) => p.value === timeRange.preset);
    return preset?.label || timeRange.preset;
  }

  if (timeRange.from && timeRange.to) {
    return 'Custom range';
  }

  return 'All time';
}

/**
 * Component for selecting time range filters.
 */
export function TimeRangeSelector({ className }: TimeRangeSelectorProps) {
  const { timeRange, setTimeRange } = useLogExplorer();

  const handlePresetSelect = (presetValue: string) => {
    setTimeRange({ preset: presetValue });
  };

  const handleClearTimeRange = () => {
    setTimeRange({});
  };

  const currentLabel = getTimeRangeLabel(timeRange);
  const hasTimeFilter = timeRange.preset || timeRange.from || timeRange.to;

  return (
    <div className={cn('flex items-center gap-1', className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className={cn(
              'h-8 gap-1.5',
              hasTimeFilter && 'border-primary/50'
            )}
          >
            <Clock className="h-3.5 w-3.5" />
            <span className="text-sm">{currentLabel}</span>
            <ChevronDown className="h-3 w-3 opacity-50" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-48">
          {TIME_RANGE_PRESETS.map((preset) => (
            <DropdownMenuItem
              key={preset.value}
              onClick={() => handlePresetSelect(preset.value)}
              className={cn(
                'cursor-pointer',
                timeRange.preset === preset.value && 'bg-muted'
              )}
            >
              {preset.label}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={handleClearTimeRange}
            className="cursor-pointer"
          >
            All time
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {hasTimeFilter && (
        <Button
          variant="ghost"
          size="icon"
          onClick={handleClearTimeRange}
          className="h-8 w-8"
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
}
