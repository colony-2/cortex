import { test, expect } from '@playwright/test';

test.describe('Side Panel', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/cells');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    await page.waitForSelector('.react-flow__node', { state: 'attached' });
  });

  test('should show configuration tab when no node is selected', async ({ page }) => {
    // Check that the side panel is visible (using splitter panel)
    await expect(page.locator('.ant-splitter-panel').nth(1)).toBeVisible();
    
    // Check that the configuration tab is active
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Configuration');
    
    // Check that container subtab is active by default
    await expect(page.locator('.ant-tabs-tab-active').nth(1)).toContainText('Container');
  });

  test('should show files tab when a node is selected', async ({ page }) => {
    // Navigate to a specific box URL to trigger node selection
    await page.goto('/cell/example-api');
    
    // Wait for the page to stabilize and check that side panel is visible
    await page.waitForSelector('.ant-splitter-panel');
    
    // Config tab should be active by default
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Config');
    
    // Files tab should be visible
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Files' })).toBeVisible();
    
    // Click on Files tab to make it active
    const filesTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Files' });
    await filesTab.click();
    
    // Wait for Files tab to be active
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Wait for file browser content to load
    await page.waitForSelector('.ant-breadcrumb', { timeout: 5000 });
    await page.waitForSelector('.ant-list-item', { timeout: 5000 });
    
    // Check that file browser is visible - node name is displayed above breadcrumb
    const fileBrowserHeader = page.locator('.ant-space-vertical').first();
    await expect(fileBrowserHeader).toContainText('example-api');
    
    // Check for specific file - example-api box should have dependencies.yaml or moon.yml
    const fileItems = page.locator('.ant-list-item');
    await expect(fileItems).toHaveCount(2, { timeout: 5000 }); // example-api folder should have 2 files
  });

  test('should show Config tab with Claude Code subtab when a node is selected', async ({ page }) => {
    // Navigate to a specific box URL to trigger node selection
    await page.goto('/cell/example-frontend/config');
    
    // Wait for the page to stabilize
    await page.waitForSelector('.ant-splitter-panel');
    
    // Config tab should be active by default
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Config');
    
    // Env subtab is active by default, navigate to Claude subtab
    const claudeTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Claude' });
    await claudeTab.click();
    
    // Claude subtab should now be active
    await expect(page.locator('.ant-tabs-tab-active').nth(1)).toContainText('Claude');
    
    // Wait for content to render
    await page.waitForSelector('h4', { timeout: 5000 });
    
    // Check that Claude Code content is visible with section headers
    const configTabContent = page.locator('.ant-tabs-tabpane-active').last();
    await expect(configTabContent).toContainText('Claude Code Settings');
    await expect(configTabContent).toContainText('Available Tools');
    await expect(configTabContent).toContainText('Custom Instructions');
  });

  test('should display files when node is selected', async ({ page }) => {
    // Navigate directly to files tab for a specific box
    await page.goto('/cell/example-api/files');
    
    // Wait for the page to stabilize
    await page.waitForSelector('.ant-splitter-panel');
    
    // Files tab should be active
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Wait for file list to load
    await page.waitForSelector('.ant-list-item', { timeout: 5000 });
    
    // Check that files are displayed (example-api folder should have 2 files)
    const fileCount = await page.locator('.ant-list-item').count();
    expect(fileCount).toBe(2);
    
    // Check for expected files in example-api folder
    await expect(page.locator('.ant-list-item').filter({ hasText: 'moon.yml' })).toBeVisible();
  });

  test('should navigate folders in file browser', async ({ page }) => {
    // Navigate directly to files tab for frontend box
    await page.goto('/cell/example-frontend/files');
    
    // Wait for the page to stabilize
    await page.waitForSelector('.ant-splitter-panel');
    
    // Wait for file list to load
    await page.waitForSelector('.ant-list-item', { timeout: 5000 });
    
    // Click on foo folder
    const fooFolder = page.locator('.ant-list-item').filter({ hasText: 'foo' });
    await fooFolder.click();
    
    // Wait for breadcrumb to update
    await expect(page.locator('.ant-breadcrumb')).toContainText('foo', { timeout: 5000 });
    
    // Wait for files in subdirectory to load
    await page.waitForSelector('.ant-list-item', { timeout: 5000 });
    
    // Check that bar.txt is visible
    await expect(page.locator('.ant-list-item').filter({ hasText: 'bar.txt' })).toBeVisible();
  });

  test('should navigate using breadcrumb', async ({ page }) => {
    // Navigate directly to files tab for frontend box
    await page.goto('/cell/example-frontend/files');
    
    // Wait for the page to stabilize
    await page.waitForSelector('.ant-splitter-panel');
    
    // Wait for initial files to load
    await page.waitForSelector('.ant-list-item', { timeout: 5000 });
    
    // Navigate to foo folder
    const fooFolder = page.locator('.ant-list-item').filter({ hasText: 'foo' });
    await fooFolder.click();
    
    // Wait for breadcrumb to update
    await expect(page.locator('.ant-breadcrumb')).toContainText('foo', { timeout: 5000 });
    
    // Click home in breadcrumb
    const homeIcon = page.locator('.ant-breadcrumb .anticon-home');
    await homeIcon.click();
    
    // Wait for files to reload
    await page.waitForSelector('.ant-list-item', { timeout: 5000 });
    
    // Check we're back at root - should see folders and files
    await expect(page.locator('.ant-list-item').filter({ hasText: 'foo' })).toBeVisible();
    await expect(page.locator('.ant-list-item').filter({ hasText: 'moon.yml' })).toBeVisible();
  });

  test('should maintain side panel width', async ({ page }) => {
    // Get the splitter panel element (since we're using Splitter now, not Layout.Sider)
    const panel = page.locator('.ant-splitter-panel').nth(1);
    
    // Check that it exists and has some width
    await expect(panel).toBeVisible();
    const box = await panel.boundingBox();
    expect(box?.width).toBeGreaterThan(100);
  });
});