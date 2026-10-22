import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';

test('confirms cancellation on job details and bulk cancel/delete for selected jobs', async ({ page, request, context, baseURL }) => {
  const tenant = `actions-${randomUUID()}`;
  const base = `/api/projects/${tenant}/jobs`;
  for (const job_id of ['detail-job', 'bulk-one', 'bulk-two', 'untouched']) {
    const response = await request.post(base, { data: { job_id, recipe: 'smoke' } });
    expect(response.status(), await response.text()).toBe(201);
  }
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const mutations: string[] = [];
  page.on('request', request => {
    if (['POST', 'DELETE'].includes(request.method())) mutations.push(new URL(request.url()).pathname);
  });
  await page.goto(`/project/${tenant}/jobs/detail-job/story`);
  await page.getByRole('button', { name: 'Cancel job', exact: true }).click();
  let dialog = page.getByRole('dialog');
  await expect(dialog.getByText('detail-job', { exact: true })).toBeVisible();
  expect(mutations).toEqual([]);
  await dialog.getByRole('button', { name: 'Keep jobs' }).click();
  expect(mutations).toEqual([]);
  await page.getByRole('button', { name: 'Cancel job', exact: true }).click();
  await dialog.getByRole('button', { name: 'Cancel job', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect((await (await request.get(`${base}/detail-job`)).json()).status).toBe('CANCELLED');
  await page.getByRole('button', { name: 'Back to Jobs' }).click();
  await page.getByRole('checkbox', { name: 'Select job bulk-one', exact: true }).check();
  await page.getByRole('checkbox', { name: 'Select job bulk-two', exact: true }).check();
  await page.getByRole('button', { name: 'Cancel selected' }).click();
  dialog = page.getByRole('dialog');
  expect(mutations).toEqual([`${base}/detail-job/cancel`]);
  await dialog.getByRole('button', { name: 'Cancel 2 jobs', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  for (const id of ['bulk-one', 'bulk-two']) {
    expect((await (await request.get(`${base}/${id}`)).json()).status).toBe('CANCELLED');
    await page.getByRole('checkbox', { name: `Select job ${id}`, exact: true }).check();
  }
  await page.getByRole('button', { name: 'Delete selected' }).click();
  await expect(dialog.getByText(/JobDB retains history and artifacts/)).toBeVisible();
  await dialog.getByRole('button', { name: 'Keep jobs' }).click();
  expect((await (await request.get(`${base}?status=all`)).json()).jobs).toHaveLength(4);
  await page.getByRole('button', { name: 'Delete selected' }).click();
  await dialog.getByRole('button', { name: 'Delete 2 jobs', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('link', { name: 'bulk-one', exact: true })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'bulk-two', exact: true })).toHaveCount(0);
  const remaining = (await (await request.get(`${base}?status=all`)).json()).jobs;
  expect(remaining.map((job: { job_id: string }) => job.job_id).sort()).toEqual(['detail-job', 'untouched']);
  expect(remaining.find((job: { job_id: string }) => job.job_id === 'untouched').status).toBe('READY');
  expect(mutations).toHaveLength(5);
});

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

  await page.getByRole('combobox', { name: 'Status filter' }).click();
  await page.getByTitle('Ready', { exact: true }).click();
  await page.getByRole('combobox', { name: 'Status filter' }).press('Escape');
  await page.getByRole('combobox', { name: 'Cell filter' }).click();
  await page.getByTitle('root (self)', { exact: true }).click();
  const filteredURL = page.url();
  const storyResponse = page.waitForResponse(response => response.url().endsWith(`/jobs/${jobID}/story`));
  await page.getByRole('link', { name: jobID, exact: true }).click();
  const story = await storyResponse;
  expect(story.status(), await story.text()).toBe(200);
  expect(await story.json()).toMatchObject({ job_id: jobID });
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await expect(page.getByText('Failed to load job story')).toHaveCount(0);
  // List filters survive selection changes in the story and a full browser reload.
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await page.getByRole('button', { name: 'Back to Jobs' }).click();
  await expect(page).toHaveURL(filteredURL);
  await expect(page.getByRole('link', { name: jobID, exact: true })).toBeVisible();
  await expect(page.getByTitle('Ready', { exact: true })).toBeVisible();
  await expect(page.getByTitle('root (self)', { exact: true })).toBeVisible();
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
