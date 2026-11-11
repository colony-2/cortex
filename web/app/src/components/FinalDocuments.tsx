import { useRef, useState } from 'react';
import { Alert, Button, Card, Modal } from 'antd';
import { finalDocumentURL, stringifyJobJSON, type FinalDocuments as Manifest } from '@colony2/shared';
import ReviewDocuments, { type ReaderDocument } from './ReviewDocuments';

type Props = {tenant:string;job:string;terminal:boolean;manifest?:Manifest;error?:string;stale?:boolean;onRetry:()=>void};
export default function FinalDocuments(props: Props) { return <FinalDocumentsReader key={JSON.stringify([props.tenant,props.job,props.manifest?.manifest_id,props.manifest?.attempt_ordinal])} {...props}/>; }
function FinalDocumentsReader({tenant,job,terminal,manifest,error,stale,onRetry}: Props) {
 const [selected,setSelected] = useState<string>();
 const activating = useRef<HTMLElement>();
 if(!terminal)return null;
 const documents:Record<string,ReaderDocument> = Object.create(null);
 for(const doc of manifest?.documents ?? []) if(doc.resolution==='stored' && doc.producer) documents[doc.alias]={name:doc.alias,sizeBytes:doc.producer.sizeBytes};
 const labels: Record<string,string> = {pending:'Returned documents pending',empty:'No returned documents',absent:'Returned document declaration not recorded',null:'Returned document declaration is null',malformed:'Returned document declaration is malformed',unsupported:'Returned document declaration is unsupported'};
 const identity=stringifyJobJSON([tenant,job,manifest]) ?? '';
 return <Card size="small" className="conversation-final-documents" aria-label="Returned documents">
  <strong>Returned documents</strong>
  {stale && <p role="status">Returned documents stale — conversation could not be refreshed.</p>}
  {error ? <Alert type="warning" message="Returned documents unavailable" description={error} action={<Button onClick={onRetry}>Retry</Button>}/> : !manifest ? <p role="status">Loading returned documents…</p> : <>
   {manifest.state!=='present' && <p>{labels[manifest.state] || 'Returned document status unavailable'}</p>}
   {manifest.diagnostic && <p>{manifest.diagnostic}</p>}
   <ul>{manifest.documents?.map(doc=><li key={doc.alias}>
    {documents[doc.alias] && manifest.manifest_id ? <a href={finalDocumentURL(tenant,job,manifest.manifest_id,doc.alias)} onClick={event=>{if(event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey)return;event.preventDefault();activating.current=event.currentTarget;setSelected(doc.alias);}}>{doc.alias}</a> : <span>{doc.alias} — {doc.resolution}</span>}
    {doc.diagnostic && <p>{doc.diagnostic}</p>}
    <details><summary>Document details</summary><pre>{stringifyJobJSON(doc.producer ?? doc.ref,2)}</pre></details>
   </li>)}</ul>
  </>}
  <Modal open={selected!==undefined} title="Returned document viewer" footer={null} width="90vw" destroyOnHidden onCancel={()=>setSelected(undefined)} afterClose={()=>activating.current?.focus({preventScroll:true})}>
   {selected!==undefined && manifest?.manifest_id && <ReviewDocuments projectId={tenant} jobId={job} requestId={identity} documents={documents} overviewDocumentId={selected} documentMetadata={Object.fromEntries(Object.keys(documents).map(alias=>[alias,{title:alias}]))} source={{identity,url:alias=>documents[alias]?finalDocumentURL(tenant,job,manifest.manifest_id!,alias):''}}/>}
  </Modal>
 </Card>;
}
