import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import JobStoryPage from './JobStoryPage';
import { jobIdentity } from '../test/cellFixtures';
import { getJob, listJobChildren } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import DownloadChaptersButton from './DownloadChaptersButton';

vi.mock('@colony2/shared', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  useInputActivity: () => ({ pendingInputs: [], refresh: vi.fn() }),
}));
vi.mock('@colony2/shared/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared/api')>(),
  getJob: vi.fn(),
  listJobChildren: vi.fn().mockResolvedValue({ jobs: [] }),
}));
vi.mock('react-json-view', () => ({ default: ({ src }: { src: unknown }) => <pre>{JSON.stringify(src)}</pre> }));

// Sanitized shape of the supplied history: IDs repeat across job attempts,
// and each job attempt contains three distinct failed task-attempt nodes.
function jobAttempt(job: number) {
  return {
    id: 'n_3', kind: 'recipe', title: 'Build', status: 'failed', path: [],
    attempt: 1, job_attempt: job,
    children: [{
      id: 'n_4', kind: 'sequence', title: 'Development', status: 'failed', path: [],
      children: [{
        id: 'n_5', kind: 'state', title: 'Design', status: 'failed', path: ['design'],
        children: [1, 2, 3].map(attempt => ({
          id: `n_${34 + attempt * 2}`, kind: 'opStep', title: `Task ${job}.${attempt}`,
          status: 'failed', path: ['design', 'extension'], attempt,
          task_ordinal: (job - 1) * 11 + 7 + attempt,
          error: { code: 'container_failed', message: `Failure ${job}.${attempt}: temporary directory missing` },
        })),
      }],
    }],
  };
}
const story = {
  job_id: 'job', status: 'failed', recipe: { name: 'build' },
  root: { ...jobAttempt(3), past_attempts: [jobAttempt(1), jobAttempt(2)] },
};
function Location() {
  return <output data-testid="location">{useLocation().search}</output>;
}
function mount(search = '', response: unknown = story) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => response });
  vi.stubGlobal('fetch', fetchMock);
  render(<MemoryRouter initialEntries={[`/project/tenant/jobs/job/story${search}`]}>
    <Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<><JobStoryPage projectId="tenant" /><Location /></>} /></Routes>
  </MemoryRouter>);
  return fetchMock;
}
beforeEach(() => { vi.mocked(getJob).mockReset().mockResolvedValue({ ...jobIdentity, goal_id: 'goal' } as RecipeJob); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); });

describe('job story history', () => {
  it('preserves the expanded goal return location after selecting a node and a detail tab', async () => {
    const returnTo = '/project/tenant/goals?expanded=goal%3Agoal&detail=requirements';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
    render(<MemoryRouter initialEntries={[{ pathname: '/project/tenant/jobs/job/story', state: { goalsReturnTo: returnTo, goalId: 'goal', goalReturnTo: '/project/tenant/goals/goal?inspect=job' } }]}>
      <Routes>
        <Route path="/project/:projectId/jobs/:jobId/story" element={<><JobStoryPage projectId="tenant" /><Location /></>} />
        <Route path="/project/:projectId/goals" element={<><span>Returned to goal</span><Location /></>} />
      </Routes>
    </MemoryRouter>);
    fireEvent.click(await screen.findByText('Development.Design.Task 3.2'));
    expect(screen.getByTestId('location')).toHaveTextContent('nodeId=');
    fireEvent.click(screen.getByRole('tab', { name: 'Output', exact: true }));
    expect(screen.getByRole('link', { name: 'Goal overview' })).toHaveAttribute('href', '/project/tenant/goals/goal?inspect=job');
    fireEvent.click(screen.getByRole('button', { name: /Back to Goals/ }));
    expect(await screen.findByText('Returned to goal')).toBeVisible();
    expect(screen.getByTestId('location')).toHaveTextContent('?expanded=goal%3Agoal&detail=requirements');
  });

  it('keeps auto-refresh off after an in-flight poll or a manual refresh finishes', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    const running = { ...story, status: 'running' };
    const fetchMock = mount('', running);
    const toggle = await screen.findByRole('switch', { name: 'Auto-refresh' });
    expect(toggle).toBeChecked();

    let finishPoll!: (response: unknown) => void;
    fetchMock.mockImplementationOnce(() => new Promise(resolve => { finishPoll = resolve; }));
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    fireEvent.click(toggle);
    expect(toggle).not.toBeChecked();
    await act(async () => { finishPoll({ ok: true, json: async () => running }); });
    expect(toggle).not.toBeChecked();
    await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /Refresh$/ })); });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(toggle).not.toBeChecked();
    await act(async () => { await vi.advanceTimersByTimeAsync(10000); });
    expect(fetchMock).toHaveBeenCalledTimes(3);

    fireEvent.click(toggle);
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(fetchMock).toHaveBeenCalledTimes(4);
    expect(toggle).toBeChecked();
  });

  it('uses descending job attempt tabs and preserves inspected failures across refresh', async () => {
    const fetchMock = mount();
    await screen.findByText('Development.Design.Task 3.2');
    const mainTree = screen.getByRole('tree');
    expect(within(mainTree).queryByText(/Task 1/)).not.toBeInTheDocument();
    expect(within(mainTree).queryByText(/Previous job attempts/)).not.toBeInTheDocument();
    expect(screen.getAllByRole('tab', { name: /^Job attempt / }).map(tab => tab.textContent)).toEqual(['3', '2', '1']);
    expect(screen.getByRole('tab', { name: 'Job attempt 3' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('tab', { name: /Retries/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: 'Job attempt 1' }));
    expect(screen.getByRole('tab', { name: 'Job attempt 1' })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByText('Development.Design.Task 1.2'));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    expect(screen.getByTestId('location').textContent).toContain('nodeId=jobAttempt%3A1%7Cn_38');
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.getByText('Failure 1.2: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Job attempts' }));
    fireEvent.click(screen.getByRole('tab', { name: 'Job attempt 3' }));
    fireEvent.click(within(mainTree).getByText('Development.Design.Task 3.2'));
    expect(await screen.findByText('Failure 3.2: temporary directory missing')).toBeVisible();
    expect(screen.queryByText('Failure 1.2: temporary directory missing')).not.toBeInTheDocument();
  });

  it('restores links to a previous job attempt despite repeated node IDs', async () => {
    mount('?nodeId=jobAttempt%3A2%7Cn_40');
    expect(await screen.findByText('Failure 2.3: temporary directory missing')).toBeVisible();
  });

  it('keeps old node-ID links working and lets users collapse and expand the story', async () => {
    mount('?nodeId=n_40');
    expect(await screen.findByText('Failure 3.3: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Collapse all' }));
    await waitFor(() => expect(within(screen.getByRole('tree')).queryByText('Development.Design.Task 3.1')).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Expand all' }));
    expect(await screen.findByText('Development.Design.Task 3.1')).toBeVisible();
  });

  it('also shows failures represented as prior task attempts', async () => {
    const root = jobAttempt(1);
    const tasks = root.children[0].children[0].children;
    const response = { ...story, root: { ...root, children: [{ ...tasks[2], prior_attempts: tasks.slice(0, 2) }] } };
    mount('', response);
    await within(await screen.findByRole('tree')).findByText('Task 1.3');
    fireEvent.click(within(screen.getByRole('tree')).getByText('Task 1.3'));
    expect(within(screen.getByRole('tree')).queryByText('Task 1.1')).not.toBeInTheDocument();
    expect(screen.getAllByRole('tab', { name: /^Attempt / }).map(tab => tab.textContent)).toEqual(['3', '2', '1']);
    expect(screen.getByRole('tab', { name: 'Attempt 3' })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByRole('tab', { name: 'Attempt 1' }));
    expect(screen.getByRole('tab', { name: 'Attempt 1' })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByText('Failure 1.1: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: 'Output', exact: true }));
    fireEvent.click(screen.getByRole('tab', { name: 'Attempt 2' }));
    expect(screen.getByRole('tab', { name: 'Output', exact: true })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByRole('tab', { name: 'Overview', exact: true }));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Refresh$/ })).not.toHaveClass('ant-btn-loading'));
    expect(screen.getByRole('tab', { name: 'Attempt 2' })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(within(screen.getByRole('tree')).getByText('Task 1.3'));
    expect(screen.getByRole('tab', { name: 'Attempt 3' })).toHaveAttribute('aria-selected', 'true');
  });
  it('shows a short scoped operation name and keeps source and restart in Overview', async () => {
    const source = 'git+https://github.com/colony-2/c2ops.git//codex@abc123';
    const scopeID = 'root/scope:"op:codex"#1';
    const fetchMock = mount('', { ...story, restart_scopes: [
      { chapter_ordinal: 4, title: 'Recipe body', scope_id: 'root', started_at: '2026-10-14T05:00:00Z' },
      { chapter_ordinal: 12, title: 'Run codex', scope_id: scopeID, started_at: '2026-10-14T05:01:00Z' },
    ], root: {
      id: 'root', kind: 'recipe', title: 'recipe build', path: [], status: 'failed', children: [{
        id: 'machine', kind: 'stateMachine', title: 'stateMachine agent', path: [], status: 'failed', children: [{
          id: 'state', kind: 'state', title: 'state run', path: [], status: 'failed', children: [{
            id: 'op', kind: 'op', title: `op ${source}`, op_id: source, path: [], status: 'failed', children: [{
              id: 'task', kind: 'opStep', title: 'step extension_execution', path: [], status: 'failed',
              restart_chapter_ordinal: 12, restart_relationship: 'containing',
              task_ordinal: 14, error: { message: 'command failed' },
            }],
          }],
        }],
      }],
    } });
    fireEvent.click(await screen.findByText('agent.run.codex'));
    const tree = screen.getByRole('tree');
    expect(within(tree).getAllByRole('treeitem')).toHaveLength(2);
    expect(tree).not.toHaveTextContent('Restart');
    expect(tree).not.toHaveTextContent('git+');
    expect(screen.getByText(source, { exact: true })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByRole('button', { name: 'Restart', exact: true })).toBeEnabled();
    expect(within(dialog).getByText(text => text.startsWith('Run codex · Chapter 12 ·'))).toBeVisible();
    // A manually chosen boundary must not leak into the next opening.
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Restart point' }));
    fireEvent.click(await screen.findByText(text => text.startsWith('Recipe body · Chapter 4 ·')));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel', exact: true }));
    fireEvent.click(screen.getByRole('button', { name: 'Restart job' }));
    expect(within(dialog).getByText(text => text.startsWith('Run codex · Chapter 12 ·'))).toBeVisible();
    expect(within(dialog).getByText('This restarts the containing scope, including its work before this task.')).toBeVisible();
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({
      project_id: 'tenant', job_id: 'job', chapter_ordinal: 12, scope_id: scopeID,
      job: { existing: 'unchanged', secret: '[REDACTED]' }, visible: { vars: { example: 'available' } }, redacted_paths: ['/job/secret'],
    }) });
    fireEvent.click(within(dialog).getByRole('switch'));
    const before = await within(dialog).findByRole('region', { name: 'Context before patching' });
    expect(before).toHaveTextContent('unchanged');
    expect(before).toHaveTextContent('/job/secret');
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Add line' })[0]);
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. git.author'), { target: { value: 'answer' } });
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. "james@example.com"'), { target: { value: 'patched' } });
    expect(before).toHaveTextContent('unchanged');
    expect(before).not.toHaveTextContent('patched');
    expect(within(dialog).queryByText('Full context is unavailable for this restart point.')).not.toBeInTheDocument();
    // Changing targets preserves edits, even if inspection fails. The old
    // snapshot must disappear and retry must inspect the newly selected chapter.
    fetchMock.mockResolvedValueOnce({ ok: false, status: 422, statusText: 'Unprocessable Entity', text: async () => 'History unavailable' });
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Restart point' }));
    fireEvent.click(await screen.findByText(text => text.startsWith('Recipe body · Chapter 4 ·')));
    expect(await within(dialog).findByText(/History unavailable/)).toBeVisible();
    expect(within(dialog).queryByRole('region', { name: 'Context before patching' })).not.toBeInTheDocument();
    expect(within(dialog).getByPlaceholderText('e.g. "james@example.com"')).toHaveValue('patched');
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({
      project_id: 'tenant', job_id: 'job', chapter_ordinal: 4, scope_id: 'root',
      job: { existing: 'earlier value' }, visible: {}, redacted_paths: [],
    }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Retry context' }));
    expect(await within(dialog).findByRole('region', { name: 'Context before patching' })).toHaveTextContent('earlier value');
    expect(within(dialog).getByPlaceholderText('e.g. "james@example.com"')).toHaveValue('patched');
    expect(within(dialog).queryByText('Patch Preview')).not.toBeInTheDocument();
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({ job_id: 'restarted' }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/jobs/job/restart'),
      expect.objectContaining({ method: 'POST', body: expect.stringContaining('"scope_id":"root"') }),
    ));
    const call = fetchMock.mock.calls.find(([url]) => String(url).endsWith('/jobs/job/restart'))!;
    expect(JSON.parse(call[1]!.body as string)).toEqual({ scope_id: 'root', goal: { mode: 'continue' }, request_id: expect.any(String), patch: { job: { answer: 'patched' } } });
    expect(screen.queryByRole('tab', { name: /^Attempt / })).not.toBeInTheDocument();
  });

  it('restarts from submission without carrying over a recipe context patch', async () => {
    const fetchMock = mount('', { ...story,
      root: { ...jobAttempt(1), restart_chapter_ordinal: 4 },
      restart_scopes: [
        { chapter_ordinal: 0, scope_id: 'opaque-submission-id', title: 'Job submission', started_at: '' },
        { chapter_ordinal: 4, scope_id: 'root', title: 'Recipe body', started_at: '' },
      ],
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({
      project_id: 'tenant', job_id: 'job', chapter_ordinal: 4, scope_id: 'root', job: {}, visible: {}, redacted_paths: [],
    }) });
    fireEvent.click(within(dialog).getByRole('switch'));
    await within(dialog).findByRole('region', { name: 'Context before patching' });
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Add line' })[0]);
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. git.author'), { target: { value: 'answer' } });
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. "james@example.com"'), { target: { value: 'patched' } });
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Restart point' }));
    fireEvent.click(await screen.findByText(text => text.startsWith('Job submission · Chapter 0 ·')));
    expect(within(dialog).getByRole('switch')).toBeDisabled();
    expect(within(dialog).getByRole('switch')).not.toBeChecked();
    expect(within(dialog).getByText('Submission context is read-only. No context patch will be applied.')).toBeVisible();
    expect(within(dialog).queryByPlaceholderText('e.g. git.author')).not.toBeInTheDocument();
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({ job_id: 'resubmitted' }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/jobs/job/restart'), expect.objectContaining({ method: 'POST' }),
    ));
    const call = fetchMock.mock.calls.find(([url]) => String(url).endsWith('/jobs/job/restart'))!;
    expect(JSON.parse(call[1]!.body as string)).toEqual({
      scope_id: 'opaque-submission-id', goal: { mode: 'continue' }, request_id: expect.any(String),
    });
  });

  it('does not offer restarts without recorded scopes', async () => {
    mount();
    await screen.findByText('Development.Design.Task 3.2');
    expect(screen.queryByRole('button', { name: 'Restart job' })).not.toBeInTheDocument();
  });

  it('shows c2j’s unavailable reason instead of inferring a target', async () => {
    mount('?nodeId=root', { ...story, root: {
      id: 'root', kind: 'recipeSourceResolution', title: 'Resolve recipe', status: 'failed', path: [],
      invocation_id: 'root/task:resolve', task_ordinal: 4, restart_unavailable_reason: 'Recipe source resolution has no recipe scope.',
    }, restart_scopes: [{ chapter_ordinal: 4, scope_id: 'root', started_at: '' }] });
    fireEvent.click(await screen.findByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('Recipe source resolution has no recipe scope.')).toBeVisible();
    expect(within(dialog).getByRole('button', { name: 'Restart', exact: true })).toBeDisabled();
  });

  it('uses the recorded historical chapter even when the current attempt has the same scope ID', async () => {
    const fetchMock = mount('?nodeId=jobAttempt%3A2%7Cn_3', { ...story,
      root: { ...jobAttempt(3), restart_chapter_ordinal: 30, past_attempts: [
        { ...jobAttempt(2), restart_chapter_ordinal: 14, restart_relationship: 'body' },
      ] },
      restart_scopes: [
        { chapter_ordinal: 30, scope_id: 'same-scope', title: 'Current body', started_at: '' },
        { chapter_ordinal: 14, scope_id: 'same-scope', title: 'Previous body', started_at: '' },
      ],
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/Previous body · Chapter 14/)).toBeVisible();
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({ job_id: 'new-job' }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/jobs/job/restart'),
      expect.objectContaining({ body: expect.stringContaining('"chapter_ordinal":14') }),
    ));
  });

  it('selects an exact scope in a compact chapter and reuses a restart request after a lost response', async () => {
    const fetch = mount('', { ...story,
      root: { ...jobAttempt(1), restart_chapter_ordinal: 4, restart_scope_id: 'target/scope' },
      restart_scopes: [
        { chapter_ordinal: 4, scope_id: 'other', title: 'Other scope', started_at: '' },
        { chapter_ordinal: 4, scope_id: 'target/scope', title: 'Selected scope', started_at: '' },
      ],
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/Selected scope · Chapter 4/)).toBeVisible();
    fetch.mockResolvedValueOnce({ ok: true, json: async () => ({ project_id: 'tenant', job_id: 'job', scope_id: 'target/scope', chapter_ordinal: 4, job: {}, visible: {}, redacted_paths: [] }) });
    fireEvent.click(within(dialog).getByRole('switch'));
    await within(dialog).findByRole('region', { name: 'Context before patching' });
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining('restart-context?scope_id=target%2Fscope'), expect.anything());
    fetch.mockRejectedValueOnce(new TypeError('Connection lost'));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Restart', exact: true })).toBeEnabled());
    const first = fetch.mock.calls.find(([url]) => String(url).endsWith('/restart'))!;
    expect(JSON.parse(first[1]!.body as string)).toMatchObject({ scope_id: 'target/scope', request_id: expect.any(String) });
    expect(JSON.parse(first[1]!.body as string)).not.toHaveProperty('chapter_ordinal');
    fetch.mockResolvedValueOnce({ ok: true, json: async () => ({ job_id: 'resumed' }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetch.mock.calls.filter(([url]) => String(url).endsWith('/restart'))).toHaveLength(2));
    const requests = fetch.mock.calls.filter(([url]) => String(url).endsWith('/restart'));
    expect(requests[1][1]!.body).toBe(requests[0][1]!.body);
  });

});

// Test the page action directly for lifecycle races, alongside its page integration.

function downloadBrowser() {
  const create = vi.fn().mockReturnValue('blob:chapters');
  const revoke = vi.fn();
  const BrowserURL = globalThis.URL;
  vi.stubGlobal('URL', class extends BrowserURL {
    static createObjectURL = create;
    static revokeObjectURL = revoke;
  });
  const clicked: HTMLAnchorElement[] = [];
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { clicked.push(this); });
  return { create, revoke, clicked, restore: () => click.mockRestore() };
}

describe('chapter download action', () => {
  it('is disabled without a job and exposes a labelled keyboard button with a download icon', () => {
    render(<DownloadChaptersButton projectId="tenant" jobId="" />);
    const button = screen.getByRole('button', { name: 'Download chapters' });
    expect(button).toBeDisabled();
    expect(button.querySelector('[aria-label="download"]')).not.toBeNull();
  });

  it('prevents duplicates, downloads a safe filename, cleans up, and allows another download', async () => {
    const browser = downloadBrowser();
    try {
      let resolve!: (response: unknown) => void;
      const fetch = vi.fn().mockImplementation(() => new Promise(done => { resolve = done; }));
      vi.stubGlobal('fetch', fetch);
      render(<DownloadChaptersButton projectId="tenant" jobId={'../雪\\bad\n'} />);
      const button = screen.getByRole('button', { name: 'Download chapters' });
      fireEvent.click(button);
      fireEvent.click(button);
      expect(fetch).toHaveBeenCalledTimes(1);
      expect(button).toHaveClass('ant-btn-loading');
      const blob = new Blob(['opaque export']);
      await act(async () => { resolve({ ok: true, blob: async () => blob }); });
      expect(browser.create).toHaveBeenCalledWith(blob);
      expect(browser.clicked).toHaveLength(1);
      expect(browser.clicked[0].download).toBe('job-雪_bad-chapters.json');
      expect(document.body.contains(browser.clicked[0])).toBe(false);
      await waitFor(() => expect(browser.revoke).toHaveBeenCalledWith('blob:chapters'));
      expect(button).not.toHaveClass('ant-btn-loading');
      fireEvent.click(button);
      expect(fetch).toHaveBeenCalledTimes(2);
    } finally { browser.restore(); }
  });

  it('preserves the story and retries after failure regardless of historical tree selection', async () => {
    const browser = downloadBrowser();
    try {
      let downloads = 0;
      const fetch = vi.fn().mockImplementation(async (url: string) => {
        if (url.endsWith('/chapters/download')) {
          downloads++;
          if (downloads === 1) return { ok: false, status: 500, statusText: 'Error', text: async () => 'reader failed' };
          return { ok: true, blob: async () => new Blob(['{}']) };
        }
        return { ok: true, json: async () => ({ ...story, status: 'running' }) };
      });
      vi.stubGlobal('fetch', fetch);
      render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}>
        <Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} /></Routes>
      </MemoryRouter>);
      await screen.findByText('Development.Design.Task 3.2');
      fireEvent.click(screen.getByRole('switch', { name: 'Auto-refresh' }));
      fireEvent.click(screen.getByRole('tab', { name: 'Job attempt 1' }));
      fireEvent.click(await screen.findByText('Development.Design.Task 1.2'));
      fireEvent.click(screen.getByRole('button', { name: 'Download chapters' }));
      expect(await screen.findByText('Could not download chapters')).toBeVisible();
      expect(screen.getAllByText('Development.Design.Task 1.2')[0]).toBeVisible();
      fireEvent.click(screen.getByRole('button', { name: 'Collapse all' }));
      expect(screen.queryByText('Failed to load job story')).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Download chapters' }));
      await waitFor(() => expect(browser.clicked).toHaveLength(1));
      await waitFor(() => expect(browser.revoke).toHaveBeenCalled());
      expect(screen.queryByText('Could not download chapters')).not.toBeInTheDocument();
      const urls = fetch.mock.calls.map(call => call[0] as string).filter(url => url.includes('/chapters/download'));
      expect(urls.map(url => new URL(url).pathname)).toEqual(['/api/projects/tenant/jobs/job/chapters/download', '/api/projects/tenant/jobs/job/chapters/download']);
    } finally { browser.restore(); }
  });

  it.each(['unmount', 'job', 'tenant'])('aborts on %s and suppresses stale successes and errors', async (change) => {
    const browser = downloadBrowser();
    try {
      for (const failure of [false, true]) {
        let settle!: (response: unknown) => void;
        let reject!: (reason: Error) => void;
        const fetch = vi.fn().mockImplementation(() => new Promise((resolve, fail) => { settle = resolve; reject = fail; }));
        vi.stubGlobal('fetch', fetch);
        const view = render(<DownloadChaptersButton key="tenant/job" projectId="tenant" jobId="job" />);
        fireEvent.click(screen.getByRole('button', { name: 'Download chapters' }));
        const signal = fetch.mock.calls[0][1].signal as AbortSignal;
        if (change === 'unmount') view.unmount();
        else view.rerender(<DownloadChaptersButton key={change === 'job' ? 'tenant/new' : 'new/job'} projectId={change === 'tenant' ? 'new' : 'tenant'} jobId={change === 'job' ? 'new' : 'job'} />);
        expect(signal.aborted).toBe(true);
        await act(async () => {
          if (failure) reject(new Error('stale failure'));
          else settle({ ok: true, blob: async () => new Blob(['stale']) });
        });
        expect(browser.create).not.toHaveBeenCalled();
        expect(screen.queryByText('Could not download chapters')).not.toBeInTheDocument();
        view.unmount();
      }
    } finally { browser.restore(); }
  });
});

function metadata(overrides: Partial<RecipeJob> = {}): RecipeJob {
  return { ...jobIdentity, tenant_id: 'tenant', job_id: 'job', status: 'COMPLETED', store: 'ARCHIVED',
    job_type: 'recipe', recipe: 'build', created_at: '', available_at: '', ...overrides };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
describe('job story context', () => {
  it.each(['ACTIVE', 'COMPLETED'] as const)('JC-001,003,010 displays literal canonical cell for %s', async status => {
    const cell_name = '<img src=x> 雪 self';
    vi.mocked(getJob).mockResolvedValue(metadata({ status, cell_name, title: 'Title', prompt: 'Prompt text' }));
    const fetchMock = mount();
    const cell = await screen.findByLabelText('Job cell');
    expect(cell.textContent).toBe(`Cell: ${cell_name}`);
    const heading = screen.getByRole('heading', { name: 'Job Story: Title' });
    const prompt = screen.getByRole('region', { name: 'Job prompt' });
    expect(heading.compareDocumentPosition(cell) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(cell.compareDocumentPosition(prompt) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(cell.querySelector('img')).toBeNull();
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/cells'))).toBe(false);
  });
  it.each(['cell_id', 'cell_name', 'repo'].flatMap(field => [undefined, '  '].map(value => ({ field, value }))))(
    'JC-009 rejects invalid $field=$value through real API validation', async ({ field, value }) => {
      const actual = await vi.importActual<typeof import('@colony2/shared/api')>('@colony2/shared/api');
      vi.mocked(getJob).mockImplementation(actual.getJob);
      const invalid = metadata({ [field]: value });
      vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(JSON.stringify(url.endsWith('/story') ? story : invalid))));
      renderContext(false);
      await screen.findByText('Job details unavailable');
      expect(screen.getByText(new RegExp(`required ${field}`))).toBeVisible();
      expect(screen.queryByLabelText('Job cell')).toBeNull();
      expect(screen.queryByText('Unknown cell')).toBeNull();
    });
  it('shows normalized title and literal full prompt below the heading', async () => {
    const prompt = '<img src=x onerror=alert(1)>\n雪 ' + 'long '.repeat(150);
    vi.mocked(getJob).mockResolvedValue(metadata({ title: '<Title>', metadata: { title: 'wrong' }, prompt }));
    const { container } = renderContext();
    const heading = await screen.findByRole('heading', { level: 2, name: 'Job Story: <Title>' });
    const region = screen.getByRole('region', { name: 'Job prompt' });
    expect(region.querySelector('div.ant-typography')?.textContent).toBe(prompt);
    expect(heading.compareDocumentPosition(region) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(container.querySelector('img')).toBeNull();
  });
  it.each([undefined, '', '  \n '])('shows an empty prompt: %s', async value => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: value, prompt: value }));
    mount();
    await screen.findByText('Development.Design.Task 3.2');
    expect(screen.getByRole('heading', { name: 'Job Story', exact: true })).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent(`Cell: ${jobIdentity.cell_name}`); // JC-002
    expect(screen.getByRole('region', { name: 'Job prompt' })).toHaveTextContent('No prompt recorded for this job.');
  });
  it('does not manufacture a prompt from other context', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Title is not a prompt', parent: { job_id: 'parent' }, metadata: { prompt: 'Wrong nested prompt' } }));
    mount('?input=true');
    const region = await screen.findByRole('region', { name: 'Job prompt' });
    expect(region).toHaveTextContent('No prompt recorded for this job.');
    expect(region).not.toHaveTextContent('Title is not a prompt');
    expect(region).not.toHaveTextContent('Wrong nested prompt');
    fireEvent.click(await screen.findByText('Development.Design.Task 3.2'));
    expect(region).toHaveTextContent('No prompt recorded for this job.');
  });
  it.each(['failed_app', 'failed_system', 'failed_timeout'].flatMap(completion_status => [
    { completion_status, status: 'READY' as const, store: 'ARCHIVED' as const },
    { completion_status, status: 'COMPLETED' as const, store: 'ACTIVE' as const },
  ]))('shows terminal $completion_status for $status/$store jobs', async ({ completion_status, status, store }) => {
    vi.mocked(getJob).mockResolvedValue(metadata({ status, store, completion_status,
      completion_detail: '  <script>bad()</script> & 雪\n\tsecond line' }));
    mount('', { ...story, root: null });
    const message = await screen.findByText(completion_status === 'failed_timeout' ? 'Job timed out' : 'Job failed');
    expect(message.closest('[role="alert"]')?.textContent).toContain('  <script>bad()</script> & 雪\n\tsecond line');
    expect(document.querySelector('script')).toBeNull();
  });
  it.each(['failed_app', 'failed_system', 'failed_timeout'].flatMap(completion_status =>
    [undefined, '', '  \n '].map(completion_detail => ({ completion_status, completion_detail }))))(
    'shows terminal failures without details: %j', async overrides => {
      vi.mocked(getJob).mockResolvedValue(metadata(overrides));
      mount();
      expect(await screen.findByText('No failure details recorded for this job.')).toBeVisible();
      expect(screen.getByText(overrides.completion_status === 'failed_timeout' ? 'Job timed out' : 'Job failed')).toBeVisible();
    });
  it.each([
    { completion_status: 'success' }, { completion_status: 'cancelled' }, {},
    { status: 'ACTIVE', store: 'ACTIVE', completion_status: 'failed_app' },
    { completion_status: 'future_outcome' },
  ])('does not invent failures for %j', async overrides => {
    vi.mocked(getJob).mockResolvedValue(metadata({ completion_detail: 'retained failure', ...overrides } as Partial<RecipeJob>));
    mount();
    await screen.findByText('Development.Design.Task 3.2');
    expect(screen.queryByText('Job failed')).toBeNull();
    fireEvent.click(screen.getByText('Development.Design.Task 3.2'));
    expect(await screen.findByText('Failure 3.2: temporary directory missing')).toBeVisible();
  });
  it('keeps context independent of historical task selection', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ cell_name: 'owning-cell', parent: { job_id: 'parent', cell_name: 'parent-cell' }, metadata: { workspace_cell: 'workspace-cell' }, title: 'Current', prompt: 'Current prompt', completion_status: 'failed_app', completion_detail: 'Current failure' }));
    mount();
    fireEvent.click(await screen.findByRole('tab', { name: 'Job attempt 1' }));
    fireEvent.click(screen.getByText('Development.Design.Task 1.2'));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    expect(screen.getByText('Current failure')).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent('Cell: owning-cell'); // JC-004
    expect(screen.getByRole('heading', { name: 'Job Story: Current' })).toBeVisible();
  });
  it('loads metadata despite navigation goal state and preserves its precedence', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ goal_id: 'other', title: 'Loaded' }));
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
    render(<MemoryRouter initialEntries={[{ pathname: '/project/tenant/jobs/job/story', state: { goalId: 'navigation' } }]}>
      <Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} /></Routes>
    </MemoryRouter>);
    await screen.findByRole('heading', { name: 'Job Story: Loaded' });
    expect(screen.getByRole('link', { name: 'Goal overview' })).toHaveAttribute('href', '/project/tenant/goals/navigation');
  });
  it('renders story while metadata is pending and recovers metadata after failure', async () => {
    const pending = deferred<RecipeJob>();
    vi.mocked(getJob).mockReturnValueOnce(pending.promise);
    mount();
    await screen.findByText('Development.Design.Task 3.2');
    expect(screen.getByRole('status', { name: 'Job details loading' })).toHaveTextContent('Loading job details...');
    expect(screen.queryByText('No prompt recorded for this job.')).toBeNull();
    expect(screen.queryByLabelText('Job cell')).toBeNull(); // JC-005
    await act(async () => { pending.reject(new Error('metadata unavailable')); });
    expect(screen.getByText('Job details unavailable')).toBeVisible();
    expect(screen.getByText('Cell, prompt, and completion information could not be loaded.')).toBeVisible();
    expect(screen.queryByLabelText('Job cell')).toBeNull();
    expect(screen.getByRole('heading', { name: 'Job Story' })).toBeVisible();
    expect(screen.queryByText('Failed to load job story')).toBeNull();
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Recovered', prompt: 'New prompt', completion_status: 'failed_system', completion_detail: 'New detail' }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await screen.findByRole('heading', { name: 'Job Story: Recovered' });
    expect(screen.getByText('New prompt')).toBeVisible();
    expect(screen.getByText('New detail')).toBeVisible();
    expect(screen.queryByText('Job details unavailable')).toBeNull();
  });
  it('renders metadata while story is pending and keeps story errors separate', async () => {
    const pending = deferred<Response>();
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Independent' }));
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(pending.promise));
    renderContext(false);
    await screen.findByRole('heading', { name: 'Job Story: Independent' });
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    await act(async () => { pending.reject(new Error('transport failure')); });
    expect(await screen.findByText('Failed to load job story')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible();
    expect(screen.queryByText('Job failed')).toBeNull();
    log.mockRestore();
  });
  it.each(['success', 'failure'])('ignores an older refresh %s after a newer response', async outcome => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Initial' }));
    mount();
    await screen.findByRole('heading', { name: 'Job Story: Initial' });
    const old = deferred<RecipeJob>();
    vi.mocked(getJob).mockReturnValueOnce(old.promise).mockResolvedValue(metadata({ title: 'Newest', cell_name: 'newest-cell' }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await screen.findByRole('heading', { name: 'Job Story: Newest' });
    await act(async () => { if (outcome === 'success') old.resolve(metadata({ title: 'Obsolete', cell_name: 'obsolete-cell' })); else old.reject(new Error('obsolete')); });
    expect(screen.getByRole('heading', { name: 'Job Story: Newest' })).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent('Cell: newest-cell'); // JC-008
  });
  it('retries only metadata and escapes its diagnostic', async () => {
    const diagnostic = '<img src=x onerror=alert(1)> 雪\n second line';
    vi.mocked(getJob).mockRejectedValueOnce(new Error(diagnostic));
    const fetchMock = mount();
    expect(await screen.findByText('Job details unavailable')).toBeVisible();
    expect(screen.getByText(/<img src=x/).textContent).toBe(diagnostic);
    expect(document.querySelector('img')).toBeNull();
    const reads = fetchMock.mock.calls.length;
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Recovered', prompt: 'Recovered prompt', completion_status: 'failed_app', completion_detail: 'Recovered diagnostic' }));
    fireEvent.click(screen.getByRole('button', { name: 'Retry', exact: true }));
    expect(await screen.findByText('Recovered prompt')).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent(`Cell: ${jobIdentity.cell_name}`); // JC-006
    expect(screen.getByText('Recovered diagnostic')).toBeVisible();
    expect(getJob).toHaveBeenLastCalledWith('tenant', 'job');
    expect(fetchMock.mock.calls.length).toBe(reads);
    expect(screen.queryByText('Job details unavailable')).toBeNull();
  });
  it('clears context after a failed refresh and recovers with Refresh', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Old', prompt: 'Old prompt', completion_status: 'failed_app', completion_detail: 'Old diagnostic' }));
    mount();
    await screen.findByText('Old prompt');
    vi.mocked(getJob).mockRejectedValueOnce(new Error('refresh failed'));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await screen.findByText('Job details unavailable');
    expect(screen.queryByText('Old prompt')).toBeNull();
    expect(screen.queryByLabelText('Job cell')).toBeNull(); // JC-007,008
    expect(screen.queryByText('Old diagnostic')).toBeNull();
    expect(screen.getByRole('heading', { name: 'Job Story', exact: true })).toBeVisible();
    vi.mocked(getJob).mockResolvedValue(metadata({ prompt: 'Fresh prompt' }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    expect(await screen.findByText('Fresh prompt')).toBeVisible();
    expect(screen.queryByText('Job details unavailable')).toBeNull();
  });
  it('keeps simultaneous metadata and story errors independently retryable', async () => {
    vi.mocked(getJob).mockRejectedValueOnce(new Error('metadata failed'));
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('story failed')));
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    renderContext(false);
    await screen.findByText('Job details unavailable');
    await screen.findByText('Failed to load job story');
    vi.mocked(getJob).mockResolvedValue(metadata({ prompt: 'Loaded prompt' }));
    fireEvent.click(screen.getByText('Job details unavailable').closest('[role="alert"]')!.querySelector('button')!);
    expect(await screen.findByText('Loaded prompt')).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent(`Cell: ${jobIdentity.cell_name}`); // JC-006
    expect(screen.getByText('Failed to load job story')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry', exact: true })).toBeVisible();
    log.mockRestore();
  });
  it.each(['success', 'failure'])('ignores obsolete %s after a newer metadata failure', async outcome => {
    vi.mocked(getJob).mockResolvedValue(metadata({ prompt: 'Initial' }));
    mount();
    await screen.findByText('Initial');
    const old = deferred<RecipeJob>();
    vi.mocked(getJob).mockReturnValueOnce(old.promise).mockRejectedValueOnce(new Error('new failure'));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await screen.findByText('new failure');
    await act(async () => { if (outcome === 'success') old.resolve(metadata({ prompt: 'Obsolete' })); else old.reject(new Error('old failure')); });
    expect(screen.getByText('new failure')).toBeVisible();
    expect(screen.queryByText('Obsolete')).toBeNull();
    expect(screen.queryByText('old failure')).toBeNull();
  });
  it('retains final metadata after terminal story polling stops', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    vi.mocked(getJob).mockResolvedValue(metadata({ status: 'ACTIVE', store: 'ACTIVE' }));
    const fetchMock = mount('', { ...story, status: 'running' });
    await screen.findByRole('switch', { name: 'Auto-refresh' });
    const final = deferred<RecipeJob>();
    vi.mocked(getJob).mockReturnValueOnce(final.promise);
    fetchMock.mockResolvedValue({ ok: true, json: async () => story });
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(screen.queryByRole('switch', { name: 'Auto-refresh' })).toBeNull();
    await act(async () => { final.resolve(metadata({ status: 'COMPLETED', store: 'ACTIVE', completion_status: 'failed_app', completion_detail: 'Final diagnostic' })); });
    expect(await screen.findByText('Final diagnostic')).toBeVisible();
  });
  it.each(['job', 'tenant'])('clears metadata warnings immediately across %s navigation', async change => {
    vi.mocked(getJob).mockRejectedValueOnce(new Error('Old warning'));
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
    const current = deferred<RecipeJob>();
    let navigate!: ReturnType<typeof useNavigate>;
    function Nav() { navigate = useNavigate(); return null; }
    function Host() { return <JobStoryPage projectId={useLocation().pathname.split('/')[2]} />; }
    render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}><Nav /><Routes>
      <Route path="/project/:projectId/jobs/:jobId/story" element={<Host />} />
    </Routes></MemoryRouter>);
    await screen.findByText('Old warning');
    vi.mocked(getJob).mockReturnValueOnce(current.promise);
    act(() => navigate(change === 'job' ? '/project/tenant/jobs/new/story' : '/project/other/jobs/job/story'));
    expect(screen.queryByText('Old warning')).toBeNull();
    expect(screen.queryByText('Job details unavailable')).toBeNull();
    expect(screen.getByRole('status', { name: 'Job details loading' })).toBeVisible();
    await act(async () => { current.resolve(metadata()); });
    expect(screen.getByText('No prompt recorded for this job.')).toBeVisible();
  });
  it.each(['job', 'tenant'].flatMap(change => ['success', 'failure'].map(outcome => ({ change, outcome }))))('isolates context across $change changes and late $outcome', async ({ change, outcome }) => {
    const old = deferred<RecipeJob>();
    vi.mocked(getJob).mockResolvedValueOnce(metadata({ title: 'Old', prompt: 'Old prompt' })).mockReturnValueOnce(old.promise);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
    function Host() {
      const location = useLocation();
      const tenant = location.pathname.split('/')[2];
      return <JobStoryPage projectId={tenant} />;
    }
    let navigate!: ReturnType<typeof import('react-router-dom')['useNavigate']>;
    function Nav() { navigate = useNavigate(); return null; }
    render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}><Nav /><Routes>
      <Route path="/project/:projectId/jobs/:jobId/story" element={<Host />} />
    </Routes></MemoryRouter>);
    await screen.findByRole('heading', { name: 'Job Story: Old' });
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    const current = deferred<RecipeJob>();
    vi.mocked(getJob).mockReturnValueOnce(current.promise);
    act(() => navigate(change === 'job' ? '/project/tenant/jobs/new/story' : '/project/other/jobs/job/story'));
    expect(screen.queryByText('Old prompt')).toBeNull();
    expect(screen.queryByLabelText('Job cell')).toBeNull(); // JC-007,008
    await act(async () => { if (outcome === 'success') old.resolve(metadata({ title: 'Obsolete', prompt: 'Leaked', cell_name: 'leaked-cell' })); else old.reject(new Error('Leaked warning')); });
    expect(screen.queryByText('Leaked warning')).toBeNull();
    expect(screen.queryByText('Leaked')).toBeNull();
    expect(screen.queryByLabelText('Job cell')).toBeNull(); // JC-007
    await act(async () => { current.resolve(metadata({ title: 'Current', cell_name: 'current-cell' })); });
    expect(screen.getByRole('heading', { name: 'Job Story: Current' })).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent('Cell: current-cell');
  });
});
function renderContext(stub = true) {
  if (stub) vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
  return render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}>
    <Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} /></Routes>
  </MemoryRouter>);
}

describe('SAV story artifact contents', () => {
  const artifacts = [{ taskOrdinal: 7, name: 'design.md' }, { taskOrdinal: 8, name: '雪/data.bin' }];
  const artifactStory = { ...story, root: { id: 'root', kind: 'opStep', title: 'Artifacts', status: 'success', path: [], artifact_keys: artifacts } };
  async function open(body: string | Uint8Array, name = 'design.md') {
    const fetchMock = mount('?nodeId=root', artifactStory);
    fetchMock.mockImplementation(async (url: string) => url.includes('/artifacts/') ? new Response(body) : { ok: true, json: async () => artifactStory });
    fireEvent.click(await screen.findByRole('tab', { name: 'Artifacts (2)' }));
    fireEvent.click(screen.getByRole('link', { name, exact: true }));
    return fetchMock;
  }
  it('SAV-001,003 renders shared Markdown and source without refetching', async () => {
    const fetchMock = await open('# Story contents\n\n**bold**');
    await screen.findByRole('heading', { name: 'Story contents' });
    const calls = fetchMock.mock.calls.length;
    fireEvent.click(screen.getByRole('radio', { name: 'Markdown', exact: true }));
    expect(screen.getByLabelText('Markdown source').textContent).toBe('# Story contents\n\n**bold**');
    expect(fetchMock.mock.calls.length).toBe(calls);
  });
  it('SAV-002,010 renders raw binary bytes and retains encoded download identity', async () => {
    const fetchMock = await open(new Uint8Array([255, 0, 254, 65]), '雪/data.bin');
    expect(await screen.findByLabelText('Binary bytes')).toHaveTextContent('ff 00 fe 41');
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/tasks/8/artifacts/%E9%9B%AA%2Fdata.bin'), expect.anything());
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function(this: HTMLAnchorElement) {
      expect(this.download).toBe('雪/data.bin');
      expect(this.href).toContain('/tasks/8/artifacts/%E9%9B%AA%2Fdata.bin');
    });
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /Download/ }));
    expect(click).toHaveBeenCalled(); click.mockRestore();
  });
  it('SAV-009 displays an empty artifact', async () => {
    await open(''); await screen.findByText('Empty document');
  });
  it('SAV-020 retries inline errors without closing the modal', async () => {
    const fetchMock = await open('# recovered');
    await screen.findByRole('heading', { name: 'recovered' });
    fireEvent.click(within(screen.getByRole('dialog')).getAllByRole('button', { name: 'Close', exact: true }).at(-1)!);
    fetchMock.mockImplementation(async (url: string) => url.includes('/artifacts/') ? { ok: false } : { ok: true, json: async () => artifactStory });
    fireEvent.click(screen.getByRole('link', { name: 'design.md' }));
    await screen.findByText(/Could not load this document/);
    expect(within(screen.getByRole('dialog')).getByRole('button', { name: /Download/ })).toBeEnabled();
    fetchMock.mockImplementation(async () => new Response('# retried'));
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await screen.findByRole('heading', { name: 'retried' });
  });
  it('SAV-021,022 aborts closed loads and reopens with fresh state', async () => {
    const fetchMock = await open('# initial');
    await screen.findByRole('heading', { name: 'initial' });
    fireEvent.click(within(screen.getByRole('dialog')).getAllByRole('button', { name: 'Close', exact: true }).at(-1)!);
    let resolve!: (r: Response) => void;
    fetchMock.mockImplementation(() => new Promise(r => { resolve = r; }));
    fireEvent.click(screen.getByRole('link', { name: 'design.md' }));
    await screen.findByLabelText('Loading artifact');
    const signal = (fetchMock.mock.calls.at(-1) as unknown as [string, {signal: AbortSignal}])[1].signal;
    fireEvent.click(within(screen.getByRole('dialog')).getAllByRole('button', { name: 'Close', exact: true }).at(-1)!);
    expect(signal.aborted).toBe(true);
    await act(async () => resolve(new Response('# stale')));
    expect(screen.queryByRole('heading', { name: 'stale' })).not.toBeInTheDocument();
    fetchMock.mockImplementation(async () => new Response('# fresh'));
    fireEvent.click(screen.getByRole('link', { name: 'design.md' }));
    await screen.findByRole('heading', { name: 'fresh' });
  });
});

describe('SAV rich story formats and attachment boundaries', () => {
  const keys = [{name:'notes.md',taskOrdinal:1},{name:'image.png',taskOrdinal:2},{name:'duplicate.png',taskOrdinal:3},{name:'duplicate.png',taskOrdinal:4}];
  const node = { id:'root',kind:'opStep',title:'Contents',status:'success',path:[],artifact_keys:keys };
  async function start(name: string, content: string | Uint8Array) {
    const current = {...story,root:{...node,artifact_keys:[{name,taskOrdinal:1},...keys.slice(1)]}};
    const mock = mount('?nodeId=root',current);
    mock.mockImplementation(async (url:string) => url.includes('/artifacts/') ? new Response(content) : {ok:true,json:async()=>current});
    fireEvent.click(await screen.findByRole('tab',{name:'Artifacts (4)'}));
    fireEvent.click(screen.getByRole('link',{name,exact:true}));
    return mock;
  }
  it.each([
    ['data.json','{"n":9007199254740993}','Formatted JSON'],
    ['data.json','{"n":bad}', 'JSON preview unavailable'],
    ['data.jsonl','{}\nbad','Line 2: invalid JSON'],
    ['file.patch','--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n','Unified'],
    ['index.html','<script>bad()</script>','HTML and SVG are shown as source only'],
    ['data.zip','PK\x03\x04','Archives are not extracted'],
  ])('SAV-004,005,006,011 shared %s behavior',async(name,content,expected)=>{
    await start(name,content);
    expect((await screen.findAllByText(new RegExp(expected)))[0]).toBeInTheDocument();
  });
  it('SAV-012 truncates streamed text and retains original download',async()=>{
    await start('large.log','a'.repeat(2*1024*1024+1));
    await screen.findByText(/Preview truncated/);
    expect(within(screen.getByRole('dialog')).getByRole('button',{name:/Download/})).toBeEnabled();
  });
  it('SAV-016,017 loads only unique same-node attachments',async()=>{
    const OriginalURL = URL;
    vi.stubGlobal('URL', class extends OriginalURL { static createObjectURL = vi.fn(() => 'blob:test'); static revokeObjectURL = vi.fn(); });
    const mock = await start('notes.md','![valid](image.png)\n![duplicate](duplicate.png)\n![missing](outside.png)\n![external](https://evil.test/a.png)\n![absolute](/image.png)');
    const png = new Uint8Array(24); png.set([137,80,78,71]); const view=new DataView(png.buffer); view.setUint32(16,2);view.setUint32(20,2);
    mock.mockImplementation(async(url:string)=>new Response(url.includes('image.png')?png:'unexpected'));
    const attachedImage = await screen.findByRole('img',{name:'image.png'});
    fireEvent.click(screen.getByRole('button', {name:'Actual size'}));
    expect(attachedImage).toHaveAttribute('width', '2');
    fireEvent.error(attachedImage);
    expect(screen.getByText(/Could not decode this image/)).toBeInTheDocument();
    expect(within(screen.getByRole('dialog')).getByRole('button', {name:/Download/})).toBeEnabled();
    expect(screen.getAllByText(/Blocked external or ambiguous image/)).toHaveLength(4);
    const requests=mock.mock.calls.map(c=>String(c[0])).filter(u=>u.includes('/artifacts/'));
    expect(requests).toHaveLength(2);
    expect(requests[1]).toContain('/tasks/2/artifacts/image.png');
  });
  it('SAV-018 retains the opened attachment snapshot after refresh',async()=>{
    const mock = await start('notes.md','![missing](new.png)');
    await screen.findByText(/Blocked external or ambiguous image/);
    mock.mockImplementation(async()=>({ok:true,json:async()=>({...story,root:{...node,artifact_keys:[...keys,{name:'new.png',taskOrdinal:9}]}})}));
    fireEvent.click(screen.getByRole('button',{name:/Refresh$/,exact:true}));
    await screen.findByRole('tab', {name:'Artifacts (5)'});
    expect(screen.getByText(/Blocked external or ambiguous image/)).toBeInTheDocument();
    expect(mock.mock.calls.filter(c=>String(c[0]).includes('/artifacts/'))).toHaveLength(1);
  });
});

describe('SAV-021 route identity cancellation', () => {
  it.each(['/project/other/jobs/job/story?nodeId=root', '/project/tenant/jobs/other/story?nodeId=root'])('aborts an old artifact on %s', async destination => {
    let navigate!: ReturnType<typeof useNavigate>;
    let resolve!: (r:Response)=>void;
    let signal!: AbortSignal;
    const current = {...story,root:{id:'root',kind:'opStep',title:'Contents',status:'success',path:[],artifact_keys:[{name:'notes.md',taskOrdinal:1}]}};
    vi.stubGlobal('fetch',vi.fn((url:string,options?:{signal:AbortSignal})=>{
      if(url.includes('/artifacts/')) {signal=options!.signal;return new Promise(r=>{resolve=r;});}
      return Promise.resolve({ok:true,json:async()=>current});
    }));
    function Host(){navigate=useNavigate();const project=useLocation().pathname.split('/')[2];return <JobStoryPage projectId={project}/>;}
    render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story?nodeId=root']}><Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<Host/>}/></Routes></MemoryRouter>);
    fireEvent.click(await screen.findByRole('tab',{name:'Artifacts (1)'}));
    fireEvent.click(screen.getByRole('link',{name:'notes.md'}));
    await screen.findByLabelText('Loading artifact');
    await act(async()=>navigate(destination));
    expect(signal.aborted).toBe(true);
    await act(async()=>resolve(new Response('# stale route')));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading',{name:'stale route'})).not.toBeInTheDocument();
  });
});

describe('MAL node-local navigation', () => {
  const keys=[{name:'docs/bar.md',taskOrdinal:1},{name:'docs/foo.md',taskOrdinal:2},{name:'docs/data.json',taskOrdinal:3},{name:'dup.md',taskOrdinal:4},{name:'dup.md',taskOrdinal:5}];
  const current={...story,root:{id:'root',kind:'opStep',title:'Links',status:'success',path:[],artifact_keys:keys,children:[{id:'foreign',kind:'opStep',title:'Other node',status:'success',path:[],artifact_keys:[{name:'docs/foo.md',taskOrdinal:999},{name:'docs/new.md',taskOrdinal:998}]}]}};
  async function start() {
    let refreshed=false;
    const mock=mount('?nodeId=root',current);
    mock.mockImplementation(async(url:string)=>{
      if(url.includes('/artifacts/')) return new Response(url.includes('bar.md')?'# Bar\n[next](foo.md#target) [ambiguous](../dup.md)':url.includes('foo.md')?'# Target\n[self](foo.md#target) [outside](new.md) [json](data.json)':'{"n":42}');
      return {ok:true,json:async()=>refreshed?{...current,root:{...current.root,artifact_keys:[...keys,{name:'docs/new.md',taskOrdinal:9}]}}:current};
    });
    fireEvent.click(await screen.findByRole('tab',{name:'Artifacts (5)'}));
    fireEvent.click(screen.getByRole('link',{name:'docs/bar.md'}));
    await screen.findByRole('heading',{name:'Bar'});
    return {mock,refresh:()=>{refreshed=true;}};
  }
  it('MAL-017,018,019,021,022,023,028,031 keeps the original snapshot through refresh and chained navigation', async()=>{
    const {mock,refresh}=await start();
    fireEvent.click(screen.getByRole('button',{name:'ambiguous'}));expect(within(screen.getByRole('dialog')).getByRole('status')).toHaveTextContent('ambiguous');
    refresh();fireEvent.click(screen.getByRole('button',{name:/Refresh$/,exact:true}));await screen.findByRole('tab',{name:'Artifacts (6)'});
    fireEvent.click(screen.getByText('Other node', {selector:'strong'}));await screen.findByRole('tab',{name:'Artifacts (2)'});
    fireEvent.click(screen.getByRole('button',{name:'next'}));await screen.findByRole('heading',{name:'Target'});
    expect(within(screen.getByRole('dialog')).getByText('docs/foo.md')).toBeInTheDocument();
    const count=mock.mock.calls.filter(c=>String(c[0]).includes('/artifacts/')).length;
    fireEvent.click(screen.getByRole('button',{name:'self'}));fireEvent.click(screen.getByRole('button',{name:'outside'}));
    expect(within(screen.getByRole('dialog')).getByRole('status')).toHaveTextContent('missing');
    expect(mock.mock.calls.filter(c=>String(c[0]).includes('/artifacts/'))).toHaveLength(count);
    const click=vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(function(this:HTMLAnchorElement){expect(this.href).toContain('/tasks/2/artifacts/docs%2Ffoo.md');expect(this.download).toBe('docs/foo.md');});
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button',{name:/Download/}));expect(click).toHaveBeenCalled();click.mockRestore();
    fireEvent.click(screen.getByRole('button',{name:'json'}));await screen.findByLabelText('Formatted JSON');
    expect(mock.mock.calls.filter(c=>String(c[0]).includes('/artifacts/')).map(c=>c[0])).toEqual([
      expect.stringContaining('/tasks/1/artifacts/docs%2Fbar.md'),expect.stringContaining('/tasks/2/artifacts/docs%2Ffoo.md'),expect.stringContaining('/tasks/3/artifacts/docs%2Fdata.json')]);
  });
  it('MAL-033 retries a linked target without closing the modal',async()=>{
    const {mock}=await start();let recovered=false;
    mock.mockImplementation(async()=>new Response(recovered?'# Recovered':'bad',{status:recovered?200:503}));
    fireEvent.click(screen.getByRole('button',{name:'next'}));await screen.findByText(/Could not load/);
    expect(screen.getByRole('dialog')).toHaveTextContent('docs/foo.md');
    recovered=true;fireEvent.click(screen.getByRole('button',{name:'Retry'}));await screen.findByRole('heading',{name:'Recovered'});
    expect(mock.mock.calls.at(-1)?.[0]).toContain('/tasks/2/artifacts/docs%2Ffoo.md');
  });
});

it('keeps story cancellation confirmed by default and retries only after an explicit action', async () => {
  const response = { ...story, status: 'running' };
  const transport = mount('', response);
  const changes: string[] = [];
  let fail = true;
  transport.mockImplementation(async (input: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      changes.push(new URL(input, 'http://localhost').pathname);
      return fail
        ? { ok: false, status: 503, text: async () => 'Cancellation unavailable' }
        : { ok: true, status: 204 };
    }
    return { ok: true, json: async () => response };
  });
  const trigger = await screen.findByRole('button', { name: 'Cancel job', exact: true });
  fireEvent.click(trigger);
  let dialog = screen.getByRole('dialog');
  expect(changes).toEqual([]);
  fireEvent.click(within(dialog).getByRole('button', { name: 'Keep jobs', exact: true }));
  expect(changes).toEqual([]);
  fireEvent.click(trigger);
  dialog = screen.getByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel job', exact: true }));
  await within(dialog).findByText(/Cancellation unavailable/);
  await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Cancel job', exact: true })).toBeEnabled());
  expect(changes).toEqual(['/api/projects/tenant/jobs/job/cancel']);
  fail = false;
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel job', exact: true }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(changes).toEqual(['/api/projects/tenant/jobs/job/cancel', '/api/projects/tenant/jobs/job/cancel']);
  expect(trigger).toBeDisabled();
});

it('REL-001,010,030,031 keeps URL relationships through past-attempt selection and child loading failure', async () => {
  vi.mocked(getJob).mockResolvedValue({ ...jobIdentity, tenant_id:'tenant', job_id:'job', goal_id:'goal', parent:{job_id:'parent',goal_id:'parent-goal'} } as RecipeJob);
  vi.mocked(listJobChildren).mockReset().mockRejectedValueOnce(new Error('offline'));
  mount();
  fireEvent.click(await screen.findByText('Related jobs and goals', {selector:'summary'}));
  await screen.findByText('Child jobs unavailable');
  expect(screen.getByRole('link',{name:'Job · parent'})).toHaveAttribute('href','/project/tenant/jobs/parent/story');
  expect(screen.getByRole('link',{name:'Goal · parent-goal'})).toHaveAttribute('href','/project/tenant/goals/parent-goal');
  fireEvent.click(screen.getByRole('button',{name:'Job attempts'}));
  fireEvent.click(screen.getByRole('tab',{name:'Job attempt 1'}));
  fireEvent.click(await screen.findByText('Development.Design.Task 1.2'));
  await screen.findByText('Failure 1.2: temporary directory missing');
  expect(listJobChildren).toHaveBeenCalledTimes(1);
  expect(listJobChildren).toHaveBeenCalledWith('tenant','job',{pageSize:20,pageToken:undefined});
  expect(getJob).toHaveBeenCalledTimes(1);
  expect(screen.getByRole('link',{name:'Job · parent'})).toBeVisible();
});
