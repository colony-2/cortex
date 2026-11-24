import { createContext, useContext, useMemo } from 'react';
import { useCurrentInput, type HistoryPage } from '@colony2/shared';

export interface ThreadSnapshot {
  page?: HistoryPage;
  error?: string;
  refresh: () => Promise<void>;
}
export const ThreadSnapshotContext = createContext<ThreadSnapshot | undefined>(undefined);

export function hasThreadInput(thread?: ThreadSnapshot) {
  return !!thread && (!thread.page || 'current_input' in thread.page);
}

export function useThreadInput(project: string, job: string, enabled = true, source?: ThreadSnapshot) {
  const context = useContext(ThreadSnapshotContext);
  const thread = source ?? context;
  const unified = hasThreadInput(thread);
  const separate = useCurrentInput(project, job, enabled && !unified);
  const snapshot = useMemo(() => {
    if (!thread || !unified) return undefined;
    const input = thread.page?.current_input;
    const status = thread.error ? 'unavailable' : thread.page ? 'current' : 'loading';
    return {
      details: thread.error ? undefined : input, readableDetails: input,
      version: input ? input.form.request_id ?? thread.page?.snapshot_token : undefined,
      readableVersion: input?.form.request_id, error: thread.error, loading: status === 'loading',
      discovery: {status, error:thread.error}, refresh:thread.refresh,
    } as const;
  }, [thread, unified]);
  return snapshot ?? separate;
}
