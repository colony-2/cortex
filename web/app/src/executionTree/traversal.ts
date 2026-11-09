import { getTreeChildren, getTreeNode, type TreeNode, type TreeNodeResponse, type TreeChildrenResponse } from '@colony2/shared';

export const DEFAULT_BUDGETS = { nodes: 1000, ascent: 64, depth: 64, pages: 200, requests: 300, milliseconds: 30000, concurrency: 4 };
export type Budgets = typeof DEFAULT_BUDGETS;
export interface Branch { state: 'pending' | 'loading' | 'more' | 'exhausted' | 'failed' | 'limited' | 'malformed'; token: string; seen: Set<string>; depth: number; reason?: string }
export interface TreeState {
  nodes: Map<string, TreeNode>; branches: Map<string, Branch>; warnings: string[];
  blocked: Set<string>; loading: boolean; error?: string; boundary?: string;
  started: string; observed?: string; stop?: string;
}
export interface TreeAPI {
  node: (tenant: string, id: string, signal: AbortSignal) => Promise<TreeNodeResponse>;
  children: (tenant: string, id: string, token: string, signal: AbortSignal) => Promise<TreeChildrenResponse>;
}
const api: TreeAPI = { node: getTreeNode, children: getTreeChildren };
/** One controller per route/refresh. Presentation collapse never affects discovery. */
export class Traversal {
  state: TreeState = { nodes: new Map(), branches: new Map(), warnings: [], blocked: new Set(), loading: false, started: new Date().toISOString() };
  private controller = new AbortController();
  private disposed = false;
  private pages = 0;
  private ascentHops = 0;
  private ancestor?: string;
  private initialized = false;
  constructor(readonly tenant: string, readonly entry: string, private changed: () => void, private source: TreeAPI = api, readonly budgets: Budgets = DEFAULT_BUDGETS, private now = () => performance.now()) {}
  get viewCapped() { return this.pages >= this.budgets.pages || this.state.nodes.size >= this.budgets.nodes; }
  get canResume() {
    return !this.viewCapped && (!!this.ancestor && this.ascentHops < this.budgets.ascent
      || [...this.state.branches.values()].some(b => ['pending','more'].includes(b.state)
        || b.state === 'limited' && ['Request limit','Elapsed-time limit'].includes(b.reason || '')));
  }
  cancel() { this.disposed = true; this.controller.abort(); }
  private emit() { if (!this.disposed) this.changed(); }
  private warning(message: string) { if (!this.state.warnings.includes(message)) this.state.warnings.push(message); }
  private accept(node: TreeNode, parent?: string): boolean {
    if (!node?.job_id?.trim() || node.tenant_id !== this.tenant || (parent !== undefined && node.parent_job_id !== parent)) {
      this.warning('Invalid identity, tenant or child relationship omitted'); return false;
    }
    const old = this.state.nodes.get(node.job_id);
    if (old && old.parent_job_id !== node.parent_job_id) {
      this.state.blocked.add(node.job_id); this.warning(`Conflicting parent observations: ${node.job_id}`); return false;
    }
    let cursor = node.parent_job_id;
    const seen = new Set([node.job_id]);
    while (cursor) {
      if (seen.has(cursor)) { this.state.blocked.add(node.job_id); this.warning(`Cycle boundary: ${node.job_id}`); return false; }
      seen.add(cursor); cursor = this.state.nodes.get(cursor)?.parent_job_id;
    }
    if (!old && this.state.nodes.size >= this.budgets.nodes) return false;
    this.state.nodes.set(node.job_id, node);
    if (node.recipe_parent_job_id && node.recipe_parent_job_id !== node.parent_job_id) this.warning(`Recipe attribution differs from runtime parent: ${node.job_id}`);
    if (!this.state.branches.has(node.job_id)) this.state.branches.set(node.job_id, { state: 'pending', token: '', seen: new Set(), depth: 0 });
    return true;
  }
  async run(retry?: string) {
    if (this.disposed || this.state.loading) return;
    this.state.loading = true; this.state.error = undefined; this.state.stop = undefined;
    this.controller = new AbortController();
    const signal = this.controller.signal;
    const start = this.now(); let requests = 0;
    const timeout = setTimeout(() => this.controller.abort(), this.budgets.milliseconds);
    const limit = () => {
      if (this.now() - start >= this.budgets.milliseconds || signal.aborted) return 'Elapsed-time limit';
      if (requests >= this.budgets.requests) return 'Request limit';
      return undefined;
    };
    const call = async <T,>(fn: () => Promise<T>): Promise<T> => {
      const reason = limit(); if (reason) throw new Error(reason);
      requests++;
      // Race also bounds fixture/transports that ignore cancellation.
      return new Promise<T>((resolve, reject) => {
        const abort = () => reject(new Error('Elapsed-time limit'));
        signal.addEventListener('abort', abort, { once: true });
        fn().then(resolve, reject).finally(() => signal.removeEventListener('abort', abort));
      });
    };
    this.emit();
    try {
      if (!this.initialized) {
        const result = await call(() => this.source.node(this.tenant, this.entry, signal));
        if (this.disposed) return;
        if (result.node.job_id !== this.entry || !this.accept(result.node)) throw new Error('Entry unavailable');
        this.state.observed = result.observed_at; this.ancestor = result.node.parent_job_id;
        this.initialized = true; this.emit();
      }
      if (!retry) while (this.ancestor) {
        if (this.ascentHops >= this.budgets.ascent) { this.state.boundary = 'Ancestor hop limit'; break; }
        if (this.state.nodes.size >= this.budgets.nodes) { this.state.boundary = 'Node limit'; break; }
        if (limit()) { this.state.stop = limit(); break; }
        const id = this.ancestor;
        if (this.state.nodes.has(id)) { this.state.boundary = 'Cycle boundary'; this.ancestor = undefined; break; }
        try {
          const result = await call(() => this.source.node(this.tenant, id, signal));
          if (this.disposed) return;
          if (result.node.job_id !== id || !this.accept(result.node)) { this.state.boundary = 'Malformed ancestor boundary'; this.ancestor = undefined; break; }
          this.state.observed = result.observed_at; this.ascentHops++;
          this.ancestor = result.node.parent_job_id; this.state.boundary = undefined; this.emit();
        } catch { this.state.boundary = limit() || 'Ancestor unavailable; known component retained'; break; }
      }
      // All known ancestry is seeded, even if an upper child listing fails.
      const depth = (id: string) => { let d = 0, p = this.state.nodes.get(id)?.parent_job_id; const seen = new Set([id]); while (p && this.state.nodes.has(p) && !seen.has(p)) { seen.add(p); d++; p = this.state.nodes.get(p)?.parent_job_id; } return d; };
      const queue = [...this.state.nodes.keys()].filter(id => retry ? id === retry : !['exhausted', 'failed', 'malformed'].includes(this.state.branches.get(id)!.state));
      queue.sort((a,b) => depth(a)-depth(b) || a.localeCompare(b));
      for (let i = 0; i < queue.length; i++) {
        if (this.disposed) return;
        const id = queue[i], branch = this.state.branches.get(id)!;
        branch.depth = depth(id);
        if (branch.depth >= this.budgets.depth) { branch.state = 'limited'; branch.reason = 'Depth limit'; continue; }
        while (true) {
          const reason = limit() || (this.pages >= this.budgets.pages ? 'Page limit' : undefined) || (this.state.nodes.size >= this.budgets.nodes ? 'Node limit' : undefined);
          if (reason) { branch.state = 'limited'; branch.reason = reason; this.state.stop = reason; break; }
          branch.state = 'loading'; branch.reason = undefined; this.emit();
          const token = branch.token; this.pages++;
          try {
            const page = await call(() => this.source.children(this.tenant, id, token, signal));
            if (this.disposed) return;
            if (!Array.isArray(page.nodes)) throw new Error('Malformed child page');
            this.state.observed = page.observed_at;
            let omitted = false, malformed = false;
            for (const node of page.nodes) {
              const existed = this.state.nodes.has(node.job_id);
              if (!this.accept(node, id)) {
                if (!existed && this.state.nodes.size >= this.budgets.nodes) omitted = true;
                else malformed = true;
                continue;
              }
              if (!existed) queue.push(node.job_id);
            }
            if (omitted) { branch.state = 'limited'; branch.reason = 'Node limit; page records omitted. Open branch as entry'; break; }
            if (malformed) { branch.state = 'malformed'; branch.reason = 'Malformed relationships omitted'; break; }
            if (!page.next_page_token) { branch.state = 'exhausted'; break; }
            if (page.next_page_token === token || branch.seen.has(page.next_page_token)) { branch.state = 'malformed'; branch.reason = 'Repeated cursor; incomplete branch'; break; }
            branch.seen.add(page.next_page_token); branch.token = page.next_page_token; branch.state = 'more'; this.emit();
          } catch {
            if (this.disposed) return;
            branch.state = signal.aborted ? 'limited' : 'failed'; branch.reason = signal.aborted ? 'Elapsed-time limit' : 'Child page unavailable';
            if (signal.aborted) this.state.stop = 'Elapsed-time limit';
            break;
          }
        }
        this.emit();
        if (limit() || this.pages >= this.budgets.pages) { this.state.stop ||= limit() || 'Page limit'; break; }
      }
    } catch { if (!this.disposed) { if (!this.initialized) this.state.error = 'Entry job unavailable. Retry or return to jobs.'; else this.state.stop = limit() || 'Discovery interrupted'; } }
    finally { clearTimeout(timeout); if (!this.disposed) { this.state.loading = false; this.emit(); } }
  }
}
