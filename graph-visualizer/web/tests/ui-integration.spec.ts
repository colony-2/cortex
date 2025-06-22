import { test, expect } from '@playwright/test';

test.describe('UI Integration Tests', () => {
  test('should open file browser when clicking action button', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load
    await page.waitForSelector('.svelte-flow', { timeout: 10000 });
    
    // Click on a node to select it
    const firstNode = page.locator('.svelte-flow__node').first();
    await firstNode.click();
    
    // Verify node is selected
    await expect(page.locator('.node-box.selected')).toHaveCount(1);
    
    // Click file browser button
    const fileBrowserBtn = page.locator('.node-box.selected .action-btn').first();
    await fileBrowserBtn.click();
    
    // Verify file browser opens
    await expect(page.locator('.file-browser')).toBeVisible();
    
    // Verify split screen is active
    await expect(page.locator('.app-container.split')).toHaveCount(1);
    
    // Verify browser header shows node name
    const browserHeader = page.locator('.browser-header h3');
    await expect(browserHeader).toBeVisible();
    
    // Close file browser
    const closeBtn = page.locator('.browser-header .action-btn');
    await closeBtn.click();
    
    // Verify file browser is closed
    await expect(page.locator('.file-browser')).not.toBeVisible();
  });
  
  test('should highlight parent and child edges on node selection', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load
    await page.waitForSelector('.svelte-flow', { timeout: 10000 });
    
    // Find a node with dependencies (e.g., 'api' node)
    const apiNode = page.locator('.svelte-flow__node').filter({ hasText: 'api' }).first();
    await apiNode.click();
    
    // Check that some edges have changed color
    const edges = page.locator('.svelte-flow__edge');
    const edgeCount = await edges.count();
    
    // Check for colored edges by looking for parent and child edge classes
    const parentEdgeCount = await page.locator('.parent-edge .svelte-flow__edge-path').count();
    const childEdgeCount = await page.locator('.child-edge .svelte-flow__edge-path').count();
    const coloredEdges = parentEdgeCount + childEdgeCount;
    
    // Also verify the colors are correct
    if (coloredEdges > 0) {
      // Check a parent edge color
      if (parentEdgeCount > 0) {
        const parentPath = page.locator('.parent-edge .svelte-flow__edge-path').first();
        const parentStyle = await parentPath.evaluate(el => window.getComputedStyle(el).stroke);
        expect(parentStyle).toBe('rgb(76, 175, 80)'); // Green
      }
      
      // Check a child edge color
      if (childEdgeCount > 0) {
        const childPath = page.locator('.child-edge .svelte-flow__edge-path').first();
        const childStyle = await childPath.evaluate(el => window.getComputedStyle(el).stroke);
        expect(childStyle).toBe('rgb(33, 150, 243)'); // Blue
      }
    }
    
    // Should have at least some colored edges
    expect(coloredEdges).toBeGreaterThan(0);
  });
});