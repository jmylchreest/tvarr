'use client';

import React from 'react';
import { StateTransition, CircuitState, getStateColorClasses } from '@/types/circuit-breaker';
import { ArrowRight, Clock } from 'lucide-react';

interface StateTimelineProps {
  transitions: StateTransition[];
  maxItems?: number;
  className?: string;
}

export function StateTimeline({ transitions, maxItems = 5, className = '' }: StateTimelineProps) {
  if (!transitions || transitions.length === 0) {
    return (
      <div className={`text-xs text-muted-foreground text-center py-2 ${className}`}>
        No state transitions recorded
      </div>
    );
  }

  const displayedTransitions = transitions.slice(0, maxItems);
  const hasMore = transitions.length > maxItems;

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMins / 60);
    const diffDays = Math.floor(diffHours / 24);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString();
  };

  const getStateBadgeClass = (state: CircuitState) => {
    const base = 'px-1.5 py-0.5 rounded text-xs font-medium';
    return `${base} ${getStateColorClasses(state)}`;
  };

  return (
    <div className={`space-y-2 ${className}`}>
      <div className="text-xs font-medium text-muted-foreground flex items-center gap-1">
        <Clock className="h-3 w-3" />
        Recent Transitions
      </div>
      <div className="space-y-1.5">
        {displayedTransitions.map((transition, index) => (
          <div
            key={`${transition.timestamp}-${index}`}
            className="flex items-center gap-2 text-xs bg-muted/30 rounded px-2 py-1.5"
          >
            <span className={getStateBadgeClass(transition.from_state)}>
              {transition.from_state}
            </span>
            <ArrowRight className="h-3 w-3 text-muted-foreground" />
            <span className={getStateBadgeClass(transition.to_state)}>{transition.to_state}</span>
            <span className="text-muted-foreground ml-auto">{formatTime(transition.timestamp)}</span>
          </div>
        ))}
        {hasMore && (
          <div className="text-xs text-muted-foreground text-center">
            +{transitions.length - maxItems} more transitions
          </div>
        )}
      </div>
    </div>
  );
}

// Compact version for inline display
export function StateTimelineCompact({
  transitions,
  className = '',
}: {
  transitions: StateTransition[];
  className?: string;
}) {
  const lastTransition = transitions[0];

  if (!lastTransition) {
    return (
      <span className={`text-xs text-muted-foreground ${className}`}>No transitions recorded</span>
    );
  }

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  return (
    <span className={`text-xs ${className}`}>
      Last: {lastTransition.from_state} → {lastTransition.to_state} at{' '}
      {formatTime(lastTransition.timestamp)}
      {lastTransition.reason && (
        <span className="text-muted-foreground"> ({lastTransition.reason})</span>
      )}
    </span>
  );
}
