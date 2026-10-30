import { test, expect } from '@playwright/test';
import { setCellFilter, setStatusFilter } from './helpers/goalList';

test('renders 200 compact rows, expands a full message, and follows empty page cursors without detail requests', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const completion = '  <script>literal text</script> 雪\n' + 'The stored failure message keeps its whitespace.\n'.repeat(12) + '\tfinal line\n';
  const jobs = Array.from({ length: 200 }, (_, index) => ({ tenant_id: 'restored', job_id: `job-${index}`,
    status: index ? 'READY' : 'COMPLETED', store: index ? 'ACTIVE' : 'ARCHIVED',
    completion_status: index ? undefined : 'future_outcome', completion_detail: index ? undefined : completion,
    client_payload_revision: 5, blocking_wait_summary: { waitId: `wait-${index}` },
  }));
  const listReads: URLSearchParams[] = [];
  const detailReads: string[] = [];
  await page.route('**/api/projects/restored/jobs?*', route => {
    const query = new URL(route.request().url()).searchParams;
    listReads.push(query);
    return route.fulfill({ json: query.get('pageToken') === 'empty' ? { jobs: [], next_page_token: 'last' }
      : query.get('pageToken') === 'last' ? { jobs: [] } : { jobs, next_page_token: 'empty' } });
  });
  page.on('request', request => { if (/\/projects\/restored\/jobs\/[^/?]+$/.test(new URL(request.url()).pathname)) detailReads.push(request.url()); });
  await page.goto('/project/restored/jobs');
  await expect(page.getByRole('checkbox', { name: 'Select job job-199', exact: true })).toBeAttached();
  await expect(page.getByRole('checkbox', { name: /^Select job / })).toHaveCount(200);
  const row = page.getByRole('link', { name: 'Job job-0: Finished (future_outcome)', exact: true }).locator('xpath=ancestor::tr');
  await row.getByRole('button', { name: 'Expand', exact: true }).click();
  const message = row.locator('.job-completion');
  await expect(message).toContainText('final line');
  expect(await message.evaluate(element => Array.from(element.childNodes).filter(node => node.nodeType === Node.TEXT_NODE).map(node => node.textContent).join(''))).toBe(completion);
  await expect(message).toHaveCSS('white-space', 'pre-wrap');
  await expect(message.locator('script')).toHaveCount(0);
  expect(listReads).toHaveLength(1);
  expect(detailReads).toEqual([]);
  await page.getByRole('button', { name: 'Next jobs', exact: true }).click();
  await expect(page.getByRole('checkbox', { name: /^Select job / })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Next jobs', exact: true })).toBeEnabled();
  expect(listReads.map(query => query.get('pageToken'))).toEqual([null, 'empty']);
  await page.getByRole('button', { name: 'Next jobs', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Next jobs', exact: true })).toHaveCount(0);
  expect(listReads.map(query => query.get('pageToken'))).toEqual([null, 'empty', 'last']);
  expect(detailReads).toEqual([]);
});

test('renders a single compact row or grouped attempts with metadata and waiting links', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const created = '2026-10-25T10:00:00Z';
  const first = { tenant_id: 'goal-layout', job_id: 'first', goal_id: 'shared-goal', prompt: 'Build a reliable release dashboard', recipe: 'build', job_type: 'recipe', cell_name: 'platform', repo: 'https://github.com/example/platform.git', git_ref: 'main', submitted_at: created, created_at: created, store: 'ACTIVE', status: 'READY', next_route: { jobType: 'recipe', taskType: 'input:collect_user_input' }, parent: { job_id: 'coordinator', tenant_id: 'goal-layout' }, execution: { status: 'specified', source: 'yield', published: true, demand: { effective: { memory: '4Gi' } } } };
  const second = { ...first, job_id: 'second', store: 'ARCHIVED', status: 'COMPLETED', completion_status: 'failed_app', completion_detail: 'sequence node 0 failed: sequence node 2 failed: The deployment check failed' };
  const solo = { ...first, job_id: 'solo', goal_id: 'solo-goal', prompt: 'Wait for deployment approval', status: 'PENDING_JOBS', next_route: undefined, parent: undefined, blocking_wait: { waitId: 'wait', state: 'PENDING', dependencies: [{ target: { goalId: 'dependency' }, condition: 'success', state: 'PENDING' }, { target: { jobId: 'job-dependency' }, condition: 'complete', state: 'PENDING' }] } };
  const compact = (job: any) => {
    const { blocking_wait, ...summary } = job;
    return { ...summary, client_payload_revision: 8, ...(blocking_wait ? { blocking_wait_summary: { waitId: blocking_wait.waitId } } : {}) };
  };
  const detailsRead: string[] = [];
  await page.route('**/api/projects/goal-layout/jobs/*', route => {
    const id = new URL(route.request().url()).pathname.split('/').pop()!;
    detailsRead.push(id);
    return route.fulfill({ json: [first, second, solo].find(job => job.job_id === id) });
  });
  let reads = 0;
  await page.route('**/api/projects/goal-layout/jobs?*', route => {
    reads++;
    const goal = new URL(route.request().url()).searchParams.get('goal');
    return route.fulfill({ json: { jobs: goal === 'shared-goal' ? [first, second].map(compact) : [first, second, solo].map(compact) } });
  });
  await page.goto('/project/goal-layout/goals');
  await expect(page.getByRole('link', { name: first.prompt, exact: true })).toHaveCount(1);
  await expect(page.getByRole('link', { name: 'Wait for deployment approval', exact: true })).toBeVisible();
  const row = page.getByRole('link', { name: first.prompt, exact: true }).locator('xpath=ancestor::tr');
  await expect(row.getByRole('link', { name: 'Open input/review' })).toHaveAttribute('href', '/project/goal-layout/reviews/first');
  await expect(row.getByRole('link', { name: 'Parent' })).toHaveAttribute('href', '/project/goal-layout/jobs/coordinator/story');
  await expect(row.getByRole('link', { name: 'platform' })).toHaveAttribute('href', first.repo);
  await expect(page.getByText('Waiting for dependencies')).toBeVisible();
  expect(detailsRead).toEqual([]);
  const initialReads = reads;
  await row.getByRole('button', { name: 'Expand row', exact: true }).click();
  await page.getByText('Execution requirements', { exact: true }).click();
  await expect(page.getByText('Execution requirements', { exact: true }).locator('..')).toContainText('4Gi');
  expect(reads).toBe(initialReads);
  expect(detailsRead).toEqual(['first']);
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
    const cancelled = query.get('displayStatus') === 'CANCELLED';
    const job_id = cancelled ? 'cancelled-on-another-page' : query.has('pageToken') ? 'later-ready' : 'first-ready';
    return route.fulfill({ json: { jobs: [{ tenant_id: 'filter-test', job_id, prompt: job_id, goal_id: job_id, recipe: 'build', job_type: 'recipe', status: cancelled ? 'CANCELLED' : 'READY', store: cancelled ? 'ARCHIVED' : 'ACTIVE' }], next_page_token: cancelled ? undefined : 'next-page' } });
  });
  await page.goto('/project/filter-test/goals?status=READY&pageToken=later-page');
  await expect(page.getByRole('link', { name: 'later-ready', exact: true })).toBeVisible();
  await setStatusFilter(page, ['Cancelled']);
  await expect(page.getByRole('link', { name: 'cancelled-on-another-page', exact: true })).toBeVisible();
  await expect.poll(() => queries.at(-1)?.getAll('displayStatus')).toEqual(['CANCELLED']);
  expect(queries.at(-1)?.has('pageToken')).toBe(false);
  expect(new URL(page.url()).searchParams.has('pageToken')).toBe(false);
  const filteredURL = page.url();
  await setStatusFilter(page, null);
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

test('filters by displayed outcomes and combines them with scheduling states', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const jobs = [
    { job_id: 'ready', status: 'READY', store: 'ACTIVE', completion_status: 'failed_app' },
    { job_id: 'succeeded', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success' },
    { job_id: 'failed', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_app' },
    { job_id: 'unavailable', status: 'COMPLETED', store: 'ARCHIVED' },
    { job_id: 'cancelling', status: 'ACTIVE', store: 'ACTIVE', cancel_requested: true },
  ];
  const statusByID: Record<string, string> = { ready: 'READY', succeeded: 'success', failed: 'failed_app', unavailable: 'OUTCOME_UNAVAILABLE', cancelling: 'CANCEL_REQUESTED' };
  const queries: URLSearchParams[] = [];
  await page.route('**/api/projects/outcome-filter/jobs?*', route => {
    const query = new URL(route.request().url()).searchParams;
    queries.push(query);
    const selected = query.getAll('displayStatus');
    return route.fulfill({ json: { jobs: jobs.filter(job => !selected.length || selected.includes(statusByID[job.job_id])) } });
  });
  await page.goto('/project/outcome-filter/goals?status=READY&pageToken=old-page');
  await expect(page.getByRole('link', { name: 'Job ready: Ready', exact: true })).toBeVisible();
  await setStatusFilter(page, ['Ready', 'Succeeded']);
  await expect(page.getByRole('link', { name: 'Job succeeded: Succeeded', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Job ready: Ready', exact: true })).toBeVisible();
  expect(queries.at(-1)?.getAll('displayStatus')).toEqual(['READY', 'success']);
  expect(queries.at(-1)?.has('pageToken')).toBe(false);
  expect(new URL(page.url()).searchParams.has('pageToken')).toBe(false);
  await setStatusFilter(page, ['Succeeded', 'Failed (application)']);
  await expect(page.getByRole('link', { name: 'Job failed: Failed (application)', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Job ready: Ready', exact: true })).toHaveCount(0);
  expect(queries.at(-1)?.getAll('displayStatus')).toEqual(['success', 'failed_app']);
  await page.reload();
  await expect(page.getByRole('link', { name: 'Job succeeded: Succeeded', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Job failed: Failed (application)', exact: true })).toBeVisible();
  await setStatusFilter(page, ['Finished (outcome unavailable)']);
  await expect(page.getByRole('link', { name: 'Job unavailable: Finished (outcome unavailable)', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Job succeeded: Succeeded', exact: true })).toHaveCount(0);
  await setStatusFilter(page, ['Cancellation requested']);
  await expect(page.getByRole('link', { name: 'Job cancelling: Cancellation requested', exact: true })).toBeVisible();
  await expect(page.getByRole('checkbox', { name: /^Select job / })).toHaveCount(1);
});

test('starts with every status checked and supports individual exclusions, none, and all', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const jobs = [
    { job_id: 'ready', status: 'READY', store: 'ACTIVE' },
    { job_id: 'success', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success' },
    { job_id: 'future', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'future_outcome' },
  ];
  const queries: URLSearchParams[] = [];
  await page.route('**/api/projects/status-menu/jobs?*', route => {
    const query = new URL(route.request().url()).searchParams;
    queries.push(query);
    const selected = query.getAll('displayStatus');
    return route.fulfill({ json: { jobs: jobs.filter(job => !selected.length || selected.includes(job.job_id === 'ready' ? 'READY' : job.job_id === 'future' ? 'completion:future_outcome' : 'success')) } });
  });
  await page.goto('/project/status-menu/goals');
  await expect(page.getByRole('link', { name: 'Job success: Succeeded', exact: true })).toBeVisible();
  // The column filter is keyboard accessible without triggering column sorting.
  const icon = page.getByRole('img', { name: 'Filter statuses', exact: true });
  await icon.focus();
  await icon.press('Enter');
  const dropdown = page.locator('.ant-table-filter-dropdown:visible');
  await expect(dropdown.getByRole('checkbox', { name: 'Select all', exact: true })).toBeChecked();
  await dropdown.getByText('Succeeded', { exact: true }).click();
  expect(queries).toHaveLength(1); // Nothing changes until Apply.
  await dropdown.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page.getByRole('link', { name: 'Job success: Succeeded', exact: true })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Job future: Finished (future_outcome)', exact: true })).toBeVisible();
  expect(queries.at(-1)?.getAll('displayStatus')).not.toContain('success');
  expect(new URL(page.url()).searchParams.has('sort')).toBe(false);
  const beforeNone = queries.length;
  await setStatusFilter(page, []);
  await expect(page.getByText('No statuses selected', { exact: true })).toBeVisible();
  expect(new URL(page.url()).searchParams.get('status')).toBe('none');
  expect(queries).toHaveLength(beforeNone);
  await page.reload();
  await expect(page.getByText('No statuses selected', { exact: true })).toBeVisible();
  expect(queries).toHaveLength(beforeNone);
  await setStatusFilter(page, null);
  await expect(page.getByRole('link', { name: 'Job success: Succeeded', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Job future: Finished (future_outcome)', exact: true })).toBeVisible();
  expect(queries.at(-1)?.getAll('status')).toEqual(['all']);
  expect(new URL(page.url()).searchParams.has('status')).toBe(false);
  await page.goBack();
  await expect(page.getByText('No statuses selected', { exact: true })).toBeVisible();
});

test('derives cells from returned jobs and combines local cell filtering with status and history', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const jobs = ['alpha', 'beta', 'gamma'].flatMap(cell_name => [
    { job_id: `${cell_name}-ready`, cell_name, status: 'READY', store: 'ACTIVE' },
    { job_id: `${cell_name}-success`, cell_name, status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success' },
  ]);
  const queries: URLSearchParams[] = [];
  let catalogReads = 0;
  await page.route('**/api/projects/cell-filter/cells', route => {
    catalogReads++;
    return route.fulfill({ status: 500, body: 'No local cell configuration' });
  });
  await page.route('**/api/projects/cell-filter/jobs?*', route => {
    const query = new URL(route.request().url()).searchParams;
    queries.push(query);
    const cells = query.getAll('cell');
    const statuses = query.getAll('displayStatus');
    return route.fulfill({ json: { jobs: jobs.filter(job => (!cells.length || cells.includes(job.cell_name))
      && (!statuses.length || statuses.includes(job.completion_status || job.status))) } });
  });
  await page.goto('/project/cell-filter/goals?pageToken=old-page');
  const selections = page.getByRole('checkbox', { name: /^Select job / });
  await expect(selections).toHaveCount(6);
  await selections.first().check();
  const icon = page.getByRole('img', { name: 'Filter cells', exact: true });
  await icon.focus();
  await icon.press('Enter');
  const dropdown = page.locator('.ant-table-filter-dropdown:visible');
  await expect(dropdown.getByRole('checkbox', { name: 'Select all', exact: true })).toBeChecked();
  await dropdown.getByText('gamma', { exact: true }).click();
  expect(queries).toHaveLength(1);
  await dropdown.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(selections).toHaveCount(4);
  expect(queries).toHaveLength(1); // Cell filtering keeps the loaded page and its cursor.
  expect(queries.at(-1)?.getAll('cell')).toEqual([]);
  expect(new URL(page.url()).searchParams.get('pageToken')).toBe('old-page');
  expect(new URL(page.url()).searchParams.has('sort')).toBe(false);
  await expect(page.getByText('0 selected', { exact: true })).toBeVisible();
  await setStatusFilter(page, ['Succeeded']);
  await expect(selections).toHaveCount(2);
  expect(queries.at(-1)?.getAll('cell')).toEqual([]);
  expect(queries.at(-1)?.getAll('displayStatus')).toEqual(['success']);
  expect(queries.at(-1)?.has('pageToken')).toBe(false);
  const filteredURL = page.url();
  await page.reload();
  await expect(selections).toHaveCount(2);
  const beforeNone = queries.length;
  await setCellFilter(page, []);
  await expect(page.getByText('No cells selected', { exact: true })).toBeVisible();
  expect(queries).toHaveLength(beforeNone);
  expect(new URL(page.url()).searchParams.get('cellSelection')).toBe('none');
  await page.reload();
  await expect(page.getByText('No cells selected', { exact: true })).toBeVisible();
  expect(queries).toHaveLength(beforeNone + 1); // Reload still loads choices while showing no rows.
  // Changing Status must not undo Cell's empty selection, even through an empty page.
  await setStatusFilter(page, []);
  await expect(page.getByText('No statuses selected', { exact: true })).toBeVisible();
  await setStatusFilter(page, null);
  await expect(page.getByText('No cells selected', { exact: true })).toBeVisible();
  await expect.poll(() => queries.length).toBe(beforeNone + 2);
  await setStatusFilter(page, ['Succeeded']);
  await setCellFilter(page, null);
  await expect(selections).toHaveCount(3);
  expect(queries.at(-1)?.getAll('cell')).toEqual([]);
  expect(queries.at(-1)?.getAll('displayStatus')).toEqual(['success']);
  await page.goBack();
  await expect(page.getByText('No cells selected', { exact: true })).toBeVisible();
  await page.goto(filteredURL);
  await expect(selections).toHaveCount(2);
  expect(catalogReads).toBe(0);
  await page.screenshot({ path: '/tmp/cortex-cell-filter.png', fullPage: true });
});

test('filters missing cell names and refreshes choices from each returned page', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const jobs = [undefined, '', ' \n ', 'self', 'platform'].map((cell_name, i) => ({
    job_id: `job-${i}`, cell_name, status: 'READY', store: 'ACTIVE',
  }));
  let reads = 0;
  await page.route('**/api/projects/unknown-cell/jobs?*', route => {
    reads++;
    return route.fulfill({ json: new URL(route.request().url()).searchParams.has('pageToken')
      ? { jobs: [{ job_id: 'next', cell_name: 'next-cell', status: 'READY', store: 'ACTIVE' }] }
      : { jobs, next_page_token: 'later' } });
  });
  await page.goto('/project/unknown-cell/goals');
  const selections = page.getByRole('checkbox', { name: /^Select job / });
  await expect(selections).toHaveCount(5);
  await setCellFilter(page, ['Unknown cell']);
  await expect(selections).toHaveCount(3);
  expect(new URL(page.url()).searchParams.getAll('cell')).toEqual(['']);
  expect(reads).toBe(1);
  await page.reload();
  await expect(selections).toHaveCount(3);
  expect(reads).toBe(2);
  await page.getByRole('button', { name: 'Next jobs', exact: true }).click();
  await expect.poll(() => reads).toBe(3);
  await expect(selections).toHaveCount(0);
  await page.getByRole('button', { name: 'Filter cells', exact: true }).click();
  const dropdown = page.locator('.ant-table-filter-dropdown:visible');
  await expect(dropdown.getByRole('checkbox', { name: 'Select Unknown cell', exact: true })).toBeChecked();
  await expect(dropdown.getByRole('checkbox', { name: 'Select next-cell', exact: true })).not.toBeChecked();
  await expect(dropdown.getByText('platform', { exact: true })).toHaveCount(0);
  await dropdown.getByRole('checkbox', { name: 'Select all', exact: true }).check();
  await dropdown.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(selections).toHaveCount(1);
  await expect(page.getByRole('link', { name: 'Job next: Ready', exact: true })).toBeVisible();
  expect(reads).toBe(3);
});

test('searches complete prompts and sorts locally while preserving the view through goal navigation', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
  const jobs = [
    { job_id: 'zeta', goal_id: 'zeta', title: 'Zeta task', prompt: 'Common task for Zeta', created_at: '2026-10-29T08:00:00Z', status: 'READY', store: 'ACTIVE' },
    { job_id: 'alpha', goal_id: 'alpha', title: 'Alpha task', prompt: `${'Common task with a long preamble. '.repeat(10)}Needle\n  beyond truncation`, created_at: '2026-10-29T09:00:00Z', status: 'READY', store: 'ACTIVE' },
    { job_id: 'alpha-old', goal_id: 'alpha', prompt: 'Historical requirement for this common task', created_at: '2026-10-28T09:00:00Z', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_app' },
    { job_id: 'middle', goal_id: 'middle', title: 'Middle task', prompt: 'Common task for Middle', created_at: '2026-10-29T10:00:00Z', status: 'READY', store: 'ACTIVE' },
  ];
  let reads = 0;
  await page.route('**/api/projects/local-search/jobs?*', route => {
    reads++;
    const goal = new URL(route.request().url()).searchParams.get('goal');
    return route.fulfill({ json: { jobs: goal ? jobs.filter(job => job.goal_id === goal) : jobs, next_page_token: goal ? undefined : 'later' } });
  });
  await page.route('**/api/projects/local-search/jobs/alpha', route => route.fulfill({ json: jobs[1] }));
  await page.goto('/project/local-search/goals');
  const names = page.locator('a.goal-prompt');
  await expect(names).toHaveText(['Zeta task', 'Alpha task', 'Middle task']);
  const search = page.getByRole('textbox', { name: 'Search goals' });
  const searchBox = await page.locator('.goal-search').boundingBox();
  const actionBox = await page.getByRole('button', { name: 'Delete selected', exact: true }).boundingBox();
  const toolbarBox = await page.locator('.goal-list-actions').boundingBox();
  expect(Math.abs(searchBox!.y + searchBox!.height / 2 - actionBox!.y - actionBox!.height / 2)).toBeLessThan(2);
  expect(Math.abs(searchBox!.x + searchBox!.width - toolbarBox!.x - toolbarBox!.width)).toBeLessThan(2);
  expect(searchBox!.x).toBeGreaterThan(actionBox!.x + actionBox!.width);
  await search.fill('NEEDLE beyond truncation');
  await expect(names).toHaveText(['Alpha task']);
  await expect(page.getByText('2 attempts on this page', { exact: true })).toBeVisible();
  await search.fill('historical requirement');
  await expect(names).toHaveText(['Alpha task']);
  await search.fill('missing text');
  await expect(page.getByText('No goals match this search on this page', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Next jobs', exact: true })).toBeVisible();
  await search.fill('common task');
  await expect(names).toHaveCount(3);
  const header = page.getByRole('columnheader', { name: 'Submitted', exact: true });
  await header.click();
  await expect(header).toHaveAttribute('aria-sort', 'ascending');
  await expect(names).toHaveText(['Zeta task', 'Alpha task', 'Middle task']);
  await header.click();
  await expect(header).toHaveAttribute('aria-sort', 'descending');
  await expect(names).toHaveText(['Middle task', 'Alpha task', 'Zeta task']);
  expect(reads).toBe(1);
  const listURL = page.url();
  const row = page.getByRole('link', { name: 'Alpha task', exact: true }).locator('xpath=ancestor::tr');
  await row.getByRole('link', { name: 'Goal overview', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Goal overview', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Back to Goals', exact: true }).click();
  await expect(page).toHaveURL(listURL);
  await expect(search).toHaveValue('common task');
  await expect(names).toHaveText(['Middle task', 'Alpha task', 'Zeta task']);
  await page.reload();
  await expect(search).toHaveValue('common task');
  await expect(names).toHaveText(['Middle task', 'Alpha task', 'Zeta task']);
  await expect(header).toHaveAttribute('aria-sort', 'descending');
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
    await page.route('**/api/projects/cell-label/jobs/first', route => route.fulfill({ json: { job_id: 'first', cell_name: cellName, status: 'READY', store: 'ACTIVE' } }));
    await page.goto('/project/cell-label/goals');
    const label = cellName?.trim() ? cellName : 'Unknown cell';
    const row = page.getByRole('link', { name: 'Cell identity goal', exact: true }).locator('xpath=ancestor::tr');
    await expect(row.getByText(label, { exact: true })).toBeVisible();
    await row.getByRole('link', { name: 'Goal overview' }).click();
    await expect(page.getByRole('region', { name: 'Attempts' }).getByText(label, { exact: true })).toHaveCount(2);
    await expect.poll(() => detailReads).toBe(1);
  });
}
