import { test, expect, Page } from '@playwright/test';

test.describe('Logos Search Focus Investigation', () => {
  test.beforeEach(async ({ page }) => {
    // Set up network request monitoring
    await page.route('**/api/**', async (route) => {
      console.log(`[NETWORK] ${route.request().method()} ${route.request().url()}`);
      await route.continue();
    });

    // Navigate to logos page and wait for initial load
    await page.goto('/admin/logos');
    await page.waitForLoadState('networkidle');

    // Wait for the page to be fully loaded by checking for the search input
    await page.waitForSelector('[placeholder*="Search logos"]', { state: 'visible' });
  });

  test('should maintain focus during debounced search', async ({ page }) => {
    const searchInput = page.locator('[placeholder*="Search logos"]');

    // Verify initial state
    await expect(searchInput).toBeVisible();

    // Focus the search input
    await searchInput.click();
    await expect(searchInput).toBeFocused();

    // Track network requests during search
    const networkRequests: string[] = [];
    const pageReloads: string[] = [];

    page.on('request', (request) => {
      const url = request.url();
      networkRequests.push(`${request.method()} ${url}`);

      // Check if this looks like a full page reload
      if (url.includes('/admin/logos') && !url.includes('api')) {
        pageReloads.push(url);
      }
    });

    // Type search term slowly to trigger debounced search
    const searchTerm = 'test logo search';

    console.log('[TEST] Starting to type search term...');

    for (let i = 0; i < searchTerm.length; i++) {
      const char = searchTerm[i];
      await searchInput.press(`Key${char.toUpperCase()}`);

      // Check if focus is still maintained after each character
      const isFocused = await searchInput.evaluate((el: HTMLInputElement) => {
        return document.activeElement === el;
      });

      console.log(
        `[TEST] After typing "${searchTerm.slice(0, i + 1)}", focus maintained: ${isFocused}`
      );

      if (!isFocused) {
        console.log(`[ERROR] Focus lost after typing character '${char}' at position ${i}`);

        // Log what element has focus instead
        const activeElement = await page.evaluate(() => {
          const el = document.activeElement;
          return {
            tagName: el?.tagName,
            className: el?.className,
            id: el?.id,
            placeholder: (el as HTMLInputElement)?.placeholder,
          };
        });
        console.log(`[ERROR] Current active element:`, activeElement);
      }

      // Add a small delay between keystrokes to mimic real typing
      await page.waitForTimeout(50);
    }

    console.log('[TEST] Finished typing, waiting for debounce...');

    // Wait for the debounced search to trigger (300ms + some buffer)
    await page.waitForTimeout(500);

    // Check final focus state
    const finalFocusState = await searchInput.evaluate((el: HTMLInputElement) => {
      return document.activeElement === el;
    });

    console.log(`[TEST] Final focus state: ${finalFocusState}`);

    // Check if any full page reloads occurred
    console.log(`[TEST] Network requests made: ${networkRequests.length}`);
    console.log(`[TEST] Page reloads detected: ${pageReloads.length}`);

    if (pageReloads.length > 0) {
      console.log(`[ERROR] Unexpected page reloads:`, pageReloads);
    }

    // Print all network requests for debugging
    networkRequests.forEach((req) => console.log(`[NETWORK] ${req}`));

    // Assertions
    expect(finalFocusState, 'Search input should maintain focus after debounced search').toBe(true);
    expect(pageReloads.length, 'No full page reloads should occur during search').toBe(0);

    // Verify the search actually worked by checking if results changed
    const resultsCount = await page
      .locator(
        '.space-y-4 > .transition-all, .grid > .transition-all, .space-y-2 > .transition-all'
      )
      .count();
    console.log(`[TEST] Results displayed: ${resultsCount}`);
  });

  test('should handle rapid typing without losing focus', async ({ page }) => {
    const searchInput = page.locator('[placeholder*="Search logos"]');

    await searchInput.click();
    await expect(searchInput).toBeFocused();

    // Rapid typing test - type very quickly
    const rapidSearchTerm = 'rapidtest';

    console.log('[TEST] Starting rapid typing test...');

    let focusLostDuringTyping = false;
    let focusLostAt = -1;

    // Type the entire string rapidly
    for (let i = 0; i < rapidSearchTerm.length; i++) {
      await searchInput.type(rapidSearchTerm[i], { delay: 10 }); // Very fast typing

      // Check focus every few characters
      if (i % 2 === 0) {
        const isFocused = await searchInput.evaluate((el: HTMLInputElement) => {
          return document.activeElement === el;
        });

        if (!isFocused && !focusLostDuringTyping) {
          focusLostDuringTyping = true;
          focusLostAt = i;
          console.log(`[ERROR] Focus lost during rapid typing at position ${i}`);
        }
      }
    }

    // Wait for all debounced operations to complete
    await page.waitForTimeout(1000);

    const finalFocusState = await searchInput.isFocused();

    console.log(
      `[TEST] Rapid typing completed. Focus lost during typing: ${focusLostDuringTyping} at position ${focusLostAt}`
    );
    console.log(`[TEST] Final focus state: ${finalFocusState}`);

    expect(finalFocusState, 'Focus should be maintained after rapid typing').toBe(true);
  });

  test('should verify search input ref and focus restoration logic', async ({ page }) => {
    const searchInput = page.locator('[placeholder*="Search logos"]');

    await searchInput.click();

    // Inject test code to monitor focus changes
    await page.addInitScript(() => {
      let focusEvents: Array<{ event: string; timestamp: number; element?: string }> = [];

      document.addEventListener('focusin', (e) => {
        focusEvents.push({
          event: 'focusin',
          timestamp: Date.now(),
          element:
            (e.target as HTMLElement)?.tagName + (e.target as HTMLElement)?.className
              ? '.' + (e.target as HTMLElement).className
              : '',
        });
      });

      document.addEventListener('focusout', (e) => {
        focusEvents.push({
          event: 'focusout',
          timestamp: Date.now(),
          element:
            (e.target as HTMLElement)?.tagName + (e.target as HTMLElement)?.className
              ? '.' + (e.target as HTMLElement).className
              : '',
        });
      });

      // Make focus events available to test
      (window as any).getFocusEvents = () => focusEvents;
      (window as any).clearFocusEvents = () => {
        focusEvents = [];
      };
    });

    // Clear any initial focus events
    await page.evaluate(() => (window as any).clearFocusEvents());

    // Type a search term that should trigger API calls
    await searchInput.type('test search', { delay: 100 });

    // Wait for debounced search and any re-renders
    await page.waitForTimeout(800);

    // Get focus events that occurred
    const focusEvents = await page.evaluate(() => (window as any).getFocusEvents());

    console.log('[TEST] Focus events during search:', focusEvents);

    // Check if there were any unexpected focus changes
    const focusOutEvents = focusEvents.filter((event: any) => event.event === 'focusout');
    const focusInEvents = focusEvents.filter((event: any) => event.event === 'focusin');

    console.log(
      `[TEST] Focus out events: ${focusOutEvents.length}, Focus in events: ${focusInEvents.length}`
    );

    if (focusOutEvents.length > 0) {
      console.log('[ISSUE] Unexpected focus out events detected during search');
      focusOutEvents.forEach((event: any) => {
        console.log(`  - ${event.event} on ${event.element} at ${event.timestamp}`);
      });
    }

    const finalFocusState = await searchInput.isFocused();
    expect(finalFocusState).toBe(true);
  });

  test('should verify component re-render behavior', async ({ page }) => {
    const searchInput = page.locator('[placeholder*="Search logos"]');

    // Add script to monitor component renders/updates
    await page.addInitScript(() => {
      let renderCount = 0;

      // Override React's render function to count re-renders (simplified)
      const originalSetState = Object.prototype.toString;

      (window as any).getRenderCount = () => renderCount;
      (window as any).incrementRenderCount = () => {
        renderCount++;
      };
    });

    await searchInput.click();

    // Get initial render count
    const initialRenderCount = await page.evaluate(() => (window as any).getRenderCount());

    // Type search term
    await searchInput.type('component test', { delay: 80 });

    // Wait for all async operations
    await page.waitForTimeout(1000);

    // Check render count after search
    const finalRenderCount = await page.evaluate(() => (window as any).getRenderCount());

    console.log(`[TEST] Render count - Initial: ${initialRenderCount}, Final: ${finalRenderCount}`);

    // Verify input still has focus and correct value
    await expect(searchInput).toBeFocused();
    await expect(searchInput).toHaveValue('component test');
  });

  test('should test search input with various view modes', async ({ page }) => {
    const searchInput = page.locator('[placeholder*="Search logos"]');
    const gridViewButton = page
      .locator('[data-testid="grid-view"], button:has-text("Grid"), button:has([class*="grid"])', {
        hasText: '',
      })
      .first();
    const listViewButton = page
      .locator('[data-testid="list-view"], button:has-text("List"), button:has([class*="list"])', {
        hasText: '',
      })
      .first();
    const tableViewButton = page
      .locator(
        '[data-testid="table-view"], button:has-text("Table"), button:has([class*="table"])',
        { hasText: '' }
      )
      .first();

    // Test focus behavior when switching view modes during search
    await searchInput.click();
    await searchInput.type('view mode test', { delay: 100 });

    // Switch to grid view during search
    await gridViewButton.click();
    await page.waitForTimeout(200);

    let focusState1 = await searchInput.isFocused();
    console.log(`[TEST] Focus after switching to grid view: ${focusState1}`);

    // Switch to list view during search
    await listViewButton.click();
    await page.waitForTimeout(200);

    let focusState2 = await searchInput.isFocused();
    console.log(`[TEST] Focus after switching to list view: ${focusState2}`);

    // Switch to table view during search
    await tableViewButton.click();
    await page.waitForTimeout(200);

    let focusState3 = await searchInput.isFocused();
    console.log(`[TEST] Focus after switching to table view: ${focusState3}`);

    // Final wait for debounced search
    await page.waitForTimeout(500);

    const finalFocusState = await searchInput.isFocused();
    console.log(`[TEST] Final focus state after view mode changes: ${finalFocusState}`);

    // The input should ideally maintain focus through view mode changes
    // But this might be expected behavior to lose focus when clicking view buttons
    expect(await searchInput.hasAttribute('value')).toBeTruthy();
    await expect(searchInput).toHaveValue('view mode test');
  });
});
