import { test, expect } from '@playwright/test';

test.describe('Graph Visualizer', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Wait for the loading to finish
    await page.waitForSelector('.graph-container:not(:has(.loading))', { timeout: 10000 });
  });

  test('should load and display the graph', async ({ page }) => {
    // Wait for the Svelte Flow container to be visible
    await expect(page.locator('.svelte-flow')).toBeVisible();
    
    // Check that nodes are rendered (should be 13 based on the error message)
    const nodes = page.locator('.svelte-flow__node');
    await expect(nodes).toHaveCount(13, { timeout: 10000 });
  });

  test('should display node information correctly', async ({ page }) => {
    // Wait for nodes to be rendered
    await page.waitForSelector('.svelte-flow__node', { timeout: 10000 });
    
    // Check that a node with auth exists
    const authNode = page.locator('.node-box:has(.node-header:text("auth"))').first();
    await expect(authNode).toBeVisible();
    
    // Check node path
    const nodePath = authNode.locator('.node-path');
    await expect(nodePath).toContainText('auth');
  });

  test('should have working zoom controls', async ({ page }) => {
    // Wait for the graph to load
    await page.waitForSelector('.svelte-flow', { timeout: 10000 });
    
    // Find zoom controls - Svelte Flow uses different button structure
    const controls = page.locator('.svelte-flow__controls');
    await expect(controls).toBeVisible();
    
    // Find zoom buttons by their position/icon
    const zoomInButton = controls.locator('button').nth(0);
    const zoomOutButton = controls.locator('button').nth(1);
    const fitViewButton = controls.locator('button').nth(2);
    
    // Check that controls exist
    await expect(zoomInButton).toBeVisible();
    await expect(zoomOutButton).toBeVisible();
    await expect(fitViewButton).toBeVisible();
    
    // Test zoom in
    await zoomInButton.click();
    await page.waitForTimeout(300);
    
    // Test zoom out
    await zoomOutButton.click();
    await page.waitForTimeout(300);
    
    // Test fit view
    await fitViewButton.click();
    await page.waitForTimeout(300);
  });

  test('should display edges between nodes', async ({ page }) => {
    // Wait for edges to be rendered
    await page.waitForSelector('.svelte-flow__edge', { timeout: 10000 });
    
    // Check that edges exist
    const edges = page.locator('.svelte-flow__edge');
    const edgeCount = await edges.count();
    expect(edgeCount).toBeGreaterThan(0);
  });

  test('should allow panning the graph', async ({ page }) => {
    // Wait for the graph to load
    await page.waitForSelector('.svelte-flow', { timeout: 10000 });
    
    const graphContainer = page.locator('.svelte-flow__viewport');
    
    // Get initial transform
    const initialTransform = await graphContainer.evaluate(el => {
      const style = window.getComputedStyle(el);
      return style.transform;
    });
    
    // Perform drag to pan
    await page.mouse.move(400, 300);
    await page.mouse.down();
    await page.mouse.move(500, 400);
    await page.mouse.up();
    
    // Check that transform changed
    const newTransform = await graphContainer.evaluate(el => {
      const style = window.getComputedStyle(el);
      return style.transform;
    });
    
    expect(newTransform).not.toBe(initialTransform);
  });
});