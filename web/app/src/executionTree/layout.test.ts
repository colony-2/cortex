import { expect, it } from 'vitest';
import { JobGraph } from './graph';
import { crosses, GEOMETRY, layoutGraph, routeEdge, revealPaths } from './layout';
function fixture(reverse=false){
 const g=new JobGraph('t'),ids=['entry','a','b','shared','leaf'];(reverse?[...ids].reverse():ids).forEach(id=>g.add(id));
 const links=[['entry','a','child'],['entry','b','wait'],['a','shared','wait'],['b','shared','child'],['shared','leaf','wait']] as const;
 (reverse?[...links].reverse():links).forEach(([a,b,kind])=>g.connect(g.key(a),g.key(b),kind));return g;
}
it('DL04,06 includes cycle routes and complete label extents in the canvas used by Fit',()=>{
 const g=new JobGraph('t');['entry','dependency','nested','child','leaf'].forEach(id=>g.add(id));
 for(const [a,b,kind] of [['entry','dependency','wait'],['dependency','nested','wait'],['nested','child','child'],['child','leaf','wait'],['nested','leaf','wait'],['leaf','dependency','wait']] as const)g.connect(g.key(a),g.key(b),kind);
 // Also exercise the left/top extents of a self return with two relationship lanes.
 g.connect(g.key('entry'),g.key('entry'),'wait');g.connect(g.key('entry'),g.key('dependency'),'child');
 const layout=layoutGraph(g);
 const contained=(x:number,y:number)=>{expect(x).toBeGreaterThanOrEqual(0);expect(y).toBeGreaterThanOrEqual(0);expect(x).toBeLessThanOrEqual(layout.width);expect(y).toBeLessThanOrEqual(layout.height);};
 for(const card of layout.positions.values()){contained(card.x,card.y);contained(card.x+card.width,card.y+card.height);}
 for(const edge of g.edges.values()){
  const route=layout.routes.get(edge.key)!;for(const p of route.points)contained(p.x,p.y);
  if(edge.kind==='wait'){contained(route.label.x,route.label.y);contained(route.label.x+route.label.width,route.label.y+route.label.height);}
 }
});
it('DL01 deterministic placement prefers runtime parents across reply permutations',()=>{
 const a=fixture(),b=fixture(true),la=layoutGraph(a),lb=layoutGraph(b);
 expect([...la.positions].sort()).toEqual([...lb.positions].sort());expect(la.parents.get(a.key('shared'))).toBe(a.key('b'));
});
it('DL02,03 ordinary forward routes attach to local side ports independently of canvas height',()=>{
 const start={x:32,y:500,width:272,height:228},end={...start,x:376,y:780};
 const route=routeEdge(start,end,[start,end]);expect(route.points).toEqual([{x:304,y:614},{x:340,y:614},{x:340,y:894},{x:376,y:894}]);
 expect(routeEdge(start,end,[start,end,{...end,x:720,y:3000}]).path).toBe(route.path);
});
it('DL04–06 long, backwards, self and dual links avoid card interiors with finite local paths',()=>{
 const g=fixture();g.connect(g.key('shared'),g.key('entry'),'wait');g.connect(g.key('a'),g.key('a'),'wait');g.connect(g.key('entry'),g.key('a'),'wait');
 const {positions,routes}=layoutGraph(g);
 expect(positions.size).toBe(5);for(const [key,route] of routes){expect(route.points.length).toBeLessThan(100);expect(route.path).not.toMatch(/NaN|Infinity/);for(let i=1;i<route.points.length;i++)for(const r of positions.values())expect(crosses(route.points[i-1],route.points[i],r),key).toBe(false);}
 const labels=[...g.edges.values()].filter(e=>e.kind==='wait').map(e=>routes.get(e.key)!.label);
 const overlaps=(a:any,b:any)=>a.x<b.x+b.width&&a.x+a.width>b.x&&a.y<b.y+b.height&&a.y+a.height>b.y;
 for(let i=0;i<labels.length;i++){for(const card of positions.values())expect(overlaps(labels[i],card)).toBe(false);for(let j=0;j<i;j++)expect(overlaps(labels[i],labels[j])).toBe(false);}
 const dual=[...g.edges.values()].filter(e=>e.source===g.key('entry')&&e.target===g.key('a')).map(e=>routes.get(e.key)!.path);expect(dual[0]).not.toBe(dual[1]);
 const start={x:32,y:32,...GEOMETRY},end={...start,x:720};const middle={...start,x:376};const long=routeEdge(start,end,[start,middle,end]);for(let i=1;i<long.points.length;i++)expect(crosses(long.points[i-1],long.points[i],middle)).toBe(false);
});
it('DL07 collapse preserves a shared job reached by another visible path; search reveals known wait paths',()=>{
 const g=fixture(),layout=layoutGraph(g,new Set([g.key('a')]));expect(layout.positions.has(g.key('shared'))).toBe(true);
 const revealed=revealPaths(g,[g.key('leaf')]);const search=layoutGraph(g,new Set([g.key('entry')]),revealed);expect(search.positions.has(g.key('leaf'))).toBe(true);
});
