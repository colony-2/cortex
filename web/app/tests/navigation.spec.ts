import { expect } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';
import { mkdirSync } from 'node:fs';
const captures = process.env.NAV_EVIDENCE_DIR || '/tmp/cortex-navigation-captures';
mkdirSync(captures, { recursive: true });
import { mockCells } from './helpers/cells';
test.beforeEach(async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'nav@example.com', url: baseURL! }]);
  await page.addInitScript(() => { if (!(window as any).navStreams) (window as any).EventSource = class { close() {} addEventListener() {} removeEventListener() {} }; });
  await page.route('**/api/projects', route => route.fulfill({ json: [{ tenant_id: 'navigation' }] }));
  await page.route(/\/api\/projects\/[^/]+\/admin$/, route => route.fulfill({ json: { revision: "r1", hooks: [], configs: [], git: [], secretReferences: [] } }));
  await mockCells(page, ['platform']);
  await page.route(/\/user-inputs\/pending$/, route => route.fulfill({ json: [] }));
  await page.route(/\/api\/projects\/[^/]+\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs: [] } }));
});
test('desktop geometry, page state, accessible trigger and preference', async ({ page }) => {
  await page.goto('/project/navigation/goals');
  const sidebar = page.locator('.desktop-navigation');
  await expect(sidebar).toHaveCSS('width', '220px');
  const search = page.getByRole('textbox', { name: 'Search goals' });
  await search.fill('Retain this');
  await search.evaluate(element => { (element as any).__navigationMount = 'original'; });
  const before = await page.locator('.app-content').boundingBox();
  await page.getByRole('button', { name: 'Collapse navigation', exact: true }).click();
  await expect(sidebar).toHaveCSS('width', '80px');
  await expect(search).toHaveValue('Retain this');
  expect(await search.evaluate(element => (element as any).__navigationMount)).toBe('original');
  expect((await page.locator('.app-content').boundingBox())!.width - before!.width).toBe(140);
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toHaveAttribute('aria-expanded', 'false');
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ animations: 'disabled', path: `${captures}/collapsed.png` });
  await page.reload();
  await expect(sidebar).toHaveCSS('width', '80px');
  await page.getByRole('button', { name: 'Expand navigation' }).focus();
  await page.keyboard.press('Enter');
  await expect(sidebar).toHaveCSS('width', '220px');
  await page.evaluate(() => scrollTo(0, 0));
  await page.screenshot({ animations: 'disabled', path: `${captures}/expanded.png` });
});
test('collapsed links, tooltips, selection and responsive return', async ({ page }) => {
  await page.goto('/project/navigation/goals');
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  const menu = page.locator('.desktop-navigation .ant-menu');
  for (const name of ['Reviews', 'Cells', 'Admin', 'Goals']) {
    const link = menu.getByRole('link', { name, exact: true });
    await link.hover();
    await expect(page.getByRole('tooltip', { name, exact: true })).toBeVisible();
    await link.click();
    await expect(menu.locator('.ant-menu-item-selected')).toHaveAttribute('aria-label', name);
  }
  await page.setViewportSize({ width: 767, height: 800 });
  await page.getByRole('button', { name: 'Open navigation' }).click();
  await expect(page.getByRole('dialog').getByRole('link', { name: 'Cells', exact: true })).toBeVisible();
  await page.getByRole('dialog').getByRole('link', { name: 'Cells', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Open navigation' })).toBeFocused();
  await page.setViewportSize({ width: 768, height: 400 });
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '80px');
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

for (const collapsed of [false, true]) {
  test(`all destinations retain mouse, keyboard, touch and modified links (${collapsed})`, async ({ page, context }) => {
    await page.goto('/project/navigation/goals');
    if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
    const menu = page.locator('.desktop-navigation .ant-menu');
    for (const [name, route] of [['Reviews', 'reviews'], ['Cells', 'cells'], ['Admin', 'admin'], ['Goals', 'goals']]) {
      const item = menu.getByRole('menuitem', { name, exact: true });
      const link = item.getByRole('link', { name, exact: true });
      await item.focus();
      if (collapsed) await expect(page.getByRole('tooltip', { name, exact: true })).toBeVisible();
      await page.keyboard.press('Enter');
      await expect(page).toHaveURL(new RegExp(`/${route}$`));
      await expect(item).toHaveClass(/ant-menu-item-selected/);
      await link.click();
      await expect(page).toHaveURL(new RegExp(`/${route}$`));
      // Dispatch a genuine touchscreen tap in the separately touch-enabled case below.
      const popupPromise = context.waitForEvent('page');
      await link.click({ modifiers: ['ControlOrMeta'] });
      const popup = await popupPromise;
      await popup.waitForLoadState();
      expect(popup.url()).toContain(`/project/navigation/${route}`);
      await popup.close();
    }
    await menu.getByRole('menuitem', { name: 'Goals', exact: true }).focus();
    await page.keyboard.press('ArrowDown');
    await expect(menu.getByRole('link', { name: 'Reviews', exact: true })).toBeFocused();
    expect(await menu.locator('a').evaluateAll(links => links.every(link => link.tabIndex === -1))).toBe(true);
  });
}

test('Space toggles once, no captions/header toggle, tenant persistence and nested selection', async ({ page }) => {
  await page.goto('/project/navigation/goals');
  await expect(page.locator('.app-header .navigation-toggle')).toHaveCount(0);
  const toggle = page.locator('.navigation-toggle');
  await expect(toggle).toHaveText('');
  await expect(toggle.locator('[aria-label="menu-fold"]')).toHaveCount(1);
  await toggle.focus(); await page.keyboard.press('Space');
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await expect(toggle.locator('[aria-label="menu-unfold"]')).toHaveCount(1);
  await page.getByRole('searchbox', { name: 'Tenant ID' }).fill('second');
  await page.getByRole('button', { name: 'Use', exact: true }).click();
  await expect(page).toHaveURL(/\/project\/second\/jobs$/);
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '80px');
  await page.reload();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '80px');
  await page.goto('/project/second/cells/cell-platform');
  await expect(page.locator('.desktop-navigation .ant-menu-item-selected')).toHaveAttribute('aria-label', 'Cells');
  await page.goto('/project/second/goals/a');
  await expect(page.locator('.desktop-navigation .ant-menu-item-selected')).toHaveAttribute('aria-label', 'Goals');
});

test('denied reads and writes keep navigation usable in memory', async ({ page }) => {
  await page.addInitScript(() => {
    Storage.prototype.getItem = () => { throw new Error('read denied'); };
    Storage.prototype.setItem = () => { throw new Error('write denied'); };
  });
  await page.goto('/project/navigation/goals');
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '220px');
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '80px');
  await page.getByRole('button', { name: 'Expand navigation' }).click();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '220px');
  await page.reload();
  await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '220px');
});

test('badges update through activity events in both widths', async ({ page }) => {
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
  const reviews = page.locator('.desktop-navigation').getByRole('link', { name: 'Reviews', exact: true });
  for (const collapsed of [false, true]) {
    if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
    for (const count of [0, 7, 120, 3]) {
      await page.evaluate(count => {
        const inputs = Array.from({ length: count }, (_, i) => ({
          id: String(i), tenant_id: 'navigation', cell_id: 'cell-platform', cell_name: 'platform',
          cell_alias: 'platform', cell_version: 1, repo: 'github.com/acme/platform', ref: 'main',
        }));
        (window as any).navStreams.at(-1).listeners.inputs_changed({ data: JSON.stringify({ inputs }) });
      }, count);
      await expect(reviews).toHaveAccessibleDescription(`${count} pending reviews`);
      await expect(page.locator('.desktop-navigation')).toHaveCSS('width', collapsed ? '80px' : '220px');
      const badge = reviews.locator('.ant-badge-count');
      if (!count) { await expect(badge).toHaveCount(0); continue; }
      await expect(badge).toHaveCount(1);
      await expect(badge).toHaveAttribute('title', String(count));
      if (count > 99) await expect(badge).toHaveText('99+');
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

test('scroll, short desktop, mobile drawer dismissal and desktop return', async ({ page }) => {
  await page.goto('/project/navigation/goals');
  await page.getByRole('button', { name: 'Collapse navigation' }).click();
  await page.locator('.app-content').evaluate(el => (el as HTMLElement).style.minHeight = '2000px');
  await page.evaluate(() => scrollTo(0, 1000));
  await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeInViewport({ ratio: 1 });
  for (const [width, height] of [[768, 260], [1280, 300], [767, 800], [390, 844]]) {
    await page.setViewportSize({ width, height });
    await page.evaluate(() => scrollTo(0, 0));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    if (width >= 768) {
      await expect(page.getByRole('button', { name: 'Expand navigation' })).toBeInViewport({ ratio: 1 });
      for (const name of ['Goals', 'Reviews', 'Cells', 'Admin']) {
        const link = page.locator('.desktop-navigation').getByRole('link', { name, exact: true });
        await link.scrollIntoViewIfNeeded();
        // Chromium rounds scroll offsets to device pixels; allow less than one CSS pixel.
        const bounds = (await link.boundingBox())!;
        expect(bounds.y).toBeGreaterThanOrEqual(-1);
        expect(bounds.y + bounds.height).toBeLessThanOrEqual(height + 1);
        expect(await link.evaluate(el => { const r = el.getBoundingClientRect(); return el.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); })).toBe(true);
      }
      await page.screenshot({ animations: 'disabled', path: `${captures}/short-${width}.png` });
      continue;
    }
    const opener = page.getByRole('button', { name: 'Open navigation' });
    await opener.click();
    const dialog = page.getByRole('dialog');
    for (const name of ['Goals', 'Reviews', 'Cells', 'Admin']) await expect(dialog.getByRole('link', { name, exact: true })).toBeVisible();
    if (width === 390) await page.screenshot({ animations: 'disabled', path: `${captures}/mobile.png` });
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(opener).toBeFocused();
    await opener.click();
    await page.setViewportSize({ width: 768, height: 400 });
    await expect(page.getByRole('dialog')).not.toBeVisible();
    await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '80px');
  }
});

test.describe('touch', () => {
  test.use({ hasTouch: true });
  test('all desktop icons accept touch', async ({ page }) => {
    await page.goto('/project/navigation/goals');
    for (const collapsed of [false, true]) {
      if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).tap();
      for (const [name, route] of [['Reviews', 'reviews'], ['Cells', 'cells'], ['Admin', 'admin'], ['Goals', 'goals']]) {
        await page.locator('.desktop-navigation').getByRole('link', { name, exact: true }).tap();
        await expect(page).toHaveURL(new RegExp(`/${route}$`));
      }
    }
  });
});

test.describe('native zoom navigation', () => {
  test.use({ nativeZoom: true });
  test('200 percent uses full drawer then restores desktop preference', async ({ page }) => {
    await page.goto('/project/navigation/goals');
    await page.getByRole('button', { name: 'Collapse navigation' }).click();
    await setNativeZoom(page, 2);
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await expect(page.getByRole('dialog').getByRole('link', { name: 'Reviews', exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await captureNativeZoom(page, `${captures}/zoom-200.png`);
    await page.getByRole('dialog').getByRole('link', { name: 'Cells', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Open navigation' })).toBeFocused();
    await setNativeZoom(page, 1);
    await expect(page.locator('.desktop-navigation')).toHaveCSS('width', '80px');
  });
});

for (const collapsed of [false, true]) {
  test(`login and tenant switching remain usable (${collapsed})`, async ({ page, context }) => {
    await page.goto('/project/navigation/goals');
    if (collapsed) await page.getByRole('button', { name: 'Collapse navigation' }).click();
    await context.clearCookies();
    await page.reload();
    await page.getByPlaceholder('Email address').fill('returning@example.com');
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
    await expect(page.locator('.desktop-navigation')).toHaveCSS('width', collapsed ? '80px' : '220px');
    await page.getByRole('searchbox', { name: 'Tenant ID' }).fill('tenant-two');
    await page.getByRole('button', { name: 'Use', exact: true }).click();
    await expect(page).toHaveURL(/\/project\/tenant-two\/jobs$/);
    await expect(page.locator('.desktop-navigation').getByRole('link', { name: 'Reviews', exact: true })).toHaveAttribute('href', '/project/tenant-two/reviews');
    await expect(page.locator('.desktop-navigation')).toHaveCSS('width', collapsed ? '80px' : '220px');
  });
}

test('expanded narrow desktop keeps navigation and trigger reachable', async ({ page }) => {
  await page.goto('/project/navigation/goals');
  for (const [width, height] of [[768, 260], [1280, 300]]) {
    await page.setViewportSize({ width, height });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await expect(page.getByRole('button', { name: 'Collapse navigation' })).toBeInViewport({ ratio: 1 });
    for (const name of ['Goals', 'Reviews', 'Cells', 'Admin']) {
      const link = page.locator('.desktop-navigation').getByRole('link', { name, exact: true });
      await link.scrollIntoViewIfNeeded();
      const bounds = (await link.boundingBox())!;
      expect(bounds.y).toBeGreaterThanOrEqual(-1);
      expect(bounds.y + bounds.height).toBeLessThanOrEqual(height + 1);
      expect(await link.evaluate(el => { const r = el.getBoundingClientRect(); return el.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)); })).toBe(true);
    }
  }
});
