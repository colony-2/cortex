import { API_BASE } from '../apiBase';
import { APIError } from '../api';

export interface ChangesDiagnostic { version: number; code: string; detail?: string; operation?: string; missing?: string[]; limit?: string }
export interface ChangesProducer { job_id: string; task: string; attempt: number }
export interface ChangesIdentity {
  version: number; tenant: string; job_id: string; repository: string; cell?: string; scope: string;
  state: ChangesProducer; boundary: string; active_base?: string; active_base_origin: string;
  base?: string; base_origin: 'active' | 'explicit'; empty_base: boolean; unborn: boolean;
  head?: string; base_tree: string; head_tree: string; object_format: string;
  sources: { producer: ChangesProducer; key: { jobId: string; taskOrdinal: number; name: string; sizeBytes?: number } }[];
  repository_objects?: string[]; source_id: string; selection: string;
}
export interface ChangesCommit {
  hash: string; tree: string; parents: string[]; subject: string; position: number;
  author: { name: string; email: string; time: string }; committer: { name: string; email: string; time: string };
}
export interface ChangesHistory extends ChangesIdentity {
  diff: 'complete' | 'unavailable'; history: 'complete' | 'unsupported'; diagnostic?: ChangesDiagnostic;
  complete: boolean; total: number; commits: ChangesCommit[]; next_cursor?: string;
}
export interface ChangesDiff extends ChangesIdentity {
  history: 'complete' | 'unsupported'; diagnostic?: ChangesDiagnostic; diff: 'complete'; complete: boolean;
  from_commit?: string; to_commit: string; from_tree: string; to_tree: string; bytes: number;
}
export interface ChangesPreview { metadata: ChangesDiff; patch: string; preview_complete: boolean }
export type ChangesComparison = { all: true } | { commit: string } | { from: string; through: string };

export class ChangesReadError extends APIError {
  constructor(message: string, status: number, public diagnostic?: ChangesDiagnostic) {
    super(message, status, diagnostic?.code);
  }
}

function root(tenant: string, job: string) {
  return `${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/changes`;
}
async function read<T>(url: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ChangesReadError(body.error || 'Could not read changes.', response.status, body.diagnostic);
  }
  return response.json();
}
function comparisonQuery(selection: string, comparison: ChangesComparison) {
  return new URLSearchParams({ selection, ...Object.fromEntries(Object.entries(comparison).map(([k, v]) => [k, String(v)])) });
}
export const reviewChangesService = {
  history(tenant: string, job: string, signal: AbortSignal, cursor?: string): Promise<ChangesHistory> {
    const query = new URLSearchParams({ limit: '50', ...(cursor ? { cursor } : {}) });
    return read(`${root(tenant, job)}?${query}`, signal);
  },
  diff(tenant: string, job: string, selection: string, comparison: ChangesComparison, signal: AbortSignal): Promise<ChangesPreview> {
    return read(`${root(tenant, job)}/diff?${comparisonQuery(selection, comparison)}`, signal);
  },
  downloadURL(tenant: string, job: string, selection: string, comparison: ChangesComparison): string {
    const query = comparisonQuery(selection, comparison);
    query.set('format', 'patch');
    return `${root(tenant, job)}/diff?${query}`;
  },
};
