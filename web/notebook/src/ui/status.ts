import type { JobStatus, NodeStatus } from '../backend/types';

export function statusTagColor(status: JobStatus | NodeStatus | string): string {
  const colors: Record<string, string> = {
    running: 'blue',
    completed: 'green',
    succeeded: 'green',
    failed: 'red',
    canceled: 'default',
    terminated: 'default',
    timed_out: 'orange',
    skipped: 'gold',
    pending: 'default',
    unknown: 'default',
  };
  return colors[status] ?? 'default';
}

