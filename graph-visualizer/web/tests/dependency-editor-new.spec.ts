import { test, expect } from '@playwright/test';

test.describe('New Dependency Editor', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    
    // Wait for graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { state: 'visible' });
    
    // Wait for nodes to be rendered
    await page.waitForTimeout(1000);
    
    // Select the api node
    await page.evaluate(() => {
      window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: 'api' } }));
    });
    
    // Wait for side panel to appear
    await page.waitForSelector('.ant-tabs-content', { state: 'visible' });
    
    // Click on Config tab
    await page.click('.ant-tabs-tab:has-text("Config")');
    
    // Click on Dependencies subtab
    await page.click('.ant-tabs-tab:has-text("Dependencies")');
    
    // Wait for dependency editor to load
    await page.waitForSelector('text=Dependencies for api', { state: 'visible' });
  });

  test('should display all nodes in a single list', async ({ page }) => {
    // Check if the list is visible
    await expect(page.locator('.ant-list')).toBeVisible();
    
    // Check for the legend with exact text matching
    await expect(page.getByText('Current dependency', { exact: true })).toBeVisible();
    await expect(page.getByText('Can be added', { exact: true })).toBeVisible();
    await expect(page.getByText('Direct parent', { exact: true })).toBeVisible();
    await expect(page.getByText('Indirect parent', { exact: true })).toBeVisible();
  });

  test('should show current dependencies with remove buttons', async ({ page }) => {
    // Look for items with blue dependency tags
    const dependencies = page.locator('.ant-tag-blue');
    const count = await dependencies.count();
    
    if (count > 0) {
      // Check that each dependency has a remove button
      const firstDep = page.locator('.ant-list-item').filter({ has: page.locator('.ant-tag-blue') }).first();
      await expect(firstDep.locator('button:has-text("Remove")')).toBeVisible();
    }
  });

  test('should show available nodes with add buttons', async ({ page }) => {
    // Look for items with green available tags
    const available = page.locator('.ant-tag-green');
    const count = await available.count();
    
    if (count > 0) {
      // Check that each available node has an add button
      const firstAvailable = page.locator('.ant-list-item').filter({ has: page.locator('.ant-tag-green') }).first();
      await expect(firstAvailable.locator('button:has-text("Add")')).toBeVisible();
    }
  });

  test('should show parent nodes with cannot add buttons', async ({ page }) => {
    // Look for items with orange or red tags (parents)
    const parents = page.locator('.ant-tag-orange, .ant-tag-red');
    const count = await parents.count();
    
    if (count > 0) {
      // Check that parent nodes have disabled "Cannot Add" buttons
      const firstParent = page.locator('.ant-list-item').filter({ 
        has: page.locator('.ant-tag-orange, .ant-tag-red') 
      }).first();
      const cannotAddButton = firstParent.locator('button:has-text("Cannot Add")');
      await expect(cannotAddButton).toBeVisible();
      await expect(cannotAddButton).toBeDisabled();
    }
  });

  test('should update list when adding a dependency', async ({ page }) => {
    // Find an available node to add
    const availableItem = page.locator('.ant-list-item').filter({ 
      has: page.locator('.ant-tag-green') 
    }).first();
    
    const hasAvailable = await availableItem.count() > 0;
    
    if (hasAvailable) {
      // Get the node name before clicking
      const nodeName = await availableItem.locator('.ant-list-item-meta-title').textContent();
      
      // Click the Add button
      await availableItem.locator('button:has-text("Add")').click();
      
      // Wait for success message
      await expect(page.locator('.ant-message-success')).toBeVisible();
      
      // Wait for list to refresh
      await page.waitForTimeout(1000);
      
      // Check that the node now has a blue dependency tag
      const updatedItem = page.locator('.ant-list-item').filter({ 
        hasText: nodeName || '' 
      });
      await expect(updatedItem.locator('.ant-tag-blue')).toBeVisible();
    }
  });

  test('should update list when removing a dependency', async ({ page }) => {
    // Find a current dependency to remove
    const depItem = page.locator('.ant-list-item').filter({ 
      has: page.locator('.ant-tag-blue') 
    }).first();
    
    const hasDeps = await depItem.count() > 0;
    
    if (hasDeps) {
      // Get the node name before clicking
      const nodeName = await depItem.locator('.ant-list-item-meta-title').textContent();
      
      // Click the Remove button
      await depItem.locator('button:has-text("Remove")').click();
      
      // Wait for success message
      await expect(page.locator('.ant-message-success')).toBeVisible();
      
      // Wait for list to refresh
      await page.waitForTimeout(1000);
      
      // Check that the node now has a green available tag
      const updatedItem = page.locator('.ant-list-item').filter({ 
        hasText: nodeName || '' 
      });
      await expect(updatedItem.locator('.ant-tag-green')).toBeVisible();
    }
  });

  test('should show summary counts', async ({ page }) => {
    // Check that summary section exists
    await expect(page.locator('text=Summary:')).toBeVisible();
    
    // Check for count displays
    await expect(page.locator('text=/current dependencies/')).toBeVisible();
    await expect(page.locator('text=/nodes available to add/')).toBeVisible();
    await expect(page.locator('text=/nodes cannot be added/')).toBeVisible();
  });
});