import { jobPath } from '../components/JobWaitLinks';
import { JobActionIcons } from '../components/JobActionIcons';
import RestartJobDialog from '../components/RestartJobDialog';
import { Button, Modal, Tooltip } from 'antd';
import { AimOutlined, ArrowLeftOutlined, CommentOutlined, InfoCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import JobActionDialog from '../components/JobActionDialog';
import { canArchiveImmediately, isFinishedJob } from '../utils/jobStatus';
import { useJobCancellation } from './useJobCancellation';
import { useEffect, useLayoutEffect, useReducer, useRef, useState, type ReactNode } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import type { TreeNode } from '@colony2/shared';
import { dagPath, dagReturnPath } from './navigation';
import { Traversal, emptyWait } from './traversal';
import { headline, jobStatus, statusPalette } from './presentation';
import { timing } from './timing';
import { waitTargets, type WaitTarget } from './waits';
import { TargetLink, WaitPane } from './WaitPane';
import { GEOMETRY, layoutGraph, revealPaths } from './layout';
import { useOptionalJobInbox } from '../hooks/useJobInbox';
export { GEOMETRY } from './layout';
export default function TreePage(){
  const {projectId='',jobId=''}=useParams(),location=useLocation();
  const returnTo=dagReturnPath(location.state?.treeWaitFrom,projectId);
  return <main>{returnTo&&<nav><Link className="compact-touch-link" to={returnTo}>Back to waiting job graph</Link></nav>}<TreeWorkspace key={JSON.stringify([projectId,jobId])} tenant={projectId} entry={jobId}/></main>;
}
function TreePrompt({ prompt }: { prompt: string }) {
  const [expanded, setExpanded] = useState(false);
  const excerpt = prompt.match(/^\s*\S+(?:\s+\S+){74}(?=\s+\S)/)?.[0];
  return <div className="tree-prompt">
    <strong>Prompt</strong>
    <p>{excerpt && !expanded ? `${excerpt.trim()}…` : prompt}</p>
    {excerpt && <Button size="small" type="text" aria-expanded={expanded} onClick={() => setExpanded(value => !value)}>{expanded ? 'Show less' : 'Show more'}</Button>}
  </div>;
}
export function TreeWorkspace({tenant,entry,active=true,embedded=false,onJobChanged,relationships}:{tenant:string;entry:string;active?:boolean;embedded?:boolean;onJobChanged?:(node:TreeNode)=>void;relationships?:ReactNode}){
  const inbox=useOptionalJobInbox();
  const [action, setAction] = useState<{job:TreeNode; type:"restart" | "archive"}>();
  const pending=new Set(inbox?.tenant===tenant?inbox.pending.map(input=>input.id):[]);
  const [,update]=useReducer(n=>n+1,0),traversal=useRef<Traversal>();
  const [selected,select]=useState(entry),[collapsed,collapse]=useState(new Set<string>()),[search,setSearch]=useState(''),[zoom,setZoom]=useState(1),[preferredTarget,setPreferredTarget]=useState<string>();
  const cancellation=useJobCancellation(tenant,entry,onJobChanged),detailHeading=useRef<HTMLHeadingElement>(null),viewport=useRef<HTMLDivElement>(null),selectedCard=useRef<HTMLButtonElement>(null),initiallyRevealed=useRef(false);
  const workspace = useRef<HTMLDivElement>(null);
  const detailPane = useRef<HTMLElement>(null);
  const pan = useRef<{ pointer: number; x: number; y: number; left: number; top: number }>();
  const [showRelationships, setShowRelationships] = useState(false);
  useEffect(() => { if (!active) setShowRelationships(false); }, [active]);
  useLayoutEffect(() => { if (detailPane.current) detailPane.current.scrollTop = 0; }, [selected]);
  useLayoutEffect(() => {
    const element = workspace.current;
    if (!element || !active) return;
    const measure = () => {
      const top = element.getBoundingClientRect().top + window.scrollY;
      const available = window.innerHeight - top - (embedded || window.innerWidth >= 768 ? 24 : 12);
      element.style.setProperty('--tree-workspace-height', `${Math.max(280, available)}px`);
    };
    measure();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(measure);
    for (const header of document.querySelectorAll('.app-header, .job-story-header, .job-detail-tabs')) observer?.observe(header);
    window.addEventListener('resize', measure);
    return () => { observer?.disconnect(); window.removeEventListener('resize', measure); };
  }, [active, embedded]);
  const [now,setNow]=useState(Date.now),[documentVisible,setDocumentVisible]=useState(document.visibilityState!=='hidden');
  const visible=active&&documentVisible,visibleRef=useRef(visible);visibleRef.current=visible;
  const previousConsumer=useRef(entry),focusedCard=useRef<HTMLElement>();
  useEffect(()=>{const changed=()=>setDocumentVisible(document.visibilityState!=='hidden');document.addEventListener('visibilitychange',changed);return()=>document.removeEventListener('visibilitychange',changed);},[]);
  useEffect(()=>{if(!visible)return;setNow(Date.now());const timer=window.setInterval(()=>setNow(Date.now()),1000);return()=>window.clearInterval(timer);},[visible]);
  useEffect(()=>{
    const controller=new Traversal(tenant,entry,update);traversal.current=controller;select(entry);collapse(new Set());initiallyRevealed.current=false;controller.interrupted=true;
    let current=true;queueMicrotask(()=>{if(current&&visibleRef.current&&controller.interrupted)void controller.run();});
    return()=>{current=false;controller.cancel();};
  },[tenant,entry]);
  const wasActive = useRef(active);
  useLayoutEffect(() => {
    const returning = active && !wasActive.current;
    wasActive.current = active;
    const controller = traversal.current;
    if (returning && visible && controller?.state.observed && !controller.interrupted) controller.refresh();
  }, [active, visible]);
  useEffect(()=>{const controller=traversal.current;if(!visible)controller?.pause();else queueMicrotask(()=>{if(traversal.current===controller&&visibleRef.current&&controller?.interrupted)void controller.run();});},[visible]);
  useEffect(()=>{const s=traversal.current?.state;if(!visible||!s||s.loading||!s.nodes.has(entry)||initiallyRevealed.current)return;initiallyRevealed.current=true;if(selected===entry)selectedCard.current?.scrollIntoView?.({block:'center',inline:'center'});});
  const traversalForRoute = traversal.current;
  const controller = traversalForRoute?.tenant === tenant && traversalForRoute?.entry === entry ? traversalForRoute : undefined;
  // The traversal retains the previous observation while verifying it. Publish
  // the replacement diagram and its selected details in the same render.
  const readingGraph = controller?.interrupted || (controller?.state.loading && controller.state.previous);
  const state = readingGraph ? undefined : controller?.state;
  const graph = state?.graph;
  const nodes=state?[...state.nodes.values()].map(cancellation.overlay):[],byID=new Map(nodes.map(n=>[n.job_id,n]));
  const matches=new Set(nodes.filter(n=>search&&[n.job_id,n.cell,n.job_type,n.goal_id,n.status,n.completion_status,n.title,n.prompt].some(v=>v?.toLowerCase().includes(search.toLowerCase()))).map(n=>n.job_id));
  const targetRows=graph?[...graph.edges.values()].filter(e=>e.kind==='wait').flatMap(e=>e.rows):[];
  const allTargets=waitTargets(targetRows),waits=controller?.waitView(selected)||{tenant,state:emptyWait(),rows:[],canLoad:false,refresh:()=>{},load:async()=>{}};
  const targets=waitTargets(waits.rows);
  const reveal=graph?revealPaths(graph,[...matches].map(id=>graph.key(id)).concat(preferredTarget?[preferredTarget]:[])):new Set<string>();
  const layout=graph?layoutGraph(graph,collapsed,reveal):{positions:new Map(),routes:new Map(),width:320,height:280,parents:new Map()};
  const {positions,routes,width:canvasWidth,height:canvasHeight}=layout,{width,height}=GEOMETRY;
  const drawnKeys=new Set(allTargets.filter(t=>positions.has(t.key)).map(t=>t.key));
  const detail=byID.get(selected),cancellationState=detail&&cancellation.state(detail.job_id);
  const detailStatus=detail&&jobStatus(detail,pending.has(detail.job_id)).label;
  useEffect(()=>{if(visible&&detail)controller?.inspect(detail.job_id);},[selected,detail?.job_id,visible,controller]);
  const selectedKey=graph?.key(selected),incoming=graph?[...graph.edges.values()].filter(e=>e.kind==='wait'&&e.target===selectedKey):[];
  const selectRuntime=(id:string)=>{const consumer=graph&&[...graph.edges.values()].find(e=>e.kind==='wait'&&e.target===graph.key(id));if(consumer)previousConsumer.current=graph!.nodes.get(consumer.source)!.id;select(id);setPreferredTarget(undefined);if(visible)controller?.inspect(id);};
  const selectTarget=(target:WaitTarget)=>{previousConsumer.current=selected;selectRuntime(target.id);setPreferredTarget(target.key);};
  const showTarget=(target:WaitTarget)=>{setPreferredTarget(target.key);requestAnimationFrame(()=>{const el=[...(viewport.current?.querySelectorAll<HTMLElement>('[data-wait-ref],[data-job-id]')||[])].find(e=>e.dataset.waitRef===target.key||e.dataset.jobId===target.id);el?.scrollIntoView?.({block:'center',inline:'center',behavior:'smooth'});});};
  useEffect(()=>{
    if(!graph||state?.loading)return;
    if(selected!==entry&&!graph.nodes.has(graph.key(selected))){select(graph.nodes.has(graph.key(previousConsumer.current))?previousConsumer.current:entry);requestAnimationFrame(()=>detailHeading.current?.focus({preventScroll:true}));}
    const old=focusedCard.current;if(old&&!old.isConnected&&(!document.activeElement||document.activeElement===document.body)){focusedCard.current=undefined;requestAnimationFrame(()=>{const key=old.dataset.waitRef;const item=key&&graph.nodes.get(key);const card=item&&[...(viewport.current?.querySelectorAll<HTMLElement>('[data-job-id] .tree-card-select')||[])].find(el=>el.closest<HTMLElement>('[data-job-id]')?.dataset.jobId===item.id);if(card)card.focus({preventScroll:true});else detailHeading.current?.focus({preventScroll:true});});}
  });
  const toggle=(key:string)=>collapse(old=>{const next=new Set(old);next.has(key)?next.delete(key):next.add(key);return next;});
  const findEntry=()=>{selectRuntime(entry);collapse(new Set());requestAnimationFrame(()=>{selectedCard.current?.focus({preventScroll:true});selectedCard.current?.scrollIntoView?.({block:'center',inline:'center',behavior:'smooth'});});};
  const root=state?.confirmedRoot?byID.get(state.confirmedRoot):undefined;
  const branchText=(id:string)=>{const b=state?.branches.get(id);const kids=graph&&[...graph.edges.values()].some(e=>e.kind==='child'&&e.source===graph.key(id));return b?.reason||(b?.state==='exhausted'&&!kids?'No children found in this traversal':b?.state||'pending');};
  const incomplete=!!controller?.canResume||!!state?.boundary||!!state?.stop||!!state?.warnings.length||[...(state?.branches.values()||[])].some(b=>b.state!=='exhausted')||[...(state?.waits.entries()||[])].some(([id,b])=>controller?.distances().has(graph!.key(id))&&(b.state!=='exhausted'||b.observation.snapshot?.state!=='available'||b.observation.snapshot.status==='PENDING_JOBS'&&!b.observation.rows.length&&!b.observation.hasGoalWait))||!!state?.summaries.size;
  const cycles=graph?.components().filter(c=>c.length>1||[...graph.edges.values()].some(e=>e.source===c[0]&&e.target===c[0]))||[];
  const returnTo=dagPath(tenant,entry,byID.get(entry)?.job_type);
  return <section className={embedded?'execution-tree':'app-page execution-tree'} onFocusCapture={e=>{const el=(e.target as HTMLElement).closest<HTMLElement>('[data-job-id],[data-wait-ref]');if(el)focusedCard.current=el;}}>
    <div className="tree-workspace" ref={workspace}>
      <div className="tree-diagram-column">
        <div className="tree-toolbar" role="group" aria-label="Graph zoom">
          <button aria-label="Zoom out" onClick={()=>setZoom(z=>Math.max(.05,z-.2))}>−</button>
          <output aria-label="Zoom">{Math.round(zoom*100)}%</output>
          <button aria-label="Zoom in" onClick={()=>setZoom(z=>Math.min(2,z+.2))}>+</button>
          <button onClick={()=>setZoom(Math.max(.05,Math.min(1,(viewport.current?.clientWidth||800)/canvasWidth,(viewport.current?.clientHeight||500)/canvasHeight)))}>Fit</button>
          {relationships && <Tooltip title="Related jobs"><Button aria-label="Related jobs" icon={<InfoCircleOutlined aria-hidden />} onClick={() => setShowRelationships(true)} /></Tooltip>}
        </div>
        {state?.error?<div role="alert"><p>{state.error}</p><button onClick={()=>void controller?.run()}>Retry entry</button></div>:<div ref={viewport} className="tree-viewport" aria-label="Execution diagram" onPointerDown={event => {
          if (event.button !== 0 || (event.target as Element).closest('button, a')) return;
          const element = event.currentTarget;
          pan.current = { pointer: event.pointerId, x: event.clientX, y: event.clientY, left: element.scrollLeft, top: element.scrollTop };
          element.setPointerCapture(event.pointerId);
        }} onPointerMove={event => {
          const start = pan.current;
          if (start?.pointer !== event.pointerId) return;
          event.currentTarget.scrollLeft = start.left + start.x - event.clientX;
          event.currentTarget.scrollTop = start.top + start.y - event.clientY;
        }} onLostPointerCapture={() => { pan.current = undefined; }}><div className="tree-zoom-frame" style={{width:canvasWidth*zoom,height:canvasHeight*zoom}}><div className="tree-canvas" style={{width:canvasWidth,height:canvasHeight,transform:`scale(${zoom})`}}>
          <svg aria-label="Runtime parent connections" width="100%" height="100%">{graph&&[...graph.edges.values()].filter(e=>e.kind==='child'&&routes.has(e.key)).map(e=><path key={e.key} data-parent={graph.nodes.get(e.source)?.id} data-child={graph.nodes.get(e.target)?.id} d={routes.get(e.key)!.path} className={e.previous?'tree-waits-previous':''}/>)}</svg>
          <svg className="tree-wait-connections" aria-label="Waits-for connections" width="100%" height="100%">
            <defs><marker id={`wait-arrow-${embedded?'embedded':'standalone'}`} markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><polygon points="0,0 8,4 0,8" fill="#6941a5"/></marker></defs>
            {graph&&[...graph.edges.values()].filter(e=>e.kind==='wait'&&routes.has(e.key)).map(e=>{const route=routes.get(e.key)!,conditions=[...new Set(e.rows.map(r=>r.legacy?'legacy':r.condition))].join(', ');return <g key={e.key} className={e.previous?'tree-waits-previous':''}><path data-wait-source={graph.nodes.get(e.source)?.id} data-wait-target={e.target} markerEnd={`url(#wait-arrow-${embedded?'embedded':'standalone'})`} d={route.path}><title>Waits for · {conditions} · {graph.nodes.get(e.target)?.id}</title></path><text x={route.label.x} y={route.label.y+12}>{conditions.length>18?'Wait conditions':conditions}</text></g>;})}
          </svg>
          {graph&&[...positions].map(([key,p])=>{const item=graph.nodes.get(key)!;const n=byID.get(item.id),target=allTargets.find(t=>t.key===key);if(!n)return <button key={key} data-wait-ref={key} className="tree-wait-reference" style={{left:p.x,top:p.y,width,height}} aria-label={`Job wait target · ${item.id}`} aria-pressed={selected===item.id} onClick={()=>target&&selectTarget(target)}><span>Job summary unavailable</span><strong>{item.id}</strong><span>{state?.summaries.get(item.id)?.state==='loading'?'Loading summary…':'Select to inspect'}</span></button>;
            const status=jobStatus(n,pending.has(n.job_id));
            const [fill,border,text]=statusPalette[status.color]||statusPalette.default,time=timing(n,state?.family.has(n.job_id)?root:undefined,now,!state?.family.has(n.job_id));
            const hasKids=[...graph.edges.values()].some(e=>e.source===key&&e.target!==key);
            return <div key={key} data-job-id={item.id} className={`tree-card ${target?'tree-job-wait-target':''} ${item.id===entry?'tree-entry':''} ${matches.has(item.id)?'tree-match':''}`} style={{left:p.x,top:p.y,width,height,background:fill,borderColor:border,color:text}}><button className="tree-card-select" ref={item.id===entry?selectedCard:undefined} aria-label={`${headline(n)} · ${n.cell||'Unknown cell'}${item.id===entry?' · Opened job':''}`} aria-pressed={selected===item.id} onClick={()=>selectRuntime(item.id)}><span className="tree-cell">{n.cell||'Unknown cell'}</span><strong title={n.title||n.prompt}>{headline(n)}</strong><span className="tree-status">{status.label||'Unknown status'}</span><span>{time.submission}</span><span>{time.span}</span>{item.id===entry&&<span className="tree-opened">Opened job</span>}{target&&<span className="tree-wait-badge">Wait target</span>}</button>{hasKids&&<button className="tree-branch" aria-label={`Toggle branch ${headline(n)}`} aria-expanded={!collapsed.has(key)||reveal.has(key)} onClick={()=>toggle(key)}>{collapsed.has(key)?'Expand':'Collapse'}</button>}</div>;
          })}
        </div></div></div>}
      </div>
      <aside ref={detailPane} aria-label="Selected job details" className="tree-details" tabIndex={0}>
        {state&&!state.error&&<div className="tree-details-header"><h2 ref={detailHeading} tabIndex={-1}>Selected job</h2>
          {detail&&<div className="tree-detail-actions" role="group" aria-label="Selected job actions">
            <JobActionIcons graph={dagPath(tenant, detail.job_id, detail.job_type)} onRefresh={() => controller?.refresh()}
              onRestart={() => setAction({job:detail, type:'restart'})} onArchive={() => setAction({job:detail, type:'archive'})}
              onCancel={cancellation.eligible(detail) ? () => cancellation.open(detail) : undefined} />
            <Tooltip title="Open thread" trigger={['hover','focus']}><Link className="tree-detail-action" aria-label="Open thread" to={jobPath(tenant,detail.job_id)} state={{treeReturnTo:returnTo}}><CommentOutlined/></Link></Tooltip>
          </div>}
        </div>}
        {detail?.prompt && <TreePrompt key={detail.job_id} prompt={detail.prompt} />}
        <div className="tree-tools" role="group" aria-label="Graph tools">
          {!embedded&&<Tooltip title="Back to jobs"><Link className="tree-detail-action" aria-label="Back to jobs" to={`/project/${encodeURIComponent(tenant)}/jobs`}><ArrowLeftOutlined/></Link></Tooltip>}
          <Tooltip title="Refresh tree"><Button aria-label="Refresh tree" icon={<ReloadOutlined/>} onClick={()=>controller?.refresh()}/></Tooltip>
          <Tooltip title="Find opened job"><Button aria-label="Find opened job" icon={<AimOutlined/>} disabled={!byID.has(entry)} onClick={findEntry}/></Tooltip>
          {state && controller?.canResume&&<Button disabled={state.loading} onClick={()=>void controller.run()}>Resume discovery</Button>}
          <input aria-label="Search loaded jobs" placeholder="Find a job" type="search" value={search} onChange={event=>setSearch(event.target.value)}/>
          {search&&<output>{matches.size} loaded matches</output>}
        </div>
        {!!allTargets.length&&<div className="tree-wait-actions"><button onClick={()=>showTarget(allTargets[0])}>Reveal wait targets</button><span>{drawnKeys.size} of {allTargets.length} loaded targets drawn</span></div>}
        {state&&!state.error&&<>
        {incoming.length>0&&<section aria-label="Waiting jobs"><h3>Waiting jobs</h3>{incoming.map(edge=><div key={edge.key}><p>Waiting job: {graph?.nodes.get(edge.source)?.id}{edge.previous?' · Previous observation':''}</p>{edge.rows.map(row=><p key={row.key}>Waits for · {row.condition} · Wait {row.waitID}{row.deadline?` · Deadline ${row.deadline}`:''}{row.blockedReason?` · ${row.blockedReason}`:''}</p>)}<button onClick={()=>selectRuntime(graph!.nodes.get(edge.source)!.id)}>Back to waiting job</button></div>)}</section>}
        {detail&&<>
          {state.summaries.has(selected)&&<div role="status"><p>Previous job summary · {state.summaries.get(selected)?.reason||'awaiting verification'}</p><button disabled={state.loading} onClick={()=>void controller?.retrySummary(selected)}>Retry summary</button></div>}
          <p className="tree-detail-status" role="status">{detailStatus||'Unknown status'}</p>
          {cancellationState?.busy&&<p role="status">Checking cancellation status…</p>}{cancellationState?.unavailable&&<p role="status">Status unavailable</p>}{cancellationState?.notice&&<p role="status">{cancellationState.notice}</p>}{cancellationState?.error&&<p role="alert">{cancellationState.error}</p>}
          {(!isFinishedJob(detail)&&!cancellation.eligible(detail)||cancellationState?.unavailable)&&<Button disabled={cancellationState?.busy} onClick={()=>void cancellation.refresh(detail)}>Refresh status</Button>}
          <TargetLink tenant={tenant} id={selected} returnTo={returnTo}/>
          <dl>{Object.entries({Tenant:detail.tenant_id,Job:detail.job_id,Title:detail.title||'Unavailable',Cell:detail.cell||'Unknown',Type:detail.job_type,Goal:detail.goal_id||'None recorded',Store:detail.store||'Unavailable','Cancellation requested':detail.cancel_requested?'Yes':'No',Completion:detail.completion_status||'Unknown',Submitted:detail.created_at||'Unavailable',Updated:detail.updated_at||'Unavailable','Logical termination':detail.archived_at||'Unavailable','Runtime parent':detail.parent_job_id||'No recorded parent','Recipe attribution':detail.recipe_parent_job_id||'None recorded','Child coverage':branchText(detail.job_id),'Wait coverage':waits.state.stopped||waits.state.error||state.waits.get(detail.job_id)?.state||'pending'}).map(([key,value])=><div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
          {detail.job_type!=='recipe'&&<p>Generic job summary; no recipe story available.</p>}
          {state.branches.get(detail.job_id)?.state==='failed'&&<button disabled={state.loading} onClick={()=>void controller?.run(detail.job_id)}>Retry child page</button>}
          {(controller?.viewCapped||['limited','malformed'].includes(state.branches.get(detail.job_id)?.state||''))&&<p><Link className="compact-touch-link" to={dagPath(tenant,detail.job_id,detail.job_type)}>Open branch as entry</Link></p>}
          <WaitPane key={selected} controller={waits} targets={targets} drawn={drawnKeys} selectTarget={selectTarget} showTarget={showTarget} returnTo={returnTo}/>
        </>}
        {!detail&&<><code className="tree-wait-identity">{selected}</code><p>{state.summaries.get(selected)?.state==='loading'?'Loading summary…':'Job summary unavailable. Successful incoming wait details are retained.'}</p><button disabled={state.loading} onClick={()=>void controller?.retrySummary(selected)}>Retry summary</button><TargetLink tenant={tenant} id={selected} returnTo={returnTo}/></>}
        </>}
        <section className="tree-coverage" aria-label="Discovery coverage">
          {!state||state.loading ? <p role="status">Discovering…</p> : <details>
            <summary>{state.error?'Entry unavailable':incomplete?'Partial discovery':'Discovery finished'}</summary>
            <p>{nodes.length} loaded jobs · Started {state.started} · Latest observation {state.observed||'pending'}</p>
            {state.previous&&<p>Previous observation · awaiting verification</p>}{state.boundary&&<p>{state.boundary}</p>}{state.stop&&<p>{state.stop}</p>}{state.warnings.map(w=><p key={w}>{w}</p>)}
            {!!cycles.length&&<p>{cycles.length} cycle boundaries</p>}
          </details>}
        </section>
      </aside>
    </div>
    {relationships && <Modal title="Job relationships" open={active && showRelationships} footer={null} onCancel={() => setShowRelationships(false)} width="min(760px, 94vw)"
      styles={{ body: { maxHeight: '72dvh', overflow: 'auto' } }}>{relationships}</Modal>}
    {action?.type === 'restart' && <RestartJobDialog key={action.job.job_id} projectId={tenant} jobId={action.job.job_id} job={action.job} onClose={() => setAction(undefined)} />}
    {action?.type === 'archive' && <JobActionDialog projectId={tenant} jobIds={[action.job.job_id]} action="archive" job={action.job} immediate={canArchiveImmediately([action.job.job_id], [action.job])}
      onClose={() => setAction(undefined)} onComplete={() => { setAction(undefined); inbox?.refresh(); controller?.refresh(); }} />}
    {cancellation.target&&<JobActionDialog projectId={cancellation.target.tenant_id} jobIds={[cancellation.target.job_id]} action="cancel" scopeText="Cancel this job? Child jobs already submitted will not be cancelled. Running work may take time to stop, and actions already performed will not be undone." disabled={!cancellation.eligible(cancellation.overlay(cancellation.target))&&!cancellation.state(cancellation.target.job_id)?.busy} onBusyChange={cancellation.busy} onSettled={cancellation.settled} onClose={cancellation.close} onComplete={()=>{}} focusFallback={()=>detailHeading.current?.focus({preventScroll:true})}/>}
  </section>;
}
