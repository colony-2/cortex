import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { getTreeNode, getTreeWaits, type RecipeJob } from '@colony2/shared';
import { useOptionalJobInbox } from '../hooks/useJobInbox';
import { normalizeWaits, WAIT_LIMITS } from '../executionTree/waits';
import { messagePreview } from '../utils/jobInbox';
import { jobPath } from './JobWaitLinks';

function DependencyLink({ tenant, id, known }: { tenant: string; id: string; known?: string }) {
  const [label, setLabel] = useState<string>();
  useEffect(() => {
    if (known) return;
    const abort = new AbortController();
    queueMicrotask(() => {
      if (abort.signal.aborted) return;
      void getTreeNode(tenant, id, abort.signal).then(({ node }) => {
        if (!abort.signal.aborted && node.tenant_id === tenant && node.job_id === id) {
          setLabel(node.title?.trim() || messagePreview(node.prompt) || id);
        }
      }).catch(() => {});
    });
    return () => abort.abort();
  }, [tenant, id, known]);
  return <Link title={id} to={jobPath(tenant, id)}>{known || label || id}</Link>;
}

export default function ThreadDependencies({ tenant, job, details }: { tenant: string; job: string; details?: RecipeJob }) {
  const inbox = useOptionalJobInbox();
  const [discovered, setDiscovered] = useState<string[]>([]);
  const recorded = details?.wait_for ?? [];
  const wait = details?.blocking_wait;
  const hasWaitDetails = wait?.state === 'PENDING' || recorded.length > 0;
  const targets = wait?.state === 'PENDING' ? wait.dependencies.flatMap(dependency =>
    dependency.state === 'PENDING' && dependency.target?.jobId ? [dependency.target.jobId] : []) : recorded;
  const ids = [...new Set(hasWaitDetails ? targets : discovered)].filter(id => id && id !== job);
  useEffect(() => {
    if (hasWaitDetails) { setDiscovered([]); return; }
    const abort = new AbortController();
    const loadWaits = async () => {
      const targets = new Set<string>();
      const seen = new Set<string>();
      let token = '';
      let inspected = 0;
      for (let pageNumber = 0; !abort.signal.aborted && pageNumber < WAIT_LIMITS.pages && inspected < WAIT_LIMITS.rows; pageNumber++) {
        if (seen.has(token)) break;
        seen.add(token);
        const page = await getTreeWaits(tenant, job, token, abort.signal);
        const normalized = normalizeWaits(page, tenant, job, WAIT_LIMITS.rows - inspected);
        inspected += normalized.inspected;
        for (const row of normalized.rows) if (row.kind === 'job') targets.add(row.id);
        if (!pageNumber && !page.waits.length) for (const id of page.consumer_snapshot?.legacy_wait_for ?? []) targets.add(id);
        if (!abort.signal.aborted) setDiscovered([...targets]);
        token = page.next_page_token ?? '';
        if (!token) break;
      }
    };
    queueMicrotask(() => { void loadWaits().catch(() => {}); });
    return () => abort.abort();
  }, [tenant, job, hasWaitDetails]);

  if (!ids.length) return null;
  return <ul className="thread-dependencies" aria-label="Jobs this thread is waiting for">{ids.map(id => {
    const summary = inbox?.tenant === tenant ? inbox.knownJob(id) : undefined;
    return <li key={id}><DependencyLink tenant={tenant} id={id} known={summary?.title?.trim() || messagePreview(summary?.prompt)} /></li>;
  })}</ul>;
}
