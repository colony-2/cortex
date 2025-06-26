import { test, expect } from '@playwright/test';

test.describe('URL State Management', () => {
  test('should update URL when selecting a node', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    // Click on a node
    const node = await page.locator('.react-flow__node').first();
    const nodeId = await node.getAttribute('data-node-id');
    await node.click();
    
    // Check URL contains node parameter
    await expect(page).toHaveURL(new RegExp(`\\?.*node=${nodeId}`));
  });

  test('should update URL when changing tabs', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load and select a node
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.locator('.react-flow__node').first().click();
    
    // Wait for side panel
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Click on Changes tab
    await page.getByText('Changes').click();
    
    // Check URL contains tab parameter
    await expect(page).toHaveURL(/\?.*tab=changes/);
  });

  test('should update URL when changing subtabs in Changes', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load and select a node
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.locator('.react-flow__node').first().click();
    
    // Navigate to Changes tab
    await page.getByText('Changes').click();
    
    // Click on History subtab
    await page.getByRole('tab', { name: 'History' }).click();
    
    // Check URL contains subtab parameter
    await expect(page).toHaveURL(/\?.*subtab=history/);
  });

  test('should restore state from URL on page load', async ({ page }) => {
    // First, navigate and select a node to get its ID
    await page.goto('/');
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    const node = await page.locator('.react-flow__node').first();
    const nodeId = await node.getAttribute('data-node-id');
    
    // Navigate directly with URL parameters
    await page.goto(`/?node=${nodeId}&tab=changes&subtab=history`);
    
    // Wait for the page to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    // Check that the node is selected (has blue border)
    const selectedNode = await page.locator(`.react-flow__node[data-node-id="${nodeId}"]`);
    const borderStyle = await selectedNode.locator('.graph-node').evaluate((el) => {
      return window.getComputedStyle(el).border;
    });
    expect(borderStyle).toContain('2px'); // Selected nodes have 2px border
    
    // Check that Changes tab is active
    const changesTab = await page.locator('.ant-tabs-tab-active').textContent();
    expect(changesTab).toContain('Changes');
    
    // Check that History subtab is active
    const historySubtab = await page.locator('.ant-tabs-tab-active').last().textContent();
    expect(historySubtab).toContain('History');
  });

  test('should preserve URL state on page reload', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load and select a node
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    const node = await page.locator('.react-flow__node').first();
    const nodeId = await node.getAttribute('data-node-id');
    await node.click();
    
    // Navigate to Changes tab and History subtab
    await page.getByText('Changes').click();
    await page.getByRole('tab', { name: 'History' }).click();
    
    // Get current URL
    const urlBefore = page.url();
    
    // Reload the page
    await page.reload();
    
    // Wait for page to load again
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    // Check URL is preserved
    expect(page.url()).toBe(urlBefore);
    
    // Check state is restored
    const selectedNode = await page.locator(`.react-flow__node[data-node-id="${nodeId}"]`);
    const borderStyle = await selectedNode.locator('.graph-node').evaluate((el) => {
      return window.getComputedStyle(el).border;
    });
    expect(borderStyle).toContain('2px');
    
    // Check tabs are restored
    const changesTab = await page.locator('.ant-tabs-tab-active').textContent();
    expect(changesTab).toContain('Changes');
  });
});