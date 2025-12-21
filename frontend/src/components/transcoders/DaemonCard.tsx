'use client';

import { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Server,
  MoreVertical,
  Pause,
  Play,
  ChevronDown,
  ChevronUp,
  Cpu,
  MemoryStick,
  Clock,
  Activity,
  Zap,
  Settings,
} from 'lucide-react';
import { Daemon, DaemonState } from '@/types/api';
import { GPUSessionStatus } from './GPUSessionStatus';

interface DaemonCardProps {
  daemon: Daemon;
  onDrain?: (id: string) => Promise<void>;
  onActivate?: (id: string) => Promise<void>;
}

function getStateBadgeVariant(state: DaemonState): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (state) {
    case 'connected':
      return 'secondary'; // Dark gray like "idle" states
    case 'transcoding':
      return 'default'; // Blue/primary for active transcoding
    case 'draining':
      return 'secondary';
    case 'unhealthy':
      return 'destructive';
    case 'disconnected':
      return 'destructive'; // Red for disconnected
    case 'connecting':
      return 'secondary';
    default:
      return 'outline';
  }
}

// TODO: Add success/warning/info theme colors to globals.css for proper semantic styling
function getStateColor(state: DaemonState): string {
  switch (state) {
    case 'connected':
      return 'text-muted-foreground';
    case 'transcoding':
      return 'text-primary'; // Active state - uses theme primary
    case 'draining':
      return 'text-yellow-600 dark:text-yellow-400'; // TODO: needs --warning theme color
    case 'unhealthy':
      return 'text-destructive';
    case 'disconnected':
      return 'text-destructive';
    case 'connecting':
      return 'text-primary';
    default:
      return 'text-muted-foreground';
  }
}

function formatUptime(connectedAt: string): string {
  const connected = new Date(connectedAt);
  const now = new Date();
  const diffMs = now.getTime() - connected.getTime();
  const diffSecs = Math.floor(diffMs / 1000);
  const days = Math.floor(diffSecs / 86400);
  const hours = Math.floor((diffSecs % 86400) / 3600);
  const mins = Math.floor((diffSecs % 3600) / 60);

  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export function DaemonCard({ daemon, onDrain, onActivate }: DaemonCardProps) {
  const [isHwAccelExpanded, setIsHwAccelExpanded] = useState(false);
  const [isCapabilitiesExpanded, setIsCapabilitiesExpanded] = useState(false);
  const [isActioning, setIsActioning] = useState(false);

  const handleDrain = async () => {
    if (!onDrain) return;
    setIsActioning(true);
    try {
      await onDrain(daemon.id);
    } finally {
      setIsActioning(false);
    }
  };

  const handleActivate = async () => {
    if (!onActivate) return;
    setIsActioning(true);
    try {
      await onActivate(daemon.id);
    } finally {
      setIsActioning(false);
    }
  };

  const systemStats = daemon.system_stats;
  const capabilities = daemon.capabilities;
  const gpus = capabilities?.gpus ?? [];
  const gpuStats = systemStats?.gpus ?? [];

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2">
            <Server className={`h-5 w-5 ${getStateColor(daemon.state)}`} />
            <div>
              <CardTitle className="text-base">{daemon.name}</CardTitle>
              <p className="text-xs text-muted-foreground">{daemon.address}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant={getStateBadgeVariant(daemon.state)}>
              {daemon.state}
            </Badge>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8">
                  <MoreVertical className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {daemon.state === 'connected' && onDrain && (
                  <DropdownMenuItem onClick={handleDrain} disabled={isActioning}>
                    <Pause className="mr-2 h-4 w-4" />
                    Drain
                  </DropdownMenuItem>
                )}
                {daemon.state === 'draining' && onActivate && (
                  <DropdownMenuItem onClick={handleActivate} disabled={isActioning}>
                    <Play className="mr-2 h-4 w-4" />
                    Activate
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem disabled className="text-xs text-muted-foreground">
                  ID: {daemon.id}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {/* Quick Stats Row */}
        <div className="grid grid-cols-4 gap-2 text-xs">
          <div className="flex items-center gap-1">
            <Activity className="h-3 w-3 text-muted-foreground" />
            <span>{daemon.active_jobs} jobs</span>
          </div>
          <div className="flex items-center gap-1">
            <Clock className="h-3 w-3 text-muted-foreground" />
            <span>{formatUptime(daemon.connected_at)}</span>
          </div>
          <div className="flex items-center gap-1">
            <Cpu className="h-3 w-3 text-muted-foreground" />
            <span>{systemStats?.cpu_percent.toFixed(0) ?? '-'}%</span>
          </div>
          <div className="flex items-center gap-1">
            <MemoryStick className="h-3 w-3 text-muted-foreground" />
            <span>{systemStats?.memory_percent.toFixed(0) ?? '-'}%</span>
          </div>
        </div>

        {/* System Resource Bars */}
        {systemStats && (
          <div className="space-y-2">
            <div className="space-y-1">
              <div className="flex justify-between text-xs">
                <span className="text-muted-foreground">CPU</span>
                <span>{systemStats.cpu_percent.toFixed(1)}%</span>
              </div>
              <Progress value={systemStats.cpu_percent} className="h-1.5" />
            </div>
            <div className="space-y-1">
              <div className="flex justify-between text-xs">
                <span className="text-muted-foreground">Memory</span>
                <span>
                  {formatBytes(systemStats.memory_used)} / {formatBytes(systemStats.memory_total)}
                </span>
              </div>
              <Progress value={systemStats.memory_percent} className="h-1.5" />
            </div>
          </div>
        )}

        {/* GPU Sessions */}
        {gpus.length > 0 && (
          <div className="space-y-2">
            <div className="text-xs font-medium text-muted-foreground">GPU Sessions</div>
            <div className="space-y-2">
              {gpus.map((gpu, idx) => (
                <GPUSessionStatus
                  key={gpu.index}
                  gpu={gpu}
                  stats={gpuStats.find((s) => s.index === gpu.index)}
                />
              ))}
            </div>
          </div>
        )}

        {/* Hardware Acceleration Section */}
        {(capabilities?.hw_accels?.length ?? 0) > 0 && (
          <Collapsible open={isHwAccelExpanded} onOpenChange={setIsHwAccelExpanded}>
            <div className="rounded-lg border">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-between h-9 px-3 rounded-lg hover:bg-muted/50"
                >
                  <div className="flex items-center gap-2">
                    <Zap className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="text-xs font-medium">Hardware Acceleration</span>
                  </div>
                  {isHwAccelExpanded ? (
                    <ChevronUp className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="px-3 pb-3">
                <TooltipProvider delayDuration={100}>
                  {capabilities?.hw_accels?.map((hw) => (
                    <div key={hw.type} className="pt-2 first:pt-1">
                      <div className="text-xs font-medium text-muted-foreground mb-1.5">
                        {hw.type.toUpperCase()} {hw.device && <span className="font-normal">({hw.device})</span>}
                      </div>

                      {/* HW Encoders */}
                      {((hw.hw_encoders?.length ?? 0) > 0 || (hw.filtered_encoders?.length ?? 0) > 0) && (
                        <div className="mb-2">
                          <div className="text-xs text-muted-foreground mb-1">HW Encoders</div>
                          <div className="flex flex-wrap gap-1">
                            {hw.hw_encoders?.map((enc) => (
                              <Badge key={enc} variant="default" className="text-xs">
                                {enc}
                              </Badge>
                            ))}
                            {hw.filtered_encoders?.map((fe) => (
                              <Tooltip key={fe.name}>
                                <TooltipTrigger asChild>
                                  <Badge variant="destructive" className="text-xs opacity-70 cursor-help">
                                    {fe.name}
                                  </Badge>
                                </TooltipTrigger>
                                <TooltipContent side="top" className="max-w-xs">
                                  <p className="text-xs">{fe.reason}</p>
                                </TooltipContent>
                              </Tooltip>
                            ))}
                          </div>
                        </div>
                      )}

                      {/* HW Decoders */}
                      {hw.hw_decoders?.length > 0 && (
                        <div>
                          <div className="text-xs text-muted-foreground mb-1">HW Decoders</div>
                          <div className="flex flex-wrap gap-1">
                            {hw.hw_decoders.map((dec) => (
                              <Badge key={dec} variant="secondary" className="text-xs">
                                {dec}
                              </Badge>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </TooltipProvider>
              </CollapsibleContent>
            </div>
          </Collapsible>
        )}

        {/* Capabilities Section */}
        {capabilities &&
          ((capabilities.video_encoders?.length ?? 0) > 0 ||
            (capabilities.audio_encoders?.length ?? 0) > 0) && (
            <Collapsible open={isCapabilitiesExpanded} onOpenChange={setIsCapabilitiesExpanded}>
              <div className="rounded-lg border">
                <CollapsibleTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="w-full justify-between h-9 px-3 rounded-lg hover:bg-muted/50"
                  >
                    <div className="flex items-center gap-2">
                      <Settings className="h-3.5 w-3.5 text-muted-foreground" />
                      <span className="text-xs font-medium">Encoders</span>
                    </div>
                    {isCapabilitiesExpanded ? (
                      <ChevronUp className="h-4 w-4 text-muted-foreground" />
                    ) : (
                      <ChevronDown className="h-4 w-4 text-muted-foreground" />
                    )}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="px-3 pb-3 space-y-3">
                  {capabilities.video_encoders?.length > 0 && (
                    <div>
                      <div className="text-xs font-medium text-muted-foreground mb-1 pt-1">
                        Video
                      </div>
                      <div className="flex flex-wrap gap-1">
                        {capabilities.video_encoders.map((enc) => (
                          <Badge key={enc} variant="outline" className="text-xs">
                            {enc}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                  {capabilities.audio_encoders?.length > 0 && (
                    <div>
                      <div className="text-xs font-medium text-muted-foreground mb-1">Audio</div>
                      <div className="flex flex-wrap gap-1">
                        {capabilities.audio_encoders.map((enc) => (
                          <Badge key={enc} variant="outline" className="text-xs">
                            {enc}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}
                </CollapsibleContent>
              </div>
            </Collapsible>
          )}

        {/* Footer */}
        <div className="text-xs text-muted-foreground pt-2 border-t">
          version: {daemon.version}
        </div>
      </CardContent>
    </Card>
  );
}
