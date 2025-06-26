import { Tabs, Empty, Typography, Input, List } from 'antd';
import { FileOutlined, CodeOutlined, SettingOutlined, HistoryOutlined, ContainerOutlined, EditOutlined, CheckSquareOutlined, AppstoreOutlined, BranchesOutlined } from '@ant-design/icons';
import FileBrowser from './FileBrowser';
import EnvEditor from './EnvEditor';
import DependencyEditor from './DependencyEditor';
import GitChanges from './GitChanges';
import type { DependencyNode } from '../types';

const { Title } = Typography;
const { TextArea } = Input;

interface SidePanelProps {
  selectedNode: DependencyNode | null;
}

// Claude Code subtab content with section headers
const ClaudeCodeContent = () => {
  return (
    <div style={{ padding: '16px', height: '100%', overflowY: 'auto' }}>
      <div style={{ marginBottom: '32px' }}>
        <Title level={4}>Claude Code Settings</Title>
        <Empty description="Settings configuration coming soon" />
      </div>
      
      <div style={{ marginBottom: '32px' }}>
        <Title level={4}>Available Tools</Title>
        <Empty description="Tools configuration coming soon" />
      </div>
      
      <div style={{ marginBottom: '32px' }}>
        <Title level={4}>Custom Instructions</Title>
        <TextArea 
          placeholder="Enter custom instructions for Claude Code..."
          rows={10}
          style={{ marginTop: '16px' }}
        />
      </div>
    </div>
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

// Config tabs for selected node
const NodeConfigTabs = ({ node }: { node: DependencyNode }) => {
  const items = [
    {
      key: 'claude-code',
      label: (
        <span>
          <CodeOutlined />
          Claude Code
        </span>
      ),
      children: <ClaudeCodeContent />,
    },
    {
      key: 'env',
      label: (
        <span>
          <ContainerOutlined />
          Env
        </span>
      ),
      children: <EnvEditor node={node} />,
    },
    {
      key: 'dependencies',
      label: (
        <span>
          <BranchesOutlined />
          Dependencies
        </span>
      ),
      children: <DependencyEditor node={node} />,
    },
  ];

  return (
    <Tabs
      defaultActiveKey="claude-code"
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
      key: 'config',
      label: (
        <span>
          <AppstoreOutlined />
          Config
        </span>
      ),
      children: <NodeConfigTabs node={selectedNode} />,
    },
    {
      key: 'changes',
      label: (
        <span>
          <HistoryOutlined />
          Changes
        </span>
      ),
      children: <GitChanges node={selectedNode} />,
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
        defaultActiveKey="config"
        items={items}
        style={{ flex: 1 }}
        tabBarStyle={{ marginBottom: 0, paddingLeft: '16px', paddingRight: '16px' }}
      />
    </div>
  );
}