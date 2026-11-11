import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Modal } from 'antd';
import { finalDocumentURL, getFinalDocuments, type FinalDocuments } from '@colony2/shared';
import ReviewDocuments, { type ReaderDocument } from './ReviewDocuments';

export default function ReturnedDocuments({tenant,job,refreshKey}: {tenant:string; job:string; refreshKey?:unknown}) {
  const [manifest,setManifest] = useState<FinalDocuments>();
  const [pending,setPending] = useState(false);
  const [error,setError] = useState('');
  const [retry,setRetry] = useState(0);
  const [selected,setSelected] = useState<string>();
  const opener = useRef<HTMLElement>();
  useEffect(() => {
    const controller = new AbortController();
    setManifest(undefined); setSelected(undefined); setPending(false); setError('');
    getFinalDocuments(tenant,job,controller.signal).then(result => {
      if (controller.signal.aborted) return;
      if ('status' in result) setPending(true); else setManifest(result);
    }).catch(e => { if (!controller.signal.aborted) setError(String(e)); });
    return () => controller.abort();
  },[tenant,job,refreshKey,retry]);
  const documents: Record<string, ReaderDocument> = Object.create(null);
  for (const document of manifest?.documents || []) {
    if (document.resolution === 'stored' && document.producer) {
      // Aliases define manifest navigation; producer storage names only identify
      // bytes on the server and must not introduce extra navigable paths.
      documents[document.alias] = {name:document.alias,sizeBytes:document.producer.sizeBytes};
    }
  }
  return <Card size="small" className="conversation-step" aria-label="Returned documents">
    <strong>Returned documents</strong>
    {error ? <Alert type="warning" message="Returned documents unavailable" description={error} action={<Button onClick={() => setRetry(v=>v+1)}>Retry</Button>} />
      : pending ? <p>Final documents pending.</p>
      : !manifest ? <p role="status">Loading returned documents…</p>
      : <>
        <p>{manifest.state === 'empty' ? 'No documents returned (explicit empty manifest).' : manifest.state === 'absent' ? 'Final document declaration not recorded.' : manifest.state === 'null' ? 'Final document declaration is null.' : manifest.state === 'present' ? 'Declared final documents' : `Final document declaration: ${manifest.state}`}</p>
        {manifest.diagnostic && <p>{manifest.diagnostic}</p>}
        <ul>{manifest.documents?.map(document => <li key={document.alias}>
          {documents[document.alias] ? <a href={finalDocumentURL(tenant,job,manifest.manifest_id!,document.alias)} onClick={event => {
            if(event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
            event.preventDefault(); opener.current=event.currentTarget; setSelected(document.alias);
          }}>{document.alias}</a> : <span>{document.alias} — {document.resolution}{document.diagnostic ? `: ${document.diagnostic}` : ''}</span>}
        </li>)}</ul>
      </>}
    <Modal title="Returned document viewer" open={selected !== undefined} footer={null} width="90vw" destroyOnHidden onCancel={() => setSelected(undefined)} afterClose={() => opener.current?.focus({preventScroll:true})}>
      {manifest && selected && <ReviewDocuments projectId={tenant} jobId={job} requestId={`final:${manifest.manifest_id}`} documents={documents} overviewDocumentId={selected} source={{identity:JSON.stringify([tenant,job,manifest.manifest_id!,selected]),url:alias => documents[alias] ? finalDocumentURL(tenant,job,manifest.manifest_id!,alias) : ''}} />}
    </Modal>
  </Card>;
}
