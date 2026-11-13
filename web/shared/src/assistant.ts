import { API_BASE } from './apiBase';

// Browser DTOs deliberately omit recipe/model configuration and executable actions.
export interface AssistantSource { kind: 'global' | 'story' | 'review'; job_id?: string; request_id?: string }
export interface AssistantStart { readonly generation?: string; readonly request_id: string; readonly submitted_at: string; readonly prompt: string; readonly source: Readonly<AssistantSource> }
export interface AssistantSubmission {
  readonly request_id: string; readonly submission_id: string; readonly submitted_at: string;
  readonly command: 'send' | 'finish'; readonly prompt?: string;
}
export interface AssistantLifecycleRequest {
  readonly generation?: string;
  readonly request_id: string; readonly submitted_at: string; readonly job_id: string;
}
export interface AssistantArtifact { origin?: AssistantOrigin; id: string; name: string; mime: string; size: number; available: boolean }
export interface AssistantCoverage {
  id: string; name: string; outcome: string; detail?: string; producer_job_id?: string; producer_task?: string; artifact_id?: string;
}
export interface AssistantOrigin { job_id: string; task: number; attempt?: number; invocation?: string; path?: string[] }
export interface AssistantEntry { receipt?: AssistantReceipt; origin?: AssistantOrigin; citations?: string[]; key: string; role: string; status: string; text?: string; artifacts?: AssistantArtifact[] }
export interface AssistantReplay { entries: AssistantEntry[]; cursor?: string; has_more: boolean; complete: boolean; watermark: string }
export interface AssistantSession {
  id: string; job_id: string; goal_id?: string; lifecycle?: 'goal' | 'legacy'; status: string; source: AssistantSource; captured_at?: string;
  coverage?: AssistantCoverage[]; current_request_id?: string; replay: AssistantReplay;
}
export interface AssistantSessions { sessions: AssistantSession[]; cursor?: string }
export interface AssistantReceipt { request_id: string; submission_id: string; outcome: string; submitted_at?: string }
export interface AssistantCapabilities { available: boolean; generation?: string }
export interface AssistantPage { cursor?: string; limit?: number }
export class AssistantAPIError extends Error {
  constructor(public readonly status: number, public readonly code: string) {
    super(code === 'unresolved' ? 'This request may have succeeded. Automatic recovery is unavailable after the server restarted. Use conversation history to inspect recorded conversations.' : code === 'forbidden' ? 'Assistant access is unavailable' : code === 'conflict'
      ? 'Conversation changed. Refresh before retrying.' : code === 'invalid'
        ? 'This assistant request is invalid' : 'Assistant is unavailable. Refresh to recover accepted messages.');
    this.name = 'AssistantAPIError';
  }
}

function base(tenant: string): string { return `${API_BASE}/projects/${encodeURIComponent(tenant)}/assistant`; }
function sessionPath(tenant: string, id: string): string { return `${base(tenant)}/sessions/${encodeURIComponent(id)}`; }
function query(page?: AssistantPage): string {
  const q = new URLSearchParams();
  if (page?.cursor) q.set('cursor', page.cursor);
  if (page?.limit !== undefined) q.set('limit', String(page.limit));
  return q.size ? `?${q}` : '';
}
async function request<T>(url: string, method: 'GET' | 'POST', body?: unknown, signal?: AbortSignal): Promise<T> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (signal?.aborted) controller.abort();
  else signal?.addEventListener('abort', abort, { once: true });
  const timer = setTimeout(abort, 20_000);
  try {
    const response = await fetch(url, {
      method, signal: controller.signal, credentials: 'same-origin', cache: 'no-store',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) {
      const error = await response.json().catch(() => ({})) as { code?: unknown };
      throw new AssistantAPIError(response.status, typeof error.code === 'string' ? error.code : 'unavailable');
    }
    return await response.json() as T;
  } finally { clearTimeout(timer); signal?.removeEventListener('abort', abort); }
}
export function assistantCapabilities(tenant: string, signal?: AbortSignal): Promise<AssistantCapabilities> {
  return request(`${base(tenant)}/capabilities`, 'GET', undefined, signal);
}
export function listAssistantSessions(tenant: string, page?: AssistantPage, signal?: AbortSignal): Promise<AssistantSessions> {
  return request(`${base(tenant)}/sessions${query(page)}`, 'GET', undefined, signal);
}
export function getAssistantSession(tenant: string, id: string, signal?: AbortSignal): Promise<AssistantSession> {
  return request(sessionPath(tenant, id), 'GET', undefined, signal);
}
export function replayAssistantSession(tenant: string, id: string, page?: AssistantPage, signal?: AbortSignal): Promise<AssistantReplay> {
  return request(`${sessionPath(tenant, id)}/replay${query(page)}`, 'GET', undefined, signal);
}
export function startAssistantSession(tenant: string, input: AssistantStart, signal?: AbortSignal): Promise<AssistantSession> {
  // Project the allowed envelope, even for untyped JS callers.
  return request(`${base(tenant)}/sessions`, 'POST', {
    generation: input.generation, request_id: input.request_id, submitted_at: input.submitted_at, prompt: input.prompt,
    source: { kind: input.source.kind, job_id: input.source.job_id, request_id: input.source.request_id },
  }, signal);
}
export function respondAssistantSession(tenant: string, id: string, input: AssistantSubmission, signal?: AbortSignal): Promise<AssistantReceipt> {
  if (input.command !== 'send' && input.command !== 'finish') return Promise.reject(new AssistantAPIError(400, 'invalid'));
  return request(`${sessionPath(tenant, id)}/respond`, 'POST', {
    request_id: input.request_id, submission_id: input.submission_id, submitted_at: input.submitted_at,
    command: input.command, prompt: input.command === 'send' ? input.prompt : undefined,
  }, signal);
}
function lifecycleBody(input: AssistantLifecycleRequest) {
  return { generation: input.generation, request_id: input.request_id, submitted_at: input.submitted_at, job_id: input.job_id };
}
export function closeAssistantSession(tenant: string, id: string, input: AssistantLifecycleRequest, signal?: AbortSignal): Promise<AssistantSession> {
  return request(`${sessionPath(tenant, id)}/close`, 'POST', lifecycleBody(input), signal);
}
export function reopenAssistantSession(tenant: string, id: string, input?: AssistantLifecycleRequest, signal?: AbortSignal): Promise<AssistantSession> {
  return request(`${sessionPath(tenant, id)}/reopen`, 'POST', input ? lifecycleBody(input) : {}, signal);
}
export function freezeAssistantLifecycle(jobID: string, generation?: string): AssistantLifecycleRequest {
  return Object.freeze({ generation, request_id: crypto.randomUUID(), submitted_at: new Date().toISOString(), job_id: jobID });
}
export function assistantArtifactURL(tenant: string, id: string, artifact: string): string {
  // No source URL or producer key is accepted by this browser API.
  return `${sessionPath(tenant, id)}/artifacts/${encodeURIComponent(artifact)}`;
}
export function freezeAssistantStart(prompt: string, source: AssistantSource, generation?: string): AssistantStart {
  return Object.freeze({ generation, request_id: crypto.randomUUID(), submitted_at: new Date().toISOString(), prompt,
    source: Object.freeze({ kind: source.kind, job_id: source.job_id, request_id: source.request_id }) });
}
export function freezeAssistantSubmission(requestID: string, command: 'send' | 'finish', prompt?: string): AssistantSubmission {
  return Object.freeze({ request_id: requestID, submission_id: crypto.randomUUID(), submitted_at: new Date().toISOString(), command,
    prompt: command === 'send' ? prompt : undefined });
}
// The released facade pages backward from the latest chronological slice.
// An older continuation therefore precedes the already displayed entries.
export function mergeAssistantReplay(previous: AssistantEntry[], page: AssistantReplay): AssistantEntry[] {
  const incoming = new Map((page.entries ?? []).map(entry => [entry.key, entry]));
  return [...incoming.values(), ...previous.filter(entry => !incoming.has(entry.key))];
}
