import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Button,
  DatePicker,
  Input,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import type { WorkflowSummary, WorkflowStatus } from '@colony2/openapi-client';
import { WorkflowsService } from '@colony2/openapi-client';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

const { Title } = Typography;
const { RangePicker } = DatePicker;

interface WorkflowListPageProps {
  projectId: string;
}

export default function WorkflowListPage({ projectId }: WorkflowListPageProps) {
  const navigate = useNavigate();
  const [workflows, setWorkflows] = useState<WorkflowSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 50,
    total: 0,
  });
  const [filters, setFilters] = useState<{
    status?: WorkflowStatus[];
    ticketId?: string;
    cellId?: string;
    since?: string;
    until?: string;
  }>({});

  const loadWorkflows = async (page = 1, pageSize = 50) => {
    setLoading(true);
    try {
      const offset = (page - 1) * pageSize;
      const data = await WorkflowsService.getApiProjectsWorkflows(
        projectId,
        filters.status,
        filters.ticketId || undefined,
        filters.cellId || undefined,
        filters.since,
        filters.until,
        pageSize,
        offset,
      );
      setWorkflows(data || []);
      // Note: If API returns total count, use it here
      setPagination({
        current: page,
        pageSize,
        total: data.length === pageSize ? (page + 1) * pageSize : offset + data.length,
      });
    } catch (err) {
      console.error('Failed to load workflows', err);
      message.error('Failed to load workflows');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadWorkflows(pagination.current, pagination.pageSize);
  }, [projectId, filters]);

  const statusColors: Record<string, string> = {
    running: 'blue',
    completed: 'green',
    failed: 'red',
    canceled: 'default',
    terminated: 'default',
    timed_out: 'orange',
    unknown: 'default',
  };

  const calculateDuration = (record: WorkflowSummary): string => {
    if (!record.start_time) return '-';
    const end = record.close_time || new Date().toISOString();
    const duration = dayjs(end).diff(dayjs(record.start_time), 'second');
    if (duration > 60) {
      const minutes = Math.floor(duration / 60);
      const seconds = duration % 60;
      return `${minutes}m ${seconds}s`;
    }
    return `${duration}s`;
  };

  const columns: ColumnsType<WorkflowSummary> = [
    {
      title: 'Workflow ID',
      dataIndex: 'workflow_id',
      key: 'workflow_id',
      render: (id: string) => (
        <a onClick={() => navigate(`/project/${projectId}/workflows/${id}`)}>
          {id.slice(-12)}
        </a>
      ),
      width: 120,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={statusColors[status]}>{status.toUpperCase()}</Tag>
      ),
      width: 110,
    },
    {
      title: 'Recipe',
      dataIndex: 'recipe_name',
      key: 'recipe_name',
      ellipsis: true,
    },
    {
      title: 'Ticket',
      key: 'ticket',
      render: (_, record: WorkflowSummary) =>
        record.ticket_id ? (
          <a onClick={() => navigate(`/project/${projectId}/tickets/${record.ticket_id}`)}>
            {record.ticket_title || record.ticket_id.slice(-8)}
          </a>
        ) : (
          <span style={{ color: '#999' }}>-</span>
        ),
      ellipsis: true,
    },
    {
      title: 'Cell',
      dataIndex: 'cell_name',
      key: 'cell_name',
      render: (name?: string) => name || <span style={{ color: '#999' }}>-</span>,
      width: 120,
    },
    {
      title: 'Started',
      dataIndex: 'start_time',
      key: 'start_time',
      render: (time?: string) => (time ? dayjs(time).fromNow() : '-'),
      width: 130,
    },
    {
      title: 'Duration',
      key: 'duration',
      render: (_, record) => calculateDuration(record),
      width: 100,
    },
    {
      title: 'Actor',
      dataIndex: 'actor',
      key: 'actor',
      render: (actor: any) => actor?.actor_email || 'system',
      ellipsis: true,
      width: 150,
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Title level={2} style={{ margin: 0 }}>
            Workflows
          </Title>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => loadWorkflows(pagination.current, pagination.pageSize)}
          >
            Refresh
          </Button>
        </div>

        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
          <Select
            mode="multiple"
            placeholder="Filter by status"
            style={{ minWidth: 200 }}
            onChange={(value) => setFilters({ ...filters, status: value })}
            options={[
              { label: 'Running', value: 'running' },
              { label: 'Completed', value: 'completed' },
              { label: 'Failed', value: 'failed' },
              { label: 'Canceled', value: 'canceled' },
              { label: 'Terminated', value: 'terminated' },
              { label: 'Timed Out', value: 'timed_out' },
            ]}
            allowClear
          />
          <Input
            placeholder="Ticket ID"
            style={{ width: 200 }}
            onChange={(e) =>
              setFilters({ ...filters, ticketId: e.target.value || undefined })
            }
            allowClear
          />
          <Input
            placeholder="Cell ID"
            style={{ width: 200 }}
            onChange={(e) => setFilters({ ...filters, cellId: e.target.value || undefined })}
            allowClear
          />
          <RangePicker
            onChange={(dates) =>
              setFilters({
                ...filters,
                since: dates?.[0]?.toISOString(),
                until: dates?.[1]?.toISOString(),
              })
            }
          />
        </div>

        <Table
          columns={columns}
          dataSource={workflows}
          rowKey="workflow_id"
          loading={loading}
          pagination={{
            ...pagination,
            onChange: (page, pageSize) => {
              setPagination({ ...pagination, current: page, pageSize: pageSize || 50 });
              loadWorkflows(page, pageSize || 50);
            },
            showSizeChanger: true,
            pageSizeOptions: ['20', '50', '100', '200'],
          }}
          onRow={(record) => ({
            onClick: () => navigate(`/project/${projectId}/workflows/${record.workflow_id}`),
            style: { cursor: 'pointer' },
          })}
        />
      </Space>
    </div>
  );
}
