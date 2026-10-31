import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, copyFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';

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
  const directJob = await (await request.get(`${base}/detail-job`)).json();
  await expect(page.getByRole('link', { name: 'Goal overview', exact: true })).toHaveAttribute('href', `/project/${tenant}/goals/${directJob.goal_id}`);
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
  await page.getByRole('button', { name: 'Back to Goals' }).click();
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
  await expect(page.getByRole('heading', { name: 'Goal overview', exact: true })).toBeVisible();
  await expect(page.getByText(prompt, { exact: true })).toBeVisible();
  await expect(page.getByRole('checkbox')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Expand row' })).toHaveCount(0);
  await page.getByRole('link', { name: 'Back to Goals', exact: true }).click();
  const abbreviated = `${prompt.slice(0, 75).trimEnd()}...`;
  const name = page.getByRole('link', { name: abbreviated, exact: true });
  await expect(name).toBeVisible();
  await expect(name).toHaveCSS('text-decoration-line', 'none');
  const row = name.locator('xpath=ancestor::tr');
  await row.getByRole('button', { name: 'Expand row', exact: true }).click();
  await page.getByText('Execution requirements', { exact: true }).click();
  const detailURL = page.url();
  await name.click();
  await expect(page).toHaveURL(new RegExp(`/jobs/${job.job_id}/story`));
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job prompt' }).locator('div.ant-typography')).toHaveText(prompt);
  await page.goBack();
  await expect(page).toHaveURL(detailURL);
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toHaveAttribute('open', '');
  await page.goForward();
  await expect(page.getByRole('heading', { name: 'Job Story' })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('region', { name: 'Job prompt' }).locator('div.ant-typography')).toHaveText(prompt);
  await page.getByRole('link', { name: 'Goal overview', exact: true }).click();
  await expect(page).toHaveURL(goalURL);
  await expect(page.getByRole('button', { name: 'Expand row' })).toHaveCount(0);
  await page.getByRole('link', { name: 'Open execution', exact: true }).click();
  await page.getByRole('button', { name: 'Back to Goals' }).click();
  await expect(page).toHaveURL(detailURL);
  await page.reload();
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toHaveAttribute('open', '');
  await page.getByRole('link', { name: 'Goal overview', exact: true }).click();
  await expect(page).toHaveURL(goalURL);
  await page.goBack();
  await expect(page).toHaveURL(detailURL);
  await expect(page.getByText('Execution requirements', { exact: true })).toBeVisible();
  expect(detailURL).toContain('expanded=');
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
  await page.getByRole('link', { name: 'Reviews' }).click();
  await expect(page).toHaveURL(new RegExp(`/project/${tenant}/reviews$`));
  await expect(page.getByText('No pending reviews')).toBeVisible();

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
  await page.getByRole('link', { name: 'Reviews' }).click();
  await expect(page.getByText('No pending reviews')).toBeVisible();
});

// Real downloads in both projects; production additionally crosses HTTP JobDB and SQLite.
test('downloads persisted job chapters as JSON without changing the story route', async ({ page, request, context, baseURL }, testInfo) => {
  const { readFile } = await import('node:fs/promises');
  const tenant = `chapters-${randomUUID()}`;
  const jobId = 'chapter-export';
  const route = `/project/${tenant}/jobs/${jobId}/story`;
  const submitted = await request.post(`/api/projects/${tenant}/jobs`, {
    data: { job_id: jobId, recipe: 'smoke', inputs: { message: '雪' } },
  });
  expect(submitted.status(), await submitted.text()).toBe(201);
  let expectedCount = 1;
  const output = { result: '雪', binary: 'application JSON remains opaque' };
  const artifactContent = Buffer.from('artifact body excluded from chapter export');
  if (testInfo.project.name === 'production') {
    const backend = `http://127.0.0.1:18082/v1/tenants/${tenant}/jobs/${jobId}`;
    const leaseResponse = await request.post(`${backend}/lease`, {
      data: { workerId: 'chapter-download-browser-test', routes: [{ jobType: 'recipe' }], leaseDuration: '1m' },
    });
    expect(leaseResponse.ok(), await leaseResponse.text()).toBeTruthy();
    const { lease } = await leaseResponse.json();
    expect(lease).toBeTruthy();
    const leasePath = `${backend}/leases/${lease.leaseId}`;
    const headers = { 'X-JobDB-Lease-Token': lease.leaseToken };
    for (const [index, record] of [
      { version: 4, kind: 'header', versions: { execution_pattern_version: 4, jobdb_version: 'test' } },
      { version: 4, kind: 'restart', id: 'retained-attempt', source: { tenantId: tenant, jobId }, descriptor: [{ version: 4, kind: 'scope', id: 'past-scope', descriptor: { restart: true } }] },
    ].entries()) {
      const response = await request.post(`${leasePath}/add_chapter`, {
        headers,
        data: { chapter: { ordinal: index + 1, taskType: '__execution__', createdAt: new Date().toISOString(), body: { kind: 'execution', record }, artifacts: [] } },
      });
      expect(response.status(), await response.text()).toBe(204);
    }
    const complete = await request.post(`${leasePath}/complete`, {
      headers,
      data: {
        status: 'success',
        chapter: {
          ordinal: 3, taskType: 'recipe', createdAt: new Date().toISOString(), metadata: { fields: { attempt: { kind: 'int', intValue: 2 }, title: { kind: 'string', stringValue: '雪' } } },
          body: { kind: 'jobAttemptOutcome', outcome: { kind: 'success', output } }, artifacts: [],
        },
        artifactUploads: [{ name: 'result.txt', size: artifactContent.length, contentBase64: artifactContent.toString('base64') }],
      },
    });
    expect(complete.status(), await complete.text()).toBe(204);
    expectedCount = 4;
  }
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  await page.goto(route);
  const downloadEvent = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download chapters', exact: true }).click();
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toBe(`job-${jobId}-chapters.json`);
  expect(await download.failure()).toBeNull();
  const file = await download.path();
  expect(file).toBeTruthy();
  const text = await readFile(file!, 'utf8');
  const exported = JSON.parse(text);
  expect(text).toContain('\n  "format_version": 1,');
  expect(exported).toMatchObject({ format_version: 1, project_id: tenant, job_id: jobId });
  expect(exported.chapters.map((chapter: { ordinal: number }) => chapter.ordinal)).toEqual(Array.from({ length: expectedCount }, (_, index) => index));
  expect(exported.chapters[0].body.kind).toBe('job_start');
  expect(exported.chapters[0].body.input.encoding).toBe('base64');
  const input = JSON.parse(Buffer.from(exported.chapters[0].body.input.data, 'base64').toString('utf8'));
  expect(JSON.stringify(input)).toContain('"message":"雪"');
  if (expectedCount === 4) {
    expect(exported.chapters[2].body.record.descriptor[0].id).toBe('past-scope');
    expect(exported.chapters[3].body.kind).toBe('job_attempt_outcome');
    expect(exported.chapters[3].body.outcome.kind).toBe('application_output');
    expect(exported.chapters[3].body.outcome.output.encoding).toBe('base64');
    expect(JSON.parse(Buffer.from(exported.chapters[3].body.outcome.output.data, 'base64').toString('utf8'))).toEqual(output);
    // JobDB retains supplied metadata inside its typed chapter metadata envelope.
    expect(exported.chapters[3].metadata.metadata).toMatchObject({ kind: 'map', value: { attempt: { kind: 'int', value: 2 }, title: { kind: 'string', value: '雪' } } });
    expect(exported.chapters[3].artifacts).toEqual([{ name: 'result.txt', size: artifactContent.length, digest: expect.any(String) }]);
    expect(text).not.toContain(artifactContent.toString());
  }
  await expect(page).toHaveURL(new RegExp(`${route}$`));
  await expect(page.getByRole('button', { name: 'Download chapters', exact: true })).toBeEnabled();
});


test('wraps long job story context without overflowing its header', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const title = 'LongTitle'.repeat(65);
  const prompt = 'LongPrompt'.repeat(100) + '\nSecond prompt line';
  const detail = 'LongDiagnostic'.repeat(100) + '\nSecond diagnostic line';
  await page.route('**/api/projects/layout/jobs/layout-job', route => route.fulfill({ json: {
    tenant_id: 'layout', job_id: 'layout-job', status: 'COMPLETED', store: 'ARCHIVED',
    title, prompt, completion_status: 'failed_app', completion_detail: detail,
  } }));
  await page.route('**/api/projects/layout/jobs/layout-job/story', route => route.fulfill({ json: {
    job_id: 'layout-job', status: 'failed', root: null,
  } }));
  await page.goto('/project/layout/jobs/layout-job/story');
  for (const width of [390, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    const heading = page.getByRole('heading', { level: 2, name: `Job Story: ${title}` });
    const promptRegion = page.getByRole('region', { name: 'Job prompt' });
    const failure = page.getByRole('alert').filter({ hasText: 'Job failed' });
    await expect(heading).toBeVisible();
    await expect(promptRegion.locator('div.ant-typography')).toHaveText(prompt);
    await expect(failure).toContainText(detail);
    const header = page.getByTestId('job-story-header');
    for (const element of [header, promptRegion, failure]) {
      const box = await element.boundingBox();
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
      expect(await element.evaluate(node => node.scrollWidth - node.clientWidth), await element.getAttribute('class') || 'context').toBeLessThanOrEqual(1);
    }
    const headingBox = await header.boundingBox();
    const promptBox = await promptRegion.boundingBox();
    const failureBox = await failure.boundingBox();
    expect(promptBox!.y).toBeGreaterThanOrEqual(headingBox!.y + headingBox!.height);
    expect(failureBox!.y).toBeGreaterThanOrEqual(promptBox!.y + promptBox!.height);
    await expect(page.getByRole('button', { name: /Refresh/ })).toBeVisible();
  }
});

// This crosses real Git recipe resolution, worker completion, HTTP JobDB,
// Cortex's detail projection, and the UI embedded in the production executable.
test('shows persisted prompt and failure on the production story through direct and list navigation', async ({ page, request, context, baseURL }, testInfo) => {
  test.skip(testInfo.project.name !== 'production', 'Requires the production JobDB and pinned worker harness');
  test.setTimeout(90_000);
  const tenant = `story-${randomUUID()}`;
  const jobID = `story-${randomUUID()}`;
  const repo = mkdtempSync(join(tmpdir(), 'cortex-story-'));
  const prompt = 'Story prompt <literal> 雪\nSecond prompt line';
  const marker = 'Story context diagnostic <literal> 雪\nSecond diagnostic line';
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  let worker: ReturnType<typeof spawn> | undefined;
  let workerLog = '';
  try {
    mkdirSync(join(repo, '.c2/recipes'), { recursive: true });
    copyFileSync(resolve('tests/fixtures/recipes/browser-story-failure.yaml'), join(repo, '.c2/recipes/failure.yaml'));
    const git = (...args: string[]) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' });
    git('init', '-b', 'main');
    git('add', '.');
    git('-c', 'user.name=Cortex Browser Test', '-c', 'user.email=browser@example.com', '-c', 'commit.gpgsign=false', 'commit', '-m', 'Story failure fixture');
    const recipe = `git+${pathToFileURL(repo).href}//.c2/recipes/failure.yaml@main`;
    const base = `/api/projects/${tenant}/jobs`;
    const submitted = await request.post(base, { data: { job_id: jobID, recipe, repo: pathToFileURL(repo).href, inputs: { prompt } } });
    expect(submitted.status(), await submitted.text()).toBe(201);
    worker = spawn(resolve('../../build/c2j-test-worker'), [
      'run', 'one', '--job-id', jobID, '--input-mode', 'ops', '--jobdb', `http://127.0.0.1:18082/${tenant}`,
    ], { cwd: repo, env: { ...process.env, GIT_TERMINAL_PROMPT: '0' }, stdio: ['ignore', 'pipe', 'pipe'] });
    worker.stdout!.on('data', data => { workerLog += data.toString(); });
    worker.stderr!.on('data', data => { workerLog += data.toString(); });
    worker.on('error', error => { workerLog += error.message; });
    await expect.poll(async () => {
      const response = await request.get(`${base}/${jobID}`);
      expect(response.status()).toBe(200);
      return (await response.json()).completion_status;
    }, { timeout: 45_000, message: 'Worker must persist a terminal failure' }).toBe('failed_app');
    const stored = await (await request.get(`${base}/${jobID}`)).json();
    expect(stored.prompt).toBe(prompt);
    expect(stored.completion_detail).toContain(marker);
    await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
    const assertContext = async () => {
      const region = page.getByRole('region', { name: 'Job prompt' });
      await expect(region.locator('div.ant-typography')).toHaveText(prompt);
      const failure = page.getByRole('alert').filter({ hasText: 'Job failed' });
      await expect(failure).toContainText(stored.completion_detail);
      expect(await region.evaluate(node => node.textContent)).toBe(`Prompt${prompt}`);
      await expect(page.getByText('Job details unavailable', { exact: true })).toHaveCount(0);
    };
    await page.goto(`/project/${tenant}/jobs/${jobID}/story`);
    await assertContext();
    await page.goto(`/project/${tenant}/jobs`);
    await page.getByRole('link', { name: `Job ${jobID}: Failed (application)`, exact: true }).click();
    await assertContext();
    expect(errors).toEqual([]);
  } finally {
    if (worker?.pid && worker.exitCode === null && worker.signalCode === null) {
      const stopped = new Promise<void>(done => worker!.once('close', () => done()));
      worker.kill('SIGTERM');
      const timer = setTimeout(() => worker?.kill('SIGKILL'), 5000);
      await stopped;
      clearTimeout(timer);
    }
    await testInfo.attach('story-worker.log', { body: workerLog, contentType: 'text/plain' });
    rmSync(repo, { recursive: true, force: true });
  }
});
