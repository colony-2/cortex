import { parseReviewV2, stringifyJobJSON, type HistoryDetail, type ReviewV2Output, type ReviewV2Request, type ReviewV2Turn, type UserInputDetails } from '@colony2/shared';
import { object } from '../components/conversationPresentation';
import { isFinishedJob, jobStatus } from './jobStatus';
import type { RecipeJob } from '@colony2/shared';
export type ReviewV2Phase = 'actionable' | 'sending' | 'waiting' | 'sent' | 'other' | 'terminal' | 'unavailable';
export interface ReviewV2View {
  request: ReviewV2Request; current?: UserInputDetails; history?: HistoryDetail; phase: ReviewV2Phase;
  documentHistory?: HistoryDetail;
  declarations?: Record<string, ReviewV2Request>; message?: string; conversation: ReviewV2Turn[]; output?: ReviewV2Output; job?: RecipeJob;
}
export function reviewV2FromForm(form: unknown): ReviewV2Request | undefined {
  const f = object(form);
  if (object(f.presentation).type !== 'review/v2') return undefined;
  return parseReviewV2(f.request);
}
export function reconcileReviewV2(selected: string, current: UserInputDetails | undefined, history: HistoryDetail | undefined, job: RecipeJob | undefined, provisional?: {request: ReviewV2Request; output: ReviewV2Output}, previous?: ReviewV2View): ReviewV2View {
  const live = current && reviewV2FromForm(current.form);
  const recorded = history && reviewV2FromForm(history.raw_form);
  const liveMatches=live?.review_id===selected;
  const recordedMatches=recorded?.review_id===selected;
  const acceptedSame=history?.accepted_response.present && recordedMatches && (!liveMatches || current?.form.request_id===history?.request_id);
  const provisionalSame=provisional?.request.review_id===selected && (!liveMatches || current?.form.request_id===provisional.output.receipt.request_id) && (!recordedMatches || history?.request_id===provisional.output.receipt.request_id);
  const request = acceptedSame ? recorded : provisionalSame ? provisional?.request : liveMatches ? live : recordedMatches ? recorded : previous?.request;
  if (!request || request.review_id !== selected) throw new Error('The selected review is not available in retained history.');
  let output: ReviewV2Output | undefined;
  if (acceptedSame && history) {
    const raw = object(history.raw_accepted_output);
    const response = object(raw.response ?? object(history.accepted_response.raw).response ?? history.accepted_response.raw);
    output = {response: response as ReviewV2Output['response'],receipt:object(raw.receipt ?? history.actor.receipt)};
  } else if (provisionalSame) output = provisional?.output;
  // The successor owns the entire thread. Never append provisional history to it.
  const conversation = [...request.conversation];
  if (output?.response && typeof output.response.direction === 'string' && typeof output.response.feedback === 'string') {
    const receipt = output.receipt;
    const known = conversation.some(t => t.role === 'human' && receipt.request_id && t.receipt?.request_id === receipt.request_id && t.receipt?.submission_id === receipt.submission_id);
    if (!known) conversation.push({role:'human',direction:output.response.direction,text:output.response.feedback,attachments:output.response.attachments ?? [],settings:output.response.settings,receipt});
  }
  // Original mappings can outlive a response observation. Keep the last verified
  // manifest while acceptance/history is pending, without using it for new
  // contribution bindings. A different review or changed original cannot reuse it.
  const documentHistory = [history, previous?.documentHistory, previous?.history].find(detail => {
    const frozen = detail && reviewV2FromForm(detail.raw_form);
    return frozen?.review_id === selected && stringifyJobJSON(frozen.documents) === stringifyJobJSON(request.documents);
  });
  const base = {request,history,documentHistory,conversation,output,job};
  if (job && isFinishedJob(job)) return {...base,phase:'terminal',message:`Job ${jobStatus(job).label.toLowerCase()}.`};
  // A recorded receipt takes precedence over a stale current-form observation.
  const answeredCurrent = output?.receipt?.request_id && output.receipt.request_id === current?.form.request_id;
  if (live?.review_id === selected && !answeredCurrent) return {...base,current,phase:'actionable'};
  if (current && live?.review_id !== selected) return {...base,phase:'other',message:'Another request is now pending. This saved review remains readable.'};
  if (output) {
    const direction = request.directions.find(d => d.id === output?.response.direction);
    if (!direction) return {...base,phase:'unavailable',message:'The recorded direction is unavailable. Refresh to reconcile this review.'};
    return {...base,phase:direction.continues_review ? 'waiting' : 'sent'};
  }
  return {...base,phase:'unavailable',message:'No matching current request or recorded acceptance is available. Refresh to reconcile this review.'};
}

export function reviewV2AnswerPending(view: ReviewV2View): boolean {
  const last=view.conversation[view.conversation.length-1];
  const receipt=view.output?.receipt;
  return view.phase==='waiting' && !!view.job && !view.job.cancel_requested && !!receipt?.request_id && !!receipt?.submission_id
    && last?.role==='human' && last.receipt?.request_id===receipt.request_id && last.receipt?.submission_id===receipt.submission_id;
}
