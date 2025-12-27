'use client';

import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { Label } from '@/components/ui/label';
import { Card } from '@/components/ui/card';
import { RefreshCw, Pause, Play } from 'lucide-react';
import { UseAutoRefreshReturn } from '@/hooks/use-auto-refresh';
import { cn } from '@/lib/utils';

export interface RefreshControlProps {
  /** Auto-refresh hook return value */
  autoRefresh: UseAutoRefreshReturn;
  /** Whether the parent component is currently loading data */
  isLoading?: boolean;
  /** Variant: 'compact' for inline, 'card' for standalone card with more details */
  variant?: 'compact' | 'card';
  /** Additional class names */
  className?: string;
}

/**
 * Compact variant - slider with label and action button
 * Used in control bars alongside other controls
 */
function RefreshControlCompact({
  autoRefresh,
  isLoading = false,
  className,
}: RefreshControlProps) {
  const {
    refreshInterval,
    isAutoRefresh,
    stepValues,
    stepIndex,
    toggleAutoRefresh,
    manualRefresh,
    handleIntervalChange,
    getIntervalLabel,
  } = autoRefresh;

  // Paused state: interval > 0 but not running
  const isPaused = refreshInterval > 0 && !isAutoRefresh;

  return (
    <div className={cn('flex items-center gap-2', className)}>
      {/* Interval Slider */}
      <div className="flex items-center gap-2">
        <Label
          className={cn(
            'text-xs whitespace-nowrap w-8 text-right font-medium',
            isAutoRefresh ? 'text-success' : 'text-muted-foreground'
          )}
          title={
            refreshInterval === 0
              ? 'Auto-refresh off'
              : isAutoRefresh
                ? 'Auto-refresh active'
                : 'Auto-refresh paused'
          }
        >
          {getIntervalLabel()}
        </Label>
        <Slider
          value={[stepIndex]}
          onValueChange={handleIntervalChange}
          max={stepValues.length - 1}
          step={1}
          className="w-24"
        />
      </div>

      {/* Refresh/Pause/Play Button - changes based on state */}
      {refreshInterval === 0 ? (
        <Button
          variant="outline"
          size="sm"
          onClick={() => manualRefresh()}
          disabled={isLoading}
          className="h-8 w-8 p-0"
          title="Refresh"
        >
          <RefreshCw className={cn('h-4 w-4', isLoading && 'animate-spin')} />
        </Button>
      ) : (
        <Button
          variant="outline"
          size="sm"
          onClick={toggleAutoRefresh}
          className="h-8 w-8 p-0"
          title={isAutoRefresh ? 'Pause auto-refresh' : 'Resume auto-refresh'}
        >
          {isAutoRefresh ? (
            <Pause className="h-4 w-4" />
          ) : (
            <Play className="h-4 w-4" />
          )}
        </Button>
      )}
    </div>
  );
}

/**
 * Card variant - standalone card with full details
 * Used in dashboard-style layouts
 */
function RefreshControlCard({
  autoRefresh,
  isLoading = false,
  className,
}: RefreshControlProps) {
  const {
    refreshInterval,
    isAutoRefresh,
    stepValues,
    stepIndex,
    toggleAutoRefresh,
    manualRefresh,
    handleIntervalChange,
  } = autoRefresh;

  // Paused state: interval > 0 but not running
  const isPaused = refreshInterval > 0 && !isAutoRefresh;

  return (
    <Card className={cn('p-4 min-w-[300px]', className)}>
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex flex-col">
            <Label
              className={cn(
                'text-sm font-medium',
                isAutoRefresh ? 'text-success' : 'text-muted-foreground'
              )}
            >
              Update Interval: {refreshInterval === 0 ? 'Off' : `${refreshInterval}s`}
            </Label>
            <div className="flex items-center gap-2 text-xs mt-1 text-muted-foreground">
              {isLoading && (
                <>
                  <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-primary" />
                  <span>Refreshing...</span>
                </>
              )}
              {refreshInterval === 0 && !isLoading && <span>Auto-refresh disabled</span>}
              {refreshInterval > 0 && isAutoRefresh && !isLoading && <span>Auto-refresh active</span>}
              {isPaused && !isLoading && <span>Auto-refresh paused</span>}
            </div>
          </div>
          {refreshInterval === 0 ? (
            <Button
              onClick={() => manualRefresh()}
              variant="outline"
              size="sm"
              disabled={isLoading}
            >
              <RefreshCw className={cn('h-4 w-4 mr-2', isLoading && 'animate-spin')} />
              Refresh
            </Button>
          ) : (
            <Button
              onClick={toggleAutoRefresh}
              variant={isAutoRefresh ? 'default' : 'outline'}
              size="sm"
            >
              {isAutoRefresh ? (
                <>
                  <Pause className="h-4 w-4 mr-2" />
                  Pause
                </>
              ) : (
                <>
                  <Play className="h-4 w-4 mr-2" />
                  Resume
                </>
              )}
            </Button>
          )}
        </div>
        <Slider
          min={0}
          max={stepValues.length - 1}
          step={1}
          value={[stepIndex]}
          onValueChange={handleIntervalChange}
          className="w-full"
        />
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>{stepValues[0] === 0 ? 'Off' : `${stepValues[0]}s`}</span>
          <span>{stepValues[stepValues.length - 1]}s</span>
        </div>
      </div>
    </Card>
  );
}

/**
 * RefreshControl - A reusable auto-refresh control component
 *
 * @example
 * ```tsx
 * const autoRefresh = useAutoRefresh({
 *   onRefresh: fetchData,
 *   debugLabel: 'my-page',
 * });
 *
 * <RefreshControl autoRefresh={autoRefresh} isLoading={isLoading} variant="compact" />
 * ```
 */
export function RefreshControl({
  autoRefresh,
  isLoading = false,
  variant = 'compact',
  className,
}: RefreshControlProps) {
  if (variant === 'card') {
    return (
      <RefreshControlCard
        autoRefresh={autoRefresh}
        isLoading={isLoading}
        className={className}
      />
    );
  }

  return (
    <RefreshControlCompact
      autoRefresh={autoRefresh}
      isLoading={isLoading}
      className={className}
    />
  );
}
