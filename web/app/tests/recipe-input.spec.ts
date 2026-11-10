import { registerCell } from './helpers/cells';
import { fixtureGitTransport } from './helpers/gitTransport';
import { abbreviatePrompt } from '../src/utils/goalPresentation';
import { setStatusFilter } from './helpers/goalList';
import { expect, type Page } from '@playwright/test';
import { test as base, setNativeZoom } from './helpers/nativeZoom';
import { spawn, execFileSync, type ChildProcess } from 'node:child_process';
import { mkdtempSync, mkdirSync, copyFileSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';

async function openPendingInput(page: Page) {
  await page.getByRole('tab', { name: 'Execution', exact: true }).click();
  await page.getByRole('tab', { name: 'Pending Input', exact: true }).click();
}

const reviewGoalPrompt = 'Build a release dashboard with clear review context and reliable metadata. '.repeat(2);

// Real production Cortex, HTTP JobDB, pinned c2j CLI, Git recipe, and browser SSE.
const test = base.extend<{
  recipeFixture: string;
  recipeRun: {
    tenant: string;
    jobID: string;
    observer: Page;
    startWorker: (jobID?: string) => void;
    stopWorker: () => Promise<void>;
  };
}>({
  recipeFixture: ['browser-input', { option: true }],
  recipeRun: async ({ page, context, request, recipeFixture }, use, testInfo) => {
    const review = recipeFixture.startsWith('browser-review');
    if (review) {
      // Match browsers serving Cortex over HTTP outside localhost.
      await context.addInitScript(() => Object.defineProperty(globalThis.crypto, 'randomUUID', { value: undefined }));
    }
    const tenant = `input-${randomUUID()}`;
    await registerCell(request, tenant);
    const jobID = `recipe-${randomUUID()}`;
    const repo = mkdtempSync(join(tmpdir(), 'cortex-input-'));
    const workerPath = resolve('../../build/c2j-test-worker');
    let worker: ChildProcess | undefined;
    let workerTimer: ReturnType<typeof setTimeout> | undefined;
    let workerStopped = true;
    let workerLog = '';
    let fixtureFailed = false;
    const errors: string[] = [];
    context.on('page', browserPage => browserPage.on('pageerror', error => errors.push(error.message)));
    page.on('pageerror', error => errors.push(error.message));

    const workerEnv = fixtureGitTransport(repo);
    const startWorker = (targetJobID = jobID) => {
      workerStopped = false;
      const launch = () => {
        worker = spawn(workerPath, [
          'run', 'one', '--job-id', targetJobID, '--input-mode', 'ops',
          '--poll-interval', '100ms', '--jobdb', `http://127.0.0.1:18082/${tenant}`,
        ], {
          cwd: repo,
          env: workerEnv,
          stdio: ['ignore', 'pipe', 'pipe'],
        });
        worker.stdout!.on('data', data => { workerLog += data.toString(); });
        worker.stderr!.on('data', data => { workerLog += data.toString(); });
        worker.on('error', error => { workerLog += error.stack; });
        worker.on('close', code => {
          // run one exits with 3 while awaiting external input. Continue polling
          // so browser answers and input deadlines resume the same job.
          if (!workerStopped && code === 3) workerTimer = setTimeout(launch, 500);
        });
      };
      launch();
    };
    const stopWorker = async () => {
      workerStopped = true;
      clearTimeout(workerTimer);
      if (!worker?.pid || worker.exitCode !== null || worker.signalCode !== null) return;
      const stopped = new Promise<void>(done => worker!.once('close', () => done()));
      worker.kill('SIGTERM');
      const timeout = setTimeout(() => worker?.kill('SIGKILL'), 5_000);
      await stopped;
      clearTimeout(timeout);
    };

    try {
      mkdirSync(join(repo, '.c2/recipes'), { recursive: true });
      copyFileSync(resolve(`tests/fixtures/recipes/${recipeFixture}.yaml`), join(repo, '.c2/recipes/browser-input.yaml'));
      if (recipeFixture === 'browser-review-formats') copyFileSync(resolve('tests/fixtures/recipes/browser-document-producer.yaml'), join(repo, '.c2/recipes/producer.yaml'));
      const git = (...args: string[]) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' });
      git('init', '-b', 'main');
      git('add', '.c2');
      git('-c', 'user.name=Cortex Browser Test', '-c', 'user.email=browser@example.com', '-c', 'commit.gpgsign=false', 'commit', '-m', 'Input recipe fixture');
      const recipe = `git+${pathToFileURL(repo).href}//.c2/recipes/browser-input.yaml@main`;

      await page.goto(`/?tenantId=${tenant}`);
      await page.getByPlaceholder('Email address').fill('browser@example.com');
      await page.getByRole('button', { name: 'Continue', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Goals', exact: true })).toBeVisible();

      // This second tab must update through SSE, without refresh or navigation.
      const observer = await context.newPage();
      await observer.goto(`/project/${tenant}/reviews`);
      await expect(observer.getByText('No pending reviews')).toBeVisible();

      let externalDocument: unknown;
      if (recipeFixture === 'browser-review-formats') {
        const producerID = `producer-${randomUUID()}`;
        const producer = await request.post(`/api/projects/${tenant}/jobs`, { data: { job_id: producerID, recipe: `git+${pathToFileURL(repo).href}//.c2/recipes/producer.yaml@main`, cell: 'platform', repo: 'github.com/cortex-test/platform' } });
        expect(producer.status(), await producer.text()).toBe(201);
        startWorker(producerID);
        await expect.poll(async () => {
          const result = await request.get(`/api/projects/${tenant}/jobs/${producerID}/outcome`);
          if (result.status() !== 200) return false;
          const body = await result.json(); externalDocument = body.output?.document; return !!externalDocument;
        }, { timeout: 30000 }).toBe(true);
        await stopWorker();
        const mainRecipe = join(repo, '.c2/recipes/browser-input.yaml');
        writeFileSync(mainRecipe, readFileSync(mainRecipe, 'utf8').replace('__EXTERNAL_DOCUMENT__', JSON.stringify(externalDocument)));
        git('add', '.c2');
        git('-c', 'user.name=Cortex Browser Test', '-c', 'user.email=browser@example.com', '-c', 'commit.gpgsign=false', 'commit', '-m', 'Bind stored cross-job review fixture');
      }
      const submitted = await request.post(`/api/projects/${tenant}/jobs`, {
        data: { job_id: jobID, recipe, cell: 'platform', repo: 'github.com/cortex-test/platform', ...(review ? { inputs: { prompt: reviewGoalPrompt } } : {}) },
      });
      expect(submitted.status(), await submitted.text()).toBe(201);
      startWorker();

      // Check the backend separately so a broken persisted form reports its
      // HTTP error instead of looking like a browser/SSE visibility timeout.
      let pendingReadFailure: { status: number; body: unknown } | undefined;
      await expect.poll(async () => {
        if (pendingReadFailure) return pendingReadFailure;
        const response = await request.get(`/api/projects/${tenant}/user-inputs/pending`);
        const result = { status: response.status(), body: await response.json() };
        if (response.status() >= 500) {
          pendingReadFailure = result;
          await testInfo.attach('pending-input-error.json', {
            body: JSON.stringify(result, null, 2), contentType: 'application/json',
          });
        }
        return result;
      }, { timeout: 30_000, message: 'The worker must publish a readable pending input' }).toMatchObject({
        status: 200,
        body: expect.arrayContaining([expect.objectContaining({ id: jobID })]),
      });
      await expect(observer.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible({ timeout: 30_000 });
      const otherTenant = await request.get(`/api/projects/other-${tenant}/user-inputs/pending`);
      expect(await otherTenant.json()).toEqual([]);

      await page.getByRole('button', { name: 'Refresh', exact: true }).click();
      await expect(page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) })).toBeVisible();
      await use({ tenant, jobID, observer, startWorker, stopWorker });
      expect(errors).toEqual([]);
    } catch (error) {
      fixtureFailed = true;
      throw error;
    } finally {
      await stopWorker();
      try {
        await testInfo.attach('c2j-worker.log', { body: workerLog, contentType: 'text/plain' });
        const outcome = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
        await testInfo.attach('recipe-outcome.json', { body: await outcome.body(), contentType: 'application/json' });
        if (fixtureFailed || testInfo.status !== testInfo.expectedStatus) {
          for (const [name, path] of [
            ['pending-inputs', 'user-inputs/pending'],
            ['input-details', `user-inputs/${jobID}`],
            ['job-metadata', `jobs/${jobID}`],
          ]) {
            try {
              const response = await request.get(`/api/projects/${tenant}/${path}`);
              await testInfo.attach(`${name}.json`, {
                body: JSON.stringify({ status: response.status(), body: await response.text() }, null, 2),
                contentType: 'application/json',
              });
            } catch (error) {
              // Diagnostics must preserve the original failure, including
              // when an expired input is gone or a server has stopped.
              await testInfo.attach(`${name}-error.txt`, {
                body: String(error), contentType: 'text/plain',
              });
            }
          }
        }
      } finally {
        rmSync(repo, { recursive: true, force: true });
      }
    }
  },
});

test.beforeEach(() => test.setTimeout(90_000));

test.describe('cancel review job', () => {
  test.use({ recipeFixture: 'browser-review-clarification' });
  test('confirms cancellation without submitting a review response and clears open tabs', async ({ page, request, recipeRun }) => {
    const { tenant, jobID, observer, stopWorker } = recipeRun;
    await stopWorker();
    let releaseMetadata!: () => void;
    const metadataGate = new Promise<void>(done => { releaseMetadata = done; });
    await page.route(`**/api/projects/${tenant}/jobs/${jobID}`, async route => {
      await metadataGate;
      await route.continue();
    });
    const mutations: string[] = [];
    page.on('request', req => { if (req.method() === 'POST') mutations.push(req.url()); });
    await page.goto(`/project/${tenant}/reviews/${jobID}`);
    await page.getByRole('button', { name: 'Revise the plan', exact: true }).click();
    const feedback = page.getByLabel('Feedback or answers to open questions. Select a revision decision to request changes.');
    await feedback.fill('Keep this draft if I dismiss cancellation.');
    await page.getByRole('button', { name: 'Cancel job', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText(jobID, { exact: true })).toBeVisible();
    await dialog.getByRole('button', { name: 'Keep jobs' }).click();
    expect(mutations).toEqual([]);
    await expect(feedback).toHaveValue('Keep this draft if I dismiss cancellation.');
    await page.getByRole('button', { name: 'Cancel job', exact: true }).click();
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url().endsWith(`/jobs/${jobID}/cancel`) && r.request().method() === 'POST'),
      dialog.getByRole('button', { name: 'Cancel job', exact: true }).click(),
    ]);
    expect(response.status()).toBe(204);
    await expect(page).toHaveURL(new RegExp(`/project/${tenant}/reviews$`));
    await expect(page.getByText('No pending reviews')).toBeVisible();
    await expect(observer.getByText('No pending reviews')).toBeVisible();
    expect((await (await request.get(`/api/projects/${tenant}/jobs/${jobID}`)).json()).status).toBe('CANCELLED');
    releaseMetadata();
    await expect(page.getByText('No pending reviews')).toBeVisible();
    expect(mutations).toEqual([response.url()]);
  });
});

for (const multiField of [false, true]) {
  test.describe(multiField ? 'multi-field' : 'single-question', () => {
    test.use({ recipeFixture: multiField ? 'browser-input-fields' : 'browser-input' });
    test('answers recipe input and resumes the worker to completion', async ({ page, request, recipeRun }) => {
      const { tenant, jobID, observer, startWorker, stopWorker } = recipeRun;
      const answer = `approved-${randomUUID()}`;
      // Kill the worker while the request is pending: the answer must survive
      // in JobDB and be picked up by a fresh worker, not an in-process promise.
      await stopWorker();
      if (multiField) {
        await page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) }).click();
        await openPendingInput(page);
      } else {
        const inspections: string[] = [];
        page.on('request', req => {
          if (req.url().endsWith(`/user-inputs/${jobID}`) || req.url().endsWith(`/jobs/${jobID}`)) inspections.push(req.url());
        });
        await page.setViewportSize({ width: 390, height: 844 });
        await page.getByRole('button', { name: 'Open navigation' }).click();
        await page.getByRole('link', { name: 'Reviews', exact: true }).click();
        await expect(page.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible();
        expect(inspections).toEqual([]);
        await expect(page.getByRole('link', { name: 'Pending Inputs', exact: true })).toHaveCount(0);
        await page.getByRole('button', { name: 'Open Review', exact: true }).click();
        await expect(page).toHaveURL(new RegExp(`/reviews/${jobID}$`));
      }
      const question = multiField ? 'Release note' : 'What should the recipe publish?';
      await expect(page.getByLabel(question)).toBeVisible();
      await page.reload();
      if (multiField) await openPendingInput(page);
      // Required-field validation must keep the real request pending.
      await page.getByRole('button', { name: 'Submit', exact: true }).click();
      await expect(page.getByText(`${question} is required`, { exact: true })).toBeVisible();
      await page.getByLabel(question).fill(answer);
      if (multiField) {
        await page.getByLabel('Decision', { exact: true }).click();
        await page.getByTitle('approve', { exact: true }).click();
      }
      const responded = page.waitForResponse(r => r.url().endsWith(`/user-inputs/${jobID}/respond`) && r.request().method() === 'POST');
      await page.getByRole('button', { name: 'Submit', exact: true }).click();
      const response = await responded;
      expect(response.status(), await response.text()).toBe(200);
      await expect(observer.getByText('No pending reviews')).toBeVisible();
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toEqual([]);

      startWorker();
      await expect.poll(async () => {
        const response = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
        return response.status() === 200 ? response.json() : { status: response.status() };
      }, { timeout: 30_000, message: 'The resumed recipe must return the exact browser answer' }).toMatchObject({
        status: 'completed',
        output: multiField ? { answer, decision: 'approve' } : { answer },
      });
      if (!multiField) await page.goto(`/project/${tenant}/jobs/${jobID}/story`);
      await page.getByRole('tab', { name: 'Conversation', exact: true }).click();
      await expect(page.getByRole('region', { name: 'Job completion', exact: true })).toContainText('Succeeded');
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/project/${tenant}/jobs`);
      await expect(page.getByRole('heading', { name: 'Goals', exact: true })).toBeVisible();
      await expect(page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) })).toBeVisible();
      await setStatusFilter(page, ['Ready']);
      await expect(page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) })).toHaveCount(0);
      await setStatusFilter(page, null);
      const jobs = await (await request.get(`/api/projects/${tenant}/jobs?status=all`)).json();
      expect(jobs.jobs.find((job: { job_id: string }) => job.job_id === jobID)).toMatchObject({ status: 'COMPLETED', completion_status: 'success' });
      const row = page.getByRole('row').filter({ has: page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) }) });
      await expect(row.getByText('Succeeded', { exact: true })).toBeVisible();
      await row.getByRole('button', { name: 'Expand goal details' }).click();
      await expect(page.getByText('All job data', { exact: true })).toBeVisible();

      if (!multiField) {
        await stopWorker();
        const storyResponse = page.waitForResponse(r => r.url().endsWith(`/jobs/${jobID}/story`));
        await page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) }).click();
        const story = await (await storyResponse).json();
        expect(story.restart_scopes.length).toBeGreaterThan(0);
        await page.setViewportSize({ width: 390, height: 844 });
        await page.getByRole('tab', { name: 'Execution', exact: true }).click();
        await page.getByRole('button', { name: 'Restart job', exact: true }).click();
        const restartDialog = page.getByRole('dialog');
        await expect(restartDialog.getByRole('button', { name: 'Restart', exact: true })).toBeEnabled();
        await restartDialog.getByRole('button', { name: 'Cancel', exact: true }).click();
        // Inspect a real task, then restart at that task's recorded boundary.
        const taskNodes = (node: any): any[] => [node, ...(node.children || []).flatMap(taskNodes)];
        const task = taskNodes(story.root).filter(node => ['op', 'opStep'].includes(node.kind) && node.restart_chapter_ordinal != null).at(-1);
        expect(task).toBeTruthy();
        const expectedChapter = task.restart_chapter_ordinal;
        const expectedScope = task.restart_scope_id;
        expect(expectedScope).toBeTruthy();
        expect(story.restart_scopes.some((scope: { chapter_ordinal: number }) => scope.chapter_ordinal === expectedChapter)).toBe(true);
        await page.goto(`/project/${tenant}/jobs/${jobID}/story?nodeId=${encodeURIComponent(task.id)}`);
        await page.getByRole('button', { name: 'Restart job', exact: true }).click();
        await expect(restartDialog.getByRole('button', { name: 'Restart', exact: true })).toBeEnabled();
        await expect(restartDialog.getByText(`· Chapter ${expectedChapter} ·`, { exact: false })).toBeVisible();
        const inspected = page.waitForResponse(r => r.url().endsWith(`/jobs/${jobID}/restart-context?scope_id=${encodeURIComponent(expectedScope)}`));
        await restartDialog.getByRole('switch').click();
        const contextResponse = await inspected;
        expect(contextResponse.status(), await contextResponse.text()).toBe(200);
        const before = await contextResponse.json();
        expect(before).toMatchObject({ project_id: tenant, job_id: jobID, chapter_ordinal: expectedChapter });
        expect(Object.keys(before.job).length).toBeGreaterThan(0);
        expect(before.visible).toHaveProperty('context');
        const beforeView = restartDialog.getByRole('region', { name: 'Context before patching' });
        await expect(beforeView).toBeVisible();
        const beforeText = await beforeView.innerText();
        await restartDialog.getByRole('button', { name: 'Add line', exact: true }).first().click();
        await restartDialog.getByPlaceholder('e.g. git.author').fill('browser_note');
        await restartDialog.getByPlaceholder('e.g. "james@example.com"').fill('patched');
        expect(await beforeView.innerText()).toBe(beforeText);
        // Invalid selectors and cross-tenant reads must never become snapshots.
        expect((await request.get(`/api/projects/other-${tenant}/jobs/${jobID}/restart-context?chapter_ordinal=${expectedChapter}`)).status()).toBe(404);
        expect((await request.get(`/api/projects/${tenant}/jobs/${jobID}/restart-context?chapter_ordinal=-1`)).status()).toBe(400);
        const submissionScope = story.restart_scopes.find((scope: { chapter_ordinal: number }) => scope.chapter_ordinal === 0);
        expect(submissionScope).toBeTruthy();
        const submissionContext = await request.get(`/api/projects/${tenant}/jobs/${jobID}/restart-context?chapter_ordinal=0`);
        expect(submissionContext.status(), await submissionContext.text()).toBe(200);
        expect(await submissionContext.json()).toMatchObject({ chapter_ordinal: 0, scope_id: submissionScope.scope_id });
        expect((await request.post(`/api/projects/${tenant}/jobs/${jobID}/restart`, { data: { chapter_ordinal: expectedChapter, scope_id: 'also-supplied' } })).status()).toBe(400);
        await expect(restartDialog.getByTitle('Start a new goal', { exact: true })).toBeVisible();
        for (const width of [320, 390, 768, 1440]) {
          await page.setViewportSize({ width, height: width < 768 ? 360 : 900 });
          await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
          await restartDialog.getByRole('button', { name: 'Restart', exact: true }).scrollIntoViewIfNeeded();
          await expect(restartDialog.getByRole('button', { name: 'Restart', exact: true })).toBeInViewport();
        }
        await page.setViewportSize({ width: 390, height: 844 });
        const restarted = page.waitForResponse(r => r.url().endsWith(`/jobs/${jobID}/restart`) && r.request().method() === 'POST');
        await restartDialog.getByRole('button', { name: 'Restart', exact: true }).click();
        const restartResponse = await restarted;
        expect(restartResponse.status(), await restartResponse.text()).toBe(201);
        expect(restartResponse.request().postDataJSON()).toEqual({ scope_id: expectedScope, request_id: expect.any(String), goal: { mode: 'new' }, patch: { job: { browser_note: 'patched' } } });
        const newJobID = (await restartResponse.json()).job_id;
        expect(newJobID).not.toBe(jobID);
        await expect(page).toHaveURL(new RegExp(`/jobs/${newJobID}/story(?:\\?executionState=cells-v1)?$`));
        startWorker(newJobID);
        await expect(observer.getByText(`Job ID: ${newJobID}`, { exact: true })).toBeVisible();
      }
    });
  });
}

test('cancellation removes a pending prompt in open tabs and rejects late answers', async ({ page, request, recipeRun }) => {
  const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
  await page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) }).click();
  await openPendingInput(page);
  await expect(page.getByLabel('What should the recipe publish?')).toBeVisible();
  await stopWorker();

  // Cancellation can originate outside this browser. Both tabs must react to SSE.
  const cancelled = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/cancel`, {
    data: { reason: 'Browser integration cancellation' },
  });
  expect(cancelled.status(), await cancelled.text()).toBe(200);
  await expect(observer.getByText('No pending reviews')).toBeVisible();
  await expect(page.getByLabel('What should the recipe publish?')).not.toBeVisible();
  expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toEqual([]);

  const lateAnswer = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/respond`, {
    data: { fields: { response: 'too late' }, response: 'too late' },
  });
  expect(lateAnswer.ok()).toBe(false);
  startWorker();
  await expect.poll(async () => (await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`)).json()).toMatchObject({ status: 'canceled' });
});

test.describe('sequential prompts', () => {
  test.use({ recipeFixture: 'browser-input-sequential' });
  test('announces each prompt in the same job and preserves both answers', async ({ page, request, recipeRun }) => {
    const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
    const answer = `release-${randomUUID()}`;
    const reviewer = `reviewer-${randomUUID()}`;
    await page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) }).click();

    for (const [question, value] of [
      ['What should the recipe publish?', answer],
      ['Who should review the release?', reviewer],
    ]) {
      await expect(observer.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible({ timeout: 30_000 });
      await stopWorker();
      await openPendingInput(page);
      await expect(page.getByLabel(question)).toHaveValue('');
      await page.getByLabel(question).fill(value);
      const responded = page.waitForResponse(r => r.url().endsWith(`/user-inputs/${jobID}/respond`) && r.request().method() === 'POST');
      await page.getByRole('button', { name: 'Submit', exact: true }).click();
      expect((await responded).status()).toBe(200);
      await expect(observer.getByText('No pending reviews')).toBeVisible();
      await expect(page.getByLabel(question)).not.toBeVisible();
      startWorker();
    }

    await expect.poll(async () => (await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`)).json(), {
      timeout: 30_000,
    }).toMatchObject({ status: 'completed', output: { answer, reviewer } });
    await page.getByRole('tab', { name: 'Conversation', exact: true }).click();
    await expect(page.getByRole('region', { name: 'Job completion', exact: true })).toContainText('Succeeded');
    await expect(observer.getByText('No pending reviews')).toBeVisible();
  });
});

test.describe('input timeout', () => {
  test.use({ recipeFixture: 'browser-input-timeout' });
  test('expires an unanswered input and clears open browser prompts', async ({ page, request, recipeRun }) => {
    const { tenant, jobID, observer } = recipeRun;
    await page.getByRole('link', { name: new RegExp(`^Job ${jobID}:`) }).click();
    await openPendingInput(page);
    await expect(page.getByLabel('What should the recipe publish?')).toBeVisible();

    // Let the real c2j/JobDB task deadline expire; do not advance a browser clock
    // or synthesize an SSE event. Neither tab may need a refresh.
    await expect.poll(async () => {
      const response = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
      return response.json();
    }, { timeout: 45_000 }).toMatchObject({ status: 'failed', error: expect.anything() });
    const outcome = await (await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`)).json();
    expect(JSON.stringify(outcome.error)).toMatch(/timeout|timed out|deadline/i);
    await expect(observer.getByText('No pending reviews')).toBeVisible();
    await expect(page.getByLabel('What should the recipe publish?')).not.toBeVisible();
    expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toEqual([]);
    const lateAnswer = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/respond`, {
      data: { fields: { response: 'too late' }, response: 'too late' },
    });
    expect(lateAnswer.ok()).toBe(false);
  });
});


for (const clarification of [false, true]) {
  test.describe(clarification ? 'clarification review' : 'document review', () => {
    test.use({ recipeFixture: clarification ? 'browser-review-clarification' : 'browser-review' });
    test('reviews render immutable Markdown and submit through c2j', async ({ page, request, recipeRun }, testInfo) => {
      const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
      expect(await page.evaluate(() => typeof globalThis.crypto.randomUUID)).toBe('undefined');
      await stopWorker();
      const job = await (await request.get(`/api/projects/${tenant}/jobs/${jobID}`)).json();
      expect(job.prompt).toBe(reviewGoalPrompt);
      await page.setViewportSize({ width: 390, height: 844 });
      await page.getByRole('button', { name: 'Open navigation' }).click();
      await page.getByRole('link', { name: 'Reviews', exact: true }).click();
      await expect(page.getByText(abbreviatePrompt(job.title || job.prompt), { exact: true })).toBeVisible();
      await expect(page.getByText(`Cell: ${job.cell_name}`, { exact: true })).toBeVisible();
      await page.getByRole('button', { name: 'Open Review', exact: true }).click();
      await page.getByText('Goal context', { exact: true }).click();
      await page.getByText('Review instructions', { exact: true }).click();
      const goalPrompt = page.getByRole('region', { name: 'Goal prompt', exact: true });
      await expect(goalPrompt).toContainText(job.prompt);
      await expect(page.getByText(job.cell_name, { exact: true })).toBeVisible();
      expect(await goalPrompt.evaluate(element => Boolean(element.compareDocumentPosition(
        document.querySelector('[aria-label="Review context"]')!
      ) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
      await expect(page.getByRole('heading', { name: 'Review the release' })).toBeVisible();
      await page.getByRole('button', { name: 'Read design.md', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await expect(page.getByLabel('Rendered document').getByRole('table')).toBeVisible();
      await expect(page.locator('[contenteditable=true]')).toHaveCount(0);
      await page.getByText('Markdown', { exact: true }).click();
      await expect(page.getByLabel('Markdown source')).toContainText('**Immutable original**');
      await page.getByTitle('design · design.md', { exact: true }).click();
      await page.getByTitle('rollout · rollout.md', { exact: true }).click();
      await expect(page.getByLabel('Markdown source')).toContainText('# Rollout');
      await page.getByText('Rendered', { exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Rollout', exact: true })).toBeVisible();
      await expect(page.getByRole('checkbox')).toBeDisabled();

      const details = await (await request.get(`/api/projects/${tenant}/user-inputs/${jobID}`)).json();
      const stale = await page.request.post(`/api/projects/${tenant}/reviews/${jobID}/respond`, {
        data: { request_id: 'old-request', submission_id: randomUUID(), fields: { decision: 'approve' } },
      });
      expect(stale.status()).toBe(409);
      expect(await stale.text()).toContain('request_id does not match the pending input');
      const invalid = await page.request.post(`/api/projects/${tenant}/reviews/${jobID}/respond`, {
        data: { request_id: details.form.request_id, submission_id: randomUUID(), fields: { decision: 'unknown' } },
      });
      expect(invalid.status()).toBe(400);
      expect((await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json())).toHaveLength(1);

      const prompt = page.getByRole('region', { name: 'Review context', exact: true });
      await expect(prompt).toHaveText(details.form.fields[0].question);
      expect(await prompt.evaluate(element => Boolean(element.compareDocumentPosition(
        Array.from(document.querySelectorAll('.ant-card-head-title')).find(node => node.textContent === 'Your response')!
      ) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
      await expect(page.getByRole('radio', { name: /Approve|Revise the plan|Request changes/ })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Submit', exact: true })).toHaveCount(0);
      const action = page.getByRole('button', { name: clarification ? 'Revise the plan' : 'Approve', exact: true });
      const feedback = page.getByLabel(clarification
        ? 'Feedback or answers to open questions. Select a revision decision to request changes.' : 'What should change?', { exact: true });
      await action.click();
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toHaveLength(1);
      const confirm = page.getByRole('button', { name: 'OK', exact: true });
      if (!clarification) {
        await confirm.click();
        await expect(page.getByText('What should change? is required')).toBeVisible();
      }
      await feedback.fill('Please use the proposed mandate and clarify the ownership boundary.');
      if (!clarification) await page.locator('input[type=file]').setInputFiles({ name: 'feedback.md', mimeType: 'text/markdown', buffer: Buffer.from('# Feedback from browser') });
      await page.getByRole('button', { name: 'Read design.md', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await expect(feedback).toHaveValue('Please use the proposed mandate and clarify the ownership boundary.');

      for (const viewport of [{ width: 320, height: 568 }, { width: 768, height: 1024 }, { width: 1440, height: 900 }, { width: 390, height: 844 }]) {
        await page.setViewportSize(viewport);
        await expect(feedback).toHaveValue('Please use the proposed mandate and clarify the ownership boundary.');
        await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      }
      await page.screenshot({ path: testInfo.outputPath('phone-review.png'), fullPage: true });

      // A rejected request must remain actionable, with the draft preserved.
      let releaseFailure!: () => void;
      const failureGate = new Promise<void>(resolve => { releaseFailure = resolve; });
      await page.route(`**/reviews/${jobID}/respond`, async route => {
        await failureGate;
        await route.fulfill({
          status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Review service temporarily unavailable' }),
        });
      }, { times: 1 });
      try {
        await confirm.click();
        // Keep the request in flight until we observe the loading state. The
        // spinner must not change the action's accessible name or allow duplicates.
        await expect(confirm).toHaveAttribute('aria-busy', 'true');
        await expect(confirm).toBeDisabled();
      } finally {
        releaseFailure();
      }
      await expect(page.getByRole('alert').filter({ hasText: 'Review service temporarily unavailable' })).toBeVisible();
      await expect(confirm).toHaveAttribute('aria-busy', 'false');
      await expect(confirm).toBeEnabled();
      await expect(feedback).toHaveValue('Please use the proposed mandate and clarify the ownership boundary.');
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toHaveLength(1);
      const [response] = await Promise.all([
        page.waitForResponse(r => r.url().endsWith(`/reviews/${jobID}/respond`) && r.request().method() === 'POST'),
        confirm.click(),
      ]);
      expect(response.status(), await response.text()).toBe(200);
      await expect(page.getByText('No pending reviews')).toBeVisible();
      await expect(observer.getByText('No pending reviews')).toBeVisible();
      startWorker();
      await expect.poll(async () => {
        const response = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
        return response.status() === 200 ? response.json() : null;
      }, { timeout: 30_000 }).toMatchObject({ status: 'completed', output: {
        answers: { decision: clarification ? 'revise' : 'approve',
          feedback: 'Please use the proposed mandate and clarify the ownership boundary.',
          ...(!clarification ? { annotation: { kind: 'stored', name: 'feedback.md' } } : {}),
        },
        receipt: { request_id: details.form.request_id, actor: { id: 'browser@example.com', kind: 'human' } },
      } });
      const original = details.form.documents.design.stored.key;
      const originalResponse = await request.get(`/api/projects/${tenant}/jobs/${original.jobId}/tasks/${original.taskOrdinal}/artifacts/${original.name}`);
      expect(await originalResponse.text()).toContain('**Immutable original**');
    });
  });
}

test.describe('review job metadata', () => {
  test.use({ recipeFixture: 'browser-review' });
  for (const cellName of ['self', 'platform']) {
    test(`renders successful cell metadata ${JSON.stringify(cellName)}`, async ({ page, recipeRun }) => {
      const { tenant, jobID, stopWorker } = recipeRun;
      await stopWorker();
      await page.route(`**/api/projects/${tenant}/jobs/${jobID}`, async route => {
        const response = await route.fetch();
        await route.fulfill({ response, json: { ...await response.json(), cell_name: cellName } });
      });
      await page.goto(`/project/${tenant}/reviews`);
      const label = cellName;
      await expect(page.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible();
      await page.getByRole('button', { name: 'Open Review' }).click();
      await expect(page.getByText(label, { exact: true })).toBeVisible();
      await expect(page.getByRole('region', { name: 'Goal prompt' })).toContainText(reviewGoalPrompt);
    });
  }

  test('opens a review while metadata requests are pending', async ({ page, recipeRun }) => {
    const { tenant, jobID, stopWorker } = recipeRun;
    await stopWorker();
    let release!: () => void;
    const gate = new Promise<void>(done => { release = done; });
    await page.route(`**/api/projects/${tenant}/jobs/${jobID}`, async route => {
      await gate;
      await route.continue();
    });
    try {
      await page.goto(`/project/${tenant}/reviews`);
      await expect(page.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible();
      await page.getByRole('button', { name: 'Open Review' }).click();
      await expect(page.getByText('Loading job context...', { exact: true })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Approve', exact: true })).toBeEnabled();
    } finally { release(); }
    await expect(page.getByRole('region', { name: 'Goal prompt' })).toContainText(reviewGoalPrompt);
  });

  test('preserves feedback and attachment through context retry and submission', async ({ page, request, recipeRun }) => {
    const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
    await stopWorker();
    let failMetadata = true;
    await page.route(`**/api/projects/${tenant}/jobs/${jobID}`, route => failMetadata
      ? route.fulfill({ status: 503, json: { error: 'Metadata temporarily unavailable' } }) : route.continue());
    await page.goto(`/project/${tenant}/reviews`);
    await expect(page.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Open Review' }).click();
    await expect(page.getByText('Job context unavailable', { exact: true })).toBeVisible();
    await expect(page.getByText('Cell unavailable', { exact: true })).toBeVisible();
    await expect(page.getByText('Unknown cell', { exact: true })).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Approve', exact: true }).click();
    const feedback = page.getByLabel('What should change?', { exact: true });
    await feedback.fill('Keep this feedback through context retry');
    await page.locator('input[type=file]').setInputFiles({ name: 'feedback.md', mimeType: 'text/markdown', buffer: Buffer.from('# Browser feedback') });
    failMetadata = false;
    await page.getByRole('button', { name: 'Retry job context' }).click();
    await expect(page.getByRole('region', { name: 'Goal prompt' })).toContainText(reviewGoalPrompt);
    await expect(feedback).toHaveValue('Keep this feedback through context retry');
    await expect(page.getByText('feedback.md', { exact: true })).toBeVisible();
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url().endsWith(`/reviews/${jobID}/respond`) && r.request().method() === 'POST'),
      page.getByRole('button', { name: 'OK', exact: true }).click(),
    ]);
    expect(response.status()).toBe(200);
    await expect(page.getByText('No pending reviews')).toBeVisible();
    await expect(observer.getByText('No pending reviews')).toBeVisible();
    startWorker();
    await expect.poll(async () => {
      const outcome = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
      return outcome.status() === 200 ? outcome.json() : null;
    }, { timeout: 30_000 }).toMatchObject({ status: 'completed', output: { answers: {
      decision: 'approve', feedback: 'Keep this feedback through context retry', annotation: { kind: 'stored', name: 'feedback.md' },
    } } });
  });

  test('wraps full literal prompts and metadata on a narrow viewport', async ({ page, recipeRun }) => {
    const { tenant, jobID, stopWorker } = recipeRun;
    await stopWorker();
    const prompt = '**Literal markup**\n<script>window.bad = true</script>\n' + 'long-goal-text'.repeat(100);
    await page.route(`**/api/projects/${tenant}/jobs/${jobID}`, async route => {
      const response = await route.fetch();
      await route.fulfill({ response, json: { ...await response.json(), prompt, cell_name: 'long-cell'.repeat(30), git_ref: 'long-ref'.repeat(30) } });
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/project/${tenant}/reviews`);
    const fits = () => page.evaluate(() => {
      const content = document.querySelector('.ant-layout-content')!;
      return document.documentElement.scrollWidth <= window.innerWidth && content.scrollWidth <= content.clientWidth;
    });
    await expect(page.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible();
    expect(await fits()).toBe(true);
    await page.getByRole('button', { name: 'Open Review' }).click();
    await page.getByText('Goal context', { exact: true }).click();
    const region = page.getByRole('region', { name: 'Goal prompt' });
    await expect(region).toContainText(prompt);
    await expect(region.locator('script, strong')).toHaveCount(0);
    await page.getByRole('button', { name: 'Read design.md', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Approve', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Back to Reviews' })).toBeVisible();
    expect(await fits()).toBe(true);
    await page.getByRole('button', { name: 'Back to Reviews' }).click();
    await expect(page.getByRole('button', { name: 'Open Review' })).toBeVisible();
  });
});


test.describe('multi-format review viewers', () => {
  test.use({ recipeFixture: 'browser-review-formats', nativeZoom: true });
  test.beforeEach(() => test.setTimeout(180_000));
  test('E01-E11 previews stored formats safely and preserves review responses', async ({ page, request, recipeRun }, testInfo) => {
    const {tenant,jobID,observer,stopWorker,startWorker}=recipeRun;
    await stopWorker();
    // The observer was the last opened tab; reviewer animations need a foreground tab.
    await page.bringToFront();
    const fetched: string[]=[]; const external: string[]=[];
    page.on('request',r=>{if(r.url().includes('/documents?'))fetched.push(r.url());if(r.url().includes('blocked.invalid'))external.push(r.url());});
    await page.goto(`/project/${tenant}/reviews`);
    await expect(page.getByRole('button',{name:'Open Review',exact:true})).toBeVisible();
    expect(fetched).toHaveLength(0);
    await setNativeZoom(page,2);
    await page.getByRole('button',{name:'Open Review',exact:true}).click();
    await expect(page.getByLabel('Document preview')).toBeVisible();
    await page.getByRole('button',{name:'Approve',exact:true}).click();
    const feedback=page.getByLabel('What should change?',{exact:true});
    await feedback.fill('Multi-format feedback');
    await page.locator('input[type=file]').setInputFiles({name:'feedback.md',mimeType:'text/markdown',buffer:Buffer.from('# Feedback')});
    const read=async(name:string)=>{
      await page.getByRole('button',{name:`Read ${name}`,exact:true}).click();
      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(page.getByRole('region',{name:'Selected document',exact:true})).toBeVisible();
    };
    const back=async()=>{ await page.getByRole('navigation', {name:'Review documents'}).scrollIntoViewIfNeeded(); };
    await read('data.json');await expect(page.getByLabel('Formatted JSON')).toContainText('9007199254740993');
    await page.getByText('Formatted JSON — collapse / expand',{exact:true}).click();await expect(page.getByLabel('Formatted JSON')).toBeHidden();
    await page.getByText('Formatted JSON — collapse / expand',{exact:true}).click();await back();
    await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await setNativeZoom(page,1);await page.setViewportSize({width:320,height:700});
    await read('settings.json');await expect(page.getByLabel('Formatted JSON')).toContainText('approved');await back();
    await read('external.txt');await expect(page.getByLabel('Document source')).toHaveText('Cross-job original\n');await back();
    await read('changes.patch');await expect(page.getByLabel('Unified diff')).toContainText('-old');
    await page.getByText('Side by side',{exact:true}).click();await expect(page.getByLabel('Side by side diff')).toContainText('+new');
    await page.screenshot({path:testInfo.outputPath('dual-pane.png')});await back();
    await page.route(/\/documents\?.*document_id=log(?:&|$)/,route=>route.fulfill({status:404,body:'missing document'}),{times:1});
    await read('checks.log');await expect(page.getByText(/Could not load this document/)).toBeVisible();await page.getByRole('button',{name:'Retry',exact:true}).click();await page.getByLabel('Search document').fill('needle');await expect(page.locator('mark')).toHaveText('needle');await back();
    await read('changes.patch');await expect(page.getByLabel('Side by side diff')).toBeVisible();await back();
    for(const name of ['mock.html','delayed.html','vector.svg']){
      await read(name);
      await expect(page.getByText(/Static preview:/)).toBeVisible();
      if(name.endsWith('.html')) {
        const iframe=page.getByTitle(`Static HTML preview: ${name}`);
        await expect(iframe).toHaveAttribute('sandbox','');
        await expect(iframe).toHaveAttribute('referrerpolicy','no-referrer');
        const frame=page.frameLocator(`iframe[title="Static HTML preview: ${name}"]`);
        await expect(frame.locator('body')).toBeAttached();
        await expect(frame.locator('script, iframe, meta[http-equiv="refresh"], a[href]')).toHaveCount(0);
        await expect(frame.locator('meta[http-equiv="Content-Security-Policy"]')).toHaveAttribute('content',/default-src 'none'/);
      } else {
        const image=page.getByRole('img',{name,exact:true});
        await expect(image).toBeVisible();
        await expect.poll(()=>image.evaluate((el:HTMLImageElement)=>el.complete&&el.naturalWidth>0)).toBe(true);
        await expect(page.locator('iframe')).toHaveCount(0);
      }
      // Observe delayed refresh/navigation while the rendered preview is active.
      await page.waitForTimeout(2100);expect(external).toHaveLength(0);
      await expect(page).toHaveURL(new RegExp(`/reviews/${jobID}$`));
      await page.getByRole('radio',{name:'Source',exact:true}).focus();await page.keyboard.press('Space');
      await expect(page.getByLabel('Document source')).toContainText('blocked.invalid');
      await expect(page.locator('iframe')).toHaveCount(0);await back();
    }
    await read('screenshot.png');
    const png=page.getByRole('img',{name:'screenshot.png',exact:true});
    await expect(png).toBeVisible();await expect.poll(()=>png.evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBe(1);
    await page.getByRole('button',{name:'Actual size',exact:true}).click();await page.getByRole('button',{name:'Zoom in',exact:true}).click();await back();
    for (const ext of ['jpg','gif','webp','avif','bmp']) {
      await read(`sample.${ext}`);
      const image = page.getByRole('img',{name:`sample.${ext}`,exact:true});
      await expect(image).toBeVisible();await expect.poll(()=>image.evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBe(2);await back();
    }
    await read('wide.png');
    await page.getByRole('button',{name:'Actual size',exact:true}).click();
    const imageRegion = page.getByRole('region',{name:'Image preview',exact:true});
    await imageRegion.focus();await expect(imageRegion).toBeFocused();await page.keyboard.press('ArrowRight');
    await expect.poll(()=>imageRegion.evaluate(el=>el.scrollLeft)).toBeGreaterThan(0);
    await imageRegion.evaluate(el=>{el.scrollLeft=0;});
    // The tall image can start above the viewport after keyboard focus. Target
    // its visible intersection, above the fixed review decisions, for touch.
    const touchPoint = await imageRegion.evaluate(el=>{
      const box=el.getBoundingClientRect();
      const actions=document.querySelector('.review-actions')?.getBoundingClientRect();
      const left=Math.max(0,box.left),right=Math.min(innerWidth,box.right);
      const top=Math.max(0,box.top),bottom=Math.min(innerHeight,box.bottom,actions?.top??innerHeight);
      const x=Math.round((left+right)/2),y=Math.round((top+bottom)/2);
      return {x,y,visible:right>left&&bottom>top,hit:el.contains(document.elementFromPoint(x,y))};
    });
    expect(touchPoint.visible).toBe(true);expect(touchPoint.hit).toBe(true);
    await testInfo.attach('image-touch-target',{body:JSON.stringify(touchPoint),contentType:'application/json'});
    await page.bringToFront();
    const touchSession = await page.context().newCDPSession(page);
    await touchSession.send('Emulation.setTouchEmulationEnabled',{enabled:true,maxTouchPoints:1});
    await touchSession.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:touchPoint.x,y:touchPoint.y}]});
    for(let step=1;step<=10;step++) {
      await touchSession.send('Input.dispatchTouchEvent',{type:'touchMove',touchPoints:[{x:touchPoint.x-step*10,y:touchPoint.y}]});
      // Deliver a swipe over compositor frames so Chromium performs native panning.
      await page.waitForTimeout(20);
    }
    await touchSession.send('Input.dispatchTouchEvent',{type:'touchEnd',touchPoints:[]});
    await touchSession.detach();await expect.poll(()=>imageRegion.evaluate(el=>el.scrollLeft)).toBeGreaterThan(0);await back();
    await read('pictures.md');await expect(page.getByRole('img',{name:'screenshot.png',exact:true})).toBeVisible();
    await expect(page.getByText(/Blocked external or ambiguous image/)).toBeVisible();expect(external).toHaveLength(0);await back();
    await read('evidence.tar.gz');await expect(page.getByText(/Container data/)).toBeVisible();await back();
    expect(fetched.every(u=>!u.includes('document_id=rollout'))).toBe(true);
    await expect(feedback).toHaveValue('Multi-format feedback');
    const details=await(await request.get(`/api/projects/${tenant}/user-inputs/${jobID}`)).json();
    expect(details.form.documents.external.stored.key.jobId).not.toBe(jobID);
    for(const id of ['json','patch','png','archive','html','delayed','svg']){
      const url=`/api/projects/${tenant}/reviews/${jobID}/documents?request_id=${encodeURIComponent(details.form.request_id)}&document_id=${id}`;
      const downloaded=await request.get(url);expect(downloaded.status()).toBe(200);
      const key=details.form.documents[id].stored.key;
      const original=await request.get(`/api/projects/${tenant}/jobs/${key.jobId}/tasks/${key.taskOrdinal}/artifacts/${encodeURIComponent(key.name)}`);
      expect(await downloaded.body()).toEqual(await original.body());
    }
    await page.getByRole('button',{name:'OK',exact:true}).click();await expect(observer.getByText('No pending reviews')).toBeVisible();
    startWorker();await expect.poll(async()=>{
      const response=await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);return response.status()===200?response.json():null;
    },{timeout:30000}).toMatchObject({status:'completed',output:{answers:{feedback:'Multi-format feedback',annotation:{kind:'stored'}},receipt:{actor:{id:'browser@example.com',kind:'human'}}}});
  });
});

test.describe('conventional rejection',()=>{
  test.use({recipeFixture:'browser-review'});
  test('REV-007 submits rejection feedback with the original revise value through c2j',async({page,request,recipeRun})=>{
    const {tenant,jobID,stopWorker,startWorker}=recipeRun;
    await stopWorker();
    await page.goto(`/project/${tenant}/reviews/${jobID}`);
    await page.getByRole('button',{name:'Reject',exact:true}).click();
    const reason=page.getByLabel('What should change?',{exact:true});
    await expect(reason).toBeFocused();
    await reason.fill('   ');
    await page.getByRole('button',{name:'OK'}).click();
    await expect(page.getByText('What should change? is required')).toBeVisible();
    await reason.fill('Clarify ownership before approval.');
    const [response]=await Promise.all([
      page.waitForResponse(r=>r.url().endsWith(`/reviews/${jobID}/respond`) && r.request().method()==='POST'),
      page.getByRole('button',{name:'OK'}).click(),
    ]);
    expect(response.status(),await response.text()).toBe(200);
    startWorker();
    await expect.poll(async()=>{
      const r=await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
      return r.status()===200?r.json():null;
    },{timeout:30000}).toMatchObject({status:'completed',output:{answers:{decision:'revise',feedback:'Clarify ownership before approval.'}}});
  });
});

test.describe('nested review gallery', () => {
  test.use({recipeFixture:'browser-review'});
  test('RGI-013 decodes six attached siblings and never requests blocked images', async ({page,recipeRun},testInfo) => {
    const {tenant,jobID,stopWorker}=recipeRun;
    await stopWorker();
    const names=['review-collapsed-desktop.png','review-confirm-desktop.png','review-confirm-320.png','review-keyboard-geometry-320.png','review-200.png','review-error-320.png'];
    const png=Buffer.from(await page.evaluate(() => {
      const canvas=document.createElement('canvas');canvas.width=240;canvas.height=80;
      const ctx=canvas.getContext('2d')!;ctx.fillStyle='#dbeafe';ctx.fillRect(0,0,240,80);
      ctx.fillStyle='#17345c';ctx.font='18px sans-serif';ctx.fillText('Attached raster fixture',16,46);
      return canvas.toDataURL('image/png').split(',')[1];
    }),'base64');
    const blocked=['https://blocked.invalid/x.png','//blocked.invalid/y.png','/absolute.png','data:image/png;base64,AA','javascript:alert','duplicate.png','missing.png','../../escape.png','bad%FF','a%2fb'];
    const source='# Attached nested gallery\n\n'+names.map((name,i)=>`![capture${i}](${name})`).join('\n\n')+'\n\n'+blocked.map((ref,i)=>`![blocked${i}](${ref})`).join('\n\n');
    const doc=(name:string,display?:string)=>({kind:'stored',name:display,stored:{key:{jobId:'fixture-producer',taskOrdinal:1,name}}});
    await page.route(`**/api/projects/${tenant}/user-inputs/${jobID}`,async route=>{
      const response=await route.fetch();const details=await response.json();
      const documents:Record<string,unknown>={gallery:doc('captures/gallery.md','Gallery title.md'),duplicate1:doc('captures/duplicate.png'),duplicate2:doc('captures/duplicate.png')};
      names.forEach((name,i)=>{documents[`image/${i}`]=doc(`captures/${name}`);});
      await route.fulfill({response,json:{...details,form:{...details.form,documents,overview_document_id:'gallery'}}});
    });
    const fetched:string[]=[],external:string[]=[];
    page.on('request',request=>{if(request.url().includes('blocked.invalid')) external.push(request.url());});
    await page.route(`**/api/projects/${tenant}/reviews/${jobID}/documents?*`,async route=>{
      const id=new URL(route.request().url()).searchParams.get('document_id')!;fetched.push(id);
      await route.fulfill({contentType:id==='gallery'?'text/markdown':'image/png',body:id==='gallery'?source:png});
    });
    await page.setViewportSize({width:1440,height:900});
    await page.goto(`/project/${tenant}/reviews/${jobID}`);
    const images=page.locator('.review-markdown img');
    await expect(images).toHaveCount(6);
    await expect.poll(()=>images.evaluateAll(elements=>elements.every(el=>{const img=el as HTMLImageElement;return img.complete&&img.naturalWidth>0&&img.naturalHeight>0;}))).toBe(true);
    await expect(page.getByText(/Blocked external or ambiguous image/)).toHaveCount(blocked.length);
    expect(fetched.sort()).toEqual(['gallery',...names.map((_,i)=>`image/${i}`)].sort());
    expect(external).toEqual([]);
    const evidence=JSON.stringify({fetched,external,decodedImages:6,blockedReferences:blocked},null,2);
    writeFileSync(testInfo.outputPath('gallery-requests.json'),evidence);
    await testInfo.attach('gallery-requests',{body:evidence,contentType:'application/json'});
    // Capture without Playwright's full-page viewport changes, which can remount responsive previews.
    const session=await page.context().newCDPSession(page);
    const capture=async(name:string)=>{
      await expect.poll(()=>images.evaluateAll(elements=>elements.length===6&&elements.every(el=>{const img=el as HTMLImageElement;return img.complete&&img.naturalWidth>0;}))).toBe(true);
      await page.locator('.review-markdown').scrollIntoViewIfNeeded();
      await expect(page.getByText('Loading attached image…',{exact:true})).toHaveCount(0);
      await expect.poll(()=>images.evaluateAll(elements=>elements.length===6&&elements.every(el=>{const img=el as HTMLImageElement;return img.complete&&img.naturalWidth>0;}))).toBe(true);
      await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
      const {cssContentSize}=await session.send('Page.getLayoutMetrics');
      const {data}=await session.send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:cssContentSize.width,height:cssContentSize.height,scale:1}});
      writeFileSync(testInfo.outputPath(name),Buffer.from(data,'base64'));
    };

    await page.setViewportSize({width:390,height:844});
    await expect(images).toHaveCount(6);
    await capture('gallery-phone.png');
    await page.setViewportSize({width:1440,height:900});
    await expect(page.locator('.review-document-layout')).toBeVisible();
    await page.locator('.review-markdown').evaluate(el=>el.scrollIntoView());
    await expect(page.getByText('Loading attached image…',{exact:true})).toHaveCount(0);
    await expect.poll(()=>images.evaluateAll(elements=>elements.length===6&&elements.every(el=>(el as HTMLImageElement).naturalWidth>0))).toBe(true);
    await page.screenshot({path:testInfo.outputPath('gallery-desktop.png')});
    await session.detach();
  });
});
