import { API_BASE } from './apiBase';
import { APIError } from './api';
import { parseJobJSON } from './jobJSON';

export interface HistoryValue { present: boolean; raw?: unknown; source?: string }
export interface HistoryDiagnostic { code: string; message: string; entry_id?: string; ordinal?: number }
export interface HistoryEntry {
  entry_id: string; request_id?: string; kind: string; title: HistoryValue; summary: HistoryValue;
  state: string; state_evidence?: string[]; requested_at: HistoryValue;
  locator: { invocation_id: string; generation_ordinal?: number; collection_invocation_ids?: string[]; collection_ordinals?: number[]; accepted_ordinal?: number; job_attempt?: number; task_attempt?: number; path?: string; story_node_id?: string };
  form_fields: Record<string, unknown>; accepted_response: HistoryValue; response_source: string;
  actor: { receipt?: unknown; legacy_user_id: HistoryValue; provenance: string };
  diagnostics?: HistoryDiagnostic[];
}
export interface HistoryPage {
  merges?: HistoryMerge[]; events?: HistoryEvent[]; publication_coverage?: string;
  snapshot_revision: number; tenant_id: string; job_id: string; entries: HistoryEntry[];
  next_page_token?: string; snapshot_token: string; observed_at: string; completeness: string; diagnostics?: HistoryDiagnostic[];
}
export interface HistoryMember {
  artifact_id: string; ref: unknown; original_ref?: unknown; resolution_source?: string;
  associations: { kind: string; source: string }[]; availability: string; reason?: string;
}
export interface HistoryDetail extends HistoryEntry {
  snapshot_revision: number; raw_form?: unknown; raw_accepted_output?: unknown;
  request_manifest: HistoryMember[]; response_manifest: HistoryMember[];
  snapshot_token: string; observed_at: string; completeness: string;
  actionability: { actionable: boolean; observation: string; observed_at: string; reason?: string };
}
function base(tenant: string, job: string) {
  return `${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/conversation`;
}
async function read<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal });
  const text = await response.text();
  if (!response.ok) {
    let error = text, code: string | undefined;
    try { const body = JSON.parse(text); error = body.error || text; code = body.code; } catch { /* raw transport error */ }
    throw new APIError(error, response.status, code);
  }
  return parseJobJSON(text) as T;
}
export async function getConversation(tenant: string, job: string, options: { pageToken?: string; pageSize?: number; signal?: AbortSignal } = {}): Promise<HistoryPage> {
  const query = new URLSearchParams({ page_size: String(options.pageSize ?? 50) });
  if (options.pageToken) query.set('page_token', options.pageToken);
  const page = await read<HistoryPage>(`${base(tenant, job)}?${query}`, options.signal);
  if (page.tenant_id !== tenant || page.job_id !== job || !Array.isArray(page.entries)) throw new Error('History response does not match the selected job');
  return page;
}
export function getConversationEntry(tenant: string, job: string, entry: string, options: { snapshotToken?: string; signal?: AbortSignal } = {}): Promise<HistoryDetail> {
  const query = new URLSearchParams({ entry_id: entry });
  if (options.snapshotToken) query.set('snapshot_token', options.snapshotToken);
  return read<HistoryDetail>(`${base(tenant, job)}/entry?${query}`, options.signal).then(detail => {
    if (detail.entry_id !== entry) throw new Error('History response does not match the selected occurrence');
    return detail;
  });
}
export function conversationArtifactURL(tenant: string, job: string, entry: string, member: string, snapshotToken?: string) {
  const query = new URLSearchParams({ entry_id: entry, artifact_id: member });
  if (snapshotToken) query.set('snapshot_token', snapshotToken);
  return `${base(tenant, job)}/artifact?${query}`;
}

// Legacy links identify a frozen request rather than a durable occurrence.
// Refuse ambiguous reused requests instead of substituting any live form.
export async function getConversationRequest(tenant: string, job: string, request: string, signal?: AbortSignal): Promise<HistoryDetail> {
  let page = await getConversation(tenant, job, { signal });
  const snapshot = page.snapshot_token;
  let matches = page.entries.filter(entry => entry.request_id === request || entry.entry_id === request);
  while (page.next_page_token) {
    page = await getConversation(tenant, job, { pageToken: page.next_page_token, signal });
    matches = [...matches, ...page.entries.filter(entry => entry.request_id === request || entry.entry_id === request)];
  }
  if (matches.length !== 1) throw new Error(matches.length ? 'Request identity is ambiguous; open its occurrence link.' : 'The selected request is not retained.');
  return getConversationEntry(tenant, job, matches[0].entry_id, { snapshotToken: snapshot, signal });
}

export interface HistoryMerge {
  entry_id: string; tenant_id: string; job_id: string; job_attempt: number; task_attempt: number;
  invocation_id: string; result_ordinal: number; operation: string; completed_at?: string;
  evidence_availability: string; candidate_sha?: string; candidate_subject?: string | null;
  publication_branch?: string; publication_disposition?: string;
}
export interface HistoryEvent { ordinal: number; invocation_id: string; kind: string; entry_id: string }
export interface FinalDocument {
  alias: string; resolution: string; diagnostic?: string; ref: unknown; raw: unknown;
  producer?: { jobId: string; taskOrdinal: number; name: string; sizeBytes?: number };
}
export interface FinalDocuments {
  job_id: string; attempt_ordinal?: number; manifest_id: string; state: string;
  documents: FinalDocument[]; diagnostic?: string;
}
export async function getFinalDocuments(tenant: string, job: string, signal?: AbortSignal): Promise<FinalDocuments | {status: 'pending'}> {
  const result = await read<FinalDocuments | {status:'pending'}>(`${base(tenant,job).replace(/\/conversation$/, '')}/final-documents`, signal);
  if ('status' in result && result.status === 'pending') return result;
  const manifest = result as FinalDocuments;
  if (manifest.job_id !== job || !Array.isArray(manifest.documents) || !manifest.manifest_id) throw new Error('Returned documents do not match the selected job');
  return manifest;
}
export function finalDocumentURL(tenant: string, job: string, manifest: string, alias: string) {
  const query = new URLSearchParams({manifest_id:manifest, alias});
  return `${base(tenant,job).replace(/\/conversation$/, '')}/final-documents/document?${query}`;
}
