import { expect } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { mkdirSync } from 'node:fs';
const captures = process.env.NAV_EVIDENCE_DIR || '/tmp/cortex-navigation-captures';
mkdirSync(captures, { recursive: true });
import { cellFixture } from './helpers/cells';
test.beforeEach(async ({ context, baseURL }) => {
  // Modified links open new tabs; shell fixtures and streams must cover those too.
  await context.addCookies([{ name: 'colony2:user:email', value: 'nav@example.com', url: baseURL! }]);
  await context.addInitScript(() => { if (!(window as any).navStreams) (window as any).EventSource = class { close() {} addEventListener() {} removeEventListener() {} }; });
  await context.route('**/api/projects', route => route.fulfill({ json: [{ tenant_id: 'navigation' }] }));
  await context.route(/\/api\/projects\/[^/]+\/admin$/, route => route.fulfill({ json: { revision: "r1", hooks: [], configs: [], git: [], secretReferences: [] } }));
  await context.route(/\/api\/projects\/[^/]+\/cells(?:\?.*)?$/, route => {
    const tenant = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[3]);
    return route.fulfill({ json: { cells: [cellFixture('platform', tenant)] } });
  });
  await context.route(/\/user-inputs\/pending$/, route => route.fulfill({ json: [] }));
  await context.route(/\/api\/projects\/[^/]+\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs: [] } }));
});

const destinations = [
  ['All', '/jobs'], ['platform', '/jobs?inbox=cell&cellId=cell-platform'], ['Archive', '/jobs?inbox=archive'],
] as const;
const desktop = (page: import('@playwright/test').Page) => page.locator('.desktop-navigation');
const inboxes = (page: import('@playwright/test').Page) => desktop(page).getByRole('navigation', { name: 'Inboxes' });

test('NAV desktop geometry retains page state and uses a route-local navigation choice', async ({ page }) => {
  await page.goto('/project/navigation/jobs');
  await expect(desktop(page)).toHaveCSS('width', '256px');
  const search = page.getByRole('textbox', { name: 'Search jobs' });
  await search.fill('Retain this');
  await search.evaluate(element => { (element as any).__navigationMount = 'original'; });
  const before = (await page.locator('.app-content').boundingBox())!;
  await page.getByRole('button', { name: 'Collapse navigation', exact: true }).click();
  await expect(desktop(page)).toHaveCSS('width', '76px');
  await expect(search).toHaveValue('Retain this');
  expect(await search.evaluate(element => (element as any).__navigationMount)).toBe('original');
  expect((await page.locator('.app-content').boundingBox())!.width - before.width).toBe(180);
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toHaveAttribute('aria-expanded', 'false');
  await page.screenshot({ animations: 'disabled', path: `${captures}/collapsed.png` });
  await page.reload();
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await expect(search).toHaveValue('Retain this');
  await page.getByRole('button', { name: 'Collapse navigation' }).focus();
  await page.keyboard.press('Enter');
  await expect(desktop(page)).toHaveCSS('width', '76px');
  await page.getByRole('button', { name: 'Expand navigation' }).click();
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await page.screenshot({ animations: 'disabled', path: `${captures}/expanded.png` });
});

test('NAV collapsed links, tooltips, selection and responsive return', async ({ page }) => {
  await page.goto('/project/navigation/jobs');
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  for (const [name, route] of destinations) {
    const link = inboxes(page).getByRole('link', { name, exact: true });
    await link.hover();
    await expect(page.getByRole('tooltip', { name, exact: true })).toBeVisible();
    await link.click();
    await expect(page).toHaveURL(`/project/navigation${route}`);
    await expect(link).toHaveAttribute('aria-current', 'page');
  }
  await page.setViewportSize({ width: 767, height: 800 });
  await page.getByRole('button', { name: 'Open navigation' }).click();
  await page.getByRole('dialog').getByRole('link', { name: 'All', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Open navigation' })).toBeFocused();
  await page.setViewportSize({ width: 768, height: 400 });
  await expect(desktop(page)).toHaveCSS('width', '76px');
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('compact drawer stays open on browser history between non-review pages', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/project/navigation/jobs');
  const opener = page.getByRole('button', { name: 'Open navigation' });
  const drawer = page.getByRole('dialog', { name: 'Navigation' });

  await opener.click();
  await drawer.getByRole('link', { name: 'Archive', exact: true }).click();
  await expect(page).toHaveURL(/\/project\/navigation\/jobs\?inbox=archive$/);
  await expect(drawer).toBeHidden();
  await opener.click();
  await expect(drawer).toBeVisible();

  await page.goBack();
  await expect(page).toHaveURL(/\/project\/navigation\/jobs$/);
  await expect(drawer.getByRole('link', { name: 'All', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(opener).toHaveAttribute('aria-expanded', 'true');
  await expect(drawer).toBeVisible();

  await page.goForward();
  await expect(page).toHaveURL(/\/project\/navigation\/jobs\?inbox=archive$/);
  await expect(drawer.getByRole('link', { name: 'Archive', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(opener).toHaveAttribute('aria-expanded', 'true');
  await expect(drawer).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('compact-drawer-after-forward.png'), animations: 'disabled' });

  await page.keyboard.press('Escape');
  await expect(drawer).toBeHidden();
  await expect(opener).toBeFocused();
});

for (const collapsed of [false, true]) test(`NAV all destinations retain mouse, keyboard and modified links (${collapsed})`, async ({ page, context }) => {
  await page.goto('/project/navigation/jobs');
  if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
  for (const [name, route] of destinations) {
    const link = inboxes(page).getByRole('link', { name, exact: true });
    await link.focus();
    if (collapsed) await expect(page.getByRole('tooltip', { name, exact: true })).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(`/project/navigation${route}`);
    await expect(link).toHaveAttribute('aria-current', 'page');
    await link.click();
    await expect(page).toHaveURL(`/project/navigation${route}`);
    const popupPromise = context.waitForEvent('page');
    await link.click({ modifiers: ['ControlOrMeta'] });
    const popup = await popupPromise;
    await expect(popup).toHaveURL(`/project/navigation${route}`);
    await popup.close();
  }
  // Native links participate in normal Tab navigation.
  await inboxes(page).getByRole('link', { name: 'platform', exact: true }).focus();
  await page.keyboard.press('Tab');
  await expect(inboxes(page).getByRole('link', { name: 'Archive', exact: true })).toBeFocused();
  expect(await inboxes(page).locator('a').evaluateAll(links => links.every(link => link.tabIndex === 0))).toBe(true);
  await expect(page.getByRole('link', { name: 'Settings', exact: true })).toHaveAttribute('href', '/project/navigation/admin');
});

test('NAV Space toggles once and tenant and retired-link routes keep current destinations', async ({ page }) => {
  await page.goto('/project/navigation/jobs');
  await expect(page.locator('.app-header .navigation-toggle')).toHaveCount(0);
  const toggle = page.locator('.navigation-toggle');
  await expect(toggle).toHaveText('');
  await expect(toggle.locator('[aria-label="menu-fold"]')).toHaveCount(1);
  await toggle.focus(); await page.keyboard.press('Space');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(toggle.locator('[aria-label="menu-unfold"]')).toHaveCount(1);
  await page.getByRole('searchbox', { name: 'Tenant ID' }).fill('second');
  await page.getByRole('button', { name: 'Use', exact: true }).click();
  await expect(page).toHaveURL('/project/second/jobs');
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await expect(inboxes(page).getByRole('link', { name: 'All', exact: true })).toHaveAttribute('href', '/project/second/jobs');
  await page.reload();
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await page.goto('/project/second/goals/a');
  await expect(page.getByText('Goal links have been retired', { exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Open All jobs' }).click();
  await expect(inboxes(page).getByRole('link', { name: 'All', exact: true })).toHaveAttribute('aria-current', 'page');
});

test('NAV denied reads and writes keep navigation usable in memory', async ({ page }) => {
  await page.addInitScript(() => {
    Storage.prototype.getItem = () => { throw new Error('read denied'); };
    Storage.prototype.setItem = () => { throw new Error('write denied'); };
  });
  await page.goto('/project/navigation/jobs');
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  await expect(desktop(page)).toHaveCSS('width', '76px');
  await page.getByRole('button', { name: 'Expand navigation' }).click();
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await page.reload();
  await expect(desktop(page)).toHaveCSS('width', '256px');
});
test('NAV badges update through activity events in both widths', async ({ page }) => {
  await page.addInitScript(() => {
    (window as any).navStreams = [];
    (window as any).EventSource = class {
      readyState = 1;
      listeners: Record<string, (event: any) => void> = {};
      constructor() { (window as any).navStreams.push(this); }
      close() {} removeEventListener() {}
      addEventListener(name: string, callback: (event: any) => void) { this.listeners[name] = callback; }
    };
  });
  await page.goto('/project/navigation/goals');
  await expect.poll(() => page.evaluate(() => (window as any).navStreams.length)).toBeGreaterThan(0);
  const navigation = page.locator('.desktop-navigation').getByRole('navigation', { name: 'Inboxes' });
  const all = navigation.getByRole('link', { name: 'All', exact: true });
  const cell = navigation.getByRole('link', { name: 'platform', exact: true });
  for (const collapsed of [false, true]) {
    if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
    for (const count of [0, 7, 120, 3]) {
      await page.evaluate(count => {
        const inputs = Array.from({ length: count }, (_, i) => ({
          id: String(i), client_payload_revision: 1, tenant_id: 'navigation', cell_id: 'cell-platform', cell_name: 'platform',
          cell_alias: 'platform', cell_version: 1, repo: 'github.com/acme/platform', ref: 'main',
        }));
        (window as any).navStreams.at(-1).listeners.inputs_changed({ data: JSON.stringify({ inputs }) });
      }, count);
      await expect(cell.locator('.inbox-badge')).toHaveCount(count ? 1 : 0);
      if (count) await expect(cell.locator('.inbox-badge')).toHaveAttribute('aria-label', `${count} pending responses`);
      await expect(navigation.getByRole('link', { name: 'Archive', exact: true }).locator('.inbox-badge')).toHaveCount(0);
      await expect(page.locator('.desktop-navigation')).toHaveCSS('width', collapsed ? '76px' : '256px');
      const badge = all.locator('.inbox-badge');
      if (!count) { await expect(badge).toHaveCount(0); continue; }
      await expect(badge).toHaveCount(1);
      await expect(badge).toHaveAttribute('aria-label', `${count} pending responses`);
      await expect(badge).toHaveText(String(count));
      const box = (await badge.boundingBox())!, rail = (await page.locator('.desktop-navigation').boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(rail.x);
      expect(box.x + box.width).toBeLessThanOrEqual(rail.x + rail.width);
      // Badge counts appear in the DOM before Ant Design's scale-in animation finishes.
      await expect.poll(() => badge.evaluate(el => { const r = el.getBoundingClientRect(); return el.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); })).toBe(true);
    }
    await page.evaluate(() => scrollTo(0, 0));
    await page.screenshot({ animations: 'disabled', path: `${captures}/${collapsed ? 'collapsed' : 'expanded'}-badge.png` });
  }
});


for (const collapsed of [false, true]) test(`NAV scroll, short desktop and drawer dismissal (${collapsed})`, async ({ page }) => {
  await page.goto('/project/navigation/jobs');
  if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
  const trigger = page.getByRole('button', { name: collapsed ? 'Expand navigation' : 'Collapse navigation' });
  await page.locator('.app-content').evaluate(el => (el as HTMLElement).style.minHeight = '2000px');
  await page.evaluate(() => scrollTo(0, 1000));
  await expect(trigger).toBeInViewport({ ratio: 1 });
  for (const [width, height] of [[768, 260], [1280, 300], [767, 800], [390, 844]]) {
    await page.setViewportSize({ width, height });
    await page.evaluate(() => scrollTo(0, 0));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    if (width >= 768) {
      await expect(trigger).toBeInViewport({ ratio: 1 });
      for (const [name] of destinations) {
        const link = inboxes(page).getByRole('link', { name, exact: true });
        await link.scrollIntoViewIfNeeded();
        const bounds = (await link.boundingBox())!;
        expect(bounds.y).toBeGreaterThanOrEqual(-1);
        expect(bounds.y + bounds.height).toBeLessThanOrEqual(height + 1);
        expect(await link.evaluate(el => { const r = el.getBoundingClientRect(); return el.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); }), `${name} at ${width} × ${height}`).toBe(true);
        if (name === 'platform' && width === 768) await page.screenshot({ animations: 'disabled', path: `${captures}/short-cell-${collapsed ? 'rail' : 'expanded'}.png` });
      }
      continue;
    }
    const opener = page.getByRole('button', { name: 'Open navigation' });
    await opener.click();
    const dialog = page.getByRole('dialog');
    for (const [name] of destinations) await expect(dialog.getByRole('link', { name, exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(opener).toBeFocused();
    await opener.click();
    await page.setViewportSize({ width: 768, height: 400 });
    await expect(dialog).not.toBeVisible();
    await expect(desktop(page)).toHaveCSS('width', collapsed ? '76px' : '256px');
  }
});

test.describe('touch', () => {
  test.use({ hasTouch: true });
  test('NAV all desktop icons accept touch', async ({ page }) => {
    await page.goto('/project/navigation/jobs');
    for (const collapsed of [false, true]) {
      if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).tap();
      for (const [name, route] of destinations) {
        await inboxes(page).getByRole('link', { name, exact: true }).tap();
        await expect(page).toHaveURL(`/project/navigation${route}`);
      }
    }
  });
});

test.describe('native zoom navigation', () => {
  test.use({ nativeZoom: true });
  test('NAV 200 percent uses full drawer then restores route choice', async ({ page }) => {
    await page.goto('/project/navigation/jobs');
    await page.getByRole('button', { name: 'Collapse navigation' }).click();
    await setNativeZoom(page, 2);
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await expect(page.getByRole('dialog').getByRole('link', { name: 'All', exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await captureNativeZoom(page, `${captures}/zoom-200.png`);
    await page.getByRole('dialog').getByRole('link', { name: 'platform', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Open navigation' })).toBeFocused();
    await setNativeZoom(page, 1);
    await expect(desktop(page)).toHaveCSS('width', '76px');
  });
});

for (const collapsed of [false, true]) test(`NAV login and tenant switching remain usable (${collapsed})`, async ({ page, context }) => {
  await page.goto('/project/navigation/jobs');
  if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
  await context.clearCookies();
  await page.reload();
  await page.getByPlaceholder('Email address').fill('returning@example.com');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await expect(desktop(page)).toHaveCSS('width', '256px');
  await page.getByRole('searchbox', { name: 'Tenant ID' }).fill('tenant-two');
  await page.getByRole('button', { name: 'Use', exact: true }).click();
  await expect(page).toHaveURL('/project/tenant-two/jobs');
  await expect(inboxes(page).getByRole('link', { name: 'All', exact: true })).toHaveAttribute('href', '/project/tenant-two/jobs');
  await expect(page.getByRole('link', { name: 'Settings', exact: true })).toHaveAttribute('href', '/project/tenant-two/admin');
  await expect(desktop(page)).toHaveCSS('width', '256px');
});
