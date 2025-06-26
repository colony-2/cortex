import { test, expect } from '@playwright/test';

test.describe('Git Changes Tab', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to the application
    await page.goto('/boxes');
    
    // Wait for the graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    // Click on a node to select it
    await page.locator('.react-flow__node').first().click();
    
    // Wait for side panel to appear
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Click on the Changes tab
    await page.getByText('Changes').click();
  });

  test('should display changes tab with subtabs', async ({ page }) => {
    // Wait for subtabs to be visible
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    
    // Check that the subtabs are visible (use locator for nested tabs)
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Summary' }).last()).toBeVisible();
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Details' }).last()).toBeVisible();
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'History' }).last()).toBeVisible();
  });

  test('should show summary tab by default', async ({ page }) => {
    // Wait for subtabs
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    
    // Check that summary tab is active (last one because of nested tabs)
    const summaryTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Summary' }).last();
    await expect(summaryTab).toHaveAttribute('aria-selected', 'true');
  });

  test('should handle non-git repository gracefully', async ({ page }) => {
    // Mock the API response for non-git repo
    await page.route('**/api/nodes/*/git/status', async route => {
      await route.fulfill({
        status: 400,
        body: 'Not a git repository'
      });
    });

    // Reload the component
    await page.getByText('Changes').click();
    
    // Should show empty state
    await expect(page.getByText('Not a git repository')).toBeVisible();
  });

  test('should display git status in summary tab', async ({ page }) => {
    // Mock the API response
    await page.route('**/api/nodes/*/git/status', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          added: ['new-file.txt'],
          modified: ['existing-file.js', 'another-file.ts'],
          deleted: ['removed-file.md'],
          untracked: ['temp.log'],
          totalCount: 5
        })
      });
    });

    // Reload the component
    await page.getByText('Changes').click();
    
    // Check badges
    await expect(page.locator('.ant-badge').filter({ hasText: '1' }).first()).toBeVisible(); // Added
    await expect(page.locator('.ant-badge').filter({ hasText: '2' }).first()).toBeVisible(); // Modified
    
    // Check file list
    await expect(page.getByText('new-file.txt')).toBeVisible();
    await expect(page.getByText('existing-file.js')).toBeVisible();
    await expect(page.getByText('another-file.ts')).toBeVisible();
    await expect(page.getByText('removed-file.md')).toBeVisible();
    await expect(page.getByText('temp.log')).toBeVisible();
    
    // Check commit button
    await expect(page.getByRole('button', { name: /Commit All Changes/ })).toBeEnabled();
  });

  test('should handle commit action', async ({ page }) => {
    // Mock the git status API
    await page.route('**/api/nodes/*/git/status', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          added: ['file.txt'],
          modified: [],
          deleted: [],
          untracked: [],
          totalCount: 1
        })
      });
    });

    // Mock the commit API
    await page.route('**/api/nodes/*/git/commit', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          message: 'Commit created successfully',
          commitMessage: '[test-node] commit abc123'
        })
      });
    });

    // Reload to get the mocked status
    await page.getByText('Changes').click();
    
    // Click commit button
    await page.getByRole('button', { name: /Commit All Changes/ }).click();
    
    // Check success message
    await expect(page.getByText('Commit created: [test-node] commit abc123')).toBeVisible();
  });

  test('should display diff in details tab', async ({ page }) => {
    // Mock the diff API
    await page.route('**/api/nodes/*/git/diff', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          files: [{
            path: 'test.js',
            status: 'modified',
            additions: 5,
            deletions: 2,
            patch: '--- a/test.js\n+++ b/test.js\n@@ -1,3 +1,6 @@\n-old line\n+new line\n+added line'
          }]
        })
      });
    });

    // Click on Details tab
    await page.getByRole('tab', { name: 'Details' }).click();
    
    // Check diff content
    await expect(page.getByText('test.js')).toBeVisible();
    await expect(page.getByText('+5')).toBeVisible();
    await expect(page.getByText('-2')).toBeVisible();
    await expect(page.getByText(/old line.*new line/s)).toBeVisible();
  });

  test('should display commit history in history tab', async ({ page }) => {
    // Mock the history API
    await page.route('**/api/nodes/*/git/history', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            hash: 'abc12345',
            author: 'vibethis',
            email: 'vibethis@example.com',
            message: '[test-node] commit abc123',
            timestamp: new Date().toISOString()
          },
          {
            hash: 'def67890',
            author: 'vibethis',
            email: 'vibethis@example.com',
            message: '[test-node] initial commit',
            timestamp: new Date(Date.now() - 86400000).toISOString()
          }
        ])
      });
    });

    // Click on History tab
    await page.getByRole('tab', { name: 'History' }).click();
    
    // Check commit history
    await expect(page.getByText('abc12345')).toBeVisible();
    await expect(page.getByText('[test-node] commit abc123')).toBeVisible();
    await expect(page.getByText('def67890')).toBeVisible();
    await expect(page.getByText('[test-node] initial commit')).toBeVisible();
  });

  test('should show empty state when no changes', async ({ page }) => {
    // Mock empty status
    await page.route('**/api/nodes/*/git/status', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          added: [],
          modified: [],
          deleted: [],
          untracked: [],
          totalCount: 0
        })
      });
    });

    // Reload the component
    await page.getByText('Changes').click();
    
    // Should show empty state
    await expect(page.getByText('No changes to commit')).toBeVisible();
  });
});