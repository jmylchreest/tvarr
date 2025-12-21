'use client';

import { Badge } from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { GPUInfo } from '@/types/api';
import { cn } from '@/lib/utils';

interface GPUSessionStatusProps {
  gpu: GPUInfo;
}

/**
 * GPUSessionStatus displays GPU encode session availability.
 *
 * Session counts are the key metric for job scheduling - they tell you
 * if a GPU can accept more work. Runtime utilization metrics are not
 * collected as they add complexity without improving scheduling decisions.
 */
export function GPUSessionStatus({ gpu }: GPUSessionStatusProps) {
  const activeEncode = gpu.active_encode_sessions;
  const maxEncode = gpu.max_encode_sessions;
  const encodePercent = maxEncode > 0 ? (activeEncode / maxEncode) * 100 : 0;

  const getStatusColor = () => {
    if (encodePercent >= 100) return 'text-destructive';
    if (encodePercent >= 80) return 'text-warning';
    return 'text-success';
  };

  const getProgressColor = () => {
    if (encodePercent >= 100) return 'bg-destructive';
    if (encodePercent >= 80) return 'bg-warning';
    return 'bg-success';
  };

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="space-y-2">
          {/* GPU Name and Session Count */}
          <div className="flex items-center justify-between text-xs">
            <span className="truncate font-medium">{gpu.name}</span>
            <Badge
              variant="outline"
              className={cn('ml-2 shrink-0 text-xs', getStatusColor())}
            >
              {activeEncode}/{maxEncode}
            </Badge>
          </div>

          {/* Session Progress Bar */}
          <div className="relative h-1.5 w-full overflow-hidden rounded-full bg-secondary">
            <div
              className={cn('h-full transition-all', getProgressColor())}
              style={{ width: `${Math.min(encodePercent, 100)}%` }}
            />
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
          {gpu.max_decode_sessions > 0 && (
            <div>
              Decode Sessions: {gpu.active_decode_sessions} / {gpu.max_decode_sessions}
            </div>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}
