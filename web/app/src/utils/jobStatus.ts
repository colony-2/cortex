import type { RecipeJob, RecipeJobStatus } from '@colony2/shared/types';

export const schedulingStatuses: Record<RecipeJobStatus, { label: string; color: string }> = {
  READY: { label: 'Ready', color: 'blue' },
  PENDING_JOBS: { label: 'Waiting for jobs', color: 'gold' },
  AWAITING_FUTURE: { label: 'Scheduled', color: 'cyan' },
  ACTIVE: { label: 'Running', color: 'purple' },
  CRASH_CONCERN: { label: 'Worker unresponsive', color: 'orange' },
  EXPIRED: { label: 'Expired', color: 'orange' },
  CANCELLED: { label: 'Cancelled', color: 'default' },
  COMPLETED: { label: 'Finished (outcome unavailable)', color: 'default' },
};

const completionStatuses: Record<string, { label: string; color: string }> = {
  success: { label: 'Succeeded', color: 'green' },
  failed_app: { label: 'Failed (application)', color: 'red' },
  failed_system: { label: 'Failed (system)', color: 'red' },
  failed_timeout: { label: 'Timed out', color: 'red' },
  cancelled: { label: 'Cancelled', color: 'default' },
};

export function jobStatus(job: RecipeJob): { label: string; color: string } {
  // A previous failed attempt must never override a job that is still retrying.
  if (job.store === 'ARCHIVED' || job.status === 'COMPLETED') {
    if (job.completion_status) {
      return Object.prototype.hasOwnProperty.call(completionStatuses, job.completion_status)
        ? completionStatuses[job.completion_status]
        : { label: `Finished (${job.completion_status})`, color: 'default' };
    }
  } else if (job.cancel_requested) {
    return { label: 'Cancellation requested', color: 'orange' };
  }
  return Object.prototype.hasOwnProperty.call(schedulingStatuses, job.status)
    ? schedulingStatuses[job.status]
    : { label: job.status, color: 'default' };
}
