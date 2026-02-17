import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for tvarr frontend tests.
 *
 * Tests use page.route() to mock API responses, so no running backend is needed.
 * The Next.js dev server is started automatically via webServer config.
 *
 * Run tests:
 *   pnpm exec playwright test                    # all projects
 *   pnpm exec playwright test --project=mobile   # mobile only
 *   pnpm exec playwright test --project=desktop  # desktop only
 *
 * @see https://playwright.dev/docs/test-configuration
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,

  reporter: process.env.CI
    ? [['github'], ['html', { open: 'never' }]]
    : [['list'], ['html', { open: 'on-failure' }]],

  /* Shared settings for all projects */
  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    /* Timeout for each action (click, fill, etc.) */
    actionTimeout: 10_000,
  },

  /* Global test timeout */
  timeout: 30_000,

  /* Expect timeout */
  expect: {
    timeout: 5_000,
  },

  projects: [
    // ─── Desktop ───────────────────────────────────────
    {
      name: 'desktop',
      use: {
        ...devices['Desktop Chrome'],
      },
    },

    // ─── Mobile devices ────────────────────────────────
    // All mobile profiles use Chromium for cross-platform compatibility.
    // We override defaultBrowserType to avoid WebKit dependency issues
    // on non-Ubuntu Linux. The viewport/touch/userAgent emulation is
    // what matters for responsive testing, not the actual browser engine.
    {
      name: 'mobile',
      use: {
        // iPhone SE - small screen (320x568), good for testing tight layouts
        ...devices['iPhone SE'],
        defaultBrowserType: 'chromium',
      },
    },
    {
      name: 'mobile-large',
      use: {
        // iPhone 14 Pro Max - large phone (430x932)
        ...devices['iPhone 14 Pro Max'],
        defaultBrowserType: 'chromium',
      },
    },
    {
      name: 'mobile-android',
      use: {
        // Pixel 7 - standard Android device (412x915)
        ...devices['Pixel 7'],
      },
    },

    // ─── Tablet ────────────────────────────────────────
    {
      name: 'tablet',
      use: {
        // iPad Mini landscape - right at the md breakpoint boundary
        ...devices['iPad Mini landscape'],
        defaultBrowserType: 'chromium',
      },
    },
  ],

  /* Start Next.js dev server before running tests */
  webServer: {
    command: 'pnpm run dev',
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
