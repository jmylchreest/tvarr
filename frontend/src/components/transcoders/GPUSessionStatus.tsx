'use client';

import { Badge } from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { GPUInfo, GPUStats } from '@/types/api';
import { cn } from '@/lib/utils';

interface GPUSessionStatusProps {
  gpu: GPUInfo;
  stats?: GPUStats;
}

export function GPUSessionStatus({ gpu, stats }: GPUSessionStatusProps) {
  const activeEncode = stats?.active_encode_sessions ?? gpu.active_encode_sessions;
  const maxEncode = gpu.max_encode_sessions;
  const encodePercent = maxEncode > 0 ? (activeEncode / maxEncode) * 100 : 0;

  const getStatusColor = () => {
    if (encodePercent >= 100) return 'text-red-500';
    if (encodePercent >= 80) return 'text-yellow-500';
    return 'text-green-500';
  };

  const getProgressColor = () => {
    if (encodePercent >= 100) return 'bg-red-500';
    if (encodePercent >= 80) return 'bg-yellow-500';
    return 'bg-green-500';
  };

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="flex items-center gap-2 min-w-0">
          <div className="flex-1 min-w-0">
            <div className="flex items-center justify-between text-xs mb-1">
              <span className="truncate font-medium">{gpu.name}</span>
              <Badge
                variant="outline"
                className={cn('ml-2 shrink-0 text-xs', getStatusColor())}
              >
                {activeEncode}/{maxEncode}
              </Badge>
            </div>
            {/* Custom progress bar with dynamic color */}
            <div className="relative h-1.5 w-full overflow-hidden rounded-full bg-secondary">
              <div
                className={cn('h-full transition-all', getProgressColor())}
                style={{ width: `${Math.min(encodePercent, 100)}%` }}
              />
            </div>
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs">
        <div className="space-y-1 text-xs">
          <div className="font-semibold">{gpu.name}</div>
          <div>Class: {gpu.class}</div>
          {gpu.driver && <div>Driver: {gpu.driver}</div>}
          <div>
            Encode Sessions: {activeEncode} / {maxEncode}
          </div>
          <div>
            Decode Sessions: {gpu.active_decode_sessions} / {gpu.max_decode_sessions}
          </div>
          {stats && (
            <>
              <div className="border-t border-border pt-1 mt-1">
                <div>GPU Utilization: {stats.utilization.toFixed(1)}%</div>
                <div>
                  Memory: {stats.memory_percent.toFixed(1)}% ({formatBytes(stats.memory_used)} / {formatBytes(stats.memory_total)})
                </div>
                <div>Temperature: {stats.temperature}C</div>
                <div>Encoder: {stats.encoder_utilization.toFixed(1)}%</div>
                <div>Decoder: {stats.decoder_utilization.toFixed(1)}%</div>
              </div>
            </>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}
