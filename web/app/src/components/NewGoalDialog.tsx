import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Form, Input, Modal, Radio, Select, Typography, Upload } from 'antd';
import { createCell, getCell, submitJob } from '@colony2/shared/api';
import { createRequestId } from '@colony2/shared/cellProtocol';
import type { CortexCell, SubmitRecipeJobRequest } from '@colony2/shared/types';
import { cellNameRules, goalTargets } from '../utils/goalTarget';
import { validateGoalAttachments } from '../utils/goalAttachments';
import { useCells } from '../hooks/useCells';

type Props = { projectId: string; filters: string[] | null; onClose: () => void; onCreated: (goalId: string) => void };
export default function NewGoalDialog({ projectId, filters, onClose, onCreated }: Props) {
  const registry = useCells(projectId, 'ACTIVE');
  const [selected, setSelected] = useState<CortexCell>();
  const [mode, setMode] = useState('known');
  const [open, setOpen] = useState(true);
  const [error, setError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const [registered, setRegistered] = useState<CortexCell>();
  const [attachments, setAttachments] = useState<{ id: string; file: File }[]>([]);
  const [fileError, setFileError] = useState<string>();
  const pending = useRef(false);
  const active = useRef(true);
  const registration = useRef<{ payload: string; requestId: string; cell?: CortexCell }>();
  const attempt = useRef<{ payload: string; input: SubmitRecipeJobRequest; files: File[] }>();
  const [form] = Form.useForm<{ prompt: string; name?: string; repo?: string; target?: string }>();
  const initialId = filters?.length === 1 ? filters[0] : undefined;
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  useEffect(() => {
    let current = true;
    if (initialId) getCell(projectId, initialId).then(cell => { if (current) setSelected(cell); })
      .catch(reason => { if (current) setError(reason.message); });
    return () => { current = false; };
  }, [projectId, initialId]);
  const choices = new Map(registry.cells.map(cell => [cell.id, cell]));
  if (selected) choices.set(selected.id, selected);
  const submit = async () => {
    if (pending.current) return;
    pending.current = true;
    try {
      const values = await form.validateFields();
      validateGoalAttachments(attachments.map(item => item.file));
      if (!active.current) return;
      setSubmitting(true); setError(undefined);
      const payload = JSON.stringify({ mode, ...values, attachments: attachments.map(item => item.id) });
      if (attempt.current?.payload !== payload) {
        let current: CortexCell;
        if (mode === 'known') {
          current = await getCell(projectId, values.target!);
        } else {
          const input = { name: values.name!, repo: values.repo!.trim() };
          const cellPayload = JSON.stringify(input);
          if (registration.current?.payload !== cellPayload) {
            registration.current = { payload: cellPayload, requestId: createRequestId() };
          }
          const creation = registration.current;
          if (!creation.cell) {
            creation.cell = await createCell(projectId, { ...input, requestId: creation.requestId });
            if (active.current) { setRegistered(creation.cell); registry.reload(); }
          }
          if (!active.current) return;
          current = await getCell(projectId, creation.cell.id);
        }
        if (current.state !== 'ACTIVE') throw new Error('This cell is retired. Choose an active cell.');
        if (active.current) setSelected(current);
        const cell = current.name;
        const repo = mode === 'new' ? registration.current!.cell!.repo : undefined;
        attempt.current = { payload, files: attachments.map(item => item.file), input: { cell, ...(repo ? { repo } : {}), job_id: createRequestId(), inputs: { prompt: values.prompt, type: 'build' } } };
      }
      if (!active.current) return;
      const job = await (attempt.current.files.length ? submitJob(projectId, attempt.current.input, attempt.current.files) : submitJob(projectId, attempt.current.input));
      if (active.current) onCreated(job.goal_id);
    } catch (reason) {
      if (active.current && reason instanceof Error) setError(reason.message);
    } finally { pending.current = false; if (active.current) setSubmitting(false); }
  };
  return <Modal title="New goal" open={open} onCancel={() => { if (!pending.current) { active.current = false; setOpen(false); } }} afterClose={onClose} onOk={submit} okText={submitting ? 'Creating goal…' : 'Create goal'} confirmLoading={submitting} cancelButtonProps={{ disabled: submitting }} closable={!submitting} maskClosable={!submitting}
    okButtonProps={{ disabled: mode === 'known' && !!registry.error }}>
    {registry.error && <Alert type="error" message={registry.error} action={<Button onClick={registry.reload}>Retry cells</Button>} />}
    {error && <Alert type="error" showIcon message={error} />}
    {registered && <Alert type="info" message={`Cell “${registered.name}” is registered. If goal submission fails, retry here or choose this cell later.`} />}
    <Form form={form} disabled={submitting} layout="vertical" initialValues={{ target: initialId }}>
      <Form.Item label="Target mode"><Radio.Group value={mode} onChange={event => setMode(event.target.value)} options={[{ label: 'Existing cell', value: 'known' }, { label: 'New cell', value: 'new' }]} /></Form.Item>
      {mode === 'known' ? <>
        <Form.Item label="Cell" name="target" rules={[{ required: true, message: 'Choose a cell' }]}>
          <Select aria-label="Cell" showSearch optionFilterProp="label" loading={registry.loading} options={goalTargets([...choices.values()])}
            onChange={id => setSelected(choices.get(id))} />
        </Form.Item>
        {selected && <Typography.Paragraph className="goal-target-summary" type="secondary">{selected.name} · {selected.repo}{selected.state === 'RETIRED' && ' · Retired'}</Typography.Paragraph>}
      </> : <>
        <Typography.Paragraph type="secondary">The cell is registered first, then the goal is submitted. The cell remains available if submission fails.</Typography.Paragraph>
        <Form.Item label="Name" name="name" rules={cellNameRules}><Input /></Form.Item>
        <Form.Item label="Repository" name="repo" rules={[{ required: true, whitespace: true, message: 'Enter a repository locator' }]}><Input placeholder="github.com/organization/repository" /></Form.Item>
      </>}
      <Form.Item label="Prompt" name="prompt" rules={[{ required: true, whitespace: true, message: 'Enter a prompt' }]}><Input.TextArea rows={6} /></Form.Item>
      <Form.Item label="Attachments" extra="Files are provided to the goal as input artifacts. Up to 10 files, 25 MiB total.">
        <Upload multiple disabled={submitting} fileList={[]} beforeUpload={(file, batch) => {
          if (pending.current) return false;
          if (file === batch[0]) {
            const next = [...attachments, ...batch.map(item => ({ id: createRequestId(), file: item }))];
            try { validateGoalAttachments(next.map(item => item.file)); setAttachments(next); setFileError(undefined); }
            catch (reason) { setFileError((reason as Error).message); }
          }
          return false;
        }}><Button disabled={submitting}>Attach files</Button></Upload>
        {fileError && <Alert type="error" showIcon message={fileError} />}
        <ul>{attachments.map(item => <li key={item.id}>
          {item.file.name} · {item.file.size < 1024 ? `${item.file.size} B` : item.file.size < 1024 * 1024 ? `${(item.file.size / 1024).toFixed(1)} KiB` : `${(item.file.size / 1024 / 1024).toFixed(2)} MiB`}
          <Button type="text" disabled={submitting} aria-label={`Remove ${item.file.name}`} onClick={() => { if (!pending.current) { setAttachments(items => items.filter(value => value.id !== item.id)); setFileError(undefined); } }}>Remove</Button>
        </li>)}</ul>
      </Form.Item>
    </Form>
  </Modal>;
}
