import { describe, it, expect, vi, afterEach } from 'vitest';
import { classify, decodeText, loadDocument, TEXT_LIMIT, IMAGE_LIMIT, HEX_LIMIT, structuredAllowed, imageDimensions, hexDump, type DocumentData } from './document';
import { formatJSON, jsonLineErrors } from './structured';
import { parseDiff, alignDiff } from './diff';
const bytes = (s: string) => new TextEncoder().encode(s);
const data = (text: string): DocumentData => ({ text, bytes: bytes(text), format: 'json', truncated: false });
afterEach(() => vi.unstubAllGlobals());
describe('V01-V03 inventory dispatch and encoding', () => {
  it.each([['a.md','markdown'],['a.json','json'],['a.log','source'],['a.diff','diff'],['a.patch','diff'],['a.html','html'],['a.svg','source'],['a.yaml','source'],['a.py','source'],['a.go','source'],['go.mod','source'],['go.sum','source'],['a.sh','source'],['a.txt','source'],['a.unknown','source'],['a.jsonl','jsonl'],['a.ndjson','jsonl'],['a.tar.gz','container'],['__git_state_thin_pack__','container']])('%s → %s',(name,format) => expect(classify(name,bytes('sample'))).toBe(format));
  it('recognizes binary/container signatures regardless of suffix', () => {
    expect(classify('misleading.md',new Uint8Array([31,139,0]))).toBe('container');
    expect(classify('misleading.json',new Uint8Array([137,80,78,71]))).toBe('image');
    expect(classify('misleading.md',new Uint8Array([0,1,2]))).toBe('binary');
    expect(classify('unknown',bytes('--- a/x\n+++ b/x\n'))).toBe('diff');
  });
  it('preserves BOM UTF-16 and Unicode/CRLF', () => {
    expect(decodeText(new Uint8Array([255,254,65,0,13,0,10,0]))).toBe('A\r\n');
    expect(decodeText(new Uint8Array([254,255,0,65]))).toBe('A');
    expect(decodeText(bytes('雪\r\n '))).toBe('雪\r\n ');
  });
});
describe('J01-J04 lossless structured text', () => {
  it('preserves tokens, duplicates and ordering', () => {
    const s = '{"a":9007199254740993,"a":1e+40,"z":"\\u96ea"}';
    const f = formatJSON(s); expect(f.replace(/\s/g,'')).toBe(s); expect(f).toContain('9007199254740993');
  });
  it('rejects invalid and excessively nested inputs', () => {
    expect(() => formatJSON('{broken')).toThrow();
    expect(() => formatJSON('['.repeat(101)+'0'+']'.repeat(101))).toThrow(/nesting/);
  });
  it('identifies only malformed JSONL records', () => expect(jsonLineErrors('{"x":1}\nbad\n{"y":2}')).toEqual(['Line 2: invalid JSON']));
});
const patch = 'diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -2,2 +2,3 @@\n context\n-old\n+new\n+extra\n\\ No newline at end of file\n';
describe('D02-D06 patch semantics', () => {
  it('numbers partial hunks and aligns replacements with blanks', () => {
    const files = parseDiff(patch); expect(files).toHaveLength(1);
    const rows = alignDiff(files[0].lines); const replacement = rows.find(r => r.left?.text === '-old');
    expect(replacement?.left?.old).toBe(3); expect(replacement?.right?.next).toBe(3);
    expect(rows.find(r => r.right?.text === '+extra')?.left).toBeUndefined();
  });
  it.each([[true,false],[false,true],[true,true]])('aligns replacements with old=%s/new=%s newline markers', (oldMarker,newMarker) => {
    const marker = '\\ No newline at end of file';
    const text = ['--- a/file','+++ b/file','@@ -1 +1 @@','-old',...(oldMarker ? [marker] : []),'+new',...(newMarker ? [marker] : [])].join('\n');
    const lines = parseDiff(text)[0].lines;
    const rows = alignDiff(lines);
    const index = rows.findIndex(r => r.left?.text === '-old');
    expect(rows[index]).toMatchObject({left:{text:'-old',old:1},right:{text:'+new',next:1}});
    expect(rows[index+1].left?.text).toBe(oldMarker ? marker : undefined);
    expect(rows[index+1].right?.text).toBe(newMarker ? marker : undefined);
    expect(lines.map(l => l.text).join('\n')).toBe(text);
  });
  it('preserves both-side annotations on unchanged context', () => {
    const rows = alignDiff(parseDiff('--- a/f\n+++ b/f\n@@ -1 +1 @@\n same\n\\ No newline at end of file')[0].lines);
    expect(rows.at(-1)?.left?.newline).toBe('both');
    expect(rows.at(-1)?.right?.newline).toBe('both');
  });
  it('retains multiple files, renames, mode and binary metadata', () => {
    const files = parseDiff(patch+'diff --git a/雪 b/雪\nold mode 100644\nnew mode 100755\nrename from 雪\nrename to rain\nBinary files a/雪 and b/rain differ\n');
    expect(files).toHaveLength(2); expect(files[1].lines.map(l => l.text).join('\n')).toContain('rename to rain');
  });
  it('supports additions/deletions, CRLF and multiple hunks', () => {
    expect(parseDiff('--- /dev/null\r\n+++ b/new\r\n@@ -0,0 +1 @@\r\n+x\r\n')[0].lines.slice(-1)[0]?.next).toBe(1);
    expect(parseDiff('--- a/old\n+++ /dev/null\n@@ -1 +0,0 @@\n-x\n')[0].lines.slice(-1)[0]?.old).toBe(1);
    expect(parseDiff(patch+'@@ -10 +11 @@\n last\n')[0].lines.slice(-1)[0]?.next).toBe(11);
  });
  it.each(['diff --cc a\n@@@ x','--- a\n+++ b\n@@ broken','--- a\n+++ b\n@@ -1,2 +1,2 @@\n x','--- a\n+++ b\n@@ -1 +1 @@\n x\n+excess'])('rejects unsupported or invalid patch %s', s => expect(() => parseDiff(s)).toThrow());
  it('separates traditional unified files after completed hunks', () => {
    const files = parseDiff('--- a/first\n+++ b/first\n@@ -1 +1 @@\n first\n--- a/second\n+++ b/second\n@@ -4 +8 @@\n second\n');
    expect(files).toHaveLength(2);expect(files[1].name).toBe('a/second');expect(files[1].lines.slice(-1)[0].next).toBe(8);
  });
  it('accepts empty patches', () => expect(parseDiff('')).toEqual([]));
});
describe('L04-L10 bounded loading and dimension inspection', () => {
  const stream = (content: Uint8Array) => {
    const cancel = vi.fn(); let pos = 0;
    vi.stubGlobal('fetch',vi.fn(async () => new Response(new ReadableStream({ pull(controller) { if (pos >= content.length) controller.close(); else { controller.enqueue(content.slice(pos,pos+8192)); pos+=8192; } },cancel }))));
    return cancel;
  };
  it.each([TEXT_LIMIT-1,TEXT_LIMIT,TEXT_LIMIT+1])('bounds text at %i bytes', async n => {
    const cancel = stream(new Uint8Array(n).fill(65)); const d = await loadDocument('/text','a.txt',new AbortController().signal);
    expect(d.bytes.length).toBe(Math.min(n,TEXT_LIMIT)); expect(d.truncated).toBe(n>TEXT_LIMIT); if(n<=TEXT_LIMIT) expect(cancel).not.toHaveBeenCalled();
  });
  it('bounds binary previews and hex offsets', async () => {
    stream(new Uint8Array(HEX_LIMIT+1)); const d = await loadDocument('/binary','a.bin',new AbortController().signal);
    expect(d.bytes.length).toBe(HEX_LIMIT); expect(d.truncated).toBe(true); expect(hexDump(new Uint8Array([65,0]))).toContain('A.');
  });
  it('rejects oversized raster streams', async () => {
    const b = new Uint8Array(IMAGE_LIMIT+1); b.set([137,80,78,71]); stream(b);
    await expect(loadDocument('/image','a.png',new AbortController().signal)).rejects.toThrow(/16 MiB/);
  });
  it('enforces complete structured byte and line boundaries', () => {
    expect(structuredAllowed(data('a'.repeat(1024*1024)))).toBe(true);
    expect(structuredAllowed(data('a'.repeat(1024*1024+1)))).toBe(false);
    expect(structuredAllowed(data('\n'.repeat(19999)))).toBe(true);
    expect(structuredAllowed(data('\n'.repeat(20000)))).toBe(false);
    expect(structuredAllowed({...data('{}'),truncated:true})).toBe(false);
  });
  it('checks raster dimensions before decoding', () => {
    const b = new Uint8Array(32); const v = new DataView(b.buffer); v.setUint32(16,8000); v.setUint32(20,4000);
    expect(imageDimensions(b,'image/png')).toEqual([8000,4000]); v.setUint32(20,4001);
    expect(() => imageDimensions(b,'image/png')).toThrow(/32 megapixel/);
    expect(() => imageDimensions(new Uint8Array(0),'image/png')).toThrow();
    expect(() => imageDimensions(b,'image/unknown')).toThrow(/unsupported/);
  });
});

describe('I02 L09 raster header variants', () => {
  it.each(['image/gif','image/bmp','image/jpeg','image/webp','image/avif'])('inspects %s before decode', mime => {
    const b=new Uint8Array(100),v=new DataView(b.buffer); const put=(at:number,text:string)=>b.set(new TextEncoder().encode(text),at);
    if(mime==='image/gif'){v.setUint16(6,2,true);v.setUint16(8,3,true);}
    if(mime==='image/bmp'){v.setInt32(18,2,true);v.setInt32(22,-3,true);}
    if(mime==='image/jpeg'){b.set([255,216,255,192,0,17,8,0,3,0,2]);}
    if(mime==='image/webp'){put(12,'VP8X');b[24]=1;b[27]=2;}
    if(mime==='image/avif'){v.setUint32(0,20);put(4,'ispe');v.setUint32(12,2);v.setUint32(16,3);}
    expect(imageDimensions(b,mime)).toEqual([2,3]);
  });
  it('cancels an unfinished stream when the text ceiling is reached', async () => {
    const cancel=vi.fn();let delivered=false;
    vi.stubGlobal('fetch',vi.fn(async()=>new Response(new ReadableStream({pull(c){if(!delivered){c.enqueue(new Uint8Array(TEXT_LIMIT+100).fill(65));delivered=true;}},cancel}))));
    const d=await loadDocument('/large','a.txt',new AbortController().signal);expect(d.truncated).toBe(true);expect(cancel).toHaveBeenCalledOnce();
  });
  it('cancels pending reads after abort', async () => {
    const cancel=vi.fn();vi.stubGlobal('fetch',vi.fn(async()=>new Response(new ReadableStream({cancel}))));
    const controller=new AbortController();const promise=loadDocument('/pending','a.txt',controller.signal);
    await new Promise(resolve=>setTimeout(resolve,0));controller.abort();await expect(promise).rejects.toThrow(/Aborted/);expect(cancel).toHaveBeenCalled();
  });
});


describe('HSV-001,002 markup dispatch', () => {
  it.each([
    ['PAGE.HTML','<p>fragment</p>','html'],['page.HTM','hello','html'],
    ['a.SVG','<svg xmlns="http://www.w3.org/2000/svg"/>','svg'],
    ['unknown','<?xml version="1.0"?>\n<svg/>','svg'],
    ['unknown','<!DOCTYPE html><html/>','html'],['unknown','<html lang="en">','html'],
    ['wrong.html','<svg/>','source'],['wrong.svg','<html/>','source'],
    ['unknown','<svg-not-really/>','source'],['unknown','<p>ambiguous</p>','source'],
    ['invalid.svg','arbitrary','source'],['unknown','<?xml version="1.0"?><root/>','source'],
  ])('%s %s selects %s', (name,text,format) => expect(classify(name,bytes(text))).toBe(format));
  it('keeps binary and raster priority for markup suffixes', () => {
    expect(classify('a.html',new Uint8Array([255,0,128]))).toBe('binary');
    expect(classify('a.svg',new Uint8Array([137,80,78,71]))).toBe('image');
  });
});
