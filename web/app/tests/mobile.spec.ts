import { expect, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';

const prompt = 'Build a reliable dashboard ' + 'long-content-'.repeat(30);
const jobs = [
  { job_id: 'one', goal_id: 'g', cell_name: 'platform', repo: 'https://github.com/colony-2/platform.git', prompt, status: 'READY', store: 'ACTIVE', created_at: '2026-10-30T10:00:00Z' },
  { job_id: 'two', goal_id: 'g', cell_name: 'platform', prompt, status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success', created_at: '2026-10-29T10:00:00Z' },
  { job_id: 'three', goal_id: 'other', prompt: 'Another goal', status: 'READY', store: 'ACTIVE', created_at: '2026-10-28T10:00:00Z' },
];
const cells = [
  { id: 'self', name: 'cortex', repository_source: 'https://github.com/colony-2/cortex.git', kind: 'self', git_ref: 'main' },
  { id: 'api', name: 'api', repository_source: 'https://github.com/colony-2/api.git', kind: 'dependent', git_ref: 'long-ref-'.repeat(30) },
];
async function noOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}
async function filter(page: Page, name: string, selected: string[] | null) {
  if (!await page.getByRole('button', { name, exact: true }).isVisible()) await page.getByRole('button', { name: /^Filters and sorting/ }).click();
  await page.getByRole('button', { name, exact: true }).click();
  const popup = page.getByRole('dialog', { name });
  await popup.getByRole('button', { name: selected === null ? 'Select all' : 'Select none', exact: true }).click();
  for (const label of selected || []) await popup.getByRole('checkbox', { name: label, exact: true }).check();
  await popup.getByRole('button', { name: 'Apply', exact: true }).click();
}

test.beforeEach(async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'phone@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).sseConnections = 0;
    (window as any).EventSource = class {
      readyState = 1;
      constructor() { (window as any).sseConnections++; }
      close() {} addEventListener() {} removeEventListener() {}
    };
  });
  await page.route('**/api/projects', route => route.fulfill({ json: [{ tenant_id: 'phone' }] }));
  await page.route(/\/api\/projects\/[^/]+\/cells$/, route => route.fulfill({ json: cells }));
  await page.route(/\/api\/projects\/[^/]+\/user-inputs\/pending$/, route => route.fulfill({ json: [] }));
  await page.route(/\/api\/projects\/[^/]+\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs } }));
  await page.route(/\/api\/projects\/[^/]+\/jobs\/[^/?]+$/, route => {
    const id = new URL(route.request().url()).pathname.split('/').pop();
    return route.fulfill({ json: { ...jobs.find(job => job.job_id === id), tenant_id: 'phone' } });
  });
  await page.route(/\/api\/projects\/[^/]+\/jobs\/[^/]+\/story$/, route => route.fulfill({ json: { job_id: 'one', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'Phone recipe', path: [], attempt: 1, status: 'completed', children: [{ id: 'child', kind: 'op', title: 'Task name ' + 'x'.repeat(200), path: ['step'], attempt: 1, status: 'completed', output: { data: 'value'.repeat(200) } }] } } }));
});

test('phone filters, ordering, selection, history, and responsive state share one model', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const reads: string[] = [];
  page.on('request', request => { if (/\/jobs(?:\?|\/[^/]+$)/.test(request.url())) reads.push(request.url()); });
  await page.goto('/project/phone/goals');
  await expect(page.locator('.goal-card')).toHaveCount(2);
  await expect(page.getByText('2 attempts on this page')).toBeVisible();
  await filter(page, 'Filter cells', []);
  await expect(page.getByText('No cells selected')).toBeVisible();
  await filter(page, 'Filter cells', ['Unknown cell']);
  await expect(page.locator('.goal-card')).toHaveCount(1);
  await expect(page.getByRole('link', { name: 'Another goal', exact: true })).toBeVisible();
  await filter(page, 'Filter cells', null);
  await filter(page, 'Filter statuses', []);
  await expect(page.getByText('No statuses selected')).toBeVisible();
  await filter(page, 'Filter statuses', null);
  await expect(page.locator('.goal-card')).toHaveCount(2);
  await page.getByRole('combobox', { name: 'Sort goals' }).selectOption('prompt');
  await expect(page.locator('.goal-card').first()).toContainText('Another goal');
  await page.getByRole('combobox', { name: 'Sort direction' }).selectOption('descend');
  await expect(page.locator('.goal-card').first()).toContainText('Build a reliable dashboard');
  await page.getByRole('checkbox', { name: 'Select all goals on this page', exact: true }).check();
  await expect(page.getByText('3 selected', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Expand goal details' }).first().click();
  const attempts = page.getByRole('region', { name: 'Attempts', exact: true });
  await expect(attempts.getByRole('button', { name: 'Details', exact: true })).toHaveCount(2);
  await attempts.getByRole('button', { name: 'Details', exact: true }).last().click();
  await expect(page).toHaveURL(/inspect=two/);
  const before = [...reads]; const subscriptions = await page.evaluate(() => (window as any).sseConnections);
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(page.getByRole('table').first()).toBeVisible();
  await expect(page.getByText('3 selected', { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(attempts.getByRole('button', { name: 'Details', exact: true }).last()).toHaveAttribute('aria-pressed', 'true');
  expect(reads).toEqual(before);
  expect(await page.evaluate(() => (window as any).sseConnections)).toBe(subscriptions);
  await page.getByRole('link', { name: 'Goal overview', exact: true }).first().click();
  await page.getByRole('link', { name: 'Back to Goals', exact: true }).click();
  await expect(page).toHaveURL(/expanded=/);
  await expect(page).toHaveURL(/inspect=/);
  await noOverflow(page);
});

test('drawer keyboard navigation closes, restores focus, and changes at the md boundary', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 767, height: 800 });
  await page.goto('/project/phone/goals');
  const opener = page.getByRole('button', { name: 'Open navigation' });
  await opener.focus(); await page.keyboard.press('Enter');
  await expect(page.getByRole('dialog')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(opener).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await opener.click();
  await page.screenshot({ path: testInfo.outputPath('phone-navigation.png'), animations: 'disabled' });
  await page.getByRole('dialog').getByRole('link', { name: 'Cells', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Cells', exact: true })).toBeVisible();
  await expect(opener).toBeFocused();
  await expect(page.getByText('Ref: ' + 'long-ref-'.repeat(30))).toBeVisible();
  await opener.click();
  await page.setViewportSize({ width: 768, height: 1024 });
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(opener).toHaveCount(0);
  await expect(page.locator('.ant-layout-sider')).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await noOverflow(page);
});

for (const viewport of [{ width: 320, height: 568 }, { width: 390, height: 844 }, { width: 768, height: 1024 }, { width: 1440, height: 900 }]) {
  test(`scoped pages and dialogs contain long content at ${viewport.width}`, async ({ page, context }, testInfo) => {
    await page.setViewportSize(viewport);
    for (const path of ['/goals', '/goals/g', '/jobs/one/story', '/cells', '/reviews']) {
      await page.goto('/project/phone' + path);
      await expect(page.getByRole('heading').first()).toBeVisible();
      await noOverflow(page);
    }
    await page.goto('/project/phone/goals');
    if (viewport.width === 390) await page.screenshot({ path: testInfo.outputPath('phone-goals.png'), fullPage: true });
    await page.getByRole('button', { name: 'New goal', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('Prompt').fill(prompt);
    await expect(dialog.getByText('Target: This cell (self)')).toBeVisible();
    await dialog.getByRole('combobox', { name: 'Cell', exact: true }).focus();
    await page.keyboard.press('ArrowDown');
    await page.getByTitle('api · https://github.com/colony-2/api.git', { exact: true }).click();
    await expect(dialog.getByText('Target: https://github.com/colony-2/api.git')).toBeVisible();
    await expect(page.locator('.ant-select-dropdown:visible')).toHaveCount(0);
    await noOverflow(page);
    if (viewport.width === 390) await page.screenshot({ path: testInfo.outputPath('phone-target.png'), fullPage: true });
    await page.setViewportSize({ ...viewport, height: 360 });
    await dialog.getByRole('button', { name: 'Create goal', exact: true }).scrollIntoViewIfNeeded();
    await expect(dialog.getByRole('button', { name: 'Create goal', exact: true })).toBeInViewport();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await context.clearCookies();
    await page.reload();
    await expect(page.getByPlaceholder('Email address')).toBeVisible();
    await noOverflow(page);
  });
}

test('phone story stacks panels, provides selected details access, and preserves node selection on resize', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/project/phone/jobs/one/story');
  const tree = page.getByRole('region', { name: 'Execution story tree' });
  await expect(tree).toBeVisible();
  const top = await page.locator('.story-tree-panel').boundingBox();
  const detail = await page.locator('#selected-story-details').boundingBox();
  expect(detail!.y).toBeGreaterThan(top!.y + top!.height);
  await page.getByRole('button', { name: 'Expand all', exact: true }).click();
  await tree.getByText('Task name ' + 'x'.repeat(200), { exact: true }).click();
  await expect(page).toHaveURL(/nodeId=/);
  const selectedURL = page.url();
  const link = page.getByRole('link', { name: 'View selected details' });
  await link.click();
  await expect(page.locator('#selected-story-details')).toBeInViewport();
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(page.url().split('#')[0]).toBe(selectedURL);
  await noOverflow(page);
  await page.screenshot({ path: testInfo.outputPath('phone-story.png'), fullPage: true });
});

test('compact controls meet touch and text-entry sizes and login fits', async ({ page, context }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/project/phone/goals');
  await page.getByRole('button', { name: /^Filters and sorting/ }).click();
  for (const name of ['Open navigation', 'Filter cells', 'New goal']) {
    const box = await page.getByRole('button', { name, exact: true }).boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
  }
  expect(await page.getByRole('searchbox', { name: 'Tenant ID' }).evaluate(element => parseFloat(getComputedStyle(element).fontSize))).toBeGreaterThanOrEqual(16);
  await context.clearCookies();
  await page.reload();
  await expect(page.getByPlaceholder('Email address')).toBeVisible();
  await noOverflow(page);
});

test('goal target modes validate before posting and cancellation starts a fresh draft', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const posts: unknown[] = [];
  page.on('request', request => { if (request.method() === 'POST') posts.push(request.postDataJSON()); });
  await page.goto('/project/phone/goals?cell=platform');
  await page.getByRole('button', { name: 'New goal', exact: true }).click();
  let dialog = page.getByRole('dialog');
  await expect(dialog.locator('.goal-target-summary')).toContainText('platform');
  await dialog.getByRole('radio', { name: 'Repository', exact: true }).check();
  await dialog.getByRole('textbox', { name: 'Repository', exact: true }).fill('github/colony-2/a?invalid');
  await dialog.getByLabel('Prompt').fill('An old draft');
  await dialog.getByRole('button', { name: 'Create goal', exact: true }).click();
  await expect(dialog.getByText('Enter github/colony-2/<name> or its GitHub HTTPS URL')).toBeVisible();
  expect(posts).toEqual([]);
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'New goal', exact: true })).toBeFocused();
  await page.getByRole('button', { name: 'New goal', exact: true }).click();
  dialog = page.getByRole('dialog');
  await expect(dialog.getByLabel('Prompt')).toHaveValue('');
  await expect(dialog.getByRole('radio', { name: 'Known cell' })).toBeChecked();
  const picker = dialog.getByRole('combobox', { name: 'Cell', exact: true });
  await picker.focus(); await picker.fill('api'); await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
  await expect(dialog.locator('.goal-target-summary')).toHaveText('Target: https://github.com/colony-2/api.git');
  await dialog.getByRole('button', { name: 'Create goal', exact: true }).click();
  await expect(dialog.getByText('Enter a prompt')).toBeVisible();
  expect(posts).toEqual([]);
});

for (const width of [320, 390]) {
  test(`short compact navigation anchors have actual touch areas at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    await page.route(/\/api\/projects\/[^/]+\/jobs(?:\?.*)?$/, route => route.fulfill({ json: { jobs: jobs.map(job => ({ ...job, prompt: 'X', repo: job.repo ? 'https://github.com/colony-2/a.git' : undefined, parent: { job_id: 'two', tenant_id: 'phone' } })) } }));
    await page.goto('/project/phone/goals');
    await page.getByRole('button', { name: 'Expand goal details' }).first().click();
    const links = page.locator('.compact-cards a');
    await expect(links.first()).toBeVisible();
    expect(await links.count()).toBeGreaterThan(10);
    for (const link of await links.all()) {
      await link.scrollIntoViewIfNeeded();
      const box = await link.boundingBox();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      // The padded edge belongs to the anchor, rather than an adjacent action.
      expect(await link.evaluate(element => {
        const box = element.getBoundingClientRect();
        return element.contains(document.elementFromPoint(box.x + 2, box.y + 2));
      })).toBe(true);
    }
    await page.screenshot({ path: testInfo.outputPath(`phone-touch-areas-${width}.png`), fullPage: true });
    const overview = page.getByRole('link', { name: 'Goal overview', exact: true }).first();
    await overview.click({ position: { x: 2, y: 2 } });
    await expect(page).toHaveURL(/\/goals\/g$/);
    await page.getByRole('link', { name: 'Back to Goals', exact: true }).click();
    const opener = page.getByRole('button', { name: 'Open navigation' });
    await opener.click();
    for (const link of await page.getByRole('dialog').getByRole('link').all()) {
      const box = await link.boundingBox();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await page.getByRole('dialog').getByRole('link', { name: 'Cells', exact: true }).click({ position: { x: 2, y: 2 } });
    await expect(page).toHaveURL(/\/cells$/);
    await expect(opener).toBeFocused();
    await noOverflow(page);
  });
}

test.describe('native browser zoom', () => {
  test.use({ nativeZoom: true });
  test('200% zoom retains navigation and new-goal, batch, restart actions', async ({ page }, testInfo) => {
    await page.goto('/project/phone/goals');
    expect(await page.evaluate(() => ({ width: innerWidth, dpr: devicePixelRatio }))).toEqual({ width: 1280, dpr: 1 });
    await setNativeZoom(page, 2);
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await captureNativeZoom(page, testInfo.outputPath('native-200-navigation.png'));
    await page.getByRole('dialog').getByRole('link', { name: 'Cells', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Open navigation' })).toBeFocused();
    await noOverflow(page);
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await page.getByRole('dialog').getByRole('link', { name: 'Goals', exact: true }).click();
    const newGoal = page.getByRole('button', { name: 'New goal', exact: true });
    await newGoal.click();
    let dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'Create goal', exact: true }).click();
    await expect(dialog.getByText('Enter a prompt')).toBeVisible();
    await dialog.getByLabel('Prompt').fill(prompt);
    await noOverflow(page);
    await dialog.getByRole('button', { name: 'Create goal', exact: true }).scrollIntoViewIfNeeded();
    await expect(dialog.getByRole('button', { name: 'Create goal', exact: true })).toBeInViewport({ ratio: 1 });
    await captureNativeZoom(page, testInfo.outputPath('native-200-new-goal.png'));
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).focus();
    await page.keyboard.press('Enter');
    await expect(newGoal).toBeFocused();
    await page.getByRole('checkbox', { name: 'Select all goals on this page', exact: true }).check();
    for (const action of ['Cancel', 'Delete']) {
      const trigger = page.getByRole('button', { name: `${action} selected`, exact: true });
      await trigger.click();
      dialog = page.getByRole('dialog');
      const confirm = dialog.getByRole('button', { name: `${action} 3 jobs`, exact: true });
      await confirm.scrollIntoViewIfNeeded();
      await expect(confirm).toBeInViewport({ ratio: 1 });
      await noOverflow(page);
      await captureNativeZoom(page, testInfo.outputPath(`native-200-${action.toLowerCase()}-batch.png`));
      await dialog.getByRole('button', { name: 'Keep jobs', exact: true }).click();
      await expect(trigger).toBeFocused();
    }
    await page.route(/\/api\/projects\/[^/]+\/jobs\/one\/story$/, route => route.fulfill({ json: {
      job_id: 'one', status: 'completed',
      restart_scopes: [{ scope_id: 'scope', chapter_ordinal: 1, title: 'Recorded point', started_at: '2026-10-30T10:00:00Z' }],
      root: { id: 'root', kind: 'recipe', title: 'Phone recipe', path: [], attempt: 1, status: 'completed', restart_scope_id: 'scope', restart_chapter_ordinal: 1 },
    } }));
    await page.goto('/project/phone/jobs/one/story');
    const restart = page.getByRole('button', { name: 'Restart job', exact: true });
    await restart.click();
    dialog = page.getByRole('dialog', { name: 'Restart Job', exact: true });
    const confirm = dialog.getByRole('button', { name: 'Restart', exact: true });
    await expect(confirm).toBeEnabled();
    await confirm.scrollIntoViewIfNeeded();
    await expect(confirm).toBeInViewport({ ratio: 1 });
    await dialog.getByRole('combobox', { name: 'Restart point', exact: true }).focus();
    await expect(dialog.getByRole('combobox', { name: 'Restart point', exact: true })).toBeFocused();
    await noOverflow(page);
    await captureNativeZoom(page, testInfo.outputPath('native-200-restart.png'));
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(restart).toBeFocused();
    await setNativeZoom(page, 1);
    await expect(page.getByRole('button', { name: 'Open navigation' })).toHaveCount(0);
    await testInfo.attach('native-zoom-method', { body: `${await page.evaluate(() => navigator.userAgent)}; chrome.tabs.setZoom/getZoom=2; fixed 1280x900 viewport; innerWidth=640, innerHeight=450, devicePixelRatio=2; restored 100% after validation.`, contentType: 'text/plain' });
  });
});

for (const width of [320, 390]) {
  test(`story overview and short artifact links have touch areas at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    await page.route(/\/api\/projects\/[^/]+\/jobs\/one\/story$/, route => route.fulfill({ json: {
      job_id: 'one', status: 'completed', root: {
        id: 'root', kind: 'recipe', title: 'Phone recipe', path: [], attempt: 1, status: 'completed',
        artifact_keys: [{ taskOrdinal: 1, name: 'a', sizeBytes: 12 }],
      },
    } }));
    const artifactRequests: string[] = [];
    await page.route('**/jobs/one/tasks/1/artifacts/a', route => {
      artifactRequests.push(route.request().url());
      return route.fulfill({ contentType: 'text/plain', body: 'Artifact body' });
    });
    await page.goto('/project/phone/jobs/one/story');
    // Select by accessible action names, independently of their CSS treatment.
    const overview = page.getByRole('link', { name: 'Goal overview', exact: true });
    const assertTouchArea = async (link: ReturnType<Page['getByRole']>) => {
      await link.scrollIntoViewIfNeeded();
      const box = await link.boundingBox();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(await link.evaluate(element => {
        const box = element.getBoundingClientRect();
        return [[2, 2], [box.width - 2, box.height - 2]].every(([x, y]) =>
          element.contains(document.elementFromPoint(box.x + x, box.y + y)));
      })).toBe(true);
    };
    await assertTouchArea(overview);
    await overview.click({ position: { x: 2, y: 2 } });
    await expect(page).toHaveURL(/\/goals\/g$/);
    await page.goBack();
    await page.getByRole('tab', { name: 'Artifacts (1)', exact: true }).click();
    const artifact = page.getByRole('link', { name: 'a', exact: true });
    await assertTouchArea(artifact);
    await noOverflow(page);
    await page.screenshot({ path: testInfo.outputPath(`phone-story-touch-${width}.png`), fullPage: true, animations: 'disabled' });
    await artifact.click({ position: { x: 2, y: 2 } });
    const viewer = page.getByRole('dialog', { name: 'a', exact: true });
    await expect(viewer.getByText('Artifact body', { exact: true })).toBeVisible();
    expect(artifactRequests).toHaveLength(1);
    await noOverflow(page);
    await viewer.locator('.ant-modal-footer').getByRole('button', { name: 'Close', exact: true }).click();
    await expect(viewer).toHaveCount(0);
    await expect(artifact).toBeVisible();
  });
}

test('phone goals start with concise cards and disclose existing controls and metadata', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/project/phone/goals');
  await expect(page.getByRole('button', { name: 'Filter cells', exact: true })).toBeHidden();
  await expect(page.getByRole('textbox', { name: 'Search goals' })).toBeHidden();
  await expect(page.getByRole('button', { name: 'Cancel selected', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeHidden();
  const card = page.locator('.goal-card').first();
  await expect(card.getByRole('link', { name: 'Goal overview', exact: true })).toHaveCount(0);
  expect((await card.boundingBox())!.height).toBeLessThan(210);
  expect((await card.boundingBox())!.y).toBeLessThan(410);
  await page.screenshot({ path: testInfo.outputPath('phone-goals-concise.png'), fullPage: true });
  await page.getByRole('button', { name: /^Filters and sorting/ }).click();
  await expect(page.getByRole('button', { name: 'Filter cells', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toHaveCount(1);
  await page.getByRole('textbox', { name: 'Search goals' }).fill('Another');
  await expect(page.locator('.goal-card')).toHaveCount(1);
  await page.getByRole('button', { name: /^Filters and sorting/ }).click();
  await expect(page.getByRole('link', { name: 'Another goal', exact: true })).toBeVisible();
  await page.locator('.goal-card').getByRole('button', { name: 'Expand goal details' }).click();
  await expect(page.getByRole('link', { name: 'Goal overview', exact: true })).toBeVisible();
  await noOverflow(page);
});

for (const width of [320, 390]) {
  test(`phone review reads large documents in one full-screen scroller at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    let reads = 0;
    const source = '# Large design\n\n' + 'A paragraph with enough detail to review comfortably.\n\n'.repeat(350) + 'Last paragraph.';
    await page.route('**/user-inputs/one', route => route.fulfill({ json: {
      jobId: 'one', status: 'pending', startTime: '2026-10-30T10:00:00Z', form: {
        kind: 'review', title: 'Review the design', request_id: 'request',
        documents: { design: { kind: 'stored', name: 'design.md', stored: { key: { jobId: 'one', taskOrdinal: 1, name: 'design.md' } } } },
        fields: [
          { id: 'decision', type: 'multiple_choice', question: 'Review instructions', required: true, options: [{ value: 'approve', label: 'Approve' }, { value: 'revise', label: 'Request changes' }] },
          { id: 'feedback', type: 'paragraph_text', question: 'Feedback', required: true },
          { id: 'attachment', type: 'file_upload', question: 'Annotations' },
        ],
      },
    } }));
    await page.route('**/reviews/one/documents?*', route => { reads++; return route.fulfill({ contentType: 'text/markdown', body: source }); });
    await page.goto('/project/phone/reviews/one');
    const feedback = page.getByLabel('Feedback', { exact: true });
    await feedback.fill('Keep my draft while reading.');
    await page.locator('input[type=file]').setInputFiles({ name: 'notes.md', mimeType: 'text/markdown', buffer: Buffer.from('Notes') });
    await expect(page.getByLabel('Rendered document')).toHaveCount(0);
    expect(reads).toBe(0);
    await page.screenshot({ path: testInfo.outputPath(`phone-review-response-${width}.png`), fullPage: true });
    const read = page.getByRole('button', { name: 'Read design.md', exact: true });
    await read.click();
    const reader = page.getByRole('dialog', { name: 'design.md', exact: true });
    const article = reader.getByLabel('Rendered document');
    await expect(article).toContainText('Last paragraph.');
    expect(await article.evaluate(element => getComputedStyle(element).maxHeight)).toBe('none');
    expect(await article.evaluate(element => element.scrollHeight <= element.clientHeight + 1)).toBe(true);
    const scroller = page.locator('.review-reader .ant-drawer-body');
    expect(await scroller.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true);
    expect(await page.evaluate(() => getComputedStyle(document.body).overflowY)).toBe('hidden');
    await scroller.evaluate(element => { element.scrollTop = element.scrollHeight; });
    await expect(reader.getByRole('button', { name: 'Back to review', exact: true })).toBeInViewport({ ratio: 1 });
    await noOverflow(page);
    await reader.getByText('Document options', { exact: true }).click();
    await reader.getByText('Markdown', { exact: true }).click();
    await expect(reader.getByLabel('Markdown source')).toHaveText(source);
    expect(await reader.getByLabel('Markdown source').evaluate(element => element.scrollHeight <= element.clientHeight + 1)).toBe(true);
    await expect(reader.getByRole('link', { name: 'Download', exact: true })).toHaveAttribute('download', 'design.md');
    await page.screenshot({ path: testInfo.outputPath(`phone-review-reader-${width}.png`) });
    await reader.getByRole('button', { name: 'Back to review', exact: true }).click();
    await expect(reader).toHaveCount(0);
    await expect(feedback).toHaveValue('Keep my draft while reading.');
    await expect(page.getByText('notes.md', { exact: true })).toBeVisible();
    await expect(read).toBeFocused();
    await read.click();
    await expect(page.getByRole('dialog').getByLabel('Markdown source')).toHaveText(source);
    expect(reads).toBe(1);
    await page.keyboard.press('Escape');
    await expect(reader).toHaveCount(0);
    await expect(feedback).toHaveValue('Keep my draft while reading.');
  });
}
