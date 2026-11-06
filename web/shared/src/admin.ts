import { API_BASE } from './apiBase';

export type AdminKind = 'hooks' | 'configs' | 'git' | 'secrets';
export interface AdminEntry { name: string; value?: unknown; source?: string }
export interface AdminSnapshot {
  revision: string;
  hooks: AdminEntry[];
  configs: AdminEntry[];
  git: AdminEntry[];
  secretReferences: string[];
  secretWrites: boolean;
  repository?: string;
}
export interface AdminMutation { name: string; operation: 'set' | 'remove' | 'clear' | 'inherit'; value?: unknown; revision?: string }
function adminURL(tenant: string, kind?: AdminKind, cellId?: string) {
  return `${API_BASE}/projects/${encodeURIComponent(tenant)}/admin${kind ? `/${kind}` : ''}${cellId ? `?cellId=${encodeURIComponent(cellId)}` : ''}`;
}
async function check(response: Response, secret = false) {
  if (response.ok) return;
  // Secret responses are deliberately never echoed, even if a proxy upstream
  // accidentally includes credentials in its error body.
  if (secret) throw new Error('Secret update failed. Verify that JobDB has a writable secret store configured.');
  const body = await response.json().catch(() => ({}));
  throw new Error(body.error || `Settings request failed (${response.status})`);
}
export async function getAdmin(tenant: string, cellId?: string): Promise<AdminSnapshot> {
  const response = await fetch(adminURL(tenant, undefined, cellId), { cache: 'no-store' });
  await check(response);
  return response.json();
}
export async function updateAdmin(tenant: string, kind: AdminKind, input: AdminMutation, cellId?: string): Promise<void> {
  const response = await fetch(adminURL(tenant, kind, cellId), {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  });
  await check(response, kind === 'secrets');
}
