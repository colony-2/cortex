import { test, expect } from '@playwright/test';

const sampleProject = {
  id: 'proj1',
  name: 'Test Project',
  gitRepoPath: '/tmp/repo',
  version: 1,
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
};

const sampleCells = [
  {
    id: 'cell-a',
    name: 'Cell A',
    workingPath: '/a',
    updatedAt: new Date().toISOString(),
    dependencies: ['cell-b'],
  },
  {
    id: 'cell-b',
    name: 'Cell B',
    workingPath: '/b',
    updatedAt: new Date().toISOString(),
    dependencies: [],
  },
];

test.describe('Console cleanliness', () => {
  test.beforeEach(async ({ page, context }) => {
    await context.addCookies([
      {
        name: 'colony2:user:email',
        value: 'test@example.com',
        url: 'http://localhost:5173',
      },
    ]);

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

    await page.route('**/api/projects/proj1/cells', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(sampleCells),
      });
    });

    await page.route('**/api/projects/proj1/user-inputs/pending', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: '[]',
      });
    });
  });

  test('loads the cells list with no console errors', async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    await page.goto('/project/proj1/cells');

    await expect(page).toHaveTitle(/Test Project/);
    await expect(page.getByRole('heading', { name: 'Cells' })).toBeVisible();
    await expect(page.locator('.ant-table-tbody > tr')).toHaveCount(2, { timeout: 5000 });
    await expect(page.getByText('Cell A')).toBeVisible();
    await expect(page.getByText('Cell B')).toBeVisible();
    await expect(page.getByText('/a')).toBeVisible();
    await expect(page.getByText('/b')).toBeVisible();

    expect(consoleErrors).toEqual([]);
  });
});
