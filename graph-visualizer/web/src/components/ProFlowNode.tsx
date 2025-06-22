import { FC } from 'react';
import { Handle, Position } from '@ant-design/pro-flow';
import { Card, Tag, Space, Button } from 'antd';
import { FolderOpenOutlined } from '@ant-design/icons';

interface ProFlowNodeProps {
  data: {
    title: string;
    description?: string;
    logo?: string;
    name: string;
    type: string;
    dependencies: string[];
    onFileBrowser?: () => void;
  };
}

const ProFlowNode: FC<ProFlowNodeProps> = ({ data }) => {
  const getDependencyColor = (count: number) => {
    if (count === 0) return 'green';
    if (count <= 2) return 'blue';
    if (count <= 4) return 'orange';
    return 'red';
  };

  return (
    <Card
      size="small"
      title={
        <Space>
          <span>{data.logo || '📦'}</span>
          <span>{data.title}</span>
        </Space>
      }
      extra={
        <Button
          type="text"
          size="small"
          icon={<FolderOpenOutlined />}
          onClick={(e) => {
            e.stopPropagation();
            data.onFileBrowser?.();
          }}
          title="Browse files"
        />
      }
      style={{ width: 200 }}
      bodyStyle={{ padding: '8px 12px' }}
    >
      <Handle type="target" position={Position.Top} />
      
      <Space direction="vertical" size={4} style={{ width: '100%' }}>
        <div>
          <Tag color="purple">{data.type || 'module'}</Tag>
        </div>
        {data.dependencies.length > 0 && (
          <div>
            <Tag color={getDependencyColor(data.dependencies.length)}>
              {data.dependencies.length} dependencies
            </Tag>
          </div>
        )}
      </Space>
      
      <Handle type="source" position={Position.Bottom} />
    </Card>
  );
};

export default ProFlowNode;