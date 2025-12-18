'use client';

import { useState, useEffect, useCallback, useMemo, ReactNode } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  FileText,
  Play,
  Pause,
  Activity,
  XCircle,
  AlertTriangle,
  Info,
  Bug,
} from 'lucide-react';
import { LogEntry, LogStats } from '@/types/api';
import { logsClient } from '@/lib/logs-client';
import { getBackendUrl } from '@/lib/config';
import { useBackendConnectivity } from '@/providers/backend-connectivity-provider';
import { LogExplorer, LogColumn } from '@/components/shared/log-explorer';

/**
 * Default columns for the log explorer.
 */
const DEFAULT_LOG_COLUMNS: Array<Omit<LogColumn, 'order'>> = [
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
    width: 400,
    minWidth: 200,
    visible: true,
  },
  {
    id: 'target',
    header: 'Target',
    fieldPath: 'target',
    width: 150,
    minWidth: 80,
    visible: true,
  },
  {
    id: 'module',
    header: 'Module',
    fieldPath: 'module',
    width: 150,
    minWidth: 80,
    visible: false,
  },
  {
    id: 'file',
    header: 'File',
    fieldPath: 'file',
    width: 200,
    minWidth: 100,
    visible: false,
  },
  {
    id: 'line',
    header: 'Line',
    fieldPath: 'line',
    width: 60,
    minWidth: 40,
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
    case 'trace':
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
    case 'trace':
      return <Bug className="h-3 w-3" />;
    default:
      return <FileText className="h-3 w-3" />;
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
 * Statistics cards component for the logs page.
 */
function LogStatsCards({
  logStats,
  logsCount,
  pendingCount,
  logsConnected,
  logsPaused,
}: {
  logStats: LogStats | null;
  logsCount: number;
  pendingCount: number;
  logsConnected: boolean;
  logsPaused: boolean;
}) {
  return (
    <div className="grid gap-4 md:grid-cols-4 p-3 border-b">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Total Logs</CardTitle>
          <FileText className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{logStats?.total_logs || logsCount}</div>
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
          <div className="text-2xl font-bold text-destructive">
            {logStats?.logs_by_level?.error || 0}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Warnings</CardTitle>
          <AlertTriangle className="h-4 w-4 text-amber-500" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold text-amber-500">
            {logStats?.logs_by_level?.warn || 0}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Connection</CardTitle>
          <div
            className={`h-2 w-2 rounded-full ${logsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
          />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{logsConnected ? 'Live' : 'Offline'}</div>
          <p className="text-xs text-muted-foreground">{logsPaused && 'Paused'}</p>
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * Stream controls component.
 */
function StreamControls({
  logsConnected,
  logsPaused,
  pendingCount,
  onToggleStream,
  onTogglePause,
  onClear,
}: {
  logsConnected: boolean;
  logsPaused: boolean;
  pendingCount: number;
  onToggleStream: () => void;
  onTogglePause: () => void;
  onClear: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Button
        onClick={onToggleStream}
        variant={logsConnected ? 'destructive' : 'default'}
        size="sm"
      >
        {logsConnected ? (
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

      {logsConnected && (
        <Button onClick={onTogglePause} variant="outline" size="sm">
          {logsPaused ? (
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
          className={`h-2 w-2 rounded-full ${logsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
        />
        {logsConnected ? 'Connected' : 'Disconnected'}
      </div>
    </div>
  );
}

export function Logs() {
  const { isConnected: backendConnected } = useBackendConnectivity();

  // Logs state
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [pendingLogs, setPendingLogs] = useState<LogEntry[]>([]);
  const [logsConnected, setLogsConnected] = useState(false);
  const [logsPaused, setLogsPaused] = useState(false);
  const [logStats, setLogStats] = useState<LogStats | null>(null);

  // Handle new logs from SSE
  const handleNewLog = useCallback((log: LogEntry) => {
    // Ensure the log has required fields
    const normalizedLog: LogEntry = {
      ...log,
      id: log.id || `log-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      timestamp: log.timestamp || new Date().toISOString(),
    };

    if (logsPaused) {
      setPendingLogs((prev) => [normalizedLog, ...prev].slice(0, 100));
    } else {
      setLogs((prev) => [normalizedLog, ...prev].slice(0, 1000));
    }
  }, [logsPaused]);

  // Fetch log statistics
  const fetchLogStats = useCallback(async () => {
    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/logs/stats`);
      if (response.ok) {
        const stats: LogStats = await response.json();
        setLogStats(stats);
      }
    } catch (error) {
      console.warn('Failed to fetch log stats:', error);
    }
  }, []);

  // Toggle logs stream
  const toggleLogsStream = useCallback(() => {
    if (logsConnected) {
      logsClient.disconnect();
      setLogsConnected(false);
    } else {
      logsClient.connect();
      logsClient.subscribe(handleNewLog);
      setLogsConnected(true);
    }
  }, [logsConnected, handleNewLog]);

  // Toggle pause/resume logs
  const toggleLogsPause = useCallback(() => {
    if (logsPaused) {
      // Resume - add pending logs to main list
      setLogs((prev) => [...pendingLogs, ...prev].slice(0, 1000));
      setPendingLogs([]);
    }
    setLogsPaused(!logsPaused);
  }, [logsPaused, pendingLogs]);

  // Clear logs
  const clearLogs = useCallback(() => {
    setLogs([]);
    setPendingLogs([]);
  }, []);

  // Custom cell renderers
  const cellRenderers = useMemo(
    () => ({
      level: LevelCellRenderer,
    }),
    []
  );

  useEffect(() => {
    // Only connect to logs stream if backend is available
    if (!backendConnected) {
      logsClient.disconnect();
      setLogsConnected(false);
      return;
    }

    // Auto-connect to logs stream
    logsClient.connect();
    logsClient.subscribe(handleNewLog);
    setLogsConnected(true);

    // Fetch initial stats
    fetchLogStats();

    // Refresh stats every 30 seconds
    const statsInterval = setInterval(fetchLogStats, 30000);

    // Cleanup on unmount or when backend disconnects
    return () => {
      logsClient.disconnect();
      setLogsConnected(false);
      clearInterval(statsInterval);
    };
  }, [backendConnected, handleNewLog, fetchLogStats]);

  return (
    <div className="h-full flex flex-col">
      <LogExplorer
        data={logs}
        storageKey="logs"
        defaultColumns={DEFAULT_LOG_COLUMNS}
        excludeFields={EXCLUDE_FIELDS}
        cellRenderers={cellRenderers}
        onRefresh={fetchLogStats}
        title="Live Log Stream"
        showFieldsSidebar={true}
        showDetailPanel={true}
        emptyState={{
          title: 'No logs received',
          description: logsConnected
            ? 'Waiting for log data...'
            : 'Connect to start receiving logs',
        }}
        statsCards={
          <LogStatsCards
            logStats={logStats}
            logsCount={logs.length}
            pendingCount={pendingLogs.length}
            logsConnected={logsConnected}
            logsPaused={logsPaused}
          />
        }
        controls={
          <StreamControls
            logsConnected={logsConnected}
            logsPaused={logsPaused}
            pendingCount={pendingLogs.length}
            onToggleStream={toggleLogsStream}
            onTogglePause={toggleLogsPause}
            onClear={clearLogs}
          />
        }
      />
    </div>
  );
}
