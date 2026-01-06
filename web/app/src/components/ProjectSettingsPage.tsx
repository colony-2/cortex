import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Input, Button, Card, Typography, Space, message } from 'antd';
import { SaveOutlined, CloseOutlined } from '@ant-design/icons';
import { updateProject, type Project } from '@colony2/shared';

interface ProjectSettingsPageProps {
  project: Project | null;
  onUpdate: () => void;
}

export default function ProjectSettingsPage({ project, onUpdate }: ProjectSettingsPageProps) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    if (project) {
      form.setFieldsValue({
        name: project.name,
        gitRepoPath: project.gitRepoPath,
        gitRepoBranch: project.gitRepoBranch || '',
        defaultTicketRecipe: project.defaultTicketRecipe || '',
      });
    }
  }, [project, form]);

  const handleSave = async () => {
    if (!project) return;

    try {
      setLoading(true);
      const values = await form.validateFields();
      await updateProject(project.id, {
        name: values.name,
        gitRepoPath: values.gitRepoPath,
        gitRepoBranch: values.gitRepoBranch || undefined,
        defaultTicketRecipe: values.defaultTicketRecipe || undefined,
      });
      message.success('Project settings updated');
      onUpdate();
      navigate(`/project/${project.id}/kanban`);
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message);
      } else {
        message.error('Failed to update project settings');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = () => {
    form.resetFields();
    if (project) {
      navigate(`/project/${project.id}/kanban`);
    }
  };

  if (!project) {
    return (
      <div style={{ padding: 24, textAlign: 'center' }}>
        <Typography.Text type="secondary">No project selected</Typography.Text>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 800, margin: '0 auto' }}>
      <div style={{ marginBottom: 24 }}>
        <Typography.Title level={3}>Project Settings</Typography.Title>
        <Typography.Text type="secondary">
          Configure settings for {project.name}
        </Typography.Text>
      </div>

      <Card>
        <Form layout="vertical" form={form}>
          <Form.Item
            label="Project Name"
            name="name"
            rules={[{ required: true, message: 'Please enter a project name' }]}
          >
            <Input placeholder="Project name" />
          </Form.Item>

          <Form.Item
            label="Git Repository Path"
            name="gitRepoPath"
            rules={[{ required: true, message: 'Please enter a git repo path' }]}
          >
            <Input placeholder="/path/to/repo" />
          </Form.Item>

          <Form.Item
            label="Git Branch"
            name="gitRepoBranch"
            extra="Optional: specify the git branch to use"
          >
            <Input placeholder="main" />
          </Form.Item>

          <Form.Item
            label="Default Ticket Recipe"
            name="defaultTicketRecipe"
            extra="Optional: default recipe for new tickets"
          >
            <Input placeholder="default-recipe" />
          </Form.Item>

          <Form.Item>
            <Space>
              <Button
                type="primary"
                icon={<SaveOutlined />}
                onClick={handleSave}
                loading={loading}
              >
                Save Changes
              </Button>
              <Button icon={<CloseOutlined />} onClick={handleCancel}>
                Cancel
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
}
