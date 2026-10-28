import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { getJob } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import { useReviewJobMetadata } from './useReviewJobMetadata';

vi.mock('@colony2/shared/api', () => ({ getJob: vi.fn() }));
afterEach(() => { vi.mocked(getJob).mockReset(); vi.clearAllMocks(); });
function deferred() {
  let resolve!: (job: RecipeJob) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<RecipeJob>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const job = (prompt: string) => ({ prompt } as RecipeJob);

it('deduplicates IDs and bounds outstanding requests across refresh generations, including failures', async () => {
  const requests: ReturnType<typeof deferred>[] = [];
  vi.mocked(getJob).mockImplementation(() => { const next = deferred(); requests.push(next); return next.promise; });
  const { result } = renderHook(() => useReviewJobMetadata('tenant', ['a', 'a', 'b', 'c', 'd', 'e', 'f']));
  expect(getJob).toHaveBeenCalledTimes(4);
  act(() => result.current.refresh());
  expect(getJob).toHaveBeenCalledTimes(4);
  await act(async () => requests[0].resolve(job('obsolete')));
  expect(getJob).toHaveBeenCalledTimes(5);
  expect(result.current.metadata.get('a')?.job).toBeUndefined();
  await act(async () => requests[1].reject(new Error('gone')));
  expect(getJob).toHaveBeenCalledTimes(6);
  // Four slots remain occupied; obsolete results did not create extra capacity.
  await act(async () => requests[4].resolve(job('current')));
  expect(result.current.metadata.get('a')?.job?.prompt).toBe('current');
  expect(getJob).toHaveBeenCalledTimes(7);
});

it('isolates same IDs across tenants and ignores removal/readdition and unmount responses', async () => {
  const requests: ReturnType<typeof deferred>[] = [];
  vi.mocked(getJob).mockImplementation(() => { const next = deferred(); requests.push(next); return next.promise; });
  const { result, rerender, unmount } = renderHook(({ tenant, ids }) => useReviewJobMetadata(tenant, ids), {
    initialProps: { tenant: 'first', ids: ['same'] },
  });
  rerender({ tenant: 'second', ids: ['same'] });
  expect(getJob).toHaveBeenLastCalledWith('second', 'same');
  await act(async () => requests[0].resolve(job('old tenant')));
  expect(result.current.metadata.get('same')?.job).toBeUndefined();
  rerender({ tenant: 'second', ids: [] });
  rerender({ tenant: 'second', ids: ['same'] });
  await act(async () => requests[1].resolve(job('old lifetime')));
  expect(result.current.metadata.get('same')?.job).toBeUndefined();
  await act(async () => requests[2].resolve(job('current')));
  expect(result.current.metadata.get('same')?.job?.prompt).toBe('current');
  unmount();
});

it('keeps successes after partial failure, skips unchanged IDs, and reloads on refresh or remount', async () => {
  vi.mocked(getJob).mockImplementation(async (_, id) => { if (id === 'bad') throw new Error('failed'); return job(id); });
  const { result, rerender, unmount } = renderHook(({ ids }) => useReviewJobMetadata('tenant', ids), { initialProps: { ids: ['good', 'bad'] } });
  await waitFor(() => expect(result.current.metadata.get('bad')?.status).toBe('error'));
  expect(result.current.metadata.get('good')?.job?.prompt).toBe('good');
  rerender({ ids: ['good', 'bad', 'new'] });
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(3));
  act(() => result.current.refresh());
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(6));
  unmount();
  renderHook(() => useReviewJobMetadata('tenant', ['good']));
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(7));
});

it('does not start queued work after unmount when outstanding requests settle', async () => {
  const requests: ReturnType<typeof deferred>[] = [];
  vi.mocked(getJob).mockImplementation(() => { const next = deferred(); requests.push(next); return next.promise; });
  const { unmount } = renderHook(() => useReviewJobMetadata('tenant', ['a', 'b', 'c', 'd', 'queued']));
  expect(getJob).toHaveBeenCalledTimes(4);
  unmount();
  await act(async () => { for (const request of requests) request.resolve(job('late')); });
  expect(getJob).toHaveBeenCalledTimes(4);
});
