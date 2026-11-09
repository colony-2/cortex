import { afterEach, expect, it, vi } from 'vitest';
import { APIError, getJobLeaseEvents, getJob, listJobs } from './api';
import { leaseEvent } from './test/leaseEventFixtures';

afterEach(() => vi.unstubAllGlobals());
it('forwards exactly one opaque page and abort signal without precision, ID or order changes', async () => {
  const page = { events: [leaseEvent(), leaseEvent('1')], nextPageToken: 'opaque+/=雪' };
  const fetch = vi.fn(async () => new Response(JSON.stringify(page)));
  vi.stubGlobal('fetch', fetch);
  const signal = new AbortController().signal;
  expect(await getJobLeaseEvents('tenant', 'job', { pageSize: 2, pageToken: 'token+/=雪' }, signal)).toEqual(page);
  expect(fetch).toHaveBeenCalledTimes(1);
  const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
  expect(new URL(url).pathname).toBe('/api/projects/tenant/jobs/job/events');
  expect([...new URL(url).searchParams]).toEqual([['pageSize', '2'], ['pageToken', 'token+/=雪']]);
  expect(init.signal).toBe(signal);
});
it.each([[400,'invalid_request'],[400,'invalid_lease_event_page_token'],[401,'authentication_required'],[403,'lease_permission_denied'],[403,'cell_forbidden'],[404,'cell_not_found'],[501,'unsupported_lease_events'],[503,'backend_unavailable']])('retains HTTP %s and error code %s', async (status, code) => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'failure', code }), { status: Number(status) })));
  await expect(getJobLeaseEvents('tenant', 'job')).rejects.toMatchObject({ status, code });
});
it.each([{ events: null }, {}, { events: [leaseEvent('1', { eventType: 'expired' as never })] }, { events: [leaseEvent('1', { eventId: 1 as never })] }, { events: [leaseEvent('1', { jobKey: { tenantId: 'other', jobId: 'job' } })] }, { events: [], nextPageToken: 3 }])('rejects incompatible response %# without an empty fallback', async page => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(page))));
  await expect(getJobLeaseEvents('tenant', 'job')).rejects.toBeInstanceOf(APIError);
});
it('preserves abort rejection and does not retry', async () => {
  const fetch = vi.fn(async (_url, init) => new Promise((_resolve, reject) => init.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))));
  vi.stubGlobal('fetch', fetch);
  const controller = new AbortController();
  const request = getJobLeaseEvents('tenant', 'job', {}, controller.signal);
  controller.abort();
  await expect(request).rejects.toMatchObject({ name: 'AbortError' });
  expect(fetch).toHaveBeenCalledTimes(1);
});
it('ordinary list and detail never enrich with history', async () => {
  const job = { job_id: 'job', cell_id: 'id', cell_name: 'cell', repo: 'github.com/a/b' };
  const fetch = vi.fn(async (url: string) => new Response(JSON.stringify(url.endsWith('/job') ? job : { jobs: [job] })));
  vi.stubGlobal('fetch', fetch);
  await listJobs('tenant'); await getJob('tenant', 'job');
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch.mock.calls.every(([url]) => !url.includes('/events'))).toBe(true);
});
