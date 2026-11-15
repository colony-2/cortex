import { test, expect, type Page, type BrowserContext } from '@playwright/test';
import { jobIdentity, mockCells } from './helpers/cells';

const reviewPath = '/project/history/reviews/one';
const requestId = 'review/雪 &+%';
const ids = ['a', 'b/雪 &+%', 'c'];
const titles = ['Plan', 'Checks', 'Sources'];
const headings = ['Plan document', 'Checks document', 'Sources document'];
const bodies = [
  '# Plan document\n\nReview the plan, then read the checks.\n\n[Open checks](checks.md#checks-document) [Same heading](plan.md#plan-document) [Missing document](missing.md)',
  '# Checks document\n\nThe checks confirm the proposed behavior.\n\n[Open sources](sources.md#sources-document)',
  '# Sources document\n\nSupporting sources for this review.\n\n[Same heading](sources.md#sources-document)',
];
function link(documentId?: string, request = requestId) {
  const params = new URLSearchParams({ request_id: request });
  if (documentId !== undefined) params.set('document_id', documentId);
  return reviewPath + '?' + params;
}
async function authorize(context: BrowserContext, baseURL: string) {
  await context.addCookies([{ name: 'colony2:user:email', value: 'review@example.com', url: baseURL }]);
  await context.addInitScript(() => { (window as any).EventSource = class { readyState = 1; close() {} addEventListener() {} removeEventListener() {} }; });
}
async function fixture(page: Page) {
  const state = { details: 0, submits: 0, requests: [] as string[], completed: [] as string[], failures: new Map<string, number | 'network'>(), pending: new Map<string, Promise<void>>() };
  await page.route('**/api/**', route => route.fulfill({ status: 404, json: { error: 'No fixture for this endpoint' } }));
  await page.route('**/api/projects', route => route.fulfill({ json: [{ ...jobIdentity, tenant_id: 'history' }] }));
  await mockCells(page, ['platform']);
  await page.route('**/user-inputs/pending', route => route.fulfill({ json: [] }));
  await page.route('**/jobs/one', route => route.fulfill({ json: { ...jobIdentity, job_id: 'one', status: 'ACTIVE', prompt: 'Review document navigation' } }));
  await page.route('**/jobs/one/children?*', route => route.fulfill({ json: { jobs: [] } }));
  await page.route('**/user-inputs/one', route => {
    state.details++;
    return route.fulfill({ json: { jobId: 'one', status: 'pending', startTime: '2026-11-14T10:00:00Z', form: {
      kind: 'review', title: 'Review document navigation', request_id: requestId, overview_document_id: 'a',
      documents: Object.fromEntries(ids.map((id, i) => [id, { kind: 'stored', name: ['plan.md', 'checks.md', 'sources.md'][i], stored: { key: { jobId: 'producer', taskOrdinal: 1, name: ['plan.md', 'checks.md', 'sources.md'][i] } } }])),
      document_metadata: Object.fromEntries(ids.map((id, i) => [id, { title: titles[i], role: i === 0 ? 'primary' : 'supporting', order: i, ...(i === 0 ? { source_document_ids: [ids[1]] } : {}) }])),
      fields: [
        { id: 'decision', type: 'multiple_choice', question: 'Proceed?', required: true, options: [{ value: 'approve', label: 'Approve' }, { value: 'revise', label: 'Revise' }] },
        { id: 'feedback', type: 'paragraph_text', question: 'Reason' },
        { id: 'attachment', type: 'file_upload', question: 'Annotations' },
      ],
    } } });
  });
  await page.route('**/reviews/one/respond', route => { state.submits++; return route.fulfill({ json: { ok: true } }); });
  await page.route('**/reviews/one/documents?*', async route => {
    const params = new URL(route.request().url()).searchParams;
    expect(params.get('request_id')).toBe(requestId);
    const id = params.get('document_id')!;
    state.requests.push(id);
    if (state.pending.has(id)) await state.pending.get(id);
    const failure = state.failures.get(id);
    if (failure === 'network') return route.abort('failed');
    if (failure) return route.fulfill({ status: failure, json: { error: 'Document unavailable from server' } });
    try { await route.fulfill({ contentType: 'text/markdown', body: bodies[ids.indexOf(id)] }); }
    finally { state.completed.push(id); }
  });
  return state;
}
async function selected(page: Page, i: number) {
  await expect(page.getByRole('heading', { name: headings[i], exact: true })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('document_id')).toBe(ids[i]);
  expect(new URL(page.url()).searchParams.get('request_id')).toBe(requestId);
}
async function catalog(page: Page, i: number, mobile = false) {
  if (await page.getByRole('button', { name: 'Show documents' }).count()) await page.getByRole('button', { name: 'Show documents' }).click();
  const nav = page.getByRole('navigation', { name: 'Review documents' });
  await nav.getByRole('button', { name: mobile ? `Read ${titles[i]}` : new RegExp(`^${titles[i]} `) }).click();
  await selected(page, i);
}
test.beforeEach(async ({ context, baseURL }) => authorize(context, baseURL!));

for (const width of [1440, 390]) test(`H28-31 document history and response draft at ${width}px`, async ({ page }, testInfo) => {
  const mobile = width < 768;
  await page.setViewportSize({ width, height: 900 });
  const state = await fixture(page);
  await page.goto('/project/history/reviews');
  await page.goto(reviewPath + '?filter=one&filter=two');
  await selected(page, 0);
  await page.getByRole('button', { name: 'Reject', exact: true }).click();
  await page.getByLabel('Reason', { exact: true }).fill('Keep this response while I compare documents.');
  await page.locator('input[type=file]').setInputFiles({ name: 'notes.md', mimeType: 'text/markdown', buffer: Buffer.from('Review annotations') });
  await page.locator('input[type=file]').evaluate(element => element.setAttribute('data-history-upload', 'retained'));
  await catalog(page, 1, mobile);
  await catalog(page, 2, mobile);
  for (const [direction, index] of [['back', 1], ['back', 0], ['forward', 1], ['forward', 2]] as const) {
    if (direction === 'back') await page.goBack(); else await page.goForward();
    await selected(page, index);
    await expect(page.getByLabel('Reason', { exact: true })).toHaveValue('Keep this response while I compare documents.');
    await expect(page.locator('.review-actions')).toContainText('notes.md');
    await expect(page.locator('input[type=file]')).toHaveAttribute('data-history-upload', 'retained');
    if (direction === 'back' && index === 1) await page.screenshot({ path: testInfo.outputPath(`history-back-draft-${width}.png`), fullPage: true });
  }
  expect(new URL(page.url()).searchParams.getAll('filter')).toEqual(['one', 'two']);
  expect(state.details).toBe(1);
  expect(state.submits).toBe(0);
  await page.getByRole('heading', { name: headings[2] }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: testInfo.outputPath(`history-draft-${width}.png`), fullPage: true });
  await page.goBack(); await selected(page, 1);
  await page.goBack(); await selected(page, 0);
  await page.goBack();
  await expect(page).toHaveURL(/\/project\/history\/reviews$/);
});

test('H29 mobile selector, cards, source and attachment links share one history', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await fixture(page);
  await page.goto(reviewPath);
  await selected(page, 0);
  await page.locator('.review-selected-details').getByRole('link', { name: /Checks/ }).click();
  await selected(page, 1);
  await page.getByRole('button', { name: 'Open sources', exact: true }).click();
  await selected(page, 2);
  await page.goBack(); await selected(page, 1);
  await page.goBack(); await selected(page, 0);
  await page.getByRole('button', { name: 'Open checks', exact: true }).click();
  await selected(page, 1);
  await page.getByRole('button', { name: 'Show documents' }).click();
  await page.getByRole('combobox', { name: 'Review document' }).focus();
  await page.getByRole('combobox', { name: 'Review document' }).press('ArrowDown');
  await page.locator('.ant-select-item-option[title="c · Sources"]').click();
  await selected(page, 2);
  await catalog(page, 0, true);
  await page.goBack(); await selected(page, 2);
  await page.goBack(); await selected(page, 1);
  await page.goForward(); await selected(page, 2);
});

test('H30 encoded shared URL survives reload and opens the same document in a fresh authorized context', async ({ page, browser, baseURL }, testInfo) => {
  await fixture(page);
  await page.goto(reviewPath);
  await selected(page, 0);
  await catalog(page, 1);
  const shared = page.url();
  await page.reload();
  await selected(page, 1);
  const other = await browser.newContext({ baseURL, viewport: { width: 1440, height: 900 } });
  try {
    await authorize(other, baseURL!);
    const recipient = await other.newPage();
    const state = await fixture(recipient);
    await recipient.goto(shared);
    await selected(recipient, 1);
    expect(state.requests.length).toBeGreaterThanOrEqual(1);
    expect([...new Set(state.requests)]).toEqual([ids[1]]);
    await recipient.screenshot({ path: testInfo.outputPath('shared-checks-desktop.png'), fullPage: true });
  } finally { await other.close(); }
});

test('H32 same-document actions, viewer settings and Retry add no history', async ({ page }) => {
  const state = await fixture(page);
  await page.goto(reviewPath);
  await selected(page, 0);
  state.failures.set('c', 503);
  await page.getByRole('button', { name: 'Show documents' }).click();
  await page.getByRole('navigation', { name: 'Review documents' }).getByRole('button', { name: /^Sources / }).click();
  await expect(page.getByText(/Could not load this document/)).toBeVisible();
  expect(new URL(page.url()).searchParams.get('document_id')).toBe('c');
  state.failures.delete('c');
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await selected(page, 2);
  await catalog(page, 2);
  await page.getByRole('button', { name: 'Same heading', exact: true }).click();
  await page.getByRole('button', { name: 'Document options' }).click();
  await page.getByRole('dialog', { name: 'Document options' }).getByText('Markdown', { exact: true }).click();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Hide documents' }).click();
  await page.getByRole('button', { name: 'Show documents' }).click();
  await page.goBack();
  // Viewer settings are reader state, so the preview can remain in source mode.
  await expect.poll(() => new URL(page.url()).searchParams.get('document_id')).toBe('a');
  await expect(page.getByLabel('Markdown source')).toContainText('Plan document');
  expect(state.requests.filter(id => id === 'c')).toHaveLength(2);
});

test('H33 Back aborts a pending load; its late completion cannot replace the destination', async ({ page }) => {
  const state = await fixture(page);
  let release!: () => void;
  state.pending.set(ids[1], new Promise<void>(resolve => { release = resolve; }));
  await page.goto(reviewPath);
  await selected(page, 0);
  await page.getByRole('button', { name: 'Open checks', exact: true }).click();
  await expect.poll(() => state.requests.includes(ids[1])).toBe(true);
  await expect(page.getByLabel('Loading document')).toBeVisible();
  await page.goBack(); await selected(page, 0);
  release();
  await expect.poll(() => state.completed.includes(ids[1])).toBe(true);
  // The unit race tests also force late success and rejection after cancellation.
  await selected(page, 0);
  await expect(page.getByRole('heading', { name: headings[1] })).toHaveCount(0);
  await page.goForward(); await selected(page, 1);
});

test('H34 missing document corrects once and stale or duplicate links require deliberate recovery', async ({ page }, testInfo) => {
  const state = await fixture(page);
  await page.goto('/project/history/reviews');
  await page.goto(link('removed'));
  await selected(page, 0);
  await expect(page.getByText(/linked document is unavailable/)).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/\/project\/history\/reviews$/);
  const before = state.requests.length;
  for (const invalid of [link('a', 'older-review'), link('a') + '&document_id=a']) {
    await page.goto(invalid);
    await expect(page.getByRole('button', { name: 'Open current review' })).toBeVisible();
    expect(new URL(page.url()).pathname + new URL(page.url()).search).toBe(invalid);
    await expect(page.getByLabel('Selected document')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0);
    expect(state.requests).toHaveLength(before);
    if (invalid.includes('older-review')) await page.screenshot({ path: testInfo.outputPath('stale-review-desktop.png'), fullPage: true });
  }
  await page.getByRole('button', { name: 'Open current review' }).click();
  await selected(page, 0);
  await page.goBack();
  await expect(page.getByText('This linked review is no longer current')).toBeVisible();
  expect(state.submits).toBe(0);
});

for (const failure of [403, 409, 'network'] as const) test(`H35 attached document failure ${failure} retains selected identity`, async ({ page }) => {
  const state = await fixture(page);
  state.failures.set(ids[1], failure);
  await page.goto(link(ids[1]));
  await expect(page.getByLabel('Selected document').getByRole('alert')).toContainText(failure === 'network' ? 'Failed to fetch' : 'Could not load this document');
  expect(new URL(page.url()).searchParams.get('document_id')).toBe(ids[1]);
  expect(state.requests.length).toBeGreaterThanOrEqual(1);
  expect([...new Set(state.requests)]).toEqual([ids[1]]);
  const beforeRetry = state.requests.length;
  await expect(page.getByText(/linked document is unavailable/)).toHaveCount(0);
  state.failures.delete(ids[1]);
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await selected(page, 1);
  expect(state.requests).toHaveLength(beforeRetry + 1);
  expect([...new Set(state.requests)]).toEqual([ids[1]]);
});
