'use client';

import { useState, useEffect } from 'react';
import { useProgressContext, ProgressEvent } from '@/providers/ProgressProvider';

/**
 * Subscribe to progress events for a specific resource ID
 * @param resourceId - The resource ID to listen for (source ID, proxy ID, etc.)
 * @returns The latest progress event for this resource, or null if none
 */
export function useProgressEvents(resourceId: string): ProgressEvent | null {
  const context = useProgressContext();
  const [event, setEvent] = useState<ProgressEvent | null>(null);

  useEffect(() => {
    if (!resourceId) return;

    // Get initial state
    const initialState = context.getResourceState(resourceId);
    setEvent(initialState);

    // Subscribe to updates
    const unsubscribe = context.subscribe(resourceId, (newEvent) => {
      setEvent(newEvent);
    });

    return unsubscribe;
  }, [resourceId]);

  return event;
}

/**
 * Subscribe to progress events by operation type
 * @param operationType - The operation type to listen for (e.g., 'stream_ingestion', 'epg_ingestion', 'proxy_regeneration')
 * @returns Array of latest events for this operation type
 */
export function useProgressEventsByType(operationType: string): ProgressEvent[] {
  const context = useProgressContext();
  const [events, setEvents] = useState<ProgressEvent[]>([]);

  useEffect(() => {
    if (!operationType) return;

    const eventMap = new Map<string, ProgressEvent>();

    const unsubscribe = context.subscribeToType(operationType, (event) => {
      eventMap.set(event.id, event);
      setEvents(
        Array.from(eventMap.values()).sort(
          (a, b) => new Date(b.last_update).getTime() - new Date(a.last_update).getTime()
        )
      );
    });

    return unsubscribe;
  }, [operationType]);

  return events;
}
