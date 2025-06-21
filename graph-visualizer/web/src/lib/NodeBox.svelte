<script lang="ts">
  import { Handle, Position } from '@xyflow/svelte';
  import type { DependencyNode } from '../types';
  
  interface Props {
    data: {
      node: DependencyNode;
    };
  }
  
  const { data }: Props = $props();
</script>

<div class="node-box">
  <Handle type="target" position={Position.Top} />
  
  <div class="node-header">
    {data.node.name}
  </div>
  
  <div class="node-content">
    <div class="node-path">{data.node.path}</div>
    {#if data.node.dependencies.length > 0}
      <div class="node-deps">
        <small>Dependencies: {data.node.dependencies.length}</small>
      </div>
    {/if}
  </div>
  
  <Handle type="source" position={Position.Bottom} />
</div>

<style>
  .node-box {
    background: white;
    border: 2px solid #69b3a2;
    border-radius: 8px;
    width: 200px;
    min-height: 80px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  }
  
  .node-header {
    background: #69b3a2;
    color: white;
    padding: 8px 12px;
    font-weight: bold;
    font-size: 14px;
    text-align: center;
    border-radius: 6px 6px 0 0;
  }
  
  .node-content {
    padding: 12px;
  }
  
  .node-path {
    font-size: 12px;
    color: #666;
    margin-bottom: 8px;
  }
  
  .node-deps {
    font-size: 11px;
    color: #999;
  }
  
  :global(.svelte-flow__handle) {
    background: #69b3a2;
    width: 8px;
    height: 8px;
  }
</style>