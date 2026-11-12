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
  let respond = async (route:Route) => { await route.fulfill({ json: { events:[leaseEvent('9'),leaseEvent('2')],nextPageToken:'opaque+/=雪' } }); };
  await page.context().addCookies([{name:'colony2:user:email',value:'fixture@example.com',url:'http://127.0.0.1:15177'}]);
  await page.addInitScript(() => { (window as any).EventSource = class { readyState=1; close(){} addEventListener(){} removeEventListener(){} }; });
  page.on('request',request => { if(new URL(request.url()).pathname.endsWith('/events')) requests.push(new URL(request.url())); });
  await page.route('**/api/**',async route => {
    const url = new URL(route.request().url());
    if(url.pathname.endsWith('/events')) return respond(route);
    if(url.pathname==='/api/projects') return route.fulfill({json:[{id:'tenant',name:'Tenant',tenant_id:'tenant'}]});
    if(url.pathname.endsWith('/cells')) return route.fulfill({json:{cells:[]}});
    if(url.pathname.endsWith('/user-inputs/pending')) return route.fulfill({json:[]});
    if(url.pathname.endsWith('/children')) return route.fulfill({json:{jobs:[]}});
    if(url.pathname.endsWith('/story')) return route.fulfill({json:{...jobIdentity,job_id:'job',status:'completed',recipe:{name:'build'},root:{id:'root',kind:'recipe',title:'Lease timeline fixture',status:'completed',path:[],attempt:1,children:[]}}});
    if(/\/jobs\/[^/]+$/.test(url.pathname)) return route.fulfill({json:job});
    if(url.pathname.endsWith('/jobs')) return route.fulfill({json:{jobs:[job]}});
    return route.fulfill({json:{}});
  });
  return { requests, setResponder: (fn:typeof respond) => { respond=fn; } };
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
    await expect(pane(page).getByRole('listitem')).toHaveCount(2);
    expect(fixture.requests).toHaveLength(1);
    expect(fixture.requests[0].searchParams.has('pageToken')).toBe(false);
    await expect(pane(page).getByText('Job created',{exact:true})).toBeVisible();
    if(process.env.CORTEX_REVIEW_ARTIFACTS) await page.screenshot({path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/${which}-timeline-overview.png`,fullPage:false});
    const details=pane(page).locator('summary').first();
    await details.focus(); await page.keyboard.press('Enter');
    await expect(pane(page).getByText(preciseTime,{exact:true}).first()).toBeVisible();
    await expect(pane(page).getByText('2026-11-08T06:00:00.999999999Z',{exact:true}).first()).toBeVisible();
    expect(await page.evaluate(()=>(window as any).hacked)).toBeUndefined();
    expect(await pane(page).locator('script,img').count()).toBe(0);
    fixture.setResponder(async route=>{ await route.fulfill({json:{events:[leaseEvent('2'),leaseEvent('900719925474099312345678901234567890',{eventType:'lease_acquired',acquisitionSource:'targeted',previousLeaseId:'replaced-lease'})]}}); });
    await pane(page).getByRole('button',{name:'Load older lease events'}).click();
    await expect(pane(page).getByRole('listitem')).toHaveCount(3);
    expect(fixture.requests).toHaveLength(2);
    expect(fixture.requests[1].searchParams.get('pageToken')).toBe('opaque+/=雪');
    await expect(pane(page).locator('summary')).toHaveText(['Historical details · 9','Historical details · 2','Historical details · 900719925474099312345678901234567890']);
    await expect(pane(page).getByRole('button',{name:'Load older lease events'})).toHaveCount(0);
    await pane(page).locator('summary').last().focus(); await page.keyboard.press('Enter');
    await expect(pane(page).getByText('replaced-lease',{exact:true})).toBeVisible();
    if(process.env.CORTEX_REVIEW_ARTIFACTS) await page.screenshot({path:`${process.env.CORTEX_REVIEW_ARTIFACTS}/${which}-timeline-details.png`,fullPage:false});
    await leaveTimeline(page); await expect(pane(page).getByText('Job created',{exact:true})).toHaveCount(0);
    await timeline(page).click(); await expect.poll(()=>fixture.requests.length).toBe(3);
    expect(fixture.requests[2].searchParams.has('pageToken')).toBe(false);
  });
}
test('older failure retains rows, invalid cursor refresh discards history before requesting page one',async({page})=>{
  const fixture=await fixtures(page); await surface(page,'story'); await timeline(page).click();
  await expect(pane(page).getByRole('listitem')).toHaveCount(2);
  fixture.setResponder(async route=>{await route.fulfill({status:503,json:{error:'backend offline',code:'backend_unavailable'}});});
  await pane(page).getByRole('button',{name:'Load older lease events'}).click();
  await expect(pane(page).getByRole('alert')).toContainText('backend offline');
  await expect(pane(page).getByRole('listitem')).toHaveCount(2);
  await expect(page.getByRole('heading',{name:'Job Story: Inspect lease history'})).toBeVisible();
  fixture.setResponder(async route=>{await route.fulfill({status:400,json:{error:'cursor expired',code:'invalid_lease_event_page_token'}});});
  await pane(page).getByRole('button',{name:'Retry lease history'}).click();
  await expect(pane(page).getByRole('button',{name:'Refresh from first page'})).toBeVisible();
  expect(fixture.requests[2].searchParams.get('pageToken')).toBe('opaque+/=雪');
  let release!:()=>void; const wait=new Promise<void>(resolve=>{release=resolve;});
  fixture.setResponder(async route=>{await wait; await route.fulfill({json:{events:[]}});});
  await pane(page).getByRole('button',{name:'Refresh from first page'}).click();
  await expect(pane(page).getByRole('status')).toBeVisible(); await expect(pane(page).getByRole('listitem')).toHaveCount(0);
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
  await expect(pane(page).getByRole('listitem')).toHaveCount(2);
  let release!:()=>void; const wait=new Promise<void>(resolve=>{release=resolve;});
  fixture.setResponder(async route=>{await wait; await route.fulfill({json:{events:[leaseEvent('late')],nextPageToken:'stale'}}).catch(()=>{});});
  await pane(page).getByRole('button',{name:'Load older lease events'}).click();
  await expect(pane(page).getByRole('button',{name:'Load older lease events'})).toBeDisabled();
  expect(fixture.requests).toHaveLength(2);
  const canceled=page.waitForEvent('requestfailed',req=>req.url().includes('/events'));
  await leaveTimeline(page); await canceled;
  release();
  await expect(pane(page).getByRole('listitem')).toHaveCount(0);
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
