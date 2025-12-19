'use client';

import { useState, useEffect, useCallback } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Activity,
  Users,
  Zap,
  Clock,
  HardDrive,
  Cpu,
  MemoryStick,
  Monitor,
} from 'lucide-react';
import { RefreshControl } from '@/components/ui/refresh-control';
import { useAutoRefresh } from '@/hooks/use-auto-refresh';
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
  formatMemorySize,
} from '@/lib/format';
import { RelayFlowDiagram } from '@/components/relay';
import { SetupWizard } from '@/components/SetupWizard';

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

  // Setup wizard state
  const [setupStatus, setSetupStatus] = useState<{
    hasStreamSources: boolean;
    hasEpgSources: boolean;
    hasProxies: boolean;
    isLoaded: boolean;
  }>({
    hasStreamSources: true, // Default to true to avoid flash
    hasEpgSources: true,
    hasProxies: true,
    isLoaded: false,
  });

  // Centralized refresh function for all dashboard data
  const refreshDashboardData = useCallback(async () => {
    setIsLoading(true);

    try {
      // Fetch system health and setup status in parallel
      const [sysHealth, streamSources, epgSources, proxies] = await Promise.all([
        apiClient.healthCheck(),
        apiClient.getStreamSources().catch(() => ({ items: [] })),
        apiClient.getEpgSources().catch(() => ({ items: [] })),
        apiClient.getProxies().catch(() => ({ items: [] })),
      ]);

      setSystemHealth(sysHealth);

      // Update setup status
      setSetupStatus({
        hasStreamSources: (streamSources.items?.length ?? 0) > 0,
        hasEpgSources: (epgSources.items?.length ?? 0) > 0,
        hasProxies: (proxies.items?.length ?? 0) > 0,
        isLoaded: true,
      });

      // TODO: Add relay health endpoint when implemented
      // The /api/v1/relay/health endpoint doesn't exist yet
      // setRelayHealth(health);

      // TODO: Add other real API calls here as we implement them
      // Example: const clientData = await apiClient.getClientMetrics()
      // setClientMetrics(clientData)
    } catch (error) {
      console.error('Failed to fetch dashboard data:', error);
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Auto-refresh using the shared hook
  // Dashboard uses different step values: no "off" option, more granular intervals
  const autoRefresh = useAutoRefresh({
    onRefresh: refreshDashboardData,
    stepValues: [0, 1, 5, 10, 15, 30, 60, 90, 120],
    defaultStepIndex: 4, // 15 seconds
    debugLabel: 'dashboard',
    storageKey: 'dashboard',
  });

  // Initial data load
  useEffect(() => {
    refreshDashboardData();
  }, [refreshDashboardData]);

  // Show setup wizard if any step is incomplete
  const showSetupWizard =
    setupStatus.isLoaded &&
    (!setupStatus.hasStreamSources || !setupStatus.hasEpgSources || !setupStatus.hasProxies);

  return (
    <div className="space-y-6">
      {/* Setup Wizard for first-time users */}
      {showSetupWizard && (
        <SetupWizard
          hasStreamSources={setupStatus.hasStreamSources}
          hasEpgSources={setupStatus.hasEpgSources}
          hasProxies={setupStatus.hasProxies}
        />
      )}

      {/* Header with description and refresh controls */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <p className="text-muted-foreground">
            Real-time system monitoring and relay process metrics
          </p>
        </div>
        {/* Refresh Controls */}
        <RefreshControl autoRefresh={autoRefresh} isLoading={isLoading} variant="compact" />
      </div>

      {/* Overview Cards - REAL DATA */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
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
        pollingInterval={autoRefresh.refreshInterval * 1000}
        className="w-full"
      />
    </div>
  );
}
