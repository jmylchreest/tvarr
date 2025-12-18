'use client';

import { useState, useEffect, useRef } from 'react';
import { useProgressContext, ProgressEvent, NotificationEvent } from '@/providers/ProgressProvider';
import { BadgeGroup, BadgeGroupProps, BadgeAnimation } from './BadgeGroup';

interface AnimatedBadgeGroupProps extends Omit<BadgeGroupProps, 'animate'> {
  /** Resource ID to monitor for processing state */
  resourceId: string;
  /** Animation to use when processing. Defaults to 'sparkle' */
  processingAnimation?: BadgeAnimation;
}

const ACTIVE_STATES = ['processing', 'preparing', 'connecting', 'downloading', 'saving', 'cleanup'];

/**
 * AnimatedBadgeGroup - A BadgeGroup that automatically animates when the resource is processing.
 *
 * This component subscribes to progress events for the given resourceId and applies
 * the animation when an operation is in progress.
 *
 * Usage:
 * ```tsx
 * <AnimatedBadgeGroup
 *   resourceId={source.id}
 *   badges={[
 *     { label: 'M3U', priority: 'info' },
 *     { label: 'Idle', priority: 'secondary' },
 *   ]}
 * />
 * ```
 */
export function AnimatedBadgeGroup({
  resourceId,
  processingAnimation = 'sparkle',
  ...props
}: AnimatedBadgeGroupProps) {
  const progressContext = useProgressContext();
  const [isProcessing, setIsProcessing] = useState(false);
  // Use ref to store resourceId for stable callback
  const resourceIdRef = useRef(resourceId);
  resourceIdRef.current = resourceId;

  useEffect(() => {
    // Check initial state from all events
    const allEvents = progressContext.getAllEvents();
    const resourceEvent = allEvents.find(e => e.owner_id === resourceId);
    if (resourceEvent) {
      setIsProcessing(ACTIVE_STATES.includes(resourceEvent.state));
    }

    // Subscribe to ALL progress events and filter locally
    // This is more reliable than subscribing by resourceId because some events
    // may have owner_id set differently than expected (e.g., proxy regeneration)
    const handleProgressEvent = (event: NotificationEvent) => {
      if (event.owner_id === resourceIdRef.current) {
        setIsProcessing(ACTIVE_STATES.includes(event.state));
      }
    };

    const unsubscribe = progressContext.subscribeToAll(handleProgressEvent);
    return unsubscribe;
  }, [progressContext, resourceId]);

  return (
    <BadgeGroup
      {...props}
      animate={isProcessing ? processingAnimation : 'none'}
    />
  );
}

export default AnimatedBadgeGroup;
