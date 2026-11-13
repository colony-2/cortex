import { test, expect, type APIRequestContext } from '@playwright/test';
import { createHash, randomUUID } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import type { AssistantSession, AssistantStart } from '@colony2/shared';

const base = '/api/projects/assistant-browser/assistant';
const control = '/api/__test/assistant/';
const origin = { Origin: 'http://127.0.0.1:15176' };
const hash = (data: Buffer | string) => createHash('sha256').update(data).digest('hex');
const generated = '# Recorded discussion\n\nretained-download-marker 世界\n';
async function audit(request: APIRequestContext, job: string) {
 let value:any;
 await expect.poll(async()=>{const r=await request.get(control+`audit?job=${job}`);if(r.status()===409)return false;expect(r.ok(),await r.text()).toBeTruthy();value=await r.json();return value.worker_log_complete;},{timeout:120_000}).toBe(true);
 return value;
}
async function command(request: APIRequestContext, action: string) {
 const r = await request.post(control + action); expect(r.status(), await r.text()).toBe(204);
}
async function get(request: APIRequestContext, id: string): Promise<AssistantSession> {
 const r = await request.get(`${base}/sessions/${id}`); expect(r.ok(), await r.text()).toBeTruthy(); return r.json();
}
async function ready(request: APIRequestContext, id: string) {
 await expect.poll(async () => {const r=await request.get(`${base}/sessions/${id}`);return r.ok()?(await r.json()).current_request_id:undefined;}, { timeout: 120_000 }).toBeTruthy(); return get(request, id);
}
async function post(request: APIRequestContext, path: string, data: unknown) { return request.post(base + path, { headers: origin, data }); }
async function generation(request: APIRequestContext) { return (await (await request.get(base + '/capabilities')).json()).generation as string; }
async function start(request: APIRequestContext, prompt = 'Runtime global prompt') {
 const input: AssistantStart = { generation: await generation(request), request_id: randomUUID(), submitted_at: new Date().toISOString(), prompt, source: {kind:'global'} };
 const r = await post(request, '/sessions', input); expect(r.ok(), await r.text()).toBeTruthy(); return { input, session: await r.json() as AssistantSession };
}
async function close(request: APIRequestContext, s: AssistantSession) {
 const inFlight = {request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:s.job_id};
 await expect.poll(async () => {const r=await post(request, `/sessions/${s.id}/close`, inFlight);expect(r.ok(),await r.text()).toBeTruthy();return (await r.json()).status.toLowerCase();},{timeout:60_000}).toMatch(/cancelled|canceled|completed|failed/);
}
async function reopen(request: APIRequestContext,s:AssistantSession) {
 const input={generation:await generation(request),request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:s.job_id};
 const r=await post(request,`/sessions/${s.id}/reopen`,input);expect(r.ok(),await r.text()).toBeTruthy();const v=await r.json() as AssistantSession;
 expect(v.id).toBe(s.id);expect(v.goal_id).toBe(s.goal_id);expect(v.job_id).not.toBe(s.job_id);return {input,session:v};
}

test.describe.configure({mode:'serial'});

test('CTX-023 actual story and exact review flow, source citations, downloads and desktop/mobile captures', async ({page,context,request,baseURL},info)=>{
 await context.addCookies([{name:'colony2:user:email',value:'fixture@example.test',url:baseURL!}]);
 await expect.poll(async()=> (await (await request.get('/api/projects/assistant-browser/user-inputs/pending')).json()).some((p:any)=>p.id==='source'),{timeout:120_000}).toBe(true);
 await page.setViewportSize({width:1365,height:1000});
 await page.goto('/project/assistant-browser/jobs/source/story');
 await page.getByRole('button',{name:'Open assistant'}).click();
 const panel=page.getByRole('dialog',{name:'Contextual assistant'});
 await panel.getByRole('checkbox',{name:'Include this story'}).check();
 await panel.getByRole('textbox',{name:'Message'}).fill('Discuss this actual retained story');
 const started=page.waitForResponse(r=>r.url().endsWith('/assistant/sessions')&&r.request().method()==='POST');
 await panel.getByRole('button',{name:'Send message'}).click();
 const response=await started;expect(response.ok(),await response.text()).toBeTruthy();const story=await response.json() as AssistantSession;
 expect(story.lifecycle).toBe('goal');expect(story.source).toEqual({kind:'story',job_id:'source'});
 await expect(panel.getByLabel('Conversation transcript')).toContainText('Grounded deterministic reply',{timeout:120_000});
 const storyReady=await ready(request,story.id);
 expect(storyReady.coverage?.map(c=>c.id)).toEqual(['story']);
 await expect(panel.getByLabel('Cited sources')).toContainText('story');
 await expect(panel.getByRole('status')).toContainText('Waiting for your message');await panel.getByLabel('Conversation transcript').evaluate(el=>el.scrollTop=el.scrollHeight);await page.screenshot({path:info.outputPath('desktop-story.png')});
 await panel.getByRole('button',{name:'New conversation'}).click();
 await panel.getByRole('button',{name:'Minimize assistant'}).click();
 await page.goto('/project/assistant-browser/reviews');
 // Open the real review, using its actual runtime request rather than a synthetic page selection.
 await page.getByRole('button',{name:'Open Review',exact:true}).first().click();
 await page.getByRole('button',{name:'Open assistant'}).click();
 await expect(panel.getByRole('checkbox',{name:'Include this exact review and its documents'})).toBeVisible();
 await command(request,'expire');
 await panel.getByRole('checkbox',{name:'Include this exact review and its documents'}).check();
 await panel.getByRole('textbox',{name:'Message'}).fill('Discuss the exact documents and their omissions');
 const reviewResponse=page.waitForResponse(r=>r.url().endsWith('/assistant/sessions')&&r.request().method()==='POST');
 await panel.getByRole('button',{name:'Send message'}).click();
 const response2=await reviewResponse;expect(response2.ok(),await response2.text()).toBeTruthy();const review=await response2.json() as AssistantSession;
 const recorded=await ready(request,review.id);
 expect(recorded.source.kind).toBe('review');expect(recorded.source.request_id).toBeTruthy();
 expect(recorded.coverage?.map(c=>[c.id,c.outcome])).toEqual([['a-child','included'],['b-json','included'],['c-large','truncated'],['d-overflow','budget_omitted'],['e-picture','unsupported'],['f-unavailable','unavailable']]);
 await expect(panel.getByLabel('Conversation transcript')).toContainText('Grounded deterministic reply',{timeout:120_000});
 await panel.getByText('Source coverage (6)',{exact:true}).click();
 const cited=panel.getByRole('link',{name:'Cited source: evidence.md'});await expect(cited).toBeVisible();
 const [download]=await Promise.all([page.waitForEvent('download'),cited.click()]);const bytes=await readFile((await download.path())!);
 expect(bytes.toString()).toBe('Child evidence marker 世界 retained-original-20261112\n');
 const source=recorded.coverage!.find(c=>c.id==='a-child')!;
 const original=await request.get(`${control}original?job=${source.producer_job_id}&task=${source.producer_task}&name=evidence.md`);expect(original.ok()).toBeTruthy();expect(hash(bytes)).toBe(hash(await original.body()));
 const reply=recorded.replay.entries.find(e=>e.role==='assistant')!;expect(reply.origin?.job_id).toBe(recorded.job_id);expect(reply.origin?.invocation).toBeTruthy();expect(reply.origin?.attempt).toBe(1);
 const artifact=reply.artifacts!.find(a=>a.name==='discussion.md')!;expect(artifact.origin?.job_id).toBe(recorded.job_id);expect(artifact.origin?.task).toBe(reply.origin?.task);
 const generatedRead=await request.get(`${base}/sessions/${review.id}/artifacts/${artifact.id}`);expect(generatedRead.ok()).toBeTruthy();expect(hash(await generatedRead.body())).toBe(hash(generated));
 const originalReply=await request.get(`${control}original?job=${artifact.origin!.job_id}&task=${artifact.origin!.task}&name=discussion.md`);expect(originalReply.ok()).toBeTruthy();expect(hash(await originalReply.body())).toBe(hash(await generatedRead.body()));
 await panel.evaluate(el=>el.scrollTop=0);await page.screenshot({path:info.outputPath('desktop-review-coverage.png')});
 await panel.getByText('Source coverage (6)',{exact:true}).click();await expect(panel.getByRole('status')).toContainText('Waiting for your message');await panel.evaluate(el=>el.scrollTop=0);await panel.getByText('Grounded deterministic reply from the selected evidence.',{exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:info.outputPath('desktop-review.png')});
 await page.setViewportSize({width:390,height:844});await panel.evaluate(el=>el.scrollTop=0);await panel.getByLabel('Conversation transcript').evaluate(el=>el.scrollTop=el.scrollHeight);await page.screenshot({path:info.outputPath('mobile-review.png')});
 const evidence=await (await request.get(control+'evidence')).json();
 const modelRequests=evidence.requests as any[];
 const storyRequest=modelRequests.find(q=>JSON.stringify(q).includes('Discuss this actual retained story'));expect(storyRequest).toBeTruthy();expect(JSON.stringify(storyRequest)).toContain('assistant-review-source');
 const reviewRequest=modelRequests.find(q=>JSON.stringify(q).includes('Discuss the exact documents'));expect(reviewRequest.model).toBe('fixture');expect(reviewRequest.tools).toHaveLength(1);
 expect(JSON.stringify(reviewRequest)).toContain('retained-original-20261112');expect(JSON.stringify(reviewRequest)).toContain('independent-json-marker');expect(JSON.stringify(reviewRequest)).not.toContain('Expired evidence marker');expect(JSON.stringify(reviewRequest)).not.toContain('unsupported-marker');
 expect(JSON.stringify(reviewRequest)).toContain(source.producer_job_id);
 await writeFile(info.outputPath('story-model-request.json'),JSON.stringify(storyRequest,null,2));await writeFile(info.outputPath('review-model-request.json'),JSON.stringify(reviewRequest,null,2));
 await writeFile(info.outputPath('browser-records.json'),JSON.stringify({story:storyReady,review:recorded,download_sha256:hash(bytes),generated_sha256:hash(await generatedRead.body())},null,2));
 await writeFile(info.outputPath('release-provenance.json'),await (await request.get(control+'provenance')).body());
 for(const s of [storyReady,recorded]){const report=await audit(request,s.job_id);expect(report.retained_artifacts_scanned).toBeGreaterThan(0);expect(report.execution_records_scanned).toBeGreaterThan(0);expect(report.retention_secret_locations).toEqual([]);expect(report.inline_history_clean).toBe(true);expect(report.worker_log_secret_occurrences).toBe(0);await writeFile(info.outputPath(`audit-${s.source.kind}.json`),JSON.stringify(report,null,2));}
 // Change the real pending review. Reusing its old selection is rejected before inference.
 const reviewed=await request.post('/api/projects/assistant-browser/reviews/source/respond',{headers:{...origin,Cookie:'colony2:user:email=fixture%40example.test'},data:{request_id:recorded.source.request_id,submission_id:randomUUID(),fields:{decision:'continue to replacement fixture'}}});expect(reviewed.ok(),await reviewed.text()).toBeTruthy();
 await expect.poll(async()=>{const pending=await(await request.get('/api/projects/assistant-browser/user-inputs/pending')).json();const id=pending.find((v:any)=>v.id==='source')?.request_id;return !!id&&id!==recorded.source.request_id;},{timeout:120_000}).toBe(true);
 const changed=await post(request,'/sessions',{generation:await generation(request),request_id:randomUUID(),submitted_at:new Date().toISOString(),prompt:'Must not discuss replacement automatically',source:recorded.source});expect(changed.status()).toBe(409);
 await command(request,'replace');const retained=await get(request,recorded.id);expect(retained.source).toEqual(recorded.source);expect(retained.coverage).toEqual(recorded.coverage);expect(retained.replay.entries.find(e=>e.key===reply.key)?.origin).toEqual(reply.origin);
 const recoveredDownload=await request.get(`${base}/sessions/${review.id}/artifacts/${artifact.id}`);expect(recoveredDownload.ok()).toBeTruthy();expect(hash(await recoveredDownload.body())).toBe(hash(generated));
 await writeFile(info.outputPath('review-replacement-recovery.json'),JSON.stringify({stale_capture_status:changed.status(),recorded_source:retained.source,original_artifact:artifact,recovered_download_sha256:hash(await recoveredDownload.body())},null,2));
 await page.reload();await page.getByRole('button',{name:'Open assistant'}).click();await panel.getByRole('button',{name:'Conversation history'}).click();
 await panel.getByRole('navigation',{name:'Conversation history'}).getByRole('button',{name:`Review source · request ${recorded.source.request_id} · running`,exact:true}).click();
 await expect(panel.getByLabel('Conversation transcript')).toContainText('Grounded deterministic reply');await expect(panel.getByLabel('Captured context')).toContainText(recorded.source.request_id!);
 await context.setOffline(true);await context.setOffline(false);await expect(panel.getByLabel('Conversation transcript')).toContainText('Grounded deterministic reply');
 await close(request,storyReady);await close(request,recorded);
});

test('CTX-012,013,018,024 retained retries, concurrent sends, sequential reopen and known recovery',async({request},info)=>{
 const initial=await start(request,'Global context must contain no page source marker');let s=await ready(request,initial.session.id);
 const evidence=await(await request.get(control+'evidence')).json();const q=evidence.requests.find((q:any)=>JSON.stringify(q).includes('Global context must contain'));expect(JSON.stringify(q)).not.toContain('SOURCE ');expect(JSON.stringify(q)).not.toContain('retained-original-20261112');
 const input={request_id:s.current_request_id,submission_id:randomUUID(),submitted_at:new Date().toISOString(),command:'send',prompt:'One accepted concurrent continuation'};
 const sends=await Promise.all([post(request,`/sessions/${s.id}/respond`,input),post(request,`/sessions/${s.id}/respond`,{...input,submission_id:randomUUID(),prompt:'Competing concurrent continuation'})]);
 const statuses=sends.map(r=>r.status()).sort();expect(statuses).toEqual([200,409]);const accepted=await sends.find(r=>r.status()===200)!.json();expect(accepted.request_id).toBe(s.current_request_id);
 await expect.poll(async()=>{s=await get(request,s.id);return s.current_request_id!==input.request_id&&!!s.current_request_id;},{timeout:120_000}).toBe(true);
 const receipt=s.replay.entries.find(e=>e.receipt?.submission_id===accepted.submission_id)?.receipt;expect(receipt?.request_id).toBe(input.request_id);
 // Lose a successful close response, then reconcile it through the ordinary reopen path.
 await command(request,'lose');const uncertainClose=await post(request,`/sessions/${s.id}/close`,{request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:s.job_id});expect(uncertainClose.status()).toBe(503);
 const before=(await(await request.get(control+'evidence')).json()).requests.length;
 const opened=await reopen(request,s);s=await ready(request,s.id);expect(s.job_id).toBe(opened.session.job_id);
 expect((await(await request.get(control+'evidence')).json()).requests.length).toBe(before);expect(s.replay.entries.some(e=>e.receipt?.submission_id===accepted.submission_id)).toBe(true);
 const retry=await post(request,`/sessions/${s.id}/reopen`,opened.input);expect(retry.ok(),await retry.text()).toBeTruthy();expect((await retry.json()).job_id).toBe(s.job_id);
 // A delayed close addresses the original job, and cannot cancel its replacement.
 const delayed=await post(request,`/sessions/${s.id}/close`,{request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:initial.session.job_id});expect(delayed.ok()).toBeTruthy();expect((await get(request,s.id)).job_id).toBe(s.job_id);expect((await get(request,s.id)).current_request_id).toBeTruthy();
 await command(request,'replace');s=await get(request,s.id);expect(s.current_request_id).toBeTruthy();expect(s.replay.entries.some(e=>e.origin?.job_id===initial.session.job_id)).toBe(true);
 await close(request,s);const later=await reopen(request,s);s=await ready(request,s.id);expect(s.job_id).toBe(later.session.job_id);expect(s.id).toBe(initial.session.id);
 // Lost Reopen response retries the original request, even after its new job is prompt-ready.
 await close(request,s);await command(request,'lose');const lostReopen={generation:await generation(request),request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:s.job_id};
 expect((await post(request,`/sessions/${s.id}/reopen`,lostReopen)).status()).toBe(503);
 const reconciled=await post(request,`/sessions/${s.id}/reopen`,lostReopen);expect(reconciled.ok(),await reconciled.text()).toBeTruthy();const acceptedReopen=await reconciled.json() as AssistantSession;s=await ready(request,s.id);expect(s.job_id).toBe(acceptedReopen.job_id);
 const stableAgain=await post(request,`/sessions/${s.id}/reopen`,lostReopen);expect((await stableAgain.json()).job_id).toBe(s.job_id);
 // Lost original bytes refuse to prepare or submit another successor. Known history still works.
 await close(request,s);await command(request,'lose');const missingBytes={generation:await generation(request),request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:s.job_id};
 expect((await post(request,`/sessions/${s.id}/reopen`,missingBytes)).status()).toBe(503);const actual=await ready(request,s.id);
 await command(request,'replace');const unresolved=await post(request,`/sessions/${s.id}/reopen`,missingBytes);expect(unresolved.status()).toBe(409);expect((await unresolved.json()).code).toBe('unresolved');
 s=await get(request,s.id);expect(s.job_id).toBe(actual.job_id);expect(s.current_request_id).toBe(actual.current_request_id);
 // Replay pages retain original cross-job identities and remain stable across server replacement.
 let cursor='';const entries:any[]=[];const cursors:string[]=[];
 do {const r=await request.get(`${base}/sessions/${s.id}/replay?limit=2${cursor?'&cursor='+encodeURIComponent(cursor):''}`);expect(r.ok(),await r.text()).toBeTruthy();const page=await r.json();expect(page.entries.length).toBeLessThanOrEqual(2);entries.push(...page.entries);cursor=page.cursor||'';if(cursor){expect(cursors).not.toContain(cursor);cursors.push(cursor);}}while(cursor);
 expect(entries.some(e=>e.origin?.job_id===initial.session.job_id)).toBe(true);expect(new Set(entries.map(e=>e.key)).size).toBe(entries.length);
 const firstPage=await(await request.get(`${base}/sessions/${s.id}/replay?limit=1`)).json();expect(firstPage.cursor).toBeTruthy();const stableURL=`${base}/sessions/${s.id}/replay?limit=1&cursor=${encodeURIComponent(firstPage.cursor)}`;
 const continuation=await(await request.get(stableURL)).json();await command(request,'replace');expect(await(await request.get(stableURL)).json()).toEqual(continuation);
 const workerRuns=(await(await request.get(control+'evidence')).json()).worker_executions.filter((v:any)=>[initial.session.job_id,opened.session.job_id,s.job_id].includes(v.job_id));expect(new Set(workerRuns.map((v:any)=>v.pid)).size).toBeGreaterThan(1);
 await writeFile(info.outputPath('worker-replacement.json'),JSON.stringify(workerRuns,null,2));
 const unrelated=await(await request.post(control+`unrelated?goal=${s.goal_id}`)).json();const grouped=await(await request.get(base+'/sessions')).json();expect(grouped.sessions.filter((v:any)=>v.id===s.id)).toHaveLength(1);expect(JSON.stringify(grouped)).not.toContain(unrelated.job_id);
 await writeFile(info.outputPath('lifecycle.json'),JSON.stringify({initial:initial.session,concurrent_statuses:statuses,receipt,first_reopen:opened.session,after_replacement:s,accepted_lost_reopen:acceptedReopen,unresolved_reopen:missingBytes,paged_entries:entries,unrelated_job:unrelated},null,2));await close(request,s);
 // Explicit same-process accepted-but-lost HTTP response must retry frozen bytes.
 await command(request,'lose');const frozen:AssistantStart={generation:await generation(request),request_id:randomUUID(),submitted_at:new Date().toISOString(),prompt:'Lost response frozen request',source:{kind:'global'}};
 const lost=await post(request,'/sessions',frozen);expect(lost.status()).toBe(503);
 const recovered=await post(request,'/sessions',frozen);expect(recovered.ok(),await recovered.text()).toBeTruthy();const known=await recovered.json() as AssistantSession;
 const duplicate=await post(request,'/sessions',frozen);expect((await duplicate.json()).job_id).toBe(known.job_id);
 const conflict=await post(request,'/sessions',{...frozen,prompt:'conflicting reuse'});expect(conflict.status()).toBe(409);
 const retryAudit=await audit(request,known.job_id);expect(retryAudit.retention_secret_locations).toEqual([]);await writeFile(info.outputPath('frozen-retry.json'),JSON.stringify({frozen,known,audit:retryAudit},null,2));await close(request,known);
});

test('CTX-017 unresolved lost bytes stays visible while actual history remains selectable',async({page,context,request,baseURL},info)=>{
 await context.addCookies([{name:'colony2:user:email',value:'fixture@example.test',url:baseURL!}]);
 await page.goto('/project/assistant-browser/jobs/source/story');await page.getByRole('button',{name:'Open assistant'}).click();
 const panel=page.getByRole('dialog',{name:'Contextual assistant'});await panel.getByRole('textbox',{name:'Message'}).fill('Draft kept after uncertain start');
 await command(request,'lose');await panel.getByRole('button',{name:'Send message'}).click();await expect(panel.getByRole('button',{name:'Retry same message'})).toBeEnabled();
 await command(request,'replace');const before=(await(await request.get(control+'evidence')).json()).requests.length;
 await panel.getByRole('button',{name:'Retry same message'}).click();await expect(panel.getByRole('alert')).toContainText('may have succeeded');await expect(panel.getByRole('alert')).toContainText('Automatic recovery is unavailable');await expect(panel.getByRole('button',{name:'Retry same message'})).toBeDisabled();await expect(panel.getByRole('textbox',{name:'Message'})).toHaveValue('Draft kept after uncertain start');
 await page.setViewportSize({width:1365,height:1000});await panel.screenshot({path:info.outputPath('desktop-unresolved.png')});
 await panel.getByRole('button',{name:'Conversation history'}).click();await expect(panel.getByRole('navigation',{name:'Conversation history'})).toBeVisible();
 await panel.getByRole('navigation',{name:'Conversation history'}).getByRole('button').first().click();await expect(panel).toContainText('earlier request remains unresolved');
 await page.setViewportSize({width:390,height:844});await panel.screenshot({path:info.outputPath('mobile-history-recovery.png')});
 const after=(await(await request.get(control+'evidence')).json()).requests.length;expect(after-before).toBeLessThanOrEqual(1); // Only the already accepted original may finish.
 await writeFile(info.outputPath('unresolved-boundary.json'),JSON.stringify({provider_count_before:before,provider_count_after:after,automatic_reconciliation:'human-deferred',safe_presentation:'verified',history_selection:'does not identify the receipt'},null,2));
});


test('CTX-006,018 queued/processing/waiting cancellation, completion and send races, expired history and legacy identity',async({request,page,context,baseURL},info)=>{
 const cases:any[]=[];await command(request,'success');await command(request,'release');
 // Unresolved accepted work from the preceding case may finish legitimately. Count only this operation's model requests.
 const evidence=async(marker:string)=> (await(await request.get(control+'evidence')).json()).requests.filter((q:any)=>JSON.stringify(q).includes(marker)).length as number;
 await command(request,'pause');let before=await evidence('Queued cancellation must not infer');const queued=await start(request,'Queued cancellation must not infer');expect(queued.session.status).toMatch(/submitted|queued|running/i);await close(request,queued.session);await command(request,'resume');expect(await evidence('Queued cancellation must not infer')).toBe(before);cases.push({case:'queued',session:await get(request,queued.session.id),model_calls:0});
 // A provider exchange proves processing outside the input wait. Cancel its captured job.
 await command(request,'hold');before=await evidence('Processing cancellation marker');const processing=await start(request,'Processing cancellation marker');await expect.poll(()=>evidence('Processing cancellation marker'),{timeout:120_000}).toBe(before+1);await close(request,processing.session);await command(request,'release');cases.push({case:'processing',session:await get(request,processing.session.id)});
 let current=(await reopen(request,await get(request,processing.session.id))).session;current=await ready(request,current.id);expect(await evidence('Processing cancellation marker')).toBe(before+1);
 const inFlight={request_id:current.current_request_id,submission_id:randomUUID(),submitted_at:new Date().toISOString(),command:'send',prompt:'Send close race marker'};
 const [sent,closed]=await Promise.all([post(request,`/sessions/${current.id}/respond`,inFlight),post(request,`/sessions/${current.id}/close`,{request_id:randomUUID(),submitted_at:new Date().toISOString(),job_id:current.job_id})]);
 expect([200,409]).toContain(sent.status());expect(closed.ok(),await closed.text()).toBeTruthy();await close(request,current);
 const terminal=await get(request,current.id);expect(terminal.current_request_id).toBeFalsy();const receipt=sent.ok()?await sent.json():null;
 if(receipt){const retry=await post(request,`/sessions/${current.id}/respond`,inFlight);expect(retry.ok(),await retry.text()).toBeTruthy();expect((await retry.json()).submission_id).toBe(receipt.submission_id);}
 cases.push({case:'send-close',send_status:sent.status(),receipt,session:terminal});
 // Let inference complete while cancellation arrives. Only the recorded terminal state wins.
 await command(request,'hold');before=await evidence('Completion close race marker');const finishing=await start(request,'Completion close race marker');await expect.poll(()=>evidence('Completion close race marker'),{timeout:120_000}).toBe(before+1);
 await Promise.all([command(request,'release'),close(request,finishing.session)]);const done=await get(request,finishing.session.id);expect(done.current_request_id).toBeFalsy();cases.push({case:'completion-close',session:done});
 const next=await reopen(request,done);const waiting=await ready(request,next.session.id);expect(await evidence('Completion close race marker')).toBe(before+1);await close(request,waiting);
 // Provider failure deliberately contains the synthetic credential. The operation must redact it everywhere.
 await command(request,'error');const failed=await start(request,'Provider failure redaction marker');const failureReply=await ready(request,failed.session.id);expect(failureReply.replay.entries.some(e=>e.role==='assistant'&&e.status==='unavailable'&&!e.text)).toBe(true);await command(request,'success');
 const failAudit=await audit(request,failed.session.job_id);expect(failAudit.failed_execution_records).toBeGreaterThan(0);expect(failAudit.retained_artifacts_scanned).toBeGreaterThan(0);expect(failAudit.retention_secret_locations).toEqual([]);expect(failAudit.inline_history_clean).toBe(true);expect(failAudit.worker_log_secret_occurrences).toBe(0);cases.push({case:'failed-provider',session:await get(request,failed.session.id),audit:failAudit});
 await context.addCookies([{name:'colony2:user:email',value:'fixture@example.test',url:baseURL!}]);await page.goto('/project/assistant-browser/jobs');await page.getByRole('button',{name:'Open assistant'}).click();const failurePanel=page.getByRole('dialog',{name:'Contextual assistant'});await failurePanel.getByRole('button',{name:'Conversation history'}).click();await failurePanel.getByRole('navigation',{name:'Conversation history'}).getByRole('button').first().click();
 await expect(failurePanel.getByRole('link',{name:'View execution'})).toHaveAttribute('href',`/project/assistant-browser/jobs/${failed.session.job_id}/story`);await expect(failurePanel).toContainText('A reply is unavailable. You can send another message.');await page.setViewportSize({width:1365,height:1000});await failurePanel.getByText('A reply is unavailable. You can send another message.',{exact:true}).scrollIntoViewIfNeeded();await expect(failurePanel.getByText('A reply is unavailable. You can send another message.',{exact:true})).toBeVisible();await page.screenshot({path:info.outputPath('desktop-provider-unavailable.png')});
 const denied=await post(request,'/sessions',{generation:await generation(request),request_id:randomUUID(),submitted_at:new Date().toISOString(),prompt:'forged source',source:{kind:'story',job_id:'missing-denied'}});expect([403,409,503]).toContain(denied.status());
 // A configured historical ordinary release keeps its original session ID and read-only reopen policy.
 const legacy=await(await request.post(control+'legacy')).json();const old=await ready(request,legacy.id);expect(old.lifecycle).toBe('legacy');expect(old.id).toBe(legacy.id);expect(old.id).not.toBe(old.goal_id);
 const noRestart=await post(request,`/sessions/${old.id}/reopen`,{});expect(noRestart.status()).toBe(503);
 const finish=await post(request,`/sessions/${old.id}/respond`,{request_id:old.current_request_id,submission_id:randomUUID(),submitted_at:new Date().toISOString(),command:'finish'});expect(finish.ok(),await finish.text()).toBeTruthy();
 const originalEntry=old.replay.entries.find(e=>e.role==='assistant')!;const originalArtifact=originalEntry.artifacts![0];const beforeExpiry=await request.get(`${base}/sessions/${old.id}/artifacts/${originalArtifact.id}`);expect(beforeExpiry.ok()).toBeTruthy();
 await command(request,'expire-reply');await command(request,'replace');const restored=await get(request,old.id);expect(restored.replay.entries.some(e=>e.key===originalEntry.key&&e.text===originalEntry.text)).toBe(true);
 const expired=await request.get(`${base}/sessions/${old.id}/artifacts/${originalArtifact.id}`);expect(expired.status()).toBe(503);cases.push({case:'explicit-legacy-expired-bytes',before:old,after:restored,download_status:expired.status()});
 await writeFile(info.outputPath('runtime-edge-cases.json'),JSON.stringify(cases,null,2));
});

test('CTX-REV-001 first browser Reopen after serving replacement keeps the known goal prompt-ready', async ({page,context,request,baseURL},info) => {
 await context.addCookies([{name:'colony2:user:email',value:'fixture@example.test',url:baseURL!}]);
 await page.goto('/project/assistant-browser/jobs');
 await page.getByRole('button',{name:'Open assistant'}).click();
 const panel=page.getByRole('dialog',{name:'Contextual assistant'});
 const marker='Known browser session before serving replacement';
 await panel.getByRole('textbox',{name:'Message'}).fill(marker);
 const started=page.waitForResponse(r=>r.url().endsWith('/assistant/sessions')&&r.request().method()==='POST');
 await panel.getByRole('button',{name:'Send message'}).click();
 const initialResponse=await started;expect(initialResponse.ok(),await initialResponse.text()).toBeTruthy();
 const original=await initialResponse.json() as AssistantSession;
 await ready(request,original.id);
 await expect(panel.getByLabel('Conversation transcript')).toContainText('Grounded deterministic reply',{timeout:120_000});
 await panel.getByRole('button',{name:'Close conversation'}).click();
 await expect(panel).toBeHidden();
 await page.getByRole('button',{name:'Open assistant'}).click();
 await expect(panel.getByRole('button',{name:'Reopen conversation'})).toBeVisible();
 const oldGeneration=await generation(request);
 await command(request,'replace');
 const currentGeneration=await generation(request);expect(currentGeneration).not.toBe(oldGeneration);
 const modelCalls=async()=> (await(await request.get(control+'evidence')).json()).requests.filter((q:any)=>JSON.stringify(q).includes(marker)).length;
 const before=await modelCalls();expect(before).toBe(1);
 const reopening=page.waitForResponse(r=>r.url().endsWith(`/assistant/sessions/${original.id}/reopen`));
 await panel.getByRole('button',{name:'Reopen conversation'}).click();
 const response=await reopening;expect(response.ok(),await response.text()).toBeTruthy();
 const input=response.request().postDataJSON();expect(input.generation).toBe(currentGeneration);expect(input.job_id).toBe(original.job_id);
 const reopened=await response.json() as AssistantSession;
 expect(reopened.id).toBe(original.id);expect(reopened.goal_id).toBe(original.goal_id);expect(reopened.job_id).not.toBe(original.job_id);
 const promptReady=await ready(request,original.id);expect(promptReady.job_id).toBe(reopened.job_id);
 await expect(panel.getByRole('textbox',{name:'Message'})).toBeEnabled();
 await expect(panel.getByRole('link',{name:'View execution'})).toHaveAttribute('href',`/project/assistant-browser/jobs/${reopened.job_id}/story`);
 expect(await modelCalls()).toBe(before);expect(promptReady.replay.entries.some(e=>e.origin?.job_id===original.job_id)).toBe(true);
 await page.setViewportSize({width:1365,height:1000});await page.screenshot({path:info.outputPath('desktop-first-reopen-after-replacement.png')});
 await writeFile(info.outputPath('first-reopen-after-replacement.json'),JSON.stringify({old_generation:oldGeneration,current_generation:currentGeneration,request:input,status:response.status(),original,reopened:promptReady,prior_reopen_attempts:0,model_calls_before:before,model_calls_after:await modelCalls(),panel_reloaded:false,history_reselected:false},null,2));
 await close(request,promptReady);
});

test('CTX-REV-001 first browser Start after serving replacement freezes the current generation', async ({page,context,request,baseURL},info) => {
 await context.addCookies([{name:'colony2:user:email',value:'fixture@example.test',url:baseURL!}]);
 await page.goto('/project/assistant-browser/jobs');
 await page.getByRole('button',{name:'Open assistant'}).click();
 const panel=page.getByRole('dialog',{name:'Contextual assistant'});
 await expect(panel.getByRole('textbox',{name:'Message'})).toBeEnabled();
 const oldGeneration=await generation(request);await command(request,'replace');
 const currentGeneration=await generation(request);expect(currentGeneration).not.toBe(oldGeneration);
 await panel.getByRole('textbox',{name:'Message'}).fill('First browser start after serving replacement');
 const starting=page.waitForResponse(r=>r.url().endsWith('/assistant/sessions')&&r.request().method()==='POST');
 await panel.getByRole('button',{name:'Send message'}).click();
 const response=await starting;expect(response.ok(),await response.text()).toBeTruthy();
 const input=response.request().postDataJSON();expect(input.generation).toBe(currentGeneration);
 const accepted=await response.json() as AssistantSession;const recorded=await ready(request,accepted.id);
 await expect(panel.getByLabel('Conversation transcript')).toContainText('Grounded deterministic reply',{timeout:120_000});
 await expect(panel.getByRole('textbox',{name:'Message'})).toBeEnabled();
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:info.outputPath('mobile-first-start-after-replacement.png')});
 await writeFile(info.outputPath('first-start-after-replacement.json'),JSON.stringify({old_generation:oldGeneration,current_generation:currentGeneration,request:input,status:response.status(),session:recorded,prior_start_attempts:0,panel_reloaded:false,history_reselected:false},null,2));
 await close(request,recorded);
});
