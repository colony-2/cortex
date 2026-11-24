import { beforeEach, expect, it, vi } from 'vitest';
import { listJobs } from '@colony2/shared/api';
import { loadArchivedInbox, loadVisibleInboxPage, inboxDate, inboxRevision, inboxReadKey } from './jobInboxData';
import type { RecipeJobSummary } from '@colony2/shared/types';
vi.mock('@colony2/shared/api', () => ({ listJobs: vi.fn() }));
const job = (id: string, extra = {}) => ({ tenant_id: 'tenant', job_id: id, cell_id: 'cell', cell_name: 'Cell', status: 'COMPLETED', store: 'ARCHIVED', ...extra }) as RecipeJobSummary;
beforeEach(() => vi.mocked(listJobs).mockReset());
it('reads one page, passes the cell selector upstream, and verifies stable cell IDs', async () => {
  const signal = new AbortController().signal;
  vi.mocked(listJobs).mockResolvedValue({ jobs: [job('first'), job('other-cell', { cell_id: 'other' })], next_page_token: 'next' });
  const result = await loadVisibleInboxPage('tenant', { id: 'cell', name: 'Cell' }, undefined, signal);
  expect(listJobs).toHaveBeenCalledExactlyOnceWith('tenant', { includeHidden: false, status: [], pageSize: 200, pageToken: undefined, cellNames: ['Cell'] }, signal);
  expect(result).toEqual({ jobs: [{ ...job('first'), inboxArchived: false }], nextPageToken: 'next' });
});
it('traverses complete pages before computing explicit hiding, including completed visible jobs', async () => {
  vi.mocked(listJobs).mockImplementation(async (_, options) => options?.pageToken ? { jobs: [job('second')] } : { jobs: options?.includeHidden ? [job('first'), job('hidden')] : [job('first')], next_page_token: 'next' });
  const rows = await loadArchivedInbox('tenant');
  expect(rows.map(row => [row.job_id, row.inboxArchived])).toEqual([['hidden', true]]);
  expect(listJobs).toHaveBeenCalledTimes(6);
  expect(vi.mocked(listJobs).mock.calls.every(([, options]) => options?.status?.length === 0)).toBe(true);
});
it('rejects membership changes instead of turning a racing visible job into archived work', async () => {
  vi.mocked(listJobs).mockResolvedValueOnce({ jobs: [job('first')] }).mockResolvedValueOnce({ jobs: [job('first'), job('new')] }).mockResolvedValueOnce({ jobs: [job('first'), job('new')] });
  await expect(loadArchivedInbox('tenant')).rejects.toThrow('membership changed');
});
it('rejects wrong-tenant jobs and repeated continuations', async () => {
  vi.mocked(listJobs).mockResolvedValue({ jobs: [job('foreign', { tenant_id: 'other' })] });
  await expect(loadArchivedInbox('tenant')).rejects.toThrow('changed while loading');
  vi.mocked(listJobs).mockResolvedValue({ jobs: [], next_page_token: 'repeated' });
  await expect(loadArchivedInbox('tenant')).rejects.toThrow('cursor did not advance');
});
it('stops superseded requests without issuing more reads', async () => {
  vi.mocked(listJobs).mockResolvedValue({ jobs: [job('first')], next_page_token: 'more' });
  await expect(loadArchivedInbox('tenant', AbortSignal.abort())).rejects.toThrow('superseded');
  expect(listJobs).toHaveBeenCalledTimes(1);
});
it('uses recorded activity first, then submission or creation, and skips invalid dates and lease expiry', () => {
  const base = job('first', { created_at: '2026-01-01T00:00:00Z', lease_expires_at: '2026-01-02T00:00:00Z' });
  expect(inboxDate(base)).toBe(base.created_at);
  expect(inboxDate({ ...base, submitted_at: '0001-01-01T00:00:00Z' }, 'invalid')).toBe(base.created_at);
  const submitted = { ...base, submitted_at: '2026-01-01T06:00:00Z' };
  expect(inboxDate(submitted)).toBe(submitted.submitted_at);
  const archived = { ...submitted, archived_at: '2026-01-02T00:00:00Z' };
  expect(inboxDate(archived)).toBe(archived.archived_at);
  expect(inboxDate(archived, '2026-01-03T00:00:00Z')).toBe('2026-01-03T00:00:00Z');
  expect(inboxDate({ ...base, created_at: 'invalid' })).toBeUndefined();
  expect(inboxRevision(submitted)).toBe(inboxRevision(base));
});
it('uses separate browser read identities and detects new requests independently of read state', () => {
  const base = { ...job('first'), inboxArchived: false };
  expect(inboxReadKey('one', 'tenant', 'first')).not.toBe(inboxReadKey('two', 'tenant', 'first'));
  expect(inboxReadKey('one', 'tenant', 'first')).not.toBe(inboxReadKey('one', 'other', 'first'));
  expect(inboxRevision(base, undefined, 'new')).not.toBe(inboxRevision(base, undefined, 'old'));
});
