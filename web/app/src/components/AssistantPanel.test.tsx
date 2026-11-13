import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  assistantCapabilities, getAssistantSession, startAssistantSession, respondAssistantSession, listAssistantSessions,
  closeAssistantSession, reopenAssistantSession, AssistantAPIError, type AssistantSession,
} from '@colony2/shared';
import AssistantPanel from './AssistantPanel';
import { useAssistantPoll } from '../hooks/useAssistantPoll';
vi.mock('@colony2/shared', async original => ({ ...await original<object>(),
  assistantCapabilities: vi.fn(), getAssistantSession: vi.fn(), startAssistantSession: vi.fn(),
  respondAssistantSession: vi.fn(), closeAssistantSession: vi.fn(), reopenAssistantSession: vi.fn(), listAssistantSessions: vi.fn(), replayAssistantSession: vi.fn(),
}));
function session(overrides: Partial<AssistantSession> = {}): AssistantSession {
  return { id: 'session-1', job_id: 'execution-1', status: 'running', source: { kind: 'global' },
    current_request_id: 'request-1', replay: { entries: [{ key: 'task-1', role: 'assistant', status: 'succeeded', text: 'Durable answer' }], watermark: 'w1', has_more: false, complete: true }, ...overrides };
}
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (failure: unknown) => void; const promise = new Promise<T>((a,b) => { resolve=a; reject=b; }); return { promise, resolve, reject }; }
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(assistantCapabilities).mockResolvedValue({ available: true });
  vi.mocked(startAssistantSession).mockResolvedValue(session());
  vi.mocked(getAssistantSession).mockResolvedValue(session());
  vi.mocked(listAssistantSessions).mockResolvedValue({ sessions: [] });
});
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });
async function open() {
  fireEvent.click(screen.getByRole('button', { name: 'Open assistant' }));
  await waitFor(() => expect(screen.getByLabelText('Message')).not.toBeDisabled());
}
async function send(prompt = 'Discuss this') {
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: prompt } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await screen.findByText('Durable answer');
}

describe('assistant panel', () => {
 it('CTX-REV-001 reads current capabilities before a new Start and admits only one pending attempt', async () => {
  vi.mocked(assistantCapabilities).mockResolvedValue({ available: true, generation: 'mounted' });
  const capabilities = deferred<{ available: boolean; generation: string }>();
  render(<AssistantPanel tenantId="a" pageSource={{ kind: 'story', job_id: 'story-1' }} />);
  await open();
  vi.mocked(assistantCapabilities).mockReturnValueOnce(capabilities.promise);
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Start after replacement' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  fireEvent.submit(screen.getByLabelText('Message').closest('form')!);
  expect(assistantCapabilities).toHaveBeenCalledTimes(2);
  expect(startAssistantSession).not.toHaveBeenCalled();
  await act(async () => capabilities.resolve({ available: true, generation: 'replacement' }));
  await screen.findByText('Durable answer');
  expect(startAssistantSession).toHaveBeenCalledTimes(1);
  expect(vi.mocked(startAssistantSession).mock.calls[0][1]).toMatchObject({
   generation: 'replacement', prompt: 'Start after replacement', source: { kind: 'story', job_id: 'story-1' },
  });
 });
 it.each(['start', 'reopen'] as const)('CTX-REV-001 discards late %s capability results after a tenant change', async action => {
  const closed = session({ lifecycle: 'goal', goal_id: 'session-1', status: 'closed', current_request_id: undefined });
  vi.mocked(startAssistantSession).mockResolvedValue(closed);
  const view = render(<AssistantPanel tenantId="a" />);
  await open();
  if (action === 'reopen') await send();
  const calls = vi.mocked(startAssistantSession).mock.calls.length;
  const capabilities = deferred<{ available: boolean; generation: string }>();
  vi.mocked(assistantCapabilities).mockReturnValueOnce(capabilities.promise);
  if (action === 'start') fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Private tenant A draft' } });
  fireEvent.click(screen.getByRole('button', { name: action === 'start' ? 'Send message' : 'Reopen conversation' }));
  view.rerender(<AssistantPanel tenantId="b" />);
  await act(async () => capabilities.resolve({ available: true, generation: 'tenant-a-replacement' }));
  expect(startAssistantSession).toHaveBeenCalledTimes(calls);
  expect(reopenAssistantSession).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Message')).toHaveValue('');
 });
 it.each(['start', 'reopen'] as const)('CTX-REV-001 treats a failed %s capability read as an unattempted operation', async action => {
  vi.mocked(startAssistantSession).mockResolvedValue(session({ lifecycle: 'goal', goal_id: 'session-1', status: 'closed', current_request_id: undefined }));
  vi.mocked(reopenAssistantSession).mockResolvedValue(session({ lifecycle: 'goal', goal_id: 'session-1', job_id: 'execution-2' }));
  render(<AssistantPanel tenantId="a" />);
  await open();
  if (action === 'reopen') await send();
  const calls = vi.mocked(startAssistantSession).mock.calls.length;
  vi.mocked(assistantCapabilities).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ available: true, generation: 'current' });
  if (action === 'start') fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Unattempted draft' } });
  const name = action === 'start' ? 'Send message' : 'Reopen conversation';
  fireEvent.click(screen.getByRole('button', { name }));
  await screen.findByRole('alert');
  expect(startAssistantSession).toHaveBeenCalledTimes(calls);
  expect(reopenAssistantSession).not.toHaveBeenCalled();
  expect(screen.queryByRole('button', { name: /Retry same message|Retry reopen/ })).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole('button', { name })).toBeEnabled());
  fireEvent.click(screen.getByRole('button', { name }));
  await waitFor(() => expect(action === 'start' ? startAssistantSession : reopenAssistantSession).toHaveBeenCalledTimes(action === 'start' ? calls + 1 : 1));
 });
 it('CTX-014 and CTX-020 preserves the mounted draft on minimize and restores focus', async () => {
  render(<AssistantPanel tenantId="a" pageSource={{ kind: 'story', job_id: 'story-1' }} />);
  await open();
  const input = screen.getByLabelText('Message');
  expect(input).toHaveFocus();
  fireEvent.change(input, { target: { value: 'Unsent draft' } });
  fireEvent.keyDown(document, { key: 'Escape' });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Open assistant' })).toHaveFocus();
  await open();
  expect(screen.getByLabelText('Message')).toHaveValue('Unsent draft');
  const buttons = within(screen.getByRole('dialog')).getAllByRole('button').filter(button => !button.hasAttribute('disabled'));
  buttons[0].focus(); fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
  expect(buttons[buttons.length-1]).toHaveFocus();
 });
 it('CTX-012 freezes starts after an uncertain response without appending invented replies', async () => {
  vi.mocked(assistantCapabilities).mockResolvedValue({ available: true, generation: 'original' });
  vi.mocked(startAssistantSession).mockRejectedValueOnce(new Error('lost reply')).mockResolvedValueOnce(session());
  render(<AssistantPanel tenantId="a" pageSource={{ kind: 'review', job_id: 'review', request_id: 'exact' }} />);
  await open();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Read the review' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await screen.findByRole('alert');
  expect(screen.queryByText('Durable answer')).not.toBeInTheDocument();
  expect(screen.getByRole('checkbox')).toBeDisabled();
  const capabilityCalls = vi.mocked(assistantCapabilities).mock.calls.length;
  vi.mocked(assistantCapabilities).mockResolvedValue({ available: true, generation: 'replacement' });
  fireEvent.click(screen.getByRole('button', { name: 'Retry same message' }));
  await screen.findByText('Durable answer');
  const first = vi.mocked(startAssistantSession).mock.calls[0][1];
  expect(vi.mocked(startAssistantSession).mock.calls[1][1]).toBe(first);
  expect(first.source).toEqual({ kind: 'review', job_id: 'review', request_id: 'exact' });
  expect(first.generation).toBe('original');
  expect(assistantCapabilities).toHaveBeenCalledTimes(capabilityCalls);
 });
 it('CTX-015 discards late session results after switching tenants', async () => {
  const pending = deferred<AssistantSession>();
  vi.mocked(startAssistantSession).mockReturnValue(pending.promise);
  const view = render(<AssistantPanel tenantId="a" />);
  await open();
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Private tenant A draft' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  view.rerender(<AssistantPanel tenantId="b" />);
  await act(async () => pending.resolve(session({ replay: { entries: [{ key: 'secret', role: 'assistant', status: 'succeeded', text: 'Private tenant A answer' }], has_more: false, complete: true, watermark: 'private' } })));
  expect(screen.queryByText('Private tenant A answer')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Message')).toHaveValue('');
 });
 it('CTX-015 discards late capability results from the detached tenant', async () => {
  const old = deferred<{ available: boolean }>();
  vi.mocked(assistantCapabilities).mockReturnValueOnce(old.promise).mockResolvedValueOnce({ available: false });
  const view = render(<AssistantPanel tenantId="a" />);
  fireEvent.click(screen.getByRole('button', { name: 'Open assistant' }));
  view.rerender(<AssistantPanel tenantId="b" />);
  await screen.findByText('Assistant is unavailable. Your other pages remain available.');
  await act(async () => old.resolve({ available: true }));
  expect(screen.getByLabelText('Message')).toBeDisabled();
 });
 it('CTX-014 keeps the capture fixed across navigation and starts a new source explicitly', async () => {
  vi.mocked(startAssistantSession).mockResolvedValue(session({ source: { kind: 'story', job_id: 'story-1' } }));
  vi.mocked(getAssistantSession).mockResolvedValue(session({ source: { kind: 'story', job_id: 'story-1' } }));
  const view = render(<AssistantPanel tenantId="a" pageSource={{ kind: 'story', job_id: 'story-1' }} />);
  await open(); fireEvent.click(screen.getByRole('checkbox')); await send();
  view.rerender(<AssistantPanel tenantId="a" pageSource={{ kind: 'story', job_id: 'story-2' }} />);
  expect(screen.getByText('Story story-1')).toBeVisible();
  expect(screen.getByText(/Page changed/)).toBeVisible();
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'New conversation' }));
  expect(await screen.findByRole('checkbox')).not.toBeChecked();
  expect(startAssistantSession).toHaveBeenCalledTimes(1);
 });
 it('CTX-019 and CTX-025 keeps executable HTML, script links, image loads and arbitrary actions inert', async () => {
  const evil = session({ status: 'completed', current_request_id: undefined,
   replay: { entries: [{ key: 'evil', role: 'assistant', status: 'succeeded', text: 'Durable answer\n\n<script>window.pwned = 1</script>\n\n[run](javascript:alert%281%29) ![remote](https://evil.test/tracker)\n\n[unsafe source](/api/secret) [safe](https://example.test)', artifacts: [{ id: 'citation', name: '<img onerror=alert(1)>', mime: 'text/plain', size: 1, available: true }] }], watermark: 'evil', complete: true, has_more: false } });
  vi.mocked(startAssistantSession).mockResolvedValue(evil);
  const { container } = render(<AssistantPanel tenantId="a" />);
  await open(); await send();
  expect(container.querySelector('script')).toBeNull(); expect(container.querySelector('img')).toBeNull();
  expect(screen.getByText('run').closest('a')).toBeNull();
  expect(screen.getByText('unsafe source').closest('a')).toBeNull();
  expect(screen.getByRole('link', { name: 'safe' })).toHaveAttribute('href', 'https://example.test');
  expect(screen.getByRole('link', { name: '<img onerror=alert(1)>' })).toHaveAttribute('href', expect.stringContaining('/artifacts/citation'));
  expect(screen.queryByRole('button', { name: /approve|create goal|execute|edit file/i })).not.toBeInTheDocument();
 });
 it('CTX-003 clears retained content when refreshed access is denied', async () => {
  render(<AssistantPanel tenantId="a" />);
  await open(); await send();
  vi.mocked(getAssistantSession).mockRejectedValue(new AssistantAPIError(403, 'forbidden'));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh conversation' }));
  await screen.findByText('Assistant access is unavailable');
  expect(screen.queryByText('Durable answer')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Message')).toHaveValue('');
 });
 it('CTX-017 displays authoritative statuses without an estimate', async () => {
  vi.mocked(startAssistantSession).mockResolvedValue(session({ status: 'queued', current_request_id: undefined }));
  vi.mocked(getAssistantSession).mockResolvedValue(session({ status: 'queued', current_request_id: undefined }));
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  expect(screen.getByText('Queued')).toBeVisible();
  expect(screen.getByRole('link', { name: 'View execution' })).toHaveAttribute('href', '/project/a/jobs/execution-1/story');
  expect(screen.getByLabelText('Message')).toBeDisabled();
  expect(screen.queryByText(/seconds|minutes|ETA/)).not.toBeInTheDocument();
 });
 it('CTX-024 selects history without capturing the current page', async () => {
  vi.mocked(listAssistantSessions).mockResolvedValue({ sessions: [session()] });
  render(<AssistantPanel tenantId="a" pageSource={{ kind: 'story', job_id: 'current-page' }} />);
  await open(); fireEvent.click(screen.getByRole('button', { name: 'Conversation history' }));
  fireEvent.click(await screen.findByRole('button', { name: 'No page evidence · running' }));
  await screen.findByText('Durable answer');
  expect(startAssistantSession).not.toHaveBeenCalled();
  expect(screen.getByText('No page evidence', { selector: 'strong' })).toBeVisible();
 });
 it('CTX-009 requires reselection when the exact review was replaced', async () => {
  vi.mocked(startAssistantSession).mockRejectedValue(new AssistantAPIError(409, 'conflict'));
  render(<AssistantPanel tenantId="a" pageSource={{ kind: 'review', job_id: 'review', request_id: 'original-request' }} />);
  await open(); fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Discuss original evidence' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await screen.findByText('This review request changed. Reload the page and select its evidence again.');
  expect(screen.getByRole('button', { name: 'Reload review' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
  expect(startAssistantSession).toHaveBeenCalledTimes(1);
 });
 it('CTX-010 exposes each source outcome without claiming omitted bytes were read', async () => {
  const outcomes = ['included', 'truncated', 'unsupported', 'budget-omitted', 'denied', 'unavailable'];
  vi.mocked(startAssistantSession).mockResolvedValue(session({ status: 'completed', current_request_id: undefined,
    coverage: outcomes.map(outcome => ({ id: outcome, name: `${outcome}.txt`, outcome, producer_job_id: outcome === 'included' ? 'child-producer' : undefined })) }));
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  fireEvent.click(screen.getByText('Source coverage (6)'));
  for (const outcome of outcomes) expect(screen.getByText(`${outcome}.txt`)).toBeVisible();
  expect(screen.getByText('Producer: child-producer')).toBeVisible();
  expect(screen.queryByRole('link', { name: /^Download/ })).not.toBeInTheDocument();
 });
 it('CTX-015 ignores old polling responses after selecting another session', async () => {
  const old = deferred<AssistantSession>();
  vi.mocked(getAssistantSession).mockReturnValueOnce(old.promise).mockResolvedValue(session({ id: 'second', replay: { entries: [{ key: 'new', role: 'assistant', status: 'succeeded', text: 'Second answer' }], complete: true, has_more: false, watermark: 'second' } }));
  vi.mocked(startAssistantSession).mockResolvedValueOnce(session()).mockResolvedValueOnce(session({ id: 'second', replay: { entries: [{ key: 'new', role: 'assistant', status: 'succeeded', text: 'Second answer' }], complete: true, has_more: false, watermark: 'second' } }));
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  fireEvent.click(screen.getByRole('button', { name: 'New conversation' }));
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('New conversation'));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Another conversation' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await screen.findByText('Second answer');
  await act(async () => old.resolve(session({ replay: { entries: [{ key: 'old', role: 'assistant', status: 'succeeded', text: 'Detached answer' }], complete: true, has_more: false, watermark: 'old' } })));
  expect(screen.queryByText('Detached answer')).not.toBeInTheDocument();
  expect(screen.getByText('Second answer')).toBeVisible();
 });
 it('CTX-012 keeps an accepted prompt disabled until the next durable request exists', async () => {
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  vi.mocked(respondAssistantSession).mockImplementation(async (_tenant, _id, value) => ({ request_id: value.request_id, submission_id: value.submission_id, outcome: 'accepted' }));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Consume current prompt' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await waitFor(() => expect(respondAssistantSession).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.getByLabelText('Message')).toBeDisabled());
  // A stale read of the consumed prompt cannot enable a second independent send.
  fireEvent.click(screen.getByRole('button', { name: 'Refresh conversation' }));
  await waitFor(() => expect(screen.getByLabelText('Message')).toBeDisabled());
  vi.mocked(getAssistantSession).mockResolvedValue(session({ current_request_id: 'request-2' }));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh conversation' }));
  await waitFor(() => expect(screen.getByLabelText('Message')).not.toBeDisabled());
 });
 it('CTX-012 reconciles uncertain sends from recorded receipt identity', async () => {
  const oldRead = deferred<AssistantSession>();
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  vi.mocked(respondAssistantSession).mockRejectedValue(new Error('lost response'));
  vi.mocked(getAssistantSession).mockReturnValue(oldRead.promise);
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Accepted but lost response' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await screen.findByRole('alert');
  const submitted = vi.mocked(respondAssistantSession).mock.calls[0][2];
  await act(async () => oldRead.resolve(session({ current_request_id: 'request-2', replay: { entries: [
   { key: 'accepted-task', role: 'user', status: 'succeeded', text: 'Accepted but lost response', receipt: { request_id: submitted.request_id, submission_id: submitted.submission_id, outcome: 'accepted' } },
   { key: 'answer-task', role: 'assistant', status: 'succeeded', text: 'Confirmed durable reply' },
  ], watermark: 'receipt-confirmed', has_more: false, complete: true } })));
  await screen.findByText('Confirmed durable reply');
  expect(screen.getByLabelText('Message')).toHaveValue('');
  expect(screen.getByLabelText('Message')).not.toBeDisabled();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(respondAssistantSession).toHaveBeenCalledTimes(1);
 });
 it('CTX-018 retries an uncertain finish with the same identity and no new message', async () => {
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  vi.mocked(respondAssistantSession).mockRejectedValueOnce(new Error('lost finish response')).mockImplementationOnce(async (_tenant, _id, value) => ({ request_id: value.request_id, submission_id: value.submission_id, outcome: 'accepted' }));
  fireEvent.click(screen.getByRole('button', { name: 'Finish conversation' }));
  await screen.findByRole('alert');
  expect(screen.getByRole('button', { name: 'Retry finish' })).not.toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Retry finish' }));
  await waitFor(() => expect(respondAssistantSession).toHaveBeenCalledTimes(2));
  const original = vi.mocked(respondAssistantSession).mock.calls[0][2];
  expect(original.command).toBe('finish'); expect(original.prompt).toBeUndefined();
  expect(vi.mocked(respondAssistantSession).mock.calls[1][2]).toBe(original);
  expect(startAssistantSession).toHaveBeenCalledTimes(1);
 });
 it('CTX-012 accepts authoritative receipts without optimistic transcript changes', async () => {
  render(<AssistantPanel tenantId="a" />); await open(); await send();
  vi.mocked(respondAssistantSession).mockImplementation(async (_tenant, _id, value) => ({ request_id: value.request_id, submission_id: value.submission_id, outcome: 'accepted' }));
  fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Next durable turn' } });
  fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
  await waitFor(() => expect(respondAssistantSession).toHaveBeenCalledTimes(1));
  expect(screen.queryByText('Next durable turn', { selector: 'article *' })).not.toBeInTheDocument();
 });
});

function PollFixture({ load }: { load: (signal: AbortSignal) => Promise<void> }) {
 useAssistantPoll('tenant/session', true, load); return null;
}
it('CTX-016 polls once, backs off, pauses while hidden and refreshes on reconnect', async () => {
 vi.useFakeTimers();
 const pending = deferred<void>();
 const load = vi.fn().mockReturnValueOnce(pending.promise).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined);
 const view = render(<PollFixture load={load} />);
 expect(load).toHaveBeenCalledTimes(1);
 await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
 expect(load).toHaveBeenCalledTimes(1);
 await act(async () => pending.resolve());
 await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
 expect(load).toHaveBeenCalledTimes(2);
 await act(async () => { await vi.advanceTimersByTimeAsync(1999); });
 expect(load).toHaveBeenCalledTimes(2);
 await act(async () => { await vi.advanceTimersByTimeAsync(1); });
 expect(load).toHaveBeenCalledTimes(3);
 const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
 fireEvent(document, new Event('visibilitychange'));
 await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
 expect(load).toHaveBeenCalledTimes(3);
 hidden.mockReturnValue(false);
 await act(async () => { fireEvent(window, new Event('online')); });
 expect(load).toHaveBeenCalledTimes(4);
 view.unmount();
 expect(load.mock.calls.at(-1)?.[0].aborted).toBe(true);
});


describe('goal discussion lifecycle', () => {
 const goalSession=(overrides:Partial<AssistantSession>={})=>session({id:'goal-1',goal_id:'goal-1',lifecycle:'goal',...overrides});
 it('CTX-018 closes a processing job, preserves the draft, and restores launcher focus', async () => {
  const initial=goalSession({status:'processing',current_request_id:undefined});
  vi.mocked(startAssistantSession).mockResolvedValue(initial);
  vi.mocked(getAssistantSession).mockResolvedValue(initial);
  vi.mocked(closeAssistantSession).mockResolvedValue(goalSession({status:'closed',current_request_id:undefined}));
  render(<AssistantPanel tenantId="a" />);await open();await send();
  expect(screen.queryByRole('button',{name:'Finish conversation'})).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button',{name:'Close conversation'}));
  await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(screen.getByRole('button',{name:'Open assistant'})).toHaveFocus();
  expect(vi.mocked(closeAssistantSession).mock.calls[0][2].job_id).toBe('execution-1');
  expect(respondAssistantSession).not.toHaveBeenCalled();
 });
 it('CTX-012/018 keeps an uncertain close targeted at its original job and prevents reopen until reconciliation', async () => {
  const initial=goalSession();
  vi.mocked(startAssistantSession).mockResolvedValue(initial);
  vi.mocked(getAssistantSession).mockResolvedValue(initial);
  vi.mocked(closeAssistantSession).mockRejectedValueOnce(new Error('lost close')).mockResolvedValueOnce(goalSession({status:'closed',current_request_id:undefined}));
  render(<AssistantPanel tenantId="a" />);await open();await send();
  fireEvent.change(screen.getByLabelText('Message'),{target:{value:'Unsent draft'}});
  fireEvent.click(screen.getByRole('button',{name:'Close conversation'}));
  await screen.findByText(/Close outcome is uncertain/);
  expect(screen.getByLabelText('Message')).toBeDisabled();
  expect(screen.queryByRole('button',{name:'Reopen conversation'})).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole('button',{name:'Retry close'}));
  await waitFor(()=>expect(closeAssistantSession).toHaveBeenCalledTimes(2));
  expect(vi.mocked(closeAssistantSession).mock.calls[1][2]).toBe(vi.mocked(closeAssistantSession).mock.calls[0][2]);
  await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  fireEvent.click(screen.getByRole('button',{name:'Open assistant'}));
  expect(screen.getByLabelText('Message')).toHaveValue('Unsent draft');
 });
 it('CTX-012/015 retries reopen with its frozen operation and discards a late old-execution read', async () => {
  const initial=goalSession();const pendingRead=deferred<AssistantSession>();
  vi.mocked(startAssistantSession).mockResolvedValue(initial);
  vi.mocked(getAssistantSession).mockReturnValueOnce(pendingRead.promise).mockResolvedValue(goalSession({job_id:'execution-2',current_request_id:'request-2'}));
  vi.mocked(closeAssistantSession).mockResolvedValue(goalSession({status:'closed',current_request_id:undefined}));
  vi.mocked(reopenAssistantSession).mockRejectedValueOnce(new Error('lost reopen')).mockResolvedValueOnce(goalSession({job_id:'execution-2',current_request_id:'request-2'}));
  render(<AssistantPanel tenantId="a" />);await open();await send();
  await waitFor(()=>expect(getAssistantSession).toHaveBeenCalled());
  fireEvent.click(screen.getByRole('button',{name:'Close conversation'}));
  await waitFor(()=>expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  fireEvent.click(screen.getByRole('button',{name:'Open assistant'}));
  vi.mocked(assistantCapabilities).mockResolvedValue({available:true,generation:'fresh-reopen'});
  fireEvent.click(screen.getByRole('button',{name:'Reopen conversation'}));
  await screen.findByText(/Reopen outcome is uncertain/);
  const capabilityCalls=vi.mocked(assistantCapabilities).mock.calls.length;
  vi.mocked(assistantCapabilities).mockResolvedValue({available:true,generation:'later-replacement'});
  fireEvent.click(screen.getByRole('button',{name:'Retry reopen'}));
  await waitFor(()=>expect(screen.getByRole('link',{name:'View execution'})).toHaveAttribute('href',expect.stringContaining('execution-2')));
  expect(vi.mocked(reopenAssistantSession).mock.calls[1][2]).toBe(vi.mocked(reopenAssistantSession).mock.calls[0][2]);
  expect(vi.mocked(reopenAssistantSession).mock.calls[0][2]?.generation).toBe('fresh-reopen');
  expect(assistantCapabilities).toHaveBeenCalledTimes(capabilityCalls);
  await act(async()=>pendingRead.resolve(goalSession({current_request_id:'stale-request',replay:{entries:[{key:'stale',role:'assistant',status:'succeeded',text:'Stale execution reply'}],watermark:'stale',complete:true,has_more:false}})));
  expect(screen.queryByText('Stale execution reply')).not.toBeInTheDocument();
  expect(screen.getByRole('link',{name:'View execution'})).toHaveAttribute('href',expect.stringContaining('execution-2'));
  expect(respondAssistantSession).not.toHaveBeenCalled();
 });
});

it('CTX-015 ignores a send receipt from the previous execution after a refreshed replacement', async () => {
 const oldRead=deferred<AssistantSession>();
 const oldSend=deferred<{request_id:string;submission_id:string;outcome:string}>();
 const initial=session({id:'goal',goal_id:'goal',lifecycle:'goal'});
 vi.mocked(startAssistantSession).mockResolvedValue(initial);
 vi.mocked(getAssistantSession).mockReturnValueOnce(oldRead.promise);
 vi.mocked(respondAssistantSession).mockReturnValue(oldSend.promise);
 render(<AssistantPanel tenantId="a" />);await open();await send();
 await waitFor(()=>expect(getAssistantSession).toHaveBeenCalled());
 fireEvent.change(screen.getByLabelText('Message'),{target:{value:'Unconfirmed old message'}});
 fireEvent.click(screen.getByRole('button',{name:'Send message'}));
 await waitFor(()=>expect(respondAssistantSession).toHaveBeenCalled());
 const request=vi.mocked(respondAssistantSession).mock.calls[0][2];
 const replacement=session({id:'goal',goal_id:'goal',lifecycle:'goal',job_id:'execution-2',current_request_id:'new-prompt'});
 vi.mocked(getAssistantSession).mockResolvedValue(replacement);
 await act(async()=>oldRead.resolve(replacement));
 await act(async()=>oldSend.resolve({request_id:request.request_id,submission_id:request.submission_id,outcome:'accepted'}));
 expect(screen.getByRole('link',{name:'View execution'})).toHaveAttribute('href',expect.stringContaining('execution-2'));
 expect(screen.getByLabelText('Message')).not.toBeDisabled();
 expect(screen.getByLabelText('Message')).toHaveValue('Unconfirmed old message');
});
