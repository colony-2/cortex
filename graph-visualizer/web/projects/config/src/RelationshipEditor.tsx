import { useState, useEffect } from 'react';
import { List, Button, Typography, Space, message, Spin, Modal } from 'antd';
import { 
  DeleteOutlined, 
  PlusOutlined, 
  LinkOutlined,
  ArrowRightOutlined,
  BranchesOutlined,
  ForkOutlined
} from '@ant-design/icons';
import type { DependencyNode } from '@graph-visualizer/shared';

const { Title, Text } = Typography;

export interface RelationshipEditorProps {
  node: DependencyNode;
}

type NodeRelationship = {
  node: DependencyNode;
  type: 'relationship' | 'available' | 'parent' | 'ancestor';
  reason?: string;
};

export default function RelationshipEditor({ node }: RelationshipEditorProps) {
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
      message.error('Failed to load relationship data');
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
        // Current relationship
        result.push({ node, type: 'relationship' });
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
    
    // Sort: relationships first, then available, then parents, then ancestors
    return result.sort((a, b) => {
      const order = { relationship: 0, available: 1, parent: 2, ancestor: 3 };
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

  const handleAddRelationship = async (nodeId: string) => {
    const currentRels = node.dependencies || [];
    const newRelationships = [...currentRels, nodeId];
    await saveRelationships(newRelationships);
  };

  const handleRemoveRelationship = async (nodeId: string) => {
    const targetNode = relationships.find(r => r.node.id === nodeId)?.node;
    const nodeName = targetNode?.name || nodeId;
    
    Modal.confirm({
      title: 'Remove Relationship',
      content: `Are you sure you want to remove the relationship to "${nodeName}"?`,
      okText: 'Remove',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: async () => {
        const currentRels = node.dependencies || [];
        const newRelationships = currentRels.filter(d => d !== nodeId);
        await saveRelationships(newRelationships);
      },
    });
  };

  const saveRelationships = async (newRelationships: string[]) => {
    try {
      setSaving(true);
      
      const response = await fetch(`/api/nodes/${node.id}/relationships`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          relationships: newRelationships,
        }),
      });
      
      if (!response.ok) {
        throw new Error('Failed to save relationships');
      }
      
      message.success('Relationships updated successfully');
      
      // Dispatch event to update the graph
      window.dispatchEvent(new CustomEvent('relationshipsUpdated'));
      
      // Refresh relationships
      await fetchRelationships();
    } catch (error) {
      message.error('Failed to save relationships');
      console.error('Error saving relationships:', error);
    } finally {
      setSaving(false);
    }
  };

  const getIcon = (type: NodeRelationship['type']) => {
    switch (type) {
      case 'relationship':
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
    
    if (item.type === 'relationship') {
      actions.push(
        <Button
          type="text"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleRemoveRelationship(item.node.id)}
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
          onClick={() => handleAddRelationship(item.node.id)}
          disabled={saving}
          size="small"
          style={{ color: '#52c41a' }}
        >
          Add
        </Button>
      );
    }

    return (
      <List.Item actions={actions}>
        <List.Item.Meta
          avatar={getIcon(item.type)}
          title={<Text strong>{item.node.name}</Text>}
          description={
            item.reason ? (
              <Text type="secondary" style={{ fontSize: '12px' }}>{item.reason}</Text>
            ) : item.type === 'available' ? (
              <Text type="secondary" style={{ fontSize: '12px' }}>Available to add as relationship</Text>
            ) : null
          }
        />
      </List.Item>
    );
  };

  if (loading) {
    return (
      <div style={{ padding: '24px', textAlign: 'center' }}>
        <Spin tip="Loading relationships..." />
      </div>
    );
  }

  return (
    <div style={{ padding: '16px', height: '100%', overflowY: 'auto' }}>
      <Title level={4}>Relationships for {node.name}</Title>
      
      <div style={{ marginBottom: '24px' }}>
        <Space direction="vertical" style={{ width: '100%' }}>
          <Text type="secondary">
            Manage relationships between nodes. Circular relationships are automatically prevented.
          </Text>
          
          <Space>
            <Space><LinkOutlined style={{ color: '#1890ff' }} /> Current relationship</Space>
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