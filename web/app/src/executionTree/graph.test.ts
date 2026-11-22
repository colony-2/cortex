import { expect, it } from 'vitest';
import { JobGraph } from './graph';
import { legacyRows } from './waits';
const graph=()=>{const g=new JobGraph('t');['entry','a','b','shared'].forEach(id=>g.add(id));return g;};
it('DG01,02,04 shares identities, groups metadata, and keeps a diamond separate from cycles',()=>{
 const g=graph(),key=(id:string)=>g.key(id);g.connect(key('entry'),key('a'),'child');g.connect(key('entry'),key('b'),'wait');g.connect(key('a'),key('shared'),'wait',legacyRows('t',['shared']));
 g.connect(key('b'),key('shared'),'wait',[{...legacyRows('t',['shared'])[0],key:'another',waitID:'explicit',condition:'success',deadline:'2026-11-20T00:00:00Z',blockedReason:'waiting'}]);g.connect(key('a'),key('shared'),'child');
 expect(g.nodes.size).toBe(4);expect(g.edges.size).toBe(5);expect(g.components().every(c=>c.length===1)).toBe(true);expect(g.distances([key('entry')]).get(key('shared'))).toBe(2);
 expect([...g.edges.values()].find(e=>e.source===key('b'))?.rows[0]).toMatchObject({waitID:'explicit',condition:'success',blockedReason:'waiting'});
});
it('DG03,05 preserves mixed and self cycles and prunes orphaned components after verified edge removal',()=>{
 const g=graph(),key=(id:string)=>g.key(id);g.connect(key('entry'),key('a'),'wait');g.connect(key('a'),key('b'),'child');g.connect(key('b'),key('a'),'wait');g.connect(key('shared'),key('shared'),'wait');
 expect(g.components().some(c=>c.length===2)).toBe(true);for(const [k,e]of g.edges)if(e.source===key('entry'))g.edges.delete(k);
 g.prune([key('entry'),key('shared')]);expect([...g.nodes.values()].map(n=>n.id).sort()).toEqual(['entry','shared']);expect(g.edges.size).toBe(1);
});
