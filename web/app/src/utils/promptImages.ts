import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';
import { imageDimensions, IMAGE_LIMIT, rasterType } from '../components/review/document';
import { validateGoalAttachments } from './goalAttachments';
import type { ReviewImagePasteOutcome, ReviewImagePasteResult } from 'critrev/react';

const parser = unified().use(remarkParse).use(remarkGfm);
// Use the same CommonMark + GFM dialect as prompt readers; code is never an image.
export function markdownImages(markdown: string): string[] {
  const tree = parser.parse(markdown);
  const definitions = new Map<string, string>();
  const normalize = (id: string) => id.replace(/\s+/g, ' ').toUpperCase();
  type Node = { type: string; children?: readonly Node[]; identifier?: string; url?: string };
  function walk(node: Node, visit: (node: Node) => void) {
    visit(node);
    node.children?.forEach(child => walk(child as typeof node, visit));
  }
  walk(tree, node => { if (node.type === 'definition' && node.identifier !== undefined && node.url !== undefined && !definitions.has(normalize(node.identifier))) definitions.set(normalize(node.identifier), node.url); });
  const urls: string[] = [];
  walk(tree, node => {
    const url = node.type === 'image' ? node.url : node.type === 'imageReference' && node.identifier !== undefined ? definitions.get(normalize(node.identifier)) : undefined;
    if (url !== undefined && !urls.includes(url)) urls.push(url);
  });
  return urls;
}
export const safeImageName = (name: string) => !!name && !/[\s/\\:?#%<>\x00-\x1f\x7f]/u.test(name) && name !== '.' && name !== '..';
const supported = new Map([['image/png', 'png'], ['image/jpeg', 'jpg'], ['image/gif', 'gif'], ['image/webp', 'webp']]);
export async function rasterPreview(file: Blob, signal?: AbortSignal): Promise<string> {
  if (file.size > IMAGE_LIMIT) throw new Error('Image exceeds the 16 MiB preview limit.');
  const bytes = new Uint8Array(await file.arrayBuffer());
  if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
  const mime = rasterType(bytes);
  if (!mime || !supported.has(mime) || (file.type && file.type !== mime)) throw new Error('Use a valid PNG, JPEG, GIF, or WebP image with matching file type.');
  imageDimensions(bytes, mime);
  const url = URL.createObjectURL(new Blob([bytes], { type: mime }));
  try {
    await new Promise<void>((resolve, reject) => {
      const image = new Image();
      const cleanup = () => { image.onload = image.onerror = null; signal?.removeEventListener('abort', abort); };
      const abort = () => { cleanup(); image.src = ''; reject(new DOMException('Aborted', 'AbortError')); };
      image.onload = () => { cleanup(); resolve(); };
      image.onerror = () => { cleanup(); reject(new Error('Image could not be decoded.')); };
      signal?.addEventListener('abort', abort, { once: true });
      if (signal?.aborted) abort(); else image.src = url;
    });
    return url;
  } catch (error) { URL.revokeObjectURL(url); throw error; }
}
export type DraftImage = { file: File; original: File; status: 'pending' | 'ready' | 'invalid'; preview?: string; error?: string; controller: AbortController };
// The pool is draft-owned, including images currently absent from Markdown but in undo.
export class DraftImages {
  readonly entries = new Map<string, DraftImage>();
  private disposed = false;
  private staged: DraftImage[] = [];
  constructor(private changed: () => void, private explicit: () => readonly File[]) {}
  activate() { this.disposed = false; }
  stage(files: readonly File[]): ReviewImagePasteResult {
    if (this.disposed) return { reject: true };
    const usedNames = new Set([...this.explicit(), ...this.files()].map(file => file.name));
    const staged = files.map(original => {
      const extension = supported.get(original.type);
      if (!extension) throw new Error('Paste PNG, JPEG, GIF, or WebP images.');
      const stem = `pasted-${crypto.randomUUID()}`;
      let name = `${stem}.${extension}`;
      for (let suffix = 1; usedNames.has(name); suffix++) name = `${stem}-${suffix}.${extension}`;
      usedNames.add(name);
      const file = new File([original], name, { type: original.type });
      return { file, original, status: 'pending' as const, controller: new AbortController() };
    });
    validateGoalAttachments([...this.explicit(), ...this.files(), ...staged.map(item => item.file)]);
    this.staged = staged;
    staged.forEach(item => this.add(item));
    this.changed();
    return { images: staged.map(item => ({ src: item.file.name, alt: item.original.name || 'Pasted image' })) };
  }
  restore(files: readonly File[]) {
    files.forEach(file => this.add({ file, original: file, status: 'pending', controller: new AbortController() }));
    if (files.length) this.changed();
  }
  private add(item: DraftImage) {
    this.entries.set(item.file.name, item);
    void rasterPreview(item.file, item.controller.signal).then(preview => {
      if (this.disposed || this.entries.get(item.file.name) !== item) URL.revokeObjectURL(preview);
      else { Object.assign(item, { status: 'ready', preview }); this.changed(); }
    }, reason => {
      if (!this.disposed && this.entries.get(item.file.name) === item) { Object.assign(item, { status: 'invalid', error: (reason as Error).message }); this.changed(); }
    });
  }
  finish(outcome: ReviewImagePasteOutcome) {
    const batch = this.staged;
    this.staged = [];
    if (outcome.inserted) return;
    for (const item of batch) {
      item.controller.abort();
      if (item.preview) URL.revokeObjectURL(item.preview);
      this.entries.delete(item.file.name);
    }
    this.changed();
  }
  files() { return [...this.entries.values()].map(item => item.file); }
  previews() { return new Map([...this.entries].flatMap(([name, item]) => item.preview ? [[name, item.preview] as const] : [])); }
  submission(markdown: string, explicit: readonly File[]): File[] {
    validateGoalAttachments([...explicit, ...this.files()]);
    const images = markdownImages(markdown).flatMap(name => {
      const image = this.entries.get(name);
      if (!image) {
        if (/^pasted-[a-f\d-]+\.(png|jpg|gif|webp)$/.test(name) && !explicit.some(file => file.name === name)) throw new Error('A pasted image is missing from this draft. Paste it again or remove its reference.');
        return [];
      }
      if (image.status !== 'ready') throw new Error(image.status === 'pending' ? 'Wait for pasted images to finish checking, or remove their references.' : `Pasted image unavailable: ${image.error} Remove its reference to continue.`);
      return [image.file];
    });
    return [...explicit, ...images];
  }
  dispose() { this.disposed = true; this.entries.forEach(item => { item.controller.abort(); if (item.preview) URL.revokeObjectURL(item.preview); }); this.entries.clear(); this.staged = []; }
}
