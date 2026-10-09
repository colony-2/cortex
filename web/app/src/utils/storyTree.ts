type NodeStatus =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'skipped'
  | 'unknown';

export type StoryKind =
  | 'recipe'
  | 'recipeSourceResolution'
  | 'sequence'
  | 'op'
  | 'opStep'
  | 'stateMachine'
  | 'state'
  | 'transitionEval'
  | 'contextPatch';

export interface ArtifactKey {
  jobId: string;
  taskOrdinal: number;
  name: string;
  sizeBytes?: number;
}

type TransitionDecision =
  | { kind: 'state'; to_state_id: string }
  | { kind: 'fallthrough' };

interface TransitionEvaluation {
  expression: string;
  result: boolean;
  to_state_id: string;
}

export interface StoryNode {
  id?: string;
  kind: StoryKind;
  title: string;
  status: NodeStatus;
  started_at?: string | null;
  finished_at?: string | null;
  path: string[];
  invoke_seq?: number;
  input?: unknown | null;
  output?: unknown | null;
  artifact_keys?: ArtifactKey[];
  children?: StoryNode[];
  attempt?: number;
  job_attempt?: number;
  past_attempts?: StoryNode[];
  prior_attempts?: StoryNode[];
  error?: { message: string; code?: string } | null;
  task_ordinal?: number | null;
  restart_from_ordinal?: number | null;

  op_id?: string;
  step_id?: string;
  state_machine_id?: string;
  state_id?: string;
  sequence_id?: string;

  // transitionEval-only
  evaluations?: TransitionEvaluation[];
  decision?: TransitionDecision;
}


export interface StoryNodeRef {
  type: 'node';
  node: StoryNode;
  key: string;
  attempt: number;
  jobAttempt: number;
}

export interface StoryTreeNode {
  key: string;
  node: StoryNode;
  label: string;
  children: StoryTreeNode[];
}

// Keep source selectors intact in the model; only presentation names are shortened.
export function storyNodeName(node: StoryNode): string {
  const prefix: Partial<Record<StoryKind, string>> = {
    recipe: 'recipe ', sequence: 'sequence ', stateMachine: 'stateMachine ',
    state: 'state ', op: 'op ', opStep: 'step ',
  };
  let name = node.title || node.op_id || node.step_id || node.state_id || node.state_machine_id || node.sequence_id || node.kind;
  if (prefix[node.kind] && name.startsWith(prefix[node.kind]!)) name = name.slice(prefix[node.kind]!.length);
  if (/^(git\+|https?:\/\/|file:\/\/)/.test(name)) {
    name = name.slice(name.lastIndexOf('/') + 1).replace(/@[^@]*$/, '') || 'operation';
  }
  if (node.kind === 'recipeSourceResolution') return 'Resolve recipe';
  if (node.kind === 'transitionEval') return 'Choose next state';
  if (node.step_id === 'recipe_timeout_checkpoint' || name === 'recipe_timeout_checkpoint') return 'Check timeout';
  return name;
}

const genericNames = new Set(['root', 'state_machine', 'stateMachine', 'sequence', 'recipe']);

function operationLabel(node: StoryNode, ancestors: StoryNode[]): string {
  const op = node.kind === 'opStep' ? [...ancestors].reverse().find(n => n.kind === 'op') : undefined;
  const owner = op || node;
  const scopes = op ? ancestors.slice(0, ancestors.indexOf(op)) : ancestors;
  const machines = scopes.filter(n => n.kind === 'stateMachine');
  const machine = [...machines].reverse().find(n => !genericNames.has(storyNodeName(n))) || machines[machines.length - 1];
  let names: string[] = [];
  if (machine) {
    const state = scopes.slice(scopes.indexOf(machine) + 1).find(n => n.kind === 'state');
    names = [storyNodeName(machine), ...(state ? [storyNodeName(state)] : [])];
  } else {
    const sequence = [...scopes].reverse().find(n => n.kind === 'sequence' && !genericNames.has(storyNodeName(n)));
    const state = [...scopes].reverse().find(n => n.kind === 'state');
    names = [...(sequence ? [storyNodeName(sequence)] : []), ...(state ? [storyNodeName(state)] : [])];
  }
  names.push(storyNodeName(owner));
  return names.filter((name, index) => index === 0 || name !== names[index - 1]).join('.');
}

function isBookkeeping(node: StoryNode) {
  return node.kind === 'recipeSourceResolution' || node.kind === 'transitionEval' ||
    node.step_id === 'recipe_timeout_checkpoint' || node.title === 'step recipe_timeout_checkpoint';
}

function needsAttention(node: StoryNode) {
  return !!node.error || ['failed', 'running', 'pending', 'canceled'].includes(node.status);
}

export function buildStoryTree(root?: StoryNode | null) {
  const keyToRef = new Map<string, StoryNodeRef>();
  const keyToParent = new Map<string, string | null>();
  const legacyKeyToKey = new Map<string, string>();
  const keyFor = new Map<StoryNode, string>();
  const ancestorsByKey = new Map<string, StoryNode[]>();
  const retriesByKey = new Map<string, StoryNode[]>();
  const currentByRetryKey = new Map<string, string>();
  const childrenByNode = new Map<StoryNode, StoryNode[]>();
  const visibleKeyByKey = new Map<string, string>();
  const projectionByKey = new Map<string, StoryTreeNode[]>();
  let synthetic = 0;

  const index = (node: StoryNode, ancestors: StoryNode[], jobAttempt: number, retryOwner?: string) => {
    if (keyFor.has(node)) return;
    jobAttempt = node.job_attempt || jobAttempt;
    const key = node.id ? `jobAttempt:${jobAttempt}|${node.id}` : `synthetic:${++synthetic}`;
    keyFor.set(node, key);
    keyToRef.set(key, { type: 'node', node, key, attempt: node.attempt || 1, jobAttempt });
    ancestorsByKey.set(key, ancestors);
    if (retryOwner) currentByRetryKey.set(key, retryOwner);
    for (const legacy of [node.id, `${node.path.join('/')}|attempt:${node.attempt || 1}`]) {
      if (legacy && !legacyKeyToKey.has(legacy)) legacyKeyToKey.set(legacy, key);
    }
    retriesByKey.set(key, [...(node.prior_attempts || [])]);
    for (const child of node.children || []) index(child, [...ancestors, node], jobAttempt, retryOwner);
    for (const prior of node.prior_attempts || []) index(prior, ancestors, jobAttempt, key);
    for (const [i, past] of (node.past_attempts || []).entries()) index(past, [], past.job_attempt || i + 1, key);

    // Older replay responses emit retries as consecutive sibling steps. Only
    // fold an increasing attempt of the same invocation, never repeated calls.
    const children: StoryNode[] = [];
    for (const child of node.children || []) {
      const previous = children[children.length - 1];
      if (previous && child.kind === 'opStep' && previous.kind === child.kind &&
          (child.attempt || 1) > (previous.attempt || 1) &&
          child.title === previous.title && child.step_id === previous.step_id &&
          child.invoke_seq === previous.invoke_seq && JSON.stringify(child.path) === JSON.stringify(previous.path)) {
        const previousKey = keyFor.get(previous)!;
        const childKey = keyFor.get(child)!;
        const retries = [...(retriesByKey.get(previousKey) || []), previous, ...(retriesByKey.get(childKey) || [])];
        retriesByKey.set(childKey, [...new Set(retries)]);
        for (const prior of retries) currentByRetryKey.set(keyFor.get(prior)!, childKey);
        children[children.length - 1] = child;
      } else children.push(child);
    }
    childrenByNode.set(node, children);
  };
  if (root) index(root, [], root.job_attempt || 1);

  const project = (node: StoryNode): StoryTreeNode[] => {
    const key = keyFor.get(node)!;
    const ancestors = ancestorsByKey.get(key)!;
    const children = (childrenByNode.get(node) || []).flatMap(project);
    let result: StoryTreeNode[];
    if (isBookkeeping(node) && !needsAttention(node)) {
      result = children;
    } else if (node.kind === 'op' && children.length === 1 && children[0].node.kind === 'opStep' &&
        node.status === children[0].node.status && (!node.error || children[0].node.error)) {
      // The task is the useful inspection target: it owns errors, artifacts,
      // retry history and the restart cursor. Its enclosing op supplies its name.
      const leaf = children[0];
      leaf.label = operationLabel(node, ancestors);
      retriesByKey.set(leaf.key, [...new Set([...(node.prior_attempts || []), ...(retriesByKey.get(leaf.key) || [])])]);
      result = children;
    } else if (node.kind === 'recipe' || node.kind === 'op' || node.kind === 'opStep' ||
        node.kind === 'contextPatch' || isBookkeeping(node) ||
        (needsAttention(node) && !children.some(child => child.node.status === node.status))) {
      result = [{ key, node, label: node.kind === 'op' || node.kind === 'opStep'
        ? operationLabel(node, ancestors) : storyNodeName(node), children }];
      // Within a multi-step operation, the parent already supplies the scope.
      if (node.kind === 'op') for (const child of children) child.label = storyNodeName(child.node);
    } else if ((node.kind === 'sequence' || node.kind === 'stateMachine') &&
        (childrenByNode.get(node) || []).filter(n => !isBookkeeping(n)).length > 1 && children.length > 1 &&
        !genericNames.has(storyNodeName(node))) {
      result = [{ key, node, label: storyNodeName(node), children }];
    } else {
      result = children;
    }
    projectionByKey.set(key, result);
    return result;
  };
  const nodes = root ? project(root) : [];
  // Project historical nodes separately, so they can be explored in Retries
  // without adding any historical rows to the main execution tree.
  for (const ref of keyToRef.values()) if (!projectionByKey.has(ref.key)) project(ref.node);
  const connect = (rows: StoryTreeNode[], parent: string | null) => {
    for (const row of rows) {
      keyToParent.set(row.key, parent);
      visibleKeyByKey.set(row.key, row.key);
      connect(row.children, row.key);
    }
  };
  connect(nodes, null);
  for (const [key, projected] of projectionByKey) {
    if (!visibleKeyByKey.has(key) && projected[0]) visibleKeyByKey.set(key, projected[0].key);
  }
  return { nodes, keyToRef, keyToParent, legacyKeyToKey, keyFor, ancestorsByKey,
    retriesByKey, currentByRetryKey, projectionByKey, visibleKeyByKey, childrenByNode };
}
