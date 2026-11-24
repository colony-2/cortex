import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { jobIdentity, mockCells, registerCell } from './helpers/cells';
import { inboxFolder, storyAction } from './helpers/inbox';

// Goal-grouping, status filters, submitted-time and chained-sort controls were
// replaced by flat inboxes. Preserve data/action coverage through current UI.
test.beforeEach(async ({page}, info) => {
  if (info.project.name === 'goals-runtime') return;
  await page.addInitScript(() => { (window as any).EventSource = class { readyState=1; close(){} addEventListener(){} removeEventListener(){} }; });
  await page.route('**/api/projects', route=>route.fulfill({json:[{tenant_id:'copy'}]}));
  await page.route('**/user-inputs/pending', route=>route.fulfill({json:[]}));
});

for (const width of [1440, 390]) {
  test(`AR-018,019 story archive returns to inbox with one toast and retained history at ${width}px`, async ({ page, request, context, baseURL }, testInfo) => {
    test.skip(testInfo.project.name !== 'goals-runtime', 'Requires the disposable SQLite runtime in the goals-runtime project');
    test.setTimeout(60_000);
    await page.setViewportSize({ width, height: 1000 });
    const tenant = `story-archive-${randomUUID()}`;
    await registerCell(request, tenant);
    const base = `/api/projects/${tenant}/jobs`;
    const submitted = await request.post(base, { data: { cell: 'platform', job_id: 'original-attempt', recipe: 'smoke', inputs: { prompt: 'Review the release dashboard' } } });
    expect(submitted.status(), await submitted.text()).toBe(201);
    const source = await submitted.json();
    const backend = `http://127.0.0.1:18082/v1/tenants/${tenant}/jobs/original-attempt`;
    const acquired = await request.post(`${backend}/lease`, { data: { workerId: 'archive-browser-fixture', routes: [{ jobType: 'recipe' }], leaseDuration: '1m' } });
    expect(acquired.ok(), await acquired.text()).toBeTruthy();
    const { lease } = await acquired.json();
    const complete = await request.post(`${backend}/leases/${lease.leaseId}/complete`, {
      headers: { 'X-JobDB-Lease-Token': lease.leaseToken },
      data: { status: 'failed_app', chapter: { ordinal: 1, taskType: 'recipe', createdAt: new Date().toISOString(),
        body: { kind: 'jobAttemptOutcome', outcome: { kind: 'appError', error: { code: 'review_required', message: 'A further attempt is needed' } } }, artifacts: [] } },
    });
    expect(complete.status(), await complete.text()).toBe(204);
    const restart = await request.post(`${base}/original-attempt/restart`, { data: { request_id: randomUUID(), chapter_ordinal: 0, goal: { mode: 'continue' } } });
    expect(restart.status(), await restart.text()).toBe(201);
    const sibling = await restart.json();
    expect(sibling.goal_id).toBe(source.goal_id);
    await context.addCookies([{ name: 'colony2:user:email', value: 'browser@example.com', url: baseURL! }]);
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    const mutations: string[] = [];
    page.on('request', req => { if (req.method() === 'DELETE') mutations.push(new URL(req.url()).pathname); });
    const capture = async (name: string) => {
      if (!process.env.CORTEX_ARCHIVE_SCREENSHOTS) return;
      mkdirSync(process.env.CORTEX_ARCHIVE_SCREENSHOTS, { recursive: true });
      await page.screenshot({ path: join(process.env.CORTEX_ARCHIVE_SCREENSHOTS, `${name}-${width}.png`), fullPage: true, animations: 'disabled' });
    };

    // Finished source: immediate archive, with the saved All jobs destination.
    const savedList = `/project/${tenant}/jobs?inbox=all&q=dashboard`;
    await page.goto(savedList);
    await page.locator('.inbox-job-link[href*="/jobs/original-attempt/story"]').click();
    await expect(page.getByRole('tab', { name: 'Thread', exact: true })).toHaveAttribute('aria-selected', 'true');
    if (width === 1440) await capture('story-before-archive');
    await (await storyAction(page, 'Archive')).click();
    await expect(page.getByRole('heading', { name: 'All', exact: true })).toBeVisible();
    await expect(page).toHaveURL(savedList);
    const toast = page.locator('.ant-message-notice-content').filter({ hasText: 'Archived 1 job' });
    await expect(toast).toHaveCount(1);
    await expect(toast).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Thread', exact: true })).toHaveCount(0);
    await expect(page.getByRole('checkbox', { name: 'Select job original-attempt', exact: true })).toHaveCount(0);
    await inboxFolder(page, 'Archive');
    await expect(page.getByRole('checkbox', { name: 'Select job original-attempt', exact: true })).toBeVisible();
    await capture('goals-archived-toast');
    await inboxFolder(page, 'All');
    await expect(page.getByRole('checkbox', { name: 'Select job original-attempt', exact: true })).toHaveCount(0);
    await expect(page.getByRole('checkbox', { name: `Select job ${sibling.job_id}`, exact: true })).toBeVisible();
    const inbox = await (await request.get(`${base}?status=all`)).json();
    expect(inbox.jobs.map((job: { job_id: string }) => job.job_id)).toEqual([sibling.job_id]);
    const retained = await (await request.get(`${base}?status=all&includeHidden=true`)).json();
    expect(retained.jobs.map((job: { job_id: string }) => job.job_id).sort()).toEqual(['original-attempt', sibling.job_id].sort());
    expect(mutations).toEqual([`${base}/original-attempt`]);

    // Unfinished sibling: keep existing confirmation, then return to Inbox.
    await expect(toast).toHaveCount(0);
    await page.locator(`.inbox-job-link[href*="/jobs/${sibling.job_id}/story"]`).click();
    await (await storyAction(page, 'Archive')).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByLabel('Cancellation message (optional)')).toBeVisible();
    await dialog.getByRole('button', { name: 'Archive job', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'All', exact: true })).toBeVisible();
    await expect(toast).toHaveCount(1);
    await expect(toast).toBeVisible();
    await expect(page.getByRole('checkbox', { name: /^Select job / })).toHaveCount(0);
    await expect(page.getByText('No jobs in this inbox', { exact: true })).toBeVisible();
    if (width === 390) await capture('goals-confirmed-archive-toast');
    expect(mutations).toEqual([`${base}/original-attempt`, `${base}/${sibling.job_id}`]);
    expect(errors).toEqual([]);
  });
}


for (const width of [1440, 390]) {
  test(`inbox loads every cursor, keeps attempts separate and orders by persisted updates at ${width}px`, async ({ page, context, baseURL }) => {
    await page.setViewportSize({width, height: 900});
    await context.addCookies([{name:'colony2:user:email',value:'inbox@example.com',url:baseURL!}]);
    await mockCells(page, ['platform', 'api']);
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if(path.endsWith('/cells')) return route.fallback();
      return route.fulfill({json:path.endsWith('/projects')?[{tenant_id:'flat'}]:path.endsWith('/pending')?[]:{}});
    });
    const jobs = Array.from({length:200}, (_, index) => ({...jobIdentity,tenant_id:'flat',job_id:`job-${String(index).padStart(3,'0')}`,goal_id:'shared-goal',title:`Job ${index}`,prompt:`Literal <script>unsafe()</script> 雪 ${index}`,status:index?'READY':'COMPLETED',store:index?'ACTIVE':'ARCHIVED',archived_at:index?undefined:'2026-11-20T12:00:00Z',created_at:'2026-11-10T00:00:00Z'}));
    const queries: URLSearchParams[] = [], detailReads:string[]=[];
    await page.route('**/api/projects/flat/jobs?*', route => {
      const q=new URL(route.request().url()).searchParams; queries.push(q);
      return route.fulfill({json:q.get('pageToken')==='empty'?{jobs:[],next_page_token:'last'}:q.get('pageToken')==='last'?{jobs:[{...jobs[1],job_id:'hidden',title:'Hidden job'}].filter(()=>q.get('includeHidden')==='true')}:{jobs,next_page_token:'empty'}});
    });
    page.on('request',r=>{if(/\/jobs\/[^/?]+$/.test(new URL(r.url()).pathname))detailReads.push(r.url());});
    await page.goto('/project/flat/goals');
    await expect(page).toHaveURL('/project/flat/jobs');
    await expect(page.getByRole('list',{name:'Jobs'}).getByRole('listitem')).toHaveCount(200);
    expect(queries.map(q=>q.get('pageToken'))).toEqual([null]);
    const more = page.getByRole('button', { name: 'Load more jobs' });
    await more.click();
    await expect.poll(() => queries.map(q=>q.get('pageToken'))).toEqual([null,'empty']);
    await more.click();
    await expect(more).toHaveCount(0);
    expect(queries.map(q=>q.get('pageToken'))).toEqual([null,'empty','last']);
    expect(queries.map(q=>q.get('includeHidden'))).toEqual(['false','false','false']);
    expect(queries.every(q=>q.get('pageSize')==='200'&&q.get('status')==='all')).toBe(true);
    expect(detailReads).toEqual([]);
    const rows=page.locator('.inbox-job-row');
    await expect(rows.first().getByRole('link')).toHaveAttribute('href','/project/flat/jobs/job-000/story');
    await expect(rows.first().locator('time')).toHaveAttribute('datetime','2026-11-20T12:00:00Z');
    await expect(rows.nth(1).locator('time')).toHaveAttribute('datetime', '2026-11-10T00:00:00Z');
    await expect(rows.locator('script')).toHaveCount(0);
    const initial=queries.length;
    await page.evaluate(() => window.scrollTo(0, 0));
    await inboxFolder(page,'Archive');
    await expect(page.getByRole('checkbox',{name:'Select job hidden',exact:true})).toBeVisible();
    await expect(rows).toHaveCount(1);
    expect(queries.slice(initial).map(q=>q.get('includeHidden'))).toEqual(['false','false','false','true','true','true','false','false','false']);
    await inboxFolder(page,'All');
    await expect(rows).toHaveCount(200);
    expect(queries.slice(initial + 9).map(q=>q.get('pageToken'))).toEqual([null]);
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  });

  test(`inbox search matches every term in a job and survives exact-job navigation at ${width}px`, async ({page, context, baseURL})=>{
    await page.setViewportSize({width,height:900});
    await context.addCookies([{name:'colony2:user:email',value:'inbox@example.com',url:baseURL!}]);
    const jobs=[{...jobIdentity,tenant_id:'search',job_id:'one',goal_id:'same',title:'Cloud release',prompt:'Needle dashboard',status:'READY',store:'ACTIVE'},
      {...jobIdentity,tenant_id:'search',job_id:'two',goal_id:'same',title:'Other attempt',prompt:'Dashboard elsewhere',status:'READY',store:'ACTIVE'}];
    let reads=0;
    await page.route('**/api/**',route=>{
      const path=new URL(route.request().url()).pathname;
      if(path.endsWith('/jobs')){reads++;return route.fulfill({json:{jobs}});}
      return route.fulfill({json:path.endsWith('/projects')?[{tenant_id:'search'}]:path.endsWith('/cells')?{cells:[]}:path.endsWith('/pending')?[]:path.endsWith('/jobs/one')?jobs[0]:{}});
    });
    await mockCells(page, ['platform']);
    await page.goto('/project/search/jobs?inbox=cell&cellId=cell-platform');
    await expect(page.locator('.inbox-job-row')).toHaveCount(2);
    const initial=reads;
    await page.getByRole('searchbox', { name: 'Search threads' }).fill('NEEDLE dashboard');
    await expect(page.locator('.inbox-job-row')).toHaveCount(1);
    const saved=page.url();
    expect(reads).toBe(initial);
    await page.getByRole('link',{name:'platform: Cloud release',exact:true}).click();
    await expect(page.getByRole('heading',{name:'Cloud release',exact:true})).toBeVisible();
    await expect(page.getByRole('tab',{name:'Thread',exact:true})).toHaveAttribute('aria-selected','true');
    await page.getByRole('link',{name:'Back to inbox'}).click();
    await expect(page).toHaveURL(saved);
    await expect(page.getByRole('searchbox', { name: 'Search threads' })).toHaveValue('NEEDLE dashboard');
    await page.reload();
    await expect(page.locator('.inbox-job-row')).toHaveCount(1);
    await page.getByRole('searchbox', { name: 'Search threads' }).fill('Needle elsewhere');
    await expect(page.getByText('No jobs match this search')).toBeVisible();
  });

  test(`archive partial failures retain only failed selection and retry without resending successes at ${width}px`, async ({page,context,baseURL})=>{
    await page.setViewportSize({width,height:900});
    await context.addCookies([{name:'colony2:user:email',value:'inbox@example.com',url:baseURL!}]);
    const jobs=['finished','failed','ready'].map(id=>({...jobIdentity,tenant_id:'archive-ui',job_id:id,prompt:id,status:id==='ready'?'READY':'COMPLETED',store:'ACTIVE',completion_status:'success'}));
    const hidden=new Set<string>(),changes:string[]=[];
    let fail=true,release!:()=>void;
    const held=new Promise<void>(resolve=>{release=resolve;});
    await page.route('**/api/**',route=>{
      const u=new URL(route.request().url());
      return route.fulfill({json:u.pathname.endsWith('/projects')?[{tenant_id:'archive-ui'}]:u.pathname.endsWith('/cells')?{cells:[]}:u.pathname.endsWith('/pending')?[]:{}});
    });
    await page.route('**/api/projects/archive-ui/jobs?*',route=>route.fulfill({json:{jobs:jobs.filter(job=>new URL(route.request().url()).searchParams.get('includeHidden')==='true'||!hidden.has(job.job_id))}}));
    await page.route('**/api/projects/archive-ui/jobs/*',async route=>{
      const id=new URL(route.request().url()).pathname.split('/').at(-1)!;
      if(route.request().method()==='GET') return route.fulfill({json:jobs.find(job=>job.job_id===id)});
      expect(route.request().method()).toBe('DELETE');changes.push(id);await held;
      if(id==='failed'&&fail)return route.fulfill({status:503,json:{error:'Try again'}});
      hidden.add(id);return route.fulfill({status:204});
    });
    await page.goto('/project/archive-ui/jobs');
    const list = page.getByRole('list', { name: 'Jobs' });
    const listTop = () => list.evaluate(element => element.getBoundingClientRect().top);
    await expect(list.getByRole('listitem')).toHaveCount(3);
    const top = await listTop();
    for(const id of ['finished','failed']) await page.getByRole('checkbox',{name:`Select job ${id}`,exact:true}).check();
    const archive=page.getByRole('button',{name:'Archive selected',exact:true});
    await archive.press('Enter');
    await expect.poll(()=>changes.length).toBe(2);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({hasText:'Archiving jobs…'})).toBeVisible();
    expect(await listTop()).toBe(top);
    await expect(archive).toBeDisabled();release();
    const dialog=page.getByRole('dialog');await expect(dialog).toContainText('Try again');
    await expect(page.getByRole('checkbox',{name:'Select job finished',exact:true})).toHaveCount(0);
    await expect(page.getByRole('checkbox',{name:'Select job failed',exact:true})).toBeChecked();
    fail=false;await dialog.getByRole('button',{name:'Archive job',exact:true}).click();
    await expect(dialog).toHaveCount(0);
    await expect(list.getByRole('listitem')).toHaveCount(1);
    expect(await listTop()).toBe(top);
    expect(changes.slice(0, 2).sort()).toEqual(['failed', 'finished']);
    expect(changes.slice(2)).toEqual(['failed']);
    await page.getByRole('checkbox',{name:'Select job ready',exact:true}).check();
    await archive.click();await expect(dialog).toBeVisible();
    await dialog.getByRole('button',{name:'Keep jobs'}).click();
    await expect(archive).toBeFocused();expect(changes).toHaveLength(3);
    await inboxFolder(page,'Archive');await expect(page.locator('.inbox-job-row')).toHaveCount(2);
    await expect(page.getByRole('button',{name:'Archive selected'})).toHaveCount(0);
  });
}

// GJC-001/006/009–014: real clipboard, native input and isolated row feedback.
for (const width of [1280, 320, 390]) {
  test.describe(`job ID copying at ${width}px`, () => {
    test.use({ viewport: { width, height: 900 }, hasTouch: width < 768, permissions: ['clipboard-read', 'clipboard-write'] });
    for (const alias of ['goals', 'jobs']) test(`copies the title job on ${alias}`, async ({ page, context, baseURL }, testInfo) => {
      const id = '3M11h0hfoxIoOk6knxCN6DW9o0u';
      const title = 'Build a reliable dashboard with a long title for a compact screen';
      const job = { ...jobIdentity, tenant_id: 'copy', job_id: id, goal_id: 'different-goal', prompt: title, status: 'READY', store: 'ACTIVE', created_at: '2026-11-12T00:00:00Z' };
      await context.addCookies([{ name: 'colony2:user:email', value: 'copy@example.com', url: baseURL! }]);
      await mockCells(page, ['platform']);
      await page.route('**/api/projects/copy/jobs?*', route => route.fulfill({ json: { jobs: [job, { ...job, job_id: 'untouched', goal_id: 'other-goal', prompt: 'Another goal' }] } }));
      await page.route('**/api/projects/copy/jobs/*', route => route.fulfill({ json: job }));
      await page.goto(`/project/copy/${alias}?executionState=cells-v1&sort=prompt&order=ascend`);
      const button = page.getByRole('button', { name: `Copy job ID ${id}`, exact: true });
      const row = button.locator('xpath=ancestor::li[contains(@class,"inbox-job-row")][1]');
      const other = page.getByRole('button', { name: 'Copy job ID untouched', exact: true }).locator('xpath=ancestor::li[contains(@class,"inbox-job-row")][1]');
      await expect(button).toBeVisible();
      await expect(button.locator('svg')).toHaveCount(1);
      expect(await button.evaluate(element => element.closest('a') === null && element.textContent === '')).toBe(true);
      await expect(row.getByRole('link', { name: `platform: ${title}`, exact: true })).toHaveAttribute('href', new RegExp(`/jobs/${id}/story`));
      await row.getByRole('checkbox', { name: `Select job ${id}`, exact: true }).check();
      const before = page.url();
      // The compatibility adapter completes all three list scans before rendering.
      await expect(page.getByRole('list', { name: 'Jobs', exact: true }).getByRole('listitem')).toHaveCount(2);
      const requests: string[] = [];
      page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) requests.push(request.url()); });
      const unchanged = async () => {
        expect(page.url()).toBe(before);
        await expect(row.getByRole('checkbox', { name: `Select job ${id}`, exact: true })).toBeChecked();
        await expect(other.getByRole('checkbox')).not.toBeChecked();
        await expect(row.getByRole('link', { name: `platform: ${title}`, exact: true })).toHaveAttribute('href', new RegExp(`/jobs/${id}/story`));
        await expect(other.getByRole('status')).toBeEmpty();
        expect(requests).toEqual([]);
      };
      await button.focus();
      await expect(button).toBeFocused();
      await expect(page.getByRole('tooltip', { name: `Copy job ID ${id}`, exact: true })).toBeVisible();
      const toast = page.locator('.ant-message-notice-content').filter({ hasText: /^Job ID copied$/ });
      const geometry = await row.boundingBox();
      for (const key of ['Enter', 'Space']) {
        await expect(toast).toHaveCount(0);
        await page.evaluate(() => navigator.clipboard.writeText('sentinel'));
        await button.focus(); await page.keyboard.press(key);
        await expect(toast).toHaveCount(1);
        await expect(toast.locator('.ant-message-success')).toHaveText('Job ID copied');
        await expect(row.getByRole('status')).toHaveText('Job ID copied');
        await expect(button).toBeFocused();
        expect(await row.boundingBox()).toEqual(geometry);
        await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(id);
        await unchanged();
      }
      await expect(toast).toHaveCount(0);
      await page.evaluate(() => navigator.clipboard.writeText('sentinel'));
      if (width < 768) await button.tap(); else await button.click();
      await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(id);
      await unchanged();
      await expect(toast).toHaveCount(1);
      const appearance = await toast.evaluate(element => {
        const css = getComputedStyle(element);
        return { background: css.backgroundColor, radius: css.borderRadius, shadow: css.boxShadow, font: css.fontSize, padding: css.padding };
      });
      const placement = (await toast.boundingBox())!;
      expect(placement.y).toBeLessThan(100);
      expect(Math.abs(placement.x + placement.width / 2 - width / 2)).toBeLessThan(2);
      expect(await row.boundingBox()).toEqual(geometry);
      await button.blur(); await page.mouse.move(0, 0);
      await expect(page.getByRole('tooltip')).toHaveCount(0);
      if (alias === 'goals') await page.screenshot({ path: testInfo.outputPath(`copy-success-${width}.png`), fullPage: true, animations: 'disabled' });
      await expect(toast).toHaveCount(0);
      await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: () => Promise.reject(new Error('Fixture permission denied')) } }));
      await button.click();
      await expect(row.getByRole('status')).toContainText('Could not copy job ID');
      await expect(toast).toHaveCount(0);
      await expect(row.getByRole('status')).not.toContainText('Job ID copied');
      const manual = row.locator('.goal-copy-manual-id');
      await expect(manual).toHaveValue(id);
      await expect(manual).toHaveCSS('user-select', 'text');
      await manual.focus();
      expect(await manual.evaluate(element => {
        const field = element as HTMLTextAreaElement;
        return field.value.slice(field.selectionStart, field.selectionEnd);
      })).toBe(id);
      await unchanged();
      if (width < 768) {
        const target = (await button.boundingBox())!;
        expect(target.width).toBeGreaterThanOrEqual(44); expect(target.height).toBeGreaterThanOrEqual(44);
        for (const neighbor of [row.getByRole('link', { name: `platform: ${title}`, exact: true }), row.getByRole('checkbox', { name: `Select job ${id}`, exact: true })]) {
          const box = (await neighbor.boundingBox())!;
          expect(target.x + target.width <= box.x || box.x + box.width <= target.x || target.y + target.height <= box.y || box.y + box.height <= target.y).toBe(true);
        }
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      }
      if (alias === 'goals') await page.screenshot({ path: testInfo.outputPath(`copy-failure-${width}.png`), fullPage: true, animations: 'disabled' });
      // Start archive's reference from a fresh fixture after the copy journey.
      await page.reload();
      await row.getByRole('checkbox', { name: `Select job ${id}`, exact: true }).check();
      // Exercise archive's real dialog and toast as the visual reference.
      await page.route(`**/api/projects/copy/jobs/${id}`, route => route.request().method() === 'DELETE'
        ? route.fulfill({ status: 204 }) : route.fulfill({ json: job }));
      await page.getByRole('button', { name: 'Archive selected', exact: true }).click();
      await page.getByRole('dialog').getByRole('button', { name: 'Archive job', exact: true }).click();
      const archiveToast = page.locator('.ant-message-notice-content').filter({ hasText: /^Archived 1 job$/ });
      await expect(archiveToast).toHaveCount(1);
      await expect(archiveToast.locator('.ant-message-success')).toHaveText('Archived 1 job');
      expect(await archiveToast.evaluate(element => {
        const css = getComputedStyle(element);
        return { background: css.backgroundColor, radius: css.borderRadius, shadow: css.boxShadow, font: css.fontSize, padding: css.padding };
      })).toEqual(appearance);
      expect((await archiveToast.boundingBox())!.y).toBeLessThan(100);
      if (alias === 'goals') await page.screenshot({ path: testInfo.outputPath(`archive-reference-${width}.png`), fullPage: true, animations: 'disabled' });

    });
  });
}
