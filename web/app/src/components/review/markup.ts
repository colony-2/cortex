import { imageDimensions, rasterType, structuredAllowed, type DocumentData } from './document';

export const RESTRICTIONS = 'Static preview: scripts, navigation and external dependencies are disabled. Some content may be missing.';
const svgNS = 'http://www.w3.org/2000/svg';
const svgTags = new Set('svg g defs symbol use path rect circle ellipse line polyline polygon text tspan textPath linearGradient radialGradient stop clipPath mask pattern title desc image'.toLowerCase().split(' '));
const attributes = new Set('id class title lang dir style role aria-label width height colspan rowspan scope start reversed open alt viewbox preserveaspectratio x y x1 x2 y1 y2 cx cy r rx ry dx dy d points transform fill fill-opacity fill-rule stroke stroke-width stroke-opacity stroke-linecap stroke-linejoin stroke-dasharray stroke-dashoffset opacity font-family font-size font-weight font-style text-anchor dominant-baseline offset stop-color stop-opacity gradientunits gradienttransform spreadmethod patternunits patterncontentunits patterntransform clip-path clip-rule mask maskunits maskcontentunits textlength lengthadjust'.split(' '));

export interface MarkupPreview { content: string; width: number; height: number; release: () => void; embeddedImages: number }

/** Only strict base64 raster data is accepted. Inspect dimensions before browser decoding. */
export function rasterData(value: string): { bytes: Uint8Array; mime: string; pixels: number } {
  const match = /^data:(image\/(?:png|jpeg|gif|webp|bmp|avif));base64,([A-Za-z0-9+/]*={0,2})$/i.exec(value);
  if (!match || !match[2] || match[2].length % 4 || match[2].length > Math.ceil(1048576 / 3) * 4) throw new Error('Unsupported or oversized embedded image.');
  const decoded = atob(match[2]);
  // Reject noncanonical padding and unused bits as well as invalid alphabet/encoding.
  if (btoa(decoded) !== match[2]) throw new Error('Invalid embedded image encoding.');
  const bytes = Uint8Array.from(decoded, c => c.charCodeAt(0));
  const mime = match[1].toLowerCase();
  if (bytes.length > 1048576 || rasterType(bytes) !== mime || (mime === 'image/png' && ![137,80,78,71,13,10,26,10].every((n, i) => bytes[i] === n))) throw new Error('Embedded image type or size is invalid.');
  const [w, h] = imageDimensions(bytes, mime);
  return { bytes, mime, pixels: w * h };
}

// This subset deliberately has no resource-valued properties, custom properties,
// strings, escapes or substitutions. Unknown syntax is discarded before srcdoc.
const cssProperties = new Set(('color background background-color width height min-width max-width min-height max-height margin margin-top margin-right margin-bottom margin-left padding padding-top padding-right padding-bottom padding-left border border-width border-style border-color border-radius border-top border-right border-bottom border-left box-sizing display position top right bottom left inset z-index overflow overflow-x overflow-y opacity visibility font-size font-weight font-style font-family line-height letter-spacing word-spacing text-align text-decoration text-transform white-space word-break overflow-wrap vertical-align float clear flex flex-direction flex-wrap flex-grow flex-shrink flex-basis align-items align-self align-content justify-content justify-items gap row-gap column-gap grid-template-columns grid-template-rows grid-column grid-row order list-style-type border-collapse border-spacing table-layout').split(' '));
const cssFunctions = new Set('rgb rgba hsl hsla calc min max clamp repeat'.split(' '));
const localPaint = new Set('fill stroke clip-path mask'.split(' '));
function safeValue(value: string): string {
  if (!/^[a-zA-Z0-9_\s#%.,()+*/-]+$/.test(value) || /\/\*|\*\//.test(value)) return '';
  let depth = 0;
  for (const c of value) {
    if (c === '(') depth++;
    if (c === ')' && --depth < 0) return '';
    if (depth > 8) return '';
  }
  if (depth || /(?:^|[^\w-])(?:var|env|attr|url|image|image-set|cross-fade|expression)\s*\(/i.test(value)) return '';
  for (const m of value.matchAll(/([\w-]+)\s*\(/g)) if (!cssFunctions.has(m[1].toLowerCase())) return '';
  return value;
}
function declarations(value: string): string {
  // Quotes, comments and escapes can obscure declaration boundaries: reject the
  // whole list rather than interpreting a different grammar from the browser.
  if (/[\\@<>{}"']|\/\*|\*\//.test(value)) return '';
  return value.split(';').flatMap(declaration => {
    const match = /^\s*([a-z-]+)\s*:\s*(.*?)\s*$/i.exec(declaration);
    if (!match || !cssProperties.has(match[1].toLowerCase())) return [];
    const safe = safeValue(match[2]);
    return safe ? [`${match[1].toLowerCase()}:${safe}`] : [];
  }).join(';');
}
function stylesheet(value: string): string {
  if (/[\\@<"']|\/\*|\*\//.test(value)) return '';
  let result = '', end = 0;
  const rule = /([^{}]+)\{([^{}]*)\}/g;
  for (const match of value.matchAll(rule)) {
    if (value.slice(end, match.index).trim()) return '';
    const selector = match[1].trim();
    // Simple type/class/ID selectors and their ordinary combinators only.
    if (!/^[a-zA-Z0-9_.*#, >+~-]+$/.test(selector)) return '';
    const safe = declarations(match[2]);
    if (safe) result += `${selector}{${safe}}`;
    end = match.index! + match[0].length;
  }
  return value.slice(end).trim() ? '' : result;
}
function presentation(value: string, key: string, svg: boolean): string {
  if (svg && localPaint.has(key)) {
    return /^url\(\s*#[a-zA-Z_][\w.-]*\s*\)$/.test(value) ? value : safeValue(value);
  }
  if (svg && ['transform', 'gradienttransform', 'patterntransform'].includes(key)) {
    return /^(?:\s*(?:matrix|translate|scale|rotate|skewX|skewY)\(\s*[-+0-9.eE,\s]+\)\s*)+$/.test(value) ? value : '';
  }
  // Geometry, accessibility and text attributes are not CSS declarations, but
  // resource functions/escapes must never survive in presentation attributes.
  return /[\\@<>]|url\s*\(|[\w-]+\s*\(/i.test(value) ? safeValue(value) : value;
}

function svgDimensions(root: Element): [number, number] {
  const dimension = (name: string): number | undefined => {
    const value = root.getAttribute(name);
    if (value === null) return;
    if (!/^(?:\d+(?:\.\d*)?|\.\d+)(?:px)?$/.test(value)) throw new Error('Invalid SVG dimensions.');
    const n = Number(value.replace(/px$/, ''));
    if (!Number.isFinite(n) || n <= 0 || n > 32000) throw new Error('Invalid SVG dimensions.');
    return n;
  };
  let w = dimension('width'), h = dimension('height');
  const viewBox = root.getAttribute('viewBox');
  if (viewBox !== null) {
    const values = viewBox.trim().split(/[\s,]+/).map(Number);
    if (values.length !== 4 || values.some(n => !Number.isFinite(n) || Math.abs(n) > 1e7) || values[2] <= 0 || values[3] <= 0) throw new Error('Invalid SVG viewBox.');
    if (w === undefined && h !== undefined) w = h * values[2] / values[3];
    if (h === undefined && w !== undefined) h = w * values[3] / values[2];
    if (w === undefined && h === undefined) { w = Math.min(1024, values[2]); h = w * values[3] / values[2]; }
  }
  w ??= 800; h ??= 600;
  if (w <= 0 || h <= 0 || w > 32000 || h > 32000 || w * h > 32000000) throw new Error('SVG exceeds the bounded viewport or 32 megapixel preview limit.');
  return [w, h];
}

/** Bound the expansion graph, including containment, use, paint and CSS references. */
function checkReferences(root: Element) {
  const nodes = [root, ...root.querySelectorAll('*')];
  if (nodes.length > 2000) throw new Error('SVG complexity limit exceeded.');
  const ids = new Map<string, Element>();
  for (const el of nodes) { if (el.id) { if (ids.has(el.id)) throw new Error('Ambiguous SVG references.'); ids.set(el.id, el); } }
  const edges = new Map<Element, Element[]>();
  let references = 0;
  for (const el of nodes) {
    const children = [...el.children];
    for (const attr of [...el.attributes]) {
      const targets = attr.name === 'href' ? [attr.value.slice(1)] : [...attr.value.matchAll(/url\(\s*#([\w.-]+)\s*\)/g)].map(m => m[1]);
      for (const target of targets) {
        if (++references > 128) throw new Error('SVG reference limit exceeded.');
        const node = ids.get(target);
        if (node) children.push(node);
      }
    }
    edges.set(el, children);
  }
  let cost = 0;
  const visit = (node: Element, path: Set<Element>, depth: number) => {
    if (path.has(node) || depth > 32 || ++cost > 4096) throw new Error('Cyclic, deep or excessive SVG references.');
    path.add(node);
    for (const child of edges.get(node) || []) visit(child, path, depth + 1);
    path.delete(node);
  };
  visit(root, new Set(), 0);
}

/** Standalone SVG keeps its existing static filtering and complexity limits. */
export function prepareMarkup(data: DocumentData): MarkupPreview {
  if (!structuredAllowed(data)) throw new Error('Rendering limit exceeded or input incomplete (1 MiB / 20,000 lines); showing bounded source.');
  if (data.format !== 'svg') throw new Error('HTML uses the interactive document renderer.');
  if (typeof DOMParser === 'undefined' || typeof URL.createObjectURL !== 'function') throw new Error('Rendering unavailable in this browser; showing source.');
  if (/<!\s*(?:DOCTYPE|ENTITY)/i.test(data.text)) throw new Error('SVG DTDs and entities are unsupported.');
  const doc = new DOMParser().parseFromString(data.text, 'image/svg+xml');
  const root = doc.documentElement;
  if (root.localName !== 'svg' || root.namespaceURI !== svgNS || doc.querySelector('parsererror')) throw new Error('Invalid SVG XML; showing source.');
  const [width, height] = svgDimensions(root);
  if (root.querySelector('filter') || root.querySelector('[filter]')) throw new Error('SVG filters are unsupported; showing source.');
  let imageBytes = 0, imagePixels = 0, count = 0;
  const clean = (el: Element, depth: number) => {
    if (++count > 4000 || depth > 64) throw new Error('Markup complexity limit exceeded.');
    const tag = el.localName.toLowerCase();
    if (el.namespaceURI !== svgNS || !svgTags.has(tag)) { el.remove(); return; }
    for (const attr of [...el.attributes]) {
      const key = attr.name.toLowerCase(), value = attr.value;
      el.removeAttributeNode(attr);
      if (attr.namespaceURI || key.includes(':')) continue;
      if (key === 'href' && tag === 'image') {
        try {
          const image = rasterData(value);
          imageBytes += image.bytes.length; imagePixels += image.pixels;
          if (imageBytes > 1048576 || imagePixels > 32000000) throw new Error('Embedded image aggregate limit exceeded.');
          const binary = Array.from(image.bytes, byte => String.fromCharCode(byte)).join('');
          el.setAttribute(key, `data:${image.mime};base64,${btoa(binary)}`);
        } catch { /* Invalid images stay omitted from the static SVG copy. */ }
      } else if (key === 'href' && ['use', 'textpath'].includes(tag) && /^#[a-zA-Z_][\w.-]*$/.test(value)) el.setAttribute('href', value);
      else if (attributes.has(key)) {
        const safe = key === 'style' ? declarations(value) : presentation(value, key, true);
        if (safe) el.setAttribute(attr.name, safe);
      }
    }
    if (tag === 'style') el.textContent = stylesheet(el.textContent || '');
    for (const child of [...el.children]) clean(child, depth + 1);
  };
  clean(root, 0);
  checkReferences(root);
  root.setAttribute('xmlns', svgNS);
  root.setAttribute('width', String(width)); root.setAttribute('height', String(height));
  return {content: new XMLSerializer().serializeToString(root), width, height, release: () => {}, embeddedImages: imageBytes};
}
