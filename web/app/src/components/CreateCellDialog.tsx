import { useEffect, useRef, useState } from 'react';
import { Alert, Form, Input, Modal, Typography } from 'antd';
import { createCell } from '@colony2/shared/api';
import { createRequestId } from '@colony2/shared/cellProtocol';
import type { CortexCell } from '@colony2/shared/types';
import { cellNameRules } from '../utils/goalTarget';

function metadataValue(value?: string): Record<string, unknown> {
  const metadata = value?.trim() ? JSON.parse(value) : {};
  if (!metadata || Array.isArray(metadata) || typeof metadata !== 'object') throw new Error('Metadata must be a JSON object');
  if (Object.keys(metadata).length > 64 || Object.keys(metadata).some(key => !key || [...key].length > 128)
    || new TextEncoder().encode(JSON.stringify(metadata)).length > 16384) throw new Error('Metadata allows 64 keys (1–128 characters each) and 16 KiB of JSON');
  return metadata;
}

export default function CreateCellDialog({ projectId, onClose, onCreated }: {
  projectId: string; onClose: () => void; onCreated: (cell: CortexCell) => void;
}) {
  const [form] = Form.useForm<{ name: string; repo: string; description?: string; metadata?: string }>();
  const [open, setOpen] = useState(true);
  const [error, setError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const pending = useRef(false);
  const active = useRef(true);
  const attempt = useRef<{ payload: string; requestId: string }>();
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  const submit = async () => {
    if (pending.current) return;
    pending.current = true;
    try {
      const values = await form.validateFields();
      if (!active.current) return;
      setSubmitting(true); setError(undefined);
      const input = { name: values.name, repo: values.repo.trim(), description: values.description || '', metadata: metadataValue(values.metadata) };
      const payload = JSON.stringify(input);
      if (attempt.current?.payload !== payload) attempt.current = { payload, requestId: createRequestId() };
      const cell = await createCell(projectId, { ...input, requestId: attempt.current.requestId });
      if (active.current) onCreated(cell);
    } catch (reason) {
      if (active.current && reason instanceof Error) setError(reason.message);
    } finally {
      pending.current = false;
      if (active.current) setSubmitting(false);
    }
  };
  return <Modal title="Create cell" open={open} onCancel={() => { active.current = false; setOpen(false); }} afterClose={onClose} onOk={submit} okText="Create cell" confirmLoading={submitting} cancelButtonProps={{ disabled: submitting }} closable={!submitting} maskClosable={!submitting}>
    <Typography.Paragraph type="secondary">Register a cell and its repository in JobDB. This does not create a Git repository or submit a job.</Typography.Paragraph>
    {error && <Alert type="error" showIcon message={error} />}
    <Form form={form} layout="vertical">
      <Form.Item label="Name" name="name" rules={cellNameRules}><Input autoFocus /></Form.Item>
      <Form.Item label="Repository" name="repo" rules={[{ required: true, whitespace: true, message: 'Enter a repository locator' }]}><Input placeholder="github.com/organization/repository" /></Form.Item>
      <Form.Item label="Description" name="description" rules={[{ max: 2048 }]}><Input.TextArea rows={3} /></Form.Item>
      <Form.Item label="Metadata (optional JSON object)" name="metadata" rules={[{ validator: (_, value) => {
        try { metadataValue(value); return Promise.resolve(); } catch (reason) { return Promise.reject(reason); }
      } }]}><Input.TextArea rows={3} placeholder="{}" /></Form.Item>
    </Form>
  </Modal>;
}
