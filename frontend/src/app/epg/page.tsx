'use client';

import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import {
  Search,
  Calendar,
  Clock,
  Filter,
  Grid,
  List,
  Table as TableIcon,
  Globe,
  Play,
} from 'lucide-react';
import { DateTimePicker } from '@/components/ui/date-time-picker';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ScrollArea, ScrollBar } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
} from '@/components/ui/command';
import { Checkbox } from '@/components/ui/checkbox';
import { Check, ChevronsUpDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { VideoPlayerModal } from '@/components/video-player-modal';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';
import { CanvasEPG } from '@/components/epg/CanvasEPG';

interface Channel {
  id: string;
  name: string;
  logo_url?: string;
  group?: string;
  stream_url: string;
  source_type: string;
  source_name?: string;
}

interface EpgProgram {
  id: string;
  channel_id: string;
  channel_name: string;
  channel_logo?: string;
  title: string;
  description?: string;
  start_time: string;
  end_time: string;
  category?: string;
  rating?: string;
  source_id?: string;
  metadata?: Record<string, string>;
  is_streamable: boolean;
}

interface EpgSource {
  id: string;
  name: string;
  url?: string;
  last_updated?: string;
  channel_count: number;
  program_count: number;
}

interface SourceOption {
  id: string;
  name: string;
  type: 'epg_source' | 'stream_source';
  display_name: string;
}

interface EpgProgramsResponse {
  programs: EpgProgram[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

interface EpgGuideResponse {
  channels: Record<string, { id: string; name: string; logo?: string }>;
  programs: Record<string, EpgProgram[]>;
  time_slots: string[];
  start_time: string;
  end_time: string;
}

export default function EpgPage() {
  const [programs, setPrograms] = useState<EpgProgram[]>([]);
  const [sources, setSources] = useState<EpgSource[]>([]);
  const [sourceOptions, setSourceOptions] = useState<SourceOption[]>([]);
  const [guideData, setGuideData] = useState<EpgGuideResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  // Multi-select source filter (replaces legacy single selectedSource)
  const [selectedSources, setSelectedSources] = useState<string[]>([]);
  // Toggle a single source selection
  const handleSourceToggle = useCallback((id: string) => {
    setSelectedSources((prev) =>
      prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id]
    );
  }, []);
  // Toggle all / clear all sources
  const handleAllSourcesToggle = useCallback(() => {
    setSelectedSources((prev) =>
      prev.length === sourceOptions.length ? [] : sourceOptions.map((s) => s.id)
    );
  }, [sourceOptions]);
  const [selectedCategory, setSelectedCategory] = useState<string>('');
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table' | 'guide'>('guide');
  const [currentPage, setCurrentPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [categories, setCategories] = useState<string[]>([]);
  const [timeRange, setTimeRange] = useState<'today' | 'tomorrow' | 'week' | 'custom'>('today');
  const [customDate, setCustomDate] = useState<Date | undefined>(undefined);
  const [hidePastPrograms, setHidePastPrograms] = useState(true);
  const [channelFilter, setChannelFilter] = useState('');
  const [currentTime, setCurrentTime] = useState(new Date());
  const [guideTimeRange, setGuideTimeRange] = useState<
    '6h' | '12h' | '18h' | '24h' | '30h' | '36h' | '42h' | '48h'
  >('12h');
  const [guideStartTime, setGuideStartTime] = useState<Date | undefined>(undefined);
  const [selectedTimezone, setSelectedTimezone] = useState<string>(
    // Detect user's timezone on component mount
    typeof window !== 'undefined' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'UTC'
  );
  const [timezoneOpen, setTimezoneOpen] = useState(false);
  const [selectedChannel, setSelectedChannel] = useState<Channel | null>(null);
  const [isPlayerOpen, setIsPlayerOpen] = useState(false);
  const [currentTimeWindow, setCurrentTimeWindow] = useState(0); // Moved from below
  const loadMoreRef = useRef<HTMLDivElement>(null);

  // Common timezone list with search-friendly labels
  const timezones = [
    { value: 'UTC', label: 'UTC (Coordinated Universal Time)', offset: '+00:00' },
    { value: 'America/New_York', label: 'New York (Eastern Time)', offset: '-05:00/-04:00' },
    { value: 'America/Chicago', label: 'Chicago (Central Time)', offset: '-06:00/-05:00' },
    { value: 'America/Denver', label: 'Denver (Mountain Time)', offset: '-07:00/-06:00' },
    { value: 'America/Los_Angeles', label: 'Los Angeles (Pacific Time)', offset: '-08:00/-07:00' },
    { value: 'America/Phoenix', label: 'Phoenix (Arizona Time)', offset: '-07:00' },
    { value: 'America/Anchorage', label: 'Anchorage (Alaska Time)', offset: '-09:00/-08:00' },
    { value: 'Pacific/Honolulu', label: 'Honolulu (Hawaii Time)', offset: '-10:00' },
    { value: 'Europe/London', label: 'London (GMT/BST)', offset: '+00:00/+01:00' },
    { value: 'Europe/Paris', label: 'Paris (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Berlin', label: 'Berlin (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Rome', label: 'Rome (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Madrid', label: 'Madrid (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Amsterdam', label: 'Amsterdam (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Zurich', label: 'Zurich (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Vienna', label: 'Vienna (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Prague', label: 'Prague (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Warsaw', label: 'Warsaw (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Stockholm', label: 'Stockholm (CET/CEST)', offset: '+01:00/+02:00' },
    { value: 'Europe/Helsinki', label: 'Helsinki (EET/EEST)', offset: '+02:00/+03:00' },
    { value: 'Europe/Athens', label: 'Athens (EET/EEST)', offset: '+02:00/+03:00' },
    { value: 'Europe/Istanbul', label: 'Istanbul (Turkey Time)', offset: '+03:00' },
    { value: 'Europe/Moscow', label: 'Moscow (Moscow Time)', offset: '+03:00' },
    { value: 'Asia/Dubai', label: 'Dubai (Gulf Time)', offset: '+04:00' },
    { value: 'Asia/Karachi', label: 'Karachi (Pakistan Time)', offset: '+05:00' },
    { value: 'Asia/Kolkata', label: 'Mumbai/Delhi (India Time)', offset: '+05:30' },
    { value: 'Asia/Dhaka', label: 'Dhaka (Bangladesh Time)', offset: '+06:00' },
    { value: 'Asia/Bangkok', label: 'Bangkok (Indochina Time)', offset: '+07:00' },
    { value: 'Asia/Singapore', label: 'Singapore (Singapore Time)', offset: '+08:00' },
    { value: 'Asia/Hong_Kong', label: 'Hong Kong (Hong Kong Time)', offset: '+08:00' },
    { value: 'Asia/Shanghai', label: 'Shanghai (China Time)', offset: '+08:00' },
    { value: 'Asia/Taipei', label: 'Taipei (Taiwan Time)', offset: '+08:00' },
    { value: 'Asia/Tokyo', label: 'Tokyo (Japan Time)', offset: '+09:00' },
    { value: 'Asia/Seoul', label: 'Seoul (Korea Time)', offset: '+09:00' },
    { value: 'Australia/Adelaide', label: 'Adelaide (Central Australia)', offset: '+09:30/+10:30' },
    { value: 'Australia/Sydney', label: 'Sydney (Eastern Australia)', offset: '+10:00/+11:00' },
    { value: 'Australia/Brisbane', label: 'Brisbane (Eastern Australia)', offset: '+10:00' },
    { value: 'Australia/Perth', label: 'Perth (Western Australia)', offset: '+08:00' },
    { value: 'Pacific/Auckland', label: 'Auckland (New Zealand)', offset: '+12:00/+13:00' },
  ];

  // Helper function to format time in selected timezone
  const formatTimeInTimezone = useCallback(
    (timeString: string) => {
      try {
        const date = new Date(timeString);
        if (isNaN(date.getTime())) {
          return '--:--';
        }

        return date.toLocaleTimeString([], {
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
          timeZone: selectedTimezone,
        });
      } catch (error) {
        Debug.warn('Error formatting time in timezone:', error);
        return '--:--';
      }
    },
    [selectedTimezone]
  );

  // Helper function to format date in selected timezone
  const formatDateInTimezone = useCallback(
    (timeString: string) => {
      try {
        const date = new Date(timeString);
        if (isNaN(date.getTime())) {
          return 'Invalid Date';
        }

        return date.toLocaleDateString([], {
          weekday: 'short',
          month: 'short',
          day: 'numeric',
          timeZone: selectedTimezone,
        });
      } catch (error) {
        Debug.warn('Error formatting date in timezone:', error);
        return 'Invalid Date';
      }
    },
    [selectedTimezone]
  );

  // Helper function to format guide time in selected timezone
  const formatGuideTimeInTimezone = useCallback(
    (timeString: string) => {
      try {
        const date = new Date(timeString);
        if (isNaN(date.getTime())) {
          return '--';
        }

        return date.toLocaleTimeString([], {
          hour: 'numeric',
          hour12: true,
          timeZone: selectedTimezone,
        });
      } catch (error) {
        Debug.warn('Error formatting guide time in timezone:', error);
        return '--';
      }
    },
    [selectedTimezone]
  );

  const getTimeRangeParams = useCallback(() => {
    const now = new Date();
    let startTime: Date;
    let endTime: Date;

    switch (timeRange) {
      case 'today':
        // If hiding past programs, start from current time, otherwise start from beginning of day
        startTime = hidePastPrograms
          ? now
          : new Date(now.getFullYear(), now.getMonth(), now.getDate());
        endTime = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
        break;
      case 'tomorrow':
        startTime = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
        endTime = new Date(startTime.getTime() + 24 * 60 * 60 * 1000);
        break;
      case 'week':
        // If hiding past programs, start from current time, otherwise start from beginning of today
        startTime = hidePastPrograms
          ? now
          : new Date(now.getFullYear(), now.getMonth(), now.getDate());
        endTime = new Date(startTime.getTime() + 7 * 24 * 60 * 60 * 1000);
        break;
      case 'custom':
        if (customDate) {
          // For custom date, check if it's today and apply hidePastPrograms logic
          const isToday = customDate.toDateString() === now.toDateString();
          startTime =
            isToday && hidePastPrograms
              ? now
              : new Date(customDate.getFullYear(), customDate.getMonth(), customDate.getDate());
          endTime = new Date(
            customDate.getFullYear(),
            customDate.getMonth(),
            customDate.getDate() + 1
          );
        } else {
          // Fallback to today if no custom date is set
          startTime = hidePastPrograms
            ? now
            : new Date(now.getFullYear(), now.getMonth(), now.getDate());
          endTime = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
        }
        break;
    }

    return {
      start_time: startTime.toISOString(),
      end_time: endTime.toISOString(),
    };
  }, [timeRange, hidePastPrograms, customDate]);

  // Debounce search input to prevent excessive API calls and focus loss
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search);
    }, 300); // 300ms debounce

    return () => clearTimeout(timer);
  }, [search]);

  // Update current time every minute for live indicators
  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentTime(new Date());
    }, 60000); // Update every minute

    return () => clearInterval(timer);
  }, []);

  const fetchPrograms = useCallback(
    async (
      searchTerm: string = '',
      sourceId: string = '',
      category: string = '',
      pageNum: number = 1,
      append: boolean = false
    ) => {
      try {
        setLoading(true);

        const params = new URLSearchParams({
          page: pageNum.toString(),
          limit: '50',
          ...getTimeRangeParams(),
        });

        if (searchTerm) params.append('search', searchTerm);
        if (sourceId) params.append('source_id', sourceId);
        if (category) params.append('category', category);

        const response = await fetch(`/api/v1/epg/programs?${params}`);

        if (!response.ok) {
          throw new Error(`Failed to fetch programs: ${response.statusText}`);
        }

        const data: { success: boolean; data: EpgProgramsResponse } = await response.json();

        if (!data.success) {
          throw new Error('API returned unsuccessful response');
        }

        if (append) {
          setPrograms((prev) => {
            // Deduplicate by ID
            const existing = new Set(prev.map((program) => program.id));
            const newPrograms = data.data.programs.filter((program) => !existing.has(program.id));
            return [...prev, ...newPrograms];
          });
        } else {
          setPrograms(data.data.programs);
        }

        setCurrentPage(pageNum);
        setTotal(data.data.total);
        setHasMore(data.data.has_more);

        // Extract unique categories for filtering - only update on fresh fetch
        if (!append) {
          const uniqueCategories = Array.from(
            new Set(data.data.programs.map((p) => p.category).filter(Boolean))
          ) as string[];
          setCategories(uniqueCategories);
        }

        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'An error occurred');
        if (!append) {
          setPrograms([]);
        }
      } finally {
        setLoading(false);
      }
    },
    [getTimeRangeParams]
  );

  const fetchSources = async () => {
    try {
      const options: SourceOption[] = [];

      // Fetch ONLY EPG Sources
      try {
        const epgSourcesResponse = await fetch('/api/v1/sources/epg');
        if (epgSourcesResponse.ok) {
          const epgSourcesData: { success: boolean; data: { items: any[] } } =
            await epgSourcesResponse.json();
          if (epgSourcesData.success && epgSourcesData.data.items) {
            const activeEpgSources = epgSourcesData.data.items.filter((source) => source.is_active);
            setSources(activeEpgSources);
            activeEpgSources.forEach((source) => {
              options.push({
                id: source.id,
                name: source.name,
                type: 'epg_source',
                display_name: `${source.name} (${(source.source_type || 'epg').toUpperCase()})`,
              });
            });
          }
        }
      } catch (err) {
        Debug.warn('Failed to fetch EPG sources:', err);
      }

      // Deduplicate by normalized name + type to avoid duplicates
      const uniqueMap = new Map<string, SourceOption>();
      for (const opt of options) {
        const key = `${opt.name.trim().toLowerCase()}::${opt.type}`;
        if (!uniqueMap.has(key)) {
          uniqueMap.set(key, opt);
        }
      }
      setSourceOptions(Array.from(uniqueMap.values()));
    } catch (err) {
      console.error('Failed to fetch sources:', err);
    }
  };

  const fetchGuideData = async () => {
    try {
      setLoading(true);

      // Use guide-specific time range - start at beginning of current hour if no custom time is set
      const baseTime =
        guideStartTime ||
        (() => {
          const now = new Date();
          // Round down to the start of the current hour
          return new Date(
            now.getFullYear(),
            now.getMonth(),
            now.getDate(),
            now.getHours(),
            0,
            0,
            0
          );
        })();
      const hours = parseInt(guideTimeRange.replace('h', ''));
      const startTime = baseTime;
      const endTime = new Date(baseTime.getTime() + hours * 60 * 60 * 1000);

      const params = new URLSearchParams({
        start_time: startTime.toISOString(),
        end_time: endTime.toISOString(),
      });

      if (selectedSources.length > 0) params.append('source_id', selectedSources.join(','));

      const response = await fetch(`/api/v1/epg/guide?${params}`);

      if (!response.ok) {
        throw new Error(`Failed to fetch guide data: ${response.statusText}`);
      }

      const data: { success: boolean; data: EpgGuideResponse } = await response.json();

      if (!data.success) {
        throw new Error('API returned unsuccessful response');
      }

      setGuideData(data.data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred');
      setGuideData(null);
    } finally {
      setLoading(false);
    }
  };

  const handleLoadMore = useCallback(() => {
    if (hasMore && !loading && viewMode !== 'guide') {
      fetchPrograms(
        debouncedSearch,
        selectedSources.join(','),
        selectedCategory,
        currentPage + 1,
        true
      );
    }
  }, [
    hasMore,
    loading,
    viewMode,
    debouncedSearch,
    selectedSources,
    selectedCategory,
    currentPage,
    fetchPrograms,
  ]);

  useEffect(() => {
    fetchSources();
  }, []);

  // Handle search changes without losing focus
  const performProgramSearch = useCallback(() => {
    if (viewMode !== 'guide') {
      setPrograms([]);
      setCurrentPage(1);
      fetchPrograms(debouncedSearch, selectedSources.join(','), selectedCategory, 1, false);
    }
  }, [viewMode, debouncedSearch, selectedSources, selectedCategory, fetchPrograms]);

  useEffect(() => {
    performProgramSearch();
  }, [debouncedSearch]); // Only trigger on debounced search changes

  // Handle non-search filter changes
  useEffect(() => {
    if (viewMode === 'guide') {
      fetchGuideData();
    } else {
      setPrograms([]);
      setCurrentPage(1);
      fetchPrograms(debouncedSearch, selectedSources.join(','), selectedCategory, 1, false);
    }
  }, [
    selectedSources,
    selectedCategory,
    timeRange,
    viewMode,
    hidePastPrograms,
    customDate,
    guideTimeRange,
    guideStartTime,
  ]);

  // Intersection observer for infinite scroll - only for non-guide views
  useEffect(() => {
    const loadMoreElement = loadMoreRef.current;
    if (!loadMoreElement || viewMode === 'guide') return;

    const observer = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        // Trigger load more when the element comes into view and we have more data
        // Only trigger on intersection, not when search changes to prevent focus loss
        if (entry.isIntersecting && hasMore && !loading && !debouncedSearch) {
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
  }, [hasMore, loading, debouncedSearch, viewMode, handleLoadMore]);

  const handleSearch = (value: string) => {
    setSearch(value);
  };

  // Legacy single-source handler removed in favor of multi-select (handleSourceToggle / handleAllSourcesToggle)

  const handleCategoryFilter = (value: string) => {
    setSelectedCategory(value === 'all' ? '' : value);
  };

  // Handle channel play (only for channel rows, not individual programs)
  const handlePlayChannel = (channel: { id: string; name: string; logo?: string }) => {
    const channelData: Channel = {
      id: channel.id,
      name: channel.name,
      logo_url: channel.logo,
      stream_url: `${getBackendUrl()}/channel/${encodeURIComponent(channel.id)}/stream`,
      source_type: 'channel',
      group: '',
      source_name: 'EPG',
    };
    setSelectedChannel(channelData);
    setIsPlayerOpen(true);
  };

  // Removed handlePlayProgram - play functionality not needed in EPG viewer

  // Use timezone-aware formatters (replaced the old formatTime, formatDate, formatGuideTime)

  const isCurrentTimeSlot = (timeSlot: string) => {
    const slotTime = new Date(timeSlot);
    const now = currentTime;
    const slotEnd = new Date(slotTime.getTime() + 60 * 60 * 1000); // Add 1 hour
    return now >= slotTime && now < slotEnd;
  };

  // All channels rendered - no virtualization, just filtering and sorting
  const getAllChannels = useMemo(() => {
    if (!guideData) return [];

    let channels = Object.entries(guideData.channels);

    // Filter by search term - search channels and their programs
    if (channelFilter) {
      const searchLower = channelFilter.toLowerCase();
      channels = channels.filter(([id, channel]) => {
        // Check if channel name or ID matches
        if (
          channel.name.toLowerCase().includes(searchLower) ||
          id.toLowerCase().includes(searchLower)
        ) {
          return true;
        }

        // Check if any program title matches
        const channelPrograms = guideData.programs[id] || [];
        return channelPrograms.some(
          (program) =>
            program.title.toLowerCase().includes(searchLower) ||
            (program.description && program.description.toLowerCase().includes(searchLower))
        );
      });
    }

    // Sort alphabetically by channel name
    return channels.sort(([, a], [, b]) => a.name.localeCompare(b.name));
  }, [guideData, channelFilter]);

  // Filter programs based on hide past programs toggle and live only toggle
  const filteredPrograms = useMemo(() => {
    let filtered = programs;

    // Filter by past programs
    if (hidePastPrograms) {
      const now = new Date();
      filtered = filtered.filter((program) => {
        const endTime = new Date(program.end_time);
        // Keep programs that haven't ended yet (live and upcoming)
        return endTime > now;
      });
    }

    return filtered;
  }, [programs, hidePastPrograms]);

  const ProgramCard = ({ program }: { program: EpgProgram }) => {
    const now = new Date();
    const startTime = new Date(program.start_time);
    const endTime = new Date(program.end_time);
    const isLive = now >= startTime && now <= endTime;
    const isUpcoming = startTime > now;

    return (
      <Card className="transition-all duration-200 hover:shadow-lg">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between">
            <div className="flex-1 min-w-0">
              <div className="flex items-center space-x-2 mb-1">
                <Badge
                  variant={isLive ? 'default' : isUpcoming ? 'secondary' : 'outline'}
                  className="text-xs"
                >
                  {isLive ? 'LIVE' : isUpcoming ? 'UPCOMING' : 'PAST'}
                </Badge>
                {program.category && (
                  <Badge variant="outline" className="text-xs">
                    {program.category}
                  </Badge>
                )}
              </div>
              <CardTitle className="text-sm font-medium line-clamp-2">{program.title}</CardTitle>
              <CardDescription className="text-xs">
                {program.channel_name} • {formatDateInTimezone(program.start_time)}
              </CardDescription>
            </div>
            {program.channel_logo && (
              <img
                src={program.channel_logo}
                alt={program.channel_name}
                className="w-8 h-8 object-contain ml-2 flex-shrink-0"
                onError={(e) => {
                  (e.target as HTMLImageElement).style.display = 'none';
                }}
              />
            )}
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center text-xs text-muted-foreground">
              <Clock className="w-3 h-3 mr-1" />
              {formatTimeInTimezone(program.start_time)} - {formatTimeInTimezone(program.end_time)}
            </div>
            {program.rating && (
              <Badge variant="outline" className="text-xs">
                {program.rating}
              </Badge>
            )}
          </div>

          {program.description && (
            <p className="text-xs text-muted-foreground line-clamp-2 mb-3">{program.description}</p>
          )}

          <div className="text-xs text-muted-foreground">Channel: {program.channel_name}</div>
        </CardContent>
      </Card>
    );
  };

  const ProgramTableRow = ({ program }: { program: EpgProgram }) => {
    const now = new Date();
    const startTime = new Date(program.start_time);
    const endTime = new Date(program.end_time);
    const isLive = now >= startTime && now <= endTime;
    const isUpcoming = startTime > now;

    return (
      <TableRow className={`hover:bg-muted/50 ${isLive ? 'bg-primary/5' : ''}`}>
        <TableCell className="w-16">
          {program.channel_logo ? (
            <img
              src={program.channel_logo}
              alt={program.channel_name}
              className="w-8 h-8 object-contain"
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = 'none';
              }}
            />
          ) : (
            <div className="w-8 h-8 bg-muted rounded flex items-center justify-center text-muted-foreground text-xs">
              No Logo
            </div>
          )}
        </TableCell>
        <TableCell className="font-medium max-w-xs">
          <div className="truncate" title={program.title}>
            {program.title}
          </div>
        </TableCell>
        <TableCell className="text-sm">{program.channel_name}</TableCell>
        <TableCell className="text-sm">
          <div className="flex items-center">
            <Clock className="w-3 h-3 mr-1" />
            {formatTimeInTimezone(program.start_time)} - {formatTimeInTimezone(program.end_time)}
          </div>
        </TableCell>
        <TableCell className="text-sm">{formatDateInTimezone(program.start_time)}</TableCell>
        <TableCell>
          <Badge
            variant={isLive ? 'default' : isUpcoming ? 'secondary' : 'outline'}
            className="text-xs"
          >
            {isLive ? 'LIVE' : isUpcoming ? 'UPCOMING' : 'PAST'}
          </Badge>
        </TableCell>
        <TableCell>
          {program.category ? (
            <Badge variant="outline" className="text-xs">
              {program.category}
            </Badge>
          ) : (
            <span className="text-muted-foreground">-</span>
          )}
        </TableCell>
        <TableCell className="text-sm max-w-md">
          {program.description ? (
            <div className="truncate" title={program.description}>
              {program.description}
            </div>
          ) : (
            <span className="text-muted-foreground">-</span>
          )}
        </TableCell>
      </TableRow>
    );
  };

  const ProgramListItem = ({ program }: { program: EpgProgram }) => {
    const now = new Date();
    const startTime = new Date(program.start_time);
    const endTime = new Date(program.end_time);
    const isLive = now >= startTime && now <= endTime;
    const isUpcoming = startTime > now;

    return (
      <Card className="transition-all duration-200 hover:shadow-md">
        <CardContent className="p-4">
          <div className="flex items-center space-x-4">
            {program.channel_logo && (
              <img
                src={program.channel_logo}
                alt={program.channel_name}
                className="w-10 h-10 object-contain"
                onError={(e) => {
                  (e.target as HTMLImageElement).style.display = 'none';
                }}
              />
            )}
            <div>
              <h3 className="font-medium">{program.title}</h3>
              <div className="flex items-center space-x-2 text-sm text-muted-foreground">
                <span>{program.channel_name}</span>
                <span>•</span>
                <div className="flex items-center">
                  <Clock className="w-3 h-3 mr-1" />
                  {formatTimeInTimezone(program.start_time)} -{' '}
                  {formatTimeInTimezone(program.end_time)}
                </div>
                <span>•</span>
                <span>{formatDateInTimezone(program.start_time)}</span>
                <Badge
                  variant={isLive ? 'default' : isUpcoming ? 'secondary' : 'outline'}
                  className="text-xs"
                >
                  {isLive ? 'LIVE' : isUpcoming ? 'UPCOMING' : 'PAST'}
                </Badge>
                {program.category && (
                  <>
                    <span>•</span>
                    <Badge variant="outline" className="text-xs">
                      {program.category}
                    </Badge>
                  </>
                )}
              </div>
              {program.description && (
                <p className="text-sm text-muted-foreground mt-1 line-clamp-2">
                  {program.description}
                </p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    );
  };

  // Removed full-page skeleton return so that the filter bar remains visible during loading.
  // The loading state will now be represented only within the guide/program sections below.

  return (
    <TooltipProvider>
      <div className="container mx-auto p-6">
        <div className="mb-6">
          <p className="text-muted-foreground">
            Browse electronic program guide and schedule information
          </p>
        </div>

        <div className="space-y-6">
          {/* Unified Filters and Controls */}
          <Card>
            <CardContent className="p-4">
              <div className="flex flex-col lg:flex-row gap-4 items-center">
                {/* Search */}
                <div className="relative flex-1 min-w-0">
                  <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-muted-foreground w-4 h-4" />
                  <Input
                    placeholder={
                      viewMode === 'guide'
                        ? 'Search channels, programs...'
                        : 'Search programs, channels, descriptions...'
                    }
                    value={viewMode === 'guide' ? channelFilter : search}
                    onChange={(e) =>
                      viewMode === 'guide'
                        ? setChannelFilter(e.target.value)
                        : handleSearch(e.target.value)
                    }
                    className="pl-10"
                  />
                </div>

                {/* Date/Time Picker */}
                <DateTimePicker
                  value={viewMode === 'guide' ? guideStartTime : customDate}
                  onChange={viewMode === 'guide' ? setGuideStartTime : setCustomDate}
                  placeholder="Now"
                  className="w-48"
                />

                {/* Time Range for Guide or Date Range for Programs */}
                {viewMode === 'guide' ? (
                  <Select
                    value={guideTimeRange}
                    onValueChange={(v) =>
                      setGuideTimeRange(
                        v as '6h' | '12h' | '18h' | '24h' | '30h' | '36h' | '42h' | '48h'
                      )
                    }
                  >
                    <SelectTrigger className="w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="6h">6 Hours</SelectItem>
                      <SelectItem value="12h">12 Hours</SelectItem>
                      <SelectItem value="18h">18 Hours</SelectItem>
                      <SelectItem value="24h">24 Hours</SelectItem>
                      <SelectItem value="30h">30 Hours</SelectItem>
                      <SelectItem value="36h">36 Hours</SelectItem>
                      <SelectItem value="42h">42 Hours</SelectItem>
                      <SelectItem value="48h">48 Hours</SelectItem>
                    </SelectContent>
                  </Select>
                ) : (
                  <Select
                    value={timeRange}
                    onValueChange={(v) =>
                      setTimeRange(v as 'today' | 'tomorrow' | 'week' | 'custom')
                    }
                  >
                    <SelectTrigger className="w-40">
                      <Calendar className="w-4 h-4 mr-2" />
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="today">Today</SelectItem>
                      <SelectItem value="tomorrow">Tomorrow</SelectItem>
                      <SelectItem value="week">This Week</SelectItem>
                      <SelectItem value="custom">Custom</SelectItem>
                    </SelectContent>
                  </Select>
                )}

                {/* Source Filter (multi-select command style) */}
                <Popover>
                  <PopoverTrigger asChild>
                    <Button variant="outline" className="w-48 justify-between">
                      <div className="flex items-center">
                        <Filter className="w-4 h-4 mr-2" />
                        <span>
                          {selectedSources.length === 0
                            ? 'All Sources'
                            : selectedSources.length === sourceOptions.length
                              ? 'All Sources'
                              : `${selectedSources.length} Source${selectedSources.length > 1 ? 's' : ''}`}
                        </span>
                      </div>
                      <ChevronsUpDown className="w-4 h-4 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="p-0 w-64">
                    <Command>
                      <CommandInput placeholder="Filter sources..." />
                      <CommandList>
                        <CommandGroup>
                          <CommandItem
                            onSelect={() => handleAllSourcesToggle()}
                            className="cursor-pointer"
                          >
                            <Checkbox
                              checked={
                                selectedSources.length === sourceOptions.length &&
                                sourceOptions.length > 0
                              }
                              className="mr-2"
                            />
                            All Sources
                          </CommandItem>
                          <CommandSeparator />
                          {sourceOptions.map((option) => {
                            const checked = selectedSources.includes(option.id);
                            return (
                              <CommandItem
                                key={option.id}
                                onSelect={() => handleSourceToggle(option.id)}
                                className="cursor-pointer"
                              >
                                <Checkbox checked={checked} className="mr-2" />
                                <span className="truncate">{option.display_name}</span>
                              </CommandItem>
                            );
                          })}
                        </CommandGroup>
                      </CommandList>
                    </Command>
                  </PopoverContent>
                </Popover>

                {/* Timezone Selector */}
                <Popover open={timezoneOpen} onOpenChange={setTimezoneOpen}>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      role="combobox"
                      aria-expanded={timezoneOpen}
                      className="w-64 justify-between"
                    >
                      <Globe className="w-4 h-4 mr-2" />
                      {selectedTimezone
                        ? timezones.find((tz) => tz.value === selectedTimezone)?.label
                        : 'Select timezone...'}
                      <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-80 p-0">
                    <Command>
                      <CommandInput placeholder="Search timezone..." />
                      <CommandList>
                        <CommandEmpty>No timezone found.</CommandEmpty>
                        <CommandGroup>
                          {timezones.map((timezone) => (
                            <CommandItem
                              key={timezone.value}
                              value={timezone.value}
                              onSelect={(currentValue) => {
                                setSelectedTimezone(currentValue);
                                setTimezoneOpen(false);
                              }}
                            >
                              <Check
                                className={cn(
                                  'mr-2 h-4 w-4',
                                  selectedTimezone === timezone.value ? 'opacity-100' : 'opacity-0'
                                )}
                              />
                              <div className="flex-1">
                                <div className="font-medium">{timezone.label}</div>
                                <div className="text-sm text-muted-foreground">
                                  {timezone.offset}
                                </div>
                              </div>
                            </CommandItem>
                          ))}
                        </CommandGroup>
                      </CommandList>
                    </Command>
                  </PopoverContent>
                </Popover>

                {/* Category Filter (only for non-guide views) */}
                {viewMode !== 'guide' && (
                  <Select value={selectedCategory || 'all'} onValueChange={handleCategoryFilter}>
                    <SelectTrigger className="w-48">
                      <Filter className="w-4 h-4 mr-2" />
                      <SelectValue placeholder="All Categories" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Categories</SelectItem>
                      {categories.map((category) => (
                        <SelectItem key={category} value={category}>
                          {category}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}

                {/* Layout Buttons */}
                <div className="flex bg-muted rounded-lg p-1">
                  <Button
                    variant={viewMode === 'guide' ? 'default' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('guide')}
                    title="TV Guide"
                  >
                    <Grid className="w-4 h-4" />
                  </Button>
                  <Button
                    variant={viewMode === 'table' ? 'default' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('table')}
                    title="Table view"
                  >
                    <TableIcon className="w-4 h-4" />
                  </Button>
                  <Button
                    variant={viewMode === 'list' ? 'default' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('list')}
                    title="List view"
                  >
                    <List className="w-4 h-4" />
                  </Button>
                  <Button
                    variant={viewMode === 'grid' ? 'default' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('grid')}
                    title="Card view"
                  >
                    <Grid className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Programs Views (grid, list, table) */}
          {viewMode !== 'guide' && (
            <div className="space-y-6">
              {/* Hide Past Programs Toggle for non-guide views */}
              <div className="flex items-center space-x-2">
                <Switch
                  id="hide-past-programs"
                  checked={hidePastPrograms}
                  onCheckedChange={setHidePastPrograms}
                />
                <label
                  htmlFor="hide-past-programs"
                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                >
                  Hide past programs (keep live and upcoming only)
                </label>
              </div>

              {/* Error Display */}
              {error && (
                <Card className="border-destructive">
                  <CardContent className="p-4">
                    <p className="text-destructive">{error}</p>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        fetchPrograms(
                          debouncedSearch,
                          selectedSources.join(','),
                          selectedCategory,
                          1,
                          false
                        )
                      }
                      className="mt-2"
                    >
                      Retry
                    </Button>
                  </CardContent>
                </Card>
              )}

              {/* Results Summary */}
              {programs.length > 0 && (
                <div className="text-sm text-muted-foreground">
                  Showing {filteredPrograms.length} of {programs.length} programs
                  {hidePastPrograms && filteredPrograms.length !== programs.length && (
                    <span className="ml-2 text-primary">
                      • {programs.length - filteredPrograms.length} past programs hidden
                    </span>
                  )}
                  {hasMore && !loading && (
                    <span className="ml-2 text-primary">
                      • {Math.ceil((total - programs.length) / 50)} more pages available
                    </span>
                  )}
                </div>
              )}

              {/* Programs Display */}
              {loading ? (
                // Loading skeletons for different view modes
                <>
                  {viewMode === 'table' ? (
                    <Card className="mb-6">
                      <CardContent className="p-0">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead className="w-16">Logo</TableHead>
                              <TableHead>Program Title</TableHead>
                              <TableHead>Channel</TableHead>
                              <TableHead>Time</TableHead>
                              <TableHead>Date</TableHead>
                              <TableHead>Status</TableHead>
                              <TableHead>Category</TableHead>
                              <TableHead>Description</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {Array.from({ length: 6 }).map((_, i) => (
                              <TableRow key={i}>
                                <TableCell>
                                  <Skeleton className="w-8 h-8 rounded" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-4 w-32" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-4 w-24" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-4 w-20" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-4 w-16" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-6 w-12 rounded-full" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-6 w-16 rounded-full" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-4 w-40" />
                                </TableCell>
                                <TableCell>
                                  <Skeleton className="h-8 w-8 rounded" />
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </CardContent>
                    </Card>
                  ) : viewMode === 'grid' ? (
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 mb-6">
                      {Array.from({ length: 12 }).map((_, i) => (
                        <Card key={i}>
                          <CardHeader className="pb-3">
                            <div className="flex items-start justify-between">
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center space-x-2 mb-1">
                                  <Skeleton className="h-5 w-12 rounded-full" />
                                  <Skeleton className="h-5 w-16 rounded-full" />
                                </div>
                                <Skeleton className="h-4 w-full mb-1" />
                                <Skeleton className="h-3 w-24" />
                              </div>
                              <Skeleton className="w-8 h-8 ml-2 rounded" />
                            </div>
                          </CardHeader>
                          <CardContent className="pt-0">
                            <div className="flex items-center justify-between mb-3">
                              <Skeleton className="h-3 w-20" />
                              <Skeleton className="h-5 w-8 rounded-full" />
                            </div>
                            <Skeleton className="h-3 w-full mb-1" />
                            <Skeleton className="h-3 w-3/4 mb-3" />
                            <div className="flex justify-between items-center">
                              <Skeleton className="h-3 w-16" />
                              <Skeleton className="h-8 w-8 rounded" />
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  ) : (
                    <div className="space-y-3 mb-6">
                      {Array.from({ length: 8 }).map((_, i) => (
                        <Card key={i}>
                          <CardContent className="p-4">
                            <div className="flex items-center justify-between">
                              <div className="flex items-center space-x-4">
                                <Skeleton className="w-10 h-10 rounded" />
                                <div>
                                  <Skeleton className="h-4 w-48 mb-2" />
                                  <div className="flex items-center space-x-2">
                                    <Skeleton className="h-3 w-20" />
                                    <Skeleton className="h-3 w-16" />
                                    <Skeleton className="h-5 w-12 rounded-full" />
                                    <Skeleton className="h-5 w-16 rounded-full" />
                                  </div>
                                </div>
                              </div>
                              <Skeleton className="h-8 w-16 rounded" />
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  )}
                </>
              ) : filteredPrograms.length > 0 ? (
                <>
                  {viewMode === 'table' ? (
                    <Card className="mb-6">
                      <CardContent className="p-0">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead className="w-16">Logo</TableHead>
                              <TableHead>Program Title</TableHead>
                              <TableHead>Channel</TableHead>
                              <TableHead>Time</TableHead>
                              <TableHead>Date</TableHead>
                              <TableHead>Status</TableHead>
                              <TableHead>Category</TableHead>
                              <TableHead>Description</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {filteredPrograms.map((program) => (
                              <ProgramTableRow key={program.id} program={program} />
                            ))}
                          </TableBody>
                        </Table>
                      </CardContent>
                    </Card>
                  ) : viewMode === 'grid' ? (
                    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 mb-6">
                      {filteredPrograms.map((program) => (
                        <ProgramCard key={program.id} program={program} />
                      ))}
                    </div>
                  ) : (
                    <div className="space-y-3 mb-6">
                      {filteredPrograms.map((program) => (
                        <ProgramListItem key={program.id} program={program} />
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
                              <p className="text-sm text-muted-foreground">
                                Loading more programs...
                              </p>
                            </div>
                          ) : (
                            <>
                              <p className="text-sm text-muted-foreground mb-2">
                                {Math.ceil((total - programs.length) / 50)} pages remaining
                              </p>
                              <Button
                                variant="outline"
                                onClick={handleLoadMore}
                                size="sm"
                                className="gap-2"
                              >
                                Load More Programs
                              </Button>
                            </>
                          )}
                        </CardContent>
                      </Card>
                    </div>
                  )}
                </>
              ) : programs.length > 0 && filteredPrograms.length === 0 ? (
                <Card>
                  <CardContent className="p-8 text-center">
                    <p className="text-muted-foreground">
                      All programs are hidden by current filters
                    </p>
                    <Button
                      variant="outline"
                      onClick={() => setHidePastPrograms(false)}
                      className="mt-4"
                    >
                      Show Past Programs
                    </Button>
                  </CardContent>
                </Card>
              ) : (
                !loading && (
                  <Card>
                    <CardContent className="p-8 text-center">
                      <p className="text-muted-foreground">No programs found</p>
                      {(search || selectedCategory) && (
                        <Button
                          variant="outline"
                          onClick={() => {
                            setSearch('');
                            setSelectedCategory('');
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
            </div>
          )}

          {/* TV Guide View */}
          {viewMode === 'guide' && (
            <div className="space-y-6">
              {/* TV Guide Grid - Canvas Rendering Only */}
              {guideData ? (
                <Card className="overflow-hidden flex flex-col h-[600px]">
                  <div className="relative flex-1 min-h-0">
                    {/* Existing guide stays visible; overlay skeleton only while reloading */}
                    {loading && (
                      <div className="absolute inset-0 z-20 pointer-events-none flex flex-col bg-background/80 backdrop-blur-sm">
                        {/* Header skeleton */}
                        <div className="flex border-b border-border px-3 py-2 bg-background/90">
                          {Array.from({ length: 6 }).map((_, i) => (
                            <div
                              key={i}
                              className="h-4 w-28 mr-4 last:mr-0 rounded bg-muted animate-pulse"
                            />
                          ))}
                        </div>
                        {/* Rows skeleton */}
                        <div className="flex-1 overflow-hidden">
                          {Array.from({ length: 8 }).map((_, r) => (
                            <div
                              key={r}
                              className="flex h-14 border-b border-border last:border-b-0 items-stretch"
                            >
                              {/* Channel cell */}
                              <div className="flex items-center gap-3 px-3 w-48">
                                <div className="w-8 h-8 rounded bg-muted animate-pulse" />
                                <div className="flex-1 space-y-1">
                                  <div className="h-3 w-24 bg-muted rounded animate-pulse" />
                                  <div className="h-2 w-16 bg-muted rounded animate-pulse" />
                                </div>
                              </div>
                              {/* Program placeholders */}
                              <div className="flex-1 flex overflow-hidden px-1">
                                {(() => {
                                  const widths = [96, 128, 160, 192];
                                  let remaining = 960;
                                  const blocks: number[] = [];
                                  while (remaining > 110) {
                                    const w = widths[Math.floor(Math.random() * widths.length)];
                                    if (w > remaining) break;
                                    blocks.push(w);
                                    remaining -= w + 6;
                                  }
                                  return blocks.map((w, i2) => (
                                    <div
                                      key={i2}
                                      style={{ width: w }}
                                      className="h-10 my-auto mr-1 last:mr-0 rounded-md bg-muted relative flex flex-col justify-center px-2 animate-pulse"
                                    >
                                      <div className="h-2 w-3/4 bg-background/40 rounded mb-1" />
                                      <div className="h-2 w-1/2 bg-background/30 rounded" />
                                    </div>
                                  ));
                                })()}
                              </div>
                            </div>
                          ))}
                        </div>
                        {/* Footer skeleton */}
                        <div className="border-t border-border bg-background/90 px-3 py-2 flex justify-between">
                          <div className="h-3 w-40 bg-muted rounded animate-pulse" />
                          <div className="h-3 w-24 bg-muted rounded animate-pulse" />
                        </div>
                      </div>
                    )}

                    <CanvasEPG
                      guideData={guideData}
                      guideTimeRange={guideTimeRange}
                      channelFilter={channelFilter}
                      currentTime={currentTime}
                      selectedTimezone={selectedTimezone}
                      onProgramClick={(program) => {
                        // Handle program clicks - could open modal, navigate, etc.
                        Debug.log('Program clicked:', program);
                      }}
                      onChannelPlay={(channel) => {
                        handlePlayChannel(channel);
                      }}
                      className="border-0"
                    />

                    {/* Guide Footer */}
                    <div className="border-t bg-muted/50 p-3">
                      <div className="flex justify-between items-center text-sm text-muted-foreground">
                        <span>
                          {getAllChannels.length} channels • {guideTimeRange} time range
                        </span>
                        <span>Updated: {new Date().toLocaleTimeString()}</span>
                      </div>
                    </div>
                  </div>
                </Card>
              ) : loading ? (
                <Card className="overflow-hidden">
                  <div className="relative">
                    {/* Skeleton TV Guide Header */}
                    <div className="flex bg-background border-b">
                      <div className="w-48 flex-shrink-0 p-3 border-r bg-muted/50">
                        <Skeleton className="h-5 w-20" />
                      </div>
                      <div className="flex-1">
                        <div className="flex">
                          {Array.from({ length: 6 }).map((_, i) => (
                            <div key={i} className="w-40 p-3 border-r text-center bg-muted/30">
                              <Skeleton className="h-6 w-16 mx-auto" />
                            </div>
                          ))}
                        </div>
                      </div>
                    </div>

                    {/* Skeleton TV Guide Rows */}
                    <div className="h-[600px] overflow-hidden">
                      {Array.from({ length: 8 }).map((_, i) => (
                        <div key={i} className="flex border-b">
                          {/* Skeleton Channel Info */}
                          <div className="w-48 flex-shrink-0 p-3 border-r bg-background">
                            <div className="flex items-center space-x-2">
                              <Skeleton className="w-6 h-6 rounded" />
                              <Skeleton className="w-8 h-8 rounded" />
                              <div className="flex-1">
                                <Skeleton className="h-4 w-24 mb-1" />
                                <Skeleton className="h-3 w-16" />
                              </div>
                            </div>
                          </div>

                          {/* Skeleton Program Cells */}
                          <div className="flex-1">
                            <div className="flex">
                              {(() => {
                                const programWidths = [80, 120, 160, 180, 240];
                                let remainingWidth = 240 * 6;
                                const programs: number[] = [];
                                let totalUsed = 0;

                                while (totalUsed < remainingWidth - 80) {
                                  const availableWidths = programWidths.filter(
                                    (w) => w <= remainingWidth - totalUsed
                                  );
                                  if (!availableWidths.length) break;
                                  const width =
                                    availableWidths[
                                      Math.floor(Math.random() * availableWidths.length)
                                    ];
                                  programs.push(width);
                                  totalUsed += width;
                                }

                                return programs.map((width, j) => (
                                  <div
                                    key={j}
                                    className="border-r h-16 p-1 flex-shrink-0"
                                    style={{ width: `${width}px` }}
                                  >
                                    {Math.random() > 0.2 ? (
                                      <div className="h-full bg-secondary p-1 rounded">
                                        <Skeleton className="h-3 w-3/4 mb-1" />
                                        <Skeleton className="h-3 w-1/2" />
                                      </div>
                                    ) : (
                                      <div className="h-full bg-muted/30 flex items-center justify-center">
                                        <Skeleton className="h-3 w-16" />
                                      </div>
                                    )}
                                  </div>
                                ));
                              })()}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>

                    {/* Skeleton Guide Footer */}
                    <div className="border-t bg-muted/50 p-3">
                      <div className="flex justify-between items-center">
                        <Skeleton className="h-4 w-32" />
                        <Skeleton className="h-4 w-20" />
                      </div>
                    </div>
                  </div>
                </Card>
              ) : (
                <Card>
                  <CardContent className="p-8 text-center">
                    <Grid className="w-12 h-12 mx-auto mb-4 opacity-50" />
                    <p className="text-muted-foreground mb-4">No guide data available</p>
                    <Button onClick={fetchGuideData} variant="outline">
                      Retry Loading Guide
                    </Button>
                  </CardContent>
                </Card>
              )}
            </div>
          )}
        </div>
      </div>

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
    </TooltipProvider>
  );
}
