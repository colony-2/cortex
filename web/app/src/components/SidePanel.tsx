import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Tabs, Empty, Typography, Input, List, Badge } from 'antd';
import { FileOutlined, CodeOutlined, SettingOutlined, HistoryOutlined, ContainerOutlined, EditOutlined, CheckSquareOutlined, AppstoreOutlined, FormOutlined } from '@ant-design/icons';
import { FileBrowser } from '@vibethis/files';
import { EnvEditor } from '@vibethis/config';
import { GitChanges } from '@vibethis/changes';
import type { DependencyCell } from '@vibethis/shared';
import { navigateToPath } from '@vibethis/shared';
import InputFormsTab from './InputFormsTab';
import { inputActivityService } from '@vibethis/shared';

const { Title } = Typography;
const { TextArea } = Input;

interface SidePanelProps {
  selectedCell: DependencyCell | null;
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

// Config tabs for selected cell
const CellConfigTabs = ({ cell }: { cell: DependencyCell | null }) => {
  const { cellId, subtab } = useParams<{ cellId?: string; subtab?: string }>();
  const navigate = useNavigate();
  
  // Use either the cell ID or the cellId from URL
  const currentCellId = cell?.id || cellId;
  const items = [
    {
      key: 'env',
      label: (
        <span>
          <ContainerOutlined />
          Env
        </span>
      ),
      children: cell ? <EnvEditor cell={cell} /> : <div style={{ padding: '16px' }}>Loading...</div>,
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
        if (currentCellId) {
          const path = navigateToPath({ cellId: currentCellId, tab: 'config', subtab: key });
          navigate(path);
        }
      }}
      items={items}
      tabPosition="left"
      style={{ height: '100%' }}
    />
  );
};

export default function SidePanel({ selectedCell }: SidePanelProps) {
  const { cellId, tab, subtab } = useParams<{ cellId?: string; tab?: string; subtab?: string }>();
  const navigate = useNavigate();
  const [pendingInputCount, setPendingInputCount] = useState(0);
  
  // If we have a cellId in the URL, we should show cell-specific tabs
  const showCellTabs = !!cellId;
  
  // Initialize activeTab from URL or default based on whether we're showing cell tabs
  const defaultTab = showCellTabs ? 'config' : 'config';
  const [activeTab, setActiveTab] = useState<string>(tab || defaultTab);
  
  // Track pending inputs for the current cell
  useEffect(() => {
    const effectiveCellId = selectedCell?.id || cellId;
    if (!effectiveCellId) {
      setPendingInputCount(0);
      return;
    }
    
    // Load initial count
    inputActivityService.getPendingInputs(effectiveCellId).then(inputs => {
      const pending = inputs.filter(i => i.status === 'pending');
      setPendingInputCount(pending.length);
    }).catch(err => {
      console.error('Failed to load pending inputs count:', err);
    });
    
    // Subscribe to updates
    const unsubscribe = inputActivityService.subscribe(effectiveCellId, (event) => {
      // Reload count on any input event
      inputActivityService.getPendingInputs(effectiveCellId).then(inputs => {
        const pending = inputs.filter(i => i.status === 'pending');
        setPendingInputCount(pending.length);
      }).catch(err => {
        console.error('Failed to update pending inputs count:', err);
      });
    });
    
    return unsubscribe;
  }, [selectedCell?.id, cellId]);

  // Update active tab when URL changes
  useEffect(() => {
    if (tab) {
      setActiveTab(tab);
    } else {
      setActiveTab('config');
    }
  }, [tab, showCellTabs]);

  const handleTabChange = (key: string) => {
    setActiveTab(key);
    if (cellId) {
      const path = navigateToPath({ cellId, tab: key });
      navigate(path);
    }
  };

  const handleGitTabChange = (gitTab: string) => {
    if (cellId) {
      const path = navigateToPath({ cellId, tab: 'changes', subtab: gitTab });
      navigate(path);
    }
  };
  
  // Show cell-specific tabs if we have a cellId OR selectedCell
  const items = (showCellTabs || selectedCell) ? [
    {
      key: 'files',
      label: (
        <span>
          <FileOutlined />
          Files
        </span>
      ),
      children: <FileBrowser cell={selectedCell} cellId={cellId} />,
    },
    {
      key: 'inputs',
      label: (
        <span>
          <FormOutlined />
          Inputs
          {pendingInputCount > 0 && (
            <Badge count={pendingInputCount} style={{ marginLeft: 8 }} />
          )}
        </span>
      ),
      children: <InputFormsTab 
        cell={selectedCell} 
        cellId={cellId}
      />,
    },
    {
      key: 'config',
      label: (
        <span>
          <AppstoreOutlined />
          Config
        </span>
      ),
      children: <CellConfigTabs cell={selectedCell} />,
    },
    {
      key: 'changes',
      label: (
        <span>
          <HistoryOutlined />
          Changes
        </span>
      ),
      children: <GitChanges cell={selectedCell} activeTab={subtab} onTabChange={handleGitTabChange} />,
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