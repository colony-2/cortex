import { expect, type Page, type TestInfo } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { mkdir, readFile } from 'node:fs/promises';
import path from 'node:path';

const tenant = 'changes-browser';
const review = (job: string) => `/project/${tenant}/reviews/${job}`;
async function capture(page: Page, info: TestInfo, name: string, zoom = false) {
  const directory = process.env.CORTEX_REVIEW_CAPTURE_DIR;
  if (directory) await mkdir(directory, { recursive: true });
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().filter(a => a.effect?.getTiming().iterations !== Infinity).map(a => a.finished.catch(() => {})));
  });
  const filename = directory ? path.join(directory, name) : info.outputPath(name);
  if (zoom) await captureNativeZoom(page, filename);
  else await page.screenshot({ path: filename, fullPage: false });
}
async function download(page: Page) {
  const pending = page.waitForEvent('download');
  await page.getByRole('link', { name: 'Download saved diff' }).click();
  const file = await pending;
  expect(file.suggestedFilename()).toBe('diff_from_base.diff');
  return readFile((await file.path())!);
}
async function openChanges(page: Page, job: string) {
  await page.goto(review(job));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
}
async function noOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}
test.beforeEach(async ({ context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'changes@example.test', url: baseURL! }]);
});

test('v2 opens saved bytes directly and preserves documents, discussion, settings and original contribution', async ({ page, request }, info) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const oracle = await (await request.get('/fixture/v2')).json();
  const legacyRequests: string[] = [];
  const reads: string[] = [];
  page.on('request', r => { if (/\/changes(?:\/diff)?(?:\?|$)/.test(r.url())) legacyRequests.push(r.url()); if (r.url().includes('/workspace-diff')) reads.push(r.url()); });
  await page.goto(review('v2'));
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  const feedback = page.getByLabel('Question or requested change (required)', { exact: true });
  await feedback.fill('Can you explain these saved changes?');
  await page.getByLabel('Attach notes', { exact: true }).setInputFiles({ name: 'v2-notes.md', mimeType: 'text/markdown', buffer: Buffer.from('Review the saved diff') });
  await page.getByRole('button', { name: 'Review settings' }).click();
  await page.getByRole('dialog').getByLabel('Example count').fill('0');
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  await page.getByRole('button', { name: 'Document options' }).click();
  await page.getByText('Markdown', { exact: true }).click();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  expect(await download(page)).toEqual(Buffer.from(oracle.all));
  expect(legacyRequests).toEqual([]); expect(reads).toHaveLength(1); expect(new URL(reads[0]).search).toBe('');
  await expect(page.getByRole('combobox', { name: /From commit|Through commit/ })).toHaveCount(0);
  await expect(page.getByRole('group', { name: 'Commits oldest first' })).toHaveCount(0);
  await expect(page.getByText('Binary file changed. Its contents were not included in this saved diff.')).toBeVisible();
  await page.getByRole('button', { name: 'Hide documents', exact: true }).click();
  await page.getByRole('button', { name: 'Hide discussion', exact: true }).click();
  await page.getByRole('button', { name: 'Show documents', exact: true }).click();
  await page.getByRole('button', { name: 'Show discussion', exact: true }).click();
  await page.getByRole('button', { name: 'Refresh changes' }).click();
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  await expect(feedback).toHaveValue('Can you explain these saved changes?');
  await expect(page.getByText('v2-notes.md (21 bytes)', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: 'Documents', exact: true }).click();
  await expect(page.getByLabel('Markdown source')).toBeVisible();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await page.getByRole('tablist').scrollIntoViewIfNeeded();
  await capture(page, info, 'saved-diff-desktop.png');
  await page.getByText('Side by side', { exact: true }).click();
  await expect(page.getByLabel('Side by side diff').last()).toContainText('+change 4');
  expect(await download(page)).toEqual(Buffer.from(oracle.all));
  await page.getByRole('heading', { name: 'a/note.txt b/note.txt', exact: true }).scrollIntoViewIfNeeded();
  await capture(page, info, 'saved-diff-side-by-side.png');
  await page.getByRole('button', { name: 'Source', exact: true }).click();
  await expect(page.getByLabel('Document source')).toContainText('diff --git');
  await noOverflow(page);
  await page.getByRole('button', { name: 'Review settings' }).click();
  await expect(page.getByRole('dialog').getByLabel('Example count')).toHaveValue('0');
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  const submitted = page.waitForResponse(r => r.url().endsWith('/reviews/v2/respond') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Send · Ask a question' }).click();
  const receipt = await (await submitted).json();
  expect(receipt.receipt.request_id).toBe('v2-first');
  expect(receipt.response).toMatchObject({ direction: 'question', feedback: 'Can you explain these saved changes?', settings: {} });
  expect(receipt.response.attachments[0]).toMatchObject({ kind: 'stored' });
  expect(receipt.response.attachments[0].name.split('/').pop()).toBe('v2-notes.md');
});

test('empty, missing and document-free reviews have distinct messages and no invented downloads', async ({ page }, info) => {
  await openChanges(page, 'v2-empty');
  await expect(page.getByText('No file changes in this saved comparison.')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download saved diff' })).toHaveCount(1); // retained zero-byte file
  expect(await download(page)).toHaveLength(0);
  await openChanges(page, 'missing');
  await expect(page.getByText('No saved workspace diff is available for this recorded point.')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download saved diff' })).toHaveCount(0);
  await capture(page, info, 'saved-diff-unavailable.png');
});

test('metadata-only empty and unexpected text retain honest status and source', async ({ page }) => {
  await page.route('**/workspace-diff', async route => {
    const response = await route.fetch(); const data = await response.json();
    data.metadata.status = 'empty'; data.metadata.bytes = 0;
    data.metadata.workspace.head_commit = data.metadata.workspace.base_commit;
    delete data.metadata.source; delete data.download_url; data.patch = '';
    await route.fulfill({ response, json: data });
  });
  await openChanges(page, 'linear');
  await expect(page.getByText('No file changes in this saved comparison.')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download saved diff' })).toHaveCount(0);
  await page.unroute('**/workspace-diff');
  await page.route('**/workspace-diff', async route => {
    const response = await route.fetch(); const data = await response.json();
    data.patch = 'Unexpected saved comparison syntax';
    await route.fulfill({ response, json: data });
  });
  await page.getByRole('button', { name: 'Refresh changes' }).click();
  await expect(page.getByLabel('Document source')).toContainText('Unexpected saved comparison syntax');
  await expect(page.getByRole('link', { name: 'Download saved diff' })).toBeVisible();
});

test('merge and reverted comparisons read saved net bytes', async ({ page, request }) => {
  for (const job of ['merge', 'revert']) {
    const oracle = await (await request.get(`/fixture/${job}`)).json();
    await openChanges(page, job);
    if (oracle.all) await expect(page.getByLabel('Unified diff').last()).toBeVisible();
    else await expect(page.getByText('No file changes in this saved comparison.')).toBeVisible();
    expect(await download(page)).toEqual(Buffer.from(oracle.all));
  }
});

test('large saved preview is bounded and the original download remains complete', async ({ page, request }, info) => {
  const oracle = await (await request.get('/fixture/large')).json();
  await openChanges(page, 'large');
  await expect(page.getByText('Preview truncated. Search covers only loaded content. Download the complete original.')).toBeVisible();
  await expect(page.getByLabel('Document source')).toContainText('diff --git');
  expect(await download(page)).toEqual(Buffer.from(oracle.all));
  await capture(page, info, 'saved-diff-truncated.png');
});

test('refresh failures clear an old download and show upgrade guidance; retry recovers', async ({ page }) => {
  await openChanges(page, 'linear');
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  await page.route('**/workspace-diff', route => route.fulfill({ status: 501, json: { error: 'Upgrade the inspection helper to read saved workspace diffs', diagnostic: { version: 1, code: 'unsupported' } } }));
  await page.getByRole('button', { name: 'Refresh changes' }).click();
  await expect(page.getByText('Upgrade the inspection helper to read saved workspace diffs')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download saved diff' })).toHaveCount(0);
  await page.unroute('**/workspace-diff');
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
});

test('obsolete refresh responses cannot replace the current preview or download', async ({ page }) => {
  await openChanges(page, 'linear'); await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let count = 0;
  await page.route('**/workspace-diff', async route => {
    if (++count === 1) { const response = await route.fetch(); await gate; await route.fulfill({ response, json: { ...(await response.json()), patch: 'OBSOLETE SAVED PATCH' } }).catch(() => {}); }
    else await route.continue();
  });
  await page.getByRole('button', { name: 'Refresh changes' }).click();
  await expect.poll(() => count).toBe(1);
  await expect(page.getByRole('link', { name: 'Download saved diff' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Refresh changes' }).click();
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  release(); await page.unrouteAll({ behavior: 'wait' });
  await expect(page.getByText('OBSOLETE SAVED PATCH')).toHaveCount(0);
});

test('successor discovery retains the old draft while saved changes advance, then requires opening the new request', async ({ page, request }, info) => {
  await page.setViewportSize({ width: 1360, height: 1000 });
  const oracle = await (await request.get('/fixture/moving')).json();
  const legacyRequests: string[] = [];
  page.on('request', r => { if (/\/changes(?:\/diff)?(?:\?|$)/.test(r.url())) legacyRequests.push(r.url()); });
  await openChanges(page, 'moving');
  const response = page.getByRole('textbox', { name: 'Your response', exact: true });
  await response.fill('Old draft');
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  expect(await download(page)).not.toEqual(Buffer.from(oracle.all));
  expect(new URL(page.url()).searchParams.get('request_id')).toBe('moving-first');

  // Delay the real successor details after discovery, without manufacturing its
  // identity or the saved workspace response. The old reader must stay readable.
  let releaseSuccessor!: () => void;
  const successorGate = new Promise<void>(resolve => { releaseSuccessor = resolve; });
  let successorReads = 0;
  await page.route(`**/api/projects/${tenant}/user-inputs/moving`, async route => {
    const details = await route.fetch();
    if (details.status() === 200 && (await details.json()).form.request_id === 'moving-second') {
      successorReads++;
      await successorGate;
    }
    await route.fulfill({ response: details });
  });
  try {
    const accepted = await page.request.post(`/api/projects/${tenant}/reviews/moving/respond`, {
      data: { request_id: 'moving-first', submission_id: 'advance-request', fields: { response: 'Continue' } },
    });
    expect(accepted.ok(), await accepted.text()).toBeTruthy();
    const advanced = await request.post('/fixture/moving/advance');
    expect(advanced.ok(), await advanced.text()).toBeTruthy();
    await expect.poll(() => successorReads).toBe(1);
    await expect(response).toBeDisabled();
    await expect(response).toHaveValue('Old draft');
    await page.getByRole('button', { name: 'Refresh changes', exact: true }).click();
    await expect(page.getByLabel('Unified diff').last()).toBeVisible();
    expect(await download(page)).toEqual(Buffer.from(oracle.all));
    await expect(response).toHaveValue('Old draft');
    await response.scrollIntoViewIfNeeded();
    await capture(page, info, 'input-successor-saved-diff-pending.png');
  } finally {
    releaseSuccessor();
  }
  await expect(page.getByText('This linked review is no longer current')).toBeVisible();
  expect(successorReads).toBe(1);
  await expect(page.getByRole('tab', { name: 'Changes', exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByText('This linked review is no longer current')).toBeVisible();
  await page.getByRole('button', { name: 'Open current review', exact: true }).click();
  await expect(response).toBeEnabled();
  await expect(response).toHaveValue('');
  expect(new URL(page.url()).searchParams.get('request_id')).toBe('moving-second');
  expect(successorReads).toBe(2); // One read per page lifetime, reused on explicit open.
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByLabel('Unified diff').last()).toBeVisible();
  expect(await download(page)).toEqual(Buffer.from(oracle.all));
  expect(legacyRequests).toEqual([]);
  await response.scrollIntoViewIfNeeded();
  await capture(page, info, 'input-successor-saved-diff-current.png');
});

test.describe('phone', () => {
  test.use({ hasTouch: true, isMobile: true });
  test('touch layouts preserve the composer without horizontal page overflow', async ({ page, request }, info) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const oracle = await (await request.get('/fixture/v2-paged')).json();
    await page.goto(review('v2-paged'));
    await page.getByRole('button', { name: 'Ask a question', exact: true }).tap();
    await page.getByLabel('Question or requested change (required)', { exact: true }).fill('Explain the saved changes.');
    await page.getByRole('tab', { name: 'Changes', exact: true }).tap();
    await expect(page.getByLabel('Unified diff').last()).toBeVisible();
    expect(await download(page)).toEqual(Buffer.from(oracle.all));
    await page.locator('.changes-preview').evaluate(el => el.scrollIntoView({ block: 'start' }));
    await capture(page, info, 'saved-diff-phone.png');
    await page.getByText('Side by side', { exact: true }).tap();
    await expect(page.getByLabel('Side by side diff').last()).toBeVisible();
    await noOverflow(page);
    await page.getByRole('tab', { name: 'Documents', exact: true }).tap();
    await expect(page.getByLabel('Question or requested change (required)', { exact: true })).toHaveValue('Explain the saved changes.');
  });
});
test.describe('native zoom', () => {
  test.use({ nativeZoom: true });
  test('200% zoom and keyboard retain readable diff controls', async ({ page }, info) => {
    await openChanges(page, 'v2-zoom');
    await expect(page.getByLabel('Unified diff').last()).toBeVisible();
    await setNativeZoom(page, 2);
    await page.getByText('Side by side', { exact: true }).click();
    await expect(page.getByLabel('Side by side diff').last()).toBeVisible();
    const refresh = page.getByRole('button', { name: 'Refresh changes' });
    await refresh.focus(); await refresh.press('Enter');
    await expect(page.getByLabel('Side by side diff').last()).toBeVisible();
    await page.locator('.changes-preview').evaluate(el => el.scrollIntoView({ block: 'start' }));
    await capture(page, info, 'saved-diff-zoom.png', true);
    await noOverflow(page);
  });
});
