import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Descriptions,
  Modal,
  Radio,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import { ArrowLeftOutlined, ReloadOutlined, DownloadOutlined, BranchesOutlined, FileTextOutlined } from '@ant-design/icons';
import type { WorkflowDetail, ChapterDetail, ArtifactReference } from '@colony2/openapi-client';
import { WorkflowsService } from '@colony2/openapi-client';
import dayjs from 'dayjs';
import ReactJson from 'react-json-view';

const { Title, Text } = Typography;
const { Panel } = Collapse;

// Helper to construct full artifact URL
// In dev mode, prepend API server base URL for relative URLs since UI runs on different port
const getArtifactUrl = (url: string): string => {
  // Check if URL is already absolute
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url;
  }

  // For relative URLs in dev mode, prepend API server base URL
  if (import.meta.env.DEV) {
    return `http://localhost:8080${url}`;
  }
  return url;
};

interface WorkflowDetailPageProps {
  projectId: string;
}

export default function WorkflowDetailPage({ projectId }: WorkflowDetailPageProps) {
  const { workflowId } = useParams<{ workflowId: string }>();
  const navigate = useNavigate();
  const [workflow, setWorkflow] = useState<WorkflowDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [selectedArtifact, setSelectedArtifact] = useState<ArtifactReference | null>(null);
  const [artifactContent, setArtifactContent] = useState<string | null>(null);
  const [artifactLoading, setArtifactLoading] = useState(false);
  const [viewMode, setViewMode] = useState<'text' | 'hex'>('text');

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

  const loadArtifact = async (artifact: ArtifactReference) => {
    if (!artifact.url) {
      message.error('Artifact URL not available');
      return;
    }

    setSelectedArtifact(artifact);
    setArtifactLoading(true);
    setArtifactContent(null);
    setViewMode('text');

    try {
      const response = await fetch(getArtifactUrl(artifact.url));
      if (!response.ok) {
        throw new Error('Failed to fetch artifact');
      }

      const blob = await response.blob();
      const text = await blob.text();
      setArtifactContent(text);
    } catch (err) {
      console.error('Failed to load artifact', err);
      message.error('Failed to load artifact content');
      setSelectedArtifact(null);
    } finally {
      setArtifactLoading(false);
    }
  };

  const downloadArtifact = (artifact: ArtifactReference) => {
    if (!artifact.url) {
      message.error('Artifact URL not available');
      return;
    }

    const link = document.createElement('a');
    link.href = getArtifactUrl(artifact.url);
    link.download = artifact.name;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const formatHex = (text: string): string => {
    const bytes = new TextEncoder().encode(text);
    let hex = '';
    let offset = 0;

    for (let i = 0; i < bytes.length; i += 16) {
      // Offset
      hex += offset.toString(16).padStart(8, '0') + '  ';

      // Hex bytes
      const chunk = bytes.slice(i, i + 16);
      const hexBytes = Array.from(chunk)
        .map((b) => b.toString(16).padStart(2, '0'))
        .join(' ');
      hex += hexBytes.padEnd(48, ' ') + '  ';

      // ASCII representation
      const ascii = Array.from(chunk)
        .map((b) => (b >= 32 && b <= 126 ? String.fromCharCode(b) : '.'))
        .join('');
      hex += ascii + '\n';

      offset += 16;
    }

    return hex;
  };

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
            <Space direction="vertical" style={{ width: '100%' }}>
              {chapter.artifacts.map((artifact) => (
                <div key={artifact.artifact_id} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ flex: 1 }}>
                    {artifact.url ? (
                      <a
                        href="#"
                        onClick={(e) => {
                          e.preventDefault();
                          loadArtifact(artifact);
                        }}
                        style={{ fontWeight: 500 }}
                      >
                        {artifact.name}
                      </a>
                    ) : (
                      <span>
                        <span style={{ fontWeight: 500 }}>{artifact.name}</span>
                        <Text type="secondary" style={{ marginLeft: 8, fontSize: 12 }}>
                          (URL not available)
                        </Text>
                      </span>
                    )}
                    {artifact.size_bytes && (
                      <Text type="secondary">
                        {' '}
                        ({(artifact.size_bytes / 1024).toFixed(2)} KB)
                      </Text>
                    )}
                    {artifact.artifact_type && (
                      <Text type="secondary"> - {artifact.artifact_type}</Text>
                    )}
                  </span>
                  <Space>
                    {artifact.url && (
                      <Button
                        size="small"
                        icon={<DownloadOutlined />}
                        onClick={() => downloadArtifact(artifact)}
                      >
                        Download
                      </Button>
                    )}
                  </Space>
                </div>
              ))}
            </Space>
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
          <Space>
            {workflowId ? (
              <Button
                icon={<BranchesOutlined />}
                onClick={() => navigate(`/project/${projectId}/workflows/${workflowId}/story`)}
              >
                Open story view
              </Button>
            ) : null}
            {workflowId ? (
              <Button
                icon={<FileTextOutlined />}
                onClick={() => navigate(`/project/${projectId}/workflows/${workflowId}/notebook`)}
              >
                Open notebook
              </Button>
            ) : null}
            <Button icon={<ReloadOutlined />} onClick={loadWorkflow}>
              Refresh
            </Button>
          </Space>
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
              {workflow.actor?.type === 'user' && workflow.actor.user
                ? workflow.actor.user.email
                : workflow.actor?.type === 'agent' && workflow.actor.agent
                ? workflow.actor.agent.cell
                : 'system'}
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
                    navigate(`/project/${projectId}/tickets/${workflow.ticket_id}`)
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

      <Modal
        title={selectedArtifact?.name || 'Artifact Viewer'}
        open={selectedArtifact !== null}
        onCancel={() => {
          setSelectedArtifact(null);
          setArtifactContent(null);
        }}
        width={1000}
        footer={[
          <Button
            key="download"
            icon={<DownloadOutlined />}
            onClick={() => selectedArtifact && downloadArtifact(selectedArtifact)}
          >
            Download
          </Button>,
          <Button key="close" onClick={() => setSelectedArtifact(null)}>
            Close
          </Button>,
        ]}
      >
        {artifactLoading ? (
          <div style={{ textAlign: 'center', padding: 32 }}>
            <Spin />
          </div>
        ) : (
          <>
            <Space style={{ marginBottom: 16 }}>
              <Text>View mode:</Text>
              <Radio.Group value={viewMode} onChange={(e) => setViewMode(e.target.value)}>
                <Radio.Button value="text">Text</Radio.Button>
                <Radio.Button value="hex">Hex</Radio.Button>
              </Radio.Group>
            </Space>
            <div
              style={{
                backgroundColor: '#f5f5f5',
                padding: 16,
                borderRadius: 4,
                maxHeight: 500,
                overflow: 'auto',
                fontFamily: 'monospace',
                fontSize: 12,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
              }}
            >
              {artifactContent ? (
                viewMode === 'text' ? (
                  artifactContent
                ) : (
                  formatHex(artifactContent)
                )
              ) : (
                <Text type="secondary">No content available</Text>
              )}
            </div>
          </>
        )}
      </Modal>
    </div>
  );
}
