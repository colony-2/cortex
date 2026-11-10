import { afterEach, describe, expect, it, vi } from 'vitest';
import { getConversation, getConversationEntry, conversationArtifactURL } from './conversation';
import { stringifyJobJSON } from './jobJSON';
afterEach(() => vi.unstubAllGlobals());
describe('CT-032 conversation transport', () => {
  it('preserves opaque tokens, encoded identities, capability and exact numbers', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('{"tenant_id":"a/b","job_id":"job","entries":[],"snapshot_token":"opaque+/=","completeness":"partial","diagnostics":[{"code":"unsupported","message":"closure"}],"value":9007199254740993}'));
    vi.stubGlobal('fetch', fetch);
    const page = await getConversation('a/b', 'job', { pageToken: '+/= ?' });
    expect(fetch.mock.calls[0][0]).toContain('a%2Fb');
    expect(new URL(fetch.mock.calls[0][0], 'http://test').searchParams.get('page_token')).toBe('+/= ?');
    expect(page.completeness).toBe('partial');
    expect(stringifyJobJSON(page)).toContain('9007199254740993');
    const url = new URL(conversationArtifactURL('a/b','job','entry/?','member/+','+/='), 'http://test');
    expect(url.searchParams.get('entry_id')).toBe('entry/?');
    expect(url.searchParams.get('snapshot_token')).toBe('+/=');
  });
  it('CT-033 retains typed expiry errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"error":"refresh snapshot","code":"history_snapshot_expired"}', {status:410})));
    await expect(getConversation('t','j')).rejects.toMatchObject({status:410,code:'history_snapshot_expired'});
  });
  it('CT-022 rejects a detail for another occurrence', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"entry_id":"other"}')));
    await expect(getConversationEntry('t','j','selected')).rejects.toThrow('selected occurrence');
  });
});

describe('CC-020 legacy request identity',()=>{
 it('rejects reused IDs across pages instead of selecting the first occurrence',async()=>{
  const {getConversationRequest}=await import('./conversation');
  const fetch=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({tenant_id:'t',job_id:'j',snapshot_token:'s',entries:[{entry_id:'one',request_id:'reused'}],next_page_token:'next'}))).mockResolvedValueOnce(new Response(JSON.stringify({tenant_id:'t',job_id:'j',snapshot_token:'s',entries:[{entry_id:'two',request_id:'reused'}]})));
  vi.stubGlobal('fetch',fetch);
  await expect(getConversationRequest('t','j','reused')).rejects.toThrow('ambiguous');expect(fetch).toHaveBeenCalledTimes(2);
 });
 it('opens a unique legacy request using its occurrence and snapshot',async()=>{
  const {getConversationRequest}=await import('./conversation');
  const fetch=vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({tenant_id:'t',job_id:'j',snapshot_token:'s',entries:[{entry_id:'only',request_id:'unique'}]}))).mockResolvedValueOnce(new Response(JSON.stringify({entry_id:'only'})));
  vi.stubGlobal('fetch',fetch);await getConversationRequest('t','j','unique');
  expect(fetch.mock.calls[1][0]).toContain('entry_id=only&snapshot_token=s');
 });
});
