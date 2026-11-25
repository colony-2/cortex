import { useEffect, useRef, useState, type CSSProperties } from 'react';
import { BrowserRouter, Link, matchPath, Navigate, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom';
import { CloudServerOutlined, MenuFoldOutlined, MenuUnfoldOutlined, MenuOutlined, SearchOutlined, SettingOutlined } from '@ant-design/icons';
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
import JobStoryPage from './components/JobStoryPage';
import { useCompact } from './hooks/useCompact';
import AccountMenu from './components/AccountMenu';
import LoginPage from './components/LoginPage';
import InboxNavigation from './components/InboxNavigation';
import { createRequestId } from '@colony2/shared/cellProtocol';
import { useSavedMessageDrafts } from './hooks/useMessageDrafts';
import { useSavedReviewDrafts } from './hooks/useReviewDraft';
import ComposeJob from './components/ComposeJob';
import { ReviewReplyWindow } from './components/ReviewReply';
import { ReviewReplyDock, reviewReplyKey, type ReviewReplyTarget } from './hooks/useReviewReplyDock';
import InboxDraftsPage from './components/InboxDraftsPage';
import { JobInboxProvider } from './hooks/useJobInbox';
import { reviewInboxPath } from './utils/reviewDocumentLocation';
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
type DockEntry = { id:string; tenant:string; review?:ReviewReplyTarget };
const sidebarWidth = 256;
const railWidth = 76;

export function AppShell({ defaultTenantId }: { defaultTenantId: string }) {
  const location = useLocation();
  const [identity, setIdentity] = useState(getUserEmail);
  useEffect(() => {
    const changed = () => setIdentity(getUserEmail());
    window.addEventListener('cortex-ui-identity', changed);
    return () => window.removeEventListener('cortex-ui-identity', changed);
  }, []);
  const tenant = initialTenantId(location.search, location.pathname, defaultTenantId);
  return <JobInboxProvider key={identity ?? ''} tenant={tenant}><InboxShell defaultTenantId={defaultTenantId} /></JobInboxProvider>;
}
function InboxShell({ defaultTenantId }: { defaultTenantId: string }) {
  const compact = useCompact();
  const location = useLocation();
  const selectors = new URLSearchParams(location.search);
  const useNavigationDrawer = compact;
  const tenantId = initialTenantId(location.search, location.pathname, defaultTenantId);
  const [collapsed, setCollapsed] = useState(false);
  const [composers, setComposers] = useState<DockEntry[]>([]);
  const visibleComposers = composers.filter(entry => entry.review || entry.tenant === tenantId);
  const [sendingComposers, setSendingComposers] = useState<Set<string>>(() => new Set());
  const [activeComposer, setActiveComposer] = useState<string>();
  const composerDock = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const dock = composerDock.current;
    const active = dock?.querySelector('.inbox-composer.active');
    if (!dock || !active || dock.scrollWidth <= dock.clientWidth) return;
    const offset = active.getBoundingClientRect().right - dock.getBoundingClientRect().right;
    if (offset > 0) dock.scrollLeft += offset;
    const left = active.getBoundingClientRect().left - dock.getBoundingClientRect().left;
    if (left < 0) dock.scrollLeft += left;
  }, [activeComposer, visibleComposers.length]);
  const openComposer = (id: string, review?:ReviewReplyTarget) => {
    setComposers(previous => previous.some(entry => entry.id === id) ? previous : [...previous, {id,tenant:review?.tenant ?? tenantId,review}]);
    setActiveComposer(id);
  };
  const closeComposer = (id: string) => {
    setComposers(previous => previous.filter(entry => entry.id !== id));
    setActiveComposer(previous => previous === id ? undefined : previous);
  };
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);
  useEffect(() => { setMenuOpen(false); }, [useNavigationDrawer, tenantId]);
  useEffect(() => { setCollapsed(false); }, [tenantId]);
  const closeMenu = () => {
    if (!useNavigationDrawer) return;
    setMenuOpen(false);
    requestAnimationFrame(() => menuButton.current?.focus({ preventScroll: true }));
  };
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const navigate = useNavigate();
  const { setCurrentProjectId } = useInputActivity();
  const messageDrafts = useSavedMessageDrafts(tenantId);
  const reviewDrafts = useSavedReviewDrafts(tenantId);
  const showDrafts = selectors.get('inbox') === 'drafts';
  const showThreadList = !showDrafts && !!(matchPath('/jobs', location.pathname) || matchPath('/project/:projectId/jobs', location.pathname));
  const searchQuery = selectors.get('q') ?? '';
  const [searchDraft, setSearchDraft] = useState(searchQuery);
  useEffect(() => setSearchDraft(searchQuery), [location.pathname, searchQuery]);

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

  const applyTenant = (tenant: string) => {
    navigate(reviewInboxPath(tenant.trim() || defaultTenantId, ''));
  };

  const searchThreads = (query: string) => {
    const params = showThreadList ? new URLSearchParams(location.search) : new URLSearchParams();
    if (query) params.set('q', query);
    else params.delete('q');
    navigate({ pathname: showThreadList ? location.pathname : `/project/${encodeURIComponent(tenantId)}/jobs`, search: params.toString() }, { replace: showThreadList });
  };

  const handleLogout = () => {
    clearUserEmail();
    setAuthenticated(false);
  };

  const rail = !useNavigationDrawer && collapsed;
  const navigation = <InboxNavigation key={tenantId} rail={rail} draftCount={messageDrafts.length + reviewDrafts.length} onCompose={() => openComposer(createRequestId())} onNavigate={closeMenu}
    onExpand={() => setCollapsed(false)} />;
  const draftsPage = <InboxDraftsPage tenant={tenantId} messages={messageDrafts} reviews={reviewDrafts} sending={sendingComposers}
    openComposer={openComposer} closeComposer={closeComposer} />;
  const markSending = (id:string, sending:boolean) => setSendingComposers(previous => {
    const next = new Set(previous);
    if (sending) next.add(id); else next.delete(id);
    return next;
  });
  const activeId = visibleComposers.find(entry => entry.id === activeComposer)?.id ?? visibleComposers[visibleComposers.length-1]?.id;
  const backgroundProps = compact && activeId ? { inert: '' } : {};
  return <ReviewReplyDock.Provider value={{
    open: target => openComposer(reviewReplyKey(target),target),
    has: id => composers.some(entry => entry.id === id),
    activate: setActiveComposer,
  }}>
    <Layout className={`app-shell${rail ? ' inbox-rail' : ''}`}
      style={{ '--inbox-sidebar-width': useNavigationDrawer ? 'auto' : `${rail ? railWidth : sidebarWidth}px` } as CSSProperties}>
      <Header className="app-header" {...backgroundProps}>
        <div className="app-brand">
          <Button type="text" className="inbox-navigation-toggle" ref={menuButton}
            aria-label={useNavigationDrawer ? 'Open navigation' : rail ? 'Expand navigation' : 'Collapse navigation'}
            aria-expanded={useNavigationDrawer ? menuOpen : !rail}
            icon={useNavigationDrawer ? <MenuOutlined /> : rail ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => useNavigationDrawer ? setMenuOpen(true) : setCollapsed(value => !value)} />
          <Typography.Text strong>cortex</Typography.Text>
        </div>
        <Input className="inbox-thread-search" prefix={<SearchOutlined aria-hidden />} type="search" aria-label="Search threads" placeholder="Search threads"
          allowClear value={showThreadList ? searchQuery : searchDraft}
          onChange={event => showThreadList ? searchThreads(event.target.value) : setSearchDraft(event.target.value)}
          onPressEnter={() => { if (!showThreadList && searchDraft.trim()) searchThreads(searchDraft.trim()); }} />
        <div className="inbox-header-actions">
          <Tooltip title="Runners"><Link className="inbox-settings" aria-label="Runners" to={`/project/${encodeURIComponent(tenantId)}/runners`}><CloudServerOutlined /></Link></Tooltip>
          <Tooltip title="Settings"><Link className="inbox-settings" aria-label="Settings" to={`/project/${encodeURIComponent(tenantId)}/admin`}><SettingOutlined /></Link></Tooltip>
          <AccountMenu tenant={tenantId} onSwitchTenant={applyTenant} onLogout={handleLogout} />
        </div>
      </Header>
      <Layout className="app-body" {...backgroundProps}>
        {useNavigationDrawer ? <Drawer title="Navigation" width="min(320px, 100%)" open={menuOpen} onClose={closeMenu} afterOpenChange={open => {
          // rc-drawer restores its captured element after this callback. Restore
          // our explicit opener afterwards, including interrupted animations.
          if (!open) queueMicrotask(() => menuButton.current?.focus({ preventScroll: true }));
        }}>{navigation}</Drawer>
          : <Sider className="desktop-navigation" width={sidebarWidth} collapsedWidth={railWidth} collapsed={rail} theme="light" trigger={null}>{navigation}</Sider>}
        <Content className="app-content">
          <Routes key={initialTenantId(location.search, location.pathname, defaultTenantId)}>
            <Route path="/" element={<Navigate to={reviewInboxPath(tenantId, '')} replace />} />
            <Route path="/jobs" element={showDrafts ? draftsPage : <JobsListPage key={tenantId} projectId={tenantId} />} />
            <Route path="/project/:projectId/jobs" element={showDrafts ? draftsPage : <JobsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/goals" element={<Navigate to={`/project/${encodeURIComponent(tenantId)}/jobs`} replace />} />
            <Route path="/project/:projectId/goals/:goalId" element={<Alert type="info" message="Goal links have been retired" description={<Link to={`/project/${encodeURIComponent(tenantId)}/jobs`}>Open All jobs</Link>} />} />
            <Route path="/project/:projectId/jobs/:jobId/tree" element={<TreePage />} />
            <Route path="/project/:projectId/jobs/:jobId/story" element={<StoryRoute fallbackProjectId={tenantId} />} />
            <Route path="/reviews" element={<LegacyReviewRedirect fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/reviews" element={<LegacyReviewRedirect fallbackProjectId={tenantId} />} />
            <Route path={reviewDetailPath} element={<LegacyReviewRedirect fallbackProjectId={tenantId} />} />
            <Route path="/cells" element={<CellsList key={tenantId} projectId={tenantId} />} />
            <Route path="/project/:projectId/cells/:cellId" element={<CellRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/cells" element={<CellsRoute fallbackProjectId={tenantId} />} />
            <Route path="/project/:projectId/runners/*" element={<RunnersPage />} />
            <Route path="/project/:projectId/admin" element={<AdminRoute fallbackProjectId={tenantId} />} />
            <Route path="*" element={<Navigate to={reviewInboxPath(tenantId, '')} replace />} />
          </Routes>
        </Content>
      </Layout>
      {!!visibleComposers.length && <div ref={composerDock} className="inbox-composer-dock">
        {visibleComposers.map(({id,review}) => review ? <ReviewReplyWindow key={id} target={review} active={activeId === id}
          onActivate={() => setActiveComposer(id)} onClose={() => closeComposer(id)} onSendingChange={sending=>markSending(id,sending)} />
          : <ComposeJob key={id} id={id} open active={activeId === id} onActivate={() => setActiveComposer(id)}
            onClose={() => closeComposer(id)} onRemove={() => closeComposer(id)} onSendingChange={sending=>markSending(id,sending)} />)}
      </div>}
    </Layout>
  </ReviewReplyDock.Provider>;
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

function LegacyReviewRedirect({ fallbackProjectId }: { fallbackProjectId: string }) {
  const projectId = useRouteProjectId(fallbackProjectId);
  const { jobId } = useParams();
  const location = useLocation();
  const selectors = new URLSearchParams(location.search);
  const path = `/project/${encodeURIComponent(projectId)}/jobs`;
  if (jobId) selectors.set('input','1'); else selectors.set('inbox','reviews');
  const state = jobId ? { inboxReturnTo: reviewInboxPath(projectId, location.search), ...location.state } : location.state;
  return <Navigate replace state={state} to={{pathname:jobId ? `${path}/${encodeURIComponent(jobId)}/story` : path, search:`?${selectors}`, hash:location.hash}} />;
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
