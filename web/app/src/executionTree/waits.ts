import type { TreeWaitsResponse } from '@colony2/shared';

export const WAIT_LIMITS = { pages: 10, rows: 1000, targets: 50, arrows: 100, paneRows: 100 };
export interface WaitRow {
  key: string; targetKey: string; kind: 'job' | 'goal'; id: string;
  waitID: string; condition: string; deadline?: string; blockedReason?: string; legacy?: boolean;
}
export interface WaitTarget { key: string; kind: 'job' | 'goal'; id: string; rows: WaitRow[] }
export const waitKey = (tenant: string, kind: 'job' | 'goal', id: string) => JSON.stringify([tenant,kind,id]);
export const waitPath = (tenant: string, kind: 'job' | 'goal', id: string) => kind === 'goal'
  ? `/project/${encodeURIComponent(tenant)}/goals/${encodeURIComponent(id)}`
  : `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(id)}/tree`;
const issueLabels: Record<string,string> = {
  missing_wait_records:'Pending wait records were missing',
  mismatched_consumer:'Records for another consumer were omitted',
  invalid_wait_record:'Invalid wait records were omitted',
  invalid_dependency:'Some target identities or conditions were invalid',
  unknown_dependency_state:'Some dependency states were unavailable',
  wait_page_record_limit:'Wait record page limit reached',
};
const nonblank = (v: unknown): v is string => typeof v === 'string' && !!v.trim();
const resolved = new Set(['SATISFIED','FAILED','TIMED_OUT','CANCELLED']);

/** Treat successful but malformed records as incomplete, never verified absence. */
export function normalizeWaits(page: TreeWaitsResponse, tenant: string, consumer: string, budget: number) {
  const rows: WaitRow[] = [], issues = new Set(page.issues.map(issue=>issueLabels[issue] || issue));
  let inspected = 0;
  if(page.waits.length>20) issues.add('Wait record page limit reached');
  for (const wait of page.waits.slice(0,20)) {
    if (inspected >= budget) { issues.add('Dependency row limit reached'); break; }
    // Even omitted records consume their inspected entry budget.
    const countOmitted = () => { if(Array.isArray(wait?.dependencies)) inspected += Math.min(wait.dependencies.length,budget-inspected); };
    if (wait?.jobKey?.tenantId !== tenant || wait?.jobKey?.jobId !== consumer) { countOmitted();issues.add('Mismatched consumer omitted'); continue; }
    if (!nonblank(wait.waitId) || wait.state !== 'PENDING' || !Array.isArray(wait.dependencies)) { countOmitted();issues.add('Invalid wait record omitted'); continue; }
    let deadline: string | undefined;
    if(wait.deadline !== undefined) {
      if(typeof wait.deadline==='string' && Number.isFinite(Date.parse(wait.deadline))) deadline=wait.deadline;
      else issues.add('Invalid wait deadline omitted');
    }
    for (let index = 0; index < wait.dependencies.length; index++) {
      if (inspected >= budget) { issues.add('Dependency row limit reached'); break; }
      inspected++;
      const d = wait.dependencies[index];
      if (!d || !nonblank(d.state)) { issues.add('Invalid dependency state omitted'); continue; }
      if (resolved.has(d.state)) continue;
      if (d.state !== 'PENDING') { issues.add('Unknown dependency state omitted'); continue; }
      const job = nonblank(d.target?.jobId), goal = nonblank(d.target?.goalId);
      if (job === goal || !nonblank(d.condition)) { issues.add('Invalid target or condition omitted'); continue; }
      const kind = job ? 'job' : 'goal', id = (job ? d.target.jobId : d.target.goalId)!;
      if (!['complete','success'].includes(d.condition)) issues.add('Condition explanation unavailable; literal value retained');
      if(d.blockedReason!==undefined && typeof d.blockedReason!=='string') issues.add('Invalid blocked reason omitted');
      rows.push({key:JSON.stringify([wait.waitId,index]),targetKey:waitKey(tenant,kind,id),kind,id,waitID:wait.waitId,condition:d.condition,deadline,blockedReason:typeof d.blockedReason==='string'?d.blockedReason:undefined});
    }
    if (inspected >= budget) break;
  }
  // At an exact boundary conservatively report a cap: no further automatic discovery.
  if (inspected >= budget) issues.add('Dependency row limit reached');
  return { rows, issues:[...issues], inspected };
}
export function legacyRows(tenant: string, ids: string[]): WaitRow[] {
  return [...new Set(ids)].filter(nonblank).slice(0,WAIT_LIMITS.rows).map(id=>({key:JSON.stringify(['legacy',id]),targetKey:waitKey(tenant,'job',id),kind:'job',id,waitID:'Legacy job wait',condition:'condition unavailable',legacy:true}));
}
export function waitTargets(rows: WaitRow[]): WaitTarget[] {
  const targets = new Map<string,WaitTarget>();
  for (const row of rows) {
    if (!targets.has(row.targetKey)) targets.set(row.targetKey,{key:row.targetKey,kind:row.kind,id:row.id,rows:[]});
    targets.get(row.targetKey)!.rows.push(row);
  }
  return [...targets.values()];
}
export function graphTargets(targets: WaitTarget[], preferred?: string) {
  const drawn = targets.slice(0, Math.min(WAIT_LIMITS.targets,WAIT_LIMITS.arrows));
  const chosen = targets.find(t=>t.key===preferred);
  if (chosen && !drawn.includes(chosen)) drawn[drawn.length-1] = chosen;
  return drawn;
}
export const conditionText = (condition: string) => condition === 'complete' ? 'Completion, regardless of outcome' : condition === 'success' ? 'Successful completion' : 'Condition explanation unavailable';
