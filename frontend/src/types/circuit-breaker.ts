// Circuit Breaker Types for Rich Visualization
// These types match the backend API response from /api/v1/circuit-breakers/stats

export type CircuitState = 'closed' | 'open' | 'half-open';

export interface ErrorCategoryCount {
  success_2xx: number;
  client_error_4xx: number;
  server_error_5xx: number;
  timeout: number;
  network_error: number;
}

export interface StateTransition {
  timestamp: string;
  from_state: CircuitState;
  to_state: CircuitState;
  reason: string;
  consecutive_count: number;
}

export interface StateDurationSummary {
  closed_duration_ms: number;
  open_duration_ms: number;
  half_open_duration_ms: number;
  total_duration_ms: number;
  closed_percentage: number;
  open_percentage: number;
  half_open_percentage: number;
}

export interface CircuitBreakerConfig {
  failure_threshold: number;
  reset_timeout: string;
  half_open_max: number;
  acceptable_status_codes?: string;
}

export interface EnhancedCircuitBreakerStats {
  name: string;
  state: CircuitState;
  state_entered_at: string;
  state_duration_ms: number;
  consecutive_failures: number;
  consecutive_successes: number;
  total_requests: number;
  total_successes: number;
  total_failures: number;
  failure_rate: number;
  error_counts: ErrorCategoryCount;
  state_durations: StateDurationSummary;
  recent_transitions: StateTransition[];
  last_failure?: string | null;
  last_success?: string | null;
  next_half_open_at?: string | null;
  config: CircuitBreakerConfig;
}

// Helper to get config values with fallback
export function getConfigValue<K extends keyof CircuitBreakerConfig>(
  stats: EnhancedCircuitBreakerStats,
  key: K
): CircuitBreakerConfig[K] {
  return stats.config[key];
}

// API response wrapper
export interface EnhancedStatsResponse {
  success: boolean;
  data: EnhancedCircuitBreakerStats[];
}

// Helper function to calculate success rate
export function calculateSuccessRate(stats: EnhancedCircuitBreakerStats): number {
  if (stats.total_requests === 0) return 100;
  return (stats.total_successes / stats.total_requests) * 100;
}

// Helper function to format duration
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days}d ${hours % 24}h`;
}

// Helper to get state color classes
export function getStateColorClasses(state: CircuitState): string {
  switch (state) {
    case 'closed':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100';
    case 'open':
      return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-100';
    case 'half-open':
      return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-100';
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-100';
  }
}
