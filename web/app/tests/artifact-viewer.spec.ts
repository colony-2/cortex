import { test, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';

for (const viewport of [{ width: 1280, height: 800 }, { width: 390, height: 844 }]) {
  test(`SAV-026,027 story viewer ${viewport.width}`, async ({ page, context, baseURL }, testInfo) => {
    await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
    await page.setViewportSize(viewport);
    const errors: string[] = [];
    page.on('pageerror', e => errors.push(e.message));
    const original = '# Browser artifact\n\n' + 'long line '.repeat(220000);
    await page.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (url.pathname.includes('/artifacts/')) return route.continue();
      let body: unknown = [];
      if (url.pathname === '/api/projects') body = [{ tenant_id: 'tenant', name: 'Test tenant' }];
      else if (url.pathname.endsWith('/story')) body = {job_id:'job', status:'success',root:{id:'root',kind:'opStep',title:'Contents',status:'success',path:[],artifact_keys:[{name:'large.md',taskOrdinal:1},{name:'huge.png',taskOrdinal:2},{name:'wide.png',taskOrdinal:3},{name:'wide.patch',taskOrdinal:4}]}};
      else if (url.pathname.endsWith('/jobs/job')) body = {job_id:'job',title:'Fixture job',status:'SUCCEEDED'};
      else if (url.pathname.includes('/events')) return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
      await route.fulfill({json:body});
    });
    await page.goto('/project/tenant/jobs/job/story?nodeId=root');
    await page.getByRole('tab', { name: 'Artifacts (4)' }).click();
    const opener = page.getByRole('link', { name: 'large.md', exact: true });
    await opener.click();
    const modal = page.getByRole('dialog');
    await expect(modal.getByText(/Preview truncated/)).toBeVisible();
    await expect.poll(async () => (await modal.boundingBox())!.width).toBeGreaterThan(Math.min(1000, viewport.width - 32) * .95);
    const download = modal.getByRole('button', { name: 'Download' });
    await expect(download).toBeInViewport();
    const body = modal.locator('.ant-modal-body');
    expect(await body.evaluate(e => e.scrollHeight >= e.clientHeight)).toBe(true);
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
    });
    await page.screenshot({path:testInfo.outputPath(`story-${viewport.width}.png`)});
    const pending = page.waitForEvent('download');
    await download.click();
    const saved = await pending;
    expect(saved.suggestedFilename()).toBe('large.md');
    expect(await readFile((await saved.path())!, 'utf8')).toBe(original);
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
    await expect(opener).toBeFocused();
    // An unavailable preview must still allow the complete original download.
    await page.route('**/artifacts/large.md', route => route.fulfill({ status: 503, body: 'unavailable' }));
    await opener.click();
    await expect(modal.getByText(/Could not load this document/)).toBeVisible();
    const unavailableDownload = page.waitForEvent('download');
    await modal.getByRole('button', { name: 'Download' }).click();
    expect(await readFile((await (await unavailableDownload).path())!, 'utf8')).toBe(original);
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
    await page.getByRole('link', { name: 'huge.png', exact: true }).click();
    await expect(modal.getByText(/32 megapixel preview limit/)).toBeVisible();
    const rejectedDownload = page.waitForEvent('download');
    await modal.getByRole('button', { name: 'Download' }).click();
    const rejected = await rejectedDownload;
    expect(rejected.suggestedFilename()).toBe('huge.png');
    const bytes = await readFile((await rejected.path())!);
    expect(bytes.length).toBe(24);
    expect(bytes.readUInt32BE(16)).toBe(10000);
    await page.keyboard.press('Escape');
    await page.getByRole('link', { name: 'wide.png', exact: true }).click();
    await expect(modal.getByRole('img', {name:'wide.png'})).toBeVisible();
    await modal.getByRole('button', {name:'Actual size'}).click();
    const imageScroll = modal.getByRole('region', {name:'Image preview'});
    expect(await imageScroll.evaluate(e => e.scrollWidth > e.clientWidth)).toBe(true);
    await expect(modal.getByRole('button', {name:'Download'})).toBeInViewport();
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
    });
    await page.screenshot({path:testInfo.outputPath(`image-${viewport.width}.png`)});
    await page.keyboard.press('Escape');
    await page.getByRole('link', { name: 'wide.patch', exact: true }).click();
    await modal.getByText('Side by side', {exact:true}).click();
    await expect(modal.getByLabel('Side by side diff')).toBeVisible();
    expect(await modal.locator('.review-diff-scroll').evaluate(e => e.scrollWidth > e.clientWidth)).toBe(true);
    await expect(modal.getByRole('button', {name:'Download'})).toBeInViewport();
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})));
    });
    await page.screenshot({path:testInfo.outputPath(`diff-${viewport.width}.png`)});
    await page.keyboard.press('Escape');
    expect(errors).toEqual([]);
  });
}
