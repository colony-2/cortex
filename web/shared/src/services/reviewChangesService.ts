import { API_BASE } from '../apiBase';
import { APIError } from '../api';

export interface ChangesDiagnostic { version: number; code: string; detail?: string }
export interface WorkspaceDiffSelectors { task?: string; scope?: string; attempt?: number; workspace?: string; boundary?: 'before' | 'after' }
export interface WorkspaceDiffProducer { job_id: string; task: string; attempt: number }
export interface WorkspaceDiffMetadata {
  version: 1; status: 'available' | 'empty' | 'unavailable';
  requested: WorkspaceDiffSelectors & { job_id: string };
  resolved: { task?: string; scope?: string; attempt?: number; status: string; boundary: 'before' | 'after'; workspace?: string };
  observation: string; cutoff: string;
  workspace?: { repository: string; cell?: string; scope_id?: string; base_ref?: string; base_commit?: string; head_commit?: string; empty_base?: boolean };
  state_producer?: WorkspaceDiffProducer;
  source?: { producer: WorkspaceDiffProducer; key: { jobId: string; taskOrdinal: number; name: string; sizeBytes: number } };
  bytes?: number; reasons?: string[];
}
export interface WorkspaceDiffPreview { metadata: WorkspaceDiffMetadata; patch: string; preview_complete: boolean; download_url?: string }
export class ChangesReadError extends APIError {
  constructor(message: string, status: number, public diagnostic?: ChangesDiagnostic) { super(message, status, diagnostic?.code); }
}

const record = (v: unknown): v is Record<string, any> => !!v && typeof v === 'object' && !Array.isArray(v);
const nonblank = (v: unknown): v is string => typeof v === 'string' && !!v.trim();
const count = (v: unknown) => Number.isSafeInteger(v) && (v as number) >= 0;
const producer = (v: unknown) => record(v) && nonblank(v.job_id) && nonblank(v.task) && count(v.attempt) && v.attempt > 0;
function invalid(): never { throw new ChangesReadError('Invalid saved workspace diff response. Upgrade Cortex and refresh changes.', 502, { version: 1, code: 'protocol' }); }

// A download must be the exact authorized artifact route supplied by Cortex.
// Compare to the resolved source, including encoded opaque tenant/name segments.
function artifactURL(tenant: string, source: NonNullable<WorkspaceDiffMetadata['source']>) {
  const k = source.key;
  return `${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(k.jobId)}/tasks/${k.taskOrdinal}/artifacts/${encodeURIComponent(k.name)}`;
}
// Go PathEscape and encodeURIComponent escape different characters. Normalize
// each segment separately: an encoded slash must remain inside its source ID,
// and a literal plus must never become a space as it would in a query string.
function canonicalPath(path: string) {
  return path.split('/').map(segment => encodeURIComponent(decodeURIComponent(segment))).join('/');
}
function validate(value: unknown, tenant: string, job: string, selectors: WorkspaceDiffSelectors): WorkspaceDiffPreview {
  if (!record(value) || !record(value.metadata) || typeof value.patch !== 'string' || typeof value.preview_complete !== 'boolean') invalid();
  const m = value.metadata;
  const expected = { job_id: job, ...selectors };
  if (m.version !== 1 || !['available', 'empty', 'unavailable'].includes(m.status) || !record(m.requested) ||
      Object.keys(m.requested).length !== Object.keys(expected).length || Object.entries(expected).some(([k,v]) => m.requested[k] !== v) ||
      !record(m.resolved) || !['before', 'after'].includes(m.resolved.boundary) || !nonblank(m.resolved.status) ||
      !nonblank(m.observation) || !nonblank(m.cutoff)) invalid();
  if (m.resolved.task && m.resolved.scope || m.resolved.task !== undefined && !nonblank(m.resolved.task) || m.resolved.scope !== undefined && !nonblank(m.resolved.scope) ||
      m.resolved.attempt !== undefined && (!count(m.resolved.attempt) || m.resolved.attempt < 1) ||
      selectors.task !== undefined && m.resolved.task !== selectors.task || selectors.scope !== undefined && m.resolved.scope !== selectors.scope ||
      selectors.attempt !== undefined && m.resolved.attempt !== selectors.attempt || m.resolved.boundary !== (selectors.boundary ?? 'after')) invalid();
  if (m.bytes !== undefined && !count(m.bytes) || m.reasons !== undefined && (!Array.isArray(m.reasons) || !m.reasons.every(nonblank))) invalid();
  if (m.state_producer !== undefined && !producer(m.state_producer)) invalid();
  if (m.workspace !== undefined && (!record(m.workspace) || !nonblank(m.workspace.repository) ||
      ['cell', 'scope_id', 'base_ref', 'base_commit', 'head_commit'].some(k => m.workspace[k] !== undefined && !nonblank(m.workspace[k])) ||
      m.workspace.empty_base !== undefined && typeof m.workspace.empty_base !== 'boolean')) invalid();
  if (m.source !== undefined && (!record(m.source) || !producer(m.source.producer) || !record(m.source.key) ||
      m.source.key.jobId !== m.source.producer.job_id || !count(m.source.key.taskOrdinal) || !['diff_from_base.diff', 'diff_from_parent.diff'].includes(m.source.key.name) ||
      !Number.isSafeInteger(m.source.key.sizeBytes) || m.source.key.sizeBytes < -1 ||
      m.bytes !== undefined && m.source.key.sizeBytes >= 0 && m.bytes !== m.source.key.sizeBytes)) invalid();
  if (m.status === 'unavailable') {
    if (!m.reasons?.length || m.source !== undefined || m.bytes !== undefined || value.patch !== '' || !value.preview_complete || value.download_url !== undefined) invalid();
  } else {
    if (!m.workspace || !producer(m.state_producer) || m.state_producer.job_id !== job || m.resolved.workspace !== m.workspace.scope_id ||
        selectors.workspace !== undefined && selectors.workspace !== 'main' && selectors.workspace !== m.workspace.scope_id ||
        m.workspace.empty_base && m.workspace.base_commit || !m.workspace.empty_base && !m.workspace.base_commit) invalid();
    if (m.status === 'available' && (!m.source || m.bytes === 0) || m.status === 'empty' && (m.bytes !== 0 || value.patch !== '' || !value.preview_complete)) invalid();
    if (!m.source && (m.status !== 'empty' || !(m.workspace.base_commit && m.workspace.base_commit === m.workspace.head_commit || m.workspace.empty_base && !m.workspace.head_commit))) invalid();
    if (m.source) {
      if (typeof value.download_url !== 'string') invalid();
      const expectedURL = new URL(artifactURL(tenant, m.source as NonNullable<WorkspaceDiffMetadata['source']>), window.location.origin);
      let actual: URL;
      try {
        actual = new URL(value.download_url, window.location.origin);
        actual.pathname = canonicalPath(actual.pathname);
        expectedURL.pathname = canonicalPath(expectedURL.pathname);
      } catch { return invalid(); }
      if (actual.origin !== window.location.origin || actual.href !== expectedURL.href || actual.username || actual.password) invalid();
    } else if (value.download_url !== undefined) invalid();
  }
  return value as unknown as WorkspaceDiffPreview;
}

export const reviewChangesService = {
  async workspaceDiff(tenant: string, job: string, selectors: WorkspaceDiffSelectors, signal: AbortSignal): Promise<WorkspaceDiffPreview> {
    const query = new URLSearchParams(Object.entries(selectors).map(([k,v]) => [k, String(v)]));
    const url = `${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/workspace-diff${query.size ? `?${query}` : ''}`;
    const response = await fetch(url, { signal });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new ChangesReadError(body.error || 'Could not read saved workspace diff.', response.status, body.diagnostic);
    }
    let body: unknown;
    try { body = await response.json(); } catch { return invalid(); }
    return validate(body, tenant, job, selectors);
  },
};
