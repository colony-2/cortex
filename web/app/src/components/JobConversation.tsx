import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Space, Typography } from 'antd';
import { APIError, conversationArtifactURL, getConversation, getConversationEntry, getConversationRequest, stringifyJobJSON, type HistoryDetail, type HistoryEntry, type HistoryPage, type HistoryValue } from '@colony2/shared';

const valueText = (value: unknown) => typeof value === 'string' ? value : stringifyJobJSON(value, 2) ?? 'Unavailable';
function RecordedValue({ value }: { value: HistoryValue }) {
  return value.present ? <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{valueText(value.raw)}</pre> : <span>Not recorded</span>;
}
export function HistoryPair({ entry, onOpen }: { entry: HistoryEntry; onOpen?: () => void }) {
  return <Card size="small" title={entry.title.present ? valueText(entry.title.raw) : `${entry.kind} request`}>
    <p>Occurrence: {entry.entry_id}</p>
    <p>State: {entry.state || 'unknown'} · Attempt: {entry.locator.job_attempt ?? 'not recorded'}</p>
    <details><summary>Occurrence provenance</summary><pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{valueText(entry.locator)}</pre>
      {entry.state_evidence?.map((evidence, index) => <p key={index}>{evidence}</p>)}
    </details>
    <p>Requested: {entry.requested_at.present ? valueText(entry.requested_at.raw) : 'not recorded'}</p>
    <RecordedValue value={{ present: Object.keys(entry.form_fields ?? {}).length > 0, raw: entry.form_fields }} />
    <Typography.Title level={5}>Accepted response</Typography.Title>
    <RecordedValue value={entry.accepted_response} />
    <p>Response source: {entry.response_source || 'not recorded'}</p>
    <p>Actor provenance: {entry.actor.provenance || 'unavailable'}</p>
    {entry.actor.receipt !== undefined && <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{valueText(entry.actor.receipt)}</pre>}
    {entry.actor.provenance === 'reported_receipt' && <p>Caller-reported identity; not an authenticated human submission.</p>}
    {entry.actor.provenance === 'automation_receipt' && <p>Automated response.</p>}
    {entry.diagnostics?.map((d, i) => <p key={i}>{d.message}</p>)}
    {onOpen && <Button onClick={onOpen}>Open recorded request</Button>}
  </Card>;
}
function HistoricalRequest({ tenant, job, entry, legacy }: { tenant: string; job: string; entry: string; legacy?: boolean }) {
  const [detail, setDetail] = useState<HistoryDetail>();
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setDetail(undefined); setError('');
    (legacy ? getConversationRequest(tenant, job, entry, controller.signal) : getConversationEntry(tenant, job, entry, { signal: controller.signal })).then(d => { if (!controller.signal.aborted) setDetail(d); }).catch(e => { if (!controller.signal.aborted) setError(String(e)); });
    return () => controller.abort();
  }, [tenant, job, entry, legacy, retry]);
  if (error) return <Alert type="warning" message="Request unavailable" description={<>{error} <Button onClick={() => setRetry(v => v + 1)}>Retry request</Button></>} />;
  if (!detail) return <p role="status">Loading recorded request…</p>;
  return <section aria-label="Recorded request">
    <HistoryPair entry={detail} />
    <p>Historical inspection is read-only. Live response actions use the current pending request.</p>
    <p>History coverage: {detail.completeness}</p>
    {[['Request documents', detail.request_manifest], ['Accepted attachments', detail.response_manifest]].map(([label, members]) => <div key={label as string}>
      <Typography.Title level={5}>{label as string}</Typography.Title>
      {(members as HistoryDetail['request_manifest'])?.length ? <ul>{(members as HistoryDetail['request_manifest']).map(member => <li key={member.artifact_id}>
        {member.availability === 'available' || member.availability === 'retained' ? <a href={conversationArtifactURL(tenant, job, detail.entry_id, member.artifact_id, detail.snapshot_token)}>Download {member.artifact_id}</a> : <><a href={conversationArtifactURL(tenant, job, detail.entry_id, member.artifact_id, detail.snapshot_token)}>Try original {member.artifact_id}</a> — {member.availability}: {member.reason || 'availability is not guaranteed'}</>}
      </li>)}</ul> : <p>None recorded</p>}
    </div>)}
  </section>;
}
export default function JobConversation({ tenant, job, occurrence, requestId, onOpen, refreshKey, running }: {
  tenant: string; job: string; occurrence?: string; requestId?: string; onOpen: (entry: string) => void; refreshKey?: unknown; running: boolean;
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
      let refreshed = next.entries;
      // Refresh the whole visible range in the new immutable snapshot before
      // replacing it. Never combine a new first page with old continuation tokens.
      if (!token) {
        const visibleCount = entriesRef.current.length;
        while (next.next_page_token && refreshed.length < visibleCount) {
          next = await getConversation(tenant, job, { pageToken: next.next_page_token, signal: abort.signal });
          refreshed = [...refreshed, ...next.entries];
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
    {(occurrence !== undefined || requestId !== undefined) && <HistoricalRequest key={`${identity}\0${occurrence ?? requestId}`} tenant={tenant} job={job} entry={occurrence ?? requestId!} legacy={occurrence === undefined} />}
    {(loadedIdentity.current === identity ? entries : []).map(entry => <HistoryPair key={entry.entry_id} entry={entry} onOpen={() => onOpen(entry.entry_id)} />)}
    {page?.next_page_token && <Button disabled={busy || !!error} onClick={() => void load(page.next_page_token)}>Load more requests</Button>}
    {!page && !error && <p role="status">Loading history…</p>}
  </section>;
}
