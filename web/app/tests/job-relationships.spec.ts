import { test, expect, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import path from 'node:path';
import { jobIdentity, mockCells } from './helpers/cells';

const current = { ...jobIdentity, tenant_id: 'demo', job_id: 'evidence-105', goal_id: 'goal-evidence', title: 'Prepare release evidence',
  prompt: 'Prepare release evidence for the navigation update.', status: 'READY', store: 'ACTIVE', job_type: 'recipe', recipe: 'build',
  created_at: '2026-11-07T00:00:00Z', parent: { job_id: 'release-104', goal_id: 'goal-release', tenant_id: 'other' } };
const children = [
  { ...current, parent: undefined, job_id: 'check-api-106', goal_id: 'goal-api', title: 'Check API compatibility', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success' },
  { ...current, parent: undefined, job_id: 'check-ui-107', goal_id: 'goal-ui', title: 'Verify review navigation' },
  { ...current, parent: undefined, job_id: 'archive-108', goal_id: undefined, title: 'Archive test evidence', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success' },
];
const story = (id: string) => ({ ...jobIdentity, job_id: id, status: 'completed', root: { id: 'root', kind: 'recipe', title: 'Release evidence', status: 'completed', path: [], attempt: 1,
  children: [{ id: 'checks', kind: 'op', title: 'Compatibility checks', status: 'completed', path: ['checks'], attempt: 1 }] } });
let inspections: string[];
let childRequests: number;
let submissions: unknown[];
let failChildren = false;
let long = false;
async function fixtures(page: Page) {
  inspections = []; childRequests = 0; submissions = []; failChildren = false; long = false;
  if(process.env.CORTEX_DEBUG_BROWSER){page.on('request',r=>console.log('REQ',r.url()));page.on('response',r=>console.log('RES',r.status(),r.url()));page.on('pageerror',e=>console.log('PAGEERROR',e.message));page.on('console',m=>console.log('CONSOLE',m.text()));}
  await page.context().addCookies([{name:'colony2:user:email',value:'review@example.com',url:'http://127.0.0.1:15175'}]);
  await page.addInitScript(() => {
    (window as any).EventSource = class { readyState=1; close(){} addEventListener(){} removeEventListener(){} };
  });
  await mockCells(page,['platform']);
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if(process.env.CORTEX_DEBUG_BROWSER) console.log('HANDLER',url.pathname);
    if (url.pathname.endsWith('/cells')) return route.fulfill({json:{cells:[]}});
    if (url.pathname === '/api/projects') return route.fulfill({json:[{id:'demo',name:'Demo',tenant_id:'demo'}]});
    if (url.pathname.endsWith('/user-inputs/pending')) return route.fulfill({json:[]});
    if (url.pathname.endsWith('/respond')) {const body = route.request().postData() || ''; const match = body.match(/name="submission"\r\n\r\n([^\r]+)\r\n/); if (!match) throw new Error('Missing multipart submission'); submissions.push(JSON.parse(match[1])); return route.fulfill({json:{ok:true}});}
    if (url.pathname.endsWith('/documents')) return route.fulfill({body:'# Release evidence\nAPI checks passed. Navigation evidence is ready.',contentType:'text/markdown'});
    if (url.pathname.includes('/user-inputs/evidence-105')) return route.fulfill({json:{jobId:'evidence-105',status:'pending',startTime:'2026-11-07T00:00:00Z',form:{kind:'review',request_id:'review-evidence',title:'Review release evidence',fields:[
      {id:'decision',type:'multiple_choice',question:'Review release evidence and approve or request changes.',required:true,options:[{value:'approve',label:'Approve'},{value:'request_changes',label:'Request changes'}]},
      {id:'feedback',type:'paragraph_text',question:'What should change?',required:false},
    ],documents:{'release-evidence.md':{name:'release-evidence.md',stored:{key:{name:'release-evidence.md',sizeBytes:80}}}}}}});
    const match=url.pathname.match(/\/jobs\/([^/]+)(?:\/(.*))?$/);
    if (match) {
      const id=decodeURIComponent(match[1]), suffix=match[2];
      if(suffix==='children') { childRequests++; return failChildren ? route.fulfill({status:503,json:{error:'offline'}}) : route.fulfill({json:{jobs:id==='evidence-105' ? children.map(child=>long ? {...child,job_id:child.job_id+'x'.repeat(180),title:child.title+'W'.repeat(180),goal_id:child.goal_id?.concat('g'.repeat(180))}:child) : []}}); }
      if(suffix==='story') return id==='release-104' ? route.fulfill({status:404,json:{error:'Job not found'}}): route.fulfill({json:story(id)});
      if(!suffix) {
        inspections.push(id);
        if(id==='release-104') return route.fulfill({status:404,json:{error:'Job not found'}});
        return route.fulfill({json:id==='evidence-105'?current:children.find(child=>child.job_id===id)||current});
      }
    }
    if(url.pathname.endsWith('/jobs')) return route.fulfill({json:{jobs:children.filter(child=>child.goal_id===url.searchParams.get('goal'))}});
    return route.fulfill({json:{}});
  });
}
async function capture(page: Page, name: string) {
  if (!process.env.CORTEX_REVIEW_ARTIFACTS) return;
  const directory=process.env.CORTEX_REVIEW_ARTIFACTS;
  mkdirSync(directory,{recursive:true});
  await page.screenshot({path:path.join(directory,name),fullPage:true});
}
const related=(page:Page)=>page.getByRole('region',{name:'Related jobs and goals'});
const disclosure=(page:Page)=>related(page).locator('details');
const summary=(page:Page)=>related(page).locator('summary');
async function initiallyClosed(page:Page) {
  await expect(summary(page)).toBeVisible();
  await expect(disclosure(page)).not.toHaveAttribute('open');
  await expect(related(page).getByRole('link',{name:'Job · release-104',exact:true})).toBeHidden();
  await expect(related(page).getByRole('button',{name:'Refresh relationships',exact:true})).toBeHidden();
  await expect.poll(()=>childRequests).toBeGreaterThan(0);
  await summary(page).focus();
  await page.keyboard.press('Tab');
  expect(await related(page).evaluate(e=>e.contains(document.activeElement))).toBe(false);
}
async function openFresh(page:Page) {
  await expect(summary(page)).toBeVisible();
  await expect(disclosure(page)).not.toHaveAttribute('open');
  await summary(page).click();
}


test.beforeEach(async({page})=>fixtures(page));

test('REL-001,010,011,012,030,033,038 story links navigate to correct identities and preserve story inspection',async({page})=>{
  await page.setViewportSize({width:1440,height:1000});
  await page.goto('/project/demo/jobs/evidence-105/story');
  await initiallyClosed(page);
  await capture(page,'story-desktop-collapsed.png');
  await summary(page).click();
  await expect(related(page).getByRole('link',{name:'Job · release-104',exact:true})).toHaveAttribute('href','/project/other/jobs/release-104/story');
  await expect(related(page).getByRole('link',{name:'Goal · goal-release',exact:true})).toHaveAttribute('href','/project/other/goals/goal-release');
  await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
  expect(inspections).toEqual(['evidence-105', 'evidence-105']); // Existing development StrictMode replay.
  await expect(page.getByText('Run Summary',{exact:true})).toBeVisible();
  expect(await related(page).evaluate(e=>e.getBoundingClientRect().top)).toBeLessThan(await page.getByText('Run Summary',{exact:true}).evaluate(e=>e.getBoundingClientRect().top));
  await page.getByText('Compatibility checks',{exact:true}).first().click();
  expect(inspections).toEqual(['evidence-105', 'evidence-105']); // Existing development StrictMode replay.
  await capture(page,'story-desktop-expanded.png');
  await related(page).getByRole('link',{name:'Job · Check API compatibility'}).click();
  await expect(page).toHaveURL(/\/jobs\/check-api-106\/story/);
  await expect(page.getByRole('heading',{name:'Job Story: Check API compatibility'})).toBeVisible();
  await expect(page.getByRole('link',{name:'Goal overview',exact:true})).toHaveAttribute('href','/project/demo/goals/goal-api');
  await openFresh(page);
  await expect(related(page).getByText('No parent recorded')).toBeVisible();
  await page.goBack();
  await openFresh(page);
  await related(page).getByRole('link',{name:'Goal · goal-api',exact:true}).click();
  await expect(page).toHaveURL(/\/goals\/goal-api/);
  await expect(page.getByText('Check API compatibility',{exact:true}).first()).toBeVisible();
  await page.goBack();
  await openFresh(page);
  await related(page).getByRole('link',{name:'Job · release-104',exact:true}).click();
  await expect(page).toHaveURL(/\/project\/other\/jobs\/release-104\/story/);
  await expect(page.getByText(/Job not found/).first()).toBeVisible();
});

test('REL-001,030,031,032,033,038 review context and document remain usable through child failure',async({page})=>{
  await page.setViewportSize({width:1440,height:1000});
  await page.goto('/project/demo/reviews/evidence-105');
  await initiallyClosed(page);
  await capture(page,'review-desktop-collapsed.png');
  await summary(page).click();
  await expect(related(page).getByRole('link',{name:'Job · release-104',exact:true})).toBeVisible();
  await expect(related(page).getByRole('link',{name:'Goal · goal-api',exact:true})).toBeVisible();
  await expect(page.getByRole('link',{name:'Open execution',exact:true})).toBeVisible();
  await expect(page.getByRole('region',{name:'Goal prompt'})).toHaveText(/Prepare release evidence/);
  expect(inspections).toEqual(['evidence-105', 'evidence-105']); // Existing development StrictMode replay.
  expect(await related(page).evaluate(e=>!!(e.previousElementSibling?.classList.contains('review-goal-context')))).toBe(true);
  const document=page.getByText('API checks passed. Navigation evidence is ready.',{exact:true}).first();
  await expect(document).toBeVisible();
  await capture(page,'review-desktop-expanded.png');
  failChildren=true;
  await related(page).getByRole('button',{name:'Refresh relationships',exact:true}).click();
  await expect(related(page).getByText('Child jobs unavailable')).toBeVisible();
  await expect(document).toBeVisible();
  await page.getByRole('button',{name:'Approve',exact:true}).click();
  expect(submissions).toHaveLength(0);
  await page.getByLabel('What should change?',{exact:true}).fill('Evidence reviewed');
  await expect(disclosure(page)).toHaveAttribute('open');
  await summary(page).click();
  await expect(disclosure(page)).not.toHaveAttribute('open');
  await page.getByRole('button',{name:'OK',exact:true}).click();
  await expect.poll(()=>submissions.length).toBe(1);
  expect(submissions[0]).toMatchObject({request_id:'review-evidence',fields:{decision:'approve',feedback:'Evidence reviewed'}});
});

for(const width of [1440,390,320]) test(`REL-034/D3,035,036,040,041,042 review ordering keyboard and wrapping ${width}`,async({page})=>{
  long=width!==390;
  await page.setViewportSize({width,height:1000});
  await page.goto('/project/demo/reviews/evidence-105');
  await initiallyClosed(page);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  await summary(page).focus();
  expect(await summary(page).evaluate(e=>getComputedStyle(e).outlineStyle)).toBe('solid');
  if(width===390) await capture(page,'review-mobile-collapsed.png');
  const requests=childRequests;
  await page.keyboard.press('Enter');
  await expect(disclosure(page)).toHaveAttribute('open');
  await page.keyboard.press('Space');
  await expect(disclosure(page)).not.toHaveAttribute('open');
  await expect(summary(page)).toBeFocused();
  await summary(page).click();
  expect(childRequests).toBe(requests);
  if(width===390) {
    long=true;
    await related(page).getByRole('button',{name:'Refresh relationships',exact:true}).click();
    await expect(related(page).getByRole('link',{name:/Job · Check API compatibilityW/})).toBeVisible();
  }
  const link=related(page).getByRole('link',{name:'Job · release-104',exact:true});
  await expect(link).toBeVisible();
  await link.focus(); await page.keyboard.press('Tab');
  const goal=related(page).getByRole('link',{name:'Goal · goal-release',exact:true});
  await expect(goal).toBeFocused();
  expect(await goal.evaluate(e=>getComputedStyle(e).outlineStyle)).toBe('solid');
  await expect(related(page).getByText('No goal recorded')).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  // D2 preserves context, documents, then response in both DOM and visual order.
  const context=page.locator('.review-goal-context');
  const documents=page.getByText('Documents (1)',{exact:true}).locator('xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " ant-card ")][1]');
  const response=page.getByText('Your response',{exact:true}).locator('xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " ant-card ")][1]');
  await expect(documents).toBeVisible();
  await expect(response).toBeVisible();
  expect(await related(page).evaluate(e=>e.closest('.review-goal-context')===null)).toBe(true);
  for (const expanded of [false,true]) {
    await summary(page).click();
    expect(await disclosure(page).evaluate(e=>(e as HTMLDetailsElement).open)).toBe(expanded);
    for (const [before,after] of [[context,related(page)],[related(page),documents],[documents,response]]) {
    const handle=await after.elementHandle();
    expect(await before.evaluate((e,target)=>Boolean(e.compareDocumentPosition(target!) & Node.DOCUMENT_POSITION_FOLLOWING),handle)).toBe(true);
    const beforeBox=await before.boundingBox(), afterBox=await after.boundingBox();
    expect(beforeBox).not.toBeNull(); expect(afterBox).not.toBeNull();
    expect(afterBox!.y).toBeGreaterThanOrEqual(beforeBox!.y+beforeBox!.height-1);
  }
  }
  if(width<768){
    expect(await related(page).evaluate(e=>e.closest('.review-secondary')!==null)).toBe(true);
    expect(await page.locator('.review-goal-context').evaluate(e=>(e as HTMLDetailsElement).open)).toBe(false);
    expect(await summary(page).evaluate(e=>e.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);
    await page.locator('.review-goal-context > summary').click();
    await expect(disclosure(page)).toHaveAttribute('open');
    await page.locator('.review-goal-context > summary').click();
    expect(await link.evaluate(e=>e.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);
    await expect(page.getByRole('button',{name:'Approve',exact:true})).toBeAttached();
  }
  if(width===390){
    long=false; await related(page).getByRole('button',{name:'Refresh relationships',exact:true}).click();
    await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
    await capture(page,'review-mobile-expanded.png');
    await summary(page).click();
  }
});

for(const width of [1440,390,320]) test(`REL-040,041,042 story default and reload ${width}`,async({page})=>{
  await page.setViewportSize({width,height:1000});
  await page.goto('/project/demo/jobs/evidence-105/story');
  await initiallyClosed(page);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  const requests=childRequests;
  await summary(page).focus();
  await page.keyboard.press('Space');
  await expect(disclosure(page)).toHaveAttribute('open');
  await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
  await page.getByText('Compatibility checks',{exact:true}).first().click();
  await expect(disclosure(page)).toHaveAttribute('open');
  await summary(page).focus(); await page.keyboard.press('Enter');
  await expect(disclosure(page)).not.toHaveAttribute('open');
  await expect(summary(page)).toBeFocused();
  expect(childRequests).toBe(requests);
  await page.reload();
  await initiallyClosed(page);
});
