import { test, expect, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { jobIdentity, mockCells } from './helpers/cells';
const at='2026-11-09T00:00:00Z';
const nodes=[
 {tenant_id:'demo',job_id:'root',job_type:'recipe',cell:'cortex',status:'ACTIVE',title:'Plan the DAG improvements',created_at:'2026-11-12T08:58:00Z'},
 {tenant_id:'demo',job_id:'middle',parent_job_id:'root',job_type:'generic',cell:'jobdb',status:'COMPLETED',prompt:'Validate compact records',created_at:'2026-11-12T09:01:48Z',archived_at:'2026-11-12T09:07:20Z',store:'ARCHIVED'},
 {tenant_id:'demo',job_id:'entry',parent_job_id:'middle',job_type:'recipe',cell:'c2j',status:'READY',goal_id:'goal',title:'Build the readable family graph',prompt:'Full prompt <script>literal</script> 雪',created_at:'2026-11-12T09:00:10Z',completion_status:'failed_app'},
 {tenant_id:'demo',job_id:'sibling',parent_job_id:'root',job_type:'recipe',cell:'root',status:'COMPLETED',completion_status:'success',title:'Review graph changes',created_at:'2026-11-12T09:02:00Z',archived_at:'2026-11-12T09:10:00Z'},
];
let fail=false, missing=false, large=false, repeated=false;
test.beforeEach(async({page})=>{
 fail=false;missing=false;large=false;repeated=false;
 await page.clock.setFixedTime(new Date('2026-11-12T09:18:42Z'));
 await page.context().addCookies([{name:'colony2:user:email',value:'test@example.com',url:'http://127.0.0.1:15176'}]);
 await page.addInitScript(()=>{(window as any).EventSource=class{close(){}addEventListener(){}removeEventListener(){}};});
 await page.route('**/api/**',async route=>{
  const u=new URL(route.request().url());
  if(u.pathname==='/api/projects')return route.fulfill({json:[{tenant_id:'demo'}]});
  if(u.pathname.endsWith('/tree-waits')) {const id=u.pathname.split('/').at(-2)!;return route.fulfill({json:{consumer:{tenant_id:'demo',job_id:id},waits:[],issues:[],observed_at:at,consumer_snapshot:{state:'available',observed_at:at,status:'READY'}}});}
  const match=u.pathname.match(/\/jobs\/([^/]+)\/(tree-node|tree-children)$/);
  if(match){const id=decodeURIComponent(match[1]);if(match[2]==='tree-node'){const node=nodes.find(n=>n.job_id===id);return route.fulfill(missing||!node?{status:404,json:{error:'unavailable'}}:{json:{node,observed_at:at}});}
   if(large&&id==='root')return route.fulfill({json:{nodes:Array.from({length:1001},(_,i)=>({tenant_id:'demo',job_id:'wide-'+i,parent_job_id:'root',job_type:'generic',status:'READY'})),observed_at:at,next_page_token:'unconsumed'}});
   if(repeated&&id==='root')return route.fulfill({json:{nodes:[],observed_at:at,next_page_token:'repeated'}});
   if(fail&&id==='root')return route.fulfill({status:503,json:{error:'private backend error'}});
   return route.fulfill({json:{nodes:nodes.filter(n=>n.parent_job_id===id),observed_at:at}});
  }
  if(u.pathname.endsWith('/input-artifacts')) return route.fulfill({json:{artifacts:[]}});
  if(u.pathname.endsWith('/story')) return route.fulfill({json:{...jobIdentity,job_id:'entry',status:'completed',root:{id:'root',kind:'recipe',title:'Tree story',status:'completed',path:[],attempt:1,children:[]}}});
  if(u.pathname.endsWith('/jobs/entry'))return route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'entry',job_type:'recipe',status:'READY',store:'ACTIVE'}});
  return route.fulfill({json:[]});
 });
});
test('TREE-001,012,013,014,017,020,021,030 diagram and loaded search preserve entry route',async({page})=>{
 await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 await expect(page.locator('path[data-parent="middle"][data-child="entry"]')).toHaveCount(1);
 await expect(page.locator('path[data-parent="root"]')).toHaveCount(2);
 const diagram=page.getByLabel('Execution diagram');
 await diagram.getByRole('button',{name:'Review graph changes · root',exact:false}).click();
 const details=page.getByRole('complementary',{name:'Selected job details'});
 await expect(details).toContainText('success');await expect(page).toHaveURL(/entry\/tree$/);
 await page.getByRole('button',{name:'Toggle branch Plan the DAG improvements'}).click();
 await expect(diagram.getByRole('button',{name:'Review graph changes · root',exact:false})).toHaveCount(0);
 await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('c2j');
 await expect(diagram.locator('path[data-child="entry"]')).toHaveCount(1);
 await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('');
 await page.getByRole('button',{name:'Find opened job'}).click();await expect(details).toContainText('Entry' .toLowerCase());
 await page.getByRole('button',{name:'Zoom in'}).click();await expect(page.getByLabel('Zoom',{exact:true})).toContainText('120%');
 await page.getByRole('button',{name:'Fit',exact:true}).click();
 await expect(details).toContainText('No children found in this traversal');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText(at);
 if(process.env.CORTEX_TREE_SCREENSHOTS){mkdirSync(process.env.CORTEX_TREE_SCREENSHOTS,{recursive:true});await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/desktop.png',fullPage:true});}
 await page.getByRole('button',{name:'Refresh tree'}).click();await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
});
test('TREE-015,016 generic summary and keyboard mobile graph',async({page})=>{
 await page.setViewportSize({width:390,height:844});await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 const outline=page.getByLabel('Execution diagram');const middle=outline.getByRole('button',{name:'Validate compact records · jobdb',exact:false});await middle.focus();await page.keyboard.press('Enter');
 const details=page.getByRole('complementary',{name:'Selected job details'});await expect(details).toContainText('Generic job summary');await expect(details.getByRole('link',{name:'Open execution'})).toHaveCount(0);
 await expect(middle).toHaveAttribute('aria-pressed','true');await expect(page.getByLabel('Execution diagram')).toBeVisible();
 const entry=outline.getByRole('button',{name:'Build the readable family graph · c2j · Opened job',exact:false});await entry.click();await expect(details.getByRole('link',{name:'Open execution'})).toHaveAttribute('href','/project/demo/jobs/entry/story');
 await expect(outline.getByRole('button',{name:'Toggle branch Plan the DAG improvements'})).toHaveAttribute('aria-expanded','true');
 if(process.env.CORTEX_TREE_SCREENSHOTS){mkdirSync(process.env.CORTEX_TREE_SCREENSHOTS,{recursive:true});await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/mobile.png',fullPage:true});}
});
test('TREE-009,012 failure keeps entry and retry recovers',async({page})=>{
 fail=true;await page.goto('/project/demo/jobs/entry/tree');await expect(page.getByText('Partial discovery',{exact:true})).toBeVisible();
 const outline=page.getByLabel('Execution diagram');await outline.getByRole('button',{name:'Plan the DAG improvements · cortex',exact:false}).click();
 await expect(page.getByRole('complementary',{name:'Selected job details'})).toContainText('Child page unavailable');fail=false;
 await page.getByRole('button',{name:'Retry child page'}).click();await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
});
test('TREE-008 unavailable entry offers retry and back with no fabricated graph',async({page})=>{
 missing=true;await page.goto('/project/demo/jobs/entry/tree');await expect(page.getByRole('alert')).toContainText('Entry job unavailable');await expect(page.getByLabel('Execution diagram')).toHaveCount(0);
 await expect(page.getByRole('link',{name:'Back to jobs'})).toHaveAttribute('href','/project/demo/goals');missing=false;await page.getByRole('button',{name:'Retry entry'}).click();await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();await expect(page.getByLabel('Execution diagram')).toBeVisible();
});

test('TREE-015,018 story opens and returns to original tree entry',async({page})=>{
 await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 await page.getByRole('complementary',{name:'Selected job details'}).getByRole('link',{name:'Open execution'}).click();
 await expect(page).toHaveURL(/entry\/story$/);
 await expect(page.getByRole('link',{name:'DAG',exact:true})).toHaveAttribute('href','/project/demo/jobs/entry/story?tab=dag');
 await page.getByRole('link',{name:'Return to DAG',exact:true}).click();
 await expect(page).toHaveURL(/entry\/story\?tab=dag$/);
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
});

test('TREE-010,012 node limits retain loaded jobs and expose branch entry',async({page})=>{
 large=true;await page.goto('/project/demo/jobs/root/tree');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('1000 loaded jobs');
 const details=page.getByRole('complementary',{name:'Selected job details'});
 await expect(details).toContainText('Node limit; page records omitted');
 await expect(details.getByRole('link',{name:'Open branch as entry'})).toHaveAttribute('href','/project/demo/jobs/root/story?tab=dag');
 await expect(page.getByText('Partial discovery',{exact:true})).toBeVisible();
});
test('TREE-005,012 repeated empty continuation is incomplete rather than empty exhaustion',async({page})=>{
 repeated=true;await page.goto('/project/demo/jobs/root/tree');
 const details=page.getByRole('complementary',{name:'Selected job details'});
 await expect(details).toContainText('Repeated cursor; incomplete branch');
 await expect(details).not.toContainText('No children found in this traversal');
 await expect(page.getByText('Partial discovery',{exact:true})).toBeVisible();
});

test('TREE-021 singleton claims exhaustion only after a successful child listing',async({page})=>{
 let release!:()=>void;const gate=new Promise<void>(resolve=>release=resolve);
 await page.route('**/jobs/root/tree-children?*',async route=>{await gate;await route.fulfill({json:{nodes:[],observed_at:at}});});
 await page.goto('/project/demo/jobs/root/tree');
 const details=page.getByRole('complementary',{name:'Selected job details'});
 await expect(details).toBeVisible();await expect(details).not.toContainText('No children found in this traversal');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('Discovering');
 release();await expect(details).toContainText('No children found in this traversal');await expect(details).toContainText('No recorded parent');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('1 loaded jobs');
});
test('TREE-007 unavailable ancestor remains a boundary without inferred bridge edges',async({page})=>{
 await page.route('**/jobs/root/tree-node',route=>route.fulfill({status:404,json:{error:'Unavailable'}}));
 await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('Ancestor unavailable; known component retained');
 await expect(page.locator('path[data-parent="middle"][data-child="entry"]')).toHaveCount(1);
 await expect(page.locator('path[data-parent="root"]')).toHaveCount(0);
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('2 loaded jobs');
});

test('DAG-01,02,04,05,08,15 embedded DAG discovers lazily and retains local state', async ({page}) => {
 let discoveries = 0;
 page.on('request', r => { if (/tree-(node|children)/.test(r.url())) discoveries++; });
 await page.goto('/project/demo/jobs/entry/story');
 await expect(page.getByRole('tab',{name:'Conversation',exact:true})).toHaveAttribute('aria-selected','true');
 expect(discoveries).toBe(0);
 await page.getByRole('tab',{name:'DAG',exact:true}).click();
 await expect(page).toHaveURL(/story\?tab=dag$/);
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 expect(discoveries).toBeGreaterThan(0);
 const panel = page.getByRole('tabpanel',{name:'DAG',exact:true});
 await expect(panel.getByRole('heading',{name:'DAG',exact:true})).toHaveCount(0);
 await expect(panel.getByRole('link',{name:'Back to jobs'})).toHaveCount(0);
 await panel.getByLabel('Execution diagram').getByRole('button',{name:'Review graph changes · root',exact:false}).click();
 await expect(page).toHaveURL(/entry\/story\?tab=dag$/);
 const detail=panel.getByRole('complementary',{name:'Selected job details'});
 await expect(detail.getByRole('link',{name:'Open execution'})).toHaveAttribute('href','/project/demo/jobs/sibling/story');
 await panel.getByRole('button',{name:'Zoom in'}).click();
 await panel.getByRole('button',{name:'Toggle branch Plan the DAG improvements'}).click();
 await panel.getByRole('searchbox').fill('c2j');
 const scroll=await panel.getByLabel('Execution diagram').evaluate(e=>{e.scrollLeft=120;e.scrollTop=40;return {left:e.scrollLeft,top:e.scrollTop};});
 const count=discoveries;
 await page.getByRole('tab',{name:'Trace',exact:true}).click();
 await page.goBack();
 await expect(page.getByRole('tab',{name:'DAG',exact:true})).toHaveAttribute('aria-selected','true');
 await expect(panel.getByLabel('Zoom',{exact:true})).toHaveText('120%');
 await expect(panel.getByRole('searchbox')).toHaveValue('c2j');
 expect(await panel.getByLabel('Execution diagram').evaluate(e=>({left:e.scrollLeft,top:e.scrollTop}))).toEqual(scroll);
 await expect(detail).toContainText('sibling');
 await panel.getByRole('searchbox').fill('');
 await expect(panel.getByRole('button',{name:'Toggle branch Plan the DAG improvements'})).toHaveAttribute('aria-expanded','false');
 expect(discoveries).toBe(count);
 await panel.getByRole('button',{name:'Toggle branch Plan the DAG improvements'}).click();
 if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/embedded-desktop.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});
 // Wait until the responsive shell has adopted its phone navigation.
 await expect(page.getByRole('button',{name:'Open navigation',exact:true})).toBeVisible();
 await expect(panel.getByLabel('Execution diagram')).toBeVisible();
 await expect(panel.getByRole('region',{name:'Execution outline'})).toHaveCount(0);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
 if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/embedded-mobile.png',fullPage:true});
});

test('DAG-06 interrupted entry aborts on tab departure and resumes', async ({page}) => {
 let release!:()=>void; const gate=new Promise<void>(r=>release=r); let calls=0; let resumed=false;
 await page.route('**/jobs/entry/tree-node',async route=>{calls++; if(!resumed) await gate; await route.fulfill({json:{node:nodes[2],observed_at:at}}).catch(()=>{});});
 await page.goto('/project/demo/jobs/entry/story?tab=dag');
 await expect.poll(()=>calls).toBe(1);
 const initialCalls=calls;
 await page.getByRole('tab',{name:'Trace',exact:true}).click();
 await expect(page.locator('#job-panel-dag .tree-coverage')).toContainText('Partial discovery');
 expect(calls).toBe(1);
 resumed=true;
 await page.getByRole('tab',{name:'DAG',exact:true}).click();
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 release();
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
 expect(calls).toBe(initialCalls+1);
 if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/resumed-discovery.png',fullPage:true});
});

test('DAG-07 identity changes dispose loaded state; DAG-09,10 recipe returns target original entry',async({page})=>{
 await page.goto('/project/demo/jobs/entry/story?tab=dag');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 await page.getByLabel('Execution diagram').getByRole('button',{name:'Plan the DAG improvements · cortex',exact:false}).click();
 await page.getByRole('complementary',{name:'Selected job details'}).getByRole('link',{name:'Open execution'}).click();
 await expect(page).toHaveURL(/root\/story$/);
 await page.getByRole('link',{name:'DAG',exact:true}).click();
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
 await expect(page.getByRole('complementary',{name:'Selected job details'}).locator('dd').nth(1)).toHaveText('root');
 await expect(page.getByLabel('Zoom',{exact:true})).toHaveText('100%');
 await page.getByRole('link',{name:'Return to DAG',exact:true}).click();
 await expect(page).toHaveURL(/entry\/story\?tab=dag$/);
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
});

 test('DAG-17,18 Run Summary in Trace and Input artifacts show directly in the browser',async({page})=>{
  await page.goto('/project/demo/jobs/entry/story?tab=summary');
  const summary=page.getByRole('tabpanel',{name:'Trace',exact:true});
  await expect(summary.getByRole('heading',{name:'Run Summary'})).toBeVisible();
  await expect(summary.locator('details')).toHaveCount(0);
  await expect(summary.getByText('COMPLETED', {exact:true})).toBeVisible();
  await expect(summary.getByRole('tree')).toBeVisible();
  await expect(page.getByRole('tab',{name:'Summary',exact:true})).toHaveCount(0);
  if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/summary-direct.png',fullPage:true});
  await page.getByRole('tab',{name:'Input artifacts',exact:true}).click();
  const inputs=page.getByRole('tabpanel',{name:'Input artifacts',exact:true});
  await expect(inputs.getByText('No input artifacts')).toBeVisible();
  await expect(inputs.locator('details')).toHaveCount(0);
  if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/input-artifacts-direct.png',fullPage:true});
 });

test('DAG-09,10 generic entry keeps standalone return navigation',async({page})=>{
 await page.goto('/project/demo/jobs/middle/tree');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 await page.getByLabel('Execution diagram').getByRole('button',{name:'Build the readable family graph · c2j',exact:false}).click();
 await page.getByRole('complementary',{name:'Selected job details'}).getByRole('link',{name:'Open execution'}).click();
 await expect(page.getByRole('link',{name:'Return to DAG',exact:true})).toHaveAttribute('href','/project/demo/jobs/middle/tree');
});

test('DAG-07 a tenant history change starts a fresh workspace',async({page})=>{
 await page.route('**/projects/other/jobs/entry/tree-node',route=>route.fulfill({json:{node:{tenant_id:'other',job_id:'entry',job_type:'recipe',status:'READY'},observed_at:at}}));
 await page.route('**/projects/other/jobs/entry/tree-children?*',route=>route.fulfill({json:{nodes:[],observed_at:at}}));
 await page.goto('/project/demo/jobs/entry/story?tab=dag');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
 await page.getByRole('button',{name:'Zoom in'}).click();
 await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('saved');
 await page.evaluate(()=>{history.pushState(null,'','/project/other/jobs/entry/story?tab=dag');dispatchEvent(new PopStateEvent('popstate'));});
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('1 loaded jobs');
 await expect(page.getByRole('complementary',{name:'Selected job details'}).locator('dd').first()).toHaveText('other');
 await expect(page.getByLabel('Zoom',{exact:true})).toHaveText('100%');
 await expect(page.getByRole('searchbox',{name:'Search loaded jobs'})).toHaveValue('');
 await expect(page.getByLabel('Execution diagram').getByRole('button',{name:'Review graph changes · root',exact:false})).toHaveCount(0);
});

async function checkGeometry(page: Page) {
 const result=await page.evaluate(()=>{
  const cards=[...document.querySelectorAll<HTMLElement>('.tree-card')];
  const boxes=new Map(cards.map(card=>[card.dataset.jobId!,card.getBoundingClientRect()]));
  const errors:string[]=[];
  for(const path of document.querySelectorAll<SVGPathElement>('.tree-canvas path')) {
   const parent=boxes.get(path.dataset.parent!)!,child=boxes.get(path.dataset.child!)!;
   const matrix=path.getScreenCTM()!;
   const start=path.getPointAtLength(0).matrixTransform(matrix),end=path.getPointAtLength(path.getTotalLength()).matrixTransform(matrix);
   const close=(a:number,b:number)=>Math.abs(a-b)<=2;
   if(!close(start.x,parent.right)||!close(start.y,parent.y+parent.height/2)||!close(end.x,child.left)||!close(end.y,child.y+child.height/2)) errors.push('Endpoint mismatch '+path.dataset.child);
   if(parent.right>=child.left)errors.push('Non-horizontal family '+path.dataset.child);
   // Sample the actual rendered path: no portion may enter any card interior.
   for(let d=0;d<=path.getTotalLength();d+=2) {
    const point=path.getPointAtLength(d).matrixTransform(matrix);
    if([...boxes.values()].some(b=>point.x>b.left+2&&point.x<b.right-2&&point.y>b.top+2&&point.y<b.bottom-2)) {errors.push('Card intersection '+path.dataset.child);break;}
   }
  }
  const scale=Number(document.querySelector('.tree-canvas')!.getAttribute('style')!.match(/scale\(([^)]+)/)![1]);
  for(const card of cards) {
   const b=card.getBoundingClientRect();
   if(Math.abs(b.width-272*scale)>2||Math.abs(b.height-228*scale)>2)errors.push('Unbounded card '+card.dataset.jobId);
   const button=card.querySelector('button')!.getBoundingClientRect();
   if(button.bottom>b.bottom+1||button.right>b.right+1)errors.push('Button overflow '+card.dataset.jobId);
  }
  return {errors,edges:document.querySelectorAll('.tree-canvas path').length,cards:cards.length};
 });
 expect(result.errors).toEqual([]);expect(result.cards).toBeGreaterThan(0);return result;
}
async function capture(page:Page,name:string) {
 if(!process.env.CORTEX_TREE_SCREENSHOTS)return;
 mkdirSync(process.env.CORTEX_TREE_SCREENSHOTS,{recursive:true});
 await page.screenshot({path:`${process.env.CORTEX_TREE_SCREENSHOTS}/${name}.png`,fullPage:!name.endsWith('confirmation'),animations:'disabled'});
}
for(const size of [{width:1600,height:1000},{width:390,height:844}]) {
 test(`DAGUI-006,008,018,025–032 rendered graph geometry, markers and controls ${size.width}`,async({page})=>{
  await page.setViewportSize(size);
  let reads=0;page.on('request',r=>{if(/tree-(node|children)/.test(r.url()))reads++;});
  await page.goto('/project/demo/jobs/entry/tree');
  await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
  const entry=page.locator('.tree-card[data-job-id="entry"]'),root=page.locator('.tree-card[data-job-id="root"]');
  const details=page.getByRole('complementary');
  await expect(entry.locator('.tree-card-select')).toHaveAttribute('aria-pressed','true');
  await expect(entry).toContainText('Submitted +2m 10s');await expect(entry).toContainText('Age 18m 32s');
  await expect(entry).toContainText('Ready');await expect(entry).not.toContainText('Failed');
  await expect(page.locator('.tree-outline')).toHaveCount(0);
  expect((await checkGeometry(page)).edges).toBe(3);
  const fill=await entry.evaluate(e=>getComputedStyle(e).backgroundColor);
  expect(fill).toBe('rgb(230, 244, 255)');
  await expect(details.getByRole('link',{name:'Goal overview'})).toHaveAttribute('href','/project/demo/goals/goal');
  await expect(details.getByRole('link',{name:'Goal overview'})).toHaveClass('tree-detail-action');
  await expect(details.getByRole('link',{name:'Open execution'})).toHaveClass('tree-detail-action');
  await page.getByRole('button',{name:'Fit',exact:true}).click();await checkGeometry(page);
  await capture(page,`dag-${size.width}`);
  await root.locator('.tree-card-select').focus();await page.keyboard.press('Enter');
  await expect(root.locator('.tree-card-select')).toHaveAttribute('aria-pressed','true');
  await expect(entry).toHaveClass(/tree-entry/);await expect(entry).toContainText('Opened job');
  await expect(entry.locator('.tree-card-select')).toHaveAttribute('aria-pressed','false');
  await capture(page,`selection-${size.width}`);
  await root.locator('.tree-branch').focus();await page.keyboard.press('Enter');
  await expect(page.locator('.tree-card')).toHaveCount(1);expect((await checkGeometry(page)).edges).toBe(0);
  const before=reads;
  await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('literal');await expect(page.locator('.tree-card')).toHaveCount(4);
  await expect(entry).toHaveClass(/tree-match/);expect(await entry.evaluate(e=>getComputedStyle(e).backgroundColor)).toBe(fill);
  expect(reads).toBe(before);await checkGeometry(page);
  await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('readable family');await expect(page.getByText('1 loaded matches')).toBeVisible();expect(reads).toBe(before);
  await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('');await expect(page.locator('.tree-card')).toHaveCount(1);
  await page.getByRole('button',{name:'Find opened job'}).focus();await page.keyboard.press('Enter');
  await expect(entry.locator('.tree-card-select')).toBeFocused();await expect(entry.locator('.tree-card-select')).toHaveAttribute('aria-pressed','true');
  await expect(entry.locator('.tree-card-select')).toBeInViewport();await checkGeometry(page);
  await page.getByRole('button',{name:'Zoom in'}).click();await checkGeometry(page);
  await page.getByRole('button',{name:'Zoom out'}).click();await checkGeometry(page);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  if(size.width<500) {
   const graph=await page.getByLabel('Execution diagram').boundingBox(),detail=await details.boundingBox();
   expect(detail!.y).toBeGreaterThanOrEqual(graph!.y+graph!.height);
  }
  await expect(page).toHaveURL(/entry\/tree$/);
 });
}
test('DAGUI-019,024 ancestor retry keeps honest unavailable timing and recovers root',async({page})=>{
 let blocked=true;
 await page.route('**/jobs/root/tree-node',r=>r.fulfill(blocked?{status:404,json:{error:'unavailable'}}:{json:{node:nodes[0],observed_at:at}}));
 await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByText('Partial discovery',{exact:true})).toBeVisible();
 const entry=page.locator('.tree-card[data-job-id="entry"]');
 await expect(entry).toContainText('Submitted Unavailable');await expect(entry).toContainText('Age 18m 32s');
 await capture(page,'partial-ancestry');
 // Partial coverage can render before the current traversal has finished.
 // Keep the fixture unavailable until the manual retry can actually start.
 const resume=page.getByRole('button',{name:'Resume discovery'});
 await expect(resume).toBeEnabled();blocked=false;await resume.click();
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();await expect(entry).toContainText('Submitted +2m 10s');
});
test('DAGUI-027 bounded long Unicode and multiline text, missing optional fields, unavailable timing',async({page})=>{
 await page.route('**/jobs/entry/tree-node',r=>r.fulfill({json:{node:{...nodes[2],title:'Long title 雪 🌳\n'.repeat(80),prompt:'full prompt'},observed_at:at}}));
 await page.route('**/jobs/middle/tree-children?*',r=>r.fulfill({json:{nodes:[],observed_at:at}}));
 await page.route('**/jobs/root/tree-node',r=>r.fulfill({json:{node:{...nodes[0],created_at:undefined},observed_at:at}}));
 await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 await checkGeometry(page);
 await expect(page.locator('.tree-card[data-job-id="entry"] strong')).toHaveText('Long title 雪 🌳\n'.repeat(80).trim());
 await expect(page.locator('.tree-card[data-job-id="entry"]')).toContainText('Submitted Unavailable');
 await capture(page,'unavailable-long-text');
});

for(const stage of ['ancestor','child-page']) {
 test(`DAGUI-020 automatic ${stage} recovery preserves cursors and selection`,async({page})=>{
  let release!:()=>void;const gate=new Promise<void>(r=>release=r);
  let pending=0,resumed=false;const cursors:string[]=[];
  const pattern=stage==='ancestor'?'**/jobs/root/tree-node':'**/jobs/root/tree-children?*';
  await page.route(pattern,async route=>{
   const token=new URL(route.request().url()).searchParams.get('pageToken')||'';
   if(stage==='child-page') {
    cursors.push(token);
    if(!token) return route.fulfill({json:{nodes:[nodes[1]],next_page_token:'next',observed_at:at}});
   }
   pending++;if(!resumed)await gate;
   await route.fulfill({json:stage==='ancestor'?{node:nodes[0],observed_at:at}:{nodes:[nodes[3]],observed_at:at}}).catch(()=>{});
  });
  await page.goto('/project/demo/jobs/entry/story?tab=dag');await expect.poll(()=>pending).toBe(1);
  const entry=page.locator('.tree-card[data-job-id="entry"]');await entry.locator('.tree-card-select').click();
  await page.getByRole('tab',{name:'Trace',exact:true}).click();resumed=true;
  await page.getByRole('tab',{name:'DAG',exact:true}).click();
  await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
  await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
  await expect(entry.locator('.tree-card-select')).toHaveAttribute('aria-pressed','true');
  expect(pending).toBe(2);if(stage==='child-page')expect(cursors).toEqual(['','next','next']);
  release();
 });
}

for(const size of [{width:1600,height:1000},{width:390,height:844}]) {
 test(`DAG-REVIEW-001 initial hidden document discovers and reveals opened child on visibility ${size.width}`,async({page})=>{
  await page.setViewportSize(size);
  await page.addInitScript(()=>{
   const state=window as Window & {dagReviewVisibility?:DocumentVisibilityState};
   state.dagReviewVisibility='hidden';
   Object.defineProperty(document,'visibilityState',{configurable:true,get:()=>state.dagReviewVisibility});
  });
  let reads=0;
  page.on('request',request=>{if(/tree-(node|children)/.test(request.url()))reads++;});
  await page.goto('/project/demo/jobs/entry/tree');
  await expect(page.getByRole('button',{name:'Resume discovery'})).toBeEnabled();
  await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('0 loaded jobs');
  expect(reads).toBe(0);await expect(page.locator('.tree-card')).toHaveCount(0);
  const visibility=(value:DocumentVisibilityState)=>page.evaluate(value=>{
   (window as Window & {dagReviewVisibility?:DocumentVisibilityState}).dagReviewVisibility=value;
   document.dispatchEvent(new Event('visibilitychange'));
  },value);
  await visibility('visible');
  await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
  await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
  await expect(page.locator('.tree-card[data-job-id="root"]')).toHaveCount(1);
  await expect(page.locator('.tree-card[data-job-id="middle"]')).toHaveCount(1);
  const opened=page.locator('.tree-card[data-job-id="entry"] .tree-card-select');
  await expect(opened).toHaveAttribute('aria-pressed','true');await expect(opened).toBeInViewport();
  await expect(opened).not.toBeFocused();await expect(opened).toContainText('Submitted +2m 10s');
  expect((await checkGeometry(page)).edges).toBe(3);
  await capture(page,`initial-visibility-${size.width}`);
  const completedReads=reads;
  await visibility('hidden');await visibility('visible');
  await expect(page.getByRole('button',{name:'Resume discovery'})).toBeDisabled();
  expect(reads).toBe(completedReads);
 });
}

for (const route of ['tree','story?tab=dag']) for (const width of [1440,390]) {
 test(`selected job header icons support tooltips and keyboard use in ${route} at ${width}px`,async({page})=>{
  await page.setViewportSize({width,height:1000});
  await page.route('**/jobs/entry/tree-node',r=>r.fulfill({json:{node:{...nodes[2],status:'ACTIVE'},observed_at:at}}));
  await page.route('**/jobs/*/tree-children?*',r=>{
   const id=new URL(r.request().url()).pathname.split('/').at(-2);
   return r.fulfill({json:{nodes:nodes.filter(n=>n.parent_job_id===id).map(n=>n.job_id==='entry'?{...n,status:'ACTIVE'}:n),observed_at:at}});
  });
  await page.goto('/project/demo/jobs/entry/'+route);
  await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
  const details=page.getByRole('complementary',{name:'Selected job details'});
  await expect(details.locator('.tree-detail-status')).toHaveText('Running');
  const actions=details.getByRole('group',{name:'Selected job actions'});
  const cancel=actions.getByRole('button',{name:'Cancel job',exact:true});
  const goal=actions.getByRole('link',{name:'Goal overview',exact:true});
  const execution=actions.getByRole('link',{name:'Open execution',exact:true});
  await expect(goal).toHaveAttribute('href','/project/demo/goals/goal');
  await expect(execution).toHaveAttribute('href','/project/demo/jobs/entry/story');
  expect(await details.locator('dl').evaluate(dl=>!!(dl.compareDocumentPosition(document.querySelector('.tree-detail-actions')!)&Node.DOCUMENT_POSITION_PRECEDING))).toBe(true);
  const measured=[details,actions,cancel,goal,execution];
  // Async discovery can briefly replace the pane; measure a complete visible frame.
  await expect.poll(async()=>Promise.all(measured.map(e=>e.boundingBox()))).not.toContain(null);
  const boxes=await Promise.all(measured.map(e=>e.boundingBox()));
  const [pane,group,...controls]=boxes;
  expect(Math.abs(group!.x+group!.width-(pane!.x+pane!.width-17))).toBeLessThanOrEqual(1);
  expect(group!.y-pane!.y).toBeLessThanOrEqual(18);
  for (const box of controls) {
   expect(box!.width).toBe(width<768?44:32);expect(box!.height).toBe(width<768?44:32);
  }
  for (const control of [cancel,goal,execution]) {
   await expect(control.locator('svg')).toHaveCount(1);expect(await control.textContent()).toBe('');
  }
  await page.mouse.move(0,0);
  await details.getByRole('heading',{name:'Selected job'}).focus();
  await page.keyboard.press('Tab');await expect(cancel).toBeFocused();
  await expect(page.getByRole('tooltip',{name:'Cancel job',exact:true})).toBeVisible();
  await page.keyboard.press('Enter');await expect(page.getByRole('dialog')).toContainText('Job: entry');
  // The modal moves focus after its opening transition; Escape must originate inside it.
  await expect.poll(()=>page.locator('.ant-modal-wrap').evaluate(e=>e.contains(document.activeElement))).toBe(true);
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0);await expect(cancel).toBeFocused();
  await page.keyboard.press('Tab');await expect(goal).toBeFocused();
  await expect(page.getByRole('tooltip',{name:'Goal overview',exact:true})).toBeVisible();
  await page.keyboard.press('Tab');await expect(execution).toBeFocused();
  await expect(page.getByRole('tooltip',{name:'Open execution',exact:true})).toBeVisible();
  await capture(page,`job-actions-${width}-${route.startsWith('story')?'dag':'tree'}`);
  await execution.blur();await expect(page.getByRole('tooltip',{name:'Open execution',exact:true})).toBeHidden();
  if(process.env.CORTEX_TREE_SCREENSHOTS)await details.screenshot({path:`${process.env.CORTEX_TREE_SCREENSHOTS}/job-actions-pane-${width}-${route.startsWith('story')?'dag':'tree'}.png`,animations:'disabled'});
  await cancel.hover();await expect(page.getByRole('tooltip',{name:'Cancel job',exact:true})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  await execution.focus();
  await page.keyboard.press('Shift+Tab');await expect(goal).toBeFocused();
  await page.keyboard.press('Enter');await expect(page).toHaveURL('/project/demo/goals/goal');
 });
}

for (const route of ['tree','story?tab=dag']) for (const width of [1440,390]) {
 test(`cancel selected generic job in ${route} at ${width}px without resetting the diagram`,async({page})=>{
  await page.setViewportSize({width,height:1000});
  const child={...nodes[1],status:width===390?'PENDING_JOBS':'CRASH_CONCERN',archived_at:undefined,store:'ACTIVE',completion_status:'failed_app'};
  let submitted=false, finished=false;
  const mutations:{tenant:string;job:string;reason:string}[]=[];
  await page.route('**/jobs/*/tree-node', async r=>{
   const id=new URL(r.request().url()).pathname.split('/').at(-2);
   const node=id==='middle' ? (finished?{...child,status:'COMPLETED',store:'ARCHIVED',archived_at:'2026-11-12T09:18:42Z',completion_status:'cancelled'}:child) : nodes.find(n=>n.job_id===id);
   await r.fulfill({json:{node,observed_at:at}});
  });
  // Listings intentionally keep old status; exact observations must win.
  await page.route('**/jobs/*/tree-children?*',r=>{
   const id=new URL(r.request().url()).pathname.split('/').at(-2);
   return r.fulfill({json:{nodes:nodes.filter(n=>n.parent_job_id===id).map(n=>n.job_id==='middle'?child:n),observed_at:at}});
  });
  await page.route('**/jobs/*/cancel',async r=>{
   expect(r.request().method()).toBe('POST');
   const path=new URL(r.request().url()).pathname;
   mutations.push({tenant:path.split('/')[3],job:path.split('/').at(-2)!,reason:r.request().postDataJSON().reason});
   submitted=true;await r.fulfill({status:204});
  });
  await page.goto('/project/demo/jobs/entry/'+route);
  await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
  const card=page.locator('.tree-card[data-job-id="middle"]');
  await card.locator('.tree-card-select').click();
  const details=page.getByRole('complementary',{name:'Selected job details'});
  await expect(details).toContainText(width===390?'Waiting for jobs':'Job keeps failing');
  await expect(details.getByRole('link',{name:'Open execution'})).toHaveCount(0);
  await card.getByRole('button',{name:'Toggle branch Validate compact records'}).click();
  await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('Review');
  await page.getByRole('button',{name:'Zoom in'}).click();
  await details.scrollIntoViewIfNeeded();
  await capture(page,`cancel-${width}-${route.startsWith('story')?'dag':'tree'}-selected`);
  const diagramScroll=await page.getByLabel('Execution diagram').evaluate(e=>[e.scrollLeft,e.scrollTop]);
  const trigger=details.getByRole('button',{name:'Cancel job'});await trigger.focus();await page.keyboard.press('Enter');
  const dialog=page.getByRole('dialog');
  await expect(dialog).toContainText('Tenant: demo');await expect(dialog).toContainText('Job: middle');
  await expect(dialog).toContainText('Child jobs already submitted will not be cancelled');
  await dialog.getByRole('textbox',{name:'Cancellation message (optional)'}).fill(' Stop selected job ');
  await expect(dialog).not.toHaveClass(/zoom-(appear|enter)/);
  await expect(page.locator('.ant-modal-mask')).not.toHaveClass(/fade-(appear|enter)/);
  await capture(page,`cancel-${width}-${route.startsWith('story')?'dag':'tree'}-confirmation`);
  await dialog.getByRole('button',{name:'Cancel job',exact:true}).focus();await page.keyboard.press('Enter');
  await expect(dialog).toHaveCount(0);expect(submitted).toBe(true);
  expect(mutations).toEqual([{tenant:'demo',job:'middle',reason:'Stop selected job'}]);
  await expect(details).toContainText('Cancellation requested');
  await expect(details.getByRole('button',{name:'Cancel job'})).toHaveCount(0);
  await expect(details.getByRole('heading',{name:'Selected job'})).toBeFocused();
  await expect(card).toContainText('Cancellation requested');
  expect(await page.getByLabel('Execution diagram').evaluate(e=>[e.scrollLeft,e.scrollTop])).toEqual(diagramScroll);
  await expect(page.getByRole('searchbox',{name:'Search loaded jobs'})).toHaveValue('Review');
  await expect(page.getByLabel('Zoom',{exact:true})).toHaveText('120%');
  await expect(card.locator('.tree-branch')).toHaveAttribute('aria-expanded','false');
  await expect(card.locator('.tree-card-select')).toHaveAttribute('aria-pressed','true');
  await capture(page,`cancel-${width}-${route.startsWith('story')?'dag':'tree'}-pending`);
  finished=true;await details.getByRole('button',{name:'Refresh status'}).click();
  await expect(details).toContainText('Cancelled');await expect(card).toContainText('Cancelled');
  await expect(details.getByRole('button',{name:'Refresh status'})).toHaveCount(0);
  await capture(page,`cancel-${width}-${route.startsWith('story')?'dag':'tree'}-finished`);
  expect(mutations).toHaveLength(1);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  await page.getByRole('button',{name:'Refresh tree'}).click();
  await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
  await card.locator('.tree-card-select').click();await expect(details).toContainText('Cancelled');
 });
}


const waitAt='2026-11-12T09:00:00Z';
const waitJob=(id:string,condition='complete')=>({target:{jobId:id},condition,state:'PENDING'});
function waitsPage(id:string,dependencies:any[]=[],token='') {
 return {consumer:{tenant_id:'demo',job_id:id},waits:dependencies.length?[{jobKey:{tenantId:'demo',jobId:id},waitId:'prerequisites',state:'PENDING',dependencies}]:[],issues:[],observed_at:waitAt,next_page_token:token,consumer_snapshot:{state:'available',observed_at:waitAt,status:'READY'}};
}
async function recursiveFixture(page:Page,{dense=false,cycle=false,partial=false}={}) {
 await mockCells(page,['platform']);
 const make=(job_id:string,title:string,parent_job_id?:string)=>({...nodes[2],job_id,title,parent_job_id,cell:'platform',status:'ACTIVE',created_at:waitAt,prompt:'Full dependency prompt 雪 <script>literal</script>'});
 const records=[make('entry','Publish the reviewed change'),make('dependency','Prepare the shared dependency','outside'),make('nested','Validate nested dependency'),make('child','Build downstream child','nested'),make('leaf','Review downstream leaf'),...(dense?Array.from({length:51},(_,i)=>make('target-'+i,'Independent task '+i)):[])];
 const links:Record<string,string[]>={entry:['dependency',...(dense?Array.from({length:51},(_,i)=>'target-'+i):[])],dependency:['nested'],child:['leaf'],...(cycle?{leaf:['dependency'],nested:['leaf']}:{}),...(partial?{nested:['missing']}:{})};
 const reads:string[]=[];
 await page.route('**/jobs/*/tree-node',r=>{const id=new URL(r.request().url()).pathname.split('/').at(-2)!;reads.push('node:'+id);const node=records.find(n=>n.job_id===id);return r.fulfill(node?{json:{node,observed_at:waitAt}}:{status:404,json:{error:'Unavailable'}});});
 await page.route('**/jobs/*/tree-children?*',r=>{const id=new URL(r.request().url()).pathname.split('/').at(-2)!;reads.push('children:'+id);return r.fulfill({json:{nodes:records.filter(n=>n.parent_job_id===id),observed_at:waitAt}});});
 await page.route('**/jobs/*/tree-waits?*',r=>{const id=new URL(r.request().url()).pathname.split('/').at(-2)!;reads.push('waits:'+id);return r.fulfill({json:waitsPage(id,(links[id]||[]).map(id=>waitJob(id)))});});
 return {records,links,reads};
}
for(const width of [1280,390,320])for(const route of ['tree','story?tab=dag']){
 test(`DB01–05 recursive mixed waits, full details, local routes and keyboard ${width} ${route}`,async({page})=>{
  await page.setViewportSize({width,height:1000});const f=await recursiveFixture(page,{cycle:true});
  await page.goto('/project/demo/jobs/entry/'+route);
  await expect(page.locator('.tree-card')).toHaveCount(5);await expect(page.getByText('Discovering…',{exact:true})).toHaveCount(0);
  expect(f.reads).not.toContain('node:outside');expect(f.reads.filter(s=>s==='waits:dependency')).toHaveLength(1);
  await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('cycle boundaries');
  const target=page.locator('[data-job-id="dependency"] .tree-card-select');await expect(target).toContainText('Prepare the shared dependency');await expect(target).toContainText('platform');await expect(target).toContainText('Wait target');await expect(target).toContainText('UTC');
  const before=f.reads.length;await target.focus();await page.keyboard.press('Enter');await expect(target).toBeFocused();
  const pane=page.getByRole('complementary',{name:'Selected job details'});await expect(pane).toContainText('Full dependency prompt 雪 <script>literal</script>');await expect(pane).toContainText('Runtime parent');await expect(pane).toContainText('outside');await expect(pane).toContainText('Waiting job: entry');
  await expect(pane.getByRole('button',{name:'Cancel job',exact:true})).toBeAttached();await expect(pane.getByRole('link',{name:'Open execution',exact:true})).toHaveAttribute('href','/project/demo/jobs/dependency/story');
  expect(f.reads.length).toBe(before);await expect(page.locator('.tree-card')).toHaveCount(5);
  await capture(page,`recursive-details-${width}-${route.startsWith('story')?'embedded':'standalone'}`);
  await page.getByRole('button',{name:'Fit',exact:true}).click();await checkWaitGeometry(page,true);
  if(width<768){const graph=await page.getByLabel('Execution diagram').boundingBox(),detail=await pane.boundingBox();expect(detail!.y).toBeGreaterThanOrEqual(graph!.y+graph!.height);for(const el of await pane.locator('button,a').all()){const box=await el.boundingBox();if(box)expect(box.height).toBeGreaterThanOrEqual(44);}}
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  await capture(page,`recursive-${width}-${route.startsWith('story')?'embedded':'standalone'}`);
  await page.getByRole('button',{name:'Toggle branch Publish the reviewed change'}).click();await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('Review downstream leaf');await expect(page.locator('[data-job-id="leaf"]')).toBeAttached();expect(f.reads.length).toBe(before);
  await page.getByRole('button',{name:'Find opened job'}).click();await expect(page.locator('[data-job-id="entry"] .tree-card-select')).toBeFocused();
 });
}
test('DB01 51 independent wait targets are hydrated without the former overlay cap',async({page})=>{
 const f=await recursiveFixture(page,{dense:true});await page.goto('/project/demo/jobs/entry/tree');await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible({timeout:20000});await expect(page.locator('.tree-card')).toHaveCount(56);
 expect(f.reads.filter(s=>s.startsWith('node:target-'))).toHaveLength(51);await expect(page.locator('path[data-wait-source="entry"]')).toHaveCount(52);
});
test('DB03 ordinary dashed paths use the local column gap despite a tall unrelated branch',async({page})=>{
 await recursiveFixture(page,{dense:true});await page.goto('/project/demo/jobs/entry/tree');await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible({timeout:20000});
 const points=await page.locator('path[data-wait-source="dependency"]').first().evaluate((el:SVGPathElement)=>{const p=el.getPointAtLength(0),end=el.getPointAtLength(el.getTotalLength());const ys=Array.from({length:21},(_,i)=>el.getPointAtLength(el.getTotalLength()*i/20).y);return {p:{x:p.x,y:p.y},end:{x:end.x,y:end.y},minY:Math.min(...ys),maxY:Math.max(...ys),d:el.getAttribute('d')};});
 expect(points.end.x).toBeGreaterThan(points.p.x);expect(points.minY).toBeGreaterThanOrEqual(Math.min(points.p.y,points.end.y));expect(points.maxY).toBeLessThanOrEqual(Math.max(points.p.y,points.end.y));expect(points.d).toMatch(/^M /);await checkWaitGeometry(page);
});
test('DB06 failed lookup and failed refresh retain readable branches until verified replacement',async({page})=>{
 await page.setViewportSize({width:1280,height:1000});await recursiveFixture(page,{partial:true});await page.goto('/project/demo/jobs/entry/tree');await expect(page.getByText('Discovering…',{exact:true})).toHaveCount(0);await expect(page.getByText('Partial discovery',{exact:true})).toBeVisible();
 const missing=page.getByRole('button',{name:'Job wait target · missing',exact:true});await missing.click();const pane=page.getByRole('complementary',{name:'Selected job details'});await expect(pane).toContainText('Job summary unavailable');await expect(pane).toContainText('Waiting job: nested');await expect(pane.getByRole('button',{name:'Cancel job',exact:true})).toHaveCount(0);await capture(page,'recursive-missing-product');
 await page.route('**/jobs/missing/tree-node',r=>r.fulfill({json:{node:{...nodes[0],job_id:'missing',title:'Recovered dependency'},observed_at:waitAt}}));await pane.getByRole('button',{name:'Retry summary'}).click();await expect(page.locator('[data-job-id="missing"]')).toContainText('Recovered dependency');
 await page.locator('[data-job-id="entry"] .tree-card-select').click();await page.route('**/jobs/entry/tree-waits?*',r=>r.fulfill({status:503,json:{error:'Unavailable'}}));await pane.getByRole('button',{name:'Refresh waits'}).click();await expect(pane).toContainText('Previous observation');await expect(page.locator('[data-job-id="dependency"]')).toBeAttached();await capture(page,'recursive-previous-product');
 await page.route('**/jobs/entry/tree-waits?*',r=>r.fulfill({json:waitsPage('entry')}));await pane.getByRole('button',{name:'Retry waits'}).click();await expect(page.locator('.tree-card')).toHaveCount(1);
});
test('DB07 exact wait job DAG and execution return preserve the original destination',async({page})=>{
 await recursiveFixture(page);await page.goto('/project/demo/jobs/entry/tree');await page.locator('[data-job-id="dependency"] .tree-card-select').click();const pane=page.getByRole('complementary',{name:'Selected job details'});await pane.locator('a[href="/project/demo/jobs/dependency/tree"]').click();await expect(page).toHaveURL('/project/demo/jobs/dependency/tree');await expect(page.getByRole('link',{name:'Back to waiting job DAG'})).toHaveAttribute('href','/project/demo/jobs/entry/story?tab=dag');
});
test('DB08 embedded reads start on activation, pause on departure, resume once and reject obsolete pages',async({page})=>{
 const f=await recursiveFixture(page);let release!:()=>void,reads=0;const gate=new Promise<void>(r=>release=r);
 await page.route('**/jobs/entry/tree-waits?*',async r=>{const current=++reads;if(current===1)await gate;await r.fulfill({json:waitsPage('entry',current===1?[waitJob('obsolete')]:[waitJob('dependency')])}).catch(()=>{});});
 await page.goto('/project/demo/jobs/entry/story');expect(f.reads).toEqual([]);await page.getByRole('tab',{name:'DAG',exact:true}).click();await expect.poll(()=>reads).toBe(1);await page.getByRole('tab',{name:'Trace',exact:true}).click();await page.getByRole('tab',{name:'DAG',exact:true}).click();await expect(page.locator('[data-job-id="leaf"]')).toBeAttached();release();await expect(page.getByText('Discovering…',{exact:true})).toHaveCount(0);expect(reads).toBe(2);await expect(page.getByRole('button',{name:'Job wait target · obsolete',exact:true})).toHaveCount(0);const count=f.reads.length;await page.clock.runFor(60000);expect(f.reads.length).toBe(count);
});
async function checkWaitGeometry(page:Page,fitted=false){
 const intersections=await page.evaluate(fitted=>{
  const cards=[...document.querySelectorAll<HTMLElement>('.tree-card,.tree-wait-reference')].map(e=>e.getBoundingClientRect());const errors:string[]=[];
  for(const path of document.querySelectorAll<SVGPathElement>('path[data-wait-target]')){const matrix=path.getScreenCTM()!;for(let d=0;d<=path.getTotalLength();d+=3){const p=path.getPointAtLength(d).matrixTransform(matrix);if(cards.some(b=>p.x>b.left+2&&p.x<b.right-2&&p.y>b.top+2&&p.y<b.bottom-2)){errors.push(path.dataset.waitTarget!);break;}}}
  const canvas=document.querySelector('.tree-canvas')!.getBoundingClientRect(),viewport=document.querySelector('.tree-viewport')!.getBoundingClientRect();
  for(const el of document.querySelectorAll<SVGGraphicsElement>('.tree-canvas path,.tree-wait-connections text')){
   const b=el.getBoundingClientRect();
   if(b.left<canvas.left-1||b.right>canvas.right+1||b.top<canvas.top-1||b.bottom>canvas.bottom+1)errors.push('Clipped route or label: '+el.textContent);
   if(fitted&&(b.left<viewport.left-1||b.right>viewport.right+1))errors.push('Outside fitted width: '+el.textContent);
  }
  return errors;
 },fitted);expect(intersections).toEqual([]);
}
