import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Divider,
  Empty,
  Input,
  Layout,
  List,
  Segmented,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import dayjs from 'dayjs';
import type { NotebookBackendMode, NotebookRecipeSummary, WorkflowStoryResponse } from '../backend/types';
import { createNotebookBackend } from '../backend/backend';
import type { NotebookCell, NotebookCellKind, NotebookMode } from '../model/notebookModel';
import {
  clearRuntime,
  defaultAuthoringDoc,
  deleteCellById,
  docToRecipeObject,
  flattenCells,
  insertChild,
  moveCellWithinParent,
  newCellId,
  storyToDoc,
  updateCellById,
} from '../model/notebookModel';
import NotebookCellCard from '../components/NotebookCellCard';
import NotebookInspector from '../components/NotebookInspector';
import { statusTagColor } from '../ui/status';

const { Sider, Content } = Layout;
const { Text, Title } = Typography;

function formatRelative(ts?: string | null): string {
  if (!ts) return '-';
  return dayjs(ts).format('YYYY-MM-DD HH:mm');
}

function findCell(root: NotebookCell, id: string): NotebookCell | null {
  if (root.id === id) return root;
  for (const child of root.children) {
    const found = findCell(child, id);
    if (found) return found;
  }
  return null;
}

function toPrettyJson(value: unknown): string {
  if (value === null || value === undefined) return '';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

type ExecContext = {
  vars: Record<string, unknown>;
};

export type RecipeNotebookInitialState = {
  backendMode?: NotebookBackendMode;
  mode?: NotebookMode;
  jobId?: string;
  autoLoadJob?: boolean;
};

function isStateCell(cell: NotebookCell): cell is Extract<NotebookCell, { kind: 'state' }> {
  return cell.kind === 'state';
}

function getString(value: unknown, fallback = ''): string {
  if (typeof value === 'string') return value;
  if (value === null || value === undefined) return fallback;
  return String(value);
}

function renderTemplate(template: string, ctx: ExecContext): string {
  return template.replace(/\{\{\s*([^}]+)\s*\}\}/g, (_m, expr) => {
    const key = String(expr ?? '').trim();
    const v = ctx.vars[key];
    if (v === null || v === undefined) return '';
    return typeof v === 'string' ? v : JSON.stringify(v);
  });
}

function nowIso(): string {
  return new Date().toISOString();
}

function executeCellRecursive(cell: NotebookCell, ctx: ExecContext): { cell: NotebookCell; ctx: ExecContext } {
  const started_at = nowIso();

  const runChildren = (children: NotebookCell[]) => {
    let nextCtx = ctx;
    const nextChildren: NotebookCell[] = [];
    const outputs: Array<{ id: string; title: string; output: unknown }> = [];
    for (const child of children) {
      const res = executeCellRecursive(child, nextCtx);
      nextChildren.push(res.cell);
      nextCtx = res.ctx;
      outputs.push({ id: res.cell.id, title: res.cell.title, output: res.cell.runtime?.output ?? null });
    }
    return { nextChildren, nextCtx, outputs };
  };

  try {
    if (cell.kind === 'markdown') {
      return {
        cell: { ...cell, runtime: { status: 'succeeded', output: null, started_at, finished_at: nowIso(), error: null } },
        ctx,
      };
    }

    if (cell.kind === 'op') {
      const params = cell.params ?? {};
      const opType = cell.opType;
      let output: unknown = null;

      if (opType === 'cells.list') {
        output = { cells: [] };
      } else if (opType === 'sleep') {
        const duration = getString(params.duration, '5s');
        const start_time = nowIso();
        const end_time = nowIso();
        output = {
          start_time,
          end_time,
          actual_duration: duration,
          completed: true,
          interrupted: false,
          error_message: '',
        };
      } else if (opType === 'command_execution') {
        const run = renderTemplate(getString(params.run, ''), ctx);
        output = {
          stdout: `stub: ran "${run}"`,
          stderr: '',
          exit_code: 0,
          success: true,
          timed_out: false,
          error_message: '',
        };
      } else if (opType === 'codex.exec') {
        const prompt = renderTemplate(getString(params.prompt, ''), ctx);
        const model = getString(params.model, 'gpt-5-codex');
        const sessionId = getString(params.sessionId, `sess_${Date.now()}`);
        output = {
          status: 'completed',
          sessionId,
          assistantSummary: `stub: completed codex.exec using ${model} (prompt chars=${prompt.length})`,
          incompleteReason: '',
          incompleteCategory: '',
          pendingDependencies: [],
        };
      } else if (opType === 'input') {
        const form = (params.form && typeof params.form === 'object' ? (params.form as any) : {}) as any;
        const question = getString(form.question, 'Question');
        output = {
          response: { question, answer: 'stub answer' },
          user_id: 'user_stub',
          metadata: { source: 'prototype' },
        };
      } else if (opType === 'llm_inference2') {
        const prompt = renderTemplate(getString(params.prompt, ''), ctx);
        const system_prompt = renderTemplate(getString(params.system_prompt, ''), ctx);
        const model = getString(params.default_model, 'gpt-4.1');
        const temperature = typeof params.temperature === 'number' ? params.temperature : Number(params.temperature ?? 0.2);
        const response = `stub llm_inference2 (${model}, temp=${temperature})\n${system_prompt ? `SYSTEM: ${system_prompt}\n` : ''}${prompt}`.slice(
          0,
          1200,
        );
        ctx.vars = { ...ctx.vars, 'llm_inference2.response': response };
        output = {
          response,
          model,
          finish_reason: 'stop',
          usage: { prompt_tokens: 100, completion_tokens: 200, total_tokens: 300 },
          tool_calls: [],
          tool_results: [],
          tool_execution_errors: [],
          tool_rounds_used: 0,
          files_written: [],
          files_read: [],
          files_deleted: [],
          execution_time_ms: 123,
          provider_metadata: {},
          telemetry: { prompt_tokens: 100, completion_tokens: 200, total_tokens: 300 },
        };
      }

      return {
        cell: {
          ...cell,
          runtime: { status: 'succeeded', output, started_at, finished_at: nowIso(), error: null },
        },
        ctx,
      };
    }

    if (cell.kind === 'state') {
      const { nextChildren, nextCtx, outputs } = runChildren(cell.children);
      const decision = cell.transitions.find((t) => t.when === 'always') ?? cell.transitions[0] ?? null;
      const output = { entered: cell.stateId, decision: decision ? { to: decision.toStateId, when: decision.when } : null, outputs };

      return {
        cell: {
          ...cell,
          children: nextChildren,
          runtime: { status: 'succeeded', output, started_at, finished_at: nowIso(), error: null },
        },
        ctx: nextCtx,
      };
    }

    if (cell.kind === 'stateMachine') {
      let nextCtx = ctx;
      let nextChildren = cell.children.slice();
      let currentStateId: string | null = null;
      let steps = 0;

      const stateCells = nextChildren.filter(isStateCell);
      const stateById = new Map(stateCells.map((s) => [s.stateId, s] as const));
      const firstState = stateCells[0];
      currentStateId = firstState?.stateId ?? null;

      while (currentStateId && steps < 10) {
        steps += 1;
        const stateCell = stateById.get(currentStateId);
        if (!stateCell || stateCell.kind !== 'state') break;

        const res = executeCellRecursive(stateCell, nextCtx);
        nextCtx = res.ctx;
        const nextStateCell = res.cell as Extract<NotebookCell, { kind: 'state' }>;
        stateById.set(currentStateId, nextStateCell);

        const decision = nextStateCell.transitions.find((t) => t.when === 'always') ?? nextStateCell.transitions[0] ?? null;
        const nextStateId = decision?.toStateId?.trim() || null;
        if (!nextStateId) break;
        if (!stateById.has(nextStateId)) break;
        if (nextStateId === currentStateId) break;
        currentStateId = nextStateId;
      }

      nextChildren = nextChildren.map((c) => (isStateCell(c) ? (stateById.get(c.stateId) ?? c) : c));
      const output = { machineId: cell.machineId, finalStateId: currentStateId, steps };

      return {
        cell: { ...cell, children: nextChildren, runtime: { status: 'succeeded', output, started_at, finished_at: nowIso(), error: null } },
        ctx: nextCtx,
      };
    }

    if (cell.kind === 'sequence' || cell.kind === 'recipe') {
      const { nextChildren, nextCtx, outputs } = runChildren(cell.children);
      const output =
        cell.kind === 'sequence'
          ? { sequence: cell.name, outputs }
          : { recipe: (cell as Extract<NotebookCell, { kind: 'recipe' }>).name, outputs };

      return {
        cell: { ...cell, children: nextChildren, runtime: { status: 'succeeded', output, started_at, finished_at: nowIso(), error: null } } as NotebookCell,
        ctx: nextCtx,
      };
    }

    return { cell, ctx };
  } catch (e) {
    const error = e instanceof Error ? e.message : 'Execution failed';
    return {
      cell: { ...cell, runtime: { status: 'failed', output: null, started_at, finished_at: nowIso(), error } } as NotebookCell,
      ctx,
    };
  }
}

function executeCellById(root: NotebookCell, id: string, ctx: ExecContext): { root: NotebookCell; ctx: ExecContext } {
  if (root.id === id) {
    const res = executeCellRecursive(root, ctx);
    return { root: res.cell, ctx: res.ctx };
  }
  if (root.children.length === 0) return { root, ctx };
  let nextCtx = ctx;
  const nextChildren = root.children.map((c) => {
    const res = executeCellById(c, id, nextCtx);
    nextCtx = res.ctx;
    return res.root;
  });
  return { root: { ...root, children: nextChildren } as NotebookCell, ctx: nextCtx };
}

function collectOpIds(root: NotebookCell): string[] {
  const out: string[] = [];
  const walk = (c: NotebookCell) => {
    if (c.kind === 'op') out.push(c.id);
    for (const ch of c.children) walk(ch);
  };
  walk(root);
  return out;
}

function createDefaultChild(kind: NotebookCellKind): NotebookCell {
  switch (kind) {
    case 'markdown':
      return { id: newCellId(), kind: 'markdown', title: 'Notes', markdown: 'Notes...\n', children: [] };
    case 'sequence':
      return { id: newCellId(), kind: 'sequence', title: 'sequence: new_sequence', name: 'new_sequence', fields: [], children: [] };
    case 'op':
      return { id: newCellId(), kind: 'op', title: 'op: new_op', opType: 'command_execution', params: { run: 'echo hello' }, children: [] };
    case 'stateMachine':
      return { id: newCellId(), kind: 'stateMachine', title: 'stateMachine: sm1', machineId: 'sm1', fields: [], children: [] };
    case 'state':
      return { id: newCellId(), kind: 'state', title: 'state: start', stateId: 'start', transitions: [], children: [] };
    case 'recipe':
    default:
      return { id: newCellId(), kind: 'recipe', title: 'recipe: new_recipe', name: 'new_recipe', fields: [], children: [] };
  }
}

export default function RecipeNotebookPage(props: { projectId: string; initial?: RecipeNotebookInitialState }): JSX.Element {
  const [backendMode, setBackendMode] = useState<NotebookBackendMode>(props.initial?.backendMode ?? 'stub');
  const backend = useMemo(() => createNotebookBackend(backendMode), [backendMode]);

  const [mode, setMode] = useState<NotebookMode>(props.initial?.mode ?? 'monitor');
  const [recipes, setRecipes] = useState<NotebookRecipeSummary[]>([]);
  const [selectedRecipe, setSelectedRecipe] = useState<string | null>(null);
  const [recipeContent, setRecipeContent] = useState<string>('');

  const [jobIdInput, setJobIdInput] = useState<string>(props.initial?.jobId ?? 'j_demo_notebook_001');
  const [story, setStory] = useState<WorkflowStoryResponse | null>(null);
  const [doc, setDoc] = useState<NotebookCell | null>(null);
  const [selectedCellId, setSelectedCellId] = useState<string | null>(null);
  const selectedCell = useMemo(
    () => (doc && selectedCellId ? findCell(doc, selectedCellId) : null),
    [doc, selectedCellId],
  );

  const [loadingRecipes, setLoadingRecipes] = useState(false);
  const [loadingStory, setLoadingStory] = useState(false);
  const [execCtx, setExecCtx] = useState<ExecContext>({ vars: {} });
  const [runCursor, setRunCursor] = useState(0);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [refreshIntervalMs] = useState(2500);
  const didAutoLoadRef = useRef(false);

  useEffect(() => {
    const initial = props.initial;
    if (!initial) return;
    if (initial.backendMode) setBackendMode(initial.backendMode);
    if (initial.mode) setMode(initial.mode);
    if (initial.jobId) setJobIdInput(initial.jobId);
  }, [props.initial]);

  useEffect(() => {
    setLoadingRecipes(true);
    backend
      .listRecipes(props.projectId)
      .then((r) => {
        setRecipes(r);
        if (!selectedRecipe && r.length > 0) setSelectedRecipe(r[0].name);
      })
      .catch((e) => {
        console.error(e);
        message.error(e instanceof Error ? e.message : 'Failed to load recipes');
      })
      .finally(() => setLoadingRecipes(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [backendMode, props.projectId]);

  useEffect(() => {
    if (!selectedRecipe) return;
    backend
      .getRecipe(props.projectId, selectedRecipe)
      .then((r) => setRecipeContent(r.content))
      .catch((e) => {
        console.error(e);
        setRecipeContent('');
        message.error(e instanceof Error ? e.message : 'Failed to load recipe');
      });
  }, [backend, props.projectId, selectedRecipe]);

  useEffect(() => {
    if (mode === 'build' || mode === 'run') {
      const nextDoc = defaultAuthoringDoc();
      setDoc(nextDoc);
      setSelectedCellId(nextDoc.id);
      setStory(null);
      setExecCtx({ vars: {} });
      setRunCursor(0);
      return;
    }
    setDoc(null);
    setSelectedCellId(null);
    setExecCtx({ vars: {} });
    setRunCursor(0);
  }, [mode]);

  const refreshStory = async (jobId: string) => {
    setLoadingStory(true);
    try {
      const next = await backend.getJobStory(props.projectId, jobId);
      setStory(next);
      const nextDoc = storyToDoc(next.root);
      setDoc(nextDoc);
      setSelectedCellId(nextDoc?.id ?? null);
    } catch (e) {
      console.error(e);
      setStory(null);
      setDoc(null);
      setSelectedCellId(null);
      message.error(e instanceof Error ? e.message : 'Failed to load story');
    } finally {
      setLoadingStory(false);
    }
  };

  useEffect(() => {
    const shouldAutoLoad = props.initial?.autoLoadJob ?? true;
    if (!shouldAutoLoad) return;
    if (mode !== 'monitor') return;
    if (!props.initial?.jobId) return;
    if (didAutoLoadRef.current) return;
    didAutoLoadRef.current = true;
    refreshStory(props.initial.jobId).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, props.initial?.jobId]);

  useEffect(() => {
    if (mode !== 'monitor') return;
    if (!autoRefresh) return;
    if (!story || story.status !== 'running') return;

    const t = window.setTimeout(() => {
      refreshStory(jobIdInput).catch(() => {});
    }, refreshIntervalMs);
    return () => window.clearTimeout(t);
  }, [autoRefresh, refreshIntervalMs, jobIdInput, mode, story?.status]);

  const saveDraft = async () => {
    if (!selectedRecipe) return;
    try {
      if (mode === 'build' || mode === 'run') {
        if (!doc) throw new Error('No recipe doc to save');
        await backend.upsertDraftRecipe(props.projectId, selectedRecipe, toPrettyJson(docToRecipeObject(doc)) || '');
      } else {
        await backend.upsertDraftRecipe(props.projectId, selectedRecipe, recipeContent);
      }
      message.success('Saved draft (stub)');
    } catch (e) {
      console.error(e);
      message.error(e instanceof Error ? e.message : 'Failed to save draft');
    }
  };

  const flattened = useMemo(() => (doc ? flattenCells(doc) : []), [doc]);

  const header = (
    <Card size="small" style={{ marginBottom: 12 }}>
      <Space wrap style={{ width: '100%', justifyContent: 'space-between' }}>
        <Space wrap>
          <Title level={4} style={{ margin: 0 }}>
            Recipe Notebook (Mock)
          </Title>
          <Tag color="default">{backend.mode.toUpperCase()}</Tag>
          <Segmented
            value={mode}
            options={[
              { label: 'Monitor', value: 'monitor' },
              { label: 'Run step-by-step', value: 'run' },
              { label: 'Build', value: 'build' },
            ]}
            onChange={(v) => setMode(v as NotebookMode)}
          />
        </Space>

        <Space wrap>
          <Text type="secondary">Backend:</Text>
          <Select
            value={backendMode}
            style={{ width: 160 }}
            options={[
              { label: 'Stub (Recommended)', value: 'stub' },
              { label: 'Real (read-only)', value: 'real' },
            ]}
            onChange={(v) => setBackendMode(v)}
          />
        </Space>
      </Space>
    </Card>
  );

  const recipePanelTitle = mode === 'build' || mode === 'run' ? 'Recipe Object (preview)' : 'Recipe Content (preview)';

  const left = (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Card size="small" title="Recipes">
        <Select
          value={selectedRecipe ?? undefined}
          style={{ width: '100%' }}
          placeholder="Select a recipe"
          loading={loadingRecipes}
          options={recipes.map((r) => ({
            label: `${r.name} (${r.status})`,
            value: r.name,
          }))}
          onChange={(v) => setSelectedRecipe(v)}
        />
        <Divider style={{ margin: '12px 0' }} />
        <List
          size="small"
          dataSource={recipes}
          loading={loadingRecipes}
          renderItem={(r) => (
            <List.Item
              style={{ cursor: 'pointer' }}
              onClick={() => setSelectedRecipe(r.name)}
              actions={[
                <Tag key="status" color={r.status === 'published' ? 'green' : 'default'}>
                  {r.status.toUpperCase()}
                </Tag>,
              ]}
            >
              <Space direction="vertical" size={0}>
                <Text strong>{r.name}</Text>
                <Text type="secondary">{formatRelative(r.updated_at)}</Text>
              </Space>
            </List.Item>
          )}
        />
      </Card>

      <Card size="small" title={recipePanelTitle}>
        {mode === 'build' || mode === 'run' ? (
          doc ? (
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{toPrettyJson(docToRecipeObject(doc))}</pre>
          ) : (
            <Empty description="No recipe doc" />
          )
        ) : selectedRecipe ? (
          <Input.TextArea value={recipeContent} onChange={(e) => setRecipeContent(e.target.value)} autoSize={{ minRows: 8, maxRows: 18 }} />
        ) : (
          <Empty description="Select a recipe" />
        )}
        <Divider style={{ margin: '12px 0' }} />
        <Space wrap>
          <Button onClick={saveDraft} disabled={!selectedRecipe}>
            Save Draft
          </Button>
          <Text type="secondary">Authoring is stub-only today.</Text>
        </Space>
      </Card>
    </Space>
  );

  const monitorControls = (
    <Card size="small" style={{ marginBottom: 12 }}>
      <Space wrap>
        <Input
          value={jobIdInput}
          onChange={(e) => setJobIdInput(e.target.value)}
          style={{ width: 320 }}
          placeholder="Job/Workflow ID to monitor"
        />
        <Button loading={loadingStory} onClick={() => refreshStory(jobIdInput)}>
          Load Story
        </Button>
        {story?.status ? <Tag color={statusTagColor(story.status)}>{story.status.toUpperCase()}</Tag> : null}
        {story?.status === 'running' ? (
          <Space size={8}>
            <Tag color="blue">MORE NODES EXPECTED</Tag>
            <Space size={6}>
              <Text type="secondary">Auto-refresh</Text>
              <Switch size="small" checked={autoRefresh} onChange={setAutoRefresh} />
            </Space>
          </Space>
        ) : null}
      </Space>
      {story?.status === 'running' ? (
        <Text type="secondary">
          This job is still running; the story is append-only. New nodes may appear as execution progresses.
        </Text>
      ) : null}
    </Card>
  );

  const runControls = (
    <Card size="small" style={{ marginBottom: 12 }}>
      <Space wrap>
        <Button
          onClick={() => {
            if (!doc) return;
            setDoc(clearRuntime(doc));
            setExecCtx({ vars: {} });
            setRunCursor(0);
            message.success('Reset run state');
          }}
          disabled={!doc}
        >
          Reset
        </Button>
        <Button
          type="primary"
          onClick={() => {
            if (!doc) return;
            const opIds = collectOpIds(doc);
            const nextId = opIds[runCursor];
            if (!nextId) {
              message.info('No more ops to run');
              return;
            }
            const res = executeCellById(doc, nextId, { vars: { ...execCtx.vars } });
            setDoc(res.root);
            setExecCtx(res.ctx);
            setRunCursor((c) => c + 1);
          }}
          disabled={!doc}
        >
          Run Next Op
        </Button>
        <Button
          onClick={() => {
            if (!doc) return;
            let nextDoc = doc;
            let nextCtx = { vars: { ...execCtx.vars } };
            for (const opId of collectOpIds(doc)) {
              const res = executeCellById(nextDoc, opId, nextCtx);
              nextDoc = res.root;
              nextCtx = res.ctx;
            }
            setDoc(nextDoc);
            setExecCtx(nextCtx);
            setRunCursor(collectOpIds(nextDoc).length);
          }}
          disabled={!doc}
        >
          Run All Ops
        </Button>
        <Tag color="default">cursor {runCursor}</Tag>
      </Space>
    </Card>
  );

  const buildControls = (
    <Card size="small" style={{ marginBottom: 12 }}>
      <Space wrap>
        <Text type="secondary">Use per-cell</Text>
        <Tag color="default">Add ▸</Tag>
        <Text type="secondary">to build nested structures (sequence → ops, stateMachine → states, etc).</Text>
      </Space>
    </Card>
  );

  const controls = mode === 'monitor' ? monitorControls : mode === 'run' ? runControls : buildControls;
  const canEdit = mode === 'build' || mode === 'run';

  return (
    <div style={{ height: '100%' }}>
      {header}

      <Layout style={{ height: 'calc(100% - 64px)', background: 'transparent' }}>
        <Sider width={360} theme="light" style={{ paddingRight: 12, background: 'transparent', overflow: 'auto' }}>
          {left}
        </Sider>

        <Content style={{ overflow: 'auto', paddingRight: 12 }}>
          {controls}

          {mode === 'monitor' && backend.mode === 'real' ? (
            <Alert
              style={{ marginBottom: 12 }}
              type="info"
              showIcon
              message="Real backend is read-only in this mock"
              description="Monitoring uses the job story endpoint. Authoring and execution are local-only in this prototype."
            />
          ) : null}

          {!doc ? (
            <Card size="small">
              <Empty description={mode === 'monitor' ? 'Load a story to populate cells' : 'No notebook yet'} />
            </Card>
          ) : (
            flattened.map(({ cell, depth, parentId, indexInParent }) => (
              <NotebookCellCard
                key={cell.id}
                cell={cell}
                depth={depth}
                selected={cell.id === selectedCellId}
                editable={canEdit}
                runnable={canEdit}
                onSelect={() => setSelectedCellId(cell.id)}
                onToggleCollapse={() => {
                  setDoc((prev) => (prev ? updateCellById(prev, cell.id, (c) => ({ ...c, collapsed: !c.collapsed })) : prev));
                }}
                onRun={() => {
                  setDoc((prev) => {
                    if (!prev) return prev;
                    const res = executeCellById(prev, cell.id, { vars: { ...execCtx.vars } });
                    setExecCtx(res.ctx);
                    return res.root;
                  });
                }}
                onAddChild={(kind) => {
                  setDoc((prev) => {
                    if (!prev) return prev;
                    const child = createDefaultChild(kind);
                    return insertChild(prev, cell.id, child);
                  });
                }}
                onUpdate={(next) => {
                  setDoc((prev) => (prev ? updateCellById(prev, cell.id, () => next) : prev));
                }}
                onMoveUp={
                  canEdit && parentId
                    ? () => setDoc((prev) => (prev ? moveCellWithinParent(prev, parentId, indexInParent, indexInParent - 1) : prev))
                    : undefined
                }
                onMoveDown={
                  canEdit && parentId
                    ? () => setDoc((prev) => (prev ? moveCellWithinParent(prev, parentId, indexInParent, indexInParent + 1) : prev))
                    : undefined
                }
                onDelete={
                  canEdit && parentId
                    ? () => {
                        setDoc((prev) => (prev ? deleteCellById(prev, cell.id) : prev));
                        setSelectedCellId((cur) => (cur === cell.id ? null : cur));
                      }
                    : undefined
                }
              />
            ))
          )}
        </Content>

        <Sider width={420} theme="light" style={{ background: 'transparent', overflow: 'auto' }}>
          <NotebookInspector cell={selectedCell} />
          {mode === 'monitor' && story ? (
            <Card size="small" title="Run Summary" style={{ marginTop: 12 }}>
              <Descriptions size="small" column={1} bordered>
                <Descriptions.Item label="Job ID">
                  <Text code>{story.job_id}</Text>
                </Descriptions.Item>
                <Descriptions.Item label="Status">
                  <Tag color={statusTagColor(story.status)}>{story.status.toUpperCase()}</Tag>
                </Descriptions.Item>
              </Descriptions>
            </Card>
          ) : null}
          {mode !== 'monitor' && doc ? (
            <Card size="small" title="Execution Context" style={{ marginTop: 12 }}>
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{toPrettyJson(execCtx.vars) || '-'}</pre>
            </Card>
          ) : null}
        </Sider>
      </Layout>
    </div>
  );
}
