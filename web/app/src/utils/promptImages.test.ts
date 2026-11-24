import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { DraftImages, markdownImages, rasterPreview } from './promptImages';

const png = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aOBsAAAAASUVORK5CYII='), c => c.charCodeAt(0));
let decode: (() => void)[];
beforeEach(() => {
  decode = [];
  vi.stubGlobal('Image', class {
    onload?: () => void;
    set src(value: string) { if (value) decode.push(() => this.onload?.()); }
  });
  vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });
  Object.defineProperty(Blob.prototype, 'arrayBuffer', { configurable: true, value: function() { return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(reader.result); reader.onerror = reject; reader.readAsArrayBuffer(this); }); } });
});
afterEach(() => vi.unstubAllGlobals());
const image = () => new File([png], 'clipboard.png', { type: 'image/png' });
it('inventories actual inline/reference GFM images once and excludes code/HTML', () => {
  expect(markdownImages('![a](one.png)\n\n![b][ref]\n\n[ref]: two.png\n\n![c](one.png)\n\n`![code](no.png)`\n\n```\n![fence](no.png)\n```\n\n<img src="no.png">')).toEqual(['one.png', 'two.png']);
  expect(markdownImages('![a][x]\n\n[x]: first.png\n[x]: second.png')).toEqual(['first.png']);
});
it('stages synchronously, blocks only live pending refs, retains undo bytes and submits once in reference order', async () => {
  const changed = vi.fn(), explicit = [new File(['brief'], 'brief.md')];
  const pool = new DraftImages(changed, () => explicit);
  const staged = pool.stage([image(), image()]);
  if (!('images' in staged)) throw new Error('staging rejected');
  const [a, b] = staged.images.map(value => value.src);
  expect(a).toMatch(/^pasted-[a-f\d-]+\.png$/); expect(b).not.toBe(a);
  expect(() => pool.submission(`![a](${a})`, explicit)).toThrow(/Wait/);
  expect(pool.submission('No live images', explicit)).toEqual(explicit);
  await waitFor(() => expect(decode).toHaveLength(2)); decode.forEach(done => done());
  await waitFor(() => expect(pool.previews().size).toBe(2));
  const files = pool.submission(`![b](${b}) ![a](${a}) ![b](${b})`, explicit);
  expect(files.map(file => file.name)).toEqual(['brief.md', b, a]);
  expect(pool.files()).toHaveLength(2);
  pool.dispose(); expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2);
});
it('rejects full batches at shared count/byte/name limits without allocating previews', () => {
  const explicit = Array.from({ length: 10 }, (_, i) => new File(['x'], `x${i}`));
  const pool = new DraftImages(vi.fn(), () => explicit);
  expect(() => pool.stage([image()])).toThrow(/10 files/); expect(pool.files()).toEqual([]);
  expect(() => pool.stage([image(), new File(['svg'], 'x.svg', { type: 'image/svg+xml' })])).toThrow(/PNG/);
  expect(pool.files()).toEqual([]); expect(URL.createObjectURL).not.toHaveBeenCalled();
});
it('RC-30 accepts the tenth shared file and preserves it when the next paste exceeds count', async () => {
  const explicit = Array.from({ length: 9 }, (_, i) => new File(['brief'], `brief-${i}.txt`));
  const pool = new DraftImages(vi.fn(), () => explicit);
  try {
    const result = pool.stage([image()]);
    if (!('images' in result)) throw new Error('staging rejected');
    pool.finish({ inserted: true, files: [] });
    const retained = pool.files()[0];
    expect(explicit.length + pool.files().length).toBe(10);
    expect(() => pool.stage([image()])).toThrow(/10 files/);
    expect(pool.files()).toEqual([retained]);
    await waitFor(() => expect(decode).toHaveLength(1)); decode[0]();
    await waitFor(() => expect(() => pool.submission(`![last](${result.images[0].src})`, explicit)).not.toThrow());
    expect(pool.submission(`![last](${result.images[0].src})`, explicit)).toEqual([...explicit, retained]);
  } finally { pool.dispose(); }
});
it('RC-30 accepts exactly 25 MiB across explicit and pasted files and rejects one extra byte', async () => {
  const budget = 25 * 1024 * 1024;
  const explicit = [new File([new Uint8Array(budget - png.length)], 'requirements.bin')];
  const pool = new DraftImages(vi.fn(), () => explicit);
  try {
    pool.stage([image()]); pool.finish({ inserted: true, files: [] });
    const retained = pool.files()[0];
    expect(explicit[0].size + retained.size).toBe(budget);
    explicit.push(new File(['x'], 'extra.txt'));
    expect(() => pool.stage([image()])).toThrow(/25 MiB/);
    expect(() => pool.submission('No live pastes', explicit)).toThrow(/25 MiB/);
    expect(pool.files()).toEqual([retained]);
    explicit.pop();
    await waitFor(() => expect(decode).toHaveLength(1)); decode[0]();
    await waitFor(() => expect(pool.previews().size).toBe(1));
    expect(pool.submission('No live pastes', explicit)).toEqual(explicit);
  } finally { pool.dispose(); }
});
it('avoids collisions with explicit files, retained images and other images in a batch', () => {
  const uuid = '00000000-0000-4000-8000-000000000000';
  const random = vi.spyOn(crypto, 'randomUUID').mockReturnValue(uuid);
  const pool = new DraftImages(vi.fn(), () => [new File([], `pasted-${uuid}.png`)]);
  try {
    pool.stage([image(), image()]);
    pool.finish({ inserted: true, files: [] });
    pool.stage([image()]);
    expect(pool.files().map(file => file.name)).toEqual([1, 2, 3].map(suffix => `pasted-${uuid}-${suffix}.png`));
  } finally { pool.dispose(); random.mockRestore(); }
});
it('rejects missing managed bytes while keeping unrelated images as unbound placeholders', () => {
  const pool = new DraftImages(vi.fn(), () => []);
  expect(() => pool.submission('![lost](pasted-00000000-0000-4000-8000-000000000000.png)', [])).toThrow(/missing from this draft/);
  expect(pool.submission('![remote](https://example.com/image.png) ![unbound](diagram.png)', [])).toEqual([]);
});
it('failed editor insertion rolls back staged bytes and ignores delayed previews', async () => {
  const changed = vi.fn(), file = image();
  const pool = new DraftImages(changed, () => []);
  pool.stage([file]);
  await waitFor(() => expect(decode).toHaveLength(1));
  pool.finish({ inserted: false, files: [file], reason: 'rejected' });
  expect(pool.files()).toEqual([]);
  await waitFor(() => expect(decode).toHaveLength(1));
  const calls = changed.mock.calls.length; decode[0]();
  await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalled());
  expect(changed).toHaveBeenCalledTimes(calls);
});
it('disposal ignores late work and a new lifecycle cannot revive old bytes', async () => {
  const changed = vi.fn(); const pool = new DraftImages(changed, () => []);
  pool.stage([image()]); await waitFor(() => expect(decode).toHaveLength(1));
  pool.dispose(); pool.activate(); const calls = changed.mock.calls.length;
  decode[0](); await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalled());
  expect(pool.files()).toEqual([]); expect(changed).toHaveBeenCalledTimes(calls);
});
it('invalid signature, spoofed MIME, oversized and excessive dimensions never decode', async () => {
  await expect(rasterPreview(new Blob(['bad'], { type: 'image/png' }))).rejects.toThrow(/valid PNG/);
  await expect(rasterPreview(new Blob([png], { type: 'image/jpeg' }))).rejects.toThrow(/matching/);
  const big = png.slice(); new DataView(big.buffer).setUint32(16, 100000);
  new DataView(big.buffer).setUint32(20, 100000);
  await expect(rasterPreview(new Blob([big], { type: 'image/png' }))).rejects.toThrow(/32 megapixel/);
  const blob = new Blob([png], { type: 'image/png' }); Object.defineProperty(blob, 'size', { value: 17 * 1024 * 1024 });
  await expect(rasterPreview(blob)).rejects.toThrow(/16 MiB/);
  const signal = new AbortController().signal;
  await expect(rasterPreview(blob, signal, true)).resolves.toBeUndefined();
  await expect(rasterPreview(new Blob([big], { type: 'image/png' }), signal, true)).resolves.toBeUndefined();
  expect(URL.createObjectURL).not.toHaveBeenCalled();
});
