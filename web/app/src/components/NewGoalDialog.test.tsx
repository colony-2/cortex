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
  const created = open(null);
  fireEvent.click(screen.getByRole('radio', { name: 'New cell' }));
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'new-cell' } });
  fireEvent.change(screen.getByLabelText('Repository'), { target: { value: 'github.com/acme/new' } });
  vi.mocked(getCell).mockResolvedValue({ ...registryCell('new-cell', 'one'), repo: 'github.com/acme/new' });
  return created;
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

function attach(files: File[]) {
 const input = document.querySelector('input[type=file]')!;
 fireEvent.change(input, { target: { files } });
}
it('ATT-001 ATT-002 ATT-003 ATT-005 manages local files and reports duplicates', async () => {
 open();
 expect(screen.getByText(/Up to 10 files, 25 MiB total/)).toBeVisible();
 const file = new File(['hello'], 'brief.md'); attach([file]);
 await screen.findByRole('button', { name: 'Remove brief.md' });
 expect(submitJob).not.toHaveBeenCalled();
 attach([file]); await screen.findByText(/already attached/);
 expect(screen.getAllByRole('button', { name: 'Remove brief.md' })).toHaveLength(1);
 fireEvent.click(screen.getByRole('button', { name: 'Remove brief.md' }));
 attach([file]); await screen.findByRole('button', { name: 'Remove brief.md' });
});
it('ATT-013 ATT-014 ATT-016 ATT-019 retains exact files on retry and distinguishes replacement bytes', async () => {
 vi.mocked(submitJob).mockRejectedValue(new Error('Connection lost'));
 open();
 const file = new File(['one'], 'x', { lastModified: 1 }); attach([file]);
 await screen.findByRole('button', { name: 'Remove x' });
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('Connection lost');
 const first = vi.mocked(submitJob).mock.calls[0];
 expect(first[2]).toEqual([file]);
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2));
 expect(vi.mocked(submitJob).mock.calls[1]).toEqual(first);
 await screen.findByRole('button', { name: 'Create goal' });
 fireEvent.click(screen.getByRole('button', { name: 'Remove x' }));
 const replacement = new File(['two'], 'x', { lastModified: 1 }); attach([replacement]);
 await screen.findByRole('button', { name: 'Remove x' });
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(3));
 expect(vi.mocked(submitJob).mock.calls[2][1].job_id).not.toBe(first[1].job_id);
 expect(vi.mocked(submitJob).mock.calls[2][2]).toEqual([replacement]);
});
it('ATT-010 preserves files across modes and rejects a missing prompt before registration', async () => {
 open(); attach([new File([], 'empty')]); await screen.findByRole('button', { name: 'Remove empty' });
 fireEvent.click(screen.getByRole('radio', { name: 'New cell' }));
 expect(screen.getByRole('button', { name: 'Remove empty' })).toBeVisible();
 fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: '' } });
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('Enter a prompt');
 expect(createCell).not.toHaveBeenCalled(); expect(submitJob).not.toHaveBeenCalled();
});
it('ATT-012 guards duplicate submission and disables file mutation while pending', async () => {
 let resolve!: (value: any) => void;
 vi.mocked(submitJob).mockImplementation(() => new Promise(done => { resolve = done; }));
 open(); attach([new File(['x'], 'x')]); await screen.findByRole('button', { name: 'Remove x' });
 const button = screen.getByRole('button', { name: 'Create goal' });
 fireEvent.click(button); fireEvent.click(button);
 await screen.findByRole('button', { name: 'Creating goal…' });
 expect(submitJob).toHaveBeenCalledTimes(1);
 expect(screen.getByRole('button', { name: 'Attach files' })).toBeDisabled();
 expect(screen.getByRole('button', { name: 'Remove x' })).toBeDisabled();
 expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
 resolve({ goal_id: 'g' });
});
it('ATT-009 retains valid selection after oversized additions', async () => {
 open(); attach([new File(['ok'], 'ok')]); await screen.findByRole('button', { name: 'Remove ok' });
 attach([new File([new Uint8Array(25 * 1024 * 1024)], 'big')]);
 await screen.findByText('Attachments must total 25 MiB or less.');
 expect(screen.getByRole('button', { name: 'Remove ok' })).toBeVisible();
 expect(screen.queryByRole('button', { name: 'Remove big' })).not.toBeInTheDocument();
});
it('ATT-011 revalidates file sizes before registration', async () => {
 fillNewCell();
 const file = new File(['ok'], 'file'); attach([file]);
 await screen.findByRole('button', { name: 'Remove file' });
 Object.defineProperty(file, 'size', { value: 26 * 1024 * 1024 });
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('Attachments must total 25 MiB or less.');
 expect(createCell).not.toHaveBeenCalled(); expect(submitJob).not.toHaveBeenCalled();
});
it('ATT-017 cancellation discards files without uploading', async () => {
 const props = { projectId: 'one', filters: ['cell-platform'], onClose: vi.fn(), onCreated: vi.fn() };
 const view = render(<NewGoalDialog {...props} />);
 attach([new File([], 'discard')]); await screen.findByRole('button', { name: 'Remove discard' });
 fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
 await waitFor(() => expect(props.onClose).toHaveBeenCalled());
 expect(submitJob).not.toHaveBeenCalled();
 view.unmount(); render(<NewGoalDialog {...props} />);
 expect(screen.queryByRole('button', { name: 'Remove discard' })).not.toBeInTheDocument();
});
it('ATT-013 retains files after registration failure, then submits them for the new cell', async () => {
 const created = fillNewCell(); const file = new File(['brief'], 'brief'); attach([file]);
 await screen.findByRole('button', { name: 'Remove brief' });
 vi.mocked(createCell).mockRejectedValueOnce(new Error('Registration failed'));
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('Registration failed');
 expect(submitJob).not.toHaveBeenCalled();
 expect(screen.getByRole('button', { name: 'Remove brief' })).toBeVisible();
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await waitFor(() => expect(submitJob).toHaveBeenCalledWith('one', expect.objectContaining({ cell: 'new-cell' }), [file]));
 expect(created).toHaveBeenCalledWith('g');
});
it('ATT-015 allocates new attempts when files are added, removed, or reordered by reselection', async () => {
 vi.mocked(submitJob).mockRejectedValue(new Error('Try again'));
 open(); const a = new File(['a'], 'a'), b = new File(['b'], 'b');
 attach([a]); await screen.findByRole('button', { name: 'Remove a' });
 async function submitAndWait(count: number) {
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(count));
  await screen.findByRole('button', { name: 'Create goal' });
 }
 await submitAndWait(1);
 attach([b]); await screen.findByRole('button', { name: 'Remove b' }); await submitAndWait(2);
 fireEvent.click(screen.getByRole('button', { name: 'Remove a' })); await submitAndWait(3);
 attach([a]); await screen.findByRole('button', { name: 'Remove a' }); await submitAndWait(4);
 const calls = vi.mocked(submitJob).mock.calls;
 expect(new Set(calls.map(call => call[1].job_id)).size).toBe(4);
 expect(calls[3][2]).toEqual([b, a]);
});
