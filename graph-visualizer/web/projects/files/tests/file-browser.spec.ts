import { test, expect } from '@playwright/test';

test.describe('File Browser', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/boxes');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    await page.waitForSelector('.react-flow__node');
  });

  test('should show files tab when a node is selected', async ({ page }) => {
    // Trigger node selection manually using the global event
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Wait for state to update
    await page.waitForTimeout(100);
    
    // Files tab should be visible
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Files' })).toBeVisible();
    
    // Click on Files tab to make it active
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Files' }).click();
    
    // Now check that Files tab is active
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Check that file browser is visible (wait for content to load)
    await expect(page.locator('.ant-tabs-content').first()).toContainText('api');
    await expect(page.locator('.ant-list-item').filter({ hasText: 'dependencies.yaml' })).toBeVisible();
  });

  test('should display files when node is selected', async ({ page }) => {
    // Trigger node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Wait for state to update
    await page.waitForTimeout(100);
    
    // Click on Files tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Files' }).click();
    
    // Wait for files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/api') && response.status() === 200
    );
    
    // Check that files are displayed (we don't know exact count, so just check > 0)
    const fileCount = await page.locator('.ant-list-item').count();
    expect(fileCount).toBeGreaterThan(0);
    await expect(page.locator('.ant-list-item').filter({ hasText: 'dependencies.yaml' })).toBeVisible();
  });

  test('should navigate folders in file browser', async ({ page }) => {
    // Trigger node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'frontend' } }));
    });
    
    // Wait for state to update
    await page.waitForTimeout(100);
    
    // Config tab is active by default, click on Files tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Files' }).click();
    
    // Wait for files to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Click on foo folder
    await page.locator('.ant-list-item').filter({ hasText: 'foo' }).click();
    
    // Wait for subdirectory to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend?path=foo') && response.status() === 200
    );
    
    // Check breadcrumb updated
    await expect(page.locator('.ant-breadcrumb')).toContainText('foo');
    
    // Check that bar.txt is visible
    await expect(page.locator('.ant-list-item').filter({ hasText: 'bar.txt' })).toBeVisible();
  });

  test('should navigate using breadcrumb', async ({ page }) => {
    // Trigger node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'frontend' } }));
    });
    
    // Wait for state to update
    await page.waitForTimeout(100);
    
    // Config tab is active by default, click on Files tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Files' }).click();
    
    // Wait for initial load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Navigate to foo folder
    await page.locator('.ant-list-item').filter({ hasText: 'foo' }).click();
    
    // Wait for navigation
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend?path=foo') && response.status() === 200
    );
    
    // Click home in breadcrumb
    await page.locator('.ant-breadcrumb .anticon-home').click();
    
    // Wait for root directory to load
    await page.waitForResponse(response => 
      response.url().includes('/api/files/frontend') && response.status() === 200
    );
    
    // Check we're back at root
    await expect(page.locator('.ant-list-item').filter({ hasText: 'foo' })).toBeVisible();
    await expect(page.locator('.ant-list-item').filter({ hasText: 'dependencies.yaml' })).toBeVisible();
  });
});