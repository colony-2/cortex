import { useEffect } from 'react';
import { Form, Input, Modal, message } from 'antd';
import { updateProject, type Project } from '@colony2/shared';

interface ProjectSettingsModalProps {
  project: Project | null;
  open: boolean;
  onClose: () => void;
  onUpdate: () => void;
}

export default function ProjectSettingsModal({ project, open, onClose, onUpdate }: ProjectSettingsModalProps) {
  const [form] = Form.useForm();

  useEffect(() => {
    if (project && open) {
      form.setFieldsValue({
        name: project.name,
        gitRepoPath: project.gitRepoPath,
        gitRepoBranch: project.gitRepoBranch || '',
        defaultTicketRecipe: project.defaultTicketRecipe || '',
      });
    }
  }, [project, open, form]);

  const handleOk = async () => {
    if (!project) return;

    try {
      const values = await form.validateFields();
      await updateProject(project.id, {
        name: values.name,
        gitRepoPath: values.gitRepoPath,
        gitRepoBranch: values.gitRepoBranch || undefined,
        defaultTicketRecipe: values.defaultTicketRecipe || undefined,
      });
      message.success('Project settings updated');
      onUpdate();
      onClose();
    } catch (error) {
      if (error instanceof Error) {
        message.error(error.message);
      } else {
        message.error('Failed to update project settings');
      }
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onClose();
  };

  return (
    <Modal
      title="Project Settings"
      open={open}
      onOk={handleOk}
      onCancel={handleCancel}
      okText="Save"
      width={600}
    >
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
      </Form>
    </Modal>
  );
}
