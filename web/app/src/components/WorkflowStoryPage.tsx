import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
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
  BranchesOutlined,
  DownloadOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import ReactJson from 'react-json-view';

const { Title, Text } = Typography;

const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

type JobStatus =
  | 'running'
  | 'completed'
  | 'failed'
  | 'canceled'
  | 'terminated'
  | 'timed_out'
  | 'unknown';

type NodeStatus =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'skipped'
  | 'unknown';

type StoryKind =
  | 'recipe'
  | 'sequence'
  | 'op'
  | 'opStep'
  | 'stateMachine'
  | 'state'
  | 'transitionEval';

interface ArtifactKey {
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

interface StoryNode {
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
  prior_attempts?: StoryNode[];

  // transitionEval-only
  evaluations?: TransitionEvaluation[];
  decision?: TransitionDecision;
}

interface WorkflowStoryResponse {
  job_id: string;
  invocation_sequence?: number;
  recipe?: any;
  status: JobStatus;
  started_at?: string | null;
  finished_at?: string | null;
  root?: StoryNode | null;
}

type NodeRef =
  | { type: 'node'; node: StoryNode; key: string; attempt: number }
  | { type: 'priorAttemptsGroup'; parent: StoryNode; key: string };

function nodeAttempt(node: StoryNode): number {
  return node.attempt ?? 1;
}

function nodeKey(node: StoryNode, attemptOverride?: number): string {
  const attempt = attemptOverride ?? nodeAttempt(node);
  return `${node.path.join('/')}|attempt:${attempt}`;
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
    default:
      return kind;
  }
}

function extractRecipeLabel(recipe: any): string {
  if (!recipe) return '-';
  return recipe.name || recipe.recipe_name || recipe.path || recipe.id || '-';
}

async function fetchStory(projectId: string, jobId: string): Promise<WorkflowStoryResponse> {
  const response = await fetch(
    `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/story`
  );
  if (!response.ok) {
    const errText = await response.text().catch(() => 'Unknown error');
    throw new Error(`${response.status} ${response.statusText}${errText ? ` - ${errText}` : ''}`);
  }
  return response.json();
}

function buildTree(root: StoryNode | null | undefined) {
  const keyToRef = new Map<string, NodeRef>();

  const buildNode = (node: StoryNode, opts?: { forceAttemptBadge?: boolean }): DataNode => {
    const attempt = nodeAttempt(node);
    const key = nodeKey(node, attempt);
    keyToRef.set(key, { type: 'node', node, key, attempt });

    const children: DataNode[] = [];

    if (node.prior_attempts && node.prior_attempts.length > 0) {
      const groupKey = `${key}|priorAttempts`;
      keyToRef.set(groupKey, { type: 'priorAttemptsGroup', parent: node, key: groupKey });
      children.push({
        key: groupKey,
        title: (
          <Text type="secondary">
            Prior attempts ({node.prior_attempts.length})
          </Text>
        ),
        children: node.prior_attempts.map((pa) => {
          const paAttempt = nodeAttempt(pa);
          const paKey = nodeKey(pa, paAttempt);
          keyToRef.set(paKey, { type: 'node', node: pa, key: paKey, attempt: paAttempt });

          return buildNode(pa, { forceAttemptBadge: true });
        }),
      });
    }

    if (node.children && node.children.length > 0) {
      children.push(...node.children.map(buildNode));
    }

    const attemptBadge =
      opts?.forceAttemptBadge ||
      (node.prior_attempts && node.prior_attempts.length > 0) ||
      attempt > 1 ? (
        <Tag style={{ marginInlineStart: 8 }}>Attempt {attempt}</Tag>
      ) : null;

    const artifactCount = node.artifact_keys?.length ?? 0;
    const artifactBadge =
      artifactCount > 0 ? (
        <Tag color="processing" style={{ marginInlineStart: 8 }}>
          Artifacts: {artifactCount}
        </Tag>
      ) : null;

    const d = durationSeconds(node.started_at ?? null, node.finished_at ?? null);
    const durationText = d !== null ? (
      <Text type="secondary" style={{ marginInlineStart: 8 }}>
        {formatDuration(node.started_at ?? null, node.finished_at ?? null)}
      </Text>
    ) : null;

    return {
      key,
      title: (
        <Space size={6}>
          <Text type="secondary">{kindLabel(node.kind)}</Text>
          <Text strong>{node.title}</Text>
          <Tag color={statusTagColor(node.status)}>{node.status.toUpperCase()}</Tag>
          {attemptBadge}
          {artifactBadge}
          {durationText}
        </Space>
      ),
      children,
    };
  };

  const treeData = root ? [buildNode(root)] : [];
  return { treeData, keyToRef };
}

function latestKeyForPathPrefix(pathPrefix: string, keyToRef: Map<string, NodeRef>): string | null {
  let best: { key: string; attempt: number } | null = null;
  const prefix = `${pathPrefix}|attempt:`;

  for (const [key, ref] of keyToRef.entries()) {
    if (ref.type !== 'node') continue;
    if (!key.startsWith(prefix)) continue;
    const attempt = ref.attempt;
    if (!best || attempt > best.attempt) best = { key, attempt };
  }

  return best?.key ?? null;
}

function ancestorKeysForNode(node: StoryNode, keyToRef: Map<string, NodeRef>): string[] {
  const keys: string[] = [];
  for (let i = 1; i < node.path.length; i++) {
    const prefix = node.path.slice(0, i).join('/');
    const k = latestKeyForPathPrefix(prefix, keyToRef);
    if (k) keys.push(k);
  }
  return keys;
}

function unionKeys(a: string[], b: string[]): string[] {
  const set = new Set<string>(a);
  for (const k of b) set.add(k);
  return Array.from(set);
}

interface WorkflowStoryPageProps {
  projectId: string;
}

export default function WorkflowStoryPage({ projectId }: WorkflowStoryPageProps) {
  const { workflowId } = useParams<{ workflowId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [story, setStory] = useState<WorkflowStoryResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const [autoRefresh, setAutoRefresh] = useState(false);
  const [refreshIntervalMs, setRefreshIntervalMs] = useState<number>(5000);
  const refreshTimerRef = useRef<number | null>(null);

  const [selectedArtifact, setSelectedArtifact] = useState<ArtifactKey | null>(null);
  const [artifactContent, setArtifactContent] = useState<string | null>(null);
  const [artifactLoading, setArtifactLoading] = useState(false);
  const [viewMode, setViewMode] = useState<'text' | 'hex'>('text');

  const jobId = workflowId || '';

  const { treeData, keyToRef } = useMemo(() => buildTree(story?.root), [story?.root]);

  const selectedRef = useMemo(() => {
    if (!selectedKey) return null;
    return keyToRef.get(selectedKey) ?? null;
  }, [keyToRef, selectedKey]);

  const selectedNode = selectedRef?.type === 'node' ? selectedRef.node : null;

  const load = async (opts?: { preserveSelection?: boolean }) => {
    if (!workflowId) return;
    setLoading(true);
    setError(null);
    try {
      const data = await fetchStory(projectId, workflowId);
      const { keyToRef: nextKeyToRef } = buildTree(data.root);
      setStory(data);

      if (data.status === 'running') {
        setAutoRefresh((prev) => prev || true);
      }

      const urlPath = searchParams.get('path');
      const urlAttempt = searchParams.get('attempt');
      const urlKey =
        urlPath && urlAttempt
          ? `${urlPath}|attempt:${urlAttempt}`
          : urlPath
          ? `${urlPath}|attempt:1`
          : null;

      const nextSelectedKey =
        urlKey && nextKeyToRef.has(urlKey)
          ? urlKey
          : opts?.preserveSelection && selectedKey && nextKeyToRef.has(selectedKey)
          ? selectedKey
          : data.root
          ? nodeKey(data.root)
          : null;

      if (nextSelectedKey && nextKeyToRef.has(nextSelectedKey)) {
        setSelectedKey(nextSelectedKey);
        const ref = nextKeyToRef.get(nextSelectedKey);
        if (ref?.type === 'node') {
          const ancestors = ancestorKeysForNode(ref.node, nextKeyToRef);
          setExpandedKeys((prev) => unionKeys(prev, ancestors));
        }
      }
    } catch (e) {
      console.error('Failed to load workflow story', e);
      setError(e instanceof Error ? e.message : 'Failed to load workflow story');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, workflowId]);

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
  }, [autoRefresh, refreshIntervalMs, story?.status, projectId, workflowId, selectedKey]);

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
    const attempt = nodeAttempt(node);
    const key = nodeKey(node, attempt);
    setSelectedKey(key);
    const ancestors = ancestorKeysForNode(node, keyToRef);
    setExpandedKeys((prev) => unionKeys(prev, ancestors));
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set('path', node.path.join('/'));
      next.set('attempt', String(attempt));
      return next;
    });
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
          {kindLabel(selectedNode.kind)} · {selectedNode.title}
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

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(`/project/${projectId}/workflows`)}>
              Back to Workflows
            </Button>
            <Title level={2} style={{ margin: 0 }}>
              Workflow Story
            </Title>
            {story?.job_id ? <Text code>{story.job_id.slice(-12)}</Text> : null}
          </Space>
          <Space>
            {workflowId ? (
              <Link to={`/project/${projectId}/workflows/${workflowId}`}>
                <Button icon={<BranchesOutlined />}>Open old workflow detail</Button>
              </Link>
            ) : null}
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
            message="Failed to load workflow story"
            description={error}
            action={
              <Button size="small" onClick={() => load()}>
                Retry
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
            <div style={{ marginTop: 12 }}>
              <Text type="secondary">Recipe metadata</Text>
              <div style={{ marginTop: 8 }}>
                <ReactJson
                  src={story.recipe}
                  collapsed={1}
                  displayDataTypes={false}
                  enableClipboard
                  theme="rjv-default"
                />
              </div>
            </div>
          ) : null}
        </Card>

        <div style={{ display: 'flex', gap: 16, alignItems: 'stretch' }}>
          <Card title="Story" style={{ flex: 1, minWidth: 420 }} bodyStyle={{ padding: 12 }}>
            {!story?.root ? (
              <Empty description="No story available yet" />
            ) : (
              <Tree
                showLine
                treeData={treeData}
                expandedKeys={expandedKeys}
                selectedKeys={selectedKey ? [selectedKey] : []}
                onExpand={(keys) => setExpandedKeys(keys as string[])}
                onSelect={(keys) => {
                  const key = (keys?.[0] as string) || null;
                  if (!key) return;
                  const ref = keyToRef.get(key);
                  if (!ref) return;
                  if (ref.type === 'node') {
                    const ancestors = ancestorKeysForNode(ref.node, keyToRef);
                    setExpandedKeys((prev) => unionKeys(prev, ancestors));
                    setSelectedKey(key);
                    setSearchParams((prev) => {
                      const next = new URLSearchParams(prev);
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

          <Card title={detailsHeader} style={{ flex: 1.2, minWidth: 520 }} bodyStyle={{ padding: 12 }}>
            {!selectedNode ? (
              <Empty description="Select a node from the story tree" />
            ) : (
              <Tabs
                items={[
                  {
                    key: 'overview',
                    label: 'Overview',
                    children: (
                      <Space direction="vertical" style={{ width: '100%' }} size="middle">
                        <Descriptions column={1} bordered size="small">
                          <Descriptions.Item label="Kind">
                            {kindLabel(selectedNode.kind)}
                          </Descriptions.Item>
                          <Descriptions.Item label="Title">
                            {selectedNode.title}
                          </Descriptions.Item>
                          <Descriptions.Item label="Status">
                            <Tag color={statusTagColor(selectedNode.status)}>
                              {selectedNode.status.toUpperCase()}
                            </Tag>
                          </Descriptions.Item>
                          <Descriptions.Item label="Attempt">
                            {nodeAttempt(selectedNode)}
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
                    label: `Retries (${selectedNode.prior_attempts?.length ?? 0})`,
                    children: selectedNode.prior_attempts && selectedNode.prior_attempts.length > 0 ? (
                      <List
                        dataSource={selectedNode.prior_attempts}
                        renderItem={(pa) => (
                          <List.Item
                            actions={[
                              <Button key="inspect" size="small" onClick={() => setSelectedFromNode(pa)}>
                                Inspect
                              </Button>,
                            ]}
                          >
                            <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                              <Space direction="vertical" size={0}>
                                <Text strong>Attempt {nodeAttempt(pa)}</Text>
                                <Text type="secondary">
                                  {formatTimestamp(pa.started_at)} → {formatTimestamp(pa.finished_at)} · {formatDuration(pa.started_at ?? null, pa.finished_at ?? null)}
                                </Text>
                              </Space>
                              <Tag color={statusTagColor(pa.status)}>{pa.status.toUpperCase()}</Tag>
                            </Space>
                          </List.Item>
                        )}
                      />
                    ) : (
                      <Empty description="No prior attempts" />
                    ),
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
    </div>
  );
}
