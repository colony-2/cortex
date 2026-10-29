import type { RecipeJobFilterStatus } from '@colony2/shared/types';
import { isJobFilterStatus } from './jobStatus';

export function readJobFilters(params: URLSearchParams): { status: RecipeJobFilterStatus[]; cell?: string } {
  // Older links used COMPLETED for the "outcome unavailable" filter label.
  const status = params.getAll('status').map(value => value === 'COMPLETED' ? 'OUTCOME_UNAVAILABLE' : value).filter(isJobFilterStatus);
  return { status, cell: params.get('cell') || undefined };
}

// Preserve list state through explicit Back to Goals navigation as well as browser history.
// Story selection has separate query fields and is deliberately excluded.
export function jobFilterSearch(params: URLSearchParams): string {
  const filters = readJobFilters(params);
  const query = new URLSearchParams();
  for (const status of filters.status) query.append('status', status);
  if (filters.cell) query.set('cell', filters.cell);
  for (const key of ['expanded', 'detail', 'inspect', 'pageToken']) {
    for (const value of params.getAll(key)) query.append(key, value);
  }
  return query.size ? `?${query}` : '';
}
