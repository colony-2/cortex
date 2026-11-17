import { Button, Tooltip } from 'antd';
import { ExportOutlined, FlagOutlined, StopOutlined } from '@ant-design/icons';
import JobActionDialog from '../components/JobActionDialog';
import { isFinishedJob } from '../utils/jobStatus';
import { useJobCancellation } from './useJobCancellation';
import { useEffect, useReducer, useRef, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import type { TreeNode } from '@colony2/shared';
import { dagPath, dagReturnPath } from './navigation';
import { Traversal } from './traversal';
import { headline, jobStatus, statusPalette } from './presentation';
import { timing } from './timing';
import { useTreeWaits } from './useTreeWaits';
import { graphTargets, waitTargets, type WaitTarget } from './waits';
import { TargetLink, WaitPane } from './WaitPane';

export const GEOMETRY = { width: 272, height: 228, columnGap: 72, rowGap: 36, padding: 24 };

const jobPath = (tenant: string, job: string) => `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}`;
export default function TreePage() {
  const { projectId = '', jobId = '' } = useParams();
  const location = useLocation();
  const returnTo = dagReturnPath(location.state?.treeWaitFrom, projectId);
  return <main>{returnTo && <nav><Link className="compact-touch-link" to={returnTo}>Back to waiting job DAG</Link></nav>}<TreeWorkspace key={JSON.stringify([projectId, jobId])} tenant={projectId} entry={jobId} /></main>;
}
export function TreeWorkspace({ tenant, entry, active = true, embedded = false, onJobChanged }: { onJobChanged?: (node: TreeNode) => void; tenant: string; entry: string; active?: boolean; embedded?: boolean }) {
  const [, update] = useReducer(n => n + 1, 0);
  const [version, refresh] = useReducer(n => n + 1, 0);
  const traversal = useRef<Traversal>();
  const [selected, select] = useState(entry), [collapsed, collapse] = useState(new Set<string>()), [search, setSearch] = useState(''), [zoom, setZoom] = useState(1);
  const cancellation = useJobCancellation(tenant, entry, onJobChanged);
  const detailHeading = useRef<HTMLHeadingElement>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const selectedCard = useRef<HTMLButtonElement>(null);
  const initiallyRevealed = useRef(false);
  const [reference, setReference] = useState<string>();
  const [preferredTarget, setPreferredTarget] = useState<string>();
  const focusedWait = useRef<{key:string;element:HTMLElement}>();
  const [now, setNow] = useState(Date.now);
  const [documentVisible, setDocumentVisible] = useState(document.visibilityState !== 'hidden');
  useEffect(() => {
    const changed = () => setDocumentVisible(document.visibilityState !== 'hidden');
    document.addEventListener('visibilitychange', changed);
    return () => document.removeEventListener('visibilitychange', changed);
  }, []);
  const visible = active && documentVisible;
  const waits = useTreeWaits(tenant, entry, selected, version, visible);
  const targets = waitTargets(waits.rows);
  const drawnTargets = graphTargets(targets,preferredTarget);
  const drawnKeys = new Set(drawnTargets.map(t=>t.key));
  const selectedReference = targets.find(t=>t.key===reference);
  const visibleRef = useRef(visible);
  visibleRef.current = visible;
  useEffect(() => {
    if (!visible) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [visible]);
  // Identity/refresh isolates discovery; tab departure retains its cursors.
  useEffect(() => {
    const controller = new Traversal(tenant, entry, update); traversal.current = controller;
    select(entry); setReference(undefined); collapse(new Set()); initiallyRevealed.current = false;
    controller.interrupted = true;
    // Effect replay must dispose its first controller before transport starts.
    let current = true;
    queueMicrotask(() => { if (current && visibleRef.current && controller.interrupted) void controller.run(); });
    return () => { current = false; controller.cancel(); };
  }, [tenant, entry, version]);
  useEffect(() => {
    const controller = traversal.current;
    if (!visible) controller?.pause();
    else queueMicrotask(() => {
      // Recheck visibility and identity; another queued start may have resumed it.
      if (traversal.current === controller && visibleRef.current && controller?.interrupted) void controller.run();
    });
  }, [visible]);
  useEffect(() => {
    const state = traversal.current?.state;
    if (!visible || !state || state.loading || !state.nodes.has(entry) || initiallyRevealed.current) return;
    initiallyRevealed.current = true;
    // A user selection during discovery wins over initial reveal.
    if (selected === entry) selectedCard.current?.scrollIntoView?.({block:'center',inline:'center'});
  });
  const controller = traversal.current, state = controller?.state;
  const nodes = state ? [...state.nodes.values()].map(cancellation.overlay) : [];
  const byID = new Map(nodes.map(node => [node.job_id, node]));
  const matches = new Set(nodes.filter(n => search && [n.job_id,n.cell,n.job_type,n.goal_id,n.status,n.completion_status,n.title,n.prompt].some(v => v?.toLowerCase().includes(search.toLowerCase()))).map(n => n.job_id));
  const revealed = new Set<string>();
  for (const id of matches) { let p: string | undefined = id; while (p && !revealed.has(p)) { revealed.add(p); p = byID.get(p)?.parent_job_id; } }
  // Reveal reused runtime targets through a separate set; saved collapse remains intact.
  const runtimeWaitIDs = [...drawnTargets.filter(t=>t.kind==='job' && byID.has(t.id)).map(t=>t.id),...(drawnTargets.length?[selected]:[])];
  for (const id of runtimeWaitIDs) {
    let p: string | undefined = id;
    const seen = new Set<string>();
    while(p && !seen.has(p)) { seen.add(p); revealed.add(p); p = byID.get(p)?.parent_job_id; }
  }
  const children = new Map<string, TreeNode[]>();
  const roots: TreeNode[] = [];
  for (const n of nodes) {
    if (n.parent_job_id && byID.has(n.parent_job_id) && !state?.blocked.has(n.job_id)) children.set(n.parent_job_id, [...(children.get(n.parent_job_id) || []), n]);
    else roots.push(n);
  }
  const sorted = (items: TreeNode[]) => items.sort((a,b) => a.job_id.localeCompare(b.job_id));
  const visibleChildren = (id: string) => collapsed.has(id) && !revealed.has(id) ? [] : sorted(children.get(id) || []);
  const positions = new Map<string, { x: number; y: number }>();
  const {width, height, columnGap, rowGap, padding} = GEOMETRY;
  let nextY = 0, maxDepth = 0;
  const place = (node: TreeNode, depth: number) => {
    const kids = visibleChildren(node.job_id); const first = nextY;
    kids.forEach(child => place(child, depth+1));
    if (!kids.length) nextY += height + rowGap;
    positions.set(node.job_id, {x:padding + depth*(width+columnGap), y:padding+(drawnTargets.length?64:0)+(first+nextY-height-rowGap)/2});
    maxDepth = Math.max(depth,maxDepth);
  };
  sorted(roots).forEach(n => place(n,0));
  const runtimeWidth = padding*2 + maxDepth*(width+columnGap)+width;
  const referencePositions = new Map<string,{x:number;y:number}>();
  let referenceIndex = 0;
  for(const target of drawnTargets) {
    if(target.kind === 'job' && positions.has(target.id)) continue;
    referencePositions.set(target.key,{x:runtimeWidth+120,y:padding+64+referenceIndex++*(height+rowGap)});
  }
  const canvasWidth = runtimeWidth+(referencePositions.size?width+144:0);
  const canvasHeight = Math.max(height+padding*2,nextY-rowGap+padding*2,referenceIndex*(height+rowGap)+padding*2)+(drawnTargets.length?64:0);
  const selectRuntime = (id: string) => { select(id); setReference(undefined); setPreferredTarget(undefined); };
  const selectTarget = (target: WaitTarget) => {
    if(target.kind==='job' && byID.has(target.id)) selectRuntime(target.id);
    else setReference(target.key);
  };
  const showTarget = (target: WaitTarget) => {
    setPreferredTarget(target.key);
    requestAnimationFrame(()=>{const element=[...(viewport.current?.querySelectorAll<HTMLElement>('[data-wait-ref],[data-job-id]') || [])].find(e=>e.dataset.waitRef===target.key || target.kind==='job' && e.dataset.jobId===target.id);
      element?.scrollIntoView?.({block:'center',inline:'center',behavior:'smooth'});});
  };
  // Restore focus only when a removed/merged reference owned it.
  useEffect(()=>{
    const focused = focusedWait.current;
    if(focused && !focused.element.isConnected && (!document.activeElement || document.activeElement===document.body)) {
      focusedWait.current=undefined;
      requestAnimationFrame(()=>detailHeading.current?.focus({preventScroll:true}));
    }
    if(!reference) return;
    const target = targets.find(t=>t.key===reference);
    const merged = target?.kind==='job' && byID.has(target.id);
    if(target && !merged) return;
    setReference(undefined);
    if(merged && target) select(target.id);
  });
  const toggle = (id: string) => collapse(previous => { const next = new Set(previous); next.has(id) ? next.delete(id) : next.add(id); return next; });
  const findEntry = () => {
    select(entry); setReference(undefined); collapse(new Set());
    requestAnimationFrame(() => {
      const target = selectedCard.current;
      target?.focus({preventScroll:true}); target?.scrollIntoView({block:'center',inline:'center',behavior:'smooth'});
    });
  };
  const branchText = (id: string) => { const b = state?.branches.get(id); return b?.reason || (b?.state === 'exhausted' && !(children.get(id)?.length) ? 'No children found in this traversal' : b?.state || 'pending'); };
  const root = state?.confirmedRoot ? byID.get(state.confirmedRoot) : undefined;
  const label = (n: TreeNode) => {
    const status = jobStatus(n), time = timing(n, root, now);
    return <><span className="tree-cell">{n.cell || 'Unknown cell'}</span>
      <strong title={n.title || n.prompt}>{headline(n)}</strong>
      <span className="tree-status">{status.label || 'Unknown status'}</span>
      <span>{time.submission}</span><span>{time.span}</span>
      {n.job_id === entry && <span className="tree-opened">Opened job</span>}</>;
  };
  const detail = byID.get(selected);
  const cancellationState = detail && cancellation.state(detail.job_id);
  const incomplete = !!controller?.canResume || !!state?.boundary || !!state?.stop || !!state?.warnings.length || [...(state?.branches.values() || [])].some(b => b.state !== 'exhausted');
  return <section onFocusCapture={event=>{const element=event.target as HTMLElement;const key=element.closest<HTMLElement>('[data-wait-ref]')?.dataset.waitRef;focusedWait.current=key?{key,element}:undefined;}} className={embedded ? "execution-tree" : "app-page execution-tree"}>
    {!embedded && <><nav><Link className="compact-touch-link" to={`/project/${encodeURIComponent(tenant)}/goals`}>Back to jobs</Link></nav>
    <h1>DAG</h1></>}
    <p>Recorded submission family · Tenant {tenant} · Opened job {entry}</p>
    <p className="tree-timing-note">Submission offsets are relative to the confirmed root. Age and time to finish include queue and waiting time.</p>
    <div className="tree-toolbar">
      <button onClick={() => refresh()}>Refresh tree</button>
      <button onClick={() => void controller?.run()} disabled={state?.loading || !controller?.canResume}>Resume discovery</button>
      <button onClick={findEntry}>Find opened job</button>
      <button aria-label="Zoom out" onClick={() => setZoom(z => Math.max(.05,z-.2))}>−</button>
      <output aria-label="Zoom">{Math.round(zoom*100)}%</output>
      <button aria-label="Zoom in" onClick={() => setZoom(z => Math.min(2,z+.2))}>+</button>
      <button onClick={() => setZoom(Math.max(.05,Math.min(1,(viewport.current?.clientWidth || 800)/canvasWidth)))}>Fit</button>
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
      <div className="tree-diagram-column">
        <p className="tree-wait-legend">Rounded card / solid line: runtime family · Chamfered card: job wait target · Diamond: goal wait target · Dashed arrow: waits for</p>
        {!!drawnTargets.length && <div className="tree-wait-actions"><button onClick={()=>showTarget(drawnTargets.find(t=>referencePositions.has(t.key)) || drawnTargets[0])}>Reveal wait targets</button><span>{drawnTargets.length} of {targets.length} loaded targets drawn{waits.state.stale?' · Previous observation':''}</span></div>}
        <div ref={viewport} className="tree-viewport" aria-label="Execution diagram">
          <div style={{width:canvasWidth*zoom,height:canvasHeight*zoom}}>
            <div className="tree-canvas" style={{width:canvasWidth,height:canvasHeight,transform:`scale(${zoom})`}}>
              <svg aria-label="Runtime parent connections" width="100%" height="100%">{[...positions].map(([id,p]) => {
                const parent = byID.get(id)?.parent_job_id, start = parent && !state?.blocked.has(id) ? positions.get(parent) : undefined;
                return start ? <path key={id} data-parent={parent} data-child={id} d={`M ${start.x+width} ${start.y+height/2} H ${start.x+width+columnGap/2} V ${p.y+height/2} H ${p.x}`} /> : null;
              })}</svg>
              <svg className={`tree-wait-connections ${waits.state.stale?'tree-waits-previous':''}`} aria-label="Waits-for connections" width="100%" height="100%">
                <defs><marker id={`wait-arrow-${embedded?'embedded':'standalone'}`} markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><polygon points="0,0 8,4 0,8" fill="#6941a5" /></marker></defs>
                {drawnTargets.map((target,index)=>{
                  const start=positions.get(selected), end=target.kind==='job'?positions.get(target.id)||referencePositions.get(target.key):referencePositions.get(target.key);
                  if(!start||!end) return null;
                  const lane=16+(index%4)*10;
                  const x1=start.x+width, y1=start.y+height/2-32;
                  const external=referencePositions.has(target.key);
                  const approach=external?runtimeWidth+30+(index%5)*12:end.x-24;
                  const path=target.kind==='job' && target.id===selected
                    ? `M ${x1} ${y1} H ${x1+24} V ${start.y-20} H ${start.x+width/2} V ${start.y}`
                    : `M ${x1} ${y1} H ${x1+24} V ${lane} H ${approach} V ${end.y+height/2-(external?0:32)} H ${end.x}`;
                  const conditions=[...new Set(target.rows.map(r=>r.legacy?'legacy · condition unavailable':r.condition))].join(', ');
                  return <g key={target.key}><path data-wait-target={target.key} markerEnd={`url(#wait-arrow-${embedded?'embedded':'standalone'})`} d={path}><title>Waits for · {conditions} · {target.id}</title></path>
                    <text x={end.x+width/2} y={end.y-10} textAnchor="middle">{waits.state.stale?'Previous observation · ':''}Waits for · {conditions.length>40?`${target.rows.length} conditions (see pane)`:conditions}</text></g>;
                })}
              </svg>
              {[...referencePositions].map(([key,p])=>{const target=drawnTargets.find(t=>t.key===key)!;return <button key={key} data-wait-ref={key} className={`tree-wait-reference tree-wait-${target.kind} ${waits.state.stale?'tree-waits-previous':''}`} style={{left:p.x,top:p.y,width,height}} aria-label={`${target.kind==='goal'?'Goal':'Job'} wait target · ${target.id}`} aria-pressed={reference===key} onClick={()=>selectTarget(target)}>
                <span>{target.kind==='goal'?'Goal wait target':'Job wait target reference'}</span><strong>{target.id}</strong><span>{waits.state.stale?'Previous observation':'Select to inspect'}</span>
              </button>;})}
              {[...positions].map(([id,p]) => { const n = byID.get(id)!; const [fill,border,text] = statusPalette[jobStatus(n).color] || statusPalette.default; return <div key={id} data-job-id={id} className={`tree-card ${id === entry ? 'tree-entry' : ''} ${matches.has(id) ? 'tree-match' : ''}`} style={{left:p.x,top:p.y,width,height,background:fill,borderColor:border,color:text}}>
                <button className="tree-card-select" aria-label={`${headline(n)} · ${n.cell || 'Unknown cell'}${id === entry ? ' · Opened job' : ''}`} ref={id === entry ? selectedCard : undefined} aria-pressed={!reference && selected === id} onClick={() => selectRuntime(id)}>{label(n)}{drawnTargets.some(t=>t.kind==='job'&&t.id===id) && <span className="tree-wait-badge">Wait target</span>}</button>
                {!!children.get(id)?.length && <button className="tree-branch" aria-label={`Toggle branch ${headline(n)}`} aria-expanded={!collapsed.has(id) || revealed.has(id)} onClick={() => toggle(id)}>{collapsed.has(id) ? 'Expand' : 'Collapse'}</button>}
              </div>; })}
            </div>
          </div>
        </div>

      </div>
      {detail && <aside aria-label={selectedReference?"Selected wait target":"Selected job details"} className="tree-details">
        <div className="tree-details-header">
          <h2 ref={detailHeading} tabIndex={-1}>{selectedReference?'Selected wait target':'Selected job'}</h2>
          {!selectedReference && <div className="tree-detail-actions" role="group" aria-label="Selected job actions">
            {cancellation.eligible(detail) && <Tooltip title="Cancel job" trigger={['hover', 'focus']}>
              <Button className="tree-detail-action tree-detail-cancel" danger aria-label="Cancel job" icon={<StopOutlined />} onClick={() => cancellation.open(detail)} />
            </Tooltip>}
            {detail.goal_id && <Tooltip title="Goal overview" trigger={['hover', 'focus']}>
              <Link className="tree-detail-action" aria-label="Goal overview" to={`/project/${encodeURIComponent(tenant)}/goals/${encodeURIComponent(detail.goal_id)}`}><FlagOutlined /></Link>
            </Tooltip>}
            {detail.job_type === 'recipe' && <Tooltip title="Open execution" trigger={['hover', 'focus']}>
              <Link className="tree-detail-action" aria-label="Open execution" to={`${jobPath(tenant,detail.job_id)}/story`} state={{ treeReturnTo: dagPath(tenant, entry, byID.get(entry)?.job_type) }}><ExportOutlined /></Link>
            </Tooltip>}
          </div>}
        </div>
        {selectedReference && <div className="tree-reference-details">
        {waits.state.stale && <p>Previous observation</p>}
        <p>{selectedReference.kind==='goal'?'Goal':'Job'} wait target</p><code className="tree-wait-identity">{selectedReference.id}</code>
        <p>Waiting job: {selected}</p>
        {selectedReference.rows.map(row=><p key={row.key}>Waits for · {row.condition} · Wait {row.waitID}</p>)}
        <div className="tree-wait-actions"><button onClick={()=>setReference(undefined)}>Back to waiting job</button><TargetLink tenant={tenant} target={selectedReference} returnTo={dagPath(tenant,entry,byID.get(entry)?.job_type)} /></div>
      </div>}
        {!selectedReference && <div>
        <p className="tree-detail-status" role="status">{jobStatus(detail).label || 'Unknown status'}</p>
        {cancellationState?.busy && <p role="status">Checking cancellation status…</p>}
        {cancellationState?.unavailable && <p role="status">Status unavailable</p>}
        {cancellationState?.notice && <p role="status">{cancellationState.notice}</p>}
        {cancellationState?.error && <p role="alert">{cancellationState?.error}</p>}
        {(!isFinishedJob(detail) && !cancellation.eligible(detail) || cancellationState?.unavailable) && <Button disabled={cancellationState?.busy} onClick={() => void cancellation.refresh(detail)}>Refresh status</Button>}

        </div>}
        <WaitPane key={selected} controller={waits} targets={targets} drawn={drawnKeys} selectTarget={selectTarget} showTarget={showTarget} returnTo={dagPath(tenant,entry,byID.get(entry)?.job_type)} />
        {!selectedReference && <>
        <dl>{Object.entries({Tenant:detail.tenant_id,Job:detail.job_id,Title:detail.title || 'Unavailable',Prompt:detail.prompt || 'Unavailable',Cell:detail.cell || 'Unknown',Type:detail.job_type,Status:jobStatus(detail).label,Completion:detail.completion_status || 'Unknown',Submitted:detail.created_at || 'Unavailable','Logical termination':detail.archived_at || 'Unavailable','Runtime parent':detail.parent_job_id || 'No recorded parent','Recipe attribution':detail.recipe_parent_job_id || 'None recorded',Coverage:branchText(detail.job_id)}).map(([key,value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
        {detail.job_type !== 'recipe' && <p>Generic job summary; no recipe story available.</p>}
        {state?.branches.get(detail.job_id)?.state === 'failed' && <button disabled={state.loading || controller?.viewCapped} onClick={() => void controller?.run(detail.job_id)}>Retry child page</button>}
        {(controller?.viewCapped || ['limited','malformed'].includes(state?.branches.get(detail.job_id)?.state || '')) && <p><Link className="compact-touch-link" to={dagPath(tenant, detail.job_id, detail.job_type)}>Open branch as entry</Link></p>}
        </>}
      </aside>}
    </div>}
    {cancellation.target && <JobActionDialog projectId={cancellation.target.tenant_id} jobIds={[cancellation.target.job_id]} action="cancel"
      scopeText="Cancel this job? Child jobs already submitted will not be cancelled. Running work may take time to stop, and actions already performed will not be undone."
      disabled={!cancellation.eligible(cancellation.overlay(cancellation.target)) && !cancellation.state(cancellation.target.job_id)?.busy}
      onBusyChange={cancellation.busy} onSettled={cancellation.settled} onClose={cancellation.close} onComplete={() => {}}
      focusFallback={() => detailHeading.current?.focus({preventScroll:true})} />}
  </section>;
}
