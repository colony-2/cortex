import React, { useState, useEffect } from 'react';
import type { DependencyNode } from '../types';
import { fetchFiles } from '../api';

interface FileBrowserProps {
  node: DependencyNode | null;
  onClose: () => void;
}

interface FileItem {
  id: string;
  name: string;
  type: 'file' | 'folder';
  size: number;
  date: Date;
  ext: string;
}

export default function FileBrowser({ node, onClose }: FileBrowserProps) {
  const [data, setData] = useState<FileItem[]>([]);
  const [currentPath, setCurrentPath] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (node?.id) {
      console.log('Loading files for node:', node.id);
      loadFiles(node.id, '');
    }
  }, [node?.id]);

  async function loadFiles(nodeId: string, path: string) {
    console.log('loadFiles called with:', { nodeId, path });
    setIsLoading(true);
    
    try {
      const url = `http://localhost:8080/api/files/${nodeId}?path=${encodeURIComponent(path)}`;
      console.log('Fetching:', url);
      
      const result = await fetchFiles(nodeId, path);
      console.log('API response:', result);
      
      // Transform files to FileManager format
      const transformedData = result.files.map((file: any) => ({
        id: file.path,
        name: file.name,
        type: file.isDir ? 'folder' : 'file' as 'folder' | 'file',
        size: file.size,
        date: new Date(),
        ext: file.type
      }));
      
      // Add parent directory if we're in a subdirectory
      const currentSubPath = result.path || path || '';
      if (currentSubPath && currentSubPath !== '') {
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
      
      setData(transformedData);
      setCurrentPath(result.path || '');
      setIsLoading(false);
      console.log('State after update - isLoading:', false, 'data length:', transformedData.length);
      
    } catch (error) {
      console.error('Failed to load files:', error);
      setData([]);
      setCurrentPath('');
      setIsLoading(false);
    }
  }

  function handleFileClick(file: FileItem) {
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

  function handleBreadcrumbClick(targetPath: string) {
    if (node) {
      loadFiles(node.id, targetPath);
    }
  }

  if (!node) return null;

  const pathSegments = currentPath.split('/').filter(Boolean);

  return (
    <div className="file-browser">
      <div className="browser-header">
        <div className="node-info">
          <h3>{node.name}</h3>
          <div className="breadcrumb">
            <button 
              className="breadcrumb-item" 
              onClick={() => handleBreadcrumbClick('')}
            >
              /{node.path}
            </button>
            {currentPath && pathSegments.map((segment, i) => (
              <React.Fragment key={i}>
                <span className="breadcrumb-separator">/</span>
                <button 
                  className="breadcrumb-item" 
                  onClick={() => {
                    const targetPath = pathSegments.slice(0, i + 1).join('/');
                    handleBreadcrumbClick(targetPath);
                  }}
                >
                  {segment}
                </button>
              </React.Fragment>
            ))}
          </div>
        </div>
        <div className="browser-actions">
          <button className="action-btn" title="View terminal" aria-label="View terminal">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="4" y="4" width="16" height="16" rx="2" ry="2"/>
              <path d="M8 12l2 2 2-2"/>
              <path d="M12 12h4"/>
            </svg>
          </button>
          <button className="action-btn" title="View dependencies" aria-label="View dependencies">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="3"/>
              <path d="M12 1v6m0 6v6m11-7h-6m-6 0H1"/>
              <path d="M20.5 7.5L15 13 9.5 7.5M9.5 16.5L15 11l5.5 5.5"/>
            </svg>
          </button>
          <button className="action-btn" onClick={onClose} title="Close file browser" aria-label="Close file browser">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>
      </div>
      
      <div className="file-manager-container">
        {isLoading ? (
          <div className="loading">Loading files...</div>
        ) : data.length > 0 ? (
          <div className="file-list">
            {data.map(file => (
              <button 
                key={file.id}
                className={`file-item ${file.type === 'folder' ? 'folder' : ''}`}
                onClick={() => handleFileClick(file)}
                type="button"
              >
                <span className="file-icon">
                  {file.type === 'folder' ? '📁' : '📄'}
                </span>
                <span className="file-name">{file.name}</span>
                {file.type === 'file' && (
                  <span className="file-size">{file.size} bytes</span>
                )}
              </button>
            ))}
          </div>
        ) : (
          <div className="no-files">No files found</div>
        )}
      </div>
    </div>
  );
}