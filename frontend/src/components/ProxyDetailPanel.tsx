'use client';

import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Edit,
  Trash2,
  Copy,
  Check,
  RefreshCw,
  Settings,
  AlertCircle,
  Play,
  Calendar,
  Filter as FilterIcon,
  Database,
  Loader2,
  Clock,
} from 'lucide-react';
import { StreamProxy, EncodingProfile, StreamSourceResponse, EpgSourceResponse, Filter } from '@/types/api';
import { apiClient } from '@/lib/api-client';
import { formatDate, formatRelativeTime } from '@/lib/utils';
import { RefreshButton } from '@/components/RefreshButton';
import { OperationStatusIndicator } from '@/components/OperationStatusIndicator';

interface ProxyDetailPanelProps {
  proxy: StreamProxy;
  onEdit: () => void;
  onDelete: () => void;
  onRegenerate: () => Promise<void>;
  isRegenerating: boolean;
  isOnline: boolean;
  isDeleting: boolean;
  encodingProfiles: EncodingProfile[];
}

interface ProxyDetails extends StreamProxy {
  stream_sources?: Array<{ source_id: string; priority_order: number; source?: { name: string } }>;
  epg_sources?: Array<{ epg_source_id: string; priority_order: number; epg_source?: { name: string } }>;
  filters?: Array<{ filter_id: string; priority_order: number; is_active: boolean; filter?: { name: string } }>;
}

interface NameMaps {
  streamSources: Map<string, string>;
  epgSources: Map<string, string>;
  filters: Map<string, string>;
}

export function ProxyDetailPanel({
  proxy,
  onEdit,
  onDelete,
  onRegenerate,
  isRegenerating,
  isOnline,
  isDeleting,
  encodingProfiles,
}: ProxyDetailPanelProps) {
  const [copiedUrls, setCopiedUrls] = useState<Set<string>>(new Set());
  const [details, setDetails] = useState<ProxyDetails | null>(null);
  const [loadingDetails, setLoadingDetails] = useState(true);
  const [nameMaps, setNameMaps] = useState<NameMaps>({
    streamSources: new Map(),
    epgSources: new Map(),
    filters: new Map(),
  });

  // Load detailed proxy info and name maps
  useEffect(() => {
    const loadDetails = async () => {
      setLoadingDetails(true);
      try {
        // Fetch proxy details and all sources/filters in parallel
        const [proxyResponse, streamRes, epgRes, filterRes] = await Promise.all([
          apiClient.getProxy(proxy.id),
          apiClient.getStreamSources(),
          apiClient.getEpgSources(),
          apiClient.getFilters(),
        ]);

        setDetails((proxyResponse.data || proxyResponse) as ProxyDetails);

        // Build name lookup maps
        const streamSourceMap = new Map<string, string>();
        (streamRes.items || []).forEach((s: StreamSourceResponse) => {
          streamSourceMap.set(s.id, s.name);
        });

        const epgSourceMap = new Map<string, string>();
        (epgRes.items || []).forEach((s: EpgSourceResponse) => {
          epgSourceMap.set(s.id, s.name);
        });

        const filterMap = new Map<string, string>();
        (Array.isArray(filterRes) ? filterRes : []).forEach((f: Filter) => {
          filterMap.set(f.id, f.name);
        });

        setNameMaps({
          streamSources: streamSourceMap,
          epgSources: epgSourceMap,
          filters: filterMap,
        });
      } catch (err) {
        console.error('Failed to load proxy details:', err);
        setDetails(null);
      } finally {
        setLoadingDetails(false);
      }
    };
    loadDetails();
  }, [proxy.id]);

  const copyToClipboard = async (text: string, urlType: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedUrls((prev) => new Set(prev).add(urlType));
      setTimeout(() => {
        setCopiedUrls((prev) => {
          const newSet = new Set(prev);
          newSet.delete(urlType);
          return newSet;
        });
      }, 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  const getEncodingProfileName = (profileId: string) => {
    const profile = encodingProfiles.find((p) => p.id === profileId);
    return profile ? profile.name : profileId;
  };

  const getStatusColor = (isActive: boolean): string => {
    return isActive ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800';
  };

  return (
    <TooltipProvider>
      <div className="h-full flex flex-col">
        {/* Header */}
        <div className="flex items-start justify-between px-6 py-4 border-b">
          <div className="flex-1 min-w-0">
            <h2 className="text-lg font-semibold truncate">{proxy.name}</h2>
            {proxy.description && (
              <p className="text-sm text-muted-foreground mt-1">{proxy.description}</p>
            )}
          </div>
          <div className="flex items-center gap-2 ml-4">
            <Button size="sm" variant="outline" onClick={onEdit}>
              <Edit className="h-4 w-4 mr-1" />
              Edit
            </Button>
            <RefreshButton
              resourceId={proxy.id}
              onRefresh={onRegenerate}
              disabled={!isOnline}
              size="sm"
              tooltipText="Regenerate"
            />
            <Button
              size="sm"
              variant="outline"
              onClick={onDelete}
              disabled={isDeleting || !isOnline}
              className="text-destructive hover:text-destructive"
            >
              {isDeleting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>

        {/* Content */}
        <ScrollArea className="flex-1 min-h-0">
          <div className="p-6 space-y-6">
            {/* Proxy Info Banner */}
            <div className="flex items-center gap-4 p-4 bg-muted/50 rounded-lg">
              <div className="flex items-center gap-2">
                <Badge variant="secondary">
                  {proxy.proxy_mode.toUpperCase()}
                </Badge>
                <Badge variant={proxy.is_active ? 'secondary' : 'outline'}>
                  {proxy.is_active ? 'Active' : 'Inactive'}
                </Badge>
                {proxy.status === 'failed' && proxy.last_error && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Badge variant="destructive" className="cursor-help">
                        <AlertCircle className="h-3 w-3 mr-1" />
                        Failed
                      </Badge>
                    </TooltipTrigger>
                    <TooltipContent side="bottom" className="max-w-[400px]">
                      <p className="font-medium">Generation Failed</p>
                      <p className="text-xs text-muted-foreground whitespace-pre-wrap">
                        {proxy.last_error}
                      </p>
                    </TooltipContent>
                  </Tooltip>
                )}
                <OperationStatusIndicator resourceId={proxy.id} />
              </div>
              <div className="flex-1" />
              <div className="text-sm text-muted-foreground">
                <Play className="h-4 w-4 inline mr-1" />
                {proxy.channel_count} channels
              </div>
              {proxy.last_generated_at && (
                <div className="text-sm text-muted-foreground">
                  <Clock className="h-4 w-4 inline mr-1" />
                  {formatRelativeTime(proxy.last_generated_at)}
                </div>
              )}
            </div>
            {/* Quick Actions */}
            <div className="flex flex-wrap gap-2">
              {proxy.m3u8_url && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => copyToClipboard(proxy.m3u8_url!, 'm3u8')}
                  disabled={!isOnline}
                >
                  {copiedUrls.has('m3u8') ? (
                    <Check className="h-4 w-4 mr-1 text-green-600" />
                  ) : (
                    <Copy className="h-4 w-4 mr-1" />
                  )}
                  Copy M3U8
                </Button>
              )}
              {proxy.xmltv_url && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => copyToClipboard(proxy.xmltv_url!, 'xmltv')}
                  disabled={!isOnline}
                >
                  {copiedUrls.has('xmltv') ? (
                    <Check className="h-4 w-4 mr-1 text-green-600" />
                  ) : (
                    <Copy className="h-4 w-4 mr-1" />
                  )}
                  Copy XMLTV
                </Button>
              )}
            </div>

            <Separator />

            {/* Statistics */}
            <div>
              <h3 className="text-sm font-medium mb-3">Statistics</h3>
              <div className="grid grid-cols-2 gap-4">
                <div className="rounded-lg border p-3">
                  <div className="text-2xl font-bold">{proxy.channel_count.toLocaleString()}</div>
                  <div className="text-xs text-muted-foreground">Channels</div>
                </div>
                <div className="rounded-lg border p-3">
                  <div className="text-2xl font-bold">{proxy.program_count.toLocaleString()}</div>
                  <div className="text-xs text-muted-foreground">Programs</div>
                </div>
              </div>
            </div>

            <Separator />

            {/* Configuration */}
            <div>
              <h3 className="text-sm font-medium mb-3">Configuration</h3>
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Mode</span>
                  <Badge variant="outline">{proxy.proxy_mode.toUpperCase()}</Badge>
                </div>
                {proxy.proxy_mode === 'smart' && proxy.encoding_profile_id && (
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-muted-foreground">Encoding Profile</span>
                    <span className="text-sm">{getEncodingProfileName(proxy.encoding_profile_id)}</span>
                  </div>
                )}
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Starting Channel</span>
                  <span className="text-sm">Ch {proxy.starting_channel_number}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Numbering Mode</span>
                  <span className="text-sm capitalize">{proxy.numbering_mode}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Upstream Timeout</span>
                  <span className="text-sm">{proxy.upstream_timeout}s</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Max Concurrent</span>
                  <span className="text-sm">
                    {proxy.max_concurrent_streams === 0 ? 'Unlimited' : proxy.max_concurrent_streams}
                  </span>
                </div>
              </div>
            </div>

            <Separator />

            {/* Options */}
            <div>
              <h3 className="text-sm font-medium mb-3">Options</h3>
              <div className="flex flex-wrap gap-2">
                {proxy.auto_regenerate && (
                  <Badge variant="secondary">
                    <RefreshCw className="h-3 w-3 mr-1" />
                    Auto-regenerate
                  </Badge>
                )}
                {proxy.cache_channel_logos && (
                  <Badge variant="secondary">
                    <Settings className="h-3 w-3 mr-1" />
                    Channel Logos
                  </Badge>
                )}
                {proxy.cache_program_logos && (
                  <Badge variant="secondary">
                    <Settings className="h-3 w-3 mr-1" />
                    Program Logos
                  </Badge>
                )}
              </div>
            </div>

            <Separator />

            {/* Sources & Filters */}
            {loadingDetails ? (
              <div className="flex items-center justify-center py-4">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            ) : details ? (
              <>
                {/* Stream Sources */}
                <div>
                  <h3 className="text-sm font-medium mb-3 flex items-center gap-2">
                    <Database className="h-4 w-4" />
                    Stream Sources ({details.stream_sources?.length || 0})
                  </h3>
                  {details.stream_sources && details.stream_sources.length > 0 ? (
                    <div className="space-y-1">
                      {details.stream_sources
                        .sort((a, b) => a.priority_order - b.priority_order)
                        .map((source, index) => (
                          <div
                            key={source.source_id}
                            className="flex items-center gap-2 px-3 py-2 rounded-md bg-muted/50"
                          >
                            <span className="text-xs text-muted-foreground font-mono w-4">
                              {index + 1}.
                            </span>
                            <span className="text-sm truncate">
                              {nameMaps.streamSources.get(source.source_id) || source.source?.name || source.source_id}
                            </span>
                          </div>
                        ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">No stream sources</p>
                  )}
                </div>

                <Separator />

                {/* EPG Sources */}
                <div>
                  <h3 className="text-sm font-medium mb-3 flex items-center gap-2">
                    <Calendar className="h-4 w-4" />
                    EPG Sources ({details.epg_sources?.length || 0})
                  </h3>
                  {details.epg_sources && details.epg_sources.length > 0 ? (
                    <div className="space-y-1">
                      {details.epg_sources
                        .sort((a, b) => a.priority_order - b.priority_order)
                        .map((source, index) => (
                          <div
                            key={source.epg_source_id}
                            className="flex items-center gap-2 px-3 py-2 rounded-md bg-muted/50"
                          >
                            <span className="text-xs text-muted-foreground font-mono w-4">
                              {index + 1}.
                            </span>
                            <span className="text-sm truncate">
                              {nameMaps.epgSources.get(source.epg_source_id) || source.epg_source?.name || source.epg_source_id}
                            </span>
                          </div>
                        ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">No EPG sources</p>
                  )}
                </div>

                <Separator />

                {/* Filters */}
                <div>
                  <h3 className="text-sm font-medium mb-3 flex items-center gap-2">
                    <FilterIcon className="h-4 w-4" />
                    Filters ({details.filters?.length || 0})
                  </h3>
                  {details.filters && details.filters.length > 0 ? (
                    <div className="space-y-1">
                      {details.filters
                        .sort((a, b) => a.priority_order - b.priority_order)
                        .map((filter, index) => (
                          <div
                            key={filter.filter_id}
                            className="flex items-center gap-2 px-3 py-2 rounded-md bg-muted/50"
                          >
                            <span className="text-xs text-muted-foreground font-mono w-4">
                              {index + 1}.
                            </span>
                            <span className={`text-sm truncate ${!filter.is_active ? 'line-through text-muted-foreground' : ''}`}>
                              {nameMaps.filters.get(filter.filter_id) || filter.filter?.name || filter.filter_id}
                            </span>
                            {!filter.is_active && (
                              <Badge variant="outline" className="text-xs">Off</Badge>
                            )}
                          </div>
                        ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted-foreground">No filters</p>
                  )}
                </div>
              </>
            ) : null}

            <Separator />

            {/* Timestamps */}
            <div>
              <h3 className="text-sm font-medium mb-3 flex items-center gap-2">
                <Clock className="h-4 w-4" />
                Timestamps
              </h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">Created</span>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="cursor-help">{formatRelativeTime(proxy.created_at)}</span>
                    </TooltipTrigger>
                    <TooltipContent>{formatDate(proxy.created_at)}</TooltipContent>
                  </Tooltip>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">Updated</span>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="cursor-help">{formatRelativeTime(proxy.updated_at)}</span>
                    </TooltipTrigger>
                    <TooltipContent>{formatDate(proxy.updated_at)}</TooltipContent>
                  </Tooltip>
                </div>
              </div>
            </div>
          </div>
        </ScrollArea>
      </div>
    </TooltipProvider>
  );
}
