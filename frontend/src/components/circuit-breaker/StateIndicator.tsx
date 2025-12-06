'use client';

import React from 'react';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { CircuitState, formatDuration, getStateColorClasses } from '@/types/circuit-breaker';
import { Clock, AlertTriangle, CheckCircle, RefreshCw } from 'lucide-react';

interface StateIndicatorProps {
  state: CircuitState;
  stateEnteredAt: string;
  resetTimeout?: string;
  consecutiveFailures?: number;
  failureThreshold?: number;
  className?: string;
}

export function StateIndicator({
  state,
  stateEnteredAt,
  resetTimeout,
  consecutiveFailures = 0,
  failureThreshold = 5,
  className = '',
}: StateIndicatorProps) {
  const stateColorClasses = getStateColorClasses(state);
  const enteredAt = new Date(stateEnteredAt);
  const now = new Date();
  const durationMs = now.getTime() - enteredAt.getTime();
  const durationStr = formatDuration(durationMs);

  const getStateIcon = () => {
    switch (state) {
      case 'closed':
        return <CheckCircle className="h-3.5 w-3.5" />;
      case 'open':
        return <AlertTriangle className="h-3.5 w-3.5" />;
      case 'half-open':
        return <RefreshCw className="h-3.5 w-3.5" />;
      default:
        return null;
    }
  };

  const getStateDescription = () => {
    switch (state) {
      case 'closed':
        return 'Circuit is closed. Requests are flowing normally.';
      case 'open':
        return `Circuit is open. Requests are being rejected. Will attempt recovery after ${resetTimeout || 'timeout'}.`;
      case 'half-open':
        return 'Circuit is testing. Limited requests allowed to check if service has recovered.';
      default:
        return 'Unknown state';
    }
  };

  // Calculate time until half-open (for open state)
  const getRecoveryInfo = () => {
    if (state !== 'open' || !resetTimeout) return null;

    // Parse reset timeout (e.g., "30s", "1m")
    const match = resetTimeout.match(/^(\d+)(ms|s|m|h)$/);
    if (!match) return null;

    const value = parseInt(match[1]);
    const unit = match[2];
    let timeoutMs = value;
    switch (unit) {
      case 's':
        timeoutMs = value * 1000;
        break;
      case 'm':
        timeoutMs = value * 60 * 1000;
        break;
      case 'h':
        timeoutMs = value * 60 * 60 * 1000;
        break;
    }

    const remainingMs = timeoutMs - durationMs;
    if (remainingMs <= 0) return 'Recovery imminent...';

    return `Recovery attempt in ${formatDuration(remainingMs)}`;
  };

  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge className={`${stateColorClasses} flex items-center gap-1.5 cursor-help`}>
            {getStateIcon()}
            <span className="capitalize">{state}</span>
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          <div className="text-xs space-y-1 max-w-xs">
            <div className="font-medium">{getStateDescription()}</div>
            {state === 'open' && resetTimeout && (
              <div className="text-muted-foreground">{getRecoveryInfo()}</div>
            )}
          </div>
        </TooltipContent>
      </Tooltip>

      <div className="flex items-center gap-1 text-xs text-muted-foreground">
        <Clock className="h-3 w-3" />
        <span>for {durationStr}</span>
      </div>

      {state === 'closed' && (
        <div className="text-xs text-muted-foreground">
          <span className="font-medium">{consecutiveFailures}</span>
          <span>/{failureThreshold} failures</span>
        </div>
      )}
    </div>
  );
}
