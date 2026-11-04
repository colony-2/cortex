import type { CortexCell } from './types';

export function validateJobCell(value: unknown): void {
  const job = value as Record<string, unknown> | null;
  for (const key of ['cell_id', 'cell_name', 'repo']) {
    if (!job || typeof job[key] !== 'string' || !(job[key] as string).trim()) {
      throw new Error(`Invalid job response: required ${key} is missing. Reload to retry.`);
    }
  }
}

export function validateCell(cell: CortexCell, tenantId: string): void {
  if (!cell || !cell.id || !cell.name || !cell.repo || !cell.revision || cell.tenantId !== tenantId
    || !['ACTIVE', 'RETIRED'].includes(cell.state)) {
    throw new Error('Invalid cell registry response. Reload to retry.');
  }
}

export function createRequestId(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('');
}
