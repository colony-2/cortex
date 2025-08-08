import { useState, useEffect, useMemo } from 'react';
import { Typography, List, Breadcrumb, Card, Spin, Empty, Tag, Space } from 'antd';
import { FolderOutlined, FileOutlined, HomeOutlined } from '@ant-design/icons';
import { fetchFiles, type DependencyCell } from '@vibethis/shared';

const { Text } = Typography;

export interface FileBrowserProps {
  cell: DependencyCell | null;
  cellId?: string;
}

interface FileItem {
  id: string;
  name: string;
  type: 'file' | 'folder';
  size: number;
  date: Date;
  ext: string;
  path: string;
  isDir: boolean;
}

export default function FileBrowser({ cell, cellId }: FileBrowserProps) {
  const [currentPath, setCurrentPath] = useState<string>('');
  const [files, setFiles] = useState<FileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Get the cell ID from either the cell prop or cellId parameter
  const currentCellId = cell?.id || cellId;
  const cellName = cell?.name || cellId || 'Loading...';

  const breadcrumbItems = useMemo(() => {
    const parts = currentPath.split('/').filter(Boolean);
    const items = [
      {
        key: '/',
        title: <HomeOutlined />,
        onClick: () => setCurrentPath(''),
      }
    ];
    
    let pathAccum = '';
    parts.forEach((part, index) => {
      pathAccum += (index === 0 ? '' : '/') + part;
      const path = pathAccum;
      items.push({
        key: path,
        title: <span>{part}</span>,
        onClick: () => setCurrentPath(path),
      });
    });
    
    return items;
  }, [currentPath]);

  useEffect(() => {
    if (!currentCellId) return;

    const loadFiles = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const result = await fetchFiles(currentCellId, currentPath);
        
        // Transform files to our format
        const transformedData = result.files.map((file: any) => ({
          id: file.path,
          name: file.name,
          type: file.isDir ? 'folder' : 'file' as 'folder' | 'file',
          size: file.size,
          date: new Date(),
          ext: file.type,
          path: file.path,
          isDir: file.isDir
        }));
        
        setFiles(transformedData);
      } catch (err) {
        console.error('Failed to load files:', err);
        setError(err instanceof Error ? err.message : 'Failed to load files');
        setFiles([]);
      } finally {
        setLoading(false);
      }
    };

    loadFiles();
  }, [currentCellId, currentPath]);

  const handleFileClick = (file: FileItem) => {
    if (file.isDir) {
      setCurrentPath(file.path);
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const getFileIcon = (file: FileItem) => {
    if (file.isDir) {
      return <FolderOutlined style={{ fontSize: '24px', color: '#1890ff' }} />;
    }
    return <FileOutlined style={{ fontSize: '24px', color: '#52c41a' }} />;
  };

  if (!currentCellId) {
    return (
      <div style={{ padding: '16px' }}>
        <Spin size="large" tip="Loading..." style={{ display: 'block', margin: '40px auto' }} />
      </div>
    );
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '16px 16px 0', borderBottom: '1px solid #f0f0f0' }}>
        <Space direction="vertical" style={{ width: '100%' }} size={8}>
          <Text strong style={{ fontSize: '16px' }}>{cellName}</Text>
          <Breadcrumb items={breadcrumbItems} />
        </Space>
      </div>
      
      <div style={{ flex: 1, padding: '16px', overflow: 'auto' }}>
        {loading ? (
          <div style={{ textAlign: 'center', padding: '40px' }}>
            <Spin size="large" tip="Loading files..." />
          </div>
        ) : error ? (
          <Card>
            <Text type="danger">{error}</Text>
          </Card>
        ) : files.length === 0 ? (
          <Empty description="No files found" />
        ) : (
          <List
            dataSource={files}
            renderItem={(file) => (
              <List.Item
                onClick={() => handleFileClick(file)}
                style={{ 
                  cursor: file.isDir ? 'pointer' : 'default',
                  padding: '12px',
                  borderRadius: '4px',
                  transition: 'background-color 0.2s'
                }}
                onMouseEnter={(e) => {
                  if (file.isDir) {
                    e.currentTarget.style.backgroundColor = '#f5f5f5';
                  }
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = 'transparent';
                }}
              >
                <List.Item.Meta
                  avatar={getFileIcon(file)}
                  title={
                    <Space>
                      <Text strong={file.isDir}>{file.name}</Text>
                      {file.type === 'file' && file.ext && (
                        <Tag color="blue">{file.ext}</Tag>
                      )}
                    </Space>
                  }
                  description={!file.isDir && <span>{formatFileSize(file.size)}</span>}
                />
              </List.Item>
            )}
          />
        )}
      </div>
    </div>
  );
}