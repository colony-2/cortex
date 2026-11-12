import { expect, it } from 'vitest';
import type { TreeNode } from '@colony2/shared';
import { timing } from './timing';
const now = Date.parse('2026-11-12T09:18:42Z');
const root: TreeNode = {tenant_id:'t',job_id:'root',job_type:'generic',status:'ACTIVE',created_at:'2026-11-12T08:58:00Z'};
const child: TreeNode = {...root,job_id:'child',created_at:'2026-11-12T09:00:10Z'};
it('DAGUI-009,010 root date/UTC, elapsed child and zero offsets', () => {
 expect(timing(root,root,now).submission).toMatch(/Nov 12, 2026.*08:58 UTC/);
 expect(timing(child,root,now).submission).toBe('Submitted +2m 10s');
 expect(timing({...child,created_at:root.created_at},root,now).submission).toBe('Submitted +0s');
});
it.each(['READY','ACTIVE','PENDING_JOBS','AWAITING_FUTURE','EXPIRED','CRASH_CONCERN'])('DAGUI-011 live %s includes waiting and cancellation request', status => {
 expect(timing({...child,status,cancel_requested:true},root,now).span).toBe('Age 18m 32s');
});
it('DAGUI-012 finished spans freeze, including custom and cancelled outcomes', () => {
 for (const completion_status of ['success','cancelled','custom']) {
  const n = {...child,status:'COMPLETED',completion_status,created_at:'2026-11-12T09:01:48Z',archived_at:'2026-11-12T09:07:20Z'};
  expect(timing(n,root,now).span).toBe('Time to finish 5m 32s');
  expect(timing(n,root,now+600000).span).toBe(timing(n,root,now).span);
 }
});
it.each([undefined,'bad','0001-01-01T00:00:00Z','1970-01-01T00:00:00Z','2026-11-12T09:30:00Z','2026-11-12T08:00:00Z'])('DAGUI-013,014 invalid end %s never grows or substitutes update time', archived_at => {
 const n = {...child,status:'COMPLETED',archived_at,updated_at:'2026-11-12T09:07:20Z'};
 expect(timing(n,root,now).span).toBe('Time to finish Unavailable');
});
it('DAGUI-014,015 independent inputs, reversed root, unknown lifecycle and zero spans', () => {
 expect(timing(child,undefined,now)).toEqual({submission:'Submitted Unavailable',span:'Age 18m 32s'});
 expect(timing(child,{...root,created_at:undefined},now).span).toBe('Age 18m 32s');
 expect(timing(root,child,now).submission).toBe('Submitted Unavailable');
 expect(timing({...child,status:'UNKNOWN'},root,now).span).toBe('Age Unavailable');
 expect(timing({...child,status:'COMPLETED',archived_at:child.created_at},root,now).span).toBe('Time to finish 0s');
 expect(timing(child,root,Date.parse(child.created_at!)).span).toBe('Age 0s');
 for (const created_at of [undefined,'bad','0','2026-11-12','2026-02-31T09:00:00Z','0001-01-01T00:00:00Z','1970-01-01T00:00:00Z','2026-12-01T00:00:00Z']) {
  expect(timing({...child,created_at},root,now)).toEqual({submission:'Submitted Unavailable',span:'Age Unavailable'});
 }
});
