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
test('creates a prompt-only goal and restores expanded list state through navigation, reload, back and forward', async ({ page, context, baseURL, request }) => {
  const tenant = `goals-${randomUUID()}`;
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  await page.goto(`/project/${tenant}/goals`);
  await page.getByRole('button', { name: 'New goal', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('Recipe')).toHaveCount(0);
  await expect(dialog.getByLabel('Job ID')).toHaveCount(0);
  const prompt = Array.from({ length: 85 }, (_, i) => `word${i}`).join(' ');
  await dialog.getByLabel('Prompt').fill(prompt);
  const submitted = page.waitForResponse(r => r.url().endsWith(`/projects/${tenant}/jobs`) && r.request().method() === 'POST');
  await dialog.getByRole('button', { name: 'Create goal', exact: true }).click();
  const response = await submitted;
  expect(response.status(), await response.text()).toBe(201);
  const job = await response.json();
  expect(job).toMatchObject({ prompt, recipe: 'build', tenant_id: tenant, status: 'READY' });
  expect(job.goal_id).toBeTruthy();
  await expect(page).toHaveURL(new RegExp(`/goals/${job.goal_id}$`));
  const goalURL = page.url();
  const abbreviated = `${prompt.split(' ').slice(0, 75).join(' ')}…`;
  await expect(page.getByRole('link', { name: abbreviated, exact: true })).toBeVisible();
  const row = page.getByRole('link', { name: `Job ${job.job_id}: Ready`, exact: true }).locator('xpath=ancestor::tr');
  await row.getByRole('button', { name: 'Expand row', exact: true }).click();
  const expandedURL = page.url();
  await page.getByText('Execution requirements', { exact: true }).click();
  const detailURL = page.url();
  await page.getByRole('link', { name: job.job_id, exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(detailURL);
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toHaveAttribute('open', '');
  await page.goForward();
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await page.reload();
  await page.getByRole('button', { name: 'Back to Jobs' }).click();
  await expect(page).toHaveURL(detailURL);
  await page.reload();
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toHaveAttribute('open', '');
  // Navigating to the same goal without its query must not lose the prior entry.
  await page.getByRole('link', { name: job.goal_id, exact: true }).click();
  await expect(page).toHaveURL(goalURL);
  await expect(page.getByText('Execution requirements', { exact: true })).toBeHidden();
  await page.goBack();
  await expect(page).toHaveURL(detailURL);
  await expect(page.getByText('Execution requirements', { exact: true })).toBeVisible();
  expect(expandedURL).toContain('expanded=');
  const listed = await request.get(`/api/projects/${tenant}/jobs?status=all&goal=${job.goal_id}`);
  expect((await listed.json()).jobs).toHaveLength(1);
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
  await expect(page.getByRole('heading', { name: 'Goals', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Pending Inputs' }).click();
  await expect(page.getByText('No pending inputs')).toBeVisible();
});
