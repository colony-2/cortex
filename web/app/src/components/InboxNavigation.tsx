import { useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Button, Input, Tooltip } from 'antd';
import { EditOutlined, InboxOutlined, FolderOutlined, ClockCircleOutlined, FileTextOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons';
import { useJobInbox } from '../hooks/useJobInbox';
import { inboxPendingBadge, visibleInboxCells, type InboxSelection } from '../utils/jobInbox';
import CreateCellDialog from './CreateCellDialog';

export default function InboxNavigation({ rail, onCompose, onNavigate, onExpand, draftCount }: {
  draftCount: number; rail: boolean; onCompose: () => void; onNavigate: () => void; onExpand: () => void;
}) {
  const inbox = useJobInbox();
  const location = useLocation();
  const query = new URLSearchParams(location.search);
  const folder = query.get('inbox') ?? 'all', selectedId = query.get('cellId') ?? undefined;
  const [expanded, setExpanded] = useState(false);
  const [search, setSearch] = useState('');
  const [creating, setCreating] = useState(false);
  const cells = expanded ? inbox.ranked.filter(cell => cell.name.toLowerCase().includes(search.toLowerCase())) : visibleInboxCells(inbox.ranked, selectedId);
  const href = (selection: InboxSelection) => `/project/${encodeURIComponent(inbox.tenant)}/jobs${selection.kind === 'all' ? '' : selection.kind === 'cell' ? `?inbox=cell&cellId=${encodeURIComponent(selection.cellId)}` : `?inbox=${selection.kind}`}`;
  const item = (selection: InboxSelection, label: string, icon: React.ReactNode) => {
    const selected = folder === selection.kind && (selection.kind !== 'cell' || selectedId === selection.cellId);
    const badge = selection.kind === 'drafts' ? draftCount || undefined : selection.kind === 'all' ? undefined : inboxPendingBadge(inbox.counts, selection);
    return <Tooltip key={selection.kind === 'cell' ? selection.cellId : selection.kind} title={rail ? label : undefined} placement="right" trigger={['hover', 'focus']}>
      <Link className={`inbox-navigation-item${selected ? ' selected' : ''}`} to={href(selection)} aria-label={label} aria-current={selected ? 'page' : undefined} onClick={onNavigate}>
        <span className="inbox-navigation-icon" aria-hidden>{icon}</span><span className="inbox-navigation-name">{label}</span>
        {badge !== undefined && <span className="inbox-badge" aria-label={badge === '?' ? 'Pending count unavailable' : selection.kind === 'drafts' ? `${badge} drafts` : `${badge} pending responses`}>{badge}</span>}
      </Link>
    </Tooltip>;
  };
  return <nav className={`inbox-navigation${rail ? ' rail' : ''}`} aria-label="Inboxes">
    <Tooltip title={rail ? 'New message' : undefined} placement="right"><Button className="inbox-compose" type="primary" icon={<EditOutlined />} aria-label="New message" onClick={() => { onCompose(); onNavigate(); }}><span className="inbox-navigation-name">New message</span></Button></Tooltip>
    {item({ kind: 'waiting' }, 'Waiting', <ClockCircleOutlined />)}
    {item({ kind: 'drafts' }, 'Drafts', <FileTextOutlined />)}
    <div className="inbox-cell-heading"><span className="inbox-navigation-name">Cells</span><Button type="text" size="small" aria-label={expanded && !rail ? 'Show fewer cells' : 'Search all cells'} aria-expanded={expanded && !rail} onClick={() => { if (rail) { onExpand(); setExpanded(true); } else setExpanded(value => !value); }} icon={<SearchOutlined />} /></div>
    {expanded && !rail && <Input aria-label="Search cells" placeholder="Find a cell" allowClear value={search} onChange={event => setSearch(event.target.value)} />}
    <div className="inbox-cell-scroll">
      {cells.map(cell => item({ kind: 'cell', cellId: cell.id }, cell.name, <span className="inbox-cell-initials">{cell.name.slice(0, 2).toUpperCase()}</span>))}
      {!rail && !inbox.registry.loading && !cells.length && <p className="inbox-nav-notice">No matching cells</p>}
    </div>
    {inbox.registry.error && <p role="status" className="inbox-nav-notice">Cells unavailable <button onClick={inbox.registry.reload}>Retry</button></p>}
    {expanded && !rail && <Button className="inbox-add-cell" type="text" icon={<PlusOutlined />} onClick={() => setCreating(true)}>Add cell</Button>}
    {item({ kind: 'all' }, 'All', <InboxOutlined />)}
    {item({ kind: 'archive' }, 'Archive', <FolderOutlined />)}
    {creating && <CreateCellDialog projectId={inbox.tenant} onClose={() => setCreating(false)} onCreated={() => { setCreating(false); inbox.registry.reload(); }} />}
  </nav>;
}
