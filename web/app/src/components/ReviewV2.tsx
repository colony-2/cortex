import { useEffect, useLayoutEffect, useRef, useState, useId } from 'react';
import { Alert, Button, Input, Tooltip, Typography, type InputRef } from 'antd';
import { SettingOutlined, ArrowLeftOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams, useLocation } from 'react-router-dom';
import { changedReviewSettings, storedDocumentURL, reviewV2ContributionError, reviewV2DirectionPolicy, createSubmissionId, inputActivityService, stringifyJobJSON, validateReviewScalar, type RecipeJob, type ReviewV2Request, type ReviewV2Turn, type UserInputDetails } from '@colony2/shared';
import { reviewV2AnswerPending } from '../utils/reviewV2State';
import { reviewReturnSearch } from '../utils/reviewFilters';
import { useReviewV2 } from '../hooks/useReviewV2';
import ReviewDocuments from './ReviewDocuments';
import ReviewChanges from './ReviewChanges';
import ReviewQuestion from './ReviewQuestion';
import ReviewPaneSeparator from './ReviewPaneSeparator';
import { automaticDocuments, reviewPaneDefaults, reviewV2Layout, type ReviewSide, type ReviewWidths } from '../utils/reviewV2Layout';
import ReviewV2Settings from './ReviewV2Settings';
import './reviewV2.css';
import ReviewV2Context from './ReviewV2Context';
import { useReviewResponsePane } from '../hooks/useReviewResponsePane';
import { resolveReviewDocumentLocation, reviewDocumentSearch } from '../utils/reviewDocumentLocation';

export function ReviewV2Discussion({turns,request,attachmentURL,declarations,pending=false,id,hidden=false,layoutReady=true}: {id?:string;hidden?:boolean;layoutReady?:boolean;turns:ReviewV2Turn[];request:ReviewV2Request;declarations?:Record<string,ReviewV2Request>;pending?:boolean;attachmentURL?:(turn:number,index:number)=>string}) {
  const [visible,setVisible]=useState(20);
  const root=useRef<HTMLElement>(null);
  const readingPosition=useRef<number>();
  const expansion=useRef<{height:number;top:number}>();
  useLayoutEffect(()=>{
    if(hidden||!layoutReady||!root.current)return;
    root.current.scrollTop=readingPosition.current??root.current.scrollHeight;
  },[hidden,layoutReady]);
  useLayoutEffect(()=>{
    if(expansion.current&&root.current){
      root.current.scrollTop=expansion.current.top+root.current.scrollHeight-expansion.current.height;
      readingPosition.current=root.current.scrollTop;
      expansion.current=undefined;
    }
  },[visible]);
  return <section ref={root} id={id} hidden={hidden} onScroll={()=>{if(root.current&&!hidden)readingPosition.current=root.current.scrollTop;}} aria-label="Discussion" data-review={request.review_id} className="review-v2-discussion">
    <Typography.Title level={4}>Discussion</Typography.Title>
    {turns.length>visible&&<Button onClick={()=>{if(root.current)expansion.current={height:root.current.scrollHeight,top:root.current.scrollTop};setVisible(n=>n+20);}}>Show earlier turns ({turns.length-visible})</Button>}
    {turns.slice(-visible).map((turn,index)=>{
      const actual=turns.length>visible?turns.length-visible+index:index;
      const frozen=turn.role==='human'&&typeof turn.receipt?.request_id==='string'?declarations?.[turn.receipt.request_id]:undefined;
      return <article key={actual} className={`review-v2-turn ${turn.role}`}>
        <strong>{turn.role==='human'?'Reviewer':'Agent'}</strong>{turn.direction&&<span> · {frozen?.directions.find(d=>d.id===turn.direction)?.label??turn.direction}</span>}
        <p style={{whiteSpace:'pre-wrap'}}>{turn.text||'No written feedback'}</p>
        {Object.entries(turn.settings??{}).map(([id,value])=><p key={id}>{frozen?.settings?.find(s=>s.id===id)?.label??id}: <code>{JSON.stringify(value)}</code></p>)}
        {turn.attachments.map((ref,i)=>{const url=attachmentURL?.(actual,i);return <p key={i}>{url?<a href={url} download>Download {(ref.name||ref.stored.key.name).split('/').pop()}</a>:<span>{(ref.name||ref.stored.key.name).split('/').pop()} (retained download unavailable)</span>}</p>;})}
      </article>;
    })}
    {pending&&<p role="status" className="review-v2-pending">Response pending</p>}
    {request.question.trim()&&<ReviewQuestion question={request.question}/>}
    {!turns.length&&!request.question.trim()&&<p>No questions yet. You can ask about the material before choosing a direction.</p>}
  </section>;
}
export default function ReviewV2({tenant,job,reviewId,initial}: {tenant:string;job:string;reviewId:string;initial?:UserInputDetails}) {
  const [contextJob, setContextJob] = useState<RecipeJob>();
  const {view,error,busy,refresh,sending,accepted,failed}=useReviewV2(tenant,job,reviewId,initial,contextJob);
  const navigate=useNavigate(); const [search]=useSearchParams(); const location=useLocation();
  const reviewsPath=`/project/${encodeURIComponent(tenant)}/reviews${reviewReturnSearch(search)}`;
  const [settingsOpen,setSettingsOpen]=useState(false);
  const [drafts,setDrafts]=useState<Record<string,unknown>>({});
  const [draftNotice,setDraftNotice]=useState('');
  const [direction,setDirection]=useState<string>();
  const [feedback,setFeedback]=useState('');
  const [files,setFiles]=useState<File[]>([]);
  const [composerOpen,setComposerOpen]=useState(false);
  const pane=useRef<HTMLDivElement>(null);
  const workspace=useRef<HTMLDivElement>(null);
  const [workspaceSize,setWorkspaceSize]=useState({width:0,narrow:false});
  const [documentsPreference,setDocumentsPreference]=useState<boolean>();
  const [discussionVisible,setDiscussionVisible]=useState(true);
  const [preferredWidths,setPreferredWidths]=useState(reviewPaneDefaults);
  const visibilityPreferences=useRef({documents:documentsPreference,discussion:discussionVisible});
  visibilityPreferences.current={documents:documentsPreference,discussion:discussionVisible};
  const [draggedWidths,setDraggedWidths]=useState<{workspace:typeof workspaceSize;documents:boolean;discussion:boolean;widths:ReviewWidths}>();
  const documentsVisible=documentsPreference??automaticDocuments(workspaceSize.width,discussionVisible);
  // A drag holds the other side at its effective width, even after shrink has
  // clamped it. A new workspace measurement restores allocation from preferences.
  const allocated=draggedWidths?.workspace===workspaceSize&&draggedWidths.documents===documentsVisible&&draggedWidths.discussion===discussionVisible?draggedWidths.widths:preferredWidths;
  const layout=reviewV2Layout(workspaceSize.width,workspaceSize.narrow,documentsVisible,discussionVisible,allocated);
  const paneId=useId();
  const documentsId=`${paneId}-documents`,discussionId=`${paneId}-discussion`;
  const documentsToggle=useRef<HTMLButtonElement>(null),discussionToggle=useRef<HTMLButtonElement>(null);
  const togglePane=(side:ReviewSide)=>{
    const visible=side==='documents'?documentsVisible:discussionVisible;
    const content=document.getElementById(side==='documents'?documentsId:discussionId);
    if(visible&&(content?.contains(document.activeElement)||document.activeElement?.getAttribute('aria-controls')===content?.id)){
      (side==='documents'?documentsToggle:discussionToggle).current?.focus();
    }
    if(side==='documents')setDocumentsPreference(!visible);else setDiscussionVisible(!visible);
  };
  const separator=(side:ReviewSide)=><ReviewPaneSeparator side={side} controls={side==='documents'?documentsId:discussionId} width={layout[side]} maximum={layout.maximum[side]} onResize={width=>{
    setPreferredWidths(previous=>({...previous,[side]:width}));
    setDraggedWidths({workspace:workspaceSize,documents:documentsVisible,discussion:discussionVisible,widths:{documents:layout.documents,discussion:layout.discussion,[side]:width}});
  }}/>;
  const feedbackInput=useRef<InputRef>(null);
  const actionButtons=useRef(new Map<string,HTMLButtonElement|HTMLAnchorElement>());
  const [unavailableDocument,setUnavailableDocument]=useState(false);
  const [submitError,setSubmitError]=useState('');
  const submitBusy=useRef(false);
  const requestRef=useRef<ReviewV2Request>();
  const submissionAttempt=useRef<{requestId:string;submissionId:string}>();
  const pendingContribution=useRef<{requestId:string;submissionId:string;direction:string;feedback:string;files:File[];settings:Record<string,unknown>}>();
  const gear=useRef<HTMLButtonElement>(null);
  const fileInput=useRef<HTMLInputElement>(null);
  const current=view?.phase==='actionable'?view.current:undefined;
  const latestDraft=useRef({feedback,files});
  latestDraft.current={feedback,files};
  const confirmContribution=(receipt?:Record<string,unknown>)=>{
    const pending=pendingContribution.current;
    if(!pending||receipt?.request_id!==pending.requestId||receipt.submission_id!==pending.submissionId)return;
    pendingContribution.current=undefined;
    // Clear only the confirmed contribution, preserving edits made after an
    // ambiguous response and settings that did not apply to this direction.
    setFeedback(previous=>previous===pending.feedback?'':previous);
    setFiles(previous=>previous.filter(file=>!pending.files.includes(file)));
    if(fileInput.current&&Array.from(fileInput.current.files??[]).every(file=>pending.files.includes(file)))fileInput.current.value='';
    setDrafts(previous=>Object.fromEntries(Object.entries(previous).filter(([id,value])=>!Object.prototype.hasOwnProperty.call(pending.settings,id)||value!==pending.settings[id])));
    const newer=latestDraft.current.feedback!==pending.feedback||latestDraft.current.files.some(file=>!pending.files.includes(file));
    if(!newer){setDirection(previous=>previous===pending.direction?undefined:previous);setComposerOpen(false);}
    setSubmitError('');
  };
  useEffect(()=>{
    confirmContribution(view?.output?.receipt);
    for(const turn of view?.conversation??[])if(turn.role==='human')confirmContribution(turn.receipt);
  },[view]);
  const requestDocuments=view?.request.documents;
  const selection=view&&requestDocuments?resolveReviewDocumentLocation(location.search,reviewId,requestDocuments,view.request.document_metadata,view.request.overview_document_id,'review_id'):undefined;
  useEffect(()=>{
    if(selection?.kind!=='selected')return;
    if(selection.unavailable)setUnavailableDocument(true);
    if(selection.search!==location.search)navigate({pathname:location.pathname,search:selection.search,hash:location.hash},{replace:true});
  },[selection?.kind,selection?.kind==='selected'?selection.search:undefined,location,navigate]);
  const validSelection=selection?.kind==='selected';
  useReviewResponsePane(pane,!!view&&validSelection);
  useEffect(()=>{
    if(!workspace.current)return;
    const element=workspace.current;
    // Only the stable outer workspace controls defaults and clamping.
    const measure=()=>{
      const width=element.getBoundingClientRect().width,narrow=window.matchMedia('(max-width: 1000px)').matches;
      const preferences=visibilityPreferences.current;
      if(!(preferences.documents??automaticDocuments(width,preferences.discussion))){
        const navigation=document.getElementById(documentsId);
        if(navigation?.contains(document.activeElement)||document.activeElement?.getAttribute('aria-controls')===documentsId)documentsToggle.current?.focus();
      }
      setWorkspaceSize(previous=>previous.width===width&&previous.narrow===narrow?previous:{width,narrow});
    };
    const observer=typeof ResizeObserver==='undefined'?undefined:new ResizeObserver(measure);
    observer?.observe(element);window.addEventListener('resize',measure);measure();
    return ()=>{observer?.disconnect();window.removeEventListener('resize',measure);};
  },[!!view,validSelection]);
  useEffect(()=>{if(composerOpen)feedbackInput.current?.focus();},[composerOpen,direction]);
  useEffect(()=>{
    if(!view||requestRef.current===view.request)return;
    if(requestRef.current){
      const notices:string[]=[];
      setDrafts(previous=>{
        const next={...previous};
        for(const [id,value] of Object.entries(previous)){
          const setting=view.request.settings?.find(s=>s.id===id);
          if(!setting || validateReviewScalar(value,setting.schema) || (setting.control==='choice'&&!setting.choices?.some(c=>c.value===value)&&!(value===null&&setting.default===null))){delete next[id];notices.push(`${setting?.label??id} changed or was removed; please inspect its new declaration.`);}
          else if(value===setting.value)delete next[id];
        }
        if(notices.length)setDraftNotice(notices.join(' '));return next;
      });
      if(direction&&!view.request.directions.some(d=>d.id===direction))setDirection(undefined);
    }
    requestRef.current=view.request;
  },[view?.request]);
  if(!view)return <div className="review-page review-v2"><Button onClick={()=>navigate(reviewsPath)}>Back to Reviews</Button>{error?<Alert type="error" message="Review unavailable" description={error} action={<Button loading={busy} onClick={()=>void refresh()}>Refresh</Button>}/>:<p role="status">Loading review…</p>}</div>;
  const {request,phase}=view;
  const selected=request.directions.find(d=>d.id===direction);
  const policy=selected?reviewV2DirectionPolicy(request,selected):undefined;
  const allowUploads=policy?.allowAttachments??request.directions.some(d=>reviewV2DirectionPolicy(request,d).allowAttachments);
  const contributionError=selected?reviewV2ContributionError(request,selected,feedback,files.length):undefined;
  const draftConflict=(policy?.mode==='forbidden'&&feedback!=='')||(!allowUploads&&files.length>0)?contributionError:undefined;
  const documents=request.documents;
  const ready=phase==='actionable';
  const check=(id:string)=>{
    const selected=request.directions.find(d=>d.id===id);
    const contributionError=selected?reviewV2ContributionError(request,selected,feedback,files.length):undefined;
    let settingsError='';try{changedReviewSettings(request,id,drafts);}catch(e){settingsError=e instanceof Error?e.message:String(e);}
    if(!selected){setSubmitError('Choose a direction.');return false;}
    if(contributionError){setSubmitError(contributionError);return false;}
    if(settingsError){setSubmitError(settingsError);return false;}
    return true;
  };
  const submit=async(id:string)=>{
    if(!current?.form.request_id||submitBusy.current||!check(id))return;
    const changed=changedReviewSettings(request,id,drafts);
    submitBusy.current=true;setSubmitError('');sending();
    let contribution:typeof pendingContribution.current;
    try{
      if(submissionAttempt.current?.requestId!==current.form.request_id)submissionAttempt.current={requestId:current.form.request_id,submissionId:createSubmissionId()};
      contribution={...submissionAttempt.current,direction:id,feedback,files,settings:changed};
      pendingContribution.current=contribution;
      const output=await inputActivityService.submitReviewV2(tenant,job,current.form.request_id,{direction:id,feedback,attachments:[],settings:changed},files,submissionAttempt.current.submissionId);
      // The server receipt is the acceptance evidence. An invalid response stays ambiguous.
      if(output.receipt?.request_id!==contribution.requestId||output.receipt.submission_id!==contribution.submissionId)throw new Error('Submission receipt is unavailable. Refresh to reconcile acceptance.');
      confirmContribution(output.receipt);accepted(request,output);
    }catch(e){failed();if(!contribution||pendingContribution.current===contribution)setSubmitError(`${e instanceof Error?e.message:String(e)} Check the refreshed request before retrying.`);await refresh();}
    finally{submitBusy.current=false;}
  };
  const status=phase==='waiting'?'Response accepted':phase==='sending'?'Sending response…':phase==='sent'?'Response sent':phase==='terminal'?view.message:phase==='other'?view.message:phase==='unavailable'?'Review state unavailable':undefined;
  const choose=(id:string)=>{
    if(!ready||!current||submitBusy.current)return;
    setDirection(id);setSubmitError('');
    const action=request.directions.find(d=>d.id===id)!;
    if(reviewV2DirectionPolicy(request,action).mode==='forbidden'){
      if(check(id)){setComposerOpen(false);void submit(id);}
      else setComposerOpen(true);
    }else setComposerOpen(true);
  };
  const response=<div ref={pane} className="review-actions review-v2-response" aria-label="Your response">
    <div className="review-v2-response-inner">
    <div className="review-v2-response-title"><strong>Your response</strong>{!!request.settings?.length&&<Button ref={gear} icon={<SettingOutlined/>} aria-label="Review settings" disabled={!ready} onClick={()=>setSettingsOpen(true)}/>}</div>
    <div role="status" aria-live="polite">{status}<span className="sr-only">{phase==='actionable'&&view.conversation[view.conversation.length-1]?.role==='agent'?'New answer available in the discussion.':''}</span></div>
    {view.message&&phase==='unavailable'&&<p>{view.message}</p>}
    {error&&<Alert type="warning" message={error}/>}
    {(submitError||draftConflict)&&<Alert type="error" message={submitError||draftConflict}/>}
    {draftNotice&&<Alert type="warning" message={draftNotice} closable onClose={()=>setDraftNotice('')}/>}
    {phase==='other'&&<Button onClick={()=>navigate(reviewsPath)}>Open pending reviews</Button>}
    <fieldset disabled={!ready} hidden={!ready && phase !== 'unavailable'} style={{border:0,padding:0,margin:0,minWidth:0}}>
      <div hidden={composerOpen&&policy?.mode!=='forbidden'} className="review-v2-directions" aria-label="Review actions">{request.directions.map(d=><Button key={d.id} ref={element=>{if(element)actionButtons.current.set(d.id,element);else actionButtons.current.delete(d.id);}} type={direction===d.id?'primary':'default'} onClick={()=>choose(d.id)}>{d.label}</Button>)}</div>
      {Object.entries(drafts).filter(([id,value])=>request.settings?.find(s=>s.id===id)?.value!==value).map(([id,value])=>{
        const setting=request.settings?.find(s=>s.id===id);
        return <div key={id}>{setting?.label??id}: <code>{JSON.stringify(value)}</code> · {direction&&setting?.applies_to.includes(direction)?'Will be sent with this action':`Retained locally; applies to ${setting?.applies_to.map(id=>request.directions.find(d=>d.id===id)?.label??id).join(', ')}`}</div>;
      })}
      {!composerOpen&&(feedback!==''||files.length>0)&&<p>Unsent feedback draft retained. Choose a feedback action to edit it.</p>}
      <section hidden={!composerOpen} aria-label="Response composer" className="review-v2-composer">
      <strong>{selected?.label}</strong>
      <label htmlFor="review-v2-feedback">{request.feedback.label}{policy?.mode==='forbidden'?' (not allowed)':policy?.mode==='required'?' (required)':' (optional)'}</label>
      <Input.TextArea ref={feedbackInput} id="review-v2-feedback" value={feedback} readOnly={policy?.mode==='forbidden'} onChange={e=>{setFeedback(e.target.value);setSubmitError('');}} autoSize={{minRows:2,maxRows:5}}/>
      {feedback!==''&&<Button onClick={()=>{setFeedback('');setSubmitError('');}}>Remove feedback draft</Button>}
      {(allowUploads||files.length>0)&&<div className="review-v2-upload">{allowUploads&&<><label htmlFor="review-v2-files">Attach notes</label><input ref={fileInput} id="review-v2-files" type="file" multiple onChange={e=>{setFiles(Array.from(e.target.files??[]));setSubmitError('');}}/></>}{files.map((file,i)=><div key={i}><span>{file.name} ({file.size} bytes)</span> <Button aria-label={`Remove ${file.name} (${i+1})`} onClick={()=>{setFiles(previous=>previous.filter((_,index)=>index!==i));if(fileInput.current)fileInput.current.value='';setSubmitError('');}}>Remove</Button></div>)}</div>}
      <div className="review-v2-directions"><Button onClick={()=>{setComposerOpen(false);if(direction)actionButtons.current.get(direction)?.focus();}}>Back</Button>{policy?.mode!=='forbidden'&&<Button type="primary" onClick={()=>direction&&void submit(direction)}>Send · {selected?.label}</Button>}</div>
      </section>
    </fieldset>
    {!ready&&<Button loading={busy} disabled={phase==='sending'} onClick={()=>void refresh()}>Refresh</Button>}
    </div>
  </div>;
  const source = {
    identity: reviewId,
    contentIdentity: (id: string) => stringifyJobJSON(documents[id]?.stored) ?? id,
    url: (id: string) => documents[id]?.stored ? storedDocumentURL(tenant, documents[id].stored.key) : '',
  };
  return <div className="review-page review-v2">
    <div className="review-v2-heading"><Button icon={<ArrowLeftOutlined/>} onClick={()=>navigate(reviewsPath)}>Back to Reviews</Button><a href={`/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/story`}>Job activity</a></div>
    <ReviewV2Context tenant={tenant} job={job} metadata={view.job} onMetadata={setContextJob}/>
    <Typography.Title level={2}>{request.title}</Typography.Title>
    {unavailableDocument&&<Alert type="warning" message="The linked document is unavailable. Showing the starting document."/>}
    {!validSelection?<Alert type="warning" message={selection?.kind==='invalid'?'This review link is malformed':'This linked review is unavailable'} action={<Button onClick={()=>navigate({pathname:location.pathname,search:reviewDocumentSearch(location.search,reviewId,undefined,'review_id'),hash:location.hash})}>Open selected review</Button>}/>:<>
    <div className="review-v2-workspace-toolbar" aria-label="Review panes">
      <Tooltip title={documentsVisible?'Hide documents':'Show documents'}><Button ref={documentsToggle} aria-label={documentsVisible?'Hide documents':'Show documents'} aria-expanded={documentsVisible} aria-controls={documentsId} icon={documentsVisible?<MenuFoldOutlined/>:<MenuUnfoldOutlined/>} onClick={()=>togglePane('documents')}/></Tooltip>
      <Tooltip title={discussionVisible?'Hide discussion':'Show discussion'}><Button ref={discussionToggle} aria-label={discussionVisible?'Hide discussion':'Show discussion'} aria-expanded={discussionVisible} aria-controls={discussionId} icon={discussionVisible?<MenuFoldOutlined style={{transform:'scaleX(-1)'}}/>:<MenuUnfoldOutlined style={{transform:'scaleX(-1)'}}/>} onClick={()=>togglePane('discussion')}/></Tooltip>
    </div>
    <div ref={workspace} className={`review-v2-layout${layout.stacked?' stacked':''}`} style={{gridTemplateColumns:layout.stacked?'minmax(0,1fr)':`${documentsVisible?`${layout.documents}px 8px `:''}minmax(0,1fr)${discussionVisible?` 8px ${layout.discussion}px`:''}`}}><div className="review-v2-reader"><ReviewDocuments changes={<ReviewChanges projectId={tenant} jobId={job}/>} projectId={tenant} jobId={job} requestId={reviewId} source={source} documents={documents} documentMetadata={request.document_metadata} overviewDocumentId={request.overview_document_id} layout={{navigationVisible:documentsVisible,stacked:layout.stacked,navigationId:documentsId,separator:documentsVisible&&!layout.stacked?separator('documents'):null}} selection={{documentId:selection?.kind==='selected'?selection.documentId:undefined,onSelect:id=>navigate({pathname:location.pathname,search:reviewDocumentSearch(location.search,reviewId,id,'review_id'),hash:location.hash})}}/></div>
      {discussionVisible&&!layout.stacked&&separator('discussion')}
      <ReviewV2Discussion layoutReady={workspaceSize.width>0} id={discussionId} hidden={!discussionVisible} pending={!error&&reviewV2AnswerPending(view)} turns={view.conversation} request={request} declarations={view.declarations} attachmentURL={(turn,index)=>{
        const ref = view.conversation[turn]?.attachments[index];
        return ref?.stored ? storedDocumentURL(tenant, ref.stored.key) : '';
      }}/>
    </div>
    {response}</>}
    <ReviewV2Settings request={request} drafts={drafts} setDraft={(id,value)=>{setDrafts(p=>({...p,[id]:value}));}} open={settingsOpen} direction={direction} onClose={()=>{setSettingsOpen(false);gear.current?.focus();}}/>
  </div>;
}
