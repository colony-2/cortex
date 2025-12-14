import { test, expect } from '@playwright/test';

const sampleProject = {
  id: 'proj1',
  name: 'Test Project',
  gitRepoPath: '/tmp/repo',
  version: 1,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
};

const sampleGraph = {
  cells: [
    { id: 'cell-a', name: 'Cell A', path: '/a', type: 'service', dependencies: ['cell-b'] },
    { id: 'cell-b', name: 'Cell B', path: '/b', type: 'library', dependencies: [] },
  ],
  edges: [{ id: 'edge-1', source: 'cell-a', target: 'cell-b' }],
};

test.describe('Console cleanliness', () => {
  test.beforeEach(async ({ page }) => {
    // Stub EventSource to avoid SSE errors in test
    await page.addInitScript(() => {
      (window as any).EventSource = class {
        readyState = 1;
        constructor() {}
        close() {}
        addEventListener() {}
        removeEventListener() {}
      } as any;
    });

    // Mock projects
    await page.route('**/api/projects', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([sampleProject]),
      });
    });

    // Mock graph
    await page.route('**/api/projects/proj1/graph', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(sampleGraph),
      });
    });

    // Mock user input endpoints
    await page.route('**/api/user-inputs/**', async (route) => {
      const url = route.request().url();
      if (url.includes('/pending')) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
      } else {
        await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
      }
    });
  });

  test('loads graph view with no console errors', async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    await page.goto('/project/proj1/cells');

    await expect(page.locator('.react-flow__node')).toHaveCount(2, { timeout: 5000 });
    await expect(page.locator('.react-flow__edge')).toHaveCount(1, { timeout: 5000 });

    expect(consoleErrors).toEqual([]);
  });
});

