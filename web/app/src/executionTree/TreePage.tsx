import { Button, Tooltip } from 'antd';
import { AimOutlined, ArrowLeftOutlined, ExportOutlined, FlagOutlined, ReloadOutlined, StopOutlined } from '@ant-design/icons';
import JobActionDialog from '../components/JobActionDialog';
import { isFinishedJob } from '../utils/jobStatus';
import { useJobCancellation } from './useJobCancellation';
import { useEffect, useReducer, useRef, useState } from 'react';
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
const jobPath = (tenant:string,job:string) => `/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}`;
export default function TreePage(){
  const {projectId='',jobId=''}=useParams(),location=useLocation();
  const returnTo=dagReturnPath(location.state?.treeWaitFrom,projectId);
  return <main>{returnTo&&<nav><Link className="compact-touch-link" to={returnTo}>Back to waiting job graph</Link></nav>}<TreeWorkspace key={JSON.stringify([projectId,jobId])} tenant={projectId} entry={jobId}/></main>;
}
export function TreeWorkspace({tenant,entry,active=true,embedded=false,onJobChanged}:{tenant:string;entry:string;active?:boolean;embedded?:boolean;onJobChanged?:(node:TreeNode)=>void}){
  const inbox=useOptionalJobInbox();
  const pending=new Set(inbox?.tenant===tenant?inbox.pending.map(input=>input.id):[]);
  const [,update]=useReducer(n=>n+1,0),traversal=useRef<Traversal>();
  const [selected,select]=useState(entry),[reference,setReference]=useState<string>(),[collapsed,collapse]=useState(new Set<string>()),[search,setSearch]=useState(''),[zoom,setZoom]=useState(1),[preferredTarget,setPreferredTarget]=useState<string>();
  const cancellation=useJobCancellation(tenant,entry,onJobChanged),detailHeading=useRef<HTMLHeadingElement>(null),viewport=useRef<HTMLDivElement>(null),selectedCard=useRef<HTMLButtonElement>(null),initiallyRevealed=useRef(false);
  const [now,setNow]=useState(Date.now),[documentVisible,setDocumentVisible]=useState(document.visibilityState!=='hidden');
  const visible=active&&documentVisible,visibleRef=useRef(visible);visibleRef.current=visible;
  const previousConsumer=useRef(entry),focusedCard=useRef<HTMLElement>();
  useEffect(()=>{const changed=()=>setDocumentVisible(document.visibilityState!=='hidden');document.addEventListener('visibilitychange',changed);return()=>document.removeEventListener('visibilitychange',changed);},[]);
  useEffect(()=>{if(!visible)return;setNow(Date.now());const timer=window.setInterval(()=>setNow(Date.now()),1000);return()=>window.clearInterval(timer);},[visible]);
  useEffect(()=>{
    const controller=new Traversal(tenant,entry,update);traversal.current=controller;select(entry);setReference(undefined);collapse(new Set());initiallyRevealed.current=false;controller.interrupted=true;
    let current=true;queueMicrotask(()=>{if(current&&visibleRef.current&&controller.interrupted)void controller.run();});
    return()=>{current=false;controller.cancel();};
  },[tenant,entry]);
  useEffect(()=>{const controller=traversal.current;if(!visible)controller?.pause();else queueMicrotask(()=>{if(traversal.current===controller&&visibleRef.current&&controller?.interrupted)void controller.run();});},[visible]);
  useEffect(()=>{const s=traversal.current?.state;if(!visible||!s||s.loading||!s.nodes.has(entry)||initiallyRevealed.current)return;initiallyRevealed.current=true;if(selected===entry)selectedCard.current?.scrollIntoView?.({block:'center',inline:'center'});});
  const controller=traversal.current,state=controller?.state,graph=state?.graph;
  const nodes=state?[...state.nodes.values()].map(cancellation.overlay):[],byID=new Map(nodes.map(n=>[n.job_id,n]));
  const matches=new Set(nodes.filter(n=>search&&[n.job_id,n.cell,n.job_type,n.goal_id,n.status,n.completion_status,n.title,n.prompt].some(v=>v?.toLowerCase().includes(search.toLowerCase()))).map(n=>n.job_id));
  const targetRows=graph?[...graph.edges.values()].filter(e=>e.kind==='wait').flatMap(e=>e.rows):[];
  const allTargets=waitTargets(targetRows),waits=controller?.waitView(selected)||{tenant,state:emptyWait(),rows:[],canLoad:false,refresh:()=>{},load:async()=>{}};
  const targets=waitTargets(waits.rows),selectedReference=reference?allTargets.find(t=>t.key===reference):undefined;
  const reveal=graph?revealPaths(graph,[...matches].map(id=>graph.key(id)).concat(preferredTarget?[preferredTarget]:[])):new Set<string>();
  const layout=graph?layoutGraph(graph,collapsed,reveal):{positions:new Map(),routes:new Map(),width:320,height:280,parents:new Map()};
  const {positions,routes,width:canvasWidth,height:canvasHeight}=layout,{width,height}=GEOMETRY;
  const drawnKeys=new Set(allTargets.filter(t=>positions.has(t.key)).map(t=>t.key));
  const detail=byID.get(selected),cancellationState=detail&&cancellation.state(detail.job_id);
  const detailStatus=detail&&jobStatus(detail,pending.has(detail.job_id)).label;
  useEffect(()=>{if(visible&&detail)controller?.inspect(detail.job_id);},[selected,detail?.job_id,visible,controller]);
  const selectedKey=graph?.key(selected),incoming=graph?[...graph.edges.values()].filter(e=>e.kind==='wait'&&e.target===(reference||selectedKey)):[];
  const selectRuntime=(id:string)=>{const consumer=graph&&[...graph.edges.values()].find(e=>e.kind==='wait'&&e.target===graph.key(id));if(consumer)previousConsumer.current=graph!.nodes.get(consumer.source)!.id;select(id);setReference(undefined);setPreferredTarget(undefined);if(visible)controller?.inspect(id);};
  const selectTarget=(target:WaitTarget)=>{previousConsumer.current=selected;if(target.kind==='job')selectRuntime(target.id);else setReference(target.key);setPreferredTarget(target.key);};
  const showTarget=(target:WaitTarget)=>{setPreferredTarget(target.key);requestAnimationFrame(()=>{const el=[...(viewport.current?.querySelectorAll<HTMLElement>('[data-wait-ref],[data-job-id]')||[])].find(e=>e.dataset.waitRef===target.key||target.kind==='job'&&e.dataset.jobId===target.id);el?.scrollIntoView?.({block:'center',inline:'center',behavior:'smooth'});});};
  useEffect(()=>{
    if(!graph||state?.loading)return;
    if(reference&&!graph.nodes.has(reference)||!reference&&selected!==entry&&!graph.nodes.has(graph.key(selected))){select(graph.nodes.has(graph.key(previousConsumer.current))?previousConsumer.current:entry);setReference(undefined);requestAnimationFrame(()=>detailHeading.current?.focus({preventScroll:true}));}
    const old=focusedCard.current;if(old&&!old.isConnected&&(!document.activeElement||document.activeElement===document.body)){focusedCard.current=undefined;requestAnimationFrame(()=>{const key=old.dataset.waitRef;const item=key&&graph.nodes.get(key);const card=item&&[...(viewport.current?.querySelectorAll<HTMLElement>('[data-job-id] .tree-card-select')||[])].find(el=>el.closest<HTMLElement>('[data-job-id]')?.dataset.jobId===item.id);if(card)card.focus({preventScroll:true});else detailHeading.current?.focus({preventScroll:true});});}
  });
  const toggle=(key:string)=>collapse(old=>{const next=new Set(old);next.has(key)?next.delete(key):next.add(key);return next;});
  const findEntry=()=>{selectRuntime(entry);collapse(new Set());requestAnimationFrame(()=>{selectedCard.current?.focus({preventScroll:true});selectedCard.current?.scrollIntoView?.({block:'center',inline:'center',behavior:'smooth'});});};
  const root=state?.confirmedRoot?byID.get(state.confirmedRoot):undefined;
  const branchText=(id:string)=>{const b=state?.branches.get(id);const kids=graph&&[...graph.edges.values()].some(e=>e.kind==='child'&&e.source===graph.key(id));return b?.reason||(b?.state==='exhausted'&&!kids?'No children found in this traversal':b?.state||'pending');};
  const incomplete=!!controller?.canResume||!!state?.boundary||!!state?.stop||!!state?.warnings.length||[...(state?.branches.values()||[])].some(b=>b.state!=='exhausted')||[...(state?.waits.entries()||[])].some(([id,b])=>controller?.distances().has(graph!.key(id))&&(b.state!=='exhausted'||b.observation.snapshot?.state!=='available'||b.observation.snapshot.status==='PENDING_JOBS'&&!b.observation.rows.length))||!!state?.summaries.size||!!allTargets.some(t=>t.kind==='goal');
  const cycles=graph?.components().filter(c=>c.length>1||[...graph.edges.values()].some(e=>e.source===c[0]&&e.target===c[0]))||[];
  const returnTo=dagPath(tenant,entry,byID.get(entry)?.job_type);
  return <section className={embedded?'execution-tree':'app-page execution-tree'} onFocusCapture={e=>{const el=(e.target as HTMLElement).closest<HTMLElement>('[data-job-id],[data-wait-ref]');if(el)focusedCard.current=el;}}>
    <div className="tree-workspace">
      <div className="tree-diagram-column">
        <div className="tree-toolbar" role="group" aria-label="Graph zoom">
          <button aria-label="Zoom out" onClick={()=>setZoom(z=>Math.max(.05,z-.2))}>−</button>
          <output aria-label="Zoom">{Math.round(zoom*100)}%</output>
          <button aria-label="Zoom in" onClick={()=>setZoom(z=>Math.min(2,z+.2))}>+</button>
          <button onClick={()=>setZoom(Math.max(.05,Math.min(1,(viewport.current?.clientWidth||800)/canvasWidth)))}>Fit</button>
        </div>
        {state?.error?<div role="alert"><p>{state.error}</p><button onClick={()=>void controller?.run()}>Retry entry</button></div>:<div ref={viewport} className="tree-viewport" aria-label="Execution diagram"><div style={{width:canvasWidth*zoom,height:canvasHeight*zoom}}><div className="tree-canvas" style={{width:canvasWidth,height:canvasHeight,transform:`scale(${zoom})`}}>
          <svg aria-label="Runtime parent connections" width="100%" height="100%">{graph&&[...graph.edges.values()].filter(e=>e.kind==='child'&&routes.has(e.key)).map(e=><path key={e.key} data-parent={graph.nodes.get(e.source)?.id} data-child={graph.nodes.get(e.target)?.id} d={routes.get(e.key)!.path} className={e.previous?'tree-waits-previous':''}/>)}</svg>
          <svg className="tree-wait-connections" aria-label="Waits-for connections" width="100%" height="100%">
            <defs><marker id={`wait-arrow-${embedded?'embedded':'standalone'}`} markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><polygon points="0,0 8,4 0,8" fill="#6941a5"/></marker></defs>
            {graph&&[...graph.edges.values()].filter(e=>e.kind==='wait'&&routes.has(e.key)).map(e=>{const route=routes.get(e.key)!,conditions=[...new Set(e.rows.map(r=>r.legacy?'legacy':r.condition))].join(', ');return <g key={e.key} className={e.previous?'tree-waits-previous':''}><path data-wait-source={graph.nodes.get(e.source)?.id} data-wait-target={e.target} markerEnd={`url(#wait-arrow-${embedded?'embedded':'standalone'})`} d={route.path}><title>Waits for · {conditions} · {graph.nodes.get(e.target)?.id}</title></path><text x={route.label.x} y={route.label.y+12}>{conditions.length>18?'Wait conditions':conditions}</text></g>;})}
          </svg>
          {graph&&[...positions].map(([key,p])=>{const item=graph.nodes.get(key)!;const n=item.kind==='job'?byID.get(item.id):undefined,target=allTargets.find(t=>t.key===key);if(!n)return <button key={key} data-wait-ref={key} className={`tree-wait-reference tree-wait-${item.kind}`} style={{left:p.x,top:p.y,width,height}} aria-label={`${item.kind==='goal'?'Goal':'Job'} wait target · ${item.id}`} aria-pressed={reference===key||!reference&&selected===item.id} onClick={()=>target&&selectTarget(target)}><span>{item.kind==='goal'?'Goal wait target':'Job summary unavailable'}</span><strong>{item.id}</strong><span>{state?.summaries.get(item.id)?.state==='loading'?'Loading summary…':'Select to inspect'}</span></button>;
            const status=jobStatus(n,pending.has(n.job_id));
            const [fill,border,text]=statusPalette[status.color]||statusPalette.default,time=timing(n,state?.family.has(n.job_id)?root:undefined,now,!state?.family.has(n.job_id));
            const hasKids=[...graph.edges.values()].some(e=>e.source===key&&e.target!==key);
            return <div key={key} data-job-id={item.id} className={`tree-card ${target?'tree-job-wait-target':''} ${item.id===entry?'tree-entry':''} ${matches.has(item.id)?'tree-match':''}`} style={{left:p.x,top:p.y,width,height,background:fill,borderColor:border,color:text}}><button className="tree-card-select" ref={item.id===entry?selectedCard:undefined} aria-label={`${headline(n)} · ${n.cell||'Unknown cell'}${item.id===entry?' · Opened job':''}`} aria-pressed={!reference&&selected===item.id} onClick={()=>selectRuntime(item.id)}><span className="tree-cell">{n.cell||'Unknown cell'}</span><strong title={n.title||n.prompt}>{headline(n)}</strong><span className="tree-status">{status.label||'Unknown status'}</span><span>{time.submission}</span><span>{time.span}</span>{item.id===entry&&<span className="tree-opened">Opened job</span>}{target&&<span className="tree-wait-badge">Wait target</span>}</button>{hasKids&&<button className="tree-branch" aria-label={`Toggle branch ${headline(n)}`} aria-expanded={!collapsed.has(key)||reveal.has(key)} onClick={()=>toggle(key)}>{collapsed.has(key)?'Expand':'Collapse'}</button>}</div>;
          })}
        </div></div></div>}
      </div>
      <aside aria-label={reference?'Selected wait target':'Selected job details'} className="tree-details">
        {state&&!state.error&&<div className="tree-details-header"><h2 ref={detailHeading} tabIndex={-1}>{reference?'Selected wait target':'Selected job'}</h2>
          {detail&&!reference&&<div className="tree-detail-actions" role="group" aria-label="Selected job actions">
            {cancellation.eligible(detail)&&<Tooltip title="Cancel job" trigger={['hover','focus']}><Button className="tree-detail-action tree-detail-cancel" danger aria-label="Cancel job" icon={<StopOutlined/>} onClick={()=>cancellation.open(detail)}/></Tooltip>}
            {detail.goal_id&&<Tooltip title="Goal overview" trigger={['hover','focus']}><Link className="tree-detail-action" aria-label="Goal overview" to={`/project/${encodeURIComponent(tenant)}/goals/${encodeURIComponent(detail.goal_id)}`}><FlagOutlined/></Link></Tooltip>}
            {detail.job_type==='recipe'&&<Tooltip title="Open execution" trigger={['hover','focus']}><Link className="tree-detail-action" aria-label="Open execution" to={`${jobPath(tenant,detail.job_id)}/story`} state={{treeReturnTo:returnTo}}><ExportOutlined/></Link></Tooltip>}
          </div>}
        </div>}
        <div className="tree-tools" role="group" aria-label="Graph tools">
          {!embedded&&<Tooltip title="Back to jobs"><Link className="tree-detail-action" aria-label="Back to jobs" to={`/project/${encodeURIComponent(tenant)}/jobs`}><ArrowLeftOutlined/></Link></Tooltip>}
          <Tooltip title="Refresh tree"><Button aria-label="Refresh tree" icon={<ReloadOutlined/>} onClick={()=>controller?.refresh()}/></Tooltip>
          <Tooltip title="Find opened job"><Button aria-label="Find opened job" icon={<AimOutlined/>} disabled={!byID.has(entry)} onClick={findEntry}/></Tooltip>
          {controller?.canResume&&<Button disabled={state?.loading} onClick={()=>void controller.run()}>Resume discovery</Button>}
          <input aria-label="Search loaded jobs" placeholder="Find a job" type="search" value={search} onChange={event=>setSearch(event.target.value)}/>
          {search&&<output>{matches.size} loaded matches</output>}
        </div>
        {!!allTargets.length&&<div className="tree-wait-actions"><button onClick={()=>showTarget(allTargets[0])}>Reveal wait targets</button><span>{drawnKeys.size} of {allTargets.length} loaded targets drawn</span></div>}
        {state&&!state.error&&<>
        {incoming.length>0&&<section aria-label="Waiting jobs"><h3>Waiting jobs</h3>{incoming.map(edge=><div key={edge.key}><p>Waiting job: {graph?.nodes.get(edge.source)?.id}{edge.previous?' · Previous observation':''}</p>{edge.rows.map(row=><p key={row.key}>Waits for · {row.condition} · Wait {row.waitID}{row.deadline?` · Deadline ${row.deadline}`:''}{row.blockedReason?` · ${row.blockedReason}`:''}</p>)}<button onClick={()=>selectRuntime(graph!.nodes.get(edge.source)!.id)}>Back to waiting job</button></div>)}</section>}
        {reference&&selectedReference&&<><p>Goal wait target</p><code className="tree-wait-identity">{selectedReference.id}</code><TargetLink tenant={tenant} target={selectedReference} returnTo={returnTo}/></>}
        {!reference&&detail&&<>
          {state.summaries.has(selected)&&<div role="status"><p>Previous job summary · {state.summaries.get(selected)?.reason||'awaiting verification'}</p><button disabled={state.loading} onClick={()=>void controller?.retrySummary(selected)}>Retry summary</button></div>}
          <p className="tree-detail-status" role="status">{detailStatus||'Unknown status'}</p>
          {cancellationState?.busy&&<p role="status">Checking cancellation status…</p>}{cancellationState?.unavailable&&<p role="status">Status unavailable</p>}{cancellationState?.notice&&<p role="status">{cancellationState.notice}</p>}{cancellationState?.error&&<p role="alert">{cancellationState.error}</p>}
          {(!isFinishedJob(detail)&&!cancellation.eligible(detail)||cancellationState?.unavailable)&&<Button disabled={cancellationState?.busy} onClick={()=>void cancellation.refresh(detail)}>Refresh status</Button>}
          <TargetLink tenant={tenant} target={{key:graph!.key(selected),id:selected,kind:'job',rows:[]}} returnTo={returnTo}/>
          <dl>{Object.entries({Tenant:detail.tenant_id,Job:detail.job_id,Title:detail.title||'Unavailable',Prompt:detail.prompt||'Unavailable',Cell:detail.cell||'Unknown',Type:detail.job_type,Goal:detail.goal_id||'None recorded',Store:detail.store||'Unavailable','Cancellation requested':detail.cancel_requested?'Yes':'No',Completion:detail.completion_status||'Unknown',Submitted:detail.created_at||'Unavailable',Updated:detail.updated_at||'Unavailable','Logical termination':detail.archived_at||'Unavailable','Runtime parent':detail.parent_job_id||'No recorded parent','Recipe attribution':detail.recipe_parent_job_id||'None recorded','Child coverage':branchText(detail.job_id),'Wait coverage':waits.state.stopped||waits.state.error||state.waits.get(detail.job_id)?.state||'pending'}).map(([key,value])=><div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl>
          {detail.job_type!=='recipe'&&<p>Generic job summary; no recipe story available.</p>}
          {state.branches.get(detail.job_id)?.state==='failed'&&<button disabled={state.loading} onClick={()=>void controller?.run(detail.job_id)}>Retry child page</button>}
          {(controller?.viewCapped||['limited','malformed'].includes(state.branches.get(detail.job_id)?.state||''))&&<p><Link className="compact-touch-link" to={dagPath(tenant,detail.job_id,detail.job_type)}>Open branch as entry</Link></p>}
          <WaitPane key={selected} controller={waits} targets={targets} drawn={drawnKeys} selectTarget={selectTarget} showTarget={showTarget} returnTo={returnTo}/>
        </>}
        {!reference&&!detail&&<><code className="tree-wait-identity">{selected}</code><p>{state.summaries.get(selected)?.state==='loading'?'Loading summary…':'Job summary unavailable. Successful incoming wait details are retained.'}</p><button disabled={state.loading} onClick={()=>void controller?.retrySummary(selected)}>Retry summary</button><TargetLink tenant={tenant} target={{key:graph!.key(selected),id:selected,kind:'job',rows:[]}} returnTo={returnTo}/></>}
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
    {cancellation.target&&<JobActionDialog projectId={cancellation.target.tenant_id} jobIds={[cancellation.target.job_id]} action="cancel" scopeText="Cancel this job? Child jobs already submitted will not be cancelled. Running work may take time to stop, and actions already performed will not be undone." disabled={!cancellation.eligible(cancellation.overlay(cancellation.target))&&!cancellation.state(cancellation.target.job_id)?.busy} onBusyChange={cancellation.busy} onSettled={cancellation.settled} onClose={cancellation.close} onComplete={()=>{}} focusFallback={()=>detailHeading.current?.focus({preventScroll:true})}/>}
  </section>;
}
