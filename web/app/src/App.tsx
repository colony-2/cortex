import { useEffect, useRef, useState } from 'react';
import { BrowserRouter, Link, matchPath, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import { CloudServerOutlined, MenuFoldOutlined, MenuUnfoldOutlined, MenuOutlined, SettingOutlined } from '@ant-design/icons';
import { Alert, Button, Drawer, Empty, Input, Layout, Spin, Tooltip, Typography } from 'antd';
import {
  InputActivityProvider,
  getUserEmail,
  clearUserEmail,
  isAuthenticated,
  listProjects,
  useInputActivity,
} from '@colony2/shared';
import RunnersPage from './runners/RunnersPage';
import TreePage from './executionTree/TreePage';
import CellsList from './components/CellsList';
import CellPage from './components/CellPage';
import AdminPage from './components/AdminPage';
import JobsListPage from './components/JobsListPage';
import PendingInputsListPage from './components/PendingInputsListPage';
import InputDetailPage from './components/InputDetailPage';
import JobStoryPage from './components/JobStoryPage';
import { useCompact } from './hooks/useCompact';
import AccountMenu from './components/AccountMenu';
import LoginPage from './components/LoginPage';
import InboxNavigation from './components/InboxNavigation';
import ComposeJob from './components/ComposeJob';
import { JobInboxProvider } from './hooks/useJobInbox';
import './inbox.css';

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

const reviewDetailPath = '/project/:projectId/reviews/:jobId';
const isStoryInput = (selectors: URLSearchParams) => selectors.has('occurrenceId') || selectors.has('requestId');

export function AppShell({ defaultTenantId }: { defaultTenantId: string }) {
  const location = useLocation();
  const [identity, setIdentity] = useState(getUserEmail);
  useEffect(() => {
    const changed = () => setIdentity(getUserEmail());
    window.addEventListener('cortex-ui-identity', changed);
    return () => window.removeEventListener('cortex-ui-identity', changed);
  }, []);
  const tenant = initialTenantId(location.search, location.pathname, defaultTenantId);
  return <JobInboxProvider key={JSON.stringify([tenant, identity])} tenant={tenant}><InboxShell defaultTenantId={defaultTenantId} /></JobInboxProvider>;
}
function InboxShell({ defaultTenantId }: { defaultTenantId: string }) {
  const compact = useCompact();
  const location = useLocation();
  const selectors = new URLSearchParams(location.search);
  const reviewOpen = !!matchPath(reviewDetailPath, location.pathname) && selectors.has('review_id') && !isStoryInput(selectors);
  const openReviewPath = reviewOpen ? location.pathname : null;
  const useNavigationDrawer = compact || reviewOpen;
  const story = /\/jobs\/[^/]+\/story$/.test(location.pathname);
  const [navigationChoice, setNavigationChoice] = useState<{path: string; expanded: boolean}>();
  const collapsed = navigationChoice?.path === location.pathname ? !navigationChoice.expanded : story;
  const toggleNavigation = () => setNavigationChoice({path: location.pathname, expanded: collapsed});
  const [composeOpen, setComposeOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  useEffect(() => { setMenuOpen(false); }, [useNavigationDrawer, openReviewPath]);
  const closeMenu = () => {
    setMenuOpen(false);
    requestAnimationFrame(() => menuButton.current?.focus({ preventScroll: true }));
  };
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const navigate = useNavigate();
  const [tenantId, setTenantId] = useState(() => initialTenantId(location.search, location.pathname, defaultTenantId));
  const [tenantDraft, setTenantDraft] = useState(tenantId);
  const { setCurrentProjectId } = useInputActivity();

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

  useEffect(() => {
    const query = new URLSearchParams(location.search);
    if (query.get('executionState') === 'cells-v1') return;
    const obsolete = ['cell', 'cellSelection', 'pageToken', 'expanded', 'detail', 'inspect'];
    if (!obsolete.some(key => query.has(key))) return;
    obsolete.forEach(key => query.delete(key));
    query.set('executionState', 'cells-v1');
    navigate({ pathname: location.pathname, search: query.toString() }, { replace: true, state: null });
  }, [location.pathname, location.search, navigate]);

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

  const rail = !useNavigationDrawer && collapsed;
  const navigation = <InboxNavigation rail={rail} onCompose={() => setComposeOpen(true)} onNavigate={closeMenu}
    onExpand={() => setNavigationChoice({path: location.pathname, expanded: true})} />;
  return (
    <Layout className="app-shell">
      <Header className="app-header">
        <div className="app-brand">
          {useNavigationDrawer && <Button ref={menuButton} aria-label="Open navigation" aria-expanded={menuOpen} icon={<MenuOutlined />} onClick={() => setMenuOpen(true)} />}
          <Typography.Text strong>cortex</Typography.Text>
        </div>
        <div className="tenant-control">
          <Typography.Text type="secondary">Tenant</Typography.Text>
          <Input.Search aria-label="Tenant ID" value={tenantDraft} onChange={event => setTenantDraft(event.target.value)} onSearch={applyTenant} enterButton="Use" />
        </div>
        <Tooltip title="Runners"><Link className="inbox-settings" aria-label="Runners" to={`/project/${encodeURIComponent(tenantId)}/runners`}><CloudServerOutlined /></Link></Tooltip>
        <Tooltip title="Settings"><Link className="inbox-settings" aria-label="Settings" to={`/project/${encodeURIComponent(tenantId)}/admin`}><SettingOutlined /></Link></Tooltip>
        <AccountMenu onLogout={handleLogout} />
      </Header>
      <Layout className="app-body">
        {useNavigationDrawer ? <Drawer title="Navigation" width="min(320px, 100%)" open={menuOpen} onClose={closeMenu} afterOpenChange={open => {
          // rc-drawer restores its captured element after this callback. Restore
          // our explicit opener afterwards, including interrupted animations.
          if (!open) queueMicrotask(() => menuButton.current?.focus({ preventScroll: true }));
        }}>{navigation}</Drawer>
          : <Sider className="desktop-navigation" width={256} collapsedWidth={76} collapsed={rail} collapsible
            theme="light" trigger={<button className="navigation-toggle" aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'}
              aria-expanded={!collapsed}>{collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}</button>}
            onCollapse={toggleNavigation}>{navigation}</Sider>}
        <Content className="app-content">
          <Routes key={initialTenantId(location.search, location.pathname, defaultTenantId)}>
            <Route path="/" element={<Navigate to={`/project/${tenantId}/jobs`} replace />} />
            <Route path="/jobs" element={<JobsListPage key={tenantId} projectId={tenantId} />} />
            <Route path="/project/:projectId/jobs" element={<JobsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/goals" element={<Navigate to={`/project/${encodeURIComponent(tenantId)}/jobs`} replace />} />
            <Route path="/project/:projectId/goals/:goalId" element={<Alert type="info" message="Goal links have been retired" description={<Link to={`/project/${encodeURIComponent(tenantId)}/jobs`}>Open All jobs</Link>} />} />
            <Route path="/project/:projectId/jobs/:jobId/tree" element={<TreePage />} />
            <Route path="/project/:projectId/jobs/:jobId/story" element={<StoryRoute fallbackProjectId={tenantId} />} />
            <Route path="/reviews" element={<PendingInputsListPage projectId={tenantId} />} />
            <Route path="/project/:projectId/reviews" element={<ReviewsRoute fallbackProjectId={tenantId} />} />
            <Route path={reviewDetailPath} element={<InputRoute fallbackProjectId={tenantId} />} />
            <Route path="/cells" element={<CellsList key={tenantId} projectId={tenantId} />} />
            <Route path="/project/:projectId/cells/:cellId" element={<CellRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/cells" element={<CellsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/runners/*" element={<RunnersPage />} />
            <Route path="/project/:projectId/admin" element={<AdminRoute fallbackProjectId={tenantId} />} />
            <Route path="*" element={<Navigate to={`/project/${tenantId}/jobs`} replace />} />
          </Routes>
        </Content>
      </Layout>
      <ComposeJob key={JSON.stringify([tenantId, getUserEmail()])} open={composeOpen} onClose={() => setComposeOpen(false)} />
    </Layout>
  );
}

function useRouteProjectId(fallbackProjectId: string): string {
  const { projectId } = useParams<{ projectId: string }>();
  return projectId || fallbackProjectId;
}

function JobsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const { goalId } = useParams<{ goalId: string }>();
  const projectId = useRouteProjectId(fallbackProjectId);
  return <JobsListPage key={projectId} projectId={projectId} goalId={goalId} />;
}

function ReviewsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  return <PendingInputsListPage projectId={useRouteProjectId(fallbackProjectId)} />;
}

function InputRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  const { jobId } = useParams();
  const location = useLocation();
  const selectors = new URLSearchParams(location.search);
  if (isStoryInput(selectors)) return <Navigate replace to={`/project/${encodeURIComponent(projectId)}/jobs/${encodeURIComponent(jobId!)}/story${location.search}`} />;
  return <InputDetailPage key={JSON.stringify([projectId, jobId])} projectId={projectId} />;
}

function CellsRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  return <CellsList key={projectId} projectId={projectId} />;
}

function AdminRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  return <AdminPage key={projectId} projectId={projectId} />;
}

function CellRoute({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  const { cellId } = useParams();
  return <CellPage key={`${projectId}/${cellId}`} projectId={projectId} cellId={cellId!} />;
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
