import { DownOutlined } from '@ant-design/icons';
import { Button, Checkbox, Dropdown, type MenuProps } from 'antd';
import { useRef, type ReactNode } from 'react';

export type InboxSelectionMode = 'all' | 'none' | 'finished' | 'unfinished';

const selectionOptions: NonNullable<MenuProps['items']> = [
  { key: 'all', label: 'All' },
  { key: 'none', label: 'None' },
  { key: 'finished', label: 'Finished' },
  { key: 'unfinished', label: 'Unfinished' },
];

export default function InboxSelectionToolbar({ items, selected, kind, disabled, onSelect, children }: {
  items: number;
  selected: number;
  kind: 'jobs' | 'drafts';
  disabled?: boolean;
  onSelect: (mode: InboxSelectionMode) => void;
  children?: ReactNode;
}) {
  const all = items > 0 && selected === items;
  const unavailable = disabled || !items;
  const optionsButton = useRef<HTMLButtonElement>(null);
  return <div className="inbox-selection-toolbar" role="group" aria-label={`${kind === 'jobs' ? 'Job' : 'Draft'} actions`}>
    <div className="inbox-selection-control">
      <Checkbox aria-label={`Select all ${kind}`} checked={all} indeterminate={selected > 0 && !all}
        disabled={unavailable} onChange={() => onSelect(all ? 'none' : 'all')} />
      <Dropdown trigger={['click']} disabled={unavailable} transitionName=""
        getPopupContainer={trigger => trigger.parentElement ?? trigger}
        popupRender={menu => <div onMouseDown={event => event.preventDefault()}>{menu}</div>} menu={{
        items: kind === 'jobs' ? selectionOptions : selectionOptions.slice(0, 2),
        onClick: ({ key }) => {
          optionsButton.current?.focus({ preventScroll: true });
          onSelect(key as InboxSelectionMode);
        },
      }}>
        <Button ref={optionsButton} type="text" className="inbox-selection-options" aria-label={`Select ${kind} by`} aria-haspopup="menu"
          icon={<DownOutlined />} disabled={unavailable} />
      </Dropdown>
    </div>
    {children}
    <span className="inbox-selected-count" role="status">{selected > 0 ? `${selected} selected` : ''}</span>
  </div>;
}
