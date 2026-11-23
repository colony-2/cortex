import { API_BASE } from './apiBase';
import { APIError } from './api';
import type { JobLeaseEventPage } from './types';

export interface PulseInstance {
  id: string;
  launch_id: string;
  state: string;
  job: { jobdb: string; instance_id: string; tenant_id: string; job_id: string };
  refs: string[];
  created_at?: string;
  started_at?: string;
}
export type NamedInstance = PulseInstance & { provider: string };
export interface PulseInstances {
  items: NamedInstance[];
  complete: boolean;
  errors: { provider: string; error: string }[];
}
export interface PulseHealth {
  name: string;
  implementation: string;
  version: string;
  protocol: number;
  state: string;
  restart_count: number;
  generation: number;
  last_failure?: string;
}
export interface PulseProvider {
  name: string;
  type: string;
  discovery: 'known' | 'unknown' | 'unavailable' | 'timeout' | 'invalid' | 'error';
  bases: { name: string; capabilities: string[] }[];
  bases_complete: boolean;
  health?: PulseHealth;
}
export interface PulseConfig {
  providers?: Record<string, Record<string, unknown>>;
  defaults?: Record<string, unknown>;
  targets?: {
    instance_id?: string;
    jobdb?: string;
    launch_services?: { name: string; priority: number }[];
  }[];
}
export interface PulseStatus {
  status: string;
  version: string;
  started_at: string;
  providers: number;
  poll: { last_finished: string; passes: number; failed_passes: number };
}
export interface LogPage {
  records: { sequence: number; timestamp: string; source: string; data: string }[];
  next_page_token: string;
  has_more: boolean;
  finalized: boolean;
  incomplete: boolean;
  estimated_total_size_bytes?: number;
}
async function read<T>(path: string, signal: AbortSignal, allowPartial = false): Promise<T> {
  const res = await fetch(API_BASE + path, { signal });
  const body = await res.json().catch(() => null);
  if (
    allowPartial &&
    res.status === 502 &&
    body?.complete === false &&
    Array.isArray(body.items) &&
    Array.isArray(body.errors)
  )
    return body;
  if (!res.ok || body === null)
    throw new APIError(body?.error || `Request failed (HTTP ${res.status})`, res.status, body?.code);
  return body;
}
const providerBase = (name: string) => '/pulse/providers/' + encodeURIComponent(name);
export const pulseConfig = (signal: AbortSignal) => read<PulseConfig>('/pulse/config', signal);
export const pulseStatus = (signal: AbortSignal) => read<PulseStatus>('/pulse/status', signal);
export const pulseProviders = (signal: AbortSignal) =>
  read<{ items: PulseProvider[] }>('/pulse/providers', signal);
export const providerCapabilities = (provider: string, signal: AbortSignal) =>
  read<PulseProvider>(providerBase(provider) + '/capabilities', signal);
export const pulseHealth = (signal: AbortSignal) =>
  read<{ items: PulseHealth[] }>('/pulse/providers/health', signal);
export function pulseInstances(signal: AbortSignal, launch?: string) {
  const query = launch ? '?' + new URLSearchParams({ launch_id: launch }) : '';
  return read<PulseInstances>('/pulse/instances' + query, signal, true);
}
export function providerInstances(provider: string, signal: AbortSignal, token?: string, launch?: string) {
  const q = new URLSearchParams({ page_size: '100' });
  if (launch) q.set('launch_id', launch);
  if (token) q.set('page_token', token);
  return read<{ items: PulseInstance[]; next_page_token?: string }>(
    providerBase(provider) + '/instances?' + q,
    signal,
  );
}
export async function launchInstances(
  launch: string,
  provider: string | undefined,
  signal: AbortSignal,
): Promise<PulseInstances> {
  if (!provider) return pulseInstances(signal, launch);
  const items: NamedInstance[] = [];
  const seen = new Set<string>();
  let token: string | undefined;
  do {
    const page = await providerInstances(provider, signal, token, launch);
    items.push(...page.items.map((instance) => ({ ...instance, provider })));
    token = page.next_page_token;
    if (token && seen.has(token)) {
      return { items, complete: false, errors: [{ provider, error: 'Repeated instance page token.' }] };
    }
    if (token) seen.add(token);
  } while (token);
  return { items, complete: true, errors: [] };
}
export const pulseLogDownload = (provider: string, launch: string) =>
  API_BASE + providerBase(provider) + '/launches/' + encodeURIComponent(launch) + '/download';
export function pulseLog(
  provider: string,
  launch: string,
  signal: AbortSignal,
  size: number,
  token?: string,
) {
  const q = new URLSearchParams({ page_size: String(size) });
  if (token) q.set('page_token', token);
  return read<LogPage>(
    providerBase(provider) + '/launches/' + encodeURIComponent(launch) + '/logs?' + q,
    signal,
  );
}
export function launchHistory(
  tenant: string,
  signal: AbortSignal,
  job: string,
  launch: string,
  token?: string,
) {
  const q = new URLSearchParams();
  if (job) q.set('jobId', job);
  if (launch) q.set('launchId', launch);
  if (token) q.set('pageToken', token);
  return read<JobLeaseEventPage>(`/projects/${encodeURIComponent(tenant)}/runner-launches?${q}`, signal);
}
