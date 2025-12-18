'use client';

import { useState, useEffect, useCallback, useMemo, ReactNode } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  MessageCircle,
  Play,
  Pause,
  Activity,
  XCircle,
  AlertTriangle,
  Info,
  Bug,
} from 'lucide-react';
import { ServiceEvent } from '@/types/api';
import { useVisibilityTracking } from '@/hooks/useVisibilityTracking';
import { useProgressContext } from '@/providers/ProgressProvider';
import { LogExplorer, LogColumn } from '@/components/shared/log-explorer';

/**
 * Default columns for the events explorer.
 */
const DEFAULT_EVENT_COLUMNS: Array<Omit<LogColumn, 'order'>> = [
  {
    id: 'timestamp',
    header: 'Time',
    fieldPath: 'timestamp',
    width: 100,
    minWidth: 80,
    visible: true,
    pinned: true,
  },
  {
    id: 'level',
    header: 'Level',
    fieldPath: 'level',
    width: 80,
    minWidth: 60,
    visible: true,
  },
  {
    id: 'message',
    header: 'Message',
    fieldPath: 'message',
    width: 350,
    minWidth: 200,
    visible: true,
  },
  {
    id: 'source',
    header: 'Source',
    fieldPath: 'source',
    width: 120,
    minWidth: 80,
    visible: true,
  },
  {
    id: 'context.state',
    header: 'State',
    fieldPath: 'context.state',
    width: 100,
    minWidth: 60,
    visible: true,
  },
  {
    id: 'context.overall_percentage',
    header: 'Progress',
    fieldPath: 'context.overall_percentage',
    width: 80,
    minWidth: 60,
    visible: false,
  },
];

/**
 * Fields to exclude from the available fields list.
 */
const EXCLUDE_FIELDS = ['id', 'timestamp'];

/**
 * Level badge variant mapping.
 */
function getLevelBadgeVariant(
  level: string
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (level) {
    case 'error':
      return 'destructive';
    case 'warn':
      return 'secondary';
    case 'info':
      return 'default';
    case 'debug':
      return 'outline';
    default:
      return 'outline';
  }
}

/**
 * Level icon mapping.
 */
function getLevelIcon(level: string) {
  switch (level) {
    case 'error':
      return <XCircle className="h-3 w-3" />;
    case 'warn':
      return <AlertTriangle className="h-3 w-3" />;
    case 'info':
      return <Info className="h-3 w-3" />;
    case 'debug':
      return <Bug className="h-3 w-3" />;
    default:
      return <MessageCircle className="h-3 w-3" />;
  }
}

/**
 * Custom cell renderer for the level column.
 */
function LevelCellRenderer(value: unknown): ReactNode {
  const level = String(value);
  return (
    <Badge variant={getLevelBadgeVariant(level)} className="text-xs gap-1">
      {getLevelIcon(level)}
      {level}
    </Badge>
  );
}

/**
 * Custom cell renderer for the state column.
 */
function StateCellRenderer(value: unknown): ReactNode {
  if (!value) return '-';
  const state = String(value);
  return (
    <Badge
      variant={
        state === 'completed'
          ? 'default'
          : state === 'failed' || state === 'error'
            ? 'destructive'
            : 'secondary'
      }
      className="text-xs"
    >
      {state}
    </Badge>
  );
}

/**
 * Custom cell renderer for progress percentage.
 */
function ProgressCellRenderer(value: unknown): ReactNode {
  if (value === null || value === undefined) return '-';
  const percentage = Number(value);
  if (isNaN(percentage)) return '-';
  return `${Math.round(percentage)}%`;
}

/**
 * Statistics cards component for the events page.
 */
function EventStatsCards({
  eventsCount,
  errorCount,
  warningCount,
  pendingCount,
  eventsConnected,
  eventsPaused,
}: {
  eventsCount: number;
  errorCount: number;
  warningCount: number;
  pendingCount: number;
  eventsConnected: boolean;
  eventsPaused: boolean;
}) {
  return (
    <div className="grid gap-4 md:grid-cols-4 p-3 border-b">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Total Events</CardTitle>
          <MessageCircle className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{eventsCount}</div>
          <p className="text-xs text-muted-foreground">
            {pendingCount > 0 && `${pendingCount} pending`}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Errors</CardTitle>
          <XCircle className="h-4 w-4 text-destructive" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold text-destructive">{errorCount}</div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Warnings</CardTitle>
          <Activity className="h-4 w-4 text-amber-500" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold text-amber-500">{warningCount}</div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Connection</CardTitle>
          <div
            className={`h-2 w-2 rounded-full ${eventsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
          />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{eventsConnected ? 'Live' : 'Offline'}</div>
          <p className="text-xs text-muted-foreground">{eventsPaused && 'Paused'}</p>
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * Stream controls component.
 */
function StreamControls({
  eventsConnected,
  eventsPaused,
  pendingCount,
  onToggleStream,
  onTogglePause,
  onClear,
}: {
  eventsConnected: boolean;
  eventsPaused: boolean;
  pendingCount: number;
  onToggleStream: () => void;
  onTogglePause: () => void;
  onClear: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Button
        onClick={onToggleStream}
        variant={eventsConnected ? 'destructive' : 'default'}
        size="sm"
      >
        {eventsConnected ? (
          <>
            <XCircle className="h-4 w-4 mr-2" />
            Disconnect
          </>
        ) : (
          <>
            <Activity className="h-4 w-4 mr-2" />
            Connect
          </>
        )}
      </Button>

      {eventsConnected && (
        <Button onClick={onTogglePause} variant="outline" size="sm">
          {eventsPaused ? (
            <>
              <Play className="h-4 w-4 mr-2" />
              Resume
              {pendingCount > 0 && (
                <Badge variant="secondary" className="ml-2">
                  {pendingCount}
                </Badge>
              )}
            </>
          ) : (
            <>
              <Pause className="h-4 w-4 mr-2" />
              Pause
            </>
          )}
        </Button>
      )}

      <Button onClick={onClear} variant="outline" size="sm">
        Clear
      </Button>

      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <div
          className={`h-2 w-2 rounded-full ${eventsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
        />
        {eventsConnected ? 'Connected' : 'Disconnected'}
      </div>
    </div>
  );
}

export function Events() {
  const progressContext = useProgressContext();

  // Events state
  const [events, setEvents] = useState<ServiceEvent[]>([]);
  const [pendingEvents, setPendingEvents] = useState<ServiceEvent[]>([]);
  const [eventsPaused, setEventsPaused] = useState(false);

  // Convert ProgressEvents to ServiceEvents for display
  useEffect(() => {
    const unsubscribe = progressContext.subscribeToAll((progressEvent) => {
      // Convert ProgressEvent to ServiceEvent format for events page
      const currentStage = progressEvent.stages.find((s) => s.id === progressEvent.current_stage);
      const serviceEvent: ServiceEvent = {
        id: progressEvent.id,
        timestamp: progressEvent.last_update,
        level:
          progressEvent.state === 'error'
            ? 'error'
            : progressEvent.state === 'processing'
              ? 'info'
              : 'debug',
        message: `${progressEvent.operation_name}: ${currentStage?.stage_step || 'Unknown'}`,
        source: progressEvent.operation_type,
        context: {
          state: progressEvent.state,
          overall_percentage: progressEvent.overall_percentage,
          current_stage: progressEvent.current_stage,
          stages: progressEvent.stages,
          error: progressEvent.error,
        },
      };

      if (eventsPaused) {
        setPendingEvents((prev) => [serviceEvent, ...prev].slice(0, 100));
      } else {
        setEvents((prev) => [serviceEvent, ...prev].slice(0, 500));
      }
    });

    return unsubscribe;
  }, [eventsPaused, progressContext]);

  // Mark visible events as seen
  useVisibilityTracking(events.map((event) => event.id));

  // Get connection status from ProgressProvider
  const eventsConnected = progressContext.isConnected;

  // Toggle events stream connection
  const toggleEventsStream = useCallback(() => {
    // Since we're using ProgressProvider for events, we can't easily toggle connection
    // This button will trigger a page refresh
    window.location.reload();
  }, []);

  // Toggle pause/resume events
  const toggleEventsPause = useCallback(() => {
    if (eventsPaused) {
      // Resume - add pending events to main list
      setEvents((prev) => [...pendingEvents, ...prev].slice(0, 500));
      setPendingEvents([]);
    }
    setEventsPaused(!eventsPaused);
  }, [eventsPaused, pendingEvents]);

  // Clear events
  const clearEvents = useCallback(() => {
    setEvents([]);
    setPendingEvents([]);
  }, []);

  // Count errors and warnings
  const errorCount = useMemo(() => events.filter((e) => e.level === 'error').length, [events]);
  const warningCount = useMemo(() => events.filter((e) => e.level === 'warn').length, [events]);

  // Custom cell renderers
  const cellRenderers = useMemo(
    () => ({
      level: LevelCellRenderer,
      'context.state': StateCellRenderer,
      'context.overall_percentage': ProgressCellRenderer,
    }),
    []
  );

  return (
    <div className="h-full flex flex-col">
      <LogExplorer
        data={events}
        storageKey="events"
        defaultColumns={DEFAULT_EVENT_COLUMNS}
        excludeFields={EXCLUDE_FIELDS}
        cellRenderers={cellRenderers}
        title="Live Events Stream"
        showFieldsSidebar={true}
        showDetailPanel={true}
        emptyState={{
          title: 'No events received',
          description: eventsConnected
            ? 'Waiting for event data...'
            : 'Connect to start receiving events',
        }}
        statsCards={
          <EventStatsCards
            eventsCount={events.length}
            errorCount={errorCount}
            warningCount={warningCount}
            pendingCount={pendingEvents.length}
            eventsConnected={eventsConnected}
            eventsPaused={eventsPaused}
          />
        }
        controls={
          <StreamControls
            eventsConnected={eventsConnected}
            eventsPaused={eventsPaused}
            pendingCount={pendingEvents.length}
            onToggleStream={toggleEventsStream}
            onTogglePause={toggleEventsPause}
            onClear={clearEvents}
          />
        }
      />
    </div>
  );
}
