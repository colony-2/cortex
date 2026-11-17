import { expect, it, vi } from 'vitest';
import { TreeWaits } from './useTreeWaits';
import type { TreeWaitsResponse } from '@colony2/shared';
const at='2026-11-17T00:00:00Z';
const page = (dependencies: any[] = [], token = ''): TreeWaitsResponse => ({consumer:{tenant_id:'t',job_id:'consumer'},waits:[{jobKey:{tenantId:'t',jobId:'consumer'},waitId:'w',state:'PENDING',dependencies}],observed_at:at,issues:[],next_page_token:token,consumer_snapshot:{state:'available',observed_at:at,status:'PENDING_JOBS'}});
const job = (id='independent',condition='complete') => ({target:{jobId:id},condition,state:'PENDING'});
const tick=async()=>{for(let i=0;i<5;i++)await Promise.resolve();};
it('one explicit page, continuation failure/retry, repeated cursor and refresh replacement',async()=>{
 let fail=false, generation=0;
 const read=vi.fn(async(_t:string,_j:string,token:string)=>{
  if(fail)throw Error();
  if(generation)return page([job('new')]);
  if(token) { const p=page([job('second')],'next');p.waits[0].waitId='w2';return p; }
  return page([job()],'next');
 });
 const c=new TreeWaits('t','consumer',()=>{},read);
 c.refresh();await tick();expect(read).toHaveBeenCalledTimes(1);expect(c.state.token).toBe('next');expect(c.rows).toHaveLength(1);
 fail=true;await c.load();expect(c.state.error).toMatch('partial');expect(c.canLoad).toBe(true);expect(c.rows).toHaveLength(1);
 fail=false;await c.load();expect(c.state.stopped).toMatch('Repeated cursor');expect(c.canLoad).toBe(false);expect(c.rows).toHaveLength(2);
 fail=true;c.refresh();await tick();expect(c.state.stale).toBe(true);expect(c.rows[0].id).toBe('independent');
 fail=false;generation++;c.refresh();await tick();expect(c.state.stale).toBe(false);expect(c.rows.map(r=>r.id)).toEqual(['new']);
});
it('obsolete responses, pauses and fresh empty observations preserve truthful coverage',async()=>{
 let release!: (p:ReturnType<typeof page>)=>void;
 const read=vi.fn().mockImplementationOnce(()=>new Promise(r=>release=r)).mockResolvedValue(page());
 const c=new TreeWaits('t','consumer',()=>{},read);
 c.refresh();const signal=read.mock.calls[0][3];c.pause();expect(signal.aborted).toBe(true);
 c.resume();await tick();release(page([job('stale')]));await tick();expect(c.rows).toEqual([]);expect(c.state.loaded).toBe(true);expect(c.state.stale).toBe(false);
 expect(read).toHaveBeenCalledTimes(2);
});
it('caps page and row budgets, counts resolved entries and preserves legacy unknown conditions',async()=>{
 const paged=new TreeWaits('t','consumer',()=>{},vi.fn(async(_t,_j,token)=>page([],String(Number(token||0)+1))));
 paged.refresh();await tick();for(let i=0;i<20;i++)await paged.load();
 expect(paged.state.pages).toBe(10);expect(paged.state.stopped).toBe('Wait page limit reached');
 const many=new TreeWaits('t','consumer',()=>{},vi.fn(async()=>page(Array.from({length:1200},(_,i)=>({...job(String(i)),state:'SATISFIED'})))));
 many.refresh();await tick();expect(many.state.inspected).toBe(1000);expect(many.rows).toHaveLength(0);expect(many.state.stopped).toMatch('row limit');
 const p=page();p.consumer_snapshot!.legacy_wait_for=['legacy'];
 const c=new TreeWaits('t','consumer',()=>{},vi.fn(async()=>p));c.refresh();await tick();expect(c.rows[0]).toMatchObject({legacy:true,condition:'condition unavailable'});
});
