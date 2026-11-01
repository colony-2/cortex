// Review previews never modify the stored bytes or their download contract.
export const TEXT_LIMIT = 2 * 1024 * 1024;
export const STRUCTURED_LIMIT = 1024 * 1024;
export const IMAGE_LIMIT = 16 * 1024 * 1024;
export const HEX_LIMIT = 64 * 1024;
export type Format = 'markdown' | 'diff' | 'json' | 'jsonl' | 'source' | 'markup' | 'image' | 'container' | 'binary';
export interface DocumentData { bytes: Uint8Array; text: string; format: Format; truncated: boolean; mime?: string }
const ascii = (b: Uint8Array, start = 0, end = b.length) => String.fromCharCode(...b.subarray(start, Math.min(end, start + 512)));
export function rasterType(b: Uint8Array): string | undefined {
  if (b[0] === 137 && ascii(b, 1, 4) === 'PNG') return 'image/png';
  if (b[0] === 255 && b[1] === 216 && b[2] === 255) return 'image/jpeg';
  if (/^GIF8[79]a/.test(ascii(b, 0, 6))) return 'image/gif';
  if (ascii(b, 0, 4) === 'RIFF' && ascii(b, 8, 12) === 'WEBP') return 'image/webp';
  if (ascii(b, 0, 2) === 'BM') return 'image/bmp';
  if (ascii(b, 4, 8) === 'ftyp' && /avif|avis/.test(ascii(b, 8, 32))) return 'image/avif';
}
export function decodeText(b: Uint8Array, fatal = false): string {
  const encoding = b[0] === 255 && b[1] === 254 ? 'utf-16le' : b[0] === 254 && b[1] === 255 ? 'utf-16be' : 'utf-8';
  return new TextDecoder(encoding, { fatal }).decode(b);
}
export function classify(name: string, b: Uint8Array): Format {
  const start = ascii(b, 0, 16);
  if (/\.(tar|gz|tgz|zip|bz2|xz|7z|rar|pack)$/i.test(name) || name === '__git_state_thin_pack__' || start.startsWith('PACK') || start.startsWith('BZh') || start.startsWith('7z\xbc\xaf') || start.startsWith('Rar!') || (b[0] === 253 && ascii(b, 1, 6) === '7zXZ\0') || start.startsWith('PK\x03\x04') || (b[0] === 31 && b[1] === 139) || ascii(b, 257, 262) === 'ustar') return 'container';
  if (rasterType(b)) return 'image';
  let text: string;
  try { text = decodeText(b, true); } catch { return 'binary'; }
  if (text.includes('\0')) return 'binary';
  if (/\.(html?|svg)$/i.test(name) || /^\s*(?:<!doctype html|<html|<svg)/i.test(text)) return 'markup';
  if (/\.(md|markdown)$/i.test(name)) return 'markdown';
  if (/\.(diff|patch)$/i.test(name) || /^(diff --git |--- .+\r?\n\+\+\+ )/m.test(text)) return 'diff';
  if (/\.(jsonl|ndjson)$/i.test(name)) return 'jsonl';
  if (/\.json$/i.test(name)) return 'json';
  return 'source';
}
export function structuredAllowed(data: DocumentData): boolean {
  return !data.truncated && data.bytes.length <= STRUCTURED_LIMIT && data.text.split('\n').length <= 20000;
}
export async function loadDocument(url: string, name: string, signal: AbortSignal): Promise<DocumentData> {
  const response = await fetch(url, { signal });
  if (!response.ok) throw new Error('Could not load this document. It may be unavailable or the review may have changed.');
  if (!response.body) throw new Error('Document streaming is unavailable. Download the original file.');
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0, limit = TEXT_LIMIT, truncated = false;
  let prefix = new Uint8Array(0);
  const abort = () => { void reader.cancel().catch(() => {}); };
  signal.addEventListener('abort', abort, { once: true });
  try {
    while (!signal.aborted) {
      const { value, done } = await reader.read();
      if (done) break;
      if (prefix.length < 512) {
        const combined = new Uint8Array(Math.min(512, prefix.length + value.length));
        combined.set(prefix); combined.set(value.subarray(0, combined.length - prefix.length), prefix.length); prefix = combined;
      }
      let sample = prefix;
      // Stream chunks and sample boundaries can split an encoded character.
      for (let trim = 0; trim < 4; trim++) {
        try { decodeText(sample, true); break; } catch { sample = sample.subarray(0, sample.length - 1); }
      }
      const format = classify(name, sample);
      limit = format === 'image' ? IMAGE_LIMIT : format === 'binary' || format === 'container' ? HEX_LIMIT : TEXT_LIMIT;
      const retained = value.slice(0, Math.max(0, limit + 1 - size));
      chunks.push(retained); size += retained.length;
      if (size > limit) { truncated = true; await reader.cancel(); break; }
    }
    if (signal.aborted) throw new DOMException('Aborted', 'AbortError');
  } finally { signal.removeEventListener('abort', abort); reader.releaseLock(); }
  const bytes = new Uint8Array(Math.min(size, limit));
  let offset = 0;
  for (const chunk of chunks) { const part = chunk.subarray(0, bytes.length - offset); bytes.set(part, offset); offset += part.length; }
  let sample = bytes;
  if (truncated) { for (let trim = 0; trim < 4; trim++) { try { decodeText(sample, true); break; } catch { sample = sample.subarray(0, sample.length - 1); } } }
  const format = classify(name, sample);
  if (format === 'image' && truncated) throw new Error('Image exceeds the 16 MiB preview limit. Download the original.');
  const retained = format === 'binary' || format === 'container' ? bytes.slice(0, HEX_LIMIT) : bytes;
  return { bytes: retained, text: decodeText(retained), format, truncated: truncated || retained.length < bytes.length, mime: rasterType(retained) };
}
// Dimensions are inspected before creating an image URL or asking the browser to decode.
export function imageDimensions(b: Uint8Array, mime: string): [number, number] {
  const v = new DataView(b.buffer, b.byteOffset, b.byteLength);
  let w = 0, h = 0;
  try {
    if (mime === 'image/png') { w = v.getUint32(16); h = v.getUint32(20); }
    if (mime === 'image/gif') { w = v.getUint16(6, true); h = v.getUint16(8, true); }
    if (mime === 'image/bmp') { w = Math.abs(v.getInt32(18, true)); h = Math.abs(v.getInt32(22, true)); }
    if (mime === 'image/jpeg') {
      let p = 2;
      while (p + 4 < b.length) {
        if (b[p++] !== 255) break;
        const marker = b[p++];
        if (marker === 255) { p--; continue; }
        if (marker === 217 || marker === 218) break;
        const len = v.getUint16(p);
        if (len < 2) break;
        if ([192,193,194,195,197,198,199,201,202,203,205,206,207].includes(marker)) { h = v.getUint16(p + 3); w = v.getUint16(p + 5); break; }
        p += len;
      }
    }
    if (mime === 'image/webp') {
      const type = ascii(b, 12, 16);
      if (type === 'VP8X') { w = 1 + b[24] + b[25] * 256 + b[26] * 65536; h = 1 + b[27] + b[28] * 256 + b[29] * 65536; }
      if (type === 'VP8 ') { w = v.getUint16(26, true) & 16383; h = v.getUint16(28, true) & 16383; }
      if (type === 'VP8L' && b[20] === 47) { const n = v.getUint32(21, true); w = 1 + (n & 16383); h = 1 + ((n >>> 14) & 16383); }
    }
    if (mime === 'image/avif') {
      // ISO BMFF ispe image spatial extents. Walk known container boxes only.
      const walk = (start: number, end: number, depth: number) => {
        if (depth > 12) return;
        for (let p = start; p + 8 <= end;) {
          const size = v.getUint32(p), type = ascii(b, p + 4, p + 8);
          if (size < 8 || p + size > end) break;
          if (type === 'ispe' && size >= 20) { const x = v.getUint32(p + 12), y = v.getUint32(p + 16); if (x * y > w * h) { w = x; h = y; } }
          if (['meta','iprp','ipco'].includes(type)) walk(p + (type === 'meta' ? 12 : 8), p + size, depth + 1);
          p += size;
        }
      }; walk(0, b.length, 0);
    }
  } catch { throw new Error('Invalid image dimensions. Download the original.'); }
  if (!w || !h || !Number.isFinite(w * h)) throw new Error('Image dimension inspection is unsupported or invalid. Download the original.');
  if (w * h > 32000000) throw new Error('Image exceeds the 32 megapixel preview limit. Download the original.');
  return [w, h];
}
export function hexDump(bytes: Uint8Array): string {
  const rows: string[] = [];
  for (let p = 0; p < Math.min(bytes.length, HEX_LIMIT); p += 16) {
    const row = bytes.subarray(p, p + 16);
    rows.push(`${p.toString(16).padStart(8, '0')}  ${Array.from(row, n => n.toString(16).padStart(2, '0')).join(' ').padEnd(47)}  ${Array.from(row, n => n >= 32 && n < 127 ? String.fromCharCode(n) : '.').join('')}`);
  }
  return rows.join('\n');
}
