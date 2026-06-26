import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Button,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { listCells, listJobs, submitJob } from '@colony2/shared/api';
import type {
  CortexCell,
  RecipeJob,
  RecipeJobStatus,
  SubmitRecipeJobRequest,
} from '@colony2/shared/types';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

const { Text, Title } = Typography;

const activeStatuses: RecipeJobStatus[] = [
  'READY',
  'PENDING_JOBS',
  'AWAITING_FUTURE',
  'ACTIVE',
  'CRASH_CONCERN',
];

const statusColors: Record<string, string> = {
  READY: 'blue',
  PENDING_JOBS: 'gold',
  AWAITING_FUTURE: 'cyan',
  ACTIVE: 'purple',
  CRASH_CONCERN: 'orange',
  EXPIRED: 'default',
  CANCELLED: 'default',
  COMPLETED: 'green',
};

interface JobsListPageProps {
  projectId: string;
}

interface SubmitJobForm {
  cell?: string;
  recipe?: string;
  job_id?: string;
  inputs?: string;
}

export default function JobsListPage({ projectId }: JobsListPageProps) {
  const [jobs, setJobs] = useState<RecipeJob[]>([]);
  const [cells, setCells] = useState<CortexCell[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [submitOpen, setSubmitOpen] = useState(false);
  const [filters, setFilters] = useState<{ status?: RecipeJobStatus[]; cell?: string }>({
    status: activeStatuses,
  });
  const [form] = Form.useForm<SubmitJobForm>();

  const load = async () => {
    setLoading(true);
    try {
      const [cellData, jobData] = await Promise.all([
        listCells(projectId),
        listJobs(projectId, {
          status: filters.status,
          cell: filters.cell,
          pageSize: 100,
        }),
      ]);
      setCells(cellData);
      setJobs(jobData.jobs || []);
    } catch (error) {
      console.error('Failed to load jobs', error);
      message.error(error instanceof Error ? error.message : 'Failed to load jobs');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, [projectId, filters.status, filters.cell]);

  const cellOptions = useMemo(
    () =>
      cells.map((cell) => ({
        value: cell.name || cell.repository_source,
        label: `${cell.name} (${cell.kind})`,
      })),
    [cells],
  );

  const submit = async () => {
    const values = await form.validateFields();
    let inputs: Record<string, unknown> | undefined;
    const rawInputs = values.inputs?.trim();
    if (rawInputs) {
      try {
        const parsed = JSON.parse(rawInputs);
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          throw new Error('Inputs must be a JSON object');
        }
        inputs = parsed as Record<string, unknown>;
      } catch (error) {
        message.error(error instanceof Error ? error.message : 'Invalid JSON inputs');
        return;
      }
    }

    const request: SubmitRecipeJobRequest = {
      cell: values.cell,
      recipe: values.recipe || undefined,
      job_id: values.job_id || undefined,
      inputs,
    };

    setSubmitting(true);
    try {
      const job = await submitJob(projectId, request);
      message.success(`Submitted job ${job.job_id}`);
      setSubmitOpen(false);
      form.resetFields();
      await load();
    } catch (error) {
      console.error('Failed to submit job', error);
      message.error(error instanceof Error ? error.message : 'Failed to submit job');
    } finally {
      setSubmitting(false);
    }
  };

  const columns: ColumnsType<RecipeJob> = [
    {
      title: 'Job',
      dataIndex: 'job_id',
      key: 'job_id',
      width: 180,
      render: (jobId: string) => (
        <Link to={`/project/${projectId}/jobs/${encodeURIComponent(jobId)}/story`}>
          <Text code>{jobId}</Text>
        </Link>
      ),
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 150,
      render: (status: RecipeJobStatus) => <Tag color={statusColors[status] || 'default'}>{status}</Tag>,
    },
    {
      title: 'Recipe',
      dataIndex: 'recipe',
      key: 'recipe',
      width: 180,
      render: (recipe?: string) => recipe || <Text type="secondary">default</Text>,
    },
    {
      title: 'Cell',
      dataIndex: 'cell_name',
      key: 'cell_name',
      width: 140,
      render: (cell?: string) => cell || <Text type="secondary">self</Text>,
    },
    {
      title: 'Repository',
      dataIndex: 'repo',
      key: 'repo',
      ellipsis: true,
      render: (repo?: string) => repo ? <Text code>{repo}</Text> : <Text type="secondary">-</Text>,
    },
    {
      title: 'Ref',
      dataIndex: 'git_ref',
      key: 'git_ref',
      width: 120,
      render: (ref?: string) => ref || <Text type="secondary">-</Text>,
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (value: string) => (value ? dayjs(value).fromNow() : '-'),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'center' }}>
          <Title level={3} style={{ margin: 0 }}>
            Jobs
          </Title>
          <Space wrap>
            <Select
              mode="multiple"
              value={filters.status}
              options={[
                ...activeStatuses.map((status) => ({ value: status, label: status })),
                { value: 'EXPIRED', label: 'EXPIRED' },
                { value: 'CANCELLED', label: 'CANCELLED' },
                { value: 'COMPLETED', label: 'COMPLETED' },
              ]}
              style={{ minWidth: 320 }}
              placeholder="Status"
              maxTagCount="responsive"
              onChange={(status) => setFilters((current) => ({ ...current, status }))}
              allowClear
            />
            <Select
              value={filters.cell}
              options={cellOptions}
              style={{ minWidth: 220 }}
              placeholder="Cell"
              onChange={(cell) => setFilters((current) => ({ ...current, cell }))}
              allowClear
              showSearch
            />
            <Button icon={<ReloadOutlined />} onClick={load}>
              Refresh
            </Button>
            <Button type="primary" icon={<PlayCircleOutlined />} onClick={() => setSubmitOpen(true)}>
              Submit Job
            </Button>
          </Space>
        </div>

        <Table
          rowKey="job_id"
          columns={columns}
          dataSource={jobs}
          loading={loading}
          pagination={{ pageSize: 25, showSizeChanger: true }}
        />
      </Space>

      <Modal
        title="Submit Job"
        open={submitOpen}
        onCancel={() => setSubmitOpen(false)}
        onOk={submit}
        okButtonProps={{ loading: submitting }}
        destroyOnClose
      >
        <Form form={form} layout="vertical" initialValues={{ inputs: '{}' }}>
          <Form.Item label="Cell" name="cell">
            <Select
              options={cellOptions}
              placeholder="self"
              allowClear
              showSearch
            />
          </Form.Item>
          <Form.Item label="Recipe" name="recipe">
            <Input placeholder="default" />
          </Form.Item>
          <Form.Item label="Job ID" name="job_id">
            <Input placeholder="auto-generated" />
          </Form.Item>
          <Form.Item label="Inputs" name="inputs">
            <Input.TextArea rows={8} spellCheck={false} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
