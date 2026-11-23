import { parseJobJSON, stringifyJobJSON } from '../../shared/src/jobJSON';
import { registerCell } from './helpers/cells';
import { fixtureGitTransport } from './helpers/gitTransport';
import { abbreviatePrompt } from '../src/utils/goalPresentation';
import { expect, type Page } from '@playwright/test';
import { test as base, setNativeZoom } from './helpers/nativeZoom';
import { spawn, execFileSync, type ChildProcess } from 'node:child_process';
import { mkdtempSync, mkdirSync, copyFileSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';

async function openPendingInput(page: Page) {
  await page.getByRole('tab', { name: 'Conversation', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Submit', exact: true })).toBeVisible();
}

const longReviewFeedback = 'Please clarify the ownership boundary before approval.\n'.repeat(38)
  + 'Café — 日本語 🙂\nUnique feedback ending: FL-08';

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
    const initialWorkerPath = testInfo.project.name === 'previous-worker'
      ? resolve('../../build/c2j-previous-worker') : workerPath;
    let worker: ChildProcess | undefined;
    let workerTimer: ReturnType<typeof setTimeout> | undefined;
    let workerStopped = true;
    let workerLog = '';
    let fixtureFailed = false;
    const errors: string[] = [];
    context.on('page', browserPage => browserPage.on('pageerror', error => errors.push(error.message)));
    page.on('pageerror', error => errors.push(error.message));

    const workerEnv = fixtureGitTransport(repo);
    const startWorker = (targetJobID = jobID, executable = workerPath) => {
      workerStopped = false;
      const launch = () => {
        worker = spawn(executable, [
          'run', 'one', '--job-id', targetJobID, '--input-mode', 'ops',
          '--poll-interval', '100ms', '--jobdb', `http://127.0.0.1:18082/${tenant}`,
        ], {
          cwd: repo,
          // Runtime subprocess cleanup must not signal the Playwright runner's group.
          detached: true,
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
      await expect(page.getByRole('heading', { name: 'All', exact: true })).toBeVisible();

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
      startWorker(jobID, initialWorkerPath);

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

      await page.getByRole('button', { name: 'Refresh inbox', exact: true }).click();
      await expect(page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`)).toBeVisible();
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
    test('answers recipe input and resumes the worker to completion', async ({ page, request, recipeRun }, testInfo) => {
      const { tenant, jobID, observer, startWorker, stopWorker } = recipeRun;
      test.setTimeout(180_000);
      const answer = `approved-${randomUUID()}`;
      // Kill the worker while the request is pending: the answer must survive
      // in JobDB and be picked up by a fresh worker, not an in-process promise.
      await stopWorker();
      if (!multiField) await page.setViewportSize({ width: 390, height: 844 });
      await page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`).click();
      await openPendingInput(page);
      const question = multiField ? 'Release note' : 'What should the recipe publish?';
      await expect(page.getByLabel(question)).toBeVisible();
      await page.screenshot({ path: testInfo.outputPath('ordinary-form-inline.png'), fullPage: true });
      await page.reload();
      await openPendingInput(page);
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
      await expect(page.getByRole('heading', { name: 'All', exact: true })).toBeVisible();
      await expect(page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`)).toBeVisible();
      const jobs = await (await request.get(`/api/projects/${tenant}/jobs?status=all`)).json();
      expect(jobs.jobs.find((job: { job_id: string }) => job.job_id === jobID)).toMatchObject({ status: 'COMPLETED', completion_status: 'success' });
      const row = page.locator('.inbox-job-row').filter({has:page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`)});
      await expect(row.getByText('Succeeded', { exact: true })).toBeVisible();

      if (!multiField) {
        await stopWorker();
        const storyResponse = page.waitForResponse(r => r.url().endsWith(`/jobs/${jobID}/story`));
        await page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`).click();
        await expect(page.getByRole('tab', { name: 'Conversation', exact: true })).toHaveAttribute('aria-selected', 'true');
        await page.getByRole('tab', { name: 'Trace', exact: true }).click();
        const story = await (await storyResponse).json();
        expect(story.restart_scopes.length).toBeGreaterThan(0);
        await page.setViewportSize({ width: 390, height: 844 });
        await page.getByRole('tab', { name: 'Trace', exact: true }).click();
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
        await restartDialog.getByPlaceholder('e.g. git.author').fill('git.author');
        await restartDialog.getByPlaceholder('e.g. "james@example.com"').fill('Patched Author <patched@example.com>');
        expect(await beforeView.innerText()).toBe(beforeText);
        await restartDialog.getByRole('button', { name: 'Add line', exact: true }).last().click();
        await restartDialog.getByPlaceholder('e.g. build').fill('seed');
        await restartDialog.getByPlaceholder('e.g. image_tag').fill('nested');
        await restartDialog.getByPlaceholder('e.g. "v2"').fill('{"value":"edited","remove":null}');
        const preview = JSON.parse((await restartDialog.getByLabel('Patch payload', { exact: true }).textContent())!);
        expect(preview).toEqual([
          { op: Object.hasOwn(before.job.git, 'author') ? 'replace' : 'add', path: '/context/git/author', value: 'Patched Author <patched@example.com>' },
          { op: 'remove', path: '/sequence/seed/outputs/nested/remove' },
          { op: 'replace', path: '/sequence/seed/outputs/nested/value', value: 'edited' },
        ]);

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
          if (width === 1440) await page.screenshot({ path: testInfo.outputPath('restart-desktop.png'), fullPage: true });
        }
        await page.setViewportSize({ width: 390, height: 844 });
        await page.screenshot({ path: testInfo.outputPath('restart-phone.png'), fullPage: true });
        const restarted = page.waitForResponse(r => r.url().endsWith(`/jobs/${jobID}/restart`) && r.request().method() === 'POST');
        await restartDialog.getByRole('button', { name: 'Restart', exact: true }).click();
        const restartResponse = await restarted;
        expect(restartResponse.status(), await restartResponse.text()).toBe(201);
        expect(restartResponse.request().postDataJSON()).toEqual({ scope_id: expectedScope, request_id: expect.any(String), goal: { mode: 'new' }, patch: preview });
        const newJobID = (await restartResponse.json()).job_id;
        expect(newJobID).not.toBe(jobID);
        await expect(page).toHaveURL(new RegExp(`/jobs/${newJobID}/story(?:\\?executionState=cells-v1)?$`));
        startWorker(newJobID);
        await expect(observer.getByText(`Job ID: ${newJobID}`, { exact: true })).toBeVisible();
        const accepted = await request.post(`/api/projects/${tenant}/user-inputs/${newJobID}/respond`, { data: { fields: { response: 'restarted answer' }, response: 'restarted answer' } });
        expect(accepted.status(), await accepted.text()).toBe(200);
        await expect.poll(async () => {
          const outcome = await request.get(`/api/projects/${tenant}/jobs/${newJobID}/outcome`);
          return outcome.status() === 200 ? outcome.json() : {};
        }, { timeout: 30000 }).toMatchObject({ status: 'completed', output: { answer: 'restarted answer', seed: { nested: { keep: 'untouched', value: 'edited' } } } });
        const completedRestart = await (await request.get(`/api/projects/${tenant}/jobs/${newJobID}/outcome`)).json();
        expect(completedRestart.output.seed.nested).not.toHaveProperty('remove');
        expect(completedRestart.output.seed.array).toEqual(['a', 'b', 'c']);
        await stopWorker();
        const restartAPI = async (patch: unknown[]) => {
          const response = await request.post(`/api/projects/${tenant}/jobs/${jobID}/restart`, { data: { scope_id: expectedScope, request_id: randomUUID(), goal: { mode: 'new' }, patch } });
          expect(response.status(), await response.text()).toBe(201);
          return (await response.json()).job_id as string;
        };
        const apiJob = await restartAPI([
          { op: 'test', path: '/sequence/seed/outputs/a~1b~0c', value: 'escaped' },
          { op: 'replace', path: '/sequence/seed/outputs/literal.key', value: 'changed dotted' },
          { op: 'add', path: '/sequence/seed/outputs/array/1', value: 'inserted' },
          { op: 'copy', from: '/sequence/seed/outputs/array/0', path: '/sequence/seed/outputs/copied' },
          { op: 'move', from: '/sequence/seed/outputs/array/0', path: '/sequence/seed/outputs/array/2' },
          { op: 'remove', path: '/sequence/seed/outputs/nested/remove' },
          { op: 'add', path: '/sequence/seed/outputs/nullValue', value: null },
          { op: 'replace', path: '/sequence/seed/outputs/object', value: { only: true } },
          { op: 'add', path: '/sequence/seed/outputs/n', value: 123 },
          { op: 'test', path: '/sequence/seed/outputs/n', value: 123 },
        ]);
        startWorker(apiJob);
        await expect.poll(async () => (await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).some((input: { id: string }) => input.id === apiJob)).toBe(true);
        expect((await request.post(`/api/projects/${tenant}/user-inputs/${apiJob}/respond`, { data: { fields: { response: 'api answer' }, response: 'api answer' } })).status()).toBe(200);
        let apiOutput: any;
        await expect.poll(async () => {
          const response = await request.get(`/api/projects/${tenant}/jobs/${apiJob}/outcome`);
          if (response.status() !== 200) return false;
          const outcome = await response.json(); apiOutput = outcome.output; return outcome.status === 'completed';
        }).toBe(true);
        expect(apiOutput.seed).toEqual({ nested: { keep: 'untouched', value: 'old' }, array: ['inserted', 'b', 'a', 'c'], object: { only: true }, 'a/b~c': 'escaped', 'literal.key': 'changed dotted', copied: 'a', nullValue: null, n: 123 });
        await stopWorker();

        // Exact persisted values must reach inspection before the browser diffs
        // them. Never obtain these tokens through APIResponse.json().
        const exactValues = parseJobJSON('{"n":9007199254740993,"untouched":184467440737095516170,"decimal":1.23000000000000000001,"array":[9007199254740993,9007199254740995]}');
        const exactResponse = await request.post(`/api/projects/${tenant}/jobs/${jobID}/restart`, {
          headers: { 'Content-Type': 'application/json' },
          data: stringifyJobJSON({ scope_id: expectedScope, request_id: randomUUID(), goal: { mode: 'new' }, patch: [{ op: 'add', path: '/sequence/seed/outputs/exact', value: exactValues }] }),
        });
        expect(exactResponse.status(), await exactResponse.text()).toBe(201);
        const exactJob = (await exactResponse.json()).job_id;
        startWorker(exactJob);
        await expect.poll(async () => (await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).some((input: { id: string }) => input.id === exactJob)).toBe(true);
        expect((await request.post(`/api/projects/${tenant}/user-inputs/${exactJob}/respond`, { data: { fields: { response: 'exact answer' }, response: 'exact answer' } })).status()).toBe(200);
        await expect.poll(async () => (await (await request.get(`/api/projects/${tenant}/jobs/${exactJob}`)).json()).completion_status).toBe('success');
        await stopWorker();
        const exactStory = await (await request.get(`/api/projects/${tenant}/jobs/${exactJob}/story`)).json();
        const capture = taskNodes(exactStory.root).find(node => node.kind === 'sequence' && node.title === 'capture');
        expect(capture?.restart_scope_id).toBeTruthy();
        const exactInspection = await request.get(`/api/projects/${tenant}/jobs/${exactJob}/restart-context?scope_id=${encodeURIComponent(capture.restart_scope_id)}`);
        expect(exactInspection.status(), await exactInspection.text()).toBe(200);
        const exactSnapshot = parseJobJSON(await exactInspection.text()) as any;
        expect(stringifyJobJSON(exactSnapshot.visible.sequence.seed.outputs.exact)).toBe('{"array":[9007199254740993,9007199254740995],"decimal":1.23000000000000000001,"n":9007199254740993,"untouched":184467440737095516170}');
        await page.goto(`/project/${tenant}/jobs/${exactJob}/story?nodeId=${encodeURIComponent(capture.id)}`);
        await page.getByRole('button', { name: 'Restart job', exact: true }).click();
        await restartDialog.getByRole('switch').click();
        await expect(restartDialog.getByRole('region', { name: 'Context before patching' })).toContainText('184467440737095516170');
        const exactRows = [
          ['exact.n', '9007199254740999'],
          ['exact.array', '[9007199254740993,9007199254740995]'],
          ['nested.value', 'exact edit'],
        ];
        for (const [path, value] of exactRows) {
          await restartDialog.getByRole('button', { name: 'Add line', exact: true }).last().click();
          await restartDialog.getByPlaceholder('e.g. build').last().fill('seed');
          await restartDialog.getByPlaceholder('e.g. image_tag').last().fill(path);
          await restartDialog.getByPlaceholder('e.g. "v2"').last().fill(value);
        }
        await page.screenshot({ path: testInfo.outputPath('restart-exact.png'), fullPage: true });
        const exactPreview = parseJobJSON((await restartDialog.getByLabel('Patch payload', { exact: true }).textContent())!);
        expect(stringifyJobJSON(exactPreview)).toBe('[{"op":"replace","path":"/sequence/seed/outputs/exact/n","value":9007199254740999},{"op":"replace","path":"/sequence/seed/outputs/nested/value","value":"exact edit"}]');
        const exactRestart = page.waitForResponse(response => response.url().endsWith(`/jobs/${exactJob}/restart`) && response.request().method() === 'POST');
        await restartDialog.getByRole('button', { name: 'Restart', exact: true }).click();
        const exactAccepted = await exactRestart;
        expect(exactAccepted.status(), await exactAccepted.text()).toBe(201);
        const exactRequest = parseJobJSON(exactAccepted.request().postData()!) as any;
        expect(stringifyJobJSON(exactRequest.patch)).toBe(stringifyJobJSON(exactPreview));
        expect(exactRequest.scope_id).toBe(capture.restart_scope_id);
        const editedExactJob = (await exactAccepted.json()).job_id;
        startWorker(editedExactJob);
        await expect.poll(async () => (await (await request.get(`/api/projects/${tenant}/jobs/${editedExactJob}`)).json()).completion_status).toBe('success');
        // Inspect the worker's captured output at the following boundary. The
        // general outcome projection currently decodes generic numbers as float64.
        const editedExactStory = await (await request.get(`/api/projects/${tenant}/jobs/${editedExactJob}/story`)).json();
        const verified = taskNodes(editedExactStory.root).find(node => node.kind === 'sequence' && node.title === 'verified');
        expect(verified?.restart_scope_id).toBeTruthy();
        const verifiedContext = await request.get(`/api/projects/${tenant}/jobs/${editedExactJob}/restart-context?scope_id=${encodeURIComponent(verified.restart_scope_id)}`);
        expect(verifiedContext.status(), await verifiedContext.text()).toBe(200);
        const exactOutput = (parseJobJSON(await verifiedContext.text()) as any).visible.sequence.capture.outputs;
        expect(stringifyJobJSON(exactOutput.seed.exact)).toBe('{"array":[9007199254740993,9007199254740995],"decimal":1.23000000000000000001,"n":9007199254740999,"untouched":184467440737095516170}');
        expect(exactOutput.seed.nested).toEqual({ keep: 'untouched', value: 'exact edit', remove: 'gone' });
        await stopWorker();
        for (const failure of [
          { op: 'replace', path: '/sequence/seed/outputs/missing', value: 1 },
          { op: 'add', path: '/sequence/seed/outputs/missing/child', value: 1 },
          { op: 'copy', from: '/sequence/seed/outputs/missing', path: '/sequence/seed/outputs/copy' },
          { op: 'add', path: '/sequence/invisible/outputs/value', value: 1 },
          { op: 'add', path: '/states/invisible/outputs/value', value: 1 },
          { op: 'test', path: '/sequence/seed/outputs/nested/value', value: 'wrong' },
        ]) {
          const failedJob = await restartAPI([{ op: 'replace', path: '/sequence/seed/outputs/nested/value', value: 'must not leak' }, failure]);
          startWorker(failedJob);
          await expect.poll(async () => (await (await request.get(`/api/projects/${tenant}/jobs/${failedJob}`)).json()).completion_status).toBe('failed_app');
          const failedStory = await (await request.get(`/api/projects/${tenant}/jobs/${failedJob}/story`)).json();
          expect(JSON.stringify(failedStory)).not.toContain('"value":"must not leak"');
          await stopWorker();
          if (failure.op === 'test') {
            await page.goto(`/project/${tenant}/jobs/${failedJob}/story?tab=execution`);
            await expect(page.getByRole('dialog')).toHaveCount(0);
            await page.getByRole('tab', { name: 'Conversation', exact: true }).click();
            await expect(page.getByRole('region', { name: 'Job completion', exact: true })).toContainText('test failed');
            await page.screenshot({ path: testInfo.outputPath('restart-worker-failure.png'), fullPage: true });
          }
        }


      }
    });
  });
}

test('cancellation removes a pending prompt in open tabs and rejects late answers', async ({ page, request, recipeRun }, testInfo) => {
  const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
  await page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`).click();
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
  await page.screenshot({ path: testInfo.outputPath('ordinary-prompt-cancelled.png'), fullPage: true });
  expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toEqual([]);

  const lateAnswer = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/respond`, {
    data: { fields: { response: 'too late' }, response: 'too late' },
  });
  expect(lateAnswer.ok()).toBe(false);
  startWorker();
  await expect.poll(async () => (await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`)).json()).toMatchObject({ status: 'canceled' });
});

test('returns a real quiet 204 when a discovered input finishes before details load', async ({ page, request, recipeRun }, testInfo) => {
  const { tenant, jobID, stopWorker } = recipeRun;
  await stopWorker();
  const reads: number[] = [];
  page.on('response', response => {
    if (response.url().endsWith(`/user-inputs/${jobID}`)) reads.push(response.status());
  });
  // The browser has already matched a real pending snapshot when it requests
  // details. Finish that request before forwarding the read to the real server.
  await page.route(`**/api/projects/${tenant}/user-inputs/${jobID}`, async route => {
    const cancelled = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/cancel`, {
      data: { reason: 'Complete between discovery and details' },
    });
    expect(cancelled.status(), await cancelled.text()).toBe(200);
    const response = await route.fetch();
    expect(response.status()).toBe(204);
    expect(await response.body()).toHaveLength(0);
    expect(response.headers()['cache-control']).toBe('no-store');
    await testInfo.attach('real-input-race.txt', {
      body: `tenant=${tenant}\njob=${jobID}\nGET details: 204, empty body, Cache-Control: no-store\n`,
      contentType: 'text/plain',
    });
    await route.fulfill({ response });
  });
  await page.goto(`/project/${tenant}/reviews/${jobID}`);
  await expect(page.getByText('No outstanding input request.', { exact: true })).toBeVisible();
  await expect.poll(() => reads).toEqual([204]);
  await page.waitForTimeout(10_500); // More than two existing history refresh periods.
  expect(reads).toEqual([204]);
});

test.describe('sequential prompts', () => {
  test.use({ recipeFixture: 'browser-input-sequential' });
  test('announces each prompt in the same job and preserves both answers', async ({ page, request, recipeRun }, testInfo) => {
    const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
    const answer = `release-${randomUUID()}`;
    const reviewer = `reviewer-${randomUUID()}`;
    const jobLink = page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`);
    await jobLink.click();

    for (const [question, value] of [
      ['What should the recipe publish?', answer],
      ['Who should review the release?', reviewer],
    ]) {
      await expect(observer.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible({ timeout: 30_000 });
      await stopWorker();
      await openPendingInput(page);
      const field = page.getByLabel(question);
      await expect(field).toHaveValue('');
      if (value === reviewer) {
        await field.scrollIntoViewIfNeeded();
        await page.screenshot({ path: testInfo.outputPath('next-request-blank.png'), animations: 'disabled' });
      }
      await field.fill(value);
      if (value === answer) {
        await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
        await jobLink.click();
        await openPendingInput(page);
        await expect(field).toHaveValue(value);
        await field.scrollIntoViewIfNeeded();
        await page.screenshot({ path: testInfo.outputPath('same-request-draft.png'), animations: 'disabled' });
      }
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
    await page.locator(`.inbox-job-link[href*="/jobs/${jobID}/story"]`).click();
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
      const details = await (await request.get(`/api/projects/${tenant}/user-inputs/${jobID}`)).json();
      await page.bringToFront();
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(`/project/${tenant}/jobs/${jobID}/story?filter=mine`);
      // Story opens the complete native review inline, without a second link.
      const inline = page.locator('.embedded-review');
      await expect(inline.getByRole('heading', { name: 'Review the release' })).toBeVisible();
      await expect(inline.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await page.screenshot({ path: testInfo.outputPath('review-story-desktop.png'), fullPage: true });
      await page.setViewportSize({ width: 390, height: 844 });
      await expect(inline.getByRole('button', { name: clarification ? 'Revise the plan' : 'Approve', exact: true })).toBeVisible();
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await page.screenshot({ path: testInfo.outputPath('review-story-phone.png'), fullPage: true });
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toHaveLength(1);
      // Retain coverage of bookmarked direct review destinations and their context.
      await page.goto(`/project/${tenant}/reviews/${jobID}?request_id=${encodeURIComponent(details.form.request_id)}&document_id=design`);
      await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await page.screenshot({ path: testInfo.outputPath('direct-review-destination.png'), fullPage: true });
      await page.getByText('Goal context', { exact: true }).click();
      await page.getByText('Review instructions', { exact: true }).click();
      const goalPrompt = page.getByRole('region', { name: 'Goal prompt', exact: true });
      await expect(goalPrompt).toContainText(job.prompt);
      await expect(page.getByText(job.cell_name, { exact: true })).toBeVisible();
      expect(await goalPrompt.evaluate(element => Boolean(element.compareDocumentPosition(
        document.querySelector('[aria-label="Review context"]')!
      ) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
      await expect(page.getByRole('heading', { name: 'Review the release' })).toBeVisible();
      await page.getByRole('button', { name: 'Show documents', exact: true }).click();
      await page.getByRole('button', { name: 'Read design.md', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await expect(page.getByLabel('Rendered document').getByRole('table')).toBeVisible();
      // Compose remains mounted outside the review so its draft can survive navigation.
      // Immutability applies to the document reader, including its source view.
      const selectedDocument = page.getByLabel('Selected document', { exact: true });
      await expect(selectedDocument.locator('[contenteditable=true]')).toHaveCount(0);
      await page.getByRole('button', { name: 'Document options' }).click();
      await page.getByText('Markdown', { exact: true }).click();
      await page.keyboard.press('Escape');
      await expect(page.getByLabel('Markdown source')).toContainText('**Immutable original**');
      await expect(selectedDocument.locator('[contenteditable=true]')).toHaveCount(0);
      await page.getByTitle('design · design.md', { exact: true }).click();
      await page.getByTitle('rollout · rollout.md', { exact: true }).click();
      await expect(page.getByLabel('Markdown source')).toContainText('# Rollout');
      await page.getByRole('button', { name: 'Document options' }).click();
      await page.getByText('Rendered', { exact: true }).click();
      await page.keyboard.press('Escape');
      await expect(page.getByRole('heading', { name: 'Rollout', exact: true })).toBeVisible();
      await expect(page.getByRole('checkbox')).toBeDisabled();

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
      await feedback.fill(longReviewFeedback);
      await feedback.press('End');
      await feedback.press('Enter');
      await feedback.pressSequentially('Keyboard edit beyond 1000');
      const completeFeedback = longReviewFeedback + '\nKeyboard edit beyond 1000';
      await expect(feedback).not.toHaveAttribute('maxlength');
      await expect(feedback).toHaveValue(completeFeedback);
      await page.getByRole('button', { name: 'Back', exact: true }).click();
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toHaveLength(1);
      await action.click();
      await expect(feedback).toHaveValue(completeFeedback);
      if (!clarification) await page.getByRole('region', { name: 'Review decisions', exact: true }).locator('input[type=file]').setInputFiles({ name: 'feedback.md', mimeType: 'text/markdown', buffer: Buffer.from('# Feedback from browser') });
      await page.getByRole('button', { name: 'Read design.md', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Design', exact: true })).toBeVisible();
      await expect(feedback).toHaveValue(completeFeedback);

      for (const viewport of [{ width: 320, height: 568 }, { width: 768, height: 1024 }, { width: 1440, height: 900 }, { width: 390, height: 844 }]) {
        await page.setViewportSize(viewport);
        await expect(feedback).toHaveValue(completeFeedback);
        await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      }
      await expect(page.locator('.ant-input-data-count')).toHaveText(String(completeFeedback.length));
      await expect(confirm).toBeInViewport();
      await feedback.evaluate(element => { element.scrollTop = element.scrollHeight; });
      await page.screenshot({ path: testInfo.outputPath('long-feedback-phone.png') });
      await page.setViewportSize({ width: 1440, height: 900 });
      await expect(confirm).toBeInViewport();
      await page.screenshot({ path: testInfo.outputPath('long-feedback-desktop.png') });

      // A rejected request must remain actionable, with the draft preserved.
      let releaseFailure!: () => void;
      const failureGate = new Promise<void>(resolve => { releaseFailure = resolve; });
      await page.route(`**/reviews/${jobID}/respond`, async route => {
        expect(route.request().postData()).toContain(JSON.stringify(completeFeedback));
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
      await expect(feedback).toHaveValue(completeFeedback);
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toHaveLength(1);
      // Chromium exposes multipart upload data at interception, but can omit
      // it from the request associated with the eventual response event.
      await page.route(`**/reviews/${jobID}/respond`, async route => {
        expect(route.request().postData()).toContain(JSON.stringify(completeFeedback));
        await route.continue();
      }, { times: 1 });
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
          feedback: completeFeedback,
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

  test('opens a review while metadata requests are pending', async ({ page, recipeRun }, testInfo) => {
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
      const viewport = page.viewportSize()!;
      await page.setViewportSize({ ...viewport, height: 1000 });
      await page.screenshot({ path: testInfo.outputPath('review-metadata-pending.png'), fullPage: true });
      await page.setViewportSize(viewport);
    } finally { release(); }
    await expect(page.getByRole('region', { name: 'Goal prompt' })).toContainText(reviewGoalPrompt);
  });

  test('preserves feedback and attachment through context retry and submission', async ({ page, request, recipeRun }, testInfo) => {
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
    await page.getByRole('region', { name: 'Review decisions', exact: true }).locator('input[type=file]').setInputFiles({ name: 'feedback.md', mimeType: 'text/markdown', buffer: Buffer.from('# Browser feedback') });
    const viewport = page.viewportSize()!;
    await page.setViewportSize({ ...viewport, height: 1000 });
    await page.screenshot({ path: testInfo.outputPath('review-metadata-unavailable.png'), fullPage: true });
    await page.setViewportSize(viewport);
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
    await page.getByRole('button', { name: 'Show documents', exact: true }).click();
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
    const details=await(await request.get(`/api/projects/${tenant}/user-inputs/${jobID}`)).json();
    // The observer was the last opened tab; reviewer animations need a foreground tab.
    await page.bringToFront();
    const fetched: string[]=[]; const external: string[]=[];
    page.on('request',r=>{if(r.url().includes('/stored-document?'))fetched.push(r.url());if(r.url().includes('blocked.invalid'))external.push(r.url());});
    await page.goto(`/project/${tenant}/reviews`);
    await expect(page.getByRole('button',{name:'Open Review',exact:true})).toBeVisible();
    expect(fetched).toHaveLength(0);
    await setNativeZoom(page,2);
    await page.getByRole('button',{name:'Open Review',exact:true}).click();
    await expect(page.getByLabel('Document preview')).toBeVisible();
    await page.getByRole('button',{name:'Approve',exact:true}).click();
    const feedback=page.getByLabel('What should change?',{exact:true});
    await feedback.fill('Multi-format feedback');
    await page.getByRole('region', { name: 'Review decisions', exact: true }).locator('input[type=file]').setInputFiles({name:'feedback.md',mimeType:'text/markdown',buffer:Buffer.from('# Feedback')});
    await page.getByRole('button',{name:'Show documents',exact:true}).click();
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
    await page.getByRole('button',{name:'Document options'}).click();
    await page.getByText('Side by side',{exact:true}).click();
    await page.keyboard.press('Escape');await expect(page.getByLabel('Side by side diff')).toContainText('+new');
    await page.screenshot({path:testInfo.outputPath('dual-pane.png')});await back();
    await page.route(/\/stored-document\?.*name=checks\.log(?:&|$)/,route=>route.fulfill({status:404,body:'missing document'}),{times:1});
    await read('checks.log');await expect(page.getByText(/Could not load this document/)).toBeVisible();await page.getByRole('button',{name:'Retry',exact:true}).click();await page.getByLabel('Search document').fill('needle');await expect(page.locator('mark')).toHaveText('needle');await back();
    await read('changes.patch');await expect(page.getByLabel('Side by side diff')).toBeVisible();await back();
    for(const [name,id] of [['mock.html','html'],['delayed.html','delayed'],['vector.svg','svg']] as const){
      await read(name);
      await expect.poll(()=>{
        const url=new URL(page.url());
        return {pathname:url.pathname,requestIds:url.searchParams.getAll('request_id'),documentIds:url.searchParams.getAll('document_id')};
      }).toEqual({pathname:`/project/${tenant}/reviews/${jobID}`,requestIds:[details.form.request_id],documentIds:[id]});
      const selectedReviewURL=page.url();
      if(name.endsWith('.html')) {
        await expect(page.getByText(/Interactive HTML runs automatically/)).toBeVisible();
        const iframe=page.getByTitle(`Interactive HTML preview: ${name}`);
        await expect(iframe).toHaveAttribute('sandbox','allow-scripts');
        await expect(iframe).toHaveAttribute('credentialless','');
        await expect(iframe).toHaveAttribute('referrerpolicy','no-referrer');
        await expect(iframe).toHaveAttribute('csp',/connect-src 'none'/);
        const frame=page.frameLocator(`iframe[title="Interactive HTML preview: ${name}"]`);
        if(name==='mock.html') {
          await expect(frame.getByRole('heading')).toHaveCSS('color','rgb(20, 100, 160)');
          await expect(frame.locator('output')).toHaveText('first second');
          await expect.poll(()=>frame.getByAltText('Authorized screenshot').evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBe(1280);
          // Native zoom plus narrow viewport can misplace Chromium's iframe
          // pointer coordinates. Exercise normal keyboard activation instead.
          await frame.getByRole('button',{name:'Activate'}).focus();
          await page.keyboard.press('Enter');
          await expect(frame.locator('output')).toHaveText('clicked');
        } else await expect(frame.locator('output')).toHaveText('connection blocked');
        await page.getByRole('region',{name:'Selected document',exact:true}).screenshot({path:testInfo.outputPath(`${name}-interactive.png`)});
      } else {
        await expect(page.getByText(/Static preview:/)).toBeVisible();
        const image=page.getByRole('img',{name,exact:true});
        await expect(image).toBeVisible();
        await expect.poll(()=>image.evaluate((el:HTMLImageElement)=>el.complete&&el.naturalWidth>0)).toBe(true);
        await expect(page.locator('iframe')).toHaveCount(0);
      }
      // Observe delayed connection attempts while the rendered preview is active.
      await page.waitForTimeout(2100);expect(external).toHaveLength(0);
      await expect(page).toHaveURL(selectedReviewURL);
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
    expect(fetched.every(u=>new URL(u).searchParams.get('name') !== details.form.documents.rollout.stored.key.name)).toBe(true);
    await expect(feedback).toHaveValue('Multi-format feedback');
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
  test('REV-007 submits rejection feedback with the original revise value through c2j',async({page,request,recipeRun},testInfo)=>{
    const {tenant,jobID,stopWorker,startWorker}=recipeRun;
    await stopWorker();
    await page.goto(`/project/${tenant}/reviews/${jobID}`);
    await page.getByRole('button',{name:'Reject',exact:true}).click();
    const reason=page.getByLabel('What should change?',{exact:true});
    await expect(reason).toBeFocused();
    await reason.fill('   ');
    await page.getByRole('button',{name:'OK'}).click();
    await expect(page.getByText('What should change? is required')).toBeVisible();
    await reason.fill(longReviewFeedback);
    await expect(reason).toHaveValue(longReviewFeedback);
    await page.getByRole('button',{name:'Back',exact:true}).click();
    await page.getByRole('button',{name:'Reject',exact:true}).click();
    await expect(reason).toHaveValue(longReviewFeedback);
    await page.screenshot({path:testInfo.outputPath('long-feedback-reject.png')});
    const [response]=await Promise.all([
      page.waitForResponse(r=>r.url().endsWith(`/reviews/${jobID}/respond`) && r.request().method()==='POST'),
      page.getByRole('button',{name:'OK'}).click(),
    ]);
    expect(response.request().postData()).toContain(JSON.stringify(longReviewFeedback));
    expect(response.status(),await response.text()).toBe(200);
    startWorker();
    await expect.poll(async()=>{
      const r=await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
      return r.status()===200?r.json():null;
    },{timeout:30000}).toMatchObject({status:'completed',output:{answers:{decision:'revise',feedback:longReviewFeedback}}});
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
    await page.route(`**/api/projects/${tenant}/jobs/fixture-producer/stored-document?*`,async route=>{
      const name=new URL(route.request().url()).searchParams.get('name')!;
      const id=name==='captures/gallery.md'?'gallery':`image/${names.indexOf(name.slice('captures/'.length))}`;fetched.push(id);
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
    await expect(page.locator('.review-reading-track')).toBeVisible();
    await page.locator('.review-markdown').evaluate(el=>el.scrollIntoView());
    await expect(page.getByText('Loading attached image…',{exact:true})).toHaveCount(0);
    await expect.poll(()=>images.evaluateAll(elements=>elements.length===6&&elements.every(el=>(el as HTMLImageElement).naturalWidth>0))).toBe(true);
    await page.screenshot({path:testInfo.outputPath('gallery-desktop.png')});
    await session.detach();
  });
});

test.describe('review/v2 continuous discussion',()=>{
  test.use({recipeFixture:'browser-review-v2'});
  test('holds the retained reader while accepted history is delayed or unavailable',async({page,recipeRun},testInfo)=>{
    const {tenant,jobID,stopWorker}=recipeRun;
    await stopWorker();
    await page.goto(`/project/${tenant}/jobs/${jobID}/story`);
    const inline=page.locator('.embedded-review');
    await expect(inline).toContainText('Keep unsent drafts');
    await expect(inline).toContainText('Approve this direction so detailed planning can begin?');
    await expect(page.getByRole('button',{name:'Open form',exact:true})).toHaveCount(0);
    await page.screenshot({path:testInfo.outputPath('review-v2-story-desktop.png'),fullPage:true});
    await page.setViewportSize({width:390,height:844});
    await expect(inline.getByRole('button',{name:'Ask a question',exact:true})).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`/jobs/${jobID}/story`));
    await page.screenshot({path:testInfo.outputPath('review-v2-story-phone.png'),fullPage:true});
    await expect(page.getByText('Original material remains readable while questions are answered.',{exact:true})).toBeVisible();
    await page.screenshot({path:testInfo.outputPath('review-v2-direct-reader-phone.png'),fullPage:true});
    await page.setViewportSize({width:1320,height:900});
    const catalog=inline.getByRole('navigation',{name:'Review documents',exact:true});
    await inline.getByRole('button',{name:'Show documents',exact:true}).click();
    await catalog.getByRole('button',{name:'Recovery notes',exact:false}).first().click();
    const original=page.getByRole('heading',{name:'Recovery',exact:true});
    await expect(original).toBeVisible();
    const reader=await original.elementHandle();
    const download=page.getByRole('link',{name:'Download',exact:true});
    const originalURL=await download.getAttribute('href');
    await page.getByRole('button',{name:'Ask a question',exact:true}).click();
    await page.getByLabel('Question or requested change (required)',{exact:true}).fill('Keep this document open while answering.');
    let release!:()=>void;
    const gate=new Promise<void>(resolve=>{release=resolve;});
    await page.route(`**/api/projects/${tenant}/jobs/${jobID}/conversation**`,async route=>{
      await gate;await route.fulfill({status:503,json:{error:'History temporarily unavailable'}}).catch(()=>{});
    });
    try {
      await page.getByRole('button',{name:'Send · Ask a question',exact:true}).click();
      await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
      await expect(original).toBeVisible();
      expect(await reader!.evaluate(el=>el.isConnected)).toBe(true);
      await expect(download).toHaveAttribute('href',originalURL!);
      await page.evaluate(()=>window.scrollTo(0,0));
      const session=await page.context().newCDPSession(page);
      const {cssContentSize}=await session.send('Page.getLayoutMetrics');
      const {data}=await session.send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:cssContentSize.width,height:cssContentSize.height,scale:1}});
      writeFileSync(testInfo.outputPath('review-v2-delayed-history.png'),Buffer.from(data,'base64'));await session.detach();
      release();
      await expect(page.getByText('Review state unavailable',{exact:true})).toBeVisible();
      await expect(original).toBeVisible();
      expect(await reader!.evaluate(el=>el.isConnected)).toBe(true);
      await expect(download).toHaveAttribute('href',originalURL!);
      await expect(page.getByText('Keep this document open while answering.',{exact:true})).toBeVisible();
    } finally {release();await page.unrouteAll({behavior:'wait'});}
  });
  test('preserves incompatible drafts until explicit removal before clean approval',async({page,recipeRun},testInfo)=>{
    const {tenant,jobID,stopWorker}=recipeRun;
    const capture=async(name:string)=>{
      await page.evaluate(()=>window.scrollTo(0,0));
      const session=await page.context().newCDPSession(page);
      await page.evaluate(()=>new Promise<void>(done=>requestAnimationFrame(()=>requestAnimationFrame(()=>done()))));
      const {cssContentSize}=await session.send('Page.getLayoutMetrics');
      const {data}=await session.send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:cssContentSize.width,height:cssContentSize.height,scale:1}});
      writeFileSync(testInfo.outputPath(name),Buffer.from(data,'base64'));await session.detach();
    };
    await stopWorker();
    await page.goto(`/project/${tenant}/reviews/${jobID}`);
    await page.getByRole('button',{name:'Review doc comments',exact:true}).click();
    await page.getByRole('button',{name:'Send · Review doc comments',exact:true}).click();
    await expect(page.getByText('This direction requires feedback or a permitted attachment.',{exact:true})).toBeVisible();
    const exactDraft='  Please explain the recovery path.\n';
    await page.getByLabel('Question or requested change (required)',{exact:true}).fill(exactDraft);
    await page.getByLabel('Attach notes',{exact:true}).setInputFiles({name:'comments.md',mimeType:'text/markdown',buffer:Buffer.from('Revise the document wording.')});
    await expect(page.getByRole('button',{name:'Send · Review doc comments',exact:true})).toBeVisible();
    await page.getByRole('button',{name:'Back',exact:true}).click();
    await page.getByRole('button',{name:'Approve direction',exact:true}).click();
    await expect(page.getByRole('button',{name:'Send · Review doc comments',exact:true})).toBeHidden();
    await expect(page.getByLabel('Question or requested change (not allowed)',{exact:true})).toHaveValue(exactDraft);
    await expect(page.getByText('comments.md (28 bytes)',{exact:true})).toBeVisible();
    await expect(page.getByText(/This direction does not allow feedback/)).toBeVisible();
    await expect(page.getByRole('button',{name:'Send · Approve direction',exact:true})).toHaveCount(0);
    await capture('review-v2-approval-drafts.png');
    // Returning to a permitted direction recovers both drafts without filtering.
    await page.getByRole('button',{name:'Request changes',exact:true}).click();
    await expect(page.getByLabel('Question or requested change (required)',{exact:true})).toHaveValue(exactDraft);
    await expect(page.getByRole('button',{name:'Remove comments.md (1)',exact:true})).toBeVisible();
    await page.getByRole('button',{name:'Review settings',exact:true}).click();
    await page.getByRole('dialog').getByLabel('Response style',{exact:true}).selectOption('0');
    await page.getByRole('dialog').getByLabel('Example count',{exact:true}).fill('');
    await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
    await page.getByRole('button',{name:'Send · Request changes',exact:true}).click();
    await expect(page.getByText(/Example count: Expected integer/)).toBeVisible();
    await page.getByRole('button',{name:'Back',exact:true}).click();
    await page.getByRole('button',{name:'Approve direction',exact:true}).click();
    await page.getByRole('button',{name:'Remove feedback draft',exact:true}).click();
    await expect(page.getByText(/This direction does not allow attachments/)).toBeVisible();
    await page.getByRole('button',{name:'Remove comments.md (1)',exact:true}).click();
    await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toBeVisible();
    await capture('review-v2-clean-approval.png');
    const posted=page.waitForRequest(r=>r.url().endsWith(`/reviews/${jobID}/respond`)&&r.method()==='POST');
    await page.getByRole('button',{name:'Approve direction',exact:true}).click();
    const body=(await posted).postData()!;
    expect(body).toContain('"feedback":""');expect(body).toContain('"settings":{"response_style":"brief"}');
    expect(body).not.toContain('comments.md');expect(body).not.toContain('explain the recovery');expect(body).not.toContain('"examples"');
    await expect(page.getByText('Response sent',{exact:true})).toBeVisible();
  });
  test('keeps the reader through two questions, uploads, gap reload and typed settings',async({page,request,recipeRun},testInfo)=>{
    const {tenant,jobID,stopWorker,startWorker}=recipeRun;
    await stopWorker();
    const inputReads:number[]=[];
    page.on('response',response=>{if(response.url().endsWith(`/user-inputs/${jobID}`))inputReads.push(response.status());});
    const capture=async(name:string)=>{
      const session=await page.context().newCDPSession(page);
      await page.evaluate(()=>new Promise<void>(done=>requestAnimationFrame(()=>requestAnimationFrame(()=>done()))));
      const {cssContentSize}=await session.send('Page.getLayoutMetrics');
      const {data}=await session.send('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:cssContentSize.width,height:cssContentSize.height,scale:1}});
      writeFileSync(testInfo.outputPath(name),Buffer.from(data,'base64'));await session.detach();
    };
    await page.goto(`/project/${tenant}/reviews/${jobID}`);
    await expect(page.getByRole('heading',{name:'Keep unsent drafts',exact:true})).toHaveCount(2); // title and immutable Markdown
    await expect(page).toHaveURL(/review_id=/);
    expect(inputReads).toEqual([200]);
    await page.getByRole('button',{name:'Show documents',exact:true}).click();
    await page.getByRole('button',{name:'Recovery notes',exact:false}).first().click();
    await expect(page.getByRole('heading',{name:'Recovery',exact:true})).toBeVisible();
    await page.getByRole('button',{name:'Ask a question',exact:true}).click();
    await page.getByRole('button',{name:'Send · Ask a question',exact:true}).click();
    await expect(page.getByText('This direction requires feedback or a permitted attachment.',{exact:true})).toBeVisible();
    await page.getByRole('button',{name:'Review settings',exact:true}).click();
    const modal=page.getByRole('dialog');
    await modal.getByLabel('Response style', {exact:true}).selectOption('0');
    await modal.getByRole('button',{name:'Use default context',exact:true}).click();
    await modal.getByRole('checkbox',{name:'Include citations',exact:true}).uncheck();
    await expect(modal.getByRole('checkbox',{name:'Include citations',exact:true})).not.toBeChecked();
    await page.screenshot({path:testInfo.outputPath('review-v2-settings-more.png')});
    await modal.locator('.ant-modal-body').evaluate(el=>{el.scrollTop=0;});
    await page.screenshot({path:testInfo.outputPath('review-v2-settings.png')});
    await modal.getByRole('button',{name:'Done',exact:true}).click();
    await expect(page.getByRole('button',{name:'Review settings',exact:true})).toBeFocused();
    const exactQuestion='  What survives a restart?\n';
    await page.getByLabel('Question or requested change (required)',{exact:true}).fill(exactQuestion);
    await expect(page.locator('.review-v2-response').getByText('Response style:',{exact:false})).toBeVisible();
    await page.getByRole('button',{name:'Hide documents',exact:true}).click();
    await page.evaluate(()=>window.scrollTo(0,0));
    await capture('review-v2-desktop.png');
    await page.getByRole('button',{name:'Send · Ask a question',exact:true}).click();
    await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
    await expect(page.getByRole('heading',{name:'Recovery',exact:true})).toBeVisible();
    const gapReads=inputReads.length;
    await page.waitForTimeout(10_500); // Two history refresh periods without probing missing input.
    expect(inputReads).toHaveLength(gapReads);
    await page.reload();
    await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
    await expect(page.getByText(exactQuestion,{exact:true})).toBeVisible();
    await capture('review-v2-waiting.png');
    expect(inputReads).toHaveLength(gapReads);
    expect(inputReads.every(status=>status===200 || status===204)).toBe(true);
    startWorker();
    await expect(page.getByText('Saved drafts return when you reopen the app.',{exact:true})).toBeVisible({timeout:30000});
    await expect.poll(() => [...inputReads]).toEqual([200,200]);
    await stopWorker();
    await page.getByRole('button',{name:'Ask a question',exact:true}).click();
    await page.getByLabel('Attach notes',{exact:true}).setInputFiles([
      {name:'notes.md',mimeType:'text/markdown',buffer:Buffer.from('first note')},
      {name:'notes.md',mimeType:'text/markdown',buffer:Buffer.from('second note')}
    ]);
    await page.getByRole('button',{name:'Send · Ask a question',exact:true}).click();
    await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
    startWorker();
    await expect(page.getByText('Earlier notes keep their original producer references.',{exact:true})).toBeVisible({timeout:30000});
    await expect.poll(() => [...inputReads]).toEqual([200,200,200]);
    await stopWorker();
    const links=page.getByRole('link',{name:'Download notes.md',exact:true});
    await expect(links).toHaveCount(2);
    expect(await (await request.get((await links.nth(0).getAttribute('href'))!)).text()).toBe('first note');
    expect(await (await request.get((await links.nth(1).getAttribute('href'))!)).text()).toBe('second note');
    await page.setViewportSize({width:390,height:844});
    await capture('review-v2-phone.png');
    await page.getByRole('button',{name:'Approve direction',exact:true}).click();
    await expect(page.getByText('Response sent',{exact:true})).toBeVisible();
    startWorker();
    let output:any;
    await expect.poll(async()=>{
      const result=await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
      if(result.status()!==200)return false;output=(await result.json()).output;return !!output?.report;
    },{timeout:30000}).toBe(true);
    expect(output.report).toBe('Brief report');
    expect(output.first.response.settings).toEqual({response_style:'brief',context_hint:null,citations:false});
    expect(output.first.response.feedback).toBe(exactQuestion);
    expect(output.second.response.attachments).toHaveLength(2);
    expect(output.second.response.attachments[0].stored.key.name).not.toBe(output.second.response.attachments[1].stored.key.name);
    expect(output.first.receipt.actor).toEqual({id:'browser@example.com',kind:'human'});
    expect(output.final.response.direction).toBe('approve');
  });
});

test.describe('review/v2 acceptance races',()=>{
 test.use({recipeFixture:'browser-review-v2'});
 test('reconciles a lost success response and starts the returned review with a clean contribution',async({page,context,request,recipeRun},testInfo)=>{
  const {tenant,jobID,stopWorker,startWorker}=recipeRun;await stopWorker();
  const path=`/project/${tenant}/reviews/${jobID}`;await page.goto(path);
  await expect(page.getByRole('button',{name:'Ask a question',exact:true})).toBeVisible();
  const other=await context.newPage();await other.goto(path);await expect(other.getByRole('button',{name:'Ask a question',exact:true})).toBeVisible();
  const bodies:string[]=[];
  await page.route(`**/api/projects/${tenant}/reviews/${jobID}/respond`,async route=>{
   bodies.push(route.request().postData()??'');const result=await route.fetch();expect(result.status()).toBe(200);
   if(bodies.length===1)await route.abort('failed');else await route.fulfill({response:result});
  });
  await page.getByRole('button',{name:'Review settings',exact:true}).click();
  const modal=page.getByRole('dialog');
  await modal.getByLabel('Response style',{exact:true}).selectOption('0');
  await modal.getByLabel('Example count',{exact:true}).fill('0'); // Does not apply to a question.
  await modal.getByRole('button',{name:'Done',exact:true}).click();
  await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)',{exact:true}).fill('Was this response accepted?');
  await page.getByLabel('Attach notes',{exact:true}).setInputFiles({name:'accepted-notes.md',mimeType:'text/markdown',buffer:Buffer.from('Already accepted notes')});
  await page.getByRole('button',{name:'Send · Ask a question',exact:true}).dblclick();
  await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
  await expect(other.getByText('Response pending',{exact:true})).toBeVisible();
  await expect(page.getByRole('region',{name:'Discussion'}).getByText('Was this response accepted?',{exact:true})).toHaveCount(1);
  expect(bodies).toHaveLength(1);await expect(page.getByRole('button',{name:'Send · Ask a question',exact:true})).toHaveCount(0);
  startWorker();
  await expect(page.getByText('Saved drafts return when you reopen the app.',{exact:true})).toBeVisible({timeout:30000});
  await stopWorker();
  await expect(other.getByText('Saved drafts return when you reopen the app.',{exact:true})).toBeVisible();
  await expect(page.getByRole('radio')).toHaveCount(0);
  await expect(page.getByLabel('Question or requested change (optional)',{exact:true})).toHaveValue('');
  await expect(page.getByLabel('Attach notes',{exact:true})).toHaveValue('');
  await expect(page.getByRole('button',{name:'Remove accepted-notes.md (1)',exact:true})).toHaveCount(0);
  await expect(page.getByText(/Check the refreshed request before retrying/)).toHaveCount(0);
  const saved=page.getByRole('link',{name:'Download accepted-notes.md',exact:true});
  await expect(saved).toHaveCount(1);
  expect(await (await request.get((await saved.getAttribute('href'))!)).text()).toBe('Already accepted notes');
  await page.getByRole('button',{name:'Review settings',exact:true}).click();
  await expect(modal.getByLabel('Example count',{exact:true})).toHaveValue('0');
  await modal.getByRole('button',{name:'Done',exact:true}).click();
  await page.evaluate(()=>window.scrollTo(0,0));
  await page.screenshot({path:testInfo.outputPath('review-v2-recovered-answer.png'),fullPage:true});
  await expect(page.getByRole('button',{name:'Approve direction',exact:true})).toBeVisible();
  await page.evaluate(()=>window.scrollTo(0,0));
  await page.screenshot({path:testInfo.outputPath('review-v2-recovered-approval.png'),fullPage:true});
  await page.getByRole('button',{name:'Approve direction',exact:true}).click();
  await expect(page.getByText('Response sent',{exact:true})).toBeVisible();
  expect(bodies).toHaveLength(2);
  expect(bodies[1]).toContain('"direction":"approve","feedback":"","attachments":[],"settings":{}');
  expect(bodies[1]).not.toContain('accepted-notes.md');expect(bodies[1]).not.toContain('Was this response accepted?');
  await other.close();
 });
 test('shows actual cancellation with the saved question readable',async({page,request,recipeRun})=>{
  const {tenant,jobID,stopWorker}=recipeRun;await stopWorker();await page.goto(`/project/${tenant}/reviews/${jobID}`);
  await page.getByRole('button',{name:'Ask a question',exact:true}).click();await page.getByLabel('Question or requested change (required)',{exact:true}).fill('Retain this question.');
  await page.getByRole('button',{name:'Send · Ask a question',exact:true}).click();
  await expect(page.getByText('Response pending',{exact:true})).toBeVisible();
  const cancelled=await request.post(`/api/projects/${tenant}/jobs/${jobID}/cancel`);expect(cancelled.status()).toBe(204);
  await expect(page.getByText('Job cancelled.',{exact:true})).toBeVisible();
  await expect(page.getByText('Retain this question.',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Ask a question',exact:true})).toHaveCount(0);
 });
});

test.describe('embedded story review layout', () => {
  test.use({ recipeFixture: 'browser-review' });
  test('wraps long literal titles and instructions at each width', async ({ page, recipeRun }, testInfo) => {
    const { tenant, jobID, stopWorker } = recipeRun;
    await stopWorker();
    await page.bringToFront();
    const title = 'Review-' + 'long-title'.repeat(35);
    const instruction = 'Read the design and rollout documents before choosing a decision. '.repeat(14);
    await page.route(`**/api/projects/${tenant}/user-inputs/${jobID}`, async route => {
      const response = await route.fetch();
      const body = await response.json();
      await route.fulfill({ response, json: { ...body, form: { ...body.form, title, fields: body.form.fields.map((field: { id: string }) => field.id === 'decision' ? { ...field, question: instruction } : field) } } });
    });
    await page.goto(`/project/${tenant}/jobs/${jobID}/story`);
    const inline = page.locator('.embedded-review');
    for (const width of [1440, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 900 });
      await expect(inline.getByRole('heading', {name:title,exact:true})).toBeVisible();
      await expect(inline).toContainText(instruction.trim());
      await expect(inline.getByRole('button',{name:'Approve',exact:true})).toBeVisible();
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    }
    await inline.screenshot({ path: testInfo.outputPath('long-review-inline-phone.png') });
  });
});
