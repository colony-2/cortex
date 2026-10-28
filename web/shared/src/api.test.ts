import { afterEach, expect, it, vi } from 'vitest';
import { listJobs } from './api';

afterEach(() => vi.unstubAllGlobals());

it('requests all statuses when the status filter is explicitly cleared', async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ jobs: [] }) });
  vi.stubGlobal('fetch', fetch);
  await listJobs('tenant', { status: [], cell: 'app' });
  const query = new URL(fetch.mock.calls[0][0]).searchParams;
  expect(query.getAll('status')).toEqual(['all']);
  expect(query.get('cell')).toBe('app');

  await listJobs('tenant', { status: ['READY', 'COMPLETED'] });
  expect(new URL(fetch.mock.calls[1][0]).searchParams.getAll('status')).toEqual(['READY', 'COMPLETED']);

  await listJobs('tenant');
  expect(new URL(fetch.mock.calls[2][0]).searchParams.has('status')).toBe(false);
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
