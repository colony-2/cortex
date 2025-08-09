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

    // Navigate to cortex - /boxes redirects to /cells
    await page.goto('http://localhost:8080/cells', { waitUntil: 'networkidle' });

    // Check that there are no JavaScript errors
    if (errors.length > 0) {
      console.error('JavaScript errors found:', errors);
    }
    expect(errors, `Found ${errors.length} JavaScript errors: ${errors.join(', ')}`).toHaveLength(0);

    // Wait for the React Flow container to be visible
    await expect(page.locator('.react-flow')).toBeVisible({ timeout: 10000 });

    // Wait for React Flow to initialize and render nodes
    // The nodes might exist in DOM but not be visible until React Flow positions them
    await page.waitForFunction(
      () => {
        const nodes = document.querySelectorAll('.react-flow__node');
        if (nodes.length === 0) return false;
        // Check if at least one node has non-zero dimensions
        for (const node of nodes) {
          const rect = (node as HTMLElement).getBoundingClientRect();
          if (rect.width > 0 && rect.height > 0) return true;
        }
        return false;
      },
      { timeout: 10000 }
    );
    
    // Verify cells are rendered and visible
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
    
    // Wait for React Flow to initialize and render nodes with proper dimensions
    await page.waitForFunction(
      () => {
        const nodes = document.querySelectorAll('.react-flow__node');
        if (nodes.length === 0) return false;
        // Check if at least one node has non-zero dimensions
        for (const node of nodes) {
          const rect = (node as HTMLElement).getBoundingClientRect();
          if (rect.width > 0 && rect.height > 0) return true;
        }
        return false;
      },
      { timeout: 10000 }
    );
    
    // Check that at least one cell is rendered with content
    const firstCell = page.locator('.react-flow__node').first();
    const cellText = await firstCell.textContent();
    expect(cellText).toBeTruthy();
    
    // Verify the cell has proper dimensions
    const boundingBox = await firstCell.boundingBox();
    expect(boundingBox).not.toBeNull();
    if (boundingBox) {
      expect(boundingBox.width).toBeGreaterThan(0);
      expect(boundingBox.height).toBeGreaterThan(0);
    }
  });
});