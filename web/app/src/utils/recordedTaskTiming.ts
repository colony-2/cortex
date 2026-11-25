import { isJobJSONNumber } from '@colony2/shared';
import type { StoryNode } from './storyTree';

const shortTaskMs = 10_000;

export function workerDurationMs(node: StoryNode): number | undefined {
  const runtime = node.metrics?.runtime;
  if (runtime?.status !== 'valid' || runtime.report?.version !== 1) return;
  let total = 0;
  let preparation = false;
  let execution = false;
  for (const metric of runtime.report.mutations ?? []) {
    if (metric.name !== 'c2j_task_duration_seconds' || metric.kind !== 'histogram') continue;
    const phase = metric.labels?.phase;
    if (phase !== 'preparation' && phase !== 'execution') continue;
    if (metric.op !== 'observe' && metric.op !== 'merge') return;
    const value = metric.op === 'merge' ? metric.delta?.sum : metric.value;
    const seconds = isJobJSONNumber(value) ? Number(value.toString()) : value;
    if (typeof seconds !== 'number' || !Number.isFinite(seconds) || seconds < 0) return;
    total += seconds;
    if (phase === 'preparation') preparation = true;
    if (phase === 'execution') execution = true;
  }
  const ms = total * 1000;
  if (Number.isFinite(ms) && preparation && (execution || runtime.phases?.execution === 'not_started')) return ms;
}

export function compactDuration(ms: number) {
  if (!Number.isFinite(ms) || ms < 0) return '—';
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  if (minutes >= 60) return `${Math.floor(minutes / 60)}h${minutes % 60}m`;
  if (minutes) return `${minutes}m${seconds % 60}s`;
  return `${seconds}s`;
}

export function waitsForUser(node: StoryNode) {
  return node.title === 'input:collect_user_input' || node.op_type === 'input:collect_user_input';
}

function taskInterval(node: StoryNode) {
  const attemptStart = Date.parse(node.started_at ?? '');
  const end = Date.parse(node.finished_at ?? '');
  const measured = workerDurationMs(node);
  const impliedStart = measured === undefined ? NaN : end - Math.round(measured);
  const estimated = Number.isFinite(new Date(impliedStart).getTime());
  let start = estimated ? impliedStart : attemptStart;
  if (estimated && Number.isFinite(attemptStart)) start = Math.max(start, attemptStart);
  return {kind: 'task' as const, node, start, end, attemptStart, estimated, measured, duration: measured ?? (end - attemptStart)};
}

export type TaskInterval = ReturnType<typeof taskInterval>;
export type ResourceWait = {kind: 'wait'; start: number; duration: number};
type TimelineRow = TaskInterval | ResourceWait;

export function recordedTimeline(nodes: StoryNode[], showShort: boolean): TimelineRow[] {
  const tasks = nodes.map(taskInterval);
  const gaps = new Map<TaskInterval, ResourceWait>();
  // Account for every interval before filtering, including overlapping work and human waits.
  // A missing timestamp or a new job attempt breaks the chain of evidence.
  let segment: TaskInterval[] = [];
  function findGaps() {
    const occupiedFrom = (t: TaskInterval) => waitsForUser(t.node) && Number.isFinite(t.attemptStart) ? t.attemptStart : t.start;
    segment.sort((a, b) => occupiedFrom(a) - occupiedFrom(b));
    let coveredUntil = segment.reduce((earliest, t) => Math.min(earliest, Number.isFinite(t.attemptStart) ? t.attemptStart : t.start), Infinity);
    for (const task of segment) {
      const duration = occupiedFrom(task) - coveredUntil;
      if (duration >= shortTaskMs) gaps.set(task, {kind: 'wait', start: coveredUntil, duration});
      coveredUntil = Math.max(coveredUntil, Number.isFinite(task.end) ? task.end : Infinity);
    }
    segment = [];
  }
  for (const task of tasks) {
    if (segment.length && segment[0].node.job_attempt !== task.node.job_attempt) findGaps();
    if (Number.isFinite(task.start)) segment.push(task);
    else findGaps();
  }
  findGaps();

  const rows: TimelineRow[] = [];
  for (const task of tasks) {
    const gap = gaps.get(task);
    if (gap) rows.push(gap);
    const short = task.duration >= 0 && task.duration < shortTaskMs;
    if (showShort || !short) rows.push(task);
  }
  return rows;
}
