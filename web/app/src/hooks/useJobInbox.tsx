import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { matchPath, useLocation } from 'react-router-dom';
import { getUserEmail, useInputActivity } from '@colony2/shared';
import type { RecipeJobSummary } from '@colony2/shared/types';
import { useCells } from './useCells';
import { inboxReadKey, inboxRevision, type InboxJob } from '../utils/jobInboxData';
import { inboxFolder, inboxPendingCounts, rankInboxCells, readInboxSort } from '../utils/jobInbox';
import { useInboxJobs } from './useInboxJobs';

const noPending: ReturnType<typeof useInputActivity>['pendingInputs'] = [];

function useInboxValue(tenant: string) {
  const [turn, setTurn] = useState(0);
  const registry = useCells(tenant, 'ALL');
  const activity = useInputActivity();
  const { pathname, search } = useLocation();
  const params = new URLSearchParams(search);
  const folder = inboxFolder(params.get('inbox'));
  const cellId = params.get('cellId');
  const onList = !!(matchPath('/jobs', pathname) || matchPath('/project/:projectId/jobs', pathname));
  const waiting = onList && folder === 'reviews';
  const pending = activity.currentProjectId === tenant ? activity.pendingInputs : noPending;
  const [allPage, setAllPage] = useState<{tenant:string; jobs: InboxJob[]}>();
  const allJobs = allPage?.tenant === tenant ? allPage.jobs : [];
  const jobSummaries = useRef(new Map<string, RecipeJobSummary>());
  const knownJobs = [...allJobs, ...pending];
  const cellFolder = onList && folder === 'cell';
  const observedCell = cellFolder ? knownJobs.find(job => job.cell_id === cellId && job.cell_name) : undefined;
  const tenantCells = registry.cells.filter(cell => cell.tenantId === tenant);
  const cell = cellFolder ? tenantCells.find(cell => cell.id === cellId)
    ?? (observedCell && { id: observedCell.cell_id, name: observedCell.cell_name }) : undefined;
  const pagedFolder = !onList || waiting || folder === 'drafts' ? undefined
    : folder === 'cell' || folder === 'archive' || folder === 'active' || folder === 'waiting' || folder === 'failed' ? folder : 'all';
  // Search and explicit sorts need the whole folder until JobDB offers those queries.
  const complete = !!(params.get('q')?.trim() || readInboxSort(params.get('sort')));
  const page = useInboxJobs(tenant, pagedFolder, cell, complete, turn);
  useEffect(() => {
    if (pagedFolder === 'all' && page.loaded) setAllPage({tenant, jobs:page.jobs});
    for (const job of page.jobs) if (job.title?.trim() || job.prompt?.trim()) jobSummaries.current.set(JSON.stringify([tenant,job.job_id]), job);
  }, [pagedFolder, page.jobs, page.loaded, tenant]);

  const counts = inboxPendingCounts(tenant, { tenantId: activity.currentProjectId ?? '', status: activity.discoveryStatus, inputs: pending });
  const pendingJobs = useMemo(() => pending.filter(input => !input.tenant_id || input.tenant_id === tenant).map(input => ({
    ...input, tenant_id: tenant, job_id: input.id, status: input.status ?? 'READY', store: input.store ?? 'ACTIVE',
    job_type: input.job_type ?? 'recipe', recipe: input.recipe ?? '', created_at: input.created_at ?? '',
    available_at: input.available_at ?? '', inboxArchived: false,
  } satisfies InboxJob)), [pending, tenant]);
  const cellUnavailable = pagedFolder === 'cell' && !cell && !registry.loading;
  const jobs = waiting ? pendingJobs : page.jobs;
  const loading = waiting ? counts.status === 'loading' : page.loading || pagedFolder === 'cell' && !cell && registry.loading;
  const error = waiting ? undefined : cellUnavailable ? registry.error || 'This cell is unavailable.' : page.error;
  const [readTurn, setReadTurn] = useState(0);
  const user = getUserEmail() ?? '';
  useEffect(() => {
    const changed = () => setReadTurn(value => value + 1);
    window.addEventListener('storage', changed);
    return () => window.removeEventListener('storage', changed);
  }, []);
  // The sidebar can prioritize cells from loaded All pages without a separate tenant scan.
  const totals = new Map<string, number>();
  for (const job of allJobs) totals.set(job.cell_id, (totals.get(job.cell_id) ?? 0) + 1);
  const cells = new Map(tenantCells.map(cell => [cell.id, { ...cell, unarchivedJobs: totals.get(cell.id) ?? 0 }]));
  for (const job of knownJobs) if (job.cell_id && job.cell_name && !cells.has(job.cell_id)) cells.set(job.cell_id, {
    id: job.cell_id, tenantId: tenant, name: job.cell_name, repo: job.repo, state: 'RETIRED', description: '', metadata: {}, aliases: [], revision: '', createdAt: '', updatedAt: '', unarchivedJobs: totals.get(job.cell_id) ?? 0,
  });
  const ranked = rankInboxCells([...cells.values()].filter(cell => cell.state === 'ACTIVE' || cell.unarchivedJobs > 0
    || (counts.status === 'current' && counts.byCell.has(cell.id)) || cell.id === cellId));
  const revision = (job: RecipeJobSummary) => {
    const request = pending.find(input => input.id === job.job_id);
    return inboxRevision(job, request?.requested_at, request?.request_id);
  };
  const memory = useState(() => new Map<string, string>())[0];
  const isUnread = (job: RecipeJobSummary & { inboxArchived?: boolean }) => {
    if (job.inboxArchived) return false;
    const key = inboxReadKey(user, tenant, job.job_id);
    let seen = memory.get(key);
    try { seen = localStorage.getItem(key) ?? seen; } catch { /* Session-only fallback. */ }
    return seen !== revision(job);
  };
  const markRead = (job: RecipeJobSummary) => {
    const key = inboxReadKey(user, tenant, job.job_id), value = revision(job);
    memory.set(key, value);
    try { localStorage.setItem(key, value); } catch { /* Session-only fallback. */ }
    setReadTurn(value => value + 1);
  };
  return { tenant, jobs, loading, error, loaded: waiting ? counts.status === 'current' : page.loaded,
    nextPageToken: page.nextPageToken, loadMore: page.loadMore, retry: cellUnavailable ? registry.reload : page.retry,
    registry: {...registry, cells:tenantCells}, ranked, pending, counts, isUnread, markRead, readTurn,
    knownJob: (id: string) => jobSummaries.current.get(JSON.stringify([tenant,id])) ?? pendingJobs.find(job => job.job_id===id),
    refresh: () => { if (cellUnavailable) registry.reload(); setTurn(value => value + 1); void activity.refresh(); } };
}
const Context = createContext<ReturnType<typeof useInboxValue> | null>(null);
export function JobInboxProvider({ tenant, children }: { tenant: string; children: ReactNode }) {
  return <Context.Provider value={useInboxValue(tenant)}>{children}</Context.Provider>;
}
export function useJobInbox() {
  const context = useOptionalJobInbox();
  if (!context) throw new Error('Job inbox provider is required');
  return context;
}
export function useOptionalJobInbox() {
  return useContext(Context);
}

// Standalone story tests and embedders need no inbox provider. Acknowledgement
// happens only after job metadata is readable, including direct story links.
export function useInboxReadReceipt(tenant: string, job: RecipeJobSummary | undefined) {
  const inbox = useContext(Context);
  const request = inbox?.pending.find(row => row.id === job?.job_id);
  const revision = job ? inboxRevision(job, request?.requested_at, request?.request_id) : undefined;
  useEffect(() => { if (job && inbox?.tenant === tenant && inbox.isUnread(job)) inbox.markRead(job); }, [tenant, job?.job_id, revision]);
}
