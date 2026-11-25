import { hasThreadInput, useThreadInput, ThreadSnapshotContext } from './useThreadInput';
import { useCallback, useContext, useEffect, useRef, useState } from 'react';
import { APIError, getConversation, getConversationEntry, inputActivityService, type ReviewV2Output, type ReviewV2Request, type UserInputDetails } from '@colony2/shared';
import { reconcileReviewV2, reviewV2FromForm, type RecordedReview, type ReviewV2View } from '../utils/reviewV2State';
import { isFinishedJob } from '../utils/jobStatus';
import { object } from '../components/conversationPresentation';

export function useReviewV2(tenant: string, job: string, selected: string, initial?: UserInputDetails, contextJob?: ReviewV2View['job']) {
  const thread = useContext(ThreadSnapshotContext);
  const unified = hasThreadInput(thread);
  const selectedInput = useThreadInput(tenant, job);
  const currentRef = useRef(selectedInput); currentRef.current = selectedInput;
  const retained = useRef<{detail?: RecordedReview; job?: ReviewV2View['job']; limitations: string}>();
  const [historyVersion, setHistoryVersion] = useState(0);
  const [view,setView] = useState<ReviewV2View>();
  const [error,setError] = useState('');
  const [busy,setBusy] = useState(false);
  const latest = useRef(view);
  const provisional = useRef<{request:ReviewV2Request;output:ReviewV2Output}>();
  const controller = useRef<AbortController>();
  const active = useRef(true);
  const running = useRef<Promise<void>>();
  const rerun = useRef(false);
  const generation = useRef(0);
  const inFlight=useRef(false);
  const refresh = useCallback((): Promise<void> => {
    if (unified) return Promise.resolve();
    const observation = currentRef.current;
    const form = observation.details?.form;
    if (provisional.current?.output.receipt.request_id !== form?.request_id && observation.discovery.status === 'current' &&
        object(form?.presentation).type === 'review/v2' && object(form?.request).review_id === selected) {
      return Promise.resolve();
    }
    if (running.current) {rerun.current = true; return running.current;}
    const turn = ++generation.current;
    const abort = new AbortController(); controller.current = abort;
    setBusy(true);
    const run = async () => {
      try {
        // Acquire a complete immutable snapshot. Snapshot expiry restarts once.
        let detail: RecordedReview | undefined;
        let pendingRequest: string | undefined;
        let page;
        for (let attempt=0;attempt<2;attempt++) {
          try {
            page = await getConversation(tenant,job,{reviewId:selected,signal:abort.signal});
            const first = page; const entries = [...page.entries];
            while (page.next_page_token) {
              page = await getConversation(tenant,job,{pageToken:page.next_page_token,signal:abort.signal});
              if (page.snapshot_token !== first.snapshot_token) throw new Error('History snapshot changed during discovery.');
              entries.push(...page.entries);
            }
            page = first;
            if(abort.signal.aborted || turn !== generation.current || !active.current)return;
            const matches = entries.filter(e => object(e.form_fields.presentation).type === 'review/v2' && object(e.form_fields.request).review_id === selected)
              .sort((a,b) => (b.locator.collection_ordinals?.[0] ?? b.locator.generation_ordinal ?? 0) - (a.locator.collection_ordinals?.[0] ?? a.locator.generation_ordinal ?? 0));
            pendingRequest = matches[0]?.state === 'pending' ? matches[0].request_id : undefined;
            if (matches.length && first.review_id === selected) {
              const entry = matches[0];
              detail = {...entry, raw_form: entry.form_fields, raw_accepted_output: entry.accepted_response.raw};
            } else if (matches.length) detail = await getConversationEntry(tenant,job,matches[0].entry_id,{snapshotToken:first.snapshot_token,signal:abort.signal});
            break;
          } catch (e) {if (attempt === 0 && e instanceof APIError && e.code === 'history_snapshot_expired') continue; throw e;}
        }
        if (abort.signal.aborted || !active.current || turn !== generation.current) return;
        const jobValue = page?.activity?.availability === 'available' ? page.activity.value : undefined;
        const limitations = [page?.activity?.availability !== 'available' ? 'Job status is unavailable.' : '', ...(page?.diagnostics ?? []).map(d=>d.message)];
        const newPendingRequest = pendingRequest && pendingRequest !== retained.current?.detail?.request_id;
        retained.current = {detail, job: jobValue, limitations: limitations.filter(Boolean).join(' ')};
        setHistoryVersion(value => value + 1);
        if (newPendingRequest && !currentRef.current.loading && pendingRequest !== currentRef.current.details?.form.request_id) {
          // Older servers discover live forms separately. A new pending history entry
          // can arrive before its discovery event, especially after reopening a review.
          void currentRef.current.refresh().catch(() => {});
        }
      } catch (e) {
        if (abort.signal.aborted || !active.current || turn !== generation.current) return;
        setError(e instanceof Error ? e.message : String(e));
        if (latest.current) {const next = {...latest.current,current:undefined,phase:'unavailable' as const}; latest.current=next; setView(next);}
      } finally {
        if (turn === generation.current) {setBusy(false); running.current=undefined;}
        if (rerun.current && active.current && !abort.signal.aborted) {rerun.current=false; void refresh();}
      }
    };
    running.current=run(); return running.current;
  },[tenant,job,selected,unified]);
  useEffect(() => {
    active.current=true;
    if (initial) {
      const request=reviewV2FromForm(initial.form);
      if (request?.review_id === selected) {const first:ReviewV2View={request,phase:'unavailable',conversation:request.conversation};latest.current=first;setView(first);}
    }
    const events = ['connected','inputs_changed','input_pending','input_completed','input_cancelled'];
    const changed=()=>{if(currentRef.current.loading && !retained.current)return;if(latest.current?.job&&isFinishedJob(latest.current.job))return;void refresh();};
    events.forEach(e=>inputActivityService.on(e,changed));
    return ()=>{active.current=false;generation.current++;controller.current?.abort();running.current=undefined;events.forEach(e=>inputActivityService.off(e,changed));};
  },[refresh,initial,selected]);
  const discovering = selectedInput.discovery.status === 'loading' || selectedInput.loading;
  useEffect(() => {
    // Prefer the waiting task, which already holds the whole review. Retained
    // history can still recover a saved review if discovery stalls or fails.
    if (discovering) {
      const timer = window.setTimeout(() => void refresh(), 5000);
      return () => window.clearTimeout(timer);
    }
    void refresh();
  }, [discovering, selectedInput.version, selectedInput.details, selectedInput.discovery.status, refresh]);
  useEffect(() => {
    const page = unified ? thread?.page : undefined;
    const entry = page?.entries.filter(entry => object(entry.form_fields.presentation).type === 'review/v2' && object(entry.form_fields.request).review_id === selected).pop();
    const saved = unified ? {
      detail:entry && {...entry, raw_form:entry.form_fields, raw_accepted_output:entry.accepted_response.raw},
      job:page?.activity?.value,
      limitations:thread?.error,
    } : retained.current;
    const current = selectedInput.details ?? undefined;
    try {
      const next = reconcileReviewV2(selected,current,saved?.detail,saved?.job ?? contextJob,provisional.current,latest.current, selectedInput.discovery.status === 'current' && !selectedInput.loading && !selectedInput.error);
      if (inFlight.current) {next.phase = 'sending'; next.current = undefined;}
      latest.current = next; setView(next);
      setError([saved?.limitations, selectedInput.error, selectedInput.discovery.error].filter(Boolean).join(' '));
    } catch (error) {
      if (!selectedInput.loading && selectedInput.discovery.status === 'current') setError(error instanceof Error ? error.message : String(error));
    }
  }, [historyVersion, selectedInput.details, selectedInput.loading, selectedInput.error, selectedInput.discovery, selected, contextJob, thread, unified]);
  const retry = useCallback(async () => {
    await Promise.all([refresh(), currentRef.current.refresh().catch(() => {})]);
  }, [refresh]);
  const terminal = view?.phase === 'terminal' || !!(view?.job && isFinishedJob(view.job));
  useEffect(()=>{
    if (unified || terminal) return;
    const timer = window.setInterval(() => {
      if (!running.current) void refresh();
    }, 5000);
    return () => window.clearInterval(timer);
  },[terminal,refresh,unified]);
  const sending=()=>{inFlight.current=true;if(latest.current){const next={...latest.current,current:undefined,phase:'sending' as const};latest.current=next;setView(next);}};
  const accepted=(request:ReviewV2Request,output:ReviewV2Output)=>{
    inFlight.current=false;provisional.current={request,output};
    const next=reconcileReviewV2(selected,undefined,undefined,latest.current?.job,provisional.current,latest.current);
    latest.current=next;setView(next);void (unified && thread ? thread.refresh() : refresh());
  };
  const safeView = view?.current && (view.current !== selectedInput.details || selectedInput.discovery.status !== 'current')
    ? {...view, current: undefined, phase: 'unavailable' as const} : view;
  return {view:safeView,error,busy,refresh:retry,sending,accepted,failed:()=>{inFlight.current=false;}};
}
