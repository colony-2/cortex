import { useEffect, useRef, useState, useId } from 'react';
import { Alert, Button, Dropdown, Input, Tooltip, type InputRef } from 'antd';
import { CheckCircleOutlined, DownOutlined, EditOutlined, ExportOutlined, ImportOutlined, PaperClipOutlined, QuestionCircleOutlined, SettingOutlined } from '@ant-design/icons';
import { useLocation, useNavigate } from 'react-router-dom';
import { changedReviewSettings, createSubmissionId, getUserEmail, inputActivityService, reviewV2ContributionError, reviewV2DirectionPolicy, useInputActivity, validateReviewScalar, type ReviewV2Direction, type ReviewV2Request } from '@colony2/shared';
import { useReviewDraft, useReviewDraftRef } from '../hooks/useReviewDraft';
import { useReviewV2 } from '../hooks/useReviewV2';
import { reviewReplyKey, type ReviewReplyTarget } from '../hooks/useReviewReplyDock';
import { reviewInboxPath, reviewThreadPath } from '../utils/reviewDocumentLocation';
import ComposerWindow from './ComposerWindow';
import ReviewV2Settings from './ReviewV2Settings';
import './reviewV2.css';

function responseIcon(direction: ReviewV2Direction) {
  const label = direction.label.trim();
  if (/^(approve|accept)\b/i.test(label)) return <CheckCircleOutlined aria-hidden />;
  if (/^request changes\b/i.test(label)) return <EditOutlined aria-hidden />;
  if (direction.continues_review && /\b(question|clarification)\b/i.test(label)) return <QuestionCircleOutlined aria-hidden />;
}

type Controller = ReturnType<typeof useReviewV2>;

export function ReviewReplyWindow({ target, active, onActivate, onClose, onSendingChange }: {
  target: ReviewReplyTarget; active: boolean; onActivate: () => void; onClose: () => void; onSendingChange:(sending:boolean)=>void;
}) {
  const {tenant,job,reviewId,title} = target;
  const controller = useReviewV2(tenant,job,reviewId);
  const { currentProjectId } = useInputActivity();
  const navigate = useNavigate();
  const sending = controller.view?.phase === 'sending';
  useEffect(() => { onSendingChange(sending); }, [sending]);
  useEffect(() => {
    if (tenant === currentProjectId) return;
    queueMicrotask(() => { void inputActivityService.getPendingInputs(tenant, true).catch(() => {}); });
  }, [tenant,currentProjectId]);
  const popIn = () => {
    onClose();
    navigate(reviewThreadPath(tenant,job,reviewId), {state:{openReviewDraft:true}});
  };
  return <ComposerWindow label="Popped-out review reply" title={title} busy={sending} active={active} onActivate={onActivate} onClose={onClose}
    extra={<Tooltip title="Return reply to thread"><Button type="text" icon={<ImportOutlined aria-hidden />} aria-label="Return reply to thread" disabled={sending} onClick={popIn} /></Tooltip>}>
    {collapsed => <div hidden={collapsed} className="review-reply-window-body" onPointerDown={onActivate}>
      <ReviewReply tenant={tenant} job={job} reviewId={reviewId} controller={controller} docked />
    </div>}
  </ComposerWindow>;
}

export default function ReviewReply({ tenant,job,reviewId,controller,onPopout,docked=false }: {
  tenant:string; job:string; reviewId:string; controller:Controller; onPopout?:()=>void; docked?:boolean;
}) {
  const {view,error,busy,refresh,sending,accepted,failed} = controller;
  const navigate = useNavigate();
  const location = useLocation();
  const fieldSuffix = useId();
  const feedbackId = docked ? `review-v2-feedback-${fieldSuffix}` : 'review-v2-feedback';
  const filesId = docked ? `review-v2-files-${fieldSuffix}` : 'review-v2-files';
  const [settingsOpen,setSettingsOpen]=useState(false);
  const draftIdentity=reviewReplyKey({tenant,job,reviewId});
  useReviewDraft(draftIdentity, 'review-kind', 'review_id');
  const [drafts,setDrafts]=useReviewDraft<Record<string,unknown>>(draftIdentity,'settings',{});
  const [draftNotice,setDraftNotice]=useState('');
  const [direction,setDirection]=useReviewDraft<string|undefined>(draftIdentity,'direction',undefined);
  const [feedback,setFeedback]=useReviewDraft(draftIdentity,'feedback','');
  const [files,setFiles]=useReviewDraft<File[]>(draftIdentity,'files',[]);
  const [composerOpen,setComposerOpen]=useReviewDraft(draftIdentity,'composer',false);
  const selected=view?.request.directions.find(d=>d.id===direction);
  const policy=view&&selected?reviewV2DirectionPolicy(view.request,selected):undefined;
  useEffect(() => {
    if (!docked && location.state?.openReviewDraft) setComposerOpen(true);
  }, [location.key]);
  const feedbackInput=useRef<InputRef>(null);
  const sendButton=useRef<HTMLButtonElement>(null);
  const actionButtons=useRef(new Map<string,HTMLButtonElement|HTMLAnchorElement>());
  const [submitError,setSubmitError]=useReviewDraft(draftIdentity,'error','');
  const submitBusy=useRef(false);
  const requestRef=useRef<ReviewV2Request>();
  const submissionAttempt=useReviewDraftRef<{requestId:string;submissionId:string}|undefined>(draftIdentity,'attempt',undefined);
  const pendingContribution=useReviewDraftRef<{requestId:string;submissionId:string;direction:string;feedback:string;files:File[];settings:Record<string,unknown>}|undefined>(draftIdentity,'contribution',undefined);
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
  useEffect(()=>{
    if(!composerOpen || !direction || view?.phase !== 'actionable') return;
    if (policy?.mode !== 'forbidden') feedbackInput.current?.focus();
    else sendButton.current?.focus();
  },[composerOpen,direction,policy?.mode,view?.phase]);
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
  if(!view) return <div className="review-v2-response" role="status">{error || 'Loading review reply…'}</div>;
  const {request,phase}=view;
  const allowUploads=policy?.allowAttachments??request.directions.some(d=>reviewV2DirectionPolicy(request,d).allowAttachments);
  const contributionError=selected?reviewV2ContributionError(request,selected,feedback,files.length):undefined;
  const draftConflict=(policy?.mode==='forbidden'&&feedback!=='')||(!allowUploads&&files.length>0)?contributionError:undefined;
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
    setDirection(id);setSubmitError('');setComposerOpen(true);
  };
  const composing = composerOpen && selected;
  const icon = selected && responseIcon(selected);
  const settingsButton = !!request.settings?.length && <Tooltip title="Review settings"><Button ref={gear} type="text" icon={<SettingOutlined aria-hidden />} aria-label="Review settings" disabled={!ready} onClick={()=>setSettingsOpen(true)}/></Tooltip>;
  const response=<section className="review-v2-response" aria-label="Your response">
    <div className="review-v2-response-inner">
    {status ? <div role="status">{status}</div> : ready && view.conversation[view.conversation.length - 1]?.role==='agent' && <span className="sr-only" role="status">New answer available in the discussion.</span>}
    {view.message&&phase==='unavailable'&&<p>{view.message}</p>}
    {error&&<Alert type="warning" message={error}/>}
    {(submitError||(composing&&draftConflict))&&<Alert type="error" message={submitError||draftConflict}/>}
    {draftNotice&&<Alert type="warning" message={draftNotice} closable onClose={()=>setDraftNotice('')}/>}
    {phase==='other'&&<Button onClick={()=>navigate(reviewInboxPath(tenant,location.search))}>Open pending reviews</Button>}
    <fieldset disabled={!ready} hidden={!ready && phase !== 'unavailable'} style={{border:0,padding:0,margin:0,minWidth:0}}>
      <div hidden={!!composing} className="review-v2-directions" role="group" aria-label="Review actions">
        {request.directions.map(d=><Button key={d.id} ref={element=>{if(element)actionButtons.current.set(d.id,element);else actionButtons.current.delete(d.id);}} icon={responseIcon(d)} onClick={()=>choose(d.id)}>{d.label}</Button>)}
        {!composing && settingsButton}
      </div>
      {Object.entries(drafts).filter(([id,value])=>request.settings?.find(s=>s.id===id)?.value!==value).map(([id,value])=>{
        const setting=request.settings?.find(s=>s.id===id);
        return <div key={id}>{setting?.label??id}: <code>{JSON.stringify(value)}</code> · {direction&&setting?.applies_to.includes(direction)?'Will be sent with this action':`Retained locally; applies to ${setting?.applies_to.map(id=>request.directions.find(d=>d.id===id)?.label??id).join(', ')}`}</div>;
      })}
      {!composing&&(feedback!==''||files.length>0)&&<p>Unsent feedback draft retained. Choose a feedback action to edit it.</p>}
      {composing && <section aria-label="Response composer" className="review-v2-composer">
        <header className="review-v2-composer-header">
          <Dropdown trigger={['click']} disabled={!ready} menu={{
            selectable:true, selectedKeys:[selected.id],
            items:request.directions.map(d=>({key:d.id,label:d.label,icon:responseIcon(d)})),
            onClick: ({key})=>choose(key),
          }}>
            <Button type="text" icon={icon} aria-haspopup="menu" aria-label={`Change response type: ${selected.label}`} title={selected.label}>
              {!icon && selected.label}<DownOutlined aria-hidden />
            </Button>
          </Dropdown>
          <span>{getUserEmail() || 'Unknown'}</span>
          {settingsButton}
          {onPopout && <Tooltip title="Pop out reply"><Button type="text" icon={<ExportOutlined aria-hidden />} aria-label="Pop out reply" onClick={onPopout} /></Tooltip>}
        </header>
        {(policy?.mode !== 'forbidden' || feedback !== '') && <div className="review-v2-feedback">
          <label htmlFor={feedbackId}>{request.feedback.label}{policy?.mode==='forbidden'?' (not allowed)':policy?.mode==='required'?' (required)':' (optional)'}</label>
          <Input.TextArea ref={feedbackInput} id={feedbackId} value={feedback} readOnly={policy?.mode==='forbidden'} onChange={e=>{setFeedback(e.target.value);setSubmitError('');}} autoSize={{minRows:3}}/>
          {feedback!==''&&<Button size="small" type="text" onClick={()=>{setFeedback('');setSubmitError('');}}>Remove feedback draft</Button>}
        </div>}
        {files.length > 0 && <div className="review-v2-upload">{files.map((file,i)=><div key={i}><span>{file.name} ({file.size} bytes)</span> <Button size="small" type="text" aria-label={`Remove ${file.name} (${i+1})`} onClick={()=>{setFiles(previous=>previous.filter((_,index)=>index!==i));if(fileInput.current)fileInput.current.value='';setSubmitError('');}}>Remove</Button></div>)}</div>}
        <div className="review-v2-directions">
          <Button ref={sendButton} type="primary" aria-label={`Send · ${selected.label}`} onClick={()=>void submit(selected.id)}>Send</Button>
          {allowUploads && <>
            <input ref={fileInput} aria-label="Attach notes" id={filesId} type="file" multiple hidden onChange={event=>{
              const attached=Array.from(event.currentTarget.files??[]);
              if(attached.length) setFiles(previous=>[...previous,...attached]);
              event.currentTarget.value='';
              setSubmitError('');
            }} />
            <Tooltip title="Attach notes"><Button type="text" icon={<PaperClipOutlined aria-hidden />} aria-label="Attach files to response" onClick={()=>fileInput.current?.click()} /></Tooltip>
          </>}
          <Button type="text" onClick={()=>{
            setComposerOpen(false);
            requestAnimationFrame(()=>actionButtons.current.get(selected.id)?.focus({preventScroll:true}));
          }}>Back</Button>
        </div>
      </section>}
    </fieldset>
    {!ready&&<Button loading={busy} disabled={phase==='sending'} onClick={()=>void refresh()}>Refresh</Button>}
    </div>
  </section>;
  return <>{response}
    <ReviewV2Settings request={request} drafts={drafts} setDraft={(id,value)=>{setDrafts(p=>({...p,[id]:value}));}} open={settingsOpen} direction={direction} onClose={()=>{setSettingsOpen(false);gear.current?.focus();}}/>
  </>;
}
