import { test, expect } from '@playwright/test';

// Helper to wait for files to be visible in the list
async function waitForFileList(page) {
  await page.waitForSelector('.ant-list', { state: 'visible', timeout: 10000 });
  await page.waitForFunction(() => {
    const listItems = document.querySelectorAll('.ant-list-item');
    return listItems.length > 0;
  }, { timeout: 10000 });
}

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
    
    // Trigger node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Wait for navigation to complete
    await page.waitForURL('**/cell/api/files');
    
    // Check that Files tab is active by default when a node is selected
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Config tab should be visible
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Config' })).toBeVisible();
    
    // Wait for file list to be loaded
    await waitForFileList(page);
    
    // Verify file items are loaded - just check that we have at least one item
    const fileItems = await page.locator('.ant-list-item').count();
    expect(fileItems).toBeGreaterThan(0);
    
    // Click on Config tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Config' }).click();
    
    // Now Config tab should be active
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Config');
  });
  
  test('should switch between nodes and update file browser', async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph data to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph to load
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    
    // Trigger api node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Wait for navigation to api node
    await page.waitForURL('**/cell/api/files');
    
    // Files tab should be active by default when node is selected
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Wait for file list to be loaded
    await waitForFileList(page);
    
    // Check that api node name is shown
    await expect(page.locator('text=api').first()).toBeVisible();
    // Verify files are loaded
    const apiFileItems = await page.locator('.ant-list-item').count();
    expect(apiFileItems).toBeGreaterThan(0);
    
    // Trigger frontend node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'frontend' } }));
    });
    
    // Wait for navigation to frontend node
    await page.waitForURL('**/cell/frontend/files');
    
    // Files tab should remain active when switching nodes
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Wait for frontend files to load
    await waitForFileList(page);
    
    // Check that frontend node name is shown
    await expect(page.locator('text=frontend').first()).toBeVisible();
    // Verify files are loaded  
    const frontendFileItems = await page.locator('.ant-list-item').count();
    expect(frontendFileItems).toBeGreaterThan(0);
  });
  
  test('should maintain side panel state when switching tabs', async ({ page }) => {
    let folderItems = 0;
    await page.goto('/');
    
    // Wait for graph to load
    await page.waitForResponse(response => 
      response.url().includes('/api/graph') && response.status() === 200
    );
    
    // Wait for graph to be rendered
    await page.waitForSelector('.react-flow', { timeout: 10000 });
    await page.waitForTimeout(1000);
    
    // Trigger node selection manually
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'frontend' } }));
    });
    
    // Wait for navigation to frontend node
    await page.waitForURL('**/cell/frontend/files');
    
    // Files tab should be active by default when node is selected
    await expect(page.locator('.ant-tabs-tab-active').first()).toContainText('Files');
    
    // Wait for file list to be loaded
    await waitForFileList(page);
    
    // Find a folder item (if any) and navigate into it
    folderItems = await page.locator('.ant-list-item').filter({ has: page.locator('[data-icon="folder"]') }).count();
    
    if (folderItems > 0) {
      // Click the first folder
      const firstFolder = page.locator('.ant-list-item').filter({ has: page.locator('[data-icon="folder"]') }).first();
      const folderName = await firstFolder.textContent();
      
      // Set up response promise before clicking
      const responsePromise = page.waitForResponse(response => 
        response.url().includes('/api/cells/frontend/files?path=') && response.status() === 200
      );
      
      await firstFolder.click();
      
      // Wait for subdirectory to load
      await responsePromise;
      
      // Check that we're in a subdirectory (breadcrumb should show something)
      const breadcrumbItems = await page.locator('.ant-breadcrumb-link').count();
      expect(breadcrumbItems).toBeGreaterThan(1);
    }
    
    // Switch to Config tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Config' }).click();
    
    // Switch back to Files tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Files' }).click();
    
    // Check that we're still in the same directory state
    if (folderItems > 0) {
      const breadcrumbItems = await page.locator('.ant-breadcrumb-link').count();
      expect(breadcrumbItems).toBeGreaterThan(1);
    }
  });
});