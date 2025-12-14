'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { Label } from '@/components/ui/label';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import {
  RefreshCw,
  Heart,
  Database,
  Server,
  Clock,
  CheckCircle,
  AlertCircle,
  XCircle,
  Activity,
  Cpu,
  MemoryStick,
  HardDrive,
  Gauge,
  Settings,
  Calendar,
  FolderOpen,
  Zap,
  Copy,
  Check,
  Shield,
  ChevronDown,
  ChevronRight,
  Image,
  Hash,
} from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { ScrollArea } from '@/components/ui/scroll-area';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid } from 'recharts';
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from '@/components/ui/chart';
import { ApiResponse, HealthData, LivezProbeResponse, ReadyzProbeResponse } from '@/types/api';
import { formatUptimeFromSeconds } from '@/lib/format';
import { getStatusIndicatorClasses, getStatusType } from '@/lib/status-colors';
import { getBackendUrl } from '@/lib/config';
import { useHealthData } from '@/hooks/use-health-data';
import { FeatureFlagsDebug } from '@/components/feature-flags-debug';
import { CircuitBreakerCard } from '@/components/circuit-breaker';
import { EnhancedCircuitBreakerStats, EnhancedStatsResponse } from '@/types/circuit-breaker';

interface ChartDataPoint {
  timestamp: string;
  time: string; // Formatted time for display
  cpuLoad: number;
  cpuLoad1minPct: number;
  cpuLoad5minPct: number;
  cpuLoad15minPct: number;
  cpuLoadPercentage: number;
  totalMemoryUsed: number;
  freeMemory: number;
  availableMemory: number;
  swapUsed: number;
  processMemory: number;
  childProcessMemory: number;
}

interface MemoryInfo {
  totalMemoryMb: number;
  usedMemoryMb: number;
  freeMemoryMb: number;
  availableMemoryMb: number;
  swapUsedMb: number;
  swapTotalMb: number;
  processMemoryMb: number;
  childProcessMemoryMb: number;
  totalProcessMemoryMb: number;
  processPercentage: number;
}



function formatMemorySize(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb.toFixed(0)} MB`;
}

function formatPercentage(value: number): string {
  return `${value.toFixed(1)}%`;
}

function formatHumanNumber(num: number): { display: string; full: string } {
  if (num >= 1000000) {
    return {
      display: `${(num / 1000000).toFixed(1)}m`,
      full: num.toLocaleString(),
    };
  } else if (num >= 1000) {
    return {
      display: `${(num / 1000).toFixed(1)}k`,
      full: num.toLocaleString(),
    };
  } else {
    return {
      display: num.toString(),
      full: num.toLocaleString(),
    };
  }
}

function formatTime(date: Date): string {
  return date.toLocaleTimeString('en-US', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function getStatusIcon(status: string | undefined | null) {
  const statusType = getStatusType(status);
  switch (statusType) {
    case 'success':
      return <CheckCircle className="h-4 w-4 text-green-500" />;
    case 'warning':
      return <AlertCircle className="h-4 w-4 text-amber-500" />;
    case 'error':
      return <XCircle className="h-4 w-4 text-destructive" />;
    default:
      return <AlertCircle className="h-4 w-4 text-muted-foreground" />;
  }
}

interface LogoStatsData {
  total_cached_logos: number;
  total_uploaded_logos: number;
  total_storage_used: number;
  total_linked_assets: number;
  cache_hit_rate?: number | null;
  filesystem_cached_logos: number;
  filesystem_cached_storage: number;
}

interface JobStatsData {
  pending_count: number;
  running_count: number;
  completed_count: number;
  failed_count: number;
  by_type: Record<string, number>;
}

interface RunnerStatusData {
  running: boolean;
  worker_count: number;
  worker_id: string;
  pending_jobs: number;
  running_jobs: number;
  poll_interval: string;
}

interface JobData {
  id: string;
  type: string;
  target_name?: string;
  status: string;
  cron_schedule?: string;
  next_run_at?: string;
  started_at?: string;
  completed_at?: string;
}

function LogoCacheCard() {
  const [statsData, setStatsData] = useState<LogoStatsData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchCacheData = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/logos/stats`);

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const data: LogoStatsData = await response.json();
      setStatsData(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchCacheData();

    // Auto-refresh every 30 seconds
    const interval = setInterval(fetchCacheData, 30000);
    return () => clearInterval(interval);
  }, [fetchCacheData]);

  const formatBytes = (bytes: number): string => {
    if (bytes >= 1024 * 1024 * 1024) {
      return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    } else if (bytes >= 1024 * 1024) {
      return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    } else if (bytes >= 1024) {
      return `${(bytes / 1024).toFixed(1)} KB`;
    }
    return `${bytes} B`;
  };

  const formatNumber = (num: number): string => {
    if (num >= 1000000) {
      return `${(num / 1000000).toFixed(1)}M`;
    } else if (num >= 1000) {
      return `${(num / 1000).toFixed(1)}K`;
    } else {
      return num.toString();
    }
  };

  const totalLogos = (statsData?.total_cached_logos ?? 0) + (statsData?.total_uploaded_logos ?? 0);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Image className="h-5 w-5" />
          Logo Cache
          {loading && <RefreshCw className="h-4 w-4 animate-spin" />}
        </CardTitle>
        <CardDescription>Logo storage and caching statistics</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <div className="flex items-center gap-2 text-destructive">
            <XCircle className="h-4 w-4" />
            <span className="text-sm">Error: {error}</span>
          </div>
        )}

        {statsData && (
          <>
            {/* Status and Overview */}
            <div className="flex items-center gap-2">
              <CheckCircle className="h-4 w-4 text-green-500" />
              <Badge className="bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100">
                Active
              </Badge>
              <Badge variant="outline" className="ml-auto">
                {formatNumber(totalLogos)} logos
              </Badge>
            </div>

            {/* Logo Counts */}
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">Logo Counts</h4>
              <div className="grid grid-cols-2 gap-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Cached:</span>
                  <span className="font-medium">
                    {formatNumber(statsData.total_cached_logos)}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Uploaded:</span>
                  <span className="font-medium">
                    {formatNumber(statsData.total_uploaded_logos)}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Linked Assets:</span>
                  <span className="font-medium">
                    {formatNumber(statsData.total_linked_assets)}
                  </span>
                </div>
              </div>
            </div>

            {/* Storage */}
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">Storage</h4>
              <div className="grid grid-cols-1 gap-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Total Used:</span>
                  <span className="font-medium">
                    {formatBytes(statsData.total_storage_used)}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Cached Storage:</span>
                  <span className="font-medium">
                    {formatBytes(statsData.filesystem_cached_storage)}
                  </span>
                </div>
                {totalLogos > 0 && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Avg per Logo:</span>
                    <span className="font-medium">
                      {formatBytes(Math.round(statsData.total_storage_used / totalLogos))}
                    </span>
                  </div>
                )}
              </div>
            </div>

            {/* Status */}
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">Status</h4>
              <div className="space-y-1 text-xs">
                <div className="flex items-center gap-2">
                  <Hash className="h-3 w-3 text-green-500" />
                  <span>Hash-based indexing active</span>
                </div>
                {statsData.cache_hit_rate != null && (
                  <div className="flex items-center gap-2">
                    <Gauge className="h-3 w-3 text-blue-500" />
                    <span>Cache hit rate: {(statsData.cache_hit_rate * 100).toFixed(1)}%</span>
                  </div>
                )}
              </div>
            </div>
          </>
        )}

        {!statsData && !loading && !error && (
          <div className="text-center py-4 text-muted-foreground">
            <Image className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-xs">Logo cache data not available</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function JobsCard() {
  const [statsData, setStatsData] = useState<JobStatsData | null>(null);
  const [runnerData, setRunnerData] = useState<RunnerStatusData | null>(null);
  const [upcomingJobs, setUpcomingJobs] = useState<JobData[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const backendUrl = getBackendUrl();

  const fetchJobsData = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const [statsRes, runnerRes, jobsRes] = await Promise.all([
        fetch(`${backendUrl}/api/v1/jobs/stats`),
        fetch(`${backendUrl}/api/v1/jobs/runner`),
        fetch(`${backendUrl}/api/v1/jobs`),
      ]);

      if (statsRes.ok) {
        const stats: JobStatsData = await statsRes.json();
        setStatsData(stats);
      }

      if (runnerRes.ok) {
        const runner: RunnerStatusData = await runnerRes.json();
        setRunnerData(runner);
      }

      if (jobsRes.ok) {
        const jobsData: { jobs: JobData[] } = await jobsRes.json();
        // Filter jobs with next_run_at and sort by next run time
        const scheduled = (jobsData.jobs || [])
          .filter((j) => j.next_run_at && j.status === 'pending')
          .sort((a, b) => {
            const aTime = a.next_run_at ? new Date(a.next_run_at).getTime() : 0;
            const bTime = b.next_run_at ? new Date(b.next_run_at).getTime() : 0;
            return aTime - bTime;
          })
          .slice(0, 5); // Show top 5 upcoming
        setUpcomingJobs(scheduled);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  }, [backendUrl]);

  useEffect(() => {
    fetchJobsData();
    const interval = setInterval(fetchJobsData, 15000); // Refresh every 15s
    return () => clearInterval(interval);
  }, [fetchJobsData]);

  const formatJobType = (type: string): string => {
    return type
      .split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ');
  };

  const formatRelativeTime = (dateStr: string): string => {
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = date.getTime() - now.getTime();
    const diffSecs = Math.round(diffMs / 1000);
    const diffMins = Math.round(diffSecs / 60);
    const diffHours = Math.round(diffMins / 60);

    if (diffSecs < 0) return 'overdue';
    if (diffSecs < 60) return `in ${diffSecs}s`;
    if (diffMins < 60) return `in ${diffMins}m`;
    if (diffHours < 24) return `in ${diffHours}h`;
    return date.toLocaleDateString();
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Calendar className="h-5 w-5" />
          Jobs
          {loading && <RefreshCw className="h-4 w-4 animate-spin" />}
        </CardTitle>
        <CardDescription>Job scheduler and execution status</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <div className="flex items-center gap-2 text-destructive">
            <XCircle className="h-4 w-4" />
            <span className="text-sm">Error: {error}</span>
          </div>
        )}

        {/* Runner Status */}
        {runnerData && (
          <div className="flex items-center gap-2">
            {runnerData.running ? (
              <CheckCircle className="h-4 w-4 text-green-500" />
            ) : (
              <XCircle className="h-4 w-4 text-red-500" />
            )}
            <Badge
              className={
                runnerData.running
                  ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100'
                  : 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-100'
              }
            >
              {runnerData.running ? 'Running' : 'Stopped'}
            </Badge>
            <Badge variant="outline" className="ml-auto">
              {runnerData.worker_count} worker{runnerData.worker_count !== 1 ? 's' : ''}
            </Badge>
          </div>
        )}

        {/* Job Counts */}
        {statsData && (
          <div className="space-y-2">
            <h4 className="text-sm font-medium text-muted-foreground">Job Status</h4>
            <div className="grid grid-cols-2 gap-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Pending:</span>
                <span className="font-medium">{statsData.pending_count}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Running:</span>
                <span className="font-medium text-blue-600">{statsData.running_count}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Completed:</span>
                <span className="font-medium text-green-600">{statsData.completed_count}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Failed:</span>
                <span className="font-medium text-red-600">{statsData.failed_count}</span>
              </div>
            </div>
          </div>
        )}

        {/* Jobs by Type */}
        {statsData?.by_type && Object.keys(statsData.by_type).length > 0 && (
          <div className="space-y-2">
            <h4 className="text-sm font-medium text-muted-foreground">Jobs by Type</h4>
            <div className="space-y-1 text-sm">
              {Object.entries(statsData.by_type).map(([type, count]) => (
                <div key={type} className="flex justify-between">
                  <span className="text-muted-foreground">{formatJobType(type)}:</span>
                  <span className="font-medium">{count}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Upcoming Jobs */}
        {upcomingJobs.length > 0 && (
          <div className="space-y-2">
            <h4 className="text-sm font-medium text-muted-foreground">Next Scheduled Runs</h4>
            <div className="space-y-2">
              {upcomingJobs.map((job) => (
                <div key={job.id} className="bg-muted/50 rounded p-2">
                  <div className="flex justify-between items-start text-xs">
                    <div>
                      <div className="font-medium">{job.target_name || 'Unknown'}</div>
                      <div className="text-muted-foreground">{formatJobType(job.type)}</div>
                    </div>
                    <div className="text-right">
                      <div className="font-medium">
                        {job.next_run_at ? formatRelativeTime(job.next_run_at) : 'N/A'}
                      </div>
                      {job.cron_schedule && (
                        <div className="text-muted-foreground font-mono text-xs">
                          {job.cron_schedule}
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {!statsData && !runnerData && !loading && !error && (
          <div className="text-center py-4 text-muted-foreground">
            <Calendar className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-xs">Job data not available</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// FFmpeg Info Types
interface FFmpegCodec {
  name: string;
  long_name?: string;
  type: string;
  can_decode: boolean;
  can_encode: boolean;
  is_lossy?: boolean;
  is_lossless?: boolean;
  is_intra_only?: boolean;
}

interface FFmpegHWAccel {
  type: string;
  name: string;
  available: boolean;
  device_name?: string;
  encoders?: string[];
  decoders?: string[];
}

interface FFmpegFormat {
  name: string;
  long_name?: string;
  can_mux: boolean;
  can_demux: boolean;
}

interface FFmpegRecommended {
  hw_accel?: string;
  hw_accel_name?: string;
  video_encoder?: string;
  audio_encoder?: string;
}

interface FFmpegInfo {
  available: boolean;
  ffmpeg_path?: string;
  ffprobe_path?: string;
  version?: string;
  major_version?: number;
  minor_version?: number;
  build_date?: string;
  configuration?: string;
  codecs?: FFmpegCodec[];
  encoders?: string[];
  decoders?: string[];
  hw_accels?: FFmpegHWAccel[];
  formats?: FFmpegFormat[];
  recommended?: FFmpegRecommended;
}

function FFmpegCard() {
  const [ffmpegInfo, setFfmpegInfo] = useState<FFmpegInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState({
    hwaccels: true,
    codecs: false,
    encoders: false,
    decoders: false,
    formats: false,
  });

  useEffect(() => {
    const fetchFFmpegInfo = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/system/ffmpeg`);
        if (response.ok) {
          const data = await response.json();
          setFfmpegInfo(data);
        } else {
          setError('Failed to fetch FFmpeg info');
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      } finally {
        setLoading(false);
      }
    };

    fetchFFmpegInfo();
  }, []);

  const toggleSection = (section: keyof typeof expanded) => {
    setExpanded((prev) => ({ ...prev, [section]: !prev[section] }));
  };

  // Group codecs by type
  const groupedCodecs = ffmpegInfo?.codecs?.reduce(
    (acc, codec) => {
      const type = codec.type || 'other';
      if (!acc[type]) acc[type] = [];
      acc[type].push(codec);
      return acc;
    },
    {} as Record<string, FFmpegCodec[]>
  );

  // Count available HW accels
  const availableHWAccels = ffmpegInfo?.hw_accels?.filter((a) => a.available) || [];

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Settings className="h-5 w-5" />
            FFmpeg Capabilities
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 text-muted-foreground">
            <RefreshCw className="h-4 w-4 animate-spin" />
            Loading FFmpeg information...
          </div>
        </CardContent>
      </Card>
    );
  }

  if (error || !ffmpegInfo?.available) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Settings className="h-5 w-5" />
            FFmpeg Capabilities
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <XCircle className="h-4 w-4 text-red-500" />
            <Badge variant="destructive">Unavailable</Badge>
          </div>
          {error && <p className="text-sm text-muted-foreground mt-2">{error}</p>}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Settings className="h-5 w-5" />
          FFmpeg Capabilities
          <Badge variant="default" className="ml-2">v{ffmpegInfo.version}</Badge>
        </CardTitle>
        <CardDescription>
          Detected FFmpeg installation and hardware acceleration support
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Basic Info */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">FFmpeg Path:</span>
              <span className="font-mono text-xs truncate max-w-[200px]" title={ffmpegInfo.ffmpeg_path}>
                {ffmpegInfo.ffmpeg_path}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">FFprobe Path:</span>
              <span className="font-mono text-xs truncate max-w-[200px]" title={ffmpegInfo.ffprobe_path}>
                {ffmpegInfo.ffprobe_path || 'Not found'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Version:</span>
              <span className="font-medium">{ffmpegInfo.version}</span>
            </div>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Encoders:</span>
              <Badge variant="outline">{ffmpegInfo.encoders?.length || 0}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Decoders:</span>
              <Badge variant="outline">{ffmpegInfo.decoders?.length || 0}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Formats:</span>
              <Badge variant="outline">{ffmpegInfo.formats?.length || 0}</Badge>
            </div>
          </div>
        </div>

        {/* Recommended Config */}
        {ffmpegInfo.recommended && (
          <div className="bg-muted/50 rounded-lg p-3 space-y-2">
            <h4 className="text-sm font-medium flex items-center gap-2">
              <Zap className="h-4 w-4 text-yellow-500" />
              Recommended Configuration
            </h4>
            <div className="grid grid-cols-2 gap-2 text-sm">
              {ffmpegInfo.recommended.hw_accel && (
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">HW Accel:</span>
                  <Badge variant="default">{ffmpegInfo.recommended.hw_accel_name || ffmpegInfo.recommended.hw_accel}</Badge>
                </div>
              )}
              {ffmpegInfo.recommended.video_encoder && (
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">Video:</span>
                  <Badge variant="outline">{ffmpegInfo.recommended.video_encoder}</Badge>
                </div>
              )}
              {ffmpegInfo.recommended.audio_encoder && (
                <div className="flex items-center gap-2">
                  <span className="text-muted-foreground">Audio:</span>
                  <Badge variant="outline">{ffmpegInfo.recommended.audio_encoder}</Badge>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Hardware Acceleration */}
        <div className="space-y-2">
          <div
            className="flex items-center gap-2 cursor-pointer select-none"
            onClick={() => toggleSection('hwaccels')}
          >
            {expanded.hwaccels ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <h4 className="text-sm font-medium">Hardware Acceleration</h4>
            <Badge variant={availableHWAccels.length > 0 ? 'default' : 'secondary'}>
              {availableHWAccels.length} available
            </Badge>
          </div>
          {expanded.hwaccels && (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              {ffmpegInfo.hw_accels?.map((accel) => (
                <div
                  key={accel.name}
                  className={`p-2 rounded border ${accel.available ? 'bg-green-50 dark:bg-green-950 border-green-200 dark:border-green-800' : 'bg-muted/50 border-transparent'}`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      {accel.available ? (
                        <CheckCircle className="h-4 w-4 text-green-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-muted-foreground" />
                      )}
                      <span className="font-medium text-sm">{accel.name.toUpperCase()}</span>
                    </div>
                    {accel.available && accel.device_name && (
                      <span className="text-xs text-muted-foreground truncate max-w-[120px]" title={accel.device_name}>
                        {accel.device_name}
                      </span>
                    )}
                  </div>
                  {accel.available && (accel.encoders?.length || accel.decoders?.length) && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {accel.encoders?.slice(0, 4).map((enc) => (
                        <Badge key={enc} variant="outline" className="text-xs">
                          {enc}
                        </Badge>
                      ))}
                      {(accel.encoders?.length || 0) > 4 && (
                        <Badge variant="secondary" className="text-xs">
                          +{(accel.encoders?.length || 0) - 4} more
                        </Badge>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Codecs Tabs */}
        {groupedCodecs && Object.keys(groupedCodecs).length > 0 && (
          <div className="space-y-2">
            <div
              className="flex items-center gap-2 cursor-pointer select-none"
              onClick={() => toggleSection('codecs')}
            >
              {expanded.codecs ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
              <h4 className="text-sm font-medium">Codecs</h4>
              <Badge variant="outline">{ffmpegInfo.codecs?.length || 0} total</Badge>
            </div>
            {expanded.codecs && (
              <Tabs defaultValue="video" className="w-full">
                <TabsList className="grid w-full grid-cols-4">
                  <TabsTrigger value="video">Video ({groupedCodecs['video']?.length || 0})</TabsTrigger>
                  <TabsTrigger value="audio">Audio ({groupedCodecs['audio']?.length || 0})</TabsTrigger>
                  <TabsTrigger value="subtitle">Subtitle ({groupedCodecs['subtitle']?.length || 0})</TabsTrigger>
                  <TabsTrigger value="data">Data ({groupedCodecs['data']?.length || 0})</TabsTrigger>
                </TabsList>
                {['video', 'audio', 'subtitle', 'data'].map((type) => (
                  <TabsContent key={type} value={type}>
                    <ScrollArea className="h-[200px] rounded border p-2">
                      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-1">
                        {groupedCodecs[type]?.map((codec) => (
                          <div
                            key={codec.name}
                            className="flex items-center gap-1 text-xs p-1 rounded bg-muted/50"
                            title={codec.long_name}
                          >
                            <span className="font-mono">{codec.name}</span>
                            {codec.can_decode && <Badge variant="outline" className="text-[10px] h-4">D</Badge>}
                            {codec.can_encode && <Badge variant="outline" className="text-[10px] h-4">E</Badge>}
                          </div>
                        ))}
                      </div>
                    </ScrollArea>
                  </TabsContent>
                ))}
              </Tabs>
            )}
          </div>
        )}

        {/* Encoders List */}
        <div className="space-y-2">
          <div
            className="flex items-center gap-2 cursor-pointer select-none"
            onClick={() => toggleSection('encoders')}
          >
            {expanded.encoders ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <h4 className="text-sm font-medium">Encoders</h4>
            <Badge variant="outline">{ffmpegInfo.encoders?.length || 0}</Badge>
          </div>
          {expanded.encoders && (
            <ScrollArea className="h-[150px] rounded border p-2">
              <div className="flex flex-wrap gap-1">
                {ffmpegInfo.encoders?.map((enc) => (
                  <Badge key={enc} variant="secondary" className="text-xs font-mono">
                    {enc}
                  </Badge>
                ))}
              </div>
            </ScrollArea>
          )}
        </div>

        {/* Decoders List */}
        <div className="space-y-2">
          <div
            className="flex items-center gap-2 cursor-pointer select-none"
            onClick={() => toggleSection('decoders')}
          >
            {expanded.decoders ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <h4 className="text-sm font-medium">Decoders</h4>
            <Badge variant="outline">{ffmpegInfo.decoders?.length || 0}</Badge>
          </div>
          {expanded.decoders && (
            <ScrollArea className="h-[150px] rounded border p-2">
              <div className="flex flex-wrap gap-1">
                {ffmpegInfo.decoders?.map((dec) => (
                  <Badge key={dec} variant="secondary" className="text-xs font-mono">
                    {dec}
                  </Badge>
                ))}
              </div>
            </ScrollArea>
          )}
        </div>

        {/* Formats List */}
        <div className="space-y-2">
          <div
            className="flex items-center gap-2 cursor-pointer select-none"
            onClick={() => toggleSection('formats')}
          >
            {expanded.formats ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <h4 className="text-sm font-medium">Formats</h4>
            <Badge variant="outline">{ffmpegInfo.formats?.length || 0}</Badge>
          </div>
          {expanded.formats && (
            <ScrollArea className="h-[150px] rounded border p-2">
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-1">
                {ffmpegInfo.formats?.map((fmt) => (
                  <div
                    key={fmt.name}
                    className="flex items-center gap-1 text-xs p-1 rounded bg-muted/50"
                    title={fmt.long_name}
                  >
                    <span className="font-mono">{fmt.name}</span>
                    {fmt.can_demux && <Badge variant="outline" className="text-[10px] h-4">D</Badge>}
                    {fmt.can_mux && <Badge variant="outline" className="text-[10px] h-4">M</Badge>}
                  </div>
                ))}
              </div>
            </ScrollArea>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export function Debug() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [liveProbe, setLiveProbe] = useState<LivezProbeResponse | null>(null);
  const [readyProbe, setReadyProbe] = useState<ReadyzProbeResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [rawJsonExpanded, setRawJsonExpanded] = useState(false);

  // Custom step values: 1, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60
  const stepValues = [1, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60];

  // Chart and refresh interval state
  const [refreshInterval, setRefreshInterval] = useState(stepValues[3]); // Default 15 seconds (index 3)
  const [chartData, setChartData] = useState<ChartDataPoint[]>([]);
  const [isAutoRefresh, setIsAutoRefresh] = useState(true); // Start enabled
  const [enhancedCBStats, setEnhancedCBStats] = useState<EnhancedCircuitBreakerStats[]>([]);

  // Use the health data hook
  const { healthData } = useHealthData(isAutoRefresh ? refreshInterval * 1000 : 0);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Keep max 60 data points (adjust based on needs)
  const MAX_DATA_POINTS = 60;

  const fetchProbesAndUpdateChart = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const backendUrl = getBackendUrl();

      // Add data point to chart if we have health data
      if (
        healthData &&
        healthData.cpu_info &&
        healthData.memory &&
        healthData.memory.process_memory
      ) {
        const now = new Date();
        const dataPoint: ChartDataPoint = {
          timestamp: now.toISOString(),
          time: formatTime(now),
          cpuLoad: (healthData.system_load || 0) * 100, // Keep for backward compatibility
          cpuLoad1minPct:
            ((healthData.cpu_info.load_1min || 0) / (healthData.cpu_info.cores || 1)) * 100,
          cpuLoad5minPct:
            ((healthData.cpu_info.load_5min || 0) / (healthData.cpu_info.cores || 1)) * 100,
          cpuLoad15minPct:
            ((healthData.cpu_info.load_15min || 0) / (healthData.cpu_info.cores || 1)) * 100,
          cpuLoadPercentage: healthData.cpu_info.load_percentage_1min || 0,
          totalMemoryUsed: healthData.memory.used_memory_mb || 0,
          freeMemory: healthData.memory.free_memory_mb || 0,
          availableMemory: healthData.memory.available_memory_mb || 0,
          swapUsed: healthData.memory.swap_used_mb || 0,
          processMemory: healthData.memory.process_memory.main_process_mb || 0,
          childProcessMemory: healthData.memory.process_memory.child_processes_mb || 0,
        };

        setChartData((prev) => {
          const newData = [...prev, dataPoint];
          // Keep only the last MAX_DATA_POINTS
          return newData.slice(-MAX_DATA_POINTS);
        });
      }

      // Fetch Kubernetes probes (/livez and /readyz)
      try {
        const liveResponse = await fetch(`${backendUrl}/livez`);
        if (liveResponse.ok) {
          const liveData: LivezProbeResponse = await liveResponse.json();
          setLiveProbe(liveData);
        } else {
          setLiveProbe({ status: 'error' });
        }
      } catch (err) {
        console.warn('Livez probe endpoint not available');
        setLiveProbe({ status: 'unreachable' });
      }

      try {
        const readyResponse = await fetch(`${backendUrl}/readyz`);
        if (readyResponse.ok) {
          const readyData: ReadyzProbeResponse = await readyResponse.json();
          setReadyProbe(readyData);
        } else {
          setReadyProbe({ status: 'error' });
        }
      } catch (err) {
        console.warn('Readyz probe endpoint not available');
        setReadyProbe({ status: 'unreachable' });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error occurred');
    } finally {
      setLoading(false);
    }
  }, [healthData, MAX_DATA_POINTS]);

  // Auto-refresh management
  const startAutoRefresh = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }

    console.log('Starting auto-refresh with interval:', refreshInterval, 'seconds');
    intervalRef.current = setInterval(() => {
      fetchProbesAndUpdateChart();
    }, refreshInterval * 1000);

    setIsAutoRefresh(true);
  }, [fetchProbesAndUpdateChart, refreshInterval]);

  const stopAutoRefresh = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
    setIsAutoRefresh(false);
  }, []);

  const toggleAutoRefresh = useCallback(() => {
    if (isAutoRefresh) {
      stopAutoRefresh();
    } else {
      startAutoRefresh();
    }
  }, [isAutoRefresh, startAutoRefresh, stopAutoRefresh]);

  const handleRefreshIntervalChange = useCallback(
    (value: number[]) => {
      const sliderIndex = value[0];
      const newInterval = stepValues[sliderIndex];
      console.log('Slider changed:', { sliderIndex, newInterval, stepValues });
      setRefreshInterval(newInterval);
      // The useEffect will handle restarting auto-refresh automatically
    },
    [stepValues]
  );

  // Initial load and cleanup
  useEffect(() => {
    fetchProbesAndUpdateChart(); // Add initial data point
    startAutoRefresh(); // Start auto-refresh immediately

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [fetchProbesAndUpdateChart, startAutoRefresh]);

  // Update auto-refresh when refresh interval changes
  useEffect(() => {
    if (isAutoRefresh) {
      console.log('Restarting auto-refresh due to interval change:', refreshInterval);
      stopAutoRefresh();
      startAutoRefresh();
    }
  }, [refreshInterval, isAutoRefresh, startAutoRefresh, stopAutoRefresh]);

  // Fetch circuit breaker enhanced stats
  useEffect(() => {
    const fetchCircuitBreakerStats = async () => {
      try {
        const backendUrl = getBackendUrl();
        const statsRes = await fetch(`${backendUrl}/api/v1/circuit-breakers/stats`);

        if (statsRes.ok) {
          const statsResult: EnhancedStatsResponse = await statsRes.json();
          if (statsResult.success && statsResult.data) {
            setEnhancedCBStats(statsResult.data);
          }
        }
      } catch (error) {
        console.warn('Failed to fetch circuit breaker stats:', error);
      }
    };

    fetchCircuitBreakerStats();

    // Refresh CB stats periodically (every 10s)
    const interval = setInterval(fetchCircuitBreakerStats, 10000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="space-y-6">
      {/* Header with refresh controls */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <p className="text-muted-foreground">
            Real-time service health monitoring with CPU and memory graphs
          </p>
        </div>

        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-4">
          {/* Refresh Interval Slider */}
          <Card className="p-4 min-w-[280px]">
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label htmlFor="refresh-interval" className="text-sm font-medium">
                  Update Interval: {refreshInterval}s
                </Label>
                <Button
                  onClick={toggleAutoRefresh}
                  variant={isAutoRefresh ? 'default' : 'outline'}
                  size="sm"
                >
                  {isAutoRefresh ? 'Stop' : 'Start'}
                </Button>
              </div>
              <Slider
                id="refresh-interval"
                min={0}
                max={stepValues.length - 1}
                step={1}
                value={[stepValues.indexOf(refreshInterval)]}
                onValueChange={handleRefreshIntervalChange}
                className="w-full"
              />
              <div className="flex justify-between text-xs text-muted-foreground">
                <span>1s</span>
                <span>60s</span>
              </div>
            </div>
          </Card>
        </div>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-destructive">
              <XCircle className="h-4 w-4" />
              <span className="font-medium">Error loading health data:</span>
              <span>{error}</span>
            </div>
          </CardContent>
        </Card>
      )}

      {/* System Overview */}
      {healthData && (
        <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Service Status</CardTitle>
              <Heart className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-2">
                {getStatusIcon(healthData?.status)}
                <div className="text-2xl font-bold">{healthData?.status || 'Unknown'}</div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Version</CardTitle>
              <Server className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">v{healthData.version}</div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Uptime</CardTitle>
              <Clock className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatUptimeFromSeconds(healthData.uptime_seconds)}</div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">CPU Load</CardTitle>
              <Activity className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {formatPercentage(healthData?.cpu_info?.load_percentage_1min || 0)}
              </div>
              <p className="text-xs text-muted-foreground">
                {(healthData?.cpu_info?.load_1min || 0).toFixed(2)} /{' '}
                {healthData?.cpu_info?.cores || 0} cores
              </p>
            </CardContent>
          </Card>

          {/* Relay System Summary */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Relay System</CardTitle>
              <Zap className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-2">
                {getStatusIcon(healthData?.components?.relay_system?.status)}
                <div className="text-2xl font-bold">
                  {healthData?.components?.relay_system?.healthy_processes || 0}
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                {healthData?.components?.relay_system?.healthy_processes || 0}/
                {healthData?.components?.relay_system?.total_processes || 0} healthy
              </p>
            </CardContent>
          </Card>

          {/* Kubernetes Probes */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">K8s Probes</CardTitle>
              <CheckCircle className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="space-y-1">
                <div className="flex items-center gap-1 text-xs">
                  <div
                    className={`h-2 w-2 rounded-full ${
                      liveProbe === null ? 'bg-gray-400' :
                      liveProbe?.status === 'ok' ? 'bg-green-500' : 'bg-red-500'
                    }`}
                  />
                  <span>Live: {liveProbe === null ? 'Checking...' : liveProbe?.status === 'ok' ? 'OK' : 'Fail'}</span>
                </div>
                <div className="flex items-center gap-1 text-xs">
                  <div
                    className={`h-2 w-2 rounded-full ${
                      readyProbe === null ? 'bg-gray-400' :
                      readyProbe?.status === 'ok' ? 'bg-green-500' : 'bg-red-500'
                    }`}
                  />
                  <span>Ready: {readyProbe === null ? 'Checking...' : readyProbe?.status === 'ok' ? 'OK' : 'Fail'}</span>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* tvarr Memory Usage */}
      {healthData && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Zap className="h-5 w-5" />
              tvarr Memory Usage
            </CardTitle>
            <CardDescription>
              Process-specific memory consumption and child process tracking
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-2">
                <div className="text-2xl font-bold text-primary">
                  {formatMemorySize(healthData.memory.process_memory.total_process_tree_mb)}
                </div>
                <p className="text-xs font-medium text-muted-foreground">Total Process Tree</p>
                <p className="text-xs text-muted-foreground">
                  {formatPercentage(healthData.memory.process_memory.percentage_of_system)} of
                  system memory
                </p>
              </div>

              <div className="space-y-2">
                <div className="text-2xl font-bold">
                  {formatMemorySize(healthData.memory.process_memory.main_process_mb)}
                </div>
                <p className="text-xs font-medium text-muted-foreground">Main Process</p>
                <p className="text-xs text-muted-foreground">Primary application</p>
              </div>

              <div className="space-y-2">
                <div className="text-2xl font-bold">
                  {formatMemorySize(healthData.memory.process_memory.child_processes_mb)}
                </div>
                <p className="text-xs font-medium text-muted-foreground">All Child Processes</p>
                <p className="text-xs text-muted-foreground">
                  {healthData.memory.process_memory.child_process_count} processes (includes FFmpeg,
                  cleanup, etc.)
                </p>
              </div>

              <div className="space-y-2">
                <div className="text-2xl font-bold">
                  {healthData.memory.process_memory.child_process_count}
                </div>
                <p className="text-xs font-medium text-muted-foreground">Active Children</p>
                <p className="text-xs text-muted-foreground">Running processes</p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Performance Charts - Side by Side */}
      <div className="grid gap-4 grid-cols-1 lg:grid-cols-2">
        {/* CPU Usage Chart */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Cpu className="h-5 w-5" />
              CPU Load Average
            </CardTitle>
            <CardDescription>
              System load averages over time ({chartData.length} data points){' '}
              {healthData && `• ${healthData?.cpu_info?.cores || 0} cores`}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {chartData.length > 0 ? (
              <ChartContainer
                config={
                  {
                    cpuLoad1minPct: {
                      label: '1min Load',
                      color: 'var(--chart-1)',
                    },
                    cpuLoad5minPct: {
                      label: '5min Load',
                      color: 'var(--chart-2)',
                    },
                    cpuLoad15minPct: {
                      label: '15min Load',
                      color: 'var(--chart-3)',
                    },
                  } satisfies ChartConfig
                }
                className="h-[300px] w-full"
              >
                <AreaChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="time" tickLine={false} axisLine={false} />
                  <YAxis
                    tickLine={false}
                    axisLine={false}
                    domain={[0, 'dataMax']}
                    tickFormatter={(value) => `${value.toFixed(1)}%`}
                  />
                  <ChartTooltip
                    content={
                      <ChartTooltipContent
                        labelFormatter={(label) => `Time: ${label}`}
                        formatter={(value, name) => {
                          const nameMap: Record<string, string> = {
                            cpuLoad1minPct: '1min Load',
                            cpuLoad5minPct: '5min Load',
                            cpuLoad15minPct: '15min Load',
                          };
                          return [`${Number(value).toFixed(1)}% `, nameMap[name] || name];
                        }}
                      />
                    }
                  />
                  <ChartLegend />
                  <Area
                    type="monotone"
                    dataKey="cpuLoad1minPct"
                    stroke="var(--color-cpuLoad1minPct)"
                    fill="var(--color-cpuLoad1minPct)"
                    fillOpacity={0.6}
                    strokeWidth={2}
                  />
                </AreaChart>
              </ChartContainer>
            ) : (
              <div className="h-[300px] flex items-center justify-center text-muted-foreground">
                <div className="text-center">
                  <Gauge className="h-12 w-12 mx-auto mb-2 opacity-50" />
                  <p>Start monitoring to see CPU usage data</p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Memory Usage Chart */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <MemoryStick className="h-5 w-5" />
              Memory Usage
            </CardTitle>
            <CardDescription>
              System memory breakdown over time{' '}
              {healthData && `(Total: ${formatMemorySize(healthData.memory.total_memory_mb)})`}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {chartData.length > 0 && healthData ? (
              <ChartContainer
                config={
                  {
                    totalMemoryUsed: {
                      label: 'Used Memory',
                      color: 'var(--chart-1)',
                    },
                    freeMemory: {
                      label: 'Free Memory',
                      color: 'var(--chart-2)',
                    },
                    swapUsed: {
                      label: 'Swap Used',
                      color: 'var(--chart-3)',
                    },
                  } satisfies ChartConfig
                }
                className="h-[300px] w-full"
              >
                <AreaChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="time" tickLine={false} axisLine={false} />
                  <YAxis
                    tickLine={false}
                    axisLine={false}
                    domain={[0, healthData.memory.total_memory_mb]}
                    tickFormatter={(value) => formatMemorySize(value)}
                  />
                  <ChartTooltip
                    content={
                      <ChartTooltipContent
                        labelFormatter={(label) =>
                          `Time: ${label} • Total: ${formatMemorySize(healthData.memory.total_memory_mb)}`
                        }
                        formatter={(value, name) => {
                          const nameMap: Record<string, string> = {
                            totalMemoryUsed: 'Used Memory',
                            freeMemory: 'Free Memory',
                            availableMemory: 'Available Memory',
                            swapUsed: 'Swap Used',
                          };
                          return [`${formatMemorySize(Number(value))} `, nameMap[name] || name];
                        }}
                      />
                    }
                  />
                  <ChartLegend />
                  <Area
                    type="monotone"
                    dataKey="totalMemoryUsed"
                    stroke="var(--color-totalMemoryUsed)"
                    fill="var(--color-totalMemoryUsed)"
                    fillOpacity={0.6}
                  />
                </AreaChart>
              </ChartContainer>
            ) : (
              <div className="h-[300px] flex items-center justify-center text-muted-foreground">
                <div className="text-center">
                  <HardDrive className="h-12 w-12 mx-auto mb-2 opacity-50" />
                  <p>Start monitoring to see memory usage data</p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* System Components */}
      {healthData && (
        <div className="grid gap-4 md:grid-cols-2">
          {/* Feature Flags Debug - Full width */}
          <div className="md:col-span-2">
            <FeatureFlagsDebug />
          </div>

          {/* Circuit Breakers - Enhanced Visualization */}
          {enhancedCBStats.length > 0 && (
            <Card className="md:col-span-2">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Shield className="h-5 w-5" />
                  Circuit Breakers
                </CardTitle>
                <CardDescription>
                  Active circuit breaker statistics with error categorization and state tracking
                </CardDescription>
              </CardHeader>
              <CardContent>
                <TooltipProvider>
                  <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
                    {enhancedCBStats.map((stats) => (
                      <CircuitBreakerCard
                        key={stats.name}
                        stats={stats}
                        showActions={true}
                        expanded={false}
                      />
                    ))}
                  </div>
                </TooltipProvider>
                {enhancedCBStats.length === 0 && (
                  <div className="text-center py-4 text-muted-foreground">
                    <Shield className="h-8 w-8 mx-auto mb-2 opacity-50" />
                    <p className="text-xs">No active circuit breakers</p>
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {/* Database Component - Enhanced */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Database className="h-5 w-5" />
                Database
              </CardTitle>
              <CardDescription>Connection pool and performance monitoring</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-2">
                {getStatusIcon(healthData?.components?.database?.status)}
                <Badge
                  className={getStatusIndicatorClasses(healthData?.components?.database?.status)}
                >
                  {healthData?.components?.database?.status || 'Unknown'}
                </Badge>
                <Badge variant="outline" className="ml-auto">
                  {healthData.components.database.response_time_status}
                </Badge>
              </div>

              {/* Connection Pool Metrics */}
              <div className="space-y-2">
                <h4 className="text-sm font-medium text-muted-foreground">Connection Pool</h4>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Active:</span>
                    <span className="font-medium">
                      {healthData.components.database.active_connections}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Pool Size:</span>
                    <span className="font-medium">
                      {healthData.components.database.connection_pool_size}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Idle:</span>
                    <span className="font-medium">
                      {healthData.components.database.idle_connections}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Utilization:</span>
                    <span className="font-medium">
                      {healthData.components.database.pool_utilization_percent}%
                    </span>
                  </div>
                </div>
              </div>

              {/* Performance Metrics */}
              <div className="space-y-2">
                <h4 className="text-sm font-medium text-muted-foreground">Performance</h4>
                <div className="grid grid-cols-1 gap-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Response Time:</span>
                    <span className="font-medium">
                      {healthData.components.database.response_time_ms}ms
                    </span>
                  </div>
                </div>
              </div>

              {/* Health Checks */}
              <div className="space-y-2">
                <h4 className="text-sm font-medium text-muted-foreground">Health Checks</h4>
                <div className="grid grid-cols-1 gap-1 text-sm">
                  <div className="flex justify-between items-center">
                    <span className="text-muted-foreground">Tables Accessible:</span>
                    <div className="flex items-center gap-1">
                      {healthData.components.database.tables_accessible ? (
                        <CheckCircle className="h-3 w-3 text-green-500" />
                      ) : (
                        <XCircle className="h-3 w-3 text-red-500" />
                      )}
                      <span className="font-medium text-xs">
                        {healthData.components.database.tables_accessible ? 'Yes' : 'No'}
                      </span>
                    </div>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-muted-foreground">Write Capability:</span>
                    <div className="flex items-center gap-1">
                      {healthData.components.database.write_capability ? (
                        <CheckCircle className="h-3 w-3 text-green-500" />
                      ) : (
                        <XCircle className="h-3 w-3 text-red-500" />
                      )}
                      <span className="font-medium text-xs">
                        {healthData.components.database.write_capability ? 'Yes' : 'No'}
                      </span>
                    </div>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-muted-foreground">No Blocking Locks:</span>
                    <div className="flex items-center gap-1">
                      {healthData.components.database.no_blocking_locks ? (
                        <CheckCircle className="h-3 w-3 text-green-500" />
                      ) : (
                        <XCircle className="h-3 w-3 text-red-500" />
                      )}
                      <span className="font-medium text-xs">
                        {healthData.components.database.no_blocking_locks ? 'Yes' : 'No'}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Jobs Card - replaces old Scheduler component */}
          <JobsCard />

          {/* FFmpeg Capabilities */}
          <FFmpegCard />

          {/* Sandbox Manager Component - Enhanced */}
          {healthData?.components?.sandbox_manager && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <FolderOpen className="h-5 w-5" />
                  Sandbox Manager
                </CardTitle>
                <CardDescription>File management and cleanup operations</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex items-center gap-2">
                  {getStatusIcon(healthData.components.sandbox_manager?.status)}
                  <Badge
                    className={getStatusIndicatorClasses(
                      healthData.components.sandbox_manager?.status
                    )}
                  >
                    {healthData.components.sandbox_manager?.status || 'Unknown'}
                  </Badge>
                  <Badge variant="outline" className="ml-auto capitalize">
                    {healthData.components.sandbox_manager?.cleanup_status ?? 'N/A'}
                  </Badge>
                </div>

                {/* Cleanup Statistics */}
                <div className="space-y-2">
                  <h4 className="text-sm font-medium text-muted-foreground">Latest Cleanup</h4>
                  <div className="grid grid-cols-2 gap-2 text-sm">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Files Cleaned:</span>
                      <span className="font-medium">
                        {healthData.components.sandbox_manager?.temp_files_cleaned ?? 'N/A'}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Space Freed:</span>
                      <span className="font-medium">
                        {healthData.components.sandbox_manager?.disk_space_freed_mb != null
                          ? formatMemorySize(healthData.components.sandbox_manager.disk_space_freed_mb)
                          : 'N/A'}
                      </span>
                    </div>
                  </div>
                  <div className="grid grid-cols-1 gap-2 text-sm">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">Last Cleanup:</span>
                      <span className="font-medium text-xs">
                        {healthData.components.sandbox_manager?.last_cleanup_run
                          ? new Date(healthData.components.sandbox_manager.last_cleanup_run).toLocaleString()
                          : 'N/A'}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Managed Directories */}
                {healthData.components.sandbox_manager?.managed_directories &&
                  healthData.components.sandbox_manager.managed_directories.length > 0 && (
                  <div className="space-y-2">
                    <h4 className="text-sm font-medium text-muted-foreground">Managed Directories</h4>
                    <div className="space-y-2">
                      {healthData.components.sandbox_manager.managed_directories.map((dir, index) => (
                        <div key={index} className="bg-muted/50 rounded p-2">
                          <div className="flex justify-between items-start text-xs">
                            <div>
                              <div className="font-medium font-mono">{dir.name}</div>
                              <div className="text-muted-foreground">
                                Retention: {dir.retention_duration} • Cleanup: {dir.cleanup_interval}
                              </div>
                            </div>
                            <div className="text-right">
                              <Badge variant="outline" className="text-xs">
                                Active
                              </Badge>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {/* Logo Cache Component */}
          <LogoCacheCard />
        </div>
      )}

      {/* Raw JSON Data - Collapsible */}
      {healthData && (
        <Card>
          <CardHeader
            className="cursor-pointer select-none"
            onClick={() => setRawJsonExpanded(!rawJsonExpanded)}
          >
            <CardTitle className="flex items-center gap-2">
              {rawJsonExpanded ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
              Raw Response Data
              <Badge variant="outline" className="text-xs">
                {rawJsonExpanded ? 'Collapse' : 'Expand'}
              </Badge>
            </CardTitle>
            <CardDescription>Complete JSON responses from health endpoints</CardDescription>
          </CardHeader>
          {rawJsonExpanded && (
            <CardContent>
              <div className="relative">
                <h4 className="font-medium mb-2">Health Data (/health)</h4>
                <div className="relative group">
                  <pre className="bg-muted p-3 rounded text-xs overflow-auto">
                    {JSON.stringify(healthData, null, 2)}
                  </pre>
                  <Button
                    variant="outline"
                    size="sm"
                    className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity h-8 w-8 p-0"
                    onClick={async () => {
                      try {
                        await navigator.clipboard.writeText(JSON.stringify(healthData, null, 2));
                        setCopied(true);
                        setTimeout(() => setCopied(false), 2000);
                      } catch (err) {
                        console.error('Failed to copy to clipboard:', err);
                      }
                    }}
                    title="Copy to clipboard"
                  >
                    {copied ? (
                      <Check className="h-3 w-3 text-green-600" />
                    ) : (
                      <Copy className="h-3 w-3" />
                    )}
                  </Button>
                </div>
              </div>
            </CardContent>
          )}
        </Card>
      )}
    </div>
  );
}
