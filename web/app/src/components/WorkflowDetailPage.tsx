import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Descriptions,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import { ArrowLeftOutlined, ReloadOutlined } from '@ant-design/icons';
import type { WorkflowDetail, ChapterDetail } from '@colony2/openapi-client';
import { WorkflowsService } from '@colony2/openapi-client';
import dayjs from 'dayjs';
import ReactJson from 'react-json-view';

const { Title, Text } = Typography;
const { Panel } = Collapse;

interface WorkflowDetailPageProps {
  projectId: string;
}

export default function WorkflowDetailPage({ projectId }: WorkflowDetailPageProps) {
  const { workflowId } = useParams<{ workflowId: string }>();
  const navigate = useNavigate();
  const [workflow, setWorkflow] = useState<WorkflowDetail | null>(null);
  const [loading, setLoading] = useState(false);

  const loadWorkflow = async () => {
    if (!workflowId) return;
    setLoading(true);
    try {
      const data = await WorkflowsService.getApiProjectsWorkflows1(
        projectId,
        workflowId,
        false,
      );
      setWorkflow(data);
    } catch (err) {
      console.error('Failed to load workflow', err);
      message.error('Failed to load workflow details');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadWorkflow();
  }, [projectId, workflowId]);

  if (loading || !workflow) {
    return (
      <div style={{ padding: '24px', textAlign: 'center' }}>
        <Spin size="large" />
      </div>
    );
  }

  const statusColors: Record<string, string> = {
    running: 'blue',
    completed: 'green',
    failed: 'red',
    canceled: 'default',
    terminated: 'default',
    timed_out: 'orange',
    unknown: 'default',
  };

  const chapterStatusColors: Record<string, string> = {
    pending: 'default',
    running: 'blue',
    completed: 'green',
    failed: 'red',
    skipped: 'orange',
  };

  const renderChapter = (chapter: ChapterDetail) => {
    const duration =
      chapter.start_time && chapter.end_time
        ? dayjs(chapter.end_time).diff(dayjs(chapter.start_time), 'second')
        : null;

    return (
      <Panel
        key={chapter.chapter_number}
        header={
          <Space>
            <Text strong>
              Chapter {chapter.chapter_number}:{' '}
              {chapter.op_name || chapter.chapter_type}
            </Text>
            <Tag color={chapterStatusColors[chapter.status]}>
              {chapter.status.toUpperCase()}
            </Tag>
            {duration !== null && <Text type="secondary">{duration}s</Text>}
          </Space>
        }
      >
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="Type">
            {chapter.chapter_type === 'recipe' ? 'Recipe Setup' : 'Operation'}
          </Descriptions.Item>
          {chapter.op_name && (
            <Descriptions.Item label="Operation">{chapter.op_name}</Descriptions.Item>
          )}
          <Descriptions.Item label="Start Time">
            {chapter.start_time
              ? dayjs(chapter.start_time).format('YYYY-MM-DD HH:mm:ss')
              : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="End Time">
            {chapter.end_time ? dayjs(chapter.end_time).format('YYYY-MM-DD HH:mm:ss') : '-'}
          </Descriptions.Item>
          {duration !== null && (
            <Descriptions.Item label="Duration">{duration} seconds</Descriptions.Item>
          )}
        </Descriptions>

        {chapter.error && (
          <Alert
            message="Error"
            description={chapter.error}
            type="error"
            showIcon
            style={{ marginTop: 16 }}
          />
        )}

        {chapter.input && Object.keys(chapter.input).length > 0 && (
          <Card title="Input" size="small" style={{ marginTop: 16 }}>
            <ReactJson
              src={chapter.input}
              collapsed={1}
              displayDataTypes={false}
              enableClipboard
              theme="rjv-default"
            />
          </Card>
        )}

        {chapter.output && Object.keys(chapter.output).length > 0 && (
          <Card title="Output" size="small" style={{ marginTop: 16 }}>
            <ReactJson
              src={chapter.output}
              collapsed={1}
              displayDataTypes={false}
              enableClipboard
              theme="rjv-default"
            />
          </Card>
        )}

        {chapter.artifacts && chapter.artifacts.length > 0 && (
          <Card title="Artifacts" size="small" style={{ marginTop: 16 }}>
            <ul style={{ margin: 0, paddingLeft: 20 }}>
              {chapter.artifacts.map((artifact) => (
                <li key={artifact.artifact_id}>
                  {artifact.url ? (
                    <a href={artifact.url} target="_blank" rel="noopener noreferrer">
                      {artifact.name}
                    </a>
                  ) : (
                    <span>{artifact.name}</span>
                  )}
                  {artifact.size_bytes && (
                    <Text type="secondary">
                      {' '}
                      ({(artifact.size_bytes / 1024).toFixed(2)} KB)
                    </Text>
                  )}
                  <Text type="secondary"> - {artifact.artifact_type}</Text>
                </li>
              ))}
            </ul>
          </Card>
        )}
      </Panel>
    );
  };

  return (
    <div style={{ padding: '24px' }}>
      <Space direction="vertical" style={{ width: '100%' }} size="large">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)}>
              Back
            </Button>
            <Title level={2} style={{ margin: 0 }}>
              Workflow Detail
            </Title>
          </Space>
          <Button icon={<ReloadOutlined />} onClick={loadWorkflow}>
            Refresh
          </Button>
        </div>

        <Card title="Workflow Information">
          <Descriptions column={2} bordered>
            <Descriptions.Item label="Workflow ID" span={2}>
              <Text code>{workflow.workflow_id}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="Status">
              <Tag color={statusColors[workflow.status]}>
                {workflow.status.toUpperCase()}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Recipe">{workflow.recipe_name}</Descriptions.Item>
            <Descriptions.Item label="Actor">
              {workflow.actor?.actor_email || 'system'}
            </Descriptions.Item>
            <Descriptions.Item label="Started">
              {workflow.start_time
                ? dayjs(workflow.start_time).format('YYYY-MM-DD HH:mm:ss')
                : '-'}
            </Descriptions.Item>
            <Descriptions.Item label="Completed">
              {workflow.close_time
                ? dayjs(workflow.close_time).format('YYYY-MM-DD HH:mm:ss')
                : '-'}
            </Descriptions.Item>
            {workflow.git_ref && (
              <Descriptions.Item label="Git Ref">{workflow.git_ref}</Descriptions.Item>
            )}
            {workflow.git_commit && (
              <Descriptions.Item label="Git Commit">
                <Text code>{workflow.git_commit.slice(0, 8)}</Text>
              </Descriptions.Item>
            )}
            {workflow.ticket_id && (
              <Descriptions.Item label="Ticket" span={2}>
                <a
                  onClick={() =>
                    navigate(`/projects/${projectId}/tickets/${workflow.ticket_id}`)
                  }
                >
                  {workflow.ticket?.title || workflow.ticket_id}
                </a>
              </Descriptions.Item>
            )}
            {workflow.cell_name && (
              <Descriptions.Item label="Cell">{workflow.cell_name}</Descriptions.Item>
            )}
          </Descriptions>
        </Card>

        {workflow.ticket && (
          <Card title="Associated Ticket">
            <Descriptions column={1} bordered>
              <Descriptions.Item label="Title">{workflow.ticket.title}</Descriptions.Item>
              <Descriptions.Item label="Description">
                {workflow.ticket.description}
              </Descriptions.Item>
              <Descriptions.Item label="Stage">{workflow.ticket.stage}</Descriptions.Item>
              <Descriptions.Item label="State">{workflow.ticket.state}</Descriptions.Item>
            </Descriptions>
          </Card>
        )}

        <Card title={`Chapters (${workflow.chapters.length})`}>
          <Collapse accordion>{workflow.chapters.map(renderChapter)}</Collapse>
        </Card>
      </Space>
    </div>
  );
}
