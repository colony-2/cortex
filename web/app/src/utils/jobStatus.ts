import type { RecipeJobSummary, RecipeJobFilterStatus } from '@colony2/shared/types';

type StatusPresentation = { label: string; color: string };

export const filterStatuses = {
  READY: { label: 'Ready', color: 'blue' },
  PENDING_JOBS: { label: 'Waiting for jobs', color: 'gold' },
  AWAITING_FUTURE: { label: 'Scheduled', color: 'cyan' },
  ACTIVE: { label: 'Running', color: 'purple' },
  CRASH_CONCERN: { label: 'Worker unresponsive', color: 'orange' },
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
export function jobStatusValue(job: RecipeJobSummary): RecipeJobFilterStatus {
  // A previous failed attempt must never override a job that is still retrying.
  if (job.store === 'ARCHIVED' || job.status === 'COMPLETED') {
    if (job.completion_status) {
      switch (job.completion_status) {
        case 'cancelled': return 'CANCELLED';
        case 'success': case 'failed_app': case 'failed_system': case 'failed_timeout': return job.completion_status;
        default: return `completion:${job.completion_status}`;
      }
    }
  } else if (job.cancel_requested) {
    return 'CANCEL_REQUESTED';
  }
  return job.status === 'COMPLETED' ? 'OUTCOME_UNAVAILABLE' : job.status;
}

export function jobStatus(job: RecipeJobSummary): StatusPresentation {
  return jobStatusPresentation(jobStatusValue(job));
}

export function jobStatusOptions(jobs: RecipeJobSummary[], selected: RecipeJobFilterStatus[]) {
  const statuses = new Set<string>([...Object.keys(filterStatuses), ...selected, ...jobs.map(jobStatusValue)]);
  return [...statuses].map(value => ({ value, label: jobStatusPresentation(value).label }));
}
