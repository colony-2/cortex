import { test, expect } from '@playwright/test';

test.describe('File Browser', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('http://localhost:5173');
    await page.waitForSelector('.svelte-flow__node');
  });

  test('should open file browser when clicking folder icon', async ({ page }) => {
    // Click the file browser button on the first node
    const firstNode = page.locator('.svelte-flow__node').first();
    await firstNode.hover();
    
    const fileBrowserBtn = firstNode.locator('button[title="Browse files"]');
    await fileBrowserBtn.click();

    // Check that file browser opened
    await expect(page.locator('.file-browser')).toBeVisible();
    await expect(page.locator('.browser-header')).toBeVisible();
  });

  test('should display node information in file browser header', async ({ page }) => {
    // Open file browser for a specific node
    const apiNode = page.locator('.svelte-flow__node').filter({ hasText: 'api' }).first();
    await apiNode.hover();
    await apiNode.locator('button[title="Browse files"]').click();

    // Check header shows correct node info
    const header = page.locator('.browser-header');
    await expect(header.locator('h3')).toContainText('api');
    await expect(header.locator('.path')).toBeVisible();
  });

  test('should close file browser when clicking close button', async ({ page }) => {
    // Open file browser
    const firstNode = page.locator('.svelte-flow__node').first();
    await firstNode.hover();
    await firstNode.locator('button[title="Browse files"]').click();

    // Verify it's open
    await expect(page.locator('.file-browser')).toBeVisible();

    // Click close button
    await page.locator('button[title="Close file browser"]').click();

    // Verify it's closed
    await expect(page.locator('.file-browser')).not.toBeVisible();
  });

  test('should highlight node when file browser is open', async ({ page }) => {
    // Get a specific node
    const apiNode = page.locator('.svelte-flow__node').filter({ hasText: 'api' }).first();
    
    // Open file browser
    await apiNode.hover();
    await apiNode.locator('button[title="Browse files"]').click();

    // Check that the node has the viewing-files class
    await expect(apiNode.locator('.node-box.viewing-files')).toBeVisible();
    
    // The header should have orange color
    const nodeHeader = apiNode.locator('.node-header');
    await expect(nodeHeader).toHaveCSS('background-color', 'rgb(255, 165, 0)');
  });

  test('should maintain file browser header action buttons', async ({ page }) => {
    // Open file browser
    const firstNode = page.locator('.svelte-flow__node').first();
    await firstNode.hover();
    await firstNode.locator('button[title="Browse files"]').click();

    // Check that all action buttons are present in the header
    const browserActions = page.locator('.browser-actions');
    await expect(browserActions.locator('button[title="View terminal"]')).toBeVisible();
    await expect(browserActions.locator('button[title="View dependencies"]')).toBeVisible();
    await expect(browserActions.locator('button[title="Close file browser"]')).toBeVisible();
  });

  test('should show file list component', async ({ page }) => {
    // Open file browser
    const firstNode = page.locator('.svelte-flow__node').first();
    await firstNode.hover();
    await firstNode.locator('button[title="Browse files"]').click();

    // Check that file list is rendered
    await expect(page.locator('.file-list')).toBeVisible();
  });

  test('should display dependencies.yaml file in api node', async ({ page }) => {
    // Capture console logs
    page.on('console', msg => {
      console.log('Browser console:', msg.type(), msg.text());
    });
    
    // Find and click on the api node specifically
    const apiNode = page.locator('.svelte-flow__node').filter({ hasText: 'api' }).first();
    await apiNode.hover();
    await apiNode.locator('button[title="Browse files"]').click();

    // Wait for file browser to be visible
    await expect(page.locator('.file-browser')).toBeVisible();
    
    // Wait a bit for the component to initialize
    await page.waitForTimeout(2000);
    
    // Debug: Take a screenshot to see what's happening
    await page.screenshot({ path: 'file-browser-debug.png' });
    
    // Check the network requests
    const requests = [];
    page.on('request', request => {
      if (request.url().includes('/api/files/')) {
        requests.push({
          url: request.url(),
          method: request.method()
        });
      }
    });
    
    page.on('response', response => {
      if (response.url().includes('/api/files/')) {
        console.log('API Response:', response.status(), response.url());
      }
    });
    
    // Wait a bit more
    await page.waitForTimeout(1000);
    
    console.log('Network requests:', requests);
    
    // Check if we see any file manager content
    const fileManagerVisible = await page.locator('.wx-filemanager').isVisible();
    console.log('FileManager visible:', fileManagerVisible);
    
    // Check if loading is still showing
    const loadingVisible = await page.locator('.loading').isVisible();
    console.log('Loading visible:', loadingVisible);
    
    // Try to find any text content in the file browser
    const fileBrowserContent = await page.locator('.file-browser').textContent();
    console.log('File browser content:', fileBrowserContent);
    
    // Now do the actual assertions with shorter timeout
    await expect(page.locator('.loading')).not.toBeVisible({ timeout: 5000 });
    
    // Check that dependencies.yaml is visible in the file list
    await expect(page.locator('text=dependencies.yaml')).toBeVisible({ timeout: 5000 });
  });

  test('should split screen when file browser is open', async ({ page }) => {
    // Initially, graph should take full width
    const graphContainer = page.locator('.graph-container');
    const initialWidth = await graphContainer.boundingBox();
    
    // Open file browser
    const firstNode = page.locator('.svelte-flow__node').first();
    await firstNode.hover();
    await firstNode.locator('button[title="Browse files"]').click();

    // Check that app container has split class
    await expect(page.locator('.app-container.split')).toBeVisible();
    
    // Graph container should be smaller now
    const newWidth = await graphContainer.boundingBox();
    expect(newWidth?.width).toBeLessThan(initialWidth?.width || 0);
    
    // Browser container should be visible
    await expect(page.locator('.browser-container')).toBeVisible();
  });
});