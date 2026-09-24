import { test as base, expect, type Page } from '@playwright/test';
import { spawn, execFileSync, type ChildProcess } from 'node:child_process';
import { mkdtempSync, mkdirSync, copyFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';

// Real production Cortex, HTTP JobDB, pinned c2j CLI, Git recipe, and browser SSE.
const test = base.extend<{
  recipeFixture: string;
  recipeRun: {
    tenant: string;
    jobID: string;
    observer: Page;
    startWorker: () => void;
    stopWorker: () => Promise<void>;
  };
}>({
  recipeFixture: ['browser-input', { option: true }],
  recipeRun: async ({ page, context, request, recipeFixture }, use, testInfo) => {
    const tenant = `input-${randomUUID()}`;
    const jobID = `recipe-${randomUUID()}`;
    const repo = mkdtempSync(join(tmpdir(), 'cortex-input-'));
    const workerPath = resolve('../../build/c2j-test-worker');
    let worker: ChildProcess | undefined;
    let workerLog = '';
    const errors: string[] = [];
    context.on('page', browserPage => browserPage.on('pageerror', error => errors.push(error.message)));
    page.on('pageerror', error => errors.push(error.message));

    const startWorker = () => {
      worker = spawn(workerPath, ['run', 'loop', '--jobdb', `http://127.0.0.1:18082/${tenant}`], {
        cwd: repo,
        env: {
          ...process.env,
          GIT_TERMINAL_PROMPT: '0',
          // Resolve the existing test cell to a real local checkout without network access.
          GIT_CONFIG_COUNT: '1',
          GIT_CONFIG_KEY_0: `url.${pathToFileURL(repo).href}.insteadOf`,
          GIT_CONFIG_VALUE_0: 'https://github.com/cortex-test/platform.git',
        },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      worker.stdout!.on('data', data => { workerLog += data.toString(); });
      worker.stderr!.on('data', data => { workerLog += data.toString(); });
      worker.on('error', error => { workerLog += error.stack; });
    };
    const stopWorker = async () => {
      if (!worker?.pid || worker.exitCode !== null || worker.signalCode !== null) return;
      const stopped = new Promise<void>(done => worker!.once('close', () => done()));
      worker.kill('SIGTERM');
      const timeout = setTimeout(() => worker?.kill('SIGKILL'), 5_000);
      await stopped;
      clearTimeout(timeout);
    };

    try {
      mkdirSync(join(repo, '.c2j/recipes'), { recursive: true });
      copyFileSync(resolve(`tests/fixtures/recipes/${recipeFixture}.yaml`), join(repo, '.c2j/recipes/browser-input.yaml'));
      const git = (...args: string[]) => execFileSync('git', ['-C', repo, ...args], { stdio: 'pipe' });
      git('init', '-b', 'main');
      git('add', '.');
      git('-c', 'user.name=Cortex Browser Test', '-c', 'user.email=browser@example.com', '-c', 'commit.gpgsign=false', 'commit', '-m', 'Input recipe fixture');
      const recipe = `git+${pathToFileURL(repo).href}//.c2j/recipes/browser-input.yaml@main`;

      await page.goto(`/?tenantId=${tenant}`);
      await page.getByPlaceholder('Email address').fill('browser@example.com');
      await page.getByRole('button', { name: 'Continue', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Jobs', exact: true })).toBeVisible();

      // This second tab must update through SSE, without refresh or navigation.
      const observer = await context.newPage();
      await observer.goto(`/project/${tenant}/inputs`);
      await expect(observer.getByText('No pending inputs')).toBeVisible();

      await page.getByRole('button', { name: 'Submit Job' }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByLabel('Job ID').fill(jobID);
      await dialog.getByLabel('Recipe').fill(recipe);
      const submitted = page.waitForResponse(r => r.url().endsWith(`/projects/${tenant}/jobs`) && r.request().method() === 'POST');
      await dialog.getByRole('button', { name: 'OK', exact: true }).click();
      expect((await submitted).status()).toBe(201);
      await expect(dialog).not.toBeVisible();
      startWorker();

      await expect(observer.getByText(`Job ID: ${jobID}`, { exact: true })).toBeVisible({ timeout: 30_000 });
      const otherTenant = await request.get(`/api/projects/other-${tenant}/user-inputs/pending`);
      expect(await otherTenant.json()).toEqual([]);

      await use({ tenant, jobID, observer, startWorker, stopWorker });
      expect(errors).toEqual([]);
    } finally {
      await stopWorker();
      try {
        await testInfo.attach('c2j-worker.log', { body: workerLog, contentType: 'text/plain' });
        const outcome = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
        await testInfo.attach('recipe-outcome.json', { body: await outcome.body(), contentType: 'application/json' });
      } finally {
        rmSync(repo, { recursive: true, force: true });
      }
    }
  },
});

test.beforeEach(() => test.setTimeout(90_000));

for (const multiField of [false, true]) {
  test.describe(multiField ? 'multi-field' : 'single-question', () => {
    test.use({ recipeFixture: multiField ? 'browser-input-fields' : 'browser-input' });
    test('answers recipe input and resumes the worker to completion', async ({ page, request, recipeRun }) => {
      const { tenant, jobID, observer, startWorker, stopWorker } = recipeRun;
      const answer = `approved-${randomUUID()}`;
      // Kill the worker while the request is pending: the answer must survive
      // in JobDB and be picked up by a fresh worker, not an in-process promise.
      await stopWorker();
      await page.getByRole('link', { name: jobID, exact: true }).click();
      await page.getByRole('tab', { name: 'Pending Input', exact: true }).click();
      const question = multiField ? 'Release note' : 'What should the recipe publish?';
      await expect(page.getByLabel(question)).toBeVisible();
      await page.reload();
      await page.getByRole('tab', { name: 'Pending Input', exact: true }).click();
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
      await expect(observer.getByText('No pending inputs')).toBeVisible();
      expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toEqual([]);

      startWorker();
      await expect.poll(async () => {
        const response = await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`);
        return response.status() === 200 ? response.json() : { status: response.status() };
      }, { timeout: 30_000, message: 'The resumed recipe must return the exact browser answer' }).toMatchObject({
        status: 'completed',
        output: multiField ? { answer, decision: 'approve' } : { answer },
      });
      await expect(page.getByRole('cell', { name: 'COMPLETED', exact: true }).first()).toBeVisible();
    });
  });
}

test('cancellation removes a pending prompt in open tabs and rejects late answers', async ({ page, request, recipeRun }) => {
  const { tenant, jobID, observer, stopWorker, startWorker } = recipeRun;
  await page.getByRole('link', { name: jobID, exact: true }).click();
  await page.getByRole('tab', { name: 'Pending Input', exact: true }).click();
  await expect(page.getByLabel('What should the recipe publish?')).toBeVisible();
  await stopWorker();

  // Cancellation can originate outside this browser. Both tabs must react to SSE.
  const cancelled = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/cancel`, {
    data: { reason: 'Browser integration cancellation' },
  });
  expect(cancelled.status(), await cancelled.text()).toBe(200);
  await expect(observer.getByText('No pending inputs')).toBeVisible();
  await expect(page.getByLabel('What should the recipe publish?')).not.toBeVisible();
  expect(await (await request.get(`/api/projects/${tenant}/user-inputs/pending`)).json()).toEqual([]);

  const lateAnswer = await request.post(`/api/projects/${tenant}/user-inputs/${jobID}/respond`, {
    data: { fields: { response: 'too late' }, response: 'too late' },
  });
  expect(lateAnswer.ok()).toBe(false);
  startWorker();
  await expect.poll(async () => (await request.get(`/api/projects/${tenant}/jobs/${jobID}/outcome`)).json()).toMatchObject({ status: 'canceled' });
});
