import { test, expect } from '@playwright/test';
import { jobIdentity, mockCells } from './helpers/cells';

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
 await expect(page.getByText('Oldest first')).toBeVisible();
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
