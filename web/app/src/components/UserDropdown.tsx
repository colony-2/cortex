import { UserOutlined, SwapOutlined, LogoutOutlined } from '@ant-design/icons';
import { Dropdown, Space, Typography, Select } from 'antd';
import type { Project } from '@colony2/shared';
import { getUserEmail } from '@colony2/shared';

interface UserDropdownProps {
  projects: Project[];
  selectedProjectId: string | null;
  loadingProjects: boolean;
  onProjectChange: (projectId: string) => void;
  onRefreshProjects: () => void;
  onLogout: () => void;
}

export default function UserDropdown({
  projects,
  selectedProjectId,
  loadingProjects,
  onProjectChange,
  onRefreshProjects,
  onLogout,
}: UserDropdownProps) {
  const userEmail = getUserEmail();

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
    {
      type: 'divider' as const,
    },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: 'Logout',
      onClick: onLogout,
      danger: true,
    },
  ];

  return (
    <Dropdown
      menu={{ items: menuItems }}
      trigger={['click']}
      placement="bottomRight"
    >
      <Space style={{ cursor: 'pointer' }}>
        <UserOutlined style={{ fontSize: 16 }} />
        <Typography.Text>
          {userEmail || 'User'}
        </Typography.Text>
      </Space>
    </Dropdown>
  );
}
