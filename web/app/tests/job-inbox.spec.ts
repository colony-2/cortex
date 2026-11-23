import { test, expect, type Page } from '@playwright/test';
import { authorizeV2, reviewV2BrowserFixture, v2Selected } from './helpers/reviewV2';
import { cellFixture } from './helpers/cells';

async function inboxFixture(page: Page, withRequirements = false) {
  const review = await reviewV2BrowserFixture(page, true);
  const cells = Array.from({ length: 40 }, (_, index) => cellFixture(index === 0 ? 'platform' : `cell-${String(index).padStart(2, '0')}`, 'v2'));
  const jobs = Array.from({ length: 52 }, (_, index) => {
    const cell = cells[index < 8 ? 0 : (index - 8) % 39 + 1];
    return { tenant_id: 'v2', job_id: index === 0 ? 'one' : `job-${index}`, goal_id: 'same-goal', cell_id: cell.id, cell_name: cell.name, repo: cell.repo, execution: withRequirements && index === 0 ? { status: 'specified', source: 'yield', published: true, demand: { effective: { base: 'linux', resources: { cpu: '4', memory: '8Gi' }, capabilities: ['browser'] } } } : undefined, title: index === 0 ? 'Make reviews easier to read' : `Improve ${cell.name} workflow`, prompt: 'Keep the current request in focus and the conversation close by.', status: index < 24 ? 'READY' : 'COMPLETED', store: index < 24 ? 'ACTIVE' : 'ARCHIVED', archived_at: index < 24 ? undefined : '2026-11-21T12:00:00Z', created_at: '2026-11-20T10:00:00Z', inboxArchived: index >= 44 };
  });
  const state = { jobs, submissions: [] as any[], jobReads: [] as URLSearchParams[] };
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
    return route.fulfill({ json: { jobs: state.jobs.filter(job => (inclusive || !job.inboxArchived) && (!names.length || names.includes(job.cell_name))) } });
  });
  await page.route('**/api/projects/v2/jobs', async route => {
    if (route.request().method() !== 'POST') return route.fallback();
    const body = route.request().postDataJSON(); state.submissions.push(body);
    return route.fulfill({ json: { tenant_id: 'v2', job_id: body.job_id, goal_id: 'opaque', cell_id: 'cell-platform', cell_name: 'platform', repo: cells[0].repo } });
  });
  await page.route('**/user-inputs/pending', route => route.fulfill({ json: state.jobs.filter(job => !job.inboxArchived && job.status === 'READY' && (job.job_id !== 'one' || review.state.pending)).map(job => ({ ...job, id: job.job_id, client_payload_revision: 1, request_id: job.job_id === 'one' ? review.state.native : `request-${job.job_id}`, requested_at: withRequirements && job.job_id === 'one' ? '2026-11-22T14:00:00Z' : '2026-11-21T14:00:00Z' })) }));
  await page.route('**/api/projects/v2/jobs/one/story', route => route.fulfill({ json: { job_id: 'one', status: 'running', root: null } }));
  await page.route('**/api/projects/v2/jobs/one/input-artifacts', route => route.fulfill({ json: { artifacts: [] } }));
  await page.route('**/api/projects/v2/jobs/one/workspace-diff*', route => route.fulfill({ json: { status: 'absent', comparisons: [] } }));
  await page.route('**/api/projects/v2/jobs/one/conversation?*', route => route.fulfill({ json: {
    tenant_id: 'v2', job_id: 'one', snapshot_token: 'snapshot', snapshot_revision: 1, completeness: 'complete',
    activity: { availability: 'available', value: { ...jobs[0], status: 'ACTIVE' } },
    entries: [1, 2, 3, 4].map(index => ({ entry_id: `previous-${index}`, request_id: `previous-native-${index}`, kind: 'ordinary', title: { present: true, raw: `Earlier update ${index}` }, summary: { present: false }, requested_at: { present: false }, state: 'answered', locator: {}, form_fields: { question: 'Review the inbox layout' }, accepted_response: { present: true, raw: 'Keep the current review in focus.' }, actor: { provenance: 'reported_receipt', receipt: {}, legacy_user_id: { present: false } } })).concat([{
      entry_id: 'entry', request_id: review.state.native, kind: 'structured', title: { present: true, raw: review.state.request.title }, summary: { present: false }, requested_at: { present: false }, state: review.state.pending ? 'pending' : 'answered', locator: { generation_ordinal: 1 }, form_fields: { request_id: review.state.native, presentation: { type: 'review/v2' }, request: review.state.request, request_schema: { type: 'object' }, response_schema: { type: 'object' } }, accepted_response: { present: !!review.state.output }, actor: { provenance: 'reported_receipt', receipt: review.state.output?.receipt, legacy_user_id: { present: false } },
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
  const waiting = page.getByRole('link', { name: 'Waiting', exact: true });
  await expect(waiting.getByLabel('24 pending responses')).toBeVisible();
  await waiting.click();
  await expect(page.getByRole('heading', { name: 'Waiting', exact: true })).toBeVisible();
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
  await all.click();
  await page.getByRole('textbox', { name: 'Search cells' }).fill('');
  await page.screenshot({ path: test.info().outputPath('inbox-desktop.png') });
  const row = page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true });
  await expect(row.getByLabel('Unread')).toBeVisible();
  await row.click(); await expect(page.getByRole('button', { name: 'Collapse navigation' })).toBeVisible();
  await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
  await expect(row.getByLabel('Unread')).toHaveCount(0);
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'Archive', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Jobs' }).getByRole('listitem')).toHaveCount(8);
  await expect(page.locator('.inbox-job-row.unread')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Archive', exact: true }).locator('.inbox-badge')).toHaveCount(0);
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
  await nav.getByRole('link', { name: 'Waiting', exact: true }).click();
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
  await expect(page.getByText('2 earlier messages')).toBeVisible();
  const earlier = page.locator('.inbox-history-message').first();
  await earlier.locator('summary').click();
  await expect(earlier.getByLabel('Accepted response', { exact: true })).toBeVisible();
  await earlier.locator('summary').click();
  await page.locator('.embedded-review').getByRole('heading', { name: 'Make reviews easier to read', exact: true }).evaluate(element => element.scrollIntoView({ block: 'start' }));
  await expect(page.getByText('FINAL DOCUMENT CONTENT', { exact: true })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('inbox-review-reading.png') });
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  const editor = page.getByLabel('Question or requested change (required)', { exact: true });
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
  await v2Selected(page, 0);
  await expect(editor).toHaveValue('Keep this response while I inspect the cell list.');
  await page.getByRole('button', { name: 'Send · Ask a question' }).click();
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

test('opens a single original message and presents its inputs as attachments', async ({ page }) => {
  const fixture = await inboxFixture(page);
  fixture.state.pending = false;
  const job = fixture.jobs[0];
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
  await expect(sender).toHaveText('Unknown');
  await expect(subject).toHaveText(job.prompt);
  expect((await sender.boundingBox())!.y).toBeLessThan((await subject.boundingBox())!.y);
  const attachments = original.getByRole('region', { name: 'Input attachments' });
  await expect(attachments).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Input artifacts' })).toHaveCount(0);
  await attachments.getByRole('button', { name: 'brief.md', exact: true }).click();
  await expect(page.getByRole('dialog').getByRole('heading', { name: 'Attached brief' })).toBeVisible();
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
  await first.getByRole('combobox', { name: 'Recipient cell' }).click();
  await page.getByRole('option', { name: 'platform', exact: true }).click();
  await first.getByLabel('Subject (optional)').fill('Inbox request');
  await first.getByRole('button', { name: 'Markdown source', exact: true }).click();
  await first.getByLabel('Message', { exact: true }).fill('First line\n\nSecond line');
  await newMessage.click();
  await expect(composers).toHaveCount(2);
  const second = composers.last();
  await second.getByLabel('Subject (optional)').fill('Discard this draft');
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
  await composers.getByRole('button', { name: 'Send', exact: true }).click();
  await expect.poll(() => fixture.submissions.length).toBe(1);
  expect(fixture.submissions[0].inputs.prompt).toBe('Inbox request\n\nFirst line\n\nSecond line');
  expect(fixture.submissions[0].job_id).toBeTruthy();
});

test('Drafts can discard open composers and review responses without shifting the list', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await inboxFixture(page);
  await page.goto('/project/v2/jobs?inbox=drafts');
  const compose = page.getByRole('button', { name: 'New message', exact: true });
  const composers = page.getByRole('region', { name: 'New message', exact: true });
  for (const subject of ['Keep this draft', 'Discard this draft']) {
    await compose.click();
    await composers.last().getByLabel('Subject (optional)').fill(subject);
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

for (const width of [1440, 768, 390]) test(`upstream runner access and job requirements remain usable at ${width}px`, async ({ page }) => {
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
