import { test, expect } from '@playwright/test';

test.describe('UI Integration Tests', () => {
  test('should show file browser in side panel when clicking node', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Wait for nodes to be rendered
    await page.waitForTimeout(1000);
    
    // Click on a node
    await page.locator('.react-flow__node').filter({ hasText: 'api' }).click({ force: true });
    
    // Check that side panel shows files tab
    await expect(page.locator('.ant-tabs-tab-active')).toContainText('Files');
    
    // Wait for files to be loaded
    await page.waitForResponse(response => 
      response.url().includes('/api/files/') && response.status() === 200
    );
    
    // Check that file list is visible
    await expect(page.locator('.ant-list')).toBeVisible();
    
    // Verify file items are loaded
    const fileItems = page.locator('.ant-list-item');
    const itemCount = await fileItems.count();
    expect(itemCount).toBeGreaterThan(0);
  });
  
  test('should switch between nodes and update file browser', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Click on api node
    await page.locator('.react-flow__node').filter({ hasText: 'api' }).click({ force: true });
    
    // Wait for api files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/api') && response.status() === 200
    );
    
    // Check that api files are shown
    await expect(page.locator('.ant-tabs-content')).toContainText('api');
    await expect(page.locator('.ant-list-item').filter({ hasText: 'dependencies.yaml' })).toBeVisible();
    
    // Click on frontend node
    await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).click({ force: true });
    
    // Wait for frontend files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Check that frontend files are shown
    await expect(page.locator('.ant-tabs-content')).toContainText('frontend');
    await expect(page.locator('.ant-list-item').filter({ hasText: 'src' })).toBeVisible();
  });
  
  test('should maintain side panel state when switching tabs', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Click on a node
    await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).click({ force: true });
    
    // Wait for files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Navigate to src folder
    await page.locator('.ant-list-item').filter({ hasText: 'src' }).click();
    
    // Wait for subdirectory to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend?path=src') && response.status() === 200
    );
    
    // Switch to Claude Code tab
    await page.locator('.ant-tabs-tab:has-text("Claude Code")').click();
    
    // Switch back to Files tab
    await page.locator('.ant-tabs-tab:has-text("Files")').click();
    
    // Check that we're still in the src directory
    await expect(page.locator('.ant-breadcrumb')).toContainText('src');
  });
});