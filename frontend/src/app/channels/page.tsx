'use client';

import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Search,
  Play,
  Filter,
  Grid,
  List,
  Eye,
  Zap,
  Check,
  Table as TableIcon,
  Trash2,
  Loader2,
} from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import type { VideoTrackInfo, AudioTrackInfo, SubtitleTrackInfo } from '@/types/api';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Checkbox } from '@/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { VideoPlayerModal } from '@/components/video-player-modal';
import { ChannelDetailsSheet } from '@/components/channel-details-sheet';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';

interface Channel {
  id: string;
  name: string;
  logo_url?: string;
  group?: string;
  stream_url: string;
  /**
   * The original upstream stream URL (before being replaced with the proxy
   * endpoint for playback). Added so the player / sheet UI can reason about
   * the true underlying format (e.g. .ts vs .m3u8) without re-parsing or
   * losing the original value once proxied.
   */
  original_stream_url?: string;
  proxy_id?: string;
  source_type: string;
  source_name?: string;
  // M3U specific fields
  tvg_id?: string;
  tvg_name?: string;
  tvg_chno?: string;
  tvg_shift?: string;
  // Codec / Probe information
  video_codec?: string;
  audio_codec?: string;
  resolution?: string;
  last_probed_at?: string;
  probe_method?: string;
  // Extended probe metadata
  container_format?: string;
  video_width?: number;
  video_height?: number;
  framerate?: string;
  bitrate?: number | null;
  video_bitrate?: number | null;
  audio_bitrate?: number | null;
  audio_channels?: number | null;
  audio_sample_rate?: number | null;
  probe_source?: string;
  // Track information (from probe)
  video_tracks?: VideoTrackInfo[];
  audio_tracks?: AudioTrackInfo[];
  subtitle_tracks?: SubtitleTrackInfo[];
  selected_video_track?: number;
  selected_audio_track?: number;
}

interface ChannelsResponse {
  channels: Channel[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

interface StreamSourceOption {
  id: string;
  name: string;
}

// Helper functions for date formatting
const formatRelativeTime = (dateString: string): string => {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSeconds = Math.floor(diffMs / 1000);
  const diffMinutes = Math.floor(diffSeconds / 60);
  const diffHours = Math.floor(diffMinutes / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSeconds < 60) return 'Just now';
  if (diffMinutes < 60) return `${diffMinutes}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;

  return date.toLocaleDateString();
};

const formatPreciseTime = (dateString: string): string => {
  const date = new Date(dateString);
  return date.toLocaleString();
};

export default function ChannelsPage() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [selectedGroup, setSelectedGroup] = useState<string>('');
  const [selectedSources, setSelectedSources] = useState<string[]>([]);
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');
  const [currentPage, setCurrentPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [groups, setGroups] = useState<string[]>([]);
  const [sources, setSources] = useState<StreamSourceOption[]>([]);
  const [selectedChannel, setSelectedChannel] = useState<Channel | null>(null);
  const [isPlayerOpen, setIsPlayerOpen] = useState(false);
  const [probingChannels, setProbingChannels] = useState<Set<string>>(new Set());
  const loadMoreRef = useRef<HTMLDivElement>(null);
  const isSearchChangeRef = useRef(false);
  const [detailsChannel, setDetailsChannel] = useState<Channel | null>(null);
  const [isDetailsOpen, setIsDetailsOpen] = useState(false);
  const [clearingCache, setClearingCache] = useState(false);

  // No longer need proxy resolution - only using direct stream sources
  // Fetch stream sources (id + name) for reliable ID-based filtering
  useEffect(() => {
    (async () => {
      try {
        const resp = await fetch('/api/v1/sources/stream');
        if (!resp.ok) return;
        const json = await resp.json();
        const items = json?.data?.items ?? json?.data ?? [];
        const mapped: StreamSourceOption[] = items
          .filter((s: any) => s && s.id && s.name)
          .map((s: any) => ({ id: s.id, name: s.name }));
        setSources(mapped);
      } catch (e) {
        Debug.warn('Failed to fetch stream sources', e);
      }
    })();
  }, []);

  // Debounce search input to prevent excessive API calls and focus loss
  useEffect(() => {
    const timer = setTimeout(() => {
      isSearchChangeRef.current = true; // Mark as search change
      setDebouncedSearch(search);
    }, 300); // 300ms debounce

    return () => clearTimeout(timer);
  }, [search]);

  /**
   * Explicit channel fetch that does NOT rely on closure-captured filter state.
   * All filter inputs are passed as arguments so regressions are easier to spot.
   */
  const fetchChannels = useCallback(
    async ({
      searchTerm = '',
      group = '',
      pageNum = 1,
      append = false,
      isSearchChange = false,
      sourceIds = [],
    }: {
      searchTerm?: string;
      group?: string;
      pageNum?: number;
      append?: boolean;
      isSearchChange?: boolean;
      sourceIds?: string[];
    }) => {
      try {
        setLoading(true);

        const params = new URLSearchParams({
          page: pageNum.toString(),
          limit: '200',
        });

        if (searchTerm) params.append('search', searchTerm);
        if (group) params.append('group', group);

        // Multi-source param (backend expects a single comma-separated source_id value)
        if (sourceIds.length > 0) {
          const joined = sourceIds.filter(Boolean).join(',');
          if (joined) {
            params.append('source_id', joined);
          }
        }

        if (Debug.isEnabled()) {
          Debug.log('[Channels] Fetch params', Object.fromEntries(params.entries()));
        }

        const apiUrl = '/api/v1/channels';
        const response = await fetch(`${apiUrl}?${params.toString()}`);

        if (!response.ok) {
          throw new Error(`Failed to fetch channels: ${response.status} ${response.statusText}`);
        }

        // Backend returns { success, items, total, page, per_page, has_next, has_previous }
        const data = await response.json();
        if (!data.success) {
          throw new Error('API returned unsuccessful response');
        }

        const channelsData = data.items || [];

        if (append) {
          setChannels((prev) => {
            const existing = new Set(prev.map((c) => c.id));
            const merged = channelsData.filter((c: Channel) => !existing.has(c.id));
            return [...prev, ...merged];
          });
        } else if (isSearchChange && pageNum === 1) {
          setChannels(channelsData);
        } else {
          setChannels(channelsData);
        }

        setCurrentPage(pageNum);
        setTotal(data.total || 0);
        setHasMore(data.has_next || false);

        if (!append) {
          const uniqueGroups = Array.from(
            new Set(channelsData.map((c: Channel) => c.group).filter(Boolean))
          ) as string[];
          setGroups(uniqueGroups);
        }

        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'An error occurred');
        if (!append) setChannels([]);
      } finally {
        setLoading(false);
      }
    },
    []
  );

  const handleLoadMore = useCallback(() => {
    if (hasMore && !loading) {
      fetchChannels({
        searchTerm: debouncedSearch,
        group: selectedGroup,
        pageNum: currentPage + 1,
        append: true,
        isSearchChange: false,
        sourceIds: selectedSources,
      });
    }
  }, [
    hasMore,
    loading,
    debouncedSearch,
    selectedGroup,
    selectedSources,
    currentPage,
    fetchChannels,
  ]);

  // Single effect that handles both search and filter changes intelligently
  useEffect(() => {
    if (isSearchChangeRef.current) {
      isSearchChangeRef.current = false;
      fetchChannels({
        searchTerm: debouncedSearch,
        group: selectedGroup,
        pageNum: 1,
        append: false,
        isSearchChange: true,
        sourceIds: selectedSources,
      });
    } else {
      setChannels([]);
      setCurrentPage(1);
      fetchChannels({
        searchTerm: debouncedSearch,
        group: selectedGroup,
        pageNum: 1,
        append: false,
        isSearchChange: false,
        sourceIds: selectedSources,
      });
    }
  }, [debouncedSearch, selectedGroup, selectedSources, fetchChannels]);

  // Intersection observer for infinite scroll
  useEffect(() => {
    const loadMoreElement = loadMoreRef.current;
    if (!loadMoreElement) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        // Trigger load more when the element comes into view and we have more data
        // Only trigger on intersection, not when search changes to prevent focus loss
        if (entry.isIntersecting && hasMore && !loading && !debouncedSearch) {
          Debug.log('[Channels] Loading more items via infinite scroll');
          handleLoadMore();
        }
      },
      {
        // Trigger when the element is 200px away from being visible
        rootMargin: '200px',
        threshold: 0.1,
      }
    );

    observer.observe(loadMoreElement);

    return () => {
      observer.unobserve(loadMoreElement);
    };
  }, [hasMore, loading, debouncedSearch, handleLoadMore]);

  const handleSearch = (value: string) => {
    setSearch(value);
  };

  const handleGroupFilter = (value: string) => {
    setSelectedGroup(value === 'all' ? '' : value);
  };

  const handleSourceToggle = (sourceId: string) => {
    setSelectedSources((prev) => {
      if (prev.includes(sourceId)) {
        return prev.filter((s) => s !== sourceId);
      } else {
        return [...prev, sourceId];
      }
    });
  };

  const handleAllSourcesToggle = () => {
    if (selectedSources.length === sources.length) {
      setSelectedSources([]);
    } else {
      setSelectedSources(sources.map((s) => s.id));
    }
  };

  const handlePlayChannel = async (channel: Channel) => {
    try {
      // Construct proxied playback URL using the new channel preview endpoint
      // /proxy/{channelId} provides zero-transcode smart delivery (passthrough/repackage only)
      const proxyUrl = `${getBackendUrl()}/proxy/${channel.id}`;

      setSelectedChannel({
        ...channel,
        // Preserve the original upstream URL (only set once if not already present)
        original_stream_url: channel.original_stream_url ?? channel.stream_url,
        stream_url: proxyUrl,
      });
      setIsPlayerOpen(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load stream');
    }
  };

  const handleProbeChannel = async (channel: Channel) => {
    try {
      setProbingChannels((prev) => new Set(prev).add(channel.id));

      const response = await fetch('/api/v1/relay/probe', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ channel_id: channel.id }),
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(`Failed to probe channel: ${response.status} ${text}`);
      }

      // Relay probe returns flat response with codec info
      const probe = await response.json();

      const updated: Partial<Channel> = {
        video_codec: probe.video_codec,
        audio_codec: probe.audio_codec,
        resolution:
          probe.video_width && probe.video_height
            ? `${probe.video_width}x${probe.video_height}`
            : undefined,
        last_probed_at: new Date().toISOString(),
        probe_method: 'ffprobe',
        video_width: probe.video_width,
        video_height: probe.video_height,
        framerate: probe.video_framerate ?? null,
        video_bitrate: probe.video_bitrate ?? null,
        audio_bitrate: probe.audio_bitrate ?? null,
        audio_channels: probe.audio_channels ?? null,
        audio_sample_rate: probe.audio_sample_rate ?? null,
        container_format: probe.container_format ?? undefined,
        // Track information
        video_tracks: probe.video_tracks ?? [],
        audio_tracks: probe.audio_tracks ?? [],
        subtitle_tracks: probe.subtitle_tracks ?? [],
        selected_video_track: probe.selected_video_track ?? -1,
        selected_audio_track: probe.selected_audio_track ?? -1,
        // Always use proxy URL for playback - upstream URLs may not be directly accessible
        stream_url: `${getBackendUrl()}/proxy/${channel.id}`,
      };

      setChannels((prev) =>
        prev.map((ch) =>
          ch.id === channel.id
            ? {
                ...ch,
                ...Object.fromEntries(Object.entries(updated).filter(([, v]) => v !== undefined)),
              }
            : ch
        )
      );
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to probe channel');
    } finally {
      setProbingChannels((prev) => {
        const newSet = new Set(prev);
        newSet.delete(channel.id);
        return newSet;
      });
    }
  };

  const handleClearCodecCache = async () => {
    if (clearingCache) return;

    try {
      setClearingCache(true);
      const result = await apiClient.clearLastKnownCodecs();
      Debug.log('[Channels] Cleared codec cache:', result);

      // Clear the codec info from displayed channels to reflect cache clear
      setChannels((prev) =>
        prev.map((ch) => ({
          ...ch,
          video_codec: undefined,
          audio_codec: undefined,
          resolution: undefined,
          last_probed_at: undefined,
          probe_method: undefined,
          container_format: undefined,
          video_width: undefined,
          video_height: undefined,
          framerate: undefined,
          bitrate: null,
          video_bitrate: null,
          audio_bitrate: null,
          audio_channels: null,
          audio_sample_rate: null,
          probe_source: undefined,
        }))
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to clear codec cache');
    } finally {
      setClearingCache(false);
    }
  };

  const LogoWithPopover = ({ channel }: { channel: Channel }) => {
    const [imageError, setImageError] = useState(false);
    const [popoverImageError, setPopoverImageError] = useState(false);

    if (!channel.logo_url || imageError) {
      return (
        <div className="w-8 h-8 bg-muted rounded flex items-center justify-center text-muted-foreground text-xs">
          No Logo
        </div>
      );
    }

    return (
      <Popover>
        <PopoverTrigger asChild>
          <div className="cursor-pointer">
            <img
              src={channel.logo_url}
              alt={channel.name}
              className="w-8 h-8 object-contain rounded hover:scale-110 transition-transform"
              onError={() => setImageError(true)}
            />
          </div>
        </PopoverTrigger>
        <PopoverContent className="w-80">
          <div className="space-y-2">
            <h4 className="font-semibold">{channel.name}</h4>
            {popoverImageError ? (
              <div className="w-full max-w-64 h-32 bg-muted rounded flex items-center justify-center mx-auto">
                <span className="text-muted-foreground text-sm">Logo not available</span>
              </div>
            ) : (
              <img
                src={channel.logo_url}
                alt={channel.name}
                className="w-full max-w-64 h-auto object-contain mx-auto"
                onError={() => setPopoverImageError(true)}
              />
            )}
          </div>
        </PopoverContent>
      </Popover>
    );
  };

  const ChannelTableRow = ({ channel }: { channel: Channel }) => {
    // Track counts for styling multi-track badges
    const videoTrackCount = channel.video_tracks?.length ?? 0;
    const audioTrackCount = channel.audio_tracks?.length ?? 0;
    const hasMultipleVideoTracks = videoTrackCount > 1;
    const hasMultipleAudioTracks = audioTrackCount > 1;

    // Build condensed probe info with optional track count indicators
    const buildProbeInfo = (): { text: string; hasMultipleTracks: boolean } | null => {
      if (!channel.video_codec && !channel.audio_codec) {
        return null;
      }
      const videoCodec = channel.video_codec || '?';
      const audioCodec = channel.audio_codec || '?';

      // Format resolution (e.g., "1920x1080" -> "1080p", "1280x720" -> "720p")
      let resolutionShort = '';
      if (channel.resolution) {
        const match = channel.resolution.match(/(\d+)x(\d+)/);
        if (match) {
          resolutionShort = `${match[2]}p`;
        } else {
          resolutionShort = channel.resolution;
        }
      }

      // Format framerate
      let fpsShort = '';
      if (channel.framerate) {
        const frStr = String(channel.framerate);
        if (frStr.includes('/')) {
          const [n, d] = frStr.split('/');
          const v = parseFloat(d) ? Math.round(parseFloat(n) / parseFloat(d)) : frStr;
          fpsShort = String(v);
        } else {
          fpsShort = String(Math.round(parseFloat(frStr)));
        }
      }

      // Build the display string with track count indicators for multiple tracks
      const videoCodecDisplay = hasMultipleVideoTracks ? `${videoCodec}(${videoTrackCount})` : videoCodec;
      const audioCodecDisplay = hasMultipleAudioTracks ? `${audioCodec}(${audioTrackCount})` : audioCodec;
      const codecPart = `${videoCodecDisplay}/${audioCodecDisplay}`;
      let detailPart = '';
      if (resolutionShort && fpsShort) {
        detailPart = ` (${resolutionShort}@${fpsShort})`;
      } else if (resolutionShort) {
        detailPart = ` (${resolutionShort})`;
      } else if (fpsShort) {
        detailPart = ` (@${fpsShort}fps)`;
      }

      return {
        text: `${codecPart}${detailPart}`,
        hasMultipleTracks: hasMultipleVideoTracks || hasMultipleAudioTracks,
      };
    };

    const probeInfo = buildProbeInfo();

    return (
      <TableRow className="hover:bg-muted/50">
        <TableCell className="w-16">
          <LogoWithPopover channel={channel} />
        </TableCell>
        <TableCell className="font-medium max-w-xs">
          <button
            type="button"
            onClick={() => {
              setDetailsChannel(channel);
              setIsDetailsOpen(true);
            }}
            className="truncate text-left w-full hover:underline focus:outline-none focus:ring-2 focus:ring-ring rounded-sm"
            title={channel.name || 'empty'}
          >
            {channel.name && channel.name.trim() !== '' ? (
              channel.name
            ) : (
              <span className="text-muted-foreground italic">empty</span>
            )}
          </button>
        </TableCell>
        <TableCell>
          {channel.group ? (
            <Badge variant="secondary" className="text-xs">
              {channel.group}
            </Badge>
          ) : (
            <span className="text-muted-foreground">-</span>
          )}
        </TableCell>
        <TableCell className="text-xs">
          {probeInfo ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className={`cursor-help font-mono ${probeInfo.hasMultipleTracks ? 'text-blue-600 dark:text-blue-400' : ''}`}>
                  {probeInfo.text}
                </span>
              </TooltipTrigger>
              <TooltipContent className="text-xs space-y-1 max-w-md">
                <div className="font-semibold border-b pb-1 mb-1">Stream Details</div>
                {channel.video_codec && <div>Video Codec: {channel.video_codec}</div>}
                {channel.audio_codec && <div>Audio Codec: {channel.audio_codec}</div>}
                {channel.resolution && <div>Resolution: {channel.resolution}</div>}
                {channel.container_format && <div>Container: {channel.container_format}</div>}
                {channel.framerate != null && channel.framerate !== '' && (
                  <div>Framerate: {channel.framerate}</div>
                )}
                {channel.audio_channels != null && (
                  <div>Audio Channels: {channel.audio_channels}</div>
                )}
                {channel.audio_sample_rate != null && (
                  <div>Audio Sample Rate: {channel.audio_sample_rate} Hz</div>
                )}
                {channel.video_bitrate != null && channel.video_bitrate > 0 && (
                  <div>Video Bitrate: {Math.round(channel.video_bitrate / 1000)} kbps</div>
                )}
                {channel.audio_bitrate != null && channel.audio_bitrate > 0 && (
                  <div>Audio Bitrate: {Math.round(channel.audio_bitrate / 1000)} kbps</div>
                )}
                {channel.bitrate != null && channel.bitrate > 0 && (
                  <div>Total Bitrate: {Math.round(channel.bitrate / 1000)} kbps</div>
                )}
                {/* Video tracks section */}
                {channel.video_tracks && channel.video_tracks.length > 0 && (
                  <div className="border-t pt-1 mt-1">
                    <div className="font-semibold">Video Tracks ({channel.video_tracks.length})</div>
                    {channel.video_tracks.map((track, idx) => (
                      <div key={track.index} className={`pl-2 ${idx === channel.selected_video_track ? 'text-primary font-medium' : 'text-muted-foreground'}`}>
                        #{track.index}: {track.codec.toUpperCase()} {track.width}x{track.height}
                        {track.framerate ? ` @${track.framerate.toFixed(1)}fps` : ''}
                        {track.language ? ` [${track.language}]` : ''}
                        {track.is_default ? ' (default)' : ''}
                      </div>
                    ))}
                  </div>
                )}
                {/* Audio tracks section */}
                {channel.audio_tracks && channel.audio_tracks.length > 0 && (
                  <div className="border-t pt-1 mt-1">
                    <div className="font-semibold">Audio Tracks ({channel.audio_tracks.length})</div>
                    {channel.audio_tracks.map((track, idx) => (
                      <div key={track.index} className={`pl-2 ${idx === channel.selected_audio_track ? 'text-primary font-medium' : 'text-muted-foreground'}`}>
                        #{track.index}: {track.codec.toUpperCase()} {track.channels}ch
                        {track.channel_layout ? ` (${track.channel_layout})` : ''}
                        {track.sample_rate ? ` ${track.sample_rate}Hz` : ''}
                        {track.language ? ` [${track.language}]` : ''}
                        {track.is_default ? ' (default)' : ''}
                      </div>
                    ))}
                  </div>
                )}
                {/* Subtitle tracks section */}
                {channel.subtitle_tracks && channel.subtitle_tracks.length > 0 && (
                  <div className="border-t pt-1 mt-1">
                    <div className="font-semibold">Subtitle Tracks ({channel.subtitle_tracks.length})</div>
                    {channel.subtitle_tracks.map((track) => (
                      <div key={track.index} className="pl-2 text-muted-foreground">
                        #{track.index}: {track.codec}
                        {track.language ? ` [${track.language}]` : ''}
                        {track.is_default ? ' (default)' : ''}
                        {track.is_forced ? ' (forced)' : ''}
                      </div>
                    ))}
                  </div>
                )}
                {channel.probe_source && <div className="border-t pt-1 mt-1">Source: {channel.probe_source}</div>}
                {channel.probe_method && !channel.probe_source && <div className="border-t pt-1 mt-1">Method: {channel.probe_method}</div>}
              </TooltipContent>
            </Tooltip>
          ) : (
            <span className="text-muted-foreground">-</span>
          )}
        </TableCell>
        <TableCell className="text-sm">
          {channel.last_probed_at ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-help text-xs">
                  {formatRelativeTime(channel.last_probed_at)}
                </span>
              </TooltipTrigger>
              <TooltipContent>
                <div className="space-y-1">
                  <div>Method: {channel.probe_method || 'Unknown'}</div>
                  <div>Precise time: {formatPreciseTime(channel.last_probed_at)}</div>
                </div>
              </TooltipContent>
            </Tooltip>
          ) : (
            <span className="text-muted-foreground">-</span>
          )}
        </TableCell>
        <TableCell className="w-32">
          <div className="flex gap-1">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" onClick={() => handlePlayChannel(channel)} className="h-8 px-2">
                  <Play className="w-3 h-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Play channel</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleProbeChannel(channel)}
                  disabled={probingChannels.has(channel.id)}
                  className="h-8 px-2"
                >
                  {probingChannels.has(channel.id) ? (
                    <div className="w-3 h-3 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  ) : (
                    <Zap className="w-3 h-3" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Probe codec information</p>
              </TooltipContent>
            </Tooltip>
          </div>
        </TableCell>
      </TableRow>
    );
  };

  const ChannelCard = ({ channel }: { channel: Channel }) => (
    <Card className="transition-all duration-200 hover:shadow-lg hover:scale-105">
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between">
          <div className="flex-1 min-w-0">
            <CardTitle className="text-sm font-medium truncate">
              <button
                type="button"
                onClick={() => {
                  setDetailsChannel(channel);
                  setIsDetailsOpen(true);
                }}
                className="truncate w-full text-left hover:underline focus:outline-none focus:ring-2 focus:ring-ring rounded-sm"
                title={channel.name || 'empty'}
              >
                {channel.name && channel.name.trim() !== '' ? (
                  channel.name
                ) : (
                  <span className="text-muted-foreground italic">empty</span>
                )}
              </button>
            </CardTitle>
            {channel.group && (
              <CardDescription className="mt-1">
                <Badge variant="secondary" className="text-xs">
                  {channel.group}
                </Badge>
              </CardDescription>
            )}
          </div>
          {channel.logo_url && (
            <img
              src={channel.logo_url}
              alt={channel.name}
              className="w-8 h-8 object-contain ml-2 flex-shrink-0"
              onError={(e) => {
                const img = e.target as HTMLImageElement;
                img.style.display = 'none';
              }}
            />
          )}
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="flex justify-between items-center">
          <div className="flex flex-col text-xs text-muted-foreground">
            <span>Source: {channel.source_name || channel.source_type}</span>
            {channel.tvg_chno && <span>Channel #: {channel.tvg_chno}</span>}
            {channel.video_codec && (
              <div className="flex gap-1 mt-1">
                <Badge variant="outline" className="text-xs">
                  {channel.video_codec}
                </Badge>
                {channel.audio_codec && (
                  <Badge variant="outline" className="text-xs">
                    {channel.audio_codec}
                  </Badge>
                )}
              </div>
            )}
          </div>
          <div className="flex gap-1 ml-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" onClick={() => handlePlayChannel(channel)}>
                  <Play className="w-4 h-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Play channel</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleProbeChannel(channel)}
                  disabled={probingChannels.has(channel.id)}
                >
                  {probingChannels.has(channel.id) ? (
                    <div className="w-4 h-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  ) : (
                    <Zap className="w-4 h-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Probe codec information</p>
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      </CardContent>
    </Card>
  );

  const ChannelListItem = ({ channel }: { channel: Channel }) => (
    <Card className="transition-all duration-200 hover:shadow-md">
      <CardContent className="p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-4">
            {channel.logo_url && (
              <img
                src={channel.logo_url}
                alt={channel.name}
                className="w-10 h-10 object-contain"
                onError={(e) => {
                  const img = e.target as HTMLImageElement;
                  img.style.display = 'none';
                }}
              />
            )}
            <div>
              <h3 className="font-medium">
                <button
                  type="button"
                  onClick={() => {
                    setDetailsChannel(channel);
                    setIsDetailsOpen(true);
                  }}
                  className="hover:underline focus:outline-none focus:ring-2 focus:ring-ring rounded-sm text-left"
                  title={channel.name || 'empty'}
                >
                  {channel.name && channel.name.trim() !== '' ? (
                    channel.name
                  ) : (
                    <span className="text-muted-foreground italic">empty</span>
                  )}
                </button>
              </h3>
              <div className="flex items-center space-x-2 text-sm text-muted-foreground">
                {channel.group && (
                  <Badge variant="secondary" className="text-xs">
                    {channel.group}
                  </Badge>
                )}
                <span>Source: {channel.source_name || channel.source_type}</span>
                {channel.tvg_chno && <span>• Ch #{channel.tvg_chno}</span>}
                {channel.video_codec && (
                  <>
                    <span>•</span>
                    <Badge variant="outline" className="text-xs">
                      {channel.video_codec}
                    </Badge>
                  </>
                )}
                {channel.audio_codec && (
                  <Badge variant="outline" className="text-xs">
                    {channel.audio_codec}
                  </Badge>
                )}
              </div>
            </div>
          </div>
          <div className="flex gap-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" onClick={() => handlePlayChannel(channel)}>
                  <Play className="w-4 h-4 mr-2" />
                  Play
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Play channel</p>
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleProbeChannel(channel)}
                  disabled={probingChannels.has(channel.id)}
                >
                  {probingChannels.has(channel.id) ? (
                    <div className="w-4 h-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  ) : (
                    <Zap className="w-4 h-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>Probe codec information</p>
              </TooltipContent>
            </Tooltip>
          </div>
        </div>
      </CardContent>
    </Card>
  );

  if (loading && channels.length === 0) {
    return (
      <div className="container mx-auto p-6">
        <div className="flex items-center justify-center h-64">
          <div className="text-center">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary mx-auto"></div>
            <p className="mt-4 text-muted-foreground">Loading channels...</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <TooltipProvider>
      <div className="container mx-auto p-6">
        <div className="mb-6">
          <p className="text-muted-foreground">
            Browse and play channels with detailed information and metadata
          </p>
        </div>

        {/* Search and Filters */}
        <Card className="mb-6">
          <CardContent className="p-6">
            <div className="flex flex-col sm:flex-row gap-4">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-muted-foreground w-4 h-4" />
                <Input
                  placeholder="Search channels..."
                  value={search}
                  onChange={(e) => handleSearch(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                    }
                  }}
                  className="pl-10"
                />
              </div>

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" className="w-full sm:w-48 justify-between">
                    <div className="flex items-center">
                      <Filter className="w-4 h-4 mr-2" />
                      <span>
                        {selectedSources.length === 0
                          ? 'All Sources'
                          : selectedSources.length === sources.length
                            ? 'All Sources'
                            : `${selectedSources.length} Source${selectedSources.length > 1 ? 's' : ''}`}
                      </span>
                    </div>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <DropdownMenuItem onClick={handleAllSourcesToggle}>
                    <Checkbox
                      checked={selectedSources.length === sources.length && sources.length > 0}
                      className="mr-2"
                    />
                    All Sources
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  {sources.map((source) => (
                    <DropdownMenuItem key={source.id} onClick={() => handleSourceToggle(source.id)}>
                      <Checkbox checked={selectedSources.includes(source.id)} className="mr-2" />
                      {source.name}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>

              <div className="flex bg-muted rounded-lg p-1">
                <Button
                  variant={viewMode === 'table' ? 'default' : 'ghost'}
                  size="sm"
                  onClick={() => setViewMode('table')}
                  title="Table view"
                >
                  <TableIcon className="w-4 h-4" />
                </Button>
                <Button
                  variant={viewMode === 'grid' ? 'default' : 'ghost'}
                  size="sm"
                  onClick={() => setViewMode('grid')}
                  title="Grid view"
                >
                  <Grid className="w-4 h-4" />
                </Button>
                <Button
                  variant={viewMode === 'list' ? 'default' : 'ghost'}
                  size="sm"
                  onClick={() => setViewMode('list')}
                  title="Compact list view"
                >
                  <List className="w-4 h-4" />
                </Button>
              </div>

              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    onClick={handleClearCodecCache}
                    disabled={clearingCache}
                    className="gap-2"
                  >
                    {clearingCache ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <Trash2 className="w-4 h-4" />
                    )}
                    <span className="hidden sm:inline">Clear Probe Cache</span>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Clear all cached codec probe results. Streams will be re-probed on next access.</p>
                </TooltipContent>
              </Tooltip>
            </div>
          </CardContent>
        </Card>

        {/* Error Display */}
        {error && (
          <Card className="mb-6 border-destructive">
            <CardContent className="p-4">
              <p className="text-destructive">{error}</p>
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  fetchChannels({
                    searchTerm: debouncedSearch,
                    group: selectedGroup,
                    pageNum: 1,
                    append: false,
                    isSearchChange: false,
                    sourceIds: selectedSources,
                  })
                }
                className="mt-2"
              >
                Retry
              </Button>
            </CardContent>
          </Card>
        )}

        {/* Results Summary */}
        {channels.length > 0 && (
          <div className="mb-4 text-sm text-muted-foreground">
            Showing {channels.length} of {total} channels
            {hasMore && !loading && (
              <span className="ml-2 text-primary">
                • {Math.ceil((total - channels.length) / 200)} more pages available
              </span>
            )}
          </div>
        )}

        {/* Channels Display */}
        {channels.length > 0 ? (
          <>
            {viewMode === 'table' ? (
              <Card className="mb-6">
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-16">Logo</TableHead>
                        <TableHead>Channel Name</TableHead>
                        <TableHead>Group</TableHead>
                        <TableHead>Probe Info</TableHead>
                        <TableHead>Last Probed</TableHead>
                        <TableHead className="w-32">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {channels.map((channel) => (
                        <ChannelTableRow key={channel.id} channel={channel} />
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
            ) : viewMode === 'grid' ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 mb-6">
                {channels.map((channel) => (
                  <ChannelCard key={channel.id} channel={channel} />
                ))}
              </div>
            ) : (
              <div className="space-y-3 mb-6">
                {channels.map((channel) => (
                  <ChannelListItem key={channel.id} channel={channel} />
                ))}
              </div>
            )}

            {/* Progressive Loading */}
            {hasMore && (
              <div ref={loadMoreRef} className="flex justify-center mt-6">
                <Card className="w-full max-w-md">
                  <CardContent className="p-4 text-center">
                    {loading ? (
                      <div className="flex items-center justify-center space-x-2">
                        <div className="animate-spin rounded-full h-4 w-4 border-2 border-primary border-t-transparent"></div>
                        <p className="text-sm text-muted-foreground">Loading more channels...</p>
                      </div>
                    ) : (
                      <>
                        <p className="text-sm text-muted-foreground mb-2">
                          {Math.ceil((total - channels.length) / 200)} pages remaining
                        </p>
                        <Button
                          variant="outline"
                          onClick={handleLoadMore}
                          size="sm"
                          className="gap-2"
                        >
                          Load More Channels
                        </Button>
                      </>
                    )}
                  </CardContent>
                </Card>
              </div>
            )}
          </>
        ) : (
          !loading && (
            <Card>
              <CardContent className="p-8 text-center">
                <p className="text-muted-foreground">No channels found</p>
                {(search || selectedGroup || selectedSources.length > 0) && (
                  <Button
                    variant="outline"
                    onClick={() => {
                      setSearch('');
                      setSelectedGroup('');
                      setSelectedSources([]);
                    }}
                    className="mt-4"
                  >
                    Clear Filters
                  </Button>
                )}
              </CardContent>
            </Card>
          )
        )}

        {/* Video Player Modal */}
        {selectedChannel && (
          <VideoPlayerModal
            isOpen={isPlayerOpen}
            onClose={() => {
              setIsPlayerOpen(false);
              setSelectedChannel(null);
            }}
            channel={selectedChannel}
          />
        )}
        <ChannelDetailsSheet
          channel={detailsChannel}
          open={isDetailsOpen}
          onOpenChange={(open) => {
            setIsDetailsOpen(open);
            if (!open) setDetailsChannel(null);
          }}
        />
      </div>
    </TooltipProvider>
  );
}
