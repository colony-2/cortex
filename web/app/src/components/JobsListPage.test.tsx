import { StrictMode } from 'react';
import { getJob } from '@colony2/shared/api';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, within, waitFor } from '@testing-library/react';
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

beforeEach(() => { vi.mocked(getJob).mockReset().mockResolvedValue({ goal_id: 'goal', job_id: 'job', status: 'READY', store: 'ACTIVE' } as RecipeJob); });
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
    return new Response(JSON.stringify(path.endsWith('/cells') ? [] : { jobs }));
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
  vi.stubGlobal('fetch', vi.fn(async (input: string) => (new Response(JSON.stringify(input.includes('/cells') ? [] : { jobs: [{ job_id: 'same-id', status: 'READY', store: 'ACTIVE' }] })))));
  const { rerender } = render(<MemoryRouter><JobsListPage projectId="first-tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job same-id' }));
  expect(screen.getByText('1 selected')).toBeVisible();
  rerender(<MemoryRouter><JobsListPage projectId="other-tenant" /></MemoryRouter>);
  expect(await screen.findByRole('checkbox', { name: 'Select job same-id' })).not.toBeChecked();
  expect(screen.getByRole('button', { name: 'Delete selected' })).toBeDisabled();
});

it('shows restored fields immediately and inspects only after expansion', async () => {
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
    if (url.pathname.endsWith('/cells')) return new Response(JSON.stringify([]));
    if (url.pathname.endsWith('/jobs')) return new Response(JSON.stringify(({ jobs: [compact(job)] })));
    throw new Error(`Unexpected extra request: ${url}`);
  });
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  vi.mocked(getJob).mockResolvedValue(job);
  expect(await screen.findByText('Failed (system)')).toBeVisible();
  expect(screen.getByText(job.completion_detail!)).toBeVisible();
  expect(getJob).not.toHaveBeenCalled();
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
  expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'failed-job');
  const jobsURL = fetch.mock.calls.map(([url]) => new URL(url, 'http://localhost')).find(url => url.pathname.endsWith('/jobs'))!;
  expect(jobsURL.pathname).toBe('/api/projects/tenant/jobs');
});

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}{location.search}</output>;
}

it.each(['', '?status=ACTIVE&status=READY&cell=platform', '?status=success&status=failed_app&status=CANCEL_REQUESTED&cell=platform', '?status=READY&cell=platform&cell=app', '?q=search+term&sort=submitted&order=descend'])('preserves list filters through story navigation: %s', async (query) => {
  const listReads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost');
    let data: unknown;
    if (url.pathname.endsWith('/story')) {
      data = { job_id: 'job', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'recipe build', status: 'succeeded', path: [] } };
    } else if (url.pathname.endsWith('/jobs')) {
      listReads.push(url);
      data = { jobs: [{ job_id: 'job', prompt: 'Search term in the full prompt', status: 'READY', recipe: 'build', store: 'ACTIVE' }] };
    } else data = [];
    return new Response(JSON.stringify(data));
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
    const expected = new URLSearchParams(query);
    expect(url.searchParams.getAll('displayStatus')).toEqual(expected.getAll('status'));
    expect(url.searchParams.getAll('status')).toEqual(expected.has('status') ? [] : ['all']);
    expect(url.searchParams.getAll('cell')).toEqual(expected.getAll('cell'));
    expect(url.searchParams.has('q')).toBe(false);
    expect(url.searchParams.has('sort')).toBe(false);
  }
});

it('searches full prompts across attempts without fetching and clears hidden selections', async () => {
  const jobs = [
    { job_id: 'new', goal_id: 'grouped', title: 'Short title', prompt: `${'Long preamble '.repeat(12)}deep\n  needle`, status: 'READY', store: 'ACTIVE', created_at: '2026-10-29T12:00:00Z' },
    { job_id: 'old', goal_id: 'grouped', prompt: 'Earlier attempt contains an older requirement', status: 'COMPLETED', completion_status: 'failed_app', store: 'ARCHIVED', created_at: '2026-10-28T12:00:00Z' },
    { job_id: 'unrelated', prompt: 'Different work', status: 'READY', store: 'ACTIVE' },
    { job_id: 'missing', status: 'READY', store: 'ACTIVE' },
  ];
  const fetch = vi.fn(async (url: string) => new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs })));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><Navigation /><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job unrelated' }));
  const search = screen.getByRole('textbox', { name: 'Search goals' });
  fireEvent.change(search, { target: { value: ' DEEP needle ' } });
  expect(screen.getByRole('link', { name: 'Short title', exact: true })).toBeVisible();
  expect(screen.getByText('2 attempts on this page')).toBeVisible();
  expect(screen.queryByRole('link', { name: 'Different work', exact: true })).not.toBeInTheDocument();
  expect(screen.getByText('0 selected')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Delete selected' })).toBeDisabled();
  fireEvent.change(search, { target: { value: 'older requirement' } });
  expect(screen.getByRole('link', { name: 'Short title', exact: true })).toBeVisible();
  fireEvent.change(search, { target: { value: 'not present' } });
  expect(screen.getByText('No goals match this search on this page')).toBeVisible();
  fireEvent.change(search, { target: { value: '' } });
  expect(screen.getByRole('link', { name: 'Different work', exact: true })).toBeVisible();
  expect(screen.getByRole('link', { name: 'Untitled goal', exact: true })).toBeVisible();
  // Sorting pushes a history entry; changing search replaces only that entry.
  // Going back must also clear selection, even though the input did not fire.
  fireEvent.click(screen.getByRole('columnheader', { name: /^Goal/ }));
  fireEvent.change(search, { target: { value: 'Different work' } });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job unrelated' }));
  expect(screen.getByText('1 selected')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Browser back' }));
  expect(search).toHaveValue('');
  expect(screen.getByText('0 selected')).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(getJob).not.toHaveBeenCalled();
});

it('sorts goal, status, cell, and submission columns locally in both directions', async () => {
  const jobs = [
    { job_id: 'z', title: 'Zebra', status: 'READY', store: 'ACTIVE', cell_name: 'alpha', submitted_at: '2026-10-29T08:00:00Z', created_at: '2026-10-30T00:00:00Z' },
    { job_id: 'a', title: 'Alpha', status: 'COMPLETED', completion_status: 'success', store: 'ARCHIVED', cell_name: 'zeta', created_at: '2026-10-29T09:00:00Z' },
    { job_id: 'm', title: 'Middle', status: 'COMPLETED', completion_status: 'failed_app', store: 'ARCHIVED', cell_name: 'middle', created_at: '2026-10-29T10:00:00Z' },
  ];
  const fetch = vi.fn(async (url: string) => new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs })));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  await screen.findByRole('link', { name: 'Zebra', exact: true });
  const names = () => Array.from(document.querySelectorAll('a.goal-prompt')).map(link => link.textContent);
  for (const [column, ascending] of [
    ['Goal', ['Alpha', 'Middle', 'Zebra']], ['Status', ['Middle', 'Zebra', 'Alpha']],
    ['Cell', ['Zebra', 'Middle', 'Alpha']], ['Submitted', ['Zebra', 'Alpha', 'Middle']],
  ] as const) {
    const header = screen.getByRole('columnheader', { name: new RegExp(`^${column}`) });
    fireEvent.click(header);
    expect(names()).toEqual(ascending);
    expect(header).toHaveAttribute('aria-sort', 'ascending');
    fireEvent.click(header);
    expect(names()).toEqual([...ascending].reverse());
    expect(header).toHaveAttribute('aria-sort', 'descending');
  }
  expect(screen.getByTestId('location')).toHaveTextContent('sort=submitted&order=descend');
  fireEvent.click(screen.getByRole('columnheader', { name: /^Submitted/ }));
  expect(names()).toEqual(['Zebra', 'Alpha', 'Middle']);
  expect(screen.getByTestId('location').textContent).not.toContain('sort=');
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(getJob).not.toHaveBeenCalled();
});

it('starts status checkboxes selected and distinguishes selecting none from selecting all', async () => {
  const reads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input);
    if (url.pathname.endsWith('/cells')) return new Response('[]');
    reads.push(url);
    return new Response(JSON.stringify({ jobs: [{ job_id: 'job', status: 'READY', store: 'ACTIVE' }] }));
  }));
  render(<MemoryRouter><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  await screen.findByRole('link', { name: 'Job job: Ready', exact: true });
  fireEvent.click(screen.getByRole('button', { name: 'Filter statuses' }));
  const dropdown = () => within(document.querySelector('.ant-table-filter-dropdown') as HTMLElement);
  expect(dropdown().getByRole('checkbox', { name: 'Select all', exact: true })).toBeChecked();
  fireEvent.click(screen.getByRole('button', { name: 'Select none', exact: true }));
  fireEvent.click(screen.getByRole('button', { name: 'Apply', exact: true }));
  expect(await screen.findByText('No statuses selected')).toBeVisible();
  expect(screen.getByTestId('location')).toHaveTextContent('status=none');
  expect(reads).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: 'Filter statuses' }));
  fireEvent.click(dropdown().getByRole('checkbox', { name: 'Select all', exact: true }));
  fireEvent.click(screen.getByRole('button', { name: 'Apply', exact: true }));
  expect(await screen.findByRole('link', { name: 'Job job: Ready', exact: true })).toBeVisible();
  expect(reads).toHaveLength(2);
  expect(reads[1].searchParams.get('status')).toBe('all');
  expect(screen.getByTestId('location').textContent).not.toContain('status=');
});

function Navigation() {
  const navigate = useNavigate();
  return <><button onClick={() => navigate(-1)}>Browser back</button><button onClick={() => navigate(1)}>Browser forward</button></>;
}
function GoalRoute() {
  return <JobsListPage projectId="tenant" goalId={useParams().goalId} />;
}

it('groups attempts and restores expanded details through goal navigation and browser back/forward with selected detail queries', async () => {
  const jobs: Partial<RecipeJob>[] = [
    { job_id: 'first', goal_id: 'goal-a', prompt: 'Build a useful feature', status: 'READY', store: 'ACTIVE', cell_name: 'platform', execution: { status: 'specified', source: 'yield', published: true, demand: { effective: { cpu: 4 } } } },
    { job_id: 'second', goal_id: 'goal-a', prompt: 'Build a useful feature', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_app' },
    { job_id: 'single', goal_id: 'goal-b', status: 'READY', store: 'ACTIVE' },
  ];
  const fetch = vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost');
    const path = url.pathname;
    if (path.endsWith('/cells')) return new Response(JSON.stringify([]));
    if (path.endsWith('/jobs')) return new Response(JSON.stringify(({ jobs: url.searchParams.get('goal') === 'goal-a' ? jobs.slice(0, 2).map(compact) : jobs.map(compact) })));
    throw new Error(`Unexpected detail query: ${path}`);
  });
  vi.stubGlobal('fetch', fetch);
  vi.mocked(getJob).mockImplementation(async (_, id) => jobs.find(job => job.job_id === id) as RecipeJob);
  render(<MemoryRouter initialEntries={['/project/tenant/goals']}><Navigation /><Location /><Routes>
    <Route path="/project/tenant/goals" element={<JobsListPage projectId="tenant" />} />
    <Route path="/project/tenant/goals/:goalId" element={<GoalRoute />} />
  </Routes></MemoryRouter>);
  const first = (await screen.findByRole('link', { name: 'Build a useful feature', exact: true })).closest('tr')!;
  expect(screen.getAllByRole('link', { name: 'Build a useful feature' })).toHaveLength(1);
  expect(screen.queryByText('goal-b')).not.toBeInTheDocument();
  fireEvent.click(within(first).getByRole('button', { name: 'Expand row' }));
  fireEvent.click(await screen.findByText('Execution requirements'));
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

it('uses compact wait indicators until dependency targets are inspected', async () => {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [
    { job_id: 'review', goal_id: 'g1', status: 'READY', store: 'ACTIVE', next_route: { jobType: 'recipe', taskType: 'input:collect_user_input' } },
    { job_id: 'waiting', goal_id: 'g2', status: 'PENDING_JOBS', store: 'ACTIVE', blocking_wait_summary: { waitId: 'wait' } },
  ] })))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(await screen.findByRole('link', { name: 'Open input/review' })).toHaveAttribute('href', '/project/tenant/reviews/review');
  expect(screen.getByText('Waiting for dependencies')).toBeVisible();
  expect(getJob).not.toHaveBeenCalled();
  expect(screen.queryByRole('link', { name: 'Execution 3' })).not.toBeInTheDocument();
  vi.mocked(getJob).mockResolvedValue({ job_id: 'waiting', status: 'PENDING_JOBS', store: 'ACTIVE', blocking_wait: {
    waitId: 'wait', state: 'PENDING', dependencies: [
      { target: { goalId: 'target-goal' }, state: 'PENDING', condition: 'success' },
      { target: { jobId: 'target-job' }, state: 'PENDING', condition: 'complete' },
    ],
  } } as RecipeJob);
  const row = screen.getByRole('link', { name: 'Job waiting: Waiting for jobs' }).closest('tr')!;
  fireEvent.click(within(row).getByRole('button', { name: 'Expand row' }));
  expect(await screen.findByRole('link', { name: 'Goal 1' })).toHaveAttribute('href', '/project/tenant/goals/target-goal');
  expect(screen.getByRole('link', { name: 'Execution 2' })).toHaveAttribute('href', '/project/tenant/jobs/target-job/story');
  expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'waiting');
});

it('submits a prompt-only goal and opens the returned goal identity', async () => {
  const fetch = vi.fn(async (_: string, init?: RequestInit) => (new Response(JSON.stringify(init?.method === 'POST'
    ? { tenant_id: 'tenant', goal_id: 'created-goal', job_id: 'created-job' } : _.includes('/cells') ? [] : { jobs: [] }))));
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
  vi.stubGlobal('fetch', vi.fn(async (input: string) => (new Response(JSON.stringify(input.includes('/cells') ? [{ name: 'configured-cell', kind: 'self' }] : { jobs })))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  const row = (await screen.findByRole('link', { name: 'Goal prompt', exact: true })).closest('tr')!;
  const label = cell_name?.trim() ? cell_name : 'Unknown cell';
  expect(within(row).getByText(label, { exact: true })).toBeVisible();
  fireEvent.click(within(row).getByRole('button', { name: 'Expand row' }));
  const attempts = await screen.findByRole('region', { name: 'Attempts' });
  expect(within(attempts).getAllByText(label, { exact: true })).toHaveLength(2);
});

function compact(job: Partial<RecipeJob>) {
  const { blocking_wait, other_goal_ids, ...summary } = job;
  return { ...summary, client_payload_revision: 7 };
}

it('renders escaped multiline completion and ordered prerequisites from the list without inspection', async () => {
  const completion = '  <img src=x onerror=alert(1)> 雪\n\tsecond line & more\n';
  vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [
    { job_id: 'failed', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'future_outcome', completion_detail: completion },
    { job_id: 'retrying', status: 'READY', store: 'ACTIVE' },
    { job_id: 'waiting', status: 'PENDING_JOBS', store: 'ACTIVE', wait_for: ['second/id', 'first'], blocking_wait_summary: { waitId: 'wait', deadline: '2027-01-01T00:00:00Z' } },
  ] }))));
  render(<MemoryRouter><JobsListPage projectId="tenant /雪" /></MemoryRouter>);
  const row = (await screen.findByRole('link', { name: 'Job failed: Finished (future_outcome)' })).closest('tr')!;
  expect(row.querySelector('.job-completion')?.textContent).toBe(completion);
  expect((row.querySelector('.job-completion') as HTMLElement).style.whiteSpace).toBe('pre-wrap');
  expect(row.querySelector('img')).toBeNull();
  const waiting = screen.getByRole('link', { name: 'Job waiting: Waiting for jobs' }).closest('tr')!;
  expect(within(waiting).getAllByRole('link').filter(link => link.textContent?.startsWith('Execution')).map(link => link.getAttribute('href'))).toEqual([
    '/project/tenant%20%2F%E9%9B%AA/jobs/second%2Fid/story', '/project/tenant%20%2F%E9%9B%AA/jobs/first/story',
  ]);
  expect(screen.getByRole('link', { name: 'Job retrying: Ready' })).toBeVisible();
  expect(getJob).not.toHaveBeenCalled();
});

it('replaces requirements on refresh and never shows stale demand or diagnostics for active or terminal jobs', async () => {
  const job = { job_id: 'job', status: 'READY', store: 'ACTIVE', execution: {
    status: 'specified', source: 'yield', published: true, demand: { effective: { resources: { cpu: '4', memory: '4Gi' }, image: 'example/image' } },
  } };
  const fetch = vi.fn(async (url: string) => new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [job] })));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(await screen.findByText(/4Gi/)).toBeVisible();
  for (const [status, label] of [
    ['unspecified', 'No requirements specified'], ['unresolved', 'Requirements not yet resolved'],
    ['malformed', 'Invalid execution requirements'], ['unsupported', 'Unsupported execution requirements'],
    ['in_flight', undefined], ['not_waiting', undefined],
  ]) {
    job.execution = { ...job.execution, status: status!, diagnostic: 'invalid demand 雪' } as typeof job.execution;
    job.status = status === 'in_flight' ? 'ACTIVE' : status === 'not_waiting' ? 'COMPLETED' : 'READY';
    job.store = status === 'not_waiting' ? 'ARCHIVED' : 'ACTIVE';
    const before = fetch.mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(fetch.mock.calls.length).toBe(before + 2));
    if (label) expect(await screen.findByText(label)).toBeVisible();
    else await waitFor(() => expect(screen.queryByText('invalid demand 雪')).not.toBeInTheDocument());
    expect(screen.queryByText(/4Gi/)).not.toBeInTheDocument();
  }
  expect(getJob).not.toHaveBeenCalled();
});

it('shows restored selected fields even while inspection is unavailable', async () => {
  const raw = `{"jobs":[{"job_id":"job","status":"READY","store":"ACTIVE","client_payload":{"other":9007199254740993},"client_payload_revision":7,"task_wait":{"inputOrdinal":0,"outputOrdinal":9007199254740995,"inputHash":"occurrence-hash","resumeJobType":"recipe"},"blocking_wait_summary":{"waitId":"wait-identity","deadline":"2027-01-01T00:00:00Z"}}]}`;
  vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(url.includes('/cells') ? '[]' : raw)));
  vi.mocked(getJob).mockRejectedValue(new Error('Inspection unavailable'));
  render(<MemoryRouter initialEntries={['/?expanded=job:job']}><JobsListPage projectId="tenant" /></MemoryRouter>);
  await screen.findByText('Could not load job details');
  fireEvent.click(screen.getByText('Technical details'));
  expect(screen.getByText('occurrence-hash')).toBeVisible();
  expect(screen.getByText('9007199254740995')).toBeVisible();
  expect(screen.getByText('wait-identity')).toBeVisible();
  fireEvent.click(screen.getByText('All job data'));
  expect(screen.getByText(/"other": 9007199254740993/)).toBeVisible();
  expect(screen.queryByText(/9007199254740992/)).not.toBeInTheDocument();
});

it('inspects only visible selections, revalidates on refresh/reopen, and ignores late attempts', async () => {
  const jobs = ['first', 'second'].map(job_id => ({ tenant_id: 'tenant', job_id, goal_id: 'goal',
    prompt: 'Grouped goal', status: 'READY', store: 'ACTIVE', client_payload_revision: 9 }));
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs })))));
  let resolveFirst!: (job: RecipeJob) => void;
  vi.mocked(getJob).mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve; }))
    .mockImplementation(async (tenant, id) => ({ tenant_id: tenant, job_id: id, other_goal_ids: [`${id} inspected`] } as RecipeJob));
  render(<StrictMode><MemoryRouter initialEntries={['/?expanded=goal:goal']}><JobsListPage projectId="tenant" /></MemoryRouter></StrictMode>);
  const attempts = await screen.findByRole('region', { name: 'Attempts' });
  await waitFor(() => expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'first'));
  fireEvent.click(within(attempts).getAllByRole('button', { name: 'Details' })[1]);
  expect((await screen.findAllByText('second inspected'))[0]).toBeVisible();
  await act(async () => resolveFirst({ job_id: 'first', other_goal_ids: ['LATE first'] } as RecipeJob));
  expect(screen.queryByText('LATE first')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(3));
  expect(getJob).toHaveBeenLastCalledWith('tenant', 'second');
  fireEvent.click(screen.getByRole('button', { name: 'Collapse row' }));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Expand row' })).toBeEnabled());
  expect(getJob).toHaveBeenCalledTimes(3);
  fireEvent.click(screen.getByRole('button', { name: 'Expand row' }));
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(4));
  expect(getJob).toHaveBeenLastCalledWith('tenant', 'second');
});

it('retries failed inspection and never reuses another tenant’s details', async () => {
  const job = { job_id: 'same', goal_id: 'goal', prompt: 'Same ID', status: 'READY', store: 'ACTIVE' };
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [job] })))));
  vi.mocked(getJob).mockRejectedValueOnce(new Error('Unavailable'))
    .mockImplementation(async (tenant) => ({ ...job, other_goal_ids: [`${tenant} details`] } as RecipeJob));
  const view = (tenant: string) => <MemoryRouter initialEntries={['/?expanded=goal:goal']}><JobsListPage projectId={tenant} /></MemoryRouter>;
  const { rerender } = render(view('first'));
  fireEvent.click(await screen.findByRole('button', { name: 'Retry details' }));
  expect((await screen.findAllByText('first details'))[0]).toBeVisible();
  rerender(view('second'));
  expect(screen.queryByText('first details')).not.toBeInTheDocument();
  expect((await screen.findAllByText('second details'))[0]).toBeVisible();
  expect(getJob).toHaveBeenLastCalledWith('second', 'same');
});

it('keeps 200 collapsed summaries compact and follows an empty page cursor', async () => {
  const jobs = Array.from({ length: 200 }, (_, index) => ({ job_id: `job-${index}`, goal_id: `goal-${index}`,
    status: 'PENDING_JOBS', store: 'ACTIVE', client_payload_revision: 5, blocking_wait_summary: { waitId: `wait-${index}` } }));
  const listReads: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (url.includes('/cells')) return new Response(JSON.stringify([]));
    listReads.push(url);
    return new Response(JSON.stringify(url.includes('pageToken=empty') ? { jobs: [], next_page_token: 'last' }
      : url.includes('pageToken=last') ? { jobs: [] } : { jobs, next_page_token: 'empty' }));
  }));
  render(<StrictMode><MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter></StrictMode>);
  await screen.findByRole('checkbox', { name: 'Select job job-199' });
  expect(listReads).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: 'Next jobs' }));
  await waitFor(() => expect(screen.queryByRole('checkbox', { name: 'Select job job-199' })).toBeNull());
  fireEvent.click(screen.getByRole('button', { name: 'Next jobs' }));
  await waitFor(() => expect(listReads).toHaveLength(3));
  expect(listReads[2]).toContain('pageToken=last');
  expect(getJob).not.toHaveBeenCalled();
});

it('groups only primary membership while honoring a server-filtered secondary goal', async () => {
  // Compatibility fields from an older backend must not restore secondary groups.
  const job = { job_id: 'member', goal_id: 'primary', other_goal_ids: ['secondary'], prompt: 'Member', status: 'READY', store: 'ACTIVE' };
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [job] })))));
  const { rerender } = render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  await screen.findByRole('link', { name: 'Member', exact: true });
  expect(screen.getAllByRole('link', { name: 'Goal overview' })).toHaveLength(1);
  expect(screen.getByRole('link', { name: 'Goal overview' })).toHaveAttribute('href', '/project/tenant/goals/primary');
  expect(getJob).not.toHaveBeenCalled();
  rerender(<MemoryRouter><JobsListPage projectId="tenant" goalId="secondary" /></MemoryRouter>);
  await waitFor(() => expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'member'));
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes('goal=secondary'))).toBe(true);
});
