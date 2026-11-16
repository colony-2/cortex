import { API_BASE } from './apiBase';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  assistantArtifactURL, freezeAssistantStart, freezeAssistantSubmission, startAssistantSession,
  respondAssistantSession, getAssistantSession, mergeAssistantReplay, AssistantAPIError,
  freezeAssistantLifecycle, closeAssistantSession, reopenAssistantSession,
} from './assistant';

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });
describe('assistant browser boundary', () => {
  it('CTX-012 freezes start identity and payload across retry and navigation', async () => {
    const source = { kind: 'review' as const, job_id: 'job/child', request_id: 'exact-review' };
    const input = freezeAssistantStart('Read this exact review', source);
    source.request_id = 'replacement';
    const fetcher = vi.fn().mockRejectedValueOnce(new Error('lost response')).mockResolvedValue(new Response('{}'));
    vi.stubGlobal('fetch', fetcher);
    await expect(startAssistantSession('tenant/a', input)).rejects.toThrow('lost response');
    await startAssistantSession('tenant/a', input);
    expect(fetcher.mock.calls[0][0]).toBe(`${API_BASE}/projects/tenant%2Fa/assistant/sessions`);
    expect(fetcher.mock.calls[0][1].body).toBe(fetcher.mock.calls[1][1].body);
    expect(JSON.parse(fetcher.mock.calls[1][1].body).source.request_id).toBe('exact-review');
    expect(Object.isFrozen(input)).toBe(true);
    expect(Object.isFrozen(input.source)).toBe(true);
  });
  it('CTX-025 projects discussion fields and rejects executable commands', async () => {
    const fetcher = vi.fn().mockImplementation(() => Promise.resolve(new Response('{}')));
    vi.stubGlobal('fetch', fetcher);
    const input = freezeAssistantStart('hello', { kind: 'global' });
    await startAssistantSession('a', { ...input, model: 'attacker', api_key: 'attacker' } as typeof input);
    expect(fetcher.mock.calls[0][1].body).not.toMatch(/model|api_key/);
    const turn = freezeAssistantSubmission('request', 'send', 'discuss');
    await respondAssistantSession('a', 's', turn);
    await expect(respondAssistantSession('a', 's', { ...turn, command: 'confirm' } as unknown as typeof turn)).rejects.toBeInstanceOf(AssistantAPIError);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('does not reflect unsafe runtime errors and bounds hung requests', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'synthetic credential', code: 'unavailable' }), { status: 503 })));
    await expect(getAssistantSession('a', 's')).rejects.toThrow('Assistant is unavailable');
    vi.useFakeTimers();
    const fetcher = vi.fn((_url, init: RequestInit) => new Promise((_resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
    }));
    vi.stubGlobal('fetch', fetcher);
    const result = getAssistantSession('a', 's').catch(error => error);
    await vi.advanceTimersByTimeAsync(20_000);
    expect((await result).name).toBe('AbortError');
  });
  it('CTX-011 builds only opaque citation download routes', () => {
    expect(assistantArtifactURL('tenant/a', 'session/b', 'https://evil.test/a?b')).toBe(`${API_BASE}/projects/tenant%2Fa/assistant/sessions/session%2Fb/artifacts/https%3A%2F%2Fevil.test%2Fa%3Fb`);
  });
  it('CTX-013 prepends older continuation pages in chronological order', () => {
    const recent = [{ key: 'task-late', role: 'assistant', status: 'succeeded', text: 'newer' }];
    expect(mergeAssistantReplay(recent, { entries: [{ key: 'task-early', role: 'user', status: 'succeeded', text: 'earlier' }], complete: true, has_more: false, watermark: 'w' }).map(entry => entry.key)).toEqual(['task-early', 'task-late']);
  });
  it('CTX-013 merges authoritative replay by task origin without invented replies', () => {
    const previous = [{ key: 'task-1', role: 'assistant', status: 'running' }];
    expect(mergeAssistantReplay(previous, { entries: [{ key: 'task-1', role: 'assistant', status: 'succeeded', text: 'recorded' }], complete: true, has_more: false, watermark: 'w' })).toEqual([
      { key: 'task-1', role: 'assistant', status: 'succeeded', text: 'recorded' },
    ]);
  });
});

it('CTX-012/018 preserves the observed execution and complete lifecycle envelope on retry', async () => {
 const fetcher=vi.fn().mockRejectedValueOnce(new Error('lost reply')).mockImplementation(() => Promise.resolve(new Response('{}')));
 vi.stubGlobal('fetch',fetcher);
 const operation=freezeAssistantLifecycle('observed-job');
 await expect(closeAssistantSession('tenant','goal',operation)).rejects.toThrow('lost reply');
 await closeAssistantSession('tenant','goal',operation);
 expect(fetcher.mock.calls[0][1].body).toBe(fetcher.mock.calls[1][1].body);
 await reopenAssistantSession('tenant','goal',{...operation, model:'override'} as typeof operation);
 expect(JSON.parse(fetcher.mock.calls[2][1].body)).toEqual(operation);
 expect(Object.isFrozen(operation)).toBe(true);
});

// Expected lengths come from Go encoding/json with its default HTML escaping.
it('matches raw UTF-8 and Go JSON string-content lengths at inclusive boundaries', async () => {
 const { assistantDraftError, goJSONStringContentBytes } = await import('./assistant');
 const v4 = { raw_utf8_bytes: 16384, go_json_string_content_bytes: 16384, http_body_bytes: 40960 };
 for (const [text, bytes] of [['a',1],['雪',3],['🧪',4],['\n"\\\t\r\b\f',14],['\u0000\u001f',12],['<>&\u2028\u2029',30]] as const) {
  expect(goJSONStringContentBytes(text)).toBe(bytes);
  const n = Math.floor(16384 / bytes);
  const exact = text.repeat(n) + 'a'.repeat(16384 - n * bytes);
  expect(goJSONStringContentBytes(exact)).toBe(16384);
  expect(assistantDraftError(exact, v4)).toBeUndefined();
  expect(assistantDraftError(exact + 'a', v4)).toMatch(/exceeds/);
 }
 for (const limit of [100,16384,32768]) {
  const historical = { raw_utf8_bytes:limit, http_body_bytes:40960 };
  expect(assistantDraftError('a'.repeat(limit), historical)).toBeUndefined();
  expect(assistantDraftError('a'.repeat(limit+1), historical)).toMatch(/UTF-8/);
  expect(assistantDraftError('\n'.repeat(limit), historical)).toBeUndefined();
 }
 expect(assistantDraftError('a'.repeat(101),{...v4,raw_utf8_bytes:100})).toMatch(/100 UTF-8/);
 expect(assistantDraftError('<'.repeat(17),{...v4,go_json_string_content_bytes:100})).toMatch(/100 bytes after service JSON/);
 for (const budget of [undefined, {} as typeof v4, {...v4,raw_utf8_bytes:NaN}, {...v4,go_json_string_content_bytes:0}, {...v4,http_body_bytes:50000}]) {
  expect(assistantDraftError('draft',budget)).toMatch(/Refresh/);
 }
 expect(assistantDraftError('draft',{raw_utf8_bytes:16384,http_body_bytes:40960},true)).toMatch(/Refresh/);
 expect(assistantDraftError('\ud800',v4)).toMatch(/invalid Unicode/);
});
it('measures the complete projected assistant body that is transmitted', async () => {
 const { assistantStartBody, assistantSubmissionBody, assistantBodyError } = await import('./assistant');
 const fetcher = vi.fn().mockImplementation(() => Promise.resolve(new Response('{}')));
 vi.stubGlobal('fetch', fetcher);
 const start = freezeAssistantStart('雪\n<>&', {kind:'review',job_id:'job',request_id:'review'},'generation');
 const submission = freezeAssistantSubmission('request','send','🧪\n');
 for (const [body, send] of [[assistantStartBody(start),()=>startAssistantSession('a',start)], [assistantSubmissionBody(submission),()=>respondAssistantSession('a','s',submission)]] as const) {
  const size = new TextEncoder().encode(JSON.stringify(body)).byteLength;
  const budget = {raw_utf8_bytes:16384,http_body_bytes:size};
  expect(assistantBodyError(body,budget)).toBeUndefined();
  expect(assistantBodyError(body,{...budget,http_body_bytes:size-1})).toMatch(/including JSON encoding/);
  await send();
  expect(fetcher.mock.calls.at(-1)![1].body).toBe(JSON.stringify(body));
 }
});
it('releases a draft only for explicit proven rejection metadata', async () => {
 for (const [code, submission, expected] of [['invalid',undefined,undefined], ['limit',undefined,undefined], ['draft_rejected',undefined,undefined], ['draft_rejected','not_started','not_started']] as const) {
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(JSON.stringify({code,submission,error:'Shorten the message'}),{status:400})));
  const error = await startAssistantSession('a',freezeAssistantStart('draft',{kind:'global'})).catch(error=>error);
  expect(error.submission).toBe(expected);
  expect(error.message.toLowerCase()).toContain(expected ? 'shorten' : 'assistant');
 }
});
