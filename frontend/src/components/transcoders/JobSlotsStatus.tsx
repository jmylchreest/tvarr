'use client';

import { Badge } from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Daemon } from '@/types/api';
import { cn } from '@/lib/utils';
import { Cpu, Zap } from 'lucide-react';

interface JobSlotsStatusProps {
  daemon: Daemon;
}

/**
 * JobSlotsStatus displays a stacked two-tone progress bar showing CPU and GPU job usage.
 *
 * The bar width represents max_concurrent_jobs (the guard limit).
 * CPU jobs fill from the left in blue, GPU jobs stack on top in green.
 * Details below show individual slot usage.
 */
export function JobSlotsStatus({ daemon }: JobSlotsStatusProps) {
  const capabilities = daemon.capabilities;
  const maxJobs = capabilities?.max_concurrent_jobs ?? 4;

  // Get limits with sensible defaults
  const maxCPUJobs = capabilities?.max_cpu_jobs || maxJobs;
  const maxGPUJobs = capabilities?.max_gpu_jobs || 0;

  // Current usage
  const activeCPU = daemon.active_cpu_jobs ?? 0;
  const activeGPU = daemon.active_gpu_jobs ?? 0;
  const totalActive = daemon.active_jobs ?? (activeCPU + activeGPU);

  // Calculate percentages relative to max_jobs (the guard)
  const cpuPercent = maxJobs > 0 ? (activeCPU / maxJobs) * 100 : 0;
  const gpuPercent = maxJobs > 0 ? (activeGPU / maxJobs) * 100 : 0;
  const totalPercent = cpuPercent + gpuPercent;

  // Status colors based on total utilization
  const getStatusColor = () => {
    if (totalPercent >= 100) return 'text-destructive';
    if (totalPercent >= 80) return 'text-warning';
    if (totalPercent > 0) return 'text-success';
    return 'text-muted-foreground';
  };

  // Determine if we have any GPU capacity
  const hasGPU = maxGPUJobs > 0;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="space-y-2">
          {/* Header with job count */}
          <div className="flex items-center justify-between text-xs">
            <span className="font-medium">Job Slots</span>
            <Badge
              variant="outline"
              className={cn('shrink-0 text-xs', getStatusColor())}
            >
              {totalActive}/{maxJobs}
            </Badge>
          </div>

          {/* Stacked Progress Bar */}
          <div className="relative h-2 w-full overflow-hidden rounded-full bg-secondary">
            {/* CPU jobs (left side, blue) */}
            {activeCPU > 0 && (
              <div
                className="absolute left-0 top-0 h-full bg-info transition-all"
                style={{ width: `${Math.min(cpuPercent, 100)}%` }}
              />
            )}
            {/* GPU jobs (stacked after CPU, green) */}
            {activeGPU > 0 && (
              <div
                className="absolute top-0 h-full bg-success transition-all"
                style={{
                  left: `${Math.min(cpuPercent, 100)}%`,
                  width: `${Math.min(gpuPercent, 100 - cpuPercent)}%`
                }}
              />
            )}
          </div>

          {/* Compact details row */}
          <div className="flex items-center gap-3 text-[10px] text-muted-foreground">
            <div className="flex items-center gap-1">
              <div className="h-2 w-2 rounded-sm bg-info" />
              <Cpu className="h-2.5 w-2.5" />
              <span>{activeCPU}/{maxCPUJobs}</span>
            </div>
            {hasGPU && (
              <div className="flex items-center gap-1">
                <div className="h-2 w-2 rounded-sm bg-success" />
                <Zap className="h-2.5 w-2.5" />
                <span>{activeGPU}/{maxGPUJobs}</span>
              </div>
            )}
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs">
        <div className="space-y-1.5 text-xs">
          <div className="font-semibold">Job Slot Usage</div>
          <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
            <div className="text-muted-foreground">Total (guard):</div>
            <div>{totalActive} / {maxJobs}</div>
            <div className="flex items-center gap-1 text-muted-foreground">
              <Cpu className="h-3 w-3" /> CPU slots:
            </div>
            <div>{activeCPU} / {maxCPUJobs}</div>
            {hasGPU && (
              <>
                <div className="flex items-center gap-1 text-muted-foreground">
                  <Zap className="h-3 w-3" /> GPU slots:
                </div>
                <div>{activeGPU} / {maxGPUJobs}</div>
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
  );
}
