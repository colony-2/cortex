<script lang="ts">
  import { SvelteFlow, Background, Controls, MiniMap } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import { onMount } from 'svelte';
  import type { Node, Edge } from '@xyflow/svelte';
  import type { DependencyGraph, DependencyNode } from '../types';
  import { fetchGraph } from './api';
  import NodeBox from './NodeBox.svelte';
  import dagre from 'dagre';
  
  let nodes = $state<Node[]>([]);
  let edges = $state<Edge[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  
  const nodeTypes = {
    dependency: NodeBox
  };
  
  function layoutGraph(graph: DependencyGraph): { nodes: Node[], edges: Edge[] } {
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
    graph.nodes.forEach((node) => {
      dagreGraph.setNode(node.id, { width: 200, height: 100 });
    });
    
    // Add edges to dagre
    graph.edges.forEach((edge) => {
      dagreGraph.setEdge(edge.source, edge.target);
    });
    
    // Calculate layout
    dagre.layout(dagreGraph);
    
    // Convert to SvelteFlow nodes
    const layoutNodes: Node[] = graph.nodes.map((node) => {
      const nodeWithPosition = dagreGraph.node(node.id);
      return {
        id: node.id,
        type: 'dependency',
        data: { node },
        position: {
          x: nodeWithPosition.x - nodeWithPosition.width / 2,
          y: nodeWithPosition.y - nodeWithPosition.height / 2
        }
      };
    });
    
    // Convert to SvelteFlow edges
    const layoutEdges: Edge[] = graph.edges.map((edge, index) => ({
      id: `e${index}`,
      source: edge.source,
      target: edge.target,
      type: 'smoothstep',
      animated: false,
      style: 'stroke: #999; stroke-width: 2;'
    }));
    
    return { nodes: layoutNodes, edges: layoutEdges };
  }
  
  onMount(async () => {
    try {
      const graph = await fetchGraph();
      const layout = layoutGraph(graph);
      nodes = layout.nodes;
      edges = layout.edges;
      loading = false;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Unknown error';
      loading = false;
    }
  });
</script>

<div class="graph-container">
  {#if loading}
    <div class="loading">Loading graph...</div>
  {:else if error}
    <div class="error">Error: {error}</div>
  {:else}
    <SvelteFlow 
      {nodes} 
      {edges} 
      {nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.2 }}
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

<style>
  .graph-container {
    width: 100vw;
    height: 100vh;
    background: #f5f5f5;
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
</style>