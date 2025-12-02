// Frontend version - injected at build time via NEXT_PUBLIC_VERSION
// Defaults to "dev" for development builds without explicit version
export const APP_VERSION = process.env.NEXT_PUBLIC_VERSION || 'dev';

// Centralized function to get backend URL
// Since the frontend is embedded and served by the same backend server,
// we can use relative URLs which automatically use the correct host and port
export function getBackendUrl(): string {
  // For development, use environment variable with fallback to localhost:8080
  if (process.env.NODE_ENV === 'development') {
    return (
      process.env.NEXT_PUBLIC_API_BASE_URL ||
      process.env.NEXT_PUBLIC_BACKEND_URL ||
      'http://localhost:8080'
    );
  }

  // For production (embedded in backend), use relative URLs
  // This automatically works with any host/port the backend is running on
  return '';
}

// API Configuration
export const API_CONFIG = {
  baseUrl: getBackendUrl(),
  endpoints: {
    streamSources: '/api/v1/sources/stream',
    epgSources: '/api/v1/sources/epg',
    proxies: '/api/v1/proxies',
    filters: '/api/v1/filters',
    dataMapping: '/api/v1/data-mapping',
    logos: '/api/v1/logos',
    relays: '/api/v1/relay',
    dashboard: '/api/v1/metrics/dashboard',
    health: '/health',
  },
} as const;

// Request timeout in milliseconds
export const REQUEST_TIMEOUT = 30000;

// Default pagination settings
export const DEFAULT_PAGE_SIZE = 20;
export const MAX_PAGE_SIZE = 100;
