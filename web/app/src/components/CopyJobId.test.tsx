import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import CopyJobId from './CopyJobId';

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
const copyButton = () => screen.getByRole('button', { name: 'Copy job ID exact-id', exact: true });
function setup(writeText = vi.fn().mockRejectedValue(new Error('denied')), platform = 'MacIntel') {
  vi.stubGlobal('navigator', { clipboard: { writeText }, platform });
  const command = vi.fn(() => true);
  Object.defineProperty(document, 'execCommand', { configurable: true, value: command });
  const view = render(<CopyJobId jobId="exact-id" title="Title" />);
  return { writeText, command, ...view };
}
it.each(['reject', 'throw'])('JC-07/08/14 reveals local recovery after %s and retries only on fresh click', async kind => {
  const { command, writeText } = setup(vi.fn(() => { if (kind === 'throw') throw new Error('denied'); return Promise.reject(new Error('denied')); }));
  copyButton().focus(); fireEvent.click(copyButton());
  expect(await screen.findByText('Could not copy job ID.')).toBeVisible();
  expect(command).not.toHaveBeenCalled(); expect(copyButton()).toHaveFocus();
  const field = screen.getByRole('textbox', { name: 'Job ID', exact: true });
  expect(field).toHaveValue('exact-id'); expect(field).toHaveAttribute('readonly');
  expect(field).toHaveAccessibleDescription('Select the job ID and press ⌘C to copy.');
  const status = screen.getByRole('status'); expect(status).toHaveAttribute('aria-atomic', 'true');
  expect(status.querySelector('button,textarea')).toBeNull();
  const retry = screen.getByRole('button', { name: 'Copy another way' }); retry.focus(); fireEvent.click(retry);
  expect(command).toHaveBeenCalledExactlyOnceWith('copy'); expect(writeText).toHaveBeenCalledTimes(1);
  expect(status).toHaveTextContent('Job ID copied'); expect(copyButton()).toHaveFocus();
  expect(screen.queryByRole('textbox')).toBeNull();
});
it('JC-09 focuses selected manual recovery when retry fails, then original copying can recover', async () => {
  const { command, writeText } = setup(undefined, 'Linux'); command.mockReturnValue(false);
  fireEvent.click(copyButton());
  const retry = await screen.findByRole('button', { name: 'Copy another way' }); retry.focus(); fireEvent.click(retry);
  const field = screen.getByRole('textbox', { name: 'Job ID', exact: true }) as HTMLTextAreaElement;
  expect(field).toHaveFocus(); expect(field.value.slice(field.selectionStart, field.selectionEnd)).toBe('exact-id');
  expect(field).toHaveAccessibleDescription('Select the job ID and press Ctrl+C to copy.');
  expect(screen.queryByRole('button', { name: 'Copy another way' })).toBeNull();
  expect(screen.getByRole('status')).not.toHaveTextContent('Job ID copied');
  writeText.mockResolvedValue(undefined); fireEvent.click(copyButton());
  expect(screen.queryByText('Could not copy job ID.')).toBeNull();
  await screen.findByText('Job ID copied');
});
it.each([true, false])('JC-02/09 uses missing API compatibility result %s on the first click', result => {
  const { command } = setup(); vi.stubGlobal('navigator', { platform: 'Linux' }); command.mockReturnValue(result);
  fireEvent.click(copyButton()); expect(command).toHaveBeenCalledOnce();
  expect(screen.getByRole('status')).toHaveTextContent(result ? 'Job ID copied' : 'Could not copy job ID.');
  expect(screen.queryByRole('button', { name: 'Copy another way' })).toBeNull();
});
it('JC-10/13 ignores stale results and leaves focus where the user moved', async () => {
  let resolve!: () => void; let reject!: (error: Error) => void;
  const writeText = vi.fn().mockImplementationOnce(() => new Promise<void>(done => { resolve = done; }))
    .mockImplementationOnce(() => new Promise<void>((_, fail) => { reject = fail; }));
  const { rerender, unmount } = setup(writeText);
  fireEvent.click(copyButton()); fireEvent.click(copyButton());
  await act(async () => reject(new Error('denied')));
  await act(async () => resolve()); expect(screen.getByRole('status')).toHaveTextContent('Could not copy job ID.');
  let finish!: () => void; writeText.mockImplementation(() => new Promise<void>(done => { finish = done; }));
  fireEvent.click(copyButton());
  const other = document.createElement('button'); document.body.append(other); other.focus();
  rerender(<CopyJobId jobId="replacement" title="New" />);
  await act(async () => finish()); expect(screen.getByRole('status')).toBeEmptyDOMElement(); expect(other).toHaveFocus();
  fireEvent.click(screen.getByRole('button', { name: 'Copy job ID replacement' })); unmount();
  await act(async () => finish()); other.remove();
});
it('JC-07 leaves focus elsewhere when a pending write rejects', async () => {
  let reject!: (error: Error) => void;
  setup(vi.fn(() => new Promise<void>((_, fail) => { reject = fail; })));
  fireEvent.click(copyButton());
  const other = document.createElement('button'); document.body.append(other); other.focus();
  await act(async () => reject(new Error('denied')));
  await screen.findByText('Could not copy job ID.'); expect(other).toHaveFocus(); other.remove();
});
