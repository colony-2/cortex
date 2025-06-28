import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Tabs, Empty, Typography, Input, List } from 'antd';
import { FileOutlined, CodeOutlined, SettingOutlined, HistoryOutlined, ContainerOutlined, EditOutlined, CheckSquareOutlined, AppstoreOutlined, BranchesOutlined } from '@ant-design/icons';
import { FileBrowser } from '@vibethis/files';
import { EnvEditor, RelationshipEditor } from '@vibethis/config';
import { GitChanges } from '@vibethis/changes';
import type { DependencyNode } from '@vibethis/shared';
import { navigateToPath } from '@vibethis/shared';

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
  const { subtab } = useParams<{ subtab?: string }>();
  const navigate = useNavigate();
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
      activeKey={subtab || 'container'}
      onChange={(key) => {
        const path = navigateToPath({ tab: 'config', subtab: key });
        navigate(path);
      }}
      items={items}
      tabPosition="left"
      style={{ height: '100%' }}
    />
  );
};

// Config tabs for selected node
const NodeConfigTabs = ({ node }: { node: DependencyNode | null }) => {
  const { boxId, subtab } = useParams<{ boxId?: string; subtab?: string }>();
  const navigate = useNavigate();
  
  // Use either the node ID or the boxId from URL
  const nodeId = node?.id || boxId;
  const items = [
    {
      key: 'env',
      label: (
        <span>
          <ContainerOutlined />
          Env
        </span>
      ),
      children: node ? <EnvEditor node={node} /> : <div style={{ padding: '16px' }}>Loading...</div>,
    },
    {
      key: 'relationships',
      label: (
        <span>
          <BranchesOutlined />
          Relationships
        </span>
      ),
      children: node ? <RelationshipEditor node={node} /> : <div style={{ padding: '16px' }}>Loading...</div>,
    },
    {
      key: 'claude',
      label: (
        <span>
          <CodeOutlined />
          Claude
        </span>
      ),
      children: <ClaudeCodeContent />,
    },
  ];

  return (
    <Tabs
      activeKey={subtab || 'env'}
      onChange={(key) => {
        if (nodeId) {
          const path = navigateToPath({ boxId: nodeId, tab: 'config', subtab: key });
          navigate(path);
        }
      }}
      items={items}
      tabPosition="left"
      style={{ height: '100%' }}
    />
  );
};

export default function SidePanel({ selectedNode }: SidePanelProps) {
  const { boxId, tab, subtab } = useParams<{ boxId?: string; tab?: string; subtab?: string }>();
  const navigate = useNavigate();
  
  // If we have a boxId in the URL, we should show node-specific tabs
  const showNodeTabs = !!boxId;
  
  // Initialize activeTab from URL or default based on whether we're showing node tabs
  const defaultTab = showNodeTabs ? 'files' : 'config';
  const [activeTab, setActiveTab] = useState<string>(tab || defaultTab);

  // Update active tab when URL changes
  useEffect(() => {
    if (tab) {
      setActiveTab(tab);
    } else if (showNodeTabs) {
      setActiveTab('files');
    } else {
      setActiveTab('config');
    }
  }, [tab, showNodeTabs]);

  const handleTabChange = (key: string) => {
    setActiveTab(key);
    if (boxId) {
      const path = navigateToPath({ boxId, tab: key });
      navigate(path);
    }
  };

  const handleGitTabChange = (gitTab: string) => {
    if (boxId) {
      const path = navigateToPath({ boxId, tab: 'changes', subtab: gitTab });
      navigate(path);
    }
  };
  
  // Show node-specific tabs if we have a boxId OR selectedNode
  const items = (showNodeTabs || selectedNode) ? [
    {
      key: 'files',
      label: (
        <span>
          <FileOutlined />
          Files
        </span>
      ),
      children: <FileBrowser node={selectedNode} boxId={boxId} />,
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
      children: <GitChanges node={selectedNode} activeTab={subtab} onTabChange={handleGitTabChange} />,
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
        activeKey={activeTab}
        onChange={handleTabChange}
        items={items}
        style={{ flex: 1 }}
        tabBarStyle={{ marginBottom: 0, paddingLeft: '16px', paddingRight: '16px' }}
      />
    </div>
  );
}