import { stringifyJobJSON, type HistoryEntry, type HistoryValue } from '@colony2/shared';
export const object = (v: unknown): Record<string, any> => v !== null && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, any> : {};
export const valueText = (v: unknown): string => typeof v === 'string' ? v : stringifyJobJSON(v, 2) ?? 'Unavailable';
export function recordedTime(value: HistoryValue): { text: string; title?: string } {
  const raw = value.raw;
  if (!value.present || typeof raw !== 'string' || !/^\d{4}-\d\d-\d\dT/.test(raw) || raw.startsWith('0001-') || raw.startsWith('0000-')) return {text:'Time not recorded'};
  const parts = /^(\d{4})-(\d{2})-(\d{2})T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.exec(raw);
  if (!parts) return {text:'Time not recorded'};
  const calendar = new Date(Date.UTC(Number(parts[1]), Number(parts[2])-1, Number(parts[3])));
  if (calendar.getUTCMonth() !== Number(parts[2])-1 || calendar.getUTCDate() !== Number(parts[3])) return {text:'Time not recorded'};
  const date = new Date(raw);
  if (!Number.isFinite(date.getTime())) return {text:'Time not recorded'};
  return {text:date.toLocaleString(undefined, {year:'numeric',month:'short',day:'numeric',hour:'numeric',minute:'2-digit'}),title:date.toLocaleString(undefined,{timeZoneName:'long'})};
}
export function savedAnswers(fields: Record<string, unknown>, response: unknown, compact = false): string {
  const questions = Array.isArray(fields.fields) ? fields.fields : [];
  const label = (question: any, answer: unknown) => {
    const options = Array.isArray(question.options) ? question.options : [];
    const single = (v: unknown) => { const option = options.find((o: any) => object(o).value === v); return option ? valueText(option.label ?? option.value) : valueText(v); };
    return Array.isArray(answer) ? answer.map(single).join(', ') : single(answer);
  };
  if (questions.length) {
    const envelope = object(response);
    const answers = Object.keys(object(envelope.fields)).length ? object(envelope.fields) : envelope;
    const known = new Set(questions.map(q => object(q).id));
    return [...questions.filter(q => Object.prototype.hasOwnProperty.call(answers, object(q).id)).map(q => `${compact ? '' : `${object(q).question || object(q).id}: `}${label(object(q), answers[object(q).id])}`), ...Object.entries(answers).filter(([id]) => !known.has(id)).map(([id,v]) => `${id}: ${valueText(v)}`)].join(compact ? ' · ' : '\n') || valueText(response);
  }
  const envelope = object(response);
  return label(fields, Object.prototype.hasOwnProperty.call(envelope,'response') ? envelope.response : response);
}
export function reviewer(entry: HistoryEntry): string {
  const actor = object(object(entry.actor.receipt).actor);
  const id = actor.id ? ` · ${valueText(actor.id)}` : '';
  switch (entry.actor.provenance) {
    case 'submission_receipt': return `Submission receipt${id}`;
    case 'reported_receipt': return `Caller-reported identity${id}; not an authenticated human submission.`;
    case 'automation_receipt': return `Automated response.${id}`;
    case 'reported_legacy': return `Legacy reported identity · ${entry.actor.legacy_user_id.present ? valueText(entry.actor.legacy_user_id.raw) : 'unavailable'}`;
    default: return 'Reviewer unavailable';
  }
}
