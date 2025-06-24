import { test, expect } from '@playwright/test';

test.describe('Side Panel', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    await page.waitForSelector('.react-flow__node');
  });

  test('should show configuration tab when no node is selected', async ({ page }) => {
    // Check that the side panel is visible
    await expect(page.locator('.ant-layout-sider')).toBeVisible();
    
    // Check that the configuration tab is active
    await expect(page.locator('.ant-tabs-tab-active')).toContainText('Configuration');
    
    // Check configuration content
    await expect(page.locator('.ant-tabs-content')).toContainText('Select a node to see its details');
  });

  test('should show files tab when a node is selected', async ({ page }) => {
    // Click on a node - use force to bypass any overlapping elements
    await page.locator('.react-flow__node').filter({ hasText: 'api' }).click({ force: true });
    
    // Wait for the files tab to appear
    await page.waitForSelector('.ant-tabs-tab:has-text("Files")');
    
    // Check that Files tab is visible and active
    await expect(page.locator('.ant-tabs-tab-active')).toContainText('Files');
    
    // Check that file browser is visible
    await expect(page.locator('.ant-tabs-content')).toContainText('api');
  });

  test('should show Claude Code tab when a node is selected', async ({ page }) => {
    // Click on a node
    await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).click({ force: true });
    
    // Click on Claude Code tab
    await page.locator('.ant-tabs-tab:has-text("Claude Code")').click();
    
    // Check that Claude Code tab content is visible
    await expect(page.locator('.ant-tabs-content')).toContainText('Claude Code integration coming soon');
  });

  test('should display files when node is selected', async ({ page }) => {
    // Click on a node
    await page.locator('.react-flow__node').filter({ hasText: 'api' }).click({ force: true });
    
    // Wait for files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/api') && response.status() === 200
    );
    
    // Check that files are displayed
    await expect(page.locator('.ant-list-item')).toHaveCount(4); // api has 4 items
    await expect(page.locator('.ant-list-item').filter({ hasText: 'dependencies.yaml' })).toBeVisible();
  });

  test('should navigate folders in file browser', async ({ page }) => {
    // Click on frontend node which has subdirectories
    await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).click({ force: true });
    
    // Wait for files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Click on src folder
    await page.locator('.ant-list-item').filter({ hasText: 'src' }).click();
    
    // Wait for subdirectory to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend?path=src') && response.status() === 200
    );
    
    // Check breadcrumb updated
    await expect(page.locator('.ant-breadcrumb')).toContainText('src');
    
    // Check that App.js is visible
    await expect(page.locator('.ant-list-item').filter({ hasText: 'App.js' })).toBeVisible();
  });

  test('should navigate using breadcrumb', async ({ page }) => {
    // Click on frontend node
    await page.locator('.react-flow__node').filter({ hasText: 'frontend' }).click({ force: true });
    
    // Navigate to src folder
    await page.locator('.ant-list-item').filter({ hasText: 'src' }).click();
    
    // Wait for navigation
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend?path=src') && response.status() === 200
    );
    
    // Click home in breadcrumb
    await page.locator('.ant-breadcrumb .anticon-home').click();
    
    // Wait for root directory to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Check we're back at root
    await expect(page.locator('.ant-list-item').filter({ hasText: 'src' })).toBeVisible();
    await expect(page.locator('.ant-list-item').filter({ hasText: 'package.json' })).toBeVisible();
  });

  test('should maintain side panel width', async ({ page }) => {
    // Get the sider element
    const sider = page.locator('.ant-layout-sider');
    
    // Check that it has the correct width
    const box = await sider.boundingBox();
    expect(box?.width).toBe(400);
  });
});