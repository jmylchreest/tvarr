import { test as base, Page } from '@playwright/test';
import {
  proxiesApiResponse,
  streamSourcesApiResponse,
  epgSourcesApiResponse,
  filtersApiResponse,
  encodingProfilesApiResponse,
  dataMappingApiResponse,
  clientDetectionRulesApiResponse,
  encoderOverridesApiResponse,
  transcodersApiResponse,
  backupsApiResponse,
  mockProxies,
  mockStreamSources,
  mockEpgSources,
  mockFilters,
  mockEncodingProfiles,
  mockDataMappingRules,
  mockClientDetectionRules,
  mockEncoderOverrides,
  mockDaemons,
  mockClusterStats,
  mockBackups,
  mockBackupSchedule,
} from './mock-data';

/**
 * API mock layer for Playwright tests.
 *
 * Intercepts all API calls at the browser level using page.route(),
 * so no running backend is needed. The Next.js dev server proxies
 * /api/v1/* to localhost:8080 — we intercept before that happens.
 *
 * Usage:
 *   import { test, expect } from '../fixtures/mock-api';
 *
 *   test('my test', async ({ page, mockApi }) => {
 *     // API is already mocked — just navigate
 *     await page.goto('/proxies/');
 *   });
 *
 *   test('custom mock', async ({ page, mockApi }) => {
 *     // Override specific endpoint
 *     mockApi.setProxies([]);
 *     await page.goto('/proxies/');
 *   });
 */

export interface MockApiFixture {
  /** Override the proxies returned by GET /api/v1/proxies */
  setProxies: (proxies: typeof mockProxies) => void;
  /** Override the stream sources returned by GET /api/v1/sources/stream */
  setStreamSources: (sources: typeof mockStreamSources) => void;
  /** Override the EPG sources returned by GET /api/v1/sources/epg */
  setEpgSources: (sources: typeof mockEpgSources) => void;
  /** Override the filters returned by GET /api/v1/filters */
  setFilters: (filters: typeof mockFilters) => void;
  /** Override the encoding profiles returned by GET /api/v1/encoding-profiles */
  setEncodingProfiles: (profiles: typeof mockEncodingProfiles) => void;
}

/**
 * Sets up all API route mocks on a page. Call before navigating.
 * Returns a MockApiFixture with methods to override defaults.
 */
async function setupApiMocks(page: Page): Promise<MockApiFixture> {
  // Mutable state — tests can override via fixture methods
  let currentProxies = [...mockProxies];
  let currentStreamSources = [...mockStreamSources];
  let currentEpgSources = [...mockEpgSources];
  let currentFilters = [...mockFilters];
  let currentEncodingProfiles = [...mockEncodingProfiles];

  // ─── Catch-all (register FIRST = lowest priority) ──────
  // Playwright processes routes in reverse registration order,
  // so routes registered later take priority. The catch-all must
  // be registered first so specific routes override it.

  await page.route('**/api/v1/**', (route) => {
    console.warn(`[mock-api] Unhandled API call: ${route.request().method()} ${route.request().url()}`);
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({}),
    });
  });

  // Themes — matches ThemeListResponse schema
  await page.route('**/api/v1/themes**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ themes: [], default: 'graphite' }),
    })
  );

  // ─── Global endpoints (every page load) ────────────────

  // Backend liveness check
  await page.route('**/livez', (route) =>
    route.fulfill({ status: 200, body: 'ok' })
  );

  // Feature flags — matches GetFeaturesOutputBody schema
  await page.route('**/api/v1/features', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          flags: {},
          config: {},
          timestamp: new Date().toISOString(),
        },
      }),
    })
  );

  // Progress operations (REST seed)
  await page.route('**/api/v1/progress/operations', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ operations: [] }),
    })
  );

  // SSE progress events — return a properly named heartbeat event.
  // IMPORTANT: The heartbeat MUST use "event: heartbeat" prefix so the
  // SSE manager routes it to the heartbeat handler (which is a no-op)
  // instead of the generic onmessage handler (which parses as ProgressEvent
  // and crashes NotificationBell when event.state is undefined).
  await page.route('**/api/v1/progress/events', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      headers: {
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
      },
      body: 'event: heartbeat\ndata: {}\n\n',
    })
  );

  // System version (sidebar) — matches Info schema
  await page.route('**/api/v1/system/version', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        version: '0.1.0-test',
        commit: 'abc1234',
        commit_sha: 'abc1234',
        date: '2025-02-17T00:00:00Z',
        branch: 'test',
        tree_state: 'clean',
        go_version: 'go1.26.0',
        platform: 'linux/amd64',
        os: 'linux',
        arch: 'amd64',
      }),
    })
  );

  // ─── Page-specific endpoints ───────────────────────────

  // GET /api/v1/proxies
  await page.route('**/api/v1/proxies', (route) => {
    if (route.request().method() === 'GET') {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(proxiesApiResponse(currentProxies)),
      });
    }
    // POST /api/v1/proxies — create
    if (route.request().method() === 'POST') {
      const newProxy = {
        ...currentProxies[0],
        id: `proxy-new-${Date.now()}`,
        name: 'New Proxy',
        status: 'pending' as const,
        channel_count: 0,
        program_count: 0,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      return route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ data: newProxy }),
      });
    }
    return route.continue();
  });

  // GET /api/v1/proxies/:id
  await page.route('**/api/v1/proxies/*', (route) => {
    const url = route.request().url();
    // Skip if this is the list endpoint (no trailing ID)
    if (url.endsWith('/proxies') || url.endsWith('/proxies/')) {
      return route.continue();
    }
    const id = url.split('/proxies/')[1]?.split('?')[0]?.split('/')[0];
    const proxy = currentProxies.find((p) => p.id === id);
    if (proxy) {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(proxy),
      });
    }
    return route.fulfill({ status: 404, body: 'Not found' });
  });

  // GET /api/v1/sources/stream
  await page.route('**/api/v1/sources/stream', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(streamSourcesApiResponse(currentStreamSources)),
    })
  );

  // GET /api/v1/sources/epg
  await page.route('**/api/v1/sources/epg', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(epgSourcesApiResponse(currentEpgSources)),
    })
  );

  // GET /api/v1/filters
  await page.route('**/api/v1/filters', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(filtersApiResponse(currentFilters)),
    })
  );

  // GET /api/v1/encoding-profiles
  await page.route('**/api/v1/encoding-profiles', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(encodingProfilesApiResponse(currentEncodingProfiles)),
    })
  );

  // GET /api/v1/data-mapping
  await page.route('**/api/v1/data-mapping', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(dataMappingApiResponse()),
    })
  );

  // GET /api/v1/client-detection-rules
  await page.route('**/api/v1/client-detection-rules', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(clientDetectionRulesApiResponse()),
    })
  );

  // GET /api/v1/encoder-overrides
  await page.route('**/api/v1/encoder-overrides', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(encoderOverridesApiResponse()),
    })
  );

  // GET /api/v1/transcoders
  await page.route('**/api/v1/transcoders', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(transcodersApiResponse()),
    })
  );

  // GET /api/v1/transcoders/stats
  await page.route('**/api/v1/transcoders/stats', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockClusterStats),
    })
  );

  // GET /api/v1/backups
  await page.route('**/api/v1/backups', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(backupsApiResponse()),
    })
  );

  // GET /api/v1/backups/schedule
  await page.route('**/api/v1/backups/schedule', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockBackupSchedule),
    })
  );

  // ─── Secondary endpoints (detail panels, editors) ──────────
  // These are called when a detail panel is opened or an editor
  // needs field/source metadata. They return arrays or objects
  // (not wrapped in a named property like the list endpoints).

  // GET /api/v1/sources — combined list of all sources (stream + epg)
  await page.route('**/api/v1/sources', (route) => {
    const url = route.request().url();
    // Don't match /api/v1/sources/stream or /api/v1/sources/epg
    if (url.includes('/sources/stream') || url.includes('/sources/epg')) {
      return route.fallback();
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([]),
    });
  });

  // GET /api/v1/filters/fields/:type — field definitions for filter editor
  await page.route('**/api/v1/filters/fields/**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([]),
    })
  );

  // GET /api/v1/data-mapping/fields/:type — field definitions for data mapping editor
  await page.route('**/api/v1/data-mapping/fields/**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([]),
    })
  );

  // GET /api/v1/data-mapping/helpers — helper functions for data mapping expressions
  await page.route('**/api/v1/data-mapping/helpers', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ helpers: [] }),
    })
  );

  // GET /api/v1/client-detection/fields — field definitions for client detection editor
  await page.route('**/api/v1/client-detection/fields', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([]),
    })
  );

  return {
    setProxies: (proxies) => { currentProxies = proxies; },
    setStreamSources: (sources) => { currentStreamSources = sources; },
    setEpgSources: (sources) => { currentEpgSources = sources; },
    setFilters: (filters) => { currentFilters = filters; },
    setEncodingProfiles: (profiles) => { currentEncodingProfiles = profiles; },
  };
}

/**
 * Viewport info fixture — provides adaptive assertions for tests that
 * run across mobile, tablet, and desktop projects.
 *
 * Usage:
 *   test('my test', async ({ page, mockApi, vp }) => {
 *     if (vp.isMobile) {
 *       // Mobile-specific assertion
 *     } else {
 *       // Desktop/tablet assertion
 *     }
 *   });
 */
export interface ViewportInfo {
  /** Viewport width in pixels */
  width: number;
  /** Viewport height in pixels */
  height: number;
  /** True when viewport width < 768px (matches Tailwind md breakpoint) */
  isMobile: boolean;
  /** True when viewport width >= 768px */
  isDesktop: boolean;
  /** True when viewport width >= 768px and < 1024px */
  isTablet: boolean;
}

/**
 * Extended test fixture with API mocking and viewport info.
 *
 * Import this instead of @playwright/test in your test files:
 *   import { test, expect } from '../fixtures/mock-api';
 *
 * Note: The viewport fixture is named `vp` (not `viewport`) to avoid
 * conflicting with Playwright's built-in `viewport` fixture.
 */
/* eslint-disable react-hooks/rules-of-hooks -- `use` is Playwright's fixture API, not React's hook */
export const test = base.extend<{ mockApi: MockApiFixture; vp: ViewportInfo }>({
  mockApi: async ({ page }, use) => {
    const fixture = await setupApiMocks(page);
    await use(fixture);
  },
  vp: async ({ page }, use) => {
    const size = page.viewportSize() ?? { width: 1280, height: 720 };
    await use({
      width: size.width,
      height: size.height,
      isMobile: size.width < 768,
      isDesktop: size.width >= 768,
      isTablet: size.width >= 768 && size.width < 1024,
    });
  },
});
/* eslint-enable react-hooks/rules-of-hooks */

export { expect } from '@playwright/test';
