import { test, expect, type Page } from '@playwright/test';
import { authorizeV2, reviewV2BrowserFixture, v2Selected } from './helpers/reviewV2';
import { cellFixture } from './helpers/cells';
import { openTenantSwitcher } from './helpers/inbox';
import { jobStatusValue } from '../src/utils/jobStatus';

async function inboxFixture(page: Page, withRequirements = false) {
  const review = await reviewV2BrowserFixture(page, true);
  const cells = Array.from({ length: 40 }, (_, index) => cellFixture(index === 0 ? 'platform' : `cell-${String(index).padStart(2, '0')}`, 'v2'));
  const jobs = Array.from({ length: 52 }, (_, index) => {
    const cell = cells[index < 8 ? 0 : (index - 8) % 39 + 1];
    return { tenant_id: 'v2', job_id: index === 0 ? 'one' : `job-${index}`, goal_id: 'same-goal', cell_id: cell.id, cell_name: cell.name, repo: cell.repo, execution: withRequirements && index === 0 ? { status: 'specified', source: 'yield', published: true, demand: { effective: { base: 'linux', resources: { cpu: '4', memory: '8Gi' }, capabilities: ['browser'] } } } : undefined, title: index === 0 ? 'Make reviews easier to read' : `Improve ${cell.name} workflow`, prompt: 'Keep the current request in focus and the conversation close by.', status: index < 24 ? 'READY' : 'COMPLETED', store: index < 24 ? 'ACTIVE' : 'ARCHIVED', archived_at: index < 24 ? undefined : '2026-11-21T12:00:00Z', created_at: '2026-11-20T10:00:00Z', inboxArchived: index >= 44 };
  });
  const state = { jobs, submissions: [] as any[], jobReads: [] as URLSearchParams[], withoutReviews: new Set<string>() };
  await page.route(/\/jobs\/one(?:\?.*)?$/, route => route.fulfill({ json: jobs[0] }));
  await page.route(/\/api\/projects\/v2\/cells(?:\/[^/?]+)?(?:\?.*)?$/, route => {
    const selected = new URL(route.request().url()).pathname.split('/')[5];
    return route.fulfill({ json: selected ? cells.find(cell => cell.id === selected) : { cells } });
  });
  await page.route(/\/api\/projects\/v2\/jobs\?/, route => {
    if (route.request().method() !== 'GET') return route.fallback();
    const params = new URL(route.request().url()).searchParams;
    state.jobReads.push(params);
    const inclusive = params.get('includeHidden') === 'true';
    const names = params.getAll('cellNames');
    const statuses = params.getAll('status'), outcomes = params.getAll('displayStatus');
    return route.fulfill({ json: { jobs: state.jobs.filter(job => (inclusive || !job.inboxArchived)
      && (!names.length || names.includes(job.cell_name))
      && (!statuses.length || statuses.includes('all') || statuses.includes(job.status))
      && (!outcomes.length || outcomes.includes(jobStatusValue(job)))) } });
  });
  await page.route('**/api/projects/v2/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    const body = route.request().postDataJSON(); state.submissions.push(body);
    return route.fulfill({ json: { tenant_id: 'v2', job_id: body.job_id, goal_id: 'opaque', cell_id: 'cell-platform', cell_name: 'platform', repo: cells[0].repo } });
  });
  await page.route('**/api/projects/v2/user-inputs/pending', route => route.fulfill({ json: state.jobs.filter(job => !job.inboxArchived && !state.withoutReviews.has(job.job_id) && job.status === 'READY' && (job.job_id !== 'one' || review.state.pending)).map(job => ({ ...job, id: job.job_id, client_payload_revision: 1, request_id: job.job_id === 'one' ? review.state.native : `request-${job.job_id}`, requested_at: withRequirements && job.job_id === 'one' ? '2026-11-22T14:00:00Z' : '2026-11-21T14:00:00Z' })) }));
  await page.route('**/api/projects/v2/jobs/one/story', route => route.fulfill({ json: { job_id: 'one', status: 'running', root: null } }));
  await page.route('**/api/projects/v2/jobs/one/input-artifacts', route => route.fulfill({ json: { artifacts: [] } }));
  await page.route('**/api/projects/v2/jobs/one/workspace-diff*', route => route.fulfill({ json: { status: 'absent', comparisons: [] } }));
  await page.route('**/api/projects/v2/jobs/one/conversation?*', route => route.fulfill({ json: {
    tenant_id: 'v2', job_id: 'one', snapshot_token: 'snapshot', snapshot_revision: 1, completeness: 'complete',
    activity: { availability: 'available', value: jobs[0] },
    entries: [1, 2, 3, 4].map(index => ({ entry_id: `previous-${index}`, request_id: `previous-native-${index}`, kind: 'ordinary', title: { present: true, raw: `Earlier update ${index}` }, summary: { present: false }, requested_at: { present: false }, state: 'answered', locator: {}, form_fields: { question: 'Review the inbox layout' }, accepted_response: { present: true, raw: 'Keep the current review in focus.' }, actor: { provenance: 'reported_receipt', receipt: {}, legacy_user_id: { present: false } } })).concat([{
      entry_id: 'entry', request_id: review.state.native, kind: 'structured', title: { present: true, raw: review.state.request.title }, summary: { present: false }, requested_at: { present: false }, state: review.state.pending ? 'pending' : 'answered', locator: { generation_ordinal: 1 }, form_fields: { request_id: review.state.native, presentation: { type: 'review/v2' }, request: review.state.request, request_schema: { type: 'object' }, response_schema: { type: 'object' } }, accepted_response: { present: !!review.state.output, raw: review.state.output }, actor: { provenance: 'reported_receipt', receipt: review.state.output?.receipt, legacy_user_id: { present: false } },
      ...(review.state.output && { review_attachments: { request: [], response: review.state.output.response.attachments.map(ref => ({ artifact_id: ref.name, ref, associations: [], availability: 'retained' })) } }),
    }] as any),
  } }));
  return { ...review, ...state };
}

const pageErrors = new Map<Page, Error[]>();
const assistantRequests = new Map<Page, string[]>();
test.beforeEach(async ({ page, baseURL }) => {
  pageErrors.set(page, []);
  assistantRequests.set(page, []);
  page.on('request', request => {
    if (/\/assistant(?:\/|$)/.test(new URL(request.url()).pathname)) assistantRequests.get(page)!.push(request.url());
  });
  page.on('pageerror', error => pageErrors.get(page)!.push(error));
  await authorizeV2(page, baseURL!);
});
test.afterEach(async ({ page }) => {
  // The inbox must retain upstream's Assistant removal across shell, story,
  // embedded review and Compose navigation, without a capabilities probe.
  expect(assistantRequests.get(page), 'Removed Assistant API requests').toEqual([]);
  expect(pageErrors.get(page)).toEqual([]);
  assistantRequests.delete(page); pageErrors.delete(page);
});

test('flat inbox, pending badges, forty cells, Archive and browser-local read state', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await inboxFixture(page);
  await page.goto('/project/v2/jobs');
  await expect(page.getByRole('list', { name: 'Jobs' }).getByRole('listitem')).toHaveCount(44);
  const all = page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'All', exact: true });
  const waiting = page.getByRole('link', { name: 'Reviews', exact: true });
  await expect(waiting.getByLabel('24 pending responses')).toBeVisible();
  await waiting.click();
  await expect(page.getByRole('heading', { name: 'Reviews', exact: true })).toBeVisible();
  await expect(page.getByRole('list', { name: 'Jobs' }).getByRole('listitem')).toHaveCount(24);
  await all.click();
  await expect(page.locator('.inbox-job-row.waiting')).toHaveCount(24);
  await expect(page.getByRole('checkbox', { name: 'Select job job-44', exact: true })).toHaveCount(0);
  await expect(page.locator('.inbox-cell-scroll > a')).toHaveCount(8);
  await page.getByRole('button', { name: 'Search all cells' }).click();
  await page.getByRole('textbox', { name: 'Search cells' }).fill('cell-39');
  await page.getByRole('link', { name: 'cell-39', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'cell-39', exact: true })).toBeVisible();
  await expect(page.getByRole('checkbox', { name: 'Select job job-46', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Add cell' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'All Cells', exact: true })).toHaveAttribute('href','/project/v2/cells');
  await all.click();
  await page.getByRole('textbox', { name: 'Search cells' }).fill('');
  await page.screenshot({ path: test.info().outputPath('inbox-desktop.png') });
  const row = page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true });
  await row.locator('time').hover();
  await expect(page.getByRole('tooltip')).toContainText('2026');
  await expect(row.getByLabel('Unread')).toBeVisible();
  await row.click(); await expect(page.getByRole('button', { name: 'Collapse navigation' })).toBeVisible();
  await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
  await expect(row.getByLabel('Unread')).toHaveCount(0);
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'Archive', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Jobs' }).getByRole('listitem')).toHaveCount(8);
  await expect(page.locator('.inbox-job-row.unread')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Archive', exact: true }).locator('.inbox-badge')).toHaveCount(0);
});

test('Active, Waiting and Failed filter visible jobs by scheduling and review state', async ({ page }) => {
  const fixture = await inboxFixture(page);
  Object.assign(fixture.jobs[1], { status: 'PENDING_JOBS', title: 'Visible dependency' });
  Object.assign(fixture.jobs[2], { status: 'AWAITING_FUTURE', title: 'Scheduled task' });
  Object.assign(fixture.jobs[3], { status: 'ACTIVE', title: 'Running task' });
  Object.assign(fixture.jobs[4], { title: 'Ready task' });
  fixture.withoutReviews.add('job-4');
  Object.assign(fixture.jobs[45], { status: 'PENDING_JOBS', store: 'ACTIVE', archived_at: undefined, title: 'Archived dependency' });
  Object.assign(fixture.jobs[24], { completion_status: 'failed_app', title: 'Visible failure' });
  Object.assign(fixture.jobs[25], { completion_status: 'failed_timeout', title: 'Visible timeout' });
  Object.assign(fixture.jobs[46], { completion_status: 'failed_system', title: 'Archived failure' });
  await page.goto('/project/v2/jobs');
  const navigation = page.getByRole('navigation', { name: 'Inboxes' });
  const list = page.getByRole('list', { name: 'Jobs' });
  await navigation.getByRole('link', { name: 'Active', exact: true }).click();
  await expect(list.getByRole('listitem')).toHaveCount(2);
  await expect(list).toContainText('Running task');
  await expect(list).toContainText('Ready task');
  expect(fixture.jobReads.at(-1)?.getAll('status')).toEqual(['ACTIVE','READY']);
  await navigation.getByRole('link', { name: 'Waiting', exact: true }).click();
  await expect(list.getByRole('listitem')).toHaveCount(2);
  await expect(list).toContainText('Visible dependency');
  await expect(list).toContainText('Scheduled task');
  expect(fixture.jobReads.at(-1)?.getAll('status')).toEqual(['PENDING_JOBS','AWAITING_FUTURE']);
  expect(fixture.jobReads.at(-1)?.get('includeHidden')).toBe('false');
  await navigation.getByRole('link', { name: 'Failed', exact: true }).click();
  await expect(list.getByRole('listitem')).toHaveCount(2);
  await expect(list).toContainText('Visible failure');
  await expect(list).toContainText('Visible timeout');
  expect(fixture.jobReads.at(-1)?.getAll('displayStatus')).toEqual(['failed_app', 'failed_system', 'failed_timeout']);
  expect(fixture.jobReads.at(-1)?.get('includeHidden')).toBe('false');
});

test('All and cell inboxes query one visible page and Archive reads on demand', async ({ page }) => {
  const fixture = await inboxFixture(page);
  await page.route(/\/api\/projects\/v2\/jobs\?/, route => {
    const params = new URL(route.request().url()).searchParams;
    if (params.has('cellNames') || params.get('includeHidden') !== 'false') return route.fallback();
    fixture.jobReads.push(params);
    const visible = fixture.jobs.filter(job => !job.inboxArchived);
    return route.fulfill({ json: params.has('pageToken') ? { jobs: visible.slice(2) } : { jobs: visible.slice(0, 2), next_page_token: 'next' } });
  });
  await page.goto('/project/v2/jobs');
  const rows = page.getByRole('list', { name: 'Jobs' }).getByRole('listitem');
  const nav = page.getByRole('navigation', { name: 'Inboxes' });
  await expect(rows).toHaveCount(2);
  expect(fixture.jobReads.map(params => params.get('includeHidden'))).toEqual(['false']);
  await page.getByRole('button', { name: 'Load more jobs' }).click();
  await expect(rows).toHaveCount(44);
  expect(fixture.jobReads[1].get('pageToken')).toBe('next');
  await nav.getByRole('link', { name: 'platform', exact: true }).click();
  await expect(rows).toHaveCount(8);
  expect(fixture.jobReads.map(params => params.getAll('cellNames'))).toEqual([[], [], ['platform']]);
  await nav.getByRole('link', { name: 'Reviews', exact: true }).click();
  await expect(rows).toHaveCount(24);
  expect(fixture.jobReads).toHaveLength(3);
  await nav.getByRole('link', { name: 'Archive', exact: true }).click();
  await expect(rows).toHaveCount(8);
  expect(fixture.jobReads.some(params => params.get('includeHidden') === 'true')).toBe(true);

  await nav.getByRole('link', { name: 'All', exact: true }).click();
  await expect(rows).toHaveCount(2);
  let release!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; });
  await page.route(/\/api\/projects\/v2\/jobs\?.*cellNames=platform/, async route => {
    await held;
    await route.fulfill({ json: { jobs: [] } }).catch(() => {});
  });
  try {
    const requested = page.waitForRequest(request => new URL(request.url()).searchParams.get('cellNames') === 'platform');
    await nav.getByRole('link', { name: 'platform', exact: true }).click();
    await requested;
    await nav.getByRole('link', { name: 'All', exact: true }).click();
    await expect(rows).toHaveCount(2);
  } finally { release(); }
});

test('the header aligns search with the thread list and preserves sorting when a thread opens', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await inboxFixture(page, true);
  await page.goto('/project/v2/jobs');
  const header = page.getByRole('banner');
  const search = header.getByRole('searchbox', { name: 'Search threads' });
  const sidebar = page.locator('.desktop-navigation');
  const alignment = async () => {
    const sidebarEdge = await sidebar.evaluate(element => element.getBoundingClientRect().right);
    const searchEdge = await search.evaluate(element => element.closest('.inbox-thread-search')!.getBoundingClientRect().left);
    return Math.abs(sidebarEdge - searchEdge);
  };
  await expect.poll(alignment).toBeLessThan(1);
  const x = (label: string) => header.getByLabel(label, { exact: true }).evaluate(element => element.getBoundingClientRect().left);
  expect(await x('Runners')).toBeLessThan(await x('Settings'));
  expect(await x('Settings')).toBeLessThan(await x('User account'));
  await header.getByRole('button', { name: 'Collapse navigation' }).click();
  await expect.poll(alignment).toBeLessThan(1);
  const rows = page.getByRole('list', { name: 'Jobs' }).getByRole('listitem');
  await expect(rows.first()).toContainText('Make reviews easier to read');
  await expect(rows.first()).toContainText('Needs review');
  for (const [mode, first] of [['Date (oldest first)', 'Finished'], ['Cell', 'cell-01'], ['Status', 'Finished'], ['Date (newest first)', 'Make reviews easier to read']]) {
    await page.getByRole('button', { name: 'Sort threads' }).click();
    await page.getByRole('menuitem', { name: mode, exact: true }).click();
    await expect(rows.first()).toContainText(first);
  }
  await page.getByRole('button', { name: 'Sort threads' }).click();
  await page.getByRole('menuitem', { name: 'Subject', exact: true }).click();
  await search.fill('Make reviews');
  await expect(rows).toHaveCount(1);
  await rows.first().getByRole('link', { name: 'platform: Make reviews easier to read' }).click();
  await expect(page.getByLabel('Job status')).toHaveText('Needs review');
  await page.getByRole('link', { name: 'Back to inbox' }).click();
  await expect(search).toHaveValue('Make reviews');
  expect(new URL(page.url()).searchParams.get('sort')).toBe('subject');
  await rows.first().getByRole('link', { name: 'platform: Make reviews easier to read' }).click();
  await search.fill('workflow');
  await expect(page).toHaveURL(/\/one\/story/);
  await search.press('Enter');
  await expect(rows).toHaveCount(43);
});

test.describe('compact inbox dates and selection', () => {
  test.use({ locale: 'en-US', timezoneId: 'UTC' });

  for (const width of [320, 1440]) test(`selection keeps job rows in place at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.clock.setFixedTime(new Date('2026-11-22T16:30:00Z'));
    await inboxFixture(page, true);
    await page.goto('/project/v2/jobs');
    const list = page.getByRole('list', { name: 'Jobs' });
    const rows = list.getByRole('listitem');
    await expect(rows).toHaveCount(44);
    const job = page.getByRole('link', { name: 'platform: Make reviews easier to read' });
    const actions = rows.filter({ has: job }).locator('.inbox-row-actions');
    if (width < 768) {
      await expect(list.getByRole('button', { name: /^(Copy job ID|Archive job) / })).toHaveCount(0);
    } else {
      await expect(actions.getByRole('button', { name: 'Copy job ID one', exact: true })).toBeVisible();
      await expect(actions.getByRole('button', { name: 'Archive job one', exact: true }).locator('svg')).toBeVisible();
    }
    await expect(job.locator('time')).toHaveText('2:00 PM');
    await job.click();
    await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
    await expect(job.getByLabel('Unread')).toHaveCount(0);
    await page.clock.setFixedTime(new Date('2026-11-23T00:01:00Z'));
    await page.getByRole('button', { name: 'Refresh inbox' }).click();
    await expect(job.locator('time')).toHaveText('Nov 22');
    const getRowTops = () => rows.evaluateAll(elements => elements.map(row => row.getBoundingClientRect().top));
    const initial = await getRowTops();
    const checkbox = page.getByRole('checkbox', { name: 'Select job one', exact: true });
    const all = page.getByRole('checkbox', { name: 'Select all jobs' });
    const options = page.getByRole('button', { name: 'Select jobs by' });
    const select = async (mode: string) => {
      await options.click();
      const item = page.getByRole('menuitem', { name: mode, exact: true });
      await expect(item).toBeInViewport();
      expect(await getRowTops(), `Opening the selection menu for ${mode}`).toEqual(initial);
      await item.click();
      await expect(item).toBeHidden();
      expect(await getRowTops(), `Choosing ${mode}`).toEqual(initial);
    };
    await checkbox.check();
    await expect(page.getByRole('button', { name: 'Archive selected' })).toBeVisible();
    await expect(all).toHaveJSProperty('indeterminate', true);
    expect(await getRowTops()).toEqual(initial);
    await checkbox.uncheck();
    expect(await getRowTops()).toEqual(initial);
    await select('All');
    await expect(all).toBeChecked();
    await select('None');
    await expect(checkbox).not.toBeChecked();
    await select('Finished');
    await expect(list.getByRole('checkbox', { checked: true })).toHaveCount(20);
    await expect(checkbox).not.toBeChecked();
    await select('Unfinished');
    await expect(list.getByRole('checkbox', { checked: true })).toHaveCount(24);
    await expect(checkbox).toBeChecked();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
  });
});

test('embedded review preserves draft through sidebar expansion and Settings, and sends the native response', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const fixture = await inboxFixture(page);
  await page.route('**/api/projects/v2/admin*', route => route.fulfill({ json: { revision: '1', hooks: [], configs: [], git: [], secretReferences: [], secretWrites: false } }));
  await page.goto('/project/v2/jobs/one/story');
  await expect(page.getByRole('button', { name: 'Collapse navigation' })).toBeVisible({ timeout: 45_000 });
  await expect(page.locator('.inbox-history-message[open] > summary').filter({ hasText: 'Make reviews easier to read' })).toBeVisible();
  await v2Selected(page, 0);
  const primary = page.locator('.embedded-review .review-primary-document');
  const heading = primary.getByRole('heading', { name: 'Viewer changes', exact: true });
  const gear = primary.getByRole('button', { name: 'Document options' });
  expect(Math.abs((await heading.boundingBox())!.y - (await gear.boundingBox())!.y)).toBeLessThan(8);
  const attachments = page.locator('.embedded-review').getByRole('navigation', { name: 'Review attachments' });
  const icons = await attachments.locator('svg').evaluateAll(elements => elements.map(icon => getComputedStyle(icon).color));
  expect(new Set(icons).size).toBe(4);
  const link = primary.getByRole('link', { name: 'Open checks' });
  // Exercise browsers that leave the thread panel focused on a pointer activation.
  await page.getByRole('tabpanel', { name: 'Thread', exact: true }).focus();
  await link.dispatchEvent('click');
  const attachment = page.getByRole('dialog', { name: 'Acceptance checks', exact: true });
  await attachment.getByRole('button', { name: 'Close', exact: true }).click();
  await expect(link).toBeFocused();
  const earlier = page.locator('.inbox-history-message').first();
  await expect(earlier.getByLabel('Accepted response', { exact: true })).toBeVisible();
  await earlier.locator('summary').click();
  await page.locator('.embedded-review').getByRole('heading', { name: 'Viewer changes', exact: true }).evaluate(element => element.scrollIntoView({ block: 'start' }));
  await expect(page.getByText('FINAL DOCUMENT CONTENT', { exact: true })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('inbox-review-reading.png') });
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  const editor = page.getByLabel('Question or requested change (required)', { exact: true });
  const initialHeight = (await editor.boundingBox())!.height;
  await editor.fill('One line of feedback.\n'.repeat(8));
  expect((await editor.boundingBox())!.height).toBeGreaterThan(initialHeight);
  expect(await editor.evaluate(el => el.scrollHeight <= el.clientHeight + 1)).toBe(true);
  await editor.fill('Keep this response while I inspect the cell list.');
  await page.getByRole('tab', { name: 'Execution', exact: true }).click();
  await page.getByRole('tab', { name: 'Thread', exact: true }).click();
  await expect(editor).toHaveValue('Keep this response while I inspect the cell list.');
  await page.getByRole('region', { name: 'Response composer' }).getByRole('button', { name: 'Back', exact: true }).click();
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '76px');
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath('inbox-review-desktop.png'), animations: 'disabled' });
  await page.getByRole('link', { name: 'Settings', exact: true }).click();
  await page.getByRole('link', { name: 'Drafts', exact: true }).click();
  await page.getByRole('link', { name: /Review response · Keep this response/ }).click();
  await expect(page).toHaveURL(/\/jobs\/one\/story\?/);
  await v2Selected(page, 0);
  await expect(editor).toHaveValue('Keep this response while I inspect the cell list.');
  await page.getByRole('tab', { name: 'Execution', exact: true }).click();
  await page.keyboard.press('Control+Enter');
  expect(fixture.state.submits).toHaveLength(0);
  await page.getByRole('tab', { name: 'Thread', exact: true }).click();
  await page.getByRole('button', { name: 'Document options', exact: true }).focus();
  await page.keyboard.press('Control+Enter');
  await expect.poll(() => fixture.state.submits.length).toBe(1);
  expect(fixture.state.submits[0].response.feedback).toBe('Keep this response while I inspect the cell list.');
});

test('opens a closed review inline and retains its attachment preview across thread tabs', async ({ page }) => {
  const fixture = await inboxFixture(page);
  fixture.state.pending = false;
  fixture.jobs[0].title = '';
  Object.assign(fixture.jobs[0], { metadata: { cortex_submitted_by: 'author@example.com' } });
  const reads: string[] = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/conversation') || path.endsWith('/conversation/entry')) reads.push(path);
  });
  await page.goto('/project/v2/jobs/one/story');
  await expect(page.getByRole('heading', { name: fixture.jobs[0].prompt, exact: true })).toBeVisible();
  await expect(page.locator('.inbox-original > summary')).toContainText('author@example.com');
  const latest = page.locator('.inbox-history-message[data-occurrence="entry"]');
  await expect(latest).toHaveAttribute('open', '');
  await expect(latest.locator('.thread-message-sender')).toHaveText('platform');
  await expect(latest.locator('.thread-message-subject')).toHaveText('Make reviews easier to read');
  await expect(latest).toHaveCSS('border-top-width', '0px');
  await expect(latest).toHaveCSS('border-bottom-width', '1px');
  const recorded = page.getByRole('region', { name: 'Recorded request', exact: true });
  await expect(recorded).toContainText(fixture.state.request.question);
  await expect(recorded.getByText('FINAL DOCUMENT CONTENT', { exact: true })).toBeVisible();
  const before = reads.length;
  await page.getByRole('tab', { name: 'Execution', exact: true }).click();
  await page.getByRole('tab', { name: 'Thread', exact: true }).click();
  await expect(recorded.getByText('FINAL DOCUMENT CONTENT', { exact: true })).toBeVisible();
  expect(reads).toHaveLength(before);
});

test('popped-out review replies share the dock and keep their draft through navigation', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const fixture = await inboxFixture(page);
  await page.route('**/api/projects/other/jobs?*', route => route.fulfill({ json: { jobs: [] } }));
  await page.route('**/api/projects/other/cells?*', route => route.fulfill({ json: { cells: [] } }));
  await page.route('**/api/projects/other/user-inputs/pending', route => route.fulfill({ json: [] }));
  await page.goto('/project/v2/jobs');
  const first = page.getByRole('region', { name: 'New message', exact: true });
  const docked = page.getByRole('region', { name: 'Popped-out review reply', exact: true });
  const feedback = 'Keep this question and its attachment.';
  const field = 'Question or requested change (required)';
  await page.getByRole('button', { name: 'New message', exact: true }).click();
  const firstBounds = (await first.boundingBox())!;
  await page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true }).click();
  await v2Selected(page, 0);
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  await page.getByLabel(field, { exact: true }).fill(feedback);
  await page.getByLabel('Attach notes', { exact: true }).setInputFiles({ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('Attached notes') });
  await expect(page.getByText('Draft', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Pop out reply', exact: true }).click();
  await expect(docked.getByLabel(field, { exact: true })).toHaveValue(feedback);
  await expect(docked.getByText(/notes\.txt/)).toBeVisible();
  expect((await first.boundingBox())!.x).toBe(firstBounds.x);
  const replyBounds = (await docked.boundingBox())!;
  expect(replyBounds.x + replyBounds.width).toBeLessThan(firstBounds.x);
  expect(replyBounds.height).toBe(firstBounds.height);
  const sendBounds = (await docked.getByRole('button', { name: 'Send · Ask a question', exact: true }).boundingBox())!;
  expect(replyBounds.y + replyBounds.height - sendBounds.y - sendBounds.height).toBeLessThan(24);
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'All', exact: true }).click();
  await expect(docked.getByLabel(field, { exact: true })).toHaveValue(feedback);
  await expect(page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true }).getByText('Draft', { exact: true })).toBeVisible();
  const tenantField = await openTenantSwitcher(page);
  await tenantField.fill('other');
  await tenantField.press('Enter');
  await expect(page).toHaveURL('/project/other/jobs');
  await expect(docked.getByLabel(field, { exact: true })).toHaveValue(feedback);
  await docked.getByRole('button', { name: 'Maximize composer', exact: true }).click();
  await expect(docked).toHaveClass(/maximized/);
  await docked.getByRole('button', { name: 'Restore composer size', exact: true }).click();
  await docked.getByRole('button', { name: 'Return reply to thread', exact: true }).click();
  await expect(page).toHaveURL(/\/jobs\/one\/story\?/);
  await expect(docked).toHaveCount(0);
  await first.getByRole('button', { name: 'Minimize composer' }).click();
  const inline = page.getByRole('region', { name: 'Response composer', exact: true });
  await expect(inline.getByLabel(field, { exact: true })).toHaveValue(feedback);
  await expect(inline.getByText(/notes\.txt/)).toBeVisible();
  await inline.getByLabel(field, { exact: true }).press('Control+Enter');
  await expect.poll(() => fixture.state.submits.length).toBe(1);
  expect(fixture.state.submits[0].response.feedback).toBe(feedback);
});

test('old review links redirect to the inbox and the matching thread document', async ({ page }) => {
  const fixture = await inboxFixture(page);
  await page.goto('/project/v2/reviews');
  await expect(page).toHaveURL('/project/v2/jobs?inbox=reviews');
  await expect(page.getByRole('heading', { name: 'Reviews', exact: true })).toBeVisible();
  const query = new URLSearchParams({ review_id: fixture.state.request.review_id, document_id: 'review/source.txt' });
  await page.goto(`/project/v2/reviews/one?${query}`);
  await expect(page).toHaveURL(/\/project\/v2\/jobs\/one\/story\?/);
  await v2Selected(page, 2);
});

test('a recorded review question aligns with other thread messages and shows its reply attachment', async ({ page }) => {
  const fixture = await inboxFixture(page);
  fixture.state.pending = false;
  fixture.state.output = {
    response: { direction: 'ask', feedback: 'Please read the retained note.', settings: {}, attachments: [
      { kind: 'stored', name: 'note.md', stored: { key: { jobId: 'producer', taskOrdinal: 1, name: 'review/note.md', sizeBytes: 16 } } },
    ] },
    receipt: { actor: { id: 'review@example.com' } },
  };
  await page.route('**/jobs/producer/stored-document?*', route =>
    new URL(route.request().url()).searchParams.get('name') === 'review/note.md'
      ? route.fulfill({ contentType: 'text/markdown', body: '# Retained note' }) : route.fallback());
  await page.goto('/project/v2/jobs/one/story');
  const request = page.locator('details.inbox-history-message[data-occurrence="entry"]').filter({ hasText: fixture.state.request.title });
  const response = page.locator('details.inbox-history-message[data-occurrence="entry"]').filter({ hasText: 'Please read the retained note.' });
  await expect(response.locator('summary')).toContainText('Question');
  const card = response.getByLabel('Accepted response', {exact:true});
  await expect(card).toHaveCSS('border-left-width', '0px');
  await expect(card).toHaveCSS('margin-left', '0px');
  await expect(request.getByRole('region', { name: 'Recorded request' })).toBeVisible();
  await expect(request).not.toContainText('Please read the retained note.');
  await expect(request).not.toContainText('Accepted response');
  await response.getByRole('button', { name: 'note.md', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'note.md', exact: true }).getByRole('heading', { name: 'Retained note' })).toBeVisible();
});

test('a waiting thread uses listed job titles and looks up missing dependencies', async ({ page }) => {
  const fixture = await inboxFixture(page);
  fixture.state.pending = false;
  Object.assign(fixture.jobs[0], { status: 'PENDING_JOBS', wait_for: ['job-1', 'unlisted'] });
  fixture.jobs[1].title = 'Cached prerequisite';
  const reads: string[] = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/tree-node')) reads.push(path);
  });
  await page.route('**/jobs/unlisted/tree-node', route => route.fulfill({ json: {
    node: { tenant_id: 'v2', job_id: 'unlisted', prompt: 'Run the unlisted prerequisite first.', status: 'ACTIVE' },
  } }));
  await page.goto('/project/v2/jobs');
  await page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true }).click();
  const dependencies = page.getByRole('list', { name: 'Jobs this thread is waiting for' });
  await expect(dependencies.getByRole('link', { name: 'Cached prerequisite' })).toHaveAttribute('href', '/project/v2/jobs/job-1/story');
  await expect(dependencies.getByRole('link', { name: 'Run the unlisted prerequisite first.' })).toHaveAttribute('href', '/project/v2/jobs/unlisted/story');
  expect(reads.filter(path => /\/(job-1|unlisted)\/tree-node$/.test(path))).toEqual(['/api/projects/v2/jobs/unlisted/tree-node']);
});

test('opens a single original message and presents its inputs as attachments', async ({ page }) => {
  const fixture = await inboxFixture(page);
  fixture.state.pending = false;
  const job = fixture.jobs[0];
  const opening = 'The original prompt starts here. ';
  job.prompt = opening + 'Keep the complete prompt readable when the message opens. '.repeat(3).trim();
  let fallbackReads = 0;
  await page.route('**/jobs/one/input-artifacts', route => {
    fallbackReads++;
    return route.fulfill({ json: { artifacts: [] } });
  });
  await page.route('**/jobs/one/conversation?*', route => route.fulfill({ json: {
    tenant_id: 'v2', job_id: 'one', completeness: 'complete', snapshot_token: 'snapshot', entries: [],
    activity: { availability: 'available', value: job },
    input_artifacts: [{ jobId: 'one', taskOrdinal: 0, inputOrdinal: 0, name: 'brief.md', sizeBytes: 20 }],
  } }));
  await page.route('**/jobs/one/tasks/0/artifacts/brief.md', route =>
    route.fulfill({ contentType: 'text/plain', body: '# Attached brief' }));
  await page.goto('/project/v2/jobs/one/story');
  const original = page.locator('.inbox-original');
  await expect(original).toHaveAttribute('open', '');
  const sender = original.locator('.thread-message-sender');
  const subject = original.locator('.thread-message-subject');
  const prose = original.getByRole('region', { name: 'Job prompt' }).locator('p');
  await expect(sender).toHaveText('Unknown');
  await expect(subject).toHaveCount(0);
  await expect(prose).toHaveText(job.prompt);
  const openOffset = await prose.evaluate(element => element.getBoundingClientRect().top - element.closest('details')!.getBoundingClientRect().top);
  await original.locator(':scope > summary').click();
  await expect(subject).toHaveText(/^The original prompt starts here\..+…$/);
  expect((await sender.boundingBox())!.y).toBeLessThan((await subject.boundingBox())!.y);
  const closedOffset = await subject.evaluate(element => element.getBoundingClientRect().top - element.closest('details')!.getBoundingClientRect().top);
  expect(openOffset).toBeCloseTo(closedOffset, 0);
  await original.locator(':scope > summary').click();
  const attachments = original.getByRole('region', { name: 'Input attachments' });
  await expect(attachments).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Input artifacts' })).toHaveCount(0);
  const brief = attachments.getByRole('button', { name: 'brief.md', exact: true });
  await expect(brief.locator('svg[data-icon="file-markdown"]')).toBeVisible();
  await brief.click();
  const preview = page.getByRole('dialog', { name: 'brief.md', exact: true });
  await expect(preview.getByRole('heading', { name: 'Attached brief' })).toBeVisible();
  await preview.getByRole('button', { name: 'Document options' }).click();
  await expect(page.getByRole('dialog', { name: 'Document options' }).getByRole('link', { name: 'Download' }))
    .toHaveAttribute('href', /\/jobs\/one\/tasks\/0\/artifacts\/brief\.md$/);
  expect(fallbackReads).toBe(0);
});

for (const width of [390, 767, 768, 800]) test(`responsive inbox and story at ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: width === 800 ? 600 : 844 });
  await inboxFixture(page); await page.goto('/project/v2/jobs');
  await expect(page.getByRole('list', { name: 'Jobs' }).getByRole('listitem')).toHaveCount(44);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (width < 768) {
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await expect(page.getByRole('dialog').getByRole('link', { name: 'Archive', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page.getByRole('button', { name: 'Open navigation' })).toBeFocused();
  }
  if (width === 390) await page.screenshot({ path: test.info().outputPath('inbox-phone.png') });
  await page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true }).click();
  const header = page.getByTestId('job-story-header');
  const cell = header.getByLabel('Job cell', { exact: true });
  const title = header.getByRole('heading', { name: 'Make reviews easier to read', exact: true });
  const status = header.getByLabel('Job status', { exact: true });
  await expect(cell).toHaveText('platform');
  const left = async (element: typeof cell) => (await element.boundingBox())!.x;
  expect(await left(header.getByRole('link', { name: 'Back to inbox' }))).toBeLessThan(await left(cell));
  expect(await left(cell)).toBeLessThan(await left(title));
  await expect(status).toHaveText('Needs review');
  expect(await status.evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  await expect(page.locator('.inbox-history-message[open] > summary').filter({ hasText: 'Make reviews easier to read' })).toBeVisible();
  await v2Selected(page, 0);
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  await page.getByLabel('Question or requested change (required)').fill('Please keep the response accessible.');
  await page.getByRole('button', { name: 'Send · Ask a question' }).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button', { name: 'Send · Ask a question' })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (width === 390) await page.screenshot({ path: test.info().outputPath('inbox-review-phone.png') });
});

test('New message docks independent composers, saves drafts and sends the restored message', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const fixture = await inboxFixture(page);
  await page.goto('/project/v2/jobs');
  const newMessage = page.getByRole('button', { name: 'New message', exact: true });
  const composers = page.getByRole('region', { name: 'New message', exact: true });
  await newMessage.click();
  const first = composers.first();
  const firstBounds = (await first.boundingBox())!;
  await expect(first.getByText('Cell', { exact: true })).toBeVisible();
  await expect(first.getByRole('toolbar', { name: 'Text formatting' })).toHaveCount(0);
  await first.getByRole('combobox', { name: 'Recipient cell' }).click();
  await page.getByRole('option', { name: 'platform', exact: true }).click();
  await expect(first.getByLabel('Subject', { exact: true })).toBeFocused();
  await expect(first.getByLabel('Subject', { exact: true })).toHaveCSS('outline-style', 'none');
  await first.getByLabel('Subject', { exact: true }).fill('Inbox request');
  await page.keyboard.press('Tab');
  await expect(first.getByLabel('Message', { exact: true })).toBeFocused();
  await expect(first.getByLabel('Message', { exact: true })).toHaveCSS('outline-style', 'none');
  await first.getByRole('button', { name: 'Formatting options' }).click();
  await expect(first.getByRole('toolbar', { name: 'Text formatting' })).toBeVisible();
  await first.getByRole('button', { name: 'Markdown source', exact: true }).click();
  await first.getByLabel('Message', { exact: true }).fill('First line\n\nSecond line');
  await newMessage.click();
  await expect(composers).toHaveCount(2);
  const second = composers.last();
  const secondBounds = (await second.boundingBox())!;
  expect((await first.boundingBox())!.x).toBe(firstBounds.x);
  expect(secondBounds.x + secondBounds.width).toBeLessThan(firstBounds.x);
  await second.getByLabel('Subject', { exact: true }).fill('Discard this draft');
  await second.getByRole('button', { name: 'Discard draft' }).click();
  await expect(composers).toHaveCount(1);
  await first.getByRole('button', { name: 'Maximize composer' }).click();
  await expect(first).toHaveClass(/maximized/);
  await first.getByRole('button', { name: 'Close composer' }).click();
  await page.getByRole('link', { name: 'Drafts', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Drafts', exact: true })).toBeVisible();
  await expect(page.getByText('Discard this draft')).toHaveCount(0);
  await page.getByRole('button', { name: 'Inbox request', exact: true }).click();
  await expect(composers.getByLabel('Message', { exact: true })).toContainText(/First line[\s\S]*Second line/);
  await composers.getByLabel('Message', { exact: true }).press('Control+Enter');
  await expect(page.getByText('Message sent', { exact: true })).toBeVisible();
  await expect(composers).toHaveCount(0);
  await expect(page).toHaveURL('/project/v2/jobs?inbox=drafts');
  await expect.poll(() => fixture.submissions.length).toBe(1);
  expect(fixture.submissions[0].inputs.prompt).toBe('Inbox request\n\nFirst line\n\nSecond line');
  expect(fixture.submissions[0].job_id).toBeTruthy();
});

test('mobile inbox and full-screen composers retain drafts and keep one window visible', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  const fixture = await inboxFixture(page, true);
  const prompt = 'A long prompt without a title should use the width available on the phone. '.repeat(4).trim();
  Object.assign(fixture.jobs[1], { title: '', prompt });
  Object.assign(fixture.jobs[2], { title: 'Scheduled task', status: 'AWAITING_FUTURE' });
  Object.assign(fixture.jobs[3], { title: 'Running task', status: 'ACTIVE' });
  await page.goto('/project/v2/jobs');
  const compose = page.getByRole('button', { name: 'New message', exact: true });
  const windows = page.getByRole('region', { name: 'New message', exact: true });
  await compose.click();
  await windows.first().getByLabel('Subject', { exact: true }).fill('First draft');
  await compose.click();
  await windows.last().getByLabel('Subject', { exact: true }).fill('Second draft');
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(windows).toHaveCount(1);
  await expect(windows.getByLabel('Subject', { exact: true })).toHaveValue('Second draft');
  const fullScreen = { x: 0, y: 0, width: 390, height: 844 };
  await expect.poll(() => windows.boundingBox()).toEqual(fullScreen);
  await expect(windows.getByRole('button', { name: 'Minimize composer' })).toHaveCount(0);
  const recipient = windows.getByRole('combobox', { name: 'Recipient cell' });
  const subject = windows.getByLabel('Subject', { exact: true });
  const body = windows.getByLabel('Message', { exact: true });
  await expect(recipient).toHaveCSS('font-size', '16px');
  await recipient.click();
  await page.getByRole('option', { name: 'platform', exact: true }).click();
  await expect(subject).toBeFocused();
  await expect(subject).toHaveCSS('font-size', '16px');
  await body.click();
  await expect(body).toBeFocused();
  await expect(body).toHaveCSS('font-size', '16px');
  await body.fill('Compose at full width.');
  await expect.poll(() => windows.boundingBox()).toEqual(fullScreen);
  await windows.getByRole('button', { name: 'Close composer' }).click();
  await expect(windows.getByLabel('Subject', { exact: true })).toHaveValue('First draft');
  await windows.getByRole('button', { name: 'Close composer' }).click();
  await expect(windows).toHaveCount(0);

  const row = page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true });
  await expect(page.getByRole('link', { name: /^req:/ })).toHaveCount(0);
  const status = row.getByText('Needs review', { exact: true });
  const time = row.locator('time');
  expect((await status.boundingBox())!.x).toBeLessThan((await time.boundingBox())!.x);
  const preview = page.locator('.inbox-job-subject').filter({ hasText: prompt });
  await expect(preview).toHaveCSS('-webkit-line-clamp', '2');
  expect(await preview.evaluate(el => el.clientHeight < el.scrollHeight)).toBe(true);

  const openFolder = async (name: string) => {
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await page.getByRole('dialog', { name: 'Navigation' }).getByRole('link', { name, exact: true }).click();
  };
  await openFolder('Active');
  await expect(page.getByRole('link', { name: 'platform: Running task', exact: true }).getByText('Running', { exact: true })).toBeVisible();
  await openFolder('Waiting');
  await expect(page.getByRole('link', { name: 'platform: Scheduled task', exact: true }).getByText('Scheduled', { exact: true })).toBeVisible();
  await openFolder('Reviews');
  await expect(row).toBeVisible();
  await expect(status).toHaveCount(0);
  await row.click();
  await v2Selected(page, 0);
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  const reply = page.getByRole('region', { name: 'Popped-out review reply', exact: true });
  await expect.poll(() => reply.boundingBox()).toEqual(fullScreen);
  await expect(page.locator('.embedded-review .review-v2-composer')).toHaveCount(0);
  const action = reply.getByRole('button', { name: 'Change response type: Ask a question' });
  await expect(action).toHaveText('Ask a question');
  await expect(reply.getByText('platform', { exact: true })).toBeVisible();
  const editor = reply.getByLabel('Question or requested change (required)');
  await expect(editor).toHaveCSS('font-size', '16px');
  await editor.fill('Keep this mobile reply.');
  await reply.getByRole('button', { name: 'Close composer' }).click();
  await expect(reply).toHaveCount(0);
  await openFolder('Drafts');
  await page.getByRole('link', { name: /Review response · Keep this mobile reply/ }).click();
  await expect(editor).toHaveValue('Keep this mobile reply.');
  await editor.press('Meta+Enter');
  await expect(page.getByText('Response submitted', { exact: true })).toBeVisible();
  await expect(reply).toHaveCount(0);
  await expect(page.getByTestId('job-story-header').getByText('Draft', { exact: true })).toHaveCount(0);
  expect(fixture.state.submits).toHaveLength(1);
});

test('Drafts can discard open composers and review responses without shifting the list', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await inboxFixture(page);
  await page.goto('/project/v2/jobs?inbox=drafts');
  const compose = page.getByRole('button', { name: 'New message', exact: true });
  const composers = page.getByRole('region', { name: 'New message', exact: true });
  for (const subject of ['Keep this draft', 'Discard this draft']) {
    await compose.click();
    await composers.last().getByLabel('Subject', { exact: true }).fill(subject);
  }
  const list = page.getByRole('list', { name: 'Drafts' });
  const keep = list.getByRole('checkbox', { name: 'Select draft Keep this draft' });
  const remove = list.getByRole('checkbox', { name: 'Select draft Discard this draft' });
  const top = () => list.evaluate(element => element.getBoundingClientRect().top);
  const initial = await top();
  await page.getByRole('button', { name: 'Select drafts by' }).click();
  await page.getByRole('menuitem', { name: 'All', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Discard drafts', exact: true })).toBeVisible();
  await keep.uncheck();
  await expect(remove).toBeChecked();
  expect(await top()).toBe(initial);
  await page.getByRole('group', { name: 'Draft actions' }).getByRole('button', { name: 'Discard draft', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Discard', exact: true }).click();
  await expect(list.getByRole('listitem')).toHaveCount(1);
  await expect(composers).toHaveCount(1);
  await expect(keep).not.toBeChecked();
  expect(await top()).toBe(initial);

  await composers.getByRole('button', { name: 'Close composer' }).click();
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'All', exact: true }).click();
  await page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true }).click();
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  await page.getByLabel('Question or requested change (required)', { exact: true }).fill('Unsent review response');
  await page.getByRole('link', { name: 'Drafts', exact: true }).click();
  await expect(list.getByRole('listitem')).toHaveCount(2);
  const bulkTop = await top();
  await page.getByRole('checkbox', { name: 'Select all drafts' }).check();
  await page.getByRole('button', { name: 'Discard drafts', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Discard', exact: true }).click();
  await expect(page.getByText('No saved drafts')).toBeVisible();
  await expect.poll(top).toBe(bulkTop);
});

for (const width of [1440, 768]) test(`upstream runner access and job requirements remain usable at ${width}px`, async ({ page }) => {
  await page.setViewportSize({ width, height: 900 });
  await inboxFixture(page, true);
  await page.goto('/project/v2/jobs');
  const runners = page.getByRole('link', { name: 'Runners', exact: true });
  await expect(runners).toHaveAttribute('href', '/project/v2/runners');
  await expect(runners).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole('button', { name: /Discuss|Assistant/ })).toHaveCount(0);
  const requirements = page.getByRole('link', { name: 'req: linux 4c/8g [browser]', exact: true });
  await expect(requirements).toBeInViewport({ ratio: 1 });
  expect(await requirements.evaluate(el => el.parentElement?.closest('a'))).toBeNull();
  const row = page.locator('.inbox-job-row').filter({ has: requirements });
  const subject = (await row.locator('.inbox-job-subject').boundingBox())!;
  const badge = (await requirements.locator('span').boundingBox())!;
  const date = (await row.locator('time').boundingBox())!;
  const cell = (await row.locator('.inbox-correspondent').boundingBox())!;
  expect(badge.x).toBeGreaterThanOrEqual(subject.x + subject.width);
  expect(Math.abs(badge.x + badge.width - date.x)).toBeLessThanOrEqual(1);
  const center = (box: { y: number; height: number }) => box.y + box.height / 2;
  for (const item of [subject, date, cell]) expect(Math.abs(center(item) - center(badge))).toBeLessThanOrEqual(1);
  const plainRow = page.locator('.inbox-job-row').filter({ hasNot: page.locator('.job-requirements-summary') }).first();
  expect((await row.boundingBox())!.height).toBe((await plainRow.boundingBox())!.height);
  expect((await requirements.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`inbox-upstream-${width}.png`), animations: 'disabled' });
  await requirements.click();
  await expect(page).toHaveURL(/jobs\/one\/story\?.*tab=runtime/);
  await expect(page.getByRole('tab', { name: 'Execution', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('region', { name: 'Execution requirements' })).toContainText('4 cores');
  await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
  await expect(requirements).toBeVisible();
});
