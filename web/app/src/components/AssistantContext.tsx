import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import type { AssistantSource } from '@colony2/shared';
import AssistantPanel from './AssistantPanel';

const Context = createContext<((source?: AssistantSource) => void) | undefined>(undefined);
export function AssistantHost({ tenantId, children }: { tenantId: string; children: ReactNode }) {
  const [source, setSource] = useState<AssistantSource>();
  return <Context.Provider value={setSource}>
    {children}
    <AssistantPanel tenantId={tenantId} pageSource={source} />
  </Context.Provider>;
}
// Only page components with an actual loaded request publish exact-review context.
// Existing isolated page tests and embeds need not mount an assistant host.
export function useAssistantSource(source?: AssistantSource) {
  const setSource = useContext(Context);
  const kind = source?.kind;
  const job = source?.job_id;
  const request = source?.request_id;
  useEffect(() => {
    setSource?.(kind ? { kind, job_id: job, request_id: request } : undefined);
    return () => setSource?.(undefined);
  }, [setSource, kind, job, request]);
}
