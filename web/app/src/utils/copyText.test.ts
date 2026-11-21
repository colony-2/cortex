import { afterEach, expect, it, vi } from 'vitest';
import { copyText, copyTextCompatibility } from './copyText';

afterEach(() => { document.body.replaceChildren(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
function command(result: boolean | Error) {
  const spy = vi.fn(() => { if (result instanceof Error) throw result; return result; });
  vi.stubGlobal('document', document);
  Object.defineProperty(document, 'execCommand', { configurable: true, value: spy });
  return spy;
}
it('JC-01 calls modern writing immediately and awaits fulfillment without compatibility', async () => {
  let done!: () => void;
  const writeText = vi.fn(() => new Promise<void>(resolve => { done = resolve; }));
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  const fallback = command(true);
  const result = copyText(' exact ID\n');
  expect(writeText).toHaveBeenCalledExactlyOnceWith(' exact ID\n');
  expect(result.method).toBe('modern');
  if (result.method !== 'modern') throw new Error('Expected modern operation');
  const settled = vi.fn(); void result.completion.then(settled);
  await Promise.resolve(); expect(settled).not.toHaveBeenCalled();
  done(); await result.completion;
  expect(fallback).not.toHaveBeenCalled();
});
it.each([undefined, {}, { writeText: 'unavailable' }])('JC-02/03 copies synchronously with incomplete API %j', clipboard => {
  vi.stubGlobal('navigator', { clipboard });
  const spy = command(true).mockImplementation(() => {
    const field = document.activeElement as HTMLTextAreaElement;
    expect(field.value).toBe(' full ID\n');
    expect(field.value.slice(field.selectionStart, field.selectionEnd)).toBe(' full ID\n');
    expect(field.readOnly).toBe(true); expect(field.tabIndex).toBe(-1);
    return true;
  });
  expect(copyText(' full ID\n')).toEqual({ method: 'compatibility', copied: true });
  expect(spy).toHaveBeenCalledExactlyOnceWith('copy');
  expect(document.querySelector('textarea')).toBeNull();
});
it.each([false, new Error('denied'), true])('JC-05 restores focus, text/document selection and scroll for %s', result => {
  const field = document.createElement('textarea'); field.value = 'previous text';
  const text = document.createElement('p'); text.textContent = 'selected document';
  document.body.append(field, text); field.focus(); field.setSelectionRange(2, 7, 'backward');
  const range = document.createRange(); range.selectNodeContents(text);
  const selection = window.getSelection()!; selection.removeAllRanges(); selection.addRange(range);
  field.scrollTop = 12; document.body.scrollTop = 24;
  command(result);
  expect(copyTextCompatibility('new ID')).toBe(result === true);
  expect(document.activeElement).toBe(field);
  expect([field.selectionStart, field.selectionEnd, field.selectionDirection]).toEqual([2, 7, 'backward']);
  expect(selection.toString()).toBe('selected document');
  expect(field.scrollTop).toBe(12); expect(document.body.scrollTop).toBe(24);
  expect(document.querySelectorAll('textarea')).toHaveLength(1);
});
it.each(['missing', 'selection throws', 'selection incomplete'])('JC-04 contains %s and removes temporary field', failure => {
  const spy = command(true);
  if (failure === 'missing') Object.defineProperty(document, 'execCommand', { configurable: true, value: undefined });
  else vi.spyOn(HTMLTextAreaElement.prototype, 'select').mockImplementation(() => { if (failure === 'selection throws') throw new Error('selection'); });
  expect(copyTextCompatibility('exact')).toBe(false);
  expect(spy).not.toHaveBeenCalled(); expect(document.querySelector('textarea')).toBeNull();
});
it('JC-06 tolerates a detached prior focus target', () => {
  const button = document.createElement('button'); document.body.append(button); button.focus();
  command(true).mockImplementation(() => { button.remove(); return true; });
  expect(copyTextCompatibility('id')).toBe(true); expect(document.querySelector('textarea')).toBeNull();
});
it('contains clipboard access exceptions without attempting compatibility after failure', async () => {
  vi.stubGlobal('navigator', Object.defineProperty({}, 'clipboard', { get: () => { throw new Error('blocked'); } }));
  const fallback = command(true);
  const result = copyText('id');
  expect(result.method).toBe('modern');
  if (result.method === 'modern') await expect(result.completion).rejects.toThrow('blocked');
  expect(fallback).not.toHaveBeenCalled();
});
