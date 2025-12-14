import { useEffect, useState } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { AppstoreOutlined, ClusterOutlined, OrderedListOutlined } from '@ant-design/icons';
import { Button, Empty, Form, Input, Layout, Menu, Modal, Select, Space, Typography, message } from 'antd';
import { InputActivityProvider, createProject, listProjects, type Project } from '@vibethis/shared';
import MainView from './components/MainView';
import { KanbanBoard } from '@vibethis/kanban';
import CellsList from './components/CellsList';

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
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [loadingProjects, setLoadingProjects] = useState(false);
  const [creating, setCreating] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [form] = Form.useForm();
  const location = useLocation();
  const navigate = useNavigate();

  const loadProjects = async () => {
    setLoadingProjects(true);
    try {
      const data = await listProjects();
      setProjects(data);
      if (!selectedProjectId && data.length > 0) {
        const savedId = localStorage.getItem('vibethis:selectedProjectId');
        const validSaved = savedId && data.find((p) => p.id === savedId);
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
      localStorage.setItem('vibethis:selectedProjectId', selectedProjectId);
    }
  }, [selectedProjectId]);

  const handleCreateProject = async () => {
    try {
      setCreating(true);
      const values = await form.validateFields();
      const created = await createProject(values);
      message.success('Project created');
      setIsCreateModalOpen(false);
      form.resetFields();
      await loadProjects();
      setSelectedProjectId(created.id);
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message);
      } else {
        message.error('Failed to create project');
      }
    } finally {
      setCreating(false);
    }
  };

  const selectedProject = projects.find((p) => p.id === selectedProjectId) || null;

  const navKey = (() => {
    const path = location.pathname;
    if (path.includes('/kanban')) return 'tickets';
    if (path.includes('/cells/list')) return 'cells-list';
    if (path.includes('/cells') || path.includes('/cell/')) return 'cells-graph';
    return 'tickets';
  })();

  const handleNav = (key: string) => {
    if (!selectedProject) return;
    switch (key) {
      case 'tickets':
        navigate(`/project/${selectedProject.id}/kanban`);
        break;
      case 'cells-graph':
        navigate(`/project/${selectedProject.id}/cells/graph`);
        break;
      case 'cells-list':
        navigate(`/project/${selectedProject.id}/cells/list`);
        break;
      default:
        navigate(`/project/${selectedProject.id}/kanban`);
    }
  };

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ background: '#fff', borderBottom: '1px solid #f0f0f0' }}>
        <Space align="center" style={{ width: '100%', justifyContent: 'space-between' }}>
          <Typography.Text strong>VibeThis</Typography.Text>
          <Space>
            <Select
              style={{ minWidth: 240 }}
              placeholder="Select project"
              loading={loadingProjects}
              value={selectedProjectId || undefined}
              onChange={(value) => setSelectedProjectId(value)}
              options={projects.map((project) => ({
                value: project.id,
                label: project.name,
              }))}
            />
            <Button onClick={loadProjects}>Refresh</Button>
            <Button type="primary" onClick={() => setIsCreateModalOpen(true)}>
              New Project
            </Button>
          </Space>
        </Space>
      </Header>
      <Layout style={{ minHeight: 'calc(100vh - 64px)' }}>
        <Sider width={220} theme="light" collapsedWidth={72}>
          <Menu
            mode="inline"
            selectedKeys={[navKey]}
            items={[
              {
                key: 'tickets',
                label: 'Tickets',
                icon: <AppstoreOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'cells-graph',
                label: 'Cells — Graph',
                icon: <ClusterOutlined />,
                disabled: !selectedProject,
              },
              {
                key: 'cells-list',
                label: 'Cells — List',
                icon: <OrderedListOutlined />,
                disabled: !selectedProject,
              },
            ]}
            onClick={({ key }) => handleNav(key)}
            style={{ height: '100%', borderRight: 0 }}
          />
        </Sider>
        <Content style={{ padding: 16 }}>
          {selectedProject ? (
            <Routes>
              {/* Default route redirects to Kanban */}
              <Route path="/" element={<Navigate to={`/project/${selectedProject.id}/kanban`} replace />} />

              {/* Cells views */}
              <Route path="/cells" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/cells/graph" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/cells/list" element={<CellsList projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cells" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cells/graph" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cells/list" element={<CellsList projectId={selectedProject.id} />} />

              {/* Kanban view */}
              <Route path="/kanban" element={<KanbanBoard projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/kanban" element={<KanbanBoard projectId={selectedProject.id} />} />

              {/* Cell detail routes */}
              <Route path="/cell/:cellId" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/cell/:cellId/:tab" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/cell/:cellId/:tab/:subtab" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cell/:cellId" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cell/:cellId/:tab" element={<MainView projectId={selectedProject.id} />} />
              <Route path="/project/:projectId/cell/:cellId/:tab/:subtab" element={<MainView projectId={selectedProject.id} />} />

              {/* Catch all - redirect to Kanban */}
              <Route path="*" element={<Navigate to={`/project/${selectedProject.id}/kanban`} replace />} />
            </Routes>
          ) : (
            <div style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Empty description="Select or create a project to continue" />
            </div>
          )}
        </Content>
      </Layout>

      <Modal
        title="Create Project"
        open={isCreateModalOpen}
        onCancel={() => setIsCreateModalOpen(false)}
        onOk={handleCreateProject}
        confirmLoading={creating}
        okText="Create"
      >
        <Form layout="vertical" form={form}>
          <Form.Item
            label="Name"
            name="name"
            rules={[{ required: true, message: 'Please enter a project name' }]}
          >
            <Input placeholder="Project name" />
          </Form.Item>
          <Form.Item
            label="Git repository path"
            name="gitRepoPath"
            rules={[{ required: true, message: 'Please enter a git repo path' }]}
          >
            <Input placeholder="/path/to/repo" />
          </Form.Item>
        </Form>
      </Modal>
    </Layout>
  );
}

export default App;
