import { Link } from 'react-router-dom';
import { useState } from 'react';
import type { WaitController } from './waits';
import { conditionText, WAIT_LIMITS, waitPath, type WaitTarget } from './waits';

export function TargetLink({tenant,target,returnTo}:{tenant:string;target:WaitTarget;returnTo:string}) {
  return <Link className="compact-touch-link" to={waitPath(tenant,target.kind,target.id)} state={{treeWaitFrom:returnTo}}>{target.kind==='goal'?'Open goal':'Open job DAG'}</Link>;
}
export function WaitPane({controller,targets,drawn,selectTarget,showTarget,returnTo}:{controller:WaitController;targets:WaitTarget[];drawn:Set<string>;selectTarget:(t:WaitTarget)=>void;showTarget:(t:WaitTarget)=>void;returnTo:string}) {
  const [count,setCount] = useState(WAIT_LIMITS.paneRows);
  const s = controller.state;
  const rows = controller.rows;
  const complete = s.loaded && !s.stale && !s.loading && !s.error && !s.token && !s.stopped && !s.issues.length && s.snapshot?.state==='available';
  return <section className={`tree-waits ${s.stale?'tree-waits-previous':''}`} aria-label="Waits for">
    <h3>Waits for</h3>
    <div className="tree-wait-actions"><button onClick={()=>controller.refresh()} disabled={s.loading}>Refresh waits</button></div>
    <div role="status" aria-live="polite">
      {s.stale && <strong>Previous observation · awaiting verification</strong>}
      {s.loading && <p>Loading pending dependencies…</p>}
      {s.error && <p>{s.error}</p>}
      {s.loaded && !s.stale && <p>{complete?'Wait coverage complete':'Partial wait coverage'} · {rows.length} loaded rows · {targets.length} distinct targets</p>}
      {s.observed && <p>Observed {s.observed}. Pages are separate observations.</p>}
      {s.snapshot?.state==='unavailable' && <p>Supplementary job information unavailable; absence of waits is unverified.</p>}
      {s.issues.map(issue=><p key={issue}>{issue}</p>)}
      {s.stopped && <p>{s.stopped}</p>}
      {complete && !rows.length && <p>{s.snapshot?.status==='PENDING_JOBS'?'Returned dependency details do not explain the waiting status. Refresh to check again.':`No unresolved waits returned at ${s.observed}.`}</p>}
    </div>
    {!!s.error && (s.stale || !s.loaded) && <button onClick={()=>s.stale || !s.loaded?controller.refresh():void controller.load()} disabled={s.loading}>Retry waits</button>}
    {s.token && s.error && !s.stale && !s.stopped && <button onClick={()=>void controller.load()} disabled={!controller.canLoad}>{s.error?'Retry page':'Load more waits'}</button>}
    {!!targets.length && <p>{targets.filter(target=>drawn.has(target.key)).length} of {targets.length} loaded targets drawn{s.token?'; more may exist':''}. Job targets in the opened scope are followed automatically.</p>}
    <ul className="tree-wait-list">{rows.slice(0,count).map(row=>{
      const target = targets.find(t=>t.key===row.targetKey)!;
      return <li key={JSON.stringify([row.key,row.targetKey,row.condition,row.deadline,row.blockedReason])} data-wait-ref={row.targetKey}>
        <strong>{row.legacy?'Legacy job wait':row.kind==='goal'?'Goal wait':'Job wait'}</strong>
        <code>{row.id}</code>
        <p>Waits for · {row.condition}</p>
        <p>{row.legacy?'Condition unavailable for legacy reference':conditionText(row.condition)}</p>
        <small>Wait {row.waitID}{row.deadline?` · Deadline ${row.deadline}`:''}</small>
        {row.blockedReason && <p>{row.blockedReason}</p>}
        <div className="tree-wait-actions">
          <button onClick={()=>selectTarget(target)}>Select target</button>
          <TargetLink tenant={controller.tenant} target={target} returnTo={returnTo} />
          <button onClick={()=>showTarget(target)}>{drawn.has(row.targetKey)?'Reveal in graph':'Show in graph'}</button>
        </div>
      </li>;
    })}</ul>
    {rows.length>count && <button onClick={()=>setCount(c=>c+WAIT_LIMITS.paneRows)}>Show more loaded targets</button>}
  </section>;
}
