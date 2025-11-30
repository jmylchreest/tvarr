import { test, expect, Page } from '@playwright/test';

const BACKEND_URL = 'http://localhost:8081';
const FRONTEND_URL = 'http://localhost:3000';

// Helper function to wait for page to be stable
async function waitForPageStable(page: Page) {
  // Wait for any loading spinners to disappear
  await page
    .waitForFunction(
      () => {
        const spinners = document.querySelectorAll('[data-testid="spinner"], .animate-spin');
        return spinners.length === 0;
      },
      { timeout: 10000 }
    )
    .catch(() => {
      // If no spinners found or timeout, continue
    });

  // Wait for network to be idle
  await page.waitForLoadState('networkidle', { timeout: 10000 }).catch(() => {
    // Continue if timeout
  });

  // Small additional wait for stability
  await page.waitForTimeout(1000);
}

// Helper function to capture console errors
async function setupConsoleCapture(
  page: Page
): Promise<{ logs: string[]; errors: string[]; networkErrors: string[] }> {
  const logs: string[] = [];
  const errors: string[] = [];
  const networkErrors: string[] = [];

  page.on('console', (msg) => {
    const text = `[${msg.type().toUpperCase()}] ${msg.text()}`;
    logs.push(text);

    if (msg.type() === 'error') {
      errors.push(text);
    }
  });

  page.on('response', (response) => {
    if (response.status() >= 400) {
      networkErrors.push(`${response.status()} ${response.url()}`);
    }
  });

  page.on('pageerror', (error) => {
    errors.push(`Page Error: ${error.message}`);
  });

  return { logs, errors, networkErrors };
}

// Helper to check backend health
async function checkBackendHealth(page: Page): Promise<boolean> {
  try {
    const response = await page.request.get(`${BACKEND_URL}/api/v1/health`);
    return response.ok();
  } catch {
    return false;
  }
}

test.describe('Data Mapping Rule Validation Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Check if backend is running
    const backendHealthy = await checkBackendHealth(page);
    if (!backendHealthy) {
      console.warn('Backend service not responding at http://localhost:8081');
      // Continue with test to capture frontend behavior
    }

    // Navigate to the data mapping page
    await page.goto('/admin/data-mapping');

    // Wait for the page to stabilize
    await waitForPageStable(page);
  });

  test('should navigate to data mapping rules page successfully', async ({ page }) => {
    const consoleCapture = await setupConsoleCapture(page);

    // Take initial screenshot
    await page.screenshot({ path: 'test-results/01-navigation-success.png', fullPage: true });

    // Verify we're on the correct page
    await expect(page).toHaveURL(/.*\/admin\/data-mapping/);

    // Verify main page elements are present
    await expect(page.locator('text=Data Mapping Rules')).toBeVisible({ timeout: 10000 });

    // Check for Create Data Mapping Rule button
    const createButton = page.locator('button:has-text("Create Data Mapping Rule")');
    await expect(createButton).toBeVisible({ timeout: 10000 });

    // Look for any statistics cards
    const totalRulesCard = page.locator('text=Total Rules').first();
    await expect(totalRulesCard).toBeVisible({ timeout: 10000 });

    console.log('Navigation test completed successfully');
    console.log(`Console logs: ${consoleCapture.logs.length} total`);
    console.log(`Console errors: ${consoleCapture.errors.length} total`);
    console.log(`Network errors: ${consoleCapture.networkErrors.length} total`);

    if (consoleCapture.errors.length > 0) {
      console.log('Console errors found:', consoleCapture.errors);
    }

    if (consoleCapture.networkErrors.length > 0) {
      console.log('Network errors found:', consoleCapture.networkErrors);
    }
  });

  test('should open create data mapping rule dialog', async ({ page }) => {
    const consoleCapture = await setupConsoleCapture(page);

    // Click the create button
    const createButton = page.locator('button:has-text("Create Data Mapping Rule")');
    await expect(createButton).toBeVisible({ timeout: 10000 });

    await createButton.click();

    // Wait for dialog to open
    await page.waitForTimeout(1000);

    // Take screenshot of the dialog
    await page.screenshot({ path: 'test-results/02-create-dialog-opened.png', fullPage: true });

    // Verify dialog is open
    const dialogTitle = page.locator('text=Create Data Mapping Rule');
    await expect(dialogTitle).toBeVisible({ timeout: 10000 });

    // Check for form fields
    await expect(page.locator('input[id="name"]')).toBeVisible();
    await expect(page.locator('select[id="source_type"]')).toBeVisible();
    await expect(page.locator('textarea[id="description"]')).toBeVisible();

    // Look for expression editor
    const expressionEditor = page.locator('textarea').first();
    await expect(expressionEditor).toBeVisible();

    console.log('Create dialog test completed');
    console.log(`Console errors: ${consoleCapture.errors.length} total`);
    console.log(`Network errors: ${consoleCapture.networkErrors.length} total`);

    if (consoleCapture.errors.length > 0) {
      console.log('Console errors found:', consoleCapture.errors);
    }
  });

  test('should test expression validation workflow', async ({ page }) => {
    const consoleCapture = await setupConsoleCapture(page);

    // Open create dialog
    const createButton = page.locator('button:has-text("Create Data Mapping Rule")');
    await createButton.click();
    await page.waitForTimeout(1000);

    // Fill in basic information
    await page.fill('input[id="name"]', 'Test Validation Rule');

    // Select source type
    await page.selectOption('select[id="source_type"]', 'stream');

    await page.fill('textarea[id="description"]', 'Testing expression validation functionality');

    // Take screenshot before entering expression
    await page.screenshot({ path: 'test-results/03-before-expression-entry.png', fullPage: true });

    // Find the expression editor textarea
    const expressionEditor = page.locator('textarea').first();
    await expect(expressionEditor).toBeVisible();

    // Test with a simple valid expression
    console.log('Testing with valid expression...');
    await expressionEditor.fill('channel_name = "Test " + channel_name');

    // Wait for validation to process
    await page.waitForTimeout(3000);

    // Take screenshot after entering expression
    await page.screenshot({ path: 'test-results/04-after-valid-expression.png', fullPage: true });

    // Look for validation badges
    const validationBadges = page.locator('[class*="badge"]');
    const badgeCount = await validationBadges.count();
    console.log(`Found ${badgeCount} validation badges`);

    // Check for specific validation states
    const expressionBadge = page.locator('text=Expression').first();
    const syntaxBadge = page.locator('text=Syntax').first();
    const fieldsBadge = page.locator('text=Fields').first();

    // Wait a bit longer for any async validation
    await page.waitForTimeout(5000);

    // Take another screenshot to capture final state
    await page.screenshot({ path: 'test-results/05-validation-complete.png', fullPage: true });

    // Try an invalid expression to test error handling
    console.log('Testing with invalid expression...');
    await expressionEditor.fill('invalid syntax here $#@!');

    // Wait for validation to process the invalid expression
    await page.waitForTimeout(3000);

    await page.screenshot({ path: 'test-results/06-invalid-expression.png', fullPage: true });

    // Check if validation detected the error
    const errorIndicators = page.locator(
      '[class*="red"], [class*="error"], [class*="destructive"]'
    );
    const errorCount = await errorIndicators.count();
    console.log(`Found ${errorCount} error indicators`);

    console.log('Expression validation test completed');
    console.log(`Console errors: ${consoleCapture.errors.length} total`);
    console.log(`Network errors: ${consoleCapture.networkErrors.length} total`);

    if (consoleCapture.errors.length > 0) {
      console.log('Console errors found:', consoleCapture.errors);
    }

    if (consoleCapture.networkErrors.length > 0) {
      console.log('Network errors found:', consoleCapture.networkErrors);
    }

    // Check if the validation process seemed to hang
    const submitButton = page.locator('button[type="submit"]');
    const isSubmitDisabled = await submitButton.isDisabled();
    console.log(`Submit button is disabled: ${isSubmitDisabled}`);

    // Look for loading indicators
    const loadingSpinners = page.locator('[class*="animate-spin"], [class*="loader"]');
    const spinnerCount = await loadingSpinners.count();
    console.log(`Found ${spinnerCount} loading spinners`);

    // Test if we can interact with the form (not hanging)
    const nameField = page.locator('input[id="name"]');
    await nameField.fill('Modified Test Rule');
    const nameValue = await nameField.inputValue();
    console.log(`Name field is interactive: ${nameValue === 'Modified Test Rule'}`);

    return {
      consoleErrors: consoleCapture.errors,
      networkErrors: consoleCapture.networkErrors,
      isHanging: spinnerCount > 0 && isSubmitDisabled,
      canInteract: nameValue === 'Modified Test Rule',
    };
  });

  test('should test editing existing rule (if any exist)', async ({ page }) => {
    const consoleCapture = await setupConsoleCapture(page);

    // Wait for page to load completely
    await waitForPageStable(page);

    // Look for existing rules
    const editButtons = page.locator(
      'button:has([class*="edit"], [data-testid="edit"]), button[title*="edit" i]'
    );
    const editButtonCount = await editButtons.count();

    console.log(`Found ${editButtonCount} edit buttons`);

    if (editButtonCount === 0) {
      console.log('No existing rules found to edit, skipping edit test');
      return;
    }

    // Click the first edit button
    await editButtons.first().click();
    await page.waitForTimeout(1000);

    // Take screenshot of edit dialog
    await page.screenshot({ path: 'test-results/07-edit-dialog-opened.png', fullPage: true });

    // Look for edit dialog
    const editDialogTitle = page.locator('text=Edit Data Mapping Rule');
    const isEditDialogVisible = await editDialogTitle.isVisible();

    if (isEditDialogVisible) {
      console.log('Edit dialog opened successfully');

      // Find the expression editor in edit mode
      const expressionEditor = page.locator('textarea').first();
      await expect(expressionEditor).toBeVisible();

      // Get current value and modify it
      const currentValue = await expressionEditor.inputValue();
      console.log(`Current expression value: ${currentValue}`);

      // Modify the expression to trigger validation
      await expressionEditor.fill('channel_name = "Edited " + channel_name');

      // Wait for validation
      await page.waitForTimeout(5000);

      await page.screenshot({ path: 'test-results/08-edit-validation.png', fullPage: true });

      // Check for hanging behavior
      const loadingSpinners = page.locator('[class*="animate-spin"], [class*="loader"]');
      const spinnerCount = await loadingSpinners.count();
      console.log(`Found ${spinnerCount} loading spinners in edit mode`);

      // Check if we can still interact with form
      const nameField = page.locator('input[id*="name"]').first();
      const canEditName = await nameField.isEnabled();
      console.log(`Name field is editable: ${canEditName}`);

      // Look for update button
      const updateButton = page.locator('button:has-text("Update"), button[type="submit"]').first();
      const isUpdateDisabled = await updateButton.isDisabled();
      console.log(`Update button is disabled: ${isUpdateDisabled}`);

      return {
        editDialogOpened: true,
        canEditFields: canEditName,
        isHanging: spinnerCount > 0 && isUpdateDisabled,
        consoleErrors: consoleCapture.errors,
        networkErrors: consoleCapture.networkErrors,
      };
    } else {
      console.log('Edit dialog did not open');
      return {
        editDialogOpened: false,
        consoleErrors: consoleCapture.errors,
        networkErrors: consoleCapture.networkErrors,
      };
    }
  });

  test('should monitor network connectivity and API responses', async ({ page }) => {
    const consoleCapture = await setupConsoleCapture(page);

    // Monitor network requests
    const requests: string[] = [];
    const responses: { url: string; status: number; ok: boolean }[] = [];

    page.on('request', (request) => {
      if (request.url().includes('/api/')) {
        requests.push(`${request.method()} ${request.url()}`);
      }
    });

    page.on('response', (response) => {
      if (response.url().includes('/api/')) {
        responses.push({
          url: response.url(),
          status: response.status(),
          ok: response.ok(),
        });
      }
    });

    // Trigger some actions that should make API calls
    await waitForPageStable(page);

    // Open create dialog which might fetch field information
    const createButton = page.locator('button:has-text("Create Data Mapping Rule")');
    await createButton.click();
    await page.waitForTimeout(2000);

    // Change source type to trigger field loading
    await page.selectOption('select[id="source_type"]', 'stream');
    await page.waitForTimeout(2000);
    await page.selectOption('select[id="source_type"]', 'epg');
    await page.waitForTimeout(2000);

    // Try entering an expression to trigger validation
    const expressionEditor = page.locator('textarea').first();
    await expressionEditor.fill('test_expression = "test"');
    await page.waitForTimeout(5000);

    await page.screenshot({ path: 'test-results/09-network-monitoring.png', fullPage: true });

    console.log(`API Requests made: ${requests.length}`);
    requests.forEach((req) => console.log(`  ${req}`));

    console.log(`API Responses received: ${responses.length}`);
    responses.forEach((resp) => console.log(`  ${resp.status} ${resp.url} (ok: ${resp.ok})`));

    const failedRequests = responses.filter((r) => !r.ok);
    console.log(`Failed API requests: ${failedRequests.length}`);

    return {
      totalRequests: requests.length,
      totalResponses: responses.length,
      failedRequests: failedRequests.length,
      consoleErrors: consoleCapture.errors,
      networkErrors: consoleCapture.networkErrors,
    };
  });
});
