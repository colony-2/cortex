export interface DiffLine { kind: 'context' | 'add' | 'delete' | 'meta'; text: string; old?: number; next?: number; newline?: 'old' | 'next' | 'both' }
export interface DiffFile { name: string; lines: DiffLine[] }
export function parseDiff(text: string): DiffFile[] {
  if (/^(diff --cc |diff --combined |@@@)/m.test(text)) throw new Error('Combined patches are unavailable in structured view; use Source.');
  const files: DiffFile[] = [];
  let file: DiffFile | undefined, old = 0, next = 0, oldLeft = 0, nextLeft = 0, inHunk = false;
  const check = () => { if (oldLeft || nextLeft) throw new Error('Incomplete patch hunk; use Source.'); };
  const lines = text.split('\n'); if (lines[lines.length - 1] === '') lines.pop();
  for (let i = 0; i < lines.length; i++) {
    const text = lines[i].replace(/\r$/, '');
    if (inHunk && !oldLeft && !nextLeft && !text.startsWith('\\ No newline')) inHunk = false;
    if (text.startsWith('diff --git ') || (!inHunk && text.startsWith('--- ') && (!file || file.lines.some(l => l.text.startsWith('+++ '))))) {
      check(); file = { name: text.startsWith('diff --git ') ? text.slice(11) : text.slice(4), lines: [] }; files.push(file); inHunk = false;
    }
    if (!file) {
      if (!text.trim()) continue;
      throw new Error('Unrecognized patch syntax; use Source.');
    }
    const hunk = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(text);
    if (hunk) {
      check(); old = +hunk[1]; next = +hunk[3]; oldLeft = hunk[2] === undefined ? 1 : +hunk[2]; nextLeft = hunk[4] === undefined ? 1 : +hunk[4]; inHunk = true;
      file.lines.push({ kind: 'meta', text }); continue;
    }
    if (text.startsWith('@@')) throw new Error('Malformed patch header; use Source.');
    if (inHunk && text.startsWith('\\ No newline')) {
      const previous = file.lines[file.lines.length - 1];
      if (!previous || previous.kind === 'meta') throw new Error('Misplaced newline marker; use Source.');
      file.lines.push({ kind: 'meta', text, newline: previous.kind === 'delete' ? 'old' : previous.kind === 'add' ? 'next' : 'both' }); continue;
    }
    if (inHunk && (oldLeft || nextLeft)) {
      const prefix = text[0];
      if (![' ', '+', '-'].includes(prefix)) throw new Error('Malformed patch line; use Source.');
      const line: DiffLine = { kind: prefix === '+' ? 'add' : prefix === '-' ? 'delete' : 'context', text };
      if (prefix !== '+') { if (--oldLeft < 0) throw new Error('Invalid old hunk count'); line.old = old++; }
      if (prefix !== '-') { if (--nextLeft < 0) throw new Error('Invalid new hunk count'); line.next = next++; }
      file.lines.push(line); continue;
    }
    inHunk = false;
    if (/^[ +\-]/.test(text) && !/^(--- |\+\+\+ )/.test(text)) throw new Error('Patch exceeds declared hunk; use Source.');
    file.lines.push({ kind: 'meta', text });
  }
  check(); return files;
}
export function alignDiff(lines: DiffLine[]): { left?: DiffLine; right?: DiffLine }[] {
  const rows: { left?: DiffLine; right?: DiffLine }[] = [];
  for (let i = 0; i < lines.length;) {
    if (lines[i].kind !== 'delete' && lines[i].kind !== 'add') { rows.push({ left: lines[i], right: lines[i] }); i++; continue; }
    // Newline annotations belong to the preceding change, not to the whole hunk.
    // Keep them inside replacement groups without consuming an alignment slot.
    const deletes: { line: DiffLine; marker?: DiffLine }[] = [], adds: { line: DiffLine; marker?: DiffLine }[] = [];
    while (i < lines.length && ['delete','add'].includes(lines[i].kind)) {
      const line = lines[i++];
      const entry: { line: DiffLine; marker?: DiffLine } = { line };
      if (lines[i]?.newline === (line.kind === 'delete' ? 'old' : 'next')) entry.marker = lines[i++];
      (line.kind === 'delete' ? deletes : adds).push(entry);
    }
    for (let j = 0; j < Math.max(deletes.length, adds.length); j++) {
      rows.push({ left: deletes[j]?.line, right: adds[j]?.line });
      if (deletes[j]?.marker || adds[j]?.marker) rows.push({ left: deletes[j]?.marker, right: adds[j]?.marker });
    }
  }
  return rows;
}
