import { JobGraph } from './graph';
export const GEOMETRY = { width:272, height:228, columnGap:72, rowGap:52, padding:32 };
export interface Point { x:number; y:number }
export interface Rect extends Point { width:number; height:number }
export interface Route { points:Point[]; path:string; label:Rect }
export function crosses(a:Point,b:Point,r:Rect) {
  if(a.x===b.x)return a.x>r.x+.1&&a.x<r.x+r.width-.1&&Math.max(a.y,b.y)>r.y+.1&&Math.min(a.y,b.y)<r.y+r.height-.1;
  return a.y>r.y+.1&&a.y<r.y+r.height-.1&&Math.max(a.x,b.x)>r.x+.1&&Math.min(a.x,b.x)<r.x+r.width-.1;
}
export function routeEdge(start:Rect,end:Rect,obstacles:Rect[],offset=0):Route {
  const first={x:start.x+start.width,y:start.y+start.height/2+offset}, last={x:end.x,y:end.y+end.height/2+offset};
  const clear=(points:Point[])=>points.slice(1).every((p,i)=>!obstacles.some(r=>crosses(points[i],p,r)));
  const midpoint=(first.x+last.x)/2+offset;
  let points=[first,{x:midpoint,y:first.y},{x:midpoint,y:last.y},last];
  if(last.x<=first.x || !clear(points)) {
    const exit=first.x+20+Math.abs(offset),enter=last.x-20-Math.abs(offset);
    // Search nearby row gaps before a more distant detour. No global top lane.
    const lanes=[start.y-22,end.y-22,start.y+start.height+22,end.y+end.height+22,...obstacles.flatMap(r=>[r.y-22,r.y+r.height+22])];
    lanes.sort((a,b)=>Math.abs(a-first.y)+Math.abs(a-last.y)-Math.abs(b-first.y)-Math.abs(b-last.y)||a-b);
    const candidate=lanes.map(y=>[first,{x:exit,y:first.y},{x:exit,y},{x:enter,y},{x:enter,y:last.y},last]).find(clear);
    if(candidate)points=candidate;
    else {
      // Grid visibility routing handles staggered long/shared links. Vertices sit in free gaps.
      const xs=[...new Set([first.x,last.x,exit,enter,...obstacles.flatMap(r=>[r.x-20,r.x+r.width+20])])].sort((a,b)=>a-b);
      const ys=[...new Set([first.y,last.y,...lanes])].sort((a,b)=>a-b);
      const valid=(p:Point)=>!obstacles.some(r=>p.x>r.x&&p.x<r.x+r.width&&p.y>r.y&&p.y<r.y+r.height);
      const key=(x:number,y:number)=>y*xs.length+x, sx=xs.indexOf(first.x),sy=ys.indexOf(first.y),target=key(xs.indexOf(last.x),ys.indexOf(last.y));
      const queue=[key(sx,sy)],parent=new Map<number,number>(),seen=new Set(queue);
      for(let i=0;i<queue.length&&!seen.has(target);i++){
        const current=queue[i],x=current%xs.length,y=Math.floor(current/xs.length),a={x:xs[x],y:ys[y]};
        for(const [nx,ny] of [[x-1,y],[x+1,y],[x,y-1],[x,y+1]]){
          if(nx<0||ny<0||nx>=xs.length||ny>=ys.length)continue;
          const next=key(nx,ny),b={x:xs[nx],y:ys[ny]};
          if(seen.has(next)||!valid(b)||!clear([a,b]))continue;
          seen.add(next);parent.set(next,current);queue.push(next);
        }
      }
      if(seen.has(target)){const path:Point[]=[];let k:number|undefined=target;while(k!==undefined){path.push({x:xs[k%xs.length],y:ys[Math.floor(k/xs.length)]});k=parent.get(k);}points=path.reverse();}
    }
  }
  const segments=points.slice(1).map((p,i)=>({a:points[i],b:p,length:Math.abs(p.x-points[i].x)+Math.abs(p.y-points[i].y)})).sort((a,b)=>b.length-a.length);
  const segment=segments.find(s=>s.a.x===s.b.x)||segments[0];
  const label={x:(segment.a.x+segment.b.x)/2+6,y:(segment.a.y+segment.b.y)/2-20,width:110,height:16};
  // Put a short label in an adjacent gap when the chosen lane is near a card.
  if(obstacles.some(r=>label.x<r.x+r.width&&label.x+label.width>r.x&&label.y<r.y+r.height&&label.y+label.height>r.y)){
    label.x=end.x;label.y=end.y-24;
  }
  return {points,path:points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' '),label};
}
export function layoutGraph(graph:JobGraph,collapsed:Set<string>=new Set(),reveal:Set<string>=new Set()) {
  const all=[...graph.nodes.keys()].sort(),edgesByPriority=[...graph.edges.values()].sort((a,b)=>(a.kind==='child'?0:1)-(b.kind==='child'?0:1)||a.source.localeCompare(b.source)||a.target.localeCompare(b.target));
  const components=graph.components(),componentByNode=new Map<string,number>();
  components.forEach((component,i)=>component.forEach(key=>componentByNode.set(key,i)));
  const parents=new Map<string,string>();
  // Place the acyclic component graph first, then give each component a finite
  // internal presentation tree. Return edges are routed, never used recursively.
  for(let i=0;i<components.length;i++){
    const component=components[i],incoming=edgesByPriority.find(e=>componentByNode.get(e.target)===i&&componentByNode.get(e.source)!==i);
    const root=incoming?.target||component[0],placed=new Set([root]);
    if(incoming)parents.set(root,incoming.source);
    while(placed.size<component.length){
      const edge=edgesByPriority.find(e=>componentByNode.get(e.source)===i&&placed.has(e.source)&&componentByNode.get(e.target)===i&&!placed.has(e.target));
      if(!edge)break;
      parents.set(edge.target,edge.source);placed.add(edge.target);
    }
  }
  const roots=all.filter(id=>!parents.has(id));
  const visible=new Set<string>(),queue=[...roots];
  for(let i=0;i<queue.length;i++){
    const id=queue[i];if(visible.has(id))continue;visible.add(id);
    if(collapsed.has(id)&&!reveal.has(id))continue;
    for(const e of graph.edges.values())if(e.source===id)queue.push(e.target);
  }
  // Reveal nodes explicitly, including the known paths supplied by search.
  reveal.forEach(id=>visible.add(id));
  const children=new Map<string,string[]>();
  for(const id of all)if(visible.has(id)){const p=parents.get(id);if(p&&visible.has(p))children.set(p,[...(children.get(p)||[]),id]);}
  const positions=new Map<string,Rect>();let row=0,maxDepth=0;
  const {width,height,columnGap,rowGap,padding}=GEOMETRY;
  const place=(id:string,depth:number)=>{
    const kids=(children.get(id)||[]).sort(),first=row;
    kids.forEach(k=>place(k,depth+1));
    if(!kids.length)row++;
    positions.set(id,{x:padding+depth*(width+columnGap),y:padding+(first+row-1)*(height+rowGap)/2,width,height});maxDepth=Math.max(maxDepth,depth);
  };
  all.filter(id=>visible.has(id)&&(!parents.has(id)||!visible.has(parents.get(id)!))).forEach(id=>place(id,0));
  const routes=new Map<string,Route>(),obstacles=[...positions.values()];
  const edges=[...graph.edges.values()].sort((a,b)=>a.key.localeCompare(b.key));
  const labels:Rect[]=[];
  const intersects=(a:Rect,b:Rect)=>a.x<b.x+b.width&&a.x+a.width>b.x&&a.y<b.y+b.height&&a.y+a.height>b.y;
  for(const edge of edges){
    const start=positions.get(edge.source),end=positions.get(edge.target);if(!start||!end)continue;
    const dual=edges.some(e=>e.source===edge.source&&e.target===edge.target&&e.kind!==edge.kind);
    const route=routeEdge(start,end,obstacles,dual&&edge.kind==='wait'?-12:0);
    if(edge.kind==='wait'){
      const candidates=[route.label,{...route.label,x:end.x,y:end.y-24},{...route.label,x:start.x,y:start.y-24}];
      for(const p of route.points)candidates.push({...route.label,x:p.x+6,y:p.y-22});
      const valid=(r:Rect)=>r.x>=0&&r.y>=0&&![...obstacles,...labels].some(other=>intersects(r,other));
      let label=candidates.find(valid);
      if(!label)for(let x=padding;x<=padding+maxDepth*(width+columnGap)+width;x+=120){const r={...route.label,x,y:8};if(valid(r)){label=r;break;}}
      if(label){route.label=label;labels.push(label);}
    }
    routes.set(edge.key,route);
  }
  // Fit and the SVG viewport must contain return lanes and complete labels too.
  // Shift all geometry together if a return route extends above or left of cards.
  let minX=0,minY=0,maxX=padding+maxDepth*(width+columnGap)+width+40,maxY=Math.max(height+padding,row*(height+rowGap)+padding);
  const include=(x:number,y:number)=>{minX=Math.min(minX,x-padding);minY=Math.min(minY,y-padding);maxX=Math.max(maxX,x);maxY=Math.max(maxY,y);};
  for(const card of positions.values()){include(card.x,card.y);include(card.x+card.width,card.y+card.height);}
  for(const edge of edges){
    const route=routes.get(edge.key);if(!route)continue;
    for(const point of route.points)include(point.x,point.y);
    if(edge.kind==='wait'){include(route.label.x,route.label.y);include(route.label.x+route.label.width,route.label.y+route.label.height);}
  }
  if(minX||minY){
    for(const card of positions.values()){card.x-=minX;card.y-=minY;}
    for(const route of routes.values()){
      for(const point of route.points){point.x-=minX;point.y-=minY;}
      route.label.x-=minX;route.label.y-=minY;
      route.path=route.points.map((p,i)=>`${i?'L':'M'} ${p.x} ${p.y}`).join(' ');
    }
  }
  return {positions,routes,parents,width:maxX-minX+padding,height:maxY-minY+padding};
}
/** Reveal actual paths, including waits, without initiating discovery. */
export function revealPaths(graph:JobGraph,targets:Iterable<string>){
  const incoming=new Set([...graph.edges.values()].map(e=>e.target)),roots=[...graph.nodes.keys()].filter(id=>!incoming.has(id));
  if(!roots.length&&graph.nodes.size)roots.push([...graph.nodes.keys()].sort()[0]);
  const parent=new Map<string,string>(),seen=new Set(roots),queue=[...roots];
  for(let i=0;i<queue.length;i++)for(const e of graph.edges.values())if(e.source===queue[i]&&!seen.has(e.target)){seen.add(e.target);parent.set(e.target,e.source);queue.push(e.target);}
  const result=new Set<string>();
  for(const id of targets){let key:string|undefined=id;while(key&&!result.has(key)){result.add(key);key=parent.get(key);}}
  return result;
}
