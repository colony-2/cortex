import { useId, useRef, useState, type ReactNode } from 'react';
import { Button, Popover } from 'antd';
import { MoreOutlined } from '@ant-design/icons';

export default function DocumentOptions({ storedBytes, children }: { storedBytes?: number; children?: ReactNode }) {
  const panelId = useId();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const close = () => { setOpen(false); trigger.current?.focus(); };
  return <div className="review-document-options"><Popover trigger="click" open={open} overlayClassName="review-options-popover" onOpenChange={visible => { setOpen(visible); if (!visible) trigger.current?.focus(); }}
    afterOpenChange={visible => {
      if (visible) { panel.current?.focus(); return; }
      // Restore dismissal focus without interrupting a control the reviewer
      // has already opened while the popover was finishing its animation.
      const active = document.activeElement;
      if (active !== trigger.current && !panel.current?.contains(active) && active?.closest('button, input, select, textarea, a, [role="combobox"], [role="option"]')) return;
      trigger.current?.focus();
    }}
    content={<div id={panelId} ref={panel} tabIndex={-1} role="dialog" aria-label="Document options" onKeyDown={event => {
      if (event.key === 'Escape') { event.stopPropagation(); close(); }
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        const controls = [...(panel.current?.querySelectorAll<HTMLElement>('button,input') || [])];
        const index = controls.indexOf(document.activeElement as HTMLElement);
        controls[(index + (event.key === 'ArrowDown' ? 1 : -1) + controls.length) % controls.length]?.focus();
        event.preventDefault();
      }
    }}><p>Stored size: {storedBytes?.toLocaleString() ?? 'Unknown'} bytes</p>{children}</div>}>
    <Button ref={trigger} icon={<MoreOutlined />} aria-label="Document options" aria-haspopup="dialog" aria-controls={open ? panelId : undefined} aria-expanded={open} onKeyDown={event => {
      if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true); }
    }} />
  </Popover></div>;
}
