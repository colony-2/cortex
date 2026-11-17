import { expect, it } from 'vitest';
import type { TreeWaitsResponse } from '@colony2/shared';
import { graphTargets, legacyRows, normalizeWaits, waitKey, waitPath, waitTargets, WAIT_LIMITS } from './waits';
export const at='2026-11-17T00:00:00Z';
export const page = (dependencies: any[] = [], token = ''): TreeWaitsResponse => ({consumer:{tenant_id:'t',job_id:'consumer'},waits:[{jobKey:{tenantId:'t',jobId:'consumer'},waitId:'w',state:'PENDING',dependencies}],observed_at:at,issues:[],next_page_token:token,consumer_snapshot:{state:'available',observed_at:at,status:'PENDING_JOBS'}});
export const job = (id='independent',condition='complete') => ({target:{jobId:id},condition,state:'PENDING'});
it('keeps exact job, goal, multiple conditions and legacy identities without target hydration',()=>{
 const p=page([job(),{...job('independent','success')}, {target:{goalId:'independent'},condition:'success',state:'PENDING'}, {...job('resolved'),state:'SATISFIED'}]);
 const n=normalizeWaits(p,'t','consumer',1000);
 expect(n.issues).toEqual([]); expect(n.inspected).toBe(4);expect(n.rows).toHaveLength(3);
 const targets=waitTargets([...n.rows,...legacyRows('t',['independent','independent'])]);
 expect(targets).toHaveLength(2);expect(targets[0].rows).toHaveLength(3);expect(targets[1].kind).toBe('goal');
 expect(waitKey('other','job','independent')).not.toBe(targets[0].key);
 expect(waitPath('t/雪','job','j/#')).toBe('/project/t%2F%E9%9B%AA/jobs/j%2F%23/tree');
 expect(waitPath('t/雪','goal','j/#')).toBe('/project/t%2F%E9%9B%AA/goals/j%2F%23');
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
it('caps distinct graph identities and exchanges a loaded target without adding ancestry',()=>{
 const targets=waitTargets(normalizeWaits(page(Array.from({length:100},(_,i)=>job(String(i)))),'t','consumer',1000).rows);
 expect(graphTargets(targets)).toHaveLength(WAIT_LIMITS.targets);
 expect(graphTargets(targets,targets[99].key).at(-1)).toBe(targets[99]);
 expect(graphTargets(targets,targets[99].key)).toHaveLength(50);
});
