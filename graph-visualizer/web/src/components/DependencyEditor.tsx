import { useState, useEffect } from 'react';
import { List, Button, Typography, Space, message, Spin, Tooltip } from 'antd';
import { 
  DeleteOutlined, 
  PlusOutlined, 
  LinkOutlined,
  ArrowRightOutlined,
  BranchesOutlined,
  StopOutlined,
  ForkOutlined
} from '@ant-design/icons';
import type { DependencyNode } from '../types';

const { Title, Text } = Typography;

interface DependencyEditorProps {
  node: DependencyNode;
}

type NodeRelationship = {
  node: DependencyNode;
  type: 'dependency' | 'available' | 'parent' | 'ancestor';
  reason?: string;
};

export default function DependencyEditor({ node }: DependencyEditorProps) {
  const [relationships, setRelationships] = useState<NodeRelationship[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    fetchRelationships();
  }, [node.id, node.dependencies]); // Re-fetch when node or its dependencies change

  const fetchRelationships = async () => {
    try {
      setLoading(true);
      
      // Fetch current graph to get all nodes
      const response = await fetch('/api/graph');
      const data = await response.json();
      
      // Find the current node in the fresh data
      const currentNode = data.nodes.find((n: DependencyNode) => n.id === node.id) || node;
      
      // Categorize all nodes based on their relationship to the current node
      const categorized = await categorizeNodes(data.nodes, currentNode);
      setRelationships(categorized);
    } catch (error) {
      message.error('Failed to load dependency data');
      console.error('Error fetching data:', error);
    } finally {
      setLoading(false);
    }
  };

  const categorizeNodes = async (allNodes: DependencyNode[], currentNode: DependencyNode): Promise<NodeRelationship[]> => {
    const result: NodeRelationship[] = [];
    const currentDeps = new Set(currentNode.dependencies || []);
    
    // Find all ancestors (nodes that depend on current node)
    const ancestors = await findAncestors(allNodes, currentNode.id);
    const ancestorIds = new Set(ancestors.map(n => n.id));
    
    for (const node of allNodes) {
      if (node.id === currentNode.id) continue; // Skip self
      
      if (currentDeps.has(node.id)) {
        // Current dependency
        result.push({ node, type: 'dependency' });
      } else if (node.dependencies?.includes(currentNode.id)) {
        // Direct parent
        result.push({ node, type: 'parent', reason: 'Depends on this node' });
      } else if (ancestorIds.has(node.id)) {
        // Ancestor (indirect parent)
        result.push({ node, type: 'ancestor', reason: 'Would create circular dependency' });
      } else {
        // Available to add
        result.push({ node, type: 'available' });
      }
    }
    
    // Sort: dependencies first, then available, then parents, then ancestors
    return result.sort((a, b) => {
      const order = { dependency: 0, available: 1, parent: 2, ancestor: 3 };
      return order[a.type] - order[b.type];
    });
  };

  const findAncestors = async (allNodes: DependencyNode[], nodeId: string): Promise<DependencyNode[]> => {
    const ancestors: DependencyNode[] = [];
    const visited = new Set<string>();
    
    const findAncestorsRecursive = (targetId: string) => {
      if (visited.has(targetId)) return;
      visited.add(targetId);
      
      const parents = allNodes.filter(n => 
        n.dependencies && n.dependencies.includes(targetId)
      );
      
      parents.forEach(parent => {
        if (!ancestors.find(a => a.id === parent.id)) {
          ancestors.push(parent);
        }
        findAncestorsRecursive(parent.id);
      });
    };
    
    findAncestorsRecursive(nodeId);
    return ancestors;
  };

  const handleAddDependency = async (nodeId: string) => {
    const currentDeps = node.dependencies || [];
    const newDependencies = [...currentDeps, nodeId];
    await saveDependencies(newDependencies);
  };

  const handleRemoveDependency = async (nodeId: string) => {
    const currentDeps = node.dependencies || [];
    const newDependencies = currentDeps.filter(d => d !== nodeId);
    await saveDependencies(newDependencies);
  };

  const saveDependencies = async (newDependencies: string[]) => {
    try {
      setSaving(true);
      
      const response = await fetch(`/api/nodes/${node.id}/dependencies`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          dependencies: newDependencies,
        }),
      });
      
      if (!response.ok) {
        throw new Error('Failed to save dependencies');
      }
      
      message.success('Dependencies updated successfully');
      
      // Dispatch event to update the graph
      window.dispatchEvent(new CustomEvent('dependenciesUpdated'));
      
      // Refresh relationships
      await fetchRelationships();
    } catch (error) {
      message.error('Failed to save dependencies');
      console.error('Error saving dependencies:', error);
    } finally {
      setSaving(false);
    }
  };

  const getIcon = (type: NodeRelationship['type']) => {
    switch (type) {
      case 'dependency':
        return <LinkOutlined style={{ color: '#1890ff' }} />;
      case 'available':
        return <ArrowRightOutlined style={{ color: '#52c41a' }} />;
      case 'parent':
        return <BranchesOutlined style={{ color: '#fa8c16' }} />;
      case 'ancestor':
        return <ForkOutlined style={{ color: '#ff4d4f' }} />;
    }
  };


  const renderItem = (item: NodeRelationship) => {
    const actions = [];
    
    if (item.type === 'dependency') {
      actions.push(
        <Button
          type="text"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleRemoveDependency(item.node.id)}
          disabled={saving}
          size="small"
        >
          Remove
        </Button>
      );
    } else if (item.type === 'available') {
      actions.push(
        <Button
          type="text"
          icon={<PlusOutlined />}
          onClick={() => handleAddDependency(item.node.id)}
          disabled={saving}
          size="small"
          style={{ color: '#52c41a' }}
        >
          Add
        </Button>
      );
    } else {
      actions.push(
        <Tooltip title={item.reason}>
          <Button
            type="text"
            icon={<StopOutlined />}
            disabled
            size="small"
          >
            Cannot Add
          </Button>
        </Tooltip>
      );
    }

    return (
      <List.Item actions={actions}>
        <List.Item.Meta
          avatar={getIcon(item.type)}
          title={<Text strong>{item.node.name}</Text>}
          description={
            item.reason && <Text type="secondary" style={{ fontSize: '12px' }}>{item.reason}</Text>
          }
        />
      </List.Item>
    );
  };

  if (loading) {
    return (
      <div style={{ padding: '24px', textAlign: 'center' }}>
        <Spin tip="Loading dependencies..." />
      </div>
    );
  }

  return (
    <div style={{ padding: '16px', height: '100%', overflowY: 'auto' }}>
      <Title level={4}>Dependencies for {node.name}</Title>
      
      <div style={{ marginBottom: '24px' }}>
        <Space direction="vertical" style={{ width: '100%' }}>
          <Text type="secondary">
            Manage dependencies between nodes. Circular dependencies are automatically prevented.
          </Text>
          
          <Space>
            <Space><LinkOutlined style={{ color: '#1890ff' }} /> Current dependency</Space>
            <Space><ArrowRightOutlined style={{ color: '#52c41a' }} /> Can be added</Space>
            <Space><BranchesOutlined style={{ color: '#fa8c16' }} /> Direct parent</Space>
            <Space><ForkOutlined style={{ color: '#ff4d4f' }} /> Indirect parent</Space>
          </Space>
        </Space>
      </div>
      
      <List
        dataSource={relationships}
        renderItem={renderItem}
        loading={loading}
        locale={{ emptyText: 'No other nodes in the system' }}
        style={{ marginTop: '16px' }}
      />
    </div>
  );
}