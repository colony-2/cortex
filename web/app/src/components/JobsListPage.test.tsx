import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import type { RecipeJob } from '@colony2/shared/types';
import JobsListPage from './JobsListPage';
import JobStoryPage from './JobStoryPage';

vi.mock('@colony2/shared', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  useInputActivity: () => ({ pendingInputs: [], refresh: vi.fn() }),
}));
vi.mock('react-json-view', () => ({ default: ({ src }: { src: unknown }) => <pre>{JSON.stringify(src)}</pre> }));

afterEach(() => vi.unstubAllGlobals());

it('shows a combined status and list-supplied details without fetching individual jobs or stories', async () => {
  const job: RecipeJob = {
    tenant_id: 'tenant', job_id: 'failed-job', job_type: 'recipe', recipe: 'build',
    status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_system',
    completion_detail: 'Container failed: missing temporary directory',
    created_at: '2026-10-04T01:00:00Z', available_at: '2026-10-04T01:00:00Z',
    archived_at: '2026-10-04T02:00:00Z',
    parent: { job_id: 'parent-job', tenant_id: 'parent-tenant' },
    wait_for: ['dependency-job'],
    task_wait: { inputOrdinal: 0, outputOrdinal: 12, inputHash: 'hash', resumeJobType: 'recipe' },
    client_payload: { example: 'saved payload' },
    execution: { status: 'not_waiting', source: 'unavailable', published: false },
  };
  const fetch = vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost');
    if (url.pathname.endsWith('/cells')) return { ok: true, json: async () => [] };
    if (url.pathname.endsWith('/jobs')) return { ok: true, json: async () => ({ jobs: [job] }) };
    throw new Error(`Unexpected extra request: ${url}`);
  });
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(await screen.findByText('Failed (system)')).toBeVisible();
  expect(screen.getByText(job.completion_detail!)).toBeVisible();
  expect(screen.queryByText('COMPLETED')).not.toBeInTheDocument();
  const row = screen.getByRole('link', { name: 'failed-job' }).closest('tr')!;
  fireEvent.click(within(row).getByRole('button', { name: 'Expand row' }));
  expect(await screen.findByRole('link', { name: 'parent-job' })).toHaveAttribute('href', '/project/parent-tenant/jobs/parent-job/story');
  expect(screen.getByRole('link', { name: 'dependency-job' })).toHaveAttribute('href', '/project/tenant/jobs/dependency-job/story');
  expect(screen.getByText('Task input ordinal')).toBeVisible();
  expect(screen.getByText('0')).toBeVisible();
  expect(screen.getByText('Finished')).toBeVisible();
  fireEvent.click(screen.getByText('All job data'));
  expect(screen.getByText(/saved payload/)).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  const jobsURL = fetch.mock.calls.map(([url]) => new URL(url, 'http://localhost')).find(url => url.pathname.endsWith('/jobs'))!;
  expect(jobsURL.searchParams.getAll('status')).toEqual(['all']);
});

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}{location.search}</output>;
}

it.each(['', '?status=ACTIVE&status=READY&cell=platform'])('preserves list filters through story navigation: %s', async (query) => {
  const listReads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost');
    let data: unknown;
    if (url.pathname.endsWith('/story')) {
      data = { job_id: 'job', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'recipe build', status: 'succeeded', path: [] } };
    } else if (url.pathname.endsWith('/jobs')) {
      listReads.push(url);
      data = { jobs: [{ job_id: 'job', status: 'READY', recipe: 'build', store: 'ACTIVE' }] };
    } else data = [];
    return { ok: true, json: async () => data };
  }));
  render(<MemoryRouter initialEntries={[`/project/tenant/jobs${query}`]}>
    <Location />
    <Routes>
      <Route path="/project/:projectId/jobs" element={<JobsListPage projectId="tenant" />} />
      <Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} />
    </Routes>
  </MemoryRouter>);
  const link = await screen.findByRole('link', { name: 'job', exact: true });
  expect(link).toHaveAttribute('href', `/project/tenant/jobs/job/story${query}`);
  fireEvent.click(link);
  await screen.findByText('Run Summary');
  fireEvent.click(screen.getByRole('button', { name: /Back to Jobs/ }));
  await screen.findByRole('link', { name: 'job', exact: true });
  expect(screen.getByTestId('location')).toHaveTextContent(`/project/tenant/jobs${query}`);
  await waitFor(() => expect(listReads).toHaveLength(2));
  for (const url of listReads) {
    expect(url.searchParams.getAll('status')).toEqual(query ? ['ACTIVE', 'READY'] : ['all']);
    expect(url.searchParams.get('cell')).toBe(query ? 'platform' : null);
  }
});
