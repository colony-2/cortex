import { describe, expect, it } from 'vitest';
import { resolveArtifactLink as resolve, type LinkCandidate } from './artifactLinks';
const candidates: LinkCandidate[] = [
  {key:'bar',path:'docs/bar.md'}, {key:'foo',path:'docs/foo.md',aliases:['details']},
  {key:'root',path:'foo.md'}, {key:'space',path:'docs/a 雪.md'}, {key:'percent',path:'docs/%20.md'},
];
describe('MAL artifact link resolution', () => {
  it('MAL-001,002 resolves sibling, chained and bounded parents', () => {
    for (const path of ['foo.md','./foo.md','nested/../foo.md']) expect(resolve(path,'docs/bar.md',candidates)).toMatchObject({kind:'target',key:'foo'});
    expect(resolve('bar.md','docs/foo.md',candidates)).toMatchObject({key:'bar'});
    expect(resolve('../foo.md','docs/bar.md',candidates)).toMatchObject({key:'root'});
  });
  it.each(['../../foo.md','../../../foo.md'])('MAL-003 rejects escape %s before alias lookup', reference => {
    expect(resolve(reference,'docs/bar.md',[...candidates,{key:'alias',path:'x',aliases:[reference]}])).toEqual({kind:'unavailable',reason:'outside-context'});
  });
  it('MAL-004 decodes once and remains case-sensitive', () => {
    expect(resolve('a%20%E9%9B%AA.md','docs/bar.md',candidates)).toMatchObject({key:'space'});
    expect(resolve('%2520.md','docs/bar.md',candidates)).toMatchObject({key:'percent'});
    expect(resolve('FOO.md','docs/bar.md',candidates)).toMatchObject({reason:'missing'});
  });
  it.each(['','bad%','%FF','a%2fb','a%5Cb','a\\b','a%00b','a%1fb','a\n'])('MAL-005 rejects invalid reference %j', reference => {
    expect(resolve(reference,'docs/bar.md',candidates)).toEqual({kind:'unavailable',reason:'invalid'});
  });
  it('MAL-006,007 prefers canonical names and restricts exact aliases', () => {
    const list = [...candidates,{key:'other',path:'other.md',aliases:['foo.md','id']}];
    expect(resolve('foo.md','docs/bar.md',list)).toMatchObject({key:'foo'});
    expect(resolve('details','docs/bar.md',list)).toMatchObject({key:'foo'});
    expect(resolve('id','docs/bar.md',list)).toMatchObject({key:'other'});
    expect(resolve('./id','docs/bar.md',list)).toMatchObject({reason:'missing'});
    expect(resolve('../id','docs/bar.md',list)).toMatchObject({reason:'missing'});
  });
  it('MAL-008 deduplicates identities and rejects distinct collisions', () => {
    expect(resolve('foo.md','docs/bar.md',[...candidates,candidates[1]])).toMatchObject({key:'foo'});
    expect(resolve('foo.md','docs/bar.md',[...candidates,{key:'collision',path:'docs/foo.md'}])).toMatchObject({reason:'ambiguous'});
    expect(resolve('details','docs/bar.md',[...candidates,{key:'collision',path:'x',aliases:['details']}])).toMatchObject({reason:'ambiguous'});
  });
  it('MAL-009 rejects missing names without prefix or basename guesses', () => {
    for(const reference of ['outbox/foo.md','a/foo.md','fo.md']) expect(resolve(reference,'bar.md',candidates)).toMatchObject({reason:'missing'});
  });
  it('MAL-010 separates query and decoded fragment', () => {
    expect(resolve('foo.md?version=3#some%20heading','docs/bar.md',candidates)).toEqual({kind:'target',key:'foo',fragment:'some heading'});
    expect(resolve('foo.md#bad%','docs/bar.md',candidates)).toMatchObject({reason:'invalid'});
  });
  it('MAL-011 classifies browser links and fragment-only references before matching', () => {
    for(const reference of ['https://example.com/foo.md','mailto:a@b','//example.com/x','/foo.md']) expect(resolve(reference,'bar.md',[])).toEqual({kind:'browser'});
    expect(resolve('#hello%2520','bar.md',[])).toEqual({kind:'fragment',fragment:'hello%20'});
  });
  it('MAL-012 excludes invalid paths but permits exact opaque IDs', () => {
    const list = [{key:'id',path:'../bad.md',aliases:['opaque-id']}];
    expect(resolve('bad.md','bar.md',list)).toMatchObject({reason:'missing'});
    expect(resolve('opaque-id','bar.md',list)).toMatchObject({key:'id'});
  });
});


it('RGI-005,008 rejects unsafe aliases and duplicate canonical paths before a unique alias', () => {
  for (const reference of ['bad%', '%FF', 'a%2fb', 'a%5Cb', 'a\\b', '../../escape.png']) {
    expect(resolve(reference,'captures/gallery.md',[{key:'alias',path:'other.png',aliases:[reference]}]).kind).toBe('unavailable');
  }
  expect(resolve('same.png','captures/gallery.md',[
    {key:'a',path:'captures/same.png'}, {key:'b',path:'captures/./same.png'},
    {key:'alias',path:'other.png',aliases:['same.png']},
  ])).toEqual({kind:'unavailable',reason:'ambiguous'});
});
