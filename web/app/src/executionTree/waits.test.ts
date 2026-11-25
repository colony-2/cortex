import { expect, it } from 'vitest';
import type { TreeWaitsResponse } from '@colony2/shared';
import { legacyRows, normalizeWaits, waitKey, waitPath, waitTargets } from './waits';
export const at='2026-11-17T00:00:00Z';
export const page = (dependencies: any[] = [], token = ''): TreeWaitsResponse => ({consumer:{tenant_id:'t',job_id:'consumer'},waits:[{jobKey:{tenantId:'t',jobId:'consumer'},waitId:'w',state:'PENDING',dependencies}],observed_at:at,issues:[],next_page_token:token,consumer_snapshot:{state:'available',observed_at:at,status:'PENDING_JOBS'}});
export const job = (id='independent',condition='complete') => ({target:{jobId:id},condition,state:'PENDING'});
it('keeps job waits and their conditions while omitting a goal with the same ID',()=>{
 const p=page([job(),{...job('independent','success')}, {target:{goalId:'independent'},condition:'success',state:'PENDING'}, {...job('resolved'),state:'SATISFIED'}]);
 const n=normalizeWaits(p,'t','consumer',1000);
 expect(n.issues).toEqual([]); expect(n.inspected).toBe(4);expect(n.rows).toHaveLength(2);expect(n.hasGoalWait).toBe(true);
 const targets=waitTargets([...n.rows,...legacyRows('t',['independent','independent'])]);
 expect(targets).toHaveLength(1);expect(targets[0].rows).toHaveLength(3);
 expect(waitKey('other','independent')).not.toBe(targets[0].key);
 expect(waitPath('t/雪','j/#')).toBe('/project/t%2F%E9%9B%AA/jobs/j%2F%23/tree');
});
it('malformed, foreign, future and truncated records cannot establish absence',()=>{
 const p=page([job('',''),{...job('dual'),target:{jobId:'dual',goalId:'dual'}},{...job(),state:'FUTURE'},job('future','literal-future'),null]);
 const n=normalizeWaits(p,'t','consumer',1000);
 expect(n.rows.map(r=>r.condition)).toEqual(['literal-future']);expect(n.issues.length).toBeGreaterThan(2);
 p.waits.push({...p.waits[0],jobKey:{tenantId:'private',jobId:'secret'}});
 expect(normalizeWaits(p,'t','consumer',1000).rows).toHaveLength(1);
 const capped=normalizeWaits(page(Array.from({length:1200},(_,i)=>job(String(i)))),'t','consumer',1000);
 expect(capped.rows).toHaveLength(1000);expect(capped.issues).toContain('Dependency row limit reached');
});
it('retains 51 distinct targets for the shared graph without overlay clipping',()=>{
 const targets=waitTargets(normalizeWaits(page(Array.from({length:51},(_,i)=>job(String(i)))),'t','consumer',1000).rows);
 expect(targets).toHaveLength(51);
});
