import { expect, it } from 'vitest';
import type { PendingInput } from '@colony2/shared';
import { readReviewFilters, writeReviewFilters, reviewReturnSearch, reviewCellOptions, reviewSortOptions, visibleReviews } from './reviewFilters';
const defaults = readReviewFilters(new URLSearchParams());
const row = (id: string, extra: Partial<PendingInput> = {}): PendingInput => ({ id, cell_id: 'one', cell_name: 'Alpha', repo: 'github.com/test/one', ...extra });
it('distinguishes All/None/subsets, deduplicates IDs and retains absent IDs', () => {
  expect(defaults).toEqual({ q: '', cells: null, sort: 'requested', order: 'ascend' });
  expect(readReviewFilters(new URLSearchParams('cellSelection=none&cellId=one')).cells).toEqual([]);
  const subset = readReviewFilters(new URLSearchParams('cellId=absent&cellId=absent&cellId=&cellId=%20'));
  expect(subset.cells).toEqual(['absent']);
  expect(writeReviewFilters(subset).toString()).toBe('cellId=absent');
});
it.each(reviewSortOptions)('round trips $value and query text', ({ value }) => {
  const [sort, order] = value.split(':');
  const state = readReviewFilters(new URLSearchParams({ sort, order, q: ' foo + bar\n雪 ' }));
  expect(readReviewFilters(writeReviewFilters(state))).toEqual(state);
});
it.each(['sort=title', 'order=descend', 'sort=title&order=no', 'sort=no&order=descend', 'sort=&order='])('rejects invalid pair %s', query => {
  expect(readReviewFilters(new URLSearchParams(query))).toEqual(defaults);
});
it('canonicalizes detail allowlist and resets without deleting unrelated list fields', () => {
  const query = new URLSearchParams('q=foo+bar&cellId=absent&sort=requested&order=ascend&returnTo=https://bad&status=READY');
  expect(reviewReturnSearch(query)).toBe('?q=foo+bar&cellId=absent');
  expect(writeReviewFilters(defaults, query).toString()).toBe('returnTo=https%3A%2F%2Fbad&status=READY');
  expect(reviewReturnSearch(new URLSearchParams())).toBe('');
});
it('derives stable options before filtering, disambiguates labels and keeps selected fallbacks', () => {
  const rows = [row('a'), row('b'), row('c', { cell_id: 'two' }), row('d', { cell_id: 'three', cell_name: '' })];
  expect(reviewCellOptions(rows, ['absent'])).toEqual([
    { value: 'one', label: 'Alpha (one)' }, { value: 'two', label: 'Alpha (two)' },
    { value: 'absent', label: 'Selected cell (absent)' }, { value: 'three', label: 'three' },
  ]);
  expect(reviewCellOptions([row('a', { cell_name: 'Renamed' })], ['one'])[0].label).toBe('Renamed');
});
it('keeps subsets restrictive as the queue changes and combines cell OR with search AND', () => {
  const inputs = [row('a', { title: 'Food options at the bar' }), row('b', { cell_id: 'two', prompt: 'bar with food' }), row('c', { cell_id: 'three', title: 'Food bar' })];
  const state = { ...defaults, q: 'foo bar', cells: ['one', 'two'] };
  expect(visibleReviews(inputs, state).map(x => x.id)).toEqual(['a', 'b']);
  expect(visibleReviews(inputs, { ...state, cells: null })).toHaveLength(3);
  expect(visibleReviews(inputs, { ...state, cells: [] })).toHaveLength(0);
  expect(visibleReviews(inputs, { ...state, cells: ['absent'] })).toHaveLength(0);
});
it('searches independent full fields and IDs without fallbacks or cell names', () => {
  const inputs = [row('foo-bar'), row('title', { title: 'Food that i had at the bar' }), row('prompt', { prompt: 'Long '.repeat(100) + 'bar food' }), row('split', { title: 'Food', prompt: 'bar' }), row('fallback', { cell_name: 'Food bar' })];
  expect(visibleReviews(inputs, { ...defaults, q: 'foo bar' }).map(x => x.id)).toEqual(['foo-bar', 'prompt', 'title']);
  expect(visibleReviews([row('fallback')], { ...defaults, q: 'Review' })).toEqual([]);
});
it.each(['ascend', 'descend'] as const)('orders actual request instants with unknowns last: %s', order => {
  const inputs = Object.freeze([row('unknown-b', { requested_at: 'bad' }), row('new', { requested_at: '2026-11-10T10:00:00Z' }), row('old', { requested_at: '2026-11-10T12:00:00+03:00' }), row('unknown-a')].map(x => Object.freeze(x)));
  const result = visibleReviews(inputs as unknown as PendingInput[], { ...defaults, order });
  expect(result.map(x => x.id)).toEqual(order === 'ascend' ? ['old', 'new', 'unknown-a', 'unknown-b'] : ['new', 'old', 'unknown-a', 'unknown-b']);
  expect(inputs[0].id).toBe('unknown-b');
});
it.each(['title', 'cell'] as const)('sorts full %s keys in both directions with oldest-first identity ties', sort => {
  const inputs = [row('b', { title: 'Alpha', requested_at: '2026-11-02', cell_name: 'Alpha' }), row('a', { title: 'Alpha', requested_at: '2026-11-01', cell_name: 'Alpha' }), row('z', { prompt: 'Zeta', cell_name: 'Zeta' })];
  expect(visibleReviews(inputs, { ...defaults, sort }).map(x => x.id)).toEqual(['a', 'b', 'z']);
  expect(visibleReviews(inputs, { ...defaults, sort, order: 'descend' }).map(x => x.id)).toEqual(['z', 'a', 'b']);
});
it('uses prompt/title fallback and deterministic occurrence ties without reversing ties', () => {
  const inputs = [row('z', { title: 'Zeta' }), row('prompt', { prompt: 'Alpha' }), row('fallback')];
  expect(visibleReviews(inputs, { ...defaults, sort: 'title' }).map(x => x.id)).toEqual(['prompt', 'fallback', 'z']);
  const occurrences = [row('same', { task_ordinal: 2, request_id: 'b' }), row('same', { task_ordinal: 1 }), row('same', { task_ordinal: 2, request_id: 'a' })];
  expect(visibleReviews(occurrences, { ...defaults, order: 'descend' }).map(x => [x.task_ordinal, x.request_id])).toEqual([[1, undefined], [2, 'a'], [2, 'b']]);
});
