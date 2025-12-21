'use client';

import { useState, useEffect } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
  Activity,
  Settings,
  Copy,
  Check,
} from 'lucide-react';
import { Daemon, DaemonState, EncoderOverride } from '@/types/api';
import { GPUSessionStatus } from './GPUSessionStatus';
import { ActiveJobStats } from './ActiveJobStats';
import { BadgeGroup, BadgeItem, BadgePriority } from '@/components/shared/BadgeGroup';
import { ScrollArea } from '@/components/ui/scroll-area';
import { apiClient } from '@/lib/api-client';

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
      return 'text-yellow-600 dark:text-yellow-400';
    case 'unhealthy':
      return 'text-red-600 dark:text-red-400';
    case 'disconnected':
      return 'text-red-600 dark:text-red-400';
    case 'connecting':
      return 'text-blue-600 dark:text-blue-400';
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
  const [isCapabilitiesExpanded, setIsCapabilitiesExpanded] = useState(false);
  const [isJobsExpanded, setIsJobsExpanded] = useState(true);
  const [isActioning, setIsActioning] = useState(false);
  const [copiedId, setCopiedId] = useState(false);
  const [encoderOverrides, setEncoderOverrides] = useState<EncoderOverride[]>([]);

  // Fetch enabled encoder overrides
  useEffect(() => {
    const fetchOverrides = async () => {
      try {
        const overrides = await apiClient.getEncoderOverrides();
        // Only keep enabled overrides
        setEncoderOverrides(overrides.filter(o => o.is_enabled));
      } catch (error) {
        console.error('Failed to fetch encoder overrides:', error);
      }
    };
    fetchOverrides();
  }, []);

  // Helper to get matching overrides for a hwaccel type
  const getMatchingOverrides = (hwType: string): EncoderOverride[] => {
    return encoderOverrides.filter(o =>
      o.codec_type === 'video' &&
      (!o.hw_accel_match || o.hw_accel_match === hwType)
    );
  };

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
                  <Check className="mr-2 h-4 w-4 text-green-600 dark:text-green-400" />
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
        <div className="p-6 space-y-4">
          {/* Compact Resource Summary */}
          <div className="space-y-3">
            {/* Inline stats row */}
            <div className="flex flex-wrap items-center gap-x-6 gap-y-1 text-sm">
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Uptime</span>
                <span className="font-medium">{formatUptime(daemon.connected_at)}</span>
              </div>
              {systemStats && (
                <>
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground">CPU</span>
                    <span className="font-medium">{systemStats.cpu_percent.toFixed(0)}%</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-muted-foreground">Memory</span>
                    <span className="font-medium">
                      {formatBytes(systemStats.memory_used)} / {formatBytes(systemStats.memory_total)}
                    </span>
                  </div>
                </>
              )}
            </div>

            {/* GPU Sessions */}
            {gpus.length > 0 && (
              <div className="space-y-2">
                {gpus.map((gpu) => (
                  <GPUSessionStatus key={gpu.index} gpu={gpu} />
                ))}
              </div>
            )}
          </div>

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
                    {daemon.active_jobs > 0 && (
                      <Badge variant="secondary" className="h-5 px-1.5 text-xs">
                        {daemon.active_jobs} active
                      </Badge>
                    )}
                  </div>
                  {isJobsExpanded ? (
                    <ChevronUp className="h-4 w-4 text-muted-foreground" />
                  ) : (
                    <ChevronDown className="h-4 w-4 text-muted-foreground" />
                  )}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="px-4 pb-4">
                {/* Active job details with stats */}
                {daemon.active_job_details && daemon.active_job_details.length > 0 ? (
                  <div className="pt-2">
                    <ActiveJobStats jobs={daemon.active_job_details} />
                  </div>
                ) : (
                  <div className="pt-2 text-sm text-muted-foreground">
                    No active transcode jobs
                  </div>
                )}
              </CollapsibleContent>
            </div>
          </Collapsible>

          {/* Capabilities (HW Accel + Encoders combined) */}
          {capabilities &&
            ((capabilities.video_encoders?.length ?? 0) > 0 ||
              (capabilities.audio_encoders?.length ?? 0) > 0 ||
              (capabilities.hw_accels?.length ?? 0) > 0) && (
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
                        <span className="text-sm font-medium">Capabilities</span>
                        {(capabilities.hw_accels?.filter(hw => hw.available).length ?? 0) > 0 && (
                          <Badge variant="secondary" className="h-5 px-1.5 text-xs">
                            {capabilities.hw_accels?.filter(hw => hw.available).map(hw => hw.type).join(', ')}
                          </Badge>
                        )}
                      </div>
                      {isCapabilitiesExpanded ? (
                        <ChevronUp className="h-4 w-4 text-muted-foreground" />
                      ) : (
                        <ChevronDown className="h-4 w-4 text-muted-foreground" />
                      )}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="px-4 pb-4 space-y-3">
                    {/* Hardware Accelerated Encoders */}
                    <TooltipProvider delayDuration={100}>
                      {capabilities.hw_accels?.filter(hw => hw.available).map((hw) => {
                        const matchingOverrides = getMatchingOverrides(hw.type);
                        return (
                          <div key={hw.type} className="pt-2 first:pt-2">
                            <p className="text-xs font-medium text-muted-foreground mb-1.5">
                              {hw.type.toUpperCase()} {hw.device && <span className="font-normal">({hw.device})</span>}
                            </p>
                            <div className="flex flex-wrap gap-1.5">
                              {hw.hw_encoders?.map((enc) => (
                                <Badge key={enc} variant="default" className="text-xs">
                                  {enc}
                                </Badge>
                              ))}
                              {hw.hw_decoders?.map((dec) => (
                                <Badge key={dec} variant="secondary" className="text-xs">
                                  {dec}
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
                              {matchingOverrides.map((override) => (
                                <Tooltip key={override.id}>
                                  <TooltipTrigger asChild>
                                    <Badge className="text-xs cursor-help bg-yellow-600 hover:bg-yellow-600/80 text-white">
                                      {override.source_codec} → {override.target_encoder}
                                    </Badge>
                                  </TooltipTrigger>
                                  <TooltipContent side="top" className="max-w-xs">
                                    <p className="text-xs font-medium">{override.name}</p>
                                    {override.description && (
                                      <p className="text-xs text-muted-foreground mt-1">{override.description}</p>
                                    )}
                                  </TooltipContent>
                                </Tooltip>
                              ))}
                            </div>
                          </div>
                        );
                      })}
                    </TooltipProvider>

                    {/* Software Encoders */}
                    {((capabilities.video_encoders?.length ?? 0) > 0 || (capabilities.audio_encoders?.length ?? 0) > 0) && (
                      <div className="pt-2 border-t">
                        <p className="text-xs font-medium text-muted-foreground mb-1.5">Software</p>
                        <div className="flex flex-wrap gap-1.5">
                          {capabilities.video_encoders?.map((enc) => (
                            <Badge key={enc} variant="outline" className="text-xs">
                              {enc}
                            </Badge>
                          ))}
                          {capabilities.audio_encoders?.map((enc) => (
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
