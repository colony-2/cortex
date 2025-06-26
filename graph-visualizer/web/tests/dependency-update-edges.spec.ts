import { test, expect } from '@playwright/test';

test.describe('Dependency Editor - Edge Updates', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/boxes');
    // Wait for the loading to finish
    await page.waitForSelector('.react-flow', { timeout: 10000 });
  });

  test('should refresh graph when dependencies are updated', async ({ page }) => {
    // Wait for nodes to be rendered
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
    
    // Find a node to click on (use auth as it exists in the example)
    const authNode = page.locator('.react-flow__node').filter({ hasText: 'auth' }).first();
    await authNode.click();
    
    // Wait for side panel
    await page.waitForSelector('.ant-tabs');
    
    // Click on Config tab
    await page.click('[role="tab"]:has-text("Config")');
    
    // Click on Dependencies subtab
    await page.click('[role="tab"]:has-text("Dependencies")');
    
    // Wait for dependency editor to load
    await page.waitForSelector('text=Dependencies for auth');
    
    // Check if there's a dropdown selector
    const selector = await page.locator('.ant-select-selector').isVisible();
    
    if (selector) {
      // Try to add a dependency
      await page.click('.ant-select-selector');
      
      // Wait for dropdown options
      await page.waitForSelector('.ant-select-dropdown', { timeout: 5000 });
      
      // Select the first available option
      const firstOption = page.locator('.ant-select-item-option').first();
      const hasOptions = await firstOption.isVisible().catch(() => false);
      
      if (hasOptions) {
        await firstOption.click();
        
        // Click Add Dependency button
        await page.click('button:has-text("Add Dependency")');
        
        // Wait for success message
        await expect(page.getByText('Dependencies updated successfully')).toBeVisible({ timeout: 10000 });
        
        // Wait for graph update message
        await expect(page.getByText('Graph updated with new dependencies')).toBeVisible({ timeout: 10000 });
      }
    }
  });
});