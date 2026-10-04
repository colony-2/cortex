import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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
  it('shows every job and task attempt on load, and displays the selected failure', async () => {
    const fetchMock = mount();
    await screen.findByText('Previous job attempts (2)');
    for (const job of [1, 2, 3]) {
      expect(screen.getByText(`Job attempt ${job}`)).toBeVisible();
      for (const attempt of [1, 2, 3]) expect(screen.getByText(`Task ${job}.${attempt}`)).toBeVisible();
    }
    fireEvent.click(screen.getByText('Task 1.2'));
    expect(await screen.findByText('Failure 1.2: temporary directory missing')).toBeVisible();
    expect(screen.getByTestId('location').textContent).toContain('nodeId=jobAttempt%3A1%7Cn_38');
    fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(screen.getByText('Failure 1.2: temporary directory missing')).toBeVisible();
    fireEvent.click(screen.getByText('Task 3.2'));
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
    await waitFor(() => expect(screen.queryByText('Task 1.1')).not.toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Expand all' }));
    expect(await screen.findByText('Task 1.1')).toBeVisible();
  });

  it('also shows failures represented as prior task attempts', async () => {
    const root = jobAttempt(1);
    const tasks = root.children[0].children[0].children;
    const response = { ...story, root: { ...root, children: [{ ...tasks[2], prior_attempts: tasks.slice(0, 2) }] } };
    mount('', response);
    await screen.findByText('Prior attempts (2)');
    fireEvent.click(screen.getByText('Task 1.1'));
    expect(await screen.findByText('Failure 1.1: temporary directory missing')).toBeVisible();
  });
});
