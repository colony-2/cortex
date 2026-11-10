import type { PendingInput } from '@colony2/shared';

export function requestTime(input: PendingInput): number {
  return input.requested_at ? Date.parse(input.requested_at) : NaN;
}
const lexical = (a: string, b: string) => a < b ? -1 : a > b ? 1 : 0;
export function compareReviewRequests(a: PendingInput, b: PendingInput): number {
  const left = requestTime(a), right = requestTime(b);
  const knownLeft = Number.isFinite(left), knownRight = Number.isFinite(right);
  if (knownLeft !== knownRight) return knownLeft ? -1 : 1;
  if (knownLeft && left !== right) return left - right;
  return lexical(a.id, b.id) || (a.task_ordinal ?? -1) - (b.task_ordinal ?? -1) || lexical(a.request_id ?? '', b.request_id ?? '');
}
export function reviewAge(timestamp: number, now: number): string {
  if (!Number.isFinite(timestamp)) return 'Request time unavailable';
  const minutes = Math.floor(Math.max(0, now - timestamp) / 60000);
  if (minutes < 1) return 'Requested just now';
  const [count, unit] = minutes < 60 ? [minutes, 'minute'] : minutes < 1440 ? [Math.floor(minutes / 60), 'hour'] : [Math.floor(minutes / 1440), 'day'];
  return `Requested ${count} ${unit}${count === 1 ? '' : 's'} ago`;
}
export function exactRequestTime(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'full', timeStyle: 'long' }).format(timestamp);
}
