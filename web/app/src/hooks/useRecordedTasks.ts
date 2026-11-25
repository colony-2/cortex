import { useEffect, useState } from 'react';
import { getRecordedTrace } from '@colony2/shared';
import type { StoryNode } from '../utils/storyTree';

export function useRecordedTasks(tenant: string, job: string, active: boolean, interval: number, revision: number) {
  const owner = JSON.stringify([tenant, job]);
  const [result, setResult] = useState<{owner: string; nodes?: StoryNode[]; error?: string}>();
  useEffect(() => {
    if (!active) return;
    const abort = new AbortController();
    let timer: number | undefined;
    async function read() {
      try {
        const result = await getRecordedTrace(tenant, job, abort.signal);
        if (abort.signal.aborted) return;
        setResult({owner, nodes: result.tasks.map(row => ({
          ...row, kind: 'task', path: row.path ?? [],
          restart_relationship: row.restart_scope_id ? 'containing' : undefined,
          inbox: row.input_artifacts?.length ? {status: 'available', entries: row.input_artifacts.map(artifact => ({
            binding: artifact.name, path: artifact.name, source_key: artifact.key,
            source_kind: 'artifact', content: artifact.key ? 'stored' : 'unsupported',
          }))} : undefined,
        }))});
        if (result.terminal) return;
      } catch (error) {
        if (!abort.signal.aborted) setResult(previous => ({owner, nodes:previous?.owner === owner ? previous.nodes : undefined, error:String(error)}));
      }
      if (!abort.signal.aborted && interval) timer = window.setTimeout(read, interval);
    }
    void read();
    return () => { abort.abort(); window.clearTimeout(timer); };
  }, [active, tenant, job, owner, interval, revision]);
  return result?.owner === owner ? result : undefined;
}
