import { hasThreadInput, ThreadSnapshotContext, useThreadInput, type ThreadSnapshot } from '../hooks/useThreadInput';
import { object, valueText } from './conversationPresentation';
import { messagePreview, readInboxSort, threadTitle } from '../utils/jobInbox';
import { useInboxReadReceipt, useOptionalJobInbox } from '../hooks/useJobInbox';
import { useSavedReviewDrafts } from '../hooks/useReviewDraft';
import { reviewThreadPath } from '../utils/reviewDocumentLocation';
import { InputRequestReader } from './InputDetailPage';
import ReviewV2 from './ReviewV2';
import { dagReturnPath } from '../executionTree/navigation';
import { TreeWorkspace } from '../executionTree/TreePage';
import InboxPanel, { inboxCount } from './InboxPanel';
import JobLeaseTimeline from './JobLeaseTimeline';
import { launchPath } from '../runners/paths';
import JobCrashConcern from './JobCrashConcern';
import JobConversation from './JobConversation';
import ReviewDocuments, { type ReaderDocument } from './ReviewDocuments';
import JobEnvironment from './JobEnvironment';
import JobDetailTabs, { jobDetailTab, type JobDetailTab } from './JobDetailTabs';
import NodePayloadView from './NodePayloadView';
import JobRelationships from './JobRelationships';
import { cellLabel } from '../utils/cellPresentation';
import { resolveArtifactLink, type NavigationIntent } from './review/artifactLinks';
import ArtifactContents from './ArtifactContents';
import './JobStoryPage.css';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  DownloadOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import ReactJson from 'react-json-view';
import { API_BASE, inputActivityService, type FormResponse, useInputActivity } from '@colony2/shared';
import InputFormRenderer from './InputFormRenderer';
import AttemptTabs from './AttemptTabs';
import CancelJobButton from './CancelJobButton';
import JobActionDialog from './JobActionDialog';
import GoalPrompt from './GoalPrompt';
import { getJob, getJobInputArtifacts, storedDocumentURL, type InputArtifact } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import { canArchiveImmediately, isFinishedJob, confirmsRecoveryCompletion, filterStatuses, jobStatus, jobStatusValue } from '../utils/jobStatus';
import { buildStoryTree, storyAttempts, storyNodeName, type ArtifactKey as StoredArtifactKey, type StoryNode, type StoryKind, type StoryNodeRef } from '../utils/storyTree';
import { adaptInputFormConfig } from '../utils/formAdapter';
import type { RestartScope } from '@colony2/shared';
import RestartContextPreview from './RestartContextPreview';
import { buildContextPatch, type PatchLine, type ScopePatchLine } from '../utils/restartPatch';
import { stringifyJobJSON, parseJobJSON, type RestartContext } from '@colony2/shared';

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
  launch_ids_by_ordinal?: Record<string,string>;
  input_artifacts?: InputArtifact[];
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
    running: filterStatuses.ACTIVE.color,
    completed: 'green',
    succeeded: filterStatuses.success.color,
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

type ArtifactKey = StoredArtifactKey & { id?: string; storedKey?: { jobId: string; taskOrdinal: number; name: string }; contentBinding?: string; inputOrdinal?: number; inputJobId?: string };
function sortInputArtifacts(rows: InputArtifact[]) {
  return [...rows].sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : (a.taskOrdinal ?? 0) - (b.taskOrdinal ?? 0));
}
function inputViewerArtifact(a: InputArtifact, jobId: string): ArtifactKey {
  return { name: a.name, taskOrdinal: a.taskOrdinal ?? 0, jobId: a.jobId || jobId, id: a.id, storedKey: 'storedKey' in a ? a.storedKey : undefined, contentBinding: a.contentBinding, inputJobId: jobId, inputOrdinal: a.inputOrdinal };
}
function artifactRowKey(a: ArtifactKey, jobId: string): string {
  return a.id || JSON.stringify([a.jobId || jobId, a.taskOrdinal, a.name]);
}
function artifactUrl(projectId: string, jobId: string, artifact: ArtifactKey): string {
  if (artifact.storedKey) return storedDocumentURL(projectId, artifact.storedKey);
  if (artifact.contentBinding !== undefined) return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(artifact.inputJobId || jobId)}/input-document?binding=${encodeURIComponent(artifact.contentBinding)}${artifact.inputOrdinal === undefined ? '' : `&ordinal=${artifact.inputOrdinal}`}`;
  return `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(artifact.jobId || jobId)}/tasks/${artifact.taskOrdinal}/artifacts/${encodeURIComponent(artifact.name)}`;
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
  const inbox = useOptionalJobInbox();
  const [activeTreeKey, setActiveTreeKey] = useState<string | null>(null);
  const { jobId: routeJobId } = useParams<{ jobId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const treeReturn = (location.state as { treeReturnTo?: unknown } | null)?.treeReturnTo;
  const treeReturnTo = dagReturnPath(treeReturn, projectId);
  const [searchParams, setURLSearchParams] = useSearchParams();
  // Node/tab selections must retain the originating goal/list entry.
  const setSearchParams: typeof setURLSearchParams = (next, options) =>
    setURLSearchParams(next, { state: location.state, ...options });
  const { pendingInputs, discoveryStatus, currentProjectId, refresh: refreshPendingInputs, setCurrentProjectId } = useInputActivity();

  useEffect(() => {setCurrentProjectId(projectId);}, [projectId, setCurrentProjectId]);

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
  const [restartNode, setRestartNode] = useState<StoryNode | null>(null);
  const [archiveTarget, setArchiveTarget] = useState<{ job?: RecipeJob; immediate: boolean }>();
  const alive = useRef(true);
  const restartBusy = useRef(false);
  const restartGoalInitialized = useRef(false);
  const restartOpener = useRef<HTMLElement | null>(null);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const [restartSelection, setRestartSelection] = useState<string>();
  const [restartSubmitting, setRestartSubmitting] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, { job: PatchLine[]; scope: ScopePatchLine[] }>>({});
  const [inspection, setInspection] = useState<{ key: string; snapshot?: RestartContext }>();
  const [applyPatch, setApplyPatch] = useState(false);

  const [pendingInputSubmitting, setPendingInputSubmitting] = useState(false);

  const jobId = routeJobId || '';
  const storageKey = `json-patch-v2:cortex:restart:${projectId}:${jobId}`;
  type SavedRestart = { payload: string; request: unknown };
  const [savedRestart, setSavedRestart] = useState<SavedRestart | null>(() => {
    try {
      const value = sessionStorage.getItem(storageKey);
      const saved = value ? parseJobJSON(value) as SavedRestart : null;
      return saved && typeof saved.payload === 'string' && saved.request && typeof saved.request === 'object' ? saved : null;
    } catch { return null; }
  });
  const [restartError, setRestartError] = useState<string>();
  const [cancelSource, setCancelSource] = useState(false);
  const [cancellationBusy, setCancellationBusy] = useState(false);
  const [cancellationResult, setCancellationResult] = useState<string>();
  const [cancellationError, setCancellationError] = useState<string>();
  const artifactIdentity = JSON.stringify([projectId, jobId]);
  const activeArtifact = artifactOwner === artifactIdentity ? selectedArtifact : null;

  const activeTab = jobDetailTab(searchParams);
  const [loadedThread, setLoadedThread] = useState<string>();
  const threadVisited = loadedThread === artifactIdentity;
  useEffect(() => { if (activeTab === 'conversation') setLoadedThread(artifactIdentity); }, [activeTab, artifactIdentity]);
  const [snapshot, setSnapshot] = useState<ThreadSnapshot>();
  const [inputInventory, setInputInventory] = useState<{ owner: string; artifacts?: InputArtifact[]; error?: string }>();
  const [inputRetry, setInputRetry] = useState(0);
  const [openedOriginal, setOpenedOriginal] = useState<string>();
  const openOriginal = useCallback(() => setOpenedOriginal(artifactIdentity), [artifactIdentity]);
  const missingThreadInventory = snapshot?.page && 'current_input' in snapshot.page && !snapshot.page.input_artifacts;
  const inputs = missingThreadInventory ? {error:'The conversation could not read the input attachments.', artifacts:undefined}
    : inputInventory?.owner === artifactIdentity ? inputInventory : undefined;
  const [inventoryObserved, setInventoryObserved] = useState<string>();
  const promptInventory = useMemo(() => inputs?.artifacts?.filter(a => a.inputOrdinal === 0), [inputs?.artifacts]);
  const receiveInventory = useCallback((rows?: InputArtifact[]) => {
    setInventoryObserved(artifactIdentity);
    if (rows) setInputInventory({ owner: artifactIdentity, artifacts: sortInputArtifacts(rows) });
  }, [artifactIdentity]);
  const fallbackInputInventory = !missingThreadInventory && openedOriginal === artifactIdentity && inventoryObserved === artifactIdentity && !inputs?.artifacts;
  useEffect(() => {
    // Older history responses can omit the inventory. Read it once for this thread.
    if (!fallbackInputInventory) return;
    const controller = new AbortController();
    setInputInventory({ owner: artifactIdentity });
    getJobInputArtifacts(projectId, jobId, controller.signal).then(result => {
      if (!controller.signal.aborted) setInputInventory({ owner: artifactIdentity, artifacts: sortInputArtifacts(result.artifacts) });
    }).catch(error => {
      if (!controller.signal.aborted) setInputInventory({ owner: artifactIdentity, error: error instanceof Error ? error.message : String(error) });
    });
    return () => controller.abort();
  }, [projectId, jobId, artifactIdentity, inputRetry, fallbackInputInventory]);

  const [historyRevision, setHistoryRevision] = useState(0);
  const pendingHistoryRevision = stringifyJobJSON(pendingInputs.filter(input => input.id === jobId));
  const lastPending = useRef<string>();
  useEffect(() => {
    if (discoveryStatus !== 'current' || currentProjectId !== projectId) return;
    if (lastPending.current !== undefined && lastPending.current !== pendingHistoryRevision) {
      setHistoryRevision(revision => revision + 1);
    }
    lastPending.current = pendingHistoryRevision;
  }, [currentProjectId, projectId, discoveryStatus, pendingHistoryRevision]);
  const dagIdentity = JSON.stringify([projectId, jobId]);
  const [dagVisited, setDagVisited] = useState<string>();
  useEffect(() => { if (activeTab === 'dag') setDagVisited(dagIdentity); }, [activeTab, dagIdentity]);
  const storyDemand = activeTab === 'execution' || restartOpen;
  const storyWanted = useRef(storyDemand);
  storyWanted.current = storyDemand;
  const threadSelection = useRef<{ owner: string; params: URLSearchParams }>();
  if (activeTab === 'conversation') threadSelection.current = { owner: artifactIdentity, params: searchParams };
  const savedThreadParams = threadSelection.current?.owner === artifactIdentity ? threadSelection.current.params : undefined;
  const threadParams = activeTab === 'conversation' ? searchParams : savedThreadParams ?? searchParams;
  const threadRequestId = threadParams.get('occurrenceId') ?? threadParams.get('requestId');
  const requestId = searchParams.get('occurrenceId') ?? searchParams.get('requestId');
  const selectTab = (tab: JobDetailTab) => setSearchParams(previous => {
    const next = new URLSearchParams(previous);
    next.set('tab', tab);
    for (const key of ['requestId', 'occurrenceId', 'input']) {
      const value = tab === 'conversation' ? savedThreadParams?.get(key) : null;
      if (value != null) next.set(key, value);
      else next.delete(key);
    }
    return next;
  });
  const wantInput = searchParams.get('input') === '1';
  const threadWantsInput = threadParams.get('input') === '1';
  const taskOrdinalParamRaw = searchParams.get('taskOrdinal');
  const taskOrdinalParam = taskOrdinalParamRaw ? Number(taskOrdinalParamRaw) : null;

  const refreshThread = useCallback(async () => setHistoryRevision(value => value + 1), []);
  const loadingThread = useMemo<ThreadSnapshot>(() => ({refresh:refreshThread}), [refreshThread]);
  const thread = activeTab === 'conversation' || threadVisited ? snapshot ?? loadingThread : undefined;
  const unified = hasThreadInput(thread);
  const selectedInput = useThreadInput(projectId, jobId, (activeTab !== 'runtime' || threadVisited) && requestId === null, thread);
  const isPendingInput = !!selectedInput.version;
  const pendingInputDetails = selectedInput.details ?? (selectedInput.discovery.status !== 'current' || wantInput ? selectedInput.readableDetails : undefined);
  const pendingForm = pendingInputDetails?.form;
  const pendingIsReview = pendingForm?.kind === 'review' || object(pendingForm?.presentation).type === 'review/v2';
  const reviewRequest = object(pendingForm?.request);
  const currentReviewId = pendingForm?.presentation?.type === 'review/v2' && typeof reviewRequest.review_id === 'string'
    ? reviewRequest.review_id : undefined;
  const threadReviewId = threadParams.get('review_id') ?? currentReviewId;
  const pendingTitle = pendingForm?.title?.trim() || valueText(reviewRequest.title ?? '').trim();
  const pendingLabel = messagePreview(pendingTitle || pendingForm?.question || valueText(reviewRequest.question ?? ''));
  const pendingInputError = selectedInput.error;
  const pendingInputLoading = selectedInput.loading || (wantInput && selectedInput.discovery.status === 'loading');

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
  const goalsHome = `/project/${encodeURIComponent(projectId)}/jobs`;
  const suppliedReturn = location.state?.inboxReturnTo || location.state?.goalsReturnTo || location.state?.returnTo;
  const inboxQuery = new URLSearchParams([...searchParams].filter(([key]) => ['inbox', 'cellId', 'q'].includes(key)));
  const inboxSort = readInboxSort(searchParams.get('sort'));
  if (inboxSort) inboxQuery.set('sort', inboxSort);
  const filterSearch = inboxQuery.size ? '?' + inboxQuery : '';
  const returnTo = typeof suppliedReturn === 'string' && suppliedReturn.split('?')[0] === goalsHome
    ? suppliedReturn : `${goalsHome}${inboxQuery.size ? '?' + inboxQuery : ''}`;
  const [loadedJob, setLoadedJob] = useState<{
    jobId: string; projectId: string; job?: RecipeJob; observedJob?: RecipeJob;
    status: 'loading' | 'loaded' | 'failed'; error?: string;
  }>();
  const metadataGeneration = useRef(0);
  const crashMutationBusy = useRef(false);
  const separateMetadata = loadedJob?.jobId === jobId && loadedJob.projectId === projectId ? loadedJob : undefined;
  const metadata = activeTab === 'conversation' && unified ? {
    status: snapshot?.error ? 'failed' as const : snapshot?.page ? 'loaded' as const : 'loading' as const,
    job: snapshot?.page?.activity?.value, observedJob:snapshot?.page?.activity?.value, error:snapshot?.error,
  } : separateMetadata;
  const job = metadata?.job;
  useInboxReadReceipt(projectId, metadata?.status === 'loaded' ? job : undefined);
  const activityJob = job ?? metadata?.observedJob;
  const activityStatus = activityJob ? jobStatusValue(activityJob) : undefined;
  const prompt = job?.prompt?.trim() ? job.prompt : undefined;
  const hasDraft = useSavedReviewDrafts(projectId).some(draft=>draft.job===jobId);
  const submittedBy = job?.metadata?.cortex_submitted_by;
  const sender = typeof submittedBy === 'string' && submittedBy.trim() ? submittedBy : 'Unknown';
  const terminalStatus = job ? jobStatusValue(job) : undefined;
  const pendingReview = isPendingInput || (currentProjectId === projectId && pendingInputs.some(input => input.id === jobId));
  const displayStatus = job && jobStatus(job, pendingReview);
  const loadMetadata = async (force = false): Promise<RecipeJob | undefined> => {
    if (activeTab === 'conversation' && unified) {
      if (snapshot?.page || snapshot?.error) await snapshot.refresh();
      return snapshot?.page?.activity?.value;
    }
    if (crashMutationBusy.current && !force) return;
    const generation = ++metadataGeneration.current;
    setLoadedJob(previous => ({ projectId, jobId, status: 'loading',
      job: previous?.projectId === projectId && previous.jobId === jobId ? previous.job : undefined,
      observedJob: previous?.projectId === projectId && previous.jobId === jobId ? previous.job ?? previous.observedJob : undefined }));
    try {
      const detail = await getJob(projectId, jobId);
      if (!alive.current || generation !== metadataGeneration.current) return;
      // getJob is an exact read; reject a mismatched response as unavailable.
      if (detail.job_id !== jobId || (detail.tenant_id && detail.tenant_id !== projectId)) throw new Error('Job detail identity does not match the selected source');
      setLoadedJob({ projectId, jobId, job: detail, observedJob: detail, status: 'loaded' });
      return detail;
    } catch (error) {
      if (alive.current && generation === metadataGeneration.current) setLoadedJob(previous => ({ projectId, jobId, status: 'failed',
        observedJob: previous?.projectId === projectId && previous.jobId === jobId ? previous.observedJob : undefined,
        error: error instanceof Error ? error.message : 'Could not load job details' }));
    }
  };
  const currentGoal = job?.goal_id;
  const sourceFinished = metadata?.status === 'loaded' && confirmsRecoveryCompletion(job);



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
  const nodeLaunch = selectedNode?.task_ordinal != null ? story?.launch_ids_by_ordinal?.[String(selectedNode.task_ordinal)] : undefined;
  const isPastJob = !!story?.root && selectedRef?.jobAttempt !== (story.root.job_attempt || 1);
  const operation = selectedNode?.kind === 'op' ? selectedNode : [...selectedAncestors].reverse().find(n => n.kind === 'op');
  const contextNodes = selectedNode ? [...selectedAncestors, ...(childrenByNode.get(selectedNode) || [])] : [];


  const storyGeneration = useRef(0);
  const inFlight = useRef(false);
  const loadStory = async () => {
    if (!jobId || !storyWanted.current || inFlight.current) return;
    inFlight.current = true;
    const generation = ++storyGeneration.current;
    setLoading(true);
    setError(null);
    try {
      const data = await fetchStory(projectId, jobId);
      if (generation !== storyGeneration.current) return;
      setStory(data);
      if (data.input_artifacts) receiveInventory(data.input_artifacts);
    } catch (e) {
      if (generation !== storyGeneration.current) return;
      console.error('Failed to load job story', e);
      setError(e instanceof Error ? e.message : 'Failed to load job story');
    } finally {
      if (generation === storyGeneration.current) { setLoading(false); inFlight.current = false; }
    }
  };

  useEffect(() => {
    crashMutationBusy.current = false;
    setLoadedJob(undefined);
    setStory(null);
    setSelectedKey(null);
    setError(null);
    setLoading(false);
    setExpandedKeys([]);
    return () => { ++metadataGeneration.current; ++storyGeneration.current; inFlight.current = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, jobId]);

  const needsMetadata = activeTab !== 'conversation' || !unified;
  useEffect(() => { if (needsMetadata) void loadMetadata(); }, [needsMetadata, projectId, jobId]);
  const previousPendingInput = useRef(isPendingInput);
  useEffect(() => {
    if (needsMetadata && previousPendingInput.current !== isPendingInput) loadMetadata();
    previousPendingInput.current = isPendingInput;
    // Input activity can change scheduler state without changing retained history.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isPendingInput]);

  const load = async () => {
    setHistoryRevision(value => value + 1);
    if (needsMetadata) loadMetadata();
    if (storyDemand) await loadStory();
  };

  useEffect(() => {
    if (!storyDemand) {
      ++storyGeneration.current;
      inFlight.current = false;
      setLoading(false);
    } else if (!story || (activeTab === 'execution' && autoRefresh && story.status === 'running')) {
      void loadStory();
    }
    // Losing one consumer keeps the other consumer's request alive.
  }, [storyDemand, activeTab]);

  useEffect(() => {
    if (activeTab !== 'execution' || !story) return;
    const { treeData: nextTreeData, keyToRef: nextKeyToRef, keyToParent: nextKeyToParent, legacyKeyToKey: nextLegacyKeyToKey } =
      treeModel;

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
    } else if (selectedKey && nextKeyToRef.has(selectedKey)) {
      nextSelectedKey = selectedKey;
    } else {
      nextSelectedKey = rootKey;
    }

    if (nextSelectedKey && nextKeyToRef.has(nextSelectedKey)) {
      setSelectedKey(nextSelectedKey);
      const ancestors = ancestorKeysForKey(nextSelectedKey, nextKeyToParent);
      setExpandedKeys((prev) => !!selectedKey
        ? unionKeys(prev, ancestors)
        : Array.from(nextKeyToRef.keys()));
    }
  }, [story, activeTab, treeModel]);

  useEffect(() => {
    if (restartOpen && story && !restartGoalInitialized.current) {
      setRestartGoalMode(story.status === 'completed' ? 'new' : 'continue');
      restartGoalInitialized.current = true;
    }
  }, [restartOpen, story]);

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

  const focusedTask = useRef<string>();
  useEffect(() => {
    if (!focusTaskOrdinal || !story?.root) { focusedTask.current = undefined; return; }
    const focus = JSON.stringify([projectId, jobId, activeTab, focusTaskOrdinal, wantInput]);
    if (focusedTask.current === focus) return;
    const key = keyForTaskOrdinal(focusTaskOrdinal, keyToRef);
    if (!key) return;

    const ref = keyToRef.get(key);
    if (!ref || ref.type !== 'node') return;

    const ancestors = ancestorKeysForKey(key, keyToParent);
    setExpandedKeys((prev) => unionKeys(prev, ancestors));
    setSelectedKey(key);
    focusedTask.current = focus;
    setActiveDetailsTab(wantInput ? 'pending_input' : 'overview');
  }, [projectId, jobId, activeTab, focusTaskOrdinal, wantInput, story?.root, keyToRef, keyToParent]);

  useEffect(() => {
    if (refreshTimerRef.current) {
      window.clearInterval(refreshTimerRef.current);
      refreshTimerRef.current = null;
    }

    if (activeTab === 'execution' && autoRefresh && story?.status === 'running') {
      refreshTimerRef.current = window.setInterval(() => {
        loadStory();
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
    if (!needsMetadata || (!autoRefresh && activeTab !== 'conversation') || !activityJob || !activityStatus || isFinishedJob(activityJob)) return;
    const timer = window.setInterval(() => {
      loadMetadata();
    }, refreshIntervalMs);
    return () => window.clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [needsMetadata, autoRefresh, refreshIntervalMs, projectId, jobId, activityStatus, story?.status, activeTab]);

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
  const draftKey = JSON.stringify([projectId, jobId, restartSelection]);
  const jobPatchLines = drafts[draftKey]?.job || [];
  const scopePatchLines = drafts[draftKey]?.scope || [];
  const setJobPatchLines = (update: PatchLine[] | ((lines: PatchLine[]) => PatchLine[])) => setDrafts(prev => {
    const draft = prev[draftKey] || { job: [], scope: [] };
    return { ...prev, [draftKey]: { ...draft, job: typeof update === 'function' ? update(draft.job) : update } };
  });
  const setScopePatchLines = (update: ScopePatchLine[] | ((lines: ScopePatchLine[]) => ScopePatchLine[])) => setDrafts(prev => {
    const draft = prev[draftKey] || { job: [], scope: [] };
    return { ...prev, [draftKey]: { ...draft, scope: typeof update === 'function' ? update(draft.scope) : update } };
  });
  const onSnapshot = useCallback((snapshot?: RestartContext) => setInspection({ key: draftKey, snapshot }), [draftKey]);
  let generatedPatch: ReturnType<typeof buildContextPatch> | undefined;
  let patchError: string | undefined;
  if (applyPatch && !submissionRestart) {
    try { generatedPatch = buildContextPatch(inspection?.key === draftKey ? inspection.snapshot : undefined, jobPatchLines, scopePatchLines); }
    catch (error) { patchError = error instanceof Error ? error.message : 'Cannot generate patch'; }
  }


  const resetRestartState = () => {
    setRestartSelection(undefined);
    setApplyPatch(false);
    setDrafts({});
    setInspection(undefined);
  };

  const openRestart = (node: StoryNode | null = null) => {
    setRestartError(undefined);
    restartOpener.current = document.activeElement as HTMLElement;
    setRestartNode(node);
    restartGoalInitialized.current = !!story;
    resetRestartState();
    setRestartGoalMode(story?.status === 'completed' ? 'new' : 'continue');
    // Compact chapters can contain multiple boundaries. Prefer the recorded scope ID.
    const matches = restartScopes.filter(scope => node?.restart_scope_id
      ? scope.scope_id === node.restart_scope_id
      : scope.chapter_ordinal === node?.restart_chapter_ordinal);
    setRestartSelection(matches.length === 1 ? scopeKey(matches[0]) : undefined);
    setRestartOpen(true);
  };

  const sendRestart = async (saved: SavedRestart) => {
    const response = await fetch(
      `${API_BASE}/projects/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId)}/restart`,
      { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: stringifyJobJSON(saved.request) }
    );
    if (!response.ok) {
      const diagnostic = await response.text().catch(() => 'Unknown error');
      throw new Error(`${response.status} ${response.statusText}${diagnostic ? ` - ${diagnostic}` : ''}`);
    }
    const data = (await response.json()) as { job_id: string };
    if (!alive.current) return;
    sessionStorage.removeItem(storageKey);
    setSavedRestart(null);
    message.success('Restarted job');
    setRestartOpen(false);
    resetRestartState();
    navigate(`/project/${projectId}/jobs/${data.job_id}/story${filterSearch}`);
  };

  const submitRestart = async (replay = false) => {
    if (!jobId || restartBusy.current || cancellationBusy) return;
    if (!replay && (!restartable || loading || error || patchError)) return;
    if (replay && !savedRestart) return;
    restartBusy.current = true;
    setRestartSubmitting(true);
    setRestartError(undefined);
    try {
      if (replay) {
        // Replay is independent of fresh admission evidence and editor state.
        await sendRestart(savedRestart!);
      } else {
        const payload = { ...(restartScopeID ? { scope_id: restartScopeID } : { chapter_ordinal: restartChapterOrdinal }), goal: { mode: restartGoalMode }, ...(generatedPatch ? { patch: generatedPatch } : {}) };
        const serialized = stringifyJobJSON(payload)!;
        if (savedRestart?.payload === serialized) throw new Error('Use Retry saved request to resend this unchanged submission.');
        if (!alive.current) return;
        const saved = { payload: serialized, request: { ...payload, request_id: newId() } };
        sessionStorage.setItem(storageKey, stringifyJobJSON(saved)!);
        setSavedRestart(saved);
        await sendRestart(saved);
      }
    } catch (e) {
      if (alive.current) setRestartError(e instanceof Error ? e.message : 'Restart failed');
    } finally {
      restartBusy.current = false;
      if (alive.current) setRestartSubmitting(false);
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
    if (!jobId || !selectedInput.details) return;
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

      await load();
    } catch (e) {
      console.error('Failed to submit input response', e);
      message.error(e instanceof Error ? e.message : 'Failed to submit response');
    } finally {
      setPendingInputSubmitting(false);
    }
  };

  const inputDocuments: Record<string, ReaderDocument> = Object.create(null);
  const inputURLs: Record<string, string> = Object.create(null);
  const inputOrder: Record<string, { order: number }> = Object.create(null);
  inputs?.artifacts?.forEach((artifact, order) => {
    const key = inputViewerArtifact(artifact, jobId);
    const id = artifactRowKey(key, jobId);
    inputDocuments[id] = {
      name: artifact.name,
      hideIdentifier: true,
      sizeBytes: artifact.sizeBytes >= 0 ? artifact.sizeBytes : undefined,
      unavailableReason: artifact.reason,
      details: <>
        {artifact.binding !== undefined && <div>Binding: {artifact.binding}</div>}
        {artifact.sources?.length ? <div>{artifact.sources.join(' · ')}</div> : null}
      </>,
    };
    inputURLs[id] = artifact.reason ? '' : artifactUrl(projectId, jobId, key);
    inputOrder[id] = { order };
  });

  const initialMessage = job ? {
    sender, subject: messagePreview(prompt) || 'Original message', onExpand: openOriginal, showAttachments: threadParams.get('tab') === 'input-artifacts',
    subjectIsBodyPreview: !!prompt,
    time: { value: job.submitted_at || job.created_at },
    node: <>
      <section aria-label="Job prompt">
        {prompt ? <GoalPrompt projectId={projectId} jobId={jobId} text={prompt} inventory={promptInventory}
          waitingForInventory={!inputs?.artifacts && !inputs?.error} /> : <p>No prompt recorded for this job.</p>}
      </section>
      {inputs?.error ? <Alert type="warning" message="Unable to load attachments" description={inputs.error}
        action={<Button onClick={() => missingThreadInventory ? void snapshot.refresh() : setInputRetry(value => value + 1)}>Retry</Button>} />
        : !inputs?.artifacts ? <Text type="secondary">Checking attachments…</Text>
        : inputs.artifacts.length > 0 && <ReviewDocuments attachmentLayout="attachments" attachmentsLabel="Input attachments"
          projectId={projectId} jobId={jobId} requestId={jobId}
          documents={inputDocuments} documentMetadata={inputOrder}
          source={{ identity: artifactIdentity, url: id => inputURLs[id] ?? '' }} />}
    </>,
  } : undefined;

  return (
    <ThreadSnapshotContext.Provider value={thread}><div className="app-page job-story-page inbox-story" style={{ padding: 20 }}>
      <Space direction="vertical" style={{ width: '100%' }} size="small">
        <div className="job-story-header" data-testid="job-story-header">
          <div className="inbox-story-subject">
            <Link className="inbox-back" aria-label="Back to inbox" to={returnTo}>←</Link>
            <div className="inbox-story-heading">
              <div className="inbox-story-title">
                {job && <span className="inbox-story-cell" aria-label="Job cell" title={cellLabel(job.cell_name)}>{cellLabel(job.cell_name)}</span>}
                <Title level={3} className={prompt && !job?.title?.trim() ? 'inbox-prompt-title' : undefined} title={job && threadTitle(job)} style={{ margin: 0 }}>{job ? threadTitle(job) : 'Thread'}</Title>
              </div>
              {(displayStatus || hasDraft) && <div className="inbox-story-meta">
                {displayStatus && <Tag color={displayStatus.color} aria-label="Job status">{displayStatus.label}</Tag>}
                {hasDraft && <Tag color="magenta">Draft</Tag>}
              </div>}
            </div>
          </div>
          <details className="inbox-story-more"><summary>More actions</summary><div className="job-story-actions" role="group" aria-label="Job actions" onClickCapture={event => {
            if (!(event.target as HTMLElement).closest('button:not([role="switch"]), a')) return;
            const menu = event.currentTarget.closest('details');
            if (menu) {
              menu.open = false;
              menu.querySelector('summary')?.focus();
            }
          }} onFocusCapture={event => {
            (event.target.closest('button, a, .ant-select') || event.target).scrollIntoView({ block: 'nearest', inline: 'nearest' });
          }}>

            <Link className="compact-touch-link" to={`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId || '')}/story?tab=dag`} state={location.state}>Graph</Link>
            {treeReturnTo && <Link className="compact-touch-link" to={treeReturnTo}>Return to Graph</Link>}
            {activeTab !== 'conversation' && (terminalStatus ? !isFinishedJob(job!) : story?.status === 'running') ? (
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
            <Button style={{alignSelf:'flex-start'}} icon={<ReloadOutlined />} onClick={() => void load()}>
              Refresh
            </Button>
            <Button onClick={() => openRestart()}>Restart</Button>
            <Button onClick={() => setArchiveTarget({ job, immediate: canArchiveImmediately([jobId], job ? [job] : []) })}>Archive</Button>
            {requestId === null && <CancelJobButton key={`${projectId}/${jobId}`} projectId={projectId} jobId={jobId}
              disabled={!jobId || !!job && !!terminalStatus && isFinishedJob(job)}
              onCancelled={async () => { if (alive.current) await Promise.all([load(), refreshPendingInputs()]); }} />}
          </div></details>
        </div>

        {!job && (!metadata || metadata.status === 'loading') && <Text role="status" aria-label="Job details loading">Loading job details...</Text>}
        {metadata?.status === 'failed' && <Alert type="warning" showIcon message="Job details unavailable"
          description={<div style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
            <div>Cell, prompt, and completion information could not be loaded.</div>
            <div>{metadata.error}</div>
          </div>}
          action={<Button size="small" onClick={() => void loadMetadata()}>Retry</Button>} />}
        <JobDetailTabs activeTab={activeTab} onChange={selectTab} />
        <div className="job-detail-panels">
        <section role="tabpanel" id="job-panel-conversation" aria-labelledby="job-tab-conversation" hidden={activeTab !== 'conversation'} tabIndex={0} className="job-detail-panel">
        {selectedInput.discovery.status === 'unavailable' && pendingInputDetails && threadRequestId === null && <Alert type="warning" showIcon message="Pending request availability is unavailable." description={selectedInput.discovery.error} action={<Button onClick={() => void selectedInput.refresh().catch(() => {})}>Retry</Button>} />}
        {threadWantsInput && !threadReviewId && threadRequestId === null && !pendingInputLoading && !pendingInputDetails && !pendingInputError && <Alert type={selectedInput.discovery.status === 'unavailable' ? 'warning' : 'info'} showIcon
          message={selectedInput.discovery.status === 'unavailable' ? 'Pending request availability is unavailable.' : 'No outstanding input request.'}
          action={<Button onClick={() => void selectedInput.refresh().catch(() => {})}>{selectedInput.discovery.status === 'unavailable' ? 'Retry' : 'Refresh'}</Button>} />}
        {(activeTab === 'conversation' || threadVisited) && <JobConversation folded onSnapshot={setSnapshot} metadata={job}
          initial={initialMessage}
          currentIsReview={pendingIsReview || !!threadReviewId}
          currentReviewId={threadReviewId}
          currentRequestTime={pendingInputDetails?.startTime}
          currentRequestLabel={pendingLabel} currentSubjectIsPrompt={!pendingTitle && !!pendingLabel} openCurrent={threadWantsInput || threadParams.has('review_id')} currentRequestId={threadRequestId === null && (!threadReviewId || threadReviewId === currentReviewId) ? pendingInputDetails?.form.request_id : undefined} cellName={job?.cell_name} onInventory={receiveInventory} onBack={() => setSearchParams(previous => { const next = new URLSearchParams(previous); next.delete('occurrenceId'); next.delete('requestId'); return next; })} tenant={projectId} job={jobId} occurrence={threadParams.get('occurrenceId') ?? undefined}
          autoRefresh={activeTab === 'conversation'} requestId={threadParams.get('requestId') ?? undefined}
          refreshKey={historyRevision} onOpen={entry => setSearchParams(previous => {
            const next = new URLSearchParams(previous); next.set('occurrenceId', entry); next.set('tab', 'conversation');
            next.delete('requestId'); next.delete('input'); return next;
          })}>
          {threadRequestId === null && !threadParams.has('occurrenceId') && (threadReviewId
            ? <ReviewV2 key={JSON.stringify([projectId,jobId,threadReviewId])} tenant={projectId} job={jobId} reviewId={threadReviewId} embedded metadata={job} />
            : (pendingInputDetails || pendingInputLoading || pendingInputError) && <InputRequestReader key={JSON.stringify([projectId,jobId,pendingInputDetails?.form.request_id])} projectId={projectId} embedded metadata={job} />)}
        </JobConversation>}
        </section>
        <section role="tabpanel" id="job-panel-runtime" aria-labelledby="job-tab-runtime" hidden={activeTab !== 'runtime'} tabIndex={0} className="job-detail-panel">
          {activeTab === 'runtime' && job && <JobEnvironment job={job} />}
          <JobCrashConcern key={dagIdentity} projectId={projectId} jobId={jobId} job={job} status={metadata?.status}
            pause={() => { crashMutationBusy.current = true; ++metadataGeneration.current; }}
            resume={() => { crashMutationBusy.current = false; }} refresh={() => loadMetadata(true)} />
          {activeTab === 'runtime' && <JobLeaseTimeline expanded projectId={projectId} jobId={jobId} createdAt={job?.created_at} />}
        </section>
        <section role="tabpanel" id="job-panel-dag" aria-labelledby="job-tab-dag" hidden={activeTab !== 'dag'} tabIndex={0} className="job-detail-panel">
          {(activeTab === 'dag' || dagVisited === dagIdentity) && <>
            <TreeWorkspace key={dagIdentity} tenant={projectId} entry={jobId!} active={activeTab === 'dag'} embedded onJobChanged={node => { if (node.tenant_id === projectId && node.job_id === jobId) { void loadMetadata(); void refreshPendingInputs(); } }}
              relationships={job && <JobRelationships showAttempts projectId={projectId} jobId={jobId!} job={job} currentGoal={currentGoal} />} />
          </>}
        </section>
        <section role="tabpanel" id="job-panel-execution" aria-labelledby="job-tab-execution" hidden={activeTab !== 'execution'} tabIndex={0} className="job-detail-panel">
        {activeTab === 'execution' && <>
        <section className="story-summary-disclosure">
          <h2>Run Summary</h2>
          <Card size="small" variant="borderless">
            <Descriptions column={{ xs: 1, sm: 2 }} bordered size="small">
              <Descriptions.Item label="Status">
                {pendingReview && terminalStatus === 'READY' && displayStatus ? <Tag color={displayStatus.color}>{displayStatus.label}</Tag> : story?.status ? (
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
        </section>

        {loading && !story && <Spin size="large" />}

        {error ? (
          <Alert
            type="error"
            showIcon
            message="Failed to load job story"
            description={error}
            action={
              <Button size="small" onClick={() => loadStory()}>
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
                          {nodeLaunch && <Descriptions.Item label="Launch ID"><Link to={launchPath(projectId,nodeLaunch,{jobId})}>{nodeLaunch}</Link></Descriptions.Item>}
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
                              <Button type="primary" onClick={() => openRestart(selectedNode)}>
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
                                  <Link to={reviewThreadPath(projectId,jobId)}>Open Review</Link>
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
                                    loading={pendingInputSubmitting || !selectedInput.details}
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
                    label: `Inbox${inboxCount(selectedNode)}`,
                    children: <InboxPanel key={JSON.stringify([projectId, jobId, selectedKey, selectedRef?.jobAttempt, selectedNode.attempt, selectedNode.invocation_id, selectedNode.task_ordinal])}
                      node={selectedNode} projectId={projectId} jobId={jobId} jobAttempt={selectedRef?.jobAttempt || 1} retry={() => load()} />,
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
        </div>
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
          htmlContext={{canonicalPath: activeArtifact.name, attachments: artifactAttachments.map(a => ({key:artifactRowKey(a, jobId), path:a.name, url:artifactUrl(projectId,jobId,a)}))}}
          key={JSON.stringify([projectId, artifactRowKey(activeArtifact, jobId)])}
          url={artifactUrl(projectId, jobId, activeArtifact)} name={activeArtifact.name}
          unavailableMessage={activeArtifact.contentBinding !== undefined ? `Document “${activeArtifact.name}” unavailable` : undefined}
          navigationIntent={artifactNavigation}
          navigation={{
            resolve: reference => resolveArtifactLink(reference, activeArtifact.name, artifactAttachments.map(a => ({key: artifactRowKey(a, jobId), path: a.name}))),
            open: (key, fragment) => {
              if (artifactOwner !== artifactIdentity) return false;
              const target = artifactAttachments.find(a => artifactRowKey(a, jobId) === key);
              if (!target) return false;
              setSelectedArtifact(target);
              setArtifactNavigation(previous => ({token: (previous?.token || 0) + 1, fragment}));
              return true;
            },
          }}
          resolveAttachment={reference => {
            const resolution = resolveArtifactLink(reference, activeArtifact.name, artifactAttachments.map(a => ({
              key: artifactRowKey(a, jobId), path: a.name, aliases: [a.name],
            })));
            if (resolution.kind !== 'target') return undefined;
            const a = artifactAttachments.find(a => artifactRowKey(a, jobId) === resolution.key);
            if (!a) return undefined;
            const key = resolution.key;
            return { key, name: a.name, url: artifactUrl(projectId, jobId, a) };
          }}
        />}
      </Modal>}

      {archiveTarget && <JobActionDialog projectId={projectId} jobIds={[jobId]} action="archive"
        job={archiveTarget.job} immediate={archiveTarget.immediate}
        onClose={() => { if (alive.current) setArchiveTarget(undefined); }}
        onComplete={() => {
          if (!alive.current) return;
          inbox?.refresh();
          navigate(returnTo);
          // Activity refresh is independent of the successful archive and return.
          void refreshPendingInputs().catch(() => {});
        }} />}
      <Modal
        title="Restart Job"
        open={restartOpen}
        onCancel={() => {
          if (restartBusy.current || cancellationBusy) return;
          setRestartOpen(false);
          resetRestartState();
        }}
        afterClose={() => restartOpener.current?.isConnected && restartOpener.current.focus()}
        width={900}
        okText="Restart"
        okButtonProps={{ 'aria-label': 'Restart', loading: restartSubmitting, disabled: restartSubmitting || cancellationBusy || !restartable || !!patchError || loading || !!error }}
        onOk={() => void submitRestart()}
      >
        <Space direction="vertical" style={{ width: '100%', marginBottom: 16 }}>
          <Alert type="info" showIcon message="The restart service checks whether this request is allowed."
            description="Restarting does not cancel the source job." />
          {metadata?.status === 'failed' && <Alert type="info" message="Source details unavailable" description={metadata.error} />}
          {cancellationResult && <Alert type="info" message={cancellationResult} description={sourceFinished ? 'Source completion is now confirmed.' : 'Cancellation remains pending until fresh job detail confirms completion.'} />}
          {cancellationError && <Alert type="error" message="Source cancellation failed" description={cancellationError} />}
          {restartError && <Alert type="error" message="Restart failed" description={restartError} />}
          <Space wrap>
            <Button disabled={restartSubmitting || cancellationBusy} onClick={() => void loadMetadata()}>Refresh source</Button>
            {!sourceFinished && <Button disabled={restartSubmitting || cancellationBusy} onClick={() => setCancelSource(true)}>Cancel source</Button>}
            <Button disabled={restartSubmitting || cancellationBusy} onClick={() => setRestartGoalMode('new')}>Start a new goal</Button>
            <Link to={returnTo}>Back to inbox</Link>
          </Space>
          {savedRestart && <Card size="small" title="Saved restart request">
            <Typography.Paragraph>The following saved request will be retried unchanged, independently of the current editor and source completion.</Typography.Paragraph>
            <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', maxHeight: 180, overflow: 'auto' }}>{stringifyJobJSON(savedRestart.request)}</pre>
            <Button disabled={restartSubmitting || cancellationBusy} onClick={() => void submitRestart(true)}>Retry saved request</Button>
          </Card>}
        </Space>
        {story ? (
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

            {error && <Alert type="error" message="Failed to load job story" description={error} action={<Button onClick={() => loadStory()}>Retry story</Button>} />}
            {!restartScopes.length && <Alert type="info" message="No recorded restart points are available." />}
            {!restartable && restartNode && <Alert type="info" showIcon
              message={restartNode?.restart_unavailable_reason || 'No recorded restart point is available for this node.'}
              description="Choose a restart point below." />}
            {restartChapterOrdinal === restartNode?.restart_chapter_ordinal && restartNode?.restart_relationship === 'containing' &&
              <Alert type="info" showIcon message="This restarts the containing scope, including its work before this task." />}
            {restartChapterOrdinal === restartNode?.restart_chapter_ordinal && restartNode?.restart_relationship === 'body' &&
              <Text type="secondary">Restart from the recipe’s executable body.</Text>}
            <Select
              aria-label="Restart point"
              placeholder="Choose a restart point"
              style={{ width: '100%' }}
              value={restartSelection}
              disabled={restartSubmitting || cancellationBusy} onChange={setRestartSelection}
              options={restartScopes.map(scope => ({
                value: scopeKey(scope),
                label: `${scope.title || scope.path?.join(" / ") || "Restart point"} · Chapter ${scope.chapter_ordinal} · ${formatTimestamp(scope.started_at)}`,
              }))}
              showSearch
              optionFilterProp="label"
            />

            <Select aria-label="Restart goal" value={restartGoalMode} disabled={restartSubmitting || cancellationBusy} onChange={setRestartGoalMode} style={{ width: '100%' }}
              options={[{ value: 'continue', label: 'Continue this goal' }, { value: 'new', label: 'Start a new goal' }]} />
            <Card title="Context Patching" size="small"><fieldset disabled={restartSubmitting || cancellationBusy} style={{ border: 0, padding: 0, margin: 0 }}>
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
                    <RestartContextPreview onSnapshot={onSnapshot} projectId={projectId} jobId={jobId} scopeId={restartScopeID} chapterOrdinal={restartable && !restartScopeID ? restartChapterOrdinal : undefined} />
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
                        Each line edits a value under <Text code>context</Text>. Use dot
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
                        Each line edits a value under a scope container’s outputs.
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
                      {patchError ? <Alert type="error" message={patchError} /> : <pre aria-label="Patch payload">{stringifyJobJSON(generatedPatch, 2)}</pre>}
                    </Card>
                  </Space>
                )}
              </Space>
            </fieldset></Card>
          </Space>
        ) : (
          <div>{error ? <Alert type="error" message="Failed to load job story" description={error} action={<Button onClick={() => loadStory()}>Retry story</Button>} /> : <Spin tip="Loading restart points…"><div style={{ minHeight: 80 }} /></Spin>}</div>
        )}
      </Modal>
      {cancelSource && <JobActionDialog projectId={projectId} jobIds={[jobId]} action="cancel" job={job}
        onBusyChange={setCancellationBusy}
        onFailure={failures => { if (alive.current) { setCancellationError(failures.map(failure => failure.error).join('; ')); void loadMetadata(); } }}
        onClose={() => { if (alive.current) { setCancelSource(false); void loadMetadata(); } }}
        onComplete={async () => {
          if (!alive.current) return;
          setCancellationError(undefined);
          setCancellationResult('Cancellation requested for this source');
          await loadMetadata();
        }} />}

    </div></ThreadSnapshotContext.Provider>
  );
}
