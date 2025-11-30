'use client';

import { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { Bell, X, GripHorizontal } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { useProgressContext, NotificationEvent } from '@/providers/ProgressProvider';
import { formatProgress } from '@/hooks/useProgressState';
import { cn } from '@/lib/utils';
import { Debug } from '@/utils/debug';

interface NotificationBellProps {
  operationType?: string; // Filter by operation type (undefined = all)
  showPopup?: boolean; // Whether to show popup on click (false = just mark as read)
  className?: string;
  resizeHandlePosition?: 'top' | 'bottom'; // Position of resize handle; affects drag direction (default 'bottom')
}

export function NotificationBell({
  operationType,
  showPopup = true,
  className,
  resizeHandlePosition = 'bottom',
}: NotificationBellProps) {
  const context = useProgressContext();
  const [events, setEvents] = useState<NotificationEvent[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const debug = Debug.createLogger('NotificationBell');
  const [height, setHeight] = useState(384); // Default h-96 = 384px
  const [isResizing, setIsResizing] = useState(false);
  const resizeRef = useRef<HTMLDivElement>(null);
  const startYRef = useRef(0);
  const startHeightRef = useRef(0);

  // Subscribe to all events and always sync with context
  useEffect(() => {
    const unsubscribe = context.subscribeToAll((event) => {
      // Instead of managing state ourselves, always sync with context
      debug.log('🔔 NotificationBell - received event update, syncing with context');
      setEvents(context.getAllEvents());
    });

    // Load initial events
    setEvents(context.getAllEvents());

    return unsubscribe;
  }, [context]);

  // Get unread count directly from context to avoid stale closure issues
  const unreadCount = useMemo(() => {
    const count = context.getUnreadCount(operationType);
    debug.log(
      '🔔 NotificationBell - getting unread count from context:',
      count,
      'for operationType:',
      operationType
    );
    return count;
  }, [context, operationType]);

  // Filter events for display purposes only
  const filteredEvents = useMemo(() => {
    debug.log('🔔 NotificationBell - filtering events for display');
    debug.log(
      '🔔 Raw events:',
      events.map((e) => ({
        id: e.id,
        hasBeenSeen: e.hasBeenSeen,
        operation_type: e.operation_type,
      }))
    );

    let filtered = events;

    // Filter by operation type if specified
    if (operationType) {
      filtered = filtered.filter((e) => e.operation_type === operationType);
      debug.log(
        '🔔 After operation type filter:',
        filtered.map((e) => ({ id: e.id, hasBeenSeen: e.hasBeenSeen }))
      );
    }

    return filtered.slice(0, 20); // Show last 20 events
  }, [events, operationType]);

  // Mark visible events as seen when popup is opened
  useEffect(() => {
    debug.log('🔔 NotificationBell useEffect triggered:', { isOpen, operationType });

    if (isOpen) {
      // Get all current events that match our filter and are unseen
      const allCurrentEvents = context.getAllEvents();
      debug.log(
        '🔔 NotificationBell - all current events:',
        allCurrentEvents.map((e) => ({
          id: e.id,
          hasBeenSeen: e.hasBeenSeen,
          operation_type: e.operation_type,
          state: e.state,
        }))
      );

      let filtered = allCurrentEvents;

      // Filter by operation type if specified
      if (operationType) {
        filtered = filtered.filter((e) => e.operation_type === operationType);
        debug.log(
          '🔔 NotificationBell - filtered by operation type:',
          operationType,
          filtered.map((e) => ({
            id: e.id,
            hasBeenSeen: e.hasBeenSeen,
            operation_type: e.operation_type,
          }))
        );
      }

      const unseenEventIds = filtered.filter((e) => !e.hasBeenSeen).map((e) => e.id);

      debug.log('🔔 NotificationBell - unseen event IDs to mark:', unseenEventIds);

      if (unseenEventIds.length > 0) {
        debug.log('🔔 NotificationBell - calling markAsSeen with:', unseenEventIds);
        context.markAsSeen(unseenEventIds);
      } else {
        debug.log('🔔 NotificationBell - no unseen events to mark');
      }
    }
  }, [isOpen, operationType]);

  // Handle bell click
  const handleBellClick = () => {
    if (showPopup) {
      setIsOpen(!isOpen);
    } else {
      // Just mark all unseen events as seen (no popup mode)
      const unseenEventIds = filteredEvents.filter((e) => !e.hasBeenSeen).map((e) => e.id);

      if (unseenEventIds.length > 0) {
        context.markAsSeen(unseenEventIds);
      }
    }
  };

  // Dismiss functionality removed for simplicity

  const formatEventTime = (event: NotificationEvent) => {
    const date = new Date(event.last_update);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMinutes = Math.floor(diffMs / 60000);

    if (diffMinutes < 1) return 'Just now';
    if (diffMinutes < 60) return `${diffMinutes}m ago`;

    const diffHours = Math.floor(diffMinutes / 60);
    if (diffHours < 24) return `${diffHours}h ago`;

    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}d ago`;
  };

  const getEventStatusColor = (state: string) => {
    switch (state) {
      case 'completed':
        return 'text-green-600 dark:text-green-400';
      case 'error':
        return 'text-destructive';
      case 'processing':
        return 'text-blue-600 dark:text-blue-400';
      case 'idle':
        return 'text-amber-600 dark:text-amber-400';
      default:
        return 'text-muted-foreground';
    }
  };

  const formatOperationTypeTitle = (operationType: string) => {
    return operationType
      .replace('_', ' ')
      .split(' ')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ');
  };

  // Resize handlers - direction depends on handle position
  const handleMouseMove = useCallback(
    (e: MouseEvent) => {
      const deltaY = e.clientY - startYRef.current;
      // If handle at top, dragging down reduces height; at bottom, dragging down increases height
      const direction = resizeHandlePosition === 'top' ? -1 : 1;
      const newHeight = Math.max(200, Math.min(600, startHeightRef.current + direction * deltaY));
      setHeight(newHeight);
    },
    [resizeHandlePosition]
  );

  const handleMouseUp = useCallback(() => {
    setIsResizing(false);
    document.removeEventListener('mousemove', handleMouseMove);
    document.removeEventListener('mouseup', handleMouseUp);
  }, [handleMouseMove]);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      setIsResizing(true);
      startYRef.current = e.clientY;
      startHeightRef.current = height;

      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    },
    [height, handleMouseMove, handleMouseUp]
  );

  // Cleanup listeners on unmount
  useEffect(() => {
    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [handleMouseMove, handleMouseUp]);

  return (
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={cn('relative', className)}
          onClick={handleBellClick}
        >
          <Bell className="h-4 w-4" />
          {unreadCount > 0 && (
            <Badge
              variant="destructive"
              className="absolute -top-1 -right-1 h-5 w-5 rounded-full p-0 text-xs flex items-center justify-center"
            >
              {unreadCount > 99 ? '99+' : unreadCount}
            </Badge>
          )}
        </Button>
      </PopoverTrigger>

      {showPopup && (
        <PopoverContent className="w-96 p-0" align="end">
          <div className="flex items-center justify-between p-4 border-b">
            <h3 className="font-semibold">
              {operationType
                ? `${formatOperationTypeTitle(operationType)} Events`
                : 'Recent Activity'}
            </h3>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setIsOpen(false)}
              className="h-6 w-6 p-0"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          {resizeHandlePosition === 'top' && (
            <div
              ref={resizeRef}
              className="flex items-center justify-center h-4 bg-muted/50 hover:bg-muted border-b cursor-ns-resize group"
              onMouseDown={handleMouseDown}
            >
              <GripHorizontal className="h-3 w-3 text-muted-foreground group-hover:text-foreground transition-colors" />
            </div>
          )}
          <div className="overflow-y-auto notification-scrollbar" style={{ height: `${height}px` }}>
            {filteredEvents.length === 0 ? (
              <div className="p-8 text-center text-muted-foreground">
                <Bell className="h-8 w-8 mx-auto mb-2" />
                <p>No recent events</p>
              </div>
            ) : (
              <div className="p-2">
                {filteredEvents.map((event) => (
                  <div
                    key={event.id}
                    className={cn(
                      'p-3 rounded-lg border mb-2 transition-colors',
                      !event.hasBeenSeen && 'bg-accent/30 border-accent/50'
                    )}
                  >
                    <div className="flex items-start justify-between mb-2">
                      <div className="font-medium text-sm">
                        {event.operation_name}
                        {event.stages &&
                          event.stages.length > 1 &&
                          (() => {
                            const currentStageIndex = event.stages.findIndex(
                              (s) => s.id === event.current_stage
                            );
                            const stageNumber = currentStageIndex >= 0 ? currentStageIndex + 1 : 1;
                            return (
                              <Badge variant="outline" className="ml-2 text-xs">
                                {stageNumber}/{event.stages.length} stages
                              </Badge>
                            );
                          })()}
                      </div>
                      <div className="text-xs text-muted-foreground">{formatEventTime(event)}</div>
                    </div>

                    <div className="flex items-center justify-between text-sm">
                      <span className={cn('font-medium', getEventStatusColor(event.state))}>
                        {event.state.charAt(0).toUpperCase() + event.state.slice(1)}
                      </span>
                      <span className="text-muted-foreground">
                        {(() => {
                          const startTime = new Date(event.started_at).getTime();
                          const updateTime = new Date(event.last_update).getTime();
                          const durationMs = updateTime - startTime;
                          return durationMs > 0 ? `${Math.floor(durationMs / 1000)}s` : '';
                        })()}
                      </span>
                    </div>

                    {/* Stage information */}
                    {(() => {
                      const currentStage = event.stages?.find((s) => s.id === event.current_stage);
                      return (
                        currentStage &&
                        currentStage.stage_step && (
                          <div className="text-xs text-muted-foreground mt-1">
                            <span className="font-medium">{currentStage.name}:</span>{' '}
                            {currentStage.stage_step}
                          </div>
                        )
                      );
                    })()}

                    {/* Enhanced progress display with stage information */}
                    {event.overall_percentage !== undefined && (
                      <div className="space-y-2 mt-2">
                        {/* Overall progress */}
                        <div className="space-y-1">
                          <div className="flex items-center justify-between text-xs">
                            <span className="text-muted-foreground">
                              {event.stages && event.stages.length > 1
                                ? 'Overall Progress'
                                : 'Progress'}
                            </span>
                            <span className="text-muted-foreground font-medium">
                              {event.overall_percentage.toFixed(1)}%
                            </span>
                          </div>
                          <Progress value={event.overall_percentage} className="h-2" />
                        </div>

                        {/* Current stage progress - only show if we have multiple stages */}
                        {(() => {
                          const currentStage = event.stages?.find(
                            (s) => s.id === event.current_stage
                          );
                          if (!currentStage || !event.stages || event.stages.length <= 1)
                            return null;

                          const currentStageIndex = event.stages.findIndex(
                            (s) => s.id === event.current_stage
                          );
                          const stageNumber = currentStageIndex >= 0 ? currentStageIndex + 1 : 1;

                          return (
                            <div className="space-y-1">
                              <div className="flex items-center justify-between text-xs">
                                <span className="text-muted-foreground">
                                  Stage: {currentStage.name}
                                  <span className="ml-1">
                                    ({stageNumber}/{event.stages.length})
                                  </span>
                                </span>
                                <span className="text-muted-foreground font-medium">
                                  {currentStage.percentage.toFixed(1)}%
                                </span>
                              </div>
                              <Progress value={currentStage.percentage} className="h-2" />
                            </div>
                          );
                        })()}
                      </div>
                    )}

                    {event.error && (
                      <div className="text-xs mt-2 p-2 bg-accent/20 border border-accent/30 rounded text-accent-foreground">
                        <span className="text-destructive font-medium">Error:</span> {event.error}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>

          {resizeHandlePosition === 'bottom' && (
            <div
              ref={resizeRef}
              className="flex items-center justify-center h-4 bg-muted/50 hover:bg-muted border-t cursor-ns-resize group"
              onMouseDown={handleMouseDown}
            >
              <GripHorizontal className="h-3 w-3 text-muted-foreground group-hover:text-foreground transition-colors" />
            </div>
          )}
        </PopoverContent>
      )}
    </Popover>
  );
}
