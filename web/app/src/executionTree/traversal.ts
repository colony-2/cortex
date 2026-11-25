import { getTreeChildren, getTreeNode, getTreeWaits, type TreeNode, type TreeNodeResponse, type TreeChildrenResponse, type TreeWaitsResponse } from '@colony2/shared';
import { JobGraph, edgeKey } from './graph';
import { legacyRows, normalizeWaits, waitTargets, type WaitObservation, type WaitController, type WaitRow } from './waits';

export const DEFAULT_BUDGETS = { nodes:1000, edges:4000, ascent:64, depth:64, pages:200, entries:10000, waitPages:10, waitEntries:1000, requests:300, milliseconds:30000, requestMilliseconds:10000, concurrency:4 };
export type Budgets = typeof DEFAULT_BUDGETS;
export interface Branch { state: 'pending' | 'loading' | 'more' | 'exhausted' | 'failed' | 'limited' | 'malformed'; token: string; seen: Set<string>; depth: number; reason?: string; observed?: string }
interface WaitBranch extends Branch { observation: WaitObservation; staged: WaitRow[]; legacyAdded: boolean }
export interface TreeState {
  nodes: Map<string, TreeNode>; branches: Map<string, Branch>; waits: Map<string,WaitBranch>; summaries: Map<string,Branch>;
  graph: JobGraph; family: Set<string>; inspected: Set<string>; warnings: string[];
  blocked: Set<string>; loading: boolean; error?: string; boundary?: string;
  confirmedRoot?: string; started: string; observed?: string; stop?: string; previous: boolean;
}
export interface TreeAPI {
  node: (tenant: string, id: string, signal: AbortSignal) => Promise<TreeNodeResponse>;
  children: (tenant: string, id: string, token: string, signal: AbortSignal) => Promise<TreeChildrenResponse>;
  waits?: (tenant: string, id: string, token: string, signal: AbortSignal) => Promise<TreeWaitsResponse>;
}
const api: TreeAPI = { node:getTreeNode, children:getTreeChildren, waits:getTreeWaits };
const branch = (): Branch => ({state:'pending',token:'',seen:new Set(),depth:0});
export const emptyWait = (): WaitObservation => ({rows:[],issues:[],loading:false,stale:false,loaded:false,token:'',pages:0,inspected:0});
const runnable=(b:Branch)=>['pending','more'].includes(b.state) || b.state==='limited' && ['Request limit','Elapsed-time limit'].includes(b.reason||'');

/** One anchored observation owns every summary, child stream and wait stream. */
export class Traversal {
  state: TreeState;
  private controller = new AbortController();
  private disposed=false; private generation=0; private pages=0; private entries=0;
  private ascentHops=0; private ancestor?:string; private initialized=false; private ascentStopped=false; private ancestorRetry=false;
  private freshSummaries=new Set<string>();
  private turn=0; private scheduled=new Map<string,number>();
  interrupted=false;
  constructor(readonly tenant:string, readonly entry:string, private changed:()=>void, private source:TreeAPI=api, readonly budgets:Budgets=DEFAULT_BUDGETS, private now=()=>performance.now()) {
    this.state={nodes:new Map(),branches:new Map(),waits:new Map(),summaries:new Map(),graph:new JobGraph(tenant),family:new Set(),inspected:new Set(),warnings:[],blocked:new Set(),loading:false,started:new Date().toISOString(),previous:false};
  }
  get viewCapped() { return this.pages>=this.budgets.pages || this.state.graph.nodes.size>=this.budgets.nodes || this.state.graph.edges.size>=this.budgets.edges; }
  get canResume() {
    const d=this.distances(),g=this.state.graph;
    return !this.disposed && (!this.initialized || !!this.ancestor && (!this.ascentStopped||this.ancestorRetry)
      || [...this.state.summaries.values()].some(runnable)
      || [...this.state.branches].some(([id,b])=>(d.has(g.key(id))||this.state.family.has(id))&&runnable(b))
      || [...this.state.waits].some(([id,b])=>(d.has(g.key(id))||this.state.inspected.has(id))&&runnable(b)));
  }
  private emit(){if(!this.disposed)this.changed();}
  private warning(message:string){if(!this.state.warnings.includes(message))this.state.warnings.push(message);}
  pause(){
    this.interrupted ||= this.state.loading; ++this.generation;this.controller.abort();this.state.loading=false;
    for(const b of [...this.state.summaries.values(),...this.state.branches.values(),...this.state.waits.values()]) if(b.state==='loading'){b.state=b.token?'more':'pending';b.reason=undefined;}
    for(const b of this.state.waits.values())b.observation.loading=false;
    this.emit();
  }
  cancel(){this.disposed=true;this.pause();}
  private ensure(id:string){
    const g=this.state.graph,key=g.key(id);
    if(!g.nodes.has(key) && g.nodes.size>=this.budgets.nodes)return false;
    g.add(id);
    if(!this.state.nodes.has(id)&&!this.state.summaries.has(id))this.state.summaries.set(id,branch());
    return true;
  }
  private connect(source:string,id:string,kind:'child'|'wait',rows:WaitRow[]=[]){
    const g=this.state.graph,from=g.key(source),to=g.key(id);
    if(!g.edges.has(edgeKey(from,to,kind))&&g.edges.size>=this.budgets.edges)return 'Edge limit';
    if(!this.ensure(id))return 'Node limit';
    // During replacement, keep prior metadata until the entire stream is verified.
    if(!(kind==='wait'&&g.edges.get(edgeKey(from,to,kind))?.previous))g.connect(from,to,kind,rows);return undefined;
  }
  private accept(node:TreeNode,parent?:string){
    if(!node?.job_id?.trim()||node.tenant_id!==this.tenant||(parent!==undefined&&node.parent_job_id!==parent)){this.warning('Invalid identity, tenant or child relationship omitted');return false;}
    const old=this.state.nodes.get(node.job_id);
    if(old&&old.parent_job_id!==node.parent_job_id){this.state.blocked.add(node.job_id);this.warning(`Conflicting parent observations: ${node.job_id}`);return false;}
    let cursor=node.parent_job_id;const seen=new Set([node.job_id]);
    while(cursor){if(seen.has(cursor)){this.state.blocked.add(node.job_id);this.warning(`Cycle boundary: ${node.job_id}`);return false;}seen.add(cursor);cursor=this.state.nodes.get(cursor)?.parent_job_id;}
    if(!this.ensure(node.job_id))return false;
    this.state.nodes.set(node.job_id,node);this.freshSummaries.add(node.job_id);this.state.summaries.delete(node.job_id);
    if(!this.state.branches.has(node.job_id))this.state.branches.set(node.job_id,branch());
    if(!this.state.waits.has(node.job_id))this.state.waits.set(node.job_id,{...branch(),observation:emptyWait(),staged:[],legacyAdded:false});
    if(node.recipe_parent_job_id&&node.recipe_parent_job_id!==node.parent_job_id)this.warning(`Recipe attribution differs from runtime parent: ${node.job_id}`);
    return true;
  }
  private confirmRoot(){
    this.state.confirmedRoot=undefined;let id:string|undefined=this.entry;const seen=new Set<string>();
    while(id){if(seen.has(id)||this.state.blocked.has(id))return;seen.add(id);const n=this.state.nodes.get(id);if(!n)return;if(!n.parent_job_id){this.state.confirmedRoot=id;return;}id=n.parent_job_id;}
  }
  distances(){return this.state.graph.distances([this.state.graph.key(this.entry)]);}
  private contextDepth(id:string){let depth=0,p=this.state.nodes.get(id)?.parent_job_id;const seen=new Set([id]);while(p&&this.state.family.has(p)&&!seen.has(p)){seen.add(p);depth++;p=this.state.nodes.get(p)?.parent_job_id;}return depth;}
  inspect(id:string){if(!this.state.nodes.has(id)||this.state.inspected.has(id)||this.distances().has(this.state.graph.key(id)))return;this.state.inspected.add(id);this.emit();if(!this.state.loading)void this.run();}
  waitView(id:string):WaitController {
    const b=this.state.waits.get(id);
    return {tenant:this.tenant,state:b?.observation||emptyWait(),rows:b?.observation.rows||[],canLoad:!!b&&b.state==='failed',refresh:()=>this.refreshWaits(id),load:()=>this.retryWaits(id)};
  }
  retrySummary(id:string){const b=this.state.summaries.get(id);if(b?.state==='failed'){b.state='pending';b.reason=undefined;}return this.run();}
  retryWaits(id:string){const b=this.state.waits.get(id);if(b?.state==='failed'){b.state=b.token?'more':'pending';b.observation.error=undefined;}return this.run();}
  refreshWaits(id:string){
    const b=this.state.waits.get(id);if(!b)return;
    if(!this.distances().has(this.state.graph.key(id)))this.state.inspected.add(id);
    this.pause();
    const old=b.observation;
    this.state.waits.set(id,{...branch(),observation:{...emptyWait(),rows:old.rows,stale:old.loaded,loaded:old.loaded,observed:old.observed,snapshot:old.snapshot},staged:[],legacyAdded:false});
    for(const e of this.state.graph.edges.values())if(e.source===this.state.graph.key(id)&&e.kind==='wait')e.previous=true;
    void this.run();
  }
  /** Keep the previous observation available for recovery if a refresh fails. */
  refresh(){
    this.pause();this.pages=0;this.entries=0;this.ascentHops=0;this.initialized=false;this.ascentStopped=false;this.ancestor=undefined;
    this.freshSummaries.clear();
    this.state.previous=true;this.state.warnings=[];this.state.boundary=undefined;this.state.started=new Date().toISOString();
    for(const [id] of this.state.branches)this.state.branches.set(id,branch());
    for(const [id,b] of this.state.waits)this.state.waits.set(id,{...branch(),observation:{...emptyWait(),rows:b.observation.rows,loaded:b.observation.loaded,stale:b.observation.loaded,observed:b.observation.observed,snapshot:b.observation.snapshot},staged:[],legacyAdded:false});
    for(const id of this.state.nodes.keys())this.state.summaries.set(id,branch());
    for(const [id]of this.state.summaries)this.state.summaries.set(id,branch());
    for(const e of this.state.graph.edges.values())e.previous=true;
    void this.run();
  }
  private prune(){
    // Derive family membership from the entry's retained ancestry and actual child
    // edges. Removed sibling branches must not become permanent pruning roots.
    const ancestors=new Set<string>();let id:string|undefined=this.entry;
    while(id&&!ancestors.has(id)){ancestors.add(id);id=this.state.nodes.get(id)?.parent_job_id;if(id&&!this.state.nodes.has(id))break;}
    const family=this.state.graph.distances([...ancestors].map(id=>this.state.graph.key(id)),[...this.state.graph.edges.values()].filter(e=>e.kind==='child'));
    this.state.family=new Set([...family.keys()].map(key=>this.state.graph.nodes.get(key)?.id).filter((id):id is string=>!!id&&this.state.nodes.has(id)));
    const roots=[...this.state.family,...this.state.inspected,this.entry].map(id=>this.state.graph.key(id));
    const keep=this.state.graph.prune(roots);
    for(const id of this.state.family)if(!keep.has(this.state.graph.key(id)))this.state.family.delete(id);
    for(const id of this.freshSummaries)if(!keep.has(this.state.graph.key(id)))this.freshSummaries.delete(id);
    for(const map of [this.state.nodes,this.state.branches,this.state.waits,this.state.summaries])for(const id of map.keys())if(!keep.has(this.state.graph.key(id)))map.delete(id);
  }
  async run(retry?:string){
    if(this.disposed||this.state.loading)return;
    if(this.ancestorRetry){this.ascentStopped=false;this.ancestorRetry=false;}
    if(retry){const b=this.state.branches.get(retry);if(b?.state==='failed'){b.state=b.token?'more':'pending';b.reason=undefined;}}
    const generation=++this.generation,stale=()=>this.disposed||generation!==this.generation;
    this.interrupted=false;this.state.loading=true;this.state.error=undefined;this.state.stop=undefined;
    this.controller=new AbortController();const signal=this.controller.signal,start=this.now();let requests=0;
    const limit=()=>signal.aborted||this.now()-start>=this.budgets.milliseconds?'Elapsed-time limit':requests>=this.budgets.requests?'Request limit':undefined;
    const call=async<T,>(fn:(s:AbortSignal)=>Promise<T>):Promise<T>=>{
      const reason=limit();if(reason)throw Error(reason);requests++;
      const abort=new AbortController();
      return new Promise<T>((resolve,reject)=>{
        const stop=()=>{abort.abort();reject(Error('Elapsed-time limit'));};
        signal.addEventListener('abort',stop,{once:true});
        const timer=setTimeout(()=>{abort.abort();reject(Error(this.now()-start>=this.budgets.milliseconds?'Elapsed-time limit':'Request timed out'));},Math.min(this.budgets.requestMilliseconds,Math.max(0,this.budgets.milliseconds-(this.now()-start))));
        const finish=()=>{clearTimeout(timer);signal.removeEventListener('abort',stop);};
        try { fn(AbortSignal.any([signal,abort.signal])).then(resolve,reject).finally(finish); } catch(error) { finish();reject(error); }
      });
    };
    const mark=(b:Branch,reason:string)=>{b.state='limited';b.reason=reason;this.state.stop=reason;};
    this.emit();
    try{
      if(!this.initialized){
        const response=await call(s=>this.source.node(this.tenant,this.entry,s));if(stale())return;
        if(response.node?.job_id!==this.entry||!this.accept(response.node))throw Error('Entry unavailable');
        this.initialized=true;this.ancestor=response.node.parent_job_id;this.state.family.add(this.entry);this.state.observed=response.observed_at;
      }
      while(!stale()){
        if(limit()){this.state.stop=limit();break;}
        const depths=this.distances();
        type Task={id:string;type:'summary'|'child'|'wait';b:Branch;priority:number;order:number};
        const tasks:Task[]=[];
        let order=0;
        for(const [id,b]of this.state.summaries)if(runnable(b))tasks.push({id,b,type:'summary',priority:depths.get(this.state.graph.key(id))??10000,order:order++});
        for(const [id,b]of this.state.branches){
          const d=depths.get(this.state.graph.key(id)),context=this.state.family.has(id);
          if(d===undefined&&!context)continue;
          b.depth=d??this.contextDepth(id);
          if(b.state==='limited'&&b.reason==='Depth limit'&&b.depth<this.budgets.depth){b.state=b.token?'more':'pending';b.reason=undefined;}
          if(!runnable(b))continue;
          if(b.depth>=this.budgets.depth){mark(b,'Depth limit');continue;}
          tasks.push({id,b,type:'child',priority:d??10000+b.depth,order:order++});
        }
        for(const [id,b]of this.state.waits){
          const d=depths.get(this.state.graph.key(id));
          if(d===undefined&&!this.state.inspected.has(id))continue;
          b.depth=d??0;
          if(b.state==='limited'&&b.reason==='Depth limit'&&b.depth<this.budgets.depth){b.state=b.token?'more':'pending';b.reason=undefined;}
          if(!runnable(b))continue;
          if(b.depth>=this.budgets.depth){mark(b,'Depth limit');b.observation.stopped=b.reason;continue;}
          tasks.push({id,b,type:'wait',priority:d??20000,order:order++});
        }
        // Round robin among ready streams at a breadth level. Each dispatch reads one page.
        tasks.sort((a,b)=>(a.priority<10000?0:1)-(b.priority<10000?0:1) || (this.scheduled.get(a.type+':'+a.id)||0)-(this.scheduled.get(b.type+':'+b.id)||0) || a.priority-b.priority || a.id.localeCompare(b.id)||a.type.localeCompare(b.type));
        const down=tasks.filter(t=>t.priority<10000);
        if(!down.length&&this.ancestor&&!this.ascentStopped){
          if(this.ascentHops>=this.budgets.ascent){this.state.boundary='Ancestor hop limit';this.ascentStopped=true;continue;}
          const id=this.ancestor;
          if(!this.ensure(id)){this.state.boundary='Node limit';this.ascentStopped=true;continue;}
          try{
            const response=this.freshSummaries.has(id)?{node:this.state.nodes.get(id)!,observed_at:this.state.observed||''}:await call(s=>this.source.node(this.tenant,id,s));if(stale())return;
            if(response.node?.job_id!==id||!this.accept(response.node)){this.state.boundary='Malformed ancestor boundary';this.ascentStopped=true;continue;}
            this.ascentHops++;this.state.family.add(id);this.state.observed=response.observed_at;
            const child=[...this.state.nodes.values()].find(n=>this.state.family.has(n.job_id)&&n.parent_job_id===id);
            if(child)this.connect(id,child.job_id,'child');
            this.ancestor=response.node.parent_job_id;this.state.boundary=undefined;this.emit();continue;
          }catch{if(stale())return;if(this.state.nodes.has(id)){const b=this.state.summaries.get(id)||branch();b.state='failed';b.reason='Ancestor summary unavailable';this.state.summaries.set(id,b);}else{this.state.summaries.delete(id);this.state.graph.nodes.delete(this.state.graph.key(id));}this.state.boundary=limit()||'Ancestor unavailable; known component retained';this.ascentStopped=true;this.ancestorRetry=true;continue;}
        }
        if(!tasks.length)break;
        let dispatched=0,reserved=0;
        const batch:Promise<void>[]=[];
        for(const task of tasks){
          if(dispatched>=Math.min(4,this.budgets.concurrency)||limit())break;
          const {id,b,type}=task;
          if(type==='summary'&&this.freshSummaries.has(id)){this.state.summaries.delete(id);continue;}
          if(type!=='summary'&&this.state.graph.nodes.size>=this.budgets.nodes){mark(b,'Node limit');if(type==='wait')this.state.waits.get(id)!.observation.stopped=b.reason;continue;}
          if(type!=='summary'&&this.pages>=this.budgets.pages){mark(b,'Page limit');if(type==='wait')this.state.waits.get(id)!.observation.stopped=b.reason;continue;}
          if(type==='wait'){
            const w=this.state.waits.get(id)!;
            if(w.observation.pages>=this.budgets.waitPages){mark(b,'Wait page limit reached');w.observation.stopped=b.reason;continue;}
            if(w.observation.inspected>=this.budgets.waitEntries||this.entries>=this.budgets.entries){mark(b,'Dependency row limit reached');w.observation.stopped=b.reason;continue;}
          }
          if(type==='wait'&&this.entries+reserved>=this.budgets.entries)continue;
          b.state='loading';b.reason=undefined;
          if(type!=='summary')this.pages++;
          // Reserve dependency capacity before concurrent replies arrive.
          const w=type==='wait'?this.state.waits.get(id):undefined;
          const allowance=w?Math.min(this.budgets.waitEntries-w.observation.inspected,this.budgets.entries-this.entries-reserved):0;
          if(w){reserved+=allowance;w.observation.pages++;w.observation.loading=true;w.observation.error=undefined;}
          dispatched++;this.scheduled.set(type+':'+id,++this.turn);
          batch.push((async()=>{
            let used=0;
            // A sibling response can prune this source while its read is in flight.
            // Identity also rejects the old stream if the same job is rediscovered.
            const current=()=>!stale()&&this.state.graph.nodes.has(this.state.graph.key(id))
              &&(type==='summary'?this.state.summaries:type==='child'?this.state.branches:this.state.waits).get(id)===b;
            try{
              if(type==='summary'){
                const result=await call(s=>this.source.node(this.tenant,id,s));if(!current())return;
                if(result.node?.job_id!==id||!this.accept(result.node))throw Error('Summary identity unavailable');
                this.state.observed=result.observed_at;return;
              }
              const token=b.token;
              if(type==='child'){
                const page=await call(s=>this.source.children(this.tenant,id,token,s));if(!current())return;
                if(!Array.isArray(page.nodes))throw Error('Malformed child page');
                const accepted=new Set<string>();let bad=false,omitted:string|undefined;
                for(const node of page.nodes){
                  if(node?.tenant_id!==this.tenant||node?.parent_job_id!==id||!node?.job_id?.trim()){this.warning('Invalid identity, tenant or child relationship omitted');bad=true;continue;}
                  const reason=this.connect(id,node.job_id,'child');if(reason){omitted=reason;break;}
                  if(!this.accept(node,id)){this.state.graph.edges.delete(edgeKey(this.state.graph.key(id),this.state.graph.key(node.job_id),'child'));bad=true;continue;}
                  accepted.add(node.job_id);if(this.state.family.has(id))this.state.family.add(node.job_id);
                }
                // Only a complete replacement can remove previous child contributions.
                for(const edge of this.state.graph.edges.values())if(edge.source===this.state.graph.key(id)&&edge.kind==='child'&&accepted.has(this.state.graph.nodes.get(edge.target)!.id))edge.previous=false;
                if(omitted){mark(b,`${omitted}; page records omitted. Open branch as entry`);return;}
                if(bad){b.state='malformed';b.reason='Malformed relationships omitted';return;}
                this.advance(b,page.next_page_token,page.observed_at);
                if(b.state==='exhausted'){for(const [key,edge]of this.state.graph.edges)if(edge.source===this.state.graph.key(id)&&edge.kind==='child'&&edge.previous)this.state.graph.edges.delete(key);this.prune();}
              }else if(w){
                const page=this.source.waits?await call(s=>this.source.waits!(this.tenant,id,token,s)):{consumer:{tenant_id:this.tenant,job_id:id},waits:[],issues:[],observed_at:new Date().toISOString(),consumer_snapshot:{state:'available' as const,observed_at:''}};
                if(!current())return;
                if(page.consumer?.tenant_id!==this.tenant||page.consumer.job_id!==id||!Array.isArray(page.waits)||!Array.isArray(page.issues))throw Error('Mismatched consumer');
                const normalized=normalizeWaits(page,this.tenant,id,allowance);used=normalized.inspected;
                const s=w.observation;
                if(!token)s.snapshot=page.consumer_snapshot;
                const rows=[...normalized.rows];
                if(!page.next_page_token&&!w.legacyAdded){
                  const legacy=s.snapshot?.legacy_wait_for||[];
                  const count=Math.min(legacy.length,Math.max(0,allowance-used));used+=count;
                  rows.push(...legacyRows(this.tenant,legacy.slice(0,count)));
                  if(count<legacy.length)normalized.issues.push('Legacy reference limit reached');
                  w.legacyAdded=true;
                }
                s.hasGoalWait ||= normalized.hasGoalWait;
                s.inspected+=used;s.observed=page.observed_at;s.loaded=true;
                s.issues=[...new Set([...s.issues,...normalized.issues])];
                const existing=new Set(w.staged.map(r=>r.key));let omitted:string|undefined;
                const rowIdentity=(r:WaitRow)=>JSON.stringify([r.key,r.targetKey,r.condition,r.deadline,r.blockedReason]);
                const retained=new Set(s.rows.map(rowIdentity));
                let stored=[...this.state.waits.values()].reduce((sum,v)=>sum+v.observation.rows.length,0);
                for(const row of rows){
                  if(existing.has(row.key)){s.issues.push('Duplicate wait entry omitted');continue;}
                  const identity=rowIdentity(row);
                  if(!retained.has(identity)&&(retained.size>=this.budgets.waitEntries||stored>=this.budgets.entries)){omitted='Retained dependency row limit';break;}
                  if(!retained.has(identity)){retained.add(identity);stored++;}
                  const reason=this.connect(id,row.id,'wait',[row]);if(reason){omitted=reason;break;}
                  existing.add(row.key);w.staged.push(row);
                }
                // Previous rows remain readable throughout an incomplete replacement.
                s.rows=s.stale?[...new Map([...s.rows,...w.staged].map(r=>[rowIdentity(r),r])).values()]:[...w.staged];
                if(omitted||normalized.issues.includes('Dependency row limit reached')||normalized.issues.includes('Legacy reference limit reached')){mark(b,(omitted||'Dependency row limit')+'; page records omitted');s.stopped=b.reason;s.token=token;return;}
                this.advance(b,page.next_page_token,page.observed_at);s.token=b.token;if(b.state==='malformed')s.stopped=b.reason;
                if(s.issues.length && !page.next_page_token){b.state='malformed';b.reason='Partial wait coverage';s.stopped=b.reason;}
                if(b.state==='exhausted'&&s.snapshot?.state==='available'){
                  s.rows=[...w.staged];s.stale=false;
                  for(const [key,edge]of this.state.graph.edges)if(edge.source===this.state.graph.key(id)&&edge.kind==='wait')this.state.graph.edges.delete(key);
                  for(const target of waitTargets(w.staged))this.connect(id,target.id,'wait',target.rows);
                  this.prune();
                }else if(s.snapshot?.state!=='available')s.issues=[...new Set([...s.issues,'Supplementary job information unavailable'])];
              }
            }catch(error){
              if(!current())return;
              const message=error instanceof Error?error.message:'';
              if(message==='Request limit'||message==='Elapsed-time limit'){mark(b,message);}
              else{b.state='failed';b.reason=type==='child'?'Child page unavailable':type==='wait'?'Wait page unavailable; pending dependency information unavailable':'Job summary unavailable';}
              if(w)w.observation.error=b.reason;
            }finally{
              if(!stale()){if(w&&this.state.waits.get(id)===w){this.entries+=used;w.observation.loading=false;}this.emit();}
            }
          })());
        }
        if(!batch.length)break;
        this.emit();await Promise.all(batch);
      }
    }catch{if(!stale()){if(this.state.nodes.has(this.entry)){this.state.stop='Entry job unavailable; previous observation retained';this.state.previous=true;}else this.state.error='Entry job unavailable. Retry or return to jobs.';}}
    finally{if(!stale()){if(this.state.stop==='Depth limit'&&![...this.state.branches.values(),...this.state.waits.values()].some(b=>b.state==='limited'&&b.reason==='Depth limit'))this.state.stop=undefined;this.confirmRoot();this.state.previous=[...this.state.graph.edges.values()].some(e=>e.previous)||[...this.state.summaries.keys()].some(id=>this.state.nodes.has(id))||(!this.initialized&&this.state.nodes.size>0);this.state.loading=false;this.emit();}}
  }
  private advance(b:Branch,next:string|undefined,observed:string){
    this.state.observed=observed;b.observed=observed;
    if(!next){b.state='exhausted';b.token='';return;}
    if(next===b.token||b.seen.has(next)){b.state='malformed';b.reason='Repeated cursor; incomplete branch';return;}
    b.seen.add(next);b.token=next;b.state='more';
  }
}
