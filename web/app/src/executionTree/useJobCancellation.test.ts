import { act, renderHook } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { getTreeNode, type TreeNode } from '@colony2/shared';
import { canCancelNode, useJobCancellation } from './useJobCancellation';
vi.mock('@colony2/shared', () => ({getTreeNode:vi.fn()}));
const node: TreeNode = {tenant_id:'t',job_id:'child',parent_job_id:'opened',job_type:'generic',status:'ACTIVE'};
beforeEach(() => { vi.mocked(getTreeNode).mockReset(); });
it.each(['recipe','generic'])('permits every unfinished %s state despite earlier failed attempts', job_type => {
 for (const status of ['READY','PENDING_JOBS','AWAITING_FUTURE','ACTIVE','CRASH_CONCERN','EXPIRED'])
  expect(canCancelNode({...node,job_type,status,completion_status:'failed_system'})).toBe(true);
 for (const extra of [{status:'COMPLETED'}, {store:'ARCHIVED'}, {archived_at:'now'}, {cancel_requested:true}, {status:''}, {status:'future-state'}, {status:'CANCELLED'}])
  expect(canCancelNode({...node, ...extra})).toBe(false);
});
it.each([true,false])('reconciles the exact job after acknowledged=%s and retains accepted pending evidence', async success => {
 vi.mocked(getTreeNode).mockResolvedValue({node,observed_at:'now'});
 const {result} = renderHook(() => useJobCancellation('t','opened'));
 act(() => result.current.open(node));
 await act(async () => { await result.current.settled(success?['child']:[],success?[]:[{id:'child',error:'failed request'}]); });
 expect(getTreeNode).toHaveBeenCalledWith('t','child',expect.any(AbortSignal));
 expect(result.current.eligible(node)).toBe(!success);
 expect(result.current.overlay(node).cancel_requested).toBe(success || undefined);
 if (!success) expect(result.current.state('child')?.error).toBe('failed request');
});
it.each([
 {status:'COMPLETED',completion_status:'success'},
 {store:'ARCHIVED',completion_status:'new-outcome'},
 {archived_at:'now',completion_status:undefined},
 {cancel_requested:true},
])('resolves an HTTP failure using persisted evidence %j', async extra => {
 const fresh = {...node,...extra};
 vi.mocked(getTreeNode).mockResolvedValue({node:fresh,observed_at:'now'});
 const {result} = renderHook(() => useJobCancellation('t','opened'));
 act(() => result.current.open(node));
 let close;
 await act(async () => { close=await result.current.settled([],[{id:'child',error:'HTTP 500'}]); });
 expect(close).toBe(true); expect(result.current.eligible(result.current.overlay(node))).toBe(false);
 expect(result.current.overlay(node)).toMatchObject(fresh);
});
it.each(['missing','forbidden','wrong-tenant','wrong-job'])('blocks retry after %s read and refreshes without resubmitting', async failure => {
 if (failure==='wrong-tenant'||failure==='wrong-job') vi.mocked(getTreeNode).mockResolvedValue({node:{...node,...(failure==='wrong-tenant'?{tenant_id:'foreign'}:{job_id:'neighbor'})},observed_at:'now'});
 else vi.mocked(getTreeNode).mockRejectedValue(Error(failure));
 const {result} = renderHook(() => useJobCancellation('t','opened'));
 act(() => result.current.open(node));
 await act(async () => {await result.current.settled(['child'],[]);});
 expect(result.current.state('child')).toMatchObject({acknowledged:true,unavailable:true});
 expect(result.current.eligible(node)).toBe(false);
 vi.mocked(getTreeNode).mockResolvedValue({node,observed_at:'now'});
 await act(async () => { await result.current.refresh(node); });
 expect(result.current.overlay(node).cancel_requested).toBe(true);
 expect(getTreeNode).toHaveBeenCalledTimes(2);
});
it('ignores superseded reads and replies after identity change or unmount', async () => {
 const replies: ((value:any)=>void)[]=[];
 vi.mocked(getTreeNode).mockImplementation(()=>new Promise(resolve=>replies.push(resolve)));
 const {result,rerender,unmount} = renderHook(({tenant})=>useJobCancellation(tenant,'opened'),{initialProps:{tenant:'t'}});
 let old!:Promise<unknown>,newer!:Promise<unknown>;
 act(()=>{old=result.current.refresh(node);newer=result.current.refresh(node);});
 await act(async()=>{replies[1]({node:{...node,status:'COMPLETED',completion_status:'success'},observed_at:'new'});await newer;});
 await act(async()=>{replies[0]({node,observed_at:'old'});await old;});
 expect(result.current.overlay(node).status).toBe('COMPLETED');
 act(()=>{old=result.current.refresh(node);});
 rerender({tenant:'other'});
 await act(async()=>{replies[2]({node,observed_at:'late'});await old;});
 expect(result.current.state('child')).toBeUndefined();
 act(()=>{old=result.current.refresh({...node,tenant_id:'other'});});
 unmount();await act(async()=>{replies[3]({node,observed_at:'late'});await old;});
});
