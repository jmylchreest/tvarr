'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Server, MonitorCheck } from 'lucide-react';
import { ClusterStats } from '@/components/transcoders/ClusterStats';
import { ConnectTranscoderButton } from '@/components/transcoders/DaemonList';
import { TranscoderDetailPanel } from '@/components/transcoders/TranscoderDetailPanel';
import { RefreshControl } from '@/components/ui/refresh-control';
import { MasterDetailLayout, MasterItem, MasterItemStatus, DetailEmpty } from '@/components/shared/layouts/MasterDetailLayout';
import { BadgeGroup, BadgeItem, BadgePriority } from '@/components/shared/BadgeGroup';
import { apiClient } from '@/lib/api-client';
import { Daemon, ClusterStats as ClusterStatsType, DaemonState } from '@/types/api';
import { Debug } from '@/utils/debug';
import { createFuzzyMatcher } from '@/lib/fuzzy-search';
import { useAutoRefresh } from '@/hooks/use-auto-refresh';

// Map daemon state to badge priority
function getStatePriority(state: DaemonState): BadgePriority {
  switch (state) {
    case 'connected':
      return 'secondary'; // Dark gray like "idle" states
    case 'transcoding':
      return 'info'; // Blue for active transcoding
    case 'draining':
      return 'warning';
    case 'unhealthy':
      return 'error';
    case 'disconnected':
      return 'error'; // Red for disconnected
    case 'connecting':
      return 'info';
    default:
      return 'default';
  }
}

// Map daemon state to icon color using semantic theme colors
function getStateColor(state: DaemonState): string {
  switch (state) {
    case 'connected':
      return 'text-muted-foreground';
    case 'transcoding':
      return 'text-info';
    case 'draining':
      return 'text-warning';
    case 'unhealthy':
      return 'text-destructive';
    case 'disconnected':
      return 'text-destructive';
    case 'connecting':
      return 'text-info';
    default:
      return 'text-muted-foreground';
  }
}

// Map daemon state to collapsed item status
function getItemStatus(daemon: Daemon): MasterItemStatus {
  // Map state to status
  switch (daemon.state) {
    case 'connected':
      return 'default';
    case 'transcoding':
      return 'active';
    case 'draining':
      return 'warning';
    case 'unhealthy':
    case 'disconnected':
      return 'error';
    case 'connecting':
      return 'active';
    default:
      return 'default';
  }
}

// Extended MasterItem with daemon reference
interface DaemonMasterItem extends MasterItem {
  daemon: Daemon;
}

export function Transcoders() {
  const [daemons, setDaemons] = useState<Daemon[]>([]);
  const [clusterStats, setClusterStats] = useState<ClusterStatsType | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [selectedDaemon, setSelectedDaemon] = useState<DaemonMasterItem | null>(null);

  const refreshData = useCallback(async () => {
    try {
      const [daemonsResponse, statsResponse] = await Promise.all([
        apiClient.listTranscoders(),
        apiClient.getClusterStats(),
      ]);
      setDaemons(daemonsResponse.daemons);
      setClusterStats(statsResponse);

      // Update selected daemon if it still exists (preserve selection across refreshes)
      if (selectedDaemon) {
        const updatedDaemon = daemonsResponse.daemons.find(d => d.id === selectedDaemon.id);
        if (updatedDaemon) {
          setSelectedDaemon(daemonToMasterItem(updatedDaemon));
        }
      }
    } catch (error) {
      Debug.error('Failed to fetch transcoder data:', error);
    } finally {
      setIsLoading(false);
    }
  }, [selectedDaemon]);

  // Auto-refresh using the shared hook
  const autoRefresh = useAutoRefresh({
    onRefresh: refreshData,
    debugLabel: 'transcoders',
    storageKey: 'transcoders',
  });

  // Initial load
  useEffect(() => {
    refreshData();
  }, []);

  const handleDrain = async (id: string) => {
    try {
      const result = await apiClient.drainTranscoder(id);
      Debug.log('Daemon draining:', result.message);
      await refreshData();
    } catch (error) {
      Debug.error('Failed to drain daemon:', error);
    }
  };

  const handleActivate = async (id: string) => {
    try {
      const result = await apiClient.activateTranscoder(id);
      Debug.log('Daemon activated:', result.message);
      await refreshData();
    } catch (error) {
      Debug.error('Failed to activate daemon:', error);
    }
  };

  // Convert daemon to master item
  const daemonToMasterItem = (daemon: Daemon): DaemonMasterItem => {
    const stateBadges: BadgeItem[] = [
      { label: daemon.state, priority: getStatePriority(daemon.state) },
    ];

    // Add active jobs badge if there are jobs
    if (daemon.active_jobs > 0) {
      stateBadges.push({
        label: `${daemon.active_jobs} job${daemon.active_jobs > 1 ? 's' : ''}`,
        priority: 'info'
      });
    }

    // Animate when in transcoding state
    const isTranscoding = daemon.state === 'transcoding';

    return {
      id: daemon.id,
      title: daemon.name,
      subtitle: daemon.address,
      icon: <Server className={`h-4 w-4 ${getStateColor(daemon.state)}`} />,
      badge: (
        <BadgeGroup
          badges={stateBadges}
          animate={isTranscoding ? 'sparkle' : 'none'}
        />
      ),
      // Status and animate for collapsed view
      status: getItemStatus(daemon),
      animate: isTranscoding,
      daemon,
    };
  };

  // Fuzzy search filter
  const fuzzyMatcher = useMemo(
    () =>
      createFuzzyMatcher<DaemonMasterItem>({
        keys: [
          { name: 'title', weight: 0.35 },
          { name: 'subtitle', weight: 0.2 },
          { name: 'state', weight: 0.15 },
          { name: 'version', weight: 0.1 },
          { name: 'encoders', weight: 0.2 },
        ],
        accessor: (item) => ({
          title: item.title,
          subtitle: item.subtitle ?? '',
          state: item.daemon.state,
          version: item.daemon.version,
          encoders: [
            ...(item.daemon.capabilities?.video_encoders ?? []),
            ...(item.daemon.capabilities?.audio_encoders ?? []),
          ].join(' '),
        }),
      }),
    []
  );

  // Convert daemons to master items and sort alphanumerically by name
  const masterItems = useMemo(() =>
    daemons
      .slice() // Create copy to avoid mutating original
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }))
      .map(daemonToMasterItem),
    [daemons]
  );

  // Custom filter function for MasterDetailLayout
  const filterFn = useCallback(
    (item: DaemonMasterItem, searchTerm: string): boolean => {
      const results = fuzzyMatcher([item], searchTerm);
      return results.length > 0;
    },
    [fuzzyMatcher]
  );

  const handleSelect = (item: DaemonMasterItem | null) => {
    setSelectedDaemon(item);
  };

  return (
    <div className="flex flex-col h-full gap-4">
      {/* Cluster Stats */}
      <ClusterStats stats={clusterStats} isLoading={isLoading} />

      {/* Controls Bar */}
      <Card>
        <CardContent className="p-3">
          <div className="flex items-center justify-between gap-3">
            {/* Connect Transcoder */}
            <ConnectTranscoderButton />

            {/* Spacer */}
            <div className="flex-1" />

            {/* Refresh Controls */}
            <RefreshControl autoRefresh={autoRefresh} isLoading={isLoading} variant="compact" />
          </div>
        </CardContent>
      </Card>

      {/* Master/Detail Layout */}
      <Card className="flex-1 min-h-0 overflow-hidden">
        <MasterDetailLayout
            items={masterItems}
            selectedId={selectedDaemon?.id}
            onSelect={handleSelect}
            isLoading={isLoading}
            title="Transcoders"
            searchPlaceholder="Search transcoders..."
            masterWidth={340}
            collapsible={true}
            defaultCollapsed={true}
            filterFn={filterFn}
            emptyState={{
              title: 'No transcoders connected',
              description: 'Connect a transcoder daemon to begin',
            }}
          >
            {(item) =>
              item ? (
                <TranscoderDetailPanel
                  daemon={item.daemon}
                  onDrain={handleDrain}
                  onActivate={handleActivate}
                />
              ) : (
                <DetailEmpty
                  title="Select a transcoder"
                  description="Choose a transcoder from the list to view details"
                  icon={<MonitorCheck className="h-12 w-12 text-muted-foreground/50" />}
                />
              )
            }
          </MasterDetailLayout>
      </Card>
    </div>
  );
}
