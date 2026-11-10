import { test, expect } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { jobIdentity } from './helpers/cells';
const at='2026-11-09T00:00:00Z';
const nodes=[
 {tenant_id:'demo',job_id:'root',job_type:'recipe',cell:'cortex',status:'ACTIVE'},
 {tenant_id:'demo',job_id:'middle',parent_job_id:'root',job_type:'generic',cell:'jobdb',status:'COMPLETED'},
 {tenant_id:'demo',job_id:'entry',parent_job_id:'middle',job_type:'recipe',cell:'c2j',status:'READY',goal_id:'goal'},
 {tenant_id:'demo',job_id:'sibling',parent_job_id:'root',job_type:'recipe',cell:'root',status:'COMPLETED',completion_status:'success'},
];
let fail=false, missing=false, large=false, repeated=false;
test.beforeEach(async({page})=>{
 fail=false;missing=false;large=false;repeated=false;
 await page.context().addCookies([{name:'colony2:user:email',value:'test@example.com',url:'http://127.0.0.1:15176'}]);
 await page.addInitScript(()=>{(window as any).EventSource=class{close(){}addEventListener(){}removeEventListener(){}};});
 await page.route('**/api/**',async route=>{
  const u=new URL(route.request().url());
  if(u.pathname==='/api/projects')return route.fulfill({json:[{tenant_id:'demo'}]});
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
 await diagram.getByRole('button',{name:'sibling',exact:false}).click();
 const details=page.getByRole('complementary',{name:'Selected job details'});
 await expect(details).toContainText('success');await expect(page).toHaveURL(/entry\/tree$/);
 await page.getByRole('button',{name:'Diagram toggle root'}).click();
 await expect(diagram.getByRole('button',{name:'sibling',exact:false})).toHaveCount(0);
 await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('c2j');
 await expect(diagram.locator('path[data-child="entry"]')).toHaveCount(1);
 await page.getByRole('searchbox',{name:'Search loaded jobs'}).fill('');
 await page.getByRole('button',{name:'Find entry job'}).click();await expect(details).toContainText('Entry' .toLowerCase());
 await page.getByRole('button',{name:'Zoom in'}).click();await expect(page.getByLabel('Zoom',{exact:true})).toContainText('120%');
 await page.getByRole('button',{name:'Fit',exact:true}).click();
 await expect(details).toContainText('No children found in this traversal');
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText(at);
 if(process.env.CORTEX_TREE_SCREENSHOTS){mkdirSync(process.env.CORTEX_TREE_SCREENSHOTS,{recursive:true});await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/desktop.png',fullPage:true});}
 await page.getByRole('button',{name:'Refresh tree'}).click();await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
});
test('TREE-015,016 generic summary and keyboard mobile outline',async({page})=>{
 await page.setViewportSize({width:390,height:844});await page.goto('/project/demo/jobs/entry/tree');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 const outline=page.getByRole('region',{name:'Execution outline'});const middle=outline.getByRole('button',{name:'middle jobdb',exact:false});await middle.focus();await page.keyboard.press('Enter');
 const details=page.getByRole('complementary',{name:'Selected job details'});await expect(details).toContainText('Generic job summary');await expect(details.getByRole('link',{name:'Open story'})).toHaveCount(0);
 await expect(middle).toHaveAttribute('aria-pressed','true');await expect(page.getByLabel('Execution diagram')).toBeHidden();
 const entry=outline.getByRole('button',{name:'entry · Entry job',exact:false});await entry.click();await expect(details.getByRole('link',{name:'Open story'})).toHaveAttribute('href','/project/demo/jobs/entry/story');
 await expect(outline.getByRole('button',{name:'Toggle branch root'})).toHaveAttribute('aria-expanded','true');
 if(process.env.CORTEX_TREE_SCREENSHOTS){mkdirSync(process.env.CORTEX_TREE_SCREENSHOTS,{recursive:true});await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/mobile.png',fullPage:true});}
});
test('TREE-009,012 failure keeps entry and retry recovers',async({page})=>{
 fail=true;await page.goto('/project/demo/jobs/entry/tree');await expect(page.getByText('Partial discovery',{exact:true})).toBeVisible();
 const outline=page.getByRole('region',{name:'Execution outline'});await outline.getByRole('button',{name:'root cortex',exact:false}).click();
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
 await page.getByRole('complementary',{name:'Selected job details'}).getByRole('link',{name:'Open story'}).click();
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
 await panel.getByLabel('Execution diagram').getByRole('button',{name:'sibling',exact:false}).click();
 await expect(page).toHaveURL(/entry\/story\?tab=dag$/);
 const detail=panel.getByRole('complementary',{name:'Selected job details'});
 await expect(detail.getByRole('link',{name:'Open story'})).toHaveAttribute('href','/project/demo/jobs/sibling/story');
 await panel.getByRole('button',{name:'Zoom in'}).click();
 await panel.getByRole('button',{name:'Diagram toggle root'}).click();
 await panel.getByRole('searchbox').fill('c2j');
 const count=discoveries;
 await page.getByRole('tab',{name:'Summary',exact:true}).click();
 await page.goBack();
 await expect(page.getByRole('tab',{name:'DAG',exact:true})).toHaveAttribute('aria-selected','true');
 await expect(panel.getByLabel('Zoom',{exact:true})).toHaveText('120%');
 await expect(panel.getByRole('searchbox')).toHaveValue('c2j');
 await expect(detail).toContainText('sibling');
 await panel.getByRole('searchbox').fill('');
 await expect(panel.getByRole('button',{name:'Diagram toggle root'})).toHaveAttribute('aria-expanded','false');
 expect(discoveries).toBe(count);
 await panel.getByRole('button',{name:'Diagram toggle root'}).click();
 if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/embedded-desktop.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});
 await expect(panel.getByLabel('Execution diagram')).toBeHidden();
 await expect(panel.getByRole('region',{name:'Execution outline'})).toBeVisible();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
 if(process.env.CORTEX_TREE_SCREENSHOTS) await page.screenshot({path:process.env.CORTEX_TREE_SCREENSHOTS+'/embedded-mobile.png',fullPage:true});
});

test('DAG-06 interrupted entry aborts on tab departure and resumes', async ({page}) => {
 let release!:()=>void; const gate=new Promise<void>(r=>release=r); let calls=0; let resumed=false;
 await page.route('**/jobs/entry/tree-node',async route=>{calls++; if(!resumed) await gate; await route.fulfill({json:{node:nodes[2],observed_at:at}}).catch(()=>{});});
 await page.goto('/project/demo/jobs/entry/story?tab=dag');
 await expect.poll(()=>calls).toBeGreaterThan(0);
 const initialCalls=calls;
 await page.getByRole('tab',{name:'Summary',exact:true}).click();
 await page.getByRole('tab',{name:'DAG',exact:true}).click();
 await expect(page.getByRole('button',{name:'Resume discovery'})).toBeEnabled();
 resumed=true;
 await page.getByRole('button',{name:'Resume discovery'}).click();
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 release();
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
 expect(calls).toBe(initialCalls+1);
});

test('DAG-07 identity changes dispose loaded state; DAG-09,10 recipe returns target original entry',async({page})=>{
 await page.goto('/project/demo/jobs/entry/story?tab=dag');
 await expect(page.getByText('Discovery finished:',{exact:false})).toBeVisible();
 await page.getByLabel('Execution diagram').getByRole('button',{name:'root cortex',exact:false}).click();
 await page.getByRole('complementary',{name:'Selected job details'}).getByRole('link',{name:'Open story'}).click();
 await expect(page).toHaveURL(/root\/story$/);
 await page.getByRole('link',{name:'DAG',exact:true}).click();
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
 await expect(page.getByRole('complementary',{name:'Selected job details'}).locator('dd').nth(1)).toHaveText('root');
 await expect(page.getByLabel('Zoom',{exact:true})).toHaveText('100%');
 await page.getByRole('link',{name:'Return to DAG',exact:true}).click();
 await expect(page).toHaveURL(/entry\/story\?tab=dag$/);
 await expect(page.getByRole('region',{name:'Discovery coverage'})).toContainText('4 loaded jobs');
});

 test('DAG-17,18 Summary and Input artifacts show directly in the browser',async({page})=>{
  await page.goto('/project/demo/jobs/entry/story?tab=summary');
  const summary=page.getByRole('tabpanel',{name:'Summary',exact:true});
  await expect(summary.getByRole('heading',{name:'Run Summary'})).toBeVisible();
  await expect(summary.locator('details')).toHaveCount(0);
  await expect(summary.getByText('Execution timing and recipe metadata load when you open Steps.')).toBeVisible();
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
 await page.getByLabel('Execution diagram').getByRole('button',{name:'entry c2j',exact:false}).click();
 await page.getByRole('complementary',{name:'Selected job details'}).getByRole('link',{name:'Open story'}).click();
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
 await expect(page.getByLabel('Execution diagram').getByRole('button',{name:'sibling',exact:false})).toHaveCount(0);
});
