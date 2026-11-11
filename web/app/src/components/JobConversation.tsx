import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Typography } from 'antd';
import { APIError, conversationArtifactURL, getConversation, getConversationEntry, getConversationRequest, stringifyJobJSON, type HistoryDetail, type HistoryEntry, type HistoryPage, type HistoryValue, type HistoryMerge } from '@colony2/shared';

import type { RecipeJob } from '@colony2/shared/types';
import { isFinishedJob, jobStatusPresentation, jobStatusValue } from '../utils/jobStatus';
import ReviewDocuments from './ReviewDocuments';
import FinalDocuments from './FinalDocuments';
import { historicalDocuments } from './historicalDocuments';
import { object, valueText, recordedTime, savedAnswers, reviewer } from './conversationPresentation';
import './conversation.css';
function RecordedValue({ value }: { value: HistoryValue }) {
  return value.present ? <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{valueText(value.raw)}</pre> : <span>Not recorded</span>;
}
function Time({value, reported = false}: {value: HistoryValue; reported?: boolean}) {
  const time = recordedTime(value);
  return <span title={time.title}>{reported ? 'Reported time: ' : ''}{time.text}</span>;
}
export function HistoryPair({ entry, onOpen, part }: { entry: HistoryEntry; onOpen?: () => void; part?: 'request' | 'response' }) {
  const [expanded, setExpanded] = useState(false);
  const answer = savedAnswers(entry.form_fields, entry.accepted_response.raw, true);
  const receipt = object(entry.actor.receipt);
  return <div className="conversation-pair" data-occurrence={entry.entry_id}>
    {part !== 'response' && <Card size="small" className="conversation-step" aria-label="Recorded request step">
      <div className="conversation-line"><strong>{entry.title.present ? valueText(entry.title.raw) : `${entry.kind} request`}</strong>{onOpen && <Button size="small" onClick={onOpen}>{entry.kind === 'review' ? 'Open review' : 'Open recorded request'}</Button>}</div>
      <details><summary className="conversation-line conversation-detail-summary"><Time value={entry.requested_at} /><span>State: {entry.state || 'unknown'} · Details</span></summary>
        <p>Occurrence: {entry.entry_id}</p><p>Attempt: {entry.locator.job_attempt ?? 'not recorded'}</p>
        <pre>{valueText(entry.locator)}</pre>
        {entry.state_evidence?.map((e,i) => <p key={i}>{e}</p>)}
        <RecordedValue value={{present:Object.keys(entry.form_fields ?? {}).length > 0,raw:entry.form_fields}} />
        <p>Response source: {entry.response_source || 'not recorded'}</p>
        <p>{reviewer(entry)}</p>
        {entry.actor.receipt !== undefined && <pre>{valueText(entry.actor.receipt)}</pre>}
        <RecordedValue value={entry.accepted_response} />
        {entry.diagnostics?.map((d,i) => <p key={i}>{d.message}</p>)}
      </details>
    </Card>}
    {part !== 'request' && entry.accepted_response.present && <Card size="small" className="conversation-step conversation-response" aria-label="Accepted response">
      <div className={expanded ? 'conversation-answer expanded' : 'conversation-answer'}>{answer}</div>
      <div className="conversation-line"><Time value={{present:Object.prototype.hasOwnProperty.call(receipt,'submitted_at'),raw:receipt.submitted_at}} reported={entry.actor.provenance === 'reported_receipt'} /><span title={reviewer(entry)}>{reviewer(entry)}</span></div>
      <button className="conversation-expand" onClick={() => setExpanded(v => !v)}>{expanded ? 'Collapse feedback' : 'Expand feedback'}</button>
    </Card>}
  </div>;
}
export function MergeCard({ merge }: { merge: HistoryMerge }) {
  const subject = merge.candidate_subject;
  const snippet = subject == null || subject === '' ? 'Commit message not recorded' : subject.length > 160 ? `${subject.slice(0,160)}…` : subject;
  return <Card size="small" className="conversation-step" aria-label="Verified publication" data-event={merge.entry_id}>
    <strong>{merge.publication_disposition === 'already_present' ? 'Publication confirmed — already present' : 'Published'}</strong>
    <p style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{snippet}</p>
    <details><summary>Publication details</summary>
      <p>Commit: {merge.candidate_sha}</p><p>Branch: {merge.publication_branch}</p>
      <p>Commit message: {subject == null || subject === '' ? 'Commit message not recorded' : subject}</p>
      <p>Job: {merge.job_id} · Attempt: {merge.job_attempt} · Task attempt: {merge.task_attempt}</p>
      <p style={{overflowWrap:'anywhere'}}>Source: {merge.invocation_id}</p><p>Result ordinal: {String(merge.result_ordinal)}</p>
      <Time value={{present:!!merge.completed_at,raw:merge.completed_at}} />
    </details>
  </Card>;
}
export function ConversationTimeline({page, entries, onOpen}: {page?: HistoryPage; entries: HistoryEntry[]; onOpen:(id:string)=>void}) {
  if (!page?.events?.length) return <>{entries.map(entry => <HistoryPair key={entry.entry_id} entry={entry} onOpen={() => onOpen(entry.entry_id)} />)}</>;
  const byID = new Map(entries.map(entry => [entry.entry_id, entry]));
  const merges = new Map((page.merges || []).map(merge => [merge.entry_id, merge]));
  const seen = new Set<string>();
  const positionedRequests = new Set(page.events.filter(e=>e.kind === 'request_generation').map(e=>e.entry_id));
  const positionedResponses = new Set(page.events.filter(e=>e.kind === 'response_acceptance').map(e=>e.entry_id));
  const missingPositions = entries.filter(e=>!positionedRequests.has(e.entry_id) || (e.accepted_response.present && !positionedResponses.has(e.entry_id)));
  return <>
    {missingPositions.length > 0 && <section aria-label="Events without retained positions"><p>Some event positions were not recorded. These occurrences remain available separately from the ordered events.</p>{missingPositions.map(entry => <div key={entry.entry_id}>
      {!positionedRequests.has(entry.entry_id) && <HistoryPair entry={entry} part="request" onOpen={()=>onOpen(entry.entry_id)} />}
      {entry.accepted_response.present && !positionedResponses.has(entry.entry_id) && <HistoryPair entry={entry} part="response" />}
    </div>)}</section>}
    {page.events.map(event => {
    const key = JSON.stringify([event.kind,event.entry_id,event.invocation_id,String(event.ordinal)]);
    if (seen.has(key)) return null;
    seen.add(key);
    if (event.kind === 'merge_completion') {
      const merge = merges.get(event.entry_id);
      return merge ? <MergeCard key={key} merge={merge} /> : null;
    }
    const entry = byID.get(event.entry_id);
    return entry && ['request_generation','response_acceptance'].includes(event.kind) ? <HistoryPair key={key} entry={entry} part={event.kind === 'request_generation' ? 'request' : 'response'} onOpen={() => onOpen(entry.entry_id)} /> : null;
  })}</>;
}
function HistoricalRequest({ tenant, job, entry, legacy, onBack }: { tenant: string; job: string; entry: string; legacy?: boolean; onBack?: () => void }) {
  const [detail, setDetail] = useState<HistoryDetail>();
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  const recoveryUsed = useRef(false);
  const recovering = useRef(false);
  const alive = useRef(true);
  const recoveryController = useRef<AbortController>();
  useEffect(() => { alive.current = true; return () => {alive.current = false; recoveryController.current?.abort();}; }, []);
  useEffect(() => {
    const controller = new AbortController();
    recoveryController.current?.abort();
    setDetail(undefined); setError('');
    (legacy ? getConversationRequest(tenant, job, entry, controller.signal) : getConversationEntry(tenant, job, entry, { signal: controller.signal })).then(d => { if (!controller.signal.aborted) setDetail(d); }).catch(e => { if (!controller.signal.aborted) setError(String(e)); });
    return () => controller.abort();
  }, [tenant, job, entry, legacy, retry]);
  const recover = async (failure: unknown) => {
    if (!(failure instanceof APIError) || failure.code !== 'history_snapshot_expired' || recovering.current || !detail) return;
    if (recoveryUsed.current) { setError('Snapshot recovery failed: the refreshed snapshot expired. Retry request to try again.'); return; }
    recoveryUsed.current = true; recovering.current = true;
    const selected = detail.entry_id;
    const abort = new AbortController(); recoveryController.current = abort;
    try {
      let page = await getConversation(tenant, job, {signal:abort.signal});
      const snapshot = page.snapshot_token;
      let found = page.entries.some(e => e.entry_id === selected);
      while (!found && page.next_page_token) { page = await getConversation(tenant, job, {pageToken:page.next_page_token,signal:abort.signal}); found = page.entries.some(e => e.entry_id === selected); }
      if (!found) throw new Error('The selected occurrence is no longer available in retained history.');
      const fresh = await getConversationEntry(tenant, job, selected, {snapshotToken:snapshot,signal:abort.signal});
      if (alive.current && !abort.signal.aborted) setDetail(fresh);
    } catch (e) { if (alive.current && !abort.signal.aborted) setError(`Snapshot recovery failed: ${String(e)}`); }
    finally {recovering.current = false;}
  };
  if (!detail && error) return <Alert type="warning" message="Request unavailable" description={<>{error} <Button onClick={() => {recoveryUsed.current = false; setRetry(v => v + 1);}}>Retry request</Button></>} />;
  if (!detail) return <p role="status">Loading recorded request…</p>;
  const catalog = historicalDocuments(detail);
  const form = object(detail.raw_form);
  return <section aria-label="Recorded request">
    <Button onClick={onBack}>Back to conversation</Button>
    <Typography.Title level={4}>{detail.title.present ? valueText(detail.title.raw) : 'Recorded request'}</Typography.Title>
    <p>Read only</p>
    {(form.question || Array.isArray(form.fields)) && <section aria-label="Review context" style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{form.question ? valueText(form.question) : form.fields.map((field: unknown) => valueText(object(field).question || object(field).id)).join('\n')}</section>}
    {detail.summary.present && <p>{valueText(detail.summary.raw)}</p>}
    {error && <Alert type="warning" message={error} action={<Button onClick={() => {recoveryUsed.current = false; setError(''); setRetry(v => v + 1);}}>Retry request</Button>} />}
    {catalog.limited && <Alert type="info" message="Limited history: the original form or document metadata was not recorded. Available evidence is shown below." />}
    <ReviewDocuments projectId={tenant} jobId={job} requestId={detail.request_id || detail.entry_id} documents={catalog.documents} documentMetadata={catalog.metadata} overviewDocumentId={catalog.overview} source={{identity:JSON.stringify([tenant,job,detail.entry_id,detail.snapshot_token]),url:id => catalog.members[id] ? conversationArtifactURL(tenant,job,detail.entry_id,catalog.members[id],detail.snapshot_token) : '',onError:failure => {void recover(failure);}}} />
    {detail.accepted_response.present && <section aria-label="Recorded answers"><Typography.Title level={5}>Accepted response</Typography.Title><pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{savedAnswers(Object.keys(form).length ? form : detail.form_fields,detail.accepted_response.raw)}</pre></section>}
    <details><summary>Recorded evidence</summary><HistoryPair entry={detail} /><p>History coverage: {detail.completeness}</p></details>
    {[['Request documents', detail.request_manifest], ['Accepted attachments', detail.response_manifest]].map(([label, members]) => <details key={label as string} open={!Object.keys(catalog.documents).length}>
      <summary>{label as string}</summary>
      {(members as HistoryDetail['request_manifest'])?.length ? <ul>{(members as HistoryDetail['request_manifest']).map(member => <li key={member.artifact_id}>
        <a href={conversationArtifactURL(tenant, job, detail.entry_id, member.artifact_id, detail.snapshot_token)}>Download {member.artifact_id}</a> — {member.availability}{member.reason ? `: ${member.reason}` : ''}
      </li>)}</ul> : <p>None recorded</p>}
    </details>)}
  </section>;
}
export function CurrentActivity({ metadata, freshness }: { metadata?: RecipeJob; freshness?: 'loading' | 'loaded' | 'failed' }) {
  if (metadata && isFinishedJob(metadata)) return null;
  const status = metadata ? jobStatusValue(metadata) : undefined;
  const label = status === 'ACTIVE' ? 'Running' : status === 'READY' || status === 'PENDING_JOBS' || status === 'AWAITING_FUTURE' ? 'Pending'
    : status && ['CANCEL_REQUESTED', 'CRASH_CONCERN', 'EXPIRED'].includes(status) ? jobStatusPresentation(status).label : 'Status unavailable';
  const reason = status === 'PENDING_JOBS' ? 'Waiting for jobs' : status === 'AWAITING_FUTURE' ? 'Scheduled' : undefined;
  return <Card size="small" className="conversation-step" aria-label="Current job activity">
    <strong>{label}</strong>{reason && <p>{reason}</p>}
    {metadata && freshness === 'failed' && <p role="status">Status stale — job details could not be refreshed.</p>}
    {metadata && freshness === 'loading' && <p>Refreshing status…</p>}
  </Card>;
}
type ConversationProps = {
  tenant: string; job: string; occurrence?: string; requestId?: string; onOpen: (entry: string) => void; onBack?: () => void; refreshKey?: unknown;
  metadata?: RecipeJob; metadataFreshness?: 'loading' | 'loaded' | 'failed'; autoRefresh?: boolean; refreshIntervalMs?: number;
};
export default function JobConversation(props: ConversationProps) {
  return <Conversation key={JSON.stringify([props.tenant,props.job])} {...props}/>;
}
function Conversation({ tenant, job, occurrence, requestId, onOpen, onBack, refreshKey, autoRefresh = false, refreshIntervalMs = 5000 }: ConversationProps) {
  const [page, setPage] = useState<HistoryPage>();
  const [entries, setEntries] = useState<HistoryEntry[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [activity, setActivity] = useState(false);
  const generation = useRef(0);
  const pageRef = useRef<HistoryPage>();
  const entriesRef = useRef<HistoryEntry[]>([]);
  const controller = useRef<AbortController>();
  const identity = `${tenant}\0${job}`;
  const loadedIdentity = useRef(identity);
  const previousRefreshKey = useRef(refreshKey);
  const load = useCallback(async (token?: string, background = false) => {
    const current = ++generation.current;
    controller.current?.abort();
    const abort = new AbortController(); controller.current = abort;
    setBusy(true);
    try {
      let next = await getConversation(tenant, job, { pageToken: token, signal: abort.signal });
      if (token && pageRef.current && next.snapshot_token !== pageRef.current.snapshot_token) throw new Error('History snapshot changed during pagination. Use the page Refresh button.');
      const first = next;
      const snapshot = next.snapshot_token;
      let refreshed = next.entries;
      // Refresh the whole visible range in the new immutable snapshot before
      // replacing it. Never combine a new first page with old continuation tokens.
      if (!token) {
        const visibleCount = entriesRef.current.length;
        while (next.next_page_token && refreshed.length < visibleCount) {
          next = await getConversation(tenant, job, { pageToken: next.next_page_token, signal: abort.signal });
          if (next.snapshot_token !== snapshot) throw new Error('History snapshot changed during refresh. Use the page Refresh button.');
          const seen = new Set(refreshed.map(e => e.entry_id));
          refreshed = [...refreshed, ...next.entries.filter(e => !seen.has(e.entry_id))];
        }
      }
      if (abort.signal.aborted || current !== generation.current) return;
      if (background && pageRef.current && stringifyJobJSON(pageRef.current.merges) !== stringifyJobJSON(next.merges)) setActivity(true);
      const merged = {...next, activity: token ? pageRef.current?.activity : first.activity, final_documents: token ? pageRef.current?.final_documents : first.final_documents};
      pageRef.current = merged; setPage(merged); setError('');
      setEntries(previous => {
        if (!token) {
          if (background && previous.length && stringifyJobJSON(previous) !== stringifyJobJSON(refreshed)) setActivity(true);
          entriesRef.current = refreshed;
          return refreshed;
        }
        const seen = new Set(previous.map(e => e.entry_id));
        const combined = [...previous, ...next.entries.filter(e => !seen.has(e.entry_id))];
        entriesRef.current = combined;
        return combined;
      });
    } catch (e) {
      if (abort.signal.aborted || current !== generation.current) return;
      setError(e instanceof APIError && e.code === 'history_snapshot_expired' ? 'History snapshot expired. Use the page Refresh button to restart pagination; the visible entries are retained until refresh succeeds.' : `${String(e)} Use the page Refresh button to try again.`);
    } finally { if (current === generation.current) setBusy(false); }
  }, [tenant, job]);
  useEffect(() => {
    loadedIdentity.current = identity; entriesRef.current = []; pageRef.current = undefined; setPage(undefined); setEntries([]); setError(''); setActivity(false);
    void load();
    return () => { ++generation.current; controller.current?.abort(); };
  }, [identity, load]);
  useEffect(() => {
    const changed = previousRefreshKey.current !== refreshKey; previousRefreshKey.current = refreshKey;
    if (loadedIdentity.current === identity && changed) void load(undefined, true);
  }, [refreshKey, identity, load]);
  useEffect(() => {
    if (!autoRefresh || (page?.activity?.value && isFinishedJob(page.activity.value))) return;
    const timer = window.setInterval(() => { void load(undefined, true); }, refreshIntervalMs);
    return () => window.clearInterval(timer);
  }, [autoRefresh, refreshIntervalMs, page?.activity, load]);
  return <section aria-label="Input history">
    {error && <Alert type="warning" message="History unavailable" description={error} />}
    {page && <p>History coverage: {page.completeness}. {page.next_page_token ? 'More retained events are available.' : 'End of this retained snapshot; coverage may be partial.'}</p>}
    {page?.publication_coverage && page.publication_coverage !== 'recorded' && <Alert type="info" message={`Publication coverage: ${page.publication_coverage}. Only verified retained publications are shown.`} />}
    {page?.diagnostics?.map((d, i) => <Alert key={i} type="info" message={d.message} />)}
    {(occurrence !== undefined || requestId !== undefined) && <HistoricalRequest key={`${identity}\0${occurrence ?? requestId}`} tenant={tenant} job={job} entry={occurrence ?? requestId!} legacy={occurrence === undefined} onBack={onBack} />}
    {/* Keep arriving activity below the historical reader so it does not move the reading position. */}
    {activity && <span role="status">New conversation activity available</span>}
    {page && !entries.length && !page.merges?.length && <p>No retained events in this history.</p>}
    <ConversationTimeline page={loadedIdentity.current === identity ? page : undefined} entries={loadedIdentity.current === identity ? entries : []} onOpen={onOpen} />
    {page?.next_page_token && <Button disabled={busy || !!error} onClick={() => void load(page.next_page_token)}>Load more events</Button>}
    {!page && !error && <p role="status">Loading history…</p>}
    <CurrentActivity metadata={page?.activity?.availability === 'available' ? page.activity.value : undefined} freshness={error ? 'failed' : 'loaded'} />
    <FinalDocuments tenant={tenant} job={job} terminal={!page?.activity?.value || isFinishedJob(page.activity.value)} manifest={page?.final_documents?.availability === 'available' ? page.final_documents.value : undefined} error={page && page.final_documents?.availability !== 'available' ? 'Returned documents could not be read.' : undefined} stale={!!error} onRetry={()=>void load()}/>

  </section>;
}
