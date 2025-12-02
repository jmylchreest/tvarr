'use client';

import { useMemo } from 'react';
import { AlertCircle, AlertTriangle } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { useProgressContext } from '@/providers/ProgressProvider';

interface OperationStatusIndicatorProps {
  resourceId: string;
  className?: string;
}

/**
 * T048/T049: Shows error or warning indicator for a resource's operation status.
 * Use this component in proxy cards and source cards to display operation failures.
 */
export function OperationStatusIndicator({ resourceId, className }: OperationStatusIndicatorProps) {
  const progressContext = useProgressContext();

  const operationState = useMemo(() => {
    const state = progressContext.getResourceState(resourceId);
    if (!state) return null;

    // Only show indicator for error or completed-with-warnings states
    if (state.state === 'error') {
      return {
        type: 'error' as const,
        message: state.error_detail?.message || state.error || 'Operation failed',
        suggestion: state.error_detail?.suggestion,
      };
    }

    if (state.state === 'completed' && state.warning_count && state.warning_count > 0) {
      return {
        type: 'warning' as const,
        message: `Completed with ${state.warning_count} warning${state.warning_count > 1 ? 's' : ''}`,
        warnings: state.warnings,
      };
    }

    return null;
  }, [progressContext, resourceId]);

  if (!operationState) {
    return null;
  }

  const isError = operationState.type === 'error';

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge
            variant={isError ? 'destructive' : 'secondary'}
            className={`${className} ${isError ? 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200' : 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'} cursor-help`}
          >
            {isError ? (
              <AlertCircle className="h-3 w-3 mr-1" />
            ) : (
              <AlertTriangle className="h-3 w-3 mr-1" />
            )}
            {isError ? 'Error' : 'Warnings'}
          </Badge>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-[300px]">
          <div className="space-y-1">
            <p className="font-medium">{operationState.message}</p>
            {'suggestion' in operationState && operationState.suggestion && (
              <p className="text-xs text-muted-foreground italic">{operationState.suggestion}</p>
            )}
            {'warnings' in operationState && operationState.warnings && operationState.warnings.length > 0 && (
              <ul className="text-xs text-muted-foreground list-disc list-inside">
                {operationState.warnings.slice(0, 3).map((warning, i) => (
                  <li key={i}>{warning}</li>
                ))}
                {operationState.warnings.length > 3 && (
                  <li>...and {operationState.warnings.length - 3} more</li>
                )}
              </ul>
            )}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
