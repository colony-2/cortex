import { isJobJSONNumber, parseJobJSON, stringifyJobJSON, type RestartContext } from '@colony2/shared';

export type PatchLine = { id: string; path: string; value: string };
export type ScopePatchLine = PatchLine & { container: 'sequence' | 'states'; scopeId: string };
type ObjectValue = Record<string, unknown>;
export type Operation = { op: 'add' | 'remove' | 'replace'; path: string; value?: unknown };
const object = (v: unknown): v is ObjectValue => !!v && typeof v === 'object' && !Array.isArray(v) && !isJobJSONNumber(v);
const own = (v: ObjectValue, k: string) => Object.prototype.hasOwnProperty.call(v, k);
const escape = (v: string) => v.replace(/~/g, '~0').replace(/\//g, '~1');
const pointer = (tokens: string[]) => '/' + tokens.map(escape).join('/');
const clone = (v: unknown) => parseJobJSON(stringifyJobJSON(v)!);
function parseValue(raw: string): unknown {
  try { return parseJobJSON(raw.trim()); } catch { return raw.trim(); }
}
// Preserve the existing recursive merge and null deletion intent. Own-property
// writes also keep user keys such as __proto__ from changing prototypes.
function merge(base: unknown, value: unknown): unknown {
  if (!object(value)) return clone(value);
  const result: ObjectValue = object(base) ? clone(base) as ObjectValue : {};
  for (const key of Object.keys(value)) {
    if (value[key] === null) delete result[key];
    else Object.defineProperty(result, key, { value: merge(own(result, key) ? result[key] : undefined, value[key]), enumerable: true, writable: true, configurable: true });
  }
  return result;
}
function diff(before: unknown, after: unknown, path: string, out: Operation[]) {
  if (object(before) && object(after)) {
    for (const key of [...new Set([...Object.keys(before), ...Object.keys(after)])].sort()) {
      const next = `${path}/${escape(key)}`;
      if (!own(after, key)) out.push({ op: 'remove', path: next });
      else if (!own(before, key)) out.push({ op: 'add', path: next, value: after[key] });
      else diff(before[key], after[key], next, out);
    }
  } else if (stringifyJobJSON(before) !== stringifyJobJSON(after)) out.push({ op: 'replace', path, value: after });
}
export function buildContextPatch(snapshot: RestartContext | undefined, jobLines: PatchLine[], scopeLines: ScopePatchLine[]): Operation[] {
  if (!snapshot) throw new Error('Load context before submitting edits. Retry context inspection.');
  if (!object(snapshot.job) || !object(snapshot.visible)) throw new Error('Editable context is unavailable.');
  const baseline: ObjectValue = { context: snapshot.job };
  for (const container of ['sequence', 'states']) if (own(snapshot.visible, container)) baseline[container] = snapshot.visible[container];
  const desired = clone(baseline) as ObjectValue;
  const redacted = (snapshot.redacted_paths || []).map(path => path === '/job' || path.startsWith('/job/') ? '/context' + path.slice(4) : path.startsWith('/visible/') ? path.slice(8) : path);
  const rows = [
    ...jobLines.filter(line => line.path.trim()).map(line => ({ line, tokens: ['context', ...line.path.split('.').map(s => s.trim()).filter(Boolean)] })),
    ...scopeLines.filter(line => line.path.trim() && line.scopeId.trim()).map(line => ({ line, tokens: [line.container, line.scopeId.trim(), 'outputs', ...line.path.split('.').map(s => s.trim()).filter(Boolean)] })),
  ];
  for (const { line, tokens } of rows) {
    if (tokens[0] === 'context' && !['environment', 'workflow', 'git', 'artifacts', 'recipe_source'].includes(tokens[1])) throw new Error('Unsupported editable context field.');
    const path = pointer(tokens);

    if (redacted.some(r => r === path || path.startsWith(r + '/'))) throw new Error(`Required context is redacted: ${path}`);
    let target = desired;
    for (const token of tokens.slice(0, -1)) {
      if (!own(target, token) || !object(target[token])) throw new Error(`Required context parent is unavailable: ${path}`);
      target = target[token] as ObjectValue;
    }
    const key = tokens[tokens.length - 1];
    const value = parseValue(line.value);
    const check = (before: unknown, edit: unknown, affected: string) => {
      if (object(before) && object(edit)) {
        for (const key of Object.keys(edit)) check(before[key], edit[key], `${affected}/${escape(key)}`);
        return;
      }
      if (redacted.some(r => r === affected || r.startsWith(affected + '/') || affected.startsWith(r + '/'))) throw new Error(`Required context is redacted: ${affected}`);
    };
    check(own(target, key) ? target[key] : undefined, value, path);
    if (value === null) delete target[key];
    else Object.defineProperty(target, key, { value: merge(own(target, key) ? target[key] : undefined, value), enumerable: true, writable: true, configurable: true });
  }
  const operations: Operation[] = [];
  diff(baseline, desired, '', operations);
  return operations;
}
