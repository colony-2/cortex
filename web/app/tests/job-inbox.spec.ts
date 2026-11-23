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
  const state = { jobs, submissions: [] as any[] };
  await page.route(/\/jobs\/one(?:\?.*)?$/, route => route.fulfill({ json: jobs[0] }));
  await page.route(/\/api\/projects\/v2\/cells(?:\/[^/?]+)?(?:\?.*)?$/, route => {
    const selected = new URL(route.request().url()).pathname.split('/')[5];
    return route.fulfill({ json: selected ? cells.find(cell => cell.id === selected) : { cells } });
  });
  await page.route(/\/api\/projects\/v2\/jobs\?/, route => {
    if (route.request().method() !== 'GET') return route.fallback();
    const inclusive = new URL(route.request().url()).searchParams.get('includeHidden') === 'true';
    return route.fulfill({ json: { jobs: state.jobs.filter(job => inclusive || !job.inboxArchived) } });
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
  await expect(all.getByLabel('24 pending responses')).toBeVisible();
  await expect(page.locator('.inbox-cell-scroll > a')).toHaveCount(8);
  await page.getByRole('button', { name: 'Search all cells' }).click();
  await page.getByRole('textbox', { name: 'Search cells' }).fill('cell-39');
  await page.getByRole('link', { name: 'cell-39', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'cell-39', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add cell' })).toBeVisible();
  await all.click();
  await page.getByRole('textbox', { name: 'Search cells' }).fill('');
  await page.screenshot({ path: test.info().outputPath('inbox-desktop.png') });
  const row = page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true });
  await expect(row.getByLabel('Unread')).toBeVisible();
  await row.click(); await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeVisible();
  await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
  await expect(row.getByLabel('Unread')).toHaveCount(0);
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'Archive', exact: true }).click();
  await expect(page.getByRole('list', { name: 'Jobs' }).getByRole('listitem')).toHaveCount(8);
  await expect(page.locator('.inbox-job-row.unread')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Archive', exact: true }).locator('.inbox-badge')).toHaveCount(0);
});

test('embedded review preserves draft through sidebar expansion and Settings, and sends the native response', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const fixture = await inboxFixture(page);
  await page.route('**/api/projects/v2/admin*', route => route.fulfill({ json: { revision: '1', hooks: [], configs: [], git: [], secretReferences: [], secretWrites: false } }));
  await page.goto('/project/v2/jobs/one/story');
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeVisible({ timeout: 45_000 });
  await v2Selected(page, 0);
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeVisible();
  await expect(page.getByText('2 earlier events')).toBeVisible();
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
  await page.getByRole('button', { name: 'Expand navigation' }).click();
  await expect(editor).toHaveValue('Keep this response while I inspect the cell list.');
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '76px');
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath('inbox-review-desktop.png'), animations: 'disabled' });
  await page.getByRole('link', { name: 'Settings', exact: true }).click();
  await page.goBack(); await v2Selected(page, 0);
  await expect(editor).toHaveValue('Keep this response while I inspect the cell list.');
  await page.getByRole('button', { name: 'Send · Ask a question' }).click();
  await expect.poll(() => fixture.state.submits.length).toBe(1);
  expect(fixture.state.submits[0].response.feedback).toBe('Keep this response while I inspect the cell list.');
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
  await page.getByRole('link', { name: 'platform: Make reviews easier to read', exact: true }).click(); await v2Selected(page, 0);
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  await page.getByLabel('Question or requested change (required)').fill('Please keep the response accessible.');
  await page.getByRole('button', { name: 'Send · Ask a question' }).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button', { name: 'Send · Ask a question' })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (width === 390) await page.screenshot({ path: test.info().outputPath('inbox-review-phone.png') });
});

test('Compose keeps the draft on close and sends a job with an optional subject', async ({ page }) => {
  const fixture = await inboxFixture(page); await page.goto('/project/v2/jobs');
  await page.getByRole('button', { name: 'Compose', exact: true }).click();
  await page.getByRole('combobox', { name: 'Recipient cell' }).click();
  await page.getByRole('option', { name: 'platform', exact: true }).click();
  await page.getByLabel('Subject (optional)').fill('Inbox request');
  await page.getByLabel('Message', { exact: true }).fill('First line\n\nSecond line');
  await page.getByRole('button', { name: 'Close composer' }).click();
  await page.getByRole('button', { name: 'Compose', exact: true }).click();
  await expect(page.getByLabel('Message', { exact: true })).toHaveText('First line\n\nSecond line');
  await page.screenshot({ path: test.info().outputPath('inbox-compose.png') });
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  await expect.poll(() => fixture.submissions.length).toBe(1);
  expect(fixture.submissions[0].inputs.prompt).toBe('Inbox request\n\nFirst line\n\nSecond line');
  expect(fixture.submissions[0].job_id).toBeTruthy();
});

for (const width of [1440, 390]) test(`upstream runner access and job requirements remain usable at ${width}px`, async ({ page }) => {
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
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath(`inbox-upstream-${width}.png`), animations: 'disabled' });
  await requirements.click();
  await expect(page).toHaveURL(/jobs\/one\/story\?.*tab=env/);
  await expect(page.getByRole('tab', { name: 'Env', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('region', { name: 'Execution requirements' })).toContainText('4 cores');
  await page.getByRole('link', { name: 'Back to inbox', exact: true }).click();
  await expect(requirements).toBeVisible();
});
