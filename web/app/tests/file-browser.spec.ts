import { test, expect } from '@playwright/test';

test.describe('File Browser', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/cells');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    await page.waitForSelector('.react-flow__node', { state: 'attached' });
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
    
    // Wait for the Files tab panel to be visible and contain content
    const filesTabPanel = page.locator('[role="tabpanel"]').filter({ has: page.locator('text=api') });
    await expect(filesTabPanel).toBeVisible();
    
    // Check that file browser is visible and contains the node name
    await expect(filesTabPanel).toContainText('api');
    
    // Check for actual files in the api directory
    await expect(page.locator('.ant-list-item').filter({ hasText: 'moon.yml' })).toBeVisible();
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
    
    // Wait for the Files tab panel to be visible
    const filesTabPanel = page.locator('[role="tabpanel"]').filter({ has: page.locator('.ant-list') });
    await expect(filesTabPanel).toBeVisible();
    
    // Wait for files to load by checking for expected content
    await expect(page.locator('.ant-list-item').filter({ hasText: 'moon.yml' })).toBeVisible({ timeout: 10000 });
    
    // Check that files are displayed (we don't know exact count, so just check > 0)
    const fileCount = await page.locator('.ant-list-item').count();
    expect(fileCount).toBeGreaterThan(0);
    
    // Check for expected files in the api directory
    await expect(page.locator('.ant-list-item').filter({ hasText: '.devcontainer' })).toBeVisible();
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
    
    // Wait for the Files tab panel to be visible
    const filesTabPanel = page.locator('[role="tabpanel"]').filter({ has: page.locator('.ant-list') });
    await expect(filesTabPanel).toBeVisible();
    
    // Wait for files to load by checking for the foo folder
    await expect(page.locator('.ant-list-item').filter({ hasText: 'foo' })).toBeVisible({ timeout: 10000 });
    
    // Click on foo folder
    await page.locator('.ant-list-item').filter({ hasText: 'foo' }).click();
    
    // Wait for subdirectory to load by checking for bar.txt
    await expect(page.locator('.ant-list-item').filter({ hasText: 'bar.txt' })).toBeVisible({ timeout: 10000 });
    
    // Check breadcrumb updated
    await expect(page.locator('.ant-breadcrumb')).toContainText('foo');
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
    
    // Wait for the Files tab panel to be visible
    const filesTabPanel = page.locator('[role="tabpanel"]').filter({ has: page.locator('.ant-list') });
    await expect(filesTabPanel).toBeVisible();
    
    // Wait for initial load by checking for the foo folder
    await expect(page.locator('.ant-list-item').filter({ hasText: 'foo' })).toBeVisible({ timeout: 10000 });
    
    // Navigate to foo folder
    await page.locator('.ant-list-item').filter({ hasText: 'foo' }).click();
    
    // Wait for navigation by checking for bar.txt
    await expect(page.locator('.ant-list-item').filter({ hasText: 'bar.txt' })).toBeVisible({ timeout: 10000 });
    
    // Click home in breadcrumb
    await page.locator('.ant-breadcrumb .anticon-home').click();
    
    // Wait for root directory to load by checking for foo folder again
    await expect(page.locator('.ant-list-item').filter({ hasText: 'foo' })).toBeVisible({ timeout: 10000 });
    
    // Check we're back at root - frontend directory should have foo folder and moon.yml
    await expect(page.locator('.ant-list-item').filter({ hasText: 'moon.yml' })).toBeVisible();
  });
});