import { useEffect, useState } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation, Link, useParams } from 'react-router-dom';
import { AppstoreOutlined, FileTextOutlined, OrderedListOutlined, SettingOutlined, ThunderboltOutlined, ProjectOutlined, PlusOutlined, FormOutlined } from '@ant-design/icons';
import { Badge, Button, Empty, Layout, Menu, message, Space, Typography } from 'antd';
import { InputActivityProvider, listProjects, type Project, isAuthenticated, clearUserEmail, CreateTicketModal, useInputActivity } from '@colony2/shared';
import { KanbanBoard } from '@colony2/kanban';
import { RecipeNotebookPage } from '@colony2/notebook';
import CellsList from './components/CellsList';
import CellDetailPage from './components/CellDetailPage';
import ProjectSettingsPage from './components/ProjectSettingsPage';
import WorkflowListPage from './components/WorkflowListPage';
import WorkflowDetailPage from './components/WorkflowDetailPage';
import WorkflowStoryPage from './components/WorkflowStoryPage';
import RecipeListPage from './components/RecipeListPage';
import RecipeDetailPage from './components/RecipeDetailPage';
import PendingInputsListPage from './components/PendingInputsListPage';
import InputDetailPage from './components/InputDetailPage';
import UserDropdown from './components/UserDropdown';
import ProjectAdminPage from './components/ProjectAdminPage';
import LoginPage from './components/LoginPage';

const { Header, Content, Sider } = Layout;

function App() {
  return (
    <BrowserRouter>
      <InputActivityProvider>
        <AppShell />
      </InputActivityProvider>
    </BrowserRouter>
  );
}

function AppShell() {
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [loadingProjects, setLoadingProjects] = useState(false);
  const [isCreateTicketOpen, setIsCreateTicketOpen] = useState(false);
  const location = useLocation();
  const { pendingCount, setCurrentProjectId } = useInputActivity();

  if (!authenticated) {
    return <LoginPage onLogin={() => setAuthenticated(true)} />;
  }

  const loadProjects = async () => {
    setLoadingProjects(true);
    try {
      const data = await listProjects();
      setProjects(data);
      if (!selectedProjectId && data.length > 0) {
        const savedId = localStorage.getItem('colony2:selectedProjectId');
        const validSaved = savedId && data.find((p: Project) => p.id === savedId);
        setSelectedProjectId((validSaved && savedId) || data[0].id);
      }
    } catch (error) {
      console.error('Failed to load projects', error);
      message.error('Failed to load projects');
    } finally {
      setLoadingProjects(false);
    }
  };

  useEffect(() => {
    loadProjects();
  }, []);

  useEffect(() => {
    if (selectedProjectId) {
      localStorage.setItem('colony2:selectedProjectId', selectedProjectId);
      // Update InputActivity context with current project
      setCurrentProjectId(selectedProjectId);
    } else {
      setCurrentProjectId(null);
    }
  }, [selectedProjectId, setCurrentProjectId]);

  const selectedProject = projects.find((p) => p.id === selectedProjectId) || null;

  useEffect(() => {
    if (selectedProject) {
      document.title = `colony2: ${selectedProject.name}`;
    } else {
      document.title = 'colony2';
    }
  }, [selectedProject]);

  const navKey = (() => {
    const path = location.pathname;
    if (path.includes('/admin/projects')) return 'project-admin';
    if (path.includes('/settings')) return 'settings';
    if (path.includes('/recipes/notebook')) return 'recipe-notebook';
    if (path.includes('/recipes')) return 'recipes';
    if (path.includes('/workflows')) return 'workflows';
    if (path.includes('/inputs')) return 'inputs';
    if (path.includes('/kanban')) return 'tickets';
    if (path.includes('/cells') || path.includes('/cell/')) return 'cells';
    return 'tickets';
  })();

  const handleLogout = () => {
    clearUserEmail();
    setAuthenticated(false);
  };

  const handleNewTicket = () => {
    if (!selectedProject) return;
    setIsCreateTicketOpen(true);
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#fff', borderBottom: '1px solid #f0f0f0' }}>
        <Space align="center" style={{ width: '100%', justifyContent: 'space-between' }}>
          <Typography.Text strong>colony2</Typography.Text>
          <Space>
            {selectedProject && (
              <Button type="primary" icon={<PlusOutlined />} onClick={handleNewTicket}>
                New Ticket
              </Button>
            )}
            <UserDropdown
              projects={projects}
              selectedProjectId={selectedProjectId}
              loadingProjects={loadingProjects}
              onProjectChange={setSelectedProjectId}
              onRefreshProjects={loadProjects}
              onLogout={handleLogout}
            />
          </Space>
        </Space>
      </Header>
      <Layout style={{ height: 'calc(100vh - 64px)' }}>
        <Sider width={220} theme="light" collapsedWidth={72}>
          <Menu
            mode="inline"
            selectedKeys={[navKey]}
            items={[
              {
                key: 'tickets',
                label: selectedProject ? (
                  <Link to={`/project/${selectedProject.id}/kanban`}>Tickets</Link>
                ) : 'Tickets',
                icon: <AppstoreOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'inputs',
                label: selectedProject ? (
                  <Badge count={pendingCount} offset={[10, 0]} size="small">
                    <Link to={`/project/${selectedProject.id}/inputs`}>Pending Inputs</Link>
                  </Badge>
                ) : 'Pending Inputs',
                icon: <FormOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'workflows',
                label: selectedProject ? (
                  <Link to={`/project/${selectedProject.id}/workflows`}>Workflows</Link>
                ) : 'Workflows',
                icon: <ThunderboltOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'recipes',
                label: selectedProject ? (
                  <Link to={`/project/${selectedProject.id}/recipes`}>Recipes</Link>
                ) : 'Recipes',
                icon: <FileTextOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'recipe-notebook',
                label: selectedProject ? (
                  <Link to={`/project/${selectedProject.id}/recipes/notebook`}>Recipe Notebook</Link>
                ) : 'Recipe Notebook',
                icon: <FileTextOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'cells',
                label: selectedProject ? (
                  <Link to={`/project/${selectedProject.id}/cells`}>Cells</Link>
                ) : 'Cells',
                icon: <OrderedListOutlined />,
                disabled: !selectedProject,
              },
              {
                type: 'divider',
              },
              {
                key: 'settings',
                label: selectedProject ? (
                  <Link to={`/project/${selectedProject.id}/settings`}>Settings</Link>
                ) : 'Settings',
                icon: <SettingOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'project-admin',
                label: <Link to="/admin/projects">Project Admin</Link>,
                icon: <ProjectOutlined />,
              },
            ]}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content style={{ padding: 16, overflow: 'auto' }}>
          <Routes>
            {/* Project Admin route - available without project selection */}
            <Route path="/admin/projects" element={<ProjectAdminPage />} />

            {selectedProject ? (
              <>
                {/* Default route redirects to Kanban */}
                <Route path="/" element={<Navigate to={`/project/${selectedProject.id}/kanban`} replace />} />

                {/* Settings */}
                <Route path="/project/:projectId/settings" element={<ProjectSettingsPage project={selectedProject} onUpdate={loadProjects} />} />

                {/* Cells views */}
                <Route path="/cells" element={<CellsList projectId={selectedProject.id} />} />
                <Route path="/project/:projectId/cells" element={<CellsList projectId={selectedProject.id} />} />

              {/* Kanban view */}
              <Route path="/kanban" element={<KanbanBoard projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/kanban" element={<KanbanBoard projectId={selectedProject.id} />} />

              {/* Pending Inputs views */}
              <Route path="/inputs" element={<PendingInputsListPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/inputs" element={<PendingInputsListPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/inputs/:jobId" element={<InputDetailPage projectId={selectedProject.id} />} />

              {/* Workflow views */}
              <Route path="/workflows" element={<WorkflowListPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/workflows" element={<WorkflowListPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/workflows/:workflowId" element={<WorkflowDetailPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/workflows/:workflowId/story" element={<WorkflowStoryPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/workflows/:workflowId/notebook" element={<WorkflowNotebookRoute projectId={selectedProject.id} />} />

              {/* Recipe views */}
              <Route path="/recipes" element={<RecipeListPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/recipes" element={<RecipeListPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/recipes/notebook" element={<RecipeNotebookPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/recipes/*" element={<RecipeDetailPage projectId={selectedProject.id} />} />

              {/* Cell detail routes */}
              <Route path="/cell/:cellId" element={<CellDetailPage projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cell/:cellId" element={<CellDetailPage projectId={selectedProject.id} />} />

                {/* Catch all - redirect to Kanban */}
                <Route path="*" element={<Navigate to={`/project/${selectedProject.id}/kanban`} replace />} />
              </>
            ) : (
              <>
                {/* No project selected - show empty state for non-admin routes */}
                <Route path="*" element={
                  <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Empty description="Select or create a project to continue" />
                  </div>
                } />
              </>
            )}
          </Routes>
        </Content>
      </Layout>

      {selectedProject && (
        <CreateTicketModal
          open={isCreateTicketOpen}
          onClose={() => setIsCreateTicketOpen(false)}
          projectId={selectedProject.id}
        />
      )}
    </Layout>
  );
}

function WorkflowNotebookRoute(props: { projectId: string }): JSX.Element {
  const { workflowId } = useParams<{ workflowId: string }>();
  if (!workflowId) {
    return <Empty description="Missing workflowId" />;
  }
  return (
    <RecipeNotebookPage
      projectId={props.projectId}
      initial={{ mode: 'monitor', backendMode: 'real', jobId: workflowId, autoLoadJob: true }}
    />
  );
}

export default App;
