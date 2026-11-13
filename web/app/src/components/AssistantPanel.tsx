import { Link, useInRouterContext } from 'react-router-dom';
import { useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import {
  assistantCapabilities, listAssistantSessions, getAssistantSession, replayAssistantSession,
  startAssistantSession, respondAssistantSession, reopenAssistantSession, closeAssistantSession, assistantArtifactURL,
  freezeAssistantStart, freezeAssistantSubmission, freezeAssistantLifecycle, mergeAssistantReplay, AssistantAPIError,
  type AssistantEntry, type AssistantSession, type AssistantSource, type AssistantStart,
  type AssistantSubmission, type AssistantLifecycleRequest,
} from '@colony2/shared';
import { useAssistantPoll } from '../hooks/useAssistantPoll';
import './AssistantPanel.css';

function label(source: AssistantSource): string {
  if (source.kind === 'global') return 'No page evidence';
  if (source.kind === 'review') return `Review ${source.job_id} · request ${source.request_id}`;
  return `Story ${source.job_id}`;
}
const finalStatuses = new Set(['completed', 'finished', 'failed', 'cancelled', 'canceled', 'terminated', 'closed']);
function displayStatus(session?: AssistantSession): string {
  if (!session) return 'New conversation';
  if (session.status.toLowerCase() === 'closing') return 'Closing';
  if (session.status.toLowerCase() === 'closed') return 'Closed';
  if (session.current_request_id) return 'Waiting for your message';
  const status = session.status.toLowerCase();
  if (status === 'submitted' || status === 'ready' || status === 'queued') return 'Queued';
  if (status === 'running' || status === 'processing' || status === 'active') return 'Processing';
  if (status === 'completed' || status === 'finished') return 'Finished';
  if (status === 'failed' || status === 'terminated') return 'Failed';
  if (status === 'cancelled' || status === 'canceled') return 'Cancelled';
  return 'Status unavailable';
}
export function safeAssistantLink(href: string | undefined): string | undefined {
  if (!href) return undefined;
  // Source citations are rendered separately using recorded artifact IDs. Markdown
  // may link only to safe external protocols or same-origin application pages.
  if (/^https?:\/\//i.test(href) || /^mailto:/i.test(href)) return href;
  if (/^\/project\//.test(href) && !href.includes('\\')) return href;
  return undefined;
}

function AssistantAppLink({ to, children }: { to: string; children: React.ReactNode }) {
  const inRouter = useInRouterContext();
  return inRouter ? <Link to={to}>{children}</Link> : <a href={to}>{children}</a>;
}

export default function AssistantPanel({ tenantId, pageSource }: { tenantId: string; pageSource?: AssistantSource }) {
  const [open, setOpen] = useState(false);
  const [opened, setOpened] = useState(false);
  const [available, setAvailable] = useState<boolean>();
  const generation = useRef<string>();
  const [unresolved, setUnresolved] = useState(false);
  const [lostNotice, setLostNotice] = useState(false);
  const [session, setSession] = useState<AssistantSession>();
  const [entries, setEntries] = useState<AssistantEntry[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [hasMore, setHasMore] = useState(false);
  const [draft, setDraft] = useState('');
  const [include, setInclude] = useState(false);
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [lifecyclePending, setLifecyclePending] = useState<'close' | 'reopen'>();
  const lifecycle = useRef<{ action: 'close' | 'reopen'; input: AssistantLifecycleRequest }>();
  const execution = useRef('');
  const [error, setError] = useState('');
  const [staleReview, setStaleReview] = useState(false);
  const [history, setHistory] = useState<AssistantSession[]>([]);
  const [historyCursor, setHistoryCursor] = useState<string>();
  const [historyBusy, setHistoryBusy] = useState(false);
  const epoch = useRef(0);
  const live = useRef(true);
  const locked = useRef(false);
  const start = useRef<AssistantStart>();
  const submission = useRef<AssistantSubmission>();
  const acceptedRequest = useRef<string>();
  const launcher = useRef<HTMLButtonElement>(null);
  const dialog = useRef<HTMLElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  // Tenant-keyed mounting is mandatory at the host. This extra guard also makes
  // the standalone component safe if its tenant prop changes without a key.
  const attachedTenant = useRef(tenantId);
  const attached = (version: number) => live.current && epoch.current === version && attachedTenant.current === tenantId;
  useEffect(() => {
    live.current = true;
    const controller = new AbortController();
    const tenant = tenantId;
    assistantCapabilities(tenantId, controller.signal).then(value => {
      if (live.current && !controller.signal.aborted && attachedTenant.current === tenant) { setAvailable(value.available); generation.current = value.generation; }
    }).catch(() => { if (live.current && !controller.signal.aborted && attachedTenant.current === tenant) setAvailable(false); });
    return () => { live.current = false; epoch.current++; controller.abort(); };
  }, [tenantId]);
  useEffect(() => {
    if (attachedTenant.current !== tenantId) {
      attachedTenant.current = tenantId; epoch.current++;
      setSession(undefined); setEntries([]); setDraft(''); setInclude(false);
      setHistory([]); setHistoryCursor(undefined); setAvailable(undefined);
      setLostNotice(false); setUnresolved(false); generation.current = undefined; setUncertain(false); setBusy(false); setHistoryBusy(false); setStaleReview(false); setError('');
      start.current = undefined; submission.current = undefined; acceptedRequest.current = undefined; locked.current = false;
      lifecycle.current = undefined; setLifecyclePending(undefined); execution.current = '';
    }
  }, [tenantId]);
  useEffect(() => { setInclude(false); }, [pageSource?.kind, pageSource?.job_id, pageSource?.request_id]);
  useEffect(() => {
    if (!open) return;
    const opener = launcher.current;
    const focusable = () => Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled),a[href],textarea:not(:disabled),input:not(:disabled),[tabindex="0"]') ?? [])
      .filter(element => !element.closest('[hidden]'));
    const contain = (event: FocusEvent) => {
      if (event.target instanceof Node && !dialog.current?.contains(event.target)) (input.current?.disabled ? focusable()[0] : input.current)?.focus();
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { event.preventDefault(); setOpen(false); }
      if (event.key === 'Tab') {
        const all = focusable(); const index = all.indexOf(document.activeElement as HTMLElement);
        if (event.shiftKey && index <= 0) { event.preventDefault(); all[all.length - 1]?.focus(); }
        else if (!event.shiftKey && (index < 0 || index === all.length - 1)) { event.preventDefault(); all[0]?.focus(); }
      }
    };
    (input.current?.disabled ? focusable()[0] : input.current)?.focus();
    document.addEventListener('focusin', contain); document.addEventListener('keydown', key);
    return () => {
      document.removeEventListener('focusin', contain); document.removeEventListener('keydown', key);
      opener?.focus({ preventScroll: true });
    };
  }, [open, available]);

  const acceptSession = (value: AssistantSession, replaceReplay = true) => {
    const nextExecution = `${value.id}/${value.job_id}`;
    if (execution.current && execution.current !== nextExecution) {
      acceptedRequest.current = undefined;
      // Old unconfirmed sends must not be automatically retargeted to a new job.
      submission.current = undefined; setUncertain(false);
    }
    execution.current = nextExecution;
    const pending = submission.current;
    if (pending && value.replay?.entries?.some(entry => entry.receipt?.request_id === pending.request_id &&
      entry.receipt.submission_id === pending.submission_id && ['accepted', 'duplicate'].includes(entry.receipt.outcome))) {
      acceptedRequest.current = pending.request_id;
      submission.current = undefined; setUncertain(false); setDraft('');
    }
    if (value.current_request_id && value.current_request_id !== acceptedRequest.current) acceptedRequest.current = undefined;
    setSession(value.current_request_id === acceptedRequest.current ? { ...value, current_request_id: undefined } : value);
    if (replaceReplay) {
      setEntries(value.replay?.entries ?? []);
      setCursor(value.replay?.cursor); setHasMore(value.replay?.has_more ?? false);
    }
  };
  const fail = (failure: unknown) => {
    if (failure instanceof AssistantAPIError && failure.code === 'unresolved') {setUnresolved(true);setLostNotice(true);}
    if (failure instanceof AssistantAPIError && failure.code === 'forbidden') {
      epoch.current++; setSession(undefined); setEntries([]); setHistory([]); setHistoryCursor(undefined);
      lifecycle.current = undefined; setLifecyclePending(undefined); execution.current = '';
      setDraft(''); setInclude(false); setAvailable(false); setBusy(false); setUncertain(false); locked.current = false; start.current = undefined; submission.current = undefined; acceptedRequest.current = undefined;
    }
    setError(failure instanceof AssistantAPIError ? failure.message : 'Connection interrupted. Refresh to recover accepted messages.');
  };
  const refresh = useAssistantPoll(`${tenantId}/${session?.id ?? ''}/${session?.job_id ?? ''}`, open && available === true && !!session && !finalStatuses.has(session.status.toLowerCase()), async signal => {
    const version = epoch.current;
    const id = session!.id;
    const observedExecution = execution.current;
    try {
      const value = await getAssistantSession(tenantId, id, signal);
      if (!attached(version) || signal.aborted || execution.current !== observedExecution) return;
      // Get returns the authoritative latest page. Preserve a loaded continuation
      // only when its watermark matches; changed history must be replayed again.
      acceptSession(value, value.replay?.watermark !== session?.replay?.watermark);
      if (!submission.current && !start.current && !lifecycle.current) setError('');
    } catch (failure) {
      if (attached(version) && !signal.aborted && execution.current === observedExecution) fail(failure);
      throw failure;
    }
  });
  const refreshOnce = async () => {
    if (!session || locked.current) return;
    if (!finalStatuses.has(session.status.toLowerCase())) { refresh(); return; }
    const version = epoch.current;
    locked.current = true; setBusy(true);
    try { const value = await getAssistantSession(tenantId, session.id); if (attached(version)) { acceptSession(value); setError(''); } }
    catch (failure) { if (attached(version)) fail(failure); }
    finally { if (attached(version)) { locked.current = false; setBusy(false); } }
  };
  const loadHistory = async (more = false) => {
    if (historyBusy || !available) return;
    const version = epoch.current;
    setHistoryBusy(true);
    try {
      const page = await listAssistantSessions(tenantId, { cursor: more ? historyCursor : undefined, limit: 20 });
      if (!attached(version)) return;
      setHistory(old => more ? [...old, ...(page.sessions ?? [])] : (page.sessions ?? [])); setHistoryCursor(page.cursor);
    } catch (failure) { if (attached(version)) fail(failure); }
    finally { if (attached(version)) setHistoryBusy(false); }
  };
  const select = async (id: string) => {
    if (locked.current || (lifecycle.current && !unresolved)) return;
    lifecycle.current = undefined; setLifecyclePending(undefined); setUnresolved(false);
    const version = ++epoch.current;
    setSession(undefined); setEntries([]); setError(''); setBusy(true); setHistoryBusy(false);
    start.current = undefined; submission.current = undefined; acceptedRequest.current = undefined; setUncertain(false); // Preserve unsent text for deliberate use.
    locked.current = true;
    try { const caps = await assistantCapabilities(tenantId);
      if (!attached(version)) return; generation.current = caps.generation;
      const value = await getAssistantSession(tenantId, id); if (attached(version)) acceptSession(value); }
    catch (failure) { if (attached(version)) fail(failure); }
    finally { if (attached(version)) { locked.current = false; setBusy(false); } }
  };
  const reset = async () => {
    if (locked.current || (lifecycle.current && !unresolved)) return;
    const version = epoch.current; locked.current = true; setBusy(true);
    try {
      const caps = await assistantCapabilities(tenantId);
      if (!attached(version)) return;
      generation.current = caps.generation;
    } catch (failure) { if (attached(version)) fail(failure); return; }
    finally { if (attached(version)) { locked.current = false; setBusy(false); } }
    lifecycle.current = undefined; setLifecyclePending(undefined);
    setUnresolved(false);
    execution.current = '';
    epoch.current++; start.current = undefined; submission.current = undefined; acceptedRequest.current = undefined;
    setSession(undefined); setEntries([]); setCursor(undefined); setHasMore(false);
    setError(''); setUncertain(false); setInclude(false); setHistoryBusy(false); setStaleReview(false);
  };
  const send = async (requestedCommand: 'send' | 'finish' = 'send') => {
    const command = submission.current?.command ?? requestedCommand;
    if (unresolved || locked.current || lifecycle.current || session?.status.toLowerCase() === 'closing' || !available || (!draft.trim() && command === 'send')) return;
    if (session && (!session.current_request_id || (session.lifecycle === 'goal' && finalStatuses.has(session.status.toLowerCase())))) return;
    const version = epoch.current;
    const observedExecution = execution.current;
    locked.current = true; setBusy(true); setError('');
    try {
      if (!session) {
        if (!start.current) {
          const caps = await assistantCapabilities(tenantId);
          if (!attached(version) || execution.current !== observedExecution) return;
          if (!caps.available) throw new AssistantAPIError(503, 'unavailable');
          // Only a new operation reads capabilities. An uncertain retry must
          // retain its original generation, identity, source and destination.
          start.current = freezeAssistantStart(draft, include && pageSource ? pageSource : { kind: 'global' }, caps.generation);
        }
        const value = await startAssistantSession(tenantId, start.current);
        if (!attached(version)) return;
        acceptSession(value);
      } else {
        const frozen = submission.current ??= freezeAssistantSubmission(session.current_request_id!, command, draft);
        const receipt = await respondAssistantSession(tenantId, session.id, frozen);
        if (!attached(version) || execution.current !== observedExecution) return;
        if (receipt.request_id !== frozen.request_id || receipt.submission_id !== frozen.submission_id || !['accepted', 'duplicate'].includes(receipt.outcome)) {
          setUncertain(true); setError('The receipt did not confirm this message. Refresh before retrying.'); refresh(); return;
        }
        // The consumed request must not become editable again during the brief
        // interval before the worker publishes its next prompt.
        acceptedRequest.current = receipt.request_id;
        // No optimistic turn is appended. The runtime replay publishes the text.
        setSession(old => old ? { ...old, current_request_id: undefined } : old);
        refresh();
      }
      if (attached(version)) { setDraft(''); setUncertain(false); start.current = undefined; submission.current = undefined; }
    } catch (failure) {
      if (!attached(version) || (observedExecution && execution.current !== observedExecution)) return;
      setUncertain(!!start.current || !!submission.current); fail(failure);
      if (!session && start.current?.source.kind === 'review' && failure instanceof AssistantAPIError && failure.code === 'conflict') {
        setStaleReview(true); setUncertain(false);
        setError('This review request changed. Reload the page and select its evidence again.');
      }
      // Reconcile authoritative state before offering the same frozen request.
      if (session) refresh();
    } finally { if (attached(version)) { locked.current = false; setBusy(false); } }
  };
  const changeLifecycle = async (action: 'close' | 'reopen') => {
    if (!session || locked.current || unresolved) return;
    if (lifecycle.current && lifecycle.current.action !== action) return;
    const version = ++epoch.current; // Ignore reads that began before this operation.
    const observedExecution = execution.current;
    locked.current = true; setBusy(true); setError('');
    try {
      if (!lifecycle.current) {
        let currentGeneration = generation.current;
        if (action === 'reopen' && session.lifecycle === 'goal') {
          const caps = await assistantCapabilities(tenantId);
          if (!attached(version) || execution.current !== observedExecution) return;
          if (!caps.available) throw new AssistantAPIError(503, 'unavailable');
          currentGeneration = caps.generation;
        }
        lifecycle.current = { action, input: freezeAssistantLifecycle(session.job_id, currentGeneration) };
      }
      const frozen = lifecycle.current;
      setLifecyclePending(action);
      const value = action === 'close'
        ? await closeAssistantSession(tenantId, session.id, frozen.input)
        : await reopenAssistantSession(tenantId, session.id, session.lifecycle === 'goal' ? frozen.input : undefined);
      if (!attached(version)) return;
      if (value.id !== session.id && session.lifecycle === 'goal') {
        setError('Conversation changed. Reconcile this request before continuing.'); return;
      }
      if (action === 'reopen' && session.lifecycle === 'goal' && value.job_id === frozen.input.job_id) {
        setError('Reopen has not confirmed a new execution. Retry to reconcile the same request.'); return;
      }
      // Invalidate reads started while cancellation/submission was in flight,
      // including a stale status for the same job after close succeeded.
      locked.current = false; setBusy(false); epoch.current++;
      acceptSession(value);
      lifecycle.current = undefined; setLifecyclePending(undefined); setError('');
      if (action === 'close' && value.job_id === frozen.input.job_id && finalStatuses.has(value.status.toLowerCase())) setOpen(false);
    } catch (failure) {
      if (attached(version)) {
        fail(failure);
        if (lifecycle.current && !(failure instanceof AssistantAPIError && failure.code === 'unresolved')) setError(`${action === 'close' ? 'Close' : 'Reopen'} outcome is uncertain. Retry to reconcile the same request.`);
      }
    } finally { if (attached(version)) { locked.current = false; setBusy(false); } }
  };
  const moreReplay = async () => {
    if (!session || !hasMore || locked.current) return;
    const version = epoch.current;
    const observedExecution = execution.current;
    locked.current = true; setBusy(true);
    try {
      const page = await replayAssistantSession(tenantId, session.id, { cursor, limit: 50 });
      if (attached(version) && execution.current === observedExecution) { setEntries(old => mergeAssistantReplay(old, page)); setCursor(page.cursor); setHasMore(page.has_more); }
    } catch (failure) { if (attached(version) && execution.current === observedExecution) fail(failure); }
    finally { if (attached(version)) { locked.current = false; setBusy(false); } }
  };
  // A prop change must hide old tenant content even before effect cleanup.
  const detached = attachedTenant.current !== tenantId;
  const view = detached ? undefined : session;
  const viewEntries = detached ? [] : entries;
  const sourceLabel = view ? label(view.source) : 'No page evidence';
  const inputUnavailable = !!view && (!view.current_request_id || view.status.toLowerCase() === 'closing' || (view.lifecycle === 'goal' && finalStatuses.has(view.status.toLowerCase())));
  return <>
    <button ref={launcher} className="assistant-launcher" aria-label="Open assistant" aria-expanded={open} onClick={() => { setOpened(true); setOpen(true); }}>Discuss</button>
    {opened && <aside ref={dialog} hidden={!open} className="assistant-panel" role="dialog" aria-modal="true" aria-label="Contextual assistant">
      <header><h2>Assistant</h2><button aria-label="Minimize assistant" onClick={() => setOpen(false)}>Minimize</button></header>
      <p>Discuss evidence. Review decisions stay on the review page.</p>
      {available !== true && <p role="status">{available === undefined ? 'Checking assistant availability…' : 'Assistant is unavailable. Your other pages remain available.'}</p>}
      <div className="assistant-toolbar">
        <button disabled={busy || unresolved || !!lifecyclePending} onClick={() => void reset()}>New conversation</button>
        <button disabled={!available || busy || historyBusy} onClick={() => void loadHistory()}>Conversation history</button>
        <button disabled={!view || busy} onClick={() => void refreshOnce()}>Refresh conversation</button>
      </div>
      {!detached && history.length > 0 && <nav aria-label="Conversation history"><ul>{history.map(value => <li key={`${value.id}/${value.job_id}`}>
        <button disabled={busy || (!!lifecyclePending && !unresolved)} onClick={() => void select(value.id)}>{label(value.source)} · {value.status}</button>
      </li>)}</ul>{historyCursor && <button disabled={historyBusy} onClick={() => void loadHistory(true)}>More conversations</button>}</nav>}
      <section aria-label="Captured context"><strong>{sourceLabel}</strong>
        {view?.captured_at && <p>Captured {new Date(view.captured_at).toLocaleString()}</p>}
        {!view && pageSource && <label><input type="checkbox" checked={include} disabled={busy || uncertain} onChange={event => setInclude(event.target.checked)} /> Include this {pageSource.kind === 'review' ? 'exact review and its documents' : 'story'}</label>}
        {view && pageSource && label(pageSource) !== sourceLabel && <p>Page changed. Start a new conversation to include this page.</p>}
        {!!view?.coverage?.length && <details><summary>Source coverage ({view.coverage.length})</summary><ul>{view.coverage.map(source => <li key={source.id}>
          <strong>{source.name}</strong>: {source.outcome}{source.detail && <p>{source.detail}</p>}
          {source.producer_job_id && <p>Producer: {source.producer_job_id}{source.producer_task && ` · ${source.producer_task}`}</p>}
          {source.artifact_id && <a href={assistantArtifactURL(tenantId, view.id, source.artifact_id)} download>Download {source.name}</a>}
        </li>)}</ul></details>}
      </section>
      <p role="status">{lifecyclePending ? (busy ? `${lifecyclePending === 'close' ? 'Closing' : 'Reopening'} conversation…` : `${lifecyclePending === 'close' ? 'Close' : 'Reopen'} needs reconciliation`) : busy ? 'Sending request…' : displayStatus(view)}</p>
      {view && <AssistantAppLink to={`/project/${encodeURIComponent(tenantId)}/jobs/${encodeURIComponent(view.job_id)}/story`}>View execution</AssistantAppLink>}
      {staleReview && <button onClick={() => window.location.reload()}>Reload review</button>}
      {lostNotice && !unresolved && <p role="status">The earlier request remains unresolved. Selecting this recorded conversation does not confirm that request. Your draft is kept for deliberate use.</p>}
      {unresolved && <button disabled={busy} onClick={() => void reset()}>Start a separate conversation</button>}
      {error && <p role="alert">{error}{uncertain && !unresolved && ' Delivery is uncertain. Retry uses the same message identity.'}</p>}
      <div className="assistant-transcript" aria-label="Conversation transcript" aria-live="polite">{viewEntries.map(entry => <article key={entry.key}>
        <strong>{entry.role}</strong> <span>{entry.status}</span>
        {entry.status === 'unavailable' && <p role="status">A reply is unavailable. You can send another message.</p>}
        <ReactMarkdown skipHtml urlTransform={safeAssistantLink} components={{
          img: ({ alt }) => <span>{alt || 'Image omitted'}</span>,
          a: ({ href, children }) => safeAssistantLink(href) ? (href?.startsWith('/project/') ? <AssistantAppLink to={href}>{children}</AssistantAppLink> : <a href={safeAssistantLink(href)} target="_blank" rel="noopener noreferrer">{children}</a>) : <span>{children}</span>,
        }}>{entry.text || ''}</ReactMarkdown>
        {!!entry.citations?.length && <ul aria-label="Cited sources">{entry.citations.map(id => {
          const source = view?.coverage?.find(source => source.id === id);
          return <li key={id}>{source?.artifact_id && view ? <a href={assistantArtifactURL(tenantId, view.id, source.artifact_id)} download>Cited source: {source.name}</a> : <span>Cited source: {source?.name ?? id}</span>}</li>;
        })}</ul>}
        {entry.artifacts?.map(artifact => artifact.available && view
          ? <a key={artifact.id} href={assistantArtifactURL(tenantId, view.id, artifact.id)} download>{artifact.name}</a>
          : <span key={artifact.id}>{artifact.name} · unavailable</span>)}
      </article>)}</div>
      {hasMore && <button disabled={busy} onClick={() => void moreReplay()}>Load earlier messages</button>}
      <form onSubmit={event => { event.preventDefault(); void send(); }}>
        <label htmlFor="assistant-message">Message</label>
        <textarea ref={input} id="assistant-message" value={detached ? '' : draft} disabled={!available || unresolved || busy || !!lifecyclePending || inputUnavailable || uncertain || staleReview || (!!view && !view.current_request_id)} maxLength={32_768} onChange={event => setDraft(event.target.value)} />
        <button type="submit" disabled={!available || unresolved || busy || !!lifecyclePending || inputUnavailable || staleReview || (!draft.trim() && !submission.current) || (!!view && !view.current_request_id)}>{uncertain ? (submission.current?.command === 'finish' ? 'Retry finish' : 'Retry same message') : 'Send message'}</button>
      </form>
      {view?.current_request_id && view.lifecycle !== 'goal' && <button disabled={busy || uncertain} onClick={() => void send('finish')}>Finish conversation</button>}
      {view && lifecyclePending && <button disabled={busy || unresolved} onClick={() => void changeLifecycle(lifecyclePending)}>Retry {lifecyclePending}</button>}
      {view?.lifecycle === 'goal' && !finalStatuses.has(view.status.toLowerCase()) && !lifecyclePending && <button disabled={busy || uncertain || view.status.toLowerCase() === 'closing'} onClick={() => void changeLifecycle('close')}>Close conversation</button>}
      {view && finalStatuses.has(view.status.toLowerCase()) && !lifecyclePending && <button disabled={busy} onClick={() => void changeLifecycle('reopen')}>Reopen conversation</button>}
    </aside>}
  </>;
}
