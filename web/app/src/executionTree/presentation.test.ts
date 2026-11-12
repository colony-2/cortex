import { expect, it } from 'vitest';
import type { TreeNode } from '@colony2/shared';
import { headline, jobStatus, statusPalette } from './presentation';
import { jobStatusPresentation } from '../utils/jobStatus';
const n: TreeNode = {tenant_id:'t',job_id:'id',job_type:'generic',status:'READY'};
it('DAGUI-004 title preference and Unicode/whitespace prompt fallback', () => {
 expect(headline({...n,title:'  readable title  ',prompt:'ignored'})).toBe('readable title');
 expect(headline({...n,title:' \n ',prompt:'  snow\n 雪  prompt  '})).toBe('snow 雪 prompt');
 expect(headline({...n,prompt:'🌳'.repeat(80)})).toBe('🌳'.repeat(75)+'...');
 expect(headline(n)).toBe('Untitled job');
});
it('DAGUI-007,008 status parity for generic summaries and previous retry failures', () => {
 for (const status of ['READY','PENDING_JOBS','AWAITING_FUTURE','ACTIVE','CRASH_CONCERN','EXPIRED','CANCELLED','mystery']) {
  expect(jobStatus({...n,status,completion_status:'failed_app'})).toEqual(jobStatusPresentation(status));
  expect(statusPalette[jobStatus({...n,status}).color]).toBeDefined();
 }
 expect(jobStatus({...n,cancel_requested:true}).label).toBe('Cancellation requested');
 expect(jobStatus({...n,status:'COMPLETED',completion_status:'custom'}).label).toBe('Finished (custom)');
 expect(jobStatus({...n,store:'ARCHIVED',status:'COMPLETED'}).label).toBe('Finished (outcome unavailable)');
 expect(jobStatus({...n,store:'ARCHIVED',completion_status:'success',cancel_requested:true}).label).toBe('Succeeded');
 expect(jobStatus({...n,store:'ARCHIVED'})).toEqual(jobStatusPresentation('READY'));
 expect(jobStatus({...n,status:'COMPLETED',completion_status:'cancelled'}).label).toBe('Cancelled');
});
