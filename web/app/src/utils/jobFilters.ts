import type { RecipeJobStatus } from '@colony2/shared/types';
import { schedulingStatuses } from './jobStatus';

export function readJobFilters(params: URLSearchParams): { status: RecipeJobStatus[]; cell?: string } {
  const status = params.getAll('status').filter((value): value is RecipeJobStatus =>
    Object.prototype.hasOwnProperty.call(schedulingStatuses, value));
  return { status, cell: params.get('cell') || undefined };
}

// Carry only list filters through story links; story selection has its own query fields.
export function jobFilterSearch(params: URLSearchParams): string {
  const filters = readJobFilters(params);
  const query = new URLSearchParams();
  for (const status of filters.status) query.append('status', status);
  if (filters.cell) query.set('cell', filters.cell);
  return query.size ? `?${query}` : '';
}
