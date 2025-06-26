import { useState, useEffect } from 'react';
import { List, Tag, Button, Select, Typography, Space, message, Spin, Alert } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import type { DependencyNode } from '../types';

const { Title, Text } = Typography;

interface DependencyEditorProps {
  node: DependencyNode;
}

export default function DependencyEditor({ node }: DependencyEditorProps) {
  const [dependencies, setDependencies] = useState<string[]>([]);
  const [availableNodes, setAvailableNodes] = useState<DependencyNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [selectedNode, setSelectedNode] = useState<string | undefined>(undefined);

  // Fetch current dependencies and available nodes
  useEffect(() => {
    fetchData();
  }, [node.id]);

  const fetchData = async () => {
    try {
      setLoading(true);
      
      // Fetch current graph to get all nodes
      const response = await fetch('/api/graph');
      const data = await response.json();
      
      // Set current dependencies
      setDependencies(node.dependencies || []);
      
      // Filter out nodes that would create circular dependencies
      const validNodes = await getValidDependencyTargets(data.nodes, node);
      setAvailableNodes(validNodes);
    } catch (error) {
      message.error('Failed to load dependency data');
      console.error('Error fetching data:', error);
    } finally {
      setLoading(false);
    }
  };

  // Get nodes that can be added as dependencies without creating cycles
  const getValidDependencyTargets = async (allNodes: DependencyNode[], currentNode: DependencyNode): Promise<DependencyNode[]> => {
    // Filter out:
    // 1. The current node itself
    // 2. Nodes that are already dependencies
    // 3. Nodes that would create circular dependencies (ancestors of current node)
    
    const ancestors = await findAncestors(allNodes, currentNode.id);
    const ancestorIds = new Set(ancestors.map(n => n.id));
    const currentDeps = new Set(currentNode.dependencies || []);
    
    return allNodes.filter(n => 
      n.id !== currentNode.id && 
      !currentDeps.has(n.id) &&
      !ancestorIds.has(n.id)
    );
  };

  // Find all ancestors (nodes that depend on the given node, directly or indirectly)
  const findAncestors = async (allNodes: DependencyNode[], nodeId: string): Promise<DependencyNode[]> => {
    const ancestors: DependencyNode[] = [];
    const visited = new Set<string>();
    
    const findAncestorsRecursive = (targetId: string) => {
      if (visited.has(targetId)) return;
      visited.add(targetId);
      
      // Find nodes that have targetId as a dependency
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

  const handleAddDependency = async () => {
    if (!selectedNode) return;
    
    const newDependencies = [...dependencies, selectedNode];
    await saveDependencies(newDependencies);
    setSelectedNode(undefined);
  };

  const handleRemoveDependency = async (depId: string) => {
    const newDependencies = dependencies.filter(d => d !== depId);
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
      
      setDependencies(newDependencies);
      message.success('Dependencies updated successfully');
      
      // Refresh available nodes
      await fetchData();
    } catch (error) {
      message.error('Failed to save dependencies');
      console.error('Error saving dependencies:', error);
    } finally {
      setSaving(false);
    }
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
      
      <Alert
        message="Dependency Management"
        description="Add or remove dependencies for this node. Circular dependencies are automatically prevented."
        type="info"
        showIcon
        style={{ marginBottom: '16px' }}
      />
      
      <div style={{ marginBottom: '24px' }}>
        <Text strong>Current Dependencies ({dependencies.length})</Text>
        <List
          style={{ marginTop: '12px' }}
          bordered
          dataSource={dependencies}
          locale={{ emptyText: 'No dependencies' }}
          renderItem={(depId) => (
            <List.Item
              actions={[
                <Button
                  type="text"
                  danger
                  icon={<DeleteOutlined />}
                  onClick={() => handleRemoveDependency(depId)}
                  disabled={saving}
                >
                  Remove
                </Button>
              ]}
            >
              <Tag color="blue">{depId}</Tag>
            </List.Item>
          )}
        />
      </div>
      
      <div>
        <Text strong>Add New Dependency</Text>
        <Space style={{ marginTop: '12px', width: '100%' }} direction="vertical">
          <Select
            style={{ width: '100%' }}
            placeholder="Select a node to add as dependency"
            value={selectedNode}
            onChange={setSelectedNode}
            disabled={saving || availableNodes.length === 0}
            showSearch
            filterOption={(input, option) => {
              const label = option?.label as string;
              return label?.toLowerCase().includes(input.toLowerCase());
            }}
            options={availableNodes.map(n => ({
              value: n.id,
              label: `${n.name} (${n.id})`
            }))}
          />
          
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={handleAddDependency}
            disabled={!selectedNode || saving}
            loading={saving}
          >
            Add Dependency
          </Button>
        </Space>
        
        {availableNodes.length === 0 && (
          <Alert
            message="No available nodes"
            description="All other nodes either already depend on this node or are already listed as dependencies."
            type="warning"
            style={{ marginTop: '16px' }}
          />
        )}
      </div>
    </div>
  );
}