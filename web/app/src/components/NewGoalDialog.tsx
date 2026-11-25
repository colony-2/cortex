import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Form, Modal, Select, Typography, Upload } from 'antd';
import { getCell, submitJob } from '@colony2/shared/api';
import { createRequestId } from '@colony2/shared/cellProtocol';
import type { CortexCell, SubmitRecipeJobRequest } from '@colony2/shared/types';
import { goalTargets } from '../utils/goalTarget';
import { validateGoalAttachments } from '../utils/goalAttachments';
import { useCells } from '../hooks/useCells';
import { useComposerViewport } from '../hooks/useComposerViewport';
import GoalPromptEditor from './GoalPromptEditor';
import { DraftImages, markdownImages } from '../utils/promptImages';
import { hasPromptContent } from '../utils/goalPrompt';

type Props = { projectId: string; filters: string[] | null; onClose: () => void; onCreated: (goalId: string) => void };
export default function NewGoalDialog({ projectId, filters, onClose, onCreated }: Props) {
  const registry = useCells(projectId, 'ACTIVE');
  const [selected, setSelected] = useState<CortexCell>();
  const [open, setOpen] = useState(true);
  const [error, setError] = useState<string>();
  const [startFailed, setStartFailed] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [attachments, setAttachments] = useState<{ id: string; file: File }[]>([]);
  const [fileError, setFileError] = useState<string>();
  const pending = useRef(false);
  const [, imagesChanged] = useState(0);
  const attachmentsRef = useRef(attachments);
  attachmentsRef.current = attachments;
  const [images] = useState(() => new DraftImages(() => imagesChanged(value => value + 1), () => attachmentsRef.current.map(item => item.file)));
  const active = useRef(true);
  const attempt = useRef<{ payload: string; input: SubmitRecipeJobRequest; files: File[] }>();
  const [form] = Form.useForm<{ prompt: string; target?: string }>();
  const prompt = Form.useWatch('prompt', form) || '';
  const liveImages = markdownImages(prompt);
  const pastedFiles = images.files();
  const livePasted = pastedFiles.filter(file => liveImages.includes(file.name));
  const retainedPasted = pastedFiles.filter(file => !liveImages.includes(file.name));
  const bytes = (files: readonly File[]) => files.reduce((total, file) => total + file.size, 0);
  const size = (value: number) => value < 1024 ? `${value} B` : value < 1024 * 1024 ? `${(value / 1024).toFixed(1)} KiB` : `${(value / 1024 / 1024).toFixed(2)} MiB`;
  const viewportStyle = useComposerViewport();
  const initialId = filters?.length === 1 ? filters[0] : undefined;
  useEffect(() => { active.current = true; images.activate(); return () => { active.current = false; images.dispose(); }; }, [images]);
  useEffect(() => {
    let current = true;
    if (initialId) getCell(projectId, initialId).then(cell => { if (current) setSelected(cell); })
      .catch(reason => { if (current) setError(reason.message); });
    return () => { current = false; };
  }, [projectId, initialId]);
  const choices = new Map(registry.cells.map(cell => [cell.id, cell]));
  if (selected) choices.set(selected.id, selected);
  const targets = goalTargets([...choices.values()]);
  const noCells = !registry.loading && !registry.error && targets.length === 0;
  const submit = async () => {
    if (pending.current || registry.loading || registry.error || noCells) return;
    pending.current = true;
    setSubmitting(true); setError(undefined);
    setStartFailed(false);
    try {
      const values = await form.validateFields();
      const files = images.submission(values.prompt, attachments.map(item => item.file));
      if (!active.current) return;
      const payload = JSON.stringify({ ...values, attachments: attachments.map(item => item.id), images: files.slice(attachments.length).map(file => file.name) });
      if (attempt.current?.payload !== payload) {
        const current = await getCell(projectId, values.target!);
        if (current.state !== 'ACTIVE') throw new Error('This cell is retired. Choose an active cell.');
        if (active.current) setSelected(current);
        const cell = current.name;
        attempt.current = { payload, files, input: { cell, inputs: { prompt: values.prompt, type: 'build' } } };
      }
      if (!active.current) return;
      const job = await (attempt.current.files.length ? submitJob(projectId, attempt.current.input, attempt.current.files) : submitJob(projectId, attempt.current.input)).catch(() => undefined);
      if (!job) {
        if (active.current) setStartFailed(true);
        return;
      }
      if (active.current) { images.dispose(); onCreated(job.goal_id); }
    } catch (reason) {
      if (active.current && reason instanceof Error) setError(reason.message);
    } finally { pending.current = false; if (active.current) setSubmitting(false); }
  };
  return <Modal className="new-goal-composer" rootClassName="new-goal-modal-root" width="min(1120px, calc(100vw - 64px))" style={viewportStyle} title="New goal" open={open} onCancel={() => { if (!pending.current) { active.current = false; images.dispose(); setOpen(false); } }} afterClose={onClose} onOk={submit} okText={submitting ? 'Creating goal…' : 'Create goal'} confirmLoading={submitting} cancelButtonProps={{ disabled: submitting }} closable={!submitting} maskClosable={!submitting}
    okButtonProps={{ 'aria-label': submitting ? 'Creating goal…' : 'Create goal', disabled: registry.loading || !!registry.error || noCells }}>
    {registry.error && <Alert type="error" message={registry.error} action={<Button onClick={registry.reload}>Retry cells</Button>} />}
    {error && <Alert type="error" showIcon message={error} />}
    {startFailed && <p role="status">request failed</p>}
    {noCells && <Alert type="info" message="Create a cell in Cells first, then choose it here to start a goal." />}
    <Form form={form} disabled={submitting} layout="vertical" initialValues={{ target: initialId }}>
        <Form.Item label="Cell" name="target" rules={[{ required: true, message: 'Choose a cell' }]}>
          <Select aria-label="Cell" showSearch optionFilterProp="label" loading={registry.loading} options={targets}
            onChange={id => setSelected(choices.get(id))} />
        </Form.Item>
        {selected && <Typography.Paragraph className="goal-target-summary" type="secondary">{selected.name} · {selected.repo}{selected.state === 'RETIRED' && ' · Retired'}</Typography.Paragraph>}
      <Form.Item className="goal-prompt-field" label="Prompt" name="prompt" rules={[{ required: true, validator: (_, value) => hasPromptContent(value || '') ? Promise.resolve() : Promise.reject(new Error('Enter a prompt with text or an image')) }]}><GoalPromptEditor images={images} disabled={submitting} locked={() => pending.current} /></Form.Item>
      {(() => {
        const live = markdownImages(prompt);
        const pendingImages = [...images.entries].filter(([name, item]) => live.includes(name) && item.status !== 'ready');
        return pendingImages.map(([name, item]) => <Alert key={name} type={item.status === 'pending' ? 'info' : 'error'} message={item.status === 'pending' ? 'Checking pasted image…' : `Pasted image unavailable: ${item.error} Remove its reference to continue.`} />);
      })()}
      <Form.Item label="Attachments" extra="Files are provided to the goal as input artifacts. Up to 10 files, 25 MiB total.">
        <div role="status" aria-label="Attachment capacity" className="goal-attachment-capacity">
          <div>{attachments.length + pastedFiles.length} / 10 files · {size(bytes([...attachments.map(item => item.file), ...pastedFiles]))} / 25 MiB used</div>
          {!!pastedFiles.length && <>
            <div>Pasted images in prompt: {livePasted.length} · {size(bytes(livePasted))}. Retained for undo: {retainedPasted.length} · {size(bytes(retainedPasted))}.</div>
            <div>Deleted images still use capacity so Undo can restore them. They are released when you close this draft; only images still in the prompt are submitted.</div>
          </>}
        </div>
        <Upload multiple disabled={submitting} fileList={[]} beforeUpload={(file, batch) => {
          if (pending.current) return false;
          if (file === batch[0]) {
            const next = [...attachments, ...batch.map(item => ({ id: createRequestId(), file: item }))];
            try { validateGoalAttachments([...next.map(item => item.file), ...images.files()]); setAttachments(next); setFileError(undefined); }
            catch (reason) { setFileError((reason as Error).message); }
          }
          return false;
        }}><Button disabled={submitting}>Attach files</Button></Upload>
        {fileError && <Alert type="error" showIcon message={fileError} />}
        <ul>{attachments.map(item => <li key={item.id}>
          {item.file.name} · {item.file.size < 1024 ? `${item.file.size} B` : item.file.size < 1024 * 1024 ? `${(item.file.size / 1024).toFixed(1)} KiB` : `${(item.file.size / 1024 / 1024).toFixed(2)} MiB`}
          <Button type="text" disabled={submitting || liveImages.includes(item.file.name)} title={liveImages.includes(item.file.name) ? 'Remove the image reference from the prompt first.' : undefined} aria-label={`Remove ${item.file.name}`} onClick={() => { if (!pending.current && !liveImages.includes(item.file.name)) { setAttachments(items => items.filter(value => value.id !== item.id)); setFileError(undefined); } }}>Remove</Button>
        </li>)}</ul>
      </Form.Item>
    </Form>
  </Modal>;
}
