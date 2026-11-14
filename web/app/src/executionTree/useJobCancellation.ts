import { useEffect, useReducer, useRef, useState } from 'react';
import { getTreeNode, type TreeNode } from '@colony2/shared';
import { isFinishedJob } from '../utils/jobStatus';

const unfinished = new Set(['READY', 'PENDING_JOBS', 'AWAITING_FUTURE', 'ACTIVE', 'CRASH_CONCERN', 'EXPIRED']);
export const canCancelNode = (node: TreeNode) => !isFinishedJob(node) && !node.cancel_requested && unfinished.has(node.status);
export interface CancellationState {
  node?: TreeNode;
  acknowledged?: boolean;
  busy?: boolean;
  unavailable?: boolean;
  error?: string;
  notice?: string;
}

/** Exact observations outlive discovery refreshes, and always win over listings. */
export function useJobCancellation(tenant: string, entry: string, onChanged?: (node: TreeNode) => void) {
  const [, update] = useReducer(n => n + 1, 0);
  const [target, setTarget] = useState<TreeNode>();
  const records = useRef(new Map<string, CancellationState>());
  const lifetime = useRef(0);
  const sequences = useRef(new Map<string, number>());
  const reads = useRef(new Set<AbortController>());
  const changed = useRef(onChanged); changed.current = onChanged;
  useEffect(() => {
    ++lifetime.current;
    records.current.clear(); sequences.current.clear(); setTarget(undefined);
    return () => { ++lifetime.current; reads.current.forEach(c => c.abort()); reads.current.clear(); };
  }, [tenant, entry]);
  const state = (id: string) => records.current.get(id);
  const patch = (id: string, values: Partial<CancellationState>) => {
    records.current.set(id, {...state(id), ...values}); update();
  };
  const refresh = async (node: TreeNode) => {
    const id = node.job_id, epoch = lifetime.current;
    const sequence = (sequences.current.get(id) || 0) + 1;
    sequences.current.set(id, sequence);
    const controller = new AbortController(); reads.current.add(controller);
    patch(id, {busy:true});
    try {
      const result = await getTreeNode(tenant, id, controller.signal);
      if (epoch !== lifetime.current || sequences.current.get(id) !== sequence) return;
      if (result.node.tenant_id !== tenant || result.node.job_id !== id) throw new Error('Unexpected job identity');
      // Reconciliation changes status, never the discovery graph's parentage.
      const fresh = {...result.node, parent_job_id:node.parent_job_id};
      patch(id, {node:fresh, unavailable:false, busy:false,
        ...(isFinishedJob(fresh) ? {acknowledged:false, error:undefined} : {})});
      changed.current?.(fresh);
      return fresh;
    } catch (error) {
      if (epoch === lifetime.current && sequences.current.get(id) === sequence)
        patch(id, {busy:false, unavailable:true, error:`Status unavailable: ${error instanceof Error ? error.message : String(error)}`});
    } finally { reads.current.delete(controller); }
  };
  const settled = async (succeeded: string[], failures: {id:string; error:string}[]) => {
    if (!target) return false;
    const epoch = lifetime.current, id = target.job_id;
    patch(id, {acknowledged:state(id)?.acknowledged || succeeded.includes(id), error:failures[0]?.error});
    const fresh = await refresh(target);
    if (epoch !== lifetime.current) return false;
    if (fresh && isFinishedJob(fresh) && failures.length) patch(id, {notice:'This job is already finished. Its recorded outcome is shown.'});
    return !!fresh && (isFinishedJob(fresh) || !!fresh.cancel_requested || !!state(id)?.acknowledged);
  };
  const eligible = (node: TreeNode) => canCancelNode(node) && !state(node.job_id)?.acknowledged
    && !state(node.job_id)?.unavailable && !state(node.job_id)?.busy;
  return { target, state, eligible, refresh, settled,
    open: (node: TreeNode) => { if (eligible(node)) setTarget({...node}); },
    close: () => setTarget(undefined),
    busy: (busy: boolean) => { if (target) patch(target.job_id, {busy}); },
    overlay: (node: TreeNode): TreeNode => {
      const record = state(node.job_id);
      const observed = record?.node ? {...record.node, parent_job_id:node.parent_job_id} : node;
      return record?.acknowledged && !isFinishedJob(observed) ? {...observed, cancel_requested:true} : observed;
    },
  };
}
