import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { RecipeJob, RecipeJobSummary } from '@colony2/shared/types';
import JobListDetails from './JobListDetails';
import { jobIdentity } from '../test/cellFixtures';

vi.mock('./JobLeaseTimeline', () => ({ default: () => null }));
afterEach(cleanup);

const job: RecipeJob = {
  ...jobIdentity, tenant_id: 'tenant', job_id: 'selected', goal_id: 'primary-H',
  job_type: 'recipe', recipe: 'build', status: 'PENDING_JOBS', store: 'ACTIVE',
  created_at: '2026-11-14T00:00:00Z', available_at: '2026-11-14T00:00:00Z',
  parent: { tenant_id: 'tenant', job_id: 'recorded-parent', goal_id: 'historical-parent-goal' },
};
const wait: NonNullable<RecipeJob['blocking_wait']> = {
  waitId: 'retained-wait', state: 'SATISFIED', deadline: '2026-11-20T12:00:00Z',
  dependencies: [{ target: { goalId: 'historical-G' }, condition: 'success', state: 'SATISFIED', satisfiedByJobId: 'producer-B', blockedReason: 'retained explanation' }],
};

it.each([
  ['summary while loading', ['summary-stale'], undefined],
  ['inspection only', undefined, ['inspection-stale']],
  ['both loaded', ['summary-stale'], ['inspection-stale']],
] as const)('excludes secondary IDs from %s without changing source data', (_, summaryIDs, inspectionIDs) => {
  // Simulate stale wire data which is deliberately absent from the summary type.
  const summary = { ...job, other_goal_ids: summaryIDs } as RecipeJobSummary;
  const inspection = inspectionIDs ? { ...job, other_goal_ids: [...inspectionIDs], blocking_wait: wait } : undefined;
  const before = JSON.stringify({ summary, inspection });
  const search = new URLSearchParams();
  for (const section of ['technical', 'data']) search.append('detail', JSON.stringify(['selected', section]));
  const { container } = render(<MemoryRouter initialEntries={['/?' + search]}><JobListDetails job={summary} inspection={inspection} projectId="tenant" /></MemoryRouter>);
  const raw = screen.getByText('All job data').closest('details')!.querySelector('pre')!.textContent!;
  expect(raw).not.toMatch(/other_goal_ids|summary-stale|inspection-stale/);
  expect(JSON.parse(raw).goal_id).toBe('primary-H');
  expect(JSON.stringify({ summary, inspection })).toBe(before);
  expect(new URL(screen.getByRole('link', { name: 'View parent execution' }).getAttribute('href')!, 'http://localhost').pathname).toBe('/project/tenant/jobs/recorded-parent/story');
  if (inspection) {
    expect(raw).not.toContain('historical-G');
    expect(screen.queryByRole('link', { name: 'View goal' })).toBeNull();
    expect(screen.queryByRole('link', { name: 'View execution' })).toBeNull();
  }
});

it('keeps job dependencies visible in a mixed wait', () => {
  const jobDependency = { target: { jobId: 'producer' }, condition: 'success', state: 'PENDING' };
  const inspection = { ...job, blocking_wait: { ...wait, state: 'PENDING', dependencies: [...wait.dependencies, jobDependency] } };
  render(<MemoryRouter><JobListDetails job={job} inspection={inspection} projectId="tenant" /></MemoryRouter>);
  expect(screen.getByRole('link', { name: 'Execution', exact: true })).toHaveAttribute('href', '/project/tenant/jobs/producer/story');
  const raw = screen.getByText('All job data').closest('details')!.querySelector('pre')!.textContent!;
  expect(JSON.parse(raw).blocking_wait.dependencies).toEqual([jobDependency]);
});

it('ENV-18 keeps each attempt’s requirements and encoded Execution link separate', () => {
  const make = (id: string, base: string) => ({ ...job, job_id: id, execution: { status: 'specified', source: 'submission', published: false, demand: { effective: { base } } } });
  render(<MemoryRouter initialEntries={['/?status=READY&cellId=cell-platform&requestId=ignored&occurrenceId=old']}>
    <JobListDetails job={make('one/雪', 'macOS')} projectId="tenant/雪" navigationState={{ goalsReturnTo: '/saved-goals' }} />
    <JobListDetails job={make('two', 'linux')} projectId="tenant/雪" />
  </MemoryRouter>);
  const first = new URL(screen.getByRole('link', { name: 'req: macOS' }).getAttribute('href')!, 'http://localhost');
  const second = new URL(screen.getByRole('link', { name: 'req: linux' }).getAttribute('href')!, 'http://localhost');
  expect(first.pathname).toBe('/project/tenant%2F%E9%9B%AA/jobs/one%2F%E9%9B%AA/story');
  expect(second.pathname).toBe('/project/tenant%2F%E9%9B%AA/jobs/two/story');
  expect(first.searchParams.get('tab')).toBe('runtime'); expect(first.searchParams.get('status')).toBe('READY');
  expect(first.searchParams.get('cellId')).toBe('cell-platform');
  expect(first.searchParams.has('requestId')).toBe(false); expect(first.searchParams.has('occurrenceId')).toBe(false);
});
