'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { Label } from '@/components/ui/label';
import {
  Activity,
  Users,
  Zap,
  TrendingUp,
  Clock,
  HardDrive,
  Cpu,
  MemoryStick,
  Monitor,
  ArrowUp,
  ArrowDown,
} from 'lucide-react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  LineChart,
  Line,
  ComposedChart,
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
  HealthData,
  RelayHealthApiResponse,
} from '@/types/api';
import { Debug } from '@/utils/debug';
import { apiClient } from '@/lib/api-client';
import {
  formatBytes,
  formatBitrate,
  formatMemorySize,
  parseStringNumber,
} from '@/lib/format';
import { RelayFlowDiagram } from '@/components/relay';

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

export function Dashboard() {
  const [dashboardMetrics, setDashboardMetrics] = useState<DashboardMetrics>(mockDashboardMetrics);
  const [clientMetrics, setClientMetrics] = useState<ClientMetrics[]>(mockClientMetrics);
  const [relayMetrics, setRelayMetrics] = useState<RelayMetrics[]>(mockRelayMetrics);
  const [isLoading, setIsLoading] = useState(false);
  const [systemHealth, setSystemHealth] = useState<HealthData | null>(null);
  const [relayHealth, setRelayHealth] = useState<RelayHealthApiResponse | null>(null);

  // Update interval management - same as debug page
  const stepValues = [1, 5, 10, 15, 30, 45, 60, 90, 120]; // Update intervals in seconds
  const [refreshInterval, setRefreshInterval] = useState(stepValues[3]); // Default 15 seconds (index 3)
  const [isAutoRefresh, setIsAutoRefresh] = useState(true); // Start enabled
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  // Centralized refresh function for all dashboard data
  const refreshDashboardData = useCallback(async () => {
    setIsLoading(true);

    try {
      // Fetch system health and relay health data
      const [sysHealth, health] = await Promise.all([
        apiClient.healthCheck(),
        apiClient.getRelayHealth(),
      ]);
      setSystemHealth(sysHealth);
      setRelayHealth(health);

      // TODO: Add other real API calls here as we implement them
      // Example: const clientData = await apiClient.getClientMetrics()
      // setClientMetrics(clientData)
    } catch (error) {
      console.error('Failed to fetch dashboard data:', error);
    } finally {
      setIsLoading(false);
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
                  {isLoading && (
                    <>
                      <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-primary" />
                      <span>Refreshing...</span>
                    </>
                  )}
                  {isAutoRefresh && !isLoading && <span>Auto-refresh active</span>}
                  {!isAutoRefresh && !isLoading && <span>Auto-refresh disabled</span>}
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

      {/* Relay Flow Visualization */}
      <RelayFlowDiagram
        pollingInterval={refreshInterval * 1000}
        className="w-full"
      />
    </div>
  );
}
