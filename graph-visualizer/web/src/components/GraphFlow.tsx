import { useState, useEffect, useCallback, useRef } from 'react';
import { FlowView } from '@ant-design/pro-flow';
import { applyNodeChanges } from '@xyflow/react';
import { message, Spin, Card, Button, Alert, Collapse, Splitter } from 'antd';
import { fetchGraph, fetchPositions, savePositions } from '../api';
import type { DependencyGraph, DependencyNode, NodePosition } from '../types';
import ProFlowNode from './ProFlowNode';
import SidePanel from './SidePanel';

interface FlowNode {
  id: string;
  position?: { x: number; y: number };
  type?: string;
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

export default function GraphFlow() {
  const [nodes, setNodes] = useState<FlowNode[]>([]);
  const [edges, setEdges] = useState<FlowEdge[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<DependencyNode | null>(null);
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const graphRef = useRef<DependencyGraph | null>(null);

  const handleNodeClick = useCallback((nodeId: string) => {
    const node = graphRef.current?.nodes.find(n => n.id === nodeId) || null;
    setSelectedNode(node);
  }, []);

  const layoutNodes = useCallback((graphData: DependencyGraph, savedPositions: NodePosition[]) => {
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
        data: {
          title: node.name,
          name: node.name,
          type: node.type,
          dependencies: node.dependencies,
          logo: '📦'
        },
      };
    });

    // Create Pro Flow edges
    const newEdges: FlowEdge[] = graphData.edges && Array.isArray(graphData.edges) 
      ? graphData.edges.map(edge => ({
          id: edge.id,
          source: edge.source,
          target: edge.target,
          type: 'radius',
        }))
      : [];

    setNodes(newNodes);
    setEdges(newEdges);
  }, []);

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
            x: n.position?.x || 0,
            y: n.position?.y || 0
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
    async function loadData() {
      try {
        const [graphData, positions] = await Promise.all([
          fetchGraph(),
          fetchPositions()
        ]);
        
        graphRef.current = graphData;
        layoutNodes(graphData, positions);
        setLoading(false);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to load graph';
        console.error('Graph loading error:', err);
        setError(errorMessage);
        setLoading(false);
        message.error(errorMessage);
      }
    }

    loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Only run once on mount

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
    <Splitter style={{ height: '100vh' }}>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <FlowView
          nodes={nodes}
          edges={edges}
          nodeTypes={{ custom: ProFlowNode }}
          onNodesChange={onNodesChange}
          onNodeClick={(_event: any, node: any) => handleNodeClick(node.id)}
          miniMap
          autoLayout={false}
          background
          style={{ width: '100%', height: '100%' }}
        />
      </Splitter.Panel>
      <Splitter.Panel defaultSize="50%" min="20%" max="80%">
        <SidePanel selectedNode={selectedNode} />
      </Splitter.Panel>
    </Splitter>
  );
}