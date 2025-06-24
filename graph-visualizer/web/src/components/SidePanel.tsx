import { Tabs, Empty, Typography, Input, List } from 'antd';
import { FileOutlined, CodeOutlined, SettingOutlined, HistoryOutlined, ToolOutlined, FileTextOutlined, ContainerOutlined, EditOutlined, CheckSquareOutlined } from '@ant-design/icons';
import FileBrowser from './FileBrowser';
import type { DependencyNode } from '../types';

const { Title } = Typography;
const { TextArea } = Input;

interface SidePanelProps {
  selectedNode: DependencyNode | null;
}

// Claude Code subtabs
const ClaudeCodeTabs = () => {
  const items = [
    {
      key: 'settings',
      label: (
        <span>
          <SettingOutlined />
          Settings
        </span>
      ),
      children: (
        <div style={{ padding: '16px' }}>
          <Title level={5}>Claude Code Settings</Title>
          <Empty description="Settings configuration coming soon" />
        </div>
      ),
    },
    {
      key: 'tools',
      label: (
        <span>
          <ToolOutlined />
          Tools
        </span>
      ),
      children: (
        <div style={{ padding: '16px' }}>
          <Title level={5}>Available Tools</Title>
          <Empty description="Tools configuration coming soon" />
        </div>
      ),
    },
    {
      key: 'instructions',
      label: (
        <span>
          <FileTextOutlined />
          Instructions
        </span>
      ),
      children: (
        <div style={{ padding: '16px' }}>
          <Title level={5}>Custom Instructions</Title>
          <TextArea 
            placeholder="Enter custom instructions for Claude Code..."
            rows={10}
            style={{ marginTop: '16px' }}
          />
        </div>
      ),
    },
  ];

  return (
    <Tabs
      defaultActiveKey="settings"
      items={items}
      tabPosition="left"
      style={{ height: '100%' }}
    />
  );
};

// Configuration subtabs
const ConfigurationTabs = () => {
  const items = [
    {
      key: 'container',
      label: (
        <span>
          <ContainerOutlined />
          Container
        </span>
      ),
      children: (
        <div style={{ padding: '16px' }}>
          <Title level={5}>Container Configuration</Title>
          <Empty description="Container settings coming soon" />
        </div>
      ),
    },
    {
      key: 'notes',
      label: (
        <span>
          <EditOutlined />
          Notes
        </span>
      ),
      children: (
        <div style={{ padding: '16px' }}>
          <Title level={5}>Notes</Title>
          <TextArea 
            placeholder="Add notes about this project..."
            rows={15}
            style={{ marginTop: '16px' }}
          />
        </div>
      ),
    },
    {
      key: 'todo',
      label: (
        <span>
          <CheckSquareOutlined />
          Todo List
        </span>
      ),
      children: (
        <div style={{ padding: '16px' }}>
          <Title level={5}>Todo List</Title>
          <List
            size="small"
            bordered
            dataSource={[]}
            renderItem={(item: string) => <List.Item>{item}</List.Item>}
            locale={{ emptyText: 'No todos yet' }}
            style={{ marginTop: '16px' }}
          />
        </div>
      ),
    },
  ];

  return (
    <Tabs
      defaultActiveKey="container"
      items={items}
      tabPosition="left"
      style={{ height: '100%' }}
    />
  );
};

export default function SidePanel({ selectedNode }: SidePanelProps) {
  const items = selectedNode ? [
    {
      key: 'files',
      label: (
        <span>
          <FileOutlined />
          Files
        </span>
      ),
      children: <FileBrowser node={selectedNode} />,
    },
    {
      key: 'claude-code',
      label: (
        <span>
          <CodeOutlined />
          Claude Code
        </span>
      ),
      children: <ClaudeCodeTabs />,
    },
    {
      key: 'changes',
      label: (
        <span>
          <HistoryOutlined />
          Changes
        </span>
      ),
      children: (
        <div style={{ padding: '24px' }}>
          <Title level={4}>Recent Changes</Title>
          <Empty description="No changes tracked yet" />
        </div>
      ),
    },
  ] : [
    {
      key: 'config',
      label: (
        <span>
          <SettingOutlined />
          Configuration
        </span>
      ),
      children: <ConfigurationTabs />,
    },
  ];

  return (
    <div style={{ height: '100%', background: '#fff', display: 'flex', flexDirection: 'column' }}>
      <Tabs
        defaultActiveKey={selectedNode ? 'files' : 'config'}
        items={items}
        style={{ flex: 1 }}
        tabBarStyle={{ marginBottom: 0, paddingLeft: '16px', paddingRight: '16px' }}
      />
    </div>
  );
}