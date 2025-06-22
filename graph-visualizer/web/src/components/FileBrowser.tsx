import { useState, useEffect, useMemo } from 'react';
import { Layout, Button, Space, Typography, List, Breadcrumb, Card, Spin, Empty, Tag, Badge } from 'antd';
import { CloseOutlined, FolderOutlined, FileOutlined, HomeOutlined, CodeOutlined, BranchesOutlined } from '@ant-design/icons';
import { fetchFiles } from '../api';
import type { DependencyNode } from '../types';

const { Header, Content } = Layout;
const { Title, Text } = Typography;

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
  path: string;
  isDir: boolean;
}

export default function FileBrowser({ node, onClose }: FileBrowserProps) {
  const [currentPath, setCurrentPath] = useState<string>('');
  const [files, setFiles] = useState<FileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
        title: part,
        onClick: () => setCurrentPath(path),
      });
    });
    
    return items;
  }, [currentPath]);

  useEffect(() => {
    if (!node) return;

    const loadFiles = async () => {
      setLoading(true);
      setError(null);
      
      try {
        console.log('Loading files for node:', node.id);
        console.log('loadFiles called with:', { nodeId: node.id, path: currentPath });
        console.log('Fetching:', `http://localhost:8080/api/files/${node.id}?path=${encodeURIComponent(currentPath)}`);
        
        const result = await fetchFiles(node.id, currentPath);
        console.log('API response:', result);
        
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
        
        console.log('Transformed data:', transformedData);
        console.log('First file:', transformedData[0]);
        console.log('Setting isLoading to false');
        console.log('State after update - isLoading:', false, 'data length:', transformedData.length);
        
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
  }, [node, currentPath]);

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

  if (!node) {
    return null;
  }

  return (
    <Layout className="file-browser" style={{ height: '100%', background: '#fff' }}>
      <Header className="browser-header" style={{ background: '#001529', padding: '0 24px' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', height: '100%' }}>
          <div className="node-info">
            <Title level={3} style={{ color: '#fff', margin: 0 }}>
              {node.name}
            </Title>
            <div className="breadcrumb" style={{ marginTop: '8px' }}>
              <Breadcrumb items={breadcrumbItems} style={{ color: '#fff' }} />
            </div>
          </div>
          <Space className="browser-actions">
            <Button 
              icon={<CodeOutlined />} 
              type="text" 
              style={{ color: '#fff' }}
              title="View terminal"
            >
              Terminal
            </Button>
            <Button 
              icon={<BranchesOutlined />} 
              type="text" 
              style={{ color: '#fff' }}
              title="View dependencies"
            >
              <Badge count={node.dependencies.length} size="small">
                Dependencies
              </Badge>
            </Button>
            <Button 
              icon={<CloseOutlined />} 
              type="text" 
              style={{ color: '#fff' }}
              onClick={onClose}
              title="Close file browser"
            >
              Close
            </Button>
          </Space>
        </div>
      </Header>
      
      <Content style={{ padding: '24px', overflow: 'auto' }}>
        {loading ? (
          <div className="loading" style={{ textAlign: 'center', padding: '40px' }}>
            <Spin size="large" tip="Loading files..." />
          </div>
        ) : error ? (
          <Card>
            <Text type="danger">{error}</Text>
          </Card>
        ) : files.length === 0 ? (
          <Empty className="no-files" description="No files found" />
        ) : (
          <List
            className="file-list"
            dataSource={files}
            renderItem={(file) => (
              <List.Item
                className={`file-item ${file.type}`}
                onClick={() => handleFileClick(file)}
                style={{ cursor: file.isDir ? 'pointer' : 'default' }}
              >
                <List.Item.Meta
                  avatar={getFileIcon(file)}
                  title={
                    <Space>
                      <Text className="file-name" strong={file.isDir}>{file.name}</Text>
                      {file.type === 'file' && file.ext && (
                        <Tag color="blue">{file.ext}</Tag>
                      )}
                    </Space>
                  }
                  description={!file.isDir && <span className="file-size">{formatFileSize(file.size)}</span>}
                />
              </List.Item>
            )}
          />
        )}
      </Content>
    </Layout>
  );
}