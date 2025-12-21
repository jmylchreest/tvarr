'use client';

import { useState, useEffect, useCallback } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import {
  Users,
  Zap,
} from 'lucide-react';
import { RefreshControl } from '@/components/ui/refresh-control';
import { useAutoRefresh } from '@/hooks/use-auto-refresh';
import { ClusterStats as ClusterStatsType } from '@/types/api';
import { apiClient } from '@/lib/api-client';
import { RelayFlowDiagram, FlowMetadata } from '@/components/relay';
import { SetupWizard } from '@/components/SetupWizard';

export function Dashboard() {
  const [isLoading, setIsLoading] = useState(false);
  const [flowMetadata, setFlowMetadata] = useState<FlowMetadata | null>(null);
  const [clusterStats, setClusterStats] = useState<ClusterStatsType | null>(null);

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
      // Fetch setup status and cluster stats in parallel
      const [streamSources, epgSources, proxies, cluster] = await Promise.all([
        apiClient.getStreamSources().catch(() => ({ items: [] })),
        apiClient.getEpgSources().catch(() => ({ items: [] })),
        apiClient.getProxies().catch(() => ({ items: [] })),
        apiClient.getClusterStats().catch(() => null),
      ]);

      setClusterStats(cluster);

      // Update setup status
      setSetupStatus({
        hasStreamSources: (streamSources.items?.length ?? 0) > 0,
        hasEpgSources: (epgSources.items?.length ?? 0) > 0,
        hasProxies: (proxies.items?.length ?? 0) > 0,
        isLoaded: true,
      });
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
    <div className="flex flex-col h-full gap-4">
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

      {/* Overview Cards - Compact */}
      <div className="grid gap-3 grid-cols-2">
        <Card className="py-3">
          <CardContent className="px-4 py-0">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground">Connected Clients</p>
                <p className="text-2xl font-bold">
                  {flowMetadata ? flowMetadata.totalClients : '–'}
                </p>
              </div>
              <Users className="h-5 w-5 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>

        <Card className="py-3">
          <CardContent className="px-4 py-0">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground">Active Transcode Jobs</p>
                <p className="text-2xl font-bold">
                  {clusterStats ? clusterStats.total_active_jobs : '–'}
                </p>
              </div>
              <Zap className="h-5 w-5 text-muted-foreground" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Relay Flow Visualization */}
      <RelayFlowDiagram
        pollingInterval={autoRefresh.refreshInterval * 1000}
        className="flex-1 min-h-0"
        onMetadataUpdate={setFlowMetadata}
      />
    </div>
  );
}
