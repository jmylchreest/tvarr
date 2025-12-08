// Global SSE connection singleton to prevent multiple connections during navigation
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';
import type { ProgressEvent as APIProgressEvent } from '@/types/api';

export interface ProgressEvent extends APIProgressEvent {
  hasBeenVisible?: boolean;
}

type EventCallback = (event: ProgressEvent) => void;
type ConnectionStateCallback = (connected: boolean) => void;

/**
 * SSE Singleton - manages a single SSE connection for the entire application.
 *
 * Design principles:
 * 1. Lazy connection - only connects when there are active subscribers
 * 2. Auto-disconnect - closes connection when all subscribers unsubscribe
 * 3. Resilient - automatically reconnects on errors with exponential backoff
 * 4. React-safe - handles Strict Mode double-mounting and HMR gracefully
 *
 * The singleton does NOT need external lifecycle management (no reset/destroy calls).
 * Connection lifecycle is driven entirely by subscriber count.
 */
class SSESingleton {
  private eventSource: EventSource | null = null;
  private subscribers: Map<string, Set<EventCallback>> = new Map();
  private allSubscribers: Set<EventCallback> = new Set();
  private connectionStateSubscribers: Set<ConnectionStateCallback> = new Set();
  private connected: boolean = false;
  private debug = Debug.createLogger('SSESingleton');
  private reconnectAttempts: number = 0;
  private reconnectTimeoutId: ReturnType<typeof setTimeout> | null = null;
  // Debounce disconnect to handle rapid subscribe/unsubscribe cycles during navigation
  private disconnectTimeoutId: ReturnType<typeof setTimeout> | null = null;
  private static readonly DISCONNECT_DEBOUNCE_MS = 100;

  constructor() {
    // Bind methods to ensure correct 'this' context
    this.handleMessage = this.handleMessage.bind(this);
    this.handleError = this.handleError.bind(this);
    this.handleOpen = this.handleOpen.bind(this);
  }

  /**
   * Get total subscriber count across all subscription types
   */
  private getTotalSubscriberCount(): number {
    let count = this.allSubscribers.size;
    for (const subscribers of this.subscribers.values()) {
      count += subscribers.size;
    }
    return count;
  }

  /**
   * Check if we should have an active connection (has subscribers)
   */
  private shouldBeConnected(): boolean {
    return this.getTotalSubscriberCount() > 0;
  }

  /**
   * Notify connection state subscribers of changes
   */
  private notifyConnectionState(connected: boolean) {
    this.connected = connected;
    this.connectionStateSubscribers.forEach((callback) => {
      try {
        callback(connected);
      } catch (error) {
        this.debug.error('Error in connection state subscriber:', error);
      }
    });
  }

  /**
   * Connect to SSE endpoint if we have subscribers and aren't already connected
   */
  private connect(): void {
    // Don't connect if no subscribers
    if (!this.shouldBeConnected()) {
      this.debug.log('No subscribers, skipping connection');
      return;
    }

    // Cancel any pending disconnect - a new subscriber arrived
    if (this.disconnectTimeoutId) {
      this.debug.log('Cancelling pending disconnect - new subscriber arrived');
      clearTimeout(this.disconnectTimeoutId);
      this.disconnectTimeoutId = null;
    }

    // Already connected or connecting
    if (this.eventSource) {
      const state = this.eventSource.readyState;
      if (state === EventSource.CONNECTING || state === EventSource.OPEN) {
        this.debug.log('Already connected or connecting, skipping');
        return;
      }
      // Clean up closed connection
      this.eventSource = null;
    }

    // Clear any pending reconnect
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }

    this.debug.log('Creating new SSE connection');

    try {
      const backendUrl = getBackendUrl();
      const sseUrl = `${backendUrl}/api/v1/progress/events`;

      this.debug.log('Establishing SSE connection to:', sseUrl);
      this.eventSource = new EventSource(sseUrl);

      // Set up event handlers
      this.eventSource.onopen = this.handleOpen;
      this.eventSource.onerror = this.handleError;
      // Log unnamed message events (should not happen with named events)
      this.eventSource.onmessage = (event: MessageEvent) => {
        console.log('[SSE RAW] Received UNNAMED message event', {
          data: event.data?.substring?.(0, 100) || event.data,
          type: event.type,
        });
        this.handleMessage(event);
      };

      // Listen to all SSE event types from backend
      // Wrap handlers to log the raw event type for debugging
      const wrapHandler = (eventType: string) => (event: MessageEvent) => {
        console.log(`[SSE RAW] Received event type: "${eventType}"`, {
          data: event.data?.substring?.(0, 100) || event.data,
          type: event.type,
        });
        this.handleMessage(event);
      };

      this.eventSource.addEventListener('progress', wrapHandler('progress'));
      this.eventSource.addEventListener('error', wrapHandler('error'));
      this.eventSource.addEventListener('completed', wrapHandler('completed'));
      this.eventSource.addEventListener('cancelled', wrapHandler('cancelled'));
      this.eventSource.addEventListener('heartbeat', () => {
        console.log('[SSE RAW] Received heartbeat');
        this.debug.log('Heartbeat received');
      });
    } catch (error) {
      this.debug.error('Failed to create EventSource:', error);
      this.scheduleReconnect();
    }
  }

  /**
   * Disconnect from SSE endpoint (immediate, used internally)
   */
  private disconnectImmediate(): void {
    this.debug.log('Disconnecting SSE (immediate)');

    // Clear any pending disconnect
    if (this.disconnectTimeoutId) {
      clearTimeout(this.disconnectTimeoutId);
      this.disconnectTimeoutId = null;
    }

    // Clear any pending reconnect
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }

    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }

    this.reconnectAttempts = 0;
    this.notifyConnectionState(false);
  }

  /**
   * Schedule a disconnect with debounce to handle rapid subscribe/unsubscribe cycles
   * This prevents connection churn during page navigation
   */
  private scheduleDisconnect(): void {
    // Already scheduled
    if (this.disconnectTimeoutId) {
      return;
    }

    // Don't schedule if we still have subscribers
    if (this.shouldBeConnected()) {
      return;
    }

    this.debug.log(`Scheduling disconnect in ${SSESingleton.DISCONNECT_DEBOUNCE_MS}ms`);
    this.disconnectTimeoutId = setTimeout(() => {
      this.disconnectTimeoutId = null;
      // Double-check we still have no subscribers
      if (!this.shouldBeConnected()) {
        this.disconnectImmediate();
      }
    }, SSESingleton.DISCONNECT_DEBOUNCE_MS);
  }

  /**
   * Schedule a reconnection attempt with exponential backoff
   */
  private scheduleReconnect(): void {
    // Don't reconnect if no subscribers
    if (!this.shouldBeConnected()) {
      this.debug.log('No subscribers, not scheduling reconnect');
      return;
    }

    // Clear any existing reconnect timeout
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }

    // Stop reconnecting after too many failures
    if (this.reconnectAttempts >= 10) {
      this.debug.error('Too many reconnection attempts, stopping');
      return;
    }

    // Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (max)
    const reconnectDelay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
    this.reconnectAttempts++;

    this.debug.log(`Scheduling reconnect attempt ${this.reconnectAttempts} in ${reconnectDelay}ms`);

    this.reconnectTimeoutId = setTimeout(() => {
      this.reconnectTimeoutId = null;
      if (this.shouldBeConnected()) {
        this.debug.log(`Attempting reconnect (attempt ${this.reconnectAttempts})`);
        this.connect();
      }
    }, reconnectDelay);
  }

  private handleOpen() {
    this.debug.log('SSE connection established successfully');
    this.reconnectAttempts = 0; // Reset on successful connection
    this.notifyConnectionState(true);
  }

  private handleError(error: Event) {
    this.debug.log('SSE connection error:', error);
    this.notifyConnectionState(false);

    // Clean up the failed connection
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }

    // Schedule reconnect if we still have subscribers
    this.scheduleReconnect();
  }

  private handleMessage(event: MessageEvent) {
    try {
      const progressEvent: ProgressEvent = JSON.parse(event.data);

      // Always log terminal events (completed, error, cancelled) for debugging
      const isTerminal = ['completed', 'error', 'cancelled'].includes(progressEvent.state);
      if (isTerminal) {
        console.log('[SSE] Terminal event received:', {
          eventId: progressEvent.id,
          ownerId: progressEvent.owner_id,
          state: progressEvent.state,
          subscriberCount: this.getTotalSubscriberCount(),
        });
      }

      this.debug.log('Broadcasting event to subscribers:', {
        eventId: progressEvent.id,
        ownerId: progressEvent.owner_id,
        operationType: progressEvent.operation_type,
        state: progressEvent.state,
        subscriberCount: this.getTotalSubscriberCount(),
      });

      // Route to resource-specific subscribers
      if (progressEvent.owner_id) {
        const resourceSubscribers = this.subscribers.get(progressEvent.owner_id);
        resourceSubscribers?.forEach((callback) => {
          try {
            callback(progressEvent);
          } catch (error) {
            this.debug.error('Error in resource subscriber:', error);
          }
        });
      }

      // Route to operation-type subscribers
      const typeSubscribers = this.subscribers.get(progressEvent.operation_type);
      typeSubscribers?.forEach((callback) => {
        try {
          callback(progressEvent);
        } catch (error) {
          this.debug.error('Error in type subscriber:', error);
        }
      });

      // Route to all subscribers
      this.allSubscribers.forEach((callback) => {
        try {
          callback(progressEvent);
        } catch (error) {
          this.debug.error('Error in all subscriber:', error);
        }
      });
    } catch (error) {
      this.debug.error('Failed to parse SSE message:', error);
    }
  }

  /**
   * Subscribe to events for a specific key (resource ID or operation type)
   */
  subscribe(key: string, callback: EventCallback): () => void {
    this.debug.log(`Subscribing to key: ${key}`);

    if (!this.subscribers.has(key)) {
      this.subscribers.set(key, new Set());
    }

    this.subscribers.get(key)!.add(callback);

    // Connect if this is the first subscriber
    this.connect();

    // Return unsubscribe function
    return () => {
      this.debug.log(`Unsubscribing from key: ${key}`);
      const keySubscribers = this.subscribers.get(key);
      if (keySubscribers) {
        keySubscribers.delete(callback);
        if (keySubscribers.size === 0) {
          this.subscribers.delete(key);
        }
      }

      // Schedule disconnect with debounce to handle navigation
      this.scheduleDisconnect();
    };
  }

  /**
   * Subscribe to all events
   */
  subscribeToAll(callback: EventCallback): () => void {
    this.debug.log('Subscribing to all events');
    this.allSubscribers.add(callback);

    // Connect if this is the first subscriber
    this.connect();

    return () => {
      this.debug.log('Unsubscribing from all events');
      this.allSubscribers.delete(callback);

      // Schedule disconnect with debounce to handle navigation
      this.scheduleDisconnect();
    };
  }

  /**
   * Subscribe to connection state changes
   */
  subscribeToConnectionState(callback: ConnectionStateCallback): () => void {
    this.connectionStateSubscribers.add(callback);
    // Immediately notify of current state
    callback(this.connected);

    return () => {
      this.connectionStateSubscribers.delete(callback);
    };
  }

  /**
   * Check if currently connected
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Force reconnection (use sparingly - e.g., when backend comes back online)
   */
  forceReconnect(): void {
    this.debug.log('Force reconnect requested');
    this.reconnectAttempts = 0;

    // Clear any pending disconnect
    if (this.disconnectTimeoutId) {
      clearTimeout(this.disconnectTimeoutId);
      this.disconnectTimeoutId = null;
    }

    // Close existing connection
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }

    // Clear any pending reconnect
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }

    // Reconnect if we have subscribers
    if (this.shouldBeConnected()) {
      this.connect();
    }
  }
}

// Global singleton instance
export const sseManager = new SSESingleton();

// Cleanup on page unload to avoid connection leaks
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    // Note: We don't call disconnect() here because the browser
    // will automatically close the connection on unload
  });
}
