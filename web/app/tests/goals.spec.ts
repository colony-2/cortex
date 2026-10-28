import { test, expect } from '@playwright/test';

test('renders a single compact row or grouped attempts with metadata and waiting links', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const created = '2026-10-25T10:00:00Z';
  const first = { tenant_id: 'goal-layout', job_id: 'first', goal_id: 'shared-goal', prompt: 'Build a reliable release dashboard', recipe: 'build', job_type: 'recipe', cell_name: 'platform', repo: 'https://github.com/example/platform.git', git_ref: 'main', submitted_at: created, created_at: created, store: 'ACTIVE', status: 'READY', next_route: { jobType: 'recipe', taskType: 'input:collect_user_input' }, parent: { job_id: 'coordinator', tenant_id: 'goal-layout' }, execution: { status: 'published', source: 'metadata', published: true, demand: { memory: '4Gi' } } };
  const second = { ...first, job_id: 'second', store: 'ARCHIVED', status: 'COMPLETED', completion_status: 'failed_app', completion_detail: 'sequence node 0 failed: sequence node 2 failed: The deployment check failed' };
  const solo = { ...first, job_id: 'solo', goal_id: 'solo-goal', prompt: 'Wait for deployment approval', status: 'PENDING_JOBS', next_route: undefined, parent: undefined, blocking_wait: { waitId: 'wait', state: 'PENDING', dependencies: [{ target: { goalId: 'dependency' }, condition: 'success', state: 'PENDING' }, { target: { jobId: 'job-dependency' }, condition: 'complete', state: 'PENDING' }] } };
  let reads = 0;
  await page.route('**/api/projects/goal-layout/jobs?*', route => {
    reads++;
    const goal = new URL(route.request().url()).searchParams.get('goal');
    return route.fulfill({ json: { jobs: goal === 'shared-goal' ? [first, second] : [first, second, solo] } });
  });
  await page.goto('/project/goal-layout/goals');
  await expect(page.getByRole('link', { name: first.prompt, exact: true })).toHaveCount(1);
  await expect(page.getByRole('link', { name: 'Wait for deployment approval', exact: true })).toBeVisible();
  const row = page.getByRole('link', { name: first.prompt, exact: true }).locator('xpath=ancestor::tr');
  await expect(row.getByRole('link', { name: 'Open input/review' })).toHaveAttribute('href', '/project/goal-layout/inputs/first');
  await expect(row.getByRole('link', { name: 'Parent' })).toHaveAttribute('href', '/project/goal-layout/jobs/coordinator/story');
  await expect(row.getByRole('link', { name: 'platform' })).toHaveAttribute('href', first.repo);
  await expect(page.getByRole('link', { name: 'Goal 1' })).toHaveAttribute('href', '/project/goal-layout/goals/dependency');
  await expect(page.getByRole('link', { name: 'Execution 2' })).toHaveAttribute('href', '/project/goal-layout/jobs/job-dependency/story');
  const initialReads = reads;
  await row.getByRole('button', { name: 'Expand row', exact: true }).click();
  await page.getByText('Execution requirements', { exact: true }).click();
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toContainText('4Gi');
  expect(reads).toBe(initialReads);
  const attempts = page.getByRole('region', { name: 'Attempts', exact: true }).getByRole('table');
  await expect(attempts).toHaveCount(1);
  await expect(attempts.getByText(second.completion_detail, { exact: true })).toBeVisible();
  await expect(page.getByText('shared-goal', { exact: true })).toHaveCount(0);
  await page.screenshot({ path: '/tmp/cortex-goals-list.png', fullPage: true });
  await row.getByRole('link', { name: 'Goal overview' }).click();
  await expect(page.getByRole('heading', { name: 'Goal overview', exact: true })).toBeVisible();
  await expect(page.getByRole('checkbox')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Expand row' })).toHaveCount(0);
  await expect(page.getByText('Execution requirements', { exact: true })).toBeVisible();
  await page.screenshot({ path: '/tmp/cortex-goal-overview.png', fullPage: true });
  await page.getByRole('region', { name: 'Attempts', exact: true }).getByRole('button', { name: 'Details', exact: true }).nth(1).click();
  const details = page.locator('.goal-attempt-detail');
  await expect(details.locator('.ant-alert').getByText(second.completion_detail, { exact: true })).toBeVisible();
  await page.getByText('Technical details', { exact: true }).click();
  await expect(details.getByText(second.completion_detail, { exact: true }).last()).toBeVisible();
  const inspectedURL = page.url();
  await page.reload();
  await expect(details.getByText(second.completion_detail, { exact: true }).last()).toBeVisible();
  await page.getByRole('link', { name: 'Back to Goals', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Attempts', exact: true })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(inspectedURL);
  await expect(details.getByText(second.completion_detail, { exact: true }).last()).toBeVisible();
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
    return route.fulfill({ json: { jobs: [{ tenant_id: 'filter-test', job_id, prompt: job_id, goal_id: job_id, recipe: 'build', job_type: 'recipe', status: cancelled ? 'CANCELLED' : 'READY', store: cancelled ? 'ARCHIVED' : 'ACTIVE' }], next_page_token: cancelled ? undefined : 'next-page' } });
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

for (const cellName of [undefined, '', '  \n ', 'self', 'platform']) {
  test(`shows explicit or unknown cell in Goals and attempts: ${JSON.stringify(cellName)}`, async ({ page, context, baseURL }) => {
    await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
    let detailReads = 0;
    page.on('request', request => { if (/\/jobs\/[^/?]+$/.test(new URL(request.url()).pathname)) detailReads++; });
    await page.route('**/api/projects/cell-label/jobs?*', route => route.fulfill({ json: { jobs: ['first', 'second'].map(job_id => ({
      tenant_id: 'cell-label', job_id, goal_id: 'goal', prompt: 'Cell identity goal', cell_name: cellName,
      status: 'READY', store: 'ACTIVE', recipe: 'build', job_type: 'recipe', created_at: '2026-10-25T10:00:00Z',
    })) } }));
    await page.goto('/project/cell-label/goals');
    const label = cellName?.trim() ? cellName : 'Unknown cell';
    const row = page.getByRole('link', { name: 'Cell identity goal', exact: true }).locator('xpath=ancestor::tr');
    await expect(row.getByText(label, { exact: true })).toBeVisible();
    await row.getByRole('link', { name: 'Goal overview' }).click();
    await expect(page.getByRole('region', { name: 'Attempts' }).getByText(label, { exact: true })).toHaveCount(2);
    expect(detailReads).toBe(0);
  });
}
