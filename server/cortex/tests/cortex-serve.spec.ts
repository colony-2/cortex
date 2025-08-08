import { test, expect } from '@playwright/test';

test.describe('Cortex Serve', () => {
  test('should load without errors', async ({ page }) => {
    // Capture console errors
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    // Capture page errors
    page.on('pageerror', err => {
      errors.push(err.message);
    });

    // Navigate to cortex
    await page.goto('http://localhost:8080/boxes', { waitUntil: 'networkidle' });

    // Check that there are no JavaScript errors
    if (errors.length > 0) {
      console.error('JavaScript errors found:', errors);
    }
    expect(errors, `Found ${errors.length} JavaScript errors: ${errors.join(', ')}`).toHaveLength(0);

    // Wait for the React Flow container to be visible
    await expect(page.locator('.react-flow')).toBeVisible({ timeout: 10000 });

    // Check that API is working by waiting for cells
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Verify cells are rendered
    const cells = page.locator('.react-flow__node');
    const cellCount = await cells.count();
    expect(cellCount, 'Should have rendered cells').toBeGreaterThan(0);

    // Check that the API endpoints are accessible
    const graphResponse = await page.request.get('http://localhost:8080/api/graph');
    expect(graphResponse.ok(), 'Graph API should return 200').toBe(true);
    
    const graphData = await graphResponse.json();
    expect(graphData.cells, 'Graph should have cells').toBeDefined();
    expect(graphData.cells.length, 'Graph should have multiple cells').toBeGreaterThan(0);
  });

  test('should display cells correctly', async ({ page }) => {
    await page.goto('http://localhost:8080/cells');
    
    // Wait for cells to load
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Check that at least one cell is visible
    const firstCell = page.locator('.react-flow__node').first();
    await expect(firstCell).toBeVisible();
    
    // Check cell contains expected elements
    const cellText = await firstCell.textContent();
    expect(cellText).toBeTruthy();
  });
});