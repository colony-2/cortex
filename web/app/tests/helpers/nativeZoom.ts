import { test as base, expect, type Page } from '@playwright/test';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// A temporary test-only extension invokes Chromium's native tab zoom API.
// Neither viewport emulation, CSS zoom nor pinch/page-scale emulation is used.
export const test = base.extend<{ nativeZoom: boolean }>({
  nativeZoom: [false, { option: true }],
  context: async ({ context, playwright, launchOptions, nativeZoom }, use) => {
    if (!nativeZoom) return use(context);
    const directory = mkdtempSync(join(tmpdir(), 'cortex-native-zoom-'));
    writeFileSync(join(directory, 'manifest.json'), JSON.stringify({
      manifest_version: 3, name: 'Cortex zoom validation', version: '1.0',
      permissions: ['tabs'], background: { service_worker: 'worker.js' },
    }));
    writeFileSync(join(directory, 'worker.js'), 'chrome.runtime.onInstalled.addListener(() => {});');
    const zoomContext = await playwright.chromium.launchPersistentContext('', {
      ...launchOptions, channel: 'chromium', headless: true, viewport: { width: 1280, height: 900 },
      args: [...(launchOptions.args || []), `--disable-extensions-except=${directory}`, `--load-extension=${directory}`],
    });
    try { await use(zoomContext); }
    finally { await zoomContext.close(); rmSync(directory, { recursive: true, force: true }); }
  },
});

export async function setNativeZoom(page: Page, factor: number) {
  const context = page.context();
  const worker = context.serviceWorkers()[0] || await context.waitForEvent('serviceworker');
  const actual = await worker.evaluate(async ({ url, factor }) => {
    // Chromium extension APIs exist only in this service worker, not the app.
    const chrome = (globalThis as any).chrome;
    const tab = (await chrome.tabs.query({})).find((tab: any) => tab.url === url);
    if (!tab) throw new Error(`No browser tab for ${url}`);
    await chrome.tabs.setZoom(tab.id, factor);
    return chrome.tabs.getZoom(tab.id);
  }, { url: page.url(), factor });
  expect(actual).toBe(factor);
  await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBe(factor);
  await expect.poll(() => page.evaluate(() => innerWidth)).toBe(1280 / factor);
  await expect.poll(() => page.evaluate(() => innerHeight)).toBe(900 / factor);
}

export async function captureNativeZoom(page: Page, path: string) {
  // Let Ant Design finish finite opening transitions before collecting evidence.
  await page.evaluate(async () => {
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    await Promise.all(document.getAnimations().filter(animation => animation.effect?.getTiming().iterations !== Infinity)
      .map(animation => animation.finished.catch(() => {})));
  });
  // Capture the compositor viewport directly; do not adjust its emulation size.
  const session = await page.context().newCDPSession(page);
  try {
    const { data } = await session.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
    writeFileSync(path, Buffer.from(data, 'base64'));
  } finally { await session.detach(); }
}
