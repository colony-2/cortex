import { describe, expect, it } from 'vitest';
import { restartScopeForNode } from './restartScope';
import type { StoryNode } from './storyTree';

const op = 'root/scope:"op:build"#1';
const nested = `${op}/attempt:2/scope:"task:block"#1`;
const scopes = [nested, 'root', op, 'root/scope:"op:build"#10'].map(scope_id => ({ scope_id, started_at: '' }));
const node: StoryNode = { kind: 'opStep', title: 'build', status: 'failed', path: [] };

describe('restart selection', () => {
  it('selects the nearest recorded boundary for the inspected invocation and retry', () => {
    expect(restartScopeForNode({ ...node, invocation_id: `${nested}/attempt:1/task:"run"#1/attempt:3` }, scopes)).toBe(nested);
    expect(restartScopeForNode({ ...node, invocation_id: `${op}/attempt:1/task:"run"#1` }, scopes)).toBe(op);
  });
  it('does not confuse repeated calls whose IDs share a prefix', () => {
    const repeated = 'root/scope:"op:build"#10';
    expect(restartScopeForNode({ ...node, invocation_id: `${repeated}/attempt:1/task:"run"#1` }, scopes)).toBe(repeated);
  });
  it('selects the outer recorded boundary for the recipe regardless of list order', () => {
    expect(restartScopeForNode({ ...node, kind: 'recipe' }, scopes)).toBe('root');
  });
  it('does not guess from titles or choose an unrelated scope', () => {
    expect(restartScopeForNode(node, scopes)).toBeUndefined();
    expect(restartScopeForNode({ ...node, invocation_id: 'other/task:1' }, scopes)).toBeUndefined();
    expect(restartScopeForNode({ ...node, kind: 'recipe' }, [scopes[0], { scope_id: 'other', started_at: '' }])).toBeUndefined();
  });
});
