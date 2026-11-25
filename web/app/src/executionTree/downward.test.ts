import { expect, it, vi } from 'vitest';
import type { TreeNode, TreeWaitsResponse } from '@colony2/shared';
import { Traversal, DEFAULT_BUDGETS, type TreeAPI } from './traversal';
const at='2026-11-17T00:00:00Z';
const node=(id:string,parent?:string):TreeNode=>({tenant_id:'t',job_id:id,parent_job_id:parent,job_type:'recipe',status:'READY',title:id});
const dep=(id:string,kind:'job'|'goal'='job')=>({target:kind==='job'?{jobId:id}:{goalId:id},condition:'complete',state:'PENDING'});
const waits=(id:string,dependencies:any[]=[],next_page_token=''):TreeWaitsResponse=>({consumer:{tenant_id:'t',job_id:id},waits:dependencies.length?[{jobKey:{tenantId:'t',jobId:id},waitId:'w',state:'PENDING',dependencies}]:[],issues:[],observed_at:at,next_page_token,consumer_snapshot:{state:'available',observed_at:at,status:'READY'}});
function fixture(records:TreeNode[],links:Record<string,string[]>={}){
 const source:Required<TreeAPI>={
  node:vi.fn(async(_,id)=>{const n=records.find(n=>n.job_id===id);if(!n)throw Error('missing');return {node:n,observed_at:at};}),
  children:vi.fn(async(_,id)=>({nodes:records.filter(n=>n.parent_job_id===id),observed_at:at})),
  waits:vi.fn(async(_,id)=>waits(id,(links[id]||[]).map(id=>dep(id)))),
 };
 return source;
}
const create=(source:TreeAPI,limits={})=>new Traversal('t','entry',()=>{},source,{...DEFAULT_BUDGETS,...limits});
it.each(['child','wait','summary'] as const)('DW27,31 ignores a delayed %s response after refresh prunes its source',async kind=>{
 const source=fixture([node('entry'),node('a')],{entry:['a']}),t=create(source);await t.run();
 let started=false,release!:()=>void,remove!:()=>void;
 const delayed=new Promise<void>(resolve=>release=resolve),removal=new Promise<void>(resolve=>remove=resolve);
 vi.mocked(source.node).mockImplementation(async(_,id)=>{if(kind==='summary'&&id==='a'){started=true;await delayed;}return {node:node(id),observed_at:at};});
 vi.mocked(source.children).mockImplementation(async(_,id)=>{
  if(kind==='child'&&id==='a'){started=true;await delayed;return {nodes:[node('orphan','a')],next_page_token:'unwanted-next-page',observed_at:at};}
  return {nodes:[],observed_at:at};
 });
 vi.mocked(source.waits).mockImplementation(async(_,id,token)=>{
  if(id==='entry'){
   // Keep the old path until the wait stream and final replacement page can race.
   if(kind==='wait'&&!token)return waits(id,[],'finish');
   await removal;return waits(id);
  }
  if(kind==='wait'&&id==='a'){started=true;await delayed;return waits(id,[dep('orphan')],'unwanted-next-page');}
  return waits(id);
 });
 t.refresh();await vi.waitFor(()=>expect(started).toBe(true));remove();
 await vi.waitFor(()=>expect(t.state.nodes.has('a')).toBe(false));release();
 await vi.waitFor(()=>expect(t.state.loading).toBe(false));
 expect([...t.state.nodes.keys()]).toEqual(['entry']);expect(t.state.graph.nodes.size).toBe(1);expect(t.state.graph.edges.size).toBe(0);
 for(const streams of [t.state.branches,t.state.waits,t.state.summaries])expect(streams.has('orphan')).toBe(false);
 expect(t.canResume).toBe(false);expect(t.state.stop).toBeUndefined();
 expect(vi.mocked(source.children).mock.calls.some(c=>c[2]==='unwanted-next-page')).toBe(false);
 expect(vi.mocked(source.waits).mock.calls.some(c=>c[2]==='unwanted-next-page')).toBe(false);
});
it('DW27,31 rejects the replaced stream when another wait rediscovers its pruned source',async()=>{
 const source=fixture([node('entry'),node('b','entry'),node('a','b')]),t=create(source);await t.run();
 let started=false,reads=0,release!:()=>void,detach!:()=>void,restore!:()=>void;
 const delayed=new Promise<void>(r=>release=r),detached=new Promise<void>(r=>detach=r),restored=new Promise<void>(r=>restore=r);
 vi.mocked(source.children).mockImplementation(async(_,id)=>{
  if(id==='b'){await detached;return {nodes:[],observed_at:at};}
  if(id==='a'&&++reads===1){started=true;await delayed;return {nodes:[node('orphan','a')],next_page_token:'obsolete-cursor',observed_at:at};}
  return {nodes:id==='entry'?[node('b','entry')]:[],observed_at:at};
 });
 vi.mocked(source.waits).mockImplementation(async(_,id)=>{if(id==='b'){await restored;return waits(id,[dep('a')]);}return waits(id);});
 t.refresh();await vi.waitFor(()=>expect(started).toBe(true));detach();
 await vi.waitFor(()=>expect(t.state.nodes.has('a')).toBe(false));restore();
 await vi.waitFor(()=>expect(t.state.graph.nodes.has(t.state.graph.key('a'))).toBe(true));release();
 await vi.waitFor(()=>expect(t.state.loading).toBe(false));
 expect([...t.state.nodes.keys()].sort()).toEqual(['a','b','entry']);expect(reads).toBe(2);
 expect(t.state.branches.get('a')?.state).toBe('exhausted');expect(t.state.graph.edges.size).toBe(2);expect(t.canResume).toBe(false);
});
it('DW01–04 follows mixed branches, deduplicates shared jobs, and never ascends external parents',async()=>{
 const source=fixture([node('entry','root'),node('root'),node('sibling','root'),node('a','outside'),node('b'),node('child','b'),node('leaf'),node('outside'),node('unrelated','outside')],{entry:['a','b'],a:['b'],child:['leaf'],sibling:['unrelated']});
 const t=create(source);await t.run();
 expect([...t.state.nodes.keys()].sort()).toEqual(['a','b','child','entry','leaf','root','sibling']);
 expect(vi.mocked(source.node).mock.calls.map(c=>c[1]).sort()).toEqual(['a','b','entry','leaf','root']);
 expect(vi.mocked(source.waits).mock.calls.map(c=>c[1]).sort()).toEqual(['a','b','child','entry','leaf']);
 expect(vi.mocked(source.children).mock.calls.filter(c=>c[1]==='b')).toHaveLength(1);
 const reads=vi.mocked(source.waits).mock.calls.length;t.inspect('a');await t.run();expect(source.waits).toHaveBeenCalledTimes(reads);
});
it('DW05–07 follows empty continuations fairly, retains failed cursors, and retries only affected streams',async()=>{
 const source=fixture([node('entry'),node('a')],{entry:['a']});let fail=true;
 vi.mocked(source.waits).mockImplementation(async(_,id,token)=>{if(id!=='entry')return waits(id);if(!token)return waits(id,[dep('a')],'next');if(fail)throw Error();return waits(id,[]);});
 const t=create(source);await t.run();expect(t.state.waits.get('entry')).toMatchObject({token:'next',state:'failed'});
 expect(t.state.waits.get('a')?.state).toBe('exhausted');
 const children=vi.mocked(source.children).mock.calls.length;fail=false;await t.retryWaits('entry');
 expect(t.state.waits.get('entry')?.state).toBe('exhausted');expect(source.children).toHaveBeenCalledTimes(children);
 const repeated=fixture([node('entry')]);vi.mocked(repeated.waits).mockImplementation(async(_,id)=>waits(id,[],'same'));
 const r=create(repeated);await r.run();expect(r.state.waits.get('entry')).toMatchObject({state:'malformed',reason:'Repeated cursor; incomplete branch'});expect(repeated.waits).toHaveBeenCalledTimes(2);
});
it('DW08–10 omits goal targets, traverses legacy jobs and rejects foreign consumers and summaries',async()=>{
 const source=fixture([node('entry'),node('legacy'),node('goal-attempt')]);
 vi.mocked(source.waits).mockImplementation(async(_,id)=>{const p=waits(id,id==='entry'?[dep('literal','goal')]:[]);if(id==='entry')p.consumer_snapshot!.legacy_wait_for=['legacy'];return p;});
 const t=create(source);await t.run();expect(t.state.nodes.has('legacy')).toBe(true);expect(t.state.nodes.has('goal-attempt')).toBe(false);
 expect([...t.state.graph.nodes.values()].map(node=>node.id)).not.toContain('literal');
 vi.mocked(source.waits).mockImplementation(async()=>waits('foreign',[dep('secret')]));t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));
 expect(t.state.nodes.has('secret')).toBe(false);expect(t.state.waits.get('entry')?.observation.stale).toBe(true);
 const wrong=fixture([node('entry')],{entry:['wanted']});vi.mocked(wrong.node).mockImplementation(async(_,id)=>({node:node(id==='entry'?id:'neighbor'),observed_at:at}));const w=create(wrong);await w.run();expect(w.state.nodes.has('neighbor')).toBe(false);expect(w.state.summaries.get('wanted')?.state).toBe('failed');
});
it.each([2,3,4])('DW11 shared node budget %i includes missing jobs and skips goals',async nodes=>{
 const source=fixture([node('entry')]);vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,[dep('missing'),dep('goal','goal'),dep('extra'),dep('last')]));
 const t=create(source,{nodes});await t.run();expect(t.state.graph.nodes.size).toBe(nodes);expect(t.state.waits.get('entry')?.state).toBe(nodes===4?'exhausted':'limited');
 if(nodes<4)expect(t.state.waits.get('entry')?.token).toBe('');
});
it.each([1,2,3])('DW12 grouped edge budget %i does not overrun',async edges=>{
 const source=fixture([node('entry'),node('a'),node('b'),node('c')],{entry:['a','b','c']});const t=create(source,{edges});await t.run();expect(t.state.graph.edges.size).toBe(edges);expect(t.state.waits.get('entry')?.state).toBe(edges<3?'limited':'exhausted');
});
it.each([1,2,3])('DW13 counts shared child/wait page attempts, including failures, at %i',async pages=>{
 const source=fixture([node('entry')]);vi.mocked(source.children).mockRejectedValue(Error());vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,[],'next'));
 const t=create(source,{pages});await t.run();await t.run('entry');expect(vi.mocked(source.children).mock.calls.length+vi.mocked(source.waits).mock.calls.length).toBe(pages);
});
it('DW14,16 mixed depth never resets at waits; a shorter path reopens the limited node',async()=>{
 const source=fixture([node('entry'),node('a'),node('b','a'),node('c'),node('leaf','c')],{entry:['a'],b:['c']});const t=create(source,{depth:3});await t.run();expect(t.state.nodes.has('c')).toBe(true);expect(t.state.nodes.has('leaf')).toBe(false);expect(t.state.branches.get('c')?.reason).toBe('Depth limit');
 vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,id==='entry'?[dep('a'),dep('c')]:id==='b'?[dep('c')]:[]));t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.nodes.has('leaf')).toBe(true);expect(t.state.stop).not.toBe('Depth limit');
});
it.each([1,2,3])('DW17 per-consumer page budget %i leaves continuation explicit',async waitPages=>{
 const source=fixture([node('entry')]);vi.mocked(source.waits).mockImplementation(async(_,id,token)=>waits(id,[],String(Number(token||0)+1)));
 const t=create(source,{waitPages});await t.run();expect(source.waits).toHaveBeenCalledTimes(waitPages);expect(t.state.waits.get('entry')?.observation.stopped).toBe('Wait page limit reached');
});
it.each([2,3,4])('DW18,19 counts malformed/resolved/legacy dependency entries within %i',async entries=>{
 const source=fixture([node('entry')]);vi.mocked(source.waits).mockImplementation(async(_,id)=>{const p=waits(id,[{...dep('resolved'),state:'SATISFIED'},null,dep('missing')]);p.consumer_snapshot!.legacy_wait_for=['legacy'];return p;});
 const t=create(source,{waitEntries:entries,entries});await t.run();expect(t.state.waits.get('entry')?.observation.inspected).toBe(entries);expect(t.state.waits.get('entry')?.observation.rows.length).toBeLessThanOrEqual(entries);expect(t.state.nodes.has('resolved')).toBe(false);
});
it('DW19,22 shared reservations bound concurrent wait inspection without permanently starving a stream',async()=>{
 const source=fixture([node('entry'),node('a','entry'),node('b','entry')]);vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,[{...dep('resolved'),state:'SATISFIED'}]));
 const t=create(source,{entries:3});await t.run();expect([...t.state.waits.values()].reduce((sum,b)=>sum+b.observation.inspected,0)).toBe(3);expect(source.waits).toHaveBeenCalledTimes(3);
});
it.each([1,2,3])('DW20,24 caps all request types at %i per run and resumes saved cursors',async requests=>{
 const source=fixture([node('entry'),node('a','entry')],{entry:['missing']});const t=create(source,{requests});
 let previous=0;
 for(let i=0;i<10&&t.canResume;i++){await t.run();const total=vi.mocked(source.node).mock.calls.length+vi.mocked(source.children).mock.calls.length+vi.mocked(source.waits).mock.calls.length;expect(total-previous).toBeLessThanOrEqual(requests);previous=total;}
 expect(t.state.nodes.has('a')).toBe(true);expect(t.state.branches.get('entry')?.state).toBe('exhausted');
});
it('DW21 releases ignored-abort hung reads at the request deadline and allows independent branches',async()=>{
 vi.useFakeTimers();try{
 const source=fixture([node('entry'),node('a','entry')]);vi.mocked(source.waits).mockImplementation((_,id)=>id==='entry'?new Promise(()=>{}):Promise.resolve(waits(id)));
 const t=create(source,{requestMilliseconds:10});const work=t.run();await vi.advanceTimersByTimeAsync(11);await work;
 expect(t.state.waits.get('entry')?.state).toBe('failed');expect(t.state.branches.get('a')?.state).toBe('exhausted');
 }finally{vi.useRealTimers();}
});
it('DW22,23 bounds global concurrency and schedules newly reached branches before draining another consumer',async()=>{
 const source=fixture([node('entry'),...Array.from({length:8},(_,i)=>node('child'+i,'entry'))],{entry:['a']});let active=0,max=0;const reads:string[]=[];
 vi.mocked(source.waits).mockImplementation(async(_,id,token)=>{active++;max=Math.max(max,active);reads.push(id+':'+token);await Promise.resolve();active--;return waits(id,[],id==='entry'?String(Number(token||0)+1):'');});
 const t=create(source);await t.run();expect(max).toBeLessThanOrEqual(4);expect(reads.findIndex(id=>id.startsWith('child'))).toBeLessThan(reads.lastIndexOf('entry:9'));
});
it('DW25,31, DG05 refresh retains failed previous branches, then prunes disconnected cycles but preserves fan-in',async()=>{
 const source=fixture([node('entry'),node('a'),node('b'),node('shared'),node('other','entry')],{entry:['a','shared'],a:['b'],b:['a'],other:['shared']});const t=create(source);await t.run();expect(t.state.nodes.has('b')).toBe(true);
 vi.mocked(source.waits).mockImplementation(async(_,id)=>{if(id==='entry')throw Error();return waits(id);});t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.nodes.has('b')).toBe(true);expect(t.state.waits.get('entry')?.observation.stale).toBe(true);
 vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,id==='other'?[dep('shared')]:[]));await t.retryWaits('entry');
 expect(t.state.nodes.has('a')).toBe(false);expect(t.state.nodes.has('b')).toBe(false);expect(t.state.nodes.has('shared')).toBe(true);expect(t.state.waits.get('entry')?.observation.stale).toBe(false);
});
it('DW26–28 pause, refresh and disposal reject late replies while preserving exact requested entry',async()=>{
 let release!:(p:TreeWaitsResponse)=>void;const source=fixture([node('entry'),node('obsolete')]);vi.mocked(source.waits).mockImplementationOnce(()=>new Promise(r=>release=r));const t=create(source);const old=t.run();await vi.waitFor(()=>expect(release).toBeDefined());t.pause();const current=t.run();await Promise.all([old,current]);release(waits('entry',[dep('obsolete')]));await Promise.resolve();expect(t.state.nodes.has('obsolete')).toBe(false);expect(t.state.loading).toBe(false);
 const absent=create(fixture([]));await absent.run();expect(absent.entry).toBe('entry');expect(absent.state.error).toMatch('Entry job unavailable');expect(absent.state.nodes.size).toBe(0);
});
it('DW29,30 missing targets and unavailable supplementary data remain boundaries while other work continues',async()=>{
 const source=fixture([node('entry'),node('good')],{entry:['missing','good']});vi.mocked(source.waits).mockImplementation(async(_,id)=>{const p=waits(id,id==='entry'?[dep('missing'),dep('good')]:[]);if(id==='good')p.consumer_snapshot={state:'unavailable',observed_at:at};return p;});const t=create(source);await t.run();expect(t.state.nodes.has('good')).toBe(true);expect(t.state.summaries.get('missing')?.state).toBe('failed');expect(t.state.waits.get('good')?.observation.issues).toContain('Supplementary job information unavailable');
});
it('DW25 refresh resets hard observation counters, removes unsupported context siblings, and counts retained storage',async()=>{
 const records=[node('entry','root'),node('root'),node('sibling','root')],source=fixture(records);
 const t=create(source,{pages:5});await t.run();expect(t.state.nodes.has('sibling')).toBe(true);
 vi.mocked(source.children).mockImplementation(async(_,id)=>({nodes:id==='root'?[records[0]]:[],observed_at:at}));
 t.refresh();await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.nodes.has('sibling')).toBe(false);expect(t.state.branches.get('root')?.state).toBe('exhausted');
});
it('DW27 tree refresh and wait refresh reject late dependency completions, including ignored abort',async()=>{
 const source=fixture([node('entry'),node('obsolete')]);let release!:(p:TreeWaitsResponse)=>void;
 vi.mocked(source.waits).mockImplementationOnce(()=>new Promise(r=>release=r));const t=create(source);const old=t.run();await vi.waitFor(()=>expect(release).toBeDefined());
 t.refresh();await old;await vi.waitFor(()=>expect(t.state.loading).toBe(false));release(waits('entry',[dep('obsolete')]));await Promise.resolve();expect(t.state.nodes.has('obsolete')).toBe(false);
});
it('DW31 incomplete multi-page replacement keeps all old metadata until final valid exhaustion',async()=>{
 const source=fixture([node('entry'),node('old'),node('new')],{entry:['old']}),t=create(source);await t.run();let fail=true;
 vi.mocked(source.waits).mockImplementation(async(_,id,token)=>{if(id!=='entry')return waits(id);if(!token)return waits(id,[dep('new')],'next');if(fail)throw Error();return waits(id);});
 t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.waits.get('entry')?.observation.rows.map(r=>r.id).sort()).toEqual(['new','old']);expect(t.state.nodes.has('old')).toBe(true);
 fail=false;await t.retryWaits('entry');expect(t.state.nodes.has('old')).toBe(false);expect(t.state.nodes.has('new')).toBe(true);
});
it('DW18,25 refresh bounds previous plus new retained row versions',async()=>{
 const source=fixture([node('entry'),node('old'),node('new')],{entry:['old']}),t=create(source,{waitEntries:1});await t.run();
 vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,id==='entry'?[dep('new')]:[]));t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.waits.get('entry')!.observation.rows.length).toBeLessThanOrEqual(1);expect(t.state.nodes.has('old')).toBe(true);expect(t.state.waits.get('entry')!.observation.stale).toBe(true);
});
it('DW30,31 a failed summary refresh retains readable fields with previous coverage and exact retry',async()=>{
 const source=fixture([node('entry'),node('target')],{entry:['target']}),t=create(source);await t.run();
 vi.mocked(source.node).mockImplementation(async(_,id)=>{if(id==='target')throw Error();return {node:node(id),observed_at:at};});t.refresh();await vi.waitFor(()=>expect(t.state.loading).toBe(false));
 expect(t.state.nodes.get('target')?.title).toBe('target');expect(t.state.summaries.get('target')?.state).toBe('failed');expect(t.state.previous).toBe(true);
 vi.mocked(source.node).mockImplementation(async(_,id)=>({node:{...node(id),title:'Updated target'},observed_at:at}));await t.retrySummary('target');expect(t.state.nodes.get('target')?.title).toBe('Updated target');expect(t.state.summaries.has('target')).toBe(false);
});
it('DW31 a failed ancestor refresh retains the previously readable graph without dangling relationships',async()=>{
 const source=fixture([node('entry','root'),node('root')]),t=create(source);await t.run();
 vi.mocked(source.node).mockImplementation(async(_,id)=>{if(id==='root')throw Error();return {node:node('entry','root'),observed_at:at};});t.refresh();await vi.waitFor(()=>expect(t.state.loading).toBe(false));
 expect(t.state.nodes.has('root')).toBe(true);expect(t.state.graph.nodes.has(t.state.graph.key('root'))).toBe(true);expect(t.state.previous).toBe(true);
 for(const e of t.state.graph.edges.values()){expect(t.state.graph.nodes.has(e.source)).toBe(true);expect(t.state.graph.nodes.has(e.target)).toBe(true);}
});
it('DW04,31 a pruned target can be hydrated again when a later verified observation restores its path',async()=>{
 const links={entry:['target']},source=fixture([node('entry'),node('target')],links),t=create(source);await t.run();links.entry=[];t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.nodes.has('target')).toBe(false);
 links.entry=['target'];t.refreshWaits('entry');await vi.waitFor(()=>expect(t.state.loading).toBe(false));expect(t.state.nodes.has('target')).toBe(true);expect(vi.mocked(source.node).mock.calls.filter(c=>c[1]==='target')).toHaveLength(2);
});
it.each(['nodes','edges','waitEntries'])('DW11,12,18 capped %s keeps the partially inspected wait page cursor',async limit=>{
 const source=fixture([node('entry'),node('a'),node('b')]);vi.mocked(source.waits).mockImplementation(async(_,id)=>waits(id,id==='entry'?[dep('a'),dep('b')]:[],id==='entry'?'next':''));
 const t=create(source,{[limit]:limit==='nodes'?2:1});await t.run();expect(t.state.waits.get('entry')).toMatchObject({state:'limited',token:''});expect(t.state.waits.get('entry')?.observation.token).toBe('');
});
