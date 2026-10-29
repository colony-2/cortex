import { describe, expect, it } from 'vitest';
import type { RecipeJob } from '@colony2/shared/types';
import { jobStatus, jobStatusOptions } from './jobStatus';
import { jobFilterSearch, readJobFilters } from './jobFilters';

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

  it('offers the displayed outcomes with a single Cancelled option and no generic Completed option', () => {
    const options = jobStatusOptions([{ ...job, completion_status: 'future_outcome' }], ['completion:previous_outcome']);
    expect(options.map(option => option.label)).toEqual([
      'Ready', 'Waiting for jobs', 'Scheduled', 'Running', 'Worker unresponsive', 'Expired',
      'Cancellation requested', 'Cancelled', 'Succeeded', 'Failed (application)', 'Failed (system)',
      'Timed out', 'Finished (outcome unavailable)', 'Finished (previous_outcome)', 'Finished (future_outcome)',
    ]);
    expect(options.some(option => option.value === 'COMPLETED')).toBe(false);
    for (const completion_status of ['success', 'failed_app', 'failed_system', 'failed_timeout', 'cancelled', '', 'future_outcome']) {
      expect(options.some(option => option.label === jobStatus({ ...job, completion_status }).label)).toBe(true);
    }
  });

  it('preserves mixed display filters and unknown outcomes through navigation', () => {
    const params = new URLSearchParams('status=READY&status=success&status=failed_app&status=CANCEL_REQUESTED&status=OUTCOME_UNAVAILABLE&status=completion%3Afuture_outcome&status=constructor&cell=platform&pageToken=next');
    expect(readJobFilters(params)).toEqual({ status: ['READY', 'success', 'failed_app', 'CANCEL_REQUESTED', 'OUTCOME_UNAVAILABLE', 'completion:future_outcome'], cell: 'platform' });
    const restored = new URLSearchParams(jobFilterSearch(params));
    expect(readJobFilters(restored)).toEqual(readJobFilters(params));
    expect(restored.get('pageToken')).toBe('next');
  });

  it('keeps the outcome-unavailable selection in links created by the old dropdown', () => {
    expect(readJobFilters(new URLSearchParams('status=COMPLETED'))).toEqual({ status: ['OUTCOME_UNAVAILABLE'], cell: undefined });
  });
});
