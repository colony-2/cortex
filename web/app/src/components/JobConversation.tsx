import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Space, Typography } from 'antd';
import { APIError, conversationArtifactURL, getConversation, getConversationEntry, getConversationRequest, stringifyJobJSON, type HistoryDetail, type HistoryEntry, type HistoryPage, type HistoryValue } from '@colony2/shared';

import ReviewDocuments from './ReviewDocuments';
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
export function HistoryPair({ entry, onOpen }: { entry: HistoryEntry; onOpen?: () => void }) {
  const [expanded, setExpanded] = useState(false);
  const answer = savedAnswers(entry.form_fields, entry.accepted_response.raw, true);
  const receipt = object(entry.actor.receipt);
  return <div className="conversation-pair" data-occurrence={entry.entry_id}>
    <Card size="small" className="conversation-step" aria-label="Recorded request step">
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
    </Card>
    {entry.accepted_response.present && <Card size="small" className="conversation-step conversation-response" aria-label="Accepted response">
      <div className={expanded ? 'conversation-answer expanded' : 'conversation-answer'}>{answer}</div>
      <div className="conversation-line"><Time value={{present:Object.prototype.hasOwnProperty.call(receipt,'submitted_at'),raw:receipt.submitted_at}} reported={entry.actor.provenance === 'reported_receipt'} /><span title={reviewer(entry)}>{reviewer(entry)}</span></div>
      <button className="conversation-expand" onClick={() => setExpanded(v => !v)}>{expanded ? 'Collapse feedback' : 'Expand feedback'}</button>
    </Card>}
  </div>;
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
export default function JobConversation({ tenant, job, occurrence, requestId, onOpen, onBack, refreshKey, running }: {
  tenant: string; job: string; occurrence?: string; requestId?: string; onOpen: (entry: string) => void; onBack?: () => void; refreshKey?: unknown; running: boolean;
}) {
  const [page, setPage] = useState<HistoryPage>();
  const [entries, setEntries] = useState<HistoryEntry[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [activity, setActivity] = useState(false);
  const generation = useRef(0);
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
      const snapshot = next.snapshot_token;
      let refreshed = next.entries;
      // Refresh the whole visible range in the new immutable snapshot before
      // replacing it. Never combine a new first page with old continuation tokens.
      if (!token) {
        const visibleCount = entriesRef.current.length;
        while (next.next_page_token && refreshed.length < visibleCount) {
          next = await getConversation(tenant, job, { pageToken: next.next_page_token, signal: abort.signal });
          if (next.snapshot_token !== snapshot) throw new Error('History snapshot changed during refresh. Retry history.');
          const seen = new Set(refreshed.map(e => e.entry_id));
          refreshed = [...refreshed, ...next.entries.filter(e => !seen.has(e.entry_id))];
        }
      }
      if (abort.signal.aborted || current !== generation.current) return;
      setPage(next); setError('');
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
      setError(e instanceof APIError && e.code === 'history_snapshot_expired' ? 'History snapshot expired. Refresh history to restart pagination; the visible entries are retained until refresh succeeds.' : String(e));
    } finally { if (current === generation.current) setBusy(false); }
  }, [tenant, job]);
  useEffect(() => {
    loadedIdentity.current = identity; entriesRef.current = []; setPage(undefined); setEntries([]); setError(''); setActivity(false);
    void load();
    return () => { ++generation.current; controller.current?.abort(); };
  }, [identity, load]);
  useEffect(() => {
    const changed = previousRefreshKey.current !== refreshKey; previousRefreshKey.current = refreshKey;
    if (loadedIdentity.current === identity && changed) void load(undefined, true);
  }, [refreshKey, identity, load]);
  useEffect(() => {
    if (!running) return;
    const timer = window.setInterval(() => { void load(undefined, true); }, 5000);
    return () => window.clearInterval(timer);
  }, [running, load]);
  return <section aria-label="Input history">
    <Space wrap><Button onClick={() => { setActivity(false); void load(); }} loading={busy}>Refresh history</Button>{activity && <span role="status">New conversation activity available</span>}</Space>
    {error && <Alert type="warning" message="History unavailable" description={error} />}
    {page && <p>History coverage: {page.completeness}. {page.next_page_token ? 'More retained requests are available.' : 'End of this retained snapshot; coverage may be partial.'}</p>}
    {page?.diagnostics?.map((d, i) => <Alert key={i} type="info" message={d.message} />)}
    {(occurrence !== undefined || requestId !== undefined) && <HistoricalRequest key={`${identity}\0${occurrence ?? requestId}`} tenant={tenant} job={job} entry={occurrence ?? requestId!} legacy={occurrence === undefined} onBack={onBack} />}
    {page && !entries.length && <p>No retained requests in this history.</p>}
    {(loadedIdentity.current === identity ? entries : []).map(entry => <HistoryPair key={entry.entry_id} entry={entry} onOpen={() => onOpen(entry.entry_id)} />)}
    {page?.next_page_token && <Button disabled={busy || !!error} onClick={() => void load(page.next_page_token)}>Load more requests</Button>}
    {!page && !error && <p role="status">Loading history…</p>}
  </section>;
}
