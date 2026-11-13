import { StrictMode } from 'react';
import { jobIdentity, registryCell } from '../test/cellFixtures';
import { getCell, listCells, getJob } from '@colony2/shared/api';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, within, waitFor } from '@testing-library/react';
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
  listCells: vi.fn(), getCell: vi.fn(),
  getJob: vi.fn().mockResolvedValue({ ...jobIdentity, goal_id: 'goal' }),
}));
vi.mock('react-json-view', () => ({ default: ({ src }: { src: unknown }) => <pre>{JSON.stringify(src)}</pre> }));

function BrowserBack() { const navigate = useNavigate(); return <button onClick={() => navigate(-1)}>Browser back</button>; }
beforeEach(() => { vi.mocked(listCells).mockResolvedValue({ cells: ['platform', 'app', 'alpha', 'zeta', 'middle'].map(name => registryCell(name)) });
 vi.mocked(getCell).mockImplementation(async (tenant, id) => registryCell(id.replace(/^cell-/, ''), tenant)); vi.mocked(getJob).mockReset().mockResolvedValue({ ...jobIdentity, goal_id: 'goal', job_id: 'job', status: 'READY', store: 'ACTIVE' } as RecipeJob); });
afterEach(async () => {
  // These tests mount portals and start asynchronous list/detail loads. Unmount
  // inside act so React drains queued commits before jsdom and fetch are removed.
  await act(async () => { cleanup(); });
  vi.unstubAllGlobals();
});

it.each([['cancel', ''], ['archive', ''], ['cancel', '?scope=roots'], ['archive', '?scope=roots']] as const)('confirms selected jobs and keeps failed %s operations available for retry in %s', async (action, query) => {
  const jobs = ['first', 'second', 'unselected'].map(job_id => ({ ...jobIdentity, job_id, status: 'READY', store: 'ACTIVE' }));
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
  render(<MemoryRouter initialEntries={[`/project/tenant/goals${query}`]}><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job first' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job second' }));
  const verb = action === 'cancel' ? 'Cancel' : 'Archive';
  fireEvent.click(screen.getByRole('button', { name: `${verb} selected` }));
  const dialog = screen.getByRole('dialog');
  expect(changes).toEqual([]);
  expect(within(dialog).queryByLabelText('Cancellation message (optional)')).not.toBeInTheDocument();
  expect(within(dialog).queryByText('unselected')).not.toBeInTheDocument();
  fireEvent.click(within(dialog).getByRole('button', { name: `${verb} 2 jobs` }));
  expect(await within(dialog).findByText('1 job operation failed')).toBeVisible();
  expect(within(dialog).queryByLabelText('Cancellation message (optional)')).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Select job first' })).not.toBeChecked());
  expect(screen.getByRole('checkbox', { name: 'Select job second' })).toBeChecked();
  const suffix = action === 'cancel' ? '/cancel' : '';
  expect(changes).toEqual([`/api/projects/tenant/jobs/first${suffix}`, `/api/projects/tenant/jobs/second${suffix}`]);
  failSecond = false;
  const retry = within(dialog).getByRole('button', { name: `${verb} job`, exact: true });
  await waitFor(() => {
    expect(retry).toBeEnabled();
    expect(retry).not.toHaveClass('ant-btn-loading');
  });
  fireEvent.click(retry);
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(changes.at(-1)).toBe(`/api/projects/tenant/jobs/second${suffix}`);
  expect(changes).toHaveLength(3);
  expect(screen.getByText('0 selected')).toBeVisible();
});

// Keep dismissal separate from partial-failure retries: both behaviors remain
// covered without combining reopening and partial retries in one timed test.
it.each(['cancel', 'archive'] as const)('keeps selected jobs without mutation when dismissing %s confirmation', async action => {
  const jobs = ['first', 'second', 'unselected'].map(job_id => ({ ...jobIdentity, job_id, status: 'READY', store: 'ACTIVE' }));
  const changes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'POST' || init?.method === 'DELETE') {
      changes.push(new URL(input, 'http://localhost').pathname);
      return new Response(null, { status: 204 });
    }
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job first' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job second' }));
  const verb = action === 'cancel' ? 'Cancel' : 'Archive';
  const trigger = screen.getByRole('button', { name: `${verb} selected` });
  trigger.focus();
  fireEvent.click(trigger);
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).queryByText('unselected')).not.toBeInTheDocument();
  expect(changes).toEqual([]);
  fireEvent.click(within(dialog).getByRole('button', { name: 'Keep jobs' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(changes).toEqual([]);
  expect(screen.getByRole('checkbox', { name: 'Select job first' })).toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Select job second' })).toBeChecked();
  expect(trigger).toHaveFocus();
  fireEvent.click(trigger);
  const reopened = screen.getByRole('dialog');
  expect(reopened).toBeVisible();
  expect(changes).toEqual([]);
  fireEvent.click(within(reopened).getByRole('button', { name: `${verb} 2 jobs`, exact: true }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  const suffix = action === 'cancel' ? '/cancel' : '';
  expect(changes).toEqual([`/api/projects/tenant/jobs/first${suffix}`, `/api/projects/tenant/jobs/second${suffix}`]);
  expect(screen.getByText('0 selected')).toBeVisible();
});

it('clears job selections when switching tenants', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string) => (new Response(JSON.stringify(input.includes('/cells') ? [] : { jobs: [{ ...jobIdentity, job_id: 'same-id', status: 'READY', store: 'ACTIVE' }] })))));
  const { rerender } = render(<MemoryRouter><JobsListPage projectId="first-tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job same-id' }));
  expect(screen.getByText('1 selected')).toBeVisible();
  rerender(<MemoryRouter><JobsListPage projectId="other-tenant" /></MemoryRouter>);
  expect(await screen.findByRole('checkbox', { name: 'Select job same-id' })).not.toBeChecked();
  expect(screen.getByRole('button', { name: 'Archive selected' })).toBeDisabled();
});

it('shows restored fields immediately and inspects only after expansion', async () => {
  const job: RecipeJob = {
    ...jobIdentity, tenant_id: 'tenant', job_id: 'failed-job', job_type: 'recipe', recipe: 'build',
    status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_system',
    completion_detail: 'Container failed: missing temporary directory',
    created_at: '2026-10-04T01:00:00Z', available_at: '2026-10-04T01:00:00Z',
    archived_at: '2026-10-04T02:00:00Z',
    parent: { ...jobIdentity, job_id: 'parent-job', tenant_id: 'parent-tenant' },
    wait_for: ['dependency-job'],
    task_wait: { inputOrdinal: 0, outputOrdinal: 12, inputHash: 'hash', resumeJobType: 'recipe' },
    client_payload: { example: 'saved payload' },
    execution: { ...jobIdentity, status: 'not_waiting', source: 'unavailable', published: false },
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
  fireEvent.click(within(row).getByRole('button', { name: 'Expand goal details' }));
  expect(await screen.findByRole('link', { name: 'View parent execution' })).toHaveAttribute('href', '/project/parent-tenant/jobs/parent-job/story');
  expect(screen.getByRole('link', { name: 'Dependency 1' })).toHaveAttribute('href', '/project/tenant/jobs/dependency-job/story?executionState=cells-v1&expanded=job%3Afailed-job');
  fireEvent.click(screen.getByText('Technical details'));
  expect(screen.getByText('Task input ordinal')).toBeVisible();
  expect(screen.getByText('0')).toBeVisible();
  expect(screen.getByText('Finished')).toBeVisible();
  fireEvent.click(screen.getByText('All job data'));
  expect(screen.getByText(/saved payload/)).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'failed-job');
  const jobsURL = fetch.mock.calls.map(([url]) => new URL(url, 'http://localhost')).find(url => url.pathname.endsWith('/jobs'))!;
  expect(jobsURL.pathname).toBe('/api/projects/tenant/jobs');
});

function Location() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}{location.search}</output>;
}

it.each(['', '?view=all', '?status=ACTIVE&status=READY&cellId=cell-platform', '?status=success&status=failed_app&status=CANCEL_REQUESTED&cellId=cell-platform', '?status=READY&cellId=cell-platform&cellId=cell-app', '?q=search+term&sort=submitted&order=descend'])('preserves list filters through story navigation: %s', async (query) => {
  const listReads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost');
    let data: unknown;
    if (url.pathname.endsWith('/story')) {
      data = { ...jobIdentity, job_id: 'job', status: 'completed', root: { id: 'root', kind: 'recipe', title: 'recipe build', status: 'succeeded', path: [] } };
    } else if (url.pathname.endsWith('/jobs')) {
      if (url.searchParams.has('goal')) return new Response(JSON.stringify({ jobs: [] }));
      listReads.push(url);
      data = { jobs: [{ ...jobIdentity, job_id: 'job', prompt: 'Search term in the full prompt', cell_name: 'platform', status: 'READY', recipe: 'build', store: 'ACTIVE' }] };
    } else data = [];
    return new Response(JSON.stringify(data));
  }));
  render(<MemoryRouter initialEntries={[`/project/tenant/jobs${query}`]}>
    <Location /><BrowserBack />
    <Routes>
      <Route path="/project/:projectId/jobs" element={<JobsListPage projectId="tenant" />} />
      <Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} />
    </Routes>
  </MemoryRouter>);
  const link = await screen.findByRole('link', { name: 'Job job: Ready', exact: true });
  expect(link).toHaveAttribute('href', `/project/tenant/jobs/job/story?executionState=cells-v1${query ? '&' + query.slice(1) : ''}`);
  fireEvent.click(link);
  await screen.findByRole('tab', { name: 'Conversation' });
  fireEvent.click(screen.getByRole('button', { name: 'Browser back' }));
  await screen.findByRole('link', { name: 'Job job: Ready', exact: true });
  expect(screen.getByTestId('location')).toHaveTextContent(`/project/tenant/jobs${query}`);
  await waitFor(() => expect(listReads).toHaveLength(2));
  for (const url of listReads) {
    const expected = new URLSearchParams(query);
    expect(url.searchParams.getAll('displayStatus')).toEqual(expected.getAll('status'));
    expect(url.searchParams.getAll('status')).toEqual(expected.has('status') ? [] : ['all']);
    expect(url.searchParams.has('cell')).toBe(false);
    expect(url.searchParams.has('q')).toBe(false);
    expect(url.searchParams.has('sort')).toBe(false);
  }
});

it.each(['', '?scope=roots'])('searches full prompts across attempts without fetching and clears hidden selections in %s', async query => {
  const jobs = [
    { ...jobIdentity, job_id: 'new', goal_id: 'grouped', title: 'Short title', prompt: `${'Long preamble '.repeat(12)}deep\n  needle`, status: 'READY', store: 'ACTIVE', created_at: '2026-10-29T12:00:00Z' },
    { ...jobIdentity, job_id: 'old', goal_id: 'grouped', prompt: 'Earlier attempt contains an older requirement', status: 'COMPLETED', completion_status: 'failed_app', store: 'ARCHIVED', created_at: '2026-10-28T12:00:00Z' },
    { ...jobIdentity, job_id: 'unrelated', prompt: 'Different work', status: 'READY', store: 'ACTIVE' },
    { ...jobIdentity, job_id: 'missing', status: 'READY', store: 'ACTIVE' },
  ];
  const fetch = vi.fn(async (url: string) => new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs })));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter initialEntries={[`/project/tenant/goals${query}`]}><Navigation /><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job unrelated' }));
  const search = screen.getByRole('textbox', { name: 'Search goals' });
  fireEvent.change(search, { target: { value: ' DEEP needle ' } });
  expect(screen.getByRole('link', { name: 'Short title', exact: true })).toBeVisible();
  expect(screen.getByText('2 attempts on this page')).toBeVisible();
  expect(screen.queryByRole('link', { name: 'Different work', exact: true })).not.toBeInTheDocument();
  expect(screen.getByText('0 selected')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Archive selected' })).toBeDisabled();
  fireEvent.change(search, { target: { value: 'older requirement' } });
  expect(screen.getByRole('link', { name: 'Short title', exact: true })).toBeVisible();
  fireEvent.change(search, { target: { value: 'not present' } });
  expect(screen.getByText('No goals match this search on this page')).toBeVisible();
  fireEvent.change(search, { target: { value: '' } });
  expect(screen.getByRole('link', { name: 'Different work', exact: true })).toBeVisible();
  expect(screen.getByRole('link', { name: 'Untitled goal', exact: true })).toBeVisible();
  const missingRow = screen.getByRole('link', { name: 'Untitled goal', exact: true }).closest('tr')!;
  const missingCells = within(missingRow).getAllByRole('cell');
  expect(missingCells).toHaveLength(6);
  expect(within(missingCells[0]).getByRole('checkbox')).not.toBeChecked();
  expect(missingCells[4]).toHaveTextContent('—');
  expect(within(missingCells[5]).getAllByRole('link')).toHaveLength(1);
  expect(within(missingCells[5]).getByRole('link')).toHaveAttribute('href', 'https://github.com/acme/platform');

  // Sorting pushes a history entry; changing search replaces only that entry.
  // Going back must also clear selection, even though the input did not fire.
  fireEvent.click(screen.getByRole('columnheader', { name: /^Goal/ }));
  fireEvent.change(search, { target: { value: 'Different work' } });
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job unrelated' }));
  expect(screen.getByText('1 selected')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Browser back' }));
  expect(search).toHaveValue('');
  expect(screen.getByText('0 selected')).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(getJob).not.toHaveBeenCalled();
});

it('sorts goal, status, cell, and submission columns locally in both directions', async () => {
  const jobs = [
    { ...jobIdentity, job_id: 'z', title: 'Zebra', status: 'READY', store: 'ACTIVE', cell_name: 'alpha', submitted_at: '2026-10-29T08:00:00Z', created_at: '2026-10-30T00:00:00Z' },
    { ...jobIdentity, job_id: 'a', title: 'Alpha', status: 'COMPLETED', completion_status: 'success', store: 'ARCHIVED', cell_name: 'zeta', created_at: '2026-10-29T09:00:00Z' },
    { ...jobIdentity, job_id: 'm', title: 'Middle', status: 'COMPLETED', completion_status: 'failed_app', store: 'ARCHIVED', cell_name: 'middle', created_at: '2026-10-29T10:00:00Z' },
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
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(getJob).not.toHaveBeenCalled();
});

it('starts status checkboxes selected and distinguishes selecting none from selecting all', async () => {
  const reads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input);
    if (url.pathname.endsWith('/cells')) return new Response('[]');
    reads.push(url);
    return new Response(JSON.stringify({ jobs: [{ ...jobIdentity, job_id: 'job', status: 'READY', store: 'ACTIVE' }] }));
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

it('switches inbox visibility on the backend and resets pagination and selection through history', async () => {
  const jobs = ['visible', 'hidden'].map(job_id => ({ ...jobIdentity, job_id, status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_app' }));
  const reads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input);
    reads.push(url);
    return new Response(JSON.stringify({ jobs: url.searchParams.get('includeHidden') === 'true' ? jobs : jobs.slice(0, 1) }));
  }));
  render(<MemoryRouter initialEntries={['/project/tenant/goals?executionState=cells-v1&status=failed_app&cellId=cell-platform&pageToken=old']}>
    <Navigation /><Location /><JobsListPage projectId="tenant" />
  </MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job visible' }));
  expect(screen.getByLabelText('Job visibility')).toHaveValue('inbox');
  fireEvent.change(screen.getByLabelText('Job visibility'), { target: { value: 'all' } });
  expect(await screen.findByRole('checkbox', { name: 'Select job hidden' })).not.toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Select job visible' })).not.toBeChecked();
  expect(screen.getByRole('button', { name: 'Archive selected' })).toBeDisabled();
  expect(screen.getByTestId('location')).toHaveTextContent('view=all');
  expect(screen.getByTestId('location')).not.toHaveTextContent('pageToken');
  const all = reads.at(-1)!.searchParams;
  expect(all.get('includeHidden')).toBe('true');
  expect(all.get('displayStatus')).toBe('failed_app');
  expect(all.get('cellNames')).toBe('platform');
  expect(all.has('pageToken')).toBe(false);
  fireEvent.click(screen.getByRole('button', { name: 'Browser back' }));
  await waitFor(() => expect(screen.queryByRole('checkbox', { name: 'Select job hidden' })).not.toBeInTheDocument());
  expect(screen.getByLabelText('Job visibility')).toHaveValue('inbox');
  await waitFor(() => expect(reads.at(-1)!.searchParams.get('pageToken')).toBe('old'));
  expect(reads.at(-1)!.searchParams.get('includeHidden')).toBe('false');
  fireEvent.click(screen.getByRole('button', { name: 'Browser forward' }));
  expect(await screen.findByRole('checkbox', { name: 'Select job hidden' })).toBeVisible();
  expect(screen.getByLabelText('Job visibility')).toHaveValue('all');
});

it('groups attempts and restores expanded details through goal navigation and browser back/forward with selected detail queries', async () => {
  const jobs: Partial<RecipeJob>[] = [
    { ...jobIdentity, job_id: 'first', goal_id: 'goal-a', prompt: 'Build a useful feature', status: 'READY', store: 'ACTIVE', cell_name: 'platform', execution: { ...jobIdentity, status: 'specified', source: 'yield', published: true, demand: { effective: { cpu: 4 } } } },
    { ...jobIdentity, job_id: 'second', goal_id: 'goal-a', prompt: 'Build a useful feature', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'failed_app' },
    { ...jobIdentity, job_id: 'single', goal_id: 'goal-b', status: 'READY', store: 'ACTIVE' },
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
  fireEvent.click(within(first).getByRole('button', { name: 'Expand goal details' }));
  fireEvent.click(await screen.findByText('Execution requirements'));
  expect(within(screen.getByText('Execution requirements').closest('details')!).getByText(/"cpu": 4/)).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
  const originalURL = screen.getByTestId('location').textContent;
  fireEvent.click(within(first).getByRole('link', { name: 'Goal overview' }));
  await screen.findByRole('heading', { name: 'Goal overview', exact: true });
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Expand goal details' })).not.toBeInTheDocument();
  expect(await screen.findByText('Execution requirements')).toBeVisible();
  expect(fetch.mock.calls.some(([input]) => {
    const query = new URL(input).searchParams;
    return query.get('goal') === 'goal-a' && query.get('includeHidden') === 'true';
  })).toBe(true);
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
    { ...jobIdentity, job_id: 'review', goal_id: 'g1', status: 'READY', store: 'ACTIVE', next_route: { jobType: 'recipe', taskType: 'input:collect_user_input' } },
    { ...jobIdentity, job_id: 'waiting', goal_id: 'g2', status: 'PENDING_JOBS', store: 'ACTIVE', blocking_wait_summary: { waitId: 'wait' } },
  ] })))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(await screen.findByRole('link', { name: 'Open input/review' })).toHaveAttribute('href', '/project/tenant/reviews/review');
  expect(screen.getByText('Waiting for dependencies')).toBeVisible();
  expect(getJob).not.toHaveBeenCalled();
  expect(screen.queryByRole('link', { name: 'Execution 3' })).not.toBeInTheDocument();
  vi.mocked(getJob).mockResolvedValue({ ...jobIdentity, job_id: 'waiting', status: 'PENDING_JOBS', store: 'ACTIVE', blocking_wait: {
    waitId: 'wait', state: 'PENDING', dependencies: [
      { target: { goalId: 'target-goal' }, state: 'PENDING', condition: 'success' },
      { target: { jobId: 'target-job' }, state: 'PENDING', condition: 'complete' },
    ],
  } } as RecipeJob);
  const row = screen.getByRole('link', { name: 'Job waiting: Waiting for jobs' }).closest('tr')!;
  fireEvent.click(within(row).getByRole('button', { name: 'Expand goal details' }));
  expect(await screen.findByRole('link', { name: 'Goal 1' })).toHaveAttribute('href', '/project/tenant/goals/target-goal');
  expect(screen.getByRole('link', { name: 'Execution 2' })).toHaveAttribute('href', '/project/tenant/jobs/target-job/story');
  expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'waiting');
});

it('submits a goal to a selected cell and opens the returned goal identity', async () => {
  const fetch = vi.fn(async (_: string, init?: RequestInit) => (new Response(JSON.stringify(init?.method === 'POST'
    ? { ...jobIdentity, tenant_id: 'tenant', goal_id: 'created-goal', job_id: 'created-job' } : _.includes('/cells') ? [] : { jobs: [] }))));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(screen.getByRole('button', { name: 'New goal' }));
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).queryByLabelText('Recipe')).not.toBeInTheDocument();
  expect(within(dialog).queryByLabelText('Job ID')).not.toBeInTheDocument();
  fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Cell' }));
  fireEvent.click(await screen.findByText('platform · github.com/acme/platform'));
  fireEvent.change(within(dialog).getByLabelText('Prompt'), { target: { value: 'Build a small tool' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/project/tenant/goals/created-goal'));
  const call = fetch.mock.calls.find(([, init]) => init?.method === 'POST')!;
  expect(JSON.parse(call[1]!.body as string)).toEqual({ cell: 'platform', inputs: { prompt: 'Build a small tool', type: 'build' } });
});

it.each(['self', 'platform'])('uses explicit cell identity in list and attempts (%j)', async cell_name => {
  const jobs = ['first', 'second'].map(job_id => ({ ...jobIdentity, job_id, goal_id: 'goal', prompt: 'Goal prompt', cell_name,
    status: 'READY', store: 'ACTIVE', repo: 'https://example.test/not-a-cell', parent: { cell_name: 'parent-cell' } }));
  vi.stubGlobal('fetch', vi.fn(async (input: string) => (new Response(JSON.stringify(input.includes('/cells') ? [{ name: 'configured-cell', kind: 'self' }] : { jobs })))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  const row = (await screen.findByRole('link', { name: 'Goal prompt', exact: true })).closest('tr')!;
  const label = cell_name;
  expect(within(row).getByText(label, { exact: true })).toBeVisible();
  fireEvent.click(within(row).getByRole('button', { name: 'Expand goal details' }));
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
    { ...jobIdentity, job_id: 'failed', status: 'COMPLETED', store: 'ARCHIVED', completion_status: 'future_outcome', completion_detail: completion },
    { ...jobIdentity, job_id: 'retrying', status: 'READY', store: 'ACTIVE' },
    { ...jobIdentity, job_id: 'waiting', status: 'PENDING_JOBS', store: 'ACTIVE', wait_for: ['second/id', 'first'], blocking_wait_summary: { waitId: 'wait', deadline: '2027-01-01T00:00:00Z' } },
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
  const job = { ...jobIdentity, job_id: 'job', status: 'READY', store: 'ACTIVE', execution: {
    ...jobIdentity, status: 'specified', source: 'yield', published: true, demand: { effective: { resources: { cpu: '4', memory: '4Gi' }, image: 'example/image' } },
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
    await waitFor(() => expect(fetch.mock.calls.length).toBe(before + 1));
    if (label) expect(await screen.findByText(label)).toBeVisible();
    else await waitFor(() => expect(screen.queryByText('invalid demand 雪')).not.toBeInTheDocument());
    expect(screen.queryByText(/4Gi/)).not.toBeInTheDocument();
  }
  expect(getJob).not.toHaveBeenCalled();
});

it('shows restored selected fields even while inspection is unavailable', async () => {
  const raw = `{"jobs":[{"cell_id":"cell-platform","cell_name":"platform","repo":"github.com/acme/platform","job_id":"job","status":"READY","store":"ACTIVE","client_payload":{"other":9007199254740993},"client_payload_revision":7,"task_wait":{"inputOrdinal":0,"outputOrdinal":9007199254740995,"inputHash":"occurrence-hash","resumeJobType":"recipe"},"blocking_wait_summary":{"waitId":"wait-identity","deadline":"2027-01-01T00:00:00Z"}}]}`;
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
  const jobs = ['first', 'second'].map(job_id => ({ ...jobIdentity, tenant_id: 'tenant', job_id, goal_id: 'goal',
    prompt: 'Grouped goal', status: 'READY', store: 'ACTIVE', client_payload_revision: 9 }));
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs })))));
  let resolveFirst!: (job: RecipeJob) => void;
  vi.mocked(getJob).mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve; }))
    .mockImplementation(async (tenant, id) => ({ ...jobIdentity, tenant_id: tenant, job_id: id, completion_detail: `${id} inspected` } as RecipeJob));
  render(<StrictMode><MemoryRouter initialEntries={['/?expanded=goal:goal']}><JobsListPage projectId="tenant" /></MemoryRouter></StrictMode>);
  const attempts = await screen.findByRole('region', { name: 'Attempts' });
  await waitFor(() => expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant', 'first'));
  fireEvent.click(within(attempts).getAllByRole('button', { name: 'Details' })[1]);
  expect((await screen.findAllByText('second inspected'))[0]).toBeVisible();
  await act(async () => resolveFirst({ ...jobIdentity, job_id: 'first', completion_detail: 'LATE first' } as RecipeJob));
  expect(screen.queryByText('LATE first')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(3));
  expect(getJob).toHaveBeenLastCalledWith('tenant', 'second');
  fireEvent.click(screen.getByRole('button', { name: 'Collapse goal details' }));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Expand goal details' })).toBeEnabled());
  expect(getJob).toHaveBeenCalledTimes(3);
  fireEvent.click(screen.getByRole('button', { name: 'Expand goal details' }));
  await waitFor(() => expect(getJob).toHaveBeenCalledTimes(4));
  expect(getJob).toHaveBeenLastCalledWith('tenant', 'second');
});

it('retries failed inspection and never reuses another tenant’s details', async () => {
  const job = { ...jobIdentity, job_id: 'same', goal_id: 'goal', prompt: 'Same ID', status: 'READY', store: 'ACTIVE' };
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [job] })))));
  vi.mocked(getJob).mockRejectedValueOnce(new Error('Unavailable'))
    .mockImplementation(async (tenant) => ({ ...job, completion_detail: `${tenant} details` } as RecipeJob));
  const view = (tenant: string) => <MemoryRouter initialEntries={['/?expanded=goal:goal']}><JobsListPage projectId={tenant} /></MemoryRouter>;
  const { rerender } = render(view('first'));
  fireEvent.click(await screen.findByRole('button', { name: 'Retry details' }));
  expect((await screen.findAllByText('first details'))[0]).toBeVisible();
  rerender(view('second'));
  expect(screen.queryByText('first details')).not.toBeInTheDocument();
  expect((await screen.findAllByText('second details'))[0]).toBeVisible();
  expect(getJob).toHaveBeenLastCalledWith('second', 'same');
});

it('keeps collapsed summaries compact and follows an empty page cursor in StrictMode', async () => {
  // The browser suite covers 200-row rendering. This test controls each response
  // to verify request deduplication and pagination without a large jsdom render.
  const jobs = Array.from({ length: 3 }, (_, index) => ({ ...jobIdentity, job_id: `job-${index}`, goal_id: `goal-${index}`,
    status: 'PENDING_JOBS', store: 'ACTIVE', client_payload_revision: 5, blocking_wait_summary: { waitId: `wait-${index}` } }));
  const requests: { url: URL; resolve: (response: Response) => void }[] = [];
  vi.stubGlobal('fetch', vi.fn((url: string) => new Promise<Response>(resolve => requests.push({ url: new URL(url), resolve }))));
  render(<StrictMode><MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter></StrictMode>);
  expect(requests).toHaveLength(1);
  await act(async () => { requests[0].resolve(new Response(JSON.stringify({ jobs, next_page_token: 'empty' }))); });
  expect(screen.getByRole('checkbox', { name: 'Select job job-2' })).toBeVisible();
  expect(getJob).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Next jobs' }));
  expect(requests).toHaveLength(2);
  expect(requests[1].url.searchParams.get('pageToken')).toBe('empty');
  expect(screen.getByRole('button', { name: 'Next jobs' })).toBeDisabled();
  expect(screen.getByRole('checkbox', { name: 'Select job job-2' })).toBeDisabled();

  await act(async () => { requests[1].resolve(new Response(JSON.stringify({ jobs: [], next_page_token: 'last' }))); });
  expect(screen.queryByRole('checkbox', { name: 'Select job job-2' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Next jobs' })).toBeEnabled();
  fireEvent.click(screen.getByRole('button', { name: 'Next jobs' }));
  expect(requests).toHaveLength(3);
  expect(requests[2].url.searchParams.get('pageToken')).toBe('last');
  await act(async () => { requests[2].resolve(new Response(JSON.stringify({ jobs: [] }))); });
  expect(screen.queryByRole('button', { name: 'Next jobs' })).not.toBeInTheDocument();
  expect(getJob).not.toHaveBeenCalled();
});

it('C1-M01 M02 M08 ignores legacy secondary membership and keeps displaced goals literal', async () => {
  // Compatibility fields from an older backend must not restore secondary groups.
  const job = { ...jobIdentity, job_id: 'member', goal_id: 'primary', other_goal_ids: ['secondary'], prompt: 'Member', status: 'READY', store: 'ACTIVE' };
  vi.stubGlobal('fetch', vi.fn(async (url: string) => (new Response(JSON.stringify(url.includes('/cells') ? [] : { jobs: [job] })))));
  const { rerender } = render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  await screen.findByRole('link', { name: 'Member', exact: true });
  expect(screen.getAllByRole('link', { name: 'Goal overview' })).toHaveLength(1);
  expect(screen.getByRole('link', { name: 'Goal overview' })).toHaveAttribute('href', '/project/tenant/goals/primary');
  expect(getJob).not.toHaveBeenCalled();
  rerender(<MemoryRouter><JobsListPage projectId="tenant" goalId="secondary" /></MemoryRouter>);
  await screen.findByText('No retained jobs for this goal');
  expect(getJob).not.toHaveBeenCalled();
  expect(screen.queryByText('Other goals')).toBeNull();
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes('goal=secondary'))).toBe(true);
});

const archiveCases = [
  ...['failed_app', 'failed_system', 'success', 'cancelled'].map(completion_status => ({
    status: 'COMPLETED', store: 'ACTIVE', completion_status, immediate: true,
  })),
  ...['ACTIVE', 'READY', 'PENDING_JOBS', 'AWAITING_FUTURE', 'EXPIRED', 'CRASH_CONCERN', 'FUTURE_STATE'].map(status => ({
    status, store: 'ACTIVE', immediate: false,
  })),
  ...['failed_timeout', 'future_outcome', ''].map(completion_status => ({
    status: 'COMPLETED', store: 'ACTIVE', completion_status, immediate: false,
  })),
  { status: 'ACTIVE', store: 'ACTIVE', completion_status: 'failed_app', immediate: false },
  { status: 'ACTIVE', store: 'ACTIVE', completion_status: 'failed_system', immediate: false },
  { status: 'CANCELLED', store: 'ACTIVE', cancel_requested: true, immediate: false },
];

it.each(archiveCases)('uses effective status for archive: %j', async ({ immediate, ...state }) => {
  const jobs = [{ ...jobIdentity, job_id: 'target', ...state }];
  const changes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') { changes.push(input); return { ok: true, status: 204 }; }
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(screen.getByRole('button', { name: 'Archive selected' })).toBeDisabled();
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job target' }));
  fireEvent.click(screen.getByRole('button', { name: 'Archive selected' }));
  if (immediate) {
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await waitFor(() => expect(changes).toHaveLength(1));
    await waitFor(() => expect(screen.getByText('0 selected')).toBeVisible());
  } else {
    const dialog = screen.getByRole('dialog');
    expect(changes).toEqual([]);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Archive job', exact: true }));
    await waitFor(() => expect(changes).toHaveLength(1));
  }
  expect(new URL(changes[0], 'http://localhost').pathname).toBe('/api/projects/tenant/jobs/target');
  expect(getJob).not.toHaveBeenCalled();
});

it.each([false, true])('recovers immediate archive failures without automatic retry (total=%s)', async total => {
  const jobs = ['first', 'second', 'untouched'].map(job_id => ({ ...jobIdentity, job_id, status: 'COMPLETED', completion_status: 'success', store: 'ACTIVE' }));
  const changes: string[] = [];
  let fail = true;
  let reads = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') {
      const id = input.split('/').at(-1)!;
      changes.push(id);
      if (fail && (total || id === 'second')) return { ok: false, status: 503, text: async () => 'archive unavailable' };
      return { ok: true, status: 204 };
    }
    reads++;
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<StrictMode><MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter></StrictMode>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job first' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job second' }));
  const button = screen.getByRole('button', { name: 'Archive selected' });
  act(() => { fireEvent.click(button); fireEvent.click(button); });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  const dialog = await screen.findByRole('dialog');
  expect(await within(dialog).findAllByText(/archive unavailable/)).toHaveLength(total ? 2 : 1);
  expect(changes).toEqual(['first', 'second']);
  expect(screen.getByRole('button', { name: 'Cancel selected' })).toBeDisabled();
  const retryName = total ? 'Archive 2 jobs' : 'Archive job';
  fireEvent.click(within(dialog).getByRole('button', { name: retryName, exact: true }));
  await waitFor(() => expect(changes).toHaveLength(total ? 4 : 3));
  await waitFor(() => expect(within(dialog).getByRole('button', { name: retryName, exact: true })).toBeEnabled());
  fail = false;
  fireEvent.click(within(dialog).getByRole('button', { name: retryName, exact: true }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(changes).toEqual(total ? ['first', 'second', 'first', 'second', 'first', 'second'] : ['first', 'second', 'second', 'second']);
  expect(screen.getByText('0 selected')).toBeVisible();
  expect(reads).toBeGreaterThan(1);
});

it('bounds immediate batches and keeps targets fixed through selection and filter changes', async () => {
  const jobs = Array.from({ length: 8 }, (_, i) => ({ ...jobIdentity, job_id: String(i), status: 'COMPLETED', store: 'ACTIVE',
    completion_status: ['success', 'failed_app', 'failed_system', 'cancelled'][i % 4] }));
  const changes: string[] = [];
  const release: (() => void)[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') {
      changes.push(input.split('/').at(-1)!);
      await new Promise<void>(resolve => release.push(resolve));
      return { ok: true, status: 204 };
    }
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<StrictMode><MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter></StrictMode>);
  await screen.findByRole('checkbox', { name: 'Select job 0' });
  for (let i = 0; i < 7; i++) fireEvent.click(screen.getByRole('checkbox', { name: `Select job ${i}` }));
  fireEvent.click(screen.getByRole('button', { name: 'Archive selected' }));
  await waitFor(() => expect(changes).toHaveLength(5));
  expect(vi.mocked(fetch).mock.calls.filter(([, init]) => !init?.method || init.method === 'GET')).toHaveLength(1);
  expect(screen.getByText('Archiving jobs…')).toHaveAttribute('role', 'status');
  expect(screen.getByRole('button', { name: 'Cancel selected' })).toBeDisabled();
  jobs[0].status = 'ACTIVE';
  fireEvent.click(screen.getAllByRole('button', { name: 'Refresh', exact: true })[0]);
  await screen.findByRole('link', { name: 'Job 0: Running', exact: true });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Clear selection' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job 7' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Search goals' }), { target: { value: 'missing' } });
  await act(async () => { release.splice(0).forEach(resolve => resolve()); });
  await waitFor(() => expect(changes).toHaveLength(7));
  expect(changes).toEqual(['0', '1', '2', '3', '4', '5', '6']);
  await act(async () => { release.splice(0).forEach(resolve => resolve()); });
  await waitFor(() => expect(screen.queryByRole('status')).not.toBeInTheDocument());
  expect(await screen.findByText('Archived 7 jobs')).toBeVisible();
  expect(getJob).not.toHaveBeenCalled();
});

it.each([true, false])('checks every grouped attempt (eligible=%s)', async eligible => {
  const jobs = [
    { ...jobIdentity, job_id: 'new', goal_id: 'group', status: 'COMPLETED', store: 'ACTIVE', completion_status: 'success', created_at: '2026-11-01' },
    { ...jobIdentity, job_id: 'old', goal_id: 'group', status: eligible ? 'COMPLETED' : 'READY', store: 'ACTIVE', completion_status: eligible ? 'failed_app' : undefined, created_at: '2026-10-01' },
  ];
  const changes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') { changes.push(input.split('/').at(-1)!); return { ok: true, status: 204 }; }
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: /Select attempts for/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Archive selected' }));
  if (eligible) {
    await waitFor(() => expect(changes).toEqual(['new', 'old']));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  } else {
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(changes).toEqual([]);
  }
});

it('keeps confirmation targets fixed after filters change', async () => {
  const jobs = ['captured', 'other'].map(job_id => ({ ...jobIdentity, job_id, status: 'READY', store: 'ACTIVE' }));
  const changes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') { changes.push(input.split('/').at(-1)!); return { ok: true, status: 204 }; }
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox', { name: 'Select job captured' }));
  fireEvent.click(screen.getByRole('button', { name: 'Archive selected' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Search goals' }), { target: { value: 'missing' } });
  expect(changes).toEqual([]);
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Archive job', exact: true }));
  await waitFor(() => expect(changes).toEqual(['captured']));
});

it('archives only the eligible attempt individually selected within a mixed group', async () => {
  const jobs = [
    { ...jobIdentity, job_id: 'finished', goal_id: 'group', status: 'COMPLETED', store: 'ACTIVE', completion_status: 'success' },
    { ...jobIdentity, job_id: 'running', goal_id: 'group', status: 'ACTIVE', store: 'ACTIVE' },
  ];
  const changes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === 'DELETE') { changes.push(input.split('/').at(-1)!); return { ok: true, status: 204 }; }
    return new Response(JSON.stringify({ jobs }));
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  const groupSelection = await screen.findByRole('checkbox', { name: 'Select attempts for Untitled goal', exact: true });
  fireEvent.click(groupSelection);
  expect(screen.getByText('2 selected')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Clear selection' }));
  const selectAll = within(screen.getAllByRole('columnheader')[0]).getByRole('checkbox');
  fireEvent.click(selectAll);
  expect(screen.getByText('2 selected')).toBeVisible();
  fireEvent.click(selectAll);
  expect(screen.getByText('0 selected')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Expand goal details' }));
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select job finished', exact: true }));
  expect(groupSelection).not.toBeChecked();
  expect(groupSelection).toBePartiallyChecked();

  fireEvent.click(screen.getByRole('button', { name: 'Archive selected' }));
  await waitFor(() => expect(changes).toEqual(['finished']));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

it.each(['cancel', 'archive'] as const)('prompts for a single selected job on %s', async action => {
  const jobs = [{ ...jobIdentity, job_id:'one', status:'READY',store:'ACTIVE' }];
  const mutations: RequestInit[] = [];
  vi.stubGlobal('fetch',vi.fn(async (_input: string, init?: RequestInit) => {
    if (init?.method === 'POST' || init?.method === 'DELETE') { mutations.push(init); return new Response(null,{status:204}); }
    return new Response(JSON.stringify({jobs}));
  }));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('checkbox',{name:'Select job one'}));
  fireEvent.click(screen.getByRole('button',{name:action === 'cancel' ? 'Cancel selected' : 'Archive selected'}));
  const dialog = screen.getByRole('dialog');
  const input = within(dialog).getByRole('textbox',{name:'Cancellation message (optional)'});
  expect(mutations).toEqual([]);
  fireEvent.change(input,{target:{value:'  chosen 雪\nreason  '}});
  fireEvent.click(within(dialog).getByRole('button',{name: action === 'cancel' ? 'Cancel job' : 'Archive job',exact:true}));
  await waitFor(() => expect(mutations).toHaveLength(1));
  expect(JSON.parse(mutations[0].body as string)).toEqual({reason:'chosen 雪\nreason'});
});

it.each(['ACTIVE', 'ARCHIVED'])('CID-010 opens an existing UUID job from the %s store', async store => {
 const id = '1fac9e89-36cf-4a4f-a797-7d4fe887c65c';
 vi.mocked(getJob).mockResolvedValue({ ...jobIdentity, job_id: id, goal_id: 'goal', store, status: 'READY' } as RecipeJob);
 const fetchMock = vi.fn(async (input: string) => new Response(JSON.stringify(input.includes('/story')
  ? { job_id: id, status: 'completed', root: { id: 'root', kind: 'recipe', title: 'UUID history', status: 'succeeded', path: [] } }
  : { jobs: [{ ...jobIdentity, job_id: id, store, status: 'READY' }] })));
 vi.stubGlobal('fetch', fetchMock);
 render(<MemoryRouter initialEntries={['/project/tenant/jobs?view=all']}>
  <Location /><Routes>
   <Route path="/project/:projectId/jobs" element={<JobsListPage projectId="tenant" />} />
   <Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} />
  </Routes>
 </MemoryRouter>);
 const link = await screen.findByRole('link', { name: `Job ${id}: Ready`, exact: true });
 expect(link.getAttribute('href')).toContain(`/project/tenant/jobs/${id}/story`);
 fireEvent.click(link);
 await screen.findByRole('tab', {name:'Conversation',exact:true});
 expect(getJob).toHaveBeenCalledWith('tenant', id);
 expect(fetchMock.mock.calls.some(([url]) => url.includes(`/projects/tenant/jobs/${id}/story`))).toBe(false);
 fireEvent.click(screen.getByRole('tab', { name: 'Trace', exact: true }));
 await screen.findByRole('tree');
 expect(fetchMock.mock.calls.some(([url]) => url.includes(`/projects/tenant/jobs/${id}/story`))).toBe(true);
});

it.each(['/goals', '/jobs', '/goals?scope=unknown'])('GS-010 defaults %s to All goals', async path => {
  const fetch = vi.fn(async (_url: string) => new Response('{"jobs":[]}'));
  vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter initialEntries={[`/project/tenant${path}`]}><JobsListPage projectId="tenant" /></MemoryRouter>);
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(screen.getByLabelText('Goal scope')).toHaveValue('all');
  expect(new URL(fetch.mock.calls[0][0]).searchParams.get('rootOnly')).toBe('false');
});

it('GS-011/013/019 resets scope state and cache while preserving other filters and grouping inherited parents', async () => {
  const jobs = ['first', 'second'].map(job_id => ({ ...jobIdentity, job_id, goal_id: 'shared', prompt: 'Find this work', status: 'READY', store: 'ACTIVE', parent: { job_id: 'parent', tenant_id: 'tenant' } }));
  const reads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => { reads.push(new URL(url)); return new Response(JSON.stringify({ jobs })); }));
  render(<MemoryRouter initialEntries={['/project/tenant/goals?executionState=cells-v1&pageToken=old&cellId=cell-platform&status=READY&view=all&q=Find&sort=prompt&order=descend&expanded=goal:shared&inspect=second']}>
    <Location /><JobsListPage projectId="tenant" />
  </MemoryRouter>);
  await screen.findByText('2 attempts on this page');
  await waitFor(() => expect(getJob).toHaveBeenCalled());
  fireEvent.click(screen.getByRole('checkbox', { name: 'Select attempts for Find this work' }));
  expect(screen.getByText('2 selected')).toBeVisible();
  const inspections = vi.mocked(getJob).mock.calls.length;
  fireEvent.change(screen.getByLabelText('Goal scope'), { target: { value: 'roots' } });
  await waitFor(() => expect(reads.at(-1)!.searchParams.get('rootOnly')).toBe('true'));
  await screen.findByText('2 attempts on this page');
  expect(screen.getByText('0 selected')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Expand goal details' })).toHaveAttribute('aria-expanded', 'false');
  const location = new URL('http://localhost' + screen.getByTestId('location').textContent!);
  for (const key of ['pageToken', 'expanded', 'inspect', 'detail']) expect(location.searchParams.has(key)).toBe(false);
  for (const [key, value] of Object.entries({ scope: 'roots', cellId: 'cell-platform', status: 'READY', view: 'all', q: 'Find', sort: 'prompt', order: 'descend' })) expect(location.searchParams.get(key)).toBe(value);
  const query = reads.at(-1)!.searchParams;
  expect(query.get('cellNames')).toBe('platform'); expect(query.get('displayStatus')).toBe('READY'); expect(query.get('includeHidden')).toBe('true'); expect(query.has('pageToken')).toBe(false);
  // Root results can retain a Parent link; no client-side classification.
  expect(screen.getByRole('link', { name: 'Parent', exact: true })).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Expand goal details' }));
  await waitFor(() => expect(vi.mocked(getJob).mock.calls.length).toBeGreaterThan(inspections));
  expect(screen.getByRole('checkbox', { name: 'Select attempts for Find this work' })).not.toBeChecked();
});

it.each(['success', 'error'])('GS-014 ignores late earlier-scope %s and clears stale rows while loading', async outcome => {
  const requests: { url: URL; resolve: (response: Response) => void }[] = [];
  vi.stubGlobal('fetch', vi.fn((url: string) => new Promise<Response>(resolve => requests.push({ url: new URL(url), resolve }))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  const job = { ...jobIdentity, job_id: 'old', prompt: 'Old scope work', status: 'READY', store: 'ACTIVE' };
  await act(async () => requests[0].resolve(new Response(JSON.stringify({ jobs: [job] }))));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh', exact: true }));
  expect(requests).toHaveLength(2);
  fireEvent.change(screen.getByLabelText('Goal scope'), { target: { value: 'roots' } });
  expect(requests).toHaveLength(3);
  expect(screen.queryByText('Old scope work')).not.toBeInTheDocument();
  await waitFor(() => expect(document.querySelector('.ant-spin-spinning')).toBeInTheDocument());
  await act(async () => requests[2].resolve(new Response(JSON.stringify({ jobs: [{ ...job, job_id: 'root', prompt: 'Current root work' }] }))));
  await act(async () => requests[1].resolve(outcome === 'success' ? new Response(JSON.stringify({ jobs: [job] })) : new Response('old error', { status: 500 })));
  expect(screen.getByText('Current root work')).toBeVisible();
  expect(screen.queryByText('Old scope work')).not.toBeInTheDocument(); expect(screen.queryByText('old error')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Goal scope')).toHaveValue('roots');
});

it('GS-016/017 retains root scope across errors, retry and continued empty pages until an explicit switch', async () => {
  let stage = 0;
  const reads: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    reads.push(new URL(url));
    return stage === 0 ? new Response('root filtering unsupported', { status: 501 }) : new Response(JSON.stringify(stage === 1 ? { jobs: [], next_page_token: 'more' } : { jobs: [] }));
  }));
  render(<MemoryRouter initialEntries={['/project/tenant/jobs?scope=roots']}><JobsListPage projectId="tenant" /></MemoryRouter>);
  expect(await screen.findByText(/root filtering unsupported/)).toBeVisible();
  expect(screen.getByLabelText('Goal scope')).toHaveValue('roots'); expect(reads).toHaveLength(1);
  stage = 1; fireEvent.click(screen.getByRole('button', { name: 'Retry', exact: true }));
  expect(await screen.findByText('No matches on this page. More results may be available.')).toBeVisible();
  stage = 2; fireEvent.click(screen.getByRole('button', { name: 'Next jobs' }));
  expect(await screen.findByText('No root goals found')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Next jobs' })).not.toBeInTheDocument();
  expect(reads.every(url => url.searchParams.get('rootOnly') === 'true')).toBe(true);
  expect(reads.at(-1)!.searchParams.get('pageToken')).toBe('more');
  stage = 0; fireEvent.click(screen.getByRole('button', { name: 'Refresh', exact: true }));
  await screen.findByText(/root filtering unsupported/);
  stage = 2; fireEvent.click(screen.getByRole('button', { name: 'All goals', exact: true }));
  await waitFor(() => expect(screen.getByLabelText('Goal scope')).toHaveValue('all'));
  await waitFor(() => expect(reads.at(-1)!.searchParams.get('rootOnly')).toBe('false'));
});

it.each(['status=none', 'cellSelection=none'])('GS-019 preserves empty selection %s when changing scope', async query => {
  const fetch = vi.fn(async () => new Response('{"jobs":[]}')); vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter initialEntries={[`/project/tenant/goals?${query}`]}><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.change(screen.getByLabelText('Goal scope'), { target: { value: 'roots' } });
  await waitFor(() => expect(document.querySelector('.ant-spin-spinning')).not.toBeInTheDocument());
  expect(screen.getByText(query.startsWith('status') ? 'No statuses selected' : 'No cells selected')).toBeVisible();
  expect(screen.getByTestId('location')).toHaveTextContent(query); expect(fetch).not.toHaveBeenCalled();
});

it.each(['recipe','generic'])('DAG-09 selected %s jobs use the appropriate DAG destination', async job_type => {
 const job={...jobIdentity,job_id:'entry',goal_id:'goal',prompt:'Grouped goal',tenant_id:'tenant',status:'READY',store:'ACTIVE',job_type} as RecipeJob;
 vi.mocked(getJob).mockResolvedValue(job);
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({jobs:[job]}))));
 render(<MemoryRouter initialEntries={['/?expanded=goal:goal']}><JobsListPage projectId="tenant" /></MemoryRouter>);
 await waitFor(()=>expect(getJob).toHaveBeenCalled());
 expect(await screen.findByRole('link',{name:'DAG',exact:true})).toHaveAttribute('href',`/project/tenant/jobs/entry/${job_type==='recipe'?'story?tab=dag':'tree'}`);
});

const copyId = '3M11h0hfoxIoOk6knxCN6DW9o0u';
function copyFixture(job_id: string, extra: Partial<RecipeJob> = {}) {
  return { ...jobIdentity, job_id, prompt: `Work ${job_id}`, status: 'READY', store: 'ACTIVE', created_at: '2026-11-01T00:00:00Z', ...extra };
}
function clipboardFixture(clipboard: unknown) {
  vi.stubGlobal('navigator', { userAgent: navigator.userAgent, clipboard });
}
for (const width of [1280, 390]) for (const alias of ['goals', 'jobs']) {
  it(`GJC-001/002/005/006/008/010/011/013 copies independently at ${width} on ${alias}`, async () => {
    vi.stubGlobal('innerWidth', width);
    let resolve!: () => void;
    const writeText = vi.fn().mockImplementationOnce(() => new Promise<void>(done => { resolve = done; }))
      .mockRejectedValueOnce(new Error('Denied')).mockResolvedValue(undefined);
    clipboardFixture({ writeText });
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ jobs: [copyFixture(copyId, { goal_id: 'different-goal' }), copyFixture('untouched')] })));
    vi.stubGlobal('fetch', fetchMock);
    const initial = `/project/tenant/${alias}?executionState=cells-v1&expanded=goal%3Adifferent-goal&q=Work&sort=prompt&order=ascend`;
    render(<MemoryRouter initialEntries={[initial]}><Location /><JobsListPage projectId="tenant" /></MemoryRouter>);
    const button = await screen.findByRole('button', { name: `Copy job ID ${copyId}`, exact: true });
    const container = button.closest(width < 768 ? '.goal-card' : 'tr')! as HTMLElement;
    const local = within(container);
    expect(button.closest('a')).toBeNull();
    expect(button.querySelector('svg')).toHaveAttribute('aria-hidden', 'true');
    expect(local.getByRole('link', { name: `Work ${copyId}`, exact: true })).toHaveAttribute('href', expect.stringContaining(`/jobs/${copyId}/story`));
    const checkbox = local.getByRole('checkbox', { name: `Select job ${copyId}`, exact: true });
    fireEvent.click(checkbox);
    await waitFor(() => expect(getJob).toHaveBeenCalled());
    const requests = fetchMock.mock.calls.length;
    const detailReads = vi.mocked(getJob).mock.calls.length;
    const location = screen.getByTestId('location').textContent;
    const unchanged = () => {
      expect(checkbox).toBeChecked();
      expect(screen.getByRole('checkbox', { name: 'Select job untouched', exact: true })).not.toBeChecked();
      expect(local.getByRole('button', { name: 'Collapse goal details' })).toHaveAttribute('aria-expanded', 'true');
      expect(screen.getByTestId('location').textContent).toBe(location);
      expect(fetchMock).toHaveBeenCalledTimes(requests);
      expect(getJob).toHaveBeenCalledTimes(detailReads);
    };
    fireEvent.click(button);
    expect(writeText).toHaveBeenLastCalledWith(copyId);
    expect(local.queryByText('Job ID copied')).not.toBeInTheDocument();
    await act(async () => resolve());
    expect(local.getByRole('status')).toHaveTextContent('Job ID copied'); unchanged();
    fireEvent.click(button);
    expect(await local.findByText('Could not copy job ID')).toBeVisible();
    expect(within(local.getByRole('status')).getByText(copyId, { selector: 'code' })).toHaveTextContent(copyId);
    expect(local.queryByText('Job ID copied')).not.toBeInTheDocument(); unchanged();
    fireEvent.click(button);
    expect(await local.findByText('Job ID copied')).toBeVisible();
    expect(writeText.mock.calls).toEqual([[copyId], [copyId], [copyId]]); unchanged();
    const untouched = screen.getByRole('button', { name: 'Copy job ID untouched', exact: true }).closest(width < 768 ? '.goal-card' : 'tr')! as HTMLElement;
    expect(within(untouched).queryByText('Job ID copied')).not.toBeInTheDocument();
    expect(within(untouched).getByRole('button', { name: 'Expand goal details' })).toHaveAttribute('aria-expanded', 'false');
  });
}
it.each([undefined, {}])('GJC-007 handles unavailable clipboard %j', async clipboard => {
  clipboardFixture(clipboard);
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ jobs: [copyFixture(copyId)] }))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: `Copy job ID ${copyId}` }));
  expect(await screen.findByText('Could not copy job ID')).toBeVisible();
  expect(screen.getByText(copyId, { selector: 'code' })).toBeVisible();
  expect(screen.queryByText('Job ID copied')).not.toBeInTheDocument();
});
it.each([true, false])('GJC-003/004 uses the title attempt with active=%s', async active => {
  const writeText = vi.fn().mockResolvedValue(undefined); clipboardFixture({ writeText });
  const jobs = [
    copyFixture('older-active', { goal_id: 'shared', created_at: '2026-11-01T00:00:00Z', status: active ? 'READY' : 'COMPLETED' }),
    copyFixture(copyId, { goal_id: 'shared', created_at: '2026-11-02T00:00:00Z', status: active ? 'READY' : 'COMPLETED' }),
    copyFixture('newer-completed', { goal_id: 'shared', created_at: '2026-11-03T00:00:00Z', status: 'COMPLETED' }),
    copyFixture('newest-archived', { goal_id: 'shared', created_at: '2026-11-04T00:00:00Z', store: 'ARCHIVED' }),
  ];
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ jobs }))));
  render(<MemoryRouter><JobsListPage projectId="tenant" /></MemoryRouter>);
  const expected = active ? copyId : 'newest-archived';
  const button = await screen.findByRole('button', { name: `Copy job ID ${expected}`, exact: true });
  const row = within(button.closest('tr')!);
  expect(row.getByRole('link', { name: `Work ${expected}`, exact: true })).toHaveAttribute('href', expect.stringContaining(`/jobs/${expected}/story`));
  fireEvent.click(button);
  await screen.findByText('Job ID copied');
  expect(writeText).toHaveBeenCalledExactlyOnceWith(expected);
});

it.each([403, 503])('C1-M08 M09 keeps loading, failed reads (%s), sparse continuation and exhaustion distinct on a literal goal URL',async status=>{
 let resolve!:(r:Response)=>void;
 const requests=vi.fn((_url:string)=>new Promise<Response>(done=>{resolve=done;}));vi.stubGlobal('fetch',requests);
 render(<MemoryRouter initialEntries={['/project/tenant/goals/former-A']}><JobsListPage projectId="tenant" goalId="former-A" /></MemoryRouter>);
 expect(await screen.findByText('Loading jobs for this goal…')).toBeVisible();
 expect(screen.queryByText('No retained jobs for this goal')).toBeNull();
 await act(async()=>resolve(new Response(JSON.stringify({error:'Controlled list failure'}),{status})));
 expect(await screen.findByText('Jobs for this goal unavailable')).toBeVisible();
 expect(screen.queryByText('No retained jobs for this goal')).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Retry',exact:true}));
 await act(async()=>resolve(new Response(JSON.stringify({jobs:[],next_page_token:'opaque+/='}))));
 expect(await screen.findByText('No retained matches on this page. More results may be available.')).toBeVisible();
 fireEvent.click(screen.getByRole('button',{name:'Next jobs'}));
 expect(requests.mock.calls.at(-1)?.[0]).toContain('opaque%2B%2F%3D');
 await act(async()=>resolve(new Response(JSON.stringify({jobs:[]}))));
 expect(await screen.findByText('No retained jobs for this goal')).toBeVisible();
 // A refreshed suffix cannot reuse evidence from the previous empty prefix.
 fireEvent.click(screen.getByRole('button',{name:'Refresh',exact:true}));
 await act(async()=>resolve(new Response(JSON.stringify({jobs:[]}))));
 expect(await screen.findByText('No retained matches on this page.')).toBeVisible();
 expect(screen.queryByText('No retained jobs for this goal')).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'First page'}));
 await act(async()=>resolve(new Response(JSON.stringify({jobs:[]}))));
 expect(await screen.findByText('No retained jobs for this goal')).toBeVisible();
 expect(getJob).not.toHaveBeenCalled();
});

it.each([false, true])('C1-M08 M09 REV-001 keeps exhausted cursor absence page-local; direct entry=%s', async direct => {
 const goalPath = '/project/tenant/goals/former-A';
 const cursor = 'opaque+/=';
 const retained = { ...jobIdentity, tenant_id: 'tenant', job_id: 'retained', goal_id: 'former-A', prompt: 'Retained primary attempt', status: 'COMPLETED', store: 'ARCHIVED' };
 const requests = vi.fn(async (input: string) => {
  const url = new URL(input, 'http://localhost');
  expect(url.searchParams.get('goal')).toBe('former-A');
  return new Response(JSON.stringify(url.searchParams.has('pageToken') ? { jobs: [] } : { jobs: [retained], next_page_token: cursor }));
 });
 vi.stubGlobal('fetch', requests);
 const initial = goalPath + (direct ? '?executionState=cells-v1&pageToken=' + encodeURIComponent(cursor) : '');
 function Location() { const location = useLocation(); return <output aria-label="Literal goal URL">{location.pathname + location.search}</output>; }
 render(<MemoryRouter initialEntries={[initial]}><Location /><JobsListPage projectId="tenant" goalId="former-A" /></MemoryRouter>);
 if (!direct) {
  expect((await screen.findAllByText('Retained primary attempt')).length).toBeGreaterThan(0);
  fireEvent.click(screen.getByRole('button', { name: 'Next jobs' }));
 }
 expect(await screen.findByText('No retained matches on this page.')).toBeVisible();
 expect(screen.queryByText('No retained jobs for this goal')).toBeNull();
 expect(screen.queryByRole('button', { name: 'Next jobs' })).toBeNull();
 expect(screen.getByLabelText('Literal goal URL').textContent).toBe(goalPath + '?executionState=cells-v1&pageToken=' + encodeURIComponent(cursor));
 expect(new URL(requests.mock.calls.at(-1)![0], 'http://localhost').searchParams.get('pageToken')).toBe(cursor);
 fireEvent.click(screen.getByRole('button', { name: 'First page' }));
 expect((await screen.findAllByText('Retained primary attempt')).length).toBeGreaterThan(0);
 expect(screen.getByLabelText('Literal goal URL').textContent).toBe(goalPath + '?executionState=cells-v1');
 expect(screen.getByRole('button', { name: 'Next jobs' })).toBeEnabled();
});
it.each([false,true])('C1-M05 M10 preserves explicitly missing selection instead of a terminal neighbor; empty=%s',async empty=>{
 const historical=['first','second'].map(job_id=>({...jobIdentity,tenant_id:'tenant',job_id,goal_id:'chosen',status:'COMPLETED',store:'ARCHIVED',completion_status:'success',prompt:'Historical results'}));
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({jobs:empty?[]:historical}))));
 vi.mocked(getJob).mockRejectedValue(new Error('404 selected identity missing'));
 render(<MemoryRouter initialEntries={['/project/tenant/goals/chosen?inspect=missing']}><JobsListPage projectId="tenant" goalId="chosen"/></MemoryRouter>);
 await screen.findByText('Could not load job details');
 expect(getJob).toHaveBeenCalledExactlyOnceWith('tenant','missing');
 expect(screen.getByText('Requested job: missing')).toBeVisible();
 expect(screen.getByRole('link',{name:'DAG',exact:true})).toHaveAttribute('href','/project/tenant/jobs/missing/story?tab=dag');
 if(!empty) expect(screen.getAllByText('Succeeded')).toHaveLength(2);
 expect(screen.queryByText('Other goals')).toBeNull();
});
