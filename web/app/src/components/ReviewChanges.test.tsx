import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ChangesReadError, reviewChangesService, type WorkspaceDiffPreview } from '@colony2/shared';
import ReviewChanges from './ReviewChanges';
import ReviewWorkspace from './ReviewWorkspace';

function preview(patch = 'diff --git a/note.txt b/note.txt\n--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-old\n+new\n'): WorkspaceDiffPreview {
  return { metadata: { version: 1, status: 'available', requested: { job_id: 'job' }, resolved: { status: 'persisted', boundary: 'after' }, observation: 'obs', cutoff: 'cutoff', workspace: { repository: 'example.test/repo', base_commit: '1'.repeat(40), head_commit: '2'.repeat(40) }, bytes: patch.length }, patch, preview_complete: true, download_url: '/api/projects/tenant/jobs/producer/tasks/7/artifacts/diff_from_base.diff' };
}
beforeEach(() => {
  vi.stubGlobal('matchMedia', (query: string) => ({ matches: query.includes('min-width'), media: query, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {} }));
  vi.spyOn(reviewChangesService, 'workspaceDiff').mockResolvedValue(preview());
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
it('opens one latest request and renders both layouts and the resolved saved-file link without commit controls', async () => {
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByLabelText('Unified diff');
  expect(reviewChangesService.workspaceDiff).toHaveBeenCalledExactlyOnceWith('tenant', 'job', {}, expect.any(AbortSignal));
  expect(screen.getByText('example.test/repo')).toBeVisible();
  expect(screen.getByRole('link', { name: 'Download saved diff' })).toHaveAttribute('href', preview().download_url);
  expect(screen.queryByLabelText('From commit')).not.toBeInTheDocument();
  fireEvent.click(screen.getByText('Side by side', { exact: true }));
  expect(screen.getByLabelText('Side by side diff')).toHaveTextContent('+new');
  fireEvent.click(screen.getByRole('button', { name: 'Source', exact: true }));
  expect(screen.getByLabelText('Document source')).toHaveTextContent('diff --git');
});
it('clears preview and download during refresh, retains layout preference, and ignores obsolete responses', async () => {
  render(<ReviewChanges projectId="tenant" jobId="job" />); await screen.findByLabelText('Unified diff');
  fireEvent.click(screen.getByText('Side by side', { exact: true }));
  let resolve!: (p: WorkspaceDiffPreview) => void;
  vi.mocked(reviewChangesService.workspaceDiff).mockImplementationOnce(() => new Promise(done => { resolve = done; })).mockResolvedValueOnce(preview());
  fireEvent.click(screen.getByRole('button', { name: 'Refresh changes' }));
  expect(screen.queryByRole('link', { name: 'Download saved diff' })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Refresh changes' }));
  await screen.findByLabelText('Side by side diff');
  await act(async () => resolve(preview('OBSOLETE PATCH')));
  expect(screen.queryByText('OBSOLETE PATCH')).not.toBeInTheDocument();
});
it('ignores ignored-abort tenant/job responses and hides old labels and download immediately', async () => {
  let resolve!: (p: WorkspaceDiffPreview) => void;
  vi.mocked(reviewChangesService.workspaceDiff).mockImplementationOnce(() => new Promise(done => { resolve = done; }));
  const { rerender } = render(<ReviewChanges projectId="old" jobId="old" />);
  rerender(<ReviewChanges projectId="tenant" jobId="job" />); await screen.findByLabelText('Unified diff');
  await act(async () => resolve(preview('OLD TENANT')));
  expect(screen.queryByText('OLD TENANT')).not.toBeInTheDocument();
  vi.mocked(reviewChangesService.workspaceDiff).mockRejectedValueOnce(new ChangesReadError('Upgrade helper', 501, { version: 1, code: 'unsupported' }));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh changes' }));
  await screen.findByText('Upgrade helper');
  expect(screen.queryByRole('link', { name: 'Download saved diff' })).not.toBeInTheDocument();
});
it.each(['empty', 'unavailable'])('explains %s without a fabricated download', async status => {
  const p = preview(''); p.metadata.status = status as 'empty' | 'unavailable'; delete p.download_url;
  vi.mocked(reviewChangesService.workspaceDiff).mockResolvedValue(p);
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText(status === 'empty' ? 'No file changes in this saved comparison.' : 'No saved workspace diff is available for this recorded point.');
  expect(screen.queryByRole('link', { name: 'Download saved diff' })).not.toBeInTheDocument();
});
it('keeps binary notices and malformed or shortened source readable', async () => {
  vi.mocked(reviewChangesService.workspaceDiff).mockResolvedValue(preview('diff --git a/binary b/binary\nBinary files a/binary and b/binary differ\n'));
  const view = render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('Binary file changed. Its contents were not included in this saved diff.'); view.unmount();
  vi.mocked(reviewChangesService.workspaceDiff).mockResolvedValue(preview('unexpected saved source'));
  const second = render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('unexpected saved source'); second.unmount();
  vi.mocked(reviewChangesService.workspaceDiff).mockResolvedValue({ ...preview('partial source'), preview_complete: false });
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('Preview truncated. Search covers only loaded content. Download the complete original.');
  expect(screen.getByRole('link', { name: 'Download saved diff' })).toBeVisible();
});
it('preserves document display, drafts and attachments through tabs and refresh', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('# Review document')));
  const props = { projectId: 'tenant', jobId: 'job', requestId: 'first', documents: { doc: { name: 'review.md' } }, response: <><input aria-label="Response draft" /><input type="file" aria-label="Attachment" /></> };
  render(<ReviewWorkspace {...props} />); await screen.findByRole('heading', { name: 'Review document' });
  fireEvent.change(screen.getByLabelText('Response draft'), { target: { value: 'Keep my draft' } });
  const attachment = screen.getByLabelText('Attachment');
  fireEvent.change(attachment, { target: { files: [new File(['answer'], 'answer.txt')] } });
  fireEvent.click(screen.getByRole('button', { name: 'Document options' }));
  fireEvent.click(screen.getByRole('radio', { name: 'Markdown', exact: true }));
  fireEvent.click(screen.getByRole('tab', { name: 'Changes', exact: true })); await screen.findByLabelText('Unified diff');
  fireEvent.click(screen.getByRole('button', { name: 'Refresh changes' }));
  await waitFor(() => expect(reviewChangesService.workspaceDiff).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole('tab', { name: 'Documents', exact: true }));
  expect(screen.getByLabelText('Markdown source')).toBeVisible();
  expect(screen.getByLabelText('Response draft')).toHaveValue('Keep my draft');
  expect(screen.getByLabelText('Attachment')).toBe(attachment);
});
