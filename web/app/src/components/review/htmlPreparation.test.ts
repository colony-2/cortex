import {afterEach,beforeEach,describe,it,expect,vi} from 'vitest';
import {prepareHTML,HTMLBudget,HTMLBudgetError,HTML_LIMITS,inspectRaster,embeddedRaster,srcsetCandidates} from './htmlPreparation';
import type {DocumentData} from './document';
import {png} from '../../../tests/helpers/raster';
const data=(text:string):DocumentData=>({text,bytes:new TextEncoder().encode(text),format:'html',truncated:false});
const signal=()=>new AbortController().signal;
const context={canonicalPath:'gallery/page.html',attachments:[{key:'blue',path:'gallery/blue.png',url:'/authorized/blue'}]};
beforeEach(()=>{vi.stubGlobal('createImageBitmap',vi.fn(async()=>({width:120,height:80,close:vi.fn()})));vi.stubGlobal('fetch',vi.fn(async()=>new Response(png())));});
afterEach(()=>vi.unstubAllGlobals());
describe('HP-005,007–010,020,021 native HTML preparation',()=>{
 it('preserves document envelope, doctype, script order/text, attributes and authored policy',async()=>{
  const source='<!doctype html><html lang="fr" data-root="yes"><head data-head="yes"><meta http-equiv="Content-Security-Policy" content="script-src \'none\'"><script>window.first="<img src=untouched>";</script></head><body data-body="yes"><p title="a &amp; b">Entity &amp; text</p><script>window.second=window.first</script></body></html>';
  const result=await prepareHTML(data(source),context,signal());const doc=new DOMParser().parseFromString(result.content,'text/html');
  expect(doc.doctype?.name).toBe('html');expect(doc.documentElement.lang).toBe('fr');expect(doc.head.dataset.head).toBe('yes');expect(doc.body.dataset.body).toBe('yes');
  expect([...doc.scripts].map(s=>s.textContent)).toEqual(['window.first="<img src=untouched>";','window.second=window.first']);expect(doc.querySelector('meta')?.getAttribute('content')).toBe("script-src 'none'");expect(doc.querySelector('p')?.title).toBe('a & b');expect(fetch).not.toHaveBeenCalled();
 });
 it('normalizes canonical paths and deduplicates img, srcset and picture reads',async()=>{
  const source='<picture><source srcset="./blue.png 1x, blue.png 2x"><img src="blue.png" srcset="blue.png 120w"></picture><img src="%62lue.png">';
  const result=await prepareHTML(data(source),context,signal());expect(fetch).toHaveBeenCalledTimes(1);expect(createImageBitmap).toHaveBeenCalledTimes(1);expect(result.imageCount).toBe(1);expect(result.warnings).toEqual([]);expect(result.content.match(/data:image\/png/g)).toHaveLength(5);
 });
 it.each(['../../blue.png','%2fblue.png','bad%zz','blue\\x.png','blue.png#fragment','missing.png','https://example.test/a.png','/api/a.png','data:image/svg+xml,bad'])('makes rejected reference inert: %s',async reference=>{
  const result=await prepareHTML(data(`<img src="${reference}">`),context,signal());expect(result.content).toContain('src="data:,"');expect(result.warnings).toHaveLength(1);expect(fetch).not.toHaveBeenCalled();
 });
 it('rejects ambiguous canonical paths without selecting an alias or fetching',async()=>{
  const result=await prepareHTML(data('<img src="blue.png">'),{...context,attachments:[...context.attachments,{key:'other',path:'gallery/blue.png',url:'/unauthorized'}]},signal());expect(result.warnings).toHaveLength(1);expect(fetch).not.toHaveBeenCalled();
 });
 it('rejects invalid canonical document identity instead of guessing the catalog root',async()=>{
  await expect(prepareHTML(data('<img src="blue.png">'),{...context,canonicalPath:'../page.html'},signal())).rejects.toThrow(/canonical artifact path/);expect(fetch).not.toHaveBeenCalled();
 });
 it('allows decoder orientation to swap axes without increasing the pixel budget',async()=>{
  vi.stubGlobal('createImageBitmap',vi.fn(async()=>({width:80,height:120,close:vi.fn()})));
  const result=await prepareHTML(data('<img src="blue.png">'),context,signal());expect(result.warnings).toEqual([]);
 });
 it('rejects base semantics before reading attachments',async()=>{await expect(prepareHTML(data('<base href="/x"><img src="blue.png">'),context,signal())).rejects.toThrow(/base URLs/);expect(fetch).not.toHaveBeenCalled();});
 it('validates embedded bytes and closes decoded rasters',async()=>{
  const close=vi.fn();vi.stubGlobal('createImageBitmap',vi.fn(async()=>({width:120,height:80,close})));
  const url='data:image/png;base64,'+png().toString('base64');const result=await prepareHTML(data(`<img src="${url}"><img src="${url}">`),context,signal());expect(result.imageCount).toBe(1);expect(result.warnings).toEqual([]);expect(close).toHaveBeenCalledOnce();expect(fetch).not.toHaveBeenCalled();
 });
 it('reports corrupt encoded bytes and decoder disagreement as named image warnings',async()=>{
  vi.stubGlobal('createImageBitmap',vi.fn(async()=>{throw new Error('Corrupt raster bytes')}));
  const result=await prepareHTML(data('<img src="blue.png">'),context,signal());expect(result.warnings[0]).toContain('blue.png: Corrupt raster');expect(result.content).toContain('data:,');
 });
 it('bounds streamed transport before decoding and does not follow redirects',async()=>{
  vi.stubGlobal('fetch',vi.fn(async()=>new Response(new Uint8Array(HTML_LIMITS.imageBytes+1))));const result=await prepareHTML(data('<img src="blue.png">'),context,signal());expect(result.warnings[0]).toMatch(/16 MiB/);expect(createImageBitmap).not.toHaveBeenCalled();expect(fetch).toHaveBeenCalledWith('/authorized/blue',expect.objectContaining({redirect:'error'}));
 });
 it('prepares with one active read/decoder and honors cancellation',async()=>{
  const controller=new AbortController();vi.stubGlobal('fetch',vi.fn(async(_url,options)=>{controller.abort();throw new DOMException('Aborted','AbortError')}));
  await expect(prepareHTML(data('<img src="blue.png">'),context,controller.signal)).rejects.toMatchObject({name:'AbortError'});expect(createImageBitmap).not.toHaveBeenCalled();
 });
 it.each([1048576,1048577])('inclusive complete byte limit %i',async n=>{const task=prepareHTML(data('x'.repeat(n)),undefined,signal());if(n===1048576)await expect(task).resolves.toHaveProperty('content');else await expect(task).rejects.toThrow(/limit/);});
 it.each([20000,20001])('inclusive complete line limit %i',async n=>{const task=prepareHTML(data('\n'.repeat(n-1)),undefined,signal());if(n===20000)await expect(task).resolves.toHaveProperty('content');else await expect(task).rejects.toThrow(/limit/);});
 it('never prepares incomplete input',async()=>{await expect(prepareHTML({...data('<p>partial'),truncated:true},undefined,signal())).rejects.toThrow(/incomplete/);expect(fetch).not.toHaveBeenCalled();});
 it('accepts exact budgets and rejects exceeded count, aggregate and expanded output',()=>{
  const count=new HTMLBudget();for(let i=0;i<32;i++)count.add(0);expect(()=>count.add(0)).toThrow(HTMLBudgetError);
  const bytes=new HTMLBudget();bytes.add(HTML_LIMITS.aggregateBytes);expect(bytes.bytes).toBe(HTML_LIMITS.aggregateBytes);expect(()=>bytes.add(1)).toThrow(HTMLBudgetError);
  const pixels=new HTMLBudget();expect(()=>pixels.addPixels(64_000_000)).not.toThrow();expect(()=>pixels.addPixels(1)).toThrow(HTMLBudgetError);
  const output=new HTMLBudget();expect(()=>output.output(HTML_LIMITS.outputBytes)).not.toThrow();expect(()=>output.output(HTML_LIMITS.outputBytes+1)).toThrow(HTMLBudgetError);
 });
 it('prevents frame preparation on actual aggregate count excess',async()=>{await expect(prepareHTML(data(Array.from({length:33},(_,i)=>`<img src="missing-${i}.png">`).join('')),context,signal())).rejects.toThrow(HTMLBudgetError);});
 it('bounds individual byte, dimensions and supported decoding formats before decode',()=>{
  expect(()=>inspectRaster(new Uint8Array(HTML_LIMITS.imageBytes+1))).toThrow();expect(()=>inspectRaster(new TextEncoder().encode('GIF89a'))).toThrow();
  const bytes=png();const v=new DataView(bytes.buffer,bytes.byteOffset,bytes.byteLength);v.setUint32(16,8000);v.setUint32(20,4000);expect(inspectRaster(bytes).width).toBe(8000);v.setUint32(16,8001);expect(()=>inspectRaster(bytes)).toThrow(/32 megapixel/);
  expect(()=>embeddedRaster('data:image/png;base64,Zh==')).toThrow();
 });
 it('tokenizes supported width/density srcsets including data commas',()=>{
  expect(srcsetCandidates('data:image/png;base64,AAAA 1x, other.png 2x')).toEqual([{reference:'data:image/png;base64,AAAA',descriptor:'1x'},{reference:'other.png',descriptor:'2x'}]);expect(srcsetCandidates('a.png, b.png 120w')).toHaveLength(2);expect(()=>srcsetCandidates('a.png bogus')).toThrow();
 });
});
