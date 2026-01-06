import { UserOutlined, SwapOutlined } from '@ant-design/icons';
import { Dropdown, Space, Typography, Menu, Select, Divider } from 'antd';
import type { Project } from '@colony2/shared';

interface UserDropdownProps {
  projects: Project[];
  selectedProjectId: string | null;
  loadingProjects: boolean;
  onProjectChange: (projectId: string) => void;
  onRefreshProjects: () => void;
}

export default function UserDropdown({
  projects,
  selectedProjectId,
  loadingProjects,
  onProjectChange,
  onRefreshProjects,
}: UserDropdownProps) {
  const menuItems = [
    {
      key: 'project-switcher',
      label: (
        <div onClick={(e) => e.stopPropagation()}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            Switch Project
          </Typography.Text>
          <Select
            style={{ width: '100%', marginTop: 8 }}
            placeholder="Select project"
            loading={loadingProjects}
            value={selectedProjectId || undefined}
            onChange={onProjectChange}
            options={projects.map((project) => ({
              value: project.id,
              label: project.name,
            }))}
          />
        </div>
      ),
    },
    {
      type: 'divider' as const,
    },
    {
      key: 'refresh',
      icon: <SwapOutlined />,
      label: 'Refresh Projects',
      onClick: onRefreshProjects,
    },
  ];

  const selectedProject = projects.find((p) => p.id === selectedProjectId);

  return (
    <Dropdown
      menu={{ items: menuItems }}
      trigger={['click']}
      placement="bottomRight"
    >
      <Space style={{ cursor: 'pointer' }}>
        <UserOutlined style={{ fontSize: 16 }} />
        <Typography.Text>
          {selectedProject ? selectedProject.name : 'User'}
        </Typography.Text>
      </Space>
    </Dropdown>
  );
}
