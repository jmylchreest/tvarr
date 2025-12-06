'use client';

import React, { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { EnhancedCircuitBreakerStats, calculateSuccessRate } from '@/types/circuit-breaker';
import { SegmentedProgressBar } from './SegmentedProgressBar';
import { StateIndicator } from './StateIndicator';
import { StateTimeline } from './StateTimeline';
import { StateDurationSummary } from './StateDurationSummary';
import { Activity, ChevronDown, ChevronUp, RotateCcw, Settings2 } from 'lucide-react';
import { getBackendUrl } from '@/lib/config';

interface CircuitBreakerCardProps {
  stats: EnhancedCircuitBreakerStats;
  onReset?: () => void;
  onConfigure?: () => void;
  showActions?: boolean;
  expanded?: boolean;
  className?: string;
}

export function CircuitBreakerCard({
  stats,
  onReset,
  onConfigure,
  showActions = true,
  expanded: initialExpanded = false,
  className = '',
}: CircuitBreakerCardProps) {
  const [isExpanded, setIsExpanded] = useState(initialExpanded);
  const [isResetting, setIsResetting] = useState(false);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const backendUrl = getBackendUrl();

  const successRate = calculateSuccessRate(stats);

  const handleResetClick = () => {
    // Show confirmation for non-closed states
    if (stats.state !== 'closed') {
      setShowResetConfirm(true);
    } else {
      // For closed state, just reset (clears counters)
      handleReset();
    }
  };

  const handleResetConfirm = () => {
    setShowResetConfirm(false);
    handleReset();
  };

  const handleReset = async () => {
    if (onReset) {
      onReset();
      return;
    }

    setIsResetting(true);
    try {
      const response = await fetch(`${backendUrl}/api/v1/circuit-breakers/${stats.name}/reset`, {
        method: 'POST',
      });
      if (!response.ok) {
        console.error('Failed to reset circuit breaker');
      }
    } catch (error) {
      console.error('Error resetting circuit breaker:', error);
    } finally {
      setIsResetting(false);
    }
  };

  return (
    <Card className={`${className}`}>
      <Collapsible open={isExpanded} onOpenChange={setIsExpanded}>
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-muted-foreground" />
              <CardTitle className="text-sm font-medium">{stats.name}</CardTitle>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="outline" className="text-xs">
                {successRate.toFixed(1)}% healthy
              </Badge>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
                  {isExpanded ? (
                    <ChevronUp className="h-4 w-4" />
                  ) : (
                    <ChevronDown className="h-4 w-4" />
                  )}
                </Button>
              </CollapsibleTrigger>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-3">
          {/* State indicator - always visible */}
          <StateIndicator
            state={stats.state}
            stateEnteredAt={stats.state_entered_at}
            resetTimeout={stats.config.reset_timeout}
            consecutiveFailures={stats.consecutive_failures}
            failureThreshold={stats.config.failure_threshold}
          />

          {/* Segmented progress bar - always visible */}
          <SegmentedProgressBar
            errorCounts={stats.error_counts}
            totalRequests={stats.total_requests}
          />

          {/* Expanded content */}
          <CollapsibleContent className="space-y-4 pt-2">
            {/* Stats grid */}
            <div className="grid grid-cols-2 gap-2 text-xs">
              <div className="bg-muted/30 rounded p-2">
                <div className="text-muted-foreground">Total Requests</div>
                <div className="font-medium">{stats.total_requests.toLocaleString()}</div>
              </div>
              <div className="bg-muted/30 rounded p-2">
                <div className="text-muted-foreground">Failures</div>
                <div className="font-medium text-red-600">
                  {stats.total_failures.toLocaleString()}
                </div>
              </div>
              <div className="bg-muted/30 rounded p-2">
                <div className="text-muted-foreground">Consecutive Fails</div>
                <div className="font-medium">
                  {stats.consecutive_failures} / {stats.config.failure_threshold}
                </div>
              </div>
              <div className="bg-muted/30 rounded p-2">
                <div className="text-muted-foreground">Half-Open Max</div>
                <div className="font-medium">{stats.config.half_open_max}</div>
              </div>
            </div>

            {/* State duration summary */}
            <StateDurationSummary durations={stats.state_durations} />

            {/* State timeline */}
            <StateTimeline transitions={stats.recent_transitions} maxItems={5} />

            {/* Actions */}
            {showActions && (
              <div className="flex flex-col gap-2 pt-2 border-t">
                {/* Reset confirmation dialog */}
                {showResetConfirm && (
                  <div className="bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800 rounded p-2 text-xs">
                    <p className="font-medium text-amber-800 dark:text-amber-200 mb-2">
                      Reset circuit breaker "{stats.name}"?
                    </p>
                    <p className="text-amber-700 dark:text-amber-300 mb-2">
                      This will force the circuit to closed state, allowing requests through immediately.
                    </p>
                    <div className="flex gap-2">
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={handleResetConfirm}
                        disabled={isResetting}
                        className="flex-1 h-6 text-xs"
                      >
                        {isResetting ? (
                          <RotateCcw className="h-3 w-3 mr-1 animate-spin" />
                        ) : (
                          'Confirm Reset'
                        )}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setShowResetConfirm(false)}
                        className="flex-1 h-6 text-xs"
                      >
                        Cancel
                      </Button>
                    </div>
                  </div>
                )}

                {/* Action buttons */}
                {!showResetConfirm && (
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleResetClick}
                      disabled={isResetting}
                      className="flex-1"
                    >
                      <RotateCcw className={`h-3 w-3 mr-1 ${isResetting ? 'animate-spin' : ''}`} />
                      Reset
                    </Button>
                    {onConfigure && (
                      <Button variant="outline" size="sm" onClick={onConfigure} className="flex-1">
                        <Settings2 className="h-3 w-3 mr-1" />
                        Configure
                      </Button>
                    )}
                  </div>
                )}
              </div>
            )}
          </CollapsibleContent>
        </CardContent>
      </Collapsible>
    </Card>
  );
}

// Compact card variant for grid display
export function CircuitBreakerCardCompact({
  stats,
  onClick,
  className = '',
}: {
  stats: EnhancedCircuitBreakerStats;
  onClick?: () => void;
  className?: string;
}) {
  return (
    <Card
      className={`cursor-pointer hover:border-primary/50 transition-colors ${className}`}
      onClick={onClick}
    >
      <CardContent className="p-3 space-y-2">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium truncate">{stats.name}</span>
          <Badge
            variant="outline"
            className={`text-xs ${
              stats.state === 'closed'
                ? 'border-green-500 text-green-600'
                : stats.state === 'open'
                  ? 'border-red-500 text-red-600'
                  : 'border-amber-500 text-amber-600'
            }`}
          >
            {stats.state}
          </Badge>
        </div>
        <SegmentedProgressBar
          errorCounts={stats.error_counts}
          totalRequests={stats.total_requests}
        />
      </CardContent>
    </Card>
  );
}
