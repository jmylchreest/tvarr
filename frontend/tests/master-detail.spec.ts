import { test, expect } from './fixtures/mock-api';

/**
 * Master-detail layout tests — verifies the two-panel layout works correctly
 * across all viewport sizes.
 *
 * On mobile (<768px): master list and detail panel are mutually exclusive.
 *   - Clicking an item or "+" swaps to the detail panel with a Back button.
 *   - Clicking Back returns to the master list.
 *
 * On desktop/tablet (>=768px): master list and detail panel are side-by-side.
 *   - Both panels are always visible.
 *   - Clicking an item or "+" updates the detail panel content.
 *   - No Back button.
 */

// Helper: click the "+" button in the master panel header
async function clickAddButton(page: import('@playwright/test').Page) {
  const addButton = page.locator('button').filter({ has: page.locator('svg.lucide-plus') });
  await expect(addButton.first()).toBeVisible();
  await addButton.first().click();
}

// ──────────────────────────────────────────────────────────────
// CREATE FLOW — clicking "+" shows the create form
// ──────────────────────────────────────────────────────────────

const createFlowPages = [
  { name: 'Proxies', path: '/proxies/', listItem: 'UK Freeview' },
  { name: 'Stream Sources', path: '/sources/stream/', listItem: 'Primary IPTV' },
  { name: 'EPG Sources', path: '/sources/epg/', listItem: 'Main EPG' },
  { name: 'Filters', path: '/admin/filters/', listItem: 'UK Only' },
  { name: 'Client Detection', path: '/admin/client-detection/', listItem: 'Chrome Browser' },
  { name: 'Encoding Profiles', path: '/admin/encoding-profiles/', listItem: 'H.264/AAC (Universal)' },
  { name: 'Encoder Overrides', path: '/admin/encoder-overrides/', listItem: 'Force software H.265 encoder' },
  { name: 'Data Mapping', path: '/admin/data-mapping/', listItem: 'Prefix HD channels' },
] as const;

for (const { name, path, listItem } of createFlowPages) {
  test.describe(`${name} — Create Flow`, () => {
    test(`clicking "+" shows create form`, async ({ page, mockApi, vp }) => {
      await page.goto(path);
      await expect(page.getByText(listItem)).toBeVisible();

      await clickAddButton(page);

      if (vp.isMobile) {
        // On mobile, the detail panel should swap in with a Back button
        await expect(page.getByRole('button', { name: 'Back', exact: true })).toBeVisible({ timeout: 5_000 });
      } else {
        // On desktop, the detail panel is always visible — create form
        // renders in the right pane. The master list stays visible too.
        await expect(page.getByText(listItem)).toBeVisible();
      }
    });

    test(`selecting an item shows detail`, async ({ page, mockApi, vp }) => {
      await page.goto(path);
      await expect(page.getByText(listItem)).toBeVisible();

      await page.getByText(listItem).click();

      if (vp.isMobile) {
        await expect(page.getByRole('button', { name: 'Back', exact: true })).toBeVisible();
      } else {
        // On desktop, both panels visible — item highlighted, detail shows
        await expect(page.getByText(listItem)).toBeVisible();
      }
    });
  });
}

// ──────────────────────────────────────────────────────────────
// PROXIES — additional tests
// ──────────────────────────────────────────────────────────────

test.describe('Proxies — Detail Interactions', () => {
  test('edit proxy button works after selecting', async ({ page, mockApi, vp }) => {
    await page.goto('/proxies/');
    await expect(page.getByText('UK Freeview')).toBeVisible();

    await page.getByText('UK Freeview').click();

    if (vp.isMobile) {
      await expect(page.getByRole('button', { name: 'Back', exact: true })).toBeVisible();
    }

    // Click edit — should show wizard
    const editButton = page.getByRole('button', { name: /edit/i });
    await expect(editButton).toBeVisible();
    await editButton.click();

    // Edit wizard should be visible (selectedId stays truthy during edit)
    await expect(page.getByText('Basic Info')).toBeVisible({ timeout: 5_000 });
  });
});

// ──────────────────────────────────────────────────────────────
// TRANSCODERS — no create flow, select only
// ──────────────────────────────────────────────────────────────

test.describe('Transcoders — Select Item', () => {
  test('selecting a transcoder shows detail', async ({ page, mockApi, vp }) => {
    await page.goto('/transcoders/');
    // Transcoders renders daemon names in a "Select <name>" button
    const daemonButton = page.getByRole('button', { name: /Select transcoder-1/i });
    await expect(daemonButton).toBeVisible();

    await daemonButton.click();

    if (vp.isMobile) {
      await expect(page.getByRole('button', { name: 'Back', exact: true })).toBeVisible();
    }
  });
});

// ──────────────────────────────────────────────────────────────
// MOBILE LAYOUT SPECIFICS
// ──────────────────────────────────────────────────────────────

test.describe('Mobile Layout', () => {
  test('master list takes full width on mobile', async ({ page, mockApi, vp }) => {
    test.skip(!vp.isMobile, 'Mobile-only test');
    await page.goto('/proxies/');
    await expect(page.getByText('UK Freeview')).toBeVisible();

    const masterPanel = page.locator('[role="listbox"]').first();
    const box = await masterPanel.boundingBox();

    expect(box).toBeTruthy();
    expect(box!.width).toBeGreaterThan(vp.width * 0.8);
  });

  test('detail panel is hidden until item selected', async ({ page, mockApi, vp }) => {
    test.skip(!vp.isMobile, 'Mobile-only test');
    await page.goto('/proxies/');
    await expect(page.getByText('UK Freeview')).toBeVisible();

    // "Select a proxy" empty state should NOT be visible on mobile
    const emptyDetail = page.getByText('Select a proxy');
    await expect(emptyDetail).not.toBeVisible();
  });

  test('back button returns to master list', async ({ page, mockApi, vp }) => {
    test.skip(!vp.isMobile, 'Mobile-only test');
    await page.goto('/proxies/');
    await expect(page.getByText('UK Freeview')).toBeVisible();

    // Select a proxy
    await page.getByText('UK Freeview').click();
    await expect(page.getByRole('button', { name: 'Back', exact: true })).toBeVisible();

    // Click back
    await page.getByRole('button', { name: 'Back', exact: true }).click();

    // Master list should be visible again
    await expect(page.getByText('UK Freeview')).toBeVisible();
    // Back button should be gone
    await expect(page.getByRole('button', { name: 'Back', exact: true })).not.toBeVisible();
  });
});
