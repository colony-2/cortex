import type {
  NotebookBackend,
  NotebookRecipeSummary,
  NotebookRecipeWithContent,
  WorkflowStoryResponse,
  StoryNode,
  StoryKind,
  NodeStatus,
} from './types';

function tsOffset(seconds: number): string {
  return new Date(Date.now() + seconds * 1000).toISOString();
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function statusForIndex(i: number, cursor: number): NodeStatus {
  if (i < cursor) return 'succeeded';
  if (i === cursor) return 'running';
  return 'pending';
}

function buildStubStory(cursor: number): WorkflowStoryResponse {
  const jobId = 'j_demo_notebook_001';
  const startedAt = tsOffset(-120);

  const opNodes: Array<{ title: string; kind: StoryKind; path: string[]; input?: unknown; output?: unknown }> = [
    {
      title: 'Load context',
      kind: 'op',
      path: ['root', 'sequence:main', 'op:load_context'],
      input: { from: 'ticket', fields: ['title', 'description'] },
      output: { ok: true, context_keys: ['ticket.title', 'ticket.description'] },
    },
    {
      title: 'Render prompt',
      kind: 'op',
      path: ['root', 'sequence:main', 'op:render_prompt'],
      input: { template: 'Summarize: {{ticket.description}}' },
      output: { prompt: 'Summarize: ...' },
    },
    {
      title: 'LLM call (retry demo)',
      kind: 'op',
      path: ['root', 'sequence:main', 'op:llm_call'],
      input: { model: 'gpt-4.1-mini', temperature: 0.2 },
      output: { completion: 'Here is a structured summary...' },
    },
    {
      title: 'Write artifacts',
      kind: 'op',
      path: ['root', 'sequence:main', 'op:write_artifacts'],
      input: { files: ['summary.md', 'plan.json'] },
      output: { written: 2 },
    },
  ];

  const cursorClamped = Math.max(0, Math.min(cursor, opNodes.length));

  const opChildren: StoryNode[] = opNodes.map((op, idx) => {
    const status = statusForIndex(idx, cursorClamped);
    const attempt = op.title.includes('retry') && status !== 'pending' ? 2 : 1;

    const base: StoryNode = {
      kind: op.kind,
      title: op.title,
      status,
      started_at: status === 'pending' ? null : tsOffset(-110 + idx * 10),
      finished_at: status === 'succeeded' ? tsOffset(-105 + idx * 10) : null,
      path: op.path,
      invoke_seq: 1,
      input: status === 'pending' ? null : op.input ?? null,
      output: status === 'succeeded' ? (op.output ?? null) : null,
      artifact_keys:
        op.title === 'Write artifacts' && status === 'succeeded'
          ? [
              { jobId, taskOrdinal: 12, name: 'summary.md', sizeBytes: 1421 },
              { jobId, taskOrdinal: 12, name: 'plan.json', sizeBytes: 532 },
            ]
          : [],
      children: [],
      attempt,
      prior_attempts: [],
      task_ordinal: status === 'pending' ? null : 10 + idx,
      restart_from_ordinal: status === 'pending' ? null : 10 + idx,
    };

    if (op.title.includes('retry') && status !== 'pending') {
      const attempt1: StoryNode = {
        ...clone(base),
        status: 'failed',
        attempt: 1,
        started_at: tsOffset(-90),
        finished_at: tsOffset(-88),
        output: { error: 'Rate limited (429)', retryable: true },
        artifact_keys: [{ jobId, taskOrdinal: 11, name: 'stderr.log', sizeBytes: 2048 }],
        task_ordinal: 11,
        restart_from_ordinal: 11,
      };
      base.prior_attempts = [attempt1];
    }

    return base;
  });

  const sequenceNode: StoryNode = {
    kind: 'sequence',
    title: 'main',
    status: cursorClamped >= opNodes.length ? 'succeeded' : 'running',
    started_at: startedAt,
    finished_at: cursorClamped >= opNodes.length ? tsOffset(-10) : null,
    path: ['root', 'sequence:main'],
    invoke_seq: 1,
    input: { entrypoint: true },
    output: cursorClamped >= opNodes.length ? { ok: true } : null,
    artifact_keys: [],
    children: opChildren,
    attempt: 1,
    prior_attempts: [],
  };

  const root: StoryNode = {
    kind: 'recipe',
    title: 'demo/notebook_recipe',
    status: cursorClamped >= opNodes.length ? 'succeeded' : 'running',
    started_at: startedAt,
    finished_at: cursorClamped >= opNodes.length ? tsOffset(-5) : null,
    path: ['root'],
    invoke_seq: 1,
    input: { ticket_id: 't_demo_123' },
    output: cursorClamped >= opNodes.length ? { result: 'done' } : null,
    artifact_keys: [],
    children: [sequenceNode],
    attempt: 1,
    prior_attempts: [],
  };

  return {
    job_id: jobId,
    invocation_sequence: 1,
    recipe: { name: 'demo/notebook_recipe', ref: 'stub' },
    status: cursorClamped >= opNodes.length ? 'completed' : 'running',
    started_at: startedAt,
    finished_at: cursorClamped >= opNodes.length ? tsOffset(-5) : null,
    root,
  };
}

export function createStubBackend(): NotebookBackend {
  const recipes: NotebookRecipeSummary[] = [
    {
      name: 'demo/notebook_recipe',
      status: 'published',
      published_ref: 'v1',
      latest_ref: 'v1',
      updated_at: tsOffset(-3600),
    },
    {
      name: 'demo/state_machine_example',
      status: 'unpublished',
      published_ref: null,
      latest_ref: 'draft',
      updated_at: tsOffset(-7200),
    },
  ];

  const recipeContent: Record<string, NotebookRecipeWithContent> = {
    'demo/notebook_recipe': {
      name: 'demo/notebook_recipe',
      ref: 'v1',
      status: 'published',
      updated_at: tsOffset(-3600),
      content: `# demo/notebook_recipe (stub)\n\nThis is placeholder content.\n\n- sequence: main\n  - op: load_context\n  - op: render_prompt\n  - op: llm_call (with retry)\n  - op: write_artifacts\n`,
    },
    'demo/state_machine_example': {
      name: 'demo/state_machine_example',
      ref: 'draft',
      status: 'unpublished',
      updated_at: tsOffset(-7200),
      content: `# demo/state_machine_example (stub)\n\nstateMachine:\n  states:\n    - id: start\n    - id: done\n`,
    },
  };

  let activeJobId: string | null = null;
  let cursor = 0;

  return {
    mode: 'stub',
    async listRecipes(): Promise<NotebookRecipeSummary[]> {
      return clone(recipes);
    },
    async getRecipe(_projectId: string, recipeName: string): Promise<NotebookRecipeWithContent> {
      const item = recipeContent[recipeName];
      if (!item) throw new Error(`Recipe not found (stub): ${recipeName}`);
      return clone(item);
    },
    async getJobStory(): Promise<WorkflowStoryResponse> {
      return clone(buildStubStory(cursor));
    },
    async startRun(_projectId: string, recipeName: string): Promise<{ jobId: string }> {
      if (!recipeContent[recipeName]) throw new Error(`Recipe not found (stub): ${recipeName}`);
      activeJobId = 'j_demo_notebook_001';
      cursor = 0;
      return { jobId: activeJobId };
    },
    async stepRun(): Promise<WorkflowStoryResponse> {
      if (!activeJobId) throw new Error('No active run (stub). Click Start Run first.');
      cursor += 1;
      return clone(buildStubStory(cursor));
    },
    async cancelRun(): Promise<void> {
      activeJobId = null;
      cursor = 0;
    },
    async upsertDraftRecipe(_projectId: string, recipeName: string, content: string): Promise<void> {
      recipeContent[recipeName] = {
        name: recipeName,
        ref: 'draft',
        status: 'unpublished',
        updated_at: new Date().toISOString(),
        content,
      };
      if (!recipes.some((r) => r.name === recipeName)) {
        recipes.unshift({
          name: recipeName,
          status: 'unpublished',
          published_ref: null,
          latest_ref: 'draft',
          updated_at: new Date().toISOString(),
        });
      }
    },
  };
}
