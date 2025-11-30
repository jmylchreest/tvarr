// Global SSE connection singleton to prevent multiple connections during navigation
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';
import type { ProgressEvent as APIProgressEvent } from '@/types/api';

export interface ProgressEvent extends APIProgressEvent {
  hasBeenVisible?: boolean;
}

type EventCallback = (event: ProgressEvent) => void;

class SSESingleton {
  private eventSource: EventSource | null = null;
  private subscribers: Map<string, Set<EventCallback>> = new Map();
  private allSubscribers: Set<EventCallback> = new Set();
  private connected: boolean = false;
  private debug = Debug.createLogger('SSESingleton');
  private setupPromise: Promise<void> | null = null;
  private reconnectAttempts: number = 0;
  private destroyed: boolean = false;
  private reconnectTimeoutId: NodeJS.Timeout | null = null;

  constructor() {
    // Bind methods to ensure correct 'this' context
    this.handleMessage = this.handleMessage.bind(this);
    this.handleError = this.handleError.bind(this);
    this.handleOpen = this.handleOpen.bind(this);
  }

  async ensureConnection(): Promise<void> {
    // Don't try to connect if we've been destroyed
    if (this.destroyed) {
      this.debug.log('SSE singleton has been destroyed, not connecting');
      return Promise.reject(new Error('SSE singleton destroyed'));
    }

    if (this.eventSource && this.eventSource.readyState === EventSource.OPEN) {
      this.debug.log('SSE connection already established');
      return;
    }

    if (this.setupPromise) {
      this.debug.log('SSE connection setup already in progress');
      return this.setupPromise;
    }

    this.debug.log('Creating new SSE connection');

    this.setupPromise = new Promise(async (resolve, reject) => {
      try {
        const backendUrl = getBackendUrl();
        const sseUrl = `${backendUrl}/api/v1/progress/events`;

        this.debug.log('Establishing SSE connection to:', sseUrl);
        this.eventSource = new EventSource(sseUrl);

        const connectionTimeout = setTimeout(() => {
          this.debug.error('SSE connection timeout');
          if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
          }
          this.connected = false;
          this.setupPromise = null;
          reject(new Error('SSE connection timeout'));
        }, 5000);

        this.eventSource.onopen = () => {
          clearTimeout(connectionTimeout);
          this.handleOpen();
          this.setupPromise = null;
          resolve();
        };

        this.eventSource.onerror = (error) => {
          clearTimeout(connectionTimeout);
          this.handleError(error);
          this.setupPromise = null;
          reject(error);
        };

        this.eventSource.onmessage = this.handleMessage;
        this.eventSource.addEventListener('progress', this.handleMessage);
        this.eventSource.addEventListener('heartbeat', (event) => {
          this.debug.log('Heartbeat received');
        });
      } catch (error) {
        this.debug.error('Failed to create EventSource:', error);
        this.setupPromise = null;
        reject(error);
      }
    });

    return this.setupPromise;
  }

  private handleOpen() {
    this.debug.log('Global SSE connection established successfully');
    this.connected = true;
    this.reconnectAttempts = 0; // Reset on successful connection
  }

  private handleError(error: Event) {
    this.debug.log('SSE connection error:', error);
    this.connected = false;

    // Don't try to reconnect if we've been destroyed
    if (this.destroyed) {
      this.debug.log('SSE singleton destroyed, not attempting reconnection');
      return;
    }

    // Clear any existing reconnect timeout
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }

    // Exponential backoff for reconnection
    const reconnectDelay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000);
    this.reconnectAttempts++;

    // Stop reconnecting after too many failures
    if (this.reconnectAttempts > 10) {
      this.debug.error('Too many reconnection attempts, stopping');
      return;
    }

    // Auto-reconnect after delay with backoff
    this.reconnectTimeoutId = setTimeout(() => {
      this.reconnectTimeoutId = null;
      if (!this.destroyed && !this.connected && !this.setupPromise) {
        this.debug.log(`Attempting to reconnect SSE (attempt ${this.reconnectAttempts})`);
        this.ensureConnection().catch((err) => {
          this.debug.error('Failed to reconnect:', err);
        });
      }
    }, reconnectDelay);
  }

  private handleMessage(event: MessageEvent) {
    try {
      const progressEvent: ProgressEvent = JSON.parse(event.data);

      this.debug.log('Broadcasting event to subscribers:', {
        eventId: progressEvent.id,
        ownerId: progressEvent.owner_id,
        operationType: progressEvent.operation_type,
        subscriberCount: this.subscribers.size + this.allSubscribers.size,
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

  subscribe(key: string, callback: EventCallback): () => void {
    this.debug.log(`Subscribing to key: ${key}`);

    if (!this.subscribers.has(key)) {
      this.subscribers.set(key, new Set());
    }

    this.subscribers.get(key)!.add(callback);

    // Ensure connection is established
    this.ensureConnection().catch((err) => {
      this.debug.error('Failed to establish connection for subscription:', err);
    });

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
    };
  }

  subscribeToAll(callback: EventCallback): () => void {
    this.debug.log('Subscribing to all events');
    this.allSubscribers.add(callback);

    // Ensure connection is established
    this.ensureConnection().catch((err) => {
      this.debug.error('Failed to establish connection for all subscription:', err);
    });

    return () => {
      this.debug.log('Unsubscribing from all events');
      this.allSubscribers.delete(callback);
    };
  }

  isConnected(): boolean {
    return this.connected;
  }

  // Only call this when the entire application is being closed/refreshed or backend is down
  destroy() {
    this.debug.log('Destroying SSE connection');
    this.destroyed = true;

    // Clear any pending reconnect timeout
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }

    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    this.connected = false;
    this.subscribers.clear();
    this.allSubscribers.clear();
    this.setupPromise = null;
    this.reconnectAttempts = 0; // Reset reconnect attempts when destroyed
  }

  // Reset state to allow reconnection (called when backend comes back up)
  reset() {
    this.debug.log('Resetting SSE singleton for reconnection');
    this.destroyed = false;
    this.reconnectAttempts = 0;

    // Clear any pending reconnect timeout
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }
  }
}

// Global singleton instance
export const sseManager = new SSESingleton();

// Cleanup on page unload
if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    sseManager.destroy();
  });
}
