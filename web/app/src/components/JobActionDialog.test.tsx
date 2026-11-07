import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { APIError, archiveJob, cancelJob, getJob, DEFAULT_CANCELLATION_REASON } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import JobActionDialog from './JobActionDialog';
import CancelJobButton from './CancelJobButton';
import { jobIdentity } from '../test/cellFixtures';

vi.mock('@colony2/shared/api', async original => ({
  ...await original<typeof import('@colony2/shared/api')>(),
  cancelJob: vi.fn(), archiveJob: vi.fn(), getJob: vi.fn(),
}));
const ready = { ...jobIdentity, job_id: 'one', status: 'READY', store: 'ACTIVE' } as RecipeJob;
beforeEach(() => { vi.mocked(cancelJob).mockReset().mockResolvedValue(); vi.mocked(archiveJob).mockReset().mockResolvedValue(); vi.mocked(getJob).mockReset().mockResolvedValue(ready); });
const field = () => screen.getByRole('textbox', { name: 'Cancellation message (optional)' });
const confirm = async (name: string) => {
  const button = within(screen.getByRole('dialog')).getByRole('button', {name,exact:true});
  await waitFor(() => { expect(button).toBeEnabled(); expect(button).not.toHaveClass('ant-btn-loading'); });
  fireEvent.click(button);
};

it.each(['cancel', 'archive'] as const)('retains a single %s message across failure and sends normalized Unicode', async action => {
  const mutation = action === 'cancel' ? cancelJob : archiveJob;
  vi.mocked(mutation).mockRejectedValueOnce(new Error('retry'));
  const completed = vi.fn();
  render(<JobActionDialog projectId="tenant" jobIds={['one']} job={ready} action={action} onClose={vi.fn()} onComplete={completed} />);
  expect(mutation).not.toHaveBeenCalled();
  fireEvent.change(field(), { target: { value: '  雪🧪\n<script>text</script>  ' } });
  await confirm(action === 'cancel' ? 'Cancel job' : 'Archive job');
  await screen.findByText('1 job operation failed');
  expect(field()).toHaveValue('  雪🧪\n<script>text</script>  ');
  expect(field()).toBeEnabled();
  await confirm(action === 'cancel' ? 'Cancel job' : 'Archive job');
  await waitFor(() => expect(completed).toHaveBeenCalledWith(['one']));
  expect(mutation).toHaveBeenNthCalledWith(2, 'tenant', 'one', '雪🧪\n<script>text</script>');
});

it('locks the accepted text and explains archive-only retries', async () => {
  vi.mocked(archiveJob).mockRejectedValueOnce(new APIError('Cancellation succeeded; archiving failed', 500, 'archive_failed_after_cancel'));
  render(<JobActionDialog projectId="tenant" jobIds={['one']} job={ready} action="archive" onClose={vi.fn()} onComplete={vi.fn()} />);
  fireEvent.change(field(), { target: { value: 'accepted' } });
  await confirm('Archive job');
  await screen.findByText(/only archiving remains/);
  expect(field()).toBeDisabled();
  await confirm('Archive job');
  await waitFor(() => expect(archiveJob).toHaveBeenCalledTimes(2));
  expect(archiveJob).toHaveBeenLastCalledWith('tenant', 'one', 'accepted');
});

it.each(['cancel', 'archive'] as const)('keeps original bulk %s mode when one failure remains', async action => {
  const mutation = action === 'cancel' ? cancelJob : archiveJob;
  vi.mocked(mutation).mockImplementation(async (_tenant, id) => { if (id === 'two') throw new Error('retry'); });
  const completed = vi.fn();
  render(<JobActionDialog projectId="tenant" jobIds={['one','two']} action={action} onClose={vi.fn()} onComplete={completed} />);
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  await confirm(action === 'cancel' ? 'Cancel 2 jobs' : 'Archive 2 jobs');
  await screen.findByText('1 job operation failed');
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  expect(completed).toHaveBeenCalledWith(['one']);
  vi.mocked(mutation).mockResolvedValue();
  await confirm(action === 'cancel' ? 'Cancel job' : 'Archive job');
  await waitFor(() => expect(mutation).toHaveBeenCalledTimes(3));
  expect(mutation).toHaveBeenLastCalledWith('tenant','two',undefined);
});

it('counts code points and prevents overlong confirmation, accepting 4,000 emoji', async () => {
  render(<JobActionDialog projectId="tenant" jobIds={['one']} action="cancel" onClose={vi.fn()} onComplete={vi.fn()} />);
  fireEvent.change(field(), { target: { value: '🧪'.repeat(4001) } });
  expect(screen.getByText(/4,001 \/ 4,000/)).toBeVisible();
  expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel job', exact: true })).toBeDisabled();
  fireEvent.change(field(), { target: { value: '🧪'.repeat(4000) } });
  await confirm('Cancel job');
  await waitFor(() => expect(cancelJob).toHaveBeenCalledWith('tenant', 'one', '🧪'.repeat(4000)));
});

it('uses the explicit default for a blank single unfinished archive', async () => {
  render(<JobActionDialog projectId="tenant" jobIds={['one']} job={ready} action="archive" onClose={vi.fn()} onComplete={vi.fn()} />);
  fireEvent.change(field(), { target: { value: ' \n ' } }); await confirm('Archive job');
  await waitFor(() => expect(archiveJob).toHaveBeenCalledWith('tenant','one',DEFAULT_CANCELLATION_REASON));
});

it('resolves unknown archive status before allowing confirmation and retries failed reads', async () => {
  vi.mocked(getJob).mockRejectedValueOnce(new Error('status unavailable'));
  render(<JobActionDialog projectId="tenant" jobIds={['one']} action="archive" onClose={vi.fn()} onComplete={vi.fn()} />);
  await screen.findByText('status unavailable');
  expect(within(screen.getByRole('dialog')).getByRole('button', { name:'Archive job', exact:true })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name:'Retry status check' }));
  await screen.findByRole('textbox');
  expect(archiveJob).not.toHaveBeenCalled();
});

it('omits reasons for finished archive and retains an existing cancellation message', async () => {
  render(<JobActionDialog projectId="tenant" jobIds={['one']} job={{ ...ready, status:'COMPLETED', store:'ARCHIVED', completion_status:'success' }} action="archive" onClose={vi.fn()} onComplete={vi.fn()} />);
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument(); await confirm('Archive job');
  await waitFor(() => expect(archiveJob).toHaveBeenCalledWith('tenant','one',undefined));
});

it('starts empty on reopen, dismisses without mutation, and restores trigger focus', async () => {
  render(<CancelJobButton projectId="tenant" jobId="one" onCancelled={vi.fn()} />);
  const trigger = screen.getByRole('button', { name:'Cancel job' }); trigger.focus(); fireEvent.click(trigger);
  fireEvent.change(field(), { target:{value:'discard'} }); await confirm('Keep jobs');
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(cancelJob).not.toHaveBeenCalled(); fireEvent.click(trigger); expect(field()).toHaveValue('');
});

it('locks editing and dismissal during submission and keeps concurrency at five', async () => {
  const releases: (() => void)[] = [];
  vi.mocked(cancelJob).mockImplementation(() => new Promise(resolve => releases.push(resolve)));
  const close = vi.fn();
  const { unmount } = render(<JobActionDialog projectId="tenant" jobIds={['one']} action="cancel" onClose={close} onComplete={vi.fn()} />);
  await confirm('Cancel job'); expect(field()).toBeDisabled();
  expect(screen.getByRole('button',{name:'Keep jobs'})).toBeDisabled();
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button',{name:'Cancel job',exact:true})); expect(cancelJob).toHaveBeenCalledTimes(1);
  await act(async () => releases.shift()!()); unmount(); vi.mocked(cancelJob).mockClear();
  render(<JobActionDialog projectId="tenant" jobIds={Array.from({length:12},(_,i)=>String(i))} action="cancel" onClose={close} onComplete={vi.fn()} />);
  await confirm('Cancel 12 jobs'); expect(cancelJob).toHaveBeenCalledTimes(5);
  await act(async () => releases.splice(0).forEach(resolve => resolve())); expect(cancelJob).toHaveBeenCalledTimes(10);
  await act(async () => releases.splice(0).forEach(resolve => resolve())); expect(cancelJob).toHaveBeenCalledTimes(12);
  await act(async () => releases.splice(0).forEach(resolve => resolve()));
});

it('retains an already requested cancellation while keeping stale completion details editable', async () => {
  const props = {projectId:'tenant',jobIds:['one'],action:'archive' as const,onClose:vi.fn(),onComplete:vi.fn()};
  const {unmount} = render(<JobActionDialog {...props} job={{...ready,cancel_requested:true}} />);
  expect(field()).toBeDisabled();
  expect(screen.getByText(/only archiving remains/)).toBeVisible();
  unmount();
  render(<JobActionDialog {...props} job={{...ready,completion_status:'cancelled'}} />);
  expect(field()).toBeEnabled();
  expect(screen.queryByText(/only archiving remains/)).not.toBeInTheDocument();
});
