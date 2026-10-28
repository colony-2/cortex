import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation, useNavigate, useParams } from 'react-router-dom';
import type { RecipeJob } from '@colony2/shared/types';
import JobsListPage from './JobsListPage';
import JobStoryPage from './JobStoryPage';

vi.mock('@colony2/shared', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  useInputActivity: () => ({ pendingInputs: [], refresh: vi.fn() }),
}));
vi.mock('@colony2/shared/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared/api')>(),
  getJob: vi.fn().mockResolvedValue({ goal_id: 'goal' }),
}));
vi.mock('react-json-view', () => ({ default: ({ src }: { src: unknown }) => <pre>{JSON.stringify(src)}</pre> }));

afterEach(() => vi.unstubAllGlobals());

it.each(['cancel', 'delete'] as const)('confirms selected jobs and keeps failed %s operations available for retry', async action => {
  const jobs = ['first', 'second', 'unselected'].map(job_id => ({ job_id, status: 'READY', store: 'ACTIVE' }));
  const changes: string[] = [];
  let failSecond = true;
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const path = new URL(input, 'http://localhost').pathname;
    if (init?.method === 'POST' || init?.method === 'DELETE') {
      changes.push(path);
      if (path.includes('/second') && failSecond) return { ok: false, status: 503, statusText: 'Unavailable', text: async () => 'Please retry' };
      return { ok: true, status: 204 };
    }
    return { ok: true, json: async () => path.endsWith('/cells') ? [] : { jobs } };
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job first' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job second' }));
  const verb = action === 'cancel' ? 'Cancel' : 'Delete';
  fireEvent.click(screen.getByRole('button', { name: `${verb} selected` }));
  let dialog = screen.getByRole('dialog');
  expect(changes).toEqual([]);
  expect(within(dialog).queryByText('unselected')).not.toBeInTheDocument();
  fireEvent.click(within(dialog).getByRole('button', { name: 'Keep jobs' }));
  expect(changes).toEqual([]);
  fireEvent.click(screen.getByRole('button', { name: `${verb} selected` }));
  dialog = screen.getByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: `${verb} 2 jobs` }));
  expect(await within(dialog).findByText('1 job operation failed')).toBeVisible();
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Select job first' })).not.toBeChecked());
  expect(screen.getByRole('checkbox', { name: 'Select job second' })).toBeChecked();
  const suffix = action === 'cancel' ? '/cancel' : '';
  expect(changes).toEqual([`/api/projects/tenant/jobs/first${suffix}`, `/api/projects/tenant/jobs/second${suffix}`]);
  failSecond = false;
  fireEvent.click(within(dialog).getByRole('button', { name: `${verb} job`, exact: true }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(changes.at(-1)).toBe(`/api/projects/tenant/jobs/second${suffix}`);
  expect(changes).toHaveLength(3);
  expect(screen.getByText('0 selected')).toBeVisible();
});

it('clears job selections when switching tenants', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string) => ({ ok: true, json: async () =>
    input.includes('/cells') ? [] : { jobs: [{ job_id: 'same-id', status: 'READY', store: 'ACTIVE' }] },
  })));
  const { rerender } = render(<MemoryRouter><JobsListPage projectId="first-tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job same-id' }));
  expect(screen.getByText('1 selected')).toBeVisible();
  rerender(<MemoryRouter><JobsListPage projectId="other-tenant" /></MemoryRouter>);
  expect(await screen.findByRole('checkbox', { name: 'Select job same-id' })).not.toBeChecked();
  expect(screen.getByRole('button', { name: 'Delete selected' })).toBeDisabled();
});

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
  const row = screen.getByRole('link', { name: 'Job failed-job: Failed (system)' }).closest('tr')!;
  fireEvent.click(within(row).getByRole('button', { name: 'Expand row' }));
  expect(await screen.findByRole('link', { name: 'View parent execution' })).toHaveAttribute('href', '/project/parent-tenant/jobs/parent-job/story');
  expect(screen.getByRole('link', { name: 'Dependency 1' })).toHaveAttribute('href', '/project/tenant/jobs/dependency-job/story?expanded=job%3Afailed-job');
  fireEvent.click(screen.getByText('Technical details'));
  expect(screen.getByText('Task input ordinal')).toBeVisible();
  expect(screen.getByText('0')).toBeVisible();
  expect(screen.getByText('Finished')).toBeVisible();
  fireEvent.click(screen.getByText('All job data'));
  expect(screen.getByText(/saved payload/)).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  const jobsURL = fetch.mock.calls.map(([url]) => new URL(url, 'http://localhost')).find(url => url.pathname.endsWith('/jobs'))!;
  expect(jobsURL.pathname).toBe('/api/projects/tenant/jobs');
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
  const link = await screen.findByRole('link', { name: 'Job job: Ready', exact: true });
  expect(link).toHaveAttribute('href', `/project/tenant/jobs/job/story${query}`);
  fireEvent.click(link);
  await screen.findByText('Run Summary');
  fireEvent.click(screen.getByRole('button', { name: /Back to Goals/ }));
  await screen.findByRole('link', { name: 'Job job: Ready', exact: true });
  expect(screen.getByTestId('location')).toHaveTextContent(`/project/tenant/jobs${query}`);
  await waitFor(() => expect(listReads).toHaveLength(2));
  for (const url of listReads) {
    expect(url.searchParams.getAll('status')).toEqual(query ? ['ACTIVE', 'READY'] : ['all']);
    expect(url.searchParams.get('cell')).toBe(query ? 'platform' : null);
  }
});

function Navigation() {
  const navigate = useNavigate();
  return <><button onClick={() => navigate(-1)}>Browser back</button><button onClick={() => navigate(1)}>Browser forward</button></>;
}
function GoalRoute() {
  return <JobsListPage projectId="tenant" goalId={useParams().goalId} />;
}

it('groups attempts and restores expanded details through goal navigation and browser back/forward without detail queries', async () => {
  const jobs: Partial<RecipeJob>[] = [
    { job_id: 'first', goal_id: 'goal-a', prompt: 'Build a useful feature', status: 'READY', store: 'ACTIVE', cell_name: 'platform', execution: { status: 'published', source: 'job', published: true, demand: { cpu: 4 } } },
    { job_id: 'second', goal_id: 'goal-a', prompt: 'Build a useful feature', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_app' },
    { job_id: 'single', goal_id: 'goal-b', status: 'READY', store: 'ACTIVE' },
  ];
  const fetch = vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost');
    const path = url.pathname;
    if (path.endsWith('/cells')) return { ok: true, json: async () => [] };
    if (path.endsWith('/jobs')) return { ok: true, json: async () => ({ jobs: url.searchParams.get('goal') === 'goal-a' ? jobs.slice(0, 2) : jobs }) };
    throw new Error(`Unexpected detail query: ${path}`);
  });
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter initialEntries={['/project/tenant/goals']}><Navigation /><Location /><Routes>
    <Route path="/project/tenant/goals" element={<JobsListPage projectId="tenant" />} />
    <Route path="/project/tenant/goals/:goalId" element={<GoalRoute />} />
  </Routes></MemoryRouter>);
  const first = (await screen.findByRole('link', { name: 'Build a useful feature', exact: true })).closest('tr')!;
  expect(screen.getAllByRole('link', { name: 'Build a useful feature' })).toHaveLength(1);
  expect(screen.queryByText('goal-b')).not.toBeInTheDocument();
  fireEvent.click(within(first).getByRole('button', { name: 'Expand row' }));
  fireEvent.click(screen.getByText('Execution requirements'));
  expect(within(screen.getByText('Execution requirements').closest('details')!).getByText(/"cpu": 4/)).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  const originalURL = screen.getByTestId('location').textContent;
  fireEvent.click(within(first).getByRole('link', { name: 'Goal overview' }));
  await screen.findByRole('heading', { name: 'Goal overview', exact: true });
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Expand row' })).not.toBeInTheDocument();
  expect(await screen.findByText('Execution requirements')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Browser back' }));
  expect(await screen.findByText('Execution requirements')).toBeVisible();
  expect(within(screen.getByText('Execution requirements').closest('details')!).getByText(/"cpu": 4/)).toBeVisible();
  expect(screen.getByTestId('location').textContent).toBe(originalURL);
  fireEvent.click(screen.getByRole('button', { name: 'Browser forward' }));
  await screen.findByRole('heading', { name: 'Goal overview', exact: true });
  expect(await screen.findByText('Execution requirements')).toBeVisible();

});

it('uses waiting routes and dependency targets for input/review, job, and goal links', async () => {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => ({ ok: true, json: async () => url.includes('/cells') ? [] : { jobs: [
    { job_id: 'review', goal_id: 'g1', status: 'READY', store: 'ACTIVE', next_route: { jobType: 'recipe', taskType: 'input:collect_user_input' } },
    { job_id: 'waiting', goal_id: 'g2', status: 'PENDING_JOBS', store: 'ACTIVE', blocking_wait: { waitId: 'wait', state: 'PENDING', dependencies: [
      { target: { goalId: 'target-goal' }, state: 'PENDING', condition: 'success' },
      { target: { jobId: 'target-job' }, state: 'PENDING', condition: 'complete' },
      { target: { jobId: 'done-job' }, state: 'SATISFIED', condition: 'complete' },
    ] } },
  ] } })));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(await screen.findByRole('link', { name: 'Open input/review' })).toHaveAttribute('href', '/project/tenant/inputs/review');
  expect(screen.getByRole('link', { name: 'Goal 1' })).toHaveAttribute('href', '/project/tenant/goals/target-goal');
  expect(screen.getByRole('link', { name: 'Execution 2' })).toHaveAttribute('href', '/project/tenant/jobs/target-job/story');
  expect(screen.queryByRole('link', { name: 'Execution 3' })).not.toBeInTheDocument();
});

it('submits a prompt-only goal and opens the returned goal identity', async () => {
  const fetch = vi.fn(async (_: string, init?: RequestInit) => ({ ok: true, json: async () => init?.method === 'POST'
    ? { tenant_id: 'tenant', goal_id: 'created-goal', job_id: 'created-job' } : _.includes('/cells') ? [] : { jobs: [] } }));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(screen.getByRole('button', { name: 'New goal' }));
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).queryByLabelText('Recipe')).not.toBeInTheDocument();
  expect(within(dialog).queryByLabelText('Job ID')).not.toBeInTheDocument();
  fireEvent.change(within(dialog).getByLabelText('Prompt'), { target: { value: 'Build a small tool' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/project/tenant/goals/created-goal'));
  const call = fetch.mock.calls.find(([, init]) => init?.method === 'POST')!;
  expect(JSON.parse(call[1]!.body as string)).toEqual({ inputs: { prompt: 'Build a small tool', type: 'build' } });
});

it.each([undefined, '', ' \n ', 'self', 'platform'])('uses explicit cell identity or Unknown cell in list and attempts (%j)', async cell_name => {
  const jobs = ['first', 'second'].map(job_id => ({ job_id, goal_id: 'goal', prompt: 'Goal prompt', cell_name,
    status: 'READY', store: 'ACTIVE', repo: 'https://example.test/not-a-cell', parent: { cell_name: 'parent-cell' } }));
  vi.stubGlobal('fetch', vi.fn(async (input: string) => ({ ok: true, json: async () => input.includes('/cells') ? [{ name: 'configured-cell', kind: 'self' }] : { jobs } })));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  const row = (await screen.findByRole('link', { name: 'Goal prompt', exact: true })).closest('tr')!;
  const label = cell_name?.trim() ? cell_name : 'Unknown cell';
  expect(within(row).getByText(label, { exact: true })).toBeVisible();
  fireEvent.click(within(row).getByRole('button', { name: 'Expand row' }));
  const attempts = await screen.findByRole('region', { name: 'Attempts' });
  expect(within(attempts).getAllByText(label, { exact: true })).toHaveLength(2);
});
