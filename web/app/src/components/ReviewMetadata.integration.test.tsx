import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import InputDetailPage from './InputDetailPage';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { InputActivityProvider, inputActivityService } from '@colony2/shared';
import PendingInputsListPage from './PendingInputsListPage';
import { inputApiHandlers, setupMockInputs, resetMockInputStore } from '../test/mocks/inputApiHandlers';
import { mockEventSource, waitForSSEConnection } from '../test/mocks/mockSSE';
import { abbreviatePrompt } from '../utils/goalPresentation';

const server = setupServer(...inputApiHandlers);
let events: ReturnType<typeof mockEventSource>;
beforeEach(() => { server.listen({ onUnhandledRequest: 'error' }); resetMockInputStore(); events = mockEventSource(); });
afterEach(() => { inputActivityService.disconnect(); inputActivityService.clearCaches(); server.resetHandlers(); server.close(); events.restore(); vi.clearAllMocks(); });
const view = (projectId = 'tenant', reviews = true) => <MemoryRouter><InputActivityProvider><PendingInputsListPage projectId={projectId} reviews={reviews} /></InputActivityProvider></MemoryRouter>;

it('uses the shared job GET, keeps headings, and refreshes metadata while preserving failed cards', async () => {
  setupMockInputs('tenant', [{ id: 'a', kind: 'review', title: 'Review phase', document_count: 2 }], []);
  const prompt = '  Build\n' + '🌱'.repeat(80);
  let reads = 0;
  server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', () => {
    reads++; return reads === 1 ? HttpResponse.json({ cell_name: 'platform', prompt }) : new HttpResponse(null, { status: 503 });
  }));
  render(view()); (await waitForSSEConnection()).emitConnected();
  await screen.findByText('Cell: platform');
  expect(screen.getByText(`Goal: ${abbreviatePrompt(prompt)}`)).toBeVisible();
  expect(screen.getByText('Review phase')).toBeVisible();
  expect(screen.getByText('Job ID: a · 2 documents')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
  await screen.findByText('Cell: Cell unavailable');
  expect(screen.getByText('Goal: Review phase')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Open Review' })).toBeEnabled();
  expect(reads).toBe(2);
});

it.each([undefined, '', ' \n '])('uses title fallback and unknown cell for successful missing metadata %j', async cell_name => {
  setupMockInputs('tenant', [{ id: 'a', kind: 'review', title: '   Review\n phase ' }, { id: 'b', kind: 'review' }], []);
  server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', () => HttpResponse.json({ cell_name, prompt: '  \n' })));
  render(view()); (await waitForSSEConnection()).emitConnected();
  await waitFor(() => expect(screen.getAllByText('Cell: Unknown cell')).toHaveLength(2));
  expect(screen.getByText('Goal: Review phase')).toBeVisible();
  expect(screen.getByText('Goal: Untitled goal')).toBeVisible();
});

it('reconciles SSE additions and removals without refetching unchanged IDs', async () => {
  setupMockInputs('tenant', [{ id: 'a', kind: 'review' }], []);
  const ids: string[] = [];
  server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', ({ params }) => { ids.push(String(params.job)); return HttpResponse.json({ cell_name: params.job }); }));
  render(view()); const sse = await waitForSSEConnection(); sse.emitConnected();
  await screen.findByText('Cell: a');
  setupMockInputs('tenant', [{ id: 'a', kind: 'review' }, { id: 'b', kind: 'review' }], []);
  sse.emitInputPending('b');
  await screen.findByText('Cell: b');
  expect(ids).toEqual(['a', 'b']);
  setupMockInputs('tenant', [{ id: 'b', kind: 'review' }], []);
  sse.emitInputCancelled('a');
  await waitFor(() => expect(screen.queryByText('Cell: a')).toBeNull());
});

it('does not schedule stale activity under a new tenant even before its connection settles', async () => {
  setupMockInputs('first', [{ id: 'old-id', kind: 'review' }], []);
  setupMockInputs('second', [{ id: 'new-id', kind: 'review' }], []);
  const reads: string[] = [];
  let release!: () => void;
  const gate = new Promise<void>(done => { release = done; });
  server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', async ({ params }) => {
    reads.push(`${params.tenant}/${params.job}`);
    if (params.tenant === 'first') await gate;
    return HttpResponse.json({ cell_name: params.tenant });
  }));
  const { rerender } = render(view('first')); (await waitForSSEConnection()).emitConnected();
  await waitFor(() => expect(reads).toContain('first/old-id'));
  rerender(view('second'));
  const sse = await waitForSSEConnection(); sse.emitConnected();
  await screen.findByText('Cell: second'); release();
  await waitFor(() => expect(reads).toEqual(['first/old-id', 'second/new-id']));
  expect(screen.queryByText('Cell: first')).toBeNull();
});

it('never fetches metadata for ordinary inputs', async () => {
  setupMockInputs('tenant', [{ id: 'ordinary' }], []);
  render(view('tenant', false)); (await waitForSSEConnection()).emitConnected();
  await screen.findByText('Input Request');
  // Strict MSW handling makes any unexpected job GET fail this test.
  expect(screen.queryByText(/Cell:/)).toBeNull();
});

it.each([404, 503, 'network'] as const)('preserves review draft and submission through metadata failure %s and retry', async failure => {
  setupMockInputs('tenant', [], [{ jobId: 'a', status: 'pending', startTime: '2026-10-25T10:00:00Z', form: {
    kind: 'review', title: 'Review phase', request_id: 'original-request', fields: [
      { id: 'decision', type: 'multiple_choice', question: 'Response question', required: true, options: [{ value: 'approve', label: 'Approve' }] },
      { id: 'feedback', type: 'paragraph_text', question: 'Feedback', required: true },
    ],
  } }]);
  let reads = 0;
  let received = false;
  const submit = vi.spyOn(inputActivityService, 'submitReview');
  server.use(
    http.get('http://localhost:8080/api/projects/tenant/jobs/a', () => {
      if (++reads === 1) return failure === 'network' ? HttpResponse.error() : new HttpResponse(null, { status: failure });
      return HttpResponse.json({ status: 'READY', store: 'ACTIVE', cell_name: 'platform', prompt: 'Complete goal prompt' });
    }),
    http.post('http://localhost:8080/api/projects/tenant/reviews/a/respond', async ({ request }) => {
      await request.text(); received = true; return HttpResponse.json({ ok: true });
    }),
  );
  render(<MemoryRouter initialEntries={['/reviews/a']}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId="tenant" reviews />} /></Routes></MemoryRouter>);
  await screen.findByText('Job context unavailable');
  expect(screen.getByText('No documents attached')).toBeVisible();
  expect(screen.getByRole('region', { name: 'Review context' })).toHaveTextContent('Response question');
  const user = userEvent.setup();
  await user.type(screen.getByLabelText('Feedback'), 'Keep this draft');
  await user.click(screen.getByRole('button', { name: 'Retry job context' }));
  await screen.findByText('Complete goal prompt');
  expect(screen.getByLabelText('Feedback')).toHaveValue('Keep this draft');
  expect(screen.getByText('Input status:')).toBeVisible();
  expect(screen.getByText('Input started:')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Approve', exact: true }));
  await waitFor(() => expect(received).toBe(true));
  expect(submit).toHaveBeenCalledWith('tenant', 'a', 'original-request', expect.objectContaining({ fields: { decision: 'approve', feedback: 'Keep this draft' } }));
  submit.mockRestore();
});
