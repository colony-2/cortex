# 04 - Workflow UI Components

**Project**: `web/app`

**Dependencies**: `03-workflow-api-handlers.md`

**Implements**: React UI for workflow exploration

---

## Overview

Build React components for browsing and inspecting workflows. Provides list view with filtering/pagination and detail view with chapter-by-chapter execution breakdown.

---

## Dependencies Installation

**File**: `web/app/package.json`

```bash
cd web/app
pnpm add react-json-view@^1.21.3
```

Ensure existing dependencies:
- `antd` (Ant Design)
- `react-router-dom`
- `dayjs`
- `@colony2/openapi-client`

---

## OpenAPI Client Regeneration

After completing spec 01 (OpenAPI) and 02 (bindings), regenerate the client:

```bash
cd web/app
pnpm run generate-client
```

**Verify generated types**:
- `WorkflowsService` with `getApiProjectsWorkflows` and `getApiProjectsWorkflows1` methods
- `WorkflowSummary`, `WorkflowDetail`, `ChapterDetail` types
- `WorkflowStatus`, `ChapterStatus` enums

---

## Component 1: Workflow List Page

**File**: `web/app/src/components/WorkflowListPage.tsx`

```tsx
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
import { ReloadOutlined, FilterOutlined } from '@ant-design/icons';
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
      const data = await WorkflowsService.getApiProjectsWorkflows(projectId, {
        status: filters.status,
        ticket_id: filters.ticketId || undefined,
        cell_id: filters.cellId || undefined,
        since: filters.since,
        until: filters.until,
        limit: pageSize,
        offset,
      });
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

  const statusColors: Record<WorkflowStatus, string> = {
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
        <a onClick={() => navigate(`/projects/${projectId}/workflows/${id}`)}>
          {id.slice(-12)}
        </a>
      ),
      width: 120,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: WorkflowStatus) => (
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
          <a onClick={() => navigate(`/projects/${projectId}/tickets/${record.ticket_id}`)}>
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
            onClick: () => navigate(`/projects/${projectId}/workflows/${record.workflow_id}`),
            style: { cursor: 'pointer' },
          })}
        />
      </Space>
    </div>
  );
}
```

---

## Component 2: Workflow Detail Page

**File**: `web/app/src/components/WorkflowDetailPage.tsx`

```tsx
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
      const data = await WorkflowsService.getApiProjectsWorkflows1(projectId, workflowId, {
        includeRawJobData: false,
      });
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
```

---

## Routing Configuration

**File**: `web/app/src/App.tsx`

Add workflow routes:

```tsx
import WorkflowListPage from './components/WorkflowListPage';
import WorkflowDetailPage from './components/WorkflowDetailPage';

// Inside Routes component:
function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* ... existing routes */}

        <Route
          path="/projects/:projectId/workflows"
          element={<WorkflowListPage projectId={projectId} />}
        />
        <Route
          path="/projects/:projectId/workflows/:workflowId"
          element={<WorkflowDetailPage projectId={projectId} />}
        />
      </Routes>
    </BrowserRouter>
  );
}
```

**Note**: Adjust based on your actual routing setup. If using a layout component, ensure workflow pages are wrapped appropriately.

---

## Navigation Integration

**File**: `web/app/src/components/ProjectLayout.tsx` (or sidebar/menu component)

Add "Workflows" navigation item:

```tsx
import { ThunderboltOutlined } from '@ant-design/icons';
import { Menu } from 'antd';
import { Link } from 'react-router-dom';

// Inside menu items:
<Menu.Item key="workflows" icon={<ThunderboltOutlined />}>
  <Link to={`/projects/${projectId}/workflows`}>Workflows</Link>
</Menu.Item>
```

---

## Component Tests

### Test 1: Workflow List Page

**File**: `web/app/src/components/WorkflowListPage.test.tsx`

```tsx
import { render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import WorkflowListPage from './WorkflowListPage';
import { WorkflowsService } from '@colony2/openapi-client';

jest.mock('@colony2/openapi-client');

describe('WorkflowListPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders workflow list', async () => {
    const mockWorkflows = [
      {
        workflow_id: 'prj_123_abc',
        run_id: 'prj_123_abc',
        status: 'running',
        recipe_name: 'test-recipe',
        created_at: '2025-12-22T10:00:00Z',
      },
    ];

    (WorkflowsService.getApiProjectsWorkflows as jest.Mock).mockResolvedValue(
      mockWorkflows
    );

    render(
      <BrowserRouter>
        <WorkflowListPage projectId="prj_123" />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/test-recipe/i)).toBeInTheDocument();
    });
  });

  it('filters workflows by status', async () => {
    // TODO: Implement filter interaction test
  });

  it('handles loading state', () => {
    (WorkflowsService.getApiProjectsWorkflows as jest.Mock).mockImplementation(
      () => new Promise(() => {}) // Never resolves
    );

    render(
      <BrowserRouter>
        <WorkflowListPage projectId="prj_123" />
      </BrowserRouter>
    );

    expect(screen.getByRole('table')).toBeInTheDocument();
  });

  it('handles error state', async () => {
    (WorkflowsService.getApiProjectsWorkflows as jest.Mock).mockRejectedValue(
      new Error('Network error')
    );

    render(
      <BrowserRouter>
        <WorkflowListPage projectId="prj_123" />
      </BrowserRouter>
    );

    await waitFor(() => {
      // Check for error message (Ant Design message.error)
    });
  });
});
```

### Test 2: Workflow Detail Page

**File**: `web/app/src/components/WorkflowDetailPage.test.tsx`

```tsx
import { render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import WorkflowDetailPage from './WorkflowDetailPage';
import { WorkflowsService } from '@colony2/openapi-client';

jest.mock('@colony2/openapi-client');

describe('WorkflowDetailPage', () => {
  const mockWorkflow = {
    workflow_id: 'prj_123_abc',
    run_id: 'prj_123_abc',
    status: 'completed',
    recipe_name: 'test-recipe',
    chapters: [
      {
        chapter_number: 0,
        chapter_type: 'recipe',
        status: 'completed',
        input: {},
        artifacts: [],
      },
    ],
    created_at: '2025-12-22T10:00:00Z',
  };

  it('renders workflow detail', async () => {
    (WorkflowsService.getApiProjectsWorkflows1 as jest.Mock).mockResolvedValue(
      mockWorkflow
    );

    render(
      <BrowserRouter>
        <Routes>
          <Route
            path="/projects/:projectId/workflows/:workflowId"
            element={<WorkflowDetailPage projectId="prj_123" />}
          />
        </Routes>
      </BrowserRouter>,
      { initialEntries: ['/projects/prj_123/workflows/prj_123_abc'] }
    );

    await waitFor(() => {
      expect(screen.getByText(/Workflow Detail/i)).toBeInTheDocument();
      expect(screen.getByText(/test-recipe/i)).toBeInTheDocument();
    });
  });

  it('renders chapters', async () => {
    (WorkflowsService.getApiProjectsWorkflows1 as jest.Mock).mockResolvedValue(
      mockWorkflow
    );

    render(
      <BrowserRouter>
        <WorkflowDetailPage projectId="prj_123" />
      </BrowserRouter>
    );

    await waitFor(() => {
      expect(screen.getByText(/Chapter 0:/i)).toBeInTheDocument();
    });
  });
});
```

---

## Implementation Checklist

- [ ] Install dependencies: `pnpm add react-json-view`
- [ ] Regenerate OpenAPI client: `pnpm run generate-client`
- [ ] Create `WorkflowListPage.tsx`
- [ ] Create `WorkflowDetailPage.tsx`
- [ ] Add routes in `App.tsx`
- [ ] Add navigation menu item
- [ ] Write component tests
- [ ] Run tests: `pnpm test`
- [ ] Build: `pnpm build`
- [ ] Manual testing in browser

---

## Manual Testing

### Test 1: Access Workflow List

1. Start dev server: `pnpm dev`
2. Navigate to `http://localhost:3000/projects/{projectId}/workflows`
3. Verify table displays with columns: ID, Status, Recipe, Ticket, Cell, Started, Duration, Actor
4. Create a ticket to trigger workflow
5. Refresh list and verify new workflow appears

### Test 2: Filter Workflows

1. In workflow list, select "Running" from status filter
2. Verify only running workflows shown
3. Clear filter and select "Completed"
4. Verify only completed workflows shown
5. Test ticket ID and cell ID filters
6. Test date range picker

### Test 3: View Workflow Detail

1. Click on a workflow row in the list
2. Verify navigation to detail page
3. Verify workflow metadata displayed (ID, status, recipe, timestamps)
4. Verify chapters list displayed
5. Expand a chapter panel
6. Verify input/output JSON viewers work
7. Test "Back" button navigation

### Test 4: Chapter Details

1. In workflow detail, expand chapter 0 (recipe setup)
2. Verify chapter type shows "Recipe Setup"
3. Expand chapter 1 (first op)
4. Verify op name displayed
5. Verify input/output JSON is formatted correctly
6. If artifacts present, verify links work

---

## Styling Considerations

**Optional**: Create custom styles for workflow pages

**File**: `web/app/src/components/WorkflowListPage.module.css`

```css
.workflowRow {
  cursor: pointer;
  transition: background-color 0.2s;
}

.workflowRow:hover {
  background-color: #f5f5f5;
}

.filterBar {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
```

Import and use in component if needed.

---

## Accessibility

Ensure components are accessible:
- [ ] All interactive elements keyboard-navigable
- [ ] Proper ARIA labels for status tags
- [ ] Table has proper headers
- [ ] Expand/collapse panels keyboard-accessible
- [ ] Error messages announced to screen readers

---

## Next Steps

After completing this spec:
1. Deploy frontend to staging
2. End-to-end testing: Create ticket → View workflow in UI → Inspect chapters
3. Performance testing with large workflow lists
4. User acceptance testing
5. Deploy to production

---

## Files Created/Modified

**New Files**:
- `web/app/src/components/WorkflowListPage.tsx`
- `web/app/src/components/WorkflowDetailPage.tsx`
- `web/app/src/components/WorkflowListPage.test.tsx`
- `web/app/src/components/WorkflowDetailPage.test.tsx`

**Modified Files**:
- `web/app/src/App.tsx` - Add routes
- `web/app/src/components/ProjectLayout.tsx` - Add navigation
- `web/app/package.json` - Add react-json-view dependency
