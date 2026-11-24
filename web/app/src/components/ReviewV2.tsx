import { useEffect, useLayoutEffect, useRef, useState, useId, type ReactNode } from 'react';
import { Alert, Button, Tooltip, Typography } from 'antd';
import { ArrowLeftOutlined, MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { storedDocumentURL, stringifyJobJSON, type RecipeJob, type ReviewV2Request, type ReviewV2Turn, type UserInputDetails } from '@colony2/shared';
import { reviewV2AnswerPending } from '../utils/reviewV2State';
import { useReviewV2 } from '../hooks/useReviewV2';
import ReviewDocuments from './ReviewDocuments';
import ReviewChanges from './ReviewChanges';
import ReviewQuestion from './ReviewQuestion';
import ReviewPaneSeparator from './ReviewPaneSeparator';
import { reviewPaneDefaults, reviewV2Layout } from '../utils/reviewV2Layout';
import './reviewV2.css';
import ReviewV2Context from './ReviewV2Context';
import ReviewReply from './ReviewReply';
import { reviewReplyKey, useReviewReplyDock } from '../hooks/useReviewReplyDock';
import { resolveReviewDocumentLocation, reviewDocumentSearch, reviewInboxPath } from '../utils/reviewDocumentLocation';
import ThreadMessageLabel, { reviewContributionLabel } from './ThreadMessageLabel';
import ThreadMessageTime from './ThreadMessageTime';

export function ReviewV2Discussion({turns,request,attachmentURL,declarations,pending=false,id,hidden=false,showMessageLabels=false,headingAction}: {id?:string;hidden?:boolean;turns:ReviewV2Turn[];request:ReviewV2Request;declarations?:Record<string,ReviewV2Request>;pending?:boolean;attachmentURL?:(turn:number,index:number)=>string;showMessageLabels?:boolean;headingAction?:ReactNode}) {
  const [visible,setVisible]=useState(20);
  return <section id={id} hidden={hidden} aria-label="Discussion" data-review={request.review_id} className="review-v2-discussion">
    <header className="review-v2-discussion-heading">
      <Typography.Title level={4}>Discussion</Typography.Title>
      {headingAction}
    </header>
    {turns.length>visible&&<Button onClick={()=>setVisible(n=>n+20)}>Show earlier turns ({turns.length-visible})</Button>}
    {turns.slice(-visible).map((turn,index)=>{
      const actual=turns.length>visible?turns.length-visible+index:index;
      const frozen=turn.role==='human'&&typeof turn.receipt?.request_id==='string'?declarations?.[turn.receipt.request_id]:undefined;
      const frozenDirection=frozen?.directions.find(d=>d.id===turn.direction);
      const direction=frozen?frozenDirection:request.directions.find(d=>d.id===turn.direction);
      const heading=<>
        <strong>{turn.role==='human'?'Reviewer':'Agent'}</strong>
        {showMessageLabels&&<ThreadMessageLabel label={turn.role==='agent'?'Answer':reviewContributionLabel(direction)}/>}
        {turn.direction&&<span> · {frozenDirection?.label??turn.direction}</span>}
      </>;
      return <article key={actual} className={`review-v2-turn ${turn.role}`}>
        {(showMessageLabels || turn.receipt?.submitted_at) ? <span className="thread-message-meta">
          {heading}
          <ThreadMessageTime value={turn.receipt?.submitted_at} />
        </span> : heading}
        <p style={{whiteSpace:'pre-wrap'}}>{turn.text||'No written feedback'}</p>
        {Object.entries(turn.settings??{}).map(([id,value])=><p key={id}>{frozen?.settings?.find(s=>s.id===id)?.label??id}: <code>{JSON.stringify(value)}</code></p>)}
        {turn.attachments.map((ref,i)=>{const url=attachmentURL?.(actual,i);return <p key={i}>{url?<a href={url} download>Download {(ref.name||ref.stored.key.name).split('/').pop()}</a>:<span>{(ref.name||ref.stored.key.name).split('/').pop()} (retained download unavailable)</span>}</p>;})}
      </article>;
    })}
    {pending&&<p role="status" className="review-v2-pending">Response pending</p>}
    {request.question.trim()&&<ReviewQuestion question={request.question} showMessageLabel={showMessageLabels}/>}
    {!turns.length&&!request.question.trim()&&<p>No questions yet. You can ask about the material before choosing a direction.</p>}
  </section>;
}
export default function ReviewV2({tenant,job,reviewId,initial,embedded=false,metadata}: {tenant:string;job:string;reviewId:string;initial?:UserInputDetails;embedded?:boolean;metadata?:RecipeJob}) {
  const [contextJob, setContextJob] = useState<RecipeJob>();
  const controller=useReviewV2(tenant,job,reviewId,initial,metadata ?? contextJob);
  const {view,error,busy,refresh}=controller;
  const dock=useReviewReplyDock();
  const target={tenant,job,reviewId,title:view?.request.title || "Review reply"};
  const replyKey=reviewReplyKey(target);
  const navigate=useNavigate(); const location=useLocation();
  const reviewsPath=reviewInboxPath(tenant,location.search);
  const workspace=useRef<HTMLDivElement>(null);
  const [workspaceSize,setWorkspaceSize]=useState({width:0,narrow:false});
  const [showDiscussion,setShowDiscussion]=useState(true);
  const [discussionWidth,setDiscussionWidth]=useState(reviewPaneDefaults.discussion);
  const hasDocuments=!!view&&Object.keys(view.request.documents).length>0;
  const discussionVisible=!hasDocuments||showDiscussion;
  const layout=reviewV2Layout(workspaceSize.width,workspaceSize.narrow,false,hasDocuments&&discussionVisible,{...reviewPaneDefaults,discussion:discussionWidth});
  const discussionId=useId();
  const discussionToggle=useRef<HTMLButtonElement>(null);
  const focusDiscussionToggle=useRef(false);
  useLayoutEffect(()=>{
    if(!focusDiscussionToggle.current)return;
    focusDiscussionToggle.current=false;
    discussionToggle.current?.focus();
  },[discussionVisible]);
  const [unavailableDocument,setUnavailableDocument]=useState(false);
  const requestDocuments=view?.request.documents;
  const selection=view&&requestDocuments?resolveReviewDocumentLocation(location.search,reviewId,requestDocuments,view.request.document_metadata,view.request.overview_document_id,'review_id'):undefined;
  useEffect(()=>{
    if(selection?.kind!=='selected')return;
    if(selection.unavailable)setUnavailableDocument(true);
    if(selection.search!==location.search)navigate({pathname:location.pathname,search:selection.search,hash:location.hash},{replace:true,state:location.state});
  },[selection?.kind,selection?.kind==='selected'?selection.search:undefined,location,navigate]);
  const validSelection=selection?.kind==='selected';
  useEffect(()=>{
    if(!workspace.current)return;
    const element=workspace.current;
    // Only the stable outer workspace controls defaults and clamping.
    const measure=()=>{
      const width=element.getBoundingClientRect().width,narrow=window.matchMedia('(max-width: 1000px)').matches;
      setWorkspaceSize(previous=>previous.width===width&&previous.narrow===narrow?previous:{width,narrow});
    };
    const observer=typeof ResizeObserver==='undefined'?undefined:new ResizeObserver(measure);
    observer?.observe(element);window.addEventListener('resize',measure);measure();
    return ()=>{observer?.disconnect();window.removeEventListener('resize',measure);};
  },[!!view,validSelection]);
  if(!view)return <div className={`review-page review-v2${embedded?' embedded-review':''}`}>{!embedded&&<Button onClick={()=>navigate(reviewsPath)}>Back to Reviews</Button>}{error?<Alert type="error" message="Review unavailable" description={error} action={<Button loading={busy} onClick={()=>void refresh()}>Refresh</Button>}/>:<p role="status">Loading review…</p>}</div>;
  const {request}=view;
  const documents=request.documents;
  const source = {
    identity: reviewId,
    contentIdentity: (id: string) => stringifyJobJSON(documents[id]?.stored) ?? id,
    url: (id: string) => documents[id]?.stored ? storedDocumentURL(tenant, documents[id].stored.key) : '',
  };
  const discussionControl = hasDocuments && validSelection && <Tooltip title={discussionVisible?'Hide discussion':'Show discussion'}>
    <Button size="small" type="text" ref={discussionToggle} aria-label={discussionVisible?'Hide discussion':'Show discussion'}
      aria-expanded={discussionVisible} aria-controls={discussionId} onClick={()=>{
        focusDiscussionToggle.current=true;
        setShowDiscussion(visible=>!visible);
      }}
      icon={discussionVisible?<MenuFoldOutlined style={{transform:'scaleX(-1)'}}/>:<MenuUnfoldOutlined style={{transform:'scaleX(-1)'}}/>} />
  </Tooltip>;
  return <div className={`review-page review-v2${embedded?' embedded-review':''}`}>
    {!embedded && <header className="review-v2-heading">
      <div className="review-v2-heading-links">
        <Button type="text" size="small" icon={<ArrowLeftOutlined aria-hidden />} onClick={()=>navigate(reviewsPath)}>Back to Reviews</Button>
        <a href={`/project/${encodeURIComponent(tenant)}/jobs/${encodeURIComponent(job)}/story`}>Job activity</a>
      </div>
      <Typography.Title level={4} className="review-v2-title">{request.title}</Typography.Title>
    </header>}
    {!embedded&&<ReviewV2Context tenant={tenant} job={job} metadata={view.job} onMetadata={setContextJob}/>}
    {unavailableDocument&&<Alert type="warning" message="The linked document is unavailable. Showing the starting document."/>}
    {!validSelection?<Alert type="warning" message={selection?.kind==='invalid'?'This review link is malformed':'This linked review is unavailable'} action={<Button onClick={()=>navigate({pathname:location.pathname,search:reviewDocumentSearch(location.search,reviewId,undefined,'review_id'),hash:location.hash})}>Open selected review</Button>}/>:<>
    <div ref={workspace} className={`review-v2-layout${!hasDocuments||layout.stacked?' stacked':''}`} style={{gridTemplateColumns:hasDocuments&&discussionVisible&&!layout.stacked?`minmax(0,1fr) 8px ${layout.discussion}px`:'minmax(0,1fr)'}}>
      {hasDocuments&&<ReviewDocuments attachmentLayout="primary" changes={<ReviewChanges projectId={tenant} jobId={job}/>} projectId={tenant} jobId={job} requestId={reviewId}
        primaryActions={!discussionVisible && discussionControl}
        source={source} documents={documents} documentMetadata={request.document_metadata} overviewDocumentId={request.overview_document_id}
        selection={{ documentId: selection.documentId, onSelect: (id, options) => navigate({
          pathname: location.pathname, search: reviewDocumentSearch(location.search, reviewId, id, 'review_id'), hash: location.hash,
        }, { ...options, state: location.state }) }} />}
      {hasDocuments&&discussionVisible&&!layout.stacked&&<ReviewPaneSeparator side="discussion" controls={discussionId} width={layout.discussion} maximum={layout.maximum.discussion} onResize={setDiscussionWidth}/>}
      <ReviewV2Discussion id={discussionId} hidden={!discussionVisible} headingAction={discussionVisible && discussionControl} pending={!error&&reviewV2AnswerPending(view)} turns={view.conversation} request={request} declarations={view.declarations} showMessageLabels={embedded} attachmentURL={(turn,index)=>{
        const ref = view.conversation[turn]?.attachments[index];
        return ref?.stored ? storedDocumentURL(tenant, ref.stored.key) : '';
      }}/>
    </div>
    {dock?.has(replyKey) ? <Button type="text" onClick={()=>dock.activate(replyKey)}>Show popped-out reply</Button>
      : <ReviewReply tenant={tenant} job={job} reviewId={reviewId} controller={controller} onPopout={dock ? ()=>dock.open(target) : undefined} />}</>}
  </div>;
}
