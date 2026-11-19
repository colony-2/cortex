import { afterEach, expect, it, vi } from 'vitest';
import { ChangesReadError, reviewChangesService } from './reviewChangesService';

afterEach(() => vi.unstubAllGlobals());
it('encodes job, tenant, cursor and comparison selections and forwards cancellation', async () => {
  const fetcher = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }));
  vi.stubGlobal('fetch', fetcher);
  const signal = new AbortController().signal;
  await reviewChangesService.history('t/a', 'j?b', signal, 'cursor+&');
  let url = new URL(fetcher.mock.calls[0][0]);
  expect(url.pathname).toBe('/api/projects/t%2Fa/jobs/j%3Fb/changes');
  expect(url.searchParams.get('cursor')).toBe('cursor+&');
  expect(fetcher.mock.calls[0][1].signal).toBe(signal);
  fetcher.mockResolvedValue(new Response('{}', { status: 200 }));
  await reviewChangesService.diff('t', 'j', 'selection+&', { from: 'first', through: 'last' }, signal);
  url = new URL(fetcher.mock.calls[1][0]);
  expect(Object.fromEntries(url.searchParams)).toEqual({ selection: 'selection+&', from: 'first', through: 'last' });
  const download = new URL(reviewChangesService.downloadURL('t', 'j', 'selection+&', { all: true }));
  expect(Object.fromEntries(download.searchParams)).toEqual({ selection: 'selection+&', all: 'true', format: 'patch' });
});
it('preserves typed read errors and stale selection details', async () => {
  const diagnostic = { version: 1, code: 'stale_selection', detail: 'recorded state advanced' };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'Refresh changes', diagnostic }), { status: 409 })));
  const promise = reviewChangesService.history('t', 'j', new AbortController().signal);
  await expect(promise).rejects.toBeInstanceOf(ChangesReadError);
  await expect(promise).rejects.toMatchObject({ message: 'Refresh changes', diagnostic });
});
it('retains cancellation instead of converting it to an empty response', async () => {
  const error = new DOMException('Aborted', 'AbortError');
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(error));
  await expect(reviewChangesService.diff('t', 'j', 's', { commit: 'c' }, new AbortController().signal)).rejects.toBe(error);
});
