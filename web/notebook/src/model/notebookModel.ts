import type { NodeStatus, StoryKind, StoryNode } from '../backend/types';

export type NotebookMode = 'monitor' | 'run' | 'build';

export type NotebookCellKind = 'markdown' | 'recipe' | 'sequence' | 'op' | 'stateMachine' | 'state';

export type OpType =
  | 'cells.list'
  | 'codex.exec'
  | 'command_execution'
  | 'input'
  | 'llm_inference2'
  | 'sleep';

export interface NotebookRuntime {
  status?: NodeStatus;
  output?: unknown;
  error?: string | null;
  started_at?: string | null;
  finished_at?: string | null;
}

export type KeyValueField = { key: string; value: string };

export type NotebookCell =
  | {
      id: string;
      kind: 'markdown';
      title: string;
      markdown: string;
      collapsed?: boolean;
      children: NotebookCell[];
      runtime?: NotebookRuntime;
      storyNode?: StoryNode;
    }
  | {
      id: string;
      kind: 'recipe';
      title: string;
      name: string;
      fields: KeyValueField[];
      collapsed?: boolean;
      children: NotebookCell[];
      runtime?: NotebookRuntime;
      storyNode?: StoryNode;
    }
  | {
      id: string;
      kind: 'sequence';
      title: string;
      name: string;
      fields: KeyValueField[];
      collapsed?: boolean;
      children: NotebookCell[];
      runtime?: NotebookRuntime;
      storyNode?: StoryNode;
    }
  | {
      id: string;
      kind: 'op';
      title: string;
      opType: OpType;
      params: Record<string, unknown>;
      collapsed?: boolean;
      children: NotebookCell[];
      runtime?: NotebookRuntime;
      storyNode?: StoryNode;
    }
  | {
      id: string;
      kind: 'stateMachine';
      title: string;
      machineId: string;
      fields: KeyValueField[];
      collapsed?: boolean;
      children: NotebookCell[];
      runtime?: NotebookRuntime;
      storyNode?: StoryNode;
    }
  | {
      id: string;
      kind: 'state';
      title: string;
      stateId: string;
      transitions: Array<{ when: 'always' | 'expression'; expression?: string; toStateId: string }>;
      collapsed?: boolean;
      children: NotebookCell[];
      runtime?: NotebookRuntime;
      storyNode?: StoryNode;
    };

export interface FlattenedNotebookCell {
  cell: NotebookCell;
  depth: number;
  parentId: string | null;
  indexInParent: number;
}

function assertNever(value: never): never {
  throw new Error(`Unexpected value: ${String(value)}`);
}

export function newCellId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

export function flattenCells(root: NotebookCell): FlattenedNotebookCell[] {
  const out: FlattenedNotebookCell[] = [];

  const walk = (cell: NotebookCell, depth: number, parentId: string | null, indexInParent: number) => {
    out.push({ cell, depth, parentId, indexInParent });
    if (cell.collapsed) return;
    for (let i = 0; i < cell.children.length; i++) {
      walk(cell.children[i], depth + 1, cell.id, i);
    }
  };

  walk(root, 0, null, 0);
  return out;
}

export function updateCellById(root: NotebookCell, id: string, updater: (cell: NotebookCell) => NotebookCell): NotebookCell {
  if (root.id === id) return updater(root);
  if (root.children.length === 0) return root;
  return { ...root, children: root.children.map((c) => updateCellById(c, id, updater)) } as NotebookCell;
}

export function insertChild(root: NotebookCell, parentId: string, child: NotebookCell, index?: number): NotebookCell {
  return updateCellById(root, parentId, (cell) => {
    const at = typeof index === 'number' ? index : cell.children.length;
    const nextChildren = cell.children.slice();
    nextChildren.splice(at, 0, child);
    return { ...cell, children: nextChildren, collapsed: false } as NotebookCell;
  });
}

export function deleteCellById(root: NotebookCell, id: string): NotebookCell {
  if (root.id === id) return root;
  const nextChildren = root.children
    .filter((c) => c.id !== id)
    .map((c) => deleteCellById(c, id));
  return { ...root, children: nextChildren } as NotebookCell;
}

export function moveCellWithinParent(root: NotebookCell, parentId: string, fromIndex: number, toIndex: number): NotebookCell {
  return updateCellById(root, parentId, (cell) => {
    if (fromIndex === toIndex) return cell;
    if (fromIndex < 0 || toIndex < 0) return cell;
    if (fromIndex >= cell.children.length || toIndex >= cell.children.length) return cell;
    const next = cell.children.slice();
    const [item] = next.splice(fromIndex, 1);
    next.splice(toIndex, 0, item);
    return { ...cell, children: next } as NotebookCell;
  });
}

export function clearRuntime(root: NotebookCell): NotebookCell {
  const cleared = { ...root, runtime: undefined } as NotebookCell;
  if (cleared.children.length === 0) return cleared;
  return { ...cleared, children: cleared.children.map(clearRuntime) } as NotebookCell;
}

function mapStoryKindToCellKind(kind: StoryKind): NotebookCellKind {
  switch (kind) {
    case 'recipe':
      return 'recipe';
    case 'sequence':
      return 'sequence';
    case 'op':
    case 'opStep':
      return 'op';
    case 'stateMachine':
      return 'stateMachine';
    case 'state':
      return 'state';
    case 'transitionEval':
    case 'contextPatch':
    default:
      return 'op';
  }
}

export function storyToDoc(root: StoryNode | null | undefined): NotebookCell | null {
  if (!root) return null;

  const inferOpType = (title: string, input: unknown): OpType => {
    if (input && typeof input === 'object' && !Array.isArray(input)) {
      const obj = input as Record<string, unknown>;
      if ('duration' in obj) return 'sleep';
      if ('default_provider' in obj || 'default_model' in obj) return 'llm_inference2';
      if ('run' in obj || 'working_directory' in obj) return 'command_execution';
      if ('form' in obj) return 'input';
      if ('worktree_path' in obj || 'cell_relative_path' in obj || 'sessionId' in obj) return 'codex.exec';
      if (Object.keys(obj).length === 0) return 'cells.list';
    }
    const t = title.toLowerCase();
    if (t.includes('sleep')) return 'sleep';
    if (t.includes('codex')) return 'codex.exec';
    if (t.includes('command') || t.includes('exec')) return 'command_execution';
    if (t.includes('input')) return 'input';
    if (t.includes('cells') && t.includes('list')) return 'cells.list';
    if (t.includes('llm') || t.includes('inference')) return 'llm_inference2';
    return 'command_execution';
  };

  const asParams = (value: unknown): Record<string, unknown> => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
    return value as Record<string, unknown>;
  };

  const asFields = (value: unknown): KeyValueField[] => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return [];
    return Object.entries(value as Record<string, unknown>).map(([key, v]) => ({
      key,
      value:
        typeof v === 'string'
          ? v
          : (() => {
              try {
                return JSON.stringify(v);
              } catch {
                return String(v);
              }
            })(),
    }));
  };

  const walk = (node: StoryNode): NotebookCell => {
    const id = `${node.path.join('/')}|attempt:${node.attempt ?? 1}`;
    const kind = mapStoryKindToCellKind(node.kind);

    const common = {
      id,
      title: node.title,
      children: [] as NotebookCell[],
      collapsed: false,
      runtime: undefined,
      storyNode: node,
    };

    const children: NotebookCell[] = [];
    if (node.prior_attempts && node.prior_attempts.length > 0) {
      for (const prior of node.prior_attempts) {
        children.push(walk({ ...prior, title: `${prior.title} (prior attempt ${prior.attempt ?? 1})` }));
      }
    }
    for (const child of node.children ?? []) children.push(walk(child));

    if (kind === 'recipe') {
      return { ...common, kind, name: node.title, fields: asFields(node.input ?? null), children } as NotebookCell;
    }
    if (kind === 'sequence') {
      return { ...common, kind, name: node.title, fields: asFields(node.input ?? null), children } as NotebookCell;
    }
    if (kind === 'stateMachine') {
      return { ...common, kind, machineId: node.title, fields: asFields(node.input ?? null), children } as NotebookCell;
    }
    if (kind === 'state') {
      return { ...common, kind, stateId: node.title, transitions: [], children } as NotebookCell;
    }
    if (kind === 'op') {
      return {
        ...common,
        kind,
        opType: inferOpType(node.title, node.input ?? null),
        params: asParams(node.input ?? null),
        children,
      } as NotebookCell;
    }

    return { ...common, kind: 'op', opType: 'command_execution', params: {}, children } as NotebookCell;
  };

  return walk(root);
}

export function defaultAuthoringDoc(): NotebookCell {
  const op1: NotebookCell = {
    id: newCellId(),
    kind: 'op',
    title: 'op: load_context',
    opType: 'cells.list',
    params: {},
    children: [],
  };

  const op2: NotebookCell = {
    id: newCellId(),
    kind: 'op',
    title: 'op: llm_complete',
    opType: 'llm_inference2',
    params: { default_provider: 'openai', default_model: 'gpt-4.1', temperature: 0.2, prompt: 'Summarize the diff.' },
    children: [],
  };

  const seq: NotebookCell = {
    id: newCellId(),
    kind: 'sequence',
    title: 'sequence: main',
    name: 'main',
    fields: [],
    collapsed: false,
    children: [op1, op2],
  };

  return {
    id: newCellId(),
    kind: 'recipe',
    title: 'recipe: new_recipe',
    name: 'new_recipe',
    fields: [],
    collapsed: false,
    children: [
      {
        id: newCellId(),
        kind: 'markdown',
        title: 'Notes',
        markdown: 'Describe what this recipe does.\n',
        children: [],
      },
      seq,
    ],
  };
}

export function docToRecipeObject(root: NotebookCell): unknown {
  const fieldsToObject = (fields: KeyValueField[]): Record<string, unknown> => {
    const out: Record<string, unknown> = {};
    for (const f of fields) {
      if (!f.key.trim()) continue;
      out[f.key] = f.value;
    }
    return out;
  };

  const serialize = (cell: NotebookCell): unknown => {
    switch (cell.kind) {
      case 'markdown':
        return { kind: 'markdown', title: cell.title, markdown: cell.markdown };
      case 'recipe':
        return { kind: 'recipe', name: cell.name, fields: fieldsToObject(cell.fields), children: cell.children.map(serialize) };
      case 'sequence':
        return { kind: 'sequence', name: cell.name, fields: fieldsToObject(cell.fields), steps: cell.children.map(serialize) };
      case 'op':
        return { kind: 'op', opType: cell.opType, params: cell.params };
      case 'stateMachine':
        return { kind: 'stateMachine', id: cell.machineId, fields: fieldsToObject(cell.fields), states: cell.children.map(serialize) };
      case 'state':
        return {
          kind: 'state',
          id: cell.stateId,
          transitions: cell.transitions,
          body: cell.children.map(serialize),
        };
    }
    return assertNever(cell);
  };

  return serialize(root);
}
