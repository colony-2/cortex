<script lang="ts">
  import type { DependencyNode } from '../types';
  
  interface Props {
    node: DependencyNode | null;
    onClose: () => void;
  }
  
  const { node, onClose }: Props = $props();
  
  let data = $state<any[]>([]);
  let currentPath = $state('');
  let isLoading = $state(false);
  let updateKey = $state(0);
  
  // Load files immediately when component mounts with a node
  $effect(() => {
    if (node && node.id) {
      console.log('Loading files for node:', node.id);
      loadFiles(node.id, '');
    }
  });
  
  async function loadFiles(nodeId: string, path: string) {
    console.log('loadFiles called with:', { nodeId, path });
    
    // Set loading state at the beginning
    isLoading = true;
    
    try {
      const url = `http://localhost:8080/api/files/${nodeId}?path=${encodeURIComponent(path)}`;
      console.log('Fetching:', url);
      
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`Failed to load files: ${response.status} ${response.statusText}`);
      }
      
      const result = await response.json();
      console.log('API response:', result);
      
      // Transform files to FileManager format
      const transformedData = result.files.map((file: any) => ({
        id: file.path,
        name: file.name,
        type: file.isDir ? 'folder' : 'file',
        size: file.size,
        date: new Date(),  // Use Date object instead of timestamp
        ext: file.type
      }));
      
      // Add parent directory if we're in a subdirectory
      if (path && path !== '') {
        transformedData.unshift({
          id: '..',
          name: '..',
          type: 'folder',
          size: 0,
          date: new Date(),
          ext: ''
        });
      }
      
      console.log('Transformed data:', transformedData);
      console.log('First file:', transformedData[0]);
      console.log('Setting isLoading to false');
      
      // Update state - force reactivity by creating new array
      data = [...transformedData];
      currentPath = result.path || '';
      
      // Force component update
      isLoading = false;
      updateKey = updateKey + 1;
      console.log('State after update - isLoading:', isLoading, 'data length:', data.length);
      
    } catch (error) {
      console.error('Failed to load files:', error);
      data = [];
      currentPath = '';
      
      isLoading = false;
      updateKey = updateKey + 1;
    }
  }
  
  function handleFileClick(file: any) {
    if (node && file.type === 'folder') {
      if (file.id === '..') {
        // Navigate to parent directory
        const parentPath = currentPath.split('/').slice(0, -1).join('/');
        loadFiles(node.id, parentPath);
      } else {
        loadFiles(node.id, file.id);
      }
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
        <button class="action-btn" title="View terminal" aria-label="View terminal">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="4" y="4" width="16" height="16" rx="2" ry="2"/>
            <path d="M8 12l2 2 2-2"/>
            <path d="M12 12h4"/>
          </svg>
        </button>
        <button class="action-btn" title="View dependencies" aria-label="View dependencies">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="3"/>
            <path d="M12 1v6m0 6v6m11-7h-6m-6 0H1"/>
            <path d="M20.5 7.5L15 13 9.5 7.5M9.5 16.5L15 11l5.5 5.5"/>
          </svg>
        </button>
        <button class="action-btn" onclick={onClose} title="Close file browser" aria-label="Close file browser">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 6L6 18M6 6l12 12"/>
          </svg>
        </button>
      </div>
    </div>
    
    <div class="file-manager-container">
      {#key updateKey}
        {#if isLoading}
          <div class="loading">Loading files...</div>
        {:else if data.length > 0}
          <div class="file-list">
            {#each data as file}
              <button 
                class="file-item" 
                class:folder={file.type === 'folder'} 
                onclick={() => handleFileClick(file)}
                type="button"
              >
                <span class="file-icon">
                  {#if file.type === 'folder'}
                    📁
                  {:else}
                    📄
                  {/if}
                </span>
                <span class="file-name">{file.name}</span>
                {#if file.type === 'file'}
                  <span class="file-size">{file.size} bytes</span>
                {/if}
              </button>
            {/each}
          </div>
        {:else}
          <div class="no-files">No files found</div>
        {/if}
      {/key}
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
    display: flex;
    flex-direction: column;
  }
  
  .file-list {
    flex: 1;
    overflow-y: auto;
    padding: 16px;
  }

  .file-item {
    display: flex;
    align-items: center;
    padding: 8px 12px;
    margin-bottom: 4px;
    background: white;
    border-radius: 4px;
    cursor: pointer;
    transition: background 0.2s;
    border: none;
    width: 100%;
    text-align: left;
    font-family: inherit;
  }

  .file-item:hover {
    background: #f0f0f0;
  }

  .file-item.folder {
    font-weight: 500;
  }

  .file-icon {
    margin-right: 8px;
    font-size: 18px;
  }

  .file-name {
    flex: 1;
    font-size: 14px;
  }

  .file-size {
    font-size: 12px;
    color: #666;
    margin-left: 8px;
  }
  
  .loading, .no-files {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    font-size: 16px;
    color: #666;
  }
  
  
  /* Fix file manager dialog styling */
  :global(.wx-portal) {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    z-index: 9999;
  }
  
  :global(.wx-modal) {
    background: rgba(0, 0, 0, 0.5);
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  :global(.wx-window) {
    background: white;
    border-radius: 8px;
    box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
    max-width: 90%;
    max-height: 90%;
  }
</style>