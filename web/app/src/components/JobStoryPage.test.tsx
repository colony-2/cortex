import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import JobStoryPage from './JobStoryPage';
import { getJob } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import DownloadChaptersButton from './DownloadChaptersButton';

vi.mock('@colony2/shared', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  useInputActivity: () => ({ pendingInputs: [], refresh: vi.fn() }),
}));
vi.mock('@colony2/shared/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared/api')>(),
  getJob: vi.fn().mockResolvedValue({ goal_id: 'goal' }),
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
beforeEach(() => { vi.mocked(getJob).mockReset().mockResolvedValue({ goal_id: 'goal' } as RecipeJob); });
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

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

    await act(async () => { fireEvent.click(screen.getByRole('button', { name: /Refresh/ })); });
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
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
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
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Refresh/ })).not.toHaveClass('ant-btn-loading'));
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
  return { tenant_id: 'tenant', job_id: 'job', status: 'COMPLETED', store: 'ARCHIVED',
    job_type: 'recipe', recipe: 'build', created_at: '', available_at: '', ...overrides };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
describe('job story context', () => {
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
  it.each([undefined, '', '  \n '])('omits blank context: %s', async value => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: value, prompt: value }));
    mount();
    await screen.findByText('Development.Design.Task 3.2');
    expect(screen.getByRole('heading', { name: 'Job Story', exact: true })).toBeVisible();
    expect(screen.queryByRole('region', { name: 'Job prompt' })).toBeNull();
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
  it.each([
    { completion_status: 'success' }, { completion_status: 'cancelled' }, {},
    { status: 'ACTIVE', store: 'ACTIVE', completion_status: 'failed_app' },
    ...[undefined, '', '  '].map(completion_detail => ({ completion_status: 'failed_app', completion_detail })),
  ])('does not invent failures for %j', async overrides => {
    vi.mocked(getJob).mockResolvedValue(metadata({ completion_detail: 'retained failure', ...overrides } as Partial<RecipeJob>));
    mount();
    await screen.findByText('Development.Design.Task 3.2');
    expect(screen.queryByText('Job failed')).toBeNull();
    fireEvent.click(screen.getByText('Development.Design.Task 3.2'));
    expect(await screen.findByText('Failure 3.2: temporary directory missing')).toBeVisible();
  });
  it('keeps context independent of historical task selection', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Current', prompt: 'Current prompt', completion_status: 'failed_app', completion_detail: 'Current failure' }));
    mount();
    fireEvent.click(await screen.findByRole('tab', { name: 'Job attempt 1' }));
    fireEvent.click(screen.getByText('Development.Design.Task 1.2'));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    expect(screen.getByText('Current failure')).toBeVisible();
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
    await act(async () => { pending.reject(new Error('metadata unavailable')); });
    expect(screen.getByRole('heading', { name: 'Job Story' })).toBeVisible();
    expect(screen.queryByText('Failed to load job story')).toBeNull();
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Recovered', prompt: 'New prompt', completion_status: 'failed_system', completion_detail: 'New detail' }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await screen.findByRole('heading', { name: 'Job Story: Recovered' });
    expect(screen.getByText('New prompt')).toBeVisible();
    expect(screen.getByText('New detail')).toBeVisible();
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
    vi.mocked(getJob).mockReturnValueOnce(old.promise).mockResolvedValue(metadata({ title: 'Newest' }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await screen.findByRole('heading', { name: 'Job Story: Newest' });
    await act(async () => { if (outcome === 'success') old.resolve(metadata({ title: 'Obsolete' })); else old.reject(new Error('obsolete')); });
    expect(screen.getByRole('heading', { name: 'Job Story: Newest' })).toBeVisible();
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
  it.each(['job', 'tenant'])('isolates context across %s changes and late responses', async change => {
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
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    const current = deferred<RecipeJob>();
    vi.mocked(getJob).mockReturnValueOnce(current.promise);
    act(() => navigate(change === 'job' ? '/project/tenant/jobs/new/story' : '/project/other/jobs/job/story'));
    expect(screen.queryByText('Old prompt')).toBeNull();
    await act(async () => { old.resolve(metadata({ title: 'Obsolete', prompt: 'Leaked' })); });
    expect(screen.queryByText('Leaked')).toBeNull();
    await act(async () => { current.resolve(metadata({ title: 'Current' })); });
    expect(screen.getByRole('heading', { name: 'Job Story: Current' })).toBeVisible();
  });
});
function renderContext(stub = true) {
  if (stub) vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
  return render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}>
    <Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant" />} /></Routes>
  </MemoryRouter>);
}
