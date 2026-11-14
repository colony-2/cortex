import type { RecipeJobSummary, RecipeJobFilterStatus } from '@colony2/shared/types';

export type JobStatusInput = { status: string; store?: string; completion_status?: string; cancel_requested?: boolean; archived_at?: string };

type StatusPresentation = { label: string; color: string };

export const filterStatuses = {
  READY: { label: 'Ready', color: 'blue' },
  PENDING_JOBS: { label: 'Waiting for jobs', color: 'gold' },
  AWAITING_FUTURE: { label: 'Scheduled', color: 'cyan' },
  ACTIVE: { label: 'Running', color: 'purple' },
  CRASH_CONCERN: { label: 'Job keeps failing', color: 'orange' },
  EXPIRED: { label: 'Expired', color: 'orange' },
  CANCEL_REQUESTED: { label: 'Cancellation requested', color: 'orange' },
  CANCELLED: { label: 'Cancelled', color: 'default' },
  success: { label: 'Succeeded', color: 'green' },
  failed_app: { label: 'Failed (application)', color: 'red' },
  failed_system: { label: 'Failed (system)', color: 'red' },
  failed_timeout: { label: 'Timed out', color: 'red' },
  OUTCOME_UNAVAILABLE: { label: 'Finished (outcome unavailable)', color: 'default' },
} satisfies Record<Exclude<RecipeJobFilterStatus, `completion:${string}`>, StatusPresentation>;

export function isJobFilterStatus(value: string): value is RecipeJobFilterStatus {
  return Object.prototype.hasOwnProperty.call(filterStatuses, value)
    || (value.startsWith('completion:') && value.length > 'completion:'.length);
}

export function jobStatusPresentation(status: string): StatusPresentation {
  if (Object.prototype.hasOwnProperty.call(filterStatuses, status)) return filterStatuses[status as keyof typeof filterStatuses];
  return { label: status.startsWith('completion:') ? `Finished (${status.slice('completion:'.length)})` : status, color: 'default' };
}

// Keep this precedence in sync with jobDisplayStatus in the Cortex API.
export function jobStatusValue(job: JobStatusInput): string {
  // Earlier attempt outcomes never override an unfinished scheduler state.
  if (job.store === 'ARCHIVED' || job.archived_at || job.status === 'COMPLETED') {
    switch (job.completion_status) {
      case 'cancelled': return 'CANCELLED';
      case 'success': case 'failed_app': case 'failed_system': case 'failed_timeout': return job.completion_status;
      default: return job.completion_status ? `completion:${job.completion_status}` : 'OUTCOME_UNAVAILABLE';
    }
  }
  return job.cancel_requested ? 'CANCEL_REQUESTED' : job.status;
}

export function jobStatus(job: JobStatusInput): StatusPresentation {
  return jobStatusPresentation(jobStatusValue(job));
}

export function jobStatusOptions(jobs: RecipeJobSummary[], selected: RecipeJobFilterStatus[]) {
  const statuses = new Set<string>([...Object.keys(filterStatuses), ...selected, ...jobs.map(jobStatusValue)]);
  return [...statuses].map(value => ({ value, label: jobStatusPresentation(value).label }));
}

// Resolve every selected attempt from the loaded page; missing data fails closed.
export function canArchiveImmediately(ids: string[], jobs: RecipeJobSummary[]): boolean {
  const summaries = new Map(jobs.map(job => [job.job_id, job]));
  return ids.length > 0 && ids.every(id => {
    const job = summaries.get(id);
    return !!job && ['failed_app', 'failed_system', 'success', 'CANCELLED'].includes(jobStatusValue(job));
  });
}

// Terminal classification is broader than the existing immediate-archive policy.
export function isFinishedJob(job: JobStatusInput): boolean {
  if (job.store === 'ARCHIVED') return true;
  const status = jobStatusValue(job);
  return status === 'CANCELLED' || status === 'OUTCOME_UNAVAILABLE'
    || status?.startsWith('completion:') || ['success', 'failed_app', 'failed_system', 'failed_timeout'].includes(status);
}

// Fresh recovery requires raw exact-detail evidence, not a display label/outcome.
export function confirmsRecoveryCompletion(job?: JobStatusInput): boolean {
  return !!job && (job.store === 'ARCHIVED' || !!job.archived_at || job.status === 'COMPLETED');
}
