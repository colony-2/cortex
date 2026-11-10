import { test, expect } from '@playwright/test';
import { jobIdentity } from './helpers/cells';

test.beforeEach(async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'colony2:user:email', value: 'conversation@example.com', url: baseURL! }]);
  await page.addInitScript(() => {
    (window as any).EventSource = class extends EventTarget {
      readyState = 1;
      constructor() { super(); (window as any).historyInputStream = this; }
      close() {}
    };
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { tenant_id: 'demo', job_id: 'job', entries: [], snapshot_token: 'snapshot', completeness: 'partial' } });
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ status: 404, json: { error: 'Not retained', code: 'history_not_found' } });
    if (url.pathname === '/api/projects') return route.fulfill({ json: [{ tenant_id: 'demo' }] });
    if (url.pathname.endsWith('/user-inputs/pending')) return route.fulfill({ json: [] });
    if (url.pathname.endsWith('/input-artifacts')) return route.fulfill({ json: { artifacts: [] } });
    if (url.pathname.endsWith('/children')) return route.fulfill({ json: { jobs: [] } });
    if (url.pathname.endsWith('/story')) return route.fulfill({ json: {
      job_id: 'job', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'Build', status: 'completed', path: [], attempt: 1 },
    } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: {
      ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Conversation fixture', prompt: 'The original request',
      status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'success', completion_detail: 'The requested work is complete.',
    } });
    return route.fulfill({ json: {} });
  });
});

test('CT-001–007 default, panel visibility, keyboard navigation and URL restoration', async ({ page }, info) => {
  await page.goto('/project/demo/jobs/job/story?tenantId=demo&filter=mine');
  const tabs = page.getByRole('tablist', { name: 'Job detail' });
  const conversation = tabs.getByRole('tab', { name: 'Conversation', exact: true });
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('heading', { name: 'Job Story: Conversation fixture' })).toBeVisible();
  await expect(page.getByLabel('Job cell')).toBeVisible();
  await expect(page.getByLabel('Job status')).toBeVisible();
  await expect(page.getByRole('button', { name: /Refresh$/ })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job prompt' })).toHaveText('PromptThe original request');
  await expect(page.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(page.getByRole('tree')).toHaveCount(0);
  await page.screenshot({ path: info.outputPath('conversation.png'), fullPage: true });
  for (const label of ['Steps', 'Relationships', 'Input artifacts', 'Summary', 'Conversation']) {
    await tabs.getByRole('tab', { name: label, exact: true }).click();
    await expect(page.locator('.job-detail-panel:visible')).toHaveCount(1);
    await expect(page.getByRole('tabpanel', { name: label, exact: true })).toBeVisible();
    await expect(page).toHaveURL(/tenantId=demo&filter=mine&tab=/);
  }
  await conversation.focus();
  await page.keyboard.press('ArrowRight');
  await expect(tabs.getByRole('tab', { name: 'Relationships', exact: true })).toBeFocused();
  expect(await tabs.getByRole('tab', { name: 'Relationships', exact: true }).evaluate(element => getComputedStyle(element).outlineStyle)).toBe('solid');
  await page.keyboard.press('End');
  await expect(tabs.getByRole('tab', { name: 'Steps', exact: true })).toBeFocused();
  await page.reload();
  await expect(tabs.getByRole('tab', { name: 'Steps', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goBack();
  await expect(tabs.getByRole('tab', { name: 'Relationships', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goForward();
  await expect(tabs.getByRole('tab', { name: 'Steps', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?tab=invalid&nodeId=root');
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?nodeId=root');
  await expect(tabs.getByRole('tab', { name: 'Steps', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?requestId=opaque%2Funknown&tab=execution');
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByText('Request unavailable', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /Submit/ })).toHaveCount(0);
});

test('CT-007 narrow tabs wrap without horizontal page overflow', async ({ page }, info) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto('/project/demo/jobs/job/story');
  const tabs = page.getByRole('tablist', { name: 'Job detail' });
  await expect(tabs).toBeVisible();
  const boxes = await tabs.getByRole('tab').evaluateAll(elements => elements.map(element => ({ top: element.getBoundingClientRect().top, right: element.getBoundingClientRect().right })));
  expect(new Set(boxes.map(box => box.top)).size).toBeGreaterThan(1);
  expect(Math.max(...boxes.map(box => box.right))).toBeLessThanOrEqual(320);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
  await tabs.getByRole('tab', { name: 'Summary', exact: true }).focus();
  await page.keyboard.press('Home');
  await expect(tabs.getByRole('tab', { name: 'Conversation', exact: true })).toBeFocused();
  await page.screenshot({ path: info.outputPath('conversation-narrow.png'), fullPage: true });
});

test('CT-014,021,031 retained answers and exact earlier review stay selected through completion', async ({ page }) => {
  let stage = 0;
  const entry = (id: string, answered: boolean) => ({
    entry_id: id, request_id: `request-${id}`, kind: 'review', title: { present: true, raw: `Review ${id}` }, summary: { present: false },
    state: answered ? 'answered' : 'pending', requested_at: { present: false }, locator: { invocation_id: id },
    form_fields: { question: `Question ${id}` }, accepted_response: { present: answered, ...(answered ? { raw: { decision: 'approve', feedback: 'Retain the original' } } : {}) },
    response_source: answered ? 'submission' : '', actor: { provenance: answered ? 'submission_receipt' : 'unavailable', legacy_user_id: { present: false } },
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { tenant_id: 'demo', job_id: 'job', entries: [entry('first', stage > 0), ...(stage ? [entry('second', stage > 1)] : [])], snapshot_token: 'snapshot', completeness: 'partial' } });
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ json: { ...entry('first', true), snapshot_token: 'snapshot', completeness: 'partial', request_manifest: [{ artifact_id: 'original', availability: 'retained', ref: {}, associations: [] }], response_manifest: [], actionability: { actionable: false, observation: 'live' } } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: { ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Conversation fixture', prompt: 'The original request', status: stage > 1 ? 'COMPLETED' : 'READY', store: stage > 1 ? 'ARCHIVED' : 'ACTIVE', completion_status: stage > 1 ? 'success' : '', completion_detail: stage > 1 ? 'Persisted completion' : '' } });
    return route.fallback();
  });
  await page.goto('/project/demo/jobs/job/story?tenantId=demo&filter=mine');
  await expect(page.getByText('State: pending', { exact: false })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job completion' })).toHaveCount(0);
  stage = 1;
  await page.getByRole('button', { name: 'Refresh history', exact: true }).click();
  await expect(page.locator('[data-occurrence="second"]')).toBeVisible();
  await expect(page.locator('.conversation-answer').first()).toBeVisible();
  await page.getByRole('button', { name: 'Open review' }).first().click();
  await expect(page).toHaveURL(/occurrenceId=first/);
  const recorded = page.getByRole('region', { name: 'Recorded request', exact: true });
  await expect(recorded.getByText('Occurrence: first')).toBeAttached();
  await expect(recorded.getByRole('link', { name: 'Download original' })).toHaveAttribute('href', /entry_id=first&artifact_id=original/);
  await expect(recorded.getByRole('button', { name: /Submit/ })).toHaveCount(0);
  await page.reload();
  await expect(recorded.getByText('Occurrence: first')).toBeAttached();
  stage = 2;
  await page.getByRole('button', { name: /Refresh$/ }).click();
  await expect(page.getByRole('region', { name: 'Job completion' })).toContainText('Persisted completion');
  await expect(page.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(recorded.getByText('Occurrence: first')).toBeAttached();
  await expect(page).toHaveURL(/occurrenceId=first/);
});


test('CT-021 automatic polling and input activity preserve historical selection, scroll and focus', async ({ page }) => {
  await page.clock.install();
  let revision = 0;
  const entry = (id: string) => ({
    entry_id: id, request_id: id, kind: 'review', title: { present: true, raw: id }, summary: { present: false },
    state: 'answered', requested_at: { present: false }, locator: { invocation_id: id },
    form_fields: { question: 'Original review\n'.repeat(80) }, accepted_response: { present: true, raw: 'Original accepted answer' },
    response_source: 'submission', actor: { provenance: 'submission_receipt', legacy_user_id: { present: false } },
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { tenant_id: 'demo', job_id: 'job', entries: [entry('first'), ...(revision ? [entry(`arrival-${revision}`)] : [])], snapshot_token: `snapshot-${revision}`, completeness: 'partial' } });
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ json: { ...entry('first'), snapshot_token: 'original-detail', completeness: 'partial', request_manifest: [{ artifact_id: 'original', availability: 'retained' }], response_manifest: [], actionability: { actionable: false } } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: { ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Reading history', status: 'READY', store: 'ACTIVE' } });
    return route.fallback();
  });
  await page.goto('/project/demo/jobs/job/story?occurrenceId=first');
  const recorded = page.getByRole('region', { name: 'Recorded request', exact: true });
  const link = recorded.getByRole('link', { name: 'Download original' });
  await expect(link).toBeVisible();
  await link.focus();
  await page.evaluate(() => window.scrollBy(0, -200));
  const scrollBefore = await page.evaluate(() => window.scrollY);
  expect(scrollBefore).toBeGreaterThan(0);
  revision = 1;
  await page.clock.runFor(5100);
  await expect(page.locator('[data-occurrence="arrival-1"]')).toBeAttached();
  await expect(link).toBeFocused();
  expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollBefore)).toBeLessThanOrEqual(1);
  await expect(page.getByRole('tab', { name: 'Conversation', exact: true })).toHaveAttribute('aria-selected', 'true');
  expect(new URL(page.url()).searchParams.get('occurrenceId')).toBe('first');
  await expect(page.getByText('New conversation activity available')).toBeAttached();

  // Send the actual event consumed by InputActivityProvider before the next
  // polling interval. Its new snapshot must refresh history without navigation.
  revision = 2;
  await page.evaluate(() => (window as any).historyInputStream.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [] }) })));
  await expect(page.locator('[data-occurrence="arrival-2"]')).toBeAttached();
  await expect(link).toBeFocused();
  expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollBefore)).toBeLessThanOrEqual(1);
  expect(new URL(page.url()).searchParams.get('occurrenceId')).toBe('first');
});

for (const width of [1280,390]) test(`CC-001 CC-009 CC-010 CC-013 CC-014 compact review and historical reader at ${width}px`, async ({page},info)=>{
 await page.setViewportSize({width,height:900});
 const doc=(name:string)=>({kind:'stored',name,stored:{key:{jobId:'original-producer',taskOrdinal:4,name,sizeBytes:100}}});
 const documents={Overview:doc('overview.md'),Data:doc('data.json'),Picture:doc('picture.png'),Markup:doc('page.html')};
 const form={kind:'review',title:'1p review request',fields:[{id:'choice',type:'multiple_choice',required:true,question:'Review the proposal and leave your decision.',options:[{value:'revise',label:'Changes Requested'}]},{id:'feedback',type:'paragraph_text',question:'Comments'}],documents,document_metadata:{Overview:{order:0,title:'Proposal overview',role:'primary',description:'Saved review proposal',source_document_ids:['Data']},Data:{order:1,title:'Original data'},Picture:{order:2},Markup:{order:3}},overview_document_id:'Overview'};
 const entry=(id:string,pending=false)=>({entry_id:id,request_id:'reused-request',kind:'review',title:{present:true,raw:'1p review request'},summary:{present:false},state:pending?'pending':'answered',requested_at:{present:true,raw:'2025-01-02T14:30:00Z'},locator:{invocation_id:id},form_fields:form,accepted_response:{present:!pending,raw:{fields:{choice:'revise',feedback:'Some comments left by j@j.com. '+ 'Full feedback. '.repeat(50)}}},response_source:'external_submission',actor:{provenance:'submission_receipt',receipt:{submitted_at:'2025-01-02T14:35:00Z',actor:{id:'j@j.com',kind:'human'}},legacy_user_id:{present:false}}});
 const mutations:string[]=[], pendingURLs:string[]=[], artifactURLs:string[]=[];
 await page.route('**/api/**',async route=>{
  const request=route.request(),url=new URL(request.url());
  if(request.method()!=='GET'){mutations.push(request.url());return route.fulfill({status:500});}
  if(url.pathname.includes('/reviews/') && /\/documents(?:\/|$)/.test(url.pathname)){pendingURLs.push(request.url());return route.fulfill({status:500});}
  if(url.pathname.endsWith('/conversation'))return route.fulfill({json:{tenant_id:'demo',job_id:'job',snapshot_token:'frozen',completeness:'complete',entries:[entry('first'),entry('second',true)]}});
  if(url.pathname.endsWith('/conversation/entry'))return route.fulfill({json:{...entry(url.searchParams.get('entry_id')!,url.searchParams.get('entry_id')==='second'),raw_form:form,snapshot_token:'frozen',completeness:'complete',request_manifest:Object.entries(documents).reverse().map(([id,ref])=>({artifact_id:id,ref,availability:'retained',associations:[{kind:'document',source:`/documents/${id}`}]})),response_manifest:[]}});
  if(url.pathname.endsWith('/conversation/artifact')){
   artifactURLs.push(request.url());const id=url.searchParams.get('artifact_id');
   if(id==='Picture')return route.fulfill({contentType:'text/plain',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=','base64')});
   return route.fulfill({contentType:'text/plain',body:id==='Overview'?'# Original proposal\n\n[Data](data.json)\n\n![Retained picture](picture.png)':id==='Data'?' {"original":true}':'<!doctype html><html><body><h1>Saved markup</h1></body></html>'});
  }
  return route.fallback();
 });
 await page.goto('/project/demo/jobs/job/story?filter=mine');
 await expect(page.locator('.conversation-step')).toHaveCount(3);
 for(const card of await page.locator('.conversation-step').all()){
  const height=await card.evaluate(el=>el.getBoundingClientRect().height);
  expect(height).toBeLessThanOrEqual(width < 768 ? 120 : 100);expect(height).toBeGreaterThan(60);
  const rows=await card.locator('.conversation-line, .conversation-answer, .conversation-expand').evaluateAll(elements=>elements.map(el=>({top:el.getBoundingClientRect().top,bottom:el.getBoundingClientRect().bottom})));
  expect(rows.length).toBeGreaterThanOrEqual(2);expect(rows.length).toBeLessThanOrEqual(3);
  for(let i=1;i<rows.length;i++)expect(rows[i].top).toBeGreaterThanOrEqual(rows[i-1].bottom);
 }
 await expect(page.locator('.conversation-answer')).toContainText('Changes Requested');
 await expect(page.locator('.conversation-step').first()).toContainText('2025');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
 await page.screenshot({path:info.outputPath(`compact-${width}.png`),fullPage:true});
 await page.getByRole('button',{name:'Open review',exact:true}).first().click();
 const reader=page.getByRole('region',{name:'Recorded request',exact:true});
 await expect(reader.getByRole('heading',{name:'Original proposal'})).toBeVisible();
 await expect(reader.getByRole('img',{name:'picture.png'})).toBeVisible();
 await expect(reader.getByLabel('Review context')).toContainText('Review the proposal');
 await expect(reader.getByLabel('Recorded answers')).toContainText('Changes Requested');
 await expect(reader.getByRole('button',{name:/Submit|Approve|Reject|Cancel job/})).toHaveCount(0);
 await expect(page.getByRole('button',{name:/Cancel job/i})).toHaveCount(0);
 const dataLink=reader.getByRole('link',{name:'Data',exact:true});
 await expect(dataLink).toHaveAttribute('href',/entry_id=first&artifact_id=Data&snapshot_token=frozen/);
 await expect(reader.getByText('Loading attached image…')).toHaveCount(0);
 await expect(reader.getByRole('img',{name:'picture.png'})).toBeVisible();
 await reader.screenshot({path:info.outputPath(`overview-${width}.png`)});
 await dataLink.click();await expect(reader.getByLabel('Formatted JSON')).toContainText('"original": true');
 await expect(reader.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/artifact_id=Data&snapshot_token=frozen/);
 await page.screenshot({path:info.outputPath(`reader-${width}.png`),fullPage:true});
 await expect(reader.getByRole('navigation',{name:'Review documents'})).toBeHidden();
 await reader.getByRole('button',{name:'Show documents',exact:true}).click();
 if(width===1280) await reader.getByRole('button',{name:/page.html/}).click();
 else await reader.getByRole('button',{name:'Read page.html',exact:true}).click();
 await expect(reader.locator('iframe[title="Static HTML preview: page.html"]')).toHaveAttribute('sandbox','');
 await expect(reader.frameLocator('iframe').getByRole('heading',{name:'Saved markup'})).toBeVisible();
 const back=reader.getByRole('button',{name:'Back to conversation'});
 // Allow Chromium's iframe hit-test surface to catch up after scrolling from
 // the distant preview to the reader header before dispatching a pointer click.
 await back.scrollIntoViewIfNeeded();
 await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
 await back.click();
 await expect(page).not.toHaveURL(/occurrenceId=/);
 expect(new URL(page.url()).searchParams.get('filter')).toBe('mine');
 await page.getByRole('button',{name:'Open review',exact:true}).last().click();
 await expect(reader.getByRole('heading',{name:'Original proposal'})).toBeVisible();
 await expect(reader.getByLabel('Recorded answers')).toHaveCount(0);
 expect(mutations).toEqual([]);expect(pendingURLs).toEqual([]);
 expect(artifactURLs.length).toBeGreaterThan(2);
 for(const url of artifactURLs){expect(url).toContain('snapshot_token=frozen');expect(url).toMatch(/entry_id=(first|second)&artifact_id=/);}
});

for(const status of [403,404,410,413]) test(`CC-016 document-specific ${status} failure retains review and retry`,async({page})=>{
 let fail=true;
 const document={kind:'stored',name:'missing.md',stored:{key:{jobId:'p',taskOrdinal:1,name:'missing.md',sizeBytes:20}}};
 await page.route('**/api/**',async route=>{
  const url=new URL(route.request().url());
  if(url.pathname.endsWith('/conversation/entry'))return route.fulfill({json:{entry_id:'old',kind:'review',title:{present:true,raw:'Saved review'},summary:{present:false},state:'pending',requested_at:{present:false},locator:{invocation_id:'old'},form_fields:{},accepted_response:{present:false},actor:{provenance:'unavailable',legacy_user_id:{present:false}},raw_form:{question:'Retain these instructions',documents:{missing:document}},snapshot_token:'s',completeness:'partial',request_manifest:[{artifact_id:'missing',ref:document,associations:[],availability:'retained'}],response_manifest:[]}});
  if(url.pathname.endsWith('/conversation/artifact'))return fail?route.fulfill({status,json:{error:'Original bytes unavailable',code:'unavailable'}}):route.fulfill({body:'# Recovered original'});
  return route.fallback();
 });
 await page.goto('/project/demo/jobs/job/story?occurrenceId=old');
 const reader=page.getByRole('region',{name:'Recorded request',exact:true});
 await expect(reader.getByText(/Could not load this document/)).toBeVisible();
 await expect(reader.getByLabel('Review context')).toHaveText('Retain these instructions');
 await expect(reader.getByRole('region',{name:'Selected document',exact:true}).getByText('missing.md',{exact:true})).toBeVisible();
 if(status===413)await expect(reader.getByText(/limited to 32 MiB/)).toBeVisible();
 fail=false;await reader.getByRole('button',{name:'Retry',exact:true}).click();await expect(reader.getByRole('heading',{name:'Recovered original'})).toBeVisible();
});
