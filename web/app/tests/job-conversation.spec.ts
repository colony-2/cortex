import { storyAction, originalRequest, expandHistory } from './helpers/inbox';
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
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'COMPLETED',store:'ARCHIVED'}},final_documents:{availability:'available',value:{job_id:'job',state:'empty'}}, tenant_id: 'demo', job_id: 'job', entries: [], snapshot_token: 'snapshot', completeness: 'partial' } });
    if (url.pathname.endsWith('/final-documents')) return route.fulfill({json:{job_id:'job',manifest_id:'none',state:'absent',documents:[]}});
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ status: 404, json: { error: 'Not retained', code: 'history_not_found' } });
    if (url.pathname === '/api/projects') return route.fulfill({ json: [{ tenant_id: 'demo' }] });
    if (url.pathname.endsWith('/cells')) return route.fulfill({ json: { cells: [] } });
    if (url.pathname.endsWith('/jobs')) return route.fulfill({ json: { jobs: [] } });
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

test('prompt-only messages show their preview when collapsed and their full text when expanded', async ({page}) => {
  const prompt = 'Please review the release plan. '.repeat(6).trim();
  const question = 'Which release should we use? '.repeat(6).trim();
  await page.route('**/conversation?*', route => route.fulfill({json:{
    tenant_id:'demo', job_id:'job', entries:[], snapshot_token:'s', input_artifacts:[],
    activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'READY',store:'ACTIVE',prompt}},
    current_input:{jobId:'job',status:'pending',startTime:'',form:{type:'short_answer',question}},
  }}));
  await page.goto('/project/demo/jobs/job/story');
  const original = page.locator('.inbox-original');
  const heading = page.getByRole('heading', {name:/Please review the release plan/});
  await expect(heading).toBeVisible();
  await original.locator('summary').click();
  await expect(heading).toBeHidden();
  await expect(original.getByLabel('Job prompt')).toHaveText(prompt);
  await expect(original.locator('summary')).not.toContainText('Please review');
  await original.locator('summary').click();
  await expect(heading).toBeVisible();
  const current = page.locator('.inbox-history-message');
  await expect(current.getByLabel(question, {exact:true})).toBeVisible();
  await expect(current.locator('summary')).not.toContainText('Which release');
  await current.locator('summary').click();
  await expect(current.locator('summary')).toContainText('Which release');
});

test('CT-001–007 default, panel visibility, keyboard navigation and URL restoration', async ({ page }, info) => {
  await page.goto('/project/demo/jobs/job/story?tenantId=demo&filter=mine');
  const tabs = page.getByRole('tablist', { name: 'Job detail' });
  const conversation = tabs.getByRole('tab', { name: 'Thread', exact: true });
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('heading', { name: 'Conversation fixture' })).toBeVisible();
  await expect(page.getByLabel('Job cell')).toBeVisible();
  await expect(page.getByLabel('Job status')).toBeVisible();
  await expect((await storyAction(page, 'Refresh'))).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job prompt' })).toBeHidden();
  await expect(await originalRequest(page)).toHaveText('The original request');
  await expect(page.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(page.getByRole('tree')).toHaveCount(0);
  await page.screenshot({ path: info.outputPath('conversation.png'), animations:'disabled',fullPage: true });
  for (const label of ['Trace', 'Graph', 'Execution', 'Thread']) {
    await tabs.getByRole('tab', { name: label, exact: true }).click();
    await expect(page.locator('.job-detail-panel:visible')).toHaveCount(1);
    await expect(page.getByRole('tabpanel', { name: label, exact: true })).toBeVisible();
    await expect(page).toHaveURL(/tenantId=demo&filter=mine&tab=/);
  }
  await conversation.focus();
  await page.keyboard.press('ArrowRight');
  await expect(tabs.getByRole('tab', { name: 'Execution', exact: true })).toBeFocused();
  expect(await tabs.getByRole('tab', { name: 'Execution', exact: true }).evaluate(element => getComputedStyle(element).outlineStyle)).toBe('solid');
  await page.keyboard.press('End');
  await expect(tabs.getByRole('tab', { name: 'Trace', exact: true })).toBeFocused();
  await page.reload();
  await expect(tabs.getByRole('tab', { name: 'Trace', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goBack();
  await expect(tabs.getByRole('tab', { name: 'Execution', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goForward();
  await expect(tabs.getByRole('tab', { name: 'Trace', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?tab=invalid&nodeId=root');
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?nodeId=root');
  await expect(tabs.getByRole('tab', { name: 'Trace', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.goto('/project/demo/jobs/job/story?requestId=opaque%2Funknown&tab=execution');
  await expect(conversation).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByText('Request unavailable', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /Submit/ })).toHaveCount(0);
});

test('CT-007 narrow tabs scroll without horizontal page overflow', async ({ page }, info) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto('/project/demo/jobs/job/story');
  const tabs = page.getByRole('tablist', { name: 'Job detail' });
  await expect(tabs).toBeVisible();
  // The compact inbox uses one scrollable tab line. Keyboard focus must reveal the last tab.
  await tabs.getByRole('tab', { name: 'Trace', exact: true }).focus();
  await expect(tabs.getByRole('tab', { name: 'Trace', exact: true })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1);
  await tabs.getByRole('tab', { name: 'Trace', exact: true }).focus();
  await page.keyboard.press('Home');
  await expect(tabs.getByRole('tab', { name: 'Thread', exact: true })).toBeFocused();
  await page.screenshot({ path: info.outputPath('conversation-narrow.png'), animations:'disabled',fullPage: true });
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
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:stage>1?'COMPLETED':'READY',store:stage>1?'ARCHIVED':'ACTIVE',completion_status:stage>1?'success':undefined,completion_detail:stage>1?'Persisted completion':undefined}},final_documents:{availability:'available',value:{job_id:'job',state:'empty'}}, tenant_id: 'demo', job_id: 'job', entries: [entry('first', stage > 0), ...(stage ? [entry('second', stage > 1)] : [])], snapshot_token: 'snapshot', completeness: 'partial' } });
    if (url.pathname.endsWith('/final-documents')) return route.fulfill({json:{job_id:'job',manifest_id:'none',state:'absent',documents:[]}});
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ json: { ...entry(url.searchParams.get('entry_id')!, stage > 0 && url.searchParams.get('entry_id') === 'first'), snapshot_token: 'snapshot', completeness: 'partial', request_manifest: [{ artifact_id: 'original', availability: 'retained', ref: {}, associations: [] }], response_manifest: [], actionability: { actionable: false, observation: 'live' } } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: { ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Conversation fixture', prompt: 'The original request', status: stage > 1 ? 'COMPLETED' : 'READY', store: stage > 1 ? 'ARCHIVED' : 'ACTIVE', completion_status: stage > 1 ? 'success' : '', completion_detail: stage > 1 ? 'Persisted completion' : '' } });
    return route.fallback();
  });
  await page.goto('/project/demo/jobs/job/story?tenantId=demo&filter=mine');
  await expect(page.locator('.inbox-history-message')).toHaveCount(1);
  await expandHistory(page);
  await expect(page.getByRole('region', { name: 'Recorded request' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Job completion' })).toHaveCount(0);
  stage = 1;
  await (await storyAction(page, 'Refresh')).click();
  await expect(page.locator('.inbox-history-message[data-occurrence="second"]')).toBeAttached();
  await expandHistory(page);
  await expect(page.locator('.inbox-history-message[data-occurrence="second"]')).toBeVisible();
  await expect(page.locator('.conversation-answer').first()).toBeVisible();
  const deepLink = new URL(page.url());
  deepLink.searchParams.set('occurrenceId', 'first');
  await page.goto(deepLink.href);
  const recorded = page.locator('section[aria-label="Recorded request"][data-occurrence="first"]').first();
  await expect(recorded).toBeVisible();
  await expect(recorded.getByRole('link', { name: 'Download original' })).toHaveAttribute('href', /entry_id=first&artifact_id=original/);
  await expect(recorded.getByRole('button', { name: /Submit/ })).toHaveCount(0);
  await page.reload();
  await expect(recorded).toHaveAttribute('data-occurrence', 'first');
  stage = 2;
  await (await storyAction(page, 'Refresh')).click();
  await expect(page.getByRole('region', { name: 'Job completion' })).toContainText('Persisted completion');
  await expect(page.getByRole('region', { name: 'Job completion' })).not.toContainText('Merge status');
  await expect(recorded).toHaveAttribute('data-occurrence', 'first');
  await expect(page).toHaveURL(/occurrenceId=first/);
});


test('CT-021 automatic polling and input activity preserve historical selection, scroll and focus', async ({ page }, info) => {
  await page.setViewportSize({ width: 1280, height: 600 });
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
    if (url.pathname.endsWith('/conversation')) return route.fulfill({ json: { activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'READY',store:'ACTIVE'}},final_documents:{availability:'available',value:{job_id:'job',state:'empty'}}, tenant_id: 'demo', job_id: 'job', entries: [entry('first'), ...(revision ? [entry(`arrival-${revision}`)] : [])], snapshot_token: `snapshot-${revision}`, completeness: 'partial' } });
    if (url.pathname.endsWith('/final-documents')) return route.fulfill({json:{job_id:'job',manifest_id:'none',state:'absent',documents:[]}});
    if (url.pathname.endsWith('/conversation/entry')) return route.fulfill({ json: { ...entry('first'), snapshot_token: 'original-detail', completeness: 'partial', request_manifest: [{ artifact_id: 'original', availability: 'retained' }], response_manifest: [], actionability: { actionable: false } } });
    if (url.pathname.endsWith('/jobs/job')) return route.fulfill({ json: { ...jobIdentity, tenant_id: 'demo', job_id: 'job', title: 'Reading history', status: 'READY', store: 'ACTIVE' } });
    return route.fallback();
  });
  await page.goto('/project/demo/jobs/job/story?occurrenceId=first');
  const recorded = page.getByRole('region', { name: 'Recorded request', exact: true });
  const link = recorded.getByRole('link', { name: 'Download original' });
  await expect(link).toBeVisible();
  await link.focus();
  await page.evaluate(() => window.scrollTo(0, Math.min(200, document.documentElement.scrollHeight - innerHeight)));
  const scrollBefore = await page.evaluate(() => window.scrollY);
  expect(scrollBefore).toBeGreaterThan(0);
  revision = 1;
  await page.clock.runFor(5100);
  await expect(page.locator('.inbox-history-message[data-occurrence="arrival-1"]').first()).toBeAttached();
  await expect(link).toBeFocused();
  expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollBefore)).toBeLessThanOrEqual(1);
  await expect(page.getByRole('tab', { name: 'Thread', exact: true })).toHaveAttribute('aria-selected', 'true');
  expect(new URL(page.url()).searchParams.get('occurrenceId')).toBe('first');
  await expect(page.getByText('New thread activity available')).toBeAttached();

  // Send the actual event consumed by InputActivityProvider before the next
  // polling interval. Its new snapshot must refresh history without navigation.
  revision = 2;
  await page.evaluate(() => (window as any).historyInputStream.dispatchEvent(new MessageEvent('inputs_changed', { data: JSON.stringify({ inputs: [] }) })));
  await expect(page.locator('.inbox-history-message[data-occurrence="arrival-2"]').first()).toBeAttached();
  await expect(link).toBeFocused();
  expect(Math.abs(await page.evaluate(() => window.scrollY) - scrollBefore)).toBeLessThanOrEqual(1);
  expect(new URL(page.url()).searchParams.get('occurrenceId')).toBe('first');
  const activity = page.getByText('New thread activity available');
  await activity.scrollIntoViewIfNeeded();
  await page.screenshot({path:info.outputPath('historical-activity-desktop.png'),animations:'disabled'});
  await page.setViewportSize({width:390,height:844});
  await activity.scrollIntoViewIfNeeded();
  await page.screenshot({path:info.outputPath('historical-activity-phone.png'),animations:'disabled'});
});

for (const width of [1280,390]) test(`CC-001 CC-009 CC-010 CC-013 CC-014 compact review and historical reader at ${width}px`, async ({page},info)=>{
 await page.setViewportSize({width,height:900});
 const doc=(name:string)=>({kind:'stored',name,stored:{key:{jobId:'original-producer',taskOrdinal:4,name,sizeBytes:100}}});
 const documents={Overview:doc('overview.md'),Data:doc('data.json'),Picture:doc('picture.png'),Markup:doc('page.html')};
 const form={kind:'review',title:'1p review request',fields:[{id:'choice',type:'multiple_choice',required:true,question:'Review the proposal and leave your decision.',options:[{value:'revise',label:'Changes Requested'}]},{id:'feedback',type:'paragraph_text',question:'Comments'}],documents,document_metadata:{Overview:{order:0,title:'Proposal overview',role:'primary',description:'Saved review proposal',source_document_ids:['Data']},Data:{order:1,title:'Original data'},Picture:{order:2},Markup:{order:3}},overview_document_id:'Overview'};
 const entry=(id:string,pending=false)=>({entry_id:id,request_id:'reused-request',kind:'review',title:{present:true,raw:'1p review request'},summary:{present:false},state:pending?'pending':'answered',requested_at:{present:true,raw:'2025-01-02T14:30:00Z'},locator:{invocation_id:id},form_fields:form,accepted_response:{present:!pending,raw:{fields:{choice:'revise',feedback:'Some comments left by j@j.com. '+ 'Full feedback. '.repeat(50)}}},response_source:'external_submission',actor:{provenance:'submission_receipt',receipt:{submitted_at:'2025-01-02T14:35:00Z',actor:{id:'j@j.com',kind:'human'}},legacy_user_id:{present:false}}});
 const mutations:string[]=[], pendingURLs:string[]=[], artifactURLs:string[]=[], detailURLs:string[]=[];
 const requestManifest=Object.entries(documents).reverse().map(([id,ref])=>({artifact_id:id,ref,availability:'retained',associations:[{kind:'document',source:`/documents/${id}`}]}));
 await page.route('**/api/**',async route=>{
  const request=route.request(),url=new URL(request.url());
  if(request.method()!=='GET'){mutations.push(request.url());return route.fulfill({status:500});}
  if(url.pathname.includes('/reviews/') && /\/documents(?:\/|$)/.test(url.pathname)){pendingURLs.push(request.url());return route.fulfill({status:500});}
  if(url.pathname.endsWith('/conversation')) {
   expect(url.searchParams.get('include_review_attachments')).toBe('true');
   const review_attachments={request:requestManifest,response:[]};
   return route.fulfill({json:{ activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'COMPLETED',store:'ARCHIVED'}},final_documents:{availability:'available',value:{job_id:'job',state:'empty'}},tenant_id:'demo',job_id:'job',snapshot_token:'frozen',completeness:'complete',entries:[{...entry('first'),review_attachments},{...entry('second',true),review_attachments}]}});
  }
  if(/\/conversation\/(entry|artifact)$/.test(url.pathname)) {
   detailURLs.push(request.url());
   return route.fulfill({status:410,json:{code:'history_snapshot_expired',error:'Snapshot expired'}});
  }
  if(url.pathname.endsWith('/stored-document')){
   artifactURLs.push(request.url());const id=Object.keys(documents).find(id=>documents[id as keyof typeof documents].stored.key.name===url.searchParams.get('name'));
   if(id==='Picture')return route.fulfill({contentType:'text/plain',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=','base64')});
   return route.fulfill({contentType:'text/plain',body:id==='Overview'?'# Original proposal\n\n[Data](data.json)\n\n![Retained picture](picture.png)':id==='Data'?' {"original":true}':'<!doctype html><html><body><h1>Saved markup</h1></body></html>'});
  }
  return route.fallback();
 });
 await page.goto('/project/demo/jobs/job/story?filter=mine');
 const messages = page.locator('.inbox-history-message:not([aria-label="Job completion"])');
 await expect(messages).toHaveCount(3);
 for (const summary of await messages.locator(':scope > summary').all()) {
  const height = await summary.evaluate(el => el.getBoundingClientRect().height);
  expect(height).toBeGreaterThanOrEqual(width < 768 ? 44 : 32);
 }
 await expandHistory(page);
 const reader=page.getByRole('region',{name:'Recorded request',exact:true});
 const first=reader.and(page.locator('[data-occurrence="first"]'));
 await expect(first.getByRole('heading',{name:'Original proposal'})).toBeVisible();
 await expect(first.getByRole('img',{name:'picture.png'})).toBeVisible();
 await expect(first.getByLabel('Review context')).toContainText('Review the proposal');
 const response=page.getByLabel('Accepted response',{exact:true});
 await expect(response).toContainText('Changes Requested');
 await expect(first.getByLabel('Recorded answers')).toHaveCount(0);
 await expect(first.getByRole('button',{name:/Submit|Approve|Reject|Cancel job/})).toHaveCount(0);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
 await page.screenshot({path:info.outputPath(`compact-${width}.png`),animations:'disabled',fullPage:true});
 const dataLink=first.getByRole('link',{name:'Data',exact:true});
 await expect(dataLink).toHaveAttribute('href',/stored-document\?ordinal=4&name=data.json/);
 await expect(first.getByText('Loading attached image…')).toHaveCount(0);
 await expect(first.getByRole('img',{name:'picture.png'})).toBeVisible();
 await first.screenshot({path:info.outputPath(`overview-${width}.png`)});
 await dataLink.click();
 const data=page.getByRole('dialog',{name:'Original data',exact:true});
 await expect(data.getByLabel('Formatted JSON')).toContainText('"original": true');
 await data.getByRole('button',{name:'Document options'}).click();
 await expect(page.getByRole('dialog',{name:'Document options'}).getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/stored-document\?ordinal=4&name=data.json/);
 await data.getByRole('button',{name:'Document options'}).click();
 await expect(page.getByRole('dialog',{name:'Document options'})).toBeHidden();
 await page.screenshot({path:info.outputPath(`reader-${width}.png`),animations:'disabled',fullPage:true});
 await data.getByRole('button',{name:'Close',exact:true}).click();
 await first.getByRole('navigation',{name:'Review attachments'}).getByRole('button',{name:'page.html',exact:true}).click();
 const markup=page.getByRole('dialog',{name:'page.html',exact:true});
 await expect(markup.locator('iframe[title="Interactive HTML preview: page.html"]')).toHaveAttribute('sandbox','allow-scripts');
 await expect(markup.frameLocator('iframe').getByRole('heading',{name:'Saved markup'})).toBeVisible();
 await markup.screenshot({path:info.outputPath(`historical-html-${width}.png`)});
 await markup.getByRole('button',{name:'Close',exact:true}).click();
 const second=reader.and(page.locator('[data-occurrence="second"]'));
 await expect(second.getByRole('heading',{name:'Original proposal'})).toBeVisible();
 await expect(second.getByLabel('Recorded answers')).toHaveCount(0);
 expect(detailURLs).toEqual([]);
 expect(mutations).toEqual([]);expect(pendingURLs).toEqual([]);
 expect(artifactURLs.length).toBeGreaterThan(2);
 for(const url of artifactURLs)expect(url).toContain('/jobs/original-producer/stored-document?ordinal=4&name=');
});

for(const status of [403,404,410,413]) test(`CC-016 document-specific ${status} failure retains review and retry`,async({page})=>{
 let fail=true;
 const document={kind:'stored',name:'missing.md',stored:{key:{jobId:'p',taskOrdinal:1,name:'missing.md',sizeBytes:20}}};
 await page.route('**/api/**',async route=>{
  const url=new URL(route.request().url());
  if(url.pathname.endsWith('/conversation/entry'))return route.fulfill({json:{entry_id:'old',kind:'review',title:{present:true,raw:'Saved review'},summary:{present:false},state:'pending',requested_at:{present:false},locator:{invocation_id:'old'},form_fields:{},accepted_response:{present:false},actor:{provenance:'unavailable',legacy_user_id:{present:false}},raw_form:{question:'Retain these instructions',documents:{missing:document}},snapshot_token:'s',completeness:'partial',request_manifest:[{artifact_id:'missing',ref:document,associations:[],availability:'retained'}],response_manifest:[]}});
  if(url.pathname.endsWith('/stored-document'))return fail?route.fulfill({status,json:{error:'Original bytes unavailable',code:'unavailable'}}):route.fulfill({body:'# Recovered original'});
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


test('CV-009 CV-010 CV-013 page refresh retains the conversation observation and retains stale observation', async ({ page }, info) => {
  page.on('pageerror', error => { console.error('Activity browser error:', error); });
  page.on('crash', () => { console.error('Activity browser crashed'); });
  let status = 'ACTIVE';
  let failed = false;
  await page.route(url => url.pathname === '/api/projects/demo/jobs/job', route => failed ? route.fulfill({status:503,json:{error:'Fixture metadata unavailable'}}) : route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'job',title:'Conversation fixture',status,store:'ACTIVE'}}));
  await page.route('**/conversation?*',route=>failed?route.fulfill({status:503,json:{error:'Unavailable conversation'}}):route.fulfill({json:{...terminalConversation({job_id:'job',state:'empty'}),activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status,store:status==='COMPLETED'?'ARCHIVED':'ACTIVE'}}}}));
  await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/api/projects/demo/jobs/job'),page.goto('/project/demo/jobs/job/story')]);
  await expect(page.getByRole('heading', {name:'Conversation fixture'})).toBeVisible();
  const tail = page.getByLabel('Current job activity');
  await expect(tail).toHaveCount(1);
  await expect(tail).toContainText('Running');
  await page.screenshot({path:info.outputPath('activity-running-desktop.png'),animations:'disabled',fullPage:true});
  status = 'PENDING_JOBS';
  await (await storyAction(page, 'Refresh')).click();
  await expect(tail).toContainText('Pending');
  await expect(tail).toContainText('Waiting');
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:info.outputPath('activity-pending-phone.png'),animations:'disabled',fullPage:true});
  failed = true;
  await (await storyAction(page, 'Refresh')).click();
  await expect(tail).toContainText('Status stale');
  await expect(tail).toContainText('Pending');
  await expect(tail).toHaveCount(1);
  failed = false; status = 'COMPLETED';
  await (await storyAction(page, 'Refresh')).click();
  await expect(tail).toHaveCount(0);
});

test('CV-002 CV-003 CV-005 CV-020 merge ordering and final document viewer',async({page},info)=>{
  const entry={entry_id:'review',kind:'review',title:{present:true,raw:'Approve publication'},summary:{present:false},state:'answered',requested_at:{present:false},locator:{invocation_id:'review',generation_ordinal:1,accepted_ordinal:3},form_fields:{},accepted_response:{present:true,raw:{feedback:'Approved after publication'}},response_source:'external_submission',actor:{provenance:'submission_receipt',legacy_user_id:{present:false}}};
  const merge={entry_id:'merge',tenant_id:'demo',job_id:'job',job_attempt:1,task_attempt:1,invocation_id:'merge/attempt:1',result_ordinal:2,operation:'merge:merge',evidence_availability:'recorded',candidate_sha:'abcdef0123456789',candidate_subject:'Finish conversation events and document previews',publication_branch:'refs/heads/main',publication_disposition:'published'};
  const fallback={...merge,entry_id:'fallback',invocation_id:'merge/attempt:2',result_ordinal:4,candidate_subject:null,publication_disposition:'already_present'};
  await page.route('**/conversation?*',route=>route.fulfill({json:{tenant_id:'demo',job_id:'job',entries:[entry],snapshot_token:'shared',snapshot_revision:4,completeness:'complete',publication_coverage:'recorded',merges:[merge,fallback],events:[{ordinal:1,invocation_id:'review',kind:'request_generation',entry_id:'review'},{ordinal:2,invocation_id:merge.invocation_id,kind:'merge_completion',entry_id:'merge'},{ordinal:3,invocation_id:'review',kind:'response_acceptance',entry_id:'review'},{ordinal:4,invocation_id:fallback.invocation_id,kind:'merge_completion',entry_id:'fallback'}]}}));
  await page.route('**/conversation?*',route=>route.fulfill({json:{...terminalConversation({job_id:'job',attempt_ordinal:1,manifest_id:'final-one',state:'present',documents:[{alias:'report.md',resolution:'stored',ref:{},raw:{},producer:{jobId:'grandchild',taskOrdinal:8,name:'private-storage.md'}},{alias:'notes.md',resolution:'stored',ref:{},raw:{},producer:{jobId:'child',taskOrdinal:4,name:'original.md'}},{alias:'external',resolution:'external',ref:{},raw:{}}]}),entries:[entry],merges:[merge,fallback],events:[{ordinal:1,kind:'request_generation',entry_id:'review'},{ordinal:2,kind:'merge_completion',entry_id:'merge'},{ordinal:3,kind:'response_acceptance',entry_id:'review'},{ordinal:4,kind:'merge_completion',entry_id:'fallback'}]}}));
  const reads:string[]=[];
  await page.route('**/final-documents/document?*',route=>{
    const url=new URL(route.request().url());reads.push(url.searchParams.get('alias')!);
    expect(url.searchParams.get('manifest_id')).toBe('final-one');
    return route.fulfill({contentType:'text/plain',body:url.searchParams.get('alias')==='report.md'?'# Final report\n\n[Read notes](notes.md)\n\nOnly declared terminal documents.':'# Supporting notes\n\nOriginal child document.'});
  });
  await page.goto('/project/demo/jobs/job/story');
  const summaries = page.locator('.inbox-history-message:not([aria-label="Job completion"]) > summary .thread-message-subject');
  await expect(summaries).toHaveCount(4);
  await expect(summaries).toHaveText([/Approve publication/, /Finish conversation events/, /Approved after publication/, /Recorded commit/]);
  await expect(page.locator('.inbox-history-message > summary .thread-message-label')).toHaveText(['Review', 'Merge', 'Feedback', 'Merge', 'Completion']);
  await expandHistory(page);
  await expect(page.getByLabel('Verified publication')).toHaveCount(2);
  await expect(page.getByText('Commit message not recorded',{exact:true}).first()).toBeVisible();
  await expect(page.getByText('Publication confirmed — already present',{exact:true})).toBeVisible();
  await page.screenshot({path:info.outputPath('merge-events-desktop.png'),animations:'disabled',fullPage:true});
  const activate=page.getByRole('link',{name:'report.md',exact:true});await activate.click();
  const viewer=page.getByRole('dialog',{name:'Attachment viewer'});
  await expect(viewer.getByRole('heading',{name:'Final report',exact:true})).toBeVisible();
  await expect(viewer.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/manifest_id=final-one/);
  await page.screenshot({path:info.outputPath('returned-document-viewer-desktop.png'),animations:'disabled',fullPage:false});
  await viewer.getByRole('link',{name:'Read notes',exact:true}).click();
  await expect(viewer.getByRole('heading',{name:'Supporting notes',exact:true})).toBeVisible();
  await viewer.getByRole('button',{name:'Close',exact:true}).click();
  await expect(activate).toBeFocused();
  expect([...new Set(reads)]).toEqual(['report.md','notes.md']);
  await expect(page.getByLabel('Current job activity')).toHaveCount(0);
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:info.outputPath('conversation-final-phone.png'),animations:'disabled',fullPage:true});
});

for(const status of [403,404]) test(`CV-021 final document ${status} leaves other members usable`,async({page})=>{
  await page.route('**/conversation?*',route=>route.fulfill({json:terminalConversation({job_id:'job',manifest_id:'errors',state:'present',documents:['missing.md','good.md'].map(alias=>({alias,resolution:'stored',producer:{jobId:'producer',taskOrdinal:1,name:alias},ref:{},raw:{}}))})}));
  await page.route('**/final-documents/document?*',route=>new URL(route.request().url()).searchParams.get('alias')==='missing.md'?route.fulfill({status,json:{error:status===403?'Producer access denied':'Bytes not retained'}}):route.fulfill({body:'# Available final document',contentType:'text/plain'}));
  await page.goto('/project/demo/jobs/job/story');
  const list=page.getByLabel('Attachments',{exact:true});
  await list.getByRole('link',{name:'missing.md',exact:true}).click();
  const viewer=page.getByRole('dialog',{name:'Attachment viewer'});
  await expect(viewer.getByRole('alert')).toContainText(status===403?'Producer access denied':'Bytes not retained');
  await expect(viewer).not.toContainText(/expired/i);
  await viewer.getByRole('button',{name:'Close',exact:true}).click();
  await list.getByRole('link',{name:'good.md',exact:true}).click();
  await expect(viewer.getByRole('heading',{name:'Available final document'})).toBeVisible();
});


test('CX-001 automatic completion supersedes an in-flight pre-publication snapshot', async ({page}, info) => {
  let terminal = false;
  let holdNext = false;
  let held = false;
  let releaseOld!: () => void;
  const oldRead = new Promise<void>(resolve => { releaseOld = resolve; });
  const merge = {entry_id:'final-merge',tenant_id:'demo',job_id:'job',job_attempt:1,task_attempt:1,invocation_id:'merge/final',result_ordinal:2,operation:'merge:merge',evidence_availability:'recorded',candidate_sha:'abcdef0123456789',candidate_subject:'Final automatically observed publication',publication_branch:'refs/heads/main',publication_disposition:'published'};
  await page.route(url => url.pathname === '/api/projects/demo/jobs/job', route => route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'job',title:'Conversation fixture',status:terminal?'COMPLETED':'ACTIVE',store:terminal?'ARCHIVED':'ACTIVE'}}));
  await page.route('**/conversation?*', async route => {
    const final = terminal;
    if (holdNext && !held) { held = true; await oldRead; }
    await route.fulfill({json:{...terminalConversation({job_id:'job',manifest_id:'terminal',state:final?'present':'pending',documents:final?[{alias:'report.md',resolution:'stored',producer:{jobId:'job',taskOrdinal:8,name:'report.md'}}]:[]}),activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:final?'COMPLETED':'ACTIVE',store:final?'ARCHIVED':'ACTIVE'}},entries:[],snapshot_token:final?'final':'before',completeness:'complete',publication_coverage:'recorded',merges:final?[merge]:[],events:final?[{ordinal:2,invocation_id:merge.invocation_id,kind:'merge_completion',entry_id:merge.entry_id}]:[]}}).catch(() => {});
  });
  await page.route('**/final-documents', route => route.fulfill({json:{job_id:'job',manifest_id:'terminal',state:'present',documents:[{alias:'report.md',resolution:'stored',ref:{},raw:{},producer:{jobId:'job',taskOrdinal:8,name:'report.md'}}]}}));
  await page.route('**/final-documents/document?*', route => route.fulfill({contentType:'text/plain',body:'# Automatically returned report'}));
  await page.goto('/project/demo/jobs/job/story');
  await expect(page.getByLabel('Current job activity')).toContainText('Running');
  holdNext = true;
  // The page drives history and metadata every five seconds. Hold the next
  // history response at the pre-completion snapshot while metadata completes.
  await expect.poll(() => held, {timeout:15000}).toBe(true);
  terminal = true;
  await expect(page.locator('.inbox-history-message > summary').filter({has:page.locator('.thread-message-sender').getByText('Publication', { exact:true })})).toHaveCount(1);
  await expandHistory(page);
  await expect(page.getByLabel('Verified publication')).toContainText(merge.candidate_subject);
  await expect(page.getByLabel('Current job activity')).toHaveCount(0);
  releaseOld();
  await expect(page.getByLabel('Verified publication')).toHaveCount(1);
  await page.getByRole('link',{name:'report.md',exact:true}).click();
  await expect(page.getByRole('heading',{name:'Automatically returned report'})).toBeVisible();
  await page.screenshot({path:info.outputPath('automatic-completion-viewer.png'),animations:'disabled'});
});

test('CV-009 CV-010 CV-013 current activity tail uses conversation and retains stale observation', async ({ page }, info) => {
  page.on('pageerror', error => { console.error('Activity browser error:', error); });
  page.on('crash', () => { console.error('Activity browser crashed'); });
  let status = 'ACTIVE';
  let failed = false;
  await page.route(url => url.pathname === '/api/projects/demo/jobs/job',route=>route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'job',title:'Conversation fixture',status,store:status==='COMPLETED'?'ARCHIVED':'ACTIVE',completion_status:status==='COMPLETED'?'success':undefined}}));
  await page.route('**/conversation?*',route=>failed ? route.fulfill({status:503,json:{error:'Fixture conversation unavailable'}}) : route.fulfill({json:{tenant_id:'demo',job_id:'job',entries:[],snapshot_token:'snapshot',completeness:'complete',activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status,store:status==='COMPLETED'?'ARCHIVED':'ACTIVE',completion_status:status==='COMPLETED'?'success':undefined}},final_documents:{availability:'available',value:{job_id:'job',state:status==='COMPLETED'?'empty':'pending'}}}}));
  await page.goto('/project/demo/jobs/job/story');
  await expect(page.getByRole('heading', {name:'Conversation fixture'})).toBeVisible();
  const tail = page.getByLabel('Current job activity');
  await expect(tail).toHaveCount(1);
  await expect(tail).toContainText('Running');
  await page.screenshot({path:info.outputPath('activity-running-desktop.png'),fullPage:true});
  status = 'PENDING_JOBS';
  await (await storyAction(page, 'Refresh')).click();
  await expect(tail).toContainText('Pending');
  await expect(tail).toContainText('Waiting');
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:info.outputPath('activity-pending-phone.png'),fullPage:true});
  failed = true;
  await (await storyAction(page, 'Refresh')).click();
  await expect(tail).toContainText('Status stale');
  await expect(tail).toContainText('Pending');
  await expect(tail).toHaveCount(1);
  failed = false; status = 'COMPLETED';
  await (await storyAction(page, 'Refresh')).click();
  await expect(tail).toHaveCount(0);
});

for(const width of [1280,390]) test(`CV-002 CV-020 CV-022 terminal publications and returned documents use scoped previews at ${width}px`, async ({page},info)=>{
 await page.setViewportSize({width,height:900});
 const entry={entry_id:'one',kind:'review',state:'answered',title:{present:true,raw:'Review changes'},summary:{present:false},requested_at:{present:false},locator:{invocation_id:'input'},form_fields:{},accepted_response:{present:true,raw:'Approved'},actor:{provenance:'automation_receipt',legacy_user_id:{present:false}}};
 const merges=[{entry_id:'merge',tenant_id:'demo',job_id:'job',invocation_id:'merge-invocation',job_attempt:1,task_attempt:1,result_ordinal:2,candidate_sha:'abc123',candidate_subject:'Add conversation publication cards',publication_branch:'refs/heads/main',publication_disposition:'published',evidence_availability:'recorded'}];
 const files=[{alias:'report.md',resolution:'stored',producer:{jobId:'grandchild',taskOrdinal:4,name:'original.md'}},{alias:'data.json',resolution:'stored',producer:{jobId:'child',taskOrdinal:8,name:'original.json'}},{alias:'outside',resolution:'external'}];
 await page.route('**/conversation?*',r=>r.fulfill({json:{tenant_id:'demo',job_id:'job',entries:[entry],merges,events:[{entry_id:'one',kind:'request_generation',ordinal:1},{entry_id:'merge',kind:'merge_completion',ordinal:2},{entry_id:'one',kind:'response_acceptance',ordinal:3}],snapshot_token:'frozen',completeness:'complete',publication_coverage:'recorded',activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'COMPLETED',store:'ARCHIVED',completion_status:'success'}},final_documents:{availability:'available',value:{job_id:'job',attempt_ordinal:9,state:'present',manifest_id:'manifest-one',documents:files}}}}));
 const forbidden:string[]=[];
 page.on('request',request=>{if(request.url().includes('/conversation/final-documents'))forbidden.push(request.url());});
 const reads:string[]=[];
 await page.route('**/final-documents/document?*',r=>{const url=new URL(r.request().url());reads.push(url.search);expect(url.searchParams.get('manifest_id')).toBe('manifest-one');return r.fulfill({contentType:'application/octet-stream',body:url.searchParams.get('alias')==='report.md'?'# Returned report\n\n[Data](data.json)':'{"retained":true}'});});
 await page.goto('/project/demo/jobs/job/story');
 await expect(page.locator('.inbox-history-message > summary').filter({has:page.locator('.thread-message-sender').getByText('Publication', { exact:true })})).toHaveCount(1);
 await expandHistory(page);
 await expect(page.getByLabel('Verified publication')).toContainText('Add conversation publication cards');
 const link=page.getByRole('link',{name:'report.md',exact:true});await link.click();
 const dialog=page.getByRole('dialog');await expect(dialog.getByRole('heading',{name:'Returned report'})).toBeVisible();
 await dialog.getByRole('link',{name:'Data',exact:true}).click();
 await expect(dialog.getByLabel('Formatted JSON')).toContainText('"retained": true');
 await expect(dialog.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/manifest_id=manifest-one&alias=data.json/);
 await page.screenshot({path:info.outputPath(`returned-document-preview-${width}.png`),fullPage:true});
 await dialog.getByRole('button',{name:'Close',exact:true}).click();await expect(link).toBeFocused();
 expect(reads.length).toBeGreaterThanOrEqual(2);
 expect(forbidden).toEqual([]);
 await page.screenshot({path:info.outputPath(`publications-and-returned-documents-${width}.png`),fullPage:true});
});

function terminalConversation(manifest:any) {
 return {tenant_id:'demo',job_id:'job',entries:[],snapshot_token:'s',completeness:'complete',
 activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'COMPLETED',store:'ARCHIVED',completion_status:'success'}},
 final_documents:{availability:'available',value:manifest}};
}
for(const status of [403,404,410,413]) test(`CV-021 CV-023 returned member ${status} leaves siblings usable`,async({page})=>{
 const files=['bad.md','good.md'].map(alias=>({alias,resolution:'stored',producer:{jobId:'producer',taskOrdinal:4,name:alias}}));
 await page.route('**/conversation?*',r=>r.fulfill({json:terminalConversation({job_id:'job',state:'present',manifest_id:'m',documents:files})}));
 await page.route('**/final-documents/document?*',r=>new URL(r.request().url()).searchParams.get('alias')==='bad.md'?r.fulfill({status,json:{error:'Original bytes unavailable'}}):r.fulfill({body:'# Usable sibling'}));
 await page.goto('/project/demo/jobs/job/story');
 await page.getByRole('link',{name:'bad.md',exact:true}).click();
 const dialog=page.getByRole('dialog');
 await expect(dialog.getByText(/Could not load this document/)).toBeVisible();
 if(status===413)await expect(dialog.getByText(/32 MiB/)).toBeVisible();
 if(status!==410)await expect(dialog.getByText(/expired/i)).toHaveCount(0);
 await dialog.getByRole('button',{name:'Close',exact:true}).click();
 await expect(dialog).toHaveCount(0);
 await page.getByRole('link',{name:'good.md',exact:true}).click();
 await expect(dialog.getByRole('heading',{name:'Usable sibling'})).toBeVisible();
});

test('CV-022 CV-030 changed attempt aborts old preview and ignores late bytes',async({page})=>{
 let attempt=1;
 let release!:()=>void;
 let started!:()=>void;
 const requested=new Promise<void>(r=>{started=r;});
 const held=new Promise<void>(r=>{release=r;});
 const failures:string[]=[];
 page.on('requestfailed',r=>{if(r.url().includes('manifest_id=m1'))failures.push(r.url());});
 await page.route('**/conversation?*',r=>r.fulfill({json:terminalConversation({job_id:'job',attempt_ordinal:attempt,state:'present',manifest_id:`m${attempt}`,documents:[{alias:'report.md',resolution:'stored',producer:{jobId:'producer',taskOrdinal:attempt,name:'report.md'}}]})}));
 await page.route('**/final-documents/document?*',async r=>{
  if(new URL(r.request().url()).searchParams.get('manifest_id')==='m1'){started();await held;await r.fulfill({body:'# Obsolete bytes'}).catch(()=>{});}
  else await r.fulfill({body:'# Current bytes'});
 });
 await page.goto('/project/demo/jobs/job/story');
 await page.getByRole('link',{name:'report.md',exact:true}).click();await requested;
 attempt=2;
 await page.locator('.inbox-story-more button').filter({hasText:/^Refresh$/}).evaluate(el=>(el as HTMLButtonElement).click());
 await expect(page.getByRole('dialog')).toHaveCount(0);
 release();
 await page.getByRole('link',{name:'report.md',exact:true}).click();
 await expect(page.getByRole('dialog').getByRole('heading',{name:'Current bytes'})).toBeVisible();
 await expect(page.getByText('Obsolete bytes')).toHaveCount(0);
 await expect.poll(()=>failures.length).toBeGreaterThan(0);
});

test('CV-028 thread refresh uses the existing observation without a document listing',async({page})=>{
 let available=false;
 let conversations=0;
 const statusReads:string[]=[],listReads:string[]=[];
 page.on('request',r=>{const path=new URL(r.url()).pathname;if(path.endsWith('/jobs/job'))statusReads.push(path);if(path.endsWith('/final-documents'))listReads.push(path);});
 await page.route('**/conversation?*',r=>{
  conversations++;
  const body=terminalConversation({job_id:'job',state:'empty'});
  if(!available)body.final_documents={availability:'unavailable'} as any;
  return r.fulfill({json:body});
 });
 await page.goto('/project/demo/jobs/job/story');
 await expect(page.getByRole('region', { name: 'Job completion' })).toBeVisible();
 await expect(page.getByLabel('Attachments', { exact: true })).toHaveCount(0);
 await page.waitForLoadState('networkidle');
 const initial=statusReads.length;
 const initialConversations=conversations;
 await (await storyAction(page, 'Refresh')).click();
 await expect.poll(()=>conversations).toBe(initialConversations+1);
 available=true;
 await (await storyAction(page, 'Refresh')).click();
 await expect.poll(()=>conversations).toBe(initialConversations+2);
 await expect(page.getByLabel('Attachments', { exact: true })).toHaveCount(0);
 expect(statusReads.length).toBe(initial+2);
 expect(listReads).toEqual([]);
});

test('CV-023 returned HTML stays sandboxed and unsupported formats retain downloads',async({page})=>{
 const aliases=['attack.html','archive.zip'];
 const docs=aliases.map(alias=>({alias,resolution:'stored',producer:{jobId:'p',taskOrdinal:4,name:alias}}));
 const probes:string[]=[];
 page.on('request',r=>{if(r.url().includes('/probe-'))probes.push(r.url());});
 await page.route('**/conversation?*',r=>r.fulfill({json:terminalConversation({job_id:'job',state:'present',manifest_id:'safe',documents:docs})}));
 await page.route('**/final-documents/document?*',r=>r.fulfill({body:new URL(r.request().url()).searchParams.get('alias')==='attack.html'?'<h1 id="safe">Safe retained HTML</h1><script>fetch("/probe-script")</script><img src="/probe-image">':Buffer.from([80,75,3,4])}));
 await page.goto('/project/demo/jobs/job/story');
 await page.getByRole('link',{name:'attack.html',exact:true}).click();
 const dialog=page.getByRole('dialog');
 await expect(dialog.locator('iframe')).toHaveAttribute('sandbox','allow-scripts');
 await expect(page.frameLocator('iframe').locator('#safe')).toBeVisible();
 expect(probes).toEqual([]);
 await expect(dialog.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/manifest_id=safe&alias=attack.html/);
 await dialog.getByRole('button',{name:'Close',exact:true}).click();
 await expect(dialog).toHaveCount(0);
 await page.getByRole('link',{name:'archive.zip',exact:true}).click();
 await expect(dialog.getByRole('link',{name:'Download',exact:true})).toHaveAttribute('href',/manifest_id=safe&alias=archive.zip/);
 await expect(dialog.locator('iframe')).toHaveCount(0);
});

test('CV-028 polling uses the complete conversation without a separate job-status read',async({page})=>{
 let conversations=0;
 const statusReads:string[]=[];
 page.on('request',r=>{if(new URL(r.url()).pathname.endsWith('/jobs/job'))statusReads.push(r.url());});
 await page.route('**/conversation?*',r=>{conversations++;return r.fulfill({json:{...terminalConversation({job_id:'job',state:'pending'}),current_input:null,input_artifacts:[],activity:{availability:'available',value:{...jobIdentity,tenant_id:'demo',job_id:'job',status:'ACTIVE',store:'ACTIVE'}}}});});
 await page.goto('/project/demo/jobs/job/story');
 await expect(page.getByLabel('Current job activity')).toContainText('Running');
 await page.waitForLoadState('networkidle');
 expect(statusReads).toHaveLength(0);
 const initialConversation=conversations;
 await expect.poll(()=>conversations,{timeout:10000}).toBeGreaterThan(initialConversation);
 expect(statusReads).toHaveLength(0);
 await expect(page.getByLabel('Current job activity')).toContainText('Running');
});

for (const width of [1440, 390]) test(`JS-001,002,026–028 safe disclosed prompt and compact actions at ${width}px`, async ({page}, info) => {
  await page.setViewportSize({width,height:900});
  const prompt='# Release plan\n\n'+('A complete paragraph explaining the requested work. '.repeat(20))+'\n\n- Preserve history\n- Keep retries\n\n```text\n'+('long-token-'.repeat(80))+'\n```\n\n<img src="https://bad.example/raw.png">\n\n![remote](https://bad.example/remote.png) [unsafe](javascript:alert(1))\n\n## Ending content';
  const remote:string[]=[]; page.on('request',request=>{if(request.url().includes('bad.example'))remote.push(request.url());});
  await page.route(url => url.pathname === '/api/projects/demo/jobs/job',route=>route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'job',goal_id:'goal',title:'Review a complete saved prompt '+ 'long-title-'.repeat(12),prompt,status:'RUNNING',store:'ACTIVE'}}));
  await page.goto('/project/demo/jobs/job/story');
  await originalRequest(page);
  await expect(page.getByRole('heading',{name:'Ending content'})).toBeVisible();
  await storyAction(page, 'Restart');
  const actions=page.getByRole('group',{name:'Job actions'});
  const boxes=await actions.locator(':scope > *').evaluateAll(nodes=>nodes.map(node=>{const r=node.getBoundingClientRect();return {top:r.top,height:r.height};}));
  expect(boxes.every(box=>box.height>0)).toBe(true);
  for(const label of ['Restart','Archive','Cancel job']){
    const button=actions.getByRole('button',{name:label,exact:true}); await button.focus(); await expect(button).toBeFocused();
    const box=await button.boundingBox(); expect(box!.height).toBeGreaterThanOrEqual(width <= 767 ? 44 : 32); expect(box!.x).toBeGreaterThanOrEqual(0); expect(box!.x+box!.width).toBeLessThanOrEqual(width);
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth)).toBeLessThanOrEqual(1);
  await expect(page.getByRole('region',{name:'Job prompt'}).locator('img,script')).toHaveCount(0);
  expect(remote).toEqual([]);
  await expect(page.getByRole('button',{name:/Back to Goals|Download chapters/})).toHaveCount(0);
  await actions.evaluate(element=>element.scrollLeft=0);
  await page.screenshot({path:info.outputPath(`prompt-actions-${width}.png`),fullPage:true,animations:'disabled'});
  if (process.env.CORTEX_INBOX_REPAIR_CAPTURE) await page.screenshot({path:`${process.env.CORTEX_INBOX_REPAIR_CAPTURE}/inbox-actions-${width === 390 ? 'phone' : 'desktop'}.png`,animations:'disabled'});
});

test('JS-015,020–023 AR-010,011 Archive retries retain cancellation and return to inbox', async ({page}, info) => {
  let deletes=0, storyReads=0;
  const payloads:unknown[]=[];
  await page.route(url => url.pathname === '/api/projects/demo/jobs/job',async route=>{
    if(route.request().method()==='DELETE'){
      deletes++;payloads.push(route.request().postDataJSON());
      return deletes===1?route.fulfill({status:503,json:{error:'Archive failed after cancellation',code:'archive_failed_after_cancel'}}):route.fulfill({status:204});
    }
    return route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'job',title:'Action retry fixture',prompt:'Inspect history after archive.',status:'RUNNING',store:deletes>1?'ARCHIVED':'ACTIVE'}});
  });
  await page.route('**/api/projects/demo/jobs/job/story',route=>{storyReads++;return route.fulfill({json:{job_id:'job',status:'completed',restart_scopes:[{scope_id:'recorded',chapter_ordinal:3,title:'Recorded restart'}],root:null}});});
  await page.route('**/api/projects/demo/jobs?*',route=>route.fulfill({json:{jobs:[]}}));
  await page.goto('/project/demo/jobs/job/story');
  const archive=await storyAction(page, 'Archive'); await archive.click();
  const dialog=page.getByRole('dialog'); await dialog.getByLabel('Cancellation message (optional)').fill('Retain my cancellation');
  await dialog.getByRole('button',{name:'Archive job',exact:true}).click();
  await expect(dialog.getByText(/only archiving remains/)).toBeVisible();
  await expect(dialog.getByLabel('Cancellation message (optional)')).toBeDisabled();
  await expect(page).toHaveURL(/\/jobs\/job\/story$/);
  await expect(page.getByText('Archived 1 job',{exact:true})).toHaveCount(0);
  await page.screenshot({path:info.outputPath('archive-retry.png'),animations:'disabled'});
  await dialog.getByRole('button',{name:'Archive job',exact:true}).click();
  await expect(dialog).toHaveCount(0);
  await expect(page).toHaveURL('/project/demo/jobs');
  await expect(page.getByRole('heading',{name:'All',exact:true})).toBeVisible();
  const toast=page.locator('.ant-message-notice-content').filter({hasText:'Archived 1 job'});
  await expect(toast).toHaveCount(1);
  await expect(toast).toBeVisible();
  await expect(archive).toHaveCount(0);
  expect(payloads).toEqual([{reason:'Retain my cancellation'},{reason:'Retain my cancellation'}]);
  expect(storyReads).toBe(0);
});

test('JS-010,036,037 header Restart submits before Trace and retries the saved request', async ({page}, info) => {
  let storyReads=0, restarts=0;
  const payloads:unknown[]=[];
  await page.route(url => url.pathname === '/api/projects/demo/jobs/job',route=>route.fulfill({json:{...jobIdentity,tenant_id:'demo',job_id:'job',title:'Restart fixture',prompt:'Restart before loading Trace.',status:'RUNNING',store:'ACTIVE'}}));
  await page.route('**/api/projects/demo/jobs/job/story',route=>{storyReads++;return route.fulfill({json:{job_id:'job',status:'completed',restart_scopes:[{scope_id:'recorded',chapter_ordinal:3,title:'Recorded restart'}],root:null}});});
  await page.route('**/api/projects/demo/jobs/job/restart',route=>{
    restarts++;payloads.push(route.request().postDataJSON());
    return restarts===1?route.fulfill({status:503,body:'Temporary restart failure'}):route.fulfill({json:{job_id:'restarted'}});
  });
  await page.goto('/project/demo/jobs/job/story');
  expect(storyReads).toBe(0);
  const restart=await storyAction(page, 'Restart'); await restart.click(); const dialog=page.getByRole('dialog');
  await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeDisabled();
  await expect(page.locator('#execution-story-tree')).toHaveCount(0);
  await dialog.getByRole('combobox',{name:'Restart point'}).click(); await page.getByText(/Recorded restart · Chapter 3/).click();
  const goalMode=dialog.locator('.ant-select').filter({has:page.getByRole('combobox',{name:'Restart goal'})});
  await expect(goalMode).toHaveText('Start a new goal');
  await expect(dialog.getByRole('button',{name:'Start a new goal',exact:true})).toBeEnabled();
  await expect(page.getByRole('listbox')).toBeHidden();
  await page.screenshot({path:info.outputPath('header-restart.png'),animations:'disabled'});
  await dialog.getByRole('button',{name:'Restart',exact:true}).click();
  await expect(page.getByText(/Temporary restart failure/)).toBeVisible();
  await expect(goalMode).toHaveText('Start a new goal');
  expect(restarts).toBe(1);
  await dialog.getByRole('button',{name:'Retry saved request',exact:true}).click();
  await expect(page).toHaveURL(/\/jobs\/restarted\/story/);
  expect(payloads[0]).toEqual(payloads[1]);
  expect(payloads[0]).toMatchObject({scope_id:'recorded',goal:{mode:'new'},request_id:expect.any(String)});
  expect(storyReads).toBe(1);
});

for(const width of [1440,390]) test(`JS-004,005,010,036 summary deep link and shared Restart at ${width}px`,async({page},info)=>{
  await page.setViewportSize({width,height:900});
  await page.route('**/api/projects/demo/jobs/job/story',route=>route.fulfill({json:{job_id:'job',status:'completed',recipe:{name:'release-checks',source:'fixture.yaml'},started_at:'2026-11-01T10:00:00Z',finished_at:'2026-11-01T10:02:15Z',invocation_sequence:42,restart_scopes:[{scope_id:'recorded',chapter_ordinal:3,title:'Recorded restart'}],root:null}}));
  await page.goto('/project/demo/jobs/job/story?tab=summary');
  await expect(page.getByRole('tab',{name:'Trace',exact:true})).toHaveAttribute('aria-selected','true');
  await expect(page.getByText('release-checks',{exact:true})).toBeVisible();
  await expect(page.getByText('2m 15s',{exact:true})).toBeVisible();
  await expect(page.getByText('42',{exact:true})).toBeVisible();
  await expect(page.getByText('The story API returned no steps for this finished job.')).toBeVisible();
  await page.screenshot({path:info.outputPath(`trace-summary-${width}.png`),fullPage:true,animations:'disabled'});
  for(const tab of ['Thread','Execution','Graph','Trace']){
    await page.getByRole('tab',{name:tab,exact:true}).click();
    const opener=await storyAction(page, 'Restart');
    await opener.click();
    const dialog=page.getByRole('dialog');
    await expect(dialog.getByRole('combobox',{name:'Restart point'})).toBeVisible();
    await expect(dialog.getByRole('button',{name:'Restart',exact:true})).toBeDisabled();
    await expect.poll(()=>dialog.evaluate(element=>element.contains(document.activeElement))).toBe(true);
    await page.keyboard.press('Escape');
    await expect(dialog).toHaveCount(0); await expect(page.locator('.inbox-story-more > summary')).toBeFocused();
    await expect(page.locator('.inbox-story-more')).not.toHaveAttribute('open');
    await expect(page.getByRole('tab',{name:tab,exact:true})).toHaveAttribute('aria-selected','true');
  }
});
