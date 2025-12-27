'use client';

import { memo, useState } from 'react';
import { Cpu, MemoryStick, Timer, Gauge, Tv, Zap, Terminal, Copy, Check } from 'lucide-react';
import { ActiveJobDetail } from '@/types/api';
import { cn } from '@/lib/utils';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';

interface ActiveJobStatsProps {
  jobs: ActiveJobDetail[];
  className?: string;
}

function formatMemory(mb: number): string {
  if (mb < 1024) {
    return `${mb.toFixed(0)} MB`;
  }
  return `${(mb / 1024).toFixed(1)} GB`;
}

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h ${minutes % 60}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  }
  return `${seconds}s`;
}

function formatSpeed(speed: number): string {
  if (speed === 0) return '0x';
  if (speed >= 1) {
    return `${speed.toFixed(1)}x`;
  }
  return `${speed.toFixed(2)}x`;
}

function getSpeedColor(speed: number): string {
  if (speed >= 1.0) return 'text-green-600 dark:text-green-400';
  if (speed >= 0.5) return 'text-yellow-600 dark:text-yellow-400';
  return 'text-red-600 dark:text-red-400';
}

function getCpuColor(cpu: number): string {
  if (cpu < 50) return 'text-green-600 dark:text-green-400';
  if (cpu < 80) return 'text-yellow-600 dark:text-yellow-400';
  return 'text-red-600 dark:text-red-400';
}

function formatHwAccel(hwAccel?: string, hwDevice?: string): string {
  if (!hwAccel) return 'CPU';
  // Show friendly names for hw accel types
  const labels: Record<string, string> = {
    vaapi: 'VAAPI',
    cuda: 'CUDA',
    nvenc: 'NVENC',
    qsv: 'QSV',
    videotoolbox: 'VT',
  };
  const label = labels[hwAccel] || hwAccel.toUpperCase();
  // For device paths, show just the device name (e.g. renderD128 from /dev/dri/renderD128)
  if (hwDevice) {
    const deviceName = hwDevice.split('/').pop() || hwDevice;
    return `${label} (${deviceName})`;
  }
  return label;
}

function FFmpegCommandDisplay({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy command:', err);
    }
  };

  // Truncate command for display
  const truncatedCommand = command.length > 60
    ? command.substring(0, 60) + '...'
    : command;

  return (
    <TooltipProvider>
      <Tooltip delayDuration={300}>
        <TooltipTrigger asChild>
          <div className="flex items-center gap-1.5 mt-2 pt-2 border-t border-border/50">
            <Terminal className="h-3 w-3 text-muted-foreground shrink-0" />
            <code className="text-[10px] text-muted-foreground font-mono truncate flex-1 bg-muted/50 px-1.5 py-0.5 rounded">
              {truncatedCommand}
            </code>
            <button
              onClick={handleCopy}
              className="p-1 hover:bg-muted rounded shrink-0 transition-colors"
              title={copied ? 'Copied!' : 'Copy command'}
            >
              {copied ? (
                <Check className="h-3 w-3 text-green-500" />
              ) : (
                <Copy className="h-3 w-3 text-muted-foreground hover:text-foreground" />
              )}
            </button>
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-[600px]">
          <code className="text-xs font-mono whitespace-pre-wrap break-all">
            {command}
          </code>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function ActiveJobStats({ jobs, className }: ActiveJobStatsProps) {
  if (!jobs || jobs.length === 0) {
    return (
      <div className={cn('text-sm text-muted-foreground italic py-2', className)}>
        No active transcode jobs
      </div>
    );
  }

  return (
    <div className={cn('space-y-2', className)}>
      {jobs.map((job) => (
        <div
          key={job.id}
          className="rounded-lg border bg-card p-3 space-y-2"
        >
          {/* Job header - channel name and hw accel */}
          <div className="flex items-center gap-2">
            <Tv className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            <span className="text-sm font-medium truncate flex-1">
              {job.channel_name || 'Unknown Channel'}
            </span>
            <div className={cn(
              'flex items-center gap-1 px-1.5 py-0.5 rounded text-xs font-medium shrink-0',
              job.hw_accel ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400' : 'bg-muted text-muted-foreground'
            )}>
              <Zap className="h-3 w-3" />
              {formatHwAccel(job.hw_accel, job.hw_device)}
            </div>
          </div>

          {/* Stats grid */}
          <div className="grid grid-cols-2 gap-x-4 gap-y-1.5">
            {/* CPU */}
            <div className="flex items-center gap-1.5">
              <Cpu className="h-3 w-3 text-blue-600 dark:text-blue-400 shrink-0" />
              <span className="text-xs text-muted-foreground">CPU</span>
              <span className={cn('text-xs tabular-nums ml-auto', getCpuColor(job.cpu_percent))}>
                {job.cpu_percent.toFixed(0)}%
              </span>
            </div>

            {/* Memory */}
            <div className="flex items-center gap-1.5">
              <MemoryStick className="h-3 w-3 text-green-600 dark:text-green-400 shrink-0" />
              <span className="text-xs text-muted-foreground">Mem</span>
              <span className="text-xs tabular-nums text-green-600 dark:text-green-400 ml-auto">
                {formatMemory(job.memory_mb)}
              </span>
            </div>

            {/* Speed */}
            <div className="flex items-center gap-1.5">
              <Gauge className="h-3 w-3 text-purple-600 dark:text-purple-400 shrink-0" />
              <span className="text-xs text-muted-foreground">Speed</span>
              <span className={cn('text-xs tabular-nums ml-auto', getSpeedColor(job.encoding_speed))}>
                {formatSpeed(job.encoding_speed)}
              </span>
            </div>

            {/* Running time */}
            <div className="flex items-center gap-1.5">
              <Timer className="h-3 w-3 text-orange-600 dark:text-orange-400 shrink-0" />
              <span className="text-xs text-muted-foreground">Time</span>
              <span className="text-xs tabular-nums text-orange-600 dark:text-orange-400 ml-auto">
                {formatDuration(job.running_time_ms)}
              </span>
            </div>
          </div>

          {/* FFmpeg command display */}
          {job.ffmpeg_command && (
            <FFmpegCommandDisplay command={job.ffmpeg_command} />
          )}
        </div>
      ))}
    </div>
  );
}

export default memo(ActiveJobStats);
export { ActiveJobStats };
