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
import { AreaChart, Area, XAxis, YAxis, CartesianGrid } from 'recharts';
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from '@/components/ui/chart';
import { ApiResponse, HealthData, KubernetesProbeResponse } from '@/types/api';
import { getStatusIndicatorClasses, getStatusType } from '@/lib/status-colors';
import { getBackendUrl } from '@/lib/config';
import { useHealthData } from '@/hooks/use-health-data';
import { FeatureFlagsDebug } from '@/components/feature-flags-debug';

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

interface CircuitBreakerProfile {
  implementation_type: string;
  failure_threshold: number;
  operation_timeout: string;
  reset_timeout: string;
  success_threshold: number;
  acceptable_status_codes: string[];
}

interface CircuitBreakerConfig {
  global: CircuitBreakerProfile;
  profiles: Record<string, CircuitBreakerProfile>;
}

function formatUptime(uptimeSeconds: number): string {
  const days = Math.floor(uptimeSeconds / 86400);
  const hours = Math.floor((uptimeSeconds % 86400) / 3600);
  const minutes = Math.floor((uptimeSeconds % 3600) / 60);
  const seconds = uptimeSeconds % 60;

  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m ${seconds}s`;
  } else if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  } else {
    return `${seconds}s`;
  }
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

interface LogoCacheData {
  logo_cache: {
    total_entries: number;
    memory_usage: {
      bytes: number;
      megabytes: string;
      bytes_per_entry: number;
      avg_entry_size_bytes: number;
    };
    storage_usage: {
      bytes: number;
      megabytes: string;
    };
    efficiency: {
      hash_based_indexing: boolean;
      smart_dimension_encoding: string;
      memory_vs_string_storage: string;
    };
    last_updated: string;
    cache_directory: string;
    max_size_mb?: number | null;
    max_age_days?: number | null;
  };
}

function LogoCacheCard() {
  const [cacheData, setCacheData] = useState<LogoCacheData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchCacheData = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/debug/logo-cache`);

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const data: LogoCacheData = await response.json();
      setCacheData(data);
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

  const formatNumber = (num: number): string => {
    if (num >= 1000000) {
      return `${(num / 1000000).toFixed(1)}M`;
    } else if (num >= 1000) {
      return `${(num / 1000).toFixed(1)}K`;
    } else {
      return num.toString();
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Image className="h-5 w-5" />
          Logo Cache
          {loading && <RefreshCw className="h-4 w-4 animate-spin" />}
        </CardTitle>
        <CardDescription>Ultra-compact logo indexing with hash-based matching</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <div className="flex items-center gap-2 text-destructive">
            <XCircle className="h-4 w-4" />
            <span className="text-sm">Error: {error}</span>
          </div>
        )}

        {cacheData && (
          <>
            {/* Status and Overview */}
            <div className="flex items-center gap-2">
              <CheckCircle className="h-4 w-4 text-green-500" />
              <Badge className="bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100">
                Active
              </Badge>
              <Badge variant="outline" className="ml-auto">
                {formatNumber(cacheData.logo_cache.total_entries)} entries
              </Badge>
            </div>

            {/* Performance Metrics */}
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">Performance</h4>
              <div className="grid grid-cols-2 gap-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Memory:</span>
                  <span className="font-medium">
                    {cacheData.logo_cache.memory_usage.megabytes} MB
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Per Entry:</span>
                  <span className="font-medium">
                    {cacheData.logo_cache.memory_usage.bytes_per_entry}B
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Storage:</span>
                  <span className="font-medium">
                    {cacheData.logo_cache.storage_usage.megabytes} MB
                  </span>
                </div>
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
              </div>
            </div>

            {/* Configuration */}
            <div className="space-y-2">
              <h4 className="text-sm font-medium text-muted-foreground">Configuration</h4>
              <div className="grid grid-cols-1 gap-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Size Limit:</span>
                  <span className="font-medium">
                    {cacheData.logo_cache.max_size_mb
                      ? `${cacheData.logo_cache.max_size_mb} MB`
                      : 'Unlimited'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Age Limit:</span>
                  <span className="font-medium">
                    {cacheData.logo_cache.max_age_days
                      ? `${cacheData.logo_cache.max_age_days} days`
                      : 'Unlimited'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Directory:</span>
                  <span className="font-medium text-xs font-mono">
                    {cacheData.logo_cache.cache_directory}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Last Updated:</span>
                  <span className="font-medium text-xs">
                    {new Date(cacheData.logo_cache.last_updated).toLocaleString()}
                  </span>
                </div>
              </div>
            </div>
          </>
        )}

        {!cacheData && !loading && (
          <div className="text-center py-4 text-muted-foreground">
            <Image className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-xs">Logo cache data not available</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function Debug() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [liveProbe, setLiveProbe] = useState<KubernetesProbeResponse | null>(null);
  const [readyProbe, setReadyProbe] = useState<KubernetesProbeResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [rawJsonExpanded, setRawJsonExpanded] = useState(false);

  // Custom step values: 1, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60
  const stepValues = [1, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60];

  // Chart and refresh interval state
  const [refreshInterval, setRefreshInterval] = useState(stepValues[3]); // Default 15 seconds (index 3)
  const [chartData, setChartData] = useState<ChartDataPoint[]>([]);
  const [isAutoRefresh, setIsAutoRefresh] = useState(true); // Start enabled
  const [circuitBreakerConfig, setCircuitBreakerConfig] = useState<CircuitBreakerConfig | null>(
    null
  );

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

      // Fetch Kubernetes probes
      try {
        const liveResponse = await fetch(`${backendUrl}/live`);
        if (liveResponse.ok) {
          const liveData: KubernetesProbeResponse = await liveResponse.json();
          setLiveProbe(liveData);
        }
      } catch (err) {
        console.warn('Live probe endpoint not available');
      }

      try {
        const readyResponse = await fetch(`${backendUrl}/ready`);
        if (readyResponse.ok) {
          const readyData: KubernetesProbeResponse = await readyResponse.json();
          setReadyProbe(readyData);
        }
      } catch (err) {
        console.warn('Ready probe endpoint not available');
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

  // Fetch circuit breaker configuration on component mount
  useEffect(() => {
    const fetchCircuitBreakerConfig = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/circuit-breakers/config`);
        const result = await response.json();

        if (result.success && result.data?.config) {
          setCircuitBreakerConfig(result.data.config);
        }
      } catch (error) {
        console.warn('Failed to fetch circuit breaker configuration:', error);
      }
    };

    fetchCircuitBreakerConfig();
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
              <div className="text-2xl font-bold">{formatUptime(healthData.uptime_seconds)}</div>
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
                    className={`h-2 w-2 rounded-full ${liveProbe?.success ? 'bg-green-500' : 'bg-red-500'}`}
                  />
                  <span>Live: {liveProbe?.success ? 'OK' : 'Fail'}</span>
                </div>
                <div className="flex items-center gap-1 text-xs">
                  <div
                    className={`h-2 w-2 rounded-full ${readyProbe?.success ? 'bg-green-500' : 'bg-red-500'}`}
                  />
                  <span>Ready: {readyProbe?.success ? 'OK' : 'Fail'}</span>
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
          {/* Feature Flags Debug */}
          <FeatureFlagsDebug />

          {/* Circuit Breakers Component */}
          {healthData?.components?.circuit_breakers &&
            Object.keys(healthData.components.circuit_breakers).length > 0 && (
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Shield className="h-5 w-5" />
                    Circuit Breakers
                  </CardTitle>
                  <CardDescription>Active circuit breaker statistics</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <TooltipProvider>
                    {Object.entries(healthData.components.circuit_breakers).map(
                      ([serviceName, stats]) => {
                        const successRate =
                          (stats.successful_calls / (stats.total_calls || 1)) * 100;
                        const successFormat = formatHumanNumber(stats.successful_calls);
                        const failedFormat = formatHumanNumber(stats.failed_calls);
                        const totalFormat = formatHumanNumber(stats.total_calls);

                        // Get configuration for this service (use profile-specific or fallback to global)
                        const config =
                          circuitBreakerConfig?.profiles?.[serviceName] ||
                          circuitBreakerConfig?.global;
                        const profileType = circuitBreakerConfig?.profiles?.[serviceName]
                          ? 'Custom'
                          : 'Global';

                        return (
                          <div key={serviceName} className="bg-muted/50 rounded p-3">
                            <div className="space-y-2">
                              {/* Header row: Service name and Status badge */}
                              <div className="flex justify-between items-center">
                                <div className="font-medium font-mono">{serviceName}</div>
                                <Badge
                                  variant={
                                    stats.state === 'Closed'
                                      ? 'default'
                                      : stats.state === 'Open'
                                        ? 'destructive'
                                        : 'secondary'
                                  }
                                  className="text-xs px-2 py-1 w-20 justify-center"
                                >
                                  {stats.state}
                                </Badge>
                              </div>

                              {/* Configuration info with success badge aligned */}
                              <div className="flex justify-between items-center">
                                <div className="text-xs text-muted-foreground">
                                  {config ? (
                                    <Tooltip>
                                      <TooltipTrigger asChild>
                                        <span className="cursor-help leading-relaxed">
                                          <div>
                                            {profileType}: {config.failure_threshold}→
                                            {config.success_threshold} thresholds
                                          </div>
                                          <div>
                                            {config.operation_timeout} op • {config.reset_timeout}{' '}
                                            reset • {config.acceptable_status_codes.join(', ')}
                                          </div>
                                        </span>
                                      </TooltipTrigger>
                                      <TooltipContent>
                                        <div className="text-xs space-y-1">
                                          <div>
                                            <strong>Type:</strong> {config.implementation_type}
                                          </div>
                                          <div>
                                            <strong>Failure Threshold:</strong>{' '}
                                            {config.failure_threshold} failures to open
                                          </div>
                                          <div>
                                            <strong>Success Threshold:</strong>{' '}
                                            {config.success_threshold} successes to close
                                          </div>
                                          <div>
                                            <strong>Operation Timeout:</strong>{' '}
                                            {config.operation_timeout}
                                          </div>
                                          <div>
                                            <strong>Reset Timeout:</strong> {config.reset_timeout}
                                          </div>
                                          <div>
                                            <strong>Acceptable Codes:</strong>{' '}
                                            {config.acceptable_status_codes.join(', ')}
                                          </div>
                                        </div>
                                      </TooltipContent>
                                    </Tooltip>
                                  ) : (
                                    <span>Profile: Loading...</span>
                                  )}
                                </div>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Badge
                                      variant="outline"
                                      className="text-xs px-2 py-1 text-green-600 border-green-600/50 w-20 justify-center"
                                    >
                                      {successFormat.display}
                                    </Badge>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p>{successFormat.full} successful calls</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>

                              {/* Success percentage and failed badge row */}
                              <div className="flex justify-between items-center">
                                <span className="text-sm font-medium text-muted-foreground">
                                  {formatPercentage(successRate)} success
                                </span>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Badge
                                      variant="outline"
                                      className="text-xs px-2 py-1 text-red-600 border-red-600/50 w-20 justify-center"
                                    >
                                      {failedFormat.display}
                                    </Badge>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p>{failedFormat.full} failed calls</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>

                              {/* Progress bar and total aligned */}
                              <div className="flex items-center gap-3">
                                <div className="flex-1 bg-gray-200 rounded-full h-2 dark:bg-gray-700">
                                  <div
                                    className="bg-gradient-to-r from-green-500 to-green-600 h-2 rounded-full transition-all duration-300"
                                    style={{ width: `${successRate}%` }}
                                  />
                                </div>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Badge
                                      variant="outline"
                                      className="text-xs px-2 py-1 w-20 justify-center"
                                    >
                                      {totalFormat.display}
                                    </Badge>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p>{totalFormat.full} total calls</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>
                            </div>
                          </div>
                        );
                      }
                    )}
                  </TooltipProvider>

                  {Object.keys(healthData.components.circuit_breakers).length === 0 && (
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

          {/* Scheduler Component - Enhanced */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Calendar className="h-5 w-5" />
                Scheduler
              </CardTitle>
              <CardDescription>Source scheduling and ingestion monitoring</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex items-center gap-2">
                {getStatusIcon(healthData?.components?.scheduler?.status)}
                <Badge
                  className={getStatusIndicatorClasses(healthData?.components?.scheduler?.status)}
                >
                  {healthData?.components?.scheduler?.status || 'Unknown'}
                </Badge>
              </div>

              {/* Sources Overview */}
              <div className="space-y-2">
                <h4 className="text-sm font-medium text-muted-foreground">Configured Sources</h4>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Stream Sources:</span>
                    <span className="font-medium">
                      {healthData?.components?.scheduler?.sources_scheduled?.stream_sources || 0}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">EPG Sources:</span>
                    <span className="font-medium">
                      {healthData?.components?.scheduler?.sources_scheduled?.epg_sources || 0}
                    </span>
                  </div>
                </div>
              </div>

              {/* Activity Status */}
              <div className="space-y-2">
                <h4 className="text-sm font-medium text-muted-foreground">Current Activity</h4>
                <div className="grid grid-cols-1 gap-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Active Ingestions:</span>
                    <span className="font-medium">
                      {healthData?.components?.scheduler?.active_ingestions || 0}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Last Cache Refresh:</span>
                    <span className="font-medium text-xs">
                      {new Date(
                        healthData?.components?.scheduler?.last_cache_refresh || new Date()
                      ).toLocaleString()}
                    </span>
                  </div>
                </div>
              </div>

              {/* Next Scheduled Times */}
              {healthData?.components?.scheduler?.next_scheduled_times &&
                healthData.components.scheduler.next_scheduled_times.length > 0 && (
                  <div className="space-y-2">
                    <h4 className="text-sm font-medium text-muted-foreground">
                      Next Scheduled Runs
                    </h4>
                    <div className="space-y-2">
                      {(healthData?.components?.scheduler?.next_scheduled_times || []).map(
                        (schedule, index) => (
                          <div key={index} className="bg-muted/50 rounded p-2">
                            <div className="flex justify-between items-start text-xs">
                              <div>
                                <div className="font-medium">{schedule.source_name}</div>
                                <div className="text-muted-foreground">{schedule.source_type}</div>
                              </div>
                              <div className="text-right">
                                <div className="font-medium">
                                  {new Date(schedule.next_run).toLocaleString()}
                                </div>
                                <div className="text-muted-foreground font-mono text-xs">
                                  {schedule.cron_expression}
                                </div>
                              </div>
                            </div>
                          </div>
                        )
                      )}
                    </div>
                  </div>
                )}
            </CardContent>
          </Card>

          {/* Sandbox Manager Component - Enhanced */}
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
                {getStatusIcon(healthData?.components?.sandbox_manager?.status)}
                <Badge
                  className={getStatusIndicatorClasses(
                    healthData?.components?.sandbox_manager?.status
                  )}
                >
                  {healthData?.components?.sandbox_manager?.status || 'Unknown'}
                </Badge>
                <Badge variant="outline" className="ml-auto capitalize">
                  {healthData.components.sandbox_manager.cleanup_status}
                </Badge>
              </div>

              {/* Cleanup Statistics */}
              <div className="space-y-2">
                <h4 className="text-sm font-medium text-muted-foreground">Latest Cleanup</h4>
                <div className="grid grid-cols-2 gap-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Files Cleaned:</span>
                    <span className="font-medium">
                      {healthData.components.sandbox_manager.temp_files_cleaned}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Space Freed:</span>
                    <span className="font-medium">
                      {formatMemorySize(healthData.components.sandbox_manager.disk_space_freed_mb)}
                    </span>
                  </div>
                </div>
                <div className="grid grid-cols-1 gap-2 text-sm">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Last Cleanup:</span>
                    <span className="font-medium text-xs">
                      {new Date(
                        healthData.components.sandbox_manager.last_cleanup_run
                      ).toLocaleString()}
                    </span>
                  </div>
                </div>
              </div>

              {/* Managed Directories */}
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
            </CardContent>
          </Card>

          {/* Logo Cache Component */}
          <LogoCacheCard />

          {/* FFmpeg Information */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Settings className="h-5 w-5" />
                FFmpeg
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-center gap-2">
                {healthData.components.relay_system.ffmpeg_available ? (
                  <CheckCircle className="h-4 w-4 text-green-500" />
                ) : (
                  <XCircle className="h-4 w-4 text-red-500" />
                )}
                <Badge
                  variant={
                    healthData.components.relay_system.ffmpeg_available ? 'default' : 'destructive'
                  }
                >
                  {healthData.components.relay_system.ffmpeg_available
                    ? 'Available'
                    : 'Unavailable'}
                </Badge>
              </div>
              <div className="space-y-1 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">FFmpeg Version:</span>
                  <span className="font-medium">
                    {healthData.components.relay_system.ffmpeg_version || 'null'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">FFprobe Version:</span>
                  <span className="font-medium">
                    {healthData.components.relay_system.ffprobe_version || 'null'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">HW Accel:</span>
                  <span className="font-medium">
                    {healthData.components.relay_system.hwaccel_available ? (
                      <CheckCircle className="h-3 w-3 text-green-500 inline" />
                    ) : (
                      <XCircle className="h-3 w-3 text-red-500 inline" />
                    )}
                  </span>
                </div>
                {healthData.components.relay_system.hwaccel_available &&
                  healthData.components.relay_system.hwaccel_capabilities && (
                    <div className="pt-2 border-t">
                      <p className="text-xs font-medium text-muted-foreground mb-2">
                        Hardware Acceleration Support Matrix:
                      </p>
                      <div className="space-y-2">
                        {healthData.components.relay_system.hwaccel_capabilities.support_matrix &&
                          Object.entries(
                            healthData.components.relay_system.hwaccel_capabilities.support_matrix
                          ).map(([accel, support]) => (
                            <div key={accel} className="bg-muted/50 rounded p-2">
                              <div className="flex justify-between items-center text-xs">
                                <div className="font-medium">{accel.toUpperCase()}</div>
                                <div className="flex gap-1">
                                  {Object.entries(support).map(([codec, supported]) => (
                                    <Badge
                                      key={codec}
                                      variant={supported ? 'default' : 'outline'}
                                      className={`text-xs ${supported ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100' : 'text-muted-foreground'}`}
                                    >
                                      {codec.toUpperCase()}
                                    </Badge>
                                  ))}
                                </div>
                              </div>
                            </div>
                          ))}
                      </div>
                    </div>
                  )}
              </div>
            </CardContent>
          </Card>
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
