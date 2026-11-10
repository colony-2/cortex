import { describe, it, expect, vi } from 'vitest';
import { Traversal, DEFAULT_BUDGETS, type TreeAPI } from './traversal';
import type { TreeNode } from '@colony2/shared';
const n = (job_id: string, parent_job_id?: string): TreeNode => ({tenant_id:'t',job_id,parent_job_id,job_type:'generic',status:'READY'});
const observed_at = '2026-11-09T00:00:00Z';
const page = (nodes: TreeNode[] = [], next_page_token?: string) => ({nodes,next_page_token,observed_at});
function fixture(records: TreeNode[], children?: TreeAPI['children']) {
  const source: TreeAPI = {
    node: vi.fn(async (_,id) => {const node=records.find(n=>n.job_id===id);if(!node) throw Error(); return {node,observed_at};}),
    children: vi.fn(children || (async (_,id)=>page(records.filter(n=>n.parent_job_id===id)))),
  };
  return source;
}
const create = (source: TreeAPI, entry='entry', limits={}, now?:()=>number) => new Traversal('t',entry,()=>{},source,{...DEFAULT_BUDGETS,...limits},now);
describe('recorded submission tree',()=>{
  it('TREE-001,002,023 traverses from grandchild through generic ancestors without goal edges',async()=>{
    const source=fixture([n('root'),n('middle','root'),n('entry','middle'),n('sibling','root'),{...n('unrelated'),goal_id:'same'}]);
    const t=create(source);await t.run();
    expect([...t.state.nodes.keys()].sort()).toEqual(['entry','middle','root','sibling']);
    expect(t.state.nodes.get('entry')?.parent_job_id).toBe('middle');
  });
  it('TREE-005 follows empty pages and stops repeated tokens',async()=>{
    const source=fixture([n('entry')],async(_,id,token)=> id !== 'entry' ? page() : token ? page([n('child','entry')],'a') : page([],'a'));
    const t=create(source);await t.run();expect(t.state.nodes.has('child')).toBe(true);
    expect(t.state.branches.get('entry')).toMatchObject({state:'malformed',reason:'Repeated cursor; incomplete branch'});
    expect(source.children).toHaveBeenCalledTimes(3);
  });
  it('TREE-006,027 rejects invalid tenants, mismatched parents, conflicts and cycles',async()=>{
    const source=fixture([n('entry','root'),n('root')],async(_,id)=>id==='root'?page([n('entry','root'),{...n('foreign','root'),tenant_id:'x'},n('wrong','elsewhere'),n('', 'root')]):page([n('root','entry')]));
    const t=create(source);await t.run();expect(t.state.nodes.size).toBe(2);expect(t.state.warnings.length).toBeGreaterThan(0);expect(t.state.blocked.has('root')).toBe(true);
  });
  it('TREE-007 missing ancestor preserves known entry and its descendants',async()=>{
    const source=fixture([n('entry','missing'),n('child','entry')]);const t=create(source);await t.run();
    expect(t.state.boundary).toContain('Ancestor unavailable');expect(t.state.nodes.has('child')).toBe(true);
    const singleton=create(fixture([n('entry')]));await singleton.run();expect(singleton.state.boundary).toBeUndefined();expect(singleton.state.nodes.get('entry')?.parent_job_id).toBeUndefined();
  });
  it('TREE-009 retries only failed cursor; successful unrelated branches stay loaded',async()=>{
    let fail=true;const calls:string[]=[];
    const source=fixture([n('entry')],async(_,id,token)=>{calls.push(id+':'+token);if(id==='entry'&&!token)return page([n('child','entry')],'next');if(id==='entry'&&fail)throw Error();return page();});
    const t=create(source);await t.run();expect(t.state.branches.get('entry')?.token).toBe('next');expect(t.state.nodes.has('child')).toBe(true);fail=false;
    const before=calls.length;await t.run('entry');expect(calls.slice(before)).toEqual(['entry:next']);expect(t.state.branches.get('entry')?.state).toBe('exhausted');
  });
  it('TREE-010 node cap retains page cursor and entry component',async()=>{
    const t=create(fixture([n('entry'),n('a','entry'),n('b','entry')]),'entry',{nodes:2});await t.run();
    expect(t.state.nodes.size).toBe(2);expect(t.state.nodes.has('entry')).toBe(true);expect(t.state.branches.get('entry')).toMatchObject({token:'',state:'limited'});
  });
  it.each([{ascent:1},{depth:1},{pages:1},{requests:1}])('TREE-010,011 stops bounded work %j',async limits=>{
    const t=create(fixture([n('entry','mid'),n('mid','root'),n('root'),n('leaf','entry')]),'entry',limits);await t.run();
    expect(t.state.nodes.has('entry')).toBe(true);expect(t.state.boundary || t.state.stop || [...t.state.branches.values()].some(b=>b.state==='limited')).toBeTruthy();
  });
  it('TREE-010 elapsed limit cancels a hung call',async()=>{
    vi.useFakeTimers();const source=fixture([n('entry')]);source.children=vi.fn(()=>new Promise(()=>{}));
    const t=create(source,'entry',{milliseconds:10});const work=t.run();await vi.advanceTimersByTimeAsync(11);await work;
    expect(t.state.stop).toBe('Elapsed-time limit');expect(t.state.nodes.has('entry')).toBe(true);vi.useRealTimers();
  });
  it('TREE-011 resume resets operation request budget but preserves page cap',async()=>{
    const source=fixture([n('entry'),n('a','entry'),n('b','a')]);const t=create(source,'entry',{requests:2,pages:2});await t.run();
    expect(t.state.stop).toBe('Request limit');await t.run();expect(t.state.nodes.has('b')).toBe(true);expect(t.state.stop).toBe('Page limit');const calls=vi.mocked(source.children).mock.calls.length;await t.run();expect(vi.mocked(source.children).mock.calls.length).toBe(calls);
  });
  it('TREE-019 canceled generation ignores a late response',async()=>{
    let resolve!:(v:any)=>void;const source=fixture([]);source.node=()=>new Promise(r=>resolve=r);const t=create(source);const work=t.run();t.cancel();resolve({node:n('entry'),observed_at});await work;expect(t.state.nodes.size).toBe(0);
  });
  it('TREE-027 later status observations do not conflict with stable parents',async()=>{
    const source=fixture([n('entry','root'),n('root')],async(_,id)=>page(id==='root'?[{...n('entry','root'),status:'COMPLETED'}]:[]));const t=create(source);await t.run();expect(t.state.nodes.get('entry')?.status).toBe('COMPLETED');expect(t.state.blocked.size).toBe(0);
  });
  it('production defaults stay bounded',()=>expect(DEFAULT_BUDGETS).toEqual({nodes:1000,ascent:64,depth:64,pages:200,requests:300,milliseconds:30000,concurrency:4}));
});

it('TREE-011 bounds concurrency during descent and retries',async()=>{
 let active=0,max=0;
 const source=fixture([n('entry')],async(_,id)=>{
  active++;max=Math.max(active,max);await Promise.resolve();active--;
  return page(id==='entry'?Array.from({length:12},(_,i)=>n(String(i),'entry')):[]);
 });
 const t=create(source);await t.run();expect(t.state.nodes.size).toBe(13);expect(max).toBeLessThanOrEqual(DEFAULT_BUDGETS.concurrency);expect(active).toBe(0);
});
it('TREE-019 route cancellation aborts descendant work and suppresses stale observations',async()=>{
 let resolve!:(v:any)=>void;let signal!:AbortSignal;
 let started!:()=>void;const ready=new Promise<void>(r=>started=r);
 const source=fixture([n('entry')]);source.children=(_,__,___,s)=>{signal=s;started();return new Promise(r=>resolve=r);};
 const t=create(source);const work=t.run();await ready;t.cancel();
 expect(signal.aborted).toBe(true);resolve(page([n('late','entry')]));await work;expect(t.state.nodes.has('late')).toBe(false);
});

it('TREE-006 stops a newly observed ancestor cycle without a bridge edge',async()=>{
 const t=create(fixture([n('entry','root'),n('root','entry')]));await t.run();
 expect([...t.state.nodes.keys()]).toEqual(['entry']);expect(t.state.warnings.some(w=>w.startsWith('Cycle boundary'))).toBe(true);
 expect(t.state.boundary).toContain('Malformed ancestor');
});
it('TREE-003,023 metadata-only attribution, waits and history do not create parents',async()=>{
 const node=Object.assign(n('entry'),{recipe_parent_job_id:'unrelated',goal_id:'shared',wait_for:['unrelated'],history:{prior_job_id:'unrelated'}});
 const t=create(fixture([node,{...n('unrelated'),goal_id:'shared'}]));await t.run();
 expect(t.state.nodes.size).toBe(1);expect(t.state.nodes.get('entry')?.parent_job_id).toBeUndefined();expect(t.state.warnings).toContain('Recipe attribution differs from runtime parent: entry');
});
it('TREE-010 retains ancestry preferentially when ascent reaches the node cap',async()=>{
 const t=create(fixture([n('entry','middle'),n('middle','root'),n('root')]),'entry',{nodes:2});await t.run();
 expect([...t.state.nodes.keys()]).toEqual(['entry','middle']);expect(t.state.boundary).toBe('Node limit');expect(t.canResume).toBe(false);
});

it('DAG-06 paused entry can resume and ignores late transport and old finalizers', async () => {
  let finish!: (value: any) => void;
  let oldSignal!: AbortSignal;
  const source = fixture([n('entry')]);
  vi.mocked(source.node).mockImplementationOnce((_, __, signal) => { oldSignal = signal; return new Promise(r => finish = r); });
  const t = create(source);
  const old = t.run(); t.pause();
  expect(oldSignal.aborted).toBe(true);
  expect(t.canResume).toBe(true);
  expect(t.state.error).toBeUndefined();
  const next = t.run(); await old; await next;
  finish({node:n('obsolete'), observed_at}); await Promise.resolve();
  expect([...t.state.nodes.keys()]).toEqual(['entry']);
  expect(t.state.loading).toBe(false);
  expect(t.state.error).toBeUndefined();
});
it('DAG-06 interrupted child cursor resumes without stale nodes or overlapping requests', async () => {
  let finish!: (value: any) => void;
  let ready!: () => void; const started = new Promise<void>(r => ready = r);
  const source = fixture([n('entry')]);
  vi.mocked(source.children).mockImplementationOnce(async () => page([], 'next')).mockImplementationOnce(() => {ready(); return new Promise(r => finish = r);});
  const t = create(source); const old = t.run(); await started; t.pause();
  expect(t.state.branches.get('entry')).toMatchObject({state:'more', token:'next'});
  const next = t.run(); const duplicate = t.run(); await Promise.all([old,next,duplicate]);
  finish(page([n('stale','entry')])); await Promise.resolve();
  expect(t.state.nodes.has('stale')).toBe(false);
  expect(source.children).toHaveBeenCalledTimes(3);
  expect(t.state.branches.get('entry')?.state).toBe('exhausted');
});
it('DAG-06 old finalizers cannot mark a new pending run idle or accept obsolete ancestry', async () => {
 let first!: (value:any)=>void, second!: (value:any)=>void;
 const source=fixture([n('entry')]);
 vi.mocked(source.node).mockImplementationOnce(()=>new Promise(r=>first=r)).mockImplementationOnce(()=>new Promise(r=>second=r));
 const t=create(source); const old=t.run(); t.pause(); const current=t.run(); await old;
 expect(t.state.loading).toBe(true);
 first({node:n('wrong','obsolete-parent'),observed_at}); await Promise.resolve();
 expect(t.state.nodes.size).toBe(0);
 expect(t.state.loading).toBe(true);
 second({node:n('entry'),observed_at}); await current;
 expect(t.state.nodes.size).toBe(1);
 expect(t.state.loading).toBe(false);
});
