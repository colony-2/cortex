import { describe, expect, it } from 'vitest';
import { buildStoryTree, storyAttempts, storyNodeName, type StoryNode, type StoryTreeNode } from './storyTree';

let id = 0;
function node(kind: StoryNode['kind'], title: string, children: StoryNode[] = [], extra: Partial<StoryNode> = {}): StoryNode {
  return { id: `n${++id}`, kind, title, children, path: [], status: 'succeeded', ...extra };
}
const selector = 'git+https://github.com/colony-2/c2ops.git//codex@ded76dfbd877d3d0749e509844ecdbc57197b572';
const flat = (nodes: StoryTreeNode[]): StoryTreeNode[] => nodes.flatMap(n => [n, ...flat(n.children)]);

function operation() {
  const prior = node('opStep', 'step extension_execution', [], {
    attempt: 1, task_ordinal: 30, status: 'failed', error: { message: 'first failure' },
  });
  const latest = node('opStep', 'step extension_execution', [], {
    attempt: 2, task_ordinal: 31, restart_from_ordinal: 30, status: 'failed',
    error: { message: 'second failure' }, artifact_keys: [{ jobId: 'job', taskOrdinal: 31, name: 'stderr.txt' }],
  });
  const op = node('op', `op ${selector}`, [
    node('opStep', 'step recipe_timeout_checkpoint', [], { step_id: 'recipe_timeout_checkpoint' }), prior, latest,
  ], { status: 'failed' });
  return { op, prior, latest };
}

describe('compact execution tree', () => {
  it('collapses wrappers and sibling retries, retaining the task as the inspection target', () => {
    const { op, prior, latest } = operation();
    const root = node('recipe', 'recipe build', [
      node('recipeSourceResolution', 'recipe source resolution'),
      node('sequence', 'sequence build', [node('stateMachine', 'stateMachine development-agent', [
        node('state', 'state run', [node('stateMachine', 'stateMachine state_machine', [
          node('state', 'state fresh', [op]),
        ])]),
      ])]),
    ], { status: 'failed' });
    const model = buildStoryTree(root);
    const rows = flat(model.nodes);
    expect(rows.map(n => n.label)).toEqual(['build', 'development-agent.run.codex']);
    expect(rows[1].node).toBe(latest);
    expect(rows[1].node.restart_from_ordinal).toBe(30);
    expect(rows[1].node.artifact_keys?.[0].name).toBe('stderr.txt');
    expect(model.retriesByKey.get(rows[1].key)).toEqual([prior]);
    expect(storyAttempts(model, model.keyFor.get(prior)!).map(ref => ref.attempt)).toEqual([2, 1]);
    expect(storyAttempts(model, model.keyFor.get(op)!)).toHaveLength(1);
    expect(model.keyToRef.get(model.keyFor.get(prior)!)?.node.error?.message).toBe('first failure');
    expect(root.children).toHaveLength(2); // Projection never mutates the response.
    expect(op.children).toHaveLength(3);
  });

  it('preserves meaningful branches and multi-step operations', () => {
    const root = node('recipe', 'recipe build', [node('sequence', 'sequence release', [
      node('op', 'op publish', [node('opStep', 'step upload'), node('opStep', 'step verify')]),
      node('op', 'op notify'),
    ])]);
    const rows = flat(buildStoryTree(root).nodes);
    expect(rows.map(n => n.label)).toEqual(['build', 'release', 'release.publish', 'upload', 'verify', 'release.notify']);
    expect(rows[2].children).toHaveLength(2);
  });

  it('does not coalesce separate invocations or repeated calls with reset attempt numbers', () => {
    const children = [1, 2, 1, 2].map((attempt, i) => node('opStep', 'step run', [], {
      attempt, invoke_seq: i === 3 ? 2 : 1,
    }));
    const root = node('recipe', 'recipe build', children);
    expect(buildStoryTree(root).nodes[0].children).toHaveLength(3);
  });

  it('keeps source-resolution failures and structural failures visible', () => {
    const failure = node('recipeSourceResolution', 'recipe source resolution', [], { status: 'failed', error: { message: 'missing recipe' } });
    const scope = node('stateMachine', 'stateMachine build', [node('op', 'op done')], { status: 'failed', error: { message: 'bad transition' } });
    const rows = flat(buildStoryTree(node('recipe', 'recipe build', [failure, scope])).nodes);
    expect(rows.some(n => n.node === failure && n.label === 'Resolve recipe')).toBe(true);
    expect(rows.some(n => n.node === scope)).toBe(true);
  });

  it('indexes prior job attempts without displaying them in the main tree', () => {
    const past = node('recipe', 'recipe build', [node('op', 'op old')], { id: 'root', job_attempt: 1 });
    const root = node('recipe', 'recipe build', [node('op', 'op new')], { id: 'root', job_attempt: 2, past_attempts: [past] });
    const model = buildStoryTree(root);
    expect(flat(model.nodes).map(n => n.label)).toEqual(['build', 'new']);
    expect(model.keyToRef.get('jobAttempt:1|root')?.node).toBe(past);
    expect(model.legacyKeyToKey.get('root')).toBe('jobAttempt:2|root');
    expect(storyAttempts(model, 'jobAttempt:1|root').map(ref => ref.jobAttempt)).toEqual([2, 1]);
    expect(storyAttempts(model, model.keyFor.get(past.children![0])!)).toHaveLength(1);
  });

  it('uses explicit names, shortening only qualified source selectors', () => {
    expect(storyNodeName(node('op', 'op Review changes', [], { op_id: selector }))).toBe('Review changes');
    expect(storyNodeName(node('op', `op ${selector}`))).toBe('codex');
  });
});
