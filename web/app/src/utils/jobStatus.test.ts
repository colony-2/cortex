import { describe, expect, it } from 'vitest';
import type { RecipeJob } from '@colony2/shared/types';
import { jobStatus } from './jobStatus';

const job: RecipeJob = {
  tenant_id: 'tenant', job_id: 'job', job_type: 'recipe', recipe: 'test',
  created_at: '', available_at: '', status: 'COMPLETED', store: 'ARCHIVED',
};

describe('job display status', () => {
  it.each([
    ['success', 'Succeeded', 'green'],
    ['failed_app', 'Failed (application)', 'red'],
    ['failed_system', 'Failed (system)', 'red'],
    ['failed_timeout', 'Timed out', 'red'],
    ['cancelled', 'Cancelled', 'default'],
    ['future_outcome', 'Finished (future_outcome)', 'default'],
    ['constructor', 'Finished (constructor)', 'default'],
    ['', 'Finished (outcome unavailable)', 'default'],
  ])('displays completion %s without assuming success', (completion_status, label, color) => {
    expect(jobStatus({ ...job, completion_status })).toEqual({ label, color });
  });

  it('keeps running and retrying jobs active even if an old completion field is present', () => {
    expect(jobStatus({ ...job, status: 'ACTIVE', store: 'ACTIVE', completion_status: 'failed_system' }).label).toBe('Running');
    expect(jobStatus({ ...job, status: 'AWAITING_FUTURE', store: 'ACTIVE', completion_status: 'failed_app' }).label).toBe('Scheduled');
  });

  it('distinguishes a cancellation request from terminal cancellation', () => {
    expect(jobStatus({ ...job, status: 'CANCELLED', store: 'ACTIVE', cancel_requested: true }).label).toBe('Cancellation requested');
    expect(jobStatus({ ...job, status: 'CANCELLED', cancel_requested: true, completion_status: 'cancelled' }).label).toBe('Cancelled');
  });
});
