import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ConfigProvider } from 'antd';
import { ChangesReadError, reviewChangesService, type ChangesHistory, type ChangesPreview } from '@colony2/shared';
import ReviewChanges from './ReviewChanges';
import ReviewWorkspace from './ReviewWorkspace';

const hash = (n: number) => n.toString(16).padStart(40, '0');
function history(selection = 'source', total = 4): ChangesHistory {
  return { version: 1, tenant: 'tenant', job_id: 'job', repository: 'example.test/repo', scope: 'main', cell: 'example',
    state: { job_id: 'job', task: 'producer/task#1', attempt: 2 }, boundary: 'after', active_base: hash(1), active_base_origin: 'parent',
    base: hash(1), base_origin: 'active', empty_base: false, unborn: false, head: hash(total + 1), base_tree: hash(101), head_tree: hash(105), object_format: 'sha1', sources: [], source_id: selection, selection,
    diff: 'complete', history: 'complete', complete: true, total, commits: Array.from({ length: total }, (_, i) => ({ hash: hash(i + 2), tree: hash(102 + i), parents: [hash(i + 1)], subject: `Edit ${i + 1}`, position: i + 1,
      author: { name: 'Ada', email: 'ada@example.test', time: '2026-01-01T00:00:00Z' }, committer: { name: 'Ada', email: 'ada@example.test', time: '2026-01-01T00:00:00Z' } })) };
}
function preview(h: ChangesHistory, patch = 'diff --git a/note.txt b/note.txt\n--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-old\n+new\n'): ChangesPreview {
  return { metadata: { ...h, from_commit: h.base, to_commit: h.head!, from_tree: h.base_tree, to_tree: h.head_tree, bytes: patch.length, diff: 'complete', complete: true }, patch, preview_complete: true };
}
beforeEach(() => {
  vi.stubGlobal('matchMedia', (query: string) => ({ matches: query.includes('min-width'), media: query, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {} }));
  vi.spyOn(reviewChangesService, 'history').mockResolvedValue(history());
  vi.spyOn(reviewChangesService, 'diff').mockResolvedValue(preview(history()));
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

it('shows boundaries, source and ordered commits and selects forward/reverse inclusive ranges', async () => {
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('4 commits since base');
  expect(screen.getByText('example.test/repo')).toBeInTheDocument();
  expect(screen.getByText('Active')).toBeInTheDocument();
  const rows = screen.getByRole('group', { name: 'Commits oldest first' }).querySelectorAll('button');
  expect([...rows].map(row => row.textContent)).toEqual([1, 2, 3, 4].map(i => expect.stringContaining(`${i}. Edit ${i}`)));
  fireEvent.click(rows[1]);
  await waitFor(() => expect(reviewChangesService.diff).toHaveBeenLastCalledWith('tenant', 'job', 'source', { commit: hash(3) }, expect.any(AbortSignal)));
  fireEvent.click(rows[3], { shiftKey: true });
  await waitFor(() => expect(reviewChangesService.diff).toHaveBeenLastCalledWith('tenant', 'job', 'source', { from: hash(3), through: hash(5) }, expect.any(AbortSignal)));
  fireEvent.click(rows[0], { shiftKey: true });
  expect(screen.getByRole('status')).toHaveTextContent('Selected commits 1 through 2');
  expect(rows[0]).toHaveAttribute('aria-pressed', 'true');
  expect(rows[2]).toHaveAttribute('aria-pressed', 'false');
  fireEvent.click(screen.getByRole('button', { name: 'All changes' }));
  expect(screen.getByRole('status')).toHaveTextContent('All changes');
});

it('keeps keyboard focus separate and clamps Shift+Arrow ranges', async () => {
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  const first = await screen.findByRole('button', { name: /^1\. Edit 1\b/ });
  fireEvent.focus(first);
  fireEvent.keyDown(first, { key: 'ArrowDown' });
  expect(screen.getByRole('status')).toHaveTextContent('All changes');
  await waitFor(() => expect(screen.getByRole('button', { name: /2. Edit 2/ })).toHaveFocus());
  fireEvent.keyDown(screen.getByRole('button', { name: /2. Edit 2/ }), { key: 'ArrowDown', shiftKey: true });
  expect(screen.getByRole('status')).toHaveTextContent('Selected commits 2 through 3');
});

it('loads metadata pages before counting and retains ranges across displayed pages', async () => {
  const full = history('many', 51);
  vi.mocked(reviewChangesService.history).mockResolvedValueOnce({ ...full, commits: full.commits.slice(0, 50), next_cursor: 'page2' }).mockResolvedValueOnce({ ...full, commits: full.commits.slice(50) });
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('51 commits since base');
  expect(reviewChangesService.history).toHaveBeenLastCalledWith('tenant', 'job', expect.any(AbortSignal), 'page2');
  fireEvent.click(screen.getByRole('button', { name: /^1\. Edit 1\b/ }));
  fireEvent.click(screen.getByTitle('2'));
  fireEvent.click(screen.getByRole('button', { name: /51. Edit 51/ }), { shiftKey: true });
  expect(screen.getByRole('status')).toHaveTextContent('Selected commits 1 through 51');
  await waitFor(() => expect(reviewChangesService.diff).toHaveBeenLastCalledWith('tenant', 'job', 'many', { from: hash(2), through: hash(52) }, expect.any(AbortSignal)));
});

it.each(['From commit', 'Through commit'])('keeps a tabbable row on the displayed page after choosing a distant %s endpoint', async endpoint => {
  const full = history('many', 52);
  vi.mocked(reviewChangesService.history).mockResolvedValueOnce({ ...full, commits: full.commits.slice(0, 50), next_cursor: 'page2' }).mockResolvedValueOnce({ ...full, commits: full.commits.slice(50) });
  // Render all dropdown options in jsdom; production virtualization is exercised in the browser.
  render(<ConfigProvider virtual={false}><ReviewChanges projectId="tenant" jobId="job" /></ConfigProvider>);
  await screen.findByText('52 commits since base');
  fireEvent.mouseDown(screen.getByRole('combobox', { name: endpoint }));
  fireEvent.click(screen.getByRole('option', { name: `52. Edit 52 (${hash(53).slice(0, 8)})` }));
  const last = screen.getByRole('button', { name: /^52\. Edit 52\b/ });
  expect(last).toHaveAttribute('tabindex', '0');
  expect(screen.getByRole('group', { name: 'Commits oldest first' }).querySelectorAll('button[tabindex="0"]')).toHaveLength(1);
  last.focus();
  fireEvent.keyDown(last, { key: 'ArrowUp', shiftKey: true });
  await waitFor(() => expect(screen.getByRole('button', { name: /^51\. Edit 51\b/ })).toHaveFocus());
  expect(screen.getByRole('status')).toHaveTextContent('Selected commits 51 through 52');
  fireEvent.keyDown(screen.getByRole('button', { name: /^51\. Edit 51\b/ }), { key: 'ArrowUp', shiftKey: true });
  await waitFor(() => expect(screen.getByRole('button', { name: /^50\. Edit 50\b/ })).toHaveFocus());
  expect(screen.getByRole('status')).toHaveTextContent('Selected commits 50 through 52');
  expect(screen.getByRole('button', { name: /^50\. Edit 50\b/ })).toHaveAttribute('tabindex', '0');
});

it('clears old patches immediately and ignores obsolete responses even when abort is ignored', async () => {
  let resolve!: (value: ChangesPreview) => void;
  vi.mocked(reviewChangesService.diff).mockImplementationOnce(() => new Promise(done => { resolve = done; })).mockResolvedValueOnce(preview(history(), ''));
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  fireEvent.click(await screen.findByRole('button', { name: /2. Edit 2/ }));
  await screen.findByText('No net file changes.');
  await act(async () => resolve(preview(history(), 'OBSOLETE PATCH')));
  expect(screen.queryByText('OBSOLETE PATCH')).not.toBeInTheDocument();
});

it('ignores late history after tenant/job changes and refresh discards the previous selection', async () => {
  let resolve!: (value: ChangesHistory) => void;
  vi.mocked(reviewChangesService.history).mockImplementationOnce(() => new Promise(done => { resolve = done; })).mockResolvedValue(history('new'));
  const { rerender } = render(<ReviewChanges projectId="old" jobId="old" />);
  rerender(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('4 commits since base');
  await act(async () => resolve({ ...history('old'), repository: 'obsolete repository' }));
  expect(screen.queryByText('obsolete repository')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: /2. Edit 2/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh changes' }));
  await screen.findByText('4 commits since base');
  expect(screen.getByRole('status')).toHaveTextContent('All changes');
});

it('retries a failed diff with the same selection and clears file/source state on comparison changes', async () => {
  const patch = 'diff --git a/a b/a\nold mode 100644\nnew mode 100755\ndiff --git a/b b/b\nold mode 100644\nnew mode 100755\n';
  vi.mocked(reviewChangesService.diff).mockRejectedValueOnce(new ChangesReadError('Temporary backend failure', 502, { version: 1, code: 'backend' })).mockResolvedValue(preview(history(), patch));
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('Temporary backend failure');
  fireEvent.click(screen.getByRole('button', { name: 'Retry', exact: true }));
  await screen.findByRole('combobox', { name: 'Patch file' });
  expect(vi.mocked(reviewChangesService.diff).mock.calls.slice(0, 2).map(call => call.slice(0, 4))).toEqual([
    ['tenant', 'job', 'source', { all: true }], ['tenant', 'job', 'source', { all: true }],
  ]);
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Patch file' }));
  fireEvent.click(screen.getByTitle('a/b b/b'));
  fireEvent.click(screen.getByRole('button', { name: 'Source', exact: true }));
  expect(screen.getByLabelText('Document source')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /2. Edit 2/ }));
  await screen.findByRole('combobox', { name: 'Patch file' });
  expect(screen.queryByLabelText('Document source')).not.toBeInTheDocument();
  expect(screen.getByTitle('a/a b/a')).toBeVisible();
});

it('shows unsupported history beside All net changes and disables commit selection', async () => {
  const h = { ...history(), history: 'unsupported' as const, complete: false, commits: [], total: 0, diagnostic: { version: 1, code: 'unsupported_history' } };
  vi.mocked(reviewChangesService.history).mockResolvedValue(h);
  vi.mocked(reviewChangesService.diff).mockResolvedValue(preview(h));
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('Unsupported history: merge commit in captured range');
  await screen.findByLabelText('Unified diff');
  expect(screen.queryByLabelText('From commit')).not.toBeInTheDocument();
  expect(screen.queryByText('0 commits since base')).not.toBeInTheDocument();
});

it('distinguishes zero commits, empty net patch, unavailable history and stale selections', async () => {
  vi.mocked(reviewChangesService.history).mockResolvedValue(history('empty', 0));
  vi.mocked(reviewChangesService.diff).mockResolvedValue(preview(history('empty', 0), ''));
  const { unmount } = render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('No commits since base.');
  await screen.findByText('No net file changes.');
  unmount();
  vi.mocked(reviewChangesService.history).mockRejectedValue(new ChangesReadError('No recorded workspace', 422, { version: 1, code: 'workspace_unavailable' }));
  const next = render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('Change history is unavailable for this job.');
  expect(screen.queryByText('No commits since base.')).not.toBeInTheDocument();
  next.unmount();
  vi.mocked(reviewChangesService.history).mockResolvedValue(history());
  vi.mocked(reviewChangesService.diff).mockRejectedValue(new ChangesReadError('State advanced', 409, { version: 1, code: 'stale_selection' }));
  render(<ReviewChanges projectId="tenant" jobId="job" />);
  await screen.findByText('Recorded workspace changed. Refresh changes to view its latest status.');
});

it('keeps documents, display mode, draft and attachments mounted across tabs, resetting for a new occurrence', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('# Review document\n\n[Supporting](support.md)')));
  const props = { projectId: 'tenant', jobId: 'job', requestId: 'first', documents: { doc: { name: 'review.md' } }, response: <><input aria-label="Response draft" /><input type="file" aria-label="Attachment" /></> };
  const { rerender } = render(<ReviewWorkspace key="first" {...props} />);
  await screen.findByRole('heading', { name: 'Review document' });
  fireEvent.change(screen.getByLabelText('Response draft'), { target: { value: 'Keep my draft' } });
  const attachment = screen.getByLabelText('Attachment');
  fireEvent.change(attachment, { target: { files: [new File(['answer'], 'answer.txt')] } });
  fireEvent.click(screen.getByRole('button', { name: 'Document options' }));
  fireEvent.click(screen.getByRole('radio', { name: 'Markdown', exact: true }));
  fireEvent.click(screen.getByRole('tab', { name: 'Changes', exact: true }));
  await screen.findByText('4 commits since base');
  fireEvent.click(screen.getByRole('button', { name: /2. Edit 2/ }));
  fireEvent.click(screen.getByRole('tab', { name: 'Documents', exact: true }));
  expect(screen.getByLabelText('Markdown source')).toBeVisible();
  expect(screen.getByLabelText('Response draft')).toHaveValue('Keep my draft');
  expect(screen.getByLabelText('Attachment')).toBe(attachment);
  fireEvent.click(screen.getByRole('tab', { name: 'Changes', exact: true }));
  expect(screen.getByRole('status')).toHaveTextContent('Selected commits 2 through 2');
  rerender(<ReviewWorkspace key="second" {...props} requestId="second" />);
  expect(screen.getByRole('tab', { name: 'Documents', exact: true })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByLabelText('Response draft')).toHaveValue('');
});
