import { test, expect, type Page, type BrowserContext } from '@playwright/test';
import { jobIdentity, mockCells } from './helpers/cells';

const id = '3M11h0hfoxIoOk6knxCN6DW9o0u';
const title = 'Build a reliable dashboard with a long title for a compact screen';
async function fixture(page: Page, context: BrowserContext, origin: string, jobId = id, route = 'goals') {
  await context.addCookies([{ name: 'colony2:user:email', value: 'copy@example.com', url: origin }]);
  await page.route('**/api/projects', response => response.fulfill({ json: [{ tenant_id: 'copy', name: 'Copy test' }] }));
  await page.route('**/api/projects/copy/user-inputs/pending', response => response.fulfill({ json: [] }));
  await page.route('**/api/projects/copy/user-inputs/stream', response => response.fulfill({ contentType: 'text/event-stream', body: 'data: []\n\n' }));
  await page.route('**/api/projects/copy/assistant/capabilities', response => response.fulfill({ json: { available: false } }));
  await mockCells(page, ['platform']);
  const job = { ...jobIdentity, job_id: jobId, goal_id: 'different-goal', prompt: title, status: 'READY', store: 'ACTIVE', created_at: '2026-11-12T00:00:00Z' };
  let listReads = 0; let detailReads = 0;
  await page.route('**/api/projects/copy/jobs?*', response => {
    listReads++;
    return response.fulfill({ json: { jobs: [job, { ...job, job_id: 'untouched', goal_id: 'other-goal', prompt: 'Another goal' }] } });
  });
  await page.route('**/api/projects/copy/jobs/*', response => { detailReads++; return response.fulfill({ json: job }); });
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await page.addInitScript(() => {
    (window as any).copyProhibitedCalls = [];
    if (navigator.clipboard) navigator.clipboard.readText = () => { (window as any).copyProhibitedCalls.push('read'); return Promise.reject(new Error('Application must not read')); };
    navigator.permissions.query = () => { (window as any).copyProhibitedCalls.push('query'); return Promise.reject(new Error('Application must not query')); };
  });
  // The independent clipboard verifier may own the foreground during setup.
  await page.bringToFront();
  await page.goto(`${origin}/project/copy/${route}`);
  const button = page.getByRole('button', { name: `Copy job ID ${jobId}`, exact: true });
  await expect(button).toBeVisible();
  const row = button.locator(page.viewportSize()!.width < 768 ? 'xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " goal-card ")][1]' : 'xpath=ancestor::tr');
  return { button, row, errors, reads: () => [listReads, detailReads] };
}
async function clipboardSurface(context: BrowserContext) {
  // Independent trustworthy origin; the HTTP application origin receives no grants.
  const origin = 'http://localhost:15173';
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin });
  const surface = await context.newPage();
  await surface.route(`${origin}/clipboard-check`, route => route.fulfill({ contentType: 'text/html', body: '<title>Independent clipboard check</title><textarea aria-label="Paste target"></textarea>' }));
  await surface.goto(`${origin}/clipboard-check`);
  await surface.evaluate(() => navigator.clipboard.writeText('sentinel-before-copy'));
  return {
    seed: async () => { await surface.bringToFront(); await surface.evaluate(() => navigator.clipboard.writeText('sentinel-before-copy')); },
    expectValue: async (value: string) => { await surface.bringToFront(); await expect.poll(() => surface.evaluate(() => navigator.clipboard.readText())).toBe(value); },
  };
}
function copyToast(page: Page) {
  return page.locator('.ant-message-notice-content').filter({ hasText: /^Job ID copied$/ });
}
async function expectCopyToast(page: Page) {
  const notice = copyToast(page);
  await expect(notice).toHaveCount(1);
  await expect(notice.locator('.ant-message-success')).toHaveText('Job ID copied');
  const announcement = page.getByRole('status').filter({ hasText: /^Job ID copied$/ });
  await expect(announcement).toHaveCount(1);
  await expect(announcement).toHaveAttribute('aria-live', 'polite');
  await expect(announcement).toHaveAttribute('aria-atomic', 'true');
  expect(await announcement.ariaSnapshot()).toBe('- status: Job ID copied');
  await expect(announcement.locator('.sr-only')).toHaveCSS('clip-path', 'inset(50%)');
  await expect(announcement.locator('.sr-only')).toHaveCSS('position', 'absolute');
  await expect(page.locator('[role="status"], [role="alert"], [aria-live="polite"], [aria-live="assertive"]').filter({ hasText: /^Job ID copied$/ })).toHaveCount(1);
}
async function refuseModern(page: Page) {
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: () => Promise.reject(new Error('Injected refusal')) } }));
}
for (const width of [1280, 390, 320]) {
  test(`JC-15/18–24 modern refusal and recovery at ${width}px`, async ({ page, context, baseURL }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const surface = await clipboardSurface(context);
    // Native modern success uses a scoped test grant; HTTP cases below get none.
    await context.grantPermissions(['clipboard-write'], { origin: baseURL! });
    const { button, row, errors, reads } = await fixture(page, context, baseURL!);
    await page.bringToFront();
    const initialURL = page.url(); const initialReads = reads();
    const checkbox = row.getByRole('checkbox', { name: `Select job ${id}`, exact: true }); await checkbox.check();
    const unchanged = async () => {
      expect(page.url()).toBe(initialURL); expect(reads()).toEqual(initialReads);
      await expect(checkbox).toBeChecked();
      await expect(row.getByRole('button', { name: 'Expand goal details' })).toHaveAttribute('aria-expanded', 'false');
      await expect(row.getByRole('status')).toHaveText('Job ID copied');
      expect(errors).toEqual([]); expect(await page.evaluate(() => (window as any).copyProhibitedCalls)).toEqual([]);
    };
    for (const activation of ['Enter', 'Space', 'click']) {
      await expect(copyToast(page)).toHaveCount(0);
      await surface.seed(); await page.bringToFront(); await button.focus();
      if (activation === 'click') await button.click(); else await page.keyboard.press(activation);
      await expectCopyToast(page); await expect(row.getByRole('status')).toHaveText('Job ID copied');
      await surface.expectValue(id); await page.bringToFront(); await unchanged();
    }
    await expect(copyToast(page)).toHaveCount(0);
    await surface.seed(); await page.bringToFront(); await refuseModern(page); await button.click();
    await expect(row.getByRole('status')).toHaveText('Could not copy job ID.');
    await expect(copyToast(page)).toHaveCount(0);
    const retry = row.getByRole('button', { name: 'Copy another way' });
    const field = row.getByRole('textbox', { name: 'Job ID', exact: true });
    await expect(field).toHaveValue(id); await expect(field).toHaveAttribute('readonly');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    if (width < 768) {
      expect((await retry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      const recovery = (await row.locator('.goal-copy-recovery').boundingBox())!;
      const summary = (await row.locator('.goal-card-summary').boundingBox())!;
      expect(recovery.y).toBeGreaterThanOrEqual(summary.y + summary.height);
      expect(recovery.width).toBeCloseTo(summary.width, 0);
    }
    await button.blur();
    await page.getByRole('heading', { name: 'Goals', exact: true }).hover();
    await expect(page.getByRole('tooltip')).toHaveCount(0);
    await page.screenshot({ path: testInfo.outputPath(`copy-recovery-${width}.png`), fullPage: true, animations: 'disabled' });
    await retry.focus(); await page.keyboard.press('Enter');
    await expectCopyToast(page); await expect(button).toBeFocused(); await expect(field).toHaveCount(0);
    await surface.expectValue(id); await page.bringToFront(); await unchanged();
    await expect(copyToast(page)).toHaveCount(0);
    // Deliberately fail the command; manual keyboard copying still uses the browser.
    await page.evaluate(() => { document.execCommand = () => false; });
    await button.click(); await retry.focus(); await page.keyboard.press('Space');
    await expect(copyToast(page)).toHaveCount(0);
    await expect(field).toBeFocused(); await expect(retry).toHaveCount(0);
    expect(await field.evaluate(element => { const input = element as HTMLTextAreaElement; return input.value.slice(input.selectionStart, input.selectionEnd); })).toBe(id);
    await surface.seed(); await page.bringToFront(); await field.focus(); await page.keyboard.press('Control+c');
    await surface.expectValue(id); await page.bringToFront();
    await expect(row.getByRole('status')).toHaveText('Could not copy job ID.');
    expect(errors).toEqual([]); expect(reads()).toEqual(initialReads); await expect(checkbox).toBeChecked();
    expect(await page.evaluate(() => (window as any).copyProhibitedCalls)).toEqual([]);
  });
}
for (const route of ['jobs', 'goals']) test(`JC-16/17 non-local HTTP copies on ${route}`, async ({ page, context }, testInfo) => {
  const surface = await clipboardSurface(context);
  const { button, row, errors, reads } = await fixture(page, context, 'http://cortex-copy.test:15173', id, route);
  await page.bringToFront();
  expect(await page.evaluate(() => ({ secure: isSecureContext, clipboard: typeof navigator.clipboard }))).toEqual({ secure: false, clipboard: 'undefined' });
  const before = { url: page.url(), reads: reads() };
  await button.click(); await expectCopyToast(page); await expect(row.getByRole('status')).toHaveText('Job ID copied');
  if (route === 'jobs') await page.screenshot({ path: testInfo.outputPath('copy-http-success.png'), fullPage: true, animations: 'disabled' });
  await surface.expectValue(id); await page.bringToFront();
  expect(page.url()).toBe(before.url); expect(reads()).toEqual(before.reads); expect(errors).toEqual([]);
  expect(await page.locator('textarea[aria-hidden="true"]').count()).toBe(0);
  expect(await page.evaluate(() => (window as any).copyProhibitedCalls)).toEqual([]);
});
for (const width of [1280, 390, 320]) test(`JC-20 long ID wraps without changing value at ${width}px`, async ({ page, context, baseURL }, testInfo) => {
  await page.setViewportSize({ width, height: 900 });
  const longId = `long-${id.repeat(8)}`;
  const { button, row } = await fixture(page, context, baseURL!, longId);
  await refuseModern(page); await button.click();
  const field = row.getByRole('textbox', { name: 'Job ID', exact: true });
  await expect(field).toHaveValue(longId);
  expect(await field.evaluate(element => element.scrollWidth <= element.clientWidth + 1 && element.scrollHeight <= element.clientHeight + 2)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await button.blur();
  await field.evaluate(element => element.scrollIntoView({ block: 'center' }));
  await page.mouse.move(0, 0);
  await expect(page.getByRole('tooltip')).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath(`copy-long-id-${width}.png`), fullPage: true, animations: 'disabled' });
});
test.describe('compact touch', () => {
  test.use({ viewport: { width: 390, height: 900 }, hasTouch: true });
  test('JC-15 touch compatibility copying preserves the other card', async ({ page, context }) => {
    const surface = await clipboardSurface(context);
    const { button, row } = await fixture(page, context, 'http://cortex-copy.test:15173');
    await page.bringToFront(); await button.tap();
    await expectCopyToast(page); await expect(row.getByRole('status')).toHaveText('Job ID copied'); await surface.expectValue(id);
    await expect(page.locator('.goal-card').filter({ has: page.getByRole('button', { name: 'Copy job ID untouched', exact: true }) }).getByRole('status')).toBeEmpty();
  });
});
test('JC-23 compatibility restores surviving input selection, focus and page scroll', async ({ page, context }) => {
  const { button } = await fixture(page, context, 'http://cortex-copy.test:15173');
  await page.evaluate(() => {
    document.body.style.minHeight = '2000px';
    const input = document.createElement('textarea'); input.id = 'previous-input'; input.value = 'previous selection';
    input.style.cssText = 'position:fixed;bottom:0;left:0;'; document.body.append(input);
    input.focus({ preventScroll: true }); input.setSelectionRange(2, 7, 'backward');
    window.scrollTo(0, 60);
  });
  await button.evaluate(element => element.addEventListener('mousedown', event => event.preventDefault()));
  await button.evaluate(element => element.addEventListener('click', () => { (window as any).copyScrollBefore = scrollY; }));
  await button.click();
  await expect(button).toBeVisible();
  expect(await page.evaluate(() => {
    const field = document.querySelector('#previous-input') as HTMLTextAreaElement;
    return { focused: document.activeElement === field, selection: [field.selectionStart, field.selectionEnd, field.selectionDirection], scroll: scrollY === (window as any).copyScrollBefore, temporaryCount: document.querySelectorAll('textarea[aria-hidden="true"]').length };
  })).toEqual({ focused: true, selection: [2, 7, 'backward'], scroll: true, temporaryCount: 0 });
});

test('JC-12 goal overview retains its existing exact-job navigation', async ({ page, context, baseURL }) => {
  await fixture(page, context, baseURL!);
  await page.goto(`${baseURL}/project/copy/goals/different-goal`);
  await expect(page.getByRole('link', { name: 'Open execution', exact: true })).toHaveAttribute('href', `/project/copy/jobs/${id}/story`);
  await expect(page.getByRole('button', { name: /^Copy job ID / })).toHaveCount(0);
});
