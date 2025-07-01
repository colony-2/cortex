import { test, expect } from '@playwright/test';

test.describe('EnvEditor', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/boxes');
    await page.waitForSelector('.react-flow__node', { timeout: 10000 });
  });

  test('should show create button when devcontainer.json does not exist', async ({ page }) => {
    // Mock API response for missing devcontainer.json
    await page.route('**/api/nodes/*/files/.devcontainer/devcontainer.json', async route => {
      await route.fulfill({ status: 404 });
    });
    
    // Mock API response for container status
    await page.route('**/api/nodes/*/container/status', async route => {
      await route.fulfill({ 
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'none', containerId: null })
      });
    });
    
    // Click on a node
    await page.click('.react-flow__node');
    
    // Navigate to the Config tab first
    await page.getByRole('tab', { name: 'Config' }).click();
    
    // Wait for the Config tab content to load
    await page.waitForTimeout(500);
    
    // Now click on the Env subtab (within Config)
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Env' }).click();
    
    // Wait for the component to load
    await page.waitForTimeout(500);
    
    // Check for the empty state and create button
    await expect(page.locator('.ant-empty-description').filter({ hasText: 'No devcontainer.json file exists' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create devcontainer.json' })).toBeVisible();
    
    // Verify that container controls are NOT shown when no devcontainer.json exists
    await expect(page.getByText('Devcontainer Service Controls')).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Container' })).not.toBeVisible();
  });

  test('should show read-only view when file exists and not in edit mode', async ({ page }) => {
    // Mock API response for existing devcontainer.json
    const mockConfig = {
      name: "Test Container",
      image: "mcr.microsoft.com/devcontainers/base:ubuntu",
      features: {},
      customizations: {
        vscode: {
          extensions: []
        }
      },
      forwardPorts: [],
      postCreateCommand: ""
    };
    
    await page.route('**/api/nodes/*/files/.devcontainer/devcontainer.json', async route => {
      await route.fulfill({ 
        status: 200,
        contentType: 'text/plain',
        body: JSON.stringify(mockConfig, null, 2)
      });
    });
    
    // Mock API response for container status
    await page.route('**/api/nodes/*/container/status', async route => {
      await route.fulfill({ 
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'none', containerId: null })
      });
    });
    
    // Click on a node
    await page.click('.react-flow__node');
    
    // Navigate to the Config tab first
    await page.getByRole('tab', { name: 'Config' }).click();
    
    // Wait for the Config tab content to load
    await page.waitForTimeout(500);
    
    // Now click on the Env subtab (within Config)
    await page.locator('.ant-tabs-tab').filter({ hasText: 'Env' }).click();
    
    // Wait for the component to load
    await page.waitForTimeout(500);
    
    // Check that we're in read-only mode
    await expect(page.getByRole('button', { name: 'Edit' })).toBeVisible();
    
    // Check that the content is displayed in a card with pre tag
    await expect(page.locator('.ant-card pre')).toBeVisible();
    
    // Check that the editor is not visible
    await expect(page.locator('.monaco-editor')).not.toBeVisible();
    
    // Verify that container controls ARE shown when devcontainer.json exists
    await expect(page.getByText('Devcontainer Service Controls')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Create Container' })).toBeVisible();
  });
});