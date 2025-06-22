import React from 'react';
import { Handle, Position, type NodeProps } from 'reactflow';
import type { DependencyNode } from '../types';

interface NodeData extends DependencyNode {
  onFileBrowser: (nodeId: string) => void;
  isViewingFiles?: boolean;
}

export default function NodeBox({ data, selected }: NodeProps<NodeData>) {
  const handleFileBrowser = (e: React.MouseEvent) => {
    e.stopPropagation();
    data.onFileBrowser(data.id);
  };

  return (
    <div className={`node-box ${data.type === 'module' ? 'is-module' : ''} ${selected ? 'selected' : ''} ${data.isViewingFiles ? 'viewing-files' : ''}`}>
      <Handle type="target" position={Position.Top} />
      
      <div className="node-header">
        <h4>{data.name}</h4>
        <div className="node-actions">
          <button 
            className="action-btn" 
            onClick={handleFileBrowser}
            title="Browse files"
            aria-label="Browse files"
          >
            📁
          </button>
        </div>
      </div>
      
      <div className="node-info">
        <div className="info-item">
          <span className="label">Type:</span>
          <span className="value">{data.type}</span>
        </div>
        {data.dependencies.length > 0 && (
          <div className="info-item">
            <span className="label">Dependencies:</span>
            <span className="value">{data.dependencies.length}</span>
          </div>
        )}
      </div>
      
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}