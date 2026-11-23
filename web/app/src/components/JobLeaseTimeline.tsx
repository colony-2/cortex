import './JobLeaseTimeline.css';
import { useEffect, useRef, useState, useSyncExternalStore } from 'react';
import LeaseTable from './LeaseTable';
import { Button, Space } from 'antd';
import { instancesPath } from '../runners/paths';
import { Link, useLocation } from 'react-router-dom';
import { APIError, getJobLeaseEvents } from '@colony2/shared/api';
import { API_BASE } from '@colony2/shared/apiBase';
import { getUserEmail } from '@colony2/shared/auth';
import type { JobLeaseEvent } from '@colony2/shared/types';

function subscribeIdentity(changed: () => void) {
  // UI selection signal only; this is not an authenticated principal.
  window.addEventListener('cortex-ui-identity', changed);
  window.addEventListener('focus', changed);
  window.addEventListener('storage', changed);
  return () => {
    window.removeEventListener('cortex-ui-identity', changed);
    window.removeEventListener('focus', changed);
    window.removeEventListener('storage', changed);
  };
}
function readable(value?: string) {
  if (!value || value.startsWith('0001-') || !Number.isFinite(Date.parse(value))) return 'Recorded time unavailable';
  return new Date(value).toLocaleString();
}
type Props = { projectId: string; jobId: string; createdAt?: string; pageSize?: number; expanded?: boolean };
type State = { open: boolean; loading: boolean; rows: JobLeaseEvent[]; token?: string; error?: Error; retryToken?: string; loaded: boolean };
const empty = (): State => ({ open: false, loading: false, rows: [], loaded: false });

export default function JobLeaseTimeline(props: Props) {
  const location = useLocation();
  const identity = useSyncExternalStore(subscribeIdentity, getUserEmail, () => null);
  const pageSize = props.pageSize ?? 100;
  return <Timeline key={JSON.stringify([API_BASE, identity, location.key, props.projectId, props.jobId, pageSize])} {...props} pageSize={pageSize} />;
}

function Timeline({ projectId, jobId, createdAt, pageSize, expanded = false }: Props) {
  const [state, setState] = useState<State>(() => ({ ...empty(), open: expanded }));
  const generation = useRef(0);
  const pending = useRef<AbortController>();
  const invalidate = () => { ++generation.current; pending.current?.abort(); pending.current = undefined; };
  useEffect(() => {
    // Let StrictMode replay and the shell's identity remount settle together.
    // A child identity update can otherwise start a read that its parent
    // immediately cancels while replacing the inbox provider.
    const initialRead = expanded ? window.setTimeout(() => load(undefined, true), 0) : undefined;
    return () => { window.clearTimeout(initialRead); invalidate(); };
  }, [expanded]);

  function load(token?: string, reset = false) {
    if (pending.current && !reset) return;
    if (reset) invalidate();
    const request = ++generation.current;
    const controller = new AbortController();
    pending.current = controller; // Synchronous guard, before any render/effect.
    setState(previous => ({ ...(reset ? empty() : previous), open: true, loading: true, error: undefined, retryToken: token }));
    void getJobLeaseEvents(projectId, jobId, { pageSize, pageToken: token }, controller.signal).then(page => {
      if (generation.current !== request) return;
      pending.current = undefined;
      setState(previous => {
        const rows = [...previous.rows];
        const seen = new Set(rows.map(row => row.eventId));
        for (const row of page.events) if (!seen.has(row.eventId)) { rows.push(row); seen.add(row.eventId); }
        return { open: true, rows, token: page.nextPageToken, loading: false, loaded: true };
      });
    }).catch(error => {
      if (generation.current !== request) return;
      pending.current = undefined;
      setState(previous => ({ ...previous, loading: false, error: error instanceof Error ? error : new Error('Lease history unavailable') }));
    });
  }
  const invalidCursor = state.error instanceof APIError && state.error.code === 'invalid_lease_event_page_token';
  return <section aria-label="Lease timeline" className="job-lease-timeline">
    {!expanded && <Button aria-expanded={state.open} onClick={() => {
      if (state.open) { invalidate(); setState(empty()); }
      else load(undefined, true);
    }}>Timeline</Button>}
    {state.open && <div>
      <p><strong>Job created</strong>: {readable(createdAt)}{createdAt && readable(createdAt) !== 'Recorded time unavailable' && <> · <time dateTime={createdAt}>{createdAt}</time></>}</p>
      <p>Collection and retention may omit earlier activity. Renewal counts cover the loaded events.</p>
      <Space wrap className="lease-actions"><Button onClick={() => load(undefined, true)}>Refresh timeline</Button>
        <Link to={instancesPath(projectId, {jobId, status: 'all'})}>All launches for this job</Link></Space>
      {state.loading && <p role="status">Loading lease history…</p>}
      {state.error && <div role="alert">
        <p>Lease history unavailable. {state.error.message}</p>
        {invalidCursor ? <Button onClick={() => load(undefined, true)}>Refresh from first page</Button> :
          <Button disabled={state.loading} onClick={() => load(state.retryToken)}>Retry lease history</Button>}
      </div>}
      {state.loaded && !state.loading && !state.error && state.rows.length === 0 && <p>No retained lease events are available.</p>}
      {state.rows.length > 0 && <LeaseTable events={state.rows} projectId={projectId} jobId={jobId}/>}
      {state.token && <Button disabled={state.loading || !!state.error} onClick={() => load(state.token)}>Load older lease events</Button>}
    </div>}
  </section>;
}
