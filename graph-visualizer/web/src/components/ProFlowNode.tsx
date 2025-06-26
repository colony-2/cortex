import type { FC } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Tag, Space } from 'antd';

interface ProFlowNodeProps {
  data: {
    title: string;
    description?: string;
    logo?: string;
    name: string;
    type: string;
    dependencies: string[];
    onClick?: () => void;
  };
  id: string;
}

const ProFlowNode: FC<ProFlowNodeProps> = ({ data, id }) => {
  const getDependencyColor = (count: number) => {
    if (count === 0) return 'green';
    if (count <= 2) return 'blue';
    if (count <= 4) return 'orange';
    return 'red';
  };

  return (
    <div
      className="graph-node"
      data-node-id={id}
      style={{
        width: 200,
        cursor: 'pointer',
        background: '#fff',
        border: '1px solid #d9d9d9',
        borderRadius: '6px',
        boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
      }}
      onClick={() => {
        // Dispatch global event for node selection
        window.dispatchEvent(new CustomEvent('nodeSelected', { detail: { nodeId: id } }));
      }}
    >
      <Handle type="target" position={Position.Top} />
      
      <div style={{ padding: '8px 12px', borderBottom: '1px solid #f0f0f0' }}>
        <Space>
          <span>{data.logo || '📦'}</span>
          <span style={{ fontWeight: 500 }}>{data.title}</span>
        </Space>
      </div>
      
      <div style={{ padding: '8px 12px' }}>
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
      </div>
      
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
};

export default ProFlowNode;