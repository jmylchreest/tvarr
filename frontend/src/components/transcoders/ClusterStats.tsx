'use client';

import {
  Server,
  Cpu,
  MemoryStick,
  Activity,
  Cog,
} from 'lucide-react';
import { StatCard } from '@/components/shared/feedback/StatCard';
import { ClusterStats as ClusterStatsType } from '@/types/api';

interface ClusterStatsProps {
  stats: ClusterStatsType | null;
  isLoading?: boolean;
}

export function ClusterStats({ stats, isLoading }: ClusterStatsProps) {
  if (isLoading) {
    return (
      <div className="grid gap-2 grid-cols-2 md:grid-cols-3 lg:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="animate-pulse rounded-lg border bg-card p-3">
            <div className="flex items-center justify-between">
              <div className="flex-1">
                <div className="h-3 w-20 bg-muted rounded mb-1" />
                <div className="h-5 w-12 bg-muted rounded" />
              </div>
              <div className="h-4 w-4 bg-muted rounded" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="grid gap-2 grid-cols-2 md:grid-cols-3 lg:grid-cols-5">
      <StatCard
        title="Transcoders"
        value={`${stats?.active_daemons ?? 0}/${stats?.total_daemons ?? 0}`}
        icon={<Server className="h-4 w-4 text-blue-600 dark:text-blue-400" />}
      />
      <StatCard
        title="Active Jobs"
        value={stats?.total_active_jobs ?? 0}
        icon={<Activity className="h-4 w-4 text-green-600 dark:text-green-400" />}
      />
      <StatCard
        title="GPU Sessions"
        value={`${stats?.available_gpu_sessions ?? 0}/${stats?.total_gpu_sessions ?? 0}`}
        icon={<Cog className="h-4 w-4 text-purple-600 dark:text-purple-400" />}
      />
      <StatCard
        title="Avg CPU"
        value={`${(stats?.average_cpu_percent ?? 0).toFixed(0)}%`}
        icon={<Cpu className="h-4 w-4 text-orange-600 dark:text-orange-400" />}
      />
      <StatCard
        title="Avg Memory"
        value={`${(stats?.average_memory_percent ?? 0).toFixed(0)}%`}
        icon={<MemoryStick className="h-4 w-4 text-cyan-600 dark:text-cyan-400" />}
      />
    </div>
  );
}
