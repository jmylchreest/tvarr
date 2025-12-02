import { LogEntry, LogHandler } from '@/types/api';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';

export class LogsClient {
  private eventSource: EventSource | null = null;
  private handlers: LogHandler[] = [];
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;
  private debug = Debug.createLogger('LogsClient');

  connect() {
    if (this.eventSource) {
      this.debug.log('Disconnecting existing connection before reconnecting');
      this.disconnect();
    }

    try {
      this.debug.log('Connecting to logs stream');
      const backendUrl = getBackendUrl();
      this.eventSource = new EventSource(`${backendUrl}/api/v1/logs/stream`);

      this.eventSource.onopen = () => {
        this.debug.log('Connection opened successfully');
        this.reconnectAttempts = 0;
      };

      // Handle ALL message events generically
      this.eventSource.onmessage = (event) => {
        this.debug.log('Default message received:', event.data);
        this.parseAndHandleLog(event.data);
      };

      // Specifically listen for 'log' events
      this.eventSource.addEventListener('log', (event: MessageEvent) => {
        this.debug.log('Log event received:', event.data);
        this.parseAndHandleLog(event.data);
      });

      // Listen for other common SSE event types
      const otherEventTypes = ['message', 'data', 'update', 'entry', 'record'];
      otherEventTypes.forEach((eventType) => {
        this.eventSource!.addEventListener(eventType, (event: MessageEvent) => {
          this.debug.log(`${eventType} event received:`, event.data);
          this.parseAndHandleLog(event.data);
        });
      });

      this.eventSource.onerror = (error) => {
        console.error(
          '[Logs] Connection error:',
          error,
          'ReadyState:',
          this.eventSource?.readyState
        );
        this.handleReconnect();
      };
    } catch (error) {
      console.error('[Logs] Failed to create SSE connection:', error);
    }
  }

  private parseAndHandleLog(data: string) {
    try {
      const logData = JSON.parse(data);
      this.debug.log('Parsed log data:', logData);

      // Create a LogEntry from any JSON structure
      let logEntry: LogEntry;

      if ('level' in logData && 'message' in logData) {
        // Handle standard log entries
        logEntry = {
          id: logData.id || `log-${Date.now()}`,
          timestamp:
            logData.timestamp || logData.created_at || logData.time || new Date().toISOString(),
          level: this.normalizeLogLevel(logData.level),
          message: logData.message,
          module: logData.module || logData.component || logData.source,
          target: logData.target || logData.logger,
          file: logData.file || logData.filename,
          line: logData.line || logData.line_number,
          context: logData.context || logData.metadata || logData.extra,
          fields: logData.fields || {},
        };
      } else {
        // Handle completely generic JSON as a log entry
        logEntry = {
          id: logData.id || `log-${Date.now()}`,
          timestamp:
            logData.timestamp || logData.created_at || logData.time || new Date().toISOString(),
          level: 'info',
          message:
            logData.message ||
            logData.description ||
            logData.text ||
            JSON.stringify(logData).substring(0, 200),
          module: logData.module || logData.component || logData.source || 'unknown',
          target: logData.target || logData.logger,
          file: logData.file || logData.filename,
          line: logData.line || logData.line_number,
          context: logData,
          fields: logData.fields || {},
        };
      }

      this.handleLog(logEntry);
    } catch (error) {
      console.error('[Logs] Failed to parse log:', error, 'Raw data:', data);
      // Create a fallback log entry for unparseable data
      const fallbackLog: LogEntry = {
        id: `parse-error-${Date.now()}`,
        timestamp: new Date().toISOString(),
        level: 'error',
        message: `Failed to parse log data: ${data.substring(0, 100)}...`,
        module: 'log-parser',
        target: 'log-parser',
        fields: {},
        context: {
          raw_data: data,
          error: error instanceof Error ? error.message : 'Unknown error',
        },
      };
      this.handleLog(fallbackLog);
    }
  }

  private normalizeLogLevel(level: string): LogEntry['level'] {
    const normalized = level.toLowerCase();
    if (['trace', 'debug', 'info', 'warn', 'error'].includes(normalized)) {
      return normalized as LogEntry['level'];
    }
    // Map common alternatives
    switch (normalized) {
      case 'warning':
        return 'warn';
      case 'err':
        return 'error';
      case 'information':
        return 'info';
      default:
        return 'info';
    }
  }

  private handleLog(log: LogEntry) {
    this.debug.log(`Handling log: ${log.level} - ${log.message}`);
    this.handlers.forEach((handler) => handler(log));
  }

  private handleReconnect() {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      // Exponential backoff: 1s, 2s, 4s, 8s, 16s (max 30s)
      const delay = Math.min(this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1), 30000);
      this.debug.log(
        `Attempting to reconnect (attempt ${this.reconnectAttempts}) after ${delay}ms`
      );

      setTimeout(() => {
        if (this.eventSource?.readyState === EventSource.CLOSED) {
          this.connect();
        }
      }, delay);
    } else {
      console.error('[Logs] Max reconnect attempts reached, giving up');
      // Reset attempts so manual reconnect can work later
      this.reconnectAttempts = 0;
    }
  }

  subscribe(handler: LogHandler) {
    this.handlers.push(handler);
  }

  unsubscribe(handler: LogHandler) {
    const index = this.handlers.indexOf(handler);
    if (index > -1) {
      this.handlers.splice(index, 1);
    }
  }

  disconnect() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    this.handlers = [];
    this.reconnectAttempts = 0;
  }

  isConnected(): boolean {
    return this.eventSource?.readyState === EventSource.OPEN;
  }
}

// Export singleton instance
export const logsClient = new LogsClient();
