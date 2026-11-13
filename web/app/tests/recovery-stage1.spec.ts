import { test, expect, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import path from 'node:path';
import { jobIdentity } from './helpers/cells';

const source={...jobIdentity,tenant_id:'recovery',job_id:'source',goal_id:'chosen-primary',other_goal_ids:['former-A','former-B'],title:'Recover release validation',prompt:'Resume validation with the retained context patch.',job_type:'recipe',recipe:'build',status:'ACTIVE',store:'ACTIVE',completion_status:'failed_app',created_at:'2026-11-12T00:00:00Z',parent:{tenant_id:'recorded-tenant',job_id:'recorded-parent',goal_id:'former-A'}};
const story={job_id:'source',status:'failed',restart_scopes:[{scope_id:'retained-boundary',chapter_ordinal:12,title:'Validation boundary'}],root:{id:'root',kind:'recipe',title:'Validation',status:'failed',path:[],attempt:1,children:[]}};
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
  if(p.endsWith('/story'))return route.fulfill({json:{...story,job_id:p.includes('accepted-successor')?'accepted-successor':'source'}});
  if(p.endsWith('/jobs/source')){reads.push(p);return detailError?route.fulfill({status:detailError,json:{error:detailError===403?'Controlled detail denial':'Controlled detail unavailable'}}):route.fulfill({json:detail});}
  if(p.endsWith('/jobs/accepted-successor'))return route.fulfill({json:{...detail,job_id:'accepted-successor'}});
  if(p.endsWith('/jobs')){
   const goal=url.searchParams.get('goal');
   const jobs=goal==='chosen-primary'?[detail,{...detail,job_id:'historical-success-1',status:'COMPLETED',store:'ARCHIVED',completion_status:'success'},{...detail,job_id:'historical-success-2',status:'COMPLETED',store:'ARCHIVED',completion_status:'success'}]:[];
   return route.fulfill({json:{jobs}});
  }
  if(p.endsWith('/children'))return route.fulfill({json:{jobs:[]}});
  if(p.endsWith('/conversation'))return route.fulfill({json:{entries:[],available:true}});
  if(p.endsWith('/input-artifacts'))return route.fulfill({json:{artifacts:[]}});
  return route.fulfill({json:{}});
 });
 return {posts,cancellations,reads,terminal:()=>{detail={...detail,status:'COMPLETED',store:'ACTIVE'};},detailError:(value:number)=>{detailError=value;},cancelError:(value:number)=>{cancellationError=value;},restart:(value:string)=>{restartFailure=value;}};
}
async function capture(page:Page,name:string){
 const directory=process.env.CORTEX_REVIEW_ARTIFACTS;
 if(!directory)return;
 mkdirSync(directory,{recursive:true});
 if(!name.endsWith('phone')) { await page.setViewportSize({width:1440,height:1800});await page.locator('.ant-modal-body').evaluateAll(elements=>elements.forEach(element=>{element.scrollTop=0;})); }
 // A visible dialog may still have zero opacity during its opening animation.
 // Capture the settled application state, including nested cancellation dialogs.
 await page.evaluate(async()=>{
  await document.fonts.ready;
  await new Promise(requestAnimationFrame);
  await Promise.all(document.getAnimations().filter(animation=>animation.effect?.getComputedTiming().iterations!==Infinity).map(animation=>animation.finished.catch(()=>{})));
 });
 await page.screenshot({path:path.join(directory,name+'.png'),fullPage:true,animations:'disabled'});
}
async function open(page:Page){
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

test('C1-R01–R15 V02 V03 explicit cancellation, retained patch, terminal confirmation, conflict and saved replay',async({page})=>{
 const f=await fixture(page);await page.setViewportSize({width:1280,height:1100});
 await page.goto('/project/recovery/jobs/source/story');
 const dialog=await open(page);await patch(dialog);
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeDisabled();
 await expect(dialog.getByRole('button',{name:'Start a new goal',exact:true})).toBeEnabled();
 await capture(page,'01-live-gating');
 await dialog.getByRole('button',{name:'Cancel source',exact:true}).click();
 const confirmation=page.getByRole('dialog',{name:'Cancel job?'});
 await expect(confirmation.getByText('source',{exact:true})).toBeVisible();
 expect(f.cancellations).toEqual([]);expect(f.posts).toEqual([]);
 await capture(page,'02-cancel-confirmation');
 await confirmation.getByRole('button',{name:'Cancel job',exact:true}).click();
 await expect(confirmation).toBeHidden();
 await expect(dialog.getByText('Cancellation requested for this source')).toBeVisible();
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeDisabled();
 await expect(dialog.getByPlaceholder('e.g. "james@example.com"')).toHaveValue('retained browser draft');
 expect(f.cancellations).toEqual(['/api/projects/recovery/jobs/source/cancel']);expect(f.posts).toEqual([]);
 await capture(page,'03-pending-retained-patch');
 f.terminal();await dialog.getByRole('button',{name:'Refresh source'}).click();
 await expect(dialog.getByText('Source completion confirmed',{exact:true})).toBeVisible();
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeEnabled();
 await capture(page,'04-terminal-confirmation');
 const reads=f.reads.length;await dialog.getByRole('button',{name:'Restart',exact:true}).click();
 await expect(dialog.getByText('Restart failed',{exact:true})).toBeVisible();
 expect(f.reads.length).toBe(reads+1);expect(f.posts).toHaveLength(1);
 await expect(dialog.getByText(/Controlled admission conflict/)).toBeVisible();
 await expect(dialog.getByRole('link',{name:'View goal'})).toHaveAttribute('href','/project/recovery/goals/chosen-primary');
 await expect(dialog.getByPlaceholder('e.g. "james@example.com"')).toHaveValue('retained browser draft');
 await capture(page,'05-conflict-exact-retry');
 const saved=f.posts[0];f.detailError(503);
 await page.reload();const reloaded=await open(page);
 await expect(reloaded.getByText('Source completion unavailable')).toBeVisible();
 await expect(reloaded.getByRole('button',{name:'Retry saved request'})).toBeEnabled();
 await capture(page,'06-reloaded-saved-retry');
 const replayReads=f.reads.length;f.restart('success');
 await reloaded.getByRole('button',{name:'Retry saved request'}).click();
 await expect(page).toHaveURL(/jobs\/accepted-successor\/story/);
 expect(f.posts).toHaveLength(2);expect(f.posts[1]).toBe(saved);
 // New page detail reads are independent; replay itself added no source read.
 expect(f.reads.length).toBe(replayReads);
});

test('C1-R06 R12 V03 denial and unavailable detail retain the patch and new-goal alternative',async({page})=>{
 const f=await fixture(page);await page.setViewportSize({width:1280,height:1100});
 await page.goto('/project/recovery/jobs/source/story');const dialog=await open(page);await patch(dialog);
 f.cancelError(403);await dialog.getByRole('button',{name:'Cancel source'}).click();
 const confirmation=page.getByRole('dialog',{name:'Cancel job?'});await confirmation.getByRole('button',{name:'Cancel job',exact:true}).click();
 const failed=page.getByRole('dialog',{name:'Cancel failed jobs'});
 await expect(failed.getByText(/Controlled cancellation denial/)).toBeVisible();
 await capture(page,'07-cancellation-denied');await failed.getByRole('button',{name:'Close',exact:true}).last().click();
 await expect(dialog.getByPlaceholder('e.g. "james@example.com"')).toHaveValue('retained browser draft');
 f.detailError(403);await dialog.getByRole('button',{name:'Refresh source'}).click();
 await expect(dialog.getByText('Source completion unavailable')).toBeVisible();
 await capture(page,'08-detail-denied');
 f.detailError(503);await dialog.getByRole('button',{name:'Refresh source'}).click();
 await expect(dialog.getByText(/Controlled detail unavailable/)).toBeVisible();
 await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeDisabled();
 await capture(page,'09-detail-unavailable');
 await dialog.getByRole('button',{name:'Start a new goal',exact:true}).click();f.restart('lost');
 await dialog.getByRole('button',{name:'Restart',exact:true}).click();
 await expect(dialog.getByText('Restart failed',{exact:true})).toBeVisible();
 await expect(dialog.getByRole('button',{name:'Retry saved request'})).toBeEnabled();
 await capture(page,'10-lost-fork-response');expect(JSON.parse(f.posts[0]).goal.mode).toBe('new');expect(f.cancellations).toHaveLength(1);
});

test('C1-M01 M05 M06 M08 X02 X03 V03 primary navigation, recorded identity and literal displaced goals',async({page})=>{
 await fixture(page);await page.setViewportSize({width:1280,height:900});
 await page.goto('/project/recovery/jobs/source/story');
 // Supply remembered former membership through browser navigation state.
 await page.evaluate(()=>{history.replaceState({usr:{goalId:'former-A',goalsReturnTo:'/project/recovery/jobs?view=all'},key:'fixture'},'',location.href);});
 await page.reload();
 await expect(page.getByRole('link',{name:'Goal overview',exact:true})).toHaveAttribute('href','/project/recovery/goals/chosen-primary');
 await page.getByRole('tab',{name:'Relationships',exact:true}).click();
 const siblings=page.getByRole('region',{name:'Other attempts at this goal'});
 await expect(siblings.getByRole('link',{name:'Recover release validation',exact:true})).toHaveCount(2);
 await expect(page.getByRole('link',{name:'Job · recorded-parent'})).toHaveAttribute('href','/project/recorded-tenant/jobs/recorded-parent/story');
 await expect(page.getByRole('link',{name:'Goal · former-A'})).toHaveAttribute('href','/project/recorded-tenant/goals/former-A');
 await capture(page,'11-returned-primary-navigation');
 await page.goto('/project/recovery/goals/former-A');
 await expect(page.getByText('No retained jobs for this goal',{exact:true})).toBeVisible();
 await expect(page).toHaveURL(/goals\/former-A$/);await capture(page,'12-displaced-goal-empty');
 await page.setViewportSize({width:390,height:844});
 await capture(page,'13-displaced-goal-phone');
});

for (const phone of [false, true]) {
 test(`C1-M08 M09 REV-001 exhausted suffix and direct cursor entry stay page-local (${phone ? 'phone' : 'desktop'})`, async ({ page }) => {
  await fixture(page);
  await page.setViewportSize(phone ? { width: 390, height: 844 } : { width: 1440, height: 1800 });
  const cursor = 'opaque+/=';
  const goal = '/project/recovery/goals/paged-goal';
  const cursorURL = goal + '?executionState=cells-v1&pageToken=' + encodeURIComponent(cursor);
  const retained = { ...source, goal_id: 'paged-goal', status: 'COMPLETED', store: 'ARCHIVED' };
  const reads: string[] = [];
  await page.route('**/api/projects/recovery/jobs/source', route => route.fulfill({ json: retained }));
  // Controlled HTTP summaries model backend primary filtering: the first page
  // retains a member; an excluded-only suffix returns an empty exhausted page.
  await page.route('**/api/projects/recovery/jobs?**', async route => {
   const url = new URL(route.request().url());
   expect(url.searchParams.get('goal')).toBe('paged-goal');
   const token = url.searchParams.get('pageToken');
   reads.push(token || '');
   if (token) {
    expect(token).toBe(cursor);
    return route.fulfill({ json: { jobs: [] } });
   }
   return route.fulfill({ json: { jobs: [retained], next_page_token: cursor } });
  });
  await page.goto(goal);
  await expect(page.getByText('Recover release validation', { exact: true }).first()).toBeVisible();
  if (!phone) await capture(page, '14-retained-first-page');
  await page.getByRole('button', { name: 'Next jobs', exact: true }).click();
  await expect(page.getByText('No retained matches on this page.', { exact: true })).toBeVisible();
  await expect(page.getByText('No retained jobs for this goal', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Next jobs', exact: true })).toHaveCount(0);
  await expect(page).toHaveURL(cursorURL);
  await expect(page.getByRole('button', { name: 'First page', exact: true })).toBeVisible();
  await capture(page, phone ? '16-empty-final-phone' : '15-empty-final-desktop');
  // A reload mounts at a noninitial cursor without knowledge of earlier pages.
  await page.reload();
  await expect(page.getByText('No retained matches on this page.', { exact: true })).toBeVisible();
  await expect(page.getByText('No retained jobs for this goal', { exact: true })).toHaveCount(0);
  await expect(page).toHaveURL(cursorURL);
  await capture(page, phone ? '18-direct-cursor-phone' : '17-direct-cursor-desktop');
  await page.getByRole('button', { name: 'First page', exact: true }).click();
  await expect(page.getByText('Recover release validation', { exact: true }).first()).toBeVisible();
  await expect(page).toHaveURL(goal + '?executionState=cells-v1');
  await expect(page.getByRole('button', { name: 'Next jobs', exact: true })).toBeEnabled();
  expect(reads).toEqual(['', cursor, cursor, '']);
 });
}
