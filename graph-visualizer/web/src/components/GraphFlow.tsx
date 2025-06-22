import { useState, useEffect, useCallback, useRef } from 'react';
import { FlowView } from '@ant-design/pro-flow';
import { message, Spin } from 'antd';
import { fetchGraph, fetchPositions, savePositions } from '../api';
import type { DependencyGraph, DependencyNode, NodePosition } from '../types';
import FileBrowser from './FileBrowser';
import ProFlowNode from './ProFlowNode';

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
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<DependencyNode | null>(null);
  const [showFileBrowser, setShowFileBrowser] = useState(false);
  const [graph, setGraph] = useState<DependencyGraph | null>(null);
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleNodeClick = useCallback((nodeId: string) => {
    const node = graph?.nodes.find(n => n.id === nodeId) || null;
    setSelectedNodeId(nodeId);
    setSelectedNode(node);
    setShowFileBrowser(true);
  }, [graph]);

  const closeFileBrowser = useCallback(() => {
    setShowFileBrowser(false);
    setSelectedNode(null);
    setSelectedNodeId(null);
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
          logo: '📦',
          onFileBrowser: () => handleNodeClick(node.id)
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
  }, [handleNodeClick]);

  const handleNodeDragStop = useCallback((event: any, node: any) => {
    // Save position after drag
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    
    saveTimeoutRef.current = setTimeout(async () => {
      // Collect all node positions
      const positions = nodes.map(n => ({
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
  }, [nodes]);

  useEffect(() => {
    async function loadData() {
      try {
        const [graphData, positions] = await Promise.all([
          fetchGraph(),
          fetchPositions()
        ]);
        
        setGraph(graphData);
        layoutNodes(graphData, positions);
        setLoading(false);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load graph');
        setLoading(false);
        message.error('Failed to load graph data');
      }
    }

    loadData();
  }, [layoutNodes]);

  if (loading) {
    return (
      <div className="loading-container">
        <Spin size="large" tip="Loading graph..." />
      </div>
    );
  }

  if (error) {
    return <div className="error-container">Error: {error}</div>;
  }

  return (
    <div className={`app-container ${showFileBrowser ? 'split' : ''}`}>
      <div className="graph-container">
        <FlowView
          nodes={nodes}
          edges={edges}
          nodeTypes={{ custom: ProFlowNode }}
          onNodeClick={(id: string) => handleNodeClick(id)}
          onPaneClick={() => setSelectedNodeId(null)}
          onNodeDragStop={handleNodeDragStop}
          miniMap
          autoLayout
          background
        />
      </div>
      
      {showFileBrowser && (
        <div className="browser-container">
          <FileBrowser node={selectedNode} onClose={closeFileBrowser} />
        </div>
      )}
    </div>
  );
}