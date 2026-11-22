import { waitKey, type WaitRow } from './waits';

export interface GraphNode { key: string; id: string; kind: 'job' | 'goal' }
export interface Edge { key: string; source: string; target: string; kind: 'child' | 'wait'; rows: WaitRow[]; previous?: boolean }
export const edgeKey = (source: string, target: string, kind: Edge['kind']) => JSON.stringify([source,target,kind]);
/** A tenant-scoped graph. Runtime parentage and waits never substitute for one another. */
export class JobGraph {
  nodes = new Map<string, GraphNode>();
  edges = new Map<string, Edge>();
  constructor(readonly tenant: string) {}
  key(id: string, kind: GraphNode['kind'] = 'job') { return waitKey(this.tenant,kind,id); }
  add(id: string, kind: GraphNode['kind'] = 'job') {
    const key = this.key(id,kind);
    if (!this.nodes.has(key)) this.nodes.set(key,{key,id,kind});
    return key;
  }
  connect(source: string, target: string, kind: Edge['kind'], rows: WaitRow[] = []) {
    const key = edgeKey(source,target,kind), old = this.edges.get(key);
    const byKey = new Map((old?.rows || []).map(r=>[r.key,r]));
    rows.forEach(r=>byKey.set(r.key,r));
    this.edges.set(key,{key,source,target,kind,rows:[...byKey.values()],previous:old?.previous});
  }
  distances(roots: Iterable<string>, edges: Iterable<Edge> = this.edges.values()) {
    const adjacency = new Map<string,string[]>();
    for (const edge of edges) adjacency.set(edge.source,[...(adjacency.get(edge.source)||[]),edge.target]);
    const distance = new Map<string,number>(), queue = [...roots];
    queue.forEach(key=>distance.set(key,0));
    for (let i=0;i<queue.length;i++) for (const target of adjacency.get(queue[i])||[]) if (!distance.has(target)) {
      distance.set(target,distance.get(queue[i])!+1);queue.push(target);
    }
    return distance;
  }
  /** Tarjan components: shared diamonds are not cycles. */
  components() {
    const adjacency = new Map<string,string[]>();
    for (const e of this.edges.values()) adjacency.set(e.source,[...(adjacency.get(e.source)||[]),e.target]);
    let serial=0;
    const index=new Map<string,number>(), low=new Map<string,number>(), stack:string[]=[], active=new Set<string>(), result:string[][]=[];
    const visit=(key:string)=>{
      index.set(key,serial);low.set(key,serial++);stack.push(key);active.add(key);
      for(const target of adjacency.get(key)||[]) {
        if(!index.has(target)) {visit(target);low.set(key,Math.min(low.get(key)!,low.get(target)!));}
        else if(active.has(target)) low.set(key,Math.min(low.get(key)!,index.get(target)!));
      }
      if(low.get(key)===index.get(key)) {const component:string[]=[];let item:string;do {item=stack.pop()!;active.delete(item);component.push(item);} while(item!==key);result.push(component.sort());}
    };
    [...this.nodes.keys()].sort().forEach(key=>{if(!index.has(key))visit(key);});
    return result;
  }
  prune(roots: Iterable<string>) {
    const keep=this.distances(roots);
    for(const key of this.nodes.keys()) if(!keep.has(key))this.nodes.delete(key);
    for(const [key,e] of this.edges)if(!keep.has(e.source)||!keep.has(e.target))this.edges.delete(key);
    return keep;
  }
}
