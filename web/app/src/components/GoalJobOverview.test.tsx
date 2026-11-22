import { render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { RecipeJobSummary } from '@colony2/shared/types';
import { jobIdentity } from '../test/cellFixtures';
import GoalJobOverview from './GoalJobOverview';
vi.mock('./SelectedJobDetails', () => ({ default: ({ promptJobId }: { promptJobId?: string }) => <span>Prompt owner: {promptJobId}</span> }));
vi.mock('./GoalPrompt', () => ({ default: ({ jobId, text }: { jobId: string; text: string }) => <article aria-label={`Prompt from ${jobId}`}>{text}</article> }));
it('renders the full prompt rather than a short title and retains fallback attempt ownership', () => {
  const current = { ...jobIdentity, job_id: 'newer', title: 'Short title', prompt: 'Full **prompt**', status: 'READY', store: 'ACTIVE', created_at: '2026-11-15T00:00:00Z' } as RecipeJobSummary;
  const older = { ...current, job_id: 'older', prompt: 'Earlier ![image](pasted.png)', status: 'COMPLETED', store: 'ARCHIVED', created_at: '2026-11-14T00:00:00Z' } as RecipeJobSummary;
  const view = render(<MemoryRouter><GoalJobOverview jobs={[current, older]} projectId="t" navigationState={{}} /></MemoryRouter>);
  expect(screen.getByRole('article', { name: 'Prompt from newer' })).toHaveTextContent('Full **prompt**');
  view.rerender(<MemoryRouter><GoalJobOverview jobs={[{ ...current, prompt: undefined }, older]} projectId="t" navigationState={{}} /></MemoryRouter>);
  expect(screen.getByRole('article', { name: 'Prompt from older' })).toHaveTextContent(older.prompt!);
  expect(screen.getByText('Prompt owner: older')).toBeVisible();
});
