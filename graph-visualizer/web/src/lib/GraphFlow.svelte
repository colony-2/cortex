<script lang="ts">
  import { SvelteFlow, Background, Controls, MiniMap } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import { onMount } from 'svelte';
  import type { Node, Edge } from '@xyflow/svelte';
  import type { DependencyGraph, DependencyNode } from '../types';
  import { fetchGraph, fetchPositions, savePositions } from './api';
  import NodeBox from './NodeBox.svelte';
  import FileBrowser from './FileBrowser.svelte';
  import dagre from 'dagre';
  
  let nodes = $state.raw<Node[]>([]);
  let edges = $state.raw<Edge[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let selectedNodeId = $state<string | null>(null);
  let selectedNode = $state<DependencyNode | null>(null);
  let showFileBrowser = $state(false);
  let graph = $state<DependencyGraph | null>(null);
  let flowKey = $state(0);
  let saveTimeout: NodeJS.Timeout | null = null;
  
  const nodeTypes = {
    dependency: NodeBox
  };
  
  function handleFileBrowser(nodeId: string) {
    selectedNodeId = nodeId;
    selectedNode = graph?.nodes.find(n => n.id === nodeId) || null;
    showFileBrowser = true;
    updateNodeSelection(nodeId);
  }
  
  function handleNodeClick(params: { node: Node; event: MouseEvent }) {
    const nodeId = params.node.id;
    selectedNodeId = nodeId;
    updateNodeSelection(nodeId);
  }
  
  function updateNodeSelection(nodeId: string | null) {
    if (!graph) return;
    
    // Update node data to include selection state
    nodes = nodes.map(node => ({
      ...node,
      data: {
        ...node.data,
        onFileBrowser: handleFileBrowser
      },
      selected: node.id === nodeId
    }));
    
    // Update edge styles based on selection
    if (nodeId) {
      const parentEdges = new Set<string>();
      const childEdges = new Set<string>();
      
      edges.forEach((edge) => {
        if (edge.target === nodeId) {
          parentEdges.add(edge.id);
        } else if (edge.source === nodeId) {
          childEdges.add(edge.id);
        }
      });
      
      // Create new edges array to ensure reactivity
      const newEdges = edges.map((edge) => {
        let edgeClass = '';
        
        if (parentEdges.has(edge.id)) {
          edgeClass = 'parent-edge'; // Green for parents
        } else if (childEdges.has(edge.id)) {
          edgeClass = 'child-edge'; // Blue for children
        }
        
        return { 
          ...edge, 
          class: edgeClass,
          animated: edge.source === nodeId || edge.target === nodeId 
        };
      });
      
      edges = [...newEdges];
    } else {
      // Reset all edges to default style
      edges = edges.map(edge => ({
        ...edge,
        class: '',
        animated: false
      }));
    }
  }
  
  function layoutGraph(graphData: DependencyGraph): { nodes: Node[], edges: Edge[] } {
    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ 
      rankdir: 'TB',
      ranksep: 80,
      nodesep: 50,
      marginx: 50,
      marginy: 50
    });
    
    // Add nodes to dagre
    graphData.nodes.forEach((node) => {
      dagreGraph.setNode(node.id, { width: 200, height: 100 });
    });
    
    // Add edges to dagre
    graphData.edges.forEach((edge) => {
      dagreGraph.setEdge(edge.source, edge.target);
    });
    
    // Calculate layout
    dagre.layout(dagreGraph);
    
    // Convert to SvelteFlow nodes
    const layoutNodes: Node[] = graphData.nodes.map((node) => {
      const nodeWithPosition = dagreGraph.node(node.id);
      return {
        id: node.id,
        type: 'dependency',
        data: { 
          node,
          onFileBrowser: handleFileBrowser
        },
        position: {
          x: nodeWithPosition.x - nodeWithPosition.width / 2,
          y: nodeWithPosition.y - nodeWithPosition.height / 2
        }
      };
    });
    
    // Convert to SvelteFlow edges
    const layoutEdges: Edge[] = graphData.edges.map((edge, index) => ({
      id: `e${index}`,
      source: edge.source,
      target: edge.target,
      type: 'smoothstep',
      animated: false,
      class: ''
    }));
    
    return { nodes: layoutNodes, edges: layoutEdges };
  }
  
  onMount(async () => {
    try {
      const graphData = await fetchGraph();
      graph = graphData;
      const layout = layoutGraph(graphData);
      nodes = layout.nodes;
      edges = layout.edges;
      
      // Load saved positions
      try {
        const savedPositions = await fetchPositions();
        
        // Apply saved positions to nodes
        nodes = nodes.map(node => {
          const savedPos = savedPositions[node.id];
          if (savedPos) {
            return {
              ...node,
              position: {
                x: savedPos.x,
                y: savedPos.y
              }
            };
          }
          return node;
        });
      } catch (err) {
        console.warn('No saved positions or error loading them:', err);
      }
      
      loading = false;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unknown error';
      loading = false;
    }
  });
  
  function closeFileBrowser() {
    showFileBrowser = false;
    selectedNodeId = null;
    selectedNode = null;
    updateNodeSelection(null);
  }
  
  function handleNodeDragStop(params: { node: Node; event: MouseEvent }) {
    // Debounce position saves
    if (saveTimeout) {
      clearTimeout(saveTimeout);
    }
    
    saveTimeout = setTimeout(async () => {
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
    }, 500); // Save after 500ms of no dragging
  }
</script>

<div class="app-container" class:split={showFileBrowser}>
  <div class="graph-container">
    {#if loading}
      <div class="loading">Loading graph...</div>
    {:else if error}
      <div class="error">Error: {error}</div>
    {:else}
      <SvelteFlow 
        bind:nodes
        bind:edges
        {nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        onnodeclick={handleNodeClick}
        onnodedragstop={handleNodeDragStop}
      >
        <Background />
        <Controls />
        <MiniMap 
          style="background: #f5f5f5; border: 1px solid #ddd;"
          nodeColor="#69b3a2"
        />
      </SvelteFlow>
    {/if}
  </div>
  
  {#if showFileBrowser}
    <div class="browser-container">
      <FileBrowser node={selectedNode} onClose={closeFileBrowser} />
    </div>
  {/if}
</div>

<style>
  .app-container {
    display: flex;
    width: 100vw;
    height: 100vh;
    overflow: hidden;
  }
  
  .graph-container {
    flex: 1;
    height: 100vh;
    background: #f5f5f5;
    transition: all 0.3s ease;
  }
  
  .app-container.split .graph-container {
    width: 60%;
  }
  
  .browser-container {
    width: 40%;
    height: 100vh;
    border-left: 1px solid #ddd;
    background: white;
    overflow: hidden;
    animation: slideIn 0.3s ease;
  }
  
  @keyframes slideIn {
    from {
      transform: translateX(100%);
    }
    to {
      transform: translateX(0);
    }
  }
  
  .loading, .error {
    display: flex;
    justify-content: center;
    align-items: center;
    height: 100vh;
    font-size: 18px;
  }
  
  .error {
    color: #d32f2f;
  }
  
  :global(.parent-edge .svelte-flow__edge-path) {
    stroke: #4CAF50 !important;
    stroke-width: 3 !important;
  }
  
  :global(.child-edge .svelte-flow__edge-path) {
    stroke: #2196F3 !important;
    stroke-width: 3 !important;
  }
  
  :global(.svelte-flow__edge-path) {
    stroke: #999;
    stroke-width: 2;
  }
</style>