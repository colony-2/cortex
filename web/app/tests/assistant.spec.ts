import { test, expect } from '@playwright/test';
import { mockCells } from './helpers/cells';

// These tests exercise the real panel in Chromium with explicit HTTP fixtures.
// CTX-023 uses a separate retained-runtime host and cannot be established here.
for (const viewport of [{ name: 'desktop', width: 1280, height: 900 }, { name: 'mobile', width: 390, height: 844 }]) {
 test.describe(viewport.name, () => {
  test.use({ viewport: { width: viewport.width, height: viewport.height } });
  test.beforeEach(async ({ page, context, baseURL }) => {
   await context.addCookies([{ name: 'colony2:user:email', value: 'fixture@example.test', url: baseURL! }]);
   await page.addInitScript(() => {
    (window as any).EventSource = class { close() {} addEventListener() {} removeEventListener() {} };
   });
   await mockCells(page, ['platform']);
   await page.route(/\/api\/projects\/[^/]+\/jobs(\?.*)?$/, route => route.fulfill({ json: { jobs: [] } }));
   await page.route(/\/api\/projects\/[^/]+\/user-inputs\/pending$/, route => route.fulfill({ json: [] }));
   await page.route('**/assistant/capabilities', route => route.fulfill({ json: { available: true } }));
  });
  test('CTX-020 contains keyboard focus, restores focus and keeps the mounted draft', async ({ page }, testInfo) => {
   await page.goto('/project/assistant-browser/jobs');
   const launcher = page.getByRole('button', { name: 'Open assistant' });
   await launcher.click();
   const dialog = page.getByRole('dialog', { name: 'Contextual assistant' });
   await expect(dialog).toBeVisible();
   const input = dialog.getByRole('textbox', { name: 'Message' });
   await expect(input).toBeFocused();
   await input.fill('A draft stays here');
   const minimize = dialog.getByRole('button', { name: 'Minimize assistant' });
   await minimize.focus(); await page.keyboard.press('Shift+Tab');
   await expect(dialog.getByRole('button', { name: 'Send message' })).toBeFocused();
   await page.keyboard.press('Tab'); await expect(minimize).toBeFocused();
   if (viewport.name === 'mobile') {
    const bounds = await dialog.boundingBox();
    expect(bounds?.x).toBe(0); expect(bounds?.y).toBe(0); expect(bounds?.width).toBe(390);
   }
   await page.keyboard.press('Escape'); await expect(dialog).toBeHidden(); await expect(launcher).toBeFocused();
   await launcher.click(); await expect(input).toHaveValue('A draft stays here');
   await page.screenshot({ path: testInfo.outputPath(`assistant-${viewport.name}.png`) });
  });
  test('CTX-012, CTX-019 and CTX-024 recovers recorded history and never displays an invented answer', async ({ page }) => {
   const conversation = { id: 'session-durable', job_id: 'execution-durable', status: 'completed', source: { kind: 'global' },
    replay: { entries: [{ key: 'task-answer', role: 'assistant', status: 'succeeded', text: 'Recorded fixture answer\n\n<script>window.compromised = 1</script>\n\n[unsafe](javascript:alert%281%29)' }], watermark: 'fixture', has_more: false, complete: true } };
   await page.route('**/assistant/sessions?*', route => route.fulfill({ json: { sessions: [conversation] } }));
   await page.route('**/assistant/sessions/session-durable', route => route.fulfill({ json: conversation }));
   await page.goto('/project/assistant-browser/jobs');
   await page.getByRole('button', { name: 'Open assistant' }).click();
   await page.getByRole('button', { name: 'Conversation history' }).click();
   await page.getByRole('button', { name: 'No page evidence · completed' }).click();
   const transcript = page.getByLabel('Conversation transcript');
   await expect(transcript).toContainText('Recorded fixture answer');
   await expect(transcript.locator('script')).toHaveCount(0);
   await expect(transcript.getByRole('link', { name: 'unsafe' })).toHaveCount(0);
   expect(await page.evaluate(() => (window as any).compromised)).toBeUndefined();
   await page.reload();
   await page.getByRole('button', { name: 'Open assistant' }).click();
   await page.getByRole('button', { name: 'Conversation history' }).click();
   await page.getByRole('button', { name: 'No page evidence · completed' }).click();
   await expect(page.getByLabel('Conversation transcript')).toContainText('Recorded fixture answer');
   await expect(page.getByLabel('Captured context')).toContainText('No page evidence');
   await expect(page.getByRole('dialog', { name: 'Contextual assistant' }).getByRole('checkbox')).toHaveCount(0);
  });
 });
}

test('CTX-014 keeps captured evidence through SPA navigation and clears it on tenant change', async ({ page, context, baseURL }) => {
 await context.addCookies([{ name: 'colony2:user:email', value: 'fixture@example.test', url: baseURL! }]);
 await page.addInitScript(() => { (window as any).EventSource = class { close() {} addEventListener() {} removeEventListener() {} }; });
 const starts: any[] = [];
 await page.route('**/api/**', async route => {
  const url = new URL(route.request().url());
  const endpoint = url.pathname;
  if (endpoint === '/api/projects') return route.fulfill({ json: [{ tenant_id: 'a' }] });
  if (endpoint.endsWith('/assistant/capabilities')) return route.fulfill({ json: { available: true } });
  if (endpoint.endsWith('/assistant/sessions') && route.request().method() === 'POST') {
   const input = route.request().postDataJSON(); starts.push(input);
   return route.fulfill({ json: { id: `session-${starts.length}`, job_id: 'second-story', status: 'completed', source: input.source,
    replay: { entries: [{ key: 'answer', role: 'assistant', status: 'succeeded', text: 'Captured fixture answer' }], watermark: 'fixture', complete: true, has_more: false } } });
  }
  if (endpoint.endsWith('/story')) return route.fulfill({ json: { job_id: endpoint.split('/').at(-2), status: 'completed', root: { id: 'root', kind: 'opStep', title: 'Evidence', status: 'succeeded', path: [] } } });
  if (/\/jobs\/[^/]+$/.test(endpoint)) return route.fulfill({ json: { tenant_id: 'a', job_id: endpoint.split('/').at(-1), cell_id: 'fixture-cell', cell_name: 'platform', cell_revision: 'fixture-revision', goal_id: 'fixture-goal', prompt: 'Source job', status: 'COMPLETED', store: 'ARCHIVED' } });
  if (endpoint.endsWith('/input-artifacts')) return route.fulfill({ json: { artifacts: [] } });
  if (endpoint.endsWith('/cells') || endpoint.endsWith('/jobs')) return route.fulfill({ json: { cells: [], jobs: [] } });
  return route.fulfill({ json: [] });
 });
 await page.goto('/project/a/jobs/first-story/story');
 await page.getByRole('button', { name: 'Open assistant' }).click();
 const dialog = page.getByRole('dialog', { name: 'Contextual assistant' });
 await dialog.getByRole('checkbox', { name: 'Include this story' }).check();
 await dialog.getByRole('textbox', { name: 'Message' }).fill('Discuss captured evidence');
 await dialog.getByRole('button', { name: 'Send message' }).click();
 await expect(dialog.getByLabel('Conversation transcript')).toContainText('Captured fixture answer');
 expect(starts[0].source).toEqual({ kind: 'story', job_id: 'first-story' });
 await dialog.getByRole('link', { name: 'View execution' }).click();
 await expect(page).toHaveURL(/\/jobs\/second-story\/story/);
 await expect(dialog.getByLabel('Captured context')).toContainText('Story first-story');
 await expect(dialog.getByText(/Page changed/)).toBeVisible();
 await dialog.getByRole('button', { name: 'New conversation' }).click();
 await expect(dialog.getByRole('checkbox')).not.toBeChecked();
 await dialog.getByRole('checkbox').check();
 await dialog.getByRole('textbox', { name: 'Message' }).fill('Use this new page');
 await dialog.getByRole('button', { name: 'Send message' }).click();
 await expect(dialog.getByLabel('Captured context')).toContainText('Story second-story');
 expect(starts[1].source).toEqual({ kind: 'story', job_id: 'second-story' });
 expect(starts[1].request_id).not.toBe(starts[0].request_id);
 await dialog.getByRole('button', { name: 'Minimize assistant' }).click();
 await page.getByRole('searchbox', { name: 'Tenant ID' }).fill('b');
 await page.getByRole('button', { name: 'Use', exact: true }).click();
 await expect(page).toHaveURL(/\/project\/b\/jobs/);
 await page.getByRole('button', { name: 'Open assistant' }).click();
 await expect(dialog.getByLabel('Conversation transcript')).toBeEmpty();
 await expect(dialog.getByRole('textbox', { name: 'Message' })).toHaveValue('');
 await expect(dialog.getByLabel('Captured context')).toContainText('No page evidence');
});
