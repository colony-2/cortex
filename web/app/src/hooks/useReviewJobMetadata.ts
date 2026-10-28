import { useEffect, useReducer, useRef, useState } from 'react';
import { getJob } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';

type Entry = { status: 'loading' | 'ready' | 'error'; job?: RecipeJob; started?: boolean };

// Page-local queue. Outstanding obsolete requests retain their slots until settled.
export function useReviewJobMetadata(projectId: string, jobIds: string[]) {
  const [, redraw] = useReducer(value => value + 1, 0);
  const [generation, refresh] = useReducer(value => value + 1, 0);
  const [state] = useState(() => ({
    projectId, generation, entries: new Map<string, Entry>(),
    active: new Set<string>(), mounted: false,
  }));
  const pump = useRef(() => {});
  pump.current = () => {
    if (!state.mounted) return;
    for (const [id, entry] of state.entries) {
      if (state.active.size >= 4) break;
      const tenant = state.projectId;
      const key = JSON.stringify([tenant, id]);
      if (entry.status !== 'loading' || entry.started || state.active.has(key)) continue;
      entry.started = true;
      state.active.add(key);
      getJob(tenant, id).then(job => {
        if (state.mounted && state.projectId === tenant && state.entries.get(id) === entry) {
          entry.status = 'ready'; entry.job = job;
        }
      }).catch(() => {
        if (state.mounted && state.projectId === tenant && state.entries.get(id) === entry) entry.status = 'error';
      }).finally(() => {
        state.active.delete(key);
        if (state.mounted) { redraw(); pump.current(); }
      });
    }
  };
  useEffect(() => {
    state.mounted = true;
    return () => { state.mounted = false; state.entries.clear(); };
  }, [state]);
  const idsKey = JSON.stringify([...new Set(jobIds)].sort());
  useEffect(() => {
    if (state.projectId !== projectId || state.generation !== generation) {
      state.entries.clear(); state.projectId = projectId; state.generation = generation;
    }
    const ids = new Set<string>(JSON.parse(idsKey));
    for (const id of state.entries.keys()) if (!ids.has(id)) state.entries.delete(id);
    for (const id of ids) if (!state.entries.has(id)) state.entries.set(id, { status: 'loading' });
    pump.current(); redraw();
  }, [projectId, generation, idsKey, state]);

  const current = state.projectId === projectId && state.generation === generation;
  return { metadata: current ? state.entries : new Map<string, Entry>(), refresh };
}
