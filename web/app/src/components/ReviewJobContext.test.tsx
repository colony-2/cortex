import { afterEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { getJob } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import ReviewJobContext from './ReviewJobContext';

vi.mock('@colony2/shared/api', () => ({ getJob: vi.fn() }));
afterEach(() => { vi.mocked(getJob).mockReset(); vi.clearAllMocks(); });
const job = { status: 'READY', store: 'ACTIVE', cell_name: 'platform', prompt: '**Build**\n<script>bad()</script>',
  recipe: 'build', job_type: 'recipe', repo: 'https://example.test/repo', git_ref: 'main', goal_id: 'goal / one',
  created_at: '2026-10-25T10:00:00Z', client_payload: { secret: 'never render this' } } as RecipeJob;

it('renders only designated fields, literal full prompt, formatted status, and encoded links', async () => {
  vi.mocked(getJob).mockResolvedValue(job);
  const { container } = render(<MemoryRouter><ReviewJobContext projectId="tenant / one" jobId="job / one" /></MemoryRouter>);
  const prompt = await screen.findByRole('region', { name: 'Goal prompt' });
  expect(prompt.textContent).toContain(job.prompt);
  expect(prompt.querySelector('script')).toBeNull();
  expect(prompt.querySelector('strong')).toBeNull();
  for (const text of ['platform', 'Ready', 'build', 'recipe', job.repo!, 'main', job.goal_id!]) expect(screen.getByText(text, { exact: true })).toBeVisible();
  expect(screen.getByRole('link', { name: 'Open execution' })).toHaveAttribute('href', '/project/tenant%20%2F%20one/jobs/job%20%2F%20one/story');
  expect(screen.getByRole('link', { name: 'Goal overview' })).toHaveAttribute('href', '/project/tenant%20%2F%20one/goals/goal%20%2F%20one');
  expect(container.textContent).not.toContain('never render this');
  expect(screen.getByText(/2026-10-25/)).toBeVisible();
});

it.each([undefined, '', '  \n '])('shows unknown names and absent prompts without invalid optional fields (%j)', async cell_name => {
  vi.mocked(getJob).mockResolvedValue({ status: 'READY', store: 'ACTIVE', cell_name, prompt: ' \n ', submitted_at: '0001-01-01T00:00:00Z', created_at: 'bad date' } as RecipeJob);
  render(<MemoryRouter><ReviewJobContext projectId="tenant" jobId="job" /></MemoryRouter>);
  await screen.findByText('Unknown cell');
  expect(screen.getByText('No prompt provided')).toBeVisible();
  expect(screen.queryByText('Submitted')).toBeNull();
  expect(screen.queryByText('Invalid Date')).toBeNull();
  expect(screen.queryByRole('link', { name: 'Goal overview' })).toBeNull();
});

it('distinguishes failures from missing metadata and retries only context', async () => {
  vi.mocked(getJob).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ ...job, cell_name: 'self' });
  render(<MemoryRouter><ReviewJobContext projectId="tenant" jobId="job" /></MemoryRouter>);
  await screen.findByText('Job context unavailable');
  expect(screen.getByText('Cell unavailable')).toBeVisible();
  expect(screen.queryByText('Unknown cell')).toBeNull();
  expect(screen.queryByText('No prompt provided')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Retry job context' }));
  await screen.findByText('self');
  expect(getJob).toHaveBeenCalledTimes(2);
});

it('clears context across identity changes and does not accept old results', async () => {
  let resolve!: (value: RecipeJob) => void;
  vi.mocked(getJob).mockImplementationOnce(() => new Promise(done => { resolve = done; })).mockResolvedValue({ ...job, prompt: 'current' });
  const { rerender } = render(<MemoryRouter><ReviewJobContext projectId="old" jobId="job" /></MemoryRouter>);
  rerender(<MemoryRouter><ReviewJobContext projectId="new" jobId="job" /></MemoryRouter>);
  await screen.findByText('current');
  resolve({ ...job, prompt: 'obsolete' });
  await waitFor(() => expect(screen.queryByText('obsolete')).toBeNull());
});
