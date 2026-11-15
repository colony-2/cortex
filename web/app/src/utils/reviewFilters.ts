import type { PendingInput } from '@colony2/shared';
import { compareReviewRequests, requestTime } from './reviewAge';
import { matchesSearchTerms, searchTerms } from './searchTerms';

export type ReviewFilters = {
  q: string;
  cells: string[] | null;
  sort: 'requested' | 'title' | 'cell';
  order: 'ascend' | 'descend';
};
const keys = ['q', 'cellId', 'cellSelection', 'sort', 'order'];
export const reviewSortOptions = [
  { value: 'requested:ascend', label: 'Requested: oldest first' },
  { value: 'requested:descend', label: 'Requested: newest first' },
  { value: 'title:ascend', label: 'Title: A–Z' },
  { value: 'title:descend', label: 'Title: Z–A' },
  { value: 'cell:ascend', label: 'Cell: A–Z' },
  { value: 'cell:descend', label: 'Cell: Z–A' },
];
export function readReviewFilters(params: URLSearchParams): ReviewFilters {
  const cells = [...new Set(params.getAll('cellId').filter(id => id.trim()))];
  const sort = params.get('sort'), order = params.get('order');
  const valid = (sort === 'requested' || sort === 'title' || sort === 'cell') && (order === 'ascend' || order === 'descend');
  return { q: params.get('q') || '', cells: params.get('cellSelection') === 'none' ? [] : cells.length ? cells : null,
    sort: valid ? sort : 'requested', order: valid ? order : 'ascend' };
}
export function writeReviewFilters(filters: ReviewFilters, base = new URLSearchParams()): URLSearchParams {
  const params = new URLSearchParams(base);
  keys.forEach(key => params.delete(key));
  if (filters.q) params.set('q', filters.q);
  if (filters.cells?.length === 0) params.set('cellSelection', 'none');
  else filters.cells?.forEach(id => params.append('cellId', id));
  if (filters.sort !== 'requested' || filters.order !== 'ascend') {
    params.set('sort', filters.sort); params.set('order', filters.order);
  }
  return params;
}
export function reviewReturnSearch(params: URLSearchParams): string {
  const query = writeReviewFilters(readReviewFilters(params)).toString();
  return query ? `?${query}` : '';
}
export const reviewTitle = (input: PendingInput) => input.title || input.prompt || 'Review';
export const reviewCell = (input: PendingInput) => input.cell_name?.trim() ? input.cell_name : input.cell_id;

export function reviewCellOptions(inputs: PendingInput[], selected: string[] | null) {
  const names = new Map(inputs.map(input => [input.cell_id, reviewCell(input)]));
  const counts = new Map<string, number>();
  names.forEach(name => counts.set(name, (counts.get(name) || 0) + 1));
  for (const id of selected || []) if (!names.has(id)) names.set(id, `Selected cell (${id})`);
  return [...names].map(([value, name]) => ({ value, label: (counts.get(name) || 0) > 1 ? `${name} (${value})` : name }))
    .sort((a, b) => a.label.localeCompare(b.label) || a.value.localeCompare(b.value));
}
export function compareReviews(a: PendingInput, b: PendingInput, filters: ReviewFilters): number {
  const direction = filters.order === 'descend' ? -1 : 1;
  let primary = 0;
  if (filters.sort === 'requested') {
    const left = requestTime(a), right = requestTime(b);
    if (Number.isFinite(left) !== Number.isFinite(right)) return Number.isFinite(left) ? -1 : 1;
    if (Number.isFinite(left) && Number.isFinite(right)) primary = left - right;
  } else {
    const key = filters.sort === 'title' ? reviewTitle : reviewCell;
    primary = key(a).localeCompare(key(b));
  }
  return primary * direction || compareReviewRequests(a, b);
}
export function visibleReviews(inputs: PendingInput[], filters: ReviewFilters): PendingInput[] {
  const terms = searchTerms(filters.q);
  return inputs.filter(input => (filters.cells === null || filters.cells.includes(input.cell_id))
    && [input.title, input.prompt, input.id].some(text => matchesSearchTerms(terms, text)))
    .sort((a, b) => compareReviews(a, b, filters));
}
