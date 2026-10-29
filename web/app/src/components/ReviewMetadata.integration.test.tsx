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
const view = (projectId = 'tenant') => <MemoryRouter><InputActivityProvider><PendingInputsListPage projectId={projectId} /></InputActivityProvider></MemoryRouter>;

it('refreshes compact labels without job detail requests', async () => {
  const prompt = 'Build ' + '🌱'.repeat(80);
  setupMockInputs('tenant', [{ id: 'a', prompt, cell_name: 'platform', client_payload_revision: 1 }], []);
  const reads = vi.fn();
  server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', () => { reads(); return HttpResponse.error(); }));
  render(view()); (await waitForSSEConnection()).emitConnected();
  expect(await screen.findByText('Cell: platform')).toBeVisible();
  expect(screen.getByText(abbreviatePrompt(prompt))).toBeVisible();
  setupMockInputs('tenant', [{ id: 'a', title: 'Updated title', cell_name: 'other', client_payload_revision: 2 }], []);
  fireEvent.click(screen.getByRole('button', { name: /Refresh/ }));
  expect(await screen.findByText('Updated title')).toBeVisible();
  expect(screen.getByText('Cell: other')).toBeVisible();
  expect(reads).not.toHaveBeenCalled();
});

it('applies streamed summaries without per-entry requests and isolates tenants', async () => {
  setupMockInputs('first', [], []); setupMockInputs('second', [], []);
  const { rerender } = render(view('first'));
  (await waitForSSEConnection()).emitConnected();
  const fetch = vi.spyOn(globalThis, 'fetch');
  inputActivityService.emit('inputs_changed', { projectId: 'first', inputs: Array.from({ length: 200 }, (_, i) => ({ id: String(i), title: `Request ${i}` })) });
  expect(await screen.findByText('Request 199')).toBeVisible();
  expect(fetch.mock.calls.filter(([url]) => String(url).includes('/jobs/'))).toHaveLength(0);
  rerender(view('second'));
  inputActivityService.emit('inputs_changed', { projectId: 'first', inputs: [{ id: 'old', title: 'Stale tenant' }] });
  expect(screen.queryByText('Request 199')).toBeNull();
  expect(screen.queryByText('Stale tenant')).toBeNull();
  fetch.mockRestore();
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
  render(<MemoryRouter initialEntries={['/reviews/a']}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId="tenant" />} /></Routes></MemoryRouter>);
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
