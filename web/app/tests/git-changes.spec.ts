import { test, expect } from '@playwright/test';

test.describe('Git Changes Tab', () => {
  // Helper function to navigate to the Changes tab
  async function navigateToChangesTab(page: any) {
    // Wait for the graph to load
    await page.waitForSelector('[data-testid="react-flow-wrapper"]', { timeout: 10000 });
    
    // Click on a node to select it
    await page.locator('.react-flow__node').first().click();
    
    // Wait for side panel to appear
    await page.waitForSelector('.ant-tabs', { timeout: 5000 });
    
    // Click on the Changes tab
    await page.getByText('Changes').click();
  }

  test('should display changes tab with subtabs', async ({ page }) => {
    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Wait for subtabs to be visible
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    
    // Check that the subtabs are visible (use locator for nested tabs)
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Summary' }).last()).toBeVisible();
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'Details' }).last()).toBeVisible();
    await expect(page.locator('.ant-tabs-tab').filter({ hasText: 'History' }).last()).toBeVisible();
  });

  test('should show summary tab by default', async ({ page }) => {
    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Wait for subtabs
    await page.waitForSelector('.ant-tabs-tab', { timeout: 5000 });
    
    // Check that summary tab is active (last one because of nested tabs)
    const summaryTab = page.locator('.ant-tabs-tab').filter({ hasText: 'Summary' }).last();
    await expect(summaryTab).toHaveClass(/ant-tabs-tab-active/);
  });

  test('should handle non-git repository gracefully', async ({ page }) => {
    // Mock the API response for non-git repo BEFORE navigation
    await page.route('**/api/cells/*/git/status', async route => {
      await route.fulfill({
        status: 400,
        body: 'Not a git repository'
      });
    });

    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Should show empty state
    await expect(page.getByText('Not a git repository')).toBeVisible();
  });

  test('should display git status in summary tab', async ({ page }) => {
    // Mock the API response BEFORE navigation
    await page.route('**/api/cells/*/git/status', async route => {
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

    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Wait for the content to load
    await page.waitForTimeout(500);
    
    // Check badges - Ant Design Badge renders count in a sup element
    await expect(page.locator('.ant-badge sup').filter({ hasText: '1' }).first()).toBeVisible(); // Added
    await expect(page.locator('.ant-badge sup').filter({ hasText: '2' }).first()).toBeVisible(); // Modified
    
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
    // Mock the git status API BEFORE navigation
    await page.route('**/api/cells/*/git/status', async route => {
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
    await page.route('**/api/cells/*/git/commit', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          message: 'Commit created successfully',
          commitMessage: '[test-node] commit abc123'
        })
      });
    });

    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Wait for the button to be visible and enabled
    await page.waitForSelector('button:has-text("Commit All Changes")', { timeout: 5000 });
    
    // Click commit button
    await page.getByRole('button', { name: /Commit All Changes/ }).click();
    
    // Check success message
    await expect(page.getByText('Commit created: [test-node] commit abc123')).toBeVisible();
  });

  test('should display diff in details tab', async ({ page }) => {
    // Mock the diff API BEFORE navigation
    await page.route('**/api/cells/*/git/diff', async route => {
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

    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Click on Details tab
    await page.getByRole('tab', { name: 'Details' }).click();
    
    // Wait for content to load
    await page.waitForTimeout(500);
    
    // Check diff content - use more specific selectors
    await expect(page.locator('code').filter({ hasText: 'test.js' })).toBeVisible();
    await expect(page.getByText('+5')).toBeVisible();
    await expect(page.getByText('-2')).toBeVisible();
    // Check for diff content in pre element
    await expect(page.locator('pre').filter({ hasText: 'old line' })).toBeVisible();
  });

  test('should display commit history in history tab', async ({ page }) => {
    // Mock the history API BEFORE navigation
    await page.route('**/api/cells/*/git/history', async route => {
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

    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Click on History tab - use last() to get the nested tab
    await page.locator('.ant-tabs-tab').filter({ hasText: 'History' }).last().click();
    
    // Wait for content to load
    await page.waitForTimeout(500);
    
    // Check commit history
    await expect(page.getByText('abc12345')).toBeVisible();
    await expect(page.getByText('[test-node] commit abc123')).toBeVisible();
    await expect(page.getByText('def67890')).toBeVisible();
    await expect(page.getByText('[test-node] initial commit')).toBeVisible();
  });

  test('should show empty state when no changes', async ({ page }) => {
    // Mock empty status BEFORE navigation
    await page.route('**/api/cells/*/git/status', async route => {
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

    await page.goto('/cells');
    await navigateToChangesTab(page);
    
    // Wait for the component to load
    await page.waitForTimeout(500);
    
    // Should show empty state
    await expect(page.getByText('No changes to commit')).toBeVisible();
  });
});