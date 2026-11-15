import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, configure, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Routes, Route, useLocation, useNavigate, useNavigationType, type Location, type NavigateFunction, type NavigationType } from 'react-router-dom';
import { inputActivityService, type UserInputDetails } from '@colony2/shared';
import InputDetailPage from './InputDetailPage';

configure({ asyncUtilTimeout: 5000 });
vi.mock('./ReviewJobContext', () => ({ default: () => null }));
vi.mock('./CancelJobButton', () => ({ default: () => <button>Cancel job</button> }));
const base = '/project/tenant/reviews/job';
const docs = Object.fromEntries(['a', 'b/雪 &+%', 'c'].map((id, i) => [id, { kind: 'stored' as const, name: `${String.fromCharCode(97 + i)}.md`, stored: { key: { jobId: 'producer', taskOrdinal: 1, name: `${String.fromCharCode(97 + i)}.md` } } }]));
function details(): UserInputDetails {
  return { jobId: 'job', status: 'pending', startTime: '2026-11-14T00:00:00Z', form: {
    kind: 'review', title: 'Review', request_id: 'r', documents: docs, fields: [
      { id: 'decision', type: 'multiple_choice', question: 'Decision', required: true, options: [{ value: 'approve', label: 'Approve' }, { value: 'revise', label: 'Revise' }] },
      { id: 'feedback', type: 'paragraph_text', question: 'Reason' },
      { id: 'attachment', type: 'file_upload', question: 'Annotations' },
    ],
  } } as UserInputDetails;
}
let data: UserInputDetails;
let getDetails: ReturnType<typeof vi.spyOn>;
let submit: ReturnType<typeof vi.spyOn>;
beforeEach(() => {
  data = details();
  getDetails = vi.spyOn(inputActivityService, 'getInputDetails').mockImplementation(async () => data);
  submit = vi.spyOn(inputActivityService, 'submitReview').mockResolvedValue(undefined as never);
  vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(`# Body ${new URL(url).searchParams.get('document_id')}`)));
});
afterEach(() => { getDetails.mockRestore(); submit.mockRestore(); vi.unstubAllGlobals(); });
interface ObservedRouter { state: { location: Location; historyAction: NavigationType }; navigate: NavigateFunction }
function HistoryProbe({ router }: { router: ObservedRouter }) {
  router.state = { location: useLocation(), historyAction: useNavigationType() };
  router.navigate = useNavigate();
  return null;
}
function mount(search = '') {
  const router = {} as ObservedRouter;
  const view = render(<MemoryRouter initialEntries={['/queue', base + search]} initialIndex={1}>
    <HistoryProbe router={router} />
    <Routes>
      <Route path="/queue" element={<h1>Previous queue</h1>} />
      <Route path="/project/:projectId/reviews" element={<h1>Review queue</h1>} />
      <Route path="/project/:projectId/reviews/:jobId" element={<InputDetailPage projectId="tenant" />} />
    </Routes>
  </MemoryRouter>);
  const go = async (to: number | string) => { await act(async () => { if (typeof to === 'number') router.navigate(to); else router.navigate(to); }); };
  return { ...view, router, go };
}
const currentId = (router: ObservedRouter) => new URLSearchParams(router.state.location.search).get('document_id');
async function choose(name: string) {
  if (screen.queryByRole('button', { name: 'Show documents' })) fireEvent.click(screen.getByRole('button', { name: 'Show documents' }));
  fireEvent.click(screen.getByRole('navigation', { name: 'Review documents' }).querySelectorAll('button')[['a', 'b', 'c'].indexOf(name)]);
  await screen.findByRole('heading', { name: `Body ${name === 'b' ? 'b/雪 &+%' : name}` });
}

it.each(['', '?document_id=a', '?request_id=r'])('H06-08 replaces partial link once: %s', async search => {
  const { router, go } = mount(search);
  await screen.findByRole('heading', { name: 'Body a' });
  await waitFor(() => expect(Object.fromEntries(new URLSearchParams(router.state.location.search))).toEqual({ request_id: 'r', document_id: 'a' }));
  expect(router.state.historyAction).toBe('REPLACE');
  await go(-1);
  expect(screen.getByRole('heading', { name: 'Previous queue' })).toBeVisible();
});
it('H09-11 preserves drafts, uploads and details through exact A/B/C history; no implicit submit', async () => {
  const { router, go, container } = mount('?keep=x&keep=y&q=foo+bar&cellId=platform&sort=title&order=descend');
  await screen.findByRole('heading', { name: 'Body a' });
  fireEvent.click(screen.getByRole('button', { name: 'Reject', exact: true }));
  await userEvent.type(screen.getByLabelText('Reason'), 'Draft stays');
  const file = new File(['notes'], 'notes.md', { type: 'text/markdown' });
  await userEvent.upload(container.querySelector('input[type=file]')!, file);
  const upload = container.querySelector('input[type=file]')!;
  await choose('b');
  expect(router.state.historyAction).toBe('PUSH');
  await choose('c');
  await choose('c');
  for (const [step, id] of [[-1, 'b/雪 &+%'], [-1, 'a'], [1, 'b/雪 &+%'], [1, 'c']] as const) {
    await go(step);
    await screen.findByRole('heading', { name: `Body ${id}` });
    expect(currentId(router)).toBe(id);
    expect(router.state.historyAction).toBe('POP');
    expect(screen.getByLabelText('Reason')).toHaveValue('Draft stays');
    expect(container.querySelector('input[type=file]')).toBe(upload);
    expect(screen.getByText('notes.md')).toBeVisible();
  }
  expect(new URLSearchParams(router.state.location.search).getAll('keep')).toEqual(['x', 'y']);
  expect(getDetails).toHaveBeenCalledTimes(1);
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'OK', exact: true }));
  await waitFor(() => expect(submit).toHaveBeenCalledWith('tenant', 'job', 'r', expect.objectContaining({ fields: { decision: 'revise', feedback: 'Draft stays' }, attachments: { attachment: file } })));
  await screen.findByRole('heading', { name: 'Review queue' });
  expect(router.state.location.search).toBe('?q=foo+bar&cellId=platform&sort=title&order=descend');
});
it.each(['?request_id=old&document_id=a', '?request_id=r&request_id=r&document_id=a', '?document_id=a&document_id=a'])('H12-14 withholds stale/invalid review and replaces only after recovery: %s', async search => {
  const { router, go } = mount(search);
  const recover = await screen.findByRole('button', { name: 'Open current review' });
  expect(router.state.location.search).toBe(search);
  expect(screen.queryByRole('button', { name: 'Approve', exact: true })).toBeNull();
  expect(screen.queryByLabelText('Selected document')).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(recover);
  await screen.findByRole('heading', { name: 'Body a' });
  expect(router.state.historyAction).toBe('REPLACE');
  expect(Object.fromEntries(new URLSearchParams(router.state.location.search))).toEqual({ request_id: 'r', document_id: 'a' });
  await go(-1);
  expect(screen.getByRole('heading', { name: 'Previous queue' })).toBeVisible();
});
it('H15 corrects missing document once, retaining warning until a deliberate selection', async () => {
  const { router, go } = mount('?request_id=r&document_id=gone');
  await screen.findByRole('heading', { name: 'Body a' });
  await waitFor(() => expect(currentId(router)).toBe('a'));
  expect(screen.getByText(/linked document is unavailable/)).toBeVisible();
  await choose('b');
  expect(screen.queryByText(/linked document is unavailable/)).toBeNull();
  await go(-1);
  await screen.findByRole('heading', { name: 'Body a' });
  await go(-1);
  expect(screen.getByRole('heading', { name: 'Previous queue' })).toBeVisible();
});
it('H16 removal replaces selection and reattachment does not resurrect it', async () => {
  const { router, go } = mount('?request_id=r&document_id=c');
  await screen.findByRole('heading', { name: 'Body c' });
  data.form.documents = { a: docs.a };
  await go(base + '?request_id=r&document_id=c&refresh=1');
  await screen.findByRole('heading', { name: 'Body a' });
  expect(currentId(router)).toBe('a');
  expect(router.state.historyAction).toBe('REPLACE');
  data.form.documents = docs;
  await go(base + '?request_id=r&document_id=a&refresh=2');
  expect(currentId(router)).toBe('a');
});
it('H17 empty catalog removes document identity, retains warning and adds no history', async () => {
  data.form.documents = {};
  const { router, go } = mount('?document_id=gone');
  await screen.findByText('No documents attached');
  await waitFor(() => expect(router.state.location.search).toBe('?request_id=r'));
  expect(screen.getByText(/linked document is unavailable/)).toBeVisible();
  expect(fetch).not.toHaveBeenCalled();
  await go(-1);
  expect(screen.getByRole('heading', { name: 'Previous queue' })).toBeVisible();
});
it('H18 keeps the URL intact while request details load or fail', async () => {
  let reject!: (error: Error) => void;
  getDetails.mockImplementation(() => new Promise((_, fail) => { reject = fail; }));
  const { router } = mount('?request_id=old&document_id=gone');
  expect(screen.getByText('Loading request...')).toBeVisible();
  expect(router.state.location.search).toBe('?request_id=old&document_id=gone');
  await act(async () => reject(new Error('offline')));
  await screen.findByText(/Failed to load request details/);
  expect(router.state.location.search).toBe('?request_id=old&document_id=gone');
  expect(fetch).not.toHaveBeenCalled();
});
it('H19 new request rejects old URL identity and resets its response draft', async () => {
  const { router, go } = mount();
  await screen.findByRole('heading', { name: 'Body a' });
  fireEvent.click(screen.getByRole('button', { name: 'Reject', exact: true }));
  await userEvent.type(screen.getByLabelText('Reason'), 'Old request');
  data.form.request_id = 'new';
  await go(base + router.state.location.search + '&refresh=1');
  await screen.findByText('This linked review is no longer current');
  expect(screen.queryByLabelText('Reason')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Open current review' }));
  await screen.findByRole('button', { name: 'Reject', exact: true });
  fireEvent.click(screen.getByRole('button', { name: 'Reject', exact: true }));
  expect(screen.getByLabelText('Reason')).toHaveValue('');
  expect(new URLSearchParams(router.state.location.search).get('request_id')).toBe('new');
  fireEvent.click(screen.getByRole('button', { name: /Back to Reviews/ }));
  await screen.findByRole('heading', { name: 'Review queue' });
  await go(-1);
  await screen.findByRole('heading', { name: 'Body a' });
});
