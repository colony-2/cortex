export interface Trust { mode: 'candidate' | 'commit'; commit?: string }
export interface Permissions {
  secrets?: { resolve?: string[] };
  jobs?: { submit?: boolean; cellNames?: string[] };
  cells?: { create?: boolean; cellNames?: string[] };
}
export interface HookPolicy {
  perms: { jobdb?: Permissions; limit?: { jobdb?: Permissions } };
  source_trust?: { 'jobdb.secrets.resolve'?: Record<string, Trust>; 'jobdb.jobs.submit'?: Trust; 'jobdb.cells.create'?: Trust };
}
export interface GitProfile { version: number; entries: { url: string; rewrite_from?: string[]; username?: string; secret_ref?: string }[] }
export interface SecretDraft { id: string; name: string; trust: Trust }
export interface ActionDraft { allowed: boolean; restricted: boolean; names: string; trust: Trust }
export interface PermissionsDraft { secrets: SecretDraft[]; jobs: ActionDraft; cells: ActionDraft }
export interface HookDraft { grant: PermissionsDraft; limited: boolean; limit: PermissionsDraft }
export interface GitDraft { id: string; url: string; rewrites: string; username: string; secret: string }
// Form row identities are local UI keys and must also work over plain HTTP.
let rowSequence = 0;
const rowId = () => `settings-row-${++rowSequence}`;
const newTrust = (): Trust => ({ mode: 'commit', commit: '' });
export const newSecret = (): SecretDraft => ({ id: rowId(), name: '', trust: newTrust() });
export const newDestination = (): GitDraft => ({ id: rowId(), url: '', rewrites: '', username: '', secret: '' });
const lines = (text: string) => text.split(/[\n,]/).map(value => value.trim()).filter(Boolean);
export function permissionsDraft(value: Permissions = {}, trust: HookPolicy['source_trust'] = {}): PermissionsDraft {
  return {
    secrets: (value.secrets?.resolve || []).map(name => ({ id: rowId(), name, trust: trust['jobdb.secrets.resolve']?.[name] || newTrust() })),
    jobs: { allowed: !!value.jobs?.submit, restricted: value.jobs?.cellNames !== undefined, names: value.jobs?.cellNames?.join('\n') || '', trust: trust['jobdb.jobs.submit'] || newTrust() },
    cells: { allowed: !!value.cells?.create, restricted: value.cells?.cellNames !== undefined, names: value.cells?.cellNames?.join('\n') || '', trust: trust['jobdb.cells.create'] || newTrust() },
  };
}
export function hookDraft(value?: HookPolicy): HookDraft {
  return { grant: permissionsDraft(value?.perms.jobdb, value?.source_trust), limited: value?.perms.limit?.jobdb !== undefined, limit: permissionsDraft(value?.perms.limit?.jobdb) };
}
function trustValue(trust: Trust): Trust {
  if (trust.mode === 'candidate') return { mode: 'candidate' };
  const commit = trust.commit?.trim() || '';
  if (!/^[0-9a-f]{40}$/.test(commit)) throw new Error('Enter a full 40-character lowercase commit SHA for each commit restriction.');
  return { mode: 'commit', commit };
}
function permissionValue(draft: PermissionsDraft): Permissions {
  const names = draft.secrets.map(row => row.name.trim());
  if (names.some(name => !/^[A-Za-z_][A-Za-z0-9_-]{0,255}$/.test(name))) throw new Error('Enter a valid name for each secret.');
  if (new Set(names).size !== names.length) throw new Error('Each secret can be granted only once.');
  const targets = (action: ActionDraft) => {
    if (!action.restricted) return {};
    const names = lines(action.names);
    if (!names.length) throw new Error('Specify at least one cell when restricting cell access.');
    if (new Set(names).size !== names.length) throw new Error('Cell names must not be repeated.');
    return { cellNames: names };
  };
  return {
    ...(names.length ? { secrets: { resolve: names } } : {}),
    ...(draft.jobs.allowed ? { jobs: { submit: true, ...targets(draft.jobs) } } : {}),
    ...(draft.cells.allowed ? { cells: { create: true, ...targets(draft.cells) } } : {}),
  };
}
export function hookValue(draft: HookDraft): HookPolicy {
  const grant = permissionValue(draft.grant);
  const source: NonNullable<HookPolicy['source_trust']> = {};
  if (draft.grant.secrets.length) source['jobdb.secrets.resolve'] = Object.fromEntries(draft.grant.secrets.map(row => [row.name.trim(), trustValue(row.trust)]));
  if (draft.grant.jobs.allowed) source['jobdb.jobs.submit'] = trustValue(draft.grant.jobs.trust);
  if (draft.grant.cells.allowed) source['jobdb.cells.create'] = trustValue(draft.grant.cells.trust);
  const limit = draft.limited ? permissionValue(draft.limit) : undefined;
  if (limit) {
    if (grant.secrets?.resolve?.some(name => !limit.secrets?.resolve?.includes(name))) throw new Error('The permission limit must include every granted secret.');
    for (const kind of ['jobs', 'cells'] as const) {
      const allowed = kind === 'jobs' ? grant.jobs?.submit : grant.cells?.create;
      const permitted = kind === 'jobs' ? limit.jobs?.submit : limit.cells?.create;
      if (allowed && (!permitted || (limit[kind]?.cellNames && (!grant[kind]?.cellNames || grant[kind]!.cellNames!.some(name => !limit[kind]!.cellNames!.includes(name)))))) throw new Error('The permission limit must include all granted job and cell actions.');
    }
  }
  return { perms: { jobdb: grant, ...(limit ? { limit: { jobdb: limit } } : {}) }, ...(Object.keys(source).length ? { source_trust: source } : {}) };
}
export function gitDraft(value?: GitProfile): GitDraft[] {
  return value ? value.entries.map(entry => ({ id: rowId(), url: entry.url, rewrites: entry.rewrite_from?.join('\n') || '', username: entry.username || '', secret: entry.secret_ref || '' })) : [newDestination()];
}
export function gitValue(draft: GitDraft[]): GitProfile {
  if (!draft.length) throw new Error('Add at least one repository destination.');
  return { version: 1, entries: draft.map(entry => {
    const url = entry.url.trim(), username = entry.username.trim(), secret = entry.secret.trim();
    let parsed: URL;
    try { parsed = new URL(url); } catch { throw new Error('Each repository destination needs a valid HTTP or HTTPS URL.'); }
    if (!['https:', 'http:'].includes(parsed.protocol) || parsed.username || parsed.password || parsed.search || parsed.hash) throw new Error('Repository URLs must use HTTP or HTTPS without credentials, query strings, or fragments.');
    if (!!username !== !!secret) throw new Error('Provide both a username and a secret name, or leave both empty.');
    if (secret && !/^[A-Za-z_][A-Za-z0-9_-]{0,255}$/.test(secret)) throw new Error('Enter a valid credential secret name.');
    const rewrites = entry.rewrites.split('\n').map(value => value.trim()).filter(Boolean);
    return { url, ...(rewrites.length ? { rewrite_from: rewrites } : {}), ...(username ? { username, secret_ref: secret } : {}) };
  }) };
}
