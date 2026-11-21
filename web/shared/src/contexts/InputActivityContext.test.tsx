import {act,renderHook,waitFor,cleanup} from '@testing-library/react';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {InputActivityProvider,useInputActivity,useCurrentInput} from './InputActivityContext';
import {inputActivityService as service} from '../services/inputActivityService';
const identity={cell_id:'cell',cell_name:'platform',repo:'github.com/acme/platform'};
const row={...identity,id:'job',client_payload_revision:1};
class Source {
 static latest: Source;
 readyState=0;onopen?:()=>void;onerror?:()=>void;
 listeners=new Map<string,Function>();
 constructor(){Source.latest=this;}
 addEventListener(name:string,fn:Function){this.listeners.set(name,fn);}
 close(){}
 snapshot(inputs:unknown[]){this.listeners.get('inputs_changed')?.({data:JSON.stringify({inputs})});}
}
beforeEach(()=>{service.disconnect();service.clearCaches();vi.stubGlobal('EventSource',Source);vi.stubGlobal('fetch',vi.fn());});
afterEach(()=>{cleanup();service.disconnect();service.clearCaches();vi.unstubAllGlobals();});
function mount(){
 const hook=renderHook(()=>({activity:useInputActivity(),input:useCurrentInput('tenant','job')}),{wrapper:InputActivityProvider});
 act(()=>hook.result.current.activity.setCurrentProjectId('tenant'));return hook;
}
it('exposes loading rather than zero until a fresh empty observation, and stale rows after disconnect',async()=>{
 vi.mocked(fetch).mockImplementation(()=>new Promise(()=>{}));
 const {result}=mount();expect(result.current.activity.discoveryStatus).toBe('loading');
 act(()=>Source.latest.onopen?.());expect(result.current.activity.discoveryStatus).toBe('loading');
 act(()=>Source.latest.snapshot([]));expect(result.current.activity.discoveryStatus).toBe('current');expect(result.current.activity.pendingCount).toBe(0);
 act(()=>Source.latest.snapshot([row]));act(()=>Source.latest.onerror?.());
 expect(result.current.activity.discoveryStatus).toBe('unavailable');expect(result.current.activity.pendingCount).toBe(1);expect(result.current.input.details).toBeUndefined();
});
it('suppresses actions during outages and restores the same verified details without another read',async()=>{
 vi.mocked(fetch).mockImplementation(async url=>new Response(JSON.stringify(String(url).endsWith('/pending')?[row]:{jobId:'job',status:'pending',startTime:'now',form:{question:'Continue?'}})));
 const {result}=mount();await waitFor(()=>expect(result.current.input.details?.form.question).toBe('Continue?'));
 const count=vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/job')).length;
 act(()=>Source.latest.onerror?.());expect(result.current.input.details).toBeUndefined();expect(result.current.input.readableDetails?.form.question).toBe('Continue?');
 act(()=>Source.latest.snapshot([row]));await waitFor(()=>expect(result.current.input.details?.form.question).toBe('Continue?'));
 expect(vi.mocked(fetch).mock.calls.filter(([url])=>String(url).endsWith('/job'))).toHaveLength(count);
});
it('never exposes another tenant’s rows while selection changes',async()=>{
 vi.mocked(fetch).mockImplementation(async url=>new Response(JSON.stringify(String(url).includes('/tenant/')?[row]:[])));
 const {result}=mount();await waitFor(()=>expect(result.current.activity.pendingCount).toBe(1));
 act(()=>result.current.activity.setCurrentProjectId('other'));
 expect(result.current.activity.pendingInputs).toEqual([]);expect(result.current.activity.discoveryStatus).toBe('loading');
});

it.each([false,true])('clears only ordinary prompts on fresh absence, including a later outage (review=%s)',async review=>{
 const form=review?{kind:'review',request_id:'request',question:'Continue?'}:{question:'Continue?'};
 vi.mocked(fetch).mockImplementation(async url=>new Response(JSON.stringify(String(url).endsWith('/pending')?[row]:{jobId:'job',status:'pending',startTime:'now',form})));
 const {result}=mount();await waitFor(()=>expect(result.current.input.details?.form.question).toBe('Continue?'));
 act(()=>Source.latest.snapshot([]));
 expect(result.current.input.details).toBeUndefined();
 expect(result.current.input.readableDetails?.form.question).toBe(review?'Continue?':undefined);
 act(()=>Source.latest.onerror?.());
 expect(result.current.input.readableDetails?.form.question).toBe(review?'Continue?':undefined);
});

it('clears an ordinary prompt when a retry returns 204 despite a stale matching snapshot',async()=>{
 let finished=false;
 vi.mocked(fetch).mockImplementation(async url=>String(url).endsWith('/pending')?new Response(JSON.stringify([row])):finished?new Response(null,{status:204}):new Response(JSON.stringify({jobId:'job',status:'pending',startTime:'now',form:{question:'Continue?'}})));
 const {result}=mount();await waitFor(()=>expect(result.current.input.details?.form.question).toBe('Continue?'));
 finished=true;
 await act(async()=>{await result.current.input.refresh();});
 expect(result.current.input.details).toBeNull();
 expect(result.current.input.readableDetails).toBeUndefined();
 act(()=>Source.latest.onerror?.());
 expect(result.current.input.readableDetails).toBeUndefined();
});
