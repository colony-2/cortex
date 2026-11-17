import { useEffect, useMemo, useReducer, useRef } from 'react';
import { getTreeWaits, type TreeWaitSnapshot, type TreeWaitsResponse } from '@colony2/shared';
import { legacyRows, normalizeWaits, WAIT_LIMITS, type WaitRow } from './waits';

export interface WaitObservation {
  rows: WaitRow[]; issues: string[]; loading: boolean; stale: boolean;
  loaded: boolean; observed?: string; snapshot?: TreeWaitSnapshot; token: string;
  pages: number; inspected: number; error?: string; stopped?: string;
}
const empty = (): WaitObservation => ({rows:[],issues:[],loading:false,stale:false,loaded:false,token:'',pages:0,inspected:0});
export class TreeWaits {
  state = empty();
  private abort?: AbortController;
  private serial = 0;
  private seen = new Set<string>();
  private first = true;
  private interrupted = true;
  private disposed = false;
  constructor(readonly tenant: string, readonly consumer: string, private changed: () => void,
    private read = getTreeWaits) {}
  get canLoad() { return !this.state.stale && !!this.state.token && !this.state.stopped && !this.state.loading; }
  get rows() {
    const s = this.state;
    return s.loaded && !s.token ? [...s.rows,...legacyRows(this.tenant,s.snapshot?.legacy_wait_for || []).slice(0,WAIT_LIMITS.rows-s.rows.length)] : s.rows;
  }
  pause() { this.interrupted ||= this.state.loading; ++this.serial; this.abort?.abort(); this.state.loading = false; }
  dispose() { this.disposed = true; this.pause(); }
  resume() { if (this.interrupted) { this.interrupted = false; void this.load(); } }
  refresh() {
    this.pause(); this.first = true; this.seen.clear(); this.interrupted = false;
    this.state = {...this.state,stale:this.state.loaded,error:undefined,stopped:undefined};
    void this.load();
  }
  async load() {
    if (this.disposed || !this.consumer || this.state.loading || (!this.first && !this.canLoad)) return;
    const serial = ++this.serial, first = this.first, token = first ? '' : this.state.token;
    const abort = new AbortController(); this.abort = abort;
    const timeout = setTimeout(()=>abort.abort(),10000);
    this.state = {...this.state,loading:true,error:undefined}; this.changed();
    try {
      const page: TreeWaitsResponse = await this.read(this.tenant,this.consumer,token,abort.signal);
      if (serial !== this.serial || this.disposed) return;
      if (first) this.state = empty();
      const s = this.state;
      const normalized = normalizeWaits(page,this.tenant,this.consumer,WAIT_LIMITS.rows-s.inspected);
      const keys = new Set(s.rows.map(r=>r.key));
      const rows = normalized.rows.filter(row=> { if(keys.has(row.key)) { normalized.issues.push('Duplicate wait entry omitted');return false; } keys.add(row.key);return true; });
      this.seen.add(token);
      const next = page.next_page_token || '', pages = s.pages+1, inspected = s.inspected+normalized.inspected;
      const stopped = next && this.seen.has(next) ? 'Repeated cursor; wait coverage is partial'
        : pages >= WAIT_LIMITS.pages && next ? 'Wait page limit reached'
        : inspected >= WAIT_LIMITS.rows ? 'Dependency row limit reached' : undefined;
      if(first && page.consumer_snapshot?.legacy_wait_for?.length && page.consumer_snapshot.legacy_wait_for.length>WAIT_LIMITS.rows) normalized.issues.push('Legacy reference limit reached');
      if(!next && ((s.snapshot || page.consumer_snapshot)?.legacy_wait_for?.length || 0) > WAIT_LIMITS.rows-s.rows.length-rows.length) normalized.issues.push('Legacy reference limit reached');
      this.state = {...s,rows:[...s.rows,...rows],issues:[...new Set([...s.issues,...normalized.issues])],loaded:true,stale:false,
        observed:page.observed_at,snapshot:first?page.consumer_snapshot:s.snapshot,token:next,pages,inspected,stopped,loading:false};
      this.first = false;
    } catch {
      if (serial !== this.serial || this.disposed) return;
      this.state = {...this.state,loading:false,error:first?'Pending dependency information unavailable':'Wait page unavailable; loaded information is partial'};
    } finally { clearTimeout(timeout); if (serial === this.serial && !this.disposed) this.changed(); }
  }
}
export function useTreeWaits(tenant: string, entry: string, consumer: string, version: number, active: boolean) {
  const [, changed] = useReducer(n=>n+1,0);
  const activeRef = useRef(active);activeRef.current = active;
  const controller = useMemo(()=>new TreeWaits(tenant,consumer,changed),[tenant,entry,consumer]);
  useEffect(()=>{
    let current = true;
    queueMicrotask(()=>{if(current && activeRef.current) controller.refresh();});
    return ()=>{current = false;controller.pause();};
  },[controller,version]); // Visibility resumes interrupted reads without a new observation.
  useEffect(()=>{
    let current = true;
    if (!active) controller.pause();
    else queueMicrotask(()=>{if(current && activeRef.current) controller.resume();});
    return ()=>{current=false;controller.pause();};
  },[controller,active]);
  return controller;
}
