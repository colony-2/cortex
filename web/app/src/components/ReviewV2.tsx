import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Input, Radio, Space, Typography } from 'antd';
import { SettingOutlined, ArrowLeftOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { changedReviewSettings, conversationArtifactURL, reviewV2ContributionError, reviewV2DirectionPolicy, createSubmissionId, inputActivityService, stringifyJobJSON, validateReviewScalar, type ReviewV2Request, type ReviewV2Turn, type UserInputDetails } from '@colony2/shared';
import { reviewReturnSearch } from '../utils/reviewFilters';
import { useReviewV2 } from '../hooks/useReviewV2';
import ReviewDocuments from './ReviewDocuments';
import ReviewV2Settings from './ReviewV2Settings';
import { historicalDocuments, historicalTurnAttachments } from './historicalDocuments';
import { useAssistantSource } from './AssistantContext';
import './reviewV2.css';

export function ReviewV2Discussion({turns,request,attachmentURL,declarations}: {turns:ReviewV2Turn[];request:ReviewV2Request;declarations?:Record<string,ReviewV2Request>;attachmentURL?:(turn:number,index:number)=>string}) {
  const [visible,setVisible]=useState(20);
  return <section aria-label="Discussion" data-review={request.review_id} className="review-v2-discussion">
    <Typography.Title level={4}>Discussion</Typography.Title>
    {turns.length>visible&&<Button onClick={()=>setVisible(n=>n+20)}>Show earlier turns ({turns.length-visible})</Button>}
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
    {!turns.length&&<p>No questions yet. You can ask about the material before choosing a direction.</p>}
  </section>;
}
export default function ReviewV2({tenant,job,reviewId,initial}: {tenant:string;job:string;reviewId:string;initial?:UserInputDetails}) {
  const {view,error,busy,refresh,sending,accepted,failed}=useReviewV2(tenant,job,reviewId,initial);
  const navigate=useNavigate(); const [search,setSearch]=useSearchParams();
  const reviewsPath=`/project/${encodeURIComponent(tenant)}/reviews${reviewReturnSearch(search)}`;
  const [settingsOpen,setSettingsOpen]=useState(false);
  const [drafts,setDrafts]=useState<Record<string,unknown>>({});
  const [draftNotice,setDraftNotice]=useState('');
  const [direction,setDirection]=useState<string>();
  const [feedback,setFeedback]=useState('');
  const [files,setFiles]=useState<File[]>([]);
  const [confirming,setConfirming]=useState(false);
  const [submitError,setSubmitError]=useState('');
  const submitBusy=useRef(false);
  const requestRef=useRef<ReviewV2Request>();
  const submissionAttempt=useRef<{requestId:string;submissionId:string}>();
  const pendingContribution=useRef<{requestId:string;submissionId:string;direction:string;feedback:string;files:File[];settings:Record<string,unknown>}>();
  const gear=useRef<HTMLButtonElement>(null);
  const fileInput=useRef<HTMLInputElement>(null);
  const current=view?.phase==='actionable'?view.current:undefined;
  useAssistantSource(current?.form.request_id?{kind:'review',job_id:job,request_id:current.form.request_id}:undefined);
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
    setDirection(previous=>previous===pending.direction?undefined:previous);
    setConfirming(false);setSubmitError('');
  };
  useEffect(()=>{
    confirmContribution(view?.output?.receipt);
    for(const turn of view?.conversation??[])if(turn.role==='human')confirmContribution(turn.receipt);
  },[view]);
  useEffect(()=>{setSearch(p=>{if(p.get('review_id')===reviewId)return p; const next=new URLSearchParams(p);next.set('review_id',reviewId);return next;},{replace:true});},[reviewId,setSearch]);
  useEffect(()=>{
    if(!view||requestRef.current===view.request)return;
    setConfirming(false);
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
  const documentHistory=view.documentHistory;
  const catalog=documentHistory?historicalDocuments({...documentHistory,raw_form:{presentation:{type:'review/v2'},request}}):undefined;
  const documents=catalog?.documents??request.documents;
  const ready=phase==='actionable'&&!submitBusy.current;
  let changed:Record<string,any>={};let settingsError='';
  try{changed=changedReviewSettings(request,direction??'',drafts);}catch(e){settingsError=e instanceof Error?e.message:String(e);}
  const check=()=>{
    if(!selected){setSubmitError('Choose a direction.');return false;}
    if(contributionError){setSubmitError(contributionError);return false;}
    if(settingsError){setSubmitError(settingsError);return false;}
    return true;
  };
  const submit=async()=>{
    if(!current?.form.request_id||submitBusy.current||!check())return;
    submitBusy.current=true;setSubmitError('');sending();
    let contribution:typeof pendingContribution.current;
    try{
      if(submissionAttempt.current?.requestId!==current.form.request_id)submissionAttempt.current={requestId:current.form.request_id,submissionId:createSubmissionId()};
      contribution={...submissionAttempt.current,direction:direction!,feedback,files,settings:changed};
      pendingContribution.current=contribution;
      const output=await inputActivityService.submitReviewV2(tenant,job,current.form.request_id,{direction:direction!,feedback,attachments:[],settings:changed},files,submissionAttempt.current.submissionId);
      // The server receipt is the acceptance evidence. An invalid response stays ambiguous.
      if(output.receipt?.request_id!==contribution.requestId||output.receipt.submission_id!==contribution.submissionId)throw new Error('Submission receipt is unavailable. Refresh to reconcile acceptance.');
      confirmContribution(output.receipt);accepted(request,output);
    }catch(e){failed();if(!contribution||pendingContribution.current===contribution)setSubmitError(`${e instanceof Error?e.message:String(e)} Check the refreshed request before retrying.`);setConfirming(false);await refresh();}
    finally{submitBusy.current=false;}
  };
  const status=phase==='waiting'?'Waiting for the next turn':phase==='sending'?'Sending response…':phase==='sent'?'Response sent':phase==='terminal'?view.message:phase==='other'?view.message:phase==='unavailable'?'Review state unavailable':undefined;
  const response=<Card className="review-v2-response">
    <div className="review-v2-response-title"><Typography.Title level={4}>Your response</Typography.Title>{!!request.settings?.length&&<Button ref={gear} icon={<SettingOutlined/>} aria-label="Review settings" disabled={!ready} onClick={()=>setSettingsOpen(true)}/>}</div>
    <div role="status" aria-live="polite">{status}<span className="sr-only">{phase==='actionable'&&view.conversation[view.conversation.length-1]?.role==='agent'?'New answer available in the discussion.':''}</span></div>
    {view.message&&phase==='unavailable'&&<p>{view.message}</p>}
    {error&&<Alert type="warning" message={error}/>}
    {(submitError||draftConflict)&&<Alert type="error" message={submitError||draftConflict}/>}
    {draftNotice&&<Alert type="warning" message={draftNotice} closable onClose={()=>setDraftNotice('')}/>}
    {phase==='other'&&<Button onClick={()=>navigate(reviewsPath)}>Open pending reviews</Button>}
    {ready&&<>
      <Radio.Group aria-label="Review direction" value={direction} onChange={e=>{setDirection(e.target.value);setConfirming(false);setSubmitError('');}} className="review-v2-directions">
        {request.directions.map(d=>{const rules=reviewV2DirectionPolicy(request,d);return <Radio key={d.id} value={d.id}>{d.label} <small>{rules.mode==='forbidden'?'No feedback or attachments':rules.mode==='required'?'Feedback required':'Feedback optional'}{rules.mode!=='forbidden'&&!rules.allowAttachments?' · No attachments':''}</small></Radio>;})}
      </Radio.Group>
      <label htmlFor="review-v2-feedback">{request.feedback.label}{policy?.mode==='forbidden'?' (not allowed)':policy?.mode==='required'?' (required)':' (optional)'}</label>
      <Input.TextArea id="review-v2-feedback" value={feedback} readOnly={policy?.mode==='forbidden'} onChange={e=>{setFeedback(e.target.value);setConfirming(false);setSubmitError('');}} autoSize={{minRows:3,maxRows:10}}/>
      {feedback!==''&&<Button onClick={()=>{setFeedback('');setConfirming(false);setSubmitError('');}}>Remove feedback draft</Button>}
      {(allowUploads||files.length>0)&&<div className="review-v2-upload">{allowUploads&&<><label htmlFor="review-v2-files">Attach notes</label><input ref={fileInput} id="review-v2-files" type="file" multiple onChange={e=>{setFiles(Array.from(e.target.files??[]));setConfirming(false);setSubmitError('');}}/></>}{files.map((file,i)=><div key={i}><span>{file.name} ({file.size} bytes)</span> <Button aria-label={`Remove ${file.name} (${i+1})`} onClick={()=>{setFiles(previous=>previous.filter((_,index)=>index!==i));if(fileInput.current)fileInput.current.value='';setConfirming(false);setSubmitError('');}}>Remove</Button></div>)}</div>}
      {confirming?<section aria-label="Confirm response"><p>Send <strong>{selected?.label}</strong>?</p>{Object.entries(changed).map(([id,v])=><p key={id}>{request.settings?.find(s=>s.id===id)?.label??id}: <code>{JSON.stringify(v)}</code></p>)}<Space><Button onClick={()=>setConfirming(false)}>Back</Button><Button type="primary" onClick={()=>void submit()}>Confirm {selected?.label}</Button></Space></section>:<Button type="primary" onClick={()=>{if(check()){setConfirming(true);setSubmitError('');}}}>Continue</Button>}
    </>}
    {!ready&&<Button loading={busy} disabled={phase==='sending'} onClick={()=>void refresh()}>Refresh</Button>}
  </Card>;
  const history=view.history;
  const source=documentHistory&&catalog?{
    identity:reviewId,
    contentIdentity:(id:string)=>stringifyJobJSON(documents[id]?.stored??documents[id])??id,
    url:(id:string)=>catalog.members[id]?conversationArtifactURL(tenant,job,documentHistory.entry_id,catalog.members[id],documentHistory.snapshot_token):'',
    onError:()=>{void refresh();},
  }:{identity:reviewId,contentIdentity:(id:string)=>stringifyJobJSON(documents[id]?.stored??documents[id])??id,url:()=>''};
  const notes=history?historicalTurnAttachments(history):undefined;
  return <div className="review-page review-v2">
    <div className="review-v2-heading"><Button icon={<ArrowLeftOutlined/>} onClick={()=>navigate(reviewsPath)}>Back to Reviews</Button><a href={`/project/${tenant}/jobs/${job}/story`}>Job activity</a></div>
    <Typography.Title level={2}>{request.title}</Typography.Title>
    <p className="review-v2-question">{request.question}</p>
    <div className="review-v2-layout"><div className="review-v2-reader"><ReviewDocuments projectId={tenant} jobId={job} requestId={reviewId} source={source} documents={documents} documentMetadata={request.document_metadata} overviewDocumentId={request.overview_document_id} response={response}/></div>
      <ReviewV2Discussion turns={view.conversation} request={request} declarations={view.declarations} attachmentURL={(turn,index)=>{
        const match=notes?.[`${turn}/${index}`];return match&&history?conversationArtifactURL(tenant,job,history.entry_id,match.member,history.snapshot_token):'';
      }}/>
    </div>
    <ReviewV2Settings request={request} drafts={drafts} setDraft={(id,value)=>{setDrafts(p=>({...p,[id]:value}));setConfirming(false);}} open={settingsOpen} direction={direction} onClose={()=>{setSettingsOpen(false);gear.current?.focus();}}/>
  </div>;
}
