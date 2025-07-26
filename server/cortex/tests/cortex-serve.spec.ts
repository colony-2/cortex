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

    // Check that API is working by waiting for nodes
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Verify nodes are rendered
    const nodes = page.locator('.react-flow__node');
    const nodeCount = await nodes.count();
    expect(nodeCount, 'Should have rendered nodes').toBeGreaterThan(0);

    // Check that the API endpoints are accessible
    const graphResponse = await page.request.get('http://localhost:8080/api/graph');
    expect(graphResponse.ok(), 'Graph API should return 200').toBe(true);
    
    const graphData = await graphResponse.json();
    expect(graphData.nodes, 'Graph should have nodes').toBeDefined();
    expect(graphData.nodes.length, 'Graph should have multiple nodes').toBeGreaterThan(0);
  });

  test('should display nodes correctly', async ({ page }) => {
    await page.goto('http://localhost:8080/boxes');
    
    // Wait for nodes to load
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Check that at least one node is visible
    const firstNode = page.locator('.react-flow__node').first();
    await expect(firstNode).toBeVisible();
    
    // Check node contains expected elements
    const nodeText = await firstNode.textContent();
    expect(nodeText).toBeTruthy();
  });
});