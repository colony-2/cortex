import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import JobStoryPage from './JobStoryPage';
import * as storyTree from '../utils/storyTree';
import { jobIdentity } from '../test/cellFixtures';
import { getJob, getJobInputArtifacts, listJobChildren } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import DownloadChaptersButton from './DownloadChaptersButton';
import { inputActivityService, getConversation, getFinalDocuments } from '@colony2/shared';

const inputState = vi.hoisted(() => ({ pendingInputs: [] as any[] }));
vi.mock('@colony2/shared', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  getFinalDocuments: vi.fn(),
  getConversation: vi.fn().mockRejectedValue(new Error('History unavailable')),
  getConversationRequest: vi.fn().mockRejectedValue(new Error('Request unavailable')),
  getConversationEntry: vi.fn().mockRejectedValue(new Error('Request unavailable')),
  useInputActivity: () => ({ pendingInputs: inputState.pendingInputs, refresh: vi.fn() }),
}));
vi.mock('@colony2/shared/api', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared/api')>(),
  getJob: vi.fn(),
  getJobInputArtifacts: vi.fn().mockResolvedValue({ artifacts: [] }),
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
function mount(search = '?tab=execution', response: unknown = story) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => response });
  vi.stubGlobal('fetch', fetchMock);
  render(<MemoryRouter initialEntries={[`/project/tenant/jobs/job/story${search}`]}>
    <Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<><JobStoryPage projectId="tenant" /><Location /></>} /></Routes>
  </MemoryRouter>);
  return fetchMock;
}
beforeEach(() => {
  inputState.pendingInputs = [];
  vi.mocked(getFinalDocuments).mockReset().mockResolvedValue({job_id:'job',manifest_id:'empty',state:'empty',documents:[]});
  vi.mocked(getConversation).mockReset().mockRejectedValue(new Error('History unavailable'));
  vi.mocked(getJobInputArtifacts).mockReset().mockResolvedValue({ artifacts: [] });
  vi.mocked(getJob).mockReset().mockResolvedValue({ ...jobIdentity, goal_id: 'goal' } as RecipeJob);
  vi.mocked(listJobChildren).mockReset().mockResolvedValue({ jobs: [] });
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); });

describe('job story history', () => {
  it('preserves the expanded goal return location after selecting a node and a detail tab', async () => {
    const returnTo = '/project/tenant/goals?expanded=goal%3Agoal&detail=requirements';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
    render(<MemoryRouter initialEntries={[{ pathname: '/project/tenant/jobs/job/story', search: '?tab=execution', state: { goalsReturnTo: returnTo, goalId: 'goal', goalReturnTo: '/project/tenant/goals/goal?inspect=job' } }]}>
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
    const fetchMock = mount('?tab=execution', running);
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
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
    const fetchMock = mount('?tab=execution', { ...story, restart_scopes: [
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
    fetchMock.mockResolvedValueOnce({ ok: true, text: async () => JSON.stringify({
      project_id: 'tenant', job_id: 'job', chapter_ordinal: 12, scope_id: scopeID,
      job: { git: { author: 'old' }, existing: 'unchanged', secret: '[REDACTED]' }, visible: { vars: { example: 'available' } }, redacted_paths: ['/job/secret'],
    }) });
    fireEvent.click(within(dialog).getByRole('switch'));
    const before = await within(dialog).findByRole('region', { name: 'Context before patching' });
    expect(before).toHaveTextContent('unchanged');
    expect(before).toHaveTextContent('/job/secret');
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Add line' })[0]);
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. git.author'), { target: { value: 'git.author' } });
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. "james@example.com"'), { target: { value: 'patched' } });
    expect(before).toHaveTextContent('unchanged');
    expect(before).not.toHaveTextContent('patched');
    expect(within(dialog).queryByText('Full context is unavailable for this restart point.')).not.toBeInTheDocument();
    // Boundary drafts are isolated, including while inspection fails.
    fetchMock.mockResolvedValueOnce({ ok: false, status: 422, statusText: 'Unprocessable Entity', text: async () => 'History unavailable' });
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Restart point' }));
    fireEvent.click(await screen.findByText(text => text.startsWith('Recipe body · Chapter 4 ·')));
    expect(await within(dialog).findByText(/History unavailable/)).toBeVisible();
    expect(within(dialog).queryByRole('region', { name: 'Context before patching' })).not.toBeInTheDocument();
    expect(within(dialog).queryByPlaceholderText('e.g. "james@example.com"')).not.toBeInTheDocument();
    fetchMock.mockResolvedValueOnce({ ok: true, text: async () => JSON.stringify({
      project_id: 'tenant', job_id: 'job', chapter_ordinal: 4, scope_id: 'root',
      job: { git: { author: 'old' }, existing: 'earlier value' }, visible: {}, redacted_paths: [],
    }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Retry context' }));
    expect(await within(dialog).findByRole('region', { name: 'Context before patching' })).toHaveTextContent('earlier value');
    expect(within(dialog).queryByPlaceholderText('e.g. "james@example.com"')).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Add line' })[0]);
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. git.author'), { target: { value: 'git.author' } });
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. "james@example.com"'), { target: { value: 'patched' } });
    expect(within(dialog).queryByText('Patch Preview')).not.toBeInTheDocument();
    fetchMock.mockResolvedValueOnce({ ok: true, json: async () => ({ job_id: 'restarted' }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/jobs/job/restart'),
      expect.objectContaining({ method: 'POST', body: expect.stringContaining('"scope_id":"root"') }),
    ));
    const call = fetchMock.mock.calls.find(([url]) => String(url).endsWith('/jobs/job/restart'))!;
    expect(JSON.parse(call[1]!.body as string)).toEqual({ scope_id: 'root', goal: { mode: 'continue' }, request_id: expect.any(String), patch: [{op:'replace',path:'/context/git/author',value:'patched'}] });
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    fireEvent.click(await screen.findByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    fetchMock.mockResolvedValueOnce({ ok: true, text: async () => JSON.stringify({
      project_id: 'tenant', job_id: 'job', chapter_ordinal: 4, scope_id: 'root', job: {}, visible: {}, redacted_paths: [],
    }) });
    fireEvent.click(within(dialog).getByRole('switch'));
    await within(dialog).findByRole('region', { name: 'Context before patching' });
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Add line' })[0]);
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. git.author'), { target: { value: 'git.author' } });
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    fireEvent.click(await screen.findByRole('button', { name: 'Restart job' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/Selected scope · Chapter 4/)).toBeVisible();
    fetch.mockResolvedValueOnce({ ok: true, text: async () => JSON.stringify({ project_id: 'tenant', job_id: 'job', scope_id: 'target/scope', chapter_ordinal: 4, job: { git: { author: 'old' } }, visible: {}, redacted_paths: [] }) });
    fireEvent.click(within(dialog).getByRole('switch'));
    await within(dialog).findByRole('region', { name: 'Context before patching' });
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining('restart-context?scope_id=target%2Fscope'), expect.anything());
    sessionStorage.setItem('cells-v1:cortex:restart:tenant:job', 'legacy merge retry');
    sessionStorage.setItem('unrelated', 'keep');
    fireEvent.click(within(dialog).getAllByRole('button', { name: 'Add line' })[0]);
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. git.author'), { target: { value: 'git.author' } });
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. "james@example.com"'), { target: { value: 'first edit' } });
    fetch.mockRejectedValueOnce(new TypeError('Connection lost'));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Restart', exact: true })).toBeEnabled());
    const first = fetch.mock.calls.find(([url]) => String(url).endsWith('/restart'))!;
    expect(JSON.parse(first[1]!.body as string)).toMatchObject({ scope_id: 'target/scope', request_id: expect.any(String) });
    expect(JSON.parse(first[1]!.body as string)).not.toHaveProperty('chapter_ordinal');
    fetch.mockResolvedValueOnce({ ok: false, status: 400, statusText: 'Bad Request', text: async () => 'Request rejected' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetch.mock.calls.filter(([url]) => String(url).endsWith('/restart'))).toHaveLength(2));
    const requests = fetch.mock.calls.filter(([url]) => String(url).endsWith('/restart'));
    expect(requests[1][1]!.body).toBe(requests[0][1]!.body);
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Restart', exact: true })).toBeEnabled());
    expect(within(dialog).getByPlaceholderText('e.g. "james@example.com"')).toHaveValue('first edit');
    fireEvent.change(within(dialog).getByPlaceholderText('e.g. "james@example.com"'), { target: { value: 'changed edit' } });
    fetch.mockResolvedValueOnce({ ok: true, json: async () => ({ job_id: 'resumed' }) });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Restart', exact: true }));
    await waitFor(() => expect(fetch.mock.calls.filter(([url]) => String(url).endsWith('/restart'))).toHaveLength(3));
    const last = fetch.mock.calls.filter(([url]) => String(url).endsWith('/restart'))[2];
    expect(JSON.parse(last[1]!.body as string).request_id).not.toBe(JSON.parse(requests[0][1]!.body as string).request_id);
    expect(sessionStorage.getItem('cells-v1:cortex:restart:tenant:job')).toBe('legacy merge retry');
    expect(sessionStorage.getItem('unrelated')).toBe('keep');

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
      fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
      await screen.findByText('Development.Design.Task 3.2');
      fireEvent.click(screen.getByRole('switch', { name: 'Auto-refresh' }));
      fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
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
    const fetchMock = mount('');
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
    mount('');
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByText('Development.Design.Task 3.2');
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
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
      mount('');
      expect(await screen.findByText('No failure details recorded for this job.')).toBeVisible();
      expect(screen.getByText(overrides.completion_status === 'failed_timeout' ? 'Job timed out' : 'Job failed')).toBeVisible();
    });
  it.each([
    { completion_status: 'success' }, { completion_status: 'cancelled' }, {},
    { status: 'ACTIVE', store: 'ACTIVE', completion_status: 'failed_app' },
    { completion_status: 'future_outcome' },
  ])('does not invent failures for %j', async overrides => {
    vi.mocked(getJob).mockResolvedValue(metadata({ completion_detail: 'retained failure', ...overrides } as Partial<RecipeJob>));
    mount('');
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByText('Development.Design.Task 3.2');
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
    expect(screen.queryByText('Job failed')).toBeNull();
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    fireEvent.click(screen.getByText('Development.Design.Task 3.2'));
    expect(await screen.findByText('Failure 3.2: temporary directory missing')).toBeVisible();
  });
  it('keeps context independent of historical task selection', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ cell_name: 'owning-cell', parent: { job_id: 'parent', cell_name: 'parent-cell' }, metadata: { workspace_cell: 'workspace-cell' }, title: 'Current', prompt: 'Current prompt', completion_status: 'failed_app', completion_detail: 'Current failure' }));
    mount('');
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    fireEvent.click(await screen.findByRole('tab', { name: 'Job attempt 1' }));
    fireEvent.click(screen.getByText('Development.Design.Task 1.2'));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
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
    mount('');
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByText('Development.Design.Task 3.2');
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByRole('heading', { name: 'Job Story: Independent' });
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    await act(async () => { pending.reject(new Error('transport failure')); });
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    expect(await screen.findByText('Failed to load job story')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible();
    expect(screen.queryByText('Job failed')).toBeNull();
    log.mockRestore();
  });
  it.each(['success', 'failure'])('ignores an older refresh %s after a newer response', async outcome => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Initial' }));
    mount('');
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
    const fetchMock = mount('');
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
    mount('');
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
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByText('Failed to load job story');
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
    vi.mocked(getJob).mockResolvedValue(metadata({ prompt: 'Loaded prompt' }));
    fireEvent.click(screen.getByText('Job details unavailable').closest('[role="alert"]')!.querySelector('button')!);
    expect(await screen.findByText('Loaded prompt')).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toHaveTextContent(`Cell: ${jobIdentity.cell_name}`); // JC-006
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    expect(await screen.findByText('Failed to load job story')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry', exact: true })).toBeVisible();
    log.mockRestore();
  });
  it.each(['success', 'failure'])('ignores obsolete %s after a newer metadata failure', async outcome => {
    vi.mocked(getJob).mockResolvedValue(metadata({ prompt: 'Initial' }));
    mount('');
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
    expect(screen.getByRole('switch', { name: 'Auto-refresh' })).toBeChecked();
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
    fireEvent.click(await screen.findByRole('tab', { name: 'Outbox (2)' }));
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
    fireEvent.click(await screen.findByRole('tab',{name:'Outbox (4)'}));
    fireEvent.click(screen.getByRole('link',{name,exact:true}));
    return mock;
  }
  it.each([
    ['data.json','{"n":9007199254740993}','Formatted JSON'],
    ['data.json','{"n":bad}', 'JSON preview unavailable'],
    ['data.jsonl','{}\nbad','Line 2: invalid JSON'],
    ['file.patch','--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n','Unified'],
    ['index.html','<script>bad()</script>','Interactive HTML runs automatically'],
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
    await screen.findByRole('tab', {name:'Outbox (5)'});
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
    fireEvent.click(await screen.findByRole('tab',{name:'Outbox (1)'}));
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
    fireEvent.click(await screen.findByRole('tab',{name:'Outbox (5)'}));
    fireEvent.click(screen.getByRole('link',{name:'docs/bar.md'}));
    await screen.findByRole('heading',{name:'Bar'});
    return {mock,refresh:()=>{refreshed=true;}};
  }
  it('MAL-017,018,019,021,022,023,028,031 keeps the original snapshot through refresh and chained navigation', async()=>{
    const {mock,refresh}=await start();
    fireEvent.click(screen.getByRole('button',{name:'ambiguous'}));expect(within(screen.getByRole('dialog')).getByRole('status')).toHaveTextContent('ambiguous');
    refresh();fireEvent.click(screen.getByRole('button',{name:/Refresh$/,exact:true}));await screen.findByRole('tab',{name:'Outbox (6)'});
    fireEvent.click(screen.getByText('Other node', {selector:'strong'}));await screen.findByRole('tab',{name:'Outbox (2)'});
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
  expect(transport.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(0);
});

it('REL-001,010,030,031 keeps URL relationships through past-attempt selection and child loading failure', async () => {
  vi.mocked(getJob).mockResolvedValue({ ...jobIdentity, tenant_id:'tenant', job_id:'job', goal_id:'goal', parent:{job_id:'parent',goal_id:'parent-goal'} } as RecipeJob);
  vi.mocked(listJobChildren).mockReset().mockRejectedValueOnce(new Error('offline'));
  mount();
  fireEvent.click(screen.getByRole('tab', { name: 'Relationships', exact: true }));
  await screen.findByText('Child jobs unavailable');
  expect(screen.getByRole('link',{name:'Job · parent'})).toHaveAttribute('href','/project/tenant/jobs/parent/story');
  expect(screen.getByRole('link',{name:'Goal · parent-goal'})).toHaveAttribute('href','/project/tenant/goals/parent-goal');
  fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
  fireEvent.click(await screen.findByRole('button',{name:'Job attempts'}));
  fireEvent.click(screen.getByRole('tab',{name:'Job attempt 1'}));
  fireEvent.click(await screen.findByText('Development.Design.Task 1.2'));
  await screen.findByText('Failure 1.2: temporary directory missing');
  expect(listJobChildren).toHaveBeenCalledTimes(1);
  expect(listJobChildren).toHaveBeenCalledWith('tenant','job',{pageSize:20,pageToken:undefined});
  expect(getJob).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole('tab', { name: 'Relationships', exact: true }));
  expect(screen.getByRole('link',{name:'Job · parent'})).toBeVisible();
});

describe('structured node payloads', () => {
  it.each(['native', 'unknown extension'])('shows %s operation and step fields, preserving tab state without requests', async selector => {
    const payload = { status: 'ok', response: { files: ['a.ts'] }, result: false };
    const root = { id: 'payload-op', kind: 'op', title: selector, op_id: selector, status: 'success', path: ['op'], input: payload, output: payload,
      children: [{ id: 'payload-step', kind: 'opStep', title: 'Recorded step', status: 'success', path: ['op', 'step'], input: payload, output: payload }, { id: 'another-step', kind: 'opStep', title: 'Other step', status: 'success', path: ['op', 'other'] }] };
    const requests = mount('?nodeId=payload-op', { ...story, root });
    await screen.findByRole('tab', { name: 'Input', exact: true });
    fireEvent.click(screen.getByRole('tab', { name: 'Input', exact: true }));
    const input = await screen.findByRole('region', { name: 'Input payload' });
    expect(within(input).getByText('status')).toBeVisible();
    expect(within(input).getByText('ok')).toBeVisible();
    fireEvent.click(within(input).getByRole('button', { name: /response Object/ }));
    fireEvent.click(within(input).getByRole('button', { name: 'JSON' }));
    const count = requests.mock.calls.length;
    fireEvent.click(screen.getByRole('tab', { name: 'Output', exact: true }));
    expect(within(await screen.findByRole('region', { name: 'Output payload' })).getByRole('button', { name: 'Structured' })).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(screen.getByRole('tab', { name: 'Input', exact: true }));
    expect(within(input).getByRole('button', { name: 'JSON' })).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(within(input).getByRole('button', { name: 'Structured' }));
    expect(within(input).getByRole('button', { name: /response Object/ })).toHaveAttribute('aria-expanded', 'true');
    expect(requests).toHaveBeenCalledTimes(count);
    fireEvent.click(within(screen.getByRole('tree')).getByText('Recorded step'));
    fireEvent.click(screen.getByRole('tab', { name: 'Input', exact: true }));
    const stepInput = await screen.findByRole('region', { name: 'Input payload' });
    expect(within(stepInput).getByRole('button', { name: 'Structured' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(stepInput).getByRole('button', { name: /response Object/ })).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(screen.getByRole('tab', { name: 'Output', exact: true }));
    expect(within(await screen.findByRole('region', { name: 'Output payload' })).getByText('false')).toBeVisible();
  });
  it('resets repeated task IDs across task and job attempts', async () => {
    const root = (job: number) => ({ id: 'payload-root', kind: 'recipe', title: 'Payload attempts', path: [], status: 'failed', job_attempt: job,
      children: [1, 2].map(attempt => ({ id: `payload-${attempt}`, kind: 'opStep', title: `Payload step`, path: ['payload'], attempt, status: 'failed', input: { nested: { value: `${job}.${attempt}` } }, output: false })) });
    mount('?nodeId=jobAttempt%3A2%7Cpayload-2', { ...story, root: { ...root(2), past_attempts: [root(1)] } });
    fireEvent.click(await screen.findByRole('tab', { name: 'Input', exact: true }));
    let input = await screen.findByRole('region', { name: 'Input payload' });
    fireEvent.click(within(input).getByRole('button', { name: /nested Object/ }));
    fireEvent.click(within(input).getByRole('button', { name: 'JSON' }));
    fireEvent.click(screen.getByRole('tab', { name: 'Attempt 1', exact: true }));
    input = await screen.findByRole('region', { name: 'Input payload' });
    expect(within(input).getByRole('button', { name: 'Structured' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(input).getByRole('button', { name: /nested Object/ })).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(within(input).getByRole('button', { name: /nested Object/ }));
    expect(within(input).getByText('2.1')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Job attempts' }));
    fireEvent.click(screen.getByRole('tab', { name: 'Job attempt 1' }));
    fireEvent.click(within(screen.getByRole('tree')).getByText('Payload step'));
    fireEvent.click(screen.getByRole('tab', { name: 'Input', exact: true }));
    input = await screen.findByRole('region', { name: 'Input payload' });
    expect(within(input).getByRole('button', { name: 'Structured' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(input).getByRole('button', { name: /nested Object/ })).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(within(input).getByRole('button', { name: /nested Object/ }));
    expect(within(input).getByText('1.2')).toBeVisible();
  });
});

it.each([null, false, 0, ''])('keeps explicit root %j visible in both story payload directions', async value => {
  mount('?nodeId=falsy', { ...story, root: { id: 'falsy', kind: 'opStep', title: 'Falsy payload', path: [], status: 'success', input: value, output: value } });
  for (const label of ['Input', 'Output']) {
    fireEvent.click(await screen.findByRole('tab', { name: label, exact: true }));
    const region = await screen.findByRole('region', { name: `${label} payload` });
    expect(region).toHaveTextContent(value === '' ? 'Empty string' : String(value));
    expect(screen.queryByText(`No ${label.toLowerCase()} payload`)).toBeNull();
  }
});

it('preserves applied patch and recipe overview JSON presentations', async () => {
  mount('?nodeId=patch', { ...story, recipe: { name: 'unchanged recipe' }, root: {
    id: 'patch', kind: 'contextPatch', title: 'Patch', status: 'success', path: [], output: { applied: true },
  } });
  expect(await screen.findByText('Applied Context Patch')).toBeVisible();
  expect(screen.getByText('{"applied":true}')).toBeVisible();
  fireEvent.click(screen.getByRole('tab', { name: 'Summary', exact: true }));
  fireEvent.click(screen.getByText('Recipe metadata'));
  expect(screen.getByText('{"name":"unchanged recipe"}')).toBeVisible();
  fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
  expect(screen.queryByRole('button', { name: 'Structured' })).toBeNull();
  fireEvent.click(screen.getByRole('tab', { name: 'Output', exact: true }));
  expect(within(await screen.findByRole('region', { name: 'Output payload' })).getByText('applied')).toBeVisible();
});
it('preserves transition decisions in the overview', async () => {
  mount('?nodeId=transition', { ...story, root: { id: 'transition', kind: 'transitionEval', title: 'Transition', status: 'success', path: [],
    evaluations: [{ expression: 'status == done', result: true, to_state_id: 'finish' }], decision: { kind: 'state', to_state_id: 'finish' } } });
  expect(await screen.findByText('status == done')).toBeVisible();
  expect(screen.getByText('to finish')).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Structured' })).toBeNull();
});

describe('input artifact inventory', () => {
  const artifacts = [{ jobId: 'job', taskOrdinal: 0, name: 'docs/brief.md', sizeBytes: 120 }, { jobId: 'job', taskOrdinal: 0, name: 'a.bin', sizeBytes: 0 }];
  it('IA-009 breaks equal-name ties by the recorded ordinal', async () => {
    vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts: [
      { jobId: 'job', taskOrdinal: 17, name: 'same.md', sizeBytes: 17 },
      { jobId: 'job', taskOrdinal: 2, name: 'same.md', sizeBytes: 2 },
    ] });
    const fetch = mount();
    fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
    const buttons = await screen.findAllByRole('button', { name: 'same.md', exact: true });
    fetch.mockResolvedValue(new Response('# Earlier ordinal'));
    fireEvent.click(buttons[0]);
    expect(await screen.findByRole('heading', { name: 'Earlier ordinal' })).toBeVisible();
    expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('/tasks/2/artifacts/same.md'), expect.anything());
  });
  it('IA-008–013 loads independently of replay, sorts and previews with focus restoration', async () => {
    let resolve!: (result: { artifacts: typeof artifacts }) => void;
    vi.mocked(getJobInputArtifacts).mockImplementationOnce(() => new Promise(r => { resolve = r; }));
    const fetch = mount('', { job_id: 'job', status: 'running', root: null });
    fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
    fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  expect(screen.getByText('Loading input artifacts')).toBeVisible();
    await act(async () => resolve({ artifacts }));
    expect(await screen.findByText('Input artifacts (2)')).toBeVisible();
    const buttons = screen.getAllByRole('button').filter(b => ['a.bin', 'docs/brief.md'].includes(b.textContent || ''));
    expect(buttons.map(b => b.textContent)).toEqual(['a.bin', 'docs/brief.md']);
    expect(screen.getByText('0 B')).toBeVisible();
    expect(screen.getByText('120 B')).toBeVisible();
    fetch.mockResolvedValue(new Response('# Input brief'));
    const opener = screen.getByRole('button', { name: 'docs/brief.md', exact: true });
    opener.focus(); fireEvent.click(opener);
    expect(await screen.findByRole('heading', { name: 'Input brief' })).toBeVisible();
    expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('/tasks/0/artifacts/docs%2Fbrief.md'), expect.anything());
    fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0]);
    await waitFor(() => expect(opener).toHaveFocus());
  });
  it('IA-010 treats unsupported inventory as a local error and retries to empty', async () => {
    vi.mocked(getJobInputArtifacts).mockRejectedValueOnce(new Error('404 unsupported')).mockResolvedValueOnce({ artifacts: [] });
    mount();
    fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
    expect(await screen.findByText('Unable to load input artifacts')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Retry', exact: true }));
    expect(await screen.findByText('No input artifacts')).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: 'Summary', exact: true }));
    expect(screen.getByText('Run Summary')).toBeVisible();
  });
  it('IA-015 keeps metadata visible when bytes are unavailable and retries preview', async () => {
    vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts });
    const fetch = mount();
    fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
    await screen.findByText('Input artifacts (2)');
    fetch.mockResolvedValue({ ok: false });
    fireEvent.click(screen.getByRole('button', { name: 'docs/brief.md', exact: true }));
    expect(await screen.findByText(/Could not load this document/)).toBeVisible();
    expect(screen.getByRole('button', { name: 'docs/brief.md', exact: true })).toBeInTheDocument();
    fetch.mockResolvedValue(new Response('# Recovered input'));
    fireEvent.click(screen.getByRole('button', { name: 'Retry', exact: true }));
    expect(await screen.findByRole('heading', { name: 'Recovered input' })).toBeVisible();
  });
});

it.each(['other/job', 'tenant/other'])('DOC-14 IA-014 aborts inventory and preview when switching to %s', async destination => {
  let finish!: (value: { artifacts: [] }) => void;
  let inventorySignal: AbortSignal | undefined;
  let previewSignal: AbortSignal | undefined;
  let finishPreview!: (value: Response) => void;
  vi.mocked(getJobInputArtifacts).mockResolvedValueOnce({ artifacts: [{id:'document:inline',kind:'value',binding:'inline',contentBinding:'inline',name:'input.md',sizeBytes:10}] })
    .mockImplementationOnce((_tenant, _job, signal) => { inventorySignal = signal; return new Promise(r => { finish = r; }); });
  const fetch = vi.fn(async (url: string, options?: RequestInit) => {
    if (url.includes('/input-document?')) { previewSignal = options?.signal as AbortSignal; return new Promise<Response>(r => { finishPreview = r; }); }
    return { ok: true, json: async () => story };
  });
  vi.stubGlobal('fetch', fetch);
  function Host() {
    const { projectId } = useParams(); const navigate = useNavigate();
    return <><button onClick={() => navigate(`/project/${destination.split('/')[0]}/jobs/${destination.split('/')[1]}/story`)}>Switch job</button><JobStoryPage projectId={projectId!} /></>;
  }
  render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}><Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<Host />} /></Routes></MemoryRouter>);
  fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  fireEvent.click(await screen.findByRole('button', { name: 'input.md', exact: true }));
  await screen.findByLabelText('Loading artifact');
  fireEvent.click(screen.getByRole('button', { name: 'Switch job' }));
  await waitFor(() => expect(previewSignal?.aborted).toBe(true));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'input.md', exact: true })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  expect(screen.getByText('Loading input artifacts')).toBeVisible();
  await act(async () => finishPreview(new Response('# Stale preview')));
  expect(screen.queryByText('Stale preview')).not.toBeInTheDocument();
  cleanup();
  expect(inventorySignal?.aborted).toBe(true);
  await act(async () => finish({ artifacts: [] }));
});

it('IA-012 keeps same-name inputs and outputs in separate navigation and image catalogs', async () => {
  const keys = [{ jobId: 'job', taskOrdinal: 0, name: 'docs/brief.md', sizeBytes: 5 }, { jobId: 'job', taskOrdinal: 0, name: 'docs/next.md', sizeBytes: 5 }, { jobId: 'job', taskOrdinal: 0, name: 'docs/image.png', sizeBytes: 5 }];
  vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts: keys });
  const current = { ...story, root: { id: 'root', kind: 'opStep', title: 'Outputs', status: 'success', path: [], artifact_keys: [{ jobId: 'job', taskOrdinal: 8, name: 'docs/brief.md' }, { jobId: 'job', taskOrdinal: 9, name: 'docs/next.md' }] } };
  const fetch = mount('?nodeId=root', current);
  fetch.mockImplementation(async (url: string) => url.includes('/artifacts/') ? new Response(url.includes('image.png') ? new Uint8Array([137,80,78,71]) : url.includes('next.md') ? '# Input target' : url.includes('/tasks/0/') ? '# Input\n\n[next](next.md) ![relative](image.png)' : '# Output') : { ok: true, json: async () => current });
  fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  fireEvent.click(await screen.findByRole('button', { name: 'docs/brief.md', exact: true }));
  await screen.findByRole('heading', { name: 'Input', exact: true });
  fireEvent.click(screen.getByRole('button', { name: 'next', exact: true }));
  await screen.findByRole('heading', { name: 'Input target' });
  const requests = fetch.mock.calls.map(c => String(c[0])).filter(url => url.includes('/artifacts/'));
  expect(requests).toContainEqual(expect.stringContaining('/tasks/0/artifacts/docs%2Fimage.png'));
  expect(requests).toContainEqual(expect.stringContaining('/tasks/0/artifacts/docs%2Fnext.md'));
  expect(requests.some(url => url.includes('/tasks/8/') || url.includes('/tasks/9/'))).toBe(false);
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0]);
  fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
  fireEvent.click(await screen.findByRole('tab', { name: 'Outbox (2)' }));
  fireEvent.click(screen.getByRole('link', { name: 'docs/brief.md', exact: true }));
  expect(await screen.findByRole('heading', { name: 'Output', exact: true })).toBeVisible();
  expect(fetch.mock.calls.at(-1)?.[0]).toContain('/tasks/8/artifacts/docs%2Fbrief.md');
});

it('IA-014 ignores a late inventory result from the previous job', async () => {
  let finish!: (value: { artifacts: { jobId: string; taskOrdinal: number; name: string; sizeBytes: number }[] }) => void;
  let oldSignal: AbortSignal | undefined;
  vi.mocked(getJobInputArtifacts).mockImplementationOnce((_tenant, _job, signal) => { oldSignal = signal; return new Promise(r => { finish = r; }); }).mockResolvedValueOnce({ artifacts: [] });
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
  function Host() {
    const navigate = useNavigate();
    return <><button onClick={() => navigate('/project/tenant/jobs/next/story')}>Next job</button><JobStoryPage projectId="tenant" /></>;
  }
  render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story']}><Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<Host />} /></Routes></MemoryRouter>);
  fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  expect(screen.getByText('Loading input artifacts')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Next job' }));
  fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  expect(await screen.findByText('No input artifacts')).toBeVisible();
  expect(oldSignal?.aborted).toBe(true);
  await act(async () => finish({ artifacts: [{ jobId: 'job', taskOrdinal: 0, name: 'stale.md', sizeBytes: 1 }] }));
  expect(screen.queryByText('stale.md')).not.toBeInTheDocument();
  expect(screen.getByText('No input artifacts')).toBeVisible();
});

it('DAG-17,18 shows Summary and Input artifacts without outer disclosures', async () => {
  vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts: [{ jobId: 'job', taskOrdinal: 0, name: 'input.md', sizeBytes: 5 }] });
  mount('?tab=input-artifacts');
  const input = await screen.findByRole('button', { name: 'input.md', exact: true });
  expect(input).toBeVisible();
  expect(input.closest('details')).toBeNull();
  fireEvent.click(screen.getByRole('tab', { name: 'Summary', exact: true }));
  expect(screen.getByRole('heading', {name:'Run Summary'}).closest('details')).toBeNull();
  expect(screen.getByText('Execution timing and recipe metadata load when you open Steps.')).toBeVisible();
  fireEvent.click(screen.getByRole('tab', { name: 'Input artifacts', exact: true }));
  expect(input).toBeVisible();
  expect(getJobInputArtifacts).toHaveBeenCalledTimes(1);
});

describe('Conversation tab layout', () => {
  it.each(['success', 'failed_app', 'failed_timeout', 'cancelled'])(
    'CT-008 CT-023 orders prompt, saved requests and persisted %s completion', async completion_status => {
      vi.mocked(getJob).mockResolvedValue(metadata({prompt: 'Saved original prompt', completion_status, completion_detail: 'Persisted terminal detail'}));
      vi.mocked(getConversation).mockResolvedValue({tenant_id:'tenant', job_id:'job', snapshot_revision:1, snapshot_token:'snapshot', observed_at:'now', completeness:'partial', entries:[{
        entry_id:'saved-occurrence', kind:'review', title:{present:true,raw:'Saved review'}, summary:{present:false}, state:'answered', requested_at:{present:false}, locator:{}, form_fields:{}, accepted_response:{present:true,raw:'Winning answer'}, response_source:'submission', actor:{provenance:'submission_receipt',legacy_user_id:{present:false}},
      }]});
      mount('');
      const prompt = await screen.findByRole('region', {name:'Job prompt'});
      const answer = (await screen.findAllByText('Winning answer'))[0];
      const completion = await screen.findByRole('region', {name:'Job completion'});
      expect(prompt).toHaveTextContent('Saved original prompt');
      expect(prompt.compareDocumentPosition(answer) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(answer.compareDocumentPosition(completion) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(completion).toHaveTextContent('Persisted terminal detail');
      expect(completion).not.toHaveTextContent('Merge status');
    });
  it('CT-023 omits completion while running even with a recorded completion field', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({status:'READY',store:'ACTIVE',completion_detail:'Do not display while running'}));
    mount('');
    await screen.findByRole('region', {name:'Job prompt'});
    expect(screen.queryByRole('region', {name:'Job completion'})).toBeNull();
  });
  it('CT-001,002 defaults to Conversation and hides the other panes from accessibility navigation', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ title: 'Build the requested feature', prompt: 'Initial request' }));
    mount('');
    expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getAllByRole('tabpanel').filter(panel => panel.classList.contains('job-detail-panel'))).toHaveLength(1);
    expect(await screen.findByRole('heading', { name: 'Job Story: Build the requested feature' })).toBeVisible();
    expect(screen.getByLabelText('Job cell')).toBeVisible();
    expect(screen.getByLabelText('Job status')).toBeVisible();
    expect(screen.getByRole('button', { name: /Refresh$/ })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Download chapters' })).toBeVisible();
    expect(screen.getByRole('region', { name: 'Job prompt' })).toHaveTextContent('Initial request');
    expect(screen.queryByRole('tree')).toBeNull();
    for (const name of ['Steps', 'Relationships', 'Input artifacts', 'Summary', 'Conversation']) {
      fireEvent.click(screen.getByRole('tab', { name, exact: true }));
      expect(screen.getAllByRole('tabpanel').filter(panel => panel.classList.contains('job-detail-panel'))).toHaveLength(1);
      expect(screen.getByRole('tabpanel', { name, exact: true })).toBeVisible();
      expect(screen.getByRole('tab', { name, exact: true })).toHaveAttribute('aria-selected', 'true');
    }
  });
  it('CT-004,005 tab selection clears overriding intent and preserves selectors and unrelated parameters through back/forward', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => story }));
    function Navigation() {
      const navigate = useNavigate();
      return <><button onClick={() => navigate(-1)}>Previous location</button><button onClick={() => navigate(1)}>Next location</button></>;
    }
    render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story?tenantId=tenant&filter=mine&nodeId=n_40&input=1&requestId=opaque%2Fid']}>
      <Navigation /><Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<><JobStoryPage projectId="tenant" /><Location /></>} /></Routes>
    </MemoryRouter>);
    expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByText('Request unavailable', { selector: '.ant-alert-message' })).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    const search = new URLSearchParams(screen.getByTestId('location').textContent!);
    expect(search.get('tab')).toBe('execution');
    expect(search.get('nodeId')).toBe('n_40');
    expect(search.get('tenantId')).toBe('tenant'); expect(search.get('filter')).toBe('mine');
    expect(search.has('input')).toBe(false); expect(search.has('requestId')).toBe(false);
    expect(await screen.findByText('Failure 3.3: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Previous location' }));
    expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByText('Request unavailable', { selector: '.ant-alert-message' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Next location' }));
    expect(screen.getByRole('tab', { name: 'Steps', exact: true })).toHaveAttribute('aria-selected', 'true');
  });
  it('CT-004 opens legacy pending input independently of the execution tree', async () => {
    const read = vi.spyOn(inputActivityService, 'getInputDetails').mockResolvedValue({
      jobId: 'job', status: 'pending', startTime: '', task_ordinal: 10,
      form: { kind: 'review', request_id: 'current', title: 'Current review' },
    });
    try {
      mount('?input=1&taskOrdinal=10', { ...story, root: null });
      expect(await screen.findByRole('link', { name: 'Open Review', exact: true })).toHaveAttribute('href', '/project/tenant/reviews/job');
      expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute('aria-selected', 'true');
      expect(screen.queryByRole('tree')).toBeNull();
      expect(read).toHaveBeenCalledWith('tenant', 'job');
    } finally { read.mockRestore(); }
  });
  it('CT-006 never substitutes current input for an unavailable historical identity', async () => {
    const read = vi.spyOn(inputActivityService, 'getInputDetails');
    try {
      mount('?requestId=unknown&input=1&taskOrdinal=10');
      await screen.findByLabelText('Job cell');
      expect(await screen.findByText('Request unavailable', { selector: '.ant-alert-message' })).toBeVisible();
      expect(screen.queryByRole('link', { name: 'Open Review' })).toBeNull();
      expect(read).not.toHaveBeenCalled();
    } finally { read.mockRestore(); }
  });
  it('CT-006 invalid tab overrides legacy execution intent and defaults to Conversation', () => {
    mount('?tab=invalid&nodeId=n_40');
    expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('tree')).toBeNull();
  });
  it('CT-020 keeps Conversation usable when the execution API fails', async () => {
    vi.mocked(getJob).mockResolvedValue(metadata({ prompt: 'Still available' }));
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Execution unavailable')));
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    renderContext(false);
    expect(await screen.findByText('Still available')).toBeVisible();
    expect(screen.getByRole('tabpanel')).toHaveAccessibleName('Conversation');
    expect(await screen.findByText('History unavailable', { selector: '.ant-alert-message' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    expect(await screen.findByText('Failed to load job story')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible();
    log.mockRestore();
  });
});

describe('Steps on demand', () => {
  it('STP-001,002,003,007,008,014,015 defers retrieval and inspector construction, then reuses cache', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    vi.mocked(getJob).mockResolvedValue({ ...jobIdentity, status: 'RUNNING', recipe: 'fixture.yaml' } as RecipeJob);
    const construction = vi.spyOn(storyTree, 'buildStoryTree');
    const requests = mount('');
    await screen.findByRole('switch', { name: 'Auto-refresh' });
    const storyReads = () => requests.mock.calls.filter(([url]) => String(url).endsWith('/story')).length;
    fireEvent.click(screen.getByRole('button', { name: /Refresh$/ }));
    await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
    expect(storyReads()).toBe(0);
    for (const name of ['Relationships', 'Input artifacts', 'Summary']) {
      fireEvent.click(screen.getByRole('tab', { name, exact: true }));
      expect(document.querySelector('#execution-story-tree')).toBeNull();
      expect(storyReads()).toBe(0);
    }
    expect(construction.mock.calls.filter(([root]) => !!root)).toHaveLength(0);
    expect(screen.getByText('fixture.yaml')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Open Steps to load execution timing and recipe metadata' }));
    await screen.findByText('Development.Design.Task 3.2');
    expect(storyReads()).toBe(1);
    fireEvent.click(screen.getByText('Development.Design.Task 3.2'));
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
    construction.mockClear();
    expect(document.querySelector('#execution-story-tree')).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
    expect(construction.mock.calls.filter(([root]) => !!root)).toHaveLength(0);
    expect(storyReads()).toBe(1);
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    expect(await screen.findByText('Failure 3.2: temporary directory missing')).toBeVisible();
    expect(storyReads()).toBe(1);
    construction.mockRestore();
  });

  it('STP-003,007 rejects a response arriving after leaving Steps', async () => {
    const requests = mount('');
    let finish!: (value: unknown) => void;
    requests.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    expect(requests.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(1);
    fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
    await act(async () => finish({ ok: true, json: async () => story }));
    expect(document.querySelector('#execution-story-tree')).toBeNull();
    fireEvent.click(screen.getByRole('tab', { name: 'Summary', exact: true }));
    expect(screen.getByText('Execution timing and recipe metadata load when you open Steps.')).toBeVisible();
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByText('Development.Design.Task 3.2');
    expect(requests.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(2);
  });

  it('STP-010 retries only Steps and recovers', async () => {
    const requests = mount('');
    requests.mockRejectedValueOnce(new Error('Steps unavailable'));
    fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
    await screen.findByText('Steps unavailable');
    const metadataReads = vi.mocked(getJob).mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'Retry', exact: true }));
    await screen.findByText('Development.Design.Task 3.2');
    expect(getJob).toHaveBeenCalledTimes(metadataReads);
  });
});

it.each(['job', 'tenant'].flatMap(change => ['success', 'failure'].map(outcome => ({ change, outcome }))))('STP-009 rejects stale story $outcome after $change identity changes', async ({ change, outcome }) => {
  const old = deferred<Response>();
  const transport = vi.fn().mockReturnValueOnce(old.promise).mockResolvedValue({ ok: true, json: async () => story });
  vi.stubGlobal('fetch', transport);
  function Host() {
    const { projectId } = useParams();
    const navigate = useNavigate();
    return <><button onClick={() => navigate(change === 'job' ? '/project/tenant/jobs/new/story' : '/project/other/jobs/job/story')}>Change identity</button><JobStoryPage projectId={projectId!} /></>;
  }
  render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story?tab=execution']}><Routes>
    <Route path="/project/:projectId/jobs/:jobId/story" element={<Host />} />
  </Routes></MemoryRouter>);
  expect(transport.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: 'Change identity' }));
  await act(async () => {
    if (outcome === 'success') old.resolve({ ok: true, json: async () => story } as Response);
    else old.reject(new Error('Obsolete story error'));
  });
  expect(screen.getByRole('tab', { name: 'Conversation' })).toHaveAttribute('aria-selected', 'true');
  fireEvent.click(screen.getByRole('tab', { name: 'Summary', exact: true }));
  expect(screen.getByText('Execution timing and recipe metadata load when you open Steps.')).toBeVisible();
  expect(transport.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(1);
  fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
  await screen.findByText('Development.Design.Task 3.2');
  expect(transport.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(2);
});

it('STP-008,011,015,016 refreshes running cache only while visible and retains full Summary', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
  const response = { ...story, status: 'running', root: null, started_at: '2026-11-01T10:00:00Z', invocation_sequence: 42, recipe: { name: 'Full recipe', extra: 'retained' } };
  const requests = mount('?tab=execution', response);
  await screen.findByText('No story available yet');
  const reads = () => requests.mock.calls.filter(([url]) => String(url).endsWith('/story')).length;
  fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
  await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
  expect(reads()).toBe(1);
  fireEvent.click(screen.getByRole('tab', { name: 'Steps', exact: true }));
  await waitFor(() => expect(reads()).toBe(2));
  await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
  expect(reads()).toBe(3);
  requests.mockResolvedValue({ ok: true, json: async () => ({ ...response, status: 'completed' }) });
  await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
  expect(reads()).toBe(4);
  await screen.findByText('The story API returned no steps for this finished job.');
  await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
  expect(reads()).toBe(4);
  fireEvent.click(screen.getByRole('tab', { name: 'Summary', exact: true }));
  expect(screen.getByText('42')).toBeVisible();
  expect(screen.getByText('Full recipe')).toBeVisible();
  expect(screen.getByText(/"extra":"retained"/)).toBeInTheDocument();
});

it('STP-013 does not retrieve hidden Steps when cancellation started there completes later', async () => {
  const requests = mount('?tab=execution', { ...story, status: 'running' });
  await screen.findByText('Development.Design.Task 3.2');
  const completion = deferred<Response>();
  requests.mockImplementation(async (_url: string, init?: RequestInit) =>
    init?.method === 'POST' ? completion.promise : { ok: true, json: async () => story });
  fireEvent.click(screen.getByRole('button', { name: 'Cancel job', exact: true }));
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel job', exact: true }));
  fireEvent.click(screen.getByRole('tab', { name: 'Conversation', exact: true }));
  await act(async () => completion.resolve({ ok: true, status: 204 } as Response));
  expect(requests.mock.calls.filter(([url]) => String(url).endsWith('/story'))).toHaveLength(1);
  expect(document.querySelector('#execution-story-tree')).toBeNull();
});


it('CX-IN-01,08,10 refreshes Inbox snapshots without changing Outbox or structured data', async () => {
  const entry = {binding:'guide.md',path:'guide.md',source_kind:'stored',content:'stored',source_key:{jobId:'source',taskOrdinal:7,name:'original.md'}};
  const fixture = (binding = entry.binding) => ({job_id:'job',status:'failed',root:{id:'inbox-step',invocation_id:'recorded-task',kind:'opStep',title:'Recorded step',status:'failed',path:[],attempt:2,job_attempt:3,task_ordinal:19,input:{kept:'input'},output:{kept:'output'},artifact_keys:[{jobId:'job',taskOrdinal:19,name:'output.md'}],inbox:{status:'available',entries:[{...entry,binding,path:binding}]}}});
  let next = fixture();
  let resolveContent!: (value:Response)=>void;
  const fetch = vi.fn().mockImplementation((url:string) => url.includes('/artifacts/') ? new Promise<Response>(resolve => {resolveContent=resolve;}) : Promise.resolve({ok:true,json:async()=>next}));
  vi.stubGlobal('fetch',fetch);
  render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story?tab=execution']}><Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<JobStoryPage projectId="tenant"/>}/></Routes></MemoryRouter>);
  const inbox = await screen.findByRole('tab',{name:'Inbox (1)'});
  expect(screen.getByRole('tab',{name:'Overview',exact:true})).toHaveAttribute('aria-selected','true');
  const labels = screen.getAllByRole('tab').map(el=>el.textContent);
  expect(labels.indexOf('Inbox (1)')+1).toBe(labels.indexOf('Outbox (1)'));
  fireEvent.click(inbox);fireEvent.click(await screen.findByRole('button',{name:'guide.md',exact:true}));
  await waitFor(()=>expect(resolveContent).toBeTypeOf('function'));
  next=fixture('replacement.md');
  fireEvent.click(screen.getByRole('button',{name:'reload Refresh',exact:true,hidden:true}));
  expect(await screen.findByRole('button',{name:'replacement.md',exact:true})).toBeVisible();
  resolveContent(new Response('OLD CONTENT'));
  await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
  expect(screen.queryByText('OLD CONTENT')).toBeNull();
  expect(screen.queryByRole('button',{name:'guide.md',exact:true})).toBeNull();
  fireEvent.click(screen.getByRole('tab',{name:'Input',exact:true}));expect(within(screen.getByRole('region',{name:'Input payload'})).getByText('input',{exact:true})).toBeVisible();
  fireEvent.click(screen.getByRole('tab',{name:'Output',exact:true}));expect(within(screen.getByRole('region',{name:'Output payload'})).getByText('output',{exact:true})).toBeVisible();
  fireEvent.click(screen.getByRole('tab',{name:'Outbox (1)'}));expect(screen.getByRole('link',{name:'output.md',exact:true})).toBeVisible();
});

it.each(['other/job', 'tenant/other'])('CX-IN-08 aborts Inbox values when switching receiving identity to %s', async destination => {
  let finish!: (response:Response)=>void; let signal: AbortSignal | undefined;
  const receiving = {job_id:'job',status:'failed',root:{id:'same-id',invocation_id:'recorded-task',kind:'opStep',title:'Value reader',status:'failed',path:[],attempt:2,job_attempt:3,task_ordinal:19,inbox:{status:'available',entries:[{binding:'value.txt',path:'value.txt',source_kind:'value',content:'value'}]}}};
  vi.stubGlobal('fetch',vi.fn((url:string,options?:RequestInit) => {
    if(url.includes('/inbox/value')){signal=options?.signal as AbortSignal;return new Promise<Response>(resolve=>{finish=resolve;});}
    return Promise.resolve({ok:true,json:async()=>receiving});
  }));
  function Host(){const {projectId}=useParams();const navigate=useNavigate();return <><button onClick={()=>navigate(`/project/${destination.split('/')[0]}/jobs/${destination.split('/')[1]}/story?tab=execution`)}>Switch receiver</button><JobStoryPage projectId={projectId!}/></>;}
  render(<MemoryRouter initialEntries={['/project/tenant/jobs/job/story?tab=execution']}><Routes><Route path="/project/:projectId/jobs/:jobId/story" element={<Host/>}/></Routes></MemoryRouter>);
  fireEvent.click(await screen.findByRole('tab',{name:'Inbox (1)'}));fireEvent.click(await screen.findByRole('button',{name:'value.txt',exact:true}));
  await waitFor(()=>expect(signal).toBeDefined());
  fireEvent.click(screen.getByRole('button',{name:'Switch receiver',hidden:true}));
  await waitFor(()=>expect(signal?.aborted).toBe(true));
  await act(async()=>finish(new Response('OLD VALUE BYTES')));
  expect(screen.queryByRole('dialog')).toBeNull();expect(screen.queryByText('OLD VALUE BYTES')).toBeNull();
  fireEvent.click(await screen.findByRole('tab',{name:'Inbox (1)'}));expect(screen.getByRole('button',{name:'value.txt',exact:true})).toBeVisible();
});

describe('page controls refresh the real conversation', () => {
  const history = {tenant_id:'tenant',job_id:'job',entries:[],snapshot_token:'s',completeness:'complete'};
  const tick = async (ms: number) => { await act(async () => {await vi.advanceTimersByTimeAsync(ms);}); };
  async function start(status = 'ACTIVE', search = '') {
    vi.useFakeTimers({toFake:['setInterval','clearInterval']});
    vi.mocked(getConversation).mockResolvedValue({...history,activity:{availability:'available',value:metadata({status,store:status === 'ACTIVE' ? 'ACTIVE' : 'ARCHIVED'} as Partial<RecipeJob>)}});
    vi.mocked(getJob).mockResolvedValue(metadata({status,store:status === 'ACTIVE' ? 'ACTIVE' : 'ARCHIVED'} as Partial<RecipeJob>));
    const fetchMock = mount(search, {...story,status:'running'});
    await screen.findByRole('button',{name:/Refresh$/});
    await act(async()=>{});
    return fetchMock;
  }
  it.each(['ACTIVE','COMPLETED'])('CR-001 CR-002 manual refresh works for %s despite execution failure',async status=>{
    const fetchMock=await start(status);
    if(status === 'ACTIVE') fireEvent.click(screen.getByRole('switch',{name:'Auto-refresh'}));
    const initial=vi.mocked(getConversation).mock.calls.length;
    await tick(20000);
    expect(getConversation).toHaveBeenCalledTimes(initial);
    fetchMock.mockRejectedValue(new Error('Execution unavailable'));
    await act(async()=>{fireEvent.click(screen.getByRole('button',{name:/Refresh$/}));});
    expect(getConversation).toHaveBeenCalledTimes(initial+1);
  });
  it('CR-003 CR-004 follows only the selected schedule and restarts after enabling',async()=>{
    await start();
    const initial=vi.mocked(getConversation).mock.calls.length;
    await tick(4999);expect(getConversation).toHaveBeenCalledTimes(initial);
    await tick(1);expect(getConversation).toHaveBeenCalledTimes(initial+1);
    async function select(label:string) {
      fireEvent.mouseDown(screen.getByRole('combobox'));
      fireEvent.click(screen.getAllByText(label).find(el=>el.classList.contains('ant-select-item-option-content'))!);
      await act(async()=>{});
    }
    await select('3s');
    await tick(2999);expect(getConversation).toHaveBeenCalledTimes(initial+1);
    await tick(1);expect(getConversation).toHaveBeenCalledTimes(initial+2);
    await select('10s');
    await tick(9999);expect(getConversation).toHaveBeenCalledTimes(initial+2);
    await tick(1);expect(getConversation).toHaveBeenCalledTimes(initial+3);
    fireEvent.click(screen.getByRole('switch',{name:'Auto-refresh'}));
    await tick(20000);expect(getConversation).toHaveBeenCalledTimes(initial+3);
    fireEvent.click(screen.getByRole('switch',{name:'Auto-refresh'}));
    await tick(10000);expect(getConversation).toHaveBeenCalledTimes(initial+4);
  });
  it.each(['hidden','pending','failing'])('CR-005 polls independently of %s execution and failed metadata',async mode=>{
    const fetchMock=await start('ACTIVE',mode === 'hidden' ? '' : '?tab=execution');
    if(mode === 'pending') fetchMock.mockImplementation(()=>new Promise(()=>{}));
    if(mode === 'failing') fetchMock.mockRejectedValue(new Error('Execution failed'));
    vi.mocked(getJob).mockRejectedValue(new Error('Metadata failed'));
    const initial=vi.mocked(getConversation).mock.calls.length;
    await tick(5000);expect(getConversation).toHaveBeenCalledTimes(initial+1);
    await tick(5000);expect(getConversation).toHaveBeenCalledTimes(initial+2);
  });
  it('CR-011 input activity refreshes history while auto-refresh is disabled',async()=>{
    await start();
    fireEvent.click(screen.getByRole('switch',{name:'Auto-refresh'}));
    const initial=vi.mocked(getConversation).mock.calls.length;
    inputState.pendingInputs = [{job_id:'another-job',request_id:'new-input'}];
    await act(async()=>{fireEvent.click(screen.getByRole('tab',{name:'Summary',exact:true}));});
    expect(getConversation).toHaveBeenCalledTimes(initial+1);
    await tick(10000);expect(getConversation).toHaveBeenCalledTimes(initial+1);
  });
  it('CR-006 completion supersedes an older read and stops periodic history',async()=>{
    await start();
    let resolveOld!: (value: typeof history)=>void;
    vi.mocked(getConversation).mockImplementationOnce(()=>new Promise(resolve=>{resolveOld=resolve;}));
    await tick(5000);
    const oldSignal=vi.mocked(getConversation).mock.calls.at(-1)![2]?.signal;
    vi.mocked(getJob).mockResolvedValue(metadata({status:'COMPLETED',store:'ARCHIVED'}));
    vi.mocked(getConversation).mockResolvedValue({...history,activity:{availability:'available',value:metadata({status:'COMPLETED',store:'ARCHIVED'})}});
    const before=vi.mocked(getConversation).mock.calls.length;
    await tick(5000);
    expect(getConversation).toHaveBeenCalledTimes(before+1); // the conversation response itself is the final observation
    expect(oldSignal?.aborted).toBe(true);
    await act(async()=>{resolveOld({...history,snapshot_token:'obsolete'});});
    const final=vi.mocked(getConversation).mock.calls.length;
    await tick(20000);expect(getConversation).toHaveBeenCalledTimes(final);
    expect(screen.queryByLabelText('Current job activity')).toBeNull();
  });
});

it('DOC-05,08,09,12 documents retain distinct bindings, source labels and blocked states', async () => {
  vi.mocked(getJobInputArtifacts).mockResolvedValue({ artifacts: [
    { id:'document:a', kind:'value', binding:'a', name:'same.md', contentBinding:'a', sizeBytes:5, sources:['Document'] },
    { id:'document:b', kind:'value', binding:'b', name:'same.md', contentBinding:'b', sizeBytes:5, sources:['Document'] },
    { id:'document:external', kind:'external', binding:'external', name:'remote.md', sizeBytes:-1, sources:['Document'], reason:'External reference — content not loaded' },
    { id:'document:bad', kind:'invalid', binding:'bad', name:'invalid.md', sizeBytes:-1, sources:['Document'], reason:'Invalid document: missing payload' },
  ] });
  const fetch = mount('?tab=input-artifacts');
  expect(await screen.findByRole('heading', {name:'Input artifacts (4)'})).toBeVisible();
  expect(screen.getByText('Binding: a')).toBeVisible();
  expect(screen.getByText('Binding: b')).toBeVisible();
  expect(screen.getByText('External reference — content not loaded')).toBeVisible();
  expect(screen.queryByRole('button', {name:'Download remote.md'})).toBeNull();
  expect(screen.getByText('Invalid document: missing payload')).toBeVisible();
  const buttons=screen.getAllByRole('button',{name:'same.md',exact:true});
  fetch.mockResolvedValue(new Response('# First binding'));
  fireEvent.click(buttons[0]);
  expect(await screen.findByRole('heading',{name:'First binding'})).toBeVisible();
  expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('/input-document?binding=a'),expect.anything());
  fireEvent.click(screen.getAllByRole('button',{name:'Close',exact:true}).at(-1)!);
  fetch.mockResolvedValue(new Response('# Second binding'));
  fireEvent.click(buttons[1]);
  expect(await screen.findByRole('heading',{name:'Second binding'})).toBeVisible();
  expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('/input-document?binding=b'),expect.anything());
});
