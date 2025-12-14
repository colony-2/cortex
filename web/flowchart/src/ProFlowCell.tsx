import type { FC } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Tag, Space } from 'antd';
import InputBadge from './components/InputBadge';

interface ProFlowCellProps {
  data: {
    title: string;
    description?: string;
    logo?: string;
    name: string;
    type: string;
    dependencies: string[];
    onClick?: () => void;
    selected?: boolean;
    pendingInputCount?: number;
    inputUrgency?: 'pending' | 'urgent' | 'overdue';
    projectId?: string;
  };
  id: string;
}

const ProFlowCell: FC<ProFlowCellProps> = ({ data, id }) => {
  const getDependencyColor = (count: number) => {
    if (count === 0) return 'green';
    if (count <= 2) return 'blue';
    if (count <= 4) return 'orange';
    return 'red';
  };

  return (
    <div
      className="graph-cell"
      data-cell-id={id}
      style={{
        width: 200,
        cursor: 'pointer',
        background: '#fff',
        border: data.selected ? '2px solid #1890ff' : '1px solid #d9d9d9',
        borderRadius: '6px',
        boxShadow: data.selected ? '0 4px 12px rgba(24,144,255,0.15)' : '0 2px 8px rgba(0,0,0,0.06)',
        transition: 'all 0.2s ease',
        position: 'relative',
      }}
      onClick={() => {
        // Dispatch global event for cell selection
        window.dispatchEvent(new CustomEvent('cellSelected', { detail: { cellId: id } }));
      }}
    >
      {data.pendingInputCount && data.pendingInputCount > 0 && (
        <InputBadge 
          count={data.pendingInputCount}
          status={data.inputUrgency}
          cellId={id}
          projectId={data.projectId}
          size="small"
        />
      )}
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
          {data.dependencies && data.dependencies.length > 0 && (
            <div>
              <Tag color={getDependencyColor(data.dependencies.length)}>
                {data.dependencies.length} relationships
              </Tag>
            </div>
          )}
        </Space>
      </div>
      
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
};

export default ProFlowCell;
