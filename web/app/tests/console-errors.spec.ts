import { jobIdentity, mockCells, cellFixture } from './helpers/cells';
import { test, expect } from '@playwright/test';

const sampleJobs = {
  jobs: [
    {
      ...jobIdentity, tenant_id: '1',
      job_id: 'job-console-1',
      prompt: 'Build the platform',
      status: 'READY',
      store: 'ACTIVE',
      job_type: 'recipe',
      recipe: 'default',
      repo: 'https://github.com/colony-2/platform.git',
      cell_name: 'platform',
      git_ref: 'main',
      created_at: new Date().toISOString(),
      available_at: new Date().toISOString(),
    },
  ],
};

test.describe('Console cleanliness', () => {
  test.beforeEach(async ({ page, context, baseURL }) => {
    await context.addCookies([
      {
        name: 'colony2:user:email',
        value: 'test@example.com',
        url: baseURL!,
      },
    ]);

    await page.addInitScript(() => {
      (window as any).EventSource = class {
        readyState = 1;
        constructor() {}
        close() {}
        addEventListener() {}
        removeEventListener() {}
      } as any;
    });

    await mockCells(page, ['platform', 'api']);

    await page.route(/\/api\/projects\/[^/]+\/jobs(\?.*)?$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(sampleJobs),
      });
    });

    await page.route(/\/api\/projects\/[^/]+\/user-inputs\/pending$/, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: '[]',
      });
    });
  });

  test('loads jobs and cells with no console errors', async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(msg.text());
      }
    });

    await page.goto('/project/1/jobs');

    await expect(page).toHaveTitle(/cortex: tenant 1/);
    await expect(page.getByRole('heading', { name: 'Goals' })).toBeVisible();
    await expect(page.getByText('Build the platform')).toBeVisible();

    await page.getByRole('link', { name: 'Cells' }).click();

    await expect(page.getByRole('heading', { name: 'Cells' })).toBeVisible();
    const rows = page.locator('.ant-table-tbody > tr[data-row-key]');
    await expect(rows).toHaveCount(2, { timeout: 5000 });
    await expect(rows.first().getByText('platform', { exact: true })).toBeVisible();
    await expect(page.getByRole('table').getByText('github.com/acme/api')).toBeVisible();

    expect(consoleErrors).toEqual([]);
  });
});
