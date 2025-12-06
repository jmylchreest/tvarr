'use client';

import React from 'react';
import { StateDurationSummary as StateDurationSummaryType, formatDuration } from '@/types/circuit-breaker';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { PieChart } from 'lucide-react';

interface StateDurationSummaryProps {
  durations: StateDurationSummaryType;
  className?: string;
}

export function StateDurationSummary({ durations, className = '' }: StateDurationSummaryProps) {
  const totalMs = durations.total_duration_ms ||
    (durations.closed_duration_ms + durations.open_duration_ms + durations.half_open_duration_ms);

  if (totalMs === 0) {
    return (
      <div className={`text-xs text-muted-foreground ${className}`}>No duration data available</div>
    );
  }

  const segments = [
    {
      label: 'Closed',
      duration: durations.closed_duration_ms,
      percentage: durations.closed_percentage,
      color: 'bg-green-500',
      textColor: 'text-green-600 dark:text-green-400',
    },
    {
      label: 'Open',
      duration: durations.open_duration_ms,
      percentage: durations.open_percentage,
      color: 'bg-red-500',
      textColor: 'text-red-600 dark:text-red-400',
    },
    {
      label: 'Half-Open',
      duration: durations.half_open_duration_ms,
      percentage: durations.half_open_percentage,
      color: 'bg-amber-500',
      textColor: 'text-amber-600 dark:text-amber-400',
    },
  ];

  return (
    <div className={`space-y-2 ${className}`}>
      <div className="text-xs font-medium text-muted-foreground flex items-center gap-1">
        <PieChart className="h-3 w-3" />
        State Duration
      </div>

      {/* Horizontal bar showing distribution */}
      <div className="h-2 bg-muted rounded-full overflow-hidden flex">
        {segments.map(
          (segment) =>
            segment.percentage > 0 && (
              <Tooltip key={segment.label}>
                <TooltipTrigger asChild>
                  <div
                    className={`h-full ${segment.color} cursor-help transition-all`}
                    style={{ width: `${segment.percentage}%` }}
                  />
                </TooltipTrigger>
                <TooltipContent>
                  <div className="text-xs">
                    <div className="font-medium">{segment.label}</div>
                    <div>{formatDuration(segment.duration)}</div>
                    <div>{segment.percentage.toFixed(1)}%</div>
                  </div>
                </TooltipContent>
              </Tooltip>
            )
        )}
      </div>

      {/* Legend with values */}
      <div className="grid grid-cols-3 gap-2 text-xs">
        {segments.map((segment) => (
          <div key={segment.label} className="text-center">
            <div className={`font-medium ${segment.textColor}`}>{segment.percentage.toFixed(1)}%</div>
            <div className="text-muted-foreground">{segment.label}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

// Compact inline version
export function StateDurationSummaryInline({
  durations,
  className = '',
}: StateDurationSummaryProps) {
  return (
    <div className={`flex gap-3 text-xs ${className}`}>
      <span className="text-green-600 dark:text-green-400">
        {durations.closed_percentage.toFixed(0)}% closed
      </span>
      {durations.open_percentage > 0 && (
        <span className="text-red-600 dark:text-red-400">
          {durations.open_percentage.toFixed(0)}% open
        </span>
      )}
      {durations.half_open_percentage > 0 && (
        <span className="text-amber-600 dark:text-amber-400">
          {durations.half_open_percentage.toFixed(0)}% half-open
        </span>
      )}
    </div>
  );
}
