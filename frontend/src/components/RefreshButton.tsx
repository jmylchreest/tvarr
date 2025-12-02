'use client';

import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Progress } from '@/components/ui/progress';
import { RefreshCw, AlertCircle, CheckCircle, Clock } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useProgressState, formatProgress } from '@/hooks/useProgressState';
import { useConflictHandler } from '@/hooks/useConflictHandler';
import { ConflictNotification } from '@/components/ConflictNotification';
import { useEffect, useRef } from 'react';

interface RefreshButtonProps {
  resourceId: string;
  onRefresh: () => Promise<void> | void;
  onComplete?: () => void;
  disabled?: boolean;
  size?: 'sm' | 'default' | 'lg';
  variant?: 'default' | 'outline' | 'ghost';
  className?: string;
  tooltipText?: string;
}

export function RefreshButton({
  resourceId,
  onRefresh,
  onComplete,
  disabled = false,
  size = 'sm',
  variant = 'ghost',
  className,
  tooltipText = 'Refresh',
}: RefreshButtonProps) {
  const progressState = useProgressState(resourceId);
  const { handleApiError, dismissConflict, getConflictState } = useConflictHandler();
  const lastEventRef = useRef<typeof progressState.event>(null);

  const conflictState = getConflictState(resourceId);

  const isRefreshing = progressState.isActive;
  const hasError = progressState.hasError;

  // Call onComplete when operation finishes
  useEffect(() => {
    const currentEvent = progressState.event;

    // Check for new completed events (different event ID than last one we saw)
    if (currentEvent && currentEvent.state === 'completed' && onComplete) {
      const lastEvent = lastEventRef.current;
      const isNewEvent = !lastEvent || lastEvent.id !== currentEvent.id;

      if (isNewEvent) {
        onComplete();
      }
    }

    lastEventRef.current = currentEvent;
  }, [progressState.event, onComplete, resourceId]);
  const isCompleted = progressState.isCompleted;

  // Determine button state and styling (icon spins, button stays static & transparent)
  const buttonState = {
    icon: hasError ? AlertCircle : isCompleted ? CheckCircle : RefreshCw,
    className: cn(
      hasError && 'text-destructive hover:text-destructive',
      isCompleted && 'text-green-600 hover:text-green-700',
      // Ensure no background / rotation effect on container
      'p-0 h-8 w-8 rounded-full bg-transparent hover:bg-transparent',
      className
    ),
    disabled: disabled || isRefreshing,
  };
  const iconClass = cn('h-4 w-4', isRefreshing && 'animate-spin');

  // Build tooltip content
  const tooltipContent = () => {
    if (!progressState.event) {
      return tooltipText;
    }

    const { event, currentStep, duration, progress, stages, operationName, error } = progressState;

    return (
      <div className="space-y-2 max-w-sm">
        <div className="font-medium">{operationName || 'Processing'}</div>

        <div className="space-y-1 text-sm">
          <div className="flex justify-between">
            <span>Status:</span>
            <span
              className={cn(
                'font-medium',
                event.state === 'completed' && 'text-green-600',
                event.state === 'error' && 'text-destructive',
                event.state === 'processing' && 'text-blue-600'
              )}
            >
              {event.state.charAt(0).toUpperCase() + event.state.slice(1)}
            </span>
          </div>

          {currentStep && (
            <div className="flex justify-between">
              <span>Step:</span>
              <span className="font-medium">{currentStep}</span>
            </div>
          )}

          {progress && (
            <div className="space-y-2">
              <div className="flex justify-between">
                <span>Overall Progress:</span>
                <span className="font-medium">{formatProgress(progress)}</span>
              </div>
              <Progress value={progress.overall_percentage} className="h-2" />
            </div>
          )}

          {stages && (
            <div className="space-y-2">
              <div className="flex justify-between">
                <span>Stage:</span>
                <span className="font-medium">
                  {stages.currentStageName || stages.currentStage || 'Unknown'}
                  {stages.completedStages !== null && stages.totalStages !== null && (
                    <span className="text-muted-foreground ml-1">
                      ({stages.completedStages + 1}/{stages.totalStages})
                    </span>
                  )}
                </span>
              </div>
              {stages.stageProgressPercentage !== null && (
                <div className="space-y-1">
                  <div className="flex justify-between text-xs">
                    <span>Stage Progress:</span>
                    <span>{stages.stageProgressPercentage.toFixed(1)}%</span>
                  </div>
                  <Progress value={stages.stageProgressPercentage} className="h-2" />
                </div>
              )}
            </div>
          )}

          <div className="flex justify-between">
            <span>Duration:</span>
            <span className="font-medium flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {duration}
            </span>
          </div>

          {error && (
            <div className="pt-1 border-t">
              <div className="text-destructive text-xs">
                <div className="font-medium">Error:</div>
                <div className="mt-1">{error}</div>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  };

  const IconComponent = buttonState.icon;

  const handleRefreshClick = async () => {
    try {
      await onRefresh();
    } catch (error) {
      // Check if it's a 409 conflict and handle it
      if (!handleApiError(error, resourceId, 'Refresh')) {
        // If not a conflict, re-throw for other error handling
        throw error;
      }
    }
  };

  return (
    <TooltipProvider>
      <ConflictNotification
        show={conflictState.show}
        message={conflictState.message}
        onDismiss={() => dismissConflict(resourceId)}
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size={size}
              variant={variant}
              onClick={handleRefreshClick}
              disabled={buttonState.disabled}
              className={buttonState.className}
            >
              <IconComponent className={iconClass} />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top" className="max-w-sm">
            {tooltipContent()}
          </TooltipContent>
        </Tooltip>
      </ConflictNotification>
    </TooltipProvider>
  );
}
