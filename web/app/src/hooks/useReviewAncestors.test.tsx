import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import * as shared from '@colony2/shared';
import { useReviewAncestors } from './useReviewAncestors';
vi.mock('@colony2/shared',async()=>({...await vi.importActual<typeof shared>('@colony2/shared'),getTreeNode:vi.fn()}));
const node=(id:string,parent?:string)=>({node:{tenant_id:'t',job_id:id,parent_job_id:parent,job_type:'build',status:'ACTIVE',recipe_parent_job_id:'attribution',goal_id:'goal'},observed_at:'now'});
beforeEach(()=>{vi.mocked(shared.getTreeNode).mockImplementation(async(_t,id)=>node(id,id==='leaf'?'root':undefined));});
afterEach(()=>{vi.clearAllMocks();vi.useRealTimers();});
it('C03 follows only runtime parents and returns oldest first, including archived records',async()=>{
 vi.mocked(shared.getTreeNode).mockImplementation(async(_t,id)=>({...node(id,id==='leaf'?'root':undefined),node:{...node(id,id==='leaf'?'root':undefined).node,store:'ARCHIVED'}}));
 const {result}=renderHook(()=>useReviewAncestors('t','leaf'));
 await waitFor(()=>expect(result.current.loading).toBe(false));
 expect(result.current.nodes.map(n=>n.job_id)).toEqual(['root','leaf']);
 expect(vi.mocked(shared.getTreeNode).mock.calls.map(c=>c[1])).toEqual(['leaf','root']);
});
it.each(['cycle','identity','failure','bound'])('C05 retains verified nodes when ascent stops for %s',async reason=>{
 vi.mocked(shared.getTreeNode).mockImplementation(async(_t,id)=>{
  if(reason==='failure'&&id==='parent')throw new Error('Forbidden');
  if(reason==='identity'&&id==='parent')return {...node('wrong'),node:{...node('wrong').node,tenant_id:'other'}};
  if(reason==='bound')return node(id,String(Number(id)+1));
  return node(id,reason==='cycle'?'leaf':'parent');
 });
 const {result}=renderHook(()=>useReviewAncestors('t',reason==='bound'?'0':'leaf'));
 await waitFor(()=>expect(result.current.loading).toBe(false));
 expect(result.current.error).toBeTruthy();expect(result.current.nodes).toHaveLength(reason==='bound'?65:1);
 expect(shared.getTreeNode).toHaveBeenCalledTimes(reason==='bound'?65:reason==='cycle'?1:2);
});
it('C05 expires a hung ascent after thirty seconds and ignores its late completion',async()=>{
 vi.useFakeTimers();let resolve!:(v:ReturnType<typeof node>)=>void;
 vi.mocked(shared.getTreeNode).mockImplementationOnce(()=>new Promise(r=>{resolve=r;}));
 const {result}=renderHook(()=>useReviewAncestors('t','leaf'));
 await act(()=>vi.advanceTimersByTimeAsync(30_000));
 expect(result.current.error).toContain('timed out');
 expect(vi.mocked(shared.getTreeNode).mock.calls[0][2]?.aborted).toBe(true);
 await act(async()=>resolve(node('leaf','root')));
 expect(result.current.nodes).toEqual([]);expect(shared.getTreeNode).toHaveBeenCalledTimes(1);
});
it('C06 retry, navigation and unmount abort obsolete reads',async()=>{
 let resolve!:(v:ReturnType<typeof node>)=>void;
 vi.mocked(shared.getTreeNode).mockImplementationOnce(()=>new Promise(r=>{resolve=r;}));
 const {result,rerender,unmount}=renderHook(({id})=>useReviewAncestors('t',id),{initialProps:{id:'leaf'}});
 const first=vi.mocked(shared.getTreeNode).mock.calls[0][2]!;
 act(()=>result.current.retry());expect(first.aborted).toBe(true);
 await waitFor(()=>expect(result.current.loading).toBe(false));
 await act(async()=>resolve(node('leaf','wrong')));expect(result.current.nodes.map(n=>n.job_id)).toEqual(['root','leaf']);
 rerender({id:'other'});await waitFor(()=>expect(result.current.nodes[0]?.job_id).toBe('other'));
 const last=vi.mocked(shared.getTreeNode).mock.calls.slice(-1)[0][2]!;unmount();expect(last.aborted).toBe(true);
});
