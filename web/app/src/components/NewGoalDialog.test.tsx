import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { createCell, getCell, listCells, submitJob } from '@colony2/shared/api';
import { registryCell } from '../test/cellFixtures';
import NewGoalDialog from './NewGoalDialog';
vi.mock('@colony2/shared/api', () => ({ createCell: vi.fn(), getCell: vi.fn(), listCells: vi.fn(), submitJob: vi.fn() }));
beforeEach(() => {
  vi.mocked(createCell).mockReset();
  vi.mocked(createCell).mockResolvedValue({ ...registryCell("new-cell", "one"), repo: "github.com/acme/new" });
  vi.mocked(listCells).mockReset(); vi.mocked(getCell).mockReset(); vi.mocked(submitJob).mockReset();
  vi.mocked(listCells).mockResolvedValue({ cells: [registryCell('platform', 'one')] });
  vi.mocked(getCell).mockResolvedValue(registryCell('platform', 'one'));
  vi.mocked(submitJob).mockResolvedValue({ goal_id: 'g' } as any);
});
function open(filters: string[] | null = ['cell-platform']) {
  const onCreated = vi.fn();
  render(<NewGoalDialog projectId="one" filters={filters} onClose={vi.fn()} onCreated={onCreated} />);
  fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Build a tool' } });
  return onCreated;
}
it('refreshes a selected stable ID and submits its renamed canonical name without a repo assertion', async () => {
  const created = open();
  await screen.findAllByText('platform · github.com/acme/platform');
  vi.mocked(getCell).mockResolvedValue({ ...registryCell('platform', 'one'), name: 'renamed' });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledWith('one', { cell: 'renamed', job_id: expect.any(String), inputs: { prompt: 'Build a tool', type: 'build' } }));
  expect(created).toHaveBeenCalledWith('g');
});
it('registers an explicit name and repo before submitting a new-cell goal', async () => {
  open(null);
  fireEvent.click(screen.getByRole('radio', { name: 'New cell' }));
  fireEvent.change(screen.getByLabelText('Repository'), { target: { value: 'git@github.com:acme/new.git' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('Enter a cell name');
  expect(submitJob).not.toHaveBeenCalled();
  vi.mocked(getCell).mockResolvedValue({ ...registryCell('new-cell', 'one'), repo: 'github.com/acme/new' });
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'new-cell' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledWith('one', expect.objectContaining({ cell: 'new-cell', repo: 'github.com/acme/new' })));
  expect(createCell).toHaveBeenCalledWith('one', { name: 'new-cell', repo: 'git@github.com:acme/new.git', requestId: expect.any(String) });
  expect(vi.mocked(createCell).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(submitJob).mock.invocationCallOrder[0]);
});
it('preserves fields and reuses the original submission after an uncertain failure', async () => {
  vi.mocked(submitJob).mockRejectedValueOnce(new Error('Connection lost')).mockResolvedValue({ goal_id: 'g' } as any);
  open();
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('Connection lost');
  const first = vi.mocked(submitJob).mock.calls[0][1];
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2));
  expect(vi.mocked(submitJob).mock.calls[1][1]).toEqual(first);
});
it('rejects a cell retired since selection', async () => {
  open();
  await screen.findAllByText('platform · github.com/acme/platform');
  vi.mocked(getCell).mockResolvedValue({ ...registryCell('platform', 'one'), state: 'RETIRED' });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('This cell is retired. Choose an active cell.');
  expect(submitJob).not.toHaveBeenCalled();
});
it('blocks submission while the registry is unavailable and allows retry', async () => {
  vi.mocked(listCells).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ cells: [] });
  open();
  await screen.findByText('offline');
  expect(screen.getByRole('button', { name: 'Create goal' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Retry cells' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create goal' })).not.toBeDisabled());
});

function fillNewCell() {
  open(null);
  fireEvent.click(screen.getByRole('radio', { name: 'New cell' }));
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'new-cell' } });
  fireEvent.change(screen.getByLabelText('Repository'), { target: { value: 'github.com/acme/new' } });
  vi.mocked(getCell).mockResolvedValue({ ...registryCell('new-cell', 'one'), repo: 'github.com/acme/new' });
}
it('retries an uncertain registration with the same ID before submitting any job', async () => {
  fillNewCell();
  vi.mocked(createCell).mockRejectedValueOnce(new Error('Registration connection lost'));
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('Registration connection lost');
  expect(submitJob).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Updated goal' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(1));
  expect(vi.mocked(createCell).mock.calls[1]).toEqual(vi.mocked(createCell).mock.calls[0]);
});
it('keeps a successfully registered cell when submission fails and does not register it again', async () => {
  fillNewCell();
  vi.mocked(submitJob).mockRejectedValueOnce(new Error('Goal connection lost'));
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('Goal connection lost');
  expect(screen.getByText(/Cell “new-cell” is registered/)).toBeVisible();
  const first = vi.mocked(submitJob).mock.calls[0];
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2));
  expect(createCell).toHaveBeenCalledTimes(1);
  expect(vi.mocked(submitJob).mock.calls[1]).toEqual(first);
});
it('accepts root as an ordinary new cell name', async () => {
  fillNewCell();
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'root' } });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(createCell).toHaveBeenCalledWith('one', expect.objectContaining({ name: 'root' })));
});
