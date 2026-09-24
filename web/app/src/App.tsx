import { useEffect, useMemo, useState } from 'react';
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import { FormOutlined, LogoutOutlined, OrderedListOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { Alert, Badge, Button, Empty, Input, Layout, Menu, Space, Spin, Typography } from 'antd';
import {
  InputActivityProvider,
  clearUserEmail,
  isAuthenticated,
  listProjects,
  useInputActivity,
} from '@colony2/shared';
import CellsList from './components/CellsList';
import JobsListPage from './components/JobsListPage';
import PendingInputsListPage from './components/PendingInputsListPage';
import InputDetailPage from './components/InputDetailPage';
import JobStoryPage from './components/JobStoryPage';
import LoginPage from './components/LoginPage';

const { Header, Content, Sider } = Layout;

function App() {
  const [defaultTenantId, setDefaultTenantId] = useState<string>();
  const [error, setError] = useState<string>();
  useEffect(() => {
    let active = true;
    listProjects().then(projects => {
      const tenant = projects[0]?.tenant_id;
      if (!tenant) throw new Error('The server did not provide a default tenant');
      if (active) setDefaultTenantId(tenant);
    }).catch(error => {
      if (active) setError(error instanceof Error ? error.message : 'Could not load Cortex configuration');
    });
    return () => { active = false; };
  }, []);

  if (error) return <Alert type="error" showIcon message="Could not load Cortex configuration" description={error} />;
  if (!defaultTenantId) return <Spin fullscreen tip="Loading Cortex…" />;
  return (
    <BrowserRouter>
      <InputActivityProvider>
        <AppShell defaultTenantId={defaultTenantId} />
      </InputActivityProvider>
    </BrowserRouter>
  );
}

function initialTenantId(search: string, pathname: string, defaultTenantId: string): string {
  const fromPath = pathname.match(/^\/project\/([^/]+)/)?.[1];
  if (fromPath) return decodeURIComponent(fromPath);
  const fromQuery = new URLSearchParams(search).get('tenantId');
  if (fromQuery?.trim()) return fromQuery.trim();
  return defaultTenantId;
}

function AppShell({ defaultTenantId }: { defaultTenantId: string }) {
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const location = useLocation();
  const navigate = useNavigate();
  const [tenantId, setTenantId] = useState(() => initialTenantId(location.search, location.pathname, defaultTenantId));
  const [tenantDraft, setTenantDraft] = useState(tenantId);
  const { pendingCount, setCurrentProjectId } = useInputActivity();

  useEffect(() => {
    const selectedTenant = initialTenantId(location.search, location.pathname, defaultTenantId);
    if (selectedTenant !== tenantId) {
      setTenantId(selectedTenant);
      setTenantDraft(selectedTenant);
    }
  }, [location.search, location.pathname, tenantId, defaultTenantId]);

  useEffect(() => {
    setCurrentProjectId(tenantId);
    document.title = `cortex: tenant ${tenantId}`;
  }, [tenantId, setCurrentProjectId]);

  const navKey = useMemo(() => {
    const path = location.pathname;
    if (path.includes('/inputs')) return 'inputs';
    if (path.includes('/cells')) return 'cells';
    return 'jobs';
  }, [location.pathname]);

  if (!authenticated) {
    return <LoginPage onLogin={() => setAuthenticated(true)} />;
  }

  const applyTenant = () => {
    const next = tenantDraft.trim() || defaultTenantId;
    setTenantId(next);
    navigate(`/project/${encodeURIComponent(next)}/jobs`);
  };

  const handleLogout = () => {
    clearUserEmail();
    setAuthenticated(false);
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#fff', borderBottom: '1px solid #f0f0f0' }}>
        <Space align="center" style={{ width: '100%', justifyContent: 'space-between' }}>
          <Space align="center">
            <Typography.Text strong>cortex</Typography.Text>
            <Typography.Text type="secondary">Tenant</Typography.Text>
            <Input.Search
              aria-label="Tenant ID"
              value={tenantDraft}
              onChange={(event) => setTenantDraft(event.target.value)}
              onSearch={applyTenant}
              enterButton="Use"
              style={{ width: 220 }}
            />
          </Space>
          <Button icon={<LogoutOutlined />} onClick={handleLogout}>
            Logout
          </Button>
        </Space>
      </Header>
      <Layout style={{ height: 'calc(100vh - 64px)' }}>
        <Sider width={220} theme="light" collapsedWidth={72}>
          <Menu
            mode="inline"
            selectedKeys={[navKey]}
            items={[
              {
                key: 'jobs',
                label: <Link to={`/project/${tenantId}/jobs`}>Jobs</Link>,
                icon: <PlayCircleOutlined />,
              },
              {
                key: 'inputs',
                label: (
                  <Badge count={pendingCount} offset={[10, 0]} size="small">
                    <Link to={`/project/${tenantId}/inputs`}>Pending Inputs</Link>
                  </Badge>
                ),
                icon: <FormOutlined />,
              },
              {
                key: 'cells',
                label: <Link to={`/project/${tenantId}/cells`}>Cells</Link>,
                icon: <OrderedListOutlined />,
              },
            ]}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content style={{ overflow: 'auto' }}>
          <Routes>
            <Route path="/" element={<Navigate to={`/project/${tenantId}/jobs`} replace />} />
            <Route path="/jobs" element={<JobsListPage projectId={tenantId} />} />
            <Route path="/project/:projectId/jobs" element={<JobsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/jobs/:jobId/story" element={<StoryRoute fallbackProjectId={tenantId} />} />
            <Route path="/inputs" element={<PendingInputsListPage projectId={tenantId} />} />
            <Route path="/project/:projectId/inputs" element={<InputsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/inputs/:jobId" element={<InputRoute fallbackProjectId={tenantId} />} />
            <Route path="/cells" element={<CellsList projectId={tenantId} />} />
            <Route path="/project/:projectId/cells" element={<CellsRoute fallbackProjectId={tenantId} />} />
            <Route path="*" element={<Navigate to={`/project/${tenantId}/jobs`} replace />} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
  );
}

function useRouteProjectId(fallbackProjectId: string): string {
  const { projectId } = useParams<{ projectId: string }>();
  return projectId || fallbackProjectId;
}

function JobsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  return <JobsListPage projectId={useRouteProjectId(fallbackProjectId)} />;
}

function InputsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  return <PendingInputsListPage projectId={useRouteProjectId(fallbackProjectId)} />;
}

function InputRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  return <InputDetailPage projectId={useRouteProjectId(fallbackProjectId)} />;
}

function CellsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  return <CellsList projectId={useRouteProjectId(fallbackProjectId)} />;
}

function StoryRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  const { jobId } = useParams<{ jobId: string }>();
  if (!jobId) {
    return <Empty description="Missing job ID" />;
  }
  return <JobStoryPage projectId={projectId} />;
}

export default App;
