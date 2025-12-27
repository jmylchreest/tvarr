'use client';

import React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { ClusterStats as ClusterStatsType } from '@/types/api';
import { Cpu, Zap } from 'lucide-react';

interface ClusterStatsProps {
  stats: ClusterStatsType | null;
  isLoading?: boolean;
  /** Optional action element to show at the start of the bar */
  action?: React.ReactNode;
}

export function ClusterStats({ stats, isLoading, action }: ClusterStatsProps) {
  if (isLoading) {
    return (
      <Card>
        <CardContent className="py-3 px-4">
          <div className="animate-pulse flex items-center gap-6">
            {action && <div className="h-8 w-24 bg-muted rounded" />}
            <div className="h-4 w-32 bg-muted rounded" />
            <div className="flex-1 h-2 bg-muted rounded" />
          </div>
        </CardContent>
      </Card>
    );
  }

  const totalDaemons = stats?.total_daemons ?? 0;
  const activeDaemons = stats?.active_daemons ?? 0;

  // Job slots data
  const totalJobs = stats?.total_active_jobs ?? 0;
  const cpuJobs = stats?.total_cpu_jobs ?? 0;
  const gpuJobs = stats?.total_gpu_jobs ?? 0;
  const maxJobs = stats?.max_concurrent_jobs ?? 0;
  const maxCPUJobs = stats?.max_cpu_jobs ?? 0;
  const maxGPUJobs = stats?.max_gpu_jobs ?? 0;

  // Calculate percentages relative to max_concurrent_jobs (the guard)
  const cpuPercent = maxJobs > 0 ? (cpuJobs / maxJobs) * 100 : 0;
  const gpuPercent = maxJobs > 0 ? (gpuJobs / maxJobs) * 100 : 0;
  const totalPercent = cpuPercent + gpuPercent;

  // Status color based on total utilization
  const getStatusColor = () => {
    if (totalPercent >= 100) return 'text-destructive';
    if (totalPercent >= 80) return 'text-warning';
    if (totalPercent > 0) return 'text-success';
    return 'text-muted-foreground';
  };

  const hasGPU = maxGPUJobs > 0;

  return (
    <Card>
      <CardContent className="py-3 px-4">
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          {/* Action (e.g., Connect button) */}
          {action}

          {/* Transcoders */}
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">Transcoders</span>
            <span className="text-sm font-semibold">{activeDaemons}/{totalDaemons}</span>
          </div>

          {/* Job Slots - Two-tone stacked progress bar */}
          {maxJobs > 0 && (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex items-center gap-2 flex-1 min-w-[200px]">
                  <span className="text-xs text-muted-foreground whitespace-nowrap">Job Slots</span>
                  <div className="flex-1 flex items-center gap-2">
                    {/* Stacked Progress Bar */}
                    <div className="flex-1 relative h-2 overflow-hidden rounded-full bg-secondary">
                      {/* CPU jobs (left side, blue) */}
                      {cpuJobs > 0 && (
                        <div
                          className="absolute left-0 top-0 h-full bg-info transition-all"
                          style={{ width: `${Math.min(cpuPercent, 100)}%` }}
                        />
                      )}
                      {/* GPU jobs (stacked after CPU, green) */}
                      {gpuJobs > 0 && (
                        <div
                          className="absolute top-0 h-full bg-success transition-all"
                          style={{
                            left: `${Math.min(cpuPercent, 100)}%`,
                            width: `${Math.min(gpuPercent, 100 - cpuPercent)}%`
                          }}
                        />
                      )}
                    </div>
                    <span className={cn("text-xs font-medium tabular-nums", getStatusColor())}>
                      {totalJobs}/{maxJobs}
                    </span>
                  </div>
                </div>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-xs">
                <div className="space-y-1.5 text-xs">
                  <div className="font-semibold">Cluster Job Slots</div>
                  <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
                    <div className="text-muted-foreground">Total (guard):</div>
                    <div>{totalJobs} / {maxJobs}</div>
                    <div className="flex items-center gap-1 text-muted-foreground">
                      <Cpu className="h-3 w-3" /> CPU slots:
                    </div>
                    <div>{cpuJobs} / {maxCPUJobs}</div>
                    {hasGPU && (
                      <>
                        <div className="flex items-center gap-1 text-muted-foreground">
                          <Zap className="h-3 w-3" /> GPU slots:
                        </div>
                        <div>{gpuJobs} / {maxGPUJobs}</div>
                      </>
                    )}
                  </div>
                  <div className="pt-1 text-muted-foreground border-t">
                    <span className="inline-block w-2 h-2 rounded-sm bg-info mr-1" />
                    CPU = software encoding
                    <br />
                    {hasGPU && (
                      <>
                        <span className="inline-block w-2 h-2 rounded-sm bg-success mr-1" />
                        GPU = hardware encoding
                      </>
                    )}
                  </div>
                </div>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
