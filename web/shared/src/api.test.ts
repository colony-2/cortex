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
