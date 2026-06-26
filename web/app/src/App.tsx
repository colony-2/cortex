import { useEffect, useMemo, useState } from 'react';
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import { FormOutlined, LogoutOutlined, OrderedListOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { Badge, Button, Empty, Input, Layout, Menu, Space, Typography } from 'antd';
import {
  InputActivityProvider,
  clearUserEmail,
  isAuthenticated,
  useInputActivity,
} from '@colony2/shared';
import CellsList from './components/CellsList';
import JobsListPage from './components/JobsListPage';
import PendingInputsListPage from './components/PendingInputsListPage';
import InputDetailPage from './components/InputDetailPage';
import JobStoryPage from './components/JobStoryPage';
import LoginPage from './components/LoginPage';

const { Header, Content, Sider } = Layout;
const defaultTenantId = '1';

function App() {
  return (
    <BrowserRouter>
      <InputActivityProvider>
        <AppShell />
      </InputActivityProvider>
    </BrowserRouter>
  );
}

function initialTenantId(search: string): string {
  const fromQuery = new URLSearchParams(search).get('tenantId');
  if (fromQuery?.trim()) return fromQuery.trim();
  return localStorage.getItem('cortex:tenantId') || defaultTenantId;
}

function AppShell() {
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const location = useLocation();
  const navigate = useNavigate();
  const [tenantId, setTenantId] = useState(() => initialTenantId(location.search));
  const [tenantDraft, setTenantDraft] = useState(tenantId);
  const { pendingCount, setCurrentProjectId } = useInputActivity();

  useEffect(() => {
    const queryTenant = new URLSearchParams(location.search).get('tenantId');
    if (queryTenant?.trim() && queryTenant.trim() !== tenantId) {
      setTenantId(queryTenant.trim());
      setTenantDraft(queryTenant.trim());
    }
  }, [location.search, tenantId]);

  useEffect(() => {
    localStorage.setItem('cortex:tenantId', tenantId);
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
