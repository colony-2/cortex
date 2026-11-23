import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { getUserEmail, useInputActivity } from '@colony2/shared';
import { useCells } from './useCells';
import { loadJobInbox, inboxReadKey, inboxRevision, type InboxJob } from '../utils/jobInboxData';
import { inboxPendingCounts, rankInboxCells } from '../utils/jobInbox';

function useInboxValue(tenant: string) {
  const [turn, setTurn] = useState(0);
  const registry = useCells(tenant, 'ALL', turn);
  const activity = useInputActivity();
  const [snapshot, setSnapshot] = useState<{ tenant: string; jobs: InboxJob[] }>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [readTurn, setReadTurn] = useState(0);
  const user = getUserEmail() ?? '';
  useEffect(() => {
    let current = true;
    setLoading(true); setError(undefined);
    loadJobInbox(tenant, () => !current).then(jobs => { if (current) setSnapshot({ tenant, jobs }); })
      .catch(reason => { if (current) setError(reason instanceof Error ? reason.message : String(reason)); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [tenant, turn]);
  useEffect(() => {
    const changed = () => setReadTurn(value => value + 1);
    window.addEventListener('storage', changed);
    return () => window.removeEventListener('storage', changed);
  }, []);
  const jobs = snapshot?.tenant === tenant ? snapshot.jobs : [];
  const pending = activity.currentProjectId === tenant ? activity.pendingInputs : [];
  const counts = inboxPendingCounts(tenant, { tenantId: activity.currentProjectId ?? '', status: activity.discoveryStatus, inputs: pending });
  const totals = new Map<string, number>();
  for (const job of jobs) if (!job.inboxArchived) totals.set(job.cell_id, (totals.get(job.cell_id) ?? 0) + 1);
  const cells = new Map(registry.cells.map(cell => [cell.id, { ...cell, unarchivedJobs: totals.get(cell.id) ?? 0 }]));
  for (const job of jobs) if (!cells.has(job.cell_id)) cells.set(job.cell_id, {
    id: job.cell_id, tenantId: tenant, name: job.cell_name, repo: job.repo, state: 'RETIRED', description: '', metadata: {}, aliases: [], revision: '', createdAt: '', updatedAt: '', unarchivedJobs: totals.get(job.cell_id) ?? 0,
  });
  const ranked = rankInboxCells([...cells.values()].filter(cell => cell.state === 'ACTIVE' || cell.unarchivedJobs > 0));
  const revision = (job: InboxJob) => {
    const request = pending.find(input => input.id === job.job_id);
    return inboxRevision(job, request?.requested_at, request?.request_id);
  };
  const memory = useState(() => new Map<string, string>())[0];
  const isUnread = (job: InboxJob) => {
    if (job.inboxArchived) return false;
    const key = inboxReadKey(user, tenant, job.job_id);
    let seen = memory.get(key);
    try { seen = localStorage.getItem(key) ?? seen; } catch { /* Session-only fallback. */ }
    return seen !== revision(job);
  };
  const markRead = (job: InboxJob) => {
    const key = inboxReadKey(user, tenant, job.job_id), value = revision(job);
    memory.set(key, value);
    try { localStorage.setItem(key, value); } catch { /* Session-only fallback. */ }
    setReadTurn(value => value + 1);
  };
  return { tenant, jobs, loading, error, loaded: snapshot?.tenant === tenant, registry, ranked, pending, counts, isUnread, markRead, readTurn,
    refresh: () => { setTurn(value => value + 1); void activity.refresh(); } };
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
export function useInboxReadReceipt(tenant: string, jobId: string | undefined, readable: boolean) {
  const inbox = useContext(Context);
  const job = inbox?.tenant === tenant ? inbox.jobs.find(row => row.job_id === jobId) : undefined;
  const request = inbox?.pending.find(row => row.id === jobId);
  const revision = job ? inboxRevision(job, request?.requested_at, request?.request_id) : undefined;
  useEffect(() => { if (readable && job && inbox?.isUnread(job)) inbox.markRead(job); }, [tenant, jobId, readable, revision]);
}
