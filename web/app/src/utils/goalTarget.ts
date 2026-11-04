import type { CortexCell } from '@colony2/shared/types';

export const cellNameRules = [
  { required: true, message: 'Enter a cell name' },
  { pattern: /^(?!\.{1,2}$)[A-Za-z0-9._-]{1,128}$/, message: 'Use 1–128 letters, numbers, dots, underscores or hyphens' },
];
export function goalTargets(cells: CortexCell[]) {
  return cells.filter(cell => cell.state === 'ACTIVE').map(cell => ({ value: cell.id, label: `${cell.name} · ${cell.repo}` }));
}
export function cellPath(tenant: string, id: string) {
  return `/project/${encodeURIComponent(tenant)}/cells/${encodeURIComponent(id)}`;
}
export function cellGoalsPath(tenant: string, id: string) {
  return `/project/${encodeURIComponent(tenant)}/goals?${new URLSearchParams({ cellId: id, executionState: 'cells-v1' })}`;
}
