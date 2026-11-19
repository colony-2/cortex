import { expect, type Page, type TestInfo } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { mkdir, readFile } from 'node:fs/promises';
import path from 'node:path';

const tenant = 'changes-browser';
const review = (job: string) => `/project/${tenant}/reviews/${job}`;
async function capture(page: Page, info: TestInfo, name: string) {
  const directory = process.env.CORTEX_REVIEW_CAPTURE_DIR;
  if (directory) await mkdir(directory, { recursive: true });
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().filter(animation => animation.effect?.getTiming().iterations !== Infinity)
      .map(animation => animation.finished.catch(() => {})));
  });
  await page.screenshot({ path: directory ? path.join(directory, name) : info.outputPath(name), fullPage: false });
}
async function download(page: Page) {
  const pending = page.waitForEvent('download');
  await page.getByText('Download patch', { exact: true }).click();
  const file = await pending;
  expect(file.suggestedFilename()).toBe('changes.patch');
  return readFile((await file.path())!, 'utf8');
}
async function touchEndpoint(page: Page, name: string, option: string) {
  const input = page.getByRole('combobox', { name, exact: true });
  const control = page.locator('.ant-select').filter({ has: input });
  await control.evaluate(el => el.scrollIntoView({ block: 'center' }));
  await control.tap();
  await expect(input).toHaveAttribute('aria-expanded', 'true');
  // Let popup placement and its opening animation settle before a touch tap.
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().filter(animation => animation.effect?.getTiming().iterations !== Infinity)
      .map(animation => animation.finished.catch(() => {})));
  });
  const list = await input.getAttribute('aria-controls');
  await page.locator('.ant-select-dropdown').filter({ has: page.locator(`[id="${list}"]`) }).getByTitle(option, { exact: true }).tap();
  await expect(input).toHaveAttribute('aria-expanded', 'false');
}
test.beforeEach(async ({ context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'changes@example.test', url: baseURL! }]);
});

test('real persisted fast-forward changes, ranges, keyboard, documents and original response', async ({ page, request }, info) => {
  await page.setViewportSize({ width: 1360, height: 1000 });
  const oracle = await (await request.get('/fixture/linear')).json();
  await page.goto(review('linear'));
  await expect(page.getByRole('tab', { name: 'Documents', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.getByRole('textbox', { name: 'Your response', exact: true }).fill('Keep this response while inspecting commits.');
  await page.locator('input[type=file]').setInputFiles({ name: 'response.txt', mimeType: 'text/plain', buffer: Buffer.from('Supporting response') });
  await page.getByRole('button', { name: 'Document options' }).click();
  await page.getByText('Markdown', { exact: true }).click();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('4 commits since base')).toBeVisible();
  await expect(page.getByLabel('Unified diff')).toHaveCount(2); // binary metadata plus text file
  expect(await download(page)).toBe(oracle.all);
  const rows = page.getByRole('group', { name: 'Commits oldest first' }).getByRole('button');
  await rows.nth(1).click();
  await expect(page.getByRole('status')).toHaveText('Selected commits 2 through 2 · net changes');
  expect(await download(page)).toBe(oracle.single[1]);
  await rows.nth(3).click({ modifiers: ['Shift'] });
  await expect(page.getByRole('status')).toHaveText('Selected commits 2 through 4 · net changes');
  expect(await download(page)).toBe(oracle.range);
  await page.getByText('Side by side', { exact: true }).click();
  await page.getByRole('tablist').evaluate(el => el.scrollIntoView({ block: 'start' }));
  await capture(page, info, 'changes-desktop-range.png');
  await page.getByRole('heading', { name: 'a/note.txt b/note.txt', exact: true }).scrollIntoViewIfNeeded();
  await capture(page, info, 'changes-desktop-diff.png');
  await rows.nth(0).click({ modifiers: ['Shift'] });
  await expect(page.getByRole('status')).toHaveText('Selected commits 1 through 2 · net changes');
  await rows.nth(0).click();
  await rows.nth(0).press('Shift+ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(page.getByRole('status')).toHaveText('Selected commits 1 through 2 · net changes');
  await rows.nth(1).press('ArrowDown');
  await expect(rows.nth(2)).toBeFocused();
  await rows.nth(2).press('Enter');
  await expect(page.getByRole('status')).toHaveText('Selected commits 3 through 3 · net changes');
  await page.getByRole('button', { name: 'Source', exact: true }).click();
  await expect(page.getByLabel('Document source')).toContainText('diff --git');
  await page.getByRole('tab', { name: 'Documents', exact: true }).click();
  await expect(page.getByLabel('Markdown source')).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Your response', exact: true })).toHaveValue('Keep this response while inspecting commits.');
  await expect(page.getByText('response.txt', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('Selected commits 3 through 3 · net changes');
  await expect(page.getByLabel('Document source')).toBeVisible();
  const submitted = page.waitForResponse(response => response.url().endsWith('/reviews/linear/respond') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Submit', exact: true }).click();
  const receipt = await (await submitted).json();
  expect(receipt.receipt.request_id).toBe('linear-first');
  expect(receipt.receipt.actor.id).toBe('changes@example.test');
  expect(receipt.fields).toMatchObject({ response: 'Keep this response while inspecting commits.', attachment: { kind: 'stored', name: 'response.txt' } });
});

test.describe('touch device', () => {
test.use({ hasTouch: true, isMobile: true });
test('phone touch endpoints, no horizontal page overflow and readable diff controls', async ({ page, request }, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const oracle = await (await request.get('/fixture/paged')).json();
  await page.goto(review('paged'));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('52 commits since base')).toBeVisible();
  await touchEndpoint(page, 'From commit', `2. change 2 | μ (${oracle.commits[1].slice(0, 8)})`);
  await touchEndpoint(page, 'Through commit', `4. change 4 | μ (${oracle.commits[3].slice(0, 8)})`);
  await expect(page.getByRole('status')).toHaveText('Selected commits 2 through 4 · net changes');
  expect(await download(page)).toBe(oracle.range);
  await page.getByRole('tablist').evaluate(el => el.scrollIntoView({ block: 'start' }));
  await capture(page, info, 'changes-phone.png');
  await page.getByText('Selected commit details', { exact: true }).tap();
  await expect(page.locator('details[open]').filter({ hasText: 'Selected commit details' })).toContainText(oracle.commits[1]);
  await page.getByRole('heading', { name: 'a/note.txt b/note.txt', exact: true }).scrollIntoViewIfNeeded();
  await capture(page, info, 'changes-phone-diff.png');
  // A different displayed metadata page retains the full-sequence anchor.
  await page.getByTitle('2', { exact: true }).click();
  await page.getByRole('button', { name: /^52\. change 52/ }).click({ modifiers: ['Shift'] });
  await expect(page.getByRole('status')).toHaveText('Selected commits 2 through 52 · net changes');
  await page.getByRole('button', { name: 'All changes', exact: true }).click();
  await page.getByRole('tablist').evaluate(el => el.scrollIntoView({ block: 'start' }));
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
});
});

for (const endpoint of ['From commit', 'Through commit']) {
  test(`keyboard range selection survives a distant ${endpoint} endpoint`, async ({ page }, info) => {
    await page.setViewportSize({ width: 1360, height: 1000 });
    await page.goto(review('paged'));
    await page.getByRole('tab', { name: 'Changes', exact: true }).click();
    await expect(page.getByText('52 commits since base')).toBeVisible();
    const selector = page.getByRole('combobox', { name: endpoint, exact: true });
    await selector.focus();
    await selector.press('ArrowDown');
    await expect(selector).toHaveAttribute('aria-expanded', 'true');
    // The open list starts on the first option; ArrowUp wraps to the last.
    await selector.press('ArrowUp');
    await selector.press('Enter');
    await expect(page.getByRole('status')).toHaveText('Selected commits 52 through 52 · net changes');
    await expect(selector).toBeFocused();
    const last = page.getByRole('button', { name: /^52\. change 52/ });
    await expect(last).toHaveAttribute('tabindex', '0');
    const group = page.getByRole('group', { name: 'Commits oldest first' });
    await expect(group.locator('button[tabindex="0"]')).toHaveCount(1);
    await page.keyboard.press('Tab');
    if (endpoint === 'From commit') {
      await expect(page.getByRole('combobox', { name: 'Through commit', exact: true })).toBeFocused();
      await page.keyboard.press('Tab');
    }
    await expect(last).toBeFocused();
    await page.keyboard.press('Shift+ArrowUp');
    await expect(page.getByRole('button', { name: /^51\. change 51/ })).toBeFocused();
    await expect(page.getByRole('status')).toHaveText('Selected commits 51 through 52 · net changes');
    await page.keyboard.press('Shift+ArrowUp');
    await expect(page.getByRole('button', { name: /^50\. change 50/ })).toBeFocused();
    await expect(page.getByRole('status')).toHaveText('Selected commits 50 through 52 · net changes');
    await expect(group.locator('button[tabindex="0"]')).toHaveCount(1);
    // Plain arrows move focus across a page boundary without changing the range.
    await page.keyboard.press('ArrowDown');
    await expect(page.getByRole('button', { name: /^51\. change 51/ })).toBeFocused();
    await expect(page.getByRole('status')).toHaveText('Selected commits 50 through 52 · net changes');
    await expect(page.getByText('Download patch', { exact: true })).toBeVisible();
    if (endpoint === 'Through commit') {
      await group.evaluate(el => el.scrollIntoView({ block: 'center' }));
      await capture(page, info, 'changes-keyboard-page-range.png');
    }
  });
}

test('unsupported merge, reverted net patch, zero commits and missing closure remain distinct', async ({ page }, info) => {
  await page.goto(review('merge'));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('Unsupported history: merge commit in captured range')).toBeVisible();
  await expect(page.getByRole('group', { name: 'Commits oldest first' })).toHaveCount(0);
  await expect(page.getByLabel('Unified diff').first()).toBeVisible();
  await page.getByLabel('Job changes').scrollIntoViewIfNeeded();
  await capture(page, info, 'changes-unsupported-history.png');
  await page.goto(review('revert'));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('4 commits since base')).toBeVisible();
  await expect(page.getByText('No net file changes.')).toBeVisible();
  await page.goto(review('empty'));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('No commits since base.')).toBeVisible();
  await page.goto(review('missing'));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('missing_closure');
  await expect(page.getByRole('textbox', { name: 'Your response', exact: true })).toBeVisible();
  await expect(page.getByText('No commits since base.')).toHaveCount(0);
});

test('limited patch preview offers a complete Git-matching download', async ({ page, request }) => {
  const oracle = await (await request.get('/fixture/large')).json();
  await page.goto(review('large'));
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('Preview truncated. Search covers only loaded content. Download the complete original.')).toBeVisible();
  await expect(page.getByText('Structured preview limit exceeded or input incomplete; showing bounded source.')).toBeVisible();
  expect(await download(page)).toBe(oracle.all);
});

test('workspace advancement rejects old selections, refreshes current history and resets a new review occurrence', async ({ page, request }) => {
  await page.goto(review('moving'));
  await page.getByRole('textbox', { name: 'Your response', exact: true }).fill('Old draft');
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('2 commits since base')).toBeVisible();
  const accepted = await request.post(`/api/projects/${tenant}/reviews/moving/respond`, { headers: { Cookie: 'colony2:user:email=changes%40example.test' }, data: { request_id: 'moving-first', submission_id: 'advance-request', fields: { response: 'Continue' } } });
  expect(accepted.ok(), await accepted.text()).toBeTruthy();
  const advanced = await request.post('/fixture/moving/advance');
  expect(advanced.ok(), await advanced.text()).toBeTruthy();
  await page.getByRole('button', { name: /^1\. change 1/ }).click();
  await expect(page.getByText('Recorded workspace changed. Refresh changes to view its latest status.')).toBeVisible();
  await page.getByRole('button', { name: 'Refresh changes', exact: true }).first().click();
  await expect(page.getByText('4 commits since base')).toBeVisible();
  await expect(page.getByRole('status')).toHaveText('All changes · base to head net changes');
  await page.reload();
  // Current main requires deliberately opening a successor native review.
  await expect(page.getByText('This linked review is no longer current')).toBeVisible();
  await page.getByRole('button', { name: 'Open current review', exact: true }).click();
  await expect(page.getByRole('tab', { name: 'Documents', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('textbox', { name: 'Your response', exact: true })).toHaveValue('');
});

test.describe('browser zoom', () => {
  test.use({ nativeZoom: true });
  test('v2 at 200% native zoom retains selection controls, full source identity and bounded diff scrolling', async ({ page, baseURL }, info) => {
    await page.goto(new URL(review('v2'), baseURL).href);
    await setNativeZoom(page, 2);
    await page.getByRole('tab', { name: 'Changes', exact: true }).click();
    await expect(page.getByText('4 commits since base')).toBeVisible();
    await page.getByRole('button', { name: /^1\. change 1/ }).click();
    await expect(page.getByRole('combobox', { name: 'Patch file' })).toBeVisible();
    await page.getByText('Selected commit details', { exact: true }).click();
    await page.getByText('Recorded source', { exact: true }).click();
    await expect(page.getByRole('combobox', { name: 'From commit' })).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
    await page.getByLabel('Job changes').getByRole('status').evaluate(el => el.scrollIntoView({ block: 'start' }));
    const directory = process.env.CORTEX_REVIEW_CAPTURE_DIR;
    if (directory) await mkdir(directory, { recursive: true });
    await captureNativeZoom(page, directory ? path.join(directory, 'changes-v2-zoom.png') : info.outputPath('changes-v2-zoom.png'));
  });
});

async function v2Open(page: Page, job = 'v2') {
  await page.goto(review(job));
  await expect(page.getByRole('tab', { name: 'Documents', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('heading', { name: 'Changes to review', exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Discussion' }).getByRole('heading', { name: 'Ready to proceed?' })).toBeVisible();
}
async function noOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}

test('v2 reader integrates commit ranges, pane controls, document history and original contribution', async ({ page, request }, info) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const oracle = await (await request.get('/fixture/v2')).json();
  await v2Open(page);
  await page.getByRole('button', { name: 'Ask a question', exact: true }).click();
  const feedback = page.getByLabel('Question or requested change (required)', { exact: true });
  await feedback.fill('Can you explain commits 2 through 4?');
  await page.getByLabel('Attach notes', { exact: true }).setInputFiles({ name: 'v2-notes.md', mimeType: 'text/markdown', buffer: Buffer.from('Compare the selected range') });
  await page.getByRole('button', { name: 'Review settings' }).click();
  await page.getByRole('dialog').getByLabel('Example count').fill('0');
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  await page.getByRole('button', { name: 'Document options' }).click();
  await page.getByText('Markdown', { exact: true }).click();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('4 commits since base')).toBeVisible();
  expect(await download(page)).toBe(oracle.all);
  const rows = page.getByRole('group', { name: 'Commits oldest first' }).getByRole('button');
  await rows.nth(1).click();
  expect(await download(page)).toBe(oracle.single[1]);
  await rows.nth(3).click({ modifiers: ['Shift'] });
  await expect(page.getByLabel('Job changes').getByRole('status')).toHaveText('Selected commits 2 through 4 · net changes');
  expect(await download(page)).toBe(oracle.range);
  await page.getByRole('button', { name: 'Hide documents', exact: true }).click();
  await page.getByRole('button', { name: 'Hide discussion', exact: true }).click();
  await expect(page.getByLabel('Job changes')).toBeVisible();
  await page.getByRole('button', { name: 'Show documents', exact: true }).click();
  await page.getByRole('button', { name: 'Show discussion', exact: true }).click();
  await expect(feedback).toHaveValue('Can you explain commits 2 through 4?');
  await expect(page.getByText('v2-notes.md (26 bytes)', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: 'Documents', exact: true }).click();
  await expect(page.getByLabel('Markdown source')).toBeVisible();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByLabel('Job changes').getByRole('status')).toContainText('2 through 4');
  await page.getByRole('navigation', { name: 'Review documents' }).getByRole('button').nth(1).click();
  await expect(page.getByRole('tab', { name: 'Documents', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect.poll(() => new URL(page.url()).searchParams.get('document_id')).toBe('notes');
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await page.goBack();
  await expect.poll(() => new URL(page.url()).searchParams.get('document_id')).toBe('overview');
  await expect(page.getByRole('tab', { name: 'Documents', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByLabel('Job changes').getByRole('status')).toContainText('2 through 4');
  await page.getByRole('tablist').scrollIntoViewIfNeeded();
  await capture(page, info, 'changes-v2-desktop-layout.png');
  await page.locator('.changes-controls').evaluate(el => el.scrollIntoView({ block: 'start' }));
  await capture(page, info, 'changes-v2-desktop-range.png');
  await page.getByText('Side by side', { exact: true }).click();
  await expect(page.getByLabel('Side by side diff').last()).toBeVisible();
  await page.getByText('Unified', { exact: true }).click();
  await page.getByRole('heading', { name: 'a/note.txt b/note.txt', exact: true }).scrollIntoViewIfNeeded();
  await capture(page, info, 'changes-v2-desktop-diff.png');
  await noOverflow(page);
  await page.getByRole('button', { name: 'Review settings' }).click();
  await expect(page.getByRole('dialog').getByLabel('Example count')).toHaveValue('0');
  await page.getByRole('button', { name: 'Done', exact: true }).click();
  const submitted = page.waitForResponse(response => response.url().endsWith('/reviews/v2/respond') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Send · Ask a question' }).click();
  const receipt = await (await submitted).json();
  expect(receipt.receipt.request_id).toBe('v2-first');
  expect(receipt.receipt.actor.id).toBe('changes@example.test');
  expect(receipt.response).toMatchObject({ direction: 'question', feedback: 'Can you explain commits 2 through 4?', settings: {} });
  expect(receipt.response.attachments[0]).toMatchObject({ kind: 'stored' });
  expect(receipt.response.attachments[0].name.split('/').pop()).toBe('v2-notes.md');
  await expect(page.getByRole('region', { name: 'Discussion' }).getByText('Response pending', { exact: true })).toBeVisible();
});

test.describe('v2 touch device', () => {
  test.use({ hasTouch: true, isMobile: true });
  test('v2 phone keeps tabs and touch ranges usable with the response composer', async ({ page, request }, info) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const oracle = await (await request.get('/fixture/v2-paged')).json();
    await v2Open(page, 'v2-paged');
    await page.getByRole('button', { name: 'Ask a question', exact: true }).tap();
    await page.getByLabel('Question or requested change (required)', { exact: true }).fill('Explain this range.');
    await page.getByRole('tab', { name: 'Changes', exact: true }).tap();
    await expect(page.getByText('52 commits since base')).toBeVisible();
    await touchEndpoint(page, 'From commit', `2. change 2 | μ (${oracle.commits[1].slice(0, 8)})`);
    await touchEndpoint(page, 'Through commit', `4. change 4 | μ (${oracle.commits[3].slice(0, 8)})`);
    expect(await download(page)).toBe(oracle.range);
    await page.locator('.changes-controls').evaluate(el => el.scrollIntoView({ block: 'start' }));
    await capture(page, info, 'changes-v2-phone-range.png');
    await page.getByRole('heading', { name: 'a/note.txt b/note.txt', exact: true }).scrollIntoViewIfNeeded();
    await capture(page, info, 'changes-v2-phone-diff.png');
    await noOverflow(page);
    await expect(page.getByRole('button', { name: 'Send · Ask a question' })).toBeInViewport();
    await page.getByRole('tab', { name: 'Documents', exact: true }).tap();
    await expect(page.getByLabel('Question or requested change (required)', { exact: true })).toHaveValue('Explain this range.');
    await page.getByRole('tab', { name: 'Changes', exact: true }).tap();
    await expect(page.getByLabel('Job changes').getByRole('status')).toContainText('2 through 4');
  });
});


test('v2 without documents still exposes Changes and distinguishes zero commits', async ({ page }) => {
  await page.goto(review('v2-empty'));
  await expect(page.getByText('No documents attached')).toBeVisible();
  await page.getByRole('tab', { name: 'Changes', exact: true }).click();
  await expect(page.getByText('No commits since base.')).toBeVisible();
  await expect(page.getByText('No net file changes.')).toBeVisible();
  await expect(page.getByRole('group', { name: 'Commits oldest first' })).toHaveCount(0);
});
