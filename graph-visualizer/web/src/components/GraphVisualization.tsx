import React, { useEffect, useRef, useState, useCallback } from 'react';
import * as d3 from 'd3';
import { Graph, Node } from '../types';
import NodeBox from './NodeBox';
import './GraphVisualization.css';

interface GraphVisualizationProps {
  graph: Graph;
}

interface TreeNode extends d3.HierarchyNode<Node> {
  x: number;
  y: number;
}

interface PositionedNode {
  data: Node;
  x: number;
  y: number;
  id: string;
}

const GraphVisualization: React.FC<GraphVisualizationProps> = ({ graph }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [zoomLevel, setZoomLevel] = useState(1);
  const [transform, setTransform] = useState({ x: 0, y: 0, k: 1 });
  const [nodes, setNodes] = useState<PositionedNode[]>([]);
  const [links, setLinks] = useState<{ source: { x: number; y: number }, target: { x: number; y: number } }[]>([]);
  const zoomRef = useRef<d3.ZoomBehavior<HTMLDivElement, unknown> | null>(null);

  useEffect(() => {
    if (!containerRef.current || !graph.nodes.length) return;

    const width = window.innerWidth;
    const height = window.innerHeight - 100;
    const margin = { top: 50, right: 50, bottom: 50, left: 50 };

    // Build hierarchy from graph data
    const nodeMap = new Map(graph.nodes.map(n => [n.id, n]));
    const childrenMap = new Map<string, string[]>();
    
    // Find root nodes (nodes that are not dependencies of any other node)
    const allDependencies = new Set(graph.edges.map(e => e.target));
    const rootNodes = graph.nodes.filter(n => !allDependencies.has(n.id));
    
    // Build children map
    graph.edges.forEach(edge => {
      if (!childrenMap.has(edge.source)) {
        childrenMap.set(edge.source, []);
      }
      childrenMap.get(edge.source)!.push(edge.target);
    });

    // Create hierarchy
    const createHierarchy = (nodeId: string, visited = new Set<string>()): any => {
      if (visited.has(nodeId)) return null;
      visited.add(nodeId);
      
      const node = nodeMap.get(nodeId);
      if (!node) return null;
      
      const children = childrenMap.get(nodeId) || [];
      return {
        ...node,
        children: children
          .map(childId => createHierarchy(childId, visited))
          .filter(child => child !== null)
      };
    };

    // If multiple roots, create a virtual root
    let root;
    if (rootNodes.length === 1) {
      root = createHierarchy(rootNodes[0].id);
    } else if (rootNodes.length > 1) {
      root = {
        id: '__root__',
        name: 'Root',
        path: '',
        dependencies: [],
        children: rootNodes.map(n => createHierarchy(n.id))
      };
    } else {
      // If no clear root, pick the first node
      root = createHierarchy(graph.nodes[0].id);
    }

    if (!root) return;

    // Create tree layout with reduced spacing
    const treeLayout = d3.tree<Node>()
      .size([width - margin.left - margin.right, height - margin.top - margin.bottom])
      .nodeSize([220, 140])
      .separation((a, b) => 1.2);

    const hierarchyRoot = d3.hierarchy(root);
    const treeData = treeLayout(hierarchyRoot) as TreeNode;

    // Center the tree
    const bounds = {
      minX: Infinity,
      maxX: -Infinity,
      minY: Infinity,
      maxY: -Infinity
    };
    
    treeData.descendants().forEach(d => {
      bounds.minX = Math.min(bounds.minX, d.x);
      bounds.maxX = Math.max(bounds.maxX, d.x);
      bounds.minY = Math.min(bounds.minY, d.y);
      bounds.maxY = Math.max(bounds.maxY, d.y);
    });
    
    const treeWidth = bounds.maxX - bounds.minX;
    const centerX = (width - margin.left - margin.right) / 2 - treeWidth / 2 - bounds.minX;
    const centerY = 50 - bounds.minY;

    // Set positioned nodes
    const positionedNodes = treeData.descendants()
      .filter(d => d.data.id !== '__root__')
      .map(d => ({
        data: d.data,
        x: d.x + centerX + margin.left,
        y: d.y + centerY + margin.top,
        id: d.data.id
      }));
    setNodes(positionedNodes);

    // Set links
    const positionedLinks = treeData.links()
      .filter(link => link.source.data.id !== '__root__')
      .map(link => ({
        source: { 
          x: (link.source as any).x + centerX + margin.left, 
          y: (link.source as any).y + centerY + margin.top 
        },
        target: { 
          x: (link.target as any).x + centerX + margin.left, 
          y: (link.target as any).y + centerY + margin.top 
        }
      }));
    setLinks(positionedLinks);

    // Zoom behavior
    const zoom = d3.zoom<HTMLDivElement, unknown>()
      .scaleExtent([0.1, 10])
      .on('zoom', (event) => {
        setTransform({
          x: event.transform.x,
          y: event.transform.y,
          k: event.transform.k
        });
        setZoomLevel(event.transform.k);
      });

    d3.select(containerRef.current).call(zoom);
    zoomRef.current = zoom;

  }, [graph]);

  const handleZoom = useCallback((direction: 'in' | 'out' | 'reset') => {
    if (!containerRef.current || !zoomRef.current) return;
    
    const container = d3.select(containerRef.current);
    
    if (direction === 'reset') {
      container.transition()
        .duration(750)
        .call(zoomRef.current.transform, d3.zoomIdentity);
    } else {
      container.transition()
        .duration(300)
        .call(zoomRef.current.scaleBy, direction === 'in' ? 1.3 : 0.7);
    }
  }, []);

  const handleNodeDoubleClick = useCallback((node: PositionedNode) => {
    if (!containerRef.current || !zoomRef.current) return;
    
    const targetZoom = 4;
    const width = window.innerWidth;
    const height = window.innerHeight;
    
    const newTransform = d3.zoomIdentity
      .translate(width / 2, height / 2)
      .scale(targetZoom)
      .translate(-node.x, -node.y);
    
    d3.select(containerRef.current)
      .transition()
      .duration(750)
      .call(zoomRef.current.transform, newTransform);
  }, []);

  return (
    <div className="graph-container">
      <div className="controls">
        <div className="zoom-controls">
          <button onClick={() => handleZoom('in')} title="Zoom In">+</button>
          <button onClick={() => handleZoom('out')} title="Zoom Out">−</button>
          <button onClick={() => handleZoom('reset')} title="Reset Zoom">⟲</button>
        </div>
        <div className="zoom-info">Zoom: {zoomLevel.toFixed(2)}x</div>
      </div>
      <div 
        ref={containerRef}
        className="graph-content"
        style={{ 
          width: '100%', 
          height: '100vh',
          overflow: 'hidden',
          position: 'relative'
        }}
      >
        <svg 
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: '100%',
            height: '100%',
            pointerEvents: 'none'
          }}
        >
          <g transform={`translate(${transform.x},${transform.y}) scale(${transform.k})`}>
            {/* Render links */}
            {links.map((link, i) => {
              const midY = (link.source.y + link.target.y) / 2;
              return (
                <path
                  key={i}
                  className="link"
                  d={`M${link.source.x},${link.source.y} C${link.source.x},${midY} ${link.target.x},${midY} ${link.target.x},${link.target.y}`}
                  fill="none"
                  stroke="#999"
                  strokeWidth="2"
                />
              );
            })}
          </g>
        </svg>
        
        {/* Render nodes as absolutely positioned divs */}
        {nodes.map(node => {
          const nodeX = node.x * transform.k + transform.x - 100 * transform.k;
          const nodeY = node.y * transform.k + transform.y - 60 * transform.k;
          const nodeWidth = 200 * transform.k;
          const nodeHeight = 120 * transform.k;
          
          return (
            <div
              key={node.id}
              style={{
                position: 'absolute',
                left: nodeX,
                top: nodeY,
                width: nodeWidth,
                height: nodeHeight,
                transform: 'scale(1)', // Prevent additional scaling
                transformOrigin: 'top left'
              }}
            >
              <NodeBox
                node={node.data}
                zoomLevel={zoomLevel}
                onDoubleClick={() => handleNodeDoubleClick(node)}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default GraphVisualization;