import { useState, useEffect } from 'react';
import { DeleteOutlined, PlusOutlined, ReloadOutlined, EditOutlined } from '@ant-design/icons';
import { Button, Table, Space, Modal, Form, Input, message, Popconfirm, Typography } from 'antd';
import { listProjects, createProject, deleteProject, updateProject, type Project } from '@colony2/shared';

export default function ProjectAdminPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [editingProject, setEditingProject] = useState<Project | null>(null);
  const [creating, setCreating] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [createForm] = Form.useForm();
  const [editForm] = Form.useForm();

  const loadProjects = async () => {
    setLoading(true);
    try {
      const data = await listProjects();
      setProjects(data);
    } catch (error) {
      console.error('Failed to load projects', error);
      message.error('Failed to load projects');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProjects();
  }, []);

  const handleCreateProject = async () => {
    try {
      setCreating(true);
      const values = await createForm.validateFields();
      await createProject(values);
      message.success('Project created successfully');
      setIsCreateModalOpen(false);
      createForm.resetFields();
      await loadProjects();
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

  const handleEditProject = (project: Project) => {
    setEditingProject(project);
    editForm.setFieldsValue({
      name: project.name,
      gitRepoPath: project.gitRepoPath,
      gitRepoBranch: project.gitRepoBranch || '',
      defaultTicketRecipe: project.defaultTicketRecipe || '',
    });
    setIsEditModalOpen(true);
  };

  const handleUpdateProject = async () => {
    if (!editingProject) return;

    try {
      setUpdating(true);
      const values = await editForm.validateFields();
      await updateProject(editingProject.id, values);
      message.success('Project updated successfully');
      setIsEditModalOpen(false);
      setEditingProject(null);
      editForm.resetFields();
      await loadProjects();
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message);
      } else {
        message.error('Failed to update project');
      }
    } finally {
      setUpdating(false);
    }
  };

  const handleDeleteProject = async (projectId: string) => {
    try {
      await deleteProject(projectId);
      message.success('Project deleted successfully');
      await loadProjects();
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message);
      } else {
        message.error('Failed to delete project');
      }
    }
  };

  const columns = [
    {
      title: 'Project Name',
      dataIndex: 'name',
      key: 'name',
      width: '25%',
      render: (text: string) => <Typography.Text strong>{text}</Typography.Text>,
    },
    {
      title: 'Git Repository Path',
      dataIndex: 'gitRepoPath',
      key: 'gitRepoPath',
      width: '30%',
      ellipsis: true,
    },
    {
      title: 'Branch',
      dataIndex: 'gitRepoBranch',
      key: 'gitRepoBranch',
      width: '15%',
      render: (text: string) => text || <Typography.Text type="secondary">-</Typography.Text>,
    },
    {
      title: 'Default Recipe',
      dataIndex: 'defaultTicketRecipe',
      key: 'defaultTicketRecipe',
      width: '15%',
      ellipsis: true,
      render: (text: string) => text || <Typography.Text type="secondary">-</Typography.Text>,
    },
    {
      title: 'Actions',
      key: 'actions',
      width: '15%',
      render: (_: unknown, record: Project) => (
        <Space>
          <Button
            icon={<EditOutlined />}
            size="small"
            onClick={() => handleEditProject(record)}
          >
            Edit
          </Button>
          <Popconfirm
            title="Delete Project"
            description="Are you sure you want to delete this project? This action cannot be undone."
            onConfirm={() => handleDeleteProject(record.id)}
            okText="Delete"
            cancelText="Cancel"
            okButtonProps={{ danger: true }}
          >
            <Button icon={<DeleteOutlined />} size="small" danger>
              Delete
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography.Title level={3} style={{ margin: 0 }}>
          Project Administration
        </Typography.Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={loadProjects} loading={loading}>
            Refresh
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setIsCreateModalOpen(true)}>
            New Project
          </Button>
        </Space>
      </div>

      <Table
        columns={columns}
        dataSource={projects}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />

      <Modal
        title="Create New Project"
        open={isCreateModalOpen}
        onCancel={() => {
          setIsCreateModalOpen(false);
          createForm.resetFields();
        }}
        onOk={handleCreateProject}
        confirmLoading={creating}
        okText="Create"
      >
        <Form layout="vertical" form={createForm}>
          <Form.Item
            label="Project Name"
            name="name"
            rules={[{ required: true, message: 'Please enter a project name' }]}
          >
            <Input placeholder="My Project" />
          </Form.Item>
          <Form.Item
            label="Git Repository Path"
            name="gitRepoPath"
            rules={[{ required: true, message: 'Please enter a git repository path' }]}
          >
            <Input placeholder="/path/to/repo" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="Edit Project"
        open={isEditModalOpen}
        onCancel={() => {
          setIsEditModalOpen(false);
          setEditingProject(null);
          editForm.resetFields();
        }}
        onOk={handleUpdateProject}
        confirmLoading={updating}
        okText="Update"
      >
        <Form layout="vertical" form={editForm}>
          <Form.Item
            label="Project Name"
            name="name"
            rules={[{ required: true, message: 'Please enter a project name' }]}
          >
            <Input placeholder="My Project" />
          </Form.Item>
          <Form.Item
            label="Git Repository Path"
            name="gitRepoPath"
            rules={[{ required: true, message: 'Please enter a git repository path' }]}
          >
            <Input placeholder="/path/to/repo" />
          </Form.Item>
          <Form.Item
            label="Git Branch"
            name="gitRepoBranch"
          >
            <Input placeholder="main" />
          </Form.Item>
          <Form.Item
            label="Default Ticket Recipe"
            name="defaultTicketRecipe"
          >
            <Input placeholder="Recipe name" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
