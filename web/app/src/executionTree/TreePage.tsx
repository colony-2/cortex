import { useEffect, useReducer, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { TreeNode } from '@colony2/shared';
import { Traversal } from './traversal';

const jobPath = (tenant: string, job: string) => `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}`;
export default function TreePage() {
  const { projectId = '', jobId = '' } = useParams();
  return <TreeView key={JSON.stringify([projectId, jobId])} tenant={projectId} entry={jobId} />;
}
function TreeView({ tenant, entry }: { tenant: string; entry: string }) {
  const [, update] = useReducer(n => n + 1, 0);
  const [version, refresh] = useReducer(n => n + 1, 0);
  const traversal = useRef<Traversal>();
  const [selected, select] = useState(entry), [collapsed, collapse] = useState(new Set<string>()), [search, setSearch] = useState(''), [zoom, setZoom] = useState(1);
  const viewport = useRef<HTMLDivElement>(null);
  const selectedCard = useRef<HTMLButtonElement>(null);
  const entryOutline = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const controller = new Traversal(tenant, entry, update); traversal.current = controller;
    select(entry); collapse(new Set()); void controller.run();
    return () => controller.cancel();
  }, [tenant, entry, version]);
  const controller = traversal.current, state = controller?.state;
  const nodes = state ? [...state.nodes.values()] : [];
  const byID = state?.nodes || new Map<string, TreeNode>();
  const matches = new Set(nodes.filter(n => search && [n.job_id,n.cell,n.job_type,n.goal_id,n.status,n.completion_status].some(v => v?.toLowerCase().includes(search.toLowerCase()))).map(n => n.job_id));
  const revealed = new Set<string>();
  for (const id of matches) { let p: string | undefined = id; while (p && !revealed.has(p)) { revealed.add(p); p = byID.get(p)?.parent_job_id; } }
  const children = new Map<string, TreeNode[]>();
  const roots: TreeNode[] = [];
  for (const n of nodes) {
    if (n.parent_job_id && byID.has(n.parent_job_id) && !state?.blocked.has(n.job_id)) children.set(n.parent_job_id, [...(children.get(n.parent_job_id) || []), n]);
    else roots.push(n);
  }
  const sorted = (items: TreeNode[]) => items.sort((a,b) => a.job_id.localeCompare(b.job_id));
  const visibleChildren = (id: string) => collapsed.has(id) && !revealed.has(id) ? [] : sorted(children.get(id) || []);
  const positions = new Map<string, { x: number; y: number }>();
  let nextX = 0, maxDepth = 0;
  const place = (node: TreeNode, depth: number) => {
    const kids = visibleChildren(node.job_id); const first = nextX;
    kids.forEach(child => place(child,depth+1));
    if (!kids.length) nextX += 260;
    positions.set(node.job_id,{x:(first+nextX-260)/2+20,y:depth*150+20}); maxDepth = Math.max(depth,maxDepth);
  };
  sorted(roots).forEach(n => place(n,0));
  const toggle = (id: string) => collapse(previous => { const next = new Set(previous); next.has(id) ? next.delete(id) : next.add(id); return next; });
  const findEntry = () => {
    select(entry); collapse(new Set());
    requestAnimationFrame(() => {
      const target = window.matchMedia('(max-width: 767px)').matches ? entryOutline.current : selectedCard.current;
      target?.focus({preventScroll:true}); target?.scrollIntoView({block:'center',inline:'center',behavior:'smooth'});
    });
  };
  const branchText = (id: string) => { const b = state?.branches.get(id); return b?.reason || (b?.state === 'exhausted' && !(children.get(id)?.length) ? 'No children found in this traversal' : b?.state || 'pending'); };
  const label = (n: TreeNode) => <><strong>{n.job_id}{n.job_id === entry ? ' · Entry job' : ''}</strong><span>{n.cell || 'Unknown cell'} · {n.job_type || 'Unknown type'}</span><span>{n.status || 'Unknown status'}{n.completion_status ? ` · ${n.completion_status}` : ''}</span></>;
  const outline = (items: TreeNode[]) => <ul>{sorted(items).map(n => <li key={n.job_id}>
    <button ref={n.job_id === entry ? entryOutline : undefined} aria-pressed={selected === n.job_id} className={matches.has(n.job_id) ? 'tree-match' : ''} onClick={() => select(n.job_id)}>{label(n)}</button>
    {!!children.get(n.job_id)?.length && <button aria-label={`Toggle branch ${n.job_id}`} aria-expanded={!collapsed.has(n.job_id) || revealed.has(n.job_id)} onClick={() => toggle(n.job_id)}>{collapsed.has(n.job_id) ? 'Expand' : 'Collapse'}</button>}
    <small>{collapsed.has(n.job_id) ? 'Collapsed · ' : ''}{branchText(n.job_id)}</small>
    {!!visibleChildren(n.job_id).length && outline(visibleChildren(n.job_id))}
  </li>)}</ul>;
  const detail = byID.get(selected);
  const incomplete = !!state?.boundary || !!state?.stop || !!state?.warnings.length || [...(state?.branches.values() || [])].some(b => b.state !== 'exhausted');
  return <main className="app-page execution-tree">
    <nav><Link className="compact-touch-link" to={`/project/${encodeURIComponent(tenant)}/goals`}>Back to jobs</Link></nav>
    <h1>Execution tree</h1>
    <p>Recorded submission family · Tenant {tenant} · Entry {entry}</p>
    <div className="tree-toolbar">
      <button onClick={() => refresh()}>Refresh tree</button>
      <button onClick={() => void controller?.run()} disabled={state?.loading || !controller?.canResume}>Resume discovery</button>
      <button onClick={findEntry}>Find entry job</button>
      <button aria-label="Zoom out" onClick={() => setZoom(z => Math.max(.2,z-.2))}>−</button>
      <output aria-label="Zoom">{Math.round(zoom*100)}%</output>
      <button aria-label="Zoom in" onClick={() => setZoom(z => Math.min(2,z+.2))}>+</button>
      <button onClick={() => setZoom(Math.min(1,(viewport.current?.clientWidth || 800)/Math.max(260,nextX+40)))}>Fit</button>
      <label>Loaded jobs <input aria-label="Search loaded jobs" type="search" value={search} onChange={e => setSearch(e.target.value)} /></label>
      {search && <span>{matches.size} loaded matches</span>}
    </div>
    <section className="tree-coverage" aria-label="Discovery coverage" aria-live="polite">
      <strong>{!state || state.loading ? 'Discovering…' : state?.error ? 'Entry unavailable' : incomplete ? 'Partial discovery' : 'Discovery finished: all reachable records returned by authorized listings during this traversal.'}</strong>
      <p>{nodes.length} loaded jobs · Started {state?.started || '…'} · Latest observation {state?.observed || 'pending'}</p>
      <p>Authorized retained records only. Missing intermediates may prevent discovery. Observations occur at different times; this is not an atomic snapshot.</p>
      {state?.boundary && <p>{state.boundary}</p>}{state?.stop && <p>{state.stop}</p>}
      {state?.warnings.map(w => <p key={w}>{w}</p>)}
    </section>
    {state?.error ? <div role="alert"><p>{state.error}</p><button onClick={() => refresh()}>Retry entry</button></div> : <div className="tree-workspace">
      <div>
        <div ref={viewport} className="tree-viewport" aria-label="Execution diagram">
          <div style={{width:Math.max(300,nextX+40)*zoom,height:(maxDepth*150+150)*zoom}}>
            <div className="tree-canvas" style={{width:Math.max(300,nextX+40),height:maxDepth*150+150,transform:`scale(${zoom})`}}>
              <svg aria-label="Runtime parent connections" width="100%" height="100%">{[...positions].map(([id,p]) => {
                const parent = byID.get(id)?.parent_job_id, start = parent && !state?.blocked.has(id) ? positions.get(parent) : undefined;
                return start ? <path key={id} data-parent={parent} data-child={id} d={`M ${start.x+110} ${start.y+100} V ${p.y-20} H ${p.x+110} V ${p.y}`} /> : null;
              })}</svg>
              {[...positions].map(([id,p]) => { const n = byID.get(id)!; return <div key={id} className={`tree-card ${id === entry ? 'tree-entry' : ''} ${matches.has(id) ? 'tree-match' : ''}`} style={{left:p.x,top:p.y}}>
                <button ref={id === entry ? selectedCard : undefined} aria-pressed={selected === id} onClick={() => select(id)}>{label(n)}</button>
                {!!children.get(id)?.length && <button aria-label={`Diagram toggle ${id}`} aria-expanded={!collapsed.has(id) || revealed.has(id)} onClick={() => toggle(id)}>{collapsed.has(id) ? 'Expand' : 'Collapse'}</button>}
              </div>; })}
            </div>
          </div>
        </div>
        <section aria-label="Execution outline" className="tree-outline"><h2>Outline</h2>{outline(roots)}</section>
      </div>
      {detail && <aside aria-label="Selected job details" className="tree-details"><h2>Selected job</h2>
        <dl>{Object.entries({Tenant:detail.tenant_id,Job:detail.job_id,Cell:detail.cell || 'Unknown',Type:detail.job_type,Status:detail.status,Completion:detail.completion_status || 'Unknown',Created:detail.created_at || 'Unknown','Runtime parent':detail.parent_job_id || 'No recorded parent','Recipe attribution':detail.recipe_parent_job_id || 'None recorded',Coverage:branchText(detail.job_id)}).map(([key,value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
        {detail.goal_id && <p><Link className="compact-touch-link" to={`/project/${encodeURIComponent(tenant)}/goals/${encodeURIComponent(detail.goal_id)}`}>Goal · {detail.goal_id}</Link></p>}
        {detail.job_type === 'recipe' ? <p><Link className="compact-touch-link" to={`${jobPath(tenant,detail.job_id)}/story`} state={{ treeReturnTo: `${jobPath(tenant,entry)}/tree` }}>Open story</Link></p> : <p>Generic job summary; no recipe story available.</p>}
        {state?.branches.get(detail.job_id)?.state === 'failed' && <button disabled={state.loading || controller?.viewCapped} onClick={() => void controller?.run(detail.job_id)}>Retry child page</button>}
        {(controller?.viewCapped || ['limited','malformed'].includes(state?.branches.get(detail.job_id)?.state || '')) && <p><Link className="compact-touch-link" to={`${jobPath(tenant,detail.job_id)}/tree`}>Open branch as entry</Link></p>}
      </aside>}
    </div>}
  </main>;
}
