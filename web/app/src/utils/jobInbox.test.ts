import { describe, expect, it } from 'vitest';
import {
  inboxJobKey, inboxPendingBadge, inboxPendingCounts,
  isInInbox, rankInboxCells, visibleInboxCells,
} from './jobInbox';

describe('job inbox identities and explicit membership', () => {
  it('JI-01 keeps jobs distinct despite duplicate titles and goal membership', () => {
    const jobs = ['first', 'second'].map(jobId => ({ tenantId: 'tenant', jobId, title: 'Same', goalId: 'shared' }));
    expect(new Set(jobs.map(inboxJobKey)).size).toBe(2);
    expect(inboxJobKey({ tenantId: 'a/b', jobId: 'c' })).not.toBe(inboxJobKey({ tenantId: 'a', jobId: 'b/c' }));
    expect(inboxJobKey({ tenantId: 'one', jobId: 'same' })).not.toBe(inboxJobKey({ tenantId: 'two', jobId: 'same' }));
  });

  it('JI-02 includes completed visible jobs regardless of execution storage', () => {
    const completed = { archived: false, cellId: 'cell', status: 'COMPLETED', store: 'ARCHIVED' };
    expect(isInInbox(completed, { kind: 'all' })).toBe(true);
    expect(isInInbox(completed, { kind: 'archive' })).toBe(false);
    const hidden = { ...completed, archived: true };
    expect(isInInbox(hidden, { kind: 'all' })).toBe(false);
    expect(isInInbox(hidden, { kind: 'cell', cellId: 'cell' })).toBe(false);
    expect(isInInbox(hidden, { kind: 'archive' })).toBe(true);
  });

  it('JI-03 uses stable cell IDs even when names collide', () => {
    expect(isInInbox({ archived: false, cellId: 'first' }, { kind: 'cell', cellId: 'first' })).toBe(true);
    expect(isInInbox({ archived: false, cellId: 'second' }, { kind: 'cell', cellId: 'first' })).toBe(false);
  });
});

describe('cell navigation', () => {
  it('JI-04 ranks full unarchived totals, then names and IDs, independently of pending', () => {
    const cells = [
      { id: 'z', name: 'alpha', unarchivedJobs: 2, pending: 2 },
      { id: 'b', name: 'alpha', unarchivedJobs: 2, pending: 0 },
      { id: 'a', name: 'beta', unarchivedJobs: 2, pending: 1 },
      { id: 'c', name: 'busy', unarchivedJobs: 10, pending: 0 },
      { id: 'd', name: 'empty', unarchivedJobs: 0, pending: 0 },
    ];
    expect(rankInboxCells(cells).map(cell => cell.id)).toEqual(['c', 'b', 'z', 'a', 'd']);
    expect(cells.map(cell => cell.id)).toEqual(['z', 'b', 'a', 'c', 'd']);
  });

  it.each([-1, 1.5, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1])('rejects invalid total %s instead of ranking it as zero', total => {
    expect(() => rankInboxCells([{ id: 'cell', name: 'Cell', unarchivedJobs: total }])).toThrow('Cell totals are unavailable');
  });

  it('rejects duplicate or unavailable attribution', () => {
    const cell = { id: 'one', name: 'Cell', unarchivedJobs: 0 };
    expect(() => rankInboxCells([cell, cell])).toThrow();
    expect(() => rankInboxCells([{ ...cell, id: '' }])).toThrow();
    expect(() => rankInboxCells([{ ...cell, name: ' ' }])).toThrow();
  });

  it('JI-19 keeps the selected low-ranked cell reachable among forty cells', () => {
    const cells = rankInboxCells(Array.from({ length: 40 }, (_, index) => ({
      id: `cell-${index}`, name: `Cell ${index}`, unarchivedJobs: 40 - index,
    })));
    expect(visibleInboxCells(cells)).toEqual(cells.slice(0, 8));
    expect(visibleInboxCells(cells, 'cell-39')).toEqual([...cells.slice(0, 8), cells[39]]);
    expect(visibleInboxCells(cells, 'cell-0')).toHaveLength(8);
    expect(visibleInboxCells(cells, 'unavailable')).toHaveLength(8);
    expect(visibleInboxCells([])).toEqual([]);
  });
});

describe('pending-response badges', () => {
  const current = { tenantId: 'tenant', status: 'current' as const, inputs: [
    { id: 'review', cell_id: 'one' }, { id: 'ordinary-input', cell_id: 'one' }, { id: 'other', cell_id: 'two' },
  ] };

  it('JI-05 counts discovery jobs by stable cell, not their read status or questions', () => {
    const counts = inboxPendingCounts('tenant', current);
    expect(counts).toEqual({ status: 'current', total: 3, byCell: new Map([['one', 2], ['two', 1]]) });
    expect(inboxPendingBadge(counts, { kind: 'all' })).toBe(3);
    expect(inboxPendingBadge(counts, { kind: 'cell', cellId: 'one' })).toBe(2);
    // Reading affects neither the discovery snapshot nor the count.
    expect(inboxPendingCounts('tenant', current)).toEqual(counts);
  });

  it('JI-06 omits confirmed zero and every Archive badge', () => {
    const counts = inboxPendingCounts('tenant', current);
    expect(inboxPendingBadge(counts, { kind: 'cell', cellId: 'empty' })).toBeUndefined();
    const empty = inboxPendingCounts('tenant', { ...current, inputs: [] });
    expect(inboxPendingBadge(empty, { kind: 'all' })).toBeUndefined();
    for (const value of [counts, empty, { status: 'unavailable' as const }, { status: 'loading' as const }]) {
      expect(inboxPendingBadge(value, { kind: 'archive' })).toBeUndefined();
    }
  });

  it.each(['loading', 'unavailable'] as const)('JI-07 distinguishes %s from confirmed empty', status => {
    const counts = inboxPendingCounts('tenant', { ...current, status });
    expect(counts).toEqual({ status });
    expect(inboxPendingBadge(counts, { kind: 'all' })).toBe('?');
    expect(inboxPendingBadge(counts, { kind: 'cell', cellId: 'one' })).toBe('?');
  });

  it('JI-07 ignores another tenant snapshot even with overlapping job and cell IDs', () => {
    expect(inboxPendingCounts('another-tenant', current)).toEqual({ status: 'loading' });
  });

  it('JI-08 fails closed on duplicate jobs or missing attribution', () => {
    for (const inputs of [
      [current.inputs[0], current.inputs[0]],
      [{ id: '', cell_id: 'one' }], [{ id: 'job', cell_id: ' ' }],
    ]) {
      const counts = inboxPendingCounts('tenant', { ...current, inputs });
      expect(counts).toEqual({ status: 'unavailable' });
      expect(inboxPendingBadge(counts, { kind: 'all' })).toBe('?');
    }
  });

  it('JI-10 preserves pending through continuing questions and removes the last badge on confirmed closure', () => {
    const request = { ...current, inputs: [current.inputs[0]] };
    expect(inboxPendingBadge(inboxPendingCounts('tenant', request), { kind: 'cell', cellId: 'one' })).toBe(1);
    expect(inboxPendingBadge(inboxPendingCounts('tenant', { ...request, inputs: [] }), { kind: 'cell', cellId: 'one' })).toBeUndefined();
  });
});
