import { jobIdentity } from '../test/cellFixtures';
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
beforeEach(() => { server.listen({ onUnhandledRequest: 'error' }); resetMockInputStore(); events = mockEventSource(); server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job/children', () => HttpResponse.json({jobs:[]}))); });
afterEach(() => { inputActivityService.disconnect(); inputActivityService.clearCaches(); server.resetHandlers(); server.close(); events.restore(); vi.clearAllMocks(); });
const view = (projectId = 'tenant') => <MemoryRouter><InputActivityProvider><PendingInputsListPage projectId={projectId} /></InputActivityProvider></MemoryRouter>;

it('refreshes compact labels without job detail requests', async () => {
  const prompt = 'Build ' + '🌱'.repeat(80);
  setupMockInputs('tenant', [{ ...jobIdentity, id: 'a', prompt, cell_name: 'platform', client_payload_revision: 1 }], []);
  const reads = vi.fn();
  server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', () => { reads(); return HttpResponse.error(); }));
  render(view()); (await waitForSSEConnection()).emitConnected();
  expect(await screen.findByText('Cell: platform')).toBeVisible();
  expect(screen.getByText(abbreviatePrompt(prompt))).toBeVisible();
  setupMockInputs('tenant', [{ ...jobIdentity, id: 'a', title: 'Updated title', cell_name: 'other', client_payload_revision: 2 }], []);
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
  inputActivityService.emit('inputs_changed', { projectId: 'first', inputs: Array.from({ length: 200 }, (_, i) => ({ ...jobIdentity, id: String(i), title: `Request ${i}` })) });
  expect(await screen.findByText('Request 199')).toBeVisible();
  expect(fetch.mock.calls.filter(([url]) => String(url).includes('/jobs/'))).toHaveLength(0);
  rerender(view('second'));
  inputActivityService.emit('inputs_changed', { projectId: 'first', inputs: [{ ...jobIdentity, id: 'old', title: 'Stale tenant' }] });
  expect(screen.queryByText('Request 199')).toBeNull();
  expect(screen.queryByText('Stale tenant')).toBeNull();
  fetch.mockRestore();
});

it.each([404, 503, 'network'] as const)('preserves review draft and submission through metadata failure %s and retry', async failure => {
  setupMockInputs('tenant', [], [{ jobId: 'a', status: 'pending', startTime: '2026-10-25T10:00:00Z', form: {
    kind: 'review', title: 'Review phase', request_id: 'original-request', fields: [
      { ...jobIdentity, id: 'decision', type: 'multiple_choice', question: 'Response question', required: true, options: [{ value: 'approve', label: 'Approve' }] },
      { ...jobIdentity, id: 'feedback', type: 'paragraph_text', question: 'Feedback', required: true },
    ],
  } }]);
  let reads = 0;
  let received = false;
  const submit = vi.spyOn(inputActivityService, 'submitReview');
  server.use(
    http.get('http://localhost:8080/api/projects/tenant/jobs/a', () => {
      if (++reads === 1) return failure === 'network' ? HttpResponse.error() : new HttpResponse(null, { status: failure });
      return HttpResponse.json({ ...jobIdentity, status: 'READY', store: 'ACTIVE', cell_name: 'platform', prompt: 'Complete goal prompt' });
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
  await user.click(screen.getByRole('button', { name: 'Approve', exact: true }));
  await user.type(screen.getByLabelText('Feedback'), 'Keep this draft');
  await user.click(screen.getByRole('button', { name: 'Retry job context' }));
  await screen.findByText('Complete goal prompt');
  expect(screen.getByLabelText('Feedback')).toHaveValue('Keep this draft');
  expect(screen.getByText('Input status:')).toBeVisible();
  expect(screen.getByText('Input started:')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'OK', exact: true }));
  await waitFor(() => expect(received).toBe(true));
  expect(submit).toHaveBeenCalledWith('tenant', 'a', 'original-request', expect.objectContaining({ fields: { decision: 'approve', feedback: 'Keep this draft' } }));
  submit.mockRestore();
});

it('REL-001,030,031 preserves parent context and review draft through child failure with one job inspection', async () => {
  setupMockInputs('tenant', [], [{jobId:'a',status:'pending',startTime:'2026-11-07T00:00:00Z',form:{kind:'review',title:'Review evidence',request_id:'relationship-review',fields:[
    {id:'decision',type:'multiple_choice',question:'Approve evidence',required:true,options:[{value:'approve',label:'Approve'}]},
    {id:'feedback',type:'paragraph_text',question:'Feedback',required:false},
  ]}}]);
  let reads=0;
  server.use(
    http.get('http://localhost:8080/api/projects/tenant/jobs/a',()=>{reads++;return HttpResponse.json({...jobIdentity,job_id:'a',tenant_id:'tenant',status:'READY',store:'ACTIVE',parent:{job_id:'parent',goal_id:'parent-goal'}});}),
    http.get('http://localhost:8080/api/projects/tenant/jobs/a/children',()=>new HttpResponse(null,{status:503})),
  );
  render(<MemoryRouter initialEntries={['/reviews/a']}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId="tenant" />} /></Routes></MemoryRouter>);
  const heading = await screen.findByRole('heading', { name: 'Related jobs and goals' });
  expect(heading).toBeVisible();
  expect(heading.closest('details')).toBeNull();
  await screen.findByText('Child jobs unavailable');
  expect(screen.getByRole('link',{name:'Job · parent'})).toHaveAttribute('href','/project/tenant/jobs/parent/story');
  expect(screen.getByRole('link',{name:'Goal · parent-goal'})).toHaveAttribute('href','/project/tenant/goals/parent-goal');
  await userEvent.click(screen.getByRole('button',{name:'Approve',exact:true}));
  await userEvent.type(screen.getByLabelText('Feedback'),'Retained draft');
  fireEvent.click(screen.getByRole('button',{name:'Retry',exact:true}));
  await screen.findByText('Child jobs unavailable');
  expect(reads).toBe(1);
  expect(screen.getByLabelText('Feedback')).toHaveValue('Retained draft');
  expect(screen.getByRole('button',{name:'OK',exact:true})).toBeEnabled();
});

it('PR-009,015 orders load, refresh and live replacements and preserves unavailable, empty and disconnected states', async () => {
  const row=(id:string,requested_at?:string)=>({...jobIdentity,id,title:`Queue ${id}`,requested_at});
  setupMockInputs('tenant',[row('new','2026-11-10T00:00:00Z'),row('old','2026-11-01T00:00:00Z'),row('unknown')], []);
  const {container}=render(view());
  const source=await waitForSSEConnection(); source.emitConnected();
  await screen.findByText('Queue old');
  const titles=()=>Array.from(container.querySelectorAll('.ant-card strong')).map(el=>el.textContent);
  expect(titles()).toEqual(['Queue old','Queue new','Queue unknown']);
  setupMockInputs('tenant',[row('new','2026-11-10T00:00:00Z'),row('old','2026-11-01T00:00:00Z')], []);
  fireEvent.click(screen.getByRole('button',{name:/Refresh/}));
  await waitFor(()=>expect(titles()).toEqual(['Queue old','Queue new']));
  source.dispatchEvent(new MessageEvent('inputs_changed',{data:JSON.stringify({inputs:[row('future','2030-01-01T00:00:00Z'),row('earliest','2026-01-01T00:00:00Z'),row('unknown')]})}));
  await waitFor(()=>expect(titles()).toEqual(['Queue earliest','Queue future','Queue unknown']));
  expect(screen.getByText('Request time unavailable')).toBeVisible();
  inputActivityService.emit('connection_failed');
  expect(await screen.findByText('Live updates unavailable. Refresh to check for requests.')).toBeVisible();
  expect(titles()).toEqual(['Queue earliest','Queue future','Queue unknown']);
  source.dispatchEvent(new MessageEvent('inputs_changed',{data:JSON.stringify({inputs:[]})}));
  expect(await screen.findByText('No pending reviews')).toBeVisible();
});

it('PR-015 keeps load errors distinct from empty state and retries the queue', async () => {
  server.use(http.get('http://localhost:8080/api/projects/tenant/user-inputs/pending',()=>new HttpResponse(null,{status:503})));
  render(view()); (await waitForSSEConnection()).emitConnected();
  expect(await screen.findByText('Could not load reviews')).toBeVisible();
  expect(screen.queryByText('No pending reviews')).toBeNull();
  setupMockInputs('tenant',[{...jobIdentity,id:'a',title:'Recovered review'}], []);
  server.use(http.get('http://localhost:8080/api/projects/tenant/user-inputs/pending',()=>HttpResponse.json([{...jobIdentity,id:'a',title:'Recovered review'}])));
  fireEvent.click(screen.getByRole('button',{name:'Retry reviews'}));
  expect(await screen.findByText('Recovered review')).toBeVisible();
  expect(screen.getByText('Request time unavailable')).toBeVisible();
});
