import { useState, useEffect, useCallback, useRef } from 'react';
import { ReactFlow, applyNodeChanges, Background, Controls, MiniMap } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { message, Spin, Card, Button, Alert, Collapse } from 'antd';
import { fetchGraph, fetchPositions, savePositions, type RelationshipGraph, type DependencyNode, type NodePosition } from '@graph-visualizer/shared';
import ProFlowNode from './ProFlowNode';

interface FlowNode {
  id: string;
  position: { x: number; y: number };
  type?: string;
  selectable?: boolean;
  data: {
    title: string;
    description?: string;
    logo?: string;
    [key: string]: any;
  };
}

interface FlowEdge {
  id: string;
  source: string;
  target: string;
  type?: string;
}

export interface GraphFlowProps {
  selectedNodeId?: string;
  onNodeSelect?: (node: DependencyNode | null) => void;
}

export default function GraphFlow({ selectedNodeId, onNodeSelect }: GraphFlowProps) {
  const [nodes, setNodes] = useState<FlowNode[]>([]);
  const [edges, setEdges] = useState<FlowEdge[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<DependencyNode | null>(null);
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const graphRef = useRef<RelationshipGraph | null>(null);
  const isInitialLoad = useRef(true);

  const handleNodeClick = useCallback((nodeId: string) => {
    const node = graphRef.current?.nodes.find(n => n.id === nodeId) || null;
    setSelectedNode(node);
    onNodeSelect?.(node);
  }, [onNodeSelect]);

  const layoutNodes = useCallback((graphData: RelationshipGraph, savedPositions: NodePosition[], selectedNodeId?: string) => {
    // Handle null or undefined nodes
    if (!graphData.nodes || !Array.isArray(graphData.nodes)) {
      setNodes([]);
      setEdges([]);
      return;
    }

    const positionMap = new Map(Array.isArray(savedPositions) ? savedPositions.map(p => [p.nodeId, p]) : []);

    // Create Pro Flow nodes
    const newNodes: FlowNode[] = graphData.nodes.map((node, index) => {
      const savedPosition = positionMap.get(node.id);
      
      // Calculate position if not saved
      const x = savedPosition?.x ?? (index % 4) * 250 + 100;
      const y = savedPosition?.y ?? Math.floor(index / 4) * 150 + 100;
      
      return {
        id: node.id,
        type: 'custom',
        position: { x, y },
        selectable: true,
        data: {
          title: node.name,
          name: node.name,
          type: node.type,
          dependencies: node.dependencies,
          logo: '📦',
          selected: selectedNodeId === node.id
        },
      };
    });

    // Create Pro Flow edges
    const newEdges: FlowEdge[] = graphData.edges && Array.isArray(graphData.edges) 
      ? graphData.edges.map(edge => ({
          id: edge.id,
          source: edge.source,
          target: edge.target,
          type: 'smoothstep',
        }))
      : [];

    setNodes(newNodes);
    setEdges(newEdges);
  }, []); // No dependencies - pure function

  const onNodesChange = useCallback((changes: any) => {
    setNodes((nds) => {
      const updatedNodes = applyNodeChanges(changes, nds as any) as FlowNode[];
      
      // Check if this was a position change (drag)
      const positionChange = changes.find((c: any) => c.type === 'position' && c.dragging === false);
      if (positionChange) {
        // Save positions after drag ends
        if (saveTimeoutRef.current) {
          clearTimeout(saveTimeoutRef.current);
        }
        
        saveTimeoutRef.current = setTimeout(async () => {
          const positions = updatedNodes.map(n => ({
            nodeId: n.id,
            x: n.position.x,
            y: n.position.y
          }));
          
          try {
            await savePositions(positions);
          } catch (err) {
            console.error('Failed to save positions:', err);
            message.error('Failed to save node positions');
          }
        }, 500);
      }
      
      return updatedNodes;
    });
  }, []);

  useEffect(() => {
    // Listen for node selection events (for tests and manual triggering)
    const handleNodeSelection = (event: CustomEvent) => {
      handleNodeClick(event.detail.nodeId);
    };
    
    // Listen for dependency update events
    const handleDependencyUpdate = async () => {
      try {
        const [graphData, positions] = await Promise.all([
          fetchGraph(),
          fetchPositions()
        ]);
        
        graphRef.current = graphData;
        layoutNodes(graphData, positions, selectedNode?.id);
        
        // Update selectedNode with fresh data if one is selected
        if (selectedNode) {
          const updatedNode = graphData.nodes.find(n => n.id === selectedNode.id);
          if (updatedNode && JSON.stringify(updatedNode) !== JSON.stringify(selectedNode)) {
            setSelectedNode(updatedNode);
            onNodeSelect?.(updatedNode);
          }
        }
        
        message.success('Graph updated with new relationships');
      } catch (err) {
        console.error('Failed to refresh graph after relationship update:', err);
        message.error('Failed to refresh graph');
      }
    };
    
    window.addEventListener('nodeSelected', handleNodeSelection as EventListener);
    window.addEventListener('relationshipsUpdated', handleDependencyUpdate as EventListener);
    
    return () => {
      window.removeEventListener('nodeSelected', handleNodeSelection as EventListener);
      window.removeEventListener('relationshipsUpdated', handleDependencyUpdate as EventListener);
    };
  }, [handleNodeClick, layoutNodes, selectedNode?.id, onNodeSelect]);

  // Load data only on initial mount
  useEffect(() => {
    if (!isInitialLoad.current) return;
    
    async function loadData() {
      try {
        const [graphData, positions] = await Promise.all([
          fetchGraph(),
          fetchPositions()
        ]);
        
        graphRef.current = graphData;
        layoutNodes(graphData, positions, selectedNodeId);
        setLoading(false);
        isInitialLoad.current = false;
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to load graph';
        console.error('Graph loading error:', err);
        setError(errorMessage);
        setLoading(false);
        message.error(errorMessage);
      }
    }

    loadData();
  }, []); // Empty dependency array - only load on mount
  
  // Handle external node selection
  useEffect(() => {
    if (selectedNodeId && graphRef.current?.nodes) {
      const node = graphRef.current.nodes.find(n => n.id === selectedNodeId);
      if (node && node.id !== selectedNode?.id) {
        setSelectedNode(node);
        // Update node visual selection state
        setNodes(prevNodes => prevNodes.map(n => ({
          ...n,
          data: {
            ...n.data,
            selected: n.id === selectedNodeId
          }
        })));
      }
    } else if (!selectedNodeId && selectedNode) {
      setSelectedNode(null);
      // Clear visual selection state
      setNodes(prevNodes => prevNodes.map(n => ({
        ...n,
        data: {
          ...n.data,
          selected: false
        }
      })));
    }
  }, [selectedNodeId, selectedNode?.id]); // Include selectedNode?.id to track changes

  if (loading) {
    return (
      <div className="loading-container">
        <Spin size="large" tip="Loading graph..." />
      </div>
    );
  }

  if (error) {
    return (
      <div className="error-container">
        <Card 
          style={{ maxWidth: 600, margin: '0 auto' }}
          title="Failed to Load Graph"
          extra={
            <Button type="primary" onClick={() => window.location.reload()}>
              Retry
            </Button>
          }
        >
          <Alert
            message="Error"
            description={error}
            type="error"
            showIcon
            style={{ marginBottom: 16 }}
          />
          <Collapse ghost>
            <Collapse.Panel header="Troubleshooting Tips" key="1">
              <ul style={{ paddingLeft: '1rem' }}>
                <li>Check if the server is running on the correct port</li>
                <li>Verify the directory contains dependencies.yaml files</li>
                <li>Ensure the .vibestate.db file exists or use the -n flag</li>
                <li>Check the browser console for more details</li>
              </ul>
            </Collapse.Panel>
          </Collapse>
        </Card>
      </div>
    );
  }

  return (
    <div data-testid="react-flow-wrapper" style={{ height: '100%' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={{ custom: ProFlowNode as any }}
        onNodesChange={onNodesChange}
        nodesDraggable={true}
        nodesConnectable={false}
        elementsSelectable={false}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}