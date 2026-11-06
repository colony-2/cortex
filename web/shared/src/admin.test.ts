import { afterEach, expect, it, vi } from 'vitest';
import { getAdmin, updateAdmin } from './admin';
afterEach(() => vi.unstubAllGlobals());
it('encodes tenant and cell selectors and sends settings revisions', async () => {
 const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ revision: 'r1' }), { status: 200 }));
 vi.stubGlobal('fetch', fetch);
 await getAdmin('tenant/name', 'cell & id');
 expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('/projects/tenant%2Fname/admin?cellId=cell%20%26%20id'), { cache: 'no-store' });
 await updateAdmin('tenant/name', 'hooks', { name: '.c2/hook.yaml', operation: 'clear', revision: 'r1' }, 'cell & id');
 expect(fetch).toHaveBeenLastCalledWith(expect.stringContaining('/admin/hooks?cellId=cell%20%26%20id'), expect.objectContaining({ method: 'POST', body: JSON.stringify({ name: '.c2/hook.yaml', operation: 'clear', revision: 'r1' }) }));
});
it('never includes a secret provider response body in an error', async () => {
 const response = new Response('provider echoed sensitive-value', { status: 502 });
 const text = vi.spyOn(response, 'text');
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));
 await expect(updateAdmin('tenant', 'secrets', { name: 'token', operation: 'set', value: 'sensitive-value' })).rejects.toThrow('Secret update failed.');
 expect(text).not.toHaveBeenCalled();
});
