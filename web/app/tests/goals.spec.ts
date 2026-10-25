import { test, expect } from '@playwright/test';

test('renders a single compact row or grouped attempts with metadata and waiting links', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const created = '2026-10-25T10:00:00Z';
  const first = { tenant_id: 'goal-layout', job_id: 'first', goal_id: 'shared-goal', prompt: 'Build a reliable release dashboard', recipe: 'build', job_type: 'recipe', cell_name: 'platform', repo: 'https://github.com/example/platform.git', git_ref: 'main', submitted_at: created, created_at: created, store: 'ACTIVE', status: 'READY', next_route: { jobType: 'recipe', taskType: 'input:collect_user_input' }, parent: { job_id: 'coordinator', tenant_id: 'goal-layout' }, execution: { status: 'published', source: 'metadata', published: true, demand: { memory: '4Gi' } } };
  const second = { ...first, job_id: 'second', store: 'ARCHIVED', status: 'COMPLETED', completion_status: 'failed_app', completion_detail: 'The deployment check failed' };
  const solo = { ...first, job_id: 'solo', goal_id: 'solo-goal', prompt: undefined, status: 'PENDING_JOBS', next_route: undefined, parent: undefined, blocking_wait: { waitId: 'wait', state: 'PENDING', dependencies: [{ target: { goalId: 'dependency' }, condition: 'success', state: 'PENDING' }, { target: { jobId: 'job-dependency' }, condition: 'complete', state: 'PENDING' }] } };
  let reads = 0;
  await page.route('**/api/projects/goal-layout/jobs?*', route => {
    reads++;
    return route.fulfill({ json: { jobs: [first, second, solo] } });
  });
  await page.goto('/project/goal-layout/goals');
  await expect(page.getByRole('link', { name: first.prompt, exact: true })).toHaveCount(1);
  await expect(page.getByRole('link', { name: 'solo-goal', exact: true })).toBeVisible();
  const row = page.getByRole('link', { name: 'Open job first' }).locator('xpath=ancestor::tr');
  await expect(row.getByRole('link', { name: 'Open input/review' })).toHaveAttribute('href', '/project/goal-layout/inputs/first');
  await expect(row.getByRole('link', { name: 'Parent' })).toHaveAttribute('href', '/project/goal-layout/jobs/coordinator/story');
  await expect(row.getByRole('link', { name: 'platform' })).toHaveAttribute('href', first.repo);
  await expect(page.getByRole('link', { name: 'Goal dependency' })).toHaveAttribute('href', '/project/goal-layout/goals/dependency');
  await expect(page.getByRole('link', { name: 'Job job-dependency' })).toHaveAttribute('href', '/project/goal-layout/jobs/job-dependency/story');
  const initialReads = reads;
  await row.getByRole('button', { name: 'Expand row', exact: true }).click();
  await page.getByText('Execution requirements', { exact: true }).click();
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toContainText('4Gi');
  expect(reads).toBe(initialReads);
  await page.screenshot({ path: '/tmp/cortex-goals-list.png', fullPage: true });
});

test('filters through the jobs API before pagination and restores filters through history', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const queries: URLSearchParams[] = [];
  const goalReads: string[] = [];
  page.on('request', request => {
    if (new URL(request.url()).pathname.startsWith('/api/projects/filter-test/goals')) goalReads.push(request.url());
  });
  await page.route('**/api/projects/filter-test/jobs?*', route => {
    const query = new URL(route.request().url()).searchParams;
    queries.push(query);
    const cancelled = query.get('status') === 'CANCELLED';
    const job_id = cancelled ? 'cancelled-on-another-page' : query.has('pageToken') ? 'later-ready' : 'first-ready';
    return route.fulfill({ json: { jobs: [{ tenant_id: 'filter-test', job_id, goal_id: job_id, recipe: 'build', job_type: 'recipe', status: cancelled ? 'CANCELLED' : 'READY', store: cancelled ? 'ARCHIVED' : 'ACTIVE' }], next_page_token: cancelled ? undefined : 'next-page' } });
  });
  await page.goto('/project/filter-test/goals?status=READY&pageToken=later-page');
  await expect(page.getByRole('link', { name: 'later-ready', exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'Status filter' }).click();
  await page.locator('.ant-select-dropdown:visible').getByText('Cancelled', { exact: true }).click();
  // Remove Ready so the new query contains exactly the requested status.
  await page.locator('.ant-select-dropdown:visible').getByText('Ready', { exact: true }).click();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('link', { name: 'cancelled-on-another-page', exact: true })).toBeVisible();
  await expect.poll(() => queries.at(-1)?.getAll('status')).toEqual(['CANCELLED']);
  expect(queries.at(-1)?.has('pageToken')).toBe(false);
  expect(new URL(page.url()).searchParams.has('pageToken')).toBe(false);
  const filteredURL = page.url();
  await page.getByRole('combobox', { name: 'Status filter' }).click();
  await page.locator('.ant-select-dropdown:visible').getByText('Cancelled', { exact: true }).click();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('link', { name: 'first-ready', exact: true })).toBeVisible();
  expect(queries.at(-1)?.getAll('status')).toEqual(['all']);
  await page.goBack();
  await expect(page).toHaveURL(filteredURL);
  await expect(page.getByRole('link', { name: 'cancelled-on-another-page', exact: true })).toBeVisible();
  await page.goForward();
  await expect(page.getByRole('link', { name: 'first-ready', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Next jobs' }).click();
  await expect(page.getByRole('link', { name: 'later-ready', exact: true })).toBeVisible();
  expect(queries.at(-1)?.get('pageToken')).toBe('next-page');
  expect(queries.at(-1)?.getAll('status')).toEqual(['all']);
  expect(goalReads).toEqual([]);
});
