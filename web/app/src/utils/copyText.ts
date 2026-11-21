/** Keep this synchronous: compatibility copying needs the original user gesture. */
export function copyTextCompatibility(text: string): boolean {
  const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const control = previous instanceof HTMLInputElement || previous instanceof HTMLTextAreaElement ? previous : null;
  const controlSelection = control && control.selectionStart !== null
    ? [control.selectionStart, control.selectionEnd!, control.selectionDirection ?? undefined] as const : null;
  const selection = window.getSelection();
  const ranges = selection ? Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index).cloneRange()) : [];
  const scroll = { x: window.scrollX, y: window.scrollY };
  const ancestors: { element: HTMLElement; top: number; left: number }[] = [];
  for (let element = previous; element; element = element.parentElement) ancestors.push({ element, top: element.scrollTop, left: element.scrollLeft });
  const temporary = document.createElement('textarea');
  temporary.value = text;
  temporary.readOnly = true;
  temporary.tabIndex = -1;
  temporary.setAttribute('aria-hidden', 'true');
  temporary.style.cssText = 'position:fixed;left:-10000px;top:0;font-size:16px;';
  try {
    document.body.appendChild(temporary);
    temporary.focus({ preventScroll: true });
    temporary.select();
    if (document.activeElement !== temporary || temporary.selectionStart !== 0 || temporary.selectionEnd !== text.length) return false;
    return typeof document.execCommand === 'function' && document.execCommand('copy') === true;
  } catch {
    return false;
  } finally {
    temporary.remove();
    // Restoration is best effort; a removed focus target cannot undo a copy.
    try { if (previous?.isConnected) previous.focus({ preventScroll: true }); } catch { /* detached or unfocusable */ }
    try {
      selection?.removeAllRanges();
      for (const range of ranges) if (range.commonAncestorContainer.isConnected) selection?.addRange(range);
    } catch { /* the selected content may have been removed */ }
    // In Chromium, restoring document ranges can reset a text control's selection.
    try { if (control?.isConnected && controlSelection) control.setSelectionRange(...controlSelection); } catch { /* unsupported input type */ }
    for (const { element, top, left } of ancestors) { element.scrollTop = top; element.scrollLeft = left; }
    if (window.scrollX !== scroll.x || window.scrollY !== scroll.y) window.scrollTo(scroll.x, scroll.y);
  }
}

export function copyText(text: string):
  | { method: 'modern'; completion: Promise<void> }
  | { method: 'compatibility'; copied: boolean } {
  try {
    const clipboard = navigator.clipboard;
    if (typeof clipboard?.writeText !== 'function') return { method: 'compatibility', copied: copyTextCompatibility(text) };
    return { method: 'modern', completion: Promise.resolve(clipboard.writeText(text)) };
  } catch (error) {
    return { method: 'modern', completion: Promise.reject(error) };
  }
}
