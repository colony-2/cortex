import type { RecipeJobFilterStatus } from '@colony2/shared/types';
import { isJobFilterStatus } from './jobStatus';

export function readJobFilters(params: URLSearchParams): { status: RecipeJobFilterStatus[] | null; cell: string[] | null } {
  // Older links used COMPLETED for the "outcome unavailable" filter label.
  const status = params.getAll('status').map(value => value === 'COMPLETED' ? 'OUTCOME_UNAVAILABLE' : value).filter(isJobFilterStatus);
  const cells = [...new Set(params.getAll('cell').filter(Boolean))];
  // No filter means all; an explicit empty selection means no rows.
  // Use a separate flag for cells so a cell named "none" remains selectable.
  return {
    status: params.getAll('status').includes('none') ? [] : status.length ? status : null,
    cell: params.get('cellSelection') === 'none' ? [] : cells.length ? cells : null,
  };
}

// Preserve list state through explicit Back to Goals navigation as well as browser history.
// Story selection has separate query fields and is deliberately excluded.
export function jobFilterSearch(params: URLSearchParams): string {
  const filters = readJobFilters(params);
  const query = new URLSearchParams();
  if (filters.status?.length === 0) query.set('status', 'none');
  for (const status of filters.status || []) query.append('status', status);
  if (filters.cell?.length === 0) query.set('cellSelection', 'none');
  for (const cell of filters.cell || []) query.append('cell', cell);
  for (const key of ['q', 'sort', 'order', 'expanded', 'detail', 'inspect', 'pageToken']) {
    for (const value of params.getAll(key)) query.append(key, value);
  }
  return query.size ? `?${query}` : '';
}
