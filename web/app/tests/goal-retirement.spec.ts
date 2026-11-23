import { test, expect } from '@playwright/test';
import { jobIdentity, cellFixture } from './helpers/cells';

// The obsolete expanded goal detail no longer exists. Recorded identities remain
// in Relationships (covered separately); legacy URLs must explain retirement.
for (const width of [1440, 390]) test(`legacy goal detail explains retirement and returns to flat jobs at ${width}px`, async ({page,context,baseURL})=>{
  await page.setViewportSize({width,height:900});
  await context.addCookies([{name:'colony2:user:email',value:'review@example.com',url:baseURL!}]);
  const reads:string[]=[];
  await page.route('**/api/**',route=>{
    const path=new URL(route.request().url()).pathname;reads.push(path);
    return route.fulfill({json:path.endsWith('/projects')?[{tenant_id:'retirement'}]:path.endsWith('/cells')?{cells:[cellFixture('platform','retirement')]}:path.endsWith('/jobs')?{jobs:[{...jobIdentity,tenant_id:'retirement',job_id:'consumer-A',goal_id:'primary-H',other_goal_ids:['STALE-SECONDARY'],title:'Preserve the recorded result producer',status:'READY',store:'ACTIVE'}]}:path.endsWith('/pending')?[]:{}});
  });
  await page.goto('/project/retirement/goals/historical-G');
  await expect(page.getByRole('alert')).toContainText('Goal links have been retired');
  await expect(page.locator('.goal-overview-content')).toHaveCount(0);
  await page.getByRole('link',{name:'Open All jobs'}).click();
  await expect(page).toHaveURL('/project/retirement/jobs');
  await expect(page.getByRole('link',{name:'platform: Preserve the recorded result producer'})).toHaveAttribute('href','/project/retirement/jobs/consumer-A/story');
  await expect(page.getByText('STALE-SECONDARY')).toHaveCount(0);
  expect(reads.some(path=>path.includes('/goals/'))).toBe(false);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});
