import InboxPanel, { inboxCount } from './InboxPanel';
import JobLeaseTimeline from './JobLeaseTimeline';
import JobConversation from './JobConversation';
import JobDetailTabs, { jobDetailTab, type JobDetailTab } from './JobDetailTabs';
import NodePayloadView from './NodePayloadView';
import JobRelationships from './JobRelationships';
import { cellLabel } from '../utils/cellPresentation';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';
import ArtifactContents from './ArtifactContents';
import './JobStoryPage.css';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams, useLocation } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  List,
  Modal,
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
import AttemptTabs from './AttemptTabs';
import CancelJobButton from './CancelJobButton';
import DownloadChaptersButton from './DownloadChaptersButton';
import { getJob, getJobInputArtifacts, type InputArtifact } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import { isFinishedJob, jobStatusPresentation, jobStatusValue } from '../utils/jobStatus';
import { goalPath } from './JobWaitLinks';
import { jobFilterSearch } from '../utils/jobFilters';
import { buildStoryTree, storyAttempts, storyNodeName, type ArtifactKey, type StoryNode, type StoryKind, type StoryNodeRef } from '../utils/storyTree';
import { adaptInputFormConfig } from '../utils/formAdapter';
import type { RestartScope } from '@colony2/shared';
import RestartContextPreview from './RestartContextPreview';

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
  restart_scopes?: RestartScope[];
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
  return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(artifact.jobId || jobId)}/tasks/${artifact.taskOrdinal}/artifacts/${encodeURIComponent(artifact.name)}`;
}

function artifactSize(bytes: number): string {
  const unit = Math.min(3, Math.max(0, Math.floor(Math.log2(Math.max(1, bytes)) / 10)));
  return `${unit === 0 ? bytes : (bytes / 1024 ** unit).toFixed(1)} ${['B', 'KB', 'MB', 'GB'][unit]}`;
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
  const { jobId } = useParams<{ jobId: string }>();
  return <JobStoryContent key={JSON.stringify([projectId, jobId])} projectId={projectId} />;
}

function JobStoryContent({ projectId }: JobStoryPageProps) {
  const [activeTreeKey, setActiveTreeKey] = useState<string | null>(null);
  const { jobId: routeJobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const treeReturn = (location.state as { treeReturnTo?: unknown } | null)?.treeReturnTo;
  const treeReturnTo = typeof treeReturn === 'string'
    && treeReturn.startsWith(`/project/${encodeURIComponent(projectId)}/jobs/`)
    && /^\/project\/[^/]+\/jobs\/[^/]+\/tree$/.test(treeReturn) ? treeReturn : undefined;
  const [searchParams, setURLSearchParams] = useSearchParams();
  // Node/tab selections must retain the originating goal/list entry.
  const setSearchParams: typeof setURLSearchParams = (next, options) =>
    setURLSearchParams(next, { state: location.state, ...options });
  const { pendingInputs, refresh: refreshPendingInputs } = useInputActivity();

  const [story, setStory] = useState<JobStoryResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [activeDetailsTab, setActiveDetailsTab] = useState<string>('overview');

  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const [autoRefresh, setAutoRefresh] = useState(true);
  const [refreshIntervalMs, setRefreshIntervalMs] = useState<number>(5000);
  const refreshTimerRef = useRef<number | null>(null);

  const [artifactNavigation, setArtifactNavigation] = useState<NavigationIntent>();
  const [selectedArtifact, setSelectedArtifact] = useState<ArtifactKey | null>(null);
  const artifactOpener = useRef<HTMLElement | null>(null);
  const [artifactOwner, setArtifactOwner] = useState('');
  const [artifactAttachments, setArtifactAttachments] = useState<ArtifactKey[]>([]);

  const [restartOpen, setRestartOpen] = useState(false);
  const [restartSelection, setRestartSelection] = useState<string>();
  const [restartSubmitting, setRestartSubmitting] = useState(false);
  const [jobPatchLines, setJobPatchLines] = useState<PatchLine[]>([]);
  const [scopePatchLines, setScopePatchLines] = useState<ScopePatchLine[]>([]);
  const [applyPatch, setApplyPatch] = useState(false);

  const [pendingInputDetails, setPendingInputDetails] = useState<UserInputDetails | null>(null);
  const [pendingInputLoading, setPendingInputLoading] = useState(false);
  const [pendingInputError, setPendingInputError] = useState<string | null>(null);
  const [pendingInputSubmitting, setPendingInputSubmitting] = useState(false);

  const jobId = routeJobId || '';
  const artifactIdentity = JSON.stringify([projectId, jobId]);
  const activeArtifact = artifactOwner === artifactIdentity ? selectedArtifact : null;

  const [inputInventory, setInputInventory] = useState<{ owner: string; artifacts?: InputArtifact[]; error?: string }>();
  const [inputRetry, setInputRetry] = useState(0);
  const inputs = inputInventory?.owner === artifactIdentity ? inputInventory : undefined;
  useEffect(() => {
    const controller = new AbortController();
    setInputInventory({ owner: artifactIdentity });
    getJobInputArtifacts(projectId, jobId, controller.signal).then(result => {
      if (!controller.signal.aborted) setInputInventory({ owner: artifactIdentity, artifacts: [...result.artifacts].sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : a.taskOrdinal - b.taskOrdinal) });
    }).catch(error => {
      if (!controller.signal.aborted) setInputInventory({ owner: artifactIdentity, error: error instanceof Error ? error.message : String(error) });
    });
    return () => controller.abort();
  }, [projectId, jobId, artifactIdentity, inputRetry]);

  const [historyRevision, setHistoryRevision] = useState(0);
  const historyRefreshKey = useMemo(() => ({ revision: historyRevision, pendingInputs }), [historyRevision, pendingInputs]);
  const activeTab = jobDetailTab(searchParams);
  const stepsActive = useRef(activeTab === 'execution');
  stepsActive.current = activeTab === 'execution';
  const requestId = searchParams.has('occurrenceId') ? searchParams.get('occurrenceId') : searchParams.get('requestId');
  const selectTab = (tab: JobDetailTab) => setSearchParams(previous => {
    const next = new URLSearchParams(previous);
    next.set('tab', tab);
    next.delete('requestId');
    next.delete('occurrenceId');
    next.delete('input');
    return next;
  });
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

  const treeModel = useMemo(
    () => buildTree(activeTab === 'execution' ? story?.root : undefined, { pendingTaskOrdinal: focusTaskOrdinal }),
    [story?.root, focusTaskOrdinal, activeTab]
  );

  const { keyToRef, keyToParent, keyFor, ancestorsByKey, projectionByKey, visibleKeyByKey, childrenByNode, renderNodes } = treeModel;
  const [restartGoalMode, setRestartGoalMode] = useState<'continue' | 'new'>('continue');
  const filterSearch = jobFilterSearch(searchParams);
  const goalsHome = `/project/${encodeURIComponent(projectId)}/goals`;
  const suppliedReturn = location.state?.goalsReturnTo || location.state?.returnTo;
  const returnTo = typeof suppliedReturn === 'string' && [goalsHome, `/project/${encodeURIComponent(projectId)}/jobs`].includes(suppliedReturn.split('?')[0])
    ? suppliedReturn : `${goalsHome}${filterSearch}`;
  const [loadedJob, setLoadedJob] = useState<{
    jobId: string; projectId: string; job?: RecipeJob;
    status: 'loading' | 'loaded' | 'failed'; error?: string;
  }>();
  const metadataGeneration = useRef(0);
  const metadata = loadedJob?.jobId === jobId && loadedJob.projectId === projectId ? loadedJob : undefined;
  const job = metadata?.job;
  const jobTitle = typeof job?.title === 'string' && job.title.trim() ? job.title : undefined;
  const prompt = job?.prompt?.trim() ? job.prompt : undefined;
  const terminalStatus = job ? jobStatusValue(job) : undefined;
  const failed = terminalStatus && ['failed_app', 'failed_system', 'failed_timeout'].includes(terminalStatus);
  const failureDetail = job?.completion_detail?.trim() ? job.completion_detail : undefined;
  const loadMetadata = () => {
    const generation = ++metadataGeneration.current;
    setLoadedJob(previous => ({ projectId, jobId, status: 'loading',
      job: previous?.projectId === projectId && previous.jobId === jobId ? previous.job : undefined }));
    void getJob(projectId, jobId).then(job => {
      if (generation === metadataGeneration.current) setLoadedJob({ projectId, jobId, job, status: 'loaded' });
    }).catch(error => {
      // Metadata failure must not prevent story inspection.
      if (generation === metadataGeneration.current) setLoadedJob({ projectId, jobId, status: 'failed',
        error: error instanceof Error ? error.message : 'Could not load job details' });
    });
  };
  const navigationGoal = typeof location.state?.goalId === 'string' ? location.state.goalId : undefined;
  const currentGoal = navigationGoal || job?.goal_id;
  const overviewPath = currentGoal ? goalPath(projectId, currentGoal) : undefined;
  const savedOverview = location.state?.goalReturnTo;
  const overviewLink = typeof savedOverview === 'string' && savedOverview.split('?')[0] === overviewPath ? savedOverview : overviewPath;


  const selectedRef = useMemo(() => {
    if (!selectedKey) return null;
    return keyToRef.get(selectedKey) ?? null;
  }, [keyToRef, selectedKey]);

  const selectedNode = selectedRef?.type === 'node' ? selectedRef.node : null;
  const selectedAncestors = selectedKey ? ancestorsByKey.get(selectedKey) || [] : [];
  const attempts = storyAttempts(treeModel, selectedKey);
  const latestAttempt = attempts[0];
  const jobRoots = story?.root ? [story.root, ...(story.root.past_attempts || [])] : [];
  const displayedRoot = jobRoots.find(root => (root.job_attempt || 1) === selectedRef?.jobAttempt) || story?.root;
  const treeData = activeTab === 'execution' && displayedRoot ? renderNodes(projectionByKey.get(keyFor.get(displayedRoot)!) || []) : [];
  const isPastJob = !!story?.root && selectedRef?.jobAttempt !== (story.root.job_attempt || 1);
  const operation = selectedNode?.kind === 'op' ? selectedNode : [...selectedAncestors].reverse().find(n => n.kind === 'op');
  const contextNodes = selectedNode ? [...selectedAncestors, ...(childrenByNode.get(selectedNode) || [])] : [];


  const storyGeneration = useRef(0);
  const inFlight = useRef(false);
  const loadStory = async (opts?: { preserveSelection?: boolean }) => {
    if (!jobId || !stepsActive.current || inFlight.current) return;
    inFlight.current = true;
    const generation = ++storyGeneration.current;
    setLoading(true);
    setError(null);
    try {
      const data = await fetchStory(projectId, jobId);
      if (generation !== storyGeneration.current) return;
      const { treeData: nextTreeData, keyToRef: nextKeyToRef, keyToParent: nextKeyToParent, legacyKeyToKey: nextLegacyKeyToKey } =
        buildTree(data.root);
      setStory(data);

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
        setExpandedKeys((prev) => opts?.preserveSelection && !!story
          ? unionKeys(prev, ancestors)
          : Array.from(nextKeyToRef.keys()));
      }
    } catch (e) {
      if (generation !== storyGeneration.current) return;
      console.error('Failed to load job story', e);
      setError(e instanceof Error ? e.message : 'Failed to load job story');
    } finally {
      if (generation === storyGeneration.current) { setLoading(false); inFlight.current = false; }
    }
  };

  useEffect(() => {
    setLoadedJob(undefined);
    setStory(null);
    setSelectedKey(null);
    setError(null);
    setLoading(false);
    setExpandedKeys([]);
    loadMetadata();
    return () => { ++metadataGeneration.current; ++storyGeneration.current; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, jobId]);

  const load = async (opts?: { preserveSelection?: boolean }) => {
    setHistoryRevision(value => value + 1);
    loadMetadata();
    await loadStory(opts);
  };

  useEffect(() => {
    stepsActive.current = activeTab === 'execution';
    if (activeTab === 'execution' && (!story || (autoRefresh && story.status === 'running'))) {
      void loadStory({ preserveSelection: !!story });
    }
    return () => {
      stepsActive.current = false;
      ++storyGeneration.current;
      inFlight.current = false;
    };
    // Cache stays above the inspector; only activation/identity changes trigger retrieval.
  }, [activeTab, projectId, jobId]);

  useEffect(() => {
    const shouldLoadDetails = !!jobId && requestId === null && (isPendingInput || wantInput);
    if (!shouldLoadDetails) {
      setPendingInputDetails(null);
      setPendingInputError(null);
      setPendingInputLoading(false);
      return;
    }

    let cancelled = false;
    setPendingInputDetails(null);
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
  }, [projectId, jobId, isPendingInput, wantInput, pendingInputs, requestId]);

  // Browser history changes the URL without reloading the job. Restore the
  // recorded execution selection while retaining the selected top-level tab.
  useEffect(() => {
    const nodeID = searchParams.get('nodeId');
    const path = searchParams.get('path');
    const legacy = path !== null ? `${path}|attempt:${searchParams.get('attempt') || '1'}` : null;
    const key = nodeID && keyToRef.has(nodeID) ? nodeID
      : treeModel.legacyKeyToKey.get(nodeID || legacy || '');
    if (!key) return;
    setSelectedKey(key);
    setExpandedKeys(previous => unionKeys(previous, ancestorKeysForKey(key, keyToParent)));
  }, [searchParams, keyToRef, keyToParent, treeModel.legacyKeyToKey]);

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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusTaskOrdinal, story?.root, keyToRef, keyToParent]);

  useEffect(() => {
    if (refreshTimerRef.current) {
      window.clearInterval(refreshTimerRef.current);
      refreshTimerRef.current = null;
    }

    if (activeTab === 'execution' && autoRefresh && story?.status === 'running') {
      refreshTimerRef.current = window.setInterval(() => {
        loadStory({ preserveSelection: true });
      }, refreshIntervalMs);
    }

    return () => {
      if (refreshTimerRef.current) {
        window.clearInterval(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, autoRefresh, refreshIntervalMs, story?.status, projectId, jobId, selectedKey]);

  // Conversation completion must keep refreshing even if execution inspection
  // is unavailable or hidden. Metadata polling is independent of story polling.
  useEffect(() => {
    if (!autoRefresh || !job || !terminalStatus || isFinishedJob(job)) return;
    const timer = window.setInterval(loadMetadata, refreshIntervalMs);
    return () => window.clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRefresh, refreshIntervalMs, projectId, jobId, terminalStatus, story?.status]);

  const openArtifact = (artifact: ArtifactKey, attachments = selectedNode?.artifact_keys || []) => {
    artifactOpener.current = document.activeElement as HTMLElement;
    setArtifactNavigation(undefined);
    setArtifactOwner(artifactIdentity);
    setArtifactAttachments(attachments.map(a => ({ ...a })));
    setSelectedArtifact(artifact);
  };
  useEffect(() => { setSelectedArtifact(null); }, [artifactIdentity]);
  useEffect(() => { if (!activeArtifact) artifactOpener.current?.focus(); }, [activeArtifact]);

  const downloadArtifact = (artifact: ArtifactKey) => {
    const url = artifactUrl(projectId, jobId, artifact);
    const link = document.createElement('a');
    link.href = url;
    link.download = artifact.name;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const setSelectedFromNode = (node: StoryNode, preserveTab = false) => {
    const key = Array.from(keyToRef.values()).find((ref) => ref.type === 'node' && ref.node === node)?.key;
    if (!key) return;
    setSelectedKey(key);
    if (!preserveTab) setActiveDetailsTab('overview');
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

  const restartScopes = story?.restart_scopes || [];
  const scopeKey = (scope: RestartScope) => JSON.stringify([scope.scope_id, scope.chapter_ordinal]);
  const restartScope = restartScopes.find(scope => scopeKey(scope) === restartSelection);
  const restartChapterOrdinal = restartScope?.chapter_ordinal;
  const submissionRestart = restartChapterOrdinal === 0;
  const restartScopeID = restartScope && restartScopes.filter(scope => scope.scope_id === restartScope.scope_id).length === 1 ? restartScope.scope_id : undefined;
  const restartable = !!restartScope;

  const resetRestartState = () => {
    setRestartSelection(undefined);
    setApplyPatch(false);
    setJobPatchLines([]);
    setScopePatchLines([]);
  };

  const openRestart = () => {
    resetRestartState();
    setRestartGoalMode(story?.status === 'completed' ? 'new' : 'continue');
    // Compact chapters can contain multiple boundaries. Prefer the recorded scope ID.
    const matches = restartScopes.filter(scope => selectedNode?.restart_scope_id
      ? scope.scope_id === selectedNode.restart_scope_id
      : scope.chapter_ordinal === selectedNode?.restart_chapter_ordinal);
    setRestartSelection(matches.length === 1 ? scopeKey(matches[0]) : undefined);
    setRestartOpen(true);
  };

  const submitRestart = async () => {
    if (!jobId || !restartable) return;

    const contextPatch = applyPatch && !submissionRestart ? buildContextPatch(jobPatchLines, scopePatchLines) : null;

    setRestartSubmitting(true);
    try {
      const payload = { ...(restartScopeID ? { scope_id: restartScopeID } : { chapter_ordinal: restartChapterOrdinal }), goal: { mode: restartGoalMode }, ...(contextPatch ? { patch: contextPatch } : {}) };
      const storageKey = 'cells-v1:' + `cortex:restart:${projectId}:${jobId}`;
      const serialized = JSON.stringify(payload);
      const stored = sessionStorage.getItem(storageKey);
      const previous = stored ? JSON.parse(stored) : null;
      const request = previous?.payload === serialized ? previous.request : { ...payload, request_id: newId() };
      sessionStorage.setItem(storageKey, JSON.stringify({ payload: serialized, request }));
      const response = await fetch(
        `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/restart`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(request),
        }
      );

      if (!response.ok) {
        const errText = await response.text().catch(() => 'Unknown error');
        throw new Error(`${response.status} ${response.statusText}${errText ? ` - ${errText}` : ''}`);
      }

      const data = (await response.json()) as { job_id: string };
      sessionStorage.removeItem(storageKey);
      message.success('Restarted job');
      setRestartOpen(false);
      resetRestartState();
      navigate(`/project/${projectId}/jobs/${data.job_id}/story${filterSearch}`);
    } catch (e) {
      console.error('Restart failed', e);
      message.error(e instanceof Error ? e.message : 'Restart failed');
    } finally {
      setRestartSubmitting(false);
    }
  };

  const selectedTreeKey = selectedNode?.kind === 'recipe' && selectedKey ? selectedKey
    : latestAttempt ? visibleKeyByKey.get(latestAttempt.key) || latestAttempt.key : null;
  const focusStoryTarget = (details: boolean) => {
    const region = document.getElementById('execution-story-tree');
    const selected = region?.querySelector<HTMLElement>('.ant-tree-node-selected');
    if (!details && selected) {
      setActiveTreeKey(selectedTreeKey);
      // rc-tree routes keyboard navigation through its controller input. The
      // activeKey prop makes that controller resume at the selected visible row.
      region?.querySelector<HTMLInputElement>('.ant-tree input')?.focus({ preventScroll: true });
      selected.scrollIntoView({ block: 'start', inline: 'nearest', behavior: 'instant' });
      return;
    }
    const target = details ? document.getElementById('selected-story-details') : region;
    if (!target) return;
    if (details) target.tabIndex = -1;
    target.focus({ preventScroll: true });
    target.scrollIntoView({ block: 'start', inline: 'nearest', behavior: 'instant' });
  };
  const detailsLink = <a className="story-navigation-link" href="#selected-story-details"
    onClick={(event) => { event.preventDefault(); focusStoryTarget(true); }}>View selected details</a>;

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
        <Text type="secondary">Select a node to inspect input/output, artifacts, and attempts.</Text>
      )}
    </Space>
  );

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
    <div className="app-page job-story-page" style={{ padding: 24 }}>
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <div className="job-story-header" data-testid="job-story-header" style={{ display: 'flex', flexWrap: 'wrap', gap: 16, justifyContent: 'space-between', alignItems: 'center', minWidth: 0 }}>
          <Space wrap style={{ minWidth: 0, maxWidth: '100%' }}>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(returnTo)}>
              Back to Goals
            </Button>
            <Title level={2} style={{ margin: 0, overflowWrap: 'anywhere' }}>
              {jobTitle ? `Job Story: ${jobTitle}` : 'Job Story'}
            </Title>
            <Link className="compact-touch-link" to={`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId || '')}/tree`}>Execution tree</Link>
            {treeReturnTo && <Link className="compact-touch-link" to={treeReturnTo}>Return to execution tree</Link>}
            {overviewLink && <Link className="compact-touch-link" to={overviewLink} state={{ goalsReturnTo: returnTo }}>Goal overview</Link>}
          </Space>
          <Space wrap style={{ maxWidth: '100%' }}>
            <DownloadChaptersButton key={JSON.stringify([projectId, jobId])} projectId={projectId} jobId={jobId} />
            {(terminalStatus ? !isFinishedJob(job!) : story?.status === 'running') ? (
              <Space>
                <Text type="secondary">Auto-refresh</Text>
                <Switch aria-label="Auto-refresh" checked={autoRefresh} onChange={setAutoRefresh} />
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
            {requestId === null && <CancelJobButton key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId}
              disabled={!jobId || !!job && !!terminalStatus && isFinishedJob(job)}
              onCancelled={async () => { await Promise.all([load({ preserveSelection: true }), refreshPendingInputs()]); }} />}
          </Space>
        </div>

        {!job && (!metadata || metadata.status === 'loading') && <Text role="status" aria-label="Job details loading">Loading job details...</Text>}
        {metadata?.status === 'failed' && <Alert type="warning" showIcon message="Job details unavailable"
          description={<div style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
            <div>Cell, prompt, and completion information could not be loaded.</div>
            <div>{metadata.error}</div>
          </div>}
          action={<Button size="small" onClick={loadMetadata}>Retry</Button>} />}
        {job && <Tag color={jobStatusPresentation(terminalStatus || 'unknown').color} aria-label="Job status">{jobStatusPresentation(terminalStatus || 'unknown').label}</Tag>}
        {job && <div aria-label="Job cell" style={{ minWidth: 0, overflowWrap: 'anywhere' }}>
          <Text strong>Cell: </Text>{cellLabel(job.cell_name)}
        </div>}
        {job && <JobLeaseTimeline projectId={projectId} jobId={job.job_id} createdAt={job.created_at} />}
        <JobDetailTabs activeTab={activeTab} onChange={selectTab} />
        <section role="tabpanel" id="job-panel-conversation" aria-labelledby="job-tab-conversation" hidden={activeTab !== 'conversation'} tabIndex={0} className="job-detail-panel">
        {job && <section aria-label="Job prompt">
          <Text strong>Prompt</Text>
          <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', marginBottom: 0 }}>{prompt || 'No prompt recorded for this job.'}</Typography.Paragraph>
        </section>}
        <JobConversation onBack={() => setSearchParams(previous => { const next = new URLSearchParams(previous); next.delete('occurrenceId'); next.delete('requestId'); return next; })} tenant={projectId} job={jobId} occurrence={searchParams.get('occurrenceId') ?? undefined}
          requestId={searchParams.get('requestId') ?? undefined} running={!!job && !!terminalStatus && !isFinishedJob(job)}
          refreshKey={historyRefreshKey} onOpen={entry => setSearchParams(previous => {
            const next = new URLSearchParams(previous); next.set('occurrenceId', entry); next.set('tab', 'conversation');
            next.delete('requestId'); next.delete('input'); return next;
          })} />
        {requestId === null && (pendingInputLoading ? (
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
                    next.set('tab', 'conversation');
                    next.delete('requestId');
                    next.delete('occurrenceId');
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
        ) : null)}

        {wantInput && requestId === null && pendingInputDetails && (pendingInputDetails.form.kind === 'review' ? (
          <Card title={pendingInputDetails.form.title || 'Review'} size="small">
            <Link to={`/project/${encodeURIComponent(projectId)}/reviews/${encodeURIComponent(jobId)}`}>Open Review</Link>
          </Card>
        ) : pendingInputForm ? (
          <Card title={pendingInputForm.title || 'Input request'} size="small">
            <InputFormRenderer form={pendingInputForm} onSubmit={submitPendingInput}
              onCancel={() => setSearchParams(previous => { const next = new URLSearchParams(previous); next.delete('input'); return next; })}
              loading={pendingInputSubmitting} />
          </Card>
        ) : null)}
        {job && terminalStatus && (job.store === 'ARCHIVED' || isFinishedJob(job)) && <section aria-label="Job completion">
          <Title level={3}>Completion</Title>
        {failed && <Alert type="error" showIcon
          message={terminalStatus === 'failed_timeout' ? 'Job timed out' : 'Job failed'}
          description={<div style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{failureDetail || 'No failure details recorded for this job.'}</div>} />}
          {!failed && <><Text strong>{jobStatusPresentation(terminalStatus || 'unknown').label}</Text>
            <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{failureDetail || 'No completion details recorded for this job.'}</Typography.Paragraph></>}
        </section>}
        </section>
        <section role="tabpanel" id="job-panel-relationships" aria-labelledby="job-tab-relationships" hidden={activeTab !== 'relationships'} tabIndex={0} className="job-detail-panel">
        {job && <JobRelationships projectId={projectId} jobId={jobId!} job={job} />}

        </section>
        <section role="tabpanel" id="job-panel-summary" aria-labelledby="job-tab-summary" hidden={activeTab !== 'summary'} tabIndex={0} className="job-detail-panel">
        <details className="relationship-disclosure story-summary-disclosure" open>
          <summary>Run Summary</summary>
          {!story && <Alert type="info" message="Execution timing and recipe metadata load when you open Steps." action={<Button onClick={() => selectTab('execution')}>Open Steps to load execution timing and recipe metadata</Button>} />}
          <Card size="small" variant="borderless">
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="Status">
                {story?.status ? (
                  <Tag color={statusTagColor(story.status)}>{story.status.toUpperCase()}</Tag>
                ) : (
                  terminalStatus || '-'
                )}
              </Descriptions.Item>
              <Descriptions.Item label="Recipe">
                {story ? extractRecipeLabel(story.recipe) : job?.recipe || '-'}
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
              <div>Job ID: <Text code>{jobId}</Text></div>
                <div style={{ marginTop: 8 }}>
                  <div role="region" aria-label="JSON data" className="technical-region" tabIndex={0}><ReactJson
                    src={story.recipe}
                    collapsed={1}
                    displayDataTypes={false}
                    enableClipboard
                    theme="rjv-default"
                  /></div>
                </div>
              </details>
            ) : null}
          </Card>
        </details>

        </section>
        <section role="tabpanel" id="job-panel-input-artifacts" aria-labelledby="job-tab-input-artifacts" hidden={activeTab !== 'input-artifacts'} tabIndex={0} className="job-detail-panel">
        <details className="relationship-disclosure input-artifacts-disclosure" open>
          <summary>{`Input artifacts${inputs?.artifacts ? ` (${inputs.artifacts.length})` : ''}`}</summary>
          <Card size="small" variant="borderless" className="input-artifacts">
            {inputs?.error ? <Alert type="error" message="Unable to load input artifacts" description={inputs.error} action={<Button onClick={() => setInputRetry(value => value + 1)}>Retry</Button>} />
              : !inputs?.artifacts ? <Text>Loading input artifacts</Text>
              : inputs.artifacts.length === 0 ? <Text>No input artifacts</Text>
              : <List dataSource={inputs.artifacts} renderItem={artifact => (
                <List.Item key={JSON.stringify([projectId, artifact.jobId, artifact.taskOrdinal, artifact.name])}>
                  <Button type="link" className="input-artifact-name" onClick={() => openArtifact(artifact, inputs.artifacts)}>{artifact.name}</Button>
                  <Space><Text type="secondary">{artifactSize(artifact.sizeBytes)}</Text>
                    <Button aria-label={`Download ${artifact.name}`} icon={<DownloadOutlined />} onClick={() => downloadArtifact(artifact)}>Download</Button></Space>
                </List.Item>
              )} />}
          </Card>
        </details>

        </section>
        <section role="tabpanel" id="job-panel-execution" aria-labelledby="job-tab-execution" hidden={activeTab !== 'execution'} tabIndex={0} className="job-detail-panel">
        {activeTab === 'execution' && <>
        {loading && !story && <Spin size="large" />}

        {error ? (
          <Alert
            type="error"
            showIcon
            message="Failed to load job story"
            description={error}
            action={
              <Button size="small" onClick={() => loadStory({ preserveSelection: true })}>
                Retry
              </Button>
            }
          />
        ) : null}

        {detailsLink}
        <div className="story-panels">
          <Card title={isPastJob ? `Story · job attempt ${selectedRef?.jobAttempt}` : 'Story'} className="story-tree-panel" styles={{ body: { padding: 12 } }}
            extra={<Space>
              {!!story?.root?.past_attempts?.length && <Button size="small" onClick={() => {
                setSelectedFromNode(displayedRoot || story.root!);
              }}>Job attempts</Button>}
              <Button size="small" onClick={() => setExpandedKeys(Array.from(keyToParent.keys()))}>Expand all</Button>
              <Button size="small" onClick={() => setExpandedKeys([])}>Collapse all</Button>
            </Space>}>
            <div role="region" aria-label="Execution story tree" id="execution-story-tree" className="story-tree-region" tabIndex={0}>
              {!story?.root ? (
                <Empty description={story && story.status !== 'running'
                  ? 'The story API returned no steps for this finished job.'
                  : 'No story available yet'} />
              ) : (
                <Tree
                  activeKey={activeTreeKey}
                  onActiveChange={(key) => setActiveTreeKey(String(key))}
                  onFocus={() => {
                    const selected = document.querySelector('#execution-story-tree .ant-tree-node-selected');
                    if (selected) {
                      setActiveTreeKey(selectedTreeKey);
                      selected.scrollIntoView({ block: 'start', inline: 'nearest', behavior: 'instant' });
                    }
                  }}
                  showLine
                  treeData={treeData}
                  expandedKeys={expandedKeys}
                  selectedKeys={selectedNode?.kind === 'recipe' && selectedKey ? [selectedKey]
                    : latestAttempt ? [visibleKeyByKey.get(latestAttempt.key) || latestAttempt.key] : []}
                  onExpand={(keys) => setExpandedKeys(keys as string[])}
                  onSelect={(_, info) => {
                    const key = String(info.node.key);
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
            </div>
            {detailsLink}
          </Card>

          <Card title={detailsHeader} id="selected-story-details" tabIndex={-1} className="story-details-panel"
            extra={<a className="story-navigation-link" href="#execution-story-tree"
              onClick={(event) => { event.preventDefault(); focusStoryTarget(false); }}>Back to selected node</a>}
            styles={{ body: { padding: 12, overflowWrap: 'anywhere' } }}>
            {!selectedNode ? (
              <Empty description="Select a node from the story tree" />
            ) : (
              <AttemptTabs
                attempts={attempts.map(ref => ({ key: ref.key, number: selectedNode.kind === 'recipe' ? ref.jobAttempt : ref.attempt }))}
                activeKey={selectedKey}
                kind={selectedNode.kind === 'recipe' ? 'Job attempt' : 'Attempt'}
                onChange={key => {
                  const ref = keyToRef.get(key);
                  if (ref) setSelectedFromNode(ref.node, true);
                }}
              >
              <Tabs
                activeKey={activeDetailsTab}
                onChange={setActiveDetailsTab}
                items={[
                  {
                    key: 'overview',
                    label: 'Overview',
                    children: (
                      <Space direction="vertical" style={{ width: '100%' }} size="middle">
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

                        {restartScopes.length > 0 ? (
                          <Card size="small" title="Restart">
                            <Space direction="vertical" style={{ width: '100%' }}>
                              <Text type="secondary">
                                Create a new job from a recorded restart point.
                              </Text>
                              <Button type="primary" onClick={openRestart}>
                                Restart job
                              </Button>
                            </Space>
                          </Card>
                        ) : null}

                        {selectedNode.kind === 'contextPatch' ? (
                          <Card title="Applied Context Patch" size="small">
                            {selectedNode.output ? (
                              <div role="region" aria-label="JSON data" className="technical-region" tabIndex={0}><ReactJson
                                src={selectedNode.output as any}
                                collapsed={2}
                                displayDataTypes={false}
                                enableClipboard
                                theme="rjv-default"
                              /></div>
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
                    children: <NodePayloadView value={selectedNode.input} label="Input"
                      identity={JSON.stringify([projectId, jobId, selectedKey, selectedRef?.jobAttempt, selectedNode.attempt, 'input'])} />,
                  },
                  {
                    key: 'output',
                    label: 'Output',
                    children: <NodePayloadView value={selectedNode.output} label="Output"
                      identity={JSON.stringify([projectId, jobId, selectedKey, selectedRef?.jobAttempt, selectedNode.attempt, 'output'])} />,
                  },
                  {
                    key: 'inbox',
                    label: `Inbox${loading ? '' : inboxCount(selectedNode)}`,
                    children: <InboxPanel key={JSON.stringify([projectId, jobId, selectedKey, selectedRef?.jobAttempt, selectedNode.attempt, storyGeneration.current])}
                      loading={loading} node={selectedNode} projectId={projectId} jobId={jobId} jobAttempt={selectedRef?.jobAttempt || 1} retry={() => load({preserveSelection:true})} />,
                  },
                  {
                    key: 'artifacts',
                    label: `Outbox (${selectedNode.artifact_keys?.length ?? 0})`,
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
                                      className="compact-touch-link"
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

                ]}
              />
              </AttemptTabs>
            )}
          </Card>
        </div>
        </>}
        </section>
      </Space>

      {activeArtifact && <Modal
        title={activeArtifact?.name || 'Artifact Viewer'}
        open={activeArtifact !== null}
        onCancel={() => setSelectedArtifact(null)}
        width={1000}
        className="story-artifact-modal"
        footer={[
          <Button key="download" icon={<DownloadOutlined />} onClick={() => activeArtifact && downloadArtifact(activeArtifact)}>Download</Button>,
          <Button key="close" onClick={() => setSelectedArtifact(null)}>Close</Button>,
        ]}
      >
        {activeArtifact && <ArtifactContents
          key={JSON.stringify([projectId, activeArtifact.jobId || jobId, activeArtifact.taskOrdinal, activeArtifact.name])}
          url={artifactUrl(projectId, jobId, activeArtifact)} name={activeArtifact.name}
          navigationIntent={artifactNavigation}
          navigation={{
            resolve: reference => resolveArtifactLink(reference, activeArtifact.name, artifactAttachments.map(a => ({key: JSON.stringify([a.jobId || jobId, a.taskOrdinal, a.name]), path: a.name}))),
            open: (key, fragment) => {
              if (artifactOwner !== artifactIdentity) return false;
              const target = artifactAttachments.find(a => JSON.stringify([a.jobId || jobId, a.taskOrdinal, a.name]) === key);
              if (!target) return false;
              setSelectedArtifact(target);
              setArtifactNavigation(previous => ({token: (previous?.token || 0) + 1, fragment}));
              return true;
            },
          }}
          resolveAttachment={reference => {
            const resolution = resolveArtifactLink(reference, activeArtifact.name, artifactAttachments.map(a => ({
              key: JSON.stringify([a.jobId, a.taskOrdinal, a.name]), path: a.name, aliases: [a.name],
            })));
            if (resolution.kind !== 'target') return undefined;
            const a = artifactAttachments.find(a => JSON.stringify([a.jobId, a.taskOrdinal, a.name]) === resolution.key);
            if (!a) return undefined;
            const key = resolution.key;
            return { key, name: a.name, url: artifactUrl(projectId, jobId, a) };
          }}
        />}
      </Modal>}

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
                  {submissionRestart
                    ? 'Restart from the original submission and inputs. Setup and recipe resolution run again, so floating recipe references may resolve to a newer version. The source job is not cancelled.'
                    : 'Choose where to resume execution. Completed work before that point is retained. The source job is not cancelled. A new patch replaces any previous patch at this same restart point.'}
                </div>
              }
            />

            {!restartable && <Alert type="info" showIcon
              message={selectedNode.restart_unavailable_reason || 'No recorded restart point is available for this node.'}
              description="Choose a restart point below." />}
            {restartChapterOrdinal === selectedNode.restart_chapter_ordinal && selectedNode.restart_relationship === 'containing' &&
              <Alert type="info" showIcon message="This restarts the containing scope, including its work before this task." />}
            {restartChapterOrdinal === selectedNode.restart_chapter_ordinal && selectedNode.restart_relationship === 'body' &&
              <Text type="secondary">Restart from the recipe’s executable body.</Text>}
            <Select
              aria-label="Restart point"
              placeholder="Choose a restart point"
              style={{ width: '100%' }}
              value={restartSelection}
              onChange={setRestartSelection}
              options={restartScopes.map(scope => ({
                value: scopeKey(scope),
                label: `${scope.title || scope.path?.join(" / ") || "Restart point"} · Chapter ${scope.chapter_ordinal} · ${formatTimestamp(scope.started_at)}`,
              }))}
              showSearch
              optionFilterProp="label"
            />

            <Select aria-label="Restart goal" value={restartGoalMode} onChange={setRestartGoalMode} style={{ width: '100%' }}
              options={[{ value: 'continue', label: 'Continue this goal' }, { value: 'new', label: 'Start a new goal' }]} />
            <Card title="Context Patching" size="small">
              <Space direction="vertical" style={{ width: '100%' }} size="middle">
                <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                  <Text type="secondary">
                    Apply additional context over the existing context before resuming execution.
                  </Text>
                  <Space>
                    <Text type="secondary">Enable</Text>
                    <Switch checked={applyPatch && !submissionRestart} disabled={submissionRestart} onChange={setApplyPatch} />
                  </Space>
                </Space>

                {submissionRestart ? (
                  <Text type="secondary">Submission context is read-only. No context patch will be applied.</Text>
                ) : !applyPatch ? (
                  <Text type="secondary">No context patch will be applied.</Text>
                ) : (
                  <Space direction="vertical" style={{ width: '100%' }} size="large">
                    <RestartContextPreview projectId={projectId} jobId={jobId} scopeId={restartScopeID} chapterOrdinal={restartable && !restartScopeID ? restartChapterOrdinal : undefined} />
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

                    <Card size="small" title="Patch payload">
                      <div role="region" aria-label="JSON data" className="technical-region" tabIndex={0}><ReactJson
                        src={buildContextPatch(jobPatchLines, scopePatchLines) ?? {}}
                        collapsed={2}
                        displayDataTypes={false}
                        enableClipboard
                        theme="rjv-default"
                      /></div>
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
