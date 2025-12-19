'use client';

import { useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
  Copy,
  Check,
} from 'lucide-react';
import { Daemon, DaemonState } from '@/types/api';
import { GPUSessionStatus } from './GPUSessionStatus';
import { BadgeGroup, BadgeItem, BadgePriority } from '@/components/shared/BadgeGroup';
import { ScrollArea } from '@/components/ui/scroll-area';

interface TranscoderDetailPanelProps {
  daemon: Daemon;
  onDrain?: (id: string) => Promise<void>;
  onActivate?: (id: string) => Promise<void>;
}

function getStatePriority(state: DaemonState): BadgePriority {
  switch (state) {
    case 'connected':
      return 'secondary'; // Dark gray like "idle" states
    case 'draining':
      return 'warning';
    case 'unhealthy':
      return 'error';
    case 'disconnected':
      return 'error'; // Red for disconnected
    case 'connecting':
      return 'info';
    default:
      return 'default';
  }
}

function getStateColor(state: DaemonState): string {
  switch (state) {
    case 'connected':
      return 'text-muted-foreground';
    case 'draining':
      return 'text-yellow-500';
    case 'unhealthy':
      return 'text-red-500';
    case 'disconnected':
      return 'text-red-500';
    case 'connecting':
      return 'text-blue-500';
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

  if (days > 0) return `${days}d ${hours}h ${mins}m`;
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

export function TranscoderDetailPanel({ daemon, onDrain, onActivate }: TranscoderDetailPanelProps) {
  const [isHwAccelExpanded, setIsHwAccelExpanded] = useState(true);
  const [isCapabilitiesExpanded, setIsCapabilitiesExpanded] = useState(false);
  const [isJobsExpanded, setIsJobsExpanded] = useState(true);
  const [isActioning, setIsActioning] = useState(false);
  const [copiedId, setCopiedId] = useState(false);

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

  const handleCopyId = async () => {
    await navigator.clipboard.writeText(daemon.id);
    setCopiedId(true);
    setTimeout(() => setCopiedId(false), 2000);
  };

  const systemStats = daemon.system_stats;
  const capabilities = daemon.capabilities;
  const gpus = capabilities?.gpus ?? [];
  const gpuStats = systemStats?.gpus ?? [];

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="flex items-start justify-between px-6 py-4 border-b">
        <div className="flex items-center gap-3">
          <Server className={`h-6 w-6 ${getStateColor(daemon.state)}`} />
          <div>
            <h2 className="text-lg font-semibold">{daemon.name}</h2>
            <p className="text-sm text-muted-foreground">{daemon.address}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <BadgeGroup
            badges={[
              { label: daemon.state, priority: getStatePriority(daemon.state) },
              ...(daemon.active_jobs > 0 ? [{ label: `${daemon.active_jobs} job${daemon.active_jobs > 1 ? 's' : ''}`, priority: 'info' as BadgePriority }] : []),
            ]}
            animate={daemon.active_jobs > 0 ? 'sparkle' : 'none'}
          />
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
              <DropdownMenuItem onClick={handleCopyId}>
                {copiedId ? (
                  <Check className="mr-2 h-4 w-4 text-green-500" />
                ) : (
                  <Copy className="mr-2 h-4 w-4" />
                )}
                Copy ID
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <ScrollArea className="flex-1 min-h-0">
        <div className="p-6 space-y-6">
          {/* Quick Stats */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="rounded-lg border p-3">
              <div className="flex items-center gap-2 text-muted-foreground mb-1">
                <Activity className="h-4 w-4" />
                <span className="text-xs font-medium">Active Jobs</span>
              </div>
              <p className="text-2xl font-bold">{daemon.active_jobs}</p>
            </div>
            <div className="rounded-lg border p-3">
              <div className="flex items-center gap-2 text-muted-foreground mb-1">
                <Clock className="h-4 w-4" />
                <span className="text-xs font-medium">Uptime</span>
              </div>
              <p className="text-2xl font-bold">{formatUptime(daemon.connected_at)}</p>
            </div>
            <div className="rounded-lg border p-3">
              <div className="flex items-center gap-2 text-muted-foreground mb-1">
                <Cpu className="h-4 w-4" />
                <span className="text-xs font-medium">CPU Usage</span>
              </div>
              <p className="text-2xl font-bold">{systemStats?.cpu_percent.toFixed(0) ?? '-'}%</p>
            </div>
            <div className="rounded-lg border p-3">
              <div className="flex items-center gap-2 text-muted-foreground mb-1">
                <MemoryStick className="h-4 w-4" />
                <span className="text-xs font-medium">Memory</span>
              </div>
              <p className="text-2xl font-bold">{systemStats?.memory_percent.toFixed(0) ?? '-'}%</p>
            </div>
          </div>

          {/* System Resources */}
          {systemStats && (
            <div className="space-y-4">
              <h3 className="text-sm font-medium">System Resources</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">CPU</span>
                    <span>{systemStats.cpu_percent.toFixed(1)}%</span>
                  </div>
                  <Progress value={systemStats.cpu_percent} className="h-2" />
                </div>
                <div className="space-y-2">
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">Memory</span>
                    <span>
                      {formatBytes(systemStats.memory_used)} / {formatBytes(systemStats.memory_total)}
                    </span>
                  </div>
                  <Progress value={systemStats.memory_percent} className="h-2" />
                </div>
              </div>
            </div>
          )}

          {/* GPU Sessions */}
          {gpus.length > 0 && (
            <div className="space-y-3">
              <h3 className="text-sm font-medium">GPU Sessions</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {gpus.map((gpu) => (
                  <GPUSessionStatus
                    key={gpu.index}
                    gpu={gpu}
                    stats={gpuStats.find((s) => s.index === gpu.index)}
                  />
                ))}
              </div>
            </div>
          )}

          {/* Transcode Jobs */}
          <Collapsible open={isJobsExpanded} onOpenChange={setIsJobsExpanded}>
            <div className="rounded-lg border">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-between h-10 px-4 rounded-lg hover:bg-muted/50"
                >
                  <div className="flex items-center gap-2">
                    <Activity className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm font-medium">Transcode Jobs</span>
                  </div>
                  {isJobsExpanded ? (
                    <ChevronUp className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="px-4 pb-4">
                <div className="grid grid-cols-2 gap-4 pt-2">
                  <div className="rounded-lg bg-muted/50 p-3">
                    <span className="text-muted-foreground text-sm">Completed</span>
                    <p className="text-2xl font-bold text-green-500">{daemon.total_jobs_completed}</p>
                  </div>
                  <div className="rounded-lg bg-muted/50 p-3">
                    <span className="text-muted-foreground text-sm">Failed</span>
                    <p className="text-2xl font-bold text-red-500">{daemon.total_jobs_failed}</p>
                  </div>
                </div>
              </CollapsibleContent>
            </div>
          </Collapsible>

          {/* Hardware Acceleration */}
          {(capabilities?.hw_accels?.length ?? 0) > 0 && (
            <Collapsible open={isHwAccelExpanded} onOpenChange={setIsHwAccelExpanded}>
              <div className="rounded-lg border">
                <CollapsibleTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="w-full justify-between h-10 px-4 rounded-lg hover:bg-muted/50"
                  >
                    <div className="flex items-center gap-2">
                      <Zap className="h-4 w-4 text-muted-foreground" />
                      <span className="text-sm font-medium">Hardware Acceleration</span>
                    </div>
                    {isHwAccelExpanded ? (
                      <ChevronUp className="h-4 w-4 text-muted-foreground" />
                    ) : (
                      <ChevronDown className="h-4 w-4 text-muted-foreground" />
                    )}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="px-4 pb-4">
                  <div className="flex flex-wrap gap-2 pt-2">
                    {capabilities?.hw_accels?.map((hw) => (
                      <Badge
                        key={hw.type}
                        variant={hw.available ? 'default' : 'outline'}
                        className="text-sm"
                      >
                        {hw.type}
                      </Badge>
                    ))}
                  </div>
                </CollapsibleContent>
              </div>
            </Collapsible>
          )}

          {/* Encoders */}
          {capabilities &&
            ((capabilities.video_encoders?.length ?? 0) > 0 ||
              (capabilities.audio_encoders?.length ?? 0) > 0) && (
              <Collapsible open={isCapabilitiesExpanded} onOpenChange={setIsCapabilitiesExpanded}>
                <div className="rounded-lg border">
                  <CollapsibleTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="w-full justify-between h-10 px-4 rounded-lg hover:bg-muted/50"
                    >
                      <div className="flex items-center gap-2">
                        <Settings className="h-4 w-4 text-muted-foreground" />
                        <span className="text-sm font-medium">Encoders</span>
                      </div>
                      {isCapabilitiesExpanded ? (
                        <ChevronUp className="h-4 w-4 text-muted-foreground" />
                      ) : (
                        <ChevronDown className="h-4 w-4 text-muted-foreground" />
                      )}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="px-4 pb-4 space-y-4">
                    {capabilities.video_encoders?.length > 0 && (
                      <div>
                        <p className="text-sm font-medium text-muted-foreground mb-2 pt-2">Video</p>
                        <div className="flex flex-wrap gap-2">
                          {capabilities.video_encoders.map((enc) => (
                            <Badge key={enc} variant="outline" className="text-sm">
                              {enc}
                            </Badge>
                          ))}
                        </div>
                      </div>
                    )}
                    {capabilities.audio_encoders?.length > 0 && (
                      <div>
                        <p className="text-sm font-medium text-muted-foreground mb-2">Audio</p>
                        <div className="flex flex-wrap gap-2">
                          {capabilities.audio_encoders.map((enc) => (
                            <Badge key={enc} variant="outline" className="text-sm">
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

          {/* Footer Info */}
          <div className="flex items-center justify-between text-xs text-muted-foreground pt-4 border-t">
            <span>version: {daemon.version}</span>
            <span className="font-mono">{daemon.id}</span>
          </div>
        </div>
      </ScrollArea>
    </div>
  );
}
