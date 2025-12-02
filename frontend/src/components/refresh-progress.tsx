'use client';

import { useEffect, useState } from 'react';
import { Progress } from '@/components/ui/progress';
import { Badge } from '@/components/ui/badge';
import { AlertCircle, CheckCircle, Loader2 } from 'lucide-react';
import { useProgressContext, ProgressEvent } from '@/providers/ProgressProvider';
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
}

export function RefreshProgress({ sourceId, isActive, onComplete }: RefreshProgressProps) {
  const progressContext = useProgressContext();
  const [progress, setProgress] = useState<ProgressState>({
    state: 'idle',
    percentage: 0,
  });

  useEffect(() => {
    if (!isActive) {
      setProgress({ state: 'idle', percentage: 0 });
      return;
    }

    Debug.log(`[RefreshProgress] Starting SSE for source ${sourceId}`);

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

      const currentStage = event.stages.find((s) => s.id === event.current_stage);
      Debug.log(
        `[RefreshProgress] Processing event for source ${sourceId}:`,
        event.state,
        event.overall_percentage,
        currentStage?.stage_step
      );

      const newState: ProgressState = {
        state: event.state,
        percentage: event.overall_percentage,
        message: currentStage?.stage_step || 'Unknown',
        operationId: event.id,
      };

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

  const getStatusColor = () => {
    switch (progress.state) {
      case 'idle':
      case 'processing':
        return 'bg-blue-100 text-blue-800';
      case 'completed':
        return 'bg-green-100 text-green-800';
      case 'error':
        return 'bg-red-100 text-red-800';
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  const getStatusIcon = () => {
    switch (progress.state) {
      case 'idle':
      case 'processing':
        return <Loader2 className="h-3 w-3 animate-spin" />;
      case 'completed':
        return <CheckCircle className="h-3 w-3" />;
      case 'error':
        return <AlertCircle className="h-3 w-3" />;
      default:
        return null;
    }
  };

  const getStatusText = () => {
    switch (progress.state) {
      case 'idle':
        return progress.message || 'Initializing...';
      case 'processing':
        return progress.message || 'Processing...';
      case 'completed':
        return 'Refresh completed';
      case 'error':
        return progress.message || 'Refresh failed';
      default:
        return '';
    }
  };

  return (
    <div className="space-y-2 p-2 border rounded-md bg-background">
      <div className="flex items-center justify-between">
        <Badge className={getStatusColor()}>
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
    </div>
  );
}
