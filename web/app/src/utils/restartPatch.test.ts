import { describe, expect, it } from 'vitest';
import { parseJobJSON, stringifyJobJSON, type RestartContext } from '@colony2/shared';
import { buildContextPatch, type PatchLine, type ScopePatchLine } from './restartPatch';
const snapshot = (raw = '{}', redacted_paths: string[] = []): RestartContext => ({ project_id: 't', job_id: 'j', chapter_ordinal: 1, scope_id: 's', job: parseJobJSON(raw) as Record<string, unknown>, visible: { sequence: { 'a/~': { outputs: { nested: { keep: 1, remove: true }, list: [1, 2] } } } }, redacted_paths });
const line = (path: string, value: string): PatchLine => ({ id: path, path, value });
const local = (path: string, value: string): ScopePatchLine => ({ ...line(path, value), container: 'sequence', scopeId: 'a/~' });
describe('generated restart diff', () => {
  it('preserves recursive intent, siblings, null deletion and deterministic order', () => {
    const before = snapshot('{"git":{"author":"old","email":"keep"}}');
    const patch = buildContextPatch(before, [line('git', '{"author":"new","absent":null}')], [local('nested', '{"remove":null,"a/b~c":2}'), local('list', '[3,4]')]);
    expect(patch).toEqual([
      { op: 'replace', path: '/context/git/author', value: 'new' },
      { op: 'replace', path: '/sequence/a~1~0/outputs/list', value: [3, 4] },
      { op: 'add', path: '/sequence/a~1~0/outputs/nested/a~1b~0c', value: 2 },
      { op: 'remove', path: '/sequence/a~1~0/outputs/nested/remove' },
    ]);
    expect(before.job).toEqual({ git: { author: 'old', email: 'keep' } });
  });
  it('preserves exact numbers in edits, baseline comparisons, nested arrays and request serialization', () => {
    const before = snapshot('{"workflow":{"n":9007199254740993,"untouched":9007199254740995,"list":[9007199254740997]}}');
    expect(buildContextPatch(before, [line('workflow.n', '9007199254740993'), line('workflow.list', '[9007199254740997]')], [])).toEqual([]);
    const patch = buildContextPatch(before, [line('workflow.n', '9007199254740999'), line('workflow.list', '[9007199254740999]')], []);
    expect(stringifyJobJSON(patch)).toBe('[{"op":"replace","path":"/context/workflow/list","value":[9007199254740999]},{"op":"replace","path":"/context/workflow/n","value":9007199254740999}]');
    expect(stringifyJobJSON(parseJobJSON(stringifyJobJSON(patch)!))).toBe(stringifyJobJSON(patch));
  });
  it('keeps text fallback and resolves overlapping rows in order', () => {
    expect(buildContextPatch(snapshot('{"git":{"author":"old"}}'), [line('git', '{"author":"one"}'), line('git.author', 'plain text')], [])).toEqual([{ op: 'replace', path: '/context/git/author', value: 'plain text' }]);
  });
  it('blocks unavailable parents and redacted affected regions without blocking unrelated redaction', () => {
    expect(() => buildContextPatch(undefined, [], [])).toThrow('Load context');
    expect(() => buildContextPatch(snapshot(), [line('git.author', 'a')], [])).toThrow('parent');
    expect(() => buildContextPatch(snapshot(), [], [{ ...local('a', '1'), scopeId: 'missing' }])).toThrow('parent');
    for (const redacted of ['/job/git', '/job/git/author', '/job/git/author/secret']) {
      expect(() => buildContextPatch(snapshot('{"git":{"author":"[REDACTED]"}}', [redacted]), [line('git.author', 'a')], [])).toThrow('redacted');
    }
    expect(buildContextPatch(snapshot('{"git":{"author":"old"}}', ['/job/environment/secret']), [line('git.author', 'new')], [])).toHaveLength(1);
    expect(buildContextPatch(snapshot('{"git":{"author":"old","secret":"[REDACTED]"}}', ['/job/git/secret']), [line('git', '{"author":"new"}')], [])).toEqual([{ op: 'replace', path: '/context/git/author', value: 'new' }]);
    expect(() => buildContextPatch(snapshot(), [line('workspace', '{}')], [])).toThrow('Unsupported');
  });
  it('emits an explicit empty array for unchanged loaded drafts', () => {
    expect(buildContextPatch(snapshot(), [], [])).toEqual([]);
  });
});
