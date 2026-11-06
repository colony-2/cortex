/** Resolve logical artifact names without constructing URLs or widening the owner's catalog. */
export interface LinkCandidate { key: string; path: string; aliases?: string[] }
export type LinkResolution =
  | { kind: 'browser' }
  | { kind: 'fragment'; fragment: string }
  | { kind: 'target'; key: string; fragment?: string }
  | { kind: 'unavailable'; reason: 'missing' | 'ambiguous' | 'invalid' | 'outside-context' };
export interface PreviewNavigation {
  resolve: (reference: string) => LinkResolution;
  open: (key: string, fragment?: string) => boolean;
}
export interface NavigationIntent { token: number; fragment?: string }
const unavailable = (reason: Extract<LinkResolution, { kind: 'unavailable' }>['reason']): LinkResolution => ({ kind: 'unavailable', reason });
const invalidPath = (path: string) => /[\\\u0000-\u001f\u007f]/.test(path);
function normalize(path: string): string | undefined {
  if (!path || path.startsWith('/') || invalidPath(path)) return undefined;
  const result: string[] = [];
  for (const part of path.split('/')) {
    if (!part || part === '.') continue;
    if (part === '..') { if (!result.length) return undefined; result.pop(); }
    else result.push(part);
  }
  return result.length ? result.join('/') : undefined;
}
export function resolveArtifactLink(reference: string, currentPath: string, candidates: LinkCandidate[]): LinkResolution {
  if (!reference || invalidPath(reference)) return unavailable('invalid');
  if (/^(?:[a-z][a-z\d+.-]*:|\/)/i.test(reference)) return { kind: 'browser' };
  const hash = reference.indexOf('#');
  let fragment: string | undefined;
  try { fragment = hash < 0 ? undefined : decodeURIComponent(reference.slice(hash + 1)); }
  catch { return unavailable('invalid'); }
  if (reference.startsWith('#')) return { kind: 'fragment', fragment: fragment! };
  const beforeHash = hash < 0 ? reference : reference.slice(0, hash);
  const rawPath = beforeHash.split('?')[0];
  if (/%(?:2f|5c)/i.test(rawPath)) return unavailable('invalid');
  let path: string;
  try { path = decodeURIComponent(rawPath); } catch { return unavailable('invalid'); }
  if (!path || invalidPath(path) || path.startsWith('/')) return unavailable('invalid');
  const current = normalize(currentPath);
  const base = current?.includes('/') ? current.slice(0, current.lastIndexOf('/') + 1) : '';
  const resolved = normalize(base + path);
  if (!resolved) return unavailable('outside-context');
  const pick = (matches: LinkCandidate[]): LinkResolution | undefined => {
    const keys = [...new Set(matches.map(c => c.key))];
    return keys.length > 1 ? unavailable('ambiguous') : keys.length === 1 ? { kind: 'target', key: keys[0], fragment } : undefined;
  };
  const canonical = pick(candidates.filter(c => normalize(c.path) === resolved));
  if (canonical) return canonical;
  if (!path.split('/').some(part => part === '.' || part === '..')) {
    const alias = pick(candidates.filter(c => c.aliases?.includes(path)));
    if (alias) return alias;
  }
  return unavailable('missing');
}

// Assign IDs before rendering, so a feedback rerender cannot change duplicate suffixes.
interface MarkdownNode { type: string; value?: string; children?: MarkdownNode[]; data?: { hProperties?: Record<string, unknown> } }
export function artifactHeadingIds() {
  return (tree: MarkdownNode) => {
    const used = new Set<string>();
    const text = (node: MarkdownNode): string => node.value || node.children?.map(text).join('') || '';
    const walk = (node: MarkdownNode) => {
      if (node.type === 'heading') {
        const base = text(node).toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').replace(/\s/g, '-');
        let id = base, suffix = 0;
        while (used.has(id)) id = `${base}-${++suffix}`;
        used.add(id);
        node.data = { ...node.data, hProperties: { ...node.data?.hProperties, id } };
      }
      node.children?.forEach(walk);
    };
    walk(tree);
  };
}
