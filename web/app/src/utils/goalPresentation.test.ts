import { expect, it } from 'vitest';
import type { RecipeJob } from '@colony2/shared/types';
import { abbreviatePrompt, latestActiveJob } from './goalPresentation';

it('limits a normalized prompt to 75 characters without splitting Unicode characters', () => {
  expect(abbreviatePrompt('  A short\n prompt  ')).toBe('A short prompt');
  expect(abbreviatePrompt('a'.repeat(75))).toBe('a'.repeat(75));
  expect(abbreviatePrompt('a'.repeat(74) + '🌱' + 'b'.repeat(40))).toBe('a'.repeat(74) + '🌱...');
});

it('opens the newest active attempt, or the newest attempt when none is active', () => {
  const jobs = [
    { job_id: 'recent-failure', store: 'ARCHIVED', status: 'COMPLETED', created_at: '2026-10-27T12:00:00Z' },
    { job_id: 'older-running', store: 'ACTIVE', status: 'ACTIVE', created_at: '2026-10-25T12:00:00Z' },
    { job_id: 'newer-waiting', store: 'ACTIVE', status: 'PENDING_JOBS', created_at: '2026-10-26T12:00:00Z' },
  ] as RecipeJob[];
  expect(latestActiveJob(jobs).job_id).toBe('newer-waiting');
  expect(latestActiveJob(jobs.map(job => ({ ...job, store: 'ARCHIVED' }))).job_id).toBe('recent-failure');
});
