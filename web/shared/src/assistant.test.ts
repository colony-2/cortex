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
