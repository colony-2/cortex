import { test, expect } from '@playwright/test';
import { jobIdentity, mockCells } from './helpers/cells';

// These flows include real focus/tooltip transitions, reloads, and form submission.
test.setTimeout(90_000);
test.beforeEach(async ({ page }) => {
 await page.route('**/assistant/capabilities', route => route.fulfill({ json: { available: false } }));
});

for (const width of [1440, 390]) test(`PR-009,013,014 queue at ${width}px`, async ({ page, context, baseURL }, testInfo) => {
 await page.setViewportSize({width,height:900});
 await page.clock.setFixedTime(new Date('2026-11-10T12:00:00Z'));
 await context.addCookies([{name:'colony2:user:email',value:'review@example.com',url:baseURL!}]);
 await page.addInitScript(() => {
  (window as any).queueEvents = {};
  (window as any).EventSource = class {
   readyState=1; onopen:any; onerror:any;
   constructor(){ setTimeout(()=>this.onopen?.(),0); }
   close(){} removeEventListener(){}
   addEventListener(type:string, listener:any){ (window as any).queueEvents[type]=listener; }
  };
 });
 const row=(id:string,requested_at?:string)=>({...jobIdentity,id,job_id:id,tenant_id:'queue',title:`Review ${id}`,requested_at,request_id:`request-${id}`,task_ordinal:2});
 let rows=[row('recent','2026-11-10T11:59:30Z'),row('unknown'),row('oldest','2026-11-08T12:00:00Z'),row('middle','2026-11-10T10:00:00Z')];
 await page.route('**/api/projects',route=>route.fulfill({json:[{tenant_id:'queue'}]}));
 await mockCells(page,['platform']);
 await page.route('**/user-inputs/pending',route=>route.fulfill({json:rows}));
 await page.route('**/user-inputs/earliest', route=>route.fulfill({json:{jobId:'earliest',status:'pending',startTime:'2026-11-01T12:00:00Z',form:{question:'Confirm the earliest request?',request_id:'request-earliest',fields:[]}}}));
 await page.route('**/jobs/earliest',route=>route.fulfill({json:{...jobIdentity,job_id:'earliest',tenant_id:'queue',status:'READY',store:'ACTIVE',prompt:'Earliest request context'}}));
 await page.route('**/jobs/earliest/children',route=>route.fulfill({json:{jobs:[]}}));
 await page.route('**/jobs?*',route=>route.fulfill({json:{jobs:[]}}));
 await page.goto('/project/queue/reviews');
 const cards=page.locator('.review-page .ant-list .ant-card');
 await expect(cards).toHaveCount(4);
 await expect(cards.nth(0)).toContainText('Review oldest'); await expect(cards.nth(1)).toContainText('Review middle'); await expect(cards.nth(2)).toContainText('Review recent'); await expect(cards.nth(3)).toContainText('Request time unavailable');
 await expect(page.getByText('Showing 4 of 4 pending requests')).toBeVisible();
 await expect(cards.nth(0)).toContainText('Requested 2 days ago');
 await page.screenshot({path:testInfo.outputPath(`queue-${width}.png`),fullPage:true,animations:'disabled'});
 const age=cards.first().locator('time');
 await age.focus(); await page.keyboard.press('Tab'); await page.keyboard.press('Shift+Tab');
 await expect(age).toBeFocused(); await expect(page.getByRole('tooltip')).toBeVisible(); await expect(page.getByRole('tooltip')).toContainText('2026');
 await expect(page.locator('.ant-tooltip').filter({has:page.getByRole('tooltip')})).toHaveCSS('opacity','1');
 await expect(age).toHaveAttribute('datetime','2026-11-08T12:00:00Z'); await expect(age).toHaveAttribute('title',/2026/);
 await page.screenshot({path:testInfo.outputPath(`queue-exact-${width}.png`),fullPage:true,animations:'allow'});
 await age.blur(); await page.mouse.move(0,0);
 await expect(page.getByRole('tooltip')).toBeHidden();
 await age.hover(); await expect(page.getByRole('tooltip')).toContainText('UTC');
 await page.mouse.move(0,0); await expect(page.getByRole('tooltip')).toBeHidden();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
 // Manual refresh uses exactly the same ordering.
 rows=[row('new','2026-11-10T11:58:00Z'),...rows.slice(2)];
 await page.getByRole('button',{name:/Refresh/}).click();
 await expect(cards).toHaveCount(3); await expect(cards.first()).toContainText('Review oldest'); await expect(cards.last()).toContainText('Review new');
 // A replacement snapshot inserts and removes cards immediately.
 await page.evaluate((inputs)=>{ (window as any).queueEvents.inputs_changed({data:JSON.stringify({inputs})}); },[row('future','2026-11-11T12:00:00Z'),row('earliest','2026-11-01T12:00:00Z')]);
 await expect(cards).toHaveCount(2); await expect(cards.first()).toContainText('Review earliest'); await expect(cards.last()).toContainText('Requested just now');
 await cards.first().getByRole('button',{name:'Open Review'}).click(); await expect(page).toHaveURL(/\/project\/queue\/reviews\/earliest$/);
 await expect(page.getByRole('textbox',{name:'Confirm the earliest request?'})).toBeVisible();
});

for (const width of [1440, 390]) test(`Reviews controls and return navigation at ${width}px`, async ({ page, context, baseURL }, testInfo) => {
 await page.setViewportSize({ width, height: 1000 });
 await page.clock.setFixedTime(new Date('2026-11-10T12:00:00Z'));
 await context.addCookies([{ name: 'colony2:user:email', value: 'review@example.com', url: baseURL! }]);
 await page.addInitScript(() => {
  (window as any).queueEvents = {};
  (window as any).EventSource = class {
   onopen:any; readyState=1;
   constructor(){ setTimeout(()=>this.onopen?.(),0); }
   close(){} removeEventListener(){}
   addEventListener(type:string, listener:any){ (window as any).queueEvents[type]=listener; }
  };
 });
 const reads: string[] = [];
 page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) reads.push(new URL(request.url()).pathname); });
 const make=(id:string,title:string,cell:string,time?:string,prompt?:string)=>({...jobIdentity,id,job_id:id,tenant_id:'controls',title,prompt,cell_id:`cell-${cell}`,cell_name:cell,requested_at:time,task_ordinal:2});
 let rows=[make('alpha','Alpha food at the bar','Zeta','2026-11-08T12:00:00Z'), make('zeta','Zeta food at the bar','Alpha','2026-11-10T10:00:00Z'), make('unknown','Middle food at the bar','Alpha'), make('split','Food','Other',undefined,'bar')];
 await page.route('**/api/projects', route=>route.fulfill({json:[{tenant_id:'controls'}]}));
 await page.route('**/user-inputs/pending', route=>route.fulfill({json:rows}));
 await page.route('**/cells**', route=>route.abort());
 await page.route('**/user-inputs/alpha', route=>route.request().method()==='POST' ? route.fulfill({json:{}}) : route.fulfill({json:{jobId:'alpha',status:'pending',startTime:'2026-11-08T12:00:00Z',task_ordinal:2,form:{question:'Confirm food?',type:'short_answer'}}}));
 await page.route('**/user-inputs/alpha/respond',route=>route.fulfill({json:{}}));
 await page.goto('/project/controls/reviews?unrelated=keep');
 const cards=page.locator('.review-page .ant-list .ant-card');
 await expect(cards).toHaveCount(4);
 expect(reads.filter(path=>/\/cells|\/jobs\/|\/user-inputs\/(?!pending)/.test(path))).toEqual([]);
 const baseline=reads.length;
 const controls=page.getByRole('button',{name:/Filters and sorting/});
 if(width<768){
  await expect(page.getByRole('textbox',{name:'Search reviews'})).toBeHidden();
  await controls.focus(); await page.keyboard.press('Enter');
 }
 const search=page.getByRole('textbox',{name:'Search reviews'});
 await search.fill('foo bar');
 await expect(cards).toHaveCount(3);
 await expect(page.getByText('Showing 3 of 4 pending requests')).toBeVisible();
 await expect(page.getByText('4 pending reviews', { exact: true })).toHaveCount(1);
 const sort=page.getByRole('combobox',{name:'Sort by'});
 for(const [value,ids] of [
  ['requested:ascend',['alpha','zeta','unknown']],['requested:descend',['zeta','alpha','unknown']],
  ['title:ascend',['alpha','unknown','zeta']],['title:descend',['zeta','unknown','alpha']],
  ['cell:ascend',['zeta','unknown','alpha']],['cell:descend',['alpha','zeta','unknown']],
 ] as const){
  await sort.selectOption(value);
  for(let index=0;index<ids.length;index++) await expect(cards.nth(index)).toContainText(`Job ID: ${ids[index]}`);
 }
 await sort.selectOption('requested:ascend');
 await page.screenshot({path:testInfo.outputPath(`reviews-controls-${width}.png`),fullPage:true,animations:'disabled'});
 const picker=page.getByRole('button',{name:'Cells: All',exact:true});
 await picker.focus(); await page.keyboard.press('Enter');
 let dialog=page.getByRole('dialog');
 // Choices include the row excluded by search.
 await expect(dialog.getByRole('checkbox',{name:'Other',exact:true})).toBeChecked();
 await dialog.getByRole('button',{name:'Select none'}).click();
 await page.keyboard.press('Escape');
 await expect(dialog).toBeHidden(); await expect(picker).toBeFocused();
 await expect(cards).toHaveCount(3);
 await picker.click(); dialog=page.getByRole('dialog');
 await dialog.getByRole('button',{name:'Select none'}).click();
 await dialog.getByRole('checkbox',{name:'Zeta',exact:true}).check();
 await page.screenshot({path:testInfo.outputPath(`reviews-picker-${width}.png`),fullPage:false,animations:'disabled'});
 await dialog.getByRole('button',{name:'Apply'}).click();
 await expect(cards).toHaveCount(1);
 expect(reads).toHaveLength(baseline);
 const listURL=page.url();
 await page.reload(); await expect(cards).toHaveCount(1);
 if(width<768){
  await expect(controls).toHaveText('Filters and sorting (active)');
  await page.screenshot({path:testInfo.outputPath(`reviews-collapsed-${width}.png`),fullPage:true,animations:'disabled'});
  await controls.click();
 }
 await expect(search).toHaveValue('foo bar');
 await cards.first().getByRole('button',{name:'Open Review'}).click();
 await expect(page).toHaveURL(/\/reviews\/alpha\?q=foo\+bar&cellId=cell-Zeta$/);
 expect(page.url()).not.toContain('unrelated');
 await page.reload();
 await page.getByRole('button',{name:'Back to Reviews'}).click();
 await expect(page).toHaveURL(/\/reviews\?q=foo\+bar&cellId=cell-Zeta$/);
 await expect(cards).toHaveCount(1);
 await cards.first().getByRole('button',{name:'Open Review'}).click();
 await page.getByRole('textbox',{name:'Confirm food?'}).fill('yes');
 await page.getByRole('button',{name:'Submit',exact:true}).click();
 await expect(page).toHaveURL(/\/reviews\?q=foo\+bar&cellId=cell-Zeta$/);
 await expect(cards).toHaveCount(1);
 if(width<768) await controls.click();
 // SSE replacement retains a disappeared selection without borrowing its timing.
 const beforeLive=reads.length;
 await page.evaluate(inputs=>(window as any).queueEvents.inputs_changed({data:JSON.stringify({inputs})}),[make('replacement','Food bar','New')]);
 await expect(page.getByText('No requests match these filters')).toBeVisible();
 await page.getByRole('button',{name:'Cells: 1 selected'}).click(); dialog=page.getByRole('dialog');
 await expect(dialog.getByRole('checkbox',{name:'Selected cell (cell-Zeta)'})).toBeChecked();
 await expect(dialog.getByRole('checkbox',{name:'New',exact:true})).not.toBeChecked();
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click();
 expect(reads).toHaveLength(beforeLive);
 await expect(page.locator('.ant-message-notice')).toHaveCount(0);
 await page.screenshot({path:testInfo.outputPath(`reviews-empty-${width}.png`),fullPage:true,animations:'disabled'});
 await page.getByRole('button',{name:'Reset',exact:true}).first().click();
 await expect(cards).toHaveCount(1); await expect(cards.first()).toContainText('Request time unavailable');
 await expect(page).toHaveURL(/\/reviews$/);
 await page.goBack(); await expect(page.getByText('No requests match these filters')).toBeVisible();
 await page.goForward(); await expect(cards).toHaveCount(1);
 // Existing Refresh reuses the active view.
 await search.fill('foo bar'); await sort.selectOption('title:descend');
 rows=[make('refreshed','Food bar refresh','New'),make('other','Unrelated','Other')];
 await page.getByRole('button',{name:/Refresh/}).click();
 await expect(cards).toHaveCount(1); await expect(cards.first()).toContainText('Job ID: refreshed');
 await expect(sort).toHaveValue('title:descend'); await expect(search).toHaveValue('foo bar');
 expect(reads.some(path=>path.includes('/cells'))).toBe(false);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
 // A tenant change starts with its own default view and hides old rows.
 await page.route('**/api/projects/other/jobs?*', route => route.fulfill({ json: { jobs: [] } }));
 await mockCells(page, ['platform']);
 await page.getByRole('searchbox',{name:'Tenant ID',exact:true}).fill('other');
 await page.getByRole('button',{name:'Use',exact:true}).click();
 await expect(page).toHaveURL(/\/project\/other\/jobs$/);
 if (width < 768) await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
 await page.getByRole('link', { name: 'Reviews', exact: true }).click();
 await expect(page).toHaveURL(/\/project\/other\/reviews$/);
 await expect(cards).toHaveCount(2);
 if (width < 768) await controls.click();
 await expect(search).toHaveValue('');
 await expect(sort).toHaveValue('requested:ascend');
 await expect(page.getByRole('button', { name: 'Cells: All', exact: true })).toBeVisible();
 expect(listURL).toContain('unrelated=keep');
});
