import { openCompose, inboxFolder, storyAction } from './helpers/inbox';
import { routeEnvironment, captureEnvironment, longBase, envJobs } from './helpers/executionRequirements';
import { jobIdentity, mockCells, cellFixture } from './helpers/cells';
import { expect, type Page } from '@playwright/test';
import { test, setNativeZoom, captureNativeZoom } from './helpers/nativeZoom';

const prompt = 'Build a reliable dashboard ' + 'long-content-'.repeat(30);
const jobs = [
  { ...jobIdentity, tenant_id: 'phone', job_id: 'one', goal_id: 'g', cell_name: 'platform', repo: 'https://github.com/colony-2/platform.git', prompt, status: 'READY', store: 'ACTIVE', created_at: '2026-10-30T10:00:00Z' },
  { ...jobIdentity, tenant_id: 'phone', job_id: 'two', goal_id: 'g', cell_name: 'platform', prompt, status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success', created_at: '2026-10-29T10:00:00Z' },
  { ...jobIdentity, tenant_id: 'phone', cell_id: 'cell-api', cell_name: 'api', job_id: 'three', goal_id: 'other', prompt: 'Another goal', status: 'READY', store: 'ACTIVE', created_at: '2026-10-28T10:00:00Z' },
];
async function noOverflow(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}
test.beforeEach(async ({ page, context, baseURL }) => {
  await page.route('**/input-artifacts', route => route.fulfill({json:{artifacts:[]}}));
  await page.route('**/children?*', route => route.fulfill({json:{jobs:[]}}));
  await page.route('**/workspace-diff*', route => route.fulfill({json:{status:'absent',comparisons:[]}}));
  await page.route('**/conversation?*', route => route.fulfill({ status: 501, json: { error: 'Fixture has no retained history', code: 'history_unsupported' } }));
  await context.addCookies([{ name: 'colony2:user:email', value: 'phone@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).sseConnections = 0;
    (window as any).EventSource = class {
      readyState = 1;
      constructor() { (window as any).sseConnections++; }
      close() {} addEventListener() {} removeEventListener() {}
    };
  });
  await page.route('**/api/projects', route => route.fulfill({ json: [{ ...jobIdentity, tenant_id: 'phone' }] }));
  await mockCells(page, ['platform', 'api']);
  await page.route(/\/api\/projects\/[^/]+\/user-inputs\/pending$/, route => route.fulfill({ json: [] }));
  await page.route(/\/api\/projects\/[^/]+\/jobs(?:\?.*)?$/, route => { const names = new URL(route.request().url()).searchParams.getAll('cellNames'); return route.fulfill({ json: { jobs: jobs.filter(job => !names.length || names.includes(job.cell_name)) } }); });
  await page.route(url => /^\/api\/projects\/[^/]+\/jobs\/[^/]+$/.test(url.pathname), route => {
    const id = new URL(route.request().url()).pathname.split('/').pop();
    return route.fulfill({ json: { ...jobs.find(job => job.job_id === id), tenant_id: 'phone' } });
  });
  await page.route(/\/api\/projects\/[^/]+\/jobs\/[^/]+\/story$/, route => route.fulfill({ json: { ...jobIdentity, tenant_id: 'phone', job_id: 'one', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'Phone recipe', path: [], attempt: 1, status: 'completed', children: [{ id: 'child', kind: 'op', title: 'Task name ' + 'x'.repeat(200), path: ['step'], attempt: 1, status: 'completed', output: { data: 'value'.repeat(200) } }] } } }));
});


// Inbox rows replace goal cards/filter dialogs. Check the same native mobile
// navigation, selection, resize, text-entry and action paths on the new surface.
for (const width of [320,390,768,1440]) test(`inbox and Compose fit long content at ${width}px`,async({page,context},info)=>{
  await page.setViewportSize({width,height:900});
  await page.goto('/project/phone/jobs');
  await expect(page.locator('.inbox-job-row')).toHaveCount(3);
  await noOverflow(page);
  await inboxFolder(page,'api');
  await expect(page.locator('.inbox-job-row')).toHaveCount(1);
  await expect(page.getByRole('heading',{name:'api',exact:true})).toBeVisible();
  const saved=page.url();
  await page.getByRole('link',{name:'api: Another goal'}).click();
  await page.getByRole('link',{name:'Back to inbox'}).click();
  await expect(page).toHaveURL(saved);
  await inboxFolder(page,'All');
  for(const id of ['one','two','three']) await page.getByRole('checkbox',{name:`Select job ${id}`,exact:true}).check();
  for(const name of ['Cancel','Archive']){
    const trigger=page.getByRole('button',{name:`${name} selected`,exact:true});await trigger.click();
    const dialog=page.getByRole('dialog');
    await expect(dialog.getByRole('button',{name:`${name} ${name === 'Cancel' ? 2 : 3} jobs`,exact:true})).toBeVisible();
    await dialog.getByRole('button',{name:'Keep jobs'}).click();await expect(trigger).toBeFocused();
  }
  const composer=await openCompose(page,'api');
  await composer.getByLabel('Message',{exact:true}).fill(prompt);
  await noOverflow(page);
  await page.setViewportSize({width,height:360});
  const send=composer.getByRole('button',{name:'Send',exact:true});await send.scrollIntoViewIfNeeded();
  await expect(send).toBeInViewport({ratio:1});
  await composer.getByRole('button',{name:'Close composer'}).click();
  await inboxFolder(page, 'Drafts');
  await page.getByRole('list', { name: 'Drafts' }).getByRole('button', { name: /Build a reliable dashboard/ }).click();
  await expect(composer.getByLabel('Message',{exact:true})).toHaveText(prompt);
  await composer.getByRole('button',{name:'Discard draft',exact:true}).click();await openCompose(page);
  await expect(composer.getByLabel('Message',{exact:true})).toHaveText('');
  await composer.getByRole('button',{name:'Discard draft',exact:true}).click();
  await context.clearCookies();await page.reload();
  await expect(page.getByPlaceholder('Email address')).toBeVisible();await noOverflow(page);
});

for(const width of [320,390]) test(`inbox links and drawer have actual touch areas at ${width}px`,async({page})=>{
  await page.setViewportSize({width,height:844});await page.goto('/project/phone/jobs');
  const links=page.locator('.inbox-job-link');await expect(links).toHaveCount(3);
  for(const link of await links.all()){
    const r=(await link.boundingBox())!;expect(r.width).toBeGreaterThanOrEqual(44);expect(r.height).toBeGreaterThanOrEqual(44);
    expect(await link.evaluate(el=>{const r=el.getBoundingClientRect();return el.contains(document.elementFromPoint(r.x+2,r.y+2));})).toBe(true);
  }
  const opener=page.getByRole('button',{name:'Open navigation'});await opener.press('Enter');
  const dialog=page.getByRole('dialog');
  for(const link of await dialog.getByRole('link').all()){
    const r=(await link.boundingBox())!;expect(r.width).toBeGreaterThanOrEqual(44);expect(r.height).toBeGreaterThanOrEqual(44);
  }
  if(width===390 && process.env.CORTEX_INBOX_REPAIR_CAPTURE) await page.screenshot({path:`${process.env.CORTEX_INBOX_REPAIR_CAPTURE}/inbox-navigation-phone.png`,animations:'disabled'});
  await dialog.getByRole('link',{name:'Archive',exact:true}).click({position:{x:2,y:2}});
  await expect(opener).toBeFocused();await expect(page.getByRole('heading',{name:'Archive',exact:true})).toBeVisible();
  await page.setViewportSize({width:768,height:900});await expect(opener).toHaveCount(0);await noOverflow(page);
});

test.describe('native browser zoom',()=>{
  test.use({nativeZoom:true});
  test('200% zoom retains Compose, batch and restart actions',async({page},info)=>{
    await page.goto('/project/phone/jobs');await setNativeZoom(page,2);
    const composer=await openCompose(page,'platform');
    await composer.getByRole('button',{name:'Send',exact:true}).click();
    await expect(composer.getByRole('alert')).toHaveText('Enter a message with text or an image');
    const message=composer.getByLabel('Message',{exact:true});
    await expect(message).toHaveAttribute('aria-invalid','true');
    await expect(message).toHaveAccessibleDescription('Enter a message with text or an image');
    await message.fill(prompt);
    const send=composer.getByRole('button',{name:'Send',exact:true});await send.scrollIntoViewIfNeeded();await expect(send).toBeInViewport({ratio:1});await noOverflow(page);
    await captureNativeZoom(page,info.outputPath('native-200-compose.png'));
    await composer.getByRole('button',{name:'Discard'}).click();
    for(const id of ['one','two','three']) await page.getByRole('checkbox',{name:`Select job ${id}`,exact:true}).check();
    for(const name of ['Cancel','Archive']){
      const trigger=page.getByRole('button',{name:`${name} selected`});await trigger.click();const dialog=page.getByRole('dialog');
      const confirm=dialog.getByRole('button',{name:`${name} ${name === 'Cancel' ? 2 : 3} jobs`,exact:true});await confirm.scrollIntoViewIfNeeded();await expect(confirm).toBeInViewport({ratio:1});await noOverflow(page);
      await dialog.getByRole('button',{name:'Keep jobs'}).click();await expect(trigger).toBeFocused();
    }
    await page.goto('/project/phone/jobs/one/story');
    const restart=await storyAction(page,'Restart');await restart.click();
    const dialog=page.getByRole('dialog');await expect(dialog.getByRole('combobox',{name:'Restart point'})).toBeVisible();
    await dialog.getByRole('button',{name:'Cancel',exact:true}).click();await expect(page.locator('.inbox-story-more > summary')).toBeFocused();
    await setNativeZoom(page,1);await expect(page.getByRole('button',{name:'Open navigation'})).toHaveCount(0);
  });
});

for(const width of [320,390,768,1440]) test(`ENV exact-job tab retains full requirements through failed Trace at ${width}px`,async({page},info)=>{
  await page.setViewportSize({width,height:1000});const reads=await routeEnvironment(page);
  await page.goto('/project/env/jobs/long/story?tab=env');
  await expect(page.getByRole('tab',{name:'Execution',exact:true})).toHaveAttribute('aria-selected','true');
  await expect(page.getByText(longBase,{exact:true})).toBeVisible();
  await expect(page.getByText('0.5 cores',{exact:true})).toBeVisible();
  await expect(page.getByText('512 MiB',{exact:true})).toBeVisible();
  await expect(page.getByText('a-long-capability-name-with-雪',{exact:true})).toBeVisible();
  expect(reads.some(path=>/\/(story|input-artifacts|conversation)$/.test(path))).toBe(false);
  await noOverflow(page);await captureEnvironment(page,info,`env-${width}`);
  await page.getByRole('tab',{name:'Trace',exact:true}).click();await expect(page.getByText('Failed to load job story',{exact:true})).toBeVisible();
  await page.getByRole('tab',{name:'Execution',exact:true}).click();await expect(page.getByText(longBase,{exact:true})).toBeVisible();await noOverflow(page);
});
test('phone story stacks panels, provides selected details access, and preserves node selection on resize', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/project/phone/jobs/one/story');
  await page.getByRole('tab', { name: 'Trace', exact: true }).click();
  const tree = page.getByRole('region', { name: 'Execution story tree' });
  await expect(tree).toBeVisible();
  const top = await page.locator('.story-tree-panel').boundingBox();
  const detail = await page.locator('#selected-story-details').boundingBox();
  expect(detail!.y).toBeGreaterThan(top!.y + top!.height);
  await page.getByRole('button', { name: 'Expand all', exact: true }).click();
  await tree.getByText('Task name ' + 'x'.repeat(200), { exact: true }).click();
  await expect(page).toHaveURL(/nodeId=/);
  const selectedURL = page.url();
  const link = page.getByRole('link', { name: 'View selected details' }).first();
  await link.click();
  await expect(page.locator('#selected-story-details')).toBeInViewport();
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(page.url().split('#')[0]).toBe(selectedURL);
  await noOverflow(page);
  await page.screenshot({ path: testInfo.outputPath('phone-story.png'), fullPage: true });
});

for (const width of [320, 390]) {
  test(`story return and short artifact links have touch areas at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    await page.route(/\/api\/projects\/[^/]+\/jobs\/one\/story$/, route => route.fulfill({ json: {
      ...jobIdentity, job_id: 'one', status: 'completed', root: {
        id: 'root', kind: 'recipe', title: 'Phone recipe', path: [], attempt: 1, status: 'completed',
        artifact_keys: [{ taskOrdinal: 1, name: 'a', sizeBytes: 12 }],
      },
    } }));
    const artifactRequests: string[] = [];
    // The shared viewer loads in an effect. Development Strict Mode aborts
    // its first mount; count completed loads, not the cancelled request.
    page.on('requestfinished', request => {
      if (new URL(request.url()).pathname.endsWith('/jobs/one/tasks/1/artifacts/a')) artifactRequests.push(request.url());
    });
    await page.route('**/jobs/one/tasks/1/artifacts/a', route => {
      return route.fulfill({ contentType: 'text/plain', body: 'Artifact body' });
    });
    await page.goto('/project/phone/jobs/one/story');
  await page.getByRole('tab', { name: 'Trace', exact: true }).click();
    // Select by accessible action names, independently of their CSS treatment.
    const overview = page.getByRole('link', { name: 'Back to inbox', exact: true });
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
    await expect(page).toHaveURL(/\/jobs$/);
    await page.goBack();
    await page.getByRole('tab', { name: 'Outbox (1)', exact: true }).click();
    const artifact = page.getByRole('link', { name: 'a', exact: true });
    await assertTouchArea(artifact);
    await noOverflow(page);
    await page.screenshot({ path: testInfo.outputPath(`phone-story-touch-${width}.png`), fullPage: true, animations: 'disabled' });
    await artifact.click({ position: { x: 2, y: 2 } });
    const viewer = page.getByRole('dialog', { name: 'a', exact: true });
    await expect(viewer.getByText('Artifact body', { exact: true })).toBeVisible();
    await expect.poll(() => artifactRequests.length).toBe(1);
    await noOverflow(page);
    await viewer.locator('.ant-modal-footer').getByRole('button', { name: 'Close', exact: true }).click();
    await expect(viewer).toHaveCount(0);
    await expect(artifact).toBeVisible();
  });
}

for (const width of [320, 390]) {
  test(`phone review reads large documents in the page scroller at ${width}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 844 });
    let reads = 0;
    await page.route('**/jobs/one/children?*', route => route.fulfill({ json: { jobs: [] } }));
    await page.route('**/user-inputs/pending', route => route.fulfill({ json: [{ ...jobIdentity, id: 'one', client_payload_revision: 1, request_id: 'request' }] }));
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
    page.on('requestfinished', request => { if (request.url().includes('/stored-document?')) reads++; });
    await page.route('**/stored-document?*', route => route.fulfill({ contentType: 'text/markdown', body: source }));
    await page.goto('/project/phone/reviews/one');
    await page.getByRole('button', { name: 'Approve', exact: true }).click();
    const feedback = page.getByLabel('Feedback', { exact: true });
    await feedback.fill('Keep my draft while reading.');
    await page.getByRole('region', { name: 'Review decisions', exact: true }).locator('input[type=file]').setInputFiles({ name: 'notes.md', mimeType: 'text/markdown', buffer: Buffer.from('Notes') });
    await expect(page.getByLabel('Rendered document')).toContainText('Last paragraph.');
    await page.getByRole('button', { name: 'Show documents', exact: true }).click();
    const read = page.getByRole('button', { name: 'Read design.md', exact: true });
    await read.click();
    const article = page.getByLabel('Rendered document');
    expect(await article.evaluate(element => element.scrollHeight <= element.clientHeight + 1)).toBe(true);
    expect(await page.evaluate(() => getComputedStyle(document.body).overflowY)).not.toBe('hidden');
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await page.getByRole('button', { name: 'Document options' }).click();
    await page.getByText('Markdown', { exact: true }).click();
    await page.keyboard.press('Escape');
    await expect(page.getByLabel('Markdown source')).toHaveText(source);
    expect(await page.getByLabel('Markdown source').evaluate(element => element.scrollHeight <= element.clientHeight + 1)).toBe(true);
    await expect(page.getByRole('link', { name: 'Download', exact: true })).toHaveAttribute('download', 'design.md');
    await noOverflow(page);
    await page.screenshot({ path: testInfo.outputPath(`phone-review-inline-${width}.png`) });
    await expect(feedback).toHaveValue('Keep my draft while reading.');
    await expect(page.getByText('notes.md', { exact: true })).toBeVisible();
    await read.click();
    await expect(page.getByLabel('Markdown source')).toHaveText(source);
    expect(reads).toBe(1);
    await expect(feedback).toHaveValue('Keep my draft while reading.');
  });
}
