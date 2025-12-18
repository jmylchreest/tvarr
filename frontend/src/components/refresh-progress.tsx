'use client';

import { useEffect, useState } from 'react';
import { Progress } from '@/components/ui/progress';
import { Badge } from '@/components/ui/badge';
import { AlertCircle, AlertTriangle, CheckCircle, Loader2 } from 'lucide-react';
import { useProgressContext, ProgressEvent } from '@/providers/ProgressProvider';
import { ErrorDetail } from '@/types/api';
import { Debug } from '@/utils/debug';

interface RefreshProgressProps {
  sourceId: string;
  isActive: boolean;
  onComplete?: () => void;
}

interface ProgressState {
  state:
    | 'idle'
    | 'processing'
    | 'completed'
    | 'error'
    | 'preparing'
    | 'connecting'
    | 'downloading'
    | 'saving'
    | 'cleanup'
    | 'cancelled';
  percentage: number;
  message?: string;
  operationId?: string;
  // T045: Error detail for structured error display
  errorDetail?: ErrorDetail;
  // T050: Warning tracking
  warningCount?: number;
  warnings?: string[];
}

export function RefreshProgress({ sourceId, isActive, onComplete }: RefreshProgressProps) {
  const progressContext = useProgressContext();
  const [progress, setProgress] = useState<ProgressState>({
    state: 'idle',
    percentage: 0,
  });

  // Helper to convert ProgressEvent to ProgressState
  const eventToState = (event: ProgressEvent): ProgressState => {
    const currentStage = event.stages.find((s) => s.id === event.current_stage);
    return {
      state: event.state,
      percentage: event.overall_percentage,
      message: currentStage?.stage_step || 'Unknown',
      operationId: event.id,
      errorDetail: event.error_detail,
      warningCount: event.warning_count,
      warnings: event.warnings,
    };
  };

  useEffect(() => {
    if (!isActive) {
      setProgress({ state: 'idle', percentage: 0 });
      return;
    }

    Debug.log(`[RefreshProgress] Starting SSE for source ${sourceId}`);

    // Check for existing state from context (handles fast operations that complete before subscription)
    const existingState = progressContext.getResourceState(sourceId);
    if (existingState) {
      Debug.log(`[RefreshProgress] Found existing state for source ${sourceId}:`, existingState.state);
      const newState = eventToState(existingState);
      setProgress(newState);

      // If already completed/error, trigger callback after delay
      if ((existingState.state === 'completed' || existingState.state === 'error') && onComplete) {
        Debug.log(`[RefreshProgress] Existing operation already ${existingState.state} for source ${sourceId}`);
        setTimeout(() => {
          onComplete();
          setProgress({ state: 'idle', percentage: 0 });
        }, 2000);
        return; // No need to subscribe for a completed operation
      }
    }

    // SSE connection is managed by ProgressProvider
    // We don't need to check connection status here

    const handleProgressEvent = (event: ProgressEvent) => {
      Debug.log('[RefreshProgress] Received SSE event:', event);

      // Only handle events for this specific source - use owner_id to match the source
      if (event.owner_id !== sourceId) {
        Debug.log(
          `[RefreshProgress] Ignoring event for different source: ${event.owner_id} vs ${sourceId}`
        );
        return;
      }

      Debug.log(
        `[RefreshProgress] Processing event for source ${sourceId}:`,
        event.state,
        event.overall_percentage
      );

      const newState = eventToState(event);
      setProgress(newState);

      // Call completion callback if operation finished
      if ((event.state === 'completed' || event.state === 'error') && onComplete) {
        Debug.log(`[RefreshProgress] Operation ${event.state} for source ${sourceId}`);
        setTimeout(() => {
          onComplete();
          setProgress({ state: 'idle', percentage: 0 });
        }, 2000); // Show final state for 2 seconds before hiding
      }
    };

    const unsubscribe = progressContext.subscribe(sourceId, handleProgressEvent);
    return unsubscribe;
  }, [sourceId, isActive, onComplete, progressContext]);

  if (progress.state === 'idle') {
    return null;
  }

  const getStatusVariant = (): 'default' | 'secondary' | 'destructive' | 'outline' => {
    switch (progress.state) {
      case 'error':
        return 'destructive';
      case 'idle':
      case 'processing':
      case 'completed':
      default:
        return 'secondary';
    }
  };

  const getStatusIcon = () => {
    switch (progress.state) {
      case 'idle':
      case 'processing':
        return <Loader2 className="h-3 w-3 animate-spin" />;
      case 'completed':
        // T050: Show warning icon if there are warnings
        if (progress.warningCount && progress.warningCount > 0) {
          return <AlertTriangle className="h-3 w-3" />;
        }
        return <CheckCircle className="h-3 w-3" />;
      case 'error':
        return <AlertCircle className="h-3 w-3" />;
      default:
        return null;
    }
  };

  // Helper to truncate long text for status badge display
  const truncateText = (text: string, maxLength: number = 80): string => {
    if (text.length <= maxLength) return text;
    return text.slice(0, maxLength - 3) + '...';
  };

  const getStatusText = () => {
    switch (progress.state) {
      case 'idle':
        return progress.message || 'Initializing...';
      case 'processing':
        return progress.message || 'Processing...';
      case 'completed':
        // T050: Show warning count if present
        if (progress.warningCount && progress.warningCount > 0) {
          return `Completed with ${progress.warningCount} warning${progress.warningCount > 1 ? 's' : ''}`;
        }
        return 'Refresh completed';
      case 'error':
        // T047: Show truncated error message - full error is shown in toast
        if (progress.errorDetail?.message) {
          return truncateText(progress.errorDetail.message);
        }
        return progress.message || 'Refresh failed';
      default:
        return '';
    }
  };

  return (
    <div className="space-y-2 p-2 border rounded-md bg-background">
      <div className="flex items-center justify-between">
        <Badge variant={getStatusVariant()}>
          {getStatusIcon()}
          <span className="ml-1 text-xs">{getStatusText()}</span>
        </Badge>
        {progress.percentage > 0 && (
          <span className="text-xs text-muted-foreground">{progress.percentage}%</span>
        )}
      </div>

      {progress.state === 'processing' && (
        <Progress value={progress.percentage} className="h-1.5" />
      )}

      {progress.message && progress.state !== 'completed' && progress.state !== 'error' && (
        <div className="text-xs text-muted-foreground">{progress.message}</div>
      )}

      {/* T047: Show error suggestion if available */}
      {progress.state === 'error' && progress.errorDetail?.suggestion && (
        <div className="text-xs text-muted-foreground italic">
          {progress.errorDetail.suggestion}
        </div>
      )}

      {/* T050: Show warning indicator when warnings are present */}
      {progress.state === 'completed' &&
        progress.warningCount != null &&
        progress.warningCount > 0 && (
          <div className="flex items-start gap-1 text-xs text-amber-600">
            <AlertTriangle className="h-3 w-3 mt-0.5 flex-shrink-0" />
            <div>
              {progress.warnings && progress.warnings.length > 0 ? (
                <ul className="list-disc list-inside">
                  {progress.warnings.slice(0, 3).map((warning, i) => (
                    <li key={i}>{warning}</li>
                  ))}
                  {progress.warnings.length > 3 && (
                    <li>...and {progress.warnings.length - 3} more</li>
                  )}
                </ul>
              ) : (
                <span>{progress.warningCount} warning(s) occurred</span>
              )}
            </div>
          </div>
        )}
    </div>
  );
}
