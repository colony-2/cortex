import { discardMessageDraft, getMessageDraft, messageDraftScope, saveMessageDraft } from '../hooks/useMessageDrafts';
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Alert, Button, Input, Select } from 'antd';
import { CloseOutlined, DeleteOutlined, FullscreenExitOutlined, FullscreenOutlined, MinusOutlined, PaperClipOutlined, PlusOutlined } from '@ant-design/icons';
import { getCell, submitJob } from '@colony2/shared/api';
import { createRequestId } from '@colony2/shared/cellProtocol';
import { useJobInbox } from '../hooks/useJobInbox';
import { composeJobPrompt } from '../utils/jobInbox';
import GoalPromptEditor from './GoalPromptEditor';
import { DraftImages, markdownImages } from '../utils/promptImages';
import { hasPromptContent } from '../utils/goalPrompt';
import { useCompact } from '../hooks/useCompact';
import { useComposerViewport } from '../hooks/useComposerViewport';
import { validateGoalAttachments } from '../utils/goalAttachments';

export default function ComposeJob({ open, onClose, id, onRemove, active = true, onActivate, onSendingChange }: {
  open: boolean;
  onClose: () => void;
  id?: string;
  onRemove?: () => void;
  active?: boolean;
  onActivate?: () => void;
  onSendingChange?: (sending: boolean) => void;
}) {
  const inbox = useJobInbox(), navigate = useNavigate();
  const reloadRecipients = inbox.registry.reload;
  // Registry records may have changed while the shell and draft stayed mounted.
  useEffect(() => { if (open) reloadRecipients(); }, [open, reloadRecipients]);
  const fieldId = useId();
  const scope = messageDraftScope(inbox.tenant);
  const [saved] = useState(() => id ? getMessageDraft(inbox.tenant, id) : undefined);
  const [recipient, setRecipient] = useState(saved?.recipient);
  const [subject, setSubject] = useState(saved?.subject ?? ''), [body, setBody] = useState(saved?.body ?? '');
  const [files, setFiles] = useState<File[]>(saved?.files ?? []);
  const filesRef = useRef(files); filesRef.current = files;
  const [imageVersion, imagesChanged] = useState(0);
  const [images] = useState(() => new DraftImages(() => imagesChanged(value => value + 1), () => filesRef.current));
  const [draftRevision, setDraftRevision] = useState(0);
  const [bodyError, setBodyError] = useState(false);
  const viewportStyle = useComposerViewport();
  const compact = useCompact();
  useLayoutEffect(() => {
    images.activate();
    images.restore(saved?.pasted ?? []);
    return () => images.dispose();
  }, [images, saved]);
  const pastedFiles = images.files(), liveImages = markdownImages(body);
  const bytes = (items: readonly File[]) => items.reduce((sum, file) => sum + file.size, 0);
  const size = (n: number) => n < 1024 ? `${n} B` : n < 1024 * 1024 ? `${(n / 1024).toFixed(1)} KiB` : `${(n / 1024 / 1024).toFixed(2)} MiB`;
  const livePasted = pastedFiles.filter(file => liveImages.includes(file.name));
  const retainedPasted = pastedFiles.filter(file => !liveImages.includes(file.name));
  const [minimized, setMinimized] = useState(false), [busy, setBusy] = useState(false);
  const collapsed = minimized || (compact && !active);
  const [maximized, setMaximized] = useState(false);
  const [error, setError] = useState<string>();
  const sending = useRef(false), attempt = useRef(saved?.attempt);
  const fileInput = useRef<HTMLInputElement>(null);
  const removed = useRef(false);
  const save = () => {
    if (id && !removed.current) saveMessageDraft(scope, id, {
      recipient, subject, body, files, pasted: images.files(), attempt: attempt.current,
    });
  };
  useLayoutEffect(() => { save(); }, [id, scope, recipient, subject, body, files, imageVersion]);
  const remove = () => {
    removed.current = true;
    if (id) discardMessageDraft(scope, id);
    onRemove?.();
  };
  const reset = () => { images.dispose(); images.activate(); setDraftRevision(value => value + 1); setBodyError(false); setRecipient(undefined); setSubject(''); setBody(''); setFiles([]); setError(undefined); attempt.current = undefined; if (fileInput.current) fileInput.current.value = ''; };
  const send = async () => {
    if (sending.current) return;
    sending.current = true; onSendingChange?.(true); setBusy(true); setError(undefined);
    try {
      setBodyError(false);
      if (!hasPromptContent(body)) { setBodyError(true); throw new Error('Enter a message with text or an image'); }
      const prompt = composeJobPrompt(subject, body);
      if (!recipient) throw new Error('Choose a registered cell');
      const submissionFiles = images.submission(body, files);
      const cell = await getCell(inbox.tenant, recipient);
      if (cell.state !== 'ACTIVE') throw new Error('This cell is retired. Choose an active cell.');
      // Keep the identity after a lost response; never retry with a fresh job ID.
      attempt.current ??= createRequestId();
      save();
      const job = await submitJob(inbox.tenant, { job_id: attempt.current, cell: cell.name, inputs: { prompt, type: 'build' } }, submissionFiles);
      if (onRemove) remove(); else { reset(); onClose(); }
      inbox.refresh();
      navigate(`/project/${encodeURIComponent(job.tenant_id)}/jobs/${encodeURIComponent(job.job_id)}/story`);
    } catch (reason) { setError(`${reason instanceof Error ? reason.message : String(reason)}${attempt.current ? ' Check All if the outcome is unknown. Retrying keeps the same job ID.' : ''}`); }
    finally { sending.current = false; onSendingChange?.(false); setBusy(false); }
  };
  const close = () => {
    if (sending.current) return;
    setMaximized(false);
    if (onRemove && !recipient && !subject.trim() && !body.trim() && !files.length && !pastedFiles.length && !attempt.current) remove();
    else onClose();
  };
  const toggleMinimized = () => {
    if (collapsed) {
      onActivate?.();
      setMinimized(false);
    } else {
      setMinimized(true);
      setMaximized(false);
    }
  };
  return <section hidden={!open} className={`inbox-composer${minimized ? ' minimized' : ''}${maximized && active ? ' maximized' : ''}${active ? ' active' : ''}`} aria-label="New message" style={viewportStyle} onKeyDown={event => {
    if (event.key === 'Escape' && !event.defaultPrevented) {
      event.stopPropagation();
      if (maximized) setMaximized(false); else close();
    }
  }}>
    <header>
      <strong title={subject || undefined}>{subject.trim() || 'New message'}</strong>
      <Button type="text" aria-label={collapsed ? 'Expand composer' : 'Minimize composer'} aria-expanded={!collapsed} icon={collapsed ? <PlusOutlined /> : <MinusOutlined />} onClick={toggleMinimized} />
      <Button type="text" aria-label={maximized ? 'Restore composer size' : 'Maximize composer'} icon={maximized ? <FullscreenExitOutlined /> : <FullscreenOutlined />} onClick={() => { onActivate?.(); setMinimized(false); setMaximized(value => !value); }} />
      <Button type="text" aria-label="Close composer" disabled={busy} icon={<CloseOutlined />} onClick={close} />
    </header>
    <form hidden={collapsed} onPointerDown={onActivate} onSubmit={event => { event.preventDefault(); void send(); }}>
      <div className="inbox-compose-body">
      <label htmlFor={`${fieldId}-recipient`}>To</label><Select virtual={false} id={`${fieldId}-recipient`} aria-label="Recipient cell" showSearch optionFilterProp="label" value={recipient} disabled={busy} loading={inbox.registry.loading} options={inbox.registry.cells.filter(cell => cell.state === 'ACTIVE').map(cell => ({ value: cell.id, label: cell.name }))} onChange={setRecipient} />
      {inbox.registry.error && <Alert type="error" message="Recipients unavailable" action={<Button onClick={inbox.registry.reload}>Retry cells</Button>} />}
      <label htmlFor={`${fieldId}-subject`}>Subject (optional)</label><Input id={`${fieldId}-subject`} value={subject} disabled={busy} onChange={event => setSubject(event.target.value)} />
      <label htmlFor={`${fieldId}-message`}>Message</label><GoalPromptEditor key={draftRevision} id={`${fieldId}-message`} aria-label="Message" aria-invalid={bodyError || undefined} aria-describedby={bodyError ? `${fieldId}-error` : undefined} value={body} disabled={busy} images={images} locked={() => sending.current} onChange={setBody} />
      {error && <div id={`${fieldId}-error`}><Alert type="error" message={error} /></div>}
      {[...images.entries].filter(([name, item]) => liveImages.includes(name) && item.status !== 'ready').map(([name, item]) => <Alert key={name} type={item.status === 'pending' ? 'info' : 'error'} message={item.status === 'pending' ? 'Checking pasted image…' : `Pasted image unavailable: ${item.error} Remove its reference to continue.`} />)}
      <div role="status" aria-label="Attachment capacity" className="goal-attachment-capacity">
        <div>{files.length + pastedFiles.length} / 10 files · {size(bytes([...files, ...pastedFiles]))} / 25 MiB used</div>
        {!!pastedFiles.length && <><div>Pasted images in prompt: {livePasted.length} · {size(bytes(livePasted))}. Retained for undo: {retainedPasted.length} · {size(bytes(retainedPasted))}.</div><div>Deleted images use capacity for Undo. Discard releases them; only images still in the message are submitted.</div></>}
      </div>
      <input ref={fileInput} hidden type="file" multiple disabled={busy} aria-label="Job attachments" onChange={event => {
        if (sending.current) return;
        const next = [...files, ...Array.from(event.target.files ?? [])];
        try { validateGoalAttachments([...next, ...images.files()]); setFiles(next); setError(undefined); } catch (reason) { setError((reason as Error).message); }
        event.target.value = '';
      }} />
      {files.length > 0 && <ul>{files.map((file, index) => <li key={index}>{file.name} <Button size="small" aria-label={`Remove ${file.name}`} disabled={busy || liveImages.includes(file.name)} onClick={() => setFiles(previous => previous.filter((_, i) => i !== index))}>Remove</Button></li>)}</ul>}
      </div>
      <footer><Button type="primary" htmlType="submit" aria-label="Send" aria-busy={busy} loading={busy} disabled={busy || !!inbox.registry.error}>Send</Button><Button icon={<PaperClipOutlined />} aria-label="Attach files" disabled={busy} onClick={() => fileInput.current?.click()} /><span /><Button type="text" danger icon={<DeleteOutlined />} aria-label="Discard draft" disabled={busy} onClick={() => { if (onRemove) remove(); else { reset(); onClose(); } }} /></footer>
    </form>
  </section>;
}
