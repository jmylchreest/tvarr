'use client';

import React from 'react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ErrorCategoryCount } from '@/types/circuit-breaker';

interface SegmentedProgressBarProps {
  errorCounts: ErrorCategoryCount;
  totalRequests: number;
  className?: string;
}

interface Segment {
  key: string;
  label: string;
  count: number;
  percentage: number;
  color: string;
  bgColor: string;
}

export function SegmentedProgressBar({
  errorCounts,
  totalRequests,
  className = '',
}: SegmentedProgressBarProps) {
  if (totalRequests === 0) {
    return (
      <div className={`h-3 bg-muted rounded-full overflow-hidden ${className}`}>
        <div className="h-full w-full flex items-center justify-center text-xs text-muted-foreground">
          No requests yet
        </div>
      </div>
    );
  }

  const segments: Segment[] = [
    {
      key: 'success',
      label: 'Success (2xx)',
      count: errorCounts.success_2xx,
      percentage: (errorCounts.success_2xx / totalRequests) * 100,
      color: 'bg-green-500',
      bgColor: 'bg-green-500/80',
    },
    {
      key: 'server_error',
      label: 'Server Error (5xx)',
      count: errorCounts.server_error_5xx,
      percentage: (errorCounts.server_error_5xx / totalRequests) * 100,
      color: 'bg-red-500',
      bgColor: 'bg-red-500/80',
    },
    {
      key: 'client_error',
      label: 'Client Error (4xx)',
      count: errorCounts.client_error_4xx,
      percentage: (errorCounts.client_error_4xx / totalRequests) * 100,
      color: 'bg-orange-500',
      bgColor: 'bg-orange-500/80',
    },
    {
      key: 'timeout',
      label: 'Timeout',
      count: errorCounts.timeout,
      percentage: (errorCounts.timeout / totalRequests) * 100,
      color: 'bg-yellow-500',
      bgColor: 'bg-yellow-500/80',
    },
    {
      key: 'network',
      label: 'Network Error',
      count: errorCounts.network_error,
      percentage: (errorCounts.network_error / totalRequests) * 100,
      color: 'bg-gray-500',
      bgColor: 'bg-gray-500/80',
    },
  ].filter((s) => s.count > 0);

  const successRate = (errorCounts.success_2xx / totalRequests) * 100;

  return (
    <div className={`space-y-1 ${className}`}>
      <div className="h-3 bg-muted rounded-full overflow-hidden flex">
        {segments.map((segment) => (
          <Tooltip key={segment.key}>
            <TooltipTrigger asChild>
              <div
                className={`h-full ${segment.bgColor} transition-all duration-300 cursor-help`}
                style={{ width: `${segment.percentage}%` }}
              />
            </TooltipTrigger>
            <TooltipContent>
              <div className="text-xs">
                <div className="font-medium">{segment.label}</div>
                <div>
                  {segment.count.toLocaleString()} ({segment.percentage.toFixed(1)}%)
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        ))}
      </div>
      <div className="flex justify-between text-xs text-muted-foreground">
        <span>{successRate.toFixed(1)}% healthy</span>
        <span>{totalRequests.toLocaleString()} total</span>
      </div>
    </div>
  );
}

// Legend component for the segmented bar
export function SegmentedProgressBarLegend() {
  const legendItems = [
    { label: 'Success', color: 'bg-green-500' },
    { label: '5xx', color: 'bg-red-500' },
    { label: '4xx', color: 'bg-orange-500' },
    { label: 'Timeout', color: 'bg-yellow-500' },
    { label: 'Network', color: 'bg-gray-500' },
  ];

  return (
    <div className="flex flex-wrap gap-3 text-xs">
      {legendItems.map((item) => (
        <div key={item.label} className="flex items-center gap-1">
          <div className={`w-2 h-2 rounded-full ${item.color}`} />
          <span className="text-muted-foreground">{item.label}</span>
        </div>
      ))}
    </div>
  );
}
