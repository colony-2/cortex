import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import JobStoryPage from './JobStoryPage';

vi.mock('@colony2/shared', async (importOriginal) => ({
  ...await importOriginal<typeof import('@colony2/shared')>(),
  useInputActivity: () => ({ pendingInputs: [], refresh: vi.fn() }),
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
afterEach(() => vi.unstubAllGlobals());

describe('job story history', () => {
  it('keeps historical job attempts in Retries and preserves inspected failures across refresh', async () => {
    const fetchMock = mount();
    await screen.findByText('Development.Design.Task 3.2');
    const mainTree = screen.getByRole('tree');
    expect(within(mainTree).queryByText(/Task 1/)).not.toBeInTheDocument();
    expect(within(mainTree).queryByText(/Previous job attempts/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Job retries (2)' }));
    expect(await screen.findByText('Job attempt 1')).toBeVisible();
    fireEvent.click(screen.getByText('Development.Design.Task 1.2'));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    expect(screen.getByTestId('location').textContent).toContain('nodeId=jobAttempt%3A1%7Cn_38');
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.getByText('Failure 1.2: temporary directory missing')).toBeVisible();
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
    fireEvent.click(screen.getByRole('tab', { name: 'Retries (2)' }));
    fireEvent.click(screen.getByRole('button', { name: 'Inspect attempt 1' }));
    expect(await screen.findByText('Failure 1.1: temporary directory missing')).toBeVisible();
  });
  it('shows a short scoped operation name and keeps source and restart in Overview', async () => {
    const source = 'git+https://github.com/colony-2/c2ops.git//codex@abc123';
    mount('', { ...story, root: {
      id: 'root', kind: 'recipe', title: 'recipe build', path: [], status: 'failed', children: [{
        id: 'machine', kind: 'stateMachine', title: 'stateMachine agent', path: [], status: 'failed', children: [{
          id: 'state', kind: 'state', title: 'state run', path: [], status: 'failed', children: [{
            id: 'op', kind: 'op', title: `op ${source}`, op_id: source, path: [], status: 'failed', children: [{
              id: 'task', kind: 'opStep', title: 'step extension_execution', path: [], status: 'failed',
              restart_from_ordinal: 12, task_ordinal: 14, error: { message: 'command failed' },
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
    expect(screen.getByRole('button', { name: 'Restart from here' })).toBeVisible();
  });

});
