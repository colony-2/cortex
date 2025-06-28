import { test, expect } from '@playwright/test';

test.describe('UI Integration Tests', () => {
  test('should show file browser in side panel when clicking node', async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Wait for nodes to be rendered
    await page.waitForTimeout(1000);
    
    // Trigger node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Wait for state to update
    await page.waitForTimeout(100);
    
    // Check that Files tab is active by default when a node is selected
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Check that file list is visible
    await expect(page.locator('.ant-list')).toBeVisible();
    
    // Verify file items are loaded
    const fileItems = page.locator('.ant-list-item');
    const itemCount = await fileItems.count();
    expect(itemCount).toBeGreaterThan(0);
  });
  
  test('should switch between nodes and update file browser', async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Click on api node
    const apiNode = await page.locator('.react-flow__node').filter({ hasText: 'api' }).first();
    await apiNode.click();
    
    // Wait for files to load
    await page.waitForTimeout(1000);
    
    // Files tab should be active by default when a node is selected
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Check that api files are shown
    await expect(page.locator('.ant-list-item').filter({ hasText: 'dependencies.yaml' })).toBeVisible();
    
    // Click on frontend node
    const frontendNode = await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).first();
    await frontendNode.click();
    
    // Wait for files to update
    await page.waitForTimeout(1000);
    
    // Files tab should remain active when switching nodes
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Check that frontend files are shown
    await expect(page.locator('.ant-list-item').filter({ hasText: 'foo' })).toBeVisible();
  });
  
  test('should maintain side panel state when switching tabs', async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph elements
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Click on frontend node
    const frontendNode = await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).first();
    await frontendNode.click();
    
    // Wait for files to load
    await page.waitForTimeout(1000);
    
    // Files tab should be active by default when a node is selected
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Navigate to foo folder
    await page.locator('.ant-list-item').filter({ hasText: 'foo' }).click();
    
    // Wait for subdirectory to load
    await page.waitForTimeout(1000);
    
    // Switch to Config tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Config' }).click();
    
    // Switch back to Files tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Files' }).click();
    
    // Check that we're still in the foo directory
    await expect(page.locator('.ant-breadcrumb')).toContainText('foo');
  });
});