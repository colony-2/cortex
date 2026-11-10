import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { prepareMarkup, rasterData } from './markup';
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

it('preserves numeric SVG transforms while excluding resource paint', () => {
  const result=prepareMarkup(svg('<g transform="translate(10, 20) rotate(30)"><rect fill="data:image/svg+xml,bad"/></g>'));
  expect(result.content).toContain('translate(10, 20) rotate(30)');
  expect(result.content).not.toContain('data:');
});
