<script lang="ts">
  import { Filemanager } from 'wx-svelte-filemanager';
  import type { DependencyNode } from '../types';
  
  interface Props {
    node: DependencyNode | null;
    onClose: () => void;
  }
  
  const { node, onClose }: Props = $props();
  
  let data = $state([]);
  let currentPath = $state('');
  
  $effect(() => {
    if (node) {
      loadFiles(node.id, '');
    }
  });
  
  async function loadFiles(nodeId: string, path: string) {
    try {
      const response = await fetch(`http://localhost:8080/api/files/${nodeId}?path=${encodeURIComponent(path)}`);
      if (!response.ok) throw new Error('Failed to load files');
      
      const result = await response.json();
      currentPath = result.path || '';
      
      // Transform files to FileManager format
      data = result.files.map((file: any) => ({
        id: file.path,
        name: file.name,
        type: file.isDir ? 'folder' : 'file',
        size: file.size,
        date: Date.now(),
        ext: file.type
      }));
    } catch (error) {
      console.error('Failed to load files:', error);
      data = [];
    }
  }
  
  function handleAction(ev: CustomEvent) {
    const { action, data: actionData } = ev.detail;
    
    if (action === 'select-folder' && node) {
      const folder = actionData;
      loadFiles(node.id, folder.id);
    }
  }
</script>

{#if node}
  <div class="file-browser">
    <div class="browser-header">
      <div class="node-info">
        <h3>{node.name}</h3>
        <div class="path">/{node.path}/{currentPath}</div>
      </div>
      <div class="browser-actions">
        <button class="action-btn" onclick={onClose} title="Close file browser">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>
    </div>
    
    <div class="file-manager-container">
      <Filemanager 
        {data}
        on:action={handleAction}
        features={{
          preview: true,
          download: false,
          upload: false,
          delete: false,
          rename: false,
          copy: false,
          move: false,
          create: false
        }}
      />
    </div>
  </div>
{/if}

<style>
  .file-browser {
    height: 100%;
    display: flex;
    flex-direction: column;
    background: #f5f5f5;
  }
  
  .browser-header {
    background: #69b3a2;
    color: white;
    padding: 16px 20px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  }
  
  .node-info h3 {
    margin: 0 0 4px 0;
    font-size: 18px;
  }
  
  .path {
    font-size: 14px;
    opacity: 0.9;
  }
  
  .browser-actions {
    display: flex;
    gap: 8px;
  }
  
  .action-btn {
    background: none;
    border: none;
    color: white;
    cursor: pointer;
    padding: 6px;
    border-radius: 4px;
    transition: background 0.2s;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .action-btn:hover {
    background: rgba(255, 255, 255, 0.2);
  }
  
  .file-manager-container {
    flex: 1;
    overflow: hidden;
  }
  
  :global(.wx-filemanager) {
    height: 100%;
  }
</style>