import { test, expect } from '@playwright/test';
import { jobIdentity, registryCell } from '../src/test/cellFixtures';

for (const width of [1440, 390]) test(`expanded detail excludes obsolete membership at ${width}px`, async ({ page, context, baseURL }, testInfo) => {
  await page.setViewportSize({ width, height: 1000 });
  await context.addCookies([{ name: 'colony2:user:email', value: 'review@example.com', url: baseURL! }]);
  const job = {
    ...jobIdentity, tenant_id: 'retirement', job_id: 'consumer-A', goal_id: 'primary-H',
    title: 'Preserve the recorded result producer', prompt: 'Keep primary identity and complete dependency history readable.',
    other_goal_ids: ['STALE-SUMMARY-SECONDARY'], status: 'PENDING_JOBS', store: 'ACTIVE',
    job_type: 'recipe', recipe: 'build', created_at: '2026-11-14T00:00:00Z', available_at: '2026-11-14T00:00:00Z',
    blocking_wait_summary: { waitId: 'retained-wait', deadline: '2026-11-20T12:00:00Z' },
    parent: { tenant_id: 'retirement', job_id: 'recorded-parent', goal_id: 'literal-parent-goal' },
  };
  const wait = {
    waitId: 'retained-wait', state: 'PENDING', deadline: '2026-11-20T12:00:00Z',
    dependencies: [
      { target: { goalId: 'historical-G' }, condition: 'success', state: 'SATISFIED', satisfiedByJobId: 'producer-B', blockedReason: 'Retained witness; producer B now has primary H' },
      { target: { jobId: 'waiting-C' }, condition: 'complete', state: 'PENDING', blockedReason: 'Awaiting the exact recorded execution' },
    ],
  };
  let release!: () => void;
  const inspected = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/jobs/consumer-A')) {
      await inspected;
      return route.fulfill({ json: { ...job, other_goal_ids: ['STALE-INSPECTION-SECONDARY'], blocking_wait: wait } });
    }
    return route.fulfill({ json: path.endsWith('/projects') ? [{ id: 'retirement', tenant_id: 'retirement', name: 'Retirement check' }]
      : path.endsWith('/cells') ? { cells: [registryCell('platform', 'retirement')] }
      : path.includes('/cells/') ? registryCell('platform', 'retirement')
      : path.includes('/user-inputs/pending') ? { jobs: [] }
      : path.endsWith('/events') ? { events: [] }
      : path.endsWith('/jobs') ? { jobs: [job] } : {} });
  });
  const query = new URLSearchParams({ executionState: 'cells-v1', expanded: 'goal:primary-H' });
  for (const section of ['technical', 'data']) query.append('detail', JSON.stringify(['consumer-A', section]));
  await page.goto('/project/retirement/goals?' + query);
  const panel = page.locator('.goal-overview-content');
  await expect(panel).toBeVisible();
  await expect(page.getByLabel('Loading dependency and membership details')).toBeVisible();
  const raw = panel.locator('details').filter({ has: page.locator('summary', { hasText: 'All job data' }) }).locator('pre');
  await expect(raw).toContainText('primary-H');
  await expect(raw).not.toContainText('other_goal_ids');
  await expect(panel).not.toContainText('STALE-SUMMARY');
  await page.screenshot({ path: testInfo.outputPath(`detail-summary-${width}.png`), fullPage: true, animations: 'disabled' });
  release();
  await expect(panel).toContainText('Retained witness; producer B now has primary H');
  await expect(raw).toContainText('satisfiedByJobId');
  await expect(raw).toContainText('producer-B');
  await expect(raw).not.toContainText('other_goal_ids');
  await expect(panel).not.toContainText('STALE-');
  await expect(panel.getByRole('link', { name: 'View goal', exact: true })).toHaveAttribute('href', '/project/retirement/goals/historical-G');
  await expect(panel.locator('a[href*="/jobs/producer-B/story"]')).toHaveCount(1);
  await expect(panel.locator('a[href*="/jobs/recorded-parent/story"]')).toHaveCount(1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath(`detail-inspected-${width}.png`), fullPage: true, animations: 'disabled' });
});
