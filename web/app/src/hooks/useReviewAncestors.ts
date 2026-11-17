import { useEffect, useReducer, useState } from 'react';
import { getTreeNode, type TreeNode } from '@colony2/shared';

// Only the execution tree's recorded runtime parent defines this branch.
export function useReviewAncestors(tenant: string, job: string) {
  const [generation, retry] = useReducer(n => n + 1, 0);
  const [state, setState] = useState<{identity: string; nodes: TreeNode[]; loading: boolean; error?: string}>();
  const identity = JSON.stringify([tenant, job]);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    const nodes: TreeNode[] = [];
    const update = (loading: boolean, error?: string) => { if (active) setState({identity, nodes: [...nodes].reverse(), loading, error}); };
    update(true);
    const timer = window.setTimeout(() => { update(false, 'Ancestor context timed out.'); controller.abort(); }, 30_000);
    void (async () => {
      try {
        let id: string | undefined = job;
        const seen = new Set<string>();
        while (id) {
          if (seen.has(id)) throw new Error('A cycle interrupts the ancestor branch.');
          seen.add(id);
          const {node} = await getTreeNode(tenant, id, controller.signal);
          if (!active || controller.signal.aborted) return;
          if (node.tenant_id !== tenant || node.job_id !== id) throw new Error('Ancestor identity could not be verified.');
          nodes.push(node); update(true);
          if (nodes.length === 65 && node.parent_job_id) throw new Error('Ancestor branch exceeds 64 parent edges.');
          id = node.parent_job_id;
        }
        update(false);
      } catch (error) {
        if (!controller.signal.aborted) update(false, error instanceof Error ? error.message : String(error));
      } finally { window.clearTimeout(timer); }
    })();
    return () => { active = false; window.clearTimeout(timer); controller.abort(); };
  }, [identity, tenant, job, generation]);
  return {...(state?.identity === identity ? state : {nodes: [], loading: true, error: undefined}), retry};
}
