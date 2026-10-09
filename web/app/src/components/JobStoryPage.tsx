import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  List,
  Modal,
  Radio,
  Select,
  Space,
  Spin,
  Switch,
  Tabs,
  Tag,
  Tree,
  Typography,
  message,
} from 'antd';
import type { DataNode } from 'antd/es/tree';
import {
  ArrowLeftOutlined,
  DownloadOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import ReactJson from 'react-json-view';
import { API_BASE, inputActivityService, type UserInputDetails, type FormResponse, useInputActivity } from '@colony2/shared';
import InputFormRenderer from './InputFormRenderer';
import { buildStoryTree, storyNodeName, type ArtifactKey, type StoryNode, type StoryKind, type StoryNodeRef } from '../utils/storyTree';
import { adaptInputFormConfig } from '../utils/formAdapter';

const { Title, Text } = Typography;

function newId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

type JobStatus =
  | 'running'
  | 'completed'
  | 'failed'
  | 'canceled'
  | 'terminated'
  | 'timed_out'
  | 'unknown';


interface JobStoryResponse {
  job_id: string;
  invocation_sequence?: number;
  recipe?: any;
  status: JobStatus;
  started_at?: string | null;
  finished_at?: string | null;
  root?: StoryNode | null;
}

type NodeRef = StoryNodeRef;

function nodeAttempt(node: StoryNode): number {
  return node.attempt ?? 1;
}


function formatTimestamp(ts?: string | null): string {
  if (!ts) return '-';
  return dayjs(ts).format('YYYY-MM-DD HH:mm:ss');
}

function durationSeconds(started?: string | null, finished?: string | null): number | null {
  if (!started || !finished) return null;
  const diff = dayjs(finished).diff(dayjs(started), 'second');
  return Number.isFinite(diff) ? diff : null;
}

function formatDuration(started?: string | null, finished?: string | null): string {
  const secs = durationSeconds(started, finished);
  if (secs === null) return '-';
  if (secs >= 60) {
    const minutes = Math.floor(secs / 60);
    const rem = secs % 60;
    return `${minutes}m ${rem}s`;
  }
  return `${secs}s`;
}

function statusTagColor(status: string): string {
  const colors: Record<string, string> = {
    running: 'blue',
    completed: 'green',
    succeeded: 'green',
    failed: 'red',
    canceled: 'default',
    terminated: 'default',
    timed_out: 'orange',
    skipped: 'gold',
    pending: 'default',
    unknown: 'default',
  };
  return colors[status] ?? 'default';
}

function artifactUrl(projectId: string, jobId: string, artifact: ArtifactKey): string {
  return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/tasks/${artifact.taskOrdinal}/artifacts/${encodeURIComponent(artifact.name)}`;
}

function formatHex(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let hex = '';
  let offset = 0;

  for (let i = 0; i < bytes.length; i += 16) {
    hex += offset.toString(16).padStart(8, '0') + '  ';

    const chunk = bytes.slice(i, i + 16);
    const hexBytes = Array.from(chunk)
      .map((b) => b.toString(16).padStart(2, '0'))
      .join(' ');
    hex += hexBytes.padEnd(48, ' ') + '  ';

    const ascii = Array.from(chunk)
      .map((b) => (b >= 32 && b <= 126 ? String.fromCharCode(b) : '.'))
      .join('');
    hex += ascii + '\n';

    offset += 16;
  }

  return hex;
}

function kindLabel(kind: StoryKind): string {
  switch (kind) {
    case 'recipe':
      return 'Recipe';
    case 'recipeSourceResolution':
      return 'Recipe Source';
    case 'sequence':
      return 'Sequence';
    case 'op':
      return 'Op';
    case 'opStep':
      return 'Op Step';
    case 'stateMachine':
      return 'State Machine';
    case 'state':
      return 'State';
    case 'transitionEval':
      return 'Transition Eval';
    case 'contextPatch':
      return 'Context Patch';
    default:
      return kind;
  }
}

function extractRecipeLabel(recipe: any): string {
  if (!recipe) return '-';
  return recipe.name || recipe.recipe_name || recipe.path || recipe.id || '-';
}

function parseUserValue(raw: string): unknown {
  const trimmed = raw.trim();
  if (!trimmed) return '';
  try {
    return JSON.parse(trimmed);
  } catch {
    return trimmed;
  }
}

function setMergePatchValue(target: Record<string, any>, path: string, value: unknown) {
  const parts = path
    .split('.')
    .map((p) => p.trim())
    .filter(Boolean);
  if (parts.length === 0) return;

  let cursor: Record<string, any> = target;
  for (let i = 0; i < parts.length - 1; i++) {
    const p = parts[i];
    const next = cursor[p];
    if (!next || typeof next !== 'object' || Array.isArray(next)) {
      cursor[p] = {};
    }
    cursor = cursor[p];
  }
  cursor[parts[parts.length - 1]] = value;
}

type PatchLine = { id: string; path: string; value: string };
type ScopePatchLine = { id: string; container: 'sequence' | 'states'; scopeId: string; path: string; value: string };

function buildContextPatch(jobLines: PatchLine[], scopeLines: ScopePatchLine[]) {
  const job: Record<string, any> = {};
  for (const line of jobLines) {
    if (!line.path.trim()) continue;
    setMergePatchValue(job, line.path, parseUserValue(line.value));
  }

  const grouped = new Map<string, { container: 'sequence' | 'states'; id: string; outputs: Record<string, any> }>();
  for (const line of scopeLines) {
    if (!line.scopeId.trim() || !line.path.trim()) continue;
    const k = `${line.container}::${line.scopeId}`;
    const existing = grouped.get(k) ?? { container: line.container, id: line.scopeId, outputs: {} };
    setMergePatchValue(existing.outputs, line.path, parseUserValue(line.value));
    grouped.set(k, existing);
  }

  const scopes = Array.from(grouped.values()).filter((s) => Object.keys(s.outputs).length > 0);

  const patch: any = {};
  if (Object.keys(job).length > 0) patch.job = job;
  if (scopes.length > 0) patch.scopes = scopes;
  return Object.keys(patch).length > 0 ? patch : null;
}

async function fetchStory(projectId: string, jobId: string): Promise<JobStoryResponse> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/story`
  );
  if (!response.ok) {
    const errText = await response.text().catch(() => 'Unknown error');
    throw new Error(`${response.status} ${response.statusText}${errText ? ` - ${errText}` : ''}`);
  }
  return response.json();
}

function keyForTaskOrdinal(taskOrdinal: number, keyToRef: Map<string, NodeRef>): string | null {
  for (const [key, ref] of keyToRef.entries()) {
    if (ref.type !== 'node') continue;
    if (ref.node.task_ordinal === taskOrdinal) return key;
  }
  return null;
}

function buildTree(root: StoryNode | null | undefined, treeOpts?: { pendingTaskOrdinal?: number | null }) {
  const model = buildStoryTree(root);
  const renderNodes = (nodes: typeof model.nodes): DataNode[] => nodes.map(({ key, node, label, children }) => ({
    key,
    title: <Space size={6} wrap>
      <Text strong>{label}</Text>
      <Tag color={statusTagColor(node.status)}>{node.status}</Tag>
      {node.task_ordinal != null && node.task_ordinal === treeOpts?.pendingTaskOrdinal && <Tag color="magenta">INPUT</Tag>}
      {durationSeconds(node.started_at, node.finished_at) !== null &&
        <Text type="secondary">{formatDuration(node.started_at, node.finished_at)}</Text>}
    </Space>,
    children: renderNodes(children),
  }));
  return { ...model, treeData: renderNodes(model.nodes), renderNodes };
}

function ancestorKeysForKey(key: string, keyToParent: Map<string, string | null>): string[] {
  const keys: string[] = [];
  let cursor = keyToParent.get(key) ?? null;
  while (cursor) {
    keys.push(cursor);
    cursor = keyToParent.get(cursor) ?? null;
  }
  return keys.reverse();
}

function unionKeys(a: string[], b: string[]): string[] {
  const set = new Set<string>(a);
  for (const k of b) set.add(k);
  return Array.from(set);
}

interface JobStoryPageProps {
  projectId: string;
}

export default function JobStoryPage({ projectId }: JobStoryPageProps) {
  const { jobId: routeJobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { pendingInputs, refresh: refreshPendingInputs } = useInputActivity();

  const [story, setStory] = useState<JobStoryResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [activeDetailsTab, setActiveDetailsTab] = useState<string>('overview');

  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const [autoRefresh, setAutoRefresh] = useState(false);
  const [refreshIntervalMs, setRefreshIntervalMs] = useState<number>(5000);
  const refreshTimerRef = useRef<number | null>(null);

  const [selectedArtifact, setSelectedArtifact] = useState<ArtifactKey | null>(null);
  const [artifactContent, setArtifactContent] = useState<string | null>(null);
  const [artifactLoading, setArtifactLoading] = useState(false);
  const [viewMode, setViewMode] = useState<'text' | 'hex'>('text');

  const [restartOpen, setRestartOpen] = useState(false);
  const [restartSubmitting, setRestartSubmitting] = useState(false);
  const [jobPatchLines, setJobPatchLines] = useState<PatchLine[]>([]);
  const [scopePatchLines, setScopePatchLines] = useState<ScopePatchLine[]>([]);
  const [applyPatch, setApplyPatch] = useState(false);

  const [pendingInputDetails, setPendingInputDetails] = useState<UserInputDetails | null>(null);
  const [pendingInputLoading, setPendingInputLoading] = useState(false);
  const [pendingInputError, setPendingInputError] = useState<string | null>(null);
  const [pendingInputSubmitting, setPendingInputSubmitting] = useState(false);

  const jobId = routeJobId || '';

  const wantInput = searchParams.get('input') === '1';
  const taskOrdinalParamRaw = searchParams.get('taskOrdinal');
  const taskOrdinalParam = taskOrdinalParamRaw ? Number(taskOrdinalParamRaw) : null;

  const isPendingInput = useMemo(() => pendingInputs.some((p) => p.id === jobId), [pendingInputs, jobId]);

  const pendingTaskOrdinal = useMemo(() => {
    const details: any = pendingInputDetails;
    const ord = details?.task_ordinal ?? details?.taskOrdinal;
    return typeof ord === 'number' ? ord : null;
  }, [pendingInputDetails]);

  const focusTaskOrdinal = taskOrdinalParam !== null && Number.isFinite(taskOrdinalParam) ? taskOrdinalParam : pendingTaskOrdinal;

  const { treeData, keyToRef, keyToParent, keyFor, ancestorsByKey, retriesByKey, currentByRetryKey,
    projectionByKey, visibleKeyByKey, childrenByNode, renderNodes } = useMemo(
    () => buildTree(story?.root, { pendingTaskOrdinal: focusTaskOrdinal }),
    [story?.root, focusTaskOrdinal]
  );

  const selectedRef = useMemo(() => {
    if (!selectedKey) return null;
    return keyToRef.get(selectedKey) ?? null;
  }, [keyToRef, selectedKey]);

  const selectedNode = selectedRef?.type === 'node' ? selectedRef.node : null;
  const selectedAncestors = selectedKey ? ancestorsByKey.get(selectedKey) || [] : [];
  const taskRetries = selectedKey ? retriesByKey.get(selectedKey) || [] : [];
  const jobRetries = selectedNode?.past_attempts || [];
  const retryOwner = selectedKey ? keyToRef.get(currentByRetryKey.get(selectedKey) || '') : undefined;
  const operation = selectedNode?.kind === 'op' ? selectedNode : [...selectedAncestors].reverse().find(n => n.kind === 'op');
  const contextNodes = selectedNode ? [...selectedAncestors, ...(childrenByNode.get(selectedNode) || [])] : [];


  const load = async (opts?: { preserveSelection?: boolean }) => {
    if (!jobId) return;
    setLoading(true);
    setError(null);
    try {
      const data = await fetchStory(projectId, jobId);
      const { treeData: nextTreeData, keyToRef: nextKeyToRef, keyToParent: nextKeyToParent, legacyKeyToKey: nextLegacyKeyToKey } =
        buildTree(data.root);
      setStory(data);

      if (data.status === 'running') {
        setAutoRefresh((prev) => prev || true);
      }

      const urlNodeId = searchParams.get('nodeId');
      const urlPath = searchParams.get('path');
      const urlAttempt = searchParams.get('attempt');
      const legacyUrlKey =
        urlPath && urlAttempt ? `${urlPath}|attempt:${urlAttempt}` : urlPath ? `${urlPath}|attempt:1` : null;
      const legacyResolvedKey = legacyUrlKey ? (nextLegacyKeyToKey.get(legacyUrlKey) ?? null) : null;
      const rootKey = (nextTreeData[0]?.key as string | undefined) ?? null;

      let nextSelectedKey: string | null = null;
      if (urlNodeId && nextKeyToRef.has(urlNodeId)) {
        nextSelectedKey = urlNodeId;
      } else if (urlNodeId && nextLegacyKeyToKey.has(urlNodeId)) {
        nextSelectedKey = nextLegacyKeyToKey.get(urlNodeId)!;
      } else if (legacyResolvedKey && nextKeyToRef.has(legacyResolvedKey)) {
        nextSelectedKey = legacyResolvedKey;
      } else if (opts?.preserveSelection && selectedKey && nextKeyToRef.has(selectedKey)) {
        nextSelectedKey = selectedKey;
      } else {
        nextSelectedKey = rootKey;
      }

      if (nextSelectedKey && nextKeyToRef.has(nextSelectedKey)) {
        setSelectedKey(nextSelectedKey);
        const ancestors = ancestorKeysForKey(nextSelectedKey, nextKeyToParent);
        setExpandedKeys((prev) => opts?.preserveSelection
          ? unionKeys(prev, ancestors)
          : Array.from(nextKeyToRef.keys()));
      }
    } catch (e) {
      console.error('Failed to load job story', e);
      setError(e instanceof Error ? e.message : 'Failed to load job story');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, jobId]);

  useEffect(() => {
    const shouldLoadDetails = !!jobId && (isPendingInput || wantInput);
    if (!shouldLoadDetails) {
      setPendingInputDetails(null);
      setPendingInputError(null);
      setPendingInputLoading(false);
      return;
    }

    let cancelled = false;
    setPendingInputLoading(true);
    setPendingInputError(null);

    inputActivityService
      .getInputDetails(projectId, jobId)
      .then((d) => {
        if (cancelled) return;
        setPendingInputDetails(d);
      })
      .catch((e) => {
        if (cancelled) return;
        console.error('Failed to load pending input details', e);
        setPendingInputError(e instanceof Error ? e.message : 'Failed to load pending input details');
        setPendingInputDetails(null);
      })
      .finally(() => {
        if (cancelled) return;
        setPendingInputLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [projectId, jobId, isPendingInput, wantInput, pendingInputs]);

  useEffect(() => {
    if (!focusTaskOrdinal || !story?.root) return;
    const key = keyForTaskOrdinal(focusTaskOrdinal, keyToRef);
    if (!key) return;

    const ref = keyToRef.get(key);
    if (!ref || ref.type !== 'node') return;

    const ancestors = ancestorKeysForKey(key, keyToParent);
    setExpandedKeys((prev) => unionKeys(prev, ancestors));
    setSelectedKey(key);
    setActiveDetailsTab(wantInput ? 'pending_input' : 'overview');
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set('nodeId', key);
      next.set('path', ref.node.path.join('/'));
      next.set('attempt', String(ref.attempt));
      next.set('taskOrdinal', String(focusTaskOrdinal));
      if (wantInput) next.set('input', '1');
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusTaskOrdinal, story?.root, keyToRef, keyToParent]);

  useEffect(() => {
    if (refreshTimerRef.current) {
      window.clearInterval(refreshTimerRef.current);
      refreshTimerRef.current = null;
    }

    if (autoRefresh && story?.status === 'running') {
      refreshTimerRef.current = window.setInterval(() => {
        load({ preserveSelection: true });
      }, refreshIntervalMs);
    }

    return () => {
      if (refreshTimerRef.current) {
        window.clearInterval(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRefresh, refreshIntervalMs, story?.status, projectId, jobId, selectedKey]);

  const openArtifact = async (artifact: ArtifactKey) => {
    setSelectedArtifact(artifact);
    setArtifactLoading(true);
    setArtifactContent(null);
    setViewMode('text');

    try {
      const url = artifactUrl(projectId, jobId, artifact);
      const response = await fetch(url);
      if (!response.ok) throw new Error('Failed to fetch artifact');
      const blob = await response.blob();
      const text = await blob.text();
      setArtifactContent(text);
    } catch (e) {
      console.error('Failed to load artifact', e);
      message.error('Failed to load artifact content');
      setSelectedArtifact(null);
    } finally {
      setArtifactLoading(false);
    }
  };

  const downloadArtifact = (artifact: ArtifactKey) => {
    const url = artifactUrl(projectId, jobId, artifact);
    const link = document.createElement('a');
    link.href = url;
    link.download = artifact.name;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const setSelectedFromNode = (node: StoryNode) => {
    const key = Array.from(keyToRef.values()).find((ref) => ref.type === 'node' && ref.node === node)?.key;
    if (!key) return;
    setSelectedKey(key);
    setActiveDetailsTab('overview');
    const ancestors = ancestorKeysForKey(key, keyToParent);
    setExpandedKeys((prev) => unionKeys(prev, ancestors));
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set('nodeId', key);
      next.set('path', node.path.join('/'));
      next.set('attempt', String(nodeAttempt(node)));
      return next;
    });
  };

  const restartable =
    !!selectedNode &&
    selectedNode.kind !== 'contextPatch' &&
    selectedNode.restart_from_ordinal !== null &&
    selectedNode.restart_from_ordinal !== undefined;

  const resetRestartState = () => {
    setApplyPatch(false);
    setJobPatchLines([]);
    setScopePatchLines([]);
  };

  const submitRestart = async () => {
    if (!jobId || !selectedNode) return;
    const stepOffset = selectedNode.restart_from_ordinal;
    if (stepOffset === null || stepOffset === undefined) return;

    const contextPatch = applyPatch ? buildContextPatch(jobPatchLines, scopePatchLines) : null;

    setRestartSubmitting(true);
    try {
      const response = await fetch(
        `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/restart`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            step_offset: stepOffset,
            ...(contextPatch ? { patch: contextPatch } : {}),
          }),
        }
      );

      if (!response.ok) {
        const errText = await response.text().catch(() => 'Unknown error');
        throw new Error(`${response.status} ${response.statusText}${errText ? ` - ${errText}` : ''}`);
      }

      const data = (await response.json()) as { job_id: string };
      message.success('Restarted job');
      setRestartOpen(false);
      resetRestartState();
      navigate(`/project/${projectId}/jobs/${data.job_id}/story`);
    } catch (e) {
      console.error('Restart failed', e);
      message.error(e instanceof Error ? e.message : 'Restart failed');
    } finally {
      setRestartSubmitting(false);
    }
  };

  const detailsHeader = (
    <Space direction="vertical" style={{ width: '100%' }} size="small">
      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
        <Text strong>Node Details</Text>
        {selectedNode ? (
          <Button
            size="small"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(JSON.stringify(selectedNode.path));
                message.success('Copied node path');
              } catch {
                message.error('Failed to copy');
              }
            }}
          >
            Copy Path
          </Button>
        ) : null}
      </Space>
      {selectedNode ? (
        <Text type="secondary">
          {projectionByKey.get(selectedKey || '')?.[0]?.label || storyNodeName(selectedNode)}
        </Text>
      ) : (
        <Text type="secondary">Select a node to inspect input/output, artifacts, and retries.</Text>
      )}
    </Space>
  );

  if (loading && !story) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Spin size="large" />
      </div>
    );
  }

  const pendingInputForm = pendingInputDetails ? adaptInputFormConfig(pendingInputDetails.form, jobId) : null;
  const selectedIsPendingNode =
    focusTaskOrdinal !== null &&
    focusTaskOrdinal !== undefined &&
    selectedNode?.task_ordinal === focusTaskOrdinal;

  const submitPendingInput = async (response: FormResponse) => {
    if (!jobId) return;
    setPendingInputSubmitting(true);
    try {
      await inputActivityService.submitResponse(projectId, jobId, response);
      message.success('Response submitted');
      await refreshPendingInputs();
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        next.delete('input');
        next.delete('taskOrdinal');
        return next;
      });
      setActiveDetailsTab('overview');
      setPendingInputDetails(null);
      await load({ preserveSelection: true });
    } catch (e) {
      console.error('Failed to submit input response', e);
      message.error(e instanceof Error ? e.message : 'Failed to submit response');
    } finally {
      setPendingInputSubmitting(false);
    }
  };

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(`/project/${projectId}/jobs`)}>
              Back to Jobs
            </Button>
            <Title level={2} style={{ margin: 0 }}>
              Job Story
            </Title>
            {story?.job_id ? <Text code>{story.job_id.slice(-12)}</Text> : null}
          </Space>
          <Space>
            {story?.status === 'running' ? (
              <Space>
                <Text type="secondary">Auto-refresh</Text>
                <Switch checked={autoRefresh} onChange={setAutoRefresh} />
                <Select
                  value={refreshIntervalMs}
                  onChange={setRefreshIntervalMs}
                  style={{ width: 120 }}
                  options={[
                    { label: '3s', value: 3000 },
                    { label: '5s', value: 5000 },
                    { label: '10s', value: 10000 },
                  ]}
                />
              </Space>
            ) : null}
            <Button icon={<ReloadOutlined />} onClick={() => load({ preserveSelection: true })}>
              Refresh
            </Button>
          </Space>
        </div>

        {error ? (
          <Alert
            type="error"
            showIcon
            message="Failed to load job story"
            description={error}
            action={
              <Button size="small" onClick={() => load()}>
                Retry
              </Button>
            }
          />
        ) : null}

        {pendingInputLoading ? (
          <Alert
            type="warning"
            showIcon
            message="Pending input"
            description="Loading input prompt..."
          />
        ) : pendingInputError ? (
          <Alert
            type="warning"
            showIcon
            message="Pending input"
            description={pendingInputError}
          />
        ) : pendingInputDetails ? (
          <Alert
            type="warning"
            showIcon
            message="Pending input required"
            description={
              <div>
                This job is waiting for input.
                {focusTaskOrdinal !== null ? (
                  <>
                    {' '}Task ordinal: <Text code>{focusTaskOrdinal}</Text>.
                  </>
                ) : null}
              </div>
            }
            action={
              <Button
                size="small"
                type="primary"
                onClick={() => {
                  setSearchParams((prev) => {
                    const next = new URLSearchParams(prev);
                    next.set('input', '1');
                    if (focusTaskOrdinal !== null) next.set('taskOrdinal', String(focusTaskOrdinal));
                    return next;
                  });
                  setActiveDetailsTab('pending_input');
                }}
              >
                Open prompt
              </Button>
            }
          />
        ) : null}

        <Card title="Run Summary" size="small">
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="Job ID" span={2}>
              {story?.job_id ? <Text code>{story.job_id}</Text> : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="Status">
              {story?.status ? (
                <Tag color={statusTagColor(story.status)}>{story.status.toUpperCase()}</Tag>
              ) : (
                '-'
              )}
            </Descriptions.Item>
            <Descriptions.Item label="Recipe">
              {extractRecipeLabel(story?.recipe)}
            </Descriptions.Item>
            <Descriptions.Item label="Started">
              {formatTimestamp(story?.started_at)}
            </Descriptions.Item>
            <Descriptions.Item label="Finished">
              {formatTimestamp(story?.finished_at)}
            </Descriptions.Item>
            <Descriptions.Item label="Duration">
              {formatDuration(story?.started_at ?? null, story?.finished_at ?? null)}
            </Descriptions.Item>
            <Descriptions.Item label="Invocation Seq">
              {story?.invocation_sequence ?? '-'}
            </Descriptions.Item>
          </Descriptions>

          {story?.recipe ? (
            <details style={{ marginTop: 12 }}>
              <summary>Recipe metadata</summary>
              <div style={{ marginTop: 8 }}>
                <ReactJson
                  src={story.recipe}
                  collapsed={1}
                  displayDataTypes={false}
                  enableClipboard
                  theme="rjv-default"
                />
              </div>
            </details>
          ) : null}
        </Card>

        <div style={{ display: 'flex', gap: 16, alignItems: 'stretch' }}>
          <Card title="Story" style={{ flex: 1, minWidth: 420 }} styles={{ body: { padding: 12, overflowX: 'auto' } }}
            extra={<Space>
              {!!story?.root?.past_attempts?.length && <Button size="small" onClick={() => {
                setSelectedFromNode(story.root!);
                setActiveDetailsTab('retries');
              }}>Job retries ({story.root.past_attempts.length})</Button>}
              <Button size="small" onClick={() => setExpandedKeys(Array.from(keyToParent.keys()))}>Expand all</Button>
              <Button size="small" onClick={() => setExpandedKeys([])}>Collapse all</Button>
            </Space>}>
            {!story?.root ? (
              <Empty description={story && story.status !== 'running'
                ? 'The story API returned no steps for this finished job.'
                : 'No story available yet'} />
            ) : (
              <Tree
                showLine
                treeData={treeData}
                expandedKeys={expandedKeys}
                selectedKeys={selectedKey ? [visibleKeyByKey.get(selectedKey) || selectedKey] : []}
                onExpand={(keys) => setExpandedKeys(keys as string[])}
                onSelect={(keys) => {
                  const key = (keys?.[0] as string) || null;
                  if (!key) return;
                  const ref = keyToRef.get(key);
                  if (!ref) return;
                  if (ref.type === 'node') {
                    const ancestors = ancestorKeysForKey(key, keyToParent);
                    setExpandedKeys((prev) => unionKeys(prev, ancestors));
                    setSelectedKey(key);
                    setActiveDetailsTab('overview');
                    setSearchParams((prev) => {
                      const next = new URLSearchParams(prev);
                      next.set('nodeId', key);
                      next.set('path', ref.node.path.join('/'));
                      next.set('attempt', String(ref.attempt));
                      return next;
                    });
                    return;
                  }
                  setSelectedKey(key);
                }}
              />
            )}
          </Card>

          <Card title={detailsHeader} style={{ flex: 1.2, minWidth: 400 }} styles={{ body: { padding: 12, overflowWrap: 'anywhere' } }}>
            {!selectedNode ? (
              <Empty description="Select a node from the story tree" />
            ) : (
              <Tabs
                activeKey={activeDetailsTab}
                onChange={setActiveDetailsTab}
                items={[
                  {
                    key: 'overview',
                    label: 'Overview',
                    children: (
                      <Space direction="vertical" style={{ width: '100%' }} size="middle">
                        {retryOwner && <Alert type="info" showIcon
                          message={`Viewing ${selectedRef?.jobAttempt !== keyToRef.get(keyFor.get(story?.root!) || '')?.jobAttempt ? `job attempt ${selectedRef?.jobAttempt}` : `attempt ${nodeAttempt(selectedNode)}`}`}
                          action={<Button size="small" onClick={() => setSelectedFromNode(retryOwner.node)}>Back to latest</Button>} />}
                        {selectedNode.error && (
                          <Alert type="error" showIcon message={selectedNode.error.code || 'Task failure'}
                            description={<pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', margin: 0 }}>{selectedNode.error.message}</pre>} />
                        )}
                        <Descriptions column={1} bordered size="small" styles={{ label: { minWidth: 130, width: 150 }, content: { wordBreak: 'break-word' } }}>
                          <Descriptions.Item label="Kind">
                            {kindLabel(selectedNode.kind)}
                          </Descriptions.Item>
                          <Descriptions.Item label="Title">
                            {selectedNode.title}
                          </Descriptions.Item>
                          {operation && <Descriptions.Item label="Operation source">
                            <Text style={{ overflowWrap: 'anywhere' }}>{operation.op_id || operation.title.replace(/^op /, '')}</Text>
                          </Descriptions.Item>}
                          <Descriptions.Item label="Status">
                            <Tag color={statusTagColor(selectedNode.status)}>
                              {selectedNode.status.toUpperCase()}
                            </Tag>
                          </Descriptions.Item>
                          <Descriptions.Item label="Job Attempt">
                            {selectedRef?.type === 'node' ? selectedRef.jobAttempt : '-'}
                          </Descriptions.Item>
                          <Descriptions.Item label="Task Attempt">
                            {nodeAttempt(selectedNode)}
                          </Descriptions.Item>
                          <Descriptions.Item label="Task Ordinal">
                            {selectedNode.task_ordinal ?? '-'}
                          </Descriptions.Item>
                          <Descriptions.Item label="Restart From Ordinal">
                            {selectedNode.restart_from_ordinal ?? '-'}
                          </Descriptions.Item>
                          <Descriptions.Item label="Invoke Seq">
                            {selectedNode.invoke_seq ?? '-'}
                          </Descriptions.Item>
                          <Descriptions.Item label="Started">
                            {formatTimestamp(selectedNode.started_at)}
                          </Descriptions.Item>
                          <Descriptions.Item label="Finished">
                            {formatTimestamp(selectedNode.finished_at)}
                          </Descriptions.Item>
                          <Descriptions.Item label="Duration">
                            {formatDuration(selectedNode.started_at ?? null, selectedNode.finished_at ?? null)}
                          </Descriptions.Item>
                          <Descriptions.Item label="Path">
                            <Text code>{selectedNode.path.join(' / ')}</Text>
                          </Descriptions.Item>
                        </Descriptions>

                        {contextNodes.length > 0 && <details>
                          <summary>Execution context ({contextNodes.length})</summary>
                          <List size="small" dataSource={contextNodes} renderItem={node => <List.Item
                            actions={[<Button key="inspect" size="small" onClick={() => setSelectedFromNode(node)}>Inspect</Button>]}>
                            <Text style={{ overflowWrap: 'anywhere' }}>{node.title}</Text>
                          </List.Item>} />
                        </details>}

                        {restartable ? (
                          <Card size="small" title="Restart">
                            <Space direction="vertical" style={{ width: '100%' }}>
                              <Text type="secondary">
                                Restarts create a new job and resume from step offset{' '}
                                <Text code>{selectedNode.restart_from_ordinal}</Text>.
                              </Text>
                              <Button type="primary" onClick={() => setRestartOpen(true)}>
                                Restart from here
                              </Button>
                            </Space>
                          </Card>
                        ) : null}

                        {selectedNode.kind === 'contextPatch' ? (
                          <Card title="Applied Context Patch" size="small">
                            {selectedNode.output ? (
                              <ReactJson
                                src={selectedNode.output as any}
                                collapsed={2}
                                displayDataTypes={false}
                                enableClipboard
                                theme="rjv-default"
                              />
                            ) : (
                              <Empty description="No patch payload found" />
                            )}
                          </Card>
                        ) : null}

                        {selectedNode.kind === 'transitionEval' ? (
                          <Card title="Transition" size="small">
                            {selectedNode.evaluations && selectedNode.evaluations.length > 0 ? (
                              <>
                                <List
                                  size="small"
                                  dataSource={selectedNode.evaluations}
                                  renderItem={(ev) => (
                                    <List.Item>
                                      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                                        <Text code>{ev.expression}</Text>
                                        <Space>
                                          <Tag color={ev.result ? 'green' : 'default'}>
                                            {ev.result ? 'true' : 'false'}
                                          </Tag>
                                          <Text type="secondary">→ {ev.to_state_id}</Text>
                                        </Space>
                                      </Space>
                                    </List.Item>
                                  )}
                                />
                                <div style={{ marginTop: 12 }}>
                                  <Text strong>Decision: </Text>
                                  {selectedNode.decision ? (
                                    selectedNode.decision.kind === 'state' ? (
                                      <Tag color="blue">to {selectedNode.decision.to_state_id}</Tag>
                                    ) : (
                                      <Tag>fallthrough</Tag>
                                    )
                                  ) : (
                                    <Text type="secondary">-</Text>
                                  )}
                                </div>
                              </>
                            ) : (
                              <Empty description="No transition evaluation data" />
                            )}
                          </Card>
                        ) : null}
                      </Space>
                    ),
                  },
                  ...(pendingInputDetails
                    ? [
                        {
                          key: 'pending_input',
                          label: pendingInputDetails.form.kind === 'review' ? 'Review' : 'Pending Input',
                          children: (
                            <Space direction="vertical" style={{ width: '100%' }} size="middle">
                              <Card size="small" title={pendingInputDetails.form.kind === 'review' ? 'Review' : 'Input Request'}>
                                <Descriptions column={1} bordered size="small">
                                  <Descriptions.Item label="Status">
                                    <Tag color="magenta">{pendingInputDetails.status}</Tag>
                                  </Descriptions.Item>
                                  <Descriptions.Item label="Started">
                                    {pendingInputDetails.startTime
                                      ? dayjs(pendingInputDetails.startTime).format('YYYY-MM-DD HH:mm:ss')
                                      : '-'}
                                  </Descriptions.Item>
                                  <Descriptions.Item label="Task Ordinal">
                                    {focusTaskOrdinal ?? '-'}
                                  </Descriptions.Item>
                                </Descriptions>
                              </Card>

                              {focusTaskOrdinal !== null && !selectedIsPendingNode ? (
                                <Alert
                                  type="info"
                                  showIcon
                                  message="Select the input node"
                                  description="The selected story node does not match the pending input’s task ordinal. Use the story tree to select the highlighted INPUT node."
                                />
                              ) : null}

                              {pendingInputDetails.form.kind === 'review' ? (
                                <Card title={pendingInputDetails.form.title || 'Review'} size="small">
                                  <Button type="primary" href={`/project/${projectId}/reviews/${jobId}`}>Open Review</Button>
                                </Card>
                              ) : pendingInputForm ? (
                                <Card title={pendingInputForm.title} size="small">
                                  <InputFormRenderer
                                    form={pendingInputForm}
                                    onSubmit={submitPendingInput}
                                    onCancel={() => {
                                      setSearchParams((prev) => {
                                        const next = new URLSearchParams(prev);
                                        next.delete('input');
                                        next.delete('taskOrdinal');
                                        return next;
                                      });
                                      setActiveDetailsTab('overview');
                                    }}
                                    loading={pendingInputSubmitting}
                                  />
                                </Card>
                              ) : (
                                <Empty description="No input form available" />
                              )}
                            </Space>
                          ),
                        },
                      ]
                    : []),
                  {
                    key: 'input',
                    label: 'Input',
                    children: selectedNode.input ? (
                      <ReactJson
                        src={selectedNode.input as any}
                        collapsed={2}
                        displayDataTypes={false}
                        enableClipboard
                        theme="rjv-default"
                      />
                    ) : (
                      <Empty description="No input payload" />
                    ),
                  },
                  {
                    key: 'output',
                    label: 'Output',
                    children: selectedNode.output ? (
                      <ReactJson
                        src={selectedNode.output as any}
                        collapsed={2}
                        displayDataTypes={false}
                        enableClipboard
                        theme="rjv-default"
                      />
                    ) : (
                      <Empty description="No output payload" />
                    ),
                  },
                  {
                    key: 'artifacts',
                    label: `Artifacts (${selectedNode.artifact_keys?.length ?? 0})`,
                    children: (
                      <>
                        {selectedNode.artifact_keys && selectedNode.artifact_keys.length > 0 ? (
                          <List
                            dataSource={selectedNode.artifact_keys}
                            renderItem={(a) => (
                              <List.Item
                                actions={[
                                  <Button
                                    key="download"
                                    size="small"
                                    icon={<DownloadOutlined />}
                                    onClick={() => downloadArtifact(a)}
                                  >
                                    Download
                                  </Button>,
                                ]}
                              >
                                <List.Item.Meta
                                  title={
                                    <a
                                      href="#"
                                      onClick={(e) => {
                                        e.preventDefault();
                                        openArtifact(a);
                                      }}
                                      style={{ fontWeight: 600 }}
                                    >
                                      {a.name}
                                    </a>
                                  }
                                  description={
                                    <Text type="secondary">
                                      task {a.taskOrdinal}
                                      {a.sizeBytes !== undefined ? ` · ${(a.sizeBytes / 1024).toFixed(2)} KB` : ''}
                                    </Text>
                                  }
                                />
                              </List.Item>
                            )}
                          />
                        ) : (
                          <Empty description="No artifacts on this node" />
                        )}
                      </>
                    ),
                  },
                  {
                    key: 'retries',
                    label: `Retries (${taskRetries.length + jobRetries.length})`,
                    children: taskRetries.length + jobRetries.length > 0 ? <Space direction="vertical" style={{ width: '100%' }}>
                      {taskRetries.length > 0 && <List style={{ width: '100%' }}
                        dataSource={taskRetries}
                        renderItem={pa => <List.Item actions={[
                          <Button key="inspect" size="small" onClick={() => setSelectedFromNode(pa)}>Inspect attempt {nodeAttempt(pa)}</Button>,
                        ]}>
                          <Space direction="vertical" size={4}>
                            <Space><Text strong>Attempt {nodeAttempt(pa)}</Text><Tag color={statusTagColor(pa.status)}>{pa.status}</Tag></Space>
                            <Text type="secondary">{formatTimestamp(pa.started_at)} · {formatDuration(pa.started_at, pa.finished_at)}</Text>
                            {pa.error && <Text type="danger" style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{pa.error.message}</Text>}
                          </Space>
                        </List.Item>} />}
                      {jobRetries.map((past, i) => <Card size="small" style={{ width: '100%' }} key={keyFor.get(past)}
                        title={`Job attempt ${past.job_attempt || i + 1}`}
                        extra={<Button size="small" onClick={() => setSelectedFromNode(past)}>Inspect job attempt {past.job_attempt || i + 1}</Button>}>
                        <Tree defaultExpandAll treeData={renderNodes(projectionByKey.get(keyFor.get(past)!) || [])}
                          onSelect={keys => {
                            const ref = keyToRef.get(String(keys[0]));
                            if (ref) setSelectedFromNode(ref.node);
                          }} />
                      </Card>)}
                    </Space> : <Empty description="No prior attempts" />,
                  },
                ]}
              />
            )}
          </Card>
        </div>
      </Space>

      <Modal
        title={selectedArtifact?.name || 'Artifact Viewer'}
        open={selectedArtifact !== null}
        onCancel={() => {
          setSelectedArtifact(null);
          setArtifactContent(null);
        }}
        width={1000}
        footer={[
          <Button
            key="download"
            icon={<DownloadOutlined />}
            onClick={() => selectedArtifact && downloadArtifact(selectedArtifact)}
          >
            Download
          </Button>,
          <Button key="close" onClick={() => setSelectedArtifact(null)}>
            Close
          </Button>,
        ]}
      >
        {artifactLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>
            <Spin />
          </div>
        ) : (
          <>
            <Space style={{ marginBottom: 16 }}>
              <Text>View mode:</Text>
              <Radio.Group value={viewMode} onChange={(e) => setViewMode(e.target.value)}>
                <Radio.Button value="text">Text</Radio.Button>
                <Radio.Button value="hex">Hex</Radio.Button>
              </Radio.Group>
            </Space>
            <div
              style={{
                backgroundColor: '#f5f5f5',
                padding: 16,
                borderRadius: 4,
                maxHeight: 500,
                overflow: 'auto',
                fontFamily: 'monospace',
                fontSize: 12,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
              }}
            >
              {artifactContent ? (
                viewMode === 'text' ? (
                  artifactContent
                ) : (
                  formatHex(artifactContent)
                )
              ) : (
                <Text type="secondary">No content available</Text>
              )}
            </div>
          </>
        )}
      </Modal>

      <Modal
        title="Restart Job"
        open={restartOpen}
        onCancel={() => {
          setRestartOpen(false);
          resetRestartState();
        }}
        width={900}
        okText="Restart"
        okButtonProps={{ loading: restartSubmitting, disabled: !restartable }}
        onOk={submitRestart}
      >
        {selectedNode ? (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Alert
              type="info"
              showIcon
              message="This will create a new job"
              description={
                <div>
                  Restarting from <Text strong>{selectedNode.title}</Text> (step offset{' '}
                  <Text code>{selectedNode.restart_from_ordinal ?? '-'}</Text>).
                </div>
              }
            />

            <Card title="Context Patching" size="small">
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Text type="secondary">
                    Apply additional context over the existing context before resuming execution.
                  </Text>
                  <Space>
                    <Text type="secondary">Enable</Text>
                    <Switch checked={applyPatch} onChange={setApplyPatch} />
                  </Space>
                </Space>

                {!applyPatch ? (
                  <Text type="secondary">No context patch will be applied.</Text>
                ) : (
                  <Space direction="vertical" style={{ width: '100%' }} size="large">
                    <Card
                      size="small"
                      title="Job Patch (global)"
                      extra={
                        <Button
                          size="small"
                          onClick={() =>
                            setJobPatchLines((prev) => [
                              ...prev,
                              { id: newId(), path: '', value: '' },
                            ])
                          }
                        >
                          Add line
                        </Button>
                      }
                    >
                      <Text type="secondary">
                        Each line sets a merge-patch value under <Text code>context</Text>. Use dot
                        paths like <Text code>git.author</Text>. Values accept JSON (e.g.{' '}
                        <Text code>"james@example.com"</Text>, <Text code>true</Text>,{' '}
                        <Text code>123</Text>, <Text code>null</Text>) or raw text.
                      </Text>
                      <div style={{ marginTop: 12 }}>
                        {jobPatchLines.length === 0 ? (
                          <Empty description="No job patch lines" />
                        ) : (
                          <List
                            size="small"
                            dataSource={jobPatchLines}
                            renderItem={(line) => (
                              <List.Item
                                actions={[
                                  <Button
                                    key="remove"
                                    size="small"
                                    onClick={() =>
                                      setJobPatchLines((prev) => prev.filter((p) => p.id !== line.id))
                                    }
                                  >
                                    Remove
                                  </Button>,
                                ]}
                              >
                                <Space style={{ width: '100%' }}>
                                  <Text style={{ width: 80 }} type="secondary">
                                    Path
                                  </Text>
                                  <Input
                                    style={{ flex: 1, padding: 6 }}
                                    value={line.path}
                                    placeholder='e.g. git.author'
                                    onChange={(e) =>
                                      setJobPatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, path: e.target.value } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 60 }} type="secondary">
                                    Value
                                  </Text>
                                  <Input
                                    style={{ flex: 1, padding: 6 }}
                                    value={line.value}
                                    placeholder='e.g. "james@example.com"'
                                    onChange={(e) =>
                                      setJobPatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, value: e.target.value } : p))
                                      )
                                    }
                                  />
                                </Space>
                              </List.Item>
                            )}
                          />
                        )}
                      </div>
                    </Card>

                    <Card
                      size="small"
                      title="Scoped Patches (local outputs)"
                      extra={
                        <Button
                          size="small"
                          onClick={() =>
                            setScopePatchLines((prev) => [
                              ...prev,
                              {
                                id: newId(),
                                container: 'sequence',
                                scopeId: '',
                                path: '',
                                value: '',
                              },
                            ])
                          }
                        >
                          Add line
                        </Button>
                      }
                    >
                      <Text type="secondary">
                        Each line applies a merge-patch value under a scope container’s outputs.
                        Container is <Text code>sequence</Text> or <Text code>states</Text>; id is the
                        step/state id; path is under outputs (dot path).
                      </Text>
                      <div style={{ marginTop: 12 }}>
                        {scopePatchLines.length === 0 ? (
                          <Empty description="No scoped patch lines" />
                        ) : (
                          <List
                            size="small"
                            dataSource={scopePatchLines}
                            renderItem={(line) => (
                              <List.Item
                                actions={[
                                  <Button
                                    key="remove"
                                    size="small"
                                    onClick={() =>
                                      setScopePatchLines((prev) => prev.filter((p) => p.id !== line.id))
                                    }
                                  >
                                    Remove
                                  </Button>,
                                ]}
                              >
                                <Space style={{ width: '100%' }} wrap>
                                  <Text style={{ width: 80 }} type="secondary">
                                    Container
                                  </Text>
                                  <Select
                                    value={line.container}
                                    style={{ width: 140 }}
                                    options={[
                                      { label: 'sequence', value: 'sequence' },
                                      { label: 'states', value: 'states' },
                                    ]}
                                    onChange={(v) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, container: v } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 30 }} type="secondary">
                                    ID
                                  </Text>
                                  <Input
                                    style={{ width: 160 }}
                                    value={line.scopeId}
                                    placeholder='e.g. build'
                                    onChange={(e) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, scopeId: e.target.value } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 40 }} type="secondary">
                                    Path
                                  </Text>
                                  <Input
                                    style={{ width: 200 }}
                                    value={line.path}
                                    placeholder='e.g. image_tag'
                                    onChange={(e) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, path: e.target.value } : p))
                                      )
                                    }
                                  />
                                  <Text style={{ width: 50 }} type="secondary">
                                    Value
                                  </Text>
                                  <Input
                                    style={{ flex: 1, minWidth: 180 }}
                                    value={line.value}
                                    placeholder='e.g. "v2"'
                                    onChange={(e) =>
                                      setScopePatchLines((prev) =>
                                        prev.map((p) => (p.id === line.id ? { ...p, value: e.target.value } : p))
                                      )
                                    }
                                  />
                                </Space>
                              </List.Item>
                            )}
                          />
                        )}
                      </div>
                    </Card>

                    <Card size="small" title="Patch Preview">
                      <ReactJson
                        src={buildContextPatch(jobPatchLines, scopePatchLines) ?? {}}
                        collapsed={2}
                        displayDataTypes={false}
                        enableClipboard
                        theme="rjv-default"
                      />
                    </Card>
                  </Space>
                )}
              </Space>
            </Card>
          </Space>
        ) : (
          <Empty description="Select a restartable node first" />
        )}
      </Modal>
    </div>
  );
}
