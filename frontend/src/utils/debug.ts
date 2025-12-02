import { isFeatureEnabled } from '@/hooks/useFeatureFlags';

/**
 * Debug logging utility that respects the debug-frontend feature flag
 */
export class Debug {
  /**
   * Log a debug message if debug-frontend feature is enabled
   */
  static log(...args: any[]): void {
    if (isFeatureEnabled('debug-frontend')) {
      console.log('[DEBUG]', ...args);
    }
  }

  /**
   * Log a debug info message if debug-frontend feature is enabled
   */
  static info(...args: any[]): void {
    if (isFeatureEnabled('debug-frontend')) {
      console.info('[DEBUG-INFO]', ...args);
    }
  }

  /**
   * Log a debug warning message if debug-frontend feature is enabled
   */
  static warn(...args: any[]): void {
    if (isFeatureEnabled('debug-frontend')) {
      console.warn('[DEBUG-WARN]', ...args);
    }
  }

  /**
   * Log a debug error message if debug-frontend feature is enabled
   */
  static error(...args: any[]): void {
    if (isFeatureEnabled('debug-frontend')) {
      console.error('[DEBUG-ERROR]', ...args);
    }
  }

  /**
   * Create a prefixed debug logger for a specific component or module
   */
  static createLogger(prefix: string) {
    return {
      log: (...args: any[]) => Debug.log(`[${prefix}]`, ...args),
      info: (...args: any[]) => Debug.info(`[${prefix}]`, ...args),
      warn: (...args: any[]) => Debug.warn(`[${prefix}]`, ...args),
      error: (...args: any[]) => Debug.error(`[${prefix}]`, ...args),
    };
  }

  /**
   * Check if debug mode is currently enabled
   */
  static isEnabled(): boolean {
    return isFeatureEnabled('debug-frontend');
  }
}

// Convenience exports for common usage
export const debugLog = Debug.log;
export const debugInfo = Debug.info;
export const debugWarn = Debug.warn;
export const debugError = Debug.error;
