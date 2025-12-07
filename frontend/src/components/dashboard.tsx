'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Activity,
  Users,
  Zap,
  Server,
  TrendingUp,
  Wifi,
  Clock,
  HardDrive,
  Cpu,
  MemoryStick,
  AlertTriangle,
  ArrowUp,
  ArrowDown,
  Monitor,
  Globe,
} from 'lucide-react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  LineChart,
  Line,
  ComposedChart,
  ReferenceLine,
} from 'recharts';
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from '@/components/ui/chart';
import {
  ClientMetrics,
  RelayMetrics,
  DashboardMetrics,
  RelayHealthApiResponse,
  RelayProcess,
  RelayConnectedClient,
  HealthData,
} from '@/types/api';
import { Debug } from '@/utils/debug';
import { getStatusIndicatorClasses, getStatusType } from '@/lib/status-colors';
import { apiClient } from '@/lib/api-client';
import {
  formatBytes,
  formatBandwidth,
  formatBitrate,
  formatUptimeFromSeconds,
  formatTimeConnected,
  formatMemorySize,
  parseStringNumber,
} from '@/lib/format';

// Interface for historical data points for relay processes
interface RelayDataPoint {
  timestamp: string;
  time: string; // Formatted time for display
  cpuUsage: number;
  memoryUsageMb: number;
  bytesReceivedTotal: number; // cumulative bytes received
  bytesDeliveredTotal: number; // cumulative bytes delivered
  bytesReceivedRate: number; // calculated bytes/second since last data point (will be negative for display)
  bytesDeliveredRate: number; // calculated bytes/second since last data point (positive for display)
}

interface RelayHistoricalData {
  [processId: string]: RelayDataPoint[];
}

// Mock data - in real app, this would come from API
const mockDashboardMetrics: DashboardMetrics = {
  active_clients: 1247,
  active_relays: 8,
  total_channels: 2851,
  total_bandwidth: 2.8, // GB/s
  system_health: 'healthy',
  uptime: '15d 4h 23m',
};

const mockClientMetrics: ClientMetrics[] = [
  {
    id: '1',
    ip_address: '192.168.1.100',
    user_agent: 'VLC/3.0.16',
    channel_name: 'HBO Max',
    channel_id: 'hbo-max',
    proxy_name: 'Premium Channels',
    connected_at: '2024-01-15T10:30:00Z',
    data_transferred: 1.2, // GB
    current_bitrate: 8.5, // Mbps
    status: 'connected',
  },
  {
    id: '2',
    ip_address: '10.0.0.50',
    user_agent: 'Kodi/19.4',
    channel_name: 'ESPN',
    channel_id: 'espn',
    proxy_name: 'Sports Package',
    connected_at: '2024-01-15T09:15:00Z',
    data_transferred: 2.8,
    current_bitrate: 12.1,
    status: 'connected',
  },
  {
    id: '3',
    ip_address: '172.16.0.25',
    user_agent: 'IPTV Smarters/3.0',
    channel_name: 'Discovery Channel',
    channel_id: 'discovery',
    proxy_name: 'Documentary Pack',
    connected_at: '2024-01-15T11:45:00Z',
    data_transferred: 0.9,
    current_bitrate: 6.2,
    status: 'buffering',
  },
];

const mockRelayMetrics: RelayMetrics[] = [
  {
    config_id: 'relay-1',
    channel_name: 'CNN HD',
    connected_clients: 45,
    upstream_bitrate: 15.2,
    downstream_bitrate: 682.5,
    cpu_usage: 23.5,
    memory_usage: 890.2,
    status: 'running',
    uptime: '2d 14h 22m',
  },
  {
    config_id: 'relay-2',
    channel_name: 'Fox Sports 1',
    connected_clients: 38,
    upstream_bitrate: 18.7,
    downstream_bitrate: 711.3,
    cpu_usage: 31.2,
    memory_usage: 1205.8,
    status: 'running',
    uptime: '1d 8h 15m',
  },
];

function formatUptime(uptime: string): string {
  return uptime;
}

function getStatusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  const statusType = getStatusType(status);
  switch (statusType) {
    case 'success':
      return 'default';
    case 'warning':
      return 'secondary';
    case 'error':
      return 'destructive';
    case 'info':
      return 'outline';
    default:
      return 'outline';
  }
}

// Helper function to validate and clean chart data
function validateChartData(
  data: RelayDataPoint[] = [],
  keys: (keyof RelayDataPoint)[]
): RelayDataPoint[] {
  if (!data || data.length < 2) return [];

  const filtered = data.filter((point) => {
    // Ensure point exists and has required structure
    if (!point || !point.time || !point.timestamp) return false;

    // Ensure all required keys exist and are valid numbers
    return keys.every((key) => {
      const value = point[key];
      return typeof value === 'number' && !isNaN(value) && isFinite(value);
    });
  });

  // Debug logging for CPU data
  if (keys.includes('cpuUsage') && filtered.length > 0) {
    const cpuValues = filtered.map((p) => p.cpuUsage);
    Debug.log('CPU chart data:', {
      count: filtered.length,
      min: Math.min(...cpuValues),
      max: Math.max(...cpuValues),
      values: cpuValues,
    });
  }

  // Ensure we have clean array indices for recharts
  return filtered.map((point, index) => ({
    ...point,
    _index: index, // Add explicit index to help recharts
  }));
}

export function Dashboard() {
  const [dashboardMetrics, setDashboardMetrics] = useState<DashboardMetrics>(mockDashboardMetrics);
  const [clientMetrics, setClientMetrics] = useState<ClientMetrics[]>(mockClientMetrics);
  const [relayMetrics, setRelayMetrics] = useState<RelayMetrics[]>(mockRelayMetrics);
  const [relayHealth, setRelayHealth] = useState<RelayHealthApiResponse | null>(null);
  const [relayHealthError, setRelayHealthError] = useState<string | null>(null);
  const [isLoadingRelays, setIsLoadingRelays] = useState(false);
  const [systemHealth, setSystemHealth] = useState<HealthData | null>(null);

  // Integrate with page loading spinner
  const [relayHistoricalData, setRelayHistoricalData] = useState<RelayHistoricalData>({});

  // Keep max 60 data points per relay process (adjust based on needs)
  const MAX_RELAY_DATA_POINTS = 60;

  // Update interval management - same as debug page
  const stepValues = [1, 5, 10, 15, 30, 45, 60, 90, 120]; // Update intervals in seconds
  const [refreshInterval, setRefreshInterval] = useState(stepValues[3]); // Default 15 seconds (index 3)
  const [isAutoRefresh, setIsAutoRefresh] = useState(true); // Start enabled
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Centralized refresh function for all dashboard data
  const refreshDashboardData = useCallback(async () => {
    setIsLoadingRelays(true);
    setRelayHealthError(null);

    try {
      // Fetch both relay health and system health data
      const [health, sysHealth] = await Promise.all([
        apiClient.getRelayHealth(),
        apiClient.healthCheck(),
      ]);
      setRelayHealth(health);
      setSystemHealth(sysHealth);

      // Update historical data for each relay process
      if (health?.processes) {
        const now = new Date();
        const timeFormatted = now.toLocaleTimeString('en-US', {
          hour12: false,
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit',
        });

        setRelayHistoricalData((prev) => {
          const updated = { ...prev };

          health.processes?.forEach((process) => {
            const processId = process.config_id;
            const currentBytesReceived = parseStringNumber(process.bytes_received_upstream);
            const currentBytesDelivered = parseStringNumber(process.bytes_delivered_downstream);

            // Get previous data point to calculate rate
            const previousData = updated[processId]?.[updated[processId].length - 1];
            let bytesReceivedRate = 0;
            let bytesDeliveredRate = 0;

            if (previousData) {
              const timeDiffMs = now.getTime() - new Date(previousData.timestamp).getTime();
              const timeDiffSeconds = timeDiffMs / 1000;

              if (timeDiffSeconds > 0) {
                // Calculate bytes per second since last data point
                const receivedDelta = Math.max(
                  0,
                  currentBytesReceived - previousData.bytesReceivedTotal
                );
                const deliveredDelta = Math.max(
                  0,
                  currentBytesDelivered - previousData.bytesDeliveredTotal
                );

                // Make ingress negative for visualization (below zero line)
                bytesReceivedRate = -(receivedDelta / timeDiffSeconds);
                // Keep egress positive (above zero line)
                bytesDeliveredRate = deliveredDelta / timeDiffSeconds;
              }
            }

            // Create new data point for this process with safe defaults and validation
            const cpuValue = parseStringNumber(process.cpu_usage_percent) || 0;
            const memoryValue = parseStringNumber(process.memory_usage_mb) || 0;
            const dataPoint: RelayDataPoint = {
              timestamp: now.toISOString(),
              time: timeFormatted,
              cpuUsage: Math.max(0, Math.min(100, isNaN(cpuValue) ? 0 : cpuValue)), // Clamp CPU between 0-100
              memoryUsageMb: Math.max(0, isNaN(memoryValue) ? 0 : memoryValue),
              bytesReceivedTotal: Math.max(0, currentBytesReceived || 0),
              bytesDeliveredTotal: Math.max(0, currentBytesDelivered || 0),
              bytesReceivedRate:
                isNaN(bytesReceivedRate) || !isFinite(bytesReceivedRate)
                  ? 0
                  : Math.max(-1000000000, Math.min(0, bytesReceivedRate)), // Clamp negative ingress
              bytesDeliveredRate:
                isNaN(bytesDeliveredRate) || !isFinite(bytesDeliveredRate)
                  ? 0
                  : Math.max(0, Math.min(1000000000, bytesDeliveredRate)), // Clamp positive egress
            };

            // Add data point to process history
            if (!updated[processId]) {
              updated[processId] = [];
            }

            updated[processId] = [...updated[processId], dataPoint].slice(-MAX_RELAY_DATA_POINTS);
          });

          return updated;
        });
      }

      // TODO: Add other real API calls here as we implement them
      // Example: const clientData = await apiClient.getClientMetrics()
      // setClientMetrics(clientData)
    } catch (error) {
      setRelayHealthError(error instanceof Error ? error.message : 'Failed to load dashboard data');
      console.error('Failed to fetch dashboard data:', error);
    } finally {
      setIsLoadingRelays(false);
    }
  }, []);

  // Auto-refresh management
  const startAutoRefresh = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }

    Debug.log('Starting dashboard auto-refresh with interval:', refreshInterval, 'seconds');
    intervalRef.current = setInterval(() => {
      refreshDashboardData();
    }, refreshInterval * 1000);

    setIsAutoRefresh(true);
  }, [refreshDashboardData, refreshInterval]);

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
      Debug.log('Dashboard refresh interval changed:', { sliderIndex, newInterval, stepValues });
      setRefreshInterval(newInterval);
    },
    [stepValues]
  );

  // Initial data load and auto-refresh setup
  useEffect(() => {
    refreshDashboardData(); // Initial load
    startAutoRefresh(); // Start auto-refresh

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [refreshDashboardData, startAutoRefresh]);

  // Update auto-refresh when refresh interval changes
  useEffect(() => {
    if (isAutoRefresh) {
      Debug.log('Restarting dashboard auto-refresh due to interval change:', refreshInterval);
      stopAutoRefresh();
      startAutoRefresh();
    }
  }, [refreshInterval, isAutoRefresh, startAutoRefresh, stopAutoRefresh]);

  return (
    <div className="space-y-6">
      {/* Header with description and refresh controls */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <p className="text-muted-foreground">
            Real-time system monitoring and relay process metrics
          </p>
        </div>
        {/* Refresh Interval Slider */}
        <Card className="p-4 min-w-[300px]">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex flex-col">
                <Label htmlFor="refresh-interval" className="text-sm font-medium">
                  Update Interval: {refreshInterval}s
                </Label>
                <div className="flex items-center gap-2 text-xs text-muted-foreground mt-1">
                  {isLoadingRelays && (
                    <>
                      <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-primary" />
                      <span>Refreshing...</span>
                    </>
                  )}
                  {isAutoRefresh && !isLoadingRelays && <span>Auto-refresh active</span>}
                  {!isAutoRefresh && !isLoadingRelays && <span>Auto-refresh disabled</span>}
                </div>
              </div>
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
              <span>{stepValues[0]}s</span>
              <span>{stepValues[stepValues.length - 1]}s</span>
            </div>
          </div>
        </Card>
      </div>

      {/* Overview Cards - REAL DATA */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Connected Clients</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {relayHealth
                ? relayHealth.processes?.reduce((acc, p) => acc + (p.connected_clients?.length ?? 0), 0) ?? 0
                : '–'}
            </div>
            <p className="text-xs text-muted-foreground">Across all relay processes</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active Relays</CardTitle>
            <Zap className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {relayHealth
                ? `${relayHealth.healthy_processes}/${relayHealth.total_processes}`
                : '–'}
            </div>
            <p className="text-xs text-muted-foreground">
              {relayHealth ? `${relayHealth.unhealthy_processes} unhealthy` : 'Loading...'}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Bandwidth</CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="space-y-1">
              <div className="flex items-center gap-2 text-lg font-bold">
                <ArrowUp className="h-3 w-3 text-blue-500" />
                <span>
                  {relayHealth
                    ? formatBytes(
                        relayHealth.processes?.reduce(
                          (acc, p) => acc + parseStringNumber(p.bytes_received_upstream),
                          0
                        ) ?? 0
                      )
                    : '–'}
                </span>
              </div>
              <div className="flex items-center gap-2 text-lg font-bold">
                <ArrowDown className="h-3 w-3 text-green-500" />
                <span>
                  {relayHealth
                    ? formatBytes(
                        relayHealth.processes?.reduce(
                          (acc, p) => acc + parseStringNumber(p.bytes_delivered_downstream),
                          0
                        ) ?? 0
                      )
                    : '–'}
                </span>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">Ingress / Egress totals</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System CPU</CardTitle>
            <Cpu className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {systemHealth ? `${systemHealth.cpu_info.load_percentage_1min.toFixed(1)}%` : '–'}
            </div>
            <p className="text-xs text-muted-foreground">1min load average</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System Memory</CardTitle>
            <HardDrive className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {systemHealth
                ? `${((systemHealth.memory.used_memory_mb / systemHealth.memory.total_memory_mb) * 100).toFixed(1)}%`
                : '–'}
            </div>
            <div className="space-y-1 text-xs text-muted-foreground">
              <p>
                Total: {systemHealth ? formatMemorySize(systemHealth.memory.total_memory_mb) : '–'}
              </p>
              <p>
                Used: {systemHealth ? formatMemorySize(systemHealth.memory.used_memory_mb) : '–'}
              </p>
              <p>
                tvarr:{' '}
                {systemHealth
                  ? formatMemorySize(systemHealth.memory.process_memory.main_process_mb)
                  : '–'}
              </p>
              <p>
                Child Processes:{' '}
                {systemHealth
                  ? formatMemorySize(systemHealth.memory.process_memory.child_processes_mb)
                  : '–'}
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System Health</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="flex items-center space-x-2">
              <div
                className={`h-2 w-2 rounded-full ${relayHealth && parseInt(relayHealth.unhealthy_processes) === 0 ? 'bg-green-500' : relayHealth && parseInt(relayHealth.unhealthy_processes) > 0 ? 'bg-yellow-500' : 'bg-gray-400'}`}
              />
              <span className="text-2xl font-bold capitalize">
                {relayHealth
                  ? parseInt(relayHealth.unhealthy_processes) === 0
                    ? 'Healthy'
                    : 'Warning'
                  : 'Unknown'}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {relayHealth
                ? `Updated ${new Date(relayHealth.last_check).toLocaleTimeString()}`
                : 'Loading...'}
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Relay Process Cards */}
      <TooltipProvider>
        <div className="space-y-4">
          {relayHealthError ? (
            <Alert variant="destructive">
              <AlertTriangle className="h-4 w-4" />
              <AlertDescription>Failed to load relay data: {relayHealthError}</AlertDescription>
            </Alert>
          ) : relayHealth ? (
            <>
              {/* Individual Relay Process Cards */}
              <div className="space-y-4">
                <h3 className="text-lg font-medium">Active Relay Processes</h3>
                {!relayHealth.processes?.length ? (
                  <Card>
                    <CardContent className="text-center py-8">
                      <Server className="h-12 w-12 mx-auto mb-4 text-muted-foreground" />
                      <p className="text-muted-foreground">No active relay processes found</p>
                    </CardContent>
                  </Card>
                ) : (
                  <div className="grid gap-4">
                    {relayHealth.processes?.map((process) => (
                      <Card key={process.config_id} className="w-full">
                        <CardHeader>
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-3">
                              <div
                                className={`h-3 w-3 rounded-full ${getStatusIndicatorClasses(process.status)}`}
                              />
                              <div>
                                <CardTitle className="text-lg">
                                  {process.channel_name || process.profile_name}
                                </CardTitle>
                                <CardDescription>
                                  Profile: {process.profile_name} | PID: {process.pid || 'N/A'} |
                                  Uptime:{' '}
                                  {formatUptimeFromSeconds(
                                    parseStringNumber(process.uptime_seconds)
                                  )}
                                </CardDescription>
                              </div>
                            </div>
                            <div className="flex items-center gap-2">
                              <Badge variant={getStatusVariant(process.status)}>
                                {process.status}
                              </Badge>
                              <Badge variant="outline">
                                {process.connected_clients?.length ?? 0} clients
                              </Badge>
                            </div>
                          </div>
                        </CardHeader>
                        <CardContent>
                          <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
                            {/* Left Columns: Charts in single column, 3 rows */}
                            <div className="lg:col-span-3 space-y-4">
                              {/* CPU Usage Chart */}
                              <div className="space-y-3">
                                <div className="flex items-center gap-2">
                                  <Cpu className="h-4 w-4 text-muted-foreground" />
                                  <span className="text-sm font-medium">CPU Usage</span>
                                  <span className="text-xs text-muted-foreground ml-auto">
                                    {parseStringNumber(process.cpu_usage_percent).toFixed(1)}%
                                  </span>
                                </div>
                                {(() => {
                                  const cpuData = validateChartData(
                                    relayHistoricalData[process.config_id],
                                    ['cpuUsage']
                                  );
                                  return cpuData.length >= 2 ? (
                                    <ChartContainer
                                      config={
                                        {
                                          cpuUsage: {
                                            label: 'CPU Usage',
                                            color: 'var(--chart-1)',
                                          },
                                        } satisfies ChartConfig
                                      }
                                      className="h-[80px] w-full"
                                    >
                                      <AreaChart data={cpuData}>
                                        <XAxis dataKey="time" hide />
                                        <YAxis
                                          domain={[
                                            0,
                                            (dataMax: number) => Math.ceil(dataMax * 1.1),
                                          ]}
                                          tickFormatter={(value) => `${value}%`}
                                          className="text-xs"
                                          width={35}
                                        />
                                        <ChartTooltip
                                          content={
                                            <ChartTooltipContent
                                              labelFormatter={(label) => `Time: ${label}`}
                                              formatter={(value) => [
                                                `${Number(value).toFixed(1)}% `,
                                                'CPU Usage',
                                              ]}
                                            />
                                          }
                                        />
                                        <Area
                                          type="monotone"
                                          dataKey="cpuUsage"
                                          stroke="var(--color-cpuUsage)"
                                          fill="var(--color-cpuUsage)"
                                          fillOpacity={0.6}
                                          strokeWidth={1.5}
                                          connectNulls={false}
                                        />
                                      </AreaChart>
                                    </ChartContainer>
                                  ) : (
                                    <div className="h-[80px] flex items-center justify-center text-muted-foreground text-xs">
                                      <span>Collecting data...</span>
                                    </div>
                                  );
                                })()}
                              </div>

                              {/* Memory Usage Chart */}
                              <div className="space-y-3">
                                <div className="flex items-center gap-2">
                                  <MemoryStick className="h-4 w-4 text-muted-foreground" />
                                  <span className="text-sm font-medium">Memory Usage</span>
                                  <span className="text-xs text-muted-foreground ml-auto">
                                    {parseStringNumber(process.memory_usage_mb).toFixed(1)} MB
                                  </span>
                                </div>
                                {(() => {
                                  const memoryData = validateChartData(
                                    relayHistoricalData[process.config_id],
                                    ['memoryUsageMb']
                                  );
                                  return memoryData.length >= 2 ? (
                                    <ChartContainer
                                      config={
                                        {
                                          memoryUsageMb: {
                                            label: 'Memory Usage',
                                            color: 'var(--chart-1)',
                                          },
                                        } satisfies ChartConfig
                                      }
                                      className="h-[80px] w-full"
                                    >
                                      <AreaChart data={memoryData}>
                                        <XAxis dataKey="time" hide />
                                        <YAxis
                                          domain={[
                                            0,
                                            (dataMax: number) => Math.ceil(dataMax * 1.1),
                                          ]}
                                          tickFormatter={(value) =>
                                            formatBytes(Number(value) * 1024 * 1024)
                                          }
                                          className="text-xs"
                                          width={50}
                                        />
                                        <ChartTooltip
                                          content={
                                            <ChartTooltipContent
                                              labelFormatter={(label) => `Time: ${label}`}
                                              formatter={(value) => [
                                                `${formatBytes(Number(value) * 1024 * 1024)} `,
                                                'Memory Usage',
                                              ]}
                                            />
                                          }
                                        />
                                        <Area
                                          type="monotone"
                                          dataKey="memoryUsageMb"
                                          stroke="var(--color-memoryUsageMb)"
                                          fill="var(--color-memoryUsageMb)"
                                          fillOpacity={0.6}
                                          strokeWidth={1.5}
                                          connectNulls={false}
                                        />
                                      </AreaChart>
                                    </ChartContainer>
                                  ) : (
                                    <div className="h-[80px] flex items-center justify-center text-muted-foreground text-xs">
                                      <span>Collecting data...</span>
                                    </div>
                                  );
                                })()}
                              </div>

                              {/* Bandwidth Chart */}
                              <div className="space-y-3">
                                <div className="flex items-center gap-2">
                                  <Wifi className="h-4 w-4 text-muted-foreground" />
                                  <span className="text-sm font-medium">Bandwidth</span>
                                  <div className="text-xs text-muted-foreground ml-auto space-y-0.5">
                                    <div className="flex items-center gap-1">
                                      <ArrowUp className="h-2 w-2 text-blue-500" />
                                      <span>
                                        Total:{' '}
                                        {formatBytes(
                                          parseStringNumber(process.bytes_received_upstream)
                                        )}
                                      </span>
                                    </div>
                                    <div className="flex items-center gap-1">
                                      <ArrowDown className="h-2 w-2 text-green-500" />
                                      <span>
                                        Total:{' '}
                                        {formatBytes(
                                          parseStringNumber(process.bytes_delivered_downstream)
                                        )}
                                      </span>
                                    </div>
                                  </div>
                                </div>
                                {(() => {
                                  const bandwidthData = validateChartData(
                                    relayHistoricalData[process.config_id],
                                    ['bytesReceivedRate', 'bytesDeliveredRate']
                                  );
                                  return bandwidthData.length >= 2 ? (
                                    <ChartContainer
                                      config={
                                        {
                                          bytesReceivedRate: {
                                            label: 'Ingress',
                                            color: 'var(--chart-1)',
                                          },
                                          bytesDeliveredRate: {
                                            label: 'Egress',
                                            color: 'var(--chart-2)',
                                          },
                                        } satisfies ChartConfig
                                      }
                                      className="h-[80px] w-full"
                                    >
                                      <AreaChart data={bandwidthData}>
                                        <XAxis dataKey="time" hide />
                                        <YAxis
                                          domain={['dataMin', 'dataMax']}
                                          tickFormatter={(value) =>
                                            formatBandwidth(Math.abs(Number(value)))
                                          }
                                          className="text-xs"
                                          width={60}
                                        />
                                        <CartesianGrid strokeDasharray="3 3" opacity={0.3} />
                                        <ReferenceLine
                                          y={0}
                                          stroke="hsl(var(--muted-foreground))"
                                          strokeDasharray="5 5"
                                          opacity={0.8}
                                        />
                                        <ChartTooltip
                                          content={
                                            <ChartTooltipContent
                                              labelFormatter={(label) => `Time: ${label}`}
                                              formatter={(value, name) => {
                                                const nameMap: Record<string, string> = {
                                                  bytesReceivedRate: 'Ingress',
                                                  bytesDeliveredRate: 'Egress',
                                                };
                                                // Show absolute values in tooltip but keep the directional indication
                                                const absValue = Math.abs(Number(value));
                                                const direction =
                                                  name === 'bytesReceivedRate' ? ' (↓)' : ' (↑)';
                                                return [
                                                  `${formatBandwidth(absValue)}${direction} `,
                                                  nameMap[name] || name,
                                                ];
                                              }}
                                            />
                                          }
                                        />
                                        <Area
                                          type="monotone"
                                          dataKey="bytesReceivedRate"
                                          stroke="var(--color-bytesReceivedRate)"
                                          fill="var(--color-bytesReceivedRate)"
                                          fillOpacity={0.6}
                                          strokeWidth={1.5}
                                        />
                                        <Area
                                          type="monotone"
                                          dataKey="bytesDeliveredRate"
                                          stroke="var(--color-bytesDeliveredRate)"
                                          fill="var(--color-bytesDeliveredRate)"
                                          fillOpacity={0.6}
                                          strokeWidth={1.5}
                                        />
                                      </AreaChart>
                                    </ChartContainer>
                                  ) : (
                                    <div className="h-[80px] flex items-center justify-center text-muted-foreground text-xs">
                                      <span>Collecting data...</span>
                                    </div>
                                  );
                                })()}
                              </div>
                            </div>

                            {/* Right Column: Connected Clients (spans full height) */}
                            <div className="space-y-3">
                              <div className="flex items-center gap-2">
                                <Globe className="h-4 w-4 text-muted-foreground" />
                                <span className="text-sm font-medium">Connected Clients</span>
                                <span className="text-xs text-muted-foreground ml-auto">
                                  {process.connected_clients?.length ?? 0} connected
                                </span>
                              </div>
                              <ScrollArea className="h-[320px]">
                                <div className="space-y-2 pr-2">
                                  {!process.connected_clients?.length ? (
                                    <div className="h-[320px] flex items-center justify-center">
                                      <span className="text-sm text-muted-foreground">
                                        No clients connected
                                      </span>
                                    </div>
                                  ) : (
                                    process.connected_clients.map((client) => (
                                      <Tooltip key={client.id}>
                                        <TooltipTrigger asChild>
                                          <div className="text-sm p-2 bg-secondary rounded cursor-pointer hover:bg-secondary/80 border">
                                            <div className="font-medium">{client.ip}</div>
                                          </div>
                                        </TooltipTrigger>
                                        <TooltipContent>
                                          <div className="space-y-1">
                                            <div>
                                              <strong>IP:</strong> {client.ip}
                                            </div>
                                            <div>
                                              <strong>User Agent:</strong>{' '}
                                              {client.user_agent || 'Unknown'}
                                            </div>
                                            <div>
                                              <strong>Connected:</strong>{' '}
                                              {formatTimeConnected(client.connected_at)}
                                            </div>
                                            <div>
                                              <strong>Data Served:</strong>{' '}
                                              {formatBytes(parseStringNumber(client.bytes_served))}
                                            </div>
                                            <div>
                                              <strong>Last Activity:</strong>{' '}
                                              {new Date(client.last_activity).toLocaleString()}
                                            </div>
                                          </div>
                                        </TooltipContent>
                                      </Tooltip>
                                    ))
                                  )}
                                </div>
                              </ScrollArea>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                )}
              </div>
            </>
          ) : isLoadingRelays ? (
            <Card>
              <CardContent className="text-center py-8">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto mb-4" />
                <p className="text-muted-foreground">Loading relay data...</p>
              </CardContent>
            </Card>
          ) : null}
        </div>
      </TooltipProvider>
    </div>
  );
}
