import { afterEach, expect, it, vi } from 'vitest';
import { getBuildInformation } from './buildInfo';
afterEach(() => vi.unstubAllGlobals());
it.each([{}, { libraries: {} }, { libraries: { c2j: { module: 'c2j', version: 123, commit: null }, jobdb: { module: 'jobdb', version: null, commit: null } } }])('rejects unsupported metadata %j', async value => {
 vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ok:true,json:async()=>value}));
 await expect(getBuildInformation()).rejects.toThrow('Unsupported');
});
it('preserves individual unknown values',async()=>{
 const value={libraries:{c2j:{module:'c2j',version:'v1',commit:null},jobdb:{module:'jobdb',version:null,commit:'abc'}}};
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>value}));
 await expect(getBuildInformation()).resolves.toEqual(value);
});
