import { expect, test, type Page, type Route } from '@playwright/test';
import { leaseEvent, preciseTime } from '../../shared/src/test/leaseEventFixtures';
import { jobIdentity } from './helpers/cells';

const job = { ...jobIdentity, tenant_id: 'tenant', job_id: 'job', goal_id: 'goal', title: 'Inspect lease history', prompt: 'Keep ordinary job details usable', status: 'READY', store: 'ACTIVE', job_type: 'recipe', recipe:'build', created_at: '2026-11-07T00:00:00.000000001Z' };
const pane = (page:Page) => page.getByRole('region',{name:'Lease timeline',exact:true});
const timeline = (page:Page) => page.getByRole('tab',{name:'Lease timeline',exact:true}).or(pane(page).getByRole('button',{name:'Timeline',exact:true}));
const leaveTimeline = async (page:Page) => { const tab=page.getByRole('tab',{name:'Conversation',exact:true}); if(await tab.count()) await tab.click(); else await timeline(page).click(); };
async function fixtures(page: Page) {
  if (process.env.CORTEX_DEBUG_BROWSER) {
    page.on('console', message => console.log('CONSOLE',message.type(),message.text()));
    page.on('pageerror', error => console.log('PAGEERROR',error.message));
    page.on('request', request => console.log('REQ',request.url()));
    page.on('response', response => console.log('RES',response.status(),response.url()));
    page.on('crash', () => console.log('CRASH'));
  }
  const requests: URL[] = [];
  const mutations: URL[] = [];
  let currentJob = {...job};
  let clearRespond = async (route:Route) => { currentJob = {...currentJob,status: 'WAITING'}; await route.fulfill({status:204}); };
  const defaultDetail = async (route:Route) => route.fulfill({json:currentJob});
  let detailRespond = defaultDetail;
  let respond = async (route:Route) => { await route.fulfill({ json: { events:[leaseEvent('9'),leaseEvent('2')],nextPageToken:'opaque+/=雪' } }); };
  await page.context().addCookies([{name:'colony2:user:email',value:'fixture@example.com',url:'http://127.0.0.1:15177'}]);
  await page.addInitScript(() => { (window as any).EventSource = class { readyState=1; close(){} addEventListener(){} removeEventListener(){} }; });
  page.on('request',request => { const url=new URL(request.url()); if(url.pathname.endsWith('/events')) requests.push(url); if(url.pathname.endsWith('/clear-crash-concern')) mutations.push(url); });
  await page.route('**/api/**',async route => {
    const url = new URL(route.request().url());
    if(url.pathname.endsWith('/clear-crash-concern')) return clearRespond(route);
    if(url.pathname.endsWith('/events')) return respond(route);
    if(url.pathname==='/api/projects') return route.fulfill({json:[{id:'tenant',name:'Tenant',tenant_id:'tenant'}]});
    if(url.pathname.endsWith('/cells')) return route.fulfill({json:{cells:[]}});
    if(url.pathname.endsWith('/user-inputs/pending')) return route.fulfill({json:[]});
    if(url.pathname.endsWith('/children')) return route.fulfill({json:{jobs:[]}});
    if(url.pathname.endsWith('/story')) return route.fulfill({json:{...jobIdentity,job_id:'job',status:'completed',recipe:{name:'build'},root:{id:'root',kind:'recipe',title:'Lease timeline fixture',status:'completed',path:[],attempt:1,children:[]}}});
    if(/\/jobs\/[^/]+$/.test(url.pathname)) return detailRespond(route);
    if(url.pathname.endsWith('/jobs')) return route.fulfill({json:{jobs:[job]}});
    return route.fulfill({json:{}});
  });
  return { requests, mutations, setJob: (status:string) => { currentJob={...currentJob,status}; }, setClear: (fn:typeof clearRespond) => {clearRespond=fn;}, setDetail: (fn:typeof detailRespond) => {detailRespond=fn;}, resetDetail: () => {detailRespond=defaultDetail;}, setResponder: (fn:typeof respond) => { respond=fn; } };
}
async function surface(page:Page, which:'story'|'detail') {
  await page.goto(which==='story' ? '/project/tenant/jobs/job/story' : '/project/tenant/jobs');
  if(which==='detail') {
    const expand = page.getByRole('button',{name:'Expand goal details'});
    await expand.first().click();
  }
  await expect(timeline(page)).toBeVisible();
}
for(const which of ['story','detail'] as const) {
  test(`${which}: zero ordinary reads, one keyboard opening, explicit paging, tied renewal details and fresh reopening`,async({page})=>{
    const fixture=await fixtures(page); await surface(page,which);
    await expect(timeline(page)).toHaveAttribute(which==='story'?'aria-selected':'aria-expanded','false');
    expect(fixture.requests).toHaveLength(0);
    await timeline(page).focus(); await page.keyboard.press('Enter');
    await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
    expect(fixture.requests).toHaveLength(1);
    expect(fixture.requests[0].searchParams.has('pageToken')).toBe(false);
    await expect(pane(page).getByText('Job created',{exact:true})).toBeVisible();
    if(process.env.CORTEX_REVIEW_ARTIFACTS) await page.screenshot({animations:'disabled',path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/${which}-timeline-overview.png`,fullPage:false});
    const details=pane(page).getByRole('button',{name:'Expand row'});
    await details.focus(); await page.keyboard.press('Enter');
    await pane(page).getByRole('button',{name:'Lease renewed · 9'}).click();
    await expect(pane(page).getByText(preciseTime,{exact:true}).first()).toBeVisible();
    await expect(pane(page).getByText('2026-11-08T06:00:00.999999999Z',{exact:true}).first()).toBeVisible();
    expect(await page.evaluate(()=>(window as any).hacked)).toBeUndefined();
    expect(await pane(page).locator('script,img').count()).toBe(0);
    fixture.setResponder(async route=>{ await route.fulfill({json:{events:[leaseEvent('2'),leaseEvent('900719925474099312345678901234567890',{eventType:'lease_acquired',acquisitionSource:'targeted',previousLeaseId:'replaced-lease'})]}}); });
    await pane(page).getByRole('button',{name:'Load older lease events'}).click();
    await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
    expect(fixture.requests).toHaveLength(2);
    expect(fixture.requests[1].searchParams.get('pageToken')).toBe('opaque+/=雪');
    await expect(pane(page).getByRole('button',{name:'Lease acquired · 900719925474099312345678901234567890'})).toBeVisible();
    await expect(pane(page).getByRole('button',{name:'Load older lease events'})).toHaveCount(0);
    await pane(page).getByRole('button',{name:'Lease acquired · 900719925474099312345678901234567890'}).focus(); await page.keyboard.press('Enter');
    await expect(pane(page).getByText('replaced-lease',{exact:true})).toBeVisible();
    if(process.env.CORTEX_REVIEW_ARTIFACTS) await page.screenshot({animations:'disabled',path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/${which}-timeline-details.png`,fullPage:false});
    await leaveTimeline(page); await expect(pane(page).getByText('Job created',{exact:true})).toHaveCount(0);
    await timeline(page).click(); await expect.poll(()=>fixture.requests.length).toBe(3);
    expect(fixture.requests[2].searchParams.has('pageToken')).toBe(false);
  });
}
test('older failure retains rows, invalid cursor refresh discards history before requesting page one',async({page})=>{
  const fixture=await fixtures(page); await surface(page,'story'); await timeline(page).click();
  await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
  fixture.setResponder(async route=>{await route.fulfill({status:503,json:{error:'backend offline',code:'backend_unavailable'}});});
  await pane(page).getByRole('button',{name:'Load older lease events'}).click();
  await expect(pane(page).getByRole('alert')).toContainText('backend offline');
  await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
  await expect(page.getByRole('heading',{name:'Job Story: Inspect lease history'})).toBeVisible();
  fixture.setResponder(async route=>{await route.fulfill({status:400,json:{error:'cursor expired',code:'invalid_lease_event_page_token'}});});
  await pane(page).getByRole('button',{name:'Retry lease history'}).click();
  await expect(pane(page).getByRole('button',{name:'Refresh from first page'})).toBeVisible();
  expect(fixture.requests[2].searchParams.get('pageToken')).toBe('opaque+/=雪');
  let release!:()=>void; const wait=new Promise<void>(resolve=>{release=resolve;});
  fixture.setResponder(async route=>{await wait; await route.fulfill({json:{events:[]}});});
  await pane(page).getByRole('button',{name:'Refresh from first page'}).click();
  await expect(pane(page).getByRole('status')).toBeVisible(); await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(0);
  expect(fixture.requests[3].searchParams.has('pageToken')).toBe(false);
  release(); await expect(pane(page).getByText('No retained lease events are available.')).toBeVisible();
  await expect(pane(page).getByText(/Collection and retention/)).toBeVisible();
});
for(const [status,code] of [[403,'forbidden'],[401,'authentication_required'],[501,'unsupported_lease_events'],[500,'backend_unavailable']] as const) {
  test(`HTTP ${status} ${code} is a local error, ordinary details work and Retry reads one page`,async({page})=>{
    const fixture=await fixtures(page);
    fixture.setResponder(async route=>{await route.fulfill({status,json:{error:code,code}});});
    await surface(page,'detail'); expect(fixture.requests).toHaveLength(0); await timeline(page).click();
    await expect(pane(page).getByRole('alert')).toContainText(code);
    await expect(pane(page).getByText('No retained lease events are available.')).toHaveCount(0);
    await expect(page.getByText('Keep ordinary job details usable',{exact:true}).first()).toBeVisible();
    fixture.setResponder(async route=>{await route.fulfill({json:{events:[]}});});
    await pane(page).getByRole('button',{name:'Retry lease history'}).click();
    await expect(pane(page).getByText('No retained lease events are available.')).toBeVisible();
    expect(fixture.requests).toHaveLength(2);
  });
}
test('overlapping older actions are disabled, closing cancels and ignores late history',async({page})=>{
  const fixture=await fixtures(page); await surface(page,'story'); await timeline(page).click();
  await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
  let release!:()=>void; const wait=new Promise<void>(resolve=>{release=resolve;});
  fixture.setResponder(async route=>{await wait; await route.fulfill({json:{events:[leaseEvent('late')],nextPageToken:'stale'}}).catch(()=>{});});
  await pane(page).getByRole('button',{name:'Load older lease events'}).click();
  await expect(pane(page).getByRole('button',{name:'Load older lease events'})).toBeDisabled();
  expect(fixture.requests).toHaveLength(2);
  const canceled=page.waitForEvent('requestfailed',req=>req.url().includes('/events'));
  await leaveTimeline(page); await canceled;
  release();
  await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(0);
  fixture.setResponder(async route=>{await route.fulfill({json:{events:[]}});});
  await timeline(page).click(); await expect(pane(page).getByText('No retained lease events are available.')).toBeVisible();
  expect(fixture.requests).toHaveLength(3); expect(fixture.requests[2].searchParams.has('pageToken')).toBe(false);
});
test('UI identity signal clears the pane and cancels work without establishing authentication',async({page})=>{
  const fixture=await fixtures(page); await surface(page,'story');
  let release!:()=>void; const wait=new Promise<void>(resolve=>{release=resolve;});
  fixture.setResponder(async route=>{await wait; await route.fulfill({json:{events:[leaseEvent('late')]}}).catch(()=>{});});
  await timeline(page).click(); await expect(pane(page).getByRole('status')).toBeVisible();
  const canceled=page.waitForEvent('requestfailed',req=>req.url().includes('/events'));
  fixture.setResponder(async route=>{await route.fulfill({json:{events:[]}});});
  await page.evaluate(()=>{document.cookie='colony2:user:email=changed@example.com; path=/';window.dispatchEvent(new Event('cortex-ui-identity'));});
  await canceled; await expect(timeline(page)).toHaveAttribute('aria-selected','true'); release();
  fixture.setResponder(async route=>{await route.fulfill({json:{events:[]}});});
  await expect(pane(page).getByText('No retained lease events are available.')).toBeVisible();
  expect(fixture.requests).toHaveLength(2);
});
test('navigation cancels the pending read and the destination starts closed',async({page})=>{
  const fixture=await fixtures(page); await surface(page,'story');
  let release!:()=>void; const wait=new Promise<void>(resolve=>{release=resolve;});
  fixture.setResponder(async route=>{await wait; await route.fulfill({json:{events:[leaseEvent('late')]}}).catch(()=>{});});
  await timeline(page).click(); await expect(pane(page).getByRole('status')).toBeVisible();
  const canceled=page.waitForEvent('requestfailed',req=>req.url().includes('/events'));
  await page.getByRole('link',{name:'Goal overview',exact:true}).click(); await canceled; release();
  await page.goto('/project/tenant/jobs/job/story'); await expect(timeline(page)).toHaveAttribute('aria-selected','false');
  expect(fixture.requests).toHaveLength(1);
});

for (const width of [1280,390]) {
 test(`crash concern ${width}px: confirmation, focus, pending guard, toast and exact refresh`,async({page})=>{
  await page.setViewportSize({width,height:900});const f=await fixtures(page);f.setJob('CRASH_CONCERN');f.setResponder(async route=>route.fulfill({json:{events:[leaseEvent('9',{workerId:'worker-1'}),leaseEvent('2',{workerId:'worker-2'})]}}));await surface(page,'story');await timeline(page).click();
  const panel=page.getByRole('tabpanel',{name:'Lease timeline',exact:true});
  const trigger=panel.getByRole('button',{name:'Clear crash concern',exact:true});await expect(trigger).toBeVisible();
  expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const warning=panel.getByText('Crash concern',{exact:true});await expect(warning).toBeVisible();
  expect((await warning.boundingBox())!.y).toBeLessThan((await pane(page).boundingBox())!.y);
  if(process.env.CORTEX_REVIEW_ARTIFACTS) await page.screenshot({animations:'disabled',path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/crash-concern-${width}.png`});
  await trigger.focus();await page.keyboard.press('Enter');const dialog=page.getByRole('dialog');await expect(dialog).toContainText('Tenant: tenant');await expect(dialog).toContainText('Job: job');
  await expect(dialog.locator('.ant-modal-footer button')).toHaveCount(1);
  await expect(page.locator('.crash-concern-dialog')).not.toHaveClass(/ant-zoom/);
  await expect(dialog.getByRole('button',{name:'Clear crash concern',exact:true})).toHaveCSS('min-height','44px');
  expect((await dialog.getByRole('button',{name:'Clear crash concern',exact:true}).boundingBox())!.height).toBeGreaterThanOrEqual(44);
  if(process.env.CORTEX_REVIEW_ARTIFACTS && width===1280) await page.screenshot({animations:'disabled',path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/clear-confirmation.png`});
  await expect.poll(()=>dialog.evaluate(el=>el.contains(document.activeElement))).toBe(true);
  await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(trigger).toBeFocused();
  // Hold the next automatic metadata read while focus is back on the trigger.
  // Refreshing must disable this same button rather than remove and recreate it.
  let finishRefresh!:()=>void;const refreshWait=new Promise<void>(resolve=>{finishRefresh=resolve;});
  f.setDetail(async route=>{await refreshWait;await route.fulfill({json:{...job,status:'CRASH_CONCERN'}});});
  await page.waitForRequest(request=>new URL(request.url()).pathname==='/api/projects/tenant/jobs/job');
  await expect(trigger).toBeDisabled();await expect(trigger).toBeFocused();
  if(process.env.CORTEX_REVIEW_ARTIFACTS && width===390) await page.screenshot({animations:'disabled',path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/crash-refresh-phone.png`});
  finishRefresh();await expect(trigger).toBeEnabled();await expect(trigger).toBeFocused();
  f.resetDetail();
  await trigger.click();await dialog.getByRole('button',{name:'Close',exact:true}).click();await expect(dialog).toHaveCount(0);await expect(trigger).toBeFocused();
  await trigger.click();await page.locator('.ant-modal-wrap').click({position:{x:10,y:10}});await expect(dialog).toHaveCount(0);await expect(trigger).toBeFocused();expect(f.mutations).toHaveLength(0);
  let release!:()=>void;const wait=new Promise<void>(resolve=>{release=resolve;});let calls=0;
  f.setClear(async route=>{calls++;expect(route.request().method()).toBe('POST');expect(route.request().postData()).toBe('{}');expect(new URL(route.request().url()).pathname).toBe('/api/projects/tenant/jobs/job/clear-crash-concern');await wait;f.setJob('WAITING');await route.fulfill({status:204});});
  await trigger.click();await dialog.getByRole('button',{name:'Clear crash concern',exact:true}).click();
  await expect(dialog.getByRole('button',{name:'Clear crash concern',exact:true})).toBeDisabled();
  await page.keyboard.press('Escape');await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'Close',exact:true})).toHaveCount(0);
  release();await expect(page.getByText('Crash concern cleared',{exact:true})).toBeVisible();
  await expect(dialog).toHaveCount(0);await expect(trigger).toHaveCount(0);await expect(page.locator('.job-crash-concern')).toBeFocused();
  expect(calls).toBe(1);expect(f.mutations).toHaveLength(1);expect(f.requests).toHaveLength(1);
  if(process.env.CORTEX_REVIEW_ARTIFACTS && width===390) await page.screenshot({animations:'disabled',path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/clear-success-phone.png`});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
 });
}
test('crash concern remains independent of history failure and reports status failure after success',async({page})=>{
 const f=await fixtures(page);f.setJob('CRASH_CONCERN');f.setResponder(async route=>route.fulfill({status:503,json:{error:'history offline'}}));
 await surface(page,'story');await timeline(page).click();const panel=page.getByRole('tabpanel',{name:'Lease timeline',exact:true});
 await expect(pane(page).getByRole('alert')).toContainText('history offline');await panel.getByRole('button',{name:'Clear crash concern',exact:true}).click();
 f.setClear(async route=>{f.setDetail(async r=>r.fulfill({status:503,json:{error:'status offline'}}));await route.fulfill({status:204});});
 await page.getByRole('dialog').getByRole('button',{name:'Clear crash concern',exact:true}).click();await expect(page.getByText('Crash concern cleared',{exact:true})).toBeVisible();
 await expect(panel.getByText('Current job status unavailable',{exact:true})).toBeVisible();await expect(panel.getByRole('button',{name:'Clear crash concern',exact:true})).toHaveCount(0);
 f.setDetail(async route=>route.fulfill({json:{...job,status:'CRASH_CONCERN'}}));await panel.getByRole('button',{name:'Refresh status'}).click();await expect(panel.getByRole('button',{name:'Clear crash concern',exact:true})).toBeVisible();expect(f.requests).toHaveLength(1);
});

test('crash concern conflict refreshes status without losing readable history or retrying mutation',async({page})=>{
 const f=await fixtures(page);f.setJob('CRASH_CONCERN');f.setClear(async route=>route.fulfill({status:409,json:{error:'job has a live lease'}}));await surface(page,'story');await timeline(page).click();
 await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
 await page.getByRole('tabpanel',{name:'Lease timeline',exact:true}).getByRole('button',{name:'Clear crash concern',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Clear crash concern',exact:true}).click();
 await expect(page.getByText('Could not confirm clearing',{exact:true})).toBeVisible();await expect(page.getByText(/job has a live lease/)).toBeVisible();await expect(page.getByRole('dialog')).toHaveCount(0);await expect(pane(page).locator('tr.ant-table-row')).toHaveCount(1);
 expect(f.mutations).toHaveLength(1);expect(f.requests).toHaveLength(1);await expect(page.getByText('Crash concern cleared',{exact:true})).toHaveCount(0);
});

test('follows a lease to its Pulse provider, log and job-filtered launches', async ({ page }) => {
  const fixture = await fixtures(page);
  fixture.setDetail(route => route.fulfill({json: {...job, launch_id: 'launch-1'}}));
  fixture.setResponder(route => route.fulfill({json: {events: [leaseEvent('7', {workerId: 'launch-1'})]}}));
  const live = {
    id: 'machine-1', launch_id: 'launch-1', provider: 'mac', state: 'running', refs: [],
    job: { jobdb: 'http://db/tenant', instance_id: 'target', job_id: 'job', tenant_id: 'tenant' },
  };
  await page.route('**/api/pulse/**', async route => {
    const path = new URL(route.request().url()).pathname;
    const provider = {
      name: 'mac', type: 'reverse', discovery: 'known', bases_complete: true,
      bases: [{name: 'macos', capabilities: ['metal']}],
      health: {name: 'mac', implementation: 'pulse-mac', version: '1', protocol: 1, state: 'healthy', restart_count: 0, generation: 3},
    };
    if (path.endsWith('/config')) return route.fulfill({json: {providers: {mac: {type: 'reverse'}}}});
    if (path.endsWith('/providers')) return route.fulfill({json: {items: [provider]}});
    if (path.endsWith('/mac/capabilities')) return route.fulfill({json: provider});
    if (path.endsWith('/status')) return route.fulfill({json: {status: 'ok', version: '1', providers: 1, poll: {}}});
    if (path.endsWith('/logs')) return route.fulfill({json: {records: [{data: 'cmVhZHkK'}], has_more: false, finalized: true, incomplete: false}});
    return route.fulfill({json: {items: [live], errors: [], complete: true}});
  });
  await page.route('**/api/projects/tenant/runner-launches?**', route =>
    route.fulfill({json: {events: [leaseEvent('2', {workerId: 'old-launch', eventType: 'lease_acquired'})]}}));
  await page.goto('/project/tenant/jobs/job/story?tab=leases');
  await expect(page.getByText('Last recorded launch ID')).toBeVisible();
  await pane(page).getByRole('link', {name: 'launch-1'}).click();
  await expect(page.getByRole('heading', {name: 'Launch launch-1'})).toBeVisible();
  await expect(page.locator('pre.runner-log')).toHaveText('ready');
  const download = page.getByRole('link', {name: 'Download log'});
  await expect(download).toHaveAttribute('download', 'launch-1.log');
  await expect(download).toHaveAttribute('href', '/api/pulse/providers/mac/launches/launch-1/download');
  await page.getByRole('link', {name: 'mac', exact: true}).click();
  await expect(page.getByRole('heading', {name: 'Active instances'})).toBeVisible();
  await expect(page.getByText('pulse-mac')).toBeVisible();
  await expect(page.getByText('metal')).toBeVisible();
  await expect(page.getByRole('table',{name:'Runner instances'}).getByRole('link',{name:'launch-1',exact:true})).toBeVisible();
  await page.getByRole('link', {name: 'Runners overview'}).click();
  await expect(page.getByRole('article')).toContainText('metal');
  await page.getByRole('navigation', {name: 'Runner navigation'}).getByRole('link', {name: 'Instances', exact: true}).click();
  await expect(page.getByRole('columnheader',{name:/Status/}).locator('.ant-table-filter-trigger')).toHaveClass(/active/);
  await expect(page.getByRole('link',{name:'job',exact:true})).toHaveAttribute('href','/project/tenant/jobs/job/story');
  await page.getByRole('link',{name:'View log for launch-1'}).click();
  await expect(page).toHaveURL(/launches\/launch-1\/logs\?provider=mac$/);
  await expect(page.locator('pre.runner-log')).toHaveText('ready');
  await page.goto('/project/tenant/jobs/job/story?tab=leases');
  await pane(page).getByRole('link', {name: 'All launches for this job'}).click();
  await expect(page.getByRole('columnheader',{name:/Job ID/}).locator('.ant-table-filter-trigger')).toHaveClass(/active/);
  await expect(page.getByRole('table')).toContainText('old-launch');
});

test('instance column controls sort, filter and restore the list from the URL', async ({page}) => {
  await fixtures(page);
  await page.setViewportSize({width:2160,height:1000});
  const active = [
    {id:'i-2',launch_id:'launch-b-0123456789',provider:'mac',state:'running',created_at:'2026-01-02T00:06:00Z',job:{job_id:'job-b',tenant_id:'other'}},
    {id:'i-1',launch_id:'launch-a-0123456789',provider:'linux',state:'queued',created_at:'2026-01-01T23:55:59Z',job:{job_id:'job-a',tenant_id:'tenant'}},
  ];
  await page.route('**/api/pulse/instances',route=>route.fulfill({json:{items:active,complete:true,errors:[]}}));
  await page.goto('/project/tenant/runners/instances');
  const table = page.getByRole('table',{name:'Runner instances'});
  const rows = table.locator('tr.ant-table-row');
  await expect(rows).toHaveCount(2);
  const mainWidth = await page.locator('.app-content').evaluate(el=>el.clientWidth);
  const pagePadding = await page.locator('.runners-page').evaluate(el=>parseFloat(getComputedStyle(el).paddingInlineStart));
  expect((await table.boundingBox())!.width).toBeGreaterThanOrEqual(mainWidth - 2 * pagePadding - 1);
  await expect(table.getByRole('columnheader',{name:'Instance',exact:true})).toHaveCount(0);
  await expect(table.getByRole('columnheader',{name:'Run duration',exact:true})).toHaveCount(0);
  const launch = table.getByRole('link',{name:'launch-a-0123456789',exact:true});
  await expect(launch).toHaveText('launch-a-0…');
  await expect(launch).toHaveAttribute('href', /launches\/launch-a-0123456789\?provider=linux$/);
  await expect(table.getByRole('link',{name:'job-b',exact:true})).toHaveAttribute('href','/project/other/jobs/job-b/story');
  await table.getByRole('columnheader',{name:/Launch ID/}).getByText('Launch ID',{exact:true}).click();
  await expect(rows.first()).toContainText('launch-a');
  await table.getByRole('columnheader',{name:/Provider/}).getByRole('button').click();
  await page.getByRole('textbox',{name:'Provider'}).fill('mac');
  await page.getByRole('button',{name:'Apply',exact:true}).click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('launch-b');
  await page.reload();
  await expect(rows).toHaveCount(1);
  await page.getByRole('button',{name:'Reset filters'}).click();
  await expect(rows).toHaveCount(2);
  await table.getByRole('columnheader',{name:'Start',exact:true}).getByRole('button').click();
  const from = page.getByPlaceholder('From (UTC)');
  const through = page.getByPlaceholder('Through (UTC)');
  await from.fill('2026-01-01 23:55');
  await from.press('Enter');
  await through.fill('2026-01-02 00:05');
  await through.press('Enter');
  await page.getByRole('button',{name:'Apply',exact:true}).click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('launch-a');
  await page.reload();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('launch-a');
  expect(new URL(page.url()).searchParams.get('date')).toBe('2026-01-01T23:55/2026-01-02T00:05');
  await page.goto('/project/tenant/runners/instances?date=2026-01-01T23:55/2026-01-02T00:06');
  await expect(rows).toHaveCount(2);
  await page.goto('/project/tenant/runners/instances?date=2026-01-02');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('launch-b');
  await page.setViewportSize({width:390,height:844});
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('lease count filters and acquisition-time sorting operate on grouped leases',async({page})=>{
  const fixture=await fixtures(page);
  const acquired = (id:string,at:string) => leaseEvent(id,{leaseId:id,workerId:id,eventType:'lease_acquired',occurredAt:at});
  fixture.setResponder(route=>route.fulfill({json:{events:[
    leaseEvent('r2',{leaseId:'beta',workerId:'beta'}),
    leaseEvent('r1',{leaseId:'beta',workerId:'beta'}),
    acquired('beta','2026-11-08T00:00:00Z'),acquired('alpha','2026-11-07T00:00:00Z'),
  ]}}));
  await page.goto('/project/tenant/jobs/job/story?tab=leases');
  const table=pane(page).getByRole('table',{name:'Recorded leases'});
  const rows=table.locator('tr.ant-table-row');
  await expect(rows).toHaveCount(2);
  await table.getByRole('columnheader',{name:'Time acquired'}).getByText('Time acquired',{exact:true}).click();
  await expect(rows.first()).toContainText('alpha');
  await table.getByRole('columnheader',{name:'Renewals'}).getByRole('button').click();
  await page.getByRole('spinbutton',{name:'Minimum renewals'}).fill('2');
  await page.getByRole('button',{name:'Apply',exact:true}).click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first().getByRole('link',{name:'beta'})).toHaveAttribute('href',/launches\/beta\?jobId=job$/);
});
