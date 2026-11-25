import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Button, Typography } from 'antd';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { storedDocumentURL, stringifyJobJSON, type RecipeJob, type UserInputDetails } from '@colony2/shared';
import { reviewV2AnswerPending } from '../utils/reviewV2State';
import { useReviewV2 } from '../hooks/useReviewV2';
import ReviewDocuments from './ReviewDocuments';
import ReviewChanges from './ReviewChanges';
import ReviewMessageBody from './ReviewMessageBody';
import './reviewV2.css';
import ReviewV2Context from './ReviewV2Context';
import ReviewReply from './ReviewReply';
import { reviewReplyKey, useReviewReplyDock } from '../hooks/useReviewReplyDock';
import { resolveReviewDocumentLocation, reviewDocumentSearch, reviewReturnPath } from '../utils/reviewDocumentLocation';

export default function ReviewV2({tenant,job,reviewId,initial,embedded=false,metadata}: {tenant:string;job:string;reviewId:string;initial?:UserInputDetails;embedded?:boolean;metadata?:RecipeJob}) {
  const [contextJob, setContextJob] = useState<RecipeJob>();
  const controller=useReviewV2(tenant,job,reviewId,initial,metadata ?? contextJob);
  const {view,error,busy,refresh}=controller;
  const dock=useReviewReplyDock();
  const navigate=useNavigate(); const location=useLocation();
  const reviewsPath=reviewReturnPath(tenant,location.search,location.state);
  const target={tenant,job,reviewId,returnTo:reviewsPath,title:view?.request.title || "Review reply",cellName:metadata?.cell_name ?? view?.job?.cell_name};
  const replyKey=reviewReplyKey(target);
  const restoredDraft = useRef<unknown>();
  const restoreDraft = useCallback(() => {
    if (!location.state?.openReviewDraft || restoredDraft.current === location.state) return false;
    restoredDraft.current = location.state;
    return true;
  }, [location.state]);
  const [unavailableDocument,setUnavailableDocument]=useState(false);
  const requestDocuments=view?.request.documents;
  const selection=view&&requestDocuments?resolveReviewDocumentLocation(location.search,reviewId,requestDocuments,view.request.document_metadata,view.request.overview_document_id,'review_id'):undefined;
  useEffect(()=>{
    if(selection?.kind!=='selected')return;
    if(selection.unavailable)setUnavailableDocument(true);
    if(selection.search!==location.search)navigate({pathname:location.pathname,search:selection.search,hash:location.hash},{replace:true,state:location.state});
  },[selection?.kind,selection?.kind==='selected'?selection.search:undefined,location,navigate]);
  const validSelection=selection?.kind==='selected';
  if(!view)return <div className={`review-page review-v2${embedded?' embedded-review':''}`}>{!embedded&&<Button onClick={()=>navigate(reviewsPath)}>Back to Reviews</Button>}{error?<Alert type="error" message="Review unavailable" description={error} action={<Button loading={busy} onClick={()=>void refresh()}>Refresh</Button>}/>:<p role="status">Loading review…</p>}</div>;
  const {request}=view;
  const documents=request.documents;
  const hasDocuments=Object.keys(documents).length>0;
  const source = {
    identity: reviewId,
    contentIdentity: (id: string) => stringifyJobJSON(documents[id]?.stored) ?? id,
    url: (id: string) => documents[id]?.stored ? storedDocumentURL(tenant, documents[id].stored.key) : '',
  };
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
    <div className="review-v2-layout">
      <ReviewMessageBody request={request} tenant={tenant} hasDocument={hasDocuments} />
      {hasDocuments&&<ReviewDocuments attachmentLayout="primary" changes={<ReviewChanges projectId={tenant} jobId={job}/>} projectId={tenant} jobId={job} requestId={reviewId}
        source={source} documents={documents} documentMetadata={request.document_metadata} overviewDocumentId={request.overview_document_id}
        selection={{ documentId: selection.documentId, onSelect: (id, options) => navigate({
          pathname: location.pathname, search: reviewDocumentSearch(location.search, reviewId, id, 'review_id'), hash: location.hash,
        }, { ...options, state: location.state }) }} />}
    </div>
    {!error && reviewV2AnswerPending(view) && <p role="status" className="review-v2-pending">Response pending</p>}
    {dock?.has(replyKey) ? <Button type="text" onClick={()=>dock.activate(replyKey)}>Show popped-out reply</Button>
      : <ReviewReply tenant={tenant} job={job} reviewId={reviewId} controller={controller} restoreDraft={restoreDraft} onSubmitted={()=>navigate(reviewsPath,{replace:true})} onPopout={dock ? ()=>dock.open(target) : undefined} />}</>}
  </div>;
}
