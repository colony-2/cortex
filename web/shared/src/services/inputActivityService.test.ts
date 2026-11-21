import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { inputActivityService as service, type PendingInput } from './inputActivityService';
import { parseJobJSON, stringifyJobJSON } from '../jobJSON';
const identity = {cell_id:'cell-1',cell_name:'platform',repo:'github.com/acme/platform'};
class Source {
 static instances: Source[] = [];
 readyState = 0; onopen?: () => void; onerror?: () => void;
 listeners = new Map<string, Function>();
 constructor(public url: string) {Source.instances.push(this);}
 addEventListener(name: string, listener: Function) {this.listeners.set(name,listener);}
 close() {this.readyState = 2;}
 send(name: string, value: unknown) {this.listeners.get(name)?.({data: stringifyJobJSON(value)});}
}
const row = (extra = {}): PendingInput => ({...identity,id:'job',client_payload_revision:1,...extra});
const details = (extra = {}) => ({jobId:'job',status:'pending',startTime:'now',form:{question:'Continue?'},...extra});
const deferred = <T,>() => {let resolve!: (value:T)=>void; let reject!: (error:Error)=>void; const promise = new Promise<T>((yes,no)=>{resolve=yes;reject=no;});return {promise,resolve,reject};};
let source: Source;
const snapshot = (inputs: PendingInput[] = [row()]) => source.send('inputs_changed',{inputs});
beforeEach(() => {
 service.disconnect(); service.clearCaches(); Source.instances=[];
 vi.stubGlobal('EventSource',Source); vi.stubGlobal('fetch',vi.fn());
 service.connect('tenant'); source=Source.instances[0];
});
afterEach(() => {service.disconnect();service.clearCaches();vi.unstubAllGlobals();vi.restoreAllMocks();});

describe('shared discovery and selected request acquisition',()=>{
 it('keeps startup, socket-open and heartbeat unknown until successful discovery',async()=>{
  const read=deferred<Response>();vi.mocked(fetch).mockReturnValue(read.promise);
  expect(service.getDiscovery('tenant').status).toBe('loading');
  source.onopen?.();source.send('heartbeat',{});
  expect(service.getDiscovery('tenant').status).toBe('loading');
  expect(await service.getInputDetails('tenant','job')).toBeNull();
  snapshot([]);expect(service.getDiscovery('tenant')).toEqual({status:'current',inputs:[]});
  read.resolve(new Response('[]'));await read.promise;
 });
 it.each(['transport','application','malformed'])('marks retained rows stale on %s failure',async kind=>{
  snapshot();
  if(kind==='transport')source.onerror?.();
  else if(kind==='application')source.send('error',{error:'storage failed'});
  else source.send('inputs_changed',{inputs:{}});
  expect(service.getDiscovery('tenant').status).toBe('unavailable');
  expect(service.getDiscovery('tenant').inputs).toHaveLength(1);
  expect(await service.getInputDetails('tenant','job')).toBeNull();expect(fetch).not.toHaveBeenCalled();
 });
 it.each([
  [row({client_payload_revision:undefined})], [row({client_payload_revision:-1})], [row({client_payload_revision:1.5})],
  [row({client_payload_revision:'7'})], [row({id:''})], [row({cell_id:undefined})], [row(),row()],
  [row({request_id:42})], [row({task_ordinal:1.5})],
 ])('rejects the entire invalid snapshot %j',inputs=>{
  snapshot();snapshot(inputs);expect(service.getDiscovery('tenant').status).toBe('unavailable');
  expect(service.getDiscovery('tenant').inputs).toEqual([row()]);
 });
 it('marks failed EventSource construction unavailable and reconnects a closed source on Retry',async()=>{
  snapshot();
  source.readyState=2;source.onerror?.();
  const read=deferred<Response>();vi.mocked(fetch).mockReturnValue(read.promise);
  const recovery=service.getPendingInputs('tenant',true);
  expect(service.getDiscovery('tenant')).toEqual({status:'loading',inputs:[row()]});
  expect(service.selectedVersion('tenant','job')).toBeUndefined();
  read.resolve(new Response('[]'));await recovery;
  expect(Source.instances).toHaveLength(2);expect(service.getDiscovery('tenant').status).toBe('current');
  vi.stubGlobal('EventSource',class {constructor(){throw new Error('Cannot open stream');}});
  service.connect('other');expect(service.getDiscovery('other').status).toBe('unavailable');
 });
 it('ignores every old connection callback, even after reconnecting to the same tenant',()=>{
  const old=source;service.disconnect();service.connect('tenant');source=Source.instances.at(-1)!;snapshot();
  old.send('inputs_changed',{inputs:[]});old.send('error',{error:'late'});old.onerror?.();old.onopen?.();
  expect(service.getDiscovery('tenant')).toEqual({status:'current',inputs:[row()]});expect(fetch).not.toHaveBeenCalled();
 });
 it.each(['success','failure'])('does not let late GET %s overwrite newer SSE data',async kind=>{
  const read=deferred<Response>();vi.mocked(fetch).mockReturnValue(read.promise);
  const pending=service.getPendingInputs('tenant',true);snapshot();
  if(kind==='success')read.resolve(new Response('[]'));else read.reject(new Error('late failure'));
  expect(await pending).toEqual([row()]);expect(service.getDiscovery('tenant').status).toBe('current');
 });
 it('does not present a late GET as fresh after disconnect',async()=>{
  const read=deferred<Response>();vi.mocked(fetch).mockReturnValue(read.promise);
  const pending=service.getPendingInputs('tenant',true);source.onerror?.();read.resolve(new Response('[]'));
  await expect(pending).rejects.toThrow();expect(service.getDiscovery('tenant').status).toBe('unavailable');
 });
 it('coalesces fresh list reads and retains stale information on failure',async()=>{
  snapshot();vi.mocked(fetch).mockResolvedValue(new Response('',{status:503}));
  const a=service.getPendingInputs('tenant',true),b=service.getPendingInputs('tenant',true);
  await expect(a).rejects.toThrow();await expect(b).rejects.toThrow();expect(fetch).toHaveBeenCalledTimes(1);
  expect(service.getDiscovery('tenant').inputs).toEqual([row()]);
 });
 it('reads only matched selected jobs and coalesces consumers',async()=>{
  expect(await service.getInputDetails('tenant','job')).toBeNull();snapshot([row(),row({id:'other'})]);
  const read=deferred<Response>();vi.mocked(fetch).mockReturnValue(read.promise);
  const a=service.getInputDetails('tenant','job'),b=service.getInputDetails('tenant','job',false,new AbortController().signal);
  expect(fetch).toHaveBeenCalledTimes(1);read.resolve(new Response(JSON.stringify(details())));
  expect(await a).toEqual(details());expect(await b).toEqual(details());
  expect(await service.getInputDetails('tenant','absent')).toBeNull();expect(fetch).toHaveBeenCalledTimes(1);
 });
 it('reuses verified details across duplicates, metadata changes, reordering and unrelated versions',async()=>{
  snapshot();vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify(details())));await service.getInputDetails('tenant','job');
  snapshot();snapshot([row({id:'other',client_payload_revision:99}),row({title:'Changed title',requested_at:'later'})]);
  expect(await service.getInputDetails('tenant','job')).toEqual(details());expect(fetch).toHaveBeenCalledTimes(1);
 });
 it.each([{client_payload_revision:2},{request_id:'new'},{task_ordinal:2}])('acquires a successor when its version changes: %j',async extra=>{
  snapshot();vi.mocked(fetch).mockImplementation(async()=>new Response(JSON.stringify(details({form:{request_id:extra.request_id,question:'Continue?'},task_ordinal:extra.task_ordinal}))));
  await service.getInputDetails('tenant','job');snapshot([row(extra)]);await service.getInputDetails('tenant','job');expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('preserves exact large revisions and detects their changes',async()=>{
  const exact=parseJobJSON('[{"id":"job","cell_id":"cell-1","cell_name":"platform","repo":"github.com/acme/platform","client_payload_revision":9007199254740993}]') as PendingInput[];
  snapshot(exact);const first=service.selectedVersion('tenant','job');
  expect(first).toContain('9007199254740993');
  snapshot(parseJobJSON(stringifyJobJSON(exact)!.replace('9007199254740993','9007199254740994')) as PendingInput[]);
  expect(service.selectedVersion('tenant','job')).not.toBe(first);
 });
 it('reuses verified details after unknown/recovery, but reacquires after confirmed removal/reappearance',async()=>{
  snapshot();vi.mocked(fetch).mockImplementation(async()=>new Response(JSON.stringify(details())));await service.getInputDetails('tenant','job');
  source.onerror?.();snapshot();await service.getInputDetails('tenant','job');expect(fetch).toHaveBeenCalledTimes(1);
  snapshot([]);snapshot();await service.getInputDetails('tenant','job');expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('ignores obsolete completions and cleanup while a successor is loading',async()=>{
  snapshot();const old=deferred<Response>(),next=deferred<Response>();vi.mocked(fetch).mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
  const a=service.getInputDetails('tenant','job');snapshot([row({client_payload_revision:2})]);const b=service.getInputDetails('tenant','job');
  old.resolve(new Response(JSON.stringify(details())));expect(await a).toBeNull();
  next.resolve(new Response(JSON.stringify(details({form:{question:'Successor'}}))));expect((await b)?.form.question).toBe('Successor');
  expect((await service.getInputDetails('tenant','job'))?.form.question).toBe('Successor');expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('invalidates a pending read across an outage, without deleting the recovered attempt',async()=>{
  snapshot();const old=deferred<Response>();vi.mocked(fetch).mockReturnValueOnce(old.promise);
  const a=service.getInputDetails('tenant','job');source.onerror?.();snapshot();vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(details())));
  const b=service.getInputDetails('tenant','job');old.resolve(new Response(JSON.stringify(details({form:{question:'Obsolete'}}))));
  expect(await a).toBeNull();expect(await b).toEqual(details());expect(await service.getInputDetails('tenant','job')).toEqual(details());
 });
 it('handles 204 without JSON parsing, revalidates once and latches unchanged absence',async()=>{
  snapshot();vi.mocked(fetch).mockResolvedValueOnce(new Response(null,{status:204})).mockResolvedValueOnce(new Response(JSON.stringify([row()])));
  const a=service.getInputDetails('tenant','job'),b=service.getInputDetails('tenant','job');expect(await a).toBeNull();expect(await b).toBeNull();
  await vi.waitFor(()=>expect(service.getDiscovery('tenant').status).toBe('current'));
  snapshot();expect(await service.getInputDetails('tenant','job')).toBeNull();expect(fetch).toHaveBeenCalledTimes(2);
 });
 it.each([403,404,409,500])('latches HTTP %s as an error instead of absence',async status=>{
  snapshot();vi.mocked(fetch).mockResolvedValue(new Response(null,{status}));
  await expect(service.getInputDetails('tenant','job')).rejects.toMatchObject({status});snapshot();
  await expect(service.getInputDetails('tenant','job')).rejects.toMatchObject({status});expect(fetch).toHaveBeenCalledTimes(1);
 });
 it.each(['malformed','network'])('latches real %s failures',async kind=>{
  snapshot();if(kind==='malformed')vi.mocked(fetch).mockResolvedValue(new Response('{'));else vi.mocked(fetch).mockRejectedValue(new Error('Network failed'));
  await expect(service.getInputDetails('tenant','job')).rejects.toThrow();await expect(service.getInputDetails('tenant','job')).rejects.toThrow();expect(fetch).toHaveBeenCalledTimes(1);
 });
 it.each([{jobId:'wrong'},{form:{request_id:'wrong'}},{task_ordinal:99}])('revalidates mismatched details without a loop: %j',async extra=>{
  snapshot([row({request_id:'native',task_ordinal:12})]);
  vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(details({form:{request_id:'native'},task_ordinal:12,...extra})))).mockResolvedValueOnce(new Response(JSON.stringify([row({request_id:'native',task_ordinal:12})])));
  expect(await service.getInputDetails('tenant','job')).toBeNull();await vi.waitFor(()=>expect(fetch).toHaveBeenCalledTimes(2));
  expect(await service.getInputDetails('tenant','job')).toBeNull();expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('refreshes discovery before explicit retry and coalesces concurrent retries',async()=>{
  snapshot();vi.mocked(fetch).mockResolvedValueOnce(new Response(null,{status:503}));await expect(service.getInputDetails('tenant','job')).rejects.toThrow();
  vi.mocked(fetch).mockImplementation(async url=>new Response(JSON.stringify(String(url).endsWith('/pending')?[row()]:details())));
  await Promise.all([service.getInputDetails('tenant','job',true),service.getInputDetails('tenant','job',true)]);
  expect(vi.mocked(fetch).mock.calls.slice(1).map(([url])=>String(url).split('/').at(-1))).toEqual(['pending','job']);
 });
 it.each(['empty','failed'])('never probes details when explicit retry discovery is %s',async kind=>{
  snapshot();vi.mocked(fetch).mockResolvedValue(kind==='empty'?new Response('[]'):new Response(null,{status:503}));
  if(kind==='empty')expect(await service.getInputDetails('tenant','job',true)).toBeNull();else await expect(service.getInputDetails('tenant','job',true)).rejects.toThrow();
  expect(fetch).toHaveBeenCalledTimes(1);
 });
 it('suppresses an answered ordinary request before discovery catches up',async()=>{
  snapshot();vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(details())));await service.getInputDetails('tenant','job');
  vi.mocked(fetch).mockResolvedValueOnce(new Response('{}')).mockResolvedValueOnce(new Response(JSON.stringify([row()])));
  await service.submitResponse('tenant','job',{fields:{answer:'yes'}});expect(await service.getInputDetails('tenant','job')).toBeNull();
 });
});

describe('completion acknowledgements',()=>{
 const kinds = ['ordinary response','legacy review','review/v2','cancel'] as const;
 const complete = (kind: typeof kinds[number]) => kind === 'ordinary response'
  ? service.submitResponse('tenant','job',{fields:{answer:'yes'}})
  : kind === 'legacy review'
   ? service.submitReview('tenant','job','request-1',{fields:{decision:'approve'}})
   : kind === 'review/v2'
    ? service.submitReviewV2('tenant','job','request-1',{direction:'approve',feedback:'',attachments:[],settings:{}},[],'submission-1')
    : service.cancelInput('tenant','job');
 const versionRow = (revision: number) => row({request_id:`request-${revision}`,client_payload_revision:revision});
 const versionDetails = (revision: number) => details({form:{request_id:`request-${revision}`,question:`Request ${revision}`}});
 const acknowledgement = () => new Response(JSON.stringify({response:{direction:'approve'},receipt:{request_id:'request-1',submission_id:'submission-1'}}));

 it.each(kinds)('preserves a cached successor after a late %s acknowledgement',async kind=>{
  snapshot([versionRow(1)]);
  const post=deferred<Response>();
  vi.mocked(fetch).mockImplementation(async(url,init)=>{
   if(init?.method==='POST')return post.promise;
   return new Response(JSON.stringify(String(url).endsWith('/pending')?[versionRow(2)]:versionDetails(2)));
  });
  const completing=complete(kind);
  // The server has accepted A and advertised B before A's POST response arrives.
  snapshot([versionRow(2)]);
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(2));
  post.resolve(acknowledgement());await completing;
  await service.getPendingInputs('tenant',true);
  expect(service.getDiscovery('tenant').inputs).toEqual([versionRow(2)]);
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(2));
  expect(vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/user-inputs/job'))).toHaveLength(1);
 });

 it.each(kinds)('preserves an in-flight successor after a late %s acknowledgement',async kind=>{
  snapshot([versionRow(1)]);
  const post=deferred<Response>(),next=deferred<Response>();
  vi.mocked(fetch).mockImplementation(async(url,init)=>{
   if(init?.method==='POST')return post.promise;
   if(String(url).endsWith('/pending'))return new Response(JSON.stringify([versionRow(2)]));
   return next.promise;
  });
  const completing=complete(kind);
  snapshot([versionRow(2)]);const reading=service.getInputDetails('tenant','job');
  post.resolve(acknowledgement());await completing;
  await service.getPendingInputs('tenant',true);
  next.resolve(new Response(JSON.stringify(versionDetails(2))));
  expect(await reading).toEqual(versionDetails(2));
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(2));
  expect(vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/user-inputs/job'))).toHaveLength(1);
 });

 it.each(kinds)('preserves a reappeared request identity after a late %s acknowledgement',async kind=>{
  snapshot([versionRow(1)]);
  const post=deferred<Response>();
  vi.mocked(fetch).mockImplementation(async(url,init)=>{
   if(init?.method==='POST')return post.promise;
   return new Response(JSON.stringify(String(url).endsWith('/pending')?[versionRow(1)]:versionDetails(1)));
  });
  const completing=complete(kind),original=service.selectedVersion('tenant','job');
  snapshot([]);snapshot([versionRow(1)]);
  expect(service.selectedVersion('tenant','job')).not.toBe(original);
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(1));
  post.resolve(acknowledgement());await completing;
  await service.getPendingInputs('tenant',true);
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(1));
  expect(vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/user-inputs/job'))).toHaveLength(1);
 });

 it.each(kinds)('does not borrow a newly discovered version for a %s started without one',async kind=>{
  const post=deferred<Response>();
  vi.mocked(fetch).mockImplementation(async(url,init)=>{
   if(init?.method==='POST')return post.promise;
   return new Response(JSON.stringify(String(url).endsWith('/pending')?[versionRow(2)]:versionDetails(2)));
  });
  const completing=complete(kind);
  snapshot([versionRow(2)]);
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(2));
  post.resolve(acknowledgement());await completing;
  await service.getPendingInputs('tenant',true);
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(2));
  expect(vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/user-inputs/job'))).toHaveLength(1);
 });

 it.each(kinds)('suppresses the matching answered version after %s despite stale discovery',async kind=>{
  snapshot([versionRow(1)]);
  vi.mocked(fetch).mockImplementation(async(url,init)=>init?.method==='POST'
   ? acknowledgement()
   : new Response(JSON.stringify(String(url).endsWith('/pending')?[versionRow(1)]:versionDetails(1))));
  expect(await service.getInputDetails('tenant','job')).toEqual(versionDetails(1));
  await complete(kind);await service.getPendingInputs('tenant',true);
  snapshot([versionRow(1)]);
  expect(await service.getInputDetails('tenant','job')).toBeNull();
  expect(vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/user-inputs/job'))).toHaveLength(1);
 });
});

describe('A05 review document URL identity', () => {
  it('encodes tenant/job path segments and request/document query identities', () => {
    const url = new URL(service.reviewDocumentURL('tenant/雪','job/a','request & 1','folder/雪.md'), 'http://localhost');
    expect(url.pathname).toContain('/projects/tenant%2F%E9%9B%AA/reviews/job%2Fa/documents');
    expect(url.searchParams.get('request_id')).toBe('request & 1');
    expect(url.searchParams.get('document_id')).toBe('folder/雪.md');
  });
});

describe('review/v2 structured submission',()=>{
 it('sends ordered multipart uploads, sparse typed settings and stable correlation; returns native receipt',async()=>{
  const output={response:{direction:'ask',feedback:' exact ',attachments:[],settings:{strange:false,zero:0,reset:null}},receipt:{request_id:'request-v2',submission_id:'correlation'}};
  const fetchMock=vi.fn().mockResolvedValue(new Response(JSON.stringify(output),{status:200}));vi.stubGlobal('fetch',fetchMock);
  const files=[new File(['one'],'same.md'),new File(['two'],'same.md')];
  const result=await service.submitReviewV2('tenant','job','request-v2',{direction:'ask',feedback:' exact ',attachments:[],settings:{strange:false,zero:0,reset:null}},files,'correlation');
  const body=fetchMock.mock.calls[0][1].body as FormData;
  expect(JSON.parse(body.get('submission') as string)).toEqual({request_id:'request-v2',submission_id:'correlation',response:output.response});
  expect(body.getAll('attachments')).toEqual(files);expect(result).toEqual(output);vi.unstubAllGlobals();
 });
});

  it('submits reviews without randomUUID, preserving the request and generating distinct IDs', async () => {
    const projectId='tenant';
    vi.stubGlobal('crypto', { getRandomValues: globalThis.crypto.getRandomValues.bind(globalThis.crypto) });
    vi.mocked(fetch).mockResolvedValue({ ok: true } as Response);
    const response = { fields: { decision: 'revise', feedback: 'Please clarify the rollout.' } };
    await service.submitReview(projectId, 'job', 'request', response);
    await service.submitReview(projectId, 'job', 'request', response);
    const submissions = vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/respond')).map(([url, init]) => {
      expect(url).toContain(`/projects/${projectId}/reviews/job/respond`);
      expect(init?.method).toBe('POST');
      return JSON.parse((init?.body as FormData).get('submission') as string);
    });
    for (const submission of submissions) {
      expect(submission).toMatchObject({ request_id: 'request', fields: response.fields });
      expect(submission.submission_id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    }
    expect(submissions[0].submission_id).not.toBe(submissions[1].submission_id);
  });
