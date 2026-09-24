import { test, expect } from '@playwright/test';

// Run against both the test API and the production binary with remote JobDB.
// No route interception or EventSource replacement: HTTP and SSE cross the API.
test('submits, filters, and opens a c2j job from the browser', async ({ page, request }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('console', message => {
    if (message.type() === 'error') errors.push(message.text());
  });

  const tenant = 'browser-submit';
  const jobID = `browser-job-${Date.now()}`;
  await page.goto(`/?tenantId=${tenant}`);
  await page.getByPlaceholder('Email address').fill('browser@example.com');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/project/${tenant}/jobs$`));
  await expect(page.getByRole('heading', { name: 'Jobs', exact: true })).toBeVisible();

  await page.getByRole('button', { name: 'Submit Job' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Job ID').fill(jobID);
  await dialog.getByLabel('Recipe').fill('smoke');
  await dialog.getByLabel('Inputs').fill('{"message":"from browser"}');
  const submission = page.waitForResponse(response =>
    response.url().endsWith(`/api/projects/${tenant}/jobs`) && response.request().method() === 'POST',
  );
  await dialog.getByRole('button', { name: 'OK', exact: true }).click();
  const response = await submission;
  expect(response.status(), await response.text()).toBe(201);
  expect(await response.json()).toMatchObject({
    job_id: jobID, tenant_id: tenant, recipe: 'smoke', status: 'READY',
    repo: 'https://github.com/cortex-test/platform.git',
    next_route: { jobType: 'recipe' },
  });
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('link', { name: jobID, exact: true })).toBeVisible();

  // The same persisted record must be returned by c2j's repository filter.
  const filtered = await request.get(`/api/projects/${tenant}/jobs?cell=platform&status=READY`);
  expect(filtered.ok()).toBeTruthy();
  expect((await filtered.json()).jobs.map((job: { job_id: string }) => job.job_id)).toContain(jobID);
  const otherCell = await request.get(`/api/projects/${tenant}/jobs?cell=api&status=READY`);
  expect(otherCell.ok()).toBeTruthy();
  expect((await otherCell.json()).jobs).toEqual([]);

  const storyResponse = page.waitForResponse(response => response.url().endsWith(`/jobs/${jobID}/story`));
  await page.getByRole('link', { name: jobID, exact: true }).click();
  const story = await storyResponse;
  expect(story.status(), await story.text()).toBe(200);
  expect(await story.json()).toMatchObject({ job_id: jobID });
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await expect(page.getByText('Failed to load job story')).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('keeps deep-linked tenant, navigation, and live input activity in sync', async ({ page, context, baseURL, request }) => {
  const tenant = 'browser-deep-link';
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  await page.addInitScript(() => localStorage.setItem('cortex:tenantId', 'wrong-tenant'));
  const streamResponse = page.waitForResponse(response => response.url().endsWith(`/projects/${tenant}/user-inputs/stream`));
  const pendingResponse = page.waitForResponse(response => response.url().endsWith(`/projects/${tenant}/user-inputs/pending`));
  await page.goto(`/project/${tenant}/cells`);
  expect((await streamResponse).status()).toBe(200);
  expect((await pendingResponse).status()).toBe(200);
  await expect(page.getByLabel('Tenant ID')).toHaveValue(tenant);
  await expect(page).toHaveTitle(`cortex: tenant ${tenant}`);
  const rows = page.locator('.ant-table-tbody > tr');
  await expect(rows).toHaveCount(2);
  await page.getByPlaceholder('Search cells').fill('platform');
  await expect(rows).toHaveCount(1);
  await page.getByRole('link', { name: 'Pending Inputs' }).click();
  await expect(page).toHaveURL(new RegExp(`/project/${tenant}/inputs$`));
  await expect(page.getByText('No pending inputs')).toBeVisible();

  await page.getByLabel('Tenant ID').fill('browser-other');
  await page.getByRole('button', { name: 'Use', exact: true }).click();
  await expect(page).toHaveURL(/\/project\/browser-other\/jobs$/);
  await expect(page).toHaveTitle('cortex: tenant browser-other');
  const jobs = await request.get('/api/projects/browser-other/jobs');
  expect(jobs.ok()).toBeTruthy();
  expect((await jobs.json()).jobs).toEqual([]);
});

test('opens the tenant from the server connection config without a URL override', async ({ page, context, baseURL }, testInfo) => {
  const tenant = testInfo.project.name === 'production' ? 'c2' : '1';
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  // A previous Cortex instance may have left a different tenant in storage.
  await page.addInitScript(() => localStorage.setItem('cortex:tenantId', 'stale-tenant'));
  const jobs = page.waitForResponse(r => new URL(r.url()).pathname === `/api/projects/${tenant}/jobs`);
  const stream = page.waitForResponse(r => r.url().endsWith(`/api/projects/${tenant}/user-inputs/stream`));
  await page.goto('/');
  expect((await jobs).status()).toBe(200);
  expect((await stream).status()).toBe(200);
  await expect(page).toHaveURL(new RegExp(`/project/${tenant}/jobs$`));
  await expect(page.getByLabel('Tenant ID')).toHaveValue(tenant);
  await expect(page.getByRole('heading', { name: 'Jobs', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Pending Inputs' }).click();
  await expect(page.getByText('No pending inputs')).toBeVisible();
});
