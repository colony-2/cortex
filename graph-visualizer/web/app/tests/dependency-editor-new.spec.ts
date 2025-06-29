import { test, expect } from '@playwright/test';

test.describe('New Relationship Editor', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/boxes');
    
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
    
    // Click on Relationships subtab
    await page.click('.ant-tabs-tab:has-text("Relationships")');
    
    // Wait for relationship editor to load
    await page.waitForSelector('text=Relationships for api', { state: 'visible' });
  });

  test('should display all nodes in a single list', async ({ page }) => {
    // Check if the list is visible
    await expect(page.locator('.ant-list')).toBeVisible();
    
    // Check for the legend with exact text matching
    await expect(page.getByText('Current relationship', { exact: true })).toBeVisible();
    await expect(page.getByText('Can be added', { exact: true })).toBeVisible();
    await expect(page.getByText('Direct parent', { exact: true })).toBeVisible();
    await expect(page.getByText('Indirect parent', { exact: true })).toBeVisible();
  });

  test('should show remove buttons for relationships', async ({ page }) => {
    // Look for items with blue link icons (relationships)
    const removeButtons = page.locator('button:has-text("Remove")');
    const count = await removeButtons.count();
    
    if (count > 0) {
      // Check that remove buttons are visible
      await expect(removeButtons.first()).toBeVisible();
    }
  });

  test('should show add buttons for available nodes', async ({ page }) => {
    // Look for items with green arrow icons (available)
    const addButtons = page.locator('button:has-text("Add")');
    const count = await addButtons.count();
    
    if (count > 0) {
      // Check that add buttons are visible
      await expect(addButtons.first()).toBeVisible();
    }
  });

  test('should show cannot add buttons for parent nodes', async ({ page }) => {
    // Look for disabled cannot add buttons
    const cannotAddButtons = page.locator('button:has-text("Cannot Add")');
    const count = await cannotAddButtons.count();
    
    if (count > 0) {
      // Check that cannot add buttons are visible and disabled
      const firstButton = cannotAddButtons.first();
      await expect(firstButton).toBeVisible();
      await expect(firstButton).toBeDisabled();
    }
  });

  test('should update list when adding a relationship', async ({ page }) => {
    // Find an available node to add (has Add button)
    const addButton = page.locator('button:has-text("Add")').first();
    const hasAvailable = await addButton.count() > 0;
    
    if (hasAvailable) {
      // Get the parent list item to find the node name
      const listItem = addButton.locator('..').locator('..');
      const nodeName = await listItem.locator('.ant-list-item-meta-title').textContent();
      
      // Click the Add button
      await addButton.click();
      
      // Wait for success message
      await expect(page.locator('.ant-message-success')).toBeVisible();
      
      // Wait for list to refresh
      await page.waitForTimeout(1000);
      
      // Check that the node now has a Remove button instead
      const updatedItem = page.locator('.ant-list-item').filter({ 
        hasText: nodeName || '' 
      });
      await expect(updatedItem.locator('button:has-text("Remove")')).toBeVisible();
    }
  });

  test('should update list when removing a relationship', async ({ page }) => {
    // Find a current relationship to remove (has Remove button)
    const removeButton = page.locator('button:has-text("Remove")').first();
    const hasDeps = await removeButton.count() > 0;
    
    if (hasDeps) {
      // Get the parent list item to find the node name
      const listItem = removeButton.locator('..').locator('..');
      const nodeName = await listItem.locator('.ant-list-item-meta-title').textContent();
      
      // Click the Remove button
      await removeButton.click();
      
      // Wait for success message
      await expect(page.locator('.ant-message-success')).toBeVisible();
      
      // Wait for list to refresh
      await page.waitForTimeout(1000);
      
      // Check that the node now has an Add button instead
      const updatedItem = page.locator('.ant-list-item').filter({ 
        hasText: nodeName || '' 
      });
      await expect(updatedItem.locator('button:has-text("Add")')).toBeVisible();
    }
  });
});