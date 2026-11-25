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
const reviewInput = {jobId:'evidence-105',status:'pending',startTime:'2026-11-07T00:00:00Z',form:{kind:'review',request_id:'review-evidence',title:'Review release evidence',fields:[
      {id:'decision',type:'multiple_choice',question:'Review release evidence and approve or request changes.',required:true,options:[{value:'approve',label:'Approve'},{value:'request_changes',label:'Request changes'}]},
      {id:'feedback',type:'paragraph_text',question:'What should change?',required:false},
    ],documents:{'release-evidence.md':{kind:'stored',name:'release-evidence.md',stored:{key:{jobId:'release-104',taskOrdinal:3,name:'release-evidence.md',sizeBytes:80}}}}}};
let inspections: string[];
let childRequests: number;
let submissions: unknown[];
let failChildren = false;
let long = false;
let pendingReview = false;
async function fixtures(page: Page, baseURL: string) {
  inspections = []; childRequests = 0; submissions = []; failChildren = false; long = false; pendingReview = false;
  if(process.env.CORTEX_DEBUG_BROWSER){page.on('request',r=>console.log('REQ',r.url()));page.on('response',r=>console.log('RES',r.status(),r.url()));page.on('pageerror',e=>console.log('PAGEERROR',e.message));page.on('console',m=>console.log('CONSOLE',m.text()));}
  await page.context().addCookies([{name:'colony2:user:email',value:'review@example.com',url:baseURL}]);
  await page.addInitScript(() => {
    (window as any).EventSource = class { readyState=1; close(){} addEventListener(){} removeEventListener(){} };
  });
  await mockCells(page,['platform']);
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if(process.env.CORTEX_DEBUG_BROWSER) console.log('HANDLER',url.pathname);
    if (url.pathname.endsWith('/cells')) return route.fulfill({json:{cells:[]}});
    if (url.pathname === '/api/projects') return route.fulfill({json:[{id:'demo',name:'Demo',tenant_id:'demo'}]});
    if (url.pathname.endsWith('/user-inputs/pending')) return route.fulfill({json:pendingReview ? [{...jobIdentity,id:'evidence-105',client_payload_revision:1,request_id:'review-evidence'}] : []});
    if (url.pathname.endsWith('/respond')) {const body = route.request().postData() || ''; const match = body.match(/name="submission"\r\n\r\n([^\r]+)\r\n/); if (!match) throw new Error('Missing multipart submission'); submissions.push(JSON.parse(match[1])); return route.fulfill({json:{ok:true}});}
    if (url.pathname === '/api/projects/demo/jobs/release-104/stored-document') {
      expect(url.searchParams.get('ordinal')).toBe('3');
      expect(url.searchParams.get('name')).toBe('release-evidence.md');
      return route.fulfill({body:'# Release evidence\nAPI checks passed. Navigation evidence is ready.',contentType:'text/markdown'});
    }
    if (url.pathname.includes('/user-inputs/evidence-105')) return route.fulfill({json:reviewInput});
    const match=url.pathname.match(/\/jobs\/([^/]+)(?:\/(.*))?$/);
    if (match) {
      const id=decodeURIComponent(match[1]), suffix=match[2];
      if(id==='release-104' && (suffix==='conversation' || suffix==='story' || !suffix)) return route.fulfill({status:404,json:{error:'Job not found'}});
      if(suffix==='conversation') return route.fulfill({json:{
        tenant_id:'demo',job_id:id,entries:[],snapshot_token:'snapshot',snapshot_revision:1,completeness:'complete',
        activity:{availability:'available',value:id===current.job_id?current:children.find(child=>child.job_id===id)},
        input_artifacts:[],current_input:pendingReview && id===current.job_id ? reviewInput : null,
      }});
      if(suffix==='children') { childRequests++; return failChildren ? route.fulfill({status:503,json:{error:'offline'}}) : route.fulfill({json:{jobs:id==='evidence-105' ? children.map(child=>long ? {...child,job_id:child.job_id+'x'.repeat(180),title:child.title+'W'.repeat(180),goal_id:child.goal_id?.concat('g'.repeat(180))}:child) : []}}); }
      if(suffix==='story') return route.fulfill({json:story(id)});
      if(!suffix) {
        inspections.push(id);
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
const related=(page:Page)=>page.getByRole('region',{name:'Related jobs'});
const disclosure=(page:Page)=>related(page).locator('details');
async function selectStoryTab(page: Page, name: string) {
  if (!new URL(page.url()).pathname.endsWith('/story')) return;
  const tab = page.getByRole('tab', { name, exact: true });
  const dialog = page.getByRole('dialog', { name: 'Job relationships', exact: true });
  if (name !== 'Graph' && await dialog.isVisible()) {
    await dialog.getByRole('button', { name: 'Close', exact: true }).click();
    await expect(dialog).toBeHidden();
  }
  if (await tab.getAttribute('aria-selected') !== 'true') await tab.click();
  if (name === 'Graph' && !await dialog.isVisible()) await page.getByRole('button', { name: 'Related jobs' }).click();
}
async function initiallyVisible(page:Page) {
  await selectStoryTab(page, 'Graph');
  await expect(related(page).getByRole('heading',{name:'Related jobs'})).toBeVisible();
  await expect(disclosure(page)).toHaveCount(0);
  await expect(related(page).getByRole('button',{name:'Refresh relationships',exact:true})).toBeVisible();
  await expect.poll(()=>childRequests).toBeGreaterThan(0);
}
async function openFresh(page:Page) { await initiallyVisible(page); }


test.beforeEach(async({page,baseURL})=>fixtures(page,baseURL!));

test('REL-001,010,011,012,030,033,038 story links navigate to correct identities and preserve story inspection',async({page})=>{
  await page.setViewportSize({width:1440,height:1000});
  await page.goto('/project/demo/jobs/evidence-105/story');
  await initiallyVisible(page);
  await capture(page,'story-desktop-direct.png');
  await expect(related(page).getByRole('link',{name:'Job · release-104',exact:true})).toHaveAttribute('href','/project/other/jobs/release-104/story');
  await expect(related(page).locator('a[href*="/goals/"]')).toHaveCount(0);
  await expect(related(page).getByRole('link',{name:'Job · Check API compatibility'})).toBeVisible();
  expect(inspections).toEqual(['evidence-105']);
  await selectStoryTab(page, 'Trace');
  await expect(page.getByText('Run Summary',{exact:true})).toBeVisible();
  await expect(related(page)).toBeHidden();
  await selectStoryTab(page, 'Trace');
  await page.getByText('Compatibility checks',{exact:true}).first().click();
  await selectStoryTab(page, 'Graph');
  expect(inspections).toEqual(['evidence-105']);
  await capture(page,'story-desktop-restored.png');
  await related(page).getByRole('link',{name:'Job · Check API compatibility'}).click();
  await expect(page).toHaveURL(/\/jobs\/check-api-106\/story/);
  await expect(page.getByRole('heading',{name:'Check API compatibility'})).toBeVisible();
  await expect(page.getByRole('link',{name:'Back to inbox',exact:true})).toHaveAttribute('href','/project/demo/jobs');
  await openFresh(page);
  await expect(related(page).getByText('No parent recorded')).toBeVisible();
  await page.goBack(); // Child's Conversation, before selecting its Relationships tab.
  await page.goBack(); // Return to the parent job.
  await openFresh(page);
  await related(page).getByRole('link',{name:'Job · release-104',exact:true}).click();
  await expect(page).toHaveURL(/\/project\/other\/jobs\/release-104\/story/);
  await expect(page.getByText(/Job not found/).first()).toBeVisible();
});

test('REL-001,030,031,032,033,038 thread review remains usable through child failure', async ({ page }) => {
  pendingReview = true;
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto('/project/demo/reviews/evidence-105');
  await expect(page).toHaveURL(/\/jobs\/evidence-105\/story\?/);
  const document = page.getByText('API checks passed. Navigation evidence is ready.', { exact: true }).first();
  await expect(document).toBeVisible();
  await initiallyVisible(page);
  await expect(related(page).getByRole('link', { name: 'Job · release-104', exact: true })).toBeVisible();
  await expect(related(page).getByRole('link', { name: 'Job · Check API compatibility', exact: true })).toBeVisible();
  failChildren = true;
  await related(page).getByRole('button', { name: 'Refresh relationships' }).click();
  await expect(related(page).getByText('Child jobs unavailable')).toBeVisible();
  await selectStoryTab(page, 'Thread');
  await expect(document).toBeVisible();
  await capture(page, 'review-desktop-response.png');
  await page.getByRole('button', { name: 'Approve', exact: true }).click();
  expect(submissions).toHaveLength(0);
  await page.getByLabel('What should change?', { exact: true }).fill('Evidence reviewed');
  await page.getByRole('button', { name: 'OK', exact: true }).click();
  await expect.poll(() => submissions.length).toBe(1);
  expect(submissions[0]).toMatchObject({ request_id: 'review-evidence', fields: { decision: 'approve', feedback: 'Evidence reviewed' } });
});

for (const width of [1440, 390, 320]) test(`REL-034/D3,035,036,040,041,042 relationships and review wrapping at ${width}px`, async ({ page }) => {
  pendingReview = true;
  long = true;
  await page.setViewportSize({ width, height: 1000 });
  await page.goto('/project/demo/reviews/evidence-105');
  await expect(page).toHaveURL(/\/jobs\/evidence-105\/story\?/);
  await initiallyVisible(page);
  const link = related(page).getByRole('link', { name: 'Job · release-104', exact: true });
  await expect(link).toBeVisible();
  await link.focus();
  await page.keyboard.press('Tab');
  const child = related(page).getByRole('link', { name: /Job · Check API compatibilityW/ });
  await expect(child).toBeFocused();
  expect(await child.evaluate(element => getComputedStyle(element).outlineStyle)).toBe('solid');
  await expect(related(page).getByRole('link', { name: /Job · Check API compatibilityW/ })).toBeVisible();
  await expect(related(page).getByText('No goal recorded')).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);

  await selectStoryTab(page, 'Thread');
  const document = page.getByRole('region', { name: 'Selected document', exact: true });
  const response = page.getByRole('region', { name: 'Review decisions', exact: true });
  const documentBox = (await document.boundingBox())!;
  const responseBox = (await response.boundingBox())!;
  expect(responseBox.y).toBeGreaterThanOrEqual(documentBox.y + documentBox.height - 1);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
  if (width === 390) await capture(page, 'review-mobile-wrapped.png');
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
  await selectStoryTab(page, 'Graph');
  expect(childRequests).toBe(requests);
  if(width===390) await capture(page,'story-mobile-direct.png');
  await page.reload();
  await initiallyVisible(page);
});

test('opening and refreshing related jobs only fetches child relationships', async ({page}) => {
  const goalReads: string[] = [];
  page.on('request', request => {
    const url = new URL(request.url());
    if (url.pathname.endsWith('/jobs') && url.searchParams.has('goal')) goalReads.push(url.href);
  });
  await page.goto('/project/demo/jobs/evidence-105/story?tab=dag');
  await initiallyVisible(page);
  await related(page).getByRole('button', {name:'Refresh relationships'}).click();
  await expect(related(page).getByText('3 child jobs shown')).toBeVisible();
  await expect(page.getByText('Other attempts at this goal')).toHaveCount(0);
  expect(goalReads).toEqual([]);
});
