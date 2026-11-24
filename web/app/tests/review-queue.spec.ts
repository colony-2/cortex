import { test, expect } from '@playwright/test';
import { jobIdentity, mockCells } from './helpers/cells';
import { openTenantSwitcher } from './helpers/inbox';

for (const width of [1440, 390]) test(`Reviews inbox search, sorting, and live updates at ${width}px`, async ({ page, context, baseURL }) => {
  await page.setViewportSize({ width, height: 1000 });
  await page.clock.setFixedTime(new Date('2026-11-10T12:00:00Z'));
  await context.addCookies([{ name: 'colony2:user:email', value: 'review@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).queueEvents = {};
    (window as any).EventSource = class {
      readyState = 1;
      close() {}
      removeEventListener() {}
      addEventListener(type: string, listener: unknown) { (window as any).queueEvents[type] = listener; }
    };
  });
  const reads: string[] = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith('/api/') && request.method() === 'GET') reads.push(path);
  });
  const row = (id: string, title: string, requested_at?: string) => ({
    ...jobIdentity, id, job_id: id, tenant_id: 'queue', title, requested_at,
    request_id: `request-${id}`, client_payload_revision: 1,
  });
  const first = row('first', 'First project review', '2026-11-08T12:00:00Z');
  const latest = row('latest', 'Latest project review', '2026-11-10T11:45:00Z');
  let rows = [first, latest, row('unknown', 'Another request')];
  await page.route('**/api/**', route => route.fulfill({ status: 404, json: { error: 'No fixture for this endpoint' } }));
  await page.route('**/api/projects', route => route.fulfill({ json: [{ tenant_id: 'queue' }, { tenant_id: 'other' }] }));
  await mockCells(page, ['platform']);
  await page.route('**/api/projects/queue/user-inputs/pending', route => route.fulfill({ json: rows }));
  await page.route('**/api/projects/other/user-inputs/pending', route => route.fulfill({ json: [] }));
  await page.route('**/jobs?*', route => route.fulfill({ json: { jobs: [] } }));
  await page.route('**/jobs/first', route => route.fulfill({ json: { ...first, status: 'READY', store: 'ACTIVE' } }));
  await page.route('**/jobs/first/conversation?*', route => route.fulfill({ json: {
    tenant_id: 'queue', job_id: 'first', entries: [], snapshot_token: 'snapshot', snapshot_revision: 1, completeness: 'complete',
  } }));
  await page.route('**/user-inputs/first', route => route.fulfill({ json: {
    jobId: 'first', status: 'pending', startTime: first.requested_at, form: { question: 'Confirm the request?', request_id: first.request_id },
  } }));
  await page.route('**/user-inputs/first/respond', route => {
    rows = rows.filter(item => item.id !== 'first');
    return route.fulfill({ json: {} });
  });

  await page.goto('/project/queue/reviews');
  await expect(page).toHaveURL('/project/queue/jobs?inbox=reviews');
  const list = page.getByRole('list', { name: 'Jobs', exact: true });
  const items = list.getByRole('listitem');
  await expect(items).toHaveCount(3);
  await expect(items.first()).toContainText('Latest project review');
  await expect(items.last()).toContainText('Date unavailable');
  const time = items.nth(1).locator('time');
  await expect(time).toContainText('Nov 8');
  await time.hover();
  await expect(page.getByRole('tooltip')).toContainText('2026');
  const search = page.getByRole('searchbox', { name: 'Search threads' });
  await search.fill('project review');
  await search.press('Enter');
  await expect(items).toHaveCount(2);
  await page.getByRole('button', { name: 'Sort threads' }).click();
  await page.getByRole('menuitem', { name: 'Date (oldest first)' }).click();
  await expect(items.first()).toContainText('First project review');
  expect(reads.filter(path => /\/jobs(?:\/|$)|\/user-inputs\/(?!pending$)/.test(path))).toEqual([]);

  await expect(page.getByRole('link', { name: 'platform: First project review' })).toBeVisible();
  await page.getByRole('link', { name: 'platform: First project review' }).click();
  await expect(page).toHaveURL(/\/project\/queue\/jobs\/first\/story\?/);
  await page.getByRole('textbox', { name: 'Confirm the request?' }).fill('Yes');
  await page.getByRole('button', { name: 'Submit', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Confirm the request?' })).toHaveCount(0);
  await page.getByRole('link', { name: 'Back to inbox' }).click();
  await expect(search).toHaveValue('project review');
  await expect(items).toHaveCount(1);
  await expect(items.first()).toContainText('Latest project review');

  const replacement = row('replacement', 'Replacement project review');
  rows = [replacement];
  await page.evaluate(inputs => {
    (window as any).queueEvents.inputs_changed({ data: JSON.stringify({ inputs }) });
  }, rows);
  await expect(items).toHaveCount(1);
  await expect(items.first()).toContainText('Replacement project review');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
  const tenant = await openTenantSwitcher(page);
  await tenant.fill('other');
  await tenant.press('Enter');
  await expect(page).toHaveURL('/project/other/jobs');
  await expect(search).toHaveValue('');
  await expect(items).toHaveCount(0);
});
