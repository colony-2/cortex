<script lang="ts">
  import { Handle, Position } from '@xyflow/svelte';
  import type { DependencyNode } from '../types';
  
  interface Props {
    data: {
      node: DependencyNode;
      onFileBrowser?: (nodeId: string) => void;
      isViewingFiles?: boolean;
    };
    selected?: boolean;
  }
  
  const { data, selected = false }: Props = $props();
  
  function handleFileBrowser() {
    data.onFileBrowser?.(data.node.id);
  }
</script>

<div class="node-box" class:selected class:viewing-files={data.isViewingFiles}>
  <Handle type="target" position={Position.Top} />
  
  <div class="node-header">
    <div class="node-title">{data.node.name}</div>
    <div class="node-actions">
      <button class="action-btn" onclick={handleFileBrowser} title="Browse files" aria-label="Browse files">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
        </svg>
      </button>
    </div>
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
    transition: all 0.2s ease;
  }
  
  .node-box.selected {
    border-color: #ff6b6b;
    box-shadow: 0 4px 16px rgba(255, 107, 107, 0.3);
  }
  
  .node-header {
    background: #69b3a2;
    color: white;
    padding: 8px 12px;
    font-weight: bold;
    font-size: 14px;
    border-radius: 6px 6px 0 0;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  
  .node-box.selected .node-header {
    background: #ff6b6b;
  }
  
  .node-box.viewing-files {
    border-color: #ffa500;
    box-shadow: 0 4px 16px rgba(255, 165, 0, 0.4);
  }
  
  .node-box.viewing-files .node-header {
    background: #ffa500;
  }
  
  .node-title {
    flex: 1;
    text-align: center;
  }
  
  .node-actions {
    display: flex;
    gap: 4px;
  }
  
  .action-btn {
    background: none;
    border: none;
    color: white;
    cursor: pointer;
    padding: 2px;
    border-radius: 4px;
    transition: background 0.2s;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .action-btn:hover {
    background: rgba(255, 255, 255, 0.2);
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