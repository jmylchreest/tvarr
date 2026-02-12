'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Search,
  Play,
  Filter,
  Zap,
  Trash2,
  Loader2,
  ChevronDown,
  ChevronUp,
  PlusCircle,
} from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import type { VideoTrackInfo, AudioTrackInfo, SubtitleTrackInfo, FilterTestChannel } from '@/types/api';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { FilterExpressionEditor } from '@/components/filter-expression-editor';
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
  const [selectedSources, setSelectedSources] = useState<string[]>([]);
  const [currentPage, setCurrentPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [sources, setSources] = useState<StreamSourceOption[]>([]);
  const [selectedChannel, setSelectedChannel] = useState<Channel | null>(null);
  const [isPlayerOpen, setIsPlayerOpen] = useState(false);
  const [probingChannels, setProbingChannels] = useState<Set<string>>(new Set());
  const loadMoreRef = useRef<HTMLDivElement>(null);
  const isSearchChangeRef = useRef(false);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [detailsChannel, setDetailsChannel] = useState<Channel | null>(null);
  const [isDetailsOpen, setIsDetailsOpen] = useState(false);
  const [clearingCache, setClearingCache] = useState(false);
  const [isTyping, setIsTyping] = useState(false);

  // Filter Preview state
  const [filterPreviewOpen, setFilterPreviewOpen] = useState(false);
  const [filterExpression, setFilterExpression] = useState('');
  const [filterResults, setFilterResults] = useState<Channel[] | null>(null);
  const [filterLoading, setFilterLoading] = useState(false);
  const [filterMatchCount, setFilterMatchCount] = useState(0);
  const [filterTotalCount, setFilterTotalCount] = useState(0);
  const [filterHasMore, setFilterHasMore] = useState(false);
  const [filterPage, setFilterPage] = useState(1);
  const [filterError, setFilterError] = useState<string | null>(null);
  const filterDebounceRef = useRef<NodeJS.Timeout | null>(null);
  const filterAbortRef = useRef<AbortController | null>(null);
  const router = useRouter();

  // No longer need proxy resolution - only using direct stream sources
  // Fetch stream sources (id + name) for reliable ID-based filtering
  useEffect(() => {
    (async () => {
      try {
        const resp = await fetch('/api/v1/sources/stream');
        if (!resp.ok) return;
        const json = await resp.json();
        const items = json?.sources ?? json?.data?.items ?? json?.data ?? [];
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

  // Track typing state for focus preservation
  useEffect(() => {
    if (search !== debouncedSearch) {
      setIsTyping(true);
    } else {
      setIsTyping(false);
    }
  }, [search, debouncedSearch]);

  // Maintain focus on search input when loading completes during typing
  useEffect(() => {
    if (
      !loading &&
      isTyping &&
      searchInputRef.current &&
      document.activeElement !== searchInputRef.current
    ) {
      // Restore focus and cursor position after API call completes
      const cursorPosition = searchInputRef.current.selectionStart;
      searchInputRef.current.focus();
      searchInputRef.current.setSelectionRange(cursorPosition || 0, cursorPosition || 0);
    }
  }, [loading, isTyping]);

  /**
   * Explicit channel fetch that does NOT rely on closure-captured filter state.
   * All filter inputs are passed as arguments so regressions are easier to spot.
   */
  const fetchChannels = useCallback(
    async ({
      searchTerm = '',
      pageNum = 1,
      append = false,
      isSearchChange = false,
      sourceIds = [],
    }: {
      searchTerm?: string;
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
        pageNum: 1,
        append: false,
        isSearchChange: false,
        sourceIds: selectedSources,
      });
    }
  }, [debouncedSearch, selectedSources, fetchChannels]);

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

  // Convert FilterTestChannel to Channel format
  const convertFilterChannelToChannel = useCallback((fc: FilterTestChannel): Channel => {
    // Build resolution string if we have dimensions
    const resolution = fc.video_width && fc.video_height
      ? `${fc.video_width}x${fc.video_height}`
      : undefined;

    return {
      id: fc.id,
      name: fc.name,
      logo_url: fc.logo_url || fc.tvg_logo,
      group: fc.group,
      stream_url: fc.stream_url,
      source_type: fc.stream_type || 'unknown',
      source_name: fc.source_name,
      tvg_id: fc.tvg_id,
      tvg_name: fc.tvg_name,
      tvg_chno: fc.tvg_chno,
      video_codec: fc.video_codec,
      audio_codec: fc.audio_codec,
      resolution,
      video_width: fc.video_width,
      video_height: fc.video_height,
      framerate: fc.video_framerate?.toString(),
      audio_channels: fc.audio_channels,
      audio_sample_rate: fc.audio_sample_rate,
      container_format: fc.container_format,
      last_probed_at: fc.last_probed_at,
    };
  }, []);

  // Fetch filter results with abort support
  const fetchFilterResults = useCallback(async (expression: string, page: number = 1, append: boolean = false) => {
    // Cancel any in-flight request
    if (filterAbortRef.current) {
      filterAbortRef.current.abort();
    }

    if (!expression.trim()) {
      setFilterResults(null);
      setFilterMatchCount(0);
      setFilterTotalCount(0);
      setFilterHasMore(false);
      setFilterError(null);
      return;
    }

    // Create new abort controller for this request
    const abortController = new AbortController();
    filterAbortRef.current = abortController;

    try {
      setFilterLoading(true);
      setFilterError(null);

      const response = await apiClient.testFilterWithChannels({
        sourceType: 'stream',
        expression,
        includeChannels: true,
        page,
        limit: 100, // Reduced from 200 for faster initial response
      });

      // Check if request was aborted
      if (abortController.signal.aborted) {
        return;
      }

      if (!response.success) {
        setFilterError(response.error || 'Filter test failed');
        if (!append) {
          setFilterResults(null);
        }
        return;
      }

      const newChannels = (response.channels || []).map(convertFilterChannelToChannel);

      if (append) {
        setFilterResults((prev) => {
          if (!prev) return newChannels;
          const existing = new Set(prev.map((c) => c.id));
          const merged = newChannels.filter((c) => !existing.has(c.id));
          return [...prev, ...merged];
        });
      } else {
        setFilterResults(newChannels);
      }

      setFilterMatchCount(response.matched_count);
      setFilterTotalCount(response.total_channels);
      setFilterHasMore(response.has_more || false);
      setFilterPage(page);
    } catch (err) {
      // Ignore abort errors
      if (err instanceof Error && err.name === 'AbortError') {
        return;
      }
      setFilterError(err instanceof Error ? err.message : 'Failed to test filter');
      if (!append) {
        setFilterResults(null);
      }
    } finally {
      // Only clear loading if this is still the current request
      if (!abortController.signal.aborted) {
        setFilterLoading(false);
      }
    }
  }, [convertFilterChannelToChannel]);

  // Handle filter expression change with debounce
  const handleFilterExpressionChange = useCallback((value: string) => {
    setFilterExpression(value);

    // Clear existing debounce timer
    if (filterDebounceRef.current) {
      clearTimeout(filterDebounceRef.current);
    }

    // Clear results immediately if expression is empty or too short
    if (!value.trim() || value.trim().length < 3) {
      setFilterResults(null);
      setFilterMatchCount(0);
      setFilterTotalCount(0);
      setFilterError(null);
      return;
    }

    // Set new debounce timer (800ms for smoother typing experience)
    filterDebounceRef.current = setTimeout(() => {
      fetchFilterResults(value, 1, false);
    }, 800);
  }, [fetchFilterResults]);

  // Handle load more for filter results
  const handleLoadMoreFilterResults = useCallback(() => {
    if (filterHasMore && !filterLoading) {
      fetchFilterResults(filterExpression, filterPage + 1, true);
    }
  }, [filterHasMore, filterLoading, filterExpression, filterPage, fetchFilterResults]);

  // Toggle filter preview mode - preserves state when collapsing
  const handleFilterPreviewToggle = useCallback((open: boolean) => {
    setFilterPreviewOpen(open);
    if (open) {
      // Clear search when opening filter preview (modes are mutually exclusive)
      setSearch('');
      setDebouncedSearch('');
    }
    // Don't clear filter state when collapsing - preserve for re-expansion
  }, []);

  // Create filter from expression
  const handleCreateFilter = useCallback(() => {
    if (!filterExpression.trim()) return;
    const params = new URLSearchParams({
      create: 'true',
      expression: filterExpression,
      source_type: 'stream',
    });
    router.push(`/admin/filters?${params.toString()}`);
  }, [filterExpression, router]);

  // Cleanup debounce timer and abort controller on unmount
  useEffect(() => {
    return () => {
      if (filterDebounceRef.current) {
        clearTimeout(filterDebounceRef.current);
      }
      if (filterAbortRef.current) {
        filterAbortRef.current.abort();
      }
    };
  }, []);

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
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => {
                setDetailsChannel(channel);
                setIsDetailsOpen(true);
              }}
              className="truncate text-left hover:underline focus:outline-none focus:ring-2 focus:ring-ring rounded-sm"
              title={channel.name || 'empty'}
            >
              {channel.name && channel.name.trim() !== '' ? (
                channel.name
              ) : (
                <span className="text-muted-foreground italic">empty</span>
            )}
            </button>
          </div>
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
                <Search className={`absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 ${filterPreviewOpen ? 'text-muted-foreground/50' : 'text-muted-foreground'}`} />
                <Input
                  ref={searchInputRef}
                  placeholder={filterPreviewOpen ? "Search disabled while Filter Preview is open" : "Search channels..."}
                  value={search}
                  onChange={(e) => handleSearch(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                    }
                  }}
                  className="pl-10"
                  disabled={filterPreviewOpen}
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

        {/* Filter Preview Panel */}
        <Collapsible open={filterPreviewOpen} onOpenChange={handleFilterPreviewToggle} className="mb-6">
          <Card>
            <CollapsibleTrigger asChild>
              <CardHeader className="cursor-pointer hover:bg-muted/50 transition-colors py-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Filter className="w-4 h-4" />
                    <CardTitle className="text-base">Filter Preview</CardTitle>
                    {filterPreviewOpen && filterMatchCount > 0 && (
                      <Badge variant="secondary" className="ml-2">
                        {filterMatchCount}/{filterTotalCount} matched
                      </Badge>
                    )}
                    {filterLoading && (
                      <Loader2 className="w-4 h-4 animate-spin ml-2" />
                    )}
                  </div>
                  {filterPreviewOpen ? (
                    <ChevronUp className="w-4 h-4 text-muted-foreground" />
                  ) : (
                    <ChevronDown className="w-4 h-4 text-muted-foreground" />
                  )}
                </div>
                <CardDescription className="text-xs mt-1">
                  Test filter expressions and see matched channels in real-time
                </CardDescription>
              </CardHeader>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <CardContent className="pt-0 pb-4">
                <div className="space-y-4">
                  {/* Filter Expression Editor */}
                  <FilterExpressionEditor
                    value={filterExpression}
                    onChange={handleFilterExpressionChange}
                    sourceType="stream"
                    placeholder='Try: group_title contains "Sports" OR channel_name contains "News"'
                    showTestResults={false}
                    autoTest={false}
                  />

                  {/* Filter Error Display */}
                  {filterError && (
                    <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
                      {filterError}
                    </div>
                  )}

                  {/* Actions */}
                  <div className="flex items-center justify-between">
                    <div className="text-sm text-muted-foreground">
                      {filterExpression.trim() && !filterLoading && filterResults !== null && (
                        <>
                          Showing {filterResults.length} of {filterMatchCount} matched channels
                          {filterMatchCount > 0 && (
                            <span className="text-muted-foreground ml-1">
                              ({filterTotalCount} total)
                            </span>
                          )}
                        </>
                      )}
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleCreateFilter}
                      disabled={!filterExpression.trim()}
                      className="gap-2"
                    >
                      <PlusCircle className="w-4 h-4" />
                      Create Filter
                    </Button>
                  </div>
                </div>
              </CardContent>
            </CollapsibleContent>
          </Card>
        </Collapsible>

        {/* Error Display */}
        {error && !filterPreviewOpen && (
          <Card className="mb-6 border-destructive">
            <CardContent className="p-4">
              <p className="text-destructive">{error}</p>
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  fetchChannels({
                    searchTerm: debouncedSearch,
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

        {/* Results Summary - Only show when NOT in filter preview mode */}
        {!filterPreviewOpen && channels.length > 0 && (
          <div className="mb-4 text-sm text-muted-foreground">
            Showing {channels.length} of {total} channels
            {debouncedSearch && (
              <span className="ml-2 text-primary">
                (filtered by "{debouncedSearch}")
              </span>
            )}
            {hasMore && !loading && !debouncedSearch && (
              <span className="ml-2 text-primary">
                {' '}
                • {Math.ceil((total - channels.length) / 200)} more pages available
              </span>
            )}
          </div>
        )}

        {/* Channels Display - Use filter results when in filter preview mode */}
        {(() => {
          const displayedChannels = filterPreviewOpen && filterResults !== null ? filterResults : channels;
          const isFilterMode = filterPreviewOpen && filterResults !== null;
          const currentHasMore = isFilterMode ? filterHasMore : hasMore;
          const currentLoading = isFilterMode ? filterLoading : loading;
          const currentTotal = isFilterMode ? filterMatchCount : total;

          return displayedChannels.length > 0 ? (
            <>
              <Card className="mb-6">
                <CardContent className="p-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-16">Logo</TableHead>
                        <TableHead>Channel Name</TableHead>
                        <TableHead>Probe Info</TableHead>
                        <TableHead>Last Probed</TableHead>
                        <TableHead className="w-32">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {displayedChannels.map((channel) => (
                        <ChannelTableRow key={channel.id} channel={channel} />
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>

              {/* Progressive Loading */}
              {currentHasMore && (
                <div ref={isFilterMode ? undefined : loadMoreRef} className="flex justify-center mt-6">
                  <Card className="w-full max-w-md">
                    <CardContent className="p-4 text-center">
                      {currentLoading ? (
                        <div className="flex items-center justify-center space-x-2">
                          <div className="animate-spin rounded-full h-4 w-4 border-2 border-primary border-t-transparent"></div>
                          <p className="text-sm text-muted-foreground">Loading more channels...</p>
                        </div>
                      ) : (
                        <>
                          <p className="text-sm text-muted-foreground mb-2">
                            {Math.ceil((currentTotal - displayedChannels.length) / 200)} pages remaining
                          </p>
                          <Button
                            variant="outline"
                            onClick={isFilterMode ? handleLoadMoreFilterResults : handleLoadMore}
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
            !currentLoading && (
              <Card>
                <CardContent className="p-8 text-center">
                  {isFilterMode ? (
                    <>
                      <p className="text-muted-foreground">
                        {filterExpression.trim() ? 'No channels matched the filter expression' : 'Enter a filter expression to preview matching channels'}
                      </p>
                    </>
                  ) : (
                    <>
                      <p className="text-muted-foreground">No channels found</p>
                      {(search || selectedSources.length > 0) && (
                        <Button
                          variant="outline"
                          onClick={() => {
                            setSearch('');
                            setSelectedSources([]);
                          }}
                          className="mt-4"
                        >
                          Clear Filters
                        </Button>
                      )}
                    </>
                  )}
                </CardContent>
              </Card>
            )
          );
        })()}

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
