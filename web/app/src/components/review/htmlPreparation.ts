import { imageDimensions, rasterType, structuredAllowed, type DocumentData } from './document';
import { resolveArtifactLink } from './artifactLinks';

export interface HTMLAttachment { key: string; path: string; url: string }
export interface HTMLContext { canonicalPath: string; attachments: HTMLAttachment[] }
export const HTML_LIMITS = Object.freeze({ images: 32, imageBytes: 16 * 1024 * 1024, pixels: 32_000_000, aggregatePixels: 64_000_000, aggregateBytes: 32 * 1024 * 1024, outputBytes: 48 * 1024 * 1024, concurrency: 1 });
export const INTERACTIVE_POLICY = "default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval' data: blob: http: https:; style-src 'unsafe-inline' http: https:; img-src data:; font-src data: http: https:; connect-src 'none'; frame-src 'none'; object-src 'none'; worker-src 'none'; base-uri 'none'; form-action 'none'";
export const HTML_NOTICE = 'Interactive HTML runs automatically. Public CSS and scripts can contact their hosts and transmit document content. Connections, nested pages and non-embedded images are blocked. Source and original Download remain available.';
export const HTML_PERMISSIONS = 'camera \'none\'; microphone \'none\'; geolocation \'none\'; display-capture \'none\'; clipboard-read \'none\'; clipboard-write \'none\'; payment \'none\'; usb \'none\'; serial \'none\'; hid \'none\'; bluetooth \'none\'; fullscreen \'none\'';
export function nativeHTMLSupported() { return 'csp' in HTMLIFrameElement.prototype && 'credentialless' in HTMLIFrameElement.prototype && typeof createImageBitmap === 'function'; }
export class HTMLBudgetError extends Error {}
export class HTMLBudget {
  images = 0; bytes = 0; pixels = 0;
  add(bytes: number) {
    if (++this.images > HTML_LIMITS.images || (this.bytes += bytes) > HTML_LIMITS.aggregateBytes) throw new HTMLBudgetError('HTML image aggregate limit exceeded (32 images / 32 MiB).');
  }
  addPixels(pixels: number) { if ((this.pixels += pixels) > HTML_LIMITS.aggregatePixels) throw new HTMLBudgetError('HTML images exceed the 64 megapixel aggregate limit.'); }
  output(bytes: number) { if (bytes > HTML_LIMITS.outputBytes) throw new HTMLBudgetError('Expanded HTML exceeds the 48 MiB preview limit.'); }
}
function cancelled(signal: AbortSignal) { if (signal.aborted) throw new DOMException('Preparation cancelled', 'AbortError'); }
async function readImage(url: string, signal: AbortSignal): Promise<Uint8Array> {
  const response = await fetch(url, { signal, redirect: 'error' });
  if (!response.ok || !response.body) throw new Error(`Attachment unavailable (${response.status}).`);
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = []; let size = 0;
  try {
    while (true) {
      cancelled(signal);
      const { done, value } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > HTML_LIMITS.imageBytes) throw new Error('Image exceeds 16 MiB.');
      chunks.push(value);
    }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
  const bytes = new Uint8Array(size); let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
  return bytes;
}
export function embeddedRaster(value: string): Uint8Array {
  const m = /^data:(image\/(?:png|jpeg));base64,([A-Za-z0-9+/]+={0,2})$/i.exec(value);
  if (!m || m[2].length > Math.ceil(HTML_LIMITS.imageBytes / 3) * 4 || m[2].length % 4) throw new Error('Only bounded base64 PNG/JPEG images are supported.');
  const binary = atob(m[2]);
  if (btoa(binary) !== m[2]) throw new Error('Invalid base64 image encoding.');
  const bytes = Uint8Array.from(binary, c => c.charCodeAt(0));
  if (rasterType(bytes) !== m[1].toLowerCase()) throw new Error('Embedded image MIME does not match its bytes.');
  return bytes;
}
export function inspectRaster(bytes: Uint8Array): { mime: string; width: number; height: number } {
  const mime = rasterType(bytes);
  if (!bytes.length || bytes.length > HTML_LIMITS.imageBytes || !['image/png', 'image/jpeg'].includes(mime || '')) throw new Error('Only PNG/JPEG raster images up to 16 MiB are supported.');
  if (mime === 'image/png') {
    if (![137,80,78,71,13,10,26,10].every((n,i) => bytes[i] === n)) throw new Error('Invalid PNG signature.');
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    for (let p = 8; p + 12 <= bytes.length;) {
      const size = view.getUint32(p);
      const tag = String.fromCharCode(...bytes.subarray(p + 4, p + 8));
      if (tag === 'acTL') throw new Error('Animated images are unsupported.');
      if (p + size + 12 > bytes.length) throw new Error('Incomplete PNG chunk.');
      p += size + 12;
    }
  }
  const [width, height] = imageDimensions(bytes, mime!);
  return { mime: mime!, width, height };
}
function dataURL(bytes: Uint8Array, mime: string) {
  let binary = '';
  for (let p = 0; p < bytes.length; p += 16384) binary += String.fromCharCode(...bytes.subarray(p, p + 16384));
  return `data:${mime};base64,${btoa(binary)}`;
}
/** Srcset URL tokens may contain commas (notably data URLs); descriptors may not. */
export function srcsetCandidates(input: string): { reference: string; descriptor: string }[] {
  const result: { reference: string; descriptor: string }[] = [];
  let p = 0;
  while (p < input.length) {
    while (/[\t\n\f\r ,]/.test(input[p] || '') && p < input.length) p++;
    const start = p;
    while (p < input.length && !/[\t\n\f\r ]/.test(input[p])) p++;
    let reference = input.slice(start, p);
    if (!reference) break;
    if (reference.endsWith(',')) { result.push({reference: reference.replace(/,+$/, ''), descriptor: ''}); continue; }
    const d = p;
    while (p < input.length && input[p] !== ',') p++;
    const descriptor = input.slice(d, p).trim(); p++;
    if (descriptor && !/^(?:[1-9]\d*w|(?:\d+(?:\.\d+)?|\.\d+)x)$/.test(descriptor)) throw new Error('Unsupported srcset descriptor.');
    result.push({reference, descriptor});
  }
  return result;
}
export interface PreparedHTML { content: string; warnings: string[]; imageCount: number; imageBytes: number; outputBytes: number }
/** Native detached-document parsing is inert in the supported, browser-tested Chromium boundary. */
export async function prepareHTML(data: DocumentData, context: HTMLContext | undefined, signal: AbortSignal): Promise<PreparedHTML> {
  if (!structuredAllowed(data)) throw new Error('Rendering limit exceeded or input incomplete (1 MiB / 20,000 lines); showing bounded source.');
  cancelled(signal);
  const canonical = (path: string) => !!path && !/^[\/]|[\\\u0000-\u001f\u007f]/.test(path) && !path.split('/').some(part => part === '..' || part === '.');
  if (context && !canonical(context.canonicalPath)) throw new Error('Document has no supported canonical artifact path.');
  const attachments = context?.attachments.filter(attachment => canonical(attachment.path)) || [];
  const doc = new DOMParser().parseFromString(data.text, 'text/html');
  if (doc.querySelector('base[href]')) throw new Error('Authored base URLs are unsupported; use absolute public dependencies and canonical attachment paths.');
  const budget = new HTMLBudget();
  const warnings: string[] = [];
  const cache = new Map<string, Promise<string>>();
  const warn = (reference: string, error: unknown) => { warnings.push(`${reference.slice(0, 120)}: ${(error as Error).message}`); return 'data:,'; };
  const resolve = async (reference: string): Promise<string> => {
    const embedded = /^data:/i.test(reference);
    const resolution = !embedded && !reference.includes('?') && context ? resolveArtifactLink(reference, context.canonicalPath, attachments) : undefined;
    const attachment = resolution && resolution.kind === 'target' ? attachments.find(a => a.key === resolution.key) : undefined;
    const key = embedded ? `embedded:${reference}` : attachment ? `attachment:${attachment.key}` : `missing:${reference}`;
    const cached = cache.get(key); if (cached) return cached;
    // Count rejected references too, to bound warnings and work on hostile documents.
    budget.add(0);
    const work = (async () => {
      try {
        cancelled(signal);
        if (!embedded && !attachment) throw new Error('Missing, ambiguous, invalid or excluded local raster reference.');
        if (resolution?.kind === 'target' && resolution.fragment !== undefined) throw new Error('Image fragments are unsupported.');
        const bytes = embedded ? embeddedRaster(reference) : await readImage(attachment!.url, signal);
        const image = inspectRaster(bytes);
        budget.bytes += bytes.length;
        if (budget.bytes > HTML_LIMITS.aggregateBytes) throw new HTMLBudgetError('HTML image aggregate exceeds 32 MiB.');
        budget.addPixels(image.width * image.height);
        const decoded = await createImageBitmap(new Blob([bytes as BlobPart], {type: image.mime}));
        try { if (!((decoded.width === image.width && decoded.height === image.height) || (decoded.width === image.height && decoded.height === image.width))) throw new Error('Image decoder dimensions disagree.'); }
        finally { decoded.close(); }
        cancelled(signal);
        return dataURL(bytes, image.mime);
      } catch (error) {
        if (error instanceof HTMLBudgetError || signal.aborted) throw error;
        return warn(embedded ? '(embedded image)' : reference, error);
      }
    })();
    cache.set(key, work); return work;
  };
  let expanded = new TextEncoder().encode(data.text).length;
  const replace = (element: Element, attribute: string, value: string) => {
    const old = element.getAttribute(attribute) || '';
    expanded += new TextEncoder().encode(value).length - new TextEncoder().encode(old).length;
    budget.output(expanded); // Bound repeated-reference expansion before inserting into the detached DOM.
    element.setAttribute(attribute, value);
  };
  // Sequential preparation bounds both transport and bitmap decoder concurrency to one.
  for (const element of doc.querySelectorAll('img, picture source')) {
    if (element.hasAttribute('src') && element.localName === 'img') replace(element, 'src', await resolve(element.getAttribute('src')!));
    if (element.hasAttribute('srcset')) {
      const original = element.getAttribute('srcset')!;
      try {
        const values = []; let srcsetLength = 0;
        for (const candidate of srcsetCandidates(original)) {
          const value = `${await resolve(candidate.reference)}${candidate.descriptor ? ` ${candidate.descriptor}` : ''}`;
          srcsetLength += value.length + (values.length ? 2 : 0);
          budget.output(expanded - original.length + srcsetLength);
          values.push(value);
        }
        replace(element, 'srcset', values.join(', '));
      } catch (error) {
        if (error instanceof HTMLBudgetError || signal.aborted) throw error;
        element.setAttribute('srcset', warn('srcset', error));
      }
    }
  }
  cancelled(signal);
  const content = (doc.doctype ? new XMLSerializer().serializeToString(doc.doctype) : '') + doc.documentElement.outerHTML;
  const outputBytes = new TextEncoder().encode(content).length;
  budget.output(outputBytes);
  return { content, warnings, imageCount: budget.images, imageBytes: budget.bytes, outputBytes };
}
