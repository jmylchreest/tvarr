'use client';

import React, {
  createContext,
  useContext,
  useRef,
  useEffect,
  useCallback,
  useState,
  useMemo,
  ReactNode,
} from 'react';
import { sseManager } from '@/lib/sse-singleton';
import { ProgressEvent as APIProgressEvent } from '@/types/api';
import { Debug } from '@/utils/debug';
import { getBackendUrl } from '@/lib/config';

// Extend the API type for UI-specific functionality
export interface ProgressEvent extends APIProgressEvent {
  hasBeenSeen?: boolean;
}

export interface NotificationEvent extends ProgressEvent {
  hasBeenSeen: boolean;
  // Composite key for grouping: owner_id + operation_type
  groupKey?: string;
}

interface ProgressEventContext {
  // Subscribe to events for specific resource IDs
  subscribe: (resourceId: string, callback: (event: ProgressEvent) => void) => () => void;
  // Subscribe to all events of a specific operation type
  subscribeToType: (operationType: string, callback: (event: ProgressEvent) => void) => () => void;
  // Subscribe to all events (for notifications)
  subscribeToAll: (callback: (event: NotificationEvent) => void) => () => void;
  // Get current state for a resource
  getResourceState: (resourceId: string) => ProgressEvent | null;
  // Get all events with visibility tracking
  getAllEvents: () => NotificationEvent[];
  // Mark events as seen/acknowledged
  markAsSeen: (eventIds: string[]) => void;
  // Get unread count for operation type
  getUnreadCount: (operationType?: string) => number;
  // Connection status
  isConnected: boolean;
}

const ProgressContext = createContext<ProgressEventContext | null>(null);

// Storage key for persisting notification state
const NOTIFICATION_STORAGE_KEY = 'tvarr-notifications';

// Helper to serialize events for storage (remove functions, convert Map to Object)
const serializeEventsForStorage = (
  events: Map<string, NotificationEvent>
): Record<string, NotificationEvent> => {
  const serializable: Record<string, NotificationEvent> = {};
  for (const [id, event] of events) {
    serializable[id] = {
      ...event,
      // Ensure all required fields are present
      hasBeenSeen: event.hasBeenSeen || false,
    };
  }
  return serializable;
};

// Helper to deserialize events from storage
const deserializeEventsFromStorage = (): Map<string, NotificationEvent> => {
  try {
    const stored = localStorage.getItem(NOTIFICATION_STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as Record<string, NotificationEvent>;
      const eventsMap = new Map<string, NotificationEvent>();

      // Only keep events from the last 24 hours to prevent stale data buildup
      const now = new Date().getTime();
      const oneDayAgo = now - 24 * 60 * 60 * 1000;

      for (const [id, event] of Object.entries(parsed)) {
        const eventTime = new Date(event.last_update).getTime();
        if (eventTime > oneDayAgo) {
          eventsMap.set(id, {
            ...event,
            hasBeenSeen: event.hasBeenSeen || false,
          });
        }
      }

      return eventsMap;
    }
  } catch (error) {
    console.warn('Failed to deserialize notification state from localStorage:', error);
  }
  return new Map();
};

// Module-level logger to avoid recreating on each render
const providerDebug = Debug.createLogger('ProgressProvider');

export function ProgressProvider({ children }: { children: ReactNode }) {
  const [events, setEvents] = useState<Map<string, NotificationEvent>>(() => {
    // Initialize with persisted state from localStorage
    if (typeof window !== 'undefined') {
      return deserializeEventsFromStorage();
    }
    return new Map();
  });
  const [connected, setConnected] = useState(false);
  const notificationSubscribersRef = useRef<Set<{ current: (event: NotificationEvent) => void }>>(
    new Set()
  );
  const hasInitializedRef = useRef(false);
  // Track if SSE subscription is active to prevent duplicate subscriptions
  const sseSubscriptionRef = useRef<(() => void) | null>(null);

  // Clean up old localStorage entries on startup
  useEffect(() => {
    if (typeof window !== 'undefined') {
      // Clean up any very old entries that might still be in storage
      const now = new Date().getTime();
      const threeDaysAgo = now - 3 * 24 * 60 * 60 * 1000;

      try {
        const stored = localStorage.getItem(NOTIFICATION_STORAGE_KEY);
        if (stored) {
          const parsed = JSON.parse(stored) as Record<string, NotificationEvent>;
          let hasOldEntries = false;

          for (const event of Object.values(parsed)) {
            const eventTime = new Date(event.last_update).getTime();
            if (eventTime < threeDaysAgo) {
              hasOldEntries = true;
              break;
            }
          }

          if (hasOldEntries) {
            providerDebug.log('Cleaning up old notification entries from localStorage');
            localStorage.removeItem(NOTIFICATION_STORAGE_KEY);
          }
        }
      } catch (error) {
        providerDebug.warn('Error during localStorage cleanup, clearing all:', error);
        localStorage.removeItem(NOTIFICATION_STORAGE_KEY);
      }
    }
  }, []);

  // Persist events to localStorage whenever they change
  useEffect(() => {
    if (typeof window !== 'undefined' && events.size > 0) {
      try {
        const serialized = serializeEventsForStorage(events);
        localStorage.setItem(NOTIFICATION_STORAGE_KEY, JSON.stringify(serialized));
        providerDebug.log('Persisted notification state to localStorage:', events.size, 'events');
      } catch (error) {
        providerDebug.error('Failed to persist notification state to localStorage:', error);
      }
    }
  }, [events]);

  // Handle progress events (now with single operation ID per process)
  const handleProgressEvent = useCallback((event: ProgressEvent) => {
    providerDebug.log('Received event:', {
      id: event.id,
      owner_id: event.owner_id,
      owner_type: event.owner_type,
      operation_type: event.operation_type,
      operation_name: event.operation_name,
      state: event.state,
      current_stage: event.current_stage,
      overall_percentage: event.overall_percentage,
      stages: event.stages,
    });

    // Create notification event with acknowledgment tracking
    const notificationEvent: NotificationEvent = {
      ...event,
      hasBeenSeen: false,
    };

    providerDebug.log('Creating new notification event:', {
      id: event.id,
      operation_type: event.operation_type,
      state: event.state,
      hasBeenSeen: notificationEvent.hasBeenSeen,
    });

    // Update events map using event.id
    setEvents((prev) => {
      const newEvents = new Map(prev);
      const existingEvent = newEvents.get(event.id);

      // Acknowledgment tracking:
      // - New events: always start not seen/acknowledged
      // - Updated events: reset acknowledgment so user sees the update
      // - Dismissed events don't exist in map, so updates recreate them fresh
      if (existingEvent) {
        // Check if there's an actual change in the event
        const hasChanged =
          existingEvent.state !== event.state ||
          existingEvent.overall_percentage !== event.overall_percentage ||
          existingEvent.current_stage !== event.current_stage ||
          existingEvent.error !== event.error;

        if (hasChanged) {
          // Reset acknowledgment for any actual update - user needs to see the change
          notificationEvent.hasBeenSeen = false;
          providerDebug.log('Reset acknowledgment for updated event:', event.id);
        } else {
          // No change, preserve acknowledgment state (for SSE reconnect scenarios)
          notificationEvent.hasBeenSeen = existingEvent.hasBeenSeen;
        }
      }

      // Store or update the event
      newEvents.set(event.id, notificationEvent);

      // Clean up old events to match backend limit and localStorage size (keep max 50 total)
      // Reduced from 100 to 50 to keep localStorage manageable
      if (newEvents.size > 50) {
        // Sort all events by last_update (oldest first)
        const sortedEvents = Array.from(newEvents.entries()).sort(
          (a, b) => new Date(a[1].last_update).getTime() - new Date(b[1].last_update).getTime()
        );

        // Remove oldest events to stay under limit
        const toRemove = sortedEvents.slice(0, newEvents.size - 50);
        toRemove.forEach(([id]) => {
          newEvents.delete(id);
          providerDebug.log('Removed old event to stay under 50 limit:', id);
        });
      }

      return newEvents;
    });

    // Notify all notification subscribers
    for (const subscriberRef of notificationSubscribersRef.current) {
      subscriberRef.current(notificationEvent);
    }

    providerDebug.log('Event processed, stored in local events map, and sent to notification subscribers');
  }, []); // NO DEPENDENCIES - stable callback to prevent SSE reconnections

  // Use global SSE singleton - subscriber-driven connection lifecycle
  // The SSE singleton handles its own connection management based on subscriber count
  useEffect(() => {
    // Skip if already subscribed (handles React Strict Mode double-mount and navigation)
    if (sseSubscriptionRef.current) {
      providerDebug.log('ProgressProvider: SSE subscription already active, skipping');
      return;
    }

    providerDebug.log('ProgressProvider: Setting up SSE subscriptions');

    // Fetch initial state from REST endpoint (only once)
    const fetchInitialState = async () => {
      // Skip if already initialized (handles React Strict Mode double-mount)
      if (hasInitializedRef.current) {
        providerDebug.log('ProgressProvider: Already initialized, skipping initial fetch');
        return;
      }
      hasInitializedRef.current = true;

      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/progress/operations`);

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const data = await response.json();
        // API returns {operations: [...]} wrapper
        const activeOperations: NotificationEvent[] = data.operations || [];
        providerDebug.log('ProgressProvider: Fetched initial active operations:', activeOperations.length);

        // Clear localStorage and rebuild from server state
        const NOTIFICATION_STORAGE_KEY = 'tvarr-notifications';
        localStorage.removeItem(NOTIFICATION_STORAGE_KEY);

        // Convert to NotificationEvent format and update state
        const initialEvents = new Map<string, NotificationEvent>();
        activeOperations.forEach((event) => {
          initialEvents.set(event.id || event.owner_id, {
            ...event,
            hasBeenSeen: false, // New events start as unread
          });
        });

        setEvents(initialEvents);
        providerDebug.log('ProgressProvider: Initialized with', initialEvents.size, 'active operations');
      } catch (error) {
        providerDebug.error('ProgressProvider: Failed to fetch initial state:', error);
        // Clear localStorage anyway to prevent stale state
        const NOTIFICATION_STORAGE_KEY = 'tvarr-notifications';
        localStorage.removeItem(NOTIFICATION_STORAGE_KEY);
        setEvents(new Map());
      }
    };

    // Fetch initial state (doesn't depend on SSE)
    fetchInitialState();

    // Subscribe to all events from the singleton
    // This automatically triggers SSE connection when first subscriber joins
    const unsubscribeFromAll = sseManager.subscribeToAll((event) => {
      handleProgressEvent(event);
    });

    // Store the unsubscribe function so we can track if subscription is active
    sseSubscriptionRef.current = unsubscribeFromAll;

    // Subscribe to connection state changes (reactive instead of polling)
    const unsubscribeConnectionState = sseManager.subscribeToConnectionState((isConnected) => {
      providerDebug.log('ProgressProvider: SSE connection state changed:', isConnected);
      setConnected(isConnected);
    });

    // Cleanup on unmount
    // When we unsubscribe, the singleton will auto-disconnect if no other subscribers
    return () => {
      providerDebug.log('ProgressProvider: Cleaning up SSE subscriptions');
      unsubscribeFromAll();
      unsubscribeConnectionState();
      sseSubscriptionRef.current = null;
    };
  }, [handleProgressEvent]);

  // Subscribe to events for specific resource ID using global singleton
  const subscribe = useCallback((resourceId: string, callback: (event: ProgressEvent) => void) => {
    providerDebug.log(`Subscribing to resource via singleton: ${resourceId}`);

    // Use the global singleton manager for subscriptions
    return sseManager.subscribe(resourceId, callback);
  }, []);

  // Subscribe to events by operation type using global singleton
  const subscribeToType = useCallback(
    (operationType: string, callback: (event: ProgressEvent) => void) => {
      providerDebug.log(`Subscribing to operation type via singleton: ${operationType}`);

      // Use the global singleton manager for subscriptions
      return sseManager.subscribe(operationType, callback);
    },
    []
  );

  // Subscribe to all events (for notifications)
  const subscribeToAll = useCallback(
    (callback: (event: NotificationEvent) => void) => {
      providerDebug.log('Subscribing to all events (for notifications)');

      // Store the callback in a ref so we can call it from handleProgressEvent
      const callbackRef = { current: callback };

      // Add callback to the subscribers set
      notificationSubscribersRef.current.add(callbackRef);

      // Send existing events to new subscriber
      for (const event of events.values()) {
        callback(event);
      }

      return () => {
        providerDebug.log('Unsubscribing from all events (notifications)');
        notificationSubscribersRef.current.delete(callbackRef);
      };
    },
    [events]
  );

  // Get current state for a resource
  const getResourceState = useCallback(
    (resourceId: string): ProgressEvent | null => {
      // Check if any event is for this resource ID using owner_id
      for (const event of events.values()) {
        if (event.owner_id === resourceId) {
          return event;
        }
      }
      return null;
    },
    [events]
  );

  // Get all events with visibility tracking
  // Note: NotificationBell expects unfiltered events to show cross-page notifications
  const getAllEvents = useCallback(() => {
    const allEvents = Array.from(events.values());
    return allEvents.sort(
      (a, b) => new Date(b.last_update).getTime() - new Date(a.last_update).getTime()
    );
  }, [events]);

  // Mark events as seen/acknowledged
  const markAsSeen = useCallback((eventIds: string[]) => {
    providerDebug.log('markAsSeen called with eventIds:', eventIds);

    setEvents((prev) => {
      const newEvents = new Map(prev);
      let hasChanges = false;

      eventIds.forEach((eventId) => {
        const event = newEvents.get(eventId);
        if (event && !event.hasBeenSeen) {
          newEvents.set(eventId, { ...event, hasBeenSeen: true });
          hasChanges = true;
          providerDebug.log(`Marked event ${eventId} as seen/acknowledged`);
        }
      });

      return hasChanges ? newEvents : prev;
    });
  }, []);

  // Dismiss functionality removed for simplicity

  // Get unseen/unacknowledged count
  const getUnreadCount = useCallback(
    (operationType?: string) => {
      let count = 0;
      for (const event of events.values()) {
        // Count events that are unseen (dismissed events are deleted from map)
        if (!event.hasBeenSeen) {
          if (!operationType || event.operation_type === operationType) {
            count++;
          }
        }
      }
      return count;
    },
    [events]
  );

  const contextValue: ProgressEventContext = useMemo(
    () => ({
      subscribe,
      subscribeToType,
      subscribeToAll,
      getResourceState,
      getAllEvents,
      markAsSeen,
      getUnreadCount,
      isConnected: connected,
    }),
    [subscribe, subscribeToType, subscribeToAll, getResourceState, getAllEvents, markAsSeen, getUnreadCount, connected]
  );

  return <ProgressContext.Provider value={contextValue}>{children}</ProgressContext.Provider>;
}

export function useProgressContext() {
  const context = useContext(ProgressContext);
  if (!context) {
    throw new Error('useProgressContext must be used within a ProgressProvider');
  }
  return context;
}
