import type {
  CortexCell, ListCellsResponse, ListCellsOptions, CreateCellRequest, SubmittedJob,
  ListRecipeJobsResponse,
  Project,
  RecipeJob,
  RecipeJobStatus,
  RecipeJobFilterStatus,
  SubmitRecipeJobRequest,
  RestartContext,
} from './types';

import { API_BASE } from './apiBase';
import { parseJobJSON } from './jobJSON';
import { validateJobCell, validateCell } from './cellProtocol';

export class APIError extends Error {
  constructor(message: string, public status: number, public code?: string) { super(message); }
}
async function handleResponse(response: Response, operation: string) {
  if (!response.ok) {
    const text = await response.text().catch(() => 'Unknown error');
    let message = text, code: string | undefined;
    try { const body = JSON.parse(text); message = body.error || body.message || text; code = body.code; } catch { /* plain HTTP error */ }
    throw new APIError(`${operation} failed: ${response.status} ${response.statusText} - ${message}`, response.status, code);
  }
  return response;
}

export async function listProjects(tenantId?: string): Promise<Project[]> {
  const query = tenantId ? `?tenantId=${encodeURIComponent(tenantId)}` : '';
  const response = await fetch(`${API_BASE}/projects${query}`);
  await handleResponse(response, 'List projects');
  return response.json();
}

export async function listCells(projectId: string, options: ListCellsOptions = {}): Promise<ListCellsResponse> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(options)) if (value !== undefined && value !== '') params.set(key, String(value));
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells?${params}`);
  await handleResponse(response, 'List cells');
  const page: ListCellsResponse = await response.json();
  if (!Array.isArray(page.cells)) throw new Error('Invalid cell registry response');
  page.cells.forEach(cell => validateCell(cell, projectId));
  return page;
}

export async function getCell(projectId: string, cellId: string): Promise<CortexCell> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells/${encodeURIComponent(cellId)}`);
  await handleResponse(response, 'Get cell');
  const cell: CortexCell = await response.json();
  validateCell(cell, projectId);
  if (cell.id !== cellId) throw new Error('Cell response does not match the selected cell');
  return cell;
}

export async function createCell(projectId: string, input: CreateCellRequest): Promise<CortexCell> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/cells`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  });
  await handleResponse(response, 'Create cell');
  const cell: CortexCell = await response.json();
  validateCell(cell, projectId);
  return cell;
}

export async function listJobs(
  projectId: string,
  options: {
    status?: RecipeJobStatus[];
    displayStatus?: RecipeJobFilterStatus[];
    cellNames?: string[];
    pageSize?: number;
    pageToken?: string;
    goal?: string;
    includeHidden?: boolean;
    rootOnly?: boolean;
  } = {},
  signal?: AbortSignal,
): Promise<ListRecipeJobsResponse> {
  const params = new URLSearchParams();
  // An explicitly cleared filter means all statuses; omitted options retain API defaults.
  if (options.status?.length === 0 || options.displayStatus?.length === 0) params.set('status', 'all');
  for (const status of options.status || []) {
    params.append('status', status);
  }
  for (const status of options.displayStatus || []) params.append('displayStatus', status);
  for (const cell of options.cellNames || []) params.append('cellNames', cell);
  if (options.goal) params.set('goal', options.goal);
  if (options.rootOnly !== undefined) params.set('rootOnly', String(options.rootOnly));
  if (options.includeHidden !== undefined) params.set('includeHidden', String(options.includeHidden));
  if (options.pageSize) params.set('pageSize', String(options.pageSize));
  if (options.pageToken) params.set('pageToken', options.pageToken);
  const query = params.toString();
  const url = `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs${query ? `?${query}` : ''}`;
  const response = signal ? await fetch(url, { signal }) : await fetch(url);
  await handleResponse(response, 'List jobs');
  const page = parseJobJSON(await response.text()) as ListRecipeJobsResponse;
  if (!Array.isArray(page.jobs)) throw new Error('Invalid job list response');
  page.jobs.forEach(validateJobCell);
  return page;
}

export async function listJobChildren(
  projectId: string, jobId: string, options: { pageSize?: number; pageToken?: string } = {},
): Promise<ListRecipeJobsResponse> {
  const params = new URLSearchParams();
  if (options.pageSize !== undefined) params.set('pageSize', String(options.pageSize));
  if (options.pageToken) params.set('pageToken', options.pageToken);
  const query = params.toString();
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/children${query ? `?${query}` : ''}`);
  await handleResponse(response, 'List child jobs');
  const page = parseJobJSON(await response.text()) as ListRecipeJobsResponse;
  if (!Array.isArray(page.jobs)) throw new Error('Invalid child job list response');
  page.jobs.forEach(validateJobCell);
  return page;
}

export async function getJob(projectId: string, jobId: string, includeLaunch = false): Promise<RecipeJob> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}${includeLaunch ? '?includeLaunch=true' : ''}`,
  );
  await handleResponse(response, 'Get job');
  const job = parseJobJSON(await response.text()) as RecipeJob;
  validateJobCell(job);
  return job;
}

export async function getRestartContext(projectId: string, jobId: string, boundary: number | string, signal?: AbortSignal): Promise<RestartContext> {
  const selector = typeof boundary === 'string' ? `scope_id=${encodeURIComponent(boundary)}` : `chapter_ordinal=${boundary}`;
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/restart-context?${selector}`,
    { signal },
  );
  await handleResponse(response, 'Inspect restart context');
  const context = parseJobJSON(await response.text()) as RestartContext;
  if (context.project_id !== projectId || context.job_id !== jobId || (typeof boundary === 'string' ? context.scope_id !== boundary : context.chapter_ordinal !== boundary)) {
    throw new Error('Restart context does not match the selected job and restart point.');
  }
  return context;
}

export const DEFAULT_CANCELLATION_REASON = 'Cancelled from Cortex';
export const CANCELLATION_BODY_BYTES = 64 * 1024;
export const CANCELLATION_SIZE_ERROR = 'This request exceeds 64 KiB, including JSON encoding. Shorten the message. Your complete draft is still editable.';
export function serializeCancellationReason(reason: string): string { return JSON.stringify({ reason }); }
export function cancellationSizeError(reason: string): string | undefined {
  return new TextEncoder().encode(serializeCancellationReason(reason)).byteLength > CANCELLATION_BODY_BYTES ? CANCELLATION_SIZE_ERROR : undefined;
}
function reasonBody(reason?: string): RequestInit {
  if (reason === undefined) return {};
  const body = serializeCancellationReason(reason);
  if (new TextEncoder().encode(body).byteLength > CANCELLATION_BODY_BYTES) throw new APIError(CANCELLATION_SIZE_ERROR, 413);
  return { headers: { 'Content-Type': 'application/json' }, body };
}
async function handleCancellationResponse(response: Response, action: string): Promise<void> {
  if (response.status === 413) throw new APIError(CANCELLATION_SIZE_ERROR, 413);
  await handleResponse(response, action);
}

export async function cancelJob(projectId: string, jobId: string, reason?: string): Promise<void> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/cancel`,
    { method: 'POST', ...reasonBody(reason) },
  );
  await handleCancellationResponse(response, 'Cancel job');
}

// The existing deletion API hides jobs and cancels unfinished work; it retains history.
export async function archiveJob(projectId: string, jobId: string, reason?: string): Promise<void> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}`,
    { method: 'DELETE', ...reasonBody(reason) },
  );
  await handleCancellationResponse(response, 'Archive job');
}

export async function submitJob(projectId: string, input: SubmitRecipeJobRequest, files?: readonly File[]): Promise<SubmittedJob> {
  let body: BodyInit = JSON.stringify(input);
  if (files?.length) {
    const form = new FormData();
    form.append('submission', JSON.stringify(input));
    // MIME filename quoting is lossy in browsers; JSON preserves original names.
    form.append('filenames', JSON.stringify(files.map(file => file.name)));
    files.forEach((file, index) => form.append('files', file, String(index)));
    body = form;
  }
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs`, {
    method: 'POST',
    ...(files?.length ? {} : { headers: { 'Content-Type': 'application/json' } }),
    body,
  });
  await handleResponse(response, 'Submit job');
  const job = parseJobJSON(await response.text()) as SubmittedJob;
  validateJobCell(job);
  if (!job.goal_id || !job.job_id || job.tenant_id !== projectId) throw new Error('Invalid submission identity');
  return job;
}

/** Keep export bytes opaque: JSON parsing here can round persisted 64-bit integers. */
export async function getJobChaptersDownload(projectId: string, jobId: string, signal?: AbortSignal): Promise<Blob> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/chapters/download`,
    { signal },
  );
  await handleResponse(response, 'Download chapters');
  return response.blob();
}

export type InputArtifact = {
  inputOrdinal?: number;
  id?: string;
  name: string;
  sizeBytes: number;
  binding?: string;
  sources?: string[];
} & ({ kind?: 'stored'; jobId: string; taskOrdinal: number; storedKey?: { jobId: string; taskOrdinal: number; name: string }; contentBinding?: never; reason?: never }
 | { kind: 'stored'; contentBinding: string; storedKey?: { jobId: string; taskOrdinal: number; name: string }; jobId?: never; taskOrdinal?: never; reason?: never }
 | { kind: 'value'; contentBinding: string; jobId?: never; taskOrdinal?: never; reason?: never }
 | { kind: 'external' | 'attached' | 'invalid'; reason: string; jobId?: never; taskOrdinal?: never; contentBinding?: never });

export function storedDocumentURL(projectId: string, key: { jobId: string; taskOrdinal: number | bigint; name: string }): string {
  const query = new URLSearchParams({ ordinal: String(key.taskOrdinal), name: key.name });
  return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(key.jobId)}/stored-document?${query}`;
}

export async function getJobInputArtifacts(projectId: string, jobId: string, signal?: AbortSignal, ordinal?: number): Promise<{ artifacts: InputArtifact[] }> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/input-artifacts${ordinal === undefined ? '' : `?ordinal=${ordinal}`}`, { signal });
  await handleResponse(response, 'Load input artifacts');
  return response.json();
}

export async function getTreeNode(tenant: string, job: string, signal?: AbortSignal): Promise<import('./types').TreeNodeResponse> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/tree-node`, { signal });
  await handleResponse(response, 'Load tree node');
  return response.json();
}
export async function getTreeChildren(tenant: string, job: string, token = '', signal?: AbortSignal): Promise<import('./types').TreeChildrenResponse> {
  const query = new URLSearchParams({ pageSize: '100', pageToken: token });
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/tree-children?${query}`, { signal });
  await handleResponse(response, 'Load tree children');
  return response.json();
}

/** One explicit page; never prefetch, traverse, log, or decode continuation tokens. */
export async function getJobLeaseEvents(
  projectId: string, jobId: string,
  options: { pageSize?: number; pageToken?: string } = {}, signal?: AbortSignal,
): Promise<import('./types').JobLeaseEventPage> {
  const params = new URLSearchParams({ pageSize: String(options.pageSize ?? 100) });
  if (options.pageToken) params.set('pageToken', options.pageToken);
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/events?${params}`, { signal });
  await handleResponse(response, 'Load lease history');
  const page = await response.json();
  // A successful legacy/noncanonical response is incompatible, not empty history.
  if (!page || !Array.isArray(page.events) || (page.nextPageToken !== undefined && typeof page.nextPageToken !== 'string')) {
    throw new APIError('Incompatible lease history response', 502, 'incompatible_response');
  }
  for (const event of page.events) {
    const strings = ['eventId', 'occurredAt', 'cell', 'cellId', 'cellRepo', 'goalId', 'jobType', 'leaseId', 'workerId', 'leaseExpiresAt'];
    if (!event || strings.some(key => typeof event[key] !== 'string') ||
        !['lease_acquired', 'lease_renewed'].includes(event.eventType) ||
        typeof event.jobKey?.tenantId !== 'string' || typeof event.jobKey?.jobId !== 'string' ||
        event.jobKey.tenantId !== projectId || event.jobKey.jobId !== jobId ||
        typeof event.route?.jobType !== 'string' ||
        ['previousLeaseExpiresAt', 'acquisitionSource', 'previousLeaseId'].some(key => event[key] !== undefined && typeof event[key] !== 'string') ||
        (event.route.taskType !== undefined && typeof event.route.taskType !== 'string')) {
      throw new APIError('Incompatible lease history event', 502, 'incompatible_response');
    }
  }
  return page;
}

/** A durable receiving attempt; no latest-attempt defaults or raw values. */
export interface InboxValueSelector {
  jobAttempt: number; invocationId: string; taskOrdinal: number; taskAttempt: number; binding: string;
}
export function inboxValueUrl(projectId: string, jobId: string, selector: InboxValueSelector): string {
  const query = new URLSearchParams({job_attempt: String(selector.jobAttempt), invocation_id: selector.invocationId,
    task_ordinal: String(selector.taskOrdinal), task_attempt: String(selector.taskAttempt), binding: selector.binding});
  return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/inbox/value?${query}`;
}

/** One bounded page; targets are references and are never hydrated here. */
export async function getTreeWaits(tenant: string, job: string, token = '', signal?: AbortSignal): Promise<import('./types').TreeWaitsResponse> {
  const query = new URLSearchParams({ pageToken: token });
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/tree-waits?${query}`, { signal });
  await handleResponse(response, 'Load pending dependencies');
  const page = await response.json();
  const snapshot = page?.consumer_snapshot;
  if (!page || page.consumer?.tenant_id !== tenant || page.consumer?.job_id !== job || !Array.isArray(page.waits)
    || typeof page.observed_at !== 'string' || !Number.isFinite(Date.parse(page.observed_at))
    || !Array.isArray(page.issues) || page.issues.some((v: unknown) => typeof v !== 'string')
    || (page.next_page_token !== undefined && typeof page.next_page_token !== 'string')
    || (!token && (!snapshot || !['available','unavailable'].includes(snapshot.state)))
    || (snapshot?.state === 'available' && (typeof snapshot.status !== 'string' || !snapshot.status.trim()))
    || (snapshot && (typeof snapshot.observed_at !== 'string' || !Number.isFinite(Date.parse(snapshot.observed_at))
      || (snapshot.legacy_wait_for !== undefined && (!Array.isArray(snapshot.legacy_wait_for) || snapshot.legacy_wait_for.some((v: unknown) => typeof v !== 'string')))))) {
    throw new APIError('Incompatible pending dependency response', 502, 'incompatible_response');
  }
  return page;
}

export async function clearCrashConcern(projectId: string, jobId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/clear-crash-concern`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}',
  });
  await handleResponse(response, 'Clear crash concern');
  if (response.status !== 204) throw new APIError('Clear crash concern returned an unexpected response; its outcome is unknown. Refresh status before retrying.', response.status);
}
