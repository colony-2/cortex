import { test, expect } from '@playwright/test';

test.describe('Tab Restoration on Page Reload', () => {
  test.beforeEach(async ({ page }) => {
    // Mock git API responses for changes tab
    await page.route('**/api/cells/*/git/status', async route => {
      await route.fulfill({
        status: 200,
        body: JSON.stringify({
          added: [],
          modified: ['test.js'],
          deleted: [],
          untracked: [],
          totalCount: 1
        })
      });
    });
    
    await page.route('**/api/cells/*/git/diff', async route => {
      await route.fulfill({
        status: 200,
        body: JSON.stringify({
          files: [{
            path: 'test.js',
            status: 'modified',
            additions: 5,
            deletions: 2,
            patch: '+++ test.js\n@@ -1,3 +1,6 @@\n console.log("test");'
          }]
        })
      });
    });
    
    await page.route('**/api/cells/*/git/history', async route => {
      await route.fulfill({
        status: 200,
        body: JSON.stringify([{
          hash: 'abc123',
          author: 'Test Author',
          email: 'test@example.com',
          message: 'Test commit',
          timestamp: '2024-01-01T12:00:00Z'
        }])
      });
    });
  });

  test('should restore Files tab when reloading /cell/<id>/files', async ({ page }) => {
    // Navigate directly to a specific box and tab
    await page.goto('/cell/api/files');
    
    // Wait for the page to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    // Wait for side panel to appear
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Check that Files tab is active
    const filesTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Files' });
    await expect(filesTab).toHaveClass(/ant-tabs-tab-active/);
    
    // Check that file browser is visible
    await expect(page.locator('.ant-list')).toBeVisible();
  });

  test('should restore Config tab when reloading /cell/<id>/config', async ({ page }) => {
    await page.goto('/cell/api/config');
    
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Check that Config tab is active
    const configTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Config' });
    await expect(configTab).toHaveClass(/ant-tabs-tab-active/);
  });

  test('should restore Config tab with Env subtab when reloading /cell/<id>/config/env', async ({ page }) => {
    await page.goto('/cell/api/config/env');
    
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Check that Config tab is active
    const configTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Config' });
    await expect(configTab).toHaveClass(/ant-tabs-tab-active/);
    
    // Wait for nested tabs to appear
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    
    // Check that Env subtab is active (look for the nested tab)
    const envSubtab = page.locator('.ant-tabs-tab').filter({ hasText: 'Env' });
    await expect(envSubtab.last()).toHaveClass(/ant-tabs-tab-active/);
  });

  test('should restore Changes tab when reloading /cell/<id>/changes', async ({ page }) => {
    await page.goto('/cell/api/changes');
    
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Check that Changes tab is active
    const changesTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Changes' });
    await expect(changesTab).toHaveClass(/ant-tabs-tab-active/);
  });

  test('should restore Changes tab with History subtab when reloading /cell/<id>/changes/history', async ({ page }) => {
    await page.goto('/cell/api/changes/history');
    
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Check that Changes tab is active
    const changesTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Changes' });
    await expect(changesTab).toHaveClass(/ant-tabs-tab-active/);
    
    // Wait for nested tabs to appear and check History subtab
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    const historySubtab = page.locator('.ant-tabs-tab').filter({ hasText: 'History' });
    await expect(historySubtab.last()).toHaveClass(/ant-tabs-tab-active/);
  });

  test('should restore Changes tab with Details subtab when reloading /cell/<id>/changes/details', async ({ page }) => {
    await page.goto('/cell/api/changes/details');
    
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Check that Changes tab is active
    const changesTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Changes' });
    await expect(changesTab).toHaveClass(/ant-tabs-tab-active/);
    
    // Check that Details subtab is active
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    const detailsSubtab = page.locator('.ant-tabs-tab').filter({ hasText: 'Details' });
    await expect(detailsSubtab.last()).toHaveClass(/ant-tabs-tab-active/);
  });
});