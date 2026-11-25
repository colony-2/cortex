vi.mock('./GoalPromptEditor', () => ({ default: ({ value, onChange, disabled, id }: any) => <textarea id={id} aria-label="Prompt" value={value || ''} disabled={disabled} onChange={event => onChange(event.target.value)} /> }));
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { createCell, getCell, listCells, submitJob } from '@colony2/shared/api';
import { registryCell } from '../test/cellFixtures';
import NewGoalDialog from './NewGoalDialog';
vi.mock('@colony2/shared/api', async importOriginal => ({ ...await importOriginal<typeof import('@colony2/shared/api')>(), createCell: vi.fn(), getCell: vi.fn(), listCells: vi.fn(), submitJob: vi.fn() }));
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
  await waitFor(() => expect(submitJob).toHaveBeenCalledWith('one', { cell: 'renamed', inputs: { prompt: 'Build a tool', type: 'build' } }));
  expect(created).toHaveBeenCalledWith('g');
});
it('CX-38 CX-39 offers existing cells without registration controls', async () => {
  open(null);
  await screen.findByRole('combobox', { name: 'Cell' });
  expect(screen.queryByRole('radio')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Repository')).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create goal' })).toBeEnabled());
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('Choose a cell');
  expect(createCell).not.toHaveBeenCalled();
  expect(submitJob).not.toHaveBeenCalled();
});
it('CX-38 CX-39 explains empty registries and disables submission', async () => {
  vi.mocked(listCells).mockResolvedValue({ cells: [] });
  open(null);
  await screen.findByText('Create a cell in Cells first, then choose it here to start a goal.');
  expect(screen.getByRole('button', { name: 'Create goal' })).toBeDisabled();
  expect(createCell).not.toHaveBeenCalled();
  expect(submitJob).not.toHaveBeenCalled();
});
it('preserves fields and reuses the original submission after an uncertain failure', async () => {
  vi.mocked(submitJob).mockRejectedValueOnce(new Error('Connection lost')).mockResolvedValue({ goal_id: 'g' } as any);
  open();
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create goal' })).toBeEnabled());
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('request failed', { exact: true });
  const first = vi.mocked(submitJob).mock.calls[0][1];
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2));
  expect(vi.mocked(submitJob).mock.calls[1][1]).toBe(first);
  expect(first).not.toHaveProperty('job_id');
  expect(createCell).not.toHaveBeenCalled();
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
  vi.mocked(listCells).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ cells: [registryCell('platform', 'one')] });
  open();
  await screen.findByText('offline');
  expect(screen.getByRole('button', { name: 'Create goal' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Retry cells' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create goal' })).not.toBeDisabled());
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
 await screen.findByText('request failed', { exact: true });
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
 expect(vi.mocked(submitJob).mock.calls[2][1]).not.toBe(first[1]);
 expect(vi.mocked(submitJob).mock.calls.every(call => !('job_id' in call[1]))).toBe(true);
 expect(vi.mocked(submitJob).mock.calls[2][2]![0]).toBe(replacement);
 expect(vi.mocked(submitJob).mock.calls[2][2]).toEqual([replacement]);
});
it('ATT-010 CX-39 preserves files and rejects a missing prompt without registration', async () => {
 open(); attach([new File([], 'empty')]); await screen.findByRole('button', { name: 'Remove empty' });
 expect(screen.getByRole('button', { name: 'Remove empty' })).toBeVisible();
 fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: '' } });
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('Enter a prompt with text or an image');
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
it('RC-39 rejects an invalid filename without replacing valid attachments', async () => {
 const valid = new File(['retained requirements'], 'brief.md', { lastModified: 1 });
 open(); attach([valid]); await screen.findByRole('button', { name: 'Remove brief.md' });
 attach([new File(['invalid'], '../secret.txt')]);
 await screen.findByText(/Invalid filename/);
 expect(screen.getAllByRole('button', { name: /^Remove / }).map(button => button.getAttribute('aria-label'))).toEqual(['Remove brief.md']);
 vi.mocked(submitJob).mockRejectedValue(new Error('Inspect retained selection'));
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('request failed', { exact: true });
 expect(vi.mocked(submitJob).mock.calls[0][2]).toEqual([valid]);
 expect(vi.mocked(submitJob).mock.calls[0][2]![0]).toBe(valid);
});
it('ATT-011 revalidates file sizes before submission', async () => {
 open();
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
it('CX-39 retains files after target refresh failure and never registers a cell', async () => {
 const created = open(); const file = new File(['brief'], 'brief'); attach([file]);
 await screen.findByRole('button', { name: 'Remove brief' });
 vi.mocked(getCell).mockRejectedValueOnce(new Error('Cell lookup failed'));
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('Cell lookup failed');
 expect(submitJob).not.toHaveBeenCalled();
 expect(screen.getByRole('button', { name: 'Remove brief' })).toBeVisible();
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await waitFor(() => expect(submitJob).toHaveBeenCalledWith('one', expect.objectContaining({ cell: 'platform' }), [file]));
 expect(created).toHaveBeenCalledWith('g');
 expect(createCell).not.toHaveBeenCalled();
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
 expect(new Set(calls.map(call => call[1])).size).toBe(4);
 expect(calls.every(call => !('job_id' in call[1]))).toBe(true);
 expect(calls.map(call => call[2])).toEqual([[a], [a, b], [b], [b, a]]);
 expect(calls[3][2]).toEqual([b, a]);
});

it('CID-001 CID-003 CID-006 refreshes edited prompts and targets while returning the complete goal identity', async () => {
 const goalId = '3LfLqNNl2VVLo8Cg5a032Lm0igK';
 vi.mocked(submitJob).mockRejectedValue(new Error('Try again'));
 vi.mocked(listCells).mockResolvedValue({ cells: [registryCell('platform', 'one'), registryCell('api', 'one')] });
 vi.mocked(getCell).mockImplementation(async (_tenant, id) => registryCell(id === 'cell-api' ? 'api' : 'platform', 'one'));
 const created = open();
 await waitFor(() => expect(screen.getByRole('button', { name: 'Create goal' })).toBeEnabled());
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await screen.findByText('request failed', { exact: true });
 const first = vi.mocked(submitJob).mock.calls[0][1];
 fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Edited prompt' } });
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2));
 await screen.findByRole('button', { name: 'Create goal' });
 expect(vi.mocked(submitJob).mock.calls[1][1]).not.toBe(first);
 expect(vi.mocked(submitJob).mock.calls[1][1].inputs?.prompt).toBe('Edited prompt');
 fireEvent.mouseDown(screen.getByRole('combobox'));
 fireEvent.click(await screen.findByText('api · github.com/acme/api', { selector: '.ant-select-item-option-content' }));
 vi.mocked(submitJob).mockResolvedValueOnce({ goal_id: goalId } as any);
 fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
 await waitFor(() => expect(created).toHaveBeenCalledWith(goalId));
 expect(vi.mocked(submitJob).mock.calls[2][1].cell).toBe('api');
 expect(vi.mocked(submitJob).mock.calls[2][1]).not.toBe(vi.mocked(submitJob).mock.calls[1][1]);
 expect(vi.mocked(submitJob).mock.calls[2][1]).not.toHaveProperty('repo');
 expect(vi.mocked(submitJob).mock.calls.every(call => !('job_id' in call[1]))).toBe(true);
});

it('submits a long new-goal prompt intact without an editor maximum', async () => {
 open();
 await screen.findAllByText('platform · github.com/acme/platform');
 const prompt = 'Keep the entire goal explanation, including café and 日本語.\n'.repeat(160).trim();
 const editor = screen.getByLabelText('Prompt');
 expect(editor).not.toHaveAttribute('maxlength');
 fireEvent.change(editor,{target:{value:prompt}});
 expect(editor).toHaveValue(prompt);
 fireEvent.click(screen.getByRole('button',{name:'Create goal'}));
 await waitFor(()=>expect(submitJob).toHaveBeenCalledWith('one',expect.objectContaining({inputs:{prompt,type:'build'}})));
});

for (const source of ['---', '## ', '> ', '```\n\n```']) it(`CX-17 rejects formatting-only source ${JSON.stringify(source)}`, async () => {
  open();
  await screen.findAllByText('platform · github.com/acme/platform');
  fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: source } });
  fireEvent.click(screen.getByRole('button', { name: 'Create goal' }));
  await screen.findByText('Enter a prompt with text or an image');
  expect(submitJob).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Prompt')).toHaveValue(source);
});

it('CX-33 keeps a referenced attachment until its image reference is removed', async () => {
  open();
  attach([new File(['image'], 'diagram.png', { type: 'image/png' })]);
  const remove = await screen.findByRole('button', { name: 'Remove diagram.png' });
  fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: '![diagram](diagram.png)' } });
  expect(remove).toBeDisabled();
  expect(remove).toHaveAttribute('title', 'Remove the image reference from the prompt first.');
  fireEvent.change(screen.getByLabelText('Prompt'), { target: { value: 'Diagram no longer needed' } });
  expect(remove).toBeEnabled();
  fireEvent.click(remove);
  expect(screen.queryByRole('button', { name: 'Remove diagram.png' })).not.toBeInTheDocument();
});
