import { expect, it } from 'vitest';
import { gitDraft, gitValue, hookDraft, hookValue } from './models';
import type { HookPolicy } from './models';

export const policy: HookPolicy = {
  perms: {
    jobdb: { secrets: { resolve: ['token', 'other'] }, jobs: { submit: true, cellNames: ['app'] }, cells: { create: true, cellNames: ['new-app'] } },
    limit: { jobdb: { secrets: { resolve: ['token', 'other', 'extra'] }, jobs: { submit: true, cellNames: ['app', 'worker'] }, cells: { create: true } } },
  },
  source_trust: { 'jobdb.secrets.resolve': { token: { mode: 'commit', commit: 'a'.repeat(40) }, other: { mode: 'candidate' } }, 'jobdb.jobs.submit': { mode: 'commit', commit: 'b'.repeat(40) }, 'jobdb.cells.create': { mode: 'candidate' } },
};
it('preserves distinct trust, target cells and the permission ceiling through editing', () => {
  expect(hookValue(hookDraft(policy))).toEqual(policy);
  const draft = hookDraft(policy);
  draft.grant.secrets.splice(0, 1);
  draft.grant.jobs.allowed = false;
  const value = hookValue(draft);
  expect(value.source_trust?.['jobdb.secrets.resolve']).toEqual({ other: { mode: 'candidate' } });
  expect(value.source_trust).not.toHaveProperty('jobdb.jobs.submit');
  expect(value.perms.jobdb?.jobs).toBeUndefined();
  expect(value.perms.limit).toEqual(policy.perms.limit);
});
it('keeps explicit deny-all ceilings and rejects missing restrictions instead of broadening access', () => {
  expect(hookValue(hookDraft({ perms: { jobdb: {}, limit: { jobdb: {} } } }))).toEqual({ perms: { jobdb: {}, limit: { jobdb: {} } } });
  const draft = hookDraft(policy);
  draft.grant.jobs.names = '';
  expect(() => hookValue(draft)).toThrow('Specify at least one cell');
  draft.grant.jobs.names = 'unexpected';
  expect(() => hookValue(draft)).toThrow('permission limit');
  draft.grant.jobs.names = 'app';
  draft.grant.secrets[0].trust.commit = '';
  expect(() => hookValue(draft)).toThrow('commit SHA');
});
it('round-trips Git destinations, rewrites, and credential references without secret values', () => {
  const profile = { version: 1, entries: [
    { url: 'https://github.com/acme/', rewrite_from: ['git@github.com:acme/', 'https://old.example/acme/'], username: 'git', secret_ref: 'token' },
    { url: 'https://public.example/' },
  ] };
  expect(gitValue(gitDraft(profile))).toEqual(profile);
  const draft = gitDraft(profile);
  draft[0].secret = '';
  expect(() => gitValue(draft)).toThrow('both a username and a secret name');
  draft[0].username = '';
  expect(gitValue(draft).entries[0]).not.toHaveProperty('secret_ref');
  draft[0].url = 'https://user:password@example.com';
  expect(() => gitValue(draft)).toThrow('without credentials');
});
