import { test, expect } from '@playwright/test';
import { jobIdentity } from './helpers/cells';

test.beforeEach(async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'conversation@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).EventSource = class extends EventTarget {
      readyState = 1;
      constructor() { super(); (window as any).historyInputStream = this; }
      close() {}
    };
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { tenant_id: 'demo', job_id: 'job', entries: [], snapshot_token: 'snapshot', completeness: 'partial' } });
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ status: 404, json: { error: 'Not retained', code: 'history_not_found' } });
    if (url.pathname === '/api/projects') return route.fulfill({ json: [{ tenant_id: 'demo' }] });
    if (url.pathname.endsWith('/user-inputs/pending')) return route.fulfill({ json: [] });
    if (url.pathname.endsWith('/input-artifacts')) return route.fulfill({ json: { artifacts: [] } });
    if (url.pathname.endsWith('/children')) return route.fulfill({ json: { jobs: [] } });
    if (url.pathname.endsWith('/story')) return route.fulfill({ json: {
      job_id: 'job', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'Build', status: 'completed', path: [], attempt: 1 },
    } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: {
      ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Conversation fixture', prompt: 'The original request',
      status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success', completion_detail: 'The requested work is complete.',
    } });
    return route.fulfill({ json: {} });
  });
});

test('CT-001–007 default, panel visibility, keyboard navigation and URL restoration', async ({ page }, info) => {
  await page.goto('/project/demo/jobs/job/story?tenantId=demo&filter=mine');
  const tabs = page.getByRole('tablist', { name: 'Job detail' });
  const conversation = tabs.getByRole('tab', { name: 'Conversation', exact: true });
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('heading', { name: 'Job Story: Conversation fixture' })).toBeVisible();
  await expect(page.getByLabel('Job cell')).toBeVisible();
  await expect(page.getByLabel('Job status')).toBeVisible();
  await expect(page.getByRole('button', { name: /Refresh$/ })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job prompt' })).toHaveText('PromptThe original request');
  await expect(page.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(page.getByRole('tree')).toHaveCount(0);
  await page.screenshot({ path: info.outputPath('conversation.png'), fullPage: true });
  for (const label of ['Execution', 'Relationships', 'Input artifacts', 'Summary', 'Conversation']) {
    await tabs.getByRole('tab', { name: label, exact: true }).click();
    await expect(page.locator('.job-detail-panel:visible')).toHaveCount(1);
    await expect(page.getByRole('tabpanel', { name: label, exact: true })).toBeVisible();
    await expect(page).toHaveURL(/tenantId=demo&filter=mine&tab=/);
  }
  await conversation.focus();
  await page.keyboard.press('ArrowRight');
  await expect(tabs.getByRole('tab', { name: 'Execution', exact: true })).toBeFocused();
  expect(await tabs.getByRole('tab', { name: 'Execution', exact: true }).evaluate(element => getComputedStyle(element).outlineStyle)).toBe('solid');
  await page.keyboard.press('End');
  await expect(tabs.getByRole('tab', { name: 'Summary', exact: true })).toBeFocused();
  await page.reload();
  await expect(tabs.getByRole('tab', { name: 'Summary', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goBack();
  await expect(tabs.getByRole('tab', { name: 'Execution', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goForward();
  await expect(tabs.getByRole('tab', { name: 'Summary', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?tab=invalid&nodeId=root');
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?nodeId=root');
  await expect(tabs.getByRole('tab', { name: 'Execution', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?requestId=opaque%2Funknown&tab=execution');
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByText('Request unavailable', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /Submit/ })).toHaveCount(0);
});

test('CT-007 narrow tabs wrap without horizontal page overflow', async ({ page }, info) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto('/project/demo/jobs/job/story');
  const tabs = page.getByRole('tablist', { name: 'Job detail' });
  await expect(tabs).toBeVisible();
  const boxes = await tabs.getByRole('tab').evaluateAll(elements => elements.map(element => ({ top: element.getBoundingClientRect().top, right: element.getBoundingClientRect().right })));
  expect(new Set(boxes.map(box => box.top)).size).toBeGreaterThan(1);
  expect(Math.max(...boxes.map(box => box.right))).toBeLessThanOrEqual(320);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
  await tabs.getByRole('tab', { name: 'Summary', exact: true }).focus();
  await page.keyboard.press('Home');
  await expect(tabs.getByRole('tab', { name: 'Conversation', exact: true })).toBeFocused();
  await page.screenshot({ path: info.outputPath('conversation-narrow.png'), fullPage: true });
});

test('CT-014,021,031 retained answers and exact earlier review stay selected through completion', async ({ page }) => {
  let stage = 0;
  const entry = (id: string, answered: boolean) => ({
    entry_id: id, request_id: `request-${id}`, kind: 'review', title: { present: true, raw: `Review ${id}` }, summary: { present: false },
    state: answered ? 'answered' : 'pending', requested_at: { present: false }, locator: { invocation_id: id },
    form_fields: { question: `Question ${id}` }, accepted_response: { present: answered, ...(answered ? { raw: { decision: 'approve', feedback: 'Retain the original' } } : {}) },
    response_source: answered ? 'submission' : '', actor: { provenance: answered ? 'submission_receipt' : 'unavailable', legacy_user_id: { present: false } },
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { tenant_id: 'demo', job_id: 'job', entries: [entry('first', stage > 0), ...(stage ? [entry('second', stage > 1)] : [])], snapshot_token: 'snapshot', completeness: 'partial' } });
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ json: { ...entry('first', true), snapshot_token: 'snapshot', completeness: 'partial', request_manifest: [{ artifact_id: 'original', availability: 'retained', ref: {}, associations: [] }], response_manifest: [], actionability: { actionable: false, observation: 'live' } } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: { ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Conversation fixture', prompt: 'The original request', status: stage > 1 ? 'COMPLETED' : 'READY', store: stage > 1 ? 'ARCHIVED' : 'ACTIVE', completion_status: stage > 1 ? 'success' : '', completion_detail: stage > 1 ? 'Persisted completion' : '' } });
    return route.fallback();
  });
  await page.goto('/project/demo/jobs/job/story?tenantId=demo&filter=mine');
  await expect(page.getByText('State: pending', { exact: false })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job completion' })).toHaveCount(0);
  stage = 1;
  await page.getByRole('button', { name: 'Refresh history', exact: true }).click();
  await expect(page.getByText('Occurrence: second')).toBeVisible();
  await expect(page.getByText(/Retain the original/)).toBeVisible();
  await page.getByRole('button', { name: 'Open recorded request' }).first().click();
  await expect(page).toHaveURL(/occurrenceId=first/);
  const recorded = page.getByRole('region', { name: 'Recorded request', exact: true });
  await expect(recorded.getByText('Occurrence: first')).toBeVisible();
  await expect(recorded.getByRole('link', { name: 'Download original' })).toHaveAttribute('href', /entry_id=first&artifact_id=original/);
  await expect(recorded.getByRole('button', { name: /Submit/ })).toHaveCount(0);
  await page.reload();
  await expect(recorded.getByText('Occurrence: first')).toBeVisible();
  stage = 2;
  await page.getByRole('button', { name: /Refresh$/ }).click();
  await expect(page.getByRole('region', { name: 'Job completion' })).toContainText('Persisted completion');
  await expect(page.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(recorded.getByText('Occurrence: first')).toBeVisible();
  await expect(page).toHaveURL(/occurrenceId=first/);
});


test('CT-021 automatic polling and input activity preserve historical selection, scroll and focus', async ({ page }) => {
  await page.clock.install();
  let revision = 0;
  const entry = (id: string) => ({
    entry_id: id, request_id: id, kind: 'review', title: { present: true, raw: id }, summary: { present: false },
    state: 'answered', requested_at: { present: false }, locator: { invocation_id: id },
    form_fields: { question: 'Original review\n'.repeat(80) }, accepted_response: { present: true, raw: 'Original accepted answer' },
    response_source: 'submission', actor: { provenance: 'submission_receipt', legacy_user_id: { present: false } },
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { tenant_id: 'demo', job_id: 'job', entries: [entry('first'), ...(revision ? [entry(`arrival-${revision}`)] : [])], snapshot_token: `snapshot-${revision}`, completeness: 'partial' } });
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ json: { ...entry('first'), snapshot_token: 'original-detail', completeness: 'partial', request_manifest: [{ artifact_id: 'original', availability: 'retained' }], response_manifest: [], actionability: { actionable: false } } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: { ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Reading history', status: 'READY', store: 'ACTIVE' } });
    return route.fallback();
  });
  await page.goto('/project/demo/jobs/job/story?occurrenceId=first');
  const recorded = page.getByRole('region', { name: 'Recorded request', exact: true });
  const link = recorded.getByRole('link', { name: 'Download original' });
  await expect(link).toBeVisible();
  await link.focus();
  await page.evaluate(() => window.scrollBy(0, -200));
  const scrollBefore = await page.evaluate(() => window.scrollY);
  expect(scrollBefore).toBeGreaterThan(0);
  revision = 1;
  await page.clock.runFor(5100);
  await expect(page.getByText('Occurrence: arrival-1')).toBeAttached();
  await expect(link).toBeFocused();
  expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollBefore)).toBeLessThanOrEqual(1);
  await expect(page.getByRole('tab', { name: 'Conversation', exact: true })).toHaveAttribute('aria-selected', 'true');
  expect(new URL(page.url()).searchParams.get('occurrenceId')).toBe('first');
  await expect(page.getByText('New conversation activity available')).toBeAttached();

  // Send the actual event consumed by InputActivityProvider before the next
  // polling interval. Its new snapshot must refresh history without navigation.
  revision = 2;
  await page.evaluate(() => (window as any).historyInputStream.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [] }) })));
  await expect(page.getByText('Occurrence: arrival-2')).toBeAttached();
  await expect(link).toBeFocused();
  expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollBefore)).toBeLessThanOrEqual(1);
  expect(new URL(page.url()).searchParams.get('occurrenceId')).toBe('first');
});
