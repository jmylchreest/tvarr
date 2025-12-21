'use client';

import React from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { ClusterStats as ClusterStatsType } from '@/types/api';

interface ClusterStatsProps {
  stats: ClusterStatsType | null;
  isLoading?: boolean;
  /** Optional action element to show at the start of the bar */
  action?: React.ReactNode;
}

export function ClusterStats({ stats, isLoading, action }: ClusterStatsProps) {
  if (isLoading) {
    return (
      <Card>
        <CardContent className="py-3 px-4">
          <div className="animate-pulse flex items-center gap-6">
            {action && <div className="h-8 w-24 bg-muted rounded" />}
            <div className="h-4 w-32 bg-muted rounded" />
            <div className="h-4 w-24 bg-muted rounded" />
            <div className="flex-1 h-2 bg-muted rounded" />
          </div>
        </CardContent>
      </Card>
    );
  }

  const totalDaemons = stats?.total_daemons ?? 0;
  const activeDaemons = stats?.active_daemons ?? 0;
  const activeJobs = stats?.total_active_jobs ?? 0;
  const totalSessions = stats?.total_gpu_sessions ?? 0;
  const availableSessions = stats?.available_gpu_sessions ?? 0;
  const usedSessions = totalSessions - availableSessions;
  const sessionPercent = totalSessions > 0 ? (usedSessions / totalSessions) * 100 : 0;

  const getSessionColor = () => {
    if (sessionPercent >= 100) return 'bg-destructive';
    if (sessionPercent >= 80) return 'bg-warning';
    return 'bg-success';
  };

  return (
    <Card>
      <CardContent className="py-3 px-4">
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          {/* Action (e.g., Connect button) */}
          {action}

          {/* Transcoders */}
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">Transcoders</span>
            <span className="text-sm font-semibold">{activeDaemons}/{totalDaemons}</span>
          </div>

          {/* Active Jobs */}
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">Active Jobs</span>
            <span className={cn(
              "text-sm font-semibold",
              activeJobs > 0 && "text-info"
            )}>{activeJobs}</span>
          </div>

          {/* GPU Sessions - Progress Bar */}
          {totalSessions > 0 && (
            <div className="flex items-center gap-2 flex-1 min-w-[200px]">
              <span className="text-xs text-muted-foreground whitespace-nowrap">GPU Sessions</span>
              <div className="flex-1 flex items-center gap-2">
                <div className="flex-1 relative h-2 overflow-hidden rounded-full bg-secondary">
                  <div
                    className={cn('h-full transition-all', getSessionColor())}
                    style={{ width: `${Math.min(sessionPercent, 100)}%` }}
                  />
                </div>
                <span className="text-xs font-medium tabular-nums">{usedSessions}/{totalSessions}</span>
              </div>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
