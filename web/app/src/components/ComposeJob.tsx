import { discardMessageDraft, getMessageDraft, messageDraftScope, saveMessageDraft } from '../hooks/useMessageDrafts';
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { Alert, Button, Input, Select, Tooltip, message, type InputRef } from 'antd';
import { CloseOutlined, DeleteOutlined, PaperClipOutlined } from '@ant-design/icons';
import { getCell, submitJob } from '@colony2/shared/api';
import { createRequestId } from '@colony2/shared/cellProtocol';
import { useJobInbox } from '../hooks/useJobInbox';
import { composeJobPrompt } from '../utils/jobInbox';
import GoalPromptEditor from './GoalPromptEditor';
import { DraftImages, markdownImages } from '../utils/promptImages';
import { hasPromptContent } from '../utils/goalPrompt';
import ComposerWindow from './ComposerWindow';
import { validateAttachmentNames } from '../utils/goalAttachments';

export default function ComposeJob({ open, onClose, id, onRemove, active = true, onActivate, onSendingChange }: {
  open: boolean;
  onClose: () => void;
  id?: string;
  onRemove?: () => void;
  active?: boolean;
  onActivate?: () => void;
  onSendingChange?: (sending: boolean) => void;
}) {
  const inbox = useJobInbox();
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
  const [images] = useState(() => new DraftImages(() => imagesChanged(value => value + 1), () => filesRef.current, {
    validate: validateAttachmentNames,
    allowWithoutPreview: true,
  }));
  const [draftRevision, setDraftRevision] = useState(0);
  const [bodyError, setBodyError] = useState(false);
  useLayoutEffect(() => {
    images.activate();
    images.restore(saved?.pasted ?? []);
    return () => images.dispose();
  }, [images, saved]);
  const pastedFiles = images.files(), liveImages = markdownImages(body);
  const [busy, setBusy] = useState(false);
  const [formattingOpen, setFormattingOpen] = useState(false);
  const [formattingContainer, setFormattingContainer] = useState<HTMLDivElement | null>(null);
  const [error, setError] = useState<string>();
  const sending = useRef(false), attempt = useRef(saved?.attempt);
  const fileInput = useRef<HTMLInputElement>(null);
  const subjectInput = useRef<InputRef>(null);
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
  const reset = () => { images.dispose(); images.activate(); setDraftRevision(value => value + 1); setBodyError(false); setRecipient(undefined); setSubject(''); setBody(''); setFiles([]); setError(undefined); setFormattingOpen(false); attempt.current = undefined; if (fileInput.current) fileInput.current.value = ''; };
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
      await submitJob(inbox.tenant, { job_id: attempt.current, cell: cell.name, inputs: { prompt, type: 'build' } }, submissionFiles);
      if (onRemove) remove(); else { reset(); onClose(); }
      inbox.refresh();
      message.success('Message sent');
    } catch (reason) { setError(`${reason instanceof Error ? reason.message : String(reason)}${attempt.current ? ' Check All if the outcome is unknown. Retrying keeps the same job ID.' : ''}`); }
    finally { sending.current = false; onSendingChange?.(false); setBusy(false); }
  };
  const close = () => {
    if (sending.current) return;
    if (onRemove && !recipient && !subject.trim() && !body.trim() && !files.length && !pastedFiles.length && !attempt.current) remove();
    else onClose();
  };
  const focusSubject = () => {
    // The Select schedules its own focus restoration when a popup option is clicked.
    window.setTimeout(() => subjectInput.current?.focus(), 0);
  };
  return <ComposerWindow open={open} label="New message" title={subject.trim() || 'New message'} active={active} busy={busy} onActivate={onActivate} onClose={close}>
    {collapsed => <form hidden={collapsed} onPointerDown={onActivate} onSubmit={event => { event.preventDefault(); void send(); }} onKeyDownCapture={event => {
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.nativeEvent.isComposing) {
        event.preventDefault();
        event.stopPropagation();
        if (!inbox.registry.error) void send();
      }
    }}>
      <div className="inbox-compose-fields">
        <Select virtual={false} id={`${fieldId}-recipient`} aria-label="Recipient cell" placeholder="Cell" variant="borderless" autoFocus={open && active}
          showSearch optionFilterProp="label" value={recipient} disabled={busy} loading={inbox.registry.loading} options={inbox.registry.cells.filter(cell => cell.state === 'ACTIVE').map(cell => ({ value: cell.id, label: cell.name }))}
          onChange={setRecipient} onSelect={focusSubject} />
        {inbox.registry.error && <Alert type="error" message="Recipients unavailable" action={<Button onClick={inbox.registry.reload}>Retry cells</Button>} />}
        <Input ref={subjectInput} id={`${fieldId}-subject`} aria-label="Subject" placeholder="Subject" variant="borderless" value={subject} disabled={busy} onChange={event => setSubject(event.target.value)} />
      </div>
      <div className="inbox-compose-body">
        <GoalPromptEditor key={draftRevision} id={`${fieldId}-message`} aria-label="Message" aria-invalid={bodyError || undefined} aria-describedby={bodyError ? `${fieldId}-error` : undefined} value={body} disabled={busy} images={images} locked={() => sending.current} onChange={setBody}
          formatting={{ visible: formattingOpen, container: formattingContainer }} />
        {error && <div id={`${fieldId}-error`}><Alert type="error" message={error} /></div>}
        {[...images.entries].filter(([name, item]) => liveImages.includes(name) && item.status !== 'ready').map(([name, item]) => <Alert key={name} type={item.status === 'pending' ? 'info' : 'error'} message={item.status === 'pending' ? 'Checking pasted image…' : `Pasted image unavailable: ${item.error} Remove its reference to continue.`} />)}
        {files.length > 0 && <ul className="inbox-compose-attachments" aria-label="Attachments">{files.map((file, index) => <li key={file.name}><PaperClipOutlined aria-hidden /><span>{file.name}</span><Button size="small" type="text" icon={<CloseOutlined aria-hidden />} aria-label={`Remove ${file.name}`} disabled={busy || liveImages.includes(file.name)} onClick={() => setFiles(previous => previous.filter((_, i) => i !== index))} /></li>)}</ul>}
      </div>
      <footer>
        <div id={`${fieldId}-formatting`} ref={setFormattingContainer} hidden={!formattingOpen} />
        <div className="inbox-compose-actions">
          <Tooltip title="Send (Ctrl or ⌘ + Enter)"><Button type="primary" shape="round" htmlType="submit" aria-label="Send" aria-keyshortcuts="Control+Enter Meta+Enter" aria-busy={busy} loading={busy} disabled={busy || !!inbox.registry.error}>Send</Button></Tooltip>
          <Tooltip title="Formatting options"><Button type="text" aria-label="Formatting options" aria-expanded={formattingOpen} aria-controls={`${fieldId}-formatting`} disabled={busy} onMouseDown={event => event.preventDefault()} onClick={() => setFormattingOpen(value => !value)}>Aa</Button></Tooltip>
          <input ref={fileInput} hidden type="file" multiple disabled={busy} aria-label="Job attachments" onChange={event => {
            if (sending.current) return;
            const next = [...files, ...Array.from(event.target.files ?? [])];
            try { validateAttachmentNames([...next, ...images.files()]); setFiles(next); setError(undefined); } catch (reason) { setError((reason as Error).message); }
            event.target.value = '';
          }} />
          <Tooltip title="Attach files"><Button type="text" icon={<PaperClipOutlined aria-hidden />} aria-label="Attach files" disabled={busy} onClick={() => fileInput.current?.click()} /></Tooltip>
          <Tooltip title="Discard draft"><Button className="inbox-compose-discard" type="text" icon={<DeleteOutlined aria-hidden />} aria-label="Discard draft" disabled={busy} onClick={() => { if (onRemove) remove(); else { reset(); onClose(); } }} /></Tooltip>
        </div>
      </footer>
    </form>}
  </ComposerWindow>;
}
