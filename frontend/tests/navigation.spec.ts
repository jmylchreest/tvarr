import { test, expect } from './fixtures/mock-api';

/**
 * Navigation tests — verifies sidebar navigation works across all viewports.
 *
 * On mobile (<768px): Sidebar is a Sheet (hamburger menu), should auto-close
 *   after clicking a navigation link.
 *
 * On desktop/tablet (>=768px): Sidebar is always visible as a collapsible panel.
 *   Navigation links work directly without a hamburger trigger.
 */

test.describe('Sidebar Navigation', () => {
  test('sidebar trigger is visible on mobile, sidebar visible on desktop', async ({ page, mockApi, vp }) => {
    await page.goto('/');

    if (vp.isMobile) {
      const trigger = page.locator('[data-sidebar="trigger"]');
      await expect(trigger).toBeVisible();
    } else {
      // On desktop, sidebar is always visible
      await expect(page.getByRole('link', { name: 'Proxies' })).toBeVisible();
    }
  });

  test('mobile sidebar sheet opens and shows nav links', async ({ page, mockApi, vp }) => {
    test.skip(!vp.isMobile, 'Mobile-only test');
    await page.goto('/');
    await page.locator('[data-sidebar="trigger"]').click();

    const mobileSidebar = page.locator('[data-mobile="true"]');
    await expect(mobileSidebar).toBeVisible();

    // Key navigation links are present
    await expect(page.getByRole('link', { name: 'Proxies' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Stream Sources' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'EPG Sources' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Filters' })).toBeVisible();
  });

  test('navigating via sidebar loads target page', async ({ page, mockApi, vp }) => {
    await page.goto('/');

    if (vp.isMobile) {
      await page.locator('[data-sidebar="trigger"]').click();
    }

    await page.getByRole('link', { name: 'Proxies' }).click();

    await expect(page).toHaveURL(/\/proxies/);
    await expect(page.getByText('UK Freeview')).toBeVisible();
  });

  test('mobile sidebar closes after navigation', async ({ page, mockApi, vp }) => {
    test.skip(!vp.isMobile, 'Mobile-only test');
    await page.goto('/');

    await page.locator('[data-sidebar="trigger"]').click();
    const mobileSidebar = page.locator('[data-mobile="true"]');
    await expect(mobileSidebar).toBeVisible();

    await page.getByRole('link', { name: 'Proxies' }).click();

    // Sidebar should auto-close after navigation
    await expect(mobileSidebar).not.toBeVisible({ timeout: 5_000 });
  });

  test('can navigate between multiple pages', async ({ page, mockApi, vp }) => {
    // Start on Proxies
    await page.goto('/proxies/');
    await expect(page.getByText('UK Freeview')).toBeVisible();

    // Navigate to Stream Sources
    if (vp.isMobile) {
      await page.locator('[data-sidebar="trigger"]').click();
      const mobileSidebar = page.locator('[data-mobile="true"]');
      await expect(mobileSidebar).toBeVisible();
    }
    await page.getByRole('link', { name: 'Stream Sources' }).click();
    await expect(page.getByText('Primary IPTV')).toBeVisible();

    // Navigate to Filters
    if (vp.isMobile) {
      // Wait for sidebar to close from previous nav, then re-open
      await expect(page.locator('[data-mobile="true"]')).not.toBeVisible({ timeout: 5_000 });
      await page.locator('[data-sidebar="trigger"]').click();
      await expect(page.locator('[data-mobile="true"]')).toBeVisible();
    }
    await page.getByRole('link', { name: 'Filters' }).click();
    await expect(page.getByText('UK Only')).toBeVisible();
  });
});
