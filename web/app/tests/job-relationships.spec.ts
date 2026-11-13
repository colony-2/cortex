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
async function selectStoryTab(page: Page, name: string) {
  if (!new URL(page.url()).pathname.endsWith('/story')) return;
  const tab = page.getByRole('tab', { name, exact: true });
  if (await tab.getAttribute('aria-selected') !== 'true') await tab.click();
}
async function initiallyVisible(page:Page) {
  await selectStoryTab(page, 'Relationships');
  await expect(related(page).getByRole('heading',{name:'Related jobs and goals'})).toBeVisible();
  await expect(disclosure(page)).toHaveCount(0);
  await expect(related(page).getByRole('button',{name:'Refresh relationships',exact:true})).toBeVisible();
  await expect.poll(()=>childRequests).toBeGreaterThan(0);
}
async function openFresh(page:Page) { await initiallyVisible(page); }


test.beforeEach(async({page})=>fixtures(page));

test('REL-001,010,011,012,030,033,038 story links navigate to correct identities and preserve story inspection',async({page})=>{
  await page.setViewportSize({width:1440,height:1000});
  await page.goto('/project/demo/jobs/evidence-105/story');
  await initiallyVisible(page);
  await capture(page,'story-desktop-direct.png');
  await expect(related(page).getByRole('link',{name:'Job · release-104',exact:true})).toHaveAttribute('href','/project/other/jobs/release-104/story');
  await expect(related(page).getByRole('link',{name:'Goal · goal-release',exact:true})).toHaveAttribute('href','/project/other/goals/goal-release');
  await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
  expect(inspections).toEqual(['evidence-105', 'evidence-105']); // Existing development StrictMode replay.
  await selectStoryTab(page, 'Trace');
  await expect(page.getByText('Run Summary',{exact:true})).toBeVisible();
  await expect(related(page)).toBeHidden();
  await selectStoryTab(page, 'Trace');
  await page.getByText('Compatibility checks',{exact:true}).first().click();
  await selectStoryTab(page, 'Relationships');
  expect(inspections).toEqual(['evidence-105', 'evidence-105']); // Existing development StrictMode replay.
  await capture(page,'story-desktop-restored.png');
  await related(page).getByRole('link',{name:'Job · Check API compatibility'}).click();
  await expect(page).toHaveURL(/\/jobs\/check-api-106\/story/);
  await expect(page.getByRole('heading',{name:'Job Story: Check API compatibility'})).toBeVisible();
  await expect(page.getByRole('link',{name:'Goal overview',exact:true})).toHaveAttribute('href','/project/demo/goals/goal-api');
  await openFresh(page);
  await expect(related(page).getByText('No parent recorded')).toBeVisible();
  await page.goBack(); // Child's Conversation, before selecting its Relationships tab.
  await page.goBack(); // Return to the parent job.
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
  await initiallyVisible(page);
  await capture(page,'review-desktop-direct.png');
  await expect(related(page).getByRole('link',{name:'Job · release-104',exact:true})).toBeVisible();
  await expect(related(page).getByRole('link',{name:'Goal · goal-api',exact:true})).toBeVisible();
  await expect(page.getByRole('link',{name:'Open execution',exact:true})).toBeVisible();
  await expect(page.getByRole('region',{name:'Goal prompt'})).toHaveText(/Prepare release evidence/);
  expect(inspections).toEqual(['evidence-105', 'evidence-105']); // Existing development StrictMode replay.
  expect(await related(page).evaluate(e=>!!(e.previousElementSibling?.classList.contains('review-goal-context')))).toBe(true);
  const document=page.getByText('API checks passed. Navigation evidence is ready.',{exact:true}).first();
  await expect(document).toBeVisible();
  await capture(page,'review-desktop-response.png');
  failChildren=true;
  await related(page).getByRole('button',{name:'Refresh relationships',exact:true}).click();
  await expect(related(page).getByText('Child jobs unavailable')).toBeVisible();
  await expect(document).toBeVisible();
  await page.getByRole('button',{name:'Approve',exact:true}).click();
  expect(submissions).toHaveLength(0);
  await page.getByLabel('What should change?',{exact:true}).fill('Evidence reviewed');
  await page.getByRole('button',{name:'OK',exact:true}).click();
  await expect.poll(()=>submissions.length).toBe(1);
  expect(submissions[0]).toMatchObject({request_id:'review-evidence',fields:{decision:'approve',feedback:'Evidence reviewed'}});
});

for(const width of [1440,390,320]) test(`REL-034/D3,035,036,040,041,042 review ordering keyboard and wrapping ${width}`,async({page})=>{
  long=width!==390;
  await page.setViewportSize({width,height:1000});
  await page.goto('/project/demo/reviews/evidence-105');
  await initiallyVisible(page);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  if(width===390) await capture(page,'review-mobile-direct.png');
  const requests=childRequests;
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
  const documents=page.getByRole('region',{name:'Selected document',exact:true});
  const response=page.getByText('Your response',{exact:true}).locator('xpath=ancestor::*[contains(concat(" ", normalize-space(@class), " "), " ant-card ")][1]');
  await expect(documents).toBeVisible();
  await expect(response).toBeVisible();
  expect(await related(page).evaluate(e=>e.closest('.review-goal-context')===null)).toBe(true);
  {
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
    await page.locator('.review-goal-context > summary').click();
    await page.locator('.review-goal-context > summary').click();
    expect(await link.evaluate(e=>e.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44);
    await expect(page.getByRole('button',{name:'Approve',exact:true})).toBeAttached();
  }
  if(width===390){
    long=false; await related(page).getByRole('button',{name:'Refresh relationships',exact:true}).click();
    await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
    await capture(page,'review-mobile-wrapped.png');
  }
});

for(const width of [1440,390,320]) test(`REL-040,041,042 story direct content and reload ${width}`,async({page})=>{
  await page.setViewportSize({width,height:1000});
  await page.goto('/project/demo/jobs/evidence-105/story');
  await initiallyVisible(page);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  const requests=childRequests;
  await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
  await selectStoryTab(page, 'Trace');
  await page.getByText('Compatibility checks',{exact:true}).first().click();
  await selectStoryTab(page, 'Relationships');
  expect(childRequests).toBe(requests);
  if(width===390) await capture(page,'story-mobile-direct.png');
  await page.reload();
  await initiallyVisible(page);
});

for(const width of [1280,390]) test(`JS-024,025,035,037 pages other attempts, retries, and preserves the chosen goal at ${width}px`, async ({page}, info) => {
  await page.setViewportSize({width,height:900});
  const cursors:(string|null)[]=[];
  let fail=true;
  await page.route('**/api/projects/demo/jobs?*',route=>{
    const url=new URL(route.request().url());
    if(!url.searchParams.has('goal'))return route.fallback();
    if(url.searchParams.get('goal')==='goal-api')return route.fulfill({json:{jobs:[]}});
    expect(url.searchParams.get('goal')).toBe('goal-evidence');
    expect(url.searchParams.getAll('status')).toEqual(['all']);
    expect(url.searchParams.get('includeHidden')).toBe('true');
    expect(url.searchParams.has('rootOnly')).toBe(false);
    const cursor=url.searchParams.get('pageToken');cursors.push(cursor);
    if(!cursor)return route.fulfill({json:{jobs:[current],next_page_token:'next'}});
    if(cursor==='next')return route.fulfill({json:{jobs:[{...children[0],goal_id:'goal-evidence',title:'Archived earlier attempt'}],next_page_token:'last'}});
    if(fail){fail=false;return route.fulfill({status:503,json:{error:'Sibling page unavailable'}});}
    return route.fulfill({json:{jobs:[{...children[0],goal_id:'goal-evidence',title:'Archived earlier attempt'},{...children[1],goal_id:'goal-evidence'}]}});
  });
  await page.goto('/project/demo/jobs/evidence-105/story?tab=relationships');
  const attempts=page.getByRole('region',{name:'Other attempts at this goal'});
  await attempts.getByRole('button',{name:'Load more attempts'}).click();
  await expect(attempts.getByRole('link',{name:'Archived earlier attempt'})).toBeVisible();
  await attempts.getByRole('button',{name:'Load more attempts'}).click();
  await expect(attempts.getByText('Other attempts unavailable')).toBeVisible();
  await attempts.getByRole('button',{name:'Retry other attempts'}).click();
  await expect(attempts.getByRole('listitem')).toHaveCount(2);
  expect(cursors.slice(-3)).toEqual(['next','last','last']);
  await page.screenshot({path:info.outputPath('sibling-attempts.png'),animations:'disabled',fullPage:true});
  await attempts.getByRole('link',{name:'Archived earlier attempt'}).click();
  await expect(page).toHaveURL(/\/jobs\/check-api-106\/story/);
  await expect(page.getByRole('link',{name:'Goal overview',exact:true})).toHaveAttribute('href','/project/demo/goals/goal-api');
  await page.getByRole('tab',{name:'Relationships',exact:true}).click();
  await expect(attempts.getByRole('link',{name:'Goal · goal-api',exact:true})).toBeVisible();
});
