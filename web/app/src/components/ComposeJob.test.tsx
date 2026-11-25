vi.mock('./GoalPromptEditor', () => ({ default: ({value, onChange, disabled, id, ...props}: any) => <textarea id={id} aria-label={props['aria-label']} value={value || ''} disabled={disabled} onChange={event => onChange(event.target.value)} /> }));
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, it, vi } from 'vitest';
import { getCell, submitJob } from '@colony2/shared/api';
import { registryCell } from '../test/cellFixtures';
import ComposeJob from './ComposeJob';
const state = vi.hoisted(() => ({ refresh: vi.fn(), reload: vi.fn(), close: vi.fn(), cells: [] as any[] }));
vi.mock('../hooks/useJobInbox', () => ({ useJobInbox: () => ({ tenant: 'tenant', registry: { cells: state.cells, reload: state.reload }, refresh: state.refresh }) }));
vi.mock('@colony2/shared/api', async importOriginal => ({ ...await importOriginal<typeof import('@colony2/shared/api')>(), getCell: vi.fn(), submitJob: vi.fn() }));
beforeEach(() => {
  vi.clearAllMocks(); state.cells = [registryCell()];
  vi.mocked(getCell).mockResolvedValue(registryCell());
  vi.mocked(submitJob).mockResolvedValue({ tenant_id: 'tenant', job_id: 'sent' } as any);
});
async function compose() {
  render(<MemoryRouter><ComposeJob open onClose={state.close} /></MemoryRouter>);
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Recipient cell' }));
  fireEvent.click(await screen.findByRole('option', { name: 'platform', exact: true }));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Please build this.' } });
}
it('validates body and rechecks the active recipient before submission', async () => {
  await compose();
  fireEvent.change(screen.getByLabelText('Subject'), { target: { value: 'Subject alone' } });
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: '  ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send', exact: true }));
  expect(await screen.findByText('Enter a message with text or an image')).toBeVisible();
  expect(submitJob).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Build' } });
  vi.mocked(getCell).mockResolvedValue({ ...registryCell(), state: 'RETIRED' });
  fireEvent.click(screen.getByRole('button', { name: 'Send', exact: true }));
  await screen.findByText('This cell is retired. Choose an active cell.');
  expect(submitJob).not.toHaveBeenCalled();
});
it('prevents concurrent sends and leaves job ID assignment to JobDB on every submission', async () => {
  await compose();
  let reject!: (error: Error) => void;
  vi.mocked(submitJob).mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; }));
  const send = screen.getByRole('button', { name: 'Send', exact: true });
  fireEvent.click(send); fireEvent.click(send);
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(1));
  expect(screen.getByRole('button', { name: 'Send', exact: true })).toBe(send);
  expect(send).toBeDisabled();
  expect(send).toHaveAttribute('aria-busy', 'true');
  reject(new Error('Response lost'));
  await screen.findByText('request failed', { exact: true });
  expect(screen.queryByRole('alert')).toBeNull();
  expect(send).toBeEnabled();
  expect(send).toHaveAttribute('aria-busy', 'false');
  expect(vi.mocked(submitJob).mock.calls[0][1]).not.toHaveProperty('job_id');
  fireEvent.click(send);
  await waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2));
  expect(vi.mocked(submitJob).mock.calls[1][1]).not.toHaveProperty('job_id');
  expect(vi.mocked(submitJob).mock.calls[1][1]).not.toHaveProperty('request_id');
  await waitFor(() => expect(state.close).toHaveBeenCalledTimes(1));
  expect(state.refresh).toHaveBeenCalledTimes(1);
});
it('forwards the original file objects without the former size and count limits', async () => {
  await compose();
  const body = 'First line\n\n  Code example\n';
  fireEvent.change(screen.getByLabelText('Subject'), { target: { value: '  Persist caller wait intent  ' } });
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: body } });
  const file = new File(['evidence'], 'notes.txt', { type: 'text/plain' });
  Object.defineProperty(file, 'size', { value: 26 * 1024 * 1024 });
  const files = [file, ...Array.from({ length: 10 }, (_, index) => new File([], `attachment-${index}.txt`))];
  fireEvent.change(screen.getByLabelText('Job attachments'), { target: { files } });
  expect(screen.getByText('notes.txt')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'Send', exact: true }));
  await waitFor(() => expect(submitJob).toHaveBeenCalledWith('tenant', {cell: 'platform', title: 'Persist caller wait intent', inputs: { prompt: body, type: 'build' }}, files));
});

it('refreshes recipients on reopening while keeping the draft', async () => {
  const view = render(<MemoryRouter><ComposeJob open={false} onClose={state.close} /></MemoryRouter>);
  expect(state.reload).not.toHaveBeenCalled();
  view.rerender(<MemoryRouter><ComposeJob open onClose={state.close} /></MemoryRouter>);
  expect(state.reload).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText('Message'), {target:{value:'Keep my draft'}});
  view.rerender(<MemoryRouter><ComposeJob open={false} onClose={state.close} /></MemoryRouter>);
  view.rerender(<MemoryRouter><ComposeJob open onClose={state.close} /></MemoryRouter>);
  expect(state.reload).toHaveBeenCalledTimes(2);
  expect(screen.getByLabelText('Message')).toHaveValue('Keep my draft');
});
