import { useState, useEffect, useCallback, useRef } from 'react';
import ReactFlow, {
  Controls,
  Background,
  MiniMap,
  useNodesState,
  useEdgesState,
  type Node,
  type Edge,
  type NodeDragHandler,
  type OnSelectionChangeFunc,
  type NodeTypes,
} from 'reactflow';
import 'reactflow/dist/style.css';
import dagre from 'dagre';
import { fetchGraph, fetchPositions, savePositions } from '../api';
import type { DependencyGraph, DependencyNode, NodePosition } from '../types';
import NodeBox from './NodeBox';
import FileBrowser from './FileBrowser';

const nodeTypes: NodeTypes = {
  dependency: NodeBox,
};

export default function GraphFlow() {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [, setSelectedNodeId] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<DependencyNode | null>(null);
  const [showFileBrowser, setShowFileBrowser] = useState(false);
  const [graph, setGraph] = useState<DependencyGraph | null>(null);
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleFileBrowser = useCallback((nodeId: string) => {
    setSelectedNodeId(nodeId);
    const node = graph?.nodes.find(n => n.id === nodeId) || null;
    setSelectedNode(node);
    setShowFileBrowser(true);
    updateNodeSelection(nodeId);
    updateNodesFileBrowserState(nodeId, true);
  }, [graph]);

  const updateNodesFileBrowserState = useCallback((nodeId: string | null, show: boolean) => {
    setNodes(nodes => nodes.map(node => ({
      ...node,
      data: {
        ...node.data,
        isViewingFiles: show && node.id === nodeId
      }
    })));
  }, [setNodes]);

  const updateNodeSelection = useCallback((nodeId: string | null) => {
    if (!graph) return;
    
    setNodes(nodes => nodes.map(node => ({
      ...node,
      selected: node.id === nodeId,
      style: {
        ...node.style,
        opacity: nodeId ? (node.id === nodeId ? 1 : 0.5) : 1,
      }
    })));

    // Update edge highlighting
    if (nodeId) {
      const node = graph.nodes.find(n => n.id === nodeId);
      if (!node) return;

      const parentEdges = new Set(
        graph.edges
          .filter(e => e.target === nodeId)
          .map(e => e.id)
      );
      
      const childEdges = new Set(
        graph.edges
          .filter(e => e.source === nodeId)
          .map(e => e.id)
      );

      setEdges(edges => edges.map(edge => {
        let edgeClass = '';
        if (parentEdges.has(edge.id)) {
          edgeClass = 'parent-edge';
        } else if (childEdges.has(edge.id)) {
          edgeClass = 'child-edge';
        }
        
        return { 
          ...edge, 
          className: edgeClass,
          animated: edge.source === nodeId || edge.target === nodeId 
        };
      }));
    } else {
      // Reset all edges to default style
      setEdges(edges => edges.map(edge => ({
        ...edge,
        className: '',
        animated: false
      })));
    }
  }, [graph, setNodes, setEdges]);

  const onSelectionChange: OnSelectionChangeFunc = useCallback(({ nodes }) => {
    if (nodes.length > 0) {
      const nodeId = nodes[0].id;
      setSelectedNodeId(nodeId);
      updateNodeSelection(nodeId);
    } else {
      setSelectedNodeId(null);
      updateNodeSelection(null);
    }
  }, [updateNodeSelection]);

  const closeFileBrowser = useCallback(() => {
    setShowFileBrowser(false);
    setSelectedNode(null);
    updateNodeSelection(null);
    updateNodesFileBrowserState(null, false);
  }, [updateNodeSelection, updateNodesFileBrowserState]);

  const layoutNodes = useCallback((graphData: DependencyGraph, savedPositions: NodePosition[]) => {
    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ rankdir: 'TB', ranksep: 100, nodesep: 80 });

    const positionMap = new Map(Array.isArray(savedPositions) ? savedPositions.map(p => [p.nodeId, p]) : []);

    // Create nodes
    const newNodes: Node[] = graphData.nodes.map(node => {
      const savedPosition = positionMap.get(node.id);
      
      if (savedPosition) {
        return {
          id: node.id,
          type: 'dependency',
          position: { x: savedPosition.x, y: savedPosition.y },
          data: { ...node, onFileBrowser: handleFileBrowser },
        };
      } else {
        dagreGraph.setNode(node.id, { width: 200, height: 100 });
        return {
          id: node.id,
          type: 'dependency',
          position: { x: 0, y: 0 },
          data: { ...node, onFileBrowser: handleFileBrowser },
        };
      }
    });

    // Add edges to dagre
    graphData.edges.forEach(edge => {
      dagreGraph.setEdge(edge.source, edge.target);
    });

    // Calculate layout only for nodes without saved positions
    if (!savedPositions || savedPositions.length < graphData.nodes.length) {
      dagre.layout(dagreGraph);
      
      newNodes.forEach(node => {
        if (!positionMap.has(node.id)) {
          const nodeWithPosition = dagreGraph.node(node.id);
          node.position = {
            x: nodeWithPosition.x - 100,
            y: nodeWithPosition.y - 50,
          };
        }
      });
    }

    // Create edges
    const newEdges: Edge[] = graphData.edges.map(edge => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      type: 'default',
    }));

    setNodes(newNodes);
    setEdges(newEdges);
  }, [handleFileBrowser, setNodes, setEdges]);

  const handleNodeDragStop: NodeDragHandler = useCallback(() => {
    // Debounce position saves
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    
    saveTimeoutRef.current = setTimeout(async () => {
      // Collect all node positions
      const positions = nodes.map(node => ({
        nodeId: node.id,
        x: node.position.x,
        y: node.position.y
      }));
      
      try {
        await savePositions(positions);
      } catch (err) {
        console.error('Failed to save positions:', err);
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
      }
    }

    loadData();
  }, [layoutNodes]);

  if (loading) {
    return <div className="loading-container">Loading graph...</div>;
  }

  if (error) {
    return <div className="error-container">Error: {error}</div>;
  }

  return (
    <div className={`app-container ${showFileBrowser ? 'split' : ''}`}>
      <div className="graph-container">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onSelectionChange={onSelectionChange}
          onNodeDragStop={handleNodeDragStop}
          nodeTypes={nodeTypes}
          fitView
          fitViewOptions={{ padding: 0.2 }}
        >
          <Background />
          <Controls />
          <MiniMap 
            style={{ background: '#f5f5f5', border: '1px solid #ddd' }}
            nodeColor="#69b3a2"
          />
        </ReactFlow>
      </div>
      
      {showFileBrowser && (
        <div className="browser-container">
          <FileBrowser node={selectedNode} onClose={closeFileBrowser} />
        </div>
      )}
    </div>
  );
}