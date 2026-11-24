import { useCallback, useEffect, useRef, useState } from 'react';
import { loadArchivedInbox, loadVisibleInboxPage, type InboxJob } from '../utils/jobInboxData';

type InboxPage = {
  key: string;
  jobs: InboxJob[];
  nextPageToken?: string;
  loading: boolean;
  loaded: boolean;
  error?: string;
  retryToken?: string;
};

const emptyJobs: InboxJob[] = [];

export function useInboxJobs(tenant: string, folder: 'all' | 'cell' | 'archive' | 'active' | 'waiting' | 'failed' | undefined,
  cell: { id: string; name: string } | undefined, complete: boolean, revision: number) {
  const cellId = cell?.id, cellName = cell?.name;
  const key = folder && (folder !== 'cell' || cellName) ? JSON.stringify([tenant, folder, cellId, cellName]) : undefined;
  const [page, setPage] = useState<InboxPage>();
  const request = useRef<AbortController>();
  const loaded = useRef<{ key: string; revision: number }>();
  const tokens = useRef(new Set<string>());
  const current = key && page?.key === key ? page : undefined;

  const load = useCallback(async (token?: string) => {
    if (!key || request.current) return;
    const controller = new AbortController();
    request.current = controller;
    if (!token) {
      tokens.current.clear();
      // Starting a different folder replaces the one retained page set.
      if (loaded.current?.key !== key) loaded.current = undefined;
    }
    setPage(previous => ({ key, jobs: emptyJobs, loaded: false, ...(previous?.key === key ? previous : {}), loading: true, error: undefined, retryToken: token }));
    try {
      const next = folder === 'archive' ? { jobs: await loadArchivedInbox(tenant, controller.signal), nextPageToken: undefined }
        : await loadVisibleInboxPage(tenant, cellId && cellName ? { id: cellId, name: cellName } : undefined, token, controller.signal,
          folder === 'active' || folder === 'waiting' || folder === 'failed' ? folder : undefined);
      if (controller.signal.aborted) return;
      if (next.nextPageToken && (token === next.nextPageToken || tokens.current.has(next.nextPageToken))) {
        throw new Error('Inbox page cursor did not advance.');
      }
      if (token) tokens.current.add(token);
      else loaded.current = { key, revision };
      setPage(previous => {
        const jobs = new Map((token && previous?.key === key ? previous.jobs : []).map(job => [job.job_id, job]));
        for (const job of next.jobs) jobs.set(job.job_id, job);
        return { key, jobs: [...jobs.values()], nextPageToken: next.nextPageToken, loaded: true, loading: false };
      });
    } catch (reason) {
      if (!controller.signal.aborted) setPage(previous => previous?.key === key ? {
        ...previous, loading: false, error: reason instanceof Error ? reason.message : String(reason),
      } : previous);
    } finally {
      if (request.current === controller) request.current = undefined;
    }
  }, [key, folder, tenant, cellId, cellName, revision]);

  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active && key && (loaded.current?.key !== key || loaded.current.revision !== revision)) void load();
    });
    return () => {
      active = false;
      request.current?.abort();
      request.current = undefined;
      setPage(previous => previous?.loading ? { ...previous, loading: false } : previous);
    };
  }, [key, load, revision]);

  const nextPageToken = current?.nextPageToken;
  const loading = !!key && (current?.loading ?? true);
  const error = current?.error;
  useEffect(() => {
    if (complete && nextPageToken && !loading && !error && loaded.current?.key === key && loaded.current?.revision === revision) void load(nextPageToken);
  }, [complete, key, nextPageToken, loading, error, revision, load]);

  return { jobs: current?.jobs ?? emptyJobs, loaded: current?.loaded ?? false, loading, error, nextPageToken,
    loadMore: () => { if (nextPageToken && !loading) void load(nextPageToken); },
    retry: () => void load(current?.retryToken),
  };
}
