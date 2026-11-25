import { Button, Tooltip } from 'antd';
import { InboxOutlined, ReloadOutlined, RollbackOutlined, StopOutlined, ApartmentOutlined } from '@ant-design/icons';
import { Link, useLocation } from 'react-router-dom';
import type { ReactNode } from 'react';

export function CancelJobIcon({onClick, disabled}: {onClick: () => void; disabled?: boolean}) {
  return <Tooltip title="Cancel job"><Button type="text" size="small" danger aria-label="Cancel job"
    icon={<StopOutlined />} disabled={disabled} onClick={onClick} /></Tooltip>;
}

export function JobActionIcons({graph, onRefresh, onRestart, onArchive, onCancel, children}: {
  graph: string; onRefresh: () => void; onRestart: () => void; onArchive: () => void;
  onCancel?: () => void; children?: ReactNode;
}) {
  const location = useLocation();
  return <div className="job-action-icons">
    <Tooltip title="Graph"><Link className="job-icon-link" aria-label="Graph" to={graph} state={location.state}><ApartmentOutlined /></Link></Tooltip>
    <Tooltip title="Refresh"><Button type="text" size="small" aria-label="Refresh" icon={<ReloadOutlined />} onClick={onRefresh} /></Tooltip>
    <Tooltip title="Restart"><Button type="text" size="small" aria-label="Restart" icon={<RollbackOutlined />} onClick={onRestart} /></Tooltip>
    <Tooltip title="Archive"><Button type="text" size="small" aria-label="Archive" icon={<InboxOutlined />} onClick={onArchive} /></Tooltip>
    {onCancel && <CancelJobIcon onClick={onCancel} />}
    {children}
  </div>;
}
