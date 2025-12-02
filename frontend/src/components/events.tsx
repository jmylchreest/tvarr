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
  MessageCircle,
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
  Check,
  ChevronsUpDown,
} from 'lucide-react';
import { ServiceEvent } from '@/types/api';
import { useVisibilityTracking } from '@/hooks/useVisibilityTracking';
import { useProgressContext } from '@/providers/ProgressProvider';

export function Events() {
  const progressContext = useProgressContext();

  // Events state
  const [events, setEvents] = useState<ServiceEvent[]>([]);
  const [pendingEvents, setPendingEvents] = useState<ServiceEvent[]>([]);
  const [eventsPaused, setEventsPaused] = useState(false);
  const [expandedEvents, setExpandedEvents] = useState<Set<string>>(new Set());
  const [showRawData, setShowRawData] = useState<Set<string>>(new Set());

  // Filter state
  const [textFilter, setTextFilter] = useState('');
  const [levelFilter, setLevelFilter] = useState('all');
  const [sourceFilter, setSourceFilter] = useState('all');
  const [sourceOpen, setSourceOpen] = useState(false);
  const [dateAfter, setDateAfter] = useState<Date | undefined>(undefined);
  const [dateBefore, setDateBefore] = useState<Date | undefined>(undefined);

  // Get unique sources from events for dropdown
  const uniqueSources = useMemo(() => {
    const sources = new Set<string>();
    events.forEach((event) => {
      if (event.source) sources.add(event.source);
    });
    return Array.from(sources).sort();
  }, [events]);

  // Filter events
  const filteredEvents = useMemo(() => {
    return events.filter((event) => {
      const matchesText =
        textFilter === '' ||
        event.message.toLowerCase().includes(textFilter.toLowerCase()) ||
        (event.context &&
          JSON.stringify(event.context).toLowerCase().includes(textFilter.toLowerCase()));

      const matchesLevel = levelFilter === 'all' || event.level === levelFilter;

      const matchesSource = sourceFilter === 'all' || sourceFilter === event.source;

      // Date range filtering
      const eventDate = new Date(event.timestamp);
      const matchesDateAfter = !dateAfter || eventDate >= dateAfter;
      const matchesDateBefore = !dateBefore || eventDate <= dateBefore;

      return matchesText && matchesLevel && matchesSource && matchesDateAfter && matchesDateBefore;
    });
  }, [events, textFilter, levelFilter, sourceFilter, dateAfter, dateBefore]);

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
  }, [eventsPaused]);

  // Mark visible events as seen
  useVisibilityTracking(filteredEvents.map((event) => event.id));

  // Get connection status from ProgressProvider
  const eventsConnected = progressContext.isConnected;

  // Toggle events stream connection
  const toggleEventsStream = () => {
    // Since we're using ProgressProvider for events, we can't easily toggle connection
    // This button will be disabled for now or could trigger a page refresh
    window.location.reload();
  };

  // Toggle pause/resume events
  const toggleEventsPause = () => {
    if (eventsPaused) {
      // Resume - add pending events to main list
      setEvents((prev) => [...pendingEvents, ...prev].slice(0, 500));
      setPendingEvents([]);
    }
    setEventsPaused(!eventsPaused);
  };

  // Clear events
  const clearEvents = () => {
    setEvents([]);
    setPendingEvents([]);
  };

  // Get level badge variant
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

  const toggleEventExpansion = (eventId: string) => {
    setExpandedEvents((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(eventId)) {
        newSet.delete(eventId);
      } else {
        newSet.add(eventId);
      }
      return newSet;
    });
  };

  const toggleRawData = (eventId: string) => {
    setShowRawData((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(eventId)) {
        newSet.delete(eventId);
      } else {
        newSet.add(eventId);
      }
      return newSet;
    });
  };

  const renderEventContext = (context: Record<string, any> | null | undefined) => {
    if (!context || typeof context !== 'object') return null;

    const filteredContext = Object.entries(context).filter(
      ([key]) => !['state', 'progress'].includes(key) // Exclude already displayed fields
    );

    if (filteredContext.length === 0) return null;

    return (
      <div className="mt-2 space-y-1">
        {filteredContext.map(([key, value]) => (
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

  return (
    <div className="space-y-6">
      {/* Events Statistics */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Events</CardTitle>
            <MessageCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{events.length}</div>
            <p className="text-xs text-muted-foreground">
              {pendingEvents.length > 0 && `${pendingEvents.length} pending`}
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
              {events.filter((e) => e.level === 'error').length}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Warnings</CardTitle>
            <Activity className="h-4 w-4 text-amber-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-amber-500">
              {events.filter((e) => e.level === 'warn').length}
            </div>
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

      {/* Live Events Stream */}
      <div className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <MessageCircle className="h-5 w-5" />
            Live Events Stream
            {pendingEvents.length > 0 && (
              <Badge variant="secondary" className="ml-2">
                {pendingEvents.length} pending
              </Badge>
            )}
          </h3>
          <p className="text-muted-foreground mt-1">
            Real-time service events and logging information from the tvarr backend
          </p>
        </div>
        <div>
          {/* Events Controls */}
          <div className="flex flex-wrap items-center gap-3 mb-4">
            <Button
              onClick={toggleEventsStream}
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
              <Button onClick={toggleEventsPause} variant="outline" size="sm">
                {eventsPaused ? (
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

            <Button onClick={clearEvents} variant="outline" size="sm">
              Clear
            </Button>

            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <div
                className={`h-2 w-2 rounded-full ${eventsConnected ? 'bg-green-500' : 'bg-gray-400'}`}
              />
              {eventsConnected ? 'Connected' : 'Disconnected'}
            </div>
          </div>

          {/* Events Filters */}
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
                    <SelectItem value="debug">Debug</SelectItem>
                    <SelectItem value="info">Info</SelectItem>
                    <SelectItem value="warn">Warning</SelectItem>
                    <SelectItem value="error">Error</SelectItem>
                  </SelectContent>
                </Select>

                <Popover open={sourceOpen} onOpenChange={setSourceOpen}>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      role="combobox"
                      aria-expanded={sourceOpen}
                      className="h-9 w-full sm:w-40 justify-between font-normal"
                    >
                      <span className="truncate">
                        {sourceFilter === 'all'
                          ? 'All sources'
                          : sourceFilter.length > 20
                            ? `${sourceFilter.substring(0, 20)}...`
                            : sourceFilter}
                      </span>
                      <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-[200px] p-0 z-50" sideOffset={4}>
                    <Command>
                      <CommandInput placeholder="Search sources..." />
                      <CommandList>
                        <CommandEmpty>No source found.</CommandEmpty>
                        <CommandGroup>
                          <CommandItem
                            value="all"
                            onSelect={() => {
                              setSourceFilter('all');
                              setSourceOpen(false);
                            }}
                          >
                            <Check
                              className={cn(
                                'mr-2 h-4 w-4',
                                sourceFilter === 'all' ? 'opacity-100' : 'opacity-0'
                              )}
                            />
                            All Sources
                          </CommandItem>
                          {uniqueSources.map((source) => (
                            <CommandItem
                              key={source}
                              value={source}
                              onSelect={(currentValue) => {
                                setSourceFilter(currentValue);
                                setSourceOpen(false);
                              }}
                            >
                              <Check
                                className={cn(
                                  'mr-2 h-4 w-4',
                                  sourceFilter === source ? 'opacity-100' : 'opacity-0'
                                )}
                              />
                              {source}
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
              {filteredEvents.length} of {events.length} events
            </div>
          </div>

          {/* Events List */}
          <div className="space-y-2">
            {filteredEvents.length === 0 ? (
              <div className="text-center text-muted-foreground py-8">
                {events.length === 0 ? 'No events received yet' : 'No events match current filters'}
              </div>
            ) : (
              filteredEvents.map((event) => {
                const isExpanded = expandedEvents.has(event.id);
                const showRaw = showRawData.has(event.id);
                const hasContext = event.context && Object.keys(event.context).length > 0;

                return (
                  <div
                    key={`${event.id}-${event.timestamp}`}
                    className="border rounded-lg hover:bg-muted/50"
                  >
                    <div className="flex items-start gap-3 p-3">
                      <div className="flex-shrink-0 mt-0.5">
                        <Badge
                          variant={getLevelBadgeVariant(event.level)}
                          className="text-xs gap-1"
                        >
                          {getLevelIcon(event.level)}
                          {event.level}
                        </Badge>
                      </div>

                      <div className="flex-1 min-w-0">
                        <div className="flex items-start justify-between gap-2">
                          <p className="text-sm font-medium leading-tight">{event.message}</p>
                          <div className="flex items-center gap-2 flex-shrink-0">
                            <span className="text-xs text-muted-foreground">
                              {new Date(event.timestamp).toLocaleTimeString()}
                            </span>

                            {/* Expand/Collapse Button */}
                            {hasContext && (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => toggleEventExpansion(event.id)}
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
                              onClick={() => toggleRawData(event.id)}
                              className="h-6 w-6 p-0"
                              title="Toggle raw data"
                            >
                              <Code className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>

                        <div className="flex items-center gap-2 mt-1">
                          <Badge variant="outline" className="text-xs">
                            {event.source}
                          </Badge>
                          {event.context?.state && (
                            <Badge
                              variant={
                                event.context.state === 'completed'
                                  ? 'default'
                                  : event.context.state === 'failed'
                                    ? 'destructive'
                                    : 'secondary'
                              }
                              className="text-xs"
                            >
                              {event.context.state}
                            </Badge>
                          )}
                          {event.context?.progress?.percentage !== null &&
                            event.context?.progress?.percentage !== undefined && (
                              <Badge variant="outline" className="text-xs">
                                {Math.round(event.context.progress.percentage)}%
                              </Badge>
                            )}
                          {hasContext && (
                            <Badge variant="outline" className="text-xs">
                              +{Object.keys(event.context || {}).length} metadata
                            </Badge>
                          )}
                        </div>

                        {/* Expanded Context */}
                        {isExpanded && hasContext && renderEventContext(event.context)}

                        {/* Raw Data */}
                        {showRaw && (
                          <div className="mt-2 p-2 bg-muted rounded text-xs">
                            <div className="font-medium mb-1">Raw Data:</div>
                            <pre className="whitespace-pre-wrap font-mono">
                              {JSON.stringify(event, null, 2)}
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
