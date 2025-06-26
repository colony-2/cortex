import React from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import type { DependencyNode } from '../types';

interface NodeData extends DependencyNode {
  onFileBrowser: (nodeId: string) => void;
  isViewingFiles?: boolean;
}

export default function NodeBox({ data }: NodeProps<NodeData>) {
  const handleFileBrowser = (e: React.MouseEvent) => {
    e.stopPropagation();
    data.onFileBrowser(data.id);
  };

  return (
    <div style={{ 
      padding: '10px', 
      border: '1px solid #ccc', 
      borderRadius: '5px', 
      background: 'white',
      width: '200px',
      minHeight: '80px'
    }}>
      <Handle type="target" position={Position.Top} />
      
      <div style={{ marginBottom: '10px' }}>
        <strong>{data.name}</strong>
        <button 
          onClick={handleFileBrowser}
          title="Browse files"
          aria-label="Browse files"
          style={{ marginLeft: '10px' }}
        >
          📁
        </button>
      </div>
      
      <div>
        <div>Type: {data.type}</div>
        {data.relationships.length > 0 && (
          <div>Dependencies: {data.relationships.length}</div>
        )}
      </div>
      
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}