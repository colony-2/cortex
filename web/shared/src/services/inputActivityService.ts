import { validateJobCell } from '../cellProtocol';
import type { RecipeJobSummary } from '../types';
import { APIError } from '../api';
import { API_BASE } from '../apiBase';
import { parseJobJSON, stringifyJobJSON } from '../jobJSON';

export function createSubmissionId(): string {
  if (typeof globalThis.crypto.randomUUID === 'function') return globalThis.crypto.randomUUID();
  // randomUUID requires a secure context; getRandomValues also works on HTTP.
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

// Simple EventEmitter implementation for browser
class EventEmitter {
  private events: Map<string, Set<Function>> = new Map();

  on(event: string, listener: Function): void {
    if (!this.events.has(event)) {
      this.events.set(event, new Set());
    }
    this.events.get(event)!.add(listener);
  }

  off(event: string, listener: Function): void {
    this.events.get(event)?.delete(listener);
  }

  emit(event: string, ...args: any[]): void {
    this.events.get(event)?.forEach(listener => {
      listener(...args);
    });
  }
}

// Types matching OpenAPI spec
export type FieldType =
  | 'boolean'
  | 'short_answer'
  | 'paragraph_text'
  | 'multiple_choice'
  | 'checkboxes'
  | 'dropdown'
  | 'linear_scale'
  | 'multiple_choice_grid'
  | 'checkbox_grid'
  | 'date'
  | 'time'
  | 'file_upload';

export interface Option {
  value: string;
  label?: string;
}

export interface LinearScale {
  min: number;
  max: number;
  min_label?: string;
  max_label?: string;
}

export interface FieldValidation {
  min_length?: number;
  max_length?: number;
  pattern?: string;
  min?: number;
  max?: number;
}

export interface FormField {
  id: string;
  type: FieldType;
  question: string;
  required?: boolean;
  placeholder?: string;
  options?: Option[];
  scale?: LinearScale;
  validation?: FieldValidation;
}

export interface Artifact {
  path: string;
}

export interface GlobPattern {
  pattern: string;
}

export interface FormContext {
  artifacts?: Artifact[];
  artifacts_from_output?: string;
  artifacts_glob?: GlobPattern[];
}

export interface ReviewDocumentMetadata {
  title?: string;
  description?: string;
  role?: '' | 'neutral' | 'primary' | 'supporting' | 'diagnostic';
  order?: number;
  source_document_ids?: string[];
}

export interface ReviewDocument {
  kind: 'stored';
  name?: string;
  stored: { key: { jobId: string; taskOrdinal: number; name: string; sizeBytes: number } };
}

export interface InputFormConfig {
  kind?: string;
  presentation?: { type?: string; [key: string]: unknown };
  request?: unknown;
  request_schema?: Record<string, unknown>;
  response_schema?: Record<string, unknown>;
  request_id?: string;
  documents?: Record<string, ReviewDocument>;
  document_metadata?: Record<string, ReviewDocumentMetadata>;
  overview_document_id?: string;
  // Single question format
  question?: string;
  type?: FieldType;
  options?: Option[];
  scale?: LinearScale;

  // Multi-field format
  title?: string;
  fields?: FormField[];
  context?: FormContext;
  timeout?: number;
  default_on_timeout?: any;
}

// Component-friendly form structure (for InputFormRenderer)
export interface InputForm {
  kind?: string;
  id: string;
  title: string;
  description?: string;
  fields: InputField[];
  // Single-question forms return this field as c2j's top-level response.
  responseField?: string;
}

export interface InputField {
  id: string;
  label: string;
  type: FieldType;
  required: boolean;
  options?: string[];
  optionLabels?: Record<string, string>;
  min?: number;
  max?: number;
  placeholder?: string;
  validation?: {
    pattern?: string;
    message?: string;
  };
}

export interface FormResponse {
  attachments?: Record<string, File>;
  fields: Record<string, any>;
  activity_id?: string;
  user_id?: string;
  response?: any;
  submitted_at?: string;
  time_to_complete?: number;
  hash?: string;
}

// API response types
export interface PendingInput extends Partial<Omit<RecipeJobSummary, 'cell_id' | 'cell_name' | 'repo'>>, Pick<RecipeJobSummary, 'cell_id' | 'cell_name' | 'repo'> {
  id: string;
  requested_at?: string;
  request_id?: string;
  task_ordinal?: number;
}

export interface UserInputDetails {
  jobId: string;
  status: string;
  startTime: string;
  form: InputFormConfig;
  hash?: string;
  task_ordinal?: number;
  taskOrdinal?: number;
}

// SSE event types based on OpenAPI InputSSEEvent
export type InputSSEEventType =
  | 'connected'
  | 'inputs_changed'
  | 'input_pending'
  | 'input_cancelled'
  | 'input_completed'
  | 'heartbeat'
  | 'error';

export interface InputSSEEvent {
  type: InputSSEEventType;
  data: any;
}

export type DiscoveryStatus = 'loading' | 'current' | 'unavailable';
export interface InputDiscovery { status: DiscoveryStatus; inputs: PendingInput[]; error?: string }
const initialDiscovery: InputDiscovery = { status: 'loading', inputs: [] };
interface DetailAttempt {
  project: string; key: string; promise: Promise<UserInputDetails | null>;
  details?: UserInputDetails | null; error?: Error;
}

function integer(value: unknown): boolean {
  const text = stringifyJobJSON(value);
  return !!text && /^(0|[1-9][0-9]*)$/.test(text);
}
function requestVersion(input: PendingInput): string {
  return stringifyJobJSON([input.id, input.request_id ?? null, input.task_ordinal ?? null, input.client_payload_revision])!;
}
function validateSnapshot(value: unknown): PendingInput[] {
  if (!Array.isArray(value)) throw new Error('Invalid pending request snapshot');
  const ids = new Set<string>();
  for (const row of value) {
    validateJobCell(row);
    if (typeof row.id !== 'string' || !row.id || ids.has(row.id) || !integer(row.client_payload_revision)
      || (row.request_id !== undefined && (typeof row.request_id !== 'string' || !row.request_id))
      || (row.task_ordinal !== undefined && !integer(row.task_ordinal))) throw new Error('Invalid pending request identity or revision');
    ids.add(row.id);
  }
  return value;
}

class InputActivityService extends EventEmitter {
  private eventSource: EventSource | null = null;
  private currentProjectId: string | null = null;
  private epoch = 0;
  private discovery = new Map<string, InputDiscovery>();
  private pendingInputsCache = new Map<string, PendingInput[]>();
  private formDetailsCache = new Map<string, DetailAttempt>();
  private memberships = new Map<string, number>();
  private pendingReads = new Map<string, { discovery: InputDiscovery; promise: Promise<PendingInput[]> }>();

  getDiscovery(project: string): InputDiscovery { return this.discovery.get(project) ?? initialDiscovery; }
  private unavailable(project: string, error: unknown): void {
    for (const [key, attempt] of this.formDetailsCache) {
      if (attempt.project === project && attempt.details === undefined && !attempt.error) this.formDetailsCache.delete(key);
    }
    this.discovery.set(project, { status: 'unavailable', inputs: this.getDiscovery(project).inputs,
      error: error instanceof Error ? error.message : 'Pending request availability is unavailable.' });
    this.emit('discovery_changed');
    this.emit('inputs_error', { projectId: project, error });
  }
  private publish(project: string, value: unknown): PendingInput[] {
    const inputs = validateSnapshot(value);
    const next = new Map(inputs.map(row => [row.id, row]));
    for (const row of this.pendingInputsCache.get(project) ?? []) {
      const successor = next.get(row.id);
      if (!successor || requestVersion(successor) !== requestVersion(row)) {
        const oldKey = JSON.stringify([project, requestVersion(row), this.memberships.get(JSON.stringify([project, row.id])) ?? 0]);
        this.formDetailsCache.delete(oldKey);
        const key = JSON.stringify([project, row.id]);
        this.memberships.set(key, (this.memberships.get(key) ?? 0) + 1);
      }
    }
    this.pendingInputsCache.set(project, inputs);
    this.discovery.set(project, {status: 'current', inputs});
    this.emit('discovery_changed');
    this.emit('inputs_changed', {projectId: project, inputs});
    return inputs;
  }
  connect(project: string): void {
    if (this.eventSource && this.eventSource.readyState !== 2 && this.currentProjectId === project) return;
    const retainedInputs = this.getDiscovery(project).inputs;
    this.disconnect();
    this.currentProjectId = project;
    this.discovery.set(project, {status: 'loading', inputs: retainedInputs});
    this.emit('discovery_changed');
    if (typeof EventSource === 'undefined') { this.unavailable(project, new Error('Live updates unavailable')); return; }
    let source: EventSource;
    try { source = new EventSource(`${API_BASE}/projects/${encodeURIComponent(project)}/user-inputs/stream`); }
    catch (error) { this.unavailable(project, error); return; }
    this.eventSource = source;
    const epoch = this.epoch;
    const active = () => this.eventSource === source && this.epoch === epoch && this.currentProjectId === project;
    source.onopen = () => {
      if (!active()) return;
      this.emit('connected');
      if (this.getDiscovery(project).status !== 'current') void this.getPendingInputs(project, true).catch(() => {});
    };
    source.addEventListener('inputs_changed', event => {
      if (!active()) return;
      try { this.publish(project, (parseJobJSON((event as MessageEvent).data) as {inputs: unknown}).inputs); }
      catch (error) { this.unavailable(project, error); }
    });
    source.addEventListener('error', event => {
      if (!active()) return;
      this.unavailable(project, new Error((event as MessageEvent).data ? 'Pending request stream failed.' : 'Pending request connection lost. Reconnecting…'));
    });
    source.onerror = () => {
      if (!active()) return;
      this.unavailable(project, new Error('Pending request connection lost. Reconnecting…'));
      this.emit('disconnected');
    };
    for (const name of ['input_pending', 'input_cancelled', 'input_completed']) {
      source.addEventListener(name, event => {
        if (!active()) return;
        try { this.emit(name, JSON.parse((event as MessageEvent).data)); void this.getPendingInputs(project, true).catch(() => {}); }
        catch (error) { this.unavailable(project, error); }
      });
    }
  }
  disconnect(): void {
    const project = this.currentProjectId;
    this.eventSource?.close(); this.eventSource = null;
    this.currentProjectId = null; this.epoch++;
    this.pendingReads.clear();
    if (project) this.unavailable(project, new Error('Pending request connection closed.'));
    this.emit('disconnected');
  }
  async getPendingInputs(project: string, force = false): Promise<PendingInput[]> {
    if (!force && this.getDiscovery(project).status === 'current') return this.getDiscovery(project).inputs;
    if (force && this.currentProjectId === project && this.eventSource?.readyState === 2) this.connect(project);
    const epoch = this.epoch, discovery = this.getDiscovery(project);
    const pending = this.pendingReads.get(project);
    if (pending?.discovery === discovery) return pending.promise;
    const accepted = () => {
      const state = this.getDiscovery(project);
      if (this.epoch === epoch && state.status === 'current') return state.inputs;
      throw new Error(state.error ?? 'Pending request availability is unavailable.');
    };
    let promise!: Promise<PendingInput[]>;
    const current = () => this.pendingReads.get(project)?.promise === promise && this.getDiscovery(project) === discovery;
    promise = (async () => {
      try {
        const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(project)}/user-inputs/pending`, {cache: 'no-store'});
        if (!response.ok) throw new APIError(`Failed to fetch pending requests: ${response.statusText}`, response.status);
        const value = parseJobJSON(await response.text());
        if (!current()) return accepted();
        return this.publish(project, value);
      } catch (error) {
        if (!current()) return accepted();
        this.unavailable(project, error); throw error;
      } finally {
        if (this.pendingReads.get(project)?.promise === promise) this.pendingReads.delete(project);
      }
    })();
    this.pendingReads.set(project, {discovery, promise});
    return promise;
  }
  selectedVersion(project: string, job: string): string | undefined {
    const state = this.getDiscovery(project);
    const row = state.status === 'current' && state.inputs.find(input => input.id === job);
    return row ? JSON.stringify([project, requestVersion(row), this.memberships.get(JSON.stringify([project, job])) ?? 0]) : undefined;
  }
  async getInputDetails(project: string, job: string, force = false, _signal?: AbortSignal): Promise<UserInputDetails | null> {
    // Retry always discovers first. A caller cannot turn a route into a probe.
    const before = this.formDetailsCache.get(this.selectedVersion(project, job) ?? '');
    if (force) await this.getPendingInputs(project, true);
    const key = this.selectedVersion(project, job);
    if (!key) return null;
    const previous = this.formDetailsCache.get(key);
    if (previous && (!force || previous !== before)) return previous.promise;
    const row = this.getDiscovery(project).inputs.find(input => input.id === job)!;
    const valid = () => this.selectedVersion(project, job) === key && this.formDetailsCache.get(key) === attempt;
    const revalidate = () => { if (valid()) void this.getPendingInputs(project, true).catch(() => {}); };
    const attempt: DetailAttempt = {project, key, promise: Promise.resolve(null)};
    attempt.promise = (async () => {
      try {
        const response = await fetch(`${API_BASE}/projects/${encodeURIComponent(project)}/user-inputs/${encodeURIComponent(job)}`, {cache: 'no-store'});
        if (!valid()) return null;
        if (response.status === 204) { attempt.details = null; revalidate(); return null; }
        if (!response.ok) throw new APIError(`Failed to fetch input details: ${response.statusText}`, response.status);
        const details = parseJobJSON(await response.text()) as UserInputDetails;
        if (!valid()) return null;
        if (!details || !details.form || typeof details.form !== 'object' || Array.isArray(details.form) || typeof details.jobId !== 'string') throw new Error('Invalid input details');
        if (details.jobId !== job || (row.request_id !== undefined && row.request_id !== details.form.request_id)
          || (row.task_ordinal !== undefined && (details.task_ordinal ?? details.taskOrdinal) !== undefined && stringifyJobJSON(row.task_ordinal) !== stringifyJobJSON(details.task_ordinal ?? details.taskOrdinal))) {
          attempt.details = null; revalidate(); return null;
        }
        attempt.details = details; return details;
      } catch (error) {
        if (!valid()) return null;
        attempt.error = error instanceof Error ? error : new Error(String(error)); throw attempt.error;
      } finally {
        if (!valid() && attempt.details === undefined && this.formDetailsCache.get(key) === attempt) this.formDetailsCache.delete(key);
        this.emit('details_changed');
      }
    })();
    this.formDetailsCache.set(key, attempt);
    return attempt.promise;
  }
  getDetailsState(project: string, job: string): DetailAttempt | undefined {
    const key = this.selectedVersion(project, job);
    return key ? this.formDetailsCache.get(key) : undefined;
  }
  private answered(project: string, job: string, key: string | undefined): void {
    // A successor may be discovered before the completion's HTTP acknowledgement.
    // Only suppress the version captured when the operation started.
    if (key && this.selectedVersion(project, job) === key) {
      this.formDetailsCache.set(key, {project, key, details: null, promise: Promise.resolve(null)});
    }
    this.emit('details_changed');
    this.emit('input_completed', {jobId: job});
    void this.getPendingInputs(project, true).catch(() => {});
  }
  async submitResponse(projectId: string, jobId: string, response: FormResponse): Promise<void> {
    const key = this.selectedVersion(projectId, jobId);
    try {
      const apiResponse = await fetch(`${API_BASE}/projects/${projectId}/user-inputs/${jobId}/respond`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(response),
      });

      if (!apiResponse.ok) {
        throw new Error(`Failed to submit response: ${apiResponse.statusText}`);
      }

      // Clear caches
      this.answered(projectId, jobId, key);
    } catch (error) {
      console.error('Failed to submit response:', error);
      throw error;
    }
  }

  reviewDocumentURL(projectId: string, jobId: string, requestId: string, documentId: string): string {
    const query = new URLSearchParams({ request_id: requestId, document_id: documentId });
    return `${API_BASE}/projects/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(jobId)}/documents?${query}`;
  }

  async submitReview(projectId: string, jobId: string, requestId: string, response: FormResponse): Promise<void> {
    const key = this.selectedVersion(projectId, jobId);
    const submission = {
      request_id: requestId,
      submission_id: createSubmissionId(),
      fields: Object.fromEntries(Object.entries(response.fields).filter(([, value]) => value != null)),
      response: response.response,
    };
    const body = new FormData();
    body.append('submission', JSON.stringify(submission));
    for (const [field, file] of Object.entries(response.attachments || {})) body.append(field, file);
    const result = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(jobId)}/respond`, {
      method: 'POST', body,
    });
    if (!result.ok) {
      const error = await result.json().catch(() => null);
      throw new Error(error?.error || 'Failed to submit review. Reload if this review has changed.');
    }
    this.answered(projectId, jobId, key);
  }

  async submitReviewV2(projectId: string, jobId: string, requestId: string, response: import('../reviewV2').ReviewV2Response, attachments: File[], submissionId = createSubmissionId()): Promise<import('../reviewV2').ReviewV2Output> {
    const key = this.selectedVersion(projectId, jobId);
    const body = new FormData();
    body.append('submission', JSON.stringify({ request_id: requestId, submission_id: submissionId, response }));
    for (const file of attachments) body.append('attachments', file);
    const result = await fetch(`${API_BASE}/projects/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(jobId)}/respond`, { method: 'POST', body });
    if (!result.ok) {
      const error = await result.json().catch(() => null);
      throw new Error(error?.error || 'Submission could not be confirmed. Refresh to check acceptance before retrying.');
    }
    const output = await result.json();
    this.answered(projectId, jobId, key);
    return output;
  }

  async cancelInput(projectId: string, jobId: string, reason?: string): Promise<void> {
    const key = this.selectedVersion(projectId, jobId);
    try {
      const response = await fetch(`${API_BASE}/projects/${projectId}/user-inputs/${jobId}/cancel`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ reason: reason || 'Cancelled by user' }),
      });

      if (!response.ok) {
        throw new Error(`Failed to cancel input: ${response.statusText}`);
      }

      // Clear caches
      this.answered(projectId, jobId, key);
    } catch (error) {
      console.error('Failed to cancel input:', error);
      throw error;
    }
  }

  getConnectionState(): 'connecting' | 'open' | 'closed' {
    if (!this.eventSource) return 'closed';
    switch (this.eventSource.readyState) {
      case 0: // CONNECTING
        return 'connecting';
      case 1: // OPEN
        return 'open';
      default:
        return 'closed';
    }
  }

  getCurrentProjectId(): string | null {
    return this.currentProjectId;
  }

  // Test utility: clear all caches
  clearCaches(): void {
    this.pendingInputsCache.clear();
    this.formDetailsCache.clear();
    this.discovery.clear(); this.memberships.clear(); this.pendingReads.clear();
  }
}

// Create singleton instance
export const inputActivityService = new InputActivityService();
