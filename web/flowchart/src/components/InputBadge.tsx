import type { FC } from 'react';
import { Badge } from 'antd';
import { useNavigate } from 'react-router-dom';
import { navigateToPath } from '@colony2/shared';

interface InputBadgeProps {
  count: number;
  status?: 'pending' | 'urgent' | 'overdue';
  onClick?: () => void;
  size?: 'small' | 'default';
  cellId: string;
  projectId?: string;
}

const InputBadge: FC<InputBadgeProps> = ({ 
  count, 
  status = 'pending', 
  cellId,
  projectId,
  size = 'small' 
}) => {
  const navigate = useNavigate();
  
  if (count === 0) {
    return null;
  }

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent cell selection when clicking badge
    const path = navigateToPath({ 
      projectId,
      cellId, 
      tab: 'inputs'
    });
    navigate(path);
  };

  const getColor = () => {
    switch (status) {
      case 'urgent':
        return '#fa8c16'; // Orange
      case 'overdue':
        return '#f5222d'; // Red
      default:
        return '#1890ff'; // Blue
    }
  };

  const badgeStyle: React.CSSProperties = {
    position: 'absolute',
    top: -8,
    right: -8,
    cursor: 'pointer',
    zIndex: 10,
    animation: status === 'urgent' || status === 'overdue' ? 'pulse 2s infinite' : undefined,
  };

  return (
    <>
      <style>
        {`
          @keyframes pulse {
            0% {
              transform: scale(1);
              opacity: 1;
            }
            50% {
              transform: scale(1.1);
              opacity: 0.8;
            }
            100% {
              transform: scale(1);
              opacity: 1;
            }
          }
        `}
      </style>
      <Badge 
        count={count}
        style={badgeStyle}
        color={getColor()}
        size={size}
        onClick={handleClick}
        title={`${count} pending input${count > 1 ? 's' : ''}`}
      />
    </>
  );
};

export default InputBadge;
