import { test, expect, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import path from 'node:path';
import { jobIdentity } from './helpers/cells';

const source={...jobIdentity,tenant_id:'recovery',job_id:'source',goal_id:'chosen-primary',other_goal_ids:['former-A','former-B'],title:'Recover release validation',prompt:'Resume validation with the retained context patch.',job_type:'recipe',recipe:'build',status:'ACTIVE',store:'ACTIVE',completion_status:'failed_app',created_at:'2026-11-12T00:00:00Z',parent:{tenant_id:'recorded-tenant',job_id:'recorded-parent',goal_id:'former-A'}};
const story={job_id:'source',status:'failed',restart_scopes:[{scope_id:'retained-boundary',chapter_ordinal:12,title:'Validation boundary'}],root:{id:'root',kind:'recipe',title:'Validation',status:'failed',path:[],attempt:1,restart_scope_id:'retained-boundary',restart_chapter_ordinal:12,children:[]}};
async function fixture(page:Page){
 let detail={...source};let detailError=0;let cancellationError=0;let restartFailure='conflict';const posts:string[]=[];const cancellations:string[]=[];const reads:string[]=[];
 await page.context().addCookies([{name:'colony2:user:email',value:'fixture@example.com',url:'http://127.0.0.1:15175'}]);
 await page.addInitScript(()=>{(window as any).EventSource=class{readyState=1;close(){}addEventListener(){}removeEventListener(){}};});
 await page.route('**/api/**',async route=>{
  const req=route.request(),url=new URL(req.url()),p=url.pathname;
  if(p==='/api/projects')return route.fulfill({json:[{id:'recovery',name:'Recovery fixtures',tenant_id:'recovery'}]});
  if(p.endsWith('/user-inputs/pending'))return route.fulfill({json:[]});
  if(p.endsWith('/cells'))return route.fulfill({json:{cells:[]}});
  if(p.endsWith('/cancel')){cancellations.push(p);if(cancellationError)return route.fulfill({status:cancellationError,json:{error:'Controlled cancellation denial'}});detail={...detail,cancel_requested:true} as typeof detail;return route.fulfill({status:204});}
  if(p.endsWith('/restart')){posts.push(req.postData()!);if(restartFailure==='lost')return route.abort('connectionfailed');if(restartFailure==='conflict')return route.fulfill({status:409,json:{error:'Controlled admission conflict; retained diagnostic'}});return route.fulfill({status:201,json:{job_id:'accepted-successor',tenant_id:'recovery',goal_id:'chosen-primary'}});}
  if(p.endsWith('/restart-context'))return route.fulfill({json:{project_id:'recovery',job_id:'source',scope_id:'retained-boundary',chapter_ordinal:12,job:{git:{author:'original'}},visible:{},redacted_paths:[]}});
  if(p.endsWith('/restart-points'))return route.fulfill({json:story.restart_scopes});
  if(p.endsWith('/story'))return route.fulfill({json:{...story,job_id:p.includes('accepted-successor')?'accepted-successor':'source'}});
  if(p.endsWith('/jobs/source')){reads.push(p);return detailError?route.fulfill({status:detailError,json:{error:detailError===403?'Controlled detail denial':'Controlled detail unavailable'}}):route.fulfill({json:detail});}
  if(p.endsWith('/jobs/accepted-successor'))return route.fulfill({json:{...detail,job_id:'accepted-successor'}});
  if(p.endsWith('/jobs')){
   const goal=url.searchParams.get('goal');
   const jobs=goal==='chosen-primary'?[detail,{...detail,job_id:'historical-success-1',status:'COMPLETED',store:'ARCHIVED',completion_status:'success'},{...detail,job_id:'historical-success-2',status:'COMPLETED',store:'ARCHIVED',completion_status:'success'}]:[];
   return route.fulfill({json:{jobs}});
  }
  if(p.endsWith('/children'))return route.fulfill({json:{jobs:[]}});
  if(p.endsWith('/conversation'))return route.fulfill({json:{tenant_id:'recovery',job_id:p.includes('accepted-successor')?'accepted-successor':'source',snapshot_token:'recovery-snapshot',snapshot_revision:1,completeness:'complete',activity:{availability:'available',value:detail},entries:[]}});
  if(p.endsWith('/input-artifacts'))return route.fulfill({json:{artifacts:[]}});
  return route.fulfill({json:{}});
 });
 return {posts,cancellations,reads,terminal:()=>{detail={...detail,status:'COMPLETED',store:'ACTIVE'};},detailError:(value:number)=>{detailError=value;},cancelError:(value:number)=>{cancellationError=value;},restart:(value:string)=>{restartFailure=value;}};
}
async function capture(page:Page,name:string,preserveViewport=false){
 const directory=process.env.CORTEX_REVIEW_ARTIFACTS;
 if(!directory)return;
 mkdirSync(directory,{recursive:true});
 if(!preserveViewport && !name.endsWith('phone')) { await page.setViewportSize({width:1440,height:2200});await page.locator('.ant-modal-body').evaluateAll(elements=>elements.forEach(element=>{element.scrollTop=0;})); }
 // A visible dialog may still have zero opacity during its opening animation.
 // Capture the settled application state, including nested cancellation dialogs.
 await page.evaluate(async()=>{
  await document.fonts.ready;
  await new Promise(requestAnimationFrame);
  await Promise.all(document.getAnimations().filter(animation=>animation.effect?.getComputedTiming().iterations!==Infinity).map(animation=>animation.finished.catch(()=>{})));
 });
 await page.screenshot({path:path.join(directory,name+'.png'),fullPage:true,animations:'disabled'});
}
async function showActions(page:Page, sourceAvailable=true){
 await page.locator('.inbox-story-more > summary').click();
 const refresh = page.getByRole('switch',{name:'Auto-refresh'});
 if(sourceAvailable && await refresh.isVisible()) await refresh.uncheck();
}
async function open(page:Page, sourceAvailable=true){
 await showActions(page, sourceAvailable);
 await page.getByRole('button',{name:'Restart',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'Restart Job'});
 await dialog.getByRole('combobox',{name:'Restart point'}).click();
 await page.getByText('Validation boundary · Chapter 12 · -',{exact:true}).click();
 return dialog;
}
async function patch(dialog:ReturnType<Page['getByRole']>){
 await dialog.getByRole('switch').click();
 await expect(dialog.getByRole('region',{name:'Context before patching'})).toBeVisible();
 await dialog.getByRole('button',{name:'Add line',exact:true}).first().click();
 await dialog.getByPlaceholder('e.g. git.author').fill('git.author');
 await dialog.getByPlaceholder('e.g. "james@example.com"').fill('retained browser draft');
}

test('R17 R18 restart of a running job reaches the service, retains conflict draft and replays unchanged after reload',async({page})=>{
 const f=await fixture(page);await page.setViewportSize({width:1280,height:1100});
 await page.goto('/project/recovery/jobs/source/story');
 const dialog=await open(page);await patch(dialog);
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
 await expect(dialog.getByText('Restart as a new goal')).toBeVisible();
 await capture(page,'01-running-restart-enabled');
 const reads=f.reads.length;
 await dialog.getByRole('button',{name:'Restart',exact:true}).click();
 await expect(dialog.getByText('Restart failed',{exact:true})).toBeVisible();
 expect(f.reads.length).toBe(reads);expect(f.posts).toHaveLength(1);
 expect(JSON.parse(f.posts[0]).goal.mode).toBe('new');
 expect(f.cancellations).toEqual([]);
 await expect(dialog.getByText(/Controlled admission conflict/)).toBeVisible();
 await expect(dialog.getByText('Validation boundary · Chapter 12 · -',{exact:true})).toBeVisible();
 await expect(dialog.getByText('Restart as a new goal',{exact:true})).toBeVisible();
 await expect(dialog.getByPlaceholder('e.g. "james@example.com"')).toHaveValue('retained browser draft');
 await capture(page,'02-running-conflict-retained-draft');
 // Explicit cancellation stays independent, and a pending cancellation adds no veto.
 await dialog.getByRole('button',{name:'Cancel source',exact:true}).click();
 const confirmation=page.getByRole('dialog',{name:'Cancel job?'});
 // Ant Design can expose a stable button during the opening prepare frame,
 // before the zoom animation starts. Wait for the dialog before submitting.
 await expect(confirmation).not.toHaveClass(/ant-zoom/);
 await confirmation.getByRole('button',{name:'Cancel job',exact:true}).click();
 await expect.poll(()=>f.cancellations).toEqual(['/api/projects/recovery/jobs/source/cancel']);
 await expect(confirmation).toBeHidden();
 await expect(dialog.getByText('Cancellation requested for this source')).toBeVisible();
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
 await expect(dialog.getByText(/Cancellation remains pending/)).toBeVisible();
 const saved=f.posts[0];f.detailError(503);
 await page.reload();const reloaded=await open(page,false);
 await expect(reloaded.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
 await expect(reloaded.getByRole('button',{name:'Retry saved request'})).toBeEnabled();
 await patch(reloaded);
 await reloaded.getByPlaceholder('e.g. "james@example.com"').fill('different editor draft');
 const replayReads=f.reads.length;f.restart('success');
 await reloaded.getByRole('button',{name:'Retry saved request'}).click();
 await expect(page).toHaveURL(/jobs\/accepted-successor\/story/);
 expect(f.posts).toHaveLength(2);expect(f.posts[1]).toBe(saved);
 expect(f.reads.length).toBe(replayReads);
 expect(f.cancellations).toEqual(['/api/projects/recovery/jobs/source/cancel']);
 expect(await page.evaluate(()=>sessionStorage.getItem('json-patch-v2:cortex:restart:recovery:source'))).toBeNull();
});

for (const entry of ['header','chapter'] as const) {
test(`R18 accepted fresh restart of a running job from ${entry} navigates without cancellation`,async({page})=>{
 const f=await fixture(page);f.restart('success');
 await page.goto('/project/recovery/jobs/source/story');
 let dialog;
 if(entry==='header') dialog=await open(page);
 else {
  await showActions(page);
  await page.getByRole('tab',{name:'Trace',exact:true}).click();
  await page.getByRole('tree').getByText('Validation',{exact:true}).click();
  await page.getByRole('button',{name:'Restart job',exact:true}).click();
  dialog=page.getByRole('dialog',{name:'Restart Job'});
  await expect(dialog.getByText('Validation boundary · Chapter 12 · -',{exact:true})).toBeVisible();
  await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
  await capture(page,'03-chapter-running-restart-enabled');
 }
 const reads=f.reads.length;
 await dialog.getByRole('button',{name:'Restart',exact:true}).click();
 await expect(page).toHaveURL(/jobs\/accepted-successor\/story/);
 await expect(dialog).toBeHidden();
 expect(f.posts).toHaveLength(1);
 expect(JSON.parse(f.posts[0])).toMatchObject({scope_id:'retained-boundary',goal:{mode:'new'}});
 expect(f.reads.length).toBe(reads);expect(f.cancellations).toEqual([]);
 expect(await page.evaluate(()=>sessionStorage.getItem('json-patch-v2:cortex:restart:recovery:source'))).toBeNull();
});

}

test('R06 R09 denied details permit explicit new goal and lost responses retain the draft',async({page})=>{
 const f=await fixture(page);await page.setViewportSize({width:1280,height:1100});
 await page.goto('/project/recovery/jobs/source/story');const dialog=await open(page);await patch(dialog);
 f.cancelError(403);await dialog.getByRole('button',{name:'Cancel source'}).click();
 const confirmation=page.getByRole('dialog',{name:'Cancel job?'});
 await expect(confirmation).not.toHaveClass(/ant-zoom/);
 await confirmation.getByRole('button',{name:'Cancel job',exact:true}).click();
 await expect.poll(()=>f.cancellations).toEqual(['/api/projects/recovery/jobs/source/cancel']);
 const failed=page.getByRole('dialog',{name:'Cancel failed jobs'});
 await expect(failed.getByText(/Controlled cancellation denial/)).toBeVisible();
 await failed.getByRole('button',{name:'Close',exact:true}).last().click();
 await expect(dialog.getByPlaceholder('e.g. "james@example.com"')).toHaveValue('retained browser draft');
 f.detailError(403);await dialog.getByRole('button',{name:'Refresh source'}).click();
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
 f.detailError(503);await dialog.getByRole('button',{name:'Refresh source'}).click();
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
 f.restart('lost');
 await dialog.getByRole('button',{name:'Restart',exact:true}).click();
 await expect(dialog.getByText('Restart failed',{exact:true})).toBeVisible();
 await expect(dialog.getByRole('button',{name:'Retry saved request'})).toBeEnabled();
 expect(JSON.parse(f.posts[0]).goal.mode).toBe('new');expect(f.cancellations).toHaveLength(1);
});

test('C1-M01 M05 M06 M08 X02 X03 V03 inbox return, recorded identity and explicit goal-link retirement',async({page})=>{
 await fixture(page);await page.setViewportSize({width:1280,height:900});
 await page.goto('/project/recovery/jobs/source/story');
 // Supply remembered former membership through browser navigation state.
 await page.evaluate(()=>{history.replaceState({usr:{goalId:'former-A',goalsReturnTo:'/project/recovery/jobs?view=all'},key:'fixture'},'',location.href);});
 await page.reload();
 await expect(page.getByRole('link',{name:'Back to inbox',exact:true})).toHaveAttribute('href','/project/recovery/jobs?view=all');
 await page.getByRole('tab',{name:'Graph',exact:true}).click();
 const siblings=page.getByRole('region',{name:'Other attempts at this goal'});
 await expect(siblings.getByRole('link',{name:'Goal · chosen-primary',exact:true})).toHaveAttribute('href','/project/recovery/goals/chosen-primary');
 await expect(siblings.getByRole('link',{name:'Recover release validation',exact:true})).toHaveCount(2);
 await expect(page.getByRole('link',{name:'Job · recorded-parent'})).toHaveAttribute('href','/project/recorded-tenant/jobs/recorded-parent/story');
 await expect(page.getByRole('link',{name:'Goal · former-A'})).toHaveAttribute('href','/project/recorded-tenant/goals/former-A');
 await capture(page,'11-returned-primary-navigation');
 await page.goto('/project/recovery/goals/former-A');
 await expect(page.getByText('Goal links have been retired',{exact:true})).toBeVisible();
 await expect(page.getByRole('link',{name:'Open All jobs',exact:true})).toHaveAttribute('href','/project/recovery/jobs');
 await expect(page.getByRole('region',{name:'Other attempts at this goal'})).toHaveCount(0);
 await expect(page).toHaveURL(/goals\/former-A$/);await capture(page,'12-displaced-goal-empty');
 await page.setViewportSize({width:390,height:844});
 await capture(page,'13-displaced-goal-phone');
});

// Goal-detail pagination was retired with the inbox replacement. Keep coverage
// of opaque cursors and exhausted suffixes at the current full-scan boundary.
for (const phone of [false, true]) {
 test(`C1-M08 M09 REV-001 legacy cursors retire explicitly and inbox scans retain jobs after empty suffixes (${phone ? 'phone' : 'desktop'})`, async ({ page }) => {
  await fixture(page);
  await page.setViewportSize(phone ? { width: 390, height: 844 } : { width: 1440, height: 1800 });
  const cursor = 'opaque+/=';
  const goal = '/project/recovery/goals/paged-goal';
  const cursorURL = goal + '?executionState=cells-v1&pageToken=' + encodeURIComponent(cursor);
  // Both jobs have archived execution records; only hidden membership puts a
  // job in Archive. The visible completed job must remain in All.
  const retained = { ...source, goal_id: 'paged-goal', status: 'COMPLETED', store: 'ARCHIVED' };
  const hidden = { ...retained, job_id: 'hidden', title: 'Hidden validation' };
  const reads: { token: string; inclusive: boolean }[] = [];
  await page.route('**/api/projects/recovery/jobs?**', async route => {
   const url = new URL(route.request().url());
   expect(url.searchParams.has('goal')).toBe(false);
   const token = url.searchParams.get('pageToken') || '';
   const inclusive = url.searchParams.get('includeHidden') === 'true';
   reads.push({ token, inclusive });
   if (token) {
    expect(token).toBe(cursor);
    return route.fulfill({ json: { jobs: [] } });
   }
   return route.fulfill({ json: { jobs: inclusive ? [retained, hidden] : [retained], next_page_token: cursor } });
  });
  await page.goto(goal);
  await expect(page.getByText('Goal links have been retired', { exact: true })).toBeVisible();
  await expect(page).toHaveURL(goal);
  await page.getByRole('link', { name: 'Open All jobs', exact: true }).click();
  const rows = page.getByRole('list', { name: 'Jobs' }).getByRole('listitem');
  await expect(rows).toHaveCount(1);
  await expect(rows.getByRole('link', { name: 'platform: Recover release validation', exact: true })).toHaveAttribute('href', '/project/recovery/jobs/source/story');
  await expect(page).toHaveURL('/project/recovery/jobs');
  await expect(page.getByRole('button', { name: 'Next jobs', exact: true })).toHaveCount(0);
  expect(reads).toEqual(expect.arrayContaining([
   { token: '', inclusive: false }, { token: cursor, inclusive: false },
   { token: '', inclusive: true }, { token: cursor, inclusive: true },
  ]));
  await capture(page, phone ? '16-retained-inbox-phone' : '15-retained-inbox-desktop');
  // A direct old cursor URL and reload must stay explicit: never reinterpret
  // that goal cursor as an inbox cursor or silently pick another goal.
  await page.goto(cursorURL);
  await page.reload();
  await expect(page.getByText('Goal links have been retired', { exact: true })).toBeVisible();
  await expect(page).toHaveURL(cursorURL);
  await page.getByRole('link', { name: 'Open All jobs', exact: true }).click();
  await expect(rows).toHaveCount(1);
  if (phone) await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await page.getByRole('navigation', { name: 'Inboxes' }).getByRole('link', { name: 'Archive', exact: true }).click();
  await expect(rows).toHaveCount(1);
  await expect(rows.getByRole('link', { name: 'platform: Hidden validation', exact: true })).toHaveAttribute('href', '/project/recovery/jobs/hidden/story?inbox=archive');
  await expect(page).toHaveURL('/project/recovery/jobs?inbox=archive');
 });
}

for (const width of [1280, 390, 320]) {
 test(`compact story actions remain reachable without overlap at ${width}px`, async ({ page }) => {
  await fixture(page);
  await page.setViewportSize({ width, height: 900 });
  await page.goto('/project/recovery/jobs/source/story');
  await showActions(page);
  const menu = page.getByRole('group', { name: 'Job actions', exact: true });
  const bounds = await menu.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
  expect(await menu.evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
  const refresh = menu.getByRole('switch', { name: 'Auto-refresh', exact: true });
  await expect(refresh).not.toBeChecked();
  await refresh.check();
  await expect(refresh).toBeChecked();
  await menu.getByTitle('5s', { exact: true }).click();
  await page.locator('.ant-select-dropdown:visible').getByText('10s', { exact: true }).click();
  await expect(menu.getByTitle('10s', { exact: true })).toBeVisible();
  if (width !== 320) await capture(page, width === 390 ? 'inbox-actions-phone' : 'inbox-actions-desktop', true);
  await menu.getByRole('button', { name: 'Restart', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Restart Job' })).toBeVisible();
 });
}
