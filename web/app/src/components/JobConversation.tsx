import type { ThreadSnapshot } from '../hooks/useThreadInput';
import { messagePreview } from '../utils/jobInbox';
import { createContext, useContext, useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import { Alert, Button, Card, Typography } from 'antd';
import { APIError, storedDocumentURL, conversationArtifactURL, getConversation, getConversationEntry, getConversationRequest, stringifyJobJSON, type HistoryDetail, type HistoryEntry, type HistoryMember, type HistoryPage, type HistoryValue, type HistoryMerge } from '@colony2/shared';

import type { RecipeJob } from '@colony2/shared/types';
import { isFinishedJob, jobStatusPresentation, jobStatusValue } from '../utils/jobStatus';
import ReviewDocuments from './ReviewDocuments';
import FinalDocuments from './FinalDocuments';
import { historicalDocuments } from './historicalDocuments';
import { object, valueText, recordedTime, savedAnswers, reviewer } from './conversationPresentation';
import './conversation.css';
import ReviewMessageBody from './ReviewMessageBody';
import { parseReviewV2 } from '@colony2/shared';
import ThreadMessageLabel, { reviewContributionLabel, type MessageLabel } from './ThreadMessageLabel';
import ThreadMessageTime from './ThreadMessageTime';
import ThreadDependencies from './ThreadDependencies';
function Time({value, reported = false}: {value: HistoryValue; reported?: boolean}) {
  const time = recordedTime(value);
  return <span title={time.title}>{reported ? 'Reported time: ' : ''}{time.text}</span>;
}
function acceptedTime(entry: HistoryEntry) {
  const receiptTime = object(entry.actor.receipt).submitted_at;
  return { value: receiptTime ?? object(entry.accepted_response.raw).submitted_at, reported: receiptTime == null || entry.actor.provenance === 'reported_receipt' };
}
export function HistoryPair({ entry, onOpen, part, inline = false, attachments }: {
  entry: HistoryEntry; onOpen?: () => void; part?: 'request' | 'response'; inline?: boolean; attachments?:ReactNode;
}) {
  const [expanded, setExpanded] = useState(false);
  const answer = savedAnswers(entry.form_fields, entry.accepted_response.raw, true);
  const receipt = object(entry.actor.receipt);
  const question = object(entry.form_fields.presentation).type === 'review/v2' ? valueText(object(entry.form_fields.request).question) : entry.form_fields.question ? valueText(entry.form_fields.question)
    : Array.isArray(entry.form_fields.fields) ? entry.form_fields.fields.map(field => valueText(object(field).question || object(field).id)).join(' · ') : '';
  return <div className="conversation-pair" data-occurrence={entry.entry_id}>
    {part !== 'response' && <Card size="small" variant={inline ? 'borderless' : undefined} className="conversation-step" aria-label="Recorded request step">
      <div className="conversation-line"><strong>{entry.title?.present ? valueText(entry.title.raw) : `${entry.kind} request`}</strong>{onOpen && <Button size="small" onClick={onOpen}>{entry.kind === 'review' || object(entry.form_fields.presentation).type === 'review/v2' ? 'Open review' : 'Open recorded request'}</Button>}</div>
      <div className="conversation-line">{!inline && <Time value={entry.requested_at} />}<span>State: {entry.state || 'unknown'}</span>{!entry.accepted_response.present && <span>{reviewer(entry)}</span>}</div>
      {question && <div className="conversation-question" title={question}>{question}</div>}

    </Card>}
    {part !== 'request' && entry.accepted_response.present && <Card size="small" variant={inline ? 'borderless' : undefined} className="conversation-step conversation-response" aria-label="Accepted response">
      {inline && !part && <div className="thread-message-meta"><ThreadMessageLabel label="Answer" /><ThreadMessageTime {...acceptedTime(entry)} /></div>}
      <div className={expanded || inline ? 'conversation-answer expanded' : 'conversation-answer'}>{answer}</div>
      {attachments}
      <div className="conversation-line">{!inline && <Time value={{present:Object.prototype.hasOwnProperty.call(receipt,'submitted_at'),raw:receipt.submitted_at}} reported={entry.actor.provenance === 'reported_receipt'} />}<span title={reviewer(entry)}>{reviewer(entry)}</span></div>
      {!inline && <button className="conversation-expand" onClick={() => setExpanded(v => !v)}>{expanded ? 'Collapse feedback' : 'Expand feedback'}</button>}
    </Card>}
  </div>;
}
export function MergeCard({ merge, inline = false }: { merge: HistoryMerge; inline?: boolean }) {
  const subject = merge.candidate_subject;
  const snippet = subject == null || subject === '' ? 'Commit message not recorded' : subject.length > 160 ? `${subject.slice(0,160)}…` : subject;
  return <Card size="small" variant={inline ? 'borderless' : undefined} className="conversation-step" aria-label="Verified publication" data-event={merge.entry_id}>
    <strong>{merge.publication_disposition === 'already_present' ? 'Publication confirmed — already present' : 'Published'}</strong>
    <p style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{snippet}</p>
    <details><summary>Publication details</summary>
      <p>Commit: {merge.candidate_sha}</p><p>Branch: {merge.publication_branch}</p>
      <p>Commit message: {subject == null || subject === '' ? 'Commit message not recorded' : subject}</p>
      <p>Job: {merge.job_id} · Attempt: {merge.job_attempt} · Task attempt: {merge.task_attempt}</p>
      <p style={{overflowWrap:'anywhere'}}>Source: {merge.invocation_id}</p><p>Result ordinal: {String(merge.result_ordinal)}</p>
      {!inline && <Time value={{present:!!merge.completed_at,raw:merge.completed_at}} />}
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
const RepeatedReview = createContext(false);
type ThreadItem = { key: string; occurrence?: string; reviewId?: string; sender: string; subject: string; subjectIsPrompt?: boolean; label?: MessageLabel; time?: { value?: unknown; reported?: boolean }; node: ReactNode; ariaLabel?: string; forceOpen?: boolean };
type InitialMessage = Omit<ThreadItem, 'key' | 'occurrence'> & { showAttachments?: boolean; onExpand?: () => void };

export function FoldedConversationTimeline({
  page, entries, onOpen, currentRequestId, cellName, tenant, job, initial, trailing = [], selectedOccurrence, ready = true,
}: {
  page?: HistoryPage; entries: HistoryEntry[]; onOpen:(id:string)=>void; currentRequestId?:string; cellName?:string; tenant?: string; job?: string;
  initial?: InitialMessage; trailing?: ThreadItem[]; selectedOccurrence?: string; ready?: boolean;
}) {
  const liveOccurrence = trailing.find(item => item.occurrence)?.occurrence;
  const recorded = entries.filter(entry => entry.accepted_response.present || (liveOccurrence
    ? entry.entry_id !== liveOccurrence : !currentRequestId || entry.request_id !== currentRequestId));
  const byID = new Map(recorded.map(entry => [entry.entry_id, entry]));
  const items: ThreadItem[] = [];
  const addEntry = (entry: HistoryEntry, part?: 'request' | 'response') => {
    const fields = entry.form_fields;
    const structured = object(fields.presentation).type === 'review/v2';
    const isReview = entry.kind === 'review' || structured;
    const review = structured ? object(fields.request) : undefined;
    const question = structured ? review?.question : fields.question;
    const title = (entry.title?.present && typeof entry.title.raw === 'string' && entry.title.raw.trim()) ||
      (typeof review?.title === 'string' ? review.title.trim() : '');
    const prompt = (question ? valueText(question) : '') ||
      (Array.isArray(fields.fields) ? valueText(object(fields.fields[0]).question ?? '') : '');
    const response = part === 'response';
    const actor = object(object(entry.actor.receipt).actor).id
      || (entry.actor.legacy_user_id?.present ? entry.actor.legacy_user_id.raw : undefined);
    const sender = response ? (actor ? valueText(actor) : 'Unknown') : cellName || 'Cell';
    const preview = response ? savedAnswers(fields, entry.accepted_response.raw, true) : title || prompt || `${entry.kind} request`;
    let label: MessageLabel = isReview ? 'Review' : 'Question';
    if (response) {
      const accepted = object(entry.accepted_response.raw);
      const directions = review?.directions;
      const direction = Array.isArray(directions) ? directions.find(d => object(d).id === object(accepted.response ?? accepted).direction) : undefined;
      label = isReview ? reviewContributionLabel(direction) : 'Answer';
    }
    items.push({key: JSON.stringify([entry.entry_id, part]), occurrence: entry.entry_id, sender, subject: messagePreview(preview), subjectIsPrompt: !response && !title && !!prompt,
      reviewId: !response && typeof review?.review_id === 'string' ? review.review_id : undefined,
      time: response ? acceptedTime(entry) : { value: entry.requested_at?.present ? entry.requested_at.raw : undefined },
      label, node: !response && isReview && tenant && job
        ? entry.review_attachments
          ? <RecordedRequest inline tenant={tenant} job={job} detail={threadReviewDetail(entry,page?.snapshot_token)} />
          : <HistoricalRequest inline tenant={tenant} job={job} entry={entry.entry_id} snapshotToken={page?.snapshot_token} />
        : <HistoryPair inline entry={entry} part={part} onOpen={()=>onOpen(entry.entry_id)} attachments={response && isReview && tenant && job && <HistoryAttachments tenant={tenant} job={job}
          entry={entry.entry_id} snapshotToken={page?.snapshot_token} members={entry.review_attachments?.response ?? []} />}/>});
  };
  const positioned = new Set<string>();
  for (const event of page?.events ?? []) {
    const key = JSON.stringify([event.kind, event.entry_id]);
    if (positioned.has(key)) continue;
    positioned.add(key);
    if (event.kind === 'merge_completion') {
      const merge = page?.merges?.find(item => item.entry_id === event.entry_id);
      if (merge) items.push({key, sender: 'Publication', subject: merge.candidate_subject || 'Recorded commit', label: 'Merge', time: { value: merge.completed_at }, node: <MergeCard inline merge={merge}/>});
      continue;
    }
    const entry = byID.get(event.entry_id);
    if (entry && event.kind === 'request_generation') addEntry(entry, 'request');
    if (entry?.accepted_response.present && event.kind === 'response_acceptance') addEntry(entry, 'response');
  }
  const missing = recorded.flatMap(entry => {
    const parts: ('request' | 'response')[] = [];
    if (!positioned.has(JSON.stringify(['request_generation', entry.entry_id]))) parts.push('request');
    if (entry.accepted_response.present && !positioned.has(JSON.stringify(['response_acceptance', entry.entry_id]))) parts.push('response');
    return parts.map(part => ({ entry, part }));
  });
  if (missing.length && page?.events?.length) {
    const ordered = items.splice(0);
    missing.forEach(({entry, part}) => addEntry(entry, part));
    items.push(...ordered);
  } else if (!page?.events?.length) {
    for (const entry of recorded) {
      const isReview = entry.kind === 'review' || object(entry.form_fields.presentation).type === 'review/v2';
      addEntry(entry, isReview ? 'request' : undefined);
      if (isReview && entry.accepted_response.present) addEntry(entry, 'response');
    }
  }
  // A selected review can also be present in history while its reply is accepted,
  // or when its permalink opens the latest saved request without a live input.
  const tail = [...trailing];
  const selectedReview = tail.find(item => item.reviewId);
  if (selectedReview && (selectedReview.occurrence || (!currentRequestId && !page?.next_page_token))) {
    for (let index = items.length - 1; index >= 0; index--) {
      const item = items[index];
      if (item.reviewId === selectedReview.reviewId && (!selectedReview.occurrence || item.occurrence === selectedReview.occurrence)) {
        items[index] = {...item, node: selectedReview.node, forceOpen: selectedReview.forceOpen};
        tail.splice(tail.indexOf(selectedReview), 1);
        break;
      }
    }
  }
  const lastKey = tail[tail.length - 1]?.key ?? items[items.length - 1]?.key;
  const latestReview = new Map<string, string>();
  for (const item of [...items, ...tail]) {
    if (item.reviewId) latestReview.set(item.reviewId, item.key);
  }
  function row(item: ThreadItem) {
    const priorReview = !!item.reviewId && latestReview.get(item.reviewId) !== item.key;
    // A permalink already displays the selected request above the timeline.
    const defaultOpen = item.forceOpen || !selectedOccurrence || item.occurrence !== selectedOccurrence;
    return <ThreadMessage key={item.key} occurrence={item.occurrence} ariaLabel={item.ariaLabel} sender={item.sender} subject={item.subject} subjectIsPrompt={item.subjectIsPrompt} label={item.label} time={item.time} defaultOpen={defaultOpen}>
      <RepeatedReview.Provider value={priorReview}>{item.node}</RepeatedReview.Provider>
    </ThreadMessage>;
  }
  return <div className="inbox-history">
    {initial && <ThreadMessage className="inbox-original" onExpand={initial.onExpand} sender={initial.sender} subject={initial.subject} subjectIsPrompt={initial.subjectIsPrompt} time={initial.time} defaultOpen={initial.showAttachments || (ready && !lastKey)}>
      {initial.node}
    </ThreadMessage>}
    {items.map(row)}
    {tail.map(row)}
  </div>;
}
function ThreadMessage({ sender, subject, subjectIsPrompt, label, time, children, occurrence, ariaLabel, onExpand, defaultOpen = false, className = 'inbox-history-message' }: {
  sender: string; subject: string; subjectIsPrompt?: boolean; label?: MessageLabel; time?: ThreadItem['time']; children: ReactNode; occurrence?: string; ariaLabel?: string; onExpand?: () => void; defaultOpen?: boolean; className?: string;
}) {
  const [expanded, setExpanded] = useState<boolean>();
  const [visited, setVisited] = useState(false);
  const open = expanded ?? defaultOpen;
  useEffect(() => {
    if (open) {
      setVisited(true);
      onExpand?.();
    }
  }, [open, onExpand]);
  return <details className={`thread-message ${className}${subjectIsPrompt ? ' has-prompt-preview' : ''}`} data-occurrence={occurrence} role={ariaLabel ? 'region' : undefined} aria-label={ariaLabel} open={open} onToggle={event => {
    if (event.currentTarget.open !== open) setExpanded(event.currentTarget.open);
  }}>
    <summary onClick={event => {
      event.preventDefault();
      setExpanded(!open);
    }}>
        <span className="thread-message-meta">
          <span className="thread-message-sender" title={sender}>{sender}</span>
          {label && <ThreadMessageLabel label={label} />}
          <ThreadMessageTime {...time} />
        </span>
        {subject && !(open && subjectIsPrompt) && <span className="thread-message-subject" title={subject}>{subject}</span>}
    </summary>
    {(open || visited) && <div className="thread-message-body">{children}</div>}
  </details>;
}
function HistoricalRequest({ tenant, job, entry, snapshotToken, legacy, onBack, inline }: { tenant: string; job: string; entry: string; snapshotToken?: string; legacy?: boolean; onBack?: () => void; inline?: boolean }) {
  const [detail, setDetail] = useState<HistoryDetail>();
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  const recoveryUsed = useRef(false);
  const recovering = useRef(false);
  const alive = useRef(true);
  const recoveryController = useRef<AbortController>();
  const initialSnapshot = useRef(snapshotToken);
  useEffect(() => { alive.current = true; return () => {alive.current = false; recoveryController.current?.abort();}; }, []);
  useEffect(() => {
    const controller = new AbortController();
    recoveryController.current?.abort();
    setDetail(undefined); setError('');
    const load = async () => {
      if (legacy) return getConversationRequest(tenant, job, entry, controller.signal);
      try {
        return await getConversationEntry(tenant, job, entry, { snapshotToken: initialSnapshot.current, signal: controller.signal });
      } catch (error) {
        if (controller.signal.aborted || !initialSnapshot.current || !(error instanceof APIError) || error.code !== 'history_snapshot_expired') throw error;
        return getConversationEntry(tenant, job, entry, { signal: controller.signal });
      }
    };
    load().then(d => { if (!controller.signal.aborted) setDetail(d); }).catch(e => { if (!controller.signal.aborted) setError(String(e)); });
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
      while (!found && page.next_page_token) { page = await getConversation(tenant, job, {pageToken:page.next_page_token,includeSections:false,signal:abort.signal}); found = page.entries.some(e => e.entry_id === selected); }
      if (!found) throw new Error('The selected occurrence is no longer available in retained history.');
      const fresh = await getConversationEntry(tenant, job, selected, {snapshotToken:snapshot,signal:abort.signal});
      if (alive.current && !abort.signal.aborted) setDetail(fresh);
    } catch (e) { if (alive.current && !abort.signal.aborted) setError(`Snapshot recovery failed: ${String(e)}`); }
    finally {recovering.current = false;}
  };
  if (!detail && error) return <Alert type="warning" message="Request unavailable" description={<>{error} <Button onClick={() => {recoveryUsed.current = false; setRetry(v => v + 1);}}>Retry request</Button></>} />;
  if (!detail) return <p role="status">Loading recorded request…</p>;
  return <RecordedRequest inline={inline} tenant={tenant} job={job} detail={detail} onBack={onBack} onError={failure => void recover(failure)} error={error}
    onRetry={() => { recoveryUsed.current = false; setError(''); setRetry(v => v + 1); }} />;
}

type RecordedRequestDetail = HistoryEntry & Pick<HistoryDetail, 'raw_form' | 'request_manifest' | 'response_manifest'> & { snapshot_token?: string };

function threadReviewDetail(entry: HistoryEntry, snapshot_token?: string): RecordedRequestDetail {
  return { ...entry, snapshot_token, raw_form: entry.form_fields, request_manifest: entry.review_attachments?.request ?? [], response_manifest: entry.review_attachments?.response ?? [] };
}

function retainedMemberURL(tenant:string, job:string, entry:string, member:HistoryMember, snapshotToken?:string) {
  const ref=object(member.ref), key=object(object(ref.stored).key);
  if (member.availability==='retained' && ref.kind==='stored' && typeof key.jobId==='string' && typeof key.name==='string' &&
      (typeof key.taskOrdinal==='number' || typeof key.taskOrdinal==='bigint')) return storedDocumentURL(tenant,{jobId:key.jobId,taskOrdinal:key.taskOrdinal,name:key.name});
  return snapshotToken && !member.reason ? conversationArtifactURL(tenant,job,entry,member.artifact_id,snapshotToken) : '';
}

function HistoryAttachments({ tenant,job,entry,snapshotToken,members }: {tenant:string;job:string;entry:string;snapshotToken?:string;members:HistoryMember[]}) {
  if (!members.length) return null;
  const urls:Record<string,string>=Object.create(null);
  const documents=Object.fromEntries(members.map(member=>{
    const ref=object(member.ref), key=object(object(ref.stored).key);
    const name=ref.name || key.name || member.artifact_id;
    const url=retainedMemberURL(tenant,job,entry,member,snapshotToken);
    urls[member.artifact_id]=url;
    return [member.artifact_id,{name,sizeBytes:key.sizeBytes,hideIdentifier:true,unavailableReason:url ? undefined : (member.reason || 'Attachment unavailable')}];
  }));
  return <ReviewDocuments attachmentLayout="attachments" attachmentsLabel="Recorded attachments" projectId={tenant} jobId={job} requestId={entry} documents={documents}
    source={{identity:JSON.stringify([entry,snapshotToken]),url:id=>urls[id] ?? ''}} />;
}

function RecordedRequest({ tenant, job, detail, onBack, onError, error, onRetry, inline }: {
  tenant: string; job: string; detail: RecordedRequestDetail; onBack?: () => void; onError?: (failure: unknown) => void; error?: string; onRetry?: () => void; inline?: boolean;
}) {
  const repeated = useContext(RepeatedReview);
  const [showRepeated, setShowRepeated] = useState(false);
  const catalog = historicalDocuments(detail);
  const rawForm = object(detail.raw_form);
  const form = object(rawForm.presentation).type === 'review/v2' ? object(rawForm.request) : rawForm;
  const inlineReview=inline && (detail.kind==='review' || object(rawForm.presentation).type==='review/v2');
  let review;
  try {if(object(rawForm.presentation).type === 'review/v2')review=parseReviewV2(rawForm.request);}catch{/* raw output remains available */}
  const artifactURL = (id: string) => detail.snapshot_token ? conversationArtifactURL(tenant, job, detail.entry_id, id, detail.snapshot_token) : '';
  const memberURL = (member: HistoryMember) => {
    const ref = object(member.ref);
    const key = object(object(ref.stored).key);
    if (member.availability === 'retained' && ref.kind === 'stored' && typeof key.jobId === 'string' && typeof key.name === 'string' &&
        (typeof key.taskOrdinal === 'number' || typeof key.taskOrdinal === 'bigint')) {
      return storedDocumentURL(tenant, { jobId: key.jobId, taskOrdinal: key.taskOrdinal, name: key.name });
    }
    return artifactURL(member.artifact_id);
  };
  const hasDocuments = Object.keys(catalog.documents).length > 0;
  const repeatedContent = repeated && hasDocuments && <p><button type="button" className="review-repeated-content" aria-expanded={showRepeated}
    onClick={() => setShowRepeated(open => !open)}>{showRepeated ? 'Hide repeated content' : '… (repeated content)'}</button></p>;
  return <section aria-label="Recorded request" data-occurrence={detail.entry_id}>
    {onBack && <Button onClick={onBack}>Back to thread</Button>}
    {!inlineReview && <Typography.Title level={4}>{detail.title.present ? valueText(detail.title.raw) : 'Recorded request'}</Typography.Title>}
    {!inlineReview && <p>Read only</p>}
    {object(rawForm.presentation).type&&object(rawForm.presentation).type!=='review/v2'&&<Alert type="warning" message="Unsupported recorded presentation. Raw data remains available."/>}
    {!review && (form.question || Array.isArray(form.fields) || repeatedContent) && <section aria-label="Review context" style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>
      {form.question ? valueText(form.question) : Array.isArray(form.fields) && form.fields.map((field: unknown) => valueText(object(field).question || object(field).id)).join('\n')}
      {repeatedContent}
    </section>}
    {!inlineReview && detail.summary.present && <p>{valueText(detail.summary.raw)}</p>}
    {error && <Alert type="warning" message={error} action={<Button onClick={onRetry}>Retry request</Button>} />}
    {catalog.limited && <Alert type="info" message="Limited history: the original form or document metadata was not recorded. Available documents and answers are shown below." />}
    {review && <ReviewMessageBody request={review} tenant={tenant} hasDocument={hasDocuments && (!repeated || showRepeated)}>
      {repeatedContent}
    </ReviewMessageBody>}
    {(!repeated || showRepeated) && ((!review && !inlineReview) || hasDocuments) && <ReviewDocuments attachmentLayout={review || inlineReview ? 'primary' : undefined} projectId={tenant} jobId={job} requestId={detail.request_id || detail.entry_id} documents={catalog.documents} documentMetadata={catalog.metadata} overviewDocumentId={catalog.overview} source={{identity:JSON.stringify([tenant,job,detail.entry_id,detail.snapshot_token]),url:id => catalog.documents[id]?.stored ? storedDocumentURL(tenant,catalog.documents[id].stored.key) : catalog.members[id] ? artifactURL(catalog.members[id]) : '',onError}} />}
    {inlineReview ? <HistoryAttachments tenant={tenant} job={job} entry={detail.entry_id} snapshotToken={detail.snapshot_token}
      members={detail.request_manifest.filter(member=>!Object.values(catalog.members).includes(member.artifact_id))} /> : <>
    {detail.accepted_response.present && <section aria-label="Recorded answers"><Typography.Title level={5}>Accepted response</Typography.Title><pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{savedAnswers(Object.keys(form).length ? form : detail.form_fields,detail.accepted_response.raw)}</pre></section>}
    <div className="conversation-line">{!inline && <Time value={detail.requested_at} />}<span>State: {detail.state || 'unknown'}</span><span>{reviewer(detail)}</span>{!inline && detail.accepted_response.present && <Time value={{present:!!object(detail.actor.receipt).submitted_at,raw:object(detail.actor.receipt).submitted_at}} reported={detail.actor.provenance === 'reported_receipt'} />}</div>
    {([['Request documents', detail.request_manifest], ['Accepted attachments', detail.response_manifest]] as const).map(([label, members]) => <details key={label} open={!Object.keys(catalog.documents).length}>
      <summary>{label}</summary>
      {members?.length ? <ul>{members.map(member => {
        const url = memberURL(member);
        return <li key={member.artifact_id}>
          {url ? <a href={url}>Download {member.artifact_id}</a> : member.artifact_id} — {member.availability}{member.reason ? `: ${member.reason}` : ''}
        </li>;
      })}</ul> : <p>None recorded</p>}
    </details>)}</>}
  </section>;
}
export function CurrentActivity({ metadata, freshness, inline = false, dependencies }: { metadata?: RecipeJob; freshness?: 'loading' | 'loaded' | 'failed'; inline?: boolean; dependencies?:ReactNode }) {
  if (metadata && isFinishedJob(metadata)) return null;
  const status = metadata ? jobStatusValue(metadata) : undefined;
  const label = status === 'ACTIVE' ? 'Running' : status === 'READY' || status === 'PENDING_JOBS' || status === 'AWAITING_FUTURE' ? 'Pending'
    : status && ['CANCEL_REQUESTED', 'CRASH_CONCERN', 'EXPIRED'].includes(status) ? jobStatusPresentation(status).label : 'Status unavailable';
  const reason = status === 'PENDING_JOBS' || status === 'AWAITING_FUTURE' ? jobStatusPresentation(status).label : undefined;
  return <Card size="small" variant={inline ? 'borderless' : undefined} className={inline ? 'thread-activity' : 'conversation-step'} aria-label="Current job activity">
    <strong>{label}</strong>{reason && <p>{reason}</p>}
    {dependencies}
    {metadata && freshness === 'failed' && <p role="status">Status stale — job details could not be refreshed.</p>}
    {metadata && freshness === 'loading' && <p>Refreshing status…</p>}
  </Card>;
}
type ConversationProps = {
  children?: ReactNode; initial?: InitialMessage; currentRequestLabel?: string; currentSubjectIsPrompt?: boolean; currentRequestTime?: string; currentReviewId?: string; currentIsReview?: boolean; openCurrent?: boolean; folded?: boolean; currentRequestId?: string; cellName?: string;
  onSnapshot?: (snapshot:ThreadSnapshot) => void;
  onInventory?: (rows: HistoryPage['input_artifacts']) => void;
  tenant: string; job: string; occurrence?: string; requestId?: string; onOpen: (entry: string) => void; onBack?: () => void; refreshKey?: unknown;
  metadata?: RecipeJob; metadataFreshness?: 'loading' | 'loaded' | 'failed'; autoRefresh?: boolean; refreshIntervalMs?: number;
};
export default function JobConversation(props: ConversationProps) {
  return <Conversation key={JSON.stringify([props.tenant,props.job])} {...props}/>;
}
function Conversation({ tenant, job, onInventory, onSnapshot, occurrence, requestId, onOpen, onBack, refreshKey, folded=false, currentRequestId, cellName, children, initial, metadata, currentRequestLabel, currentSubjectIsPrompt, currentRequestTime, currentReviewId, currentIsReview, openCurrent, autoRefresh = false, refreshIntervalMs = 5000 }: ConversationProps) {
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
  const load = useCallback(async (token?: string, background = false): Promise<void> => {
    const current = ++generation.current;
    controller.current?.abort();
    const abort = new AbortController(); controller.current = abort;
    setBusy(true);
    try {
      let next = await getConversation(tenant, job, { pageToken: token, includeSections: token ? false : undefined, includeReviewAttachments: folded, thread:!!onSnapshot && !token, signal: abort.signal });
      if (token && pageRef.current && next.snapshot_token !== pageRef.current.snapshot_token) throw new Error('History snapshot changed during pagination. Use the page Refresh button.');
      const first = next;
      const snapshot = next.snapshot_token;
      let refreshed = next.entries;
      // Refresh the whole visible range in the new immutable snapshot before
      // replacing it. Never combine a new first page with old continuation tokens.
      if (!token) {
        const visibleCount = entriesRef.current.length;
        while (next.next_page_token && refreshed.length < visibleCount) {
          next = await getConversation(tenant, job, { pageToken: next.next_page_token, includeSections: false, includeReviewAttachments: folded, signal: abort.signal });
          if (next.snapshot_token !== snapshot) throw new Error('History snapshot changed during refresh. Use the page Refresh button.');
          const seen = new Set(refreshed.map(e => e.entry_id));
          refreshed = [...refreshed, ...next.entries.filter(e => !seen.has(e.entry_id))];
        }
      }
      if (abort.signal.aborted || current !== generation.current) return;
      if (!token) onInventory?.(first.input_artifacts);
      if (background && pageRef.current && stringifyJobJSON(pageRef.current.merges) !== stringifyJobJSON(next.merges)) setActivity(true);
      const merged = {...next, activity: token ? pageRef.current?.activity : first.activity, final_documents: token ? pageRef.current?.final_documents : first.final_documents};
      pageRef.current = merged; setPage(merged); setError('');
      onSnapshot?.({page: merged, refresh: () => load(undefined, true)});
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
      if (!token) onInventory?.(undefined);
      const failure = e instanceof APIError && e.code === 'history_snapshot_expired' ? 'History snapshot expired. Use the page Refresh button to restart pagination; the visible entries are retained until refresh succeeds.' : `${String(e)} Use the page Refresh button to try again.`;
      setError(failure);
      onSnapshot?.({page: pageRef.current, error: failure, refresh: () => load(undefined, true)});
    } finally {
      if (current === generation.current) {
        controller.current = undefined;
        setBusy(false);
      }
    }
  }, [tenant, job, onInventory, onSnapshot, folded]);
  const refreshThread = useCallback(() => load(undefined, true), [load]);
  useEffect(() => {
    loadedIdentity.current = identity; entriesRef.current = []; pageRef.current = undefined; setPage(undefined); setEntries([]); setError(''); setActivity(false);
    // StrictMode can immediately discard this mount. Start the request only
    // after the mount survives its cleanup.
    let mounted = true;
    if (onSnapshot) queueMicrotask(() => { if (mounted) void load(); });
    else void load();
    return () => { mounted = false; ++generation.current; controller.current?.abort(); };
  }, [identity, load, onSnapshot]);
  useEffect(() => {
    const changed = previousRefreshKey.current !== refreshKey; previousRefreshKey.current = refreshKey;
    if (loadedIdentity.current === identity && changed) void load(undefined, true);
  }, [refreshKey, identity, load]);
  useEffect(() => {
    if (!autoRefresh || (page?.activity?.value && isFinishedJob(page.activity.value))) return;
    const timer = window.setInterval(() => {
      if (onSnapshot && controller.current && (!pageRef.current || 'current_input' in pageRef.current)) return;
      void load(undefined, true);
    }, refreshIntervalMs);
    return () => window.clearInterval(timer);
  }, [autoRefresh, refreshIntervalMs, page?.activity, load, onSnapshot]);
  const observed = page?.activity?.availability === 'available' ? page.activity.value : undefined;
  const latest = observed ?? metadata;
  const terminal = !!latest && isFinishedJob(latest);
  const outcome = terminal && latest ? jobStatusValue(latest) : undefined;
  const manifest = page?.final_documents?.availability === 'available' ? page.final_documents.value : undefined;
  const failed = outcome === 'failed_app' || outcome === 'failed_system' || outcome === 'failed_timeout';
  const outcomeLabel = failed ? (outcome === 'failed_timeout' ? 'Job timed out' : 'Job failed') : outcome && jobStatusPresentation(outcome).label;
  const completionText = latest?.completion_detail?.trim() ? latest.completion_detail : failed ? 'No failure details recorded for this job.' : undefined;
  const activeRequest = page?.current_input_entry_id && (!currentReviewId || object(page.current_input?.form.request).review_id === currentReviewId)
    ? entries.find(entry => entry.entry_id === page.current_input_entry_id)
    : currentRequestId ? entries.find(entry => entry.request_id === currentRequestId) : undefined;
  const completionSender = outcome === 'CANCELLED' ? latest?.cancelled_by || 'Unknown' : cellName || 'Job status';
  const completion: ThreadItem | undefined = (terminal || manifest?.documents?.length) ? {
    key: 'completion', ariaLabel: 'Job completion', sender: completionSender, subject: outcomeLabel || 'Attachments',
    label: failed ? 'Failure' : outcome === 'CANCELLED' ? 'Cancellation' : terminal ? 'Completion' : undefined,
    time: { value: latest?.archived_at },
    node: <div className="thread-completion">
      <div role={failed ? 'alert' : undefined}>
        {completionText && <p className="thread-completion-text">{completionText}</p>}
      </div>
      <FinalDocuments inline tenant={tenant} job={job} terminal={terminal || !!manifest?.documents?.length} manifest={manifest} onRetry={()=>void load()} />
    </div>,
  } : undefined;
  const current: ThreadItem | undefined = children ? {
    key: currentReviewId ? JSON.stringify(['review', currentReviewId]) : currentRequestId || 'current',
    reviewId: currentReviewId, forceOpen: openCurrent, occurrence: activeRequest?.entry_id,
    time: { value: activeRequest?.requested_at?.present ? activeRequest.requested_at.raw : currentRequestTime },
    sender: cellName || 'Cell', subject: currentRequestLabel || 'Waiting for your response', subjectIsPrompt: currentSubjectIsPrompt, label: currentIsReview ? 'Review' : 'Question', node: children,
  } : undefined;
  if (onSnapshot && !page && !error) return <p role="status">Loading history…</p>;
  const selected = occurrence ?? requestId;
  const matches = selected === undefined ? [] : entries.filter(entry => entry.entry_id === selected || (occurrence === undefined && entry.request_id === selected));
  return <section aria-label={folded ? 'Thread messages' : 'Input history'}>
    {error && <Alert type="warning" message="History unavailable" description={error} />}
    {page?.diagnostics?.map((d, i) => <Alert key={i} type="info" message={d.message} />)}
    {selected !== undefined && (page && 'current_input' in page
      ? matches.length === 1
        ? <RecordedRequest tenant={tenant} job={job} detail={threadReviewDetail(matches[0],page.snapshot_token)} onBack={onBack} onError={failure => { if (failure instanceof APIError && failure.code === 'history_snapshot_expired') void refreshThread(); }} />
        : <Alert type="warning" message="Request unavailable" description={matches.length ? 'Request identity is ambiguous. Open its occurrence link.' : 'The selected request is not retained.'} />
      : (!onSnapshot || page) && <HistoricalRequest key={`${identity}\0${occurrence ?? requestId}`} tenant={tenant} job={job} entry={occurrence ?? requestId!} legacy={occurrence === undefined} onBack={onBack} />)}
    {/* Keep arriving activity below the historical reader so it does not move the reading position. */}
    {activity && <span role="status">New thread activity available</span>}
    {!folded && page && !entries.length && !page.merges?.length && <p>No retained events in this history.</p>}
    {folded ? <FoldedConversationTimeline page={loadedIdentity.current === identity ? page : undefined} entries={loadedIdentity.current === identity ? entries : []} onOpen={onOpen} currentRequestId={currentRequestId} cellName={cellName} tenant={tenant} job={job}
      initial={initial} trailing={[...(current && (!completion || openCurrent) ? [current] : []), ...(completion ? [completion] : [])]}
      selectedOccurrence={occurrence} ready={!!page || !!error} />
      : <ConversationTimeline page={loadedIdentity.current === identity ? page : undefined} entries={loadedIdentity.current === identity ? entries : []} onOpen={onOpen} />}
    {page?.next_page_token && <Button disabled={busy || !!error} onClick={() => void load(page.next_page_token)}>Load more events</Button>}
    {!page && !error && <p role="status">Loading history…</p>}
    {!(folded && (current || terminal)) && <CurrentActivity inline={folded} metadata={observed ?? (latest?.status === 'PENDING_JOBS' ? latest : undefined)} freshness={error ? 'failed' : 'loaded'}
      dependencies={folded && latest?.status === 'PENDING_JOBS' && <ThreadDependencies fromHistory={!!page && 'current_input' in page} tenant={tenant} job={job} details={{...latest,wait_for:latest.wait_for ?? metadata?.wait_for,blocking_wait:latest.blocking_wait ?? metadata?.blocking_wait}} />} />}
    {!folded && <FinalDocuments tenant={tenant} job={job} terminal={!page?.activity?.value || isFinishedJob(page.activity.value)} manifest={page?.final_documents?.availability === 'available' ? page.final_documents.value : undefined} error={page && page.final_documents?.availability !== 'available' ? 'Returned documents could not be read.' : undefined} stale={!!error} onRetry={()=>void load()}/>}


  </section>;
}
