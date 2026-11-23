import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act, cleanup, render as rawRender, screen, waitFor, fireEvent } from '@testing-library/react';
import { message } from 'antd';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Routes, Route, Link } from 'react-router-dom';
import { setupServer } from 'msw/node';
import { http, HttpResponse } from 'msw';
import { clearUserEmail, InputActivityProvider, inputActivityService, setUserEmail } from '@colony2/shared';
const render = (ui: Parameters<typeof rawRender>[0]) => rawRender(ui, {wrapper: InputActivityProvider});
import InputDetailPage from './InputDetailPage';
import {jobIdentity} from '../test/cellFixtures';
import {
  inputApiHandlers,
  resetMockInputStore,
  setupMockInputs,
  createMockPendingInput,
  createMockSingleQuestionDetails,
  mockInputDetailsStore,
} from '../test/mocks/inputApiHandlers';
import { mockEventSource } from '../test/mocks/mockSSE';

const server = setupServer(...inputApiHandlers);

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('InputDetailPage - Single Question Flow', () => {
  const projectId = 'test-project-123';
  const jobId = 'job-abc-123';
  let eventSourceMock: ReturnType<typeof mockEventSource>;

  beforeEach(() => {
    global.ResizeObserver = vi.fn().mockImplementation(()=>({observe:vi.fn(),unobserve:vi.fn(),disconnect:vi.fn()}));
    vi.mocked(window.matchMedia).mockImplementation(query=>({matches:query.includes('min-width'),media:query,onchange:null,addListener:vi.fn(),removeListener:vi.fn(),addEventListener:vi.fn(),removeEventListener:vi.fn(),dispatchEvent:vi.fn()}));
    server.listen({ onUnhandledRequest: 'error' });
    resetMockInputStore();
    eventSourceMock = mockEventSource();
    mockNavigate.mockClear();
  });

  afterEach(async () => {
    // Static messages have a React root outside the rendered review page.
    // Drain its unmount effects before jsdom and the input mocks are removed.
    await act(async () => { cleanup(); message.destroy(); });
    inputActivityService.clearCaches(); // Clear caches between tests
    server.close();
    server.resetHandlers();
    eventSourceMock.restore();
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  const renderComponent = (query = '') => {
    return render(
      <MemoryRouter initialEntries={[`/project/${projectId}/reviews/${jobId}${query}`]}>
        <Routes>
          <Route
            path="/project/:projectId/reviews/:jobId"
            element={
              <InputActivityProvider>
                <InputDetailPage projectId={projectId} />
              </InputActivityProvider>
            }
          />
        </Routes>
      </MemoryRouter>
    );
  };

  const setupReview = () => {
    const details = createMockSingleQuestionDetails(jobId);
    details.form = { kind: 'review', request_id: 'request-1', documents: {}, fields: [
      { id: 'decision', type: 'multiple_choice', question: 'Decision', required: true, options: [{ value: 'approve', label: 'Approve' }, { value: 'revise', label: 'Revise' }] },
      { id: 'feedback', type: 'paragraph_text', question: 'Reason' },
      { id: 'attachment', type: 'file_upload', question: 'Annotations' },
    ] };
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [details]);
    server.use(http.get('http://localhost:8080/api/projects/:tenant/jobs/:job', () => HttpResponse.json({...jobIdentity,job_id:jobId,status:'ACTIVE'})),
      http.get('http://localhost:8080/api/projects/:tenant/jobs/:job/children', () => HttpResponse.json({jobs:[]})));
    return details;
  };

  it.each(['back', 'cancel', 'submit', 'review', 'cancel-job', 'error'])('preserves only Reviews URL fields on %s', async action => {
    if (action === 'review' || action === 'cancel-job') {
      setupReview();
      server.use(http.post('http://localhost:8080/api/projects/:tenant/reviews/:job/respond', () => HttpResponse.json({})),
        http.post('http://localhost:8080/api/projects/:tenant/jobs/:job/cancel', () => HttpResponse.json({})));
    }
    else setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    if (action === 'error') server.use(http.get('http://localhost:8080/api/projects/:tenant/user-inputs/:job', () => HttpResponse.error()));
    const query = '?q=foo+bar&cellId=absent&sort=cell&order=descend&returnTo=https://bad&status=READY';
    renderComponent(query);
    if (action === 'error') await screen.findByRole('alert');
    else await screen.findByRole('button', { name: /Back to Reviews/ });
    if (action === 'submit') {
      fireEvent.change(screen.getByRole('textbox', { name: 'How old are you?' }), { target: { value: '30' } });
      fireEvent.click(screen.getByRole('button', { name: /submit/i }));
    } else if (action === 'review') {
      fireEvent.click(await screen.findByRole('button', { name: 'Approve', exact: true }));
      fireEvent.click(screen.getByRole('button', { name: 'OK', exact: true }));
    } else if (action === 'cancel-job') {
      fireEvent.click(screen.getByRole('button', { name: 'Cancel job', exact: true }));
      fireEvent.click(screen.getByRole('dialog').querySelector('.ant-modal-footer button.ant-btn-primary')!);
    } else fireEvent.click(screen.getByRole('button', { name: action === 'cancel' ? /^Cancel$/ : /Back to Reviews/ }));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/reviews?q=foo+bar&cellId=absent&sort=cell&order=descend`));
  });

  it('REV-011/012/013 retains drafts and attachments after failure and prevents overlapping submissions', async () => {
    setupReview();
    let fail!: (error: Error) => void;
    const submit = vi.spyOn(inputActivityService, 'submitReview').mockImplementationOnce(() => new Promise((_, reject) => { fail = reject; })).mockResolvedValueOnce(undefined as never);
    const {container} = renderComponent();
    const reject = await screen.findByRole('button',{name:'Reject',exact:true});
    await userEvent.click(reject);
    await userEvent.type(screen.getByLabelText('Reason'),'Keep this draft');
    const file = new File(['notes'],'notes.md',{type:'text/markdown'});
    await userEvent.upload(container.querySelector('input[type=file]')!,file);
    const send = screen.getByRole('button',{name:'OK'});
    fireEvent.click(send); fireEvent.click(send);
    await waitFor(()=>expect(submit).toHaveBeenCalledTimes(1));
    expect(screen.getByRole('button',{name:'Cancel job',exact:true})).toBeDisabled();
    fail(new Error('Stale request: reload to continue'));
    await screen.findByText('Stale request: reload to continue');
    expect(screen.getByLabelText('Reason')).toHaveValue('Keep this draft');
    expect(screen.getByText('notes.md')).toBeInTheDocument();
    expect(container.querySelector('.review-actions')).toHaveTextContent('Stale request: reload to continue');
    await userEvent.click(send);
    await waitFor(()=>expect(submit).toHaveBeenCalledTimes(2));
    expect(submit).toHaveBeenLastCalledWith(projectId,jobId,'request-1',expect.objectContaining({fields:{decision:'revise',feedback:'Keep this draft'},attachments:{attachment:file}}));
    submit.mockRestore();
  });

  it('RM-004/009/010 retains one mounted feedback and upload field through navigation toggles', async () => {
    setupReview();
    const {container} = renderComponent();
    await userEvent.click(await screen.findByRole('button',{name:'Reject',exact:true}));
    await userEvent.type(screen.getByLabelText('Reason'),'Draft survives navigation');
    const file = new File(['notes'],'notes.md',{type:'text/markdown'});
    await userEvent.upload(container.querySelector('input[type=file]')!,file);
    const upload = container.querySelector('input[type=file]')!;
    expect(upload.closest('.review-actions')).not.toBeNull();
    expect(screen.queryByText('Why?')).toBeNull();
    await userEvent.click(screen.getByRole('button',{name:'Show documents'}));
    await userEvent.click(screen.getByRole('button',{name:'Hide documents'}));
    expect(screen.getByLabelText('Reason')).toHaveValue('Draft survives navigation');
    expect(container.querySelector('input[type=file]')).toBe(upload);
    expect(screen.getByText('notes.md')).toBeVisible();
    expect(screen.getByRole('button',{name:'OK'})).toBeVisible();
  });

  it('REV-013 disables review submission during pending cancellation', async () => {
    setupReview();
    let finish!: () => void;
    server.use(http.post('http://localhost:8080/api/projects/:tenant/jobs/:job/cancel', async () => {
      await new Promise<void>(resolve=>{finish=resolve;}); return HttpResponse.json({});
    }));
    renderComponent();
    await userEvent.click(await screen.findByRole('button',{name:'Cancel job',exact:true}));
    const dialog = screen.getByRole('dialog');
    fireEvent.click(dialog.querySelector('.ant-modal-footer button.ant-btn-primary')!);
    await waitFor(()=>expect(screen.getByRole('button',{name:'Approve',exact:true})).toBeDisabled());
    expect(screen.getByRole('button',{name:'Reject',exact:true})).toBeDisabled();
    await waitFor(()=>expect(finish).toBeTypeOf('function'));
    finish();
    await waitFor(()=>expect(mockNavigate).toHaveBeenCalled());
  });

  it('REV-014 resets request-owned draft when the tenant changes', async () => {
    const details=setupReview();
    setupMockInputs('other-tenant',[createMockPendingInput(jobId)],[details]);
    const view=(tenant:string)=><MemoryRouter initialEntries={[`/reviews/${jobId}`]}><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId={tenant}/>}/></Routes></MemoryRouter>;
    const {rerender}=render(view(projectId));
    await userEvent.click(await screen.findByRole('button',{name:'Reject',exact:true}));
    await userEvent.type(screen.getByLabelText('Reason'),'Tenant owned draft');
    rerender(view('other-tenant'));
    await userEvent.click(await screen.findByRole('button',{name:'Reject',exact:true}));
    expect(screen.getByLabelText('Reason')).toHaveValue('');
  });

  it.each(['job', 'request'])('REV-014 resets drafts when %s identity changes', async identity => {
    const details=setupReview();
    const getDetails=vi.spyOn(inputActivityService,'getInputDetails').mockResolvedValue(details);
    const view=()=> <MemoryRouter initialEntries={[`/reviews/${jobId}`]}><Link to="/reviews/next-job">Next request</Link><Routes><Route path="/reviews/:jobId" element={<InputDetailPage projectId={projectId}/>}/></Routes></MemoryRouter>;
    const {rerender}=render(view());
    await userEvent.click(await screen.findByRole('button',{name:'Reject',exact:true}));
    await userEvent.type(screen.getByLabelText('Reason'),'Earlier request draft');
    if(identity==='job') {setupMockInputs(projectId,[createMockPendingInput('next-job')],[{...details,jobId:'next-job'}]); await act(async()=>{await inputActivityService.getPendingInputs(projectId,true);}); await userEvent.click(screen.getByRole('link',{name:'Next request'}));}
    else { details.form.request_id='request-2'; await act(async()=>{(inputActivityService as any).publish(projectId,[{...createMockPendingInput(jobId),client_payload_revision:2}]);}); rerender(view()); }
    await userEvent.click(await screen.findByRole('button',{name:'Reject',exact:true}));
    expect(screen.getByLabelText('Reason')).toHaveValue('');
    getDetails.mockRestore();
  });

  it('retains a draft for the same short-answer request and isolates the next request without an ID', async () => {
    setUserEmail('draft@example.com');
    try {
      const pending = { ...createMockPendingInput(jobId), task_ordinal: 12 };
      setupMockInputs(projectId, [pending], [createMockSingleQuestionDetails(jobId)]);
      const firstVisit = renderComponent();
      const firstAnswer = await screen.findByRole('textbox', { name: 'How old are you?' });
      await userEvent.type(firstAnswer, 'Earlier draft');
      firstVisit.unmount();

      renderComponent();
      expect(await screen.findByRole('textbox', { name: 'How old are you?' })).toHaveValue('Earlier draft');

      setupMockInputs(projectId, [{ ...pending, task_ordinal: 13, client_payload_revision: 2 }], [{
        ...createMockSingleQuestionDetails(jobId), task_ordinal: 13,
        form: { question: 'Who should review the release?', type: 'short_answer' },
      }]);
      await act(async () => { await inputActivityService.getPendingInputs(projectId, true); });
      expect(await screen.findByRole('textbox', { name: 'Who should review the release?' })).toHaveValue('');
    } finally {
      clearUserEmail();
    }
  });

  it('should show loading state initially', () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    expect(screen.getByText('Checking pending requests…')).toBeInTheDocument();
  });

  it.each(['pending', 'failed'])('keeps the review and its draft mounted while job metadata is %s', async state => {
    setupReview();
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    let failed = state === 'failed';
    server.use(http.get(`http://localhost:8080/api/projects/${projectId}/jobs/${jobId}`, async () => {
      if (state === 'pending') await gate;
      return failed ? HttpResponse.json({error:'Metadata unavailable'}, {status:503})
        : HttpResponse.json({...jobIdentity, job_id:jobId, status:'ACTIVE', prompt:'Metadata recovered'});
    }));
    const {container} = renderComponent();
    try {
      await userEvent.click(await screen.findByRole('button', {name:'Reject', exact:true}));
      const feedback = screen.getByLabelText('Reason');
      await userEvent.type(feedback, 'Keep this draft while context recovers');
      await userEvent.upload(container.querySelector('input[type=file]')!, new File(['notes'], 'metadata-notes.md', {type:'text/markdown'}));
      const upload = container.querySelector('input[type=file]')!;
      if (state === 'failed') {
        await screen.findByText('Job context unavailable');
        failed = false;
        await userEvent.click(screen.getByRole('button', {name:'Retry job context'}));
      } else {
        expect(screen.getByText('Loading job context...')).toBeVisible();
        release();
      }
      await screen.findByText('Metadata recovered');
      expect(screen.getByLabelText('Reason')).toBe(feedback);
      expect(feedback).toHaveValue('Keep this draft while context recovers');
      expect(container.querySelector('input[type=file]')).toBe(upload);
      expect(screen.getByText('metadata-notes.md')).toBeVisible();
      expect(screen.getByRole('button', {name:'OK'})).toBeEnabled();
    } finally { release(); }
  });

  it('should display input details for single question', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText('Input Request')).toBeInTheDocument();
    },
      { timeout: 3000 }
    );

    expect(screen.getByText(`Job ID: ${jobId}`)).toBeInTheDocument();
    expect(screen.getByText(/Status:/)).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
  });

  it('should render form with single question', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getAllByText('How old are you?').length).toBeGreaterThan(0);
    },
      { timeout: 3000 }
    );

    // Form should have input field
    const input = screen.getByRole('textbox');
    expect(input).toBeInTheDocument();
  });

  it('should submit response successfully', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getAllByText('How old are you?').length).toBeGreaterThan(0);
    },
      { timeout: 3000 }
    );

    // Fill out the form
    const input = screen.getByRole('textbox');
    await user.type(input, '25');

    // Submit form
    const submitButton = screen.getByRole('button', { name: /submit/i });
    await user.click(submitButton);

    // Should navigate to job details
    await waitFor(
      () => {
      expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/reviews`);
    });

    // Verify input was removed from store
    const key = `${projectId}:${jobId}`;
    expect(mockInputDetailsStore.has(key)).toBe(false);
  });

  it('should show back button', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByRole('button', { name: /back to reviews/i },
      { timeout: 3000 }
    )).toBeInTheDocument();
    });
  });

  it('should navigate back when back button clicked', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      const backButton = screen.getByRole('button', { name: /back to reviews/i },
      { timeout: 3000 }
    );
      expect(backButton).toBeInTheDocument();
    });

    const backButton = screen.getByRole('button', { name: /back to reviews/i });
    await user.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/reviews`);
  });

  it('should show cancel button in form', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByRole('button', { name: /cancel/i },
      { timeout: 3000 }
    )).toBeInTheDocument();
    });
  });

  it('should navigate back when cancel button clicked', async () => {
    const user = userEvent.setup();
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      const cancelButton = screen.getByRole('button', { name: /cancel/i },
      { timeout: 3000 }
    );
      expect(cancelButton).toBeInTheDocument();
    });

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    await user.click(cancelButton);

    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/reviews`);
  });

  it('shows confirmed absence without probing details', async () => {
    setupMockInputs(projectId, [], []); // No inputs
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText(/No outstanding input request/i)).toBeInTheDocument();
    },
      { timeout: 3000 }
    );
  });

  it('keeps an exact-job failure visible when no request details establish a reader', async () => {
    setupMockInputs(projectId, [], []);
    server.use(http.get(`http://localhost:8080/api/projects/${projectId}/jobs/${jobId}`, () => HttpResponse.json({error:'Job not found'}, {status:404})));
    renderComponent();
    expect(await screen.findByText('Could not load job')).toBeInTheDocument();
    expect(screen.queryByText('No outstanding input request.')).not.toBeInTheDocument();
  });

  it('retains the draft and reports an operational detail error after retry', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();
    const answer = await screen.findByRole('textbox', { name: 'How old are you?' });
    fireEvent.change(answer, { target: { value: '30' } });
    server.use(http.get(`http://localhost:8080/api/projects/${projectId}/user-inputs/${jobId}`, () => HttpResponse.json({ error: 'storage failed' }, { status: 500 })));
    await act(async () => { await inputActivityService.getInputDetails(projectId, jobId, true).catch(() => {}); });
    expect(await screen.findByText('Could not load this request')).toBeInTheDocument();
    expect(screen.queryByText('No outstanding input request.')).not.toBeInTheDocument();
    expect(answer).toHaveValue('30');
    expect(screen.getByRole('button', { name: /submit/i })).toBeDisabled();
  });

  it('should display started time', async () => {
    setupMockInputs(projectId, [createMockPendingInput(jobId)], [createMockSingleQuestionDetails(jobId)]);
    renderComponent();

    await waitFor(
      () => {
      expect(screen.getByText(/Started:/)).toBeInTheDocument();
    },
      { timeout: 3000 }
    );
  });
});
