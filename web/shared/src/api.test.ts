import { afterEach, expect, it, vi } from 'vitest';
import { submitJob, getJob, listJobs, listCells, getCell, createCell } from './api';
import { stringifyJobJSON } from './jobJSON';

afterEach(() => vi.unstubAllGlobals());

it.each([undefined, 'null', '[1,"雪",9007199254740993]', '{"other":{"n":9007199254740993},"c2j":[]}', JSON.stringify('x'.repeat(65534))])(
  'retains opaque payloads and their revisions in list and detail responses: case %#', async payload => {
    const raw = `{"job_id":"job","cell_id":"cell-1","cell_name":"platform","repo":"github.com/acme/platform","client_payload_revision":9${payload === undefined ? '' : `,"client_payload":${payload}`}}`;
    vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(url.endsWith('/job') ? raw : `{"jobs":[${raw}],"next_page_token":"untouched"}`)));
    const page = await listJobs('tenant');
    expect(page.next_page_token).toBe('untouched');
    for (const job of [page.jobs[0], await getJob('tenant', 'job')]) {
      expect(job.client_payload_revision).toBe(9);
      expect(Object.hasOwn(job, 'client_payload')).toBe(payload !== undefined);
      expect(stringifyJobJSON(job.client_payload)).toBe(payload);
    }
  },
);

it('requests all statuses when the status filter is explicitly cleared', async () => {
  const fetch = vi.fn().mockImplementation(async () => new Response(JSON.stringify(({ jobs: [] }))));
  vi.stubGlobal('fetch', fetch);
  await listJobs('tenant', { status: [], cellNames: ['app'] });
  const query = new URL(fetch.mock.calls[0][0]).searchParams;
  expect(query.getAll('status')).toEqual(['all']);
  expect(query.get('cellNames')).toBe('app');

  await listJobs('tenant', { status: ['READY', 'COMPLETED'] });
  expect(new URL(fetch.mock.calls[1][0]).searchParams.getAll('status')).toEqual(['READY', 'COMPLETED']);

  await listJobs('tenant');
  expect(new URL(fetch.mock.calls[2][0]).searchParams.has('status')).toBe(false);
});

it('sends multiple cells as a union alongside status and pagination', async () => {
  const fetch = vi.fn().mockResolvedValue(new Response('{"jobs":[]}'));
  vi.stubGlobal('fetch', fetch);
  await listJobs('tenant', { cellNames: ['alpha', 'beta'], displayStatus: ['READY'], pageToken: 'cursor' });
  const query = new URL(fetch.mock.calls[0][0]).searchParams;
  expect(query.getAll('cellNames')).toEqual(['alpha', 'beta']);
  expect(query.getAll('displayStatus')).toEqual(['READY']);
  expect(query.get('pageToken')).toBe('cursor');
});

it('forwards inbox visibility independently of status, cells and pagination', async () => {
  const fetch = vi.fn(async () => new Response('{"jobs":[]}'));
  vi.stubGlobal('fetch', fetch);
  for (const includeHidden of [false, true]) {
    await listJobs('tenant', { includeHidden, displayStatus: ['failed_app'], cellNames: ['app'], goal: 'goal', pageToken: 'next' });
    const query = new URL(fetch.mock.calls.at(-1)![0] as string).searchParams;
    expect(query.get('includeHidden')).toBe(String(includeHidden));
    expect(query.get('displayStatus')).toBe('failed_app');
    expect(query.get('cellNames')).toBe('app');
    expect(query.get('goal')).toBe('goal');
    expect(query.get('pageToken')).toBe('next');
  }
});

it('sends mixed display statuses to the API and retains pagination and scoping', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify({ jobs: [] })));
  vi.stubGlobal('fetch', fetch);
  await listJobs('tenant', { displayStatus: ['READY', 'success', 'failed_app', 'CANCEL_REQUESTED', 'OUTCOME_UNAVAILABLE', 'completion:future outcome'], cellNames: ['app'], goal: 'goal', pageSize: 2, pageToken: 'cursor+/=' });
  const query = new URL(vi.mocked(globalThis.fetch).mock.calls[0][0] as string).searchParams;
  expect(query.getAll('displayStatus')).toEqual(['READY', 'success', 'failed_app', 'CANCEL_REQUESTED', 'OUTCOME_UNAVAILABLE', 'completion:future outcome']);
  expect(query.has('status')).toBe(false);
  expect(query.get('cellNames')).toBe('app');
  expect(query.get('goal')).toBe('goal');
  expect(query.get('pageSize')).toBe('2');
  expect(query.get('pageToken')).toBe('cursor+/=');
  await listJobs('tenant', { displayStatus: [] });
  const cleared = new URL(vi.mocked(globalThis.fetch).mock.calls[1][0] as string).searchParams;
  expect(cleared.getAll('status')).toEqual(['all']);
  expect(cleared.has('displayStatus')).toBe(false);
});

it('downloads opaque chapter bytes through encoded routes and forwards cancellation', async () => {
  const { getJobChaptersDownload } = await import('./api');
  const { API_BASE } = await import('./apiBase');
  const blob = new Blob(['{"ordinal":9007199254740993}'], { type: 'application/json' });
  const json = vi.fn();
  const fetch = vi.fn().mockResolvedValue({ ok: true, blob: async () => blob, json });
  vi.stubGlobal('fetch', fetch);
  const controller = new AbortController();
  expect(await getJobChaptersDownload('tenant ?#', 'job/a&雪', controller.signal)).toBe(blob);
  expect(fetch).toHaveBeenCalledWith(`${API_BASE}/projects/tenant%20%3F%23/jobs/job%2Fa%26%E9%9B%AA/chapters/download`, { signal: controller.signal });
  expect(json).not.toHaveBeenCalled();
  const text = await new Promise<string>(resolve => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blob); });
  expect(text).toBe('{"ordinal":9007199254740993}');
});

it('rejects HTTP and network failures instead of downloading errors', async () => {
  const { getJobChaptersDownload } = await import('./api');
  const blob = vi.fn();
  const fetch = vi.fn().mockResolvedValue({ ok: false, status: 404, statusText: 'Not Found', text: async () => 'missing job', blob });
  vi.stubGlobal('fetch', fetch);
  await expect(getJobChaptersDownload('tenant', 'job')).rejects.toThrow('404 Not Found - missing job');
  expect(blob).not.toHaveBeenCalled();
  fetch.mockRejectedValueOnce(new TypeError('network unavailable'));
  await expect(getJobChaptersDownload('tenant', 'job')).rejects.toThrow('network unavailable');
});


it('preserves registry pagination, nested metadata, opaque revisions and lifecycle selectors', async () => {
  const cell = { tenantId: 'tenant', id: 'stable', name: 'renamed', repo: 'github.com/acme/app', state: 'RETIRED',
    revision: 'opaque+/=', metadata: { nested: [null, { enabled: true }] }, aliases: ['old'] };
  const fetch = vi.fn().mockImplementation(async (url: string) => new Response(JSON.stringify(url.includes('/cells?')
    ? { cells: [cell], nextPageToken: 'next+/=' } : cell)));
  vi.stubGlobal('fetch', fetch);
  const page = await listCells('tenant', { state: 'ALL', name: 'old', pageSize: 1, pageToken: 'cursor+/=' });
  expect(page).toEqual({ cells: [cell], nextPageToken: 'next+/=' });
  expect(Object.fromEntries(new URL(fetch.mock.calls[0][0]).searchParams)).toEqual({ state: 'ALL', name: 'old', pageSize: '1', pageToken: 'cursor+/=' });
  expect(await getCell('tenant', 'stable')).toEqual(cell);
  await expect(getCell('tenant', 'wrong')).rejects.toThrow('does not match');
  await expect(getCell('other', 'stable')).rejects.toThrow('Invalid cell registry');
});

it('retains cell conflict codes for actionable creation errors', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'cell_conflict', code: 'cell_conflict' }), { status: 409 })));
  await expect(createCell('tenant', { name: 'app', repo: 'github.com/acme/app', requestId: 'retry' })).rejects.toMatchObject({ status: 409, code: 'cell_conflict' });
});

it.each(['cell_id', 'cell_name', 'repo'])('rejects missing %s instead of inventing job ownership', async field => {
  const job: Record<string, string> = { cell_id: 'stable', cell_name: 'app', repo: 'github.com/acme/app' };
  delete job[field];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(JSON.stringify(url.endsWith('/job') ? job : { jobs: [job] }))));
  await expect(listJobs('tenant')).rejects.toThrow(`required ${field}`);
  await expect(getJob('tenant', 'job')).rejects.toThrow(`required ${field}`);
});

 it('ATT-021 ATT-022 RET-009 sends ordered multipart files and retains JSON without files', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify({ tenant_id: 'tenant', job_id: 'job', goal_id: 'goal', cell_id: 'cell', cell_name: 'platform', repo: 'github.com/acme/platform' })));
  vi.stubGlobal('fetch', fetch);
  const input = { cell: 'platform', inputs: { prompt: 'hello', type: 'build' }, job_id: 'job' };
  const files = [new File(['binary'], '雪.bin'), new File([], 'empty')];
  await submitJob('tenant', input, files);
  const options = (fetch.mock.calls[0] as unknown as [string, RequestInit])[1];
  expect(options.headers).toBeUndefined();
  const body = options.body as FormData;
  expect(body.getAll('submission')).toEqual([JSON.stringify(input)]);
  expect(body.getAll('files').map(file => (file as File).name)).toEqual(['0', '1']);
  expect(body.getAll('filenames')).toEqual([JSON.stringify(['雪.bin', 'empty'])]);
  expect((body.getAll('files')[0] as File).size).toBe(files[0].size);
  await submitJob('tenant', input);
  const plain = (fetch.mock.calls[1] as unknown as [string, RequestInit])[1];
  expect(plain.headers).toEqual({ 'Content-Type': 'application/json' });
  expect(plain.body).toBe(JSON.stringify(input));
 });
it.each([{}, { tenant_id: 'other', job_id: 'job', goal_id: 'goal', cell_id: 'cell', cell_name: 'platform', repo: 'github.com/acme/platform' }, { tenant_id: 'tenant', job_id: 'job', cell_id: 'cell', cell_name: 'platform', repo: 'github.com/acme/platform' }])('RET-009 rejects invalid submission identity: %#', async identity => {
 vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(identity))));
 await expect(submitJob('tenant', { cell: 'platform' })).rejects.toThrow();
 await expect(submitJob('tenant', { cell: 'platform' }, [new File([], 'empty')])).rejects.toThrow();
});

it('REL-009,021 encodes child paths and cursors and preserves lossless summary numbers', async () => {
  const { listJobChildren } = await import('./api');
  const fetch = vi.fn(async () => new Response('{"jobs":[{"cell_id":"cell","cell_name":"platform","repo":"github.com/acme/platform","parent":{"invocation_seq":9007199254740993}}],"next_page_token":"next+/="}'));
  vi.stubGlobal('fetch', fetch);
  const page = await listJobChildren('tenant /雪', 'job /?#', { pageSize: 20, pageToken: 'cursor+/=' });
  const url = new URL(fetch.mock.calls[0][0] as string);
  expect(url.pathname).toContain('/tenant%20%2F%E9%9B%AA/jobs/job%20%2F%3F%23/children');
  expect(url.searchParams.get('pageSize')).toBe('20');
  expect(url.searchParams.get('pageToken')).toBe('cursor+/=');
  expect(String(page.jobs[0].parent?.invocation_seq)).toBe('9007199254740993');
  expect(page.next_page_token).toBe('next+/=');
});
it('REL-021 rejects malformed children and surfaces HTTP errors', async () => {
  const { listJobChildren } = await import('./api');
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch);
  for (const payload of [{ jobs: [{}] }, { jobs: null }]) {
    fetch.mockResolvedValueOnce(new Response(JSON.stringify(payload)));
    await expect(listJobChildren('tenant','job')).rejects.toThrow(/Invalid/);
  }
  fetch.mockResolvedValueOnce(new Response('{"error":"offline"}', { status: 503 }));
  await expect(listJobChildren('tenant','job')).rejects.toThrow(/503.*offline/);
});

it('sends optional cancellation/archive reasons as JSON and keeps bodyless callers compatible', async () => {
  const { cancelJob, archiveJob } = await import('./api');
  const fetch = vi.fn().mockResolvedValue({ok:true,status:204});
  vi.stubGlobal('fetch',fetch);
  await cancelJob('tenant','one','雪\nmessage');
  await archiveJob('tenant','one','Cancelled from Cortex');
  await cancelJob('tenant','bulk');
  await archiveJob('tenant','finished');
  expect(fetch.mock.calls[0][1]).toEqual({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({reason:'雪\nmessage'})});
  expect(fetch.mock.calls[1][1]).toEqual({method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({reason:'Cancelled from Cortex'})});
  expect(fetch.mock.calls[2][1]).toEqual({method:'POST'});
  expect(fetch.mock.calls[3][1]).toEqual({method:'DELETE'});
});

it('DOC-16 IA-007 lists typed input metadata with encoded identity, cancellation and errors', async () => {
  const { getJobInputArtifacts } = await import('./api');
  const { API_BASE } = await import('./apiBase');
  const result = { artifacts: [{ jobId: 'job/a&雪', taskOrdinal: 0, name: 'docs/brief.md', sizeBytes: 0 }, {id:'document:inline', kind:'value', name:'inline.md', binding:'inline', contentBinding:'inline', sources:['Document'], sizeBytes:5}, {id:'document:external', kind:'external', name:'external.md', reason:'External reference — content not loaded', sizeBytes:-1}] };
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(result)));
  vi.stubGlobal('fetch', fetch);
  const controller = new AbortController();
  expect(await getJobInputArtifacts('tenant ?#', 'job/a&雪', controller.signal)).toEqual(result);
  expect(fetch).toHaveBeenCalledWith(`${API_BASE}/projects/tenant%20%3F%23/jobs/job%2Fa%26%E9%9B%AA/input-artifacts`, { signal: controller.signal });
  fetch.mockResolvedValueOnce(new Response('unsupported', { status: 404 }));
  await expect(getJobInputArtifacts('tenant', 'job')).rejects.toThrow('404');
  fetch.mockImplementationOnce((_url, options) => new Promise((_resolve, reject) => options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))));
  const pending = getJobInputArtifacts('tenant', 'job', controller.signal);
  controller.abort();
  await expect(pending).rejects.toThrow('Aborted');
});

it.each([undefined, '1fac9e89-36cf-4a4f-a797-7d4fe887c65c', '3LfLqNNl2VVLo8Cg5a032Lm0igK', 'custom-ID'])('CID-002 CID-003 CID-009 preserves optional request ID %s and full returned identities', async jobId => {
 const identity = { tenant_id: 'Tenant-Case', job_id: '3LfLqNNl2VVLo8Cg5a032Lm0igK', goal_id: 'Goal-Complete', cell_id: 'cell', cell_name: 'platform', repo: 'github.com/acme/platform' };
 const fetchMock = vi.fn(async (_url: string, _options?: RequestInit) => new Response(JSON.stringify(identity)));
 vi.stubGlobal('fetch', fetchMock);
 const input = { cell: 'platform', repo: identity.repo, inputs: { prompt: 'hello', type: 'build' }, ...(jobId === undefined ? {} : { job_id: jobId }) };
 for (const files of [undefined, [new File(['binary\u0000bytes'], 'brief')]]) {
  expect(await submitJob(identity.tenant_id, input, files)).toEqual(identity);
  const body = fetchMock.mock.calls.at(-1)![1]!.body;
  const submission = JSON.parse(body instanceof FormData ? body.get('submission') as string : body as string);
  expect(submission).toEqual(input);
  if (jobId === undefined) expect(submission).not.toHaveProperty('job_id');
  if (body instanceof FormData) {
   const reader = new FileReader();
   const bytes = new Promise(resolve => { reader.onload = () => resolve(reader.result); });
   reader.readAsText(body.get('files') as File);
   expect(await bytes).toBe('binary\u0000bytes');
  }
 }
});
it.each(['job_id', 'goal_id', 'tenant_id', 'cell_id', 'cell_name', 'repo'])('CID-004 rejects missing returned %s for both transports', async field => {
 const identity: Record<string, string> = { tenant_id: 'tenant', job_id: 'job', goal_id: 'goal', cell_id: 'cell', cell_name: 'platform', repo: 'github.com/acme/platform' };
 delete identity[field];
 vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(identity))));
 for (const files of [undefined, [new File([], 'empty')]]) await expect(submitJob('tenant', { cell: 'platform' }, files)).rejects.toThrow();
});

it('TREE-026 encodes tree scope and opaque tokens, forwards abort, preserves generic summaries and errors', async () => {
  const {getTreeNode,getTreeChildren,APIError}=await import('./api');
  const signal=new AbortController().signal;
  const node={tenant_id:'t/雪',job_id:'j/#',job_type:'generic',status:'READY'};
  const fetch=vi.fn(async(url:string)=>new Response(JSON.stringify(url.includes('tree-children')?{nodes:[node],next_page_token:'opaque+/=',observed_at:'now'}:{node,observed_at:'now'})));
  vi.stubGlobal('fetch',fetch);
  expect((await getTreeNode('t/雪','j/#',signal)).node).toEqual(node);
  const result=await getTreeChildren('t/雪','j/#','opaque+/=',signal);expect(result.nodes[0].job_type).toBe('generic');
  const [url, options]=fetch.mock.calls[1] as unknown as [string,RequestInit];
  expect(url).toContain('/t%2F%E9%9B%AA/jobs/j%2F%23/tree-children');expect(new URL(url).searchParams.get('pageToken')).toBe('opaque+/=');expect(options.signal).toBe(signal);
  fetch.mockImplementation(async()=>new Response('{"error":"Execution tree records unavailable"}',{status:403}));
  await expect(getTreeNode('t','j')).rejects.toBeInstanceOf(APIError);
});

it('GS-009 serializes root scope independently and preserves continuation and failures', async () => {
  const fetch = vi.fn(async (_url: string) => new Response('{"jobs":[],"next_page_token":"opaque+/="}'));
  vi.stubGlobal('fetch', fetch);
  for (const rootOnly of [undefined, false, true]) {
    const page = await listJobs('tenant', { rootOnly, includeHidden: true, cellNames: ['alpha'], displayStatus: ['success'], pageToken: 'before+/=' });
    const query = new URL(fetch.mock.calls.at(-1)![0]).searchParams;
    expect(query.get('rootOnly')).toBe(rootOnly === undefined ? null : String(rootOnly));
    expect(query.get('includeHidden')).toBe('true');
    expect(query.get('cellNames')).toBe('alpha');
    expect(query.get('displayStatus')).toBe('success');
    expect(query.get('pageToken')).toBe('before+/=');
    expect(page.next_page_token).toBe('opaque+/=');
  }
  fetch.mockImplementation(async () => new Response('root filtering unsupported', { status: 501 }));
  await expect(listJobs('tenant', { rootOnly: true })).rejects.toThrow('root filtering unsupported');
  expect(fetch).toHaveBeenCalledTimes(4);
});
