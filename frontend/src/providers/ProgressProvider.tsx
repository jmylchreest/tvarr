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
import { usePathname } from 'next/navigation';
import { sseManager, ProgressEvent as SSEProgressEvent } from '@/lib/sse-singleton';
import { ProgressEvent as APIProgressEvent, ProgressStage } from '@/types/api';
import { Debug } from '@/utils/debug';
import { useBackendConnectivity } from '@/providers/backend-connectivity-provider';
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

export function ProgressProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [events, setEvents] = useState<Map<string, NotificationEvent>>(() => {
    // Initialize with persisted state from localStorage
    if (typeof window !== 'undefined') {
      return deserializeEventsFromStorage();
    }
    return new Map();
  });
  const [connected, setConnected] = useState(false);
  const debug = Debug.createLogger('ProgressProvider');
  const notificationSubscribersRef = useRef<Set<{ current: (event: NotificationEvent) => void }>>(
    new Set()
  );
  const { isConnected: backendConnected } = useBackendConnectivity();

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
            debug.log('Cleaning up old notification entries from localStorage');
            localStorage.removeItem(NOTIFICATION_STORAGE_KEY);
          }
        }
      } catch (error) {
        debug.warn('Error during localStorage cleanup, clearing all:', error);
        localStorage.removeItem(NOTIFICATION_STORAGE_KEY);
      }
    }
  }, [debug]);

  // Local filtering logic based on current page
  const getOperationTypeFilter = useCallback((path: string): string | null => {
    // Normalize path to handle both with and without trailing slashes
    const normalizedPath = path.endsWith('/') ? path : path + '/';

    switch (normalizedPath) {
      case '/sources/stream/':
        return 'stream_ingestion';
      case '/sources/epg/':
        return 'epg_ingestion';
      case '/proxies/':
        return 'proxy_regeneration';
      default:
        return null; // No filter for events page and other pages
    }
  }, []);

  const shouldIncludeCompleted = useCallback((path: string): boolean => {
    return path === '/events' || path === '/events/';
  }, []);

  // Local event filtering based on current page context
  const filterEventForCurrentPage = useCallback(
    (event: ProgressEvent, currentPath: string): boolean => {
      const operationTypeFilter = getOperationTypeFilter(currentPath);
      const includeCompleted = shouldIncludeCompleted(currentPath);

      // Operation type filtering
      if (operationTypeFilter && event.operation_type !== operationTypeFilter) {
        return false;
      }

      // Completion filtering - on most pages we don't want to show completed events cluttering the UI
      if (!includeCompleted && (event.state === 'completed' || event.state === 'error')) {
        return false;
      }

      return true;
    },
    [getOperationTypeFilter, shouldIncludeCompleted]
  );

  // Persist events to localStorage whenever they change
  useEffect(() => {
    if (typeof window !== 'undefined' && events.size > 0) {
      try {
        const serialized = serializeEventsForStorage(events);
        localStorage.setItem(NOTIFICATION_STORAGE_KEY, JSON.stringify(serialized));
        debug.log('Persisted notification state to localStorage:', events.size, 'events');
      } catch (error) {
        debug.error('Failed to persist notification state to localStorage:', error);
      }
    }
  }, [events, debug]);

  // Handle progress events (now with single operation ID per process)
  const handleProgressEvent = useCallback((event: ProgressEvent) => {
    debug.log('Received event:', {
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

    debug.log('🔔 Creating new notification event:', {
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
          debug.log('Reset acknowledgment for updated event:', event.id);
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
          debug.log('Removed old event to stay under 50 limit:', id);
        });
      }

      return newEvents;
    });

    // Notify all notification subscribers
    for (const subscriberRef of notificationSubscribersRef.current) {
      subscriberRef.current(notificationEvent);
    }

    debug.log('Event processed, stored in local events map, and sent to notification subscribers');
  }, []); // NO DEPENDENCIES - stable callback to prevent SSE reconnections

  // Use global SSE singleton - no more per-component connections
  useEffect(() => {
    // Don't attempt SSE connection if backend is not available
    if (!backendConnected) {
      debug.log('ProgressProvider: Backend not connected, destroying SSE connection');
      setConnected(false);
      // Destroy any existing SSE connection when backend goes down
      sseManager.destroy();
      return;
    }

    debug.log('ProgressProvider: Backend connected, setting up SSE singleton');

    // Only reset and connect if backend is actually connected
    // Reset the SSE singleton to allow reconnection (in case backend came back up)
    sseManager.reset();

    // First, fetch initial state from REST endpoint
    const fetchInitialState = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/progress/operations`);

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const activeOperations: NotificationEvent[] = await response.json();
        debug.log('ProgressProvider: Fetched initial active operations:', activeOperations.length);

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
        debug.log('ProgressProvider: Initialized with', initialEvents.size, 'active operations');
      } catch (error) {
        debug.error('ProgressProvider: Failed to fetch initial state:', error);
        // Clear localStorage anyway to prevent stale state
        const NOTIFICATION_STORAGE_KEY = 'tvarr-notifications';
        localStorage.removeItem(NOTIFICATION_STORAGE_KEY);
        setEvents(new Map());
      }
    };

    // Fetch initial state first, then establish SSE connection
    fetchInitialState()
      .then(() => {
        return sseManager.ensureConnection();
      })
      .then(() => {
        debug.log('ProgressProvider: SSE connection established for real-time updates');
      })
      .catch((err) => {
        debug.error('ProgressProvider: Failed to establish SSE connection', err);
      });

    // Subscribe to all events from the singleton
    const unsubscribeFromAll = sseManager.subscribeToAll((event) => {
      handleProgressEvent(event);
    });

    // Monitor connection status
    const checkConnectionStatus = () => {
      setConnected(sseManager.isConnected());
    };

    // Initial connection status check
    checkConnectionStatus();

    // Poll connection status periodically
    const statusInterval = setInterval(checkConnectionStatus, 1000);

    // Cleanup on unmount or when backend disconnects
    return () => {
      debug.log('ProgressProvider: Cleaning up SSE singleton subscriptions');
      unsubscribeFromAll();
      clearInterval(statusInterval);
    };
  }, [handleProgressEvent, backendConnected]);

  // Subscribe to events for specific resource ID using global singleton
  const subscribe = useCallback((resourceId: string, callback: (event: ProgressEvent) => void) => {
    debug.log(`Subscribing to resource via singleton: ${resourceId}`);

    // Use the global singleton manager for subscriptions
    return sseManager.subscribe(resourceId, callback);
  }, []);

  // Subscribe to events by operation type using global singleton
  const subscribeToType = useCallback(
    (operationType: string, callback: (event: ProgressEvent) => void) => {
      debug.log(`Subscribing to operation type via singleton: ${operationType}`);

      // Use the global singleton manager for subscriptions
      return sseManager.subscribe(operationType, callback);
    },
    []
  );

  // Subscribe to all events (for notifications)
  const subscribeToAll = useCallback(
    (callback: (event: NotificationEvent) => void) => {
      debug.log('Subscribing to all events (for notifications)');

      // Store the callback in a ref so we can call it from handleProgressEvent
      const callbackRef = { current: callback };

      // Add callback to the subscribers set
      notificationSubscribersRef.current.add(callbackRef);

      // Send existing events to new subscriber
      for (const event of events.values()) {
        callback(event);
      }

      return () => {
        debug.log('Unsubscribing from all events (notifications)');
        notificationSubscribersRef.current.delete(callbackRef);
      };
    },
    [events]
  );

  // Get current state for a resource
  const getResourceState = useCallback(
    (resourceId: string): ProgressEvent | null => {
      // Check if any event is for this resource ID using owner_id
      for (const [eventId, event] of events) {
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
    debug.log('🔔 markAsSeen called with eventIds:', eventIds);

    setEvents((prev) => {
      debug.log(
        '🔔 markAsSeen - current events before update:',
        Array.from(prev.values()).map((e) => ({
          id: e.id,
          hasBeenSeen: e.hasBeenSeen,
          operation_type: e.operation_type,
          state: e.state,
        }))
      );

      const newEvents = new Map(prev);
      let hasChanges = false;

      eventIds.forEach((eventId) => {
        const event = newEvents.get(eventId);
        debug.log(`🔔 Processing eventId: ${eventId}`, {
          found: !!event,
          currentlySeen: event?.hasBeenSeen,
          willUpdate: event && !event.hasBeenSeen,
        });

        if (event && !event.hasBeenSeen) {
          newEvents.set(eventId, { ...event, hasBeenSeen: true });
          hasChanges = true;
          debug.log(`🔔 ✅ Marked event ${eventId} as seen/acknowledged`);
        } else if (event && event.hasBeenSeen) {
          debug.log(`🔔 ⚠️ Event ${eventId} was already seen`);
        } else if (!event) {
          debug.log(`🔔 ❌ Event ${eventId} not found in events map`);
        }
      });

      debug.log('🔔 markAsSeen - hasChanges:', hasChanges);

      if (hasChanges) {
        debug.log(
          '🔔 markAsSeen - returning updated events:',
          Array.from(newEvents.values()).map((e) => ({
            id: e.id,
            hasBeenSeen: e.hasBeenSeen,
            operation_type: e.operation_type,
            state: e.state,
          }))
        );
      } else {
        debug.log('🔔 markAsSeen - no changes, returning previous events');
      }

      return hasChanges ? newEvents : prev;
    });
  }, []);

  // Dismiss functionality removed for simplicity

  // Get unseen/unacknowledged count
  const getUnreadCount = useCallback(
    (operationType?: string) => {
      debug.log('🔔 getUnreadCount called for operationType:', operationType);
      debug.log(
        '🔔 getUnreadCount - current events:',
        Array.from(events.values()).map((e) => ({
          id: e.id,
          operation_type: e.operation_type,
          hasBeenSeen: e.hasBeenSeen,
        }))
      );

      let count = 0;
      for (const event of events.values()) {
        // Count events that are unseen (dismissed events are deleted from map)
        if (!event.hasBeenSeen) {
          if (!operationType || event.operation_type === operationType) {
            count++;
            debug.log('🔔 getUnreadCount - counting unseen event:', event.id, event.operation_type);
          }
        }
      }

      debug.log('🔔 getUnreadCount - final count:', count);
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
    [subscribe, subscribeToType, subscribeToAll, getResourceState, markAsSeen, connected]
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
