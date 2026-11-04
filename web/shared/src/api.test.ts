import { afterEach, expect, it, vi } from 'vitest';
import { getJob, listJobs, listCells, getCell, createCell } from './api';
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
