import { test, expect } from '@playwright/test';
for (const width of [1280, 320]) {
  test(`structured payload keyboard and wrapping at ${width}px`, async ({ page }) => {
    await page.context().addCookies([{ name: 'colony2:user:email', value: 'test@example.com', url: 'http://127.0.0.1:3000' }]);
    await page.setViewportSize({ width, height: 900 });
    await page.route('**/api/**', async route => {
      const url = route.request().url();
      const root = { id: 'root', kind: 'opStep', title: 'Native step', status: 'success', path: [],
        input: { ['long-label'.repeat(20)]: 'long-value'.repeat(100), nested: { files: ['a.ts'] } }, output: false };
      let data: unknown = {};
      if (url.includes('/story')) data = { job_id: 'job', status: 'success', recipe: {}, root };
      else if (url.endsWith('/projects')) data = [{ tenant_id: 'tenant', name: 'tenant' }];
      else if (url.includes('/jobs/job')) data = { id: 'job', status: 'success', cell_name: 'cortex', cell_id: 'cell', repo: 'github.com/acme/cortex', metadata: {} };
      await route.fulfill({ json: data });
    });
    await page.goto('/project/tenant/jobs/job/story?nodeId=root');
    await page.getByRole('tab', { name: 'Input', exact: true }).click();
    const region = page.getByRole('region', { name: 'Input payload' });
    const group = region.getByRole('button', { name: /nested Object/ });
    await group.focus();
    await page.keyboard.press('Shift+Tab');
    await page.keyboard.press('Tab');
    await expect(group).toBeFocused();
    expect(await group.evaluate(el => getComputedStyle(el).outlineStyle)).toBe('solid');
    await page.keyboard.press('Enter');
    await expect(group).toHaveAttribute('aria-expanded', 'true');
    const array = region.getByRole('button', { name: /files Array/ });
    await page.keyboard.press('Tab');
    await expect(array).toBeFocused();
    await page.keyboard.press('Space');
    await expect(region.getByText('a.ts')).toBeVisible();
    const bounds = await region.evaluate(el => {
      const row = el.querySelector('.node-payload-row')!;
      const label = row.children[0].getBoundingClientRect(), value = row.children[1].getBoundingClientRect();
      return { overflow: el.scrollWidth > el.clientWidth, stacked: value.top >= label.bottom };
    });
    expect(bounds.overflow).toBe(false);
    expect(bounds.stacked).toBe(width === 320);
    await page.screenshot({ path: process.env.NPD_SCREENSHOT_DIR ? `${process.env.NPD_SCREENSHOT_DIR}/payload-${width}.png` : `test-results/payload-${width}.png`, fullPage: true });
  });
}
