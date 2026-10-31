import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Form, Input, Modal, Radio, Select, Spin, Typography } from 'antd';
import { listCells, submitJob } from '@colony2/shared/api';
import type { CortexCell, RecipeJobSummary } from '@colony2/shared/types';
import { goalTargets, initialGoalTarget, normalizeGoalRepository, type GoalTarget } from '../utils/goalTarget';

type Props = {
  projectId: string; jobs: RecipeJobSummary[]; filters: string[] | null;
  onClose: () => void; onCreated: (goalId: string) => void;
};

// Mounted for each opening. Targets are snapshots, including a job-only choice
// selected before its repository becomes available from the catalogue.
export default function NewGoalDialog({ projectId, jobs, filters, onClose, onCreated }: Props) {
  const [open, setOpen] = useState(true);
  const [catalogue, setCatalogue] = useState<CortexCell[]>([]);
  const choices = useMemo(() => goalTargets(catalogue, jobs, filters || []), [catalogue, jobs, filters]);
  const [target, setTarget] = useState<GoalTarget>(() => initialGoalTarget(choices, filters));
  const [mode, setMode] = useState('known');
  const [repository, setRepository] = useState('');
  const [catalogueError, setCatalogueError] = useState<string>();
  const [submitError, setSubmitError] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [retry, setRetry] = useState(0);
  const active = useRef(false);
  const pending = useRef(false);
  const [form] = Form.useForm<{ prompt: string; repository?: string; target?: string }>();
  useEffect(() => {
    active.current = true;
    return () => { active.current = false; };
  }, []);
  useEffect(() => {
    let current = true;
    setLoading(true); setCatalogueError(undefined);
    listCells(projectId).then(cells => { if (current) setCatalogue(cells); })
      .catch(error => { if (current) setCatalogueError(error instanceof Error ? error.message : 'Could not load cells'); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [projectId, retry]);
  const options = choices.some(choice => choice.value === target.value) ? choices : [target, ...choices];
  let canonical: string | undefined;
  try { canonical = normalizeGoalRepository(repository); } catch { /* Validation is shown on submission. */ }
  const submit = async () => {
    if (pending.current) return;
    pending.current = true;
    try {
      const values = await form.validateFields();
      if (!active.current) return;
      setSubmitting(true); setSubmitError(undefined);
      const cell = mode === 'repository' ? normalizeGoalRepository(values.repository || '') : target.cell;
      const job = await submitJob(projectId, { cell, inputs: { prompt: values.prompt, type: 'build' } });
      if (active.current) onCreated(job.goal_id!);
    } catch (error) {
      if (active.current && error instanceof Error) setSubmitError(error.message);
    } finally {
      pending.current = false;
      if (active.current) setSubmitting(false);
    }
  };
  return <Modal title="New goal" open={open} onCancel={() => { active.current = false; setOpen(false); }} afterClose={onClose} onOk={submit} okText="Create goal" okButtonProps={{ loading: submitting }}>
    {catalogueError && <Alert type="warning" showIcon message="Could not load known cells" description={<>{catalogueError} <Button onClick={() => setRetry(value => value + 1)}>Retry cells</Button></>} />}
    {loading && <div role="status"><Spin size="small" /> Loading known cells…</div>}
    {submitError && <Alert type="error" showIcon message={submitError} />}
    <Form form={form} layout="vertical" initialValues={{ target: target.value }}>
      <Form.Item label="Target mode"><Radio.Group value={mode} onChange={event => setMode(event.target.value)} options={[{ label: 'Known cell', value: 'known' }, { label: 'Repository', value: 'repository' }]} /></Form.Item>
      {mode === 'known' ? <Form.Item label="Cell" name="target" rules={[{ required: true, message: 'Choose a cell' }]}>
        <Select aria-label="Cell" showSearch optionFilterProp="label" options={options} onChange={value => setTarget(options.find(choice => choice.value === value)!)} />
      </Form.Item> : <Form.Item label="Repository" name="repository" rules={[{ validator: (_, value) => {
        try { normalizeGoalRepository(value || ''); return Promise.resolve(); } catch (error) { return Promise.reject(error); }
      } }]}>
        <Input aria-label="Repository" placeholder="github/colony-2/name" onChange={event => setRepository(event.target.value)} />
      </Form.Item>}
      <Typography.Paragraph className="goal-target-summary" type="secondary">Target: {mode === 'known' ? target.cell || 'This cell (self)' : canonical || 'Enter a repository'}</Typography.Paragraph>
      <Form.Item label="Prompt" name="prompt" rules={[{ required: true, whitespace: true, message: 'Enter a prompt' }]}><Input.TextArea rows={6} /></Form.Item>
    </Form>
  </Modal>;
}
