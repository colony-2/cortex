import { useEffect, useMemo, useRef, useState } from 'react';
import { BrowserRouter, Link, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import { FileSearchOutlined, MenuOutlined, LogoutOutlined, OrderedListOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { Alert, Badge, Button, Drawer, Empty, Input, Layout, Menu, Spin, Typography } from 'antd';
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
import { useCompact } from './hooks/useCompact';
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
  const compact = useCompact();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  useEffect(() => { if (!compact) setMenuOpen(false); }, [compact]);
  const closeMenu = () => {
    setMenuOpen(false);
    requestAnimationFrame(() => menuButton.current?.focus({ preventScroll: true }));
  };
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
    if (path.includes('/reviews')) return 'reviews';
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

  const navigation = (
          <Menu
            onClick={closeMenu}
            mode="inline"
            selectedKeys={[navKey]}
            items={[
              {
                key: 'jobs',
                label: <Link className="compact-touch-link" to={`/project/${tenantId}/goals`}>Goals</Link>,
                icon: <PlayCircleOutlined />,
              },
              {
                key: 'reviews',
                label: <Badge count={pendingCount} offset={[10, 0]} size="small"><Link className="compact-touch-link" to={`/project/${tenantId}/reviews`}>Reviews</Link></Badge>,
                icon: <FileSearchOutlined />,
              },
              {
                key: 'cells',
                label: <Link className="compact-touch-link" to={`/project/${tenantId}/cells`}>Cells</Link>,
                icon: <OrderedListOutlined />,
              },
            ]}
            style={{ height: '100%', borderRight: 0 }}
          />
  );
  return (
    <Layout className="app-shell">
      <Header className="app-header">
        <div className="app-brand">
          {compact && <Button ref={menuButton} aria-label="Open navigation" aria-expanded={menuOpen} icon={<MenuOutlined />} onClick={() => setMenuOpen(true)} />}
          <Typography.Text strong>cortex</Typography.Text>
        </div>
        <div className="tenant-control">
          <Typography.Text type="secondary">Tenant</Typography.Text>
          <Input.Search aria-label="Tenant ID" value={tenantDraft} onChange={event => setTenantDraft(event.target.value)} onSearch={applyTenant} enterButton="Use" />
        </div>
        <Button className="logout-button" icon={<LogoutOutlined />} onClick={handleLogout}>Logout</Button>
      </Header>
      <Layout className="app-body">
        {compact ? <Drawer title="Navigation" width="min(320px, 100%)" open={menuOpen} onClose={closeMenu} afterOpenChange={open => {
          // rc-drawer restores its captured element after this callback. Restore
          // our explicit opener afterwards, including interrupted animations.
          if (!open) queueMicrotask(() => menuButton.current?.focus({ preventScroll: true }));
        }}>{navigation}</Drawer>
          : <Sider width={220} theme="light">{navigation}</Sider>}
        <Content className="app-content">
          <Routes>
            <Route path="/" element={<Navigate to={`/project/${tenantId}/jobs`} replace />} />
            <Route path="/jobs" element={<JobsListPage projectId={tenantId} />} />
            <Route path="/project/:projectId/jobs" element={<JobsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/goals" element={<JobsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/goals/:goalId" element={<JobsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/jobs/:jobId/story" element={<StoryRoute fallbackProjectId={tenantId} />} />
            <Route path="/reviews" element={<PendingInputsListPage projectId={tenantId} />} />
            <Route path="/project/:projectId/reviews" element={<ReviewsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/reviews/:jobId" element={<InputRoute fallbackProjectId={tenantId} />} />
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
  const { goalId } = useParams<{ goalId: string }>();
  return <JobsListPage projectId={useRouteProjectId(fallbackProjectId)} goalId={goalId} />;
}

function ReviewsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  return <PendingInputsListPage projectId={useRouteProjectId(fallbackProjectId)} />;
}

function InputRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  const { jobId } = useParams();
  return <InputDetailPage key={JSON.stringify([projectId, jobId])} projectId={projectId} />;
}

function CellsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  return <CellsList key={projectId} projectId={projectId} />;
}

function StoryRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  const { jobId } = useParams<{ jobId: string }>();
  if (!jobId) {
    return <Empty description="Missing job ID" />;
  }
  return <JobStoryPage key={`${projectId}/${jobId}`} projectId={projectId} />;
}

export default App;
