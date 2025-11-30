'use client';

import { useState, useEffect, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import { cn } from '@/lib/utils';
import { DateTimePicker } from '@/components/ui/date-time-picker';
import {
  FileText,
  Play,
  Pause,
  Search,
  Filter,
  Activity,
  XCircle,
  AlertTriangle,
  Info,
  Bug,
  ChevronDown,
  ChevronUp,
  Code,
  AlertCircle,
  Zap,
  CheckCircle,
  Check,
  ChevronsUpDown,
} from 'lucide-react';
import { LogEntry, LogStats } from '@/types/api';
import { logsClient } from '@/lib/logs-client';
import { getBackendUrl } from '@/lib/config';
import { useBackendConnectivity } from '@/providers/backend-connectivity-provider';

export function Logs() {
  const { isConnected: backendConnected } = useBackendConnectivity();

  // Logs state
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [pendingLogs, setPendingLogs] = useState<LogEntry[]>([]);
  const [logsConnected, setLogsConnected] = useState(false);
  const [logsPaused, setLogsPaused] = useState(false);
  const [logStats, setLogStats] = useState<LogStats | null>(null);
  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set());
  const [showRawData, setShowRawData] = useState<Set<string>>(new Set());
  const [dateAfter, setDateAfter] = useState<Date | undefined>(undefined);
  const [dateBefore, setDateBefore] = useState<Date | undefined>(undefined);

  // Filter state
  const [textFilter, setTextFilter] = useState('');
  const [levelFilter, setLevelFilter] = useState('all');
  const [moduleFilter, setModuleFilter] = useState('all');
  const [moduleOpen, setModuleOpen] = useState(false);

  // Get unique modules from logs for dropdown
  const uniqueModules = useMemo(() => {
    const modules = new Set<string>();
    logs.forEach((log) => {
      if (log.target) modules.add(log.target);
      if (log.module && log.module !== log.target) modules.add(log.module);
    });
    return Array.from(modules).sort();
  }, [logs]);

  // Filter logs
  const filteredLogs = useMemo(() => {
    return logs.filter((log) => {
      const matchesText =
        textFilter === '' ||
        log.message.toLowerCase().includes(textFilter.toLowerCase()) ||
        (log.module && log.module.toLowerCase().includes(textFilter.toLowerCase())) ||
        (log.target && log.target.toLowerCase().includes(textFilter.toLowerCase())) ||
        (log.context &&
          JSON.stringify(log.context).toLowerCase().includes(textFilter.toLowerCase())) ||
        (log.fields &&
          JSON.stringify(log.fields).toLowerCase().includes(textFilter.toLowerCase())) ||
        (log.span && JSON.stringify(log.span).toLowerCase().includes(textFilter.toLowerCase()));

      const matchesLevel = levelFilter === 'all' || log.level === levelFilter;

      const matchesModule =
        moduleFilter === 'all' || moduleFilter === log.target || moduleFilter === log.module;

      // Date range filtering
      const logDate = new Date(log.timestamp);
      const matchesDateAfter = !dateAfter || logDate >= dateAfter;
      const matchesDateBefore = !dateBefore || logDate <= dateBefore;

      return matchesText && matchesLevel && matchesModule && matchesDateAfter && matchesDateBefore;
    });
  }, [logs, textFilter, levelFilter, moduleFilter, dateAfter, dateBefore]);

  // Handle new logs from SSE
  const handleNewLog = (log: LogEntry) => {
    if (logsPaused) {
      setPendingLogs((prev) => [log, ...prev].slice(0, 100)); // Keep only last 100 pending
    } else {
      setLogs((prev) => [log, ...prev].slice(0, 1000)); // Keep only last 1000 logs
    }
  };

  // Fetch log statistics
  const fetchLogStats = async () => {
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
  };

  // Toggle logs stream
  const toggleLogsStream = () => {
    if (logsConnected) {
      logsClient.disconnect();
      setLogsConnected(false);
    } else {
      logsClient.connect();
      logsClient.subscribe(handleNewLog);
      setLogsConnected(true);
    }
  };

  // Toggle pause/resume logs
  const toggleLogsPause = () => {
    if (logsPaused) {
      // Resume - add pending logs to main list
      setLogs((prev) => [...pendingLogs, ...prev].slice(0, 1000));
      setPendingLogs([]);
    }
    setLogsPaused(!logsPaused);
  };

  // Clear logs
  const clearLogs = () => {
    setLogs([]);
    setPendingLogs([]);
  };

  // Get level badge variant and icon
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
      case 'trace':
        return 'outline';
      default:
        return 'outline';
    }
  }

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

  const toggleLogExpansion = (logId: string) => {
    setExpandedLogs((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(logId)) {
        newSet.delete(logId);
      } else {
        newSet.add(logId);
      }
      return newSet;
    });
  };

  const toggleRawData = (logId: string) => {
    setShowRawData((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(logId)) {
        newSet.delete(logId);
      } else {
        newSet.add(logId);
      }
      return newSet;
    });
  };

  const renderLogFields = (fields: Record<string, any> | null | undefined) => {
    if (!fields || typeof fields !== 'object') return null;

    const filteredFields = Object.entries(fields).filter(([key]) => key !== 'message');

    if (filteredFields.length === 0) return null;

    return (
      <div className="mt-2 space-y-1">
        {filteredFields.map(([key, value]) => (
          <div key={key} className="flex items-start gap-2 text-xs">
            <span className="font-medium text-muted-foreground min-w-0 flex-shrink-0">{key}:</span>
            <span className="font-mono bg-muted px-1 py-0.5 rounded text-xs break-all">
              {typeof value === 'string' ? value : JSON.stringify(value)}
            </span>
          </div>
        ))}
      </div>
    );
  };

  const renderLogSpan = (span: any) => {
    if (!span || typeof span !== 'object') return null;

    return (
      <div className="mt-2 space-y-1">
        <div className="text-xs text-muted-foreground font-medium">Span Information:</div>
        {Object.entries(span).map(([key, value]) => (
          <div key={key} className="flex items-start gap-2 text-xs">
            <span className="font-medium text-muted-foreground min-w-0 flex-shrink-0">{key}:</span>
            <span className="font-mono bg-muted px-1 py-0.5 rounded text-xs break-all">
              {typeof value === 'string' ? value : JSON.stringify(value)}
            </span>
          </div>
        ))}
      </div>
    );
  };

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
  }, [backendConnected]);

  return (
    <div className="space-y-6">
      {/* Log Statistics */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Logs</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{logStats?.total_logs || logs.length}</div>
            <p className="text-xs text-muted-foreground">
              {pendingLogs.length > 0 && `${pendingLogs.length} pending`}
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
              {logStats?.logs_by_level?.error || logs.filter((l) => l.level === 'error').length}
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
              {logStats?.logs_by_level?.warn || logs.filter((l) => l.level === 'warn').length}
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

      {/* Live Log Stream */}
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <FileText className="h-5 w-5" />
            Live Log Stream
            {pendingLogs.length > 0 && (
              <Badge variant="secondary" className="ml-2">
                {pendingLogs.length} pending
              </Badge>
            )}
          </h3>
          <p className="text-muted-foreground mt-1">
            Real-time application logs and system information from the tvarr backend
          </p>
        </div>
        <div>
          {/* Log Controls */}
          <div className="flex flex-wrap items-center gap-3 mb-4">
            <Button
              onClick={toggleLogsStream}
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
              <Button onClick={toggleLogsPause} variant="outline" size="sm">
                {logsPaused ? (
                  <>
                    <Play className="h-4 w-4 mr-2" />
                    Resume
                  </>
                ) : (
                  <>
                    <Pause className="h-4 w-4 mr-2" />
                    Pause
                  </>
                )}
              </Button>
            )}

            <Button onClick={clearLogs} variant="outline" size="sm">
              Clear
            </Button>

            <Button onClick={fetchLogStats} variant="outline" size="sm">
              Refresh Stats
            </Button>

            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <div
                className={`h-2 w-2 rounded-full ${logsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
              />
              {logsConnected ? 'Connected' : 'Disconnected'}
            </div>
          </div>

          {/* Log Filters */}
          <div className="space-y-3 mb-4">
            {/* Responsive Filter Layout */}
            <div className="flex flex-col lg:flex-row gap-3">
              {/* Text Filter - Flexible with minimum width */}
              <div className="relative flex-1 min-w-[200px]">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Filter by text..."
                  value={textFilter}
                  onChange={(e) => setTextFilter(e.target.value)}
                  className="pl-9 h-9"
                />
              </div>

              {/* Right-aligned filters with fixed widths */}
              <div className="flex flex-col sm:flex-row gap-2 lg:ml-auto">
                <Select value={levelFilter} onValueChange={setLevelFilter}>
                  <SelectTrigger className="h-9 w-full sm:w-28">
                    <SelectValue placeholder="All levels" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Levels</SelectItem>
                    <SelectItem value="trace">Trace</SelectItem>
                    <SelectItem value="debug">Debug</SelectItem>
                    <SelectItem value="info">Info</SelectItem>
                    <SelectItem value="warn">Warning</SelectItem>
                    <SelectItem value="error">Error</SelectItem>
                  </SelectContent>
                </Select>

                <Popover open={moduleOpen} onOpenChange={setModuleOpen}>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      role="combobox"
                      aria-expanded={moduleOpen}
                      className="h-9 w-full sm:w-40 justify-between font-normal"
                    >
                      <span className="truncate">
                        {moduleFilter === 'all'
                          ? 'All modules'
                          : moduleFilter.length > 20
                            ? `${moduleFilter.substring(0, 20)}...`
                            : moduleFilter}
                      </span>
                      <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-[200px] p-0 z-50" sideOffset={4}>
                    <Command>
                      <CommandInput placeholder="Search modules..." />
                      <CommandList>
                        <CommandEmpty>No module found.</CommandEmpty>
                        <CommandGroup>
                          <CommandItem
                            value="all"
                            onSelect={() => {
                              setModuleFilter('all');
                              setModuleOpen(false);
                            }}
                          >
                            <Check
                              className={cn(
                                'mr-2 h-4 w-4',
                                moduleFilter === 'all' ? 'opacity-100' : 'opacity-0'
                              )}
                            />
                            All Modules
                          </CommandItem>
                          {uniqueModules.map((module) => (
                            <CommandItem
                              key={module}
                              value={module}
                              onSelect={(currentValue) => {
                                setModuleFilter(currentValue);
                                setModuleOpen(false);
                              }}
                            >
                              <Check
                                className={cn(
                                  'mr-2 h-4 w-4',
                                  moduleFilter === module ? 'opacity-100' : 'opacity-0'
                                )}
                              />
                              {module}
                            </CommandItem>
                          ))}
                        </CommandGroup>
                      </CommandList>
                    </Command>
                  </PopoverContent>
                </Popover>

                <div className="flex items-center gap-2">
                  <DateTimePicker
                    value={dateAfter}
                    onChange={setDateAfter}
                    placeholder="Start time"
                    className="w-32"
                  />
                  <DateTimePicker
                    value={dateBefore}
                    onChange={setDateBefore}
                    placeholder="End time"
                    className="w-32"
                  />
                  {(dateAfter || dateBefore) && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setDateAfter(undefined);
                        setDateBefore(undefined);
                      }}
                      className="h-9 w-9 p-0"
                      title="Clear date range"
                    >
                      <XCircle className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>
            </div>

            {/* Filter Stats */}
            <div className="text-sm text-muted-foreground flex items-center gap-2">
              <Filter className="h-4 w-4" />
              {filteredLogs.length} of {logs.length} logs
            </div>
          </div>

          {/* Log List */}
          <div className="space-y-2">
            {filteredLogs.length === 0 ? (
              <div className="text-center text-muted-foreground py-8">
                {logs.length === 0 ? 'No logs received yet' : 'No logs match current filters'}
              </div>
            ) : (
              filteredLogs.map((log) => {
                const isExpanded = expandedLogs.has(log.id);
                const showRaw = showRawData.has(log.id);
                const hasFields = log.fields && Object.keys(log.fields).length > 0;
                const hasSpan = log.span && typeof log.span === 'object';

                return (
                  <div
                    key={`${log.id}-${log.timestamp}`}
                    className="border rounded-lg hover:bg-muted/50"
                  >
                    <div className="flex items-start gap-3 p-3">
                      <div className="flex-shrink-0 mt-0.5">
                        <Badge variant={getLevelBadgeVariant(log.level)} className="text-xs gap-1">
                          {getLevelIcon(log.level)}
                          {log.level}
                        </Badge>
                      </div>

                      <div className="flex-1 min-w-0">
                        <div className="flex items-start justify-between gap-2">
                          <p className="text-sm font-medium leading-tight">{log.message}</p>
                          <div className="flex items-center gap-2 flex-shrink-0">
                            <span className="text-xs text-muted-foreground">
                              {new Date(log.timestamp).toLocaleTimeString()}
                            </span>

                            {/* Expand/Collapse Button */}
                            {(hasFields || hasSpan || log.context) && (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => toggleLogExpansion(log.id)}
                                className="h-6 w-6 p-0"
                              >
                                {isExpanded ? (
                                  <ChevronUp className="h-3 w-3" />
                                ) : (
                                  <ChevronDown className="h-3 w-3" />
                                )}
                              </Button>
                            )}

                            {/* Raw Data Toggle */}
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => toggleRawData(log.id)}
                              className="h-6 w-6 p-0"
                              title="Toggle raw data"
                            >
                              <Code className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>

                        <div className="flex items-center gap-2 mt-1">
                          <Badge variant="outline" className="text-xs">
                            {log.target || 'unknown'}
                          </Badge>
                          {log.module && log.module !== log.target && (
                            <Badge variant="outline" className="text-xs">
                              {log.module}
                            </Badge>
                          )}
                          {log.file && (
                            <span className="text-xs text-muted-foreground">
                              {log.file}
                              {log.line && `:${log.line}`}
                            </span>
                          )}
                          {hasFields && (
                            <Badge variant="outline" className="text-xs">
                              +{Object.keys(log.fields).length} fields
                            </Badge>
                          )}
                          {hasSpan && (
                            <Badge variant="outline" className="text-xs">
                              span
                            </Badge>
                          )}
                          {log.context && Object.keys(log.context).length > 0 && (
                            <Badge variant="outline" className="text-xs">
                              +{Object.keys(log.context).length} context
                            </Badge>
                          )}
                        </div>

                        {/* Expanded Fields */}
                        {isExpanded && hasFields && renderLogFields(log.fields)}

                        {/* Expanded Span */}
                        {isExpanded && hasSpan && renderLogSpan(log.span)}

                        {/* Context (legacy) */}
                        {isExpanded && log.context && Object.keys(log.context).length > 0 && (
                          <div className="mt-2 p-2 bg-muted rounded text-xs">
                            <div className="font-medium mb-1">Context:</div>
                            <pre className="whitespace-pre-wrap font-mono">
                              {JSON.stringify(log.context, null, 2)}
                            </pre>
                          </div>
                        )}

                        {/* Raw Data */}
                        {showRaw && (
                          <div className="mt-2 p-2 bg-muted rounded text-xs">
                            <div className="font-medium mb-1">Raw Data:</div>
                            <pre className="whitespace-pre-wrap font-mono">
                              {JSON.stringify(log, null, 2)}
                            </pre>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
