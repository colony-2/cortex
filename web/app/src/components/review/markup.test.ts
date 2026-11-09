import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { prepareMarkup, rasterData, HTML_POLICY } from './markup';
import type { DocumentData } from './document';
const data = (text: string, format: 'html' | 'svg' = 'html'): DocumentData => ({ text, bytes: new TextEncoder().encode(text), format, truncated: false });
const svg = (body: string, attrs = '') => data(`<svg xmlns="http://www.w3.org/2000/svg" ${attrs}>${body}</svg>`, 'svg');
const png = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=';
function pngHeader(w: number, h: number, size = 24) {
  const b = new Uint8Array(size); b.set([137,80,78,71,13,10,26,10]); const v=new DataView(b.buffer);v.setUint32(16,w);v.setUint32(20,h);
  let binary='';for(const n of b)binary+=String.fromCharCode(n);
  return 'data:image/png;base64,'+btoa(binary);
}
beforeEach(() => {
  const OriginalURL = URL;
  vi.stubGlobal('URL',class extends OriginalURL { static createObjectURL=vi.fn(() => 'blob:validated'); static revokeObjectURL=vi.fn(); });
});
afterEach(() => vi.unstubAllGlobals());
describe('HSV-003,004 exact rendering boundaries', () => {
  it.each([1048576,1048577])('complete %i byte HTML', n => {
    const d=data('x'.repeat(n));
    if(n===1048576)expect(prepareMarkup(d).content).toContain('xxx');
    else expect(() => prepareMarkup(d)).toThrow(/limit/);
  });
  it.each([20000,20001])('complete %i line HTML', n => {
    const d=data('\n'.repeat(n-1));
    if(n===20000)expect(prepareMarkup(d).content).toContain('Content-Security-Policy');
    else expect(() => prepareMarkup(d)).toThrow(/limit/);
  });
  it('rejects truncated valid-looking markup before parsing or URL allocation', () => {
    expect(() => prepareMarkup({...svg('<rect/>'),truncated:true})).toThrow(/incomplete/);
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });
});
describe('HSV-010,011 validated raster images', () => {
  it('accepts matching valid bounded PNG without parent blob transport', () => {
    expect(rasterData('data:image/png;base64,'+png).pixels).toBe(1);
    const result=prepareMarkup(data(`<img src="data:image/png;base64,${png}">`));
    expect(result.content).toContain(`src="data:image/png;base64,${png}"`);
    result.release();expect(URL.createObjectURL).not.toHaveBeenCalled();expect(URL.revokeObjectURL).not.toHaveBeenCalled();
  });
  it.each([
    'data:image/svg+xml;base64,PHN2Zy8+', 'data:text/html;base64,PGh0bWwvPg==',
    'data:image/jpeg;base64,'+png, 'data:image/png;base64,!!!!',
    'data:image/png;base64,a===', 'data:image/png;base64,Zh==',
    'data:image/png,not-base64', pngHeader(0,1),pngHeader(8001,4000),pngHeader(1,1,1048577),
  ])('rejects %s', value => expect(() => rasterData(value)).toThrow());
  it('enforces inclusive individual and aggregate byte/pixel bounds before URLs', () => {
    expect(rasterData(pngHeader(8000,4000)).pixels).toBe(32000000);
    expect(rasterData(pngHeader(1,1,1048576)).bytes.length).toBe(1048576);
    const result=prepareMarkup(data(`<img src="${pngHeader(4000,4000)}"><img src="${pngHeader(4000,4000)}"><img src="${pngHeader(1,1)}">`));
    expect(result.content.match(/src="data:image/g)).toHaveLength(2);
    expect(() => prepareMarkup(data(Array.from({length:3},()=>`<img src="${pngHeader(1,1,360000)}">`).join('')))).toThrow(/limit/);
  });
});
describe('HSV-006-009 filtering and policy', () => {
  it('keeps static styling/geometry but removes active surfaces and navigation', () => {
    const result=prepareMarkup(data(`<style>p{color:red}</style><p style="padding:12px">Hello</p><a href="https://evil.test" ping="/ping" download>Click</a><base href="/bad"><meta http-equiv="refresh" content="0;url=/bad"><script>parent.bad=1</script><iframe src="/bad"></iframe><form action="/bad"><input><button>Submit</button></form><object data="/bad"></object><svg onload="bad()"><rect width="10" height="10"/><foreignObject><img src="/bad"></foreignObject><animate/></svg>`));
    expect(result.content).toContain(HTML_POLICY);
    expect(result.content).toContain('padding:12px');expect(result.content).toContain('<rect');
    expect(result.content).not.toMatch(/<script|<form|<iframe|<input|<object|<base|onload|foreignObject|<animate|href=|ping=|download/);
  });
  it('removes encoded/namespaced URLs and CSS resource constructs', () => {
    const result=prepareMarkup(data(`<img src="&#104;ttps://evil.test/image"><svg><use xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="/bad"/></svg><div style="background:url(/bad)">x</div><style>@import '/bad';</style><p style="background:u\\72l('/bad')">y</p>`));
    expect(result.content).not.toContain('/bad');expect(result.content).not.toContain('evil.test');
  });
});
describe('HSV-012-015 SVG validation and complexity', () => {
  it('preserves viewBox, aspect, text, gradients and local references', () => {
    const result=prepareMarkup(svg('<defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient></defs><rect width="100" height="50" fill="url(#g)"/><text>Diagram</text>', 'viewBox="0 0 100 50"'));
    expect([result.width,result.height]).toEqual([100,50]);
    expect(result.content).toContain('viewBox="0 0 100 50"');expect(result.content).toContain('url(#g)');
  });
  it.each([
    data('<svg xmlns="http://www.w3.org/2000/svg"><g></svg>','svg'),
    data('<!DOCTYPE svg [<!ENTITY x "abc">]><svg xmlns="http://www.w3.org/2000/svg">&x;</svg>','svg'),
    svg('', 'width="-1"'),svg('', 'width="Infinity"'),svg('', 'width="10000" height="4000"'),
    svg('', 'viewBox="0 0 0 10"'), svg('', 'height="0"'),
    svg('<filter id="f"><feTurbulence/></filter>'),
    svg('<g id="a"><use href="#a"/></g>'),
    svg('<defs><g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g></defs>'),
    svg('<g>'.repeat(35)+'<rect/>'+'</g>'.repeat(35)),
    svg('<use href="#x"/>'.repeat(129)),svg('<rect/>'.repeat(2001)),
  ])('rejects invalid/expensive SVG before decoding %#', d => {
    expect(() => prepareMarkup(d)).toThrow();expect(URL.createObjectURL).not.toHaveBeenCalled();
  });
  it('removes external/active SVG content and accepts only validated raster data', () => {
    const result=prepareMarkup(svg(`<script>bad()</script><a href="/bad"><text>Link</text></a><image href="/bad"/><use href="/bad"/><rect onload="bad()" fill="url(/bad)"/><foreignObject/><image href="data:image/svg+xml;base64,PHN2Zy8+"/><image href="data:image/png;base64,${png}"/>`));
    expect(result.content).not.toMatch(/bad|script|foreignObject|image\/svg\+xml/);
    expect(result.content).toContain('data:image/png');
  });
});

describe('HSV-007,015 inline SVG shares complexity protection', () => {
  it('rejects inline cyclic references across SVG roots and invalid viewports', () => {
    expect(() => prepareMarkup(data('<svg><g id="a"><use href="#b"/></g></svg><svg><g id="b"><use href="#a"/></g></svg>'))).toThrow(/Cyclic/);
    expect(() => prepareMarkup(data('<svg width="999999999" height="1"><rect/></svg>'))).toThrow(/dimensions/);
  });
});

describe('HSV-024-026 canonical transport and conservative CSS', () => {
  it('regenerates lowercase raster sources in HTML and inline SVG without blobs', () => {
    const result=prepareMarkup(data(`<img src="DATA:IMAGE/PNG;base64,${png}" srcset="data:image/svg+xml,bad 1x"><svg width="20" height="20"><image href="DATA:IMAGE/PNG;base64,${png}"/></svg>`));
    expect(result.content.match(/data:image\/png;base64,/g)).toHaveLength(2);
    expect(result.content).not.toMatch(/DATA:|srcset|blob:/);
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });
  it.each([
    'background:url(data:image/svg+xml,bad)',
    'background:IMAGE-SET("data:image/svg+xml,bad" 1x)',
    'background:image("data:text/html,bad")',
    'background:cross-fade(url(data:image/png,bad),red,50%)',
    '--asset:url(data:image/svg+xml,bad);background:var(--asset)',
    'background:attr(title type(*))',
    'background:env(asset)',
    'background:u\\72l(data:image/png,bad)',
    'background:u/**/rl(data:image/png,bad)',
    'background:unknown(data)',
  ])('rejects resource/substitution syntax %s', css => {
    const result=prepareMarkup(data(`<p style='${css}'>safe</p><style>.test{${css}}</style>`));
    const parsed=new DOMParser().parseFromString(result.content,'text/html');
    expect(parsed.querySelector('p')?.hasAttribute('style')).toBe(false);
    expect(parsed.querySelector('style')?.textContent).toBe('');
  });
  it('preserves numeric SVG transforms while excluding resource-valued paint', () => {
    const result=prepareMarkup(svg('<g transform="translate(10, 20) rotate(30)"><rect fill="data:image/svg+xml,bad"/></g>'));
    expect(result.content).toContain('translate(10, 20) rotate(30)');
    expect(result.content).not.toContain('data:');
  });
  it('keeps supported neighboring layout, colors and numeric functions', () => {
    const result=prepareMarkup(data('<p style="padding:12px;background:url(bad);color:rgb(1, 2, 3);width:calc(100% - 10px)">safe</p><style>.card > p{display:flex;gap:4px;background:image-set(bad);color:red}</style>'));
    expect(result.content).toContain('padding:12px;color:rgb(1, 2, 3);width:calc(100% - 10px)');
    expect(result.content).toContain('display:flex;gap:4px;color:red');
    expect(result.content).not.toContain('bad');
  });
  it('drops alternate namespaces, resource attributes and malformed namespace transitions after reparse', () => {
    const result=prepareMarkup(data(`<svg><image xmlns:xlink="http://www.w3.org/1999/xlink" xlink:href="data:image/svg+xml,bad" href="data:text/html,bad"/><foreignObject><img src="data:image/svg+xml,bad"></foreignObject></svg><math><mtext><img src="data:text/html,bad"></mtext></math><picture><source srcset="data:image/svg+xml,bad"></picture><img srcset="data:image/svg+xml,bad" lowsrc="data:text/html,bad" background="data:text/html,bad">`));
    const reparsed=new DOMParser().parseFromString(result.content,'text/html');
    expect(reparsed.body.innerHTML).not.toMatch(/data:|srcset|xlink|foreignObject|math|lowsrc|background/);
  });
});
