import type { TreeNode } from '@colony2/shared';
import { isFinishedJob } from '../utils/jobStatus';
const live = new Set(['READY','PENDING_JOBS','AWAITING_FUTURE','ACTIVE','CRASH_CONCERN','EXPIRED']);
function timestamp(value: string | undefined, now: number) {
  // The compact contract uses RFC3339. Date.parse alone also accepts local
  // dates, numeric strings and rolled-over days, which cannot prove timing.
  const match = value?.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/);
  if (!match) return undefined;
  const [, year, month, day, hour, minute, second, zone] = match;
  const days = new Date(Date.UTC(Number(year), Number(month), 0)).getUTCDate();
  if (Number(month) < 1 || Number(month) > 12 || Number(day) < 1 || Number(day) > days
    || Number(hour) > 23 || Number(minute) > 59 || Number(second) > 59
    || (zone !== 'Z' && (Number(zone.slice(1,3)) > 23 || Number(zone.slice(4)) > 59))) return undefined;
  const parsed = Date.parse(value!);
  return Number.isFinite(parsed) && parsed > 0 && parsed <= now ? parsed : undefined;
}
export function elapsed(milliseconds: number) {
  let seconds = Math.floor(milliseconds / 1000);
  const parts: string[] = [];
  for (const [size, suffix] of [[86400,'d'],[3600,'h'],[60,'m'],[1,'s']] as const) {
    const value = Math.floor(seconds / size); seconds %= size;
    if (value) parts.push(`${value}${suffix}`);
  }
  return parts.join(' ') || '0s';
}
export function timing(node: TreeNode, root: TreeNode | undefined, now: number) {
  const created = timestamp(node.created_at, now), rootCreated = timestamp(root?.created_at, now);
  let submission = 'Submitted Unavailable';
  if (created !== undefined && rootCreated !== undefined) {
    if (node.job_id === root?.job_id) {
      submission = `Submitted ${new Intl.DateTimeFormat('en-US', {timeZone:'UTC',year:'numeric',month:'short',day:'numeric',hour:'2-digit',minute:'2-digit',hour12:false}).format(created)} UTC`;
    } else if (created >= rootCreated) submission = `Submitted +${elapsed(created-rootCreated)}`;
  }
  const finished = isFinishedJob(node);
  const end = timestamp(node.archived_at, now);
  let span = `${finished ? 'Time to finish' : 'Age'} Unavailable`;
  if (created !== undefined) {
    if (finished && end !== undefined && end >= created) span = `Time to finish ${elapsed(end-created)}`;
    else if (!finished && live.has(node.status)) span = `Age ${elapsed(now-created)}`;
  }
  return {submission, span};
}
